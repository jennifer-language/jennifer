// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !unix

package main

import "jennifer-lang.dev/jennifer/internal/interpreter"

// installDiagSignal is a no-op where SIGUSR1 does not exist (Windows). Returns a
// no-op stop function. On Unix (including the TinyGo build on Linux) the real
// diag_unix.go wires the SIGUSR1 diagnostics dump instead.
func installDiagSignal(_ *interpreter.Interpreter) func() { return func() {} }
