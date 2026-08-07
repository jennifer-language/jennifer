# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

# ASN.1 BER/DER encode and decode with the `asn1` library. Build a structured
# value with typed constructors, serialise it to DER bytes, then walk the
# decoded tree by child-index pointer.

use io;
use asn1;
use convert;
use encoding;

# A small SEQUENCE mixing several universal types plus a context tag - the
# shape an LDAP or SNMP field takes on the wire.
def msg as asn1.Value init asn1.sequence([
    asn1.integer(42),
    asn1.octetString(convert.bytesFromString("hello", "utf-8")),
    asn1.oid("1.2.840.113549.1.1.11"),
    asn1.boolean(true),
    asn1.tagged("context", 0, asn1.integer(7))
]);

io.printf("value: %v\n", $msg);

# Serialise to canonical DER.
def wire as bytes init asn1.encode($msg);
io.printf("DER:   %s\n", encoding.toText($wire, "hex"));

# Decode the bytes back and walk the tree. Pointer tokens are child indices.
def tree as asn1.Value init asn1.decode($wire);
io.printf("root:  %s with %d children\n", asn1.typeOf($tree), asn1.length($tree));
io.printf("[0]    integer = %d\n", asn1.asInt($tree, "/0"));
io.printf("[1]    string  = %s\n", asn1.asString($tree, "/1"));
io.printf("[2]    oid     = %s\n", asn1.asOid($tree, "/2"));
io.printf("[3]    bool    = %t\n", asn1.asBool($tree, "/3"));
io.printf(
    "[4]    %s tag %d, inner = %d\n",
    asn1.tagClass($tree, "/4"),
    asn1.tagNumber($tree, "/4"),
    asn1.asInt($tree, "/4/0"));

# Encode / decode is a stable round-trip.
io.printf(
    "stable round-trip: %t\n",
    encoding.toText($wire, "hex") == encoding.toText(asn1.encode($tree), "hex"));

# Malformed input is a catchable error, never a crash.
def truncated as bytes;
$truncated[] = 0x02;
$truncated[] = 0x05;
try {
    asn1.decode($truncated);
} catch (e) {
    io.printf("bad input: %v\n", $e.message);
}
