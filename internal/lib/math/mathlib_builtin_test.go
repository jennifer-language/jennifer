// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package mathlib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// Exercise the builtin wrappers (min / max / sqrt / pow / rand / randInt /
// randSeed) and the shared random sources, which the value-focused tests skip.

func fl(x float64) interpreter.Value { return interpreter.FloatVal(x) }
func iv(x int64) interpreter.Value   { return interpreter.IntVal(x) }
func ctx() interpreter.BuiltinCtx    { return interpreter.BuiltinCtx{} }

func TestMinMaxBuiltins(t *testing.T) {
	// int/int keeps the int kind (fast path).
	if v, _ := minFn(ctx(), []interpreter.Value{iv(3), iv(5)}); v.Kind != interpreter.KindInt || v.Int != 3 {
		t.Errorf("min(3,5) = %+v, want int 3", v)
	}
	if v, _ := maxFn(ctx(), []interpreter.Value{iv(3), iv(5)}); v.Int != 5 {
		t.Errorf("max(3,5) = %+v, want 5", v)
	}
	// mixed int/float promotes to float.
	if v, _ := maxFn(ctx(), []interpreter.Value{iv(3), fl(2.5)}); v.Kind != interpreter.KindFloat || v.Float != 3.0 {
		t.Errorf("max(3, 2.5) = %+v, want float 3.0", v)
	}
	if v, _ := minFn(ctx(), []interpreter.Value{fl(2.5), iv(3)}); v.Float != 2.5 {
		t.Errorf("min(2.5, 3) = %+v, want 2.5", v)
	}
	// errors: non-numeric operand, wrong arity.
	if _, err := minFn(ctx(), []interpreter.Value{interpreter.StringVal("x"), iv(1)}); err == nil {
		t.Error("min(string, int) should error")
	}
	if _, err := maxFn(ctx(), []interpreter.Value{iv(1)}); err == nil {
		t.Error("max arity error expected")
	}
}

func TestSqrtBuiltin(t *testing.T) {
	if v, _ := sqrtFn(ctx(), []interpreter.Value{fl(9.0)}); v.Kind != interpreter.KindFloat || v.Float != 3.0 {
		t.Errorf("sqrt(9) = %+v, want 3.0", v)
	}
	if v, _ := sqrtFn(ctx(), []interpreter.Value{iv(16)}); v.Float != 4.0 { // int accepted
		t.Errorf("sqrt(16) = %+v, want 4.0", v)
	}
	if _, err := sqrtFn(ctx(), []interpreter.Value{fl(-1.0)}); err == nil {
		t.Error("sqrt(-1) should error (undefined)")
	}
	if _, err := sqrtFn(ctx(), []interpreter.Value{interpreter.StringVal("x")}); err == nil {
		t.Error("sqrt(string) should error")
	}
	if _, err := sqrtFn(ctx(), nil); err == nil {
		t.Error("sqrt arity error expected")
	}
}

func TestPowBuiltin(t *testing.T) {
	if v, _ := powFn(ctx(), []interpreter.Value{fl(2.0), fl(10.0)}); v.Float != 1024.0 {
		t.Errorf("pow(2,10) = %+v, want 1024", v)
	}
	if v, _ := powFn(ctx(), []interpreter.Value{iv(2), iv(3)}); v.Float != 8.0 {
		t.Errorf("pow(2,3) = %+v, want 8", v)
	}
	// A result that overflows to infinity is rejected (strict).
	if _, err := powFn(ctx(), []interpreter.Value{fl(10.0), fl(400.0)}); err == nil {
		t.Error("pow(10, 400) should error (infinite)")
	}
	if _, err := powFn(ctx(), []interpreter.Value{interpreter.StringVal("x"), iv(2)}); err == nil {
		t.Error("pow(string, int) should error")
	}
	if _, err := powFn(ctx(), []interpreter.Value{fl(2.0)}); err == nil {
		t.Error("pow arity error expected")
	}
}

func TestRandSeededDeterminism(t *testing.T) {
	seq := func() []float64 {
		if _, err := randSeedFn(ctx(), []interpreter.Value{iv(42)}); err != nil {
			t.Fatalf("randSeed: %v", err)
		}
		out := make([]float64, 4)
		for k := range out {
			v, err := randFn(ctx(), nil)
			if err != nil {
				t.Fatalf("rand: %v", err)
			}
			if v.Kind != interpreter.KindFloat || v.Float < 0 || v.Float >= 1 {
				t.Fatalf("rand() = %+v, want a float in [0,1)", v)
			}
			out[k] = v.Float
		}
		return out
	}
	a, b := seq(), seq()
	for k := range a {
		if a[k] != b[k] {
			t.Errorf("seeded rand not reproducible at %d: %v vs %v", k, a[k], b[k])
		}
	}
	// rand takes no arguments; randSeed needs an int.
	if _, err := randFn(ctx(), []interpreter.Value{iv(1)}); err == nil {
		t.Error("rand(arg) should error")
	}
	if _, err := randSeedFn(ctx(), []interpreter.Value{fl(1.0)}); err == nil {
		t.Error("randSeed(float) should error")
	}
	if _, err := randSeedFn(ctx(), nil); err == nil {
		t.Error("randSeed arity error expected")
	}
}

func TestRandIntBuiltin(t *testing.T) {
	if _, err := randSeedFn(ctx(), []interpreter.Value{iv(7)}); err != nil {
		t.Fatalf("randSeed: %v", err)
	}
	for k := 0; k < 200; k++ {
		v, err := randIntFn(ctx(), []interpreter.Value{iv(1), iv(6)})
		if err != nil {
			t.Fatalf("randInt: %v", err)
		}
		if v.Kind != interpreter.KindInt || v.Int < 1 || v.Int > 6 {
			t.Fatalf("randInt(1,6) = %+v, out of [1,6]", v)
		}
	}
	// lo == hi yields that value.
	if v, _ := randIntFn(ctx(), []interpreter.Value{iv(5), iv(5)}); v.Int != 5 {
		t.Errorf("randInt(5,5) = %d, want 5", v.Int)
	}
	// errors: lo > hi, non-int operand, arity.
	if _, err := randIntFn(ctx(), []interpreter.Value{iv(6), iv(1)}); err == nil {
		t.Error("randInt(6,1) should error (lo > hi)")
	}
	if _, err := randIntFn(ctx(), []interpreter.Value{fl(1.0), iv(6)}); err == nil {
		t.Error("randInt(float, int) should error")
	}
	if _, err := randIntFn(ctx(), []interpreter.Value{iv(1)}); err == nil {
		t.Error("randInt arity error expected")
	}
}

func TestInstallAndSpecialBuiltins(t *testing.T) {
	in := interpreter.New()
	Install(in) // covers Install / installElementary / installSpecial
	for _, name := range []string{"sin", "cos", "exp", "ln", "gamma", "lgamma", "beta", "erf", "abs", "min", "max", "sqrt", "pow", "rand", "randInt", "randSeed"} {
		if in.LookupNamespacedBuiltin("math", name) == nil {
			t.Errorf("math.%s is not registered", name)
		}
	}
	// lgamma builtin: ln(gamma(5)) = ln(24) ~ 3.178.
	v, err := lgammaFn(ctx(), []interpreter.Value{fl(5.0)})
	if err != nil {
		t.Fatalf("lgamma(5): %v", err)
	}
	if v.Kind != interpreter.KindFloat || v.Float < 3.17 || v.Float > 3.18 {
		t.Errorf("lgamma(5) = %v, want ~3.178", v.Float)
	}
	// The exported special functions the stats distributions draw on.
	if p, ok := RegularizedGammaP(2.0, 2.0); !ok || p <= 0 || p >= 1 {
		t.Errorf("RegularizedGammaP(2,2) = %v ok=%v, want a probability in (0,1)", p, ok)
	}
	if b, ok := RegularizedIncBeta(0.5, 2.0, 2.0); !ok || b <= 0 || b >= 1 {
		t.Errorf("RegularizedIncBeta(0.5,2,2) = %v ok=%v", b, ok)
	}
	if lg := LgammaVal(5.0); lg < 3.17 || lg > 3.18 {
		t.Errorf("LgammaVal(5) = %v, want ~3.178", lg)
	}
}

func TestSharedRandomSources(t *testing.T) {
	if _, err := randSeedFn(ctx(), []interpreter.Value{iv(1)}); err != nil {
		t.Fatalf("randSeed: %v", err)
	}
	if x := SharedFloat64(); x < 0 || x >= 1 {
		t.Errorf("SharedFloat64() = %v, want [0,1)", x)
	}
	if n := SharedIntN(10); n < 0 || n >= 10 {
		t.Errorf("SharedIntN(10) = %d, want [0,10)", n)
	}
}
