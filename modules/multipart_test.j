# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# multipart_test.j - white-box tests for multipart.j. Run with:
#
#     jennifer test modules/multipart_test.j
#
# Pure build / parse round-trips and header extraction, no network. multipart.j
# already `use`s strings / convert / lists / math, so the overlay only adds
# testing.
use testing;

# asString decodes an all-ASCII body for exact comparison.
func asString(b as bytes) {
    return convert.stringFromBytes("utf-8", $b);
}

# A body missing the closing "--boundary--" (a truncated / torn upload) is
# rejected, not silently accepted as a complete multipart.
func testTornBodyRejected() {
    def torn as bytes init convert.bytesFromString(
        "utf-8",
        "--Z\r\nContent-Disposition: form-data; name=\"a\"\r\n\r\n1\r\n");
    def threw as bool init false;
    def kind as string init "";
    try {
        parse("multipart/form-data; boundary=Z", $torn);
    } catch (e) {
        $threw = true;
        $kind = $e.kind;
    }
    testing.assertTrue($threw);
    testing.assertEqual($kind, "multipart");
}

func testBuildExactBody() {
    def parts as list of Part init [field("title", "hi"), field("n", "42")];
    def form as Built init buildWith($parts, "B");
    testing.assertEqual($form.contentType, "multipart/form-data; boundary=B");
    testing.assertEqual(
        asString($form.body),
        "--B\r\nContent-Disposition: form-data; name=\"title\"\r\n\r\nhi\r\n" +
            "--B\r\nContent-Disposition: form-data; name=\"n\"\r\n\r\n42\r\n" +
            "--B--\r\n");
}

func testBuildFilePart() {
    def parts as list of Part init [
        file("doc", "a.txt", "text/plain", convert.bytesFromString("utf-8", "body"))
    ];
    def form as Built init buildWith($parts, "X");
    testing.assertEqual(
        asString($form.body),
        "--X\r\nContent-Disposition: form-data; name=\"doc\"; filename=\"a.txt\"\r\n" +
            "Content-Type: text/plain\r\n\r\nbody\r\n--X--\r\n");
}

func testRoundTripFields() {
    def parts as list of Part init [field("a", "one"), field("b", "two & more")];
    def form as Built init buildWith($parts, "Z");
    def back as list of Part init parse($form.contentType, $form.body);
    testing.assertEqual(len($back), 2);
    testing.assertEqual($back[0].name, "a");
    testing.assertEqual(text($back[0]), "one");
    testing.assertEqual($back[1].name, "b");
    testing.assertEqual(text($back[1]), "two & more");
    testing.assertTrue(not isFile($back[0]));
}

func testRoundTripBinaryFile() {
    def data as bytes;
    $data[] = 0;
    $data[] = 255;
    $data[] = 13;
    $data[] = 10;
    def parts as list of Part init [file("f", "x.bin", "application/octet-stream", $data)];
    def form as Built init buildWith($parts, "BND");
    def back as list of Part init parse($form.contentType, $form.body);
    testing.assertEqual(len($back), 1);
    testing.assertTrue(isFile($back[0]));
    testing.assertEqual($back[0].filename, "x.bin");
    testing.assertEqual($back[0].contentType, "application/octet-stream");
    testing.assertEqual(len($back[0].data), 4);
    testing.assertEqual($back[0].data[1], 255);
    testing.assertEqual($back[0].data[3], 10);
}

func testBoundaryExtraction() {
    testing.assertEqual(boundaryOf("multipart/form-data; boundary=abc"), "abc");
    testing.assertEqual(boundaryOf("multipart/form-data; boundary=\"a b c\""), "a b c");
    testing.assertEqual(boundaryOf("multipart/form-data; boundary=xyz; charset=utf-8"), "xyz");
    testing.assertEqual(boundaryOf("text/plain"), "");
}

func testExtractHelpers() {
    def h as string init "Content-Disposition: form-data; name=\"field\"; filename=\"up.png\"\r\nContent-Type: image/png";
    testing.assertEqual(extractParam($h, "name"), "field");
    testing.assertEqual(extractParam($h, "filename"), "up.png");
    testing.assertEqual(extractHeader($h, "content-type"), "image/png");
    testing.assertEqual(extractParam($h, "missing"), "");
}

# extractParam("name") must not match inside "filename" even when filename
# comes first, and must honor \" escapes.
func testExtractParamKeyBoundary() {
    def h as string init "form-data; filename=\"a.png\"; name=\"real\"";
    testing.assertEqual(extractParam($h, "name"), "real");
    testing.assertEqual(extractParam($h, "filename"), "a.png");
    # An escaped quote inside the value is unescaped, and a ';' inside quotes
    # is not a separator.
    def q as string init "form-data; name=\"a\\\"b;c\"";
    testing.assertEqual(extractParam($q, "name"), "a\"b;c");
}

# buildWith must escape (and neutralize CRLF in) name / filename so a crafted
# value cannot inject headers or a premature body separator; the escaped value
# round-trips through extractParam.
func testBuildEscapesParams() {
    def parts as list of Part init [
        Part{
            name: "f",
            filename: "a\".txt",
            contentType: "",
            data: convert.bytesFromString("utf-8", "x")
        }
    ];
    def form as Built init buildWith($parts, "B");
    def text as string init convert.stringFromBytes("utf-8", $form.body);
    testing.assertContains($text, "filename=\"a\\\".txt\"");
    testing.assertFalse(strings.contains($text, "a\".txt\"")); # not the raw unescaped form
    def back as list of Part init parse($form.contentType, $form.body);
    testing.assertEqual($back[0].filename, "a\".txt");
}

func testParseHandWritten() {
    def raw as string init "--B\r\nContent-Disposition: form-data; name=\"user\"\r\n\r\nalice\r\n--B--\r\n";
    def back as list of Part init parse(
        "multipart/form-data; boundary=B",
        convert.bytesFromString("utf-8", $raw));
    testing.assertEqual(len($back), 1);
    testing.assertEqual($back[0].name, "user");
    testing.assertEqual(text($back[0]), "alice");
}

func testGenerateBoundaryShape() {
    def b as string init generateBoundary();
    testing.assertTrue(strings.startsWith($b, "----JenniferFormBoundary"));
    testing.assertEqual(len($b), 24 + 24);
}

# SECURITY: a part Content-Type with CR/LF is stripped so it cannot inject an
# extra header line or a premature body separator. name / filename were already
# stripped by escapeParam; the content type was written raw before.
func testContentTypeInjectionStripped() {
    def parts as list of Part init [
        file("f", "a.txt", "text/plain\r\nX-Inj2: 1", convert.bytesFromString("utf-8", "x"))
    ];
    def body as string init asString(buildWith($parts, "B").body);
    testing.assertTrue(not strings.contains($body, "\r\nX-Inj2:"));
    testing.assertContains($body, "Content-Type: text/plainX-Inj2: 1\r\n");
}

func badBoundary() {
    return buildWith([field("a", "b")], "x\r\nInjected: 1");
}
func collidingBoundary() {
    return buildWith([field("a", "pre --XYZ post")], "XYZ");
}
# An explicit boundary must be RFC 2046-shaped and must not occur in the data.
func testBuildWithChecksBoundary() {
    testing.assertThrows("badBoundary", "multipart");
    testing.assertThrows("collidingBoundary", "multipart");
    def ok as Built init buildWith([field("a", "b")], "simple-boundary");
    testing.assertEqual($ok.contentType, "multipart/form-data; boundary=simple-boundary");
}

# A boundary that is no RFC 2045 token goes out quoted, and parses back.
func testBuildWithQuotesNonTokenBoundary() {
    def b as Built init buildWith([field("a", "b")], "x y:z=1");
    testing.assertEqual($b.contentType, 'multipart/form-data; boundary="x y:z=1"');
    def back as list of Part init parse($b.contentType, $b.body);
    testing.assertEqual(text($back[0]), "b");
}

# A body of more than MAX_PARTS parts is refused as a limit before it is all
# parsed; MAX_PARTS itself is fine.
func partsBody(n as int) {
    def one as string init "--B\r\nContent-Disposition: form-data; name=\"a\"\r\n\r\nx\r\n";
    return convert.bytesFromString("utf-8", strings.repeat($one, $n) + "--B--\r\n");
}
func tooManyParts() {
    parse("multipart/form-data; boundary=B", partsBody(MAX_PARTS + 1));
}
func testPartCountCap() {
    testing.assertEqual(
        len(parse("multipart/form-data; boundary=B", partsBody(MAX_PARTS))),
        MAX_PARTS);
    testing.assertThrows("tooManyParts", "limit");
}

func parseText(body as string) {
    return parse("multipart/form-data; boundary=b", convert.bytesFromString("utf-8", $body));
}
func parsePhantomPrefix() {
    parseText("--b\r\nContent-Disposition: form-data; name=\"a\"\r\n\r\nline1\r\n" +
        "--bXname=\"x\"\r\n\r\nINJECTED\r\n--b--\r\n");
}
func parsePhantomFirst() {
    parseText("--bXX\r\nContent-Disposition: form-data; name=\"a\"\r\n\r\nv\r\n--b--\r\n");
}
# STRICT: "--boundary" followed by more than transport padding is no delimiter,
# and RFC 2046 forbids the boundary at the start of a body-part line: kind
# "multipart". It used to split there, inventing a phantom part.
func testPhantomDelimiterRejected() {
    testing.assertThrows("parsePhantomPrefix", "multipart");
    testing.assertThrows("parsePhantomFirst", "multipart");
    # Transport padding after a delimiter is allowed.
    def ps as list of Part init parseText(
        "--b \t\r\nContent-Disposition: form-data; name=\"a\"\r\n\r\nv\r\n--b--\r\n");
    testing.assertEqual(text($ps[0]), "v");
}

# name / filename come from Content-Disposition only: the same parameters on
# another header (Content-Type's `name=`, an X- header) are not read.
func testNameOnlyFromDisposition() {
    def ps as list of Part init parseText(
        "--b\r\nContent-Type: text/plain; name=\"evil\"\r\nX-Note: x; filename=\"evil.exe\"\r\n" +
            "Content-Disposition: form-data; name=\"real\"\r\n\r\nv\r\n--b--\r\n");
    testing.assertEqual($ps[0].name, "real");
    testing.assertEqual($ps[0].filename, "");
    testing.assertEqual($ps[0].contentType, "text/plain; name=\"evil\"");
}

func parseNoDisposition() {
    parseText("--b\r\nContent-Type: text/plain\r\n\r\nv\r\n--b--\r\n");
}
func parseNoName() {
    parseText("--b\r\nContent-Disposition: form-data; filename=\"a.txt\"\r\n\r\nv\r\n--b--\r\n");
}
func parseNotFormData() {
    parseText("--b\r\nContent-Disposition: attachment; name=\"a\"\r\n\r\nv\r\n--b--\r\n");
}
func parseNoColon() {
    parseText("--b\r\nContent-Disposition: form-data; name=\"a\"\r\nbogus\r\n\r\nv\r\n--b--\r\n");
}
func parseOpenQuote() {
    parseText("--b\r\nContent-Disposition: form-data; name=\"a\r\n\r\nv\r\n--b--\r\n");
}
# STRICT: RFC 7578 4.2 requires `Content-Disposition: form-data` with a `name`
# on every part; a part without it, a header line without a colon, or an
# unterminated quoted parameter is malformed. An empty name is still a name.
func testPartWithoutFormDataNameRejected() {
    testing.assertThrows("parseNoDisposition", "multipart");
    testing.assertThrows("parseNoName", "multipart");
    testing.assertThrows("parseNotFormData", "multipart");
    testing.assertThrows("parseNoColon", "multipart");
    testing.assertThrows("parseOpenQuote", "multipart");
    def ps as list of Part init parseText(
        "--b\r\nContent-Disposition: Form-Data; name=\"\"\r\n\r\nv\r\n--b--\r\n");
    testing.assertEqual($ps[0].name, "");
}

# BOUNDARY: `boundary` is matched as a whole parameter name, so an earlier
# `xboundary=` parameter is not taken for it.
func testBoundaryIsWholeParam() {
    testing.assertEqual(boundaryOf("multipart/form-data; xboundary=fake; boundary=real"), "real");
    def ps as list of Part init parse(
        "multipart/form-data; xboundary=fake; boundary=real",
        convert.bytesFromString(
            "utf-8",
            "--real\r\nContent-Disposition: form-data; name=\"a\"\r\n\r\nv\r\n--real--\r\n"));
    testing.assertEqual(len($ps), 1);
}
