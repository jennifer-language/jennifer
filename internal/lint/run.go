// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package lint

import (
	"sort"

	"jennifer-lang.dev/jennifer/internal/lexer"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// Config holds the tunable thresholds for the checks that have one. The
// values are fixed defaults in v1 (no CLI flag beyond --checks); the struct
// keeps them in one place and makes the checks testable.
type Config struct {
	MethodMaxStmts int // L201 threshold
	MaxNesting     int // L202 threshold
	MaxLineLength  int // L203 threshold
}

// DefaultConfig returns the v1 thresholds documented for the checks.
func DefaultConfig() Config {
	return Config{MethodMaxStmts: 60, MaxNesting: 4, MaxLineLength: 100}
}

// checkCtx is the per-run state threaded through every check. Each check
// appends its findings via report / reportAt.
type checkCtx struct {
	prog       *parser.Program
	source     string // primary file's raw text, for line-oriented checks (L203)
	sourceFile string // primary file's tag, matching node Filename()
	cfg        Config
	diags      []Diagnostic
}

// report records a finding anchored at node n.
func (c *checkCtx) report(id string, n parser.Node, msg string) {
	line, col := n.Pos()
	c.reportAt(id, n.Filename(), line, col, msg)
}

// reportAt records a finding at an explicit position (used where the anchor
// is a sub-token, like a catch introducer, rather than a whole node).
func (c *checkCtx) reportAt(id, file string, line, col int, msg string) {
	c.diags = append(c.diags, Diagnostic{
		ID:       id,
		File:     file,
		Line:     line,
		Col:      col,
		Message:  msg,
		Severity: severityOf(id),
	})
}

// severityOf returns the default severity for a check ID.
func severityOf(id string) Severity {
	for _, c := range registry {
		if c.id == id {
			return c.severity
		}
	}
	return SeverityWarning
}

// Check runs every enabled check over a parsed program and returns the
// findings, sorted and with suppressed diagnostics removed. tokens is the raw
// lexer stream (including trivia) for the same source, used to read
// `# lint-disable` directives - the parser strips comments, so suppression must
// be read off the token stream. A malformed / unknown-ID directive becomes an
// L004 finding (continue-and-report), so there is no error return: whatever is
// wrong shows up as a finding in the requested format.
func Check(prog *parser.Program, tokens []lexer.Token, source, sourceFile string, enabled map[string]bool, cfg Config) []Diagnostic {
	c := &checkCtx{prog: prog, source: source, sourceFile: sourceFile, cfg: cfg}
	for _, chk := range registry {
		if chk.run == nil || !enabled[chk.id] {
			continue
		}
		chk.run(c)
	}
	diags := applySuppressions(c.diags, tokens)
	sortDiagnostics(diags)
	return diags
}

// SourceErrorDiagnostic builds a diagnostic for a positioned pipeline error
// (lex / preprocess / parse) so the CLI can render it in the requested format
// instead of bailing to stderr. id is the L0nn source-error class ("L001"
// lex, "L002" parse, "L003" preproc); pos supplies file/line/col; msg is the
// human message (already stripped of its position prefix by the caller).
func SourceErrorDiagnostic(id, file string, line, col int, msg string) Diagnostic {
	return Diagnostic{ID: id, File: file, Line: line, Col: col, Message: msg, Severity: severityOf(id)}
}

// sortDiagnostics orders findings by position then ID so output is stable.
func sortDiagnostics(diags []Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		a, b := diags[i], diags[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Col != b.Col {
			return a.Col < b.Col
		}
		return a.ID < b.ID
	})
}
