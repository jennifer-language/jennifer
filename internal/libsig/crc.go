// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("crc", `
compute(bytes, string) bytes
stream(string) crc.Stream
update(crc.Stream, bytes) null
finalize(crc.Stream) bytes
discard(crc.Stream) null
`)
}
