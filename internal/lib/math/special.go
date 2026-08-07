// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package mathlib

import (
	"fmt"
	"math"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// installSpecial registers the special functions the probability layer is
// built on: the error function, the gamma / beta functions and their logs, and
// the regularized incomplete gamma / beta - the engine behind every
// distribution CDF. Go's stdlib supplies erf / gamma / lgamma; the regularized
// incomplete forms are hand-rolled by the standard series / continued-fraction
// algorithms below.
func installSpecial(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "erf", oneFloat("erf", math.Erf))
	in.RegisterNamespaced(LibraryName, "erfc", oneFloat("erfc", math.Erfc))
	// gamma has poles at 0 and the negative integers (NaN / Inf there) and
	// overflows for large x; the strict check in oneFloat rejects both.
	in.RegisterNamespaced(LibraryName, "gamma", oneFloat("gamma", math.Gamma))
	in.RegisterNamespaced(LibraryName, "lgamma", lgammaFn)

	in.RegisterNamespaced(LibraryName, "beta", betaFn)
	in.RegisterNamespaced(LibraryName, "lbeta", lbetaFn)

	in.RegisterNamespaced(LibraryName, "regGammaP", regGammaPFn)
	in.RegisterNamespaced(LibraryName, "regGammaQ", regGammaQFn)
	in.RegisterNamespaced(LibraryName, "regBetaI", regBetaIFn)
}

// lgammaVal is the natural log of |gamma(x)|, dropping Go's sign return.
func lgammaVal(x float64) float64 {
	v, _ := math.Lgamma(x)
	return v
}

// lgammaFn returns ln|gamma(x)|. At the poles (x a non-positive integer) the
// value is +Inf, which the strict check rejects.
func lgammaFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("lgamma", args); err != nil {
		return interpreter.Null(), err
	}
	x, ok := args[0].AsFloat()
	if !ok {
		return interpreter.Null(), fmt.Errorf("lgamma(): requires int or float, got %s", args[0].Kind)
	}
	r := lgammaVal(x)
	if math.IsNaN(r) || math.IsInf(r, 0) {
		return interpreter.Null(), fmt.Errorf("lgamma(%s): result is undefined or infinite", interpreter.DisplayFloat(x))
	}
	return interpreter.FloatVal(r), nil
}

// twoPositiveFloats reads two positive float/int arguments for beta / lbeta,
// erroring on a wrong count, a non-numeric operand, or a non-positive value.
func twoPositiveFloats(name string, args []interpreter.Value) (a, b float64, err error) {
	if len(args) != 2 {
		return 0, 0, fmt.Errorf("%s expects 2 arguments, got %d", name, len(args))
	}
	var aok, bok bool
	a, aok = args[0].AsFloat()
	b, bok = args[1].AsFloat()
	if !aok || !bok {
		return 0, 0, fmt.Errorf("%s(): requires numeric operands, got %s and %s", name, args[0].Kind, args[1].Kind)
	}
	if a <= 0 || b <= 0 {
		return 0, 0, fmt.Errorf("%s(%s, %s): requires positive arguments", name, interpreter.DisplayFloat(a), interpreter.DisplayFloat(b))
	}
	return a, b, nil
}

// betaFn returns the beta function B(a, b) = gamma(a)*gamma(b)/gamma(a+b) for
// positive a, b, computed through the logs for range. An overflow errors.
func betaFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	a, b, err := twoPositiveFloats("beta", args)
	if err != nil {
		return interpreter.Null(), err
	}
	r := math.Exp(lgammaVal(a) + lgammaVal(b) - lgammaVal(a+b))
	if math.IsNaN(r) || math.IsInf(r, 0) {
		return interpreter.Null(), fmt.Errorf("beta(%s, %s): result is undefined or infinite", interpreter.DisplayFloat(a), interpreter.DisplayFloat(b))
	}
	return interpreter.FloatVal(r), nil
}

// lbetaFn returns ln B(a, b) for positive a, b - the numerically-stable log
// of the beta function.
func lbetaFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	a, b, err := twoPositiveFloats("lbeta", args)
	if err != nil {
		return interpreter.Null(), err
	}
	r := lgammaVal(a) + lgammaVal(b) - lgammaVal(a+b)
	if math.IsNaN(r) || math.IsInf(r, 0) {
		return interpreter.Null(), fmt.Errorf("lbeta(%s, %s): result is undefined or infinite", interpreter.DisplayFloat(a), interpreter.DisplayFloat(b))
	}
	return interpreter.FloatVal(r), nil
}

// Iteration limits for the hand-rolled series / continued fractions. EPS is
// near float64 relative precision; FPMIN guards a divide-by-tiny in the
// modified Lentz continued-fraction evaluation.
const (
	incMaxIter = 300
	incEPS     = 3.0e-14
	incFPMIN   = 1.0e-300
)

// gammaSeries evaluates the regularized lower incomplete gamma P(a, x) by its
// series expansion, accurate for x < a+1. The second return reports whether the
// series converged within incMaxIter iterations; a false there means the result
// is not trustworthy and the caller turns it into an error.
func gammaSeries(a, x float64) (float64, bool) {
	if x <= 0 {
		return 0, true // P(a, 0) = 0 exactly
	}
	ap := a
	del := 1.0 / a
	sum := del
	converged := false
	for n := 0; n < incMaxIter; n++ {
		ap++
		del *= x / ap
		sum += del
		if math.Abs(del) < math.Abs(sum)*incEPS {
			converged = true
			break
		}
	}
	return sum * math.Exp(-x+a*math.Log(x)-lgammaVal(a)), converged
}

// gammaContinued evaluates the regularized upper incomplete gamma Q(a, x) by
// its continued fraction (modified Lentz), accurate for x >= a+1. The second
// return reports convergence (see gammaSeries).
func gammaContinued(a, x float64) (float64, bool) {
	b := x + 1 - a
	c := 1.0 / incFPMIN
	d := 1.0 / b
	h := d
	converged := false
	for i := 1; i < incMaxIter; i++ {
		an := -float64(i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < incFPMIN {
			d = incFPMIN
		}
		c = b + an/c
		if math.Abs(c) < incFPMIN {
			c = incFPMIN
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < incEPS {
			converged = true
			break
		}
	}
	return math.Exp(-x+a*math.Log(x)-lgammaVal(a)) * h, converged
}

// regularizedGammaP returns P(a, x), the regularized lower incomplete gamma,
// in [0, 1], plus a convergence flag. Requires a > 0 and x >= 0 (checked by the
// caller).
func regularizedGammaP(a, x float64) (float64, bool) {
	if x < a+1 {
		return gammaSeries(a, x)
	}
	q, ok := gammaContinued(a, x)
	return 1 - q, ok
}

// betaContinued is the continued fraction for the regularized incomplete beta,
// evaluated by modified Lentz (Numerical Recipes betacf). The second return
// reports convergence.
func betaContinued(x, a, b float64) (float64, bool) {
	qab := a + b
	qap := a + 1
	qam := a - 1
	c := 1.0
	d := 1 - qab*x/qap
	if math.Abs(d) < incFPMIN {
		d = incFPMIN
	}
	d = 1 / d
	h := d
	converged := false
	for m := 1; m < incMaxIter; m++ {
		fm := float64(m)
		m2 := 2 * fm
		aa := fm * (b - fm) * x / ((qam + m2) * (a + m2))
		d = 1 + aa*d
		if math.Abs(d) < incFPMIN {
			d = incFPMIN
		}
		c = 1 + aa/c
		if math.Abs(c) < incFPMIN {
			c = incFPMIN
		}
		d = 1 / d
		h *= d * c
		aa = -(a + fm) * (qab + fm) * x / ((a + m2) * (qap + m2))
		d = 1 + aa*d
		if math.Abs(d) < incFPMIN {
			d = incFPMIN
		}
		c = 1 + aa/c
		if math.Abs(c) < incFPMIN {
			c = incFPMIN
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < incEPS {
			converged = true
			break
		}
	}
	return h, converged
}

// regularizedIncBeta returns I_x(a, b), the regularized incomplete beta, in
// [0, 1], plus a convergence flag. Requires 0 <= x <= 1, a > 0, b > 0 (checked
// by the caller). The continued fraction is applied to whichever of x / (1-x)
// converges fastest.
func regularizedIncBeta(x, a, b float64) (float64, bool) {
	if x <= 0 {
		return 0, true
	}
	if x >= 1 {
		return 1, true
	}
	front := math.Exp(a*math.Log(x) + b*math.Log(1-x) - (lgammaVal(a) + lgammaVal(b) - lgammaVal(a+b)))
	if x < (a+1)/(a+b+2) {
		cf, ok := betaContinued(x, a, b)
		return front * cf / a, ok
	}
	cf, ok := betaContinued(1-x, b, a)
	return 1 - front*cf/b, ok
}

// clamp01 pins a value to [0, 1] - a CDF is in [0, 1] by definition, so the
// last-ULP excursions catastrophic cancellation can produce at the extreme
// edges are corrected here (applied only after the finite / convergence checks).
func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// finiteCDF wraps a hand-rolled CDF result in the same strict contract the rest
// of `math` honors: a non-converged series / continued fraction, or a non-finite
// value, is a catchable error rather than a silently-wrong or NaN result; a
// finite converged value is clamped into [0, 1]. `ctx` is the pre-formatted
// "name(args)" for the message.
func finiteCDF(ctx string, v float64, converged bool) (interpreter.Value, error) {
	if !converged {
		return interpreter.Null(), fmt.Errorf("%s: did not converge (arguments outside the reliable range)", ctx)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return interpreter.Null(), fmt.Errorf("%s: result is undefined or infinite", ctx)
	}
	return interpreter.FloatVal(clamp01(v)), nil
}

// regGammaPFn is the Jennifer builtin for the regularized lower incomplete
// gamma P(a, x): the CDF of the gamma distribution. a > 0, x >= 0.
func regGammaPFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	a, x, err := gammaArgs("regGammaP", args)
	if err != nil {
		return interpreter.Null(), err
	}
	p, ok := regularizedGammaP(a, x)
	return finiteCDF(fmt.Sprintf("regGammaP(%s, %s)", interpreter.DisplayFloat(a), interpreter.DisplayFloat(x)), p, ok)
}

// regGammaQFn is the complement Q(a, x) = 1 - P(a, x).
func regGammaQFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	a, x, err := gammaArgs("regGammaQ", args)
	if err != nil {
		return interpreter.Null(), err
	}
	p, ok := regularizedGammaP(a, x)
	return finiteCDF(fmt.Sprintf("regGammaQ(%s, %s)", interpreter.DisplayFloat(a), interpreter.DisplayFloat(x)), 1-p, ok)
}

// gammaArgs validates (a, x) for the regularized incomplete gamma: a > 0 and
// x >= 0.
func gammaArgs(name string, args []interpreter.Value) (a, x float64, err error) {
	if len(args) != 2 {
		return 0, 0, fmt.Errorf("%s expects 2 arguments (a, x), got %d", name, len(args))
	}
	var aok, xok bool
	a, aok = args[0].AsFloat()
	x, xok = args[1].AsFloat()
	if !aok || !xok {
		return 0, 0, fmt.Errorf("%s(): requires numeric operands, got %s and %s", name, args[0].Kind, args[1].Kind)
	}
	if a <= 0 {
		return 0, 0, fmt.Errorf("%s(): a must be positive, got %s", name, interpreter.DisplayFloat(a))
	}
	if x < 0 {
		return 0, 0, fmt.Errorf("%s(): x must be non-negative, got %s", name, interpreter.DisplayFloat(x))
	}
	return a, x, nil
}

// regBetaIFn is the Jennifer builtin for the regularized incomplete beta
// I_x(a, b): the CDF engine of the beta, Student's t, F, and binomial
// distributions. 0 <= x <= 1, a > 0, b > 0.
func regBetaIFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("regBetaI expects 3 arguments (x, a, b), got %d", len(args))
	}
	x, xok := args[0].AsFloat()
	a, aok := args[1].AsFloat()
	b, bok := args[2].AsFloat()
	if !xok || !aok || !bok {
		return interpreter.Null(), fmt.Errorf("regBetaI(): requires numeric operands, got %s, %s and %s", args[0].Kind, args[1].Kind, args[2].Kind)
	}
	if x < 0 || x > 1 {
		return interpreter.Null(), fmt.Errorf("regBetaI(): x must be in [0, 1], got %s", interpreter.DisplayFloat(x))
	}
	if a <= 0 || b <= 0 {
		return interpreter.Null(), fmt.Errorf("regBetaI(): a and b must be positive, got %s and %s", interpreter.DisplayFloat(a), interpreter.DisplayFloat(b))
	}
	v, ok := regularizedIncBeta(x, a, b)
	return finiteCDF(fmt.Sprintf("regBetaI(%s, %s, %s)", interpreter.DisplayFloat(x), interpreter.DisplayFloat(a), interpreter.DisplayFloat(b)), v, ok)
}
