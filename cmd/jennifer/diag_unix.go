// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build unix

package main

import (
	"os"
	"os/signal"
	"syscall"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// installDiagSignal wires SIGUSR1 to the interpreter's diagnostics dump: on
// `kill -USR1 <pid>`, the interpreter prints a one-shot labeled snapshot (time,
// source position, spawned/live task counts, goroutines, memory) at its next
// loop / call checkpoint and keeps running. The handler only flips an atomic
// flag, so the dump stays on the interpreter goroutine (race-free). Trapping
// SIGUSR1 here also suppresses its default disposition (terminate), so a stray
// USR1 never kills the program. Returns a stop function the caller defers.
func installDiagSignal(in *interpreter.Interpreter) func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGUSR1)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
				in.RequestDiagnostics()
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
