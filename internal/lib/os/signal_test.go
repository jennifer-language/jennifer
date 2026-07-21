// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build unix && !tinygo

package oslib

import (
	stdos "os"
	"sync"
	"syscall"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func sv(s string) interpreter.Value { return interpreter.StringVal(s) }

func TestCatchAndPollSignal(t *testing.T) {
	resetSignalsForTest()
	defer resetSignalsForTest()

	// Not caught yet: never pending.
	if v, err := gotSignalFn(interpreter.BuiltinCtx{}, []interpreter.Value{sv("usr2")}); err != nil || v.Bool {
		t.Fatalf("gotSignal before catch = (%v, %v), want (false, nil)", v.Bool, err)
	}

	if _, err := catchSignalFn(interpreter.BuiltinCtx{}, []interpreter.Value{sv("usr2")}); err != nil {
		t.Fatalf("catchSignal: %v", err)
	}
	// Idempotent: a second catch is a no-op, not an error.
	if _, err := catchSignalFn(interpreter.BuiltinCtx{}, []interpreter.Value{sv("usr2")}); err != nil {
		t.Fatalf("second catchSignal: %v", err)
	}

	// Deliver a real SIGUSR2 to this process; the relay goroutine sets the flag.
	if err := syscall.Kill(stdos.Getpid(), syscall.SIGUSR2); err != nil {
		t.Fatalf("kill: %v", err)
	}
	got := false
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		v, err := gotSignalFn(interpreter.BuiltinCtx{}, []interpreter.Value{sv("usr2")})
		if err != nil {
			t.Fatalf("gotSignal: %v", err)
		}
		if v.Bool {
			got = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !got {
		t.Fatal("SIGUSR2 was never observed via gotSignal")
	}
	// The poll cleared the flag: the next poll is false.
	if v, _ := gotSignalFn(interpreter.BuiltinCtx{}, []interpreter.Value{sv("usr2")}); v.Bool {
		t.Error("gotSignal did not clear the flag")
	}
}

// TestSignalCaught proves the CLI-coordination predicate: a signal reads as
// caught only after os.catchSignal arms it, and never for an unknown name. This
// is what keeps the terminal-restore handler from usurping a signal the script
// is handling cooperatively.
func TestSignalCaught(t *testing.T) {
	resetSignalsForTest()
	defer resetSignalsForTest()

	if SignalCaught("term") {
		t.Error("SignalCaught(\"term\") before catch should be false")
	}
	if SignalCaught("bogus") {
		t.Error("SignalCaught of an unknown signal should be false")
	}
	if _, err := catchSignalFn(interpreter.BuiltinCtx{}, []interpreter.Value{sv("term")}); err != nil {
		t.Fatalf("catchSignal: %v", err)
	}
	if !SignalCaught("term") {
		t.Error("SignalCaught(\"term\") after catch should be true")
	}
	// A different, un-armed signal stays false.
	if SignalCaught("hup") {
		t.Error("SignalCaught(\"hup\") without a catch should be false")
	}
}

func TestSignalErrors(t *testing.T) {
	resetSignalsForTest()
	defer resetSignalsForTest()

	for _, fn := range []func(interpreter.BuiltinCtx, []interpreter.Value) (interpreter.Value, error){catchSignalFn, gotSignalFn} {
		// Unknown signal name.
		if _, err := fn(interpreter.BuiltinCtx{}, []interpreter.Value{sv("kill")}); err == nil {
			t.Error("unknown signal should error")
		}
		// Wrong arity.
		if _, err := fn(interpreter.BuiltinCtx{}, nil); err == nil {
			t.Error("missing arg should error")
		}
		// Non-string.
		if _, err := fn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(9)}); err == nil {
			t.Error("non-string signal name should error")
		}
	}
}

func TestCatchSignalReservesUsr1(t *testing.T) {
	resetSignalsForTest()
	defer resetSignalsForTest()
	if _, err := catchSignalFn(interpreter.BuiltinCtx{}, []interpreter.Value{sv("usr1")}); err == nil {
		t.Error("catchSignal(\"usr1\") must be rejected (reserved for diagnostics)")
	}
}

// TestCatchIsArmedOnce proves catchSignal arms a signal at most once: a repeated
// catch reuses the same relay channel, so the relay goroutine count stays bounded
// (one per signal) and a catch loop cannot leak goroutines.
func TestCatchIsArmedOnce(t *testing.T) {
	resetSignalsForTest()
	defer resetSignalsForTest()
	for i := 0; i < 50; i++ {
		if _, err := catchSignalFn(interpreter.BuiltinCtx{}, []interpreter.Value{sv("usr2")}); err != nil {
			t.Fatalf("catch %d: %v", i, err)
		}
	}
	armMu.Lock()
	ch := sigChans[sigUsr2]
	nArmed := 0
	for _, c := range sigChans {
		if c != nil {
			nArmed++
		}
	}
	armMu.Unlock()
	if ch == nil {
		t.Fatal("usr2 not armed")
	}
	if nArmed != 1 {
		t.Errorf("armed %d signals, want 1 (repeated catch must not re-arm)", nArmed)
	}
}

// TestConcurrentPollRace hammers the lock-free poll path from many goroutines
// while others set the flag - under -race this is the proof that removing the
// mutex from gotSignal did not introduce a data race.
func TestConcurrentPollRace(t *testing.T) {
	resetSignalsForTest()
	defer resetSignalsForTest()
	if _, err := catchSignalFn(interpreter.BuiltinCtx{}, []interpreter.Value{sv("usr2")}); err != nil {
		t.Fatal(err)
	}
	args := []interpreter.Value{sv("usr2")}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20000; j++ {
				gotSignalFn(interpreter.BuiltinCtx{}, args)
			}
		}()
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20000; j++ {
				pending[sigUsr2].Store(true)
			}
		}()
	}
	wg.Wait()
}
