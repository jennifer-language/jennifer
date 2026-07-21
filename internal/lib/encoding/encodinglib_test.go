// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package encodinglib

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// callFn invokes a builtin with the given Values, returning the
// result Value. Failure shape: t.Fatal so individual cases stay tidy.
func callFn(t *testing.T, fn interpreter.Builtin, args ...interpreter.Value) interpreter.Value {
	t.Helper()
	v, err := fn(interpreter.BuiltinCtx{}, args)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	return v
}

// expectErr invokes a builtin expecting an error and returns the
// error message for substring checks.
func expectErr(t *testing.T, fn interpreter.Builtin, args ...interpreter.Value) string {
	t.Helper()
	_, err := fn(interpreter.BuiltinCtx{}, args)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	return err.Error()
}

// ----- introspection ------------------------------------------------

func TestIsAscii(t *testing.T) {
	if v := callFn(t, isAsciiFn, interpreter.BytesVal([]byte("hello"))); v.Bool != true {
		t.Errorf(`isAscii("hello") = %v, want true`, v.Bool)
	}
	if v := callFn(t, isAsciiFn, interpreter.BytesVal([]byte{0x80})); v.Bool != false {
		t.Errorf(`isAscii(0x80) = %v, want false`, v.Bool)
	}
	if v := callFn(t, isAsciiFn, interpreter.BytesVal(nil)); v.Bool != true {
		t.Errorf(`isAscii(empty) = %v, want true`, v.Bool)
	}
}

func TestLenBytesVsLenRunes(t *testing.T) {
	// "café" is 5 UTF-8 bytes (c, a, f, 0xC3 0xA9) but 4 runes.
	bytesVal := callFn(t, lenBytesFn, interpreter.StringVal("café"))
	if bytesVal.Int != 5 {
		t.Errorf(`lenBytes("café") = %d, want 5`, bytesVal.Int)
	}
	runesVal := callFn(t, lenRunesFn, interpreter.BytesVal([]byte("café")))
	if runesVal.Int != 4 {
		t.Errorf(`lenRunes("café".bytes) = %d, want 4`, runesVal.Int)
	}
}

func TestLenRunesRejectsInvalidUTF8(t *testing.T) {
	msg := expectErr(t, lenRunesFn, interpreter.BytesVal([]byte{0xFF, 0xFE}))
	if !strings.Contains(msg, "valid UTF-8") {
		t.Errorf("err does not mention UTF-8 validity: %v", msg)
	}
}

// ----- toText / fromText (hex, base64, base64-url) ------------------

func TestToTextHexRoundTrip(t *testing.T) {
	enc := callFn(t, toTextFn, interpreter.BytesVal([]byte{0xDE, 0xAD, 0xBE, 0xEF}), interpreter.StringVal("hex"))
	if enc.Str != "deadbeef" {
		t.Errorf("hex got %q, want %q", enc.Str, "deadbeef")
	}
	dec := callFn(t, fromTextFn, interpreter.StringVal("DEADBEEF"), interpreter.StringVal("hex"))
	if !bytes.Equal(dec.Bytes, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Errorf("fromText hex (uppercase) = %x, want deadbeef", dec.Bytes)
	}
}

func TestToTextEmptyRoundTrip(t *testing.T) {
	for _, fmtName := range []string{"hex", "base64", "base64-url", "quoted-printable"} {
		enc := callFn(t, toTextFn, interpreter.BytesVal(nil), interpreter.StringVal(fmtName))
		if enc.Str != "" {
			t.Errorf("toText(empty, %q) = %q, want empty", fmtName, enc.Str)
		}
		dec := callFn(t, fromTextFn, interpreter.StringVal(""), interpreter.StringVal(fmtName))
		if len(dec.Bytes) != 0 {
			t.Errorf("fromText(empty, %q) = %x, want empty", fmtName, dec.Bytes)
		}
	}
}

func TestBase32Ascii85Z85RoundTrip(t *testing.T) {
	src := []byte("Hello, World") // 12 bytes - also a valid z85 length (multiple of 4)
	for _, f := range []string{"base32", "base32-hex", "ascii85", "z85"} {
		enc := callFn(t, toTextFn, interpreter.BytesVal(src), interpreter.StringVal(f))
		dec := callFn(t, fromTextFn, interpreter.StringVal(enc.Str), interpreter.StringVal(f))
		if string(dec.Bytes) != string(src) {
			t.Errorf("%s round-trip: got %q, want %q", f, dec.Bytes, src)
		}
	}
}

func TestZ85CanonicalVector(t *testing.T) {
	// The RFC 32 canonical test vector.
	in := []byte{0x86, 0x4F, 0xD2, 0x6F, 0xB5, 0x59, 0xF7, 0x5B}
	enc := callFn(t, toTextFn, interpreter.BytesVal(in), interpreter.StringVal("z85"))
	if enc.Str != "HelloWorld" {
		t.Errorf("z85(RFC 32 vector) = %q, want %q", enc.Str, "HelloWorld")
	}
	dec := callFn(t, fromTextFn, interpreter.StringVal("HelloWorld"), interpreter.StringVal("z85"))
	if !bytes.Equal(dec.Bytes, in) {
		t.Errorf("z85 decode = % x, want % x", dec.Bytes, in)
	}
}

func TestZ85Errors(t *testing.T) {
	// encode: input length must be a multiple of 4
	if _, err := toTextFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.BytesVal([]byte("abc")), interpreter.StringVal("z85")}); err == nil {
		t.Error("z85 encode of 3 bytes should error")
	}
	// decode: input length must be a multiple of 5
	if _, err := fromTextFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("abcd"), interpreter.StringVal("z85")}); err == nil {
		t.Error("z85 decode of 4 chars should error")
	}
	// decode: character outside the z85 alphabet (space)
	if _, err := fromTextFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("Hello Worl"), interpreter.StringVal("z85")}); err == nil {
		t.Error("z85 decode with an invalid character should error")
	}
}

func TestToTextQuotedPrintable(t *testing.T) {
	// RFC 2045: `=` -> `=3D`, printable ASCII stays literal, 8-bit bytes -> `=XX`.
	in := []byte("a = caf\xc3\xa9") // "a = café" (é is UTF-8 C3 A9)
	enc := callFn(t, toTextFn, interpreter.BytesVal(in), interpreter.StringVal("quoted-printable"))
	if enc.Str != "a =3D caf=C3=A9" {
		t.Errorf("QP encode = %q, want %q", enc.Str, "a =3D caf=C3=A9")
	}
	dec := callFn(t, fromTextFn, interpreter.StringVal(enc.Str), interpreter.StringVal("quoted-printable"))
	if string(dec.Bytes) != string(in) {
		t.Errorf("QP round-trip = %q, want %q", dec.Bytes, in)
	}
	// format names are exact (strict): a non-canonical spelling errors,
	// unlike the charset codec names, which normalise.
	if _, err := toTextFn(interpreter.BuiltinCtx{},
		[]interpreter.Value{interpreter.BytesVal(in), interpreter.StringVal("QUOTED_PRINTABLE")}); err == nil {
		t.Error("expected an error for a non-canonical format name (formats are exact)")
	}
}

func TestToTextQuotedPrintableSoftWrap(t *testing.T) {
	// A long all-printable run is soft-wrapped to <= 76 columns, and decode
	// unwinds the `=`+CRLF soft breaks back to the original bytes.
	in := bytes.Repeat([]byte("a"), 200)
	enc := callFn(t, toTextFn, interpreter.BytesVal(in), interpreter.StringVal("quoted-printable"))
	for _, line := range strings.Split(enc.Str, "\r\n") {
		if len(line) > 76 {
			t.Errorf("QP line exceeds 76 columns: %d (%q)", len(line), line)
		}
	}
	dec := callFn(t, fromTextFn, interpreter.StringVal(enc.Str), interpreter.StringVal("quoted-printable"))
	if !bytes.Equal(dec.Bytes, in) {
		t.Error("QP soft-wrap did not round-trip")
	}
}

func TestFromTextHexRejectsOddLength(t *testing.T) {
	msg := expectErr(t, fromTextFn, interpreter.StringVal("abc"), interpreter.StringVal("hex"))
	if !strings.Contains(msg, "encoding.fromText") {
		t.Errorf("err lacks fn name: %v", msg)
	}
}

func TestToTextBase64StandardRoundTrip(t *testing.T) {
	// "Hello" -> "SGVsbG8=" (RFC 4648 standard).
	enc := callFn(t, toTextFn, interpreter.BytesVal([]byte("Hello")), interpreter.StringVal("base64"))
	if enc.Str != "SGVsbG8=" {
		t.Errorf("base64 got %q, want SGVsbG8=", enc.Str)
	}
	dec := callFn(t, fromTextFn, interpreter.StringVal("SGVsbG8="), interpreter.StringVal("base64"))
	if string(dec.Bytes) != "Hello" {
		t.Errorf("fromText base64 got %q, want Hello", string(dec.Bytes))
	}
}

// TestBase64UrlSafeDistinguishes confirms the format string routes to
// the right alphabet. Input bytes 0xFF 0xFE produce different strings
// under the two variants because the standard alphabet uses `+` and
// `/` while url-safe uses `-` and `_`.
func TestBase64UrlSafeDistinguishes(t *testing.T) {
	input := []byte{0xFF, 0xFE}
	std := callFn(t, toTextFn, interpreter.BytesVal(input), interpreter.StringVal("base64")).Str
	url := callFn(t, toTextFn, interpreter.BytesVal(input), interpreter.StringVal("base64-url")).Str
	if std == url {
		t.Fatalf("standard and url-safe produced same output: %q", std)
	}
	if !strings.ContainsAny(std, "+/") {
		t.Errorf("standard output %q missing +/ chars", std)
	}
	if strings.ContainsAny(url, "+/") {
		t.Errorf("url-safe output %q contains +/", url)
	}
	dec := callFn(t, fromTextFn, interpreter.StringVal(url), interpreter.StringVal("base64-url"))
	if !bytes.Equal(dec.Bytes, input) {
		t.Errorf("url-safe round-trip mismatch: %x vs %x", dec.Bytes, input)
	}
}

func TestToTextUnknownFormat(t *testing.T) {
	msg := expectErr(t, toTextFn, interpreter.BytesVal(nil), interpreter.StringVal("morse"))
	if !strings.Contains(msg, `unknown text format "morse"`) {
		t.Errorf("err does not name the bad format: %v", msg)
	}
}

// ----- codec table --------------------------------------------------

func TestCodecsLists(t *testing.T) {
	v := callFn(t, codecsFn)
	if v.Kind != interpreter.KindList {
		t.Fatalf("codecs() = %v", v)
	}
	// The two hand-written codecs lead the list; the generated ISO-8859 /
	// Windows codecs follow, in registration order.
	head := []string{"ascii", "ebcdic", "iso-8859-1", "iso-8859-2"}
	if len(v.List) < len(head) {
		t.Fatalf("codecs() returned %d entries", len(v.List))
	}
	for i, s := range head {
		if v.List[i].Str != s {
			t.Errorf("codecs()[%d] = %q, want %q", i, v.List[i].Str, s)
		}
	}
}

func TestISO885915(t *testing.T) {
	// Latin-9: EURO at 0xA4, and Latin-1 code points elsewhere unchanged
	// (e.g. é at 0xE9).
	enc := callFn(t, encodeFn, interpreter.StringVal("€é"), interpreter.StringVal("iso-8859-15"))
	if len(enc.Bytes) != 2 || enc.Bytes[0] != 0xA4 || enc.Bytes[1] != 0xE9 {
		t.Errorf("iso-8859-15 encode = % x, want a4 e9", enc.Bytes)
	}
	dec := callFn(t, decodeFn, interpreter.BytesVal([]byte{0xA4, 0xE9}), interpreter.StringVal("iso-8859-15"))
	if dec.Str != "€é" {
		t.Errorf("iso-8859-15 decode = %q, want euro+e-acute", dec.Str)
	}
}

func TestGeneratedCodecs(t *testing.T) {
	// Spot-check generated tables against known Unicode mappings.
	cases := []struct {
		codec string
		b     byte
		want  string
	}{
		{"iso-8859-2", 0xA1, "Ą"},   // LATIN CAPITAL LETTER A WITH OGONEK
		{"iso-8859-7", 0xC1, "Α"},   // GREEK CAPITAL LETTER ALPHA
		{"windows-1251", 0xC0, "А"}, // CYRILLIC CAPITAL LETTER A
	}
	for _, tc := range cases {
		dec := callFn(t, decodeFn, interpreter.BytesVal([]byte{tc.b}), interpreter.StringVal(tc.codec))
		if dec.Str != tc.want {
			t.Errorf("%s 0x%02X = %q, want %q", tc.codec, tc.b, dec.Str, tc.want)
		}
	}
	// 4 base codecs + iso-8859-15 + 21 generated = 26.
	if list := callFn(t, codecsFn); len(list.List) != 26 {
		t.Errorf("codec count = %d, want 26", len(list.List))
	}
}

func TestCodecNamesAreExact(t *testing.T) {
	// The canonical names resolve...
	for _, name := range []string{"ascii", "iso-8859-1", "windows-1252", "ebcdic"} {
		c, ok := lookupCodec(name)
		if !ok {
			t.Errorf("canonical %q not found", name)
			continue
		}
		if c.canonical != name {
			t.Errorf("%q -> %q", name, c.canonical)
		}
	}
	// ...and nothing else does: no case-folding, no separator-stripping, no
	// IANA aliases. Strict, unlike a general charset library.
	for _, bad := range []string{"ASCII", "latin-1", "us-ascii", "latin_1", "ISO-8859-1", "iso88591", "cp1252", "MS 1252", "IBM-1047"} {
		if _, ok := lookupCodec(bad); ok {
			t.Errorf("%q resolved, but codec names are exact-match only", bad)
		}
	}
}

func TestEncodeDecodeAsciiRoundTrip(t *testing.T) {
	enc := callFn(t, encodeFn, interpreter.StringVal("hello"), interpreter.StringVal("ascii"))
	if !bytes.Equal(enc.Bytes, []byte("hello")) {
		t.Errorf("encode ascii: %x", enc.Bytes)
	}
	dec := callFn(t, decodeFn, interpreter.BytesVal([]byte("hello")), interpreter.StringVal("ascii"))
	if dec.Str != "hello" {
		t.Errorf("decode ascii: %q", dec.Str)
	}
}

func TestAsciiRejectsHighRune(t *testing.T) {
	msg := expectErr(t, encodeFn, interpreter.StringVal("café"), interpreter.StringVal("ascii"))
	if !strings.Contains(msg, "ASCII") {
		t.Errorf("err lacks ASCII mention: %v", msg)
	}
}

func TestAsciiRejectsHighByteOnDecode(t *testing.T) {
	msg := expectErr(t, decodeFn, interpreter.BytesVal([]byte{'h', 'i', 0xE9}), interpreter.StringVal("ascii"))
	if !strings.Contains(msg, "ASCII") {
		t.Errorf("err lacks ASCII mention: %v", msg)
	}
}

func TestLatin1RoundTrip(t *testing.T) {
	// "café" -> 0x63 0x61 0x66 0xE9 in Latin-1.
	enc := callFn(t, encodeFn, interpreter.StringVal("café"), interpreter.StringVal("iso-8859-1"))
	if !bytes.Equal(enc.Bytes, []byte{0x63, 0x61, 0x66, 0xE9}) {
		t.Errorf("encode latin-1: %x", enc.Bytes)
	}
	dec := callFn(t, decodeFn, interpreter.BytesVal([]byte{0x63, 0x61, 0x66, 0xE9}), interpreter.StringVal("iso-8859-1"))
	if dec.Str != "café" {
		t.Errorf("decode latin-1: %q", dec.Str)
	}
}

func TestLatin1RejectsBmpRune(t *testing.T) {
	// € has no byte in ISO-8859-1; encode errors, naming the codec and rune.
	msg := expectErr(t, encodeFn, interpreter.StringVal("€100"), interpreter.StringVal("iso-8859-1"))
	if !strings.Contains(msg, "iso-8859-1") || !strings.Contains(msg, "U+20AC") {
		t.Errorf("err lacks codec/rune mention: %v", msg)
	}
}

func TestWindows1252EuroDecode(t *testing.T) {
	// 0x80 in Windows-1252 is EURO SIGN; same byte in Latin-1 is a
	// C1 control char. This is the textbook reason the two codecs
	// are distinct.
	dec := callFn(t, decodeFn, interpreter.BytesVal([]byte{0x80, '1', '0', '0'}), interpreter.StringVal("windows-1252"))
	if dec.Str != "€100" {
		t.Errorf("decode 0x80 = %q, want €100", dec.Str)
	}
	enc := callFn(t, encodeFn, interpreter.StringVal("€100"), interpreter.StringVal("windows-1252"))
	if !bytes.Equal(enc.Bytes, []byte{0x80, '1', '0', '0'}) {
		t.Errorf("encode €100 = %x", enc.Bytes)
	}
}

func TestWindows1252UndefinedByte(t *testing.T) {
	// 0x81 is one of the five canonical-undefined positions.
	msg := expectErr(t, decodeFn, interpreter.BytesVal([]byte{0x81}), interpreter.StringVal("windows-1252"))
	if !strings.Contains(msg, "no mapping") {
		t.Errorf("err lacks mapping language: %v", msg)
	}
}

func TestEbcdicHelloRoundTrip(t *testing.T) {
	// EBCDIC IBM-1047: "Hello" = 0xC8 0x85 0x93 0x93 0x96
	enc := callFn(t, encodeFn, interpreter.StringVal("Hello"), interpreter.StringVal("ebcdic"))
	want := []byte{0xC8, 0x85, 0x93, 0x93, 0x96}
	if !bytes.Equal(enc.Bytes, want) {
		t.Errorf("encode Hello -> %x, want %x", enc.Bytes, want)
	}
	dec := callFn(t, decodeFn, interpreter.BytesVal(want), interpreter.StringVal("ebcdic"))
	if dec.Str != "Hello" {
		t.Errorf("decode = %q", dec.Str)
	}
}

func TestEbcdicLatin1Range(t *testing.T) {
	// EBCDIC IBM-1047 covers Latin-1 so "café" must round-trip.
	enc := callFn(t, encodeFn, interpreter.StringVal("café"), interpreter.StringVal("ebcdic"))
	dec := callFn(t, decodeFn, interpreter.BytesVal(enc.Bytes), interpreter.StringVal("ebcdic"))
	if dec.Str != "café" {
		t.Errorf("ebcdic café round-trip: enc=%x dec=%q", enc.Bytes, dec.Str)
	}
}

func TestUnknownCodec(t *testing.T) {
	msg := expectErr(t, encodeFn, interpreter.StringVal("hi"), interpreter.StringVal("klingon"))
	if !strings.Contains(msg, `unknown codec "klingon"`) {
		t.Errorf("err lacks unknown-codec phrasing: %v", msg)
	}
	if !strings.Contains(msg, "ascii") {
		t.Errorf("err does not list known codecs: %v", msg)
	}
}

func TestEncodeWrongScalarType(t *testing.T) {
	msg := expectErr(t, encodeFn, interpreter.BytesVal(nil), interpreter.StringVal("ascii"))
	if !strings.Contains(msg, "must be string") {
		t.Errorf("err lacks type complaint: %v", msg)
	}
}

func TestDecodeWrongScalarType(t *testing.T) {
	msg := expectErr(t, decodeFn, interpreter.StringVal("hi"), interpreter.StringVal("ascii"))
	if !strings.Contains(msg, "must be bytes") {
		t.Errorf("err lacks type complaint: %v", msg)
	}
}
