// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package gpiolib implements Jennifer's `gpio` library over the modern
// `/dev/gpiochipN` character-device interface (the GPIO v2 line ioctls), the
// mainline-supported GPIO API since sysfs `/sys/class/gpio` was deprecated.
//
// It deliberately reuses the pin-keyed shape of the sysfs-backed `gpio` module
// (setup / read / write / release, plus the IN / OUT direction constants) so a
// script moves between the two unchanged; the chip device is selected once with
// `gpio.chip` (default `/dev/gpiochip0`). The library holds an internal
// pin -> requested-line-fd registry, so the pin number is the key exactly as in
// the module.
//
// Build-tag split like `net`: the real Linux implementation (gpiolib_linux.go,
// `linux && !tinygo`); a friendly-error stub (gpiolib_other.go) everywhere else.
package gpiolib

import (
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "gpio"

// Value keeps builtin signatures short.
type Value = interpreter.Value

// Install registers the gpio surface. The line verbs are build-tag-selected; the
// IN / OUT direction constants match the sysfs `gpio` module.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedConst(LibraryName, "IN", interpreter.StringVal("in"))
	in.RegisterNamespacedConst(LibraryName, "OUT", interpreter.StringVal("out"))
	in.RegisterNamespaced(LibraryName, "chip", chipFn)
	in.RegisterNamespaced(LibraryName, "setup", setupFn)
	in.RegisterNamespaced(LibraryName, "read", readFn)
	in.RegisterNamespaced(LibraryName, "write", writeFn)
	in.RegisterNamespaced(LibraryName, "release", releaseFn)
}
