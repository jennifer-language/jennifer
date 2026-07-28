# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# kvstore_test.j - tests for kvstore.j over the in-process backend (no server).
# Run with:
#
#     jennifer test modules/kvstore_test.j
#
# The overlay splices kvstore.j in front of this file, so the tests reach its
# exported surface by bare identifier. The memcache / redis backends are exercised
# in the Go suite (TestSessionLifecycle, TestRatelimit, TestKvstoreRedisBackend).
use testing;

func testSetGetDelete() {
    def s as Store init inProcessStore();
    set($s, "k", "hello", 0);
    testing.assertEqual(get($s, "k"), "hello");
    testing.assertEqual(get($s, "missing"), "");
    testing.assertTrue(delete($s, "k"));
    testing.assertFalse(delete($s, "k"));
    testing.assertEqual(get($s, "k"), "");
}

func testTouch() {
    def s as Store init inProcessStore();
    set($s, "k", "v", 60);
    testing.assertTrue(touch($s, "k", 120));
    testing.assertFalse(touch($s, "absent", 60));
}

func testIncrWindow() {
    def s as Store init inProcessStore();
    # first hit creates the counter at 1, then it increments
    testing.assertEqual(incrWindow($s, "rl", 60), 1);
    testing.assertEqual(incrWindow($s, "rl", 60), 2);
    testing.assertEqual(incrWindow($s, "rl", 60), 3);
    # a separate key has its own counter
    testing.assertEqual(incrWindow($s, "other", 60), 1);
}

# Two stores are independent (each inProcessStore opens its own kv.Store).
func testStoresIsolated() {
    def a as Store init inProcessStore();
    def b as Store init inProcessStore();
    set($a, "k", "in-a", 0);
    testing.assertEqual(get($b, "k"), "");
}
