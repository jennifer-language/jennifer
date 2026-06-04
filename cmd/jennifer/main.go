// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/lexer"
	"github.com/mplx/jennifer-lang/internal/parser"
	"github.com/mplx/jennifer-lang/internal/preproc"
	"github.com/mplx/jennifer-lang/internal/stdlib"
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
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  jennifer run <file.j>    run a Jennifer program")
	fmt.Fprintln(os.Stderr, "  jennifer help            show this message")
}

func runFile(path string) int {
	if filepath.Ext(path) != ".j" {
		fmt.Fprintf(os.Stderr, "jennifer: source file must have .j extension, got %q\n", path)
		return 2
	}
	srcBytes, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jennifer: %v\n", err)
		return 1
	}
	src := string(srcBytes)
	absPath, _ := filepath.Abs(path)
	baseDir := filepath.Dir(absPath)
	tokens, err := lexer.TokenizeWithFile(src, absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", path, err.Error())
		printErrorContext(src, err)
		return 1
	}
	tokens, err = preproc.Process(tokens, baseDir, absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", path, err.Error())
		return 1
	}
	prog, err := parser.ParseTokens(tokens)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", path, err.Error())
		printErrorContext(src, err)
		return 1
	}
	in := interpreter.New()
	stdlib.Install(in)
	if err := in.Run(prog); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", path, err.Error())
		printErrorContext(src, err)
		return 1
	}
	return 0
}

// printErrorContext shows the offending source line with a caret when the error
// carries a line:col. Best-effort: it tolerates errors without positions.
func printErrorContext(src string, err error) {
	line, col, ok := extractPos(err)
	if !ok {
		return
	}
	lines := strings.Split(src, "\n")
	if line < 1 || line > len(lines) {
		return
	}
	fmt.Fprintf(os.Stderr, "  %s\n", lines[line-1])
	if col > 0 {
		fmt.Fprintf(os.Stderr, "  %s^\n", strings.Repeat(" ", col-1))
	}
}

// extractPos parses our error messages of the form "... at LINE:COL: ..." back into ints.
// Cheaper and simpler than threading a positioned-error interface through every layer for M1.
func extractPos(err error) (int, int, bool) {
	if err == nil {
		return 0, 0, false
	}
	msg := err.Error()
	i := strings.Index(msg, " at ")
	if i < 0 {
		return 0, 0, false
	}
	rest := msg[i+4:]
	colonEnd := strings.Index(rest, ":")
	if colonEnd < 0 {
		return 0, 0, false
	}
	colonEnd2 := strings.Index(rest[colonEnd+1:], ":")
	if colonEnd2 < 0 {
		return 0, 0, false
	}
	lineStr := rest[:colonEnd]
	colStr := rest[colonEnd+1 : colonEnd+1+colonEnd2]
	var line, col int
	if _, err := fmt.Sscanf(lineStr, "%d", &line); err != nil {
		return 0, 0, false
	}
	if _, err := fmt.Sscanf(colStr, "%d", &col); err != nil {
		return 0, 0, false
	}
	return line, col, true
}
