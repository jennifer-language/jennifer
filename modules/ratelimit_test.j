# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# ratelimit_test.j - white-box tests for ratelimit.j's pure helpers. Run with:
#
#     jennifer test modules/ratelimit_test.j
#
# The overlay splices ratelimit.j in front of this file, so the tests reach its
# private budget arithmetic by bare identifier. The networked allow / remaining
# path (the incr-then-add window logic) is verified against an in-process
# memcached server in the Go suite (TestRatelimit).
use testing;

func testWithinLimit() {
    testing.assertTrue(withinLimit(1, 5));      # first hit
    testing.assertTrue(withinLimit(5, 5));      # last allowed hit
    testing.assertFalse(withinLimit(6, 5));     # over the limit
}

func testRemainingFromEmpty() {
    # no hits yet -> the whole budget is available
    testing.assertEqual(remainingFrom("", 5), 5);
}

func testRemainingFromPartial() {
    testing.assertEqual(remainingFrom("2", 5), 3);
    testing.assertEqual(remainingFrom("5", 5), 0);
}

func testRemainingFromExhausted() {
    # counter past the limit clamps at 0 (never negative)
    testing.assertEqual(remainingFrom("7", 5), 0);
}
