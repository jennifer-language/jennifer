// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build unix

package main

import (
	"os"
	"os/signal"
	"syscall"

	oslib "jennifer-lang.dev/jennifer/internal/lib/os"
	termlib "jennifer-lang.dev/jennifer/internal/lib/term"
)

// termRestoreSignals are the terminating signals that would otherwise skip the
// `defer termlib.RestoreAll()` cleanup. Ctrl-C does not appear here in practice
// while a TUI is active - term.makeRaw disables ISIG, so Ctrl-C arrives as a raw
// byte, not SIGINT - but SIGINT is still reachable via `kill -INT`, and SIGTERM /
// SIGHUP (a `kill`, a dropped connection) are the common cases.
var termRestoreSignals = map[syscall.Signal]string{
	syscall.SIGINT:  "int",
	syscall.SIGTERM: "term",
	syscall.SIGHUP:  "hup",
}

// installTermSignalRestore guards the terminal against being left in raw mode by
// a terminating signal. `defer termlib.RestoreAll()` already covers every path
// that returns through runFileHook - a normal exit, `exit`, an uncaught error, a
// panic unwind - but a signal taking its default disposition kills the process
// without running any defer, so a `.j` script that is in raw mode when the signal
// lands would leave the shell wedged (no echo, no line editing).
//
// This traps SIGINT / SIGTERM / SIGHUP for the run. On delivery:
//   - If the script caught that signal itself via os.catchSignal, the handler
//     stays out of the way: the script's cooperative poll loop owns the response,
//     and its eventual clean exit runs the RestoreAll defer. This preserves the
//     os.catchSignal contract exactly.
//   - Otherwise the signal was going to terminate the process anyway. The handler
//     restores every raw-mode terminal, then resets the signal to its default
//     disposition and re-raises it, so the process dies exactly as it would have
//     unhandled (same 128+signum status, WIFSIGNALED for the parent shell) - only
//     now with a cooked terminal. The documented "an uncaught signal keeps its
//     default disposition" guarantee therefore still holds.
//
// SIGKILL and SIGSTOP are uncatchable and deliberately not handled; a `kill -9`
// can still wedge the terminal, which no in-process mechanism can prevent.
// Returns a stop function the caller defers.
func installTermSignalRestore() func() {
	ch := make(chan os.Signal, len(termRestoreSignals))
	sigs := make([]os.Signal, 0, len(termRestoreSignals))
	for s := range termRestoreSignals {
		sigs = append(sigs, s)
	}
	signal.Notify(ch, sigs...)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-ch:
				sig, ok := s.(syscall.Signal)
				if !ok {
					continue
				}
				if oslib.SignalCaught(termRestoreSignals[sig]) {
					// The script is handling this signal cooperatively; leave it be.
					continue
				}
				// An uncaught terminating signal: cook the terminal back, then die
				// as the default disposition would have.
				termlib.RestoreAll()
				signal.Reset(sig)
				_ = syscall.Kill(syscall.Getpid(), sig)
				return
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(ch)
		close(done)
	}
}
