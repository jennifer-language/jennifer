# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * Graphviz DOT graph description: build a graph of nodes and edges with
 * attributes and render it to `.dot` text for an external Graphviz tool to lay
 * out and draw (`dot -Tsvg graph.dot > graph.svg`). Pure Jennifer - no Go, no
 * system library, both binaries - it emits the text description only; graph
 * layout is Graphviz's job and is deliberately not reimplemented. Value
 * semantics throughout: every builder returns a fresh Graph, so the graph you
 * pass in is never mutated.
 * @module dot
 * @example
 * import "dot.j" as dot;
 * def g as dot.Graph init dot.digraph("deps");
 * $g = dot.edge($g, "parser", "lexer");
 * $g = dot.edge($g, "parser", "preproc");
 * def text as string init dot.render($g);
 */
use strings;
use lists;

# --- data model ----------------------------------------------------

/**
 * A single graph node: an id (its key in the graph) plus an ordered attribute
 * map (label, shape, color, ...).
 * @field id {string} the node identifier
 * @field attrs {map of string to string} DOT attributes, in insertion order
 */
export def struct Node { id as string, attrs as map of string to string };

/**
 * A directed or undirected edge between two node ids, with its own attributes.
 * @field from {string} the source node id
 * @field to {string} the destination node id
 * @field attrs {map of string to string} DOT attributes, in insertion order
 */
export def struct Edge { from as string, to as string, attrs as map of string to string };

/**
 * A whole graph: its kind (directed = digraph), name, ordered nodes and edges,
 * and default attribute blocks for the graph, its nodes, and its edges.
 * @field directed {bool} true renders a `digraph` with `->` edges, false a `graph` with `--`
 * @field name {string} the graph name (quoted in the output)
 * @field nodes {list of Node} explicit nodes, in insertion order
 * @field edges {list of Edge} edges, in insertion order
 * @field graphAttrs {map of string to string} `key="value";` graph-level attributes
 * @field nodeAttrs {map of string to string} the `node [ ... ]` default block
 * @field edgeAttrs {map of string to string} the `edge [ ... ]` default block
 */
export def struct Graph {
    directed as bool,
    name as string,
    nodes as list of Node,
    edges as list of Edge,
    graphAttrs as map of string to string,
    nodeAttrs as map of string to string,
    edgeAttrs as map of string to string
};

# --- constructors --------------------------------------------------

/**
 * A new, empty directed graph. Renders as `digraph` with `a -> b` edges.
 * @param name {string} the graph name
 * @return {Graph} an empty directed graph
 */
export func digraph(name as string) {
    return Graph{directed: true, name: $name, nodes: [], edges: [], graphAttrs: {}, nodeAttrs: {}, edgeAttrs: {}};
}

/**
 * A new, empty undirected graph. Renders as `graph` with `a -- b` edges.
 * @param name {string} the graph name
 * @return {Graph} an empty undirected graph
 */
export func graph(name as string) {
    return Graph{directed: false, name: $name, nodes: [], edges: [], graphAttrs: {}, nodeAttrs: {}, edgeAttrs: {}};
}

# --- builders (each returns a fresh Graph) -------------------------

/**
 * Add a bare node (no attributes). Adding a node is optional - an id first seen
 * on an edge is created implicitly by Graphviz - but an explicit node carries a
 * label, shape, or colour.
 * @param g {Graph} the graph
 * @param id {string} the node id
 * @return {Graph} a copy with the node appended
 */
export func node(g as Graph, id as string) {
    # The fresh-literal form (also in nodeWith / edge / edgeWith) costs one
    # fewer list copy per call than "def out init $g; $out.nodes = lists.push(...)":
    # the pushed list flows straight into the new field instead of being
    # re-copied by a field write. Inlined here so the bare form skips the
    # nested nodeWith call (another full graph copy at that boundary).
    return Graph{
        directed: $g.directed,
        name: $g.name,
        nodes: lists.push($g.nodes, Node{id: $id, attrs: {}}),
        edges: $g.edges,
        graphAttrs: $g.graphAttrs,
        nodeAttrs: $g.nodeAttrs,
        edgeAttrs: $g.edgeAttrs
    };
}

/**
 * Add a node with attributes (`{"label": "Parser", "shape": "box"}`).
 * @param g {Graph} the graph
 * @param id {string} the node id
 * @param attrs {map of string to string} the node's DOT attributes
 * @return {Graph} a copy with the node appended
 */
export func nodeWith(g as Graph, id as string, attrs as map of string to string) {
    return Graph{
        directed: $g.directed,
        name: $g.name,
        nodes: lists.push($g.nodes, Node{id: $id, attrs: $attrs}),
        edges: $g.edges,
        graphAttrs: $g.graphAttrs,
        nodeAttrs: $g.nodeAttrs,
        edgeAttrs: $g.edgeAttrs
    };
}

/**
 * Add a bare edge between two node ids.
 * @param g {Graph} the graph
 * @param src {string} the source node id
 * @param dst {string} the destination node id
 * @return {Graph} a copy with the edge appended
 */
export func edge(g as Graph, src as string, dst as string) {
    return Graph{
        directed: $g.directed,
        name: $g.name,
        nodes: $g.nodes,
        edges: lists.push($g.edges, Edge{from: $src, to: $dst, attrs: {}}),
        graphAttrs: $g.graphAttrs,
        nodeAttrs: $g.nodeAttrs,
        edgeAttrs: $g.edgeAttrs
    };
}

/**
 * Add an edge with attributes (`{"label": "calls", "style": "dashed"}`).
 * @param g {Graph} the graph
 * @param src {string} the source node id
 * @param dst {string} the destination node id
 * @param attrs {map of string to string} the edge's DOT attributes
 * @return {Graph} a copy with the edge appended
 */
export func edgeWith(g as Graph, src as string, dst as string, attrs as map of string to string) {
    return Graph{
        directed: $g.directed,
        name: $g.name,
        nodes: $g.nodes,
        edges: lists.push($g.edges, Edge{from: $src, to: $dst, attrs: $attrs}),
        graphAttrs: $g.graphAttrs,
        nodeAttrs: $g.nodeAttrs,
        edgeAttrs: $g.edgeAttrs
    };
}

/**
 * Set a graph-level attribute (rendered as `key="value";`), e.g. `rankdir` = LR
 * or a graph `label`.
 * @param g {Graph} the graph
 * @param key {string} the attribute name
 * @param value {string} the attribute value
 * @return {Graph} a copy with the attribute set
 */
export func graphAttr(g as Graph, key as string, value as string) {
    def out as Graph init $g;
    $out.graphAttrs[$key] = $value;
    return $out;
}

/**
 * Set a default node attribute, rendered once as `node [ ... ]` before the
 * nodes - so every node inherits, e.g. `shape` = box.
 * @param g {Graph} the graph
 * @param key {string} the attribute name
 * @param value {string} the attribute value
 * @return {Graph} a copy with the default set
 */
export func nodeAttr(g as Graph, key as string, value as string) {
    def out as Graph init $g;
    $out.nodeAttrs[$key] = $value;
    return $out;
}

/**
 * Set a default edge attribute, rendered once as `edge [ ... ]` before the
 * edges - so every edge inherits, e.g. `color` = gray.
 * @param g {Graph} the graph
 * @param key {string} the attribute name
 * @param value {string} the attribute value
 * @return {Graph} a copy with the default set
 */
export func edgeAttr(g as Graph, key as string, value as string) {
    def out as Graph init $g;
    $out.edgeAttrs[$key] = $value;
    return $out;
}

# --- rendering -----------------------------------------------------

# esc escapes a DOT string value: backslash and double-quote are backslash-
# escaped, and a literal newline becomes the two-character DOT escape \n.
func esc(s as string) {
    def out as string init strings.replace($s, "\\", "\\\\");
    $out = strings.replace($out, '"', '\"');
    $out = strings.replace($out, "\n", "\\n");
    return $out;
}

# attrList renders an attribute map as `k1="v1", k2="v2"` in insertion order.
func attrList(attrs as map of string to string) {
    def parts as list of string init [];
    for (def k in $attrs) {
        $parts[] = $k + '="' + esc($attrs[$k]) + '"';
    }
    return strings.join($parts, ", ");
}

/**
 * Render the graph to DOT text (a trailing newline included), ready to write to
 * a `.dot` file or pipe to `dot`.
 * @param g {Graph} the graph to render
 * @return {string} the DOT source
 */
export func render(g as Graph) {
    def kw as string init "graph";
    def op as string init " -- ";
    if ($g.directed) {
        $kw = "digraph";
        $op = " -> ";
    }
    def out as list of string init [];
    $out[] = $kw + ' "' + esc($g.name) + '" {';
    for (def k in $g.graphAttrs) {
        $out[] = '  ' + $k + '="' + esc($g.graphAttrs[$k]) + '";';
    }
    if (len($g.nodeAttrs) > 0) {
        $out[] = '  node [' + attrList($g.nodeAttrs) + '];';
    }
    if (len($g.edgeAttrs) > 0) {
        $out[] = '  edge [' + attrList($g.edgeAttrs) + '];';
    }
    for (def i in 0..len($g.nodes)) {
        def n as Node init $g.nodes[$i];
        def line as string init '  "' + esc($n.id) + '"';
        if (len($n.attrs) > 0) {
            $line = $line + ' [' + attrList($n.attrs) + ']';
        }
        $out[] = $line + ';';
    }
    for (def i in 0..len($g.edges)) {
        def e as Edge init $g.edges[$i];
        def line as string init '  "' + esc($e.from) + '"' + $op + '"' + esc($e.to) + '"';
        if (len($e.attrs) > 0) {
            $line = $line + ' [' + attrList($e.attrs) + ']';
        }
        $out[] = $line + ';';
    }
    $out[] = '}';
    return strings.join($out, "\n") + "\n";
}
