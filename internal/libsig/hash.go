// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("hash", `
compute(string, bytes) bytes
hmac(string, bytes, bytes) bytes
equal(bytes, bytes) bool
stream(string) hash.Stream
update(hash.Stream, bytes) null
finalize(hash.Stream) bytes
discard(hash.Stream) null
`)
}
