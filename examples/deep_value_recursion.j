# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * Recursion over a deeply nested value, copying a subtree per call.
 * The default binary nests 300 nodes and finishes the walk; jennifer-tiny
 * stops the build at its value-nesting cap, or the walk at its stack guard,
 * with a catchable "limit" error. Either way the program goes on intact. On
 * jennifer-tiny the per-level copy of a deep value used to overrun the fixed
 * stack below the call-depth guard and corrupt memory.
 * @module deep_value_recursion
 */

use io;

/**
 * A tree node.
 *
 * @field kids {list of Node} the children
 */
def struct Node {
    kids as list of Node
};

/**
 * Collect every node below `n`, depth first.
 *
 * @param n {Node} the subtree
 * @param acc {list of Node} the nodes so far
 * @return {list of Node} the nodes so far and those below `n`
 */
func collect(n as Node, acc as list of Node) {
    for (def c in $n.kids) {
        $acc[] = $c;
        $acc = collect($c, $acc);
    }
    return $acc;
}

def deep as Node init Node{kids: []};
try {
    for (def i in 0..300) {
        $deep = Node{kids: [$deep]};
    }
} catch (e) {
    # jennifer-tiny caps value nesting at 64 levels (M28.37)
    if ($e.kind != "limit") {
        io.printf("wrong error: %s: %s\n", $e.kind, $e.message);
    }
}
for (def round in 0..5) {
    try {
        collect($deep, []);
    } catch (e) {
        if ($e.kind != "limit") {
            io.printf("wrong error: %s: %s\n", $e.kind, $e.message);
        }
    }
    def small as Node init Node{kids: [Node{kids: [Node{kids: []}]}]};
    io.printf("round %d: %d nodes\n", $round, len(collect($small, [])));
}
