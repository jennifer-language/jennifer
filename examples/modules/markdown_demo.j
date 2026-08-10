# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The markdown module (modules/markdown.j): render a small Markdown document to HTML,
 * to styled terminal text, and to a laid-out PDF (`toPdf`), plus walk its parse tree.
 * @module markdown_demo
 */
use io;
use fs;
use os;
use path;
import "../../modules/markdown.j" as markdown;

def doc as string init "# Shopping list\n";
$doc = $doc + "\n";
$doc = $doc + "Buy **fresh** fruit and a *little* `bread`.\n";
$doc = $doc + "\n";
$doc = $doc + "- fruit\n";           # a nested list under an item
$doc = $doc + "  - apples & pears\n";
$doc = $doc + "  - figs\n";
$doc = $doc + "- bread\n";
$doc = $doc + "\n";
$doc = $doc + "> Tip: shop the ![market](http://example/market.png) early.\n";  # blockquote + image
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
io.printf("=== tablePretty (source aligned) ===\n%s\n\n", markdown.tablePretty($messy));

# The reader surfaces the parse tree, so a document can be inspected or transformed
# and then rendered - build and parse share one model.
def tree as markdown.Node init markdown.parse($doc);

io.printf("=== outline (walk the headings for a TOC) ===\n");
for (def h in markdown.findAll($tree, "heading")) {
    io.printf("  H%d: %s\n", markdown.level($h), markdown.text($h));
}

io.printf("=== links (pull every link target) ===\n");
for (def a in markdown.findAll($tree, "paragraph/link")) {
    io.printf("  %s -> %s\n", markdown.text($a), markdown.attr($a, "href"));
}

# Render straight from the tree (a parsed or hand-built one).
io.printf("\n=== rendered from the tree (HTML) ===\n%s\n", markdown.render($tree, "html"));

# The same document as a laid-out PDF, through the folded pdf renderer.
def pdfBytes as bytes init markdown.toPdf($doc);
def pdfPath as string init path.join(os.tempDir(), "markdown_demo.pdf");
fs.writeBytes($pdfPath, $pdfBytes);
io.printf("\n=== PDF (markdown.toPdf) ===\nwrote %d bytes to %s\n", len($pdfBytes), $pdfPath);
