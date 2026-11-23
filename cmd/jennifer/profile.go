// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo || tinygo_devtools

package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lexer"
	oslib "jennifer-lang.dev/jennifer/internal/lib/os"
	"jennifer-lang.dev/jennifer/internal/module"
	"jennifer-lang.dev/jennifer/internal/parser"
	"jennifer-lang.dev/jennifer/internal/preproc"
	"jennifer-lang.dev/jennifer/internal/profile"
)

// profileMaxCallEvents bounds the method-call timeline recorded for
// --format=trace so a long run can't exhaust memory. Beyond it, further call
// spans are dropped (the aggregate statement/allocs profiles are unbounded and
// unaffected).
const profileMaxCallEvents = 2_000_000

// profileStopGrace is how long a SIGINT / SIGTERM waits for the program to
// reach a loop checkpoint and unwind (so statements still running finish their
// records and deferred calls run) before the report is written as it stands.
const profileStopGrace = time.Second

// runProfile implements
// `jennifer profile [flags] <prog.j> [args...]`.
// It runs the program with the evaluator instrumented and writes the profile
// to stdout, or to the --out file. Without --out the program's own stdout is
// redirected to stderr so the profile (including the binary pprof form) owns
// stdout cleanly: `jennifer profile --format=pprof p.j > p.pb.gz` captures only
// the profile. With --out the program keeps its stdout.
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
	if opts.out == "" {
		in.Out = os.Stderr // program output to stderr; the profile owns stdout
	}
	installLibraries(in)

	// Enable module imports exactly as `run` does, so a program that reaches for
	// a shipped module (`markdown`, `http`, ...) profiles instead of aborting.
	// A bare `import "x.j"` resolves through the system module dir first, then any
	// -I dir; `@scope/package` resolves under the vendor root.
	sm, smErr := setupSysmoddir(opts.sysmoddir)
	if smErr != nil {
		fmt.Fprintf(os.Stderr, "jennifer profile: %v\n", smErr)
		return 2
	}
	searchDirs := append([]string{sm.Dir}, opts.includes...)
	in.EnableModules(baseDir, searchDirs, loadModuleProgram, installLibraries)
	in.SetVendorRoot(module.FindVendorRoot(opts.vendor, baseDir))

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
	// Instrument imported modules too: their statements are attributed to their
	// own file positions, and the dispatch seam corrects the caller's self time
	// so a module call is not double-counted.
	in.ProfileModules(true)

	view := profile.View{ByFunction: opts.by == "function", Top: opts.top, Focus: opts.focus, SysDir: module.RealPath(sm.Dir)}
	if wd, err := os.Getwd(); err == nil {
		view.WorkDir = module.RealPath(wd)
	}
	col.SetView(view)

	rep := &profileReport{col: col, format: opts.format, out: opts.out}
	runDone := make(chan struct{})
	stopSignals := installProfileSignals(in, rep, runDone)
	defer stopSignals()

	start := time.Now()
	col.SetWall(start, time.Time{})
	code := 0
	hadError := false
	runErr := in.Run(prog)
	col.SetWall(start, time.Now())
	close(runDone)
	if runErr != nil {
		if ex, ok := runErr.(*interpreter.ExitSignal); ok {
			code = ex.Code
		} else {
			fmt.Fprintf(os.Stderr, "%s: %s\n", label, runErr.Error())
			printErrorContext(src, absPath, runErr)
			code = 1
			hadError = true
		}
	}

	// After an error that collected nothing (a module-load or other failure
	// before the first statement ran), do not write a misleading valid-but-empty
	// profile - the stderr error is the whole story. A run that executed some
	// statements before erroring still emits its partial profile.
	if hadError && col.Empty() {
		return code
	}

	// Emit whatever was collected, even after a program error.
	if !rep.final() {
		return 2
	}
	return code
}

// profileReport writes the report: once at the end of the run (or when a
// signal ends it), and as numbered snapshots on SIGUSR1. The mutex orders a
// snapshot against the final report, and done keeps the final one single.
type profileReport struct {
	col    *profile.Collector
	format string
	out    string // --out FILE; "" writes the final report to stdout

	mu    sync.Mutex
	done  bool
	snaps int
}

// write renders the report in the chosen format to w.
func (r *profileReport) write(w io.Writer) error {
	switch r.format {
	case "pprof":
		return r.col.Pprof(w)
	case "trace":
		return r.col.Trace(w)
	default:
		r.col.Table(w)
		return nil
	}
}

// final writes the final report to --out or stdout, once. It reports false
// when writing failed (already said on stderr).
func (r *profileReport) final() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return true
	}
	r.done = true
	if r.out == "" {
		if err := r.write(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "jennifer profile: writing %s: %v\n", r.format, err)
			return false
		}
		return true
	}
	return r.writeFile(r.out)
}

// snapshot writes the profile so far to FILE.<n> (n counting from 1) when --out
// is set; without --out there is nowhere to put it that would not mix with the
// program's output.
func (r *profileReport) snapshot() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.out == "" || r.done {
		return
	}
	r.snaps++
	path := r.out + "." + strconv.Itoa(r.snaps)
	if r.writeFile(path) {
		fmt.Fprintf(os.Stderr, "jennifer profile: snapshot written to %s\n", path)
	}
}

// writeFile writes the report to path. The caller holds r.mu.
func (r *profileReport) writeFile(path string) bool {
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jennifer profile: %v\n", err)
		return false
	}
	werr := r.write(f)
	cerr := f.Close()
	if werr == nil {
		werr = cerr
	}
	if werr != nil {
		fmt.Fprintf(os.Stderr, "jennifer profile: writing %s: %v\n", path, werr)
		return false
	}
	return true
}

type profileOptions struct {
	allocs    bool
	format    string
	out       string
	by        string
	top       int
	focus     *regexp.Regexp
	path      string
	progArgs  []string
	sysmoddir string
	vendor    string
	includes  []string
	showHelp  bool
}

// parseProfileArgs parses the profile flags. Flags precede the file; anything
// after the file is passed to the program as its arguments. Unknown --format,
// the unsupported --allocs --format=trace combination and table-only flags
// with another format are rejected here, at argv parse, not deferred to output
// time.
func parseProfileArgs(args []string) (profileOptions, bool) {
	opts := profileOptions{format: "table", by: "statement"}
	// value reads a flag's argument, given as --flag=V or --flag V.
	i := 0
	value := func(a, name string) (string, bool) {
		if strings.HasPrefix(a, name+"=") {
			return strings.TrimPrefix(a, name+"="), true
		}
		if i+1 >= len(args) {
			fmt.Fprintf(os.Stderr, "jennifer profile: %s needs an argument\n", name)
			return "", false
		}
		i++
		return args[i], true
	}
	isFlag := func(a, name string) bool { return a == name || strings.HasPrefix(a, name+"=") }
	focus, top := "", ""
	for i < len(args) {
		a := args[i]
		if a == "-h" || a == "--help" {
			opts.showHelp = true
			return opts, true
		}
		if a == "-" || !strings.HasPrefix(a, "-") {
			break // the file path
		}
		var ok bool
		switch {
		case a == "--allocs":
			opts.allocs, ok = true, true
		case strings.HasPrefix(a, "--format="):
			opts.format, ok = strings.TrimPrefix(a, "--format="), true
		case isFlag(a, "--out"):
			opts.out, ok = value(a, "--out")
		case isFlag(a, "--by"):
			opts.by, ok = value(a, "--by")
		case isFlag(a, "--top"):
			top, ok = value(a, "--top")
		case isFlag(a, "--focus"):
			focus, ok = value(a, "--focus")
		case isFlag(a, "--sysmoddir"):
			opts.sysmoddir, ok = value(a, "--sysmoddir")
		case isFlag(a, "--vendor"):
			opts.vendor, ok = value(a, "--vendor")
		case isFlag(a, "-I"):
			var d string
			d, ok = value(a, "-I")
			opts.includes = append(opts.includes, d)
		default:
			fmt.Fprintf(os.Stderr, "jennifer profile: unknown flag %q\n", a)
			profileUsage(os.Stderr)
			return opts, false
		}
		if !ok {
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
	switch opts.by {
	case "statement", "function":
	default:
		fmt.Fprintf(os.Stderr, "jennifer profile: unknown --by %q (want statement or function)\n", opts.by)
		return opts, false
	}
	if opts.by == "function" && opts.allocs {
		fmt.Fprintln(os.Stderr, "jennifer profile: --by=function rolls up the statement profile; it does not apply to --allocs")
		return opts, false
	}
	if top != "" {
		n, err := strconv.Atoi(top)
		if err != nil || n < 1 {
			fmt.Fprintf(os.Stderr, "jennifer profile: --top wants a positive number, got %q\n", top)
			return opts, false
		}
		opts.top = n
	}
	if focus != "" {
		re, err := regexp.Compile(focus)
		if err != nil {
			fmt.Fprintf(os.Stderr, "jennifer profile: --focus: %v\n", err)
			return opts, false
		}
		opts.focus = re
	}
	if opts.format != "table" && (opts.by != "statement" || opts.top != 0 || opts.focus != nil) {
		fmt.Fprintln(os.Stderr, "jennifer profile: --by, --top and --focus shape the table; with --format=pprof use go tool pprof's own -top / -focus")
		return opts, false
	}
	return opts, true
}

func profileUsage(w *os.File) {
	fmt.Fprintln(w, "usage: jennifer profile [--allocs] [--format=table|pprof|trace] [--out FILE]")
	fmt.Fprintln(w, "                        [--by=statement|function] [--top N] [--focus RE] <file.j> [args...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run the program with the evaluator instrumented and write a profile to stdout")
	fmt.Fprintln(w, "(the program's own output goes to stderr), or to FILE with --out.")
	fmt.Fprintln(w, "  default        statement profile: hits, self, wait and cumulative time per source position")
	fmt.Fprintln(w, "  --allocs       value-semantics profile: eager copies (stores, literal elements, writes,")
	fmt.Fprintln(w, "                 slices, throw / catch / defer), spawn-frame copies and module-boundary")
	fmt.Fprintln(w, "                 rebuilds (excludes read-only params that were borrowed, not copied, so")
	fmt.Fprintln(w, "                 counts compare only within builds with the same borrow gating)")
	fmt.Fprintln(w, "  --format=table human-readable (default)")
	fmt.Fprintln(w, "  --format=pprof gzipped pprof, for `go tool pprof` / speedscope.app")
	fmt.Fprintln(w, "  --format=trace Chrome-trace JSON of the call timeline (not valid with --allocs)")
	fmt.Fprintln(w, "  --out FILE     write the report to FILE; the program keeps its stdout. SIGUSR1 also")
	fmt.Fprintln(w, "                 writes the profile so far to FILE.1, FILE.2, ...")
	fmt.Fprintln(w, "  --by=function  one row per function instead of per statement (table only)")
	fmt.Fprintln(w, "  --top N        show the first N rows of each table (table only)")
	fmt.Fprintln(w, "  --focus RE     keep the rows whose path or function matches RE (table only)")
	fmt.Fprintln(w, "  --sysmoddir D  system module directory for bare imports (default: built-in)")
	fmt.Fprintln(w, "  -I DIR         add DIR to the module search path (repeatable)")
	fmt.Fprintln(w, "  --vendor DIR   vendor root for @scope/package imports")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SIGINT / SIGTERM stop the program, write the report and exit 130 / 143, unless the")
	fmt.Fprintln(w, "program catches the signal itself (os.catchSignal).")
}
