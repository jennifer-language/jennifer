// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter

import (
	"math"
	"testing"

	"jennifer-lang.dev/jennifer/internal/parser"
)

func TestEqualFuncAndObject(t *testing.T) {
	m := &parser.MethodDef{Name: "f"}
	f1, f2 := FuncVal(m), FuncVal(m)
	if !f1.Equal(f1) || !f1.Equal(f2) {
		t.Error("func values for the same method must be equal (incl. to themselves)")
	}
	if f1.Equal(FuncVal(&parser.MethodDef{Name: "g"})) {
		t.Error("func values for different methods must differ")
	}

	a := ObjectVal("json", "Value", IntVal(5))
	b := ObjectVal("json", "Value", IntVal(5))
	c := ObjectVal("json", "Value", IntVal(6))
	if !a.Equal(a) || !a.Equal(b) {
		t.Error("opaque handles with equal content must be equal (incl. to themselves)")
	}
	if a.Equal(c) {
		t.Error("opaque handles with different content must differ")
	}
	if a.Equal(ObjectVal("xml", "Value", IntVal(5))) {
		t.Error("opaque handles of different types must differ")
	}
}

func TestMapKeyEncodeFloat(t *testing.T) {
	ki, _ := mapKeyEncode(IntVal(1))
	kf, ok := mapKeyEncode(FloatVal(1.0))
	if !ok || ki != kf {
		t.Errorf("1 and 1.0 must encode alike (exact cross-kind ==): %q vs %q", ki, kf)
	}
	z, _ := mapKeyEncode(FloatVal(0.0))
	nz, _ := mapKeyEncode(FloatVal(math.Copysign(0, -1)))
	if z != nz {
		t.Errorf("-0.0 and 0.0 must encode alike: %q vs %q", nz, z)
	}
	a, oka := mapKeyEncode(FloatVal(1.5))
	b, _ := mapKeyEncode(FloatVal(1.5))
	if !oka || a != b {
		t.Error("a non-integer float must encode stably")
	}
	// A non-integer float must not collide with the int of its truncation.
	if i1, _ := mapKeyEncode(IntVal(1)); a == i1 {
		t.Error("1.5 must not encode the same as 1")
	}
}

func TestCompareNumericExact(t *testing.T) {
	if CompareNumeric(IntVal(9007199254740993), IntVal(9007199254740992)) <= 0 {
		t.Error("larger int must compare greater")
	}
	if CompareNumeric(IntVal(1), FloatVal(1.0)) != 0 {
		t.Error("1 and 1.0 must compare equal")
	}
	// 9007199254740993 (int) vs 9007199254740992.0 (float, the int+1 rounds down):
	// exact comparison must say the int is greater, not equal.
	if CompareNumeric(IntVal(9007199254740993), FloatVal(9007199254740992.0)) <= 0 {
		t.Error("exact cross-kind comparison must not lose precision above 2^53")
	}
}
