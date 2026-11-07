// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package mathlib

import "math"

// Densities for large parameters. ln Gamma(n) ~ n ln n, so a difference of
// lgamma terms loses about n ln n * 1e-16 absolutely; at n = 1e12 that is a
// relative error near 1e-3 in the density. The forms below follow C. Loader,
// "Fast and Accurate Computation of Binomial Probabilities" (2000), as R's
// dbinom / dpois / dt / dbeta do: Stirling's correction stirlerr and the
// deviance term bd0 keep every term small.

const lnSqrt2Pi = 0.918938533204672741780329736406 // ln(sqrt(2*pi))

// Stirlerr returns ln Gamma(n+1) - ((n+0.5) ln n - n + ln sqrt(2 pi)), the
// error of Stirling's formula, for n > 0. Below 10 it is computed directly
// (all terms small); from 10 on by the asymptotic series, whose first omitted
// term is below 1e-16 there.
func Stirlerr(n float64) float64 {
	if n < 10 {
		return lgammaVal(n+1) - (n+0.5)*math.Log(n) + n - lnSqrt2Pi
	}
	const (
		s0 = 1.0 / 12
		s1 = 1.0 / 360
		s2 = 1.0 / 1260
		s3 = 1.0 / 1680
		s4 = 1.0 / 1188
		s5 = 691.0 / 360360
		s6 = 1.0 / 156
	)
	nn := n * n
	return (s0 - (s1-(s2-(s3-(s4-(s5-s6/nn)/nn)/nn)/nn)/nn)/nn) / n
}

// Bd0 returns the deviance term x ln(x/np) + np - x for x >= 0, np > 0,
// summing a series when x is close to np, where the direct form cancels.
func Bd0(x, np float64) float64 {
	if x == 0 {
		return np
	}
	if math.Abs(x-np) < 0.1*(x+np) {
		v := (x - np) / (x + np)
		s := (x - np) * v
		ej := 2 * x * v
		v *= v
		for j := 1; j < 1000; j++ {
			ej *= v
			s1 := s + ej/float64(2*j+1)
			if s1 == s {
				return s1
			}
			s = s1
		}
		return s
	}
	return x*math.Log(x/np) + np - x
}

// DbinomRaw returns the binomial density C(n, x) p^x q^(n-x) for real
// 0 <= x <= n, with q = 1 - p passed separately so it keeps its precision.
func DbinomRaw(x, n, p, q float64) float64 {
	if p == 0 {
		if x == 0 {
			return 1
		}
		return 0
	}
	if q == 0 {
		if x == n {
			return 1
		}
		return 0
	}
	if x == 0 {
		if n == 0 {
			return 1
		}
		if p < 0.1 {
			return math.Exp(-Bd0(n, n*q) - n*p)
		}
		return math.Exp(n * math.Log(q))
	}
	if x == n {
		if q < 0.1 {
			return math.Exp(-Bd0(n, n*p) - n*q)
		}
		return math.Exp(n * math.Log(p))
	}
	if x < 0 || x > n {
		return 0
	}
	lc := Stirlerr(n) - Stirlerr(x) - Stirlerr(n-x) - Bd0(x, n*p) - Bd0(n-x, n*q)
	// 1 / sqrt(2 pi x (n-x) / n), in logs.
	lf := 2*lnSqrt2Pi + math.Log(x) + math.Log1p(-x/n)
	return math.Exp(lc - 0.5*lf)
}

// DpoisRaw returns the Poisson density lambda^x e^-lambda / Gamma(x+1) for
// real x >= 0, lambda > 0.
func DpoisRaw(x, lambda float64) float64 {
	if x == 0 {
		return math.Exp(-lambda)
	}
	if x < 0 {
		return 0
	}
	return math.Exp(-Stirlerr(x)-Bd0(x, lambda)) / math.Sqrt(2*math.Pi*x)
}

// lgammacor is Stirlerr under the name R's lbeta uses (ln Gamma(x) minus its
// Stirling approximation (x-0.5) ln x - x + ln sqrt(2 pi)).
func lgammacor(x float64) float64 { return Stirlerr(x) }

// lbetaVal returns ln B(a, b) for a, b > 0. With a large argument the lgamma
// sum cancels; R's lbeta splits off the Stirling parts analytically instead.
func lbetaVal(a, b float64) float64 {
	p, q := math.Min(a, b), math.Max(a, b)
	switch {
	case p >= 10:
		corr := lgammacor(p) + lgammacor(q) - lgammacor(p+q)
		return -0.5*math.Log(q) + lnSqrt2Pi + corr + (p-0.5)*math.Log(p/(p+q)) + q*math.Log1p(-p/(p+q))
	case q >= 10:
		corr := lgammacor(q) - lgammacor(p+q)
		return lgammaVal(p) + corr + p - p*math.Log(p+q) + (q-0.5)*math.Log1p(-p/(p+q))
	}
	return lgammaVal(p) + lgammaVal(q) - lgammaVal(p+q)
}
