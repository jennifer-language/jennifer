# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Formatting / parsing / zones. Uses
 * time.fromUnix and an explicit Zone so the golden file is
 * deterministic.
 * @module time-format
 */

use io;
use time;

# 2024-06-15T12:34:56Z (UTC). Saturday in June.
def t as time.Time init time.fromIso("2024-06-15T12:34:56Z");

# --- strftime-style format ---
io.printf("date=%s\n", time.format($t, "%Y-%m-%d"));
io.printf("clock=%s\n", time.format($t, "%H:%M:%S"));
io.printf("zone=%s\n", time.format($t, "%z"));
io.printf("named=%s %s, %s %s %s\n",
    time.format($t, "%A"),
    time.format($t, "%B"),
    time.format($t, "%d"),
    time.format($t, "%Y"),
    time.format($t, "%H:%M:%S"));
io.printf("dayofyear=%s isoweekday=%s\n",
    time.format($t, "%j"),
    time.format($t, "%u"));

# --- Re-render in a fixed-offset zone ---
def vienna as time.Zone init time.zone(3600, "CET");
def tv as time.Time init time.inZone($t, $vienna);
io.printf("vienna=%s offset=%d\n",
    time.format($tv, "%Y-%m-%d %H:%M %z"),
    $tv.offset);
io.printf("vienna.name=%s\n", $vienna.name);

# --- UTC constant + ISO round-trip ---
def utc as time.Zone init time.UTC;
io.printf("UTC=%s offset=%d\n", $utc.name, $utc.offset);
io.printf("iso=%s\n", time.iso($t));
io.printf("iso-vienna=%s\n", time.iso($tv));

# --- Parse a strftime layout ---
def back as time.Time init time.parse("2024-06-15 12:34:56", "%Y-%m-%d %H:%M:%S");
io.printf("parsed unix=%d\n", time.unix($back));

# --- Round trip a parsed value through format ---
def again as string init time.format($back, "%Y-%m-%dT%H:%M:%S%z");
io.printf("round-trip=%s\n", $again);
