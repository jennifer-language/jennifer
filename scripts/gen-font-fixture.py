#!/usr/bin/env python3
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# Generates modules/testdata/font_fixture.ttf: a tiny, fully-controlled TrueType
# font used by the `font` module's tests. Exercises a simple glyph (straight
# lines), a glyph with quadratic curves, and a composite glyph. Reproducible:
# rerun to regenerate. Requires fonttools.
from fontTools.fontBuilder import FontBuilder
from fontTools.pens.ttGlyphPen import TTGlyphPen

fb = FontBuilder(1000, isTTF=True)
order = [".notdef", "A", "B", "C"]
fb.setupGlyphOrder(order)
fb.setupCharacterMap({0x41: "A", 0x42: "B", 0x43: "C"})

def notdef():
    return TTGlyphPen(None).glyph()

def glyphA():  # a triangle: three on-curve points, straight lines
    p = TTGlyphPen(None)
    p.moveTo((100, 0)); p.lineTo((500, 0)); p.lineTo((300, 700)); p.closePath()
    return p.glyph()

def glyphB():  # a shape with two quadratic curves (off-curve control points)
    p = TTGlyphPen(None)
    p.moveTo((100, 0)); p.lineTo((100, 600))
    p.qCurveTo((450, 600), (450, 300))
    p.qCurveTo((450, 0), (100, 0))
    p.closePath()
    return p.glyph()

def glyphC():  # composite: glyph A translated right by 200 units
    p = TTGlyphPen(order)
    p.addComponent("A", (1, 0, 0, 1, 200, 0))
    return p.glyph()

fb.setupGlyf({".notdef": notdef(), "A": glyphA(), "B": glyphB(), "C": glyphC()})
fb.setupHorizontalMetrics({".notdef": (600, 0), "A": (600, 100), "B": (600, 80), "C": (700, 100)})
fb.setupHorizontalHeader(ascent=800, descent=-200)
fb.setupNameTable({"familyName": "JenFixture", "styleName": "Regular"})
fb.setupOS2()
fb.setupPost()
fb.save("modules/testdata/font_fixture.ttf")
print("wrote modules/testdata/font_fixture.ttf")
