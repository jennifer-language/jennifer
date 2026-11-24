// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package doctree

import (
	"math"
	"strings"
	"testing"
	stdtime "time"
)

// BigInt keeps its digits normalised and hands a value that fits int64 back
// as an Int, so two big ints compare by string.
func TestBigIntNormalised(t *testing.T) {
	big := "123456789012345678901234567890"
	for _, in := range []string{big, "+" + big, "000" + big, "+00" + big} {
		if n := BigInt(in); n.Kind() != KindBigInt || n.Str() != big {
			t.Errorf("BigInt(%q) = %v %q, want big int %q", in, n.Kind(), n.Str(), big)
		}
	}
	if n := BigInt("-00" + big); n.Str() != "-"+big {
		t.Errorf("BigInt(-00...) = %q", n.Str())
	}
	if n := BigInt("+0000042"); n.Kind() != KindInt || n.IntVal() != 42 {
		t.Errorf("BigInt(+0000042) = %v, want int 42", n.Kind())
	}
	if n := BigInt("-9223372036854775808"); n.Kind() != KindInt || n.IntVal() != math.MinInt64 {
		t.Errorf("BigInt(MinInt64) = %v, want int", n.Kind())
	}
	if !BigInt("000"+big).Equal(BigInt(big)) || BigInt(big).Equal(BigInt("-"+big)) {
		t.Error("big int equality by normalised digits")
	}
	two63 := BigInt("9223372036854775808")
	if !two63.Equal(Float(0x1p63)) || !Float(0x1p63).Equal(two63) {
		t.Error("2^63 big int should equal the float 2^63")
	}
	if two63.Equal(Float(0x1p63+2048)) || two63.Equal(Int(math.MaxInt64)) || two63.Equal(Float(math.Inf(1))) {
		t.Error("2^63 big int equals a different number")
	}
}

// `==` and asFloat on a million-digit integer must not parse its digits
// (big.Rat.SetString took 3.7 s and 1.5 s here), and the error message shows
// only a bounded prefix of the number.
func TestBigIntHugeLinear(t *testing.T) {
	digits := strings.Repeat("7", 1_000_000)
	a, b := BigInt(digits), BigInt(digits)
	start := stdtime.Now()
	if !a.Equal(b) {
		t.Fatal("equal big ints compare unequal")
	}
	if a.Equal(Float(1e300)) || a.Equal(BigInt(digits[1:])) {
		t.Fatal("different numbers compare equal")
	}
	_, ok, err := testLib.numberFloat("t.asFloat", "node ", a)
	if !ok || err == nil {
		t.Fatalf("asFloat of a million-digit int: ok=%v err=%v", ok, err)
	}
	if d := stdtime.Since(start); d > 500*stdtime.Millisecond {
		t.Errorf("== and asFloat took %v on a million-digit integer", d)
	}
	msg := err.Error()
	if len(msg) > 200 || !strings.Contains(msg, "(1000000 digits)") || !strings.Contains(msg, "asDecimal") {
		t.Errorf("asFloat message: %.300q", msg)
	}
	if msg := testLib.bigIntErr("t.asInt", "", a).Error(); len(msg) > 200 {
		t.Errorf("asInt message is %d bytes", len(msg))
	}
	// A big int a float holds exactly still converts.
	if f, _, err := testLib.numberFloat("t.asFloat", "node ", BigInt("-9223372036854775809"+"0")); err == nil || f != 0 {
		t.Errorf("inexact big int converted: %v %v", f, err)
	}
	if f, _, err := testLib.numberFloat("t.asFloat", "node ", BigInt("18446744073709551616")); err != nil || f != 0x1p64 {
		t.Errorf("2^64: %v %v", f, err)
	}
}
