# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# ratelimit_test.j - white-box tests for ratelimit.j. Run with:
#
#     jennifer test modules/ratelimit_test.j
#
# The overlay splices ratelimit.j in front of this file, so the tests reach its
# private `fixedAt` / `slidingAt` (which take an explicit `now`) by bare
# identifier and drive window boundaries deterministically over an in-process
# `kv` store - no clock waiting, no server. The public check / peek (which read
# the wall clock) and the memcache / redis backends are exercised in the Go suite
# (TestRatelimit).
use testing;

# fixedWindow: allow up to the limit within one aligned window, then deny with a
# retry-after equal to the seconds until the window rolls.
func testFixedWindow() {
    def st as kvstore.Store init kvstore.inProcessStore();
    def lim as Limiter init fixedWindow($st, 3, 60);
    # now=100 -> window idx 1 (60..119), resets in 120-100 = 20 s.
    def r1 as Result init fixedAt($lim, "k", 100, true);
    testing.assertTrue($r1.allowed);
    testing.assertEqual($r1.remaining, 2);
    testing.assertEqual($r1.resetSeconds, 20);
    fixedAt($lim, "k", 105, true); # 2nd hit
    def r3 as Result init fixedAt($lim, "k", 110, true);
    testing.assertTrue($r3.allowed);
    testing.assertEqual($r3.remaining, 0);
    # 4th hit in the same window is denied; retry-after = seconds to the roll.
    def r4 as Result init fixedAt($lim, "k", 115, true);
    testing.assertFalse($r4.allowed);
    testing.assertEqual($r4.retryAfter, 5); # 120 - 115
    # the next aligned window (now=120 -> idx 2) starts fresh.
    def r5 as Result init fixedAt($lim, "k", 120, true);
    testing.assertTrue($r5.allowed);
    testing.assertEqual($r5.remaining, 2);
}

# peek reports the state without recording a hit.
func testPeekDoesNotRecord() {
    def st as kvstore.Store init kvstore.inProcessStore();
    def lim as Limiter init fixedWindow($st, 2, 60);
    fixedAt($lim, "k", 100, true); # record one hit
    def p as Result init fixedAt($lim, "k", 100, false);
    testing.assertTrue($p.allowed);
    testing.assertEqual($p.remaining, 1);
    def p2 as Result init fixedAt($lim, "k", 100, false);
    testing.assertEqual($p2.remaining, 1); # the earlier peek did not record
}

# slidingWindow smooths the boundary burst a fixed window would allow: after the
# previous window is filled, a fresh request at the very start of the next window
# is still blocked (the previous window carries full weight).
func testSlidingSmoothsBoundary() {
    def st as kvstore.Store init kvstore.inProcessStore();
    def lim as Limiter init slidingWindow($st, 10, 60);
    # Fill window idx 1 (now=110, elapsed 50) to the limit.
    def i as int init 0;
    while ($i < 10) {
        slidingAt($lim, "k", 110, true);
        $i = $i + 1;
    }
    # Start of window idx 2 (now=120, elapsed 0): previous window weight is 1.0,
    # so estimate = 1 (this hit) + 10 = 11 > 10 -> denied. A fixed window would
    # have allowed a fresh burst of 10 here.
    def boundary as Result init slidingAt($lim, "k", 120, true);
    testing.assertFalse($boundary.allowed);
    testing.assertTrue($boundary.retryAfter > 0);
    # Deeper into window idx 2 (now=150, elapsed 30): previous weight is 0.5, so
    # its contribution has decayed to 5 and new requests fit again.
    def later as Result init slidingAt($lim, "k", 150, true);
    testing.assertTrue($later.allowed);
}

# A non-positive limit or window is rejected up front (before a div-by-zero).
func zeroWindow() {
    fixedWindow(inProcessStoreFor(), 5, 0);
}
func zeroLimit() {
    slidingWindow(inProcessStoreFor(), 0, 60);
}
func inProcessStoreFor() {
    return kvstore.inProcessStore();
}
func testRejectsBadConfig() {
    testing.assertThrows("zeroWindow", "ratelimit");
    testing.assertThrows("zeroLimit", "ratelimit");
}

# A sliding window at a fresh key (no previous window) behaves like a plain
# counter.
func testSlidingFreshKey() {
    def st as kvstore.Store init kvstore.inProcessStore();
    def lim as Limiter init slidingWindow($st, 2, 60);
    def a as Result init slidingAt($lim, "fresh", 100, true);
    testing.assertTrue($a.allowed);
    def b as Result init slidingAt($lim, "fresh", 100, true);
    testing.assertTrue($b.allowed);
    def c as Result init slidingAt($lim, "fresh", 100, true);
    testing.assertFalse($c.allowed); # 3rd exceeds limit 2
}
