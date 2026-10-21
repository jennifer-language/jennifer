# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * Exercises the `hash` + `crc` libraries.
 * Both libraries use the codec-table shape: a single function per category
 * with the algorithm as a string argument. Digests are hex-encoded with
 * `encoding.toText` so they are printable for a golden comparison.
 * @module hash
 */

use io;
use convert;
use encoding;
use hash;
use crc;

# --- One-shot digests over a known string ---
def input as bytes init convert.bytesFromString("abc", "utf-8");
io.printf("md5(abc)    = %s\n", encoding.toText(hash.compute($input, "md5"), "hex"));
io.printf("sha1(abc)   = %s\n", encoding.toText(hash.compute($input, "sha1"), "hex"));
io.printf("sha256(abc) = %s\n", encoding.toText(hash.compute($input, "sha256"), "hex"));
io.printf("crc32(abc)  = %s\n", encoding.toText(crc.compute($input, "crc32"), "hex"));
io.printf("crc64(abc)  = %s\n", encoding.toText(crc.compute($input, "crc64"), "hex"));

# --- Stream the same input in two chunks ---
def s as hash.Stream init hash.stream("sha256");
def chunkOne as bytes init convert.bytesFromString("ab", "utf-8");
def chunkTwo as bytes init convert.bytesFromString("c", "utf-8");
hash.update($s, $chunkOne);
hash.update($s, $chunkTwo);
def streamed as bytes init hash.finalize($s);
io.printf("sha256 streamed = %s\n", encoding.toText($streamed, "hex"));

# --- CRC stream too ---
def cs as crc.Stream init crc.stream("crc32");
crc.update($cs, $chunkOne);
crc.update($cs, $chunkTwo);
def streamedCrc as bytes init crc.finalize($cs);
io.printf("crc32 streamed  = %s\n", encoding.toText($streamedCrc, "hex"));

# --- Digest widths ---
def widthMd5 as bytes init hash.compute($input, "md5");
def widthSha1 as bytes init hash.compute($input, "sha1");
def widthSha256 as bytes init hash.compute($input, "sha256");
def widthCrc32 as bytes init crc.compute($input, "crc32");
def widthCrc64 as bytes init crc.compute($input, "crc64");
io.printf(
    "widths: md5=%d sha1=%d sha256=%d crc32=%d crc64=%d\n",
    len($widthMd5),
    len($widthSha1),
    len($widthSha256),
    len($widthCrc32),
    len($widthCrc64));
