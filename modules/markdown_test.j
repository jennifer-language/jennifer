# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
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
    testing.assertEqual($spans[0].kind, SpanKind.Text);
    testing.assertEqual($spans[0].text, "just words");
}

func testInlineMixed() {
    def spans as list of Span init parseInline("a **b** c `d` [e](u)");
    testing.assertEqual(len($spans), 6);
    testing.assertEqual($spans[0].kind, SpanKind.Text);
    testing.assertEqual($spans[1].kind, SpanKind.Strong);
    testing.assertEqual($spans[1].text, "b");
    testing.assertEqual($spans[3].kind, SpanKind.Code);
    testing.assertEqual($spans[3].text, "d");
    testing.assertEqual($spans[5].kind, SpanKind.Link);
    testing.assertEqual($spans[5].text, "e");
    testing.assertEqual($spans[5].url, "u");
}

func testInlineEmphasis() {
    def spans as list of Span init parseInline("*it*");
    testing.assertEqual($spans[0].kind, SpanKind.Em);
    testing.assertEqual($spans[0].text, "it");
}

# Space-flanked `*` (multiplication / decoration) is not emphasis (CommonMark
# flanking rules).
func testInlineSpaceFlankedStarIsLiteral() {
    def spans as list of Span init parseInline("compute 3 * 4 * 5 fast");
    testing.assertEqual(len($spans), 1);
    testing.assertEqual($spans[0].kind, SpanKind.Text);
    testing.assertEqual($spans[0].text, "compute 3 * 4 * 5 fast");
    # But a tight `*x*` still emphasizes.
    def em as list of Span init parseInline("a *x* b");
    testing.assertEqual($em[1].kind, SpanKind.Em);
    testing.assertEqual($em[1].text, "x");
}

func testInlineUnterminatedIsText() {
    # A lone marker with no closer stays literal text.
    def spans as list of Span init parseInline("a * b ` c");
    testing.assertEqual(len($spans), 1);
    testing.assertEqual($spans[0].kind, SpanKind.Text);
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
    testing.assertEqual($blocks[0].kind, BlockKind.Heading);
    testing.assertEqual($blocks[0].level, 1);
    testing.assertEqual($blocks[1].kind, BlockKind.Paragraph);
    testing.assertEqual($blocks[2].kind, BlockKind.List);
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
    testing.assertEqual($blocks[0].kind, BlockKind.Code);
    testing.assertEqual($blocks[0].text, "l1\nl2");
}

# A longer opening fence is not closed by a shorter inner backtick run: a
# ````-fenced block may contain a ``` line.
func testFenceLongerFenceContainsShorter() {
    def blocks as list of Block init parseBlocks("````\ncode with ``` inside\n````");
    testing.assertEqual(len($blocks), 1);
    testing.assertEqual($blocks[0].kind, BlockKind.Code);
    testing.assertEqual($blocks[0].text, "code with ``` inside");
}

# --- images, blockquotes, nested lists ---

func testInlineImage() {
    def spans as list of Span init parseInline("![a cat](/cat.png)");
    testing.assertEqual(len($spans), 1);
    testing.assertEqual($spans[0].kind, SpanKind.Image);
    testing.assertEqual($spans[0].text, "a cat");
    testing.assertEqual($spans[0].url, "/cat.png");
    # an image mixed with surrounding text and a title
    def mixed as list of Span init parseInline("see ![x](/u.png \"t\") end");
    testing.assertEqual($mixed[1].kind, SpanKind.Image);
    testing.assertEqual($mixed[1].title, "t");
}

func testHtmlImage() {
    testing.assertEqual(
        toHtml("![a cat](/cat.png \"kitty\")"),
        "<p><img src=\"/cat.png\" alt=\"a cat\" title=\"kitty\"></p>");
}

# An image src runs through the same scheme allowlist as a link href.
func testHtmlImageNeutralizesScriptSrc() {
    def out as string init toHtml("![x](javascript:alert(1))");
    testing.assertContains($out, "src=\"#\"");
    testing.assertFalse(strings.contains($out, "src=\"javascript"));
}

func testBlockquoteParse() {
    def blocks as list of Block init parseBlocks("> hello\n> world");
    testing.assertEqual(len($blocks), 1);
    testing.assertEqual($blocks[0].kind, BlockKind.Quote);
    testing.assertEqual(len($blocks[0].children), 1);
    testing.assertEqual($blocks[0].children[0].kind, BlockKind.Paragraph);
}

func testHtmlBlockquote() {
    testing.assertEqual(
        toHtml("> quoted **text**"),
        "<blockquote><p>quoted <strong>text</strong></p></blockquote>");
}

# A `> >` line nests one blockquote inside another (parsed recursively).
func testHtmlNestedBlockquote() {
    testing.assertEqual(
        toHtml("> a\n>\n> > b"),
        "<blockquote><p>a</p><blockquote><p>b</p></blockquote></blockquote>");
}

# A blockquote can hold a list.
func testHtmlBlockquoteWithList() {
    testing.assertEqual(
        toHtml("> - a\n> - b"),
        "<blockquote><ul><li>a</li><li>b</li></ul></blockquote>");
}

func testNestedListParse() {
    def blocks as list of Block init parseBlocks("- a\n  - b\n  - c\n- d");
    testing.assertEqual(len($blocks), 1);
    testing.assertEqual($blocks[0].kind, BlockKind.List);
    testing.assertEqual(len($blocks[0].items), 2); # a, d at the top level
    testing.assertEqual($blocks[0].items[0], "a");
    testing.assertEqual(len($blocks[0].children[0].items), 2); # b, c nested under a
    testing.assertEqual(len($blocks[0].children[1].items), 0); # d has no nesting
}

func testHtmlNestedList() {
    testing.assertEqual(
        toHtml("- a\n  - b\n  - c\n- d"),
        "<ul><li>a<ul><li>b</li><li>c</li></ul></li><li>d</li></ul>");
}

func testHtmlNestedOrderedUnderUnordered() {
    testing.assertEqual(
        toHtml("- top\n  1. one\n  2. two"),
        "<ul><li>top<ol><li>one</li><li>two</li></ol></li></ul>");
}

func testAnsiNestedList() {
    testing.assertEqual(ansi.strip(toAnsi("- a\n  - b\n- c")), "  - a\n    - b\n  - c");
}

# Inconsistently indented deeper items (4 then 2 spaces, both under `a`) must not
# drop content: the second deeper run merges into the first item's sub-list.
func testNestedListInconsistentIndentKeepsAll() {
    testing.assertEqual(
        toHtml("- a\n    - b\n  - c"),
        "<ul><li>a<ul><li>b</li><li>c</li></ul></li></ul>");
}

# A three-level list nests to three depths.
func testNestedListThreeLevels() {
    testing.assertEqual(
        toHtml("- a\n  - b\n    - c\n- d"),
        "<ul><li>a<ul><li>b<ul><li>c</li></ul></li></ul></li><li>d</li></ul>");
}

func testAnsiBlockquote() {
    testing.assertEqual(ansi.strip(toAnsi("> hello\n> world")), "> hello world");
}

# --- HTML rendering (public) ---

func testHtmlHeading() {
    testing.assertEqual(toHtml("## Hi & <you>"), "<h2>Hi &amp; &lt;you&gt;</h2>");
}

func testHtmlEmphasisAndCode() {
    testing.assertEqual(
        toHtml("a **b** *c* `d<e>`"),
        "<p>a <strong>b</strong> <em>c</em> <code>d&lt;e&gt;</code></p>");
}

func testHtmlLinkEscapesHref() {
    testing.assertEqual(
        toHtml("[t](http://x/?a=1&b=2)"),
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
    testing.assertEqual($c[1], "b|c"); # escaped pipe is literal
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
    testing.assertEqual($blocks[0].kind, BlockKind.Table);
    testing.assertEqual(len($blocks[0].headings), 2);
    testing.assertEqual($blocks[0].aligns[1], "right");
    testing.assertEqual(len($blocks[0].rows), 2);
    testing.assertEqual($blocks[0].rows[0][1], "2");
}

func testTableInterruptsParagraph() {
    def blocks as list of Block init parseBlocks("intro\n| a | b |\n| --- | --- |\n| 1 | 2 |");
    testing.assertEqual(len($blocks), 2);
    testing.assertEqual($blocks[0].kind, BlockKind.Paragraph);
    testing.assertEqual($blocks[1].kind, BlockKind.Table);
}

# A pipe line with no delimiter row underneath is an ordinary paragraph.
func testPipeLineWithoutDelimiterIsParagraph() {
    def blocks as list of Block init parseBlocks("a | b | c");
    testing.assertEqual(len($blocks), 1);
    testing.assertEqual($blocks[0].kind, BlockKind.Paragraph);
}

func testHtmlTable() {
    def src as string init "| N | S |\n| :--- | ---: |\n| Ada | 95 |";
    testing.assertEqual(
        toHtml($src),
        "<table><thead><tr><th align=\"left\">N</th><th align=\"right\">S</th></tr></thead>" +
            "<tbody><tr><td align=\"left\">Ada</td><td align=\"right\">95</td></tr></tbody></table>");
}

func testHtmlTableCellsAreInline() {
    def src as string init "| a |\n| --- |\n| **b** & `c<d>` |";
    testing.assertEqual(
        toHtml($src),
        "<table><thead><tr><th>a</th></tr></thead><tbody><tr><td>" +
            "<strong>b</strong> &amp; <code>c&lt;d&gt;</code>" +
            "</td></tr></tbody></table>");
}

func testHtmlTableShortRowPads() {
    def src as string init "| A | B | C |\n| --- | --- | --- |\n| only |";
    testing.assertEqual(
        toHtml($src),
        "<table><thead><tr><th>A</th><th>B</th><th>C</th></tr></thead>" +
            "<tbody><tr><td>only</td><td></td><td></td></tr></tbody></table>");
}

func testAnsiTableAligns() {
    # Columns align to the widest cell; right column is right-padded, divider rules.
    def src as string init "| step | ms |\n| :--- | ---: |\n| parse | 12 |\n| render | 8 |";
    testing.assertEqual(
        ansi.strip(toAnsi($src)),
        "step   | ms\n-------+---\nparse  | 12\nrender |  8");
}

# Authored tables round-trip through the reader now.
func testTableRoundTripsToHtml() {
    def rows as list of list of string init [];
    $rows[] = ["1", "2"];
    def src as string init table(["A", "B"], ["left", "right"], $rows);
    testing.assertEqual(
        toHtml($src),
        "<table><thead><tr><th align=\"left\">A</th><th align=\"right\">B</th></tr></thead>" +
            "<tbody><tr><td align=\"left\">1</td><td align=\"right\">2</td></tr></tbody></table>");
}

# --- tablePretty (public) ---

func testTablePrettyAligns() {
    def messy as string init "| Name | Score |\n|:-|-:|\n| Ada | 95 |\n| Grace | 8 |";
    testing.assertEqual(
        tablePretty($messy),
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

# --- link href sanitization (XSS) ---

func testSafeHrefAllowsSafeSchemes() {
    testing.assertEqual(safeHref("http://example.com"), "http://example.com");
    testing.assertEqual(safeHref("https://example.com/p?q=1#x"), "https://example.com/p?q=1#x");
    testing.assertEqual(safeHref("mailto:me@example.com"), "mailto:me@example.com");
}

func testSafeHrefAllowsRelative() {
    testing.assertEqual(safeHref("/about"), "/about");
    testing.assertEqual(safeHref("./page"), "./page");
    testing.assertEqual(safeHref("../up"), "../up");
    testing.assertEqual(safeHref("#section"), "#section");
    testing.assertEqual(safeHref("page.html"), "page.html");
    testing.assertEqual(safeHref(""), "");
}

func testSafeHrefBlocksScriptSchemes() {
    testing.assertEqual(safeHref("javascript:alert(1)"), "#");
    # Case-insensitive.
    testing.assertEqual(safeHref("JavaScript:alert(1)"), "#");
    # Leading whitespace is stripped before the scheme is read (browsers do too).
    testing.assertEqual(safeHref("  javascript:alert(1)"), "#");
    # An embedded control char inside the scheme does not smuggle it past.
    testing.assertEqual(safeHref("java\tscript:alert(1)"), "#");
    testing.assertEqual(safeHref("data:text/html,x"), "#");
    testing.assertEqual(safeHref("vbscript:msgbox(1)"), "#");
}

func testToHtmlNeutralizesJavascriptLink() {
    def out as string init toHtml("[click](javascript:alert(1))");
    testing.assertContains($out, "href=\"#\"");
    testing.assertFalse(strings.contains($out, "href=\"javascript"));
}

func testToHtmlKeepsSafeLink() {
    def out as string init toHtml("[post](https://example.com/x)");
    testing.assertContains($out, "href=\"https://example.com/x\"");
}

# --- public document tree: reader (parse + accessors) ---

func testParseDocumentRoot() {
    def doc as Node init parse("# H\n\ntext\n");
    testing.assertEqual(typeOf($doc), "document");
    testing.assertEqual(len(children($doc)), 2);
}

func testReaderHeadingLevelText() {
    def doc as Node init parse("## Hello *world*\n");
    def h as Node init get($doc, "heading");
    testing.assertEqual(typeOf($h), "heading");
    testing.assertEqual(level($h), 2);
    # text flattens the inline children (the emphasis contributes "world").
    testing.assertEqual(text($h), "Hello world");
}

func testReaderLinkAttrs() {
    def doc as Node init parse("see [site](https://example.com \"home\")\n");
    def a as Node init get($doc, "paragraph/link");
    testing.assertEqual(attr($a, "href"), "https://example.com");
    testing.assertEqual(attr($a, "title"), "home");
    testing.assertEqual(text($a), "site");
}

func testReaderCodeLanguage() {
    def doc as Node init parse("```python\nprint(1)\n```\n");
    def c as Node init get($doc, "code");
    testing.assertEqual(attr($c, "lang"), "python");
    testing.assertEqual(text($c), "print(1)");
}

func testReaderOrderedList() {
    def doc as Node init parse("1. a\n2. b\n");
    def l as Node init get($doc, "list");
    testing.assertEqual(attr($l, "ordered"), "true");
    testing.assertEqual(len(findAll($l, "item")), 2);
}

func testReaderUnorderedList() {
    def doc as Node init parse("- a\n- b\n- c\n");
    def l as Node init get($doc, "list");
    testing.assertEqual(attr($l, "ordered"), "false");
    testing.assertEqual(len(findAll($l, "item")), 3);
}

func testReaderTableCellAlign() {
    def doc as Node init parse("| A | B |\n|:--|--:|\n| 1 | 2 |\n");
    def t as Node init get($doc, "table");
    # The header is row[1]; its second cell is right-aligned.
    def cell as Node init get($t, "row[1]/cell[2]");
    testing.assertEqual(attr($cell, "align"), "right");
}

func testReaderNestedListItem() {
    def doc as Node init parse("- top\n  - nested\n");
    def item as Node init get($doc, "list/item");
    testing.assertTrue(has($item, "list"));
}

func testSelectorWildcardAndIndex() {
    def doc as Node init parse("# a\n\n# b\n\n# c\n");
    testing.assertEqual(len(findAll($doc, "*")), 3);
    testing.assertEqual(text(get($doc, "heading[2]")), "b");
}

func testTextFlattensDescendants() {
    def doc as Node init parse("a **bold** and `code`\n");
    def p as Node init get($doc, "paragraph");
    testing.assertEqual(text($p), "a bold and code");
}

func testGetMissingIsEmptyNode() {
    def doc as Node init parse("just text\n");
    testing.assertEqual(typeOf(get($doc, "table")), "");
    testing.assertFalse(has($doc, "table"));
}

# --- public document tree: render (from the tree) ---

func testRenderHtmlMatchesToHtml() {
    def src as string init "# H\n\ntext with [x](http://a) and `c`\n\n- one\n- two\n";
    testing.assertEqual(render(parse($src), "html"), toHtml($src));
}

func testRenderAnsiMatchesToAnsi() {
    def src as string init "# H\n\n| A | B |\n|--|--|\n| 1 | 2 |\n";
    testing.assertEqual(render(parse($src), "ansi"), toAnsi($src));
}

func testRenderRoundTripTransform() {
    # parse -> rewrite a link's url -> render: the new url appears, the old is gone.
    def doc as Node init parse("go [here](http://old.example)\n");
    def docKids as list of Node init children($doc);
    def para as Node init $docKids[0];
    def pkids as list of Node init children($para);
    def j as int init 0;
    while ($j < len($pkids)) {
        if (typeOf($pkids[$j]) == "link") {
            $pkids[$j].url = "http://new.example";
        }
        $j = $j + 1;
    }
    $para.children = $pkids;
    $docKids[0] = $para;
    $doc.children = $docKids;
    def out as string init render($doc, "html");
    testing.assertContains($out, "http://new.example");
    testing.assertFalse(strings.contains($out, "old.example"));
}

func renderBadFormat() {
    render(parse("x\n"), "pdf");
}

func testRenderUnknownFormatThrows() {
    testing.assertThrows("renderBadFormat", "markdown");
}

func deepBlockConversion() {
    # A depth past the cap is a catchable "markdown" error, not a stack overflow.
    def b as Block init headingBlock(1, "x");
    blockToPublic($b, 200);
}

func testDepthCapThrows() {
    testing.assertThrows("deepBlockConversion", "markdown");
}

func testRenderEmptyTableNodeNoCrash() {
    # A hand-built table with no rows renders empty rather than indexing past the end.
    def t as Node;
    $t.kind = "table";
    def doc as Node;
    $doc.kind = "document";
    $doc.children = [$t];
    testing.assertEqual(render($doc, "ansi"), "");
}

# --- PDF rendering (markdown.toPdf, white-box) ---

use binary;

func pdfMarker() {
    return convert.bytesFromString("%PDF", "utf-8");
}

# --- options + metrics ---------------------------------------------

func testDefaults() {
    def o as PdfOptions init pdfDefaults();
    testing.assertEqual($o.pageWidth, 612);
    testing.assertEqual($o.pageHeight, 792);
    testing.assertEqual($o.margin, 54);
    testing.assertEqual($o.bodyFont, "Helvetica");
    testing.assertEqual($o.boldFont, "Helvetica-Bold");
    testing.assertEqual($o.monoFont, "Courier");
}

func testLineHeight() {
    testing.assertEqual(lineH(11), 15);
    testing.assertEqual(lineH(24), 33);
}

func testHeadingSizeByLevel() {
    def o as PdfOptions init pdfDefaults();
    testing.assertEqual(headingSize(1, $o), 22);
    testing.assertEqual(headingSize(2, $o), 17);
    testing.assertEqual(headingSize(6, $o), $o.bodySize);
    # An out-of-range level falls back to the body size.
    testing.assertEqual(headingSize(9, $o), $o.bodySize);
}

# --- inline layout --------------------------------------------------

func testFontForInlineByKind() {
    def o as PdfOptions init pdfDefaults();
    def doc as Node init parse("x **b** *i* `c`\n");
    def p as Node init get($doc, "paragraph");
    for (def n in children($p)) {
        def k as string init typeOf($n);
        def f as string init fontForInline($n, $o);
        if ($k == "strong") {
            testing.assertEqual($f, $o.boldFont);
        } elseif ($k == "emphasis") {
            testing.assertEqual($f, $o.italicFont);
        } elseif ($k == "codespan") {
            testing.assertEqual($f, $o.monoFont);
        } else {
            testing.assertEqual($f, $o.bodyFont);
        }
    }
}

func testInlineWordsSplitsAndTags() {
    def o as PdfOptions init pdfDefaults();
    def doc as Node init parse("hello world **bold text**\n");
    def p as Node init get($doc, "paragraph");
    def words as list of IWord init inlineWords(children($p), $o);
    testing.assertEqual(len($words), 4);
    testing.assertEqual($words[0].text, "hello");
    testing.assertEqual($words[0].font, $o.bodyFont);
    testing.assertEqual($words[3].text, "text");
    testing.assertEqual($words[3].font, $o.boldFont);
}

func testImageInlineRendersAlt() {
    def o as PdfOptions init pdfDefaults();
    def doc as Node init parse("![a cat](/c.png)\n");
    def p as Node init get($doc, "paragraph");
    def words as list of IWord init inlineWords(children($p), $o);
    # "[a" and "cat]" - the alt text bracketed.
    testing.assertEqual($words[0].text, "[a");
    testing.assertEqual($words[1].text, "cat]");
}

func testPackLinesRespectsWidth() {
    def o as PdfOptions init pdfDefaults();
    def words as list of IWord init [
        IWord{text: "aaaa", font: "Helvetica"},
        IWord{text: "bbbb", font: "Helvetica"},
        IWord{text: "cccc", font: "Helvetica"}
    ];
    # A wide column fits all three on one line.
    testing.assertEqual(len(packLines($words, 11, 500, $o)), 1);
    # A tiny column forces one word per line.
    testing.assertEqual(len(packLines($words, 11, 5, $o)), 3);
}

# --- pagination mechanics ------------------------------------------

func testEnsureSpaceFlushesWhenFull() {
    def s as Layout init newLayout(pdfDefaults());
    # Drive the pen near the bottom margin, then demand more than fits.
    $s.y = 77;
    $s = ensureSpace($s, 50);
    # A flush resets the pen to the top of a fresh page (pageHeight - margin).
    testing.assertEqual($s.y, 738);
}

func testEnsureSpaceKeepsPageWhenRoom() {
    def s as Layout init newLayout(pdfDefaults());
    def before as int init $s.y;
    $s = ensureSpace($s, 20);
    testing.assertEqual($s.y, $before);
}

# --- end-to-end rendering ------------------------------------------

func testRenderProducesValidPdf() {
    def out as bytes init toPdf("# Hi\n\nA paragraph of text.\n");
    testing.assertTrue(len($out) > 100);
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}

func testRenderAllBlockTypes() {
    def md as string init "# H\n\ntext **b** *i* `c`\n\n- a\n- b\n  - nested\n\n1. one\n2. two\n\n> quoted\n\n```\ncode\n```\n\n| A | B |\n|:--|--:|\n| 1 | 2 |\n";
    def out as bytes init toPdf($md);
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}

func testEmptyDocRendersBlankPdf() {
    def out as bytes init toPdf("");
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}

func testRenderTreeOnParsedTree() {
    def doc as Node init parse("# Tree\n\nrendered via renderPdf.\n");
    def out as bytes init renderPdf($doc, pdfDefaults());
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}

func testRenderWithA4Options() {
    def o as PdfOptions init pdfDefaults();
    $o.pageWidth = 595;
    $o.pageHeight = 842;
    def out as bytes init toPdfWith("# A4\n\nbody text\n", $o);
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}

# A long document overflows onto more pages, so it renders more bytes than a short
# one (a proxy for pagination without decoding the PDF).
func testLongDocumentPaginates() {
    def md as string init "# Long\n\n";
    def i as int init 0;
    while ($i < 80) {
        $md = $md + "Paragraph with several words that occupy a line of the body column area.\n\n";
        $i = $i + 1;
    }
    def long as bytes init toPdf($md);
    def short as bytes init toPdf("# Short\n\none line.\n");
    testing.assertTrue(len($long) > len($short));
}

func testFillHelpers() {
    testing.assertFalse(noFill().on);
    def g as Fill init gray(200);
    testing.assertTrue($g.on);
    testing.assertEqual($g.r, 200);
    testing.assertEqual($g.b, 200);
    def c as Fill init rgb(10, 20, 30);
    testing.assertEqual($c.r, 10);
    testing.assertEqual($c.g, 20);
    testing.assertEqual($c.b, 30);
}

func testHeadingAndTableFillRender() {
    def o as PdfOptions init pdfDefaults();
    $o.tableHeaderFill = gray(230);
    $o.headingStyles = [headingStyle(gray(205)), headingStyle(gray(225))];
    def out as bytes init toPdfWith("# H1\n\n## H2\n\n| A | B |\n|--|--|\n| 1 | 2 |\n", $o);
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}
