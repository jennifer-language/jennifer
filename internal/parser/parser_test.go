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
    io.printf($x + $x);
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
	if got := Sprint(body.Stmts[1]); got != "ExprStmt(QCall(io.printf, (Var($x) + Var($x))))" {
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
	src := `func app() { io.printf("hi"); }`
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	got := Sprint(prog.Methods[0].Body.Stmts[0])
	if got != `ExprStmt(QCall(io.printf, Str("hi")))` {
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
	src := `func app() { io.printf(1); }`
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
		// Since M13.1 (structs), the parser accepts any IDENT as a type
		// name; unknown struct types are surfaced at runtime by the
		// interpreter ("unknown struct type"), not by the parser.
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

// TestParseM6Constructs covers the new list/map syntax forms added in M6:
// type declarations, literals, indexing reads + writes, and for-each.
// We use Sprint() to assert AST shape rather than poking at internal
// fields - keeps the test stable across small AST refactors.
func TestParseM6Constructs(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			"list type with literal init",
			`def xs as list of int init [1, 2, 3];`,
			`Define($xs as list of int = List[Int(1), Int(2), Int(3)])`,
		},
		{
			"empty list literal",
			`def xs as list of int init [];`,
			`Define($xs as list of int = List[])`,
		},
		{
			"map type with literal init",
			`def m as map of string to int init {"a": 1, "b": 2};`,
			`Define($m as map of string to int = Map{Str("a"): Int(1), Str("b"): Int(2)})`,
		},
		{
			"nested list type",
			`def xs as list of list of int init [[1, 2], [3, 4]];`,
			`Define($xs as list of list of int = List[List[Int(1), Int(2)], List[Int(3), Int(4)]])`,
		},
		{
			"index read",
			`def y as int init $xs[0];`,
			`Define($y as int = Index(Var($xs), Int(0)))`,
		},
		{
			"index write",
			`$xs[0] = 99;`,
			`IndexAssign(Index(Var($xs), Int(0)) = Int(99))`,
		},
		{
			"chained index write",
			`$g[0][1] = 99;`,
			`IndexAssign(Index(Index(Var($g), Int(0)), Int(1)) = Int(99))`,
		},
		{
			"for-each over list",
			`for (def x in $xs) { return; }`,
			`ForEach($x in Var($xs), Block[Return])`,
		},
		{
			"for-each over map",
			`for (def k in $m) { return; }`,
			`ForEach($k in Var($m), Block[Return])`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, err := Parse(c.src)
			if err != nil {
				t.Fatalf("parse %q: %v", c.src, err)
			}
			// One top-level stmt expected.
			if len(prog.TopLevel) != 1 {
				t.Fatalf("expected 1 top-level stmt, got %d", len(prog.TopLevel))
			}
			got := Sprint(prog.TopLevel[0])
			if got != c.want {
				t.Errorf("got %q\nwant %q", got, c.want)
			}
		})
	}
}

// TestParseM6Rejections covers the parser-level error paths for the
// new syntax: malformed literals, missing keywords, bad identifiers.
func TestParseM6Rejections(t *testing.T) {
	bad := []struct {
		name, src, want string
	}{
		{"list with missing of", `def xs as list int init [];`, "after `list`"},
		{"map with missing of", `def m as map string to int init {};`, "after `map`"},
		{"map with missing to", `def m as map of string int init {};`, "after map key type"},
		{"map literal missing colon", `def m as map of string to int init {"a" 1};`, "between map key and value"},
		{"unclosed list literal", `def xs as list of int init [1, 2;`, "to close list literal"},
		{"unclosed index", `def y as int init $xs[0;`, "to close index expression"},
		{"for-each underscore in iter var", `for (def my_var in $xs) {}`, "may not contain"},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.src)
			if err == nil {
				t.Errorf("expected error, got nil")
				return
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

func TestParseQualifiedCall(t *testing.T) {
	// `bio.translate($seq)` is a qualified call: prefix.callee(args).
	src := `use bio; bio.translate($seq);`
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(prog.TopLevel) != 1 {
		t.Fatalf("expected one stmt, got %d", len(prog.TopLevel))
	}
	got := Sprint(prog.TopLevel[0])
	want := "ExprStmt(QCall(bio.translate, Var($seq)))"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestParseQualifiedCallZeroArg(t *testing.T) {
	src := `use os; os.platform();`
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := Sprint(prog.TopLevel[0])
	want := "ExprStmt(QCall(os.platform))"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestParseQualifiedConstRef(t *testing.T) {
	src := `use bio; def x as int init bio.STOPS;`
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := Sprint(prog.TopLevel[0])
	want := "Define($x as int = QConst(bio.STOPS))"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestParseUseWithAlias(t *testing.T) {
	src := `use bio as b; b.translate($x);`
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("imports: got %+v", prog.Imports)
	}
	imp := prog.Imports[0]
	if imp.Name != "bio" || imp.AsName != "b" {
		t.Errorf("import: got Name=%q AsName=%q, want bio/b", imp.Name, imp.AsName)
	}
	if Sprint(imp) != "Import(bio as b)" {
		t.Errorf("sprint: got %q", Sprint(imp))
	}
}

func TestParseQualifiedErrors(t *testing.T) {
	bad := []struct {
		name string
		src  string
		want string
	}{
		{"const-form wants UPPERCASE", `use bio; def x as int init bio.lower;`, "uppercase"},
		{"method name with underscore", `use bio; bio.my_call();`, "may not contain"},
		{"alias with underscore", `use bio as b_alias;`, "may not contain"},
		{"missing IDENT after dot", `use bio; bio.();`, "after `.`"},
		{"missing IDENT after as", `use bio as ;`, "after `as`"},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.src)
			if err == nil {
				t.Errorf("expected error, got nil")
				return
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

func TestParseAppendForm(t *testing.T) {
	src := `$xs[] = 42;`
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(prog.TopLevel) != 1 {
		t.Fatalf("expected one stmt, got %d", len(prog.TopLevel))
	}
	got := Sprint(prog.TopLevel[0])
	want := "Append(Var($xs) = Int(42))"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestParseAppendFormRejectsRead(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"bare read", `io.printf($xs[]);`, "append form"},
		{"read in expression", `def y as int init $xs[] + 1;`, "append form"},
		{"$xs[] without =", `$xs[];`, "write-only"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.src)
			if err == nil {
				t.Errorf("expected error, got nil")
				return
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}
