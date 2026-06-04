// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package parser

import (
	"strings"
	"testing"
)

func TestParseHelloProgram(t *testing.T) {
	src := `import stdlib;
def app() {
    define $x as int init 21;
    printf($x + $x);
}`
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(prog.Imports) != 1 || prog.Imports[0].Name != "stdlib" {
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
	src := `def app() { define $r as int init 1 + 2 * 3; }`
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
	src := `def app() { define $r as int init (1 + 2) * 3; }`
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
	src := `def app() { printf("hi"); }`
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	got := Sprint(prog.Methods[0].Body.Stmts[0])
	if got != `ExprStmt(Call(printf, Str("hi")))` {
		t.Errorf("got %s", got)
	}
}

func TestDefAndDefineAreInterchangeable(t *testing.T) {
	// `def` for a variable (was historically only for methods)
	src1 := `def app() { def $x as int init 5; }`
	p1, err := Parse(src1)
	if err != nil {
		t.Fatalf("def-as-variable parse error: %v", err)
	}
	if got := Sprint(p1.Methods[0].Body.Stmts[0]); got != "Define($x as int = Int(5))" {
		t.Errorf("def-as-variable: got %s", got)
	}
	// `define` for a method (was historically only for variables)
	src2 := `define app() { printf(1); }`
	p2, err := Parse(src2)
	if err != nil {
		t.Fatalf("define-as-method parse error: %v", err)
	}
	if p2.Methods[0].Name != "app" {
		t.Errorf("define-as-method: got %s", p2.Methods[0].Name)
	}
}

func TestMethodInsideBlockRejected(t *testing.T) {
	_, err := Parse(`def app() { def inner() {} }`)
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
		{"missing semi", `import stdlib def app() {}`, "expected SEMI"},
		{"top level expression", `42;`, "expected `import`, `def`, or `define`"},
		{"define needs init in M1", `def app() { define $x as int; }`, "expected INIT"},
		{"unknown type", `def app() { define $x as bool init 1; }`, "expected type"},
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
