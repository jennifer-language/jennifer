# `cron` - cron schedules

Import with `import "cron.j" as cron;`. Parses and evaluates **cron expressions**,
the five-field `minute hour day-of-month month day-of-week` spec. `parse` builds
a `Schedule`, `matches` tests whether a `time.Time` fires it, and `next` finds
the next fire at or after a time. A pure calculator over `time` (no clock, no
sleeping), so it runs on both binaries; a scheduler is your own `spawn` +
`time.sleep` loop over `cron.next`.

```jennifer
use io;
use time;
import "cron.j" as cron;

def s as cron.Schedule init cron.parse("30 9 * * 1-5");   # 09:30 on weekdays
def fire as time.Time init cron.next($s, time.now());
io.printf("next run: %s\n", time.iso($fire));
```

Runnable: [`examples/modules/cron_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/cron_demo.j).

## Functions

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `cron.parse(expr)` | `Schedule` | Parse a five-field expression. |
| `cron.matches(schedule, t)` | `bool` | Whether the schedule fires at `t` (minute granularity; seconds are ignored). |
| `cron.next(schedule, after)` | `time.Time` | The next fire at or after `after`, keeping its zone offset. |

`parse` returns a `Schedule`:

| Field | Type | Meaning |
| ----- | ---- | ------- |
| `minutes` | `list of int` | allowed minutes (0-59) |
| `hours` | `list of int` | allowed hours (0-23) |
| `daysOfMonth` | `list of int` | allowed days of the month (1-31) |
| `months` | `list of int` | allowed months (1-12) |
| `weekdays` | `list of int` | allowed ISO weekdays (1-7, Monday = 1; cron's `0` and `7` both become 7) |
| `domStar` | `bool` | the day-of-month field starts with `*` (`*` or `*/n`; drives the rule [below](#the-day-of-month--day-of-week-rule)) |
| `dowStar` | `bool` | the day-of-week field starts with `*` |
| `reboot` | `bool` | `true` for an `@reboot` schedule |

## Fields

Five whitespace-separated fields, each with the usual operators (a number may
have leading zeros; one past the field's range throws kind `"cron"`):

| Field | Range | |
| ----- | ----- | - |
| minute | 0-59 | |
| hour | 0-23 | |
| day of month | 1-31 | |
| month | 1-12 | |
| day of week | 0-7 | `0` and `7` are both Sunday |

Each field accepts `*` (every value), a single number, an `a-b` range, an
`a,b,c` list, and a `/n` step: on a wildcard (every `n`th value), a range
(`a-b/n`), or a value (`a/n`, meaning `a` to the field maximum). Examples:

| Expression | Fires |
| ---------- | ----- |
| `* * * * *` | every minute |
| `*/15 * * * *` | every 15 minutes |
| `0 9 * * 1-5` | 09:00 on weekdays (Mon-Fri) |
| `0 0 1 * *` | midnight on the 1st of each month |
| `30 3 * * 0` | 03:30 on Sundays |
| `0 0 13 * 5` | midnight on the 13th and on every Friday (see below) |

### Named months and weekdays

The month field also accepts the three-letter names `JAN FEB MAR APR MAY JUN JUL
AUG SEP OCT NOV DEC` (as 1-12) and the day-of-week field accepts `SUN MON TUE WED
THU FRI SAT` (as 0-6). Names are **case-insensitive** (`jan`, `Jan`, `JAN` all
work) and can appear anywhere a number can - single values, ranges, and lists:

| Expression | Same as |
| ---------- | ------- |
| `0 9 * * MON-FRI` | `0 9 * * 1-5` (09:00 on weekdays) |
| `0 0 1 JAN,JUL *` | `0 0 1 1,7 *` (1st of January and July) |
| `30 3 * * SUN` | `30 3 * * 0` (03:30 on Sundays) |

Like the numeric `0`, `SUN` is Sunday (there is no name for the alternate `7`).

### Nickname macros

A whole expression may instead be one of the standard `@` nicknames, which expand
to a five-field expression before parsing:

| Nickname | Expands to | Fires |
| -------- | ---------- | ----- |
| `@yearly`, `@annually` | `0 0 1 1 *` | midnight on January 1st |
| `@monthly` | `0 0 1 * *` | midnight on the 1st of each month |
| `@weekly` | `0 0 * * 0` | midnight each Sunday |
| `@daily`, `@midnight` | `0 0 * * *` | midnight every day |
| `@hourly` | `0 * * * *` | the top of every hour |

Nicknames are case-insensitive. An unknown `@name` throws kind `"cron"`.

### `@reboot`

`@reboot` means "at startup", so it has **no time-based schedule**. It parses to a
`Schedule` whose `reboot` field is `true`, and that flag changes the two evaluators:

- `cron.matches($s, t)` is **always `false`** for a `@reboot` schedule (no clock
  time ever fires it).
- `cron.next($s, after)` **throws** kind `"cron"`: there is no next fire time.

Test for it yourself and run the job once at program start:

```jennifer
use io;
import "cron.j" as cron;

func runJob() {
    io.printf("job ran\n");
}

def s as cron.Schedule init cron.parse("@reboot");
if ($s.reboot) {
    runJob();   # fire once, at startup
}
```

### The day-of-month / day-of-week rule

When **both** the day-of-month and day-of-week fields are restricted (neither
starts with `*`), a day matching **either** one fires (standard cron
behavior). So `0 0 13 * 5` fires on the 13th *and* on every Friday, not only on
Friday the 13th. When one of the two starts with `*`, a day must match **both**
(as in Vixie cron and cronie): a plain `*` matches every day, so only the other
field constrains, while a step such as `*/2` still restricts its own field.
`0 0 */2 * *` fires on odd days of the month, and `0 0 */2 * 1` only on Mondays
that fall on an odd day.

## `next`

`cron.next(schedule, after)` returns the first matching minute at or after
`after` (with its seconds zeroed), preserving the input's zone offset. If `after`
already sits exactly on a matching minute, it is returned. The search skips
non-matching days and hours whole and gives up after a nine-year horizon (a
Feb-29-only schedule can wait eight years across 2100): an impossible schedule
(e.g. `0 0 31 2 *`, February 31st) throws kind `"cron"` rather than looping
forever.

Zones are fixed-offset (as in the `time` library), so `next` does no DST
arithmetic.

## Scope

- **Standard five fields**, plus named months / weekdays and the `@` nickname
  macros (including `@reboot`) above. No seconds field and no non-standard
  extensions (`L`, `W`, `#`, `?`).
- **A calculator, not a runner.** It never touches the clock. Drive it yourself:
  `time.sleep(time.sub(cron.next($s, time.now()), time.now()))`, then run the job.

## See also

- [time.md](../libraries/time.md) - the `time.Time` cron computes over.
- [concurrency.md](../user-guide/concurrency.md) - `spawn` for a background scheduler loop.
- [modules/index.md](index.md) - the module catalog and import rules.

## API reference

Every exported function, struct and constant of `cron`, with its signature, generated
from the module's doc comments: [the `cron` API reference](../api/cron.md).
