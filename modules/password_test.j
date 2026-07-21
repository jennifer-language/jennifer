# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# password_test.j - white-box tests for password.j. Run with:
#
#     jennifer test modules/password_test.j
#
# Everything here is offline: generation is exercised as a property
# (generate(schema) conforms to schema, has the right length, and meets the
# minimums) over many crypto-random draws - the invariant holds every time, so
# no seed is needed (crypto randomness is not seedable); validate, complexity,
# and binaryLog are pure. password.j already `use`s crypto / strings / convert,
# so the overlay only adds testing.
use testing;

func testBinaryLogExactPowersOfTwo() {
    testing.assertEqual(binaryLog(1.0), 0.0);
    testing.assertEqual(binaryLog(2.0), 1.0);
    testing.assertEqual(binaryLog(4.0), 2.0);
    testing.assertEqual(binaryLog(8.0), 3.0);
    testing.assertEqual(binaryLog(1024.0), 10.0);
}

func testDefaultGenerateConforms() {
    def s as Schema init schema();
    def i as int init 0;
    while ($i < 50) {
        def pw as string init generate($s);
        testing.assertEqual(len($pw), 16);
        def r as Report init validate($s, $pw);
        testing.assertTrue($r.valid);
        $i = $i + 1;
    }
}

func testGenerateVariableLengthInRange() {
    def s as Schema init withLength(schema(), 12, 20);
    def i as int init 0;
    while ($i < 50) {
        def pw as string init generate($s);
        testing.assertTrue(len($pw) >= 12);
        testing.assertTrue(len($pw) <= 20);
        $i = $i + 1;
    }
}

func testGenerateNoSymbolsHasNoSymbols() {
    # disable symbols; the leftover minSymbols from the default must be ignored
    def s as Schema init withClasses(withLength(schema(), 24, 24), true, true, true, false);
    def i as int init 0;
    while ($i < 30) {
        def pw as string init generate($s);
        def c as Strength init complexity($pw);
        # only lower/upper/digit classes -> pool 62, no symbols counted
        testing.assertEqual($c.poolSize, 62);
        $i = $i + 1;
    }
}

func testGenerateExcludeAmbiguous() {
    def s as Schema init withoutAmbiguous(withLength(schema(), 40, 40));
    def i as int init 0;
    while ($i < 30) {
        def pw as string init generate($s);
        testing.assertEqual(countIn(AMBIGUOUS, $pw), 0);
        $i = $i + 1;
    }
}

func testGenerateInfeasibleThrows() {
    # minimums (4) exceed the length (2)
    def s as Schema init withLength(schema(), 2, 2);
    def threw as bool init false;
    try {
        generate($s);
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "password");
    }
    testing.assertTrue($threw);
}

func testValidateTooShortAndMinimums() {
    def s as Schema init schema();
    def r as Report init validate($s, "abc");
    testing.assertTrue(not $r.valid);
    # "abc": too short, plus missing upper/digits/symbols = 4 reasons
    testing.assertEqual(len($r.reasons), 4);
}

func testValidatePassesCompliant() {
    def s as Schema init withLength(schema(), 8, 32);
    def r as Report init validate($s, "Abcdef1!");
    testing.assertTrue($r.valid);
    testing.assertEqual(len($r.reasons), 0);
}

func testValidateTooLong() {
    def s as Schema init withLength(schema(), 4, 6);
    def r as Report init validate($s, "Abcdefghij1!");
    testing.assertTrue(not $r.valid);
}

func testComplexityClassesAndPool() {
    def c as Strength init complexity("Abc123!@");
    testing.assertEqual($c.length, 8);
    testing.assertEqual($c.classes, 4);
    # 26 + 26 + 10 + len(SYMBOLS)
    testing.assertEqual($c.poolSize, 62 + len(SYMBOLS));
}

func testComplexityBands() {
    testing.assertEqual(complexity("aaaa").label, "very weak");
    testing.assertEqual(complexity("").label, "very weak");
    # 20 chars over the full 90-symbol pool is well past 128 bits
    testing.assertEqual(complexity("Abc123!@Xyz789#$Klm4").label, "very strong");
}

func testComplexityEmptyIsZero() {
    def c as Strength init complexity("");
    testing.assertEqual($c.length, 0);
    testing.assertEqual($c.classes, 0);
    testing.assertEqual($c.poolSize, 0);
    testing.assertEqual($c.entropy, 0.0);
}
