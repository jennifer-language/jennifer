#!/usr/bin/env python3
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# Regenerate the two hostile-font fixtures embedded (base64) in
# modules/font_test.j, used to prove the font module's work-budget guards reject
# a malicious font as a fast catchable error instead of hanging:
#
#   FIXTURE_COMPBOMB  - a TrueType font whose composite glyph 'C0' (mapped to 'A')
#                       fans out 16^4 = 65536 components nested only 4 deep, so the
#                       composite DEPTH cap does not catch it - the cumulative
#                       component budget does.
#   FIXTURE_CFFBOMB   - a CFF font whose glyph charstring pushes 58 operands with
#                       no consuming operator, overflowing the Type2 48-entry
#                       operand stack. Built by byte-surgery on the CFF fixture's
#                       CharStrings INDEX (fontTools cannot emit an invalid
#                       charstring, and its minimal CFFs use a layout this parser
#                       does not decode), so we grow one glyph in place and fix the
#                       sfnt table directory.
#
# Needs fontTools (dev-only). Prints each base64; paste into the consts in
# modules/font_test.j (or wire this into a build step). The CFF surgery reads the
# existing FIXTURE_CFF base64 from modules/font_test.j.

import base64
import io
import os
import re
import struct

HERE = os.path.dirname(__file__)
TESTFILE = os.path.join(HERE, "..", "modules", "font_test.j")


def build_compbomb():
    from fontTools.fontBuilder import FontBuilder
    from fontTools.ttLib.tables._g_l_y_f import Glyph, GlyphComponent
    from fontTools.pens.ttGlyphPen import TTGlyphPen

    order = [".notdef", "base", "C0", "C1", "C2", "C3"]
    fb = FontBuilder(1000, isTTF=True)
    fb.setupGlyphOrder(order)
    fb.setupCharacterMap({0x41: "C0"})  # 'A' -> the bomb root

    pen = TTGlyphPen(None)
    pen.moveTo((0, 0))
    pen.lineTo((100, 0))
    pen.lineTo((100, 100))
    pen.lineTo((0, 100))
    pen.closePath()
    base = pen.glyph()

    fan = 16

    def comp(child):
        g = Glyph()
        g.numberOfContours = -1
        g.components = []
        for _ in range(fan):
            c = GlyphComponent()
            c.glyphName = child
            c.flags = 0x0002  # ARGS_ARE_XY_VALUES
            c.x = 0
            c.y = 0
            c.transform = [[1, 0], [0, 1]]
            g.components.append(c)
        return g

    glyphs = {".notdef": Glyph(), "base": base, "C3": comp("base"),
              "C2": comp("C3"), "C1": comp("C2"), "C0": comp("C1")}
    fb.font.recalcBBoxes = False
    fb.font.recalcTimestamp = False
    fb.setupGlyf(glyphs)
    fb.setupHorizontalMetrics({n: (500, 0) for n in order})
    fb.setupHorizontalHeader(ascent=800, descent=-200)
    fb.setupNameTable({"familyName": "CompBomb", "styleName": "Regular"})
    fb.setupOS2()
    fb.setupPost()
    # pin the head timestamps so the fixture is byte-reproducible run to run
    fb.font["head"].created = 0
    fb.font["head"].modified = 0
    out = io.BytesIO()
    fb.font.save(out)
    return out.getvalue()


def build_cffbomb():
    src = open(TESTFILE).read()
    fixcff = re.search(r'FIXTURE_CFF as string init "([^"]+)"', src).group(1)
    data = bytearray(base64.b64decode(fixcff))

    numT = struct.unpack(">H", data[4:6])[0]
    dirs = {}
    for i in range(numT):
        off = 12 + i * 16
        tag = bytes(data[off:off + 4]).decode("latin1")
        _cksum, toff, tlen = struct.unpack(">III", data[off + 4:off + 16])
        dirs[tag] = (off, toff, tlen)
    _cff_dir, cff_off, cff_len = dirs["CFF "]

    # CharStrings INDEX lives at cff+69 in this fixture; it runs to the end of the
    # CFF table (the Private DICT is empty and sits at the boundary).
    cs = cff_off + 69
    old_index = bytes(data[cs:cff_off + cff_len])
    g0 = bytes(data[628:631])   # .notdef
    g1 = bytes(data[631:644])   # glyph 1
    bomb = bytes([0x8B] * 58 + [0x0E])   # 58 push-0 then endchar -> 58 operands (> 48 cap)
    d0, d1, d2 = len(g0), len(g1), len(bomb)
    offs = [1, 1 + d0, 1 + d0 + d1, 1 + d0 + d1 + d2]
    new_index = struct.pack(">H", 3) + bytes([1]) + bytes(offs) + g0 + g1 + bomb
    delta = len(new_index) - len(old_index)
    assert delta % 4 == 0, delta

    nb = bytearray(bytes(data[:cs]) + new_index + bytes(data[cff_off + cff_len:]))
    for tag, (off, toff, tlen) in dirs.items():
        if tag == "CFF ":
            struct.pack_into(">I", nb, off + 12, cff_len + delta)
        elif toff > cff_off:
            struct.pack_into(">I", nb, off + 8, toff + delta)
    return bytes(nb)


def main():
    comp = build_compbomb()
    cff = build_cffbomb()
    print("FIXTURE_COMPBOMB (%d bytes):" % len(comp))
    print(base64.b64encode(comp).decode())
    print()
    print("FIXTURE_CFFBOMB (%d bytes):" % len(cff))
    print(base64.b64encode(cff).decode())


if __name__ == "__main__":
    main()
