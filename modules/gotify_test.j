# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# gotify_test.j - white-box tests for gotify.j's pure form encoding. Run with:
#
#     jennifer test modules/gotify_test.j
#
# The overlay splices gotify.j in front of this file, so the tests reach its
# private URL / form encoder by bare identifier. The networked push (the POST
# with the X-Gotify-Key header) is verified against an in-process HTTP server in
# the Go suite (TestGotifyPush).
use testing;

func testUrlEncodePlain() {
    testing.assertEqual(urlEncode("hello"), "hello");
}

func testUrlEncodeSpace() {
    testing.assertEqual(urlEncode("a b c"), "a+b+c");
}

func testUrlEncodeReserved() {
    testing.assertEqual(urlEncode("a&b=c?d"), "a%26b%3Dc%3Fd");
}

func testUrlEncodeUnreserved() {
    # A-Z a-z 0-9 - _ . ~ are left literal
    testing.assertEqual(urlEncode("A-Z_a.z~9"), "A-Z_a.z~9");
}

func testUrlEncodeUnicode() {
    # per-byte over UTF-8: "é" is C3 A9
    testing.assertEqual(urlEncode("café"), "caf%C3%A9");
}

func testFormBody() {
    testing.assertEqual(formBody("Hi there", "a&b", 5),
        "title=Hi+there&message=a%26b&priority=5");
}
