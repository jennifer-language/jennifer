// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	add("time", `
now() time.Time
utc() time.Time
fromUnix(int) time.Time
fromUnixMillis(int) time.Time
fromUnixNanos(int) time.Time
unix(time.Time) int
unixMillis(time.Time) int
unixNanos(time.Time) int
year(time.Time) int
month(time.Time) int
day(time.Time) int
hour(time.Time) int
minute(time.Time) int
second(time.Time) int
nanosecond(time.Time) int
weekday(time.Time) int
fromSeconds(int) time.Duration
fromMilliseconds(int) time.Duration
fromMinutes(int) time.Duration
fromHours(int) time.Duration
seconds(time.Duration) int
milliseconds(time.Duration) int
minutes(time.Duration) int
hours(time.Duration) int
add(time.Time, time.Duration) time.Time
sub(time.Time, time.Time) time.Duration
before(time.Time, time.Time) bool
after(time.Time, time.Time) bool
equal(time.Time, time.Time) bool
sleep(time.Duration) null
zone(int, string) time.Zone
inZone(time.Time, time.Zone) time.Time
local() time.Zone
format(time.Time, string) string
parse(string, string) time.Time
iso(time.Time) string
fromIso(string) time.Time
`)
}
