# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * The module the v4 `module handle call` row of examples/benchmark.j calls
 * into: a "handle" struct (a tree of nodes keyed by id), a function that
 * builds one, and one that takes it and does constant work. A call of `has`
 * costs whatever crossing the module boundary with the tree costs, so the row
 * measures that crossing alone.
 * Not a runnable example on its own.
 * @module tree
 */
use maps;

/**
 * One node of the tree.
 * @field id {string} the node's key in the tree
 * @field parent {string} the id of the node before it
 */
export def struct Node {
    id as string,
    parent as string
};

/**
 * The handle: every node, keyed by id.
 * @field root {string} the id of the first node
 * @field nodes {map} every node, keyed by its id
 */
export def struct Tree {
    root as string,
    nodes as map of string to Node
};

/**
 * Build a tree of `n` nodes, each the child of the one before.
 * @param n {int} the node count
 * @return {Tree} the tree
 */
export func chain(n as int) {
    def t as Tree init Tree{root: "n0", nodes: {}};
    def i as int init 0;
    while ($i < $n) {
        $t.nodes["n{$i}"] = Node{id: "n{$i}", parent: "n{$i - 1}"};
        $i = $i + 1;
    }
    return $t;
}

/**
 * Whether the tree holds a node: one map lookup, whatever the tree's size.
 * @param t {Tree} the tree
 * @param id {string} the node id
 * @return {bool} true when the node exists
 */
export func has(t as Tree, id as string) {
    return maps.has($t.nodes, $id);
}
