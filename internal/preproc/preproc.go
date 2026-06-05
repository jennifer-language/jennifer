// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// Package preproc handles Jennifer's file-import preprocessor.
//
// A file import has the form `import "name.j";` and is replaced, at the
// location it appears, by the tokens of the referenced file. The path is
// resolved relative to the directory of the file that contains the import.
// File imports are processed recursively, with a cycle check to prevent
// infinite inclusion.
//
// Library imports use the `use` keyword (e.g. `use io;`) and are left in
// place; the parser turns them into ImportStmt nodes.
package preproc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mplx/jennifer-lang/internal/lexer"
)

// PreprocessError carries context across files.
type PreprocessError struct {
	Msg  string
	File string
	Line int
	Col  int
}

func (e *PreprocessError) Error() string {
	if e.File == "" {
		return fmt.Sprintf("preprocess error at %d:%d: %s", e.Line, e.Col, e.Msg)
	}
	return fmt.Sprintf("preprocess error at %s:%d:%d: %s", e.File, e.Line, e.Col, e.Msg)
}

// Position implements the positioned-error interface used by the CLI.
func (e *PreprocessError) Position() (file string, line, col int) {
	return e.File, e.Line, e.Col
}

// Process expands all file imports in `tokens`.
// `baseDir` is the directory used to resolve relative `.j` filenames.
// `selfPath`, if non-empty, is the absolute path of the file that produced
// `tokens`; it is added to the visited set so a file can't import itself
// transitively.
func Process(tokens []lexer.Token, baseDir, selfPath string) ([]lexer.Token, error) {
	visited := map[string]bool{}
	if selfPath != "" {
		abs, err := filepath.Abs(selfPath)
		if err == nil {
			visited[abs] = true
		}
	}
	return processTokens(tokens, baseDir, visited)
}

func processTokens(tokens []lexer.Token, baseDir string, visited map[string]bool) ([]lexer.Token, error) {
	out := make([]lexer.Token, 0, len(tokens))
	i := 0
	for i < len(tokens) {
		tok := tokens[i]

		// `import "path/file.j";` - file import
		if tok.Type == lexer.TOKEN_IMPORT {
			expanded, advance, err := handleImport(tokens, i, baseDir, visited)
			if err != nil {
				return nil, err
			}
			out = append(out, expanded...)
			i += advance
			continue
		}

		// `use NAME ;` - library import. Check for a common mistake
		// (`use file.j;`) and produce a helpful error.
		if tok.Type == lexer.TOKEN_USE {
			if err := validateUse(tokens, i); err != nil {
				return nil, err
			}
			// pass through unchanged
			out = append(out, tok)
			i++
			continue
		}

		out = append(out, tok)
		i++
	}
	return out, nil
}

// handleImport processes an `import` token. Possible shapes:
//
//	import "path.j" ;     (canonical file import)
//	import NAME ;          (old library form - error: use `use NAME;`)
//	import NAME.j ;        (old file form - error: quote the path)
//
// Returns the spliced tokens (empty if it was a pass-through) and the number
// of input tokens consumed.
func handleImport(tokens []lexer.Token, i int, baseDir string, visited map[string]bool) ([]lexer.Token, int, error) {
	imp := tokens[i]
	next := tokens[i+1]

	// `import "path.j" ;`
	if next.Type == lexer.TOKEN_STRING && i+2 < len(tokens) && tokens[i+2].Type == lexer.TOKEN_SEMI {
		path := next.Lexeme
		if !strings.HasSuffix(path, ".j") {
			return nil, 0, &PreprocessError{
				Msg:  fmt.Sprintf("import path %q must end with `.j`", path),
				File: next.File, Line: next.Line, Col: next.Col,
			}
		}
		spliced, err := spliceFile(path, baseDir, visited, next)
		if err != nil {
			return nil, 0, err
		}
		return spliced, 3, nil // import STRING ;
	}

	// `import NAME ;` - looks like the old library form
	if next.Type == lexer.TOKEN_IDENT && i+2 < len(tokens) && tokens[i+2].Type == lexer.TOKEN_SEMI {
		return nil, 0, &PreprocessError{
			Msg:  fmt.Sprintf("`import` is for files; use `use %s;` for system libraries", next.Lexeme),
			File: imp.File, Line: imp.Line, Col: imp.Col,
		}
	}

	// `import NAME.j ;` - the old unquoted file form
	if next.Type == lexer.TOKEN_IDENT && i+4 < len(tokens) &&
		tokens[i+2].Type == lexer.TOKEN_DOT &&
		tokens[i+3].Type == lexer.TOKEN_IDENT &&
		tokens[i+4].Type == lexer.TOKEN_SEMI {
		return nil, 0, &PreprocessError{
			Msg:  fmt.Sprintf("file imports take a string literal: `import \"%s.%s\";`", next.Lexeme, tokens[i+3].Lexeme),
			File: imp.File, Line: imp.Line, Col: imp.Col,
		}
	}

	return nil, 0, &PreprocessError{
		Msg:  "expected `import \"path.j\";`",
		File: imp.File, Line: imp.Line, Col: imp.Col,
	}
}

func spliceFile(path, baseDir string, visited map[string]bool, originTok lexer.Token) ([]lexer.Token, error) {
	fullPath := path
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(baseDir, path)
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return nil, &PreprocessError{Msg: err.Error(), File: originTok.File, Line: originTok.Line, Col: originTok.Col}
	}
	if visited[absPath] {
		return nil, &PreprocessError{
			Msg:  fmt.Sprintf("circular import: %s is already being included", absPath),
			File: originTok.File, Line: originTok.Line, Col: originTok.Col,
		}
	}
	srcBytes, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, &PreprocessError{
			Msg:  fmt.Sprintf("cannot read imported file %q: %v", fullPath, err),
			File: originTok.File, Line: originTok.Line, Col: originTok.Col,
		}
	}
	incToks, err := lexer.TokenizeWithFile(string(srcBytes), fullPath)
	if err != nil {
		return nil, err
	}
	childVisited := copyVisited(visited)
	childVisited[absPath] = true
	expanded, err := processTokens(incToks, filepath.Dir(fullPath), childVisited)
	if err != nil {
		return nil, err
	}
	// drop trailing EOF
	out := make([]lexer.Token, 0, len(expanded))
	for _, t := range expanded {
		if t.Type == lexer.TOKEN_EOF {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// validateUse catches the common mistake `use foo.j;` (using `use` for a file).
// `use NAME;` is fine and passes through unchanged.
func validateUse(tokens []lexer.Token, i int) error {
	if i+4 >= len(tokens) {
		return nil
	}
	if tokens[i+1].Type == lexer.TOKEN_IDENT &&
		tokens[i+2].Type == lexer.TOKEN_DOT &&
		tokens[i+3].Type == lexer.TOKEN_IDENT &&
		tokens[i+4].Type == lexer.TOKEN_SEMI {
		t := tokens[i]
		return &PreprocessError{
			Msg:  fmt.Sprintf("`use` is for system libraries; for files use `import \"%s.%s\";`", tokens[i+1].Lexeme, tokens[i+3].Lexeme),
			File: t.File, Line: t.Line, Col: t.Col,
		}
	}
	return nil
}

func copyVisited(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
