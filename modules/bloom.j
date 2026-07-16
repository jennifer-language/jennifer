# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * A Bloom filter: a compact, probabilistic set. `add` records a string;
 * `mightContain` tests membership with **no false negatives** (a member always
 * reports true) but possible **false positives** (a non-member may report true,
 * with a probability that grows as the filter fills). The bit array is packed
 * into `bytes`; the `hashes` positions per item come from double-hashing one
 * SHA-256 digest (`pos_i = (h1 + i*h2) mod size`), so a single hash yields all k
 * positions.
 *
 * Value-semantic: `add` returns a fresh filter (the bit array is copied), so
 * chain adds (`$f = bloom.add($f, x)`). Over `hash` + `bytes`; runs on both
 * binaries.
 * @module bloom
 * @example
 * import "bloom.j" as bloom;
 * def f as bloom.Filter init bloom.new(1024, 4);
 * $f = bloom.add($f, "alice");
 * $f = bloom.add($f, "bob");
 * bloom.mightContain($f, "alice");   # true
 * bloom.mightContain($f, "carol");   # almost always false
 */
use hash;
use convert;
use lists;

/**
 * A Bloom filter.
 * @field bits {bytes} the packed bit array
 * @field size {int} the number of bits
 * @field hashes {int} the number of hash positions per item (k)
 */
export def struct Filter {
    bits as bytes,
    size as int,
    hashes as int
};

func fail(msg as string) {
    throw Error{ kind: "bloom", message: "bloom: " + $msg, file: "", line: 0, col: 0 };
}

# readLong reads a 32-bit big-endian value from a digest at an offset.
func readLong(buf as bytes, off as int) {
    return ($buf[$off] << 24) | ($buf[$off + 1] << 16) | ($buf[$off + 2] << 8) | $buf[$off + 3];
}

# positions returns the k bit positions for an item via double hashing.
func positions(item as string, size as int, hashes as int) {
    def digest as bytes init hash.compute(convert.bytesFromString($item, "utf-8"), "sha256");
    def h as int init readLong($digest, 0);
    def g as int init readLong($digest, 4);
    # Guard the double-hash step: if g is 0 (mod size) every position collapses
    # to the same bit, degrading the filter to a single hash. Force a non-zero
    # step (Kirsch-Mitzenmacher).
    def step as int init $g % $size;
    if ($step == 0) {
        $step = 1;
    }
    def out as list of int init [];
    def i as int init 0;
    while ($i < $hashes) {
        $out[] = ($h + $i * $step) % $size;
        $i = $i + 1;
    }
    return $out;
}

/**
 * Create an empty filter with `size` bits and `hashes` hash functions (k).
 * @param size {int} the number of bits (must be >= 1)
 * @param hashes {int} the number of hash positions per item (must be >= 1)
 * @return {Filter} the empty filter
 * @throws {Error} kind "bloom" if size or hashes is < 1
 */
export func new(size as int, hashes as int) {
    if ($size < 1) {
        fail("size must be >= 1");
    }
    if ($hashes < 1) {
        fail("hashes must be >= 1");
    }
    def bits as bytes;
    def nbytes as int init ($size + 7) // 8;
    def i as int init 0;
    while ($i < $nbytes) {
        $bits[] = 0;
        $i = $i + 1;
    }
    return Filter{ bits: $bits, size: $size, hashes: $hashes };
}

/**
 * Add an item to the filter. Returns a fresh filter.
 * @param f {Filter} the filter
 * @param item {string} the item to add
 * @return {Filter} a filter with the item recorded
 */
export func add(f as Filter, item as string) {
    def out as Filter init $f;
    def ps as list of int init positions($item, $f.size, $f.hashes);
    for (def pos in $ps) {
        def bi as int init $pos // 8;
        def bit as int init $pos % 8;
        $out.bits[$bi] = $out.bits[$bi] | (1 << $bit);
    }
    return $out;
}

/**
 * Add every item of a list. Returns a fresh filter.
 * @param f {Filter} the filter
 * @param items {list of string} the items to add
 * @return {Filter} a filter with all items recorded
 */
export func addAll(f as Filter, items as list of string) {
    # One copy of the filter, then set bits directly per item: calling `add`
    # per item would deep-copy the whole bit array on every item (O(items x
    # size)).
    def out as Filter init $f;
    for (def item in $items) {
        def ps as list of int init positions($item, $out.size, $out.hashes);
        for (def pos in $ps) {
            def bi as int init $pos // 8;
            def bit as int init $pos % 8;
            $out.bits[$bi] = $out.bits[$bi] | (1 << $bit);
        }
    }
    return $out;
}

/**
 * Test whether an item might be in the filter. False positives are possible;
 * false negatives never happen (a previously added item always returns true).
 * @param f {Filter} the filter
 * @param item {string} the item to test
 * @return {bool} true if the item might be present, false if it is definitely absent
 */
export func mightContain(f as Filter, item as string) {
    def ps as list of int init positions($item, $f.size, $f.hashes);
    for (def pos in $ps) {
        def bi as int init $pos // 8;
        def bit as int init $pos % 8;
        if ((($f.bits[$bi] >> $bit) & 1) == 0) {
            return false;
        }
    }
    return true;
}
