# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# dot_test.j - white-box tests for dot.j. Run with:
#
#     jennifer test modules/dot_test.j
#
# The overlay splices dot.j in front of this file, so the tests reach its
# private helpers (esc, attrList) by bare identifier as well as its exports.
use testing;
use strings;

func testDigraphRendersDirected() {
    def g as Graph init digraph("deps");
    $g = edge($g, "parser", "lexer");
    def out as string init render($g);
    testing.assertTrue(strings.contains($out, 'digraph "deps" {'));
    testing.assertTrue(strings.contains($out, '"parser" -> "lexer";'));
    testing.assertTrue(strings.endsWith($out, '}' + "\n"));
}

func testGraphRendersUndirected() {
    def g as Graph init graph("net");
    $g = edge($g, "a", "b");
    def out as string init render($g);
    testing.assertTrue(strings.startsWith($out, 'graph "net" {'));
    testing.assertTrue(strings.contains($out, '"a" -- "b";'));
    testing.assertFalse(strings.contains($out, "->"));
}

func testNodeAndEdgeAttrs() {
    def g as Graph init digraph("g");
    $g = nodeWith($g, "p", {"label": "Parser", "shape": "box"});
    $g = edgeWith($g, "p", "l", {"label": "calls"});
    def out as string init render($g);
    testing.assertTrue(strings.contains($out, '"p" [label="Parser", shape="box"];'));
    testing.assertTrue(strings.contains($out, '"p" -> "l" [label="calls"];'));
}

func testDefaultBlocks() {
    def g as Graph init digraph("g");
    $g = nodeAttr($g, "shape", "box");
    $g = edgeAttr($g, "color", "gray");
    $g = graphAttr($g, "rankdir", "LR");
    def out as string init render($g);
    testing.assertTrue(strings.contains($out, 'rankdir="LR";'));
    testing.assertTrue(strings.contains($out, 'node [shape="box"];'));
    testing.assertTrue(strings.contains($out, 'edge [color="gray"];'));
}

func testEscaping() {
    # A label carrying a quote, a backslash, and a newline must be escaped so the
    # DOT stays well-formed.
    testing.assertEqual(esc('say "hi"'), 'say \"hi\"');
    testing.assertEqual(esc("a\\b"), "a\\\\b");
    testing.assertEqual(esc("l1\nl2"), "l1\\nl2");
    def g as Graph init digraph("g");
    $g = nodeWith($g, "n", {"label": 'a "b"'});
    testing.assertTrue(strings.contains(render($g), 'label="a \"b\""'));
}

func testAttrListOrderPreserved() {
    def m as map of string to string init {"first": "1", "second": "2", "third": "3"};
    testing.assertEqual(attrList($m), 'first="1", second="2", third="3"');
}

func testValueSemanticsNoMutation() {
    # Building on a graph must not change the original (value semantics).
    def base as Graph init digraph("g");
    def more as Graph init edge($base, "a", "b");
    testing.assertEqual(len($base.edges), 0);
    testing.assertEqual(len($more.edges), 1);
}

func testImplicitNodesFromEdgesOnly() {
    # Edges alone are enough; no explicit node block is required.
    def g as Graph init digraph("g");
    $g = edge($g, "x", "y");
    def out as string init render($g);
    testing.assertEqual(len($g.nodes), 0);
    testing.assertTrue(strings.contains($out, '"x" -> "y";'));
}
