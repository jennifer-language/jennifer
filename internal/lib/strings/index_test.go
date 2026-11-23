// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package stringslib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestIndexNoWrap: an index past 2^32 is an error on every build. Before the
// fix a 32-bit build (GOARCH=386, jennifer-tiny) narrowed it with int() and
// wrapped to a valid index: indexOf(..., 2^32+1) returned 3 and
// substring("hello", 1, 2^32+3) returned "el".
func TestIndexNoWrap(t *testing.T) {
	in := newLib(t)
	cases := []struct {
		fn   string
		args []interpreter.Value
	}{
		{"indexOf", []interpreter.Value{sv("abcabc"), sv("a"), iv(1<<32 + 1)}},
	}
	for i, c := range cases {
		if v, err := rawFn(in, c.fn, c.args...); err == nil {
			t.Errorf("case %d %s: got %q, want an error", i, c.fn, v.Display())
		}
	}
}

// TestIndexOfFromRunes: indexOf with from counts runes relative to from and
// returns absolute rune indices, on multi-byte and invalid-UTF-8 input.
func TestIndexOfFromRunes(t *testing.T) {
	in := newLib(t)
	s := "äb\xffäbäb"
	want := []int64{1, 4, 6}
	var got []int64
	for from := int64(0); ; {
		idx := callFn(t, in, "indexOf", sv(s), sv("b"), iv(from)).Int
		if idx < 0 {
			break
		}
		got = append(got, idx)
		from = idx + 1
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("hits %v, want %v", got, want)
	}
	if got := callFn(t, in, "indexOf", sv(s), sv("b"), iv(7)).Int; got != -1 {
		t.Fatalf("from = len: got %d", got)
	}
	_, err := rawFn(in, "indexOf", sv(s), sv("b"), iv(8))
	if err == nil || err.Error() != "strings.indexOf: from index 8 out of range (length 7)" {
		t.Fatalf("from past end: %v", err)
	}
	_, err = rawFn(in, "indexOf", sv(s), sv("b"), iv(-1))
	if err == nil || err.Error() != "strings.indexOf: from negative index -1" {
		t.Fatalf("negative from: %v", err)
	}
}

// TestSplitCharsJoinCaps: split and chars refuse more than
// limits.MaxRangeElements parts, join a result over the string limit.
// Before the fix all three built the result.
func TestSplitCharsJoinCaps(t *testing.T) {
	in := newLib(t)
	commas := strings.Repeat(",", limits.MaxRangeElements)
	if _, err := rawFn(in, "split", sv(commas), sv(",")); err == nil || !strings.Contains(err.Error(), "strings.split: ") {
		t.Fatalf("split: %v", err)
	}
	if _, err := rawFn(in, "chars", sv(commas+",")); err == nil || !strings.Contains(err.Error(), "strings.chars: ") {
		t.Fatalf("chars: %v", err)
	}
	// At the cap both still work.
	if v := callFn(t, in, "split", sv(commas[1:]), sv(",")); len(v.List) != limits.MaxRangeElements {
		t.Fatalf("split at cap: %d parts", len(v.List))
	}
	// 1026 empty parts joined with a 1 MiB separator: 1 GiB + 1 MiB of
	// separators (the separator scales with the limit on tiny).
	sep := strings.Repeat("x", limits.MaxStringBytes>>10)
	parts := make([]interpreter.Value, 1026)
	for i := range parts {
		parts[i] = sv("")
	}
	list := interpreter.ListVal(stringListType, parts)
	if _, err := rawFn(in, "join", list, sv(sep)); err == nil || !strings.Contains(err.Error(), "strings.join: result exceeds") {
		t.Fatalf("join: %v", err)
	}
	if got := callFn(t, in, "join", interpreter.ListVal(stringListType, parts[:3]), sv("-")).Str; got != "--" {
		t.Fatalf("join: %q", got)
	}
}
