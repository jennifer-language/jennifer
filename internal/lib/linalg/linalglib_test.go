// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package linalglib

import (
	"math"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// vec builds a `list of float` Value from raw floats.
func vec(fs ...float64) interpreter.Value {
	cells := make([]interpreter.Value, len(fs))
	for i, f := range fs {
		cells[i] = interpreter.FloatVal(f)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeFloat), cells)
}

// mat builds a `list of list of float` Value from raw rows.
func mat(rows ...[]float64) interpreter.Value {
	rowType := parser.ListType(parser.PrimitiveType(parser.TypeFloat))
	vals := make([]interpreter.Value, len(rows))
	for i, r := range rows {
		vals[i] = vec(r...)
	}
	return interpreter.ListVal(rowType, vals)
}

// call invokes a builtin with the given args.
func call(fn interpreter.Builtin, args ...interpreter.Value) (interpreter.Value, error) {
	return fn(interpreter.BuiltinCtx{}, args)
}

// floats reads back a `list of float` result.
func floats(t *testing.T, v interpreter.Value) []float64 {
	t.Helper()
	if v.Kind != interpreter.KindList {
		t.Fatalf("expected list result, got %s", v.Kind)
	}
	out := make([]float64, len(v.List))
	for i, e := range v.List {
		f, ok := e.AsFloat()
		if !ok {
			t.Fatalf("element %d is not numeric: %s", i, e.Kind)
		}
		out[i] = f
	}
	return out
}

func approxEq(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestVectors(t *testing.T) {
	if v, err := call(dotFn, vec(1, 2, 3), vec(4, 5, 6)); err != nil || v.Float != 32 {
		t.Fatalf("dot = %v, %v; want 32", v, err)
	}
	if v, err := call(normFn, vec(3, 4)); err != nil || v.Float != 5 {
		t.Fatalf("norm = %v, %v; want 5", v, err)
	}
	if v, err := call(distanceFn, vec(0, 0), vec(3, 4)); err != nil || v.Float != 5 {
		t.Fatalf("distance = %v, %v; want 5", v, err)
	}
	if v, err := call(scaleFn, vec(1, 2), interpreter.FloatVal(3)); err != nil {
		t.Fatalf("scale err %v", err)
	} else if got := floats(t, v); got[0] != 3 || got[1] != 6 {
		t.Fatalf("scale = %v; want [3 6]", got)
	}
	if v, err := call(addFn, vec(1, 2), vec(3, 4)); err != nil {
		t.Fatalf("add err %v", err)
	} else if got := floats(t, v); got[0] != 4 || got[1] != 6 {
		t.Fatalf("add = %v; want [4 6]", got)
	}
	if v, err := call(subFn, vec(5, 7), vec(3, 4)); err != nil {
		t.Fatalf("sub err %v", err)
	} else if got := floats(t, v); got[0] != 2 || got[1] != 3 {
		t.Fatalf("sub = %v; want [2 3]", got)
	}
	// x cross y = z (right-hand rule).
	if v, err := call(crossFn, vec(1, 0, 0), vec(0, 1, 0)); err != nil {
		t.Fatalf("cross err %v", err)
	} else if got := floats(t, v); got[0] != 0 || got[1] != 0 || got[2] != 1 {
		t.Fatalf("cross = %v; want [0 0 1]", got)
	}
}

func TestMatrices(t *testing.T) {
	a := mat([]float64{1, 2}, []float64{3, 4})
	if v, err := call(matmulFn, a, mat([]float64{5, 6}, []float64{7, 8})); err != nil {
		t.Fatalf("matmul err %v", err)
	} else {
		got := v.List
		r0 := floats(t, got[0])
		r1 := floats(t, got[1])
		if r0[0] != 19 || r0[1] != 22 || r1[0] != 43 || r1[1] != 50 {
			t.Fatalf("matmul = [%v %v]; want [[19 22][43 50]]", r0, r1)
		}
	}
	if v, err := call(determinantFn, a); err != nil || v.Float != -2 {
		t.Fatalf("det = %v, %v; want -2", v, err)
	}
	if v, err := call(transposeFn, a); err != nil {
		t.Fatalf("transpose err %v", err)
	} else {
		r0 := floats(t, v.List[0])
		if r0[0] != 1 || r0[1] != 3 {
			t.Fatalf("transpose row0 = %v; want [1 3]", r0)
		}
	}
	// inverse then multiply back -> identity.
	inv, err := call(inverseFn, a)
	if err != nil {
		t.Fatalf("inverse err %v", err)
	}
	prod, err := call(matmulFn, a, inv)
	if err != nil {
		t.Fatalf("matmul(a, inv) err %v", err)
	}
	id := prod.List
	if !approxEq(floats(t, id[0])[0], 1) || !approxEq(floats(t, id[0])[1], 0) ||
		!approxEq(floats(t, id[1])[0], 0) || !approxEq(floats(t, id[1])[1], 1) {
		t.Fatalf("a * inverse(a) is not identity: %v", prod)
	}
}

func TestSolve(t *testing.T) {
	// 2x + y = 3 ; x + 3y = 5  ->  x = 0.8, y = 1.4
	x, err := call(solveFn, mat([]float64{2, 1}, []float64{1, 3}), vec(3, 5))
	if err != nil {
		t.Fatalf("solve err %v", err)
	}
	got := floats(t, x)
	if !approxEq(got[0], 0.8) || !approxEq(got[1], 1.4) {
		t.Fatalf("solve = %v; want [0.8 1.4]", got)
	}
}

func TestIdentityAndShape(t *testing.T) {
	id, err := call(identityFn, interpreter.IntVal(3))
	if err != nil {
		t.Fatalf("identity err %v", err)
	}
	if len(id.List) != 3 {
		t.Fatalf("identity rows = %d; want 3", len(id.List))
	}
	diag := floats(t, id.List[1])
	if diag[0] != 0 || diag[1] != 1 || diag[2] != 0 {
		t.Fatalf("identity row1 = %v; want [0 1 0]", diag)
	}
	sh, err := call(shapeFn, mat([]float64{1, 2, 3}, []float64{4, 5, 6}))
	if err != nil {
		t.Fatalf("shape err %v", err)
	}
	if sh.List[0].Int != 2 || sh.List[1].Int != 3 {
		t.Fatalf("shape = [%d %d]; want [2 3]", sh.List[0].Int, sh.List[1].Int)
	}
}

func TestMatrixArithmetic(t *testing.T) {
	a := mat([]float64{1, 2}, []float64{3, 4})
	b := mat([]float64{5, 6}, []float64{7, 8})
	// add / sub / scale are polymorphic over matrices.
	if v, err := call(addFn, a, b); err != nil {
		t.Fatalf("mat add err %v", err)
	} else if r := floats(t, v.List[0]); r[0] != 6 || r[1] != 8 {
		t.Fatalf("mat add row0 = %v; want [6 8]", r)
	}
	if v, err := call(subFn, b, a); err != nil {
		t.Fatalf("mat sub err %v", err)
	} else if r := floats(t, v.List[1]); r[0] != 4 || r[1] != 4 {
		t.Fatalf("mat sub row1 = %v; want [4 4]", r)
	}
	if v, err := call(scaleFn, a, interpreter.FloatVal(2)); err != nil {
		t.Fatalf("mat scale err %v", err)
	} else if r := floats(t, v.List[0]); r[0] != 2 || r[1] != 4 {
		t.Fatalf("mat scale row0 = %v; want [2 4]", r)
	}
	// trace of the main diagonal.
	if v, err := call(traceFn, a); err != nil || v.Float != 5 {
		t.Fatalf("trace = %v, %v; want 5", v, err)
	}
	// zeros builds a rows x cols matrix.
	if v, err := call(zerosFn, interpreter.IntVal(2), interpreter.IntVal(3)); err != nil {
		t.Fatalf("zeros err %v", err)
	} else if len(v.List) != 2 || len(v.List[0].List) != 3 || floats(t, v.List[0])[2] != 0 {
		t.Fatalf("zeros = %v; want 2x3 of 0", v)
	}
}

func TestMatVec(t *testing.T) {
	a := mat([]float64{1, 2}, []float64{3, 4})
	// matrix x vector -> vector (column).
	if v, err := call(matmulFn, a, vec(1, 1)); err != nil {
		t.Fatalf("M*v err %v", err)
	} else if got := floats(t, v); got[0] != 3 || got[1] != 7 {
		t.Fatalf("M*v = %v; want [3 7]", got)
	}
	// vector x matrix -> vector (row).
	if v, err := call(matmulFn, vec(1, 1), a); err != nil {
		t.Fatalf("v*M err %v", err)
	} else if got := floats(t, v); got[0] != 4 || got[1] != 6 {
		t.Fatalf("v*M = %v; want [4 6]", got)
	}
	// two vectors are ambiguous - an error, not a silent dot product.
	if _, err := call(matmulFn, vec(1, 2), vec(3, 4)); err == nil {
		t.Error("matmul of two vectors: expected an error, got nil")
	}
}

func TestNormAndNormalize(t *testing.T) {
	// normalize -> unit vector.
	if v, err := call(normalizeFn, vec(3, 4)); err != nil {
		t.Fatalf("normalize err %v", err)
	} else if got := floats(t, v); !approxEq(got[0], 0.6) || !approxEq(got[1], 0.8) {
		t.Fatalf("normalize = %v; want [0.6 0.8]", got)
	}
	// the zero vector has no direction.
	if _, err := call(normalizeFn, vec(0, 0)); err == nil {
		t.Error("normalize(zero): expected an error, got nil")
	}
	// norm is polymorphic: Frobenius norm of a matrix.
	if v, err := call(normFn, mat([]float64{3, 0}, []float64{0, 4})); err != nil || v.Float != 5 {
		t.Fatalf("frobenius norm = %v, %v; want 5", v, err)
	}
	// Strict: a magnitude overflow must error, not silently return a zero vector
	// (which dividing by +Inf would yield) - consistent with norm.
	big := math.Pow(10, 308)
	if _, err := call(normalizeFn, vec(big, big)); err == nil {
		t.Error("normalize of an overflowing vector: expected an error, got nil")
	}
}

// TestElementBudget pins the shared allocation bound directly, without building
// an oversized value: exceedsElementBudget is the single check every entry point
// uses (both constructors and the vector / matrix readers), so exercising its
// logic covers the reader guard - which otherwise could only be tested by
// actually materialising a ~285 MiB input. A square shape at the cap is allowed;
// one element past it, an oversized single dimension, and a thin N x 1 shape whose
// product exceeds the cap are all rejected.
func TestElementBudget(t *testing.T) {
	budget := int64(limits.MaxMatrixElements)
	isqrt := int64(1) << 10 // 1024; 1024*1024 == 1<<20 == cap on the default build
	cases := []struct {
		r, c   int64
		exceed bool
	}{
		{1, 1, false},
		{isqrt, isqrt, isqrt*isqrt > budget}, // square at (or under) the cap
		{budget, 1, false},                   // a full column, exactly the cap
		{budget + 1, 1, true},                // one past the cap in one dimension
		{1, budget + 1, true},                // thin row past the cap
		{budget, 2, true},                    // product exceeds the cap
		{budget / 2, 3, true},                // product exceeds the cap without a huge single dim
	}
	for _, c := range cases {
		if got := exceedsElementBudget(c.r, c.c); got != c.exceed {
			t.Errorf("exceedsElementBudget(%d, %d) = %t, want %t", c.r, c.c, got, c.exceed)
		}
	}
}

// TestConstructorAllocationBounded pins the OOM guard at the builtin boundary:
// identity / zeros size their allocation from an integer argument, so an oversize
// dimension must be a catchable error rather than a fatal, uncatchable make() OOM.
// The guard fires before any allocation, so these stay cheap. A dimension at or
// below the cap still builds.
func TestConstructorAllocationBounded(t *testing.T) {
	over := int64(limits.MaxMatrixElements) + 1
	if _, err := call(identityFn, interpreter.IntVal(over)); err == nil {
		t.Error("identity past the element cap: expected an error, got nil")
	}
	if _, err := call(zerosFn, interpreter.IntVal(over), interpreter.IntVal(2)); err == nil {
		t.Error("zeros past the element cap: expected an error, got nil")
	}
	// A thin N x 1 shape must not slip past via the product check.
	if _, err := call(zerosFn, interpreter.IntVal(over), interpreter.IntVal(1)); err == nil {
		t.Error("zeros with an oversized thin dimension: expected an error, got nil")
	}
	// A modest matrix still builds.
	if v, err := call(identityFn, interpreter.IntVal(4)); err != nil || len(v.List) != 4 {
		t.Fatalf("identity(4) = %v, %v; want a 4x4 matrix", v, err)
	}
}

// TestStrictErrors pins the catchable-error contract: dimension mismatches,
// non-rectangular matrices, non-square operands, singular inverse/solve, and a
// bad cross length are all errors rather than silent wrong answers.
func TestStrictErrors(t *testing.T) {
	cases := []struct {
		name string
		call func() (interpreter.Value, error)
	}{
		{"dot mismatch", func() (interpreter.Value, error) { return call(dotFn, vec(1, 2), vec(1)) }},
		{"non-rectangular", func() (interpreter.Value, error) {
			return call(transposeFn, mat([]float64{1, 2}, []float64{3}))
		}},
		{"matmul inner mismatch", func() (interpreter.Value, error) {
			return call(matmulFn, mat([]float64{1, 2, 3}), mat([]float64{1, 2}))
		}},
		{"non-square determinant", func() (interpreter.Value, error) {
			return call(determinantFn, mat([]float64{1, 2, 3}, []float64{4, 5, 6}))
		}},
		{"singular inverse", func() (interpreter.Value, error) {
			return call(inverseFn, mat([]float64{1, 2}, []float64{2, 4}))
		}},
		{"singular solve", func() (interpreter.Value, error) {
			return call(solveFn, mat([]float64{1, 2}, []float64{2, 4}), vec(1, 2))
		}},
		{"cross non-3", func() (interpreter.Value, error) { return call(crossFn, vec(1, 2), vec(3, 4)) }},
		{"identity zero", func() (interpreter.Value, error) { return call(identityFn, interpreter.IntVal(0)) }},
		{"matrix shape mismatch", func() (interpreter.Value, error) {
			return call(addFn, mat([]float64{1, 2}), mat([]float64{1, 2}, []float64{3, 4}))
		}},
		{"vector plus matrix", func() (interpreter.Value, error) {
			return call(addFn, vec(1, 2), mat([]float64{1, 2}))
		}},
		{"matvec dim mismatch", func() (interpreter.Value, error) {
			return call(matmulFn, mat([]float64{1, 2}), vec(1, 2, 3))
		}},
		{"trace non-square", func() (interpreter.Value, error) {
			return call(traceFn, mat([]float64{1, 2, 3}, []float64{4, 5, 6}))
		}},
		{"zeros non-positive", func() (interpreter.Value, error) {
			return call(zerosFn, interpreter.IntVal(0), interpreter.IntVal(3))
		}},
	}
	for _, c := range cases {
		if _, err := c.call(); err == nil {
			t.Errorf("%s: expected an error, got nil", c.name)
		}
	}
	// A singular matrix has determinant 0 (a valid, finite result), not an error.
	if v, err := call(determinantFn, mat([]float64{1, 2}, []float64{2, 4})); err != nil || v.Float != 0 {
		t.Errorf("determinant of singular = %v, %v; want 0, nil", v, err)
	}
}

// TestNonFiniteRejected pins the strict stance: a computation whose magnitudes
// overflow float64 (to +/-Inf, and thence NaN) is a catchable error, never a
// non-finite Value leaking into the type system.
func TestNonFiniteRejected(t *testing.T) {
	big := math.Pow(2, 1023) // doubling this overflows to +Inf
	if _, err := call(dotFn, vec(big, big), vec(big, big)); err == nil {
		t.Error("dot of overflowing vectors: expected an error, got nil")
	}
	if _, err := call(scaleFn, vec(big), interpreter.FloatVal(4)); err == nil {
		t.Error("scale past the float ceiling: expected an error, got nil")
	}
	if _, err := call(matmulFn, mat([]float64{big, big}), mat([]float64{big}, []float64{big})); err == nil {
		t.Error("matmul overflow: expected an error, got nil")
	}
}
