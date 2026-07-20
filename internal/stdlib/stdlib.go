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
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/archive"
	"jennifer-lang.dev/jennifer/internal/lib/compress"
	"jennifer-lang.dev/jennifer/internal/lib/convert"
	"jennifer-lang.dev/jennifer/internal/lib/crc"
	cryptolib "jennifer-lang.dev/jennifer/internal/lib/crypto"
	"jennifer-lang.dev/jennifer/internal/lib/encoding"
	"jennifer-lang.dev/jennifer/internal/lib/fs"
	gpiolib "jennifer-lang.dev/jennifer/internal/lib/gpio"
	"jennifer-lang.dev/jennifer/internal/lib/hash"
	"jennifer-lang.dev/jennifer/internal/lib/httpd"
	iiclib "jennifer-lang.dev/jennifer/internal/lib/iic"
	intllib "jennifer-lang.dev/jennifer/internal/lib/intl"
	"jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/lib/json"
	"jennifer-lang.dev/jennifer/internal/lib/lists"
	"jennifer-lang.dev/jennifer/internal/lib/maps"
	"jennifer-lang.dev/jennifer/internal/lib/math"
	"jennifer-lang.dev/jennifer/internal/lib/meta"
	"jennifer-lang.dev/jennifer/internal/lib/net"
	"jennifer-lang.dev/jennifer/internal/lib/os"
	"jennifer-lang.dev/jennifer/internal/lib/regex"
	seriallib "jennifer-lang.dev/jennifer/internal/lib/serial"
	spilib "jennifer-lang.dev/jennifer/internal/lib/spi"
	"jennifer-lang.dev/jennifer/internal/lib/strings"
	"jennifer-lang.dev/jennifer/internal/lib/task"
	termlib "jennifer-lang.dev/jennifer/internal/lib/term"
	"jennifer-lang.dev/jennifer/internal/lib/testing"
	"jennifer-lang.dev/jennifer/internal/lib/time"
	"jennifer-lang.dev/jennifer/internal/lib/toml"
	"jennifer-lang.dev/jennifer/internal/lib/uuid"
	xmllib "jennifer-lang.dev/jennifer/internal/lib/xml"
	yamllib "jennifer-lang.dev/jennifer/internal/lib/yaml"
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
	cryptolib.Install(in)
	compresslib.Install(in)
	archivelib.Install(in)
	encodinglib.Install(in)
	jsonlib.Install(in)
	tomllib.Install(in)
	xmllib.Install(in)
	yamllib.Install(in)
	intllib.Install(in)
	tasklib.Install(in)
	fslib.Install(in)
	netlib.Install(in)
	httpdlib.Install(in)
	termlib.Install(in)
	regexlib.Install(in)
	testinglib.Install(in)
	uuidlib.Install(in)
	seriallib.Install(in)
	spilib.Install(in)
	iiclib.Install(in)
	gpiolib.Install(in)
}
