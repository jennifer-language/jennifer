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
	"github.com/mplx/jennifer-lang/internal/lib/crc"
	"github.com/mplx/jennifer-lang/internal/lib/encoding"
	"github.com/mplx/jennifer-lang/internal/lib/hash"
	"github.com/mplx/jennifer-lang/internal/lib/io"
	"github.com/mplx/jennifer-lang/internal/lib/lists"
	"github.com/mplx/jennifer-lang/internal/lib/maps"
	"github.com/mplx/jennifer-lang/internal/lib/math"
	"github.com/mplx/jennifer-lang/internal/lib/meta"
	"github.com/mplx/jennifer-lang/internal/lib/os"
	"github.com/mplx/jennifer-lang/internal/lib/strings"
	"github.com/mplx/jennifer-lang/internal/lib/task"
	"github.com/mplx/jennifer-lang/internal/lib/time"
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
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		// args[3:] become the user program's command-line arguments.
		// Convention (matches Python sys.argv, Go os.Args): index 0 of
		// the user-visible `os.ARGS` is the script path, the rest are
		// the user-supplied args.
		userArgs := append([]string{os.Args[2]}, os.Args[3:]...)
		oslib.SetUserArgs(userArgs)
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
	description = "jennifer - Jennifer programming language interpreter"
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
	fmt.Fprintln(os.Stderr, "  jennifer fmt <file>      format the source per docs/user-guide/style-guide.md")
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
	listslib.Install(in)
	mapslib.Install(in)
	oslib.Install(in)
	metalib.Install(in)
	timelib.Install(in)
	hashlib.Install(in)
	crclib.Install(in)
	encodinglib.Install(in)
	tasklib.Install(in)
	runErr := in.Run(prog)

	// M16.0: the exit-time loud-fail. Even when Run returned cleanly,
	// any spawned task that ended with an error and was never
	// task.wait'd / task.discard'd has its error printed to stderr
	// and bumps the exit code. Tasks that are still in flight at exit
	// have not been observed either, so the scan blocks on each
	// until it finishes (the goroutine has nowhere to go if we don't
	// drain it; the alternative is silently dropping its outcome).
	unwaited := in.UnwaitedTaskErrors()
	var unwaitedExit *interpreter.ExitSignal
	for _, e := range unwaited {
		if ex, ok := e.(*interpreter.ExitSignal); ok {
			// An unwaited spawn invoked `exit EXPR;` - that exit code
			// becomes the program's exit code, dominating any other
			// unwaited error.
			if unwaitedExit == nil {
				unwaitedExit = ex
			}
			continue
		}
		fmt.Fprintf(os.Stderr, "%s: unwaited spawn error: %s\n", label, e.Error())
		printErrorContext(src, absPath, e)
	}

	if runErr != nil {
		// `exit;` / `exit EXPR;` (M11) - user-requested clean
		// termination from the main flow. Propagate the requested exit
		// code without printing a runtime error trace.
		if ex, ok := runErr.(*interpreter.ExitSignal); ok {
			return ex.Code
		}
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, runErr.Error())
		printErrorContext(src, absPath, runErr)
		return 1
	}
	if unwaitedExit != nil {
		return unwaitedExit.Code
	}
	if len(unwaited) > 0 {
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
// Writes to os.Stderr; for callers that need a different destination (e.g.
// the REPL's CRLF-translating wrapper) use printErrorContextTo.
func printErrorContext(src, mainFile string, err error) {
	printErrorContextTo(os.Stderr, src, mainFile, err)
}

// printErrorContextTo is the writer-parametric form. The REPL uses it to
// route caret output through a crlfWriter when stdin is in raw mode.
func printErrorContextTo(w io.Writer, src, mainFile string, err error) {
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
	fmt.Fprintf(w, "  %s\n", srcLine)
	if col > 0 {
		fmt.Fprintf(w, "  %s^\n", caretIndent(srcLine, col))
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
