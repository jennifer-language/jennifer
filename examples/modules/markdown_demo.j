# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The markdown module (modules/markdown.j): render a small Markdown document to HTML and to styled terminal text.
 * @module markdown_demo
 */
use io;
import "../../modules/markdown.j" as markdown;

def doc as string init "# Shopping list\n";
$doc = $doc + "\n";
$doc = $doc + "Buy **fresh** fruit and a *little* `bread`.\n";
$doc = $doc + "\n";
$doc = $doc + "- apples & pears\n";
$doc = $doc + "- figs\n";
$doc = $doc + "\n";
$doc = $doc + "See [the recipe](http://example/recipe?id=1&v=2).\n";
$doc = $doc + "\n";
$doc = $doc + "```\ntotal = 3 items\n```";

io.printf("=== HTML ===\n%s\n\n", markdown.toHtml($doc));
io.printf("=== ANSI (styled on a TTY, plain when piped) ===\n%s\n\n", markdown.toAnsi($doc));

# The authoring helpers build Markdown text (the inverse of rendering).
def feats as list of string init ["fast", "small", "strict"];
def built as string init markdown.header("h2", "Why Jennifer") + "\n\n";
$built = $built + "It is " + markdown.style("bold", "great") + ":\n\n";
$built = $built + markdown.bullets($feats);
io.printf("=== authored Markdown ===\n%s\n\n", $built);

# Tabular data out as a GFM table.
def rows as list of list of string init [];
$rows[] = ["parse", "12", "fast"];
$rows[] = ["render", "8", "faster"];
def cols as list of string init ["step", "ms", "note"];
def aligns as list of string init ["left", "right", "none"];
def tbl as string init markdown.table($cols, $aligns, $rows);
io.printf("=== authored table ===\n%s\n\n", $tbl);

# The reader round-trips a table: author it, then render both ways.
io.printf("=== that table as HTML ===\n%s\n\n", markdown.toHtml($tbl));
io.printf("=== that table as ANSI ===\n%s\n\n", markdown.toAnsi($tbl));

# tablePretty aligns the source columns of a handcrafted table.
def messy as string init "| Name | Score |\n|:-|-:|\n| Ada | 95 |\n| Grace | 8 |";
io.printf("=== tablePretty (source aligned) ===\n%s\n", markdown.tablePretty($messy));
