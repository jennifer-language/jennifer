# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# totp_test.j - white-box tests for totp.j. Run with:
#
#     jennifer test modules/totp_test.j
#
# The overlay splices totp.j in first, so these tests reach its private helpers
# (hotp, decodeSecret) and the exported surface by bare identifier.
# totp.j already `use`s hash / encoding / time / strings / convert, so the
# overlay only adds testing. The generateAt vectors are RFC 6238 Appendix B.
use testing;

# seed base32-encodes an ASCII seed the way RFC 6238 Appendix B specifies them.
func seed(ascii as string) {
    return encoding.toText(convert.bytesFromString($ascii, "utf-8"), "base32");
}

# eight returns an 8-digit Options (Appendix B prints 8-digit codes).
func eight() {
    def o as Options;
    $o.digits = 8;
    return $o;
}

# A verify window past the cap is rejected typed (CPU-DoS guard) - fires before
# the secret is even decoded.
func testVerifyWindowClamped() {
    def o as Options;
    def threw as bool init false;
    try {
        verifyWindowAt("AAAA", "000000", 0, 1000000, $o);
    } catch (e) {
        $threw = true;
    }
    testing.assertTrue($threw);
}

# A code length outside [6, 8] is rejected (non-standard + powTen overflow guard).
func testBadDigitsRejected() {
    def o as Options;
    $o.digits = 20;
    def threw as bool init false;
    try {
        digitsOf($o);
    } catch (e) {
        $threw = true;
    }
    testing.assertTrue($threw);
}

func testRfcSha() {
    def s as string init seed("12345678901234567890");
    def o as Options init eight();
    testing.assertEqual(generateAt($s, 59, $o), "94287082");
    testing.assertEqual(generateAt($s, 1111111109, $o), "07081804");
    testing.assertEqual(generateAt($s, 1234567890, $o), "89005924");
    testing.assertEqual(generateAt($s, 2000000000, $o), "69279037");
    testing.assertEqual(generateAt($s, 20000000000, $o), "65353130");
}

func testRfcShaTwoFiftySix() {
    def s as string init seed("12345678901234567890123456789012");
    def o as Options init eight();
    $o.algorithm = Algorithm.Sha256;
    testing.assertEqual(generateAt($s, 59, $o), "46119246");
    testing.assertEqual(generateAt($s, 1111111109, $o), "68084774");
    testing.assertEqual(generateAt($s, 20000000000, $o), "77737706");
}

func testRfcShaFiveTwelve() {
    def s as string init seed("1234567890123456789012345678901234567890123456789012345678901234");
    def o as Options init eight();
    $o.algorithm = Algorithm.Sha512;
    testing.assertEqual(generateAt($s, 59, $o), "90693936");
    testing.assertEqual(generateAt($s, 1111111111, $o), "99943326");
    testing.assertEqual(generateAt($s, 20000000000, $o), "47863826");
}

func testSixDigitDefault() {
    def s as string init seed("12345678901234567890");
    def o as Options; # zero-value: 6 digits, sha1
    testing.assertEqual(generateAt($s, 59, $o), "287082"); # last 6 of 94287082
}

func testVerifySkewWindow() {
    def s as string init seed("12345678901234567890");
    def o as Options;
    def code as string init generateAt($s, 59, $o); # code for step floor(59/30)=1
    testing.assertTrue(verifyAt($s, $code, 59, $o)); # same step
    testing.assertTrue(verifyAt($s, $code, 75, $o)); # next step, within +1
    testing.assertTrue(verifyAt($s, $code, 29, $o)); # previous step, within -1
    testing.assertFalse(verifyAt($s, $code, 120, $o)); # two steps away, outside window
    testing.assertFalse(verifyAt($s, "000000", 59, $o)); # wrong code
}

func testSecretNormalization() {
    def o as Options;
    def canon as string init generateAt("JBSWY3DPEHPK3PXP", 59, $o);
    testing.assertEqual(generateAt("jbswy3dpehpk3pxp", 59, $o), $canon); # lowercase
    testing.assertEqual(generateAt("JBSW Y3DP EHPK 3PXP", 59, $o), $canon); # spaced
}

func testHotpRfc4226Vectors() {
    # RFC 4226 Appendix D: ASCII secret "12345678901234567890", counters 0..9.
    def s as string init seed("12345678901234567890");
    testing.assertEqual(hotp($s, 0), "755224");
    testing.assertEqual(hotp($s, 1), "287082");
    testing.assertEqual(hotp($s, 2), "359152");
    testing.assertEqual(hotp($s, 3), "969429");
    testing.assertEqual(hotp($s, 4), "338314");
    testing.assertEqual(hotp($s, 5), "254676");
    testing.assertEqual(hotp($s, 6), "287922");
    testing.assertEqual(hotp($s, 7), "162583");
    testing.assertEqual(hotp($s, 8), "399871");
    testing.assertEqual(hotp($s, 9), "520489");
}

func testHotpIsTotpBuildingBlock() {
    # TOTP is HOTP over floor(unixSeconds / period): at t=59, step = 1.
    def s as string init seed("12345678901234567890");
    def o as Options; # 6 digits, 30 s, sha1
    testing.assertEqual(generateAt($s, 59, $o), hotp($s, 1));
}

func testGenerateSecretRoundTrips() {
    def sec as string init generateSecret();
    # 20 random bytes -> 160 bits -> 32 base32 chars, unpadded.
    testing.assertEqual(len($sec), 32);
    testing.assertFalse(strings.contains($sec, "="));
    def o as Options;
    def code as string init generateAt($sec, 1000000000, $o);
    testing.assertTrue(verifyAt($sec, $code, 1000000000, $o));
}

func testGenerateSecretNLength() {
    def sec as string init generateSecretN(10); # 80 bits -> 16 base32 chars
    testing.assertEqual(len($sec), 16);
    def o as Options;
    def code as string init generateAt($sec, 500, $o);
    testing.assertTrue(verifyAt($sec, $code, 500, $o));
}

func secretZeroBytes() {
    return generateSecretN(0);
}

func testGenerateSecretNRejectsNonPositive() {
    testing.assertThrows("secretZeroBytes", "totp");
}

func testVerifyWindowAccepts() {
    def s as string init seed("12345678901234567890");
    def o as Options;
    def prev as string init generateAt($s, 29, $o); # step floor(29/30)=0
    # now is step floor(120/30)=4, three steps away: a window of 3 reaches it.
    testing.assertFalse(verifyWindowAt($s, $prev, 120, 3, $o));
    testing.assertTrue(verifyWindowAt($s, $prev, 120, 4, $o));
}

func testVerifyWindowRejectsOutside() {
    def s as string init seed("12345678901234567890");
    def o as Options;
    def code as string init generateAt($s, 59, $o); # step 1
    # window 0 only checks the current step: same time passes, adjacent fails.
    testing.assertTrue(verifyWindowAt($s, $code, 59, 0, $o));
    testing.assertFalse(verifyWindowAt($s, $code, 29, 0, $o)); # step 0, outside window 0
    testing.assertTrue(verifyWindowAt($s, $code, 29, 1, $o)); # step 0, within window 1
}

func testCounterBytesBigEndian() {
    def b as bytes init counterBytes(1);
    testing.assertEqual(len($b), 8);
    testing.assertEqual($b[7], 1);
    testing.assertEqual($b[0], 0);
}

func testUriDefault() {
    def o as Options;
    testing.assertEqual(
        uri("ACME", "jane@acme.example", "JBSWY3DPEHPK3PXP", $o),
        "otpauth://totp/ACME:jane%40acme.example?secret=JBSWY3DPEHPK3PXP&issuer=ACME&algorithm=SHA1&digits=6&period=30");
}

func testUriCustom() {
    def o as Options;
    $o.digits = 8;
    $o.period = 60;
    $o.algorithm = Algorithm.Sha256;
    testing.assertEqual(
        uri("A B", "u", "S", $o),
        "otpauth://totp/A%20B:u?secret=S&issuer=A%20B&algorithm=SHA256&digits=8&period=60");
}
