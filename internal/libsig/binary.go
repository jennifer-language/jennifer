// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("binary", `
make(int, int?) bytes
concat(bytes, bytes) bytes
join(list of bytes, bytes?) bytes
indexOf(bytes, bytes, int?, int?) int
contains(bytes, bytes) bool
split(bytes, bytes) list of bytes
startsWith(bytes, bytes) bool
endsWith(bytes, bytes) bool
bitXor(bytes, bytes) bytes
bitAnd(bytes, bytes) bytes
bitOr(bytes, bytes) bytes
bitNot(bytes) bytes
readInt(string, bytes, int, int) int
readUint(string, bytes, int, int) int
readFloat(string, bytes, int, int) float
readInts(string, bytes, int, int, int, int?) list of int
readFloats(string, bytes, int, int, int, int?) list of float
fromInt(string, int, int) bytes
fromUint(string, int, int) bytes
fromFloat(string, float, int) bytes
fromInts(string, list of int, int) bytes
fromFloats(string, list of float, int) bytes
setInt(string, bytes, int, int, int) bytes
setUint(string, bytes, int, int, int) bytes
setFloat(string, bytes, int, float, int) bytes
`)
}
