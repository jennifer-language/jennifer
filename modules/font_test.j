# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# font_test.j - white-box tests for font.j. Run with:
#
#     jennifer test modules/font_test.j
#
# Parses a tiny, committed TrueType fixture (embedded here as base64 so the test
# is self-contained; regenerate the .ttf with scripts/gen-font-fixture.py). The
# fixture has a straight-line glyph (A), a quadratic-curve glyph (B), and a
# composite glyph (C = A shifted +200), with known metrics and outlines.
use testing;
use encoding;
use strings;

# The fixture font, base64-encoded.
def const FIXTURE as string init "AAEAAAAKAIAAAwAgT1MvMkE4QfgAAAEoAAAAYGNtYXAADACWAAABmAAAADRnbHlm830+jQAAAdgAAABGaGVhZC8911AAAACsAAAANmhoZWEFZgJdAAAA5AAAACRobXR4CcQBGAAAAYgAAAAQbG9jYQAvABoAAAHMAAAACm1heHAACAALAAABCAAAACBuYW1lQARACAAAAiAAAABpcG9zdABQACUAAAKMAAAAKgABAAAAAQAAPi9jll8PPPUAAQPoAAAAAOaEydoAAAAA5oTJ2gBkAAACvAK8AAAAAwACAAAAAAAAAAEAAAMg/zgAAAK8AFAAZAH0AAEAAAAAAAAAAAAAAAAAAAAEAAEAAAAEAAUAAQADAAEAAgAAAAAAAAAAAAAAAAABAAEAAwJxAZAABQAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAPz8/PwAAAEEAQwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAgAAACWAAAAlgAZAJYAFACvABkAAAAAgAAAAMAAAAUAAMAAQAAABQABAAgAAAABAAEAAEAAABD//8AAABB////wAABAAAAAAAAAAAADAAaACMAAAABAGQAAAH0ArwAAgAAMyEDZAGQyAK8AAABAGQAAAHCAlgABAAAMxEgERBkAV4CWP7U/tT//wEsAAACvAK8AAcAAQDIAAAAAAAAAAQANgABAAAAAAABAAoAAAABAAAAAAACAAcACgADAAEECQABABQAEQADAAEECQACAA4AJUplbkZpeHR1cmVSZWd1bGFyAEoAZQBuAEYAaQB4AHQAdQByAGUAUgBlAGcAdQBsAGEAcgAAAAACAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAkACUAJgAA";

func loadFixture() {
    return parse(encoding.fromText(FIXTURE, "base64"));
}

func testHeaderAndName() {
    def f as Font init loadFixture();
    testing.assertEqual(unitsPerEm($f), 1000);
    testing.assertEqual(name($f), "JenFixture");
    testing.assertEqual($f.numGlyphs, 4);
    testing.assertEqual($f.cmapFmt, 4);              # BMP cmap
}

func testAdvances() {
    def f as Font init loadFixture();
    testing.assertEqual(advance($f, 65), 600);       # A
    testing.assertEqual(advance($f, 66), 600);       # B
    testing.assertEqual(advance($f, 67), 700);       # C
    # A codepoint the font lacks maps to glyph 0 (.notdef, advance 600).
    testing.assertEqual(advance($f, 90), 600);
}

func testSimpleGlyphPath() {
    # A is a triangle of three on-curve points, straight lines.
    testing.assertEqual(glyphPath(loadFixture(), 65),
        "M 100 0 L 500 0 L 300 700 L 100 0 Z");
}

func testQuadraticGlyphPath() {
    # B has two quadratic curves -> two Q commands.
    testing.assertEqual(glyphPath(loadFixture(), 66),
        "M 100 0 L 100 600 Q 450 600 450 300 Q 450 0 100 0 Z");
}

func testCompositeGlyphPath() {
    # C is glyph A translated +200 in x.
    testing.assertEqual(glyphPath(loadFixture(), 67),
        "M 300 0 L 700 0 L 500 700 L 300 0 Z");
}

func testGlyphContours() {
    def g as Font init loadFixture();
    def gl as Glyph init glyph($g, 65);
    testing.assertEqual($gl.advance, 600);
    testing.assertEqual($gl.xMin, 100);
    testing.assertEqual($gl.xMax, 500);
    testing.assertEqual($gl.yMax, 700);
    testing.assertEqual(len($gl.contours), 1);
    testing.assertEqual(len($gl.contours[0].points), 3);
    testing.assertTrue($gl.contours[0].points[0].onCurve);
    testing.assertEqual($gl.contours[0].points[1].x, 500);
}

func testEmptyGlyphHasNoContours() {
    # .notdef (glyph 0) in this fixture is empty.
    def gl as Glyph init glyphById(loadFixture(), 0, 0);
    testing.assertEqual(len($gl.contours), 0);
}

# ---- byte readers (private) ----

func testByteReaders() {
    def b as bytes;
    $b[] = 0x12; $b[] = 0x34; $b[] = 0x56; $b[] = 0x78;
    testing.assertEqual(ushort($b, 0), 4660);        # 0x1234
    testing.assertEqual(ulong($b, 0), 305419896);    # 0x12345678
    def s as bytes;
    $s[] = 0xFF; $s[] = 0xFE;                          # 0xFFFE -> -2 signed
    testing.assertEqual(sshort($s, 0), -2);
    testing.assertEqual(ubyte($s, 0), 255);
}

# ---- cmap format 12 (private, via a synthetic subtable) ----

func testCoverageLookupFmtTwelve() {
    # A minimal format-12 subtable mapping U+1F600 -> glyph 42.
    def sub as bytes;
    $sub[] = 0; $sub[] = 12;                           # format 12
    $sub[] = 0; $sub[] = 0;                            # reserved
    $sub[] = 0; $sub[] = 0; $sub[] = 0; $sub[] = 0;    # length (unused)
    $sub[] = 0; $sub[] = 0; $sub[] = 0; $sub[] = 0;    # language
    $sub[] = 0; $sub[] = 0; $sub[] = 0; $sub[] = 1;    # nGroups = 1
    $sub[] = 0; $sub[] = 1; $sub[] = 0xF6; $sub[] = 0; # startChar 0x1F600
    $sub[] = 0; $sub[] = 1; $sub[] = 0xF6; $sub[] = 0; # endChar   0x1F600
    $sub[] = 0; $sub[] = 0; $sub[] = 0; $sub[] = 42;   # startGID 42
    testing.assertEqual(coverageLookup($sub, 0, 128512), 42);   # 0x1F600
    testing.assertEqual(coverageLookup($sub, 0, 128513), 0);    # out of range
}

# ---- error handling ----

func testRejectsTooShort() {
    testing.assertThrows("parseTiny", "font");
}
func parseTiny() {
    def b as bytes;
    $b[] = 1; $b[] = 2;
    parse($b);
}

func testRejectsOtto() {
    testing.assertThrows("parseOtto", "font");
}
func parseOtto() {
    def b as bytes;
    # "OTTO" then padding
    $b[] = 0x4F; $b[] = 0x54; $b[] = 0x54; $b[] = 0x4F;
    for (def i as int init 0; $i < 12; $i = $i + 1) {
        $b[] = 0;
    }
    parse($b);
}
