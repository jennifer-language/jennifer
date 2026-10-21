// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package parser

import (
	"fmt"
	"testing"
)

// moveArgs lists the MoveArg of every `$v = call(...)` in a method body, in
// source order, through nested blocks.
func moveArgs(m *MethodDef) []int {
	var out []int
	walkStmtTree(m.Body.Stmts, func(s Stmt) {
		if a, ok := s.(*AssignStmt); ok {
			switch c := a.Value.(type) {
			case *CallExpr:
				out = append(out, c.MoveArg)
			case *QualifiedCallExpr:
				out = append(out, c.MoveArg)
			}
		}
	})
	return out
}

func TestMoveArgAnalysis(t *testing.T) {
	src := `
def g as list of int init [];
func f(a as list of int, b as int) { $a[0] = $b; return $a; }
func run(p as list of int) {
    def x as list of int init [1];
    $x = f($x, 1);
    $x = f($x, len($x));
    $p = f($p, 2);
    $g = f($g, 3);
    def y as list of int init [];
    $y = f($x, 4);
    try {
        $x = f($x, 5);
    } catch (e) {
        $x = f($x, 6);
    }
    for (def it in [[1]]) {
        $it = f($it, 7);
    }
    $x = m.put($x, 8);
    return $y;
}`
	prog, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := Resolve(prog); err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(moveArgs(methodNamed(prog, "run")))
	// moved; named twice; parameter; global; another target; in try; in catch;
	// for-each iterator; module call.
	if want := "[1 0 1 0 0 0 1 0 1]"; got != want {
		t.Errorf("MoveArg = %s, want %s", got, want)
	}
}

func TestReturnsOwnedAnalysis(t *testing.T) {
	src := `
def g as list of int init [];
def struct S { items as list of int };
func local() { def x as list of int init [1]; return $x; }
func literal(n as int) { if ($n > 0) { return [$n]; } return; }
func copiedParam(p as list of int) { $p[0] = 1; return $p; }
func borrowedParam(p as list of int) { return $p; }
func global() { return $g; }
func field(s as S) { return $s.items; }
func ownedField() { def s as S init S{items: []}; return $s.items; }
func iterator(xs as list of list of int) { for (def x in $xs) { return $x; } return []; }
func viaOwned() { return local(); }
func viaBuiltin() { return lists.reverse([1]); }
func recurse(n as int) { if ($n == 0) { return []; } return recurse($n - 1); }
func mixed(n as int) { if ($n == 0) { return $g; } return local(); }`
	prog, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := Resolve(prog); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"local": true, "literal": true, "copiedParam": true, "borrowedParam": false,
		"global": false, "field": false, "ownedField": true, "iterator": false,
		"viaOwned": true, "viaBuiltin": false, "recurse": true, "mixed": false,
	}
	for name, w := range want {
		if got := methodNamed(prog, name).ReturnsOwned; got != w {
			t.Errorf("%s: ReturnsOwned = %v, want %v", name, got, w)
		}
	}
	// Idempotent: resolving again recomputes the same flags.
	if err := Resolve(prog); err != nil {
		t.Fatal(err)
	}
	if !methodNamed(prog, "recurse").ReturnsOwned || methodNamed(prog, "mixed").ReturnsOwned {
		t.Error("a second Resolve changed the flags")
	}
}
