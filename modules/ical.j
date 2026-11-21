# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

# A hand-rolled iCalendar parser: its parse dispatch legitimately runs past the
# L201 statement-count limit. Every other lint check stays active.
# lint-disable-file: L201

/**
 * Build and parse iCalendar (RFC 5545): a `Calendar` holding `Event`s
 * (`VEVENT`) and `Todo`s (`VTODO`), encoded to a `VCALENDAR` and parsed back.
 * Pure Jennifer over `strings` / `lists` + `time` - no Go, no system engine.
 *
 * Beyond the basics (`UID` / `SUMMARY` / `DESCRIPTION` / `LOCATION`), an event
 * supports **recurrence** (`RRULE` / `RDATE` / `EXDATE`, with an `occurrences`
 * expander for `FREQ` / `INTERVAL` / `COUNT` / `UNTIL`), `DURATION`, **all-day** dates
 * (`VALUE=DATE`) and a named-zone **`TZID`**, an `ORGANIZER` and `ATTENDEE`s, and
 * `VALARM` alarms. Times go through `time`: UTC values write as
 * `20240615T130000Z`; a TZID event stores its wall clock as a floating value
 * paired with the zone name (because `time` models fixed-offset zones only, add a
 * matching `VTIMEZONE` for a strict consumer). Text values are RFC 5545-escaped
 * and content lines folded at 75 octets, so `parse(encode(cal))` round-trips;
 * `parse` throws an `IcalError` for malformed input.
 * @module ical
 * @example
 * import "ical.j" as ical;
 * use time;
 * def ev as ical.Event init ical.recur(
 *     ical.event("a@b", time.utc(), time.utc(), "Standup"), ical.rule("WEEKLY", 1, 10));
 * def cal as ical.Calendar init ical.add(ical.calendar(), $ev);
 * def text as string init ical.encode($cal);
 */
use encoding;
use strings;
use time;
use convert;
use lists;
use maps;

# The vCard / iCalendar content-line codec (TEXT escaping, 75-char folding, the
# name / value split, `emit`) is shared with vcard.j via this include.
include "ical_vcard_shared.inc.j";

/**
 * A malformed calendar or recurrence rule. Every error of kind `"ical"` is one,
 * so its fields are safe to read once the kind matches.
 * @field kind {string} always `"ical"`
 * @field message {string} what is wrong, prefixed `line N: ` when tied to a line
 * @field line {int} the 1-based physical line of `parse`'s input the fault is
 *     on (a folded line counts from its first physical line); 0 when the fault is
 *     not tied to a line (an `occurrences` rule, input with no `VCALENDAR`)
 * @example
 * import "ical.j" as ical;
 * use io;
 * try {
 *     ical.parse("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nDTSTART:2024\r\n");
 * } catch (e) {
 *     if ($e.kind == "ical") {
 *         io.printf("bad calendar at line %d: %s\n", $e.line, $e.message);
 *     }
 * }
 */
export def struct IcalError {
    kind as string,
    message as string,
    line as int
};

/**
 * An event attendee (`ATTENDEE`).
 * @field address {string} the calendar address (e.g. `mailto:bob@example.com`)
 * @field cn {string} the common name (`CN` parameter; "" when unset)
 * @field role {string} the participation role (`ROLE`; e.g. "REQ-PARTICIPANT"; "" when unset)
 */
export def struct Attendee {
    address as string,
    cn as string,
    role as string
};

/**
 * An event alarm (`VALARM`).
 * @field action {string} the `ACTION` ("DISPLAY" / "AUDIO" / "EMAIL")
 * @field trigger {string} the `TRIGGER` value (e.g. `-PT15M` = 15 minutes before)
 * @field description {string} the `DESCRIPTION` (the reminder text; "" when unset)
 */
export def struct Alarm {
    action as string,
    trigger as string,
    description as string
};

/**
 * A to-do item (a `VTODO`).
 * @field uid {string} the globally-unique `UID`
 * @field stamp {time.Time} the `DTSTAMP`
 * @field summary {string} the `SUMMARY`
 * @field due {time.Time} the `DUE` instant (valid only when `hasDue`)
 * @field hasDue {bool} whether a `DUE` is set
 * @field description {string} the `DESCRIPTION` ("" when unset)
 * @field status {string} the `STATUS` (e.g. "NEEDS-ACTION" / "COMPLETED"; "" when unset)
 */
export def struct Todo {
    uid as string,
    stamp as time.Time,
    summary as string,
    due as time.Time,
    hasDue as bool,
    description as string,
    status as string
};

/**
 * A calendar: a product identifier, its events, and its to-dos.
 * @field prodid {string} the `PRODID` product identifier
 * @field events {list of Event} the calendar's events
 * @field todos {list of Todo} the calendar's to-dos (`VTODO`s)
 */
export def struct Calendar {
    prodid as string,
    events as list of Event,
    todos as list of Todo
};

/**
 * A single calendar event (a `VEVENT`).
 * @field uid {string} the globally-unique `UID`
 * @field stamp {time.Time} the `DTSTAMP` (creation / last-modified instant)
 * @field start {time.Time} the `DTSTART` start instant
 * @field end {time.Time} the `DTEND` end instant
 * @field summary {string} the `SUMMARY` (title)
 * @field description {string} the `DESCRIPTION` ("" when unset)
 * @field location {string} the `LOCATION` ("" when unset)
 * @field allDay {bool} whether the event is an all-day event (`VALUE=DATE`)
 * @field tzid {string} the `TZID` time-zone name for a local `DTSTART` / `DTEND` ("" = UTC)
 * @field rrule {string} the `RRULE` recurrence rule value ("" when non-recurring)
 * @field rdates {list of time.Time} extra recurrence instants (`RDATE`)
 * @field exdates {list of time.Time} excluded recurrence instants (`EXDATE`)
 * @field organizer {string} the `ORGANIZER` calendar address ("" when unset)
 * @field attendees {list of Attendee} the `ATTENDEE`s
 * @field alarms {list of Alarm} the event's `VALARM`s
 */
export def struct Event {
    uid as string,
    stamp as time.Time,
    start as time.Time,
    end as time.Time,
    summary as string,
    description as string,
    location as string,
    allDay as bool,
    tzid as string,
    rrule as string,
    rdates as list of time.Time,
    exdates as list of time.Time,
    organizer as string,
    attendees as list of Attendee,
    alarms as list of Alarm
};

# --- constructors (exported) ------------------------------------------------

/**
 * A calendar with the default Jennifer `PRODID` and no events.
 * @return {Calendar} the empty calendar
 */
export func calendar() {
    def evs as list of Event init [];
    def tds as list of Todo init [];
    return Calendar{prodid: "-//Jennifer//ical//EN", events: $evs, todos: $tds};
}

/**
 * A calendar with a caller-supplied `PRODID`.
 * @param prodid {string} the product identifier
 * @return {Calendar} the empty calendar
 */
export func calendarWith(prodid as string) {
    def evs as list of Event init [];
    def tds as list of Todo init [];
    return Calendar{prodid: $prodid, events: $evs, todos: $tds};
}

/**
 * An event. `DTSTAMP` defaults to the start; the optional fields
 * (`DESCRIPTION` / `LOCATION` / recurrence / organizer / attendees / alarms) are
 * empty until set with the `describe` / `locate` / `recur` / ... builders.
 * @param uid {string} the unique identifier
 * @param start {time.Time} the start instant
 * @param end {time.Time} the end instant
 * @param summary {string} the title
 * @return {Event} the event
 */
export func event(uid as string, start as time.Time, end as time.Time, summary as string) {
    def rdates as list of time.Time init [];
    def exdates as list of time.Time init [];
    def attendees as list of Attendee init [];
    def alarms as list of Alarm init [];
    return Event{
        uid: $uid,
        stamp: $start,
        start: $start,
        end: $end,
        summary: $summary,
        description: "",
        location: "",
        allDay: false,
        tzid: "",
        rrule: "",
        rdates: $rdates,
        exdates: $exdates,
        organizer: "",
        attendees: $attendees,
        alarms: $alarms
    };
}

/**
 * A copy of the event with its description set (value-semantic).
 * @param ev {Event} the event
 * @param description {string} the description text
 * @return {Event} a fresh event with the description set
 */
export func describe(ev as Event, description as string) {
    $ev.description = $description;
    return $ev;
}

/**
 * A copy of the event with its location set (value-semantic).
 * @param ev {Event} the event
 * @param location {string} the location text
 * @return {Event} a fresh event with the location set
 */
export func locate(ev as Event, location as string) {
    $ev.location = $location;
    return $ev;
}

/**
 * A copy of the calendar with an event appended (value-semantic).
 * @param cal {Calendar} the calendar
 * @param ev {Event} the event to add
 * @return {Calendar} a fresh calendar with the event appended
 */
export func add(cal as Calendar, ev as Event) {
    $cal.events[] = $ev;
    return $cal;
}

/**
 * A copy of the event marked (or unmarked) as an all-day event. All-day
 * `DTSTART` / `DTEND` encode as `VALUE=DATE` (a bare `YYYYMMDD`).
 * @param ev {Event} the event
 * @param isAllDay {bool} whether the event is all-day
 * @return {Event} a fresh event with the all-day flag set
 */
export func withAllDay(ev as Event, isAllDay as bool) {
    $ev.allDay = $isAllDay;
    return $ev;
}

/**
 * A copy of the event with its time-zone id (`TZID`) set, so `DTSTART` / `DTEND`
 * encode as local time in that zone (`DTSTART;TZID=America/New_York:...`) instead
 * of UTC. The named zone is preserved verbatim; add a matching `VTIMEZONE` for a
 * strict consumer.
 * @param ev {Event} the event
 * @param tzid {string} the IANA time-zone name (e.g. "Europe/London"); "" restores UTC
 * @return {Event} a fresh event with the time zone set
 */
export func withZone(ev as Event, tzid as string) {
    $ev.tzid = $tzid;
    return $ev;
}

/**
 * A copy of the event with a recurrence rule (`RRULE`). Pass a raw RRULE value
 * (e.g. `"FREQ=WEEKLY;COUNT=10"`) or build one with `rule`.
 * @param ev {Event} the event
 * @param rrule {string} the RRULE value
 * @return {Event} a fresh recurring event
 */
export func recur(ev as Event, rrule as string) {
    $ev.rrule = $rrule;
    return $ev;
}

/**
 * Build a simple `RRULE` value from a frequency, interval, and count.
 * `INTERVAL` is omitted when 1 and `COUNT` when 0 (i.e. unbounded).
 * @param freq {string} the frequency ("DAILY" / "WEEKLY" / "MONTHLY" / "YEARLY")
 * @param interval {int} the interval (every N periods; 1 = every period)
 * @param count {int} the number of occurrences (0 = unbounded)
 * @return {string} the RRULE value
 */
export func rule(freq as string, interval as int, count as int) {
    def s as string init "FREQ=" + strings.upper($freq);
    if ($interval > 1) {
        $s = $s + ";INTERVAL={$interval}";
    }
    if ($count > 0) {
        $s = $s + ";COUNT={$count}";
    }
    return $s;
}

/**
 * A copy of the event with an extra recurrence instant (`RDATE`) appended.
 * @param ev {Event} the event
 * @param t {time.Time} the extra instant
 * @return {Event} a fresh event with the RDATE added
 */
export func addRdate(ev as Event, t as time.Time) {
    $ev.rdates[] = $t;
    return $ev;
}

/**
 * A copy of the event with an excluded recurrence instant (`EXDATE`) appended.
 * @param ev {Event} the event
 * @param t {time.Time} the excluded instant
 * @return {Event} a fresh event with the EXDATE added
 */
export func addExdate(ev as Event, t as time.Time) {
    $ev.exdates[] = $t;
    return $ev;
}

/**
 * A copy of the event with its `ORGANIZER` set.
 * @param ev {Event} the event
 * @param address {string} the calendar address (e.g. `mailto:a@example.com`)
 * @return {Event} a fresh event with the organizer set
 */
export func withOrganizer(ev as Event, address as string) {
    $ev.organizer = $address;
    return $ev;
}

/**
 * An attendee.
 * @param address {string} the calendar address (e.g. `mailto:bob@example.com`)
 * @param cn {string} the common name ("" for none)
 * @param role {string} the role ("" for none; e.g. "REQ-PARTICIPANT")
 * @return {Attendee} the attendee
 */
export func attendee(address as string, cn as string, role as string) {
    return Attendee{address: $address, cn: $cn, role: $role};
}

/**
 * A copy of the event with an `ATTENDEE` appended.
 * @param ev {Event} the event
 * @param a {Attendee} the attendee
 * @return {Event} a fresh event with the attendee added
 */
export func addAttendee(ev as Event, a as Attendee) {
    $ev.attendees[] = $a;
    return $ev;
}

/**
 * An alarm (`VALARM`).
 * @param action {string} the action ("DISPLAY" / "AUDIO" / "EMAIL")
 * @param trigger {string} the trigger value (e.g. `-PT15M`)
 * @param description {string} the reminder text ("" for none)
 * @return {Alarm} the alarm
 */
export func alarm(action as string, trigger as string, description as string) {
    return Alarm{action: $action, trigger: $trigger, description: $description};
}

/**
 * A copy of the event with a `VALARM` appended.
 * @param ev {Event} the event
 * @param a {Alarm} the alarm
 * @return {Event} a fresh event with the alarm added
 */
export func addAlarm(ev as Event, a as Alarm) {
    $ev.alarms[] = $a;
    return $ev;
}

# --- to-dos (VTODO) ---------------------------------------------------------

/**
 * A to-do (`VTODO`). `DTSTAMP` defaults to `stamp`; `DUE` / `STATUS` /
 * `DESCRIPTION` are set with the builders.
 * @param uid {string} the unique identifier
 * @param stamp {time.Time} the `DTSTAMP`
 * @param summary {string} the title
 * @return {Todo} the to-do
 */
export func todo(uid as string, stamp as time.Time, summary as string) {
    return Todo{
        uid: $uid,
        stamp: $stamp,
        summary: $summary,
        due: $stamp,
        hasDue: false,
        description: "",
        status: ""
    };
}

/**
 * A copy of the to-do with its `DUE` instant set.
 * @param td {Todo} the to-do
 * @param due {time.Time} the due instant
 * @return {Todo} a fresh to-do with the due date set
 */
export func withDue(td as Todo, due as time.Time) {
    $td.due = $due;
    $td.hasDue = true;
    return $td;
}

/**
 * A copy of the to-do with its `STATUS` set (e.g. "NEEDS-ACTION" / "COMPLETED").
 * @param td {Todo} the to-do
 * @param status {string} the status
 * @return {Todo} a fresh to-do with the status set
 */
export func withStatus(td as Todo, status as string) {
    $td.status = $status;
    return $td;
}

/**
 * A copy of the to-do with its `DESCRIPTION` set.
 * @param td {Todo} the to-do
 * @param description {string} the description
 * @return {Todo} a fresh to-do with the description set
 */
export func describeTodo(td as Todo, description as string) {
    $td.description = $description;
    return $td;
}

/**
 * A copy of the calendar with a to-do appended.
 * @param cal {Calendar} the calendar
 * @param td {Todo} the to-do to add
 * @return {Calendar} a fresh calendar with the to-do appended
 */
export func addTodo(cal as Calendar, td as Todo) {
    $cal.todos[] = $td;
    return $cal;
}

# --- date-time (private) ----------------------------------------------------

# valueColon returns the index of the value-separating colon, scanning the
# name/parameter section while tracking double-quote state so a colon inside a
# quoted parameter value is not mistaken for the separator. Returns -1 when
# there is no unquoted colon.
func valueColon(line as string) {
    # Fast path: no quote before the first colon (the usual line).
    def first as int init strings.indexOf($line, ":");
    if ($first < 0) {
        return -1;
    }
    if (not strings.contains($line[..$first], "\"")) {
        return $first;
    }
    def cs as list of string init strings.chars($line);
    def inQuote as bool init false;
    def i as int init 0;
    while ($i < len($cs)) {
        def ch as string init $cs[$i];
        if ($ch == "\"") {
            $inQuote = not $inQuote;
        } elseif ($ch == ":" and not $inQuote) {
            return $i;
        }
        $i = $i + 1;
    }
    return -1;
}

# formatDateTime renders an instant as a UTC iCalendar DATE-TIME (`...Z`),
# normalising to UTC first so a non-UTC time.Time still emits a correct value.
func formatDateTime(t as time.Time) {
    def u as time.Time init time.inZone($t, time.UTC);
    return time.format($u, "%Y%m%dT%H%M%SZ");
}

# formatDate renders an instant as a bare iCalendar DATE (`YYYYMMDD`), for an
# all-day value.
func formatDate(t as time.Time) {
    return time.format(time.inZone($t, time.UTC), "%Y%m%d");
}

# formatFloating renders an instant as a "floating" DATE-TIME (no `Z`), the wall
# clock a `TZID` value carries. Because `time` models fixed-offset zones only
# (no IANA / DST), a TZID event stores the wall-clock components as the instant's
# UTC fields and pairs them with the zone name; this round-trips exactly.
func formatFloating(t as time.Time) {
    return time.format(time.inZone($t, time.UTC), "%Y%m%dT%H%M%S");
}

# emitDt renders a DTSTART / DTEND line for an event: `VALUE=DATE` for all-day, a
# `TZID`-parameter local value for a zoned event, else the UTC `...Z` form.
func emitDt(name as string, t as time.Time, ev as Event) {
    if ($ev.allDay) {
        return emitLine($name + ";VALUE=DATE", formatDate($t));
    }
    if (not ($ev.tzid == "")) {
        return emitLine($name + ";TZID=" + quoteParam($ev.tzid), formatFloating($t));
    }
    return emitLine($name, formatDateTime($t));
}

# joinDates renders a list of instants as a comma-separated UTC DATE-TIME value
# (for RDATE / EXDATE).
func joinDates(ts as list of time.Time) {
    def parts as list of string init [];
    for (def t in $ts) {
        $parts[] = formatDateTime($t);
    }
    return strings.join($parts, ",");
}

# --- encode (exported) ------------------------------------------------------

# attendeeName builds the `ATTENDEE` name section with its CN / ROLE parameters.
func attendeeName(a as Attendee) {
    def s as string init "ATTENDEE";
    if (not ($a.cn == "")) {
        $s = $s + ";CN=" + quoteParam($a.cn);
    }
    if (not ($a.role == "")) {
        $s = $s + ";ROLE=" + quoteParam($a.role);
    }
    return $s;
}

# encodeEvent renders one VEVENT (with recurrence / organizer / attendees /
# alarms) as a fresh list of lines; `encode` appends them in place, so a
# calendar with many events is not re-copied per event (O(E^2)).
func encodeEvent(ev as Event) {
    def lines as list of string init [];
    $lines[] = "BEGIN:VEVENT";
    $lines[] = emitLine("UID", escapeText($ev.uid));
    $lines[] = emitLine("DTSTAMP", formatDateTime($ev.stamp));
    $lines[] = emitDt("DTSTART", $ev.start, $ev);
    $lines[] = emitDt("DTEND", $ev.end, $ev);
    $lines[] = emitLine("SUMMARY", escapeText($ev.summary));
    if (not ($ev.description == "")) {
        $lines[] = emitLine("DESCRIPTION", escapeText($ev.description));
    }
    if (not ($ev.location == "")) {
        $lines[] = emitLine("LOCATION", escapeText($ev.location));
    }
    if (not ($ev.rrule == "")) {
        $lines[] = emitLine("RRULE", $ev.rrule);
    }
    if (len($ev.rdates) > 0) {
        $lines[] = emitLine("RDATE", joinDates($ev.rdates));
    }
    if (len($ev.exdates) > 0) {
        $lines[] = emitLine("EXDATE", joinDates($ev.exdates));
    }
    if (not ($ev.organizer == "")) {
        $lines[] = emitLine("ORGANIZER", $ev.organizer);
    }
    for (def a in $ev.attendees) {
        $lines[] = emitLine(attendeeName($a), $a.address);
    }
    for (def al in $ev.alarms) {
        $lines[] = "BEGIN:VALARM";
        $lines[] = emitLine("ACTION", $al.action);
        $lines[] = emitLine("TRIGGER", $al.trigger);
        if (not ($al.description == "")) {
            $lines[] = emitLine("DESCRIPTION", escapeText($al.description));
        }
        $lines[] = "END:VALARM";
    }
    $lines[] = "END:VEVENT";
    return $lines;
}

# encodeTodo renders one VTODO as a fresh list of lines.
func encodeTodo(td as Todo) {
    def lines as list of string init [];
    $lines[] = "BEGIN:VTODO";
    $lines[] = emitLine("UID", escapeText($td.uid));
    $lines[] = emitLine("DTSTAMP", formatDateTime($td.stamp));
    $lines[] = emitLine("SUMMARY", escapeText($td.summary));
    if ($td.hasDue) {
        $lines[] = emitLine("DUE", formatDateTime($td.due));
    }
    if (not ($td.status == "")) {
        $lines[] = emitLine("STATUS", $td.status);
    }
    if (not ($td.description == "")) {
        $lines[] = emitLine("DESCRIPTION", escapeText($td.description));
    }
    $lines[] = "END:VTODO";
    return $lines;
}

/**
 * Render a calendar to iCalendar text (RFC 5545): a `VCALENDAR` wrapping one
 * `VEVENT` per event (and a `VTODO` per to-do), CRLF line endings, escaped text,
 * folded long lines. Optional properties (`DESCRIPTION` / `LOCATION` /
 * recurrence / organizer / attendees / alarms) are emitted only when set.
 * @param cal {Calendar} the calendar to encode
 * @return {string} the iCalendar text (CRLF-terminated)
 */
export func encode(cal as Calendar) {
    def lines as list of string init [];
    $lines[] = "BEGIN:VCALENDAR";
    $lines[] = "VERSION:2.0";
    $lines[] = emitLine("PRODID", escapeText($cal.prodid));
    for (def ev in $cal.events) {
        def evl as list of string init encodeEvent($ev);
        for (def l in $evl) {
            $lines[] = $l;
        }
    }
    for (def td in $cal.todos) {
        def tdl as list of string init encodeTodo($td);
        for (def l in $tdl) {
            $lines[] = $l;
        }
    }
    $lines[] = "END:VCALENDAR";
    return strings.join($lines, "\r\n") + "\r\n";
}

# --- parse (exported) -------------------------------------------------------

# icalError builds the IcalError for a fault at a physical line (0 = not tied
# to a line of input), prefixing the message with the line number.
func icalError(line as int, message as string) {
    def m as string init $message;
    if ($line > 0) {
        $m = "line {$line}: {$message}";
    }
    return IcalError{kind: "ical", message: $m, line: $line};
}

# isDateValue reports whether a property's `VALUE` parameter marks a DATE.
func isDateValue(nameSection as string) {
    return strings.upper(paramValue($nameSection, "VALUE")) == "DATE";
}

# allDigits reports whether a non-empty string holds ASCII digits only.
func allDigits(s as string) {
    if ($s == "") {
        return false;
    }
    for (def c in strings.chars($s)) {
        if ($c < "0" or $c > "9") {
            return false;
        }
    }
    return true;
}

# parseDt parses a DATE (`20240615`, when `isDate`) or DATE-TIME
# (`20240615T130000Z`, or floating without the `Z`, read as UTC) value. A value
# of the wrong shape, or an impossible date or time (February 31, hour 25), is
# an IcalError naming the property.
func parseDt(v as string, isDate as bool, line as int, prop as string) {
    def layout as string init "%Y%m%dT%H%M%S";
    def ok as bool init false;
    if ($isDate) {
        $layout = "%Y%m%d";
        $ok = len($v) == 8 and allDigits($v);
    } elseif (len($v) == 16 and strings.endsWith($v, "Z")) {
        $layout = "%Y%m%dT%H%M%SZ";
        $ok = allDigits($v[..8]) and $v[8..9] == "T" and allDigits($v[9..15]);
    } elseif (len($v) == 15) {
        $ok = allDigits($v[..8]) and $v[8..9] == "T" and allDigits($v[9..]);
    }
    if (not $ok) {
        def want as string init "a DATE-TIME (YYYYMMDDTHHMMSS[Z])";
        if ($isDate) {
            $want = "a DATE (YYYYMMDD)";
        }
        throw icalError($line, "{$prop}: \"{$v}\" is not {$want}");
    }
    try {
        return time.parse($v, $layout);
    } catch (e) {
        if (not ($e.kind == "runtime")) {
            throw $e;
        }
        throw icalError($line, "{$prop}: \"{$v}\" is not a valid date or time");
    }
}

# splitCommaDates parses a comma-separated DATE / DATE-TIME list (RDATE /
# EXDATE) into instants. An empty item is an error, and so is `VALUE=PERIOD`,
# which this module does not model.
func splitCommaDates(nameSection as string, value as string, line as int, prop as string) {
    def vt as string init strings.upper(paramValue($nameSection, "VALUE"));
    if (not ($vt == "" or $vt == "DATE" or $vt == "DATE-TIME")) {
        throw icalError($line, "{$prop}: VALUE={$vt} is not supported");
    }
    def out as list of time.Time init [];
    for (def v in strings.split($value, ",")) {
        $out[] = parseDt($v, $vt == "DATE", $line, $prop);
    }
    return $out;
}

# checkedText unescapes a TEXT value, throwing on a backslash that does not start
# a valid RFC 5545 escape.
func checkedText(value as string, line as int, prop as string) {
    def bad as int init badEscape($value);
    if ($bad >= 0) {
        def esc as string init $value[$bad..];
        if (len($esc) > 2) {
            $esc = $esc[..2];
        }
        throw icalError($line, "{$prop}: invalid escape \"{$esc}\"");
    }
    return unescapeText($value);
}

# durationNumber reads the digits of one DURATION element starting at `i`; it
# returns the index just past them, or the start index when there are none. A
# run of more than nine digits is rejected by the caller.
func durationDigitsEnd(cs as list of string, i as int) {
    def j as int init $i;
    while ($j < len($cs) and $cs[$j] >= "0" and $cs[$j] <= "9") {
        $j = $j + 1;
    }
    return $j;
}

# parseDuration parses an RFC 5545 3.3.6 DURATION (`[+|-]P` then `nW`, or `nD`
# with an optional time part, or a time part `T` `nH` [`nM` [`nS`]] / `nM` [`nS`]
# / `nS`) into signed seconds. A day counts as 86400 seconds (the module's
# instants are UTC). Anything off the grammar is an IcalError.
func parseDuration(v as string, line as int) {
    def cs as list of string init strings.chars($v);
    def n as int init len($cs);
    def i as int init 0;
    def sign as int init 1;
    def bad as IcalError init icalError($line, "DURATION: \"{$v}\" is not a valid duration");
    if ($i < $n and ($cs[$i] == "+" or $cs[$i] == "-")) {
        if ($cs[$i] == "-") {
            $sign = -1;
        }
        $i = $i + 1;
    }
    if ($i >= $n or not ($cs[$i] == "P")) {
        throw $bad;
    }
    $i = $i + 1;
    def total as int init 0;
    # The date part: nW (alone), or nD optionally followed by a time part.
    if ($i < $n and not ($cs[$i] == "T")) {
        def j as int init durationDigitsEnd($cs, $i);
        if ($j == $i or $j - $i > 9 or $j >= $n) {
            throw $bad;
        }
        def num as int init convert.toInt(strings.join($cs[$i..$j], ""));
        if ($cs[$j] == "W") {
            if (not ($j + 1 == $n)) {
                throw $bad;
            }
            return $sign * $num * 604800;
        }
        if (not ($cs[$j] == "D")) {
            throw $bad;
        }
        $total = $num * 86400;
        $i = $j + 1;
        if ($i == $n) {
            return $sign * $total;
        }
    }
    # The time part: T, then H / M / S in that order, each continuing the last.
    if ($i >= $n or not ($cs[$i] == "T")) {
        throw $bad;
    }
    $i = $i + 1;
    def units as list of string init ["H", "M", "S"];
    def mult as list of int init [3600, 60, 1];
    def last as int init -1;
    while ($i < $n) {
        def j as int init durationDigitsEnd($cs, $i);
        if ($j == $i or $j - $i > 9 or $j >= $n) {
            throw $bad;
        }
        def u as int init -1;
        for (def k in 0..3) {
            if ($cs[$j] == $units[$k]) {
                $u = $k;
            }
        }
        if ($u < 0 or ($last >= 0 and not ($u == $last + 1))) {
            throw $bad;
        }
        $total = $total + convert.toInt(strings.join($cs[$i..$j], "")) * $mult[$u];
        $last = $u;
        $i = $j + 1;
    }
    if ($last < 0) {
        throw $bad;
    }
    return $sign * $total;
}

# The raw properties of the VEVENT being read, with the lines they are on.
def struct EventAcc {
    uid as string,
    summary as string,
    description as string,
    location as string,
    startStr as string,
    startLine as int,
    allDay as bool,
    tzid as string,
    endStr as string,
    endLine as int,
    endIsDate as bool,
    durStr as string,
    durLine as int,
    stampStr as string,
    stampLine as int,
    rrule as string,
    rruleLine as int,
    rdates as list of time.Time,
    exdates as list of time.Time,
    organizer as string,
    attendees as list of Attendee,
    alarms as list of Alarm,
    alAction as string,
    alTrigger as string,
    alDesc as string
};

# The raw properties of the VTODO being read.
def struct TodoAcc {
    uid as string,
    summary as string,
    description as string,
    status as string,
    stampStr as string,
    stampLine as int,
    dueStr as string,
    dueLine as int,
    dueIsDate as bool
};

func newEventAcc() {
    def ts as list of time.Time init [];
    def at as list of Attendee init [];
    def am as list of Alarm init [];
    return EventAcc{
        uid: "",
        summary: "",
        description: "",
        location: "",
        startStr: "",
        startLine: 0,
        allDay: false,
        tzid: "",
        endStr: "",
        endLine: 0,
        endIsDate: false,
        durStr: "",
        durLine: 0,
        stampStr: "",
        stampLine: 0,
        rrule: "",
        rruleLine: 0,
        rdates: $ts,
        exdates: $ts,
        organizer: "",
        attendees: $at,
        alarms: $am,
        alAction: "",
        alTrigger: "",
        alDesc: ""
    };
}

func newTodoAcc() {
    return TodoAcc{
        uid: "",
        summary: "",
        description: "",
        status: "",
        stampStr: "",
        stampLine: 0,
        dueStr: "",
        dueLine: 0,
        dueIsDate: false
    };
}

# eventEnd computes an event's end from DTEND or DURATION: at most one may be
# set, DTEND must have DTSTART's value type and not precede it, and a DURATION
# must not be negative. With neither, a DATE event lasts one day and a
# DATE-TIME event ends at its start (RFC 5545 3.6.1).
func eventEnd(a as EventAcc, start as time.Time) {
    if (not ($a.endStr == "") and not ($a.durStr == "")) {
        def ln as int init $a.endLine;
        if ($a.durLine > $ln) {
            $ln = $a.durLine;
        }
        throw icalError($ln, "DTEND and DURATION are both set");
    }
    if (not ($a.endStr == "")) {
        if (not ($a.endIsDate == $a.allDay)) {
            throw icalError($a.endLine, "DTEND: value type differs from DTSTART's");
        }
        def end as time.Time init parseDt($a.endStr, $a.endIsDate, $a.endLine, "DTEND");
        if (time.before($end, $start)) {
            throw icalError($a.endLine, "DTEND is before DTSTART");
        }
        return $end;
    }
    def secs as int init 0;
    if (not ($a.durStr == "")) {
        $secs = parseDuration($a.durStr, $a.durLine);
        if ($secs < 0) {
            throw icalError($a.durLine, "DURATION is negative");
        }
    } elseif ($a.allDay) {
        $secs = 86400;
    }
    try {
        return time.add($start, time.fromSeconds($secs));
    } catch (e) {
        if (not ($e.kind == "runtime")) {
            throw $e;
        }
        throw icalError($a.durLine, "DURATION: the end is past the representable time range");
    }
}

# finishEvent builds the Event of a closed VEVENT.
func finishEvent(a as EventAcc) {
    def start as time.Time init parseDt($a.startStr, $a.allDay, $a.startLine, "DTSTART");
    def ev as Event init event($a.uid, $start, eventEnd($a, $start), $a.summary);
    $ev.description = $a.description;
    $ev.location = $a.location;
    if (not ($a.stampStr == "")) {
        $ev.stamp = parseDt($a.stampStr, false, $a.stampLine, "DTSTAMP");
    }
    $ev.allDay = $a.allDay;
    $ev.tzid = $a.tzid;
    $ev.rrule = $a.rrule;
    $ev.rdates = $a.rdates;
    $ev.exdates = $a.exdates;
    $ev.organizer = $a.organizer;
    $ev.attendees = $a.attendees;
    $ev.alarms = $a.alarms;
    if (not ($a.rrule == "")) {
        parseRule($ev, false, $a.rruleLine);
    }
    return $ev;
}

# finishTodo builds the Todo of a closed VTODO.
func finishTodo(a as TodoAcc) {
    def stamp as time.Time init time.utc();
    if (not ($a.stampStr == "")) {
        $stamp = parseDt($a.stampStr, false, $a.stampLine, "DTSTAMP");
    }
    def td as Todo init todo($a.uid, $stamp, $a.summary);
    $td.description = $a.description;
    $td.status = $a.status;
    if (not ($a.dueStr == "")) {
        $td.due = parseDt($a.dueStr, $a.dueIsDate, $a.dueLine, "DUE");
        $td.hasDue = true;
    }
    return $td;
}

# checkBegin validates a BEGIN against the open components: the outermost must
# be a VCALENDAR, which never nests, and a VEVENT / VTODO sits directly in it.
func checkBegin(stack as list of string, comp as string, line as int) {
    def depth as int init len($stack);
    if ($comp == "") {
        throw icalError($line, "BEGIN without a component name");
    }
    if ($depth == 0 and not ($comp == "VCALENDAR")) {
        throw icalError($line, "expected BEGIN:VCALENDAR, got BEGIN:{$comp}");
    }
    if ($depth > 0 and $comp == "VCALENDAR") {
        throw icalError($line, "VCALENDAR nested in {$stack[$depth - 1]}");
    }
    if ($depth > 1 and ($comp == "VEVENT" or $comp == "VTODO")) {
        throw icalError($line, "{$comp} nested in {$stack[$depth - 1]}");
    }
    return null;
}

/**
 * Parse iCalendar text into a `Calendar`. Unfolds folded lines and reads the
 * `PRODID`, each `VEVENT` (`UID` / `DTSTAMP` / `DTSTART` / `DTEND` / `DURATION` /
 * `SUMMARY` / `DESCRIPTION` / `LOCATION`, the all-day `VALUE=DATE` and `TZID`
 * parameters, `RRULE` / `RDATE` / `EXDATE`, `ORGANIZER` / `ATTENDEE`, and
 * nested `VALARM`s), and each `VTODO`. A `VTIMEZONE` and other components are
 * checked for structure and skipped. An event with no `DTSTART` is skipped; one
 * with neither `DTEND` nor `DURATION` ends at its start (a DATE start: one day
 * later).
 * @param text {string} the iCalendar text: one or more `VCALENDAR`s
 * @return {Calendar} the parsed calendar
 * @throws {IcalError} kind "ical" for malformed input: no `VCALENDAR`, a line
 *     without a `:`, a property outside a component, a missing or mismatched
 *     `END`, an invalid TEXT escape, a malformed or impossible date, a malformed
 *     `DURATION`, `DTEND` together with `DURATION`, or an end before the start
 */
export func parse(text as string) {
    def cal as Calendar init calendar();
    def events as list of Event init [];
    def todos as list of Todo init [];
    # The open components, outermost first, and the lines their BEGINs are on.
    def stack as list of string init [];
    def stackLines as list of int init [];
    def sawCalendar as bool init false;
    def ev as EventAcc init newEventAcc();
    def td as TodoAcc init newTodoAcc();

    for (def cl in contentLines($text)) {
        def line as string init $cl.text;
        def ln as int init $cl.line;
        if ($line == "") {
            continue;
        }
        def colon as int init valueColon($line);
        if ($colon < 0) {
            throw icalError($ln, "content line has no ':' separator");
        }
        if ($colon == 0) {
            throw icalError($ln, "content line has no property name");
        }
        def nameSection as string init $line[..$colon];
        def name as string init propName($nameSection);
        def value as string init $line[$colon + 1..];
        def depth as int init len($stack);

        if ($name == "BEGIN") {
            def comp as string init strings.upper($value);
            checkBegin($stack, $comp, $ln);
            $sawCalendar = true;
            if ($depth == 1 and $comp == "VEVENT") {
                $ev = newEventAcc();
            } elseif ($depth == 1 and $comp == "VTODO") {
                $td = newTodoAcc();
            } elseif ($depth == 2 and $stack[1] == "VEVENT" and $comp == "VALARM") {
                $ev.alAction = "";
                $ev.alTrigger = "";
                $ev.alDesc = "";
            }
            $stack[] = $comp;
            $stackLines[] = $ln;
            continue;
        }

        if ($name == "END") {
            def comp as string init strings.upper($value);
            if ($depth == 0) {
                throw icalError($ln, "END:{$comp} without a matching BEGIN");
            }
            def top as string init $stack[$depth - 1];
            if (not ($comp == $top)) {
                def at as int init $stackLines[$depth - 1];
                throw icalError($ln, "END:{$comp} does not close BEGIN:{$top} (line {$at})");
            }
            $stack = $stack[..$depth - 1];
            $stackLines = $stackLines[..$depth - 1];
            if ($depth == 3 and $stack[1] == "VEVENT" and $comp == "VALARM") {
                $ev.alarms[] = Alarm{
                    action: $ev.alAction,
                    trigger: $ev.alTrigger,
                    description: $ev.alDesc
                };
            } elseif ($depth == 2 and $comp == "VEVENT" and not ($ev.startStr == "")) {
                $events[] = finishEvent($ev);
            } elseif ($depth == 2 and $comp == "VTODO" and not ($td.uid == "" and
                $td.summary == "")) {
                $todos[] = finishTodo($td);
            }
            continue;
        }

        if ($depth == 0) {
            throw icalError($ln, "property {$name} outside BEGIN:VCALENDAR");
        }
        def top as string init $stack[$depth - 1];
        if ($depth == 1 and $name == "PRODID") {
            $cal.prodid = checkedText($value, $ln, $name);
        } elseif ($depth == 3 and $top == "VALARM" and $stack[1] == "VEVENT") {
            match ($name) {
                when "ACTION" { $ev.alAction = $value; }
                when "TRIGGER" { $ev.alTrigger = $value; }
                when "DESCRIPTION" { $ev.alDesc = checkedText($value, $ln, $name); }
            }
        } elseif ($depth == 2 and $top == "VEVENT") {
            match ($name) {
                when "UID" { $ev.uid = checkedText($value, $ln, $name); }
                when "SUMMARY" { $ev.summary = checkedText($value, $ln, $name); }
                when "DESCRIPTION" { $ev.description = checkedText($value, $ln, $name); }
                when "LOCATION" { $ev.location = checkedText($value, $ln, $name); }
                when "DTSTART" {
                    $ev.startStr = $value;
                    $ev.startLine = $ln;
                    $ev.allDay = isDateValue($nameSection);
                    $ev.tzid = paramValue($nameSection, "TZID");
                }
                when "DTEND" {
                    $ev.endStr = $value;
                    $ev.endLine = $ln;
                    $ev.endIsDate = isDateValue($nameSection);
                }
                when "DURATION" {
                    $ev.durStr = $value;
                    $ev.durLine = $ln;
                }
                when "DTSTAMP" {
                    $ev.stampStr = $value;
                    $ev.stampLine = $ln;
                }
                when "RRULE" {
                    $ev.rrule = $value;
                    $ev.rruleLine = $ln;
                }
                when "RDATE" {
                    $ev.rdates = lists.concat(
                        $ev.rdates,
                        splitCommaDates($nameSection, $value, $ln, $name));
                }
                when "EXDATE" {
                    $ev.exdates = lists.concat(
                        $ev.exdates,
                        splitCommaDates($nameSection, $value, $ln, $name));
                }
                when "ORGANIZER" { $ev.organizer = $value; }
                when "ATTENDEE" {
                    $ev.attendees[] = Attendee{
                        address: $value,
                        cn: paramValue($nameSection, "CN"),
                        role: paramValue($nameSection, "ROLE")
                    };
                }
            }
        } elseif ($depth == 2 and $top == "VTODO") {
            match ($name) {
                when "UID" { $td.uid = checkedText($value, $ln, $name); }
                when "SUMMARY" { $td.summary = checkedText($value, $ln, $name); }
                when "DESCRIPTION" { $td.description = checkedText($value, $ln, $name); }
                when "STATUS" { $td.status = $value; }
                when "DTSTAMP" {
                    $td.stampStr = $value;
                    $td.stampLine = $ln;
                }
                when "DUE" {
                    $td.dueStr = $value;
                    $td.dueLine = $ln;
                    $td.dueIsDate = isDateValue($nameSection);
                }
            }
        }
    }
    if (len($stack) > 0) {
        def last as string init $stack[len($stack) - 1];
        throw icalError($stackLines[len($stack) - 1], "BEGIN:{$last} has no matching END:{$last}");
    }
    if (not $sawCalendar) {
        throw icalError(0, "no BEGIN:VCALENDAR in the input");
    }
    $cal.events = $events;
    $cal.todos = $todos;
    return $cal;
}

# --- recurrence expansion (exported) ----------------------------------------

# The most recurrence steps `occurrences` takes before it throws "limit".
def const OCCURRENCE_LIMIT as int init 100000;

# A parsed and validated RRULE.
def struct Rule {
    freq as string,
    interval as int,
    count as int,
    hasUntil as bool,
    until as time.Time
};

# positiveRulePart parses an INTERVAL / COUNT value: 1 to 9 digits, at least 1.
func positiveRulePart(key as string, v as string, line as int) {
    if (not allDigits($v) or len($v) > 9 or convert.toInt($v) < 1) {
        throw icalError($line, "RRULE: {$key}={$v} is not a positive integer");
    }
    return convert.toInt($v);
}

# parseRule validates an event's RRULE: `KEY=VALUE` parts, each key once, a
# known FREQ, positive INTERVAL / COUNT, an UNTIL of the DTSTART's value type and
# never together with COUNT. `occurrences` does not expand the BY* parts, so
# for it (`expand`) they are rejected rather than ignored (ignoring one yields
# wrong instants); `parse` (`expand` false) accepts them for the round trip.
# `line` is the RRULE's input line (0 outside `parse`).
func parseRule(ev as Event, expand as bool, line as int) {
    def r as Rule init Rule{freq: "", interval: 1, count: 0, hasUntil: false, until: $ev.start};
    def seen as map of string to bool init {};
    for (def part in strings.split($ev.rrule, ";")) {
        def eq as int init strings.indexOf($part, "=");
        if ($eq <= 0 or $eq == len($part) - 1) {
            throw icalError($line, "RRULE: \"{$part}\" is not a KEY=VALUE part");
        }
        def key as string init strings.upper($part[..$eq]);
        def v as string init $part[$eq + 1..];
        if (maps.has($seen, $key)) {
            throw icalError($line, "RRULE: {$key} appears twice");
        }
        $seen[$key] = true;
        match ($key) {
            when "FREQ" {
                $r.freq = strings.upper($v);
                if (not lists.contains(
                    [
                        "SECONDLY",
                        "MINUTELY",
                        "HOURLY",
                        "DAILY",
                        "WEEKLY",
                        "MONTHLY",
                        "YEARLY"
                    ],
                    $r.freq)) {
                    throw icalError($line, "RRULE: FREQ={$v} is not a frequency");
                }
            }
            when "INTERVAL" { $r.interval = positiveRulePart($key, $v, $line); }
            when "COUNT" { $r.count = positiveRulePart($key, $v, $line); }
            when "UNTIL" {
                $r.hasUntil = true;
                $r.until = parseDt($v, $ev.allDay, $line, "RRULE UNTIL");
            }
            when "WKST" {
                # The week start matters only to BYDAY / BYWEEKNO, which are rejected.
                if (not lists.contains(
                    ["MO", "TU", "WE", "TH", "FR", "SA", "SU"],
                    strings.upper($v))) {
                    throw icalError($line, "RRULE: WKST={$v} is not a weekday");
                }
            }
            when "BYSECOND", "BYMINUTE", "BYHOUR", "BYDAY", "BYMONTHDAY", "BYYEARDAY",
                 "BYWEEKNO", "BYMONTH", "BYSETPOS" {
                if ($expand) {
                    throw icalError($line, "RRULE: {$key} is not supported by occurrences");
                }
            }
            else { throw icalError($line, "RRULE: unknown part {$key}"); }
        }
    }
    if ($r.freq == "") {
        throw icalError($line, "RRULE: FREQ is missing");
    }
    if ($r.count > 0 and $r.hasUntil) {
        throw icalError($line, "RRULE: COUNT and UNTIL are both set");
    }
    return $r;
}

func pad2(n as int) {
    def s as string init convert.toString($n);
    if (len($s) < 2) {
        return "0" + $s;
    }
    return $s;
}

func pad4(n as int) {
    def s as string init convert.toString($n);
    while (len($s) < 4) {
        $s = "0" + $s;
    }
    return $s;
}

# daysInMonth returns the number of days in month `m` of year `y` (leap-aware).
func daysInMonth(y as int, m as int) {
    if ($m == 2) {
        if (($y % 4 == 0 and not ($y % 100 == 0)) or $y % 400 == 0) {
            return 29;
        }
        return 28;
    }
    if ($m == 4 or $m == 6 or $m == 9 or $m == 11) {
        return 30;
    }
    return 31;
}

# buildUtc constructs a UTC instant from calendar components.
func buildUtc(y as int, mo as int, d as int, h as int, mi as int, s as int) {
    return time.parse(
        pad4($y) + pad2($mo) + pad2($d) + "T" + pad2($h) + pad2($mi) + pad2($s) + "Z",
        "%Y%m%dT%H%M%SZ");
}

# addMonthsUtc adds `n` calendar months to an instant, keeping the day of the
# month. It returns an empty list when that day does not exist in the target
# month (Jan 31 + 1 month): RFC 5545 3.3.10 drops such an invalid instance
# rather than moving it. YEARLY reuses this with n = 12 * years.
func addMonthsUtc(t as time.Time, n as int) {
    def out as list of time.Time init [];
    def u as time.Time init time.inZone($t, time.UTC);
    def total as int init (time.month($u) - 1) + $n;
    def ny as int init time.year($u) + $total // 12;
    def nmo as int init $total % 12 + 1;
    if (time.day($u) <= daysInMonth($ny, $nmo)) {
        $out[] = buildUtc($ny, $nmo, time.day($u), time.hour($u), time.minute($u), time.second($u));
    }
    return $out;
}

# occurrenceAt returns the n-th recurrence period's instance (0-based) as a list
# of zero or one instants, computed from `start` (not from the previous
# instance); empty when that period's date is invalid. A step past the
# representable time range is an IcalError.
func occurrenceAt(start as time.Time, freq as string, interval as int, n as int) {
    try {
        if ($freq == "MONTHLY") {
            return addMonthsUtc($start, $interval * $n);
        }
        if ($freq == "YEARLY") {
            return addMonthsUtc($start, 12 * $interval * $n);
        }
        def unit as int init 1;
        match ($freq) {
            when "MINUTELY" { $unit = 60; }
            when "HOURLY" { $unit = 3600; }
            when "DAILY" { $unit = 86400; }
            when "WEEKLY" { $unit = 604800; }
        }
        def out as list of time.Time init [
            time.add($start, time.fromSeconds($unit * $interval * $n))
        ];
        return $out;
    } catch (e) {
        if (not ($e.kind == "runtime")) {
            throw $e;
        }
        throw icalError(0, "RRULE: the recurrence runs past the representable time range");
    }
}

# isExcluded reports whether an instant matches any EXDATE of the event.
func isExcluded(ev as Event, t as time.Time) {
    for (def x in $ev.exdates) {
        if (time.equal($t, $x)) {
            return true;
        }
    }
    return false;
}

# sortTimes sorts instants ascending (a bottom-up merge sort over time.before).
func sortTimes(ts as list of time.Time) {
    def cur as list of time.Time init $ts;
    def n as int init len($cur);
    def width as int init 1;
    while ($width < $n) {
        def next as list of time.Time init [];
        def lo as int init 0;
        while ($lo < $n) {
            def mid as int init $lo + $width;
            if ($mid > $n) {
                $mid = $n;
            }
            def hi as int init $mid + $width;
            if ($hi > $n) {
                $hi = $n;
            }
            def i as int init $lo;
            def j as int init $mid;
            while ($i < $mid or $j < $hi) {
                if ($j >= $hi or ($i < $mid and not time.before($cur[$j], $cur[$i]))) {
                    $next[] = $cur[$i];
                    $i = $i + 1;
                } else {
                    $next[] = $cur[$j];
                    $j = $j + 1;
                }
            }
            $lo = $hi;
        }
        $cur = $next;
        $width = $width * 2;
    }
    return $cur;
}

/**
 * Expand an event's recurrence set into up to `max` occurrence instants, sorted
 * and without duplicates: `DTSTART`, the `RRULE` series from it (`FREQ`
 * `SECONDLY` ... `YEARLY`, `INTERVAL`, `COUNT`, `UNTIL`), and the `RDATE`s,
 * minus the `EXDATE`s. A `MONTHLY` / `YEARLY` instance whose date does not exist
 * (Jan 31 + 1 month, Feb 29 in a common year) is skipped and not counted, as RFC
 * 5545 requires. The `BY*` parts are not expanded: a rule holding one throws
 * rather than yield wrong instants.
 * @param ev {Event} the event
 * @param max {int} the maximum number of occurrences to return
 * @return {list of time.Time} the occurrence instants (at most `max`)
 * @throws {IcalError} kind "ical" for a malformed or unsupported `RRULE`, or a
 *     series that runs past the representable time range
 * @throws {Error} kind "limit" when reaching `max` takes more than 100000
 *     recurrence steps
 */
export func occurrences(ev as Event, max as int) {
    def out as list of time.Time init [];
    if ($max <= 0) {
        return $out;
    }
    def series as list of time.Time init [];
    if ($ev.rrule == "") {
        if (not isExcluded($ev, $ev.start)) {
            $series[] = $ev.start;
        }
    } else {
        def r as Rule init parseRule($ev, true, 0);
        # COUNT counts the instances the rule generates, excluded ones included.
        def produced as int init 0;
        def n as int init 0;
        while (len($series) < $max) {
            if ($r.count > 0 and $produced >= $r.count) {
                break;
            }
            if ($n >= OCCURRENCE_LIMIT) {
                throw Error{
                    kind: "limit",
                    message: "ical: occurrences: more than {OCCURRENCE_LIMIT} recurrence steps"
                };
            }
            def cs as list of time.Time init occurrenceAt($ev.start, $r.freq, $r.interval, $n);
            $n = $n + 1;
            if (len($cs) == 0) {
                continue;
            }
            # DTSTART is always the first instance, even past UNTIL.
            if ($r.hasUntil and $n > 1 and time.after($cs[0], $r.until)) {
                break;
            }
            $produced = $produced + 1;
            if (not isExcluded($ev, $cs[0])) {
                $series[] = $cs[0];
            }
        }
    }
    if (len($ev.rdates) == 0) {
        return $series;
    }
    # The series is ascending; merge in the RDATEs, sorted, dropping duplicates.
    def all as list of time.Time init $series;
    for (def t in $ev.rdates) {
        if (not isExcluded($ev, $t)) {
            $all[] = $t;
        }
    }
    for (def t in sortTimes($all)) {
        if (len($out) >= $max) {
            break;
        }
        if (len($out) == 0 or not time.equal($out[len($out) - 1], $t)) {
            $out[] = $t;
        }
    }
    return $out;
}
