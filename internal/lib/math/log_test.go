// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package mathlib

import (
	"fmt"
	"math"
	"strconv"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// callable returns the installed math builtin name.
func callable(name string) interpreter.Builtin {
	in := interpreter.New()
	Install(in)
	return in.LookupNamespacedBuiltin("math", name)
}

// TestLogExactAtPowers: ln(x)/ln(base) gave log(1000, 10) = 2.9999999999999996
// (floor() then counted 2 digits) and log(1e-300, 10) = -299.99999999999994.
func TestLogExactAtPowers(t *testing.T) {
	for k := -307; k <= 308; k++ {
		x, _ := strconv.ParseFloat(fmt.Sprintf("1e%d", k), 64)
		if got := logBase(x, 10); got != float64(k) {
			t.Errorf("log(1e%d, 10) = %v", k, got)
		}
	}
	for k := -1074; k <= 1023; k++ {
		if got := logBase(math.Ldexp(1, k), 2); got != float64(k) {
			t.Errorf("log(2^%d, 2) = %v", k, got)
		}
	}
	for _, b := range []float64{3, 5, 7, 16, 1000} {
		for k := 0.0; math.Pow(b, k) < 1<<53; k++ {
			if got := logBase(math.Pow(b, k), b); got != k {
				t.Errorf("log(%v^%v, %v) = %v", b, k, b, got)
			}
		}
	}
	// Not a power: unchanged, within an ulp or two of the quotient.
	for _, c := range [][2]float64{{999, 10}, {1001, 10}, {2, 10}, {10, 3}, {0.5, 7}} {
		want := math.Log(c[0]) / math.Log(c[1])
		if got := logBase(c[0], c[1]); got != want {
			t.Errorf("log(%v, %v) = %v, want %v", c[0], c[1], got, want)
		}
	}
	// Subnormal arguments: Go's amd64 Log is off there (ln(1e-308) gave
	// -708.718 instead of -709.196).
	for _, c := range [][2]float64{{1e-308, -709.1962086421661}, {5e-324, -744.4400719213812}, {0x1p-1060, -1060 * math.Ln2}} {
		if got := callOK(t, callable("ln"), f(c[0])).Float; !closeTo(got, c[1], 1e-12) {
			t.Errorf("ln(%v) = %v, want %v", c[0], got, c[1])
		}
	}
	if got := callOK(t, callable("log10"), f(1e-310)).Float; !closeTo(got, -310, 1e-9) {
		t.Errorf("log10(1e-310) = %v", got)
	}
	callErr(t, callable("log"), f(0), f(10))
	callErr(t, callable("log"), f(10), f(1))
	if v := callOK(t, callable("log"), i(1000), i(10)); v.Float != 3 {
		t.Errorf("math.log(1000, 10) = %v", v.Float)
	}
}
