// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("compress", `
pack(bytes, string, string?) bytes
unpack(bytes, string, int?) bytes
stream(string, string?) compress.Stream
update(compress.Stream, bytes) null
finalize(compress.Stream) bytes
discard(compress.Stream) null
`)
}
