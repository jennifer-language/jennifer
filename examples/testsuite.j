# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * A `jennifer test` target showing the assertion
 * vocabulary. Unlike testing.j (which self-drives testing.run and
 * renders reports), this file only defines test methods and lets the
 * runner orchestrate them. Run it with:
 *   jennifer test examples/testsuite.j
 *   jennifer test --format=tap examples/testsuite.j
 *   jennifer test --isolated examples/testsuite.j
 * The runner discovers every zero-arg method named test*, calls setUp
 * before each and tearDown after (both optional), and reports pass/fail.
 * @module testsuite
 */

use testing;
use lists;

# setUp runs before each test. Here it just counts invocations; a real
# suite would build fixtures.
def setupCount as int init 0;

func setUp() {
    $setupCount = $setupCount + 1;
}

# --- subjects under test -------------------------------------------

func add(a as int, b as int) {
    return $a + $b;
}

func classify(n as int) {
    if ($n < 0) {
        throw Error{kind: "negative", message: "n is negative", file: "", line: 0, col: 0};
    }
    return $n;
}

# A helper the assertThrows test reaches by name; it must throw.
func mustBeNegative() {
    classify(-1);
}

# --- tests (discovered by the `test` prefix) -----------------------

func testAdd() {
    testing.assertEqual(add(2, 3), 5);
    testing.assertNotEqual(add(2, 3), 6);
}

func testBooleans() {
    testing.assertTrue(add(1, 1) == 2);
    testing.assertFalse(add(1, 1) == 3);
}

func testContains() {
    testing.assertContains(lists.range(0, 5), 3);
    testing.assertContains("jennifer", "nn");
    testing.assertContains({"x": 1, "y": 2}, "y");
}

func testThrowsOnNegative() {
    testing.assertThrows("mustBeNegative", "negative");
}

# Table-driven: one method, a loop of cases. The first failing row
# throws and aborts the method, reporting that row's position.
func testAddTable() {
    def cases as list of list of int init [[1, 1, 2], [2, 3, 5], [10, 5, 15]];
    for (def c in $cases) {
        testing.assertEqual(add($c[0], $c[1]), $c[2]);
    }
}
