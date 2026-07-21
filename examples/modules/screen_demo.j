# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

# screen_demo.j - the output-only layer of the `screen` module: build a cell
# buffer, draw a bordered panel with a title and a progress bar, and animate it
# with the flicker-free `diff` update loop (only the changed cells are
# repainted each frame). Run it in a real terminal to see the drawing:
#
#     jennifer run examples/modules/screen_demo.j
#
# The interactive layer (raw-mode key events via `screen.nextKey`) needs a live
# TTY and is not shown here; see docs/modules/screen.md.

use io;
use time;
use convert;
import "../../modules/screen.j" as screen;

def const ROWS as int init 7;
def const COLS as int init 40;
def const STEPS as int init 20;

# frame builds the dashboard buffer for a given progress step (0..STEPS).
func frame(step as int) {
    def buf as screen.Buffer init screen.newScreen(ROWS, COLS);
    $buf = screen.box($buf, 0, 0, COLS, ROWS);
    $buf = screen.textColor($buf, 2, 1, "screen module - live dashboard", "cyan");
    $buf = screen.text($buf, 2, 3, "Progress");

    # The bar: a filled run proportional to step, inside brackets.
    def barWidth as int init COLS - 8;
    def filled as int init ($step * $barWidth) // STEPS;
    $buf = screen.set($buf, 2, 4, "[");
    $buf = screen.set($buf, COLS - 3, 4, "]");
    $buf = screen.fill($buf, 3, 4, $filled, 1, "#");
    def pct as int init ($step * 100) // STEPS;
    $buf = screen.text($buf, 2, 5, "  " + convert.toString($pct) + "% complete");
    return $buf;
}

# Enter the alternate screen so the animation does not scroll the shell.
io.printf("%s%s%s", screen.enterAlt(), screen.hideCursor(), screen.clear());

def prev as screen.Buffer init frame(0);
io.printf("%s", screen.render($prev));

for (def i as int init 1; $i <= STEPS; $i = $i + 1) {
    def next as screen.Buffer init frame($i);
    # Only the cells that changed since the last frame are repainted.
    io.printf("%s", screen.diff($prev, $next));
    $prev = $next;
    time.sleep(time.fromMilliseconds(80));
}

# Restore the terminal and print a final line in the normal buffer.
io.printf("%s%s", screen.showCursor(), screen.exitAlt());
io.printf("done - rendered %d frames with per-cell diffing\n", STEPS);
