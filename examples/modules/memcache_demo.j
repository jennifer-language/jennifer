#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Cache values in memcached with the memcache module.
 * Needs a memcached server on 127.0.0.1:11211 and the default `jennifer` binary (the module uses `net`). With no server running it prints the connection error rather than failing. Not a golden test (it needs a live server); it demonstrates the surface.
 * @module memcache_demo
 */
use io;
import "../../modules/memcache.j" as memcache;

def opts as memcache.Options init memcache.Options{host: "127.0.0.1", port: 11211};

try {
    def mc as memcache.Session init memcache.connect($opts);

    # set / get with a TTL.
    memcache.set($mc, "greeting", "hello from jennifer", 60);
    io.printf("get      -> %s\n", memcache.get($mc, "greeting"));
    io.printf("miss     -> [%s]\n", memcache.get($mc, "absent"));

    # add stores only if absent: the second call reports false.
    io.printf("add new  -> %t\n", memcache.add($mc, "once", "1", 60));
    io.printf("add again-> %t\n", memcache.add($mc, "once", "2", 60));

    # atomic counters.
    memcache.set($mc, "hits", "0", 0);
    io.printf("incr     -> %d\n", memcache.incr($mc, "hits", 1));
    io.printf("incr     -> %d\n", memcache.incr($mc, "hits", 4));
    io.printf("decr     -> %d\n", memcache.decr($mc, "hits", 2));

    # touch re-arms the TTL; delete removes.
    io.printf("touch    -> %t\n", memcache.touch($mc, "greeting", 120));
    io.printf("delete   -> %t\n", memcache.delete($mc, "greeting"));

    # clean up.
    memcache.delete($mc, "once");
    memcache.delete($mc, "hits");
    memcache.quit($mc);
} catch (e) {
    io.printf("no memcached server at %s:%d (%s)\n", $opts.host, $opts.port, $e.message);
}
