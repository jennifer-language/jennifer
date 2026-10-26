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
		if len(args) != 1 && len(args) != 2 {
			return interpreter.Null(), fmt.Errorf("xml.decode expects 1 or 2 arguments (string[, ns]), got %d", len(args))
		}
		if args[0].Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("xml.decode: argument must be string, got %s", args[0].Kind)
		}
		// An optional prefix map puts bindings in scope at the root: a fragment
		// cut out of a larger document decodes against its root's declarations.
		var seed *nsScope
		if len(args) == 2 {
			ns, err := prefixMap("xml.decode", args[1])
			if err != nil {
				return interpreter.Null(), err
			}
			if seed, err = seedScope(ns); err != nil {
				return interpreter.Null(), fmt.Errorf("xml.decode: %v", err)
			}
		}
		tree, err := decodeXMLIn(args[0].Str, seed)
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
	in.RegisterNamespaced(LibraryName, "localName", localNameFn)
	in.RegisterNamespaced(LibraryName, "namespace", namespaceFn)
	in.RegisterNamespaced(LibraryName, "lookupPrefix", lookupPrefixFn)
	in.RegisterNamespaced(LibraryName, "attrNS", attrNSFn)
	in.RegisterNamespaced(LibraryName, "hasAttrNS", hasAttrNSFn)

	// Build surface - non-mutating, each returns a fresh xml.Value.
	in.RegisterNamespaced(LibraryName, "element", elementFn)
	in.RegisterNamespaced(LibraryName, "elementNS", elementNSFn)
	in.RegisterNamespaced(LibraryName, "setAttr", setAttrFn)
	in.RegisterNamespaced(LibraryName, "setAttrNS", setAttrNSFn)
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
	if err := encodeNode(&sb, n, pretty, 0, &outScope{}); err != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", verb, err)
	}
	return interpreter.StringVal(sb.String()), nil
}

// outScope is the namespace bindings in force at the point encode has reached:
// the declarations written so far on the open elements, innermost last.
type outScope struct {
	b []nsBinding
}

type nsBinding struct{ prefix, uri string }

func (s *outScope) lookup(prefix string) (string, bool) {
	for i := len(s.b) - 1; i >= 0; i-- {
		if s.b[i].prefix == prefix {
			return s.b[i].uri, true
		}
	}
	return (*nsScope)(nil).lookup(prefix)
}

// declarations returns the namespace declarations e needs beyond those written
// on it or its ancestors, and pushes them (with e's own) onto s. A decoded
// document needs none - its own xmlns attributes are kept - but a sub-tree
// encoded on its own, or an element built with xml.elementNS, does. A
// namespace-unaware element (xml.element) needs none: its name is verbatim.
func (s *outScope) declarations(e *element) ([]nsBinding, error) {
	for _, a := range e.attrs {
		if p, decl := isNSDecl(a.name); decl {
			s.b = append(s.b, nsBinding{p, a.value})
		}
	}
	var extra []nsBinding
	need := func(prefix, uri string) error {
		if got, _ := s.lookup(prefix); got == uri {
			return nil
		}
		for _, a := range e.attrs {
			if p, decl := isNSDecl(a.name); decl && p == prefix {
				return fmt.Errorf("prefix %q on <%s> is declared as %s but names %s", prefix, e.name, a.value, uri)
			}
		}
		for _, x := range extra {
			if x.prefix == prefix {
				return fmt.Errorf("prefix %q on <%s> names both %s and %s", prefix, e.name, x.uri, uri)
			}
		}
		extra = append(extra, nsBinding{prefix, uri})
		s.b = append(s.b, nsBinding{prefix, uri})
		return nil
	}
	if e.aware {
		if p, _ := splitQName(e.name); p != "xml" {
			if err := need(p, e.ns); err != nil {
				return nil, err
			}
		}
	}
	for _, a := range e.attrs {
		p, _ := splitQName(a.name)
		if p == "" || p == "xml" || a.ns == "" || a.ns == xmlnsNamespace {
			continue
		}
		if err := need(p, a.ns); err != nil {
			return nil, err
		}
	}
	return extra, nil
}

// encodeNode writes n's XML image. Pretty-printing indents element-only
// content; an element containing any text child is emitted inline so its
// character data stays byte-exact. sc carries the namespace declarations in
// force, so an element gets an xmlns declaration only where it needs one.
func encodeNode(sb *strings.Builder, n node, pretty bool, depth int, sc *outScope) error {
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
	mark := len(sc.b)
	defer func() { sc.b = sc.b[:mark] }()
	extra, err := sc.declarations(e)
	if err != nil {
		return err
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
	for _, x := range extra {
		sb.WriteString(" xmlns")
		if x.prefix != "" {
			sb.WriteByte(':')
			sb.WriteString(x.prefix)
		}
		sb.WriteString(`="`)
		escapeAttr(sb, x.uri)
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
		if err := encodeNode(sb, c, pretty, depth+1, sc); err != nil {
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
	return wrap(n.el.with(attrs, n.el.children)), nil
}

// checkNSName validates a qualified name for elementNS / setAttrNS against the
// namespace it is to carry: a valid qualified name, no reserved xmlns prefix,
// xml only with its own namespace, and a prefix only with a non-empty URI.
func checkNSName(fn, what, uri, qname string) error {
	if !validName(qname) || !validQName(qname) {
		return fmt.Errorf("%s: %q is not a valid qualified %s name", fn, qname, what)
	}
	p, _ := splitQName(qname)
	switch {
	case p == "xmlns" || qname == "xmlns":
		return fmt.Errorf("%s: %q is reserved for namespace declarations", fn, qname)
	case p == "xml" && uri != xmlNamespace:
		return fmt.Errorf("%s: the xml prefix can only name %s", fn, xmlNamespace)
	case p != "xml" && (uri == xmlNamespace || uri == xmlnsNamespace):
		return fmt.Errorf("%s: %s is reserved", fn, uri)
	case p != "" && uri == "":
		return fmt.Errorf("%s: prefixed name %q needs a namespace URI", fn, qname)
	}
	return nil
}

// elementNSFn: xml.elementNS(uri, qname) -> a fresh empty element in namespace
// uri ("" for none). encode declares the prefix (or default namespace) it
// needs wherever it is not already in scope.
func elementNSFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("xml.elementNS expects 2 arguments (uri, qname), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString || args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("xml.elementNS: uri and qname must be strings")
	}
	uri, qname := args[0].Str, args[1].Str
	if err := checkNSName("xml.elementNS", "element", uri, qname); err != nil {
		return interpreter.Null(), err
	}
	p, _ := splitQName(qname)
	var scope *nsScope
	if p != "xml" {
		scope = &nsScope{prefix: p, uri: uri}
	}
	return wrap(node{el: newElement(&element{name: qname, ns: uri, scope: scope, aware: true})}), nil
}

// setAttrNSFn: xml.setAttrNS(node, uri, qname, value) -> the element with the
// attribute named (uri, local part of qname) added or updated. A namespaced
// attribute needs a prefix (an unprefixed attribute is in no namespace).
func setAttrNSFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	const fn = "xml.setAttrNS"
	if len(args) != 4 {
		return interpreter.Null(), fmt.Errorf("%s expects 4 arguments (xml.Value, uri, qname, value), got %d", fn, len(args))
	}
	n, err := takeNode(fn, args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if err := requireElement(fn, n); err != nil {
		return interpreter.Null(), err
	}
	for _, a := range args[1:] {
		if a.Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("%s: uri, qname and value must be strings", fn)
		}
	}
	uri, qname, value := args[1].Str, args[2].Str, args[3].Str
	if err := checkNSName(fn, "attribute", uri, qname); err != nil {
		return interpreter.Null(), err
	}
	p, local := splitQName(qname)
	if p == "" && uri != "" {
		return interpreter.Null(), fmt.Errorf("%s: an attribute in namespace %s needs a prefix", fn, uri)
	}
	attrs := make([]attr, 0, len(n.el.attrs)+1)
	replaced := false
	for _, a := range n.el.attrs {
		ap, al := splitQName(a.name)
		sameName := al == local && a.ns != xmlnsNamespace && ((uri == "" && ap == "") || (uri != "" && ap != "" && a.ns == uri))
		if sameName && !replaced {
			attrs = append(attrs, attr{name: qname, value: value, ns: uri})
			replaced = true
			continue
		}
		if a.name == qname {
			return interpreter.Null(), fmt.Errorf("%s: <%s> already has an attribute %q in another namespace", fn, n.el.name, qname)
		}
		attrs = append(attrs, a)
	}
	if !replaced {
		attrs = append(attrs, attr{name: qname, value: value, ns: uri})
	}
	return wrap(n.el.with(attrs, n.el.children)), nil
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
	return wrap(n.el.with(n.el.attrs, []node{textNode(args[1].Str)})), nil
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
	return wrap(parent.el.with(parent.el.attrs, next)), nil
}
