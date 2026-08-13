// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// Read-only-parameter borrow: a never-written compound parameter is bound by
// alias instead of deep-copied, but only in a module (declarations-only, so no
// mutable global can alias the argument and be mutated during the call). These
// tests lock the soundness gate and the value-semantics parity.

// The soundness gate: an entry program that declares a mutable top-level global
// keeps copy semantics, because a parameter could alias that global and the body
// could mutate it. If borrow leaked in here, $p[0] would read 99 (the shared
// backing) instead of 1 (a copy). The presence of `def g` (mutable) is what
// disables the script-wide borrow widening.
func TestBorrowGateEntryProgramKeepsCopySemantics(t *testing.T) {
	out, err := runAlias(t, `
		use io;
		def g as list of int init [1, 2, 3];
		func f(p as list of int) {
			$g[0] = 99;
			return $p[0];
		}
		io.printf("%d", f($g));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "1" {
		t.Fatalf("entry-program value semantics broken: got %q, want %q (borrow must not apply with a mutable global)", out, "1")
	}
}

// A single-file script with NO mutable top-level global (only const + funcs)
// widens borrow to its own methods - the aliasing hole is unrepresentable, so it
// is sound. Value semantics must still hold: a borrowed param copied to a local
// and mutated must not disturb the borrowed original. The list comes from a
// const so the script stays globals-immutable.
func TestBorrowEntryScriptGlobalsFreeValueSemantics(t *testing.T) {
	out, err := runAlias(t, `
		use io;
		def const XS as list of int init [5, 6, 7];
		func firstAfterLocalMutate(xs as list of int) {
			def ys as list of int init $xs;
			$ys[0] = 999;
			return $xs[0];
		}
		io.printf("%d", firstAfterLocalMutate(XS));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// 5: the borrowed param is unaffected by mutating a local copy of it. If the
	// borrow aliased ys to xs, this would read 999.
	if out != "5" {
		t.Fatalf("globals-free script borrow value semantics: got %q, want %q", out, "5")
	}
}

// A module borrows read-only list/struct params (no mutable globals exist to
// create the aliasing hole). Reads must be correct and the caller's value must
// be untouched even when the callee copies the borrowed param and mutates the
// copy.
func TestBorrowModuleValueSemantics(t *testing.T) {
	out, err := runModuleMain(t, map[string]string{
		"m.j": `
			# xs is never written -> borrowed. Copying it to a local and mutating
			# the local must not disturb the borrowed original.
			export func firstAfterLocalMutate(xs as list of int) {
				def ys as list of int init $xs;
				$ys[0] = 999;
				return $xs[0];
			}
			# both xs params are borrowed (never written); a per-element read loop
			# through a second borrowing helper must total correctly.
			export func at(xs as list of int, i as int) { return $xs[$i]; }
			export func total(xs as list of int) {
				def s as int init 0;
				def i as int init 0;
				while ($i < len($xs)) { $s = $s + at($xs, $i); $i = $i + 1; }
				return $s;
			}
		`,
		"main.j": `
			use io;
			import "./m.j" as m;
			def xs as list of int init [5, 6, 7];
			def r as int init m.firstAfterLocalMutate($xs);
			io.printf("%d %d %d", $r, $xs[0], m.total($xs));
		`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// r=5 (borrowed original intact despite local copy's mutation);
	// xs[0]=5 (caller's list untouched); total=18.
	if out != "5 5 18" {
		t.Fatalf("module borrow value semantics: got %q, want %q", out, "5 5 18")
	}
}

// A borrowed struct parameter (countNodes shape): a recursive read-only walk
// over a value-semantic tree must count correctly with the param aliased.
func TestBorrowModuleStructParam(t *testing.T) {
	out, err := runModuleMain(t, map[string]string{
		"tree.j": `
			export def struct Node { v as int, kids as list of Node };
			# n is never written -> borrowed (struct type is borrow-safe).
			export func sum(n as Node) {
				def s as int init $n.v;
				for (def k in $n.kids) { $s = $s + sum($k); }
				return $s;
			}
		`,
		"main.j": `
			use io;
			import "./tree.j" as t;
			def leaf1 as t.Node init t.Node{v: 2, kids: []};
			def leaf2 as t.Node init t.Node{v: 3, kids: []};
			def root as t.Node init t.Node{v: 1, kids: [$leaf1, $leaf2]};
			io.printf("%d", t.sum($root));
		`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "6" {
		t.Fatalf("borrowed struct-param walk: got %q, want %q", out, "6")
	}
}

// Returning a borrowed parameter (which aliases the caller's binding) then
// mutating the caller's value must not be observable through the returned
// value: the receiving `def` eager-copies, so the two are independent.
func TestBorrowModuleReturnedParamIsCopiedAtCaller(t *testing.T) {
	out, err := runModuleMain(t, map[string]string{
		"m.j": `
			# xs never written -> borrowed; returning it hands back a value that
			# shares the caller's backing until the caller copies it.
			export func identity(xs as list of int) { return $xs; }
		`,
		"main.j": `
			use io;
			import "./m.j" as m;
			def xs as list of int init [1, 2, 3];
			def ys as list of int init m.identity($xs);
			$xs[0] = 99;
			io.printf("%d %d", $ys[0], $xs[0]);
		`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// ys[0]=1 (independent copy), xs[0]=99 (mutated). If the return aliased,
	// ys[0] would read 99.
	if out != "1 99" {
		t.Fatalf("returned-borrow independence: got %q, want %q", out, "1 99")
	}
}

// A module whose borrowed argument is a module constant (the other backing a
// module value can share) must never be mutated - consts are deep-const, so this
// is a smoke that the borrow path leaves the const intact across repeated calls.
func TestBorrowModuleConstArgIntact(t *testing.T) {
	out, err := runModuleMain(t, map[string]string{
		"m.j": `
			def const BASE as list of int init [10, 20, 30];
			export func at(xs as list of int, i as int) { return $xs[$i]; }
			export func probe() {
				return at(BASE, 0) + at(BASE, 1) + at(BASE, 2) + at(BASE, 0);
			}
		`,
		"main.j": `
			use io;
			import "./m.j" as m;
			io.printf("%d", m.probe());
		`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "70" || strings.Contains(out, "err") {
		t.Fatalf("borrowed const arg: got %q, want %q", out, "70")
	}
}
