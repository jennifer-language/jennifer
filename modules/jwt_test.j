# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
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
    return convert.bytesFromString("a-shared-secret-value", "utf-8");
}
func sampleClaims() {
    return json.decode("{\"sub\":\"ada\",\"role\":\"admin\",\"iat\":1000}");
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

func testDecodeDoesNotVerify() {
    # A token with a garbage signature still decodes (decode never checks it).
    def tok as string init sign(sampleClaims(), secret(), "HS256");
    def tampered as string init strings.substring($tok, 0, len($tok) - 3) + "AAA";
    testing.assertEqual(json.asString(decode($tampered), "/sub"), "ada");
}

func testTamperedSignatureRejected() {
    testing.assertThrows("verifyTampered", "value");
}
func verifyTampered() {
    def tok as string init sign(sampleClaims(), secret(), "HS256");
    def bad as string init strings.substring($tok, 0, len($tok) - 3) + "AAA";
    verify($bad, secret(), "HS256");
}

func testWrongKeyRejected() {
    testing.assertThrows("verifyWrongKey", "value");
}
func verifyWrongKey() {
    def tok as string init sign(sampleClaims(), secret(), "HS256");
    verify($tok, convert.bytesFromString("different-secret", "utf-8"), "HS256");
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
    def claims as json.Value init json.decode("{\"sub\":\"x\",\"exp\":1}");
    def tok as string init sign($claims, secret(), "HS256");
    verify($tok, secret(), "HS256");
}

func testNotBeforeRejected() {
    testing.assertThrows("verifyNotYet", "value");
}
func verifyNotYet() {
    # nbf far in the future.
    def claims as json.Value init json.decode("{\"sub\":\"x\",\"nbf\":9999999999}");
    def tok as string init sign($claims, secret(), "HS256");
    verify($tok, secret(), "HS256");
}

func testValidExpAndNbfAccepted() {
    # exp in the far future, nbf in the past -> valid now.
    def claims as json.Value init json.decode("{\"sub\":\"ok\",\"exp\":9999999999,\"nbf\":1}");
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
        "{\"alg\":\"HS256\",\"typ\":\"JWT\",\"crit\":[\"exp\"]}",
        "utf-8"));
    def payload as string init encodeSegment(convert.bytesFromString("{\"sub\":\"x\"}", "utf-8"));
    verify($head + "." + $payload + ".AAAA", secret(), "HS256");
}
func testCritHeaderRejected() {
    testing.assertThrows("verifyCritToken", "value");
}

# ---- verifyWith: issuer / audience policy ----

func claimsWithIss() {
    return json.decode("{\"sub\":\"ada\",\"iss\":\"good-iss\",\"aud\":\"good-aud\"}");
}
func claimsWithAudList() {
    return json.decode("{\"sub\":\"ada\",\"aud\":[\"x\",\"good-aud\"]}");
}

func testVerifyWithMatchingIssAndAud() {
    def tok as string init sign(claimsWithIss(), secret(), "HS256");
    def back as json.Value init verifyWith(
        $tok,
        secret(),
        "HS256",
        Policy{iss: "good-iss", aud: "good-aud"});
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}

func testVerifyWithEmptyPolicyActsLikeVerify() {
    def tok as string init sign(claimsWithIss(), secret(), "HS256");
    def back as json.Value init verifyWith($tok, secret(), "HS256", Policy{iss: "", aud: ""});
    testing.assertEqual(json.asString($back, "/iss"), "good-iss");
}

func testVerifyWithAudienceArrayMatch() {
    def tok as string init sign(claimsWithAudList(), secret(), "HS256");
    def back as json.Value init verifyWith(
        $tok,
        secret(),
        "HS256",
        Policy{iss: "", aud: "good-aud"});
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}

func testVerifyWithWrongIssuerRejected() {
    testing.assertThrows("verifyBadIss", "value");
}
func verifyBadIss() {
    def tok as string init sign(claimsWithIss(), secret(), "HS256");
    verifyWith($tok, secret(), "HS256", Policy{iss: "wrong-iss", aud: ""});
}

func testVerifyWithWrongAudienceRejected() {
    testing.assertThrows("verifyBadAud", "value");
}
func verifyBadAud() {
    def tok as string init sign(claimsWithIss(), secret(), "HS256");
    verifyWith($tok, secret(), "HS256", Policy{iss: "", aud: "wrong-aud"});
}

# ---- verifyLeeway: clock-skew tolerance ----

# expiredBy builds a token whose exp is `secondsAgo` seconds in the past.
func expiredBy(secondsAgo as int) {
    def now as int init time.unix(time.now());
    def claims as json.Value init json.decode("{\"sub\":\"x\",\"exp\":" +
        convert.toString($now - $secondsAgo) + "}");
    return sign($claims, secret(), "HS256");
}
# notYetBy builds a token whose nbf is `secondsAhead` seconds in the future.
func notYetBy(secondsAhead as int) {
    def now as int init time.unix(time.now());
    def claims as json.Value init json.decode("{\"sub\":\"y\",\"nbf\":" +
        convert.toString($now + $secondsAhead) + "}");
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

# signWithKid signs like jwt.sign but adds a "kid" to the header, so the token
# selects a key by id. Uses the module's private encodeSegment / computeSig.
func signWithKid(claims as json.Value, key as bytes, alg as string, kid as string) {
    def headerJson as string init "{\"alg\":\"" + $alg + "\",\"typ\":\"JWT\",\"kid\":\"" + $kid +
        "\"}";
    def head as string init encodeSegment(convert.bytesFromString($headerJson, "utf-8"));
    def payload as string init encodeSegment(convert.bytesFromString(json.encode($claims), "utf-8"));
    def signingInput as string init $head + "." + $payload;
    def sig as bytes init computeSig($alg, convert.bytesFromString($signingInput, "utf-8"), $key);
    return $signingInput + "." + encodeSegment($sig);
}
func keyRing() {
    return {"k1": "the-first-secret", "k2": "a-shared-secret-value"};
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
    def withKid as string init "{\"kid\":\"" + $kid + "\"," + strings.substring($jwk, 1, len($jwk));
    return "{\"keys\":[" + $withKid + "]}";
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
    verifyJwks(signWithKid(sampleClaims(), secret(), "HS256", "k1"), "{\"keys\":[]}", "HS256");
}

func testVerifyJwksNoKeysArrayRejected() {
    testing.assertThrows("jwksNoKeys", "value");
}
func jwksNoKeys() {
    def priv as bytes init rsaKey();
    def tok as string init signWithKid(sampleClaims(), $priv, "RS256", "rk1");
    verifyJwks($tok, "{\"notkeys\":1}", "RS256");
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
