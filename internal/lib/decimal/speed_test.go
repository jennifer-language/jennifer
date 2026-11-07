// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package decimallib

import (
	"math/big"
	"testing"
	"time"
)

// within fails when f takes longer than d (the per-factor loops took 0.5-2.3 s
// on these inputs).
func within(t *testing.T, d time.Duration, what string, f func()) {
	t.Helper()
	start := time.Now()
	f()
	if el := time.Since(start); el > d {
		t.Errorf("%s took %v, want under %v", what, el, d)
	}
}

// TestDivExactLargePowers: 1 / 5^143000 and 1 / 2^332000 are refused quickly
// (one division per factor of 5, one shift per factor of 2 before).
func TestDivExactLargePowers(t *testing.T) {
	for _, base := range []int64{5, 2} {
		n := 143000
		if base == 2 {
			n = 332000
		}
		den := dec{coef: new(big.Int).Exp(big.NewInt(base), big.NewInt(int64(n)), nil)}
		within(t, 300*time.Millisecond, "div(1, pow)", func() {
			if _, err := divExact(dec{coef: big.NewInt(1)}, den); err != errLimit {
				t.Errorf("1 / %d^%d: got %v, want errLimit", base, n, err)
			}
		})
	}
	// The exponent is still exact: 3 / (2^7 * 5^300) = 3 * 2^293 / 10^300.
	den := new(big.Int).Mul(new(big.Int).Lsh(big.NewInt(1), 7), new(big.Int).Exp(big.NewInt(5), big.NewInt(300), nil))
	got, err := divExact(dec{coef: big.NewInt(3)}, dec{coef: den})
	want := new(big.Int).Mul(big.NewInt(3), new(big.Int).Lsh(big.NewInt(1), 293))
	if err != nil || got.scale != 300 || got.coef.Cmp(want) != 0 {
		t.Errorf("3 / (2^7 5^300) = %v scale %d (%v), want 3*2^293 scale 300", got.coef, got.scale, err)
	}
	for _, e := range []int{0, 1, 2, 3, 7, 8, 63, 64, 65, 1000} {
		r := new(big.Int).Mul(big.NewInt(3), new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(e)), nil))
		if n := removeFives(r); n != e || r.Cmp(big.NewInt(3)) != 0 {
			t.Errorf("removeFives(3*5^%d) = %d, rest %v", e, n, r)
		}
	}
	if _, err := divExact(dec{coef: big.NewInt(1)}, dec{coef: big.NewInt(3 * 1024)}); err != errInexact {
		t.Errorf("1 / 3072: got %v, want errInexact", err)
	}
}

// TestNormalizeManyZeros: 1 with 100000 zero places normalizes to 1 quickly.
func TestNormalizeManyZeros(t *testing.T) {
	d := dec{coef: new(big.Int).Exp(big.NewInt(10), big.NewInt(maxDigits), nil), scale: maxDigits}
	within(t, 300*time.Millisecond, "normalize", func() {
		got := normalize(d)
		if got.scale != 0 || got.coef.Cmp(big.NewInt(1)) != 0 {
			t.Errorf("normalize = %v scale %d, want 1", got.coef, got.scale)
		}
	})
	for in, want := range map[string]string{"1.2300": "1.23", "-0.500": "-0.5", "0.000": "0", "100": "100", "-120.0": "-120"} {
		d, err := parse(in)
		if err != nil {
			t.Fatal(err)
		}
		if got := normalize(d).String(); got != want {
			t.Errorf("normalize(%s) = %s, want %s", in, got, want)
		}
	}
}
