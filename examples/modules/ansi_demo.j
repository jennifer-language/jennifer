# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Demonstrates the ansi module (modules/ansi.j), the first module built on
 * Jennifer's module system.
 * Run it in a terminal to see colour; piped or redirected it suppresses
 * styling (non-TTY / NO_COLOR), so the text stays clean either way.
 * @module ansi_demo
 */

use io;
import "../../modules/ansi.j" as ansi;

io.printf("%s\n", ansi.bold(ansi.red("error:")) + " something broke");
io.printf("%s\n", ansi.green("ok") + " / " + ansi.yellow("warn"));
io.printf("%s\n", ansi.rgb("truecolor orange", 255, 128, 0));
io.printf("%s\n", ansi.underline(ansi.cyan("nested + underlined")));

# strip is the inverse of the wrappers, whether or not colour is on.
def styled as string init ansi.bold(ansi.blue("styled"));
io.printf("stripped: [%s]\n", ansi.strip($styled));
