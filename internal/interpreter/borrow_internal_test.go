// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter

import (
	"bytes"
	"testing"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// hasMutableTopLevelGlobal decides whether a single-file script may widen
// read-only-parameter borrow to its own methods: only a mutable top-level `def`
// creates a global a method could alias through an argument and mutate. A
// `def const` is immutable, and a `def` nested in a top-level block lives in
// that block's frame (not the global scope methods reach), so neither counts.
func TestHasMutableTopLevelGlobal(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"no globals at all", `func f(x as int) { return $x; }`, false},
		{"only const global", `def const MAX as int init 10; func f(x as int) { return $x + MAX; }`, false},
		{"struct + func only", `def struct P { a as int }; func f(p as P) { return $p.a; }`, false},
		{"const list global", `def const XS as list of int init [1, 2]; func f() { return 0; }`, false},
		{"mutable int global", `def g as int init 0; func f(x as int) { return $x; }`, true},
		{"mutable list global", `def g as list of int init [1]; func f() { return 0; }`, true},
		{"mutable global uninitialized", `def g as int; func f() { return 0; }`, true},
		{"const then mutable", `def const A as int init 1; def g as int init 2; func f() { return 0; }`, true},
		{"def nested in top-level for is not a global", `func f(x as int) { return $x; } for (def i in 0..3) { def z as int init $i; }`, false},
		{"def nested in top-level if is not a global", `func f(x as int) { return $x; } if (true) { def z as int init 1; }`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, err := parser.Parse(c.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := hasMutableTopLevelGlobal(prog); got != c.want {
				t.Errorf("hasMutableTopLevelGlobal = %v, want %v", got, c.want)
			}
		})
	}
}

// Run wires the flag: a globals-free entry program sets entryGlobalsImmutable
// (so its own borrowable params alias), a program with a mutable global does
// not, and a module context never sets it (isModule already gates borrow).
func TestEntryGlobalsImmutableFlagWiring(t *testing.T) {
	runFlag := func(t *testing.T, src string, asModule bool) bool {
		t.Helper()
		prog, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		in := New()
		var buf bytes.Buffer
		in.Out = &buf
		if asModule {
			in.SetModuleContext(true)
		}
		if err := in.Run(prog); err != nil {
			t.Fatalf("run: %v", err)
		}
		return in.entryGlobalsImmutable
	}

	if got := runFlag(t, `def const A as int init 1; func f(x as int) { return $x; }`, false); !got {
		t.Errorf("globals-free script: entryGlobalsImmutable = false, want true")
	}
	if got := runFlag(t, `def g as int init 0; func f(x as int) { return $x; }`, false); got {
		t.Errorf("script with mutable global: entryGlobalsImmutable = true, want false")
	}
	// A module never sets the flag (its borrow is gated by isModule, which is
	// the stronger declarations-only guarantee). export is legal here.
	if got := runFlag(t, `export func f(x as int) { return $x; }`, true); got {
		t.Errorf("module context: entryGlobalsImmutable = true, want false")
	}
}
