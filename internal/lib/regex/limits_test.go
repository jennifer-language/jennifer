// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package regexlib

import (
	"runtime"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

func callRegex(name string, args ...string) (Value, error) {
	in := interpreter.New()
	Install(in)
	vs := make([]Value, len(args))
	for i, a := range args {
		vs[i] = interpreter.StringVal(a)
	}
	return in.LookupNamespacedBuiltin("regex", name)(interpreter.BuiltinCtx{}, vs)
}

// allocDuring returns the bytes allocated while f runs.
func allocDuring(f func()) uint64 {
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	f()
	runtime.ReadMemStats(&b)
	return b.TotalAlloc - a.TotalAlloc
}

// TestFindAllMatchCap: an empty pattern matches at every position; past the
// match budget findAll errors instead of building ~2 KB per match. The
// pre-fix code built all 600001 matches (over 1 GB).
func TestFindAllMatchCap(t *testing.T) {
	resetCacheForTest()
	var err error
	got := allocDuring(func() {
		_, err = callRegex("findAll", "", strings.Repeat("a", 600000))
	})
	if err == nil || !strings.Contains(err.Error(), "regex.findAll: more than") {
		t.Fatalf("got %v", err)
	}
	if got > 128<<20 {
		t.Fatalf("allocated %d bytes before refusing", got)
	}
	// Groups shrink the budget: 1000 groups allow only a few thousand matches.
	if _, err := callRegex("findAll", strings.Repeat("()", 1000), strings.Repeat("a", 3000)); err == nil {
		t.Fatal("1000-group pattern over 3001 positions should error")
	}
	v, err := callRegex("findAll", "a", "banana")
	if err != nil || len(v.List) != 3 {
		t.Fatalf("small findAll: %v", err)
	}
}

// TestReplaceOutputCap: a replacement whose result could pass
// limits.MaxStringBytes is refused before building it.
func TestReplaceOutputCap(t *testing.T) {
	resetCacheForTest()
	s := strings.Repeat("a", limits.MaxStringBytes>>10)
	var err error
	got := allocDuring(func() {
		_, err = callRegex("replace", "a", s, strings.Repeat("b", 1025))
	})
	if err == nil || !strings.Contains(err.Error(), "regex.replace: result could exceed") {
		t.Fatalf("literal: got %v", err)
	}
	if got > 64<<20 {
		t.Fatalf("allocated %d bytes before refusing", got)
	}
	// A capture reference multiplies the match: one 1 MiB match, $0 x 1100.
	if _, err := callRegex("replace", "a+", s, strings.Repeat("$0", 1100)); err == nil {
		t.Fatal("$0 amplification should error")
	}
	v, err := callRegex("replace", `(\w+)@(\w+)`, "x a@b y", "$2 at $1")
	if err != nil || v.Str != "x b at a y" {
		t.Fatalf("groups: %q, %v", v.Str, err)
	}
	v, err = callRegex("replace", "", "ab", "-")
	if err != nil || v.Str != "-a-b-" {
		t.Fatalf("empty pattern: %q, %v", v.Str, err)
	}
}
