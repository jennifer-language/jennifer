#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The influxdb module (modules/influxdb.j): an InfluxDB 1.x client over `http`.
 * Build line-protocol points and write them, then run an InfluxQL query and
 * print the parsed rows. Needs the default `jennifer` binary (net). Point it at
 * a running InfluxDB (base URL + database as the first two arguments, default
 * http://localhost:8086 / metrics); without a server the write / query throw
 * `Error{kind: "influxdb"}`, which this demo catches and reports.
 * Run: jennifer run examples/modules/influxdb_demo.j [url] [db]
 * @module influxdb_demo
 */
use io;
use os;
use strings;
import "../../modules/influxdb.j" as influxdb;

def url as string init "http://localhost:8086";
def db as string init "metrics";
if (len(os.ARGS) > 1) { $url = os.ARGS[1]; }
if (len(os.ARGS) > 2) { $db = os.ARGS[2]; }

def client as influxdb.Client init influxdb.client($url, $db);

# Build two points with mixed field types (float, int, string, bool) and tags.
def cpu as influxdb.Point init influxdb.point("cpu");
$cpu = influxdb.tag($cpu, "host", "server01");
$cpu = influxdb.field($cpu, "value", 0.64);
$cpu = influxdb.intField($cpu, "cores", 8);

def status as influxdb.Point init influxdb.point("status");
$status = influxdb.tag($status, "host", "server01");
$status = influxdb.stringField($status, "state", "ok");
$status = influxdb.boolField($status, "healthy", true);

io.printf("line protocol:\n  %s\n  %s\n", influxdb.line($cpu), influxdb.line($status));

io.printf("\nwriting to %s (db %s) ...\n", $url, $db);
try {
    influxdb.write($client, [$cpu, $status]);
    io.printf("wrote 2 points\n");

    def r as influxdb.Result init influxdb.query($client, "SELECT last(\"value\") FROM cpu");
    io.printf("\nquery returned %d series:\n", len($r.series));
    for (def s in $r.series) {
        io.printf("  %s  columns=%s\n", $s.name, strings.join($s.columns, ","));
        for (def row in $s.values) {
            io.printf("    %s\n", strings.join($row, " | "));
        }
    }
} catch (e) {
    io.printf("influxdb unavailable: %s\n", $e.message);
}
