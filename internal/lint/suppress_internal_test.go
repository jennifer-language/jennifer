// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package lint

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/lexer"
)

// L0nn source errors are documented as "always active" and are not
// user-selectable; a lint-disable directive must never silence one, even though
// its ID is a known ID (so the directive itself is valid, not an L004). A
// selectable Lnnn on the same directive is still suppressed.
func TestSuppressionCannotSilenceSourceErrors(t *testing.T) {
	diags := []Diagnostic{
		{ID: "L001", File: "a.j", Line: 1, Col: 1, Message: "lex boom", Severity: severityOf("L001")},
		{ID: "L101", File: "a.j", Line: 2, Col: 1, Message: "unused", Severity: severityOf("L101")},
	}
	tokens := []lexer.Token{
		{Type: lexer.TOKEN_COMMENT_LINE, Lexeme: "# lint-disable-file: L001, L101", File: "a.j", Line: 1, Col: 1},
	}
	out := applySuppressions(diags, tokens)

	var sawL001, sawL101 bool
	for _, d := range out {
		switch d.ID {
		case "L001":
			sawL001 = true
		case "L101":
			sawL101 = true
		}
	}
	if !sawL001 {
		t.Error("L001 (source error) must not be suppressible by a directive")
	}
	if sawL101 {
		t.Error("L101 (selectable) should have been suppressed")
	}
}
