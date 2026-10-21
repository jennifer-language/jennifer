// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("term", `
makeRaw(string) term.State
restore(term.State) null
size(string) term.Size
readByte() int
readByteTimeout(int) int
`)
}
