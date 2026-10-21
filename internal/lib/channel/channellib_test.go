// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package channellib

import (
	"strings"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func newLib(t *testing.T) *interpreter.Interpreter {
	t.Helper()
	in := interpreter.New()
	Install(in)
	return in
}

func invoke(t *testing.T, in *interpreter.Interpreter, name string, args ...interpreter.Value) (interpreter.Value, error) {
	t.Helper()
	fn := in.LookupNamespacedBuiltin("channel", name)
	if fn == nil {
		t.Fatalf("builtin channel.%s is not registered", name)
	}
	return fn(interpreter.BuiltinCtx{}, args)
}

func mustInvoke(t *testing.T, in *interpreter.Interpreter, name string, args ...interpreter.Value) interpreter.Value {
	t.Helper()
	v, err := invoke(t, in, name, args...)
	if err != nil {
		t.Fatalf("channel.%s: unexpected error: %v", name, err)
	}
	return v
}

func wantErr(t *testing.T, in *interpreter.Interpreter, name string, substr string, args ...interpreter.Value) {
	t.Helper()
	_, err := invoke(t, in, name, args...)
	if err == nil {
		t.Errorf("channel.%s: expected an error containing %q, got nil", name, substr)
		return
	}
	if substr != "" && !strings.Contains(err.Error(), substr) {
		t.Errorf("channel.%s: error %q does not contain %q", name, err.Error(), substr)
	}
}

func makeChan(t *testing.T, in *interpreter.Interpreter, capacity int64) interpreter.Value {
	t.Helper()
	return mustInvoke(t, in, "make", interpreter.IntVal(capacity))
}

func TestInstallRegistersEveryBuiltin(t *testing.T) {
	in := newLib(t)
	for _, name := range []string{"make", "send", "recv", "close", "select", "len", "capacity"} {
		if in.LookupNamespacedBuiltin("channel", name) == nil {
			t.Errorf("channel.%s is not registered by Install", name)
		}
	}
}

func TestMakeReturnsChannel(t *testing.T) {
	in := newLib(t)
	for _, capacity := range []int64{0, 1, 5} {
		ch := makeChan(t, in, capacity)
		if ch.Kind != interpreter.KindChannel {
			t.Fatalf("make(%d): kind = %s, want channel", capacity, ch.Kind)
		}
		if ch.Chan == nil {
			t.Fatalf("make(%d): channel state is nil", capacity)
		}
		if got := mustInvoke(t, in, "capacity", ch); got.Int != capacity {
			t.Errorf("make(%d): capacity() = %d, want %d", capacity, got.Int, capacity)
		}
	}
}

func TestMakeErrors(t *testing.T) {
	in := newLib(t)
	wantErr(t, in, "make", "1 argument")                                               // no args
	wantErr(t, in, "make", "1 argument", interpreter.IntVal(1), interpreter.IntVal(2)) // too many
	wantErr(t, in, "make", "must be int", interpreter.FloatVal(1.0))                   // wrong kind
	wantErr(t, in, "make", ">= 0", interpreter.IntVal(-1))                             // negative
	wantErr(t, in, "make", "exceeds the limit", interpreter.IntVal(int64(limits.MaxChannelCapacity)+1))
}

func TestSendRecvRoundTrip(t *testing.T) {
	in := newLib(t)
	ch := makeChan(t, in, 2)
	mustInvoke(t, in, "send", ch, interpreter.IntVal(42))
	mustInvoke(t, in, "send", ch, interpreter.StringVal("hi"))
	// Buffered FIFO: values come back in send order.
	if got := mustInvoke(t, in, "recv", ch); got.Kind != interpreter.KindInt || got.Int != 42 {
		t.Errorf("recv 1 = %+v, want int 42", got)
	}
	if got := mustInvoke(t, in, "recv", ch); got.Kind != interpreter.KindString || got.Str != "hi" {
		t.Errorf("recv 2 = %+v, want string \"hi\"", got)
	}
}

// TestSendDeepCopiesValue validates that sendFn takes a private copy: a mutation
// of the original compound value after the send must not reach the receiver.
func TestSendDeepCopiesValue(t *testing.T) {
	in := newLib(t)
	ch := makeChan(t, in, 1)
	orig := interpreter.ListVal(parser.PrimitiveType(parser.TypeInt),
		[]interpreter.Value{interpreter.IntVal(1), interpreter.IntVal(2)})
	mustInvoke(t, in, "send", ch, orig)
	orig.List[0] = interpreter.IntVal(999) // mutate the original after the send
	got := mustInvoke(t, in, "recv", ch)
	if got.Kind != interpreter.KindList || len(got.List) != 2 || got.List[0].Int != 1 {
		t.Errorf("send must deep-copy: received first element = %+v, want 1 (unaffected by later mutation)", got.List[0])
	}
}

func TestSendOnClosedChannelErrors(t *testing.T) {
	in := newLib(t)
	ch := makeChan(t, in, 1)
	mustInvoke(t, in, "close", ch)
	wantErr(t, in, "send", "closed", ch, interpreter.IntVal(1))
}

// TestRecvDrainsThenErrorsAfterClose: buffered values still receive after close;
// once drained, recv is a catchable error.
func TestRecvDrainsThenErrorsAfterClose(t *testing.T) {
	in := newLib(t)
	ch := makeChan(t, in, 2)
	mustInvoke(t, in, "send", ch, interpreter.IntVal(10))
	mustInvoke(t, in, "send", ch, interpreter.IntVal(20))
	mustInvoke(t, in, "close", ch)
	if got := mustInvoke(t, in, "recv", ch); got.Int != 10 {
		t.Errorf("recv after close 1 = %d, want 10", got.Int)
	}
	if got := mustInvoke(t, in, "recv", ch); got.Int != 20 {
		t.Errorf("recv after close 2 = %d, want 20", got.Int)
	}
	wantErr(t, in, "recv", "closed", ch) // drained + closed
}

func TestCloseAndDoubleClose(t *testing.T) {
	in := newLib(t)
	ch := makeChan(t, in, 0)
	mustInvoke(t, in, "close", ch)
	wantErr(t, in, "close", "already closed", ch)
}

func TestLenTracksBuffer(t *testing.T) {
	in := newLib(t)
	ch := makeChan(t, in, 3)
	if got := mustInvoke(t, in, "len", ch); got.Int != 0 {
		t.Errorf("len of fresh channel = %d, want 0", got.Int)
	}
	mustInvoke(t, in, "send", ch, interpreter.IntVal(1))
	mustInvoke(t, in, "send", ch, interpreter.IntVal(2))
	if got := mustInvoke(t, in, "len", ch); got.Int != 2 {
		t.Errorf("len after 2 sends = %d, want 2", got.Int)
	}
	mustInvoke(t, in, "recv", ch)
	if got := mustInvoke(t, in, "len", ch); got.Int != 1 {
		t.Errorf("len after 1 recv = %d, want 1", got.Int)
	}
}

func TestSelectReturnsReadyValue(t *testing.T) {
	in := newLib(t)
	a := makeChan(t, in, 1)
	b := makeChan(t, in, 1)
	mustInvoke(t, in, "send", b, interpreter.IntVal(99))
	chans := interpreter.ListVal(parser.ChannelType(parser.PrimitiveType(parser.TypeInt)), []interpreter.Value{a, b})
	got := mustInvoke(t, in, "select", chans)
	if got.Kind != interpreter.KindInt || got.Int != 99 {
		t.Errorf("select returned %+v, want int 99 from the ready channel", got)
	}
}

// TestSelectSkipsClosedChannel: a closed, drained channel in the set is dropped,
// and select still delivers from a live one.
func TestSelectSkipsClosedChannel(t *testing.T) {
	in := newLib(t)
	a := makeChan(t, in, 1)
	b := makeChan(t, in, 1)
	mustInvoke(t, in, "send", a, interpreter.IntVal(7))
	mustInvoke(t, in, "close", b) // b is closed and empty
	chans := interpreter.ListVal(parser.ChannelType(parser.PrimitiveType(parser.TypeInt)), []interpreter.Value{b, a})
	got := mustInvoke(t, in, "select", chans)
	if got.Int != 7 {
		t.Errorf("select over [closed b, live a] = %+v, want 7 from a", got)
	}
}

func TestSelectAllClosedErrors(t *testing.T) {
	in := newLib(t)
	a := makeChan(t, in, 0)
	b := makeChan(t, in, 0)
	mustInvoke(t, in, "close", a)
	mustInvoke(t, in, "close", b)
	chans := interpreter.ListVal(parser.ChannelType(parser.PrimitiveType(parser.TypeInt)), []interpreter.Value{a, b})
	wantErr(t, in, "select", "all channels are closed", chans)
}

func TestSelectArgumentErrors(t *testing.T) {
	in := newLib(t)
	// not a list
	wantErr(t, in, "select", "list of channel", interpreter.IntVal(1))
	// empty list
	wantErr(t, in, "select", "empty", interpreter.ListVal(parser.ChannelType(parser.PrimitiveType(parser.TypeInt)), nil))
	// a non-channel element
	bad := interpreter.ListVal(parser.ChannelType(parser.PrimitiveType(parser.TypeInt)), []interpreter.Value{interpreter.IntVal(1)})
	wantErr(t, in, "select", "must be a channel", bad)
}

func TestCapacityValues(t *testing.T) {
	in := newLib(t)
	if got := mustInvoke(t, in, "capacity", makeChan(t, in, 0)); got.Int != 0 {
		t.Errorf("capacity(make 0) = %d, want 0", got.Int)
	}
	if got := mustInvoke(t, in, "capacity", makeChan(t, in, 7)); got.Int != 7 {
		t.Errorf("capacity(make 7) = %d, want 7", got.Int)
	}
}

// TestNonChannelArgument: every op that takes a channel rejects a non-channel.
func TestNonChannelArgument(t *testing.T) {
	in := newLib(t)
	notCh := interpreter.IntVal(5)
	wantErr(t, in, "send", "must be a channel", notCh, interpreter.IntVal(1))
	wantErr(t, in, "recv", "must be a channel", notCh)
	wantErr(t, in, "close", "must be a channel", notCh)
	wantErr(t, in, "len", "must be a channel", notCh)
	wantErr(t, in, "capacity", "must be a channel", notCh)
}

// TestUninitializedChannel: a KindChannel value with no state (never made) is a
// catchable error, not a nil-deref crash.
func TestUninitializedChannel(t *testing.T) {
	in := newLib(t)
	uninit := interpreter.Value{Kind: interpreter.KindChannel} // Chan == nil
	wantErr(t, in, "recv", "no state", uninit)
	wantErr(t, in, "send", "no state", uninit, interpreter.IntVal(1))
	wantErr(t, in, "len", "no state", uninit)
	wantErr(t, in, "capacity", "no state", uninit)
	wantErr(t, in, "close", "no state", uninit)
}

// TestArityErrors: the fixed-arity ops reject wrong argument counts.
func TestArityErrors(t *testing.T) {
	in := newLib(t)
	ch := makeChan(t, in, 1)
	wantErr(t, in, "send", "2 arguments", ch)        // missing value
	wantErr(t, in, "recv", "1 argument")             // no args
	wantErr(t, in, "close", "1 argument", ch, ch)    // too many
	wantErr(t, in, "len", "1 argument")              // no args
	wantErr(t, in, "capacity", "1 argument", ch, ch) // too many
	wantErr(t, in, "select", "1 argument")           // no args
}

// TestSendElementTypeCheck: once a channel is bound to `channel of T` (ElemTyp
// set), a send of the wrong kind fails at the send site; the right kind passes.
func TestSendElementTypeCheck(t *testing.T) {
	in := newLib(t)
	ch := makeChan(t, in, 1)
	intT := parser.PrimitiveType(parser.TypeInt)
	ch.Chan.ElemTyp.Store(&intT)
	wantErr(t, in, "send", "must be int", ch, interpreter.StringVal("nope"))
	mustInvoke(t, in, "send", ch, interpreter.IntVal(1)) // right type is accepted
}

// TestSelectFanInStress drives select as a fan-in over several channels fed by
// concurrent senders, unbuffered and buffered, until every channel is closed.
// Every sent value must arrive exactly once: a lost wake-up would hang the test,
// and a select that consumed a value it did not return would lose it.
func TestSelectFanInStress(t *testing.T) {
	for _, capacity := range []int64{0, 1, 8} {
		in := newLib(t)
		const senders, perSender = 6, 300
		chans := make([]interpreter.Value, senders)
		for i := range chans {
			chans[i] = makeChan(t, in, capacity)
		}
		sendFn := in.LookupNamespacedBuiltin("channel", "send")
		closeFn := in.LookupNamespacedBuiltin("channel", "close")
		for s := 0; s < senders; s++ {
			go func(s int) {
				for k := 0; k < perSender; k++ {
					if _, err := sendFn(interpreter.BuiltinCtx{}, []interpreter.Value{chans[s], interpreter.IntVal(int64(s*perSender + k))}); err != nil {
						panic(err)
					}
				}
				if _, err := closeFn(interpreter.BuiltinCtx{}, []interpreter.Value{chans[s]}); err != nil {
					panic(err)
				}
			}(s)
		}
		list := interpreter.Value{Kind: interpreter.KindList, List: chans}
		seen := make(map[int64]bool)
		for {
			v, err := invoke(t, in, "select", list)
			if err != nil {
				if !strings.Contains(err.Error(), "all channels are closed") {
					t.Fatalf("capacity %d: select: %v", capacity, err)
				}
				break
			}
			if seen[v.Int] {
				t.Fatalf("capacity %d: value %d received twice", capacity, v.Int)
			}
			seen[v.Int] = true
		}
		if len(seen) != senders*perSender {
			t.Errorf("capacity %d: received %d values, want %d", capacity, len(seen), senders*perSender)
		}
	}
}

// TestSelectWakesOnLaterSend: a select blocked on empty channels is woken by a
// send that happens after it started waiting.
func TestSelectWakesOnLaterSend(t *testing.T) {
	in := newLib(t)
	a, b := makeChan(t, in, 0), makeChan(t, in, 0)
	got := make(chan interpreter.Value, 1)
	go func() {
		v, err := invoke(t, in, "select", interpreter.Value{Kind: interpreter.KindList, List: []interpreter.Value{a, b}})
		if err != nil {
			panic(err)
		}
		got <- v
	}()
	time.Sleep(20 * time.Millisecond)
	mustInvoke(t, in, "send", b, interpreter.IntVal(7))
	select {
	case v := <-got:
		if v.Int != 7 {
			t.Errorf("select returned %d, want 7", v.Int)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("select was not woken by a later send")
	}
}
