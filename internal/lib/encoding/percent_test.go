// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package encodinglib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func TestPercentCodec(t *testing.T) {
	in := interpreter.New()
	Install(in)
	toText := in.LookupNamespacedBuiltin("encoding", "toText")
	fromText := in.LookupNamespacedBuiltin("encoding", "fromText")

	enc := func(s string) string {
		v, err := toText(interpreter.BuiltinCtx{}, []interpreter.Value{
			interpreter.BytesVal([]byte(s)), interpreter.StringVal("uri-percent"),
		})
		if err != nil {
			t.Fatalf("toText(%q): %v", s, err)
		}
		return v.Str
	}
	dec := func(s string) (string, error) {
		v, err := fromText(interpreter.BuiltinCtx{}, []interpreter.Value{
			interpreter.StringVal(s), interpreter.StringVal("uri-percent"),
		})
		if err != nil {
			return "", err
		}
		return string(v.Bytes), nil
	}

	cases := map[string]string{
		"abcXYZ-._~09": "abcXYZ-._~09", // unreserved left literal
		"a b":          "a%20b",        // space -> %20 (not +)
		"&=/?#[]@":     "%26%3D%2F%3F%23%5B%5D%40",
		"a+b":          "a%2Bb",     // + is escaped (RFC 3986), not a space
		"café":         "caf%C3%A9", // multi-byte UTF-8
	}
	for in, want := range cases {
		if got := enc(in); got != want {
			t.Errorf("encode(%q) = %q, want %q", in, got, want)
		}
		back, err := dec(want)
		if err != nil || back != in {
			t.Errorf("decode(%q) = %q, err %v; want %q", want, back, err, in)
		}
	}

	// A literal + decodes to +, never a space (strict RFC 3986).
	if got, _ := dec("a+b"); got != "a+b" {
		t.Errorf("decode(a+b) = %q, want a+b (RFC 3986 leaves + literal)", got)
	}
	// Malformed escapes are errors.
	for _, bad := range []string{"%", "%2", "%zz", "%2g", "abc%"} {
		if _, err := dec(bad); err == nil {
			t.Errorf("decode(%q) should error on a malformed escape", bad)
		}
	}
}

func TestFormCodec(t *testing.T) {
	in := interpreter.New()
	Install(in)
	toText := in.LookupNamespacedBuiltin("encoding", "toText")
	fromText := in.LookupNamespacedBuiltin("encoding", "fromText")

	enc := func(s string) string {
		v, _ := toText(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.BytesVal([]byte(s)), interpreter.StringVal("uri-form")})
		return v.Str
	}
	dec := func(s string) (string, error) {
		v, err := fromText(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(s), interpreter.StringVal("uri-form")})
		if err != nil {
			return "", err
		}
		return string(v.Bytes), nil
	}

	// Space becomes "+" (the form difference from percent); "+" round-trips
	// through "%2B".
	if got := enc("a b+c"); got != "a+b%2Bc" {
		t.Errorf("form encode(\"a b+c\") = %q, want \"a+b%%2Bc\"", got)
	}
	if got, _ := dec("a+b%2Bc"); got != "a b+c" {
		t.Errorf("form decode round-trip failed: %q", got)
	}
	// "+" decodes to a space (the whole point).
	if got, _ := dec("hello+world"); got != "hello world" {
		t.Errorf("form decode(\"hello+world\") = %q, want \"hello world\"", got)
	}
	// Reserved bytes still escape the same as percent.
	if got := enc("&=/"); got != "%26%3D%2F" {
		t.Errorf("form encode(\"&=/\") = %q", got)
	}
	if _, err := dec("bad%zz"); err == nil {
		t.Error("form decode should reject a malformed escape")
	}
}
