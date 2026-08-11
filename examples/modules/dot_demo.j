# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The dot module (modules/dot.j): build a Graphviz graph of nodes and edges and
 * render it to .dot text. Pipe the output to Graphviz to draw it:
 *   jennifer run examples/modules/dot_demo.j | dot -Tsvg > pipeline.svg
 * @module dot_demo
 */
use io;
import "../../modules/dot.j" as dot;

# The interpreter's own stage pipeline, left to right, boxed nodes.
def g as dot.Graph init dot.digraph("pipeline");
$g = dot.graphAttr($g, "rankdir", "LR");
$g = dot.nodeAttr($g, "shape", "box");
$g = dot.nodeWith($g, "lexer", {"label": "Lexer"});
$g = dot.nodeWith($g, "preproc", {"label": "Preprocessor"});
$g = dot.nodeWith($g, "parser", {"label": "Parser"});
$g = dot.nodeWith($g, "interp", {"label": "Interpreter"});
$g = dot.edge($g, "lexer", "preproc");
$g = dot.edgeWith($g, "preproc", "parser", {"label": "tokens"});
$g = dot.edgeWith($g, "parser", "interp", {"label": "AST"});

io.printf("%s", dot.render($g));
