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
bitXor(bytes, bytes) bytes
bitAnd(bytes, bytes) bytes
bitOr(bytes, bytes) bytes
bitNot(bytes) bytes
readInt(bytes, int, int, string) int
readUint(bytes, int, int, string) int
readFloat(bytes, int, int, string) float
readInts(bytes, int, int, int, string, int?) list of int
readFloats(bytes, int, int, int, string, int?) list of float
`)
}
