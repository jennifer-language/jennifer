// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("gpio", `
chip(string) null
setup(int, string) null
read(int) int
write(int, int) null
release(int) null
`)
}
