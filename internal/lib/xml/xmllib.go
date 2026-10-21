// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package xmllib is the `xml` library: hand-rolled XML encode / decode over an
// opaque `xml.Value`, designed like `json` and `toml`. `decode` parses a
// document into a KindObject holding the compact node tree (xmlnode.go); the
// accessors walk it by an XPath-style path; the small build surface
// (`element` / `setAttr` / `setText` / `append`) constructs a tree to encode.
// No `encoding/xml` (reflect-bound), so both binaries build it.
package xmllib

import (
	"fmt"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// Install registers the xml surface.
func Install(in *interpreter.Interpreter) {
	// An xml.Value displays as its (compact) XML (xmlValue.DisplayNative), so
	// `$v` at the REPL and `%v` show the document rather than an opaque
	// `<xml.Value>`. An uninitialised `def v as xml.Value;` holds an empty
	// text node.
	in.RegisterNamespacedObject(LibraryName, "Value", nil)
	in.SetNativeObjectZero(LibraryName, "Value", &xmlValue{root: textNode("")})

	in.RegisterNamespaced(LibraryName, "decode", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		if len(args) != 1 {
			return interpreter.Null(), fmt.Errorf("xml.decode expects 1 argument (string), got %d", len(args))
		}
		if args[0].Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("xml.decode: argument must be string, got %s", args[0].Kind)
		}
		tree, err := decodeXML(args[0].Str)
		if err != nil {
			return interpreter.Null(), fmt.Errorf("xml.decode: %v", err)
		}
		return wrap(tree), nil
	})
	in.RegisterNamespaced(LibraryName, "encode", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return encodeFn(args, false)
	})
	in.RegisterNamespaced(LibraryName, "encodePretty", func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		return encodeFn(args, true)
	})

	// Read accessors (xmlaccess.go).
	in.RegisterNamespaced(LibraryName, "typeOf", typeOfFn)
	in.RegisterNamespaced(LibraryName, "tag", tagFn)
	in.RegisterNamespaced(LibraryName, "text", textFn)
	in.RegisterNamespaced(LibraryName, "attr", attrFn)
	in.RegisterNamespaced(LibraryName, "hasAttr", hasAttrFn)
	in.RegisterNamespaced(LibraryName, "attrs", attrsFn)
	in.RegisterNamespaced(LibraryName, "children", childrenFn)
	in.RegisterNamespaced(LibraryName, "get", getFn)
	in.RegisterNamespaced(LibraryName, "findAll", findAllFn)
	in.RegisterNamespaced(LibraryName, "has", hasFn)

	// Build surface - non-mutating, each returns a fresh xml.Value.
	in.RegisterNamespaced(LibraryName, "element", elementFn)
	in.RegisterNamespaced(LibraryName, "setAttr", setAttrFn)
	in.RegisterNamespaced(LibraryName, "setText", setTextFn)
	in.RegisterNamespaced(LibraryName, "append", appendFn)
}

// ---- encode ----

func encodeFn(args []interpreter.Value, pretty bool) (interpreter.Value, error) {
	verb := "xml.encode"
	if pretty {
		verb = "xml.encodePretty"
	}
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("%s expects 1 argument (xml.Value), got %d", verb, len(args))
	}
	n, err := takeNode(verb, args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	var sb strings.Builder
	if err := encodeNode(&sb, n, pretty, 0); err != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", verb, err)
	}
	return interpreter.StringVal(sb.String()), nil
}

// encodeNode writes n's XML image. Pretty-printing indents element-only
// content; an element containing any text child is emitted inline so its
// character data stays byte-exact.
func encodeNode(sb *strings.Builder, n node, pretty bool, depth int) error {
	// Guard the recursion: a tree built deep via xml.append in a loop (or a
	// value printed with %v) would otherwise overflow the Go stack, an
	// uncatchable crash. Matches the decode-side maxDepth; deeper than that
	// cannot come from decode anyway.
	if depth >= maxDepth {
		return fmt.Errorf("element nesting exceeds %d levels", maxDepth)
	}
	e := n.el
	if e == nil {
		escapeText(sb, n.text)
		return nil
	}
	sb.WriteByte('<')
	sb.WriteString(e.name)
	for _, a := range e.attrs {
		sb.WriteByte(' ')
		sb.WriteString(a.name)
		sb.WriteString(`="`)
		escapeAttr(sb, a.value)
		sb.WriteByte('"')
	}
	if len(e.children) == 0 {
		sb.WriteString("/>")
		return nil
	}
	sb.WriteByte('>')
	block := pretty && !hasTextChild(e.children)
	for _, c := range e.children {
		if block {
			newlineIndent(sb, depth+1)
		}
		if err := encodeNode(sb, c, pretty, depth+1); err != nil {
			return err
		}
	}
	if block {
		newlineIndent(sb, depth)
	}
	sb.WriteString("</")
	sb.WriteString(e.name)
	sb.WriteByte('>')
	return nil
}

func hasTextChild(kids []node) bool {
	for _, c := range kids {
		if c.el == nil {
			return true
		}
	}
	return false
}

func newlineIndent(sb *strings.Builder, depth int) {
	sb.WriteByte('\n')
	for i := 0; i < depth*2; i++ {
		sb.WriteByte(' ')
	}
}

// escapeText escapes character data: `&`, `<`, and `>` (the last defensively,
// so `]]>` can never appear).
func escapeText(sb *strings.Builder, s string) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			sb.WriteString("&amp;")
		case '<':
			sb.WriteString("&lt;")
		case '>':
			sb.WriteString("&gt;")
		default:
			sb.WriteByte(s[i])
		}
	}
}

// escapeAttr escapes a (double-quoted) attribute value. Whitespace other than
// the plain space is emitted as a character reference: a compliant parser
// normalizes a *literal* tab / newline / CR in an attribute value to a space,
// so escaping them keeps the exact bytes across a round-trip (a reference is
// not normalized).
func escapeAttr(sb *strings.Builder, s string) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			sb.WriteString("&amp;")
		case '<':
			sb.WriteString("&lt;")
		case '"':
			sb.WriteString("&quot;")
		case '\t':
			sb.WriteString("&#9;")
		case '\n':
			sb.WriteString("&#10;")
		case '\r':
			sb.WriteString("&#13;")
		default:
			sb.WriteByte(s[i])
		}
	}
}

// ---- build surface ----

// validName reports whether s is usable as an element or attribute name: no
// whitespace, empty, or characters the syntax reserves.
func validName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isSpace(c) || c == '<' || c == '>' || c == '/' || c == '=' || c == '"' || c == '\'' || c == '&' {
			return false
		}
	}
	return true
}

// elementFn: xml.element(name) -> a fresh empty element.
func elementFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("xml.element expects 1 argument (name), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("xml.element: name must be string, got %s", args[0].Kind)
	}
	if !validName(args[0].Str) {
		return interpreter.Null(), fmt.Errorf("xml.element: %q is not a valid element name", args[0].Str)
	}
	return wrap(elementNode(args[0].Str, nil, nil)), nil
}

// setAttrFn: xml.setAttr(node, name, value) -> the element with the attribute
// added or updated.
func setAttrFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("xml.setAttr expects 3 arguments (xml.Value, name, value), got %d", len(args))
	}
	n, err := takeNode("xml.setAttr", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if err := requireElement("xml.setAttr", n); err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindString || args[2].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("xml.setAttr: name and value must be strings")
	}
	if !validName(args[1].Str) {
		return interpreter.Null(), fmt.Errorf("xml.setAttr: %q is not a valid attribute name", args[1].Str)
	}
	// Copy the attribute list so the shared source element is never mutated.
	attrs := make([]attr, len(n.el.attrs), len(n.el.attrs)+1)
	copy(attrs, n.el.attrs)
	replaced := false
	for i := range attrs {
		if attrs[i].name == args[1].Str {
			attrs[i].value = args[2].Str
			replaced = true
			break
		}
	}
	if !replaced {
		attrs = append(attrs, attr{name: args[1].Str, value: args[2].Str})
	}
	return wrap(elementNode(n.el.name, attrs, n.el.children)), nil
}

// setTextFn: xml.setText(node, s) -> the element with its children replaced by
// a single text node.
func setTextFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("xml.setText expects 2 arguments (xml.Value, text), got %d", len(args))
	}
	n, err := takeNode("xml.setText", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if err := requireElement("xml.setText", n); err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("xml.setText: text must be string, got %s", args[1].Kind)
	}
	return wrap(elementNode(n.el.name, n.el.attrs, []node{textNode(args[1].Str)})), nil
}

// appendFn: xml.append(parent, child) -> the parent element with child (an
// xml.Value element) appended to its children.
func appendFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("xml.append expects 2 arguments (parent, child), got %d", len(args))
	}
	parent, err := takeNode("xml.append", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if err := requireElement("xml.append", parent); err != nil {
		return interpreter.Null(), err
	}
	child, err := takeNode("xml.append", args[1])
	if err != nil {
		return interpreter.Null(), err
	}
	// Bound the depth of a tree built by appending (decode bounds its own):
	// equality and encoding recurse once per level.
	if child.height()+1 > maxDepth {
		return interpreter.Null(), fmt.Errorf("xml.append: the result would nest deeper than %d levels", maxDepth)
	}
	kids := parent.el.children
	next := make([]node, 0, len(kids)+1)
	next = append(next, kids...)
	next = append(next, child)
	return wrap(elementNode(parent.el.name, parent.el.attrs, next)), nil
}
