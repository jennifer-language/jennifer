# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# web_test.j - white-box tests for web.j. Run with:
#
#     jennifer test modules/web_test.j
#
# The overlay splices web.j in first, so these tests reach its private helpers
# (splitPath, matchRoute) and the private Match struct by bare identifier, as
# well as the exported registration surface. The serving loop itself is covered
# by the Go integration test (cmd/jennifer/web_test.go) and the demo, since it
# needs a live listener. web.j already `use`s httpd / json / lists /
# maps / strings / convert / uuid, so the overlay only adds testing.
use testing;

# Dummy handler func values to register as routes / middleware.
func hHome(ctx as Context) {
    return;
}
func hUser(ctx as Context) {
    return;
}

func testSplitPath() {
    testing.assertEqual(len(splitPath("/a/b/")), 2);
    testing.assertEqual(splitPath("/a/b")[0], "a");
    testing.assertEqual(splitPath("/a/b")[1], "b");
    testing.assertEqual(len(splitPath("/")), 0);
    testing.assertEqual(len(splitPath("")), 0);
}

func testMatchExact() {
    def app as App init new();
    $app = get($app, "/home", hHome);
    def m as Match init matchRoute($app, "GET", "/home");
    testing.assertTrue($m.found);
}

func testMatchParam() {
    def app as App init new();
    $app = get($app, "/users/:id", hUser);
    def m as Match init matchRoute($app, "GET", "/users/42");
    testing.assertTrue($m.found);
    testing.assertEqual($m.params["id"], "42");
}

func testMultiParam() {
    def app as App init new();
    $app = get($app, "/users/:uid/posts/:pid", hUser);
    def m as Match init matchRoute($app, "GET", "/users/7/posts/99");
    testing.assertEqual($m.params["uid"], "7");
    testing.assertEqual($m.params["pid"], "99");
}

func testMatchWildcard() {
    def app as App init new();
    $app = get($app, "/files/*path", hHome);
    def m as Match init matchRoute($app, "GET", "/files/a/b/c");
    testing.assertTrue($m.found);
    testing.assertEqual($m.params["path"], "a/b/c");
}

func testMatchWildcardPrefixEmpty() {
    def app as App init new();
    $app = get($app, "/files/*path", hHome);
    def m as Match init matchRoute($app, "GET", "/files");
    testing.assertTrue($m.found);
    testing.assertEqual($m.params["path"], "");
}

func testMatchSpaFallback() {
    def app as App init new();
    $app = get($app, "/*path", hHome);
    testing.assertEqual(
        matchRoute($app, "GET", "/deep/nested/page").params["path"],
        "deep/nested/page");
    testing.assertEqual(matchRoute($app, "GET", "/").params["path"], "");
}

func testWildcardPrecedence() {
    def app as App init new();
    $app = get($app, "/files/:id", hHome);
    $app = get($app, "/files/*path", hHome);
    # a single segment matches the specific :id route (registered first)
    def one as Match init matchRoute($app, "GET", "/files/report");
    testing.assertTrue(maps.has($one.params, "id"));
    testing.assertEqual($one.params["id"], "report");
    # a multi-segment path falls through to the wildcard
    def many as Match init matchRoute($app, "GET", "/files/a/b");
    testing.assertTrue(maps.has($many.params, "path"));
    testing.assertEqual($many.params["path"], "a/b");
}

# Routing bounds the path before splitting it: runs of "/" collapse, and a
# path past MAX_PATH_SEGMENTS segments matches nothing, wildcard included.
func testMatchBoundsTheSegments() {
    def app as App init new();
    $app = get($app, "/a/:id", hHome);
    $app = get($app, "/*path", hHome);
    def m as Match init matchRoute($app, "GET", "//a///7//");
    testing.assertEqual($m.params["id"], "7");
    testing.assertTrue(matchRoute($app, "GET", strings.replicate("/", 65536)).found);
    testing.assertTrue(matchRoute($app, "GET", strings.replicate("/x", MAX_PATH_SEGMENTS)).found);
    testing.assertFalse(
        matchRoute($app, "GET", strings.replicate("/x", MAX_PATH_SEGMENTS + 1)).found);
    testing.assertFalse(matchRoute($app, "GET", strings.replicate("/x", 32768)).found);
}

# A route's pattern is split once when it is registered or mounted.
func testRoutesKeepTheirSegments() {
    def sub as App init get(new(), "/deck/:name", hHome);
    def app as App init mount(new(), "/v1/", $sub);
    testing.assertEqual($sub.routes[0].segs, ["deck", ":name"]);
    testing.assertEqual($app.routes[0].segs, ["v1", "deck", ":name"]);
    testing.assertEqual(matchRoute($app, "GET", "/v1/deck/x").params["name"], "x");
    # a Route without segs is still matched from its pattern
    def bare as App init new();
    def r as Route;
    $r.method = "GET";
    $r.pattern = "/a/:id";
    $r.handler = hHome;
    $bare.routes[] = $r;
    testing.assertEqual(matchRoute($bare, "GET", "/a/3").params["id"], "3");
}

func testNoMatchPath() {
    def app as App init new();
    $app = get($app, "/home", hHome);
    def m as Match init matchRoute($app, "GET", "/nope");
    testing.assertFalse($m.found);
}

func testMethodMismatch() {
    def app as App init new();
    $app = get($app, "/home", hHome);
    def m as Match init matchRoute($app, "POST", "/home");
    testing.assertFalse($m.found);
}

func testRegistrationIsImmutable() {
    def app as App init new();
    def grown as App init get($app, "/home", hHome);
    testing.assertEqual(len($app.routes), 0);
    testing.assertEqual(len($grown.routes), 1);
}

func testVerbsSetMethod() {
    def app as App init new();
    $app = post($app, "/a", hHome);
    $app = delete($app, "/b", hHome);
    testing.assertEqual($app.routes[0].method, "POST");
    testing.assertEqual($app.routes[1].method, "DELETE");
}

func testBeforeRegisters() {
    def app as App init new();
    $app = before($app, hHome);
    testing.assertEqual(len($app.middleware), 1);
}

func testNotFoundSets() {
    def app as App init new();
    testing.assertFalse($app.hasNotFound);
    $app = notFound($app, hHome);
    testing.assertTrue($app.hasNotFound);
}

# An undefined handler is now a parse-time error at the call site (a bare name
# that is not a top-level function), so there is no runtime web.route rejection to
# test - the resolver's own tests cover it.

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
    testing.assertEqual(
        formatSetCookie("sid", "abc", $o),
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
    testing.assertFalse(csrfValid($secret, $rand + ".deadbeef")); # bad signature
    testing.assertFalse(csrfValid("othersecret", $good)); # wrong secret
    testing.assertFalse(csrfValid($secret, "nodot")); # malformed
    testing.assertFalse(csrfValid($secret, ""));
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
    def o as CookieOptions init CookieOptions{
        path: "/a\r\nSet-Cookie: evil=1",
        domain: "",
        maxAge: 0,
        httpOnly: false,
        secure: false,
        sameSite: ""
    };
    formatSetCookie("sid", "abc", $o);
}
func injectCookieDomain() {
    def o as CookieOptions init CookieOptions{
        path: "",
        domain: "x\r\nSet-Cookie: evil=1",
        maxAge: 0,
        httpOnly: false,
        secure: false,
        sameSite: ""
    };
    formatSetCookie("sid", "abc", $o);
}
func testCookiePathRejectsCrlf() {
    testing.assertThrows("injectCookiePath", "web");
}
func testCookieDomainRejectsCrlf() {
    testing.assertThrows("injectCookieDomain", "web");
}
func testCleanCookieAccepted() {
    def o as CookieOptions init CookieOptions{
        path: "/",
        domain: "example.com",
        maxAge: 0,
        httpOnly: true,
        secure: true,
        sameSite: "Lax"
    };
    testing.assertContains(formatSetCookie("sid", "abc", $o), "Path=/; Domain=example.com");
}

# ---- SameSite is one of the three values ----

func sameSiteCookie(v as string, secure as bool) {
    def o as CookieOptions;
    $o.sameSite = $v;
    $o.secure = $secure;
    return formatSetCookie("sid", "abc", $o);
}
func sameSiteInjected() {
    return sameSiteCookie("Lax; Domain=evil.example", false);
}
func sameSiteLowercase() {
    return sameSiteCookie("lax", false);
}
func sameSiteNoneInsecure() {
    return sameSiteCookie("None", false);
}
func testSameSiteValidated() {
    testing.assertThrows("sameSiteInjected", "web");
    testing.assertThrows("sameSiteLowercase", "web");
    testing.assertThrows("sameSiteNoneInsecure", "web");
    testing.assertEqual(sameSiteCookie("Strict", false), "sid=abc; SameSite=Strict");
    testing.assertEqual(sameSiteCookie("None", true), "sid=abc; Secure; SameSite=None");
    testing.assertEqual(sameSiteCookie("", false), "sid=abc");
}

# ---- Secure cookie default (session / CSRF hardening) ----

# The CSRF cookie is __Host- prefixed whenever it is Secure (unplantable from a
# sibling subdomain or over plaintext HTTP), plain "csrf" in insecure dev mode.
func testCsrfCookieName() {
    testing.assertEqual(csrfCookieName(), "__Host-csrf");
    os.setEnv("JENNIFER_WEB_INSECURE_COOKIES", "1");
    testing.assertEqual(csrfCookieName(), "csrf");
    os.setEnv("JENNIFER_WEB_INSECURE_COOKIES", "");
}

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

# --- additions -------------------------------------------------------------

func hErr(err as Error) {
    return;
}

# OM-019: a HEAD request matches a route registered with GET.
func testHeadFallsBackToGet() {
    def app as App init new();
    $app = get($app, "/home", hHome);
    def m as Match init matchRoute($app, "HEAD", "/home");
    testing.assertTrue($m.found);
    # A HEAD with no GET route still misses.
    testing.assertFalse(matchRoute($app, "HEAD", "/nope").found);
    # An explicit HEAD route still wins for HEAD (registered directly).
    def app2 as App init route(new(), "HEAD", "/ping", hHome);
    testing.assertTrue(matchRoute($app2, "HEAD", "/ping").found);
}

# OM-009: web.onError sets the error handler.
func testOnErrorRegisters() {
    def app as App init onError(new(), hErr);
    testing.assertTrue($app.hasOnError);
    testing.assertFalse(new().hasOnError);
}

# hostOnly drops the port from both address forms and leaves a bare host alone.
func testHostOnly() {
    testing.assertEqual(hostOnly("192.0.2.7:53412"), "192.0.2.7");
    testing.assertEqual(hostOnly("[2001:db8::1]:53412"), "2001:db8::1");
    testing.assertEqual(hostOnly("[::1]:8080"), "::1");
    testing.assertEqual(hostOnly("192.0.2.7"), "192.0.2.7");
    testing.assertEqual(hostOnly("2001:db8::1"), "2001:db8::1");
    testing.assertEqual(hostOnly("example.com:443"), "example.com");
    testing.assertEqual(hostOnly(""), "");
}

func emptyCsrfSecret() {
    return csrfValid("", "r." + "00");
}
# An empty CSRF secret is a configuration error, not a key anyone can sign with.
func testCsrfEmptySecretRefused() {
    testing.assertThrows("emptyCsrfSecret", "web");
}

func formTooManyFields() {
    parseForm(strings.replicate("a=1&", 10001));
}
# M29.2: parseForm runs in Go (encoding.parseForm) with a field cap. It is
# strict: a malformed escape or a value that is not UTF-8 throws "encoding".
func formMalformed() {
    parseForm("k=100%");
}

func formNotUtf8() {
    parseForm("k=%FF");
}

func testParseFormGo() {
    testing.assertEqual(parseForm(strings.replicate("a=1&", 10000))["a"], "1");
    testing.assertThrows("formTooManyFields", "limit");
    testing.assertEqual(len(parseForm(strings.replicate("&", 100000))), 0);
    testing.assertEqual(parseForm("k=a%2Bb+c")["k"], "a+b c");
    testing.assertEqual(parseForm("k=100%25")["k"], "100%");
    testing.assertThrows("formMalformed", "encoding");
    testing.assertThrows("formNotUtf8", "encoding");
    def f as map of string to string init parseForm("b=1&a=2&b=3");
    testing.assertEqual(maps.keys($f), ["b", "a"]);
    testing.assertEqual($f["b"], "3");
    testing.assertEqual(parseCookie(strings.replicate(";", 65536) + "sid=x", "sid"), "x");
}
