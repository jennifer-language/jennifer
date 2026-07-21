// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package spilib implements Jennifer's `spi` library: SPI devices
// (`/dev/spidev0.0`, ...). Open a device, set mode / speed, then `transfer`
// bytes full-duplex (write and read happen together over one exchange), the
// `SPI_IOC_MESSAGE` ioctl that plain `fs` cannot issue.
//
// Build-tag split like `net`: the real Linux implementation (spilib_linux.go,
// `linux && !tinygo`) over golang.org/x/sys/unix; a friendly-error stub
// (spilib_other.go) everywhere else. Blocking; compose with `spawn`.
package spilib

import (
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "spi"

// Value keeps builtin signatures short.
type Value = interpreter.Value

// Install registers the spi surface. The device verbs are build-tag-selected.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedStruct(LibraryName, "Device", []parser.StructField{
		{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)},
	})
	in.RegisterNamespaced(LibraryName, "open", openFn)
	in.RegisterNamespaced(LibraryName, "configure", configureFn)
	in.RegisterNamespaced(LibraryName, "transfer", transferFn)
	in.RegisterNamespaced(LibraryName, "close", closeFn)
}
