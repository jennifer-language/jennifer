#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Talk to a Redis server with the redis module.
 * Needs a Redis server listening on 127.0.0.1:6379 and the default `jennifer` binary (the module uses `net`). With no server running it prints the connection error rather than failing. Not a golden test (it needs a live server); it demonstrates the surface.
 * @module redis_demo
 */
use io;
import "../../modules/redis.j" as redis;

def opts as redis.Options init redis.Options{host: "127.0.0.1", port: 6379,
    security: "none", user: "", password: "", db: 0};

try {
    def db as redis.Session init redis.connect($opts);

    io.printf("ping     -> %s\n", redis.ping($db));

    redis.set($db, "greeting", "hello from jennifer");
    io.printf("get      -> %s\n", redis.get($db, "greeting"));
    io.printf("exists   -> %t\n", redis.exists($db, "greeting"));

    # A counter: INCR returns the new value each time.
    def n as int init 0;
    def i as int init 0;
    while ($i < 3) {
        $n = redis.incr($db, "demo:hits");
        $i = $i + 1;
    }
    io.printf("counter  -> %d\n", $n);

    # The generic command / Reply for anything without a typed helper.
    redis.command($db, ["RPUSH", "demo:queue", "one", "two", "three"]);
    def range as redis.Reply init redis.command($db, ["LRANGE", "demo:queue", "0", "-1"]);
    io.printf("queue    -> %d items\n", len($range.items));
    for (def item in $range.items) {
        io.printf("           %s\n", $item.str);
    }

    # An error reply is catchable.
    try {
        redis.command($db, ["INCR", "greeting"]);
    } catch (e) {
        io.printf("caught   -> %s\n", $e.message);
    }

    # Clean up the demo keys.
    redis.del($db, "greeting");
    redis.del($db, "demo:hits");
    redis.del($db, "demo:queue");
    redis.quit($db);
} catch (e) {
    io.printf("no Redis server at %s:%d (%s)\n", $opts.host, $opts.port, $e.message);
}
