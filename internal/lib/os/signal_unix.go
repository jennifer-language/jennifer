// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build unix

// Unix implementation of the os signal polling API: os.catchSignal(name) starts
// trapping a signal, os.gotSignal(name) polls-and-clears whether it has arrived.
// The model is cooperative, never preemptive: a delivered signal only sets an
// atomic flag, and the program reads it at a point of its own choosing - so no
// Jennifer code runs in a signal context and the single-threaded / value-
// semantics guarantees hold. Trapping is opt-in per signal, so a signal the
// program never catches keeps its default disposition (SIGINT / SIGTERM still
// terminate an ordinary script). The state is process-global because signals are
// a property of the process, not of one interpreter.
//
// The five signals occupy fixed slots, so gotSignal - the polled hot path - is a
// name switch plus one atomic Swap, with no lock. catchSignal (rare) holds a
// mutex only to arm a signal once (idempotent) and start its relay goroutine.
package oslib

import (
	"fmt"
	stdos "os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// Signal slots. usr1 has a slot for a uniform gotSignal (it stays false - usr1 is
// reserved for interpreter diagnostics and cannot be caught).
const (
	sigInt  = iota // "int"
	sigTerm        // "term"
	sigHup         // "hup"
	sigUsr1        // "usr1" (reserved; never armed)
	sigUsr2        // "usr2"
	numSignals
)

// lookupSignal maps a Jennifer signal name (letters-only, lowercased, no SIG
// prefix) to its slot and syscall signal. USR1 / USR2 / HUP are Unix-only, which
// is why this whole file is build-tag gated.
func lookupSignal(name string) (idx int, sig syscall.Signal, ok bool) {
	switch name {
	case "int":
		return sigInt, syscall.SIGINT, true
	case "term":
		return sigTerm, syscall.SIGTERM, true
	case "hup":
		return sigHup, syscall.SIGHUP, true
	case "usr1":
		return sigUsr1, syscall.SIGUSR1, true
	case "usr2":
		return sigUsr2, syscall.SIGUSR2, true
	}
	return 0, 0, false
}

var (
	// pending[i] is set by a signal's relay goroutine and cleared by gotSignal.
	// Read/written atomically, so the poll path needs no lock.
	pending [numSignals]atomic.Bool

	// armMu guards arming: catchSignal starts each signal's Notify + relay at
	// most once. Only touched by catchSignal (rare) and the test reset.
	armMu    sync.Mutex
	sigChans [numSignals]chan stdos.Signal // notify channel per armed signal
)

// catchSignalFn implements os.catchSignal(name): begin trapping the named signal.
// Idempotent (a second catch of the same signal is a no-op). Once caught, the
// signal no longer takes its default action; poll it with os.gotSignal.
func catchSignalFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	name, err := signalNameArg("os.catchSignal", args)
	if err != nil {
		return interpreter.Null(), err
	}
	idx, sig, ok := lookupSignal(name)
	if !ok {
		return interpreter.Null(), unknownSignal("os.catchSignal", name)
	}
	if idx == sigUsr1 {
		// SIGUSR1 is the interpreter's diagnostics signal (`kill -USR1 <pid>`
		// prints a state snapshot); reserve it so a program signal uses usr2.
		return interpreter.Null(), fmt.Errorf("os.catchSignal: \"usr1\" is reserved for interpreter diagnostics (kill -USR1 dumps interpreter state); use \"usr2\" for a program signal")
	}

	armMu.Lock()
	defer armMu.Unlock()
	if sigChans[idx] != nil {
		return interpreter.Null(), nil // already armed
	}
	ch := make(chan stdos.Signal, 1)
	signal.Notify(ch, sig)
	sigChans[idx] = ch
	// One relay goroutine per caught signal: every delivery sets the flag. The
	// buffered channel plus a set (not a count) means a burst collapses to a
	// single pending, which is exactly the poll semantics. Bounded to at most one
	// goroutine per signal for the life of the process.
	go func() {
		for range ch {
			pending[idx].Store(true)
		}
	}()
	return interpreter.Null(), nil
}

// gotSignalFn implements os.gotSignal(name) -> bool: whether the named signal has
// arrived since the last poll, clearing the flag. A signal never caught is never
// pending (its slot stays false), so a poll without a prior catch is harmless.
// Lock-free: a name switch plus one atomic Swap.
func gotSignalFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	name, err := signalNameArg("os.gotSignal", args)
	if err != nil {
		return interpreter.Null(), err
	}
	idx, _, ok := lookupSignal(name)
	if !ok {
		return interpreter.Null(), unknownSignal("os.gotSignal", name)
	}
	return interpreter.BoolVal(pending[idx].Swap(false)), nil
}

// SignalCaught reports whether the program has armed the named signal via
// os.catchSignal (name is a Jennifer signal name: "int" / "term" / "hup" / ...).
// The CLI's terminal-restore handler consults this so it never usurps a signal
// the script is handling cooperatively: for a caught signal the script's own poll
// loop owns the response and its eventual clean exit runs the RestoreAll defer,
// so the CLI handler stays out of the way; only an *uncaught* terminating signal
// takes the restore-and-die path. Read under armMu, so it observes a consistent
// arm state.
func SignalCaught(name string) bool {
	idx, _, ok := lookupSignal(name)
	if !ok {
		return false
	}
	armMu.Lock()
	defer armMu.Unlock()
	return sigChans[idx] != nil
}

func signalNameArg(fnName string, args []interpreter.Value) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("%s expects 1 argument (signal name), got %d", fnName, len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return "", fmt.Errorf("%s: signal name must be string, got %s", fnName, args[0].Kind)
	}
	return args[0].Str, nil
}

func unknownSignal(fnName, name string) error {
	return fmt.Errorf("%s: unknown signal %q; known: \"int\", \"term\", \"hup\", \"usr1\", \"usr2\"", fnName, name)
}

// resetSignalsForTest stops every relay and clears the registry between tests.
func resetSignalsForTest() {
	armMu.Lock()
	defer armMu.Unlock()
	for i := range sigChans {
		if sigChans[i] != nil {
			signal.Stop(sigChans[i])
			close(sigChans[i])
			sigChans[i] = nil
		}
		pending[i].Store(false)
	}
}
