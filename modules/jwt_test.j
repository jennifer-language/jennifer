# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# jwt_test.j - white-box tests for jwt.j. Run with:
#
#     jennifer test modules/jwt_test.j
#
# The overlay splices jwt.j in front of this file, so the tests reach its
# private helpers (encodeSegment / decodeSegment, algHash, family, requireAlg)
# by bare identifier as well as its exported surface. HMAC and EdDSA keys are
# makeable in pure Jennifer, so they are covered here; RS256 / ES256 need PEM
# keys and are covered by the Go test (cmd/jennifer/jwt_test.go).
use testing;
use json;
use crypto;
use convert;
use strings;

func secret() {
    return convert.bytesFromString(
        "utf-8",
        "a-shared-secret-value-padded-out-to-sixty-four-bytes-for-hs512!!");
}
func sampleClaims() {
    return json.decode('{"sub":"ada","role":"admin","iat":1000}');
}

func testHmacRoundTripAllSizes() {
    for (def alg in ["HS256", "HS384", "HS512"]) {
        def tok as string init sign($alg, sampleClaims(), secret());
        testing.assertEqual(len(strings.split($tok, ".")), 3);
        def back as json.Value init verify($alg, $tok, secret());
        testing.assertEqual(json.asString($back, "/sub"), "ada");
        testing.assertEqual(json.asString($back, "/role"), "admin");
        testing.assertEqual(json.asInt($back, "/iat"), 1000);
    }
}

func testHeaderIsAlgAndTyp() {
    def tok as string init sign("HS256", sampleClaims(), secret());
    def head as json.Value init header($tok);
    testing.assertEqual(json.asString($head, "/alg"), "HS256");
    testing.assertEqual(json.asString($head, "/typ"), "JWT");
}

func testSignWithKidSetsHeader() {
    def tok as string init signWithKid("HS256", sampleClaims(), secret(), "key-1");
    def head as json.Value init header($tok);
    testing.assertEqual(json.asString($head, "/alg"), "HS256");
    testing.assertEqual(json.asString($head, "/kid"), "key-1");
    testing.assertEqual(json.asString($head, "/typ"), "JWT");
    testing.assertEqual(json.asString(verify("HS256", $tok, secret()), "/sub"), "ada");
    # Selecting the key by kid works on a token signed this way.
    def keys as map of string to string init {
        "key-1": "a-shared-secret-value-padded-out-to-sixty-four-bytes-for-hs512!!"
    };
    testing.assertEqual(json.asString(verifyWithKeys("HS256", $tok, $keys), "/role"), "admin");
}

func testSignWithKidEscapesAndEmpty() {
    # A kid with a quote stays one JSON string, not injected header members.
    def tok as string init signWithKid("HS256", sampleClaims(), secret(), 'k"x');
    testing.assertEqual(json.asString(header($tok), "/kid"), 'k"x');
    # An empty kid gives exactly the plain sign header.
    def plain as string init signWithKid("HS256", sampleClaims(), secret(), "");
    testing.assertFalse(json.has(header($plain), "/kid"));
    testing.assertEqual($plain, sign("HS256", sampleClaims(), secret()));
}

func testSignWithKidRejectsBadAlg() {
    testing.assertThrows("badKidAlg", "value");
}

func badKidAlg() {
    signWithKid("none", sampleClaims(), secret(), "k");
}

func testDecodeDoesNotVerify() {
    # A token with a garbage signature still decodes (decode never checks it).
    def tok as string init sign("HS256", sampleClaims(), secret());
    def tampered as string init $tok[..len($tok) - 3] + "AAA";
    testing.assertEqual(json.asString(decode($tampered), "/sub"), "ada");
}

func testTamperedSignatureRejected() {
    testing.assertThrows("verifyTampered", "value");
}
func verifyTampered() {
    def tok as string init sign("HS256", sampleClaims(), secret());
    def bad as string init $tok[..len($tok) - 3] + "AAA";
    verify("HS256", $bad, secret());
}

func testWrongKeyRejected() {
    testing.assertThrows("verifyWrongKey", "value");
}
func verifyWrongKey() {
    def tok as string init sign("HS256", sampleClaims(), secret());
    verify(
        "HS256",
        $tok,
        convert.bytesFromString("utf-8", "a-different-secret-of-at-least-32-bytes"));
}

func testAlgConfusionRejected() {
    # An HS256 token must not verify when the caller expects a different alg,
    # even with the same key bytes - the header alg is checked.
    testing.assertThrows("verifyAsWrongAlg", "value");
}
func verifyAsWrongAlg() {
    def tok as string init sign("HS256", sampleClaims(), secret());
    verify("HS384", $tok, secret());
}

func testUnsupportedAlgRejected() {
    testing.assertThrows("signNoneAlg", "value");
    testing.assertThrows("signBogusAlg", "value");
}
func signNoneAlg() {
    sign("none", sampleClaims(), secret());
}
func signBogusAlg() {
    sign("HS999", sampleClaims(), secret());
}

func testMalformedTokenRejected() {
    testing.assertThrows("verifyTwoSegments", "value");
    testing.assertThrows("decodeEmpty", "value");
}
func verifyTwoSegments() {
    verify("HS256", "a.b", secret());
}
func decodeEmpty() {
    decode("");
}

func testExpiredRejected() {
    testing.assertThrows("verifyExpired", "value");
}
func verifyExpired() {
    def claims as json.Value init json.decode('{"sub":"x","exp":1}');
    def tok as string init sign("HS256", $claims, secret());
    verify("HS256", $tok, secret());
}

func testNotBeforeRejected() {
    testing.assertThrows("verifyNotYet", "value");
}
func verifyNotYet() {
    # nbf far in the future.
    def claims as json.Value init json.decode('{"sub":"x","nbf":9999999999}');
    def tok as string init sign("HS256", $claims, secret());
    verify("HS256", $tok, secret());
}

func testValidExpAndNbfAccepted() {
    # exp in the far future, nbf in the past -> valid now.
    def claims as json.Value init json.decode('{"sub":"ok","exp":9999999999,"nbf":1}');
    def tok as string init sign("HS256", $claims, secret());
    testing.assertEqual(json.asString(verify("HS256", $tok, secret()), "/sub"), "ok");
}

func testEddsaRoundTrip() {
    def kp as crypto.Keypair init crypto.signKeypair();
    def tok as string init sign("EdDSA", sampleClaims(), $kp.private);
    testing.assertEqual(json.asString(verify("EdDSA", $tok, $kp.public), "/sub"), "ada");
    testing.assertEqual(json.asString(header($tok), "/alg"), "EdDSA");
}

func testEddsaWrongKeyRejected() {
    testing.assertThrows("verifyEddsaWrongKey", "value");
}
func verifyEddsaWrongKey() {
    def kp as crypto.Keypair init crypto.signKeypair();
    def other as crypto.Keypair init crypto.signKeypair();
    def tok as string init sign("EdDSA", sampleClaims(), $kp.private);
    verify("EdDSA", $tok, $other.public);
}

# ---- private helpers ----

func testSegmentCodecRoundTrip() {
    # Unpadded base64url round-trips arbitrary bytes (lengths hitting each
    # padding case: 0, 1, 2 mod 3).
    for (def s in ["", "a", "ab", "abc", "hello world"]) {
        def b as bytes init convert.bytesFromString("utf-8", $s);
        testing.assertEqual(convert.stringFromBytes("utf-8", decodeSegment(encodeSegment($b))), $s);
    }
}

func testSegmentEncodeHasNoPadding() {
    def b as bytes init convert.bytesFromString("utf-8", "hi");
    testing.assertFalse(strings.contains(encodeSegment($b), "="));
}

func testAlgHashAndFamily() {
    testing.assertEqual(algHash("HS256"), "sha256");
    testing.assertEqual(algHash("RS384"), "sha384");
    testing.assertEqual(algHash("ES512"), "sha512");
    testing.assertEqual(algHash("EdDSA"), "");
    testing.assertEqual(family("HS256"), "hmac");
    testing.assertEqual(family("RS256"), "rsa");
    testing.assertEqual(family("ES256"), "ecdsa");
    testing.assertEqual(family("EdDSA"), "eddsa");
}

# ---- canonical base64url (token-malleability rejection) ----

# RFC 7515 A.1's signature ends in "JXk"; flipping the last character's lowest
# bit ("JXl") changes only base64 trailing-padding bits, so a lenient decoder
# yields the SAME 32 signature bytes - a second spelling of the same token.
# decodeSegment must reject the non-canonical spelling.
func testNonCanonicalTrailingBitsRejected() {
    testing.assertThrows("decodeTrailingBitFlip", "value");
}
func decodeTrailingBitFlip() {
    decodeSegment("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXl");
}
func testCanonicalRfcSignatureAccepted() {
    # The canonical spelling of the same segment decodes fine (32 bytes).
    testing.assertEqual(len(decodeSegment("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")), 32);
}

# A segment carrying the "=" padding JWT forbids is also a second spelling.
func testPaddedSegmentRejected() {
    testing.assertThrows("verifyPaddedSignature", "value");
}
func verifyPaddedSignature() {
    def tok as string init sign("HS256", sampleClaims(), secret());
    verify("HS256", $tok + "=", secret());
}

# ---- reject a token carrying an unsupported crit header (RFC 7515) ----

func verifyCritToken() {
    def head as string init encodeSegment(convert.bytesFromString(
        "utf-8",
        '{"alg":"HS256","typ":"JWT","crit":["exp"]}'));
    def payload as string init encodeSegment(convert.bytesFromString("utf-8", '{"sub":"x"}'));
    verify("HS256", $head + "." + $payload + ".AAAA", secret());
}
func testCritHeaderRejected() {
    testing.assertThrows("verifyCritToken", "value");
}

# ---- verifyWith: issuer / audience policy ----

func claimsWithIss() {
    return json.decode('{"sub":"ada","iss":"good-iss","aud":"good-aud"}');
}
func claimsWithAudList() {
    return json.decode('{"sub":"ada","aud":["x","good-aud"]}');
}

func testVerifyWithMatchingIssAndAud() {
    def tok as string init sign("HS256", claimsWithIss(), secret());
    def back as json.Value init verifyWith(
        "HS256",
        $tok,
        secret(),
        Policy{iss: "good-iss", aud: "good-aud", requireExp: false, leeway: 0});
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}

func testVerifyWithEmptyPolicyActsLikeVerify() {
    def tok as string init sign("HS256", claimsWithIss(), secret());
    def back as json.Value init verifyWith(
        "HS256",
        $tok,
        secret(),
        Policy{iss: "", aud: "", requireExp: false, leeway: 0});
    testing.assertEqual(json.asString($back, "/iss"), "good-iss");
}

func testVerifyWithAudienceArrayMatch() {
    def tok as string init sign("HS256", claimsWithAudList(), secret());
    def back as json.Value init verifyWith(
        "HS256",
        $tok,
        secret(),
        Policy{iss: "", aud: "good-aud", requireExp: false, leeway: 0});
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}

func testVerifyWithWrongIssuerRejected() {
    testing.assertThrows("verifyBadIss", "value");
}
func verifyBadIss() {
    def tok as string init sign("HS256", claimsWithIss(), secret());
    verifyWith(
        "HS256",
        $tok,
        secret(),
        Policy{iss: "wrong-iss", aud: "", requireExp: false, leeway: 0});
}

func testVerifyWithWrongAudienceRejected() {
    testing.assertThrows("verifyBadAud", "value");
}
func verifyBadAud() {
    def tok as string init sign("HS256", claimsWithIss(), secret());
    verifyWith(
        "HS256",
        $tok,
        secret(),
        Policy{iss: "", aud: "wrong-aud", requireExp: false, leeway: 0});
}

# ---- verifyLeeway: clock-skew tolerance ----

# expiredBy builds a token whose exp is `secondsAgo` seconds in the past.
func expiredBy(secondsAgo as int) {
    def now as int init time.unix(time.now());
    def claims as json.Value init json.decode('{"sub":"x","exp":' +
        convert.toString($now - $secondsAgo) + '}');
    return sign("HS256", $claims, secret());
}
# notYetBy builds a token whose nbf is `secondsAhead` seconds in the future.
func notYetBy(secondsAhead as int) {
    def now as int init time.unix(time.now());
    def claims as json.Value init json.decode('{"sub":"y","nbf":' +
        convert.toString($now + $secondsAhead) + '}');
    return sign("HS256", $claims, secret());
}

func testLeewayAcceptsRecentlyExpired() {
    # Expired 5s ago, but within a 60s leeway -> accepted.
    def back as json.Value init verifyLeeway("HS256", expiredBy(5), secret(), 60);
    testing.assertEqual(json.asString($back, "/sub"), "x");
}
func testLeewayRejectsWellExpired() {
    testing.assertThrows("verifyExpiredBeyondLeeway", "value");
}
func verifyExpiredBeyondLeeway() {
    # Expired 300s ago, only 60s leeway -> still rejected.
    verifyLeeway("HS256", expiredBy(300), secret(), 60);
}
func testZeroLeewayStillRejectsExpired() {
    testing.assertThrows("verifyExpiredZeroLeeway", "value");
}
func verifyExpiredZeroLeeway() {
    verifyLeeway("HS256", expiredBy(5), secret(), 0);
}

func testLeewayAcceptsNotYetWithinLeeway() {
    # nbf 5s in the future, within a 60s leeway -> accepted.
    def back as json.Value init verifyLeeway("HS256", notYetBy(5), secret(), 60);
    testing.assertEqual(json.asString($back, "/sub"), "y");
}
func testLeewayRejectsNotYetBeyondLeeway() {
    testing.assertThrows("verifyNotYetBeyondLeeway", "value");
}
func verifyNotYetBeyondLeeway() {
    verifyLeeway("HS256", notYetBy(300), secret(), 60);
}

func testNegativeLeewayRejected() {
    testing.assertThrows("verifyNegativeLeeway", "value");
}
func verifyNegativeLeeway() {
    def tok as string init sign("HS256", sampleClaims(), secret());
    verifyLeeway("HS256", $tok, secret(), -1);
}

# ---- verifyWithKeys: kid-based key selection ----

func keyRing() {
    return {
        "k1": "the-first-secret-padded-out-to-sixty-four-bytes-for-the-hs-tests",
        "k2": "a-shared-secret-value-padded-out-to-sixty-four-bytes-for-hs512!!"
    };
}

func testVerifyWithKeysSelectsRightKey() {
    # Token signed under secret() carries kid "k2", which maps to that same
    # secret in the ring -> verifies.
    def tok as string init signWithKid("HS256", sampleClaims(), secret(), "k2");
    def back as json.Value init verifyWithKeys("HS256", $tok, keyRing());
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}
func testVerifyWithKeysUnknownKidRejected() {
    testing.assertThrows("verifyUnknownKid", "value");
}
func verifyUnknownKid() {
    def tok as string init signWithKid("HS256", sampleClaims(), secret(), "nope");
    verifyWithKeys("HS256", $tok, keyRing());
}
func testVerifyWithKeysMissingKidRejected() {
    testing.assertThrows("verifyNoKidHeader", "value");
}
func verifyNoKidHeader() {
    # A plain jwt.sign token has no kid header.
    def tok as string init sign("HS256", sampleClaims(), secret());
    verifyWithKeys("HS256", $tok, keyRing());
}
func testVerifyWithKeysWrongKeyRejected() {
    testing.assertThrows("verifyKidWrongKey", "value");
}
func verifyKidWrongKey() {
    # kid "k1" is found, but its secret does not match secret() -> bad signature.
    def tok as string init signWithKid("HS256", sampleClaims(), secret(), "k1");
    verifyWithKeys("HS256", $tok, keyRing());
}

# ---- verifyJwks: kid-based key selection from a JWKS ----

# rsaKey caches one generated RSA key for the JWKS tests (generation is slow).
func rsaKey() {
    return crypto.rsaGenerateKey(2048);
}
# jwksFor builds a one-key JWKS from a private key's public JWK, tagged with kid.
# Splices "kid" into crypto.jwkPublic's canonical {"e":..,"kty":..,"n":..} object.
func jwksFor(privKey as bytes, kid as string) {
    def jwk as string init crypto.jwkPublic($privKey);
    def withKid as string init '{"kid":"' + $kid + "\"," + $jwk[1..];
    return '{"keys":[' + $withKid + ']}';
}

func testVerifyJwksRoundTrip() {
    def priv as bytes init rsaKey();
    def jwks as string init jwksFor($priv, "rk1");
    def tok as string init signWithKid("RS256", sampleClaims(), $priv, "rk1");
    def back as json.Value init verifyJwks("RS256", $tok, $jwks);
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}

func testVerifyJwksEc() {
    def priv as bytes init crypto.ecGenerateKey("p256");
    def jwks as string init jwksFor($priv, "ec1");
    def tok as string init signWithKid("ES256", sampleClaims(), $priv, "ec1");
    def back as json.Value init verifyJwks("ES256", $tok, $jwks);
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}

func testVerifyJwksUnknownKidRejected() {
    testing.assertThrows("jwksUnknownKid", "value");
}
func jwksUnknownKid() {
    def priv as bytes init rsaKey();
    def jwks as string init jwksFor($priv, "present");
    def tok as string init signWithKid("RS256", sampleClaims(), $priv, "absent");
    verifyJwks("RS256", $tok, $jwks);
}

func testVerifyJwksHmacRejected() {
    testing.assertThrows("jwksHmac", "value");
}
func jwksHmac() {
    # An HMAC alg has no JWKS public key.
    verifyJwks("HS256", signWithKid("HS256", sampleClaims(), secret(), "k1"), '{"keys":[]}');
}

func testVerifyJwksNoKeysArrayRejected() {
    testing.assertThrows("jwksNoKeys", "value");
}
func jwksNoKeys() {
    def priv as bytes init rsaKey();
    def tok as string init signWithKid("RS256", sampleClaims(), $priv, "rk1");
    verifyJwks("RS256", $tok, '{"notkeys":1}');
}

# ---- verifyWithKeys now also accepts a JWK map value (via crypto.jwkToPem) ----

func testVerifyWithKeysAcceptsJwk() {
    # The kid-map value is a raw JWK (from crypto.jwkPublic), not a pre-converted PEM.
    def priv as bytes init rsaKey();
    def ring as map of string to string init {"rk": crypto.jwkPublic($priv)};
    def tok as string init signWithKid("RS256", sampleClaims(), $priv, "rk");
    def back as json.Value init verifyWithKeys("RS256", $tok, $ring);
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}

func testVerifyWithKeysStillAcceptsPem() {
    # A PEM value (the pre-existing contract) still passes through unchanged.
    def priv as bytes init rsaKey();
    def ring as map of string to string init {"rk": crypto.jwkToPem(crypto.jwkPublic($priv))};
    def tok as string init signWithKid("RS256", sampleClaims(), $priv, "rk");
    def back as json.Value init verifyWithKeys("RS256", $tok, $ring);
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}

# requireExp refuses a token with no exp (RFC 7519 makes it optional, so plain
# verify accepts one); a token that does carry exp still verifies normally.
func testVerifyWithRequireExpRejectsTokenWithoutExp() {
    testing.assertThrows("verifyNoExpRequired", "value");
}
func verifyNoExpRequired() {
    def tok as string init sign("HS256", json.set(json.map(), "/sub", "1"), secret());
    return verifyWith(
        "HS256",
        $tok,
        secret(),
        Policy{iss: "", aud: "", requireExp: true, leeway: 0});
}
func testVerifyWithRequireExpAcceptsTokenWithExp() {
    def claims as json.Value init json.set(json.map(), "/exp", time.unix(time.now()) + 600);
    def tok as string init sign("HS256", $claims, secret());
    def back as json.Value init verifyWith(
        "HS256",
        $tok,
        secret(),
        Policy{iss: "", aud: "", requireExp: true, leeway: 0});
    testing.assertTrue(json.has($back, "/exp"));
}
func testVerifyWithoutRequireExpStillAcceptsNoExp() {
    def tok as string init sign("HS256", json.set(json.map(), "/sub", "1"), secret());
    def back as json.Value init verifyWith(
        "HS256",
        $tok,
        secret(),
        Policy{iss: "", aud: "", requireExp: false, leeway: 0});
    testing.assertFalse(json.has($back, "/exp"));
}

# ---- verifyWithKeysWith / verifyJwksWith: key sets with a claim policy ----

func strictPolicy() {
    return Policy{iss: "good-iss", aud: "good-aud", requireExp: true, leeway: 0};
}
# claimsFor builds claims with the given aud and, when withExp, an hour of life.
func claimsFor(aud as string, withExp as bool) {
    def c as json.Value init json.set(json.set(json.map(), "/sub", "ci"), "/iss", "good-iss");
    $c = json.set($c, "/aud", $aud);
    if ($withExp) {
        $c = json.set($c, "/exp", time.unix(time.now()) + 3600);
    }
    return $c;
}

func testVerifyWithKeysWithAcceptsMatchingToken() {
    def tok as string init signWithKid("HS256", claimsFor("good-aud", true), secret(), "k2");
    def back as json.Value init verifyWithKeysWith("HS256", $tok, keyRing(), strictPolicy());
    testing.assertEqual(json.asString($back, "/sub"), "ci");
}
func testVerifyWithKeysWithRejectsMissingExp() {
    testing.assertThrows("keysNoExp", "value");
}
func keysNoExp() {
    def tok as string init signWithKid("HS256", claimsFor("good-aud", false), secret(), "k2");
    verifyWithKeysWith("HS256", $tok, keyRing(), strictPolicy());
}
func testVerifyWithKeysWithRejectsForeignAudience() {
    testing.assertThrows("keysForeignAud", "value");
}
func keysForeignAud() {
    def tok as string init signWithKid("HS256", claimsFor("other-service", true), secret(), "k2");
    verifyWithKeysWith("HS256", $tok, keyRing(), strictPolicy());
}
# A failure is reported under the verifier's own name.
func testVerifyWithKeysWithNamesItself() {
    def msg as string init "";
    try {
        keysNoExp();
    } catch (e) {
        $msg = $e.message;
    }
    testing.assertTrue(strings.startsWith($msg, "jwt.verifyWithKeysWith:"));
}

func testVerifyJwksWithAcceptsMatchingToken() {
    def priv as bytes init crypto.ecGenerateKey("p256");
    def tok as string init signWithKid("ES256", claimsFor("good-aud", true), $priv, "ec1");
    def back as json.Value init verifyJwksWith(
        "ES256",
        $tok,
        jwksFor($priv, "ec1"),
        strictPolicy());
    testing.assertEqual(json.asString($back, "/iss"), "good-iss");
}
func testVerifyJwksWithRejectsMissingExp() {
    testing.assertThrows("jwksNoExp", "value");
}
func jwksNoExp() {
    def priv as bytes init crypto.ecGenerateKey("p256");
    def tok as string init signWithKid("ES256", claimsFor("good-aud", false), $priv, "ec1");
    verifyJwksWith("ES256", $tok, jwksFor($priv, "ec1"), strictPolicy());
}
func testVerifyJwksWithRejectsForeignAudience() {
    testing.assertThrows("jwksForeignAud", "value");
}
func jwksForeignAud() {
    def priv as bytes init crypto.ecGenerateKey("p256");
    def tok as string init signWithKid("ES256", claimsFor("other-service", true), $priv, "ec1");
    verifyJwksWith("ES256", $tok, jwksFor($priv, "ec1"), strictPolicy());
}
# A non-string iss is a policy mismatch, not a json accessor error.
func testPolicyNonStringIssuerRejectedCleanly() {
    def msg as string init "";
    try {
        def c as json.Value init json.set(claimsFor("good-aud", true), "/iss", 7);
        verifyWith("HS256", sign("HS256", $c, secret()), secret(), strictPolicy());
    } catch (e) {
        $msg = $e.message;
    }
    testing.assertTrue(strings.contains($msg, "issuer (iss) does not match"));
}

# ---- HMAC key length (RFC 7518 3.2) ----

func shortKeySign() {
    sign("HS256", sampleClaims(), convert.bytesFromString("utf-8", "too-short"));
}
func emptyKeySign() {
    def empty as bytes;
    sign("HS256", sampleClaims(), $empty);
}
# A 32-byte key is enough for HS256 but not for HS384.
func key32ForHs384() {
    sign(
        "HS384",
        sampleClaims(),
        convert.bytesFromString("utf-8", "0123456789abcdef0123456789abcdef"));
}
# The verifying side refuses a short key too, so a misconfigured verifier can
# never accept tokens forged with an empty secret.
func shortKeyVerify() {
    def tok as string init sign("HS256", sampleClaims(), secret());
    def empty as bytes;
    verify("HS256", $tok, $empty);
}
func testHmacKeyTooShortRejected() {
    testing.assertThrows("shortKeySign", "value");
    testing.assertThrows("emptyKeySign", "value");
    testing.assertThrows("key32ForHs384", "value");
    testing.assertThrows("shortKeyVerify", "value");
    def key32 as bytes init convert.bytesFromString("utf-8", "0123456789abcdef0123456789abcdef");
    def tok as string init sign("HS256", sampleClaims(), $key32);
    testing.assertEqual(json.asString(verify("HS256", $tok, $key32), "/sub"), "ada");
}

# ---- header and claims must be JSON objects ----

# rawToken signs an arbitrary header / payload JSON with secret() under HS256,
# bypassing sign's object check, the way a hostile issuer could.
func rawToken(headerJson as string, payloadJson as string) {
    def input as string init encodeSegment(convert.bytesFromString("utf-8", $headerJson)) + "." +
        encodeSegment(convert.bytesFromString("utf-8", $payloadJson));
    def sig as bytes init computeSig("HS256", convert.bytesFromString("utf-8", $input), secret());
    return $input + "." + encodeSegment($sig);
}
func signArrayClaims() {
    sign("HS256", json.decode("[1,2]"), secret());
}
func verifyArrayClaims() {
    verify("HS256", rawToken('{"alg":"HS256","typ":"JWT"}', '[1,2]'), secret());
}
func verifyStringClaims() {
    verify("HS256", rawToken('{"alg":"HS256","typ":"JWT"}', '"hi"'), secret());
}
func verifyArrayHeader() {
    verify("HS256", rawToken('["HS256"]', '{"sub":"x"}'), secret());
}
func decodeArrayClaims() {
    decode(rawToken('{"alg":"HS256"}', '[1]'));
}
func testNonObjectSegmentsRejected() {
    testing.assertThrows("signArrayClaims", "value");
    testing.assertThrows("verifyArrayClaims", "value");
    testing.assertThrows("verifyStringClaims", "value");
    testing.assertThrows("verifyArrayHeader", "value");
    testing.assertThrows("decodeArrayClaims", "value");
}

# ---- member types: a wrong type is a value error, never a runtime one ----

func numericAlg() {
    verify("HS256", rawToken('{"alg":256}', '{"sub":"x"}'), secret());
}
func numericKid() {
    verifyWithKeys("HS256", rawToken('{"alg":"HS256","kid":7}', '{"sub":"x"}'), keyRing());
}
func stringExp() {
    verify("HS256", rawToken('{"alg":"HS256"}', '{"exp":"99999999999"}'), secret());
}
func floatExpPast() {
    verify("HS256", rawToken('{"alg":"HS256"}', '{"exp":1000.5}'), secret());
}
func floatNbfFuture() {
    verify("HS256", rawToken('{"alg":"HS256"}', '{"nbf":99999999999.5}'), secret());
}
func testWrongMemberTypesRejectedAsValue() {
    testing.assertThrows("numericAlg", "value");
    testing.assertThrows("numericKid", "value");
    testing.assertThrows("stringExp", "value");
    testing.assertThrows("floatExpPast", "value");
    testing.assertThrows("floatNbfFuture", "value");
}

# A fractional NumericDate is legal (RFC 7519 2) and is honoured, not refused.
func testFractionalTimeClaimsAccepted() {
    def tok as string init rawToken(
        '{"alg":"HS256"}',
        '{"sub":"f","exp":99999999999.5,"nbf":1000.25}');
    testing.assertEqual(json.asString(verify("HS256", $tok, secret()), "/sub"), "f");
}

# A non-string element of an `aud` array is skipped: a string element still
# matches, and an array of only non-strings is a clean mismatch.
func testAudienceArraySkipsNonStrings() {
    def p as Policy init Policy{iss: "", aud: "svc", requireExp: false, leeway: 0};
    def tok as string init sign("HS256", json.decode('{"aud":[1,"svc"]}'), secret());
    testing.assertEqual(json.typeOf(verifyWith("HS256", $tok, secret(), $p), "/aud"), "list");
}
func audOnlyNumbers() {
    def p as Policy init Policy{iss: "", aud: "svc", requireExp: false, leeway: 0};
    verifyWith("HS256", sign("HS256", json.decode('{"aud":[1,2]}'), secret()), secret(), $p);
}
func testAudienceArrayOfNumbersIsMismatch() {
    testing.assertThrows("audOnlyNumbers", "value");
}

# ---- Policy.leeway ----

func recentlyExpiredToken() {
    def c as json.Value init json.set(sampleClaims(), "/exp", time.unix(time.now()) - 10);
    return signWithKid("HS256", $c, secret(), "k2");
}
func testPolicyLeewayAcceptsRecentlyExpired() {
    def p as Policy init Policy{iss: "", aud: "", requireExp: true, leeway: 60};
    testing.assertEqual(
        json.asString(verifyWith("HS256", recentlyExpiredToken(), secret(), $p), "/sub"),
        "ada");
    def back as json.Value init verifyWithKeysWith("HS256", recentlyExpiredToken(), keyRing(), $p);
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}
func policyNoLeeway() {
    def p as Policy init Policy{iss: "", aud: "", requireExp: true, leeway: 0};
    verifyWith("HS256", recentlyExpiredToken(), secret(), $p);
}
func policyNegativeLeeway() {
    def p as Policy init Policy{iss: "", aud: "", requireExp: false, leeway: -1};
    verifyWith("HS256", sign("HS256", sampleClaims(), secret()), secret(), $p);
}
func testPolicyLeewayBounds() {
    testing.assertThrows("policyNoLeeway", "value");
    testing.assertThrows("policyNegativeLeeway", "value");
}

# ---- JWK usage checks (use / alg / kty) ----

# jwksWithMembers is jwksFor with extra JWK members spliced in (e.g. `"use":"enc",`).
func jwksWithMembers(privKey as bytes, kid as string, members as string) {
    def jwk as string init crypto.jwkPublic($privKey);
    return '{"keys":[{"kid":"' + $kid + "\"," + $members + $jwk[1..] + ']}';
}
func ecToken(priv as bytes) {
    return signWithKid("ES256", sampleClaims(), $priv, "ec1");
}
func jwkForEncryption() {
    def priv as bytes init crypto.ecGenerateKey("p256");
    verifyJwks("ES256", ecToken($priv), jwksWithMembers($priv, "ec1", '"use":"enc",'));
}
func jwkForOtherAlg() {
    def priv as bytes init crypto.ecGenerateKey("p256");
    verifyJwks("ES256", ecToken($priv), jwksWithMembers($priv, "ec1", '"alg":"ES384",'));
}
# An RSA JWK selected for an ES256 token is a family mismatch.
func jwkWrongKty() {
    def ec as bytes init crypto.ecGenerateKey("p256");
    verifyJwks("ES256", ecToken($ec), jwksFor(rsaKey(), "ec1"));
}
# The same checks guard a JWK held in a verifyWithKeys map.
func keyMapJwkForEncryption() {
    def priv as bytes init crypto.ecGenerateKey("p256");
    def jwk as string init crypto.jwkPublic($priv);
    def keys as map of string to string init {"ec1": '{"use":"enc",' + $jwk[1..]};
    verifyWithKeys("ES256", ecToken($priv), $keys);
}
func jwksEdDsa() {
    verifyJwks("EdDSA", sign("EdDSA", sampleClaims(), crypto.signKeypair().private), '{"keys":[]}');
}
func testJwkUsageChecked() {
    testing.assertThrows("jwkForEncryption", "value");
    testing.assertThrows("jwkForOtherAlg", "value");
    testing.assertThrows("jwkWrongKty", "value");
    testing.assertThrows("keyMapJwkForEncryption", "value");
    testing.assertThrows("jwksEdDsa", "value");
    # Matching use / alg members are accepted.
    def priv as bytes init crypto.ecGenerateKey("p256");
    def ok as string init jwksWithMembers($priv, "ec1", '"use":"sig","alg":"ES256",');
    testing.assertEqual(json.asString(verifyJwks("ES256", ecToken($priv), $ok), "/sub"), "ada");
}

# ---- jwksKeys ----

func testJwksKeysConvertsUsableKeys() {
    def a as bytes init crypto.ecGenerateKey("p256");
    def b as bytes init crypto.ecGenerateKey("p256");
    def jwkA as string init crypto.jwkPublic($a);
    def jwkB as string init crypto.jwkPublic($b);
    def set as string init '{"keys":[' + '{"kid":"a",' + $jwkA[1..] + "," +
        '{"kid":"b","use":"enc",' + $jwkB[1..] + "," + $jwkB + "," +
        '{"kid":"r",' + crypto.jwkPublic(rsaKey())[1..] + ']}';
    def keys as map of string to string init jwksKeys("ES256", $set);
    # Only "a" survives: "b" is for encryption, the kid-less key cannot be
    # selected, and "r" is an RSA key.
    testing.assertEqual(maps.keys($keys), ["a"]);
    testing.assertTrue(strings.startsWith($keys["a"], "-----BEGIN PUBLIC KEY-----"));
    def tok as string init signWithKid("ES256", sampleClaims(), $a, "a");
    testing.assertEqual(json.asString(verifyWithKeys("ES256", $tok, $keys), "/sub"), "ada");
}
func jwksKeysHmac() {
    jwksKeys("HS256", '{"keys":[]}');
}
func jwksKeysNoArray() {
    jwksKeys("ES256", '{"key":[]}');
}
func testJwksKeysRejects() {
    testing.assertThrows("jwksKeysHmac", "value");
    testing.assertThrows("jwksKeysNoArray", "value");
}

# ---- absent members read as "missing", not as an error ----

# An issuer policy rejects a token that carries no iss at all.
func testVerifyWithMissingIssRejected() {
    testing.assertThrows("verifyNoIss", "value");
}
func verifyNoIss() {
    def tok as string init sign("HS256", json.decode('{"sub":"ada"}'), secret());
    verifyWith(
        "HS256",
        $tok,
        secret(),
        Policy{iss: "good-iss", aud: "", requireExp: false, leeway: 0});
}

# A JWK without a kid is never selected; a JWKS without keys is rejected.
func testJwkWithoutKidAndJwksWithoutKeys() {
    testing.assertEqual(jwkKid(json.decode('{"kty":"oct"}')), "");
    testing.assertThrowsWith("jwksSet", ["jwt.test", '{"nokeys":[]}'], "value");
}

# An unsupported algorithm is named in the error when it looks like one; any
# other value (a token or key passed in the wrong place) is left out, so it
# cannot end up in a log.
func testUnsupportedAlgErrorDoesNotEchoSecrets() {
    def key as bytes init convert.bytesFromString("utf-8", "k");
    def named as string init "";
    try {
        verify("HS257", "a.b.c", $key);
    } catch (e) {
        $named = $e.message;
    }
    testing.assertContains($named, "unsupported algorithm HS257");
    def hidden as string init "";
    try {
        verify("eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ4In0.sig", "HS256", $key);
    } catch (e) {
        $hidden = $e.message;
    }
    testing.assertContains($hidden, "unsupported algorithm");
    testing.assertFalse(strings.contains($hidden, "eyJ"));
}
