// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package preproc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/lexer"
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
	src := `use io; func app() { io.printf(1); }`
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

func TestFileIncludeSplices(t *testing.T) {
	dir := writeTmp(t, map[string]string{
		"helpers.j": `def bonus as int init 7;`,
		"main.j":    `func app() { include "helpers.j"; io.printf($bonus); }`,
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
	// We expect: func app ( ) { def bonus as int init 7 ; io.printf ( $bonus ) ; } EOF
	// (`io.printf` is `IDENT DOT IDENT` after the namespace migration.)
	wantTypes := []lexer.TokenType{
		lexer.TOKEN_FUNC, lexer.TOKEN_IDENT, lexer.TOKEN_LPAREN, lexer.TOKEN_RPAREN, lexer.TOKEN_LBRACE,
		lexer.TOKEN_DEFINE, lexer.TOKEN_IDENT, lexer.TOKEN_AS, lexer.TOKEN_INT_TYPE, lexer.TOKEN_INIT, lexer.TOKEN_INT, lexer.TOKEN_SEMI,
		lexer.TOKEN_IDENT, lexer.TOKEN_DOT, lexer.TOKEN_IDENT, lexer.TOKEN_LPAREN, lexer.TOKEN_VARREF, lexer.TOKEN_RPAREN, lexer.TOKEN_SEMI,
		lexer.TOKEN_RBRACE, lexer.TOKEN_EOF,
	}
	if !typesEqual(tokenTypes(out), wantTypes) {
		t.Errorf("after splice, types:\n got: %v\nwant: %v", tokenTypes(out), wantTypes)
	}
}

// Trivia (comments, blank lines) inside an INCLUDED file must be stripped the
// same as in the top-level file, so a nested `include /* note */ "x.j";` (or a
// comment before a statement) does not break the include recognizer one level
// deep.
func TestIncludedFileTriviaStripped(t *testing.T) {
	dir := writeTmp(t, map[string]string{
		"leaf.j": `def leaf as int init 3;`,
		"mid.j": `# a comment before an include
		include /* pinned */ "leaf.j";
		def mid as int init 4;`,
		"main.j": `func app() { include "mid.j"; io.printf($leaf); io.printf($mid); }`,
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
	// No trivia tokens should survive in the spliced output.
	for _, tk := range out {
		switch tk.Type {
		case lexer.TOKEN_COMMENT_LINE, lexer.TOKEN_COMMENT_BLOCK, lexer.TOKEN_BLANK_LINE:
			t.Errorf("trivia token %v survived the splice", tk.Type)
		}
	}
}

func TestNestedFileImports(t *testing.T) {
	dir := writeTmp(t, map[string]string{
		"a.j":    `def a as int init 1;`,
		"b.j":    `include "a.j"; def b as int init 2;`,
		"main.j": `func app() { include "b.j"; io.printf($a + $b); }`,
	})
	mainPath := filepath.Join(dir, "main.j")
	src, _ := os.ReadFile(mainPath)
	toks, _ := lexer.TokenizeWithFile(string(src), mainPath)
	out, err := Process(toks, dir, mainPath)
	if err != nil {
		t.Fatalf("preproc: %v", err)
	}
	// Two DEFINE tokens (one each from a.j and b.j) and one FUNC (for app).
	defineCount := 0
	funcCount := 0
	varrefs := []string{}
	for _, tk := range out {
		switch tk.Type {
		case lexer.TOKEN_DEFINE:
			defineCount++
		case lexer.TOKEN_FUNC:
			funcCount++
		case lexer.TOKEN_VARREF:
			varrefs = append(varrefs, tk.Lexeme)
		}
	}
	if defineCount != 2 {
		t.Errorf("got %d DEFINE tokens, want 2", defineCount)
	}
	if funcCount != 1 {
		t.Errorf("got %d FUNC tokens, want 1", funcCount)
	}
	// Only the use sites carry $; defs use bare IDENT.
	wantVarrefs := []string{"a", "b"}
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
		"a.j":    `include "b.j"; def a as int init 1;`,
		"b.j":    `include "a.j"; def b as int init 2;`,
		"main.j": `func app() { include "a.j"; }`,
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
	src := `func app() { include "foo.go"; }`
	toks, _ := lexer.Tokenize(src)
	_, err := Process(toks, ".", "")
	if err == nil {
		t.Fatal("expected error for non-.j import, got nil")
	}
	if !strings.Contains(err.Error(), ".j") {
		t.Errorf("error should mention .j: %v", err)
	}
}

func TestRejectsUnquotedFileSplice(t *testing.T) {
	src := `func app() { include foo.j; }`
	toks, _ := lexer.Tokenize(src)
	_, err := Process(toks, ".", "")
	if err == nil {
		t.Fatal("expected error for unquoted file splice")
	}
	if !strings.Contains(err.Error(), "string literal") {
		t.Errorf("error should suggest string literal: %v", err)
	}
}

func TestRejectsIncludeOfLibraryName(t *testing.T) {
	// `include foo;` (no quoted path) looks like a library import; the
	// preprocessor suggests `use foo;` instead.
	src := `include io; func app() {}`
	toks, _ := lexer.Tokenize(src)
	_, err := Process(toks, ".", "")
	if err == nil {
		t.Fatal("expected error for `include io;`")
	}
	if !strings.Contains(err.Error(), "use io") {
		t.Errorf("error should suggest `use io;`: %v", err)
	}
}

func TestModuleImportPassesThrough(t *testing.T) {
	// `import "x.j" [as NAME];` is a module import: passed through to the
	// parser unchanged (not spliced like `include`, not rejected).
	src := `import "foo.j" as foo; func app() {}`
	toks, _ := lexer.Tokenize(src)
	out, err := Process(toks, ".", "")
	if err != nil {
		t.Fatalf("module import should pass through, got: %v", err)
	}
	if out[0].Type != lexer.TOKEN_IMPORT {
		t.Errorf("import token not preserved: %v", out[0])
	}
}

func TestUnquotedImportRejected(t *testing.T) {
	// `import foo;` (unquoted) is the common mistake - a module path must be
	// a quoted string; a system library uses `use`.
	src := `import foo; func app() {}`
	toks, _ := lexer.Tokenize(src)
	_, err := Process(toks, ".", "")
	if err == nil {
		t.Fatal("expected an error for unquoted `import foo;`")
	}
	if !strings.Contains(err.Error(), "quoted") {
		t.Errorf("error should mention quoting: %v", err)
	}
}

func TestRejectsUseForFile(t *testing.T) {
	src := `use foo.j; func app() {}`
	toks, _ := lexer.Tokenize(src)
	_, err := Process(toks, ".", "")
	if err == nil {
		t.Fatal("expected error for `use foo.j;`")
	}
	if !strings.Contains(err.Error(), `include "foo.j"`) {
		t.Errorf("error should suggest `include \"foo.j\";`: %v", err)
	}
}

func TestMissingFile(t *testing.T) {
	dir := t.TempDir()
	src := `func app() { include "nope.j"; }`
	toks, _ := lexer.Tokenize(src)
	_, err := Process(toks, dir, "")
	if err == nil {
		t.Fatal("expected missing-file error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot read") {
		t.Errorf("error should mention cannot read: %v", err)
	}
}

func TestIncludeAtTopLevel(t *testing.T) {
	dir := writeTmp(t, map[string]string{
		"top.j":  `func helper() { io.printf(1); }`,
		"main.j": `use io; include "top.j"; func app() { helper(); }`,
	})
	mainPath := filepath.Join(dir, "main.j")
	src, _ := os.ReadFile(mainPath)
	toks, _ := lexer.TokenizeWithFile(string(src), mainPath)
	out, err := Process(toks, dir, mainPath)
	if err != nil {
		t.Fatalf("preproc: %v", err)
	}
	// `use io;` is preserved; the file import is spliced and contributes a method def.
	if out[0].Type != lexer.TOKEN_USE || out[1].Lexeme != "io" {
		t.Errorf("first import not preserved: %v %v", out[0], out[1])
	}
}
