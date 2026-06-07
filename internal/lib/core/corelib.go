// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// Package corelib implements Jennifer's `core` library: the small set of
// structural builtins that every program needs without ceremony.
//
// `core` is special among libraries. It is auto-loaded by the interpreter
// at startup (see Interpreter.New) and writing `use core;` in source is a
// parse-time-equivalent error - the library is already available, and an
// explicit `use core;` signals confusion that's better surfaced loudly.
//
// Pass 1 contents:
//   - JENNIFER_VERSION (string constant) - the interpreter's build version.
//     Underscored, language-prefixed name follows the PHP_VERSION /
//     RUBY_VERSION precedent and leaves room for future host/build
//     constants (JENNIFER_BUILDTIME, PLATFORM, OSNAME, ARCH).
//
// Pass 2 (during the M5 cleanup that introduced this library) adds:
//   - len(string | list | map) - polymorphic structural length.
//
// Reserve this library carefully. It is the escape hatch from Jennifer's
// "nothing for free" library discipline; it should hold a handful of
// universally-needed structural primitives and nothing more. Anything that
// could plausibly belong to a topic library (io, math, strings, ...) goes
// there instead.
//
// The Go package is named corelib to follow the convention used by the
// other libraries (iolib, mathlib, stringslib).
package corelib

import (
	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/version"
)

// LibraryName is the Jennifer name the interpreter uses to track the core
// library. Programs do NOT write `use core;` - the interpreter pre-imports
// it and explicit imports are rejected.
const LibraryName = "core"

// Install registers the core library's builtins and constants on an
// interpreter. The caller is also expected to mark `core` as
// pre-imported; that happens in Interpreter.New, not here, so this
// installer remains a plain Register-only function consistent with the
// other libraries.
func Install(in *interpreter.Interpreter) {
	in.RegisterConst(LibraryName, "JENNIFER_VERSION", interpreter.StringVal(version.Version))
}
