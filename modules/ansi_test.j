# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
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

# Reset the colour-gating env after every test so a forced setting never leaks
# into another test (the framework runs tearDown after each test* method).
func tearDown() {
    os.setEnv("FORCE_COLOR", "");
    os.setEnv("NO_COLOR", "");
}

# With colour forced on, every wrapper emits real SGR escapes (the enabled
# path); strip() recovers the original, so the round-trip is deterministic. The
# shortcuts are exercised as first-class func values in a list.
func testShortcutsRoundTripWithColour() {
    os.setEnv("NO_COLOR", ""); # NO_COLOR is checked first, so clear it
    os.setEnv("FORCE_COLOR", "1");
    def fns as list of func init [
        black,
        red,
        green,
        yellow,
        blue,
        magenta,
        cyan,
        white,
        gray,
        bold,
        dim,
        italic,
        underline,
        reverse,
        strike
    ];
    for (def f in $fns) {
        def wrapped as string init $f("sample");
        testing.assertNotEqual($wrapped, "sample"); # escapes were emitted
        testing.assertEqual(strip($wrapped), "sample");
    }
    testing.assertContains(strike("x"), "9m"); # strikethrough is SGR 9
    testing.assertEqual(strip(bgColor("hi", "green")), "hi");
    # rgb clamps out-of-range channels (exercises clampChannel's low/high/pass branches).
    testing.assertEqual(strip(rgb("hi", -5, 300, 128)), "hi");
    testing.assertEqual(strip(rgb("hi", 0, 255, 64)), "hi");
}

# Colour gating: NO_COLOR and FORCE_COLOR=0 both force the wrappers to no-op.
func testColourGatingDisabled() {
    os.setEnv("NO_COLOR", "1");
    testing.assertEqual(red("x"), "x"); # unchanged when disabled
    os.setEnv("NO_COLOR", "");
    os.setEnv("FORCE_COLOR", "0");
    testing.assertEqual(bold("x"), "x");
    testing.assertEqual(bgColor("x", "red"), "x");
}

# The 256-colour wrappers emit the indexed SGR form and strip round-trips.
func testColor256RoundTrips() {
    os.setEnv("NO_COLOR", "");
    os.setEnv("FORCE_COLOR", "1");
    testing.assertContains(color256("x", 196), "38;5;196m"); # foreground form
    testing.assertContains(bgColor256("x", 21), "48;5;21m"); # background form
    testing.assertEqual(strip(color256("x", 196)), "x");
    testing.assertEqual(strip(bgColor256("hi", 21)), "hi");
}

# An out-of-range palette index clamps to [0, 255] rather than emitting a
# malformed sequence.
func testColor256Clamps() {
    os.setEnv("NO_COLOR", "");
    os.setEnv("FORCE_COLOR", "1");
    testing.assertContains(color256("x", 300), "38;5;255m"); # clamped high
    testing.assertContains(color256("x", -5), "38;5;0m"); # clamped low
}

# rgbToColor256 picks the nearest palette index: cube primaries and corners,
# and the grayscale ramp for a neutral mid-gray.
func testRgbToColor256() {
    testing.assertEqual(rgbToColor256(255, 0, 0), 196); # pure red (cube corner)
    testing.assertEqual(rgbToColor256(0, 0, 0), 16); # black (cube origin)
    testing.assertEqual(rgbToColor256(255, 255, 255), 231); # white (cube corner)
    testing.assertEqual(rgbToColor256(128, 128, 128), 244); # mid-gray (ramp beats cube)
    # Out-of-range channels clamp to (255, 0, 255) = magenta before quantizing.
    testing.assertEqual(rgbToColor256(300, -10, 999), 201);
}
