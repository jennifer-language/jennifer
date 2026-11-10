# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * gzip / zlib / deflate round-trips + streaming.
 * The compressed bytes vary with the zlib version, so this prints only stable
 * facts (round-trip success, that compressible data shrank).
 * @module compress
 */

use io;
use compress;
use convert;
use strings;

def text as string init "jennifer jennifer jennifer jennifer jennifer jennifer";
def raw as bytes init convert.bytesFromString("utf-8", $text);

def g as bytes init compress.unpack("gzip", compress.pack("gzip", $raw));
io.printf("gzip round-trip:    %t\n", convert.stringFromBytes("utf-8", $g) == $text);
def z as bytes init compress.unpack("zlib", compress.pack("zlib", $raw));
io.printf("zlib round-trip:    %t\n", convert.stringFromBytes("utf-8", $z) == $text);
def d as bytes init compress.unpack("deflate", compress.pack("deflate", $raw));
io.printf("deflate round-trip: %t\n", convert.stringFromBytes("utf-8", $d) == $text);

def best as int init len(compress.pack("gzip", $raw, "best"));
def fast as int init len(compress.pack("gzip", $raw, "fast"));
io.printf("best <= fast size:  %t\n", $best <= $fast);

# A realistically-sized compressible input shrinks. A few dozen bytes is too
# small to beat gzip's ~18-byte framing at the default level, so use a larger
# buffer here rather than $raw for a stable demonstration.
def bulk as bytes init convert.bytesFromString("utf-8", strings.repeat("jennifer ", 40));
io.printf("compressible shrank:%t\n", len(compress.pack("gzip", $bulk)) < len($bulk));

# streaming, in two chunks, matches a one-shot pack of the whole input
def s as compress.Stream init compress.stream("gzip");
compress.update($s, convert.bytesFromString("utf-8", "jennifer jennifer jennifer "));
compress.update($s, convert.bytesFromString("utf-8", "jennifer jennifer jennifer"));
def st as bytes init compress.unpack("gzip", compress.finalize($s));
io.printf("stream round-trip:  %t\n", convert.stringFromBytes("utf-8", $st) == $text);
