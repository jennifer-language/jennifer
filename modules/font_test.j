# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
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

# The TrueType fixture, base64-encoded (from scripts/gen-font-fixture.py): a
# straight-line glyph (A), a quadratic-curve glyph (B), a composite glyph (C),
# OS/2 v2 metrics (cap-height 700, x-height 500), and a kern table (A/B, A/C).
def const FIXTURE as string init "AAEAAAALAIAAAwAwT1MvMkdkQxAAAAE4AAAAYGNtYXAADACWAAABqAAAADRnbHlm830+jQAAAegAAABGaGVhZC9UHlgAAAC8AAAANmhoZWEFwAJdAAAA9AAAACRobXR4CcQBGAAAAZgAAAAQa2Vybv/y//AAAAIwAAAAHmxvY2EALwAaAAAB3AAAAAptYXhwAAgACwAAARgAAAAgbmFtZUAEQAgAAAJQAAAAaXBvc3QAUAAlAAACvAAAACoAAQAAAAEAAMWqXcpfDzz1AAED6AAAAADmj+1eAAAAAOaP7V4AZAAAArwCvAAAAAMAAgAAAAAAAAABAAADIP84AFoCvABQAGQB9AABAAAAAAAAAAAAAAAAAAAABAABAAAABAAFAAEAAwABAAIAAAAAAAAAAAAAAAAAAQABAAICcQGQAAUABAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAD8/Pz8AAABBAEMDDP8kAGQAAAAAAAAAAAAAAAAB9AK8AAAAIAAAAlgAAAJYAGQCWABQArwAZAAAAAIAAAADAAAAFAADAAEAAAAUAAQAIAAAAAQABAABAAAAQ///AAAAQf///8AAAQAAAAAAAAAAAAwAGgAjAAAAAQBkAAAB9AK8AAIAADMhA2QBkMgCvAAAAQBkAAABwgJYAAQAADMRIBEQZAFeAlj+1P7U//8BLAAAArwCvAAHAAEAyAAAAAAAAAABAAAAGgABAAIADAABAAAAAQAC/84AAQAD/+IAAAAAAAQANgABAAAAAAABAAoAAAABAAAAAAACAAcACgADAAEECQABABQAEQADAAEECQACAA4AJUplbkZpeHR1cmVSZWd1bGFyAEoAZQBuAEYAaQB4AHQAdQByAGUAUgBlAGcAdQBsAGEAcgAAAAACAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAkACUAJgAA";

# The OpenType/CFF fixture: the same A / B glyphs as PostScript (Type2) outlines.
def const FIXTURE_CFF as string init "T1RUTwAJAIAAAwAQQ0ZGIGUglzAAAAIoAAAAc09TLzJHZEL2AAABAAAAAGBjbWFwAAwAlQAAAdQAAAA0aGVhZC6OHlgAAACcAAAANmhoZWEF1AH2AAAA1AAAACRobXR4ArwAZAAAApwAAAAIbWF4cAADUAAAAAD4AAAABm5hbWXxQu4lAAABYAAAAHJwb3N0AAMAAAAAAggAAAAgAAEAAAABAAA8620GXw889QADA+gAAAAA5o/tXgAAAADmj+1eAGQAAAH0ArwAAAADAAIAAAAAAAAAAQAAAyD/OABaAlgAZABkAfQAAQAAAAAAAAAAAAAAAAAAAAEAAFAAAAMAAAACAlgBkAAFAAQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAA/Pz8/AAAAQQBCAwz/JABkAAAAAAAAAAAAAAAAAfQCvAAAACAAAAAAAAQANgABAAAAAAABAA0AAAABAAAAAAACAAcADQADAAEECQABABoAFAADAAEECQACAA4ALkplbkZpeHR1cmVDRkZSZWd1bGFyAEoAZQBuAEYAaQB4AHQAdQByAGUAQwBGAEYAUgBlAGcAdQBsAGEAcgAAAAAAAgAAAAMAAAAUAAMAAQAAABQABAAgAAAABAAEAAEAAABC//8AAABB////wAABAAAAAAADAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAEAQABAQEOSmVuRml4dHVyZUNGRgABAQET+BsC74v4iPlQBcwPi/cHEtARAAEBAQ5KZW5GaXh0dXJlQ0ZGAAABACIBAAMBAQQRKPjsDvjs7xb4JAb7XPlQBQ747O8W+OwH9/KL+8D7Kvsq+1yL+yofDgACWAAAAGQAZA==";

func loadFixture() {
    return parse(encoding.fromText(FIXTURE, "base64"));
}

func loadCff() {
    return parse(encoding.fromText(FIXTURE_CFF, "base64"));
}

# A hand-crafted hostile CFF whose glyph A calls a global subroutine that calls
# itself forever - it must be rejected (a bounded, catchable error), not hang.
def const FIXTURE_EVIL as string init "T1RUTwAJAIAAAwAQQ0ZGIOlJdlUAAAIMAAAANU9TLzJBOEHdAAABAAAAAGBjbWFwAAwAlAAAAbgAAAA0aGVhZCw2JtgAAACcAAAANmhoZWEDIf86AAAA1AAAACRobXR4AlgAAAAAAkQAAAAGbWF4cAACUAAAAAD4AAAABm5hbWVgRFF7AAABYAAAAFdwb3N0AAMAAAAAAewAAAAgAAEAAAABAABtzOCLXw889QADA+gAAAAA5o/y/AAAAADmj/L8AAAAAAAAAAAAAAADAAIAAAAAAAAAAQAAAyD/OAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAFAAAAIAAAADAlgBkAAFAAQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAA/Pz8/AAAAQQBBAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACAAAAAAAAQANgABAAAAAAABAAQAAAABAAAAAAACAAcABAADAAEECQABAAgACwADAAEECQACAA4AE0V2aWxSZWd1bGFyAEUAdgBpAGwAUgBlAGcAdQBsAGEAcgAAAAACAAAAAwAAABQAAwABAAAAFAAEACAAAAAEAAQAAQAAAEH//wAAAEH////AAAEAAAAAAAMAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABAAQBAAEBAQVFdmlsAAEBAQitD4vAErARAAAAAQEBAyAdAAAiAAIBAQYL+OyLFg6LixUgHQAAAAJYAAAAAAAA";

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

# An OTTO (CFF) container is now recognised, but a bare header with no tables is
# still rejected for the missing required tables.
func testRejectsMalformedCff() {
    testing.assertThrows("parseOtto", "font");
}
func parseOtto() {
    def b as bytes;
    # "OTTO" then padding (no table directory)
    $b[] = 0x4F; $b[] = 0x54; $b[] = 0x54; $b[] = 0x4F;
    for (def i as int init 0; $i < 12; $i = $i + 1) {
        $b[] = 0;
    }
    parse($b);
}

# ---- OS/2 + hhea metrics ----

func testVerticalMetrics() {
    def f as Font init loadFixture();
    testing.assertEqual(ascender($f), 780);     # OS/2 sTypoAscender
    testing.assertEqual(descender($f), -220);   # OS/2 sTypoDescender
    testing.assertEqual(lineGap($f), 100);      # OS/2 sTypoLineGap
    testing.assertEqual(capHeight($f), 700);    # OS/2 sCapHeight (v2)
    testing.assertEqual(xHeight($f), 500);      # OS/2 sxHeight (v2)
}

# ---- kern table ----

func testKerning() {
    def f as Font init loadFixture();
    testing.assertEqual(kern($f, 65, 66), -50);   # A/B pair
    testing.assertEqual(kern($f, 65, 67), -30);   # A/C pair
    testing.assertEqual(kern($f, 66, 65), 0);     # B/A: no entry
    testing.assertEqual(kern($f, 65, 90), 0);     # Z absent from the font
}

func testKernAbsentTable() {
    # the CFF fixture has no kern table, so every pair is 0.
    testing.assertEqual(kern(loadCff(), 65, 66), 0);
}

# ---- CFF / OTTO outlines ----

func testCffParsesAndMetrics() {
    def f as Font init loadCff();
    testing.assertEqual(name($f), "JenFixtureCFF");
    testing.assertTrue($f.cff != 0);            # CFF-backed
    testing.assertEqual(unitsPerEm($f), 1000);
    testing.assertEqual(advance($f, 65), 600);  # from hmtx, shared with glyf
    testing.assertEqual(capHeight($f), 700);
}

# CFF glyph A (straight lines) and B (two cubic curves) render to native SVG
# paths, pinned exactly (cross-checked against fontTools at IoU 1.0).
func testCffGlyphPathLines() {
    testing.assertEqual(glyphPath(loadCff(), 65), "M 100 0 L 500 0 L 300 700 Z");
}

func testCffGlyphPathCurves() {
    testing.assertEqual(glyphPath(loadCff(), 66),
        "M 100 0 L 100 600 C 450 600 450 300 450 150 C 450 0 250 0 100 0 Z");
}

func testCffGlyphStruct() {
    # glyph() returns the quadratic-approximated struct; A is all straight lines,
    # so it round-trips to on-curve points with the exact bounding box.
    def g as Glyph init glyph(loadCff(), 65);
    testing.assertEqual($g.advance, 600);
    testing.assertEqual($g.xMin, 100);
    testing.assertEqual($g.xMax, 500);
    testing.assertEqual($g.yMax, 700);
    testing.assertEqual(len($g.contours), 1);
}

func testCffSubrBias() {
    testing.assertEqual(subrBias(0), 107);
    testing.assertEqual(subrBias(1240), 1131);
    testing.assertEqual(subrBias(33900), 32768);
}

# A codepoint the CFF font lacks maps to glyph 0 (.notdef, empty here) -> "".
func testCffMissingGlyph() {
    testing.assertEqual(glyphPath(loadCff(), 0x4E00), "");   # a CJK char absent from the fixture
}

# A charstring with runaway subroutine recursion is rejected, not hung.
func testCffRecursionGuardThrows() {
    testing.assertThrows("outlineEvil", "font");
}
func outlineEvil() {
    def f as Font init parse(encoding.fromText(FIXTURE_EVIL, "base64"));
    glyphPath($f, 65);
}
