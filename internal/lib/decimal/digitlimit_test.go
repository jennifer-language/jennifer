// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package decimallib

import (
	"fmt"
	"math/big"
	"strings"
	"testing"
)

// roundTrip checks that v's toString parses back to the same value.
func roundTrip(t *testing.T, what string, v Value) {
	t.Helper()
	n, ok := v.AsNativeObject(LibraryName, "Decimal")
	if !ok {
		t.Fatalf("%s: not a Decimal: %v", what, v)
	}
	d := n.(*decValue).d
	back, err := parse(d.String())
	if err != nil {
		t.Fatalf("%s: toString does not parse back: %v", what, err)
	}
	if back.scale != d.scale || back.coef.Cmp(d.coef) != 0 {
		t.Fatalf("%s: round trip changed the value", what)
	}
}

// TestDigitLimitRoundTrip: parse and every result enforce the same digit
// count. "1e100000" parsed under the 100,000-digit cap (the bit-length test
// let a 100,001-digit coefficient through) but its toString did not; a value
// at scale 100000 printed as "0.<100000 digits>", which parse refused for its
// leading zero.
func TestDigitLimitRoundTrip(t *testing.T) {
	nines := strings.Repeat("9", maxDigits)
	add := binary("add", add)
	mul := binary("mul", mul)

	// At the limit: maxDigits coefficient digits, or scale maxDigits.
	half := maxDigits / 2
	roundTrip(t, "1e(max-1)", call(t, fromFn, str(fmt.Sprintf("1e%d", maxDigits-1))))
	roundTrip(t, "99...9", call(t, fromFn, str(nines)))
	roundTrip(t, "-99...9", call(t, fromFn, str("-"+nines)))
	roundTrip(t, "0.99...9", call(t, fromFn, str("0."+nines)))
	roundTrip(t, "0.00...1", call(t, fromFn, str("0."+strings.Repeat("0", maxDigits-1)+"1")))
	roundTrip(t, "div(1, 3, max)", call(t, divFn, num(1), num(3), num(maxDigits)))
	roundTrip(t, "div(-1, 7, max)", call(t, divFn, num(-1), num(7), num(maxDigits)))
	roundTrip(t, "99...8 + 1", call(t, add, str(nines[1:]+"8"), num(1)))
	roundTrip(t, "1e(half-1) * 9e(half)", call(t, mul, str(fmt.Sprintf("1e%d", half-1)), str(fmt.Sprintf("9e%d", half))))

	// Just over: one digit more is kind "limit" wherever it arises.
	over := fmt.Sprintf("more than %d digits", maxDigits)
	fails(t, "limit", over, fromFn, str(fmt.Sprintf("1e%d", maxDigits)))
	fails(t, "limit", over, fromFn, str(fmt.Sprintf("10e%d", maxDigits-1)))
	fails(t, "limit", over, fromFn, str("1"+nines))
	fails(t, "limit", over, fromFn, str("0.0"+nines))
	fails(t, "limit", over, add, str(nines), num(1))
	fails(t, "limit", over, add, str("-"+nines), num(-1))
	fails(t, "limit", over, mul, str(fmt.Sprintf("1e%d", half)), str(fmt.Sprintf("1e%d", half)))
	fails(t, "limit", over, powFn, num(10), num(maxDigits))
	fails(t, "limit", over, roundFn, str(nines), num(1))
	// Leading zeros are not digits.
	roundTrip(t, "00099...9", call(t, fromFn, str("000"+nines)))
}

// TestDigitCapFitsBigPowers: 5^4^3^2 = 5^262144 (Rosetta Code's
// arbitrary-precision integers task) has 183,231 digits and fits the default
// build's cap.
func TestDigitCapFitsBigPowers(t *testing.T) {
	if maxDigits < 183231 {
		t.Skipf("maxDigits %d on this build is below 5^262144's 183,231 digits", maxDigits)
	}
	got := call(t, powFn, num(5), num(262144))
	str := s(got)
	if len(str) != 183231 {
		t.Fatalf("5^262144 has %d digits, want 183231", len(str))
	}
	if str[:20] != "62060698786608744707" || str[len(str)-20:] != "92256259918212890625" {
		t.Errorf("5^262144 = %s...%s", str[:20], str[len(str)-20:])
	}
	roundTrip(t, "5^262144", got)
}

// TestScanDigits: the split scan gives SetString's value at every length
// around its chunk boundaries, with leading zeros in either half.
func TestScanDigits(t *testing.T) {
	digits := strings.Repeat("9071234568", 1+(8*scanChunk+3)/10)
	for _, n := range []int{0, 1, scanChunk - 1, scanChunk, scanChunk + 1, 2 * scanChunk, 2*scanChunk + 1, 3*scanChunk + 7, 8*scanChunk + 3} {
		for _, in := range []string{digits[:n], strings.Repeat("0", n)} {
			want := new(big.Int)
			if in != "" {
				want.SetString(in, 10)
			}
			if got := scanDigits(in); got.Cmp(want) != 0 {
				t.Errorf("scanDigits of %d digits (%.10s...) differs from SetString", n, in)
			}
		}
	}
	// A zero run across the split point.
	in := strings.Repeat("1", scanChunk) + strings.Repeat("0", 2*scanChunk) + "5"
	want, _ := new(big.Int).SetString(in, 10)
	if got := scanDigits(in); got.Cmp(want) != 0 {
		t.Errorf("scanDigits with an inner zero run differs from SetString")
	}
}
