// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package lexer

import (
	"strings"
	"testing"
)

func TestTokenizeSimpleProgram(t *testing.T) {
	src := `use io;
func app() {
    def x as int init 21;
    io.printf($x + $x);
}`
	toks, err := Tokenize(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []TokenType{
		TOKEN_USE, TOKEN_IDENT, TOKEN_SEMI,
		TOKEN_FUNC, TOKEN_IDENT, TOKEN_LPAREN, TOKEN_RPAREN, TOKEN_LBRACE,
		TOKEN_DEFINE, TOKEN_IDENT, TOKEN_AS, TOKEN_INT_TYPE, TOKEN_INIT, TOKEN_INT, TOKEN_SEMI,
		// `io.printf` lexes as IDENT DOT IDENT under the namespace-first design.
		TOKEN_IDENT, TOKEN_DOT, TOKEN_IDENT,
		TOKEN_LPAREN, TOKEN_VARREF, TOKEN_PLUS, TOKEN_VARREF, TOKEN_RPAREN, TOKEN_SEMI,
		TOKEN_RBRACE, TOKEN_EOF,
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d:\n%v", len(toks), len(want), toks)
	}
	for i, w := range want {
		if toks[i].Type != w {
			t.Errorf("tok %d: got %s, want %s (lexeme=%q)", i, toks[i].Type, w, toks[i].Lexeme)
		}
	}
}

func TestTokenizeStringEscapes(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`"hello"`, "hello"},
		{`"line\nbreak"`, "line\nbreak"},
		{`"tab\there"`, "tab\there"},
		{`"quote\"in"`, `quote"in`},
		{`'single'`, "single"},
		{`'with\'apos'`, "with'apos"},
		{`"back\\slash"`, `back\slash`},
	}
	for _, c := range cases {
		toks, err := Tokenize(c.src)
		if err != nil {
			t.Errorf("Tokenize(%q) error: %v", c.src, err)
			continue
		}
		if len(toks) != 2 || toks[0].Type != TOKEN_STRING {
			t.Errorf("Tokenize(%q): unexpected tokens %v", c.src, toks)
			continue
		}
		if toks[0].Lexeme != c.want {
			t.Errorf("Tokenize(%q): got lexeme %q, want %q", c.src, toks[0].Lexeme, c.want)
		}
	}
}

func TestTokenizeNumbersAndOperators(t *testing.T) {
	toks, err := Tokenize("1 + 2 * 3 - 4 / 5 % 6;")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	want := []TokenType{
		TOKEN_INT, TOKEN_PLUS, TOKEN_INT, TOKEN_STAR, TOKEN_INT,
		TOKEN_MINUS, TOKEN_INT, TOKEN_SLASH, TOKEN_INT, TOKEN_PERCENT, TOKEN_INT,
		TOKEN_SEMI, TOKEN_EOF,
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(toks), len(want))
	}
	for i, w := range want {
		if toks[i].Type != w {
			t.Errorf("tok %d: got %s, want %s", i, toks[i].Type, w)
		}
	}
}

func TestTokenizeComments(t *testing.T) {
	// Comments and blank lines are emitted as trivia tokens; the
	// parser skips them at statement boundaries but the formatter
	// round-trips them.
	src := `# line comment
include /* block */ stdlib; # trailing
/* multi
   line */
def`
	toks, err := Tokenize(src)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	want := []TokenType{
		TOKEN_COMMENT_LINE,
		TOKEN_INCLUDE,
		TOKEN_COMMENT_BLOCK,
		TOKEN_IDENT,
		TOKEN_SEMI,
		TOKEN_COMMENT_LINE,
		TOKEN_COMMENT_BLOCK,
		TOKEN_DEFINE,
		TOKEN_EOF,
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(toks), len(want), toks)
	}
	for i, w := range want {
		if toks[i].Type != w {
			t.Errorf("tok %d: got %s, want %s", i, toks[i].Type, w)
		}
	}
}

func TestTokenizeShebang(t *testing.T) {
	// Line 1 col 1 `#!` is TOKEN_COMMENT_SHEBANG; an ordinary `#` on
	// line 1 below is TOKEN_COMMENT_LINE.
	src := "#!/usr/bin/env jennifer\n# normal\ndef"
	toks, err := Tokenize(src)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	want := []TokenType{
		TOKEN_COMMENT_SHEBANG,
		TOKEN_COMMENT_LINE,
		TOKEN_DEFINE,
		TOKEN_EOF,
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(toks), len(want), toks)
	}
	for i, w := range want {
		if toks[i].Type != w {
			t.Errorf("tok %d: got %s, want %s", i, toks[i].Type, w)
		}
	}
	if toks[0].Lexeme != "#!/usr/bin/env jennifer" {
		t.Errorf("shebang lexeme = %q", toks[0].Lexeme)
	}
}

func TestTokenizeBlankLineCollapses(t *testing.T) {
	// Multiple consecutive blank lines collapse into one
	// TOKEN_BLANK_LINE so the formatter never emits more than one
	// consecutive blank line on output.
	src := "def\n\n\n\ndef"
	toks, err := Tokenize(src)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	want := []TokenType{TOKEN_DEFINE, TOKEN_BLANK_LINE, TOKEN_DEFINE, TOKEN_EOF}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(toks), len(want), toks)
	}
	for i, w := range want {
		if toks[i].Type != w {
			t.Errorf("tok %d: got %s, want %s", i, toks[i].Type, w)
		}
	}
}

func TestTokenizeNestedBlockComment(t *testing.T) {
	// Block comments nest. A `/*` inside a block comment
	// increments the depth counter; only matching `*/`s close.
	src := "def /* outer /* inner */ still in outer */ def"
	toks, err := Tokenize(src)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	want := []TokenType{TOKEN_DEFINE, TOKEN_COMMENT_BLOCK, TOKEN_DEFINE, TOKEN_EOF}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(toks), len(want), toks)
	}
	if toks[1].Lexeme != "/* outer /* inner */ still in outer */" {
		t.Errorf("block comment lexeme = %q", toks[1].Lexeme)
	}
}

func TestTokenizeVarRefRejectsBareDollar(t *testing.T) {
	if _, err := Tokenize("$"); err == nil {
		t.Error("expected error for bare '$'")
	}
	if _, err := Tokenize("$ x"); err == nil {
		t.Error("expected error for '$ x' (space after $)")
	}
}

func TestTokenizeRejectsUnterminatedString(t *testing.T) {
	if _, err := Tokenize(`"unterminated`); err == nil {
		t.Error("expected error for unterminated string")
	}
}

// TestTokenizeRejectsNonASCIIDigit checks a non-ASCII digit (Arabic-Indic 3,
// U+0663) is a clean lex error, not a confusing downstream strconv failure.
func TestTokenizeRejectsNonASCIIDigit(t *testing.T) {
	if _, err := Tokenize("def x as int init ٣;"); err == nil {
		t.Error("expected lex error for non-ASCII digit U+0663")
	}
}

// TestTokenizeStripsLeadingBOM checks a leading UTF-8 BOM is dropped so the file
// lexes normally (and a shebang after it is still recognized).
func TestTokenizeStripsLeadingBOM(t *testing.T) {
	toks, err := Tokenize("\uFEFFdef x as int init 1;")
	if err != nil {
		t.Fatalf("lex with BOM: %v", err)
	}
	if len(toks) == 0 || toks[0].Type != TOKEN_DEFINE {
		t.Fatalf("BOM not stripped: first token %v", toks[0])
	}
	if toks[0].Col != 1 {
		t.Errorf("BOM shifted column: got col %d, want 1", toks[0].Col)
	}
}

// TestTokenizeM6Tokens covers the punctuation and keywords needed
// for list/map syntax: `[`, `]`, `:` and the keywords `list`, `map`,
// `of`, `to`, `in`.
func TestTokenizeM6Tokens(t *testing.T) {
	src := `def xs as list of int init [1, 2, 3];
def m as map of string to int init {"a": 1};
for (def x in $xs) { io.printf($x); }`
	toks, err := Tokenize(src)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	// Collect just the types so the assertion is readable.
	var types []TokenType
	for _, tok := range toks {
		if tok.Type == TOKEN_EOF {
			break
		}
		types = append(types, tok.Type)
	}
	// Spot-check by counting occurrences - the exact stream is long.
	count := func(want TokenType) int {
		n := 0
		for _, tt := range types {
			if tt == want {
				n++
			}
		}
		return n
	}
	want := map[TokenType]int{
		TOKEN_LIST:     1,
		TOKEN_MAP:      1,
		TOKEN_OF:       2,
		TOKEN_TO:       1,
		TOKEN_IN:       1,
		TOKEN_LBRACKET: 1,
		TOKEN_RBRACKET: 1,
		TOKEN_COLON:    1,
	}
	for tt, n := range want {
		if got := count(tt); got != n {
			t.Errorf("%s: got %d, want %d", tt, got, n)
		}
	}
}

// TestTokenizeBracketsAndColon directly checks the three new punctuation
// tokens to give a small per-character failure mode when something is
// broken in lexer.Next().
func TestTokenizeBracketsAndColon(t *testing.T) {
	cases := []struct {
		src  string
		want TokenType
	}{
		{"[", TOKEN_LBRACKET},
		{"]", TOKEN_RBRACKET},
		{":", TOKEN_COLON},
	}
	for _, c := range cases {
		toks, err := Tokenize(c.src)
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		if len(toks) < 2 || toks[0].Type != c.want {
			t.Errorf("%q: got %+v, want %s", c.src, toks, c.want)
		}
	}
}

// TestTokenizeIdentifierUnderscores covers the constant-name relaxation:
// the lexer accepts `_` inside IDENTs (so `MAX_RETRIES` is a single token),
// but rejects identifiers that *end* with `_` since no name kind allows
// that. A leading `_` is still rejected because `_` isn't an isIdentStart.
//
// The lexer deliberately permits consecutive `_` (e.g. `MAX__INT`); the
// "no consecutive underscores" rule applies only to constant names and
// is enforced by the parser, so the lexer can stay context-free.
func TestTokenizeIdentifierUnderscores(t *testing.T) {
	// Accepted by the lexer (parser may still reject for its own reasons).
	for _, src := range []string{"MAX_RETRIES", "MAX__INT", "FOO_BAR_BAZ", "A_B"} {
		toks, err := Tokenize(src)
		if err != nil {
			t.Errorf("%q: unexpected lex error: %v", src, err)
			continue
		}
		if len(toks) < 1 || toks[0].Type != TOKEN_IDENT || toks[0].Lexeme != src {
			t.Errorf("%q: expected single IDENT lexeme, got %+v", src, toks)
		}
	}
	// Rejected: trailing `_`.
	for _, src := range []string{"MAX_", "FOO__", "X_"} {
		if _, err := Tokenize(src); err == nil {
			t.Errorf("%q: expected lex error for trailing `_`", src)
		}
	}
	// Rejected: leading `_` - the lexer never starts an identifier on `_`,
	// so this falls through to "unexpected character".
	if _, err := Tokenize("_MAX"); err == nil {
		t.Error("expected lex error for leading `_`")
	}
}

func TestTokenizeFloatLiterals(t *testing.T) {
	cases := []struct {
		src    string
		want   TokenType
		lexeme string
		extra  []TokenType // extra tokens before EOF
	}{
		{"3.14", TOKEN_FLOAT, "3.14", nil},
		{"0.5", TOKEN_FLOAT, "0.5", nil},
		{"42", TOKEN_INT, "42", nil},
		// trailing dot without digit is INT(3) DOT
		{"3.", TOKEN_INT, "3", []TokenType{TOKEN_DOT}},
		// dot followed by ident (file-import shape) stays INT(3) DOT IDENT(j)
		{"3.j", TOKEN_INT, "3", []TokenType{TOKEN_DOT, TOKEN_IDENT}},
	}
	for _, c := range cases {
		toks, err := Tokenize(c.src)
		if err != nil {
			t.Errorf("Tokenize(%q): %v", c.src, err)
			continue
		}
		if toks[0].Type != c.want || toks[0].Lexeme != c.lexeme {
			t.Errorf("Tokenize(%q): first token = %s(%q), want %s(%q)", c.src, toks[0].Type, toks[0].Lexeme, c.want, c.lexeme)
		}
		for i, e := range c.extra {
			if toks[i+1].Type != e {
				t.Errorf("Tokenize(%q): tok[%d] = %s, want %s", c.src, i+1, toks[i+1].Type, e)
			}
		}
	}
}

func TestTokenizeComparisonOperators(t *testing.T) {
	toks, err := Tokenize("< > <= >= == != =")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []TokenType{TOKEN_LT, TOKEN_GT, TOKEN_LE, TOKEN_GE, TOKEN_EQ, TOKEN_NEQ, TOKEN_ASSIGN, TOKEN_EOF}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(toks), len(want), toks)
	}
	for i, w := range want {
		if toks[i].Type != w {
			t.Errorf("tok %d: got %s, want %s", i, toks[i].Type, w)
		}
	}
	// The `!=` token keeps its two-char lexeme so `jennifer fmt` re-emits it.
	if toks[5].Lexeme != "!=" {
		t.Errorf("NEQ lexeme: got %q, want %q", toks[5].Lexeme, "!=")
	}
}

// A bare `!` is not an operator (logical negation is the word `not`); it lexes
// to a positioned error that points at both `not` and `!=`.
func TestTokenizeBareBangIsFriendlyError(t *testing.T) {
	_, err := Tokenize("$x = ! $y")
	if err == nil {
		t.Fatal("expected a lex error for bare '!', got nil")
	}
	msg := err.Error()
	for _, want := range []string{"'!'", "not", "!="} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q should mention %q", msg, want)
		}
	}
}

func TestTokenizeDeferKeyword(t *testing.T) {
	toks, err := Tokenize("defer cleanup();")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if toks[0].Type != TOKEN_DEFER {
		t.Errorf("first token = %s, want DEFER", toks[0].Type)
	}
	// `defer` is a keyword, not an identifier.
	if toks[1].Type != TOKEN_IDENT || toks[1].Lexeme != "cleanup" {
		t.Errorf("second token = %s(%q), want IDENT(cleanup)", toks[1].Type, toks[1].Lexeme)
	}
}

func TestTokenizeM2Keywords(t *testing.T) {
	src := "const if elseif else while for true false null float bool return"
	toks, err := Tokenize(src)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []TokenType{
		TOKEN_CONST, TOKEN_IF, TOKEN_ELSEIF, TOKEN_ELSE, TOKEN_WHILE, TOKEN_FOR,
		TOKEN_TRUE, TOKEN_FALSE, TOKEN_NULL, TOKEN_FLOAT_TYPE, TOKEN_BOOL_TYPE, TOKEN_RETURN, TOKEN_EOF,
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(toks), len(want), toks)
	}
	for i, w := range want {
		if toks[i].Type != w {
			t.Errorf("tok %d: got %s (%q), want %s", i, toks[i].Type, toks[i].Lexeme, w)
		}
	}
}

func TestTokenizeLogicalKeywords(t *testing.T) {
	toks, err := Tokenize("and or not")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []TokenType{TOKEN_AND, TOKEN_OR, TOKEN_NOT, TOKEN_EOF}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(toks), len(want))
	}
	for i, w := range want {
		if toks[i].Type != w {
			t.Errorf("tok %d: got %s, want %s", i, toks[i].Type, w)
		}
	}
}

func TestTokenizeDefAndFuncKeywords(t *testing.T) {
	// `def` introduces a variable/constant; `func` introduces a method.
	// `define` is no longer a keyword - it lexes as a plain identifier.
	defToks, _ := Tokenize("def")
	funcToks, _ := Tokenize("func")
	defineToks, _ := Tokenize("define")
	if defToks[0].Type != TOKEN_DEFINE {
		t.Errorf("def -> %s, want TOKEN_DEFINE", defToks[0].Type)
	}
	if funcToks[0].Type != TOKEN_FUNC {
		t.Errorf("func -> %s, want TOKEN_FUNC", funcToks[0].Type)
	}
	if defineToks[0].Type != TOKEN_IDENT {
		t.Errorf("define -> %s, want TOKEN_IDENT (no longer a keyword)", defineToks[0].Type)
	}
}

func TestTokenizeRejectsUnterminatedBlockComment(t *testing.T) {
	if _, err := Tokenize(`/* never closed`); err == nil {
		t.Error("expected error for unterminated block comment")
	}
}

func TestTokenizeTracksLineAndColumn(t *testing.T) {
	src := "import\n  stdlib;"
	toks, err := Tokenize(src)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if toks[0].Line != 1 || toks[0].Col != 1 {
		t.Errorf("import at %d:%d, want 1:1", toks[0].Line, toks[0].Col)
	}
	if toks[1].Line != 2 || toks[1].Col != 3 {
		t.Errorf("stdlib at %d:%d, want 2:3", toks[1].Line, toks[1].Col)
	}
}

func TestTokenizeErrdeferKeyword(t *testing.T) {
	toks, err := Tokenize("errdefer cleanup();")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if toks[0].Type != TOKEN_ERRDEFER {
		t.Errorf("first token = %s, want ERRDEFER", toks[0].Type)
	}
	// `errdefer` is a keyword, not an identifier.
	if toks[1].Type != TOKEN_IDENT || toks[1].Lexeme != "cleanup" {
		t.Errorf("second token = %s(%q), want IDENT(cleanup)", toks[1].Type, toks[1].Lexeme)
	}
}
