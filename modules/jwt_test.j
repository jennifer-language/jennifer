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
        "a-shared-secret-value-padded-out-to-sixty-four-bytes-for-hs512!!",
        "utf-8");
}
func sampleClaims() {
    return json.decode('{"sub":"ada","role":"admin","iat":1000}');
}

func testHmacRoundTripAllSizes() {
    for (def alg in ["HS256", "HS384", "HS512"]) {
        def tok as string init sign(sampleClaims(), secret(), $alg);
        testing.assertEqual(len(strings.split($tok, ".")), 3);
        def back as json.Value init verify($tok, secret(), $alg);
        testing.assertEqual(json.asString($back, "/sub"), "ada");
        testing.assertEqual(json.asString($back, "/role"), "admin");
        testing.assertEqual(json.asInt($back, "/iat"), 1000);
    }
}

func testHeaderIsAlgAndTyp() {
    def tok as string init sign(sampleClaims(), secret(), "HS256");
    def head as json.Value init header($tok);
    testing.assertEqual(json.asString($head, "/alg"), "HS256");
    testing.assertEqual(json.asString($head, "/typ"), "JWT");
}

func testSignWithKidSetsHeader() {
    def tok as string init signWithKid(sampleClaims(), secret(), "HS256", "key-1");
    def head as json.Value init header($tok);
    testing.assertEqual(json.asString($head, "/alg"), "HS256");
    testing.assertEqual(json.asString($head, "/kid"), "key-1");
    testing.assertEqual(json.asString($head, "/typ"), "JWT");
    testing.assertEqual(json.asString(verify($tok, secret(), "HS256"), "/sub"), "ada");
    # Selecting the key by kid works on a token signed this way.
    def keys as map of string to string init {
        "key-1": "a-shared-secret-value-padded-out-to-sixty-four-bytes-for-hs512!!"
    };
    testing.assertEqual(json.asString(verifyWithKeys($tok, $keys, "HS256"), "/role"), "admin");
}

func testSignWithKidEscapesAndEmpty() {
    # A kid with a quote stays one JSON string, not injected header members.
    def tok as string init signWithKid(sampleClaims(), secret(), "HS256", 'k"x');
    testing.assertEqual(json.asString(header($tok), "/kid"), 'k"x');
    # An empty kid gives exactly the plain sign header.
    def plain as string init signWithKid(sampleClaims(), secret(), "HS256", "");
    testing.assertFalse(json.has(header($plain), "/kid"));
    testing.assertEqual($plain, sign(sampleClaims(), secret(), "HS256"));
}

func testSignWithKidRejectsBadAlg() {
    testing.assertThrows("badKidAlg", "value");
}

func badKidAlg() {
    signWithKid(sampleClaims(), secret(), "none", "k");
}

func testDecodeDoesNotVerify() {
    # A token with a garbage signature still decodes (decode never checks it).
    def tok as string init sign(sampleClaims(), secret(), "HS256");
    def tampered as string init $tok[..len($tok) - 3] + "AAA";
    testing.assertEqual(json.asString(decode($tampered), "/sub"), "ada");
}

func testTamperedSignatureRejected() {
    testing.assertThrows("verifyTampered", "value");
}
func verifyTampered() {
    def tok as string init sign(sampleClaims(), secret(), "HS256");
    def bad as string init $tok[..len($tok) - 3] + "AAA";
    verify($bad, secret(), "HS256");
}

func testWrongKeyRejected() {
    testing.assertThrows("verifyWrongKey", "value");
}
func verifyWrongKey() {
    def tok as string init sign(sampleClaims(), secret(), "HS256");
    verify(
        $tok,
        convert.bytesFromString("a-different-secret-of-at-least-32-bytes", "utf-8"),
        "HS256");
}

func testAlgConfusionRejected() {
    # An HS256 token must not verify when the caller expects a different alg,
    # even with the same key bytes - the header alg is checked.
    testing.assertThrows("verifyAsWrongAlg", "value");
}
func verifyAsWrongAlg() {
    def tok as string init sign(sampleClaims(), secret(), "HS256");
    verify($tok, secret(), "HS384");
}

func testUnsupportedAlgRejected() {
    testing.assertThrows("signNoneAlg", "value");
    testing.assertThrows("signBogusAlg", "value");
}
func signNoneAlg() {
    sign(sampleClaims(), secret(), "none");
}
func signBogusAlg() {
    sign(sampleClaims(), secret(), "HS999");
}

func testMalformedTokenRejected() {
    testing.assertThrows("verifyTwoSegments", "value");
    testing.assertThrows("decodeEmpty", "value");
}
func verifyTwoSegments() {
    verify("a.b", secret(), "HS256");
}
func decodeEmpty() {
    decode("");
}

func testExpiredRejected() {
    testing.assertThrows("verifyExpired", "value");
}
func verifyExpired() {
    def claims as json.Value init json.decode('{"sub":"x","exp":1}');
    def tok as string init sign($claims, secret(), "HS256");
    verify($tok, secret(), "HS256");
}

func testNotBeforeRejected() {
    testing.assertThrows("verifyNotYet", "value");
}
func verifyNotYet() {
    # nbf far in the future.
    def claims as json.Value init json.decode('{"sub":"x","nbf":9999999999}');
    def tok as string init sign($claims, secret(), "HS256");
    verify($tok, secret(), "HS256");
}

func testValidExpAndNbfAccepted() {
    # exp in the far future, nbf in the past -> valid now.
    def claims as json.Value init json.decode('{"sub":"ok","exp":9999999999,"nbf":1}');
    def tok as string init sign($claims, secret(), "HS256");
    testing.assertEqual(json.asString(verify($tok, secret(), "HS256"), "/sub"), "ok");
}

func testEddsaRoundTrip() {
    def kp as crypto.Keypair init crypto.signKeypair();
    def tok as string init sign(sampleClaims(), $kp.private, "EdDSA");
    testing.assertEqual(json.asString(verify($tok, $kp.public, "EdDSA"), "/sub"), "ada");
    testing.assertEqual(json.asString(header($tok), "/alg"), "EdDSA");
}

func testEddsaWrongKeyRejected() {
    testing.assertThrows("verifyEddsaWrongKey", "value");
}
func verifyEddsaWrongKey() {
    def kp as crypto.Keypair init crypto.signKeypair();
    def other as crypto.Keypair init crypto.signKeypair();
    def tok as string init sign(sampleClaims(), $kp.private, "EdDSA");
    verify($tok, $other.public, "EdDSA");
}

# ---- private helpers ----

func testSegmentCodecRoundTrip() {
    # Unpadded base64url round-trips arbitrary bytes (lengths hitting each
    # padding case: 0, 1, 2 mod 3).
    for (def s in ["", "a", "ab", "abc", "hello world"]) {
        def b as bytes init convert.bytesFromString($s, "utf-8");
        testing.assertEqual(convert.stringFromBytes(decodeSegment(encodeSegment($b)), "utf-8"), $s);
    }
}

func testSegmentEncodeHasNoPadding() {
    def b as bytes init convert.bytesFromString("hi", "utf-8");
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
    def tok as string init sign(sampleClaims(), secret(), "HS256");
    verify($tok + "=", secret(), "HS256");
}

# ---- reject a token carrying an unsupported crit header (RFC 7515) ----

func verifyCritToken() {
    def head as string init encodeSegment(convert.bytesFromString(
        '{"alg":"HS256","typ":"JWT","crit":["exp"]}',
        "utf-8"));
    def payload as string init encodeSegment(convert.bytesFromString('{"sub":"x"}', "utf-8"));
    verify($head + "." + $payload + ".AAAA", secret(), "HS256");
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
    def tok as string init sign(claimsWithIss(), secret(), "HS256");
    def back as json.Value init verifyWith(
        $tok,
        secret(),
        "HS256",
        Policy{iss: "good-iss", aud: "good-aud", requireExp: false, leeway: 0});
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}

func testVerifyWithEmptyPolicyActsLikeVerify() {
    def tok as string init sign(claimsWithIss(), secret(), "HS256");
    def back as json.Value init verifyWith(
        $tok,
        secret(),
        "HS256",
        Policy{iss: "", aud: "", requireExp: false, leeway: 0});
    testing.assertEqual(json.asString($back, "/iss"), "good-iss");
}

func testVerifyWithAudienceArrayMatch() {
    def tok as string init sign(claimsWithAudList(), secret(), "HS256");
    def back as json.Value init verifyWith(
        $tok,
        secret(),
        "HS256",
        Policy{iss: "", aud: "good-aud", requireExp: false, leeway: 0});
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}

func testVerifyWithWrongIssuerRejected() {
    testing.assertThrows("verifyBadIss", "value");
}
func verifyBadIss() {
    def tok as string init sign(claimsWithIss(), secret(), "HS256");
    verifyWith(
        $tok,
        secret(),
        "HS256",
        Policy{iss: "wrong-iss", aud: "", requireExp: false, leeway: 0});
}

func testVerifyWithWrongAudienceRejected() {
    testing.assertThrows("verifyBadAud", "value");
}
func verifyBadAud() {
    def tok as string init sign(claimsWithIss(), secret(), "HS256");
    verifyWith(
        $tok,
        secret(),
        "HS256",
        Policy{iss: "", aud: "wrong-aud", requireExp: false, leeway: 0});
}

# ---- verifyLeeway: clock-skew tolerance ----

# expiredBy builds a token whose exp is `secondsAgo` seconds in the past.
func expiredBy(secondsAgo as int) {
    def now as int init time.unix(time.now());
    def claims as json.Value init json.decode('{"sub":"x","exp":' +
        convert.toString($now - $secondsAgo) + '}');
    return sign($claims, secret(), "HS256");
}
# notYetBy builds a token whose nbf is `secondsAhead` seconds in the future.
func notYetBy(secondsAhead as int) {
    def now as int init time.unix(time.now());
    def claims as json.Value init json.decode('{"sub":"y","nbf":' +
        convert.toString($now + $secondsAhead) + '}');
    return sign($claims, secret(), "HS256");
}

func testLeewayAcceptsRecentlyExpired() {
    # Expired 5s ago, but within a 60s leeway -> accepted.
    def back as json.Value init verifyLeeway(expiredBy(5), secret(), "HS256", 60);
    testing.assertEqual(json.asString($back, "/sub"), "x");
}
func testLeewayRejectsWellExpired() {
    testing.assertThrows("verifyExpiredBeyondLeeway", "value");
}
func verifyExpiredBeyondLeeway() {
    # Expired 300s ago, only 60s leeway -> still rejected.
    verifyLeeway(expiredBy(300), secret(), "HS256", 60);
}
func testZeroLeewayStillRejectsExpired() {
    testing.assertThrows("verifyExpiredZeroLeeway", "value");
}
func verifyExpiredZeroLeeway() {
    verifyLeeway(expiredBy(5), secret(), "HS256", 0);
}

func testLeewayAcceptsNotYetWithinLeeway() {
    # nbf 5s in the future, within a 60s leeway -> accepted.
    def back as json.Value init verifyLeeway(notYetBy(5), secret(), "HS256", 60);
    testing.assertEqual(json.asString($back, "/sub"), "y");
}
func testLeewayRejectsNotYetBeyondLeeway() {
    testing.assertThrows("verifyNotYetBeyondLeeway", "value");
}
func verifyNotYetBeyondLeeway() {
    verifyLeeway(notYetBy(300), secret(), "HS256", 60);
}

func testNegativeLeewayRejected() {
    testing.assertThrows("verifyNegativeLeeway", "value");
}
func verifyNegativeLeeway() {
    def tok as string init sign(sampleClaims(), secret(), "HS256");
    verifyLeeway($tok, secret(), "HS256", -1);
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
    def tok as string init signWithKid(sampleClaims(), secret(), "HS256", "k2");
    def back as json.Value init verifyWithKeys($tok, keyRing(), "HS256");
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}
func testVerifyWithKeysUnknownKidRejected() {
    testing.assertThrows("verifyUnknownKid", "value");
}
func verifyUnknownKid() {
    def tok as string init signWithKid(sampleClaims(), secret(), "HS256", "nope");
    verifyWithKeys($tok, keyRing(), "HS256");
}
func testVerifyWithKeysMissingKidRejected() {
    testing.assertThrows("verifyNoKidHeader", "value");
}
func verifyNoKidHeader() {
    # A plain jwt.sign token has no kid header.
    def tok as string init sign(sampleClaims(), secret(), "HS256");
    verifyWithKeys($tok, keyRing(), "HS256");
}
func testVerifyWithKeysWrongKeyRejected() {
    testing.assertThrows("verifyKidWrongKey", "value");
}
func verifyKidWrongKey() {
    # kid "k1" is found, but its secret does not match secret() -> bad signature.
    def tok as string init signWithKid(sampleClaims(), secret(), "HS256", "k1");
    verifyWithKeys($tok, keyRing(), "HS256");
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
    def tok as string init signWithKid(sampleClaims(), $priv, "RS256", "rk1");
    def back as json.Value init verifyJwks($tok, $jwks, "RS256");
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}

func testVerifyJwksEc() {
    def priv as bytes init crypto.ecGenerateKey("p256");
    def jwks as string init jwksFor($priv, "ec1");
    def tok as string init signWithKid(sampleClaims(), $priv, "ES256", "ec1");
    def back as json.Value init verifyJwks($tok, $jwks, "ES256");
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}

func testVerifyJwksUnknownKidRejected() {
    testing.assertThrows("jwksUnknownKid", "value");
}
func jwksUnknownKid() {
    def priv as bytes init rsaKey();
    def jwks as string init jwksFor($priv, "present");
    def tok as string init signWithKid(sampleClaims(), $priv, "RS256", "absent");
    verifyJwks($tok, $jwks, "RS256");
}

func testVerifyJwksHmacRejected() {
    testing.assertThrows("jwksHmac", "value");
}
func jwksHmac() {
    # An HMAC alg has no JWKS public key.
    verifyJwks(signWithKid(sampleClaims(), secret(), "HS256", "k1"), '{"keys":[]}', "HS256");
}

func testVerifyJwksNoKeysArrayRejected() {
    testing.assertThrows("jwksNoKeys", "value");
}
func jwksNoKeys() {
    def priv as bytes init rsaKey();
    def tok as string init signWithKid(sampleClaims(), $priv, "RS256", "rk1");
    verifyJwks($tok, '{"notkeys":1}', "RS256");
}

# ---- verifyWithKeys now also accepts a JWK map value (via crypto.jwkToPem) ----

func testVerifyWithKeysAcceptsJwk() {
    # The kid-map value is a raw JWK (from crypto.jwkPublic), not a pre-converted PEM.
    def priv as bytes init rsaKey();
    def ring as map of string to string init {"rk": crypto.jwkPublic($priv)};
    def tok as string init signWithKid(sampleClaims(), $priv, "RS256", "rk");
    def back as json.Value init verifyWithKeys($tok, $ring, "RS256");
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}

func testVerifyWithKeysStillAcceptsPem() {
    # A PEM value (the pre-existing contract) still passes through unchanged.
    def priv as bytes init rsaKey();
    def ring as map of string to string init {"rk": crypto.jwkToPem(crypto.jwkPublic($priv))};
    def tok as string init signWithKid(sampleClaims(), $priv, "RS256", "rk");
    def back as json.Value init verifyWithKeys($tok, $ring, "RS256");
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}

# requireExp refuses a token with no exp (RFC 7519 makes it optional, so plain
# verify accepts one); a token that does carry exp still verifies normally.
func testVerifyWithRequireExpRejectsTokenWithoutExp() {
    testing.assertThrows("verifyNoExpRequired", "value");
}
func verifyNoExpRequired() {
    def tok as string init sign(json.set(json.map(), "/sub", "1"), secret(), "HS256");
    return verifyWith(
        $tok,
        secret(),
        "HS256",
        Policy{iss: "", aud: "", requireExp: true, leeway: 0});
}
func testVerifyWithRequireExpAcceptsTokenWithExp() {
    def claims as json.Value init json.set(json.map(), "/exp", time.unix(time.now()) + 600);
    def tok as string init sign($claims, secret(), "HS256");
    def back as json.Value init verifyWith(
        $tok,
        secret(),
        "HS256",
        Policy{iss: "", aud: "", requireExp: true, leeway: 0});
    testing.assertTrue(json.has($back, "/exp"));
}
func testVerifyWithoutRequireExpStillAcceptsNoExp() {
    def tok as string init sign(json.set(json.map(), "/sub", "1"), secret(), "HS256");
    def back as json.Value init verifyWith(
        $tok,
        secret(),
        "HS256",
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
    def tok as string init signWithKid(claimsFor("good-aud", true), secret(), "HS256", "k2");
    def back as json.Value init verifyWithKeysWith($tok, keyRing(), "HS256", strictPolicy());
    testing.assertEqual(json.asString($back, "/sub"), "ci");
}
func testVerifyWithKeysWithRejectsMissingExp() {
    testing.assertThrows("keysNoExp", "value");
}
func keysNoExp() {
    def tok as string init signWithKid(claimsFor("good-aud", false), secret(), "HS256", "k2");
    verifyWithKeysWith($tok, keyRing(), "HS256", strictPolicy());
}
func testVerifyWithKeysWithRejectsForeignAudience() {
    testing.assertThrows("keysForeignAud", "value");
}
func keysForeignAud() {
    def tok as string init signWithKid(claimsFor("other-service", true), secret(), "HS256", "k2");
    verifyWithKeysWith($tok, keyRing(), "HS256", strictPolicy());
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
    def tok as string init signWithKid(claimsFor("good-aud", true), $priv, "ES256", "ec1");
    def back as json.Value init verifyJwksWith(
        $tok,
        jwksFor($priv, "ec1"),
        "ES256",
        strictPolicy());
    testing.assertEqual(json.asString($back, "/iss"), "good-iss");
}
func testVerifyJwksWithRejectsMissingExp() {
    testing.assertThrows("jwksNoExp", "value");
}
func jwksNoExp() {
    def priv as bytes init crypto.ecGenerateKey("p256");
    def tok as string init signWithKid(claimsFor("good-aud", false), $priv, "ES256", "ec1");
    verifyJwksWith($tok, jwksFor($priv, "ec1"), "ES256", strictPolicy());
}
func testVerifyJwksWithRejectsForeignAudience() {
    testing.assertThrows("jwksForeignAud", "value");
}
func jwksForeignAud() {
    def priv as bytes init crypto.ecGenerateKey("p256");
    def tok as string init signWithKid(claimsFor("other-service", true), $priv, "ES256", "ec1");
    verifyJwksWith($tok, jwksFor($priv, "ec1"), "ES256", strictPolicy());
}
# A non-string iss is a policy mismatch, not a json accessor error.
func testPolicyNonStringIssuerRejectedCleanly() {
    def msg as string init "";
    try {
        def c as json.Value init json.set(claimsFor("good-aud", true), "/iss", 7);
        verifyWith(sign($c, secret(), "HS256"), secret(), "HS256", strictPolicy());
    } catch (e) {
        $msg = $e.message;
    }
    testing.assertTrue(strings.contains($msg, "issuer (iss) does not match"));
}

# ---- HMAC key length (RFC 7518 3.2) ----

func shortKeySign() {
    sign(sampleClaims(), convert.bytesFromString("too-short", "utf-8"), "HS256");
}
func emptyKeySign() {
    def empty as bytes;
    sign(sampleClaims(), $empty, "HS256");
}
# A 32-byte key is enough for HS256 but not for HS384.
func key32ForHs384() {
    sign(
        sampleClaims(),
        convert.bytesFromString("0123456789abcdef0123456789abcdef", "utf-8"),
        "HS384");
}
# The verifying side refuses a short key too, so a misconfigured verifier can
# never accept tokens forged with an empty secret.
func shortKeyVerify() {
    def tok as string init sign(sampleClaims(), secret(), "HS256");
    def empty as bytes;
    verify($tok, $empty, "HS256");
}
func testHmacKeyTooShortRejected() {
    testing.assertThrows("shortKeySign", "value");
    testing.assertThrows("emptyKeySign", "value");
    testing.assertThrows("key32ForHs384", "value");
    testing.assertThrows("shortKeyVerify", "value");
    def key32 as bytes init convert.bytesFromString("0123456789abcdef0123456789abcdef", "utf-8");
    def tok as string init sign(sampleClaims(), $key32, "HS256");
    testing.assertEqual(json.asString(verify($tok, $key32, "HS256"), "/sub"), "ada");
}

# ---- header and claims must be JSON objects ----

# rawToken signs an arbitrary header / payload JSON with secret() under HS256,
# bypassing sign's object check, the way a hostile issuer could.
func rawToken(headerJson as string, payloadJson as string) {
    def input as string init encodeSegment(convert.bytesFromString($headerJson, "utf-8")) + "." +
        encodeSegment(convert.bytesFromString($payloadJson, "utf-8"));
    def sig as bytes init computeSig("HS256", convert.bytesFromString($input, "utf-8"), secret());
    return $input + "." + encodeSegment($sig);
}
func signArrayClaims() {
    sign(json.decode("[1,2]"), secret(), "HS256");
}
func verifyArrayClaims() {
    verify(rawToken('{"alg":"HS256","typ":"JWT"}', '[1,2]'), secret(), "HS256");
}
func verifyStringClaims() {
    verify(rawToken('{"alg":"HS256","typ":"JWT"}', '"hi"'), secret(), "HS256");
}
func verifyArrayHeader() {
    verify(rawToken('["HS256"]', '{"sub":"x"}'), secret(), "HS256");
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
    verify(rawToken('{"alg":256}', '{"sub":"x"}'), secret(), "HS256");
}
func numericKid() {
    verifyWithKeys(rawToken('{"alg":"HS256","kid":7}', '{"sub":"x"}'), keyRing(), "HS256");
}
func stringExp() {
    verify(rawToken('{"alg":"HS256"}', '{"exp":"99999999999"}'), secret(), "HS256");
}
func floatExpPast() {
    verify(rawToken('{"alg":"HS256"}', '{"exp":1000.5}'), secret(), "HS256");
}
func floatNbfFuture() {
    verify(rawToken('{"alg":"HS256"}', '{"nbf":99999999999.5}'), secret(), "HS256");
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
    testing.assertEqual(json.asString(verify($tok, secret(), "HS256"), "/sub"), "f");
}

# A non-string element of an `aud` array is skipped: a string element still
# matches, and an array of only non-strings is a clean mismatch.
func testAudienceArraySkipsNonStrings() {
    def p as Policy init Policy{iss: "", aud: "svc", requireExp: false, leeway: 0};
    def tok as string init sign(json.decode('{"aud":[1,"svc"]}'), secret(), "HS256");
    testing.assertEqual(json.typeOf(verifyWith($tok, secret(), "HS256", $p), "/aud"), "list");
}
func audOnlyNumbers() {
    def p as Policy init Policy{iss: "", aud: "svc", requireExp: false, leeway: 0};
    verifyWith(sign(json.decode('{"aud":[1,2]}'), secret(), "HS256"), secret(), "HS256", $p);
}
func testAudienceArrayOfNumbersIsMismatch() {
    testing.assertThrows("audOnlyNumbers", "value");
}

# ---- Policy.leeway ----

func recentlyExpiredToken() {
    def c as json.Value init json.set(sampleClaims(), "/exp", time.unix(time.now()) - 10);
    return signWithKid($c, secret(), "HS256", "k2");
}
func testPolicyLeewayAcceptsRecentlyExpired() {
    def p as Policy init Policy{iss: "", aud: "", requireExp: true, leeway: 60};
    testing.assertEqual(
        json.asString(verifyWith(recentlyExpiredToken(), secret(), "HS256", $p), "/sub"),
        "ada");
    def back as json.Value init verifyWithKeysWith(recentlyExpiredToken(), keyRing(), "HS256", $p);
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}
func policyNoLeeway() {
    def p as Policy init Policy{iss: "", aud: "", requireExp: true, leeway: 0};
    verifyWith(recentlyExpiredToken(), secret(), "HS256", $p);
}
func policyNegativeLeeway() {
    def p as Policy init Policy{iss: "", aud: "", requireExp: false, leeway: -1};
    verifyWith(sign(sampleClaims(), secret(), "HS256"), secret(), "HS256", $p);
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
    return signWithKid(sampleClaims(), $priv, "ES256", "ec1");
}
func jwkForEncryption() {
    def priv as bytes init crypto.ecGenerateKey("p256");
    verifyJwks(ecToken($priv), jwksWithMembers($priv, "ec1", '"use":"enc",'), "ES256");
}
func jwkForOtherAlg() {
    def priv as bytes init crypto.ecGenerateKey("p256");
    verifyJwks(ecToken($priv), jwksWithMembers($priv, "ec1", '"alg":"ES384",'), "ES256");
}
# An RSA JWK selected for an ES256 token is a family mismatch.
func jwkWrongKty() {
    def ec as bytes init crypto.ecGenerateKey("p256");
    verifyJwks(ecToken($ec), jwksFor(rsaKey(), "ec1"), "ES256");
}
# The same checks guard a JWK held in a verifyWithKeys map.
func keyMapJwkForEncryption() {
    def priv as bytes init crypto.ecGenerateKey("p256");
    def jwk as string init crypto.jwkPublic($priv);
    def keys as map of string to string init {"ec1": '{"use":"enc",' + $jwk[1..]};
    verifyWithKeys(ecToken($priv), $keys, "ES256");
}
func jwksEdDsa() {
    verifyJwks(sign(sampleClaims(), crypto.signKeypair().private, "EdDSA"), '{"keys":[]}', "EdDSA");
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
    testing.assertEqual(json.asString(verifyJwks(ecToken($priv), $ok, "ES256"), "/sub"), "ada");
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
    def keys as map of string to string init jwksKeys($set, "ES256");
    # Only "a" survives: "b" is for encryption, the kid-less key cannot be
    # selected, and "r" is an RSA key.
    testing.assertEqual(maps.keys($keys), ["a"]);
    testing.assertTrue(strings.startsWith($keys["a"], "-----BEGIN PUBLIC KEY-----"));
    def tok as string init signWithKid(sampleClaims(), $a, "ES256", "a");
    testing.assertEqual(json.asString(verifyWithKeys($tok, $keys, "ES256"), "/sub"), "ada");
}
func jwksKeysHmac() {
    jwksKeys('{"keys":[]}', "HS256");
}
func jwksKeysNoArray() {
    jwksKeys('{"key":[]}', "ES256");
}
func testJwksKeysRejects() {
    testing.assertThrows("jwksKeysHmac", "value");
    testing.assertThrows("jwksKeysNoArray", "value");
}
