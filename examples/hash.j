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
def input as bytes init convert.bytesFromString("utf-8", "abc");
io.printf("md5(abc)    = %s\n", encoding.toText("hex", hash.compute("md5", $input)));
io.printf("sha1(abc)   = %s\n", encoding.toText("hex", hash.compute("sha1", $input)));
io.printf("sha256(abc) = %s\n", encoding.toText("hex", hash.compute("sha256", $input)));
io.printf("crc32(abc)  = %s\n", encoding.toText("hex", crc.compute("crc32", $input)));
io.printf("crc64(abc)  = %s\n", encoding.toText("hex", crc.compute("crc64", $input)));

# --- Stream the same input in two chunks ---
def s as hash.Stream init hash.stream("sha256");
def chunkOne as bytes init convert.bytesFromString("utf-8", "ab");
def chunkTwo as bytes init convert.bytesFromString("utf-8", "c");
hash.update($s, $chunkOne);
hash.update($s, $chunkTwo);
def streamed as bytes init hash.finalize($s);
io.printf("sha256 streamed = %s\n", encoding.toText("hex", $streamed));

# --- CRC stream too ---
def cs as crc.Stream init crc.stream("crc32");
crc.update($cs, $chunkOne);
crc.update($cs, $chunkTwo);
def streamedCrc as bytes init crc.finalize($cs);
io.printf("crc32 streamed  = %s\n", encoding.toText("hex", $streamedCrc));

# --- Digest widths ---
def widthMd5 as bytes init hash.compute("md5", $input);
def widthSha1 as bytes init hash.compute("sha1", $input);
def widthSha256 as bytes init hash.compute("sha256", $input);
def widthCrc32 as bytes init crc.compute("crc32", $input);
def widthCrc64 as bytes init crc.compute("crc64", $input);
io.printf(
    "widths: md5=%d sha1=%d sha256=%d crc32=%d crc64=%d\n",
    len($widthMd5),
    len($widthSha1),
    len($widthSha256),
    len($widthCrc32),
    len($widthCrc64));
