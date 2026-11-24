# `time` - instants, durations, and arithmetic

Enable with `use time;`. One type for absolute instants (`time.Time`),
one for signed spans (`time.Duration`), and fixed-offset zones
(`time.Zone`), with constructors, calendar accessors, arithmetic /
comparison, formatting and parsing. IANA zone names and DST are not
supported (see [Zones](#zones)).

```jennifer
use io;
use time;

def start as time.Time init time.now();
# ... do work ...
def gap as time.Duration init time.sub(time.now(), $start);
io.printf("took %d ms\n", time.milliseconds($gap));
```

## Types

The library registers these namespaced structs. Their fields are
visible (Jennifer structs have no privacy), but use the functions
below - `time.unix($t)` rather than `$t.nanos / 1000000000` - since
the field shape can change between releases.

```text
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
  later time from an earlier one produces a negative duration. A duration is an
  int64 nanosecond count, so a constructor given a count too large to represent
  (e.g. `time.fromSeconds` of ~9.2e9 or more), and a `time.sub` whose difference
  overflows, raise a catchable error instead of wrapping, like the language's
  integer arithmetic.
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
| `time.fromUnixMilliseconds(ms)`   | `time.Time`     | At UTC. `ms` is `int`.                                                           |
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
  UTC zone. It coexists with the `time.utc()` function (the *current*
  instant in UTC) because constants and functions live in separate
  namespace maps.
- **`time.PROGRAM_START`** (`time.Time`) - the moment the time
  library was installed, just before the user's source file is read.
  Use it to time the whole program without a `time.now()` snapshot at
  the top. See
  [Measuring total runtime](#measuring-total-runtime) below.

## Accessors

### Unix integer accessors

| Call                        | Returns | Notes                                                                     |
| --------------------------- | ------- | ------------------------------------------------------------------------- |
| `time.unix($t)`             | `int`   | Unix seconds since 1970-01-01T00:00:00Z, floored (pre-epoch -1 ms -> -1). |
| `time.unixMilliseconds($t)` | `int`   | Unix milliseconds. Floored.                                               |
| `time.unixNanos($t)`        | `int`   | Unix nanoseconds (no loss of precision).                                  |

### Deprecated

Still working; `jennifer lint` reports a call (L301).

| Call                      | Use instead                 | Removed in |
| ------------------------- | --------------------------- | ---------- |
| `time.fromUnixMillis(ms)` | `time.fromUnixMilliseconds` | 0.29.0     |
| `time.unixMillis($t)`     | `time.unixMilliseconds`     | 0.29.0     |

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
| `time.sleep($d)`           | `null`          | Block the running task for the requested duration. A zero `$d` returns at once; a negative one throws `"runtime"`. |

Comparison is on the underlying UTC instant, so `13:00 CET` and
`12:00 UTC` compare equal. The library has no operator overloading:
write `time.add($t, $d)`, not `$t + $d`.

## Measuring total runtime

`time.PROGRAM_START` is captured before the interpreter reads the
user source, so subtracting it from the current instant gives the
total runtime:

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

`time.PROGRAM_START` is a constant and always returns the same
instant.

## Pausing execution

`time.sleep($d)` blocks the running task for the requested
duration:

```jennifer
use time;
time.sleep(time.fromMilliseconds(250));   # 250 ms pause
time.sleep(time.fromSeconds(2));          # 2 second pause
```

A zero duration returns at once; a negative one throws `"runtime"`, as a
negative wait does everywhere, so a computed "remaining budget" checks its
sign:

```jennifer
def deadline as time.Time init time.add(time.now(), time.fromSeconds(5));
# ... some work that may already overshoot ...
def remaining as time.Duration init time.sub($deadline, time.now());
if (time.milliseconds($remaining) > 0) {
    time.sleep($remaining);
}
```

`time.sleep` returns `null`; call `time.now()` afterwards for the
wake-up instant. Under `spawn`, sleep blocks one task, not the whole
interpreter, and `task.cancel` ends the sleep early with a catchable
"task cancelled".

## Zones

A `time.Zone` is a fixed offset plus a display name; the library
does not resolve an IANA name like `"Europe/Vienna"`, since that
depends on the date (DST) and on tzdata the interpreter does not
ship. To view a `time.Time` in another wall-clock, build a `Zone`
with the wanted offset and call `time.inZone`:

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

DST-aware, IANA-named zones are a [horizon](../horizon.md#library-completions)
idea (backed by Go's `time.LoadLocation`); build a fixed-offset `Zone`.

## Formatting and parsing

`time.format` and `time.parse` use a strftime-style layout. The
codes below are the whole set; everything outside this list is a
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
| `%I` | Hour 01-12 (12-hour clock)               | `01`             | exactly 2 digits, 1..12; combine with `%p` |
| `%p` | Meridiem `AM` / `PM`                     | `PM`             | `AM`/`PM`, case-insensitive; selects the half for `%I` |
| `%M` | Minute 00-59                             | `34`             | exactly 2 digits, 0..59 |
| `%S` | Second 00-59                             | `56`             | exactly 2 digits, 0..59 |
| `%z` | UTC offset                               | `+0000`, `+0100` | `+HHMM`, `-HHMM`, or `Z` |
| `%a` | Short weekday (English)                  | `Sat`            | 3 letters, case-insensitive (informational, does not set the date) |
| `%A` | Long weekday (English)                   | `Saturday`       | full name, case-insensitive (informational) |
| `%b` | Short month name (English)               | `Jun`            | 3 letters, case-insensitive (sets the month) |
| `%B` | Long month name (English)                | `June`           | full name, case-insensitive (sets the month) |
| `%j` | Day of year 001-366                      | `167`            | format-only             |
| `%u` | ISO weekday 1-7 (Mon=1, Sun=7)           | `6`              | format-only             |
| `%%` | Literal `%`                              | `%`              | matches `%` in input    |

Codes not listed (e.g. `%y`, `%e`) are reserved and error if used
in a layout.

### Number width: `pad`

The numeric codes are zero-padded to a default width: `%m %d %H %I %M %S`
to 2, `%j` to 3, `%Y` to 4. Each takes `io.printf`'s modifier syntax with one
modifier, `pad`, the minimum width: `pad=0` prints the number unpadded, a
larger `pad` widens it with zeros.

```jennifer
def d as time.Time init time.parse("2027-11-05", "%Y-%m-%d");
io.printf("%s\n", time.format($d, "%A, %B %d|pad=0, %Y"));   # Friday, November 5, 2027
io.printf("%s\n", time.format($d, "%d|pad=0.%m|pad=0.%Y"));  # 5.11.2027
io.printf("%s\n", time.format($d, "%j|pad=0 %Y|pad=6"));     # 309 002027
```

The [modifier rules](io.md#format-modifiers) are printf's: a `|` right after a code
always starts a modifier list (write `||` for a literal `|` there), a bare
value ends at the first character outside `[A-Za-z0-9_]` (so the `,` above
ends `pad=0`), and an unknown modifier, a second `pad`, or a modifier on a
code that takes none (`%A %B %a %b %p %z %u`) is an error. A `pad` above 4096
is an error of kind `"limit"`.

`time.parse` reads the same layouts: a code with `pad=0` reads one digit up to
its default width (greedily, so `%H|pad=0%M` reads `123` as 12:3), and a `pad`
above the default reads exactly that many digits. A code without `pad` keeps
its exact width.

**Weekday and month names are English only.** `%a` / `%A` / `%b` /
`%B` render (and parse) the English names regardless of the process
locale; `time` ships no locale data (CLDR). For a regional name, read
the numeric field (`time.month($t)` 1..12, `time.weekday($t)` 1..7 ISO,
or the `%m` / `%u` verbs) and index a name table.

Missing parts default to year 1970, month 1, day 1, all the
time-of-day at zero, offset 0 (UTC). Trailing input after the
layout consumed errors with a positioned message.

### `time.iso` / `time.fromIso`

ISO 8601 / RFC 3339 round-trip without a layout. The
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

## Errors and messages

### Errors

| Kind | Raised for |
|---|---|
| `"time"` | an input string in `parse` / `fromIso` that does not match the layout or RFC 3339, names an impossible date, or lies outside the representable range |
| `"limit"` | a `pad` above 4096 |
| `"runtime"` | misuse: a wrong argument count or type, a malformed layout (unknown code, bad modifier), an offset past the 26 h cap, arithmetic or `fromUnix*` past the representable range, a negative `sleep` |

### Messages

The boundary checks are uniform:

| Case | Message |
| --- | --- |
| Wrong argument count | `time.now expects 0 arguments, got 1`, `time.fromUnix expects 1 argument (seconds), got 2` |
| Wrong scalar type | `time.fromUnix: seconds must be int, got float` |
| Wrong struct type | `time.seconds: argument must be a time.Duration, got struct` |
| Offset past the 26 h cap (also a hand-built `time.Time` or `time.Zone`) | `time.zone: offset 99999 is outside +/-93600 seconds (~26h cap)` |
| Layout errors carry the offending verb or position | `time.format: unknown format verb %Q at position 3` |
| Input errors carry the verb or position | `time.parse: %m: month 13 out of range 1..12` |
| `pad` above 4096, kind `"limit"` | `time.format: %d: pad=5000 exceeds the maximum 4096` |

All errors are positioned at the call site.

## See also


- [imports.md](../user-guide/imports.md) - the library catalog.
- [io.md](io.md) - `io.printf` for displaying results.
