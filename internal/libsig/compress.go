// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("compress", `
pack(string, bytes, string?) bytes
unpack(string, bytes, int?) bytes
stream(string, string?) compress.Stream
update(compress.Stream, bytes) null
finalize(compress.Stream) bytes
discard(compress.Stream) null
`)
}
