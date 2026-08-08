# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Slurp all of stdin at once with io.readLines(), then print the lines in
 * reverse order (like `tac`) - the slurp counterpart to echo.j's streaming
 * loop. Reading everything first is the natural fit when you need the whole
 * input before you can start (reverse, sort, count). Splitting is
 * OS-independent (LF or CRLF). Run with stdin piped in, e.g.
 * `printf 'one\ntwo\nthree\n' | jennifer run examples/readlines.j`. Not part of
 * the golden-file suite (the harness can't feed stdin).
 * @module readlines
 */

use io;
use lists;

def lines as list of string init io.readLines();
io.printf("%d line(s), reversed:\n", len($lines));
for (def line in lists.reverse($lines)) {
    io.printf("%s\n", $line);
}
