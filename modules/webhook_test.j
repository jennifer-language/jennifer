# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# webhook_test.j - white-box tests for webhook.j's pure sign / verify. Run with:
#
#     jennifer test modules/webhook_test.j
#
# The overlay splices webhook.j in front, so these tests reach its private
# helpers (hexMac, equalConstantTime) and the exported sign / verify by bare
# identifier. The networked `send` (the signed POST) is verified against an
# in-process HTTP server in the Go suite (TestWebhookSend). webhook.j already
# `use`s hash / encoding / convert, so the overlay only adds testing / strings.
use testing;
use strings;

# The signature is GitHub's documented example (secret / payload / expected).
def const SECRET as string init "It's a Secret to Everybody";
def const PAYLOAD as string init "Hello, World!";
def const SIG as string init "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17";

func testSignVector() {
    testing.assertEqual(sign(PAYLOAD, SECRET), SIG);
}

func testHexMac() {
    # hexMac is the bare hex, no sha256= prefix.
    testing.assertEqual(
        hexMac(PAYLOAD, SECRET),
        "757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17");
}

func testVerifyValid() {
    testing.assertTrue(verify(PAYLOAD, SIG, SECRET));
}

func testVerifyCaseInsensitive() {
    # The hex digest and `sha256=` prefix are case-insensitive.
    testing.assertTrue(verify(PAYLOAD, strings.upper(SIG), SECRET));
}

func testVerifyWrongSecret() {
    testing.assertFalse(verify(PAYLOAD, SIG, "wrong"));
}

func testVerifyTamperedPayload() {
    testing.assertFalse(verify("Hello, World?", SIG, SECRET));
}

func testVerifyMalformedSignature() {
    testing.assertFalse(verify(PAYLOAD, "sha256=deadbeef", SECRET)); # wrong length
    testing.assertFalse(verify(PAYLOAD, "", SECRET)); # empty
    testing.assertFalse(verify(PAYLOAD, "notaprefix", SECRET)); # no sha256=
}

func testEqualConstantTime() {
    testing.assertTrue(equalConstantTime("abc", "abc"));
    testing.assertFalse(equalConstantTime("abc", "abd")); # same length, differs
    testing.assertFalse(equalConstantTime("abc", "ab")); # different length
    testing.assertTrue(equalConstantTime("", ""));
}

func testSignRoundTrip() {
    def s as string init sign("{\"event\":\"push\"}", "k3y");
    testing.assertTrue(strings.startsWith($s, "sha256="));
    testing.assertTrue(verify("{\"event\":\"push\"}", $s, "k3y"));
    testing.assertFalse(verify("{\"event\":\"pull\"}", $s, "k3y"));
}

# --- timestamped, replay-protected schemes ----------------------------------

def const TS as int init 1492774800; # a fixed unix timestamp for signing
def const NOW as int init 1492774830; # 30s later: within a 300s tolerance
def const OLD as int init 1492780800; # now 100 minutes later: beyond tolerance
def const BODY as string init "{\"event\":\"payment\"}";
def const TOL as int init 300;

func testStripeRoundTrip() {
    def h as string init stripeSign(SECRET, BODY, TS);
    testing.assertTrue(strings.startsWith($h, "t=1492774800,v1="));
    testing.assertTrue(stripeVerify(SECRET, BODY, $h, TOL, NOW));
}

func testStripeTamperedBody() {
    def h as string init stripeSign(SECRET, BODY, TS);
    testing.assertFalse(stripeVerify(SECRET, "{\"event\":\"refund\"}", $h, TOL, NOW));
}

func testStripeWrongSecret() {
    def h as string init stripeSign(SECRET, BODY, TS);
    testing.assertFalse(stripeVerify("wrong", BODY, $h, TOL, NOW));
}

func testStripeReplayRejected() {
    # A correct signature whose timestamp is beyond tolerance is still rejected.
    def h as string init stripeSign(SECRET, BODY, TS);
    testing.assertFalse(stripeVerify(SECRET, BODY, $h, TOL, OLD));
}

func testStripeMultipleV1() {
    # Extra v1= entries: valid if any matches; malformed / missing parts fail.
    def h as string init stripeSign(SECRET, BODY, TS);
    testing.assertTrue(stripeVerify(SECRET, BODY, $h + ",v1=deadbeef", TOL, NOW));
    testing.assertFalse(stripeVerify(SECRET, BODY, "v1=deadbeef", TOL, NOW)); # no t=
    testing.assertFalse(stripeVerify(SECRET, BODY, "t=1492774800", TOL, NOW)); # no v1=
}

func testSlackRoundTrip() {
    def sig as string init slackSign(SECRET, BODY, TS);
    testing.assertTrue(strings.startsWith($sig, "v0="));
    testing.assertTrue(slackVerify(SECRET, BODY, TS, $sig, TOL, NOW));
}

func testSlackTamperedBody() {
    def sig as string init slackSign(SECRET, BODY, TS);
    testing.assertFalse(slackVerify(SECRET, "{\"event\":\"refund\"}", TS, $sig, TOL, NOW));
}

func testSlackReplayRejected() {
    def sig as string init slackSign(SECRET, BODY, TS);
    testing.assertFalse(slackVerify(SECRET, BODY, TS, $sig, TOL, OLD));
}

func testGenericSha1Hex() {
    def sig as string init timestampedSign(SECRET, BODY, TS, "sha1", "hex");
    testing.assertTrue(timestampedVerify(SECRET, BODY, TS, $sig, "sha1", "hex", TOL, NOW));
    testing.assertFalse(timestampedVerify(SECRET, "x", TS, $sig, "sha1", "hex", TOL, NOW));
    testing.assertFalse(timestampedVerify(SECRET, BODY, TS, $sig, "sha1", "hex", TOL, OLD));
}

func testGenericSha256Base64() {
    def sig as string init timestampedSign(SECRET, BODY, TS, "sha256", "base64");
    testing.assertTrue(timestampedVerify(SECRET, BODY, TS, $sig, "sha256", "base64", TOL, NOW));
    testing.assertFalse(timestampedVerify(SECRET, "x", TS, $sig, "sha256", "base64", TOL, NOW));
    testing.assertFalse(timestampedVerify(SECRET, BODY, TS, $sig, "sha256", "base64", TOL, OLD));
}

func testFreshHelper() {
    testing.assertTrue(fresh(NOW, TS, TOL)); # 30s apart, within 300s
    testing.assertFalse(fresh(OLD, TS, TOL)); # far apart
    testing.assertTrue(fresh(TS, NOW, TOL)); # symmetric (future skew)
    testing.assertTrue(absInt(-5) == 5);
    testing.assertTrue(absInt(5) == 5);
}
