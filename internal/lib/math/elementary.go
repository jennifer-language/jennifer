// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package mathlib

import (
	"fmt"
	"math"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// oneFloat wraps a total-or-domain-restricted float64->float64 Go function as
// a Jennifer builtin: arity 1, accepts int or float, returns float. It keeps
// math's strict stance - a NaN or +/-Inf result (an out-of-domain input like
// asin(2) or ln(0), or an overflow like exp(1e9)) becomes a catchable error
// rather than a propagating NaN. Functions whose domain is all reals (sin,
// atan, cbrt, erf, ...) simply never trip the check.
func oneFloat(name string, fn func(float64) float64) interpreter.Builtin {
	return func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		if err := arityOne(name, args); err != nil {
			return interpreter.Null(), err
		}
		x, ok := args[0].AsFloat()
		if !ok {
			return interpreter.Null(), fmt.Errorf("%s(): requires int or float, got %s", name, args[0].Kind)
		}
		r := fn(x)
		if math.IsNaN(r) || math.IsInf(r, 0) {
			return interpreter.Null(), fmt.Errorf("%s(%s): result is undefined or infinite", name, interpreter.DisplayFloat(x))
		}
		return interpreter.FloatVal(r), nil
	}
}

// twoFloat is the two-argument analogue of oneFloat (atan2, hypot, log-with-base):
// arity 2, both int or float, float result, strict on a non-finite outcome.
func twoFloat(name string, fn func(float64, float64) float64) interpreter.Builtin {
	return func(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
		if err := arityTwo(name, args); err != nil {
			return interpreter.Null(), err
		}
		a, aok := args[0].AsFloat()
		b, bok := args[1].AsFloat()
		if !aok || !bok {
			return interpreter.Null(), fmt.Errorf("%s(): requires numeric operands, got %s and %s", name, args[0].Kind, args[1].Kind)
		}
		r := fn(a, b)
		if math.IsNaN(r) || math.IsInf(r, 0) {
			return interpreter.Null(), fmt.Errorf("%s(%s, %s): result is undefined or infinite", name, interpreter.DisplayFloat(a), interpreter.DisplayFloat(b))
		}
		return interpreter.FloatVal(r), nil
	}
}

// installElementary registers the everyday trigonometric, hyperbolic,
// exponential / logarithmic, and combinatorial functions on the `math`
// namespace, plus the TAU constant.
func installElementary(in *interpreter.Interpreter) {
	// Trigonometry (radians). asin / acos error outside [-1, 1] via the
	// strict NaN check; atan2 takes (y, x).
	in.RegisterNamespaced(LibraryName, "sin", oneFloat("sin", math.Sin))
	in.RegisterNamespaced(LibraryName, "cos", oneFloat("cos", math.Cos))
	in.RegisterNamespaced(LibraryName, "tan", oneFloat("tan", math.Tan))
	in.RegisterNamespaced(LibraryName, "asin", oneFloat("asin", math.Asin))
	in.RegisterNamespaced(LibraryName, "acos", oneFloat("acos", math.Acos))
	in.RegisterNamespaced(LibraryName, "atan", oneFloat("atan", math.Atan))
	in.RegisterNamespaced(LibraryName, "atan2", twoFloat("atan2", math.Atan2))

	// Hyperbolic. acosh errors for x < 1, atanh outside (-1, 1); sinh / cosh
	// error when they overflow to infinity.
	in.RegisterNamespaced(LibraryName, "sinh", oneFloat("sinh", math.Sinh))
	in.RegisterNamespaced(LibraryName, "cosh", oneFloat("cosh", math.Cosh))
	in.RegisterNamespaced(LibraryName, "tanh", oneFloat("tanh", math.Tanh))
	in.RegisterNamespaced(LibraryName, "asinh", oneFloat("asinh", math.Asinh))
	in.RegisterNamespaced(LibraryName, "acosh", oneFloat("acosh", math.Acosh))
	in.RegisterNamespaced(LibraryName, "atanh", oneFloat("atanh", math.Atanh))

	// Exponentials and logarithms. exp / expm1 error on overflow; ln / log10 /
	// log2 / log1p error on a non-positive argument (the pole / domain edge).
	in.RegisterNamespaced(LibraryName, "exp", oneFloat("exp", math.Exp))
	in.RegisterNamespaced(LibraryName, "expm1", oneFloat("expm1", math.Expm1))
	in.RegisterNamespaced(LibraryName, "ln", oneFloat("ln", math.Log))
	in.RegisterNamespaced(LibraryName, "log10", oneFloat("log10", math.Log10))
	in.RegisterNamespaced(LibraryName, "log2", oneFloat("log2", math.Log2))
	in.RegisterNamespaced(LibraryName, "log1p", oneFloat("log1p", math.Log1p))
	// log(x, base): arbitrary base. A non-positive x, a non-positive base, or
	// base 1 all yield a non-finite ln-ratio and so error via the strict check.
	in.RegisterNamespaced(LibraryName, "log", twoFloat("log", func(x, base float64) float64 {
		return math.Log(x) / math.Log(base)
	}))

	// Roots / magnitude. cbrt is total (handles negatives); hypot(x, y) is the
	// overflow-safe sqrt(x*x + y*y).
	in.RegisterNamespaced(LibraryName, "cbrt", oneFloat("cbrt", math.Cbrt))
	in.RegisterNamespaced(LibraryName, "hypot", twoFloat("hypot", math.Hypot))

	in.RegisterNamespaced(LibraryName, "sign", signFn)
	in.RegisterNamespaced(LibraryName, "trunc", truncFn)

	// Combinatorics (int in, int out; overflow is a catchable error).
	in.RegisterNamespaced(LibraryName, "factorial", factorialFn)
	in.RegisterNamespaced(LibraryName, "comb", combFn)
	in.RegisterNamespaced(LibraryName, "perm", permFn)
	in.RegisterNamespaced(LibraryName, "gcd", gcdFn)
	in.RegisterNamespaced(LibraryName, "lcm", lcmFn)

	// TAU = 2*PI, the full-turn constant (companion to PI / E).
	in.RegisterNamespacedConst(LibraryName, "TAU", interpreter.FloatVal(2*math.Pi))
}

// signFn returns -1, 0, or 1 with the sign of the operand, preserving its type
// (int -> int, float -> float). A NaN float would have no meaningful sign, so
// it errors (defensive - the language does not normally admit NaN).
func signFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("sign", args); err != nil {
		return interpreter.Null(), err
	}
	v := args[0]
	switch v.Kind {
	case interpreter.KindInt:
		switch {
		case v.Int < 0:
			return interpreter.IntVal(-1), nil
		case v.Int > 0:
			return interpreter.IntVal(1), nil
		default:
			return interpreter.IntVal(0), nil
		}
	case interpreter.KindFloat:
		f := v.Float
		if math.IsNaN(f) {
			return interpreter.Null(), fmt.Errorf("sign(): argument is not a number")
		}
		switch {
		case f < 0:
			return interpreter.FloatVal(-1), nil
		case f > 0:
			return interpreter.FloatVal(1), nil
		default: // +0.0 and -0.0
			return interpreter.FloatVal(0), nil
		}
	}
	return interpreter.Null(), fmt.Errorf("sign(): requires int or float, got %s", v.Kind)
}

// truncFn rounds toward zero, returning int - the companion to floor / ceil /
// round. An int argument is the identity.
func truncFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("trunc", args); err != nil {
		return interpreter.Null(), err
	}
	v := args[0]
	if v.Kind == interpreter.KindInt {
		return v, nil
	}
	if v.Kind != interpreter.KindFloat {
		return interpreter.Null(), fmt.Errorf("trunc(): requires int or float, got %s", v.Kind)
	}
	return floatToInt("trunc", math.Trunc(v.Float))
}

// factorialFn returns n! as an int, rejecting a negative n and an overflow
// past int64 (21! and beyond).
func factorialFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityOne("factorial", args); err != nil {
		return interpreter.Null(), err
	}
	if args[0].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("factorial(): requires an int, got %s", args[0].Kind)
	}
	n := args[0].Int
	if n < 0 {
		return interpreter.Null(), fmt.Errorf("factorial(%d): undefined for a negative argument", n)
	}
	result := int64(1)
	for i := int64(2); i <= n; i++ {
		if result > math.MaxInt64/i {
			return interpreter.Null(), fmt.Errorf("factorial(%d): result overflows an int", n)
		}
		result *= i
	}
	return interpreter.IntVal(result), nil
}

// combFn returns the binomial coefficient nCr (n choose k) as an exact int.
// Negative n or k errors; k > n is 0 (as in Python's math.comb). Uses the
// multiplicative form with per-step exact division, erroring only if an
// intermediate product would overflow int64.
func combFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityTwo("comb", args); err != nil {
		return interpreter.Null(), err
	}
	if args[0].Kind != interpreter.KindInt || args[1].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("comb(): requires int operands, got %s and %s", args[0].Kind, args[1].Kind)
	}
	n, k := args[0].Int, args[1].Int
	if n < 0 || k < 0 {
		return interpreter.Null(), fmt.Errorf("comb(%d, %d): requires non-negative arguments", n, k)
	}
	if k > n {
		return interpreter.IntVal(0), nil
	}
	if k > n-k { // symmetry: fewer multiplications, smaller intermediates
		k = n - k
	}
	// Multiplicative form, dividing before multiplying so the running value is
	// always an exact binomial coefficient C(n-k+i, i). Cancelling gcd(num, i)
	// first makes that division exact (ii then divides result), and the only
	// product formed, (result/ii)*num, equals the next coefficient - so the
	// largest intermediate is the final answer itself. comb therefore succeeds
	// for every result that fits in an int64, and the overflow check fires only
	// when the true result genuinely does not.
	result := int64(1)
	for i := int64(1); i <= k; i++ {
		num := n - k + i
		g := int64(gcdU(uint64(num), uint64(i)))
		num /= g
		ii := i / g
		result /= ii // exact: gcd(num, ii) == 1 and ii divides result*num
		if result > math.MaxInt64/num {
			return interpreter.Null(), fmt.Errorf("comb(%d, %d): result overflows an int", args[0].Int, args[1].Int)
		}
		result *= num
	}
	return interpreter.IntVal(result), nil
}

// permFn returns the number of k-permutations of n (nPr = n! / (n-k)!) as an
// exact int. Negative arguments error; k > n is 0 (as in Python's math.perm).
func permFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityTwo("perm", args); err != nil {
		return interpreter.Null(), err
	}
	if args[0].Kind != interpreter.KindInt || args[1].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("perm(): requires int operands, got %s and %s", args[0].Kind, args[1].Kind)
	}
	n, k := args[0].Int, args[1].Int
	if n < 0 || k < 0 {
		return interpreter.Null(), fmt.Errorf("perm(%d, %d): requires non-negative arguments", n, k)
	}
	if k > n {
		return interpreter.IntVal(0), nil
	}
	result := int64(1)
	for i := int64(0); i < k; i++ {
		m := n - i
		if m > 0 && result > math.MaxInt64/m {
			return interpreter.Null(), fmt.Errorf("perm(%d, %d): result overflows an int", n, k)
		}
		result *= m
	}
	return interpreter.IntVal(result), nil
}

// absU returns the magnitude of x as a uint64, correct even for math.MinInt64
// (whose negation overflows int64).
func absU(x int64) uint64 {
	if x < 0 {
		return uint64(^x) + 1
	}
	return uint64(x)
}

// gcdU is the unsigned Euclidean gcd.
func gcdU(a, b uint64) uint64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// gcdFn returns the greatest common divisor of two ints (non-negative;
// gcd(0, 0) == 0). Magnitudes are taken in uint64 so MinInt64 is handled; the
// one result that does not fit int64 (gcd(MinInt64, MinInt64) == 2^63) errors.
func gcdFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityTwo("gcd", args); err != nil {
		return interpreter.Null(), err
	}
	if args[0].Kind != interpreter.KindInt || args[1].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("gcd(): requires int operands, got %s and %s", args[0].Kind, args[1].Kind)
	}
	g := gcdU(absU(args[0].Int), absU(args[1].Int))
	if g > math.MaxInt64 {
		return interpreter.Null(), fmt.Errorf("gcd(%d, %d): result overflows an int", args[0].Int, args[1].Int)
	}
	return interpreter.IntVal(int64(g)), nil
}

// lcmFn returns the least common multiple of two ints (non-negative;
// lcm(x, 0) == 0). Computed as |a|/gcd * |b| in uint64, erroring if it
// overflows int64.
func lcmFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arityTwo("lcm", args); err != nil {
		return interpreter.Null(), err
	}
	if args[0].Kind != interpreter.KindInt || args[1].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("lcm(): requires int operands, got %s and %s", args[0].Kind, args[1].Kind)
	}
	a, b := args[0].Int, args[1].Int
	if a == 0 || b == 0 {
		return interpreter.IntVal(0), nil
	}
	ua, ub := absU(a), absU(b)
	q := ua / gcdU(ua, ub) // divide first to keep the product small
	if q > uint64(math.MaxInt64)/ub {
		return interpreter.Null(), fmt.Errorf("lcm(%d, %d): result overflows an int", a, b)
	}
	l := q * ub
	if l > math.MaxInt64 {
		return interpreter.Null(), fmt.Errorf("lcm(%d, %d): result overflows an int", a, b)
	}
	return interpreter.IntVal(int64(l)), nil
}
