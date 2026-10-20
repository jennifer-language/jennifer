// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("convert", `
toInt(int|float|string|bool) int
toInt(string, int) int
toFloat(int|float|string|bool) float
toString(any) string
toString(int, int) string
toBool(bool|int|float|string) bool
typeOf(any) string
objectType(any) string
bytesFromString(string, string) bytes
stringFromBytes(bytes, string) string
toCodepoint(string) int
fromCodepoint(int) string
`)
}
