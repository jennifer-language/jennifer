# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# ringbuffer_test.j - white-box tests for ringbuffer.j. Run with:
#
#     jennifer test modules/ringbuffer_test.j
#
# Pure bounded-FIFO behaviour, no network. ringbuffer.j already `use`s lists, so
# the overlay only adds testing.
use testing;

func testPushWithinCapacity() {
    def r as RingBuffer init new(3);
    $r = push($r, "a");
    $r = push($r, "b");
    testing.assertEqual(size($r), 2);
    testing.assertTrue(not isFull($r));
    testing.assertEqual(first($r), "a");
    testing.assertEqual(last($r), "b");
}

func testPushOverwritesOldest() {
    def r as RingBuffer init new(3);
    $r = push($r, "a");
    $r = push($r, "b");
    $r = push($r, "c");
    $r = push($r, "d");   # overwrites "a"
    testing.assertEqual(size($r), 3);
    testing.assertTrue(isFull($r));
    testing.assertEqual(first($r), "b");
    testing.assertEqual(last($r), "d");
}

func testPopFifo() {
    def r as RingBuffer init new(3);
    $r = push($r, "x");
    $r = push($r, "y");
    testing.assertEqual(first($r), "x");
    $r = pop($r);
    testing.assertEqual(first($r), "y");
    testing.assertEqual(size($r), 1);
}

func testEmptyOperationsThrow() {
    def r as RingBuffer init new(2);
    def threwPop as bool init false;
    try {
        pop($r);
    } catch (e) {
        $threwPop = true;
        testing.assertEqual($e.kind, "ringbuffer");
    }
    testing.assertTrue($threwPop);
    def threwFirst as bool init false;
    try {
        first($r);
    } catch (e) {
        $threwFirst = true;
    }
    testing.assertTrue($threwFirst);
    testing.assertTrue(isEmpty($r));
}

func testPushIsValueSemantic() {
    def base as RingBuffer init new(3);
    def withA as RingBuffer init push($base, "a");
    testing.assertEqual(size($withA), 1);
    # the original is unchanged
    testing.assertEqual(size($base), 0);
    testing.assertTrue(isEmpty($base));
}

func testToListOrder() {
    def r as RingBuffer init new(4);
    $r = push($r, "1");
    $r = push($r, "2");
    $r = push($r, "3");
    def items as list of string init toList($r);
    testing.assertEqual(len($items), 3);
    testing.assertEqual($items[0], "1");
    testing.assertEqual($items[2], "3");
    testing.assertEqual(capacity($r), 4);
}

func testInvalidCapacityThrows() {
    def threw as bool init false;
    try {
        new(0);
    } catch (e) {
        $threw = true;
        testing.assertEqual($e.kind, "ringbuffer");
    }
    testing.assertTrue($threw);
}
