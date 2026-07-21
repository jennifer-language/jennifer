// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package version exposes the interpreter's build version as a single
// string. The value is injected at build time via `-ldflags -X` (see the
// Makefile and scripts/version.sh); a plain `go build` / `go run` /
// `go test` leaves it at the default of "dev".
package version

// Version is the project's build version. Format:
//
//   - "<tag>"                       when HEAD is exactly on a semver tag
//   - "<tag>-dev+<N>.<shortsha>"    when HEAD is N commits past a tag
//   - "0.0.0-dev+<N>.<shortsha>"    when no tag exists yet
//   - "dev"                         when built without the version ldflag
//
// The string is read in two places: the CLI's `help` output and the
// `core` library's `JENNIFER_VERSION` constant (referenced from Jennifer
// code as the bare identifier `JENNIFER_VERSION` - the `core` library
// that owns it is auto-loaded so no `use` statement is needed).
var Version = "dev"
