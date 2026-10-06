// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import "testing"

// Local-binding borrow (DefineStmt.Borrow): an in-method `def x as T init EXPR;`
// that is never written and whose initializer aliases only never-written roots
// is bound by alias instead of deep-copied, under the same borrow gate as a
// read-only parameter. These tests pin the value-semantics parity (the alias
// must be observationally identical to a copy) and every disqualifier, since a
// wrong borrow here silently returns stale or corrupted data.

// The headline win: a read-only local bound from a field/index chain through a
// read-only parameter aliases rather than copies, and reads correctly.
func TestDefBorrowReadOnlyLocalIsCorrect(t *testing.T) {
	out, err := runAllLibs(t, `
		use io;
		use maps;
		def struct Sh { name as string, cells as map of string to int };
		def struct Bk { sheets as list of Sh };
		func byName(b as Bk, nm as string) {
			for (def i in 0..len($b.sheets)) {
				if ($b.sheets[$i].name == $nm) { return $b.sheets[$i]; }
			}
			return $b.sheets[0];
		}
		func cellOf(s as Sh, key as string) {
			if (maps.has($s.cells, $key)) { return $s.cells[$key]; }
			return 0;
		}
		func read(bk as Bk, nm as string, key as string) {
			def s as Sh init byName($bk, $nm);
			return cellOf($s, $key);
		}
		def bk as Bk init Bk{sheets: [Sh{name: "a", cells: {"x": 7}}, Sh{name: "b", cells: {"x": 9}}]};
		io.printf("%d", read($bk, "b", "x"));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "9" {
		t.Fatalf("borrowed local read wrong: got %q, want %q", out, "9")
	}
}

// A borrowed local must not let a mutation leak. Here the local would alias the
// source list, but the source is written later in the method, so the resolver
// must NOT borrow (the root is written), and the read sees the pre-write value.
func TestDefBorrowSourceWrittenLaterCopies(t *testing.T) {
	out, err := runAllLibs(t, `
		use io;
		func f() {
			def a as list of int init [1, 2, 3];
			def s as list of int init $a;
			$a[0] = 99;
			return $s[0];
		}
		io.printf("%d", f());
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "1" {
		t.Fatalf("value semantics broken: got %q, want %q (source written later must force a copy)", out, "1")
	}
}

// The param variant of the same hazard: a local aliasing a parameter whose
// backing the method then index-writes must copy, not borrow.
func TestDefBorrowParamWrittenLaterCopies(t *testing.T) {
	out, err := runAllLibs(t, `
		use io;
		def g as list of int init [5, 6, 7];
		func f(p as list of int) {
			def s as list of int init $p;
			$p[0] = 99;
			return $s[0];
		}
		io.printf("%d", f($g));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "5" {
		t.Fatalf("value semantics broken: got %q, want %q", out, "5")
	}
}

// The borrow gate: a method that is NOT globals-safe (it transitively mutates a
// global) must copy its defs even though the initializer statically looks
// borrowable, because the aliased global could be mutated during the binding's
// life. DefineStmt.Borrow may be set statically; env.borrowDefs must veto it.
func TestDefBorrowGateNonGlobalSafeMethodCopies(t *testing.T) {
	out, err := runAllLibs(t, `
		use io;
		def g as list of int init [1, 2, 3];
		func rd() { return $g; }
		func mutate() { $g[0] = 99; }
		func f() {
			def s as list of int init rd();
			mutate();
			return $s[0];
		}
		io.printf("%d", f());
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "1" {
		t.Fatalf("borrow gate broken: got %q, want %q (non-globals-safe method must copy)", out, "1")
	}
}

// A globals-safe method in a script that holds a mutable global elsewhere SHOULD
// borrow a local read from that global, and still read correctly - the global is
// not mutated during the globals-safe method's run.
func TestDefBorrowGlobalSafeMethodReadsGlobalCorrectly(t *testing.T) {
	out, err := runAllLibs(t, `
		use io;
		def g as list of int init [1, 2, 3];
		func rd() { return $g; }
		func f() {
			def s as list of int init rd();
			return $s[1];
		}
		io.printf("%d", f());
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "2" {
		t.Fatalf("globals-safe borrow read wrong: got %q, want %q", out, "2")
	}
}

// A borrowed local handed to a mutator must not corrupt the source: the mutator
// copies its own (non-borrowed) parameter, so the original structure the local
// aliases is untouched.
func TestDefBorrowedLocalPassedToMutatorKeepsSource(t *testing.T) {
	out, err := runAllLibs(t, `
		use io;
		func bump(xs as list of int) { $xs[0] = $xs[0] + 100; return $xs[0]; }
		func f(src as list of int) {
			def s as list of int init $src;
			def got as int init bump($s);
			return $s[0] + $got;
		}
		def base as list of int init [1, 2, 3];
		io.printf("%d", f($base));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// s[0] stays 1 (bump mutated its own copy); got is 101.
	if out != "102" {
		t.Fatalf("borrowed local corrupted by mutator: got %q, want %q", out, "102")
	}
}

// Item 4: a match subject must be a variable, which forces a bind; that bind now
// borrows, so matching a nested enum value costs no copy and still dispatches
// correctly.
func TestDefBorrowMatchSubject(t *testing.T) {
	out, err := runAllLibs(t, `
		use io;
		def enum Cell { Empty, Num { v as int } };
		def struct Sheet { cells as list of Cell };
		func valueAt(sh as Sheet, idx as int) {
			def c as Cell init $sh.cells[$idx];
			match ($c) {
				when Num(n) { return $n.v; }
				when Empty { return -1; }
			}
			return -2;
		}
		def sh as Sheet init Sheet{cells: [Cell.Empty, Cell.Num{v: 42}]};
		io.printf("%d %d", valueAt($sh, 1), valueAt($sh, 0));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "42 -1" {
		t.Fatalf("match-on-borrowed-subject wrong: got %q, want %q", out, "42 -1")
	}
}

// A top-level `def` (a global) is never borrowed: its frame is the global root,
// where borrowDefs is false, so the whole-value copy that keeps globals
// independent still happens.
func TestDefBorrowTopLevelGlobalStillCopies(t *testing.T) {
	out, err := runAllLibs(t, `
		use io;
		def a as list of int init [1, 2, 3];
		def b as list of int init $a;
		$a[0] = 99;
		io.printf("%d", $b[0]);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "1" {
		t.Fatalf("top-level global aliased: got %q, want %q", out, "1")
	}
}
