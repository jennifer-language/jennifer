#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Parse a few cron expressions and show the next time each fires after a fixed
 * instant, then the next fire relative to the current clock. cron is a pure
 * calculator - a real scheduler is a `spawn` + `time.sleep` loop over `cron.next`.
 * @module cron_demo
 */
use io;
use time;
import "../../modules/cron.j" as cron;

def base as time.Time init time.fromIso("2026-03-14T10:30:00+00:00");   # a Saturday
io.printf("next fire after %s:\n", time.iso($base));

def specs as list of string init [
    "*/15 * * * *",     # every 15 minutes
    "0 9 * * 1-5",      # 09:00 on weekdays
    "0 0 1 * *",        # midnight on the 1st of each month
    "0 0 13 * 5",       # midnight on Friday the 13th (dom OR dow)
    "0 0 * * 0"         # midnight on Sundays
];
for (def spec in $specs) {
    def s as cron.Schedule init cron.parse($spec);
    io.printf("  %s  ->  %s\n", $spec, time.iso(cron.next($s, $base)));
}

# Relative to the current clock:
def hourly as cron.Schedule init cron.parse("0 * * * *");
io.printf("next top of the hour: %s\n", time.iso(cron.next($hourly, time.now())));
