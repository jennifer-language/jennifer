// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package regexlib_test

import "testing"

// TestRegexErrorBranches covers the shared validation paths (takeStringArg,
// compilePattern, arity) across every regex builtin - the error branches the
// happy-path tests skip.
func TestRegexErrorBranches(t *testing.T) {
	cases := []struct{ name, src string }{
		// An invalid pattern is a catchable compile error for each consumer.
		{"bad pattern matches", `use regex; def b as bool init regex.matches("(", "x");`},
		{"bad pattern find", `use regex; def m as regex.Match init regex.find("(", "x");`},
		{"bad pattern findAll", `use regex; def ms as list of regex.Match init regex.findAll("(", "x");`},
		{"bad pattern replace", `use regex; def s as string init regex.replace("(", "x", "y");`},
		{"bad pattern split", `use regex; def xs as list of string init regex.split("(", "x");`},
		// A non-string argument is rejected (takeStringArg).
		{"non-string pattern", `use regex; def b as bool init regex.matches(5, "x");`},
		{"non-string text", `use regex; def b as bool init regex.matches("a", 5);`},
		{"non-string escape", `use regex; def s as string init regex.escape(5);`},
		// Wrong argument counts.
		{"matches arity", `use regex; def b as bool init regex.matches("a");`},
		{"replace arity", `use regex; def s as string init regex.replace("a", "b");`},
		{"escape arity", `use regex; def s as string init regex.escape("a", "b");`},
	}
	for _, c := range cases {
		if _, err := runProg(t, c.src); err == nil {
			t.Errorf("%s: expected a runtime error, got nil", c.name)
		}
	}
}

// TestRegexRuneIndices covers the byte->rune index conversion for match
// positions over multi-byte input (the `at` helper).
func TestRegexRuneIndices(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use regex;
		# "b" sits after a 3-byte rune, so its rune index (1) differs from its byte offset (3).
		def m as regex.Match init regex.find("b", "世b");
		io.printf("start=%d end=%d\n", $m.start, $m.end);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "start=1 end=2\n" {
		t.Errorf("rune indices = %q, want \"start=1 end=2\\n\"", out)
	}
}
