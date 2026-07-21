// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// xml.Value accessors and the internal node model. `xml.decode` returns an
// opaque `xml.Value` (a KindObject wrapping the parsed tree); the functions
// here are the only way to reach inside it, since operators, `[index]`, and
// `.field` all reject an object - the same shape as `json` / `toml`.
//
// A node is one of two kinds, encoded as an ordered `map` so the tree is an
// ordinary interpreter Value (KindObject can only wrap a Value):
//
//	element: { kind: "element", name: <string>, attrs: <map string->string>,
//	           children: <list of node> }
//	text:    { kind: "text", text: <string> }
//
// `children` keeps text and element nodes in document order (so mixed content
// and whitespace round-trip); `xml.children` exposes only the element children
// and `xml.text` concatenates the text ones.
//
// Navigation uses a small XPath-style path dialect in place of JSON Pointer:
// `/`-separated steps of `name`, `name[k]` (the 1-based k-th same-named element
// child), or `*` (any element child), relative to the passed node.
package xmllib

import (
	"fmt"
	"strconv"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix (`xml.`) and the `use` name.
const LibraryName = "xml"

// ---- node model ----

func sv(s string) interpreter.Value { return interpreter.StringVal(s) }

// stringMap builds an ordered `map` Value with string keys (the node encoding
// and attribute tables); the value type is left generic.
func stringMap(entries []interpreter.MapEntry) interpreter.Value {
	st := parser.PrimitiveType(parser.TypeString)
	return interpreter.Value{Kind: interpreter.KindMap, Map: entries, KeyTyp: &st}
}

// nodeList wraps element/text nodes as the internal `children` list.
func nodeList(nodes []interpreter.Value) interpreter.Value {
	return interpreter.Value{Kind: interpreter.KindList, List: nodes}
}

// elementNode builds an element node from its name, attributes, and children.
func elementNode(name string, attrs []interpreter.MapEntry, children []interpreter.Value) interpreter.Value {
	return stringMap([]interpreter.MapEntry{
		{Key: sv("kind"), Value: sv("element")},
		{Key: sv("name"), Value: sv(name)},
		{Key: sv("attrs"), Value: stringMap(attrs)},
		{Key: sv("children"), Value: nodeList(children)},
	})
}

// textNode builds a text (character-data) node.
func textNode(s string) interpreter.Value {
	return stringMap([]interpreter.MapEntry{
		{Key: sv("kind"), Value: sv("text")},
		{Key: sv("text"), Value: sv(s)},
	})
}

// mapField looks up a string key in a node/attrs map.
func mapField(m interpreter.Value, key string) (interpreter.Value, bool) {
	if m.Kind != interpreter.KindMap {
		return interpreter.Value{}, false
	}
	for _, e := range m.Map {
		if e.Key.Kind == interpreter.KindString && e.Key.Str == key {
			return e.Value, true
		}
	}
	return interpreter.Value{}, false
}

func nodeKind(n interpreter.Value) string {
	if v, ok := mapField(n, "kind"); ok && v.Kind == interpreter.KindString {
		return v.Str
	}
	return ""
}

func nodeName(n interpreter.Value) string {
	if v, ok := mapField(n, "name"); ok && v.Kind == interpreter.KindString {
		return v.Str
	}
	return ""
}

func nodeAttrs(n interpreter.Value) interpreter.Value {
	v, _ := mapField(n, "attrs")
	return v
}

func nodeChildren(n interpreter.Value) []interpreter.Value {
	if v, ok := mapField(n, "children"); ok && v.Kind == interpreter.KindList {
		return v.List
	}
	return nil
}

// elementChildren returns just the element (non-text) children of n.
func elementChildren(n interpreter.Value) []interpreter.Value {
	var out []interpreter.Value
	for _, c := range nodeChildren(n) {
		if nodeKind(c) == "element" {
			out = append(out, c)
		}
	}
	return out
}

// ---- argument plumbing ----

func takeNode(fn string, v interpreter.Value) (interpreter.Value, error) {
	inner, ok := v.AsObject(LibraryName, "Value")
	if !ok {
		return interpreter.Value{}, fmt.Errorf("%s: argument must be an xml.Value, got %s", fn, v.Kind)
	}
	return inner, nil
}

func wrap(n interpreter.Value) interpreter.Value {
	return interpreter.ObjectVal(LibraryName, "Value", n)
}

// valueType is the parser type stamped on a returned `list of xml.Value` so it
// binds to a declared `list of xml.Value`.
func valueType() parser.Type {
	return parser.Type{Kind: parser.TypeStruct, StructNS: LibraryName, StructName: "Value"}
}

func requireElement(fn string, n interpreter.Value) error {
	if nodeKind(n) != "element" {
		return fmt.Errorf("%s: node is a %s, not an element", fn, nodeKind(n))
	}
	return nil
}

// ---- path dialect ----

type pathStep struct {
	name     string
	index    int
	hasIndex bool
	wildcard bool
}

// parsePath splits an XPath-style path into steps. A leading '/' is optional
// (the path is always relative to the passed node); "" yields no steps.
func parsePath(fn, path string) ([]pathStep, error) {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil, nil
	}
	parts := strings.Split(path, "/")
	steps := make([]pathStep, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("%s: empty step in path", fn)
		}
		if p == "*" {
			steps = append(steps, pathStep{wildcard: true})
			continue
		}
		st := pathStep{name: p}
		if i := strings.IndexByte(p, '['); i >= 0 {
			if !strings.HasSuffix(p, "]") || i == 0 {
				return nil, fmt.Errorf("%s: malformed step %q", fn, p)
			}
			k, err := strconv.Atoi(p[i+1 : len(p)-1])
			if err != nil || k < 1 {
				return nil, fmt.Errorf("%s: index in %q must be a positive (1-based) integer", fn, p)
			}
			st.name, st.index, st.hasIndex = p[:i], k, true
		}
		steps = append(steps, st)
	}
	return steps, nil
}

// resolvePath walks the steps from node, returning every element node that
// matches the full path (in document order).
func resolvePath(node interpreter.Value, steps []pathStep) []interpreter.Value {
	current := []interpreter.Value{node}
	for _, st := range steps {
		var next []interpreter.Value
		for _, cur := range current {
			var matches []interpreter.Value
			for _, kid := range elementChildren(cur) {
				if st.wildcard || nodeName(kid) == st.name {
					matches = append(matches, kid)
				}
			}
			if st.hasIndex {
				if st.index-1 < len(matches) {
					next = append(next, matches[st.index-1])
				}
			} else {
				next = append(next, matches...)
			}
		}
		current = next
	}
	return current
}

// nodeAndPath is the (xml.Value, path) shape shared by the path accessors.
func nodeAndPath(fn string, args []interpreter.Value) ([]pathStep, interpreter.Value, error) {
	if len(args) != 2 {
		return nil, interpreter.Value{}, fmt.Errorf("%s expects 2 arguments (xml.Value, path), got %d", fn, len(args))
	}
	node, err := takeNode(fn, args[0])
	if err != nil {
		return nil, interpreter.Value{}, err
	}
	if args[1].Kind != interpreter.KindString {
		return nil, interpreter.Value{}, fmt.Errorf("%s: path must be string, got %s", fn, args[1].Kind)
	}
	steps, err := parsePath(fn, args[1].Str)
	if err != nil {
		return nil, interpreter.Value{}, err
	}
	return steps, node, nil
}

// ---- read accessors ----

// typeOfFn: xml.typeOf(node) -> "element" | "text".
func typeOfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("xml.typeOf expects 1 argument (xml.Value), got %d", len(args))
	}
	node, err := takeNode("xml.typeOf", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	return sv(nodeKind(node)), nil
}

// tagFn: xml.tag(node) -> the element's tag name.
func tagFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("xml.tag expects 1 argument (xml.Value), got %d", len(args))
	}
	node, err := takeNode("xml.tag", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if err := requireElement("xml.tag", node); err != nil {
		return interpreter.Null(), err
	}
	return sv(nodeName(node)), nil
}

// textFn: xml.text(node) -> the concatenated character data (an element's
// direct text children, or a text node's own string).
func textFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("xml.text expects 1 argument (xml.Value), got %d", len(args))
	}
	node, err := takeNode("xml.text", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if nodeKind(node) == "text" {
		t, _ := mapField(node, "text")
		return sv(t.Str), nil
	}
	var b strings.Builder
	for _, c := range nodeChildren(node) {
		if nodeKind(c) == "text" {
			t, _ := mapField(c, "text")
			b.WriteString(t.Str)
		}
	}
	return sv(b.String()), nil
}

// attrFn: xml.attr(node, name) -> the attribute's value; errors if absent.
func attrFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, name, err := nodeAndName("xml.attr", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if v, ok := mapField(nodeAttrs(node), name); ok {
		return sv(v.Str), nil
	}
	return interpreter.Null(), fmt.Errorf("xml.attr: no attribute %q", name)
}

// hasAttrFn: xml.hasAttr(node, name) -> bool.
func hasAttrFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	node, name, err := nodeAndName("xml.hasAttr", args)
	if err != nil {
		return interpreter.Null(), err
	}
	_, ok := mapField(nodeAttrs(node), name)
	return interpreter.BoolVal(ok), nil
}

func nodeAndName(fn string, args []interpreter.Value) (interpreter.Value, string, error) {
	if len(args) != 2 {
		return interpreter.Value{}, "", fmt.Errorf("%s expects 2 arguments (xml.Value, name), got %d", fn, len(args))
	}
	node, err := takeNode(fn, args[0])
	if err != nil {
		return interpreter.Value{}, "", err
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Value{}, "", fmt.Errorf("%s: name must be string, got %s", fn, args[1].Kind)
	}
	if err := requireElement(fn, node); err != nil {
		return interpreter.Value{}, "", err
	}
	return node, args[1].Str, nil
}

// attrsFn: xml.attrs(node) -> list of string, the attribute names in order.
func attrsFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("xml.attrs expects 1 argument (xml.Value), got %d", len(args))
	}
	node, err := takeNode("xml.attrs", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if err := requireElement("xml.attrs", node); err != nil {
		return interpreter.Null(), err
	}
	attrs := nodeAttrs(node)
	out := make([]interpreter.Value, 0, len(attrs.Map))
	for _, e := range attrs.Map {
		out = append(out, sv(e.Key.Str))
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeString), out), nil
}

// childrenFn: xml.children(node) -> list of xml.Value, the element children.
func childrenFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("xml.children expects 1 argument (xml.Value), got %d", len(args))
	}
	node, err := takeNode("xml.children", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	kids := elementChildren(node)
	out := make([]interpreter.Value, len(kids))
	for i, k := range kids {
		out[i] = wrap(k)
	}
	return interpreter.ListVal(valueType(), out), nil
}

// getFn: xml.get(node, path) -> the first element matching the path; errors if
// none match. With path "" it returns the node itself.
func getFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	steps, node, err := nodeAndPath("xml.get", args)
	if err != nil {
		return interpreter.Null(), err
	}
	matches := resolvePath(node, steps)
	if len(matches) == 0 {
		return interpreter.Null(), fmt.Errorf("xml.get: no element matches path %q", args[1].Str)
	}
	return wrap(matches[0]), nil
}

// findAllFn: xml.findAll(node, path) -> list of xml.Value, every match.
func findAllFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	steps, node, err := nodeAndPath("xml.findAll", args)
	if err != nil {
		return interpreter.Null(), err
	}
	matches := resolvePath(node, steps)
	out := make([]interpreter.Value, len(matches))
	for i, m := range matches {
		out[i] = wrap(m)
	}
	return interpreter.ListVal(valueType(), out), nil
}

// hasFn: xml.has(node, path) -> bool. A malformed path errors; a well-formed
// path with no match is a plain false.
func hasFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	steps, node, err := nodeAndPath("xml.has", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(len(resolvePath(node, steps)) > 0), nil
}
