// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("binary", `
make(int, int?) bytes
concat(bytes, bytes) bytes
join(list of bytes, bytes?) bytes
slice(bytes, int, int?) bytes
indexOf(bytes, bytes, int?, int?) int
contains(bytes, bytes) bool
split(bytes, bytes) list of bytes
startsWith(bytes, bytes) bool
endsWith(bytes, bytes) bool
`)
}
