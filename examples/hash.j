# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Exercises the `hash` + `crc` libraries.
 * Both libraries use the codec-table shape: a single function per category
 * with the algorithm as a string argument. This example carries a tiny
 * `bytesToHex` helper so the digests are printable for a golden comparison.
 * @module hash
 */

use io;
use convert;
use strings;
use lists;
use hash;
use crc;

/**
 * Lowercase hexadecimal encoding of a byte string.
 * @param b {bytes} the bytes to encode
 * @return {string} two hex digits per input byte
 */
func bytesToHex(b as bytes) {
    def hexchars as string init "0123456789abcdef";
    def out as list of string init [];
    def i as int init 0;
    while ($i < len($b)) {
        def v as int init $b[$i];
        $out[] = strings.substring($hexchars, $v // 16, $v // 16 + 1);
        $out[] = strings.substring($hexchars, $v % 16, $v % 16 + 1);
        $i = $i + 1;
    }
    return strings.join($out, "");
}

# --- One-shot digests over a known string ---
def input as bytes init convert.bytesFromString("abc", "utf-8");
io.printf("md5(abc)    = %s\n", bytesToHex(hash.compute($input, "md5")));
io.printf("sha1(abc)   = %s\n", bytesToHex(hash.compute($input, "sha1")));
io.printf("sha256(abc) = %s\n", bytesToHex(hash.compute($input, "sha256")));
io.printf("crc32(abc)  = %s\n", bytesToHex(crc.compute($input, "crc32")));
io.printf("crc64(abc)  = %s\n", bytesToHex(crc.compute($input, "crc64")));

# --- Stream the same input in two chunks ---
def s as hash.Stream init hash.stream("sha256");
def chunkOne as bytes init convert.bytesFromString("ab", "utf-8");
def chunkTwo as bytes init convert.bytesFromString("c", "utf-8");
hash.update($s, $chunkOne);
hash.update($s, $chunkTwo);
def streamed as bytes init hash.finalize($s);
io.printf("sha256 streamed = %s\n", bytesToHex($streamed));

# --- CRC stream too ---
def cs as crc.Stream init crc.stream("crc32");
crc.update($cs, $chunkOne);
crc.update($cs, $chunkTwo);
def streamedCrc as bytes init crc.finalize($cs);
io.printf("crc32 streamed  = %s\n", bytesToHex($streamedCrc));

# --- Digest widths ---
def widthMd as bytes init hash.compute($input, "md5");
def widthShaOne as bytes init hash.compute($input, "sha1");
def widthShaTwoFiveSix as bytes init hash.compute($input, "sha256");
def widthCrcThirtyTwo as bytes init crc.compute($input, "crc32");
def widthCrcSixtyFour as bytes init crc.compute($input, "crc64");
io.printf("widths: md5=%d sha1=%d sha256=%d crc32=%d crc64=%d\n",
    len($widthMd), len($widthShaOne), len($widthShaTwoFiveSix),
    len($widthCrcThirtyTwo), len($widthCrcSixtyFour));
