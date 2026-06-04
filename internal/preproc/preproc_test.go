// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package preproc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mplx/jennifer-lang/internal/lexer"
)

// tokenTypes returns the type slice for comparison.
func tokenTypes(toks []lexer.Token) []lexer.TokenType {
	out := make([]lexer.TokenType, len(toks))
	for i, t := range toks {
		out[i] = t.Type
	}
	return out
}

func typesEqual(a, b []lexer.TokenType) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// writeTmp returns the directory and creates `files` (name -> content) within it.
func writeTmp(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func TestPassesThroughLibraryImports(t *testing.T) {
	src := `import stdlib; def app() { printf(1); }`
	toks, err := lexer.Tokenize(src)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	out, err := Process(toks, "", "")
	if err != nil {
		t.Fatalf("preproc: %v", err)
	}
	if !typesEqual(tokenTypes(toks), tokenTypes(out)) {
		t.Errorf("library import was rewritten unexpectedly")
	}
}

func TestFileImportSplices(t *testing.T) {
	dir := writeTmp(t, map[string]string{
		"helpers.j": `define $bonus as int init 7;`,
		"main.j":    `def app() { import helpers.j; printf($bonus); }`,
	})
	mainPath := filepath.Join(dir, "main.j")
	src, _ := os.ReadFile(mainPath)
	toks, err := lexer.TokenizeWithFile(string(src), mainPath)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	out, err := Process(toks, dir, mainPath)
	if err != nil {
		t.Fatalf("preproc: %v", err)
	}
	// After splicing, no DOT tokens should remain (no file-import-shaped sequences left).
	for _, tk := range out {
		if tk.Type == lexer.TOKEN_DOT {
			t.Errorf("DOT token survived preprocessing: %v", tk)
		}
	}
	// We expect: def app ( ) { define $bonus as int init 7 ; printf ( $bonus ) ; } EOF
	wantTypes := []lexer.TokenType{
		lexer.TOKEN_DEFINE, lexer.TOKEN_IDENT, lexer.TOKEN_LPAREN, lexer.TOKEN_RPAREN, lexer.TOKEN_LBRACE,
		lexer.TOKEN_DEFINE, lexer.TOKEN_VARREF, lexer.TOKEN_AS, lexer.TOKEN_INT_TYPE, lexer.TOKEN_INIT, lexer.TOKEN_INT, lexer.TOKEN_SEMI,
		lexer.TOKEN_IDENT, lexer.TOKEN_LPAREN, lexer.TOKEN_VARREF, lexer.TOKEN_RPAREN, lexer.TOKEN_SEMI,
		lexer.TOKEN_RBRACE, lexer.TOKEN_EOF,
	}
	if !typesEqual(tokenTypes(out), wantTypes) {
		t.Errorf("after splice, types:\n got: %v\nwant: %v", tokenTypes(out), wantTypes)
	}
}

func TestNestedFileImports(t *testing.T) {
	dir := writeTmp(t, map[string]string{
		"a.j":    `define $a as int init 1;`,
		"b.j":    `import a.j; define $b as int init 2;`,
		"main.j": `def app() { import b.j; printf($a + $b); }`,
	})
	mainPath := filepath.Join(dir, "main.j")
	src, _ := os.ReadFile(mainPath)
	toks, _ := lexer.TokenizeWithFile(string(src), mainPath)
	out, err := Process(toks, dir, mainPath)
	if err != nil {
		t.Fatalf("preproc: %v", err)
	}
	// Should have two `define` statements for $a and $b spliced in.
	defineCount := 0
	varrefs := []string{}
	for _, tk := range out {
		if tk.Type == lexer.TOKEN_DEFINE {
			defineCount++
		}
		if tk.Type == lexer.TOKEN_VARREF {
			varrefs = append(varrefs, tk.Lexeme)
		}
	}
	// 3 defines: app (method), $a, $b
	if defineCount != 3 {
		t.Errorf("got %d DEFINE tokens, want 3", defineCount)
	}
	// varrefs should contain a, b, a, b (define $a, define $b, $a, $b)
	wantVarrefs := []string{"a", "b", "a", "b"}
	if len(varrefs) != len(wantVarrefs) {
		t.Fatalf("got varrefs %v, want %v", varrefs, wantVarrefs)
	}
	for i := range varrefs {
		if varrefs[i] != wantVarrefs[i] {
			t.Errorf("varref %d: got %q, want %q", i, varrefs[i], wantVarrefs[i])
		}
	}
}

func TestDetectsCircularImport(t *testing.T) {
	dir := writeTmp(t, map[string]string{
		"a.j":    `import b.j; define $a as int init 1;`,
		"b.j":    `import a.j; define $b as int init 2;`,
		"main.j": `def app() { import a.j; }`,
	})
	mainPath := filepath.Join(dir, "main.j")
	src, _ := os.ReadFile(mainPath)
	toks, _ := lexer.TokenizeWithFile(string(src), mainPath)
	_, err := Process(toks, dir, mainPath)
	if err == nil {
		t.Fatal("expected circular-import error, got nil")
	}
	if !strings.Contains(err.Error(), "circular import") {
		t.Errorf("error doesn't mention circular import: %v", err)
	}
}

func TestRejectsNonJExtension(t *testing.T) {
	src := `def app() { import foo.go; }`
	toks, _ := lexer.Tokenize(src)
	_, err := Process(toks, ".", "")
	if err == nil {
		t.Fatal("expected error for non-.j import, got nil")
	}
	if !strings.Contains(err.Error(), ".j") {
		t.Errorf("error should mention .j: %v", err)
	}
}

func TestMissingFile(t *testing.T) {
	dir := t.TempDir()
	src := `def app() { import nope.j; }`
	toks, _ := lexer.Tokenize(src)
	_, err := Process(toks, dir, "")
	if err == nil {
		t.Fatal("expected missing-file error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot read") {
		t.Errorf("error should mention cannot read: %v", err)
	}
}

func TestImportAtTopLevel(t *testing.T) {
	dir := writeTmp(t, map[string]string{
		"top.j":  `def helper() { printf(1); }`,
		"main.j": `import stdlib; import top.j; def app() { helper(); }`,
	})
	mainPath := filepath.Join(dir, "main.j")
	src, _ := os.ReadFile(mainPath)
	toks, _ := lexer.TokenizeWithFile(string(src), mainPath)
	out, err := Process(toks, dir, mainPath)
	if err != nil {
		t.Fatalf("preproc: %v", err)
	}
	// First import (stdlib) remains; second import is spliced and contributes a method def.
	if out[0].Type != lexer.TOKEN_IMPORT || out[1].Lexeme != "stdlib" {
		t.Errorf("first import not preserved: %v %v", out[0], out[1])
	}
}
