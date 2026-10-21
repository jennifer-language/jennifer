// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("yaml", `
# shared document surface (internal/lib/doctree)
typeOf(yaml.Value, string?) string
get(yaml.Value, string?) yaml.Value
has(yaml.Value, string) bool
keys(yaml.Value, string?) list of string
length(yaml.Value, string?) int
asInt(yaml.Value, string?) int
asFloat(yaml.Value, string?) float
asString(yaml.Value, string?) string
asBool(yaml.Value, string?) bool
isNull(yaml.Value, string?) bool
isDatetime(yaml.Value, string?) bool
asDatetime(yaml.Value, string?) time.Time
list(list of any?) yaml.Value
map(map of string to any?) yaml.Value
set(yaml.Value, string, any) yaml.Value
insert(yaml.Value, string, any) yaml.Value
append(yaml.Value, string, any) yaml.Value
remove(yaml.Value, string) yaml.Value
move(yaml.Value, string, string) yaml.Value
decode(string) yaml.Value
decodeAll(string) list of yaml.Value
encode(any) string
encodePretty(any) string
`)
}
