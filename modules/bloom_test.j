# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# bloom_test.j - white-box tests for bloom.j. Run with:
#
#     jennifer test modules/bloom_test.j
#
# Pure probabilistic-set behaviour, no network. Non-membership results are
# deterministic here because SHA-256 and the filter parameters are fixed.
# bloom.j already `use`s hash / convert / lists, so the overlay only adds testing.
use testing;

func testNoFalseNegatives() {
    # Every added item must report present (the Bloom guarantee).
    def f as Filter init new(2048, 5);
    def items as list of string init ["alice", "bob", "carol", "dave", "eve", "frank"];
    $f = addAll($f, $items);
    for (def item in $items) {
        testing.assertTrue(mightContain($f, $item));
    }
}

func testAbsentReportFalse() {
    def f as Filter init addAll(new(2048, 5), ["alice", "bob", "carol"]);
    # Confirmed non-colliding non-members at these parameters.
    testing.assertTrue(not mightContain($f, "dave"));
    testing.assertTrue(not mightContain($f, "eve"));
    testing.assertTrue(mightContain($f, "alice"));
}

func testEmptyFilterAllAbsent() {
    def f as Filter init new(1024, 4);
    testing.assertTrue(not mightContain($f, "anything"));
    testing.assertTrue(not mightContain($f, ""));
}

func testAddIsValueSemantic() {
    def base as Filter init new(1024, 4);
    def withX as Filter init add($base, "x");
    testing.assertTrue(mightContain($withX, "x"));
    # the original filter is unchanged (add returned a fresh copy)
    testing.assertTrue(not mightContain($base, "x"));
}

func testFilterShape() {
    def f as Filter init new(1000, 3);
    testing.assertEqual($f.size, 1000);
    testing.assertEqual($f.hashes, 3);
    # 1000 bits pack into ceil(1000/8) = 125 bytes
    testing.assertEqual(len($f.bits), 125);
}

func testInvalidParamsThrow() {
    def threw as bool init false;
    try {
        new(0, 3);
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "bloom");
    }
    testing.assertTrue($threw);
    def threwHashes as bool init false;
    try {
        new(64, 0);
    } catch (e) {
        $threwHashes = true;
    }
    testing.assertTrue($threwHashes);
}
