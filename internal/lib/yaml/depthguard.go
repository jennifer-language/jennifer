// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package yamllib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// exceedsDepth reports whether v's container nesting is deeper than budget
// levels. It recurses at most budget+1 Go frames (returning early once the
// budget is spent), so it is itself safe on an over-deep tree.
func exceedsDepth(v interpreter.Value, budget int) bool {
	switch v.Kind {
	case interpreter.KindList:
		if budget <= 0 {
			return true
		}
		for _, e := range v.List {
			if exceedsDepth(e, budget-1) {
				return true
			}
		}
	case interpreter.KindMap:
		if budget <= 0 {
			return true
		}
		for _, e := range v.Map {
			if exceedsDepth(e.Value, budget-1) {
				return true
			}
		}
	case interpreter.KindStruct:
		if budget <= 0 {
			return true
		}
		for _, f := range v.Fields {
			if exceedsDepth(f.Value, budget-1) {
				return true
			}
		}
	case interpreter.KindObject:
		if inner, ok := v.AsObject(LibraryName, "Value"); ok {
			return exceedsDepth(inner, budget)
		}
	}
	return false
}

// checkWriteDepth rejects a write result whose tree nests past MaxNestingDepth.
// The write API (set / insert / append) can otherwise grow a yaml.Value past the
// depth the decoder enforces, and every outbound path (encode, %v, ==, the
// binding-boundary DeepCopy) then recurses one Go frame per level into an
// uncatchable stack overflow. Bounding it at construction keeps them all safe.
func checkWriteDepth(fnName string, v interpreter.Value) error {
	if exceedsDepth(v, limits.MaxNestingDepth) {
		return fmt.Errorf("%s: result nests deeper than the %d-level limit", fnName, limits.MaxNestingDepth)
	}
	return nil
}
