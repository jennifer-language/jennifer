// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("path", `
base(string) string
dir(string) string
ext(string) string
stem(string) string
join(string, string...) string
clean(string) string
isAbs(string) bool
split(string) list of string
`)
}
