// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build (!tinygo || tinygo_devtools) && unix

package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	oslib "jennifer-lang.dev/jennifer/internal/lib/os"
	termlib "jennifer-lang.dev/jennifer/internal/lib/term"
)

// installProfileSignals wires the signals `jennifer profile` handles while the
// program runs:
//
//   - SIGUSR1 prints the interpreter diagnostics dump (as under `run`) and, with
//     --out, writes the profile so far to FILE.<n>; the program keeps running.
//   - SIGINT / SIGTERM, unless the program caught that signal itself
//     (os.catchSignal), stop the program: it exits as if by `exit 130` / `exit
//     143` at its next loop checkpoint, so statements in progress finish their
//     records and deferred calls run, and runProfile writes the report. A
//     program that reaches no checkpoint within profileStopGrace (blocked in
//     httpd.accept, say) gets the report written as it stands, and the process
//     exits with that code.
//
// runDone is closed when the run returns. Returns a stop function.
func installProfileSignals(in *interpreter.Interpreter, rep *profileReport, runDone <-chan struct{}) func() {
	ch := make(chan os.Signal, 4)
	signal.Notify(ch, syscall.SIGUSR1, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-ch:
				switch s {
				case syscall.SIGUSR1:
					in.RequestDiagnostics()
					rep.snapshot()
				case syscall.SIGINT, syscall.SIGTERM:
					name, code := "int", 128+int(syscall.SIGINT)
					if s == syscall.SIGTERM {
						name, code = "term", 128+int(syscall.SIGTERM)
					}
					if oslib.SignalCaught(name) {
						continue // the program handles it; the report follows its exit
					}
					in.RequestExit(code)
					select {
					case <-runDone:
						// The run unwound; runProfile writes the report.
					case <-time.After(profileStopGrace):
						termlib.RestoreAll()
						rep.final()
						os.Exit(code)
					}
					return
				}
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
