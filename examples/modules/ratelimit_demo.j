#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Throttle requests with a fixed-window limiter on memcached.
 * Needs a memcached server on 127.0.0.1:11211 and the default `jennifer` binary (ratelimit builds on the memcache module, which uses `net`). With no server running it prints the connection error rather than failing. Not a golden test (it needs a live server); it demonstrates the surface.
 * @module ratelimit_demo
 */
use io;
import "../../modules/ratelimit.j" as ratelimit;
import "../../modules/memcache.j" as memcache;

def opts as memcache.Options init memcache.Options{host: "127.0.0.1", port: 11211};

try {
    def mc as memcache.Session init memcache.connect($opts);

    # Allow 3 requests per 60-second window for one client.
    def key as string init "demo:ip:203.0.113.7";
    def limit as int init 3;

    # Drain the key first so the demo is repeatable.
    memcache.delete($mc, $key);

    def i as int init 1;
    while ($i <= 5) {
        def ok as bool init ratelimit.allow($mc, $key, $limit, 60);
        io.printf("request %d -> %t   (remaining: %d)\n", $i,
            $ok, ratelimit.remaining($mc, $key, $limit));
        $i = $i + 1;
    }

    memcache.delete($mc, $key);
    memcache.quit($mc);
} catch (e) {
    io.printf("no memcached server at %s:%d (%s)\n", $opts.host, $opts.port, $e.message);
}
