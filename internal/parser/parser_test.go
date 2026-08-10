// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package parser

import (
	"math"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/limits"
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

// `!=` parses at the comparison rung, looser than the bit ops (Python-style),
// so `2 & 3 != 2` groups as `(2 & 3) != 2`, mirroring `==`.
func TestParseNotEqualPrecedence(t *testing.T) {
	src := `func app() { def r as bool init 2 & 3 != 2; }`
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	got := Sprint(prog.Methods[0].Body.Stmts[0])
	want := "Define($r as bool = ((Int(2) & Int(3)) != Int(2)))"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// `defer` parses to a DeferStmt wrapping the call; a namespaced call keeps its
// QualifiedCallExpr shape so the interpreter can dispatch it normally.
func TestParseDeferStmt(t *testing.T) {
	prog, err := Parse(`func app() { defer fs.close($f); }`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	got := Sprint(prog.Methods[0].Body.Stmts[0])
	want := "Defer(QCall(fs.close, Var($f)))"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// `defer` on a non-call is a parse error pointing at the required form.
func TestParseDeferRejectsNonCall(t *testing.T) {
	_, err := Parse(`func app() { defer 1 + 2; }`)
	if err == nil || !contains(err.Error(), "requires a function call") {
		t.Errorf("expected defer-needs-a-call error, got %v", err)
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

// A deeply-nested expression must be rejected with a positioned parse error
// rather than being parsed to a depth that later overflows the (recover-less)
// Go stack at resolve/eval time. This is the untrusted-input guard for the
// `jennifer run -` stdin path and downloaded scripts. The cap is build-tag
// split (see internal/limits); this exercises the standard-Go value.
func TestDeeplyNestedExpressionRejected(t *testing.T) {
	// One level past the cap, in each nested-container form.
	n := limits.MaxNestingDepth + 1
	cases := map[string]string{
		"list":  "use io; def x as int init " + strings.Repeat("[", n) + "1" + strings.Repeat("]", n) + ";",
		"paren": "use io; def x as int init " + strings.Repeat("(", n) + "1" + strings.Repeat(")", n) + ";",
	}
	for name, src := range cases {
		_, err := Parse(src)
		if err == nil || !strings.Contains(err.Error(), "nesting exceeds") {
			t.Errorf("%s: expected nesting-limit parse error, got %v", name, err)
		}
	}
	// Nesting comfortably under the cap still parses (the guard must not
	// over-reject ordinary, if unusual, expressions).
	under := limits.MaxNestingDepth - 4
	ok := "use io; def x as int init " + strings.Repeat("(", under) + "1" +
		strings.Repeat(")", under) + ";"
	if _, err := Parse(ok); err != nil {
		t.Errorf("depth-%d expression should parse, got %v", under, err)
	}
}

// Statement-block nesting (if/while/... bodies via parseBlock) and compound-type
// nesting (list of / map of via parseType) share the expression guard's rationale:
// unbounded, they overflow the recover-less Go stack (fatal on the fixed TinyGo
// stack). Both must reject past the cap with a positioned parse error.
func TestDeeplyNestedStatementsAndTypesRejected(t *testing.T) {
	n := limits.MaxNestingDepth + 1
	stmt := "func f() { " + strings.Repeat("if (true) { ", n) + "def x as int init 1; " +
		strings.Repeat("} ", n) + "}"
	if _, err := Parse(stmt); err == nil || !strings.Contains(err.Error(), "block nesting exceeds") {
		t.Errorf("expected block-nesting parse error, got %v", err)
	}
	typ := "def x as " + strings.Repeat("list of ", n) + "int;"
	if _, err := Parse(typ); err == nil || !strings.Contains(err.Error(), "compound type nesting exceeds") {
		t.Errorf("expected type-nesting parse error, got %v", err)
	}
	// Modest, ordinary nesting under the cap still parses in both forms.
	okStmt := "func f() { if (true) { while (true) { def x as int init 1; break; } } }"
	if _, err := Parse(okStmt); err != nil {
		t.Errorf("ordinary nested statements should parse, got %v", err)
	}
	okType := "def x as list of map of string to list of int;"
	if _, err := Parse(okType); err != nil {
		t.Errorf("ordinary nested type should parse, got %v", err)
	}
}

// A parse error must not echo an arbitrarily long lexeme (or the underlying
// strconv error, which re-embeds the numeral) - a 1 MiB bad literal would
// otherwise become a 1 MiB diagnostic. describeLexeme bounds the lexeme and
// numErrReason drops the numeral from the strconv reason.
func TestParseErrorOutputBounded(t *testing.T) {
	if d := describeLexeme(strings.Repeat("9", 5000)); len(d) > 200 || !strings.HasSuffix(d, "...") {
		t.Errorf("describeLexeme not bounded: len=%d", len(d))
	}
	if describeLexeme("short") != "short" {
		t.Error("a short lexeme must pass through unchanged")
	}
	// An overflowing int literal: the message stays small and must not re-echo
	// the whole numeral (via either the %q lexeme or the strconv error).
	big := strings.Repeat("9", 5000)
	_, err := Parse("def x as int init " + big + ";")
	if err == nil {
		t.Fatal("expected an overflow parse error")
	}
	if n := len(err.Error()); n > 600 {
		t.Errorf("error message not bounded: %d bytes", n)
	}
	if c := strings.Count(err.Error(), "9"); c > 400 {
		t.Errorf("error re-echoes the full numeral (%d nines)", c)
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
		// The parser accepts any IDENT as a type
		// name; unknown struct types are surfaced at runtime by the
		// interpreter ("unknown struct type"), not by the parser.
		{"const needs uppercase", `func app() { def const lower as int init 1; }`, "must be uppercase"},
		{"const rejects trailing underscore", `func app() { def const MAX_ as int init 1; }`, "may not end with"},
		{"const rejects double-underscore-then-trailing", `func app() { def const MAX__ as int init 1; }`, "may not end with"},
		{"const rejects lowercase with underscore", `func app() { def const max_int as int init 1; }`, "must be uppercase"},
		{"const rejects consecutive underscores", `func app() { def const MAX__INT as int init 1; }`, "consecutive"},
		{"const rejects four-in-a-row underscores", `func app() { def const MAX____RETRIES as int init 1; }`, "consecutive"},
		{"const rejects underscore-then-digit chunk", `func app() { def const AES_256 as int init 1; }`, "chunk"},
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
		// In-chunk digits: a chunk starts with a letter, then upper/digits.
		`def const SHA256 as int init 1;`,
		`def const HTTP2 as int init 2;`,
		`def const RFC5322 as int init 1;`,
		`def const SCRAM_SHA256 as int init 1;`,
		`def const X509 as int init 1;`,
	}
	for _, src := range good {
		if _, err := Parse(src); err != nil {
			t.Errorf("%q: unexpected parse error: %v", src, err)
		}
	}
}

// TestParseDigitIdentifiers covers the identifier digit relaxation for the
// letters-only kinds (variables, methods, parameters): interior / trailing
// digits are allowed, `_` is still rejected.
func TestParseDigitIdentifiers(t *testing.T) {
	good := []string{
		`def x2 as int init 5;`,
		`def sha256 as int init 1;`,
		`func md5(v2 as int) { return $v2; }`,
		`func toUtf8(s as string) { return $s; }`,
	}
	for _, src := range good {
		if _, err := Parse(src); err != nil {
			t.Errorf("%q: unexpected parse error: %v", src, err)
		}
	}
	// `_` stays constants-only, even alongside digits.
	if _, err := Parse(`def x_2 as int init 1;`); err == nil {
		t.Error("expected error: `_` is not allowed in a variable name")
	}
}

// TestParseM6Constructs covers the list/map syntax forms:
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
	src := `use os; os.getEnv("HOME");`
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := Sprint(prog.TopLevel[0])
	want := `ExprStmt(QCall(os.getEnv, Str("HOME")))`
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

// TestParseConstFieldAccess: `CONST.field` on a const struct parses as field
// access, not a qualified constant reference (which would reject the lowercase
// field name). Previously only the `(CONST).field` workaround parsed.
func TestParseConstFieldAccess(t *testing.T) {
	prog, err := Parse(`def x as int init ORIGIN.x;`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := Sprint(prog.TopLevel[0])
	want := "Define($x as int = Field(Const(ORIGIN).x))"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	// A chained access keeps threading through the postfix loop.
	prog2, err := Parse(`def y as int init DIAG.to.x;`)
	if err != nil {
		t.Fatalf("parse chained: %v", err)
	}
	got2 := Sprint(prog2.TopLevel[0])
	want2 := "Define($y as int = Field(Field(Const(DIAG).to).x))"
	if got2 != want2 {
		t.Errorf("chained: got %s, want %s", got2, want2)
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

// The most-negative int literal (magnitude 2^63, one past MaxInt64) is valid
// only when negated: -9223372036854775808 is math.MinInt64. The bare magnitude
// stays a range error, and the hex form negates too.
func TestMostNegativeIntLiteral(t *testing.T) {
	for _, src := range []string{
		"def a as int init -9223372036854775808;",
		"def a as int init -0x8000000000000000;",
	} {
		prog, err := Parse(src)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		def := prog.TopLevel[0].(*DefineStmt)
		lit, ok := def.InitExpr.(*IntLit)
		if !ok {
			t.Fatalf("%q: init is %T, want *IntLit", src, def.InitExpr)
		}
		if lit.Value != math.MinInt64 {
			t.Errorf("%q: Value = %d, want MinInt64", src, lit.Value)
		}
	}
	// The bare (un-negated) magnitude is still out of range.
	if _, err := Parse("def a as int init 9223372036854775808;"); err == nil {
		t.Error("bare 9223372036854775808 should be a range error")
	}
	// A magnitude past 2^63 is out of range even negated.
	if _, err := Parse("def a as int init -9223372036854775809;"); err == nil {
		t.Error("-9223372036854775809 should be a range error")
	}
}

// A statement that starts with an lvalue chain but continues with a binary
// operator flows through tryParseIndexAssign's seeded re-parse. The pending
// seed is the leading operand, so a following `-` (or `~`/`not`) must bind
// as a BINARY operator, never as a prefix on whatever comes next.
func TestSeededExprStmtBinaryMinus(t *testing.T) {
	cases := []struct{ src, want string }{
		{`$x[0] - 1;`, "ExprStmt((Index(Var($x), Int(0)) - Int(1)))"},
		{`$x[0] - [1][0];`, "ExprStmt((Index(Var($x), Int(0)) - Index(List[Int(1)], Int(0))))"},
		{`$x[0] - -1;`, "ExprStmt((Index(Var($x), Int(0)) - (- Int(1))))"},
	}
	for _, c := range cases {
		prog, err := Parse(c.src)
		if err != nil {
			t.Errorf("parse %q: %v", c.src, err)
			continue
		}
		if len(prog.TopLevel) != 1 {
			t.Errorf("%q: expected one stmt, got %d", c.src, len(prog.TopLevel))
			continue
		}
		if got := Sprint(prog.TopLevel[0]); got != c.want {
			t.Errorf("%q:\n got %s\nwant %s", c.src, got, c.want)
		}
	}
}

// `errdefer` parses to a DeferStmt with OnError set; Sprint shows the variant.
func TestParseErrdeferStmt(t *testing.T) {
	prog, err := Parse(`func app() { errdefer net.close($c); }`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	st, ok := prog.Methods[0].Body.Stmts[0].(*DeferStmt)
	if !ok || !st.OnError {
		t.Fatalf("expected a DeferStmt with OnError, got %T", prog.Methods[0].Body.Stmts[0])
	}
	got := Sprint(st)
	want := "Errdefer(QCall(net.close, Var($c)))"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// `errdefer` on a non-call is a parse error naming the errdefer form.
func TestParseErrdeferRejectsNonCall(t *testing.T) {
	_, err := Parse(`func app() { errdefer $x; }`)
	if err == nil || !contains(err.Error(), "`errdefer` requires a function call") {
		t.Errorf("expected errdefer-needs-a-call error, got %v", err)
	}
}

// TestParseMatchErrors covers the structural rules the parser enforces on a
// `match`: `else` must be last (no `when` after it) and appear at most once.
func TestParseMatchErrors(t *testing.T) {
	cases := []struct{ name, src, wantSubstr string }{
		{"when after else", `match ($x) { else { } when 1 { } }`, "else"},
		{"two else", `match ($x) { when 1 { } else { } else { } }`, "one `else`"},
		{"missing body", `match ($x) { when 1 }`, "block"},
	}
	for _, c := range cases {
		_, err := Parse(c.src)
		if err == nil {
			t.Errorf("%s: expected a parse error, got none", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.wantSubstr) {
			t.Errorf("%s: error %q does not contain %q", c.name, err.Error(), c.wantSubstr)
		}
	}
}

// TestParseMatchValueStructLiteral checks the `{` disambiguation: a bare
// `when Name { ... }` reads `Name` as a value and `{` as the arm block, while a
// parenthesized `when (Name{...})` is a struct-literal value.
func TestParseMatchValueStructLiteral(t *testing.T) {
	// `when MAX {` - MAX is a constant value; `{` opens the block (no struct lit).
	if _, err := Parse(`def const MAX as int init 9; match ($x) { when MAX { } }`); err != nil {
		t.Errorf("bare constant when-value should parse: %v", err)
	}
	// Parenthesized struct-literal value is allowed.
	if _, err := Parse(`def struct P { x as int }; match ($p) { when (P{x: 1}) { } }`); err != nil {
		t.Errorf("parenthesized struct-literal when-value should parse: %v", err)
	}
}

// TestParseEnumDef checks `def enum` declaration parsing: payloaded and
// payload-less variants, and the Sprint form.
func TestParseEnumDef(t *testing.T) {
	prog, err := Parse(`def enum Shape { Circle { r as float }, Rect { w as float, h as float }, Empty };`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(prog.Enums) != 1 {
		t.Fatalf("want 1 enum, got %d", len(prog.Enums))
	}
	e := prog.Enums[0]
	if e.Name != "Shape" || len(e.Variants) != 3 {
		t.Fatalf("got name=%q variants=%d", e.Name, len(e.Variants))
	}
	if e.Variants[0].Name != "Circle" || len(e.Variants[0].Fields) != 1 {
		t.Errorf("variant 0: %+v", e.Variants[0])
	}
	if e.Variants[2].Name != "Empty" || len(e.Variants[2].Fields) != 0 {
		t.Errorf("variant 2 (payload-less): %+v", e.Variants[2])
	}
	got := Sprint(e)
	want := "Enum(Shape{Circle{r as float}, Rect{w as float, h as float}, Empty})"
	if got != want {
		t.Errorf("Sprint: got %q want %q", got, want)
	}
}

// TestParseEnumErrors checks positioned rejections in enum declarations.
func TestParseEnumErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"empty enum", `def enum E { };`, "at least one variant"},
		{"dup variant", `def enum E { A, A };`, "declared twice"},
		{"underscore name", `def enum My_Enum { A };`, "may not contain"},
	}
	for _, c := range cases {
		if _, err := Parse(c.src); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}

// TestResolveRejectsDuplicateDeclarations pins that a duplicate top-level
// struct / enum / method is a parse-time (resolve) error, not a runtime one -
// so a diamond `include` of a shared header of types and functions fails before
// any output prints. The message keeps the "defined more than once" phrasing the
// interpreter's runtime hoist check uses, and names the first definition.
func TestResolveRejectsDuplicateDeclarations(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"method", "func foo() { return 1; }\nfunc foo() { return 2; }", `method "foo" is defined more than once`},
		{"struct", "def struct P { x as int };\ndef struct P { y as int };", `struct "P" is defined more than once`},
		{"enum", "def enum E { A, B };\ndef enum E { C, D };", `enum "E" is defined more than once`},
		{"enum-vs-struct", "def struct T { x as int };\ndef enum T { A, B };", `enum "T" collides with a struct of the same name`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := Parse(tc.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if err := Resolve(prog); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want resolve error containing %q, got %v", tc.want, err)
			}
		})
	}
}
