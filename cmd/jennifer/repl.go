// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/lexer"
	"github.com/mplx/jennifer-lang/internal/lib/convert"
	crclib "github.com/mplx/jennifer-lang/internal/lib/crc"
	hashlib "github.com/mplx/jennifer-lang/internal/lib/hash"
	iolib "github.com/mplx/jennifer-lang/internal/lib/io"
	listslib "github.com/mplx/jennifer-lang/internal/lib/lists"
	mapslib "github.com/mplx/jennifer-lang/internal/lib/maps"
	mathlib "github.com/mplx/jennifer-lang/internal/lib/math"
	metalib "github.com/mplx/jennifer-lang/internal/lib/meta"
	oslib "github.com/mplx/jennifer-lang/internal/lib/os"
	stringslib "github.com/mplx/jennifer-lang/internal/lib/strings"
	timelib "github.com/mplx/jennifer-lang/internal/lib/time"
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
//
// When stdin is a terminal we install raw mode and use the line editor
// (lineedit.go) so users get cursor keys, history, and the standard
// editing shortcuts. When stdin is not a terminal (piped input, tests,
// `jennifer repl < script.j`) we fall back to bufio line reading - the
// editor would do nothing useful on a non-interactive stream anyway.
func runRepl() int {
	in := interpreter.New()
	// Mark this interpreter as REPL-owned so stdin-consuming builtins
	// (`readLine`, `eof`) refuse rather than fighting the line editor
	// for input. A proper side channel is a future milestone.
	in.InREPL = true
	iolib.Install(in)
	convert.Install(in)
	mathlib.Install(in)
	stringslib.Install(in)
	listslib.Install(in)
	mapslib.Install(in)
	oslib.Install(in)
	metalib.Install(in)
	timelib.Install(in)
	hashlib.Install(in)
	crclib.Install(in)

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "jennifer: %v\n", err)
		return 1
	}

	// Print the banner in cooked mode so newlines auto-translate. Raw
	// mode (if we enter it) starts on the next line.
	fmt.Println(description)
	fmt.Println("type :quit (or Ctrl-D) to exit; :help for help")

	// Try to install raw mode. If stdin isn't a TTY (piped input, tests,
	// CI), withRawMode returns ok=false and we use the bufio fallback.
	restoreTerm, rawOK := withRawMode(os.Stdin)
	defer restoreTerm()

	history := newReplHistory()

	// Build the line reader. In raw mode the editor returns lines without
	// a trailing newline; we re-append "\n" so the rest of the loop's
	// bufio-style logic (line concatenation into buf, EOF detection) works
	// without branching.
	var readLine func(prompt string) (line string, err error)
	// stderrW and stdoutW carry the right newline convention for the mode:
	// in raw mode they translate \n to \r\n so multi-line error / banner
	// output doesn't stairstep; in cooked mode they're the bare os files.
	var stderrW io.Writer = os.Stderr
	var stdoutW io.Writer = os.Stdout
	var editor *lineEditor
	if rawOK {
		// Single wrapper shared across the editor's output channel, the
		// REPL's print helpers, and the interpreter's Out. That way every
		// byte going to stdout passes through the CRLF translator AND
		// updates lastByteIsNL, so the editor can decide whether to emit
		// a fresh line before drawing the next prompt.
		stdoutWrap := &crlfWriter{w: os.Stdout}
		editor = newLineEditor(os.Stdin, stdoutWrap, history)
		readLine = func(prompt string) (string, error) {
			s, err := editor.readLine(prompt)
			if err != nil {
				return "", err
			}
			return s + "\n", nil
		}
		stderrW = &crlfWriter{w: os.Stderr}
		stdoutW = stdoutWrap
		// Route `printf` / `sprintf`'s side-effect output through the
		// same wrapper, so user programs that write to stdout get both
		// the CRLF translation and the trailing-newline tracking.
		in.Out = stdoutWrap
	} else {
		stdin := bufio.NewReader(os.Stdin)
		readLine = func(prompt string) (string, error) {
			fmt.Print(prompt)
			return stdin.ReadString('\n')
		}
	}

	var buf strings.Builder
	for {
		prompt := replPrompt
		if buf.Len() > 0 {
			prompt = replContPrompt
		}

		line, readErr := readLine(prompt)

		// Ctrl+C in raw mode: discard the in-progress accumulator and
		// start over at a fresh prompt. The editor already printed "^C"
		// on the line being cancelled.
		if errors.Is(readErr, errLineCancelled) {
			buf.Reset()
			continue
		}
		// Treat EOF as an exit signal when the buffer is empty; otherwise
		// let the current input be evaluated (the user finished without
		// a newline).
		if readErr == io.EOF {
			if buf.Len() == 0 && line == "" {
				fmt.Fprintln(stdoutW)
				return 0
			}
		} else if readErr != nil {
			fmt.Fprintf(stderrW, "\njennifer: read error: %v\n", readErr)
			return 1
		}

		// REPL directives only count on a fresh prompt (not mid-block).
		if buf.Len() == 0 {
			switch strings.TrimSpace(line) {
			case ":quit", ":exit":
				return 0
			case ":help":
				printReplHelp(stdoutW)
				continue
			case "":
				if readErr == io.EOF {
					return 0
				}
				continue
			}
		}

		buf.WriteString(line)

		// Try to lex what we have. A lex error is final - clear the buffer,
		// and record the attempt in history so the user can up-arrow to
		// edit and retry.
		tokens, lerr := lexer.TokenizeWithFile(buf.String(), replFileTag)
		if lerr != nil {
			fmt.Fprintf(stderrW, "%s\n", lerr.Error())
			printErrorContextTo(stderrW, buf.String(), replFileTag, lerr)
			history.Add(strings.TrimRight(buf.String(), "\n"))
			buf.Reset()
			continue
		}
		if !inputComplete(tokens) {
			if readErr == io.EOF {
				// User ended input mid-statement.
				fmt.Fprintln(stderrW, "jennifer: incomplete input at EOF")
				return 1
			}
			continue
		}

		src := buf.String()
		buf.Reset()
		// Record every complete submission in history before evaluation
		// so a failing preproc/parse/runtime error still leaves the input
		// recoverable via Up-arrow. Matches the convention used by Python,
		// Node, bash, ghci, etc. Trimmed so blank edits at the end don't
		// make every entry distinct; adjacent duplicates are collapsed by
		// replHistory.Add.
		history.Add(strings.TrimRight(src, "\n"))

		tokens, perr := preproc.Process(tokens, cwd, replFileTag)
		if perr != nil {
			fmt.Fprintf(stderrW, "%s\n", perr.Error())
			printErrorContextTo(stderrW, src, replFileTag, perr)
			continue
		}
		prog, parseErr := parser.ParseTokens(tokens)
		if parseErr != nil {
			fmt.Fprintf(stderrW, "%s\n", parseErr.Error())
			printErrorContextTo(stderrW, src, replFileTag, parseErr)
			continue
		}
		val, rerr := in.EvalInteractive(prog)
		if rerr != nil {
			// `exit;` / `exit EXPR;` terminates the REPL with the
			// requested code, the same way it terminates a batch run.
			if ex, ok := rerr.(*interpreter.ExitSignal); ok {
				return ex.Code
			}
			fmt.Fprintf(stderrW, "%s\n", rerr.Error())
			printErrorContextTo(stderrW, src, replFileTag, rerr)
			continue
		}
		printReplValue(stdoutW, val)

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
		switch t.Type {
		case lexer.TOKEN_EOF,
			lexer.TOKEN_COMMENT_LINE,
			lexer.TOKEN_COMMENT_BLOCK,
			lexer.TOKEN_COMMENT_SHEBANG,
			lexer.TOKEN_BLANK_LINE:
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
func printReplValue(w io.Writer, v interpreter.Value) {
	if v.Kind == interpreter.KindNull {
		return
	}
	if v.Kind == interpreter.KindString {
		fmt.Fprintf(w, "%q\n", v.Str)
		return
	}
	fmt.Fprintln(w, v.Display())
}

func printReplHelp(w io.Writer) {
	fmt.Fprintln(w, "jennifer REPL")
	fmt.Fprintln(w, "  - statements end with ;")
	fmt.Fprintln(w, "  - unclosed braces continue on the next line")
	fmt.Fprintln(w, "  - a final bare expression prints its value")
	fmt.Fprintln(w, "  - cursor keys, Home/End, Ctrl+Left/Right, Ctrl+W edit the line")
	fmt.Fprintln(w, "  - Up/Down browse history")
	fmt.Fprintln(w, "  - :quit / :exit / Ctrl-D to exit; Ctrl-C cancels the line")
}

// crlfWriter wraps an io.Writer and translates every '\n' it sees into
// '\r\n'. Used inside the REPL loop when stdin is in raw mode (which
// disables the kernel's OPOST flag, so newlines no longer auto-translate
// on the way out). Cooked-mode writes go to the underlying writer
// directly; we only install the wrapper when the editor is active.
//
// It also tracks whether the most recent byte was a newline. The editor
// reads `lastByteIsNL` before each redraw and emits a fresh `\r\n` if
// the cursor isn't already on a clean line. Without this, a program
// like `printf("%s", meta.VERSION);` (no trailing newline) would
// have its output wiped by the next prompt's `\r ESC[K` redraw.
type crlfWriter struct {
	w            io.Writer
	lastByteIsNL bool
}

func (c *crlfWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	n := 0
	last := 0
	for i := 0; i < len(p); i++ {
		if p[i] == '\n' {
			if i > last {
				m, err := c.w.Write(p[last:i])
				n += m
				if err != nil {
					return n, err
				}
			}
			if _, err := c.w.Write([]byte{'\r', '\n'}); err != nil {
				return n, err
			}
			n++
			last = i + 1
		}
	}
	if last < len(p) {
		m, err := c.w.Write(p[last:])
		n += m
		if err != nil {
			return n, err
		}
	}
	c.lastByteIsNL = p[len(p)-1] == '\n'
	return n, nil
}
