// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	addDataKind("encoding", "encoding")
	add("encoding", `
isAscii(bytes) bool
lenBytes(string) int
lenRunes(bytes) int
toText(string, bytes) string
fromText(string, string) bytes
encode(string, string) bytes
decode(string, bytes) string
codecs() list of string
parseForm(string) map of string to string
parseCookies(string) map of string to string
parseMultipart(string, bytes) list of encoding.FormPart
`)
}
