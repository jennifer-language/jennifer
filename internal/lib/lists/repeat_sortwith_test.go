// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package listslib

import (
	"errors"
	"math"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

func TestReplicate(t *testing.T) {
	got := okCall(t, replicateFn, intList(1, 0), interpreter.IntVal(3))
	want := []int64{1, 0, 1, 0, 1, 0}
	if len(got.List) != len(want) {
		t.Fatalf("replicate([1, 0], 3) has %d elements, want %d", len(got.List), len(want))
	}
	for i, w := range want {
		if got.List[i].Int != w {
			t.Errorf("replicate([1, 0], 3)[%d] = %d, want %d", i, got.List[i].Int, w)
		}
	}
	if v := okCall(t, replicateFn, intList(1, 2), interpreter.IntVal(0)); len(v.List) != 0 {
		t.Errorf("replicate(xs, 0) = %v, want []", v.List)
	}
	if v := okCall(t, replicateFn, intList(), interpreter.IntVal(math.MaxInt64)); len(v.List) != 0 {
		t.Errorf("replicate([], huge) = %v, want []", v.List)
	}
	errCall(t, "replicate negative", replicateFn, intList(1), interpreter.IntVal(-1))
	errCall(t, "replicate float count", replicateFn, intList(1), interpreter.FloatVal(2))
	errCall(t, "replicate non-list", replicateFn, interpreter.IntVal(1), interpreter.IntVal(2))
	errCall(t, "replicate arity", replicateFn, intList(1))
}

// TestReplicateDeepCopies: replicate([[0, 0]], 3) is three independent rows.
func TestReplicateDeepCopies(t *testing.T) {
	src := `
use io;
use lists;
def grid as list of list of int init lists.replicate([[0, 0]], 3);
$grid[1][0] = 7;
io.printf("%d %d %d %d\n", len($grid), $grid[0][0], $grid[1][0], $grid[2][0]);
`
	if got := runLists(t, src); got != "3 0 7 0\n" {
		t.Errorf("replicate rows: got %q, want \"3 0 7 0\\n\"", got)
	}
}

// TestReplicateCap: a result over the concat cap is kind "limit" before anything
// is built, including a count whose product with the length would overflow; a
// result exactly at the cap is fine.
func TestReplicateCap(t *testing.T) {
	defer func(n int) { maxListElements = n }(maxListElements)
	maxListElements = 10
	if v := okCall(t, replicateFn, intList(1, 2), interpreter.IntVal(5)); len(v.List) != 10 {
		t.Errorf("replicate at the cap: %d elements, want 10", len(v.List))
	}
	var ce *limits.CapError
	for _, n := range []int64{6, math.MaxInt64, math.MaxInt64 / 2} {
		if _, err := replicateFn(interpreter.BuiltinCtx{}, []interpreter.Value{intList(1, 2), interpreter.IntVal(n)}); !errors.As(err, &ce) {
			t.Errorf("replicate(2 elements, %d): %T %v, want a limit error", n, err, err)
		}
	}
}

// TestReplicateErrorKinds: the cap reaches a program as kind "limit"; a negative
// count is kind "runtime".
func TestReplicateErrorKinds(t *testing.T) {
	src := `
use io;
use lists;
try {
    def xs as list of int init lists.replicate([1, 2, 3], 9223372036854775807);
    io.printf("no throw\n");
} catch (e) {
    io.printf("%s\n", $e.kind);
}
try {
    def ys as list of int init lists.replicate([1], -1);
    io.printf("no throw\n");
} catch (e) {
    io.printf("%s\n", $e.kind);
}
`
	if got := runLists(t, src); got != "limit\nruntime\n" {
		t.Errorf("replicate errors: got %q, want \"limit\\nruntime\\n\"", got)
	}
}

// TestSortWith: descending, a two-key order (descending length, then
// ascending), stability, and no comparator call for an empty or one-element
// list.
func TestSortWith(t *testing.T) {
	src := `
use io;
use lists;
def calls as int init 0;
func desc(a as int, b as int) { return $b - $a; }
func byLenThenAsc(a as string, b as string) {
    if (len($a) != len($b)) { return len($b) - len($a); }
    if ($a < $b) { return -1; }
    if ($a > $b) { return 1; }
    return 0;
}
func byFirst(a as string, b as string) {
    if ($a[0..1] < $b[0..1]) { return -1; }
    if ($a[0..1] > $b[0..1]) { return 1; }
    return 0;
}
func counting(a as int, b as int) { $calls = $calls + 1; return $a - $b; }
func show(xs as list of string) {
    for (def x in $xs) { io.printf("%s ", $x); }
    io.printf("\n");
}
def d as list of int init lists.sortWith([3, 1, 4, 1, 5, 9, 2, 6], desc);
for (def x in $d) { io.printf("%d ", $x); }
io.printf("\n");
show(lists.sortWith(["this", "is", "a", "set", "of", "strings", "to", "sort"], byLenThenAsc));
show(lists.sortWith(["b2", "a1", "b1", "a2", "a3", "b3"], byFirst));
def e as list of int init lists.sortWith([], counting);
def one as list of int init lists.sortWith([42], counting);
io.printf("%d %d %d calls=%d\n", len($e), len($one), $one[0], $calls);
`
	want := "9 6 5 4 3 2 1 1 \n" +
		"strings sort this set is of to a \n" +
		"a1 a2 a3 b2 b1 b3 \n" +
		"0 1 42 calls=0\n"
	if got := runLists(t, src); got != want {
		t.Errorf("sortWith:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestSortWithErrors: a throwing comparator's error reaches the caller with
// its own kind; a non-int result is a runtime error.
func TestSortWithErrors(t *testing.T) {
	src := `
use io;
use lists;
func boom(a as int, b as int) { throw Error{kind: "bad", message: "no"}; }
func notInt(a as int, b as int) { return true; }
try {
    def xs as list of int init lists.sortWith([3, 1, 2], boom);
    io.printf("no throw\n");
} catch (e) {
    io.printf("%s\n", $e.kind);
}
try {
    def ys as list of int init lists.sortWith([3, 1, 2], notInt);
    io.printf("no throw\n");
} catch (e) {
    io.printf("%s\n", $e.kind);
}
`
	if got := runLists(t, src); got != "bad\nruntime\n" {
		t.Errorf("sortWith errors: got %q, want \"bad\\nruntime\\n\"", got)
	}
}

// TestSortWithInconsistentComparator: a comparator that is not an order
// (always "a first", or answers that cycle regardless of the arguments) gives
// a permutation of the input - no crash, no lost or duplicated element.
func TestSortWithInconsistentComparator(t *testing.T) {
	src := `
use io;
use lists;
def n as int init 0;
func always(a as int, b as int) { return -1; }
func cycling(a as int, b as int) { $n = $n + 1; return $n * 7 % 3 - 1; }
def xs as list of int init lists.range(0, 200);
def a as list of int init lists.sortWith($xs, always);
def b as list of int init lists.sortWith($xs, cycling);
io.printf("%d %d %t %t\n", len($a), len($b), lists.sort($a) == $xs, lists.sort($b) == $xs);
`
	if got := runLists(t, src); got != "200 200 true true\n" {
		t.Errorf("inconsistent comparator: got %q", got)
	}
}
