// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/lexer"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// TestAstJSONIsValid covers the AST JSON emitter end to end: parse a
// representative program, walk it through emitNode, and assert the
// output is parseable JSON. encoding/json is used in the *test* only -
// the emitter itself stays hand-rolled for TinyGo.
func TestAstJSONIsValid(t *testing.T) {
	cases := []string{
		`use io; io.printf("hi\n");`,
		`def x as int init 21; io.printf($x + $x);`,
		`func fact(n as int) { if ($n == 0) { return 1; } return $n * fact($n - 1); }`,
		`def const MAX_RETRIES as int init 3; io.printf(MAX_RETRIES);`,
		`for (def i as int init 0; $i < 3; $i = $i + 1) { io.printf($i); }`,
		`def xs as list of int init [1, 2, 3]; def y as int init $xs[0];`,
		`def m as map of string to int init {"a": 1, "b": 2};`,
		`def g as list of list of int init [[1, 2], [3, 4]]; $g[0][1] = 99;`,
		`for (def x in $xs) { return; }`,
	}
	for _, src := range cases {
		prog, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		var b strings.Builder
		emitNode(&b, prog, 0)
		var v any
		if err := json.Unmarshal([]byte(b.String()), &v); err != nil {
			t.Errorf("invalid JSON for %q: %v\noutput:\n%s", src, err, b.String())
		}
	}
}

// TestAstJSONShape checks a few key fields on a known-shape program so a
// future refactor that drops or renames a field is caught here.
func TestAstJSONShape(t *testing.T) {
	prog, err := parser.Parse(`use io; def x as int init 42; io.printf($x);`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var b strings.Builder
	emitNode(&b, prog, 0)
	out := b.String()

	mustContain := []string{
		`"type": "Program"`,
		`"type": "ImportStmt"`, `"name": "io"`,
		`"type": "DefineStmt"`, `"varName": "x"`, `"varType": "int"`,
		`"type": "IntLit"`, `"value": 42`,
		`"type": "ExprStmt"`,
		`"type": "QualifiedCallExpr"`, `"prefix": "io"`, `"callee": "printf"`,
		`"type": "VarExpr"`, `"name": "x"`,
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("AST JSON missing %q\nfull output:\n%s", want, out)
		}
	}
}

// TestAstJSONM6Shape covers the new AST nodes: list/map
// literals, index expressions, for-each. We assert a handful of field
// signatures per node to catch a future refactor that drops a field or
// renames it.
func TestAstJSONM6Shape(t *testing.T) {
	src := `
def xs as list of int init [10, 20];
def m as map of string to int init {"a": 1};
def y as int init $xs[0];
$xs[1] = 99;
for (def x in $xs) { return; }
`
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var b strings.Builder
	emitNode(&b, prog, 0)
	out := b.String()

	mustContain := []string{
		`"type": "ListLit"`, `"elements": [`,
		`"type": "MapLit"`, `"keys": [`, `"values": [`,
		`"type": "IndexExpr"`, `"target":`, `"index":`,
		`"type": "IndexAssignStmt"`,
		`"type": "ForEachStmt"`, `"varName": "x"`, `"coll":`,
		`"varType": "list of int"`,
		`"varType": "map of string to int"`,
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("AST JSON missing %q\nfull output:\n%s", want, out)
		}
	}
}

// TestTokenDumpCoversAllKinds is a thin sanity check on the lexer's
// token-type names: every kind we expect to see in real programs has a
// String() that doesn't fall through to the "TokenType(N)" fallback.
func TestTokenDumpCoversAllKinds(t *testing.T) {
	src := `use io; def const MAX as int init 1; func f() { if (true) { return; } } $x = -1.5; io.printf("hi");`
	tokens, err := lexer.TokenizeWithFile(src, "<test>")
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	for _, tok := range tokens {
		name := tok.Type.String()
		if strings.HasPrefix(name, "TokenType(") {
			t.Errorf("token %v has no friendly String()", tok)
		}
	}
}
