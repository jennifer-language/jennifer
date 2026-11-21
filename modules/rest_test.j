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
use encoding;
use testing;
use net;
use task;
use time;

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
    def open as Client init withSkipVerify($cli);
    testing.assertEqual($open.options.tls.skipVerify, true);
    testing.assertEqual($cli.options.tls.skipVerify, false); # original unchanged (value semantics)
}

func testWithCA() {
    def cli as Client init client("https://appliance");
    def pem as bytes init encoding.encode("utf-8", "-----BEGIN CERTIFICATE-----");
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

# Origins compare scheme, host and port, with the default port written out.
func testOriginOf() {
    testing.assertEqual(originOf("https://API.example.com/v1"), "https://api.example.com:443");
    testing.assertEqual(originOf("HTTP://h:8080/x?y"), "http://h:8080");
    testing.assertEqual(originOf("http://h/x"), originOf("http://h:80/y"));
}

# The Link header is parsed whole: commas inside <...> or quotes do not split
# it, rel is a space-separated, case-insensitive list, only the first rel of a
# link counts, and empty list elements are skipped.
func testParseNextLinkGrammar() {
    testing.assertEqual(
        parseNextLink('<https://api/items?ids=1,2&page=2>; rel="next"'),
        "https://api/items?ids=1,2&page=2");
    testing.assertEqual(parseNextLink('</b2>; rel="prev next"'), "/b2");
    testing.assertEqual(parseNextLink('</b2>;REL=Next'), "/b2");
    testing.assertEqual(parseNextLink('<a>; title="x, rel=next", <b>; rel="next"'), "b");
    testing.assertEqual(parseNextLink('<a>; rel="prev"; rel="next"'), "");
    testing.assertEqual(parseNextLink(' , <a>; rel=last ,, <b> ; rel = "next" '), "b");
    testing.assertThrows("linkNoBrackets", "value");
    testing.assertThrows("linkTrailingJunk", "value");
    testing.assertThrows("linkUnclosedQuote", "value");
}

func linkNoBrackets() {
    parseNextLink('https://api/x; rel="next"');
}

func linkTrailingJunk() {
    parseNextLink('<a>; rel="next" junk');
}

func linkUnclosedQuote() {
    parseNextLink('<a>; rel="next');
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

# ---- paginate over a live server ----

# servePages answers up to `n` connections, until the listener closes; the reply to a request path carries the
# Link header `links` maps it to (none when absent). Returns the paths seen.
func servePages(listener as net.Listener, links as map of string to string, n as int) {
    def seen as list of string init [];
    for (def i in 0..$n) {
        def c as net.Conn;
        try {
            $c = net.accept($listener);
        } catch (e) {
            # walk closed the listener: the client asked for no more pages.
            if ($e.kind != "closed") {
                throw $e;
            }
            return $seen;
        }
        def req as string init encoding.decode("utf-8", net.readBytes($c, 4096));
        def path as string init strings.split(strings.split($req, "\r\n")[0], " ")[1];
        $seen[] = $path;
        def extra as string init "";
        if (maps.has($links, $path)) {
            $extra = "Link: " + $links[$path] + "\r\n";
        }
        net.writeBytes(
            $c,
            encoding.encode(
                "utf-8",
                "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n" + $extra +
                    "Content-Length: 2\r\n\r\n" + '{}'));
        time.sleep(time.fromMilliseconds(30));
        net.close($c);
    }
    return $seen;
}

# walk paginates from `start` against a server answering `n` requests, and
# returns the paths it saw, or the error kind and status.
func walk(links as map of string to string, start as string, n as int, host as string) {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def port as string init strings.split(net.address($listener), ":")[1];
    def server as task of list of string init spawn {
        return servePages($listener, $links, $n);
    };
    def out as string init "";
    try {
        def pages as list of json.Value init paginate(
            client("http://" + $host + ":" + $port),
            $start,
            {},
            10);
        $out = "{len($pages)} pages";
    } catch (e) {
        if ($e.kind != "rest") {
            throw $e;
        }
        $out = "rest {$e.status}";
    }
    net.close($listener);
    def seen as list of string init task.wait($server);
    return $out + " " + strings.join($seen, " ");
}

# A comma inside the target, a multi-token rel and a relative target resolved
# against the page URL are all followed.
func testPaginateFollowsLinkGrammar() {
    testing.assertEqual(
        walk({"/items": '</items?ids=1,2&page=2>; rel="next"'}, "/items", 2, "127.0.0.1"),
        "2 pages /items /items?ids=1,2&page=2");
    testing.assertEqual(
        walk({"/a/b": '<c>; rel="prev next"'}, "/a/b", 2, "127.0.0.1"),
        "2 pages /a/b /a/c");
}

# A next link to another origin is an error carrying the page's status, not a
# quiet end of the walk.
func testPaginateCrossOriginNextThrows() {
    testing.assertEqual(
        walk({"/c": "<http://localhost:1/c2>; rel=next"}, "/c", 1, "127.0.0.1"),
        "rest 200 /c");
    testing.assertEqual(
        walk({"/d": "<https://127.0.0.1/d2>; rel=next"}, "/d", 1, "127.0.0.1"),
        "rest 200 /d");
    testing.assertEqual(walk({"/e": "<e2>; rel=next junk"}, "/e", 1, "127.0.0.1"), "rest 200 /e");
}
