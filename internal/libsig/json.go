// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("json", `
# shared document surface (internal/lib/doctree)
typeOf(json.Value, string?) string
get(json.Value, string?) json.Value
has(json.Value, string) bool
keys(json.Value, string?) list of string
length(json.Value, string?) int
asInt(json.Value, string?) int
asFloat(json.Value, string?) float
asString(json.Value, string?) string
asBool(json.Value, string?) bool
isNull(json.Value, string?) bool
list() json.Value
map() json.Value
set(json.Value, string, any) json.Value
insert(json.Value, string, any) json.Value
append(json.Value, string, any) json.Value
remove(json.Value, string) json.Value
move(json.Value, string, string) json.Value
encode(any) string
encodePretty(any) string
decode(string) json.Value
`)
}
