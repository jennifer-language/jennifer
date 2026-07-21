# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# web_test.j - white-box tests for web.j. Run with:
#
#     jennifer test modules/web_test.j
#
# The overlay splices web.j in first, so these tests reach its private helpers
# (splitPath, matchRoute) and the private Match struct by bare identifier, as
# well as the exported registration surface. The serving loop itself is covered
# by the Go integration test (cmd/jennifer/web_test.go) and the demo, since it
# needs a live listener. web.j already `use`s httpd / meta / json / lists /
# maps / strings / convert / uuid, so the overlay only adds testing.
use testing;

# Dummy handlers so route registration's meta.defined check passes.
func hHome(ctx as Context) { return; }
func hUser(ctx as Context) { return; }

func testSplitPath() {
    testing.assertEqual(len(splitPath("/a/b/")), 2);
    testing.assertEqual(splitPath("/a/b")[0], "a");
    testing.assertEqual(splitPath("/a/b")[1], "b");
    testing.assertEqual(len(splitPath("/")), 0);
    testing.assertEqual(len(splitPath("")), 0);
}

func testMatchExact() {
    def app as App init new();
    $app = get($app, "/home", "hHome");
    def m as Match init matchRoute($app, "GET", "/home");
    testing.assertTrue($m.found);
    testing.assertEqual($m.handler, "hHome");
}

func testMatchParam() {
    def app as App init new();
    $app = get($app, "/users/:id", "hUser");
    def m as Match init matchRoute($app, "GET", "/users/42");
    testing.assertTrue($m.found);
    testing.assertEqual($m.params["id"], "42");
}

func testMultiParam() {
    def app as App init new();
    $app = get($app, "/users/:uid/posts/:pid", "hUser");
    def m as Match init matchRoute($app, "GET", "/users/7/posts/99");
    testing.assertEqual($m.params["uid"], "7");
    testing.assertEqual($m.params["pid"], "99");
}

func testMatchWildcard() {
    def app as App init new();
    $app = get($app, "/files/*path", "hHome");
    def m as Match init matchRoute($app, "GET", "/files/a/b/c");
    testing.assertTrue($m.found);
    testing.assertEqual($m.params["path"], "a/b/c");
}

func testMatchWildcardPrefixEmpty() {
    def app as App init new();
    $app = get($app, "/files/*path", "hHome");
    def m as Match init matchRoute($app, "GET", "/files");
    testing.assertTrue($m.found);
    testing.assertEqual($m.params["path"], "");
}

func testMatchSpaFallback() {
    def app as App init new();
    $app = get($app, "/*path", "hHome");
    testing.assertEqual(matchRoute($app, "GET", "/deep/nested/page").params["path"], "deep/nested/page");
    testing.assertEqual(matchRoute($app, "GET", "/").params["path"], "");
}

func testWildcardPrecedence() {
    def app as App init new();
    $app = get($app, "/files/:id", "hHome");
    $app = get($app, "/files/*path", "hHome");
    # a single segment matches the specific :id route (registered first)
    def one as Match init matchRoute($app, "GET", "/files/report");
    testing.assertTrue(maps.has($one.params, "id"));
    testing.assertEqual($one.params["id"], "report");
    # a multi-segment path falls through to the wildcard
    def many as Match init matchRoute($app, "GET", "/files/a/b");
    testing.assertTrue(maps.has($many.params, "path"));
    testing.assertEqual($many.params["path"], "a/b");
}

func testNoMatchPath() {
    def app as App init new();
    $app = get($app, "/home", "hHome");
    def m as Match init matchRoute($app, "GET", "/nope");
    testing.assertFalse($m.found);
}

func testMethodMismatch() {
    def app as App init new();
    $app = get($app, "/home", "hHome");
    def m as Match init matchRoute($app, "POST", "/home");
    testing.assertFalse($m.found);
}

func testRegistrationIsImmutable() {
    def app as App init new();
    def grown as App init get($app, "/home", "hHome");
    testing.assertEqual(len($app.routes), 0);
    testing.assertEqual(len($grown.routes), 1);
}

func testVerbsSetMethod() {
    def app as App init new();
    $app = post($app, "/a", "hHome");
    $app = delete($app, "/b", "hHome");
    testing.assertEqual($app.routes[0].method, "POST");
    testing.assertEqual($app.routes[1].method, "DELETE");
}

func testBeforeRegisters() {
    def app as App init new();
    $app = before($app, "hHome");
    testing.assertEqual(len($app.middleware), 1);
}

func testNotFoundSets() {
    def app as App init new();
    $app = notFound($app, "hHome");
    testing.assertEqual($app.notFound, "hHome");
}

# badRoute registers an undefined handler, which web.route must reject.
func badRoute() {
    def app as App init new();
    def bad as App init route($app, "GET", "/x", "definitelyNotDefined");
}

func testRouteValidatesHandler() {
    testing.assertThrows("badRoute", "web");
}

func testParseCookie() {
    testing.assertEqual(parseCookie("sid=abc; theme=dark", "theme"), "dark");
    testing.assertEqual(parseCookie("sid=abc; theme=dark", "sid"), "abc");
    testing.assertEqual(parseCookie("sid=abc", "sid"), "abc");
    testing.assertEqual(parseCookie("", "sid"), "");
    testing.assertEqual(parseCookie("a=1; b=2", "c"), "");
}

func testFormatSetCookieMinimal() {
    def o as CookieOptions;
    testing.assertEqual(formatSetCookie("sid", "abc", $o), "sid=abc");
}

func testFormatSetCookieFull() {
    def o as CookieOptions;
    $o.path = "/";
    $o.maxAge = 3600;
    $o.httpOnly = true;
    $o.sameSite = "Lax";
    testing.assertEqual(formatSetCookie("sid", "abc", $o),
        "sid=abc; Path=/; Max-Age=3600; HttpOnly; SameSite=Lax");
}

func testFormatSetCookieExpire() {
    def o as CookieOptions;
    $o.maxAge = -1;
    $o.secure = true;
    testing.assertEqual(formatSetCookie("sid", "", $o), "sid=; Max-Age=-1; Secure");
}

func testAcceptsGzip() {
    testing.assertTrue(acceptsGzip("gzip, deflate, br"));
    testing.assertTrue(acceptsGzip("gzip"));
    testing.assertFalse(acceptsGzip("deflate, br"));
    testing.assertFalse(acceptsGzip(""));
}

func testCsrfValid() {
    def secret as string init "topsecret";
    def rand as string init "sometoken";
    def good as string init $rand + "." + csrfSign($secret, $rand);
    testing.assertTrue(csrfValid($secret, $good));
    testing.assertFalse(csrfValid($secret, $rand + ".deadbeef"));   # bad signature
    testing.assertFalse(csrfValid("othersecret", $good));           # wrong secret
    testing.assertFalse(csrfValid($secret, "nodot"));               # malformed
    testing.assertFalse(csrfValid($secret, ""));
}

func testPercentDecode() {
    testing.assertEqual(percentDecode("hello"), "hello");
    testing.assertEqual(percentDecode("a+b"), "a b");
    testing.assertEqual(percentDecode("a%20b"), "a b");
    testing.assertEqual(percentDecode("%C3%A9"), "é");
    # a stray % with no two hex digits is left as-is
    testing.assertEqual(percentDecode("100%"), "100%");
}

func testParseForm() {
    def f as map of string to string init parseForm("name=Jane+Doe&city=New%20York&flag=");
    testing.assertEqual($f["name"], "Jane Doe");
    testing.assertEqual($f["city"], "New York");
    testing.assertEqual($f["flag"], "");
    testing.assertEqual(len(parseForm("")), 0);
}

func testParseBasicAuth() {
    # base64("user:pass") = "dXNlcjpwYXNz"
    def ok as BasicCredentials init parseBasicAuth("Basic dXNlcjpwYXNz");
    testing.assertTrue($ok.present);
    testing.assertEqual($ok.user, "user");
    testing.assertEqual($ok.password, "pass");
    # scheme is case-insensitive
    testing.assertTrue(parseBasicAuth("basic dXNlcjpwYXNz").present);
    # wrong scheme / absent / malformed
    testing.assertFalse(parseBasicAuth("Bearer dXNlcjpwYXNz").present);
    testing.assertFalse(parseBasicAuth("").present);
    testing.assertFalse(parseBasicAuth("Basic !!!not-base64!!!").present);
}

func testParseBearer() {
    testing.assertEqual(parseBearer("Bearer abc.def.ghi"), "abc.def.ghi");
    testing.assertEqual(parseBearer("bearer tok"), "tok");
    testing.assertEqual(parseBearer("Basic dXNlcjpwYXNz"), "");
    testing.assertEqual(parseBearer(""), "");
}

func testEtagMatches() {
    testing.assertTrue(etagMatches("\"v1\"", "\"v1\"", "v1"));
    testing.assertTrue(etagMatches("v1", "\"v1\"", "v1"));
    testing.assertTrue(etagMatches("*", "\"v1\"", "v1"));
    testing.assertFalse(etagMatches("\"v2\"", "\"v1\"", "v1"));
    testing.assertFalse(etagMatches("", "\"v1\"", "v1"));
}

func testCorsRegisters() {
    def app as App init new();
    testing.assertFalse(corsEnabled($app));
    def opts as CorsOptions;
    $opts.allowOrigin = "*";
    $opts.allowMethods = "GET, POST";
    def out as App init cors($app, $opts);
    testing.assertTrue(corsEnabled($out));
    testing.assertEqual($out.cors.allowOrigin, "*");
    # Registration is immutable: the original app is unchanged.
    testing.assertFalse(corsEnabled($app));
}


# ---- CRLF injection in cookie Path / Domain ----

func injectCookiePath() {
    def o as CookieOptions init CookieOptions{path: "/a\r\nSet-Cookie: evil=1", domain: "", maxAge: 0, httpOnly: false, secure: false, sameSite: ""};
    formatSetCookie("sid", "abc", $o);
}
func injectCookieDomain() {
    def o as CookieOptions init CookieOptions{path: "", domain: "x\r\nSet-Cookie: evil=1", maxAge: 0, httpOnly: false, secure: false, sameSite: ""};
    formatSetCookie("sid", "abc", $o);
}
func testCookiePathRejectsCrlf() {
    testing.assertThrows("injectCookiePath", "web");
}
func testCookieDomainRejectsCrlf() {
    testing.assertThrows("injectCookieDomain", "web");
}
func testCleanCookieAccepted() {
    def o as CookieOptions init CookieOptions{path: "/", domain: "example.com", maxAge: 0, httpOnly: true, secure: true, sameSite: "Lax"};
    testing.assertContains(formatSetCookie("sid", "abc", $o), "Path=/; Domain=example.com");
}

# ---- Secure cookie default (session / CSRF hardening) ----

func testSecureCookiesDefaultOn() {
    # No opt-out env var set: framework cookies are Secure by default.
    testing.assertTrue(secureCookies());
}

func testSecureCookiesOptOut() {
    os.setEnv("JENNIFER_WEB_INSECURE_COOKIES", "1");
    testing.assertFalse(secureCookies());
    # Restore (empty reads as unset, so Secure is back on).
    os.setEnv("JENNIFER_WEB_INSECURE_COOKIES", "");
    testing.assertTrue(secureCookies());
}
