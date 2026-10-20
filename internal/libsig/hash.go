// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("hash", `
compute(bytes, string) bytes
hmac(bytes, bytes, string) bytes
equal(bytes, bytes) bool
stream(string) hash.Stream
update(hash.Stream, bytes) null
finalize(hash.Stream) bytes
discard(hash.Stream) null
`)
}
