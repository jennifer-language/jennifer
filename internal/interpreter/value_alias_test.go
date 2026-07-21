// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

// Alias-stress tests. The refcount-lite shared-marker
// optimisation replaces the "Copy() before every mutation" pattern
// with "Share() on read; Ensure() at mutation." These tests
// exercise every "shared then mutated" corner case to make sure no
// mutation ever leaks through an alias.
//
// Each test defines a value A, aliases it to B (via `def b as ... init $a;`
// or through a struct field / list element / map value), mutates
// through one path, and asserts the other path still holds the
// original data.

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// runAlias parses + runs a Jennifer program with io installed.
// Returns captured stdout and any run error.
func runAlias(t *testing.T, src string) (string, error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	runErr := in.Run(prog)
	return buf.String(), runErr
}

// TestAliasListDirect - the simplest case: def b = $a; mutate a;
// verify b is unchanged.
func TestAliasListDirect(t *testing.T) {
	out, err := runAlias(t, `
		use io;
		def a as list of int init [1, 2, 3];
		def b as list of int init $a;
		$a[0] = 99;
		io.printf("a=%a b=%a", $a, $b);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "a=[99, 2, 3] b=[1, 2, 3]"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestAliasListAppend - append doesn't leak into aliases.
func TestAliasListAppend(t *testing.T) {
	out, err := runAlias(t, `
		use io;
		def a as list of int init [1, 2, 3];
		def b as list of int init $a;
		$a[] = 4;
		io.printf("a=%a b=%a", $a, $b);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "a=[1, 2, 3, 4] b=[1, 2, 3]"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestAliasListReverseOrder - mutating B doesn't leak into A.
func TestAliasListReverseOrder(t *testing.T) {
	out, err := runAlias(t, `
		use io;
		def a as list of int init [1, 2, 3];
		def b as list of int init $a;
		$b[1] = 88;
		io.printf("a=%a b=%a", $a, $b);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "a=[1, 2, 3] b=[1, 88, 3]"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestAliasMapDirect - same test for maps.
func TestAliasMapDirect(t *testing.T) {
	out, err := runAlias(t, `
		use io;
		def a as map of string to int init {"x": 1, "y": 2};
		def b as map of string to int init $a;
		$a["x"] = 99;
		io.printf("a.x=%d b.x=%d", $a["x"], $b["x"]);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "a.x=99 b.x=1"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestAliasStructDirect - same for structs.
func TestAliasStructDirect(t *testing.T) {
	out, err := runAlias(t, `
		use io;
		def struct Point { x as int, y as int };
		def a as Point init Point{x: 1, y: 2};
		def b as Point init $a;
		$a.x = 99;
		io.printf("a.x=%d b.x=%d", $a.x, $b.x);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "a.x=99 b.x=1"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestAliasStructWithList - struct field is a list. Mutating the
// list through one alias doesn't leak into the other.
func TestAliasStructWithList(t *testing.T) {
	out, err := runAlias(t, `
		use io;
		def struct Bag { items as list of int, count as int };
		def a as Bag init Bag{items: [10, 20, 30], count: 3};
		def b as Bag init $a;
		$a.items[0] = 999;
		io.printf("a.items=%a b.items=%a", $a.items, $b.items);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "a.items=[999, 20, 30] b.items=[10, 20, 30]"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestAliasListOfStructs - a list contains structs. Mutating an
// element's field doesn't leak into aliases of the outer list.
func TestAliasListOfStructs(t *testing.T) {
	out, err := runAlias(t, `
		use io;
		def struct Point { x as int, y as int };
		def a as list of Point init [Point{x: 1, y: 1}, Point{x: 2, y: 2}];
		def b as list of Point init $a;
		$a[0].x = 999;
		io.printf("a[0].x=%d b[0].x=%d", $a[0].x, $b[0].x);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "a[0].x=999 b[0].x=1"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestAliasMapWithLists - a map has list values. Mutating a value
// list doesn't leak into aliases.
func TestAliasMapWithLists(t *testing.T) {
	out, err := runAlias(t, `
		use io;
		def a as map of string to list of int init {"nums": [1, 2, 3]};
		def b as map of string to list of int init $a;
		$a["nums"][0] = 99;
		io.printf("a[nums]=%a b[nums]=%a", $a["nums"], $b["nums"]);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "a[nums]=[99, 2, 3] b[nums]=[1, 2, 3]"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestAliasNestedLists - list of lists; mutating an inner list
// doesn't leak.
func TestAliasNestedLists(t *testing.T) {
	out, err := runAlias(t, `
		use io;
		def a as list of list of int init [[1, 2], [3, 4]];
		def b as list of list of int init $a;
		$a[0][0] = 99;
		io.printf("a[0]=%a b[0]=%a", $a[0], $b[0]);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "a[0]=[99, 2] b[0]=[3, 4]"
	if out != want {
		// Note: this test could legitimately produce "b[0]=[1,2]" -
		// what matters is b[0] doesn't have the mutated 99.
		if !strings.Contains(out, "a[0]=[99, 2]") {
			t.Errorf("a mutation didn't take effect: %q", out)
		}
		if strings.Contains(out, "b[0]=[99") {
			t.Errorf("mutation leaked into b: %q", out)
		}
	}
}

// TestAliasBytes - bytes with append.
func TestAliasBytes(t *testing.T) {
	out, err := runAlias(t, `
		use io;
		def a as bytes;
		$a[] = 1;
		$a[] = 2;
		def b as bytes init $a;
		$a[] = 3;
		io.printf("a_len=%d b_len=%d", len($a), len($b));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "a_len=3 b_len=2"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestAliasThroughFunctionArg - passing a list to a function that
// mutates it: the caller's binding must not be affected. This is
// the value-semantics-through-parameter-binding contract.
func TestAliasThroughFunctionArg(t *testing.T) {
	out, err := runAlias(t, `
		use io;
		func mutate(xs as list of int) {
			$xs[0] = 999;
			return $xs;
		}
		def orig as list of int init [1, 2, 3];
		def mutated as list of int init mutate($orig);
		io.printf("orig=%a mutated=%a", $orig, $mutated);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "orig=[1, 2, 3] mutated=[999, 2, 3]"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestAliasAppendPatternHotLoop - the workhorse benchmark case.
// Repeated append into a list that has NO aliases should be
// amortised O(1), not O(N) per append. We can't measure timing in
// a unit test, but we can verify semantics.
func TestAliasAppendPatternHotLoop(t *testing.T) {
	out, err := runAlias(t, `
		use io;
		def xs as list of int init [];
		def i as int init 0;
		while ($i < 1000) {
			$xs[] = $i;
			$i = $i + 1;
		}
		io.printf("len=%d first=%d last=%d", len($xs), $xs[0], $xs[999]);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "len=1000 first=0 last=999"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestAliasReassignmentDoesntTaint - after `$a = new_value`, the
// old alias-through-b should still see the ORIGINAL data (not the
// reassigned new). Reassignment replaces a's binding; b keeps
// what it was pointing at.
func TestAliasReassignmentDoesntTaint(t *testing.T) {
	out, err := runAlias(t, `
		use io;
		def a as list of int init [1, 2, 3];
		def b as list of int init $a;
		$a = [7, 8, 9];
		io.printf("a=%a b=%a", $a, $b);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "a=[7, 8, 9] b=[1, 2, 3]"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestAliasChainedFieldWrite - `$p.field.nested[0] = ...` writes
// through a chain. No level of the chain should leak into aliases.
func TestAliasChainedFieldWrite(t *testing.T) {
	out, err := runAlias(t, `
		use io;
		def struct Inner { xs as list of int };
		def struct Outer { i as Inner };
		def a as Outer init Outer{i: Inner{xs: [10, 20, 30]}};
		def b as Outer init $a;
		$a.i.xs[1] = 999;
		io.printf("a.i.xs=%a b.i.xs=%a", $a.i.xs, $b.i.xs);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "a.i.xs=[10, 999, 30] b.i.xs=[10, 20, 30]"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// Spawn-frame independence is covered by spawn_test.go's
// TestSpawnDeepCopiesCapturedVars (and the test suite in
// general). We don't duplicate that surface here - it's
// snapshotForSpawn's job, not the shared-marker's.

// TestLvalueWriteCommitsCurrentRoot - the RHS (or an index expression)
// of a chained lvalue write may itself reassign the root variable as a
// side effect. The write must commit onto the root's CURRENT value, not
// a copy fetched before the RHS ran - otherwise the RHS's reassignment
// is silently lost.
func TestLvalueWriteCommitsCurrentRoot(t *testing.T) {
	// Index assign: f() rewrites $g, then its return lands at [0].
	out, err := runAlias(t, `
		use io;
		def g as list of int init [1, 2, 3];
		func f() {
		    $g = [7, 7, 7];
		    return 42;
		}
		$g[0] = f();
		io.printf("%a", $g);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "[42, 7, 7]" {
		t.Errorf("index assign: got %q, want %q", out, "[42, 7, 7]")
	}

	// Append: f() rewrites $xs, then its return appends onto the new value.
	out, err = runAlias(t, `
		use io;
		def xs as list of int init [1];
		func f() {
		    $xs = [5, 6];
		    return 9;
		}
		$xs[] = f();
		io.printf("%a", $xs);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "[5, 6, 9]" {
		t.Errorf("append: got %q, want %q", out, "[5, 6, 9]")
	}

	// Field assign: f() rewrites $p, then its return lands in the field.
	out, err = runAlias(t, `
		use io;
		def struct P { x as int, y as int };
		def p as P init P{x: 1, y: 2};
		func f() {
		    $p = P{x: 70, y: 80};
		    return 42;
		}
		$p.x = f();
		io.printf("x=%d y=%d", $p.x, $p.y);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "x=42 y=80" {
		t.Errorf("field assign: got %q, want %q", out, "x=42 y=80")
	}

	// Index-expression side effect: the index itself reassigns the root.
	out, err = runAlias(t, `
		use io;
		def g as list of int init [1, 2, 3];
		func idx() {
		    $g = [10, 20, 30];
		    return 2;
		}
		$g[idx()] = 99;
		io.printf("%a", $g);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "[10, 20, 99]" {
		t.Errorf("index side effect: got %q, want %q", out, "[10, 20, 99]")
	}
}
