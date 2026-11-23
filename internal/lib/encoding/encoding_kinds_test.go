// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package encodinglib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.21: malformed input is kind "encoding", a cap stays "limit", and misuse
// (wrong arguments, an unknown format or codec, a string a codec cannot
// encode) stays "runtime" ("").
func TestErrorKinds(t *testing.T) {
	str := interpreter.StringVal
	byt := func(b ...byte) interpreter.Value { return interpreter.BytesVal(b) }
	call := func(fn interpreter.Builtin, args ...interpreter.Value) func() error {
		return func() error { _, err := fn(interpreter.BuiltinCtx{}, args); return err }
	}
	manyFields := strings.Repeat("a=1&", MaxFormFields+1)
	cases := []struct {
		name string
		call func() error
		kind string
	}{
		{"lenRunes invalid UTF-8", call(lenRunesFn, byt(0xff)), "encoding"},
		{"hex odd length", call(fromTextFn, str("hex"), str("abc")), "encoding"},
		{"hex bad digit", call(fromTextFn, str("hex"), str("zz")), "encoding"},
		{"base64 bad char", call(fromTextFn, str("base64"), str("!!!!")), "encoding"},
		{"base64 line break", call(fromTextFn, str("base64"), str("QQ==\n")), "encoding"},
		{"base64 not canonical", call(fromTextFn, str("base64"), str("QR==")), "encoding"},
		{"base32 bad", call(fromTextFn, str("base32"), str("1")), "encoding"},
		{"base64-url bad", call(fromTextFn, str("base64-url"), str("+")), "encoding"},
		{"ascii85 bad", call(fromTextFn, str("ascii85"), str("{")), "encoding"},
		{"z85 length", call(fromTextFn, str("z85"), str("abc")), "encoding"},
		{"z85 char", call(fromTextFn, str("z85"), str("abc d")), "encoding"},
		{"z85 overflow", call(fromTextFn, str("z85"), str("#####")), "encoding"},
		{"quoted-printable bad", call(fromTextFn, str("quoted-printable"), str("a\x01b")), "encoding"},
		{"uri-percent truncated", call(fromTextFn, str("uri-percent"), str("%4")), "encoding"},
		{"uri-percent invalid", call(fromTextFn, str("uri-percent"), str("%zz")), "encoding"},
		{"uri-form invalid", call(fromTextFn, str("uri-form"), str("%zz")), "encoding"},
		{"decode utf-8", call(decodeFn, str("utf-8"), byt(0xc3)), "encoding"},
		{"decode utf-16 odd", call(decodeFn, str("utf-16le"), byt(0x41)), "encoding"},
		{"decode utf-16 surrogate", call(decodeFn, str("utf-16be"), byt(0xdc, 0x00)), "encoding"},
		{"decode utf-32 scalar", call(decodeFn, str("utf-32be"), byt(0, 0x11, 0, 0)), "encoding"},
		{"decode ascii", call(decodeFn, str("ascii"), byt(0x80)), "encoding"},
		{"decode unmapped", call(decodeFn, str("windows-1252"), byt(0x81)), "encoding"},
		{"parseForm escape", call(parseFormFn, str("a=%zz")), "encoding"},
		{"parseMultipart no boundary", call(parseMultipartFn, str("multipart/form-data"), byt()), "encoding"},
		{"parseMultipart truncated", call(parseMultipartFn, str("multipart/form-data; boundary=X"), byt('-', '-', 'X')), "encoding"},
		{"parseForm fields cap", call(parseFormFn, str(manyFields)), "limit"},
		{"fromText unknown format", call(fromTextFn, str("base99"), str("")), ""},
		{"fromText not a string", call(fromTextFn, str("hex"), interpreter.IntVal(1)), ""},
		{"toText z85 length", call(toTextFn, str("z85"), byt(1)), ""},
		{"toText unknown format", call(toTextFn, str("base99"), byt()), ""},
		{"decode unknown codec", call(decodeFn, str("klingon"), byt()), ""},
		{"encode unmappable", call(encodeFn, str("ascii"), str("é")), ""},
		{"lenRunes not bytes", call(lenRunesFn, str("x")), ""},
	}
	for _, c := range cases {
		err := c.call()
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		if got := dataerr.KindOf(err); got != c.kind {
			t.Errorf("%s: %v: kind %q, want %q", c.name, err, got, c.kind)
		}
	}
}
