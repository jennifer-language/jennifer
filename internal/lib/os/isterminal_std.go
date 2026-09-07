// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

// Standard-Go terminal detection for os.isTerminal, over golang.org/x/term
// (already a dependency of the term library). term.IsTerminal issues the real
// terminal-ioctl probe, so a redirect from a non-terminal character device
// (/dev/null, /dev/zero, /dev/random) correctly reads false - unlike the
// character-device mode bit, which the tiny build falls back to.

package oslib

import (
	stdos "os"

	"golang.org/x/term"
)

// isTerminalFile reports whether f is an interactive terminal. A pipe, a
// regular-file redirect, and a non-terminal character device all report false;
// a real terminal (including a pty) reports true. A closed or un-probeable
// descriptor reports false.
func isTerminalFile(f *stdos.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
