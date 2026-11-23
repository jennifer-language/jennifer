// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	addDataKind("io", "io")
	add("io", `
# the first argument is always the format (one non-string value: lint L302)
printf(string, any...) null
eprintf(string, any...) null
sprintf(string, any...) string
readLine(string?) string
eof() bool
readBytes(int) bytes
readChars(int) string
readLines() list of string
`)
}
