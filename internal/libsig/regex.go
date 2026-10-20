// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("regex", `
matches(string, string) bool
find(string, string) regex.Match
findAll(string, string) list of regex.Match
replace(string, string, string) string
split(string, string) list of string
escape(string) string
`)
}
