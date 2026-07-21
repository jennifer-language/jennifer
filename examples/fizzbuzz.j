# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Classic FizzBuzz from 1 to 15.
 * Demonstrates for loop, if/elseif/else, comparison, modulo.
 * @module fizzbuzz
 */

use io;

for (def i as int init 1; $i <= 15; $i = $i + 1) {
    if ($i % 15 == 0) {
        io.printf("FizzBuzz\n");
    } elseif ($i % 3 == 0) {
        io.printf("Fizz\n");
    } elseif ($i % 5 == 0) {
        io.printf("Buzz\n");
    } else {
        io.printf($i);
        io.printf("\n");
    }
}
