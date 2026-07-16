# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
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
    return convert.stringFromBytes($b, "utf-8");
}

func testBuildExactBody() {
    def parts as list of Part init [field("title", "hi"), field("n", "42")];
    def form as Built init buildWith($parts, "B");
    testing.assertEqual($form.contentType, "multipart/form-data; boundary=B");
    testing.assertEqual(asString($form.body),
        "--B\r\nContent-Disposition: form-data; name=\"title\"\r\n\r\nhi\r\n" +
        "--B\r\nContent-Disposition: form-data; name=\"n\"\r\n\r\n42\r\n" +
        "--B--\r\n");
}

func testBuildFilePart() {
    def parts as list of Part init [file("doc", "a.txt", "text/plain", convert.bytesFromString("body", "utf-8"))];
    def form as Built init buildWith($parts, "X");
    testing.assertEqual(asString($form.body),
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
    def parts as list of Part init [Part{name: "f", filename: "a\".txt", contentType: "", data: convert.bytesFromString("x", "utf-8")}];
    def form as Built init buildWith($parts, "B");
    def text as string init convert.stringFromBytes($form.body, "utf-8");
    testing.assertContains($text, "filename=\"a\\\".txt\"");
    testing.assertFalse(strings.contains($text, "a\".txt\""));   # not the raw unescaped form
    def back as list of Part init parse($form.contentType, $form.body);
    testing.assertEqual($back[0].filename, "a\".txt");
}

func testParseHandWritten() {
    def raw as string init "--B\r\nContent-Disposition: form-data; name=\"user\"\r\n\r\nalice\r\n--B--\r\n";
    def back as list of Part init parse("multipart/form-data; boundary=B", convert.bytesFromString($raw, "utf-8"));
    testing.assertEqual(len($back), 1);
    testing.assertEqual($back[0].name, "user");
    testing.assertEqual(text($back[0]), "alice");
}

func testGenerateBoundaryShape() {
    def b as string init generateBoundary();
    testing.assertTrue(strings.startsWith($b, "----JenniferFormBoundary"));
    testing.assertEqual(len($b), 24 + 24);
}
