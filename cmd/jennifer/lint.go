// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

//go:build !tinygo

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"jennifer-lang.dev/jennifer/internal/lexer"
	"jennifer-lang.dev/jennifer/internal/lint"
	"jennifer-lang.dev/jennifer/internal/parser"
	"jennifer-lang.dev/jennifer/internal/preproc"
)

// Exit codes, mirroring gofmt -l / shellcheck: 0 clean, 1 findings at or
// above the severity floor, 2 the linter itself failed (bad flags, IO,
// parse error, bad config).
const (
	lintExitClean   = 0
	lintExitFinding = 1
	lintExitFailure = 2
)

// runLint implements `jennifer lint [--checks=IDS] [--format=FMT] <file.j>`.
// It reports patterns that are compile-legal but suspect; see
// docs/milestones.md M16.6 and internal/lint for the check catalog.
func runLint(args []string) int {
	opts, ok := parseLintArgs(args)
	if !ok {
		return lintExitFailure
	}
	if opts.showHelp {
		lintUsage(os.Stdout)
		return lintExitClean
	}
	// Compute findings per file, then emit. `--format=json` aggregates every
	// file's findings into ONE array - a stream of per-file `[...]` documents
	// is not valid JSON - while human and github stream per file (line-oriented
	// formats where per-file output concatenates cleanly). The worst outcome
	// wins (failure > finding > clean).
	worst := lintExitClean
	var jsonAll []lint.Diagnostic
	for _, path := range opts.paths {
		diags, src, absPath, failed := lintComputeDiags(path, opts)
		if failed {
			if lintExitFailure > worst {
				worst = lintExitFailure
			}
			continue
		}
		if opts.format == "json" {
			jsonAll = append(jsonAll, diags...)
		} else {
			renderDiagnostics(diags, opts.format, src, absPath)
		}
		for _, d := range diags {
			if d.Severity >= lint.SeverityFloor && lintExitFinding > worst {
				worst = lintExitFinding
			}
		}
	}
	if opts.format == "json" {
		renderJSON(jsonAll)
	}
	return worst
}

// lintComputeDiags lexes / preprocesses / parses / checks one path and returns
// its findings, source, and file tag, plus failed=true for an invocation
// failure (IO, bad extension, or bad --checks / .jennifer-lint) that has
// already been reported to stderr. Rendering is the caller's job so it can
// aggregate JSON across files. Selection is resolved per file so a
// `.jennifer-lint` alongside each file applies.
func lintComputeDiags(path string, opts lintOptions) (diags []lint.Diagnostic, src, absPath string, failed bool) {
	src, _, absPath, baseDir, loaded := loadProgramSource(path)
	if !loaded {
		return nil, src, absPath, true // IO / bad extension: an invocation failure
	}

	// Lex / preprocess / parse errors are format-honest source findings (L0nn),
	// not stderr bail-outs, so a --format=json pipeline always gets valid output
	// that says why the file couldn't be checked. Keep the raw token stream
	// (trivia intact) for the suppression pass; preproc.Process compacts trivia
	// out of its input in place, so hand it a copy.
	rawTokens, err := lexer.TokenizeWithFile(src, absPath)
	if err != nil {
		return []lint.Diagnostic{sourceErrorDiag("L001", err, absPath)}, src, absPath, false
	}
	procInput := make([]lexer.Token, len(rawTokens))
	copy(procInput, rawTokens)
	tokens, err := preproc.Process(procInput, baseDir, absPath)
	if err != nil {
		return []lint.Diagnostic{sourceErrorDiag("L003", err, absPath)}, src, absPath, false
	}
	prog, err := parser.ParseTokens(tokens)
	if err != nil {
		return []lint.Diagnostic{sourceErrorDiag("L002", err, absPath)}, src, absPath, false
	}

	// A finding can anchor to an `include`d file (its path rides on the spliced
	// tokens), but `# lint-disable` directives live in comments the preprocessor
	// strips. Re-lex each distinct included file with trivia intact and append
	// its raw tokens, so a directive in an include can suppress its own findings.
	suppressTokens := rawTokens
	seenFile := map[string]bool{absPath: true}
	for _, t := range tokens {
		if t.File == "" || seenFile[t.File] {
			continue
		}
		seenFile[t.File] = true
		incSrc, rerr := os.ReadFile(t.File)
		if rerr != nil {
			continue
		}
		incToks, lerr := lexer.TokenizeWithFile(string(incSrc), t.File)
		if lerr != nil {
			continue
		}
		suppressTokens = append(suppressTokens, incToks...)
	}

	dotfile, hasDotfile := findDotfile(baseDir)
	enabled, err := lint.ResolveSelection(opts.checks, opts.hasChecks, dotfile, hasDotfile)
	if err != nil {
		// A bad --checks / .jennifer-lint is an invocation problem with no
		// source position, so it stays a stderr exit-2 failure.
		fmt.Fprintf(os.Stderr, "jennifer lint: %s\n", err.Error())
		return nil, src, absPath, true
	}

	return lint.Check(prog, suppressTokens, src, absPath, enabled, lint.DefaultConfig()), src, absPath, false
}

// sourceErrorDiag turns a positioned lex / preprocess / parse error into an
// L0nn source finding (severity error) so it flows through the same rendering
// and aggregation as any other finding.
func sourceErrorDiag(id string, err error, absPath string) lint.Diagnostic {
	file, line, col := absPath, 0, 0
	if p, ok := err.(interface{ Position() (string, int, int) }); ok {
		if f, l, c := p.Position(); f != "" {
			file, line, col = f, l, c
		} else {
			line, col = l, c
		}
	}
	return lint.SourceErrorDiagnostic(id, file, line, col, stripPositionPrefix(err.Error(), file, line, col))
}

// stripPositionPrefix drops the leading "FILE:LINE:COL: " that lexer / parser /
// preproc errors embed, leaving just the message (the position is carried in
// the finding's own fields).
func stripPositionPrefix(msg, file string, line, col int) string {
	marker := fmt.Sprintf("%s:%d:%d: ", file, line, col)
	if i := strings.Index(msg, marker); i >= 0 {
		return msg[i+len(marker):]
	}
	return msg
}

// lintOptions is the parsed command line.
type lintOptions struct {
	paths     []string
	checks    string
	hasChecks bool
	format    string
	showHelp  bool
}

// parseLintArgs parses the lint subcommand's flags. Supported:
// --checks=IDS, --format=human|json|github, -h/--help, and exactly one
// positional file path. Errors print to stderr and return ok=false.
func parseLintArgs(args []string) (lintOptions, bool) {
	opts := lintOptions{format: "human"}
	var positionals []string
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			opts.showHelp = true
			return opts, true
		case strings.HasPrefix(a, "--checks="):
			opts.checks = strings.TrimPrefix(a, "--checks=")
			opts.hasChecks = true
		case strings.HasPrefix(a, "--format="):
			opts.format = strings.TrimPrefix(a, "--format=")
		case a == "-":
			// The stdin convention, same as `run -`; a positional, not a flag.
			positionals = append(positionals, a)
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "jennifer lint: unknown flag %q\n", a)
			lintUsage(os.Stderr)
			return opts, false
		default:
			positionals = append(positionals, a)
		}
	}
	switch opts.format {
	case "human", "json", "github":
	default:
		fmt.Fprintf(os.Stderr, "jennifer lint: unknown --format %q (want human, json, or github)\n", opts.format)
		return opts, false
	}
	if len(positionals) < 1 {
		fmt.Fprintln(os.Stderr, "jennifer lint: expected at least one source file")
		lintUsage(os.Stderr)
		return opts, false
	}
	opts.paths = positionals
	return opts, true
}

// findDotfile searches startDir and its ancestors for a `.jennifer-lint`
// project config, returning the first found.
func findDotfile(startDir string) (string, bool) {
	dir := startDir
	for {
		if b, err := os.ReadFile(filepath.Join(dir, ".jennifer-lint")); err == nil {
			return string(b), true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// renderDiagnostics writes findings in the requested format to stdout.
func renderDiagnostics(diags []lint.Diagnostic, format, mainSrc, mainFile string) {
	switch format {
	case "json":
		renderJSON(diags)
	case "github":
		renderGitHub(diags)
	default:
		renderHuman(diags, mainSrc, mainFile)
	}
}

// renderHuman prints positioned diagnostics with source-context carets,
// matching the parse/runtime error rendering.
func renderHuman(diags []lint.Diagnostic, mainSrc, mainFile string) {
	for _, d := range diags {
		fmt.Fprintf(os.Stdout, "%s:%d:%d: %s: %s [%s]\n",
			d.File, d.Line, d.Col, d.Severity, d.Message, d.ID)
		printErrorContextTo(os.Stdout, mainSrc, mainFile, &lint.PositionError{
			File: d.File, Line: d.Line, Col: d.Col,
		})
	}
}

// renderJSON prints the findings as a single JSON array (valid JSON, so
// `jennifer lint --format=json | jq` works directly) for editor and CI
// integration. Empty output is `[]`.
func renderJSON(diags []lint.Diagnostic) {
	type finding struct {
		ID       string `json:"id"`
		File     string `json:"file"`
		Line     int    `json:"line"`
		Col      int    `json:"col"`
		Message  string `json:"message"`
		Severity string `json:"severity"`
	}
	out := make([]finding, 0, len(diags))
	for _, d := range diags {
		out = append(out, finding{d.ID, d.File, d.Line, d.Col, d.Message, d.Severity.String()})
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return
	}
	os.Stdout.Write(b)
	os.Stdout.Write([]byte("\n"))
}

// renderGitHub prints GitHub Actions workflow-command annotations. Property
// values (file) and the message data are escaped per the workflow-command
// spec so paths or messages containing special characters can't produce a
// malformed annotation.
func renderGitHub(diags []lint.Diagnostic) {
	for _, d := range diags {
		level := "warning"
		switch d.Severity {
		case lint.SeverityInfo:
			level = "notice"
		case lint.SeverityError:
			level = "error"
		}
		fmt.Fprintf(os.Stdout, "::%s file=%s,line=%d,col=%d::%s\n",
			level, ghProp(d.File), d.Line, d.Col, ghData("["+d.ID+"] "+d.Message))
	}
}

// ghData escapes the message portion of a workflow command (%, CR, LF).
func ghData(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

// ghProp escapes a workflow-command property value: everything ghData does,
// plus the property delimiters `:` and `,`. Order matters - `%` is escaped
// first (in ghData) so the escapes introduced here are not double-escaped.
func ghProp(s string) string {
	s = ghData(s)
	s = strings.ReplaceAll(s, ":", "%3A")
	s = strings.ReplaceAll(s, ",", "%2C")
	return s
}

// lintUsage prints the subcommand's help.
func lintUsage(w *os.File) {
	fmt.Fprintln(w, "usage: jennifer lint [--checks=IDS] [--format=human|json|github] <file.j>...")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Report compile-legal but suspect patterns. Checks:")
	for _, c := range lint.Catalog() {
		fmt.Fprintf(w, "  %s  %-9s  %s\n", c.ID, c.Severity, c.Desc)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "--checks: comma-separated IDs to run, or !ID to exclude (one direction).")
	fmt.Fprintln(w, "A .jennifer-lint file at the tree root sets per-project defaults.")
	fmt.Fprintln(w, "Suppress inline with `# lint-disable: ID` or `# lint-disable-file: ID`.")
}
