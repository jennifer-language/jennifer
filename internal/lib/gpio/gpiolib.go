// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package gpiolib implements Jennifer's `gpio` library over the modern
// `/dev/gpiochipN` character-device interface (the GPIO v2 line ioctls), the
// mainline-supported GPIO API since sysfs `/sys/class/gpio` was deprecated.
//
// A chip is a handle (`gpio.open(path) -> gpio.Chip`); setup / read / write /
// release take the chip and the pin, with the IN / OUT direction constants of
// the deprecated sysfs-backed `gpio` module. The library holds an internal
// (chip, pin) -> requested-line-fd registry. The pin-only forms on the
// process-wide chip (`gpio.chip`, JENNIFER_GPIO_CHIP, default
// `/dev/gpiochip0`) are deprecated (M29.49).
//
// Build-tag split like `net`: the real Linux implementation (gpiolib_linux.go,
// `linux && !tinygo`); a friendly-error stub (gpiolib_other.go) everywhere else.
package gpiolib

import (
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
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
	in.RegisterNamespacedStruct(LibraryName, "Chip", []parser.StructField{
		{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)},
	})
	in.RegisterNamespaced(LibraryName, "open", openFn)
	in.RegisterNamespaced(LibraryName, "close", closeFn)
	in.RegisterNamespaced(LibraryName, "chip", chipFn)
	in.RegisterNamespaced(LibraryName, "setup", setupFn)
	in.RegisterNamespaced(LibraryName, "read", readFn)
	in.RegisterNamespaced(LibraryName, "write", writeFn)
	in.RegisterNamespaced(LibraryName, "release", releaseFn)
}
