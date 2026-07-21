# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# pdfwriter_test.j - white-box tests for pdfwriter.j. Run with:
#
#     jennifer test modules/pdfwriter_test.j
#
# These cover the builders and the private helpers (escapeString, colorComp,
# zeroPad, font tracking) and a byte-level sanity check on render output; the
# rendered PDF is validated with qpdf / pdftotext in the Go suite
# (cmd/jennifer/pdfwriter_test.go). pdfwriter.j already `use`s strings / lists /
# convert / compress, so the overlay only adds testing.
use testing;

# --- private helpers --------------------------------------------------------

func testEscapeString() {
    testing.assertEqual(escapeString("plain"), "plain");
    testing.assertEqual(escapeString("a(b)c"), "a\\(b\\)c");
    testing.assertEqual(escapeString("back\\slash"), "back\\\\slash");
    testing.assertEqual(escapeString("line\nbreak"), "line\\nbreak");
}

func testColorComp() {
    testing.assertEqual(colorComp(0), "0");
    testing.assertEqual(colorComp(255), "1");
    testing.assertTrue(strings.startsWith(colorComp(128), "0.5"));
}

func testZeroPad() {
    testing.assertEqual(zeroPad(42, 10), "0000000042");
    testing.assertEqual(zeroPad(0, 10), "0000000000");
    testing.assertEqual(zeroPad(1234567890, 10), "1234567890");
}

# --- builders ---------------------------------------------------------------

func testTextContent() {
    def p as Page init text(page(612, 792), 72, 720, "Helvetica", 24, "Hi");
    testing.assertEqual($p.content, "BT\n/Helvetica 24 Tf\n72 720 Td\n(Hi) Tj\nET\n");
    testing.assertEqual(len($p.fonts), 1);
    testing.assertEqual($p.fonts[0], "Helvetica");
}

func testTextEscapesInContent() {
    def p as Page init text(page(612, 792), 0, 0, "Courier", 10, "a(b)\\c");
    testing.assertTrue(strings.contains($p.content, "(a\\(b\\)\\\\c) Tj"));
}

func testUnknownFontThrows() {
    def threw as bool init false;
    try {
        text(page(612, 792), 0, 0, "ComicSans", 12, "no");
    } catch (e) {
        $threw = true;
    }
    testing.assertTrue($threw);
}

func testFontDedup() {
    def p as Page init page(612, 792);
    $p = text($p, 0, 0, "Helvetica", 12, "a");
    $p = text($p, 0, 20, "Helvetica", 12, "b");
    testing.assertEqual(len($p.fonts), 1);           # same font once
    $p = text($p, 0, 40, "Times-Roman", 12, "c");
    testing.assertEqual(len($p.fonts), 2);           # a second distinct font
}

func testLineContent() {
    def p as Page init line(page(612, 792), 72, 100, 540, 100);
    testing.assertEqual($p.content, "72 100 m\n540 100 l\nS\n");
}

func testRectFilledVsStroke() {
    def filled as Page init rect(page(612, 792), 10, 20, 30, 40, true);
    testing.assertEqual($filled.content, "10 20 30 40 re\nf\n");
    def stroked as Page init rect(page(612, 792), 10, 20, 30, 40, false);
    testing.assertEqual($stroked.content, "10 20 30 40 re\nS\n");
}

func testColorContent() {
    def p as Page init color(page(612, 792), 255, 0, 0);
    testing.assertEqual($p.content, "1 0 0 rg\n1 0 0 RG\n");
}

# --- render (byte-level sanity) ---------------------------------------------

func sampleDoc() {
    def p as Page init text(page(612, 792), 72, 720, "Helvetica", 24, "Hi");
    $p = rect($p, 72, 600, 200, 60, true);
    return addPage(document(), $p);
}

func testRenderHeaderBytes() {
    def out as bytes init render(sampleDoc());
    testing.assertEqual($out[0], 0x25);   # %
    testing.assertEqual($out[1], 0x50);   # P
    testing.assertEqual($out[2], 0x44);   # D
    testing.assertEqual($out[3], 0x46);   # F
    testing.assertTrue(len($out) > 200);
}

func testRenderEmptyDocument() {
    # A document with no pages still renders a structurally-complete PDF.
    def out as bytes init render(document());
    testing.assertEqual($out[0], 0x25);
    testing.assertTrue(len($out) > 60);
}

# --- metadata ---------------------------------------------------------------

func testDefaultProducer() {
    def doc as Document init document();
    testing.assertEqual($doc.info["Producer"], "Jennifer pdfwriter");
    testing.assertEqual(len($doc.info), 1);   # no CreationDate auto-stamped
}

func testInfoSetter() {
    def doc as Document init info(document(), "Title", "My Report");
    $doc = info($doc, "Author", "Ada");
    testing.assertEqual($doc.info["Title"], "My Report");
    testing.assertEqual($doc.info["Author"], "Ada");
    testing.assertEqual($doc.info["Producer"], "Jennifer pdfwriter");   # default preserved
}

func testPdfDate() {
    testing.assertEqual(pdfDate(time.fromIso("2026-07-14T16:00:00Z")), "D:20260714160000+00'00'");
    testing.assertEqual(pdfDate(time.fromIso("2026-01-02T03:04:05+01:00")), "D:20260102030405+01'00'");
}

func testRenderIsDeterministic() {
    # Same document renders to identical bytes (no timestamps): test-friendly.
    def doc as Document init info(document(), "Title", "Stable");
    $doc = addPage($doc, text(page(612, 792), 72, 720, "Helvetica", 12, "hi"));
    def a as bytes init render($doc);
    def b as bytes init render($doc);
    testing.assertEqual(len($a), len($b));
    def i as int init 0;
    while ($i < len($a)) {
        testing.assertEqual($a[$i], $b[$i]);
        $i = $i + 1;
    }
}
