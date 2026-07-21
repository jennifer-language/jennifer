// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package lint reports patterns in Jennifer source that are compile-legal
// but stylistically or semantically suspect. It sits between `jennifer fmt`
// (which normalises lexical shape) and the parser (which rejects the
// outright illegal): the linter's slot is "the code parses, it runs, and
// something about it is still worth flagging."
//
// Each check has a stable ID (L001, L002, ...) so suppression comments and
// project configuration are portable and greppable. The public entry point
// is Check; the CLI (cmd/jennifer/lint.go) wraps it with file I/O and
// output-format rendering.
package lint

import "sort"

// Severity ranks a finding. The CLI's exit code is driven by whether any
// finding lands at or above a severity floor (see SeverityFloor).
type Severity int

const (
	// SeverityInfo is advisory - style thresholds a reader may reasonably
	// disagree with (method length, nesting depth). Below the exit-code
	// floor by default, so an info-only run still exits 0.
	SeverityInfo Severity = iota
	// SeverityWarning is the default for correctness-adjacent findings
	// (dead code, empty catch, non-Error throw, constant conditions).
	SeverityWarning
	// SeverityError is reserved for findings a future check deems
	// unambiguously wrong. No v1 check emits it, but the scale leaves room.
	SeverityError
)

// String renders the severity as it appears in diagnostics and the JSON
// output shape.
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	default:
		return "unknown"
	}
}

// SeverityFloor is the exit-code threshold: a run with findings at or above
// this severity exits 1, otherwise 0. Fixed at Warning for v1 (info-level
// style findings are reported but don't fail a build), mirroring the
// gofmt -l / shellcheck triaging shape.
const SeverityFloor = SeverityWarning

// Diagnostic is one finding. The field set matches the --format=json shape
// documented for the CLI: {id, file, line, col, message, severity}.
type Diagnostic struct {
	ID       string   // stable check ID, e.g. "L002"
	File     string   // source file the finding is in
	Line     int      // 1-based line
	Col      int      // 1-based column (rune-counted, matching the lexer)
	Message  string   // human-readable description
	Severity Severity // drives exit code and rendering
}

// check is the internal registry entry for one lint rule. desc feeds the
// help/catalog; run walks the program and appends diagnostics. IDs are grouped
// by concern: L0nn source errors (the file doesn't parse, or a directive is
// bad), L1nn correctness, L2nn complexity/style, L3nn API lifecycle. selectable
// is false for the L0nn source errors - they are always active and produced by
// the pipeline / suppression pass, so they can't be turned off with --checks.
type check struct {
	id         string
	desc       string
	severity   Severity
	selectable bool
	run        func(c *checkCtx)
}

// registry lists every check in ID order. A check with a nil run is either a
// source error (L0nn, produced outside the AST walk) or a reserved-but-empty
// family (L301 deprecation).
//
// It is populated in init() rather than as a var initializer: the check
// functions transitively reference severityOf, which reads registry, and Go's
// variable-initialization-cycle analysis rejects that even though nothing runs
// at init. An init() body is exempt from that analysis.
var registry []check

func init() {
	registry = []check{
		// L0nn - source errors: always active, not user-selectable.
		{id: "L001", desc: "lex-error: the source could not be tokenized", severity: SeverityError, run: nil},
		{id: "L002", desc: "parse-error: the source could not be parsed", severity: SeverityError, run: nil},
		{id: "L003", desc: "preproc-error: an include could not be spliced", severity: SeverityError, run: nil},
		{id: "L004", desc: "invalid-directive: a malformed or unknown-ID lint-disable comment", severity: SeverityError, run: nil},
		// L1nn - correctness / bug-risk.
		{id: "L101", desc: "unused-local: a local binding that is never referenced", severity: SeverityWarning, selectable: true, run: checkUnusedLocal},
		{id: "L102", desc: "dead-code-after-terminator: statements after return/throw/exit/break/continue", severity: SeverityWarning, selectable: true, run: checkDeadCode},
		{id: "L103", desc: "empty-catch: a catch block with no body", severity: SeverityWarning, selectable: true, run: checkEmptyCatch},
		{id: "L104", desc: "throw-non-error: a throw whose value is not statically an Error", severity: SeverityWarning, selectable: true, run: checkThrowNonError},
		{id: "L105", desc: "constant-condition: a statically constant if/while condition", severity: SeverityWarning, selectable: true, run: checkConstantCondition},
		// L2nn - complexity & style.
		{id: "L201", desc: "method-too-long: method body exceeds the statement-count threshold", severity: SeverityInfo, selectable: true, run: checkMethodTooLong},
		{id: "L202", desc: "nesting-too-deep: block nesting exceeds the depth threshold", severity: SeverityInfo, selectable: true, run: checkNestingTooDeep},
		{id: "L203", desc: "line-too-long: a source line exceeds the column limit", severity: SeverityInfo, selectable: true, run: checkLineTooLong},
		// L3nn - API lifecycle.
		{id: "L301", desc: "deprecation: use of a still-supported but retiring API (populated as features are deprecated)", severity: SeverityWarning, selectable: true, run: nil},
		{id: "L302", desc: "removed-api: use of an API that has been removed", severity: SeverityWarning, selectable: true, run: checkRemovedApi},
	}
}

// KnownIDs returns every registered ID in ID order - used to validate IDs in
// directives and config.
func KnownIDs() []string {
	ids := make([]string, len(registry))
	for i, c := range registry {
		ids[i] = c.id
	}
	sort.Strings(ids)
	return ids
}

// isKnownID reports whether id names a registered check or source error.
func isKnownID(id string) bool {
	for _, c := range registry {
		if c.id == id {
			return true
		}
	}
	return false
}

// isSelectable reports whether id names a check that --checks / .jennifer-lint
// may enable or exclude (the L0nn source errors are always on).
func isSelectable(id string) bool {
	for _, c := range registry {
		if c.id == id {
			return c.selectable
		}
	}
	return false
}

// selectableIDs returns the IDs that participate in --checks selection (every
// check except the always-on L0nn source errors).
func selectableIDs() []string {
	var ids []string
	for _, c := range registry {
		if c.selectable {
			ids = append(ids, c.id)
		}
	}
	sort.Strings(ids)
	return ids
}

// Catalog returns each check's ID, description, and default severity for
// help output. Order is stable (registry order).
func Catalog() []struct {
	ID       string
	Desc     string
	Severity Severity
} {
	out := make([]struct {
		ID       string
		Desc     string
		Severity Severity
	}, len(registry))
	for i, c := range registry {
		out[i].ID = c.id
		out[i].Desc = c.desc
		out[i].Severity = c.severity
	}
	return out
}
