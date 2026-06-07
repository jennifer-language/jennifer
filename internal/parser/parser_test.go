// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package parser

import (
	"strings"
	"testing"
)

func TestParseHelloProgram(t *testing.T) {
	src := `use io;
func app() {
    def x as int init 21;
    printf($x + $x);
}`
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(prog.Imports) != 1 || prog.Imports[0].Name != "io" {
		t.Errorf("imports: got %+v, want [stdlib]", prog.Imports)
	}
	if len(prog.Methods) != 1 || prog.Methods[0].Name != "app" {
		t.Fatalf("methods: got %+v, want [app]", prog.Methods)
	}
	body := prog.Methods[0].Body
	if len(body.Stmts) != 2 {
		t.Fatalf("body: got %d stmts, want 2", len(body.Stmts))
	}
	if got := Sprint(body.Stmts[0]); got != "Define($x as int = Int(21))" {
		t.Errorf("define: got %s", got)
	}
	if got := Sprint(body.Stmts[1]); got != "ExprStmt(Call(printf, (Var($x) + Var($x))))" {
		t.Errorf("call: got %s", got)
	}
}

func TestParseOperatorPrecedence(t *testing.T) {
	src := `func app() { def r as int init 1 + 2 * 3; }`
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	got := Sprint(prog.Methods[0].Body.Stmts[0])
	want := "Define($r as int = (Int(1) + (Int(2) * Int(3))))"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestParseParenGrouping(t *testing.T) {
	src := `func app() { def r as int init (1 + 2) * 3; }`
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	got := Sprint(prog.Methods[0].Body.Stmts[0])
	want := "Define($r as int = ((Int(1) + Int(2)) * Int(3)))"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestParseStringLiteralCall(t *testing.T) {
	src := `func app() { printf("hi"); }`
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	got := Sprint(prog.Methods[0].Body.Stmts[0])
	if got != `ExprStmt(Call(printf, Str("hi")))` {
		t.Errorf("got %s", got)
	}
}

func TestDefRejectsDollarAtDefinitionSite(t *testing.T) {
	// The `$` sigil is reserved for use-site references. At a def site we want
	// a helpful error pointing the user at the bare name.
	_, err := Parse(`func app() { def $x as int init 5; }`)
	if err == nil || !strings.Contains(err.Error(), "drop the `$`") {
		t.Errorf("expected $-at-def-site hint, got %v", err)
	}
}

func TestFuncIntroducesMethod(t *testing.T) {
	src := `func app() { printf(1); }`
	p, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(p.Methods) != 1 || p.Methods[0].Name != "app" {
		t.Errorf("expected one method named app, got %+v", p.Methods)
	}
}

func TestMethodInsideBlockRejected(t *testing.T) {
	_, err := Parse(`func app() { func inner() {} }`)
	if err == nil || !contains(err.Error(), "top level") {
		t.Errorf("expected nested-method error, got %v", err)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func TestParseErrors(t *testing.T) {
	bad := []struct {
		name string
		src  string
		want string // substring of error
	}{
		{"missing semi", `use stdlib func app() {}`, "expected SEMI"},
		// `42;` and `def x ...;` are now both valid at top level - no
		// equivalent rejection test belongs here.
		{"truly unknown type", `func app() { def x as widget init 1; }`, "expected type"},
		{"const needs uppercase", `func app() { def const lower as int init 1; }`, "must be uppercase"},
		{"const rejects trailing underscore", `func app() { def const MAX_ as int init 1; }`, "may not end with"},
		{"const rejects double-underscore-then-trailing", `func app() { def const MAX__ as int init 1; }`, "may not end with"},
		{"const rejects lowercase with underscore", `func app() { def const max_int as int init 1; }`, "must be uppercase"},
		{"const rejects consecutive underscores", `func app() { def const MAX__INT as int init 1; }`, "consecutive"},
		{"const rejects four-in-a-row underscores", `func app() { def const MAX____RETRIES as int init 1; }`, "consecutive"},
		{"var rejects underscore", `func app() { def my_var as int init 1; }`, "may not contain"},
		{"method name rejects underscore", `func my_method() {}`, "may not contain"},
		{"param rejects underscore", `func f(my_arg as int) {}`, "may not contain"},
		{"library name rejects underscore", `use my_lib;`, "may not contain"},
		{"call site rejects underscore", `foo_bar();`, "may not contain"},
		{"const needs init", `func app() { def const X as int; }`, "constants require"},
	}
	for _, c := range bad {
		_, err := Parse(c.src)
		if err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q does not contain %q", c.name, err.Error(), c.want)
		}
	}
}

// TestConstNameAccepts exercises the constant naming rule's accepting side:
// uppercase chunks separated by single `_` characters. The rule is
// `[A-Z]+(_[A-Z]+)*`, so consecutive underscores like `MAX__INT` are
// rejected (covered by TestParseErrors above).
func TestConstNameAccepts(t *testing.T) {
	good := []string{
		`def const A as int init 1;`,
		`def const MAX as int init 1;`,
		`def const MAX_RETRIES as int init 1;`,
		`def const HTTP_OK as int init 200;`,
		`def const A_B_C_D as int init 1;`,
	}
	for _, src := range good {
		if _, err := Parse(src); err != nil {
			t.Errorf("%q: unexpected parse error: %v", src, err)
		}
	}
}
