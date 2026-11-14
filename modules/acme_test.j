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
        kid: ""
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
