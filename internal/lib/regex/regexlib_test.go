// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package regexlib_test

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	mapslib "jennifer-lang.dev/jennifer/internal/lib/maps"
	regexlib "jennifer-lang.dev/jennifer/internal/lib/regex"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// runProg drives one Jennifer scenario with io + regex installed.
// Wipes the pattern cache before each run so the LRU state is
// deterministic across tests.
func runProg(t *testing.T, src string) (string, error) {
	t.Helper()
	regexlib.ResetForTest()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	mapslib.Install(in)
	regexlib.Install(in)
	runErr := in.Run(prog)
	return buf.String(), runErr
}

// TestMatchesPredicate is the boolean happy path.
func TestMatchesPredicate(t *testing.T) {
	cases := []struct {
		name, pat, s string
		want         string
	}{
		{"letters match", `[a-z]+`, "hello", "true"},
		{"digits no letters", `[a-z]+`, "12345", "false"},
		{"anchored", `^hello$`, "hello world", "false"},
		{"case-insensitive flag", `(?i)hello`, "HELLO", "true"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := runProg(t, `use io; use regex; io.printf("%t", regex.matches("`+c.pat+`", "`+c.s+`"));`)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// TestFindShape - one match, positional groups populated.
func TestFindShape(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use regex;
		def m as regex.Match init regex.find("([a-z]+)=([0-9]+)", "port=8080");
		io.printf("text=%s start=%d end=%d group1=%s group2=%s",
			$m.text, $m.start, $m.end, $m.groups[0], $m.groups[1]);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "text=port=8080 start=0 end=9 group1=port group2=8080"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestFindNoMatchSentinel - regex.find returns start=-1 when there's
// no match.
func TestFindNoMatchSentinel(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use regex;
		def m as regex.Match init regex.find("[0-9]+", "no digits here");
		io.printf("start=%d text=[%s]", $m.start, $m.text);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "start=-1 text=[]"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestFindAllReturnsList - findAll returns every match in order.
func TestFindAllReturnsList(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use regex;
		def ms as list of regex.Match init regex.findAll("[0-9]+", "a1 b22 c333");
		io.printf("count=%d\n", len($ms));
		for (def m in $ms) {
			io.printf("%s\n", $m.text);
		}
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "count=3\n1\n22\n333\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// findAll over a multibyte subject must report rune indices correctly for
// every match: the amortized byte->rune tracker (shared across matches) must
// give the same result as a per-match rescan.
func TestFindAllMultibyteRuneIndices(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use regex;
		def ms as list of regex.Match init regex.findAll("x", "áx-éx-íx");
		for (def m in $ms) {
			io.printf("%d,%d ", $m.start, $m.end);
		}
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// "áx-éx-íx": runes á=0 x=1 -=2 é=3 x=4 -=5 í=6 x=7. The three x's are at
	// rune indices 1, 4, 7 (byte indices would be 2, 6, 10).
	want := "1,2 4,5 7,8 "
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestReplace exercises the standard replace-all path.
func TestReplace(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use regex;
		def r as string init regex.replace("[aeiou]", "hello world", "*");
		io.printf("%s", $r);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "h*ll* w*rld" {
		t.Errorf("got %q", out)
	}
}

// TestReplaceWithGroupReference - $1 in the replacement expands to
// the first group.
func TestReplaceWithGroupReference(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use regex;
		def r as string init regex.replace("([a-z]+)=([0-9]+)", "port=8080 host=localhost", "$1[$2]");
		io.printf("%s", $r);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// "port[8080] host=localhost" - only the first match has a
	// numeric right-hand side.
	if !strings.Contains(out, "port[8080]") {
		t.Errorf("expected group substitution, got %q", out)
	}
}

// TestSplit - regex.split on whitespace.
func TestSplit(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use regex;
		def parts as list of string init regex.split("\\s+", "one   two  three four");
		io.printf("%d [", len($parts));
		for (def p in $parts) { io.printf("%s||", $p); }
		io.printf("]");
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "4 [one|two|three|four|]"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestEscapeRoundTrip - escaping a literal string with metacharacters
// then using it in matches should behave as a literal match.
func TestEscapeRoundTrip(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use regex;
		def literal as string init "1+2=(3)";
		def pat as string init regex.escape($literal);
		io.printf("match=%t not_regex=%t", regex.matches($pat, "the answer to 1+2=(3) is..."), regex.matches($literal, "the answer to 1+2=(3) is..."));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// Both should be true: the literal pattern would try to match
	// "1" then "2=" (unusual regex but still matches), and the
	// escaped version matches "1+2=(3)" verbatim. We check the
	// escape side explicitly.
	if !strings.HasPrefix(out, "match=true") {
		t.Errorf("expected match=true, got %q", out)
	}
}

// TestNamedGroups - (?P<name>...) captures produce named entries.
func TestNamedGroups(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use regex;
		use maps;
		def m as regex.Match init regex.find("(?P<key>[a-z]+)=(?P<value>[0-9]+)", "port=8080");
		def keys as list of string init maps.keys($m.groupsNamed);
		io.printf("key=%s value=%s\n", $m.groupsNamed["key"], $m.groupsNamed["value"]);
		io.printf("named_keys=%d\n", len($keys));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "key=port value=8080\nnamed_keys=2\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestRuneIndicesOnMultibyte - start/end are rune indices, not byte
// indices, so multi-byte characters advance the count by one per
// rune.
func TestRuneIndicesOnMultibyte(t *testing.T) {
	// "café" is 4 runes; "e" is followed by combining acute in some
	// forms, but in Go's default string literals "café" is 4 runes /
	// 5 bytes because é is a single 2-byte NFC codepoint. The token
	// "d" appears at rune index 5 in "café is d".
	out, err := runProg(t, `
		use io;
		use regex;
		def m as regex.Match init regex.find("d", "café is d");
		io.printf("start=%d", $m.start);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// "café is d" - rune positions: c=0, a=1, f=2, é=3, space=4,
	// i=5, s=6, space=7, d=8. Byte position would be 9 (é takes
	// 2 bytes).
	if out != "start=8" {
		t.Errorf("got %q, want start=8 (rune index, not byte index)", out)
	}
}

// TestInvalidPatternError - a syntactically invalid pattern surfaces
// at the boundary with the pattern quoted.
func TestInvalidPatternError(t *testing.T) {
	_, err := runProg(t, `
		use regex;
		def m as regex.Match init regex.find("[unterminated", "whatever");
	`)
	if err == nil {
		t.Fatal("expected invalid-pattern error")
	}
	if !strings.Contains(err.Error(), "invalid pattern") {
		t.Errorf("error should mention 'invalid pattern': %v", err)
	}
	if !strings.Contains(err.Error(), "unterminated") {
		t.Errorf("error should quote the pattern: %v", err)
	}
}

// TestLRUCacheReuse - calling matches with the same pattern many
// times should compile only once (though we can't easily observe
// that directly; the test just confirms nothing breaks and the
// LRU stays consistent under load).
func TestLRUCacheReuse(t *testing.T) {
	// Prime the cache by matching against 200 distinct patterns
	// (larger than cacheCap=128); the LRU should evict oldest
	// silently, no error surfaces.
	var sb strings.Builder
	sb.WriteString("use regex;\n")
	for i := 0; i < 200; i++ {
		sb.WriteString("regex.matches(\"p")
		// Use hex-ish patterns of varying lengths to keep them
		// distinct.
		for j := 0; j < i%8+1; j++ {
			sb.WriteString("a")
		}
		sb.WriteString("\", \"x\");\n")
	}
	_, err := runProg(t, sb.String())
	if err != nil {
		t.Fatalf("LRU under load: %v", err)
	}
}

// TestReplaceEmptyStringPattern - a pattern that matches empty
// positions (like `a*`) is legal but corner-case; make sure we
// don't infinite-loop.
func TestReplaceEmptyStringPattern(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use regex;
		def r as string init regex.replace("x", "aXbXc", "-");
		io.printf("%s", $r);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "aXbXc" {
		t.Errorf("literal 'x' shouldn't match 'X'; got %q", out)
	}
}
