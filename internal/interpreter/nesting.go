// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// Value nesting is bounded where values are built (M28.37). Every walk over a
// value - DeepCopy, Equal, retagStructs, findImpostorStruct, stampDeclaredType,
// Display, the format encoders - recurses once per nesting level, so a value
// nested without bound (`$n = N{kids: [$n]}` in a loop) overran the stack: on
// jennifer-tiny's fixed 4 MB stack at a few thousand levels, silently
// corrupting memory (a nil dereference, "slice out of range", "close of closed
// channel" from a clobbered channel), and on the default binary's growable
// stack eventually as Go's fatal stack overflow. Neither was catchable.
//
// Unbounded nesting needs a recursive type, and only a struct or enum type can
// be recursive, so every level past the static depth of a declared type is
// added by an interpreter store: a list / map / struct / enum literal, or an
// index / field / append write. Those check that the value they build stays
// within limits.MaxNestingDepth container levels (1000, 64 on jennifer-tiny)
// and raise a catchable "limit" error otherwise, the cap the json / xml / toml
// decoders and encoders already apply. A library builtin adds at most the
// static nesting of its result type on top, so every value a walk meets is at
// most about MaxNestingDepth deep. A literal caches the height it measured
// (Value.nest), so wrapping a value again and again - a tree built by moving
// the accumulator into each new node - costs O(1) per level, not a walk.

// nestHeight returns *v's nesting height, or budget+1 once it is more than
// budget. A scalar (including bytes, a task, a channel and an opaque library
// object, whose own trees the library bounds) is 0; a list, map or struct is
// one more than its deepest element, key or field (an empty one is 1); an enum
// is a container only when its variant carries a payload. A value's cached
// height (Value.nest) answers at once; otherwise the walk skips a list or map
// whose declared element types are primitive and stops as soon as the budget
// is spent, so it never recurses more than budget+1 levels whatever the
// value's real depth. It takes a pointer and builds no Value, so its TinyGo
// frame stays small.
//
//go:noinline
func nestHeight(v *Value, budget int) int {
	switch v.Kind {
	case KindList, KindMap, KindStruct:
	case KindEnum:
		if len(v.Fields) == 0 {
			return 0
		}
	default:
		return 0
	}
	if v.nest != 0 {
		return min(int(v.nest), budget+1)
	}
	if budget < 1 {
		return budget + 1
	}
	h := 0
	switch v.Kind {
	case KindList:
		if primitiveType(v.ElemTyp) {
			return 1
		}
		for k := range v.List {
			if h = max(h, nestHeight(&v.List[k], budget-1)); h >= budget {
				return budget + 1
			}
		}
	case KindMap:
		if primitiveType(v.KeyTyp) && primitiveType(v.ValTyp) {
			return 1
		}
		for k := range v.Map {
			h = max(h, nestHeight(&v.Map[k].Key, budget-1), nestHeight(&v.Map[k].Value, budget-1))
			if h >= budget {
				return budget + 1
			}
		}
	default:
		for k := range v.Fields {
			if h = max(h, nestHeight(&v.Fields[k].Value, budget-1)); h >= budget {
				return budget + 1
			}
		}
	}
	return h + 1
}

// literalChild folds one element, key or field value of a literal into *h, the
// deepest so far, and returns the catchable "limit" error, positioned at node,
// when the literal would nest past limits.MaxNestingDepth. The literal then
// caches its height, *h + 1, in Value.nest.
func literalChild(v *Value, node parser.Node, h *int) error {
	c := nestHeight(v, limits.MaxNestingDepth-1)
	if c > limits.MaxNestingDepth-1 {
		file, line, col := posFor(node)
		return nestingLimitError(file, line, col)
	}
	*h = max(*h, c)
	return nil
}

// checkNesting returns the catchable "limit" error, positioned at node, when v
// stored `levels` containers below the root of the value being written (the
// step count for an lvalue write) would nest the result past
// limits.MaxNestingDepth; nil otherwise. The root's other branches were within
// the cap already. The writer clears the cached height of every container
// along the path (clearNestPath).
func checkNesting(levels int, v *Value, node parser.Node) error {
	budget := limits.MaxNestingDepth - levels
	if nestHeight(v, budget) <= budget {
		return nil
	}
	file, line, col := posFor(node)
	return nestingLimitError(file, line, col)
}

// nestingLimitError is the "limit" error for a value built past the nesting cap.
func nestingLimitError(file string, line, col int) *runtimeError {
	return &runtimeError{
		Kind: "limit",
		Msg:  fmt.Sprintf("value nests deeper than the %d-level limit", limits.MaxNestingDepth),
		File: file, Line: line, Col: col,
	}
}
