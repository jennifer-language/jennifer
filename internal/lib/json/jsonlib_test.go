// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package jsonlib

import (
	"math"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func enc(t *testing.T, v interpreter.Value) string {
	t.Helper()
	out, err := encodeFn([]interpreter.Value{v}, false)
	if err != nil {
		t.Fatalf("encode(%v): %v", v, err)
	}
	return out.Str
}

func dec(t *testing.T, s string) interpreter.Value {
	t.Helper()
	out, err := decodeFn([]interpreter.Value{interpreter.StringVal(s)})
	if err != nil {
		t.Fatalf("decode(%q): %v", s, err)
	}
	return out
}

func TestEncodeScalars(t *testing.T) {
	cases := []struct {
		v    interpreter.Value
		want string
	}{
		{interpreter.Null(), "null"},
		{interpreter.BoolVal(true), "true"},
		{interpreter.BoolVal(false), "false"},
		{interpreter.IntVal(42), "42"},
		{interpreter.IntVal(-7), "-7"},
		{interpreter.FloatVal(3.5), "3.5"},
		{interpreter.FloatVal(2.0), "2.0"}, // float keeps a decimal so it round-trips as float
		{interpreter.StringVal("hi\n\"x\""), `"hi\n\"x\""`},
		{interpreter.BytesVal([]byte("hi")), `"aGk="`},
	}
	for _, c := range cases {
		if got := enc(t, c.v); got != c.want {
			t.Errorf("encode(%v) = %q, want %q", c.v, got, c.want)
		}
	}
}

func TestEncodeCompound(t *testing.T) {
	list := interpreter.ListVal(parser.PrimitiveType(parser.TypeInt), []interpreter.Value{
		interpreter.IntVal(1), interpreter.IntVal(2),
	})
	if got := enc(t, list); got != "[1,2]" {
		t.Errorf("list = %q", got)
	}
	m := interpreter.MapVal(parser.PrimitiveType(parser.TypeString), parser.PrimitiveType(parser.TypeInt),
		[]interpreter.MapEntry{
			{Key: interpreter.StringVal("a"), Value: interpreter.IntVal(1)},
			{Key: interpreter.StringVal("b"), Value: interpreter.IntVal(2)},
		})
	if got := enc(t, m); got != `{"a":1,"b":2}` {
		t.Errorf("map = %q", got)
	}
	st := interpreter.StructVal("Point", []interpreter.StructField{
		{Name: "x", Value: interpreter.IntVal(3)},
		{Name: "y", Value: interpreter.IntVal(4)},
	})
	if got := enc(t, st); got != `{"x":3,"y":4}` {
		t.Errorf("struct = %q", got)
	}
}

func TestEncodePretty(t *testing.T) {
	m := interpreter.MapVal(parser.PrimitiveType(parser.TypeString), parser.PrimitiveType(parser.TypeInt),
		[]interpreter.MapEntry{{Key: interpreter.StringVal("a"), Value: interpreter.IntVal(1)}})
	out, err := encodeFn([]interpreter.Value{m}, true)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": 1\n}"
	if out.Str != want {
		t.Errorf("pretty = %q, want %q", out.Str, want)
	}
	// empty collections stay compact even when pretty
	empty := interpreter.ListVal(parser.PrimitiveType(parser.TypeInt), nil)
	got, _ := encodeFn([]interpreter.Value{empty}, true)
	if got.Str != "[]" {
		t.Errorf("empty pretty list = %q, want []", got.Str)
	}
}

func TestEncodeErrors(t *testing.T) {
	// non-string map key
	badKey := interpreter.MapVal(parser.PrimitiveType(parser.TypeInt), parser.PrimitiveType(parser.TypeInt),
		[]interpreter.MapEntry{{Key: interpreter.IntVal(1), Value: interpreter.IntVal(2)}})
	if _, err := encodeFn([]interpreter.Value{badKey}, false); err == nil {
		t.Error("expected error for non-string map key")
	}
	// non-finite float
	if _, err := encodeFn([]interpreter.Value{interpreter.FloatVal(math.Inf(1))}, false); err == nil {
		t.Error("expected error for non-finite float")
	}
	// arity
	if _, err := encodeFn(nil, false); err == nil {
		t.Error("expected arity error")
	}
}

func TestDecodeScalars(t *testing.T) {
	if v := dec(t, "null"); v.Kind != interpreter.KindNull {
		t.Errorf("null -> %v", v.Kind)
	}
	if v := dec(t, "true"); v.Kind != interpreter.KindBool || !v.Bool {
		t.Errorf("true -> %v", v)
	}
	if v := dec(t, "42"); v.Kind != interpreter.KindInt || v.Int != 42 {
		t.Errorf("42 -> %v", v)
	}
	if v := dec(t, "-7"); v.Kind != interpreter.KindInt || v.Int != -7 {
		t.Errorf("-7 -> %v", v)
	}
	// integral vs non-integral
	for _, s := range []string{"3.14", "1.0", "1e3", "2E2"} {
		if v := dec(t, s); v.Kind != interpreter.KindFloat {
			t.Errorf("%s -> %v, want float", s, v.Kind)
		}
	}
	if v := dec(t, `"a\nb"`); v.Kind != interpreter.KindString || v.Str != "a\nb" {
		t.Errorf("escaped string -> %q", v.Str)
	}
	// \u escape + surrogate pair (emoji)
	if v := dec(t, `"A😀"`); v.Str != "A\U0001F600" {
		t.Errorf("unicode escapes -> %q", v.Str)
	}
}

func TestDecodeCompound(t *testing.T) {
	v := dec(t, `[1, 2, 3]`)
	if v.Kind != interpreter.KindList || len(v.List) != 3 || v.List[0].Int != 1 {
		t.Fatalf("array -> %v", v)
	}
	o := dec(t, `{"a": 1, "b": [true, null]}`)
	if o.Kind != interpreter.KindMap || len(o.Map) != 2 {
		t.Fatalf("object -> %v", o)
	}
	if o.Map[0].Key.Str != "a" || o.Map[0].Value.Int != 1 {
		t.Errorf("object[a] wrong: %v", o.Map[0])
	}
	// last-wins on duplicate keys
	d := dec(t, `{"k": 1, "k": 2}`)
	if len(d.Map) != 1 || d.Map[0].Value.Int != 2 {
		t.Errorf("dup keys -> %v, want single k=2", d)
	}
}

func TestDecodeErrors(t *testing.T) {
	for _, s := range []string{"", "{bad}", "[1,]", `{"a": }`, "truue", "12x", `"unterminated`, "[1] 2"} {
		if _, err := decodeFn([]interpreter.Value{interpreter.StringVal(s)}); err == nil {
			t.Errorf("decode(%q) should error", s)
		}
	}
	// position is reported
	_, err := decodeFn([]interpreter.Value{interpreter.StringVal("\n  }")})
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("expected a line-2 position, got %v", err)
	}
}

// Nesting beyond the decoder's depth cap must surface as a normal, catchable
// decode error. Unbounded recursion exhausts the Go stack, which is fatal and
// uncatchable - a remote DoS wherever untrusted JSON is decoded (web.bodyJson).
func TestDecodeDepthCap(t *testing.T) {
	deepArr := strings.Repeat("[", maxNestingDepth+1) + strings.Repeat("]", maxNestingDepth+1)
	if _, err := decodeFn([]interpreter.Value{interpreter.StringVal(deepArr)}); err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Errorf("deep array: expected nesting-depth error, got %v", err)
	}
	deepObj := strings.Repeat(`{"k":`, maxNestingDepth+1) + "1" + strings.Repeat("}", maxNestingDepth+1)
	if _, err := decodeFn([]interpreter.Value{interpreter.StringVal(deepObj)}); err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Errorf("deep object: expected nesting-depth error, got %v", err)
	}
	// A document exactly at the cap still decodes.
	okArr := strings.Repeat("[", maxNestingDepth) + strings.Repeat("]", maxNestingDepth)
	if _, err := decodeFn([]interpreter.Value{interpreter.StringVal(okArr)}); err != nil {
		t.Errorf("depth-%d document should decode, got %v", maxNestingDepth, err)
	}
	// Truncated deep input (openers only, the cheap-to-build attack shape)
	// must error normally instead of crashing the process.
	if _, err := decodeFn([]interpreter.Value{interpreter.StringVal(strings.Repeat("[", 5_000_000))}); err == nil {
		t.Error("truncated deep input: expected error, got nil")
	}
}

func TestRoundTrip(t *testing.T) {
	// decode(encode(v)) preserves kind for each value kind with a JSON image
	vals := []interpreter.Value{
		interpreter.Null(), interpreter.BoolVal(true), interpreter.IntVal(5),
		interpreter.FloatVal(2.0), interpreter.StringVal("hi"),
		interpreter.ListVal(parser.PrimitiveType(parser.TypeInt), []interpreter.Value{interpreter.IntVal(1)}),
	}
	for _, v := range vals {
		s := enc(t, v)
		back := dec(t, s)
		if back.Kind != v.Kind {
			t.Errorf("round-trip %q: kind %v -> %v", s, v.Kind, back.Kind)
		}
	}
	// text round-trip: decode then re-encode a nested document
	in := `{"nums":[1,2.5],"ok":true,"s":"x"}`
	if out := enc(t, dec(t, in)); out != in {
		t.Errorf("text round-trip: %q -> %q", in, out)
	}
}
