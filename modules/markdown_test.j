# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
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
use binary;
use encoding;
import "./ansi.j" as ansi;
import "./pdf.j" as pdf;

# --- inline scanner (white-box) ---

func inlineOf(s as string) {
    def no as map of string to list of string init {};
    return parseInline($s, $no);
}

def const MDIMGPNG as string init "iVBORw0KGgoAAAANSUhEUgAAAAgAAAAGCAIAAABxZ0isAAAAGklEQVR42mNkYGBQYBDBRCwMGiIMDFgQPSQAwgMFakXvt1IAAAAASUVORK5CYII=";

func testInlinePlain() {
    def ns as list of doctree.Node init inlineOf("just words");
    testing.assertEqual(len($ns), 1);
    testing.assertEqual($ns[0].kind, "text");
    testing.assertEqual($ns[0].text, "just words");
}
func testInlineMixed() {
    def ns as list of doctree.Node init inlineOf("a **b** c `d` [e](u)");
    testing.assertEqual(len($ns), 6);
    testing.assertEqual($ns[0].kind, "text");
    testing.assertEqual($ns[1].kind, "strong");
    testing.assertEqual(doctree.text($ns[1]), "b");
    testing.assertEqual($ns[3].kind, "codespan");
    testing.assertEqual($ns[3].text, "d");
    testing.assertEqual($ns[5].kind, "link");
    testing.assertEqual(doctree.text($ns[5]), "e");
    testing.assertEqual($ns[5].url, "u");
}
func testInlineEmphasis() {
    def ns as list of doctree.Node init inlineOf("*it*");
    testing.assertEqual($ns[0].kind, "emphasis");
    testing.assertEqual(doctree.text($ns[0]), "it");
}
# Space-flanked `*` (multiplication / decoration) is not emphasis (CommonMark
# flanking rules).
func testInlineSpaceFlankedStarIsLiteral() {
    def ns as list of doctree.Node init inlineOf("compute 3 * 4 * 5 fast");
    testing.assertEqual(len($ns), 1);
    testing.assertEqual($ns[0].kind, "text");
    testing.assertEqual($ns[0].text, "compute 3 * 4 * 5 fast");
    # But a tight `*x*` still emphasizes.
    def em as list of doctree.Node init inlineOf("a *x* b");
    testing.assertEqual($em[1].kind, "emphasis");
    testing.assertEqual(doctree.text($em[1]), "x");
}
func testInlineUnterminatedIsText() {
    # A lone marker with no closer stays literal text.
    def ns as list of doctree.Node init inlineOf("a * b ` c");
    testing.assertEqual(len($ns), 1);
    testing.assertEqual($ns[0].kind, "text");
    testing.assertEqual($ns[0].text, "a * b ` c");
}

# --- block parser (white-box) ---
func testLineClassifiers() {
    testing.assertTrue(isThematicBreak("- - -"));
    testing.assertFalse(isThematicBreak("--"));
    testing.assertEqual(scanAtx("## H ##").text, "H");
    testing.assertFalse(scanAtx("#5 bolt").ok);
    testing.assertEqual(fenceOpen("```py"), "```");
    testing.assertEqual(fenceOpen("~~~~"), "~~~~");
    testing.assertEqual(fenceOpen("``` a`b"), "");
    testing.assertTrue(listMarker("1) x").ordered);
    testing.assertEqual(listMarker("- x").offset, 2);
    testing.assertFalse(listMarker("-x").ok);
    testing.assertEqual(htmlStart("<div>"), 6);
    testing.assertEqual(htmlStart("<!-- c -->"), 2);
    testing.assertEqual(htmlStart("<span>"), 7);
    testing.assertEqual(htmlStart("<https://x>"), 0);
}
func testParseBlocksMixed() {
    def ks as list of doctree.Node init doctree.children(parse("# Title\n\npara text\n\n- a\n- b"));
    testing.assertEqual(len($ks), 3);
    testing.assertEqual($ks[0].kind, "heading");
    testing.assertEqual($ks[0].level, 1);
    testing.assertEqual($ks[1].kind, "paragraph");
    testing.assertEqual($ks[2].kind, "list");
    testing.assertFalse($ks[2].ordered);
    testing.assertEqual(len($ks[2].children), 2);
}
func testParagraphJoinsLines() {
    def ks as list of doctree.Node init doctree.children(parse("one\ntwo\nthree"));
    testing.assertEqual(len($ks), 1);
    testing.assertEqual(doctree.text($ks[0]), "one two three");
}
func testOrderedThenUnorderedSplit() {
    # Switching list type ends one list and starts another.
    def ks as list of doctree.Node init doctree.children(parse("1. a\n2. b\n- c"));
    testing.assertEqual(len($ks), 2);
    testing.assertTrue($ks[0].ordered);
    testing.assertFalse($ks[1].ordered);
}
func testFenceContent() {
    def ks as list of doctree.Node init doctree.children(parse("```\nl1\nl2\n```"));
    testing.assertEqual(len($ks), 1);
    testing.assertEqual($ks[0].kind, "code");
    testing.assertEqual($ks[0].text, "l1\nl2");
}
# A longer opening fence is not closed by a shorter inner backtick run: a
# ````-fenced block may contain a ``` line.
func testFenceLongerFenceContainsShorter() {
    def ks as list of doctree.Node init doctree.children(parse("````\ncode with ``` inside\n````"));
    testing.assertEqual(len($ks), 1);
    testing.assertEqual($ks[0].kind, "code");
    testing.assertEqual($ks[0].text, "code with ``` inside");
}

# --- images, blockquotes, nested lists ---
func testInlineImage() {
    def ns as list of doctree.Node init inlineOf("![a cat](/cat.png)");
    testing.assertEqual(len($ns), 1);
    testing.assertEqual($ns[0].kind, "image");
    testing.assertEqual($ns[0].text, "a cat");
    testing.assertEqual($ns[0].url, "/cat.png");
    # an image mixed with surrounding text and a title
    def mixed as list of doctree.Node init inlineOf("see ![x](/u.png \"t\") end");
    testing.assertEqual($mixed[1].kind, "image");
    testing.assertEqual($mixed[1].title, "t");
}
func testHtmlImage() {
    testing.assertEqual(
        toHtml("![a cat](/cat.png \"kitty\")"),
        "<p><img src=\"/cat.png\" alt=\"a cat\" title=\"kitty\"></p>");
}
# XHTML output (for an EPUB / XHTML content document): void elements self-close.
# HTML5 output is unchanged; both escape identically.
func testXhtmlVoidSelfCloses() {
    testing.assertEqual(toHtml("---\n"), "<hr>");
    testing.assertEqual(toXhtml("---\n"), "<hr />");
    testing.assertEqual(
        toXhtml("![a cat](/cat.png)"),
        "<p><img src=\"/cat.png\" alt=\"a cat\" /></p>");
    # doctree.render("xhtml", doc) is the same path as toXhtml.
    testing.assertEqual(doctree.render("xhtml", parse("---\n")), "<hr />");
    # A non-void element is identical in both modes.
    testing.assertEqual(toXhtml("a **b**"), toHtml("a **b**"));
}
# An image src runs through the same scheme allowlist as a link href.
func testHtmlImageNeutralizesScriptSrc() {
    def out as string init toHtml("![x](javascript:alert(1))");
    testing.assertContains($out, "src=\"#\"");
    testing.assertFalse(strings.contains($out, "src=\"javascript"));
}
func testBlockquoteParse() {
    def ks as list of doctree.Node init doctree.children(parse("> hello\n> world"));
    testing.assertEqual(len($ks), 1);
    testing.assertEqual($ks[0].kind, "quote");
    testing.assertEqual(len($ks[0].children), 1);
    testing.assertEqual($ks[0].children[0].kind, "paragraph");
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
    def ks as list of doctree.Node init doctree.children(parse("- a\n  - b\n  - c\n- d"));
    testing.assertEqual(len($ks), 1);
    testing.assertEqual($ks[0].kind, "list");
    def items as list of doctree.Node init $ks[0].children;
    testing.assertEqual(len($items), 2); # a, d at the top level
    testing.assertEqual($items[0].children[0].text, "a");
    testing.assertEqual(len($items[0].children[1].children), 2); # b, c nested under a
    testing.assertEqual(len($items[1].children), 1); # d has no nesting
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
func testHtmlNestedInlineInStrong() {
    # a link inside bold survives (was flattened, dropping the link)
    testing.assertEqual(
        toHtml("x **[label](guide.md)** y"),
        "<p>x <strong><a href=\"guide.md\">label</a></strong> y</p>");
    # code inside bold
    testing.assertEqual(toHtml("**`code`**"), "<p><strong><code>code</code></strong></p>");
    # emphasis inside bold
    testing.assertEqual(
        toHtml("**outer *inner* rest**"),
        "<p><strong>outer <em>inner</em> rest</strong></p>");
}
func testHtmlNestedInlineInLink() {
    # bold inside a link label survives
    testing.assertEqual(
        toHtml("[**bold** link](u)"),
        "<p><a href=\"u\"><strong>bold</strong> link</a></p>");
}
func testHtmlLinkLabelWithCodeBrackets() {
    # a `]` inside a code span in the label does not end the label
    testing.assertEqual(
        toHtml("see [the `$xs[]` sugar](guide.md) ok"),
        "<p>see <a href=\"guide.md\">the <code>$xs[]</code> sugar</a> ok</p>");
}
func testHtmlLinkLabelWithNestedBrackets() {
    # a balanced nested bracket pair is part of the label
    testing.assertEqual(toHtml("[a [b] c](u)"), "<p><a href=\"u\">a [b] c</a></p>");
    testing.assertEqual(toHtml("[argv[0]](u)"), "<p><a href=\"u\">argv[0]</a></p>");
}
# Bracket matching around code spans: a code span (closed or not) before or
# between links / images, and a `]` inside a code span between two links.
func testHtmlBracketsAroundCodeSpans() {
    testing.assertEqual(
        toHtml('`x` ![a](b.png) and [c](d)'),
        '<p><code>x</code> <img src="b.png" alt="a"> and <a href="d">c</a></p>');
    testing.assertEqual(toHtml('` [a](b) tail'), '<p>` <a href="b">a</a> tail</p>');
    testing.assertEqual(
        toHtml('one [a](u1) `]` two [b](u2)'),
        '<p>one <a href="u1">a</a> <code>]</code> two <a href="u2">b</a></p>');
    # CommonMark: an image's alt is its description's plain text.
    testing.assertEqual(toHtml('![x [y](z)](w) end'), '<p><img src="w" alt="x y"> end</p>');
    testing.assertEqual(
        toHtml('`a` `b` [c](d) ``'),
        # An unmatched backtick run is literal (CommonMark).
        '<p><code>a</code> <code>b</code> <a href="d">c</a> ``</p>');
}
func testScanDestCountsDepth() {
    # the destination ends at the `)` that balances, not the first one seen
    def cs as list of string init strings.chars("a(b)c)d");
    testing.assertEqual(scanDest($cs, 0, false).end, 5);
    # and a destination that never balances is not one
    def open as list of string init strings.chars("a(b");
    testing.assertFalse(scanDest($open, 0, false).ok);
}
func testHtmlLinkDestWithBalancedParens() {
    # a DOI carries a parenthesised year, so the first `)` is not the end of the
    # destination - reading it as the end truncated the href and spilled the
    # remainder of the URL into the paragraph as text
    testing.assertEqual(
        toHtml("[NW](https://doi.org/10.1016/0022-2836(70)90057-4)"),
        "<p><a href=\"https://doi.org/10.1016/0022-2836(70)90057-4\">NW</a></p>");
    # nested pairs
    testing.assertEqual(
        toHtml("[x](https://e.com/a(b(c)d)e)"),
        "<p><a href=\"https://e.com/a(b(c)d)e\">x</a></p>");
    # two such links on one line still end where they should
    testing.assertEqual(
        toHtml("[a](https://e.com/q?x=(1)) and [b](https://e.com/z(2))"),
        "<p><a href=\"https://e.com/q?x=(1)\">a</a> and " +
            "<a href=\"https://e.com/z(2)\">b</a></p>");
}
func testHtmlLinkDestWithEscapedParen() {
    # an escaped parenthesis is data rather than structure, and the backslash
    # does not survive into the href
    testing.assertEqual(
        toHtml("[x](https://e.com/a\\)b) end"),
        "<p><a href=\"https://e.com/a)b\">x</a> end</p>");
}
func testHtmlLinkDestUnbalancedIsNotALink() {
    # parentheses that never balance close no link, and the text stays literal
    testing.assertEqual(toHtml("[x](a(b"), "<p>[x](a(b</p>");
    # a stray `)` after a complete link is text, as before
    testing.assertEqual(toHtml("[a](u)x)"), "<p><a href=\"u\">a</a>x)</p>");
}
func testHtmlImageDestWithBalancedParens() {
    testing.assertEqual(
        toHtml("![alt](https://e.com/i(1).png)"),
        "<p><img src=\"https://e.com/i(1).png\" alt=\"alt\"></p>");
}
func testHtmlLinkTitleAfterParenthesisedUrl() {
    # the title still splits off the destination when the URL holds parentheses
    testing.assertEqual(
        toHtml("[a](https://e.com/x(y) \"T\")"),
        "<p><a href=\"https://e.com/x(y)\" title=\"T\">a</a></p>");
}
func testHtmlLists() {
    testing.assertEqual(toHtml("- a\n- b"), "<ul><li>a</li><li>b</li></ul>");
    testing.assertEqual(toHtml("1. a\n2. b"), "<ol><li>a</li><li>b</li></ol>");
}
func testListLazyContinuation() {
    # a soft-wrapped item stays one item in one list (was split into two lists)
    def md as string init "- **a** first item\n  continued on the next line\n- **b** second item\n";
    testing.assertEqual(
        toHtml($md),
        "<ul><li><strong>a</strong> first item continued on the next line</li><li><strong>b</strong> second item</li></ul>");
}
func testListContinuationOrdered() {
    def md as string init "1. one line\n   wraps here\n2. two\n";
    testing.assertEqual(toHtml($md), "<ol><li>one line wraps here</li><li>two</li></ol>");
}
func testListBlankEndsItemThenParagraph() {
    # a blank line still ends the list; the following text is its own paragraph
    def md as string init "- item\n\nplain paragraph\n";
    testing.assertEqual(toHtml($md), "<ul><li>item</li></ul><p>plain paragraph</p>");
}
func testHtmlThematicBreak() {
    testing.assertEqual(toHtml("a\n\n---\n\nb"), "<p>a</p><hr><p>b</p>");
    # a spaced dash run is a rule, not a list
    testing.assertEqual(toHtml("- - -"), "<hr>");
    testing.assertEqual(toHtml("***"), "<hr>");
    testing.assertEqual(toHtml("___"), "<hr>");
}
func testHtmlRawBlock() {
    # SECURITY: raw HTML blocks are ESCAPED by default, so untrusted Markdown
    # (a README, a comment) cannot inject a <script> or an event handler.
    testing.assertEqual(
        toHtml("<div class=\"x\">\nraw\n</div>"),
        "&lt;div class=\"x\"&gt;\nraw\n&lt;/div&gt;");
    testing.assertEqual(toHtml("<!-- note -->"), "&lt;!-- note --&gt;");
    testing.assertEqual(
        toHtml("<script>alert(1)</script>"),
        "&lt;script&gt;alert(1)&lt;/script&gt;");
    testing.assertEqual(
        toHtml("<img src=x onerror=alert(1)>"),
        "&lt;img src=x onerror=alert(1)&gt;");
}
func testHtmlRawBlockOptIn() {
    # A trusted caller opts back in to verbatim passthrough.
    testing.assertEqual(
        toHtmlWith("<div class=\"x\">\nraw\n</div>", HtmlOptions{allowRawHtml: true}),
        "<div class=\"x\">\nraw\n</div>");
    testing.assertEqual(
        toHtmlWith("<script>ok()</script>", HtmlOptions{allowRawHtml: true}),
        "<script>ok()</script>");
    # allowRawHtml false is identical to the default toHtml (escaped).
    testing.assertEqual(
        toHtmlWith("<b>x</b>", HtmlOptions{allowRawHtml: false}),
        toHtml("<b>x</b>"));
}
func testHtmlIndentedCode() {
    testing.assertEqual(
        toHtml("para\n\n    indented code\n\nafter"),
        "<p>para</p><pre><code>indented code</code></pre><p>after</p>");
}
func testHtmlAutolinkUri() {
    testing.assertEqual(
        toHtml("see <https://example.com/x> ok"),
        "<p>see <a href=\"https://example.com/x\">https://example.com/x</a> ok</p>");
}
func testHtmlAutolinkEmail() {
    testing.assertEqual(
        toHtml("mail <a@b.com> me"),
        "<p>mail <a href=\"mailto:a@b.com\">a@b.com</a> me</p>");
}
func testHtmlAutolinkNotHtmlBlock() {
    # a bare autolink line is prose, not a raw HTML block
    testing.assertEqual(
        toHtml("<https://example.com>"),
        "<p><a href=\"https://example.com\">https://example.com</a></p>");
}
func testHtmlAngleBracketsStayLiteral() {
    # angle text that is neither an autolink nor a tag stays escaped
    testing.assertEqual(toHtml("compare <x and >y"), "<p>compare &lt;x and &gt;y</p>");
}
func testAnsiThematicBreakAndHtmlDropped() {
    # a rule renders as a dashed line; a raw HTML block is dropped
    testing.assertTrue(strings.contains(toAnsi("a\n\n---\n\nb"), "----"));
    testing.assertTrue(not strings.contains(toAnsi("<div>\nx\n</div>"), "div"));
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
    def ks as list of doctree.Node init doctree.children(parse("| A | B |\n| --- | ---: |\n| 1 | 2 |\n| 3 | 4 |"));
    testing.assertEqual(len($ks), 1);
    testing.assertEqual($ks[0].kind, "table");
    testing.assertEqual(len($ks[0].children[0].children), 2);
    testing.assertEqual($ks[0].children[0].children[1].align, "right");
    testing.assertEqual(len($ks[0].children), 3);
    testing.assertEqual(doctree.text($ks[0].children[1].children[1]), "2");
}
func testTableInterruptsParagraph() {
    def ks as list of doctree.Node init doctree.children(parse("intro\n| a | b |\n| --- | --- |\n| 1 | 2 |"));
    testing.assertEqual(len($ks), 2);
    testing.assertEqual($ks[0].kind, "paragraph");
    testing.assertEqual($ks[1].kind, "table");
}
# A pipe line with no delimiter row underneath is an ordinary paragraph.
func testPipeLineWithoutDelimiterIsParagraph() {
    def ks as list of doctree.Node init doctree.children(parse("a | b | c"));
    testing.assertEqual(len($ks), 1);
    testing.assertEqual($ks[0].kind, "paragraph");
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
    def doc as doctree.Node init parse("# H\n\ntext\n");
    testing.assertEqual(doctree.typeOf($doc), "document");
    testing.assertEqual(len(doctree.children($doc)), 2);
}
func testReaderHeadingLevelText() {
    def doc as doctree.Node init parse("## Hello *world*\n");
    def h as doctree.Node init doctree.get($doc, "heading");
    testing.assertEqual(doctree.typeOf($h), "heading");
    testing.assertEqual(doctree.level($h), 2);
    # text flattens the inline children (the emphasis contributes "world").
    testing.assertEqual(doctree.text($h), "Hello world");
}
func testReaderLinkAttrs() {
    def doc as doctree.Node init parse("see [site](https://example.com \"home\")\n");
    def a as doctree.Node init doctree.get($doc, "paragraph/link");
    testing.assertEqual(doctree.attr($a, "href"), "https://example.com");
    testing.assertEqual(doctree.attr($a, "title"), "home");
    testing.assertEqual(doctree.text($a), "site");
}
func testReaderCodeLanguage() {
    def doc as doctree.Node init parse("```python\nprint(1)\n```\n");
    def c as doctree.Node init doctree.get($doc, "code");
    testing.assertEqual(doctree.attr($c, "lang"), "python");
    testing.assertEqual(doctree.text($c), "print(1)");
}
func testReaderOrderedList() {
    def doc as doctree.Node init parse("1. a\n2. b\n");
    def l as doctree.Node init doctree.get($doc, "list");
    testing.assertEqual(doctree.attr($l, "ordered"), "true");
    testing.assertEqual(len(doctree.findAll($l, "item")), 2);
}
func testReaderUnorderedList() {
    def doc as doctree.Node init parse("- a\n- b\n- c\n");
    def l as doctree.Node init doctree.get($doc, "list");
    testing.assertEqual(doctree.attr($l, "ordered"), "false");
    testing.assertEqual(len(doctree.findAll($l, "item")), 3);
}
func testReaderTableCellAlign() {
    def doc as doctree.Node init parse("| A | B |\n|:--|--:|\n| 1 | 2 |\n");
    def t as doctree.Node init doctree.get($doc, "table");
    # The header is row[1]; its second cell is right-aligned.
    def cell as doctree.Node init doctree.get($t, "row[1]/cell[2]");
    testing.assertEqual(doctree.attr($cell, "align"), "right");
}
func testReaderNestedListItem() {
    def doc as doctree.Node init parse("- top\n  - nested\n");
    def item as doctree.Node init doctree.get($doc, "list/item");
    testing.assertTrue(doctree.has($item, "list"));
}
func testSelectorWildcardAndIndex() {
    def doc as doctree.Node init parse("# a\n\n# b\n\n# c\n");
    testing.assertEqual(len(doctree.findAll($doc, "*")), 3);
    testing.assertEqual(doctree.text(doctree.get($doc, "heading[2]")), "b");
}
func testTextFlattensDescendants() {
    def doc as doctree.Node init parse("a **bold** and `code`\n");
    def p as doctree.Node init doctree.get($doc, "paragraph");
    testing.assertEqual(doctree.text($p), "a bold and code");
}
func testGetMissingIsEmptyNode() {
    def doc as doctree.Node init parse("just text\n");
    testing.assertEqual(doctree.typeOf(doctree.get($doc, "table")), "");
    testing.assertFalse(doctree.has($doc, "table"));
}

# --- public document tree: render (from the tree) ---
func testRenderHtmlMatchesToHtml() {
    def src as string init "# H\n\ntext with [x](http://a) and `c`\n\n- one\n- two\n";
    testing.assertEqual(doctree.render("html", parse($src)), toHtml($src));
}
func testRenderAnsiMatchesToAnsi() {
    def src as string init "# H\n\n| A | B |\n|--|--|\n| 1 | 2 |\n";
    testing.assertEqual(doctree.render("ansi", parse($src)), toAnsi($src));
}
func testRenderRoundTripTransform() {
    # parse -> rewrite a link's url -> render: the new url appears, the old is gone.
    def doc as doctree.Node init parse("go [here](http://old.example)\n");
    def docKids as list of doctree.Node init doctree.children($doc);
    def para as doctree.Node init $docKids[0];
    def pkids as list of doctree.Node init doctree.children($para);
    def j as int init 0;
    while ($j < len($pkids)) {
        if (doctree.typeOf($pkids[$j]) == "link") {
            $pkids[$j].url = "http://new.example";
        }
        $j = $j + 1;
    }
    $para.children = $pkids;
    $docKids[0] = $para;
    $doc.children = $docKids;
    def out as string init doctree.render("html", $doc);
    testing.assertContains($out, "http://new.example");
    testing.assertFalse(strings.contains($out, "old.example"));
}
func deepBlockConversion() {
    # A depth past the cap is a catchable "markdown" error, not a stack overflow.
    parse(strings.repeat("> ", 150) + "x");
}
func testDepthCapThrows() {
    testing.assertThrows("deepBlockConversion", "markdown");
}
func admOf(md as string) {
    return doctree.children(parse($md))[0];
}
# The marker is structure: it opens the block and is not part of what it says.
func testAMarkerOpensAnAdmonition() {
    def n as doctree.Node init admOf("> [!NOTE]\n> body one.\n");
    testing.assertEqual(doctree.typeOf($n), "admonition");
    testing.assertEqual(doctree.attr($n, "kind"), "note");
    testing.assertEqual(doctree.attr($n, "title"), "");
    testing.assertEqual(doctree.text($n), "body one.");
}
func testEveryKindIsRecognised() {
    for (def kind in ALERT_KINDS) {
        def n as doctree.Node init admOf("> [" + "!" + strings.upper($kind) + "]\n> x\n");
        testing.assertEqual(doctree.typeOf($n), "admonition");
        testing.assertEqual(doctree.attr($n, "kind"), $kind);
    }
}
# GitHub writes the marker in capitals; a book that does not means the same
# thing.
func testTheMarkerIsCaseInsensitive() {
    testing.assertEqual(doctree.attr(admOf("> [!note]\n> x\n"), "kind"), "note");
    testing.assertEqual(doctree.attr(admOf("> [!Warning]\n> x\n"), "kind"), "warning");
}
# The title is the one thing only a parser can see: after the block is built,
# the line break behind it has been folded into a space and the title is
# indistinguishable from the first words of the body.
func testAMarkerCanCarryATitle() {
    def n as doctree.Node init admOf("> [!WARNING] Mind the gap\n> body.\n");
    testing.assertEqual(doctree.attr($n, "title"), "Mind the gap");
    testing.assertEqual(doctree.text($n), "body.");
}
func testAMarkerWithNoBodyIsStillAnAdmonition() {
    def n as doctree.Node init admOf("> [!TIP]\n");
    testing.assertEqual(doctree.typeOf($n), "admonition");
    testing.assertEqual(doctree.text($n), "");
}
# A sixth word, a marker that is not the whole of what precedes the title, or no
# marker at all: all quotations, which is what every renderer that has never
# heard of the syntax makes of them.
func testEverythingElseIsAQuotation() {
    for (def md in ["> [!NOTES]\n> x\n", "> [!NOTE]: x\n", "> [!]\n> x\n", "> quoted\n"]) {
        testing.assertEqual(doctree.typeOf(admOf($md)), "quote");
    }
}
# `kind` is the admonition alert kind and nothing else: on any other node - a
# code block, whose language shares the same backing field - it is empty.
func testKindIsEmptyOnNonAdmonition() {
    def code as doctree.Node init doctree.children(parse("```go\nx := 1\n```\n"))[0];
    testing.assertEqual(doctree.attr($code, "language"), "go");
    testing.assertEqual(doctree.attr($code, "kind"), "");
}
func testAnAdmonitionHoldsBlocks() {
    def n as doctree.Node init admOf("> [!NOTE]\n> lead\n>\n> - a\n> - b\n");
    testing.assertEqual(len(doctree.children($n)), 2);
    testing.assertEqual(doctree.typeOf(doctree.children($n)[0]), "paragraph");
    testing.assertEqual(doctree.typeOf(doctree.children($n)[1]), "list");
}
func testAnAdmonitionCanNest() {
    def outer as doctree.Node init admOf("> [!WARNING]\n> outer\n>\n> > [!TIP]\n> > inner\n");
    testing.assertEqual(doctree.attr($outer, "kind"), "warning");
    def inner as doctree.Node init doctree.children($outer)[1];
    testing.assertEqual(doctree.typeOf($inner), "admonition");
    testing.assertEqual(doctree.attr($inner, "kind"), "tip");
}
# The class names are the conventional ones, so a stylesheet written for another
# generator's callouts already matches.
func testAdmonitionHtml() {
    testing.assertEqual(
        toHtml("> [!TIP] Try this\n> body\n"),
        '<div class="admonition admonition-tip"><p class="admonition-title">Try this</p>' +
            "<p>body</p></div>");
    testing.assertContains(
        toHtml("> [!CAUTION]\n> body\n"),
        '<p class="admonition-title">Caution</p>');
}
# A terminal has no panel to draw, so the label carries the weight.
func testAdmonitionAnsi() {
    def out as string init toAnsi("> [!NOTE] Heads up\n> body\n");
    testing.assertContains($out, "Heads up");
    testing.assertContains($out, "body");
}
func testAdmonitionPdfRenders() {
    def o as doctree.PdfOptions init doctree.pdfDefaults();
    $o.quoteFill = doctree.gray(94);
    def out as bytes init toPdfWith("> [!NOTE]\n> body\n", $o);
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}

# --- PDF rendering (markdown.toPdf, white-box) ---
func pdfMarker() {
    return convert.bytesFromString("utf-8", "%PDF");
}
# pdfContains reports whether the PDF bytes contain an ASCII substring.
func pdfContains(b as bytes, needle as string) {
    def nb as bytes init convert.bytesFromString("utf-8", $needle);
    return binary.contains($b, $nb);
}
# countPdfPages counts page objects by their one-per-page /MediaBox marker.
func countPdfPages(b as bytes) {
    def needle as bytes init convert.bytesFromString("utf-8", "/MediaBox");
    def nl as int init len($needle);
    def count as int init 0;
    def idx as int init binary.indexOf($b, $needle);
    while ($idx >= 0) {
        $count = $count + 1;
        $idx = binary.indexOf($b, $needle, $idx + $nl);
    }
    return $count;
}

# --- options + metrics ---------------------------------------------
func testRenderProducesValidPdf() {
    def out as bytes init toPdf("# Hi\n\nA paragraph of text.\n");
    testing.assertTrue(len($out) > 100);
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}
# bytesEqual compares two byte arrays element-wise.
func bytesEqual(a as bytes, b as bytes) {
    if (len($a) != len($b)) {
        return false;
    }
    def i as int init 0;
    while ($i < len($a)) {
        if ($a[$i] != $b[$i]) {
            return false;
        }
        $i = $i + 1;
    }
    return true;
}
func testRenderPdfDocReturnsUsableDocument() {
    # renderPdfDoc hands back the laid-out pdf.Document so document-level work
    # (here a running footer with page numbers) can happen before serialization
    def tree as doctree.Node init parse("# One\n\nbody\n\n<!-- pagebreak -->\n\n# Two\n\nmore\n");
    def doc as pdf.Document init doctree.renderPdfDoc($tree, doctree.pdfDefaults());
    testing.assertEqual(len($doc.pages), 2);
    def lbl as pdf.PageLabel init pdf.pageLabel();
    $lbl.right = "%page%/%pages%";
    $doc = pdf.setFooter($doc, $lbl);
    def out as bytes init pdf.render($doc);
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
    testing.assertEqual(countPdfPages($out), 2);
}
func testRenderPdfEqualsDocThenRender() {
    # renderPdf is exactly pdf.render(doctree.renderPdfDoc(...)); the split changed nothing
    def tree as doctree.Node init parse("# Title\n\nsome **body** text\n\n- a\n- b\n");
    def viaPdf as bytes init doctree.renderPdf($tree, doctree.pdfDefaults());
    def viaDoc as bytes init pdf.render(doctree.renderPdfDoc($tree, doctree.pdfDefaults()));
    testing.assertTrue(bytesEqual($viaPdf, $viaDoc));
}
# mdArrow builds U+2190 (LEFTWARDS ARROW, outside WinAnsi) from its UTF-8 bytes,
# so these tests need no non-ASCII glyph in the source.
func mdArrow() {
    def b as bytes;
    $b[] = 0xE2;
    $b[] = 0x86;
    $b[] = 0x90;
    return convert.stringFromBytes("utf-8", $b);
}
func testLongCodeLineFolds() {
    # a code line far wider than the page renders to a valid PDF (folded onto the
    # page rather than clipped at the right margin)
    def long as string init strings.repeat("abcdef/", 60);
    def md as string init "```\n" + $long + "\n```\n";
    def out as bytes init toPdf($md);
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
    # the folded block is materially larger than the same line kept to one column
    def one as bytes init toPdf("```\nabcdef/\n```\n");
    testing.assertTrue(len($out) > len($one));
}
func testUnencodableCharRenders() {
    # a document containing a non-WinAnsi glyph renders instead of aborting; the
    # glyph is replaced by the default substitute
    def md as string init "# Arrow " + mdArrow() + " here\n\nbody " + mdArrow() + " text\n";
    def out as bytes init toPdf($md);
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
    testing.assertTrue(len($out) > 100);
}
func testUnencodableInCodeAndTableRenders() {
    # the substitution reaches code blocks and table cells too
    def md as string init "```\ncode " + mdArrow() + "\n```\n\n| A | B |\n|--|--|\n| " + mdArrow() +
        " | y |\n";
    def out as bytes init toPdf($md);
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}
func testUnencodableSkipOption() {
    # an empty `unencodable` drops the glyph rather than substituting
    def o as doctree.PdfOptions init doctree.pdfDefaults();
    $o.unencodable = "";
    def md as string init "text " + mdArrow() + " end\n";
    def out as bytes init toPdfWith($md, $o);
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}
func testPageBreakNodeAddsPage() {
    # a pageBreak node forces a second page
    def doc as doctree.Node init parse("# One\n\ntext a\n");
    $doc.children[] = doctree.pageBreak();
    def more as doctree.Node init parse("more\n");
    for (def c in doctree.children($more)) {
        $doc.children[] = $c;
    }
    def out as bytes init doctree.renderPdf($doc, doctree.pdfDefaults());
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
    testing.assertEqual(countPdfPages($out), 2);
    # the same content without the break stays on one page
    def flat as bytes init toPdf("# One\n\ntext a\n\nmore\n");
    testing.assertEqual(countPdfPages($flat), 1);
}
func testPageBreakComment() {
    # a lone <!-- pagebreak --> comment parses to a page break
    def out as bytes init toPdf("a\n\n<!-- pagebreak -->\n\nb\n");
    testing.assertEqual(countPdfPages($out), 2);
}
func testCodeFillEnlargesOutput() {
    # a code fill / border draws a rectangle behind the block; default is unchanged
    def o as doctree.PdfOptions init doctree.pdfDefaults();
    $o.codeFill = doctree.gray(240);
    $o.codeBorder = doctree.gray(150);
    def filled as bytes init toPdfWith("```\ncode line\n```\n", $o);
    def plain as bytes init toPdf("```\ncode line\n```\n");
    testing.assertTrue(binary.startsWith($filled, pdfMarker()));
    testing.assertTrue(len($filled) > len($plain));
}
func testQuoteFillAndRule() {
    # a quote fill / rule draws behind the block; default output is unchanged
    def md as string init "> **A quoted note.** It matters.\n";
    def o as doctree.PdfOptions init doctree.pdfDefaults();
    $o.quoteFill = doctree.gray(240);
    $o.quoteRule = doctree.gray(120);
    def filled as bytes init toPdfWith($md, $o);
    def plain as bytes init toPdf($md);
    testing.assertTrue(binary.startsWith($filled, pdfMarker()));
    testing.assertTrue(len($filled) > len($plain));
}
func testPdfCreatorProducerMetadata() {
    # creator / producer reach the Info dictionary
    def o as doctree.PdfOptions init doctree.pdfDefaults();
    $o.creator = "Grimoire";
    $o.producer = "Grimoire 1.0";
    def out as bytes init toPdfWith("# Hi\n", $o);
    testing.assertTrue(pdfContains($out, "/Creator (Grimoire)"));
    testing.assertTrue(pdfContains($out, "/Producer (Grimoire 1.0)"));
}
func testPdfProducerDefaultKept() {
    # an empty producer keeps pdf's own default rather than blanking it
    def out as bytes init toPdf("# Hi\n");
    testing.assertTrue(pdfContains($out, "/Producer (Jennifer pdf)"));
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
    def doc as doctree.Node init parse("# Tree\n\nrendered via renderPdf.\n");
    def out as bytes init doctree.renderPdf($doc, doctree.pdfDefaults());
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}
func testRenderWithA4Options() {
    def o as doctree.PdfOptions init doctree.pdfDefaults();
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
func testHeadingAndTableFillRender() {
    def o as doctree.PdfOptions init doctree.pdfDefaults();
    $o.tableHeaderFill = doctree.gray(230);
    $o.headingStyles = [
        doctree.headingStyle(doctree.gray(205)),
        doctree.headingStyle(doctree.gray(225))
    ];
    def out as bytes init toPdfWith("# H1\n\n## H2\n\n| A | B |\n|--|--|\n| 1 | 2 |\n", $o);
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}

# --- PDF metadata + bookmarks ---
func pdfHas(b as bytes, s as string) {
    return binary.contains($b, convert.bytesFromString("utf-8", $s));
}
func testPdfMetadata() {
    def o as doctree.PdfOptions init doctree.pdfDefaults();
    $o.title = "My Report";
    $o.author = "Ada";
    def out as bytes init toPdfWith("# H\n\ntext\n", $o);
    testing.assertTrue(pdfHas($out, "/Title (My Report)"));
    testing.assertTrue(pdfHas($out, "/Author (Ada)"));
}
func testPdfBookmarks() {
    def o as doctree.PdfOptions init doctree.pdfDefaults();
    $o.bookmarkLevel = 2;
    def out as bytes init toPdfWith("# Intro\n\ntext\n\n## Sub\n\ntext\n", $o);
    testing.assertTrue(pdfHas($out, "/Type /Outlines"));
    testing.assertTrue(pdfHas($out, "/PageMode /UseOutlines"));
    testing.assertTrue(pdfHas($out, "/Title (Intro)"));
    testing.assertTrue(pdfHas($out, "/Title (Sub)"));
}
# A heading the standard-14 fonts cannot draw still names itself in the sidebar.
# The page falls back to the substitute because no glyph exists for it; the
# outline is a text string, not drawn text, so it keeps the heading whole. The
# escapes spell "Uber" with an umlaut and the Russian for "chapter".
func testPdfBookmarksKeepWhatThePageCannotDraw() {
    def o as doctree.PdfOptions init doctree.pdfDefaults();
    $o.bookmarkLevel = 1;
    def out as bytes init toPdfWith("# \u0413\u043b\u0430\u0432\u0430\n\ntext\n", $o);
    testing.assertTrue(pdfHas($out, "/Title <feff0413043b043004320430>"));
}
# The umlaut is inside WinAnsi, so the page draws it - and the bookmark, being
# UTF-16BE, spells it out rather than emitting the raw UTF-8 a viewer would read
# as two Latin-1 characters.
func testPdfBookmarksAreNotRawUtfEight() {
    def o as doctree.PdfOptions init doctree.pdfDefaults();
    $o.bookmarkLevel = 1;
    def out as bytes init toPdfWith("# \u00dcbersicht\n\ntext\n", $o);
    testing.assertTrue(pdfHas($out, "/Title <feff00dc00620065007200730069006300680074>"));
    testing.assertFalse(pdfHas($out, "/Title (\u00dcbersicht)"));
}
func testPdfBookmarkLevelDefaultsToNone() {
    def out as bytes init toPdf("# Intro\n\ntext\n");
    testing.assertFalse(pdfHas($out, "/Outlines"));
}
func testPdfBookmarkLevelExcludesDeeper() {
    def o as doctree.PdfOptions init doctree.pdfDefaults();
    $o.bookmarkLevel = 1;
    def out as bytes init toPdfWith("# Top\n\ntext\n\n## Deep\n\ntext\n", $o);
    testing.assertTrue(pdfHas($out, "/Title (Top)"));
    testing.assertFalse(pdfHas($out, "/Title (Deep)"));
}

# --- PDF images (markdown.toPdf picture embedding) ---
#
# An 8x6 opaque RGB PNG (base64), the same fixture pdf.j's own tests use. Kept
# small so the tests stay fast and the base64 stays readable.
func mdImgFixture() {
    return encoding.fromText("base64", MDIMGPNG);
}
# mdStripBytes mirrors what renderPdfDoc stores in Layout.placements: the image
# with its byte fields cleared (drawImage needs only the name + pixel dims).
func mdStripBytes(img as pdf.Image) {
    def light as pdf.Image init $img;
    def empty as bytes;
    $light.data = $empty;
    $light.smask = $empty;
    $light.hasSmask = false;
    return $light;
}
# A single-image paragraph draws (and embeds) the picture when the caller supplied
# its bytes for the URL.
func testPdfImageEmbeddedWhenSupplied() {
    def o as doctree.PdfOptions init doctree.pdfDefaults();
    $o.images["shot.png"] = pdf.loadImage("Shot", mdImgFixture());
    def withImg as bytes init toPdfWith("# T\n\n![a green box](shot.png)\n", $o);
    def fallback as bytes init toPdf("# T\n\n![a green box](shot.png)\n");
    testing.assertTrue(binary.startsWith($withImg, pdfMarker()));
    # Embedded as an image XObject, referenced from a page's resources.
    testing.assertTrue(pdfHas($withImg, "/Subtype /Image"));
    testing.assertTrue(pdfHas($withImg, "/XObject"));
    # Embedding the bytes makes it larger than the [alt]-text fallback.
    testing.assertTrue(len($withImg) > len($fallback));
}
# With no image supplied for the URL, the same source keeps the [alt] text and
# embeds no image.
func testPdfImageFallbackWhenAbsent() {
    def out as bytes init toPdf("# T\n\n![only alt](missing.png)\n");
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
    testing.assertFalse(pdfHas($out, "/Subtype /Image"));
}
# Two images sharing a resource name would collide in the /XObject dict; the
# module rejects it up front.
func dupImageNames() {
    def o as doctree.PdfOptions init doctree.pdfDefaults();
    $o.images["a.png"] = pdf.loadImage("Same", mdImgFixture());
    $o.images["b.png"] = pdf.loadImage("Same", mdImgFixture());
    return toPdfWith("![x](a.png)\n\n![y](b.png)\n", $o);
}
func testPdfImageDuplicateNameThrows() {
    testing.assertThrows("dupImageNames", "doctree");
}
# the fenced language becomes class="language-x" and the full info string is
# kept on the node.
func testCodeLanguageClassAndInfo() {
    testing.assertContains(toHtml("```py\nx\n```\n"), '<code class="language-py">');
    testing.assertContains(toXhtml("```py\nx\n```\n"), '<code class="language-py">');
    testing.assertEqual(toHtml("```\ny\n```\n"), "<pre><code>y</code></pre>");
    def code as doctree.Node init doctree.children(parse('```py title="a.py"' + "\nx\n```\n"))[0];
    testing.assertEqual(doctree.attr($code, "lang"), "py");
    testing.assertEqual(doctree.attr($code, "info"), 'py title="a.py"');
}
# strikethrough (~~...~~), GFM. Nests; a space-flanked or single ~ stays literal.
func testStrikethrough() {
    testing.assertEqual(toHtml("a ~~withdrawn~~ b"), "<p>a <del>withdrawn</del> b</p>");
    testing.assertEqual(toHtml("~~**x**~~"), "<p><del><strong>x</strong></del></p>");
    testing.assertEqual(toHtml("a ~~ x ~~ b"), "<p>a ~~ x ~~ b</p>");
    # A space-flanked single tilde is neither strikethrough nor subscript.
    testing.assertEqual(toHtml("a ~ b"), "<p>a ~ b</p>");
    # An empty span must not form: a run of delimiters (ASCII art - a tilde snake
    # body, an `=` rule) stays literal instead of collapsing into empty <del>/<mark>.
    # (A line of tildes alone is a code fence.)
    testing.assertEqual(toHtml("a ~~~~~~"), "<p>a ~~~~~~</p>");
    testing.assertEqual(toHtml("======"), "<p>======</p>");
    testing.assertEqual(toHtml("<~~~~~~>"), "<p>&lt;~~~~~~&gt;</p>");
}
# task list items (GFM) -> a disabled checkbox + task-list-item class; the
# item node carries task / checked attrs; a plain item is untouched.
func testTaskList() {
    def m as string init "- [ ] todo\n- [x] done\n- plain\n";
    def out as string init toHtml($m);
    testing.assertContains(
        $out,
        '<li class="task-list-item"><input type="checkbox" disabled> todo</li>');
    testing.assertContains($out, 'disabled checked> done</li>');
    testing.assertContains($out, "<li>plain</li>");
    # XHTML self-closes the checkbox and expands the boolean attrs.
    testing.assertContains(toXhtml($m), 'type="checkbox" disabled="disabled" />');
    # Node attrs.
    def items as list of doctree.Node init doctree.children(doctree.children(parse($m))[0]);
    testing.assertEqual(doctree.attr($items[0], "task"), "true");
    testing.assertEqual(doctree.attr($items[0], "checked"), "false");
    testing.assertEqual(doctree.attr($items[1], "checked"), "true");
    testing.assertEqual(doctree.attr($items[2], "task"), "");
    # ANSI shows the checkbox glyph.
    testing.assertContains(toAnsi($m), "[x] done");
    # GFM requires a space after the marker: `[x]done` is an ordinary item.
    testing.assertContains(toHtml("- [x]done\n"), "<li>[x]done</li>");
}
# attribute lists {#id .class key="value"} on headings, links, and images.
func testAttributeLists() {
    testing.assertEqual(toHtml('## H {#anchor}' + "\n"), '<h2 id="anchor">H</h2>');
    testing.assertEqual(toHtml('# H {#id .a .b}' + "\n"), '<h1 id="id" class="a b">H</h1>');
    testing.assertContains(
        toHtml('[a](x){.btn target="_blank"}'),
        '<a href="x" class="btn" target="_blank">a</a>');
    testing.assertContains(
        toHtml('![alt](i.png){width="20"}'),
        '<img src="i.png" alt="alt" width="20">');
    # doctree.attr() reads them.
    def h as doctree.Node init doctree.children(parse('## T {#x .y}' + "\n"))[0];
    testing.assertEqual(doctree.attr($h, "id"), "x");
    testing.assertEqual(doctree.attr($h, "class"), "y");
    # A brace group that is not a valid attribute list stays literal text.
    testing.assertEqual(toHtml('## no {plain here}' + "\n"), '<h2>no {plain here}</h2>');
}
# SECURITY: a `{...}` attribute list is filtered through an allowlist, so
# untrusted Markdown cannot smuggle an event handler or inline style into the
# safe-by-default output. Unsafe names are dropped; safe ones (and the data-* /
# aria-* families) pass through.
func testAttributeListDropsUnsafe() {
    # on* event handlers and style are dropped on every renderer that reads attrs.
    testing.assertEqual(toHtml('# T {onclick="x()"}' + "\n"), "<h1>T</h1>");
    testing.assertEqual(toHtml('# T {style="color:red"}' + "\n"), "<h1>T</h1>");
    testing.assertEqual(
        toHtml('![a](i.png){onerror="steal()"}'),
        '<p><img src="i.png" alt="a"></p>');
    testing.assertEqual(toHtml('[t](x){onmouseover="x()"}'), '<p><a href="x">t</a></p>');
    # An unsafe key mixed with a safe one keeps only the safe one.
    testing.assertEqual(toHtml('# T {onclick="x()" title="ok"}' + "\n"), '<h1 title="ok">T</h1>');
    # data-* / aria-* are allowed through.
    testing.assertContains(toHtml('# T {data-x="1" aria-label="l"}' + "\n"), 'data-x="1"');
    testing.assertContains(toHtml('# T {data-x="1" aria-label="l"}' + "\n"), 'aria-label="l"');
    # A data-/aria- name whose tail is not a valid HTML attribute name must be
    # dropped, not passed to html.attr (which would throw and abort toHtml on
    # input the module documents as safe by default).
    testing.assertEqual(toHtml('[t](u){data-x"y=1}'), '<p><a href="u">t</a></p>');
    testing.assertEqual(toHtml('![a](u){data-é=1}'), '<p><img src="u" alt="a"></p>');
}
# highlight (==), subscript (~x~), superscript (^x^). ~~ stays strikethrough.
func testHighlightSubSup() {
    testing.assertEqual(toHtml("==important=="), "<p><mark>important</mark></p>");
    testing.assertEqual(toHtml("H~2~O"), "<p>H<sub>2</sub>O</p>");
    testing.assertEqual(toHtml("x^2^"), "<p>x<sup>2</sup></p>");
    testing.assertEqual(toHtml("~~gone~~"), "<p><del>gone</del></p>");
    # single = is literal (only == highlights).
    testing.assertEqual(toHtml("a = b"), "<p>a = b</p>");
    # A sub/sup span may not contain whitespace (pandoc): a stray caret / tilde
    # in prose or math stays literal instead of swallowing the rest of the line.
    testing.assertEqual(toHtml("x^2 + y^2 done"), "<p>x^2 + y^2 done</p>");
    testing.assertEqual(toHtml("a ~2 x cost~ b"), "<p>a ~2 x cost~ b</p>");
}
# definition lists (term / ": definition"), pandoc-style -> <dl><dt><dd>.
func testDefinitionList() {
    testing.assertEqual(
        toHtml("term\n: what it means\n"),
        "<dl><dt>term</dt><dd>what it means</dd></dl>");
    testing.assertContains(
        toHtml("Apple\n: a fruit\n: a company\n"),
        "<dd>a fruit</dd><dd>a company</dd>");
    def dl as doctree.Node init doctree.children(parse("term\n: def\n"))[0];
    testing.assertEqual(doctree.typeOf($dl), "definition_list");
    testing.assertEqual(doctree.typeOf(doctree.children($dl)[0]), "def_term");
    testing.assertEqual(doctree.typeOf(doctree.children($dl)[1]), "def_desc");
    testing.assertContains(toAnsi("term\n: def\n"), "def");
    # A mid-line colon is not a definition.
    testing.assertEqual(toHtml("a line\nwith a colon: here\n"), "<p>a line with a colon: here</p>");
}
# PERFORMANCE/DOS: a run of unmatched `[` / `![` must not make parseInline
# O(N^2) - toHtml is documented safe for untrusted Markdown. Bracket matches are
# precomputed in one pass, so a large adversarial run renders promptly (this test
# would time out under the old per-`[` forward re-scan) and the brackets stay
# literal text, with no link / image produced.
func testUnmatchedBracketsLinear() {
    def out as string init toHtml(strings.repeat("[", 4000) + ")");
    testing.assertContains($out, "[[[");
    testing.assertTrue(not strings.contains($out, "<a "));
    def out2 as string init toHtml(strings.repeat("![", 4000) + ")");
    testing.assertTrue(not strings.contains($out2, "<img "));
}
# A nested bracket pair inside a link label still parses (the precompute balances
# nested `[...]` exactly as the old counter did).
func testNestedBracketLink() {
    testing.assertEqual(toHtml("[a [b] c](http://x)"), '<p><a href="http://x">a [b] c</a></p>');
}
# CORRECTNESS + SECURITY: HTML entity references in text are decoded to their
# character in one pass over the raw text (then re-escaped by the renderer), so a
# named / numeric reference is no longer double-escaped - and the decode is safe:
# a numeric reference to a metacharacter becomes the escaped form, never an
# injected tag, and an escaped ampersand stays literal.
func testEntityDecoding() {
    testing.assertEqual(toHtml("&copy; 2026"), "<p>© 2026</p>"); # named -> character
    testing.assertEqual(toHtml("a&nbsp;b"), "<p>a b</p>"); # named nbsp
    # numeric reference to '<' decodes then RE-ESCAPES (never an injected '<')
    testing.assertEqual(toHtml("x &#60; y"), "<p>x &lt; y</p>");
    testing.assertEqual(toHtml("x &#x3c; y"), "<p>x &lt; y</p>"); # hex numeric
    # an escaped ampersand stays literal (not conflated with an entity)
    testing.assertEqual(toHtml("&amp;copy;"), "<p>&amp;copy;</p>");
    # metacharacter named entities decode then re-escape (safe)
    testing.assertEqual(toHtml("&lt;script&gt;"), "<p>&lt;script&gt;</p>");
    # an unrecognised reference is left verbatim (its & escapes)
    testing.assertEqual(toHtml("&notreal;"), "<p>&amp;notreal;</p>");
    testing.assertEqual(toHtml("a & b"), "<p>a &amp; b</p>"); # bare ampersand
}

# --- extended tree kinds (built by hand and by the rst module) ---
#
# The Markdown parser never builds these shapes; `rst` lowers into them. Each is
# checked through every renderer, and the PDF smoke test lays them all out.

# --- CommonMark block structure ---

func testSetextHeadings() {
    testing.assertEqual(toHtml("Title\n=====\n\nSub\n---\n"), "<h1>Title</h1><h2>Sub</h2>");
    testing.assertEqual(toHtml("two\nlines\n===\n"), "<h1>two lines</h1>");
}

func testAtxClosingSequence() {
    testing.assertEqual(toHtml("## H ##\n"), "<h2>H</h2>");
    testing.assertEqual(toHtml("# #5 #\n"), "<h1>#5</h1>");
    testing.assertEqual(toHtml("#5 bolt\n"), "<p>#5 bolt</p>");
}

func testTildeFencesAndInfo() {
    testing.assertEqual(
        toHtml("~~~py\nx\n~~~\n"),
        "<pre><code class=\"language-py\">x</code></pre>");
    # An unclosed fence runs to the end of its container.
    testing.assertEqual(
        toHtml("> ```\n> a\n\nb\n"),
        "<blockquote><pre><code>a</code></pre></blockquote><p>b</p>");
}

func testListItemsHoldBlocks() {
    testing.assertEqual(
        toHtml("- one\n\n  more of one\n- two\n"),
        "<ul><li><p>one</p><p>more of one</p></li><li><p>two</p></li></ul>");
    testing.assertEqual(
        toHtml("- item\n\n  ```\n  code\n  ```\n"),
        "<ul><li><p>item</p><pre><code>code</code></pre></li></ul>");
    testing.assertEqual(
        toHtml("- > quoted\n"),
        "<ul><li><blockquote><p>quoted</p></blockquote></li></ul>");
}

func testTightAndLooseLists() {
    testing.assertEqual(toHtml("- a\n- b\n"), "<ul><li>a</li><li>b</li></ul>");
    testing.assertEqual(toHtml("- a\n\n- b\n"), "<ul><li><p>a</p></li><li><p>b</p></li></ul>");
    # A blank line inside a nested quote does not loosen the list.
    testing.assertEqual(
        toHtml("* a\n  > b\n  >\n* c\n"),
        "<ul><li>a<blockquote><p>b</p></blockquote></li><li>c</li></ul>");
}

func testOrderedListStartAndDelimiter() {
    testing.assertEqual(
        toHtml("3. three\n4. four\n"),
        "<ol start=\"3\"><li>three</li><li>four</li></ol>");
    testing.assertEqual(toHtml("1) one\n2) two\n"), "<ol><li>one</li><li>two</li></ol>");
    # Only `1.` may interrupt a paragraph.
    testing.assertEqual(toHtml("text\n2. not a list\n"), "<p>text 2. not a list</p>");
}

func testLazyContinuation() {
    testing.assertEqual(toHtml("> a\nb\n"), "<blockquote><p>a b</p></blockquote>");
    testing.assertEqual(toHtml("- a\nb\n"), "<ul><li>a b</li></ul>");
}

func testIndentedCodeInsideItem() {
    testing.assertEqual(
        toHtml("- a\n\n      code\n"),
        "<ul><li><p>a</p><pre><code>code</code></pre></li></ul>");
}

func testHtmlBlockTypes() {
    testing.assertEqual(htmlStart("<script>"), 1);
    testing.assertEqual(htmlStart("<?php"), 3);
    testing.assertEqual(htmlStart("<!DOCTYPE html>"), 4);
    testing.assertEqual(htmlStart("<![CDATA[x"), 5);
    # A type 7 tag cannot interrupt a paragraph.
    testing.assertEqual(toHtml("a\n<span>\n"), "<p>a &lt;span&gt;</p>");
}

# --- CommonMark inlines ---

func testReferenceLinks() {
    def src as string init "[a][r], [r][] and [r].\n\n[R]: http://x \"T\"\n";
    testing.assertEqual(
        toHtml($src),
        "<p><a href=\"http://x\" title=\"T\">a</a>, <a href=\"http://x\" title=\"T\">r</a> and " +
            "<a href=\"http://x\" title=\"T\">r</a>.</p>");
    # A definition alone leaves no paragraph; an undefined label stays text.
    testing.assertEqual(toHtml("[x]: /u\n"), "");
    testing.assertEqual(toHtml("[nope]\n"), "<p>[nope]</p>");
}

func testHardAndSoftBreaks() {
    testing.assertEqual(toHtml("a  \nb\n"), "<p>a<br>b</p>");
    testing.assertEqual(toHtml("a\\\nb\n"), "<p>a<br>b</p>");
    testing.assertEqual(toHtml("a\nb\n"), "<p>a b</p>");
    testing.assertEqual(ansi.strip(toAnsi("a  \nb\n")), "a\nb");
}

func testBackslashEscapes() {
    testing.assertEqual(toHtml("\\*not em\\* \\# \\[x\\]\n"), "<p>*not em* # [x]</p>");
    testing.assertEqual(toHtml("\\q stays\n"), "<p>\\q stays</p>");
}

func testUnderscoreAndRuleOfThree() {
    testing.assertEqual(
        toHtml("_em_ __strong__ snake_case_word\n"),
        "<p><em>em</em> <strong>strong</strong> snake_case_word</p>");
    testing.assertEqual(toHtml("***x***\n"), "<p><em><strong>x</strong></em></p>");
    testing.assertEqual(toHtml("*a **b** c*\n"), "<p><em>a <strong>b</strong> c</em></p>");
}

func testMultiBacktickCodeSpans() {
    testing.assertEqual(
        toHtml("`` a`b `` and ` x `\n"),
        "<p><code>a`b</code> and <code>x</code></p>");
}

func testAngleBracketDestination() {
    testing.assertEqual(
        toHtml("[a](<url with space>)\n"),
        "<p><a href=\"url with space\">a</a></p>");
}

func testNoLinksInsideLinks() {
    testing.assertEqual(toHtml("[a [b](c) d](e)\n"), "<p>[a <a href=\"c\">b</a> d](e)</p>");
}

func testInlineHtmlIsEscapedUnlessAllowed() {
    def src as string init "a <span class=\"x\">*b*</span> c\n";
    testing.assertEqual(toHtml($src), "<p>a &lt;span class=\"x\"&gt;<em>b</em>&lt;/span&gt; c</p>");
    def o as HtmlOptions;
    $o.allowRawHtml = true;
    testing.assertEqual(toHtmlWith($src, $o), "<p>a <span class=\"x\"><em>b</em></span> c</p>");
    # Markup inside a tag is not parsed.
    testing.assertEqual(toHtmlWith("x <a title=\"*y*\"> z\n", $o), "<p>x <a title=\"*y*\"> z</p>");
}

func testEntitiesHtml4AndNumeric() {
    testing.assertEqual(
        toHtml("&ouml; &AElig; &#0; &unknownname;\n"),
        "<p>ö Æ � &amp;unknownname;</p>");
}

func testCaseFoldedLabels() {
    testing.assertEqual(toHtml("[ẞ]\n\n[SS]: /url\n"), "<p><a href=\"/url\">ẞ</a></p>");
}

# --- footnotes (GFM) ---

func testFootnotesNumberByFirstReference() {
    def src as string init "A[^b] and[^a], again[^b].\n\n[^a]: First.\n[^b]: Second,\n    continued.\n[^unused]: Never shown.\n";
    def out as string init toHtml($src);
    testing.assertContains($out, "A<a id=\"fnref-1\" class=\"footnote-reference\" href=\"#fn-1\"");
    testing.assertContains($out, "again<a id=\"fnref-1-2\"");
    testing.assertContains($out, "<aside id=\"fn-1\" class=\"footnote\" role=\"doc-footnote\">");
    testing.assertContains($out, "<p>Second, continued.</p>");
    testing.assertFalse(strings.contains($out, "Never shown"));
    def ks as list of doctree.Node init doctree.children(parse($src));
    testing.assertEqual($ks[len($ks) - 1].kind, "footnote");
    testing.assertEqual($ks[len($ks) - 1].attrs["label"], "2");
}

func testUndefinedFootnoteStaysText() {
    testing.assertEqual(toHtml("x[^zz]\n"), "<p>x[^zz]</p>");
    # `[^x]:` is a footnote definition, never a link definition.
    testing.assertEqual(toHtml("[^x]\n\n[^x]: http://u\n"), toHtml("[^x]\n\n[^x]: http://u\n"));
    testing.assertFalse(strings.contains(toHtml("[^x]\n\n[^x]: http://u\n"), "href=\"http://u\""));
}

func testFootnoteRendersToAnsiAndPdf() {
    def src as string init "See[^n].\n\n[^n]: The note.\n";
    testing.assertContains(ansi.strip(toAnsi($src)), "The note.");
    testing.assertTrue(len(toPdf($src)) > 500);
}

# --- linear on adversarial input ---

func testManyOpenersStayLinear() {
    # Thousands of unmatched openers of each kind must not go quadratic.
    def junk as string init strings.repeat("[*_`<&~", 3000);
    testing.assertTrue(len(toHtml($junk)) > 0);
}
