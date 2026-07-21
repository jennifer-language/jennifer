// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"jennifer-lang.dev/jennifer/internal/lexer"
	"jennifer-lang.dev/jennifer/internal/parser"
	"jennifer-lang.dev/jennifer/internal/preproc"
)

// printDevUsage lists the development subcommands in the top-level usage
// text. Build-tag split (paired with the stub in devtools_tinygo.go) so the
// constrained TinyGo build, which omits these subcommands, doesn't advertise
// commands it will only reject.
func printDevUsage(w io.Writer) {
	fmt.Fprintln(w, "  jennifer tokens <file>   dump the lexer's token stream")
	fmt.Fprintln(w, "  jennifer ast <file>      dump the parsed AST as JSON")
	fmt.Fprintln(w, "  jennifer fmt <file>      format the source per docs/user-guide/style-guide.md")
	fmt.Fprintln(w, "  jennifer lint <file>...  report compile-legal but suspect patterns")
	fmt.Fprintln(w, "  jennifer profile <file>  run with the evaluator instrumented; write a profile")
	fmt.Fprintln(w, "  jennifer test <file>     discover and run the file's test methods")
	fmt.Fprintln(w, "  jennifer serve <file>    run a web app (--watch reloads on change)")
}

// loadProgramSource opens a Jennifer source from `path` (or stdin if path is
// "-"), returning the source text, the absolute path (or "<stdin>"), and the
// base directory used to resolve relative file imports. Errors are written to
// stderr; on failure the caller exits non-zero.
func loadProgramSource(path string) (src, label, absPath, baseDir string, ok bool) {
	if path == "-" {
		bytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "jennifer: reading stdin: %v\n", err)
			return "", "", "", "", false
		}
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "jennifer: %v\n", err)
			return "", "", "", "", false
		}
		return string(bytes), "<stdin>", "<stdin>", cwd, true
	}
	if filepath.Ext(path) != ".j" {
		fmt.Fprintf(os.Stderr, "jennifer: source file must have .j extension, got %q\n", path)
		return "", "", "", "", false
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jennifer: %v\n", err)
		return "", "", "", "", false
	}
	abs, _ := filepath.Abs(path)
	return string(bytes), path, abs, filepath.Dir(abs), true
}

// dumpTokens prints the lexer's output for path, one token per line, in a
// column-aligned `LINE:COL TYPE [lexeme]` format. Imports are not expanded -
// the user sees the raw stream the lexer produced from the file as written.
func dumpTokens(path string) int {
	src, label, absPath, _, ok := loadProgramSource(path)
	if !ok {
		return 1
	}
	tokens, err := lexer.TokenizeWithFile(src, absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return 1
	}
	// Compute column widths so the output aligns even with multi-line files
	// whose later tokens have larger line/col numbers.
	maxPos := 0
	maxType := 0
	for _, t := range tokens {
		p := fmt.Sprintf("%d:%d", t.Line, t.Col)
		if len(p) > maxPos {
			maxPos = len(p)
		}
		if n := len(t.Type.String()); n > maxType {
			maxType = n
		}
	}
	for _, t := range tokens {
		pos := fmt.Sprintf("%d:%d", t.Line, t.Col)
		// Show the lexeme when present and meaningful. EOF, punctuation, and
		// keyword tokens carry redundant or empty lexemes; the type name
		// alone is clearer. IDENT / VARREF / STRING / INT / FLOAT are the
		// kinds where the payload matters.
		switch t.Type {
		case lexer.TOKEN_IDENT, lexer.TOKEN_VARREF,
			lexer.TOKEN_STRING, lexer.TOKEN_INT, lexer.TOKEN_FLOAT:
			fmt.Printf("%-*s  %-*s  %q\n", maxPos, pos, maxType, t.Type.String(), t.Lexeme)
		default:
			fmt.Printf("%-*s  %s\n", maxPos, pos, t.Type.String())
		}
	}
	return 0
}

// dumpAST prints the parsed (post-preprocessing) AST as JSON with two-space
// indentation. The emitter is hand-rolled rather than using `encoding/json`
// for TinyGo safety: TinyGo's encoding/json relies on reflect, which we
// avoid everywhere else in this codebase for the same reason.
func dumpAST(path string) int {
	src, label, absPath, baseDir, ok := loadProgramSource(path)
	if !ok {
		return 1
	}
	tokens, err := lexer.TokenizeWithFile(src, absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return 1
	}
	tokens, err = preproc.Process(tokens, baseDir, absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return 1
	}
	prog, err := parser.ParseTokens(tokens)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return 1
	}
	var b strings.Builder
	emitNode(&b, prog, 0)
	b.WriteByte('\n')
	io.WriteString(os.Stdout, b.String())
	return 0
}
