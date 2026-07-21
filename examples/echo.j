# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Read lines from stdin, print each surrounded by `[ ]`.
 * Demonstrates the canonical `while (not io.eof()) { io.readLine() }` loop.
 * Run with stdin piped in, e.g. `echo -e 'one\ntwo\nthree' | jennifer run
 * examples/echo.j`. Not part of the golden-file test suite (the harness can't
 * feed stdin).
 * @module echo
 */

use io;

def ln as int init 0;
while (not io.eof()) {
    def line as string init io.readLine();
    $ln = $ln + 1;
    io.printf("[%d|pad=3][%s]\n", $ln, $line);
}
