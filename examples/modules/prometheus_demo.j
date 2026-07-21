#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Build a metric set and render it as Prometheus text exposition.
 * The exposition half is pure text and runs on both binaries; print it (as here) or write it to a `*.prom` file for the node_exporter textfile collector, POST it to a Pushgateway, or serve it from a `/metrics` handler. The retrieval half (`prometheus.query`) needs the default `jennifer` binary and a live Prometheus, so it is not shown here.
 * @module prometheus_demo
 */
use io;
import "../../modules/prometheus.j" as prometheus;

# A counter with a few labelled series.
def reqs as prometheus.Metric init prometheus.counter("http_requests_total",
    "Total HTTP requests by method and status");
$reqs = prometheus.observe($reqs, {"method": "get", "code": "200"}, 1027.0);
$reqs = prometheus.observe($reqs, {"method": "post", "code": "200"}, 3.0);
$reqs = prometheus.observe($reqs, {"method": "get", "code": "404"}, 12.0);

# A gauge with a single, label-less sample.
def temp as prometheus.Metric init prometheus.gauge("cpu_temperature_celsius",
    "Current CPU temperature");
$temp = prometheus.observe($temp, {}, 41.5);

io.printf("%s", prometheus.render([$reqs, $temp]));
