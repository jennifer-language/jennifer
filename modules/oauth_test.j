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
    def node as json.Value init json.decode('{"access_token":"x","token_type":"Bearer"}');
    def t as Token init tokenFromNode($node, 1000);
    testing.assertEqual($t.tokenType, "Bearer");
    testing.assertEqual($t.refreshToken, "");
    testing.assertEqual($t.expiresAt, 0); # no expires_in -> unknown
}

func testParseTokenError() {
    def threw as bool init false;
    try {
        parseTokenBody('{"error":"invalid_grant","error_description":"bad token"}', 400, 1000);
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
    def key as bytes init convert.bytesFromString("utf-8", "an-assertion-secret-of-32-bytes!");
    def tok as string init jwtAssertion(
        "HS256",
        "i",
        "s",
        "https://idp/token",
        300,
        json.map(),
        $key,
        "k1");
    testing.assertEqual(json.asString(jwt.header($tok), "/kid"), "k1");
    def c as json.Value init jwt.verify("HS256", $tok, $key);
    testing.assertEqual(json.asString($c, "/sub"), "s");
    testing.assertEqual(json.asInt($c, "/exp") - json.asInt($c, "/iat"), 300);
}

func testJwtBearerParams() {
    def bare as Config init Config{
        tokenUrl: "t",
        deviceUrl: "",
        authorizeUrl: "",
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
        authorizeUrl: "",
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
    # A fresh path: save creates it, makes it owner-only 0600 (384) and only
    # then writes the token.
    def path as string init $dir + "/token.json";
    def t as Token init Token{
        accessToken: "secret-access",
        tokenType: "Bearer",
        refreshToken: "r",
        scope: "",
        expiresAt: 0,
        idToken: "eyJ.id.tok",
        extra: {"instance_url": "https://eu1.example.com", "a/b~c": "1"}
    };
    save($path, $t);
    def st as fs.Stat init fs.stat($path);
    testing.assertEqual($st.mode, 384);
    # Round-trips.
    def back as Token init load($path);
    testing.assertEqual($back.accessToken, "secret-access");
    testing.assertEqual($back.idToken, "eyJ.id.tok");
    testing.assertEqual($back.extra, {"instance_url": "https://eu1.example.com", "a/b~c": "1"});
    fs.removeAll($dir);
}

# ---- token fields beyond the standard five ----

func testTokenKeepsIdTokenAndExtraFields() {
    def node as json.Value init json.decode('{"access_token":"a","token_type":"Bearer","id_token":"h.p.s",' +
        '"instance_url":"https://eu1.my.salesforce.com","ext_expires_in":7200,' +
        '"signed":true,"ratio":1.5,"nested":{"x":1},"list":[1],"none":null,"a/b~c":"k"}');
    def t as Token init tokenFromNode($node, 1000);
    testing.assertEqual($t.idToken, "h.p.s");
    testing.assertEqual(
        $t.extra,
        {
            "instance_url": "https://eu1.my.salesforce.com",
            "ext_expires_in": "7200",
            "signed": "true",
            "ratio": "1.5",
            "a/b~c": "k"
        });
    def plain as Token init tokenFromNode(
        json.decode('{"access_token":"a","token_type":"bearer"}'),
        1000);
    testing.assertEqual($plain.idToken, "");
    testing.assertEqual(len($plain.extra), 0);
}

func testLoadReadsATokenFileFromBefore026() {
    def dir as string init fs.makeTempDir();
    def path as string init $dir + "/old.json";
    fs.writeNew(
        $path,
        '{"accessToken":"a","tokenType":"Bearer","refreshToken":"r","scope":"","expiresAt":5}');
    def t as Token init load($path);
    fs.removeAll($dir);
    testing.assertEqual($t.refreshToken, "r");
    testing.assertEqual($t.idToken, "");
    testing.assertEqual(len($t.extra), 0);
}

# ---- discovery ----

func testMetadataUrls() {
    testing.assertEqual(
        metadataUrls("https://accounts.google.com"),
        [
            "https://accounts.google.com/.well-known/openid-configuration",
            "https://accounts.google.com/.well-known/oauth-authorization-server"
        ]);
    testing.assertEqual(
        metadataUrls("https://id.example.com/realms/main/"),
        [
            "https://id.example.com/realms/main/.well-known/openid-configuration",
            "https://id.example.com/.well-known/oauth-authorization-server/realms/main"
        ]);
}

func testEndpointOk() {
    for (def ok in [
        "https://a.example/token",
        "http://127.0.0.1:8080/t",
        "http://localhost/t",
        "http://[::1]:9/t",
        "HTTPS://A/t"
    ]) {
        testing.assertTrue(endpointOk($ok));
    }
    for (def bad in [
        "http://a.example/token",
        "http://127.0.0.1.evil.example/",
        "https://",
        "ftp://a/t",
        "https://a b/t",
        "/token",
        ""
    ]) {
        testing.assertFalse(endpointOk($bad));
    }
}

# ---- client assertions ----

func testClientAssertion() {
    def key as bytes init convert.bytesFromString("utf-8", "a-shared-secret-of-enough-length!");
    def a as string init clientAssertion("HS256", "app-1", "https://idp.example/token", $key, "k1");
    def c as json.Value init jwt.verify("HS256", $a, $key);
    testing.assertEqual(json.asString($c, "/iss"), "app-1");
    testing.assertEqual(json.asString($c, "/sub"), "app-1");
    testing.assertEqual(json.asString($c, "/aud"), "https://idp.example/token");
    testing.assertEqual(json.asInt($c, "/exp") - json.asInt($c, "/iat"), 300);
    testing.assertEqual(len(json.asString($c, "/jti")), 22);
    def b as string init clientAssertion("HS256", "app-1", "https://idp.example/token", $key, "k1");
    testing.assertFalse(json.asString(jwt.decode($b), "/jti") == json.asString($c, "/jti"));
    def cfg as Config init Config{
        tokenUrl: "t",
        deviceUrl: "",
        authorizeUrl: "",
        clientId: "app-1",
        clientSecret: "never-sent",
        scope: ""
    };
    def p as map of string to string init clientAssertionParams($cfg, "A.B.C");
    testing.assertEqual(
        $p,
        {
            "grant_type": "client_credentials",
            "client_id": "app-1",
            "client_assertion_type": "urn:ietf:params:oauth:client-assertion-type:jwt-bearer",
            "client_assertion": "A.B.C"
        });
}

# ---- authorization code + PKCE ----

func testPkceChallenge() {
    # computed independently: base64url(sha256(verifier)), unpadded
    testing.assertEqual(
        challengeOf("dBjftJeZ4CVP-mJ92K9XyLHtyD3fhAgnzlYkKyqBVfE"),
        "3UENYDN5lPY1bNfkjUWF19xYY9RvyOLzg0sR0Sft7k4");
}

func testAuthorizeBuildsTheUrl() {
    def cfg as Config init Config{
        tokenUrl: "https://idp.example/token",
        deviceUrl: "",
        authorizeUrl: "https://idp.example/authorize?tenant=x",
        clientId: "app 1",
        clientSecret: "",
        scope: "openid email"
    };
    def r as AuthRequest init authorize($cfg, "http://127.0.0.1:8765/callback");
    testing.assertEqual(len($r.state), 22);
    testing.assertEqual(len($r.verifier), 43);
    testing.assertEqual(
        $r.url,
        "https://idp.example/authorize?tenant=x&response_type=code&client_id=app+1" +
            "&redirect_uri=http%3A%2F%2F127.0.0.1%3A8765%2Fcallback&scope=openid+email" +
            "&state=" + $r.state + "&code_challenge=" + challengeOf($r.verifier) +
            "&code_challenge_method=S256");
    def again as AuthRequest init authorize($cfg, "http://127.0.0.1:8765/callback");
    testing.assertFalse($again.state == $r.state);
    testing.assertFalse($again.verifier == $r.verifier);
    for (def bad in ["", "http://127.0.0.1/cb#x", "http://a b/"]) {
        def msg as string init "";
        try {
            authorize($cfg, $bad);
        } catch (e) {
            $msg = $e.message;
        }
        testing.assertContains($msg, "authorize: the redirect URI must be");
    }
    $cfg.authorizeUrl = "";
    def msg2 as string init "";
    try {
        authorize($cfg, "http://127.0.0.1/cb");
    } catch (e) {
        $msg2 = $e.kind + ": " + $e.message;
    }
    testing.assertEqual($msg2, "oauth: authorize: the config has no authorizeUrl");
}

func testCodeFromRedirect() {
    def req as AuthRequest init AuthRequest{
        url: "",
        state: "st4te",
        verifier: "v",
        redirectUri: "http://127.0.0.1:1/cb"
    };
    testing.assertEqual(
        codeFromRedirect($req, "http://127.0.0.1:1/cb?code=c%2B1&state=st4te"),
        "c+1");
    testing.assertEqual(codeFromRedirect($req, "state=st4te&code=abc#frag"), "abc");
    def cases as map of string to string init {
        "http://127.0.0.1:1/cb?code=abc&state=other": "the redirect's state does not match",
        "http://127.0.0.1:1/cb?code=abc": "the redirect's state does not match",
        "http://127.0.0.1:1/cb?state=st4te&error=access_denied&error_description=User+said+no": "authorization refused: access_denied: User said no",
        "http://127.0.0.1:1/cb?state=st4te": "the redirect carries no code",
        "http://127.0.0.1:1/cb?state=st4te&code=%zz": "the redirect's query is malformed"
    };
    for (def url in maps.keys($cases)) {
        def msg as string init "";
        try {
            codeFromRedirect($req, $url);
        } catch (e) {
            $msg = $e.kind + ": " + $e.message;
        }
        testing.assertContains($msg, "oauth: ");
        testing.assertContains($msg, $cases[$url]);
    }
    def p as map of string to string init codeParams(
        Config{
            tokenUrl: "t",
            deviceUrl: "",
            authorizeUrl: "",
            clientId: "id",
            clientSecret: "",
            scope: ""
        },
        $req,
        "abc");
    testing.assertEqual(
        $p,
        {
            "grant_type": "authorization_code",
            "code": "abc",
            "redirect_uri": "http://127.0.0.1:1/cb",
            "client_id": "id",
            "code_verifier": "v"
        });
}

# ---- strictness: malformed replies are oauth errors ----

/**
 * The error `parseTokenBody` throws for a reply, as "kind: message"; "" if it
 * accepts it.
 */
func tokenError(body as string, status as int) {
    try {
        parseTokenBody($body, $status, 1000);
    } catch (e) {
        return $e.kind + ": " + $e.message;
    }
    return "";
}

func testMalformedTokenRepliesAreOauthErrors() {
    def cases as list of list of string init [
        ['{"token_type":"Bearer"}', "the token response has no access_token"],
        [
            '{"access_token":12345,"token_type":"Bearer"}',
            "the token response's access_token is not a string"
        ],
        ['{"access_token":"","token_type":"Bearer"}', "the token response's access_token is empty"],
        ['{"access_token":"A"}', "the token response has no token_type"],
        ['{"access_token":"A","token_type":""}', "the token response's token_type is empty"],
        [
            '{"access_token":"A","token_type":"Bearer","expires_in":315360001}',
            "expires_in must be a whole number"
        ],
        [
            '{"access_token":"A","token_type":"Bearer","expires_in":9223372036854775807}',
            "expires_in must be a whole number"
        ],
        [
            '{"access_token":"A","token_type":"Bearer","expires_in":"3599"}',
            "expires_in must be a whole number of seconds from 1 to 315360000, got \"3599\""
        ],
        [
            '{"access_token":"A","token_type":"Bearer","expires_in":3599.5}',
            "expires_in must be a whole number"
        ],
        [
            '{"access_token":"A","token_type":"Bearer","expires_in":0}',
            "expires_in must be a whole number"
        ],
        [
            '{"access_token":"A","token_type":"Bearer","expires_in":-5}',
            "expires_in must be a whole number"
        ],
        [
            '{"access_token":"A","token_type":"Bearer","scope":null}',
            "the token response's scope is not a string"
        ],
        [
            '{"access_token":"A","token_type":"Bearer","refresh_token":7}',
            "refresh_token is not a string"
        ],
        ['{"access_token":"A","token_type":"Bearer","id_token":{}}', "id_token is not a string"],
        ['[1,2]', "the token response is not a JSON object"],
        ['{"error":{"code":400}}', "malformed error response: error is not a string"],
        [
            '{"error":"x","error_description":5}',
            "malformed error response: error_description is not a string"
        ]
    ];
    for (def c in $cases) {
        def got as string init tokenError($c[0], 200);
        testing.assertContains($got, "oauth: ");
        testing.assertContains($got, $c[1]);
    }
    testing.assertEqual(
        tokenError("<html>bad gateway</html>", 502),
        "oauth: non-JSON response from the token endpoint (HTTP 502)");
    testing.assertEqual(
        tokenError('{"message":"down"}', 503),
        "oauth: the token endpoint answered HTTP 503");
    testing.assertEqual(
        tokenError('{"error":"x\u001b[31mred","error_description":"a\nb"}', 400),
        "oauth: x?[31mred: a?b");
}

func testDeviceRepliesAreChecked() {
    def ok as DeviceAuth init deviceFromNode(
        json.decode('{"device_code":"d","user_code":"WDJB-MJHT","verification_url":"https://g.co/device"}'),
        1000);
    testing.assertEqual($ok.verificationUri, "https://g.co/device");
    testing.assertEqual($ok.userCode, "WDJB-MJHT");
    testing.assertEqual($ok.interval, 5);
    testing.assertEqual($ok.expiresAt, 1900);
    for (def bad in [
        '{"user_code":"u","verification_uri":"https://x"}',
        '{"device_code":"d","user_code":"u"}',
        '{"device_code":"d","user_code":"u","verification_uri":"https://x","interval":0}',
        '{"device_code":"d","user_code":5,"verification_uri":"https://x"}',
        '{"device_code":"","user_code":"u","verification_uri":"https://x"}',
        '{"device_code":"d","user_code":"U\u001b","verification_uri":"https://x"}',
        '{"device_code":"d","user_code":"' + strings.repeat("A", 65) +
            '","verification_uri":"https://x"}',
        '{"device_code":"d","user_code":"u","verification_uri":"javascript:alert(1)"}',
        '{"device_code":"d","user_code":"u","verification_uri":"http://evil.example/d"}',
        '{"device_code":"d","user_code":"u","verification_uri":"https://x","interval":601}',
        '{"device_code":"d","user_code":"u","verification_uri":"https://x","expires_in":86401}',
        '{"device_code":"d","user_code":"u","verification_uri":"https://x","expires_in":9223372036854775807}'
    ]) {
        def msg as string init "";
        try {
            deviceFromNode(json.decode($bad), 1000);
        } catch (e) {
            $msg = $e.kind;
        }
        testing.assertEqual($msg, "oauth");
    }
    testing.assertEqual(pollState('{"error":{"x":1}}'), "invalid_response");
}

func testRefreshSendsNoEmptySecret() {
    def pub as Config init Config{
        tokenUrl: "t",
        deviceUrl: "",
        authorizeUrl: "",
        clientId: "id",
        clientSecret: "",
        scope: ""
    };
    testing.assertEqual(
        refreshParams($pub, "r"),
        {"grant_type": "refresh_token", "refresh_token": "r", "client_id": "id"});
    $pub.clientSecret = "s";
    testing.assertEqual(refreshParams($pub, "r")["client_secret"], "s");
}

func testExtraClaimNamesWithSlashes() {
    def extra as json.Value init json.set(json.map(), "/https:~1~1example.com~1claims", "v");
    $extra = json.set($extra, "/a~0b", "w");
    def c as json.Value init assertionClaims("iss", "", "aud", 60, $extra, 1000);
    testing.assertEqual(json.asString($c, "/https:~1~1example.com~1claims"), "v");
    testing.assertEqual(json.asString($c, "/a~0b"), "w");
}

# ---- URLs ----

func testEndpointsRefuseALoginPart() {
    for (def bad in [
        "http://localhost:1@127.0.0.2:4/token",
        "http://[::1]@evil.example/t",
        "http://127.0.0.1:80@evil.example/t",
        "https://@",
        "https://?",
        "https://#",
        "https:///token",
        "https://a.example/t#f",
        "https://u@a.example/t",
        "https://a.example:x/t"
    ]) {
        testing.assertFalse(endpointOk($bad));
    }
}

func testRedirectUris() {
    for (def ok in [
        "http://127.0.0.1:8765/cb",
        "http://[::1]:1/",
        "https://app.example/cb",
        "com.example.app:/callback",
        "com.example.app:/cb?x=1"
    ]) {
        testing.assertTrue(redirectOk($ok));
    }
    for (def bad in [
        "cb",
        "/cb",
        "http://evil.example/cb",
        "http://localhost@evil.example/",
        "myapp:/cb",
        "https://a.example/cb#x",
        "javascript:alert(1)",
        ""
    ]) {
        testing.assertFalse(redirectOk($bad));
    }
}

func testAuthorizeRefusesAnUnsafeAuthorizeUrl() {
    def cfg as Config init Config{
        tokenUrl: "https://idp.example/token",
        deviceUrl: "",
        authorizeUrl: "http://evil.example/a",
        clientId: "c",
        clientSecret: "",
        scope: ""
    };
    def msg as string init "";
    try {
        authorize($cfg, "http://127.0.0.1:1/cb");
    } catch (e) {
        $msg = $e.message;
    }
    testing.assertEqual(
        $msg,
        "authorize: the authorizeUrl must be https, got http://evil.example/a");
}

func testAuthorizeWithAddsParameters() {
    def cfg as Config init Config{
        tokenUrl: "https://idp.example/token",
        deviceUrl: "",
        authorizeUrl: "https://idp.example/authorize",
        clientId: "c",
        clientSecret: "",
        scope: ""
    };
    def r as AuthRequest init authorizeWith(
        $cfg,
        "http://127.0.0.1:1/cb",
        {"access_type": "offline", "prompt": "consent"});
    testing.assertTrue(strings.endsWith(
        $r.url,
        "&code_challenge_method=S256&access_type=offline&prompt=consent"));
    for (def k in ["state", "redirect_uri", "code_challenge", "scope"]) {
        def msg as string init "";
        try {
            authorizeWith($cfg, "http://127.0.0.1:1/cb", {$k: "x"});
        } catch (e) {
            $msg = $e.message;
        }
        testing.assertEqual($msg, "authorize: an extra parameter may not set " + $k);
    }
}

func testRedirectsAreCheckedStrictly() {
    def req as AuthRequest init AuthRequest{
        url: "",
        state: "S",
        verifier: "v",
        redirectUri: "http://127.0.0.1:8765/cb"
    };
    testing.assertEqual(codeFromRedirect($req, "HTTP://127.0.0.1:8765/cb?state=S&code=c"), "c");
    def cases as map of string to string init {
        "?code=one&state=S&code=two": "the redirect repeats code",
        "?state=S&state=S&code=x": "the redirect repeats state",
        "https://evil.example/cb?state=S&code=c": "the redirect goes to https://evil.example/cb, not",
        "http://127.0.0.1:8765/other?state=S&code=c": "not to the redirect URI",
        "http://127.0.0.1:9999/cb?state=S&code=c": "not to the redirect URI",
        "?state=S&error=e%1Bx": "authorization refused: e?x"
    };
    for (def url in maps.keys($cases)) {
        def msg as string init "";
        try {
            codeFromRedirect($req, $url);
        } catch (e) {
            $msg = $e.kind + ": " + $e.message;
        }
        testing.assertContains($msg, "oauth: ");
        testing.assertContains($msg, $cases[$url]);
    }
}

func testLoadRefusesAMalformedFile() {
    def dir as string init fs.makeTempDir();
    def path as string init $dir + "/bad.json";
    fs.writeNew($path, '{"accessToken":5}');
    def msg as string init "";
    try {
        load($path);
    } catch (e) {
        $msg = $e.kind + ": " + $e.message;
    }
    fs.removeAll($dir);
    testing.assertEqual($msg, "oauth: saved token file is malformed: " + $path);
}

func testClientCredentialsSendNoEmptyFields() {
    def c as Config init Config{
        tokenUrl: "https://idp.example/token",
        deviceUrl: "",
        authorizeUrl: "",
        clientId: "id",
        clientSecret: "",
        scope: ""
    };
    testing.assertEqual(clientParams($c), {"grant_type": "client_credentials", "client_id": "id"});
    $c.clientSecret = "s";
    $c.scope = "a b";
    testing.assertEqual(clientParams($c)["client_secret"], "s");
    testing.assertEqual(clientParams($c)["scope"], "a b");
}

func testGrantsRefuseAPlainHttpEndpoint() {
    for (def url in ["http://idp.example/token", "t", "https://user@idp.example/token"]) {
        def c as Config init Config{
            tokenUrl: $url,
            deviceUrl: $url,
            authorizeUrl: "",
            clientId: "id",
            clientSecret: "TOPSECRET",
            scope: ""
        };
        def msg as string init "";
        try {
            clientCredentials($c);
        } catch (e) {
            $msg = $e.kind + ": " + $e.message;
        }
        testing.assertContains($msg, "oauth: the endpoint must be an https URL");
        $msg = "";
        try {
            deviceStart($c);
        } catch (e) {
            $msg = $e.kind + ": " + $e.message;
        }
        testing.assertContains($msg, "oauth: the endpoint must be an https URL");
    }
}

func testDeviceWaitRefusesABadInterval() {
    def c as Config init Config{
        tokenUrl: "https://idp.example/token",
        deviceUrl: "",
        authorizeUrl: "",
        clientId: "id",
        clientSecret: "",
        scope: ""
    };
    for (def n in [0, -1, 601, 9223372036854775807]) {
        def d as DeviceAuth init DeviceAuth{
            deviceCode: "d",
            userCode: "u",
            verificationUri: "https://x",
            interval: $n,
            expiresAt: 0
        };
        def msg as string init "";
        try {
            deviceWait($c, $d);
        } catch (e) {
            $msg = $e.kind + ": " + $e.message;
        }
        testing.assertContains($msg, "oauth: deviceWait: the interval must be from 1 to 600 s");
    }
}

func testSaveReplacesALinkWithoutFollowingIt() {
    def dir as string init fs.makeTempDir();
    defer fs.removeAll($dir);
    def target as string init $dir + "/target";
    fs.writeString($target, "keep");
    fs.symlink($target, $dir + "/token.json");
    def tok as Token init Token{
        accessToken: "A",
        tokenType: "Bearer",
        refreshToken: "",
        scope: "",
        expiresAt: 0,
        idToken: "",
        extra: {}
    };
    save($dir + "/token.json", $tok);
    testing.assertEqual(fs.readString($target), "keep");
    testing.assertEqual(load($dir + "/token.json").accessToken, "A");
    testing.assertEqual(fs.stat($dir + "/token.json").mode & 0o777, 0o600);
    testing.assertEqual(fs.list($dir), ["target", "token.json"]);
}
