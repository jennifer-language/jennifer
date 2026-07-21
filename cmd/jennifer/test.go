// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lexer"
	testinglib "jennifer-lang.dev/jennifer/internal/lib/testing"
	"jennifer-lang.dev/jennifer/internal/module"
	"jennifer-lang.dev/jennifer/internal/parser"
	"jennifer-lang.dev/jennifer/internal/preproc"
	"jennifer-lang.dev/jennifer/internal/profile"
)

// Exit codes mirror `jennifer lint`: 0 all pass, 1 one or more failed, 2 the
// runner itself failed (parse / IO / bad flags).
const (
	testExitPass    = 0
	testExitFail    = 1
	testExitFailure = 2
)

// runTest implements `jennifer test [flags] FILE.j`: it discovers the
// zero-arg test methods in FILE.j (by the `test*` convention or --filter),
// runs each through the testing runner with optional setUp/tearDown hooks,
// and prints a report. --testing-single runs exactly one method (the subprocess
// mode --isolated fires per test).
func runTest(args []string) int {
	opts, ok := parseTestArgs(args)
	if !ok {
		return testExitFailure
	}
	if opts.showHelp {
		testUsage(os.Stdout)
		return testExitPass
	}

	// --coverage installs a statement profiler and runs in-process (the
	// per-test subprocesses of --isolated each have their own counters, so the
	// two don't compose - coverage always runs in-process).
	var cov *profile.Collector
	if opts.coverage {
		cov = profile.NewCollector(profile.ModeStatement, 0)
	}

	in, prog, code := loadForTestProg(opts.path, cov)
	if in == nil {
		return code
	}

	// Single-method mode: the per-test subprocess used by --isolated.
	if opts.single != "" {
		return runSingleTest(in, opts.single)
	}

	tests := discoverTests(in, opts.filter)
	if len(tests) == 0 {
		fmt.Fprintln(os.Stderr, "jennifer test: no test methods found (default convention: names starting with `test`)")
		return testExitPass
	}

	hasSetUp := hasMethod(in, "setUp")
	hasTearDown := hasMethod(in, "tearDown")

	recs := make([]testinglib.Record, 0, len(tests))
	for _, name := range tests {
		if opts.isolated && !opts.coverage {
			recs = append(recs, runIsolatedTest(opts.path, name))
		} else {
			recs = append(recs, runInProcessTest(in, name, hasSetUp, hasTearDown))
		}
	}

	report, err := testinglib.RenderReport(recs, opts.format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jennifer test: %s\n", err.Error())
		return testExitFailure
	}
	// A machine-readable coverage report owns stdout (like `profile --format=
	// pprof`), so the human test report moves to stderr and the JSON parses.
	if opts.coverage && opts.coverageFormat == "json" {
		fmt.Fprint(os.Stderr, report)
	} else {
		fmt.Print(report)
	}

	if opts.coverage {
		creport, err := renderCoverage(prog, cov.StatementHits(), opts.coverageFormat)
		if err != nil {
			fmt.Fprintf(os.Stderr, "jennifer test: %s\n", err.Error())
			return testExitFailure
		}
		fmt.Print(creport)
	}

	for _, r := range recs {
		if !r.Passed {
			return testExitFail
		}
	}
	return testExitPass
}

// loadForTest parses, prepares, and runs FILE.j's top level so its methods are
// hoisted and reachable by name. Returns the ready interpreter, or nil plus an
// exit code on failure.
func loadForTest(path string) (*interpreter.Interpreter, int) {
	in, _, code := loadForTestProg(path, nil)
	return in, code
}

// loadForTestProg is loadForTest plus two coverage hooks: it installs the given
// statement profiler (if non-nil) before running, so hits are captured from the
// file's top-level init onward, and it returns the parsed program so a caller
// can enumerate the executable positions (the coverage denominator).
func loadForTestProg(path string, cov *profile.Collector) (*interpreter.Interpreter, *parser.Program, int) {
	src, label, absPath, baseDir, ok := loadProgramSource(path)
	if !ok {
		return nil, nil, testExitFailure
	}
	tokens, err := lexer.TokenizeWithFile(src, absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return nil, nil, testExitFailure
	}
	tokens, err = preproc.Process(tokens, baseDir, absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return nil, nil, testExitFailure
	}
	// White-box module overlay: running `jennifer test MODULE_test.j`
	// splices the sibling `MODULE.j` in front of the test file, so the test
	// methods reach the module's private names by bare identifier and slot
	// numbering covers both. The combined program is run as the module it
	// tests (module context), so the module's `export` markers are legal.
	moduleContext := false
	if base := overlayBaseFor(absPath); base != "" {
		baseToks, ok := tokenizeForSplice(base)
		if !ok {
			return nil, nil, testExitFailure
		}
		tokens = spliceTokens(baseToks, tokens)
		moduleContext = true
	}
	prog, err := parser.ParseTokens(tokens)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return nil, nil, testExitFailure
	}
	in := interpreter.New()
	installLibraries(in)
	// Enable `import "..."` so a module that itself imports sibling modules can
	// be tested through its overlay: local imports resolve relative to the test
	// file's directory (a shipped overlay's sibling module), bare names through
	// the default system module dir. Harmless for a file with no imports.
	sm, _ := setupSysmoddir("")
	in.EnableModules(baseDir, []string{sm.Dir}, loadModuleProgram, installLibraries)
	in.SetVendorRoot(module.FindVendorRoot("", baseDir))
	if moduleContext {
		in.SetModuleContext(true)
	}
	if cov != nil {
		// Statement profiling captures the coverage hits from top-level init on.
		in.SetProfiler(cov, true, false, false)
	}
	if err := in.Run(prog); err != nil {
		fmt.Fprintf(os.Stderr, "%s: setting up test file: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return nil, nil, testExitFailure
	}
	return in, prog, testExitPass
}

// overlayBaseFor returns the module file a `_test.j` overlay tests: for
// `.../MODULE_test.j` it is `.../MODULE.j` when that sibling exists, else "".
// A file not ending in `_test.j` has no base (returns "").
func overlayBaseFor(absPath string) string {
	const suffix = "_test.j"
	if !strings.HasSuffix(absPath, suffix) {
		return ""
	}
	base := strings.TrimSuffix(absPath, suffix) + ".j"
	if info, err := os.Stat(base); err == nil && !info.IsDir() {
		return base
	}
	return ""
}

// tokenizeForSplice reads, lexes, and preprocesses a file for splicing into
// another token stream, reporting any error to stderr.
func tokenizeForSplice(path string) ([]lexer.Token, bool) {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jennifer test: %v\n", err)
		return nil, false
	}
	toks, err := lexer.TokenizeWithFile(string(src), path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", path, err.Error())
		printErrorContext(string(src), path, err)
		return nil, false
	}
	toks, err = preproc.Process(toks, filepath.Dir(path), path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", path, err.Error())
		printErrorContext(string(src), path, err)
		return nil, false
	}
	return toks, true
}

// spliceTokens concatenates two token streams into one program: the base's
// trailing EOF is dropped so the overlay's tokens continue the same stream.
func spliceTokens(base, overlay []lexer.Token) []lexer.Token {
	if n := len(base); n > 0 && base[n-1].Type == lexer.TOKEN_EOF {
		base = base[:n-1]
	}
	return append(base, overlay...)
}

// runSingleTest runs one method and prints a single concise result line
// (returning 0 on pass, 1 on fail). It is the per-test subprocess --isolated
// fires: on failure the parent captures this one line as the test's message.
// Unknown method names are rejected.
func runSingleTest(in *interpreter.Interpreter, name string) int {
	if !hasMethod(in, name) {
		fmt.Fprintf(os.Stderr, "jennifer test: no method %q in the source file\n", name)
		return testExitFailure
	}
	rec := timeCall(in, name)
	if rec.Passed {
		fmt.Printf("PASS %s (%d ms)\n", name, rec.Ms)
		return testExitPass
	}
	pos := ""
	if rec.File != "" {
		pos = fmt.Sprintf(" %s:%d:%d", rec.File, rec.Line, rec.Col)
	}
	fmt.Printf("[%s]%s %s\n", rec.ErrorKind, pos, rec.ErrorMessage)
	return testExitFail
}

// runInProcessTest runs setUp (if present), the test, then tearDown (if
// present) in the shared interpreter. A setUp failure fails the test without
// running its body.
func runInProcessTest(in *interpreter.Interpreter, name string, hasSetUp, hasTearDown bool) testinglib.Record {
	if hasSetUp {
		if _, err := in.CallByName("setUp"); err != nil {
			k, m, f, l, c := interpreter.ClassifyError(err)
			return testinglib.Record{Name: name, Passed: false, ErrorKind: k, ErrorMessage: "setUp: " + m, File: f, Line: l, Col: c}
		}
	}
	rec := timeCall(in, name)
	if hasTearDown {
		// A tearDown error doesn't override a passing test result but is
		// surfaced so it isn't silently swallowed.
		if _, err := in.CallByName("tearDown"); err != nil && rec.Passed {
			k, m, f, l, c := interpreter.ClassifyError(err)
			rec.Passed = false
			rec.ErrorKind, rec.ErrorMessage, rec.File, rec.Line, rec.Col = k, "tearDown: "+m, f, l, c
		}
	}
	return rec
}

// runIsolatedTest fires this binary as a subprocess to run one test with a
// clean interpreter state, recording the child's exit code and output.
// Coarser than in-process (no structured kind/position), the trade for
// isolation.
func runIsolatedTest(path, name string) testinglib.Record {
	self, err := os.Executable()
	if err != nil {
		return testinglib.Record{Name: name, Passed: false, ErrorKind: "isolated", ErrorMessage: "cannot locate self: " + err.Error()}
	}
	start := time.Now()
	out, runErr := exec.Command(self, "test", "--testing-single="+name, path).CombinedOutput()
	rec := testinglib.Record{Name: name, Ms: time.Since(start).Milliseconds(), Passed: runErr == nil}
	if !rec.Passed {
		rec.ErrorKind = "isolated"
		rec.ErrorMessage = strings.TrimSpace(string(out))
	}
	return rec
}

// timeCall invokes a zero-arg method, timing it and classifying any error.
func timeCall(in *interpreter.Interpreter, name string) testinglib.Record {
	start := time.Now()
	_, err := in.CallByName(name)
	rec := testinglib.Record{Name: name, Ms: time.Since(start).Milliseconds()}
	if err == nil {
		rec.Passed = true
	} else {
		rec.ErrorKind, rec.ErrorMessage, rec.File, rec.Line, rec.Col = interpreter.ClassifyError(err)
	}
	return rec
}

// discoverTests returns the sorted method names that match the filter (a
// user-supplied regex, else the default `test` prefix).
func discoverTests(in *interpreter.Interpreter, filter *regexp.Regexp) []string {
	var tests []string
	for _, name := range in.MethodNames() {
		match := false
		if filter != nil {
			match = filter.MatchString(name)
		} else {
			match = strings.HasPrefix(name, "test")
		}
		if match {
			tests = append(tests, name)
		}
	}
	sort.Strings(tests)
	return tests
}

func hasMethod(in *interpreter.Interpreter, name string) bool {
	for _, n := range in.MethodNames() {
		if n == name {
			return true
		}
	}
	return false
}

type testOptions struct {
	path           string
	format         string
	filter         *regexp.Regexp
	isolated       bool
	single         string
	showHelp       bool
	coverage       bool
	coverageFormat string
}

func parseTestArgs(args []string) (testOptions, bool) {
	opts := testOptions{format: "text", coverageFormat: "text"}
	var positionals []string
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			opts.showHelp = true
			return opts, true
		case a == "--coverage":
			opts.coverage = true
		case strings.HasPrefix(a, "--coverage="):
			opts.coverage = true
			opts.coverageFormat = strings.TrimPrefix(a, "--coverage=")
		case strings.HasPrefix(a, "--format="):
			opts.format = strings.TrimPrefix(a, "--format=")
		case strings.HasPrefix(a, "--filter="):
			re, err := regexp.Compile(strings.TrimPrefix(a, "--filter="))
			if err != nil {
				fmt.Fprintf(os.Stderr, "jennifer test: invalid --filter regex: %v\n", err)
				return opts, false
			}
			opts.filter = re
		case a == "--isolated":
			opts.isolated = true
		case strings.HasPrefix(a, "--testing-single="):
			opts.single = strings.TrimPrefix(a, "--testing-single=")
		case a == "-" || !strings.HasPrefix(a, "-"):
			positionals = append(positionals, a)
		default:
			fmt.Fprintf(os.Stderr, "jennifer test: unknown flag %q\n", a)
			testUsage(os.Stderr)
			return opts, false
		}
	}
	switch opts.format {
	case "text", "tap", "junit":
	default:
		fmt.Fprintf(os.Stderr, "jennifer test: unknown --format %q (want text, tap, or junit)\n", opts.format)
		return opts, false
	}
	switch opts.coverageFormat {
	case "text", "json":
	default:
		fmt.Fprintf(os.Stderr, "jennifer test: unknown --coverage format %q (want text or json)\n", opts.coverageFormat)
		return opts, false
	}
	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, "jennifer test: expected exactly one source file")
		testUsage(os.Stderr)
		return opts, false
	}
	opts.path = positionals[0]
	return opts, true
}

func testUsage(w *os.File) {
	fmt.Fprintln(w, "usage: jennifer test [--filter=REGEX] [--format=text|tap|junit] [--isolated] [--coverage[=text|json]] <file.j>")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Discover and run the test methods in a Jennifer source file.")
	fmt.Fprintln(w, "  default            run methods whose name starts with `test`")
	fmt.Fprintln(w, "  --filter=REGEX     run methods matching the regex instead")
	fmt.Fprintln(w, "  --format=FMT       text (default), tap, or junit report")
	fmt.Fprintln(w, "  --isolated         run each test in a fresh interpreter subprocess")
	fmt.Fprintln(w, "  --coverage[=FMT]   also report statement coverage: text (default) or json")
	fmt.Fprintln(w, "                     (json owns stdout; the test report moves to stderr)")
	fmt.Fprintln(w, "  setUp / tearDown methods, if present, run around each test")
	fmt.Fprintln(w, "Exit: 0 all pass, 1 failures, 2 runner error.")
}
