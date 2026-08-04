// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import "testing"

// TestFuncValueBareNameAndCall: a bare method name is a function value; call it
// through a variable.
func TestFuncValueBareNameAndCall(t *testing.T) {
	out, err := run(t, `
use io;
func greet(name as string) { return "hi " + $name; }
def f as func init greet;
io.printf("%s\n", $f("ada"));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "hi ada\n" {
		t.Errorf("got %q", out)
	}
}

// TestFuncValuePassedAndReturned: a func value passes into a method as a `func`
// param and is called inside.
func TestFuncValuePassedAndReturned(t *testing.T) {
	out, err := run(t, `
use io;
func double(n as int) { return $n * 2; }
func applyTo(fn as func, x as int) { return $fn($x); }
io.printf("%d\n", applyTo(double, 21));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "42\n" {
		t.Errorf("got %q", out)
	}
}

// TestFuncValueInListAndChainedCall: a func in a list, called through the index,
// and a returned func called immediately.
func TestFuncValueInListAndChainedCall(t *testing.T) {
	out, err := run(t, `
use io;
func inc(n as int) { return $n + 1; }
func picker() { return inc; }
def fns as list of func init [inc, inc];
io.printf("%d %d\n", $fns[0](10), picker()(41));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "11 42\n" {
		t.Errorf("got %q", out)
	}
}

// TestFuncValueRecursionGuardStillFires: deep recursion through a function value
// trips the catchable call-depth guard (not a fatal Go stack overflow).
func TestFuncValueRecursionGuardStillFires(t *testing.T) {
	_, err := run(t, `
func loop(n as int) { return loop($n + 1); }
def f as func init loop;
$f(0);
`)
	if err == nil {
		t.Fatal("expected a call-stack-too-deep error")
	}
	if !contains(err.Error(), "call stack too deep") {
		t.Errorf("got %q, want a call-stack-too-deep error", err.Error())
	}
}

// TestFuncValueErrors: the runtime rejections around function values.
func TestFuncValueErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"call a non-func", `def x as int init 5; $x(1);`, "only a `func` value is callable"},
		{"call the null zero", `def f as func; $f(1);`, "uninitialized `func` value"},
		{"arity mismatch", `func add(a as int, b as int) { return $a+$b; } def f as func init add; $f(1);`, "takes 2 argument(s), got 1"},
		{"arg type mismatch", `func dbl(n as int) { return $n*2; } def f as func init dbl; $f("x");`, "must be int, got string"},
		{"assign non-func to func var", `def f as func init 5;`, "func"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := run(t, c.src)
			if err == nil {
				t.Fatalf("expected an error")
			}
			if !contains(err.Error(), c.want) {
				t.Errorf("got %q, want it to contain %q", err.Error(), c.want)
			}
		})
	}
}

// TestFuncValueDisplay: a function value renders as `<func NAME>`.
func TestFuncValueDisplay(t *testing.T) {
	out, err := run(t, `
use io;
func greet() { return "x"; }
def f as func init greet;
io.printf("%v\n", $f);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "<func greet>\n" {
		t.Errorf("got %q", out)
	}
}

// TestHigherOrderLists exercises the map / filter / reduce / find / any / all /
// sortBy higher-order layer end to end.
func TestHigherOrderLists(t *testing.T) {
	out, err := run(t, `
use io;
use lists;
func dbl(n as int) { return $n * 2; }
func isEven(n as int) { return $n % 2 == 0; }
func add(a as int, b as int) { return $a + $b; }
func neg(n as int) { return 0 - $n; }

def xs as list of int init [1, 2, 3, 4, 5];
def doubled as list of int init lists.map($xs, dbl);
def evens as list of int init lists.filter($xs, isEven);
def sorted as list of int init lists.sortBy($xs, neg);
io.printf("%v %v %d %d %t %t %v\n",
    $doubled, $evens,
    lists.reduce($xs, add, 0),
    lists.find($xs, isEven),
    lists.any($xs, isEven), lists.all($xs, isEven),
    $sorted);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "[2, 4, 6, 8, 10] [2, 4] 15 2 true false [5, 4, 3, 2, 1]\n"
	if out != want {
		t.Errorf("got  %q\nwant %q", out, want)
	}
}

// TestHigherOrderMapTypeChecked: lists.map returns a generic list whose element
// type is validated at the binding - a wrong declared type is rejected.
func TestHigherOrderMapTypeChecked(t *testing.T) {
	// Mapping ints to strings; binding to `list of int` must fail.
	_, err := run(t, `
use lists;
func name(n as int) { return "x"; }
def bad as list of int init lists.map([1, 2], name);
`)
	if err == nil {
		t.Fatal("expected a type error binding list-of-string map result to list of int")
	}
}

// TestHigherOrderFindNoMatchCatchable: lists.find with no match raises a
// catchable error.
func TestHigherOrderFindNoMatchCatchable(t *testing.T) {
	out, err := run(t, `
use io;
use lists;
func big(n as int) { return $n > 100; }
try {
    lists.find([1, 2, 3], big);
} catch (e) {
    io.printf("caught\n");
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "caught\n" {
		t.Errorf("got %q", out)
	}
}

// TestFilterCallbackMustReturnBool: a filter callback that returns a non-bool is
// a positioned error.
func TestFilterCallbackMustReturnBool(t *testing.T) {
	_, err := run(t, `
use lists;
func idn(n as int) { return $n; }
lists.filter([1, 2, 3], idn);
`)
	if err == nil {
		t.Fatal("expected an error: filter callback must return bool")
	}
	if !contains(err.Error(), "must return bool") {
		t.Errorf("got %q", err.Error())
	}
}

// TestHigherOrderEmptyList: none of the higher-order functions crash on an empty
// list (sortBy previously panicked fatally in validateSortable). Regression guard.
func TestHigherOrderEmptyList(t *testing.T) {
	out, err := run(t, `
use io;
use lists;
func idn(n as int) { return $n; }
func yes(n as int) { return true; }
func add(a as int, b as int) { return $a + $b; }
def e as list of int init [];
io.printf("map=%d filter=%d reduce=%d any=%t all=%t sortBy0=%d sortBy1=%d\n",
    len(lists.map($e, idn)),
    len(lists.filter($e, yes)),
    lists.reduce($e, add, 0),
    lists.any($e, yes),
    lists.all($e, yes),
    len(lists.sortBy($e, idn)),
    len(lists.sortBy([7], idn)));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// any over empty is false; all over empty is true (vacuous).
	want := "map=0 filter=0 reduce=0 any=false all=true sortBy0=0 sortBy1=1\n"
	if out != want {
		t.Errorf("got  %q\nwant %q", out, want)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
