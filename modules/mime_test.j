# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
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
    testing.assertEqual(crlf("a\r\nb"), "a\r\nb"); # already CRLF, no doubling
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
    testing.assertEqual(
        encode($m),
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
    testing.assertContains(encode($a), "aGVsbG8="); # base64("hello")
}

func testWithHeaderReplaces() {
    def m as Part init text("text/plain", "x");
    def n as Part init withHeader($m, "Subject", "one");
    def nn as Part init withHeader($n, "subject", "two"); # case-insensitive replace
    testing.assertEqual(headerValue($nn, "Subject"), "two");
}

func testMultipartEncode() {
    def kids as list of Part init [];
    $kids[] = text("text/plain", "A");
    $kids[] = text("text/plain", "B");
    def mp as Part init multipart("mixed", "BX", $kids);
    testing.assertEqual(
        encode($mp),
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
    testing.assertEqual(
        body(parse(encode(attachment("a.txt", "text/plain", "hi\nthere & more")))),
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
    testing.assertEqual(
        decodeWord("=?utf-8?B?V2lsbGtvbW1lbiBpbiBJbnN0YnJ1Y2s=?="),
        "Willkommen in Instbruck");
}

func testDecodeQWord() {
    testing.assertEqual(decodeWord("=?UTF-8?Q?caf=C3=A9?="), "café");
    testing.assertEqual(decodeWord("=?UTF-8?Q?a_b?="), "a b"); # "_" is a space
}

func testDecodeAdjacentWordsCollapseSpace() {
    def two as string init encodeWord("Hello") + " " + encodeWord("World");
    testing.assertEqual(decodeWord($two), "HelloWorld");
}

func testDecodeKeepsSurroundingText() {
    testing.assertEqual(decodeWord("Re: " + encodeWord("café") + " today"), "Re: café today");
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
    testing.assertFalse(strings.contains($enc, "Grüße")); # raw form must not leak
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
    def long as string init strings.repeat("é", 60); # 120 bytes -> multiple words
    def e as string init encodeWord($long);
    testing.assertContains($e, "\r\n "); # folded
    testing.assertEqual(decodeWord($e), $long); # and reversible
}

# --- multipart bodies + attachments (public) ---

func testBinaryAttachmentRoundTrip() {
    # Bytes that are NOT valid UTF-8, so a text decode would have corrupted them.
    def raw as bytes;
    $raw[] = 0xff;
    $raw[] = 0x00;
    $raw[] = 0xC3;
    $raw[] = 0x28;
    $raw[] = 0x89;
    def att as Part init attachmentBytes("logo.png", "image/png", $raw);
    def msg as Part init multipart("mixed", "B1", [text("text/plain", "see attached"), $att]);
    def back as Part init parse(encode($msg));

    testing.assertEqual(len(walk($back)), 2); # two leaves
    
    def atts as list of Part init attachments($back);
    testing.assertEqual(len($atts), 1);
    testing.assertEqual(filename($atts[0]), "logo.png");
    testing.assertEqual(contentType($atts[0]), "image/png");
    testing.assertTrue(isAttachment($atts[0]));
    # Byte-for-byte survival is the whole point of the bytes path.
    testing.assertEqual(encoding.toText(data($atts[0]), "hex"), encoding.toText($raw, "hex"));

    def tbs as list of Part init textBodies($back);
    testing.assertEqual(len($tbs), 1);
    testing.assertEqual(body($tbs[0]), "see attached");
    testing.assertFalse(isAttachment($tbs[0]));
}

func testTextBodiesAlternative() {
    def kids as list of Part init [];
    $kids[] = text("text/plain", "plain view");
    $kids[] = text("text/html", "<p>html view</p>");
    def msg as Part init parse(encode(multipart("alternative", "B2", $kids)));
    testing.assertEqual(len(textBodies($msg)), 2); # both alternatives are readable bodies
    def html as list of Part init findParts($msg, "text/html");
    testing.assertEqual(len($html), 1);
    testing.assertEqual(body($html[0]), "<p>html view</p>");
    testing.assertEqual(len(attachments($msg)), 0); # nothing is an attachment here
}

func testWalkSingleText() {
    def p as Part init parse("Content-Type: text/plain\r\n\r\njust text");
    testing.assertEqual(len(walk($p)), 1); # a leaf walks to itself
    testing.assertEqual(len(attachments($p)), 0);
    testing.assertEqual(len(textBodies($p)), 1);
    testing.assertEqual(body($p), "just text");
    testing.assertEqual(convert.stringFromBytes(data($p), "utf-8"), "just text");
}

func testFilenameNameFallback() {
    # No Content-Disposition; filename falls back to the Content-Type name= param,
    # and a name= alone still marks the part as an attachment.
    def p as Part init parse("Content-Type: image/png; name=\"pic.png\"\r\nContent-Transfer-Encoding: base64\r\n\r\naGk=");
    testing.assertEqual(filename($p), "pic.png");
    testing.assertTrue(isAttachment($p));
    testing.assertEqual(disposition($p), "");
    testing.assertEqual(body($p), ""); # binary part: no text view
    testing.assertEqual(convert.stringFromBytes(data($p), "utf-8"), "hi");
}

# --- M23.6: charset on decode ------------------------------------------------

# A text body declaring a non-UTF-8 charset is decoded from that charset, not
# force-read as UTF-8 (which would have mangled the high bytes).
func testDecodeLatin1Body() {
    def p as Part init parse(
        "Content-Type: text/plain; charset=iso-8859-1\r\n" +
            "Content-Transfer-Encoding: quoted-printable\r\n\r\ncaf=E9 r=E9sum=E9\r\n");
    testing.assertEqual(strings.trim(body($p)), "café résumé");
}

func testDecodeWindows1252Body() {
    # 0x92 is a right single quote in Windows-1252 (undefined in Latin-1).
    def p as Part init parse(
        "Content-Type: text/plain; charset=windows-1252\r\n" +
            "Content-Transfer-Encoding: quoted-printable\r\n\r\nit=92s\r\n");
    testing.assertEqual(strings.trim(body($p)), "it’s");
}

func testDecodeDefaultCharsetIsUtf8() {
    def p as Part init parse("Content-Type: text/plain\r\n\r\nplain ascii");
    testing.assertEqual(body($p), "plain ascii");
}

# An unknown charset label must not crash parse; it falls back to UTF-8.
func testDecodeUnknownCharsetFallsBack() {
    def p as Part init parse("Content-Type: text/plain; charset=x-bogus-99\r\n\r\nhello");
    testing.assertEqual(body($p), "hello");
}

# --- M23.6: RFC 2231 filenames (private helpers) -----------------------------

func testSplitParams() {
    def ps as list of Header init splitParams("attachment; filename=\"a.txt\"; size=10");
    testing.assertEqual(len($ps), 2); # the leading token is skipped
    testing.assertEqual($ps[0].name, "filename");
    testing.assertEqual($ps[0].value, "a.txt"); # unquoted
    testing.assertEqual($ps[1].name, "size");
    testing.assertEqual($ps[1].value, "10");
}

func testGetParamExactMatch() {
    # A substring scan would find "name=" inside "filename="; getParam must not.
    def h as string init "image/png; filename=\"disp.png\"; name=\"real.png\"";
    testing.assertEqual(getParam($h, "name"), "real.png");
    testing.assertEqual(getParam($h, "charset"), "");
}

func testPctRoundTrip() {
    testing.assertEqual(pctEncode("café.txt"), "caf%C3%A9.txt");
    testing.assertEqual(convert.stringFromBytes(pctDecodeBytes("caf%C3%A9.txt"), "utf-8"), "café.txt");
    testing.assertEqual(hexByte(0xE9), "E9");
}

func testStripExtPrefix() {
    def parts as list of string init stripExtPrefix("UTF-8'en'caf%C3%A9");
    testing.assertEqual($parts[0], "utf-8");
    testing.assertEqual($parts[1], "caf%C3%A9"); # language segment dropped
    # a value with no prefix comes back charset-less and untouched
    def bare as list of string init stripExtPrefix("plain.txt");
    testing.assertEqual($bare[0], "");
    testing.assertEqual($bare[1], "plain.txt");
}

# --- M23.6: RFC 2231 filenames (decode) --------------------------------------

func testFilenameExtended() {
    def p as Part init parse(
        "Content-Type: image/png\r\n" +
            "Content-Disposition: attachment; filename*=UTF-8''caf%C3%A9.txt\r\n\r\n");
    testing.assertEqual(filename($p), "café.txt");
    testing.assertTrue(isAttachment($p));
}

func testFilenameContinuedPlain() {
    def p as Part init parse(
        "Content-Type: application/octet-stream\r\n" +
            "Content-Disposition: attachment; filename*0=\"a very long \"; filename*1=\"name.dat\"\r\n\r\n");
    testing.assertEqual(filename($p), "a very long name.dat");
}

func testFilenameContinuedExtended() {
    def p as Part init parse(
        "Content-Type: application/octet-stream\r\n" +
            "Content-Disposition: attachment; filename*0*=UTF-8''%E2%82%AC%20; filename*1*=rates.txt\r\n\r\n");
    testing.assertEqual(filename($p), "€ rates.txt");
}

func testFilenamePrefersDisposition() {
    # filename in the disposition wins over a name= in the content type.
    def p as Part init parse(
        "Content-Type: image/png; name=\"ct.png\"\r\n" +
            "Content-Disposition: attachment; filename=\"disp.png\"\r\n\r\n");
    testing.assertEqual(filename($p), "disp.png");
}

# --- M23.6: RFC 2231 filenames (encode + round-trip) -------------------------

func testEncodeAsciiFilenameStaysQuoted() {
    def a as Part init attachment("plain.txt", "text/plain", "hi");
    testing.assertContains(encode($a), "Content-Disposition: attachment; filename=\"plain.txt\"");
}

func testEncodeNonAsciiFilenameIsExtended() {
    def raw as bytes;
    $raw[] = 1;
    $raw[] = 2;
    def a as Part init attachmentBytes("café.txt", "image/png", $raw);
    def enc as string init encode($a);
    testing.assertContains($enc, "filename*=UTF-8''caf%C3%A9.txt");
    testing.assertFalse(strings.contains($enc, "filename=")); # the plain form is not emitted
    def back as Part init parse($enc);
    testing.assertEqual(filename($back), "café.txt");
}

# A filename carrying a backslash or a quote survives the encode / parse round
# trip: encode escapes both (`\` -> `\\`, `"` -> `\"`) and parse's unquote
# reverses both (a plain `\"`-only unescape doubled the backslash).
func testAsciiFilenameEscapesRoundTrip() {
    def back as Part init parse(encode(attachment("a\\b\"c.txt", "text/plain", "hi")));
    testing.assertEqual(filename($back), "a\\b\"c.txt");
}

# A quoted filename holding both an escaped quote and a semicolon must not be
# split at that inner semicolon (the `;` is inside the quoted-string).
func testQuotedFilenameWithSemicolon() {
    def p as Part init parse(
        "Content-Type: application/octet-stream\r\n" +
            "Content-Disposition: attachment; filename=\"a\\\";b.txt\"; size=1\r\n\r\n");
    testing.assertEqual(filename($p), "a\";b.txt");
}

# The extended form also decodes a non-UTF-8 charset (Latin-1 here).
func testFilenameExtendedLatin1() {
    def p as Part init parse(
        "Content-Type: image/png\r\n" +
            "Content-Disposition: attachment; filename*=iso-8859-1'en'caf%E9.txt\r\n\r\n");
    testing.assertEqual(filename($p), "café.txt");
}
