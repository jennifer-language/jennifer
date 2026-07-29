# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# font_cff.j - the CFF (Compact Font Format) / Type2 charstring backend for the
# font module, included (textual splice) into font.j. It parses the `CFF ` table
# of an OpenType `OTTO` font - its INDEX / DICT structures, global and local
# subroutines, and (for CID-keyed fonts) the FDArray / FDSelect - and interprets
# each glyph's Type2 charstring into an outline. CFF outlines are cubic; each
# cubic is split into two quadratics so the result fits font.j's quadratic
# `Glyph` model and renders through the same `glyphPath`. Private to the module;
# it declares no `use` of its own and relies on font.j's imports.

# The parsed CFF context: table offsets discovered once per glyph query.
def struct Cff {
    charStrings as int,     # CharStrings INDEX offset
    gsubr as int,           # global subr INDEX offset
    lsubr as int,           # local subr INDEX offset (non-CID; 0 when none)
    isCID as bool,
    fdselect as int,        # FDSelect offset (CID)
    fdarray as int          # FDArray INDEX offset (CID)
};

# slong reads a signed 32-bit big-endian integer.
func slong(b as bytes, o as int) {
    def v as int init ulong($b, $o);
    if ($v >= 2147483648) {
        return $v - 4294967296;
    }
    return $v;
}

# readOffN reads a `size`-byte (1..4) big-endian unsigned integer.
func readOffN(b as bytes, o as int, size as int) {
    def v as int init 0;
    for (def i as int init 0; $i < $size; $i = $i + 1) {
        $v = ($v << 8) | $b[$o + $i];
    }
    return $v;
}

# cffIndex parses a CFF INDEX at `off`, returning the list of [start, end] byte
# ranges of its entries. An empty INDEX yields [].
func cffIndex(b as bytes, off as int) {
    def count as int init ushort($b, $off);
    if ($count == 0) {
        return [];
    }
    def offSize as int init $b[$off + 2];
    def offArr as int init $off + 3;
    def dataBase as int init $offArr + ($count + 1) * $offSize - 1;
    def ranges as list of list of int init [];
    for (def i as int init 0; $i < $count; $i = $i + 1) {
        def s as int init $dataBase + readOffN($b, $offArr + $i * $offSize, $offSize);
        def e as int init $dataBase + readOffN($b, $offArr + ($i + 1) * $offSize, $offSize);
        $ranges[] = [$s, $e];
    }
    return $ranges;
}

# cffIndexCount returns an INDEX's entry count (O(1)).
func cffIndexCount(b as bytes, off as int) {
    return ushort($b, $off);
}

# cffIndexEntry returns the [start, end] byte range of a single INDEX entry
# without materialising the whole INDEX - O(1), so a glyph query on a large
# (e.g. CJK, 65k-glyph) CharStrings INDEX does not walk every entry.
func cffIndexEntry(b as bytes, off as int, i as int) {
    def count as int init ushort($b, $off);
    def offSize as int init $b[$off + 2];
    def offArr as int init $off + 3;
    def dataBase as int init $offArr + ($count + 1) * $offSize - 1;
    def s as int init $dataBase + readOffN($b, $offArr + $i * $offSize, $offSize);
    def e as int init $dataBase + readOffN($b, $offArr + ($i + 1) * $offSize, $offSize);
    return [$s, $e];
}

# cffIndexEnd returns the byte offset just past an INDEX (to reach the next one).
func cffIndexEnd(b as bytes, off as int) {
    def count as int init ushort($b, $off);
    if ($count == 0) {
        return $off + 2;
    }
    def offSize as int init $b[$off + 2];
    def offArr as int init $off + 3;
    def dataBase as int init $offArr + ($count + 1) * $offSize - 1;
    return $dataBase + readOffN($b, $offArr + $count * $offSize, $offSize);
}

# parseDict parses a CFF DICT in [start, end) into a map from operator key to its
# operand list. A two-byte operator `12 x` is keyed `1200 + x`. Real operands
# (which our operators never need) are recorded as 0.
func parseDict(b as bytes, start as int, end as int) {
    def ops as map of int to list of int init {};
    def stack as list of int init [];
    def pos as int init $start;
    while ($pos < $end) {
        def b0 as int init $b[$pos];
        if ($b0 <= 21) {
            def key as int init $b0;
            $pos = $pos + 1;
            if ($b0 == 12) {
                $key = 1200 + $b[$pos];
                $pos = $pos + 1;
            }
            $ops[$key] = $stack;
            $stack = [];
        } elseif ($b0 == 28) {
            $stack[] = sshort($b, $pos + 1);
            $pos = $pos + 3;
        } elseif ($b0 == 29) {
            $stack[] = slong($b, $pos + 1);
            $pos = $pos + 5;
        } elseif ($b0 == 30) {
            # real number: consume nibbles until one is 0xf
            $pos = $pos + 1;
            def going as bool init true;
            while ($going) {
                def by as int init $b[$pos];
                $pos = $pos + 1;
                if ((($by >> 4) & 0xf) == 0xf or ($by & 0xf) == 0xf) {
                    $going = false;
                }
            }
            $stack[] = 0;
        } elseif ($b0 <= 246) {
            $stack[] = $b0 - 139;
            $pos = $pos + 1;
        } elseif ($b0 <= 250) {
            $stack[] = ($b0 - 247) * 256 + $b[$pos + 1] + 108;
            $pos = $pos + 2;
        } else {
            $stack[] = -($b0 - 251) * 256 - $b[$pos + 1] - 108;
            $pos = $pos + 2;
        }
    }
    return $ops;
}

# dictOp returns operand `idx` of a DICT operator, or `fallback` when absent.
func dictOp(ops as map of int to list of int, key as int, idx as int, fallback as int) {
    if (maps.has($ops, $key) and $idx < len($ops[$key])) {
        return $ops[$key][$idx];
    }
    return $fallback;
}

# subrBias is the Type2 subroutine-number bias for a subr count.
func subrBias(count as int) {
    if ($count < 1240) {
        return 107;
    }
    if ($count < 33900) {
        return 1131;
    }
    return 32768;
}

# cffContext parses the CFF header and top-level structures of font `f`.
func cffContext(f as Font) {
    def b as bytes init $f.data;
    def cff as int init $f.cff;
    def p as int init $cff + $b[$cff + 2];   # skip header (hdrSize at +2)
    $p = cffIndexEnd($b, $p);                # Name INDEX
    def topStart as int init $p;
    def topRanges as list of list of int init cffIndex($b, $p);
    $p = cffIndexEnd($b, $p);                # Top DICT INDEX
    $p = cffIndexEnd($b, $p);                # String INDEX
    def gsubr as int init $p;                # Global Subr INDEX
    def top as map of int to list of int init parseDict($b, $topRanges[0][0], $topRanges[0][1]);
    def charStrings as int init $cff + dictOp($top, 17, 0, 0);
    def isCID as bool init maps.has($top, 1230);   # ROS operator marks a CID font
    def lsubr as int init 0;
    if (maps.has($top, 18)) {
        def privSize as int init $top[18][0];
        def privOff as int init $cff + $top[18][1];
        def priv as map of int to list of int init parseDict($b, $privOff, $privOff + $privSize);
        if (maps.has($priv, 19)) {
            $lsubr = $privOff + $priv[19][0];
        }
    }
    def fdselect as int init 0;
    def fdarray as int init 0;
    if ($isCID) {
        $fdarray = $cff + dictOp($top, 1236, 0, 0);
        $fdselect = $cff + dictOp($top, 1237, 0, 0);
    }
    return Cff{
        charStrings: $charStrings,
        gsubr: $gsubr,
        lsubr: $lsubr,
        isCID: $isCID,
        fdselect: $fdselect,
        fdarray: $fdarray
    };
}

# fdSelectLookup maps a glyph id to its font-DICT index (FDSelect format 0 / 3).
func fdSelectLookup(b as bytes, off as int, gid as int) {
    def fmt as int init $b[$off];
    if ($fmt == 0) {
        return $b[$off + 1 + $gid];
    }
    if ($fmt == 3) {
        def nRanges as int init ushort($b, $off + 1);
        def base as int init $off + 3;
        for (def i as int init 0; $i < $nRanges; $i = $i + 1) {
            def first as int init ushort($b, $base + $i * 3);
            def next as int init ushort($b, $base + ($i + 1) * 3);
            if ($gid >= $first and $gid < $next) {
                return $b[$base + $i * 3 + 2];
            }
        }
    }
    return 0;
}

# cffLocalSubr returns the local-subr INDEX offset for a glyph (the shared subrs
# for a non-CID font, or the FD-specific subrs for a CID font; 0 when none).
func cffLocalSubr(f as Font, ctx as Cff, gid as int) {
    if (not $ctx.isCID) {
        return $ctx.lsubr;
    }
    def b as bytes init $f.data;
    def fd as int init fdSelectLookup($b, $ctx.fdselect, $gid);
    def fdRanges as list of list of int init cffIndex($b, $ctx.fdarray);
    if ($fd >= len($fdRanges)) {
        return 0;
    }
    def fdDict as map of int to list of int init parseDict($b, $fdRanges[$fd][0], $fdRanges[$fd][1]);
    if (not maps.has($fdDict, 18)) {
        return 0;
    }
    def privOff as int init $f.cff + $fdDict[18][1];
    def priv as map of int to list of int init parseDict($b, $privOff, $privOff + $fdDict[18][0]);
    if (maps.has($priv, 19)) {
        return $privOff + $priv[19][0];
    }
    return 0;
}

# cffGlyphById decodes CFF glyph `gid` into a Glyph (quadratic outline).
func cffGlyphById(f as Font, gid as int) {
    def adv as int init advanceOf($f, $gid);
    def ctx as Cff init cffContext($f);
    def contours as list of list of list of int init runCharstring($f, $ctx, $gid);
    def out as list of Contour init [];
    def minx as int init 0;
    def miny as int init 0;
    def maxx as int init 0;
    def maxy as int init 0;
    def seen as bool init false;
    for (def c as int init 0; $c < len($contours); $c = $c + 1) {
        def cmds as list of list of int init $contours[$c];
        def pts as list of Point init [];
        def px as int init 0;
        def py as int init 0;
        for (def i as int init 0; $i < len($cmds); $i = $i + 1) {
            def cmd as list of int init $cmds[$i];
            if ($cmd[0] == 0) {
                $px = $cmd[1];
                $py = $cmd[2];
                $pts[] = Point{x: $px, y: $py, onCurve: true};
            } elseif ($cmd[0] == 1) {
                $px = $cmd[1];
                $py = $cmd[2];
                $pts[] = Point{x: $px, y: $py, onCurve: true};
            } else {
                # cubic (px,py)->(cmd5,cmd6) via controls (cmd1,cmd2),(cmd3,cmd4):
                # split into two quadratics so it fits the on/off-curve model.
                $pts = appendCubicAsQuads($pts, $px, $py, $cmd[1], $cmd[2], $cmd[3], $cmd[4], $cmd[5], $cmd[6]);
                $px = $cmd[5];
                $py = $cmd[6];
            }
        }
        if (len($pts) > 0) {
            $out[] = Contour{points: $pts};
            for (def k as int init 0; $k < len($pts); $k = $k + 1) {
                if (not $seen or $pts[$k].x < $minx) { $minx = $pts[$k].x; }
                if (not $seen or $pts[$k].y < $miny) { $miny = $pts[$k].y; }
                if (not $seen or $pts[$k].x > $maxx) { $maxx = $pts[$k].x; }
                if (not $seen or $pts[$k].y > $maxy) { $maxy = $pts[$k].y; }
                $seen = true;
            }
        }
    }
    return Glyph{advance: $adv, xMin: $minx, yMin: $miny, xMax: $maxx, yMax: $maxy, contours: $out};
}

# appendCubicAsQuads splits a cubic Bezier into two quadratics and appends their
# off-curve control + on-curve end points.
func appendCubicAsQuads(pts as list of Point, p0x as int, p0y as int, c1x as int, c1y as int, c2x as int, c2y as int, p3x as int, p3y as int) {
    def out as list of Point init $pts;
    # de Casteljau split at t = 0.5
    def m0x as int init ($p0x + $c1x) // 2;
    def m0y as int init ($p0y + $c1y) // 2;
    def m1x as int init ($c1x + $c2x) // 2;
    def m1y as int init ($c1y + $c2y) // 2;
    def m2x as int init ($c2x + $p3x) // 2;
    def m2y as int init ($c2y + $p3y) // 2;
    def q0x as int init ($m0x + $m1x) // 2;
    def q0y as int init ($m0y + $m1y) // 2;
    def q1x as int init ($m1x + $m2x) // 2;
    def q1y as int init ($m1y + $m2y) // 2;
    def midx as int init ($q0x + $q1x) // 2;
    def midy as int init ($q0y + $q1y) // 2;
    # quadratic control of half-cubic (P0,m0,q0,mid) = (3*m0 - P0 + 3*q0 - mid)/4
    def qax as int init (3 * $m0x - $p0x + 3 * $q0x - $midx) // 4;
    def qay as int init (3 * $m0y - $p0y + 3 * $q0y - $midy) // 4;
    $out[] = Point{x: $qax, y: $qay, onCurve: false};
    $out[] = Point{x: $midx, y: $midy, onCurve: true};
    def qbx as int init (3 * $q1x - $midx + 3 * $m2x - $p3x) // 4;
    def qby as int init (3 * $q1y - $midy + 3 * $m2y - $p3y) // 4;
    $out[] = Point{x: $qbx, y: $qby, onCurve: false};
    $out[] = Point{x: $p3x, y: $p3y, onCurve: true};
    return $out;
}

# runCharstring interprets a glyph's Type2 charstring, returning its contours as
# a list of command lists ([0,x,y]=moveto, [1,x,y]=lineto, [2,..]=curveto).
func runCharstring(f as Font, ctx as Cff, gid as int) {
    def b as bytes init $f.data;
    if ($gid >= cffIndexCount($b, $ctx.charStrings)) {
        return [];
    }
    def csRange as list of int init cffIndexEntry($b, $ctx.charStrings, $gid);
    # Read subr entries on demand (O(1) each) rather than materialising the whole
    # subr INDEX, which a CJK font can make very large.
    def gsubrOff as int init $ctx.gsubr;
    def gsubrCount as int init cffIndexCount($b, $gsubrOff);
    def gBias as int init subrBias($gsubrCount);
    def lsubrOff as int init cffLocalSubr($f, $ctx, $gid);
    def lsubrCount as int init 0;
    if ($lsubrOff > 0) {
        $lsubrCount = cffIndexCount($b, $lsubrOff);
    }
    def lBias as int init subrBias($lsubrCount);

    def contours as list of list of list of int init [];
    def cur as list of list of int init [];
    def stack as list of int init [];
    def x as int init 0;
    def y as int init 0;
    def open as bool init false;
    def nStems as int init 0;
    def haveWidth as bool init false;
    def done as bool init false;
    def frames as list of list of int init [[$csRange[1], $csRange[0]]];

    repeat {
        if (len($frames) == 0 or $done) {
            break;
        }
        def ti as int init len($frames) - 1;
        def fend as int init $frames[$ti][0];
        def pos as int init $frames[$ti][1];
        if ($pos >= $fend) {
            $frames = lists.slice($frames, 0, $ti);
            continue;
        }
        def b0 as int init $b[$pos];

        if ($b0 == 28) {
            $stack[] = sshort($b, $pos + 1);
            $frames[$ti][1] = $pos + 3;
            continue;
        }
        if ($b0 == 255) {
            def fx as int init slong($b, $pos + 1);
            $stack[] = ($fx + 32768) // 65536;   # 16.16 fixed, rounded to int
            $frames[$ti][1] = $pos + 5;
            continue;
        }
        if ($b0 >= 32) {
            if ($b0 <= 246) {
                $stack[] = $b0 - 139;
                $frames[$ti][1] = $pos + 1;
            } elseif ($b0 <= 250) {
                $stack[] = ($b0 - 247) * 256 + $b[$pos + 1] + 108;
                $frames[$ti][1] = $pos + 2;
            } else {
                $stack[] = -($b0 - 251) * 256 - $b[$pos + 1] - 108;
                $frames[$ti][1] = $pos + 2;
            }
            continue;
        }

        # --- operators ---
        def ns as int init len($stack);
        if ($b0 == 1 or $b0 == 3 or $b0 == 18 or $b0 == 23) {
            # hstem / vstem / hstemhm / vstemhm
            if (not $haveWidth and ($ns % 2) == 1) {
                $stack = lists.slice($stack, 1, $ns);
            }
            $haveWidth = true;
            $nStems = $nStems + len($stack) // 2;
            $stack = [];
            $frames[$ti][1] = $pos + 1;
        } elseif ($b0 == 19 or $b0 == 20) {
            # hintmask / cntrmask: absorb implicit vstem args, then skip mask bytes
            if (not $haveWidth and ($ns % 2) == 1) {
                $stack = lists.slice($stack, 1, $ns);
            }
            $haveWidth = true;
            $nStems = $nStems + len($stack) // 2;
            $stack = [];
            $frames[$ti][1] = $pos + 1 + ($nStems + 7) // 8;
        } elseif ($b0 == 21) {
            # rmoveto
            def a as list of int init $stack;
            if (not $haveWidth and len($a) > 2) {
                $a = lists.slice($a, 1, len($a));
            }
            $haveWidth = true;
            if ($open) { $contours[] = $cur; }
            $cur = [];
            $x = $x + $a[0];
            $y = $y + $a[1];
            $cur[] = [0, $x, $y];
            $open = true;
            $stack = [];
            $frames[$ti][1] = $pos + 1;
        } elseif ($b0 == 22 or $b0 == 4) {
            # hmoveto / vmoveto
            def a as list of int init $stack;
            if (not $haveWidth and len($a) > 1) {
                $a = lists.slice($a, 1, len($a));
            }
            $haveWidth = true;
            if ($open) { $contours[] = $cur; }
            $cur = [];
            if ($b0 == 22) {
                $x = $x + $a[0];
            } else {
                $y = $y + $a[0];
            }
            $cur[] = [0, $x, $y];
            $open = true;
            $stack = [];
            $frames[$ti][1] = $pos + 1;
        } elseif ($b0 == 5) {
            # rlineto
            def i as int init 0;
            while ($i + 2 <= len($stack)) {
                $x = $x + $stack[$i];
                $y = $y + $stack[$i + 1];
                $cur[] = [1, $x, $y];
                $i = $i + 2;
            }
            $stack = [];
            $frames[$ti][1] = $pos + 1;
        } elseif ($b0 == 6 or $b0 == 7) {
            # hlineto / vlineto: alternating lines
            def horiz as bool init $b0 == 6;
            def i as int init 0;
            while ($i < len($stack)) {
                if ($horiz) {
                    $x = $x + $stack[$i];
                } else {
                    $y = $y + $stack[$i];
                }
                $cur[] = [1, $x, $y];
                $horiz = not $horiz;
                $i = $i + 1;
            }
            $stack = [];
            $frames[$ti][1] = $pos + 1;
        } elseif ($b0 == 8) {
            # rrcurveto
            def i as int init 0;
            while ($i + 6 <= len($stack)) {
                def c1x as int init $x + $stack[$i];
                def c1y as int init $y + $stack[$i + 1];
                def c2x as int init $c1x + $stack[$i + 2];
                def c2y as int init $c1y + $stack[$i + 3];
                $x = $c2x + $stack[$i + 4];
                $y = $c2y + $stack[$i + 5];
                $cur[] = [2, $c1x, $c1y, $c2x, $c2y, $x, $y];
                $i = $i + 6;
            }
            $stack = [];
            $frames[$ti][1] = $pos + 1;
        } elseif ($b0 == 24) {
            # rcurveline: curves then a final line
            def i as int init 0;
            while ($i + 6 <= len($stack) - 2) {
                def c1x as int init $x + $stack[$i];
                def c1y as int init $y + $stack[$i + 1];
                def c2x as int init $c1x + $stack[$i + 2];
                def c2y as int init $c1y + $stack[$i + 3];
                $x = $c2x + $stack[$i + 4];
                $y = $c2y + $stack[$i + 5];
                $cur[] = [2, $c1x, $c1y, $c2x, $c2y, $x, $y];
                $i = $i + 6;
            }
            $x = $x + $stack[$i];
            $y = $y + $stack[$i + 1];
            $cur[] = [1, $x, $y];
            $stack = [];
            $frames[$ti][1] = $pos + 1;
        } elseif ($b0 == 25) {
            # rlinecurve: lines then a final curve
            def i as int init 0;
            while ($i + 2 <= len($stack) - 6) {
                $x = $x + $stack[$i];
                $y = $y + $stack[$i + 1];
                $cur[] = [1, $x, $y];
                $i = $i + 2;
            }
            def c1x as int init $x + $stack[$i];
            def c1y as int init $y + $stack[$i + 1];
            def c2x as int init $c1x + $stack[$i + 2];
            def c2y as int init $c1y + $stack[$i + 3];
            $x = $c2x + $stack[$i + 4];
            $y = $c2y + $stack[$i + 5];
            $cur[] = [2, $c1x, $c1y, $c2x, $c2y, $x, $y];
            $stack = [];
            $frames[$ti][1] = $pos + 1;
        } elseif ($b0 == 26 or $b0 == 27) {
            # vvcurveto (26) / hhcurveto (27)
            def i as int init 0;
            def d1 as int init 0;
            if ((len($stack) % 4) == 1) {
                $d1 = $stack[0];
                $i = 1;
            }
            while ($i + 4 <= len($stack)) {
                def c1x as int init 0;
                def c1y as int init 0;
                if ($b0 == 26) {
                    $c1x = $x + $d1;
                    $c1y = $y + $stack[$i];
                } else {
                    $c1x = $x + $stack[$i];
                    $c1y = $y + $d1;
                }
                def c2x as int init $c1x + $stack[$i + 1];
                def c2y as int init $c1y + $stack[$i + 2];
                if ($b0 == 26) {
                    $x = $c2x;
                    $y = $c2y + $stack[$i + 3];
                } else {
                    $x = $c2x + $stack[$i + 3];
                    $y = $c2y;
                }
                $cur[] = [2, $c1x, $c1y, $c2x, $c2y, $x, $y];
                $d1 = 0;
                $i = $i + 4;
            }
            $stack = [];
            $frames[$ti][1] = $pos + 1;
        } elseif ($b0 == 30 or $b0 == 31) {
            # vhcurveto (30) / hvcurveto (31): alternating curves
            def horiz as bool init $b0 == 31;
            def i as int init 0;
            def n as int init len($stack);
            while ($i + 4 <= $n) {
                def last5 as bool init ($n - $i) == 5;
                def c1x as int init 0;
                def c1y as int init 0;
                def c2x as int init 0;
                def c2y as int init 0;
                if ($horiz) {
                    $c1x = $x + $stack[$i];
                    $c1y = $y;
                    $c2x = $c1x + $stack[$i + 1];
                    $c2y = $c1y + $stack[$i + 2];
                    $y = $c2y + $stack[$i + 3];
                    if ($last5) {
                        $x = $c2x + $stack[$i + 4];
                    } else {
                        $x = $c2x;
                    }
                } else {
                    $c1x = $x;
                    $c1y = $y + $stack[$i];
                    $c2x = $c1x + $stack[$i + 1];
                    $c2y = $c1y + $stack[$i + 2];
                    $x = $c2x + $stack[$i + 3];
                    if ($last5) {
                        $y = $c2y + $stack[$i + 4];
                    } else {
                        $y = $c2y;
                    }
                }
                $cur[] = [2, $c1x, $c1y, $c2x, $c2y, $x, $y];
                $horiz = not $horiz;
                $i = $i + 4;
            }
            $stack = [];
            $frames[$ti][1] = $pos + 1;
        } elseif ($b0 == 10 or $b0 == 29) {
            # callsubr (local) / callgsubr (global)
            def idx as int init $stack[len($stack) - 1];
            $stack = lists.slice($stack, 0, len($stack) - 1);
            $frames[$ti][1] = $pos + 1;
            def subOff as int init $lsubrOff;
            def subCount as int init $lsubrCount;
            def bias as int init $lBias;
            if ($b0 == 29) {
                $subOff = $gsubrOff;
                $subCount = $gsubrCount;
                $bias = $gBias;
            }
            def si as int init $idx + $bias;
            if ($si >= 0 and $si < $subCount) {
                # Bound subroutine nesting: the Type2 limit is 10, so a chain far
                # past that is a malformed (or hostile) font trying to make the
                # frame stack grow without end. Reject rather than hang.
                if (len($frames) >= 64) {
                    throw Error{kind: "font", message: "font.glyph: charstring subroutine nesting too deep", file: "", line: 0, col: 0};
                }
                def r as list of int init cffIndexEntry($b, $subOff, $si);
                $frames[] = [$r[1], $r[0]];
            }
        } elseif ($b0 == 11) {
            # return
            $frames = lists.slice($frames, 0, $ti);
        } elseif ($b0 == 14) {
            # endchar (widths / seac ignored - outlines only)
            $done = true;
        } elseif ($b0 == 12) {
            # escape operators: flex family produce two curves
            def op2 as int init $b[$pos + 1];
            $frames[$ti][1] = $pos + 2;
            if ($op2 == 34) {
                # hflex
                def c1x as int init $x + $stack[0];
                def c1y as int init $y;
                def c2x as int init $c1x + $stack[1];
                def c2y as int init $c1y + $stack[2];
                def e1x as int init $c2x + $stack[3];
                def e1y as int init $c2y;
                $cur[] = [2, $c1x, $c1y, $c2x, $c2y, $e1x, $e1y];
                def c3x as int init $e1x + $stack[4];
                def c3y as int init $e1y;
                def c4x as int init $c3x + $stack[5];
                def c4y as int init $c1y;
                $x = $c4x + $stack[6];
                $y = $c1y;
                $cur[] = [2, $c3x, $c3y, $c4x, $c4y, $x, $y];
            } elseif ($op2 == 35) {
                # flex
                def c1x as int init $x + $stack[0];
                def c1y as int init $y + $stack[1];
                def c2x as int init $c1x + $stack[2];
                def c2y as int init $c1y + $stack[3];
                def e1x as int init $c2x + $stack[4];
                def e1y as int init $c2y + $stack[5];
                $cur[] = [2, $c1x, $c1y, $c2x, $c2y, $e1x, $e1y];
                def c3x as int init $e1x + $stack[6];
                def c3y as int init $e1y + $stack[7];
                def c4x as int init $c3x + $stack[8];
                def c4y as int init $c3y + $stack[9];
                $x = $c4x + $stack[10];
                $y = $c4y + $stack[11];
                $cur[] = [2, $c3x, $c3y, $c4x, $c4y, $x, $y];
            } elseif ($op2 == 36) {
                # hflex1
                def sy as int init $y;
                def c1x as int init $x + $stack[0];
                def c1y as int init $y + $stack[1];
                def c2x as int init $c1x + $stack[2];
                def c2y as int init $c1y + $stack[3];
                def e1x as int init $c2x + $stack[4];
                def e1y as int init $c2y;
                $cur[] = [2, $c1x, $c1y, $c2x, $c2y, $e1x, $e1y];
                def c3x as int init $e1x + $stack[5];
                def c3y as int init $e1y;
                def c4x as int init $c3x + $stack[6];
                def c4y as int init $c3y + $stack[7];
                $x = $c4x + $stack[8];
                $y = $sy;
                $cur[] = [2, $c3x, $c3y, $c4x, $c4y, $x, $y];
            } elseif ($op2 == 37) {
                # flex1
                def sx as int init $x;
                def sy as int init $y;
                def c1x as int init $x + $stack[0];
                def c1y as int init $y + $stack[1];
                def c2x as int init $c1x + $stack[2];
                def c2y as int init $c1y + $stack[3];
                def e1x as int init $c2x + $stack[4];
                def e1y as int init $c2y + $stack[5];
                $cur[] = [2, $c1x, $c1y, $c2x, $c2y, $e1x, $e1y];
                def c3x as int init $e1x + $stack[6];
                def c3y as int init $e1y + $stack[7];
                def c4x as int init $c3x + $stack[8];
                def c4y as int init $c3y + $stack[9];
                def dx as int init 0;
                def dy as int init 0;
                def k as int init 0;
                while ($k < 11) {
                    if (($k % 2) == 0) {
                        $dx = $dx + $stack[$k];
                    } else {
                        $dy = $dy + $stack[$k];
                    }
                    $k = $k + 1;
                }
                if (absInt($dx) > absInt($dy)) {
                    $x = $c4x + $stack[10];
                    $y = $sy;
                } else {
                    $x = $sx;
                    $y = $c4y + $stack[10];
                }
                $cur[] = [2, $c3x, $c3y, $c4x, $c4y, $x, $y];
            }
            $stack = [];
        } else {
            # any other operator: clear the stack and move on
            $stack = [];
            $frames[$ti][1] = $pos + 1;
        }
    } until (false);

    if ($open) {
        $contours[] = $cur;
    }
    return $contours;
}

# absInt is the absolute value of an int.
func absInt(v as int) {
    if ($v < 0) {
        return 0 - $v;
    }
    return $v;
}

# cffGlyphPath renders CFF glyph `gid` as an SVG path with native cubic (`C`)
# segments - the accurate outline, without the quadratic approximation `glyph`
# applies to fit its point model.
func cffGlyphPath(f as Font, gid as int) {
    def ctx as Cff init cffContext($f);
    def contours as list of list of list of int init runCharstring($f, $ctx, $gid);
    def parts as list of string init [];
    for (def c as int init 0; $c < len($contours); $c = $c + 1) {
        def cmds as list of list of int init $contours[$c];
        def d as string init "";
        for (def i as int init 0; $i < len($cmds); $i = $i + 1) {
            def cmd as list of int init $cmds[$i];
            if ($cmd[0] == 0) {
                $d = "M " + num($cmd[1]) + " " + num($cmd[2]);
            } elseif ($cmd[0] == 1) {
                $d = $d + " L " + num($cmd[1]) + " " + num($cmd[2]);
            } else {
                $d = $d + " C " + num($cmd[1]) + " " + num($cmd[2]) + " " + num($cmd[3]) + " " +
                    num($cmd[4]) + " " + num($cmd[5]) + " " + num($cmd[6]);
            }
        }
        if (len($d) > 0) {
            $parts[] = $d + " Z";
        }
    }
    return strings.join($parts, " ");
}
