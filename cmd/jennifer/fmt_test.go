// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lexer"
	"jennifer-lang.dev/jennifer/internal/parser"
	"jennifer-lang.dev/jennifer/internal/preproc"
	"jennifer-lang.dev/jennifer/internal/stdlib"
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
			// Examples whose output depends on wall time can't be
			// compared between two consecutive runs. They still get
			// the formatter applied (errors would surface as parse
			// failures), but we skip the equality check.
			if name == "benchmark.j" {
				t.Skipf("%s prints wall-clock timings; output varies between runs", name)
				return
			}
			// term.j drives raw-mode terminal input: run interactively it would
			// block on a key read (and could disrupt the terminal running the
			// tests). It guards on os.isTerminal, so it never produces golden
			// output; skip it here rather than risk a hang.
			if name == "term.j" {
				t.Skipf("%s reads raw terminal input; not runnable non-interactively", name)
				return
			}
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
	stdlib.InstallAll(in)
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
			`io.printf( "hi" );`,
			"io.printf(\"hi\");\n",
		},
		{
			"keyword if takes space before paren",
			`if($x>0){return;}`,
			"if ($x > 0) {\n    return;\n}\n",
		},
		{
			"for header keeps semicolons inline",
			`for(def i as int init 0;$i<3;$i=$i+1){io.printf("x");}`,
			"for (def i as int init 0; $i < 3; $i = $i + 1) {\n    io.printf(\"x\");\n}\n",
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
			`os . getEnv ( "HOME" );`,
			"os.getEnv(\"HOME\");\n",
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

// TestFmtBlockOpeners covers the block-opener token set: `{` after
// `try`, `spawn`, and `repeat` must expand as a statement block
// (newline+indent), not collapse to a map-literal-style inline body.
// Regression for the bug where these three braced constructs
// stayed inline because openBlock only fired for `)` and `else`.
func TestFmtBlockOpeners(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			"try/catch expands",
			`try{def x as int init 1;$x=$x+1;}catch(e){io.printf("boom");}`,
			"try {\n    def x as int init 1;\n    $x = $x + 1;\n} catch (e) {\n    io.printf(\"boom\");\n}\n",
		},
		{
			"spawn block expands",
			`def t as task of int init spawn{return 42;};`,
			"def t as task of int init spawn {\n    return 42;\n};\n",
		},
		{
			"repeat/until expands with cuddled until",
			`repeat{$i=$i+1;}until($i>3);`,
			"repeat {\n    $i = $i + 1;\n} until ($i > 3);\n",
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

// TestFmtStructDeclMultiline covers the struct-declaration reflow:
// `def struct Name { f as T, g as U };` must land one field per line,
// with `};` cuddled on the closing brace. A short struct / enum literal
// (`Name{f: v, g: w}`) stays inline with a tight brace and tight body
// body that mark it as bound data (a long one wraps one field per line).
func TestFmtStructDeclMultiline(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			"two-field struct decl expands",
			`def struct Point{x as int,y as int};`,
			"def struct Point {\n    x as int,\n    y as int\n};\n",
		},
		{
			"struct decl with list-of-int field",
			`def struct Bag{items as list of int,count as int};`,
			"def struct Bag {\n    items as list of int,\n    count as int\n};\n",
		},
		{
			"short struct literal stays inline, tight everywhere",
			`def p as Point init Point{x:1,y:2};`,
			"def p as Point init Point{x: 1, y: 2};\n",
		},
		{
			"struct literal inside a program keeps outer indent",
			`func f(){def p as Point init Point{x:1,y:2};return $p;}`,
			"func f() {\n    def p as Point init Point{x: 1, y: 2};\n    return $p;\n}\n",
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

// TestFmtColumnReflow covers the column-based reflow at `+` / `and`
// / `or`. Source line breaks at these joiners are preserved even
// when the whole expression would fit on one line, so a human's
// deliberate multi-line string-concat survives round-trip. Short
// expressions never wrap - the formatter doesn't insert breaks
// where the user didn't ask for them.
func TestFmtColumnReflow(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			"short concat stays on one line",
			`def s as string init "a" + "b" + "c";`,
			"def s as string init \"a\" + \"b\" + \"c\";\n",
		},
		{
			"source break after + is preserved",
			"def s as string init \"aa\" +\n\"bb\" +\n\"cc\";\n",
			"def s as string init \"aa\" +\n    \"bb\" +\n    \"cc\";\n",
		},
		{
			"source break after and preserved",
			"if ($ok and\n$ready) {\nreturn;\n}\n",
			"if ($ok and\n    $ready) {\n    return;\n}\n",
		},
		{
			// Fills the line, then breaks after the `+` whose next operand would
			// overflow - so every line stays under the limit (the old reactive
			// break left a 108-column first line).
			"long concat auto-wraps after +",
			`def s as string init "` + strings.Repeat("x", 40) + `" + "` + strings.Repeat("y", 40) + `" + "` + strings.Repeat("z", 40) + `";`,
			"def s as string init \"" + strings.Repeat("x", 40) + "\" +\n    \"" + strings.Repeat("y", 40) + "\" + \"" + strings.Repeat("z", 40) + "\";\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := fmtSource(t, c.src)
			if got != c.want {
				t.Errorf("got %q\nwant %q", got, c.want)
			}
			// Idempotency: formatting the output again produces the
			// same output. Column-reflow decisions from the first
			// pass have to be preserved by the source-line-break
			// rule on the second pass.
			twice := fmtSource(t, got)
			if twice != got {
				t.Errorf("not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", got, twice)
			}
		})
	}
}

// TestFmtLenHugsParen covers the `len(EXPR)` built-in: as a
// keyword-shaped call it must hug its `(`, matching how user
// method calls and type-conversion casts render.
func TestFmtLenHugsParen(t *testing.T) {
	got := fmtSource(t, `def n as int init len ( $xs );`)
	want := "def n as int init len($xs);\n"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

// TestFmtPreservesComments exercises trivia preservation: line
// comments (leading and trailing), block comments, blank lines, and
// shebang.
func TestFmtPreservesComments(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			"leading line comment on its own line",
			"# top\nuse io;\n",
			"# top\nuse io;\n",
		},
		{
			"trailing line comment same source line",
			"use io; # imports\n",
			"use io; # imports\n",
		},
		{
			"single blank line separates blocks",
			"use io;\n\ndef x as int init 1;\n",
			"use io;\n\ndef x as int init 1;\n",
		},
		{
			"consecutive blank lines collapse to one",
			"use io;\n\n\n\ndef x as int init 1;\n",
			"use io;\n\ndef x as int init 1;\n",
		},
		{
			"shebang stays at file head",
			"#!/usr/bin/env -S jennifer run\nuse io;\n",
			"#!/usr/bin/env -S jennifer run\nuse io;\n",
		},
		{
			"block comment inline preserved",
			"def x as int init /* note */ 5;\n",
			"def x as int init /* note */ 5;\n",
		},
		{
			"nested block comment",
			"def x as int init /* outer /* inner */ still */ 5;\n",
			"def x as int init /* outer /* inner */ still */ 5;\n",
		},
		{
			"leading comment block before def",
			"# why this matters\ndef x as int init 1;\n",
			"# why this matters\ndef x as int init 1;\n",
		},
		{
			"inline block comment between LPAREN and operand",
			"io.printf(/* note */ $x);\n",
			"io.printf(/* note */ $x);\n",
		},
		{
			"inline block comment between operand and operator",
			"def y as int init 1 /* foo */ + 2;\n",
			"def y as int init 1 /* foo */ + 2;\n",
		},
		{
			"inline block comment before RPAREN",
			"io.printf($x /* trail */);\n",
			"io.printf($x /* trail */);\n",
		},
		{
			"leading doc comment before func gets its own line",
			"/** doc */\nfunc f() { return; }\n",
			"/** doc */\nfunc f() {\n    return;\n}\n",
		},
		{
			"multiline doc comment before def preserved on its own lines",
			"/**\n * Summary.\n */\ndef const MAX as int init 5;\n",
			"/**\n * Summary.\n */\ndef const MAX as int init 5;\n",
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

// TestFmtCanonicalizesPragmas confirms fmt rewrites a `# pragma-jennifer-*:`
// requirement directive to its canonical single-spaced form (fixing missing/wide
// spacing) while leaving ordinary comments - including prose that merely mentions
// the marker - untouched.
func TestFmtCanonicalizesPragmas(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			"no space after hash and wide gaps",
			"#pragma-jennifer-version:                  >=0.1.0\nuse io;\n",
			"# pragma-jennifer-version: >=0.1.0\nuse io;\n",
		},
		{
			"version internal whitespace is stripped entirely",
			"#pragma-jennifer-version:          >=            0. 5 .0\nuse io;\n",
			"# pragma-jennifer-version: >=0.5.0\nuse io;\n",
		},
		{
			"capability list normalizes to comma-space without merging names",
			"#  pragma-jennifer-capability:   net   exec,sql\nuse io;\n",
			"# pragma-jennifer-capability: net, exec, sql\nuse io;\n",
		},
		{
			"already canonical is stable",
			"# pragma-jennifer-version: >=0.24.0\nuse io;\n",
			"# pragma-jennifer-version: >=0.24.0\nuse io;\n",
		},
		{
			"prose mentioning the marker is left verbatim",
			"# see the pragma-jennifer-version: note\nuse io;\n",
			"# see the pragma-jennifer-version: note\nuse io;\n",
		},
		{
			"malformed directive (no colon) is left verbatim",
			"# pragma-jennifer-version >=0.1.0\nuse io;\n",
			"# pragma-jennifer-version >=0.1.0\nuse io;\n",
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

// TestFmtMatchReflow pins the canonical `match` layout: each `when` / `else` arm
// starts on its own line (a flat case list, like switch/match in other
// languages). An arm holding a single short statement stays inline
// (`when 1 { a(); }`); a multi-statement arm expands one statement per line.
// Unlike an if-chain's `} else {`, match arms do NOT cuddle the previous arm's
// `}`.
func TestFmtMatchReflow(t *testing.T) {
	src := `use io;
match ($x) { when 1 { a(); } when 2, 3 { b(); c(); } else { d(); } }`
	want := `use io;
match ($x) {
    when 1 { a(); }
    when 2, 3 {
        b();
        c();
    }
    else { d(); }
}
`
	if got := fmtSource(t, src); got != want {
		t.Errorf("match reflow mismatch:\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

// TestFmtTriviaBeforeCloseBrace covers the fix for a trailing comment or a blank
// line immediately before a closing `}`: the brace must still dedent to its
// block's indent, not stay at the body indent. A `}` that closes a map literal
// (which does not dedent) must be unaffected.
func TestFmtTriviaBeforeCloseBrace(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"trailing comment before block close",
			"use io;\nif (1 == 1) {\n    a();   # note\n}\n",
			"use io;\nif (1 == 1) {\n    a(); # note\n}\n"},
		{"blank line before block close",
			"use io;\nif (1 == 1) {\n\n    a();\n\n}\n",
			"use io;\nif (1 == 1) {\n\n    a();\n\n}\n"},
		{"trailing comment before struct-decl close",
			"def struct P {\n    x as int,\n    y as int   # last\n};\n",
			"def struct P {\n    x as int,\n    y as int # last\n};\n"},
		{"trailing comment before match-arm close",
			"use io;\nmatch ($x) {\n    when 1 {\n        a();  # c\n    }\n}\n",
			"use io;\nmatch ($x) {\n    when 1 {\n        a(); # c\n    }\n}\n"},
	}
	for _, c := range cases {
		got := fmtSource(t, c.src)
		if got != c.want {
			t.Errorf("%s:\n--- got ---\n%s--- want ---\n%s", c.name, got, c.want)
		}
		// idempotent
		if again := fmtSource(t, got); again != got {
			t.Errorf("%s: not idempotent:\n--- once ---\n%s--- twice ---\n%s", c.name, got, again)
		}
	}
}

// TestFmtLiteralWrapping covers the length- and shape-aware literal formatting:
// a short struct / enum / map / list literal stays inline (struct / enum
// literals with a tight brace and tight body), an empty literal stays tight,
// and an inline arm keeps a single short statement on its line.
func TestFmtLiteralWrapping(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"short struct literal: tight body (no padding)",
			`def p as P init P{a:1,b:2};`,
			"def p as P init P{a: 1, b: 2};\n",
		},
		{
			"short map literal stays tight",
			`def m as map of string to int init {"a":1,"b":2};`,
			"def m as map of string to int init {\"a\": 1, \"b\": 2};\n",
		},
		{
			"short list stays inline",
			`def xs as list of int init [1,2,3];`,
			"def xs as list of int init [1, 2, 3];\n",
		},
		{
			"empty struct literal is tight",
			`def p as P init P{};`,
			"def p as P init P{};\n",
		},
		{
			"nested struct literal stays tight",
			`def c as C init C{name:"x",inner:D{k:1}};`,
			"def c as C init C{name: \"x\", inner: D{k: 1}};\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := fmtSource(t, c.src)
			if got != c.want {
				t.Errorf("got %q\nwant %q", got, c.want)
			}
			if again := fmtSource(t, got); again != got {
				t.Errorf("not idempotent:\n--- once ---\n%s--- twice ---\n%s", got, again)
			}
		})
	}
}

// TestFmtNeverAuthorsLongLine is the fmt / lint contract (M23.9): for source
// whose over-length is entirely due to wrappable constructs - a long literal or
// a binary-operator chain - fmt's output has no line past the 100-column limit,
// so fmt never hands `lint` an L203 (line-too-long) the source did not already
// have. A single over-long token or a long call-argument list has no safe break
// point and is deliberately out of scope (it stays on one line).
func TestFmtNeverAuthorsLongLine(t *testing.T) {
	srcs := []string{
		// long struct literal (wraps one field per line)
		`def r as Rule init Rule{chain:"forward",action:"drop",proto:"tcp",src:"10.0.0.0/8",dst:"0.0.0.0/0",comment:"a fairly long descriptive comment here"};`,
		// long list of maps (list wraps; each short map stays inline on its line)
		`def rows as list of map of string to string init [{"id":"1","name":"alpha","note":"first row here"},{"id":"2","name":"beta","note":"second row here too indeed"}];`,
		// long concat (fills, then breaks after a joiner)
		`def s as string init "prefix aaaaaaaaaaaaaaaaaaaaaaaaaaaa" + $x + " middle bbbbbbbbbbbbbbbbbbbbbbbb " + $y + " suffix cccccccccccccccccccccccccccc";`,
		// nested literals inside a block (extra indent), decided independently
		`func f() { def c as Cfg init Cfg{name:"service",tags:["a","b","c"],limits:Limits{max:100,min:1},note:"some longer note to push the width over the limit"}; }`,
	}
	for i, src := range srcs {
		out := fmtSource(t, src)
		for _, line := range strings.Split(out, "\n") {
			if w := len([]rune(line)); w > maxLineLength {
				t.Errorf("case %d: fmt authored a %d-column line (over %d):\n%s",
					i, w, maxLineLength, line)
			}
		}
		if again := fmtSource(t, out); again != out {
			t.Errorf("case %d: not idempotent:\n--- once ---\n%s--- twice ---\n%s", i, out, again)
		}
	}
}

// TestFmtWriteInPlace covers `jennifer fmt -w`: it rewrites a file to its
// canonical form, leaves an already-canonical file untouched, and refuses to
// send several files to stdout without -w.
func TestFmtWriteInPlace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.j")
	if err := os.WriteFile(path, []byte("func f(){def p as P init P{x:1,y:2};}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runFmt([]string{"-w", path}); code != 0 {
		t.Fatalf("fmt -w exit %d", code)
	}
	got, _ := os.ReadFile(path)
	want := "func f() {\n    def p as P init P{x: 1, y: 2};\n}\n"
	if string(got) != want {
		t.Errorf("in-place result:\n got %q\nwant %q", got, want)
	}
	// A second pass is a no-op (already canonical); content stays put.
	info1, _ := os.Stat(path)
	if code := runFmt([]string{"--write", path}); code != 0 {
		t.Fatalf("second fmt -w exit %d", code)
	}
	again, _ := os.ReadFile(path)
	if string(again) != want {
		t.Errorf("second pass changed a canonical file: %q", again)
	}
	if info2, err := os.Stat(path); err == nil && !info2.ModTime().Equal(info1.ModTime()) {
		t.Errorf("canonical file was rewritten (mtime changed) on a no-op pass")
	}
	// Several files to stdout (no -w) is an error.
	b := filepath.Join(dir, "b.j")
	os.WriteFile(b, []byte("def x as int init 1;\n"), 0o644)
	if code := runFmt([]string{path, b}); code != 2 {
		t.Errorf("multiple files to stdout: got exit %d, want 2", code)
	}
}

// TestFmtKeywordMemberCallHugsParen covers a namespaced call whose member name
// is a word the lexer tokenises as a keyword - a type keyword (`json.map()`,
// `json.list()`) or a statement keyword (`strings.repeat()`). The `.`-qualified
// call must still hug its `(` (no `json.map ()` / `strings.repeat ()`), while a
// genuine leading keyword (`if (`, `while (`) keeps its space.
func TestFmtKeywordMemberCallHugsParen(t *testing.T) {
	cases := map[string]string{
		`def r as json.Value init json.map ( );`:        "def r as json.Value init json.map();\n",
		`def l as json.Value init json.list ( );`:       "def l as json.Value init json.list();\n",
		`def d as toml.Value init toml.map ( );`:        "def d as toml.Value init toml.map();\n",
		`def k as string init strings.repeat ("k", 1);`: "def k as string init strings.repeat(\"k\", 1);\n",
		"if ($x) {\nreturn;\n}":                         "if ($x) {\n    return;\n}\n",
		"while ($x) {\nreturn;\n}":                      "while ($x) {\n    return;\n}\n",
	}
	for src, want := range cases {
		if got := fmtSource(t, src); got != want {
			t.Errorf("src %q:\n got %q\nwant %q", src, got, want)
		}
	}
}

// TestFmtSpawnBlockInContainer covers a container holding `spawn { ... }` block
// elements: the block is inherently multiline, so the container must wrap (one
// element per line) and a block `}` must hug a following `,` / `)` / `;` rather
// than pushing it to the next line (`},` not `}\n,`).
func TestFmtSpawnBlockInContainer(t *testing.T) {
	src := "def many as list of task of int init [spawn { return 1; }, spawn { return 2; }];"
	want := "def many as list of task of int init [\n" +
		"    spawn {\n        return 1;\n    },\n" +
		"    spawn {\n        return 2;\n    }\n" +
		"];\n"
	if got := fmtSource(t, src); got != want {
		t.Errorf("spawn-in-list:\n got %q\nwant %q", got, want)
	}
	// A block `}` hugs the `)` and `;` of an enclosing call (`});`).
	call := "use task;\ndef v as int init task.wait(spawn { return 5; });"
	callWant := "use task;\ndef v as int init task.wait(spawn {\n    return 5;\n});\n"
	if got := fmtSource(t, call); got != callWant {
		t.Errorf("spawn-in-call:\n got %q\nwant %q", got, callWant)
	}
	// Idempotent.
	if again := fmtSource(t, fmtSource(t, src)); again != want {
		t.Errorf("not idempotent")
	}
}

// TestFmtRejectsDirectory covers the deliberate non-feature: `fmt` formats the
// files it is named and does no directory walking (file selection is the
// shell's job), so a directory argument is a usage error, not a tree walk.
func TestFmtRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.j"), []byte("func f(){return;}\n"), 0o644)
	if code := runFmt([]string{"-w", dir}); code != 2 {
		t.Errorf("directory argument: got exit %d, want 2 (usage error)", code)
	}
	// The .j inside must be untouched (no silent walk).
	got, _ := os.ReadFile(filepath.Join(dir, "a.j"))
	if string(got) != "func f(){return;}\n" {
		t.Errorf("directory contents were rewritten: %q", got)
	}
}

// TestFmtPreservesLiteralLexemes covers M23.10 fidelity: fmt re-emits a numeric
// or string literal's exact source spelling (digit separators, base prefix,
// quote style, escapes, embedded newlines) rather than the processed value.
func TestFmtPreservesLiteralLexemes(t *testing.T) {
	cases := map[string]string{
		`def n as int init 1_000_000;`:     "def n as int init 1_000_000;\n",
		`def h as int init 0xDEAD_BEEF;`:   "def h as int init 0xDEAD_BEEF;\n",
		`def b as int init 0b1010_0110;`:   "def b as int init 0b1010_0110;\n",
		`def f as float init 1_000.000_5;`: "def f as float init 1_000.000_5;\n",
		`def q as string init 'single';`:   "def q as string init 'single';\n",
		"def e as string init \"a\\nb\";":  "def e as string init \"a\\nb\";\n",
		"def m as string init \"x\ny\";":   "def m as string init \"x\ny\";\n", // raw newline preserved
	}
	for src, want := range cases {
		if got := fmtSource(t, src); got != want {
			t.Errorf("src %q:\n got %q\nwant %q", src, got, want)
		}
		if again := fmtSource(t, fmtSource(t, src)); again != want {
			t.Errorf("src %q not idempotent", src)
		}
	}
}

// TestFmtWrapsLongCall covers M23.10 call-argument wrapping: a long call with two
// or more arguments wraps one per line with `)` hugging the last argument; a
// short call, a single-argument call, and an empty call stay inline.
func TestFmtWrapsLongCall(t *testing.T) {
	long := `def s as E init ns.event("standup-2024-06-17@team", fromIso("2024-06-17T09:00:00Z"), fromIso("2024-06-17T09:15:00Z"), "Daily standup");`
	want := "def s as E init ns.event(\n" +
		"    \"standup-2024-06-17@team\",\n" +
		"    fromIso(\"2024-06-17T09:00:00Z\"),\n" +
		"    fromIso(\"2024-06-17T09:15:00Z\"),\n" +
		"    \"Daily standup\");\n"
	if got := fmtSource(t, long); got != want {
		t.Errorf("long call:\n got %q\nwant %q", got, want)
	}
	inline := map[string]string{
		`def x as int init foo(1, 2);`: "def x as int init foo(1, 2);\n", // short: inline
		`func f() { a(); }`:            "func f() {\n    a();\n}\n",      // empty: tight
	}
	for src, w := range inline {
		if got := fmtSource(t, src); got != w {
			t.Errorf("src %q:\n got %q\nwant %q", src, got, w)
		}
	}
	if again := fmtSource(t, fmtSource(t, long)); again != want {
		t.Errorf("long call not idempotent")
	}
}

// TestFmtPreservesTokenStream is the definitive fidelity guard: across every .j
// in examples/ and modules/, formatting must change only whitespace/trivia - the
// sequence of non-trivia tokens (type + lexeme) is byte-identical before and
// after. This is what makes a corpus reflow safe: fmt can never alter a literal
// value, an identifier, or the program's structure.
func TestFmtPreservesTokenStream(t *testing.T) {
	roots := []string{
		filepath.Join("..", "..", "examples"),
		filepath.Join("..", "..", "modules"),
	}
	nonTrivia := func(src string) []lexer.Token {
		toks, err := lexer.TokenizeWithFile(src, "<f>")
		if err != nil {
			return nil
		}
		var out []lexer.Token
		for _, tk := range toks {
			switch tk.Type {
			case lexer.TOKEN_COMMENT_LINE, lexer.TOKEN_COMMENT_BLOCK,
				lexer.TOKEN_COMMENT_SHEBANG, lexer.TOKEN_BLANK_LINE:
				continue
			}
			out = append(out, tk)
		}
		return out
	}
	for _, root := range roots {
		abs, _ := filepath.Abs(root)
		filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".j") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			orig := nonTrivia(string(b))
			if orig == nil {
				return nil // unlexable source; not this test's concern
			}
			formatted := fmtSource(t, string(b))
			after := nonTrivia(formatted)
			rel, _ := filepath.Rel(abs, path)
			if len(orig) != len(after) {
				t.Errorf("%s: token count changed %d -> %d after fmt", rel, len(orig), len(after))
				return nil
			}
			for i := range orig {
				if orig[i].Type != after[i].Type || orig[i].Lexeme != after[i].Lexeme {
					t.Errorf("%s: token %d changed: %v(%q) -> %v(%q)", rel, i,
						orig[i].Type, orig[i].Lexeme, after[i].Type, after[i].Lexeme)
					return nil
				}
			}
			return nil
		})
	}
}

// TestFmtWriteFollowsSymlink (OF-022): `fmt -w` on a symlink rewrites its target
// and leaves the link a symlink, rather than replacing the link with a file.
func TestFmtWriteFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.j")
	link := filepath.Join(dir, "link.j")
	os.WriteFile(target, []byte("func f(){def p as P init P{x:1};}\n"), 0o644)
	if err := os.Symlink("target.j", link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if code := runFmt([]string{"-w", link}); code != 0 {
		t.Fatalf("fmt -w link exit %d", code)
	}
	if info, _ := os.Lstat(link); info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link.j is no longer a symlink after fmt -w")
	}
	got, _ := os.ReadFile(target)
	if want := "func f() {\n    def p as P init P{x: 1};\n}\n"; string(got) != want {
		t.Errorf("target not formatted through the link:\n got %q\nwant %q", got, want)
	}
}

// TestFmtWriteRespectsReadOnly (OF-023): `fmt -w` refuses a read-only file and
// leaves it untouched, rather than overriding the mode via the atomic rename.
func TestFmtWriteRespectsReadOnly(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ro.j")
	orig := "func g(){def q as Q init Q{y:2};}\n"
	os.WriteFile(p, []byte(orig), 0o444)
	if code := runFmt([]string{"-w", p}); code != 1 {
		t.Errorf("read-only file: got exit %d, want 1", code)
	}
	if got, _ := os.ReadFile(p); string(got) != orig {
		t.Errorf("read-only file was modified: %q", got)
	}
}

// TestSameCodeTokens (OF-025) pins the write-guard predicate: whitespace / comment
// differences are equal; a changed value, a dropped token, or a lost digit
// separator / quote style are not.
func TestSameCodeTokens(t *testing.T) {
	equal := [][2]string{
		{"def x as int init 1 ;", "def x as int init 1;"},     // whitespace
		{"def x as int init 1;  # c", "def x as int init 1;"}, // comment
		{"def n as int init 1_000_000;", "def n as int init 1_000_000;"},
	}
	for _, c := range equal {
		if !sameCodeTokens(c[0], c[1]) {
			t.Errorf("expected equal: %q vs %q", c[0], c[1])
		}
	}
	diff := [][2]string{
		{"def x as int init 1;", "def x as int init 2;"},               // value
		{"def x as int init 1;", "def x as int init 1 + 1;"},           // extra tokens
		{"def n as int init 1_000_000;", "def n as int init 1000000;"}, // digit-sep fidelity
		{"def q as string init 'x';", "def q as string init \"x\";"},   // quote fidelity
	}
	for _, c := range diff {
		if sameCodeTokens(c[0], c[1]) {
			t.Errorf("expected different: %q vs %q", c[0], c[1])
		}
	}
}

// TestFmtFuncSignatureBraceWidth pins the fix for the trailing ` {`: fmt counts
// the block-opening ` {` of a `func` signature when deciding whether to wrap, so
// it never emits a signature line wider than the 100-column lint limit. A
// signature whose joined `) {` form is 102 columns must wrap (and stay wrapped -
// the bug was a two-state fmt/lint loop).
func TestFmtFuncSignatureBraceWidth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sig.j")
	// Joined `) {` line is 102 columns (ASCII, so byte length == column count).
	src := "export func publish(dir as string, url as string, dbPath as string, outDir as string, now as string) {\n    return 1;\n}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runFmt([]string{"-w", path}); code != 0 {
		t.Fatalf("fmt -w exit %d", code)
	}
	got, _ := os.ReadFile(path)
	for i, line := range strings.Split(string(got), "\n") {
		if len(line) > 100 {
			t.Errorf("line %d is %d columns (over 100):\n%s", i+1, len(line), line)
		}
	}
	// Idempotent: the wrapped form is stable.
	first := string(got)
	if code := runFmt([]string{"-w", path}); code != 0 {
		t.Fatalf("second fmt -w exit %d", code)
	}
	if again, _ := os.ReadFile(path); string(again) != first {
		t.Errorf("fmt not idempotent on the wrapped signature:\n%s", again)
	}

	// A signature whose full `) {` line is exactly 100 columns must stay on one
	// line (not over-wrapped by the +2).
	fit := filepath.Join(dir, "fit.j")
	// "func f(" (7) + params (90) + ") {" (3) = 100.
	params := "p0 as int, p1 as int, p2 as int, p3 as int, p4 as int, p5 as int, p6 as intxxxxxxxxxxxxxxx"
	one := "func f(" + params + ") {\n    return 1;\n}\n"
	if len("func f("+params+") {") != 100 {
		t.Fatalf("test setup: signature is %d cols, want 100", len("func f("+params+") {"))
	}
	os.WriteFile(fit, []byte(one), 0o644)
	if code := runFmt([]string{"-w", fit}); code != 0 {
		t.Fatalf("fmt -w exit %d", code)
	}
	gotFit, _ := os.ReadFile(fit)
	if !strings.HasPrefix(string(gotFit), "func f("+params+") {\n") {
		t.Errorf("a 100-column signature was wrapped instead of kept on one line:\n%s", gotFit)
	}
}

// TestFmtCheckMode pins `jennifer fmt -l / --check`: it lists unformatted files
// to stdout without mutating them and exits 0 clean / 1 needs-formatting / 2
// broken - the non-mutating CI-gate mode.
func TestFmtCheckMode(t *testing.T) {
	dir := t.TempDir()
	clean := filepath.Join(dir, "clean.j")
	os.WriteFile(clean, []byte("func f() {\n    return 1;\n}\n"), 0o644)
	dirty := filepath.Join(dir, "dirty.j")
	os.WriteFile(dirty, []byte("func g(){return 2;}\n"), 0o644)

	if code := runFmt([]string{"--check", clean}); code != 0 {
		t.Errorf("--check on a clean file: exit %d, want 0", code)
	}

	// A dirty file lists to stdout and exits 1, and must not be rewritten.
	orig, _ := os.ReadFile(dirty)
	var code int
	out := captureStdout(t, func() { code = runFmt([]string{"-l", dirty}) })
	if code != 1 {
		t.Errorf("-l on a dirty file: exit %d, want 1", code)
	}
	if strings.TrimSpace(out) != dirty {
		t.Errorf("-l listed %q, want %q", strings.TrimSpace(out), dirty)
	}
	if after, _ := os.ReadFile(dirty); string(after) != string(orig) {
		t.Errorf("--check mutated the file it was only meant to check")
	}

	// A mix lists only the dirty one and exits 1.
	out = captureStdout(t, func() { code = runFmt([]string{"--check", clean, dirty}) })
	if code != 1 || strings.TrimSpace(out) != dirty {
		t.Errorf("--check mixed: exit %d out %q, want 1 and %q", code, strings.TrimSpace(out), dirty)
	}

	// A missing file is a broken invocation: exit 2.
	if code := runFmt([]string{"--check", filepath.Join(dir, "nope.j")}); code != 2 {
		t.Errorf("--check on a missing file: exit %d, want 2", code)
	}

	// -w and --check together is a usage error.
	if code := runFmt([]string{"-w", "--check", clean}); code != 2 {
		t.Errorf("-w --check together: exit %d, want 2", code)
	}
}
