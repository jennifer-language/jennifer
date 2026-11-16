// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package decimallib

import (
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
// count. "1e100000" parsed (the bit-length test let a 100,001-digit
// coefficient through) but its toString did not; a value at scale 100000
// printed as "0.<100000 digits>", which parse refused for its leading zero.
func TestDigitLimitRoundTrip(t *testing.T) {
	nines := strings.Repeat("9", maxDigits)
	add := binary("add", add)
	mul := binary("mul", mul)

	// At the limit: maxDigits coefficient digits, or scale maxDigits.
	roundTrip(t, "1e99999", call(t, fromFn, str("1e99999")))
	roundTrip(t, "99...9", call(t, fromFn, str(nines)))
	roundTrip(t, "-99...9", call(t, fromFn, str("-"+nines)))
	roundTrip(t, "0.99...9", call(t, fromFn, str("0."+nines)))
	roundTrip(t, "0.00...1", call(t, fromFn, str("0."+strings.Repeat("0", maxDigits-1)+"1")))
	roundTrip(t, "div(1, 3, max)", call(t, divFn, num(1), num(3), num(maxDigits)))
	roundTrip(t, "div(-1, 7, max)", call(t, divFn, num(-1), num(7), num(maxDigits)))
	roundTrip(t, "99...8 + 1", call(t, add, str(nines[1:]+"8"), num(1)))
	roundTrip(t, "1e49999 * 9e50000", call(t, mul, str("1e49999"), str("9e50000")))

	// Just over: one digit more is kind "limit" wherever it arises.
	over := "more than 100000 digits"
	fails(t, "limit", over, fromFn, str("1e100000"))
	fails(t, "limit", over, fromFn, str("10e99999"))
	fails(t, "limit", over, fromFn, str("1"+nines))
	fails(t, "limit", over, fromFn, str("0.0"+nines))
	fails(t, "limit", over, add, str(nines), num(1))
	fails(t, "limit", over, add, str("-"+nines), num(-1))
	fails(t, "limit", over, mul, str("1e50000"), str("1e50000"))
	fails(t, "limit", over, powFn, num(10), num(maxDigits))
	fails(t, "limit", over, roundFn, str(nines), num(1))
	// Leading zeros are not digits.
	roundTrip(t, "00099...9", call(t, fromFn, str("000"+nines)))
}
