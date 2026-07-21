# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# memcache_test.j - white-box tests for memcache.j's pure helpers. Run with:
#
#     jennifer test modules/memcache_test.j
#
# The overlay splices memcache.j in front of this file, so the tests reach its
# private storage-header builder by bare identifier. The networked command path
# (set / add / get / delete / incr / touch) is verified against an in-process
# memcached server in the Go suite (TestMemcacheCommands); the byte-length
# framing that a protocol depends on is what the overlay pins.
use testing;

func testStoreHeaderSet() {
    testing.assertEqual(storeHeader("set", "greeting", "hello", 60), "set greeting 0 60 5");
}

func testStoreHeaderAdd() {
    testing.assertEqual(storeHeader("add", "lock", "1", 30), "add lock 0 30 1");
}

func testStoreHeaderEmptyValue() {
    testing.assertEqual(storeHeader("set", "k", "", 0), "set k 0 0 0");
}

func testStoreHeaderByteLength() {
    # bytes is the UTF-8 *byte* length, not the rune count: "café" is 5 bytes.
    testing.assertEqual(storeHeader("set", "k", "café", 0), "set k 0 0 5");
    # "über" is ü(2) + b(1) + e(1) + r(1) = 5 bytes (4 runes)
    testing.assertEqual(storeHeader("set", "k", "über", 120), "set k 0 120 5");
}
