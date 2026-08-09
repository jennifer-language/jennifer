// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package lexer

import (
	"strings"
	"testing"
)

// A pathologically deep nested block comment is a catchable lex error, not one
// giant unbounded comment token.
func TestBlockCommentNestingCapped(t *testing.T) {
	deep := strings.Repeat("/*", 2000) + "x" + strings.Repeat("*/", 2000)
	_, err := Tokenize(deep)
	if err == nil || !strings.Contains(err.Error(), "block comment nesting exceeds") {
		t.Fatalf("expected a nesting-cap lex error, got %v", err)
	}
	// Ordinary nesting still lexes.
	if _, err := Tokenize("/* outer /* inner */ back */ use io;"); err != nil {
		t.Errorf("ordinary nested comment should lex, got %v", err)
	}
}

// A source file that lexes into more than the token budget is a positioned,
// catchable lex error rather than an unbounded allocation (the "token bomb": a
// small, dense file amplifies to millions of Token structs). The budget is
// lowered here so the test needn't generate millions of tokens.
func TestTokenBudgetExceeded(t *testing.T) {
	saved := maxTokens
	maxTokens = 10
	defer func() { maxTokens = saved }()

	// 13 ';' -> 13 SEMI + EOF = 14 tokens, past the budget of 10.
	_, err := Tokenize(strings.Repeat(";", 13))
	if err == nil {
		t.Fatal("expected a token-budget error, got nil")
	}
	le, ok := err.(*LexError)
	if !ok {
		t.Fatalf("expected *LexError, got %T", err)
	}
	if !strings.Contains(le.Msg, "token budget exceeded") {
		t.Errorf("unexpected message: %q", le.Msg)
	}
	if le.Line < 1 || le.Col < 1 {
		t.Errorf("budget error must be positioned, got line=%d col=%d", le.Line, le.Col)
	}

	// Comfortably under the budget still lexes.
	if _, err := Tokenize(strings.Repeat(";", 5)); err != nil {
		t.Errorf("under-budget source should lex, got %v", err)
	}
}

// An over-long identifier / variable name is rejected, and neither the scan nor
// the error message retains the whole (potentially megabyte-long) run: the scan
// stops just past the 64-char cap and the message is truncated.
func TestOverlongIdentifierIsBoundedError(t *testing.T) {
	long := strings.Repeat("a", 200000)
	for _, c := range []struct{ src, want string }{
		{"def " + long + " as int init 1;", "identifier"},
		{"$" + long, "variable name"},
	} {
		_, err := Tokenize(c.src)
		if err == nil {
			t.Fatalf("expected an over-long error for %q...", c.src[:16])
		}
		msg := err.Error()
		if !strings.Contains(msg, "exceeds 64 characters") || !strings.Contains(msg, c.want) {
			t.Errorf("unexpected message: %q", msg)
		}
		// The message must be bounded (truncated), not carry the whole run.
		if len(msg) > 256 {
			t.Errorf("error message not truncated: %d bytes", len(msg))
		}
	}
}

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
		// double-quoted literals are cooked (escapes processed)
		{`"hello"`, "hello"},
		{`"line\nbreak"`, "line\nbreak"},
		{`"tab\there"`, "tab\there"},
		{`"quote\"in"`, `quote"in`},
		{`"back\\slash"`, `back\slash`},
		{`"it's"`, "it's"}, // embed a single quote via the cooked form
		// single-quoted literals are RAW (no escape processing)
		{`'single'`, "single"},
		{`'raw \n stays'`, `raw \n stays`},             // backslash-n is two literal chars
		{`'\d+\.\d+'`, `\d+\.\d+`},                     // a regex, verbatim
		{"'line one\nline two'", "line one\nline two"}, // multi-line raw spans a newline
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

func TestRawSingleQuotedStrings(t *testing.T) {
	// A backslash never escapes in a raw literal, so the first single quote ends
	// it: 'a\' is the two-char string `a\`, then `x` is an identifier.
	toks, err := Tokenize(`'a\' x`)
	if err != nil {
		t.Fatalf("Tokenize error: %v", err)
	}
	if toks[0].Type != TOKEN_STRING || toks[0].Lexeme != `a\` {
		t.Errorf("raw 'a\\' = %q (%v), want `a\\` string", toks[0].Lexeme, toks[0].Type)
	}
	if toks[1].Type != TOKEN_IDENT || toks[1].Lexeme != "x" {
		t.Errorf("token after raw string = %q (%v), want ident x", toks[1].Lexeme, toks[1].Type)
	}

	// An unterminated raw literal is a positioned lex error (like the cooked form).
	if _, err := Tokenize(`'no closing quote`); err == nil {
		t.Error("expected an unterminated-string error for an unclosed raw literal")
	}

	// fmt fidelity: the raw source span (with quotes) is preserved in Token.Raw,
	// so a raw literal re-emits as-is and does not become a cooked one.
	toks, err = Tokenize(`'raw \n'`)
	if err != nil {
		t.Fatalf("Tokenize error: %v", err)
	}
	if toks[0].Raw != `'raw \n'` {
		t.Errorf("Token.Raw = %q, want the verbatim single-quoted source", toks[0].Raw)
	}
}

// TestUnicodeEscapes covers the \u (4 hex, BMP) and \U (8 hex, any plane)
// escapes. The backslash and every escaped source is built from rune values
// rather than written as a literal \u, so an editor / tool that normalises a
// literal backslash-u sequence into its character cannot silently defeat the
// test (the escaped input must reach the lexer intact).
func TestUnicodeEscapes(t *testing.T) {
	bs := string(rune(0x5C)) // a single backslash

	pos := []struct {
		src  string
		want string
	}{
		{`"` + bs + `u0041"`, "A"},                             // A -> A
		{`"caf` + bs + `u00e9"`, "caf" + string(rune(0x00E9))}, // café (lowercase hex)
		{`"` + bs + `u20AC"`, string(rune(0x20AC))},            // € (uppercase hex)
		{`"` + bs + `U0001F600"`, string(rune(0x1F600))},       // astral plane, via \U
		{`"` + bs + `U0010FFFF"`, string(rune(0x10FFFF))},      // the maximum valid code point (boundary)
		{`"e` + bs + `u0301"`, "e" + string(rune(0x0301))},     // NFD: base + combining acute
	}
	for _, c := range pos {
		toks, err := Tokenize(c.src)
		if err != nil {
			t.Errorf("Tokenize(%q): %v", c.src, err)
			continue
		}
		if len(toks) < 1 || toks[0].Type != TOKEN_STRING || toks[0].Lexeme != c.want {
			t.Errorf("Tokenize(%q): lexeme = %q, want %q", c.src, toks[0].Lexeme, c.want)
		}
	}

	// A raw single-quoted string does NOT process \u - it stays literal.
	raw := `'` + bs + `u0041'`
	toks, err := Tokenize(raw)
	if err != nil {
		t.Fatalf("Tokenize(%q): %v", raw, err)
	}
	if toks[0].Lexeme != bs+"u0041" {
		t.Errorf("raw %q: lexeme = %q, want a literal backslash-u sequence", raw, toks[0].Lexeme)
	}

	// Invalid escapes are positioned lex errors, not a silent replacement char.
	bad := []string{
		`"` + bs + `uD800"`,     // low surrogate
		`"` + bs + `uDFFF"`,     // high surrogate
		`"` + bs + `U00110000"`, // above U+10FFFF
		`"` + bs + `U80000000"`, // high bit set: overflows int32 rune to negative
		`"` + bs + `UFFFFFFFF"`, // max 8-hex value: must not slip through as U+FFFD
		`"` + bs + `u12"`,       // too few hex digits (needs 4)
		`"` + bs + `uZZZZ"`,     // non-hex digits
		`"` + bs + `U0001F60"`,  // 7 digits (needs 8)
		`"` + bs + `u"`,         // no digits at all
	}
	for _, src := range bad {
		if _, err := Tokenize(src); err == nil {
			t.Errorf("Tokenize(%q) should be a lex error", src)
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

// TestTokenizeIdentifierDigits covers the digit relaxation: identifiers are
// letter-initial and may carry interior / trailing digits (`sha256`, `x2`,
// `HTTP2`, `SCRAM_SHA256`), while a token that starts with a digit is still a
// number, so the lexer stays unambiguous.
func TestTokenizeIdentifierDigits(t *testing.T) {
	for _, src := range []string{"sha256", "x2", "SHA256", "HTTP2", "iPv4", "md5", "SCRAM_SHA256"} {
		toks, err := Tokenize(src)
		if err != nil {
			t.Errorf("%q: unexpected lex error: %v", src, err)
			continue
		}
		if len(toks) < 1 || toks[0].Type != TOKEN_IDENT || toks[0].Lexeme != src {
			t.Errorf("%q: expected single IDENT lexeme, got %+v", src, toks)
		}
	}
	// A digit-initial token is a number, not an identifier: `2x` is `2` then `x`.
	toks, err := Tokenize("2x")
	if err != nil {
		t.Fatalf("2x: unexpected lex error: %v", err)
	}
	if len(toks) < 2 || toks[0].Type != TOKEN_INT || toks[1].Type != TOKEN_IDENT || toks[1].Lexeme != "x" {
		t.Errorf("2x: expected [INT, IDENT x], got %+v", toks)
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

// Scientific-notation float literals: an `[eE][+-]?digits` exponent makes the
// literal a float even with no fractional part. The exponent keeps its source
// spelling (strconv.ParseFloat accepts either case); only the mantissa's `_`
// separators are stripped. `Raw` always preserves the exact source.
func TestTokenizeScientificFloats(t *testing.T) {
	cases := []struct {
		src    string
		want   TokenType
		lexeme string
		raw    string
	}{
		{"1e10", TOKEN_FLOAT, "1e10", "1e10"},
		{"6.022e23", TOKEN_FLOAT, "6.022e23", "6.022e23"},
		{"1.6e-19", TOKEN_FLOAT, "1.6e-19", "1.6e-19"},
		{"2.5E8", TOKEN_FLOAT, "2.5E8", "2.5E8"},            // uppercase E kept (ParseFloat accepts it)
		{"1e+5", TOKEN_FLOAT, "1e+5", "1e+5"},               // explicit + sign kept
		{"1_000.5e3", TOKEN_FLOAT, "1000.5e3", "1_000.5e3"}, // mantissa separators stripped, raw kept
		{"0e0", TOKEN_FLOAT, "0e0", "0e0"},
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
		if toks[0].Raw != c.raw {
			t.Errorf("Tokenize(%q): Raw = %q, want %q (fmt round-trip)", c.src, toks[0].Raw, c.raw)
		}
	}
	// `0xe5`: the `e` is a hex digit, never an exponent - stays an INT.
	if toks, err := Tokenize("0xe5"); err != nil || toks[0].Type != TOKEN_INT || toks[0].Lexeme != "0xe5" {
		t.Errorf("Tokenize(0xe5): got %v (%v), want INT 0xe5", toks[0], err)
	}
	// A missing exponent digit is a positioned lex error, not a silent split.
	for _, bad := range []string{"1e", "1e+", "1.5e-"} {
		if _, err := Tokenize(bad); err == nil {
			t.Errorf("Tokenize(%q): expected a lex error for a digitless exponent", bad)
		}
	}
	// A huge exponent must scan in linear time: the exponent is sliced from the
	// source in one shot, not grown character by character (which would be
	// O(n^2) and let a large literal wedge the lexer). `1e<20000 zeros>` is a
	// valid literal (== 1.0); it must tokenize to a single FLOAT, fast.
	big := "1e" + strings.Repeat("0", 20000)
	toks, err := Tokenize(big)
	if err != nil {
		t.Fatalf("Tokenize(1e<20000 zeros>): unexpected error %v", err)
	}
	if toks[0].Type != TOKEN_FLOAT || len(toks[0].Lexeme) != len(big) {
		t.Errorf("Tokenize(1e<20000 zeros>): got %s len %d, want FLOAT len %d", toks[0].Type, len(toks[0].Lexeme), len(big))
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

// TestTokenizeMatchKeywords covers the `match` / `when` keywords (M22.4). `else`
// is shared with `if`; it already has coverage above.
func TestTokenizeMatchKeywords(t *testing.T) {
	toks, err := Tokenize("match when else")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []TokenType{TOKEN_MATCH, TOKEN_WHEN, TOKEN_ELSE, TOKEN_EOF}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(toks), len(want), toks)
	}
	for i, w := range want {
		if toks[i].Type != w {
			t.Errorf("tok %d: got %s (%q), want %s", i, toks[i].Type, toks[i].Lexeme, w)
		}
	}
}
