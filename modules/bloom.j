# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0

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
use math;

# The math library ships no natural log, so the optimal-sizing math below uses
# this local ln. LN2 is the natural log of 2, used both as the reduction step
# in ln and directly in the optimal bit/hash formulas.
def const LN2 as float init 0.6931471805599453;

# MAX_SIZE caps a filter's bit count so a caller (or a derived sizing) cannot
# preallocate an unbounded bitvector; the packed array stays within the 64 MiB
# house limit (512 Mibit / 8), which also keeps the position math clear of int64
# overflow.
def const MAX_SIZE as int init 536870912;

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
    throw Error{kind: "bloom", message: "bloom: " + $msg, file: "", line: 0, col: 0};
}

# ln returns the natural logarithm of x (x > 0). The math library has no log,
# so this uses range reduction by 2 (with LN2) to pull the argument into
# [0.75, 1.5), then the fast-converging atanh series
# ln(y) = 2 * (t + t^3/3 + t^5/5 + ...) with t = (y - 1) / (y + 1); |t| stays
# below ~0.2 after reduction, so a few dozen terms give full float precision.
func ln(x as float) {
    if ($x <= 0.0) {
        fail("ln requires x > 0");
    }
    def n as int init 0;
    def y as float init $x;
    while ($y >= 1.5) {
        $y = $y / 2.0;
        $n = $n + 1;
    }
    while ($y < 0.75) {
        $y = $y * 2.0;
        $n = $n - 1;
    }
    def t as float init ($y - 1.0) / ($y + 1.0);
    def t2 as float init $t * $t;
    def term as float init $t;
    def sum as float init 0.0;
    def k as int init 1;
    while ($k < 40) {
        $sum = $sum + $term / $k;
        $term = $term * $t2;
        $k = $k + 2;
    }
    return 2.0 * $sum + $n * LN2;
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
    if ($size > MAX_SIZE) {
        fail("size " + convert.toString($size) + " exceeds the maximum of " + convert.toString(MAX_SIZE));
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
    return Filter{bits: $bits, size: $size, hashes: $hashes};
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

/**
 * Create a filter sized for `n` expected elements at a target false-positive
 * rate `fpr`. Picks the optimal bit count m = ceil(-(n*ln(fpr))/(ln(2)^2)) and
 * hash count k = round((m/n)*ln(2)) (clamped to k >= 1), then returns an empty
 * Filter of that shape.
 * @param n {int} the expected number of elements (must be >= 1)
 * @param fpr {float} the target false-positive rate, in the open interval (0, 1)
 * @return {Filter} an empty filter sized for the requested load
 * @throws {Error} kind "bloom" if n < 1 or fpr is not in (0, 1)
 */
export func optimal(n as int, fpr as float) {
    if ($n < 1) {
        fail("n must be >= 1");
    }
    if ($fpr <= 0.0 or $fpr >= 1.0) {
        fail("fpr must be in (0, 1)");
    }
    def lnFpr as float init ln($fpr);
    def m as int init math.ceil((-($n * $lnFpr)) / (LN2 * LN2));
    if ($m < 1) {
        $m = 1;
    }
    def k as int init math.round(($m / $n) * LN2);
    if ($k < 1) {
        $k = 1;
    }
    return new($m, $k);
}

/**
 * Serialize a filter to bytes: a 4-byte big-endian `size`, a 4-byte big-endian
 * `hashes`, then the raw bit array. `deserialize` reverses it exactly.
 * @param f {Filter} the filter to encode
 * @return {bytes} the encoded filter
 */
export func serialize(f as Filter) {
    # The header stores size / hashes as 4 bytes each; a value past 2^32-1 would
    # silently truncate. Such a filter is > 512 MB (impractical), but reject it
    # rather than emit a corrupt header.
    if ($f.size > 4294967295 or $f.hashes > 4294967295) {
        fail("filter is too large to serialize (size / hashes exceed 32 bits)");
    }
    def out as bytes;
    $out[] = ($f.size >> 24) & 0xff;
    $out[] = ($f.size >> 16) & 0xff;
    $out[] = ($f.size >> 8) & 0xff;
    $out[] = $f.size & 0xff;
    $out[] = ($f.hashes >> 24) & 0xff;
    $out[] = ($f.hashes >> 16) & 0xff;
    $out[] = ($f.hashes >> 8) & 0xff;
    $out[] = $f.hashes & 0xff;
    def i as int init 0;
    while ($i < len($f.bits)) {
        $out[] = $f.bits[$i];
        $i = $i + 1;
    }
    return $out;
}

/**
 * Reconstruct a filter from bytes produced by `serialize`. The result is
 * identical (same size, k, and bits), so membership is preserved.
 * @param b {bytes} the encoded filter
 * @return {Filter} the reconstructed filter
 * @throws {Error} kind "bloom" if the byte layout is malformed
 */
export func deserialize(b as bytes) {
    if (len($b) < 8) {
        fail("serialized filter is too short");
    }
    def size as int init readLong($b, 0);
    def hashes as int init readLong($b, 4);
    # Re-apply new()'s invariants: a size=0 blob would make `mightContain` divide
    # by zero at query time, and a hashes=0 blob would make it match *everything*
    # (an empty positions loop) - a corrupt / crafted filter must be rejected here,
    # not turn into a universal "yes" oracle.
    if ($size < 1) {
        fail("serialized filter has an invalid size (must be >= 1)");
    }
    if ($hashes < 1) {
        fail("serialized filter has an invalid hash count (must be >= 1)");
    }
    def expected as int init ($size + 7) // 8;
    if (len($b) != 8 + $expected) {
        fail("serialized filter length does not match its declared size");
    }
    def bits as bytes;
    def i as int init 0;
    while ($i < $expected) {
        $bits[] = $b[8 + $i];
        $i = $i + 1;
    }
    return Filter{bits: $bits, size: $size, hashes: $hashes};
}

/**
 * Union two filters of the same size and k with a bitwise OR. An item present
 * in either input is present in the result. Returns a fresh filter.
 * @param a {Filter} the first filter
 * @param b {Filter} the second filter
 * @return {Filter} a filter holding the members of both
 * @throws {Error} kind "bloom" if the two filters differ in size or k
 */
export func union(a as Filter, b as Filter) {
    if ($a.size != $b.size) {
        fail("union: filters differ in size");
    }
    if ($a.hashes != $b.hashes) {
        fail("union: filters differ in hash count");
    }
    def out as Filter init $a;
    def i as int init 0;
    while ($i < len($out.bits)) {
        $out.bits[$i] = $out.bits[$i] | $b.bits[$i];
        $i = $i + 1;
    }
    return $out;
}

/**
 * Alias for `union`.
 * @param a {Filter} the first filter
 * @param b {Filter} the second filter
 * @return {Filter} a filter holding the members of both
 * @throws {Error} kind "bloom" if the two filters differ in size or k
 */
export func merge(a as Filter, b as Filter) {
    return union($a, $b);
}
