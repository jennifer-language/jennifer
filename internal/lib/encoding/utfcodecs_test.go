// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package encodinglib

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func encodeAs(t *testing.T, codec, s string) []byte {
	t.Helper()
	return callFn(t, encodeFn, interpreter.StringVal(codec), interpreter.StringVal(s)).Bytes()
}

func decodeAs(t *testing.T, codec string, b []byte) string {
	t.Helper()
	return callFn(t, decodeFn, interpreter.StringVal(codec), interpreter.BytesVal(b)).Str()
}

// "aé€😀": ASCII, two-byte, three-byte and a supplementary-plane rune
// (a surrogate pair in UTF-16).
const utfSample = "aé€\U0001F600"

func TestUTFEncode(t *testing.T) {
	cases := []struct {
		codec string
		want  []byte
	}{
		{"utf-8", []byte(utfSample)},
		{"utf-16be", []byte{0, 'a', 0, 0xE9, 0x20, 0xAC, 0xD8, 0x3D, 0xDE, 0x00}},
		{"utf-16le", []byte{'a', 0, 0xE9, 0, 0xAC, 0x20, 0x3D, 0xD8, 0x00, 0xDE}},
		{"utf-16", []byte{0xFE, 0xFF, 0, 'a', 0, 0xE9, 0x20, 0xAC, 0xD8, 0x3D, 0xDE, 0x00}},
		{"utf-32be", []byte{0, 0, 0, 'a', 0, 0, 0, 0xE9, 0, 0, 0x20, 0xAC, 0, 1, 0xF6, 0}},
		{"utf-32le", []byte{'a', 0, 0, 0, 0xE9, 0, 0, 0, 0xAC, 0x20, 0, 0, 0, 0xF6, 1, 0}},
		{"utf-32", []byte{0, 0, 0xFE, 0xFF, 0, 0, 0, 'a', 0, 0, 0, 0xE9, 0, 0, 0x20, 0xAC, 0, 1, 0xF6, 0}},
	}
	for _, c := range cases {
		got := encodeAs(t, c.codec, utfSample)
		if !bytes.Equal(got, c.want) {
			t.Errorf("%s: encode = % X, want % X", c.codec, got, c.want)
		}
		if back := decodeAs(t, c.codec, got); back != utfSample {
			t.Errorf("%s: round trip = %q", c.codec, back)
		}
	}
}

func TestUTFDecodeBOM(t *testing.T) {
	// The unmarked names read a BOM and pick the order; without one, BE.
	le := append([]byte{0xFF, 0xFE}, encodeAs(t, "utf-16le", utfSample)...)
	if got := decodeAs(t, "utf-16", le); got != utfSample {
		t.Errorf("utf-16 with an LE BOM = %q", got)
	}
	if got := decodeAs(t, "utf-16", encodeAs(t, "utf-16be", "ab")); got != "ab" {
		t.Errorf("utf-16 without a BOM = %q", got)
	}
	le32 := append([]byte{0xFF, 0xFE, 0, 0}, encodeAs(t, "utf-32le", utfSample)...)
	if got := decodeAs(t, "utf-32", le32); got != utfSample {
		t.Errorf("utf-32 with an LE BOM = %q", got)
	}
	// In a name with a byte order, U+FEFF is a character, kept.
	if got := decodeAs(t, "utf-16be", []byte{0xFE, 0xFF, 0, 'a'}); got != "\uFEFFa" {
		t.Errorf("utf-16be kept BOM = %q", got)
	}
}

func TestUTFDecodeStrict(t *testing.T) {
	cases := []struct {
		codec string
		in    []byte
		want  string
	}{
		{"utf-8", []byte{'a', 0xC3}, "invalid UTF-8 at byte 1"},
		{"utf-8", []byte{0xED, 0xA0, 0x80}, "invalid UTF-8 at byte 0"},
		{"utf-16be", []byte{0, 'a', 0}, "not a whole number of 16-bit units"},
		{"utf-16be", []byte{0, 'a', 0xD8, 0x3D}, "high surrogate 0xD83D at byte 2 has no low surrogate"},
		{"utf-16be", []byte{0xD8, 0x3D, 0, 'a'}, "high surrogate 0xD83D at byte 0 has no low surrogate"},
		{"utf-16le", []byte{0x00, 0xDE}, "unpaired low surrogate 0xDE00 at byte 0"},
		{"utf-16", []byte{0xFF, 0xFE, 0x00, 0xDE}, "unpaired low surrogate 0xDE00 at byte 2"},
		{"utf-32be", []byte{0, 0, 0, 'a', 0}, "not a whole number of 32-bit units"},
		{"utf-32be", []byte{0, 0x11, 0, 0}, "0x00110000 at byte 0 is not a Unicode scalar value"},
		{"utf-32le", []byte{0, 0xD8, 0, 0}, "0x0000D800 at byte 0 is not a Unicode scalar value"},
	}
	for _, c := range cases {
		msg := expectErr(t, decodeFn, interpreter.StringVal(c.codec), interpreter.BytesVal(c.in))
		if !strings.Contains(msg, c.want) {
			t.Errorf("%s % X: err = %q, want %q", c.codec, c.in, msg, c.want)
		}
	}
}

func TestUTFEmpty(t *testing.T) {
	for _, c := range []string{"utf-8", "utf-16le", "utf-16be", "utf-32le", "utf-32be"} {
		if got := encodeAs(t, c, ""); len(got) != 0 {
			t.Errorf("%s: encode empty = % X", c, got)
		}
		if got := decodeAs(t, c, nil); got != "" {
			t.Errorf("%s: decode empty = %q", c, got)
		}
	}
	if got := encodeAs(t, "utf-16", ""); !bytes.Equal(got, []byte{0xFE, 0xFF}) {
		t.Errorf("utf-16: encode empty = % X, want the BOM", got)
	}
}
