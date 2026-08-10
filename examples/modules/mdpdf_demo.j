# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The mdpdf module (modules/mdpdf.j): turn a Markdown document into a laid-out PDF -
 * headings, styled paragraphs, lists, a blockquote, a code block, and a GFM table -
 * flowed onto paginated pages with the `pdf` layout primitives.
 * @module mdpdf_demo
 */
use io;
use fs;
use os;
use path;
use strings;
import "../../modules/markdown.j" as markdown;
import "../../modules/mdpdf.j" as mdpdf;

# Build the source by appending lines to a list and joining once - O(N), unlike
# repeated `$doc = $doc + ...` string concatenation, which copies the whole growing
# string each time.
def lines as list of string init [];
$lines[] = "# Quarterly Report";
$lines[] = "";
$lines[] = "This summary has **bold**, *italic*, and `inline code`, and enough words to wrap onto a second line within the page column.";
$lines[] = "";
$lines[] = "## Highlights";
$lines[] = "";
$lines[] = "- Revenue up, on top of a strong prior quarter";
$lines[] = "  - EMEA led the growth";
$lines[] = "- Costs held flat";
$lines[] = "";
$lines[] = "> A quoted remark from the review, indented from the body.";
$lines[] = "";
$lines[] = "```";
$lines[] = "total = revenue - costs";
$lines[] = "```";
$lines[] = "";
$lines[] = "| Metric | Q3 | Q4 |";
$lines[] = "|:-------|---:|---:|";
$lines[] = "| Revenue | 90 | 120 |";
$lines[] = "| Margin | 30 | 44 |";
def doc as string init strings.join($lines, "\n");

# The simple path: Markdown string in, PDF bytes out.
def out as bytes init mdpdf.render($doc);

# Read the tree first when you want to transform or inspect before rendering: the
# `markdown` reader walks it, and `mdpdf` renders the same tree.
def tree as markdown.Node init markdown.parse($doc);
io.printf("headings in the document:\n");
for (def h in markdown.findAll($tree, "heading")) {
    io.printf("  - %s\n", markdown.text($h));
}

# Custom geometry: A4 with tighter margins, rendered from the parsed tree.
def a4 as mdpdf.Options init mdpdf.defaults();
$a4.pageWidth = 595;
$a4.pageHeight = 842;
$a4.margin = 54;
def a4Out as bytes init mdpdf.renderTree($tree, $a4);

def outPath as string init path.join(os.tempDir(), "mdpdf_demo.pdf");
fs.writeBytes($outPath, $out);
io.printf("\nwrote %d bytes (Letter) and rendered %d bytes (A4) from the same tree\n", len($out), len($a4Out));
io.printf("open it: %s\n", $outPath);
