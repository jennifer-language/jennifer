// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package lint

import (
	"fmt"
	"strings"

	"jennifer-lang.dev/jennifer/internal/lexer"
)

// Suppression directives are read off the raw token stream: the parser
// discards comments, so the AST carries no directive information. Two forms,
// both line comments:
//
//	# lint-disable: L101, L102        (trailing) - suppresses those IDs on this line
//	# lint-disable-file: L201, L202   (file head) - suppresses those IDs file-wide
//
// There is no blanket "disable all": a directive names IDs, so a review can
// grep for what was silenced. Unknown or missing IDs are errors.

const (
	dirNone = iota
	dirLine
	dirFile
)

// PositionError is a linter error that carries a source position, so the CLI
// can render it with the same source-context caret as parse and runtime
// errors. It satisfies the CLI's positioned interface structurally.
type PositionError struct {
	File string
	Line int
	Col  int
	Msg  string
}

func (e *PositionError) Error() string { return e.Msg }

// Position exposes the error's source location.
func (e *PositionError) Position() (string, int, int) { return e.File, e.Line, e.Col }

// applySuppressions removes findings silenced by `# lint-disable` directives
// in the token stream and returns the survivors plus one L004 invalid-directive
// finding per malformed / unknown-ID directive. A bad directive suppresses
// nothing - the findings it meant to silence still surface - and is reported
// alongside them: continue-and-report, not abort. So the output is always the
// requested format, never a stderr bail-out.
func applySuppressions(diags []Diagnostic, tokens []lexer.Token) []Diagnostic {
	lineSup := map[string]map[int]map[string]bool{}
	fileSup := map[string]map[string]bool{}
	var bad []Diagnostic

	for _, t := range tokens {
		if t.Type != lexer.TOKEN_COMMENT_LINE {
			continue
		}
		ids, kind, err := parseDirective(t.Lexeme)
		if kind == dirNone {
			continue
		}
		if err != nil {
			bad = append(bad, Diagnostic{
				ID: "L004", File: t.File, Line: t.Line, Col: t.Col,
				Message: err.Error(), Severity: severityOf("L004"),
			})
			continue
		}
		switch kind {
		case dirLine:
			byLine := lineSup[t.File]
			if byLine == nil {
				byLine = map[int]map[string]bool{}
				lineSup[t.File] = byLine
			}
			set := byLine[t.Line]
			if set == nil {
				set = map[string]bool{}
				byLine[t.Line] = set
			}
			for _, id := range ids {
				set[id] = true
			}
		case dirFile:
			set := fileSup[t.File]
			if set == nil {
				set = map[string]bool{}
				fileSup[t.File] = set
			}
			for _, id := range ids {
				set[id] = true
			}
		}
	}

	out := make([]Diagnostic, 0, len(diags)+len(bad))
	for _, d := range diags {
		if fileSup[d.File][d.ID] {
			continue
		}
		if lineSup[d.File][d.Line][d.ID] {
			continue
		}
		out = append(out, d)
	}
	return append(out, bad...)
}

// parseDirective inspects a line comment's text. It returns the named IDs and
// which directive form it is; kind is dirNone for an ordinary comment. An
// error is returned for a directive that names an unknown ID or names none.
func parseDirective(lexeme string) (ids []string, kind int, err error) {
	body := strings.TrimSpace(strings.TrimLeft(lexeme, "#"))
	var rest string
	switch {
	case strings.HasPrefix(body, "lint-disable-file:"):
		kind = dirFile
		rest = body[len("lint-disable-file:"):]
	case strings.HasPrefix(body, "lint-disable:"):
		kind = dirLine
		rest = body[len("lint-disable:"):]
	default:
		return nil, dirNone, nil
	}
	for _, tok := range strings.FieldsFunc(rest, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	}) {
		id := strings.TrimSpace(tok)
		if id == "" {
			continue
		}
		if !isKnownID(id) {
			return nil, kind, fmt.Errorf("unknown check ID %q in lint-disable directive", id)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, kind, fmt.Errorf("lint-disable directive names no check IDs (there is no blanket disable-all)")
	}
	return ids, kind, nil
}
