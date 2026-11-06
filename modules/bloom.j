# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * A Bloom filter: a compact, probabilistic set. `add` records a string;
 * `mightContain` tests membership with **no false negatives** (a member always
 * reports true) but possible **false positives** (a non-member may report true,
 * with a probability that grows as the filter fills). The bit array is packed
 * into `bytes`; the `hashes` positions per item come from double-hashing one
 * SHA-256 digest (`pos_i = (h1 + i*h2) mod size`), so a single hash yields all k
 * positions.
 *
 * Value-semantic: `add` / `addAll` / `union` return the updated filter. Called
 * as `$f = bloom.add($f, x)` on a local they update the bit array in place;
 * otherwise the filter is copied. Over `hash` + `math` + `binary`; runs on both
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
use binary;

# LN2 is ln(2), for the optimal bit / hash formulas.
def const LN2 as float init 0.6931471805599453;

# MAX_SIZE caps a filter's bit count so a caller (or a derived sizing) cannot
# preallocate an unbounded bitvector; the packed array stays within the 64 MiB
# house limit (512 Mibit / 8), which also keeps the position math clear of int64
# overflow.
def const MAX_SIZE as int init 536870912;

# MAX_HASHES caps k: every add / lookup costs k steps, so a crafted serialized
# filter with a huge hash count would make each lookup loop billions of times.
# Optimal k stays below 100 for any practical false-positive rate.
def const MAX_HASHES as int init 255;

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
    throw Error{kind: "bloom", message: "bloom: " + $msg};
}

# readLong reads a 32-bit big-endian value from a digest at an offset.
func readLong(buf as bytes, off as int) {
    return ($buf[$off] << 24) | ($buf[$off + 1] << 16) | ($buf[$off + 2] << 8) | $buf[$off + 3];
}

/**
 * Create an empty filter with `size` bits and `hashes` hash functions (k).
 * @param size {int} the number of bits (must be >= 1)
 * @param hashes {int} the number of hash positions per item (1 to 255)
 * @return {Filter} the empty filter
 * @throws {Error} kind "bloom" if size or hashes is out of range
 */
export func new(size as int, hashes as int) {
    if ($size < 1) {
        fail("size must be >= 1");
    }
    if ($size > MAX_SIZE) {
        fail("size {$size} exceeds the maximum of {MAX_SIZE}");
    }
    if ($hashes < 1 or $hashes > MAX_HASHES) {
        fail("hashes must be in 1..{MAX_HASHES}");
    }
    return Filter{bits: binary.make(($size + 7) // 8), size: $size, hashes: $hashes};
}

# setItem sets an item's k bits. The double-hash step is forced non-zero: with
# g = 0 (mod size) every position would collapse to one bit
# (Kirsch-Mitzenmacher).
func setItem(f as Filter, item as string) {
    def digest as bytes init hash.compute(convert.bytesFromString($item, "utf-8"), "sha256");
    def h as int init readLong($digest, 0);
    def step as int init readLong($digest, 4) % $f.size;
    if ($step == 0) {
        $step = 1;
    }
    for (def i in 0..$f.hashes) {
        def pos as int init ($h + $i * $step) % $f.size;
        def bi as int init $pos // 8;
        $f.bits[$bi] = $f.bits[$bi] | (1 << ($pos % 8));
    }
    return $f;
}

/**
 * Add an item to the filter.
 * @param f {Filter} the filter
 * @param item {string} the item to add
 * @return {Filter} the filter with the item recorded
 */
export func add(f as Filter, item as string) {
    $f = setItem($f, $item);
    return $f;
}

/**
 * Add every item of a list.
 * @param f {Filter} the filter
 * @param items {list of string} the items to add
 * @return {Filter} the filter with all items recorded
 */
export func addAll(f as Filter, items as list of string) {
    for (def item in $items) {
        $f = setItem($f, $item);
    }
    return $f;
}

/**
 * Test whether an item might be in the filter. False positives are possible;
 * false negatives never happen (a previously added item always returns true).
 * @param f {Filter} the filter
 * @param item {string} the item to test
 * @return {bool} true if the item might be present, false if it is definitely absent
 */
export func mightContain(f as Filter, item as string) {
    def digest as bytes init hash.compute(convert.bytesFromString($item, "utf-8"), "sha256");
    def h as int init readLong($digest, 0);
    def step as int init readLong($digest, 4) % $f.size;
    if ($step == 0) {
        $step = 1;
    }
    for (def i in 0..$f.hashes) {
        def pos as int init ($h + $i * $step) % $f.size;
        if ((($f.bits[$pos // 8] >> ($pos % 8)) & 1) == 0) {
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
    def lnFpr as float init math.ln($fpr);
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
    def hdr as bytes;
    $hdr[] = ($f.size >> 24) & 0xff;
    $hdr[] = ($f.size >> 16) & 0xff;
    $hdr[] = ($f.size >> 8) & 0xff;
    $hdr[] = $f.size & 0xff;
    $hdr[] = ($f.hashes >> 24) & 0xff;
    $hdr[] = ($f.hashes >> 16) & 0xff;
    $hdr[] = ($f.hashes >> 8) & 0xff;
    $hdr[] = $f.hashes & 0xff;
    # One Go-side copy appends the bit array to the header - bulk, not a
    # per-byte loop.
    return binary.concat($hdr, $f.bits);
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
    if ($hashes < 1 or $hashes > MAX_HASHES) {
        fail("serialized filter has an invalid hash count (must be 1..{MAX_HASHES})");
    }
    def expected as int init ($size + 7) // 8;
    if (len($b) != 8 + $expected) {
        fail("serialized filter length does not match its declared size");
    }
    # One Go-side slice copies the payload out of the blob - bulk, not a
    # per-byte loop.
    def bits as bytes init binary.slice($b, 8, len($b));
    return Filter{bits: $bits, size: $size, hashes: $hashes};
}

/**
 * Union two filters of the same size and k with a bitwise OR. An item present
 * in either input is present in the result.
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
    for (def i in 0..len($a.bits)) {
        $a.bits[$i] = $a.bits[$i] | $b.bits[$i];
    }
    return $a;
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
