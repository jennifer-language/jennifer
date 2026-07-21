# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Demonstrates file imports.
 * `import "greetinglib.j";` splices the contents of greetinglib.j at this point,
 * making $name and $greeting available to the surrounding scope.
 * @module main
 */

use io;
include "greetinglib.j";

io.printf($greeting);
io.printf($name);
io.printf("!\n");
