// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package seriallib implements Jennifer's `serial` library: blocking access to a
// serial port (`/dev/ttyUSB0`, `/dev/ttyAMA0`, ...) with termios configuration
// (baud rate, data bits, parity, stop bits) that plain `fs` cannot reach.
//
// Build-tag split like `net`: the real Linux implementation
// (seriallib_linux.go, `linux && !tinygo`) drives the port through
// `golang.org/x/sys/unix` termios ioctls; every other build (Windows, macOS, or
// the TinyGo `jennifer-tiny`) selects seriallib_other.go, whose entry points
// return a friendly error pointing at the default `jennifer` binary on Linux.
//
// Blocking on purpose - concurrency is composed with `spawn`, not a duplicated
// async surface (same stance as `fs` / `net`). Handles use the integer-registry
// pattern: `serial.Port{id as int}` on the Jennifer side, live `*os.File` state
// in a package registry.
package seriallib

import (
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "serial"

// Value keeps builtin signatures short.
type Value = interpreter.Value

// Install registers the serial surface. The port-driving verbs (open / openWith
// / read / write / flush / close) are provided by the build-tag-selected file.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedStruct(LibraryName, "Port", []parser.StructField{
		{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)},
	})
	// Full line configuration for openWith; open() is the 8N1 shorthand.
	in.RegisterNamespacedStruct(LibraryName, "Options", []parser.StructField{
		{Name: "baud", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "dataBits", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "parity", Type: parser.PrimitiveType(parser.TypeString)},
		{Name: "stopBits", Type: parser.PrimitiveType(parser.TypeInt)},
	})
	in.RegisterNamespaced(LibraryName, "open", openFn)
	in.RegisterNamespaced(LibraryName, "openWith", openWithFn)
	in.RegisterNamespaced(LibraryName, "read", readFn)
	in.RegisterNamespaced(LibraryName, "write", writeFn)
	in.RegisterNamespaced(LibraryName, "flush", flushFn)
	in.RegisterNamespaced(LibraryName, "close", closeFn)
}
