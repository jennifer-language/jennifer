# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
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
    def key as bytes init crypto.ecGenerateKey("p256");
    return Client{directory: "", newNonce: "", newAccount: "", newOrder: "",
        accountKey: $key, alg: "ES256", kid: ""};
}

func testEncodeSegStripsPadding() {
    def b as bytes init convert.bytesFromString("hello", "utf-8");
    testing.assertFalse(strings.contains(encodeSeg($b), "="));
    # Round-trip shape: base64url of "hi" is "aGk" (padding removed).
    testing.assertEqual(encodeSeg(convert.bytesFromString("hi", "utf-8")), "aGk");
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
    def httpCh as Challenge init Challenge{kind: "http-01", url: "u1", token: "t1", status: "pending"};
    def dnsCh as Challenge init Challenge{kind: "dns-01", url: "u2", token: "t2", status: "pending"};
    def chs as list of Challenge init [];
    $chs[] = $httpCh;
    $chs[] = $dnsCh;
    def authz as Authorization init Authorization{domain: "example.com", status: "pending", challenges: $chs};
    testing.assertEqual(challenge($authz, "http-01").url, "u1");
    testing.assertEqual(challenge($authz, "dns-01").token, "t2");
}

func testChallengeMissingThrows() {
    testing.assertThrows("selectMissing", "acme");
}
func selectMissing() {
    def chs as list of Challenge init [];
    $chs[] = Challenge{kind: "http-01", url: "u", token: "t", status: "pending"};
    def authz as Authorization init Authorization{domain: "d", status: "pending", challenges: $chs};
    challenge($authz, "tls-alpn-01");
}

func testParseOrderReadsFields() {
    # parseOrder builds an Order from a response body; feed it a canned one.
    def resp as http.Response init http.Response{status: 200, statusText: "OK", headers: {},
        body: "{\"status\":\"pending\",\"authorizations\":[\"https://ca/authz/1\",\"https://ca/authz/2\"],\"finalize\":\"https://ca/finalize\"}"};
    def o as Order init parseOrder("https://ca/order/1", $resp);
    testing.assertEqual($o.url, "https://ca/order/1");
    testing.assertEqual($o.status, "pending");
    testing.assertEqual(len($o.authorizations), 2);
    testing.assertEqual($o.authorizations[1], "https://ca/authz/2");
    testing.assertEqual($o.certificate, "");        # not issued yet
}
