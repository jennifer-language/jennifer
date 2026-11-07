// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package mathlib

import (
	"math"
	"testing"
)

// TestLargeParameterReference pins the incomplete gamma / beta and lbeta
// against mpmath (50 digits). Their prefactors took differences of lgamma
// terms ~ a ln a, which cancel: regBetaI(0.5, 1e6, 1e6) was 0.50000000047,
// regBetaI(3.7e-12, 1, 1e12) was off by 4.4e-5 relative, regGammaQ(1e6,
// 1.001e6) by 4.2e-10.
func TestLargeParameterReference(t *testing.T) {
	rel := func(got, want float64) float64 { return math.Abs(got-want) / math.Abs(want) }
	for _, c := range []struct {
		fn      string
		x, a, b float64
		want    float64
		tol     float64
	}{
		{"regBetaI", 0.5, 1e6, 1e6, 0.5, 1e-12},
		{"regBetaI", 3.7e-12, 1, 1e12, 0.97527647352982984, 1e-14},
		{"regBetaI", 0.9, 1000, 50, 1.6931176560279121e-10, 1e-13},
		{"regBetaI", 0.4, 30, 50, 0.68294284603088809, 1e-14},
		{"regBetaI", 0.3, 2.5, 4, 0.35219758590676721, 1e-14},
		{"regGammaP", 1e4, 1e4, 0, 0.50132980833995520, 1e-13},
		{"regGammaQ", 1e4, 1e4, 0, 0.49867019166004480, 1e-13},
		{"regGammaP", 1e6, 1.001e6, 0, 0.84134478636834029, 1e-14},
		{"regGammaQ", 1e6, 1.001e6, 0, 0.15865521363165971, 1e-13},
		{"regGammaQ", 12, 30, 0, 6.3877025399273365e-05, 1e-14},
		{"lbeta", 1, 1e12, 0, -27.631021115928548, 1e-15},
		{"lbeta", 1e6, 1e6, 0, -1386300.0033629211, 1e-15},
		{"lbeta", 15, 1e8, 0, -251.11899102654675, 1e-15},
		{"lbeta", 2.5, 30, 0, -8.2794635759734335, 1e-15},
	} {
		var got float64
		var ok bool
		switch c.fn {
		case "regBetaI":
			got, ok = regularizedIncBeta(c.x, c.a, c.b)
		case "regGammaP":
			got, ok = regularizedGammaP(c.x, c.a)
		case "regGammaQ":
			got, ok = RegularizedGammaQ(c.x, c.a)
		case "lbeta":
			got, ok = lbetaVal(c.x, c.a), true
		}
		if !ok || rel(got, c.want) > c.tol {
			t.Errorf("%s(%v, %v, %v) = %v (ok %v), want %v (rel %.1e > %.0e)", c.fn, c.x, c.a, c.b, got, ok, c.want, rel(got, c.want), c.tol)
		}
	}
	// Stirlerr at the series / direct switch agrees with the direct form.
	for _, n := range []float64{9.999999, 10, 10.5, 15} { // the direct form cancels beyond
		direct := lgammaVal(n+1) - (n+0.5)*math.Log(n) + n - lnSqrt2Pi
		if d := math.Abs(Stirlerr(n) - direct); d > 1e-14 {
			t.Errorf("Stirlerr(%v) = %v, direct %v", n, Stirlerr(n), direct)
		}
	}
}
