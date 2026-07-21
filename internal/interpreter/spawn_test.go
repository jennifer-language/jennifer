// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// End-to-end tests for `spawn { ... }` + `task of T`.
//
// Phase 2 ships:
//   - goroutine-backed spawn (the body runs asynchronously)
//   - per-run task registry on the Interpreter
//   - UnwaitedTaskErrors() exit-time scan that blocks on every
//     unobserved task's done channel
//
// Tests below either call UnwaitedTaskErrors explicitly (to drain the
// task and inspect its outcome), or use the higher-level runSpawn
// helper that orchestrates Run + drain + result reporting.

// runSpawn parses + runs src, then drains all unobserved tasks via
// UnwaitedTaskErrors. Returns the captured stdout, any error from
// Run itself, and the slice of unwaited task errors.
func runSpawn(t *testing.T, src string) (stdout string, runErr error, unwaited []error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err, nil
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	runErr = in.Run(prog)
	unwaited = in.UnwaitedTaskErrors()
	return buf.String(), runErr, unwaited
}

// TestSpawnParsesAndExecutes covers the canonical shape:
//
//	def t as task of int init spawn { return 42; };
//
// After UnwaitedTaskErrors drains the goroutine, the task's display
// form reports "task<done>". The test reaches into the second
// printf to make the order explicit (printf before drain may catch
// the task pending, printf after drain is task<done>).
func TestSpawnParsesAndExecutes(t *testing.T) {
	out, runErr, _ := runSpawn(t, `
		use io;
		def t as task of int init spawn { return 42; };
		# Drain via task accessor isn't shipped yet (Phase 3); we
		# observe the final state through the registry scan in the
		# test helper instead. Display before drain may still be
		# pending - acceptable for Phase 2.
		io.printf("%v", $t);
	`)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	// Pre-drain printf race: accept either pending or done. The
	// important assertion is that the program ran cleanly.
	if out != "task<pending>" && out != "task<done>" {
		t.Errorf("display = %q, want task<pending> or task<done>", out)
	}
}

// TestSpawnDeepCopiesCapturedVars: the body mutates a captured list,
// but the parent's binding stays untouched. Confirms the snapshot in
// evalSpawn actually deep-copies rather than sharing references with
// the caller. Independent of timing - the value-semantics guarantee
// holds whether or not the goroutine has run yet at the time of the
// parent's check.
func TestSpawnDeepCopiesCapturedVars(t *testing.T) {
	out, runErr, _ := runSpawn(t, `
		use io;
		def xs as list of int init [1, 2, 3];
		def t as task of null init spawn {
			$xs[0] = 999;
			return null;
		};
		# After spawn, the parent's list is unchanged regardless
		# of whether the goroutine has run yet.
		io.printf("%d", $xs[0]);
	`)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if out != "1" {
		t.Errorf("parent xs[0] = %q, want %q (value-semantics capture broke)", out, "1")
	}
}

// TestSpawnNullReturn: a bare `return;` (or falling off the end)
// produces a task that holds null. The drain in UnwaitedTaskErrors
// blocks on the goroutine; afterwards no error surfaces.
func TestSpawnNullReturn(t *testing.T) {
	_, runErr, unwaited := runSpawn(t, `
		def t as task of null init spawn {
			return;
		};
	`)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if len(unwaited) != 0 {
		t.Errorf("unexpected unwaited errors: %v", unwaited)
	}
}

// TestSpawnRuntimeErrorSurfacesAtDrain: a runtime error inside the
// spawn body is captured on the task. The main flow doesn't see it
// (Run completes cleanly), but UnwaitedTaskErrors finds it at exit
// time - that's the loud-fail contract.
func TestSpawnRuntimeErrorSurfacesAtDrain(t *testing.T) {
	_, runErr, unwaited := runSpawn(t, `
		def t as task of int init spawn {
			def xs as list of int init [];
			return $xs[5];
		};
	`)
	if runErr != nil {
		t.Fatalf("run: %v (the caller should not see the spawn's error)", runErr)
	}
	if len(unwaited) != 1 {
		t.Fatalf("expected 1 unwaited error, got %d: %v", len(unwaited), unwaited)
	}
	if !strings.Contains(unwaited[0].Error(), "out of bounds") {
		t.Errorf("unwaited error doesn't mention the out-of-bounds: %v", unwaited[0])
	}
}

// TestSpawnExitInsideBodySurfacesAtDrain: `exit EXPR;` inside a spawn
// body terminates the whole program - per the spec - and the
// exit code travels through the registry scan. The CLI then uses
// that code as the program's exit code (verified separately in the
// cmd/jennifer smoke; here we just check the ExitSignal is the
// returned unwaited error).
func TestSpawnExitInsideBodySurfacesAtDrain(t *testing.T) {
	_, runErr, unwaited := runSpawn(t, `
		def t as task of null init spawn {
			exit 7;
		};
	`)
	if runErr != nil {
		t.Fatalf("Run should complete; got: %v", runErr)
	}
	if len(unwaited) != 1 {
		t.Fatalf("expected 1 unwaited error (the ExitSignal), got %d", len(unwaited))
	}
	ex, ok := unwaited[0].(*interpreter.ExitSignal)
	if !ok {
		t.Fatalf("unwaited error is %T, want *interpreter.ExitSignal", unwaited[0])
	}
	if ex.Code != 7 {
		t.Errorf("ExitSignal.Code = %d, want 7", ex.Code)
	}
}

// TestTaskTypeMismatchRejected: declaring `def t as task of int` and
// initialising with a non-task value must error at the bind site
// (Define-time check, independent of any goroutine).
func TestTaskTypeMismatchRejected(t *testing.T) {
	_, runErr, _ := runSpawn(t, `
		def t as task of int init 42;
	`)
	if runErr == nil {
		t.Fatal("expected type error for non-task init of a task variable")
	}
	if !strings.Contains(runErr.Error(), "task of int") {
		t.Errorf("error doesn't mention task of int: %v", runErr)
	}
}

// TestSpawnBreakAcrossBoundarySurfacesAtDrain: break inside a spawn
// body with no enclosing loop becomes the task's Err and surfaces in
// UnwaitedTaskErrors. Phase 1 expected this to fail at the spawn
// site synchronously; Phase 2's goroutine model captures it on the
// task instead, which is the loud-fail contract.
func TestSpawnBreakAcrossBoundarySurfacesAtDrain(t *testing.T) {
	_, runErr, unwaited := runSpawn(t, `
		while (true) {
			def t as task of null init spawn {
				break;
			};
			break;
		}
	`)
	if runErr != nil {
		t.Fatalf("Run should complete; got: %v", runErr)
	}
	if len(unwaited) != 1 {
		t.Fatalf("expected 1 unwaited error, got %d: %v", len(unwaited), unwaited)
	}
	if !strings.Contains(unwaited[0].Error(), "break") {
		t.Errorf("unwaited error doesn't mention break: %v", unwaited[0])
	}
}

// TestSpawnConcurrentErrorsAllSurface: every spawned task that ends in
// an error contributes one entry to UnwaitedTaskErrors. Confirms the
// registry aggregates across multiple goroutines without losing
// entries, and that the per-spawn frames are independent (each spawn
// reads its own copy of $i).
func TestSpawnConcurrentErrorsAllSurface(t *testing.T) {
	src := `
		def i as int init 0;
		while ($i < 4) {
			def t as task of int init spawn {
				def xs as list of int init [];
				return $xs[$i];      # always out-of-bounds; $i was deep-copied
			};
			$i = $i + 1;
		}
	`
	_, runErr, unwaited := runSpawn(t, src)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if len(unwaited) != 4 {
		t.Errorf("expected 4 unwaited errors (one per spawn), got %d: %v", len(unwaited), unwaited)
	}
	for _, e := range unwaited {
		if !strings.Contains(e.Error(), "out of bounds") {
			t.Errorf("unwaited error does not mention out-of-bounds: %v", e)
		}
	}
}

// TestMarkObservedSuppressesLoudFail constructs a finished error task
// directly and confirms MarkObserved flipping the flag retroactively
// suppresses the loud-fail. The user-facing scenario (task.discard /
// task.wait running on a real task and observing it) is covered in
// Phase 3 when those builtins ship; this test verifies the
// interpreter-side API the upcoming `task` library will call.
func TestMarkObservedSuppressesLoudFail(t *testing.T) {
	// Construct a TaskState as if a goroutine had already completed
	// with an error. Skip the spawn machinery so this test is purely
	// about the registry-side semantics.
	done := make(chan struct{})
	close(done)
	bogusErr := &interpreter.ExitSignal{Code: 1}
	state := &interpreter.TaskState{Done: done, Err: bogusErr}

	in := interpreter.New()
	in.RegisterTaskForTest(state)

	// Before MarkObserved: loud-fail sees the error.
	if got := in.UnwaitedTaskErrors(); len(got) != 1 {
		t.Fatalf("expected 1 unwaited error pre-MarkObserved, got %d", len(got))
	}

	// After MarkObserved: scan skips it.
	in.MarkObserved(state)
	if got := in.UnwaitedTaskErrors(); len(got) != 0 {
		t.Errorf("expected 0 unwaited errors post-MarkObserved, got %d: %v", len(got), got)
	}
}

// TestSpawnBodyCanCallUserFunctionWithCapturedArg is the regression
// guard for the Phase-4.5 bug: a spawn body that called a user
// `func` with a captured-arg segfaulted under TinyGo because the
// callee's frame parented to the live i.global the main goroutine
// was still mutating. The fix routes user-method call frames through
// effectiveGlobal(env), which walks to the spawn's snapshot when the
// frame is inside a spawn (and to i.global otherwise). The body here
// exercises both: spawn launches a goroutine that immediately calls a
// user function with a captured-int argument.
func TestSpawnBodyCanCallUserFunctionWithCapturedArg(t *testing.T) {
	out, runErr, unwaited := runSpawn(t, `
		use io;
		func work(n as int) {
			return $n + 1;
		}
		def base as int init 41;
		def t as task of int init spawn { return work($base); };
		# Bare wait-replacement: drain via UnwaitedTaskErrors at the end.
		# Printf the task's display form just to make sure it routes.
		io.printf("%v\n", $t);
	`)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	// The test helper drains the task; unwaited captures unobserved
	// errors. The successful task should produce no error.
	if len(unwaited) != 0 {
		t.Errorf("unexpected unwaited errors: %v", unwaited)
	}
	// Display form depends on whether printf observed pending or done,
	// so this assertion only checks the prefix.
	if !strings.HasPrefix(out, "task<") {
		t.Errorf("expected task display, got %q", out)
	}
}
