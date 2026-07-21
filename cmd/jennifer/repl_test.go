// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/lexer"
)

func TestInputComplete(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"empty", "", true},
		{"whitespace only", "   \n\t", true},
		{"comment only", "# hi\n", true},
		{"single statement", "$x = 1;", true},
		{"missing semicolon", "$x = 1", false},
		{"unclosed brace", "func f() {", false},
		{"unclosed paren", "io.printf(1 + 2", false},
		{"closed block ends statement", "if (true) { $x = 1; }", true},
		{"open then closed brace", "func f() {\n  return 1;\n}", true},
		{"trailing semi after block", "func f() { return 1; };", true},
		{"semi inside string is ignored as terminator", "$x = \"hi;\";", true},
		{"unclosed semi inside string keeps brace open", "$x = \"hi;\"", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tokens, err := lexer.TokenizeWithFile(c.src, "<repl>")
			if err != nil {
				// A lex error means the REPL would surface it and reset -
				// "completeness" doesn't apply, but for these cases we don't
				// expect lex errors. Surface as failure to catch regressions.
				t.Fatalf("unexpected lex error: %v", err)
			}
			if got := inputComplete(tokens); got != c.want {
				t.Errorf("inputComplete(%q) = %v, want %v", c.src, got, c.want)
			}
		})
	}
}
