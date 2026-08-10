# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0

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
 * expander for `FREQ` / `INTERVAL` / `COUNT` / `UNTIL`), **all-day** dates
 * (`VALUE=DATE`) and a named-zone **`TZID`**, an `ORGANIZER` and `ATTENDEE`s, and
 * `VALARM` alarms. Times go through `time`: UTC values write as
 * `20240615T130000Z`; a TZID event stores its wall clock as a floating value
 * paired with the zone name (because `time` models fixed-offset zones only, add a
 * matching `VTIMEZONE` for a strict consumer). Text values are RFC 5545-escaped
 * and content lines folded at 75 octets, so `parse(encode(cal))` round-trips.
 * @module ical
 * @example
 * import "ical.j" as ical;
 * use time;
 * def ev as ical.Event init ical.recur(
 *     ical.event("a@b", time.utc(), time.utc(), "Standup"), ical.rule("WEEKLY", 1, 10));
 * def cal as ical.Calendar init ical.add(ical.calendar(), $ev);
 * def text as string init ical.encode($cal);
 */
use strings;
use lists;
use time;
use convert;

# The vCard / iCalendar content-line codec (TEXT escaping, 75-char folding, the
# name / value split, `emit`) is shared with vcard.j via this include.
include "ical_vcard_shared.inc.j";

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
    $cal.events = lists.push($cal.events, $ev);
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
        $s = $s + ";INTERVAL=" + convert.toString($interval);
    }
    if ($count > 0) {
        $s = $s + ";COUNT=" + convert.toString($count);
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
    $ev.rdates = lists.push($ev.rdates, $t);
    return $ev;
}

/**
 * A copy of the event with an excluded recurrence instant (`EXDATE`) appended.
 * @param ev {Event} the event
 * @param t {time.Time} the excluded instant
 * @return {Event} a fresh event with the EXDATE added
 */
export func addExdate(ev as Event, t as time.Time) {
    $ev.exdates = lists.push($ev.exdates, $t);
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
    $ev.attendees = lists.push($ev.attendees, $a);
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
    $ev.alarms = lists.push($ev.alarms, $a);
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
    $cal.todos = lists.push($cal.todos, $td);
    return $cal;
}

# --- date-time (private) ----------------------------------------------------

# valueColon returns the index of the value-separating colon, scanning the
# name/parameter section while tracking double-quote state so a colon inside a
# quoted parameter value is not mistaken for the separator. Returns -1 when
# there is no unquoted colon.
func valueColon(line as string) {
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

# parseDateTime accepts the UTC `...Z` form, a floating DATE-TIME (parsed as
# UTC), and a bare DATE.
func parseDateTime(v as string) {
    if (strings.endsWith($v, "Z")) {
        return time.parse($v, "%Y%m%dT%H%M%SZ");
    }
    if (strings.contains($v, "T")) {
        return time.parse($v, "%Y%m%dT%H%M%S");
    }
    return time.parse($v, "%Y%m%d");
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

# encodeEvent appends one VEVENT (with recurrence / organizer / attendees /
# alarms) to `lines`, returning the extended list.
func encodeEvent(lines as list of string, ev as Event) {
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

# encodeTodo appends one VTODO to `lines`, returning the extended list.
func encodeTodo(lines as list of string, td as Todo) {
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
        $lines = encodeEvent($lines, $ev);
    }
    for (def td in $cal.todos) {
        $lines = encodeTodo($lines, $td);
    }
    $lines[] = "END:VCALENDAR";
    return strings.join($lines, "\r\n") + "\r\n";
}

# --- parse (exported) -------------------------------------------------------

# splitCommaDates parses a comma-separated DATE-TIME value (RDATE / EXDATE) into
# a list of instants.
func splitCommaDates(value as string) {
    def out as list of time.Time init [];
    for (def v in strings.split($value, ",")) {
        def t as string init strings.trim($v);
        if (not ($t == "")) {
            $out[] = parseDateTime($t);
        }
    }
    return $out;
}

/**
 * Parse iCalendar text into a `Calendar`. Unfolds folded lines and reads the
 * `PRODID`, each `VEVENT` (`UID` / `DTSTAMP` / `DTSTART` / `DTEND` / `SUMMARY` /
 * `DESCRIPTION` / `LOCATION`, the all-day `VALUE=DATE` and `TZID` parameters,
 * `RRULE` / `RDATE` / `EXDATE`, `ORGANIZER` / `ATTENDEE`, and nested `VALARM`s),
 * and each `VTODO`. A `VTIMEZONE` is parsed and skipped. An event with no
 * `DTSTART` is skipped; a missing `DTEND` defaults to the start.
 * @param text {string} the iCalendar text
 * @return {Calendar} the parsed calendar
 */
export func parse(text as string) {
    def cal as Calendar init calendar();
    def events as list of Event init [];
    def todos as list of Todo init [];
    # The current component: "" (top level), "VEVENT", "VTODO", or "VTIMEZONE".
    def mode as string init "";
    # Depth of an unrecognised nested sub-component whose properties are skipped.
    def skipDepth as int init 0;
    # Whether we are inside a VALARM within the current VEVENT.
    def inAlarm as bool init false;

    # Current-event accumulators.
    def uid as string init "";
    def summary as string init "";
    def description as string init "";
    def location as string init "";
    def startStr as string init "";
    def endStr as string init "";
    def stampStr as string init "";
    def allDay as bool init false;
    def tzid as string init "";
    def rruleStr as string init "";
    def rdates as list of time.Time init [];
    def exdates as list of time.Time init [];
    def organizer as string init "";
    def attendees as list of Attendee init [];
    def alarms as list of Alarm init [];
    # Current-alarm accumulators.
    def alAction as string init "";
    def alTrigger as string init "";
    def alDesc as string init "";
    # Current-todo accumulators.
    def tUid as string init "";
    def tSummary as string init "";
    def tDesc as string init "";
    def tStatus as string init "";
    def tStampStr as string init "";
    def tDueStr as string init "";

    for (def line in splitLines(unfold($text))) {
        if ($line == "") {
            continue;
        }
        def colon as int init valueColon($line);
        if ($colon < 0) {
            continue;
        }
        def nameSection as string init strings.substring($line, 0, $colon);
        def name as string init propName($nameSection);
        def value as string init strings.substring($line, $colon + 1, len($line));

        if ($name == "BEGIN") {
            def comp as string init strings.upper($value);
            if ($mode == "") {
                if ($comp == "VEVENT") {
                    $mode = "VEVENT";
                    $skipDepth = 0;
                    $inAlarm = false;
                    $uid = "";
                    $summary = "";
                    $description = "";
                    $location = "";
                    $startStr = "";
                    $endStr = "";
                    $stampStr = "";
                    $allDay = false;
                    $tzid = "";
                    $rruleStr = "";
                    def rd as list of time.Time init [];
                    $rdates = $rd;
                    def ed as list of time.Time init [];
                    $exdates = $ed;
                    $organizer = "";
                    def at as list of Attendee init [];
                    $attendees = $at;
                    def am as list of Alarm init [];
                    $alarms = $am;
                } elseif ($comp == "VTODO") {
                    $mode = "VTODO";
                    $skipDepth = 0;
                    $tUid = "";
                    $tSummary = "";
                    $tDesc = "";
                    $tStatus = "";
                    $tStampStr = "";
                    $tDueStr = "";
                } elseif ($comp == "VTIMEZONE") {
                    $mode = "VTIMEZONE";
                    $skipDepth = 0;
                }
                continue;
            }
            if ($mode == "VEVENT" and $comp == "VALARM" and $skipDepth == 0) {
                $inAlarm = true;
                $alAction = "";
                $alTrigger = "";
                $alDesc = "";
                continue;
            }
            $skipDepth = $skipDepth + 1;
            continue;
        }

        if ($name == "END") {
            def comp as string init strings.upper($value);
            if ($mode == "VEVENT" and $inAlarm and $comp == "VALARM") {
                $alarms[] = Alarm{action: $alAction, trigger: $alTrigger, description: $alDesc};
                $inAlarm = false;
                continue;
            }
            if ($skipDepth > 0) {
                $skipDepth = $skipDepth - 1;
                continue;
            }
            if ($mode == "VEVENT" and $comp == "VEVENT") {
                if (not ($startStr == "")) {
                    if ($endStr == "") {
                        $endStr = $startStr;
                    }
                    def ev as Event init event(
                        $uid,
                        parseDateTime($startStr),
                        parseDateTime($endStr),
                        $summary);
                    $ev = describe($ev, $description);
                    $ev = locate($ev, $location);
                    if (not ($stampStr == "")) {
                        $ev.stamp = parseDateTime($stampStr);
                    }
                    $ev.allDay = $allDay;
                    $ev.tzid = $tzid;
                    $ev.rrule = $rruleStr;
                    $ev.rdates = $rdates;
                    $ev.exdates = $exdates;
                    $ev.organizer = $organizer;
                    $ev.attendees = $attendees;
                    $ev.alarms = $alarms;
                    $events[] = $ev;
                }
                $mode = "";
            } elseif ($mode == "VTODO" and $comp == "VTODO") {
                if (not ($tUid == "") or not ($tSummary == "")) {
                    def stamp as time.Time init time.utc();
                    if (not ($tStampStr == "")) {
                        $stamp = parseDateTime($tStampStr);
                    }
                    def td as Todo init todo($tUid, $stamp, $tSummary);
                    $td = describeTodo($td, $tDesc);
                    $td = withStatus($td, $tStatus);
                    if (not ($tDueStr == "")) {
                        $td = withDue($td, parseDateTime($tDueStr));
                    }
                    $todos[] = $td;
                }
                $mode = "";
            } elseif ($mode == "VTIMEZONE" and $comp == "VTIMEZONE") {
                $mode = "";
            }
            continue;
        }

        # A property line. Skip inside an unrecognised nested component or a
        # (parse-and-skip) VTIMEZONE.
        if ($skipDepth > 0 or $mode == "VTIMEZONE") {
            continue;
        }
        if ($mode == "VEVENT" and $inAlarm) {
            match ($name) {
                when "ACTION" { $alAction = $value; }
                when "TRIGGER" { $alTrigger = $value; }
                when "DESCRIPTION" { $alDesc = unescapeText($value); }
            }
            continue;
        }
        if ($mode == "VEVENT") {
            match ($name) {
                when "UID" { $uid = unescapeText($value); }
                when "SUMMARY" { $summary = unescapeText($value); }
                when "DESCRIPTION" { $description = unescapeText($value); }
                when "LOCATION" { $location = unescapeText($value); }
                when "DTSTART" {
                    $startStr = $value;
                    if (strings.upper(paramValue($nameSection, "VALUE")) == "DATE") {
                        $allDay = true;
                    }
                    def z as string init paramValue($nameSection, "TZID");
                    if (not ($z == "")) {
                        $tzid = $z;
                    }
                }
                when "DTEND" { $endStr = $value; }
                when "DTSTAMP" { $stampStr = $value; }
                when "RRULE" { $rruleStr = $value; }
                when "RDATE" {
                    for (def t in splitCommaDates($value)) {
                        $rdates[] = $t;
                    }
                }
                when "EXDATE" {
                    for (def t in splitCommaDates($value)) {
                        $exdates[] = $t;
                    }
                }
                when "ORGANIZER" { $organizer = $value; }
                when "ATTENDEE" {
                    $attendees[] = Attendee{
                        address: $value,
                        cn: paramValue($nameSection, "CN"),
                        role: paramValue($nameSection, "ROLE")
                    };
                }
            }
            continue;
        }
        if ($mode == "VTODO") {
            match ($name) {
                when "UID" { $tUid = unescapeText($value); }
                when "SUMMARY" { $tSummary = unescapeText($value); }
                when "DESCRIPTION" { $tDesc = unescapeText($value); }
                when "STATUS" { $tStatus = $value; }
                when "DTSTAMP" { $tStampStr = $value; }
                when "DUE" { $tDueStr = $value; }
            }
            continue;
        }
        if ($mode == "" and $name == "PRODID") {
            $cal.prodid = unescapeText($value);
        }
    }
    $cal.events = $events;
    $cal.todos = $todos;
    return $cal;
}

# --- recurrence expansion (exported) ----------------------------------------

# rruleField returns the value of a `KEY=VALUE` field in an RRULE (case-insensitive
# key), or "" when absent.
func rruleField(rrule as string, key as string) {
    def want as string init strings.upper($key);
    for (def part in strings.split($rrule, ";")) {
        def eq as int init strings.indexOf($part, "=");
        if ($eq >= 0 and strings.upper(strings.substring($part, 0, $eq)) == $want) {
            return strings.substring($part, $eq + 1, len($part));
        }
    }
    return "";
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
    return time.parse(pad4($y) + pad2($mo) + pad2($d) + "T" + pad2($h) + pad2($mi) + pad2($s) + "Z",
        "%Y%m%dT%H%M%SZ");
}

# addMonthsUtc adds `n` calendar months to an instant, clamping the day to the
# target month's length (Jan 31 + 1 month -> Feb 28/29). YEARLY reuses this with
# n = 12 * years.
func addMonthsUtc(t as time.Time, n as int) {
    def u as time.Time init time.inZone($t, time.UTC);
    def total as int init (time.month($u) - 1) + $n;
    def ny as int init time.year($u) + $total // 12;
    def nmo as int init $total % 12 + 1;
    def d as int init time.day($u);
    def dim as int init daysInMonth($ny, $nmo);
    if ($d > $dim) {
        $d = $dim;
    }
    return buildUtc($ny, $nmo, $d, time.hour($u), time.minute($u), time.second($u));
}

# occurrenceAt returns the n-th occurrence (0-based) computed from `start`, so a
# MONTHLY / YEARLY day-clamp (Jan 31 -> Feb 29) does not propagate into later
# months - each occurrence is derived from DTSTART, not the previous (clamped)
# instant, matching RFC 5545 recurrence semantics.
func occurrenceAt(start as time.Time, freq as string, interval as int, n as int) {
    if ($freq == "WEEKLY") {
        return time.add($start, time.fromHours(24 * 7 * $interval * $n));
    }
    if ($freq == "MONTHLY") {
        return addMonthsUtc($start, $interval * $n);
    }
    if ($freq == "YEARLY") {
        return addMonthsUtc($start, 12 * $interval * $n);
    }
    return time.add($start, time.fromHours(24 * $interval * $n)); # DAILY (and default)
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

/**
 * Expand an event's recurrence into up to `max` occurrence instants, in order:
 * the `RRULE` series from `DTSTART` (honouring `FREQ` / `INTERVAL` / `COUNT` /
 * `UNTIL`), plus any `RDATE`s, minus any `EXDATE`s. A non-recurring event yields
 * just its start. Only the frequency rules are expanded - `BYDAY` / `BYMONTH` and
 * other `BY*` parts are not applied (the base cadence is still produced), so this
 * covers the common "every N days / weeks / months / years" case.
 * @param ev {Event} the event
 * @param max {int} the maximum number of occurrences to return
 * @return {list of time.Time} the occurrence instants (at most `max`)
 */
export func occurrences(ev as Event, max as int) {
    def out as list of time.Time init [];
    if ($max <= 0) {
        return $out;
    }
    if (not ($ev.rrule == "")) {
        def freq as string init strings.upper(rruleField($ev.rrule, "FREQ"));
        def interval as int init 1;
        def iv as string init rruleField($ev.rrule, "INTERVAL");
        if (not ($iv == "")) {
            $interval = convert.toInt($iv);
        }
        if ($interval < 1) {
            $interval = 1;
        }
        def count as int init 0;
        def cv as string init rruleField($ev.rrule, "COUNT");
        if (not ($cv == "")) {
            $count = convert.toInt($cv);
        }
        def untilStr as string init rruleField($ev.rrule, "UNTIL");
        def hasUntil as bool init not ($untilStr == "");
        def untilT as time.Time init $ev.start;
        if ($hasUntil) {
            $untilT = parseDateTime($untilStr);
        }
        def n as int init 0;
        def guard as int init 0;
        while (len($out) < $max and $guard < 100000) {
            $guard = $guard + 1;
            if ($count > 0 and $n >= $count) {
                break;
            }
            def cur as time.Time init occurrenceAt($ev.start, $freq, $interval, $n);
            if ($hasUntil and time.after($cur, $untilT)) {
                break;
            }
            if (not isExcluded($ev, $cur)) {
                $out[] = $cur;
            }
            $n = $n + 1;
        }
    } elseif (len($ev.rdates) == 0) {
        # A one-off event: its start, unless excluded.
        if (not isExcluded($ev, $ev.start)) {
            $out[] = $ev.start;
        }
    }
    for (def t in $ev.rdates) {
        if (len($out) >= $max) {
            return $out;
        }
        if (not isExcluded($ev, $t)) {
            $out[] = $t;
        }
    }
    return $out;
}
