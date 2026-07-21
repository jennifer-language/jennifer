# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Recursion + parameters + multi-arg printf.
 * Demonstrates method parameters, return values, recursion, and format-string
 * printf.
 * @module factorial
 */

use io;

/**
 * Factorial of a non-negative integer, computed recursively.
 * @param n {int} the operand (assumed >= 0)
 * @return {int} n! (n factorial)
 */
func fact(n as int) {
    if ($n == 0) { return 1; }
    return $n * fact($n - 1);
}

for (def i as int init 0; $i <= 8; $i = $i + 1) {
    io.printf("%d! = %d\n", $i, fact($i));
}
