// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	addDataKind("convert", "convert")
	add("convert", `
toInt(int|float|string|bool) int
toInt(string, int) int
toFloat(int|float|string|bool) float
toString(any) string
toBool(bool|int|float|string) bool
typeOf(any) string
objectType(any) string
toCodepoint(string) int
fromCodepoint(int) string
`)
}
