# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

# font_demo.j - parse a TrueType font and render a word to an SVG document,
# outlining each glyph and laying them out by advance width. Uses the committed
# test fixture (which contains the glyphs A, B, C); point FONT at a real .ttf and
# change WORD to render anything.
#
#     jennifer run examples/modules/font_demo.j > word.svg
#
# The font stores y pointing up, so the whole word is flipped with an SVG
# transform for screen coordinates.

use io;
use convert;
import "../../modules/font.j" as font;

def const FONT as string init "modules/testdata/font_fixture.ttf";
def const WORD as string init "CAB";

def f as font.Font init font.open(FONT);
def upem as int init font.unitsPerEm($f);

io.printf("<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 -%d %d %d\">\n",
    $upem, $upem * len(strings.chars(WORD)), $upem * 2);
# Flip y (font y-up -> screen y-down) and give the outlines a fill.
io.printf("  <g transform=\"scale(1,-1)\" fill=\"#222\">\n");

def x as int init 0;
def chars as list of string init strings.chars(WORD);
for (def i as int init 0; $i < len($chars); $i = $i + 1) {
    def cp as int init charCode($chars[$i]);
    def d as string init font.glyphPath($f, $cp);
    if (len($d) > 0) {
        io.printf("    <path transform=\"translate(%d,0)\" d=\"%s\"/>\n", $x, $d);
    }
    $x = $x + font.advance($f, $cp);
}
io.printf("  </g>\n</svg>\n");

use strings;

# charCode returns the codepoint of a one-character string (ASCII here).
func charCode(ch as string) {
    def b as bytes init convert.bytesFromString($ch, "utf-8");
    return $b[0];
}
