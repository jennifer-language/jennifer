# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Generate scannable barcodes as images (not printer commands - the complement
 * to `label`, which emits printer-native barcode commands). `encode(data,
 * symbology, opts) -> Symbol` builds a device-independent representation - a
 * black/white module matrix for 2D codes, a run of bar widths for 1D codes -
 * and the renderers turn a `Symbol` into output: `svg` (resolution-independent,
 * embeds in HTML / email), `png` (a monochrome PNG hand-encoded over `compress`
 * + `crc`, no image library), `terminal` (Unicode half-block art), and `matrix`
 * (the raw cells).
 *
 * Symbologies: 2D `qr` (Reed-Solomon over GF(256), EC levels L/M/Q/H, automatic
 * version selection 1-10 and data-mask scoring, byte mode); 1D `code128`,
 * `ean13`, `ean8`, `itf`, `code39`. Pure `.j` over `compress` (zlib) + `crc`
 * (CRC-32) + `encoding` + the bitwise operators; runs on both binaries.
 * @module barcode
 * @example
 * import "barcode.j" as barcode;
 * def opts as barcode.Options init barcode.defaults();
 * def qr as barcode.Symbol init barcode.encode("https://example.com", "qr", $opts);
 * def svg as string init barcode.svg($qr, $opts);
 * def png as bytes init barcode.png($qr, $opts);
 */
use lists;
use maps;
use strings;
use convert;
use compress;
use crc;
use encoding;
include "./barcode_ecc.j";

/**
 * A device-independent encoded symbol.
 * @field kind {string} "matrix" (2D) or "linear" (1D)
 * @field size {int} the matrix dimension (2D; 0 for 1D)
 * @field matrix {list of list of bool} the 2D module grid (true = dark)
 * @field bars {list of int} 1D bar/space run widths, starting with a bar
 * @field text {string} the encoded data
 */
export def struct Symbol {
    kind as string,
    size as int,
    matrix as list of list of bool,
    bars as list of int,
    text as string
};

/**
 * Rendering options.
 * @field scale {int} pixels (PNG) or units (SVG) per module / narrow bar
 * @field height {int} bar height for 1D codes (module units)
 * @field quiet {int} quiet-zone width in modules / narrow bars
 * @field ecLevel {string} QR error-correction level: "L", "M", "Q", or "H"
 * @field foreground {string} the dark colour (SVG only; PNG is always black), e.g. "#000000"
 * @field background {string} the light colour (SVG only; PNG is always white), e.g. "#ffffff"
 */
export def struct Options {
    scale as int,
    height as int,
    quiet as int,
    ecLevel as string,
    foreground as string,
    background as string
};

func fail(msg as string) {
    throw Error{ kind: "barcode", message: "barcode: " + $msg, file: "", line: 0, col: 0 };
}

# A QR drawing surface (private): the module grid plus a reserved (function
# pattern) mask. Threaded through the placement helpers and returned each time,
# since lists / structs are value-semantic.
def struct Canvas {
    mods as list of list of int,
    reserved as list of list of bool
};

/**
 * Sensible default rendering options (scale 4, quiet 4, EC level M, black on
 * white).
 * @return {Options} the defaults
 */
export func defaults() {
    return Options{ scale: 4, height: 40, quiet: 4, ecLevel: "M", foreground: "#000000", background: "#ffffff" };
}

# --- QR: tables (private) ---------------------------------------------------

# ecLevelBits maps a QR EC level to its 2-bit format value.
func ecLevelBits(level as string) {
    if ($level == "L") { return 1; }
    if ($level == "M") { return 0; }
    if ($level == "Q") { return 3; }
    if ($level == "H") { return 2; }
    fail("unknown QR EC level: " + $level);
}

# blockTable returns the EC block structure keyed "V-L" -> [ecPerBlock,
# g1blocks, g1data, g2blocks, g2data] for versions 1-10.
func blockTable() {
    def t as map of string to list of int init {};
    $t["1-L"] = [7, 1, 19, 0, 0];    $t["1-M"] = [10, 1, 16, 0, 0];
    $t["1-Q"] = [13, 1, 13, 0, 0];   $t["1-H"] = [17, 1, 9, 0, 0];
    $t["2-L"] = [10, 1, 34, 0, 0];   $t["2-M"] = [16, 1, 28, 0, 0];
    $t["2-Q"] = [22, 1, 22, 0, 0];   $t["2-H"] = [28, 1, 16, 0, 0];
    $t["3-L"] = [15, 1, 55, 0, 0];   $t["3-M"] = [26, 1, 44, 0, 0];
    $t["3-Q"] = [18, 2, 17, 0, 0];   $t["3-H"] = [22, 2, 13, 0, 0];
    $t["4-L"] = [20, 1, 80, 0, 0];   $t["4-M"] = [18, 2, 32, 0, 0];
    $t["4-Q"] = [26, 2, 24, 0, 0];   $t["4-H"] = [16, 4, 9, 0, 0];
    $t["5-L"] = [26, 1, 108, 0, 0];  $t["5-M"] = [24, 2, 43, 0, 0];
    $t["5-Q"] = [18, 2, 15, 2, 16];  $t["5-H"] = [22, 2, 11, 2, 12];
    $t["6-L"] = [18, 2, 68, 0, 0];   $t["6-M"] = [16, 4, 27, 0, 0];
    $t["6-Q"] = [24, 4, 19, 0, 0];   $t["6-H"] = [28, 4, 15, 0, 0];
    $t["7-L"] = [20, 2, 78, 0, 0];   $t["7-M"] = [18, 4, 31, 0, 0];
    $t["7-Q"] = [18, 2, 14, 4, 15];  $t["7-H"] = [26, 4, 13, 1, 14];
    $t["8-L"] = [24, 2, 97, 0, 0];   $t["8-M"] = [22, 2, 38, 2, 39];
    $t["8-Q"] = [22, 4, 18, 2, 19];  $t["8-H"] = [26, 4, 14, 2, 15];
    $t["9-L"] = [30, 2, 116, 0, 0];  $t["9-M"] = [22, 3, 36, 2, 37];
    $t["9-Q"] = [20, 4, 16, 4, 17];  $t["9-H"] = [24, 4, 12, 4, 13];
    $t["10-L"] = [18, 2, 68, 2, 69]; $t["10-M"] = [26, 4, 43, 1, 44];
    $t["10-Q"] = [24, 6, 19, 2, 20]; $t["10-H"] = [28, 6, 15, 2, 16];
    return $t;
}

# alignPositions returns the alignment-pattern centre coordinates for a version.
func alignPositions(version as int) {
    if ($version == 1) { return []; }
    if ($version == 2) { return [6, 18]; }
    if ($version == 3) { return [6, 22]; }
    if ($version == 4) { return [6, 26]; }
    if ($version == 5) { return [6, 30]; }
    if ($version == 6) { return [6, 34]; }
    if ($version == 7) { return [6, 22, 38]; }
    if ($version == 8) { return [6, 24, 42]; }
    if ($version == 9) { return [6, 26, 46]; }
    return [6, 28, 50];
}

# totalDataCodewords sums the data codewords across both groups.
func totalDataCodewords(info as list of int) {
    return $info[1] * $info[2] + $info[3] * $info[4];
}

# --- QR: data encoding (private) --------------------------------------------

# selectVersion picks the smallest version (1-10) holding `nbytes` in byte mode.
func selectVersion(nbytes as int, level as string) {
    def t as map of string to list of int init blockTable();
    def v as int init 1;
    while ($v <= 10) {
        def info as list of int init $t[convert.toString($v) + "-" + $level];
        def total as int init totalDataCodewords($info);
        def countBits as int init 8;
        if ($v >= 10) {
            $countBits = 16;
        }
        def capacity as int init ($total * 8 - 4 - $countBits) // 8;
        if ($capacity >= $nbytes) {
            return $v;
        }
        $v = $v + 1;
    }
    fail("data too large for QR versions 1-10 (level " + $level + ")");
}

# pushBits appends the low `count` bits of `value` (MSB first) to a bit list.
func pushBits(bitList as list of int, value as int, count as int) {
    def i as int init $count - 1;
    while ($i >= 0) {
        $bitList[] = ($value >> $i) & 1;
        $i = $i - 1;
    }
    return $bitList;
}

# encodeData builds the padded data codewords for a byte-mode payload.
func encodeData(data as bytes, version as int, level as string) {
    def info as list of int init blockTable()[convert.toString($version) + "-" + $level];
    def total as int init totalDataCodewords($info);
    def totalBits as int init $total * 8;
    def bitList as list of int init [];
    $bitList = pushBits($bitList, 4, 4);   # byte mode indicator 0100
    def countBits as int init 8;
    if ($version >= 10) {
        $countBits = 16;
    }
    $bitList = pushBits($bitList, len($data), $countBits);
    def i as int init 0;
    while ($i < len($data)) {
        $bitList = pushBits($bitList, $data[$i], 8);
        $i = $i + 1;
    }
    # terminator: up to 4 zero bits
    def term as int init 4;
    if ($totalBits - len($bitList) < 4) {
        $term = $totalBits - len($bitList);
    }
    $bitList = pushBits($bitList, 0, $term);
    # pad to a byte boundary
    while (len($bitList) % 8 > 0) {
        $bitList[] = 0;
    }
    # pack bits into codewords
    def cw as list of int init [];
    def b as int init 0;
    while ($b < len($bitList)) {
        def byte as int init 0;
        def k as int init 0;
        while ($k < 8) {
            $byte = ($byte << 1) | $bitList[$b + $k];
            $k = $k + 1;
        }
        $cw[] = $byte;
        $b = $b + 8;
    }
    # pad codewords with 0xEC / 0x11 until full
    def pad as int init 236;
    while (len($cw) < $total) {
        $cw[] = $pad;
        if ($pad == 236) {
            $pad = 17;
        } else {
            $pad = 236;
        }
    }
    return $cw;
}

# interleave splits data codewords into blocks, computes EC per block, and
# interleaves data then EC codewords into the final sequence.
func interleave(cw as list of int, version as int, level as string) {
    def info as list of int init blockTable()[convert.toString($version) + "-" + $level];
    def ecPer as int init $info[0];
    def field as GF init buildGF();
    # split into blocks (group1 then group2), record each block's data + EC
    def dataBlocks as list of list of int init [];
    def ecBlocks as list of list of int init [];
    def pos as int init 0;
    def gi as int init 0;
    while ($gi < 2) {
        def nblocks as int init $info[1];
        def perblock as int init $info[2];
        if ($gi == 1) {
            $nblocks = $info[3];
            $perblock = $info[4];
        }
        def bi as int init 0;
        while ($bi < $nblocks) {
            def block as list of int init [];
            def j as int init 0;
            while ($j < $perblock) {
                $block[] = $cw[$pos];
                $pos = $pos + 1;
                $j = $j + 1;
            }
            $dataBlocks[] = $block;
            $ecBlocks[] = rsEncode($field, $block, $ecPer);
            $bi = $bi + 1;
        }
        $gi = $gi + 1;
    }
    def out as list of int init [];
    # interleave data codewords column by column
    def maxData as int init $info[2];
    if ($info[4] > $maxData) {
        $maxData = $info[4];
    }
    def col as int init 0;
    while ($col < $maxData) {
        def r as int init 0;
        while ($r < len($dataBlocks)) {
            if ($col < len($dataBlocks[$r])) {
                $out[] = $dataBlocks[$r][$col];
            }
            $r = $r + 1;
        }
        $col = $col + 1;
    }
    # interleave EC codewords column by column
    def ec as int init 0;
    while ($ec < $ecPer) {
        def r as int init 0;
        while ($r < len($ecBlocks)) {
            $out[] = $ecBlocks[$r][$ec];
            $r = $r + 1;
        }
        $ec = $ec + 1;
    }
    return $out;
}

# --- QR: matrix construction (private) --------------------------------------

# newGrid builds a size x size grid filled with `value`.
func newGrid(size as int, value as int) {
    def grid as list of list of int init [];
    def r as int init 0;
    while ($r < $size) {
        def row as list of int init [];
        def c as int init 0;
        while ($c < $size) {
            $row[] = $value;
            $c = $c + 1;
        }
        $grid[] = $row;
        $r = $r + 1;
    }
    return $grid;
}

# placeFinder stamps a 7x7 finder pattern with its top-left at (row, col).
func placeFinder(cv as Canvas, row as int, col as int) {
    def r as int init 0;
    while ($r < 7) {
        def c as int init 0;
        while ($c < 7) {
            def dark as bool init ($r == 0 or $r == 6 or $c == 0 or $c == 6 or ($r >= 2 and $r <= 4 and $c >= 2 and $c <= 4));
            def bit as int init 0;
            if ($dark) {
                $bit = 1;
            }
            $cv.mods[$row + $r][$col + $c] = $bit;
            $cv.reserved[$row + $r][$col + $c] = true;
            $c = $c + 1;
        }
        $r = $r + 1;
    }
    return $cv;
}

# reserveArea marks a rectangle reserved (function pattern).
func reserveArea(cv as Canvas, rTop as int, cLeft as int, rBot as int, cRight as int) {
    def n as int init len($cv.mods);
    def r as int init $rTop;
    while ($r <= $rBot) {
        def c as int init $cLeft;
        while ($c <= $cRight) {
            if ($r >= 0 and $c >= 0 and $r < $n and $c < $n) {
                $cv.reserved[$r][$c] = true;
            }
            $c = $c + 1;
        }
        $r = $r + 1;
    }
    return $cv;
}

# placeAlignment stamps a 5x5 alignment pattern centred at (cr, cc).
func placeAlignment(cv as Canvas, cr as int, cc as int) {
    def dr as int init -2;
    while ($dr <= 2) {
        def dc as int init -2;
        while ($dc <= 2) {
            def ar as int init 2;
            if ($dr < 0) { $ar = -$dr; }
            if ($dr > 0) { $ar = $dr; }
            def ac as int init 2;
            if ($dc < 0) { $ac = -$dc; }
            if ($dc > 0) { $ac = $dc; }
            def ring as int init $ar;
            if ($ac > $ring) { $ring = $ac; }
            def bit as int init 0;
            if ($ring == 0 or $ring == 2) {
                $bit = 1;
            }
            $cv.mods[$cr + $dr][$cc + $dc] = $bit;
            $cv.reserved[$cr + $dr][$cc + $dc] = true;
            $dc = $dc + 1;
        }
        $dr = $dr + 1;
    }
    return $cv;
}

# maskBit returns the mask condition for a cell under a given mask pattern.
func maskBit(mask as int, r as int, c as int) {
    if ($mask == 0) { return ($r + $c) % 2 == 0; }
    if ($mask == 1) { return $r % 2 == 0; }
    if ($mask == 2) { return $c % 3 == 0; }
    if ($mask == 3) { return ($r + $c) % 3 == 0; }
    if ($mask == 4) { return ($r // 2 + $c // 3) % 2 == 0; }
    if ($mask == 5) { return ($r * $c) % 2 + ($r * $c) % 3 == 0; }
    if ($mask == 6) { return (($r * $c) % 2 + ($r * $c) % 3) % 2 == 0; }
    return (($r + $c) % 2 + ($r * $c) % 3) % 2 == 0;
}

# formatValue computes the 15-bit format information for (level, mask).
func formatValue(level as string, mask as int) {
    def data as int init (ecLevelBits($level) << 3) | $mask;
    def rem as int init $data << 10;
    def i as int init 14;
    while ($i >= 10) {
        if ((($rem >> $i) & 1) == 1) {
            $rem = $rem ^ (0x537 << ($i - 10));
        }
        $i = $i - 1;
    }
    return (($data << 10) | $rem) ^ 0x5412;
}

# versionValue computes the 18-bit version information (6-bit version + 12-bit
# BCH, generator 0x1f25) for versions 7 and up.
func versionValue(version as int) {
    def rem as int init $version << 12;
    def i as int init 17;
    while ($i >= 12) {
        if ((($rem >> $i) & 1) == 1) {
            $rem = $rem ^ (0x1f25 << ($i - 12));
        }
        $i = $i - 1;
    }
    return ($version << 12) | $rem;
}

# placeVersion reserves and writes the two version-information blocks (only for
# versions >= 7) and returns the updated canvas.
func placeVersion(cv as Canvas, version as int, size as int) {
    if ($version < 7) {
        return $cv;
    }
    def bits as int init versionValue($version);
    def i as int init 0;
    while ($i < 18) {
        def b as int init ($bits >> $i) & 1;
        def a as int init $size - 11 + $i % 3;
        def c as int init $i // 3;
        $cv.mods[$c][$a] = $b;        # top-right block
        $cv.reserved[$c][$a] = true;
        $cv.mods[$a][$c] = $b;        # bottom-left block
        $cv.reserved[$a][$c] = true;
        $i = $i + 1;
    }
    return $cv;
}

# placeFormat writes the 15 format bits (two copies) for (level, mask) and
# returns the updated grid.
func placeFormat(mods as list of list of int, level as string, mask as int, size as int) {
    def bits as int init formatValue($level, $mask);
    # first copy: bits 0-8 down column 8, bits 9-14 leftward along row 8
    def i as int init 0;
    while ($i <= 5) {
        $mods[$i][8] = ($bits >> $i) & 1;
        $i = $i + 1;
    }
    $mods[7][8] = ($bits >> 6) & 1;
    $mods[8][8] = ($bits >> 7) & 1;
    $mods[8][7] = ($bits >> 8) & 1;
    $i = 9;
    while ($i <= 14) {
        $mods[8][14 - $i] = ($bits >> $i) & 1;
        $i = $i + 1;
    }
    # second copy: bits 0-7 rightward along row 8, bits 8-14 up column 8
    $i = 0;
    while ($i <= 7) {
        $mods[8][$size - 1 - $i] = ($bits >> $i) & 1;
        $i = $i + 1;
    }
    $i = 8;
    while ($i <= 14) {
        $mods[$size - 15 + $i][8] = ($bits >> $i) & 1;
        $i = $i + 1;
    }
    $mods[$size - 8][8] = 1;
    return $mods;
}

# buildFunctionPatterns places finders, separators, timing, alignment, and the
# dark module, and marks the format / (unused) version reservation areas.
func buildFunctionPatterns(cv as Canvas, version as int, size as int) {
    $cv = placeFinder($cv, 0, 0);
    $cv = placeFinder($cv, 0, $size - 7);
    $cv = placeFinder($cv, $size - 7, 0);
    # separators (reserved) around the finders
    $cv = reserveArea($cv, 7, 0, 7, 7);
    $cv = reserveArea($cv, 0, 7, 7, 7);
    $cv = reserveArea($cv, 7, $size - 8, 7, $size - 1);
    $cv = reserveArea($cv, 0, $size - 8, 7, $size - 8);
    $cv = reserveArea($cv, $size - 8, 0, $size - 8, 7);
    $cv = reserveArea($cv, $size - 8, 7, $size - 1, 7);
    # timing patterns
    def i as int init 8;
    while ($i < $size - 8) {
        def bit as int init 0;
        if ($i % 2 == 0) {
            $bit = 1;
        }
        $cv.mods[6][$i] = $bit;
        $cv.reserved[6][$i] = true;
        $cv.mods[$i][6] = $bit;
        $cv.reserved[$i][6] = true;
        $i = $i + 1;
    }
    # alignment patterns (skip those overlapping finders)
    def pos as list of int init alignPositions($version);
    def a as int init 0;
    while ($a < len($pos)) {
        def bpos as int init 0;
        while ($bpos < len($pos)) {
            def cr as int init $pos[$a];
            def cc as int init $pos[$bpos];
            def onFinder as bool init ($cr == 6 and $cc == 6) or ($cr == 6 and $cc == $size - 7) or ($cr == $size - 7 and $cc == 6);
            if (not $onFinder) {
                $cv = placeAlignment($cv, $cr, $cc);
            }
            $bpos = $bpos + 1;
        }
        $a = $a + 1;
    }
    # dark module
    $cv.mods[$size - 8][8] = 1;
    $cv.reserved[$size - 8][8] = true;
    # reserve format-info areas
    $cv = reserveArea($cv, 8, 0, 8, 8);
    $cv = reserveArea($cv, 0, 8, 8, 8);
    $cv = reserveArea($cv, $size - 8, 8, $size - 1, 8);
    $cv = reserveArea($cv, 8, $size - 8, 8, $size - 1);
    # version information (versions 7+)
    $cv = placeVersion($cv, $version, $size);
    return $cv;
}

# placeDataBits lays the codeword bit stream into the non-reserved cells in the
# QR zigzag order.
func placeDataBits(cv as Canvas, codewords as list of int, size as int) {
    def bitIdx as int init 0;
    def totalBits as int init len($codewords) * 8;
    def right as int init $size - 1;
    while ($right >= 1) {
        if ($right == 6) {
            $right = 5;
        }
        def vert as int init 0;
        while ($vert < $size) {
            def j as int init 0;
            while ($j < 2) {
                def col as int init $right - $j;
                def upward as bool init (($right + 1) & 2) == 0;
                def row as int init $vert;
                if ($upward) {
                    $row = $size - 1 - $vert;
                }
                if (not $cv.reserved[$row][$col] and $bitIdx < $totalBits) {
                    def byte as int init $codewords[$bitIdx // 8];
                    def bit as int init ($byte >> (7 - ($bitIdx % 8))) & 1;
                    $cv.mods[$row][$col] = $bit;
                    $bitIdx = $bitIdx + 1;
                }
                $j = $j + 1;
            }
            $vert = $vert + 1;
        }
        $right = $right - 2;
    }
    return $cv;
}

# penalty scores a masked matrix (lower is better) per the four QR rules.
func penalty(mods as list of list of int, size as int) {
    def score as int init 0;
    # rule 1: runs of 5+ same colour in rows and columns
    def r as int init 0;
    while ($r < $size) {
        def runV as int init 1;
        def runH as int init 1;
        def c as int init 1;
        while ($c < $size) {
            if ($mods[$r][$c] == $mods[$r][$c - 1]) {
                $runH = $runH + 1;
            } else {
                if ($runH >= 5) { $score = $score + $runH - 2; }
                $runH = 1;
            }
            if ($mods[$c][$r] == $mods[$c - 1][$r]) {
                $runV = $runV + 1;
            } else {
                if ($runV >= 5) { $score = $score + $runV - 2; }
                $runV = 1;
            }
            $c = $c + 1;
        }
        if ($runH >= 5) { $score = $score + $runH - 2; }
        if ($runV >= 5) { $score = $score + $runV - 2; }
        $r = $r + 1;
    }
    # rule 2: 2x2 blocks of one colour
    $r = 0;
    while ($r < $size - 1) {
        def c as int init 0;
        while ($c < $size - 1) {
            def v as int init $mods[$r][$c];
            if ($mods[$r][$c + 1] == $v and $mods[$r + 1][$c] == $v and $mods[$r + 1][$c + 1] == $v) {
                $score = $score + 3;
            }
            $c = $c + 1;
        }
        $r = $r + 1;
    }
    # rule 3: 1:1:3:1:1 finder-like patterns (with 4 light on one side) in rows/cols
    $r = 0;
    while ($r < $size) {
        def c as int init 0;
        while ($c <= $size - 11) {
            if (finderLike($mods, $r, $c, true)) { $score = $score + 40; }
            $c = $c + 1;
        }
        $r = $r + 1;
    }
    $r = 0;
    while ($r <= $size - 11) {
        def c as int init 0;
        while ($c < $size) {
            if (finderLike($mods, $r, $c, false)) { $score = $score + 40; }
            $c = $c + 1;
        }
        $r = $r + 1;
    }
    # rule 4: dark-module proportion deviation from 50%
    def dark as int init 0;
    $r = 0;
    while ($r < $size) {
        def c as int init 0;
        while ($c < $size) {
            if ($mods[$r][$c] == 1) { $dark = $dark + 1; }
            $c = $c + 1;
        }
        $r = $r + 1;
    }
    def total as int init $size * $size;
    def percent as int init ($dark * 100) // $total;
    def dev as int init $percent - 50;
    if ($dev < 0) { $dev = -$dev; }
    $score = $score + ($dev // 5) * 10;
    return $score;
}

# matchesPattern tests an 11-cell pattern at (r,c) along a row or column.
func matchesPattern(mods as list of list of int, r as int, c as int, horizontal as bool, pat as list of int) {
    def i as int init 0;
    while ($i < 11) {
        def v as int init 0;
        if ($horizontal) {
            $v = $mods[$r][$c + $i];
        } else {
            $v = $mods[$r + $i][$c];
        }
        if (not ($v == $pat[$i])) {
            return false;
        }
        $i = $i + 1;
    }
    return true;
}

# finderLike tests the 11-cell 1:1:3:1:1 finder pattern with the 4-cell light run
# on *either* side, per the QR mask rule 3. Testing only the light-run-after form
# would miss half the finder-like occurrences and skew mask selection.
func finderLike(mods as list of list of int, r as int, c as int, horizontal as bool) {
    return matchesPattern($mods, $r, $c, $horizontal, [1, 0, 1, 1, 1, 0, 1, 0, 0, 0, 0])
        or matchesPattern($mods, $r, $c, $horizontal, [0, 0, 0, 0, 1, 0, 1, 1, 1, 0, 1]);
}

# qrMatrix builds the final masked QR module grid for a payload.
func qrMatrix(data as bytes, level as string) {
    def version as int init selectVersion(len($data), $level);
    def size as int init 17 + 4 * $version;
    def codewords as list of int init interleave(encodeData($data, $version, $level), $version, $level);

    def reserved as list of list of bool init [];
    def r as int init 0;
    while ($r < $size) {
        def row as list of bool init [];
        def c as int init 0;
        while ($c < $size) {
            $row[] = false;
            $c = $c + 1;
        }
        $reserved[] = $row;
        $r = $r + 1;
    }
    def cv as Canvas init Canvas{ mods: newGrid($size, 0), reserved: $reserved };
    $cv = buildFunctionPatterns($cv, $version, $size);
    $cv = placeDataBits($cv, $codewords, $size);
    def baseMods as list of list of int init $cv.mods;

    # try all 8 masks, keep the lowest-penalty result
    def bestScore as int init -1;
    def bestMods as list of list of int init $baseMods;
    def mask as int init 0;
    while ($mask < 8) {
        def trial as list of list of int init copyGrid($baseMods);
        def rr as int init 0;
        while ($rr < $size) {
            def cc as int init 0;
            while ($cc < $size) {
                if (not $cv.reserved[$rr][$cc] and maskBit($mask, $rr, $cc)) {
                    $trial[$rr][$cc] = 1 - $trial[$rr][$cc];
                }
                $cc = $cc + 1;
            }
            $rr = $rr + 1;
        }
        $trial = placeFormat($trial, $level, $mask, $size);
        def sc as int init penalty($trial, $size);
        if ($bestScore < 0 or $sc < $bestScore) {
            $bestScore = $sc;
            $bestMods = $trial;
        }
        $mask = $mask + 1;
    }
    return boolGrid($bestMods, $size);
}

# copyGrid deep-copies an int grid.
func copyGrid(grid as list of list of int) {
    def out as list of list of int init [];
    for (def row in $grid) {
        def copy as list of int init [];
        for (def v in $row) {
            $copy[] = $v;
        }
        $out[] = $copy;
    }
    return $out;
}

# boolGrid converts an int (0/1) grid to a bool (dark) grid.
func boolGrid(grid as list of list of int, size as int) {
    def out as list of list of bool init [];
    for (def row in $grid) {
        def brow as list of bool init [];
        for (def v in $row) {
            $brow[] = $v == 1;
        }
        $out[] = $brow;
    }
    return $out;
}

# --- encode dispatch (exported) ---------------------------------------------

/**
 * Encode data as a symbol of the given symbology. 2D: "qr". 1D: "code128",
 * "ean13", "ean8", "itf", "code39".
 * @param data {string} the payload
 * @param symbology {string} the symbology
 * @param opts {Options} rendering / EC options (uses `ecLevel` for QR)
 * @return {Symbol} the encoded symbol
 * @throws {Error} kind "barcode" on an unknown symbology or invalid data
 */
export func encode(data as string, symbology as string, opts as Options) {
    if ($symbology == "qr") {
        def raw as bytes init convert.bytesFromString($data, "utf-8");
        def m as list of list of bool init qrMatrix($raw, $opts.ecLevel);
        def noBars as list of int init [];
        return Symbol{ kind: "matrix", size: len($m), matrix: $m, bars: $noBars, text: $data };
    }
    if ($symbology == "code128") {
        return linearSymbol($data, codeOneTwentyEightBars($data));
    }
    if ($symbology == "code39") {
        return linearSymbol($data, codeThirtyNineBars($data));
    }
    if ($symbology == "ean13") {
        return linearSymbol($data, eanThirteenBars($data));
    }
    if ($symbology == "ean8") {
        return linearSymbol($data, eanEightBars($data));
    }
    if ($symbology == "itf") {
        return linearSymbol($data, itfBars($data));
    }
    fail("unknown symbology: " + $symbology);
}

func linearSymbol(data as string, bars as list of int) {
    def empty as list of list of bool init [];
    return Symbol{ kind: "linear", size: 0, matrix: $empty, bars: $bars, text: $data };
}

# --- 1D symbologies (private) -----------------------------------------------
# Each returns a list of run widths in narrow-bar units, starting with a bar.

# bitsToBars converts a "1010" module string to run-length widths.
func bitsToBars(bitstr as string) {
    def bars as list of int init [];
    def cs as list of string init strings.chars($bitstr);
    def cur as string init "1";
    def run as int init 0;
    for (def ch in $cs) {
        if ($ch == $cur) {
            $run = $run + 1;
        } else {
            $bars[] = $run;
            $cur = $ch;
            $run = 1;
        }
    }
    $bars[] = $run;
    return $bars;
}

# codeOneTwentyEightBars encodes with Code 128 (code set B), auto start / checksum / stop.
func codeOneTwentyEightBars(data as string) {
    def patterns as list of string init codeOneTwentyEightPatterns();
    def cs as list of string init strings.chars($data);
    def values as list of int init [104];   # Start B
    def sum as int init 104;
    def pos as int init 1;
    for (def ch in $cs) {
        def code as int init charToCode($ch);
        if ($code < 0) {
            fail("code128: unsupported character");
        }
        $values[] = $code;
        $sum = $sum + $code * $pos;
        $pos = $pos + 1;
    }
    $values[] = $sum % 103;   # checksum
    $values[] = 106;          # Stop
    def bits as string init "";
    for (def v in $values) {
        $bits = $bits + $patterns[$v];
    }
    $bits = $bits + "11";   # final bar of the stop pattern
    return bitsToBars($bits);
}

# charToCode maps an ASCII char to a Code 128 set-B value (space..~ -> 0..94).
func charToCode(ch as string) {
    def code as int init convert.toCodepoint($ch);
    if ($code >= 32 and $code <= 126) {
        return $code - 32;
    }
    return -1;
}

# codeThirtyNineBars encodes Code 39 (with start / stop `*`, no check digit).
func codeThirtyNineBars(data as string) {
    def bits as string init codeThirtyNineChar("*");
    def cs as list of string init strings.chars(strings.upper($data));
    for (def ch in $cs) {
        # `*` is the Code 39 start/stop delimiter; in the payload it would encode
        # a stop mid-symbol and truncate the scan, so reject it.
        if ($ch == "*") {
            fail("code39: '*' is the start/stop delimiter and cannot appear in the data");
        }
        $bits = $bits + "0" + codeThirtyNineChar($ch);
    }
    $bits = $bits + "0" + codeThirtyNineChar("*");
    return bitsToBars($bits);
}

# eanThirteenBars / eanEightBars / itfBars.
func eanThirteenBars(data as string) {
    return eanBars($data, 13);
}

func eanEightBars(data as string) {
    return eanBars($data, 8);
}

# eanBars encodes EAN-13 (13 digits) or EAN-8 (8 digits); a missing final check
# digit is computed.
func eanBars(data as string, digits as int) {
    def ds as list of int init digitList($data);
    if (len($ds) == $digits - 1) {
        $ds[] = eanCheck($ds, $digits);
    } elseif (len($ds) == $digits) {
        # Full-length input: verify the supplied check digit rather than trusting
        # it, so a mistyped GTIN fails at encode instead of producing a
        # well-formed but unscannable symbol. eanCheck computes over the body
        # digits, so slice off the supplied check digit first.
        def body as list of int init lists.slice($ds, 0, $digits - 1);
        if (not ($ds[$digits - 1] == eanCheck($body, $digits))) {
            fail("ean: check digit mismatch (last digit does not verify)");
        }
    }
    if (not (len($ds) == $digits)) {
        fail("ean: expected " + convert.toString($digits) + " digits");
    }
    if ($digits == 13) {
        return eanThirteenEncode($ds);
    }
    return eanEightEncode($ds);
}

# digitList parses a string of digits into ints.
func digitList(data as string) {
    def out as list of int init [];
    for (def ch in strings.chars($data)) {
        def code as int init convert.toCodepoint($ch);
        if ($code < 48 or $code > 57) {
            fail("expected digits, got '" + $ch + "'");
        }
        $out[] = $code - 48;
    }
    return $out;
}

# eanCheck computes the EAN check digit over the first digits.
func eanCheck(ds as list of int, digits as int) {
    def sum as int init 0;
    def i as int init 0;
    while ($i < len($ds)) {
        def weight as int init 1;
        # for EAN-13 odd positions (0-based even) weight 1, else 3; EAN-8 opposite parity
        def oddThree as bool init ($digits == 13 and $i % 2 == 1) or ($digits == 8 and $i % 2 == 0);
        if ($oddThree) {
            $weight = 3;
        }
        $sum = $sum + $ds[$i] * $weight;
        $i = $i + 1;
    }
    return (10 - ($sum % 10)) % 10;
}

# EAN L / G / R digit patterns (7 modules each).
func eanL(d as int) {
    def p as list of string init ["0001101", "0011001", "0010011", "0111101", "0100011",
        "0110001", "0101111", "0111011", "0110111", "0001011"];
    return $p[$d];
}

func eanG(d as int) {
    def p as list of string init ["0100111", "0110011", "0011011", "0100001", "0011101",
        "0111001", "0000101", "0010001", "0001001", "0010111"];
    return $p[$d];
}

func eanR(d as int) {
    def p as list of string init ["1110010", "1100110", "1101100", "1000010", "1011100",
        "1001110", "1010000", "1000100", "1001000", "1110100"];
    return $p[$d];
}

# eanThirteenEncode builds the module string for 13 digits (first digit sets the
# L/G parity pattern of the left group).
func eanThirteenEncode(ds as list of int) {
    def parity as list of string init ["LLLLLL", "LLGLGG", "LLGGLG", "LLGGGL", "LGLLGG",
        "LGGLLG", "LGGGLL", "LGLGLG", "LGLGGL", "LGGLGL"];
    def pat as string init $parity[$ds[0]];
    def bits as string init "101";   # start guard
    def i as int init 1;
    while ($i <= 6) {
        def ch as string init strings.substring($pat, $i - 1, $i);
        if ($ch == "L") {
            $bits = $bits + eanL($ds[$i]);
        } else {
            $bits = $bits + eanG($ds[$i]);
        }
        $i = $i + 1;
    }
    $bits = $bits + "01010";   # centre guard
    while ($i <= 12) {
        $bits = $bits + eanR($ds[$i]);
        $i = $i + 1;
    }
    $bits = $bits + "101";     # end guard
    return bitsToBars($bits);
}

# eanEightEncode builds the module string for 8 digits (L then R, no parity).
func eanEightEncode(ds as list of int) {
    def bits as string init "101";
    def i as int init 0;
    while ($i < 4) {
        $bits = $bits + eanL($ds[$i]);
        $i = $i + 1;
    }
    $bits = $bits + "01010";
    while ($i < 8) {
        $bits = $bits + eanR($ds[$i]);
        $i = $i + 1;
    }
    $bits = $bits + "101";
    return bitsToBars($bits);
}

# itfBars encodes Interleaved 2 of 5 (an even number of digits).
func itfBars(data as string) {
    def ds as list of int init digitList($data);
    if (not (len($ds) % 2 == 0)) {
        fail("itf: needs an even number of digits");
    }
    # narrow=1 wide=3; patterns are 5 bars, N/W per digit.
    def widths as list of string init ["NNWWN", "WNNNW", "NWNNW", "WWNNN", "NNWNW",
        "WNWNN", "NWWNN", "NNNWW", "WNNWN", "NWNWN"];
    def bars as list of int init [1, 1, 1, 1];   # start: narrow bar/space x2
    def i as int init 0;
    while ($i < len($ds)) {
        def barW as string init $widths[$ds[$i]];
        def spaceW as string init $widths[$ds[$i + 1]];
        def k as int init 0;
        while ($k < 5) {
            $bars[] = itfWidth(strings.substring($barW, $k, $k + 1));
            $bars[] = itfWidth(strings.substring($spaceW, $k, $k + 1));
            $k = $k + 1;
        }
        $i = $i + 2;
    }
    # stop: wide bar, narrow space, narrow bar
    $bars[] = 3;
    $bars[] = 1;
    $bars[] = 1;
    return $bars;
}

func itfWidth(nw as string) {
    if ($nw == "W") {
        return 3;
    }
    return 1;
}

# codeThirtyNineChar returns the 9-element module string for one Code 39 character.
func codeThirtyNineChar(ch as string) {
    def keys as string init "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-. $/+%*";
    def pats as list of string init [
        "101001101101", "110100101011", "101100101011", "110110010101", "101001101011",
        "110100110101", "101100110101", "101001011011", "110100101101", "101100101101",
        "110101001011", "101101001011", "110110100101", "101011001011", "110101100101",
        "101101100101", "101010011011", "110101001101", "101101001101", "101011001101",
        "110101010011", "101101010011", "110110101001", "101011010011", "110101101001",
        "101101101001", "101010110011", "110101011001", "101101011001", "101011011001",
        "110010101011", "100110101011", "110011010101", "100101101011", "110010110101",
        "100110110101", "100101011011", "110010101101", "100110101101", "100100100101",
        "100100101001", "100101001001", "101001001001", "100101101101"];
    def idx as int init strings.indexOf($keys, $ch);
    if ($idx < 0) {
        fail("code39: unsupported character '" + $ch + "'");
    }
    return $pats[$idx];
}

# codeOneTwentyEightPatterns is the Code 128 module-pattern table (values
# 0..106, 11 modules each; the encoder appends the 2-module termination bar
# after the stop pattern, entry 106).
func codeOneTwentyEightPatterns() {
    return ["11011001100", "11001101100", "11001100110", "10010011000", "10010001100",
        "10001001100", "10011001000", "10011000100", "10001100100", "11001001000",
        "11001000100", "11000100100", "10110011100", "10011011100", "10011001110",
        "10111001100", "10011101100", "10011100110", "11001110010", "11001011100",
        "11001001110", "11011100100", "11001110100", "11101101110", "11101001100",
        "11100101100", "11100100110", "11101100100", "11100110100", "11100110010",
        "11011011000", "11011000110", "11000110110", "10100011000", "10001011000",
        "10001000110", "10110001000", "10001101000", "10001100010", "11010001000",
        "11000101000", "11000100010", "10110111000", "10110001110", "10001101110",
        "10111011000", "10111000110", "10001110110", "11101110110", "11010001110",
        "11000101110", "11011101000", "11011100010", "11011101110", "11101011000",
        "11101000110", "11100010110", "11101101000", "11101100010", "11100011010",
        "11101111010", "11001000010", "11110001010", "10100110000", "10100001100",
        "10010110000", "10010000110", "10000101100", "10000100110", "10110010000",
        "10110000100", "10011010000", "10011000010", "10000110100", "10000110010",
        "11000010010", "11001010000", "11110111010", "11000010100", "10001111010",
        "10100111100", "10010111100", "10010011110", "10111100100", "10011110100",
        "10011110010", "11110100100", "11110010100", "11110010010", "11011011110",
        "11011110110", "11110110110", "10101111000", "10100011110", "10001011110",
        "10111101000", "10111100010", "11110101000", "11110100010", "10111011110",
        "10111101110", "11101011110", "11110101110", "11010000100", "11010010000",
        "11010011100", "11000111010"];
}

# --- renderers (exported) ---------------------------------------------------

/**
 * The raw cells of a 2D symbol.
 * @param symbol {Symbol} a matrix symbol
 * @return {list of list of bool} the module grid (true = dark)
 */
export func matrix(symbol as Symbol) {
    if (not ($symbol.kind == "matrix")) {
        fail("matrix: not a 2D symbol");
    }
    return $symbol.matrix;
}

/**
 * Render a symbol as Unicode half-block art for a terminal (2D only).
 * @param symbol {Symbol} a matrix symbol
 * @return {string} the terminal rendering
 */
export func terminal(symbol as Symbol) {
    if (not ($symbol.kind == "matrix")) {
        fail("terminal: 1D symbols are better viewed as an image");
    }
    def m as list of list of bool init $symbol.matrix;
    def n as int init len($m);
    def parts as list of string init [];
    def r as int init 0;
    while ($r < $n) {
        def c as int init 0;
        while ($c < $n) {
            def top as bool init $m[$r][$c];
            def bot as bool init false;
            if ($r + 1 < $n) {
                $bot = $m[$r + 1][$c];
            }
            $parts[] = halfBlock($top, $bot);
            $c = $c + 1;
        }
        $parts[] = "\n";
        $r = $r + 2;
    }
    return strings.join($parts, "");
}

# halfBlock picks the Unicode half-block glyph for a top/bottom cell pair.
# Dark modules are shown with the *light* glyph on a dark terminal by using the
# convention full-block=dark; here dark=space-inverted: dark true -> filled.
func halfBlock(top as bool, bot as bool) {
    if ($top and $bot) {
        return convert.fromCodepoint(0x2588);   # full block
    }
    if ($top) {
        return convert.fromCodepoint(0x2580);   # upper half
    }
    if ($bot) {
        return convert.fromCodepoint(0x2584);   # lower half
    }
    return " ";
}

/**
 * Render a symbol as an SVG string.
 * @param symbol {Symbol} the symbol
 * @param opts {Options} scale / quiet / height / colours
 * @return {string} the SVG document
 */
export func svg(symbol as Symbol, opts as Options) {
    if ($symbol.kind == "matrix") {
        return svgMatrix($symbol, $opts);
    }
    return svgLinear($symbol, $opts);
}

func svgMatrix(symbol as Symbol, opts as Options) {
    def m as list of list of bool init $symbol.matrix;
    def n as int init len($m);
    def s as int init $opts.scale;
    def q as int init $opts.quiet;
    def dim as int init ($n + 2 * $q) * $s;
    # Accumulate rects in a list and join once: an SVG can hold thousands of
    # rects, and a growing `string +` per rect is O(N^2) in the output size.
    def parts as list of string init [];
    $parts[] = svgHeader($dim, $dim, $opts.background);
    def r as int init 0;
    while ($r < $n) {
        def c as int init 0;
        while ($c < $n) {
            if ($m[$r][$c]) {
                $parts[] = svgRect(($c + $q) * $s, ($r + $q) * $s, $s, $s, $opts.foreground);
            }
            $c = $c + 1;
        }
        $r = $r + 1;
    }
    $parts[] = "</svg>\n";
    return strings.join($parts, "");
}

func svgLinear(symbol as Symbol, opts as Options) {
    def s as int init $opts.scale;
    def q as int init $opts.quiet;
    def h as int init $opts.height * $s;
    def totalUnits as int init 0;
    for (def w in $symbol.bars) {
        $totalUnits = $totalUnits + $w;
    }
    def width as int init ($totalUnits + 2 * $q) * $s;
    def height as int init $h + 2 * $q * $s;
    def parts as list of string init [];
    $parts[] = svgHeader($width, $height, $opts.background);
    def x as int init $q * $s;
    def dark as bool init true;
    for (def w in $symbol.bars) {
        if ($dark) {
            $parts[] = svgRect($x, $q * $s, $w * $s, $h, $opts.foreground);
        }
        $x = $x + $w * $s;
        $dark = not $dark;
    }
    $parts[] = "</svg>\n";
    return strings.join($parts, "");
}

func svgHeader(w as int, h as int, bg as string) {
    return "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"" + convert.toString($w) +
        "\" height=\"" + convert.toString($h) + "\" viewBox=\"0 0 " + convert.toString($w) +
        " " + convert.toString($h) + "\">\n<rect width=\"" + convert.toString($w) +
        "\" height=\"" + convert.toString($h) + "\" fill=\"" + $bg + "\"/>\n";
}

func svgRect(x as int, y as int, w as int, h as int, fill as string) {
    return "<rect x=\"" + convert.toString($x) + "\" y=\"" + convert.toString($y) +
        "\" width=\"" + convert.toString($w) + "\" height=\"" + convert.toString($h) +
        "\" fill=\"" + $fill + "\"/>\n";
}

/**
 * Render a symbol as a monochrome (grayscale) PNG.
 * @param symbol {Symbol} the symbol
 * @param opts {Options} scale / quiet / height
 * @return {bytes} the PNG image
 */
export func png(symbol as Symbol, opts as Options) {
    def raster as list of list of int init rasterize($symbol, $opts);
    def height as int init len($raster);
    def width as int init 0;
    if ($height > 0) {
        $width = len($raster[0]);
    }
    # raw image: each row prefixed with filter byte 0, one gray byte per pixel
    def raw as bytes;
    def r as int init 0;
    while ($r < $height) {
        $raw[] = 0;
        def c as int init 0;
        while ($c < $width) {
            $raw[] = $raster[$r][$c];
            $c = $c + 1;
        }
        $r = $r + 1;
    }
    def idat as bytes init compress.pack($raw, "zlib");
    def out as bytes init pngSignature();
    $out = catBytes($out, pngChunk("IHDR", ihdr($width, $height)));
    $out = catBytes($out, pngChunk("IDAT", $idat));
    $out = catBytes($out, pngChunk("IEND", emptyPng()));
    return $out;
}

# rasterize produces the grayscale pixel grid (0 = dark, 255 = light).
func rasterize(symbol as Symbol, opts as Options) {
    def s as int init $opts.scale;
    def q as int init $opts.quiet;
    if ($symbol.kind == "matrix") {
        def m as list of list of bool init $symbol.matrix;
        def n as int init len($m);
        def dim as int init ($n + 2 * $q) * $s;
        def rows as list of list of int init [];
        def py as int init 0;
        while ($py < $dim) {
            def row as list of int init [];
            def px as int init 0;
            while ($px < $dim) {
                def mx as int init $px // $s - $q;
                def my as int init $py // $s - $q;
                def dark as bool init $mx >= 0 and $my >= 0 and $mx < $n and $my < $n and $m[$my][$mx];
                if ($dark) {
                    $row[] = 0;
                } else {
                    $row[] = 255;
                }
                $px = $px + 1;
            }
            $rows[] = $row;
            $py = $py + 1;
        }
        return $rows;
    }
    # linear
    def totalUnits as int init 0;
    for (def w in $symbol.bars) {
        $totalUnits = $totalUnits + $w;
    }
    def width as int init ($totalUnits + 2 * $q) * $s;
    def height as int init $opts.height * $s + 2 * $q * $s;
    # build a per-x dark flag row
    def xdark as list of bool init [];
    def qi as int init 0;
    while ($qi < $q * $s) {
        $xdark[] = false;
        $qi = $qi + 1;
    }
    def dark as bool init true;
    for (def w in $symbol.bars) {
        def k as int init 0;
        while ($k < $w * $s) {
            $xdark[] = $dark;
            $k = $k + 1;
        }
        $dark = not $dark;
    }
    while (len($xdark) < $width) {
        $xdark[] = false;
    }
    def rows as list of list of int init [];
    def py as int init 0;
    while ($py < $height) {
        def inBar as bool init $py >= $q * $s and $py < $height - $q * $s;
        def row as list of int init [];
        def px as int init 0;
        while ($px < $width) {
            if ($inBar and $xdark[$px]) {
                $row[] = 0;
            } else {
                $row[] = 255;
            }
            $px = $px + 1;
        }
        $rows[] = $row;
        $py = $py + 1;
    }
    return $rows;
}

# --- PNG helpers (private) --------------------------------------------------

func emptyPng() {
    def b as bytes;
    return $b;
}

func catBytes(a as bytes, b as bytes) {
    def out as bytes init $a;
    def i as int init 0;
    while ($i < len($b)) {
        $out[] = $b[$i];
        $i = $i + 1;
    }
    return $out;
}

func putLong(b as bytes, v as int) {
    $b[] = ($v >> 24) & 0xff;
    $b[] = ($v >> 16) & 0xff;
    $b[] = ($v >> 8) & 0xff;
    $b[] = $v & 0xff;
    return $b;
}

func pngSignature() {
    def b as bytes;
    $b[] = 137; $b[] = 80; $b[] = 78; $b[] = 71; $b[] = 13; $b[] = 10; $b[] = 26; $b[] = 10;
    return $b;
}

func ihdr(width as int, height as int) {
    def b as bytes;
    $b = putLong($b, $width);
    $b = putLong($b, $height);
    $b[] = 8;    # bit depth
    $b[] = 0;    # colour type: grayscale
    $b[] = 0;    # compression
    $b[] = 0;    # filter
    $b[] = 0;    # interlace
    return $b;
}

# pngChunk builds a length + type + data + CRC-32 chunk.
func pngChunk(typ as string, data as bytes) {
    def typeBytes as bytes init convert.bytesFromString($typ, "utf-8");
    def out as bytes;
    $out = putLong($out, len($data));
    $out = catBytes($out, $typeBytes);
    $out = catBytes($out, $data);
    def crcInput as bytes init catBytes($typeBytes, $data);
    $out = catBytes($out, crc.compute($crcInput, "crc32"));
    return $out;
}
