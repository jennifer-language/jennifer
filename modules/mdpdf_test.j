# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# mdpdf_test.j - white-box tests for mdpdf.j. Run with:
#
#     jennifer test modules/mdpdf_test.j
#
# The overlay splices mdpdf.j in front of this file, so the tests reach its private
# helpers (lineH, headingSize, packLines, inlineWords, fontForInline, newLayout,
# ensureSpace) and structs (Options, Layout, IWord) by bare identifier, alongside its
# exported render / renderWith / renderTree / defaults and the spliced `markdown` /
# `pdf` imports. PDF output is checked structurally: it starts with the `%PDF` marker.
use testing;
use binary;
use convert;

func pdfMarker() {
    return convert.bytesFromString("%PDF", "utf-8");
}

# --- options + metrics ---------------------------------------------

func testDefaults() {
    def o as Options init defaults();
    testing.assertEqual($o.pageWidth, 612);
    testing.assertEqual($o.pageHeight, 792);
    testing.assertEqual($o.margin, 72);
    testing.assertEqual($o.bodyFont, "Helvetica");
    testing.assertEqual($o.boldFont, "Helvetica-Bold");
    testing.assertEqual($o.monoFont, "Courier");
}

func testLineHeight() {
    testing.assertEqual(lineH(11), 15);
    testing.assertEqual(lineH(24), 33);
}

func testHeadingSizeByLevel() {
    def o as Options init defaults();
    testing.assertEqual(headingSize(1, $o), 22);
    testing.assertEqual(headingSize(2, $o), 17);
    testing.assertEqual(headingSize(6, $o), $o.bodySize);
    # An out-of-range level falls back to the body size.
    testing.assertEqual(headingSize(9, $o), $o.bodySize);
}

# --- inline layout --------------------------------------------------

func testFontForInlineByKind() {
    def o as Options init defaults();
    def doc as markdown.Node init markdown.parse("x **b** *i* `c`\n");
    def p as markdown.Node init markdown.get($doc, "paragraph");
    for (def n in markdown.children($p)) {
        def k as string init markdown.typeOf($n);
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
    def o as Options init defaults();
    def doc as markdown.Node init markdown.parse("hello world **bold text**\n");
    def p as markdown.Node init markdown.get($doc, "paragraph");
    def words as list of IWord init inlineWords(markdown.children($p), $o);
    testing.assertEqual(len($words), 4);
    testing.assertEqual($words[0].text, "hello");
    testing.assertEqual($words[0].font, $o.bodyFont);
    testing.assertEqual($words[3].text, "text");
    testing.assertEqual($words[3].font, $o.boldFont);
}

func testImageInlineRendersAlt() {
    def o as Options init defaults();
    def doc as markdown.Node init markdown.parse("![a cat](/c.png)\n");
    def p as markdown.Node init markdown.get($doc, "paragraph");
    def words as list of IWord init inlineWords(markdown.children($p), $o);
    # "[a" and "cat]" - the alt text bracketed.
    testing.assertEqual($words[0].text, "[a");
    testing.assertEqual($words[1].text, "cat]");
}

func testPackLinesRespectsWidth() {
    def o as Options init defaults();
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
    def s as Layout init newLayout(defaults());
    # Drive the pen near the bottom margin, then demand more than fits.
    $s.y = 77;
    $s = ensureSpace($s, 50);
    # A flush resets the pen to the top of a fresh page.
    testing.assertEqual($s.y, 720);
}

func testEnsureSpaceKeepsPageWhenRoom() {
    def s as Layout init newLayout(defaults());
    def before as int init $s.y;
    $s = ensureSpace($s, 20);
    testing.assertEqual($s.y, $before);
}

# --- end-to-end rendering ------------------------------------------

func testRenderProducesValidPdf() {
    def out as bytes init render("# Hi\n\nA paragraph of text.\n");
    testing.assertTrue(len($out) > 100);
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}

func testRenderAllBlockTypes() {
    def md as string init "# H\n\ntext **b** *i* `c`\n\n- a\n- b\n  - nested\n\n1. one\n2. two\n\n> quoted\n\n```\ncode\n```\n\n| A | B |\n|:--|--:|\n| 1 | 2 |\n";
    def out as bytes init render($md);
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}

func testEmptyDocRendersBlankPdf() {
    def out as bytes init render("");
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}

func testRenderTreeOnParsedTree() {
    def doc as markdown.Node init markdown.parse("# Tree\n\nrendered via renderTree.\n");
    def out as bytes init renderTree($doc, defaults());
    testing.assertTrue(binary.startsWith($out, pdfMarker()));
}

func testRenderWithA4Options() {
    def o as Options init defaults();
    $o.pageWidth = 595;
    $o.pageHeight = 842;
    def out as bytes init renderWith("# A4\n\nbody text\n", $o);
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
    def long as bytes init render($md);
    def short as bytes init render("# Short\n\none line.\n");
    testing.assertTrue(len($long) > len($short));
}
