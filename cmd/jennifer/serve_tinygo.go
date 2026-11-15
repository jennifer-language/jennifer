// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build tinygo

package main

import (
	"fmt"
	"io"
	"os"
)

// printServeUsage is the serve line of the development usage text in
// jennifer-tiny-dev (the `tinygo_devtools` build), which carries the other
// development subcommands but not serve.
func printServeUsage(w io.Writer) {
	fmt.Fprintln(w, "  (serve: use the default `jennifer` binary)")
}

// runServe needs the network stack (the httpd engine) and, for --watch,
// os/exec - neither present in the constrained TinyGo build, with or without
// the `tinygo_devtools` tag.
func runServe(args []string) int {
	fmt.Fprintln(os.Stderr,
		"jennifer serve needs the network stack; not available in the constrained (TinyGo) build; use the default `jennifer` binary")
	return 2
}
