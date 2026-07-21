# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Exercises the `time` library. Uses `time.fromUnix`
 * throughout so the golden file is deterministic (time.now() would
 * vary with the wall clock).
 * @module time
 */

use io;
use time;

# --- Fix an instant: 2024-06-15T12:00:00Z (a Saturday) ---
def t as time.Time init time.fromUnix(1718452800);

io.printf("year=%d\n", time.year($t));
io.printf("month=%d\n", time.month($t));
io.printf("day=%d\n", time.day($t));
io.printf("hour=%d\n", time.hour($t));
io.printf("minute=%d\n", time.minute($t));
io.printf("second=%d\n", time.second($t));
io.printf("weekday=%d\n", time.weekday($t));

# --- Unix accessors round-trip ---
io.printf("unix=%d\n", time.unix($t));
io.printf("unixMillis=%d\n", time.unixMillis($t));

# --- Duration: 90 seconds shown three ways ---
def d as time.Duration init time.fromSeconds(90);
io.printf("seconds=%d ms=%d minutes=%d\n",
    time.seconds($d), time.milliseconds($d), time.minutes($d));

# --- Arithmetic: t + 1 hour, then subtract back to a Duration ---
def hour as time.Duration init time.fromHours(1);
def later as time.Time init time.add($t, $hour);
def gap as time.Duration init time.sub($later, $t);
io.printf("later.unix=%d gap.seconds=%d\n", time.unix($later), time.seconds($gap));

# --- Comparison: earlier instants compare correctly ---
def earlier as time.Time init time.fromUnix(1718452700);
io.printf("earlier<t=%t t<earlier=%t equal=%t\n",
    time.before($earlier, $t),
    time.before($t, $earlier),
    time.equal($t, $t));
