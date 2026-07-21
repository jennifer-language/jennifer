# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * gzip / zlib / deflate round-trips + streaming.
 * The compressed bytes vary with the zlib version, so this prints only stable
 * facts (round-trip success, that compressible data shrank).
 * @module compress
 */

use io;
use compress;
use convert;

def text as string init "jennifer jennifer jennifer jennifer jennifer jennifer";
def raw as bytes init convert.bytesFromString($text, "utf-8");

def g as bytes init compress.unpack(compress.pack($raw, "gzip"), "gzip");
io.printf("gzip round-trip:    %t\n", convert.stringFromBytes($g, "utf-8") == $text);
def z as bytes init compress.unpack(compress.pack($raw, "zlib"), "zlib");
io.printf("zlib round-trip:    %t\n", convert.stringFromBytes($z, "utf-8") == $text);
def d as bytes init compress.unpack(compress.pack($raw, "deflate"), "deflate");
io.printf("deflate round-trip: %t\n", convert.stringFromBytes($d, "utf-8") == $text);

def best as int init len(compress.pack($raw, "gzip", "best"));
def fast as int init len(compress.pack($raw, "gzip", "fast"));
io.printf("best <= fast size:  %t\n", $best <= $fast);
io.printf("compressible shrank:%t\n", len(compress.pack($raw, "gzip")) < len($raw));

# streaming, in two chunks, matches a one-shot pack of the whole input
def s as compress.Stream init compress.stream("gzip");
compress.update($s, convert.bytesFromString("jennifer jennifer jennifer ", "utf-8"));
compress.update($s, convert.bytesFromString("jennifer jennifer jennifer", "utf-8"));
def st as bytes init compress.unpack(compress.finalize($s), "gzip");
io.printf("stream round-trip:  %t\n", convert.stringFromBytes($st, "utf-8") == $text);
