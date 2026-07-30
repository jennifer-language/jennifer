#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

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

statsd.increment($c, "requests"); # web.requests:1|c
statsd.count($c, "errors", 2); # web.errors:2|c
statsd.gauge($c, "queue.depth", 7); # web.queue.depth:7|g
statsd.timing($c, "response", 42); # web.response:42|ms
statsd.set($c, "users", "u123"); # web.users:u123|s

# Sample rate: a "|@rate" suffix (agent scales the count back up by 1/rate).
statsd.countRate($c, "sampled", 1, 0.1); # web.sampled:1|c|@0.1

# DogStatsD tags: a "|#k:v,k2:v2" suffix, control-character validated.
def tags as map of string to string init {"env": "prod", "host": "h1"};
statsd.countTagged($c, "hits", 1, $tags); # web.hits:1|c|#env:prod,host:h1

# Float value: StatsD accepts fractional gauges (e.g. a load average).
statsd.gaugeFloat($c, "load", 3.5); # web.load:3.5|g

# Batching: pack several metrics into ONE datagram (lines joined by "\n").
def b as statsd.Batch init statsd.batch($c);
$b = statsd.addCount($b, "hits", 3);
$b = statsd.addGauge($b, "queue.depth", 9);
$b = statsd.addIncrement($b, "errors");
statsd.flush($c, $b);

statsd.close($c);

io.printf("sent single metrics, a sampled counter, a tagged counter, a float gauge,\n");
io.printf("and a 3-metric batch packet:\n");
for (def line in $b.lines) {
    io.printf("  %s\n", $line);
}
