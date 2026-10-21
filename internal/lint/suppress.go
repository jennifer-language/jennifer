// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

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
//
// A line directive covers its own line, and also every line of a token that
// spans several lines and has nowhere to carry a directive of its own:
//
//   - a multi-line string literal or block comment that ends on the
//     directive's line (`'; # lint-disable: L203` after a multi-line string);
//   - a block comment (a `/** */` docblock) that starts on the line right
//     after a directive standing alone on its line.
//
// A line comment may also end in a directive (`# long text # lint-disable:
// L203`), since a comment has no room for a second comment token; that
// embedded form is only a directive when it parses as one, so ordinary prose
// that mentions `# lint-disable` stays an ordinary comment.

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

	spans := multiLineSpans(tokens)
	lastEnd := map[string]int{} // per file: the end line of the previous non-trivia token
	for _, t := range tokens {
		if t.Type != lexer.TOKEN_COMMENT_LINE {
			if t.Type != lexer.TOKEN_BLANK_LINE {
				lastEnd[t.File] = tokenEndLine(t)
			}
			continue
		}
		alone := lastEnd[t.File] < t.Line
		ids, kind, err := parseDirective(t.Lexeme)
		if kind == dirNone {
			ids, kind = embeddedDirective(t.Lexeme)
			if kind == dirNone {
				continue
			}
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
			for _, line := range coveredLines(spans[t.File], t.Line, alone) {
				set := byLine[line]
				if set == nil {
					set = map[string]bool{}
					byLine[line] = set
				}
				for _, id := range ids {
					set[id] = true
				}
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
		// L0nn source errors are "always active" and not user-selectable; a
		// directive can never silence a lex/preproc/parse failure (parseDirective
		// already rejects an unknown ID, but an L0nn ID is known-yet-unsuppressible).
		if isSelectable(d.ID) {
			if fileSup[d.File][d.ID] {
				continue
			}
			if lineSup[d.File][d.Line][d.ID] {
				continue
			}
		}
		out = append(out, d)
	}
	return append(out, bad...)
}

// span is the line range of one token that covers several source lines.
type span struct {
	start, end int
	block      bool // a block comment (docblock), not a string literal
}

// tokenEndLine is the last source line a token occupies: a string literal's
// raw spelling or a block comment's text may carry newlines.
func tokenEndLine(t lexer.Token) int {
	text := t.Raw
	if text == "" && t.Type == lexer.TOKEN_COMMENT_BLOCK {
		text = t.Lexeme
	}
	return t.Line + strings.Count(text, "\n")
}

// multiLineSpans collects, per file, the tokens that span several lines.
func multiLineSpans(tokens []lexer.Token) map[string][]span {
	out := map[string][]span{}
	for _, t := range tokens {
		if end := tokenEndLine(t); end > t.Line {
			out[t.File] = append(out[t.File], span{start: t.Line, end: end, block: t.Type == lexer.TOKEN_COMMENT_BLOCK})
		}
	}
	return out
}

// coveredLines lists the lines a line directive on `line` applies to: its own,
// every line of a multi-line token ending there, and - for a directive alone on
// its line - every line of a block comment starting on the next line.
func coveredLines(spans []span, line int, alone bool) []int {
	lines := []int{line}
	for _, sp := range spans {
		if sp.end == line || (alone && sp.block && sp.start == line+1) {
			for l := sp.start; l <= sp.end; l++ {
				if l != line {
					lines = append(lines, l)
				}
			}
		}
	}
	return lines
}

// embeddedDirective finds a directive at the end of a line comment's text
// (`# long note # lint-disable: L203`). It returns dirNone unless the text
// after the last ` # ` marker parses as a valid line directive - an embedded
// marker that does not is prose, so it is never reported as L004, and an
// embedded `lint-disable-file` is not honoured.
func embeddedDirective(lexeme string) ([]string, int) {
	i := strings.LastIndex(lexeme, " # lint-disable")
	if i <= 0 {
		return nil, dirNone
	}
	ids, kind, err := parseDirective(lexeme[i+1:])
	if err != nil || kind != dirLine {
		// Only the line form: a file-wide directive belongs at the file head,
		// not at the end of some comment where a reader would not look for it.
		return nil, dirNone
	}
	return ids, kind
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
