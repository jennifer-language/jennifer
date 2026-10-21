// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// JSON Pointer addressing and the tree-level write verbs. Every verb is
// non-mutating: it returns a new root with the edit applied. Only the spine
// from the root to the edited node is copied (each container on it gets fresh
// element / key slices); untouched sibling subtrees are shared by reference,
// which is safe because a sealed tree is never mutated in place.
//
// Writes are strict - no auto-vivification. Set creates only the final pointer
// segment; a missing intermediate, or a set / insert on a scalar, is an error.
// The `-` end-marker (append position) is honoured by Insert only.

package doctree

import (
	"fmt"
	"sort"
	"strings"
	"sync/atomic"

	"jennifer-lang.dev/jennifer/internal/limits"
)

// ParsePointer splits a JSON Pointer (RFC 6901) into its unescaped reference
// tokens. "" yields no tokens (the node itself). A non-empty pointer must
// start with '/'. Escapes: `~1` -> `/`, `~0` -> `~` (in that order).
func ParsePointer(fnName, ptr string) ([]string, error) {
	if ptr == "" {
		return nil, nil
	}
	if ptr[0] != '/' {
		return nil, fmt.Errorf("%s: JSON pointer %q must be empty or start with '/'", fnName, ptr)
	}
	parts := strings.Split(ptr[1:], "/")
	for i, p := range parts {
		p = strings.ReplaceAll(p, "~1", "/")
		p = strings.ReplaceAll(p, "~0", "~")
		parts[i] = p
	}
	return parts, nil
}

// Walk resolves reference tokens against n, returning the addressed sub-node.
// A missing key, an out-of-range or malformed list index, or a descent into a
// scalar is an error naming the failing token. A key lookup is a binary search
// on a large map, so walking every key of a map is O(n log n), not O(n^2).
func Walk(fnName string, n Node, tokens []string) (Node, error) {
	cur := n
	for _, tok := range tokens {
		switch cur.k {
		case KindMap:
			pos := cur.Find(tok)
			if pos < 0 {
				return Node{}, fmt.Errorf("%s: no key %q", fnName, tok)
			}
			cur = cur.c.elems[pos]
		case KindList:
			idx, ok := ArrayIndex(tok)
			if !ok {
				return Node{}, fmt.Errorf("%s: %q is not a valid list index", fnName, tok)
			}
			if idx >= cur.Len() {
				return Node{}, fmt.Errorf("%s: list index %d out of range [0, %d)", fnName, idx, cur.Len())
			}
			cur = cur.c.elems[idx]
		default:
			return Node{}, fmt.Errorf("%s: cannot descend into %s at %q", fnName, cur.TypeName(), tok)
		}
	}
	return cur, nil
}

// Resolve parses ptr and walks it against n.
func Resolve(fnName string, n Node, ptr string) (Node, error) {
	tokens, err := ParsePointer(fnName, ptr)
	if err != nil {
		return Node{}, err
	}
	return Walk(fnName, n, tokens)
}

// ArrayIndex parses a JSON Pointer list-index token: "0" or [1-9][0-9]* (no
// leading zeros, no sign, no `-` end-marker on the read side).
func ArrayIndex(tok string) (int, bool) {
	if tok == "0" {
		return 0, true
	}
	if tok == "" || tok[0] < '1' || tok[0] > '9' {
		return 0, false
	}
	n := 0
	for i := 0; i < len(tok); i++ {
		if tok[i] < '0' || tok[i] > '9' {
			return 0, false
		}
		// Reject before the multiply/add can overflow int to a negative value
		// (a 19-20 digit token would otherwise wrap past a length check and
		// panic on a negative slice index).
		if n > (maxInt-9)/10 {
			return 0, false
		}
		n = n*10 + int(tok[i]-'0')
	}
	return n, true
}

// maxInt is the platform int max, used to reject overflowing pointer indices.
const maxInt = int(^uint(0) >> 1)

// CheckDepth validates the depth a single write introduces: a value placed at
// a pointer path of length pathLen reaches pathLen + its own nesting. The
// edited document is already within the limit (decode caps it and every
// earlier write was checked here), so the edit site is the only place that can
// newly exceed it - O(value), not O(document).
func CheckDepth(fnName string, pathLen int, v Node) error {
	if pathLen > limits.MaxNestingDepth || v.ExceedsDepth(limits.MaxNestingDepth-pathLen) {
		return fmt.Errorf("%s: result nests deeper than the %d-level limit", fnName, limits.MaxNestingDepth)
	}
	return nil
}

// ----- copy-on-write container helpers -------------------------------------
//
// Each helper returns a new node over freshly allocated slices; the input's
// slices are never written within their length (they may be shared with other
// handles). listAppend may write past the end of a list's slice, into spare
// capacity no other handle can see (see container.tail). A map's
// key slice and sorted index are reused as-is when the key set is unchanged.

// mapReplaceAt returns m with member i's value replaced.
func mapReplaceAt(m Node, i int, val Node) Node {
	elems := make([]Node, m.Len())
	copy(elems, m.c.elems)
	elems[i] = val
	return Node{k: KindMap, c: newContainer(elems, m.c.keys, m.c.order)}
}

// mapAdd returns m with a new member key: val appended (key must be absent).
// The sorted index gains the new member by insertion (O(n)) rather than a
// re-sort.
func mapAdd(m Node, key string, val Node) Node {
	n := m.Len()
	keys := make([]string, n+1)
	copy(keys, m.c.keys)
	keys[n] = key
	elems := make([]Node, n+1)
	copy(elems, m.c.elems)
	elems[n] = val
	var order []uint32
	switch {
	case n+1 <= indexThreshold:
	case m.c.order == nil:
		order = buildOrder(keys)
	default:
		at := sort.Search(n, func(i int) bool { return keys[m.c.order[i]] >= key })
		order = make([]uint32, 0, n+1)
		order = append(order, m.c.order[:at]...)
		order = append(order, uint32(n))
		order = append(order, m.c.order[at:]...)
	}
	return Node{k: KindMap, c: newContainer(elems, keys, order)}
}

// mapSet upserts key: val.
func mapSet(m Node, key string, val Node) Node {
	if i := m.Find(key); i >= 0 {
		return mapReplaceAt(m, i, val)
	}
	return mapAdd(m, key, val)
}

// mapRemoveAt returns m without member i, keeping document order.
func mapRemoveAt(m Node, i int) Node {
	n := m.Len()
	keys := make([]string, 0, n-1)
	keys = append(keys, m.c.keys[:i]...)
	keys = append(keys, m.c.keys[i+1:]...)
	elems := make([]Node, 0, n-1)
	elems = append(elems, m.c.elems[:i]...)
	elems = append(elems, m.c.elems[i+1:]...)
	var order []uint32
	if m.c.order != nil && n-1 > indexThreshold {
		order = make([]uint32, 0, n-1)
		for _, o := range m.c.order {
			switch {
			case int(o) == i:
			case int(o) > i:
				order = append(order, o-1)
			default:
				order = append(order, o)
			}
		}
	}
	return Node{k: KindMap, c: newContainer(elems, keys, order)}
}

func listReplace(l Node, idx int, val Node) Node {
	out := make([]Node, l.Len())
	copy(out, l.c.elems)
	out[idx] = val
	return List(out)
}

func listInsert(l Node, idx int, val Node) Node {
	if idx == l.Len() {
		return listAppend(l, val)
	}
	out := make([]Node, 0, l.Len()+1)
	out = append(out, l.c.elems[:idx]...)
	out = append(out, val)
	out = append(out, l.c.elems[idx:]...)
	return List(out)
}

// listAppend adds val at the end of l. When l holds the tail claim and its
// backing has room, the new list extends that backing in place; otherwise it
// copies into a backing with doubled capacity and takes the claim itself. Either
// way l is unchanged, so building a list one append at a time is linear.
func listAppend(l Node, val Node) Node {
	c := l.c
	h := uint32(1)
	if val.c != nil {
		h = val.c.height + 1
	}
	if c.height > h {
		h = c.height
	}
	n := len(c.elems)
	if n < cap(c.elems) && atomic.CompareAndSwapUint32(&c.tail, 1, 0) {
		return Node{k: KindList, c: &container{elems: append(c.elems, val), height: h, tail: 1}}
	}
	out := make([]Node, n+1, 2*n+4)
	copy(out, c.elems)
	out[n] = val
	return Node{k: KindList, c: &container{elems: out, height: h, tail: 1}}
}

func listRemove(l Node, idx int) Node {
	out := make([]Node, 0, l.Len()-1)
	out = append(out, l.c.elems[:idx]...)
	out = append(out, l.c.elems[idx+1:]...)
	return List(out)
}

// ----- spine rebuild -------------------------------------------------------

// editAt rebuilds the spine from n down to the container that holds the final
// pointer segment, then applies op(container, finalSeg) to produce the new
// container - splicing each new child back up on the way out. Intermediate
// segments must already exist (strict: no auto-vivify).
func editAt(fnName string, n Node, tokens []string, op func(container Node, seg string) (Node, error)) (Node, error) {
	if len(tokens) == 1 {
		return op(n, tokens[0])
	}
	seg := tokens[0]
	switch n.k {
	case KindMap:
		pos := n.Find(seg)
		if pos < 0 {
			return Node{}, fmt.Errorf("%s: no key %q", fnName, seg)
		}
		nc, err := editAt(fnName, n.c.elems[pos], tokens[1:], op)
		if err != nil {
			return Node{}, err
		}
		return mapReplaceAt(n, pos, nc), nil
	case KindList:
		idx, ok := ArrayIndex(seg)
		if !ok {
			return Node{}, fmt.Errorf("%s: %q is not a valid list index", fnName, seg)
		}
		if idx >= n.Len() {
			return Node{}, fmt.Errorf("%s: list index %d out of range [0, %d)", fnName, idx, n.Len())
		}
		nc, err := editAt(fnName, n.c.elems[idx], tokens[1:], op)
		if err != nil {
			return Node{}, err
		}
		return listReplace(n, idx, nc), nil
	default:
		return Node{}, fmt.Errorf("%s: cannot descend into %s at %q", fnName, n.TypeName(), seg)
	}
}

// Set places raw at the location tokens addresses: upsert a map key, or
// replace an in-range list index. An empty pointer replaces the whole node.
func Set(fnName string, n Node, tokens []string, raw Node) (Node, error) {
	if len(tokens) == 0 {
		return raw, nil
	}
	return editAt(fnName, n, tokens, func(c Node, seg string) (Node, error) {
		switch c.k {
		case KindMap:
			return mapSet(c, seg, raw), nil
		case KindList:
			idx, ok := ArrayIndex(seg)
			if !ok {
				return Node{}, fmt.Errorf("%s: %q is not a valid list index (use append / insert to grow a list)", fnName, seg)
			}
			if idx >= c.Len() {
				return Node{}, fmt.Errorf("%s: list index %d out of range [0, %d); use append / insert to grow", fnName, idx, c.Len())
			}
			return listReplace(c, idx, raw), nil
		default:
			return Node{}, fmt.Errorf("%s: cannot set a member of %s", fnName, c.TypeName())
		}
	})
}

// Insert inserts raw into the list at the location tokens addresses; the
// final segment is a list index or `-` (append at end).
func Insert(fnName string, n Node, tokens []string, raw Node) (Node, error) {
	if len(tokens) == 0 {
		return Node{}, fmt.Errorf("%s: pointer must end in a list index (or `-`)", fnName)
	}
	return editAt(fnName, n, tokens, func(c Node, seg string) (Node, error) {
		if c.k != KindList {
			return Node{}, fmt.Errorf("%s: expected a list, got a %s", fnName, c.TypeName())
		}
		idx := c.Len()
		if seg != "-" {
			i, ok := ArrayIndex(seg)
			if !ok {
				return Node{}, fmt.Errorf("%s: %q is not a valid list index", fnName, seg)
			}
			if i > c.Len() {
				return Node{}, fmt.Errorf("%s: list index %d out of range [0, %d]", fnName, i, c.Len())
			}
			idx = i
		}
		return listInsert(c, idx, raw), nil
	})
}

// Remove drops the map member or list element tokens addresses.
func Remove(fnName string, n Node, tokens []string) (Node, error) {
	return editAt(fnName, n, tokens, func(c Node, seg string) (Node, error) {
		switch c.k {
		case KindMap:
			pos := c.Find(seg)
			if pos < 0 {
				return Node{}, fmt.Errorf("%s: no key %q", fnName, seg)
			}
			return mapRemoveAt(c, pos), nil
		case KindList:
			idx, ok := ArrayIndex(seg)
			if !ok {
				return Node{}, fmt.Errorf("%s: %q is not a valid list index", fnName, seg)
			}
			if idx >= c.Len() {
				return Node{}, fmt.Errorf("%s: list index %d out of range [0, %d)", fnName, idx, c.Len())
			}
			return listRemove(c, idx), nil
		default:
			return Node{}, fmt.Errorf("%s: cannot remove a member of %s", fnName, c.TypeName())
		}
	})
}
