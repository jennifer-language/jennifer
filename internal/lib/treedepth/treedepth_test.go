// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package treedepth

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// nest builds n nested single-element lists around a scalar, so the innermost
// scalar sits at container depth n.
func nest(n int) interpreter.Value {
	v := interpreter.IntVal(1)
	for i := 0; i < n; i++ {
		v = interpreter.ListVal(parser.Type{}, []interpreter.Value{v})
	}
	return v
}

func TestExceedsBoundary(t *testing.T) {
	if Exceeds(nest(5), 5, "json", "Value") {
		t.Errorf("depth 5 within budget 5 should be allowed")
	}
	if !Exceeds(nest(6), 5, "json", "Value") {
		t.Errorf("depth 6 past budget 5 should be rejected")
	}
	if Exceeds(interpreter.IntVal(1), 0, "json", "Value") {
		t.Errorf("a scalar never exceeds any budget")
	}
}

// TestCheckAtMatchesFullScan pins that the touched-node check gives the same
// verdict as a whole-result scan: value at pathLen reaches pathLen + its depth.
func TestCheckAtMatchesFullScan(t *testing.T) {
	cap := limits.MaxNestingDepth
	// A scalar placed exactly at the limit is fine; one past is not.
	if err := CheckAt("t.set", cap, interpreter.IntVal(1), "json", "Value"); err != nil {
		t.Errorf("scalar at depth cap should pass: %v", err)
	}
	if err := CheckAt("t.set", cap+1, interpreter.IntVal(1), "json", "Value"); err == nil {
		t.Errorf("scalar at depth cap+1 should fail")
	}
	// A value with its own nesting: pathLen + depth(v).
	if err := CheckAt("t.set", cap-3, nest(3), "json", "Value"); err != nil {
		t.Errorf("nest(3) at depth cap-3 should pass: %v", err)
	}
	if err := CheckAt("t.set", cap-3, nest(4), "json", "Value"); err == nil {
		t.Errorf("nest(4) at depth cap-3 (=cap+1) should fail")
	}
}

func TestCheckAtErrorText(t *testing.T) {
	err := CheckAt("json.set", limits.MaxNestingDepth+1, interpreter.IntVal(1), "json", "Value")
	if err == nil || !strings.Contains(err.Error(), "nests deeper") {
		t.Fatalf("want a depth-limit error, got %v", err)
	}
}
