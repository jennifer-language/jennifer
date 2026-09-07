// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build tinygo

// TinyGo terminal detection for os.isTerminal. golang.org/x/term is excluded
// from the tiny build (same as the term library), so this falls back to the
// character-device mode bit: correct for a pty and for pipes / regular-file
// redirects, wrong only for a non-terminal character device (/dev/null reads
// true here). That residual imprecision is the documented tiny-build limit; the
// default binary uses the real x/term probe.

package oslib

import (
	stdos "os"
)

// isTerminalFile approximates "is f a terminal?" by the character-device mode
// bit. A stat error reports false.
func isTerminalFile(f *stdos.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&stdos.ModeCharDevice != 0
}
