# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
def cafeBytes as bytes init convert.bytesFromString($cafe, "utf-8");
io.printf("len(cafe runes) = %d\n", len($cafe));
io.printf("encoding.lenBytes(cafe) = %d\n", encoding.lenBytes($cafe));
io.printf("encoding.lenRunes(bytes) = %d\n", encoding.lenRunes($cafeBytes));
io.printf("encoding.isAscii(cafe) = %t\n", encoding.isAscii($cafeBytes));
io.printf("encoding.isAscii(hello) = %t\n",
    encoding.isAscii(convert.bytesFromString("hello", "utf-8")));

# --- toText / fromText: hex ---
def hi as bytes init convert.bytesFromString("Hi!", "utf-8");
io.printf("toText hex(Hi!) = %s\n", encoding.toText($hi, "hex"));
def back as bytes init encoding.fromText("4869210a", "hex");
io.printf("fromText hex roundtrip len = %d\n", len($back));

# --- toText / fromText: base64 variants ---
def src as bytes init convert.bytesFromString("Hello", "utf-8");
io.printf("toText base64     = %s\n", encoding.toText($src, "base64"));
io.printf("toText base64-url = %s\n", encoding.toText($src, "base64-url"));
def roundStd as bytes init encoding.fromText("SGVsbG8=", "base64");
io.printf("fromText base64 = %s\n", convert.stringFromBytes($roundStd, "utf-8"));

# --- toText / fromText: quoted-printable (RFC 2045 MIME transfer encoding) ---
def qpIn as bytes init convert.bytesFromString("a = café", "utf-8");
def qp as string init encoding.toText($qpIn, "quoted-printable");
io.printf("toText quoted-printable = %s\n", $qp);
def qpBack as bytes init encoding.fromText($qp, "quoted-printable");
io.printf("qp round-trips = %t\n", convert.stringFromBytes($qpBack, "utf-8") == "a = café");

# --- Codec list ---
def codecs as list of string init encoding.codecs();
for (def name in $codecs) {
    io.printf("codec: %s\n", $name);
}

# --- ISO-8859-1 round-trip ---
def latin as bytes init encoding.encode($cafe, "iso-8859-1");
io.printf("encode iso-8859-1 'café' bytes = %d %d %d %d\n",
    $latin[0], $latin[1], $latin[2], $latin[3]);
io.printf("decode iso-8859-1 = %s\n", encoding.decode($latin, "iso-8859-1"));

# --- Windows-1252: 0x80 is the EURO SIGN (versus Latin-1's C1 control) ---
def euroBytes as bytes init encoding.encode("€100", "windows-1252");
io.printf("encode windows-1252 '€100' first byte = %d\n", $euroBytes[0]);
io.printf("decode back = %s\n", encoding.decode($euroBytes, "windows-1252"));

# --- EBCDIC IBM-1047 ---
def helloEbcdic as bytes init encoding.encode("Hello", "ebcdic");
io.printf("encode ebcdic 'Hello' bytes = %d %d %d %d %d\n",
    $helloEbcdic[0], $helloEbcdic[1], $helloEbcdic[2],
    $helloEbcdic[3], $helloEbcdic[4]);
io.printf("decode ebcdic = %s\n", encoding.decode($helloEbcdic, "ebcdic"));

# --- Codec names are exact (canonical only, no IANA aliases) ---
try {
    encoding.decode($latin, "ISO-8859-1");   # an alias, not the canonical name
    io.printf("ISO-8859-1: unexpectedly accepted\n");
} catch (aliasErr) {
    io.printf("codec names are exact: use 'iso-8859-1', not 'ISO-8859-1'\n");
}
