// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package treedepth is the shared container-nesting depth guard for the
// json / toml / yaml write surfaces. Each of those libraries used to carry an
// identical copy of the recursion; it lives here once, parameterised by the
// library's opaque-object type so a nested json.Value / toml.Value / yaml.Value
// is still descended.
package treedepth

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// Exceeds reports whether v's container nesting is deeper than budget levels.
// It recurses at most budget+1 Go frames (returning early once the budget is
// spent), so it is itself safe on an over-deep tree. objNS/objName name the
// library's opaque object kind (e.g. "json"/"Value") so a wrapped document node
// embedded in the tree is unwrapped and descended.
func Exceeds(v interpreter.Value, budget int, objNS, objName string) bool {
	switch v.Kind {
	case interpreter.KindList:
		if budget <= 0 {
			return true
		}
		for _, e := range v.List {
			if Exceeds(e, budget-1, objNS, objName) {
				return true
			}
		}
	case interpreter.KindMap:
		if budget <= 0 {
			return true
		}
		for _, e := range v.Map {
			if Exceeds(e.Value, budget-1, objNS, objName) {
				return true
			}
		}
	case interpreter.KindStruct:
		if budget <= 0 {
			return true
		}
		for _, f := range v.Fields {
			if Exceeds(f.Value, budget-1, objNS, objName) {
				return true
			}
		}
	case interpreter.KindObject:
		if inner, ok := v.AsObject(objNS, objName); ok {
			return Exceeds(inner, budget, objNS, objName)
		}
	}
	return false
}

// CheckAt validates the depth a single write introduces. A value v placed at a
// pointer path of length pathLen reaches, at its deepest, pathLen + v's own
// nesting. Because the document being edited is already within the limit (decode
// caps it, and every prior write was checked here), the edit site is the only
// place that can newly exceed it - so this checks pathLen + depth(v) rather than
// re-scanning the whole result, which is O(v) instead of O(document). fnName is
// the calling verb, for the error text (unchanged from the old whole-tree scan).
func CheckAt(fnName string, pathLen int, v interpreter.Value, objNS, objName string) error {
	if pathLen > limits.MaxNestingDepth || Exceeds(v, limits.MaxNestingDepth-pathLen, objNS, objName) {
		return fmt.Errorf("%s: result nests deeper than the %d-level limit", fnName, limits.MaxNestingDepth)
	}
	return nil
}
