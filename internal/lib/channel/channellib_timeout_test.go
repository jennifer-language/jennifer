// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package channellib

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	tasklib "jennifer-lang.dev/jennifer/internal/lib/task"
	timelib "jennifer-lang.dev/jennifer/internal/lib/time"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// TestRecvTimeoutReturnsAWaitingValue: a value already in the channel comes
// back at once, even with ms 0 (a poll).
func TestRecvTimeoutReturnsAWaitingValue(t *testing.T) {
	in := newLib(t)
	ch := makeChan(t, in, 2)
	mustInvoke(t, in, "send", ch, interpreter.IntVal(5))
	mustInvoke(t, in, "send", ch, interpreter.IntVal(6))
	if v := mustInvoke(t, in, "recvTimeout", ch, interpreter.IntVal(0)); v.Int != 5 {
		t.Errorf("recvTimeout(ch, 0) = %d, want 5", v.Int)
	}
	if v := mustInvoke(t, in, "recvTimeout", ch, interpreter.IntVal(1000)); v.Int != 6 {
		t.Errorf("recvTimeout(ch, 1000) = %d, want 6", v.Int)
	}
}

// TestRecvTimeoutTimesOut: an empty channel throws after about ms (its kind,
// "timeout", is checked in TestRecvTimeoutInAProgram), and takes nothing: a value sent afterwards is still there.
func TestRecvTimeoutTimesOut(t *testing.T) {
	in := newLib(t)
	ch := makeChan(t, in, 1)
	start := time.Now()
	_, err := invoke(t, in, "recvTimeout", ch, interpreter.IntVal(80))
	took := time.Since(start)
	if err == nil || !strings.Contains(err.Error(), "channel.recvTimeout: timed out after 80 ms") {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if took < 70*time.Millisecond || took > 2*time.Second {
		t.Errorf("took %v, want about 80 ms", took)
	}
	_, err = invoke(t, in, "recvTimeout", ch, interpreter.IntVal(0))
	if err == nil || !strings.Contains(err.Error(), "timed out after 0 ms") {
		t.Errorf("poll of an empty channel: err = %v, want a timeout", err)
	}
	mustInvoke(t, in, "send", ch, interpreter.IntVal(9))
	if v := mustInvoke(t, in, "recv", ch); v.Int != 9 {
		t.Errorf("recv after the timeout = %d, want 9", v.Int)
	}
}

// TestRecvTimeoutWakesOnLaterSend: a waiting recvTimeout gets a value sent
// before its deadline, on an unbuffered channel too.
func TestRecvTimeoutWakesOnLaterSend(t *testing.T) {
	in := newLib(t)
	ch := makeChan(t, in, 0)
	go func() {
		time.Sleep(30 * time.Millisecond)
		if _, err := sendFn(interpreter.BuiltinCtx{}, []interpreter.Value{ch, interpreter.IntVal(7)}); err != nil {
			panic(err)
		}
	}()
	if v := mustInvoke(t, in, "recvTimeout", ch, interpreter.IntVal(5000)); v.Int != 7 {
		t.Errorf("recvTimeout = %d, want 7", v.Int)
	}
}

// TestRecvTimeoutClosedAndArguments: a closed channel drains, then throws as
// recv does; a bad ms or argument count is refused.
func TestRecvTimeoutClosedAndArguments(t *testing.T) {
	in := newLib(t)
	ch := makeChan(t, in, 1)
	mustInvoke(t, in, "send", ch, interpreter.IntVal(1))
	mustInvoke(t, in, "close", ch)
	if v := mustInvoke(t, in, "recvTimeout", ch, interpreter.IntVal(10)); v.Int != 1 {
		t.Errorf("recvTimeout on a closed channel with a value = %d, want 1", v.Int)
	}
	wantErr(t, in, "recvTimeout", "receive on a closed channel", ch, interpreter.IntVal(10))
	open := makeChan(t, in, 1)
	wantErr(t, in, "recvTimeout", "ms must be 0 to", open, interpreter.IntVal(-1))
	wantErr(t, in, "recvTimeout", "ms must be int", open, interpreter.StringVal("1"))
	wantErr(t, in, "recvTimeout", "2 arguments", open)
	wantErr(t, in, "recvTimeout", "must be a channel", interpreter.IntVal(1), interpreter.IntVal(1))
}

// TestRecvTimeoutInAProgram: the error is catchable with kind "timeout", and
// task.cancel wakes a task waiting in recvTimeout at once.
func TestRecvTimeoutInAProgram(t *testing.T) {
	prog, err := parser.Parse(`use channel; use io; use task; use time;
def ch as channel of int init channel.make(1);
try { channel.recvTimeout($ch, 20); } catch (e) { io.printf("%s;", $e.kind); }
def waiter as task of string init spawn {
    try { channel.recvTimeout($ch, 60000); } catch (e) { return $e.message; }
    return "returned";
};
time.sleep(time.fromMilliseconds(50));
def t0 as time.Time init time.now();
task.cancel($waiter);
io.printf("%s;", task.wait($waiter));
io.printf("%v", time.milliseconds(time.sub(time.now(), $t0)) < 1000);`)
	if err != nil {
		t.Fatal(err)
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	tasklib.Install(in)
	timelib.Install(in)
	Install(in)
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v (out=%q)", err, buf.String())
	}
	if want := "timeout;task cancelled;true"; buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}
