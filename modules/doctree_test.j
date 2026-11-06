# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# doctree_test.j - white-box tests for doctree.j. Run with:
#
#     jennifer test modules/doctree_test.j
#
# The overlay splices doctree.j in front of this file, so the tests reach its
# private helpers (nodeOf, gridRows, blockify, the PDF layout) by bare
# identifier. Trees are built by hand: importing a reader here would load a
# second doctree with its own Node type. Reading and rendering together are
# tested end to end by markdown_test.j and rst_test.j. Terminal output is
# compared with styling removed (`ansi.strip`, reachable through the spliced
# import).
use testing;
use binary;

# An 8x6 PNG, the image fixture shared with markdown_test.j.
def const MDIMGPNG as string init "iVBORw0KGgoAAAANSUhEUgAAAAgAAAAGCAIAAABxZ0isAAAAGklEQVR42mNk" +
    "YGBQYBDBRCwMGiIMDFgQPSQAwgMFakXvt1IAAAAASUVORK5CYII=";

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
func renderBadFormat() {
    render(tDoc([tPara("x")]), "pdf");
}
func testRenderUnknownFormatThrows() {
    testing.assertThrows("renderBadFormat", "doctree");
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

# --- admonitions -----------------------------------------------------
func pdfMarker() {
    return convert.bytesFromString("%PDF", "utf-8");
}
# pdfContains reports whether the PDF bytes contain an ASCII substring.
func pdfContains(b as bytes, needle as string) {
    def nb as bytes init convert.bytesFromString($needle, "utf-8");
    return binary.contains($b, $nb);
}
# countPdfPages counts page objects by their one-per-page /MediaBox marker.
func countPdfPages(b as bytes) {
    def needle as bytes init convert.bytesFromString("/MediaBox", "utf-8");
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
func testPackLinesRespectsWidth() {
    def o as PdfOptions init pdfDefaults();
    def words as list of IWord init [
        IWord{text: "aaaa", font: "Helvetica", width: 0.0},
        IWord{text: "bbbb", font: "Helvetica", width: 0.0},
        IWord{text: "cccc", font: "Helvetica", width: 0.0}
    ];
    # A wide column fits all three on one line.
    testing.assertEqual(len(packLines($words, 11, 500, $o)), 1);
    # A tiny column forces one word per line.
    testing.assertEqual(len(packLines($words, 11, 5, $o)), 3);
}

# --- pagination mechanics ------------------------------------------
func testNeedsBreakWhenFull() {
    def s as Layout init newLayout(pdfDefaults());
    # Drive the pen near the bottom margin, then demand more than fits.
    $s.y = 77;
    testing.assertTrue(needsBreak($s, 50));
    $s = flushPage($s);
    # A flush resets the pen to the top of a fresh page (pageHeight - margin).
    testing.assertEqual($s.y, 738);
}
func testNoBreakWhenRoom() {
    def s as Layout init newLayout(pdfDefaults());
    testing.assertFalse(needsBreak($s, 20));
    # A fresh page never breaks, even for an over-tall line.
    testing.assertFalse(needsBreak($s, 5000));
}

# --- end-to-end rendering ------------------------------------------
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
# mdArrow builds U+2190 (LEFTWARDS ARROW, outside WinAnsi) from its UTF-8 bytes,
# so these tests need no non-ASCII glyph in the source.
func mdArrow() {
    def b as bytes;
    $b[] = 0xE2;
    $b[] = 0x86;
    $b[] = 0x90;
    return convert.stringFromBytes($b, "utf-8");
}
func testQuoteBackgroundDefaultOff() {
    # with both off (the default), a quote renders exactly as before
    testing.assertEqual(pdfDefaults().quoteFill.on, false);
    testing.assertEqual(pdfDefaults().quoteRule.on, false);
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
func pdfHas(b as bytes, s as string) {
    return binary.contains($b, convert.bytesFromString($s, "utf-8"));
}
func mdImgFixture() {
    return encoding.fromText(MDIMGPNG, "base64");
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
func mdImageNode(url as string, alt as string) {
    def n as Node init nodeOf("image");
    $n.url = $url;
    $n.text = $alt;
    return $n;
}
# White-box: renderImage appends the draw operator and advances the pen down.
func testRenderImageDrawsAndAdvances() {
    def o as PdfOptions init pdfDefaults();
    def st as Layout init newLayout($o);
    $st.placements["p.png"] = mdStripBytes(pdf.loadImage("Pic", mdImgFixture()));
    def y0 as int init $st.y;
    def out as Layout init renderImage($st, mdImageNode("p.png", "alt"));
    testing.assertTrue(strings.contains($out.page.content, "/Pic Do"));
    testing.assertTrue($out.y < $y0);
}
# White-box: a picture wider than the text column is capped to the column width
# (aspect ratio kept), so a screenshot never overflows the margin.
func testRenderImageScalesToColumn() {
    def o as PdfOptions init pdfDefaults();
    def wide as pdf.Image init pdf.loadImage("Wide", mdImgFixture());
    $wide.width = 4000;
    $wide.height = 1000;
    def st as Layout init newLayout($o);
    $st.placements["w.png"] = mdStripBytes($wide);
    def out as Layout init renderImage($st, mdImageNode("w.png", "alt"));
    # drawImage writes "<w> 0 0 <h> <x> <y> cm"; the drawn width equals the column.
    testing.assertTrue(strings.contains($out.page.content, "{$st.width} 0 0 "));
}
# White-box: an image whose URL has no supplied bytes falls back to [alt] text,
# drawing no image.
func testRenderImageMissingFallsBackToText() {
    def o as PdfOptions init pdfDefaults();
    def st as Layout init newLayout($o);
    def out as Layout init renderImage($st, mdImageNode("nope.png", "the caption"));
    testing.assertFalse(strings.contains($out.page.content, " Do\n"));
    testing.assertTrue(strings.contains($out.page.content, "Tj"));
}
# Default options keep images off (empty map) and imageDpi at 96.
func testPdfImageDefaults() {
    testing.assertEqual(len(pdfDefaults().images), 0);
    testing.assertEqual(pdfDefaults().imageDpi, 96);
}

# --- markdown extensions: fenced info, strikethrough, task lists -----
func tNode(kind as string, kids as list of Node) {
    def n as Node init nodeOf($kind);
    $n.children = $kids;
    return $n;
}
func tText(s as string) {
    return textNode($s);
}
func tPara(s as string) {
    return tNode("paragraph", [tText($s)]);
}
func tCode(s as string) {
    def n as Node init nodeOf("code");
    $n.text = $s;
    return $n;
}
func tDoc(kids as list of Node) {
    return tNode("document", $kids);
}
func plainAnsi(doc as Node) {
    return ansi.strip(render($doc, "ansi"));
}
func testItemHoldsBlocksHtml() {
    def item as Node init tNode("item", [tPara("a"), tCode("x")]);
    def l as Node init tNode("list", [$item]);
    testing.assertEqual(
        render(tDoc([$l]), "html"),
        "<ul><li><p>a</p><pre><code>x</code></pre></li></ul>");
}
func testItemHoldsBlocksAnsi() {
    def item as Node init tNode("item", [tPara("a"), tCode("x")]);
    def l as Node init tNode("list", [$item, tNode("item", [tText("b")])]);
    testing.assertEqual(plainAnsi(tDoc([$l])), "  - a\n        x\n  - b");
}
func testItemOpeningWithNestedListAnsi() {
    def inner as Node init tNode("list", [tNode("item", [tText("n")])]);
    def item as Node init tNode("item", [$inner, tPara("after")]);
    def l as Node init tNode("list", [$item]);
    # The nested list's indent matches the text column, so its first item shares
    # the marker's line, the compact shape a Markdown sub-list also takes.
    testing.assertEqual(plainAnsi(tDoc([$l])), "  - - n\n    after");
}
func testOrderedListStartAndType() {
    def l as Node init tNode("list", [tNode("item", [tText("x")]), tNode("item", [tText("y")])]);
    $l.ordered = true;
    $l.attrs["start"] = "3";
    $l.attrs["enumtype"] = "i";
    testing.assertEqual(
        render(tDoc([$l]), "html"),
        '<ol start="3" type="i"><li>x</li><li>y</li></ol>');
    testing.assertEqual(plainAnsi(tDoc([$l])), "  iii. x\n  iv. y");
    # A malformed start or an unknown type falls back to 1 / digits.
    $l.attrs["start"] = "-2";
    $l.attrs["enumtype"] = "z";
    testing.assertEqual(render(tDoc([$l]), "html"), "<ol><li>x</li><li>y</li></ol>");
}
func testEnumMarkers() {
    testing.assertEqual(enumMarker("a", 1), "a");
    testing.assertEqual(enumMarker("a", 26), "z");
    testing.assertEqual(enumMarker("A", 27), "AA");
    testing.assertEqual(enumMarker("A", 703), "AAA");
    testing.assertEqual(enumMarker("i", 4), "iv");
    testing.assertEqual(enumMarker("I", 1994), "MCMXCIV");
    testing.assertEqual(enumMarker("I", 4000), "4000");
    testing.assertEqual(enumMarker("i", 0), "0");
    testing.assertEqual(enumMarker("1", 12), "12");
}
func testBlockAttrsMergeAndFilter() {
    def a as Node init tNode("admonition", [tPara("body")]);
    $a.lang = "note";
    $a.attrs["class"] = "extra";
    $a.attrs["id"] = "n1";
    $a.attrs["onclick"] = "alert(1)";
    $a.attrs["style"] = "x";
    testing.assertEqual(
        render(tDoc([$a]), "html"),
        '<div id="n1" class="admonition admonition-note extra">' +
            '<p class="admonition-title">Note</p><p>body</p></div>');
    def p as Node init tPara("t");
    $p.attrs["id"] = "here";
    testing.assertEqual(render(tDoc([$p]), "html"), '<p id="here">t</p>');
    def c as Node init tCode("x");
    $c.lang = "py";
    $c.attrs["class"] = "literal-block";
    testing.assertEqual(
        render(tDoc([$c]), "html"),
        '<pre class="literal-block"><code class="language-py">x</code></pre>');
}
func testLineBlock() {
    def nested as Node init tNode("line_block", [tNode("line", [tText("in")])]);
    def lb as Node init tNode(
        "line_block",
        [tNode("line", [tText("one")]), tNode("line", []), $nested]);
    testing.assertEqual(
        render(tDoc([$lb]), "html"),
        '<div class="line-block"><div class="line">one</div><div class="line"><br></div>' +
            '<div class="line-block"><div class="line">in</div></div></div>');
    testing.assertEqual(plainAnsi(tDoc([$lb])), "one\n\n    in");
    testing.assertContains(render(tDoc([$lb]), "xhtml"), "<br />");
}
func testFootnoteAndReference() {
    def ref as Node init nodeOf("footnote_ref");
    $ref.text = "1";
    $ref.url = "#fn1";
    $ref.attrs["id"] = "r1";
    def para as Node init tNode("paragraph", [tText("See"), $ref]);
    def fn as Node init tNode("footnote", [tPara("Note.")]);
    $fn.attrs["id"] = "fn1";
    $fn.attrs["label"] = "1";
    $fn.attrs["backref"] = "r1";
    testing.assertEqual(
        render(tDoc([$para, $fn]), "html"),
        '<p>See<a id="r1" class="footnote-reference" href="#fn1" role="doc-noteref">[1]</a></p>' +
            '<aside id="fn1" class="footnote" role="doc-footnote"><span class="label">' +
            '<a role="doc-backlink" href="#r1">[1]</a></span><p>Note.</p></aside>');
    testing.assertEqual(plainAnsi(tDoc([$para, $fn])), "See[1]\n\n[1] Note.");
    testing.assertEqual(text($ref), "1");
}
func testCitationAndUnsafeReference() {
    def ref as Node init nodeOf("citation_ref");
    $ref.text = "CIT2002";
    $ref.url = "javascript:alert(1)";
    def cit as Node init tNode("citation", [tPara("A book."), tPara("Second.")]);
    $cit.attrs["label"] = "CIT2002";
    testing.assertEqual(
        render(tDoc([tNode("paragraph", [$ref]), $cit]), "html"),
        '<p><a class="citation-reference" href="#">[CIT2002]</a></p>' +
            '<aside class="citation"><span class="label">[CIT2002]</span>' +
            '<p>A book.</p><p>Second.</p></aside>');
    # The note's later lines indent past its label.
    testing.assertEqual(plainAnsi(tDoc([$cit])), "[CIT2002] A book.\n          Second.");
}
func testFigure() {
    def img as Node init nodeOf("image");
    $img.url = "a.png";
    $img.text = "A";
    def fig as Node init tNode(
        "figure",
        [$img, tNode("caption", [tText("Cap")]), tNode("legend", [tPara("Leg")])]);
    $fig.attrs["class"] = "wide";
    testing.assertEqual(
        render(tDoc([$fig]), "html"),
        '<figure class="wide"><img src="a.png" alt="A"><figcaption><p>Cap</p>' +
            '<div class="legend"><p>Leg</p></div></figcaption></figure>');
    testing.assertEqual(plainAnsi(tDoc([$fig])), "[image] A (a.png)\nCap\nLeg");
}
func testTopicRubricContainer() {
    def t as Node init tNode("topic", [tPara("body")]);
    $t.title = "Abstract";
    testing.assertEqual(
        render(tDoc([$t]), "html"),
        '<aside class="topic"><p class="topic-title">Abstract</p><p>body</p></aside>');
    testing.assertEqual(plainAnsi(tDoc([$t])), "Abstract\nbody");
    def r as Node init tNode("rubric", [tText("Aside")]);
    testing.assertEqual(render(tDoc([$r]), "html"), '<p class="rubric">Aside</p>');
    def box as Node init tNode("container", [tPara("a"), tPara("b")]);
    $box.attrs["class"] = "box";
    testing.assertEqual(render(tDoc([$box]), "html"), '<div class="box"><p>a</p><p>b</p></div>');
    testing.assertEqual(plainAnsi(tDoc([$box])), "a\n\nb");
}
func testInlineSpanCiteAbbrCode() {
    def sp as Node init tNode("span", [tText("s")]);
    $sp.attrs["class"] = "x";
    def ct as Node init tNode("cite", [tText("Book")]);
    def ab as Node init tNode("abbr", [tText("HTML")]);
    $ab.title = "HyperText Markup Language";
    def cs as Node init nodeOf("codespan");
    $cs.text = "E=mc^2";
    $cs.attrs["class"] = "math";
    def p as Node init tNode("paragraph", [$sp, $ct, $ab, $cs]);
    testing.assertEqual(
        render(tDoc([$p]), "html"),
        '<p><span class="x">s</span><cite>Book</cite>' +
            '<abbr title="HyperText Markup Language">HTML</abbr>' +
            '<code class="math">E=mc^2</code></p>');
    testing.assertEqual(plainAnsi(tDoc([$p])), "sBookHTMLE=mc^2");
}
func tRow(cells as list of string) {
    def kids as list of Node init [];
    for (def c in $cells) {
        $kids[] = tNode("cell", [tText($c)]);
    }
    return tNode("row", $kids);
}
func testTableHeaderRows() {
    def t as Node init tNode("table", [tRow(["a", "b"]), tRow(["c", "d"])]);
    $t.attrs["headerRows"] = "0";
    testing.assertEqual(
        render(tDoc([$t]), "html"),
        "<table><tbody><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></tbody></table>");
    testing.assertEqual(plainAnsi(tDoc([$t])), "a | b\nc | d");
    $t.attrs["headerRows"] = "2";
    testing.assertEqual(plainAnsi(tDoc([$t])), "a | b\nc | d\n--+--");
    testing.assertContains(
        render(tDoc([$t]), "html"),
        "<thead><tr><th>a</th><th>b</th></tr><tr><th>c</th>");
}
func testTableSpans() {
    def wide as Node init tNode("cell", [tText("wide")]);
    $wide.attrs["colspan"] = "2";
    def tall as Node init tNode("cell", [tText("tall")]);
    $tall.attrs["rowspan"] = "2";
    def t as Node init tNode(
        "table",
        [
            tNode("row", [$wide, tNode("cell", [tText("h")])]),
            tNode("row", [$tall, tNode("cell", [tText("x")]), tNode("cell", [tText("y")])]),
            tRow(["p", "q"])
        ]);
    def out as string init render(tDoc([$t]), "html");
    testing.assertContains($out, '<th colspan="2">wide</th>');
    testing.assertContains($out, '<td rowspan="2">tall</td>');
    # The grid: the span's covered slots are empty, row 3 shifts past the rowspan.
    def g as list of list of Node init gridRows($t);
    testing.assertEqual(len($g), 3);
    testing.assertEqual(len($g[0]), 3);
    testing.assertEqual(text($g[0][2]), "h");
    testing.assertEqual(text($g[2][0]), "");
    testing.assertEqual(text($g[2][1]), "p");
    testing.assertEqual(text($g[2][2]), "q");
    testing.assertEqual(
        plainAnsi(tDoc([$t])),
        "wide |   | h\n-----+---+--\ntall | x | y\n     | p | q");
    # An absurd span is capped, not allocated.
    $wide.attrs["colspan"] = "999999999";
    def huge as Node init tNode("table", [tNode("row", [$wide])]);
    testing.assertEqual(len(gridRows($huge)[0]), 1000);
}
func testCellHoldsBlocks() {
    def cell as Node init tNode("cell", [tPara("one"), tPara("two")]);
    def t as Node init tNode("table", [tNode("row", [$cell])]);
    testing.assertEqual(
        render(tDoc([$t]), "html"),
        "<table><thead><tr><th><p>one</p><p>two</p></th></tr></thead><tbody></tbody></table>");
    testing.assertEqual(plainAnsi(tDoc([$t])), "one two\n-------");
    testing.assertEqual(plainText($cell), "one two");
}
func testDescriptionHoldsBlocks() {
    def dl as Node init tNode(
        "definition_list",
        [tNode("def_term", [tText("term")]), tNode("def_desc", [tPara("p1"), tCode("c")])]);
    testing.assertEqual(
        render(tDoc([$dl]), "html"),
        "<dl><dt>term</dt><dd><p>p1</p><pre><code>c</code></pre></dd></dl>");
    testing.assertEqual(plainAnsi(tDoc([$dl])), "term\n    p1\n        c");
}
func testBlockifyGroupsInlineRuns() {
    def out as list of Node init blockify([tText("a"), tText("b"), tCode("x"), tText("c")]);
    testing.assertEqual(len($out), 3);
    testing.assertEqual($out[0].kind, "paragraph");
    testing.assertEqual(text($out[0]), "ab");
    testing.assertEqual($out[1].kind, "code");
    testing.assertEqual(text($out[2]), "c");
}
# Every extended kind lays out in the PDF renderer without error.
func testExtendedKindsPdf() {
    def ref as Node init nodeOf("footnote_ref");
    $ref.text = "1";
    $ref.url = "#fn1";
    def fn as Node init tNode("footnote", [tPara("Footnote text."), tCode("code")]);
    $fn.attrs["label"] = "1";
    def img as Node init nodeOf("image");
    $img.url = "missing.png";
    $img.text = "alt";
    def topic as Node init tNode("topic", [tPara("Topic body")]);
    $topic.title = "Topic";
    def l as Node init tNode(
        "list",
        [tNode("item", [tPara("item para"), tCode("item code")]), tNode("item", [tText("plain")])]);
    $l.ordered = true;
    $l.attrs["start"] = "100";
    def wide as Node init tNode("cell", [tText("wide")]);
    $wide.attrs["colspan"] = "2";
    def doc as Node init tDoc([
        tNode("paragraph", [tText("See "), $ref, tNode("cite", [tText(" a book")])]),
        tNode("line_block", [tNode("line", [tText("l1")]), tNode("line", [])]),
        $fn,
        tNode(
            "figure",
            [$img, tNode("caption", [tText("Caption")]), tNode("legend", [tPara("Legend")])]),
        $topic,
        tNode("rubric", [tText("Rubric")]),
        tNode("container", [tPara("in a box")]),
        $l,
        tNode(
            "definition_list",
            [tNode("def_term", [tText("t")]), tNode("def_desc", [tPara("d")])]),
        tNode("table", [tNode("row", [$wide]), tRow(["a", "b"])])
    ]);
    def out as bytes init renderPdf($doc, pdfDefaults());
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}

# --- PDF inline layout and admonition labels (white-box, hand-built trees) ---

func tSpan(kind as string, s as string) {
    return tNode($kind, [tText($s)]);
}

func tCodespan(s as string) {
    def n as Node init nodeOf("codespan");
    $n.text = $s;
    return $n;
}

func tImage(alt as string, url as string) {
    def n as Node init nodeOf("image");
    $n.text = $alt;
    $n.url = $url;
    return $n;
}

func tAdmonition(kind as string, title as string) {
    def n as Node init tNode("admonition", [tPara("x")]);
    $n.lang = $kind;
    $n.title = $title;
    return $n;
}

func testFontForInlineByKind() {
    def o as PdfOptions init pdfDefaults();
    def p as Node init tNode(
        "paragraph",
        [
            tText("x "),
            tSpan("strong", "b"),
            tText(" "),
            tSpan("emphasis", "i"),
            tText(" "),
            tCodespan("c")
        ]);
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
    def p as Node init tNode("paragraph", [tText("hello world "), tSpan("strong", "bold text")]);
    def words as list of IWord init inlineWords(children($p), $o);
    testing.assertEqual(len($words), 4);
    testing.assertEqual($words[0].text, "hello");
    testing.assertEqual($words[0].font, $o.bodyFont);
    testing.assertEqual($words[3].text, "text");
    testing.assertEqual($words[3].font, $o.boldFont);
}

func testImageInlineRendersAlt() {
    def o as PdfOptions init pdfDefaults();
    def p as Node init tNode("paragraph", [tImage("a cat", "/c.png")]);
    def words as list of IWord init inlineWords(children($p), $o);
    # "[a" and "cat]" - the alt text bracketed.
    testing.assertEqual($words[0].text, "[a");
    testing.assertEqual($words[1].text, "cat]");
}

# An image inline in a sentence is NOT drawn - only a lone-image paragraph
# routes to renderImage; a mixed paragraph flows as text (the [alt] run).
func testInlineImageNotDrawn() {
    def o as PdfOptions init pdfDefaults();
    def st as Layout init newLayout($o);
    $st.placements["a.png"] = mdStripBytes(pdf.loadImage("Inl", mdImgFixture()));
    def para as Node init tNode(
        "paragraph",
        [tText("text before "), tImage("tiny", "a.png"), tText(" after")]);
    def out as Layout init renderBlock($st, $para, 0);
    testing.assertFalse(strings.contains($out.page.content, "/Inl Do"));
    testing.assertTrue(strings.contains($out.page.content, "Tj"));
}

func testTheLabelTableTranslates() {
    def o as PdfOptions init pdfDefaults();
    $o.admonitionLabels = {"note": "Hinweis", "warning": "Warnung"};
    testing.assertEqual(pdfAlertLabel(tAdmonition("note", ""), $o), "Hinweis");
    # A kind the table does not name is called by its own name rather than
    # blank.
    testing.assertEqual(pdfAlertLabel(tAdmonition("tip", ""), $o), "Tip");
}

# The title is the author's words, so it outranks any table.
func testATitleOutranksTheTable() {
    def o as PdfOptions init pdfDefaults();
    $o.admonitionLabels = {"note": "Hinweis"};
    testing.assertEqual(pdfAlertLabel(tAdmonition("note", "Mind the gap"), $o), "Mind the gap");
}

# The label reaches the page as an ordinary bold paragraph, so the quotation
# renderer measures and paints it with the rest.
func testTitledPrependsTheLabel() {
    def n as Node init tAdmonition("note", "");
    def out as Node init titled($n, pdfDefaults());
    testing.assertEqual(len(children($out)), len(children($n)) + 1);
    def label as Node init children($out)[0];
    testing.assertEqual(typeOf($label), "paragraph");
    testing.assertEqual(typeOf(children($label)[0]), "strong");
    testing.assertEqual(text($label), "Note");
    # The caller's tree is untouched: nodes are values.
    testing.assertEqual(len(children($n)), 1);
}

func testTableTitleIsACaption() {
    def cell as Node init nodeOf("cell");
    $cell.children = [textNode("x")];
    def row as Node init nodeOf("row");
    $row.children = [$cell];
    def t as Node init nodeOf("table");
    $t.title = "Prices";
    $t.attrs = {"headerRows": "0"};
    $t.children = [$row];
    def doc as Node init nodeOf("document");
    $doc.children = [$t];
    testing.assertEqual(
        render($doc, "html"),
        "<table><caption>Prices</caption><tbody><tr><td>x</td></tr></tbody></table>");
    testing.assertContains(render($doc, "ansi"), "Prices");
    testing.assertTrue(len(renderPdf($doc, pdfDefaults())) > 500);
}
