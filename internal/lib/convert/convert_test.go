// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package convert

import (
	"math"
	"strings"
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
	mustErr(t, "toInt(\"3.5\")", toIntFn, S("3.5"))   // not an integer string
	mustErr(t, "toInt(\"0xff\")", toIntFn, S("0xff")) // one-arg is decimal; a prefix errors
	// ... with a hint pointing at the radix form.
	if _, err := toIntFn(interpreter.BuiltinCtx{}, []interpreter.Value{S("0xff")}); err == nil || !strings.Contains(err.Error(), "toInt(s, radix)") {
		t.Errorf("toInt(\"0xff\") should hint at the radix form, got %v", err)
	}
	mustErr(t, "toInt(null)", toIntFn, interpreter.Null())
	mustErr(t, "toInt() arity", toIntFn)
}

func TestToIntRadix(t *testing.T) {
	I, S := interpreter.IntVal, interpreter.StringVal
	// Bases 2 / 8 / 10 / 16 parse bare digits; a sign is allowed.
	for _, c := range []struct {
		s    string
		base int64
		want int64
	}{
		{"41", 16, 65}, {"ff", 16, 255}, {"-ff", 16, -255},
		{"17", 8, 15}, {"101", 2, 5}, {"42", 10, 42},
	} {
		if v := mustVal(t, toIntFn, S(c.s), I(c.base)); v.Int != c.want {
			t.Errorf("toInt(%q, %d) = %d, want %d", c.s, c.base, v.Int, c.want)
		}
	}
	// Errors: bad digit for the base, an unsupported radix, a radix on a non-string,
	// and a `0x`-prefixed string (prefixes are not accepted - strip them first).
	mustErr(t, "toInt(\"zz\",16)", toIntFn, S("zz"), I(16))
	mustErr(t, "toInt(\"10\",7)", toIntFn, S("10"), I(7))
	mustErr(t, "toInt(\"10\",0)", toIntFn, S("10"), I(0))
	mustErr(t, "toInt(int,16)", toIntFn, I(255), I(16))
	mustErr(t, "toInt(\"0x41\",16)", toIntFn, S("0x41"), I(16))
	// A prefixed string errors with an actionable hint (not a bare "invalid"),
	// since 0x/0o/0b is never accepted under an explicit radix.
	if _, err := toIntFn(interpreter.BuiltinCtx{}, []interpreter.Value{S("0x41"), I(16)}); err == nil || !strings.Contains(err.Error(), "prefix is not accepted") {
		t.Errorf("toInt(\"0x41\", 16) should hint about the prefix, got %v", err)
	}
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

// TestToStringRadixRemoved: the radix form went in 0.28.0 (M29.35); its error
// names the printf form to write. toInt still parses a radix.
func TestToStringRadixRemoved(t *testing.T) {
	I, S := interpreter.IntVal, interpreter.StringVal
	_, err := toStringFn(interpreter.BuiltinCtx{}, []interpreter.Value{I(204), I(16)})
	if err == nil || !strings.Contains(err.Error(), `io.sprintf("%d|base=16", n)`) {
		t.Errorf("toString(204, 16): want the removal error, got %v", err)
	}
	if v := mustVal(t, toIntFn, S("cc"), I(16)); v.Int != 204 {
		t.Errorf("toInt(\"cc\", 16) = %d, want 204", v.Int)
	}
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
	for _, name := range []string{"toInt", "toFloat", "toString", "toBool", "typeOf", "objectType", "toCodepoint", "fromCodepoint"} {
		if in.LookupNamespacedBuiltin("convert", name) == nil {
			t.Errorf("convert.%s is not registered", name)
		}
	}
}

// TestToFloatDecimalOnly: toFloat accepts the decimal form only, like toInt.
// Before the fix strconv.ParseFloat's Go literal forms got through:
// "1_000" was 1000.0 and "0x1p4" 16.0.
func TestToFloatDecimalOnly(t *testing.T) {
	ok := map[string]float64{
		"1": 1, "-2.5": -2.5, "+3": 3, "1.": 1, ".5": 0.5, "1e3": 1000, "1E-2": 0.01,
		"-1.5e+2": -150, "007": 7, "0.0": 0,
	}
	for s, want := range ok {
		v, err := toFloatFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(s)})
		if err != nil || v.Float != want {
			t.Errorf("toFloat(%q) = %v, %v; want %g", s, v.Float, err, want)
		}
	}
	for _, s := range []string{
		"1_000", "1_0.5", "0x1p4", "0x10", "0X1P-2", "0b1", "0o7", "", ".", "+", "-.",
		"e5", "1e", "1e+", " 1", "1 ", "1.2.3", "１", "Infinity", "nan",
	} {
		if _, err := toFloatFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(s)}); err == nil {
			t.Errorf("toFloat(%q) should error", s)
		}
	}
}

// TestToCodepointInvalidUTF8: a lone invalid byte is an error. Before the fix
// toCodepoint("\xff") returned 65533 (U+FFFD), a character the input lacks.
func TestToCodepointInvalidUTF8(t *testing.T) {
	for _, s := range []string{"\xff", "\x80", "\xc3"} {
		if v, err := toCodepointFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(s)}); err == nil {
			t.Errorf("toCodepoint(%q) = %d, want an error", s, v.Int)
		}
	}
	if v, err := toCodepointFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("\uFFFD")}); err != nil || v.Int != 0xFFFD {
		t.Errorf("toCodepoint(U+FFFD) = %v, %v", v.Int, err)
	}
}

// TestMessagePrefix pins M29.52: every convert error starts with "convert.fn".
func TestMessagePrefix(t *testing.T) {
	type fn = func(interpreter.BuiltinCtx, []interpreter.Value) (interpreter.Value, error)
	s, i, f := interpreter.StringVal, interpreter.IntVal, interpreter.FloatVal
	cases := []struct {
		name string
		fn   fn
		args []interpreter.Value
	}{
		{"toCodepoint", toCodepointFn, []interpreter.Value{s("ab")}},
		{"fromCodepoint", fromCodepointFn, []interpreter.Value{i(-1)}},
		{"toInt", toIntFn, nil},
		{"toInt", toIntFn, []interpreter.Value{s("abc")}},
		{"toInt", toIntFn, []interpreter.Value{s("0x1f")}},
		{"toInt", toIntFn, []interpreter.Value{s("zz"), i(16)}},
		{"toInt", toIntFn, []interpreter.Value{s("0x1f"), i(16)}},
		{"toInt", toIntFn, []interpreter.Value{s("1"), i(3)}},
		{"toInt", toIntFn, []interpreter.Value{f(1e300)}},
		{"toFloat", toFloatFn, []interpreter.Value{s("x")}},
		{"toFloat", toFloatFn, []interpreter.Value{s("NaN")}},
		{"toString", toStringFn, nil},
		{"toBool", toBoolFn, []interpreter.Value{i(2)}},
		{"toBool", toBoolFn, []interpreter.Value{f(0.5)}},
		{"toBool", toBoolFn, []interpreter.Value{s("yes")}},
		{"typeOf", typeOfFn, nil},
		{"objectType", objectTypeFn, []interpreter.Value{i(1)}},
	}
	for _, c := range cases {
		_, err := c.fn(interpreter.BuiltinCtx{}, c.args)
		if err == nil || !strings.HasPrefix(err.Error(), "convert."+c.name) {
			t.Errorf("%s%v: got %v, want prefix convert.%s", c.name, c.args, err, c.name)
		}
	}
}
