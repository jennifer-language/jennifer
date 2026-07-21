// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package termlib implements Jennifer's `term` library: the terminal host
// capabilities an interactive TUI needs and pure `.j` cannot reach - raw mode
// (unbuffered, no-echo input), the terminal size, and raw single-byte reads from
// stdin. Higher-level screen control, key decoding, and rendering sit in a `.j`
// module on top; this library is only the host primitives.
//
// Surface: `term.makeRaw(stream)` -> `term.State` and `term.restore(state)`
// (enter / leave raw mode); `term.size(stream)` -> `term.Size{rows, cols}`; and
// `term.readByte()` -> int (one raw byte from stdin, -1 at end of input).
//
// Build-tag split like `net` / `os`: the default `jennifer` binary ships the
// real implementation (termlib_std.go, over golang.org/x/term - already a
// repository dependency, here promoted to a build-tag-gated library dependency);
// `jennifer-tiny` ships stubs (termlib_tinygo.go) that return a friendly error,
// since a minimal / embedded target may have no controlling terminal.
//
// The raw-mode state uses the integer-handle-into-a-registry pattern from `fs`
// and `net`: `term.State{id as int}` on the Jennifer side indexes a package
// registry holding the saved termios state; `restore` consumes the handle.
package termlib

import (
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "term"

// Value type alias keeps signatures short.
type Value = interpreter.Value

// Install registers the term surface. The four verbs are implemented by the
// build-tag-selected file (termlib_std.go or termlib_tinygo.go); this file
// registers the shared struct types and wires the verbs.
func Install(in *interpreter.Interpreter) {
	// term.State is the raw-mode handle passed back to term.restore.
	in.RegisterNamespacedStruct(LibraryName, "State", []parser.StructField{
		{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)},
	})
	// term.Size is what term.size returns.
	in.RegisterNamespacedStruct(LibraryName, "Size", []parser.StructField{
		{Name: "rows", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "cols", Type: parser.PrimitiveType(parser.TypeInt)},
	})

	in.RegisterNamespaced(LibraryName, "makeRaw", makeRawFn)
	in.RegisterNamespaced(LibraryName, "restore", restoreFn)
	in.RegisterNamespaced(LibraryName, "size", sizeFn)
	in.RegisterNamespaced(LibraryName, "readByte", readByteFn)
}
