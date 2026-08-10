# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# barcode_test.j - white-box tests for barcode.j (+ the spliced barcode_ecc.inc.j).
# Run with:
#
#     jennifer test modules/barcode_test.j
#
# Pinned against known vectors (Reed-Solomon, format / version BCH, byte-mode
# codewords, 1D bar patterns, PNG signature) and structural round-trips, all
# offline; the "does it actually scan" check is the Go suite decoding a rendered
# PNG. barcode.j already `use`s lists / maps / strings / convert / compress /
# crc / encoding, so the overlay only adds testing.
use testing;

# An SVG colour containing a quote (an attribute-breakout attempt) is rejected;
# a valid hex colour passes.
func testSafeColorRejects() {
    testing.assertEqual(safeColor("#ff0000"), "#ff0000");
    def threw as bool init false;
    try {
        safeColor("#000\" onload=\"alert(1)");
    } catch (e) {
        $threw = true;
    }
    testing.assertTrue($threw);
}

func testReedSolomonVector() {
    # thonky "HELLO WORLD" v1-M: 16 data codewords -> 10 EC codewords.
    def field as GF init buildGF();
    def data as list of int init [32, 91, 11, 120, 209, 114, 220, 77, 67, 64, 236, 17, 236, 17, 236, 17];
    def ec as list of int init rsEncode($field, $data, 10);
    def want as list of int init [196, 35, 39, 119, 235, 215, 231, 226, 93, 23];
    testing.assertEqual(len($ec), 10);
    def i as int init 0;
    while ($i < 10) {
        testing.assertEqual($ec[$i], $want[$i]);
        $i = $i + 1;
    }
}

func testGfMul() {
    def field as GF init buildGF();
    testing.assertEqual(gfMul($field, 2, 2), 4);
    testing.assertEqual(gfMul($field, 128, 2), 29);   # reduction by 0x11d
    testing.assertEqual(gfMul($field, 0, 5), 0);
}

func testFormatBch() {
    testing.assertEqual(formatValue("M", 0), 0x5412);
    testing.assertEqual(formatValue("L", 0), 0x77c4);
}

func testVersionBch() {
    testing.assertEqual(versionValue(7), 0x07c94);
}

func testByteModeCodewords() {
    # "Hi" at version 1-M: 0100 (byte mode) 00000010 (len 2) then H, i, terminator.
    def cw as list of int init encodeData(convert.bytesFromString("Hi", "utf-8"), "byte", false, 1, "M");
    testing.assertEqual(len($cw), 16);   # v1-M total data codewords
    testing.assertEqual($cw[0], 0x40);
    testing.assertEqual($cw[1], 0x24);
    testing.assertEqual($cw[2], 0x86);
    testing.assertEqual($cw[3], 0x90);
    testing.assertEqual($cw[4], 236);    # first pad byte 0xEC
    testing.assertEqual($cw[5], 17);     # 0x11
}

func testCodeThirtyNinePattern() {
    testing.assertEqual(code39Char("*"), "100101101101");
    testing.assertEqual(code39Char("0"), "101001101101");
}

func testQrStructure() {
    def o as Options init defaults();
    def qr as Symbol init encode("HI", "qr", $o);
    testing.assertEqual($qr.kind, SymbolKind.Matrix);
    testing.assertEqual($qr.size, 21);   # version 1
    def m as list of list of bool init matrix($qr);
    # top-left finder: solid dark border row, then a light separator at col 7
    def c as int init 0;
    while ($c < 7) {
        testing.assertTrue($m[0][$c]);
        $c = $c + 1;
    }
    testing.assertTrue(not $m[0][7]);
    # finder inner ring: (1,1) is light, (2,2)..(4,4) dark centre
    testing.assertTrue(not $m[1][1]);
    testing.assertTrue($m[3][3]);
}

func testLinearSymbol() {
    def o as Options init defaults();
    def sym as Symbol init encode("ABC", "code128", $o);
    testing.assertEqual($sym.kind, SymbolKind.Linear);
    testing.assertTrue(len($sym.bars) > 0);
    # bar widths start with a bar and are all positive
    for (def w in $sym.bars) {
        testing.assertTrue($w > 0);
    }
}

# Code 128 reference symbol: "A" in set B is value 33; the checksum is
# (104 + 33*1) % 103 = 34. Start B / 33 / 34 are 11-module patterns and the
# stop is 13 modules (the 11-module stop char plus the 2-module termination
# bar), 46 modules total. The symbol must end with the 2-wide termination
# bar; a doubled termination makes the final bar 4 wide and unscannable.
func testCodeOneTwentyEightReference() {
    def o as Options init defaults();
    def sym as Symbol init encode("A", "code128", $o);
    def want as list of int init [2, 1, 1, 2, 1, 4,
        1, 1, 1, 3, 2, 3,
        1, 3, 1, 1, 2, 3,
        2, 3, 3, 1, 1, 1,
        2];
    testing.assertEqual($sym.bars, $want);
    def total as int init 0;
    for (def w in $sym.bars) {
        $total = $total + $w;
    }
    testing.assertEqual($total, 46);
}

func testPngSignature() {
    def o as Options init defaults();
    def qr as Symbol init encode("HI", "qr", $o);
    def img as bytes init png($qr, $o);
    testing.assertEqual($img[0], 137);
    testing.assertEqual($img[1], 80);    # 'P'
    testing.assertEqual($img[2], 78);    # 'N'
    testing.assertEqual($img[3], 71);    # 'G'
    testing.assertTrue(len($img) > 50);
}

# --- M23.6: UPC-A / UPC-E / Code93 / GS1-128 ---

# barBits renders a Linear symbol's bar runs back to a "1"=bar module string.
func barBits(sym as Symbol) {
    def out as string init "";
    def dark as bool init true;
    for (def w in $sym.bars) {
        def ch as string init "0";
        if ($dark) { $ch = "1"; }
        def i as int init 0;
        while ($i < $w) { $out = $out + $ch; $i = $i + 1; }
        $dark = not $dark;
    }
    return $out;
}

# UPC-A is EAN-13 with a leading 0 - the two symbols are bit-identical.
func testUpcaIsEan13() {
    def o as Options init defaults();
    def a as Symbol init encode("03600029145", "upca", $o);
    def e as Symbol init encode("0036000291452", "ean13", $o);
    testing.assertEqual($a.bars, $e.bars);
}

# UPC-E "04252614" (number system 0, body 425261, check 4) expands to UPC-A
# 042100005264; the module string is pinned against an independent reference.
func testUpceReferenceAndExpand() {
    testing.assertEqual(upceCheck(0, [4, 2, 5, 2, 6, 1]), 4);
    testing.assertEqual(upceExpand(0, [4, 2, 5, 2, 6, 1]), [0, 4, 2, 1, 0, 0, 0, 0, 5, 2, 6]);
    def o as Options init defaults();
    testing.assertEqual(
        barBits(encode("04252614", "upce", $o)),
        "101001110100100110111001001101101011110011001010101");
}

func testUpceRejectsBadCheck() {
    def o as Options init defaults();
    def threw as bool init false;
    try {
        encode("04252615", "upce", $o);   # wrong check digit (should be 4)
    } catch (e) {
        $threw = true;
    }
    testing.assertTrue($threw);
}

# Code 93 "TEST93": the two check characters are C=41 K=6, pinned against an
# independent reference bit string.
func testCode93Reference() {
    def vals as list of int init [29, 14, 28, 29, 9, 3];   # T E S T 9 3
    testing.assertEqual(code93Check($vals, 20), 41);
    def withC as list of int init $vals;
    $withC[] = 41;
    testing.assertEqual(code93Check($withC, 15), 6);
    def o as Options init defaults();
    testing.assertEqual(
        barBits(encode("TEST93", "code93", $o)),
        "1010111101101001101100100101101011001101001101000010101010000101011101101001000101010111101");
}

# GS1-128 leads with FNC1 (102) and separates a variable-length element string
# from the next AI with another FNC1; a fixed-length AI gets no separator.
func testGs1FixedLengthNoSeparator() {
    testing.assertTrue(gs1FixedLength("01"));    # GTIN, fixed
    testing.assertFalse(gs1FixedLength("10"));   # batch/lot, variable
    def els as list of int init gs1Elements("(01)09501101020917(10)ABC123");
    testing.assertEqual($els[0], 102);           # leading FNC1
    testing.assertEqual(len($els), 25);          # 1 + 16 + 8, no separator (01 is fixed)
    # no FNC1 appears after the leading one
    def i as int init 1;
    while ($i < len($els)) {
        testing.assertFalse($els[$i] == 102);
        $i = $i + 1;
    }
}

func testGs1VariableInsertsSeparator() {
    # AI 10 (variable) precedes AI 11 (fixed date), so a FNC1 separator is inserted.
    def els as list of int init gs1Elements("(10)ABC(11)260101");
    def seps as int init 0;
    for (def v in $els) {
        if ($v == 102) { $seps = $seps + 1; }
    }
    testing.assertEqual($seps, 2);   # leading FNC1 + one separator
}

# --- M23.6: QR versions 11-40 + numeric / alphanumeric modes ---

# The QR spec's worked example: "01234567" in version 1-M numeric mode produces
# these 16 data codewords.
func testQrNumericSpecVector() {
    def cw as list of int init encodeData(convert.bytesFromString("01234567", "utf-8"), "numeric", false, 1, "M");
    testing.assertEqual($cw, [16, 32, 12, 86, 97, 128, 236, 17, 236, 17, 236, 17, 236, 17, 236, 17]);
}

func testQrModeSelection() {
    testing.assertEqual(chooseMode(convert.bytesFromString("12345", "utf-8")), "numeric");
    testing.assertEqual(chooseMode(convert.bytesFromString("HELLO WORLD", "utf-8")), "alphanumeric");
    testing.assertEqual(chooseMode(convert.bytesFromString("Hello", "utf-8")), "byte");   # lowercase -> byte
    testing.assertEqual(chooseMode(convert.bytesFromString("", "utf-8")), "byte");
}

func testQrAlnumValue() {
    testing.assertEqual(alnumValue(convert.toCodepoint("0")), 0);
    testing.assertEqual(alnumValue(convert.toCodepoint("A")), 10);
    testing.assertEqual(alnumValue(convert.toCodepoint(":")), 44);
    testing.assertEqual(alnumValue(convert.toCodepoint("a")), -1);   # lowercase not in set
}

# Alphanumeric pairs pack 11 bits each: "HE" -> 17*45+14 = 779, "LO" -> ...
func testQrAlphanumericPacking() {
    def cw as list of int init encodeData(convert.bytesFromString("HE", "utf-8"), "alphanumeric", false, 1, "M");
    # mode 0010, count 000000010 (2), 779 as 11 bits 01100001011 -> 0x20 0x13 0x0B
    testing.assertEqual($cw[0], 32);   # 0x20
    testing.assertEqual($cw[1], 19);   # 0x13
    testing.assertEqual($cw[2], 11);   # 0x0B
}

# Version selection reaches beyond version 10 for large payloads, and the whole
# 1-40 block table is present.
# A byte-mode payload with non-ASCII bytes gets an ECI(26) UTF-8 declaration so a
# reader decodes it as UTF-8; pure-ASCII byte mode and the digit modes do not.
func testQrEci() {
    testing.assertTrue(qrUsesEci("byte", convert.bytesFromString("café", "utf-8")));
    testing.assertFalse(qrUsesEci("byte", convert.bytesFromString("cafe", "utf-8")));
    testing.assertFalse(qrUsesEci("numeric", convert.bytesFromString("12345", "utf-8")));
    # the ECI header is mode 0111 + designator 00011010 (26): first codeword 0x71
    def cw as list of int init encodeData(convert.bytesFromString("é", "utf-8"), "byte", true, 1, "M");
    testing.assertEqual($cw[0], 113);   # 0111 0001 (ECI mode + start of designator)
}

func testQrHighVersionSelection() {
    testing.assertTrue(selectVersion("numeric", 700, false, "M") >= 11);
    testing.assertEqual(selectVersion("byte", 2000, false, "L"), 33);   # 2000 bytes at L -> v33
    testing.assertEqual(len(blockTable()["40-H"]), 5);   # v40 present
    testing.assertEqual(len(alignPositions(40)), 7);     # v40 alignment centres
}

# --- M23.6: DataMatrix ECC200 ---

func testDmEncodeAscii() {
    # digit pairs -> value+130; a lone char / letter -> value+1
    testing.assertEqual(dmEncodeAscii(convert.bytesFromString("42", "utf-8")), [172]);   # 42+130
    testing.assertEqual(dmEncodeAscii(convert.bytesFromString("A", "utf-8")), [66]);      # 65+1
    testing.assertEqual(dmEncodeAscii(convert.bytesFromString("123", "utf-8")), [142, 52]); # "12"->142, "3"->51+1
}

func testDmSymbolSelection() {
    testing.assertEqual(dmSymbolFor(3)[0], 10);    # 10x10 holds 3
    testing.assertEqual(dmSymbolFor(5)[0], 12);
    testing.assertEqual(dmSymbolFor(12)[0], 16);
    testing.assertEqual(dmSymbolFor(44)[0], 26);   # largest single-region square
}

func testDmTooLargeThrows() {
    def o as Options init defaults();
    # 90 digits -> 45 digit-pair codewords, over the 44-codeword single-region max
    def big as string init strings.repeat("1234567890", 9);
    def threw as bool init false;
    try {
        encode($big, "datamatrix", $o);
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "barcode");
    }
    testing.assertTrue($threw);
}

# The full 10x10 symbol for "123456" is pinned against zint (an independent,
# spec-conformant ECC200 encoder) - byte-for-byte identical.
func testDmMatrixReference() {
    def o as Options init defaults();
    def sym as Symbol init encode("123456", "datamatrix", $o);
    testing.assertEqual($sym.kind, SymbolKind.Matrix);
    testing.assertEqual($sym.size, 10);
    def want as list of string init ["1010101010", "1100101101", "1100000100",
        "1100011101", "1100001000", "1000001111", "1110110000", "1111011001",
        "1001110100", "1111111111"];
    def m as list of list of bool init matrix($sym);
    def r as int init 0;
    while ($r < 10) {
        def line as string init "";
        for (def cell in $m[$r]) {
            if ($cell) { $line = $line + "1"; } else { $line = $line + "0"; }
        }
        testing.assertEqual($line, $want[$r]);
        $r = $r + 1;
    }
    # finder: left column and bottom row are solid dark
    def k as int init 0;
    while ($k < 10) {
        testing.assertTrue($m[$k][0]);       # left L
        testing.assertTrue($m[9][$k]);       # bottom L
        $k = $k + 1;
    }
}

# The terminal renderer wraps the symbol in a 4-module quiet zone (2 blank
# half-block lines top/bottom, 4 light columns each side) so a camera QR scanner
# can find the finder patterns.
func testTerminalQuietZone() {
    def o as Options init defaults();
    def art as string init terminal(encode("HI", "qr", $o));   # v1 = 21x21
    def lines as list of string init strings.split($art, "\n");
    # the padded grid is 21 + 8 = 29 modules; rendered 2 rows per line -> 15 lines
    # (plus a trailing empty from the final newline)
    testing.assertEqual(len($lines[0]), 29);          # width = size + 2*quiet
    testing.assertTrue(isBlankLine($lines[0]));       # top quiet zone
    testing.assertTrue(isBlankLine($lines[1]));
    testing.assertFalse(isBlankLine($lines[2]));      # symbol begins
    # each symbol line keeps 4 light columns on the left
    testing.assertEqual(strings.substring($lines[2], 0, 4), "    ");
}

func isBlankLine(line as string) {
    for (def ch in strings.chars($line)) {
        if (not ($ch == " ")) { return false; }
    }
    return true;
}

# A 1D SVG carries a human-readable text line by default; it can be turned off,
# and 2D codes never get one.
func testHumanReadableText() {
    def o as Options init defaults();
    def sym as Symbol init encode("0036000291452", "ean13", $o);
    def out as string init svg($sym, $o);
    testing.assertContains($out, "<text");
    testing.assertContains($out, ">0036000291452</text>");
    testing.assertContains($out, "text-anchor=\"middle\"");
    # turning the option off removes the text line
    def off as Options init $o;
    $off.humanReadable = false;
    testing.assertFalse(containsText(svg($sym, $off)));
    # a QR (2D) never gets a text line
    testing.assertFalse(containsText(svg(encode("HI", "qr", $o), $o)));
}

func testHumanReadableEscapesXml() {
    def o as Options init defaults();
    testing.assertContains(svg(encode("A&B", "code128", $o), $o), ">A&amp;B</text>");
}

func containsText(svgDoc as string) {
    return strings.contains($svgDoc, "<text");
}
use strings;

func testUnknownSymbologyThrows() {
    def o as Options init defaults();
    def threw as bool init false;
    try {
        encode("x", "nosuch", $o);
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "barcode");
    }
    testing.assertTrue($threw);
}
