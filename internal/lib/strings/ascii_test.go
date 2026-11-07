// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package stringslib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestOneByteRunePath: substring and indexOf(from) on strings whose runes are
// all one byte (Value.RuneLen() == byte length) take byte offsets directly;
// results and messages match the rune walk. "ab\xffcd" has an invalid byte,
// still one rune of one byte.
func TestOneByteRunePath(t *testing.T) {
	in := newLib(t)
	for _, s := range []string{"abcabc", "ab\xffcd", ""} {
		for from := int64(0); from <= int64(len(s)); from++ {
			for _, sub := range []string{"c", "\xff", "", "zz"} {
				want := int64(-1)
				if i := strings.Index(s[from:], sub); i >= 0 {
					want = from + int64(i)
				}
				if got := callFn(t, in, "indexOf", sv(s), sv(sub), iv(from)).Int; got != want {
					t.Errorf("indexOf(%q, %q, %d) = %d, want %d", s, sub, from, got, want)
				}
			}
			for end := from; end <= int64(len(s)); end++ {
				if got := callFn(t, in, "substring", sv(s), iv(from), iv(end)).Str; got != s[from:end] {
					t.Errorf("substring(%q, %d, %d) = %q", s, from, end, got)
				}
			}
			if got := callFn(t, in, "substring", sv(s), iv(from)).Str; got != s[from:] {
				t.Errorf("substring(%q, %d) = %q", s, from, got)
			}
		}
	}
	bad := []struct {
		fn   string
		args []interpreter.Value
		msg  string
	}{
		{"indexOf", []interpreter.Value{sv("abc"), sv("a"), iv(4)}, "indexOf(): from index 4 out of range (length 3)"},
		{"substring", []interpreter.Value{sv("abc"), iv(4)}, "substring(): start out of range (length 3)"},
		{"substring", []interpreter.Value{sv("abc"), iv(4), iv(5)}, "substring(): start out of range (length 3)"},
		{"substring", []interpreter.Value{sv("abc"), iv(1), iv(4)}, "substring(): end out of range (length 3)"},
		{"substring", []interpreter.Value{sv("abc"), iv(2), iv(1)}, "substring(): end 1 is before start 2"},
	}
	for i, c := range bad {
		if _, err := rawFn(in, c.fn, c.args...); err == nil || err.Error() != c.msg {
			t.Errorf("case %d: got %v, want %q", i, err, c.msg)
		}
	}
	// A string Value built without StringVal has no cached count: RuneLen
	// counts, the result is the same.
	raw := interpreter.Value{Kind: interpreter.KindString, Str: "xyzxyz"}
	if got := callFn(t, in, "indexOf", raw, sv("x"), iv(1)).Int; got != 3 {
		t.Errorf("uncached indexOf: %d", got)
	}
}

// BenchmarkIndexOfFromWalk searches from offsets spread over a 240,000-rune
// ASCII string (a scan loop: indexOf($s, sub, $pos)).
func BenchmarkIndexOfFromWalk(b *testing.B) {
	in := interpreter.New()
	Install(in)
	fn := in.LookupNamespacedBuiltin("strings", "indexOf")
	s := sv(strings.Repeat("abcdefghij", 24000))
	sub := sv("j")
	for i := 0; i < b.N; i++ {
		from := int64(i*100) % 239900
		if _, err := fn(interpreter.BuiltinCtx{}, []interpreter.Value{s, sub, iv(from)}); err != nil {
			b.Fatal(err)
		}
	}
}
