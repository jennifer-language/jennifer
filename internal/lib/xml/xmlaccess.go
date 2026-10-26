// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// xml.Value accessors and the internal node model. `xml.decode` returns an
// opaque `xml.Value` (a KindObject holding the compact node tree); the functions
// here are the only way to reach inside it, since operators, `[index]`, and
// `.field` all reject an object - the same shape as `json` / `toml`.
//
// A node is an element or a text run (the compact tree in xmlnode.go). An
// element's children keep text and element nodes in document order (so mixed
// content and whitespace round-trip); `xml.children` exposes only the element
// children and `xml.text` concatenates the text ones.
//
// Navigation uses a small XPath-style path dialect in place of JSON Pointer:
// `/`-separated steps of `name`, `name[k]` (the 1-based k-th same-named element
// child), `*` (any element child), or `*:local` (that local name in any
// namespace), relative to the passed node. Names match as written unless the
// caller passes a prefix map, which makes `p:local` mean (map[p], local) - the
// XPath way, independent of the prefixes a document chose.
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

func sv(s string) interpreter.Value { return interpreter.StringVal(s) }

// own copies a string out of the tree: names and plain text are slices of the
// whole input, which a short value kept by the program must not hold alive.
func own(s string) interpreter.Value { return interpreter.StringVal(strings.Clone(s)) }

// elementChildren returns just the element (non-text) children of n.
func elementChildren(n node) []node {
	if n.el == nil {
		return nil
	}
	var out []node
	for _, c := range n.el.children {
		if c.el != nil {
			out = append(out, c)
		}
	}
	return out
}

// ---- argument plumbing ----

func takeNode(fn string, v interpreter.Value) (node, error) {
	n, ok := asNode(v)
	if !ok {
		return node{}, fmt.Errorf("%s: argument must be an xml.Value, got %s", fn, v.Kind)
	}
	return n, nil
}

// valueType is the parser type stamped on a returned `list of xml.Value` so it
// binds to a declared `list of xml.Value`.
func valueType() parser.Type {
	return parser.Type{Kind: parser.TypeStruct, StructNS: LibraryName, StructName: "Value"}
}

func requireElement(fn string, n node) error {
	if n.el == nil {
		return fmt.Errorf("%s: node is a %s, not an element", fn, n.kindName())
	}
	return nil
}

// ---- path dialect ----

type pathStep struct {
	name     string
	index    int
	hasIndex bool
	wildcard bool
	anyNS    bool   // `*:local`: match the local name in any namespace
	byNS     bool   // a prefix map was given: match (uri, local)
	uri      string // the step's namespace under the map
	local    string // the step's local name (anyNS / byNS)
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
		if st.name == "" {
			return nil, fmt.Errorf("%s: malformed step %q", fn, p)
		}
		if strings.HasPrefix(st.name, "*:") {
			st.anyNS, st.local = true, st.name[2:]
			if st.local == "" || strings.IndexByte(st.local, ':') >= 0 {
				return nil, fmt.Errorf("%s: malformed step %q", fn, p)
			}
		}
		steps = append(steps, st)
	}
	return steps, nil
}

// bindPathPrefixes resolves each named step's prefix through the caller's
// prefix map, so the step matches by (namespace, local name). An unprefixed
// step takes the map's "" entry (no namespace when absent); the xml prefix is
// bound implicitly; any other prefix missing from the map is an error.
func bindPathPrefixes(fn string, steps []pathStep, ns map[string]string) error {
	for i := range steps {
		st := &steps[i]
		if st.wildcard || st.anyNS {
			continue
		}
		if !validQName(st.name) {
			return fmt.Errorf("%s: step %q is not a valid qualified name", fn, st.name)
		}
		p, local := splitQName(st.name)
		uri, ok := ns[p]
		if !ok {
			switch p {
			case "":
				uri, ok = "", true
			case "xml":
				uri, ok = xmlNamespace, true
			}
		}
		if !ok {
			return fmt.Errorf("%s: prefix %q in step %q is not in the namespace map", fn, p, st.name)
		}
		st.byNS, st.uri, st.local = true, uri, local
	}
	return nil
}

// matches reports whether element kid satisfies the step's name test.
func (st *pathStep) matches(kid *element) bool {
	switch {
	case st.wildcard:
		return true
	case st.anyNS:
		return kid.localName() == st.local
	case st.byNS:
		if !kid.aware {
			// A namespace-unaware element is in no namespace, under its full name.
			return st.uri == "" && kid.name == st.local
		}
		return kid.ns == st.uri && kid.localName() == st.local
	}
	return kid.name == st.name
}

// resolvePath walks the steps from node, returning every element node that
// matches the full path (in document order).
func resolvePath(start node, steps []pathStep) []node {
	current := []node{start}
	for _, st := range steps {
		var next []node
		for _, cur := range current {
			var matches []node
			for _, kid := range elementChildren(cur) {
				if st.matches(kid.el) {
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

// nodeAndPath is the (xml.Value, path[, ns]) shape shared by the path
// accessors; ns is a `map of string to string` from prefix to namespace URI.
func nodeAndPath(fn string, args []interpreter.Value) ([]pathStep, node, error) {
	if len(args) != 2 && len(args) != 3 {
		return nil, node{}, fmt.Errorf("%s expects 2 or 3 arguments (xml.Value, path[, ns]), got %d", fn, len(args))
	}
	n, err := takeNode(fn, args[0])
	if err != nil {
		return nil, node{}, err
	}
	if args[1].Kind != interpreter.KindString {
		return nil, node{}, fmt.Errorf("%s: path must be string, got %s", fn, args[1].Kind)
	}
	steps, err := parsePath(fn, args[1].Str)
	if err != nil {
		return nil, node{}, err
	}
	if len(args) == 3 {
		ns, err := prefixMap(fn, args[2])
		if err != nil {
			return nil, node{}, err
		}
		if err := bindPathPrefixes(fn, steps, ns); err != nil {
			return nil, node{}, err
		}
	}
	return steps, n, nil
}

// prefixMap reads a `map of string to string` (prefix -> namespace URI).
func prefixMap(fn string, v interpreter.Value) (map[string]string, error) {
	if v.Kind != interpreter.KindMap {
		return nil, fmt.Errorf("%s: namespace map must be a map of string to string, got %s", fn, v.Kind)
	}
	ns := make(map[string]string, len(v.Map))
	for _, e := range v.Map {
		if e.Key.Kind != interpreter.KindString || e.Value.Kind != interpreter.KindString {
			return nil, fmt.Errorf("%s: namespace map must be a map of string to string", fn)
		}
		ns[e.Key.Str] = e.Value.Str
	}
	return ns, nil
}

// ---- read accessors ----

// typeOfFn: xml.typeOf(node) -> "element" | "text".
func typeOfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("xml.typeOf expects 1 argument (xml.Value), got %d", len(args))
	}
	n, err := takeNode("xml.typeOf", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	return sv(n.kindName()), nil
}

// tagFn: xml.tag(node) -> the element's tag name.
func tagFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("xml.tag expects 1 argument (xml.Value), got %d", len(args))
	}
	n, err := takeNode("xml.tag", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if err := requireElement("xml.tag", n); err != nil {
		return interpreter.Null(), err
	}
	return own(n.el.name), nil
}

// textFn: xml.text(node) -> the concatenated character data (an element's
// direct text children, or a text node's own string).
func textFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("xml.text expects 1 argument (xml.Value), got %d", len(args))
	}
	n, err := takeNode("xml.text", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if n.el == nil {
		return own(n.text), nil
	}
	var b strings.Builder
	for _, c := range n.el.children {
		if c.el == nil {
			b.WriteString(c.text)
		}
	}
	return sv(b.String()), nil
}

// attrFn: xml.attr(node, name) -> the attribute's value; errors if absent.
func attrFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	n, name, err := nodeAndName("xml.attr", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if v, ok := n.el.attr(name); ok {
		return own(v), nil
	}
	return interpreter.Null(), fmt.Errorf("xml.attr: no attribute %q", name)
}

// hasAttrFn: xml.hasAttr(node, name) -> bool.
func hasAttrFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	n, name, err := nodeAndName("xml.hasAttr", args)
	if err != nil {
		return interpreter.Null(), err
	}
	_, ok := n.el.attr(name)
	return interpreter.BoolVal(ok), nil
}

func nodeAndName(fn string, args []interpreter.Value) (node, string, error) {
	if len(args) != 2 {
		return node{}, "", fmt.Errorf("%s expects 2 arguments (xml.Value, name), got %d", fn, len(args))
	}
	n, err := takeNode(fn, args[0])
	if err != nil {
		return node{}, "", err
	}
	if args[1].Kind != interpreter.KindString {
		return node{}, "", fmt.Errorf("%s: name must be string, got %s", fn, args[1].Kind)
	}
	if err := requireElement(fn, n); err != nil {
		return node{}, "", err
	}
	return n, args[1].Str, nil
}

// attrsFn: xml.attrs(node) -> list of string, the attribute names in order.
func attrsFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("xml.attrs expects 1 argument (xml.Value), got %d", len(args))
	}
	n, err := takeNode("xml.attrs", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if err := requireElement("xml.attrs", n); err != nil {
		return interpreter.Null(), err
	}
	out := make([]interpreter.Value, len(n.el.attrs))
	for i, a := range n.el.attrs {
		out[i] = own(a.name)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeString), out), nil
}

// childrenFn: xml.children(node) -> list of xml.Value, the element children.
func childrenFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("xml.children expects 1 argument (xml.Value), got %d", len(args))
	}
	n, err := takeNode("xml.children", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	kids := elementChildren(n)
	out := make([]interpreter.Value, len(kids))
	for i, k := range kids {
		out[i] = wrap(k)
	}
	return interpreter.ListVal(valueType(), out), nil
}

// getFn: xml.get(node, path) -> the first element matching the path; errors if
// none match. With path "" it returns the node itself.
func getFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	steps, n, err := nodeAndPath("xml.get", args)
	if err != nil {
		return interpreter.Null(), err
	}
	matches := resolvePath(n, steps)
	if len(matches) == 0 {
		return interpreter.Null(), fmt.Errorf("xml.get: no element matches path %q", args[1].Str)
	}
	return wrap(matches[0]), nil
}

// findAllFn: xml.findAll(node, path) -> list of xml.Value, every match.
func findAllFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	steps, n, err := nodeAndPath("xml.findAll", args)
	if err != nil {
		return interpreter.Null(), err
	}
	matches := resolvePath(n, steps)
	out := make([]interpreter.Value, len(matches))
	for i, m := range matches {
		out[i] = wrap(m)
	}
	return interpreter.ListVal(valueType(), out), nil
}

// hasFn: xml.has(node, path) -> bool. A malformed path errors; a well-formed
// path with no match is a plain false.
func hasFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	steps, n, err := nodeAndPath("xml.has", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(len(resolvePath(n, steps)) > 0), nil
}

// ---- namespaces ----

// localNameFn: xml.localName(node) -> the element's name without its prefix.
func localNameFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	n, err := oneElement("xml.localName", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return own(n.el.localName()), nil
}

// namespaceFn: xml.namespace(node) -> the element's namespace URI ("" for none,
// and for an element built with xml.element).
func namespaceFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	n, err := oneElement("xml.namespace", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return own(n.el.ns), nil
}

func oneElement(fn string, args []interpreter.Value) (node, error) {
	if len(args) != 1 {
		return node{}, fmt.Errorf("%s expects 1 argument (xml.Value), got %d", fn, len(args))
	}
	n, err := takeNode(fn, args[0])
	if err != nil {
		return node{}, err
	}
	return n, requireElement(fn, n)
}

// lookupPrefixFn: xml.lookupPrefix(node, prefix) -> the namespace URI bound to
// prefix at the element ("" for an undeclared default namespace); errors when
// a non-empty prefix is not bound there.
func lookupPrefixFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	n, prefix, err := nodeAndName("xml.lookupPrefix", args)
	if err != nil {
		return interpreter.Null(), err
	}
	uri, ok := n.el.scope.lookup(prefix)
	if !ok {
		return interpreter.Null(), fmt.Errorf("xml.lookupPrefix: prefix %q is not bound at <%s>", prefix, n.el.name)
	}
	return own(uri), nil
}

// findNS returns the attribute named (uri, local): a prefixed attribute in
// namespace uri, or for uri "" an unprefixed one.
func (e *element) findNS(uri, local string) (attr, bool) {
	for _, a := range e.attrs {
		p, l := splitQName(a.name)
		if l != local || a.ns == xmlnsNamespace {
			continue
		}
		if uri == "" && p == "" {
			return a, true
		}
		if uri != "" && p != "" && a.ns == uri {
			return a, true
		}
	}
	return attr{}, false
}

func nodeURILocal(fn string, args []interpreter.Value) (node, string, string, error) {
	if len(args) != 3 {
		return node{}, "", "", fmt.Errorf("%s expects 3 arguments (xml.Value, uri, local), got %d", fn, len(args))
	}
	n, err := takeNode(fn, args[0])
	if err != nil {
		return node{}, "", "", err
	}
	if args[1].Kind != interpreter.KindString || args[2].Kind != interpreter.KindString {
		return node{}, "", "", fmt.Errorf("%s: uri and local name must be strings", fn)
	}
	if err := requireElement(fn, n); err != nil {
		return node{}, "", "", err
	}
	return n, args[1].Str, args[2].Str, nil
}

// attrNSFn: xml.attrNS(node, uri, local) -> the value of the attribute named
// (uri, local); errors if absent.
func attrNSFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	n, uri, local, err := nodeURILocal("xml.attrNS", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if a, ok := n.el.findNS(uri, local); ok {
		return own(a.value), nil
	}
	return interpreter.Null(), fmt.Errorf("xml.attrNS: no attribute {%s}%s", uri, local)
}

// hasAttrNSFn: xml.hasAttrNS(node, uri, local) -> bool.
func hasAttrNSFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	n, uri, local, err := nodeURILocal("xml.hasAttrNS", args)
	if err != nil {
		return interpreter.Null(), err
	}
	_, ok := n.el.findNS(uri, local)
	return interpreter.BoolVal(ok), nil
}
