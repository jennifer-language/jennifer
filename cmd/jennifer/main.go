// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/lexer"
	"github.com/mplx/jennifer-lang/internal/lib/convert"
	"github.com/mplx/jennifer-lang/internal/lib/io"
	"github.com/mplx/jennifer-lang/internal/lib/math"
	"github.com/mplx/jennifer-lang/internal/lib/core"
	"github.com/mplx/jennifer-lang/internal/lib/strings"
	"github.com/mplx/jennifer-lang/internal/parser"
	"github.com/mplx/jennifer-lang/internal/preproc"
	"github.com/mplx/jennifer-lang/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		if len(os.Args) != 3 {
			usage()
			os.Exit(2)
		}
		os.Exit(runFile(os.Args[2]))
	case "repl":
		if len(os.Args) != 2 {
			usage()
			os.Exit(2)
		}
		os.Exit(runRepl())
	case "tokens":
		if len(os.Args) != 3 {
			usage()
			os.Exit(2)
		}
		os.Exit(dumpTokens(os.Args[2]))
	case "ast":
		if len(os.Args) != 3 {
			usage()
			os.Exit(2)
		}
		os.Exit(dumpAST(os.Args[2]))
	case "fmt":
		if len(os.Args) != 3 {
			usage()
			os.Exit(2)
		}
		os.Exit(runFmt(os.Args[2]))
	case "version", "--version", "-v":
		fmt.Println(version.Version)
		os.Exit(0)
	case "-h", "--help", "help":
		usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "jennifer: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

// License info displayed by `jennifer help` and on usage errors.
// Keep in sync with the SPDX headers at the top of each source file.
const (
	licenseID   = "LGPL-3.0-only"
	copyright   = "Copyright (C) 2026 <developer@mplx.eu>"
	description = "jennifer — Jennifer programming language interpreter"
)

func usage() {
	fmt.Fprintln(os.Stderr, description)
	fmt.Fprintln(os.Stderr, copyright)
	fmt.Fprintln(os.Stderr, "License: "+licenseID)
	fmt.Fprintln(os.Stderr, "Version: "+version.Version)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  jennifer run <file.j>    run a Jennifer program")
	fmt.Fprintln(os.Stderr, "  jennifer run -           read source from stdin")
	fmt.Fprintln(os.Stderr, "  jennifer repl            interactive REPL")
	fmt.Fprintln(os.Stderr, "  jennifer tokens <file>   dump the lexer's token stream")
	fmt.Fprintln(os.Stderr, "  jennifer ast <file>      dump the parsed AST as JSON")
	fmt.Fprintln(os.Stderr, "  jennifer fmt <file>      format the source per docs/stylespec.md")
	fmt.Fprintln(os.Stderr, "  jennifer version         print the version and exit")
	fmt.Fprintln(os.Stderr, "  jennifer help            show this message")
}

func runFile(path string) int {
	var (
		src     string
		label   string // path used in error messages
		absPath string // file tag for tokens (preproc cycle check)
		baseDir string // base for relative file imports
	)
	if path == "-" {
		bytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "jennifer: reading stdin: %v\n", err)
			return 1
		}
		src = string(bytes)
		label = "<stdin>"
		absPath = "<stdin>"
		// File imports from stdin resolve relative to the current working
		// directory - there's no source-file location to anchor against.
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "jennifer: %v\n", err)
			return 1
		}
		baseDir = cwd
	} else {
		if filepath.Ext(path) != ".j" {
			fmt.Fprintf(os.Stderr, "jennifer: source file must have .j extension, got %q\n", path)
			return 2
		}
		srcBytes, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "jennifer: %v\n", err)
			return 1
		}
		src = string(srcBytes)
		label = path
		abs, _ := filepath.Abs(path)
		absPath = abs
		baseDir = filepath.Dir(abs)
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
	in := interpreter.New()
	iolib.Install(in)
	convert.Install(in)
	mathlib.Install(in)
	stringslib.Install(in)
	corelib.Install(in)
	if err := in.Run(prog); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return 1
	}
	return 0
}

// positioned is the interface every Jennifer error type implements. It lets
// the CLI extract structured position info (file, line, col) without parsing
// the error's string form.
type positioned interface {
	Position() (file string, line, col int)
}

// printErrorContext shows the offending source line with a caret when the error
// carries a position. If the error originates in a different file than `src`
// (e.g. inside an imported `.j`), it loads that file and prints the snippet
// from there. Best-effort: silently does nothing if the file can't be read.
func printErrorContext(src, mainFile string, err error) {
	p, ok := err.(positioned)
	if !ok {
		return
	}
	file, line, col := p.Position()
	if line == 0 && col == 0 {
		return
	}

	// If the error names a different file, load it. Otherwise use src.
	source := src
	if file != "" && file != mainFile && file != "<stdin>" {
		bytes, ferr := os.ReadFile(file)
		if ferr != nil {
			// Can't load the imported file - skip the snippet.
			return
		}
		source = string(bytes)
	}

	lines := strings.Split(source, "\n")
	if line < 1 || line > len(lines) {
		return
	}
	srcLine := lines[line-1]
	fmt.Fprintf(os.Stderr, "  %s\n", srcLine)
	if col > 0 {
		fmt.Fprintf(os.Stderr, "  %s^\n", caretIndent(srcLine, col))
	}
}

// caretIndent builds the indent string that places a caret under column `col`
// (1-based, rune-counted - matching the lexer). Tabs in the source are
// replicated as tabs in the indent so the caret aligns regardless of the
// terminal's tab-stop width; other characters become single spaces.
func caretIndent(srcLine string, col int) string {
	var b strings.Builder
	i := 0
	for _, r := range srcLine {
		if i >= col-1 {
			break
		}
		if r == '\t' {
			b.WriteByte('\t')
		} else {
			b.WriteByte(' ')
		}
		i++
	}
	return b.String()
}
