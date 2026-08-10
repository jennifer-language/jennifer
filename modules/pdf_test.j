# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# pdf_test.j - white-box tests for pdf.j. Run with:
#
#     jennifer test modules/pdf_test.j
#
# These cover the builders and the private helpers (escapeString, colorComp,
# zeroPad, font tracking) and a byte-level sanity check on render output; the
# rendered PDF is validated with qpdf / pdftotext in the Go suite
# (cmd/jennifer/pdf_test.go). pdf.j already `use`s strings / lists /
# convert / compress, so the overlay only adds testing.
use testing;
use encoding;

# The font-module TrueType fixture (glyphs A / B / C) and CFF fixture, base64.
def const TTF as string init "AAEAAAALAIAAAwAwT1MvMkdkQxAAAAE4AAAAYGNtYXAADACWAAABqAAAADRnbHlm830+jQAAAegAAABGaGVhZC9UHlgAAAC8AAAANmhoZWEFwAJdAAAA9AAAACRobXR4CcQBGAAAAZgAAAAQa2Vybv/y//AAAAIwAAAAHmxvY2EALwAaAAAB3AAAAAptYXhwAAgACwAAARgAAAAgbmFtZUAEQAgAAAJQAAAAaXBvc3QAUAAlAAACvAAAACoAAQAAAAEAAMWqXcpfDzz1AAED6AAAAADmj+1eAAAAAOaP7V4AZAAAArwCvAAAAAMAAgAAAAAAAAABAAADIP84AFoCvABQAGQB9AABAAAAAAAAAAAAAAAAAAAABAABAAAABAAFAAEAAwABAAIAAAAAAAAAAAAAAAAAAQABAAICcQGQAAUABAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAD8/Pz8AAABBAEMDDP8kAGQAAAAAAAAAAAAAAAAB9AK8AAAAIAAAAlgAAAJYAGQCWABQArwAZAAAAAIAAAADAAAAFAADAAEAAAAUAAQAIAAAAAQABAABAAAAQ///AAAAQf///8AAAQAAAAAAAAAAAAwAGgAjAAAAAQBkAAAB9AK8AAIAADMhA2QBkMgCvAAAAQBkAAABwgJYAAQAADMRIBEQZAFeAlj+1P7U//8BLAAAArwCvAAHAAEAyAAAAAAAAAABAAAAGgABAAIADAABAAAAAQAC/84AAQAD/+IAAAAAAAQANgABAAAAAAABAAoAAAABAAAAAAACAAcACgADAAEECQABABQAEQADAAEECQACAA4AJUplbkZpeHR1cmVSZWd1bGFyAEoAZQBuAEYAaQB4AHQAdQByAGUAUgBlAGcAdQBsAGEAcgAAAAACAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAkACUAJgAA";
def const CFF as string init "T1RUTwAJAIAAAwAQQ0ZGIGUglzAAAAIoAAAAc09TLzJHZEL2AAABAAAAAGBjbWFwAAwAlQAAAdQAAAA0aGVhZC6OHlgAAACcAAAANmhoZWEF1AH2AAAA1AAAACRobXR4ArwAZAAAApwAAAAIbWF4cAADUAAAAAD4AAAABm5hbWXxQu4lAAABYAAAAHJwb3N0AAMAAAAAAggAAAAgAAEAAAABAAA8620GXw889QADA+gAAAAA5o/tXgAAAADmj+1eAGQAAAH0ArwAAAADAAIAAAAAAAAAAQAAAyD/OABaAlgAZABkAfQAAQAAAAAAAAAAAAAAAAAAAAEAAFAAAAMAAAACAlgBkAAFAAQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAA/Pz8/AAAAQQBCAwz/JABkAAAAAAAAAAAAAAAAAfQCvAAAACAAAAAAAAQANgABAAAAAAABAA0AAAABAAAAAAACAAcADQADAAEECQABABoAFAADAAEECQACAA4ALkplbkZpeHR1cmVDRkZSZWd1bGFyAEoAZQBuAEYAaQB4AHQAdQByAGUAQwBGAEYAUgBlAGcAdQBsAGEAcgAAAAAAAgAAAAMAAAAUAAMAAQAAABQABAAgAAAABAAEAAEAAABC//8AAABB////wAABAAAAAAADAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAEAQABAQEOSmVuRml4dHVyZUNGRgABAQET+BsC74v4iPlQBcwPi/cHEtARAAEBAQ5KZW5GaXh0dXJlQ0ZGAAABACIBAAMBAQQRKPjsDvjs7xb4JAb7XPlQBQ747O8W+OwH9/KL+8D7Kvsq+1yL+yofDgACWAAAAGQAZA==";

func ttfBytes() {
    return encoding.fromText(TTF, "base64");
}

# Tiny 8x6 raster fixtures (base64): an opaque RGB PNG, an RGBA PNG (alpha ramp),
# and a baseline RGB JPEG - the three image code paths (predictor passthrough,
# alpha decode + soft mask, DCTDecode).
def const RGBPNG as string init "iVBORw0KGgoAAAANSUhEUgAAAAgAAAAGCAIAAABxZ0isAAAAGklEQVR42mNkYGBQYBDBRCwMGiIMDFgQPSQAwgMFakXvt1IAAAAASUVORK5CYII=";
def const RGBAPNG as string init "iVBORw0KGgoAAAANSUhEUgAAAAgAAAAGCAYAAAD+Bd/7AAAAGUlEQVR42mNkaPjPwMDAoIALMzEQAINBAQDFdgJrNAocYQAAAABJRU5ErkJggg==";
def const JPGIMG as string init "/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAUDBAQEAwUEBAQFBQUGBwwIBwcHBw8LCwkMEQ8SEhEPERETFhwXExQaFRERGCEYGh0dHx8fExciJCIeJBweHx7/2wBDAQUFBQcGBw4ICA4eFBEUHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh7/wAARCAAGAAgDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwCr8N/hHpf7vmLt2ooor08BVn7FanXwhnuY/wBmw/es/9k=";

func rgbPng() {
    return encoding.fromText(RGBPNG, "base64");
}

func rgbaPng() {
    return encoding.fromText(RGBAPNG, "base64");
}

func jpgImg() {
    return encoding.fromText(JPGIMG, "base64");
}

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
    testing.assertEqual(len($p.fonts), 1); # same font once
    $p = text($p, 0, 40, "Times-Roman", 12, "c");
    testing.assertEqual(len($p.fonts), 2); # a second distinct font
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
    testing.assertEqual($out[0], 0x25); # %
    testing.assertEqual($out[1], 0x50); # P
    testing.assertEqual($out[2], 0x44); # D
    testing.assertEqual($out[3], 0x46); # F
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
    testing.assertEqual($doc.info["Producer"], "Jennifer pdf");
    testing.assertEqual(len($doc.info), 1); # no CreationDate auto-stamped
}

func testInfoSetter() {
    def doc as Document init info(document(), "Title", "My Report");
    $doc = info($doc, "Author", "Ada");
    testing.assertEqual($doc.info["Title"], "My Report");
    testing.assertEqual($doc.info["Author"], "Ada");
    testing.assertEqual($doc.info["Producer"], "Jennifer pdf"); # default preserved
}

func testPdfDate() {
    testing.assertEqual(pdfDate(time.fromIso("2026-07-14T16:00:00Z")), "D:20260714160000+00'00'");
    testing.assertEqual(
        pdfDate(time.fromIso("2026-01-02T03:04:05+01:00")),
        "D:20260102030405+01'00'");
}

# --- embedded fonts (M23.6) -------------------------------------------------

func testScaleMetric() {
    testing.assertEqual(scaleMetric(1000, 1000), 1000);   # 1:1 at 1000 upem
    testing.assertEqual(scaleMetric(1024, 2048), 500);    # half em at 2048 upem
    testing.assertEqual(scaleMetric(-220, 1000), -220);   # negative (descender)
}

func testHexGid() {
    testing.assertEqual(hexGid(0), "0000");
    testing.assertEqual(hexGid(65), "0041");
    testing.assertEqual(hexGid(0xABCD), "ABCD");
}

func testToUnicodeHex() {
    testing.assertEqual(toUnicodeHex(65), "0041");         # 'A'
    testing.assertEqual(toUnicodeHex(0x65E5), "65E5");     # a BMP CJK char
    testing.assertEqual(toUnicodeHex(0x1F600), "D83DDE00"); # astral -> surrogate pair
}

# textUnicode writes a hex-GID string and records each glyph used.
func testTextUnicodeContent() {
    def lf as LoadedFont init loadFont("Body", ttfBytes());
    def p as Page init textUnicode(page(400, 200), 40, 150, $lf, 24, "AB");
    # fixture maps A->gid 1, B->gid 2, so the run is <00010002>
    testing.assertTrue(strings.contains($p.content, "/Body 24 Tf"));
    testing.assertTrue(strings.contains($p.content, "<00010002> Tj"));
    testing.assertEqual(len($p.glyphUses), 2);
    testing.assertEqual($p.glyphUses[0].gid, 1);
    testing.assertEqual($p.glyphUses[0].cp, 65);
}

# A rendered document with an embedded font carries the Type0 composite, its
# CIDFontType2 descendant, the embedded FontFile2, and a ToUnicode map.
func testEmbeddedFontObjects() {
    def lf as LoadedFont init loadFont("Body", ttfBytes());
    def doc as Document init addFont(document(), $lf);
    $doc = addPage($doc, textUnicode(page(400, 200), 40, 150, $lf, 24, "ABC"));
    def out as bytes init render($doc);
    def s as string init convert.stringFromBytes(bytesSlice($out, 0, 8), "utf-8");
    testing.assertEqual($s, "%PDF-1.7");
    def txt as string init pdfText($out);
    testing.assertTrue(strings.contains($txt, "/Subtype /Type0"));
    testing.assertTrue(strings.contains($txt, "/CIDFontType2"));
    testing.assertTrue(strings.contains($txt, "/FontFile2"));
    testing.assertTrue(strings.contains($txt, "/ToUnicode"));
    testing.assertTrue(strings.contains($txt, "/Encoding /Identity-H"));
}

# pdfText decodes the ASCII-clause bytes of a PDF (the object dictionaries are
# ASCII; the compressed streams are opaque but we only search the dictionaries).
func pdfText(out as bytes) {
    def sb as list of string init [];
    def i as int init 0;
    while ($i < len($out)) {
        def b as int init $out[$i];
        if ($b >= 32 and $b < 127) {
            $sb[] = convert.fromCodepoint($b);
        } else {
            $sb[] = "\n";
        }
        $i = $i + 1;
    }
    return strings.join($sb, "");
}

func bytesSlice(b as bytes, lo as int, hi as int) {
    def out as bytes;
    def i as int init $lo;
    while ($i < $hi) {
        $out[] = $b[$i];
        $i = $i + 1;
    }
    return $out;
}

# A resource name that could break out of the PDF /Font or /XObject dictionary
# must be rejected (injection guard), for both fonts and images.
func testLoadFontRejectsInjectingName() {
    def threw as bool init false;
    try {
        loadFont("Evil>> /OpenAction << /S /JavaScript >>", ttfBytes());
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "pdf");
    }
    testing.assertTrue($threw);
}

func testLoadImageRejectsInjectingName() {
    def threw as bool init false;
    try {
        loadImage("A B", rgbPng());       # a space is not a legal resource name
    } catch (e) {
        $threw = true;
    }
    testing.assertTrue($threw);
}

func testResourceNameAcceptsLettersDigits() {
    # a legal letters-then-digits name is accepted (no false positive)
    def lf as LoadedFont init loadFont("Body2", ttfBytes());
    testing.assertEqual($lf.name, "Body2");
    def img as Image init loadImage("Logo1", rgbPng());
    testing.assertEqual($img.name, "Logo1");
}

func testLoadFontRejectsCff() {
    def threw as bool init false;
    try {
        loadFont("X", encoding.fromText(CFF, "base64"));
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "pdf");
    }
    testing.assertTrue($threw);
}

# --- images (M23.6) ---------------------------------------------------------

func testBeHelpers() {
    def b as bytes;
    $b[] = 0x12;
    $b[] = 0x34;
    $b[] = 0x56;
    $b[] = 0x78;
    testing.assertEqual(beU16($b, 0), 0x1234);
    testing.assertEqual(beU16($b, 2), 0x5678);
    testing.assertEqual(beU32($b, 0), 0x12345678);
}

func testPaeth() {
    testing.assertEqual(paeth(2, 3, 4), 2);      # p=1: |p-a|=1 smallest -> a
    testing.assertEqual(paeth(10, 20, 10), 20);  # p=20: |p-b|=0 smallest -> b
    testing.assertEqual(paeth(200, 100, 50), 200); # p=250: |p-a|=50 smallest -> a
}

# An opaque RGB PNG embeds via a FlateDecode predictor: no pixel decode, /Predictor 15.
func testLoadImagePngRgb() {
    def img as Image init loadImage("Rgb", rgbPng());
    testing.assertEqual($img.width, 8);
    testing.assertEqual($img.height, 6);
    testing.assertEqual($img.colorSpace, "/DeviceRGB");
    testing.assertEqual($img.filter, "FlateDecode");
    testing.assertEqual($img.predictor, 15);
    testing.assertEqual($img.colors, 3);
    testing.assertEqual($img.bits, 8);
    testing.assertTrue(not $img.hasSmask);
}

# An RGBA PNG is decoded, split into a colour stream and an alpha soft mask.
func testLoadImagePngRgba() {
    def img as Image init loadImage("Rgba", rgbaPng());
    testing.assertEqual($img.width, 8);
    testing.assertEqual($img.height, 6);
    testing.assertEqual($img.colorSpace, "/DeviceRGB");
    testing.assertEqual($img.predictor, 0);      # decoded, no predictor
    testing.assertTrue($img.hasSmask);
    testing.assertTrue(len($img.smask) > 0);
}

# A JPEG embeds as-is via DCTDecode.
func testLoadImageJpeg() {
    def img as Image init loadImage("Jpg", jpgImg());
    testing.assertEqual($img.width, 8);
    testing.assertEqual($img.height, 6);
    testing.assertEqual($img.colorSpace, "/DeviceRGB");
    testing.assertEqual($img.filter, "DCTDecode");
    testing.assertTrue(not $img.hasSmask);
}

func testLoadImageRejectsGarbage() {
    def threw as bool init false;
    def junk as bytes;
    def i as int init 0;
    while ($i < 20) {
        $junk[] = 0x41;
        $i = $i + 1;
    }
    try {
        loadImage("bad", $junk);
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "pdf");
    }
    testing.assertTrue($threw);
}

# A glyph id past 0xFFFF cannot be a 2-byte Identity-H code; hexGid must reject
# it, not silently truncate to the low 16 bits.
func testHexGidRejectsOverflow() {
    def threw as bool init false;
    try {
        hexGid(0x10000);
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "pdf");
    }
    testing.assertTrue($threw);
}

# A PNG whose chunk length runs past the buffer (truncated / hostile) is a clean
# pdf error, not a raw index-out-of-range abort.
func testLoadImageRejectsTruncated() {
    def threw as bool init false;
    try {
        loadImage("T", bytesSlice(rgbPng(), 0, 40));
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "pdf");
    }
    testing.assertTrue($threw);
}

func testDrawImageContent() {
    def img as Image init loadImage("Rgb", rgbPng());
    def p as Page init drawImage(page(200, 200), $img, 50, 700, 128, 96);
    testing.assertEqual($p.content, "q\n128 0 0 96 50 700 cm\n/Rgb Do\nQ\n");
}

# A rendered document with images carries image XObjects: an opaque predictor
# image, a soft-masked image, and a DCTDecode image, all listed in /XObject.
func testImageObjects() {
    def doc as Document init document();
    $doc = addImage($doc, loadImage("Rgb", rgbPng()));
    $doc = addImage($doc, loadImage("Rgba", rgbaPng()));
    $doc = addImage($doc, loadImage("Jpg", jpgImg()));
    def p as Page init drawImage(page(300, 300), loadImage("Rgb", rgbPng()), 10, 10, 80, 60);
    $doc = addPage($doc, $p);
    def txt as string init pdfText(render($doc));
    testing.assertTrue(strings.contains($txt, "/Subtype /Image"));
    testing.assertTrue(strings.contains($txt, "/Filter /DCTDecode"));
    testing.assertTrue(strings.contains($txt, "/SMask"));
    testing.assertTrue(strings.contains($txt, "/Predictor 15"));
    testing.assertTrue(strings.contains($txt, "/XObject << /Rgb"));
}

# --- text layout (M23.6) ----------------------------------------------------

func testMeasureEm() {
    testing.assertEqual(measureEm("Helvetica", "AV"), 1334);    # 667 + 667
    testing.assertEqual(measureEm("Helvetica", " "), 278);
    testing.assertEqual(measureEm("Times-Roman", "A"), 722);
    testing.assertEqual(measureEm("Courier", "WWWW"), 2400);    # monospaced 600
    testing.assertEqual(measureEm("Helvetica-Oblique", "AV"), 1334); # shares Helvetica
}

func testMeasureText() {
    testing.assertEqual(measureText("Helvetica", 12, "AV"), (1334 * 12) / 1000);
    testing.assertEqual(measureText("Courier", 10, "WWWW"), 24.0);
}

func testMeasureRejects() {
    def threw1 as bool init false;
    try {
        measureText("Arial", 12, "x");
    } catch (e) {
        $threw1 = true;
    }
    testing.assertTrue($threw1);                                # unknown font
    def threw2 as bool init false;
    try {
        measureText("Symbol", 12, "x");
    } catch (e) {
        $threw2 = true;
    }
    testing.assertTrue($threw2);                                # symbol font, no metrics
}

func testWrapText() {
    def lines as list of string init wrapText("Helvetica", 12, "aaa bbb ccc ddd eee fff", 60);
    testing.assertTrue(len($lines) >= 2);
    for (def l in $lines) {
        testing.assertTrue(measureText("Helvetica", 12, $l) <= 60);
    }
}

func testWrapHardNewline() {
    def lines as list of string init wrapText("Courier", 10, "one\ntwo\n\nfour", 500);
    testing.assertEqual(len($lines), 4);                        # one / two / blank / four
    testing.assertEqual($lines[0], "one");
    testing.assertEqual($lines[2], "");
    testing.assertEqual($lines[3], "four");
}

func testWrapCollapsesSpaces() {
    def lines as list of string init wrapText("Courier", 10, "a    b", 500);
    testing.assertEqual(len($lines), 1);
    testing.assertEqual($lines[0], "a b");
}

func testPackOverflow() {
    # a single word wider than the column lands alone (overflow, not truncated)
    def lines as list of string init wrapText("Helvetica", 12, "supercalifragilistic short", 20);
    testing.assertEqual(len($lines), 2);
    testing.assertEqual($lines[0], "supercalifragilistic");
}

func testAlignStart() {
    testing.assertEqual(alignStart(50, 300, 100.0, "left"), 50);
    testing.assertEqual(alignStart(50, 300, 100.0, "right"), 250);   # 50 + (300 - 100)
    testing.assertEqual(alignStart(50, 300, 100.0, "center"), 150);  # 50 + (300 - 100) / 2
}

func testTextBlockJustify() {
    def t as string init "the quick brown fox jumps over lazy dogs while the sun sets slowly";
    def j as Page init textBlock(page(400, 300), 50, 250, 150, "Helvetica", 12, 16, $t, "justify");
    def l as Page init textBlock(page(400, 300), 50, 250, 150, "Helvetica", 12, 16, $t, "left");
    testing.assertTrue(strings.contains($j.content, "Tj"));
    # justify places each word of a non-last line separately, so it emits more ops
    testing.assertTrue(len($j.content) > len($l.content));
}

func testTextBlockUnknownAlign() {
    def threw as bool init false;
    try {
        textBlock(page(400, 300), 0, 0, 100, "Helvetica", 12, 14, "hi", "middle");
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "pdf");
    }
    testing.assertTrue($threw);
}

func testMeasureTextUnicode() {
    def lf as LoadedFont init loadFont("Body", ttfBytes());
    def w as float init measureTextUnicode($lf, 20, "AB");
    testing.assertTrue($w > 0.0);
    # additive: width("AB") == width("A") + width("B")
    testing.assertEqual($w, measureTextUnicode($lf, 20, "A") + measureTextUnicode($lf, 20, "B"));
}

use convert;

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
