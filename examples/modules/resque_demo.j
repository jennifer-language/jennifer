#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Schedule background jobs and drain them from a worker, both sides in one file, with the resque module.
 * Needs a Redis server on 127.0.0.1:6379 and the default jennifer binary (resque runs on the redis module, which uses net). The jobs it enqueues are Resque wire-compatible, so a Ruby-resque worker could process them instead. With no server running it prints the connection error rather than failing.
 * @module resque_demo
 */
use io;
import "../../modules/resque.j" as resque;
import "../../modules/redis.j" as redis;

# handle dispatches one reserved job on its class (a worker's job table).
func handle(db as redis.Session, job as resque.Job) {
    if ($job.class == "Ping") {
        io.printf("[%s] pong\n", $job.queue);
    } elseif ($job.class == "SendWelcome") {
        io.printf("[%s] welcome mail -> %s (%s)\n", $job.queue, $job.args[0], $job.args[1]);
    } elseif ($job.class == "Resize") {
        io.printf("[%s] resize %s to %s\n", $job.queue, $job.args[0], $job.args[1]);
    } else {
        resque.fail($db, $job, "unknown class");
    }
}

def opts as redis.Options init redis.Options{host: "127.0.0.1", port: 6379,
    security: "none", user: "", password: "", db: 0};

try {
    def db as redis.Session init redis.connect($opts);

    # --- producer: schedule some work onto two queues -----------------
    resque.enqueue($db, "high", "Ping", []);
    resque.enqueue($db, "email", "SendWelcome", ["user@example.com", "en"]);
    resque.enqueue($db, "email", "Resize", ["avatar.png", "128"]);

    io.printf("queues: %d   pending: %d\n", len(resque.queues($db)), resque.size($db));

    # --- worker: reserve in priority order, dispatch on the class -----
    def done as bool init false;
    while (not $done) {
        def job as resque.Job init resque.reserve($db, ["high", "email"]);
        if (len($job.class) == 0) {
            $done = true;                # every queue drained
        } else {
            try {
                handle($db, $job);
            } catch (e) {
                resque.fail($db, $job, $e.message);
            }
        }
    }

    io.printf("drained; pending: %d\n", resque.size($db));
    redis.quit($db);
} catch (e) {
    io.printf("no Redis server at %s:%d (%s)\n", $opts.host, $opts.port, $e.message);
}
