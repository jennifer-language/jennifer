// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	addDataKind("toml", "toml")
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
asFloats(toml.Value, string?) list of float
asInts(toml.Value, string?) list of int
asStrings(toml.Value, string?) list of string
asBool(toml.Value, string?) bool
asDecimal(toml.Value, string?) decimal.Decimal
isDatetime(toml.Value, string?) bool
asDatetime(toml.Value, string?) time.Time
asLocal(toml.Value, string?) toml.Local
list(list of any?) toml.Value
map(map of string to any?) toml.Value
set(toml.Value, string, any) toml.Value
insert(toml.Value, string, any) toml.Value
append(toml.Value, string, any) toml.Value
remove(toml.Value, string) toml.Value
move(toml.Value, string, string) toml.Value
decode(string) toml.Value
decodeWith(string, toml.DecodeOptions) toml.Value
encode(any) string
encodePretty(any) string
`)
}
