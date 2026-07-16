# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# idna_test.j - white-box tests for idna.j. Run with:
#
#     jennifer test modules/idna_test.j
#
# The overlay splices idna.j in front of this file, so the tests reach its
# private helpers (charToDigit, digitToChar, threshold, encodeLabel,
# decodeLabel) by bare identifier alongside the exported conversions.
use testing;

# --- Punycode encode / decode against known values ---

func testToAsciiVectors() {
    testing.assertEqual(toAscii("münchen.de"), "xn--mnchen-3ya.de");
    testing.assertEqual(toAscii("bücher.de"), "xn--bcher-kva.de");
    testing.assertEqual(toAscii("café.fr"), "xn--caf-dma.fr");
}

func testToAsciiAsciiUnchanged() {
    testing.assertEqual(toAscii("example.com"), "example.com");
    testing.assertEqual(toAscii("Example.COM"), "example.com");   # lowercased
}

func testToAsciiMultiLabel() {
    testing.assertEqual(toAscii("sub.münchen.example"), "sub.xn--mnchen-3ya.example");
}

func testToUnicodeReverses() {
    testing.assertEqual(toUnicode("xn--mnchen-3ya.de"), "münchen.de");
    testing.assertEqual(toUnicode("example.com"), "example.com");
}

func testRoundTrip() {
    def domains as list of string init ["bücher.münchen.example", "café.fr", "señor.es"];
    for (def d in $domains) {
        testing.assertEqual(toUnicode(toAscii($d)), $d);
    }
}

func testIsAscii() {
    testing.assertTrue(isAscii("example.com"));
    testing.assertFalse(isAscii("münchen.de"));
}

# --- private Punycode helpers (white-box) ---

func testCharToDigit() {
    testing.assertEqual(charToDigit("a"), 0);
    testing.assertEqual(charToDigit("z"), 25);
    testing.assertEqual(charToDigit("0"), 26);
    testing.assertEqual(charToDigit("9"), 35);
    testing.assertEqual(charToDigit("A"), 0);       # case-insensitive
    testing.assertEqual(charToDigit("-"), -1);      # not a digit
}

func testDigitToChar() {
    testing.assertEqual(digitToChar(0), "a");
    testing.assertEqual(digitToChar(25), "z");
    testing.assertEqual(digitToChar(26), "0");
}

func testThreshold() {
    testing.assertEqual(threshold(36, 72), 1);      # k <= bias -> tmin
    testing.assertEqual(threshold(108, 72), 26);    # k >= bias+tmax -> tmax
    testing.assertEqual(threshold(80, 72), 8);      # k - bias
}

func testEncodeLabelDirect() {
    # "münchen" (no dot) encodes to "mnchen-3ya".
    testing.assertEqual(encodeLabel(codePoints("münchen")), "mnchen-3ya");
    testing.assertEqual(decodeLabel("mnchen-3ya"), "münchen");
}

# A malformed ACE label must throw a typed idna error rather than crash or
# return garbage (a spoofing aid).
func decodeInvalidDigit() { decodeLabel("mnchen-!ya"); }
func decodeTruncated() { decodeLabel("mnchen-3y"); }   # drops the terminating digit
func testDecodeLabelRejectsMalformed() {
    testing.assertThrows("decodeInvalidDigit", "idna");
    testing.assertThrows("decodeTruncated", "idna");
}
