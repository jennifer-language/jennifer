#!/usr/bin/env python3
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# Generates the `font` module's test fixtures:
#   modules/testdata/font_fixture.ttf     - a tiny TrueType (glyf) font
#   modules/testdata/font_fixture_cff.otf - a tiny OpenType/CFF font
# Both carry a simple glyph (A, straight lines), a curved glyph (B), OS/2 v2
# vertical metrics (cap-height, x-height), and the TTF adds a legacy kern table.
# The fixtures are also embedded as base64 in modules/font_test.j (this script
# prints the base64 to paste); rerun to regenerate. Requires fonttools.
import base64
import io

from fontTools.fontBuilder import FontBuilder
from fontTools.pens.t2CharStringPen import T2CharStringPen
from fontTools.pens.ttGlyphPen import TTGlyphPen
from fontTools.ttLib import newTable
from fontTools.ttLib.tables._k_e_r_n import KernTable_format_0


def common_metrics(fb, family):
    fb.setupHorizontalHeader(ascent=800, descent=-200, lineGap=90)
    fb.setupNameTable({"familyName": family, "styleName": "Regular"})
    fb.setupOS2(
        version=2,
        sTypoAscender=780,
        sTypoDescender=-220,
        sTypoLineGap=100,
        sCapHeight=700,
        sxHeight=500,
    )
    fb.setupPost()


def build_ttf():
    fb = FontBuilder(1000, isTTF=True)
    order = [".notdef", "A", "B", "C"]
    fb.setupGlyphOrder(order)
    fb.setupCharacterMap({0x41: "A", 0x42: "B", 0x43: "C"})

    def glyph_a():  # a triangle: three on-curve points, straight lines
        p = TTGlyphPen(None)
        p.moveTo((100, 0)); p.lineTo((500, 0)); p.lineTo((300, 700)); p.closePath()
        return p.glyph()

    def glyph_b():  # two quadratic curves (off-curve control points)
        p = TTGlyphPen(None)
        p.moveTo((100, 0)); p.lineTo((100, 600))
        p.qCurveTo((450, 600), (450, 300)); p.qCurveTo((450, 0), (100, 0))
        p.closePath()
        return p.glyph()

    def glyph_c():  # composite: glyph A translated right by 200 units
        p = TTGlyphPen(order)
        p.addComponent("A", (1, 0, 0, 1, 200, 0))
        return p.glyph()

    fb.setupGlyf({".notdef": TTGlyphPen(None).glyph(), "A": glyph_a(), "B": glyph_b(), "C": glyph_c()})
    fb.setupHorizontalMetrics({".notdef": (600, 0), "A": (600, 100), "B": (600, 80), "C": (700, 100)})
    common_metrics(fb, "JenFixture")
    # legacy kern table (version 0, format 0): A/B = -50, A/C = -30
    kern = newTable("kern")
    kern.version = 0
    st = KernTable_format_0()
    st.apple = False
    st.coverage = 1
    st.format = 0
    st.kernTable = {("A", "B"): -50, ("A", "C"): -30}
    kern.kernTables = [st]
    fb.font["kern"] = kern
    buf = io.BytesIO()
    fb.save(buf)
    return buf.getvalue()


def build_cff():
    fb = FontBuilder(1000, isTTF=False)
    order = [".notdef", "A", "B"]
    fb.setupGlyphOrder(order)
    fb.setupCharacterMap({0x41: "A", 0x42: "B"})

    def charstring(draw):
        p = T2CharStringPen(600, None)
        draw(p)
        return p.getCharString()

    def glyph_a(p):
        p.moveTo((100, 0)); p.lineTo((500, 0)); p.lineTo((300, 700)); p.closePath()

    def glyph_b(p):  # lines plus two cubic curves
        p.moveTo((100, 0)); p.lineTo((100, 600))
        p.curveTo((450, 600), (450, 300), (450, 150))
        p.curveTo((450, 0), (250, 0), (100, 0))
        p.closePath()

    cs = {".notdef": charstring(lambda p: None), "A": charstring(glyph_a), "B": charstring(glyph_b)}
    fb.setupCFF("JenFixtureCFF", {"FullName": "JenFixtureCFF"}, cs, {})
    fb.setupHorizontalMetrics({".notdef": (600, 0), "A": (600, 100), "B": (600, 100)})
    common_metrics(fb, "JenFixtureCFF")
    buf = io.BytesIO()
    fb.save(buf)
    return buf.getvalue()


ttf = build_ttf()
cff = build_cff()
with open("modules/testdata/font_fixture.ttf", "wb") as f:
    f.write(ttf)
with open("modules/testdata/font_fixture_cff.otf", "wb") as f:
    f.write(cff)
print("wrote modules/testdata/font_fixture.ttf and font_fixture_cff.otf")
print("\nTTF base64 (paste into font_test.j FIXTURE):\n" + base64.b64encode(ttf).decode())
print("\nCFF base64 (paste into font_test.j FIXTURE_CFF):\n" + base64.b64encode(cff).decode())
