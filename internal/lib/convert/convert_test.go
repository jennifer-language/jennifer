// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package convert

import (
	"math"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// convert.toInt truncates a float toward zero but must reject the values int64
// cannot hold (NaN, +/-Inf, out of range) rather than returning garbage from a
// bare int64(f) cast - convert is canonical-only.
func TestToIntRejectsUnrepresentableFloats(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 1e300, -1e300, 9223372036854775808.0} {
		if _, err := toIntFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.FloatVal(v)}); err == nil {
			t.Errorf("toInt(%g) should error, got nil", v)
		}
	}
	// In-range truncation still works.
	if r, err := toIntFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.FloatVal(3.9)}); err != nil || r.Int != 3 {
		t.Errorf("toInt(3.9) = %+v, err %v; want 3", r, err)
	}
}

// toFloat must reject the non-finite spellings strconv.ParseFloat accepts
// ("NaN", "Inf", "Infinity"): Jennifer's float model forbids those values.
func TestToFloatRejectsNonFinite(t *testing.T) {
	for _, s := range []string{"NaN", "nan", "Inf", "inf", "+Inf", "-Inf", "Infinity"} {
		if _, err := toFloatFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(s)}); err == nil {
			t.Errorf("toFloat(%q) should error", s)
		}
	}
	// A normal float string still parses.
	if v, err := toFloatFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("3.5")}); err != nil || v.Float != 3.5 {
		t.Errorf("toFloat(\"3.5\") = %+v, err %v", v, err)
	}
}

// callFn invokes a convert builtin directly. ok asserts success and the returned
// value; !ok asserts an error.
func mustVal(t *testing.T, fn func(interpreter.BuiltinCtx, []interpreter.Value) (interpreter.Value, error), args ...interpreter.Value) interpreter.Value {
	t.Helper()
	v, err := fn(interpreter.BuiltinCtx{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return v
}

func mustErr(t *testing.T, label string, fn func(interpreter.BuiltinCtx, []interpreter.Value) (interpreter.Value, error), args ...interpreter.Value) {
	t.Helper()
	if _, err := fn(interpreter.BuiltinCtx{}, args); err == nil {
		t.Errorf("%s: expected an error, got nil", label)
	}
}

func TestToIntAcrossKinds(t *testing.T) {
	I, F, S, B := interpreter.IntVal, interpreter.FloatVal, interpreter.StringVal, interpreter.BoolVal
	if v := mustVal(t, toIntFn, I(7)); v.Int != 7 {
		t.Errorf("toInt(int 7) = %d", v.Int)
	}
	if v := mustVal(t, toIntFn, F(-3.9)); v.Int != -3 { // truncate toward zero
		t.Errorf("toInt(-3.9) = %d, want -3", v.Int)
	}
	if v := mustVal(t, toIntFn, S("-42")); v.Int != -42 {
		t.Errorf("toInt(\"-42\") = %d", v.Int)
	}
	if v := mustVal(t, toIntFn, B(true)); v.Int != 1 {
		t.Errorf("toInt(true) = %d, want 1", v.Int)
	}
	if v := mustVal(t, toIntFn, B(false)); v.Int != 0 {
		t.Errorf("toInt(false) = %d, want 0", v.Int)
	}
	mustErr(t, "toInt(\"abc\")", toIntFn, S("abc"))
	mustErr(t, "toInt(\"3.5\")", toIntFn, S("3.5")) // not an integer string
	mustErr(t, "toInt(null)", toIntFn, interpreter.Null())
	mustErr(t, "toInt() arity", toIntFn)
}

func TestToFloatAcrossKinds(t *testing.T) {
	I, F, S, B := interpreter.IntVal, interpreter.FloatVal, interpreter.StringVal, interpreter.BoolVal
	if v := mustVal(t, toFloatFn, I(5)); v.Float != 5.0 {
		t.Errorf("toFloat(int 5) = %g", v.Float)
	}
	if v := mustVal(t, toFloatFn, F(2.5)); v.Float != 2.5 {
		t.Errorf("toFloat(2.5) = %g", v.Float)
	}
	if v := mustVal(t, toFloatFn, B(true)); v.Float != 1.0 {
		t.Errorf("toFloat(true) = %g", v.Float)
	}
	mustErr(t, "toFloat(\"x\")", toFloatFn, S("x"))
	mustErr(t, "toFloat(null)", toFloatFn, interpreter.Null())
}

func TestToString(t *testing.T) {
	cases := []struct {
		in   interpreter.Value
		want string
	}{
		{interpreter.IntVal(42), "42"},
		{interpreter.BoolVal(true), "true"},
		{interpreter.BoolVal(false), "false"},
		{interpreter.Null(), "null"},
		{interpreter.StringVal("hi"), "hi"},
		{interpreter.FloatVal(2.5), "2.5"},
	}
	for _, c := range cases {
		if v := mustVal(t, toStringFn, c.in); v.Kind != interpreter.KindString || v.Str != c.want {
			t.Errorf("toString(%+v) = %q, want %q", c.in, v.Str, c.want)
		}
	}
	mustErr(t, "toString() arity", toStringFn)
}

func TestToBoolCanonicalOnly(t *testing.T) {
	I, F, S, B := interpreter.IntVal, interpreter.FloatVal, interpreter.StringVal, interpreter.BoolVal
	if v := mustVal(t, toBoolFn, B(true)); !v.Bool {
		t.Error("toBool(true) should be true")
	}
	if v := mustVal(t, toBoolFn, I(0)); v.Bool {
		t.Error("toBool(0) should be false")
	}
	if v := mustVal(t, toBoolFn, I(1)); !v.Bool {
		t.Error("toBool(1) should be true")
	}
	if v := mustVal(t, toBoolFn, F(0.0)); v.Bool {
		t.Error("toBool(0.0) should be false")
	}
	if v := mustVal(t, toBoolFn, S("false")); v.Bool {
		t.Error("toBool(\"false\") should be false")
	}
	// Non-canonical inputs error (no truthiness coercion).
	mustErr(t, "toBool(2)", toBoolFn, I(2))
	mustErr(t, "toBool(0.5)", toBoolFn, F(0.5))
	mustErr(t, "toBool(\"yes\")", toBoolFn, S("yes"))
	mustErr(t, "toBool(null)", toBoolFn, interpreter.Null())
}

func TestTypeOf(t *testing.T) {
	cases := []struct {
		in   interpreter.Value
		want string
	}{
		{interpreter.IntVal(1), "int"},
		{interpreter.FloatVal(1), "float"},
		{interpreter.StringVal(""), "string"},
		{interpreter.BoolVal(true), "bool"},
		{interpreter.Null(), "null"},
		{interpreter.BytesVal(nil), "bytes"},
	}
	for _, c := range cases {
		if v := mustVal(t, typeOfFn, c.in); v.Str != c.want {
			t.Errorf("typeOf(%+v) = %q, want %q", c.in, v.Str, c.want)
		}
	}
}

func TestObjectType(t *testing.T) {
	obj := interpreter.ObjectVal("json", "Value", interpreter.Null())
	if v := mustVal(t, objectTypeFn, obj); v.Str != "json.Value" {
		t.Errorf("objectType(json.Value) = %q", v.Str)
	}
	// typeOf of the same object reports the generic kind.
	if v := mustVal(t, typeOfFn, obj); v.Str != "object" {
		t.Errorf("typeOf(object) = %q, want \"object\"", v.Str)
	}
	mustErr(t, "objectType(int)", objectTypeFn, interpreter.IntVal(1))
}

func TestBytesFromString(t *testing.T) {
	// "h\u00e9" = 0x68, then U+00E9 as 0xC3 0xA9.
	v := mustVal(t, bytesFromStringFn, interpreter.StringVal("h\u00e9"), interpreter.StringVal("utf-8"))
	if v.Kind != interpreter.KindBytes {
		t.Fatalf("kind = %s, want bytes", v.Kind)
	}
	want := []byte{0x68, 0xC3, 0xA9}
	if string(v.Bytes) != string(want) {
		t.Errorf("bytes = % x, want % x", v.Bytes, want)
	}
	mustErr(t, "bad codec", bytesFromStringFn, interpreter.StringVal("x"), interpreter.StringVal("ascii"))
	mustErr(t, "non-string value", bytesFromStringFn, interpreter.IntVal(1), interpreter.StringVal("utf-8"))
	mustErr(t, "arity", bytesFromStringFn, interpreter.StringVal("x"))
}

func TestStringFromBytesRoundTrip(t *testing.T) {
	orig := "h\u00e9llo \u4e16\u754c"
	b := mustVal(t, bytesFromStringFn, interpreter.StringVal(orig), interpreter.StringVal("utf-8"))
	s := mustVal(t, stringFromBytesFn, b, interpreter.StringVal("utf-8"))
	if s.Str != orig {
		t.Errorf("round-trip = %q, want %q", s.Str, orig)
	}
	mustErr(t, "bad codec", stringFromBytesFn, interpreter.BytesVal([]byte{0x41}), interpreter.StringVal("ascii"))
	mustErr(t, "non-bytes", stringFromBytesFn, interpreter.StringVal("x"), interpreter.StringVal("utf-8"))
}

// TestStringFromBytesRejectsInvalidUTF8 exercises the hand-rolled validator:
// overlong encodings, surrogates, out-of-range lead bytes, stray continuations,
// and truncated sequences must all be rejected; valid multi-byte forms pass.
func TestStringFromBytesRejectsInvalidUTF8(t *testing.T) {
	invalid := map[string][]byte{
		"lone continuation":   {0x80},
		"lone 0xFF":           {0xFF},
		"overlong 2-byte":     {0xC0, 0x80},
		"overlong 3-byte":     {0xE0, 0x80, 0x80},
		"surrogate U+D800":    {0xED, 0xA0, 0x80},
		"truncated 3-byte":    {0xE2, 0x82},
		"truncated 4-byte":    {0xF0, 0x9F, 0x98},
		"out-of-range 0xF5":   {0xF5, 0x80, 0x80, 0x80},
		"overlong 4-byte":     {0xF0, 0x80, 0x80, 0x80},
		"bad continuation":    {0xC3, 0x28},
		"five-byte lead 0xF8": {0xF8, 0x80, 0x80, 0x80, 0x80},
	}
	for name, b := range invalid {
		if _, err := stringFromBytesFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.BytesVal(b), interpreter.StringVal("utf-8")}); err == nil {
			t.Errorf("stringFromBytes(%s) should reject invalid UTF-8", name)
		}
	}
	valid := map[string][]byte{
		"ascii":  {0x41},
		"2-byte": {0xC3, 0xA9},       // é
		"3-byte": {0xE4, 0xB8, 0x96}, // 世
		"4-byte": {0xF0, 0x9F, 0x98, 0x80},
		"empty":  {},
	}
	for name, b := range valid {
		if _, err := stringFromBytesFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.BytesVal(b), interpreter.StringVal("utf-8")}); err != nil {
			t.Errorf("stringFromBytes(%s) should accept valid UTF-8: %v", name, err)
		}
	}
}

func TestCodepointRoundTrip(t *testing.T) {
	// toCodepoint of a one-rune string, and its inverse.
	for _, c := range []struct {
		s string
		n int64
	}{{"A", 65}, {"\u00e9", 0xE9}, {"\u20ac", 0x20AC}, {"\U0001F600", 0x1F600}} {
		if v := mustVal(t, toCodepointFn, interpreter.StringVal(c.s)); v.Int != c.n {
			t.Errorf("toCodepoint(%q) = %d, want %d", c.s, v.Int, c.n)
		}
		if v := mustVal(t, fromCodepointFn, interpreter.IntVal(c.n)); v.Str != c.s {
			t.Errorf("fromCodepoint(%d) = %q, want %q", c.n, v.Str, c.s)
		}
	}
	// toCodepoint requires exactly one code point.
	mustErr(t, "two runes", toCodepointFn, interpreter.StringVal("ab"))
	mustErr(t, "empty", toCodepointFn, interpreter.StringVal(""))
	mustErr(t, "NFD é (2 code points)", toCodepointFn, interpreter.StringVal("é"))
	mustErr(t, "non-string", toCodepointFn, interpreter.IntVal(65))
	// fromCodepoint rejects negatives, surrogates, and out-of-range values.
	mustErr(t, "negative", fromCodepointFn, interpreter.IntVal(-1))
	mustErr(t, "surrogate", fromCodepointFn, interpreter.IntVal(0xD800))
	mustErr(t, "above MaxRune", fromCodepointFn, interpreter.IntVal(0x110000))
	mustErr(t, "non-int", fromCodepointFn, interpreter.StringVal("A"))
}

func TestInstallRegistersEveryConvertBuiltin(t *testing.T) {
	in := interpreter.New()
	Install(in)
	for _, name := range []string{"toInt", "toFloat", "toString", "toBool", "typeOf", "objectType", "bytesFromString", "stringFromBytes", "toCodepoint", "fromCodepoint"} {
		if in.LookupNamespacedBuiltin("convert", name) == nil {
			t.Errorf("convert.%s is not registered", name)
		}
	}
}
