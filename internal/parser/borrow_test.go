// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package parser

import "testing"

// Read-only-parameter borrow analysis. markBorrowableParams (run from Resolve)
// flags a parameter borrowable when the body never writes it and its type is
// borrow-safe. These tests pin the write-scan (any lvalue root anywhere in the
// body, through nested control flow, disqualifies) and the type gate, because a
// false positive here would let the interpreter alias a parameter the callee
// mutates - a value-semantics violation.

func methodNamed(p *Program, name string) *MethodDef {
	for _, m := range p.Methods {
		if m.Name == name {
			return m
		}
	}
	return nil
}

func TestBorrowableParamAnalysis(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []bool
	}{
		{"flat list, never written", `func f(xs as list of int) { def n as int init len($xs); }`, []bool{true}},
		{"list reassigned", `func f(xs as list of int) { $xs = [1]; }`, []bool{false}},
		{"list index-written", `func f(xs as list of int) { $xs[0] = 1; }`, []bool{false}},
		{"list appended", `func f(xs as list of int) { $xs[] = 9; }`, []bool{false}},
		{"written inside if/while", `func f(xs as list of int) { if (true) { while (false) { $xs[0] = 1; } } }`, []bool{false}},
		{"written in for-each body", `func f(xs as list of int) { for (def i in 0..3) { $xs[0] = 1; } }`, []bool{false}},
		{"reassigned in for step", `func f(xs as list of int) { for (def i as int init 0; false; $xs = [1]) { def z as int init 0; } }`, []bool{false}},
		{"written in match arm", `func f(xs as list of int, k as int) { match ($k) { when 1 { $xs[0] = 1; } else { def z as int init 0; } } }`, []bool{false, false}},
		{"written in try/catch", `func f(xs as list of int) { try { $xs[0] = 1; } catch (e) { def z as int init 0; } }`, []bool{false}},
		{"flat map, never written", `func f(m as map of string to int) { def n as int init len($m); }`, []bool{true}},
		{"map key-written", `func f(m as map of string to int) { $m["a"] = 1; }`, []bool{false}},
		{"struct, never written", `def struct P { a as int }; func f(p as P) { def n as int init $p.a; }`, []bool{true}},
		{"struct field written", `def struct P { a as int }; func f(p as P) { $p.a = 1; }`, []bool{false}},
		{"struct chained lvalue written", `def struct P { xs as list of int }; func f(p as P) { $p.xs[0] = 1; }`, []bool{false}},
		{"bytes, never written", `func f(b as bytes) { def n as int init len($b); }`, []bool{true}},
		{"nested list is borrow-unsafe type", `func f(xs as list of list of int) { def n as int init len($xs); }`, []bool{false}},
		{"map of list is borrow-unsafe type", `func f(m as map of string to list of int) { def n as int init len($m); }`, []bool{false}},
		{"list of struct is borrow-safe", `def struct P { a as int }; func f(xs as list of P) { def n as int init len($xs); }`, []bool{true}},
		{"scalar not borrowed", `func f(x as int) { def n as int init $x; }`, []bool{false}},
		{"string not borrowed", `func f(s as string) { def n as int init len($s); }`, []bool{false}},
		{"two params, one written", `func f(xs as list of int, ys as list of int) { $ys[0] = 1; def n as int init len($xs); }`, []bool{true, false}},
		{"read-only helper reading both", `func f(xs as list of string, i as int) { def a as string init $xs[$i]; }`, []bool{true, false}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog := mustResolve(t, c.src)
			m := methodNamed(prog, "f")
			if m == nil {
				t.Fatal("method f not found")
			}
			if len(m.Params) != len(c.want) {
				t.Fatalf("param count %d, want %d", len(m.Params), len(c.want))
			}
			for i, want := range c.want {
				if m.Params[i].Borrow != want {
					t.Errorf("param %q (%s): Borrow = %v, want %v", m.Params[i].Name, m.Params[i].Type, m.Params[i].Borrow, want)
				}
			}
		})
	}
}

// Re-resolving is idempotent: the borrow flags come out the same, never
// accumulating (the flag is assigned, not OR-ed).
func TestBorrowAnalysisIdempotent(t *testing.T) {
	prog := mustResolve(t, `func f(xs as list of int, ys as list of int) { $ys[0] = 1; def n as int init len($xs); }`)
	if err := Resolve(prog); err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	m := methodNamed(prog, "f")
	if !m.Params[0].Borrow || m.Params[1].Borrow {
		t.Errorf("after re-resolve: got [%v %v], want [true false]", m.Params[0].Borrow, m.Params[1].Borrow)
	}
}
