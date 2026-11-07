// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package linalglib

import (
	"math"
	"strings"
	"testing"
)

// TestSingularUpToRounding: [[1,2,3],[4,5,6],[7,8,9]] is singular, but its
// last pivot rounds to ~1e-15, not 0; inverse returned entries near 6e14 and
// solve an arbitrary vector. Badly scaled but regular matrices still work.
func TestSingularUpToRounding(t *testing.T) {
	for _, m := range [][][]float64{
		{{1, 2, 3}, {4, 5, 6}, {7, 8, 9}},
		{{0.1, 0.2, 0.3}, {0.4, 0.5, 0.6}, {0.7, 0.8, 0.9}},
		{{1e-30, 2e-30}, {3e-30, 6.000000000000001e-30}},
	} {
		a := mat(m...)
		if _, err := call(inverseFn, a); err == nil || !strings.Contains(err.Error(), "singular") {
			t.Errorf("inverse(%v): got %v, want a singular error", m, err)
		}
		b := make([]float64, len(m))
		for i := range b {
			b[i] = float64(i + 1)
		}
		if _, err := call(solveFn, a, vec(b...)); err == nil || !strings.Contains(err.Error(), "singular") {
			t.Errorf("solve(%v): got %v, want a singular error", m, err)
		}
	}
	// Regular, however scaled.
	for _, tc := range []struct {
		m    [][]float64
		b, x []float64
	}{
		{[][]float64{{1e-20, 0}, {0, 1}}, []float64{1e-20, 2}, []float64{1, 2}},
		{[][]float64{{1e-20, 1}, {0, 1}}, []float64{2, 1}, []float64{1e20, 1}},
		{[][]float64{{1e200, 0}, {0, 1e-200}}, []float64{3e200, 4e-200}, []float64{3, 4}},
		{[][]float64{{2, 1}, {1, 3}}, []float64{3, 5}, []float64{0.8, 1.4}},
	} {
		v, err := call(solveFn, mat(tc.m...), vec(tc.b...))
		if err != nil {
			t.Errorf("solve(%v, %v): %v", tc.m, tc.b, err)
			continue
		}
		got := floats(t, v)
		for i := range got {
			if math.Abs(got[i]-tc.x[i]) > 1e-12*math.Abs(tc.x[i]) {
				t.Errorf("solve(%v, %v) = %v, want %v", tc.m, tc.b, got, tc.x)
				break
			}
		}
		if _, err := call(inverseFn, mat(tc.m...)); err != nil {
			t.Errorf("inverse(%v): %v", tc.m, err)
		}
	}
}
