# `time` - instants, durations, and arithmetic

Enable with `use time;`. Covers one type for absolute instants on the
wall-clock timeline (`time.Time`) and one for signed spans
(`time.Duration`), with constructors from Unix integers, calendar
accessors, and arithmetic / comparison helpers.

This page covers the `time` library surface. A benchmark
example uses `time.now()` to measure elapsed time.
IANA zone names and daylight-saving transitions are not part
of the library yet - it ships fixed-offset zones only. IANA /
DST support is planned as a Go-backed extension to this
library (delegating to the host tz database), not a
hand-maintained data map.

```jennifer
use io;
use time;

def start as time.Time init time.now();
# ... do work ...
def gap as time.Duration init time.sub(time.now(), $start);
io.printf("took %d ms\n", time.milliseconds($gap));
```

## Types

The library registers two namespaced structs at install time. Field
names exist because Jennifer structs have no privacy mechanism, but
the conventional API is the function set below - prefer
`time.unix($t)` over `$t.nanos / 1000000000`, since the field shape
can change between milestones.

```jennifer
def struct time.Time { nanos as int, offset as int };
def struct time.Duration { nanos as int };
def struct time.Zone { offset as int, name as string };
```

- `time.Time.nanos` - UTC nanoseconds since the Unix epoch
  (1970-01-01T00:00:00Z). Fits in `int` (Go `int64`) for any year
  between 1678 and 2262.
- `time.Time.offset` - seconds east of UTC. The calendar accessors
  use this to compute wall-clock parts.
- `time.Duration.nanos` - signed nanosecond span. Subtracting a
  later time from an earlier one produces a negative duration.
- `time.Zone.offset` - seconds east of UTC (`3600` for CET,
  `-28800` for PST, `0` for UTC). Capped at +/- 26 hours to catch
  obvious mistakes.
- `time.Zone.name` - display string (e.g. `"CET"`, `"UTC"`).
  Free-form; the empty string is allowed but `%z` and the ISO
  formatter use the numeric offset regardless of the name.

## Constructors

| Call                              | Returns         | Notes                                                                            |
| --------------------------------- | --------------- | -------------------------------------------------------------------------------- |
| `time.now()`                      | `time.Time`     | Current instant in the host's local zone.                                        |
| `time.utc()`                      | `time.Time`     | Current instant in UTC (offset = 0).                                             |
| `time.fromUnix(seconds)`          | `time.Time`     | At UTC. `seconds` is `int`.                                                      |
| `time.fromUnixMillis(ms)`         | `time.Time`     | At UTC. `ms` is `int`.                                                           |
| `time.fromUnixNanos(ns)`          | `time.Time`     | At UTC. `ns` is `int`.                                                           |
| `time.fromIso(s)`                 | `time.Time`     | Parse an RFC 3339 string. Accepts `Z` or `+HH:MM`, optional fractional seconds.  |
| `time.parse(s, layout)`           | `time.Time`     | Parse using a strftime layout. See [Format codes](#format-codes) below.          |
| `time.fromSeconds(n)`             | `time.Duration` | Span of `n` seconds.                                                             |
| `time.fromMilliseconds(n)`        | `time.Duration` | Span of `n` milliseconds.                                                        |
| `time.fromMinutes(n)`             | `time.Duration` | Span of `n` minutes.                                                             |
| `time.fromHours(n)`               | `time.Duration` | Span of `n` hours.                                                               |
| `time.zone(offset, name)`         | `time.Zone`     | Fixed-offset zone constructor. `offset` is seconds east of UTC.                  |
| `time.local()`                    | `time.Zone`     | Host's current zone (name + offset). The only OS-zone read in the library.      |

All scalar arguments are strict: a non-int / non-string argument is
a positioned runtime error. There is no float-seconds form; pass
milliseconds or nanoseconds when sub-second precision is needed.

Two constants live alongside the constructors:

- **`time.UTC`** (= `Zone{offset: 0, name: "UTC"}`) - the canonical
  UTC zone. Coexists with the `time.utc()` function (which returns
  the *current* instant in UTC) because constants and functions
  live in separate namespace maps.
- **`time.PROGRAM_START`** (`time.Time`) - the moment the time
  library was installed, which for the `jennifer` and
  `jennifer-tiny` binaries is just before the user's source file is
  read. Use it to anchor "total elapsed since program start"
  timing without scattering a `def start = time.now();` line at
  the top of every script. See
  [Measuring total runtime](#measuring-total-runtime) below.

## Accessors

### Unix integer accessors

| Call                  | Returns | Notes                                                       |
| --------------------- | ------- | ----------------------------------------------------------- |
| `time.unix($t)`       | `int`   | Unix seconds since 1970-01-01T00:00:00Z (truncates toward zero). |
| `time.unixMillis($t)` | `int`   | Unix milliseconds.                                          |
| `time.unixNanos($t)`  | `int`   | Unix nanoseconds (no loss of precision).                    |

### Calendar accessors

All take a `time.Time` and return `int`. The wall-clock parts honour
the time's stored `offset`.

| Call                   | Range            | Notes                                                                |
| ---------------------- | ---------------- | -------------------------------------------------------------------- |
| `time.year($t)`        | full year        | E.g. `2024`. No two-digit year form.                                 |
| `time.month($t)`       | 1-12             | January = 1.                                                         |
| `time.day($t)`         | 1-31             | Day of month.                                                        |
| `time.hour($t)`        | 0-23             | 24-hour clock.                                                       |
| `time.minute($t)`      | 0-59             |                                                                      |
| `time.second($t)`      | 0-59             | Whole seconds; sub-second precision lives in `nanosecond` / `unixNanos`. |
| `time.nanosecond($t)`  | 0-999_999_999    | Fractional second.                                                   |
| `time.weekday($t)`     | 1-7 (ISO 8601)   | Monday = 1, Sunday = 7 (not Go's 0-based).                           |

### Duration accessors

All take a `time.Duration` and return `int`, truncating toward zero
at the requested unit.

| Call                       | Notes                                |
| -------------------------- | ------------------------------------ |
| `time.seconds($d)`         | Whole seconds in the span.           |
| `time.milliseconds($d)`    | Whole milliseconds.                  |
| `time.minutes($d)`         | Whole minutes.                       |
| `time.hours($d)`           | Whole hours.                         |

## Arithmetic and comparison

| Call                       | Returns         | Notes                                                  |
| -------------------------- | --------------- | ------------------------------------------------------ |
| `time.add($t, $d)`         | `time.Time`     | `$t` shifted by `$d`. Preserves the time's offset.     |
| `time.sub($a, $b)`         | `time.Duration` | Signed: negative when `$a` is earlier than `$b`.       |
| `time.before($a, $b)`      | `bool`          | Strictly earlier (UTC instant compare).                |
| `time.after($a, $b)`       | `bool`          | Strictly later.                                        |
| `time.equal($a, $b)`       | `bool`          | Same UTC instant (the stored `offset` is ignored).     |
| `time.sleep($d)`           | `null`          | Block the running task for the requested duration. Negative or zero `$d` returns immediately. |

Comparison is on the underlying UTC instant, so `13:00 CET` and
`12:00 UTC` compare equal. The library has no operator overloading
in v1: write `time.add($t, $d)`, not `$t + $d`.

## Measuring total runtime

`time.PROGRAM_START` is captured before the interpreter reads the
user source, so anchoring elapsed-time against it gives "total
runtime since the program launched" with no per-script setup.
Subtract the current instant from it and read the duration at
whatever unit you want:

```jennifer
use io;
use time;

# ... script body ...

def elapsed as time.Duration init time.sub(time.now(), time.PROGRAM_START);
io.printf("ran for %d ms\n", time.milliseconds($elapsed));
```

For per-section timing inside the script, take a local snapshot
of `time.now()` and subtract that instead:

```jennifer
def stepStart as time.Time init time.now();
# ... one step of the workload ...
def stepMs as int init time.milliseconds(time.sub(time.now(), $stepStart));
io.printf("step took %d ms\n", $stepMs);
```

`time.PROGRAM_START` is a constant - reading it multiple times
always returns the same instant, so it's safe as a long-lived
anchor without ever needing to "refresh" it.

## Pausing execution

`time.sleep($d)` blocks the running task for the requested
duration. Pair it with the duration constructors:

```jennifer
use time;
time.sleep(time.fromMilliseconds(250));   # 250 ms pause
time.sleep(time.fromSeconds(2));          # 2 second pause
```

Negative or zero durations return immediately - safe to use with
a computed "remaining budget" without checking the sign:

```jennifer
def deadline as time.Time init time.add(time.now(), time.fromSeconds(5));
# ... some work that may already overshoot ...
def remaining as time.Duration init time.sub($deadline, time.now());
time.sleep($remaining);   # no-op if work already took longer than 5s
```

`time.sleep` returns `null`; the caller who wants the wake-up
instant calls `time.now()` after it returns. With concurrency,
sleep blocks one task instead of the whole interpreter.

## Zones

A `time.Zone` is a fixed offset plus a display name; the core
library never resolves an IANA name like `"Europe/Vienna"` to an
offset, because that resolution depends on date (DST) and on
tzdata the interpreter doesn't ship. To shift a `time.Time` into a
different wall-clock view, build a `Zone` with the offset you
want, then call `time.inZone`:

```jennifer
def t as time.Time init time.now();
def vienna as time.Zone init time.zone(3600, "CET");
def tv as time.Time init time.inZone($t, $vienna);
# $tv represents the same UTC instant as $t, but calendar
# accessors and formatters now report wall-clock parts for CET.
```

| Call                       | Returns       | Notes                                                                    |
| -------------------------- | ------------- | ------------------------------------------------------------------------ |
| `time.zone(offset, name)`  | `time.Zone`   | `offset` in seconds east of UTC; capped at +/- 26h.                      |
| `time.inZone($t, $z)`      | `time.Time`   | Re-render `$t` in `$z`'s wall-clock. UTC instant is preserved.           |
| `time.local()`             | `time.Zone`   | Host's current zone. Reads `time.Now().Zone()` once.                     |
| `time.UTC` (constant)      | `time.Zone`   | `Zone{offset: 0, name: "UTC"}`. Canonical UTC.                           |

DST-aware and IANA-named zones (`"Europe/Vienna"`) are not
resolved today. They are planned as a **Go-backed extension** to
this library - delegating to the host tz database
(`time.LoadLocation`) for historically correct DST - rather than
a hand-maintained data map. Until then, build a fixed-offset
`Zone` for the offset you need.

## Formatting and parsing

`time.format` and `time.parse` use a strftime-style layout. The
codes below cover the v1 set; everything outside this list is a
positioned error.

```jennifer
def t as time.Time init time.fromUnix(1718454896);
def s as string init time.format($t, "%Y-%m-%dT%H:%M:%S%z");
# s = "2024-06-15T12:34:56+0000"
def back as time.Time init time.parse($s, "%Y-%m-%dT%H:%M:%S%z");
```

### Format codes

| Code | Meaning                                  | Format output    | Parse expectation       |
| ---- | ---------------------------------------- | ---------------- | ----------------------- |
| `%Y` | 4-digit year                             | `2024`           | exactly 4 digits        |
| `%m` | Month 01-12                              | `06`             | exactly 2 digits, 1..12 |
| `%d` | Day of month 01-31                       | `15`             | exactly 2 digits, 1..31 |
| `%H` | Hour 00-23                               | `12`             | exactly 2 digits, 0..23 |
| `%M` | Minute 00-59                             | `34`             | exactly 2 digits, 0..59 |
| `%S` | Second 00-59                             | `56`             | exactly 2 digits, 0..59 |
| `%z` | UTC offset                               | `+0000`, `+0100` | `+HHMM`, `-HHMM`, or `Z` (lenient) |
| `%a` | Short weekday (English)                  | `Sat`            | 3 letters, case-insensitive (informational, doesn't set the date) |
| `%A` | Long weekday (English)                   | `Saturday`       | full name, case-insensitive (informational) |
| `%b` | Short month name (English)               | `Jun`            | 3 letters, case-insensitive (sets the month) |
| `%B` | Long month name (English)                | `June`           | full name, case-insensitive (sets the month) |
| `%j` | Day of year 001-366                      | `167`            | format-only in v1       |
| `%u` | ISO weekday 1-7 (Mon=1, Sun=7)           | `6`              | format-only in v1       |
| `%%` | Literal `%`                              | `%`              | matches `%` in input    |

Codes not listed (e.g. `%I`, `%p`, `%y`, `%e`) are reserved and
error if used in a layout - the v1 set deliberately stays small
and adds only when a use case appears.

Missing parts default to year 1970, month 1, day 1, all the
time-of-day at zero, offset 0 (UTC). Trailing input after the
layout consumed errors with a positioned message.

### `time.iso` / `time.fromIso`

ISO 8601 / RFC 3339 round-trip without needing a layout. The
output uses `Z` for UTC and `+HH:MM` for any other offset;
fractional seconds appear only when non-zero, with trailing zeros
trimmed.

```jennifer
def t as time.Time init time.utc();
io.printf("%s\n", time.iso($t));         # 2024-06-15T12:34:56Z
def parsed as time.Time init time.fromIso("2024-06-15T13:34:56+01:00");
```

`time.fromIso` accepts both `Z` and `+HH:MM` (or `-HH:MM`), with
optional fractional seconds of up to 9 digits.

## Errors

The boundary checks are uniform:

- Wrong argument count: `time.now expects 0 arguments, got 1`,
  `time.fromUnix expects 1 argument (seconds), got 2`.
- Wrong scalar type:
  `time.fromUnix: seconds must be int, got float`.
- Wrong struct type:
  `time.seconds: argument must be a time.Duration, got struct`.
- Format / parse layout errors carry the offending verb or
  position: `time.format: unknown format verb %Q at position 4`,
  `time.parse: %m: month 13 out of range 1..12`.

All errors are positioned at the call site.

## See also

- [milestones.md](../milestones.md) - formatting, parsing,
  fixed-offset zones, `examples/benchmark.j`, and the planned
  Go-backed IANA / DST extension.
- [imports.md](../user-guide/imports.md) - the library catalog.
- [io.md](io.md) - `io.printf` for displaying results.
