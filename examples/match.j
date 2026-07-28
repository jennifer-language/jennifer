#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * `match` - multi-way value dispatch. The subject is evaluated once and
 * compared to each `when` arm's values by strict `==`; the first matching arm
 * runs. `else` is the optional default. No fall-through: an arm is an
 * independent block, and `break` / `continue` act on the enclosing loop.
 * @module match
 */
use io;

def const MAX as int init 9;

# One or more values per arm; `when MAX` shows a non-literal value.
func classify(n as int) {
    match ($n) {
        when 0 { return "zero"; }
        when 1, 2, 3 { return "small"; }
        when MAX { return "the max"; }
        else { return "other"; }
    }
    return "unreached";
}

for (def i in [0, 2, 7, 9]) {
    io.printf("classify(%d) = %s\n", $i, classify($i));
}

# A match need not be exhaustive and need not have an else: no matching arm is a
# well-defined no-op. And break / continue inside an arm act on the loop, not the
# match (no C-style fall-through, no "break breaks the switch" trap).
def kept as int init 0;
for (def n in [1, 2, 3, 4, 5]) {
    match ($n) {
        when 2 {
            # skip 2: continue acts on the for loop, not the match
            continue;
        }
        when 4 {
            # stop at 4: break acts on the for loop, not the match
            break;
        }
    }
    $kept = $kept + $n;
}
io.printf("kept sum = %d\n", $kept);

# The subject can be any type; here a string dispatch.
def cmd as string init "stop";
match ($cmd) {
    when "start" { io.printf("starting\n"); }
    when "stop", "halt" { io.printf("stopping\n"); }
    else { io.printf("unknown command\n"); }
}
