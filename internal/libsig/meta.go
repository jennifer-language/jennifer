// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("meta", `
hasCapability(string) bool
call(string, any...) any
defined(string) bool
callMain(string, any...) any
definedMain(string) bool
`)
}
