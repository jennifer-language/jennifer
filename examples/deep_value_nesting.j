# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * A value nested past the nesting cap (1000 levels, 64 on jennifer-tiny) is a
 * catchable "limit" error where it is built, so every walk over a value stays
 * bounded. jennifer-tiny used to crash on this build and copy (M28.37), the
 * default binary at about 100,000 levels.
 * @module deep_value_nesting
 */

use io;
use json;

/**
 * A tree node.
 *
 * @field kids {list of Node} the children
 */
def struct Node {
    kids as list of Node
};

def n as Node init Node{kids: []};
def wraps as int init 0;
try {
    for (def i in 0..1000) {
        $n = Node{kids: [$n]};
        $wraps = $wraps + 1;
    }
    io.printf("built all 1000 levels\n");
} catch (e) {
    io.printf("build stopped: %s\n", $e.kind);
}
try {
    def m as map of string to Node init {"a": $n};
    io.printf("copied into a map\n");
} catch (e) {
    io.printf("copy stopped: %s\n", $e.kind);
}
def c as Node init $n;
io.printf("equal: %t\n", $c == $n);
$c.kids = [];
io.printf("copy independent: %t\n", $c != $n and len($n.kids) == 1);
def s as string init json.encode($n);
io.printf("json: %t\n", len($s) == 11 * ($wraps + 1));
def small as Node init Node{kids: [Node{kids: []}]};
io.printf("after: %d\n", len($small.kids));
