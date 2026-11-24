// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package linalglib

import (
	"math"
	"math/rand/v2"
	"testing"
)

// TestMatmulBitIdentical: matmul's i-k-j loop gives the same bits as the
// i-j-k sum it replaced (each element summed from k = 0 up, from 0), for
// matrix x matrix and vector x matrix, with signed values of mixed magnitude
// where any reordering of the sum would change the rounding.
func TestMatmulBitIdentical(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	val := func() float64 { return (rng.Float64() - 0.5) * math.Pow(10, float64(rng.IntN(12)-6)) }
	m, n, p := 13, 37, 11
	a := make([][]float64, m)
	for i := range a {
		a[i] = make([]float64, n)
		for k := range a[i] {
			a[i][k] = val()
		}
	}
	b := make([][]float64, n)
	for k := range b {
		b[k] = make([]float64, p)
		for j := range b[k] {
			b[k][j] = val()
		}
	}
	v, err := call(matmulFn, mat(a...), mat(b...))
	if err != nil {
		t.Fatal(err)
	}
	rows := v.List()
	for i := 0; i < m; i++ {
		got := floats(t, rows[i])
		for j := 0; j < p; j++ {
			s := 0.0
			for k := 0; k < n; k++ {
				s += a[i][k] * b[k][j]
			}
			if math.Float64bits(got[j]) != math.Float64bits(s) {
				t.Fatalf("[%d][%d] = %v, i-j-k sum %v", i, j, got[j], s)
			}
		}
	}
	rv, err := call(matmulFn, vec(a[0]...), mat(b...))
	if err != nil {
		t.Fatal(err)
	}
	got := floats(t, rv)
	for j := 0; j < p; j++ {
		s := 0.0
		for k := 0; k < n; k++ {
			s += a[0][k] * b[k][j]
		}
		if math.Float64bits(got[j]) != math.Float64bits(s) {
			t.Fatalf("vector x matrix [%d] = %v, i-j-k sum %v", j, got[j], s)
		}
	}
}
