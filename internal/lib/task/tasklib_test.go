// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package tasklib_test

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	tasklib "jennifer-lang.dev/jennifer/internal/lib/task"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// runProg drives one Phase 3 end-to-end scenario: parse, run,
// drain task registry. Returns captured stdout, runErr from the
// interpreter, and any unwaited task errors surfaced at exit.
func runProg(t *testing.T, src string) (string, error, []error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err, nil
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	tasklib.Install(in)
	runErr := in.Run(prog)
	unwaited := in.UnwaitedTaskErrors()
	return buf.String(), runErr, unwaited
}

// TestWaitReturnsResult: the canonical success path. wait blocks
// until the spawn body finishes, returns the value, the loud-fail
// scan reports nothing.
func TestWaitReturnsResult(t *testing.T) {
	out, runErr, unwaited := runProg(t, `
		use io;
		use task;
		def t as task of int init spawn { return 42; };
		def n as int init task.wait($t);
		io.printf("%d", $n);
	`)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if len(unwaited) != 0 {
		t.Errorf("unexpected unwaited errors: %v", unwaited)
	}
	if out != "42" {
		t.Errorf("got %q, want %q", out, "42")
	}
}

// TestWaitRethrowsError: a runtime error inside the spawn body
// becomes the wait's returned error, which propagates as a runtime
// error at the wait site.
func TestWaitRethrowsError(t *testing.T) {
	_, runErr, _ := runProg(t, `
		use task;
		def t as task of int init spawn {
			def xs as list of int init [];
			return $xs[5];
		};
		def n as int init task.wait($t);
	`)
	if runErr == nil {
		t.Fatal("expected runtime error to surface at the wait site")
	}
	if !strings.Contains(runErr.Error(), "out of bounds") {
		t.Errorf("error doesn't mention the body's failure: %v", runErr)
	}
}

// TestWaitMarksObserved: a wait that returns successfully suppresses
// the exit-time loud-fail even when other tasks in the same program
// don't get waited on. Wait counts as observation in both success
// and rethrow branches; this case exercises the success branch.
func TestWaitMarksObserved(t *testing.T) {
	_, runErr, unwaited := runProg(t, `
		use task;
		def t as task of int init spawn { return 1; };
		def n as int init task.wait($t);
	`)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if len(unwaited) != 0 {
		t.Errorf("loud-fail should be quiet after a successful wait, got: %v", unwaited)
	}
}

// TestWaitMarksObservedOnRethrow: even when wait re-raises an error,
// the task counts as observed. The exit-time scan sees the task was
// already reported via wait, not again via the loud-fail.
func TestWaitMarksObservedOnRethrow(t *testing.T) {
	_, runErr, unwaited := runProg(t, `
		use task;
		def t as task of int init spawn {
			def xs as list of int init [];
			return $xs[5];
		};
		try {
			def n as int init task.wait($t);
		} catch (e) { }   # swallow the rethrow
	`)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if len(unwaited) != 0 {
		t.Errorf("wait+catch should suppress the loud-fail, got: %v", unwaited)
	}
}

// TestPollAfterWait: task.poll returns true once the task is done.
// Before is technically racy with the goroutine; wait first to make
// the test deterministic.
func TestPollAfterWait(t *testing.T) {
	out, runErr, _ := runProg(t, `
		use io;
		use task;
		def t as task of int init spawn { return 1; };
		def n as int init task.wait($t);
		io.printf("%t", task.poll($t));
	`)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if out != "true" {
		t.Errorf("poll after wait = %q, want %q", out, "true")
	}
}

// TestDiscardSuppressesLoudFail: task.discard turns an error-producing
// task into fire-and-forget so it doesn't trip the exit-time scan.
func TestDiscardSuppressesLoudFail(t *testing.T) {
	_, runErr, unwaited := runProg(t, `
		use task;
		def t as task of int init spawn {
			def xs as list of int init [];
			return $xs[5];
		};
		task.discard($t);
	`)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if len(unwaited) != 0 {
		t.Errorf("discard should suppress loud-fail, got: %v", unwaited)
	}
}

// TestWaitAllReturnsResultsInOrder: list of task -> list of T, with
// results in list order regardless of completion order.
func TestWaitAllReturnsResultsInOrder(t *testing.T) {
	out, runErr, _ := runProg(t, `
		use io;
		use task;
		def a as task of int init spawn { return 1; };
		def b as task of int init spawn { return 2; };
		def c as task of int init spawn { return 3; };
		def ts as list of task of int init [$a, $b, $c];
		def xs as list of int init task.waitAll($ts);
		io.printf("%d-%d-%d", $xs[0], $xs[1], $xs[2]);
	`)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if out != "1-2-3" {
		t.Errorf("got %q, want %q", out, "1-2-3")
	}
}

// TestWaitAllPropagatesFirstError: if any task errored, waitAll
// drains every survivor (so the loud-fail is quiet) and then
// re-raises the first error in list order at the call site.
func TestWaitAllPropagatesFirstError(t *testing.T) {
	_, runErr, unwaited := runProg(t, `
		use task;
		def ok as task of int init spawn { return 1; };
		def bad as task of int init spawn {
			def xs as list of int init [];
			return $xs[9];
		};
		def alsoOk as task of int init spawn { return 2; };
		def ts as list of task of int init [$ok, $bad, $alsoOk];
		def results as list of int init task.waitAll($ts);
	`)
	if runErr == nil {
		t.Fatal("expected the bad task's error to propagate")
	}
	if !strings.Contains(runErr.Error(), "out of bounds") {
		t.Errorf("error doesn't mention out-of-bounds: %v", runErr)
	}
	if len(unwaited) != 0 {
		t.Errorf("survivors should have been drained + observed, but loud-fail saw: %v", unwaited)
	}
}

// TestWaitAnyReturnsIndex: waitAny blocks until any task is done and
// returns the index. Caller then waits on that task; the others
// remain to be observed (or hit the loud-fail).
func TestWaitAnyReturnsIndex(t *testing.T) {
	out, runErr, _ := runProg(t, `
		use io;
		use task;
		def only as task of int init spawn { return 42; };
		def ts as list of task of int init [$only];
		def idx as int init task.waitAny($ts);
		def val as int init task.wait($ts[$idx]);
		io.printf("idx=%d val=%d", $idx, $val);
	`)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if out != "idx=0 val=42" {
		t.Errorf("got %q, want %q", out, "idx=0 val=42")
	}
}

// TestWaitAnyEmptyListErrors: waitAny on an empty list has no task
// to wait on; surfaces a positioned runtime error.
func TestWaitAnyEmptyListErrors(t *testing.T) {
	_, runErr, _ := runProg(t, `
		use task;
		def ts as list of task of int init [];
		def idx as int init task.waitAny($ts);
	`)
	if runErr == nil {
		t.Fatal("expected error for empty list")
	}
	if !strings.Contains(runErr.Error(), "empty") {
		t.Errorf("error doesn't mention empty list: %v", runErr)
	}
}

// TestTaskNonTaskArgRejected: passing a non-task to wait / poll /
// discard surfaces at the boundary.
func TestTaskNonTaskArgRejected(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		expect string
	}{
		{
			name: "wait",
			src: `
				use task;
				def x as int init task.wait(42);
			`,
			expect: "task.wait",
		},
		{
			name: "poll",
			src: `
				use task;
				def b as bool init task.poll(42);
			`,
			expect: "task.poll",
		},
		{
			name: "discard",
			src: `
				use task;
				task.discard(42);
			`,
			expect: "task.discard",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, runErr, _ := runProg(t, tc.src)
			if runErr == nil {
				t.Fatal("expected boundary error")
			}
			if !strings.Contains(runErr.Error(), tc.expect) {
				t.Errorf("error doesn't mention %q: %v", tc.expect, runErr)
			}
		})
	}
}

// TestTaskKeywordInTypeStillWorks confirms the parser tweak for
// `task.IDENT` didn't break the type-position role of the same
// keyword.
func TestTaskKeywordInTypeStillWorks(t *testing.T) {
	_, runErr, _ := runProg(t, `
		use task;
		def t as task of int init spawn { return 5; };
		def n as int init task.wait($t);
	`)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
}

// TestBareTaskInExpressionPositionRejected: `task` without a
// following `.method(...)` in expression position is a parse error
// pointing the user at the two valid uses.
func TestBareTaskInExpressionPositionRejected(t *testing.T) {
	_, runErr, _ := runProg(t, `
		def x as int init task + 1;
	`)
	if runErr == nil {
		t.Fatal("expected parse error for bare `task`")
	}
	if !strings.Contains(runErr.Error(), "task of T") && !strings.Contains(runErr.Error(), "method") {
		t.Errorf("error doesn't guide the user: %v", runErr)
	}
}
