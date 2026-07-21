// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !linux || tinygo

// Stub of the `serial` library for non-Linux hosts (Windows, macOS) and the
// TinyGo `jennifer-tiny` build. Serial-port termios I/O is a Linux `/dev` +
// ioctl feature reached through golang.org/x/sys/unix, which those builds do not
// carry; every entry point returns a friendly error rather than a cryptic one.
// Same shape as the `net` TinyGo stub.

package seriallib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func unavailable(fn string) (Value, error) {
	return interpreter.Null(), fmt.Errorf("%s: serial-port I/O is Linux-only; use the default `jennifer` binary on Linux", fn)
}

// ResetForTest is a no-op where no state exists.
func ResetForTest() {}

func openFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("serial.open") }
func openWithFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("serial.openWith")
}
func readFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)  { return unavailable("serial.read") }
func writeFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("serial.write") }
func flushFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("serial.flush") }
func closeFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("serial.close") }
