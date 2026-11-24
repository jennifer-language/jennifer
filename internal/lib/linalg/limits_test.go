// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package linalglib

import (
	"math"
	"runtime"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
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

// TestCubicRoutinesCancel: matmul, determinant, solve and inverse poll the
// task's cancel flag once per outer row / column, so task.cancel stops them.
func TestCubicRoutinesCancel(t *testing.T) {
	rows := [][]float64{{4, 1, 2}, {1, 5, 3}, {2, 3, 6}}
	a := mat(rows...)
	v := vec(1, 2, 3)
	cases := []struct {
		name string
		fn   interpreter.Builtin
		args []interpreter.Value
	}{
		{"matmul", matmulFn, []interpreter.Value{a, a}},
		{"determinant", determinantFn, []interpreter.Value{a}},
		{"solve", solveFn, []interpreter.Value{a, v}},
		{"inverse", inverseFn, []interpreter.Value{a}},
	}
	for _, tc := range cases {
		ts := &interpreter.TaskState{}
		ts.RequestCancel()
		_, err := tc.fn(interpreter.BuiltinCtx{Cancel: ts}, tc.args)
		if err == nil || !strings.Contains(err.Error(), "task cancelled") {
			t.Errorf("%s: got %v, want task cancelled", tc.name, err)
		}
		if _, err := tc.fn(interpreter.BuiltinCtx{Cancel: &interpreter.TaskState{}}, tc.args); err != nil {
			t.Errorf("%s, uncancelled task: %v", tc.name, err)
		}
	}
}
