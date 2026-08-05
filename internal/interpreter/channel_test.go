// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"bytes"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	channellib "jennifer-lang.dev/jennifer/internal/lib/channel"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	tasklib "jennifer-lang.dev/jennifer/internal/lib/task"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// runChan runs a program with io + task + channel installed.
func runChan(t *testing.T, src string) (string, error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	tasklib.Install(in)
	channellib.Install(in)
	runErr := in.Run(prog)
	return buf.String(), runErr
}

// TestChannelProducerConsumer: an unbuffered channel carries values from a
// producer spawn to the main consumer, which drains until close via try/catch.
func TestChannelProducerConsumer(t *testing.T) {
	out, err := runChan(t, `
use io;
use channel;
use task;
def ch as channel of int init channel.make(0);
def producer as task of int init spawn {
    def i as int init 0;
    while ($i < 5) { channel.send($ch, $i * 10); $i = $i + 1; }
    channel.close($ch);
    return 0;
};
def sum as int init 0;
try {
    while (true) { $sum = $sum + channel.recv($ch); }
} catch (e) {
}
task.wait($producer);
io.printf("sum=%d\n", $sum);
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "sum=100\n" {
		t.Errorf("out = %q, want sum=100", out)
	}
}

// TestChannelBufferedLenCap: a buffered channel reports len / capacity and
// preserves FIFO order.
func TestChannelBufferedLenCap(t *testing.T) {
	out, err := runChan(t, `
use io;
use channel;
def ch as channel of string init channel.make(3);
channel.send($ch, "a");
channel.send($ch, "b");
io.printf("len=%d cap=%d %s%s\n", channel.len($ch), channel.capacity($ch), channel.recv($ch), channel.recv($ch));
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "len=2 cap=3 ab\n" {
		t.Errorf("out = %q, want 'len=2 cap=3 ab'", out)
	}
}

// TestChannelValueSemantics: the value is copied at the send site, so a later
// mutation by the sender does not reach the receiver's copy.
func TestChannelValueSemantics(t *testing.T) {
	out, err := runChan(t, `
use io;
use channel;
def ch as channel of list of int init channel.make(1);
def xs as list of int init [1, 2, 3];
channel.send($ch, $xs);
$xs[0] = 999;
def got as list of int init channel.recv($ch);
io.printf("%v\n", $got);
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "[1, 2, 3]\n" {
		t.Errorf("out = %q, want [1, 2, 3] (send must copy)", out)
	}
}

// TestChannelSelectFanIn: select merges values from several channels and ends
// when all are closed.
func TestChannelSelectFanIn(t *testing.T) {
	out, err := runChan(t, `
use io;
use channel;
use task;
def a as channel of int init channel.make(0);
def b as channel of int init channel.make(0);
def pa as task of int init spawn { channel.send($a, 1); channel.close($a); return 0; };
def pb as task of int init spawn { channel.send($b, 2); channel.close($b); return 0; };
def total as int init 0;
def n as int init 0;
try {
    while (true) { $total = $total + channel.select([$a, $b]); $n = $n + 1; }
} catch (e) {
}
task.wait($pa); task.wait($pb);
io.printf("n=%d total=%d\n", $n, $total);
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "n=2 total=3\n" {
		t.Errorf("out = %q, want n=2 total=3", out)
	}
}

// TestChannelClosedErrors: send / close / recv on a closed channel are catchable,
// not fatal panics.
func TestChannelClosedErrors(t *testing.T) {
	out, err := runChan(t, `
use io;
use channel;
def ch as channel of int init channel.make(1);
channel.close($ch);
try { channel.send($ch, 1); } catch (e) { io.printf("send\n"); }
try { channel.close($ch); } catch (e) { io.printf("close\n"); }
try { channel.recv($ch); } catch (e) { io.printf("recv\n"); }
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "send\nclose\nrecv\n" {
		t.Errorf("out = %q, want send/close/recv", out)
	}
}

// TestChannelTypeChecks: a channel's element type is enforced at the binding.
func TestChannelTypeChecks(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"recv int into string binding", `use channel; def ch as channel of int init channel.make(1); channel.send($ch, 5); def s as string init channel.recv($ch);`},
		{"send wrong type errors at send site", `use channel; def ch as channel of int init channel.make(1); channel.send($ch, "x");`},
		{"channel of int to channel of string", `use channel; def ch as channel of int init channel.make(0); def bad as channel of string init $ch;`},
		{"make negative capacity", `use channel; def ch as channel of int init channel.make(-1);`},
		{"make huge capacity is catchable not a crash", `use channel; def ch as channel of int init channel.make(999999999999999);`},
		{"make over the capacity ceiling (OOM zone) is catchable", `use channel; def ch as channel of int init channel.make(10000000);`},
		{"send arity", `use channel; def ch as channel of int init channel.make(1); channel.send($ch);`},
		{"select on non-list", `use channel; channel.select(5);`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := runChan(t, c.src)
			if err == nil {
				t.Fatalf("expected an error")
			}
		})
	}
}

// TestChannelConcurrentBindNoRace: a generic channel held in a list (so its
// ElemTyp is unstamped) bound to a typed slot by several spawns concurrently must
// not race on ElemTyp. Meaningful under `go test -race`; a plain run just checks
// it completes. Regression guard for the set-once (CAS) stamp.
func TestChannelConcurrentBindNoRace(t *testing.T) {
	_, err := runChan(t, `
use channel;
use task;
def chans as list of channel of int init [channel.make(5)];
def a as task of int init spawn { def c as channel of int init $chans[0]; return 0; };
def b as task of int init spawn { def c as channel of int init $chans[0]; return 0; };
def d as task of int init spawn { def c as channel of int init $chans[0]; return 0; };
task.wait($a); task.wait($b); task.wait($d);
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestChannelIdentifierStillWorks: `channel` is a contextual keyword, so it stays
// usable as a struct field / parameter name (the amqp module relies on this).
func TestChannelIdentifierStillWorks(t *testing.T) {
	out, err := runChan(t, `
use io;
def struct Frame { channel as int, payload as int };
func onChannel(channel as int) { return $channel * 2; }
def f as Frame init Frame{channel: 7, payload: 3};
io.printf("%d %d\n", $f.channel, onChannel(20));
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "7 40\n" {
		t.Errorf("out = %q, want '7 40'", out)
	}
}
