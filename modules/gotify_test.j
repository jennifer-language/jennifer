# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# gotify_test.j - white-box tests for gotify.j's pure form encoding. Run with:
#
#     jennifer test modules/gotify_test.j
#
# The overlay splices gotify.j in front of this file, so the tests reach its
# private form-body builder by bare identifier. The form encoding itself lives
# in the shared `uri` module (tested in uri_test.j); here we verify formBody
# stitches the encoded title / message / priority together. The networked push
# (the POST with the X-Gotify-Key header) is verified against an in-process HTTP
# server in the Go suite (TestGotifyPush).
use testing;

func testFormBody() {
    # space -> "+", "&" -> "%26" (form encoding via uri.encodeForm)
    testing.assertEqual(formBody("Hi there", "a&b", 5), "title=Hi+there&message=a%26b&priority=5");
}

# --- JSON body + extras (gotify.j's `use json;` is in scope after the splice) ---

func testJsonBodyBase() {
    # No extras set: title / message / priority present, no `extras` object.
    def e as Extras init Extras{markdown: false, clickUrl: ""};
    def doc as json.Value init json.decode(jsonBody("Deploy", "build 1234 is live", 5, $e));
    testing.assertEqual(json.asString($doc, "/title"), "Deploy");
    testing.assertEqual(json.asString($doc, "/message"), "build 1234 is live");
    testing.assertEqual(json.asInt($doc, "/priority"), 5);
    testing.assertEqual(json.has($doc, "/extras"), false);
}

func testJsonBodyMarkdown() {
    def e as Extras init Extras{markdown: true, clickUrl: ""};
    def doc as json.Value init json.decode(jsonBody("T", "M", 5, $e));
    testing.assertEqual(
        json.asString($doc, "/extras/client::display/contentType"),
        "text/markdown");
    # click extras absent when no URL is given
    testing.assertEqual(json.has($doc, "/extras/client::notification"), false);
}

func testJsonBodyClick() {
    def e as Extras init Extras{markdown: false, clickUrl: "https://x.example/go"};
    def doc as json.Value init json.decode(jsonBody("T", "M", 4, $e));
    testing.assertEqual(
        json.asString($doc, "/extras/client::notification/click/url"),
        "https://x.example/go");
    # markdown display extras absent when markdown is false
    testing.assertEqual(json.has($doc, "/extras/client::display"), false);
}

func testJsonBodyBoth() {
    def e as Extras init Extras{markdown: true, clickUrl: "https://x.example/go"};
    def doc as json.Value init json.decode(jsonBody("Deploy", "done", 8, $e));
    testing.assertEqual(
        json.asString($doc, "/extras/client::display/contentType"),
        "text/markdown");
    testing.assertEqual(
        json.asString($doc, "/extras/client::notification/click/url"),
        "https://x.example/go");
}
