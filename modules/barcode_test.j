# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# barcode_test.j - white-box tests for barcode.j (+ the spliced barcode_ecc.j).
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
    def cw as list of int init encodeData(convert.bytesFromString("Hi", "utf-8"), 1, "M");
    testing.assertEqual(len($cw), 16);   # v1-M total data codewords
    testing.assertEqual($cw[0], 0x40);
    testing.assertEqual($cw[1], 0x24);
    testing.assertEqual($cw[2], 0x86);
    testing.assertEqual($cw[3], 0x90);
    testing.assertEqual($cw[4], 236);    # first pad byte 0xEC
    testing.assertEqual($cw[5], 17);     # 0x11
}

func testCodeThirtyNinePattern() {
    testing.assertEqual(codeThirtyNineChar("*"), "100101101101");
    testing.assertEqual(codeThirtyNineChar("0"), "101001101101");
}

func testQrStructure() {
    def o as Options init defaults();
    def qr as Symbol init encode("HI", "qr", $o);
    testing.assertEqual($qr.kind, "matrix");
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
    testing.assertEqual($sym.kind, "linear");
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
