// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build tinygo && !tinygo_devtools

package main

import (
	"fmt"
	"io"
	"os"
)

// printDevUsage tells the user where the development subcommands live. Paired
// with the real listing in dump.go; the run-only TinyGo build points at the
// default binary rather than advertising commands it only rejects.
func printDevUsage(w io.Writer) {
	fmt.Fprintln(w, "  (tokens, ast, fmt, lint, profile, test, serve: use the default `jennifer` binary)")
}

// The constrained TinyGo build is a run-only interpreter: it executes
// Jennifer source (`run`, `repl`) but omits the development subcommands
// (`tokens`, `ast`, `fmt`, `lint`, `profile`, `test`). Those pull in the
// lexer-dump, AST-JSON, formatter, lint, profiler and test-runner machinery
// that add weight the minimal-footprint `jennifer-tiny` binary (embedded
// systems, minimal containers) has no use for. Each is stubbed here to a
// friendly pointer at the default `jennifer` binary, mirroring how os.run /
// net degrade under TinyGo. Build the default standard-Go `jennifer` binary
// for development work.
//
// The `tinygo_devtools` tag compiles the real subcommands into a TinyGo build
// instead (`jennifer-tiny-dev`, for CI: it runs a deck's test overlays on the
// constrained runtime). The runtime is the same either way; only these entry
// points differ.

func devToolUnavailable(name string) int {
	fmt.Fprintf(os.Stderr,
		"jennifer %s is a development feature, not available in the constrained (TinyGo) build; use the default `jennifer` binary\n",
		name)
	return 2
}

func dumpTokens(string) int        { return devToolUnavailable("tokens") }
func dumpAST(string) int           { return devToolUnavailable("ast") }
func runFmt(args []string) int     { return devToolUnavailable("fmt") }
func runLint(args []string) int    { return devToolUnavailable("lint") }
func runProfile(args []string) int { return devToolUnavailable("profile") }
func runTest(args []string) int    { return devToolUnavailable("test") }
