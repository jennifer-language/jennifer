#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The ntp module (modules/ntp.j): a simple SNTP client. Query a time server and
 * print its time alongside the local clock offset and round-trip delay. Needs
 * the default `jennifer` binary (net); pass a server as the first argument, or
 * it defaults to pool.ntp.org.
 * Run: jennifer run examples/modules/ntp_demo.j [server]
 * @module ntp_demo
 */
use io;
use os;
use time;
import "../../modules/ntp.j" as ntp;

# os.ARGS[0] is the script path; an optional server follows.
def server as string init "pool.ntp.org";
if (len(os.ARGS) > 1) {
    $server = os.ARGS[1];
}

io.printf("querying %s ...\n", $server);
def r as ntp.Result init ntp.query($server);
io.printf("server time : %s\n", time.iso($r.serverTime));
io.printf("local time  : %s\n", time.iso(time.utc()));
io.printf("clock offset: %d ms\n", time.milliseconds($r.offset));
io.printf("round trip  : %d ms\n", time.milliseconds($r.delay));
