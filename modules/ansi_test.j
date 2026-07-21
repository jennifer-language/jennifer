# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# ansi_test.j - white-box tests for ansi.j. Run with:
#
#     jennifer test modules/ansi_test.j
#
# The overlay splices ansi.j in front of this file, so the tests reach its
# private helpers (makeEsc, lookup) and private code tables (ESC, FG, BG,
# STYLE) by bare identifier, alongside its exported surface.
use testing;

# ESC is the single escape byte ansi.j builds privately from a `bytes`.
func testEscIsOneByte() {
    testing.assertEqual(len(ESC), 1);
}

# The private code tables + private lookup, reached white-box.
func testSgrCodes() {
    testing.assertEqual(lookup(FG, "red", "colour"), "31");
    testing.assertEqual(lookup(FG, "cyan", "colour"), "36");
    testing.assertEqual(lookup(BG, "green", "background"), "42");
    testing.assertEqual(lookup(STYLE, "bold", "style"), "1");
}

# strip inverts the wrappers whether or not colour is currently enabled, so
# this round-trip is deterministic regardless of the test's TTY.
func testStripRoundTrips() {
    testing.assertEqual(strip(color("x", "red")), "x");
    testing.assertEqual(strip(bold(underline("hi"))), "hi");
}

# An unknown colour name throws before any TTY gating (lookup runs first).
func badColour() {
    return color("x", "chartreuse");
}
func testUnknownColourThrows() {
    testing.assertThrows("badColour", "value");
}
