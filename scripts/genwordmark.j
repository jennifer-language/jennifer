# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# Regenerate the Jennifer wordmark lockups and the mdBook header injector - the
# Jennifer port of scripts/genwordmark.py, dogfooding the `font` module in place
# of Python's fontTools. It outlines the 'jennifer' text to vector paths so the
# result renders identically everywhere, with no font dependency at display time.
# Byte-for-byte identical output to the Python version (the two differ only in
# this generator's own name in the theme/logo.js banner).
#
# Run from the repository root:  jennifer run scripts/genwordmark.j
#
# Requires the DejaVu Sans Mono Bold face (path in FONT below). The committed
# SVGs are the source of truth; run this only when the wordmark design changes.
#
# Outputs:
#   site/logo/jennifer-wordmark.svg       README lockup, ink text (the <img> default)
#   site/logo/jennifer-wordmark-dark.svg  README lockup, paper text (prefers dark)
#   theme/logo.js                         inline header lockup, currentColor text

use io;
use fs;
use convert;
use strings;
use math;
import "../modules/font.j" as font;

def const FONT as string init "/usr/share/fonts/TTF/DejaVuSansMono-Bold.ttf";
def const TEXT as string init "jennifer";
def const FS as float init 62.0;
def const MARK_W as float init 100.0;
def const GAP as float init 30.0;
def const PADR as float init 12.0;
def const CENTER_Y as float init 50.0;

# The monogram tile: a fixed, self-contained mark (same in every variant).
def const MARK as string init "  <rect width=\"100\" height=\"100\" rx=\"26\" fill=\"#1B1420\"/>\n  <g fill=\"none\" stroke=\"#EC5C4B\" stroke-width=\"12\" stroke-linecap=\"round\" stroke-linejoin=\"round\">\n    <path d=\"M35 33 H67\"/>\n    <path d=\"M61 33 V54 C61 70 49 74 39.5 70.5 C33 68 31 61 31.5 56.5\"/>\n  </g>";

# ---- number formatting (matches Python str() / %.Nf) ----

func fmtI(v as int) {
    return convert.toString($v);
}
# midStr formats an implied on-curve midpoint (a+b)/2 as a float, exactly as
# SVGPathPen does via Python str() - "838.0", "745.5", "-316.5".
func midStr(a as int, b as int) {
    def m as float init ($a + $b) / 2;
    return convert.toString($m);
}
func dpOne(v as float) {
    return io.sprintf("%f|prec=1", $v);
}
func dpTwo(v as float) {
    return io.sprintf("%f|prec=2", $v);
}
func dpThree(v as float) {
    return io.sprintf("%f|prec=3", $v);
}
func dpSix(v as float) {
    return io.sprintf("%f|prec=6", $v);
}

# ---- SVGPathPen-compatible path emission ----

# lineSeg renders a straight segment, choosing H (horizontal) / V (vertical) / L,
# and skipping a zero-length move - exactly as SVGPathPen does.
func lineSeg(cx as int, cy as int, x as int, y as int) {
    if ($x == $cx and $y == $cy) {
        return "";
    }
    if ($x == $cx) {
        return "V" + fmtI($y);
    }
    if ($y == $cy) {
        return "H" + fmtI($x);
    }
    return "L" + fmtI($x) + " " + fmtI($y);
}

# qcurveSeg renders a run of off-curve control points ending on-curve at
# (endx, endy): one Q per control, with an implied midpoint between consecutive
# controls.
func qcurveSeg(offs as list of font.Point, endx as int, endy as int) {
    def seg as string init "";
    def m as int init len($offs);
    for (def k as int init 0; $k < $m; $k = $k + 1) {
        def ctrl as font.Point init $offs[$k];
        def mx as string init "";
        def my as string init "";
        if ($k < $m - 1) {
            def nxt as font.Point init $offs[$k + 1];
            $mx = midStr($ctrl.x, $nxt.x);
            $my = midStr($ctrl.y, $nxt.y);
        } else {
            $mx = fmtI($endx);
            $my = fmtI($endy);
        }
        $seg = $seg + "Q" + fmtI($ctrl.x) + " " + fmtI($ctrl.y) + " " + $mx + " " + $my;
    }
    return $seg;
}

# contourSvg renders one contour (starting at point 0, as SVGPathPen does).
func contourSvg(pts as list of font.Point) {
    def n as int init len($pts);
    def pStart as font.Point init $pts[0];
    def d as string init "M" + fmtI($pStart.x) + " " + fmtI($pStart.y);
    def cx as int init $pStart.x;
    def cy as int init $pStart.y;
    def pending as list of font.Point init [];
    for (def i as int init 1; $i < $n; $i = $i + 1) {
        def pt as font.Point init $pts[$i];
        if ($pt.onCurve) {
            if (len($pending) == 0) {
                $d = $d + lineSeg($cx, $cy, $pt.x, $pt.y);
            } else {
                $d = $d + qcurveSeg($pending, $pt.x, $pt.y);
                $pending = [];
            }
            $cx = $pt.x;
            $cy = $pt.y;
        } else {
            $pending[] = $pt;
        }
    }
    if (len($pending) > 0) {
        $d = $d + qcurveSeg($pending, $pStart.x, $pStart.y);
    }
    return $d + "Z";
}

func glyphSvg(gl as font.Glyph) {
    def parts as list of string init [];
    for (def c as int init 0; $c < len($gl.contours); $c = $c + 1) {
        $parts[] = contourSvg($gl.contours[$c].points);
    }
    return strings.join($parts, "");
}

func charCode(ch as string) {
    def b as bytes init convert.bytesFromString($ch, "utf-8");
    return $b[0];
}

# ---- lockup + header templates ----

func lockup(fill as string, note as string, totalW as float, dx as float, dy as float, glyphLines as string) {
    return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
        "<!-- SPDX-License-Identifier: LGPL-3.0-only -->\n" +
        "<!-- Copyright (C) 2026 mplx <jennifer@mplx.dev> -->\n" +
        "<!-- Jennifer wordmark lockup: monogram + 'jennifer' (DejaVu Sans Mono Bold, outlined).\n" +
        "     " + $note + " The mark tile is self-contained; a <picture> element picks the variant. -->\n" +
        "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 " + dpTwo($totalW) + " 100\" width=\"" + dpTwo($totalW) + "\" height=\"100\" role=\"img\" aria-label=\"Jennifer\">\n" +
        "  <title>Jennifer</title>\n" +
        MARK + "\n" +
        "  <g fill=\"" + $fill + "\" transform=\"translate(" + dpThree($dx) + "," + dpThree($dy) + ")\">\n" +
        $glyphLines + "\n" +
        "  </g>\n" +
        "</svg>\n";
}

# ---- main ----

def f as font.Font init font.parse(fs.readBytes(FONT));
def scale as float init FS / font.unitsPerEm($f);

def ds as list of string init [];
def gxs as list of float init [];
def x as float init 0.0;
def xmin as float init 1000000000.0;
def ymin as float init 1000000000.0;
def xmax as float init 0.0 - 1000000000.0;
def ymax as float init 0.0 - 1000000000.0;

def chars as list of string init strings.chars(TEXT);
for (def i as int init 0; $i < len($chars); $i = $i + 1) {
    def cp as int init charCode($chars[$i]);
    def gl as font.Glyph init font.glyph($f, $cp);
    def d as string init glyphSvg($gl);
    if (len($d) > 0) {
        $ds[] = $d;
        $gxs[] = $x;
        $xmin = math.min($xmin, $x + $gl.xMin * $scale);
        $xmax = math.max($xmax, $x + $gl.xMax * $scale);
        $ymin = math.min($ymin, 0.0 - $gl.yMax * $scale);
        $ymax = math.max($ymax, 0.0 - $gl.yMin * $scale);
    }
    $x = $x + $gl.advance * $scale;
}

def dx as float init (MARK_W + GAP) - $xmin;
def dy as float init CENTER_Y - ($ymin + $ymax) / 2;
def wordW as float init $xmax - $xmin;
def totalW as float init MARK_W + GAP + $wordW + PADR;

# The per-glyph <path> lines (indented, newline-joined) and their one-line form.
def indented as list of string init [];
def inline as list of string init [];
for (def i as int init 0; $i < len($ds); $i = $i + 1) {
    def xform as string init "translate(" + dpThree($gxs[$i]) + ",0) scale(" + dpSix($scale) + "," + dpSix(0.0 - $scale) + ")";
    $indented[] = "    <path d=\"" + $ds[$i] + "\" transform=\"" + $xform + "\"/>";
    $inline[] = "<path d=\"" + $ds[$i] + "\" transform=\"" + $xform + "\"/>";
}
def glyphLines as string init strings.join($indented, "\n");

fs.writeString("site/logo/jennifer-wordmark.svg",
    lockup("#241C29", "Ink text, for light backgrounds (the <img> default).", $totalW, $dx, $dy, $glyphLines));
fs.writeString("site/logo/jennifer-wordmark-dark.svg",
    lockup("#F2ECE6", "Paper text, for dark backgrounds (the prefers-color-scheme:dark source).", $totalW, $dx, $dy, $glyphLines));

# --- header variant: text uses currentColor (inherits the mdBook theme) ---
def headerSvg as string init "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 " + dpTwo($totalW) + " 100\" " +
    "role=\"img\" aria-label=\"Jennifer\"><title>Jennifer</title>" +
    "<rect width=\"100\" height=\"100\" rx=\"26\" fill=\"#1B1420\"/>" +
    "<g fill=\"none\" stroke=\"#EC5C4B\" stroke-width=\"12\" stroke-linecap=\"round\" stroke-linejoin=\"round\">" +
    "<path d=\"M35 33 H67\"/>" +
    "<path d=\"M61 33 V54 C61 70 49 74 39.5 70.5 C33 68 31 61 31.5 56.5\"/></g>" +
    "<g fill=\"currentColor\" transform=\"translate(" + dpThree($dx) + "," + dpThree($dy) + ")\">" +
    strings.join($inline, "") + "</g></svg>";

# The SVG is embedded as a single-quoted JS string literal (it contains only
# double quotes and no backslashes, so this matches Python's repr()).
def logoJs as string init "// SPDX-License-Identifier: LGPL-3.0-only\n" +
    "// Copyright (C) 2026 mplx <jennifer@mplx.dev>\n" +
    "//\n" +
    "// Replaces the mdBook menu-bar title with the Jennifer wordmark lockup.\n" +
    "// The 'jennifer' text is currentColor, so it inherits the active mdBook\n" +
    "// theme's foreground and stays legible on light / ayu / coal / navy / rust.\n" +
    "// Registered via book.toml [output.html] additional-js.\n" +
    "// Generated by scripts/genwordmark.j; do not hand-edit.\n" +
    "(function () {\n" +
    "  var SVG = '" + $headerSvg + "';\n" +
    "  function place() {\n" +
    "    var t = document.querySelector('.menu-title');\n" +
    "    if (!t || t.dataset.jlogo) return;\n" +
    "    t.dataset.jlogo = '1';\n" +
    "    t.setAttribute('aria-label', 'Jennifer');\n" +
    "    t.classList.add('has-logo');\n" +
    "    t.innerHTML = SVG;\n" +
    "  }\n" +
    "  if (document.readyState !== 'loading') place();\n" +
    "  else document.addEventListener('DOMContentLoaded', place);\n" +
    "})();\n";
fs.writeString("theme/logo.js", $logoJs);

io.printf("wordmark viewBox width=%s; wrote both wordmark SVGs + theme/logo.js\n", dpOne($totalW));
