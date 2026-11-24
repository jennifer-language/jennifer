// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package stringslib

import (
	"errors"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

func newLib(t *testing.T) *interpreter.Interpreter {
	t.Helper()
	in := interpreter.New()
	Install(in)
	return in
}

// callFn invokes a strings builtin and fails the test on error.
func callFn(t *testing.T, in *interpreter.Interpreter, name string, args ...interpreter.Value) interpreter.Value {
	t.Helper()
	v, err := in.LookupNamespacedBuiltin("strings", name)(interpreter.BuiltinCtx{}, args)
	if err != nil {
		t.Fatalf("strings.%s: unexpected error: %v", name, err)
	}
	return v
}

// rawFn invokes a strings builtin and returns its error (for error-path tests).
func rawFn(in *interpreter.Interpreter, name string, args ...interpreter.Value) (interpreter.Value, error) {
	return in.LookupNamespacedBuiltin("strings", name)(interpreter.BuiltinCtx{}, args)
}

func sv(s string) interpreter.Value { return interpreter.StringVal(s) }
func iv(n int64) interpreter.Value  { return interpreter.IntVal(n) }

func listStrs(v interpreter.Value) []string {
	out := make([]string, len(v.List()))
	for i, e := range v.List() {
		out[i] = e.Str()
	}
	return out
}

func TestUpperLower(t *testing.T) {
	in := newLib(t)
	if got := callFn(t, in, "upper", sv("héllo")).Str(); got != "HÉLLO" {
		t.Errorf("upper: got %q, want %q", got, "HÉLLO")
	}
	if got := callFn(t, in, "lower", sv("HÉLLO")).Str(); got != "héllo" {
		t.Errorf("lower: got %q, want %q", got, "héllo")
	}
}

func TestContainsStartsEnds(t *testing.T) {
	in := newLib(t)
	cases := []struct {
		fn, str, arg string
		want         bool
	}{
		{"contains", "hello", "ell", true},
		{"contains", "hello", "xyz", false},
		{"contains", "hello", "", true},
		{"startsWith", "hello", "he", true},
		{"startsWith", "hello", "lo", false},
		{"startsWith", "hi", "", true},
		{"endsWith", "hello", "lo", true},
		{"endsWith", "hello", "he", false},
	}
	for _, c := range cases {
		if got := callFn(t, in, c.fn, sv(c.str), sv(c.arg)).Bool(); got != c.want {
			t.Errorf("%s(%q, %q) = %v, want %v", c.fn, c.str, c.arg, got, c.want)
		}
	}
}

func TestIndexOfRuneIndexed(t *testing.T) {
	in := newLib(t)
	// é is one rune (two bytes); the index is rune-based, not byte-based.
	if got := callFn(t, in, "indexOf", sv("héllo"), sv("l")).Int(); got != 2 {
		t.Errorf("indexOf rune index: got %d, want 2", got)
	}
	if got := callFn(t, in, "indexOf", sv("abc"), sv("z")).Int(); got != -1 {
		t.Errorf("indexOf not found: got %d, want -1", got)
	}
	if got := callFn(t, in, "indexOf", sv("abcabc"), sv("bc")).Int(); got != 1 {
		t.Errorf("indexOf first occurrence: got %d, want 1", got)
	}
}

func TestIndexOfFrom(t *testing.T) {
	in := newLib(t)
	// first "b" is rune 2; resuming at 3 finds the second at rune 6; é is one rune.
	if got := callFn(t, in, "indexOf", sv("héllo"), sv("l"), iv(3)).Int(); got != 3 {
		t.Errorf("indexOf from: got %d, want 3", got)
	}
	if got := callFn(t, in, "indexOf", sv("a.b.c.b"), sv("b"), iv(3)).Int(); got != 6 {
		t.Errorf("indexOf from offset: got %d, want 6", got)
	}
	if got := callFn(t, in, "indexOf", sv("abc"), sv("a"), iv(1)).Int(); got != -1 {
		t.Errorf("indexOf from past match: got %d, want -1", got)
	}
	// from == len (rune count) is allowed: an empty-tail resume finds nothing.
	if got := callFn(t, in, "indexOf", sv("abc"), sv("x"), iv(3)).Int(); got != -1 {
		t.Errorf("indexOf from at end: got %d, want -1", got)
	}
}

func TestLastIndexOf(t *testing.T) {
	in := newLib(t)
	if got := callFn(t, in, "lastIndexOf", sv("a.b.c.b"), sv("b")).Int(); got != 6 {
		t.Errorf("lastIndexOf: got %d, want 6", got)
	}
	// rune-indexed: two "héllo", the last starts at rune 6.
	if got := callFn(t, in, "lastIndexOf", sv("héllo.héllo"), sv("héllo")).Int(); got != 6 {
		t.Errorf("lastIndexOf rune index: got %d, want 6", got)
	}
	if got := callFn(t, in, "lastIndexOf", sv("abc"), sv("z")).Int(); got != -1 {
		t.Errorf("lastIndexOf absent: got %d, want -1", got)
	}
}

func TestCount(t *testing.T) {
	in := newLib(t)
	cases := []struct {
		s, sub string
		want   int64
	}{
		{"GATTACA", "A", 3},
		{"aaaa", "aa", 2}, // non-overlapping
		{"héllo héllo", "é", 2},
		{"abc", "z", 0},
		{"", "a", 0},
	}
	for _, c := range cases {
		if got := callFn(t, in, "count", sv(c.s), sv(c.sub)).Int(); got != c.want {
			t.Errorf("count(%q, %q): got %d, want %d", c.s, c.sub, got, c.want)
		}
	}
	if _, err := rawFn(in, "count", sv("abc"), sv("")); err == nil {
		t.Error("count with an empty substring: want an error")
	}
}

func TestTrim(t *testing.T) {
	in := newLib(t)
	if got := callFn(t, in, "trim", sv("  hi \t ")).Str(); got != "hi" {
		t.Errorf("trim: got %q", got)
	}
	if got := callFn(t, in, "trimLeft", sv("  hi  ")).Str(); got != "hi  " {
		t.Errorf("trimLeft: got %q", got)
	}
	if got := callFn(t, in, "trimRight", sv("  hi  ")).Str(); got != "  hi" {
		t.Errorf("trimRight: got %q", got)
	}
}

// TestTrimAffixAndSet: trimPrefix / trimSuffix remove one exact affix;
// trimSet / trimLeftSet / trimRightSet strip any rune of the cutset. The
// whitespace trims are untouched by either.
func TestTrimAffixAndSet(t *testing.T) {
	in := newLib(t)
	cases := []struct {
		fn, s, arg, want string
	}{
		{"trimPrefix", "v1.2.3", "v", "1.2.3"},
		{"trimPrefix", "vv1", "v", "v1"}, // once, not repeatedly
		{"trimPrefix", "1.2", "v", "1.2"},
		{"trimPrefix", "abc", "", "abc"},
		{"trimPrefix", "ab", "abc", "ab"},
		{"trimSuffix", "line\r", "\r", "line"},
		{"trimSuffix", " a \r\r", "\r", " a \r"},
		{"trimSuffix", "file.tar.gz", ".gz", "file.tar"},
		{"trimSuffix", "abc", "x", "abc"},
		{"trimSet", "3.1400", "0", "3.14"},
		{"trimSet", "xxhixyx", "xy", "hi"},
		{"trimSet", "--a-b--", "-", "a-b"},
		{"trimSet", "äöhiöä", "öä", "hi"}, // runes, not bytes
		{"trimSet", "aaa", "a", ""},
		{"trimSet", " hi ", "", " hi "},
		{"trimLeftSet", "0012300", "0", "12300"},
		{"trimRightSet", "0012300", "0", "00123"},
		{"trimRightSet", "1.500", "0.", "1.5"},
		{"trimRightSet", "100", "0.", "1"},
		{"trimLeftSet", "  x  ", "", "  x  "},
	}
	for _, c := range cases {
		if got := callFn(t, in, c.fn, sv(c.s), sv(c.arg)).Str(); got != c.want {
			t.Errorf("%s(%q, %q) = %q, want %q", c.fn, c.s, c.arg, got, c.want)
		}
	}
	for _, fn := range []string{"trimPrefix", "trimSuffix", "trimSet", "trimLeftSet", "trimRightSet"} {
		if _, err := rawFn(in, fn, sv("a")); err == nil {
			t.Errorf("%s with one argument: want an error", fn)
		}
		if _, err := rawFn(in, fn, sv("a"), iv(1)); err == nil {
			t.Errorf("%s with an int cutset/affix: want an error", fn)
		}
		if _, err := rawFn(in, fn, iv(1), sv("a")); err == nil {
			t.Errorf("%s on an int: want an error", fn)
		}
	}
	// The whitespace trims stay whitespace-only.
	if got := callFn(t, in, "trim", sv("00 x 00")).Str(); got != "00 x 00" {
		t.Errorf("trim touched non-whitespace: %q", got)
	}
}

func TestReplace(t *testing.T) {
	in := newLib(t)
	if got := callFn(t, in, "replace", sv("a.b.c"), sv("."), sv("-")).Str(); got != "a-b-c" {
		t.Errorf("replace all: got %q", got)
	}
	if got := callFn(t, in, "replace", sv("abc"), sv("z"), sv("Q")).Str(); got != "abc" {
		t.Errorf("replace no-op: got %q", got)
	}
	// M29.52: an empty old is misuse (kind "runtime"), as in count and split.
	_, err := rawFn(in, "replace", sv("abc"), sv(""), sv("-"))
	var ce *limits.CapError
	if err == nil || err.Error() != "strings.replace: old must not be empty" || errors.As(err, &ce) || dataerr.KindOf(err) != "" {
		t.Errorf("replace with an empty old: got %v", err)
	}
}

// TestMessagePrefix pins M29.52: every strings error starts with "strings.fn".
func TestMessagePrefix(t *testing.T) {
	in := newLib(t)
	cases := []struct {
		fn   string
		args []interpreter.Value
	}{
		{"upper", nil},
		{"upper", []interpreter.Value{iv(1)}},
		{"indexOf", []interpreter.Value{sv("a")}},
		{"indexOf", []interpreter.Value{sv("a"), sv("a"), iv(-1)}},
		{"count", []interpreter.Value{sv("a"), sv("")}},
		{"replicate", []interpreter.Value{sv("a"), iv(-1)}},
		{"split", []interpreter.Value{sv("a"), sv("")}},
		{"join", nil},
		{"join", []interpreter.Value{sv("a")}},
	}
	for _, c := range cases {
		_, err := rawFn(in, c.fn, c.args...)
		if err == nil || !strings.HasPrefix(err.Error(), "strings."+c.fn) {
			t.Errorf("%s: got %v, want prefix strings.%s", c.fn, err, c.fn)
		}
	}
}

func TestReplicate(t *testing.T) {
	in := newLib(t)
	if got := callFn(t, in, "replicate", sv("ab"), iv(3)).Str(); got != "ababab" {
		t.Errorf("replicate = %q", got)
	}
	if got := callFn(t, in, "replicate", sv("ab"), iv(3)).Str(); got != "ababab" {
		t.Errorf("repeat 3: got %q", got)
	}
	if got := callFn(t, in, "replicate", sv("x"), iv(0)).Str(); got != "" {
		t.Errorf("repeat 0: got %q", got)
	}
	if _, err := rawFn(in, "replicate", sv("x"), iv(-1)); err == nil {
		t.Error("repeat(-1) should error")
	}
}

func TestSplitCharsJoin(t *testing.T) {
	in := newLib(t)
	// split preserves empty segments
	parts := callFn(t, in, "split", sv("a,,b"), sv(","))
	if got := listStrs(parts); len(got) != 3 || got[0] != "a" || got[1] != "" || got[2] != "b" {
		t.Errorf("split: got %q, want [a  b]", got)
	}
	if _, err := rawFn(in, "split", sv("abc"), sv("")); err == nil {
		t.Error("split with empty separator should error")
	}
	// chars: one rune per entry, Unicode-aware
	chars := callFn(t, in, "chars", sv("héllo"))
	if got := listStrs(chars); len(got) != 5 || got[1] != "é" {
		t.Errorf("chars: got %q", got)
	}
	// join is the inverse of split for a non-empty separator
	if got := callFn(t, in, "join", parts, sv(",")).Str(); got != "a,,b" {
		t.Errorf("join(split(...)): got %q, want %q", got, "a,,b")
	}
}

func TestStringsArityAndTypeErrors(t *testing.T) {
	in := newLib(t)
	if _, err := rawFn(in, "upper"); err == nil {
		t.Error("upper() with no args should error")
	}
	if _, err := rawFn(in, "upper", iv(5)); err == nil {
		t.Error("upper(int) should error")
	}
	if _, err := rawFn(in, "contains", sv("a")); err == nil {
		t.Error("contains(one arg) should error")
	}
	if _, err := rawFn(in, "join", sv("notalist"), sv(",")); err == nil {
		t.Error("join(string, sep) should error (first arg must be a list)")
	}
}
