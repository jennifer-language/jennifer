// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("archive", `
pack(list of archive.Entry, string) bytes
unpack(bytes, string) list of archive.Entry
unpackWith(bytes, string, archive.UnpackOptions) list of archive.Entry
`)
}
