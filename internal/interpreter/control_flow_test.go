// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// ---- break ----

func TestBreakExitsForLoop(t *testing.T) {
	out, err := run(t, `
use io;
for (def i as int init 0; $i < 10; $i = $i + 1) {
    if ($i == 3) { break; }
    io.printf("%d ", $i);
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "0 1 2 " {
		t.Errorf("got %q, want %q", out, "0 1 2 ")
	}
}

func TestBreakExitsWhile(t *testing.T) {
	out, err := run(t, `
use io;
def i as int init 0;
while (true) {
    if ($i == 3) { break; }
    io.printf("%d ", $i);
    $i = $i + 1;
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "0 1 2 " {
		t.Errorf("got %q", out)
	}
}

func TestBreakExitsForEach(t *testing.T) {
	out, err := run(t, `
use io;
def xs as list of int init [10, 20, 30, 40, 50];
for (def x in $xs) {
    if ($x > 25) { break; }
    io.printf("%d ", $x);
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "10 20 " {
		t.Errorf("got %q", out)
	}
}

// A for-each iterates a stable snapshot of the collection: an in-loop element
// write (and an in-loop append, which may or may not reallocate the backing)
// must not change what the current loop yields. Iteration behaviour must not
// depend on Go slice capacity.
func TestForEachIteratesSnapshot(t *testing.T) {
	// In-place element write during iteration: the loop still yields 1 2 3.
	out, err := run(t, `
use io;
def xs as list of int init [1, 2, 3];
for (def x in $xs) { $xs[2] = 99; io.printf("%d ", $x); }
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "1 2 3 " {
		t.Errorf("element-write during iteration: got %q, want %q", out, "1 2 3 ")
	}
	// An in-loop append (reallocates) must not change iteration either, and
	// must not extend it.
	out, err = run(t, `
use io;
def xs as list of int init [1, 2, 3];
for (def x in $xs) { $xs[] = 0; $xs[2] = 99; io.printf("%d ", $x); }
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "1 2 3 " {
		t.Errorf("append during iteration: got %q, want %q", out, "1 2 3 ")
	}
}

// `break` from the inner loop must not escape to the outer one - the
// outer loop completes its iterations normally.
func TestBreakOnlyExitsInnermostLoop(t *testing.T) {
	out, err := run(t, `
use io;
for (def i as int init 0; $i < 3; $i = $i + 1) {
    for (def j as int init 0; $j < 5; $j = $j + 1) {
        if ($j == 2) { break; }
        io.printf("(%d,%d) ", $i, $j);
    }
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "(0,0) (0,1) (1,0) (1,1) (2,0) (2,1) "
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestBreakOutsideLoopErrors(t *testing.T) {
	_, err := run(t, `
use io;
break;
`)
	if err == nil || !strings.Contains(err.Error(), "only valid inside a loop") {
		t.Errorf("expected loop-misuse error, got %v", err)
	}
}

// `break` inside a method body must not cross the call boundary into
// the caller's loop.
func TestBreakDoesNotCrossMethodBoundary(t *testing.T) {
	_, err := run(t, `
use io;
func f() {
    break;
}
for (def i as int init 0; $i < 3; $i = $i + 1) {
    f();
}
`)
	if err == nil || !strings.Contains(err.Error(), "only valid inside a loop") {
		t.Errorf("expected loop-misuse error, got %v", err)
	}
}

// ---- continue ----

func TestContinueSkipsForBody(t *testing.T) {
	out, err := run(t, `
use io;
for (def i as int init 0; $i < 5; $i = $i + 1) {
    if ($i % 2 == 0) { continue; }
    io.printf("%d ", $i);
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "1 3 " {
		t.Errorf("got %q", out)
	}
}

// `continue` in a C-style for loop still runs the step (mirrors C/Go).
func TestContinueRunsForStep(t *testing.T) {
	out, err := run(t, `
use io;
for (def i as int init 0; $i < 5; $i = $i + 1) {
    if ($i == 2) { continue; }
    io.printf("%d ", $i);
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "0 1 3 4 " {
		t.Errorf("got %q", out)
	}
}

func TestContinueInWhile(t *testing.T) {
	out, err := run(t, `
use io;
def i as int init 0;
while ($i < 5) {
    $i = $i + 1;
    if ($i == 3) { continue; }
    io.printf("%d ", $i);
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "1 2 4 5 " {
		t.Errorf("got %q", out)
	}
}

func TestContinueOutsideLoopErrors(t *testing.T) {
	_, err := run(t, `continue;`)
	if err == nil || !strings.Contains(err.Error(), "only valid inside a loop") {
		t.Errorf("expected loop-misuse error, got %v", err)
	}
}

// ---- repeat..until ----

func TestRepeatRunsBodyAtLeastOnce(t *testing.T) {
	out, err := run(t, `
use io;
repeat {
    io.printf("once\n");
} until (true);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "once\n" {
		t.Errorf("got %q", out)
	}
}

func TestRepeatStopsWhenUntilTrue(t *testing.T) {
	out, err := run(t, `
use io;
def n as int init 0;
repeat {
    io.printf("%d ", $n);
    $n = $n + 1;
} until ($n >= 3);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "0 1 2 " {
		t.Errorf("got %q", out)
	}
}

func TestRepeatBreak(t *testing.T) {
	out, err := run(t, `
use io;
def n as int init 0;
repeat {
    if ($n == 2) { break; }
    io.printf("%d ", $n);
    $n = $n + 1;
} until (false);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "0 1 " {
		t.Errorf("got %q", out)
	}
}

func TestRepeatContinueRevisitsUntil(t *testing.T) {
	// `continue` in repeat means "go check `until`"; the loop still
	// terminates normally when `until` becomes true.
	out, err := run(t, `
use io;
def n as int init 0;
repeat {
    $n = $n + 1;
    if ($n % 2 == 0) { continue; }
    io.printf("%d ", $n);
} until ($n >= 5);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "1 3 5 " {
		t.Errorf("got %q", out)
	}
}

func TestRepeatUntilNonBoolErrors(t *testing.T) {
	_, err := run(t, `repeat { } until (42);`)
	if err == nil || !strings.Contains(err.Error(), "must be bool") {
		t.Errorf("expected bool-condition error, got %v", err)
	}
}

// ---- exit ----

func TestExitReturnsSignal(t *testing.T) {
	_, err := run(t, `exit 7;`)
	if err == nil {
		t.Fatal("expected ExitSignal, got nil")
	}
	ex, ok := err.(*interpreter.ExitSignal)
	if !ok {
		t.Fatalf("expected *ExitSignal, got %T", err)
	}
	if ex.Code != 7 {
		t.Errorf("code = %d, want 7", ex.Code)
	}
}

func TestBareExitYieldsZero(t *testing.T) {
	_, err := run(t, `exit;`)
	ex, ok := err.(*interpreter.ExitSignal)
	if !ok {
		t.Fatalf("expected *ExitSignal, got %T (%v)", err, err)
	}
	if ex.Code != 0 {
		t.Errorf("code = %d, want 0", ex.Code)
	}
}

// `exit` inside a deeply nested method call terminates the whole
// program, not just the caller frame.
func TestExitFromMethodTerminatesProgram(t *testing.T) {
	out, err := run(t, `
use io;
func boom() {
    exit 3;
}
func wrap() {
    boom();
    io.printf("never\n");
}
io.printf("before ");
wrap();
io.printf("after ");
`)
	ex, ok := err.(*interpreter.ExitSignal)
	if !ok {
		t.Fatalf("expected *ExitSignal, got %T", err)
	}
	if ex.Code != 3 {
		t.Errorf("code = %d, want 3", ex.Code)
	}
	if out != "before " {
		t.Errorf("printed %q; lines after exit must NOT run", out)
	}
}

func TestExitNonIntErrors(t *testing.T) {
	_, err := run(t, `exit "bye";`)
	if err == nil || !strings.Contains(err.Error(), "must be int") {
		t.Errorf("expected int-required error, got %v", err)
	}
}
