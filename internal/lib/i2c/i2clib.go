// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package i2clib implements Jennifer's `i2c` library: the I2C (Inter-IC) bus
// (`/dev/i2c-1`, ...). The name carries a digit, which identifiers now admit.
//
// Open a bus at a 7-bit slave address, then `read` / `write` raw bytes or the
// register-oriented `readReg` / `writeReg`. Slave selection is the `I2C_SLAVE`
// ioctl, which is what forces a Go library rather than plain `fs`.
//
// Build-tag split like `net`: the real Linux implementation (i2clib_linux.go,
// `linux && !tinygo`) over golang.org/x/sys/unix; a friendly-error stub
// (i2clib_other.go) everywhere else. Blocking; compose with `spawn`.
package i2clib

import (
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "i2c"

// Value keeps builtin signatures short.
type Value = interpreter.Value

// Install registers the i2c surface. The bus verbs are build-tag-selected.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedStruct(LibraryName, "Bus", []parser.StructField{
		{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)},
	})
	in.RegisterNamespaced(LibraryName, "open", openFn)
	in.RegisterNamespaced(LibraryName, "read", readFn)
	in.RegisterNamespaced(LibraryName, "write", writeFn)
	in.RegisterNamespaced(LibraryName, "readReg", readRegFn)
	in.RegisterNamespaced(LibraryName, "writeReg", writeRegFn)
	in.RegisterNamespaced(LibraryName, "close", closeFn)
}
