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
type element struct {
	name     string
	attrs    []attr
	children []node
	height   uint32
}

// attr is one name="value" pair.
type attr struct {
	name, value string
}

func textNode(s string) node { return node{text: s} }

func elementNode(name string, attrs []attr, children []node) node {
	var h uint32
	for _, c := range children {
		if c.el != nil && c.el.height > h {
			h = c.el.height
		}
	}
	return node{el: &element{name: name, attrs: attrs, children: children, height: h + 1}}
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
	for _, a := range e.attrs {
		if a.name == name {
			return a.value, true
		}
	}
	return "", false
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
	if len(a.attrs) > manyAttrs {
		bv := make(map[string]string, len(b.attrs))
		for _, at := range b.attrs {
			bv[at.name] = at.value
		}
		for _, at := range a.attrs {
			if v, ok := bv[at.name]; !ok || v != at.value {
				return false
			}
		}
	} else {
		for _, at := range a.attrs {
			if v, ok := b.attr(at.name); !ok || v != at.value {
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
	if err := encodeNode(&sb, v.root, false, 0); err != nil {
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
