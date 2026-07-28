# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
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

func testOptimalSizing() {
    # Size for 500 elements at a 1% target false-positive rate.
    def f as Filter init optimal(500, 0.01);
    # The chosen shape should be far larger than the element count and use a
    # handful of hash functions.
    testing.assertTrue($f.size > 4000);
    testing.assertTrue($f.hashes >= 4);
    # Load it with 500 members.
    def items as list of string init [];
    def i as int init 0;
    while ($i < 500) {
        $items[] = "item" + convert.toString($i);
        $i = $i + 1;
    }
    $f = addAll($f, $items);
    # No false negatives: every member reports present.
    $i = 0;
    while ($i < 500) {
        testing.assertTrue(mightContain($f, "item" + convert.toString($i)));
        $i = $i + 1;
    }
    # Probe 500 non-members; the observed false-positive count should stay well
    # under 5% (the 1% target has plenty of headroom). Deterministic: SHA-256
    # and the derived filter parameters are fixed.
    def fp as int init 0;
    $i = 0;
    while ($i < 500) {
        if (mightContain($f, "absent" + convert.toString($i))) {
            $fp = $fp + 1;
        }
        $i = $i + 1;
    }
    testing.assertTrue($fp < 25);
}

func testOptimalInvalidThrows() {
    def t1 as bool init false;
    try {
        optimal(0, 0.01);
    } catch (e) {
        $t1 = true;
        testing.assertEqual($e.kind, "bloom");
    }
    testing.assertTrue($t1);
    def t2 as bool init false;
    try {
        optimal(100, 0.0);
    } catch (e) {
        $t2 = true;
    }
    testing.assertTrue($t2);
    def t3 as bool init false;
    try {
        optimal(100, 1.0);
    } catch (e) {
        $t3 = true;
    }
    testing.assertTrue($t3);
}

func testSerializeRoundTrip() {
    def f as Filter init addAll(new(2048, 5), ["alice", "bob", "carol", "dave"]);
    def enc as bytes init serialize($f);
    def g as Filter init deserialize($enc);
    # Reconstructed shape is identical.
    testing.assertEqual($g.size, $f.size);
    testing.assertEqual($g.hashes, $f.hashes);
    testing.assertEqual(len($g.bits), len($f.bits));
    # Membership is preserved.
    testing.assertTrue(mightContain($g, "alice"));
    testing.assertTrue(mightContain($g, "dave"));
    # Re-encoding yields byte-identical output (proves an exact round-trip).
    def enc2 as bytes init serialize($g);
    testing.assertEqual(len($enc2), len($enc));
    def i as int init 0;
    while ($i < len($enc)) {
        testing.assertEqual($enc2[$i], $enc[$i]);
        $i = $i + 1;
    }
}

func testDeserializeRejectsGarbage() {
    def threw as bool init false;
    def junk as bytes;
    $junk[] = 1;
    $junk[] = 2;
    try {
        deserialize($junk);
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "bloom");
    }
    testing.assertTrue($threw);
}

func testUnion() {
    def a as Filter init addAll(new(2048, 5), ["alice", "bob"]);
    def b as Filter init addAll(new(2048, 5), ["carol", "dave"]);
    # Snapshot a before the union to prove it is left unmodified.
    def aBefore as bytes init serialize($a);
    def u as Filter init union($a, $b);
    # The union holds the members of both inputs.
    testing.assertTrue(mightContain($u, "alice"));
    testing.assertTrue(mightContain($u, "bob"));
    testing.assertTrue(mightContain($u, "carol"));
    testing.assertTrue(mightContain($u, "dave"));
    # union is value-semantic: input a is unchanged.
    def aAfter as bytes init serialize($a);
    testing.assertEqual(len($aAfter), len($aBefore));
    def i as int init 0;
    while ($i < len($aBefore)) {
        testing.assertEqual($aAfter[$i], $aBefore[$i]);
        $i = $i + 1;
    }
    # merge is an alias for union.
    def m as Filter init merge($a, $b);
    testing.assertTrue(mightContain($m, "carol"));
}

func testUnionMismatchThrows() {
    def threwSize as bool init false;
    try {
        union(new(1024, 4), new(2048, 4));
    } catch (e) {
        $threwSize = true;
        testing.assertEqual($e.kind, "bloom");
    }
    testing.assertTrue($threwSize);
    def threwK as bool init false;
    try {
        union(new(1024, 4), new(1024, 5));
    } catch (e) {
        $threwK = true;
    }
    testing.assertTrue($threwK);
}
