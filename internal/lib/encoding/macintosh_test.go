// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package encodinglib

import (
	"bytes"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// Mac OS Roman ("macintosh", M29.56), from Apple's ROMAN.TXT: the controls
// the file leaves out map to themselves, 0xDB is the euro sign (Apple's
// mapping since Mac OS 8.5, as WHATWG has it), 0xF0 the Apple logo in the
// private use area.
func TestMacintoshDecode(t *testing.T) {
	cases := []struct {
		b    byte
		want string
	}{
		{0x00, "\x00"}, {0x0A, "\n"}, {0x1F, "\x1f"}, {0x7F, "\x7f"},
		{'A', "A"}, {0x80, "Ä"}, {0x8E, "é"}, {0xA5, "•"}, {0xBD, "Ω"},
		{0xC9, "…"}, {0xD0, "–"}, {0xDB, "€"}, {0xDE, "ﬁ"}, {0xF0, ""},
		{0xFF, "ˇ"},
	}
	for _, c := range cases {
		got := callFn(t, decodeFn, interpreter.StringVal("macintosh"), interpreter.BytesVal([]byte{c.b})).Str()
		if got != c.want {
			t.Errorf("decode 0x%02X = %q, want %q", c.b, got, c.want)
		}
	}
}

// Every byte is defined, so all 256 decode and encode back to themselves.
func TestMacintoshRoundTripAllBytes(t *testing.T) {
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	s := callFn(t, decodeFn, interpreter.StringVal("macintosh"), interpreter.BytesVal(all))
	back := callFn(t, encodeFn, interpreter.StringVal("macintosh"), s)
	if !bytes.Equal(back.Bytes(), all) {
		t.Fatalf("round trip differs: %x", back.Bytes())
	}
}

// A character the set lacks is an error naming it, as for the other tables.
func TestMacintoshEncodeUnmapped(t *testing.T) {
	for _, s := range []string{"Ł", "¤", "€½"} {
		expectErr(t, encodeFn, interpreter.StringVal("macintosh"), interpreter.StringVal(s))
	}
	enc := callFn(t, encodeFn, interpreter.StringVal("macintosh"), interpreter.StringVal("Café €5"))
	if want := []byte{'C', 'a', 'f', 0x8E, ' ', 0xDB, '5'}; !bytes.Equal(enc.Bytes(), want) {
		t.Errorf("encode = %x, want %x", enc.Bytes(), want)
	}
}
