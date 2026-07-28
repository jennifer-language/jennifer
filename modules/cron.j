# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Parse and evaluate cron expressions - the five-field schedule spec `minute
 * hour day-of-month month day-of-week`. `parse` turns an expression into a
 * `Schedule`, `matches` tests whether a `time.Time` fires it, and `next` finds
 * the next fire at or after a given time. Each field takes `*`, single values,
 * `a-b` ranges, `a,b,c` lists, and `/n` steps (`a-b/n`, or a wildcard with a
 * step). Day-of-week is `0-7` (both `0` and `7` are Sunday). When both
 * day-of-month and day-of-week are restricted, a day matching **either** fires
 * (the standard cron rule). The month field also accepts the three-letter names
 * `JAN`-`DEC` and the weekday field `SUN`-`SAT` (case-insensitive), anywhere a
 * number works. A whole expression may be a nickname macro (`@daily`,
 * `@hourly`, ...); `@reboot` parses to a Schedule that never fires (startup only).
 *
 * A pure calculator over `time` - no clock, no sleeping. A scheduler is the
 * caller's loop (`spawn` + `time.sleep` until `cron.next`). Both binaries.
 * @module cron
 * @example
 * def s as cron.Schedule init cron.parse("30 9 * * 1-5");   # 09:30 on weekdays
 * def fire as time.Time init cron.next($s, time.now());
 * io.printf("next run: %s\n", time.iso($fire));
 */
use time;
use strings;
use convert;
use lists;

# The search horizon for `next`: give up after this many days rather than loop
# forever on an impossible schedule (e.g. Feb 31). Nine years covers the widest
# real gap: a Feb-29-only schedule spans 8 years across a non-leap century
# boundary (2096 -> 2104, since 2100 is not a leap year).
def const HORIZON_DAYS as int init 366 * 9;

/**
 * A parsed cron schedule: the allowed values per field. `weekdays` are ISO
 * weekdays (Monday = 1 ... Sunday = 7), normalized from the cron `0-7` form.
 * @field minutes {list of int} allowed minutes (0-59)
 * @field hours {list of int} allowed hours (0-23)
 * @field daysOfMonth {list of int} allowed days of the month (1-31)
 * @field months {list of int} allowed months (1-12)
 * @field weekdays {list of int} allowed ISO weekdays (1-7, Monday = 1)
 * @field domStar {bool} whether the day-of-month field was "*"
 * @field dowStar {bool} whether the day-of-week field was "*"
 * @field reboot {bool} true only for the `@reboot` macro (a startup-only
 *   schedule that never fires a time: `matches` is always false and `next`
 *   throws)
 */
export def struct Schedule {
    minutes as list of int,
    hours as list of int,
    daysOfMonth as list of int,
    months as list of int,
    weekdays as list of int,
    domStar as bool,
    dowStar as bool,
    reboot as bool
};

# --- field parsing (private) ------------------------------------------------

# fail throws a catchable cron error.
func fail(message as string) {
    throw Error{kind: "cron", message: "cron: " + $message, file: "", line: 0, col: 0};
}

# nameToNumber translates a three-letter month (`JAN`-`DEC` -> 1-12) or weekday
# (`SUN`-`SAT` -> 0-6) name to its number, case-insensitively, returning -1 when
# the field takes no names or the token is not a known name. Only the month and
# day-of-week fields carry names; every field's numbers still parse as before.
func nameToNumber(s as string, field as string) {
    def up as string init strings.upper($s);
    if ($field == "month") {
        def months as list of string init [
            "JAN",
            "FEB",
            "MAR",
            "APR",
            "MAY",
            "JUN",
            "JUL",
            "AUG",
            "SEP",
            "OCT",
            "NOV",
            "DEC"
        ];
        def i as int init 0;
        for (def m in $months) {
            if ($m == $up) {
                return $i + 1;
            }
            $i = $i + 1;
        }
    }
    if ($field == "day-of-week") {
        def days as list of string init ["SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"];
        def j as int init 0;
        for (def d in $days) {
            if ($d == $up) {
                return $j;
            }
            $j = $j + 1;
        }
    }
    return 0 - 1;
}

# toIntChecked parses a bare decimal, throwing a "cron"-kind error (not
# convert's generic one) when the substring isn't a digit run, so a caller
# catching kind == "cron" doesn't crash on malformed input like "a * * * *".
# A month or weekday name is first translated to its number, so names work
# anywhere a number does (single values, `a-b` ranges, `a,b,c` lists).
func toIntChecked(s as string, field as string, term as string) {
    if (len($s) == 0) {
        fail("empty number in " + $field + " field: " + $term);
    }
    def named as int init nameToNumber($s, $field);
    if ($named >= 0) {
        return $named;
    }
    for (def ch in strings.chars($s)) {
        if (strings.indexOf("0123456789", $ch) < 0) {
            fail("non-numeric value in " + $field + " field: " + $term);
        }
    }
    return convert.toInt($s);
}

# parseTerm expands one comma-term of a field (`*`, `a`, `a-b`, and any `/step`)
# into its list of integer values, validated against [minv, maxv].
func parseTerm(term as string, minv as int, maxv as int, field as string) {
    def base as string init $term;
    def step as int init 1;
    def slash as int init strings.indexOf($term, "/");
    if ($slash >= 0) {
        $base = strings.substring($term, 0, $slash);
        $step = toIntChecked(strings.substring($term, $slash + 1, len($term)), $field, $term);
        if ($step <= 0) {
            fail("step must be positive in " + $field + " field: " + $term);
        }
    }
    def start as int init $minv;
    def end as int init $maxv;
    if (not ($base == "*")) {
        def dash as int init strings.indexOf($base, "-");
        if ($dash >= 0) {
            $start = toIntChecked(strings.substring($base, 0, $dash), $field, $term);
            $end = toIntChecked(strings.substring($base, $dash + 1, len($base)), $field, $term);
        } else {
            $start = toIntChecked($base, $field, $term);
            # a bare value with a step runs from the value to the field's max
            if ($slash >= 0) {
                $end = $maxv;
            } else {
                $end = $start;
            }
        }
    }
    if ($start < $minv or $end > $maxv or $start > $end) {
        fail("value out of range in " + $field + " field: " + $term);
    }
    def out as list of int init [];
    def v as int init $start;
    while ($v <= $end) {
        $out[] = $v;
        $v = $v + $step;
    }
    return $out;
}

# parseField expands a whole field (its comma-terms) into its value list.
func parseField(spec as string, minv as int, maxv as int, field as string) {
    def out as list of int init [];
    for (def term in strings.split($spec, ",")) {
        for (def v in parseTerm($term, $minv, $maxv, $field)) {
            $out[] = $v;
        }
    }
    return $out;
}

# fields splits an expression on runs of whitespace, dropping empties.
func fields(expr as string) {
    def flat as string init strings.replace(strings.trim($expr), "\t", " ");
    def out as list of string init [];
    for (def f in strings.split($flat, " ")) {
        if (not ($f == "")) {
            $out[] = $f;
        }
    }
    return $out;
}

# --- parse (exported) -------------------------------------------------------

# macroExpansion maps a nickname macro (lowercased, leading `@`) to its standard
# five-field expression, or "" when the name is not a known time-based macro.
# `@reboot` is not here - it has no time schedule and is handled in parse.
func macroExpansion(name as string) {
    if ($name == "@yearly" or $name == "@annually") {
        return "0 0 1 1 *";
    }
    if ($name == "@monthly") {
        return "0 0 1 * *";
    }
    if ($name == "@weekly") {
        return "0 0 * * 0";
    }
    if ($name == "@daily" or $name == "@midnight") {
        return "0 0 * * *";
    }
    if ($name == "@hourly") {
        return "0 * * * *";
    }
    return "";
}

# parseNickname handles a whole-expression `@macro`. `@reboot` yields a
# startup-only Schedule (reboot true, empty fields) that never fires; the other
# macros expand to a standard expression and re-enter parse.
func parseNickname(spec as string) {
    def low as string init strings.lower($spec);
    if ($low == "@reboot") {
        return Schedule{
            minutes: [],
            hours: [],
            daysOfMonth: [],
            months: [],
            weekdays: [],
            domStar: false,
            dowStar: false,
            reboot: true
        };
    }
    def expanded as string init macroExpansion($low);
    if ($expanded == "") {
        fail("unknown nickname macro: " + $spec);
    }
    return parse($expanded);
}

/**
 * Parse a five-field cron expression into a Schedule. The whole expression may
 * instead be a nickname macro (`@yearly` / `@annually`, `@monthly`, `@weekly`,
 * `@daily` / `@midnight`, `@hourly`, `@reboot`). The month field accepts
 * `JAN`-`DEC` and the weekday field `SUN`-`SAT` (case-insensitive) anywhere a
 * number works.
 * @param expr {string} `minute hour day-of-month month day-of-week`, or a `@macro`
 * @return {Schedule} the parsed schedule
 * @throws {Error} kind "cron" on the wrong field count or an out-of-range value
 */
export func parse(expr as string) {
    def trimmed as string init strings.trim($expr);
    if (strings.startsWith($trimmed, "@")) {
        return parseNickname($trimmed);
    }
    def parts as list of string init fields($expr);
    if (not (len($parts) == 5)) {
        fail("expression needs 5 fields (minute hour day month weekday), got " +
            convert.toString(len($parts)));
    }
    # day-of-week: parse 0-7, then fold 0 (cron Sunday) to ISO 7
    def dowRaw as list of int init parseField($parts[4], 0, 7, "day-of-week");
    def weekdays as list of int init [];
    for (def d in $dowRaw) {
        if ($d == 0) {
            $weekdays[] = 7;
        } else {
            $weekdays[] = $d;
        }
    }
    return Schedule{
        minutes: parseField($parts[0], 0, 59, "minute"),
        hours: parseField($parts[1], 0, 23, "hour"),
        daysOfMonth: parseField($parts[2], 1, 31, "day-of-month"),
        months: parseField($parts[3], 1, 12, "month"),
        weekdays: $weekdays,
        # A `*/n` field is unrestricted for the DOM-OR-DOW rule, matching
        # Vixie/cronie: treat any field starting with `*` as a star.
        domStar: strings.startsWith($parts[2], "*"),
        dowStar: strings.startsWith($parts[4], "*"),
        reboot: false
    };
}

# --- match / next (exported) ------------------------------------------------

# dayMatches applies the cron day rule: month must match, then day-of-month and
# day-of-week combine with OR when both are restricted, else the restricted one.
func dayMatches(schedule as Schedule, t as time.Time) {
    if (not lists.contains($schedule.months, time.month($t))) {
        return false;
    }
    def domMatch as bool init lists.contains($schedule.daysOfMonth, time.day($t));
    def dowMatch as bool init lists.contains($schedule.weekdays, time.weekday($t));
    if ($schedule.domStar and $schedule.dowStar) {
        return true;
    }
    if ($schedule.domStar) {
        return $dowMatch;
    }
    if ($schedule.dowStar) {
        return $domMatch;
    }
    return $domMatch or $dowMatch;
}

/**
 * Report whether a time fires the schedule (minute granularity - seconds are
 * ignored).
 * @param schedule {Schedule} the schedule
 * @param t {time.Time} the instant to test
 * @return {bool} true if the schedule fires at `t`
 */
export func matches(schedule as Schedule, t as time.Time) {
    # a @reboot schedule has no time-based fire: it never matches a clock time.
    if ($schedule.reboot) {
        return false;
    }
    if (not lists.contains($schedule.minutes, time.minute($t))) {
        return false;
    }
    if (not lists.contains($schedule.hours, time.hour($t))) {
        return false;
    }
    return dayMatches($schedule, $t);
}

# truncateMinute returns `t` with its seconds and sub-seconds zeroed, keeping the
# time's offset.
func truncateMinute(t as time.Time) {
    def sub as int init time.second($t) * 1000000000 + time.nanosecond($t);
    return time.add($t, time.Duration{nanos: 0 - $sub});
}

# nextMidnight jumps to 00:00 of the following day (seconds already zeroed).
func nextMidnight(t as time.Time) {
    def sinceMidnight as int init time.hour($t) * 60 + time.minute($t);
    return time.add($t, time.fromMinutes(1440 - $sinceMidnight));
}

/**
 * Find the next time at or after `after` that fires the schedule. Searches
 * minute by minute (skipping non-matching days whole), up to a five-year
 * horizon.
 * @param schedule {Schedule} the schedule
 * @param after {time.Time} the earliest acceptable fire time (its zone is kept)
 * @return {time.Time} the next fire time (seconds zeroed)
 * @throws {Error} kind "cron" if nothing matches within the horizon
 */
export func next(schedule as Schedule, after as time.Time) {
    # a @reboot schedule fires only at startup, never at a clock time.
    if ($schedule.reboot) {
        fail("@reboot has no scheduled time (it fires only at startup)");
    }
    def t as time.Time init truncateMinute($after);
    if (time.unixNanos($t) < time.unixNanos($after)) {
        $t = time.add($t, time.fromMinutes(1));
    }
    def deadline as time.Time init time.add($after, time.fromHours(24 * HORIZON_DAYS));
    while (time.unixNanos($t) < time.unixNanos($deadline)) {
        if (dayMatches($schedule, $t)) {
            if (matches($schedule, $t)) {
                return $t;
            }
            $t = time.add($t, time.fromMinutes(1));
        } else {
            $t = nextMidnight($t);
        }
    }
    fail("no matching time within " + convert.toString(HORIZON_DAYS) + " days");
}
