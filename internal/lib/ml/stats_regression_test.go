// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package mllib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	statslib "jennifer-lang.dev/jennifer/internal/lib/stats"
)

// TestMultipleRegressionReplacement (M29.48): ml.linearRegression gives the
// numbers of the deprecated stats.multipleRegression ([intercept, b1, ...]
// against ml.intercept and ml.coefficients) on a noisy, non-exact design.
func TestMultipleRegressionReplacement(t *testing.T) {
	X := mat([]float64{1, 1}, []float64{2, 1}, []float64{1, 2}, []float64{3, 2},
		[]float64{2, 3}, []float64{4, 1.5}, []float64{0.5, 4}, []float64{3.5, 3.5})
	y := vec(6.1, 7.8, 9.3, 12.9, 14.2, 11.1, 13.4, 18.0)

	in := interpreter.New()
	statslib.Install(in)
	beta, err := in.LookupNamespacedBuiltin("stats", "multipleRegression")(interpreter.BuiltinCtx{}, []interpreter.Value{X, y})
	if err != nil {
		t.Fatal(err)
	}
	m := ok(t, linearRegressionFn, X, y)
	icpt := ok(t, interceptFn, m).Float
	coef := fvals(ok(t, coefficientsFn, m))
	got := append([]float64{icpt}, coef...)
	want := fvals(beta)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if !near(got[i], want[i], 1e-9) {
			t.Errorf("coefficient %d: ml %v, stats %v", i, got[i], want[i])
		}
	}
}
