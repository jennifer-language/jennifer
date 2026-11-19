# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
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
# server in the Go suite (TestGotifyPush). The transport-security check
# (checkUrl) is tested here: a refused URL throws before any connection.
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

# --- transport security ------------------------------------------------

# refusal returns the kind checkUrl throws for url, or "" when it passes.
func refusal(url as string, insecure as bool) {
    try {
        checkUrl(Config{url: $url, token: "tok", allowInsecureHttp: $insecure});
    } catch (e) {
        return $e.kind;
    }
    return "";
}

func testPlainHttpToALanHostIsRefused() {
    for (def url in [
        "http://192.168.1.10:8080",
        "HTTP://unraid.local",
        "http://localhost.example.com",
        "http://127.0.0.1@evil.example",
        "http://127.0.0.1.evil.example",
        "http://128.0.0.1",
        "http://127.0.0.256",
        "http://[::2]",
        "htps://push.example.com",
        "push.example.com",
        ""
    ]) {
        testing.assertEqual(refusal($url, false), "gotify");
    }
}

func testHttpsAndLoopbackAreAllowed() {
    for (def url in [
        "https://push.example.com",
        "HTTPS://push.example.com",
        "http://127.0.0.1:8080",
        "http://127.1.2.3",
        "http://localhost",
        "http://LOCALHOST:80/gotify",
        "http://user:pass@127.0.0.1",
        "http://[::1]:9"
    ]) {
        testing.assertEqual(refusal($url, false), "");
    }
}

func testAllowInsecureHttpOptsIn() {
    testing.assertEqual(refusal("http://192.168.1.10:8080", true), "");
    testing.assertEqual(refusal("http://tower.lan", true), "");
    # The opt-in is for plain http only; another scheme is still refused.
    testing.assertEqual(refusal("ftp://192.168.1.10", true), "gotify");
}

func testPushRefusesPlainHttpBeforeSending() {
    # gotify.invalid never resolves: a "gotify" kind proves the refusal came first.
    def g as Config init Config{
        url: "http://gotify.invalid",
        token: "tok",
        allowInsecureHttp: false
    };
    def kinds as list of string;
    try {
        push($g, "T", "M", 1);
    } catch (e) {
        $kinds[] = $e.kind;
    }
    try {
        pushExtras($g, "T", "M", 1, Extras{markdown: true, clickUrl: ""});
    } catch (e) {
        $kinds[] = $e.kind;
    }
    testing.assertEqual($kinds, ["gotify", "gotify"]);
}
