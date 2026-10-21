// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// --- Append form ($xs[] = item) --------------------------------------

func TestAppendFormGrowsList(t *testing.T) {
	out, err := run(t, `
use io;
def xs as list of int init [10, 20];
$xs[] = 30;
$xs[] = 40;
for (def x in $xs) { io.printf("%d ", $x); }
io.printf("\n");
io.printf("len=%d\n", len($xs));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "10 20 30 40 \nlen=4\n" {
		t.Errorf("got %q", out)
	}
}

func TestAppendFormChecksElementType(t *testing.T) {
	_, err := run(t, `
def xs as list of int init [];
$xs[] = "oops";
`)
	if err == nil {
		t.Fatal("expected type error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot append string to list") {
		t.Errorf("err = %v", err)
	}
}

func TestAppendFormRejectsConst(t *testing.T) {
	_, err := run(t, `
def const NUMS as list of int init [1, 2];
$NUMS[] = 3;
`)
	if err == nil {
		t.Fatal("expected const error, got nil")
	}
	if !strings.Contains(err.Error(), "const is deep") {
		t.Errorf("err = %v", err)
	}
}

func TestAppendFormRejectsNonList(t *testing.T) {
	_, err := run(t, `
def m as map of string to int init {};
$m[] = 1;
`)
	if err == nil {
		t.Fatal("expected non-list error, got nil")
	}
	if !strings.Contains(err.Error(), "requires a list") {
		t.Errorf("err = %v", err)
	}
}

// TestAppendFormChained covers `[] =` at the end of an lvalue chain: a struct
// field, a nested list, a map value, bytes in a struct, a module-free struct
// in a list - each grows in place, keeps value semantics (a copy taken before
// the append does not see it) and type-checks the element.
func TestAppendFormChained(t *testing.T) {
	out, err := run(t, `
use io;
def struct Bag { items as list of int, raw as bytes, groups as map of string to list of string };
def b as Bag;
def before as Bag init $b;
$b.items[] = 1;
$b.items[] = 2;
$b.raw[] = 255;
$b.groups["a"] = [];
$b.groups["a"][] = "x";
def grid as list of list of int init [[], [9]];
$grid[1][] = 10;
def bags as list of Bag init [$b];
$bags[0].items[] = 3;
io.printf("%v %d %v %v %v %v\n", $b.items, len($b.raw), $b.groups, $grid, $bags[0].items, $before.items);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if want := "[1, 2] 1 {\"a\": [\"x\"]} [[], [9, 10]] [1, 2, 3] []\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	// A parameter written only through a chained append is the callee's own
	// copy (never borrowed): the caller's value is unchanged.
	out, err = run(t, `
use io;
def struct Bag { items as list of int };
func f(s as Bag) { $s.items[] = 1; return len($s.items); }
func g(xs as list of list of int) { $xs[0][] = 7; return len($xs[0]); }
func main() {
    def b as Bag init Bag{items: [5]};
    def l as list of list of int init [[1]];
    io.printf("%d %d | %d %d\n", f($b), len($b.items), g($l), len($l[0]));
}
main();
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "2 1 | 2 1\n" {
		t.Errorf("chained append through a parameter leaked to the caller: %q", out)
	}
	for _, c := range []struct{ src, want string }{
		{"def struct B { items as list of int };\ndef b as B;\n$b.items[] = \"s\";", "cannot append string to list"},
		{"def struct B { n as int };\ndef b as B;\n$b.n[] = 1;", "requires a list or bytes, got int"},
		{"def struct B { items as list of int };\ndef const C as B init B{items: []};\nfunc f() { $C.items[] = 1; }\nf();", "const"},
		{"def grid as list of list of int init [[]];\n$grid[3][] = 1;", "out of bounds"},
		{"def struct B { raw as bytes };\ndef b as B;\n$b.raw[] = 256;", "out of range"},
	} {
		if _, err := run(t, c.src); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: err = %v, want it to contain %q", c.src, err, c.want)
		}
	}
}

// --- lists library --------------------------------------------------

func TestListsPushPop(t *testing.T) {
	out, err := run(t, `
use io;
use lists;
def xs as list of int init [1, 2, 3];
$xs = lists.push($xs, 4);
io.printf("after push: len=%d last=%d\n", len($xs), lists.last($xs));
$xs = lists.pop($xs);
$xs = lists.pop($xs);
io.printf("after two pops: len=%d last=%d\n", len($xs), lists.last($xs));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "after push: len=4 last=4\nafter two pops: len=2 last=2\n" {
		t.Errorf("got %q", out)
	}
}

func TestListsFirstLast(t *testing.T) {
	out, err := run(t, `
use io;
use lists;
def xs as list of string init ["a", "b", "c"];
io.printf("%s/%s\n", lists.first($xs), lists.last($xs));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "a/c\n" {
		t.Errorf("got %q", out)
	}
}

func TestListsFirstEmptyErrors(t *testing.T) {
	_, err := run(t, `use lists; def xs as list of int init []; lists.first($xs);`)
	if err == nil || !strings.Contains(err.Error(), "list is empty") {
		t.Errorf("err = %v", err)
	}
}

func TestListsHeadTail(t *testing.T) {
	out, err := run(t, `
use io;
use lists;
def xs as list of int init [1, 2, 3, 4, 5];
def front as list of int init lists.head($xs, 2);
def back as list of int init lists.tail($xs, 2);
io.printf("head=[%d, %d] tail=[%d, %d]\n", $front[0], $front[1], $back[0], $back[1]);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "head=[1, 2] tail=[4, 5]\n" {
		t.Errorf("got %q", out)
	}
}

func TestListsReverse(t *testing.T) {
	out, err := run(t, `
use io;
use lists;
def xs as list of int init [1, 2, 3];
def rev as list of int init lists.reverse($xs);
io.printf("orig: %d,%d,%d\n", $xs[0], $xs[1], $xs[2]);
io.printf("rev:  %d,%d,%d\n", $rev[0], $rev[1], $rev[2]);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "orig: 1,2,3\nrev:  3,2,1\n" {
		t.Errorf("got %q", out)
	}
}

func TestListsSortInts(t *testing.T) {
	out, err := run(t, `
use io;
use lists;
def xs as list of int init [3, 1, 4, 1, 5, 9, 2, 6];
def s as list of int init lists.sort($xs);
for (def v in $s) { io.printf("%d ", $v); }
io.printf("\n");
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "1 1 2 3 4 5 6 9 \n" {
		t.Errorf("got %q", out)
	}
}

func TestListsSortStrings(t *testing.T) {
	out, err := run(t, `
use io;
use lists;
def xs as list of string init ["c", "a", "b"];
def s as list of string init lists.sort($xs);
for (def v in $s) { io.printf("%s ", $v); }
io.printf("\n");
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "a b c \n" {
		t.Errorf("got %q", out)
	}
}

func TestListsContains(t *testing.T) {
	out, err := run(t, `
use io;
use lists;
def xs as list of int init [10, 20, 30];
io.printf("%t %t\n", lists.contains($xs, 20), lists.contains($xs, 99));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "true false\n" {
		t.Errorf("got %q", out)
	}
}

func TestListsConcat(t *testing.T) {
	out, err := run(t, `
use io;
use lists;
def a as list of int init [1, 2];
def b as list of int init [3, 4, 5];
def c as list of int init lists.concat($a, $b);
io.printf("len=%d first=%d last=%d\n", len($c), lists.first($c), lists.last($c));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "len=5 first=1 last=5\n" {
		t.Errorf("got %q", out)
	}
}

func TestListsSlice(t *testing.T) {
	out, err := run(t, `
use io;
use lists;
def xs as list of int init [10, 20, 30, 40, 50];
def two as list of int init lists.slice($xs, 1, 3);
def tail as list of int init lists.slice($xs, 3);
io.printf("two: %d,%d\n", $two[0], $two[1]);
io.printf("tail: %d,%d\n", $tail[0], $tail[1]);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "two: 20,30\ntail: 40,50\n" {
		t.Errorf("got %q", out)
	}
}

func TestListsValueSemanticsOnInput(t *testing.T) {
	// `lists.push` returns a new list; the input must be untouched.
	out, err := run(t, `
use io;
use lists;
def xs as list of int init [1, 2];
def ys as list of int init lists.push($xs, 3);
io.printf("xs len=%d ys len=%d\n", len($xs), len($ys));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "xs len=2 ys len=3\n" {
		t.Errorf("got %q", out)
	}
}

// --- maps library ---------------------------------------------------

func TestMapsKeysValues(t *testing.T) {
	out, err := run(t, `
use io;
use maps;
def m as map of string to int init {"a": 1, "b": 2, "c": 3};
def ks as list of string init maps.keys($m);
def vs as list of int init maps.values($m);
for (def k in $ks) { io.printf("%s ", $k); }
io.printf("\n");
for (def v in $vs) { io.printf("%d ", $v); }
io.printf("\n");
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "a b c \n1 2 3 \n" {
		t.Errorf("got %q", out)
	}
}

func TestMapsDelete(t *testing.T) {
	out, err := run(t, `
use io;
use maps;
def m as map of string to int init {"a": 1, "b": 2, "c": 3};
def shrunk as map of string to int init maps.delete($m, "b");
io.printf("len=%d has(a)=%t has(b)=%t has(c)=%t\n",
    len($shrunk), maps.has($shrunk, "a"), maps.has($shrunk, "b"), maps.has($shrunk, "c"));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "len=2 has(a)=true has(b)=false has(c)=true\n" {
		t.Errorf("got %q", out)
	}
}

func TestMapsDeleteMissingErrors(t *testing.T) {
	_, err := run(t, `
use maps;
def m as map of string to int init {"a": 1};
def shrunk as map of string to int init maps.delete($m, "missing");
`)
	if err == nil || !strings.Contains(err.Error(), "no entry for key") {
		t.Errorf("err = %v", err)
	}
}

func TestMapsMerge(t *testing.T) {
	out, err := run(t, `
use io;
use maps;
def a as map of string to int init {"x": 1, "y": 2};
def b as map of string to int init {"y": 99, "z": 3};
def merged as map of string to int init maps.merge($a, $b);
io.printf("x=%d y=%d z=%d\n", $merged["x"], $merged["y"], $merged["z"]);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "x=1 y=99 z=3\n" {
		t.Errorf("got %q", out)
	}
}

func TestMapsValueSemanticsOnInput(t *testing.T) {
	out, err := run(t, `
use io;
use maps;
def a as map of string to int init {"x": 1};
def b as map of string to int init maps.merge($a, {"y": 2});
io.printf("a len=%d b len=%d\n", len($a), len($b));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "a len=1 b len=2\n" {
		t.Errorf("got %q", out)
	}
}
