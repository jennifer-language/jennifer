// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !linux || tinygo

// Stub of the `spi` library for non-Linux hosts and the TinyGo `jennifer-tiny`
// build: SPI is a Linux `/dev` + ioctl feature those builds do not carry.

package spilib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func unavailable(fn string) (Value, error) {
	return interpreter.Null(), fmt.Errorf("%s: SPI is Linux-only; use the default `jennifer` binary on Linux", fn)
}

// ResetForTest is a no-op where no state exists.
func ResetForTest() {}

func openFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("spi.open") }
func configureFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("spi.configure")
}
func transferFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("spi.transfer")
}
func closeFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("spi.close") }
