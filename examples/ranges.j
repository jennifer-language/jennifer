# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# Range syntax: `lo..hi` is half-open [lo, hi). Three uses:
#   - list construction:  def r as list of int init 0..5;
#   - for-each iteration:  for (def i in 0..5) { ... }
#   - slicing:             $xs[a..b], $xs[a..], $xs[..b], $xs[..]
# Every form materialises a value-semantic copy - a slice never aliases.

use io;
use convert;

# A range literal builds a `list of int`.
def r as list of int init 1..6;
io.printf("range 1..6:");
for (def x in $r) {
    io.printf(" %d", $x);
}
io.printf("\n");

# A bare range in a for-each iterates lazily.
def sum as int init 0;
for (def i in 0..10) {
    $sum = $sum + $i;
}
io.printf("sum 0..10: %d\n", $sum);

# Slicing a list yields a fresh copy.
def xs as list of int init [10, 20, 30, 40, 50];
def mid as list of int init $xs[1..4];
io.printf("xs[1..4]:");
for (def v in $mid) {
    io.printf(" %d", $v);
}
io.printf("\n");

# Open-ended slices default to 0 / len.
io.printf("open ends: %d %d %d\n", len($xs[2..]), len($xs[..3]), len($xs[..]));

# A slice is a copy - mutating it leaves the source untouched.
$mid[0] = 99;
io.printf("after mid[0]=99: xs[1]=%d mid[0]=%d\n", $xs[1], $mid[0]);

# Strings slice by rune; bytes slice by byte.
def s as string init "hello world";
io.printf("s[0..5]=%s s[6..]=%s\n", $s[0..5], $s[6..]);

def b as bytes init convert.bytesFromString("abcdef", "utf-8");
def sub as bytes init $b[2..5];
io.printf("bytes b[2..5] len=%d first=%d\n", len($sub), $sub[0]);
