# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
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
use task;
use httpd;
use io;

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

func testCallerHostOnlyForTheSameAuthority() {
    # the URL's own host with the default port spelled out is sent as written
    def a as string init buildRequest(
        "GET",
        parseUrl("http://10.0.0.5/p"),
        {"host": "10.0.0.5:80"},
        "");
    testing.assertContains($a, "Host: 10.0.0.5:80\r\n");
    testing.assertFalse(strings.contains($a, "Host: 10.0.0.5\r\n"));
    def b as string init buildRequest(
        "GET",
        parseUrl("https://Scan.lan:8443/"),
        {"Host": "scan.LAN:8443"},
        "");
    testing.assertContains($b, "Host: scan.LAN:8443\r\n");
    # another host, or another port, is dropped for the default
    def c as string init buildRequest("GET", parseUrl("http://h/"), {"Host": "evil"}, "");
    testing.assertContains($c, "Host: h\r\n");
    testing.assertFalse(strings.contains($c, "evil"));
    def d as string init buildRequest("GET", parseUrl("http://h/"), {"Host": "h:81"}, "");
    testing.assertContains($d, "Host: h\r\n");
    testing.assertFalse(strings.contains($d, "h:81"));
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
        "utf-8",
        "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 5\r\n\r\nhello");
    def r as Response init parseResponse($raw);
    testing.assertEqual($r.status, 200);
    testing.assertEqual($r.statusText, "OK");
    testing.assertEqual($r.headers["content-type"], "text/plain");
    testing.assertEqual($r.body, "hello");
}

func testParseResponseChunked() {
    def raw as bytes init convert.bytesFromString(
        "utf-8",
        "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n" +
            "5\r\nhello\r\n6\r\n world\r\n0\r\n\r\n");
    def r as Response init parseResponse($raw);
    testing.assertEqual($r.body, "hello world");
}

func testParseResponseStatusText() {
    def raw as bytes init convert.bytesFromString(
        "utf-8",
        "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n");
    def r as Response init parseResponse($raw);
    testing.assertEqual($r.status, 404);
    testing.assertEqual($r.statusText, "Not Found");
    testing.assertEqual($r.body, "");
}

# A status line with no reason phrase ("HTTP/1.1 200\r\n") must parse (the code
# is the whole remainder, the reason phrase empty) rather than throw.
func testParseResponseEmptyReasonPhrase() {
    def raw as bytes init convert.bytesFromString(
        "utf-8",
        "HTTP/1.1 200\r\nContent-Length: 0\r\n\r\n");
    def r as Response init parseResponse($raw);
    testing.assertEqual($r.status, 200);
    testing.assertEqual($r.statusText, "");
}

func testHeaderLookup() {
    def raw as bytes init convert.bytesFromString(
        "utf-8",
        "HTTP/1.1 204 No Content\r\nX-Test: abc\r\n\r\n");
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
    def head as bytes init convert.bytesFromString(
        "utf-8",
        "HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\n");
    def raw as bytes init $head;
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
        "utf-8",
        "HTTP/1.1 200 OK\r\n" +
            "Set-Cookie: a=1; Path=/\r\n" +
            "Set-Cookie: b=2; HttpOnly\r\n" +
            "Content-Length: 0\r\n\r\n");
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

# A cross-origin redirect hop keeps only the allowlisted headers (any case):
# Authorization, Cookie, a caller-set Host and every custom header, which may
# hold an API key, are dropped.
func testStripCredentialHeaders() {
    def h as map of string to string init {
        "authorization": "Bearer x",
        "COOKIE": "a=1",
        "Proxy-Authorization": "Basic eA==",
        "PRIVATE-TOKEN": "glpat-x",
        "X-API-Key": "k",
        "x-goog-api-key": "g",
        "api-key": "a",
        "Host": "api.example.com",
        "Accept": "application/json",
        "User-Agent": "j/1",
        "If-None-Match": "\"e1\"",
        "Range": "bytes=0-9"
    };
    def s as map of string to string init stripCredentialHeaders($h);
    testing.assertEqual(
        lists.sort(maps.keys($s)),
        ["Accept", "If-None-Match", "Range", "User-Agent"]);
}

# Origin comparison drives the cross-origin decision: same host/scheme = same
# origin; a different host is cross-origin (credentials would be dropped).
func testOriginComparison() {
    testing.assertTrue(originOf("https://api.example.com/a") ==
        originOf("https://api.example.com/b"));
    testing.assertFalse(originOf("https://api.example.com/a") ==
        originOf("https://evil.example.net/a"));
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

func testResolveLocationNetworkPath() {
    testing.assertEqual(resolveLocation("https://a.com/x", "//b.com/y"), "https://b.com/y");
}

# ---- scheme refusal ----

# schemeError returns the message parseUrl throws for url, or "" if it parses.
func schemeError(url as string) {
    try {
        parseUrl($url);
    } catch (e) {
        testing.assertEqual($e.kind, "http");
        return $e.message;
    }
    return "";
}

func testParseUrlRefusesMissingScheme() {
    def msg as string init schemeError("hooks.example.com/services/T/B/secret");
    testing.assertContains($msg, "no scheme");
    testing.assertFalse(strings.contains($msg, "secret")); # the URL is kept out
}

func testParseUrlRefusesUnknownScheme() {
    testing.assertContains(
        schemeError("htps://hooks.example.com/secret"),
        "unsupported URL scheme");
    testing.assertContains(schemeError("ftp://example.com/x"), "unsupported URL scheme");
    testing.assertFalse(strings.contains(schemeError("htps://h/secret"), "secret"));
}

func testParseUrlSchemeIsCaseInsensitive() {
    testing.assertEqual(parseUrl("HTTPS://h/p").scheme, "https");
    testing.assertEqual(parseUrl("HTTPS://h/p").port, 443);
}

func testIsDowngrade() {
    testing.assertTrue(isDowngrade("https://a.com/x", "http://a.com/x"));
    testing.assertFalse(isDowngrade("http://a.com/x", "https://a.com/x"));
    testing.assertFalse(isDowngrade("https://a.com/x", "https://b.com/x"));
    testing.assertFalse(isDowngrade("http://a.com/x", "http://b.com/x"));
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

# ---- chunked framing ----

func scanAll(text as string, step as int) {
    def b as bytes init convert.bytesFromString("utf-8", $text);
    def st as ChunkScan init ChunkScan{mode: 0, remaining: 0, line: $b[0..0], data: 0};
    def i as int init 0;
    while ($i < len($b)) {
        def end as int init $i + $step;
        if ($end > len($b)) {
            $end = len($b);
        }
        $st = scanChunks($st, $b[$i..$end], 0);
        $i = $end;
    }
    return $st.mode;
}

# The scan ends exactly after the terminal chunk and its trailers, however the
# bytes are split into reads, and data ending in CRLF does not fool it.
func testScanChunks() {
    def body as string init "5\r\nhello\r\n6;ext=1\r\nab\r\n\r\n\r\n0\r\nX-Sum: 1\r\n\r\n";
    for (def step in [1, 2, 3, 7, 1000]) {
        testing.assertEqual(scanAll($body, $step), 3);
        testing.assertEqual(scanAll($body[..len($body) - 2], $step), 2); # trailers not finished
    }
    testing.assertEqual(scanAll("6\r\nab\r\n\r\n\r\n", 1000), 0); # CRLFCRLF inside data: next size line awaited
    testing.assertThrows("scanBadSize", "http");
}

func scanBadSize() {
    scanAll("zz\r\nxyz\r\n0\r\n\r\n", 1000);
}

# A garbled size line or a chunk not ended by CRLF is an error, not a short body.
func testDechunkStrict() {
    testing.assertThrows("dechunkBadSize", "http");
    testing.assertThrows("dechunkEmptySize", "http");
    testing.assertThrows("dechunkNoCrlf", "http");
    testing.assertThrows("dechunkHugeSize", "http");
    def ok as bytes init dechunk(convert.bytesFromString("utf-8", "3;x=y\r\nabc\r\n0\r\n\r\n"));
    testing.assertEqual(convert.stringFromBytes("utf-8", $ok), "abc");
}

func dechunkBadSize() {
    dechunk(convert.bytesFromString("utf-8", "3\r\nabc\r\nzz\r\nxyz\r\n0\r\n\r\n"));
}

func dechunkEmptySize() {
    dechunk(convert.bytesFromString("utf-8", "3\r\nabc\r\n\r\nxyz\r\n0\r\n\r\n"));
}

func dechunkNoCrlf() {
    dechunk(convert.bytesFromString("utf-8", "3\r\nabcX\r\n0\r\n\r\n"));
}

func dechunkHugeSize() {
    dechunk(convert.bytesFromString("utf-8", "ffffffffffffffffffff\r\nabc\r\n0\r\n\r\n"));
}

func testResponseClosesConn() {
    testing.assertEqual(responseClosesConn({"connection": "close"}, false), true);
    testing.assertEqual(responseClosesConn({"content-length": "5"}, false), false);
    testing.assertEqual(responseClosesConn({"transfer-encoding": "chunked"}, false), false);
    # no framing header at all -> body ran to EOF -> connection is closed
    testing.assertEqual(responseClosesConn({}, false), true);
    # an option list, and HTTP/1.0 (persistent only with keep-alive)
    testing.assertEqual(
        responseClosesConn({"connection": "keep-alive, Close", "content-length": "5"}, false),
        true);
    testing.assertEqual(responseClosesConn({"content-length": "5"}, true), true);
    testing.assertEqual(
        responseClosesConn({"connection": "Keep-Alive", "content-length": "5"}, true),
        false);
}

# statusOf reads the code off a response head, with or without a reason phrase,
# and yields 0 for a line it cannot read (parseRaw then reports it).
func testStatusOf() {
    testing.assertEqual(statusOf("HTTP/1.1 304 Not Modified\r\nContent-Length: 3"), 304);
    testing.assertEqual(statusOf("HTTP/1.1 200\r\nContent-Length: 3"), 200);
    testing.assertEqual(statusOf("HTTP/1.1 100 Continue"), 100);
    testing.assertThrows("statusGarbage", "http");
    testing.assertThrows("statusLetters", "http");
}

func statusGarbage() {
    statusOf("garbage");
}

func statusLetters() {
    statusOf("HTTP/1.1 abc OK");
}

# hasNoBody: the HEAD answer and every 1xx / 204 / 304 carry no body.
func testHasNoBody() {
    testing.assertTrue(hasNoBody("HEAD", 200));
    testing.assertTrue(hasNoBody("head", 200));
    testing.assertTrue(hasNoBody("GET", 204));
    testing.assertTrue(hasNoBody("GET", 304));
    testing.assertTrue(hasNoBody("GET", 101));
    testing.assertFalse(hasNoBody("GET", 200));
    testing.assertFalse(hasNoBody("POST", 404));
}

# parseRaw does not dechunk an empty body: a HEAD answer may advertise chunked.
func testParseRawBodilessChunkedHead() {
    def r as BytesResponse init parseRaw(convert.bytesFromString(
        "utf-8",
        "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n"));
    testing.assertEqual($r.status, 200);
    testing.assertEqual(len($r.body), 0);
}

# ---- hardening ----

func parseResponseText(text as string) {
    return parseRaw(convert.bytesFromString("utf-8", $text));
}

func clNegative() {
    parseResponseText("HTTP/1.1 200 OK\r\nContent-Length: -5\r\n\r\nhello");
}

func clConflict() {
    parseResponseText("HTTP/1.1 200 OK\r\nContent-Length: 5\r\nContent-Length: 6\r\n\r\nhello");
}

func badStatus() {
    parseResponseText("HTTP/1.1 abc OK\r\n\r\n");
}

func testResponseFramingErrors() {
    testing.assertThrows("clNegative", "http");
    testing.assertThrows("clConflict", "http");
    testing.assertThrows("badStatus", "http");
    # two equal lengths fold to "5, 5", which is fine
    def r as BytesResponse init parseResponseText("HTTP/1.1 200 OK\r\nContent-Length: 5\r\nContent-Length: 5\r\n\r\nhello!");
    testing.assertEqual(len($r.body), 5);
}

func urlNoHost() {
    parseUrl("http:///x");
}

func urlEmptyUserHost() {
    parseUrl("http://@/x");
}

func urlBadPort() {
    parseUrl("http://h:abc/");
}

func urlPortRange() {
    parseUrl("http://h:99999/");
}

func urlSpaceHost() {
    parseUrl("http://h x/");
}

func urlOpenBracket() {
    parseUrl("http://[::1/x");
}

func testParseUrlRefusesBadAuthority() {
    testing.assertThrows("urlNoHost", "http");
    testing.assertThrows("urlEmptyUserHost", "http");
    testing.assertThrows("urlBadPort", "http");
    testing.assertThrows("urlPortRange", "http");
    testing.assertThrows("urlSpaceHost", "http");
    testing.assertThrows("urlOpenBracket", "http");
    testing.assertEqual(parseUrl("http://h:/x").port, 80); # empty port: the default
    testing.assertEqual(parseUrl("https://h:65535/").port, 65535);
}

func targetWithSpace() {
    buildRequest("GET", parseUrl("http://h/a b"), {}, "");
}

func targetNoSlash() {
    buildRequest("GET", Url{scheme: "http", host: "h", port: 80, path: "items"}, {}, "");
}

func methodWithSpace() {
    buildRequest("GET /x", parseUrl("http://h/"), {}, "");
}

func headerNameNotToken() {
    buildRequest("GET", parseUrl("http://h/"), {"X Bad": "v"}, "");
}

func testRequestLineChecks() {
    testing.assertThrows("targetWithSpace", "http");
    testing.assertThrows("targetNoSlash", "http");
    testing.assertThrows("methodWithSpace", "http");
    testing.assertThrows("headerNameNotToken", "http");
    testing.assertContains(
        buildRequest("OPTIONS", Url{scheme: "http", host: "h", port: 80, path: "*"}, {}, ""),
        "OPTIONS * HTTP/1.1");
}

# An empty POST / PUT / PATCH says Content-Length: 0; GET does not; a caller's
# Transfer-Encoding is not sent next to our Content-Length.
func testFramingHeaders() {
    for (def m in ["POST", "PUT", "PATCH", "post"]) {
        testing.assertContains(
            buildRequest($m, parseUrl("http://h/x"), {}, ""),
            "Content-Length: 0\r\n");
    }
    testing.assertFalse(strings.contains(
        buildRequest("GET", parseUrl("http://h/x"), {}, ""),
        "Content-Length"));
    def req as string init buildRequest(
        "POST",
        parseUrl("http://h/x"),
        {"Transfer-Encoding": "chunked"},
        "abc");
    testing.assertFalse(strings.contains(strings.lower($req), "transfer-encoding"));
    testing.assertContains($req, "Content-Length: 3\r\n");
}

# Location values resolve per RFC 3986.
func testResolveLocationRfc() {
    def cur as string init "http://h/a/b?x=1";
    testing.assertEqual(resolveLocation($cur, "/next?u=http://x"), "http://h/next?u=http://x");
    testing.assertEqual(resolveLocation($cur, "?page=2"), "http://h/a/b?page=2");
    testing.assertEqual(resolveLocation($cur, "../c"), "http://h/c");
    testing.assertEqual(resolveLocation($cur, "./d"), "http://h/a/d");
    testing.assertEqual(resolveLocation($cur, " /e "), "http://h/e");
}

# Neither many attempts nor a huge Retry-After overflows; both clamp.
func testRetryDelayNoOverflow() {
    def none as BytesResponse init parseResponseText("HTTP/1.1 503 x\r\n\r\n");
    testing.assertEqual(retryDelayMs($none, 70, 250), MAX_BACKOFF_MS);
    def huge as BytesResponse init parseResponseText("HTTP/1.1 503 x\r\nRetry-After: 99999999999999999999\r\n\r\n");
    testing.assertEqual(retryDelayMs($huge, 0, 100), MAX_BACKOFF_MS);
}

# A timed-out exchange must not leave its late answer on the socket for the
# next exchange to read as its own.
func testExchangeAfterTimeoutReconnects() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    def server as task of null init spawn {
        def first as net.Conn init net.accept($listener);
        net.readBytes($first, 4096);
        time.sleep(time.fromMilliseconds(500));
        try {
            net.writeBytes(
                $first,
                convert.bytesFromString(
                    "utf-8",
                    "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nfirst"));
        } catch (e) { # lint-disable: L103
            # the client has closed this socket
        }
        def second as net.Conn init net.accept($listener);
        net.readBytes($second, 4096);
        net.writeBytes(
            $second,
            convert.bytesFromString("utf-8", "HTTP/1.1 200 OK\r\nContent-Length: 6\r\n\r\nsecond"));
        net.close($first);
        net.close($second);
    };
    def o as Options init defaultOptions();
    $o.timeoutMs = 200;
    def s as Session init connect("http://" + $addr + "/", $o);
    def timedOut as bool init false;
    try {
        exchange($s, "GET", "/one", {}, "");
    } catch (e) {
        $timedOut = true;
    }
    testing.assertTrue($timedOut);
    time.sleep(time.fromMilliseconds(500));
    def x as Exchange init exchange($s, "GET", "/two", {}, "");
    testing.assertEqual($x.response.body, "second");
    close($x.session);
    close($s); # already closed: not an error
    task.wait($server);
    net.close($listener);
}

# --- HTTP Digest -------------------------------------------------------------

# The value of one parameter in a Digest Authorization header.
func digestParam(auth as string, name as string) {
    for (def c in authChallenges($auth)) {
        if (maps.has($c.params, $name)) {
            return $c.params[$name];
        }
    }
    return "";
}

func testDigestRfc2617Vector() {
    def c as AuthChallenge init pickDigest(
        'Digest realm="testrealm@host.com", qop="auth,auth-int", ' +
            'nonce="dcd98b7102dd2f0e8b11d0f600bfb0c093", opaque="5ccc069c403ebaf9f0171e9517f40e41"');
    def a as string init digestAnswer(
        $c,
        "GET",
        "/dir/index.html",
        "Mufasa",
        "Circle Of Life",
        "",
        "0a4f113b");
    testing.assertEqual(digestParam($a, "response"), "6629fae49393a05397450978507c4ef1");
    testing.assertEqual(digestParam($a, "qop"), "auth");
    testing.assertEqual(digestParam($a, "nc"), "00000001");
    testing.assertEqual(digestParam($a, "opaque"), "5ccc069c403ebaf9f0171e9517f40e41");
    testing.assertEqual(digestParam($a, "uri"), "/dir/index.html");
}

# RFC 7616 section 3.9.1: the server offers SHA-256 and MD5; SHA-256 wins.
func testDigestRfc7616Vectors() {
    def common as string init 'realm="http-auth@example.org", qop="auth, auth-int", ' +
        'nonce="7ypf/xlj9XXwfDPEoM4URrv/xwf94BcCAzFZH4GiTo0v", ' +
        'opaque="FQhe/qaU925kfnzjCev0ciny7QMkPqMAFRtzCUYo5tdS"';
    def both as string init "Digest " + $common + ", algorithm=SHA-256, Digest " + $common +
        ", algorithm=MD5";
    def cnonce as string init "f2/wE4q74E6zIJEtWaHKaf5wv/H5QzzpXusqGemxURZJ";
    def a as string init digestAnswer(
        pickDigest($both),
        "GET",
        "/dir/index.html",
        "Mufasa",
        "Circle of Life",
        "",
        $cnonce);
    testing.assertEqual(digestParam($a, "algorithm"), "SHA-256");
    testing.assertEqual(
        digestParam($a, "response"),
        "753927fa0e85d155564e2e272a28d1802ca10daf4496794697cf8db5856cb6c1");
    def md5 as string init digestAnswer(
        pickDigest("Digest " + $common + ", algorithm=MD5"),
        "GET",
        "/dir/index.html",
        "Mufasa",
        "Circle of Life",
        "",
        $cnonce);
    testing.assertEqual(digestParam($md5, "response"), "8ca523f5e9506fed4657c9700eebdbec");
}

# Vectors computed independently (Python hashlib): userhash, -sess with
# auth-int, and the RFC 2069 form without qop.
func testDigestVariants() {
    def uh as string init digestAnswer(
        pickDigest('Digest realm="api@example.org", nonce="n", qop=auth, algorithm=SHA-256, userhash=true'),
        "GET",
        "/",
        "Jäsøn Doe",
        "pw",
        "",
        "c");
    testing.assertEqual(
        digestParam($uh, "username"),
        "5a1a8a47df5c298551b9b42ba9b05835174a5bd7d511ff7fe9191d8e946fc4e7");
    testing.assertContains($uh, "userhash=true");
    def si as string init digestAnswer(
        pickDigest('Digest realm="r", nonce="n1", qop="auth-int", algorithm=SHA-256-sess'),
        "POST",
        "/x",
        "u",
        "p",
        "body",
        "c1");
    testing.assertEqual(digestParam($si, "qop"), "auth-int");
    testing.assertEqual(
        digestParam($si, "response"),
        "25185ec7021622e83d7808a8e70fd8e1e44506a1a5e82790b39378ab99570d98");
    def legacy as string init digestAnswer(
        pickDigest('Digest realm="r", nonce="n1"'),
        "GET",
        "/a",
        "u",
        "p",
        "",
        "c");
    testing.assertEqual(digestParam($legacy, "response"), "fc75c2c23bd81c0757cb57651b2e61ad");
    testing.assertFalse(strings.contains($legacy, "cnonce"));
}

# Several challenges in one value (joined headers), commas and escaped quotes
# inside quoted strings, a token68 scheme before them.
func testDigestChallengeParsing() {
    def cs as list of AuthChallenge init authChallenges(
        'Basic abc==, Digest realm="a, b \"c\"", nonce=xyz, Bearer realm="x"');
    testing.assertEqual(len($cs), 3);
    testing.assertEqual($cs[1].scheme, "Digest");
    testing.assertEqual($cs[1].params["realm"], "a, b \"c\"");
    testing.assertEqual($cs[1].params["nonce"], "xyz");
    testing.assertEqual($cs[2].params["realm"], "x");
    # The quoted realm comes back escaped.
    def a as string init digestAnswer(
        pickDigest('Digest realm="a, b \"c\"", nonce=xyz'),
        "GET",
        "/",
        "u",
        "p",
        "",
        "c");
    testing.assertContains($a, 'realm="a, b \"c\""');
}

func noUsableDigest() {
    pickDigest('Basic realm="x", Digest realm="r", nonce="n", algorithm=SHA-512-256, Digest nonce="m", qop=other');
}

func injectedNonce() {
    digestAnswer(
        pickDigest("Digest realm=\"r\", nonce=\"a\r\nX-Evil: 1\""),
        "GET",
        "/",
        "u",
        "p",
        "",
        "c");
}

func testDigestRefusals() {
    testing.assertThrows("noUsableDigest", "http");
    testing.assertThrows("injectedNonce", "http");
    try {
        noUsableDigest();
    } catch (e) {
        testing.assertContains($e.message, "no Digest challenge");
    }
    try {
        injectedNonce();
    } catch (e) {
        testing.assertContains($e.message, "control character");
    }
}

# A loopback server that answers each request with the next canned reply and
# hands back the Authorization headers it saw.
func digestServer(srv as httpd.Server, replies as list of list of string) {
    def seen as list of string init [];
    for (def r in $replies) {
        def req as httpd.Request init httpd.accept($srv);
        $seen[] = httpd.header($req, "Authorization");
        if ($r[1] != "") {
            httpd.setHeader($req, "WWW-Authenticate", $r[1]);
        }
        httpd.respond($req, convert.toInt($r[0]), $r[2]);
    }
    return $seen;
}

func digestRun(replies as list of list of string, password as string) {
    def srv as httpd.Server init httpd.listen("127.0.0.1:0");
    def t as task of list of string init spawn {
        return digestServer($srv, $replies);
    };
    def url as string init "http://" + httpd.address($srv) + "/cam/snap?ch=1";
    def r as Response init sendDigest(
        "GET",
        $url,
        {"Authorization": "Basic old"},
        "",
        defaultOptions(),
        "admin",
        $password);
    def seen as list of string init task.wait($t);
    httpd.shutdown($srv);
    def out as list of string init [$r.body, convert.toString($r.status)];
    for (def a in $seen) {
        $out[] = $a;
    }
    return $out;
}

def const CHALLENGE as string init 'Digest realm="cam", qop="auth", nonce="n1", algorithm=MD5';

func testSendDigestRoundTrip() {
    def out as list of string init digestRun(
        [["401", CHALLENGE, "no"], ["200", "", "image"]],
        "secret");
    testing.assertEqual($out[0], "image");
    # The first request carried no credentials (the caller's Authorization is replaced).
    testing.assertEqual($out[2], "");
    def auth as string init $out[3];
    testing.assertEqual(digestParam($auth, "uri"), "/cam/snap?ch=1");
    testing.assertEqual(len(digestParam($auth, "cnonce")), 32);
    # The answer checks out against the server's own computation.
    def want as string init digestAnswer(
        pickDigest(CHALLENGE),
        "GET",
        "/cam/snap?ch=1",
        "admin",
        "secret",
        "",
        digestParam($auth, "cnonce"));
    testing.assertEqual($auth, $want);
}

func testSendDigestStaleNonceRetriesOnce() {
    def stale as string init 'Digest realm="cam", qop="auth", nonce="n2", stale=true';
    def out as list of string init digestRun(
        [
            ["401", CHALLENGE, ""],
            ["401", $stale, ""],
            ["200", "", "ok"]
        ],
        "secret");
    testing.assertEqual($out[0], "ok");
    testing.assertEqual(digestParam($out[4], "nonce"), "n2");
}

func testSendDigestWrongPasswordStopsAfterOneAnswer() {
    def out as list of string init digestRun(
        [["401", CHALLENGE, ""], ["401", CHALLENGE, "denied"]],
        "wrong");
    testing.assertEqual($out[1], "401");
    testing.assertEqual($out[0], "denied");
    testing.assertEqual(len($out), 4);
}

func testSendDigestBasicOnlyIsReturned() {
    def out as list of string init digestRun([["401", 'Basic realm="x"', "basic only"]], "pw");
    testing.assertEqual($out[1], "401");
    testing.assertEqual($out[0], "basic only");
}

# --- Retry-After as an HTTP-date (M28.69) --------------------------------

# A Retry-After date 10 s ahead, in each of the three HTTP-date forms, is a
# wait of about 10 s; the obsolete forms read as the IMF-fixdate does.
func testRetryAfterHttpDate() {
    def at as time.Time init time.add(time.utc(), time.fromSeconds(10));
    def imf as string init time.format($at, "%a, %d %b %Y %H:%M:%S GMT");
    def ms as int init retryAfterMs($imf);
    testing.assertTrue($ms > 8000 and $ms <= 10000);
    def rfc850 as string init time.format($at, "%A, %d-%b-") +
        time.format($at, "%Y")[2..4] + time.format($at, " %H:%M:%S GMT");
    testing.assertEqual(httpDateStamp($rfc850), httpDateStamp($imf));
    def day as string init time.format($at, "%d");
    if (strings.startsWith($day, "0")) {
        $day = " " + $day[1..];
    }
    def asctime as string init time.format($at, "%a %b ") + $day + time.format($at, " %H:%M:%S %Y");
    testing.assertEqual(httpDateStamp($asctime), httpDateStamp($imf));
    def eb as bytes;
    def r as BytesResponse init BytesResponse{
        status: 503,
        statusText: "x",
        headers: {"retry-after": $imf},
        body: $eb
    };
    def delay as int init retryDelayMs($r, 0, 100);
    testing.assertTrue($delay > 8000 and $delay <= 10000);
}

# The examples of RFC 9110 5.6.7, and a two-digit year read within 50 years.
func testHttpDateForms() {
    testing.assertEqual(httpDateStamp("Sun, 06 Nov 1994 08:49:37 GMT"), "06 Nov 1994 08:49:37");
    testing.assertEqual(httpDateStamp("Sunday, 06-Nov-94 08:49:37 GMT"), "06 Nov 1994 08:49:37");
    testing.assertEqual(httpDateStamp("Sun Nov  6 08:49:37 1994"), "06 Nov 1994 08:49:37");
    testing.assertEqual(httpDateStamp("Sun Nov 16 08:49:37 1994"), "16 Nov 1994 08:49:37");
    def soon as int init (time.year(time.utc()) + 10) % 100;
    def yy as string init convert.toString($soon);
    if ($soon < 10) {
        $yy = "0" + $yy;
    }
    testing.assertEqual(
        httpDateStamp("Monday, 01-Jan-" + $yy + " 00:00:00 GMT")[7..11],
        convert.toString(time.year(time.utc()) + 10));
}

# Delay-seconds: leading zeros do not make a number long (0000001 is 1 s, not
# the cap); a genuinely large one is clamped to the cap without overflowing.
func testRetryAfterSeconds() {
    testing.assertEqual(retryAfterMs("0000001"), 1000);
    testing.assertEqual(retryAfterMs("00000000000000000000002"), 2000);
    testing.assertEqual(retryAfterMs("0000000"), 0);
    testing.assertEqual(retryAfterMs("0"), 0);
    testing.assertEqual(retryAfterMs("5"), 5000);
    testing.assertEqual(retryAfterMs("31"), MAX_BACKOFF_MS);
    testing.assertEqual(retryAfterMs("99999999999999999999999"), MAX_BACKOFF_MS);
}

# A passed date waits 0, one past `time`'s range (2262) the cap; a malformed
# one counts as absent, so the backoff alone decides.
func testRetryAfterDateEdges() {
    testing.assertEqual(retryAfterMs("Sun, 06 Nov 1994 08:49:37 GMT"), 0);
    testing.assertEqual(retryAfterMs("Mon, 01 Jan 1900 00:00:00 GMT"), 0);
    testing.assertEqual(retryAfterMs("Fri, 01 Jan 3000 00:00:00 GMT"), MAX_BACKOFF_MS);
    testing.assertEqual(retryAfterMs("Thu, 01 Jan 2099 00:00:00 GMT"), MAX_BACKOFF_MS);
    for (def bad in [
        "sun, 06 Nov 1994 08:49:37 GMT",
        "Sun, 06 Nov 1994 08:49:37 UTC",
        "Sun, 6 Nov 1994 08:49:37 GMT",
        "Sun, 31 Feb 2030 08:49:37 GMT",
        "Sun, 06 Nov 2030 08:49:60 GMT",
        "Sun Nov 6 08:49:37 1994",
        "2030-11-06T08:49:37Z",
        "-5",
        "soon"
    ]) {
        testing.assertEqual(retryAfterMs($bad), -1);
        def eb as bytes;
        def r as BytesResponse init BytesResponse{
            status: 503,
            statusText: "x",
            headers: {"retry-after": $bad},
            body: $eb
        };
        testing.assertEqual(retryDelayMs($r, 0, 100), 100);
    }
}

# --- transport failures (M28.65) -------------------------------------------

# serveOnce accepts one connection on `listener`, reads the request, writes
# `reply` (none when empty), waits a little so it arrives, and then closes the
# connection, or aborts it (a reset) when `reset` is set.
func serveOnce(listener as net.Listener, reply as string, reset as bool) {
    def c as net.Conn init net.accept($listener);
    net.readBytes($c, 4096);
    if ($reply != "") {
        net.writeBytes($c, convert.bytesFromString("utf-8", $reply));
        time.sleep(time.fromMilliseconds(100));
    }
    if ($reset) {
        net.abort($c);
    } else {
        net.close($c);
    }
}

# failureOf sends a GET to a server that answers with `reply` and then closes
# or resets, and returns "kind" or, for a NetError, "net op reason sent replied".
func failureOf(reply as string, reset as bool) {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    def server as task of null init spawn {
        serveOnce($listener, $reply, $reset);
    };
    def got as string init "no error";
    try {
        request("GET", "http://" + $addr + "/", {}, "");
    } catch (e) {
        $got = $e.kind;
        if ($e.kind == "net") {
            def ne as net.NetError init $e;
            $got = io.sprintf("net %s %s %v %v", $ne.op, $ne.reason, $ne.sent, $ne.replied);
        }
    }
    task.wait($server);
    net.close($listener);
    return $got;
}

# A server that closes or resets before any byte of its reply is a NetError
# saying the request was sent and nothing came back; one that cuts its reply
# short is kind "closed" (a reset: a NetError that says a reply came).
func testTransportFailureFacts() {
    testing.assertEqual(failureOf("", false), "net read closed true false");
    testing.assertEqual(failureOf("", true), "net read reset true false");
    testing.assertEqual(failureOf("HTTP/1.1 200 OK\r\n", false), "closed");
    testing.assertEqual(failureOf("HTTP/1.1 200 OK\r\n", true), "net read reset true true");
    testing.assertEqual(
        failureOf("HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\nabc", true),
        "net read reset true true");
    testing.assertEqual(
        failureOf("HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\nabc", false),
        "closed");
    testing.assertEqual(
        failureOf("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nab", false),
        "closed");
    testing.assertEqual(
        failureOf("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok", false),
        "no error");
}

# An uncapped body past net.readN's 256 MiB a call is read in pieces: a
# server that then closes early cuts the reply short ("closed"), where one
# oversized read was a runtime error before.
func testUncappedBodyPastReadLimit() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    def server as task of null init spawn {
        serveOnce($listener, "HTTP/1.1 200 OK\r\nContent-Length: 300000000\r\n\r\nabc", false);
    };
    def got as string init "no error";
    try {
        requestWith("GET", "http://" + $addr + "/", {}, "", 5000, -1);
    } catch (e) {
        $got = $e.kind;
    }
    task.wait($server);
    net.close($listener);
    testing.assertEqual($got, "closed");
}

# A refused connection is a NetError of the connect, nothing sent.
func testRefusedIsNetError() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    net.close($listener);
    def got as string init "no error";
    try {
        request("GET", "http://" + $addr + "/", {}, "");
    } catch (e) {
        def ne as net.NetError init $e;
        $got = io.sprintf("%s %s %s %v %v", $ne.kind, $ne.op, $ne.reason, $ne.sent, $ne.replied);
    }
    testing.assertEqual($got, "net connect refused false false");
}

# --- one body copy less (M28.70) -------------------------------------------

# A Content-Length body read exactly, over-read, or with the head and body in
# one read comes back whole and cut at its length.
func testContentLengthBodyCut() {
    def r as BytesResponse init parseResponseText("HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\nabcdef");
    testing.assertEqual(convert.stringFromBytes("utf-8", $r.body), "abc");
    def big as string init strings.repeat("x", 70000);
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    def server as task of null init spawn {
        serveOnce(
            $listener,
            "HTTP/1.1 200 OK\r\nContent-Length: 70000\r\n\r\n" + $big + "trailing",
            false);
    };
    def got as Response init request("GET", "http://" + $addr + "/", {}, "");
    task.wait($server);
    net.close($listener);
    testing.assertEqual(len($got.body), 70000);
    testing.assertEqual($got.body, $big);
}

# A positive timeout bounds the connect too: an address that never answers
# (10.255.255.1 drops the SYN) fails in about the timeout, not in 30 s. Where
# it is unreachable outright the connect fails at once, which passes as well.
func testTimeoutBoundsConnect() {
    def t0 as time.Time init time.now();
    def kind as string init "no error";
    try {
        requestWith("GET", "http://10.255.255.1/", {}, "", 300, 0);
    } catch (e) {
        $kind = $e.kind;
    }
    testing.assertTrue($kind == "timeout" or $kind == "net");
    testing.assertTrue(time.milliseconds(time.sub(time.now(), $t0)) < 10000);
}

# ---- strict response heads ----

# parseStrict parses `text` as a response and returns "ok" or the error kind.
func parseStrict(text as string) {
    try {
        parseResponseText($text);
    } catch (e) {
        return $e.kind;
    }
    return "ok";
}

# A malformed head is an error, kind "http": never a dropped line, a guessed
# protocol or an ignored transfer coding.
func testParseRawRejectsMalformedHeads() {
    testing.assertEqual(parseStrict("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"), "ok");
    testing.assertEqual(parseStrict("HTTP/1.0 200\r\nContent-Length: 2\r\n\r\nok"), "ok");
    testing.assertEqual(
        parseStrict("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nGarbageLine\r\n\r\nok"),
        "http");
    testing.assertEqual(
        parseStrict("HTTP/1.1 200 OK\r\nX-A: 1\r\n  folded\r\nContent-Length: 2\r\n\r\nok"),
        "http");
    testing.assertEqual(parseStrict("FOO/9 200 OK\r\nContent-Length: 2\r\n\r\nok"), "http");
    testing.assertEqual(parseStrict("HTTP/2 200 OK\r\nContent-Length: 2\r\n\r\nok"), "http");
    testing.assertEqual(parseStrict("HTTP/1.1 200 OK\r\nContent-Length : 2\r\n\r\nok"), "http");
    testing.assertEqual(
        parseStrict("HTTP/1.1 200 OK\r\n: x\r\nContent-Length: 2\r\n\r\nok"),
        "http");
    testing.assertEqual(
        parseStrict("HTTP/1.1 200 OK\r\nX-A: a\rb\r\nContent-Length: 2\r\n\r\nok"),
        "http");
}

# Only chunked may be applied as a transfer coding (the client sends no TE
# header), and never next to a Content-Length.
func testParseRawRejectsTransferCodings() {
    testing.assertEqual(
        parseStrict("HTTP/1.1 200 OK\r\nTransfer-Encoding: Chunked\r\n\r\n2\r\nok\r\n0\r\n\r\n"),
        "ok");
    testing.assertEqual(
        parseStrict("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked, gzip\r\n\r\n2\r\nok\r\n0\r\n\r\n"),
        "http");
    testing.assertEqual(
        parseStrict("HTTP/1.1 200 OK\r\nTransfer-Encoding: gzip\r\nContent-Length: 2\r\n\r\nok"),
        "http");
    testing.assertEqual(
        parseStrict("HTTP/1.1 200 OK\r\nTransfer-Encoding: gzip, chunked\r\n\r\n2\r\nok\r\n0\r\n\r\n"),
        "http");
    testing.assertEqual(
        parseStrict("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\nContent-Length: 2\r\n\r\n2\r\nok\r\n0\r\n\r\n"),
        "http");
}

func nonUtf8Head() {
    def raw as bytes init convert.bytesFromString("utf-8", "HTTP/1.1 200 OK\r\nX-Bin: ");
    $raw[] = 255;
    parseRaw(binary.concat($raw, convert.bytesFromString("utf-8", "\r\nContent-Length: 0\r\n\r\n")));
}

func nonUtf8Body() {
    def raw as bytes init convert.bytesFromString(
        "utf-8",
        "HTTP/1.1 200 OK\r\nContent-Length: 1\r\n\r\n");
    $raw[] = 255;
    parseResponse($raw);
}

# Bytes that are not UTF-8 in a head, or in a text Response's body, are a
# malformed response (kind "http"), not a runtime error.
func testNonUtf8IsHttpError() {
    testing.assertThrows("nonUtf8Head", "http");
    testing.assertThrows("nonUtf8Body", "http");
}

func testIsCleartextRemote() {
    testing.assertTrue(isCleartextRemote("http://192.0.2.7/api"));
    testing.assertTrue(isCleartextRemote("HTTP://example.com"));
    testing.assertTrue(isCleartextRemote("http://127.0.0.1@evil.example/"));
    testing.assertTrue(isCleartextRemote("http://127.0.0.256/"));
    testing.assertTrue(isCleartextRemote("http://localhost.evil.example/"));
    testing.assertFalse(isCleartextRemote("https://example.com/"));
    testing.assertFalse(isCleartextRemote("http://127.0.0.1:8080/"));
    testing.assertFalse(isCleartextRemote("http://127.9.8.7/"));
    testing.assertFalse(isCleartextRemote("http://LocalHost/"));
    testing.assertFalse(isCleartextRemote("http://[::1]:9/"));
    testing.assertThrows("cleartextNoScheme", "http");
}

func cleartextNoScheme() {
    isCleartextRemote("example.com/x");
}

# ---- stale keep-alive sockets ----

# serveIdleClose answers one request on each of two connections: the first
# with `first`, after which it closes the socket while the client holds it
# idle (a keep-alive timeout), the second with "two". It returns the request
# lines it saw.
func serveIdleClose(listener as net.Listener, accepts as int) {
    def seen as list of string init [];
    for (def i in 0..$accepts) {
        def c as net.Conn init net.accept($listener);
        def req as string init convert.stringFromBytes("utf-8", net.readBytes($c, 4096));
        $seen[] = strings.split($req, "\r\n")[0];
        def body as string init "one";
        if ($i > 0) {
            $body = "two";
        }
        net.writeBytes(
            $c,
            convert.bytesFromString(
                "utf-8",
                "HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\n" + $body));
        time.sleep(time.fromMilliseconds(50));
        net.close($c);
    }
    return $seen;
}

# staleExchange runs one exchange, lets the server drop the idle socket, then
# sends `method` on it, returning the body or the error kind.
func staleExchange(method as string, headers as map of string to string, accepts as int) {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    def server as task of list of string init spawn {
        return serveIdleClose($listener, $accepts);
    };
    def s as Session init connect("http://" + $addr, defaultOptions());
    def x as Exchange init exchange($s, "GET", "/a", {}, "");
    $s = $x.session;
    time.sleep(time.fromMilliseconds(250));
    def got as string init "";
    try {
        $x = exchange($s, $method, "/b", $headers, "");
        $got = $x.response.body;
        close($x.session);
    } catch (e) {
        $got = $e.kind;
        close($s);
        if ($accepts > 1) {
            # No resend reached the server: feed it the connection it waits
            # for, so a regression fails the assertion instead of hanging.
            def unblock as net.Conn init net.connect($addr, 1000);
            net.writeBytes($unblock, convert.bytesFromString("utf-8", "NONE / HTTP/1.1\r\n\r\n"));
            net.readAll($unblock, 4096, 1000);
            net.close($unblock);
        }
    }
    def seen as list of string init task.wait($server);
    net.close($listener);
    return $got + " " + strings.join($seen, ",");
}

# A reused socket the server closed while idle fails before any reply byte: an
# idempotent request (or one with an Idempotency-Key) is resent once on a new
# connection; a POST is not, since the server may already have acted on it.
func testExchangeResendsOnStaleKeepAlive() {
    testing.assertEqual(staleExchange("GET", {}, 2), "two GET /a HTTP/1.1,GET /b HTTP/1.1");
    testing.assertEqual(staleExchange("DELETE", {}, 2), "two GET /a HTTP/1.1,DELETE /b HTTP/1.1");
    testing.assertEqual(
        staleExchange("POST", {"Idempotency-Key": "k1"}, 2),
        "two GET /a HTTP/1.1,POST /b HTTP/1.1");
    testing.assertEqual(staleExchange("POST", {}, 1), "net GET /a HTTP/1.1");
}

# A fresh connection that closes without a reply is not retried: only a reused
# socket can be stale.
func testExchangeFreshCloseNotRetried() {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    def server as task of null init spawn {
        net.close(net.accept($listener));
        serveOnce($listener, "", false);
    };
    # A session whose socket is gone: exchange dials a fresh one, which the
    # server then closes unanswered.
    def s as Session init connect("http://" + $addr, defaultOptions());
    $s.open = false;
    net.close($s.conn);
    def got as string init "no error";
    try {
        exchange($s, "GET", "/", {}, "");
    } catch (e) {
        $got = $e.kind;
    }
    task.wait($server);
    net.close($listener);
    testing.assertEqual($got, "net");
}
