// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package mathlib

import (
	"math"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// call invokes a registered math builtin by name with float/int arguments,
// returning the value or failing the test on error.
func callOK(t *testing.T, fn interpreter.Builtin, args ...interpreter.Value) interpreter.Value {
	t.Helper()
	v, err := fn(interpreter.BuiltinCtx{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return v
}

// callErr asserts that a builtin rejects its arguments.
func callErr(t *testing.T, fn interpreter.Builtin, args ...interpreter.Value) {
	t.Helper()
	if _, err := fn(interpreter.BuiltinCtx{}, args); err == nil {
		t.Fatalf("expected an error, got none")
	}
}

func f(x float64) interpreter.Value { return interpreter.FloatVal(x) }
func i(n int64) interpreter.Value   { return interpreter.IntVal(n) }

func closeTo(got, want, tol float64) bool { return math.Abs(got-want) <= tol }

// TestElementaryValues spot-checks the trig / hyperbolic / exp / log surface
// against known values, and the strict domain rejections.
func TestElementaryValues(t *testing.T) {
	if v := callOK(t, oneFloat("sin", math.Sin), f(math.Pi/2)); !closeTo(v.Float, 1, 1e-12) {
		t.Errorf("sin(pi/2) = %v, want 1", v.Float)
	}
	if v := callOK(t, oneFloat("cos", math.Cos), f(0)); !closeTo(v.Float, 1, 1e-12) {
		t.Errorf("cos(0) = %v, want 1", v.Float)
	}
	if v := callOK(t, twoFloat("atan2", math.Atan2), f(1), f(1)); !closeTo(v.Float, math.Pi/4, 1e-12) {
		t.Errorf("atan2(1,1) = %v, want pi/4", v.Float)
	}
	if v := callOK(t, oneFloat("exp", math.Exp), f(0)); !closeTo(v.Float, 1, 1e-12) {
		t.Errorf("exp(0) = %v, want 1", v.Float)
	}
	if v := callOK(t, oneFloat("ln", math.Log), f(math.E)); !closeTo(v.Float, 1, 1e-12) {
		t.Errorf("ln(e) = %v, want 1", v.Float)
	}
	// log(x, base): log base 2 of 8 is 3.
	logFn := twoFloat("log", func(x, b float64) float64 { return math.Log(x) / math.Log(b) })
	if v := callOK(t, logFn, f(8), f(2)); !closeTo(v.Float, 3, 1e-12) {
		t.Errorf("log(8, 2) = %v, want 3", v.Float)
	}
	if v := callOK(t, twoFloat("hypot", math.Hypot), f(3), f(4)); !closeTo(v.Float, 5, 1e-12) {
		t.Errorf("hypot(3,4) = %v, want 5", v.Float)
	}

	// Domain rejections: asin(2), ln(0), ln(-1), acosh(0), atanh(1), exp(1e9).
	callErr(t, oneFloat("asin", math.Asin), f(2))
	callErr(t, oneFloat("ln", math.Log), f(0))
	callErr(t, oneFloat("ln", math.Log), f(-1))
	callErr(t, oneFloat("acosh", math.Acosh), f(0))
	callErr(t, oneFloat("atanh", math.Atanh), f(1))
	callErr(t, oneFloat("exp", math.Exp), f(1e9))
	// log base 1 is undefined (division by ln(1)=0).
	callErr(t, logFn, f(8), f(1))
}

// TestSignTrunc checks type-preserving sign and toward-zero trunc.
func TestSignTrunc(t *testing.T) {
	if v := callOK(t, signFn, i(-5)); v.Kind != interpreter.KindInt || v.Int != -1 {
		t.Errorf("sign(-5) = %v, want int -1", v)
	}
	if v := callOK(t, signFn, f(3.2)); v.Kind != interpreter.KindFloat || v.Float != 1 {
		t.Errorf("sign(3.2) = %v, want float 1", v)
	}
	if v := callOK(t, signFn, f(0)); v.Kind != interpreter.KindFloat || v.Float != 0 {
		t.Errorf("sign(0.0) = %v, want float 0", v)
	}
	if v := callOK(t, truncFn, f(-2.7)); v.Kind != interpreter.KindInt || v.Int != -2 {
		t.Errorf("trunc(-2.7) = %v, want int -2", v)
	}
	if v := callOK(t, truncFn, f(2.7)); v.Int != 2 {
		t.Errorf("trunc(2.7) = %v, want 2", v.Int)
	}
}

// TestCombinatorics checks factorial / comb / perm / gcd / lcm, including the
// overflow and boundary rejections.
func TestCombinatorics(t *testing.T) {
	if v := callOK(t, factorialFn, i(5)); v.Int != 120 {
		t.Errorf("factorial(5) = %d, want 120", v.Int)
	}
	if v := callOK(t, factorialFn, i(20)); v.Int != 2432902008176640000 {
		t.Errorf("factorial(20) = %d, want 2432902008176640000", v.Int)
	}
	callErr(t, factorialFn, i(21)) // overflows int64
	callErr(t, factorialFn, i(-1))

	if v := callOK(t, combFn, i(10), i(3)); v.Int != 120 {
		t.Errorf("comb(10,3) = %d, want 120", v.Int)
	}
	if v := callOK(t, combFn, i(52), i(5)); v.Int != 2598960 {
		t.Errorf("comb(52,5) = %d, want 2598960", v.Int)
	}
	if v := callOK(t, combFn, i(5), i(8)); v.Int != 0 { // k > n
		t.Errorf("comb(5,8) = %d, want 0", v.Int)
	}
	// Large but int64-representable central coefficients must compute, not
	// error on an intermediate overflow (the divide-before-multiply property:
	// the largest intermediate equals the final result).
	if v := callOK(t, combFn, i(62), i(31)); v.Int != 465428353255261088 {
		t.Errorf("comb(62,31) = %d, want 465428353255261088", v.Int)
	}
	if v := callOK(t, combFn, i(66), i(33)); v.Int != 7219428434016265740 {
		t.Errorf("comb(66,33) = %d, want 7219428434016265740", v.Int)
	}
	callErr(t, combFn, i(67), i(33)) // C(67,33) genuinely exceeds int64
	if v := callOK(t, permFn, i(10), i(3)); v.Int != 720 {
		t.Errorf("perm(10,3) = %d, want 720", v.Int)
	}
	if v := callOK(t, gcdFn, i(48), i(-36)); v.Int != 12 {
		t.Errorf("gcd(48,-36) = %d, want 12", v.Int)
	}
	if v := callOK(t, gcdFn, i(0), i(0)); v.Int != 0 {
		t.Errorf("gcd(0,0) = %d, want 0", v.Int)
	}
	if v := callOK(t, lcmFn, i(4), i(6)); v.Int != 12 {
		t.Errorf("lcm(4,6) = %d, want 12", v.Int)
	}
	if v := callOK(t, lcmFn, i(7), i(0)); v.Int != 0 {
		t.Errorf("lcm(7,0) = %d, want 0", v.Int)
	}
	// gcd(MinInt64, MinInt64) == 2^63 does not fit int64.
	callErr(t, gcdFn, i(math.MinInt64), i(math.MinInt64))
	callErr(t, lcmFn, i(math.MaxInt64), i(math.MaxInt64-1)) // overflow
}

// TestGammaBeta checks the gamma / beta surface against exact values.
func TestGammaBeta(t *testing.T) {
	// gamma(5) = 4! = 24; gamma(0.5) = sqrt(pi).
	if v := callOK(t, oneFloat("gamma", math.Gamma), f(5)); !closeTo(v.Float, 24, 1e-9) {
		t.Errorf("gamma(5) = %v, want 24", v.Float)
	}
	if v := callOK(t, oneFloat("gamma", math.Gamma), f(0.5)); !closeTo(v.Float, math.Sqrt(math.Pi), 1e-12) {
		t.Errorf("gamma(0.5) = %v, want sqrt(pi)", v.Float)
	}
	callErr(t, oneFloat("gamma", math.Gamma), f(0))  // pole
	callErr(t, oneFloat("gamma", math.Gamma), f(-2)) // pole

	// B(2,3) = 1!*2!/4! = 1/12.
	if v := callOK(t, betaFn, f(2), f(3)); !closeTo(v.Float, 1.0/12.0, 1e-12) {
		t.Errorf("beta(2,3) = %v, want 1/12", v.Float)
	}
	callErr(t, betaFn, f(0), f(3)) // non-positive
	// lbeta round-trips: exp(lbeta(a,b)) == beta(a,b).
	lb := callOK(t, lbetaFn, f(2.5), f(4.5))
	be := callOK(t, betaFn, f(2.5), f(4.5))
	if !closeTo(math.Exp(lb.Float), be.Float, 1e-12) {
		t.Errorf("exp(lbeta) = %v, beta = %v", math.Exp(lb.Float), be.Float)
	}
}

// TestRegularizedIncomplete pins the regularized incomplete gamma / beta - the
// distribution-CDF engine - against reference values, and checks the
// complement and symmetry identities.
func TestRegularizedIncomplete(t *testing.T) {
	// P(a,x): chi-square(k=2) CDF at x=2 is P(1, 1) = 1 - e^-1 = 0.6321...
	if v := callOK(t, regGammaPFn, f(1), f(1)); !closeTo(v.Float, 1-math.Exp(-1), 1e-10) {
		t.Errorf("regGammaP(1,1) = %v, want %v", v.Float, 1-math.Exp(-1))
	}
	// P + Q == 1.
	p := callOK(t, regGammaPFn, f(3.5), f(2.7))
	q := callOK(t, regGammaQFn, f(3.5), f(2.7))
	if !closeTo(p.Float+q.Float, 1, 1e-12) {
		t.Errorf("P + Q = %v, want 1", p.Float+q.Float)
	}
	// P(a, 0) == 0, and P grows to ~1 for large x.
	if v := callOK(t, regGammaPFn, f(2), f(0)); v.Float != 0 {
		t.Errorf("regGammaP(2,0) = %v, want 0", v.Float)
	}
	if v := callOK(t, regGammaPFn, f(2), f(100)); !closeTo(v.Float, 1, 1e-9) {
		t.Errorf("regGammaP(2,100) = %v, want ~1", v.Float)
	}
	callErr(t, regGammaPFn, f(0), f(1))  // a must be > 0
	callErr(t, regGammaPFn, f(1), f(-1)) // x must be >= 0

	// I_x(a,b): I_0.5(2,2) == 0.5 by symmetry; more precisely I_0.5(2,2)=0.5.
	if v := callOK(t, regBetaIFn, f(0.5), f(2), f(2)); !closeTo(v.Float, 0.5, 1e-12) {
		t.Errorf("regBetaI(0.5,2,2) = %v, want 0.5", v.Float)
	}
	// Symmetry: I_x(a,b) == 1 - I_{1-x}(b,a).
	ix := callOK(t, regBetaIFn, f(0.3), f(2.5), f(4.5))
	comp := callOK(t, regBetaIFn, f(0.7), f(4.5), f(2.5))
	if !closeTo(ix.Float, 1-comp.Float, 1e-12) {
		t.Errorf("I_0.3(2.5,4.5) = %v, want %v", ix.Float, 1-comp.Float)
	}
	// Endpoints.
	if v := callOK(t, regBetaIFn, f(0), f(2), f(3)); v.Float != 0 {
		t.Errorf("regBetaI(0,..) = %v, want 0", v.Float)
	}
	if v := callOK(t, regBetaIFn, f(1), f(2), f(3)); v.Float != 1 {
		t.Errorf("regBetaI(1,..) = %v, want 1", v.Float)
	}
	callErr(t, regBetaIFn, f(1.5), f(2), f(3)) // x out of [0,1]
	callErr(t, regBetaIFn, f(0.5), f(0), f(3)) // a must be > 0
}

// TestStrictCDFContract pins the DF-math1 audit fixes: the three hand-rolled
// CDF builtins and lbeta must honor math's strict contract - an extreme-but-
// legal argument that drives the internal series / continued fraction to a
// non-finite or non-converged result is a catchable error, never a leaked NaN.
func TestStrictCDFContract(t *testing.T) {
	big := 1e308 // reachable via math.pow(10, 308); no NaN literal needed

	// F1: the incomplete gamma / beta builtins no longer leak NaN.
	callErr(t, regGammaPFn, f(big), f(big))
	callErr(t, regGammaQFn, f(big), f(big))
	callErr(t, regBetaIFn, f(0.5), f(big), f(1))
	// F2: lbeta gains the guard its sibling beta already had.
	callErr(t, lbetaFn, f(big), f(big))

	// F4 + contract: every finite CDF result stays within [0, 1] by definition.
	for _, tc := range []struct{ a, x float64 }{{1, 1}, {2, 0.5}, {3.5, 2.7}, {0.5, 10}, {50, 40}} {
		v := callOK(t, regGammaPFn, f(tc.a), f(tc.x))
		if v.Float < 0 || v.Float > 1 {
			t.Errorf("regGammaP(%v,%v) = %v, outside [0,1]", tc.a, tc.x, v.Float)
		}
	}
	for _, tc := range []struct{ x, a, b float64 }{{0.3, 2, 3}, {0.5, 5, 5}, {0.99, 2, 8}, {0.01, 8, 2}} {
		v := callOK(t, regBetaIFn, f(tc.x), f(tc.a), f(tc.b))
		if v.Float < 0 || v.Float > 1 {
			t.Errorf("regBetaI(%v,%v,%v) = %v, outside [0,1]", tc.x, tc.a, tc.b, v.Float)
		}
	}
}

// TestErf checks the error function against known values.
func TestErf(t *testing.T) {
	if v := callOK(t, oneFloat("erf", math.Erf), f(0)); v.Float != 0 {
		t.Errorf("erf(0) = %v, want 0", v.Float)
	}
	// erf(1) = 0.8427007929...
	if v := callOK(t, oneFloat("erf", math.Erf), f(1)); !closeTo(v.Float, 0.8427007929497149, 1e-12) {
		t.Errorf("erf(1) = %v", v.Float)
	}
	// erf(x) + erfc(x) == 1.
	e := callOK(t, oneFloat("erf", math.Erf), f(0.7))
	ec := callOK(t, oneFloat("erfc", math.Erfc), f(0.7))
	if !closeTo(e.Float+ec.Float, 1, 1e-12) {
		t.Errorf("erf + erfc = %v, want 1", e.Float+ec.Float)
	}
}
