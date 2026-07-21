# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Demonstrates the `term` library: raw mode plus single-key reads, the host
 * primitives an interactive TUI is built on. Run it in a real terminal
 * (`jennifer run examples/term.j`) and press keys - it echoes each key's byte
 * value until you press `q`. Off a terminal (a pipe, CI) it says so and stops,
 * so it needs no golden file and is not auto-run.
 *
 * Raw mode disables the terminal's line buffering and its newline cooking, so
 * prints inside the loop use an explicit `\r\n`.
 * @module term
 */

use io;
use os;
use term;

if (not os.isTerminal("stdin")) {
    io.printf("term needs an interactive terminal; run this in a real TTY.\n");
} else {
    def dim as term.Size init term.size("stdout");
    io.printf("terminal is %d rows x %d cols. Press keys (q to quit).\r\n",
        $dim.rows, $dim.cols);

    # Enter raw mode; the returned handle is passed back to term.restore so the
    # terminal is always put back the way it was.
    def state as term.State init term.makeRaw("stdin");
    def running as bool init true;
    while ($running) {
        def b as int init term.readByte();
        if ($b == -1 or $b == 113) {   # -1 = end of input, 113 = 'q'
            $running = false;
        } else {
            io.printf("byte %d\r\n", $b);
        }
    }
    term.restore($state);
    io.printf("bye.\n");
}
