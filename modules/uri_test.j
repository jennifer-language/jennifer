# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# White-box tests for modules/uri.j. Run: jennifer test modules/uri_test.j
#
# The overlay splices uri.j in front of this file, so the tests reach both its
# exported surface and its private helpers (removeDotSegments, mergePath,
# lastIndexOf, indexFrom) by bare identifier.

use testing;

# --- percent encode / decode ---

func testEncodeUnreservedAndReserved() {
    testing.assertEqual(encode("aZ9-._~"), "aZ9-._~");   # unreserved stays literal
    testing.assertEqual(encode("a b&c=1/?#"), "a%20b%26c%3D1%2F%3F%23");
    testing.assertEqual(encode("café"), "caf%C3%A9");    # multi-byte UTF-8
}

func testDecodeRoundTrip() {
    testing.assertEqual(decode("caf%C3%A9"), "café");
    testing.assertEqual(decode("a%20b"), "a b");
    testing.assertEqual(decode("a+b"), "a+b");           # "+" is literal here (RFC 3986)
}

func badPercent() {
    return decode("bad%zz");
}
func testDecodeMalformedThrows() {
    testing.assertThrows("badPercent", "runtime");
}

# --- form encode / decode (application/x-www-form-urlencoded) ---

func testFormEncodeSpaceAsPlus() {
    testing.assertEqual(encodeForm("a b"), "a+b");        # space -> "+"
    testing.assertEqual(encodeForm("a+b"), "a%2Bb");      # literal "+" escapes
    testing.assertEqual(encodeForm("x&y=1"), "x%26y%3D1");
}

func testFormDecode() {
    testing.assertEqual(decodeForm("a+b"), "a b");        # "+" -> space
    testing.assertEqual(decodeForm("a%2Bb"), "a+b");      # "%2B" -> literal "+"
    testing.assertEqual(decodeForm("caf%C3%A9"), "café");
}

# --- parse / build ---

func testParseFullUrl() {
    def u as Uri init parse("https://user@example.com:8443/a/b?x=1&y=2#top");
    testing.assertEqual($u.scheme, "https");
    testing.assertEqual($u.user, "user");
    testing.assertEqual($u.host, "example.com");
    testing.assertEqual($u.port, "8443");
    testing.assertEqual($u.path, "/a/b");
    testing.assertEqual($u.query, "x=1&y=2");
    testing.assertEqual($u.fragment, "top");
    testing.assertEqual(build($u), "https://user@example.com:8443/a/b?x=1&y=2#top");
}

func testParseNoAuthorityAndNoPort() {
    def m as Uri init parse("mailto:alice@example.com");    # scheme, no "//"
    testing.assertEqual($m.scheme, "mailto");
    testing.assertEqual($m.host, "");
    testing.assertEqual($m.path, "alice@example.com");

    def h as Uri init parse("http://host/only/path");       # no port, no user
    testing.assertEqual($h.host, "host");
    testing.assertEqual($h.port, "");
    testing.assertEqual($h.path, "/only/path");
}

func testParseRelativeAndIpv6() {
    def r as Uri init parse("/just/a/path?q=1");            # no scheme, no authority
    testing.assertEqual($r.scheme, "");
    testing.assertEqual($r.host, "");
    testing.assertEqual($r.path, "/just/a/path");
    testing.assertEqual($r.query, "q=1");

    def v6 as Uri init parse("http://[::1]:9000/x");        # IPv6 literal host
    testing.assertEqual($v6.host, "[::1]");
    testing.assertEqual($v6.port, "9000");

    def bare as Uri init parse("http://example.com");       # authority, empty path
    testing.assertEqual($bare.host, "example.com");
    testing.assertEqual($bare.path, "");
}

func testBuildPartial() {
    def only as Uri init Uri{scheme: "", user: "", host: "", port: "", path: "/rel", query: "", fragment: ""};
    testing.assertEqual(build($only), "/rel");
}

# --- query strings ---

func testBuildQueryEncodesAndOrders() {
    def m as map of string to string init {};
    $m["name"] = "a b";
    $m["tag"] = "x&y";
    testing.assertEqual(buildQuery($m), "name=a+b&tag=x%26y");   # form: space -> "+", insertion order
    def empty as map of string to string init {};
    testing.assertEqual(buildQuery($empty), "");
}

func testParseQuery() {
    def m as map of string to string init parseQuery("a=1&b=hello+world&c=%20x&flag");
    testing.assertEqual($m["a"], "1");
    testing.assertEqual($m["b"], "hello world");    # "+" -> space
    testing.assertEqual($m["c"], " x");             # "%20" -> space
    testing.assertEqual($m["flag"], "");            # bare key -> ""
    def none as map of string to string init parseQuery("");
    testing.assertEqual(len($none), 0);
    def dbl as map of string to string init parseQuery("a=1&&b=2");   # empty pair skipped
    testing.assertEqual(len($dbl), 2);
}

func testQueryRoundTrip() {
    def m as map of string to string init {};
    $m["q"] = "a b/c&d";
    $m["n"] = "42";
    def back as map of string to string init parseQuery(buildQuery($m));
    testing.assertEqual($back["q"], "a b/c&d");
    testing.assertEqual($back["n"], "42");
}

# --- reference resolution (RFC 3986 section 5) ---

func testResolveRelativePaths() {
    def base as string init "http://h/a/b/page.html";
    testing.assertEqual(resolve($base, "sibling"), "http://h/a/b/sibling");
    testing.assertEqual(resolve($base, "../img.png"), "http://h/a/img.png");
    testing.assertEqual(resolve($base, "../../top"), "http://h/top");
    testing.assertEqual(resolve($base, "./here"), "http://h/a/b/here");
    testing.assertEqual(resolve($base, "/root/x"), "http://h/root/x");
}

func testResolveEdgeCases() {
    def base as string init "http://h/a/b?oldq#oldf";
    # An empty reference path keeps the base path; an empty query falls back to base query.
    testing.assertEqual(resolve($base, "#newf"), "http://h/a/b?oldq#newf");
    testing.assertEqual(resolve($base, "?newq"), "http://h/a/b?newq");
    # An absolute reference (own scheme) is returned as-is, dot-normalised.
    testing.assertEqual(resolve($base, "https://other/x/../y"), "https://other/y");
    # A network-path reference (starts with "//") swaps the authority.
    testing.assertEqual(resolve($base, "//other/z"), "http://other/z");
}

func testDotSegmentHelpers() {
    testing.assertEqual(removeDotSegments("/a/b/c/./../../g"), "/a/g");
    testing.assertEqual(removeDotSegments("mid/content=5/../6"), "mid/6");
    testing.assertEqual(removeDotSegments("/../a"), "/a");        # ".." at root is dropped
    testing.assertEqual(removeDotSegments("a/./b"), "a/b");
    # Leading and lone dot-segment forms.
    testing.assertEqual(removeDotSegments("../a"), "a");
    testing.assertEqual(removeDotSegments("./a"), "a");
    testing.assertEqual(removeDotSegments("/."), "/");
    testing.assertEqual(removeDotSegments("/.."), "/");
    testing.assertEqual(removeDotSegments("."), "");
    testing.assertEqual(removeDotSegments(".."), "");
    testing.assertEqual(lastIndexOf("/a/b/c", "/"), 4);
    testing.assertEqual(lastIndexOf("none", "/"), -1);
    testing.assertEqual(indexFrom("a/b/c", "/", 2), 3);
    # mergePath edges: base with authority + empty path, and a slashless base path.
    testing.assertEqual(resolve("http://h", "rel"), "http://h/rel");
    def noSlash as Uri init Uri{scheme: "", user: "", host: "", port: "", path: "noslash", query: "", fragment: ""};
    testing.assertEqual(mergePath($noSlash, "x"), "x");
}
