#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The statsd module (modules/statsd.j): a fire-and-forget StatsD client over
 * UDP. Push a handful of metrics (counter / gauge / timer / set) to an agent.
 * Needs the default `jennifer` binary (net). Because StatsD is UDP, this sends
 * without error even when no agent is listening - point it at a real agent
 * (host:port as the first argument, default 127.0.0.1:8125) to see the metrics
 * land. Watch them with e.g. `nc -u -l 8125`.
 * Run: jennifer run examples/modules/statsd_demo.j [host:port]
 * @module statsd_demo
 */
use io;
use os;
import "../../modules/statsd.j" as statsd;

def address as string init "127.0.0.1:8125";
if (len(os.ARGS) > 1) {
    $address = os.ARGS[1];
}

io.printf("emitting metrics to %s (prefix \"web\") ...\n", $address);
def c as statsd.Client init statsd.clientWith($address, "web");

statsd.increment($c, "requests");        # web.requests:1|c
statsd.count($c, "errors", 2);           # web.errors:2|c
statsd.gauge($c, "queue.depth", 7);      # web.queue.depth:7|g
statsd.timing($c, "response", 42);       # web.response:42|ms
statsd.set($c, "users", "u123");         # web.users:u123|s
statsd.close($c);

io.printf("sent: requests(+1), errors(+2), queue.depth=7, response=42ms, users<-u123\n");
