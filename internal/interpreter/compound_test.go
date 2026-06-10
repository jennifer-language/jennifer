// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter_test

import (
	"strings"
	"testing"
)

// TestListBasic exercises list literal + index read + display + len.
func TestListBasic(t *testing.T) {
	out, err := run(t, `
use io;
def xs as list of int init [10, 20, 30];
printf("%d %d %d\n", $xs[0], $xs[1], $xs[2]);
printf("%d\n", len($xs));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "10 20 30\n3\n" {
		t.Errorf("got %q", out)
	}
}

func TestListIndexWrite(t *testing.T) {
	out, err := run(t, `
use io;
def xs as list of int init [1, 2, 3];
$xs[1] = 99;
printf("%d %d %d\n", $xs[0], $xs[1], $xs[2]);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "1 99 3\n" {
		t.Errorf("got %q", out)
	}
}

func TestListIndexOutOfBoundsRead(t *testing.T) {
	_, err := run(t, `
def xs as list of int init [1, 2];
def y as int init $xs[5];
`)
	if err == nil || !strings.Contains(err.Error(), "out of bounds") {
		t.Errorf("got %v", err)
	}
}

func TestListIndexOutOfBoundsWrite(t *testing.T) {
	_, err := run(t, `
def xs as list of int init [1, 2];
$xs[5] = 99;
`)
	if err == nil || !strings.Contains(err.Error(), "out of bounds") {
		t.Errorf("got %v", err)
	}
}

func TestListTypeMismatchInElement(t *testing.T) {
	_, err := run(t, `
def xs as list of int init [1, 2, 3];
$xs[0] = "nope";
`)
	if err == nil || !strings.Contains(err.Error(), "list element") {
		t.Errorf("got %v", err)
	}
}

func TestNestedListIndexWrite(t *testing.T) {
	out, err := run(t, `
use io;
def g as list of list of int init [[1, 2], [3, 4]];
$g[0][1] = 99;
printf("%d %d %d %d\n", $g[0][0], $g[0][1], $g[1][0], $g[1][1]);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "1 99 3 4\n" {
		t.Errorf("got %q", out)
	}
}

func TestMapBasic(t *testing.T) {
	out, err := run(t, `
use io;
def m as map of string to int init {"a": 1, "b": 2};
printf("%d %d\n", $m["a"], $m["b"]);
printf("%d\n", len($m));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "1 2\n2\n" {
		t.Errorf("got %q", out)
	}
}

func TestMapMissingKeyErrors(t *testing.T) {
	_, err := run(t, `
def m as map of string to int init {"a": 1};
def y as int init $m["missing"];
`)
	if err == nil || !strings.Contains(err.Error(), "no entry for key") {
		t.Errorf("got %v", err)
	}
}

func TestMapKeyWriteExtendsAndOrders(t *testing.T) {
	out, err := run(t, `
use io;
def m as map of string to int init {"a": 1};
$m["b"] = 2;
$m["c"] = 3;
$m["a"] = 99;
printf("%d %d %d\n", $m["a"], $m["b"], $m["c"]);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "99 2 3\n" {
		t.Errorf("got %q", out)
	}
}

func TestForEachList(t *testing.T) {
	out, err := run(t, `
use io;
def xs as list of int init [10, 20, 30];
for (def x in $xs) {
    printf("%d ", $x);
}
printf("\n");
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "10 20 30 \n" {
		t.Errorf("got %q", out)
	}
}

func TestForEachMapIteratesKeysInInsertionOrder(t *testing.T) {
	out, err := run(t, `
use io;
def m as map of string to int init {"first": 1, "second": 2, "third": 3};
for (def k in $m) {
    printf("%s=%d ", $k, $m[$k]);
}
printf("\n");
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "first=1 second=2 third=3 \n" {
		t.Errorf("got %q", out)
	}
}

func TestForEachMapInsertionOrderAfterMutation(t *testing.T) {
	// Insertion order is determined by *first* insertion; updating an
	// existing key doesn't move it. New keys append at the end.
	out, err := run(t, `
use io;
def m as map of string to int init {"a": 1, "b": 2};
$m["a"] = 99;
$m["c"] = 3;
for (def k in $m) {
    printf("%s ", $k);
}
printf("\n");
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "a b c \n" {
		t.Errorf("got %q", out)
	}
}

func TestConstListRejectsReassign(t *testing.T) {
	_, err := run(t, `
def const NUMS as list of int init [1, 2, 3];
$NUMS = [4, 5];
`)
	if err == nil || !strings.Contains(err.Error(), "constant") {
		t.Errorf("got %v", err)
	}
}

func TestConstListRejectsIndexAssign(t *testing.T) {
	_, err := run(t, `
def const NUMS as list of int init [1, 2, 3];
$NUMS[0] = 99;
`)
	if err == nil || !strings.Contains(err.Error(), "constant") {
		t.Errorf("got %v", err)
	}
}

func TestConstNestedRejectsDeepWrite(t *testing.T) {
	_, err := run(t, `
def const G as list of list of int init [[1, 2], [3, 4]];
$G[0][1] = 99;
`)
	if err == nil || !strings.Contains(err.Error(), "constant") {
		t.Errorf("got %v", err)
	}
}

func TestListValueSemanticsAssign(t *testing.T) {
	// $ys = $xs copies. Mutating $ys must not change $xs.
	out, err := run(t, `
use io;
def xs as list of int init [1, 2, 3];
def ys as list of int init [0];
$ys = $xs;
$ys[0] = 99;
printf("%d %d\n", $xs[0], $ys[0]);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "1 99\n" {
		t.Errorf("got %q", out)
	}
}

func TestListValueSemanticsFunctionParam(t *testing.T) {
	// Passing $xs to a function copies. Mutating the parameter must not
	// change the caller's list.
	out, err := run(t, `
use io;
def xs as list of int init [1, 2, 3];
func mutate(ys as list of int) {
    $ys[0] = 999;
    return $ys[0];
}
def r as int init mutate($xs);
printf("returned=%d original=%d\n", $r, $xs[0]);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "returned=999 original=1\n" {
		t.Errorf("got %q", out)
	}
}

func TestZeroValueList(t *testing.T) {
	// Uninitialized list-of-int gets an empty list, not null.
	out, err := run(t, `
use io;
def xs as list of int;
printf("%d\n", len($xs));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "0\n" {
		t.Errorf("got %q", out)
	}
}

func TestZeroValueMap(t *testing.T) {
	out, err := run(t, `
use io;
def m as map of string to int;
printf("%d\n", len($m));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "0\n" {
		t.Errorf("got %q", out)
	}
}

func TestHasMap(t *testing.T) {
	out, err := run(t, `
use io;
use maps;
def m as map of string to int init {"a": 1};
printf("%t %t\n", maps.has($m, "a"), maps.has($m, "b"));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "true false\n" {
		t.Errorf("got %q", out)
	}
}

func TestStringsSplit(t *testing.T) {
	out, err := run(t, `
use io;
use strings;
def parts as list of string init split("a,b,c", ",");
printf("%d %s %s %s\n", len($parts), $parts[0], $parts[1], $parts[2]);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "3 a b c\n" {
		t.Errorf("got %q", out)
	}
}

func TestStringsChars(t *testing.T) {
	out, err := run(t, `
use io;
use strings;
def cs as list of string init chars("héllo");
printf("%d %s %s\n", len($cs), $cs[0], $cs[1]);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "5 h é\n" {
		t.Errorf("got %q", out)
	}
}

func TestStringsJoin(t *testing.T) {
	out, err := run(t, `
use io;
use strings;
def parts as list of string init ["a", "b", "c"];
printf("%s\n", join($parts, "-"));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "a-b-c\n" {
		t.Errorf("got %q", out)
	}
}

func TestStringsSplitJoinRoundTrip(t *testing.T) {
	out, err := run(t, `
use io;
use strings;
def src as string init "alpha,beta,gamma";
def parts as list of string init split($src, ",");
printf("%s\n", join($parts, ","));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "alpha,beta,gamma\n" {
		t.Errorf("got %q", out)
	}
}
