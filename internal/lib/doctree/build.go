// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package doctree

// Builder grows containers in place, for a decoder that cannot build each
// container bottom-up in one go: TOML reopens a table after its siblings
// (`[a]`, `[b]`, `[a.c]`) and appends to arrays of tables. The containers it
// grows must be ones the decoder alone holds; Seal turns the result into an
// ordinary immutable tree. A Node handle to a container stays valid while it
// grows, because growth happens behind the container pointer.
type Builder struct {
	// idx is a lookup index for each map that has grown past indexThreshold,
	// so building a wide table stays O(n), not O(n^2). Dropped by Seal.
	idx map[*container]map[string]int
}

// NewMap returns an empty map container for building.
func (b *Builder) NewMap() Node { return Node{k: KindMap, c: &container{height: 1}} }

// NewList returns an empty list container for building.
func (b *Builder) NewList() Node { return Node{k: KindList, c: &container{height: 1}} }

// Lookup returns the position of key in the map m, or -1.
func (b *Builder) Lookup(m Node, key string) int {
	c := m.c
	if len(c.keys) <= indexThreshold {
		for i, k := range c.keys {
			if k == key {
				return i
			}
		}
		return -1
	}
	ix := b.index(c)
	if i, ok := ix[key]; ok {
		return i
	}
	return -1
}

// index returns (building on first use) the lookup index of a large map.
func (b *Builder) index(c *container) map[string]int {
	if b.idx == nil {
		b.idx = map[*container]map[string]int{}
	}
	ix, ok := b.idx[c]
	if !ok {
		ix = make(map[string]int, len(c.keys))
		for i, k := range c.keys {
			ix[k] = i
		}
		b.idx[c] = ix
	}
	return ix
}

// Add appends the member key: v to the map m (key must be absent) and returns
// its position.
func (b *Builder) Add(m Node, key string, v Node) int {
	c := m.c
	c.keys = append(c.keys, key)
	c.elems = append(c.elems, v)
	pos := len(c.keys) - 1
	if ix, ok := b.idx[c]; ok {
		ix[key] = pos
	}
	return pos
}

// Push appends v to the list l.
func (b *Builder) Push(l Node, v Node) {
	l.c.elems = append(l.c.elems, v)
}

// Seal finishes a built tree: every container's slices are trimmed to their
// length, every large map gets its sorted lookup index, and every container its
// height. The tree must not
// be grown afterwards. Recursion is bounded by the decoder's nesting cap.
func (b *Builder) Seal(root Node) Node {
	seal(root)
	b.idx = nil
	return root
}

func seal(n Node) {
	if !n.IsContainer() {
		return
	}
	c := n.c
	if cap(c.elems) > len(c.elems) {
		elems := make([]Node, len(c.elems))
		copy(elems, c.elems)
		c.elems = elems
	}
	if n.k == KindMap {
		if cap(c.keys) > len(c.keys) {
			keys := make([]string, len(c.keys))
			copy(keys, c.keys)
			c.keys = keys
		}
		c.order = buildOrder(c.keys)
	}
	for _, e := range c.elems {
		seal(e)
	}
	c.height = heightOf(c.elems) // after the children: bottom-up
}
