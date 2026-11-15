# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * Runaway recursion with a heavy body is a catchable "limit" error.
 * On jennifer-tiny such a body used to pass the fixed 4 MB stack below the
 * call-depth cap and corrupt memory (M28.72); the program goes on intact.
 * @module deep_recursion
 */

use io;

/**
 * A document with a name and some items.
 *
 * @field name {string} the title
 * @field items {list of int} the items
 */
def struct Doc {
    name as string,
    items as list of int
};

/**
 * Descend forever through nested loops, copying a struct per level.
 *
 * @param d {Doc} the document so far
 * @param depth {int} how deep this call is
 * @return {Doc} never returns normally
 */
func walk(d as Doc, depth as int) {
    def items as list of int init $d.items;
    for (def k in [1]) {
        for (def j in [1]) {
            if ($depth >= 0) {
                def inner as Doc init walk(Doc{name: "n", items: []}, $depth + 1);
                for (def x in $inner.items) {
                    $items[] = $x;
                }
            }
        }
    }
    def out as Doc init $d;
    $out.items = $items;
    return $out;
}

try {
    walk(Doc{name: "top", items: []}, 0);
} catch (e) {
    io.printf("caught kind %s\n", $e.kind);
}
def after as Doc init Doc{name: "after", items: [1, 2]};
io.printf("%s %d\n", $after.name, len($after.items));
