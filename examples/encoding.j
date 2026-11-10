# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * Exercises the `encoding` library.
 * Covers introspection helpers, hex / base64 round-trips through
 * `toText`/`fromText`, and the codec table for ascii, latin-1, windows-1252,
 * and ebcdic.
 * @module encoding
 */

use io;
use convert;
use encoding;

# --- Introspection ---
def cafe as string init "café";
def cafeBytes as bytes init convert.bytesFromString("utf-8", $cafe);
io.printf("len(cafe runes) = %d\n", len($cafe));
io.printf("encoding.lenBytes(cafe) = %d\n", encoding.lenBytes($cafe));
io.printf("encoding.lenRunes(bytes) = %d\n", encoding.lenRunes($cafeBytes));
io.printf("encoding.isAscii(cafe) = %t\n", encoding.isAscii($cafeBytes));
io.printf(
    "encoding.isAscii(hello) = %t\n",
    encoding.isAscii(convert.bytesFromString("utf-8", "hello")));

# --- toText / fromText: hex ---
def hi as bytes init convert.bytesFromString("utf-8", "Hi!");
io.printf("toText hex(Hi!) = %s\n", encoding.toText("hex", $hi));
def back as bytes init encoding.fromText("hex", "4869210a");
io.printf("fromText hex roundtrip len = %d\n", len($back));

# --- toText / fromText: base64 variants ---
def src as bytes init convert.bytesFromString("utf-8", "Hello");
io.printf("toText base64     = %s\n", encoding.toText("base64", $src));
io.printf("toText base64-url = %s\n", encoding.toText("base64-url", $src));
def roundStd as bytes init encoding.fromText("base64", "SGVsbG8=");
io.printf("fromText base64 = %s\n", convert.stringFromBytes("utf-8", $roundStd));

# --- toText / fromText: quoted-printable (RFC 2045 MIME transfer encoding) ---
def qpIn as bytes init convert.bytesFromString("utf-8", "a = café");
def qp as string init encoding.toText("quoted-printable", $qpIn);
io.printf("toText quoted-printable = %s\n", $qp);
def qpBack as bytes init encoding.fromText("quoted-printable", $qp);
io.printf("qp round-trips = %t\n", convert.stringFromBytes("utf-8", $qpBack) == "a = café");

# --- Codec list ---
def codecs as list of string init encoding.codecs();
for (def name in $codecs) {
    io.printf("codec: %s\n", $name);
}

# --- ISO-8859-1 round-trip ---
def latin as bytes init encoding.encode("iso-8859-1", $cafe);
io.printf(
    "encode iso-8859-1 'café' bytes = %d %d %d %d\n",
    $latin[0],
    $latin[1],
    $latin[2],
    $latin[3]);
io.printf("decode iso-8859-1 = %s\n", encoding.decode("iso-8859-1", $latin));

# --- Windows-1252: 0x80 is the EURO SIGN (versus Latin-1's C1 control) ---
def euroBytes as bytes init encoding.encode("windows-1252", "€100");
io.printf("encode windows-1252 '€100' first byte = %d\n", $euroBytes[0]);
io.printf("decode back = %s\n", encoding.decode("windows-1252", $euroBytes));

# --- EBCDIC IBM-1047 ---
def helloEbcdic as bytes init encoding.encode("ebcdic", "Hello");
io.printf(
    "encode ebcdic 'Hello' bytes = %d %d %d %d %d\n",
    $helloEbcdic[0],
    $helloEbcdic[1],
    $helloEbcdic[2],
    $helloEbcdic[3],
    $helloEbcdic[4]);
io.printf("decode ebcdic = %s\n", encoding.decode("ebcdic", $helloEbcdic));

# --- Codec names are exact (canonical only, no IANA aliases) ---
try {
    encoding.decode("ISO-8859-1", $latin); # an alias, not the canonical name
    io.printf("ISO-8859-1: unexpectedly accepted\n");
} catch (aliasErr) {
    io.printf("codec names are exact: use 'iso-8859-1', not 'ISO-8859-1'\n");
}
