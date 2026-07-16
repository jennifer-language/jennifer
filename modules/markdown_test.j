# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# markdown_test.j - white-box tests for markdown.j. Run with:
#
#     jennifer test modules/markdown_test.j
#
# The overlay splices markdown.j in front of this file, so the tests reach its
# private helpers (parseInline, parseBlocks, lineType) and private structs
# (Span, Block) by bare identifier, alongside its exported toHtml / toAnsi. The
# spliced `import ... as ansi;` also makes `ansi.strip` reachable, so toAnsi
# output is compared with styling removed (deterministic regardless of TTY).
use testing;

# --- inline scanner (white-box) ---

func testInlinePlain() {
    def spans as list of Span init parseInline("just words");
    testing.assertEqual(len($spans), 1);
    testing.assertEqual($spans[0].kind, "text");
    testing.assertEqual($spans[0].text, "just words");
}

func testInlineMixed() {
    def spans as list of Span init parseInline("a **b** c `d` [e](u)");
    testing.assertEqual(len($spans), 6);
    testing.assertEqual($spans[0].kind, "text");
    testing.assertEqual($spans[1].kind, "strong");
    testing.assertEqual($spans[1].text, "b");
    testing.assertEqual($spans[3].kind, "code");
    testing.assertEqual($spans[3].text, "d");
    testing.assertEqual($spans[5].kind, "link");
    testing.assertEqual($spans[5].text, "e");
    testing.assertEqual($spans[5].url, "u");
}

func testInlineEmphasis() {
    def spans as list of Span init parseInline("*it*");
    testing.assertEqual($spans[0].kind, "em");
    testing.assertEqual($spans[0].text, "it");
}

# Space-flanked `*` (multiplication / decoration) is not emphasis (CommonMark
# flanking rules).
func testInlineSpaceFlankedStarIsLiteral() {
    def spans as list of Span init parseInline("compute 3 * 4 * 5 fast");
    testing.assertEqual(len($spans), 1);
    testing.assertEqual($spans[0].kind, "text");
    testing.assertEqual($spans[0].text, "compute 3 * 4 * 5 fast");
    # But a tight `*x*` still emphasizes.
    def em as list of Span init parseInline("a *x* b");
    testing.assertEqual($em[1].kind, "em");
    testing.assertEqual($em[1].text, "x");
}

func testInlineUnterminatedIsText() {
    # A lone marker with no closer stays literal text.
    def spans as list of Span init parseInline("a * b ` c");
    testing.assertEqual(len($spans), 1);
    testing.assertEqual($spans[0].kind, "text");
    testing.assertEqual($spans[0].text, "a * b ` c");
}

# --- block parser (white-box) ---

func testLineType() {
    testing.assertEqual(lineType("", ""), "blank");
    testing.assertEqual(lineType("```", "```"), "fence");
    testing.assertEqual(lineType("# H", "# H"), "heading");
    testing.assertEqual(lineType("- x", "- x"), "ul");
    testing.assertEqual(lineType("1. x", "1. x"), "ol");
    testing.assertEqual(lineType("word", "word"), "plain");
}

func testParseBlocksMixed() {
    def blocks as list of Block init parseBlocks("# Title\n\npara text\n\n- a\n- b");
    testing.assertEqual(len($blocks), 3);
    testing.assertEqual($blocks[0].kind, "heading");
    testing.assertEqual($blocks[0].level, 1);
    testing.assertEqual($blocks[1].kind, "paragraph");
    testing.assertEqual($blocks[2].kind, "list");
    testing.assertFalse($blocks[2].ordered);
    testing.assertEqual(len($blocks[2].items), 2);
}

func testParagraphJoinsLines() {
    def blocks as list of Block init parseBlocks("one\ntwo\nthree");
    testing.assertEqual(len($blocks), 1);
    testing.assertEqual($blocks[0].text, "one two three");
}

func testOrderedThenUnorderedSplit() {
    # Switching list type ends one list and starts another.
    def blocks as list of Block init parseBlocks("1. a\n2. b\n- c");
    testing.assertEqual(len($blocks), 2);
    testing.assertTrue($blocks[0].ordered);
    testing.assertFalse($blocks[1].ordered);
}

func testFenceContent() {
    def blocks as list of Block init parseBlocks("```\nl1\nl2\n```");
    testing.assertEqual(len($blocks), 1);
    testing.assertEqual($blocks[0].kind, "code");
    testing.assertEqual($blocks[0].text, "l1\nl2");
}

# A longer opening fence is not closed by a shorter inner backtick run: a
# ````-fenced block may contain a ``` line.
func testFenceLongerFenceContainsShorter() {
    def blocks as list of Block init parseBlocks("````\ncode with ``` inside\n````");
    testing.assertEqual(len($blocks), 1);
    testing.assertEqual($blocks[0].kind, "code");
    testing.assertEqual($blocks[0].text, "code with ``` inside");
}

# --- HTML rendering (public) ---

func testHtmlHeading() {
    testing.assertEqual(toHtml("## Hi & <you>"), "<h2>Hi &amp; &lt;you&gt;</h2>");
}

func testHtmlEmphasisAndCode() {
    testing.assertEqual(toHtml("a **b** *c* `d<e>`"),
        "<p>a <strong>b</strong> <em>c</em> <code>d&lt;e&gt;</code></p>");
}

func testHtmlLinkEscapesHref() {
    testing.assertEqual(toHtml("[t](http://x/?a=1&b=2)"),
        "<p><a href=\"http://x/?a=1&amp;b=2\">t</a></p>");
}

func testHtmlLists() {
    testing.assertEqual(toHtml("- a\n- b"), "<ul><li>a</li><li>b</li></ul>");
    testing.assertEqual(toHtml("1. a\n2. b"), "<ol><li>a</li><li>b</li></ol>");
}

func testHtmlCodeBlockEscapes() {
    testing.assertEqual(toHtml("```\nx < y & z\n```"), "<pre><code>x &lt; y &amp; z</code></pre>");
}

# --- ANSI rendering (public, styling stripped for determinism) ---

func testAnsiHeadingPlain() {
    testing.assertEqual(ansi.strip(toAnsi("# Title")), "Title");
}

func testAnsiListMarkers() {
    testing.assertEqual(ansi.strip(toAnsi("- one\n- two")), "  - one\n  - two");
}

func testAnsiOrderedNumbers() {
    testing.assertEqual(ansi.strip(toAnsi("1. a\n2. b")), "  1. a\n  2. b");
}

func testAnsiLinkShowsUrl() {
    testing.assertEqual(ansi.strip(toAnsi("see [site](http://x)")), "see site (http://x)");
}

func testAnsiCodeBlockIndented() {
    testing.assertEqual(ansi.strip(toAnsi("```\nl1\nl2\n```")), "    l1\n    l2");
}

func testAnsiBlocksSeparated() {
    # Two blocks are separated by a blank line.
    testing.assertEqual(ansi.strip(toAnsi("# H\n\nbody")), "H\n\nbody");
}

# --- authoring helpers (public) ---

func testAuthorHeader() {
    testing.assertEqual(header("h1", "Top"), "# Top");
    testing.assertEqual(header("h3", "Sub"), "### Sub");
    testing.assertEqual(header("h6", "Deep"), "###### Deep");
}

func testAuthorHeaderBadLevelThrows() {
    testing.assertThrows("badHeader", "value");
}
func badHeader() {
    return header("h7", "x");
}

func testAuthorStyle() {
    testing.assertEqual(style("bold", "b"), "**b**");
    testing.assertEqual(style("italic", "i"), "*i*");
    testing.assertEqual(style("code", "c"), "`c`");
}

func testAuthorStyleBadKindThrows() {
    testing.assertThrows("badStyle", "value");
}
func badStyle() {
    return style("underline", "x");
}

func testAuthorLink() {
    testing.assertEqual(link("site", "http://x"), "[site](http://x)");
}

func testAuthorLists() {
    def items as list of string init ["a", "b", "c"];
    testing.assertEqual(bullets($items), "- a\n- b\n- c");
    testing.assertEqual(numbered($items), "1. a\n2. b\n3. c");
}

func testAuthorCodeBlock() {
    testing.assertEqual(codeBlock("x = 1"), "```\nx = 1\n```");
}

# Authoring output round-trips back through the renderer.
func testAuthorRoundTripsToHtml() {
    def doc as string init header("h2", "T") + "\n\n" + style("bold", "hi") + " " + link("z", "u");
    testing.assertEqual(toHtml($doc), "<h2>T</h2><p><strong>hi</strong> <a href=\"u\">z</a></p>");
}

func testTableBasic() {
    def rows as list of list of string init [];
    $rows[] = ["Ada", "95"];
    $rows[] = ["Bo", "88"];
    def out as string init table(["Name", "Score"], [], $rows);
    testing.assertEqual($out, "| Name | Score |\n| --- | --- |\n| Ada | 95 |\n| Bo | 88 |");
}

func testTableAlignment() {
    def out as string init table(["L", "C", "R"], ["left", "center", "right"], []);
    testing.assertEqual($out, "| L | C | R |\n| :--- | :---: | ---: |");
}

func testTableEscapesPipeAndNewline() {
    def rows as list of list of string init [];
    $rows[] = ["a|b", "c\nd"];
    def out as string init table(["X", "Y"], [], $rows);
    testing.assertEqual($out, "| X | Y |\n| --- | --- |\n| a\\|b | c d |");
}

func testTableShortRowPads() {
    def rows as list of list of string init [];
    $rows[] = ["only"];
    def out as string init table(["A", "B", "C"], [], $rows);
    testing.assertEqual($out, "| A | B | C |\n| --- | --- | --- |\n| only |  |  |");
}

func testTableBadAlignThrows() {
    testing.assertThrows("badTable", "value");
}
func badTable() {
    return table(["A"], ["middle"], []);
}

# White-box: private alignment-cell mapping.
func testPrivateAlignSep() {
    testing.assertEqual(alignSep("left"), ":---");
    testing.assertEqual(alignSep("right"), "---:");
    testing.assertEqual(alignSep("center"), ":---:");
    testing.assertEqual(alignSep(""), "---");
    testing.assertEqual(alignSep("none"), "---");
}

# --- table parsing (white-box + public) ---

func testSplitCells() {
    def c as list of string init splitCells("| a | b\\|c | d |");
    testing.assertEqual(len($c), 3);
    testing.assertEqual($c[0], "a");
    testing.assertEqual($c[1], "b|c");   # escaped pipe is literal
    testing.assertEqual($c[2], "d");
}

func testDelimiterRowDetection() {
    testing.assertTrue(isDelimiterRow("| --- | :---: |"));
    testing.assertTrue(isDelimiterRow("---"));
    testing.assertFalse(isDelimiterRow("| a | b |"));
    testing.assertFalse(isDelimiterRow(""));
}

func testParseAligns() {
    def a as list of string init parseAligns("| :--- | ---: | :---: | --- |");
    testing.assertEqual($a[0], "left");
    testing.assertEqual($a[1], "right");
    testing.assertEqual($a[2], "center");
    testing.assertEqual($a[3], "none");
}

func testParseBlocksTable() {
    def blocks as list of Block init parseBlocks("| A | B |\n| --- | ---: |\n| 1 | 2 |\n| 3 | 4 |");
    testing.assertEqual(len($blocks), 1);
    testing.assertEqual($blocks[0].kind, "table");
    testing.assertEqual(len($blocks[0].headings), 2);
    testing.assertEqual($blocks[0].aligns[1], "right");
    testing.assertEqual(len($blocks[0].rows), 2);
    testing.assertEqual($blocks[0].rows[0][1], "2");
}

func testTableInterruptsParagraph() {
    def blocks as list of Block init parseBlocks("intro\n| a | b |\n| --- | --- |\n| 1 | 2 |");
    testing.assertEqual(len($blocks), 2);
    testing.assertEqual($blocks[0].kind, "paragraph");
    testing.assertEqual($blocks[1].kind, "table");
}

# A pipe line with no delimiter row underneath is an ordinary paragraph.
func testPipeLineWithoutDelimiterIsParagraph() {
    def blocks as list of Block init parseBlocks("a | b | c");
    testing.assertEqual(len($blocks), 1);
    testing.assertEqual($blocks[0].kind, "paragraph");
}

func testHtmlTable() {
    def src as string init "| N | S |\n| :--- | ---: |\n| Ada | 95 |";
    testing.assertEqual(toHtml($src),
        "<table><thead><tr><th align=\"left\">N</th><th align=\"right\">S</th></tr></thead>" +
        "<tbody><tr><td align=\"left\">Ada</td><td align=\"right\">95</td></tr></tbody></table>");
}

func testHtmlTableCellsAreInline() {
    def src as string init "| a |\n| --- |\n| **b** & `c<d>` |";
    testing.assertEqual(toHtml($src),
        "<table><thead><tr><th>a</th></tr></thead><tbody><tr><td>" +
        "<strong>b</strong> &amp; <code>c&lt;d&gt;</code>" +
        "</td></tr></tbody></table>");
}

func testHtmlTableShortRowPads() {
    def src as string init "| A | B | C |\n| --- | --- | --- |\n| only |";
    testing.assertEqual(toHtml($src),
        "<table><thead><tr><th>A</th><th>B</th><th>C</th></tr></thead>" +
        "<tbody><tr><td>only</td><td></td><td></td></tr></tbody></table>");
}

func testAnsiTableAligns() {
    # Columns align to the widest cell; right column is right-padded, divider rules.
    def src as string init "| step | ms |\n| :--- | ---: |\n| parse | 12 |\n| render | 8 |";
    testing.assertEqual(ansi.strip(toAnsi($src)),
        "step   | ms\n-------+---\nparse  | 12\nrender |  8");
}

# Authored tables round-trip through the reader now.
func testTableRoundTripsToHtml() {
    def rows as list of list of string init [];
    $rows[] = ["1", "2"];
    def src as string init table(["A", "B"], ["left", "right"], $rows);
    testing.assertEqual(toHtml($src),
        "<table><thead><tr><th align=\"left\">A</th><th align=\"right\">B</th></tr></thead>" +
        "<tbody><tr><td align=\"left\">1</td><td align=\"right\">2</td></tr></tbody></table>");
}

# --- tablePretty (public) ---

func testTablePrettyAligns() {
    def messy as string init "| Name | Score |\n|:-|-:|\n| Ada | 95 |\n| Grace | 8 |";
    testing.assertEqual(tablePretty($messy),
        "| Name  | Score |\n| :---- | ----: |\n| Ada   |    95 |\n| Grace |     8 |");
}

func testTablePrettyPreservesOtherLines() {
    def src as string init "# Title\n\n| a | b |\n| - | - |\n| 1 | 2 |\n\ntail | pipe not a table";
    def out as string init tablePretty($src);
    # Non-table lines pass through byte-for-byte.
    testing.assertTrue(strings.startsWith($out, "# Title\n\n"));
    testing.assertTrue(strings.endsWith($out, "\ntail | pipe not a table"));
    testing.assertContains($out, "| a   | b   |");
}

func testTablePrettyIdempotent() {
    def src as string init "| a | b |\n| --- | --- |\n| x\\|y | z |";
    def once as string init tablePretty($src);
    testing.assertEqual(tablePretty($once), $once);
    # Escaped pipe survives.
    testing.assertContains($once, "x\\|y");
}

func testTablePrettyNoTableUnchanged() {
    def src as string init "just a paragraph\nwith two lines";
    testing.assertEqual(tablePretty($src), $src);
}
