// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !linux || tinygo

// Stub of the `gpio` library for non-Linux hosts and the TinyGo `jennifer-tiny`
// build: the GPIO character device is a Linux `/dev` + ioctl feature those
// builds do not carry. (The sysfs-backed `gpio` MODULE is the portable default;
// this ioctl library lands only where sysfs GPIO is unavailable.)

package gpiolib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func unavailable(fn string) (Value, error) {
	return interpreter.Null(), fmt.Errorf("%s: the gpiochip character device is Linux-only; use the default `jennifer` binary on Linux (or the sysfs `gpio` module)", fn)
}

// ResetForTest is a no-op where no state exists.
func ResetForTest() {}

func chipFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)  { return unavailable("gpio.chip") }
func setupFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("gpio.setup") }
func readFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)  { return unavailable("gpio.read") }
func writeFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("gpio.write") }
func releaseFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("gpio.release")
}
