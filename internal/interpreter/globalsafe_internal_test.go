// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter

import (
	"bytes"
	"fmt"
	"testing"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// computeEntryGlobalSafe (the per-function escape analysis) marks a method
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
		// Computed even when no parameter can borrow: a local def borrows on it too.
		{"no borrowable param", `def g as int init 0; func f(x as int) { return $x; }`, map[string]bool{"f": true}},
		{"local def only", `def g as list of int init [1]; func f() { def q as list of int init $g; return $q[0]; }`, map[string]bool{"f": true}},
		// A spawn body's global writes land in its snapshot.
		{"spawn writing a global", `def g as int init 0; func f(xs as list of int) { def t as task of null init spawn { $g = 1; }; return $xs[0]; }`, map[string]bool{"f": true}},
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
		// A pattern arm's head `Formula(f)` is not a call: it must not disqualify.
		{"binding match arm", `def enum V { Num { n as int }, Formula { src as string } }; def g as int init 0; func f(v as V, xs as list of int) { match ($v) { when Formula(x) { return $x.src; } else { return ""; } } }`, map[string]bool{"f": true}},
		{"binding match arm body writes a global", `def enum V { Num { n as int }, Formula { src as string } }; def g as int init 0; func f(v as V) { match ($v) { when Formula(x) { $g = 1; } else { } } }`, map[string]bool{"f": false}},
		// A value arm's values do run, so a call there still counts.
		{"value match arm calling a writer", `def g as int init 0; func h() { $g = 1; return 1; } func f(n as int) { match ($n) { when h() { return 1; } else { return 0; } } }`, map[string]bool{"f": false}},
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

// The worklist propagates "unsafe" up a long call chain to its root (and does it
// in time linear in the chain; the old per-round rescan was quadratic).
func TestComputeEntryGlobalSafeLongChain(t *testing.T) {
	const n = 3000
	var b bytes.Buffer
	b.WriteString("def g as int init 0;\nfunc f0(xs as list of int) { $g = 1; return 0; }\n")
	for k := 1; k < n; k++ {
		fmt.Fprintf(&b, "func f%d(xs as list of int) { return f%d($xs); }\n", k, k-1)
	}
	b.WriteString("func pure(xs as list of int) { return len($xs); }\n")
	prog, err := parser.Parse(b.String())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in := New()
	in.Out = &bytes.Buffer{}
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	for name, m := range in.methods {
		if want := name == "pure"; m.GlobalSafe != want {
			t.Fatalf("%s: GlobalSafe = %v, want %v", name, m.GlobalSafe, want)
		}
	}
}

// A spawn body is not slot-resolved, so its calls carry no Method. A call to a
// plain user function there must still resolve by name, or every program that
// spawns a worker would look re-entering and lose borrow run-wide. The spawns
// sit in a function that is never called: the analysis is static, and a task
// left running past the test would race the next one.
func TestSpawnReentryResolvesSpawnBodyCalls(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"plain callee", `def g as int init 0; func work(n as int) { return $n * 2; } func launch() { def t as task of int init spawn { return work(3); }; }`, false},
		{"callee writing a global", `def g as int init 0; func work() { $g = 1; return 0; } func launch() { def t as task of int init spawn { return work(); }; }`, false},
		{"callee calling a func value", `def g as int init 0; func one() { return 1; } func work(fn as func) { return $fn(); } func launch() { def t as task of int init spawn { return work(one); }; }`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, err := parser.Parse(c.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			in := New()
			in.Out = &bytes.Buffer{}
			if err := in.Run(prog); err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := in.Host().spawnReentry.Load(); got != c.want {
				t.Errorf("spawnReentry = %v, want %v", got, c.want)
			}
		})
	}
}

// stampArgSnapshots marks exactly the arguments that alias a global and are
// followed by an argument that may mutate globals.
func TestStampArgSnapshots(t *testing.T) {
	src := `
		def g as list of int init [1];
		def k as int init 0;
		func w() { $g[0] = 9; return 0; }
		func r() { return 1; }
		func f(a as list of int, b as int) { return 0; }
		f($g, w());
		f($g, r());
		f([1], w());
		f($g, $k + w());
		f($g, len($g));
	`
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in := New()
	in.Out = &bytes.Buffer{}
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	want := [][]bool{{true, false}, nil, nil, {true, false}, nil}
	k := 0
	for _, s := range prog.TopLevel {
		es, ok := s.(*parser.ExprStmt)
		if !ok {
			continue
		}
		c := es.Expr.(*parser.CallExpr)
		if fmt.Sprint(c.ArgSnap) != fmt.Sprint(want[k]) {
			t.Errorf("call %d: ArgSnap = %v, want %v", k, c.ArgSnap, want[k])
		}
		k++
	}
}
