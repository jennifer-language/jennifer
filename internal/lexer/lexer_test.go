// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package lexer

import "testing"

func TestTokenizeSimpleProgram(t *testing.T) {
	src := `use io;
func app() {
    def x as int init 21;
    printf($x + $x);
}`
	toks, err := Tokenize(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []TokenType{
		TOKEN_USE, TOKEN_IDENT, TOKEN_SEMI,
		TOKEN_FUNC, TOKEN_IDENT, TOKEN_LPAREN, TOKEN_RPAREN, TOKEN_LBRACE,
		TOKEN_DEFINE, TOKEN_IDENT, TOKEN_AS, TOKEN_INT_TYPE, TOKEN_INIT, TOKEN_INT, TOKEN_SEMI,
		TOKEN_IDENT, TOKEN_LPAREN, TOKEN_VARREF, TOKEN_PLUS, TOKEN_VARREF, TOKEN_RPAREN, TOKEN_SEMI,
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
	src := `// line comment
import /* block */ stdlib; // trailing
/* multi
   line */
def`
	toks, err := Tokenize(src)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	want := []TokenType{TOKEN_IMPORT, TOKEN_IDENT, TOKEN_SEMI, TOKEN_DEFINE, TOKEN_EOF}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(toks), len(want), toks)
	}
	for i, w := range want {
		if toks[i].Type != w {
			t.Errorf("tok %d: got %s, want %s", i, toks[i].Type, w)
		}
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
	toks, err := Tokenize("< > <= >= == =")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []TokenType{TOKEN_LT, TOKEN_GT, TOKEN_LE, TOKEN_GE, TOKEN_EQ, TOKEN_ASSIGN, TOKEN_EOF}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(toks), len(want), toks)
	}
	for i, w := range want {
		if toks[i].Type != w {
			t.Errorf("tok %d: got %s, want %s", i, toks[i].Type, w)
		}
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
