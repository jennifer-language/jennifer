// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("uuid", `
v4() string
v7() string
parse(string) bytes
isValid(string) bool
version(string) int
`)
}
