// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !linux || tinygo

// Stub of the `iic` library for non-Linux hosts and the TinyGo `jennifer-tiny`
// build: I2C is a Linux `/dev` + ioctl feature those builds do not carry.

package iiclib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func unavailable(fn string) (Value, error) {
	return interpreter.Null(), fmt.Errorf("%s: I2C is Linux-only; use the default `jennifer` binary on Linux", fn)
}

// ResetForTest is a no-op where no state exists.
func ResetForTest() {}

func openFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)    { return unavailable("iic.open") }
func readFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)    { return unavailable("iic.read") }
func writeFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)   { return unavailable("iic.write") }
func readRegFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("iic.readReg") }
func writeRegFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("iic.writeReg")
}
func closeFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("iic.close") }
