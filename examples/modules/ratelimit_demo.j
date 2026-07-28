#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Throttle requests with the ratelimit module (fixed or sliding window). This
 * demo uses the in-process backend (kvstore.inProcessStore), so it runs with no
 * server; swap in kvstore.redisStore($rc) / memcacheStore($mc) for a distributed
 * limiter shared across processes.
 * @module ratelimit_demo
 */
use io;
import "../../modules/ratelimit.j" as ratelimit;
import "../../modules/kvstore.j" as kvstore;

def store as kvstore.Store init kvstore.inProcessStore();

# Allow 3 requests per 60-second window for one client.
def lim as ratelimit.Limiter init ratelimit.fixedWindow($store, 3, 60);
def key as string init "ip:203.0.113.7";

def i as int init 1;
while ($i <= 5) {
    def r as ratelimit.Result init ratelimit.check($lim, $key);
    io.printf(
        "request %d -> allowed=%t  remaining=%d  retryAfter=%d\n",
        $i,
        $r.allowed,
        $r.remaining,
        $r.retryAfter);
    $i = $i + 1;
}

# The Result carries everything a 429 needs: on a denied request, set
# Retry-After: r.retryAfter and X-RateLimit-Reset: r.resetSeconds.
def peek as ratelimit.Result init ratelimit.peek($lim, $key);
io.printf(
    "peek -> allowed=%t remaining=%d reset=%d\n",
    $peek.allowed,
    $peek.remaining,
    $peek.resetSeconds);
