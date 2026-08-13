// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter

import (
	"bytes"
	"testing"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// computeEntryGlobalSafe (the M25.2 per-function escape analysis) marks a method
// GlobalSafe when it, and everything it transitively calls by name, mutates no
// global. A false positive would let borrow alias a parameter whose backing the
// callee can mutate, so the write-scan, the fixpoint, and the dynamic-dispatch
// disqualifiers are pinned here. Every program declares a mutable global so the
// analysis runs (a globals-free script takes the entryGlobalsImmutable path and
// leaves GlobalSafe unset). These cases use no libraries, so a plain New() can
// Run them; library-callback disqualifiers are covered by the behaviour tests in
// borrow_test.go (they need libraries installed).
func TestComputeEntryGlobalSafe(t *testing.T) {
	safeOf := func(t *testing.T, src string) map[string]bool {
		t.Helper()
		prog, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		in := New()
		var buf bytes.Buffer
		in.Out = &buf
		if err := in.Run(prog); err != nil {
			t.Fatalf("run: %v", err)
		}
		res := map[string]bool{}
		for name, m := range in.methods {
			res[name] = m.GlobalSafe
		}
		return res
	}

	cases := []struct {
		name string
		src  string
		want map[string]bool
	}{
		{"pure reader", `def g as int init 0; func f(xs as list of int) { return $xs[0]; }`, map[string]bool{"f": true}},
		{"skipped when no param can borrow", `def g as int init 0; func f(x as int) { return $x; }`, map[string]bool{"f": false}},
		{"reassigns global", `def g as int init 0; func f(xs as list of int) { $g = 1; return $xs[0]; }`, map[string]bool{"f": false}},
		{"index-writes global", `def g as list of int init []; func f(xs as list of int) { $g[0] = 1; return $xs[0]; }`, map[string]bool{"f": false}},
		{"appends to global", `def g as list of int init []; func f(xs as list of int) { $g[] = 1; return $xs[0]; }`, map[string]bool{"f": false}},
		{"writes a local is fine", `def g as int init 0; func f(xs as list of int) { def y as int init 0; $y = 1; return $xs[0] + $y; }`, map[string]bool{"f": true}},
		{"writes global in nested block", `def g as int init 0; func f(xs as list of int) { if (true) { while (false) { $g = 1; } } return $xs[0]; }`, map[string]bool{"f": false}},
		{"transitive writer", `def g as int init 0; func h() { $g = 1; } func f(xs as list of int) { h(); return $xs[0]; }`, map[string]bool{"f": false, "h": false}},
		{"transitive pure", `def g as int init 0; func h(x as int) { return $x + 1; } func f(xs as list of int) { return h($xs[0]); }`, map[string]bool{"f": true, "h": true}},
		{"self recursion pure", `def g as int init 0; func f(xs as list of int, i as int) { if ($i >= len($xs)) { return 0; } return $xs[$i] + f($xs, $i + 1); }`, map[string]bool{"f": true}},
		{"mutual recursion pure", `def g as int init 0; func a(xs as list of int) { return b($xs); } func b(xs as list of int) { return len($xs); }`, map[string]bool{"a": true, "b": true}},
		{"cycle reaching a writer", `def g as int init 0; func a(xs as list of int) { b($xs); return $xs[0]; } func b(xs as list of int) { $g = 1; a($xs); }`, map[string]bool{"a": false, "b": false}},
		{"function-value call disqualifies", `def g as int init 0; func apply(fn as func, xs as list of int) { return $fn($xs[0]); }`, map[string]bool{"apply": false}},
		{"deep transitive: safe -> safe -> writer", `def g as int init 0; func c() { $g = 1; } func b() { c(); } func a(xs as list of int) { b(); return $xs[0]; }`, map[string]bool{"a": false, "b": false, "c": false}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := safeOf(t, c.src)
			for name, want := range c.want {
				if got[name] != want {
					t.Errorf("method %q: GlobalSafe = %v, want %v", name, got[name], want)
				}
			}
		})
	}
}
