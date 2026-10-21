// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("asn1", `
decode(bytes) asn1.Value
encode(asn1.Value) bytes
typeOf(asn1.Value, string?) string
tagClass(asn1.Value, string?) string
tagNumber(asn1.Value, string?) int
isConstructed(asn1.Value, string?) bool
get(asn1.Value, string?) asn1.Value
has(asn1.Value, string) bool
length(asn1.Value, string?) int
asInt(asn1.Value, string?) int
asBool(asn1.Value, string?) bool
asString(asn1.Value, string?) string
asBytes(asn1.Value, string?) bytes
asOid(asn1.Value, string?) string
isNull(asn1.Value, string?) bool
integer(int) asn1.Value
enumerated(int) asn1.Value
boolean(bool) asn1.Value
null() asn1.Value
octetString(bytes) asn1.Value
utf8String(string) asn1.Value
printableString(string) asn1.Value
ia5String(string) asn1.Value
oid(string) asn1.Value
sequence(list of asn1.Value) asn1.Value
set(list of asn1.Value) asn1.Value
tagged(string, int, asn1.Value) asn1.Value
retag(string, int, asn1.Value) asn1.Value
`)
}
