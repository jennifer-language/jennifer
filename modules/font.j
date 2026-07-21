# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * A pure-Jennifer TrueType / SFNT font parser: read a `.ttf` from `bytes` and
 * expose its metrics, character map, and glyph outlines. No Go, no dependency -
 * just the `bytes` type and the bitwise operators for the big-endian tables - so
 * it runs on **both binaries**.
 *
 * Named `font` (not `truetype`) because the SFNT container also carries CFF /
 * PostScript outlines: v1 ships the **TrueType `glyf` backend**, and a CFF
 * backend can be added later, detected on parse. It parses the core tables -
 * `head`, `cmap` (formats 4 and 12), `maxp` / `hhea` / `hmtx`, `loca` / `glyf`
 * (simple **and** composite glyphs, quadratic curves), and `name` - enough to
 * lay out and outline a string.
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
    family as string
};

# ---- parse ----

/**
 * Parse a TrueType / SFNT font from its bytes.
 * @param b {bytes} the font file contents
 * @return {Font} the parsed font
 * @throws {Error} on a malformed font, or a CFF / unsupported container
 */
export func parse(b as bytes) {
    if (len($b) < 12) {
        throw Error{kind: "font", message: "font.parse: too short to be a font", file: "", line: 0, col: 0};
    }
    def version as int init ulong($b, 0);
    # 0x00010000 (TrueType), or "true" / "ttcf". "OTTO" is CFF (not supported).
    if ($version == 1330926671) {
        throw Error{kind: "font", message: "font.parse: OpenType/CFF fonts (OTTO) are not supported yet; only TrueType glyf outlines", file: "", line: 0, col: 0};
    }
    if ($version != 65536 and $version != 1953658213) {
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
    requireTable($tables, "loca");
    requireTable($tables, "glyf");

    def head as int init $tables["head"];
    def upem as int init ushort($b, $head + 18);
    def longLoca as bool init sshort($b, $head + 50) == 1;
    def numGlyphs as int init ushort($b, $tables["maxp"] + 4);
    def numHMetrics as int init ushort($b, $tables["hhea"] + 34);

    def chosen as list of int init pickCmap($b, $tables["cmap"]);

    return Font{
        data: $b,
        unitsPerEm: $upem,
        numGlyphs: $numGlyphs,
        longLoca: $longLoca,
        numHMetrics: $numHMetrics,
        loca: $tables["loca"],
        glyf: $tables["glyf"],
        hmtx: $tables["hmtx"],
        cmapSub: $chosen[0],
        cmapFmt: $chosen[1],
        family: readFamily($b, $tables)
    };
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

# coverageLookup is the segmented-coverage lookup (full Unicode range).
func coverageLookup(b as bytes, sub as int, cp as int) {
    def nGroups as int init ulong($b, $sub + 12);
    for (def i as int init 0; $i < $nGroups; $i = $i + 1) {
        def g as int init $sub + 16 + $i * 12;
        def startc as int init ulong($b, $g);
        def endc as int init ulong($b, $g + 4);
        if ($cp >= $startc and $cp <= $endc) {
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

# glyphById decodes glyph `gid`. `depth` guards composite recursion.
func glyphById(f as Font, gid as int, depth as int) {
    if ($depth > 8) {
        throw Error{kind: "font", message: "font.glyph: composite nesting too deep", file: "", line: 0, col: 0};
    }
    def adv as int init advanceOf($f, $gid);
    def rng as list of int init glyfRange($f, $gid);
    if ($rng[1] <= $rng[0]) {
        # empty glyph (no outline), e.g. a space
        return Glyph{advance: $adv, xMin: 0, yMin: 0, xMax: 0, yMax: 0, contours: []};
    }
    def g as int init $f.glyf + $rng[0];
    def numContours as int init sshort($f.data, $g);
    def xMin as int init sshort($f.data, $g + 2);
    def yMin as int init sshort($f.data, $g + 4);
    def xMax as int init sshort($f.data, $g + 6);
    def yMax as int init sshort($f.data, $g + 8);
    if ($numContours < 0) {
        def cs as list of Contour init composite($f, $g + 10, $depth);
        return Glyph{advance: $adv, xMin: $xMin, yMin: $yMin, xMax: $xMax, yMax: $yMax, contours: $cs};
    }
    def cs as list of Contour init simpleGlyph($f.data, $g + 10, $numContours);
    return Glyph{advance: $adv, xMin: $xMin, yMin: $yMin, xMax: $xMax, yMax: $yMax, contours: $cs};
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

# composite decodes a composite glyph's components, translating (and scaling)
# each referenced glyph's contours into place.
func composite(f as Font, p as int, depth as int) {
    def out as list of Contour init [];
    def pos as int init $p;
    repeat {
        def flags as int init ushort($f.data, $pos);
        def compGid as int init ushort($f.data, $pos + 2);
        $pos = $pos + 4;
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
        def sub as Glyph init glyphById($f, $compGid, $depth + 1);
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
    return $out;
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
