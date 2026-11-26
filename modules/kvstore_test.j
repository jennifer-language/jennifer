# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# kvstore_test.j - tests for kvstore.j over the in-process backend (no server).
# Run with:
#
#     jennifer test modules/kvstore_test.j
#
# The overlay splices kvstore.j in front of this file, so the tests reach its
# exported surface by bare identifier. The memcache / redis backends (built by
# memcache.kvStore / redis.kvStore) are exercised in the Go suite
# (TestSessionLifecycle, TestRatelimit, TestKvstoreRedisBackend); here a fake
# Backend checks the Remote dispatch. No network: these run on jennifer-tiny-dev.
use testing;
use net;

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

func negativeSet() {
    set(inProcessStore(), "k", "v", -1);
}

func negativeTouch() {
    def s as Store init inProcessStore();
    set($s, "k", "v", 0);
    touch($s, "k", -1);
}

func negativeWindow() {
    incrWindow(inProcessStore(), "hits", -60);
}

# A negative ttlSec means different things to the backends, so kvstore
# refuses it before any backend sees it.
func testNegativeTtlIsRefused() {
    testing.assertThrows("negativeSet", "kvstore");
    testing.assertThrows("negativeTouch", "kvstore");
    testing.assertThrows("negativeWindow", "kvstore");
}

# --- the Remote dispatch, over a fake Backend that records each call ---

def const FAKE as kv.Store init kv.open();

func fakeSet(b as Backend, key as string, value as string, ttlSec as int) {
    kv.set(FAKE, $b.name + ":" + $key, $value + "/{$ttlSec}", 0);
    return null;
}
func fakeGet(b as Backend, key as string) {
    return kv.get(FAKE, $b.name + ":" + $key);
}
func fakeDelete(b as Backend, key as string) {
    return kv.delete(FAKE, $b.name + ":" + $key);
}
func fakeTouch(b as Backend, key as string, ttlSec as int) {
    return $ttlSec == 7;
}
func fakeIncrWindow(b as Backend, key as string, ttlSec as int) {
    return $ttlSec * 2;
}

func fakeStore() {
    def conn as net.Conn;
    return remoteStore(Backend{
        name: "fake",
        conn: $conn,
        timeoutMs: 0,
        set: fakeSet,
        get: fakeGet,
        delete: fakeDelete,
        touch: fakeTouch,
        incrWindow: fakeIncrWindow
    });
}

func testRemoteDispatch() {
    def s as Store init fakeStore();
    set($s, "k", "v", 30);
    testing.assertEqual(get($s, "k"), "v/30");
    testing.assertTrue(delete($s, "k"));
    testing.assertFalse(delete($s, "k"));
    testing.assertTrue(touch($s, "k", 7));
    testing.assertFalse(touch($s, "k", 8));
    testing.assertEqual(incrWindow($s, "w", 21), 42);
}

func remoteNegativeTtl() {
    set(fakeStore(), "k", "v", -1);
}

# The TTL check runs before the dispatch, for a remote backend too.
func testRemoteNegativeTtlIsRefused() {
    testing.assertThrows("remoteNegativeTtl", "kvstore");
}
