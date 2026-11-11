// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package jsonlib

import (
	"math"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// A float encodes the way JavaScript and Go's encoding/json print it: plain
// digits between 1e-6 and 1e21, an unpadded exponent outside, and always a
// `.` or exponent so it decodes back as a float.
func TestEncodeFloatForm(t *testing.T) {
	cases := []struct {
		f    float64
		want string
	}{
		{1000000, "1000000.0"},
		{123456789, "123456789.0"},
		{1731300000.5, "1731300000.5"},
		{1e20, "100000000000000000000.0"},
		{1e21, "1e+21"},
		{1.5e300, "1.5e+300"},
		{0.000001, "0.000001"},
		{1e-7, "1e-7"},
		{-2.5e-10, "-2.5e-10"},
		{0, "0.0"},
		{math.Copysign(0, -1), "-0.0"},
		{0.1, "0.1"},
		{-3, "-3.0"},
	}
	for _, c := range cases {
		got := enc(t, interpreter.FloatVal(c.f))
		if got != c.want {
			t.Errorf("encode(%v) = %s, want %s", c.f, got, c.want)
			continue
		}
		back, err := decodeFn([]interpreter.Value{interpreter.StringVal(got)})
		if err != nil {
			t.Errorf("decode(%s): %v", got, err)
			continue
		}
		if back.TypeName() != "float" || math.Float64bits(back.FloatVal()) != math.Float64bits(c.f) {
			t.Errorf("decode(%s) = %s %v, want float %v", got, back.TypeName(), back.FloatVal(), c.f)
		}
	}
}

// Every ASCII byte and a spread of multibyte text survive an encode / decode
// round trip, escaped exactly where the encoder promises.
func TestEncodeStringEveryASCIIByte(t *testing.T) {
	var sb strings.Builder
	for c := 0; c < 0x80; c++ {
		sb.WriteByte(byte(c))
	}
	sb.WriteString("é漢字😀\u2028x\u2029")
	s := sb.String()
	out := enc(t, interpreter.StringVal(s))
	u := `\` + "u" // built up so no tool rewrites a literal escape
	for _, want := range []string{u + "0000", u + "001f", `\n`, `\"`, `\\`, u + "003c", u + "003e", u + "0026", u + "2028", u + "2029", "é漢字😀"} {
		if !strings.Contains(out, want) {
			t.Errorf("encoded text lacks %q: %s", want, out)
		}
	}
	for i := 1; i < len(out)-1; i++ {
		if out[i] < 0x20 {
			t.Fatalf("raw control byte %#x at %d in %q", out[i], i, out)
		}
	}
	back, err := decodeFn([]interpreter.Value{interpreter.StringVal(out)})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if back.Str() != s {
		t.Errorf("round trip changed the string:\n got %q\nwant %q", back.Str(), s)
	}
}

// The decoder's escape path copies plain runs in bulk; escapes at the start,
// the end, back to back and between long runs all decode exactly.
func TestDecodeEscapedStrings(t *testing.T) {
	long := strings.Repeat("abcdefgh", 64)
	u := `\` + "u"
	cases := map[string]string{
		`"` + u + "00e9" + u + "6f22" + u + "d83d" + u + `de00z"`: "\u00e9\u6f22\U0001F600z",
		`"\n"`:                         "\n",
		`"\nabc"`:                      "\nabc",
		`"abc\n"`:                      "abc\n",
		`"a\\\"b"`:                     `a\"b`,
		`"` + long + `\t` + long + `"`: long + "\t" + long,
		`"\/\b\f\r"`:                   "/\b\f\r",
	}
	for in, want := range cases {
		n, err := decodeFn([]interpreter.Value{interpreter.StringVal(in)})
		if err != nil {
			t.Errorf("decode(%s): %v", in, err)
			continue
		}
		if n.Str() != want {
			t.Errorf("decode(%s) = %q, want %q", in, n.Str(), want)
		}
	}
	for _, bad := range []string{"\"a\\nb\x01\"", `"a\nb`, `"a\`, `"\x"`} {
		if _, err := decodeFn([]interpreter.Value{interpreter.StringVal(bad)}); err == nil {
			t.Errorf("decode(%q): expected an error", bad)
		}
	}
}

// An unexpected non-ASCII character is named as the character, not as its
// first byte read as Latin-1.
func TestDecodeErrorNamesTheCharacter(t *testing.T) {
	_, err := decodeFn([]interpreter.Value{interpreter.StringVal("é")})
	if err == nil || !strings.Contains(err.Error(), `"é"`) {
		t.Errorf("error = %v, want it to name \"é\"", err)
	}
}

// RFC 6901: `~` is only ever `~0` or `~1`; anything else is an error, not a
// literal tilde.
func TestPointerRejectsBadTilde(t *testing.T) {
	d := doc(t, `{"a~2": 1, "b~": 2, "~1x": 3}`)
	for _, p := range []string{"/a~2", "/b~", "/~"} {
		if _, err := asIntFn(interpreter.BuiltinCtx{}, []interpreter.Value{d, ptr(p)}); err == nil ||
			!strings.Contains(err.Error(), "must be followed by 0 or 1") {
			t.Errorf("asInt(%s): err = %v, want a tilde error", p, err)
		}
		if _, err := hasFn(interpreter.BuiltinCtx{}, []interpreter.Value{d, ptr(p)}); err == nil {
			t.Errorf("has(%s): want a malformed-pointer error", p)
		}
	}
	// `~01` is `~1` (decode ~0 first would wrongly give `/`).
	if got := call(t, asIntFn, d, ptr("/~01x")); got.Int != 3 {
		t.Errorf("/~01x = %d, want 3", got.Int)
	}
}

// An enum stored with set / list / map takes the form json.encode gives it,
// so building a document and encoding a value agree.
func TestWritesAcceptEnums(t *testing.T) {
	payload := interpreter.EnumVal("", "", "Shape", "Circle", []interpreter.StructField{
		{Name: "r", Value: interpreter.FloatVal(2.5)},
	})
	bare := interpreter.EnumVal("", "", "Shape", "Empty", nil)
	v := call(t, setFn, call(t, mapFn), ptr("/s"), payload)
	v = call(t, setFn, v, ptr("/e"), bare)
	if got := jstr(t, v); got != `{"s":{"Circle":{"r":2.5}},"e":"Empty"}` {
		t.Errorf("set enums = %s", got)
	}
	list := interpreter.ListVal(parser.Type{}, []interpreter.Value{payload, bare})
	if got, want := jstr(t, call(t, listFn, list)), enc(t, list); got != want {
		t.Errorf("json.list(enums) = %s, json.encode = %s", got, want)
	}
}
