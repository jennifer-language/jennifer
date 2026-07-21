# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The canonical first Jennifer program.
 * Prints 42.
 * @module hello
 */

use io;

def x as int init 21;
io.printf($x + $x);
