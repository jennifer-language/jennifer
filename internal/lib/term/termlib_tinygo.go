// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build tinygo

// TinyGo stub for the `term` library. A minimal / embedded `jennifer-tiny`
// target may have no controlling terminal, and terminal control pulls in
// golang.org/x/term, which the tiny build deliberately excludes. Every entry
// point returns a friendly Jennifer-level error pointing at the default
// `jennifer` binary - the same pattern as `net` and `os.run` under TinyGo.
package termlib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// ResetForTest is a no-op under TinyGo since no state exists.
func ResetForTest() {}

// RestoreAll is a no-op under TinyGo: the `term` library is stubbed there, so no
// raw-mode registry exists to restore. Defined so the CLI's teardown call
// compiles for both binaries.
func RestoreAll() {}

// unavailable is the shared error every stub returns.
func unavailable(fnName string) (Value, error) {
	return interpreter.Null(), fmt.Errorf("%s: `jennifer-tiny` (TinyGo build) has no terminal control; use the default `jennifer` binary", fnName)
}

func makeRawFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("term.makeRaw")
}
func restoreFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("term.restore")
}
func sizeFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("term.size")
}
func readByteFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("term.readByte")
}
