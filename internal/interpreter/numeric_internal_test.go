// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter

import (
	"math"
	"testing"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// TestFloorDivLargeAndSpecial pins floorDiv against large quotients and
// non-finite inputs, where the int64 round-trip would otherwise produce
// platform-defined garbage.
func TestFloorDivLargeAndSpecial(t *testing.T) {
	cases := []struct{ a, b, want float64 }{
		{7, 2, 3},
		{-7, 2, -4},
		{6, 3, 2},
		{-6, 3, -2},
		{1e300, 1e-10, math.Inf(1)}, // 1e310 overflows to +Inf, not int64 garbage
	}
	for _, c := range cases {
		if got := floorDiv(c.a, c.b); got != c.want {
			t.Errorf("floorDiv(%g, %g) = %g, want %g", c.a, c.b, got, c.want)
		}
	}
	if got := floorDiv(math.NaN(), 1); !math.IsNaN(got) {
		t.Errorf("floorDiv(NaN, 1) = %g, want NaN", got)
	}
}

// TestDisplayFloatNonFinite confirms +Inf / -Inf / NaN render without the `.0`
// suffix, while ordinary whole floats keep it.
func TestDisplayFloatNonFinite(t *testing.T) {
	cases := []struct {
		f    float64
		want string
	}{
		{2, "2.0"},
		{3.14, "3.14"},
		{math.Inf(1), "+Inf"},
		{math.Inf(-1), "-Inf"},
		{math.NaN(), "NaN"},
	}
	for _, c := range cases {
		if got := DisplayFloat(c.f); got != c.want {
			t.Errorf("DisplayFloat(%g) = %q, want %q", c.f, got, c.want)
		}
	}
}

// TestCompareIntFloatNaN pins the NaN guard in compareIntFloat: a NaN is
// unordered, so it must never report equal (== 0) to an integer. Without the
// guard, int64(NaN) truncates to MinInt64 (amd64) and a NaN would compare as an
// ordinary integer, corrupting mixed int/float comparisons.
func TestCompareIntFloatNaN(t *testing.T) {
	nan := math.NaN()
	for _, n := range []int64{-1, 0, 1, math.MinInt64, math.MaxInt64} {
		if compareIntFloat(n, nan) == 0 {
			t.Errorf("compareIntFloat(%d, NaN) == 0 (reported equal); a NaN is never equal to an integer", n)
		}
	}
	// Finite comparisons are unaffected.
	if compareIntFloat(4, 4.0) != 0 {
		t.Error("compareIntFloat(4, 4.0) should be 0")
	}
	if compareIntFloat(3, 3.5) != -1 {
		t.Error("compareIntFloat(3, 3.5) should be -1")
	}
}

// TestEvalComparisonNaN pins the NaN branch of evalComparison's mixed int/float
// ordering path directly. Since `stats` now guards NaN production, this path is
// unreachable from Jennifer source, so a manufactured NaN Value is the only way
// to exercise it - keeping the interpreter-side hardening from silently rotting.
// A NaN is unordered: <, >, <=, >= are all false; == is false; != is true.
func TestEvalComparisonNaN(t *testing.T) {
	in := New()
	nan := FloatVal(math.NaN())
	one := IntVal(1)

	check := func(label string, op parser.BinaryOp, lv, rv Value, want bool) {
		t.Helper()
		v, err := in.evalComparison(op, lv, rv, "", 0, 0)
		if err != nil {
			t.Fatalf("%s: unexpected error %v", label, err)
		}
		if v.Kind != KindBool {
			t.Fatalf("%s: result kind %s, want bool", label, v.Kind)
		}
		if v.Bool != want {
			t.Errorf("%s: got %t, want %t", label, v.Bool, want)
		}
	}

	// Both operand orders: float-NaN vs int, and int vs float-NaN.
	for _, o := range []struct {
		who    string
		lv, rv Value
	}{
		{"NaN,int", nan, one},
		{"int,NaN", one, nan},
	} {
		check(o.who+" <", parser.OpLt, o.lv, o.rv, false)
		check(o.who+" >", parser.OpGt, o.lv, o.rv, false)
		check(o.who+" <=", parser.OpLe, o.lv, o.rv, false)
		check(o.who+" >=", parser.OpGe, o.lv, o.rv, false)
		check(o.who+" ==", parser.OpEq, o.lv, o.rv, false)
		check(o.who+" !=", parser.OpNeq, o.lv, o.rv, true)
	}

	// A finite mixed comparison is unaffected by the guard.
	check("3<3.5", parser.OpLt, IntVal(3), FloatVal(3.5), true)
	check("4==4.0", parser.OpEq, IntVal(4), FloatVal(4.0), true)
	check("5>4.5", parser.OpGt, IntVal(5), FloatVal(4.5), true)
}
