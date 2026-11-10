// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("archive", `
pack(string, list of archive.Entry) bytes
unpack(string, bytes) list of archive.Entry
unpackWith(string, bytes, archive.UnpackOptions) list of archive.Entry
`)
}
