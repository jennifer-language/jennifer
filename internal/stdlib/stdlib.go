// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// Package stdlib is the single source of truth for which system libraries are
// baked into the interpreter. InstallAll registers every standard library on a
// fresh interpreter; the run / repl / profile / test entry points and the test
// harnesses all call it, so the set never drifts between them. Adding a library
// is one line here (plus the package), not an edit in every caller.
//
// This is also the seam a future build-time library-selection scheme would cut
// along (build-tag-gated entries); see docs/milestones.md.
package stdlib

import (
	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/lib/archive"
	"github.com/mplx/jennifer-lang/internal/lib/compress"
	"github.com/mplx/jennifer-lang/internal/lib/convert"
	"github.com/mplx/jennifer-lang/internal/lib/crc"
	"github.com/mplx/jennifer-lang/internal/lib/encoding"
	"github.com/mplx/jennifer-lang/internal/lib/fs"
	"github.com/mplx/jennifer-lang/internal/lib/hash"
	"github.com/mplx/jennifer-lang/internal/lib/io"
	"github.com/mplx/jennifer-lang/internal/lib/json"
	"github.com/mplx/jennifer-lang/internal/lib/lists"
	"github.com/mplx/jennifer-lang/internal/lib/maps"
	"github.com/mplx/jennifer-lang/internal/lib/math"
	"github.com/mplx/jennifer-lang/internal/lib/meta"
	"github.com/mplx/jennifer-lang/internal/lib/net"
	"github.com/mplx/jennifer-lang/internal/lib/os"
	"github.com/mplx/jennifer-lang/internal/lib/regex"
	"github.com/mplx/jennifer-lang/internal/lib/strings"
	"github.com/mplx/jennifer-lang/internal/lib/task"
	"github.com/mplx/jennifer-lang/internal/lib/testing"
	"github.com/mplx/jennifer-lang/internal/lib/time"
	"github.com/mplx/jennifer-lang/internal/lib/toml"
	"github.com/mplx/jennifer-lang/internal/lib/uuid"
)

// InstallAll activates every standard library on a fresh interpreter. Order is
// not significant (namespaces are independent); it follows rough
// introduction/topic order for readability.
func InstallAll(in *interpreter.Interpreter) {
	iolib.Install(in)
	convert.Install(in)
	mathlib.Install(in)
	stringslib.Install(in)
	listslib.Install(in)
	mapslib.Install(in)
	oslib.Install(in)
	metalib.Install(in)
	timelib.Install(in)
	hashlib.Install(in)
	crclib.Install(in)
	compresslib.Install(in)
	archivelib.Install(in)
	encodinglib.Install(in)
	jsonlib.Install(in)
	tomllib.Install(in)
	tasklib.Install(in)
	fslib.Install(in)
	netlib.Install(in)
	regexlib.Install(in)
	testinglib.Install(in)
	uuidlib.Install(in)
}
