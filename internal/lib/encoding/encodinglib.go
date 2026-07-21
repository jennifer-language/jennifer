// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package encodinglib implements Jennifer's `encoding` library:
// byte/string introspection helpers, hex/base64 round-trip,
// and a codec table for converting Jennifer strings into single-byte
// encodings (and back).
//
// The cross-kind UTF-8 codec ships with `convert`
// (`convert.bytesFromString` / `convert.stringFromBytes`); this
// library is where the codec proliferation happens because that's
// where the table-based implementations belong.
//
// The library ships `"ascii"`, `"iso-8859-1"`, `"windows-1252"`, and
// `"ebcdic"` (IBM-1047) as hand-written codecs, plus `"iso-8859-15"`
// and the ISO-8859-{2..16} / Windows-{1250..1258} families generated
// from the Unicode Consortium mapping files (codecs_gen.go).
//
// The Go package is named encodinglib to avoid colliding with Go's
// standard `encoding` package, which this implementation depends on.
package encodinglib

import (
	"bytes"
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime/quotedprintable"
	"strings"
	"unicode/utf8"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "encoding"

// Install registers the encoding surface.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "isAscii", isAsciiFn)
	in.RegisterNamespaced(LibraryName, "lenBytes", lenBytesFn)
	in.RegisterNamespaced(LibraryName, "lenRunes", lenRunesFn)

	in.RegisterNamespaced(LibraryName, "toText", toTextFn)
	in.RegisterNamespaced(LibraryName, "fromText", fromTextFn)

	in.RegisterNamespaced(LibraryName, "encode", encodeFn)
	in.RegisterNamespaced(LibraryName, "decode", decodeFn)
	in.RegisterNamespaced(LibraryName, "codecs", codecsFn)
}

// ----- introspection helpers -----------------------------------------

// isAsciiFn returns true iff every byte in `b` is < 0x80.
func isAsciiFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("encoding.isAscii expects 1 argument (bytes), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("encoding.isAscii: argument must be bytes, got %s", args[0].Kind)
	}
	for _, by := range args[0].Bytes {
		if by >= 0x80 {
			return interpreter.BoolVal(false), nil
		}
	}
	return interpreter.BoolVal(true), nil
}

// lenBytesFn returns the UTF-8 byte length of a Jennifer string. Pair
// this with `len(s)` (rune count) when the difference matters.
func lenBytesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("encoding.lenBytes expects 1 argument (string), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("encoding.lenBytes: argument must be string, got %s", args[0].Kind)
	}
	return interpreter.IntVal(int64(len(args[0].Str))), nil
}

// lenRunesFn returns the rune count of UTF-8 encoded bytes; errors
// positionally on invalid UTF-8.
func lenRunesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("encoding.lenRunes expects 1 argument (bytes), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("encoding.lenRunes: argument must be bytes, got %s", args[0].Kind)
	}
	if !utf8.Valid(args[0].Bytes) {
		return interpreter.Null(), fmt.Errorf("encoding.lenRunes: input is not valid UTF-8")
	}
	return interpreter.IntVal(int64(utf8.RuneCount(args[0].Bytes))), nil
}

// ----- binary-to-text encodings (toText / fromText) -----------------
//
// Hex, base64 (standard, RFC 4648 section 4), and base64-url (RFC
// 4648 section 5) live in a small format table rather than getting
// one verb each. Two reasons:
//   - Jennifer's letters-only identifier rule rejects digits in
//     method names, so `encoding.base64` would not parse.
//   - The codec-table shape already used by `encoding.encode/decode`
//     and `convert.bytesFromString` carries over (stance #1).

// textFormatList is the rendered known-format string for error messages.
const textFormatList = `"hex", "base32", "base32-hex", "base64", "base64-url", "ascii85", "z85", "quoted-printable"`

// The format names are exact (strict): unlike the charset codec names
// (encode / decode), which normalise because they mirror external standards
// with variant spellings, these are the library's own fixed vocabulary, so
// there is no normalisation - `"BASE64"` is an error, not `"base64"`.
func textEncode(format string, b []byte) (string, error) {
	switch format {
	case "hex":
		return hex.EncodeToString(b), nil
	case "base32":
		return base32.StdEncoding.EncodeToString(b), nil
	case "base32-hex":
		return base32.HexEncoding.EncodeToString(b), nil
	case "base64":
		return base64.StdEncoding.EncodeToString(b), nil
	case "base64-url":
		return base64.URLEncoding.EncodeToString(b), nil
	case "ascii85":
		dst := make([]byte, ascii85.MaxEncodedLen(len(b)))
		return string(dst[:ascii85.Encode(dst, b)]), nil
	case "z85":
		return z85Encode(b)
	case "quoted-printable":
		var buf bytes.Buffer
		w := quotedprintable.NewWriter(&buf)
		if _, err := w.Write(b); err != nil {
			w.Close()
			return "", err
		}
		if err := w.Close(); err != nil {
			return "", err
		}
		return buf.String(), nil
	}
	return "", fmt.Errorf("unknown text format %q; known: %s", format, textFormatList)
}

func textDecode(format string, s string) ([]byte, error) {
	switch format {
	case "hex":
		out, err := hex.DecodeString(s)
		if err != nil {
			return nil, err
		}
		return out, nil
	case "base32":
		return base32.StdEncoding.DecodeString(s)
	case "base32-hex":
		return base32.HexEncoding.DecodeString(s)
	case "base64":
		return base64.StdEncoding.DecodeString(s)
	case "base64-url":
		return base64.URLEncoding.DecodeString(s)
	case "ascii85":
		dst := make([]byte, 4*len(s)+4)
		ndst, _, err := ascii85.Decode(dst, []byte(s), true)
		if err != nil {
			return nil, err
		}
		return dst[:ndst], nil
	case "z85":
		return z85Decode(s)
	case "quoted-printable":
		out, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(s)))
		if err != nil {
			return nil, err
		}
		return out, nil
	}
	return nil, fmt.Errorf("unknown text format %q; known: %s", format, textFormatList)
}

// ----- z85 (ZeroMQ Base85, RFC 32) -----------------------------------
//
// No Go stdlib codec. 4 bytes <-> 5 chars over a source-safe 85-char
// alphabet, no padding: encode input must be a multiple of 4 bytes,
// decode input a multiple of 5 chars.

const z85Alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ.-:+=^!/*?&<>()[]{}@%$#"

var z85Reverse = func() [256]int16 {
	var r [256]int16
	for i := range r {
		r[i] = -1
	}
	for i := 0; i < len(z85Alphabet); i++ {
		r[z85Alphabet[i]] = int16(i)
	}
	return r
}()

func z85Encode(b []byte) (string, error) {
	if len(b)%4 != 0 {
		return "", fmt.Errorf("z85: input length must be a multiple of 4, got %d", len(b))
	}
	var sb strings.Builder
	sb.Grow(len(b) / 4 * 5)
	for i := 0; i < len(b); i += 4 {
		n := uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3])
		var chunk [5]byte
		for j := 4; j >= 0; j-- {
			chunk[j] = z85Alphabet[n%85]
			n /= 85
		}
		sb.Write(chunk[:])
	}
	return sb.String(), nil
}

func z85Decode(s string) ([]byte, error) {
	if len(s)%5 != 0 {
		return nil, fmt.Errorf("z85: input length must be a multiple of 5, got %d", len(s))
	}
	out := make([]byte, 0, len(s)/5*4)
	for i := 0; i < len(s); i += 5 {
		var n uint64
		for j := 0; j < 5; j++ {
			idx := z85Reverse[s[i+j]]
			if idx < 0 {
				return nil, fmt.Errorf("z85: invalid character %q at position %d", s[i+j], i+j)
			}
			n = n*85 + uint64(idx)
		}
		if n > 0xFFFFFFFF {
			return nil, fmt.Errorf("z85: group at position %d exceeds 32 bits", i)
		}
		out = append(out, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	return out, nil
}

func toTextFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("encoding.toText expects 2 arguments (bytes, format), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("encoding.toText: first argument must be bytes, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("encoding.toText: format must be string, got %s", args[1].Kind)
	}
	out, err := textEncode(args[1].Str, args[0].Bytes)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("encoding.toText: %v", err)
	}
	return interpreter.StringVal(out), nil
}

func fromTextFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("encoding.fromText expects 2 arguments (string, format), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("encoding.fromText: first argument must be string, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("encoding.fromText: format must be string, got %s", args[1].Kind)
	}
	out, err := textDecode(args[1].Str, args[0].Str)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("encoding.fromText: %v", err)
	}
	return interpreter.BytesVal(out), nil
}

// ----- codec table ---------------------------------------------------

// encodeFn implements `encoding.encode(s, codec) -> bytes`.
func encodeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("encoding.encode expects 2 arguments (string, codec), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("encoding.encode: first argument must be string, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("encoding.encode: codec must be string, got %s", args[1].Kind)
	}
	c, ok := lookupCodec(args[1].Str)
	if !ok {
		return interpreter.Null(), fmt.Errorf("encoding.encode: unknown codec %q; known: %s",
			args[1].Str, knownCodecList())
	}
	out, err := c.encodeFn(args[0].Str)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("encoding.encode (%s): %v", c.canonical, err)
	}
	return interpreter.BytesVal(out), nil
}

// decodeFn implements `encoding.decode(b, codec) -> string`.
func decodeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("encoding.decode expects 2 arguments (bytes, codec), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("encoding.decode: first argument must be bytes, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("encoding.decode: codec must be string, got %s", args[1].Kind)
	}
	c, ok := lookupCodec(args[1].Str)
	if !ok {
		return interpreter.Null(), fmt.Errorf("encoding.decode: unknown codec %q; known: %s",
			args[1].Str, knownCodecList())
	}
	out, err := c.decodeFn(args[0].Bytes)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("encoding.decode (%s): %v", c.canonical, err)
	}
	return interpreter.StringVal(out), nil
}

// codecsFn implements `encoding.codecs() -> list of string`. Returns
// the canonical codec names in registration order.
func codecsFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("encoding.codecs expects 0 arguments, got %d", len(args))
	}
	out := make([]interpreter.Value, len(canonicalCodecOrder))
	for i, name := range canonicalCodecOrder {
		out[i] = interpreter.StringVal(name)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeString), out), nil
}
