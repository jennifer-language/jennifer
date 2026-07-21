# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# sasl_test.j - white-box tests for sasl.j. Run with:
#
#     jennifer test modules/sasl_test.j
#
# The overlay splices sasl.j in front of this file, so the tests reach its
# private helpers (baseEncode, ctrlA) by bare identifier alongside the exported
# encoders. Reference base64 values were computed independently.
use testing;

func testBaseEncode() {
    testing.assertEqual(baseEncode("hello"), "aGVsbG8=");
    testing.assertEqual(baseEncode(""), "");
}

func testCtrlAIsOneByte() {
    testing.assertEqual(len(ctrlA()), 1);
}

func testPlain() {
    # base64("\0user\0pass")
    testing.assertEqual(plain("user", "pass"), "AHVzZXIAcGFzcw==");
}

func testLoginSteps() {
    testing.assertEqual(loginUser("user"), "dXNlcg==");
    testing.assertEqual(loginPass("pass"), "cGFzcw==");
}

func testBearer() {
    # SASL XOAUTH2: base64("user=...\x01auth=Bearer ...\x01\x01")
    testing.assertEqual(bearer("me@gmail.com", "ya29.TOKEN"),
        "dXNlcj1tZUBnbWFpbC5jb20BYXV0aD1CZWFyZXIgeWEyOS5UT0tFTgEB");
}

# negotiate picks the strongest supported password mechanism, case-insensitively,
# and never auto-selects XOAUTH2 (a token, not a password).
func testNegotiate() {
    testing.assertEqual(negotiate(["PLAIN", "LOGIN", "CRAM-MD5", "SCRAM-SHA-1", "SCRAM-SHA-256"]), "scram-sha-256");
    testing.assertEqual(negotiate(["cram-md5", "scram-sha-1"]), "scram-sha-1");
    testing.assertEqual(negotiate(["PLAIN", "CRAM-MD5"]), "cram");
    testing.assertEqual(negotiate(["PLAIN", "LOGIN"]), "");
    testing.assertEqual(negotiate([]), "");
    # XOAUTH2 is advertised but must not be auto-picked (needs a token).
    testing.assertEqual(negotiate(["PLAIN", "XOAUTH2"]), "");
}

# CRAM-MD5, RFC 2195 worked example: user "tim", password "tanstaaftanstaaf",
# the challenge below -> "tim b913a602c7eda7a495b4e6e7334d3890".
func testCramVector() {
    def challenge as string init baseEncode("<1896.697170952@postoffice.reston.mci.net>");
    def resp as string init baseDecode(cram("tim", "tanstaaftanstaaf", $challenge));
    testing.assertEqual($resp, "tim b913a602c7eda7a495b4e6e7334d3890");
}

# SCRAM-SHA-1, the RFC 5802 section 5 worked example: user "user", password
# "pencil", the client nonce pinned so the exact wire bytes are checked.
func testScramShaOne() {
    def st as Scram init scramStartNonce("user", "sha1", "fyko+d2lbbFgONRv9qkxdawL");
    testing.assertEqual(baseDecode(scramClientFirst($st)),
        "n,,n=user,r=fyko+d2lbbFgONRv9qkxdawL");
    def serverFirst as string init baseEncode(
        "r=fyko+d2lbbFgONRv9qkxdawL3rfcNHYJY1ZVvWVs7j,s=QSXCR+Q6sek8bf92,i=4096");
    $st = scramClientFinal($st, $serverFirst, "pencil");
    testing.assertEqual($st.clientFinal,
        "c=biws,r=fyko+d2lbbFgONRv9qkxdawL3rfcNHYJY1ZVvWVs7j,p=v0X8v3Bz2T0CJGbJQyF0X+HI4Ts=");
    # The server proves it too: v= must verify, a wrong one must not.
    testing.assertTrue(scramVerify($st, baseEncode("v=rmF9pqV8S7suAoZWja4dJRkFsKQ=")));
    testing.assertFalse(scramVerify($st, baseEncode("v=AAAAAAAAAAAAAAAAAAAAAAAAAAA=")));
}

# SCRAM-SHA-256, the RFC 7677 section 3 worked example (same user/password).
func testScramShaTwoFiftySix() {
    def st as Scram init scramStartNonce("user", "sha256", "rOprNGfwEbeRWgbNEkqO");
    def serverFirst as string init baseEncode(
        "r=rOprNGfwEbeRWgbNEkqO%hvYDpWUa2RaTCAfuxFIlj)hNlF$k0,s=W22ZaJ0SNY7soEsUEjb6gQ==,i=4096");
    $st = scramClientFinal($st, $serverFirst, "pencil");
    testing.assertEqual($st.clientFinal,
        "c=biws,r=rOprNGfwEbeRWgbNEkqO%hvYDpWUa2RaTCAfuxFIlj)hNlF$k0,p=dHzbZapWIk4jUhN+Ute9ytag9zjfMHgsqmmiz7AndVQ=");
    testing.assertTrue(scramVerify($st, baseEncode("v=6rriTRBi23WpRR/wtup+mMhUZUn/dB5nLTJRsjl95G4=")));
}

# A server nonce that does not extend the client nonce is a MITM signal.
func testScramRejectsBadServerNonce() {
    def st as Scram init scramStartNonce("user", "sha256", "clientNonceXYZ");
    def threw as bool init false;
    try {
        scramClientFinal($st, baseEncode("r=totallyDifferent,s=W22ZaJ0SNY7soEsUEjb6gQ==,i=4096"), "pencil");
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "sasl");
    }
    testing.assertTrue($threw);
}
