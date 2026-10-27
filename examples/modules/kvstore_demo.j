#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * One key/value surface over a selectable backend with kvstore. This demo uses
 * the in-process backend (kvstore.inProcessStore), so it runs with no server;
 * swap in kvstore.redisStore($rc) / memcacheStore($mc) for a store shared across
 * processes, or kvstore.fileStore(path) to keep it across runs. The calls below
 * stay the same whichever backend you pick.
 * @module kvstore_demo
 */
use io;
import "../../modules/kvstore.j" as kvstore;

def store as kvstore.Store init kvstore.inProcessStore();

# set / get with a TTL in seconds (0 = no expiry).
kvstore.set($store, "greeting", "hello", 60);
io.printf("greeting: %s\n", kvstore.get($store, "greeting"));
io.printf("missing:  [%s]\n", kvstore.get($store, "nope"));

# touch re-arms the expiry; delete reports whether the key was there.
io.printf("touch:    %t\n", kvstore.touch($store, "greeting", 120));
io.printf("delete:   %t\n", kvstore.delete($store, "greeting"));
io.printf("again:    %t\n", kvstore.delete($store, "greeting"));

# incrWindow is the atomic counter a fixed-window rate limiter is built on: the
# first hit creates it at 1 with the TTL, later hits count up.
for (def i in 0..3) {
    io.printf("hit %d:    %d\n", $i + 1, kvstore.incrWindow($store, "hits:203.0.113.7", 60));
}
