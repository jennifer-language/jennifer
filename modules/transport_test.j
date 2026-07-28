# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# transport_test.j - white-box tests for transport.j. Run with:
#
#     jennifer test modules/transport_test.j
#
# The overlay splices transport.j in front, so the `Security` enum and its
# variants are reachable by bare name.
use testing;

func testEncrypted() {
    testing.assertFalse(encrypted(Security.None));
    testing.assertTrue(encrypted(Security.Tls));
    testing.assertTrue(encrypted(Security.Starttls));
}

func testEquality() {
    def s as Security init Security.Tls;
    testing.assertTrue($s == Security.Tls);
    testing.assertFalse($s == Security.None);
    testing.assertFalse($s == Security.Starttls);
}

# The zero value of an enum is its first declared variant (None = plaintext).
func testZeroIsNone() {
    def s as Security;
    testing.assertTrue($s == Security.None);
}

# A match over a Security parameter covers every variant.
func modeName(s as Security) {
    match ($s) {
        when None { return "none"; }
        when Tls { return "tls"; }
        when Starttls { return "starttls"; }
    }
    return "?";
}

func testMatchDispatch() {
    testing.assertEqual(modeName(Security.None), "none");
    testing.assertEqual(modeName(Security.Tls), "tls");
    testing.assertEqual(modeName(Security.Starttls), "starttls");
}
