#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * The bloom module (modules/bloom.j): a probabilistic set. Add a set of known
 * words, then test membership for a mix of known and unknown words - members
 * always report present, non-members almost always report absent. Pure `.j`;
 * runs on both binaries.
 * Run: jennifer run examples/modules/bloom_demo.j
 * @module bloom_demo
 */
use io;
import "../../modules/bloom.j" as bloom;

def known as list of string init ["apple", "banana", "cherry", "date", "elderberry"];
def f as bloom.Filter init bloom.addAll(bloom.new(4096, 6), $known);
io.printf("added %d words to a %d-bit filter (k=%d)\n", len($known), $f.size, $f.hashes);

def queries as list of string init ["apple", "cherry", "fig", "grape", "banana", "kiwi"];
for (def w in $queries) {
    def hit as bool init bloom.mightContain($f, $w);
    if ($hit) {
        io.printf("  %s|pad=12 -> possibly present\n", $w);
    } else {
        io.printf("  %s|pad=12 -> definitely absent\n", $w);
    }
}
