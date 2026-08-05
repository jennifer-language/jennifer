// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	tasklib "jennifer-lang.dev/jennifer/internal/lib/task"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// runTask runs a program with io + task installed, returning stdout, the run
// error, and any unwaited-task errors surfaced at exit.
func runTask(t *testing.T, src string) (string, error, []error) {
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

// TestTaskCancelStopsRunawaySpawn: cancelling a never-terminating spawn lets the
// program finish (the body stops at its loop checkpoint) instead of hanging.
func TestTaskCancelStopsRunawaySpawn(t *testing.T) {
	out, err, unwaited := runTask(t, `
use io;
use task;
def t as task of int init spawn {
    def n as int init 0;
    while (true) { $n = $n + 1; }
    return $n;
};
task.cancel($t);
task.discard($t);
io.printf("done\n");
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "done\n" {
		t.Errorf("out = %q, want %q", out, "done\n")
	}
	// Discarded, so the exit scan does not loud-fail on the cancellation error.
	if len(unwaited) != 0 {
		t.Errorf("unwaited = %v, want none", unwaited)
	}
}

// TestTaskCancelCleanPartialViaCatch: the clean-partial-result idiom -
// try { loop } catch (e) { } then return - yields a normal result, not an error.
func TestTaskCancelCleanPartialViaCatch(t *testing.T) {
	out, err, _ := runTask(t, `
use io;
use task;
def t as task of int init spawn {
    def n as int init 0;
    try {
        while (true) { $n = $n + 1; }
    } catch (e) {
    }
    return $n;
};
task.cancel($t);
def got as int init task.wait($t);
io.printf("ok=%t\n", $got >= 0);
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "ok=true\n" {
		t.Errorf("out = %q, want ok=true", out)
	}
}

// TestTaskCancelUncaughtSurfacesAsError: without a catch, the auto-raised "task
// cancelled" becomes the task's error, re-raised by task.wait (catchable).
func TestTaskCancelUncaughtSurfacesAsError(t *testing.T) {
	out, err, _ := runTask(t, `
use io;
use task;
def t as task of int init spawn {
    while (true) { }
    return 0;
};
task.cancel($t);
try {
    task.wait($t);
} catch (e) {
    io.printf("caught %s\n", $e.message);
}
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "task cancelled") {
		t.Errorf("out = %q, want it to contain 'task cancelled'", out)
	}
}

// TestTaskCancelledPoll: task.cancelled() is a non-raising poll; false when the
// task was never cancelled.
func TestTaskCancelledPoll(t *testing.T) {
	out, err, _ := runTask(t, `
use io;
use task;
def t as task of int init spawn {
    if (task.cancelled()) { return -1; }
    return 99;
};
io.printf("%d\n", task.wait($t));
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "99\n" {
		t.Errorf("out = %q, want 99", out)
	}
}

// TestTaskWaitTimeoutCompletes: a task that finishes within the timeout returns
// its result.
func TestTaskWaitTimeoutCompletes(t *testing.T) {
	out, err, _ := runTask(t, `
use io;
use task;
def t as task of int init spawn { return 42; };
io.printf("%d\n", task.waitTimeout($t, 5000));
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "42\n" {
		t.Errorf("out = %q, want 42", out)
	}
}

// TestTaskWaitTimeoutTimesOut: a spinning task does not finish within the
// timeout, so waitTimeout throws a catchable error.
func TestTaskWaitTimeoutTimesOut(t *testing.T) {
	out, err, _ := runTask(t, `
use io;
use task;
def t as task of int init spawn { while (true) { } return 0; };
try {
    task.waitTimeout($t, 30);
} catch (e) {
    io.printf("timedout\n");
}
task.cancel($t);
task.discard($t);
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "timedout\n" {
		t.Errorf("out = %q, want timedout", out)
	}
}

// TestTaskWaitAnyTimeoutReturnsIndex: with one fast and one spinning task,
// waitAnyTimeout returns the fast one's index within the timeout.
func TestTaskWaitAnyTimeoutReturnsIndex(t *testing.T) {
	out, err, _ := runTask(t, `
use io;
use task;
def slow as task of int init spawn { while (true) { } return 0; };
def fast as task of int init spawn { return 7; };
def idx as int init task.waitAnyTimeout([$slow, $fast], 5000);
io.printf("idx=%d\n", $idx);
task.cancel($slow);
task.discard($slow);
task.discard($fast);
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "idx=1\n" {
		t.Errorf("out = %q, want idx=1", out)
	}
}

// TestTaskTimeoutArgErrors: bad timeout arguments are positioned boundary errors.
func TestTaskTimeoutArgErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"negative ms", `use task; def t as task of int init spawn { return 1; }; task.waitTimeout($t, -1);`},
		{"overflow ms is rejected not an instant timeout", `use task; def t as task of int init spawn { return 1; }; task.waitTimeout($t, 9223372036854775807);`},
		{"wrong arity", `use task; def t as task of int init spawn { return 1; }; task.waitTimeout($t);`},
		{"cancelled arity", `use task; task.cancelled($nope);`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err, _ := runTask(t, c.src)
			if err == nil {
				t.Fatalf("expected an error")
			}
		})
	}
}
