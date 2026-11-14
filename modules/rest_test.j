# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# rest_test.j - white-box tests for rest.j's pure helpers. Run with:
#
#     jennifer test modules/rest_test.j
#
# The overlay splices rest.j in front of this file, so the tests reach its URL
# joining / query building / encoding / auth helpers by bare identifier. The
# networked CRUD path (get / post / put / patch / delete over http) is verified
# against an in-process REST server in the Go suite (TestRestCrud).
use testing;

func testJoinUrl() {
    testing.assertEqual(joinUrl("http://api", "/users"), "http://api/users");
    testing.assertEqual(joinUrl("http://api/", "/users"), "http://api/users");
    testing.assertEqual(joinUrl("http://api", "users"), "http://api/users");
    testing.assertEqual(joinUrl("http://api/", "users"), "http://api/users");
    testing.assertEqual(joinUrl("http://api/v/x", "/y"), "http://api/v/x/y");
}

func testQueryStringEmpty() {
    testing.assertEqual(queryString({}), "");
}

func testQueryStringEncodes() {
    testing.assertEqual(queryString({"q": "a b"}), "?q=a+b");
    testing.assertEqual(queryString({"x": "a&b=c"}), "?x=a%26b%3Dc");
}

func testQueryStringMulti() {
    # insertion order is preserved
    testing.assertEqual(queryString({"a": "1", "b": "2"}), "?a=1&b=2");
}

# Percent-encoding now lives in the `url` module (tested in url_test.j); rest's
# query building is covered by the testQueryString* cases below.

func testBearer() {
    testing.assertEqual(bearer("my-token"), "Bearer my-token");
}

func testBasic() {
    # base64("user:pass") == "dXNlcjpwYXNz"
    testing.assertEqual(basic("user", "pass"), "Basic dXNlcjpwYXNz");
}

func testWithHeader() {
    def cli as Client init client("http://api");
    def authed as Client init withHeader($cli, "Authorization", bearer("x"));
    testing.assertEqual($authed.headers["Authorization"], "Bearer x");
    testing.assertEqual(len($cli.headers), 0); # original unchanged (value semantics)
}

func testClientDefaults() {
    def cli as Client init client("http://api");
    testing.assertEqual($cli.baseUrl, "http://api");
    testing.assertEqual(len($cli.headers), 0);
    testing.assertEqual($cli.options.tls.skipVerify, false); # verify on by default
    testing.assertEqual(len($cli.options.tls.caCert), 0);
    testing.assertEqual($cli.options.maxRedirects, 0); # no redirects by default
    testing.assertEqual($cli.options.maxRetries, 0); # no retries by default
    testing.assertEqual($cli.options.timeoutMs, 0); # default timeout
}

func testInsecure() {
    def cli as Client init client("https://appliance");
    def open as Client init insecure($cli);
    testing.assertEqual($open.options.tls.skipVerify, true);
    testing.assertEqual($cli.options.tls.skipVerify, false); # original unchanged (value semantics)
}

func testWithCA() {
    def cli as Client init client("https://appliance");
    def pem as bytes init convert.bytesFromString("utf-8", "-----BEGIN CERTIFICATE-----");
    def pinned as Client init withCA($cli, $pem);
    testing.assertEqual(len($pinned.options.tls.caCert), len($pem));
    testing.assertEqual($pinned.options.tls.skipVerify, false); # withCA still authenticates
    testing.assertEqual(len($cli.options.tls.caCert), 0); # original unchanged
}

func testPolicyBuilders() {
    def cli as Client init client("http://api");
    def tuned as Client init withRetries(withRedirects(withTimeout($cli, 5000), 3), 2);
    testing.assertEqual($tuned.options.timeoutMs, 5000);
    testing.assertEqual($tuned.options.maxRedirects, 3);
    testing.assertEqual($tuned.options.maxRetries, 2);
    # original unchanged (value semantics)
    testing.assertEqual($cli.options.timeoutMs, 0);
    testing.assertEqual($cli.options.maxRedirects, 0);
}

func testJoinUrlAbsolutePassthrough() {
    # a Link-header "next" is an absolute URL: it must not be re-prefixed
    testing.assertEqual(
        joinUrl("http://api/v1", "https://api.example.com/items?page=2"),
        "https://api.example.com/items?page=2");
    testing.assertEqual(joinUrl("http://api", "http://other/x"), "http://other/x");
}

func testParseNextLink() {
    def link as string init "<https://api/items?page=2>; rel=\"next\", <https://api/items?page=9>; rel=\"last\"";
    testing.assertEqual(parseNextLink($link), "https://api/items?page=2");
    # no next relation -> ""
    testing.assertEqual(parseNextLink("<https://api/items?page=1>; rel=\"prev\""), "");
    testing.assertEqual(parseNextLink(""), "");
}

# A relative or same-origin next link is followable; a different-origin absolute
# URL is not (so paginate won't replay the client's credentials cross-origin).
func testSameOrigin() {
    testing.assertTrue(sameOrigin("https://api.example.com/v1", "/items?page=2"));
    testing.assertTrue(sameOrigin(
        "https://api.example.com/v1",
        "https://api.example.com/items?p=2"));
    testing.assertFalse(sameOrigin("https://api.example.com/v1", "https://evil.example.net/items"));
    testing.assertFalse(sameOrigin("https://api.example.com/v1", "http://api.example.com/items"));
}

# A non-2xx where a JSON body was needed throws an HttpError: kind "rest" with
# the status, headers and body as fields, the message as before.
func testANon2xxIsAnHttpError() {
    def r as Response init Response{
        status: 429,
        headers: {"retry-after": "30"},
        body: '{"message":"slow down"}'
    };
    try {
        decodeOk("rest.getJson", $r);
        throw Error{kind: "missed", message: "a 429 decoded"};
    } catch (e) {
        testing.assertEqual($e.kind, "rest");
        testing.assertEqual($e.status, 429);
        testing.assertEqual($e.headers["retry-after"], "30");
        testing.assertEqual($e.body, '{"message":"slow down"}');
        testing.assertEqual($e.message, 'rest.getJson: HTTP 429: {"message":"slow down"}');
    }
    $r.status = 200;
    testing.assertEqual(json.asString(decodeOk("rest.getJson", $r), "/message"), "slow down");
}
