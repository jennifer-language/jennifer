# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
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
    def u as Url init parseUrl("http://host"); # no path -> "/"
    testing.assertEqual($u.path, "/");
    def h as Url init parseUrl("https://host"); # https default port
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
    testing.assertEqual(hostHeader(parseUrl("http://h/")), "h"); # default port omitted
    testing.assertEqual(hostHeader(parseUrl("https://h/")), "h");
    testing.assertEqual(hostHeader(parseUrl("http://h:8080/")), "h:8080");
}

func testBuildRequestGet() {
    def req as string init buildRequest("GET", parseUrl("http://h/p"), {}, "");
    testing.assertTrue(strings.startsWith($req, "GET /p HTTP/1.1\r\n"));
    testing.assertContains($req, "Host: h\r\n");
    testing.assertContains($req, "Connection: close\r\n");
    testing.assertContains($req, "User-Agent: jennifer-http\r\n");
    testing.assertTrue(strings.endsWith($req, "\r\n\r\n")); # no body
}

func testBuildRequestPostBody() {
    def hdrs as map of string to string init {"Content-Type": "application/json"};
    def req as string init buildRequest("POST", parseUrl("http://h/i"), $hdrs, '{}');
    testing.assertContains($req, "Content-Type: application/json\r\n");
    testing.assertContains($req, "Content-Length: 2\r\n"); # "{}" is 2 bytes
    testing.assertTrue(strings.endsWith($req, "\r\n\r\n\{\}"));
}

func testBuildRequestPatch() {
    def hdrs as map of string to string init {"Content-Type": "application/json"};
    def req as string init buildRequest("PATCH", parseUrl("http://h/i"), $hdrs, '{}');
    testing.assertTrue(strings.startsWith($req, "PATCH /i HTTP/1.1\r\n"));
    testing.assertContains($req, "Content-Length: 2\r\n");
}

func testBuildRequestOptions() {
    def req as string init buildRequest("OPTIONS", parseUrl("http://h/i"), {}, "");
    testing.assertTrue(strings.startsWith($req, "OPTIONS /i HTTP/1.1\r\n"));
    testing.assertTrue(strings.endsWith($req, "\r\n\r\n")); # no body
}

func testParseResponseContentLength() {
    def raw as bytes init convert.bytesFromString(
        "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 5\r\n\r\nhello",
        "utf-8");
    def r as Response init parseResponse($raw);
    testing.assertEqual($r.status, 200);
    testing.assertEqual($r.statusText, "OK");
    testing.assertEqual($r.headers["content-type"], "text/plain");
    testing.assertEqual($r.body, "hello");
}

func testParseResponseChunked() {
    def raw as bytes init convert.bytesFromString(
        "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n" +
            "5\r\nhello\r\n6\r\n world\r\n0\r\n\r\n",
        "utf-8");
    def r as Response init parseResponse($raw);
    testing.assertEqual($r.body, "hello world");
}

func testParseResponseStatusText() {
    def raw as bytes init convert.bytesFromString(
        "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n",
        "utf-8");
    def r as Response init parseResponse($raw);
    testing.assertEqual($r.status, 404);
    testing.assertEqual($r.statusText, "Not Found");
    testing.assertEqual($r.body, "");
}

# A status line with no reason phrase ("HTTP/1.1 200\r\n") must parse (the code
# is the whole remainder, the reason phrase empty) rather than throw.
func testParseResponseEmptyReasonPhrase() {
    def raw as bytes init convert.bytesFromString(
        "HTTP/1.1 200\r\nContent-Length: 0\r\n\r\n",
        "utf-8");
    def r as Response init parseResponse($raw);
    testing.assertEqual($r.status, 200);
    testing.assertEqual($r.statusText, "");
}

func testHeaderLookup() {
    def raw as bytes init convert.bytesFromString(
        "HTTP/1.1 204 No Content\r\nX-Test: abc\r\n\r\n",
        "utf-8");
    def r as Response init parseResponse($raw);
    testing.assertEqual(header($r, "x-test"), "abc");
    testing.assertEqual(header($r, "X-TEST"), "abc"); # case-insensitive
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

# ---- CRLF injection via the HTTP method ----

func injectMethod() {
    def u as Url init parseUrl("http://example.com/");
    buildRequest("GET\r\nX-Injected: 1", $u, {}, "");
}
func testMethodRejectsCrlf() {
    testing.assertThrows("injectMethod", "http");
}
func testCleanMethodAccepted() {
    def u as Url init parseUrl("http://example.com/");
    testing.assertContains(buildRequest("GET", $u, {}, ""), "GET / HTTP/1.1");
}
func testTlsOptionsZeroVerifies() {
    # The zero TlsOptions (what a plain request uses) full-verifies: skipVerify
    # off, no extra CA. The live skipVerify / caCert paths are in the Go suite.
    def o as TlsOptions;
    testing.assertEqual($o.skipVerify, false);
    testing.assertEqual(len($o.caCert), 0);
}
func testParseRawKeepsBinaryBody() {
    # parseRaw must return the body as raw bytes untouched - the whole point of
    # the byte path. Feed it a response whose body is not valid UTF-8 (a 0xff
    # byte) and confirm parseRaw preserves it, where parseResponse would throw.
    def raw as bytes;
    def head as bytes init convert.bytesFromString(
        "HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\n",
        "utf-8");
    def i as int init 0;
    while ($i < len($head)) {
        $raw[] = $head[$i];
        $i = $i + 1;
    }
    $raw[] = 0xff; # invalid as UTF-8 lead byte
    $raw[] = 0x00;
    $raw[] = 0x41;
    def r as BytesResponse init parseRaw($raw);
    testing.assertEqual($r.status, 200);
    testing.assertEqual(len($r.body), 3);
    testing.assertEqual($r.body[0], 255);
    testing.assertEqual($r.body[1], 0);
    testing.assertEqual($r.body[2], 65);
}

func testParseUrlQueryOnly() { # OM-020
    def u as Url init parseUrl("http://host?q=1&k=2");
    testing.assertEqual($u.host, "host");
    testing.assertEqual($u.path, "/?q=1&k=2");
    def u2 as Url init parseUrl("http://host:8080/p?x=1");
    testing.assertEqual($u2.host, "host");
    testing.assertEqual($u2.port, 8080);
    testing.assertEqual($u2.path, "/p?x=1");
}

# ---- Basic-auth helper ----

func testBasicAuth() {
    # base64("aladdin:opensesame")
    testing.assertEqual(basic("aladdin", "opensesame"), "Basic YWxhZGRpbjpvcGVuc2VzYW1l");
}

# ---- keep-alive request line: Connection header ----

func testBuildHeadKeepAlive() {
    def head as string init buildHeadConn("GET", parseUrl("http://h/p"), {}, 0, true);
    testing.assertEqual(strings.contains($head, "Connection: keep-alive"), true);
    def headClose as string init buildHeadConn("GET", parseUrl("http://h/p"), {}, 0, false);
    testing.assertEqual(strings.contains($headClose, "Connection: close"), true);
}

# ---- cookie jar ----

func testSetCookiesFromRawUnfolds() {
    # two Set-Cookie lines must survive as two entries (not comma-folded into one)
    def raw as bytes init convert.bytesFromString(
        "HTTP/1.1 200 OK\r\n" +
            "Set-Cookie: a=1; Path=/\r\n" +
            "Set-Cookie: b=2; HttpOnly\r\n" +
            "Content-Length: 0\r\n\r\n",
        "utf-8");
    def cs as list of string init setCookiesFromRaw($raw);
    testing.assertEqual(len($cs), 2);
    testing.assertEqual($cs[0], "a=1; Path=/");
    testing.assertEqual($cs[1], "b=2; HttpOnly");
}

func testJarAddAndHeader() {
    def jar as map of string to string init {};
    $jar = jarAdd($jar, "a=1; Path=/");
    $jar = jarAdd($jar, "b=2; HttpOnly");
    $jar = jarAdd($jar, "a=9"); # replaces a
    testing.assertEqual($jar["a"], "9");
    testing.assertEqual($jar["b"], "2");
    # jarHeader joins name=value with "; " (insertion order: a then b)
    testing.assertEqual(jarHeader($jar), "a=9; b=2");
}

func testJarAddIgnoresMalformed() {
    def jar as map of string to string init {};
    $jar = jarAdd($jar, "novalue"); # no "=" -> ignored
    testing.assertEqual(len($jar), 0);
}

# ---- redirect classification + Location resolution ----

func testIsRedirect() {
    testing.assertEqual(isRedirect(301), true);
    testing.assertEqual(isRedirect(308), true);
    testing.assertEqual(isRedirect(200), false);
    testing.assertEqual(isRedirect(404), false);
}

# Credential headers (any case) are dropped; ordinary headers are kept - the
# sanitization applied before a cross-origin redirect hop.
func testStripCredentialHeaders() {
    def h as map of string to string init {"authorization": "Bearer x", "COOKIE": "a=1",
        "Accept": "application/json", "X-Custom": "y"};
    def s as map of string to string init stripCredentialHeaders($h);
    testing.assertFalse(maps.has($s, "authorization"));
    testing.assertFalse(maps.has($s, "COOKIE"));
    testing.assertTrue(maps.has($s, "Accept"));
    testing.assertTrue(maps.has($s, "X-Custom"));
}

# Origin comparison drives the cross-origin decision: same host/scheme = same
# origin; a different host is cross-origin (credentials would be dropped).
func testOriginComparison() {
    testing.assertTrue(originOf("https://api.example.com/a") == originOf("https://api.example.com/b"));
    testing.assertFalse(originOf("https://api.example.com/a") == originOf("https://evil.example.net/a"));
}

func testResolveLocationAbsolute() {
    testing.assertEqual(resolveLocation("http://a.com/x", "https://b.com/y"), "https://b.com/y");
}

func testResolveLocationAbsolutePath() {
    testing.assertEqual(resolveLocation("http://a.com:8080/x/y", "/z"), "http://a.com:8080/z");
}

func testResolveLocationRelativePath() {
    testing.assertEqual(resolveLocation("http://a.com/x/y", "z"), "http://a.com/x/z");
}

# ---- retry classification + backoff ----

func testIsRetryable() {
    testing.assertEqual(isRetryable(429), true);
    testing.assertEqual(isRetryable(503), true);
    testing.assertEqual(isRetryable(500), true);
    testing.assertEqual(isRetryable(404), false);
    testing.assertEqual(isRetryable(200), false);
}

func testRetryDelayExponential() {
    def eb as bytes;
    def r as BytesResponse init BytesResponse{status: 503, statusText: "x", headers: {}, body: $eb};
    # base 100ms doubles per attempt: 100, 200, 400
    testing.assertEqual(retryDelayMs($r, 0, 100), 100);
    testing.assertEqual(retryDelayMs($r, 1, 100), 200);
    testing.assertEqual(retryDelayMs($r, 2, 100), 400);
}

func testRetryDelayHonoursRetryAfter() {
    def eb as bytes;
    def r as BytesResponse init BytesResponse{
        status: 429,
        statusText: "x",
        headers: {"retry-after": "2"},
        body: $eb
    };
    # Retry-After 2s (2000ms) beats the 100ms base
    testing.assertEqual(retryDelayMs($r, 0, 100), 2000);
}

func testRetryDelayClamped() {
    def eb as bytes;
    def r as BytesResponse init BytesResponse{
        status: 429,
        statusText: "x",
        headers: {"retry-after": "99999"},
        body: $eb
    };
    testing.assertEqual(retryDelayMs($r, 0, 100), MAX_BACKOFF_MS);
}

# ---- framed-read completeness helpers ----

func testEndsWithDoubleCRLF() {
    testing.assertEqual(endsWithDoubleCRLF(convert.bytesFromString("x\r\n\r\n", "utf-8")), true);
    testing.assertEqual(endsWithDoubleCRLF(convert.bytesFromString("x\r\n", "utf-8")), false);
}

func testChunkedComplete() {
    # a full chunked body (with terminal 0-chunk) is complete; a partial one isn't
    def full as bytes init convert.bytesFromString("hh\r\n\r\n5\r\nhello\r\n0\r\n\r\n", "utf-8");
    # bodyStart = 4 (after the leading "hh\r\n\r\n" header terminator at index 2)
    def hdrEnd as int init headerEnd($full);
    testing.assertEqual(chunkedComplete($full, $hdrEnd + 4), true);
    def partial as bytes init convert.bytesFromString("hh\r\n\r\n5\r\nhel", "utf-8");
    testing.assertEqual(chunkedComplete($partial, headerEnd($partial) + 4), false);
}

func testResponseClosesConn() {
    testing.assertEqual(responseClosesConn({"connection": "close"}), true);
    testing.assertEqual(responseClosesConn({"content-length": "5"}), false);
    testing.assertEqual(responseClosesConn({"transfer-encoding": "chunked"}), false);
    # no framing header at all -> body ran to EOF -> connection is closed
    testing.assertEqual(responseClosesConn({}), true);
}
