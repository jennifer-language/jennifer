// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package regexlib

import (
	"regexp"
	"regexp/syntax"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestBraceFreeProgramSize backs needsSizeCheck: patterns built from dense
// brace-free units compile to at most maxInstPerByte instructions per byte.
func TestBraceFreeProgramSize(t *testing.T) {
	units := []string{
		"|", "()", "(?:)*", "a*", "(a|)", `\b`, "[^a]", ".", "(?i)k", "x?",
		"(|)*", "()*?", "(?:a*)*", "(()|())", "a+?", "^$", `\pL`, "[[:alpha:]]",
		"(?s).", "(?U)a*", "é", "[a-zä]",
	}
	for _, u := range units {
		p := strings.Repeat(u, 2000/len(u))
		re, err := syntax.Parse(p, syntax.Perl)
		if err != nil {
			t.Fatalf("%q: %v", u, err)
		}
		prog, err := syntax.Compile(re.Simplify())
		if err != nil {
			t.Fatalf("%q: %v", u, err)
		}
		ratio := float64(len(prog.Inst)) / float64(len(p))
		if ratio > maxInstPerByte {
			t.Errorf("unit %q: %.2f instructions per byte, over %d", u, ratio, maxInstPerByte)
		}
	}
}

// TestCompileMissSingleParse: a cache miss on a short brace-free pattern
// costs about one regexp.Compile. Before the fix checkProgramSize parsed and
// compiled it a second time (about twice the allocations).
func TestCompileMissSingleParse(t *testing.T) {
	const pattern = `^([a-z0-9._%+-]+)@([a-z0-9-]+(?:\.[a-z0-9-]+)*)\.([a-z]+)$`
	base := testing.AllocsPerRun(20, func() {
		if _, err := regexp.Compile(pattern); err != nil {
			t.Fatal(err)
		}
	})
	miss := testing.AllocsPerRun(20, func() {
		resetCacheForTest()
		if _, err := compilePattern("t", pattern); err != nil {
			t.Fatal(err)
		}
	})
	if miss > base*1.3 {
		t.Fatalf("cache miss: %.0f allocs, regexp.Compile alone %.0f", miss, base)
	}
}

// TestSplitPartCap: split refuses more than limits.MaxRangeElements parts.
// Before the fix it built one Value per part.
func TestSplitPartCap(t *testing.T) {
	resetCacheForTest()
	s := strings.Repeat(",", limits.MaxRangeElements)
	if _, err := callRegex("split", ",", s); err == nil || !strings.Contains(err.Error(), "regex.split: more than") {
		t.Fatalf("got %v", err)
	}
	v, err := callRegex("split", ",", s[1:])
	if err != nil || len(v.List) != limits.MaxRangeElements {
		t.Fatalf("at the cap: %v, %d parts", err, len(v.List))
	}
}
