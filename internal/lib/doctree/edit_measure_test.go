// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package doctree

import (
	"fmt"
	"math/rand"
	"testing"
)

// checkMeasured fails when any container in n carries a height or size that a
// full re-measure of its elements disagrees with.
func checkMeasured(t *testing.T, step int, n Node) {
	t.Helper()
	if n.c == nil {
		return
	}
	for _, e := range n.c.elems {
		checkMeasured(t, step, e)
	}
	h, size := measure(n.c.elems)
	if n.c.height != h || n.c.size != size {
		t.Fatalf("step %d: container carries height %d size %d, measure gives %d %d",
			step, n.c.height, n.c.size, h, size)
	}
}

// randomValue is a scalar or a small nested container, so edits move heights
// both up and down.
func randomValue(r *rand.Rand) Node {
	switch r.Intn(4) {
	case 0:
		return Int(r.Int63())
	case 1:
		return List([]Node{Int(1), List([]Node{String("x")})})
	case 2:
		return Map([]string{"a"}, []Node{Map([]string{"b"}, []Node{List([]Node{Int(2)})})})
	default:
		return String("s")
	}
}

// The incremental height / size of every edit verb matches a full measure,
// including edits that lower a container's height (the deepest member replaced
// by a scalar, or removed).
func TestIncrementalMeasureMatchesFullMeasure(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	root := Map(nil, nil)
	root, _ = Set("t", root, []string{"m"}, Map(nil, nil))
	root, _ = Set("t", root, []string{"l"}, List(nil))
	for step := 0; step < 4000; step++ {
		var err error
		key := fmt.Sprintf("k%d", r.Intn(20))
		m, _ := Walk("t", root, []string{"m"})
		l, _ := Walk("t", root, []string{"l"})
		switch r.Intn(6) {
		case 0, 1:
			root, err = Set("t", root, []string{"m", key}, randomValue(r))
		case 2:
			if m.Find(key) >= 0 {
				root, err = Remove("t", root, []string{"m", key})
			}
		case 3:
			root, err = Insert("t", root, []string{"l", fmt.Sprint(r.Intn(l.Len() + 1))}, randomValue(r))
		case 4:
			if l.Len() > 0 {
				root, err = Set("t", root, []string{"l", fmt.Sprint(r.Intn(l.Len()))}, randomValue(r))
			}
		case 5:
			if l.Len() > 0 {
				root, err = Remove("t", root, []string{"l", fmt.Sprint(r.Intn(l.Len()))})
			}
		}
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		checkMeasured(t, step, root)
	}
}
