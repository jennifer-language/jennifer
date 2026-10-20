// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("io", `
# one value prints its display form; more than one needs a format string first
printf(any) null
printf(string, any...) null
eprintf(any) null
eprintf(string, any...) null
sprintf(any) string
sprintf(string, any...) string
readLine(string?) string
eof() bool
readBytes(int) bytes
readChars(int) string
readLines() list of string
`)
}
