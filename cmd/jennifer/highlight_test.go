// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
	"testing"
)

var ansiSeq = regexp.MustCompile("\x1b\\[[0-9]*m")

func stripANSI(s string) string { return ansiSeq.ReplaceAllString(s, "") }

// The core safety property: colouring only wraps text in zero-width escapes,
// so removing every escape must reproduce the input byte for byte. If this
// ever fails the REPL would show the user something other than what they
// typed.
func TestHighlightPreservesText(t *testing.T) {
	cases := []string{
		"",
		"def x as int init 41;",
		"$doc;",
		`   use io;   # indented, trailing comment`,
		`if ($x > 3) { return "hi, world"; }`,
		`def m as map of string to int init {"a": 1, "b": 2};`,
		`printf("%d\n", $x);`,
		`def s as string init 'single quoted';`,
		"5 // 2", // // is the floor-div operator, not a comment
		`spawn { return $x; }`,
	}
	for _, src := range cases {
		if got := stripANSI(highlightLine(src)); got != src {
			t.Errorf("stripANSI(highlight(%q)) = %q, want unchanged", src, got)
		}
	}
}

func TestHighlightColorsCategories(t *testing.T) {
	out := highlightLine(`def x as int init 41; # note`)
	checks := []struct {
		what string
		sgr  string
	}{
		{"keyword", sgrKeyword},
		{"type", sgrType},
		{"number", sgrNumber},
		{"comment", sgrComment},
	}
	for _, c := range checks {
		if !strings.Contains(out, "\x1b["+c.sgr+"m") {
			t.Errorf("expected a %s span (SGR %s) in %q", c.what, c.sgr, out)
		}
	}
}

func TestHighlightVarRef(t *testing.T) {
	out := highlightLine("$total;")
	if !strings.Contains(out, "\x1b["+sgrVar+"m$total") {
		t.Errorf("expected $total coloured as a variable, got %q", out)
	}
}

// A line the lexer rejects (an unterminated string) must echo verbatim - no
// escapes, no dropped characters.
func TestHighlightLexErrorFallsBackVerbatim(t *testing.T) {
	src := `def s as string init "oops`
	if got := highlightLine(src); got != src {
		t.Errorf("lex-error input should be returned unchanged; got %q", got)
	}
}

// readLine, when color is on, redraws the committed line with highlighting
// before the newline. Drive the editor directly with a canned key stream.
func TestReadLineRecoloursOnCommit(t *testing.T) {
	newEditor := func(input string, color bool) (*lineEditor, *bytes.Buffer) {
		var out bytes.Buffer
		e := &lineEditor{
			in:      bufio.NewReader(strings.NewReader(input)),
			out:     &out,
			history: &replHistory{max: defaultHistoryCap},
			color:   color,
		}
		return e, &out
	}

	// Color on: the committed line is recoloured (the $x carries a var span).
	e, out := newEditor("$x;\r", true)
	line, err := e.readLine(">>> ")
	if err != nil {
		t.Fatalf("readLine: %v", err)
	}
	if line != "$x;" {
		t.Errorf("line = %q, want %q", line, "$x;")
	}
	if !strings.Contains(out.String(), "\x1b["+sgrVar+"m") {
		t.Errorf("committed line was not recoloured: %q", out.String())
	}

	// Color off: no SGR colour escapes in the output at all.
	e2, out2 := newEditor("$x;\r", false)
	if _, err := e2.readLine(">>> "); err != nil {
		t.Fatalf("readLine (no color): %v", err)
	}
	if strings.Contains(out2.String(), "\x1b["+sgrVar+"m") {
		t.Errorf("colour emitted despite color=false: %q", out2.String())
	}
}
