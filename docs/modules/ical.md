# `ical` - iCalendar (RFC 5545) build and parse

Import with `import "ical.j" as ical;`. Build a calendar of events and encode it
to **iCalendar** text (a `VCALENDAR` of `VEVENT`s), and parse that text back into
a `Calendar`. Pure Jennifer over `strings` / `lists` + `time`; runs on both
binaries.

```jennifer
import "ical.j" as ical;
use time;

def ev as ical.Event init ical.event(
    "launch@team",
    time.fromIso("2024-06-20T14:00:00Z"),
    time.fromIso("2024-06-20T15:30:00Z"),
    "Product launch");
def cal as ical.Calendar init ical.add(ical.calendar(), $ev);
def text as string init ical.encode($cal);   # BEGIN:VCALENDAR ... END:VCALENDAR
```

Runnable: [`examples/modules/ical_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/ical_demo.j).

## Types

The structs have public fields (`$cal.events`, `$ev.summary`); construct them
with the builder functions.

```jennifer
def struct ical.Calendar { prodid as string, events as list of Event, todos as list of Todo };
def struct ical.Event {
    uid as string, stamp as time.Time,    # UID / DTSTAMP
    start as time.Time, end as time.Time, # DTSTART / DTEND
    summary as string, description as string, location as string,
    allDay as bool,                       # VALUE=DATE all-day event
    tzid as string,                       # TZID ("" = UTC)
    rrule as string,                      # RRULE ("" = non-recurring)
    rdates as list of time.Time,          # RDATE
    exdates as list of time.Time,         # EXDATE
    organizer as string,                  # ORGANIZER
    attendees as list of Attendee,        # ATTENDEE
    alarms as list of Alarm               # VALARM
};
def struct ical.Attendee { address as string, cn as string, role as string };
def struct ical.Alarm { action as string, trigger as string, description as string };
def struct ical.Todo {
    uid as string, stamp as time.Time, summary as string,
    due as time.Time, hasDue as bool, description as string, status as string
};
def struct ical.IcalError { kind as string, message as string, line as int };
```

## Building

| Call | Returns | |
| ---- | ------- | - |
| `ical.calendar()` | `Calendar` | an empty calendar with the default `PRODID` |
| `ical.calendarWith(prodid)` | `Calendar` | an empty calendar with a custom `PRODID` |
| `ical.event(uid, start, end, summary)` | `Event` | an event; `DTSTAMP` defaults to `start` |
| `ical.describe(ev, description)` / `locate(ev, location)` | `Event` | set the description / location |
| `ical.withAllDay(ev, bool)` / `withZone(ev, tzid)` | `Event` | mark all-day / set a `TZID` zone |
| `ical.recur(ev, rrule)` | `Event` | set the `RRULE` (build one with `ical.rule(freq, interval, count)`) |
| `ical.addRdate(ev, t)` / `addExdate(ev, t)` | `Event` | add an extra / excluded instant |
| `ical.withOrganizer(ev, addr)` / `addAttendee(ev, ical.attendee(addr, cn, role))` | `Event` | set organizer / add an attendee |
| `ical.addAlarm(ev, ical.alarm(action, trigger, description))` | `Event` | add a `VALARM` |
| `ical.occurrences(ev, max)` | `list of time.Time` | expand the recurrence into up to `max` instants |
| `ical.todo(uid, stamp, summary)` + `withDue` / `withStatus` / `describeTodo` | `Todo` | build a to-do |
| `ical.add(cal, ev)` / `addTodo(cal, td)` | `Calendar` | a copy with the event / to-do appended |

The builders are value-semantic: `describe` / `locate` / `add` return a fresh
copy and never mutate their argument, so thread the result:

```jennifer
import "ical.j" as ical;
use time;

def start as time.Time init time.fromIso("2024-06-20T14:00:00Z");
def end as time.Time init time.add($start, time.fromHours(1));
def ev as ical.Event init ical.event("id", $start, $end, "Meeting");
$ev = ical.describe($ev, "agenda...");
$ev = ical.locate($ev, "Room 5");
def cal as ical.Calendar init ical.add(ical.calendar(), $ev);
```

## Encoding and parsing

| Call | Returns | |
| ---- | ------- | - |
| `ical.encode(cal)` | `string` | the calendar as RFC 5545 text (CRLF-terminated) |
| `ical.parse(text)` | `Calendar` | parse iCalendar text back into a calendar |

`parse(encode(cal))` round-trips the data. `encode` writes CRLF line endings,
escapes text values, folds long lines, and emits `DESCRIPTION` / `LOCATION` only
when set. `parse` unfolds folded lines, reads the relevant property parameters
(`VALUE=DATE`, `TZID`, `CN` / `ROLE`), reads `VEVENT` / `VTODO` (and nested
`VALARM`) components, skips a `VTIMEZONE` (and any other component), unescapes
text, and skips a `VEVENT` with no `DTSTART`. An event's end is its `DTEND`, or
`DTSTART` + `DURATION` (RFC 5545 3.3.6: `P1W`, `P1DT2H`, `PT1H30M`, ...); with
neither, a DATE event lasts one day and a DATE-TIME event ends at its start.
Input holding several `VCALENDAR`s parses into one calendar.

## Errors

`parse` is strict: malformed input throws an `ical.IcalError` (kind `"ical"`)
instead of being skipped or repaired. Its `line` is the 1-based physical line
the fault is on (a folded line counts from its first line), 0 when the fault is
not tied to one; the `message` starts `line N: `. It throws for:

- no `BEGIN:VCALENDAR` (empty or junk input), a property outside it, a line
  without a `:`;
- an `END` that does not close the innermost open `BEGIN`, a `BEGIN` left
  open, a nested `VCALENDAR`, a `VEVENT` / `VTODO` inside another component;
- a backslash in a TEXT value (`SUMMARY`, `DESCRIPTION`, `LOCATION`, `UID`,
  `PRODID`) that is not one of the escapes `\\` `\;` `\,` `\n` `\N`;
- a date of the wrong shape or an impossible one (February 31, hour 25), a
  `VALUE=DATE` value that is not a bare date, `VALUE=PERIOD` in `RDATE`;
- a malformed or negative `DURATION`, `DTEND` together with `DURATION`, a
  `DTEND` before `DTSTART` or of another value type;
- a malformed `RRULE` (see [Recurrence](#recurrence)).

`occurrences` throws an `IcalError` (line 0) for a malformed or unsupported
`RRULE` and for a series that runs past the representable time range, and a
generic `"limit"` error when reaching `max` takes more than 100000 recurrence
steps.

```jennifer
import "ical.j" as ical;
use io;

try {
    ical.parse("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:a\r\nDTSTART:20240231T090000Z\r\n" +
        "END:VEVENT\r\nEND:VCALENDAR\r\n");
} catch (e) {
    if ($e.kind == "ical") {
        io.printf("%s\n", $e.message); # line 4: DTSTART: "20240231T090000Z" is not a valid date or time
    }
}
```

## Dates and times

`DTSTAMP` / `DTSTART` / `DTEND` go through `time`. `encode` writes each as a UTC
`DATE-TIME` (`20240615T130000Z`), normalising a non-UTC `time.Time` to UTC first,
so the output is always a `Z` value. `parse` accepts the UTC `...Z` form and a
floating `DATE-TIME` (no `Z`, read as UTC); a bare `DATE` (`20240615`) needs the
`VALUE=DATE` parameter, as RFC 5545 requires.

## Text escaping and folding

Text values (`SUMMARY` / `DESCRIPTION` / `LOCATION` / `UID` / `PRODID`) use RFC
5545 escaping: a backslash, `;`, `,`, and any newline become `\\`, `\;`, `\,`,
and `\n`. Content lines longer than 75 octets are folded onto continuation
lines (a CRLF followed by a space), and `parse` rejoins them.

## Recurrence

An event carries a raw `RRULE` string plus `RDATE` / `EXDATE` instant lists.
`ical.rule(freq, interval, count)` builds a simple rule (`"FREQ=WEEKLY;COUNT=10"`);
`ical.occurrences(ev, max)` expands the recurrence set into its first `max`
instants, sorted and without duplicates: `DTSTART` (always part of the set), the
`RRULE` series from it (`FREQ` `SECONDLY` / `MINUTELY` / `HOURLY` / `DAILY` /
`WEEKLY` / `MONTHLY` / `YEARLY`, `INTERVAL`, `COUNT`, `UNTIL`), and the `RDATE`s,
minus the `EXDATE`s. Each instance is computed from `DTSTART`; a `MONTHLY` /
`YEARLY` instance whose date does not exist (January 31 + 1 month, February 29
in a common year) is skipped and does not count towards `COUNT`, as RFC 5545
3.3.10 requires - it is never moved to the month's last day.

```jennifer
import "ical.j" as ical;
use io;
use time;

def ev as ical.Event init ical.recur(
    ical.event("pay", time.fromIso("2024-01-31T09:00:00Z"), time.fromIso("2024-01-31T09:30:00Z"), "Pay"),
    "FREQ=MONTHLY;COUNT=3");
for (def t in ical.occurrences($ev, 10)) {
    io.printf("%s\n", time.iso($t)); # 2024-01-31, 2024-03-31, 2024-05-31 (09:00:00Z)
}
```

`parse` checks an `RRULE`'s syntax: `KEY=VALUE` parts, each key once, a known
`FREQ`, a positive `INTERVAL` / `COUNT`, an `UNTIL` of `DTSTART`'s value type
and not together with `COUNT`, a weekday `WKST`. The `BY*` parts (`BYDAY`,
`BYMONTH`, ...) are accepted and round-tripped in the string, but `occurrences`
does not expand them and throws for a rule holding one rather than return the
wrong instants.

## Scope

- **Fixed-offset time zones.** A `TZID` event stores its wall clock as a floating
  value paired with the zone name and round-trips exactly, but the instant is not
  converted through the named IANA zone (the `time` library ships fixed-offset
  zones only). A `VTIMEZONE` is **parsed and skipped**, not interpreted; add one
  yourself for a strict consumer. All-day (`VALUE=DATE`) events are supported.
- **`VEVENT` + `VTODO`.** `VJOURNAL` / `VFREEBUSY` are not modelled. `VALARM`s are
  read into an event's `alarms`; other nested components are skipped.
- **Fold width in octets.** Long lines fold at 75 octets on rune boundaries,
  never splitting a multi-byte character.

## See also

- [time.md](../libraries/time.md) - the instant / duration types the dates use.
- [strings.md](../libraries/strings.md) - the text surface the codec is built on.
- [modules/index.md](index.md) - the module catalog and import rules.
