// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("xml", `
decode(string) xml.Value
encode(xml.Value) string
encodePretty(xml.Value) string
typeOf(xml.Value) string
tag(xml.Value) string
text(xml.Value) string
attr(xml.Value, string) string
hasAttr(xml.Value, string) bool
attrs(xml.Value) list of string
children(xml.Value) list of xml.Value
get(xml.Value, string) xml.Value
findAll(xml.Value, string) list of xml.Value
has(xml.Value, string) bool
element(string) xml.Value
setAttr(xml.Value, string, string) xml.Value
setText(xml.Value, string) xml.Value
append(xml.Value, xml.Value) xml.Value
`)
}
