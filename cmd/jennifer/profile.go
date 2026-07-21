// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package main

import (
	"fmt"
	"os"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lexer"
	oslib "jennifer-lang.dev/jennifer/internal/lib/os"
	"jennifer-lang.dev/jennifer/internal/parser"
	"jennifer-lang.dev/jennifer/internal/preproc"
	"jennifer-lang.dev/jennifer/internal/profile"
)

// profileMaxCallEvents bounds the method-call timeline recorded for
// --format=trace so a long run can't exhaust memory. Beyond it, further call
// spans are dropped (the aggregate statement/allocs profiles are unbounded and
// unaffected).
const profileMaxCallEvents = 2_000_000

// runProfile implements
// `jennifer profile [--allocs] [--format=table|pprof|trace] <prog.j> [args...]`.
// It runs the program with the evaluator instrumented and writes the profile
// to stdout. The program's own stdout is redirected to stderr so the profile
// (including the binary pprof form) owns stdout cleanly: `jennifer profile
// --format=pprof p.j > p.pb.gz` captures only the profile.
func runProfile(args []string) int {
	opts, ok := parseProfileArgs(args)
	if !ok {
		return 2
	}
	if opts.showHelp {
		profileUsage(os.Stdout)
		return 0
	}

	src, label, absPath, baseDir, loaded := loadProgramSource(opts.path)
	if !loaded {
		return 2
	}
	tokens, err := lexer.TokenizeWithFile(src, absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return 2
	}
	tokens, err = preproc.Process(tokens, baseDir, absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return 2
	}
	prog, err := parser.ParseTokens(tokens)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return 2
	}

	oslib.SetUserArgs(append([]string{opts.path}, opts.progArgs...))

	in := interpreter.New()
	in.Out = os.Stderr // program output to stderr; the profile owns stdout
	installLibraries(in)

	mode := profile.ModeStatement
	if opts.allocs {
		mode = profile.ModeAllocs
	}
	col := profile.NewCollector(mode, profileMaxCallEvents)
	switch {
	case opts.allocs:
		in.SetProfiler(col, false, false, true)
	case opts.format == "trace":
		in.SetProfiler(col, false, true, false)
	default:
		in.SetProfiler(col, true, false, false)
	}

	code := 0
	if runErr := in.Run(prog); runErr != nil {
		if ex, ok := runErr.(*interpreter.ExitSignal); ok {
			code = ex.Code
		} else {
			fmt.Fprintf(os.Stderr, "%s: %s\n", label, runErr.Error())
			printErrorContext(src, absPath, runErr)
			code = 1
		}
	}

	// Emit whatever was collected, even after a program error.
	switch opts.format {
	case "pprof":
		if err := col.Pprof(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "jennifer profile: writing pprof: %v\n", err)
			return 2
		}
	case "trace":
		if err := col.Trace(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "jennifer profile: writing trace: %v\n", err)
			return 2
		}
	default:
		col.Table(os.Stdout)
	}
	return code
}

type profileOptions struct {
	allocs   bool
	format   string
	path     string
	progArgs []string
	showHelp bool
}

// parseProfileArgs parses the profile flags. Flags precede the file; anything
// after the file is passed to the program as its arguments. Unknown --format
// and the unsupported --allocs --format=trace combination are rejected here,
// at argv parse, not deferred to output time.
func parseProfileArgs(args []string) (profileOptions, bool) {
	opts := profileOptions{format: "table"}
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "-h" || a == "--help" {
			opts.showHelp = true
			return opts, true
		}
		if a == "-" || !strings.HasPrefix(a, "-") {
			break // the file path
		}
		switch {
		case a == "--allocs":
			opts.allocs = true
		case strings.HasPrefix(a, "--format="):
			opts.format = strings.TrimPrefix(a, "--format=")
		default:
			fmt.Fprintf(os.Stderr, "jennifer profile: unknown flag %q\n", a)
			profileUsage(os.Stderr)
			return opts, false
		}
		i++
	}
	if i >= len(args) {
		fmt.Fprintln(os.Stderr, "jennifer profile: expected a source file")
		profileUsage(os.Stderr)
		return opts, false
	}
	opts.path = args[i]
	opts.progArgs = args[i+1:]

	switch opts.format {
	case "table", "pprof", "trace":
	default:
		fmt.Fprintf(os.Stderr, "jennifer profile: unknown --format %q (want table, pprof, or trace)\n", opts.format)
		return opts, false
	}
	if opts.allocs && opts.format == "trace" {
		fmt.Fprintln(os.Stderr, "jennifer profile: --allocs --format=trace is not supported (allocation events have no timeline); use table or pprof")
		return opts, false
	}
	return opts, true
}

func profileUsage(w *os.File) {
	fmt.Fprintln(w, "usage: jennifer profile [--allocs] [--format=table|pprof|trace] <file.j> [args...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run the program with the evaluator instrumented and write a profile to stdout")
	fmt.Fprintln(w, "(the program's own output goes to stderr).")
	fmt.Fprintln(w, "  default        statement profile: hits + wall-clock time per source position")
	fmt.Fprintln(w, "  --allocs       value-semantics profile: COW detachments and spawn-frame copies")
	fmt.Fprintln(w, "  --format=table human-readable (default)")
	fmt.Fprintln(w, "  --format=pprof gzipped pprof, for `go tool pprof` / speedscope.app")
	fmt.Fprintln(w, "  --format=trace Chrome-trace JSON of the call timeline (not valid with --allocs)")
}
