// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/lexer"
	"github.com/mplx/jennifer-lang/internal/lib/convert"
	iolib "github.com/mplx/jennifer-lang/internal/lib/io"
	listslib "github.com/mplx/jennifer-lang/internal/lib/lists"
	mapslib "github.com/mplx/jennifer-lang/internal/lib/maps"
	mathlib "github.com/mplx/jennifer-lang/internal/lib/math"
	corelib "github.com/mplx/jennifer-lang/internal/lib/core"
	oslib "github.com/mplx/jennifer-lang/internal/lib/os"
	stringslib "github.com/mplx/jennifer-lang/internal/lib/strings"
	"github.com/mplx/jennifer-lang/internal/parser"
	"github.com/mplx/jennifer-lang/internal/preproc"
)

// fmtSource is a test helper: lex, format, return canonical text. Uses
// formatTokens directly so the assertion isolates the formatter from CLI
// concerns (file I/O, exit codes).
func fmtSource(t *testing.T, src string) string {
	t.Helper()
	toks, err := lexer.TokenizeWithFile(src, "<test>")
	if err != nil {
		t.Fatalf("lex %q: %v", src, err)
	}
	return formatTokens(toks)
}

// TestFmtRoundTripStability ensures the formatter is idempotent: a
// program formatted twice must be byte-identical to the once-formatted
// version. This is the bedrock invariant for any code formatter.
func TestFmtRoundTripStability(t *testing.T) {
	dir, err := filepath.Abs(filepath.Join("..", "..", "examples"))
	if err != nil {
		t.Fatalf("locate examples: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read examples: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".j") {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			bytes, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			once := fmtSource(t, string(bytes))
			twice := fmtSource(t, once)
			if once != twice {
				t.Errorf("fmt is not idempotent for %s:\n--- once ---\n%s\n--- twice ---\n%s",
					name, once, twice)
			}
		})
	}
}

// TestFmtPreservesRuntimeBehavior runs each example, formats it, runs
// the formatted version, and compares stdout. A formatter must never
// change a program's behavior.
func TestFmtPreservesRuntimeBehavior(t *testing.T) {
	dir, err := filepath.Abs(filepath.Join("..", "..", "examples"))
	if err != nil {
		t.Fatalf("locate examples: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".j") {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)
			origOut, err := runProgramOutput(path, "")
			if err != nil {
				t.Fatalf("run original: %v", err)
			}
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			fmted := fmtSource(t, string(src))
			fmtOut, err := runProgramOutput(path, fmted)
			if err != nil {
				t.Fatalf("run formatted: %v", err)
			}
			if origOut != fmtOut {
				t.Errorf("formatted %s produced different output:\n--- orig ---\n%s\n--- formatted run ---\n%s",
					name, origOut, fmtOut)
			}
		})
	}
}

// runProgramOutput runs a Jennifer source. If src is empty, the file at
// path is read; otherwise src is used directly and path is the absolute
// path used to resolve file imports. Returns captured stdout.
func runProgramOutput(path, src string) (string, error) {
	abs, _ := filepath.Abs(path)
	if src == "" {
		b, err := os.ReadFile(abs)
		if err != nil {
			return "", err
		}
		src = string(b)
	}
	tokens, err := lexer.TokenizeWithFile(src, abs)
	if err != nil {
		return "", err
	}
	tokens, err = preproc.Process(tokens, filepath.Dir(abs), abs)
	if err != nil {
		return "", err
	}
	prog, err := parser.ParseTokens(tokens)
	if err != nil {
		return "", err
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	convert.Install(in)
	mathlib.Install(in)
	stringslib.Install(in)
	listslib.Install(in)
	mapslib.Install(in)
	oslib.Install(in)
	corelib.Install(in)
	if err := in.Run(prog); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// TestFmtSpacingRules exercises the documented spacing rules with small
// inputs whose expected output is hand-written from docs/user-guide/style-guide.md.
func TestFmtSpacingRules(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			"binary operators get surrounding spaces",
			`def x as int init 1+2*3;`,
			"def x as int init 1 + 2 * 3;\n",
		},
		{
			"unary minus hugs its operand",
			`def x as int init -5;`,
			"def x as int init -5;\n",
		},
		{
			"unary minus with var",
			`$x=-$y;`,
			"$x = -$y;\n",
		},
		{
			"not keyword takes a space",
			`if(not $ok){return;}`,
			"if (not $ok) {\n    return;\n}\n",
		},
		{
			"call sites hug paren",
			`printf( "hi" );`,
			"printf(\"hi\");\n",
		},
		{
			"keyword if takes space before paren",
			`if($x>0){return;}`,
			"if ($x > 0) {\n    return;\n}\n",
		},
		{
			"for header keeps semicolons inline",
			`for(def i as int init 0;$i<3;$i=$i+1){printf("x");}`,
			"for (def i as int init 0; $i < 3; $i = $i + 1) {\n    printf(\"x\");\n}\n",
		},
		{
			"else cuddles closing brace",
			`if($x){return 1;}else{return 0;}`,
			"if ($x) {\n    return 1;\n} else {\n    return 0;\n}\n",
		},
		{
			"elseif cuddles too",
			`if($x){a();}elseif($y){b();}else{c();}`,
			"if ($x) {\n    a();\n} elseif ($y) {\n    b();\n} else {\n    c();\n}\n",
		},
		{
			"list literal no padding inside",
			`def xs as list of int init [ 1 , 2 , 3 ];`,
			"def xs as list of int init [1, 2, 3];\n",
		},
		{
			"empty list literal",
			`def xs as list of int init [ ];`,
			"def xs as list of int init [];\n",
		},
		{
			"map literal: no inside padding, space after colon",
			`def m as map of string to int init { "a" : 1 , "b" : 2 };`,
			"def m as map of string to int init {\"a\": 1, \"b\": 2};\n",
		},
		{
			"empty map literal",
			`def m as map of string to int init { };`,
			"def m as map of string to int init {};\n",
		},
		{
			"index read hugs target",
			`def y as int init $xs [ 0 ];`,
			"def y as int init $xs[0];\n",
		},
		{
			"chained index write hugs target",
			`$g [ 0 ] [ 1 ] = 99;`,
			"$g[0][1] = 99;\n",
		},
		{
			"for-each header",
			`for ( def x in $xs ) { return; }`,
			"for (def x in $xs) {\n    return;\n}\n",
		},
		{
			"block brace still expands",
			`func f() { return; }`,
			"func f() {\n    return;\n}\n",
		},
		{
			"qualified call hugs the dot",
			`os . platform (  );`,
			"os.platform();\n",
		},
		{
			"qualified call with args",
			`bio . translate ( $seq );`,
			"bio.translate($seq);\n",
		},
		{
			"qualified constant reference",
			`def x as int init bio . STOPS ;`,
			"def x as int init bio.STOPS;\n",
		},
		{
			"use with alias",
			`use bio   as   b ;`,
			"use bio as b;\n",
		},
		{
			"append form hugs $xs[]",
			`$xs [ ] = 42 ;`,
			"$xs[] = 42;\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := fmtSource(t, c.src)
			if got != c.want {
				t.Errorf("got %q\nwant %q", got, c.want)
			}
		})
	}
}
