// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// The compact xml.Value tree. A decoded document is held as Go structs - an
// element is its name, its ordered attributes and its ordered children; a text
// node is its character data - rather than as interpreter Values (an element
// encoded as a 4-entry map of Values cost several KB, so a document took a few
// hundred times its text in memory). Names, and text or attribute values that
// need no entity decoding, are substrings of the input. The tree is immutable
// once built: the build surface (setAttr / setText / append) copies only the
// element it edits, so handles, their copies and any sub-tree `xml.get` hands
// out share structure safely, across goroutines too.
package xmllib

import (
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// node is one entry in an element's content: an element (el set) or a text
// run (el nil, text holds the character data).
type node struct {
	text string
	el   *element
}

// element is an XML element. attrs keep document order; children keep text and
// elements in document order so mixed content and whitespace round-trip.
// height is the element nesting depth (1 + the deepest child element's), kept
// so xml.append can bound the depth of a tree it builds in O(1). Never mutated
// after construction.
//
// ns is the element's namespace URI and scope the namespace bindings in scope
// at it (resolved by decode, so a sub-tree handle with no parent still knows
// them). aware marks an element that takes part in namespaces - every decoded
// element, and one built with xml.elementNS; an element built with
// xml.element is namespace-unaware: its name is used verbatim and encode adds
// no declaration for it.
type element struct {
	name     string
	ns       string
	scope    *nsScope
	attrs    []attr
	children []node
	height   uint32
	aware    bool
}

// attr is one name="value" pair; ns is the attribute's namespace URI ("" for
// an unprefixed attribute, which is in no namespace).
type attr struct {
	name, value, ns string
}

// nsScope is one namespace binding (prefix "" is the default namespace; uri ""
// on it undeclares the default) linked to the bindings outside it. A chain is
// immutable and shared: an element that declares nothing points at its
// parent's chain, so only declarations allocate.
type nsScope struct {
	prefix, uri string
	parent      *nsScope
}

// The two namespaces bound by definition (Namespaces in XML 1.0, section 3).
const (
	xmlNamespace   = "http://www.w3.org/XML/1998/namespace"
	xmlnsNamespace = "http://www.w3.org/2000/xmlns/"
)

// lookup returns the URI bound to prefix in the chain. The `xml` and `xmlns`
// prefixes are bound implicitly; an unbound default namespace is "" (bound);
// an unbound other prefix reports false.
func (s *nsScope) lookup(prefix string) (string, bool) {
	for ; s != nil; s = s.parent {
		if s.prefix == prefix {
			return s.uri, true
		}
	}
	switch prefix {
	case "":
		return "", true
	case "xml":
		return xmlNamespace, true
	case "xmlns":
		return xmlnsNamespace, true
	}
	return "", false
}

// splitQName splits a qualified name into prefix and local part ("" prefix
// when there is no colon).
func splitQName(name string) (prefix, local string) {
	if i := strings.IndexByte(name, ':'); i >= 0 {
		return name[:i], name[i+1:]
	}
	return "", name
}

// validQName reports whether name is a qualified name: at most one colon, and
// not at either end.
func validQName(name string) bool {
	i := strings.IndexByte(name, ':')
	if i < 0 {
		return true
	}
	return i > 0 && i < len(name)-1 && strings.IndexByte(name[i+1:], ':') < 0
}

// isNSDecl reports whether an attribute name declares a namespace (`xmlns` or
// `xmlns:p`), returning the prefix it binds ("" for the default).
func isNSDecl(name string) (string, bool) {
	if name == "xmlns" {
		return "", true
	}
	if strings.HasPrefix(name, "xmlns:") {
		return name[len("xmlns:"):], true
	}
	return "", false
}

func textNode(s string) node { return node{text: s} }

// elementNode builds a namespace-unaware element (xml.element).
func elementNode(name string, attrs []attr, children []node) node {
	return node{el: newElement(&element{name: name, attrs: attrs, children: children})}
}

// newElement sets e's height from its children and returns it.
func newElement(e *element) *element {
	var h uint32
	for _, c := range e.children {
		if c.el != nil && c.el.height > h {
			h = c.el.height
		}
	}
	e.height = h + 1
	return e
}

// with returns a copy of e with new attributes and children, keeping its name
// and namespace state (the build surface edits through it).
func (e *element) with(attrs []attr, children []node) node {
	c := *e
	c.attrs, c.children = attrs, children
	return node{el: newElement(&c)}
}

// localName is the element's name without its prefix.
func (e *element) localName() string {
	_, l := splitQName(e.name)
	return l
}

// height is n's element nesting depth (0 for a text node).
func (n node) height() int {
	if n.el == nil {
		return 0
	}
	return int(n.el.height)
}

// manyAttrs is the attribute count above which a set of names is checked
// through a map rather than by scanning (an element may carry any number).
const manyAttrs = 8

func (n node) isElement() bool { return n.el != nil }

// kindName is the node's type as xml.typeOf reports it.
func (n node) kindName() string {
	if n.el != nil {
		return "element"
	}
	return "text"
}

// attr returns the value of the named attribute.
func (e *element) attr(name string) (string, bool) {
	if a, ok := e.find(name); ok {
		return a.value, true
	}
	return "", false
}

// find returns the attribute with the given qualified name.
func (e *element) find(name string) (attr, bool) {
	for _, a := range e.attrs {
		if a.name == name {
			return a, true
		}
	}
	return attr{}, false
}

// equal reports whether two nodes hold the same content. Attributes compare as
// a set (their order is not significant, as with map equality); children
// compare in order.
func (n node) equal(o node) bool {
	if n.el == nil || o.el == nil {
		return n.el == nil && o.el == nil && n.text == o.text
	}
	a, b := n.el, o.el
	if a == b {
		return true
	}
	if a.name != b.name || len(a.attrs) != len(b.attrs) || len(a.children) != len(b.children) {
		return false
	}
	// Two namespace-aware elements with the same qualified name are different
	// names when their prefixes resolve to different namespaces.
	if a.aware && b.aware && a.ns != b.ns {
		return false
	}
	both := a.aware && b.aware
	same := func(x, y attr) bool { return x.value == y.value && (!both || x.ns == y.ns) }
	if len(a.attrs) > manyAttrs {
		bv := make(map[string]attr, len(b.attrs))
		for _, at := range b.attrs {
			bv[at.name] = at
		}
		for _, at := range a.attrs {
			if o, ok := bv[at.name]; !ok || !same(at, o) {
				return false
			}
		}
	} else {
		for _, at := range a.attrs {
			if o, ok := b.find(at.name); !ok || !same(at, o) {
				return false
			}
		}
	}
	for i := range a.children {
		if !a.children[i].equal(b.children[i]) {
			return false
		}
	}
	return true
}

// xmlValue is the native payload of an xml.Value handle: one node, the root of
// a document or any sub-tree of one.
type xmlValue struct {
	root node
}

// EqualNative implements interpreter.NativeObject.
func (v *xmlValue) EqualNative(other interpreter.NativeObject) bool {
	o, ok := other.(*xmlValue)
	return ok && v.root.equal(o.root)
}

// DisplayNative implements interpreter.NativeObject: the compact XML, so `$v`
// at the REPL and `%v` show the document.
func (v *xmlValue) DisplayNative() string {
	var sb strings.Builder
	if err := encodeNode(&sb, v.root, false, 0, &outScope{}); err != nil {
		return "<xml.Value>"
	}
	return sb.String()
}

// wrap makes an xml.Value handle over n.
func wrap(n node) interpreter.Value {
	return interpreter.NativeObjectVal(LibraryName, "Value", &xmlValue{root: n})
}

// asNode returns the node an xml.Value handle holds. A handle with no native
// payload (an uninitialised value built outside the registered zero) reads as
// an empty text node, the same as the registered zero.
func asNode(v interpreter.Value) (node, bool) {
	if p, ok := v.AsNativeObject(LibraryName, "Value"); ok {
		if xv, ok := p.(*xmlValue); ok {
			return xv.root, true
		}
		return node{}, false
	}
	if _, ok := v.AsObject(LibraryName, "Value"); ok {
		return textNode(""), true
	}
	return node{}, false
}
