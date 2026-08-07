# `ical` - iCalendar (RFC 5545) build and parse

Import with `import "ical.j" as ical;`. Build a calendar of events and encode it
to **iCalendar** text (a `VCALENDAR` of `VEVENT`s), and parse that text back into
a `Calendar`. Pure Jennifer over `strings` / `lists` + `time` - no Go engine, so
it runs on **both** binaries.

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

Both structs have public fields (read them directly - `$cal.events`,
`$ev.summary`); the builder functions are the conventional way to construct them.

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

The builders are **value-semantic** - `describe` / `locate` / `add` return a
fresh copy and never mutate their argument, so you thread them:

```jennifer
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
`VALARM`) components, skips a `VTIMEZONE`, unescapes text, skips a `VEVENT` with
no `DTSTART`, and defaults a missing `DTEND` to the start.

## Dates and times

`DTSTAMP` / `DTSTART` / `DTEND` go through `time`. `encode` writes each as a UTC
`DATE-TIME` (`20240615T130000Z`), normalising a non-UTC `time.Time` to UTC first,
so the output is always a correct `Z` value. `parse` accepts the UTC `...Z` form,
a floating `DATE-TIME` (no `Z`, read as UTC), and a bare `DATE` (`20240615`).

## Text escaping and folding

Text values (`SUMMARY` / `DESCRIPTION` / `LOCATION` / `UID` / `PRODID`) use RFC
5545 escaping: a backslash, `;`, `,`, and any newline become `\\`, `\;`, `\,`,
and `\n`. Content lines longer than 75 characters are folded onto continuation
lines (a CRLF followed by a space), and `parse` rejoins them - so a long
description survives the round-trip intact.

## Recurrence

An event carries a raw `RRULE` string plus `RDATE` / `EXDATE` instant lists.
`ical.rule(freq, interval, count)` builds a simple rule (`"FREQ=WEEKLY;COUNT=10"`);
`ical.occurrences(ev, max)` expands it into concrete instants - the `RRULE` series
from `DTSTART` (honouring `FREQ` / `INTERVAL` / `COUNT` / `UNTIL`, with `MONTHLY` /
`YEARLY` day-clamping computed from `DTSTART`), plus `RDATE`s, minus `EXDATE`s.
The `BY*` parts (`BYDAY`, `BYMONTH`, ...) are round-tripped in the string but not
expanded (the base cadence is still produced).

## Scope

- **Fixed-offset time zones.** A `TZID` event stores its wall clock as a floating
  value paired with the zone name and round-trips exactly, but the instant is not
  converted through the named IANA zone (the `time` library ships fixed-offset
  zones only). A `VTIMEZONE` is **parsed and skipped**, not interpreted; add one
  yourself for a strict consumer. All-day (`VALUE=DATE`) events are supported.
- **`VEVENT` + `VTODO`.** `VJOURNAL` / `VFREEBUSY` are not modelled. `VALARM`s are
  read into an event's `alarms`; other nested components are skipped.
- **Fold width in characters.** Long lines fold on rune boundaries at 75
  characters (never splitting a multi-byte character), rather than strictly on
  75 octets - valid output that every reader unfolds.

## See also

- [time.md](../libraries/time.md) - the instant / duration types the dates use.
- [strings.md](../libraries/strings.md) - the text surface the codec is built on.
- [modules/index.md](index.md) - the module catalog and import rules.
