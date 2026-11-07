// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package listslib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestConcatPushCap: concat and push refuse a result over
// limits.MaxRangeElements elements with a catchable error. Before the fix
// `$xs = lists.concat($xs, $xs)` doubled until the process ran out of memory.
func TestConcatPushCap(t *testing.T) {
	if maxListElements != limits.MaxRangeElements {
		t.Fatalf("maxListElements %d, want limits.MaxRangeElements", maxListElements)
	}
	defer func(n int) { maxListElements = n }(maxListElements)
	maxListElements = 1000
	half := interpreter.Value{Kind: interpreter.KindList, List: make([]interpreter.Value, 500)}
	full, err := concatFn(interpreter.BuiltinCtx{}, []interpreter.Value{half, half})
	if err != nil || len(full.List) != 1000 {
		t.Fatalf("concat at the cap: %v", err)
	}
	if _, err := concatFn(interpreter.BuiltinCtx{}, []interpreter.Value{full, intList(1)}); err == nil || !strings.Contains(err.Error(), "lists.concat: ") {
		t.Fatalf("concat past the cap: %v", err)
	}
	if _, err := pushFn(interpreter.BuiltinCtx{}, []interpreter.Value{full, interpreter.IntVal(1)}); err == nil || !strings.Contains(err.Error(), "lists.push: ") {
		t.Fatalf("push past the cap: %v", err)
	}
}
