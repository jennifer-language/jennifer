// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// Package metalib implements Jennifer's `meta` library: introspection
// constants describing the running interpreter itself. Today there is one
// such constant - VERSION - but the library is the natural home for any
// future build/host/platform info.
//
// The Go package is named metalib to follow the convention used by the
// other libraries (iolib, mathlib, stringslib).
package metalib

import (
	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/version"
)

// LibraryName is the Jennifer name programs `use` to enable these constants.
const LibraryName = "meta"

// Install registers the meta library's constants on an interpreter. There
// are no functions today, but a constant alone is enough to make the
// library visible (knownLibs is keyed off Register*).
func Install(in *interpreter.Interpreter) {
	in.RegisterConst(LibraryName, "VERSION", interpreter.StringVal(version.Version))
}
