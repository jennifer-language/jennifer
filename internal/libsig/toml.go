// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("toml", `
# shared document surface (internal/lib/doctree)
typeOf(toml.Value, string?) string
get(toml.Value, string?) toml.Value
has(toml.Value, string) bool
keys(toml.Value, string?) list of string
length(toml.Value, string?) int
asInt(toml.Value, string?) int
asFloat(toml.Value, string?) float
asString(toml.Value, string?) string
asBool(toml.Value, string?) bool
isDatetime(toml.Value, string?) bool
asDatetime(toml.Value, string?) time.Time
list() toml.Value
map() toml.Value
set(toml.Value, string, any) toml.Value
insert(toml.Value, string, any) toml.Value
append(toml.Value, string, any) toml.Value
remove(toml.Value, string) toml.Value
move(toml.Value, string, string) toml.Value
decode(string) toml.Value
encode(any) string
encodePretty(any) string
`)
}
