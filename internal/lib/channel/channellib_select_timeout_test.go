// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package channellib

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.45: channel.selectTimeout is channel.select bounded by a deadline.

// errKind is the kind of a builtin's error: a KindedError's Kind field, ""
// (runtime) for a plain error.
func errKind(err error) string {
	if err == nil {
		return "<nil>"
	}
	v := reflect.ValueOf(err)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		if f := v.FieldByName("Kind"); f.IsValid() && f.Kind() == reflect.String {
			return f.String()
		}
	}
	return ""
}

func chans(cs ...interpreter.Value) interpreter.Value {
	return interpreter.Value{Kind: interpreter.KindList, List: cs}
}

// TestSelectTimeoutReturnsAReadyValue: a waiting value comes back at once,
// with ms 0 (a poll) too.
func TestSelectTimeoutReturnsAReadyValue(t *testing.T) {
	in := newLib(t)
	a, b := makeChan(t, in, 1), makeChan(t, in, 1)
	mustInvoke(t, in, "send", b, interpreter.IntVal(4))
	if v := mustInvoke(t, in, "selectTimeout", chans(a, b), interpreter.IntVal(0)); v.Int != 4 {
		t.Errorf("selectTimeout(.., 0) = %d, want 4", v.Int)
	}
	mustInvoke(t, in, "send", a, interpreter.IntVal(5))
	if v := mustInvoke(t, in, "selectTimeout", chans(a, b), interpreter.IntVal(1000)); v.Int != 5 {
		t.Errorf("selectTimeout(.., 1000) = %d, want 5", v.Int)
	}
}

// TestSelectTimeoutTimesOut: empty channels throw "timeout" after about ms and
// take nothing: a value sent afterwards is still there.
func TestSelectTimeoutTimesOut(t *testing.T) {
	in := newLib(t)
	a, b := makeChan(t, in, 1), makeChan(t, in, 1)
	start := time.Now()
	_, err := invoke(t, in, "selectTimeout", chans(a, b), interpreter.IntVal(80))
	took := time.Since(start)
	if err == nil || !strings.Contains(err.Error(), "channel.selectTimeout: timed out after 80 ms") {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if k := errKind(err); k != "timeout" {
		t.Errorf("kind = %q, want timeout", k)
	}
	if took < 70*time.Millisecond || took > 2*time.Second {
		t.Errorf("took %v, want about 80 ms", took)
	}
	if _, err := invoke(t, in, "selectTimeout", chans(a, b), interpreter.IntVal(0)); err == nil || !strings.Contains(err.Error(), "timed out after 0 ms") {
		t.Errorf("poll of empty channels: err = %v, want a timeout", err)
	}
	mustInvoke(t, in, "send", b, interpreter.IntVal(9))
	if v := mustInvoke(t, in, "recv", b); v.Int != 9 {
		t.Errorf("recv after the timeout = %d, want 9", v.Int)
	}
}

// TestSelectTimeoutWakesOnLaterSend: a waiting selectTimeout gets a value sent
// on an unbuffered channel before its deadline.
func TestSelectTimeoutWakesOnLaterSend(t *testing.T) {
	in := newLib(t)
	a, b := makeChan(t, in, 0), makeChan(t, in, 0)
	go func() {
		time.Sleep(30 * time.Millisecond)
		if _, err := sendFn(interpreter.BuiltinCtx{}, []interpreter.Value{b, interpreter.IntVal(7)}); err != nil {
			panic(err)
		}
	}()
	if v := mustInvoke(t, in, "selectTimeout", chans(a, b), interpreter.IntVal(5000)); v.Int != 7 {
		t.Errorf("selectTimeout = %d, want 7", v.Int)
	}
}

// TestSelectTimeoutClosedAndArguments: every channel closed and drained throws
// "closed" as select does (not "timeout"); a closed channel among open ones is
// skipped; bad arguments are "runtime".
func TestSelectTimeoutClosedAndArguments(t *testing.T) {
	in := newLib(t)
	closed := makeChan(t, in, 1)
	mustInvoke(t, in, "close", closed)
	_, err := invoke(t, in, "selectTimeout", chans(closed), interpreter.IntVal(5000))
	if err == nil || errKind(err) != "closed" || !strings.Contains(err.Error(), "all channels are closed") {
		t.Errorf("all closed: err = %v (kind %q)", err, errKind(err))
	}
	open := makeChan(t, in, 1)
	_, err = invoke(t, in, "selectTimeout", chans(closed, open), interpreter.IntVal(20))
	if errKind(err) != "timeout" {
		t.Errorf("closed + open: err = %v, want a timeout", err)
	}
	for _, c := range []struct {
		args []interpreter.Value
		frag string
	}{
		{[]interpreter.Value{chans(open), interpreter.IntVal(-1)}, "ms must be 0 to"},
		{[]interpreter.Value{chans(open), interpreter.IntVal(maxTimeoutMillis + 1)}, "ms must be 0 to"},
		{[]interpreter.Value{chans(open), interpreter.StringVal("1")}, "ms must be int"},
		{[]interpreter.Value{chans(), interpreter.IntVal(1)}, "the list is empty"},
		{[]interpreter.Value{chans(open)}, "2 arguments"},
	} {
		_, err := invoke(t, in, "selectTimeout", c.args...)
		if err == nil || !strings.Contains(err.Error(), c.frag) || errKind(err) != "" {
			t.Errorf("args %v: err = %v, want runtime %q", c.args, err, c.frag)
		}
	}
	// recvTimeout's upper bound matches.
	wantErr(t, in, "recvTimeout", "ms must be 0 to", open, interpreter.IntVal(maxTimeoutMillis+1))
}

// TestSelectTimeoutNoLeak: timed-out selects leave no goroutine or waker behind.
func TestSelectTimeoutNoLeak(t *testing.T) {
	in := newLib(t)
	a, b := makeChan(t, in, 0), makeChan(t, in, 1)
	runtime.GC()
	before := runtime.NumGoroutine()
	for i := 0; i < 50; i++ {
		if _, err := invoke(t, in, "selectTimeout", chans(a, b), interpreter.IntVal(1)); errKind(err) != "timeout" {
			t.Fatalf("err = %v", err)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Errorf("goroutines: %d before, %d after", before, n)
	}
	// The wakers were unsubscribed: a send still lands in the buffer.
	mustInvoke(t, in, "send", b, interpreter.IntVal(1))
	if v := mustInvoke(t, in, "recv", b); v.Int != 1 {
		t.Errorf("recv = %d", v.Int)
	}
}
