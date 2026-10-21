// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package doctree is the compact document tree behind the `json`, `toml` and
// `yaml` libraries' opaque `Value`. A decoded document is held as a tree of
// Nodes - a 40-byte struct per value - rather than as interpreter Values (272
// bytes each, so 544 bytes per map member before its strings), so a document's
// in-memory size is a small multiple of its text instead of tens of times it.
//
// The tree is immutable once built: accessors read it, the write surface copies
// only the spine it edits, so every handle, its copies, and any sub-tree a `get`
// hands out share structure safely, across goroutines too. (A decoder that has
// to grow containers out of order uses a Builder on containers it alone holds,
// then seals the result.)
//
// The package also carries everything the three libraries share above the
// tree: JSON Pointer addressing, the non-mutating write verbs, the read
// accessors, and their registration (Lib). Each library keeps only its own
// decoder and encoder.
package doctree

import (
	"math"
	"sort"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// Kind is a node's type.
type Kind uint8

const (
	KindNull Kind = iota
	KindBool
	KindInt
	KindFloat
	KindString
	// KindBytes is binary data (a YAML `!!binary` scalar); the bytes are held
	// in the node's string.
	KindBytes
	// KindDatetime is a date-time kept as its verbatim text plus a
	// library-defined form tag (TOML's four forms; YAML's one timestamp).
	KindDatetime
	KindList
	KindMap
)

// Node is one document value. Scalars keep their payload inline: num holds a
// bool (0 / 1), an int64, a float64's bits, or a datetime's form tag; str holds
// a string, the bytes of a bytes value, or a datetime's text. A list or map
// points at its container, so copying a Node (into a sub-tree handle, or a
// rebuilt spine) never copies its children.
type Node struct {
	k   Kind
	num uint64
	str string
	c   *container
}

// container holds a list's elements, or a map's members in document order:
// keys[i] names elems[i]. order is set for a map with more than
// indexThreshold members: the member indices sorted by key, so a key lookup is
// a binary search (4 bytes per member) rather than a linear scan. height is
// the container's nesting depth (1 + its deepest child container's), kept so a
// depth check is O(1) even on a tree whose subtrees are shared many times.
// Containers are never mutated once their node is handed out (see Builder).
type container struct {
	elems  []Node
	keys   []string
	order  []uint32
	height uint32
}

// newContainer builds a container over its slices, computing its height.
func newContainer(elems []Node, keys []string, order []uint32) *container {
	return &container{elems: elems, keys: keys, order: order, height: heightOf(elems)}
}

// heightOf is the height of a container holding elems.
func heightOf(elems []Node) uint32 {
	var h uint32
	for _, e := range elems {
		if e.c != nil && e.c.height > h {
			h = e.c.height
		}
	}
	return h + 1
}

// indexThreshold is the member count above which a map carries a sorted
// lookup index; below it a linear scan is as fast and costs nothing.
const indexThreshold = 8

// Null returns the null node.
func Null() Node { return Node{k: KindNull} }

// Bool returns a bool node.
func Bool(b bool) Node {
	if b {
		return Node{k: KindBool, num: 1}
	}
	return Node{k: KindBool}
}

// Int returns an int node.
func Int(n int64) Node { return Node{k: KindInt, num: uint64(n)} }

// Float returns a float node.
func Float(f float64) Node { return Node{k: KindFloat, num: math.Float64bits(f)} }

// String returns a string node.
func String(s string) Node { return Node{k: KindString, str: s} }

// Bytes returns a bytes node holding a copy of b.
func Bytes(b []byte) Node { return Node{k: KindBytes, str: string(b)} }

// Datetime returns a datetime node: the verbatim text plus the owning
// library's form tag.
func Datetime(text string, form uint8) Node {
	return Node{k: KindDatetime, num: uint64(form), str: text}
}

// List builds a list over elems (taken by reference: the caller must not
// modify the slice afterwards).
func List(elems []Node) Node {
	return Node{k: KindList, c: newContainer(elems, nil, nil)}
}

// Map builds a map over parallel keys / elems (taken by reference), indexing
// it when it is large. The keys must be distinct.
func Map(keys []string, elems []Node) Node {
	return Node{k: KindMap, c: newContainer(elems, keys, buildOrder(keys))}
}

// buildOrder returns the sorted-by-key index for a large key set, or nil.
func buildOrder(keys []string) []uint32 {
	if len(keys) <= indexThreshold {
		return nil
	}
	order := make([]uint32, len(keys))
	for i := range order {
		order[i] = uint32(i)
	}
	sort.Slice(order, func(a, b int) bool { return keys[order[a]] < keys[order[b]] })
	return order
}

// Kind returns the node's type.
func (n Node) Kind() Kind { return n.k }

// BoolVal is a bool node's value.
func (n Node) BoolVal() bool { return n.num != 0 }

// IntVal is an int node's value.
func (n Node) IntVal() int64 { return int64(n.num) }

// FloatVal is a float node's value.
func (n Node) FloatVal() float64 { return math.Float64frombits(n.num) }

// Str is a string node's value, a bytes node's bytes, or a datetime's text.
func (n Node) Str() string { return n.str }

// Form is a datetime node's form tag.
func (n Node) Form() uint8 { return uint8(n.num) }

// IsContainer reports whether n is a list or a map.
func (n Node) IsContainer() bool { return n.k == KindList || n.k == KindMap }

// Len is a container's element / member count (0 for a scalar).
func (n Node) Len() int {
	if n.c == nil {
		return 0
	}
	return len(n.c.elems)
}

// Elem is a container's i-th element (a map's i-th member value, in document
// order).
func (n Node) Elem(i int) Node { return n.c.elems[i] }

// Key is a map's i-th key, in document order.
func (n Node) Key(i int) string { return n.c.keys[i] }

// Elems is a container's element slice. It is shared with the tree: callers
// must not modify it.
func (n Node) Elems() []Node {
	if n.c == nil {
		return nil
	}
	return n.c.elems
}

// Keys is a map's key slice, in document order. It is shared with the tree:
// callers must not modify it.
func (n Node) Keys() []string {
	if n.c == nil {
		return nil
	}
	return n.c.keys
}

// Find returns the position of key in a map node, or -1.
func (n Node) Find(key string) int {
	c := n.c
	if c == nil {
		return -1
	}
	if c.order == nil {
		for i, k := range c.keys {
			if k == key {
				return i
			}
		}
		return -1
	}
	j := sort.Search(len(c.order), func(i int) bool { return c.keys[c.order[i]] >= key })
	if j < len(c.order) && c.keys[c.order[j]] == key {
		return int(c.order[j])
	}
	return -1
}

// TypeName is the node's type in Jennifer's vocabulary (list / map, as
// convert.typeOf reports them, never "array" / "object" / "table").
func (n Node) TypeName() string {
	switch n.k {
	case KindNull:
		return "null"
	case KindBool:
		return "bool"
	case KindInt:
		return "int"
	case KindFloat:
		return "float"
	case KindString:
		return "string"
	case KindBytes:
		return "bytes"
	case KindDatetime:
		return "datetime"
	case KindList:
		return "list"
	case KindMap:
		return "map"
	}
	return "unknown"
}

// Equal reports whether two nodes hold the same value, with the language's
// `==` rules: numbers compare exactly across int / float (1 == 1.0), and map
// member order does not matter.
func (n Node) Equal(o Node) bool {
	if (n.k == KindInt || n.k == KindFloat) && (o.k == KindInt || o.k == KindFloat) {
		return n.ScalarValue().Equal(o.ScalarValue())
	}
	if n.k != o.k {
		return false
	}
	switch n.k {
	case KindNull:
		return true
	case KindBool:
		return n.num == o.num
	case KindString, KindBytes:
		return n.str == o.str
	case KindDatetime:
		return n.str == o.str && n.num == o.num
	case KindList:
		if n.c == o.c {
			return true
		}
		if n.Len() != o.Len() {
			return false
		}
		for i := range n.c.elems {
			if !n.c.elems[i].Equal(o.c.elems[i]) {
				return false
			}
		}
		return true
	case KindMap:
		if n.c == o.c {
			return true
		}
		if n.Len() != o.Len() {
			return false
		}
		for i, k := range n.c.keys {
			j := o.Find(k)
			if j < 0 || !n.c.elems[i].Equal(o.c.elems[j]) {
				return false
			}
		}
		return true
	}
	return false
}

// ScalarValue renders a scalar node as the interpreter Value an `as*`
// accessor returns. Containers and datetimes have no plain scalar form
// (callers check the kind); they render as null. A string is copied out: a
// decoded string may be a slice of the whole input text, which a short value
// kept by the program must not hold alive.
func (n Node) ScalarValue() interpreter.Value {
	switch n.k {
	case KindBool:
		return interpreter.BoolVal(n.BoolVal())
	case KindInt:
		return interpreter.IntVal(n.IntVal())
	case KindFloat:
		return interpreter.FloatVal(n.FloatVal())
	case KindString:
		return interpreter.StringVal(strings.Clone(n.str))
	case KindBytes:
		return interpreter.BytesVal([]byte(n.str))
	}
	return interpreter.Null()
}

// Height is n's container nesting depth: 0 for a scalar, 1 for a container of
// scalars, and so on.
func (n Node) Height() int {
	if n.c == nil {
		return 0
	}
	return int(n.c.height)
}

// ExceedsDepth reports whether n's container nesting is deeper than budget
// levels. O(1): containers carry their height.
func (n Node) ExceedsDepth(budget int) bool {
	return n.Height() > budget
}
