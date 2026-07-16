# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# http_test.j - white-box tests for http.j's pure helpers. Run with:
#
#     jennifer test modules/http_test.j
#
# The overlay splices http.j in front of this file, so the tests reach its
# private URL parser, request builder, and response parser (including chunked
# decoding) by bare identifier. The networked request path is verified against
# an in-process HTTP server in the Go suite (TestHttpClient).
use testing;

func testParseUrlHttp() {
    def u as Url init parseUrl("http://example.com/path?q=1");
    testing.assertEqual($u.scheme, "http");
    testing.assertEqual($u.host, "example.com");
    testing.assertEqual($u.port, 80);
    testing.assertEqual($u.path, "/path?q=1");
}

func testParseUrlHttpsPort() {
    def u as Url init parseUrl("https://api.test:8443/v/x");
    testing.assertEqual($u.scheme, "https");
    testing.assertEqual($u.host, "api.test");
    testing.assertEqual($u.port, 8443);
    testing.assertEqual($u.path, "/v/x");
}

func testParseUrlDefaults() {
    def u as Url init parseUrl("http://host");     # no path -> "/"
    testing.assertEqual($u.path, "/");
    def h as Url init parseUrl("https://host");     # https default port
    testing.assertEqual($h.port, 443);
}

# IPv6 literal hosts, userinfo, and fragments.
func testParseUrlBracketHostUserinfoFragment() {
    # Bracketed IPv6 with a port: the inner colons are not the port separator.
    def a as Url init parseUrl("http://[::1]:8080/x");
    testing.assertEqual($a.host, "[::1]");
    testing.assertEqual($a.port, 8080);
    testing.assertEqual($a.path, "/x");
    testing.assertEqual(hostHeader($a), "[::1]:8080");
    # Userinfo is stripped at the last '@'; the fragment never reaches the path.
    def b as Url init parseUrl("http://user:p@ss@host:9/p#frag");
    testing.assertEqual($b.host, "host");
    testing.assertEqual($b.port, 9);
    testing.assertEqual($b.path, "/p");
    # An IPv6 host with no port keeps its brackets and the scheme default.
    def c as Url init parseUrl("http://[fe80::1]/");
    testing.assertEqual($c.host, "[fe80::1]");
    testing.assertEqual($c.port, 80);
}

func testHostHeader() {
    testing.assertEqual(hostHeader(parseUrl("http://h/")), "h");        # default port omitted
    testing.assertEqual(hostHeader(parseUrl("https://h/")), "h");
    testing.assertEqual(hostHeader(parseUrl("http://h:8080/")), "h:8080");
}

func testBuildRequestGet() {
    def req as string init buildRequest("GET", parseUrl("http://h/p"), {}, "");
    testing.assertTrue(strings.startsWith($req, "GET /p HTTP/1.1\r\n"));
    testing.assertContains($req, "Host: h\r\n");
    testing.assertContains($req, "Connection: close\r\n");
    testing.assertContains($req, "User-Agent: jennifer-http\r\n");
    testing.assertTrue(strings.endsWith($req, "\r\n\r\n"));    # no body
}

func testBuildRequestPostBody() {
    def hdrs as map of string to string init {"Content-Type": "application/json"};
    def req as string init buildRequest("POST", parseUrl("http://h/i"), $hdrs, "{}");
    testing.assertContains($req, "Content-Type: application/json\r\n");
    testing.assertContains($req, "Content-Length: 2\r\n");     # "{}" is 2 bytes
    testing.assertTrue(strings.endsWith($req, "\r\n\r\n{}"));
}

func testBuildRequestPatch() {
    def hdrs as map of string to string init {"Content-Type": "application/json"};
    def req as string init buildRequest("PATCH", parseUrl("http://h/i"), $hdrs, "{}");
    testing.assertTrue(strings.startsWith($req, "PATCH /i HTTP/1.1\r\n"));
    testing.assertContains($req, "Content-Length: 2\r\n");
}

func testBuildRequestOptions() {
    def req as string init buildRequest("OPTIONS", parseUrl("http://h/i"), {}, "");
    testing.assertTrue(strings.startsWith($req, "OPTIONS /i HTTP/1.1\r\n"));
    testing.assertTrue(strings.endsWith($req, "\r\n\r\n"));      # no body
}

func testParseResponseContentLength() {
    def raw as bytes init convert.bytesFromString(
        "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 5\r\n\r\nhello", "utf-8");
    def r as Response init parseResponse($raw);
    testing.assertEqual($r.status, 200);
    testing.assertEqual($r.statusText, "OK");
    testing.assertEqual($r.headers["content-type"], "text/plain");
    testing.assertEqual($r.body, "hello");
}

func testParseResponseChunked() {
    def raw as bytes init convert.bytesFromString(
        "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n" +
        "5\r\nhello\r\n6\r\n world\r\n0\r\n\r\n", "utf-8");
    def r as Response init parseResponse($raw);
    testing.assertEqual($r.body, "hello world");
}

func testParseResponseStatusText() {
    def raw as bytes init convert.bytesFromString(
        "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n", "utf-8");
    def r as Response init parseResponse($raw);
    testing.assertEqual($r.status, 404);
    testing.assertEqual($r.statusText, "Not Found");
    testing.assertEqual($r.body, "");
}

# A status line with no reason phrase ("HTTP/1.1 200\r\n") must parse (the code
# is the whole remainder, the reason phrase empty) rather than throw.
func testParseResponseEmptyReasonPhrase() {
    def raw as bytes init convert.bytesFromString(
        "HTTP/1.1 200\r\nContent-Length: 0\r\n\r\n", "utf-8");
    def r as Response init parseResponse($raw);
    testing.assertEqual($r.status, 200);
    testing.assertEqual($r.statusText, "");
}

func testHeaderLookup() {
    def raw as bytes init convert.bytesFromString(
        "HTTP/1.1 204 No Content\r\nX-Test: abc\r\n\r\n", "utf-8");
    def r as Response init parseResponse($raw);
    testing.assertEqual(header($r, "x-test"), "abc");
    testing.assertEqual(header($r, "X-TEST"), "abc");     # case-insensitive
    testing.assertEqual(header($r, "missing"), "");
}

# A header value carrying CRLF must be rejected: concatenated onto the wire it
# would inject an extra header or smuggle a second request (request splitting).
func injectViaHeaderValue() {
    def hdrs as map of string to string init {"X-Evil": "a\r\nX-Injected: yes"};
    buildRequest("GET", parseUrl("http://h/p"), $hdrs, "");
}

func injectViaHeaderName() {
    def hdrs as map of string to string init {"X\r\nInjected": "v"};
    buildRequest("GET", parseUrl("http://h/p"), $hdrs, "");
}

func injectViaPath() {
    def u as Url init Url{scheme: "http", host: "h", port: 80, path: "/p\r\nX-Injected: yes"};
    buildRequest("GET", $u, {}, "");
}

func testRejectsHeaderInjection() {
    testing.assertThrows("injectViaHeaderValue", "http");
    testing.assertThrows("injectViaHeaderName", "http");
    testing.assertThrows("injectViaPath", "http");
}
