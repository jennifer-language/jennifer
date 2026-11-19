# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# ansi_test.j - white-box tests for ansi.j. Run with:
#
#     jennifer test modules/ansi_test.j
#
# The overlay splices ansi.j in front of this file, so the tests reach its
# private helper (lookup) and private code tables (ESC, FG, BG,
# STYLE) by bare identifier, alongside its exported surface.
use testing;

# ESC is the single private escape byte ansi.j defines.
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
# into another test (the framework runs tearDown after each test* function).
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

# An out-of-range palette index or channel throws kind "value" (it used to be
# clamped to 0-255 silently), with colour on and off alike.
func color256High() {
    color256("x", 256);
}
func color256Low() {
    color256("x", -1);
}
func bgColor256High() {
    bgColor256("x", 300);
}
func rgbLow() {
    rgb("x", -5, 0, 0);
}
func rgbHigh() {
    rgb("x", 0, 0, 256);
}
func rgbToColor256High() {
    rgbToColor256(300, 0, 0);
}
func rgbToColor256Low() {
    rgbToColor256(0, -10, 0);
}
func assertOutOfRangeThrows() {
    testing.assertThrows("color256High", "value");
    testing.assertThrows("color256Low", "value");
    testing.assertThrows("bgColor256High", "value");
    testing.assertThrows("rgbLow", "value");
    testing.assertThrows("rgbHigh", "value");
    testing.assertThrows("rgbToColor256High", "value");
    testing.assertThrows("rgbToColor256Low", "value");
}
func testOutOfRangeThrows() {
    os.setEnv("NO_COLOR", "");
    os.setEnv("FORCE_COLOR", "1");
    assertOutOfRangeThrows();
    os.setEnv("FORCE_COLOR", "0");
    assertOutOfRangeThrows();
    os.setEnv("FORCE_COLOR", "1");
    # the bounds themselves are valid
    testing.assertContains(color256("x", 255), "38;5;255m");
    testing.assertContains(bgColor256("x", 0), "48;5;0m");
    testing.assertContains(rgb("x", 0, 255, 0), "38;2;0;255;0m");
}

# rgbToColor256 picks the nearest palette index: cube primaries and corners,
# and the grayscale ramp for a neutral mid-gray.
func testRgbToColor256() {
    testing.assertEqual(rgbToColor256(255, 0, 0), 196); # pure red (cube corner)
    testing.assertEqual(rgbToColor256(0, 0, 0), 16); # black (cube origin)
    testing.assertEqual(rgbToColor256(255, 255, 255), 231); # white (cube corner)
    testing.assertEqual(rgbToColor256(128, 128, 128), 244); # mid-gray (ramp beats cube)
    testing.assertEqual(rgbToColor256(255, 0, 255), 201); # magenta (cube corner)
}

# Each wrapper resets only its own attribute, so an inner wrapper does not end
# an outer one: the text after red() is still bold.
func testWrappersNest() {
    os.setEnv("NO_COLOR", "");
    os.setEnv("FORCE_COLOR", "1");
    def e as string init ESC;
    testing.assertEqual(
        bold("a " + red("b") + " c"),
        $e + "[1ma " + $e + "[31mb" + $e + "[39m c" + $e + "[22m");
    testing.assertEqual(bgColor("x", "blue"), $e + "[44mx" + $e + "[49m");
    testing.assertEqual(italic("x"), $e + "[3mx" + $e + "[23m");
    testing.assertEqual(color256("x", 200), $e + "[38;5;200mx" + $e + "[39m");
    testing.assertEqual(bgColor256("x", 17), $e + "[48;5;17mx" + $e + "[49m");
}

# strip removes every escape sequence, not only SGR: CSI (7- and 8-bit), OSC
# ended by BEL or ST, DCS / APC strings, and two-byte ESC sequences.
func testStripRemovesControlSequences() {
    testing.assertEqual(strip("a\u001b]0;pwned\u0007b"), "ab");
    testing.assertEqual(strip("a\u001b]52;c;Zm9v\u001b\\b"), "ab");
    testing.assertEqual(strip("a\u001b]8;;http://x\u001b\\link\u001b]8;;\u001b\\b"), "alinkb");
    testing.assertEqual(strip("a\u001bPq#0\u001b\\b"), "ab");
    testing.assertEqual(strip("a\u001b_x\u009cb"), "ab");
    testing.assertEqual(strip("a\u001b[2J\u001b[1;1Hb"), "ab");
    testing.assertEqual(strip("a\u009b31mb\u009d0;t\u0007c"), "abc");
    testing.assertEqual(strip("a\u001bcb\u001b7c"), "abc");
    testing.assertEqual(strip("tab\tand\nnewline"), "tab\tand\nnewline");
}
