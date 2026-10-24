// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("strings", `
upper(string) string
lower(string) string
fold(string) string
contains(string, string) bool
startsWith(string, string) bool
endsWith(string, string) bool
indexOf(string, string, int?) int
lastIndexOf(string, string) int
count(string, string) int
trim(string) string
trimLeft(string) string
trimRight(string) string
replace(string, string, string) string
repeat(string, int) string
substring(string, int, int?) string
split(string, string) list of string
chars(string) list of string
join(list of string, string) string
`)
}
