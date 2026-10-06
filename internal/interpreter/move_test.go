// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import "testing"

// Move-on-last-use (VarExpr.Move): `$v = <fresh literal embedding $v>` stores
// $v's backing by move instead of copying it, since the assignment overwrites
// the binding immediately after. These tests pin that the result never aliases
// the moved backing twice and that value semantics elsewhere are untouched - a
// wrong move would silently share mutable backing between two places.

// The headline win: incremental left-leaning tree construction is linear and
// correct (the quadratic copy of the accumulated tree is gone).
func TestMoveIncrementalTreeIsCorrect(t *testing.T) {
	out, err := runAllLibs(t, `
		use io;
		use convert;
		def enum E { Num { v as float }, Bin { op as string, parts as list of E } };
		func build(n as int) {
			def acc as E init E.Num{v: 0.0};
			for (def i in 0..$n) {
				$acc = E.Bin{op: "+", parts: [$acc, E.Num{v: convert.toFloat($i)}]};
			}
			return $acc;
		}
		func leaves(e as E) {
			match ($e) {
				when Num(x) { return 1; }
				when Bin(b) { def t as int init 0; for (def p in $b.parts) { $t = $t + leaves($p); } return $t; }
			}
			return 0;
		}
		io.printf("%d", leaves(build(500)));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "501" {
		t.Fatalf("tree build wrong: got %q, want %q", out, "501")
	}
}

// The double-occurrence hazard: when the target appears in two movable positions
// only the first is moved and the second is copied, so the two slots hold
// independent backings. Mutating one must not change the other. A recursive
// struct through a list field gives a mutable shape to test this.
func TestMoveDoubleOccurrenceStaysIndependent(t *testing.T) {
	out, err := runAllLibs(t, `
		use io;
		def struct Tree { tag as int, kids as list of Tree };
		func f() {
			def t as Tree init Tree{tag: 1, kids: []};
			$t = Tree{tag: 0, kids: [$t, $t]};
			$t.kids[0].tag = 99;
			return $t.kids[1].tag;
		}
		io.printf("%d", f());
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "1" {
		t.Fatalf("move aliased two slots: got %q, want %q (second slot must be an independent copy)", out, "1")
	}
}

// The same hazard through a map literal: `$m = {"self": $m, "copy": $m}` moves
// the first value and copies the second; mutating one must not reach the other.
func TestMoveMapDoubleOccurrenceIndependent(t *testing.T) {
	out, err := runAllLibs(t, `
		use io;
		def struct Node { tag as int, children as map of string to Node };
		func f() {
			def n as Node init Node{tag: 5, children: {}};
			$n = Node{tag: 0, children: {"a": $n, "b": $n}};
			$n.children["a"].tag = 42;
			return $n.children["b"].tag;
		}
		io.printf("%d", f());
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "5" {
		t.Fatalf("map move aliased: got %q, want %q", out, "5")
	}
}

// The moved value is correctly placed: after one wrap, the moved old value is
// readable at its new position with its original contents.
func TestMoveWrappedValuePreserved(t *testing.T) {
	out, err := runAllLibs(t, `
		use io;
		def struct Tree { tag as int, kids as list of Tree };
		func f() {
			def t as Tree init Tree{tag: 7, kids: []};
			$t = Tree{tag: 0, kids: [$t]};
			return $t.kids[0].tag;
		}
		io.printf("%d", f());
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "7" {
		t.Fatalf("moved value lost: got %q, want %q", out, "7")
	}
}

// A copy taken before the move-assign stays independent of the overwrite: the
// def copies (the source is written later, so it is not borrowed), so mutating
// the new structure must not reach the earlier snapshot.
func TestMoveEarlierCopyUnaffected(t *testing.T) {
	out, err := runAllLibs(t, `
		use io;
		def struct Tree { tag as int, kids as list of Tree };
		func f() {
			def t as Tree init Tree{tag: 3, kids: []};
			def snap as Tree init $t;
			$t = Tree{tag: 0, kids: [$t]};
			$t.kids[0].tag = 88;
			return $snap.tag;
		}
		io.printf("%d", f());
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "3" {
		t.Fatalf("earlier copy aliased by move: got %q, want %q", out, "3")
	}
}

// A second read of the target in a call argument within the same RHS sees the
// still-valid pre-assignment value (the binding is overwritten only after the
// whole RHS is built), and the moved slot keeps its contents.
func TestMoveSecondReadInCall(t *testing.T) {
	out, err := runAllLibs(t, `
		use io;
		def struct Tree { tag as int, kids as list of Tree };
		func tagOf(x as Tree) { return $x.tag; }
		func f() {
			def t as Tree init Tree{tag: 4, kids: []};
			$t = Tree{tag: tagOf($t), kids: [$t]};
			return $t.tag + $t.kids[0].tag;
		}
		io.printf("%d", f());
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// tag = tagOf(old t) = 4; kids[0] = old t (tag 4); 4 + 4 = 8.
	if out != "8" {
		t.Fatalf("second-read-in-call wrong: got %q, want %q", out, "8")
	}
}
