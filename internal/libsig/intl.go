// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("intl", `
load(string, map of string to string) null
setLocale(string) null
locale() string
tr(string, map of string to any?) string
`)
}
