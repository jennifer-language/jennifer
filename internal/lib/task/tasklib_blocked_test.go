// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package tasklib_test

import (
	"bytes"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	channellib "jennifer-lang.dev/jennifer/internal/lib/channel"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	tasklib "jennifer-lang.dev/jennifer/internal/lib/task"
	timelib "jennifer-lang.dev/jennifer/internal/lib/time"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// runBounded runs src with io, task, channel and time installed and fails the
// test if it does not finish within limit (a blocked builtin that ignores
// task.cancel hangs forever).
func runBounded(t *testing.T, src string, limit time.Duration) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	tasklib.Install(in)
	channellib.Install(in)
	timelib.Install(in)
	done := make(chan error, 1)
	go func() { done <- in.Run(prog) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(limit):
		t.Fatalf("program still blocked after %v (output so far: %q)", limit, buf.String())
	}
	return buf.String()
}

// TestWaitTimeoutZeroOnFinishedTask: waitTimeout($t, 0) on a finished task
// returns its result every time. Before the fix the done and timer cases were
// both ready and select picked one at random (about half threw "timeout").
func TestWaitTimeoutZeroOnFinishedTask(t *testing.T) {
	out := runBounded(t, `
		use io;
		use task;
		def t as task of int init spawn { return 7; };
		def first as int init task.wait($t);
		def timeouts as int init 0;
		def sum as int init 0;
		for (def i as int init 0; $i < 300; $i = $i + 1) {
			try {
				$sum = $sum + task.waitTimeout($t, 0);
			} catch (e) {
				$timeouts = $timeouts + 1;
			}
		}
		io.printf("timeouts=%d sum=%d", $timeouts, $sum);
	`, 10*time.Second)
	if out != "timeouts=0 sum=2100" {
		t.Errorf("out = %q, want timeouts=0 sum=2100", out)
	}
}

// TestCancelInterruptsBlockedBuiltins: task.cancel wakes a body blocked in
// time.sleep, channel.recv / send / select or task.wait; the call raises the
// catchable "task cancelled" the loop checkpoint raises.
func TestCancelInterruptsBlockedBuiltins(t *testing.T) {
	cases := []struct {
		name string
		call string
	}{
		{"sleep", `time.sleep(time.fromSeconds(3600));`},
		{"recv", `def v as int init channel.recv($ch);`},
		{"send", `channel.send($ch, 1);`},
		{"select", `def v as int init channel.select([$ch]);`},
		{"wait", `def v as int init task.wait($inner);`},
		{"waitAll", `def v as list of int init task.waitAll([$inner]);`},
		{"waitAny", `def v as int init task.waitAny([$inner]);`},
		{"waitTimeout", `def v as int init task.waitTimeout($inner, 3600000);`},
		{"waitAnyTimeout", `def v as int init task.waitAnyTimeout([$inner], 3600000);`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := runBounded(t, `
				use io;
				use task;
				use channel;
				use time;
				def ch as channel of int init channel.make(0);
				def gate as channel of int init channel.make(0);
				def inner as task of int init spawn {
					try { def g as int init channel.recv($gate); } catch (e) { }
					return 0;
				};
				def t as task of string init spawn {
					try {
						`+c.call+`
						return "returned";
					} catch (e) {
						return $e.message;
					}
				};
				time.sleep(time.fromMilliseconds(20));
				task.cancel($t);
				io.printf("%s", task.wait($t));
				channel.close($gate);
				def r as int init task.wait($inner);
			`, 5*time.Second)
			if out != "task cancelled" {
				t.Errorf("out = %q, want \"task cancelled\"", out)
			}
		})
	}
}

// TestCancelBeforeBlockingCall: a task cancelled before it reaches a blocking
// call raises at that call instead of blocking.
func TestCancelBeforeBlockingCall(t *testing.T) {
	out := runBounded(t, `
		use io;
		use task;
		use channel;
		def ch as channel of int init channel.make(0);
		def start as channel of int init channel.make(1);
		def t as task of string init spawn {
			def g as int init channel.recv($start);
			try {
				def v as int init channel.recv($ch);
				return "returned";
			} catch (e) {
				return $e.message;
			}
		};
		task.cancel($t);
		channel.send($start, 1);
		io.printf("%s", task.wait($t));
	`, 5*time.Second)
	if out != "task cancelled" {
		t.Errorf("out = %q, want \"task cancelled\"", out)
	}
}

// TestWaitPrefersFinishedTaskOverCancel: a cancelled waiter still takes the
// result of a task that has already finished.
func TestWaitPrefersFinishedTaskOverCancel(t *testing.T) {
	out := runBounded(t, `
		use io;
		use task;
		def inner as task of int init spawn { return 5; };
		def r as int init task.wait($inner);
		def u as task of int init spawn { return task.wait($inner) + task.waitTimeout($inner, 0); };
		task.cancel($u);
		def got as int init 0;
		try { $got = task.wait($u); } catch (e) { $got = -1; }
		io.printf("%d", $got);
	`, 5*time.Second)
	// u has no loop checkpoint, and inner is finished, so both waits return
	// whether or not the cancel lands first.
	if out != "10" {
		t.Errorf("out = %q, want 10", out)
	}
}
