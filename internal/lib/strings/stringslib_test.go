// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package stringslib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
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
	out := make([]string, len(v.List))
	for i, e := range v.List {
		out[i] = e.Str
	}
	return out
}

func TestUpperLower(t *testing.T) {
	in := newLib(t)
	if got := callFn(t, in, "upper", sv("héllo")).Str; got != "HÉLLO" {
		t.Errorf("upper: got %q, want %q", got, "HÉLLO")
	}
	if got := callFn(t, in, "lower", sv("HÉLLO")).Str; got != "héllo" {
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
		if got := callFn(t, in, c.fn, sv(c.str), sv(c.arg)).Bool; got != c.want {
			t.Errorf("%s(%q, %q) = %v, want %v", c.fn, c.str, c.arg, got, c.want)
		}
	}
}

func TestIndexOfRuneIndexed(t *testing.T) {
	in := newLib(t)
	// é is one rune (two bytes); the index is rune-based, not byte-based.
	if got := callFn(t, in, "indexOf", sv("héllo"), sv("l")).Int; got != 2 {
		t.Errorf("indexOf rune index: got %d, want 2", got)
	}
	if got := callFn(t, in, "indexOf", sv("abc"), sv("z")).Int; got != -1 {
		t.Errorf("indexOf not found: got %d, want -1", got)
	}
	if got := callFn(t, in, "indexOf", sv("abcabc"), sv("bc")).Int; got != 1 {
		t.Errorf("indexOf first occurrence: got %d, want 1", got)
	}
}

func TestTrim(t *testing.T) {
	in := newLib(t)
	if got := callFn(t, in, "trim", sv("  hi \t ")).Str; got != "hi" {
		t.Errorf("trim: got %q", got)
	}
	if got := callFn(t, in, "trimLeft", sv("  hi  ")).Str; got != "hi  " {
		t.Errorf("trimLeft: got %q", got)
	}
	if got := callFn(t, in, "trimRight", sv("  hi  ")).Str; got != "  hi" {
		t.Errorf("trimRight: got %q", got)
	}
}

func TestReplace(t *testing.T) {
	in := newLib(t)
	if got := callFn(t, in, "replace", sv("a.b.c"), sv("."), sv("-")).Str; got != "a-b-c" {
		t.Errorf("replace all: got %q", got)
	}
	if got := callFn(t, in, "replace", sv("abc"), sv("z"), sv("Q")).Str; got != "abc" {
		t.Errorf("replace no-op: got %q", got)
	}
}

func TestRepeat(t *testing.T) {
	in := newLib(t)
	if got := callFn(t, in, "repeat", sv("ab"), iv(3)).Str; got != "ababab" {
		t.Errorf("repeat 3: got %q", got)
	}
	if got := callFn(t, in, "repeat", sv("x"), iv(0)).Str; got != "" {
		t.Errorf("repeat 0: got %q", got)
	}
	if _, err := rawFn(in, "repeat", sv("x"), iv(-1)); err == nil {
		t.Error("repeat(-1) should error")
	}
}

func TestSubstring(t *testing.T) {
	in := newLib(t)
	// rune-indexed, exclusive end
	if got := callFn(t, in, "substring", sv("héllo"), iv(0), iv(2)).Str; got != "hé" {
		t.Errorf("substring(0,2): got %q, want %q", got, "hé")
	}
	if got := callFn(t, in, "substring", sv("héllo"), iv(2)).Str; got != "llo" {
		t.Errorf("substring(2) 2-arg: got %q, want %q", got, "llo")
	}
	if got := callFn(t, in, "substring", sv("abc"), iv(0), iv(3)).Str; got != "abc" {
		t.Errorf("substring full: got %q", got)
	}
	if got := callFn(t, in, "substring", sv("abc"), iv(1), iv(1)).Str; got != "" {
		t.Errorf("substring empty slice: got %q", got)
	}
	if _, err := rawFn(in, "substring", sv("abc"), iv(1), iv(99)); err == nil {
		t.Error("substring end past length should error")
	}
	if _, err := rawFn(in, "substring", sv("abc"), iv(2), iv(1)); err == nil {
		t.Error("substring start>end should error")
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
	if got := callFn(t, in, "join", parts, sv(",")).Str; got != "a,,b" {
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
