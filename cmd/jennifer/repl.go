// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/lexer"
	"github.com/mplx/jennifer-lang/internal/lib/convert"
	iolib "github.com/mplx/jennifer-lang/internal/lib/io"
	mathlib "github.com/mplx/jennifer-lang/internal/lib/math"
	corelib "github.com/mplx/jennifer-lang/internal/lib/core"
	stringslib "github.com/mplx/jennifer-lang/internal/lib/strings"
	"github.com/mplx/jennifer-lang/internal/parser"
	"github.com/mplx/jennifer-lang/internal/preproc"
)

const (
	replPrompt     = ">>> "
	replContPrompt = "... "
	// fileTag is what we use as the synthetic file label for REPL input.
	// Errors raised inside the REPL use this so the cross-file snippet loader
	// in printErrorContext treats them as "no external file to load".
	replFileTag = "<repl>"
)

// runRepl drives the interactive read-eval-print loop. Returns a process exit
// code: 0 on clean exit (EOF or :quit), 1 on a fatal startup error.
func runRepl() int {
	in := interpreter.New()
	iolib.Install(in)
	convert.Install(in)
	mathlib.Install(in)
	stringslib.Install(in)
	corelib.Install(in)

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "jennifer: %v\n", err)
		return 1
	}

	stdin := bufio.NewReader(os.Stdin)
	fmt.Println(description)
	fmt.Println("type :quit (or Ctrl-D) to exit; :help for help")

	var buf strings.Builder
	for {
		prompt := replPrompt
		if buf.Len() > 0 {
			prompt = replContPrompt
		}
		fmt.Print(prompt)

		line, readErr := stdin.ReadString('\n')
		// Treat EOF as an exit signal when the buffer is empty; otherwise let
		// the current input be evaluated (the user finished without a newline).
		if readErr == io.EOF {
			if buf.Len() == 0 && line == "" {
				fmt.Println()
				return 0
			}
		} else if readErr != nil {
			fmt.Fprintf(os.Stderr, "\njennifer: read error: %v\n", readErr)
			return 1
		}

		// REPL directives only count on a fresh prompt (not mid-block).
		if buf.Len() == 0 {
			switch strings.TrimSpace(line) {
			case ":quit", ":exit":
				return 0
			case ":help":
				printReplHelp()
				continue
			case "":
				if readErr == io.EOF {
					return 0
				}
				continue
			}
		}

		buf.WriteString(line)

		// Try to lex what we have. A lex error is final - clear the buffer.
		tokens, lerr := lexer.TokenizeWithFile(buf.String(), replFileTag)
		if lerr != nil {
			fmt.Fprintf(os.Stderr, "%s\n", lerr.Error())
			printErrorContext(buf.String(), replFileTag, lerr)
			buf.Reset()
			continue
		}
		if !inputComplete(tokens) {
			if readErr == io.EOF {
				// User ended input mid-statement.
				fmt.Fprintln(os.Stderr, "jennifer: incomplete input at EOF")
				return 1
			}
			continue
		}

		src := buf.String()
		buf.Reset()

		tokens, perr := preproc.Process(tokens, cwd, replFileTag)
		if perr != nil {
			fmt.Fprintf(os.Stderr, "%s\n", perr.Error())
			printErrorContext(src, replFileTag, perr)
			continue
		}
		prog, parseErr := parser.ParseTokens(tokens)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "%s\n", parseErr.Error())
			printErrorContext(src, replFileTag, parseErr)
			continue
		}
		val, rerr := in.EvalInteractive(prog)
		if rerr != nil {
			fmt.Fprintf(os.Stderr, "%s\n", rerr.Error())
			printErrorContext(src, replFileTag, rerr)
			continue
		}
		printReplValue(val)

		if readErr == io.EOF {
			return 0
		}
	}
}

// inputComplete reports whether the accumulated tokens form a self-contained
// input the parser can consume. The REPL uses this to decide whether to
// prompt for another line.
//
// The rules:
//   - All `{` / `(` must be closed.
//   - The last non-EOF token must be `;` or `}` (the two statement
//     terminators), or there must be no real tokens at all (blank/comment-only
//     input is "complete" - evaluating it is a no-op).
//
// Unbalanced *closing* delimiters are deliberately left to the parser to
// diagnose, since they cannot be fixed by reading more input.
func inputComplete(tokens []lexer.Token) bool {
	depth := 0
	lastIdx := -1
	for i, t := range tokens {
		if t.Type == lexer.TOKEN_EOF {
			continue
		}
		lastIdx = i
		switch t.Type {
		case lexer.TOKEN_LBRACE, lexer.TOKEN_LPAREN:
			depth++
		case lexer.TOKEN_RBRACE, lexer.TOKEN_RPAREN:
			depth--
		}
	}
	if depth > 0 {
		return false
	}
	if lastIdx < 0 {
		return true
	}
	tt := tokens[lastIdx].Type
	return tt == lexer.TOKEN_SEMI || tt == lexer.TOKEN_RBRACE
}

// printReplValue echoes the result of a REPL expression. Null is suppressed
// (so void calls like `printf(...)` don't append `null` to their own output).
// Strings are shown with surrounding double quotes so the user can tell
// them apart from numbers.
func printReplValue(v interpreter.Value) {
	if v.Kind == interpreter.KindNull {
		return
	}
	if v.Kind == interpreter.KindString {
		fmt.Printf("%q\n", v.Str)
		return
	}
	fmt.Println(v.Display())
}

func printReplHelp() {
	fmt.Println("jennifer REPL")
	fmt.Println("  - statements end with ;")
	fmt.Println("  - unclosed braces continue on the next line")
	fmt.Println("  - a final bare expression prints its value")
	fmt.Println("  - :quit / :exit / Ctrl-D to exit")
}
