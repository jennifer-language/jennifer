# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
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
func signNoneAlg() { sign(sampleClaims(), secret(), "none"); }
func signBogusAlg() { sign(sampleClaims(), secret(), "HS999"); }

func testMalformedTokenRejected() {
    testing.assertThrows("verifyTwoSegments", "value");
    testing.assertThrows("decodeEmpty", "value");
}
func verifyTwoSegments() { verify("a.b", secret(), "HS256"); }
func decodeEmpty() { decode(""); }

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
    def head as string init encodeSegment(convert.bytesFromString("{\"alg\":\"HS256\",\"typ\":\"JWT\",\"crit\":[\"exp\"]}", "utf-8"));
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
    def back as json.Value init verifyWith($tok, secret(), "HS256", Policy{iss: "good-iss", aud: "good-aud"});
    testing.assertEqual(json.asString($back, "/sub"), "ada");
}

func testVerifyWithEmptyPolicyActsLikeVerify() {
    def tok as string init sign(claimsWithIss(), secret(), "HS256");
    def back as json.Value init verifyWith($tok, secret(), "HS256", Policy{iss: "", aud: ""});
    testing.assertEqual(json.asString($back, "/iss"), "good-iss");
}

func testVerifyWithAudienceArrayMatch() {
    def tok as string init sign(claimsWithAudList(), secret(), "HS256");
    def back as json.Value init verifyWith($tok, secret(), "HS256", Policy{iss: "", aud: "good-aud"});
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
