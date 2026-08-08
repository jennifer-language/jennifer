// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package tasklib_test

import "testing"

// These drive the cancellation + bounded-wait builtins (cancel / cancelled /
// waitAnyTimeout + the requireMillis validation) through real spawned tasks,
// which the wait/poll tests do not reach.

// TestCancelAndCancelled: task.cancel signals a spawn; task.cancelled() polls
// the flag. Whether the explicit poll sees it first or the loop checkpoint
// raises the catchable "task cancelled" first, the task ends cancelled.
func TestCancelAndCancelled(t *testing.T) {
	out, runErr, unwaited := runProg(t, `
		use io;
		use task;
		def t as task of bool init spawn {
			try {
				while (true) {
					if (task.cancelled()) { return true; }
				}
			} catch (e) {
				return true;
			}
			return false;
		};
		task.cancel($t);
		def r as bool init task.wait($t);
		io.printf("cancelled=%t", $r);
	`)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if len(unwaited) != 0 {
		t.Errorf("unexpected unwaited errors: %v", unwaited)
	}
	if out != "cancelled=true" {
		t.Errorf("out = %q, want cancelled=true", out)
	}
}

// TestWaitAnyTimeoutSuccess: a task that finishes within the deadline returns
// its index.
func TestWaitAnyTimeoutSuccess(t *testing.T) {
	out, runErr, _ := runProg(t, `
		use io;
		use task;
		def t as task of int init spawn { return 7; };
		def i as int init task.waitAnyTimeout([$t], 2000);
		io.printf("idx=%d", $i);
	`)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if out != "idx=0" {
		t.Errorf("out = %q, want idx=0", out)
	}
}

// TestWaitAnyTimeoutTimesOut: a task still running when the timer fires makes
// waitAnyTimeout throw the catchable timeout error; the task is then cancelled
// and observed so exit stays clean.
func TestWaitAnyTimeoutTimesOut(t *testing.T) {
	out, runErr, unwaited := runProg(t, `
		use io;
		use task;
		def t as task of int init spawn {
			try {
				while (true) { }
			} catch (e) {
			}
			return 0;
		};
		try {
			def i as int init task.waitAnyTimeout([$t], 20);
			io.printf("no timeout");
		} catch (e) {
			io.printf("timeout ");
		}
		task.cancel($t);
		def r as int init task.wait($t);
		io.printf("done=%t", $r >= 0);
	`)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if len(unwaited) != 0 {
		t.Errorf("unexpected unwaited errors: %v", unwaited)
	}
	if out != "timeout done=true" {
		t.Errorf("out = %q, want \"timeout done=true\"", out)
	}
}

// TestWaitAnyTimeoutArgErrors: the millisecond argument and the task list are
// validated (requireMillis + empty-list guard).
func TestWaitAnyTimeoutArgErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"negative ms", `
			use task;
			def t as task of int init spawn { return 1; };
			def w as int init task.wait($t);
			def bad as int init task.waitAnyTimeout([$t], -5);
		`},
		{"non-int ms", `
			use task;
			def t as task of int init spawn { return 1; };
			def w as int init task.wait($t);
			def bad as int init task.waitAnyTimeout([$t], "soon");
		`},
		{"empty list", `
			use task;
			def none as list of task of int;
			def i as int init task.waitAnyTimeout($none, 10);
		`},
	}
	for _, c := range cases {
		if _, runErr, _ := runProg(t, c.src); runErr == nil {
			t.Errorf("%s: expected a runtime error, got nil", c.name)
		}
	}
}
