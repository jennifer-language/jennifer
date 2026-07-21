# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
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
    testing.assertEqual(hexMac(PAYLOAD, SECRET),
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
    testing.assertFalse(verify(PAYLOAD, "sha256=deadbeef", SECRET));   # wrong length
    testing.assertFalse(verify(PAYLOAD, "", SECRET));                  # empty
    testing.assertFalse(verify(PAYLOAD, "notaprefix", SECRET));        # no sha256=
}

func testEqualConstantTime() {
    testing.assertTrue(equalConstantTime("abc", "abc"));
    testing.assertFalse(equalConstantTime("abc", "abd"));   # same length, differs
    testing.assertFalse(equalConstantTime("abc", "ab"));    # different length
    testing.assertTrue(equalConstantTime("", ""));
}

func testSignRoundTrip() {
    def s as string init sign("{\"event\":\"push\"}", "k3y");
    testing.assertTrue(strings.startsWith($s, "sha256="));
    testing.assertTrue(verify("{\"event\":\"push\"}", $s, "k3y"));
    testing.assertFalse(verify("{\"event\":\"pull\"}", $s, "k3y"));
}
