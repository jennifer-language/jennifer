# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# mime_test.j - white-box tests for mime.j. Run with:
#
#     jennifer test modules/mime_test.j
#
# The overlay splices mime.j in front of this file, so the tests reach its
# private helpers (crlf, stripWS, wrapLines, findHeader, extractBoundary,
# typeOnly, splitHeaderLine, parseHeaders) by bare identifier as well as its
# exported surface.
use testing;

# --- private text helpers (white-box) ---

func testCrlf() {
    testing.assertEqual(crlf("a\nb"), "a\r\nb");
    testing.assertEqual(crlf("a\r\nb"), "a\r\nb");   # already CRLF, no doubling
}

func testStripWS() {
    testing.assertEqual(stripWS("ab cd\r\nef"), "abcdef");
}

func testWrapLines() {
    def s as string init strings.repeat("x", 80);
    def w as string init wrapLines($s);
    # 76 x's, CRLF, then 4 more.
    testing.assertTrue(strings.startsWith($w, strings.repeat("x", 76) + "\r\n"));
    testing.assertTrue(strings.endsWith($w, "xxxx"));
}

func testFindHeaderCaseInsensitive() {
    def hs as list of Header init [];
    $hs[] = Header{name: "Content-Type", value: "text/plain"};
    testing.assertEqual(findHeader($hs, "content-type"), "text/plain");
    testing.assertEqual(findHeader($hs, "Missing"), "");
}

func testExtractBoundary() {
    testing.assertEqual(extractBoundary("multipart/mixed; boundary=\"abc\""), "abc");
    testing.assertEqual(extractBoundary("multipart/mixed; boundary=bare; x=1"), "bare");
    testing.assertEqual(extractBoundary("text/plain"), "");
}

func testTypeOnly() {
    testing.assertEqual(typeOnly("text/plain; charset=utf-8"), "text/plain");
    testing.assertEqual(typeOnly("text/html"), "text/html");
}

func testSplitHeaderLine() {
    def h as Header init splitHeaderLine("Subject:  Hi there ");
    testing.assertEqual($h.name, "Subject");
    testing.assertEqual($h.value, "Hi there");
}

func testParseHeadersUnfolds() {
    def hs as list of Header init parseHeaders("Subject: a\r\n  continued\r\nTo: b");
    testing.assertEqual(len($hs), 2);
    testing.assertEqual($hs[0].value, "a continued");
    testing.assertEqual($hs[1].name, "To");
}

# --- building + encoding (public) ---

func testTextAsciiUsesSevenBit() {
    def m as Part init text("text/plain", "plain");
    testing.assertEqual(headerValue($m, "Content-Transfer-Encoding"), "7bit");
    testing.assertEqual(encode($m),
        "Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 7bit\r\n\r\nplain");
}

func testTextNonAsciiIsQuotedPrintable() {
    def m as Part init text("text/plain", "café");
    testing.assertEqual(headerValue($m, "Content-Transfer-Encoding"), "quoted-printable");
    testing.assertContains(encode($m), "caf=C3=A9");
}

func testAttachmentUsesBaseEncoding() {
    def a as Part init attachment("f.txt", "text/plain", "hello");
    testing.assertEqual(headerValue($a, "Content-Transfer-Encoding"), "base64");
    testing.assertContains(encode($a), "Content-Disposition: attachment; filename=\"f.txt\"");
    testing.assertContains(encode($a), "aGVsbG8=");   # base64("hello")
}

func testWithHeaderReplaces() {
    def m as Part init text("text/plain", "x");
    def n as Part init withHeader($m, "Subject", "one");
    def nn as Part init withHeader($n, "subject", "two");   # case-insensitive replace
    testing.assertEqual(headerValue($nn, "Subject"), "two");
}

func testMultipartEncode() {
    def kids as list of Part init [];
    $kids[] = text("text/plain", "A");
    $kids[] = text("text/plain", "B");
    def mp as Part init multipart("mixed", "BX", $kids);
    testing.assertEqual(encode($mp),
        "Content-Type: multipart/mixed; boundary=\"BX\"\r\n\r\n" +
        "--BX\r\nContent-Type: text/plain; charset=utf-8\r\n" +
        "Content-Transfer-Encoding: 7bit\r\n\r\nA\r\n" +
        "--BX\r\nContent-Type: text/plain; charset=utf-8\r\n" +
        "Content-Transfer-Encoding: 7bit\r\n\r\nB\r\n" +
        "--BX--\r\n");
}

# --- parsing + round-trips (public) ---

func testParseLeaf() {
    def p as Part init parse("Subject: Hi\r\nContent-Type: text/plain\r\n\r\nbody text");
    testing.assertEqual(headerValue($p, "Subject"), "Hi");
    testing.assertEqual(contentType($p), "text/plain");
    testing.assertEqual(body($p), "body text");
}

func testRoundTripQuotedPrintable() {
    testing.assertEqual(body(parse(encode(text("text/plain", "café & résumé")))), "café & résumé");
}

func testRoundTripBaseEncoding() {
    testing.assertEqual(body(parse(encode(attachment("a.txt", "text/plain", "hi\nthere & more")))),
        "hi\nthere & more");
}

func testRoundTripMultipart() {
    def kids as list of Part init [];
    $kids[] = text("text/plain", "one");
    $kids[] = text("text/html", "<i>two</i>");
    def mp as Part init withHeader(multipart("alternative", "ZZ", $kids), "Subject", "S");
    def back as Part init parse(encode($mp));
    testing.assertEqual(headerValue($back, "Subject"), "S");
    testing.assertEqual(contentType($back), "multipart/alternative");
    def kb as list of Part init parts($back);
    testing.assertEqual(len($kb), 2);
    testing.assertEqual(contentType($kb[0]), "text/plain");
    testing.assertEqual(body($kb[0]), "one");
    testing.assertEqual(body($kb[1]), "<i>two</i>");
}

# --- address formatting (public) ---

func testAddress() {
    testing.assertEqual(address("", "a@b.com"), "a@b.com");
    testing.assertEqual(address("Ada", "a@b.com"), "Ada <a@b.com>");
    testing.assertEqual(address("Ada, Countess", "a@b.com"), "\"Ada, Countess\" <a@b.com>");
}

# --- RFC 2047 encoded-words (public) ---

func testEncodeDecodeRoundTrip() {
    testing.assertEqual(decodeWord(encodeWord("café")), "café");
    testing.assertEqual(decodeWord(encodeWord("Grüße aus München")), "Grüße aus München");
    testing.assertEqual(decodeWord(encodeWord("ascii only")), "ascii only");
}

func testEncodeWordShape() {
    testing.assertTrue(strings.startsWith(encodeWord("ö"), "=?UTF-8?B?"));
    testing.assertTrue(strings.endsWith(encodeWord("ö"), "?="));
}

func testDecodeBWord() {
    testing.assertEqual(decodeWord("=?utf-8?B?V2lsbGtvbW1lbiBpbiBJbnN0YnJ1Y2s=?="),
        "Willkommen in Instbruck");
}

func testDecodeQWord() {
    testing.assertEqual(decodeWord("=?UTF-8?Q?caf=C3=A9?="), "café");
    testing.assertEqual(decodeWord("=?UTF-8?Q?a_b?="), "a b");   # "_" is a space
}

func testDecodeAdjacentWordsCollapseSpace() {
    def two as string init encodeWord("Hello") + " " + encodeWord("World");
    testing.assertEqual(decodeWord($two), "HelloWorld");
}

func testDecodeKeepsSurroundingText() {
    testing.assertEqual(decodeWord("Re: " + encodeWord("café") + " today"),
        "Re: café today");
}

func testDecodeNoEncodedWord() {
    testing.assertEqual(decodeWord("plain subject"), "plain subject");
}

func testDecodeBadWordLeftVerbatim() {
    # invalid base64 payload -> left as-is, parse never crashes
    testing.assertEqual(decodeWord("=?UTF-8?B?!!!notb64!!!?="), "=?UTF-8?B?!!!notb64!!!?=");
}

func testEncodeAppliesToSubject() {
    def m as Part init withHeader(text("text/plain", "hi"), "Subject", "Grüße");
    def enc as string init encode($m);
    testing.assertContains($enc, "Subject: =?UTF-8?B?");
    testing.assertFalse(strings.contains($enc, "Grüße"));   # raw form must not leak
}

func testSubjectRoundTripThroughParse() {
    def m as Part init withHeader(text("text/plain", "hi"), "Subject", "Grüße aus München");
    testing.assertEqual(headerValue(parse(encode($m)), "Subject"), "Grüße aus München");
}

func testAddressNameEncoded() {
    def v as string init address("Jörg Müller", "j@x.de");
    testing.assertTrue(strings.startsWith($v, "=?UTF-8?B?"));
    testing.assertTrue(strings.endsWith($v, " <j@x.de>"));
    # and it decodes back through a parsed From header
    def p as Part init parse("From: " + $v + "\r\n\r\n");
    testing.assertEqual(headerValue($p, "From"), "Jörg Müller <j@x.de>");
}

# A multi-address To/Cc/From with non-ASCII display names encodes each mailbox
# (rather than serializing the whole value as raw 8-bit).
func testMultiAddressEachEncoded() {
    def v as string init encodeAddressHeader("Jörg Müller <a@x.de>, José <b@y.es>");
    testing.assertContains($v, "<a@x.de>");
    testing.assertContains($v, "<b@y.es>");
    testing.assertContains($v, "=?UTF-8?B?");
    # No raw non-ASCII byte survives (both names were encoded).
    testing.assertFalse(strings.contains($v, "Jörg"));
    testing.assertFalse(strings.contains($v, "José"));
    # A comma inside a quoted display name is not a mailbox separator.
    def q as string init encodeAddressHeader("\"Müller, Jörg\" <a@x.de>");
    testing.assertContains($q, "<a@x.de>");
    testing.assertContains($q, "=?UTF-8?B?");
}

func testEncodeWordFoldsLong() {
    def long as string init strings.repeat("é", 60);   # 120 bytes -> multiple words
    def e as string init encodeWord($long);
    testing.assertContains($e, "\r\n ");        # folded
    testing.assertEqual(decodeWord($e), $long); # and reversible
}
