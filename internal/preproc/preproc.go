// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// Package preproc handles Jennifer's file-import preprocessor.
//
// A file import has the form `import name.j;` and is replaced, at the location it
// appears, by the tokens of the referenced file. The referenced file's path is
// resolved relative to the directory of the file that contains the import. File
// imports are processed recursively, with a cycle check to prevent infinite
// inclusion.
//
// Library imports (e.g. `import stdlib;`) are left in place and remain as
// ImportStmt nodes for the interpreter to handle.
package preproc

import (
	"fmt"
	"os"
	"path/filepath"

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

// Process expands all file imports in `tokens`.
// `baseDir` is the directory used to resolve relative `.j` filenames.
// `selfPath`, if non-empty, is the absolute path of the file that produced `tokens`;
// it is added to the visited set so a file can't import itself transitively.
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
		// Look for `import IDENT . IDENT(=j) ;` - a file import.
		// Anything else (including `import IDENT ;` - a library import) is left alone.
		if isFileImportPattern(tokens, i) {
			ident := tokens[i+1] // the file's base name (without .j)
			suffix := tokens[i+3]
			if suffix.Lexeme != "j" {
				return nil, &PreprocessError{
					Msg:  fmt.Sprintf("file imports must reference a .j file, got `.%s`", suffix.Lexeme),
					File: suffix.File, Line: suffix.Line, Col: suffix.Col,
				}
			}
			// Resolve the path. The bare name "name.j" is taken relative to baseDir.
			filename := ident.Lexeme + ".j"
			fullPath := filepath.Join(baseDir, filename)
			absPath, err := filepath.Abs(fullPath)
			if err != nil {
				return nil, &PreprocessError{Msg: err.Error(), File: ident.File, Line: ident.Line, Col: ident.Col}
			}
			if visited[absPath] {
				return nil, &PreprocessError{
					Msg:  fmt.Sprintf("circular import: %s is already being included", absPath),
					File: ident.File, Line: ident.Line, Col: ident.Col,
				}
			}
			srcBytes, err := os.ReadFile(fullPath)
			if err != nil {
				return nil, &PreprocessError{
					Msg:  fmt.Sprintf("cannot read imported file %q: %v", fullPath, err),
					File: ident.File, Line: ident.Line, Col: ident.Col,
				}
			}
			incToks, err := lexer.TokenizeWithFile(string(srcBytes), fullPath)
			if err != nil {
				return nil, err
			}
			// Recurse with a new baseDir = dir of the included file, and mark this file as visited.
			childVisited := copyVisited(visited)
			childVisited[absPath] = true
			expanded, err := processTokens(incToks, filepath.Dir(fullPath), childVisited)
			if err != nil {
				return nil, err
			}
			// Splice expanded tokens (drop trailing EOF).
			for _, t := range expanded {
				if t.Type == lexer.TOKEN_EOF {
					continue
				}
				out = append(out, t)
			}
			i += 5
			continue
		}
		out = append(out, tokens[i])
		i++
	}
	return out, nil
}

func isFileImportPattern(toks []lexer.Token, i int) bool {
	if i+4 >= len(toks) {
		return false
	}
	return toks[i].Type == lexer.TOKEN_IMPORT &&
		toks[i+1].Type == lexer.TOKEN_IDENT &&
		toks[i+2].Type == lexer.TOKEN_DOT &&
		toks[i+3].Type == lexer.TOKEN_IDENT &&
		toks[i+4].Type == lexer.TOKEN_SEMI
}

func copyVisited(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
