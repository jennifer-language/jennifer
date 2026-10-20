// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

// Value-semantics tests for the two ownership elisions (parser/ownership.go):
// a call result the callee solely owns is stored without a copy, and the
// argument of `$v = f($v, ...)` moves into the parameter. Each case pins what a
// copy-everywhere implementation prints, including the paths where an elision
// must NOT happen (a try in the same method, a global, a borrowed parameter, a
// for-each iterator) because the old value stays observable.

import (
	"testing"
)

const mutators = `
func bump(xs as list of int, i as int) {
    $xs[$i] = $xs[$i] + 1;
    return $xs;
}
func bumpThenFail(xs as list of int) {
    $xs[0] = 99;
    throw Error{kind: "test", message: "boom"};
}
`

func TestOwnershipValueSemantics(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"moved argument mutated and returned", `
func run() {
    def a as list of int init [1, 2, 3];
    $a = bump($a, 0);
    $a = bump($a, 0);
    def keep as list of int init $a;
    $a = bump($a, 1);
    io.printf("a=%a keep=%a\n", $a, $keep);
    return;
}
run();`, "a=[3, 3, 3] keep=[3, 2, 3]\n"},

		{"try in the same method keeps the old value", `
func run() {
    def a as list of int init [1, 2];
    try {
        $a = bumpThenFail($a);
    } catch (e) {
        io.printf("caught a=%a\n", $a);
    }
    return;
}
run();`, "caught a=[1, 2]\n"},

		{"a global is never moved", `
def g as list of int init [1, 2];
func h() {
    $g = bumpThenFail($g);
    return;
}
try {
    h();
} catch (e) {
    io.printf("g=%a\n", $g);
}`, "g=[1, 2]\n"},

		{"top-level code is never moved", `
def a as list of int init [1, 2];
try {
    $a = bumpThenFail($a);
} catch (e) {
    io.printf("a=%a\n", $a);
}`, "a=[1, 2]\n"},

		{"another target leaves the argument alone", `
func run() {
    def a as list of int init [1, 2];
    def b as list of int init bump($a, 0);
    $b = bump($b, 0);
    io.printf("a=%a b=%a\n", $a, $b);
    return;
}
run();`, "a=[1, 2] b=[3, 2]\n"},

		{"a variable named twice is not moved", `
func pair(x as list of int, y as list of int) {
    $x[0] = 99;
    return $y;
}
func run() {
    def a as list of int init [1, 2];
    $a = pair($a, $a);
    io.printf("a=%a\n", $a);
    return;
}
run();`, "a=[1, 2]\n"},

		{"owned local results are independent", `
func mk() {
    def x as list of int init [1];
    return $x;
}
def p as list of int init mk();
def q as list of int init mk();
$p[0] = 5;
io.printf("p=%a q=%a\n", $p, $q);`, "p=[5] q=[1]\n"},

		{"a returned global is copied", `
def g as list of int init [1, 2];
func get() {
    return $g;
}
def y as list of int init get();
$y[0] = 9;
def z as list of int init [0];
$z = get();
$z[1] = 8;
io.printf("g=%a y=%a z=%a\n", $g, $y, $z);`, "g=[1, 2] y=[9, 2] z=[1, 8]\n"},

		{"a returned borrowed parameter is copied", `
def struct S { items as list of int };
func id(p as list of int) {
    return $p;
}
func items(s as S) {
    return $s.items;
}
func run() {
    def x as list of int init [1, 2];
    def y as list of int init id($x);
    $y[0] = 9;
    def s as S init S{items: [3, 4]};
    def got as list of int init items($s);
    $got[0] = 9;
    io.printf("x=%a y=%a s=%a got=%a\n", $x, $y, $s.items, $got);
    return;
}
run();`, "x=[1, 2] y=[9, 2] s=[3, 4] got=[9, 4]\n"},

		{"a moved argument returned through a borrowed parameter", `
func idp(p as list of int) {
    return $p;
}
func run() {
    def a as list of int init [1, 2];
    $a = idp($a);
    $a = bump($a, 1);
    io.printf("a=%a\n", $a);
    return;
}
run();`, "a=[1, 3]\n"},

		{"a returned for-each iterator is copied", `
def g as list of list of int init [[1], [2]];
func firstOf() {
    for (def x in $g) {
        return $x;
    }
    return [];
}
def f as list of int init firstOf();
$f[0] = 9;
io.printf("g=%a f=%a\n", $g, $f);`, "g=[[1], [2]] f=[9]\n"},

		{"a moved parameter stashed in a global", `
def g as list of int init [];
func stash(p as list of int) {
    $g = $p;
    $p[0] = 7;
    return $p;
}
func run() {
    def a as list of int init [1, 2];
    $a = stash($a);
    io.printf("a=%a g=%a\n", $a, $g);
    return;
}
run();`, "a=[7, 2] g=[1, 2]\n"},

		{"a recursive builder returns owned values", `
func build(n as int) {
    if ($n == 0) {
        return [0];
    }
    def r as list of int init build($n - 1);
    $r[] = $n;
    return $r;
}
def a as list of int init build(3);
def b as list of int init build(3);
$a[0] = 9;
io.printf("a=%a b=%a\n", $a, $b);`, "a=[9, 1, 2, 3] b=[0, 1, 2, 3]\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runAlias(t, "use io;\n"+mutators+tc.src)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out != tc.want {
				t.Errorf("got %q\nwant %q", out, tc.want)
			}
		})
	}
}

// The same elisions across a module boundary: a module function that mutates
// and returns its struct parameter, called as `$b = m.f($b, ...)` from a method.
func TestOwnershipAcrossModule(t *testing.T) {
	files := map[string]string{
		"grid.j": `
export def struct Grid { cells as list of string };
export func make(n as int) {
    def cells as list of string init [];
    for (def i in 0..$n) {
        $cells[] = ".";
    }
    return Grid{cells: $cells};
}
export func put(g as Grid, i as int, s as string) {
    $g.cells[$i] = $s;
    return $g;
}
export func putThenFail(g as Grid, i as int) {
    $g.cells[$i] = "!";
    throw Error{kind: "test", message: "boom"};
}`,
		"main.j": `
use io;
import "./grid.j";
func run() {
    def g as grid.Grid init grid.make(3);
    $g = grid.put($g, 0, "a");
    def keep as grid.Grid init $g;
    $g = grid.put($g, 1, "b");
    def other as grid.Grid init grid.put($g, 2, "c");
    try {
        $g = grid.putThenFail($g, 2);
    } catch (e) {
        io.printf("caught ");
    }
    io.printf("g=%a keep=%a other=%a\n", $g.cells, $keep.cells, $other.cells);
    return;
}
run();`,
	}
	out, err := runModuleMain(t, files)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "caught g=[a, b, .] keep=[a, ., .] other=[a, b, c]\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}
