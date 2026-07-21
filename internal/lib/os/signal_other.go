// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !unix

// Stub of the os signal polling API for non-Unix hosts (Windows). The
// Unix-specific signals (SIGUSR1 / SIGUSR2 / SIGHUP) do not exist there, so
// os.catchSignal is a positioned error rather than a crash, and os.gotSignal
// reports nothing pending. Aborting a script is unaffected - Ctrl-C on Windows
// terminates through the Go runtime, not through this library. Both binaries
// share this stub on non-Unix; on Unix (including the TinyGo build on Linux) the
// real signal_unix.go is used instead.
package oslib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func catchSignalFn(_ interpreter.BuiltinCtx, _ []interpreter.Value) (interpreter.Value, error) {
	return interpreter.Null(), fmt.Errorf("os.catchSignal: signal handling is not available on this platform; Unix signals do not exist here")
}

func gotSignalFn(_ interpreter.BuiltinCtx, _ []interpreter.Value) (interpreter.Value, error) {
	return interpreter.BoolVal(false), nil
}

func resetSignalsForTest() {}
