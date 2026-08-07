// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Probability distributions for the `stats` library: the pdf / pmf, cdf, and
// quantile (inverse cdf) of the distributions statistical inference is built on
// - normal, Student's t, chi-square, F, binomial, Poisson. The continuous CDFs
// reduce to the regularized incomplete gamma / beta exposed from `math`;
// the quantiles invert them numerically (Acklam's rational
// approximation for the normal, bracketed bisection for the rest). Sampling
// draws on `math`'s shared random source. Strict, like the rest of `stats`: an
// out-of-domain parameter or a probability outside [0, 1] is a catchable error,
// never a NaN.
package statslib

import (
	"fmt"
	"math"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	mathlib "jennifer-lang.dev/jennifer/internal/lib/math"
)

// clamp01 pins a value to [0, 1] - a CDF is in [0, 1] by definition, so the
// last-ULP excursions the regularized incomplete functions can produce at the
// extreme edges are corrected here.
func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// scalars reads exactly `want` numeric (int or float) scalar arguments, erroring
// on a wrong count or a non-numeric operand.
func scalars(name string, args []interpreter.Value, want int) ([]float64, error) {
	if len(args) != want {
		return nil, fmt.Errorf("stats.%s expects %d arguments, got %d", name, want, len(args))
	}
	fs := make([]float64, want)
	for i, a := range args {
		f, ok := a.AsFloat()
		if !ok {
			return nil, fmt.Errorf("stats.%s: argument %d must be int or float, got %s", name, i+1, a.Kind)
		}
		fs[i] = f
	}
	return fs, nil
}

// cdfResult wraps a CDF value in the strict contract: a non-converged internal
// series / continued fraction, or a non-finite value, is a catchable error; a
// finite value is clamped into [0, 1].
func cdfResult(name string, v float64, ok bool) (interpreter.Value, error) {
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.%s: did not converge for the given parameters", name)
	}
	if !isFinite(v) {
		return interpreter.Null(), fmt.Errorf("stats.%s: result is undefined or infinite", name)
	}
	return interpreter.FloatVal(clamp01(v)), nil
}

// --- standard-normal building blocks ---

// stdNormalCdf is Phi(z) = 0.5*erfc(-z/sqrt2).
func stdNormalCdf(z float64) float64 { return 0.5 * math.Erfc(-z/math.Sqrt2) }

// stdNormalInv is the inverse standard-normal CDF (the probit) by Acklam's
// rational approximation plus one Halley refinement, accurate to full double
// precision on (0, 1). p must be in (0, 1) (checked by the caller).
func stdNormalInv(p float64) float64 {
	a := [...]float64{-3.969683028665376e+01, 2.209460984245205e+02, -2.759285104469687e+02, 1.383577518672690e+02, -3.066479806614716e+01, 2.506628277459239e+00}
	b := [...]float64{-5.447609879822406e+01, 1.615858368580409e+02, -1.556989798598866e+02, 6.680131188771972e+01, -1.328068155288572e+01}
	c := [...]float64{-7.784894002430293e-03, -3.223964580411365e-01, -2.400758277161838e+00, -2.549732539343734e+00, 4.374664141464968e+00, 2.938163982698783e+00}
	d := [...]float64{7.784695709041462e-03, 3.224671290700398e-01, 2.445134137142996e+00, 3.754408661907416e+00}
	const plow = 0.02425
	const phigh = 1 - plow
	var x float64
	switch {
	case p < plow:
		q := math.Sqrt(-2 * math.Log(p))
		x = (((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) / ((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	case p <= phigh:
		q := p - 0.5
		r := q * q
		x = (((((a[0]*r+a[1])*r+a[2])*r+a[3])*r+a[4])*r + a[5]) * q / (((((b[0]*r+b[1])*r+b[2])*r+b[3])*r+b[4])*r + 1)
	default:
		q := math.Sqrt(-2 * math.Log(1-p))
		x = -(((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) / ((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	}
	// One Halley step against the true CDF for full precision.
	e := stdNormalCdf(x) - p
	u := e * math.Sqrt(2*math.Pi) * math.Exp(x*x/2)
	return x - u/(1+x*u/2)
}

// bisectCdf inverts a monotone-increasing CDF for the probability p, over the
// bracket [lo, hi] on which cdf(lo) <= p <= cdf(hi). 200 halving steps take the
// bracket well below double precision. The cdf callback returns a convergence
// flag; a false aborts the inversion.
func bisectCdf(cdf func(float64) (float64, bool), p, lo, hi float64) (float64, bool) {
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		v, ok := cdf(mid)
		if !ok {
			return 0, false
		}
		if v < p {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2, true
}

// bracketPositive finds an upper bound hi >= 0 with cdf(hi) >= p for a CDF
// supported on [0, inf), doubling from 1. It gives up past 1e300 (returns
// false), which only a pathological p (>= 1) or a broken cdf reaches.
func bracketPositive(cdf func(float64) (float64, bool), p float64) (float64, bool) {
	hi := 1.0
	for hi < 1e300 {
		v, ok := cdf(hi)
		if !ok {
			return 0, false
		}
		if v >= p {
			return hi, true
		}
		hi *= 2
	}
	return 0, false
}

// checkProb rejects a quantile probability outside the open interval (0, 1) -
// the boundaries map to +/-infinity for an unbounded support.
func checkProb(name string, p float64) error {
	if p <= 0 || p >= 1 {
		return fmt.Errorf("stats.%s: probability must be in (0, 1), got %s", name, interpreter.DisplayFloat(p))
	}
	return nil
}

// --- Normal ---

func normalPdfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	a, err := scalars("normalPdf", args, 3)
	if err != nil {
		return interpreter.Null(), err
	}
	x, mean, sd := a[0], a[1], a[2]
	if sd <= 0 {
		return interpreter.Null(), fmt.Errorf("stats.normalPdf: sd must be positive, got %s", interpreter.DisplayFloat(sd))
	}
	z := (x - mean) / sd
	return finiteResult("normalPdf", math.Exp(-0.5*z*z)/(sd*math.Sqrt(2*math.Pi)))
}

func normalCdfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	a, err := scalars("normalCdf", args, 3)
	if err != nil {
		return interpreter.Null(), err
	}
	x, mean, sd := a[0], a[1], a[2]
	if sd <= 0 {
		return interpreter.Null(), fmt.Errorf("stats.normalCdf: sd must be positive, got %s", interpreter.DisplayFloat(sd))
	}
	return cdfResult("normalCdf", stdNormalCdf((x-mean)/sd), true)
}

func normalQuantileFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	a, err := scalars("normalQuantile", args, 3)
	if err != nil {
		return interpreter.Null(), err
	}
	p, mean, sd := a[0], a[1], a[2]
	if sd <= 0 {
		return interpreter.Null(), fmt.Errorf("stats.normalQuantile: sd must be positive, got %s", interpreter.DisplayFloat(sd))
	}
	if err := checkProb("normalQuantile", p); err != nil {
		return interpreter.Null(), err
	}
	return finiteResult("normalQuantile", mean+sd*stdNormalInv(p))
}

func normalSampleFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	a, err := scalars("normalSample", args, 2)
	if err != nil {
		return interpreter.Null(), err
	}
	mean, sd := a[0], a[1]
	if sd <= 0 {
		return interpreter.Null(), fmt.Errorf("stats.normalSample: sd must be positive, got %s", interpreter.DisplayFloat(sd))
	}
	// Box-Muller: one standard-normal draw from two uniforms (u1 in (0, 1]).
	u1 := 1 - mathlib.SharedFloat64()
	u2 := mathlib.SharedFloat64()
	z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
	return finiteResult("normalSample", mean+sd*z)
}

// --- Student's t ---

// tCdfStd returns the Student-t CDF at t with df degrees of freedom, plus a
// convergence flag from the incomplete beta.
func tCdfStd(t, df float64) (float64, bool) {
	x := df / (df + t*t)
	ib, ok := mathlib.RegularizedIncBeta(x, df/2, 0.5)
	// The exported incomplete beta returns the raw value; fold a finiteness
	// check into the ok flag so a non-finite result (extreme parameters) can
	// never leak into a CDF or a downstream p-value.
	if !ok || !isFinite(ib) {
		return 0, false
	}
	if t <= 0 {
		return 0.5 * ib, true
	}
	return 1 - 0.5*ib, true
}

func tPdfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	a, err := scalars("tPdf", args, 2)
	if err != nil {
		return interpreter.Null(), err
	}
	t, df := a[0], a[1]
	if df <= 0 {
		return interpreter.Null(), fmt.Errorf("stats.tPdf: df must be positive, got %s", interpreter.DisplayFloat(df))
	}
	logp := mathlib.LgammaVal((df+1)/2) - mathlib.LgammaVal(df/2) - 0.5*math.Log(df*math.Pi) - (df+1)/2*math.Log1p(t*t/df)
	return finiteResult("tPdf", math.Exp(logp))
}

func tCdfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	a, err := scalars("tCdf", args, 2)
	if err != nil {
		return interpreter.Null(), err
	}
	t, df := a[0], a[1]
	if df <= 0 {
		return interpreter.Null(), fmt.Errorf("stats.tCdf: df must be positive, got %s", interpreter.DisplayFloat(df))
	}
	v, ok := tCdfStd(t, df)
	return cdfResult("tCdf", v, ok)
}

func tQuantileFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	a, err := scalars("tQuantile", args, 2)
	if err != nil {
		return interpreter.Null(), err
	}
	p, df := a[0], a[1]
	if df <= 0 {
		return interpreter.Null(), fmt.Errorf("stats.tQuantile: df must be positive, got %s", interpreter.DisplayFloat(df))
	}
	if err := checkProb("tQuantile", p); err != nil {
		return interpreter.Null(), err
	}
	v, ok := tQuantile(p, df)
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.tQuantile: did not converge for the given parameters")
	}
	return finiteResult("tQuantile", v)
}

// tQuantile inverts the t CDF using its symmetry about 0: solve on [0, hi) for
// p >= 0.5 and mirror otherwise.
func tQuantile(p, df float64) (float64, bool) {
	if p == 0.5 {
		return 0, true
	}
	cdf := func(t float64) (float64, bool) { return tCdfStd(t, df) }
	if p < 0.5 {
		hi, ok := bracketPositive(func(t float64) (float64, bool) {
			v, ok := cdf(-t)
			return 1 - v, ok
		}, 1-p)
		if !ok {
			return 0, false
		}
		r, ok := bisectCdf(func(t float64) (float64, bool) {
			v, ok := cdf(-t)
			return 1 - v, ok
		}, 1-p, 0, hi)
		return -r, ok
	}
	hi, ok := bracketPositive(cdf, p)
	if !ok {
		return 0, false
	}
	return bisectCdf(cdf, p, 0, hi)
}

// --- Chi-square ---

// chiSquareCdfStd returns the chi-square CDF at x with df degrees of freedom.
func chiSquareCdfStd(x, df float64) (float64, bool) {
	if x <= 0 {
		return 0, true
	}
	v, ok := mathlib.RegularizedGammaP(df/2, x/2)
	return v, ok && isFinite(v)
}

func chiSquareCdfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	a, err := scalars("chiSquareCdf", args, 2)
	if err != nil {
		return interpreter.Null(), err
	}
	x, df := a[0], a[1]
	if df <= 0 {
		return interpreter.Null(), fmt.Errorf("stats.chiSquareCdf: df must be positive, got %s", interpreter.DisplayFloat(df))
	}
	if x < 0 {
		return interpreter.Null(), fmt.Errorf("stats.chiSquareCdf: x must be non-negative, got %s", interpreter.DisplayFloat(x))
	}
	v, ok := chiSquareCdfStd(x, df)
	return cdfResult("chiSquareCdf", v, ok)
}

func chiSquareQuantileFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	a, err := scalars("chiSquareQuantile", args, 2)
	if err != nil {
		return interpreter.Null(), err
	}
	p, df := a[0], a[1]
	if df <= 0 {
		return interpreter.Null(), fmt.Errorf("stats.chiSquareQuantile: df must be positive, got %s", interpreter.DisplayFloat(df))
	}
	if err := checkProb("chiSquareQuantile", p); err != nil {
		return interpreter.Null(), err
	}
	cdf := func(x float64) (float64, bool) { return chiSquareCdfStd(x, df) }
	hi, ok := bracketPositive(cdf, p)
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.chiSquareQuantile: did not converge for the given parameters")
	}
	v, ok := bisectCdf(cdf, p, 0, hi)
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.chiSquareQuantile: did not converge for the given parameters")
	}
	return finiteResult("chiSquareQuantile", v)
}

// --- F ---

// fCdfStd returns the F CDF at x with (d1, d2) degrees of freedom.
func fCdfStd(x, d1, d2 float64) (float64, bool) {
	if x <= 0 {
		return 0, true
	}
	v, ok := mathlib.RegularizedIncBeta(d1*x/(d1*x+d2), d1/2, d2/2)
	return v, ok && isFinite(v)
}

func fCdfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	a, err := scalars("fCdf", args, 3)
	if err != nil {
		return interpreter.Null(), err
	}
	x, d1, d2 := a[0], a[1], a[2]
	if d1 <= 0 || d2 <= 0 {
		return interpreter.Null(), fmt.Errorf("stats.fCdf: degrees of freedom must be positive, got %s and %s", interpreter.DisplayFloat(d1), interpreter.DisplayFloat(d2))
	}
	if x < 0 {
		return interpreter.Null(), fmt.Errorf("stats.fCdf: x must be non-negative, got %s", interpreter.DisplayFloat(x))
	}
	v, ok := fCdfStd(x, d1, d2)
	return cdfResult("fCdf", v, ok)
}

func fQuantileFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	a, err := scalars("fQuantile", args, 3)
	if err != nil {
		return interpreter.Null(), err
	}
	p, d1, d2 := a[0], a[1], a[2]
	if d1 <= 0 || d2 <= 0 {
		return interpreter.Null(), fmt.Errorf("stats.fQuantile: degrees of freedom must be positive, got %s and %s", interpreter.DisplayFloat(d1), interpreter.DisplayFloat(d2))
	}
	if err := checkProb("fQuantile", p); err != nil {
		return interpreter.Null(), err
	}
	cdf := func(x float64) (float64, bool) { return fCdfStd(x, d1, d2) }
	hi, ok := bracketPositive(cdf, p)
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.fQuantile: did not converge for the given parameters")
	}
	v, ok := bisectCdf(cdf, p, 0, hi)
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.fQuantile: did not converge for the given parameters")
	}
	return finiteResult("fQuantile", v)
}

// --- Binomial ---

// logChoose returns ln C(n, k) via lgamma (no overflow for large n).
func logChoose(n, k float64) float64 {
	return mathlib.LgammaVal(n+1) - mathlib.LgammaVal(k+1) - mathlib.LgammaVal(n-k+1)
}

func binomialPmfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	k, n, p, err := binomialArgs("binomialPmf", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if k < 0 || k > n {
		return interpreter.FloatVal(0), nil
	}
	switch p {
	case 0:
		return boolFloat(k == 0), nil
	case 1:
		return boolFloat(k == n), nil
	}
	logp := logChoose(float64(n), float64(k)) + float64(k)*math.Log(p) + float64(n-k)*math.Log(1-p)
	return finiteResult("binomialPmf", math.Exp(logp))
}

func binomialCdfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	k, n, p, err := binomialArgs("binomialCdf", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if k < 0 {
		return interpreter.FloatVal(0), nil
	}
	if k >= n {
		return interpreter.FloatVal(1), nil
	}
	// P(X <= k) = I_{1-p}(n-k, k+1).
	v, ok := mathlib.RegularizedIncBeta(1-p, float64(n-k), float64(k+1))
	return cdfResult("binomialCdf", v, ok)
}

// binomialArgs reads (k int, n int, p float) for the binomial builtins,
// validating n >= 0 and 0 <= p <= 1.
func binomialArgs(name string, args []interpreter.Value) (k, n int64, p float64, err error) {
	if len(args) != 3 {
		return 0, 0, 0, fmt.Errorf("stats.%s expects 3 arguments (k, n, p), got %d", name, len(args))
	}
	if args[0].Kind != interpreter.KindInt || args[1].Kind != interpreter.KindInt {
		return 0, 0, 0, fmt.Errorf("stats.%s: k and n must be ints", name)
	}
	pf, ok := args[2].AsFloat()
	if !ok {
		return 0, 0, 0, fmt.Errorf("stats.%s: p must be int or float, got %s", name, args[2].Kind)
	}
	k, n = args[0].Int, args[1].Int
	if n < 0 {
		return 0, 0, 0, fmt.Errorf("stats.%s: n must be non-negative, got %d", name, n)
	}
	if pf < 0 || pf > 1 {
		return 0, 0, 0, fmt.Errorf("stats.%s: p must be in [0, 1], got %s", name, interpreter.DisplayFloat(pf))
	}
	return k, n, pf, nil
}

// --- Poisson ---

func poissonPmfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	k, lambda, err := poissonArgs("poissonPmf", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if k < 0 {
		return interpreter.FloatVal(0), nil
	}
	logp := -lambda + float64(k)*math.Log(lambda) - mathlib.LgammaVal(float64(k)+1)
	return finiteResult("poissonPmf", math.Exp(logp))
}

func poissonCdfFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	k, lambda, err := poissonArgs("poissonCdf", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if k < 0 {
		return interpreter.FloatVal(0), nil
	}
	// P(X <= k) = Q(k+1, lambda) = 1 - P(k+1, lambda).
	p, ok := mathlib.RegularizedGammaP(float64(k)+1, lambda)
	return cdfResult("poissonCdf", 1-p, ok)
}

// poissonArgs reads (k int, lambda float) with lambda > 0.
func poissonArgs(name string, args []interpreter.Value) (k int64, lambda float64, err error) {
	if len(args) != 2 {
		return 0, 0, fmt.Errorf("stats.%s expects 2 arguments (k, lambda), got %d", name, len(args))
	}
	if args[0].Kind != interpreter.KindInt {
		return 0, 0, fmt.Errorf("stats.%s: k must be an int", name)
	}
	lf, ok := args[1].AsFloat()
	if !ok {
		return 0, 0, fmt.Errorf("stats.%s: lambda must be int or float, got %s", name, args[1].Kind)
	}
	if lf <= 0 {
		return 0, 0, fmt.Errorf("stats.%s: lambda must be positive, got %s", name, interpreter.DisplayFloat(lf))
	}
	return args[0].Int, lf, nil
}

// boolFloat maps a bool to the float 1.0 / 0.0 (a degenerate pmf spike).
func boolFloat(b bool) interpreter.Value {
	if b {
		return interpreter.FloatVal(1)
	}
	return interpreter.FloatVal(0)
}

// betaQuantile inverts the regularized incomplete beta I_x(a,b) = p on [0, 1] -
// the engine behind the Clopper-Pearson proportion interval. p in [0, 1].
func betaQuantile(p, a, b float64) (float64, bool) {
	if p <= 0 {
		return 0, true
	}
	if p >= 1 {
		return 1, true
	}
	return bisectCdf(func(x float64) (float64, bool) { return mathlib.RegularizedIncBeta(x, a, b) }, p, 0, 1)
}
