# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0

# A hand-rolled SFNT font parser: its glyph decoder legitimately runs past the
# L201 statement-count limit. Every other lint check stays active.
# lint-disable-file: L201

/**
 * A pure-Jennifer TrueType / OpenType (SFNT) font parser: read a `.ttf` / `.otf`
 * from `bytes` and expose its metrics, character map, and glyph outlines. No Go,
 * no dependency - just the `bytes` type and the bitwise operators for the
 * big-endian tables - so it runs on **both binaries**.
 *
 * Both outline backends ship: the **TrueType `glyf`** backend (simple and
 * composite glyphs, quadratic curves) and the **CFF / PostScript** backend for
 * OpenType `OTTO` fonts (a Type2 charstring interpreter with global / local
 * subroutines and CID-keyed FDArray / FDSelect, so CJK fonts outline too),
 * detected on parse; a CFF glyph's cubic curves render as native `C` segments in
 * `glyphPath` and are approximated as quadratics in the `Glyph` struct. It parses
 * the core tables - `head`, `cmap` (formats 4 and 12), `maxp` / `hhea` / `hmtx`,
 * `OS/2` (vertical metrics), the legacy `kern` table, `loca` / `glyf` or `CFF `,
 * and `name`.
 *
 * @module font
 * @example
 * import "font.j" as font;
 * def f as font.Font init font.open("/usr/share/fonts/TTF/DejaVuSans.ttf");
 * io.printf("family %s, %d upem\n", font.name($f), font.unitsPerEm($f));
 * io.printf("<path d=\"%s\"/>\n", font.glyphPath($f, 65));   # the letter 'A'
 */
use fs;
use convert;
use strings;
use maps;
use lists;
include "./font_cff.j";

# ---- big-endian byte readers ----

func ubyte(b as bytes, o as int) {
    return $b[$o];
}
func ushort(b as bytes, o as int) {
    return ($b[$o] << 8) | $b[$o + 1];
}
func ulong(b as bytes, o as int) {
    return ($b[$o] << 24) | ($b[$o + 1] << 16) | ($b[$o + 2] << 8) | $b[$o + 3];
}
func sshort(b as bytes, o as int) {
    def v as int init ushort($b, $o);
    if ($v >= 32768) {
        return $v - 65536;
    }
    return $v;
}
func sbyte(b as bytes, o as int) {
    def v as int init $b[$o];
    if ($v >= 128) {
        return $v - 256;
    }
    return $v;
}
func tag(b as bytes, o as int) {
    def out as bytes;
    $out[] = $b[$o];
    $out[] = $b[$o + 1];
    $out[] = $b[$o + 2];
    $out[] = $b[$o + 3];
    return convert.stringFromBytes($out, "utf-8");
}

# ---- data ----

/**
 * A point in a glyph contour, in font-unit coordinates.
 * @field x {int} the x coordinate
 * @field y {int} the y coordinate
 * @field onCurve {bool} whether the point is on the curve (else a quadratic control point)
 */
export def struct Point {
    x as int,
    y as int,
    onCurve as bool
};

/**
 * One closed contour of a glyph: its ordered points.
 * @field points {list of Point} the contour points
 */
export def struct Contour {
    points as list of Point
};

/**
 * A glyph outline: its contours, advance width, and bounding box, all in font
 * units. An empty glyph (e.g. a space) has no contours.
 * @field advance {int} the horizontal advance width
 * @field xMin {int} the bounding-box minimum x
 * @field yMin {int} the bounding-box minimum y
 * @field xMax {int} the bounding-box maximum x
 * @field yMax {int} the bounding-box maximum y
 * @field contours {list of Contour} the glyph contours
 */
export def struct Glyph {
    advance as int,
    xMin as int,
    yMin as int,
    xMax as int,
    yMax as int,
    contours as list of Contour
};

/**
 * A parsed font. Holds the raw bytes plus the table offsets and header values
 * needed to answer metric / cmap / outline queries; glyph outlines are decoded
 * on demand.
 * @field data {bytes} the raw font file
 * @field unitsPerEm {int} the em square size (the coordinate scale)
 * @field numGlyphs {int} the number of glyphs
 * @field longLoca {bool} whether the loca table uses 32-bit offsets
 * @field numHMetrics {int} the number of long horizontal metrics
 * @field loca {int} the loca table offset
 * @field glyf {int} the glyf table offset
 * @field hmtx {int} the hmtx table offset
 * @field cmapSub {int} the chosen cmap subtable offset
 * @field cmapFmt {int} the chosen cmap subtable format (4 or 12)
 * @field family {string} the font family name
 * @field ascender {int} the typographic ascender (OS/2 sTypoAscender, else hhea)
 * @field descender {int} the typographic descender (negative; OS/2 sTypoDescender, else hhea)
 * @field lineGap {int} the typographic line gap (OS/2 sTypoLineGap, else hhea)
 * @field capHeight {int} the capital height (OS/2 sCapHeight; 0 when unavailable)
 * @field xHeight {int} the x-height (OS/2 sxHeight; 0 when unavailable)
 * @field kern {int} the kern-table offset (0 when the font has no kern table)
 * @field cff {int} the CFF-table offset (0 for a TrueType/glyf font)
 * @field xMin {int} the font bounding-box minimum x (head xMin)
 * @field yMin {int} the font bounding-box minimum y (head yMin)
 * @field xMax {int} the font bounding-box maximum x (head xMax)
 * @field yMax {int} the font bounding-box maximum y (head yMax)
 */
export def struct Font {
    data as bytes,
    unitsPerEm as int,
    numGlyphs as int,
    longLoca as bool,
    numHMetrics as int,
    loca as int,
    glyf as int,
    hmtx as int,
    cmapSub as int,
    cmapFmt as int,
    family as string,
    ascender as int,
    descender as int,
    lineGap as int,
    capHeight as int,
    xHeight as int,
    kern as int,
    cff as int,
    xMin as int,
    yMin as int,
    xMax as int,
    yMax as int
};

# ---- parse ----

/**
 * Parse a TrueType / OpenType (SFNT) font from its bytes - a `glyf` (TrueType)
 * or `CFF ` (OpenType/PostScript) outline font.
 * @param b {bytes} the font file contents
 * @return {Font} the parsed font
 * @throws {Error} on a malformed font or an unrecognised container
 */
export func parse(b as bytes) {
    if (len($b) < 12) {
        throw Error{kind: "font", message: "font.parse: too short to be a font", file: "", line: 0, col: 0};
    }
    def version as int init ulong($b, 0);
    # 0x00010000 / "true" / "ttcf" are TrueType-outline SFNTs; "OTTO" carries CFF
    # (PostScript) outlines.
    def isCff as bool init $version == 1330926671;   # "OTTO"
    if (not ($isCff or $version == 65536 or $version == 1953658213)) {
        throw Error{kind: "font", message: "font.parse: unrecognised sfnt version", file: "", line: 0, col: 0};
    }
    def numTables as int init ushort($b, 4);
    def tables as map of string to int init {};
    for (def i as int init 0; $i < $numTables; $i = $i + 1) {
        def rec as int init 12 + $i * 16;
        $tables[tag($b, $rec)] = ulong($b, $rec + 8);
    }
    requireTable($tables, "head");
    requireTable($tables, "maxp");
    requireTable($tables, "hhea");
    requireTable($tables, "hmtx");
    requireTable($tables, "cmap");
    def cffOff as int init 0;
    def locaOff as int init 0;
    def glyfOff as int init 0;
    if ($isCff) {
        requireTable($tables, "CFF ");
        $cffOff = $tables["CFF "];
    } else {
        requireTable($tables, "loca");
        requireTable($tables, "glyf");
        $locaOff = $tables["loca"];
        $glyfOff = $tables["glyf"];
    }

    def head as int init $tables["head"];
    def upem as int init ushort($b, $head + 18);
    def longLoca as bool init sshort($b, $head + 50) == 1;
    def numGlyphs as int init ushort($b, $tables["maxp"] + 4);
    def numHMetrics as int init ushort($b, $tables["hhea"] + 34);

    def chosen as list of int init pickCmap($b, $tables["cmap"]);
    def vm as list of int init verticalMetrics($b, $tables);
    def kernOff as int init 0;
    if (maps.has($tables, "kern")) {
        $kernOff = $tables["kern"];
    }

    return Font{
        data: $b,
        unitsPerEm: $upem,
        numGlyphs: $numGlyphs,
        longLoca: $longLoca,
        numHMetrics: $numHMetrics,
        loca: $locaOff,
        glyf: $glyfOff,
        hmtx: $tables["hmtx"],
        cmapSub: $chosen[0],
        cmapFmt: $chosen[1],
        family: readFamily($b, $tables),
        ascender: $vm[0],
        descender: $vm[1],
        lineGap: $vm[2],
        capHeight: $vm[3],
        xHeight: $vm[4],
        kern: $kernOff,
        cff: $cffOff,
        xMin: sshort($b, $head + 36),
        yMin: sshort($b, $head + 38),
        xMax: sshort($b, $head + 40),
        yMax: sshort($b, $head + 42)
    };
}

# verticalMetrics returns [ascender, descender, lineGap, capHeight, xHeight]. The
# ascender / descender / lineGap come from OS/2 sTypo* when the OS/2 table is
# present (else the hhea values); capHeight / xHeight need OS/2 version 2+.
func verticalMetrics(b as bytes, tables as map of string to int) {
    def hhea as int init $tables["hhea"];
    def asc as int init sshort($b, $hhea + 4);
    def desc as int init sshort($b, $hhea + 6);
    def gap as int init sshort($b, $hhea + 8);
    def cap as int init 0;
    def xh as int init 0;
    if (maps.has($tables, "OS/2")) {
        def os2 as int init $tables["OS/2"];
        def ver as int init ushort($b, $os2);
        $asc = sshort($b, $os2 + 68);
        $desc = sshort($b, $os2 + 70);
        $gap = sshort($b, $os2 + 72);
        if ($ver >= 2) {
            $xh = sshort($b, $os2 + 86);
            $cap = sshort($b, $os2 + 88);
        }
    }
    return [$asc, $desc, $gap, $cap, $xh];
}

func requireTable(tables as map of string to int, name as string) {
    if (not maps.has($tables, $name)) {
        throw Error{kind: "font", message: "font.parse: missing required table '" + $name + "'", file: "", line: 0, col: 0};
    }
}

/**
 * Load and parse a font from a file path.
 * @param path {string} the .ttf file path
 * @return {Font} the parsed font
 * @throws {Error} on a read or parse error
 */
export func open(path as string) {
    return parse(fs.readBytes($path));
}

# ---- cmap ----

# pickCmap chooses the best Unicode subtable and returns [absOffset, format].
# Preference: Windows format 12 (3,10) > Windows format 4 (3,1) > Unicode (0,*).
func pickCmap(b as bytes, cmap as int) {
    def n as int init ushort($b, $cmap + 2);
    def best as int init -1;
    def bestScore as int init -1;
    for (def i as int init 0; $i < $n; $i = $i + 1) {
        def rec as int init $cmap + 4 + $i * 8;
        def plat as int init ushort($b, $rec);
        def enc as int init ushort($b, $rec + 2);
        def sub as int init $cmap + ulong($b, $rec + 4);
        def score as int init -1;
        if ($plat == 3 and $enc == 10) {
            $score = 4;
        } elseif ($plat == 3 and $enc == 1) {
            $score = 3;
        } elseif ($plat == 0) {
            $score = 2;
        }
        if ($score > $bestScore) {
            $bestScore = $score;
            $best = $sub;
        }
    }
    if ($best < 0) {
        throw Error{kind: "font", message: "font.parse: no supported (Unicode) cmap subtable", file: "", line: 0, col: 0};
    }
    return [$best, ushort($b, $best)];
}

# glyphIndex maps a codepoint to a glyph id via the chosen cmap subtable.
func glyphIndex(f as Font, cp as int) {
    if ($f.cmapFmt == 12) {
        return coverageLookup($f.data, $f.cmapSub, $cp);
    }
    if ($f.cmapFmt == 4) {
        return segmentLookup($f.data, $f.cmapSub, $cp);
    }
    return 0;
}

# segmentLookup is the segment-mapping (BMP) lookup.
func segmentLookup(b as bytes, sub as int, cp as int) {
    if ($cp > 65535) {
        return 0;
    }
    def segBytes as int init ushort($b, $sub + 6);
    def segCount as int init $segBytes // 2;
    def endCodes as int init $sub + 14;
    def startCodes as int init $endCodes + $segBytes + 2;
    def idDeltas as int init $startCodes + $segBytes;
    def idRangeOffsets as int init $idDeltas + $segBytes;
    for (def i as int init 0; $i < $segCount; $i = $i + 1) {
        def endc as int init ushort($b, $endCodes + $i * 2);
        if ($cp <= $endc) {
            def startc as int init ushort($b, $startCodes + $i * 2);
            if ($cp < $startc) {
                return 0;
            }
            def ro as int init ushort($b, $idRangeOffsets + $i * 2);
            if ($ro == 0) {
                return ($cp + sshort($b, $idDeltas + $i * 2)) % 65536;
            }
            # idRangeOffset indexes into the glyphIdArray that follows.
            def gidAddr as int init $idRangeOffsets + $i * 2 + $ro + ($cp - $startc) * 2;
            def gid as int init ushort($b, $gidAddr);
            if ($gid == 0) {
                return 0;
            }
            return ($gid + sshort($b, $idDeltas + $i * 2)) % 65536;
        }
    }
    return 0;
}

# coverageLookup is the segmented-coverage lookup (full Unicode range). The
# groups are sorted by start code, so binary-search them (a CJK font has
# thousands of groups; a linear scan per lookup is far too slow).
func coverageLookup(b as bytes, sub as int, cp as int) {
    def nGroups as int init ulong($b, $sub + 12);
    def lo as int init 0;
    def hi as int init $nGroups - 1;
    while ($lo <= $hi) {
        def mid as int init ($lo + $hi) // 2;
        def g as int init $sub + 16 + $mid * 12;
        def startc as int init ulong($b, $g);
        def endc as int init ulong($b, $g + 4);
        if ($cp < $startc) {
            $hi = $mid - 1;
        } elseif ($cp > $endc) {
            $lo = $mid + 1;
        } else {
            return ulong($b, $g + 8) + ($cp - $startc);
        }
    }
    return 0;
}

# ---- metrics ----

/**
 * The font's units-per-em (the coordinate scale of every outline / metric).
 * @param f {Font} the font
 * @return {int} units per em
 */
export func unitsPerEm(f as Font) {
    return $f.unitsPerEm;
}

/**
 * The font family name.
 * @param f {Font} the font
 * @return {string} the family name
 */
export func name(f as Font) {
    return $f.family;
}

/**
 * The typographic ascender in font units (OS/2 `sTypoAscender`, else the hhea
 * ascent). The distance from the baseline to the top of the em box.
 * @param f {Font} the font
 * @return {int} the ascender
 */
export func ascender(f as Font) {
    return $f.ascender;
}

/**
 * The typographic descender in font units (OS/2 `sTypoDescender`, else hhea).
 * Conventionally negative (below the baseline).
 * @param f {Font} the font
 * @return {int} the descender
 */
export func descender(f as Font) {
    return $f.descender;
}

/**
 * The typographic line gap in font units (OS/2 `sTypoLineGap`, else hhea): the
 * recommended extra leading between lines. Line height = ascender - descender +
 * lineGap.
 * @param f {Font} the font
 * @return {int} the line gap
 */
export func lineGap(f as Font) {
    return $f.lineGap;
}

/**
 * The capital height in font units (OS/2 `sCapHeight`): the height of a flat
 * capital such as `H`. 0 when the font has no OS/2 v2+ table.
 * @param f {Font} the font
 * @return {int} the cap height, or 0 when unavailable
 */
export func capHeight(f as Font) {
    return $f.capHeight;
}

/**
 * The x-height in font units (OS/2 `sxHeight`): the height of a lowercase `x`.
 * 0 when the font has no OS/2 v2+ table.
 * @param f {Font} the font
 * @return {int} the x-height, or 0 when unavailable
 */
export func xHeight(f as Font) {
    return $f.xHeight;
}

# advanceOf reads the horizontal advance of a glyph id from hmtx.
func advanceOf(f as Font, gid as int) {
    if ($gid < $f.numHMetrics) {
        return ushort($f.data, $f.hmtx + $gid * 4);
    }
    return ushort($f.data, $f.hmtx + ($f.numHMetrics - 1) * 4);
}

/**
 * The horizontal advance width of the glyph for a codepoint, in font units.
 * @param f {Font} the font
 * @param cp {int} the Unicode codepoint
 * @return {int} the advance width
 */
export func advance(f as Font, cp as int) {
    return advanceOf($f, glyphIndex($f, $cp));
}

/**
 * The glyph id (index) a codepoint maps to through the font's character map, or 0
 * (`.notdef`) when the font lacks the codepoint. The identifier a PDF embeds
 * under Identity encoding.
 * @param f {Font} the font
 * @param cp {int} the Unicode codepoint
 * @return {int} the glyph id
 */
export func glyphId(f as Font, cp as int) {
    return glyphIndex($f, $cp);
}

/**
 * The horizontal advance width of a glyph id, in font units (the raw hmtx metric,
 * for building a PDF `W` widths array keyed by glyph / CID).
 * @param f {Font} the font
 * @param gid {int} the glyph id
 * @return {int} the advance width
 */
export func advanceGid(f as Font, gid as int) {
    return advanceOf($f, $gid);
}

/**
 * Batch glyph-id lookup: map a list of codepoints to their glyph ids in one call.
 * Prefer this over calling `glyphId` per character - the font is value-semantic
 * (its raw bytes are copied on every call), so a per-character loop copies the
 * whole font each time; this copies it once for the whole batch.
 * @param f {Font} the font
 * @param cps {list of int} the codepoints
 * @return {list of int} the glyph ids, in order
 */
export func glyphIds(f as Font, cps as list of int) {
    # Copy the font bytes ONCE into cmapBatch, which reads them with direct
    # indexing - so the whole batch pays a single font copy, not one per
    # codepoint (each helper call that takes the bytes copies the whole font).
    return cmapBatch($f.data, $f.cmapFmt, $f.cmapSub, $cps);
}

# cmapBatch maps a list of codepoints to glyph ids in one pass, indexing the
# font bytes `d` directly (the format-4 / format-12 lookups inlined so no
# per-codepoint call re-copies the font). Semantics match glyphIndex.
func cmapBatch(d as bytes, fmt as int, sub as int, cps as list of int) {
    def out as list of int init [];
    if ($fmt == 4) {
        def segBytes as int init ($d[$sub + 6] << 8) | $d[$sub + 7];
        def segCount as int init $segBytes // 2;
        def endCodes as int init $sub + 14;
        def startCodes as int init $endCodes + $segBytes + 2;
        def idDeltas as int init $startCodes + $segBytes;
        def idRangeOffsets as int init $idDeltas + $segBytes;
        for (def cp in $cps) {
            def gid as int init 0;
            if ($cp <= 65535) {
                def i as int init 0;
                while ($i < $segCount) {
                    def eo as int init $endCodes + $i * 2;
                    def endc as int init ($d[$eo] << 8) | $d[$eo + 1];
                    if ($cp <= $endc) {
                        def so as int init $startCodes + $i * 2;
                        def startc as int init ($d[$so] << 8) | $d[$so + 1];
                        if ($cp >= $startc) {
                            def roAddr as int init $idRangeOffsets + $i * 2;
                            def ro as int init ($d[$roAddr] << 8) | $d[$roAddr + 1];
                            def dAddr as int init $idDeltas + $i * 2;
                            def delta as int init ($d[$dAddr] << 8) | $d[$dAddr + 1];
                            if ($delta >= 32768) {
                                $delta = $delta - 65536;
                            }
                            if ($ro == 0) {
                                $gid = ($cp + $delta) % 65536;
                            } else {
                                def gAddr as int init $roAddr + $ro + ($cp - $startc) * 2;
                                def raw as int init ($d[$gAddr] << 8) | $d[$gAddr + 1];
                                if ($raw != 0) {
                                    $gid = ($raw + $delta) % 65536;
                                }
                            }
                        }
                        $i = $segCount;
                    } else {
                        $i = $i + 1;
                    }
                }
            }
            $out[] = $gid;
        }
    } elseif ($fmt == 12) {
        def nGroups as int init ($d[$sub + 12] << 24) | ($d[$sub + 13] << 16) |
            ($d[$sub + 14] << 8) | $d[$sub + 15];
        for (def cp in $cps) {
            def gid as int init 0;
            def lo as int init 0;
            def hi as int init $nGroups - 1;
            while ($lo <= $hi) {
                def mid as int init ($lo + $hi) // 2;
                def g as int init $sub + 16 + $mid * 12;
                def startc as int init ($d[$g] << 24) | ($d[$g + 1] << 16) |
                    ($d[$g + 2] << 8) | $d[$g + 3];
                def endc as int init ($d[$g + 4] << 24) | ($d[$g + 5] << 16) |
                    ($d[$g + 6] << 8) | $d[$g + 7];
                if ($cp < $startc) {
                    $hi = $mid - 1;
                } elseif ($cp > $endc) {
                    $lo = $mid + 1;
                } else {
                    $gid = (($d[$g + 8] << 24) | ($d[$g + 9] << 16) | ($d[$g + 10] << 8) |
                        $d[$g + 11]) + ($cp - $startc);
                    $lo = $hi + 1;
                }
            }
            $out[] = $gid;
        }
    } else {
        for (def cp in $cps) {
            $out[] = 0;
        }
    }
    return $out;
}

/**
 * Batch advance-width lookup: the advances (font units) for a list of glyph ids,
 * in one call. Prefer this over `advanceGid` per glyph (see `glyphIds`).
 * @param f {Font} the font
 * @param gids {list of int} the glyph ids
 * @return {list of int} the advance widths, in order
 */
export func advances(f as Font, gids as list of int) {
    return hmtxBatch($f.data, $f.hmtx, $f.numHMetrics, $gids);
}

# hmtxBatch reads advances for a list of glyph ids in one pass, indexing the font
# bytes `d` directly so the batch pays a single font copy. Semantics match
# advanceOf (a gid past numHMetrics shares the last long metric's advance).
func hmtxBatch(d as bytes, hmtx as int, nh as int, gids as list of int) {
    def out as list of int init [];
    for (def gid in $gids) {
        def g as int init $gid;
        if ($g >= $nh) {
            $g = $nh - 1;
        }
        def o as int init $hmtx + $g * 4;
        $out[] = ($d[$o] << 8) | $d[$o + 1];
    }
    return $out;
}

/**
 * The number of glyphs in the font.
 * @param f {Font} the font
 * @return {int} the glyph count
 */
export func numGlyphs(f as Font) {
    return $f.numGlyphs;
}

/**
 * Whether the font uses CFF / PostScript outlines (an OpenType `OTTO`) rather
 * than TrueType `glyf` outlines.
 * @param f {Font} the font
 * @return {bool} true for a CFF font
 */
export func isCff(f as Font) {
    return $f.cff != 0;
}

/**
 * The font bounding box in font units as [xMin, yMin, xMax, yMax] (the `head`
 * table's global glyph bounds, for a PDF `FontBBox`).
 * @param f {Font} the font
 * @return {list of int} [xMin, yMin, xMax, yMax]
 */
export func bbox(f as Font) {
    return [$f.xMin, $f.yMin, $f.xMax, $f.yMax];
}

/**
 * The raw font file bytes (for embedding the font in a container such as a PDF
 * `FontFile2`).
 * @param f {Font} the font
 * @return {bytes} the font file contents
 */
export func data(f as Font) {
    return $f.data;
}

/**
 * The kerning adjustment between two codepoints in font units (negative pulls
 * them closer). Reads the legacy `kern` table (version 0, horizontal format-0
 * subtables); 0 when the font has no `kern` table or no pair entry. Modern fonts
 * carry kerning in GPOS instead, which this does not read.
 * @param f {Font} the font
 * @param left {int} the left codepoint
 * @param right {int} the right codepoint
 * @return {int} the kerning adjustment, or 0
 */
export func kern(f as Font, left as int, right as int) {
    if ($f.kern == 0) {
        return 0;
    }
    def g1 as int init glyphIndex($f, $left);
    def g2 as int init glyphIndex($f, $right);
    if ($g1 == 0 or $g2 == 0) {
        return 0;
    }
    return kernPair($f.data, $f.kern, $g1, $g2);
}

# kernPair sums the horizontal format-0 kerning for a glyph pair across the kern
# table's subtables.
func kernPair(b as bytes, kern as int, leftGid as int, rightGid as int) {
    if (ushort($b, $kern) != 0) {
        return 0;   # Apple 'kern' (version 1.0) layout not supported
    }
    def nTables as int init ushort($b, $kern + 2);
    def pos as int init $kern + 4;
    def total as int init 0;
    for (def t as int init 0; $t < $nTables; $t = $t + 1) {
        def stLen as int init ushort($b, $pos + 2);
        def coverage as int init ushort($b, $pos + 4);
        if (($coverage & 1) != 0 and ($coverage >> 8) == 0) {
            $total = $total + kernFormatZero($b, $pos + 6, $leftGid, $rightGid);
        }
        $pos = $pos + $stLen;
    }
    return $total;
}

# kernFormatZero binary-searches a format-0 subtable's sorted pair array.
func kernFormatZero(b as bytes, p as int, leftGid as int, rightGid as int) {
    def nPairs as int init ushort($b, $p);
    def pairs as int init $p + 8;   # past nPairs / searchRange / entrySelector / rangeShift
    def key as int init ($leftGid << 16) | $rightGid;
    def lo as int init 0;
    def hi as int init $nPairs - 1;
    while ($lo <= $hi) {
        def mid as int init ($lo + $hi) // 2;
        def rec as int init $pairs + $mid * 6;
        def k as int init (ushort($b, $rec) << 16) | ushort($b, $rec + 2);
        if ($k == $key) {
            return sshort($b, $rec + 4);
        }
        if ($k < $key) {
            $lo = $mid + 1;
        } else {
            $hi = $mid - 1;
        }
    }
    return 0;
}

# ---- loca / glyf ----

# glyfRange returns [start, end] byte offsets of a glyph id within the glyf table.
func glyfRange(f as Font, gid as int) {
    if ($f.longLoca) {
        return [ulong($f.data, $f.loca + $gid * 4), ulong($f.data, $f.loca + ($gid + 1) * 4)];
    }
    return [ushort($f.data, $f.loca + $gid * 2) * 2, ushort($f.data, $f.loca + ($gid + 1) * 2) * 2];
}

/**
 * The full outline of the glyph for a codepoint: its contours (on / off-curve
 * points), advance, and bounding box. A codepoint the font lacks maps to glyph 0
 * (`.notdef`).
 * @param f {Font} the font
 * @param cp {int} the Unicode codepoint
 * @return {Glyph} the glyph outline
 */
export func glyph(f as Font, cp as int) {
    def gid as int init glyphIndex($f, $cp);
    return glyphById($f, $gid, 0);
}

# Composite glyphs recurse (a component may itself be composite). The depth cap
# (> 8) bounds nesting but NOT fan-out: a chain of composites each referencing the
# next many times decodes exponentially many components. COMPOSITE_BUDGET caps the
# cumulative component count across one glyph decode - threaded through the
# recursion - so a hostile font is a fast catchable error, not a hang. Real fonts
# use a handful of components a few levels deep, far under this.
def const COMPOSITE_BUDGET as int init 4096;

# DecodeResult / CompResult thread the remaining component budget back out of the
# recursion alongside the decoded value (Jennifer is value-semantic, so a shared
# mutable counter is not available).
def struct DecodeResult {
    glyph as Glyph,
    budget as int
};
def struct CompResult {
    contours as list of Contour,
    budget as int
};

# glyphById decodes glyph `gid`. `depth` guards composite nesting; a fresh
# component budget is seeded here, one per top-level glyph decode.
func glyphById(f as Font, gid as int, depth as int) {
    def r as DecodeResult init decodeGlyphB($f, $gid, $depth, COMPOSITE_BUDGET);
    return $r.glyph;
}

# decodeGlyphB is glyphById's budget-threading worker: it returns the decoded
# glyph plus the component budget left after decoding it.
func decodeGlyphB(f as Font, gid as int, depth as int, budget as int) {
    if ($f.cff != 0) {
        return DecodeResult{glyph: cffGlyphById($f, $gid), budget: $budget};
    }
    if ($depth > 8) {
        throw Error{kind: "font", message: "font.glyph: composite nesting too deep", file: "", line: 0, col: 0};
    }
    def adv as int init advanceOf($f, $gid);
    def rng as list of int init glyfRange($f, $gid);
    if ($rng[1] <= $rng[0]) {
        # empty glyph (no outline), e.g. a space
        return DecodeResult{glyph: Glyph{advance: $adv, xMin: 0, yMin: 0, xMax: 0, yMax: 0, contours: []}, budget: $budget};
    }
    def g as int init $f.glyf + $rng[0];
    def numContours as int init sshort($f.data, $g);
    def xMin as int init sshort($f.data, $g + 2);
    def yMin as int init sshort($f.data, $g + 4);
    def xMax as int init sshort($f.data, $g + 6);
    def yMax as int init sshort($f.data, $g + 8);
    if ($numContours < 0) {
        def cr as CompResult init compositeB($f, $g + 10, $depth, $budget);
        return DecodeResult{glyph: Glyph{advance: $adv, xMin: $xMin, yMin: $yMin, xMax: $xMax, yMax: $yMax, contours: $cr.contours}, budget: $cr.budget};
    }
    def cs as list of Contour init simpleGlyph($f.data, $g + 10, $numContours);
    return DecodeResult{glyph: Glyph{advance: $adv, xMin: $xMin, yMin: $yMin, xMax: $xMax, yMax: $yMax, contours: $cs}, budget: $budget};
}

# simpleGlyph decodes a simple glyph's contours starting at `p` (just past the
# 10-byte glyph header).
func simpleGlyph(b as bytes, p as int, numContours as int) {
    def ends as list of int init [];
    def pos as int init $p;
    for (def i as int init 0; $i < $numContours; $i = $i + 1) {
        $ends[] = ushort($b, $pos);
        $pos = $pos + 2;
    }
    def numPoints as int init 0;
    if ($numContours > 0) {
        $numPoints = $ends[$numContours - 1] + 1;
    }
    # skip instructions
    def instrLen as int init ushort($b, $pos);
    $pos = $pos + 2 + $instrLen;
    # flags (with the repeat encoding)
    def flags as list of int init [];
    repeat {
        if (len($flags) >= $numPoints) {
            break;
        }
        def fl as int init $b[$pos];
        $pos = $pos + 1;
        $flags[] = $fl;
        if (($fl & 8) != 0) {
            def rep as int init $b[$pos];
            $pos = $pos + 1;
            for (def r as int init 0; $r < $rep; $r = $r + 1) {
                if (len($flags) < $numPoints) {
                    $flags[] = $fl;
                }
            }
        }
    } until (false);
    # x coordinates (delta-encoded)
    def xs as list of int init [];
    def x as int init 0;
    for (def i as int init 0; $i < $numPoints; $i = $i + 1) {
        def fl as int init $flags[$i];
        if (($fl & 2) != 0) {
            def dx as int init $b[$pos];
            $pos = $pos + 1;
            if (($fl & 16) == 0) {
                $dx = 0 - $dx;
            }
            $x = $x + $dx;
        } elseif (($fl & 16) == 0) {
            $x = $x + sshort($b, $pos);
            $pos = $pos + 2;
        }
        $xs[] = $x;
    }
    # y coordinates
    def ys as list of int init [];
    def y as int init 0;
    for (def i as int init 0; $i < $numPoints; $i = $i + 1) {
        def fl as int init $flags[$i];
        if (($fl & 4) != 0) {
            def dy as int init $b[$pos];
            $pos = $pos + 1;
            if (($fl & 32) == 0) {
                $dy = 0 - $dy;
            }
            $y = $y + $dy;
        } elseif (($fl & 32) == 0) {
            $y = $y + sshort($b, $pos);
            $pos = $pos + 2;
        }
        $ys[] = $y;
    }
    # split into contours by end indices
    def out as list of Contour init [];
    def startPt as int init 0;
    for (def c as int init 0; $c < $numContours; $c = $c + 1) {
        def pts as list of Point init [];
        for (def i as int init $startPt; $i <= $ends[$c]; $i = $i + 1) {
            $pts[] = Point{x: $xs[$i], y: $ys[$i], onCurve: ($flags[$i] & 1) != 0};
        }
        $out[] = Contour{points: $pts};
        $startPt = $ends[$c] + 1;
    }
    return $out;
}

# compositeB decodes a composite glyph's components, translating (and scaling)
# each referenced glyph's contours into place. It threads the component budget:
# each component decrements it and a sub-decode continues from the remainder, so
# the cumulative component count across the whole recursion is bounded.
func compositeB(f as Font, p as int, depth as int, budget as int) {
    def out as list of Contour init [];
    def pos as int init $p;
    def bud as int init $budget;
    repeat {
        def flags as int init ushort($f.data, $pos);
        def compGid as int init ushort($f.data, $pos + 2);
        $pos = $pos + 4;
        $bud = $bud - 1;
        if ($bud < 0) {
            throw Error{kind: "font", message: "font.glyph: composite exceeds component budget (malformed / hostile font)", file: "", line: 0, col: 0};
        }
        def dx as int init 0;
        def dy as int init 0;
        if (($flags & 1) != 0) {
            # ARG_1_AND_2_ARE_WORDS
            $dx = sshort($f.data, $pos);
            $dy = sshort($f.data, $pos + 2);
            $pos = $pos + 4;
        } else {
            $dx = sbyte($f.data, $pos);
            $dy = sbyte($f.data, $pos + 1);
            $pos = $pos + 2;
        }
        # transform matrix (F2Dot14), default identity
        def a as float init 1.0;
        def bb as float init 0.0;
        def cc as float init 0.0;
        def d as float init 1.0;
        if (($flags & 8) != 0) {
            # WE_HAVE_A_SCALE
            $a = scaleAt($f.data, $pos);
            $d = $a;
            $pos = $pos + 2;
        } elseif (($flags & 64) != 0) {
            # WE_HAVE_AN_X_AND_Y_SCALE
            $a = scaleAt($f.data, $pos);
            $d = scaleAt($f.data, $pos + 2);
            $pos = $pos + 4;
        } elseif (($flags & 128) != 0) {
            # WE_HAVE_A_TWO_BY_TWO
            $a = scaleAt($f.data, $pos);
            $bb = scaleAt($f.data, $pos + 2);
            $cc = scaleAt($f.data, $pos + 4);
            $d = scaleAt($f.data, $pos + 6);
            $pos = $pos + 8;
        }
        def subr as DecodeResult init decodeGlyphB($f, $compGid, $depth + 1, $bud);
        $bud = $subr.budget;
        def sub as Glyph init $subr.glyph;
        for (def ci as int init 0; $ci < len($sub.contours); $ci = $ci + 1) {
            def src as list of Point init $sub.contours[$ci].points;
            def moved as list of Point init [];
            for (def pi as int init 0; $pi < len($src); $pi = $pi + 1) {
                def px as int init $src[$pi].x;
                def py as int init $src[$pi].y;
                def nx as int init round($a * $px + $cc * $py) + $dx;
                def ny as int init round($bb * $px + $d * $py) + $dy;
                $moved[] = Point{x: $nx, y: $ny, onCurve: $src[$pi].onCurve};
            }
            $out[] = Contour{points: $moved};
        }
        if (($flags & 32) == 0) {
            break;
        }
    } until (false);
    return CompResult{contours: $out, budget: $bud};
}

func scaleAt(b as bytes, o as int) {
    return sshort($b, $o) / 16384.0;
}
func round(v as float) {
    if ($v >= 0.0) {
        return convert.toInt($v + 0.5);
    }
    return convert.toInt($v - 0.5);
}

# ---- path ----

/**
 * The glyph outline for a codepoint as an SVG path `d` string, in font-unit
 * coordinates (y-up, as fonts store them - flip y for screen rendering).
 * Quadratic segments render as `Q` commands. An empty glyph yields `""`.
 * @param f {Font} the font
 * @param cp {int} the Unicode codepoint
 * @return {string} the SVG path data
 */
export func glyphPath(f as Font, cp as int) {
    if ($f.cff != 0) {
        return cffGlyphPath($f, glyphIndex($f, $cp));
    }
    def gl as Glyph init glyph($f, $cp);
    def parts as list of string init [];
    for (def c as int init 0; $c < len($gl.contours); $c = $c + 1) {
        $parts[] = contourPath($gl.contours[$c].points);
    }
    return strings.join($parts, " ");
}

func num(n as int) {
    return convert.toString($n);
}

# contourPath renders one contour's on / off-curve points to an SVG subpath.
func contourPath(pts as list of Point) {
    def n as int init len($pts);
    if ($n == 0) {
        return "";
    }
    # Build a working sequence that starts on an on-curve point and returns to it.
    def seq as list of Point init [];
    def startIdx as int init -1;
    for (def i as int init 0; $i < $n; $i = $i + 1) {
        if ($pts[$i].onCurve) {
            $startIdx = $i;
            break;
        }
    }
    if ($startIdx < 0) {
        # all off-curve: synthesize an on-curve start at the midpoint of the
        # last and first control points.
        def mid as Point init Point{x: ($pts[$n - 1].x + $pts[0].x) // 2, y: ($pts[$n - 1].y + $pts[0].y) // 2, onCurve: true};
        $seq[] = $mid;
        for (def i as int init 0; $i < $n; $i = $i + 1) {
            $seq[] = $pts[$i];
        }
        $seq[] = $mid;
    } else {
        for (def k as int init 0; $k < $n; $k = $k + 1) {
            $seq[] = $pts[($startIdx + $k) % $n];
        }
        $seq[] = $pts[$startIdx];
    }
    def d as string init "M " + num($seq[0].x) + " " + num($seq[0].y);
    def i as int init 1;
    def m as int init len($seq);
    repeat {
        if ($i >= $m) {
            break;
        }
        if ($seq[$i].onCurve) {
            $d = $d + " L " + num($seq[$i].x) + " " + num($seq[$i].y);
            $i = $i + 1;
        } else {
            def cx as int init $seq[$i].x;
            def cy as int init $seq[$i].y;
            if ($i + 1 < $m and $seq[$i + 1].onCurve) {
                $d = $d + " Q " + num($cx) + " " + num($cy) + " " + num($seq[$i + 1].x) + " " + num($seq[$i + 1].y);
                $i = $i + 2;
            } else {
                # two consecutive off-curve points: the implied on-curve point is
                # their midpoint.
                def midx as int init ($cx + $seq[$i + 1].x) // 2;
                def midy as int init ($cy + $seq[$i + 1].y) // 2;
                $d = $d + " Q " + num($cx) + " " + num($cy) + " " + num($midx) + " " + num($midy);
                $i = $i + 1;
            }
        }
    } until (false);
    return $d + " Z";
}

# ---- name ----

# readFamily reads name ID 1 (family), preferring a Windows UTF-16BE record.
func readFamily(b as bytes, tables as map of string to int) {
    if (not maps.has($tables, "name")) {
        return "";
    }
    def nm as int init $tables["name"];
    def count as int init ushort($b, $nm + 2);
    def storage as int init $nm + ushort($b, $nm + 4);
    def best as string init "";
    def bestScore as int init -1;
    for (def i as int init 0; $i < $count; $i = $i + 1) {
        def rec as int init $nm + 6 + $i * 12;
        def plat as int init ushort($b, $rec);
        def nameId as int init ushort($b, $rec + 6);
        if ($nameId == 1) {
            def sLen as int init ushort($b, $rec + 8);
            def sOff as int init $storage + ushort($b, $rec + 10);
            def score as int init 0;
            def value as string init "";
            if ($plat == 3 or $plat == 0) {
                $score = 2;
                $value = decodeWide($b, $sOff, $sLen);
            } else {
                $score = 1;
                $value = decodeAscii($b, $sOff, $sLen);
            }
            if ($score > $bestScore) {
                $bestScore = $score;
                $best = $value;
            }
        }
    }
    return $best;
}

func decodeWide(b as bytes, off as int, length as int) {
    def out as bytes;
    def i as int init 0;
    repeat {
        if ($i + 1 >= $length) {
            break;
        }
        def hi as int init $b[$off + $i];
        def lo as int init $b[$off + $i + 1];
        def cp as int init ($hi << 8) | $lo;
        # BMP only (family names are); emit UTF-8.
        $out = appendChar($out, $cp);
        $i = $i + 2;
    } until (false);
    return convert.stringFromBytes($out, "utf-8");
}
func decodeAscii(b as bytes, off as int, length as int) {
    def out as bytes;
    for (def i as int init 0; $i < $length; $i = $i + 1) {
        $out[] = $b[$off + $i];
    }
    return convert.stringFromBytes($out, "utf-8");
}
# appendChar returns out with a BMP codepoint's UTF-8 bytes appended.
func appendChar(out as bytes, cp as int) {
    def b as bytes init $out;
    if ($cp < 128) {
        $b[] = $cp;
    } elseif ($cp < 2048) {
        $b[] = 192 | ($cp >> 6);
        $b[] = 128 | ($cp & 63);
    } else {
        $b[] = 224 | ($cp >> 12);
        $b[] = 128 | (($cp >> 6) & 63);
        $b[] = 128 | ($cp & 63);
    }
    return $b;
}
