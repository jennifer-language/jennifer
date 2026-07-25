# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
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
    testing.assertEqual(queryString({"q": "a b"}), "?q=a%20b");
    testing.assertEqual(queryString({"x": "a&b=c"}), "?x=a%26b%3Dc");
}

func testQueryStringMulti() {
    # insertion order is preserved
    testing.assertEqual(queryString({"a": "1", "b": "2"}), "?a=1&b=2");
}

func testUrlEncode() {
    testing.assertEqual(urlEncode("hello world"), "hello%20world");     # space -> %20
    testing.assertEqual(urlEncode("café"), "caf%C3%A9");                 # per-byte UTF-8
    testing.assertEqual(urlEncode("A-Z_a.z~0"), "A-Z_a.z~0");           # unreserved literal
}

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
    testing.assertEqual(len($cli.headers), 0);      # original unchanged (value semantics)
}

func testClientDefaults() {
    def cli as Client init client("http://api");
    testing.assertEqual($cli.baseUrl, "http://api");
    testing.assertEqual(len($cli.headers), 0);
    testing.assertEqual($cli.tls.skipVerify, false);   # verify on by default
    testing.assertEqual(len($cli.tls.caCert), 0);
}

func testInsecure() {
    def cli as Client init client("https://appliance");
    def open as Client init insecure($cli);
    testing.assertEqual($open.tls.skipVerify, true);
    testing.assertEqual($cli.tls.skipVerify, false);   # original unchanged (value semantics)
}

func testWithCA() {
    def cli as Client init client("https://appliance");
    def pem as bytes init convert.bytesFromString("-----BEGIN CERTIFICATE-----", "utf-8");
    def pinned as Client init withCA($cli, $pem);
    testing.assertEqual(len($pinned.tls.caCert), len($pem));
    testing.assertEqual($pinned.tls.skipVerify, false);   # withCA still authenticates
    testing.assertEqual(len($cli.tls.caCert), 0);         # original unchanged
}
