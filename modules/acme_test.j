# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# acme_test.j - white-box tests for acme.j. Run with:
#
#     jennifer test modules/acme_test.j
#
# Covers the pure, network-free surface (challenge selection and the HTTP-01 /
# DNS-01 response computation) and the private helpers (encodeSeg / jsonEsc). The
# networked flow - connect / register / order / finalize - is covered end to end
# against an in-process ACME server by the Go test (cmd/jennifer/acme_test.go).
use testing;
use crypto;
use httpd;
use task;
use convert;
use strings;

# A client with a real account key but no network endpoints (the pure functions
# only touch the key).
func client() {
    def key as bytes init crypto.keypair("p256").private;
    return Client{
        directory: "",
        newNonce: "",
        newAccount: "",
        newOrder: "",
        accountKey: $key,
        alg: "ES256",
        kid: "",
        allowInsecureHttp: false
    };
}

func testSignAlgo() {
    testing.assertEqual(signAlgo("ES256"), "ecdsa-sha256");
    testing.assertEqual(signAlgo("ES384"), "ecdsa-sha384");
    testing.assertEqual(signAlgo("RS256"), "rsa-sha256");
    testing.assertEqual(signAlgo("RS512"), "rsa-sha512");
}

func testEncodeSegStripsPadding() {
    def b as bytes init convert.bytesFromString("utf-8", "hello");
    testing.assertFalse(strings.contains(encodeSeg($b), "="));
    # Round-trip shape: base64url of "hi" is "aGk" (padding removed).
    testing.assertEqual(encodeSeg(convert.bytesFromString("utf-8", "hi")), "aGk");
}

func testJsonEsc() {
    testing.assertEqual(jsonEsc("plain"), "plain");
    testing.assertEqual(jsonEsc("a\"b"), "a\\\"b");
    testing.assertEqual(jsonEsc("a\\b"), "a\\\\b");
}

func testKeyAuthorizationShape() {
    def ka as string init keyAuthorization(client(), "mytoken");
    # token "." thumbprint, thumbprint unpadded base64url.
    testing.assertTrue(strings.startsWith($ka, "mytoken."));
    testing.assertFalse(strings.contains($ka, "="));
    # The thumbprint is SHA-256 -> 32 bytes -> 43 base64url chars.
    testing.assertEqual(len($ka), len("mytoken.") + 43);
}

func testKeyAuthorizationDeterministic() {
    def c as Client init client();
    testing.assertEqual(keyAuthorization($c, "t"), keyAuthorization($c, "t"));
    # Different tokens -> different key authorizations.
    testing.assertFalse(keyAuthorization($c, "a") == keyAuthorization($c, "b"));
}

func testDnsRecordShape() {
    # DNS-01 value is base64url(SHA-256(keyAuth)) -> 43 unpadded chars.
    def rec as string init dnsRecord(client(), "mytoken");
    testing.assertEqual(len($rec), 43);
    testing.assertFalse(strings.contains($rec, "="));
}

func testChallengeSelection() {
    def httpCh as Challenge init Challenge{
        kind: "http-01",
        url: "u1",
        token: "t1",
        status: "pending"
    };
    def dnsCh as Challenge init Challenge{
        kind: "dns-01",
        url: "u2",
        token: "t2",
        status: "pending"
    };
    def chs as list of Challenge init [];
    $chs[] = $httpCh;
    $chs[] = $dnsCh;
    def authz as Authorization init Authorization{
        domain: "example.com",
        status: "pending",
        challenges: $chs
    };
    testing.assertEqual(challenge("http-01", $authz).url, "u1");
    testing.assertEqual(challenge("dns-01", $authz).token, "t2");
}

func testChallengeMissingThrows() {
    testing.assertThrows("selectMissing", "acme");
}
func selectMissing() {
    def chs as list of Challenge init [];
    $chs[] = Challenge{kind: "http-01", url: "u", token: "t", status: "pending"};
    def authz as Authorization init Authorization{domain: "d", status: "pending", challenges: $chs};
    challenge("tls-alpn-01", $authz);
}

func testParseOrderReadsFields() {
    # parseOrder builds an Order from a response body; feed it a canned one.
    def resp as http.Response init http.Response{
        status: 200,
        statusText: "OK",
        headers: {},
        body: '{"status":"pending","authorizations":["https://ca/authz/1","https://ca/authz/2"],"finalize":"https://ca/finalize"}'
    };
    def o as Order init parseOrder("https://ca/order/1", $resp);
    testing.assertEqual($o.url, "https://ca/order/1");
    testing.assertEqual($o.status, "pending");
    testing.assertEqual(len($o.authorizations), 2);
    testing.assertEqual($o.authorizations[1], "https://ca/authz/2");
    testing.assertEqual($o.certificate, ""); # not issued yet
}

# ---- complete JSON escaping + nonce validation ----

func testJsonEscControlChars() {
    testing.assertEqual(jsonEsc("line1\nline2"), "line1\\nline2");
    testing.assertEqual(jsonEsc("t\tab"), "t\\tab");
    def nul as bytes;
    $nul[] = 0;
    testing.assertEqual(jsonEsc(convert.stringFromBytes("utf-8", $nul)), "\\u0000");
    testing.assertEqual(jsonEsc("plain"), "plain");
}
func testIsNonceSafe() {
    testing.assertTrue(isNonceSafe("abcXYZ012-_"));
    testing.assertFalse(isNonceSafe("has space"));
    testing.assertFalse(isNonceSafe("has\rcr"));
    testing.assertFalse(isNonceSafe(""));
}

# A CA's error answer is an AcmeError with its problem document and Retry-After;
# a body that is not one leaves them empty.
func testProblemDocumentsReachTheError() {
    def resp as http.Response init http.Response{
        status: 429,
        statusText: "Too Many Requests",
        headers: {"retry-after": "3600"},
        body: '{"type":"urn:ietf:params:acme:error:rateLimited","detail":"too many certificates","status":429}'
    };
    def e as AcmeError init problemError("acme: request failed", $resp);
    testing.assertEqual($e.kind, "acme");
    testing.assertEqual($e.status, 429);
    testing.assertEqual($e.problemType, "urn:ietf:params:acme:error:rateLimited");
    testing.assertEqual($e.detail, "too many certificates");
    testing.assertEqual($e.retryAfter, 3600);
    testing.assertEqual(json.asInt($e.problem, "/status"), 429);
    $resp.body = "<html>bad gateway</html>";
    $resp.headers = {"retry-after": "Wed, 21 Oct 2026 07:28:00 GMT"};
    $e = problemError("acme: request failed", $resp);
    testing.assertEqual($e.problemType, "");
    testing.assertEqual($e.retryAfter, 0);
    testing.assertTrue(json.isNull($e.problem, ""));
    testing.assertThrows("selectMissing", "acme");
}

# A challenge token must be base64url: one with a `/` or `..` (a path under
# /.well-known/acme-challenge/) is refused, a normal one is kept.
func traversalToken() {
    parseAuthorization(
        '{"identifier":{"value":"a.example"},"status":"pending","challenges":[' +
            '{"type":"http-01","url":"https://ca/c/1","token":"../../etc/x","status":"pending"}]}',
        200);
}
func testChallengeTokenMustBeBase64url() {
    testing.assertThrows("traversalToken", "acme");
    def a as Authorization init parseAuthorization(
        '{"identifier":{"value":"a.example"},"status":"pending","challenges":[' +
            '{"type":"http-01","url":"https://ca/c/1","token":"Ab-_9z","status":"pending"}]}',
        200);
    testing.assertEqual($a.challenges[0].token, "Ab-_9z");
    testing.assertEqual($a.domain, "a.example");
}

# ---- malformed CA answers ----

# kindOf runs a parse of a canned answer and returns the kind it threw ("" when
# it did not throw).
func orderKind(body as string) {
    def resp as http.Response init http.Response{
        status: 200,
        statusText: "OK",
        headers: {},
        body: $body
    };
    try {
        parseOrder("https://ca/order/1", $resp);
    } catch (e) {
        return $e.kind;
    }
    return "";
}

func authzKind(body as string) {
    try {
        parseAuthorization($body, 200);
    } catch (e) {
        return $e.kind;
    }
    return "";
}

func testMalformedOrdersAreAcmeErrors() {
    def ok as string init '{"status":"ready","authorizations":["https://ca/a/1"],' +
        '"finalize":"https://ca/f"}';
    testing.assertEqual(orderKind($ok), "");
    for (def body in [
        "<html>",
        "[]",
        '{"status":"pending"}',
        '{"status":"pending","authorizations":"x","finalize":"https://ca/f"}',
        '{"status":"pending","authorizations":[5],"finalize":"https://ca/f"}',
        '{"status":"pending","authorizations":[],"finalize":7}',
        '{"status":"bogus","authorizations":[],"finalize":"https://ca/f"}',
        '{"status":"valid","authorizations":[],"finalize":"https://ca/f","certificate":1}'
    ]) {
        testing.assertEqual(orderKind($body), "acme");
    }
}

func testMalformedAuthorizationsAreAcmeErrors() {
    def chal as string init '{"type":"http-01","url":"https://ca/c/1","token":"abc",' +
        '"status":"pending"}';
    testing.assertEqual(
        authzKind('{"identifier":{"value":"x"},"status":"pending","challenges":[' + $chal + ']}'),
        "");
    for (def body in [
        "not json",
        '{"identifier":{"value":"x"},"status":"pending"}',
        '{"identifier":{"value":"x"},"status":"pending","challenges":[5]}',
        '{"identifier":{"value":"x"},"status":"pending","challenges":[' +
            '{"type":"http-01","url":5,"token":"abc","status":"pending"}]}',
        '{"identifier":{"value":"x"},"status":"pending","challenges":[' +
            '{"type":"http-01","url":"https://ca/c/1","status":"pending"}]}',
        '{"identifier":{"value":"x"},"status":"pending","challenges":[' +
            '{"type":"http-01","url":"https://ca/c/1","token":"abc","status":"gone"}]}',
        '{"status":"pending","challenges":[' + $chal + ']}',
        '{"identifier":{"value":"x"},"status":"weird","challenges":[' + $chal + ']}'
    ]) {
        testing.assertEqual(authzKind($body), "acme");
    }
}

# ---- https only (RFC 8555 6.1) ----

func connectKind(url as string) {
    try {
        connect($url, crypto.keypair("p256").private);
    } catch (e) {
        return $e.kind;
    }
    return "";
}

func testPlainHttpToARemoteCaIsRefused() {
    # 192.0.2.1 (TEST-NET-1) is never reached: the refusal comes first.
    testing.assertEqual(connectKind("http://192.0.2.1/directory"), "acme");
    requireHttps("http://192.0.2.1/directory", true);
    requireHttps("https://ca.example/directory", false);
    requireHttps("http://127.0.0.1:4000/directory", false);
    # A client whose endpoints are plain http (a directory naming them) is
    # refused per request too.
    def c as Client init client();
    $c.newNonce = "http://192.0.2.1/nonce";
    def kind as string init "";
    try {
        fetchNonce($c);
    } catch (e) {
        $kind = $e.kind;
    }
    testing.assertEqual($kind, "acme");
}

# fakeCa answers `count` requests: a directory (good or broken), nonces and an
# authorization in the given state.
func fakeCa(srv as httpd.Server, count as int, authz as string) {
    def base as string init "http://" + httpd.address($srv);
    for (def i in 0..$count) {
        def r as httpd.Request init httpd.accept($srv);
        def p as string init httpd.path($r);
        if ($p == "/dir") {
            httpd.respond(
                $r,
                200,
                '{"newNonce":"' + $base + '/nonce","newAccount":"' + $base + '/acct","newOrder":"' +
                    $base +
                    '/order"}');
        } elseif ($p == "/baddir") {
            httpd.respond($r, 200, '{"newAccount":"x"}');
        } elseif ($p == "/htmldir") {
            httpd.respond($r, 200, "<html>");
        } elseif ($p == "/remotedir") {
            httpd.respond(
                $r,
                200,
                '{"newNonce":"http://192.0.2.1/n","newAccount":"http://192.0.2.1/a",' +
                    '"newOrder":"http://192.0.2.1/o"}');
        } elseif ($p == "/nonce") {
            httpd.setHeader($r, "Replay-Nonce", "abc");
            httpd.respond($r, 200, "");
        } else {
            httpd.respond($r, 200, $authz);
        }
    }
    return null;
}

func testConnectRefusesAMalformedDirectory() {
    def srv as httpd.Server init httpd.listen("127.0.0.1:0");
    defer httpd.shutdown($srv);
    def server as task of null init spawn {
        return fakeCa($srv, 4, "");
    };
    def base as string init "http://" + httpd.address($srv);
    testing.assertEqual(connectKind($base + "/baddir"), "acme");
    testing.assertEqual(connectKind($base + "/htmldir"), "acme");
    testing.assertEqual(connectKind($base + "/remotedir"), "acme");
    # loopback http is allowed
    testing.assertEqual(connectKind($base + "/dir"), "");
    task.wait($server);
}

func testAnInvalidAuthorizationThrows() {
    def srv as httpd.Server init httpd.listen("127.0.0.1:0");
    defer httpd.shutdown($srv);
    def server as task of null init spawn {
        return fakeCa(
            $srv,
            3,
            '{"identifier":{"type":"dns","value":"x.example"},"status":"invalid","challenges":[' +
                '{"type":"http-01","url":"https://ca/c/1","token":"abc","status":"invalid",' +
                '"error":{"type":"urn:ietf:params:acme:error:connection","detail":"timeout"}}]}');
    };
    def c as Client init connect(
        "http://" + httpd.address($srv) + "/dir",
        crypto.keypair("p256").private);
    $c.kid = "http://" + httpd.address($srv) + "/acct/1";
    def err as AcmeError;
    def threw as bool init false;
    try {
        pollAuthorization($c, "http://" + httpd.address($srv) + "/authz/1", 1, 3);
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "acme");
        $err = $e;
    }
    task.wait($server);
    testing.assertTrue($threw);
    testing.assertEqual($err.problemType, "urn:ietf:params:acme:error:connection");
    testing.assertEqual($err.detail, "timeout");
}
