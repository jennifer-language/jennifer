// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package listslib

import (
	"bytes"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/parser"
)

type listFn = func(interpreter.BuiltinCtx, []interpreter.Value) (interpreter.Value, error)

func okCall(t *testing.T, fn listFn, args ...interpreter.Value) interpreter.Value {
	t.Helper()
	v, err := fn(interpreter.BuiltinCtx{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return v
}

func errCall(t *testing.T, label string, fn listFn, args ...interpreter.Value) {
	t.Helper()
	if _, err := fn(interpreter.BuiltinCtx{}, args); err == nil {
		t.Errorf("%s: expected an error, got nil", label)
	}
}

func TestFirstAndLast(t *testing.T) {
	xs := intList(10, 20, 30)
	if v := okCall(t, firstFn, xs); v.Int() != 10 {
		t.Errorf("first = %d, want 10", v.Int())
	}
	if v := okCall(t, lastFn, xs); v.Int() != 30 {
		t.Errorf("last = %d, want 30", v.Int())
	}
	errCall(t, "first(empty)", firstFn, intList())
	errCall(t, "last(empty)", lastFn, intList())
	errCall(t, "first(non-list)", firstFn, interpreter.IntVal(1))
	errCall(t, "first() arity", firstFn)
}

func TestReverseIsNonMutating(t *testing.T) {
	xs := intList(1, 2, 3)
	rev := okCall(t, reverseFn, xs)
	if len(rev.List()) != 3 || rev.List()[0].Int() != 3 || rev.List()[2].Int() != 1 {
		t.Errorf("reverse = %v, want [3 2 1]", rev.List())
	}
	if xs.List()[0].Int() != 1 {
		t.Errorf("reverse mutated the input")
	}
	errCall(t, "reverse(non-list)", reverseFn, interpreter.IntVal(1))
}

func TestConcat(t *testing.T) {
	a := intList(1, 2)
	b := intList(3, 4)
	c := okCall(t, concatFn, a, b)
	if len(c.List()) != 4 || c.List()[0].Int() != 1 || c.List()[3].Int() != 4 {
		t.Errorf("concat = %v, want [1 2 3 4]", c.List())
	}
	// Inputs unchanged.
	if len(a.List()) != 2 || len(b.List()) != 2 {
		t.Errorf("concat mutated an input")
	}
	errCall(t, "concat(list, non-list)", concatFn, a, interpreter.IntVal(1))
	errCall(t, "concat arity", concatFn, a)
}

// runLists parses and runs a Jennifer program with io + lists installed and
// returns its stdout. This exercises the higher-order builtins through their
// real call path (ctx.Invoke of a func value), which a direct Fn call cannot.
func runLists(t *testing.T, src string) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	Install(in)
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	return buf.String()
}

// TestHigherOrderFunctions drives map / filter / reduce / find / any / all /
// sortBy through a real program, asserting scalar results (so it does not depend
// on list-display formatting).
func TestHigherOrderFunctions(t *testing.T) {
	src := `
use io;
use lists;
func dbl(x as int) { return $x * 2; }
func isEven(x as int) { return $x % 2 == 0; }
func add(a as int, b as int) { return $a + $b; }
func neg(x as int) { return 0 - $x; }
def xs as list of int init [1, 2, 3, 4];

def m as list of int init lists.map($xs, dbl);
io.printf("map %d %d\n", $m[0], $m[3]);

def f as list of int init lists.filter($xs, isEven);
io.printf("filter %d %d %d\n", len($f), $f[0], $f[1]);

io.printf("reduce %d\n", lists.reduce($xs, add, 0));
io.printf("find %d\n", lists.find($xs, isEven));
io.printf("any %t all %t\n", lists.any($xs, isEven), lists.all($xs, isEven));

def s as list of int init lists.sortBy($xs, neg);
io.printf("sortBy %d %d\n", $s[0], $s[3]);
`
	got := runLists(t, src)
	want := "map 2 8\n" +
		"filter 2 2 4\n" +
		"reduce 10\n" +
		"find 2\n" +
		"any true all false\n" +
		"sortBy 4 1\n"
	if got != want {
		t.Errorf("higher-order results:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFindNoMatchThrows: lists.find with no match raises a catchable error.
func TestFindNoMatchThrows(t *testing.T) {
	src := `
use io;
use lists;
func big(x as int) { return $x > 100; }
def xs as list of int init [1, 2, 3];
try {
    def z as int init lists.find($xs, big);
    io.printf("no throw\n");
} catch (e) {
    io.printf("threw\n");
}
`
	if got := runLists(t, src); got != "threw\n" {
		t.Errorf("find with no match: got %q, want \"threw\\n\"", got)
	}
}

func TestInstallRegistersEveryListBuiltin(t *testing.T) {
	in := interpreter.New()
	Install(in)
	for _, name := range []string{
		"push", "first", "last", "reverse", "sort",
		"contains", "concat", "replicate", "shuffle", "range",
		"map", "filter", "reduce", "find", "any", "all", "sortBy", "sortWith",
	} {
		if in.LookupNamespacedBuiltin("lists", name) == nil {
			t.Errorf("lists.%s is not registered", name)
		}
	}
}
