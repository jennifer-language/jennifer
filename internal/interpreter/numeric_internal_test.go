// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter

import (
	"math"
	"testing"
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
