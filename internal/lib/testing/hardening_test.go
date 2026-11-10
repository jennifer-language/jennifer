// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package testinglib_test

import (
	"strings"
	"testing"
)

// TestRunRecursionKeepsDepth - recursion through testing.run accumulates on the
// caller's depth counter: the innermost call records a catchable "limit"
// failure instead of overflowing the Go stack.
func TestRunRecursionKeepsDepth(t *testing.T) {
	out, err := runProg(t, `
		use io; use testing;
		func r() { testing.run("r"); }
		testing.run("r");
		def n as int init 0;
		for (def x in testing.results()) { if ($x.errorKind == "limit") { $n = $n + 1; } }
		io.printf("limit=%d", $n);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "limit=1" {
		t.Errorf("got %q, want limit=1", out)
	}
}

// TestRunWithRecursionKeepsDepth - same for testing.runWith.
func TestRunWithRecursionKeepsDepth(t *testing.T) {
	out, err := runProg(t, `
		use io; use testing;
		func r(n as int) { testing.runWith("r", [$n + 1]); }
		testing.runWith("r", [0]);
		def n as int init 0;
		for (def x in testing.results()) { if ($x.errorKind == "limit") { $n = $n + 1; } }
		io.printf("limit=%d", $n);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "limit=1" {
		t.Errorf("got %q, want limit=1", out)
	}
}

// TestAssertThrowsRecursionKeepsDepth - recursion through assertThrows and
// assertThrowsWith ends in a catchable error.
func TestAssertThrowsRecursionKeepsDepth(t *testing.T) {
	out, err := runProg(t, `
		use io; use testing;
		func r() { testing.assertThrows("r", "x"); }
		func w(n as int) { testing.assertThrowsWith("w", [$n], "x"); }
		try { r(); } catch (e) { io.printf("a=%s ", $e.kind); }
		try { w(1); } catch (e) { io.printf("b=%s", $e.kind); }
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "a=assertion b=assertion" {
		t.Errorf("got %q", out)
	}
}

// TestAssertThrowsUsageErrors - a missing function or an argument list that
// doesn't fit is a usage error, not "the function threw".
func TestAssertThrowsUsageErrors(t *testing.T) {
	out, err := runProg(t, `
		use io; use testing;
		func one(n as int) { throw Error{kind: "runtime", message: "x", file: "", line: 0, col: 0}; }
		func a() { testing.assertThrows("nosuch", "unknown"); }
		func b() { testing.assertThrowsWith("one", ["x"], "runtime"); }
		func c() { testing.assertThrowsWith("one", [], "runtime"); }
		func d() { testing.assertThrowsWith("one", [1], "runtime"); }
		for (def n in ["a", "b", "c", "d"]) {
			def r as testing.Result init testing.run($n);
			io.printf("%s=%t:%s ", $n, $r.passed, $r.errorMessage);
		}
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{
		`a=false:testing.assertThrows: function "nosuch" is not defined`,
		`b=false:testing.assertThrowsWith: argument 1 to "one" must be int, got string`,
		`c=false:testing.assertThrowsWith: function "one" takes 1 parameter(s), got 0`,
		"d=true:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
}

// TestAssertEqualShowsKinds - values that display alike but differ in kind
// name their kinds.
func TestAssertEqualShowsKinds(t *testing.T) {
	out, err := runProg(t, `
		use io; use testing;
		func f() { testing.assertEqual(1, "1"); }
		def r as testing.Result init testing.run("f");
		io.printf("%s", $r.errorMessage);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "assertEqual: 1 (int) != 1 (string)" {
		t.Errorf("got %q", out)
	}
}

// TestReportsEscapeControls - control characters in a message, kind or file
// reach neither text nor TAP output raw, and a newline in `file` can't forge a
// TAP result line.
func TestReportsEscapeControls(t *testing.T) {
	out, err := runProg(t, `
		use io; use testing;
		func f() { throw Error{kind: "x: y", message: "\u001b]0;pwned\u0007\u001b[2J\u009b", file: "a.j\nok 99 - forged", line: 1, col: 1}; }
		func g() { throw Error{kind: "k", message: "two\nlines\u001b[2J", file: "", line: 1, col: 1}; }
		testing.run("f");
		testing.run("g");
		io.printf("%s", testing.report("text", testing.results()));
		io.printf("%s", testing.report("tap", testing.results()));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, bad := range []string{"\x1b", "\x07", "\u009b", "\nok 99"} {
		if strings.Contains(out, bad) {
			t.Errorf("report contains raw %q:\n%s", bad, out)
		}
	}
	for _, want := range []string{
		`\u001b]0;pwned\u0007`,
		`  kind: "x: y"`,
		`  file: "a.j\nok 99 - forged"`,
		`  message: "two\nlines\u001b[2J"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
