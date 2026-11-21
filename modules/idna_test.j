# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
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
    testing.assertEqual(toAscii("Example.COM"), "example.com"); # lowercased
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
    testing.assertEqual(charToDigit("A"), 0); # case-insensitive
    testing.assertEqual(charToDigit("-"), -1); # not a digit
}

func testDigitToChar() {
    testing.assertEqual(digitToChar(0), "a");
    testing.assertEqual(digitToChar(25), "z");
    testing.assertEqual(digitToChar(26), "0");
}

func testThreshold() {
    testing.assertEqual(threshold(36, 72), 1); # k <= bias -> tmin
    testing.assertEqual(threshold(108, 72), 26); # k >= bias+tmax -> tmax
    testing.assertEqual(threshold(80, 72), 8); # k - bias
}

func testEncodeLabelDirect() {
    # "münchen" (no dot) encodes to "mnchen-3ya".
    testing.assertEqual(encodeLabel(codePoints("münchen")), "mnchen-3ya");
    testing.assertEqual(decodeLabel("mnchen-3ya"), "münchen");
}

# A malformed ACE label must throw a typed idna error rather than crash or
# return garbage (a spoofing aid).
func decodeInvalidDigit() {
    decodeLabel("mnchen-!ya");
}
func decodeTruncated() {
    decodeLabel("mnchen-3y");
} # drops the terminating digit
func testDecodeLabelRejectsMalformed() {
    testing.assertThrows("decodeInvalidDigit", "idna");
    testing.assertThrows("decodeTruncated", "idna");
}

# ROBUSTNESS: a punycode label whose decode accumulators would overflow int64
# fails with the module's own kind "idna" (an RFC 3492 section 6.4 overflow
# check), not the raw kind "runtime" of an integer-overflow fault - a hostname
# from a peer or certificate could otherwise trigger the mislabelled error.
func decodeOverflowLabel() {
    return toUnicode("xn--99999999999999999999999999999");
}
func testPunycodeOverflowKindIdna() {
    testing.assertThrows("decodeOverflowLabel", "idna");
}

# Over-long labels and domains are rejected up front (the encoder and decoder
# are quadratic in label length), not processed.
func longUnicodeLabel() {
    return toAscii(strings.replicate("ü", 60) + ".de");
}
func longAceLabel() {
    return toUnicode("xn--" + strings.replicate("a", 100000));
}
func longDomain() {
    return toAscii(strings.replicate("abcdefghij.", 24));
}
func testLengthLimits() {
    testing.assertThrows("longUnicodeLabel", "idna");
    testing.assertThrows("longAceLabel", "idna");
    testing.assertThrows("longDomain", "idna");
    testing.assertEqual(
        toAscii(strings.replicate("abcdefghi.", 25) + "de."),
        strings.replicate("abcdefghi.", 25) + "de.");
}

# An `xn--` label must be a real A-label (RFC 5891 section 5.4): one decoding to
# pure ASCII or to nothing would alias an ASCII name (`xn--abc-.com` read as
# `abc.com`), and a non-canonical encoding would give one name two A-labels.
func asciiAliasUnicode() {
    return toUnicode("xn--abc-.com");
}
func emptyAceUnicode() {
    return toUnicode("xn--.de");
}
func asciiAliasAscii() {
    return toAscii("xn--abc-.com");
}
func uppercaseULabel() {
    # "Mnchen" with an encoded ü: the decoded label carries an uppercase M.
    return toUnicode("xn--Mnchen-3ya.de");
}
func testALabelMustDecodeToULabel() {
    testing.assertThrows("asciiAliasUnicode", "idna");
    testing.assertThrows("emptyAceUnicode", "idna");
    testing.assertThrows("asciiAliasAscii", "idna");
    testing.assertEqual(toUnicode("xn--MNCHEN-3YA.de"), "münchen.de");
    testing.assertEqual(toAscii("xn--mnchen-3ya.de"), "xn--mnchen-3ya.de");
}

# Controls, spaces and invisible characters (U+200B and friends) make a label
# that renders like another one; they are rejected on both sides. ASCII
# punctuation such as `_` (`_dmarc`) stays allowed in an ASCII label.
func spaceLabel() {
    return toAscii("ex ample.com");
}
func nulLabel() {
    return toAscii("ex" + convert.fromCodepoint(0) + ".com");
}
func zeroWidthLabel() {
    return toAscii("exa" + convert.fromCodepoint(0x200B) + "mple.com");
}
func zeroWidthUnicodeLabel() {
    return toAscii("mü" + convert.fromCodepoint(0x200B) + "nchen.de");
}
func underscoreULabel() {
    return toAscii("mü_nchen.de");
}
func hyphenULabel() {
    return toAscii("-münchen.de");
}
func spaceUnicode() {
    return toUnicode("ex ample.com");
}
func testRejectsInvisibleCharacters() {
    testing.assertThrows("spaceLabel", "idna");
    testing.assertThrows("nulLabel", "idna");
    testing.assertThrows("zeroWidthLabel", "idna");
    testing.assertThrows("zeroWidthUnicodeLabel", "idna");
    testing.assertThrows("underscoreULabel", "idna");
    testing.assertThrows("hyphenULabel", "idna");
    testing.assertThrows("spaceUnicode", "idna");
    testing.assertEqual(toAscii("_dmarc.Example.com"), "_dmarc.example.com");
    testing.assertEqual(toUnicode("_dmarc.xn--mnchen-3ya.de"), "_dmarc.münchen.de");
}
