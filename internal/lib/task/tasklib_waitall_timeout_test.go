// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package tasklib_test

import (
	"strings"
	"testing"
	"time"
)

// M29.45: task.waitAllTimeout is task.waitAll bounded by one deadline.

// TestWaitAllTimeoutResults: every task finishes in time; the results come
// back in list order and nothing is left unobserved.
func TestWaitAllTimeoutResults(t *testing.T) {
	out := runBounded(t, `
		use io;
		use task;
		def ts as list of task of int init [];
		for (def i as int init 0; $i < 5; $i = $i + 1) {
			def n as int init $i;
			$ts[] = spawn { return $n * 10; };
		}
		def rs as list of int init task.waitAllTimeout($ts, 5000);
		io.printf("%d %d %d", len($rs), $rs[0], $rs[4]);
	`, 10*time.Second)
	if out != "5 0 40" {
		t.Errorf("out = %q, want \"5 0 40\"", out)
	}
}

// TestWaitAllTimeoutLeavesTasksRunning: on timeout the tasks keep running and
// stay unobserved, so a later waitAll still returns every result.
func TestWaitAllTimeoutLeavesTasksRunning(t *testing.T) {
	out := runBounded(t, `
		use io;
		use task;
		use channel;
		def gate as channel of int init channel.make(0);
		def fast as task of int init spawn { return 1; };
		def slow as task of int init spawn { def g as int init channel.recv($gate); return 2; };
		def kind as string init "none";
		try {
			def rs as list of int init task.waitAllTimeout([$fast, $slow], 30);
		} catch (e) {
			$kind = $e.kind;
		}
		def polled as bool init task.poll($slow);
		channel.send($gate, 1);
		def rs as list of int init task.waitAll([$fast, $slow]);
		io.printf("%s %t %d %d", $kind, $polled, $rs[0], $rs[1]);
	`, 10*time.Second)
	if out != "timeout false 1 2" {
		t.Errorf("out = %q, want \"timeout false 1 2\"", out)
	}
}

// TestWaitAllTimeoutRethrows: a body error is re-raised after every task is
// observed, as task.waitAll does.
func TestWaitAllTimeoutRethrows(t *testing.T) {
	out, runErr, unwaited := runProg(t, `
		use io;
		use task;
		def a as task of int init spawn { throw Error{kind: "app", message: "boom"}; };
		def b as task of int init spawn { return 2; };
		try {
			def rs as list of int init task.waitAllTimeout([$a, $b], 5000);
		} catch (e) {
			io.printf("%s %s", $e.kind, $e.message);
		}
	`)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if len(unwaited) != 0 {
		t.Errorf("unwaited errors: %v", unwaited)
	}
	if out != "app boom" {
		t.Errorf("out = %q, want \"app boom\"", out)
	}
}

// TestWaitAllTimeoutZeroPolls: ms 0 returns the results of tasks that are
// already finished, every time (no random timeout from a ready timer), and
// an empty list returns an empty list.
func TestWaitAllTimeoutZeroPolls(t *testing.T) {
	out := runBounded(t, `
		use io;
		use task;
		def a as task of int init spawn { return 3; };
		def b as task of int init spawn { return 4; };
		def c as task of int init spawn { return 5; };
		def first as list of int init task.waitAll([$a, $b, $c]);
		def timeouts as int init 0;
		for (def i as int init 0; $i < 200; $i = $i + 1) {
			try {
				def rs as list of int init task.waitAllTimeout([$a, $b, $c], 0);
			} catch (e) {
				$timeouts = $timeouts + 1;
			}
		}
		def none as list of int init task.waitAllTimeout([], 0);
		io.printf("timeouts=%d empty=%d", $timeouts, len($none));
	`, 10*time.Second)
	if out != "timeouts=0 empty=0" {
		t.Errorf("out = %q", out)
	}
}

// TestTimeoutBounds: every task xTimeout takes 0 to ~9.2e12 ms; a negative or
// larger value is a "runtime" error.
func TestTimeoutBounds(t *testing.T) {
	for _, call := range []string{
		"task.waitTimeout($t, %s)",
		"task.waitAnyTimeout([$t], %s)",
		"task.waitAllTimeout([$t], %s)",
	} {
		for _, ms := range []string{"-1", "9223372036855"} {
			src := `
				use io;
				use task;
				def t as task of int init spawn { return 1; };
				def r as int init task.wait($t);
				try {
					` + strings.Replace(call, "%s", ms, 1) + `;
					io.printf("no error");
				} catch (e) {
					io.printf("%s %s", $e.kind, $e.message);
				}
			`
			out := runBounded(t, src, 5*time.Second)
			if !strings.HasPrefix(out, "runtime ") || !strings.Contains(out, "timeout (ms)") {
				t.Errorf("%s with %s: out = %q", call, ms, out)
			}
		}
	}
}
