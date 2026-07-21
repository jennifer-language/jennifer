// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package testinglib_test

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	testinglib "jennifer-lang.dev/jennifer/internal/lib/testing"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// runProg parses + runs a Jennifer program with io + testing
// installed. Wipes the accumulator so each test starts fresh.
func runProg(t *testing.T, src string) (string, error) {
	t.Helper()
	testinglib.ResetForTest()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	testinglib.Install(in)
	runErr := in.Run(prog)
	return buf.String(), runErr
}

// TestRunPassPath - a test method that returns normally produces
// a passing testing.Result with the elapsed time populated.
func TestRunPassPath(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use testing;
		func myTest() { return; }
		def r as testing.Result init testing.run("myTest");
		io.printf("passed=%t kind=[%s] name=%s", $r.passed, $r.errorKind, $r.name);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "passed=true kind=[] name=myTest"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestRunFailViaThrow - a user throw with an Error struct populates
// the result's error fields.
func TestRunFailViaThrow(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use testing;
		func badTest() {
			throw Error{kind: "assertion", message: "1 != 2", file: "", line: 5, col: 3};
		}
		def r as testing.Result init testing.run("badTest");
		io.printf("passed=%t kind=%s msg=%s line=%d",
			$r.passed, $r.errorKind, $r.errorMessage, $r.line);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "passed=false kind=assertion msg=1 != 2 line=5"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestRunFailViaRuntimeError - a runtime error (out-of-bounds)
// surfaces with kind=runtime and a helpful message.
func TestRunFailViaRuntimeError(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use testing;
		func indexTest() {
			def xs as list of int init [];
			def x as int init $xs[5];
		}
		def r as testing.Result init testing.run("indexTest");
		io.printf("passed=%t kind=%s",
			$r.passed, $r.errorKind);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.HasPrefix(out, "passed=false") {
		t.Errorf("got %q, expected failure", out)
	}
	if !strings.Contains(out, "kind=runtime") && !strings.Contains(out, "kind=out_of_bounds") {
		t.Errorf("expected runtime error kind, got %q", out)
	}
}

// TestRunCapturesExit - `exit` inside a test body is contained by
// the runner; the surrounding program keeps running.
func TestRunCapturesExit(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use testing;
		func exitTest() { exit 42; }
		def r as testing.Result init testing.run("exitTest");
		io.printf("passed=%t kind=%s msg=%s AFTER",
			$r.passed, $r.errorKind, $r.errorMessage);
	`)
	if err != nil {
		t.Fatalf("run should complete despite exit inside test: %v", err)
	}
	want := "passed=false kind=exit msg=exit code 42 AFTER"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestRunUnknownMethod - naming a method that doesn't exist errors
// with a message the runner can display.
func TestRunUnknownMethod(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use testing;
		def r as testing.Result init testing.run("noSuchMethod");
		io.printf("passed=%t kind=%s", $r.passed, $r.errorKind);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// Unknown methods surface via CallByName -> plain error ->
	// ClassifyError -> kind=unknown, msg=... . Not a Jennifer
	// runtime error, but a testing.Result nonetheless.
	if !strings.HasPrefix(out, "passed=false") {
		t.Errorf("expected failure, got %q", out)
	}
	if !strings.Contains(out, "kind=unknown") {
		t.Errorf("expected kind=unknown, got %q", out)
	}
}

// TestRunRejectsMethodWithParams - v1 only invokes zero-arg methods.
func TestRunRejectsMethodWithParams(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use testing;
		func paramTest(n as int) { return $n; }
		def r as testing.Result init testing.run("paramTest");
		io.printf("kind=%s", $r.errorKind);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// paramTest takes an int; testing.run rejects it via
	// CallByName's zero-arg guard. Kind ends up "unknown" since
	// it isn't a Jennifer runtime error.
	if !strings.Contains(out, "kind=unknown") {
		t.Errorf("expected unknown kind for param-count refusal, got %q", out)
	}
}

// TestResultsAccumulator - each testing.run appends; testing.results
// returns the whole list.
func TestResultsAccumulator(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use testing;
		func passA() { return; }
		func passB() { return; }
		func passC() { return; }
		testing.run("passA");
		testing.run("passB");
		testing.run("passC");
		def all as list of testing.Result init testing.results();
		io.printf("count=%d\n", len($all));
		for (def r in $all) {
			io.printf("%s\n", $r.name);
		}
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "count=3\npassA\npassB\npassC\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestResetClears - testing.reset wipes the accumulator.
func TestResetClears(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use testing;
		func a() { return; }
		testing.run("a");
		testing.reset();
		def rs as list of testing.Result init testing.results();
		io.printf("after_reset=%d", len($rs));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "after_reset=0" {
		t.Errorf("got %q", out)
	}
}

// TestReportText - the "text" format contains a status line per
// test and a total line.
func TestReportText(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use testing;
		func passing() { return; }
		func failing() { throw Error{kind: "assertion", message: "boom", file: "", line: 0, col: 0}; }
		testing.run("passing");
		testing.run("failing");
		def txt as string init testing.report(testing.results(), "text");
		io.printf("%s", $txt);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"PASS passing", "FAIL failing", "1 passed, 1 failed"} {
		if !strings.Contains(out, want) {
			t.Errorf("text report missing %q\nfull:\n%s", want, out)
		}
	}
}

// TestReportTAP - "tap" format has plan and per-test ok/not ok lines.
func TestReportTAP(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use testing;
		func passing() { return; }
		func failing() { throw Error{kind: "assertion", message: "boom", file: "", line: 0, col: 0}; }
		testing.run("passing");
		testing.run("failing");
		def tap as string init testing.report(testing.results(), "tap");
		io.printf("%s", $tap);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"TAP version 14", "1..2", "ok 1 - passing", "not ok 2 - failing"} {
		if !strings.Contains(out, want) {
			t.Errorf("TAP report missing %q\nfull:\n%s", want, out)
		}
	}
}

// TestReportTAPEscapesMultilineMessage - a message with newlines must not split
// a TAP report line; it renders as a YAML block scalar so `prove` still counts
// one failing test, and the `1..1` plan matches the emitted result lines.
func TestReportTAPEscapesMultilineMessage(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use testing;
		func failing() { throw Error{kind: "assertion", message: "line one\nline two # not a directive", file: "", line: 0, col: 0}; }
		testing.run("failing");
		def tap as string init testing.report(testing.results(), "tap");
		io.printf("%s", $tap);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// A raw newline inside `message: line one\nline two` would create a bogus
	// TAP line; the block scalar keeps every message line indented under it.
	if !strings.Contains(out, "  message: |") {
		t.Errorf("multi-line message not rendered as a block scalar:\n%s", out)
	}
	// Exactly one `not ok` line (the message's own `# not a directive` and its
	// second line must not read as extra results).
	if n := strings.Count(out, "not ok"); n != 1 {
		t.Errorf("expected exactly 1 'not ok' line, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "    line two # not a directive") {
		t.Errorf("second message line not indented under block scalar:\n%s", out)
	}
}

// TestReportJUnit - "junit" format is well-formed XML with a
// testsuite root and testcase entries.
func TestReportJUnit(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use testing;
		func passing() { return; }
		func failing() { throw Error{kind: "assertion", message: "boom", file: "", line: 0, col: 0}; }
		testing.run("passing");
		testing.run("failing");
		def xml as string init testing.report(testing.results(), "junit");
		io.printf("%s", $xml);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{
		"<?xml", "<testsuite ", `tests="2"`, `failures="1"`,
		`name="passing"`, `name="failing"`, "<failure ", `type="assertion"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("JUnit report missing %q\nfull:\n%s", want, out)
		}
	}
}

// TestReportUnknownFormat - a wrong format string errors at the
// boundary with the known set listed.
func TestReportUnknownFormat(t *testing.T) {
	_, err := runProg(t, `
		use testing;
		func passing() { return; }
		testing.run("passing");
		def x as string init testing.report(testing.results(), "html");
	`)
	if err == nil {
		t.Fatal("expected unknown-format error")
	}
	if !strings.Contains(err.Error(), "text") ||
		!strings.Contains(err.Error(), "tap") ||
		!strings.Contains(err.Error(), "junit") {
		t.Errorf("error should list known formats: %v", err)
	}
}

// TestResultsIsCopy - mutating the returned list doesn't affect
// the accumulator (value semantics).
func TestResultsIsCopy(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use testing;
		func a() { return; }
		testing.run("a");
		def rs as list of testing.Result init testing.results();
		# Grab another copy after (would-be) mutation. Value semantics
		# on the list means the accumulator is untouched.
		def rsAgain as list of testing.Result init testing.results();
		io.printf("first=%d second=%d", len($rs), len($rsAgain));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "first=1 second=1" {
		t.Errorf("got %q", out)
	}
}
