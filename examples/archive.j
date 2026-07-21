# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * tar / zip / tar.gz bundle round-trips.
 * The packed bytes vary with the zlib version, so this prints only stable
 * facts (entry count, names, and decoded content survive a pack/unpack cycle).
 * @module archive
 */

use io;
use archive;
use convert;

def alpha as bytes init convert.bytesFromString("hello", "utf-8");
def bravo as bytes init convert.bytesFromString("read me", "utf-8");
def one as archive.Entry init archive.Entry{
    name: "hi.txt", data: $alpha, mode: 0o644, mtime: 1700000000
};
def two as archive.Entry init archive.Entry{
    name: "docs/readme.txt", data: $bravo, mode: 0o644, mtime: 1700000000
};
def es as list of archive.Entry init [$one, $two];

for (def fmt in ["tar", "zip", "tar.gz"]) {
    def back as list of archive.Entry init archive.unpack(archive.pack($es, $fmt), $fmt);
    def first as string init convert.stringFromBytes($back[0].data, "utf-8");
    io.printf("%s: entries=%d first=%s:%s\n", $fmt, len($back), $back[0].name, $first);
}
