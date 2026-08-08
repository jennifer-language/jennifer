# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

# A hand-rolled barcode encoder (QR / Reed-Solomon) plus renderers: its
# table-building methods legitimately run past the L201 statement-count limit.
# Every other lint check stays active.
# lint-disable-file: L201

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
 * version selection 1-40, data-mask scoring, and numeric / alphanumeric / byte
 * mode chosen for compactness) and `datamatrix` (ECC200, square symbols 10x10 to
 * 26x26); 1D `code128`, `code93`, `ean13`, `ean8`, `upca`, `upce`, `itf`,
 * `code39`, and `gs1-128` (FNC1 + Application Identifiers). A 1D SVG also carries
 * a human-readable text line (Options.humanReadable). Pure `.j` over `compress`
 * (zlib) + `crc` (CRC-32) + `encoding` + the bitwise operators; runs on both
 * binaries.
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
 * The kind of an encoded symbol: `Matrix` (a 2D module grid, e.g. QR) or
 * `Linear` (a 1D run of bar / space widths).
 */
export def enum SymbolKind { Matrix, Linear };

/**
 * A device-independent encoded symbol.
 * @field kind {SymbolKind} `Matrix` (2D) or `Linear` (1D)
 * @field size {int} the matrix dimension (2D; 0 for 1D)
 * @field matrix {list of list of bool} the 2D module grid (true = dark)
 * @field bars {list of int} 1D bar/space run widths, starting with a bar
 * @field text {string} the encoded data
 */
export def struct Symbol {
    kind as SymbolKind,
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
 * @field humanReadable {bool} render the data as a text line under a 1D barcode (SVG only; on by default)
 */
export def struct Options {
    scale as int,
    height as int,
    quiet as int,
    ecLevel as string,
    foreground as string,
    background as string,
    humanReadable as bool
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
 * Sensible default rendering options (scale 8, quiet 4, EC level M, black on
 * white). Scale 8 (8 px per module / narrow bar) keeps a code comfortably
 * scannable off a screen by a phone camera, where a smaller render invites focus
 * and moire trouble; drop `opts.scale` for a more compact image.
 * @return {Options} the defaults
 */
export func defaults() {
    return Options{ scale: 8, height: 40, quiet: 4, ecLevel: "M", foreground: "#000000", background: "#ffffff", humanReadable: true };
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
# g1blocks, g1data, g2blocks, g2data] for versions 1-40 (all four EC levels).
func blockTable() {
    def t as map of string to list of int init {};
    $t["1-L"] = [7, 1, 19, 0, 0]; $t["1-M"] = [10, 1, 16, 0, 0]; $t["1-Q"] = [13, 1, 13, 0, 0]; $t["1-H"] = [17, 1, 9, 0, 0];
    $t["2-L"] = [10, 1, 34, 0, 0]; $t["2-M"] = [16, 1, 28, 0, 0]; $t["2-Q"] = [22, 1, 22, 0, 0]; $t["2-H"] = [28, 1, 16, 0, 0];
    $t["3-L"] = [15, 1, 55, 0, 0]; $t["3-M"] = [26, 1, 44, 0, 0]; $t["3-Q"] = [18, 2, 17, 0, 0]; $t["3-H"] = [22, 2, 13, 0, 0];
    $t["4-L"] = [20, 1, 80, 0, 0]; $t["4-M"] = [18, 2, 32, 0, 0]; $t["4-Q"] = [26, 2, 24, 0, 0]; $t["4-H"] = [16, 4, 9, 0, 0];
    $t["5-L"] = [26, 1, 108, 0, 0]; $t["5-M"] = [24, 2, 43, 0, 0]; $t["5-Q"] = [18, 2, 15, 2, 16]; $t["5-H"] = [22, 2, 11, 2, 12];
    $t["6-L"] = [18, 2, 68, 0, 0]; $t["6-M"] = [16, 4, 27, 0, 0]; $t["6-Q"] = [24, 4, 19, 0, 0]; $t["6-H"] = [28, 4, 15, 0, 0];
    $t["7-L"] = [20, 2, 78, 0, 0]; $t["7-M"] = [18, 4, 31, 0, 0]; $t["7-Q"] = [18, 2, 14, 4, 15]; $t["7-H"] = [26, 4, 13, 1, 14];
    $t["8-L"] = [24, 2, 97, 0, 0]; $t["8-M"] = [22, 2, 38, 2, 39]; $t["8-Q"] = [22, 4, 18, 2, 19]; $t["8-H"] = [26, 4, 14, 2, 15];
    $t["9-L"] = [30, 2, 116, 0, 0]; $t["9-M"] = [22, 3, 36, 2, 37]; $t["9-Q"] = [20, 4, 16, 4, 17]; $t["9-H"] = [24, 4, 12, 4, 13];
    $t["10-L"] = [18, 2, 68, 2, 69]; $t["10-M"] = [26, 4, 43, 1, 44]; $t["10-Q"] = [24, 6, 19, 2, 20]; $t["10-H"] = [28, 6, 15, 2, 16];
    $t["11-L"] = [20, 4, 81, 0, 0]; $t["11-M"] = [30, 1, 50, 4, 51]; $t["11-Q"] = [28, 4, 22, 4, 23]; $t["11-H"] = [24, 3, 12, 8, 13];
    $t["12-L"] = [24, 2, 92, 2, 93]; $t["12-M"] = [22, 6, 36, 2, 37]; $t["12-Q"] = [26, 4, 20, 6, 21]; $t["12-H"] = [28, 7, 14, 4, 15];
    $t["13-L"] = [26, 4, 107, 0, 0]; $t["13-M"] = [22, 8, 37, 1, 38]; $t["13-Q"] = [24, 8, 20, 4, 21]; $t["13-H"] = [22, 12, 11, 4, 12];
    $t["14-L"] = [30, 3, 115, 1, 116]; $t["14-M"] = [24, 4, 40, 5, 41]; $t["14-Q"] = [20, 11, 16, 5, 17]; $t["14-H"] = [24, 11, 12, 5, 13];
    $t["15-L"] = [22, 5, 87, 1, 88]; $t["15-M"] = [24, 5, 41, 5, 42]; $t["15-Q"] = [30, 5, 24, 7, 25]; $t["15-H"] = [24, 11, 12, 7, 13];
    $t["16-L"] = [24, 5, 98, 1, 99]; $t["16-M"] = [28, 7, 45, 3, 46]; $t["16-Q"] = [24, 15, 19, 2, 20]; $t["16-H"] = [30, 3, 15, 13, 16];
    $t["17-L"] = [28, 1, 107, 5, 108]; $t["17-M"] = [28, 10, 46, 1, 47]; $t["17-Q"] = [28, 1, 22, 15, 23]; $t["17-H"] = [28, 2, 14, 17, 15];
    $t["18-L"] = [30, 5, 120, 1, 121]; $t["18-M"] = [26, 9, 43, 4, 44]; $t["18-Q"] = [28, 17, 22, 1, 23]; $t["18-H"] = [28, 2, 14, 19, 15];
    $t["19-L"] = [28, 3, 113, 4, 114]; $t["19-M"] = [26, 3, 44, 11, 45]; $t["19-Q"] = [26, 17, 21, 4, 22]; $t["19-H"] = [26, 9, 13, 16, 14];
    $t["20-L"] = [28, 3, 107, 5, 108]; $t["20-M"] = [26, 3, 41, 13, 42]; $t["20-Q"] = [30, 15, 24, 5, 25]; $t["20-H"] = [28, 15, 15, 10, 16];
    $t["21-L"] = [28, 4, 116, 4, 117]; $t["21-M"] = [26, 17, 42, 0, 0]; $t["21-Q"] = [28, 17, 22, 6, 23]; $t["21-H"] = [30, 19, 16, 6, 17];
    $t["22-L"] = [28, 2, 111, 7, 112]; $t["22-M"] = [28, 17, 46, 0, 0]; $t["22-Q"] = [30, 7, 24, 16, 25]; $t["22-H"] = [24, 34, 13, 0, 0];
    $t["23-L"] = [30, 4, 121, 5, 122]; $t["23-M"] = [28, 4, 47, 14, 48]; $t["23-Q"] = [30, 11, 24, 14, 25]; $t["23-H"] = [30, 16, 15, 14, 16];
    $t["24-L"] = [30, 6, 117, 4, 118]; $t["24-M"] = [28, 6, 45, 14, 46]; $t["24-Q"] = [30, 11, 24, 16, 25]; $t["24-H"] = [30, 30, 16, 2, 17];
    $t["25-L"] = [26, 8, 106, 4, 107]; $t["25-M"] = [28, 8, 47, 13, 48]; $t["25-Q"] = [30, 7, 24, 22, 25]; $t["25-H"] = [30, 22, 15, 13, 16];
    $t["26-L"] = [28, 10, 114, 2, 115]; $t["26-M"] = [28, 19, 46, 4, 47]; $t["26-Q"] = [28, 28, 22, 6, 23]; $t["26-H"] = [30, 33, 16, 4, 17];
    $t["27-L"] = [30, 8, 122, 4, 123]; $t["27-M"] = [28, 22, 45, 3, 46]; $t["27-Q"] = [30, 8, 23, 26, 24]; $t["27-H"] = [30, 12, 15, 28, 16];
    $t["28-L"] = [30, 3, 117, 10, 118]; $t["28-M"] = [28, 3, 45, 23, 46]; $t["28-Q"] = [30, 4, 24, 31, 25]; $t["28-H"] = [30, 11, 15, 31, 16];
    $t["29-L"] = [30, 7, 116, 7, 117]; $t["29-M"] = [28, 21, 45, 7, 46]; $t["29-Q"] = [30, 1, 23, 37, 24]; $t["29-H"] = [30, 19, 15, 26, 16];
    $t["30-L"] = [30, 5, 115, 10, 116]; $t["30-M"] = [28, 19, 47, 10, 48]; $t["30-Q"] = [30, 15, 24, 25, 25]; $t["30-H"] = [30, 23, 15, 25, 16];
    $t["31-L"] = [30, 13, 115, 3, 116]; $t["31-M"] = [28, 2, 46, 29, 47]; $t["31-Q"] = [30, 42, 24, 1, 25]; $t["31-H"] = [30, 23, 15, 28, 16];
    $t["32-L"] = [30, 17, 115, 0, 0]; $t["32-M"] = [28, 10, 46, 23, 47]; $t["32-Q"] = [30, 10, 24, 35, 25]; $t["32-H"] = [30, 19, 15, 35, 16];
    $t["33-L"] = [30, 17, 115, 1, 116]; $t["33-M"] = [28, 14, 46, 21, 47]; $t["33-Q"] = [30, 29, 24, 19, 25]; $t["33-H"] = [30, 11, 15, 46, 16];
    $t["34-L"] = [30, 13, 115, 6, 116]; $t["34-M"] = [28, 14, 46, 23, 47]; $t["34-Q"] = [30, 44, 24, 7, 25]; $t["34-H"] = [30, 59, 16, 1, 17];
    $t["35-L"] = [30, 12, 121, 7, 122]; $t["35-M"] = [28, 12, 47, 26, 48]; $t["35-Q"] = [30, 39, 24, 14, 25]; $t["35-H"] = [30, 22, 15, 41, 16];
    $t["36-L"] = [30, 6, 121, 14, 122]; $t["36-M"] = [28, 6, 47, 34, 48]; $t["36-Q"] = [30, 46, 24, 10, 25]; $t["36-H"] = [30, 2, 15, 64, 16];
    $t["37-L"] = [30, 17, 122, 4, 123]; $t["37-M"] = [28, 29, 46, 14, 47]; $t["37-Q"] = [30, 49, 24, 10, 25]; $t["37-H"] = [30, 24, 15, 46, 16];
    $t["38-L"] = [30, 4, 122, 18, 123]; $t["38-M"] = [28, 13, 46, 32, 47]; $t["38-Q"] = [30, 48, 24, 14, 25]; $t["38-H"] = [30, 42, 15, 32, 16];
    $t["39-L"] = [30, 20, 117, 4, 118]; $t["39-M"] = [28, 40, 47, 7, 48]; $t["39-Q"] = [30, 43, 24, 22, 25]; $t["39-H"] = [30, 10, 15, 67, 16];
    $t["40-L"] = [30, 19, 118, 6, 119]; $t["40-M"] = [28, 18, 47, 31, 48]; $t["40-Q"] = [30, 34, 24, 34, 25]; $t["40-H"] = [30, 20, 15, 61, 16];
    return $t;
}

# alignPositions returns the alignment-pattern centre coordinates for a version
# (1-40); the row/column combinations of these give the alignment centres.
func alignPositions(version as int) {
    def t as list of list of int init [
        [], [6, 18], [6, 22], [6, 26], [6, 30], [6, 34], [6, 22, 38], [6, 24, 42],
        [6, 26, 46], [6, 28, 50], [6, 30, 54], [6, 32, 58], [6, 34, 62], [6, 26, 46, 66],
        [6, 26, 48, 70], [6, 26, 50, 74], [6, 30, 54, 78], [6, 30, 56, 82], [6, 30, 58, 86],
        [6, 34, 62, 90], [6, 28, 50, 72, 94], [6, 26, 50, 74, 98], [6, 30, 54, 78, 102],
        [6, 28, 54, 80, 106], [6, 32, 58, 84, 110], [6, 30, 58, 86, 114], [6, 34, 62, 90, 118],
        [6, 26, 50, 74, 98, 122], [6, 30, 54, 78, 102, 126], [6, 26, 52, 78, 104, 130],
        [6, 30, 56, 82, 108, 134], [6, 34, 60, 86, 112, 138], [6, 30, 58, 86, 114, 142],
        [6, 34, 62, 90, 118, 146], [6, 30, 54, 78, 102, 126, 150], [6, 24, 50, 76, 102, 128, 154],
        [6, 28, 54, 80, 106, 132, 158], [6, 32, 58, 84, 110, 136, 162], [6, 26, 54, 82, 110, 138, 166],
        [6, 30, 58, 86, 114, 142, 170]];
    return $t[$version - 1];
}

# totalDataCodewords sums the data codewords across both groups.
func totalDataCodewords(info as list of int) {
    return $info[1] * $info[2] + $info[3] * $info[4];
}

# --- QR: data encoding (private) --------------------------------------------

# alnumValue maps an alphanumeric-mode character (byte) to its value 0..44, or -1
# when the character is not in the QR alphanumeric set.
func alnumValue(b as int) {
    if ($b >= 48 and $b <= 57) { return $b - 48; }        # 0-9
    if ($b >= 65 and $b <= 90) { return $b - 65 + 10; }   # A-Z
    if ($b == 32) { return 36; }   # space
    if ($b == 36) { return 37; }   # $
    if ($b == 37) { return 38; }   # %
    if ($b == 42) { return 39; }   # *
    if ($b == 43) { return 40; }   # +
    if ($b == 45) { return 41; }   # -
    if ($b == 46) { return 42; }   # .
    if ($b == 47) { return 43; }   # /
    if ($b == 58) { return 44; }   # :
    return -1;
}

# chooseMode picks the most compact QR mode: numeric (all digits), else
# alphanumeric (all in the alphanumeric set), else byte.
func chooseMode(data as bytes) {
    if (len($data) == 0) {
        return "byte";
    }
    def numeric as bool init true;
    def alnum as bool init true;
    def i as int init 0;
    while ($i < len($data)) {
        def b as int init $data[$i];
        if ($b < 48 or $b > 57) { $numeric = false; }
        if (alnumValue($b) < 0) { $alnum = false; }
        $i = $i + 1;
    }
    if ($numeric) { return "numeric"; }
    if ($alnum) { return "alphanumeric"; }
    return "byte";
}

# qrModeIndicator is the 4-bit mode indicator value.
func qrModeIndicator(mode as string) {
    if ($mode == "numeric") { return 1; }
    if ($mode == "alphanumeric") { return 2; }
    return 4;
}

# qrCountBits is the character-count-indicator width for a mode and version.
func qrCountBits(mode as string, version as int) {
    if ($mode == "numeric") {
        if ($version <= 9) { return 10; }
        if ($version <= 26) { return 12; }
        return 14;
    }
    if ($mode == "alphanumeric") {
        if ($version <= 9) { return 9; }
        if ($version <= 26) { return 11; }
        return 13;
    }
    if ($version <= 9) { return 8; }
    return 16;
}

# qrDataBits is the bit count the payload itself occupies (excluding the mode
# indicator and character count).
func qrDataBits(mode as string, n as int) {
    if ($mode == "numeric") {
        def rem as int init $n % 3;
        def extra as int init 0;
        if ($rem == 1) { $extra = 4; } elseif ($rem == 2) { $extra = 7; }
        return ($n // 3) * 10 + $extra;
    }
    if ($mode == "alphanumeric") {
        return ($n // 2) * 11 + ($n % 2) * 6;
    }
    return $n * 8;
}

# qrUsesEci reports whether a byte-mode payload carries non-ASCII bytes and so
# needs an ECI(26) UTF-8 declaration, without which a reader must guess the
# charset (and may guess wrong).
func qrUsesEci(mode as string, data as bytes) {
    if (not ($mode == "byte")) {
        return false;
    }
    def i as int init 0;
    while ($i < len($data)) {
        if ($data[$i] >= 128) {
            return true;
        }
        $i = $i + 1;
    }
    return false;
}

# selectVersion picks the smallest version (1-40) whose data capacity in `mode`
# holds `n` characters at the given EC level. `eci` adds the 12-bit ECI header.
func selectVersion(mode as string, n as int, eci as bool, level as string) {
    def t as map of string to list of int init blockTable();
    def eciBits as int init 0;
    if ($eci) {
        $eciBits = 12;
    }
    def v as int init 1;
    while ($v <= 40) {
        def info as list of int init $t[convert.toString($v) + "-" + $level];
        def totalBits as int init totalDataCodewords($info) * 8;
        def need as int init 4 + qrCountBits($mode, $v) + qrDataBits($mode, $n) + $eciBits;
        if ($totalBits >= $need) {
            return $v;
        }
        $v = $v + 1;
    }
    fail("data too large for QR (max version 40, level " + $level + ")");
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
# encodeNumeric appends the numeric-mode payload (groups of 3 digits -> 10 bits;
# a trailing 2 digits -> 7 bits, 1 digit -> 4 bits).
func encodeNumeric(bitList as list of int, data as bytes) {
    def out as list of int init $bitList;
    def i as int init 0;
    def n as int init len($data);
    while ($i + 3 <= $n) {
        def v as int init ($data[$i] - 48) * 100 + ($data[$i + 1] - 48) * 10 + ($data[$i + 2] - 48);
        $out = pushBits($out, $v, 10);
        $i = $i + 3;
    }
    def rem as int init $n - $i;
    if ($rem == 2) {
        $out = pushBits($out, ($data[$i] - 48) * 10 + ($data[$i + 1] - 48), 7);
    } elseif ($rem == 1) {
        $out = pushBits($out, $data[$i] - 48, 4);
    }
    return $out;
}

# encodeAlphanumeric appends the alphanumeric-mode payload (pairs -> 11 bits, a
# trailing single -> 6 bits).
func encodeAlphanumeric(bitList as list of int, data as bytes) {
    def out as list of int init $bitList;
    def i as int init 0;
    def n as int init len($data);
    while ($i + 2 <= $n) {
        $out = pushBits($out, alnumValue($data[$i]) * 45 + alnumValue($data[$i + 1]), 11);
        $i = $i + 2;
    }
    if ($n - $i == 1) {
        $out = pushBits($out, alnumValue($data[$i]), 6);
    }
    return $out;
}

func encodeData(data as bytes, mode as string, eci as bool, version as int, level as string) {
    def info as list of int init blockTable()[convert.toString($version) + "-" + $level];
    def total as int init totalDataCodewords($info);
    def totalBits as int init $total * 8;
    def bitList as list of int init [];
    if ($eci) {
        # ECI mode indicator (0111) + a one-byte designator for assignment 26
        # (UTF-8): a strict reader then decodes the byte segment as UTF-8.
        $bitList = pushBits($bitList, 7, 4);
        $bitList = pushBits($bitList, 26, 8);
    }
    $bitList = pushBits($bitList, qrModeIndicator($mode), 4);
    $bitList = pushBits($bitList, len($data), qrCountBits($mode, $version));
    if ($mode == "numeric") {
        $bitList = encodeNumeric($bitList, $data);
    } elseif ($mode == "alphanumeric") {
        $bitList = encodeAlphanumeric($bitList, $data);
    } else {
        def i as int init 0;
        while ($i < len($data)) {
            $bitList = pushBits($bitList, $data[$i], 8);
            $i = $i + 1;
        }
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
    def mode as string init chooseMode($data);
    def eci as bool init qrUsesEci($mode, $data);
    def version as int init selectVersion($mode, len($data), $eci, $level);
    def size as int init 17 + 4 * $version;
    def codewords as list of int init interleave(encodeData($data, $mode, $eci, $version, $level), $version, $level);

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

# --- DataMatrix ECC200 (private) --------------------------------------------
# Square symbols with a single data region (10x10 .. 26x26); larger symbols need
# region interleaving and are out of scope. GF(256) uses the ECC200 primitive
# polynomial 0x12d and a generator with root base 1.

# dmSymbols returns the ECC200 square symbol table: [totalSize, dataCW, ecCW].
func dmSymbols() {
    return [[10, 3, 5], [12, 5, 7], [14, 8, 10], [16, 12, 12], [18, 18, 14],
        [20, 22, 18], [22, 30, 20], [24, 36, 24], [26, 44, 28]];
}

# dmSymbolFor picks the smallest square symbol holding `nData` data codewords.
func dmSymbolFor(nData as int) {
    for (def s in dmSymbols()) {
        if ($s[1] >= $nData) {
            return $s;
        }
    }
    fail("datamatrix: data too large for a single-region square symbol (max 44 codewords)");
}

# dmEncodeAscii encodes bytes in ASCII encodation: a digit pair -> value+130, an
# ASCII byte -> value+1, an extended byte -> upper-shift (235) then (value-128)+1.
func dmEncodeAscii(data as bytes) {
    def cw as list of int init [];
    def i as int init 0;
    def n as int init len($data);
    while ($i < $n) {
        def b as int init $data[$i];
        if ($i + 1 < $n and $b >= 48 and $b <= 57 and $data[$i + 1] >= 48 and $data[$i + 1] <= 57) {
            $cw[] = ($b - 48) * 10 + ($data[$i + 1] - 48) + 130;
            $i = $i + 2;
        } elseif ($b > 127) {
            $cw[] = 235;
            $cw[] = ($b - 128) + 1;
            $i = $i + 1;
        } else {
            $cw[] = $b + 1;
            $i = $i + 1;
        }
    }
    return $cw;
}

# dmPad pads the data codewords to capacity: a first pad 129 (end-of-message),
# then the 253-state randomised pad for the rest.
func dmPad(cw as list of int, capacity as int) {
    def out as list of int init $cw;
    if (len($out) < $capacity) {
        $out[] = 129;
    }
    while (len($out) < $capacity) {
        def pos as int init len($out) + 1;   # 1-based codeword position
        def r as int init ((149 * $pos) % 253) + 1;
        def v as int init 129 + $r;
        if ($v > 254) {
            $v = $v - 254;
        }
        $out[] = $v;
    }
    return $out;
}

# dmWrap applies the ECC200 placement wrap-around for an off-grid coordinate.
func dmWrap(row as int, col as int, nrow as int, ncol as int) {
    def r as int init $row;
    def c as int init $col;
    if ($r < 0) {
        $r = $r + $nrow;
        $c = $c + 4 - (($nrow + 4) % 8);
    }
    if ($c < 0) {
        $c = $c + $ncol;
        $r = $r + 4 - (($ncol + 4) % 8);
    }
    return [$r, $c];
}

# dmBit returns bit `bitNum` (1=MSB .. 8=LSB) of a codeword.
func dmBit(cw as list of int, chr as int, bitNum as int) {
    return ($cw[$chr - 1] >> (8 - $bitNum)) & 1;
}

# dmPlaceUtah places the 8 bits of codeword `chr` in the standard L (utah) shape
# around (row, col), wrapping off-grid coordinates. Returns the updated grid.
func dmPlaceUtah(grid as list of list of int, row as int, col as int, chr as int, cw as list of int, nrow as int, ncol as int) {
    def g as list of list of int init $grid;
    def off as list of list of int init [[-2, -2, 1], [-2, -1, 2], [-1, -2, 3],
        [-1, -1, 4], [-1, 0, 5], [0, -2, 6], [0, -1, 7], [0, 0, 8]];
    for (def o in $off) {
        def rc as list of int init dmWrap($row + $o[0], $col + $o[1], $nrow, $ncol);
        $g[$rc[0]][$rc[1]] = dmBit($cw, $chr, $o[2]);
    }
    return $g;
}

# dmPlaceCorner places codeword `chr` at the 8 absolute [row, col, bit] positions
# of a corner special case. Returns the updated grid.
func dmPlaceCorner(grid as list of list of int, positions as list of list of int, chr as int, cw as list of int) {
    def g as list of list of int init $grid;
    for (def p in $positions) {
        $g[$p[0]][$p[1]] = dmBit($cw, $chr, $p[2]);
    }
    return $g;
}

# dmPlace maps the codeword stream into an nrow x ncol bit grid via the ISO 16022
# ECC200 placement algorithm (Annex F).
func dmPlace(cw as list of int, nrow as int, ncol as int) {
    def grid as list of list of int init newGrid($nrow, -1);
    def c1 as list of list of int init [[$nrow - 1, 0, 1], [$nrow - 1, 1, 2], [$nrow - 1, 2, 3],
        [0, $ncol - 2, 4], [0, $ncol - 1, 5], [1, $ncol - 1, 6], [2, $ncol - 1, 7], [3, $ncol - 1, 8]];
    def c2 as list of list of int init [[$nrow - 3, 0, 1], [$nrow - 2, 0, 2], [$nrow - 1, 0, 3],
        [0, $ncol - 4, 4], [0, $ncol - 3, 5], [0, $ncol - 2, 6], [0, $ncol - 1, 7], [1, $ncol - 1, 8]];
    def c3 as list of list of int init [[$nrow - 3, 0, 1], [$nrow - 2, 0, 2], [$nrow - 1, 0, 3],
        [0, $ncol - 2, 4], [0, $ncol - 1, 5], [1, $ncol - 1, 6], [2, $ncol - 1, 7], [3, $ncol - 1, 8]];
    def c4 as list of list of int init [[$nrow - 1, 0, 1], [$nrow - 1, $ncol - 1, 2], [0, $ncol - 3, 3],
        [0, $ncol - 2, 4], [0, $ncol - 1, 5], [1, $ncol - 3, 6], [1, $ncol - 2, 7], [1, $ncol - 1, 8]];
    def chr as int init 1;
    def row as int init 4;
    def col as int init 0;
    def more as bool init true;
    while ($more) {
        if ($row == $nrow and $col == 0) {
            $grid = dmPlaceCorner($grid, $c1, $chr, $cw);
            $chr = $chr + 1;
        } elseif ($row == $nrow - 2 and $col == 0 and not ($ncol % 4 == 0)) {
            $grid = dmPlaceCorner($grid, $c2, $chr, $cw);
            $chr = $chr + 1;
        } elseif ($row == $nrow - 2 and $col == 0 and $ncol % 8 == 4) {
            $grid = dmPlaceCorner($grid, $c3, $chr, $cw);
            $chr = $chr + 1;
        } elseif ($row == $nrow + 4 and $col == 2 and $ncol % 8 == 0) {
            $grid = dmPlaceCorner($grid, $c4, $chr, $cw);
            $chr = $chr + 1;
        }
        # diagonal sweep upward and to the right
        def up as bool init true;
        while ($up) {
            if ($row < $nrow and $col >= 0 and $grid[$row][$col] == -1) {
                $grid = dmPlaceUtah($grid, $row, $col, $chr, $cw, $nrow, $ncol);
                $chr = $chr + 1;
            }
            $row = $row - 2;
            $col = $col + 2;
            $up = $row >= 0 and $col < $ncol;
        }
        $row = $row + 1;
        $col = $col + 3;
        # diagonal sweep downward and to the left
        def down as bool init true;
        while ($down) {
            if ($row >= 0 and $col < $ncol and $grid[$row][$col] == -1) {
                $grid = dmPlaceUtah($grid, $row, $col, $chr, $cw, $nrow, $ncol);
                $chr = $chr + 1;
            }
            $row = $row + 2;
            $col = $col - 2;
            $down = $row < $nrow and $col >= 0;
        }
        $row = $row + 3;
        $col = $col + 1;
        $more = $row < $nrow or $col < $ncol;
    }
    # the bottom-right corner module is unused by the sweep; fix it to a known
    # pattern (both dark) so it is deterministic.
    if ($grid[$nrow - 1][$ncol - 1] == -1) {
        $grid[$nrow - 1][$ncol - 1] = 1;
        $grid[$nrow - 2][$ncol - 2] = 1;
    }
    return $grid;
}

# dmMatrix builds the full DataMatrix symbol: encode, RS, place, then wrap the
# data region in the finder (solid L on left/bottom, timing on top/right).
func dmMatrix(data as bytes) {
    def cw as list of int init dmEncodeAscii($data);
    def sym as list of int init dmSymbolFor(len($cw));
    def size as int init $sym[0];
    def padded as list of int init dmPad($cw, $sym[1]);
    def field as GF init buildGFPrim(0x12d);
    def ec as list of int init rsEncodeBase($field, $padded, $sym[2], 1);
    def all as list of int init $padded;
    for (def e in $ec) {
        $all[] = $e;
    }
    def region as int init $size - 2;
    def placed as list of list of int init dmPlace($all, $region, $region);
    def m as list of list of bool init [];
    def r as int init 0;
    while ($r < $size) {
        def rowb as list of bool init [];
        def c as int init 0;
        while ($c < $size) {
            def dark as bool init false;
            if ($c == 0 or $r == $size - 1) {
                $dark = true;                       # left / bottom solid L
            } elseif ($r == 0) {
                $dark = $c % 2 == 0;                # top timing
            } elseif ($c == $size - 1) {
                $dark = $r % 2 == 1;                # right timing
            } else {
                $dark = $placed[$r - 1][$c - 1] == 1;   # data region
            }
            $rowb[] = $dark;
            $c = $c + 1;
        }
        $m[] = $rowb;
        $r = $r + 1;
    }
    return $m;
}

# --- encode dispatch (exported) ---------------------------------------------

/**
 * Encode data as a symbol of the given symbology. 2D: "qr", "datamatrix". 1D:
 * "code128", "code93", "ean13", "ean8", "upca", "upce", "itf", "code39",
 * "gs1-128".
 * @param data {string} the payload
 * @param symbology {string} the symbology
 * @param opts {Options} rendering / EC options (uses `ecLevel` for QR)
 * @return {Symbol} the encoded symbol
 * @throws {Error} kind "barcode" on an unknown symbology or invalid data
 */
export func encode(data as string, symbology as string, opts as Options) {
    match ($symbology) {
        when "qr" {
            def raw as bytes init convert.bytesFromString($data, "utf-8");
            def m as list of list of bool init qrMatrix($raw, $opts.ecLevel);
            def noBars as list of int init [];
            return Symbol{ kind: SymbolKind.Matrix, size: len($m), matrix: $m, bars: $noBars, text: $data };
        }
        when "datamatrix" {
            def raw as bytes init convert.bytesFromString($data, "utf-8");
            def m as list of list of bool init dmMatrix($raw);
            def noBars as list of int init [];
            return Symbol{ kind: SymbolKind.Matrix, size: len($m), matrix: $m, bars: $noBars, text: $data };
        }
        when "code128" { return linearSymbol($data, code128Bars($data)); }
        when "code39" { return linearSymbol($data, code39Bars($data)); }
        when "code93" { return linearSymbol($data, code93Bars($data)); }
        when "ean13" { return linearSymbol($data, ean13Bars($data)); }
        when "ean8" { return linearSymbol($data, ean8Bars($data)); }
        when "upca" { return linearSymbol($data, upcaBars($data)); }
        when "upce" { return linearSymbol($data, upceBars($data)); }
        when "itf" { return linearSymbol($data, itfBars($data)); }
        when "gs1-128" { return linearSymbol($data, gs1128Bars($data)); }
        else { fail("unknown symbology: " + $symbology); }
    }
}

func linearSymbol(data as string, bars as list of int) {
    def empty as list of list of bool init [];
    return Symbol{ kind: SymbolKind.Linear, size: 0, matrix: $empty, bars: $bars, text: $data };
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

# code128Bars encodes with Code 128 (code set B), auto start / checksum / stop.
func code128Bars(data as string) {
    def patterns as list of string init code128Patterns();
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

# code39Bars encodes Code 39 (with start / stop `*`, no check digit).
func code39Bars(data as string) {
    def bits as string init code39Char("*");
    def cs as list of string init strings.chars(strings.upper($data));
    for (def ch in $cs) {
        # `*` is the Code 39 start/stop delimiter; in the payload it would encode
        # a stop mid-symbol and truncate the scan, so reject it.
        if ($ch == "*") {
            fail("code39: '*' is the start/stop delimiter and cannot appear in the data");
        }
        $bits = $bits + "0" + code39Char($ch);
    }
    $bits = $bits + "0" + code39Char("*");
    return bitsToBars($bits);
}

# ean13Bars / ean8Bars / itfBars.
func ean13Bars(data as string) {
    return eanBars($data, 13);
}

func ean8Bars(data as string) {
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
        return ean13Encode($ds);
    }
    return ean8Encode($ds);
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

# ean13Encode builds the module string for 13 digits (first digit sets the
# L/G parity pattern of the left group).
func ean13Encode(ds as list of int) {
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

# ean8Encode builds the module string for 8 digits (L then R, no parity).
func ean8Encode(ds as list of int) {
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

# code39Char returns the 9-element module string for one Code 39 character.
func code39Char(ch as string) {
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

# --- UPC-A / UPC-E (private) -------------------------------------------------

# upcaBars encodes UPC-A: it is exactly an EAN-13 with a leading 0, so 11 data
# digits (check computed) or a full 12 (check verified) map straight through.
func upcaBars(data as string) {
    def ds as list of int init digitList($data);
    if (not (len($ds) == 11 or len($ds) == 12)) {
        fail("upca: expected 11 or 12 digits");
    }
    return eanBars("0" + $data, 13);
}

# upceExpand expands a 6-digit UPC-E body to the 11-digit UPC-A number (number
# system + 5 manufacturer + 5 item), per the last-digit compression rules.
func upceExpand(ns as int, six as list of int) {
    def x1 as int init $six[0];
    def x2 as int init $six[1];
    def x3 as int init $six[2];
    def x4 as int init $six[3];
    def x5 as int init $six[4];
    def x6 as int init $six[5];
    def out as list of int init [$ns];
    if ($x6 <= 2) {
        $out[] = $x1; $out[] = $x2; $out[] = $x6;
        $out[] = 0; $out[] = 0; $out[] = 0; $out[] = 0;
        $out[] = $x3; $out[] = $x4; $out[] = $x5;
    } elseif ($x6 == 3) {
        $out[] = $x1; $out[] = $x2; $out[] = $x3;
        $out[] = 0; $out[] = 0; $out[] = 0; $out[] = 0; $out[] = 0;
        $out[] = $x4; $out[] = $x5;
    } elseif ($x6 == 4) {
        $out[] = $x1; $out[] = $x2; $out[] = $x3; $out[] = $x4;
        $out[] = 0; $out[] = 0; $out[] = 0; $out[] = 0; $out[] = 0;
        $out[] = $x5;
    } else {
        $out[] = $x1; $out[] = $x2; $out[] = $x3; $out[] = $x4; $out[] = $x5;
        $out[] = 0; $out[] = 0; $out[] = 0; $out[] = 0;
        $out[] = $x6;
    }
    return $out;
}

# upceCheck is the UPC-E check digit: the UPC-A check of the expanded number.
func upceCheck(ns as int, six as list of int) {
    def upca as list of int init upceExpand($ns, $six);
    def body as list of int init [0];   # UPC-A is EAN-13 with a leading 0
    for (def d in $upca) {
        $body[] = $d;
    }
    return eanCheck($body, 13);
}

# upceParity returns the 6-character L/G parity string for a number system and
# check digit (number system 1 is the complement of number system 0).
func upceParity(ns as int, check as int) {
    def zero as list of string init ["EEEOOO", "EEOEOO", "EEOOEO", "EEOOOE", "EOEEOO",
        "EOOEEO", "EOOOEE", "EOEOEO", "EOEOOE", "EOOEOE"];
    def p as string init $zero[$check];
    if ($ns == 0) {
        return $p;
    }
    def out as string init "";
    for (def ch in strings.chars($p)) {
        if ($ch == "E") {
            $out = $out + "O";
        } else {
            $out = $out + "E";
        }
    }
    return $out;
}

# upceBars encodes UPC-E. Input is 6 digits (number system 0, check computed),
# 7 digits (number system + 6, check computed), or 8 digits (number system + 6 +
# check, verified). Odd (O) parity uses the L code, even (E) parity the G code.
func upceBars(data as string) {
    def ds as list of int init digitList($data);
    def ns as int init 0;
    def six as list of int init [];
    def check as int init -1;
    if (len($ds) == 6) {
        $six = $ds;
    } elseif (len($ds) == 7) {
        $ns = $ds[0];
        $six = lists.slice($ds, 1, 7);
    } elseif (len($ds) == 8) {
        $ns = $ds[0];
        $six = lists.slice($ds, 1, 7);
        $check = $ds[7];
    } else {
        fail("upce: expected 6, 7, or 8 digits");
    }
    if (not ($ns == 0 or $ns == 1)) {
        fail("upce: number system must be 0 or 1");
    }
    def computed as int init upceCheck($ns, $six);
    if ($check >= 0 and not ($check == $computed)) {
        fail("upce: check digit mismatch");
    }
    def parity as string init upceParity($ns, $computed);
    def bits as string init "101";   # start guard
    def i as int init 0;
    while ($i < 6) {
        if (strings.substring($parity, $i, $i + 1) == "O") {
            $bits = $bits + eanL($six[$i]);
        } else {
            $bits = $bits + eanG($six[$i]);
        }
        $i = $i + 1;
    }
    $bits = $bits + "010101";   # end guard
    return bitsToBars($bits);
}

# --- Code 93 (private) ------------------------------------------------------

# code93Set is the ordered Code 93 character set (values 0..46); index in this
# string is the character's value used for the checksum.
func code93Set() {
    return "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-. $/+%";
}

# code93Widths returns the element-width string (bar,space,bar,space,bar,space,
# summing to 9) for a Code 93 value 0..46, plus the start/stop `*` at index 47.
func code93Widths() {
    # values 0..46 (the 43 printable characters then the four shift characters
    # ($) (%) (/) (+), which a check character may land on), then `*` at index 47.
    return ["131112", "111213", "111312", "111411", "121113", "121212", "121311",
        "111114", "131211", "141111", "211113", "211212", "211311", "221112",
        "221211", "231111", "112113", "112212", "112311", "122112", "132111",
        "111123", "111222", "111321", "121122", "131121", "212112", "212211",
        "211122", "211221", "221121", "222111", "112122", "112221", "122121",
        "123111", "121131", "311112", "311211", "321111", "112131", "113121",
        "211131", "121221", "312111", "311121", "122211", "111141"];
}

# appendWidths appends a Code 93 element-width string's runs to a bar list.
func appendWidths(bars as list of int, widths as string) {
    def out as list of int init $bars;
    for (def ch in strings.chars($widths)) {
        $out[] = convert.toCodepoint($ch) - 48;
    }
    return $out;
}

# code93Value returns the Code 93 value of a character, or -1 if unsupported.
func code93Value(ch as string) {
    return strings.indexOf(code93Set(), $ch);
}

# code93Checks computes the two Code 93 check characters C and K over the data
# values (weighted right-to-left, cycling 1..20 for C and 1..15 for K, mod 47).
func code93Check(values as list of int, maxWeight as int) {
    def sum as int init 0;
    def weight as int init 1;
    def i as int init len($values) - 1;
    while ($i >= 0) {
        $sum = $sum + $values[$i] * $weight;
        $weight = $weight + 1;
        if ($weight > $maxWeight) {
            $weight = 1;
        }
        $i = $i - 1;
    }
    return $sum % 47;
}

# code93Bars encodes Code 93 (base 47-character set) with start/stop `*` and the
# two check characters C and K.
func code93Bars(data as string) {
    def widths as list of string init code93Widths();
    def values as list of int init [];
    for (def ch in strings.chars(strings.upper($data))) {
        def v as int init code93Value($ch);
        if ($v < 0) {
            fail("code93: unsupported character '" + $ch + "'");
        }
        $values[] = $v;
    }
    def c as int init code93Check($values, 20);
    def withC as list of int init $values;
    $withC[] = $c;
    def k as int init code93Check($withC, 15);
    def bars as list of int init [];
    $bars = appendWidths($bars, $widths[47]);   # start *
    for (def v in $values) {
        $bars = appendWidths($bars, $widths[$v]);
    }
    $bars = appendWidths($bars, $widths[$c]);
    $bars = appendWidths($bars, $widths[$k]);
    $bars = appendWidths($bars, $widths[47]);   # stop *
    $bars[] = 1;   # termination bar
    return $bars;
}

# --- GS1-128 (private) ------------------------------------------------------

# gs1FixedLength reports whether an Application Identifier's first two digits mark
# a fixed-length element string (so no FNC1 separator is needed after its value).
func gs1FixedLength(ai as string) {
    if (len($ai) < 2) {
        return false;
    }
    def two as string init strings.substring($ai, 0, 2);
    def fixed as string init " 00 01 02 03 04 11 12 13 14 15 16 17 18 19 20 31 32 33 34 35 36 41 ";
    return strings.contains($fixed, " " + $two + " ");
}

# gs1Elements parses "(AI)value(AI)value..." into a flat character list with FNC1
# markers (code 102): a FNC1 leads the data, and separates a variable-length
# element string from the next AI. The parentheses are display-only (not encoded).
func gs1Elements(data as string) {
    def out as list of int init [102];   # leading FNC1 marks a GS1 structure
    def cs as list of string init strings.chars($data);
    def i as int init 0;
    def prevVariable as bool init false;
    def started as bool init false;
    while ($i < len($cs)) {
        if (not ($cs[$i] == "(")) {
            fail("gs1-128: expected '(' starting an Application Identifier");
        }
        # read the AI inside the parentheses
        def ai as string init "";
        $i = $i + 1;
        while ($i < len($cs) and not ($cs[$i] == ")")) {
            $ai = $ai + $cs[$i];
            $i = $i + 1;
        }
        if ($i >= len($cs)) {
            fail("gs1-128: unclosed Application Identifier");
        }
        $i = $i + 1;   # skip ')'
        # a separator FNC1 precedes this AI when the previous value was variable
        if ($started and $prevVariable) {
            $out[] = 102;
        }
        $started = true;
        # emit the AI digits then the value up to the next '('
        for (def ch in strings.chars($ai)) {
            def code as int init charToCode($ch);
            if ($code < 0) {
                fail("gs1-128: invalid AI character '" + $ch + "'");
            }
            $out[] = $code;
        }
        def valueLen as int init 0;
        while ($i < len($cs) and not ($cs[$i] == "(")) {
            def code as int init charToCode($cs[$i]);
            if ($code < 0) {
                fail("gs1-128: invalid value character '" + $cs[$i] + "'");
            }
            $out[] = $code;
            $valueLen = $valueLen + 1;
            $i = $i + 1;
        }
        $prevVariable = not gs1FixedLength($ai);
    }
    return $out;
}

# gs1128Bars encodes GS1-128: Code 128 (set B) with a leading FNC1 and FNC1
# separators after variable-length element strings.
func gs1128Bars(data as string) {
    def patterns as list of string init code128Patterns();
    def elements as list of int init gs1Elements($data);
    def values as list of int init [104];   # Start B
    def sum as int init 104;
    def pos as int init 1;
    for (def code in $elements) {
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
    $bits = $bits + "11";
    return bitsToBars($bits);
}

# code128Patterns is the Code 128 module-pattern table (values
# 0..106, 11 modules each; the encoder appends the 2-module termination bar
# after the stop pattern, entry 106).
func code128Patterns() {
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
    if (not ($symbol.kind == SymbolKind.Matrix)) {
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
    if (not ($symbol.kind == SymbolKind.Matrix)) {
        fail("terminal: 1D symbols are better viewed as an image");
    }
    def m as list of list of bool init $symbol.matrix;
    def n as int init len($m);
    # A 4-module light quiet zone on every side: without it a camera QR scanner
    # cannot locate the finder patterns (a terminal render is otherwise flush to
    # the surrounding text). The grid walked below is the symbol plus this border.
    def q as int init 4;
    def size as int init $n + 2 * $q;
    def parts as list of string init [];
    def r as int init 0;
    while ($r < $size) {
        def c as int init 0;
        while ($c < $size) {
            def top as bool init termCell($m, $n, $q, $r, $c);
            def bot as bool init false;
            if ($r + 1 < $size) {
                $bot = termCell($m, $n, $q, $r + 1, $c);
            }
            $parts[] = halfBlock($top, $bot);
            $c = $c + 1;
        }
        $parts[] = "\n";
        $r = $r + 2;
    }
    return strings.join($parts, "");
}

# termCell reads a module of the quiet-zone-padded terminal grid: the symbol's
# own cell inside the border, else light (the quiet zone).
func termCell(m as list of list of bool, n as int, q as int, r as int, c as int) {
    if ($r >= $q and $r < $q + $n and $c >= $q and $c < $q + $n) {
        return $m[$r - $q][$c - $q];
    }
    return false;
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
    match ($symbol.kind) {
        when Matrix { return svgMatrix($symbol, $opts); }
        when Linear { return svgLinear($symbol, $opts); }
    }
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
    # A human-readable text line sits below the bars: reserve a band sized to the
    # glyph height plus padding when the option is on and the symbol has text.
    def showText as bool init $opts.humanReadable and len($symbol.text) > 0;
    def fontSize as int init 8 * $s;
    def textBand as int init 0;
    if ($showText) {
        $textBand = $fontSize + 2 * $s;
    }
    def height as int init $h + 2 * $q * $s + $textBand;
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
    if ($showText) {
        # baseline just below the bars, centred across the full symbol width
        def baseline as int init $q * $s + $h + $fontSize;
        $parts[] = svgText($width // 2, $baseline, $fontSize, $symbol.text, $opts.foreground);
    }
    $parts[] = "</svg>\n";
    return strings.join($parts, "");
}

# svgText renders a centred monospace text line (the human-readable data).
func svgText(cx as int, y as int, size as int, text as string, fill as string) {
    return "<text x=\"" + convert.toString($cx) + "\" y=\"" + convert.toString($y) +
        "\" font-family=\"monospace\" font-size=\"" + convert.toString($size) +
        "\" text-anchor=\"middle\" fill=\"" + $fill + "\">" + escapeXmlText($text) + "</text>\n";
}

# escapeXmlText escapes the text-significant XML characters for an SVG text node.
func escapeXmlText(s as string) {
    def out as string init strings.replace($s, "&", "&amp;");
    $out = strings.replace($out, "<", "&lt;");
    $out = strings.replace($out, ">", "&gt;");
    return $out;
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
    if ($symbol.kind == SymbolKind.Matrix) {
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
