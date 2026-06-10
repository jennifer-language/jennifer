// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

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
for (def x in $xs) { printf("%d ", $x); }
printf("\n");
printf("len=%d\n", len($xs));
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

// --- lists library --------------------------------------------------

func TestListsPushPop(t *testing.T) {
	out, err := run(t, `
use io;
use lists;
def xs as list of int init [1, 2, 3];
$xs = lists.push($xs, 4);
printf("after push: len=%d last=%d\n", len($xs), lists.last($xs));
$xs = lists.pop($xs);
$xs = lists.pop($xs);
printf("after two pops: len=%d last=%d\n", len($xs), lists.last($xs));
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
printf("%s/%s\n", lists.first($xs), lists.last($xs));
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
printf("head=[%d, %d] tail=[%d, %d]\n", $front[0], $front[1], $back[0], $back[1]);
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
printf("orig: %d,%d,%d\n", $xs[0], $xs[1], $xs[2]);
printf("rev:  %d,%d,%d\n", $rev[0], $rev[1], $rev[2]);
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
for (def v in $s) { printf("%d ", $v); }
printf("\n");
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
for (def v in $s) { printf("%s ", $v); }
printf("\n");
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
printf("%t %t\n", lists.contains($xs, 20), lists.contains($xs, 99));
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
printf("len=%d first=%d last=%d\n", len($c), lists.first($c), lists.last($c));
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
printf("two: %d,%d\n", $two[0], $two[1]);
printf("tail: %d,%d\n", $tail[0], $tail[1]);
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
printf("xs len=%d ys len=%d\n", len($xs), len($ys));
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
for (def k in $ks) { printf("%s ", $k); }
printf("\n");
for (def v in $vs) { printf("%d ", $v); }
printf("\n");
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
printf("len=%d has(a)=%t has(b)=%t has(c)=%t\n",
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
printf("x=%d y=%d z=%d\n", $merged["x"], $merged["y"], $merged["z"]);
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
printf("a len=%d b len=%d\n", len($a), len($b));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "a len=1 b len=2\n" {
		t.Errorf("got %q", out)
	}
}
