# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Strings, escape sequences, multiple printf calls.
 * @module greeting
 */

use io;

def name as string init "Jennifer";
io.printf("hello, ");
io.printf($name);
io.printf("!\n");
