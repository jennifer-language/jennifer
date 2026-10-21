// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lexer"
	"jennifer-lang.dev/jennifer/internal/lint"
	"jennifer-lang.dev/jennifer/internal/module"
	"jennifer-lang.dev/jennifer/internal/parser"
	"jennifer-lang.dev/jennifer/internal/preproc"
	"jennifer-lang.dev/jennifer/internal/stdlib"
)

// Exit codes, mirroring gofmt -l / shellcheck: 0 clean, 1 findings at or
// above the severity floor, 2 the linter invocation itself failed (bad flags,
// unreadable file, bad `--checks` / config). A lex / preprocess / parse error in
// the *target* is not a failure - it is rendered as a source-error finding (L001
// / L002 / L003) in the chosen format, so it exits 1, not 2.
const (
	lintExitClean   = 0
	lintExitFinding = 1
	lintExitFailure = 2
)

// runLint implements `jennifer lint [--checks=IDS] [--format=FMT] <file.j>`.
// It reports patterns that are compile-legal but suspect; see internal/lint
// and docs/technical/cli_lint.md for the check catalog.
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
	// White-box module overlay: linting `MODULE_test.j` splices the sibling
	// `MODULE.j` in front of the test file - exactly as `jennifer test` does - so
	// scope analysis sees the module's private consts / structs / enums / methods.
	// Without it the resolver reports false positives that `test` never hits (e.g. a
	// `when Variant(bind)` binder over a module-private enum goes unrecognised,
	// leaving the bound name "undefined"). Findings anchored to a base-module file
	// are dropped afterwards: those are reported when the module is linted directly.
	baseFiles := map[string]bool{}
	if base := overlayBaseFor(absPath); base != "" {
		if baseToks, ok := tokenizeForSplice(base); ok {
			for _, t := range baseToks {
				if t.File != "" {
					baseFiles[t.File] = true
				}
			}
			tokens = spliceTokens(baseToks, tokens)
		}
	}
	prog, err := parser.ParseTokens(tokens)
	if err != nil {
		return []lint.Diagnostic{sourceErrorDiag("L002", err, absPath)}, src, absPath, false
	}
	// Scope analysis is where undefined names, shadowing and enum-match
	// checking (variant names, duplicate coverage, exhaustiveness) live. Without
	// it `lint` passed clean on a file `run` refuses to load - the opposite of
	// what a pre-commit check is for. Cross-module enum matches still need the
	// modules loaded, so those are reported by `run` / `test`, not here.
	if err := parser.Resolve(prog); err != nil {
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

	if isIncludeFragment(absPath) {
		enabled = withoutFragmentChecks(enabled)
		fmt.Fprintf(os.Stderr, "jennifer lint: note: %s is an include fragment; names it takes from the file that includes it are not checked (L107 / L108 / L112) - lint that file\n", path)
	}

	cfg := lint.DefaultConfig()
	cfg.Modules = lintModules(prog, baseDir, opts)
	cfg.LibStructs, cfg.LibConsts, cfg.Libraries = lintLibSurface()
	diags = lint.Check(prog, suppressTokens, src, absPath, enabled, cfg)
	if len(baseFiles) > 0 {
		// Drop findings that landed in the spliced base module (or its includes);
		// they belong to `lint MODULE.j`, not to linting the overlay.
		kept := diags[:0]
		for _, d := range diags {
			if !baseFiles[d.File] {
				kept = append(kept, d)
			}
		}
		diags = kept
	}
	return diags, src, absPath, false
}

// isIncludeFragment reports whether path names an include fragment: a part of a
// multi-file module (`NAME.inc.j`) that is only `include`d, never run or
// imported on its own, so it may call methods and read constants and
// namespaces that the including file defines.
func isIncludeFragment(path string) bool {
	return strings.HasSuffix(filepath.Base(path), ".inc.j")
}

// withoutFragmentChecks returns the selection minus the checks that resolve
// names a fragment takes from its including file (an undefined call, an
// undefined bare name, an undeclared namespace). Linted alone those would flag
// every borrowed name; linting the including file covers them, since the
// fragment is spliced into it.
func withoutFragmentChecks(enabled map[string]bool) map[string]bool {
	out := make(map[string]bool, len(enabled))
	for id, on := range enabled {
		out[id] = on
	}
	for _, id := range []string{"L107", "L108", "L112"} {
		delete(out, id)
	}
	return out
}

var (
	libSurfaceOnce    sync.Once
	libSurfaceStructs map[string]*parser.StructDef
	libSurfaceConsts  map[string]parser.Type
	libSurfaceLibs    []string
)

// lintLibSurface returns the standard library's registered struct types and
// constant types (keyed "lib.Name") and the library names, read once from an
// interpreter with every library installed, so lint types a library struct's
// fields and a library constant exactly as registered and checks each `use`.
func lintLibSurface() (map[string]*parser.StructDef, map[string]parser.Type, []string) {
	libSurfaceOnce.Do(func() {
		in := interpreter.New()
		stdlib.InstallAll(in)
		libSurfaceLibs = in.KnownLibraries()
		libSurfaceStructs = map[string]*parser.StructDef{}
		for k, def := range in.NSStructs {
			libSurfaceStructs[k.NS+"."+k.Name] = def
		}
		libSurfaceConsts = map[string]parser.Type{}
		for k, v := range in.NSConstants {
			if t, ok := constType(v); ok {
				libSurfaceConsts[k.NS+"."+k.Name] = t
			}
		}
	})
	return libSurfaceStructs, libSurfaceConsts, libSurfaceLibs
}

// constType returns the static type of a library constant's value: a scalar's
// kind, a struct's identity, a collection with its recorded element type (open
// when none is recorded). ok=false for any other kind.
func constType(v interpreter.Value) (parser.Type, bool) {
	switch v.Kind {
	case interpreter.KindInt:
		return parser.Type{Kind: parser.TypeInt}, true
	case interpreter.KindFloat:
		return parser.Type{Kind: parser.TypeFloat}, true
	case interpreter.KindString:
		return parser.Type{Kind: parser.TypeString}, true
	case interpreter.KindBool:
		return parser.Type{Kind: parser.TypeBool}, true
	case interpreter.KindBytes:
		return parser.Type{Kind: parser.TypeBytes}, true
	case interpreter.KindStruct, interpreter.KindObject:
		if v.StructNS == "" || v.ModPath != "" {
			return parser.Type{}, false
		}
		return parser.Type{Kind: parser.TypeStruct, StructNS: v.StructNS, StructName: v.StructName}, true
	case interpreter.KindList:
		return parser.Type{Kind: parser.TypeList, Element: v.ElemTyp}, true
	case interpreter.KindMap:
		if v.KeyTyp == nil || v.ValTyp == nil {
			return parser.Type{Kind: parser.TypeMap}, true
		}
		return parser.Type{Kind: parser.TypeMap, KeyType: v.KeyTyp, ValType: v.ValTyp}, true
	}
	return parser.Type{}, false
}

// lintModules resolves and parses each `import`ed module the way `run` would
// (relative to the program's directory, then the system module dir and any -I
// dirs, `@` decks under the vendor root), keyed by the prefix the import binds,
// so L109 / L110 / L111 / L113 / L114 can check calls and literals into it. An import that does not
// resolve or parse is left out: `run` reports that, and lint stays quiet about
// what it cannot see.
func lintModules(prog *parser.Program, baseDir string, opts lintOptions) map[string]*parser.Program {
	if len(prog.ModuleImports) == 0 {
		return nil
	}
	sm := module.ResolveSysmoddir(opts.sysmoddir, os.Getenv)
	search := append([]string{sm.Dir}, opts.includes...)
	vendorRoot := module.FindVendorRoot(opts.vendor, baseDir)
	mods := map[string]*parser.Program{}
	for _, mi := range prog.ModuleImports {
		path, err := module.Resolve(mi.Path, baseDir, search, vendorRoot)
		if err != nil {
			continue
		}
		mp, err := loadModuleProgram(path)
		if err != nil {
			continue
		}
		alias := mi.AsName
		if alias == "" {
			alias = strings.TrimSuffix(filepath.Base(mi.Path), ".j")
		}
		mods[alias] = mp
	}
	return mods
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
	// Module resolution for `import`s, as `run` takes it: the system module dir
	// (--sysmoddir), extra search dirs (-I), and the vendor root (--vendor).
	sysmoddir string
	includes  []string
	vendor    string
}

// parseLintArgs parses the lint subcommand's flags. Supported:
// --checks=IDS, --format=human|json|github, the module-resolution flags
// --sysmoddir / -I / --vendor (as `run`), -h/--help, and one or more positional
// file paths. Errors print to stderr and return ok=false.
func parseLintArgs(args []string) (lintOptions, bool) {
	opts := lintOptions{format: "human"}
	var positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		// A flag taking a directory, either `--flag=DIR` or `--flag DIR`.
		dirArg := func(flag string) (string, bool) {
			if strings.HasPrefix(a, flag+"=") {
				return strings.TrimPrefix(a, flag+"="), true
			}
			if a == flag {
				if i+1 >= len(args) {
					fmt.Fprintf(os.Stderr, "jennifer lint: %s needs a directory argument\n", flag)
					return "", false
				}
				i++
				return args[i], true
			}
			return "", false
		}
		switch {
		case a == "--sysmoddir" || strings.HasPrefix(a, "--sysmoddir="):
			dir, ok := dirArg("--sysmoddir")
			if !ok {
				return opts, false
			}
			opts.sysmoddir = dir
			continue
		case a == "--vendor" || strings.HasPrefix(a, "--vendor="):
			dir, ok := dirArg("--vendor")
			if !ok {
				return opts, false
			}
			opts.vendor = dir
			continue
		case a == "-I" || strings.HasPrefix(a, "-I="):
			dir, ok := dirArg("-I")
			if !ok {
				return opts, false
			}
			opts.includes = append(opts.includes, dir)
			continue
		}
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
		// The file/line/col properties drive GitHub's inline code annotation, but the
		// raw workflow log shows only the message - so repeat the position in the
		// message text, making a plain log line self-identifying when many files lint.
		msg := fmt.Sprintf("%s:%d:%d: [%s] %s", d.File, d.Line, d.Col, d.ID, d.Message)
		fmt.Fprintf(os.Stdout, "::%s file=%s,line=%d,col=%d::%s\n",
			level, ghProp(d.File), d.Line, d.Col, ghData(msg))
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
	fmt.Fprintln(w, "usage: jennifer lint [--checks=IDS] [--format=human|json|github] [--sysmoddir DIR] [-I DIR]... [--vendor DIR] <file.j>...")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Report compile-legal but suspect patterns. Checks:")
	for _, c := range lint.Catalog() {
		fmt.Fprintf(w, "  %s  %-9s  %s\n", c.ID, c.Severity, c.Desc)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "--checks: comma-separated IDs to run, or !ID to exclude (one direction).")
	fmt.Fprintln(w, "A .jennifer-lint file at the tree root sets per-project defaults.")
	fmt.Fprintln(w, "Suppress inline with `# lint-disable: ID` or `# lint-disable-file: ID`.")
	fmt.Fprintln(w, "--sysmoddir / -I / --vendor resolve `import`s as `run` does, so calls and")
	fmt.Fprintln(w, "literals into a module are checked against its exported surface.")
}
