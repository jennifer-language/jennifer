// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package linalglib

import (
	"math"
	"runtime"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestMatmulOutputCap: an N x 1 column times a 1 x N row is within the
// element cap on input but N^2 on output; refused before allocating it.
func TestMatmulOutputCap(t *testing.T) {
	n := int(math.Sqrt(float64(limits.MaxMatrixElements))) + 76
	col := make([][]float64, n)
	row := make([]float64, n)
	for i := range col {
		col[i] = []float64{1}
		row[i] = 1
	}
	a, b := mat(col...), mat(row)
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	_, err := call(matmulFn, a, b)
	runtime.ReadMemStats(&m1)
	if err == nil || !strings.Contains(err.Error(), "linalg.matmul: result too large") {
		t.Fatalf("got %v", err)
	}
	if got := m1.TotalAlloc - m0.TotalAlloc; got > 4<<20 {
		t.Fatalf("allocated %d bytes before refusing", got)
	}
}
