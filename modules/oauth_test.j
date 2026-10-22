# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# oauth_test.j - white-box tests for oauth.j's pure helpers. Run with:
#
#     jennifer test modules/oauth_test.j
#
# The overlay splices oauth.j in front of this file, so the tests reach its
# private form-body builder, token parser, poll classifier, and expiry check by
# bare identifier (the clock is passed in as `nowUnix` so they are deterministic).
# The form encoding itself lives in the shared `uri` module (tested in
# uri_test.j). The networked grants (client-credentials / refresh / device flow)
# are verified against an in-process OAuth2 token endpoint in the Go suite
# (TestOauthFlows).
use testing;

func testFormBody() {
    def p as map of string to string init {"grant_type": "client_credentials", "scope": "a b"};
    testing.assertEqual(formBody($p), "grant_type=client_credentials&scope=a+b");
}

func testTokenFromNode() {
    def node as json.Value init json.decode('{"access_token":"abc","token_type":"Bearer",' +
        '"expires_in":3600,"refresh_token":"r1","scope":"mail"}');
    def t as Token init tokenFromNode($node, 1000);
    testing.assertEqual($t.accessToken, "abc");
    testing.assertEqual($t.tokenType, "Bearer");
    testing.assertEqual($t.refreshToken, "r1");
    testing.assertEqual($t.scope, "mail");
    testing.assertEqual($t.expiresAt, 4600); # nowUnix 1000 + 3600
}

func testTokenFromNodeDefaults() {
    def node as json.Value init json.decode('{"access_token":"x"}');
    def t as Token init tokenFromNode($node, 1000);
    testing.assertEqual($t.tokenType, "Bearer"); # default
    testing.assertEqual($t.refreshToken, "");
    testing.assertEqual($t.expiresAt, 0); # no expires_in -> unknown
}

func testParseTokenError() {
    def threw as bool init false;
    try {
        parseTokenBody('{"error":"invalid_grant","error_description":"bad token"}', 1000);
    } catch (e) {
        $threw = true;
        testing.assertContains($e.message, "invalid_grant");
        testing.assertContains($e.message, "bad token");
    }
    testing.assertTrue($threw);
}

func testPollState() {
    testing.assertEqual(pollState('{"access_token":"x"}'), "success");
    def pending as string init pollState('{"error":"authorization_pending"}');
    testing.assertEqual($pending, "authorization_pending");
    testing.assertEqual(pollState('{"error":"slow_down"}'), "slow_down");
}

func testTokenExpired() {
    testing.assertFalse(tokenExpired(0, 1000)); # unknown expiry -> not expired
    testing.assertFalse(tokenExpired(2000, 1000)); # 1030 < 2000
    testing.assertTrue(tokenExpired(1020, 1000)); # 1030 >= 1020 (within 30s skew)
    testing.assertTrue(tokenExpired(1000, 1000)); # already past
}

# ---- JWT bearer grant (RFC 7523) ----

func testAssertionClaims() {
    def extra as json.Value init json.set(json.map(), "/scope", "a b");
    def c as json.Value init assertionClaims(
        "svc@example.com",
        "",
        "https://idp/token",
        300,
        $extra,
        1000);
    testing.assertEqual(json.asString($c, "/iss"), "svc@example.com");
    testing.assertFalse(json.has($c, "/sub")); # empty subject is left out
    testing.assertEqual(json.asString($c, "/aud"), "https://idp/token");
    testing.assertEqual(json.asInt($c, "/iat"), 1000);
    testing.assertEqual(json.asInt($c, "/exp"), 1300);
    testing.assertEqual(json.asString($c, "/scope"), "a b");
    def withSub as json.Value init assertionClaims(
        "i",
        "user@example.com",
        "a",
        60,
        json.map(),
        1000);
    testing.assertEqual(json.asString($withSub, "/sub"), "user@example.com");
}

func reservedExtraClaim() {
    def extra as json.Value init json.set(json.map(), "/exp", 9);
    assertionClaims("i", "", "a", 60, $extra, 1000);
}

func emptyIssuer() {
    assertionClaims("", "", "a", 60, json.map(), 1000);
}

func zeroLifetime() {
    assertionClaims("i", "", "a", 0, json.map(), 1000);
}

func listExtra() {
    assertionClaims("i", "", "a", 60, json.list(), 1000);
}

func testAssertionClaimsRejects() {
    testing.assertThrows("reservedExtraClaim", "oauth");
    testing.assertThrows("emptyIssuer", "oauth");
    testing.assertThrows("zeroLifetime", "oauth");
    testing.assertThrows("listExtra", "oauth");
}

func testJwtAssertionSigns() {
    def key as bytes init convert.bytesFromString("an-assertion-secret-of-32-bytes!", "utf-8");
    def tok as string init jwtAssertion(
        "i",
        "s",
        "https://idp/token",
        300,
        json.map(),
        $key,
        "HS256",
        "k1");
    testing.assertEqual(json.asString(jwt.header($tok), "/kid"), "k1");
    def c as json.Value init jwt.verify($tok, $key, "HS256");
    testing.assertEqual(json.asString($c, "/sub"), "s");
    testing.assertEqual(json.asInt($c, "/exp") - json.asInt($c, "/iat"), 300);
}

func testJwtBearerParams() {
    def bare as Config init Config{
        tokenUrl: "t",
        deviceUrl: "",
        clientId: "",
        clientSecret: "",
        scope: ""
    };
    def p as map of string to string init jwtBearerParams($bare, "A.B.C");
    testing.assertEqual($p["grant_type"], "urn:ietf:params:oauth:grant-type:jwt-bearer");
    testing.assertEqual($p["assertion"], "A.B.C");
    testing.assertEqual(len($p), 2); # no empty scope / client fields
    def full as Config init Config{
        tokenUrl: "t",
        deviceUrl: "",
        clientId: "id",
        clientSecret: "sec",
        scope: "x"
    };
    def q as map of string to string init jwtBearerParams($full, "A.B.C");
    testing.assertEqual($q["scope"], "x");
    testing.assertEqual($q["client_id"], "id");
    testing.assertEqual($q["client_secret"], "sec");
}

# ---- token store permissions (0600) ----

func testSaveTightensPermissions() {
    def dir as string init fs.makeTempDir();
    # A fresh path so writeString creates it at the default 0644; save must
    # then chmod it to owner-only 0600 (384).
    def path as string init $dir + "/token.json";
    def t as Token init Token{
        accessToken: "secret-access",
        tokenType: "Bearer",
        refreshToken: "r",
        scope: "",
        expiresAt: 0
    };
    save($path, $t);
    def st as fs.Stat init fs.stat($path);
    testing.assertEqual($st.mode, 384);
    # Round-trips.
    def back as Token init load($path);
    testing.assertEqual($back.accessToken, "secret-access");
    fs.removeAll($dir);
}
