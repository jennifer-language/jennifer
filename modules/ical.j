# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

/**
 * Build and parse iCalendar (RFC 5545): a `Calendar` holding `Event`s, encoded
 * to a `VCALENDAR` with `VEVENT`s and parsed back. Pure Jennifer over `strings`
 * / `lists` + `time` - no Go, no system engine.
 *
 * Dates and times go through `time`: `DTSTAMP` / `DTSTART` / `DTEND` are written
 * as UTC `DATE-TIME` values (`20240615T130000Z`) and parsed back into
 * `time.Time`. Text values (`SUMMARY` / `DESCRIPTION` / `LOCATION` / `UID`) are
 * RFC 5545-escaped (`\` `;` `,` and newline), and content lines are folded at 75
 * characters and unfolded on the way in, so `parse(encode(cal))` round-trips.
 * @module ical
 * @example
 * import "ical.j" as ical;
 * use time;
 * def ev as ical.Event init ical.event("a@b", time.utc(), time.utc(), "Launch");
 * def cal as ical.Calendar init ical.add(ical.calendar(), $ev);
 * def text as string init ical.encode($cal);
 */
use strings;
use lists;
use time;
use convert;

# The vCard / iCalendar content-line codec (TEXT escaping, 75-char folding, the
# name / value split, `emit`) is shared with vcard.j via this include.
include "ical_vcard_shared.j";

/**
 * A calendar: a product identifier and its events.
 * @field prodid {string} the `PRODID` product identifier
 * @field events {list of Event} the calendar's events
 */
export def struct Calendar {
    prodid as string,
    events as list of Event
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
 */
export def struct Event {
    uid as string,
    stamp as time.Time,
    start as time.Time,
    end as time.Time,
    summary as string,
    description as string,
    location as string
};

# --- constructors (exported) ------------------------------------------------

/**
 * A calendar with the default Jennifer `PRODID` and no events.
 * @return {Calendar} the empty calendar
 */
export func calendar() {
    return Calendar{ prodid: "-//Jennifer//ical//EN", events: [] };
}

/**
 * A calendar with a caller-supplied `PRODID`.
 * @param prodid {string} the product identifier
 * @return {Calendar} the empty calendar
 */
export func calendarWith(prodid as string) {
    return Calendar{ prodid: $prodid, events: [] };
}

/**
 * An event. `DTSTAMP` defaults to the start; `DESCRIPTION` / `LOCATION` are
 * empty until set with `describe` / `locate`.
 * @param uid {string} the unique identifier
 * @param start {time.Time} the start instant
 * @param end {time.Time} the end instant
 * @param summary {string} the title
 * @return {Event} the event
 */
export func event(uid as string, start as time.Time, end as time.Time, summary as string) {
    return Event{ uid: $uid, stamp: $start, start: $start, end: $end, summary: $summary, description: "", location: "" };
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

# --- date-time (private) ----------------------------------------------------

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

# --- encode (exported) ------------------------------------------------------

/**
 * Render a calendar to iCalendar text (RFC 5545): a `VCALENDAR` wrapping one
 * `VEVENT` per event, CRLF line endings, escaped text, folded long lines.
 * `DESCRIPTION` / `LOCATION` are emitted only when non-empty.
 * @param cal {Calendar} the calendar to encode
 * @return {string} the iCalendar text (CRLF-terminated)
 */
export func encode(cal as Calendar) {
    def lines as list of string init [];
    $lines[] = "BEGIN:VCALENDAR";
    $lines[] = "VERSION:2.0";
    $lines[] = emitLine("PRODID", escapeText($cal.prodid));
    for (def ev in $cal.events) {
        $lines[] = "BEGIN:VEVENT";
        $lines[] = emitLine("UID", escapeText($ev.uid));
        $lines[] = emitLine("DTSTAMP", formatDateTime($ev.stamp));
        $lines[] = emitLine("DTSTART", formatDateTime($ev.start));
        $lines[] = emitLine("DTEND", formatDateTime($ev.end));
        $lines[] = emitLine("SUMMARY", escapeText($ev.summary));
        if (not ($ev.description == "")) {
            $lines[] = emitLine("DESCRIPTION", escapeText($ev.description));
        }
        if (not ($ev.location == "")) {
            $lines[] = emitLine("LOCATION", escapeText($ev.location));
        }
        $lines[] = "END:VEVENT";
    }
    $lines[] = "END:VCALENDAR";
    return strings.join($lines, "\r\n") + "\r\n";
}

# --- parse (exported) -------------------------------------------------------

/**
 * Parse iCalendar text into a `Calendar`. Unfolds folded lines, reads the
 * `PRODID` and each `VEVENT`'s properties (`UID` / `DTSTAMP` / `DTSTART` /
 * `DTEND` / `SUMMARY` / `DESCRIPTION` / `LOCATION`), and unescapes text values.
 * Property parameters (after a `;`) are ignored; an event with no `DTSTART` is
 * skipped; a missing `DTEND` defaults to the start.
 * @param text {string} the iCalendar text
 * @return {Calendar} the parsed calendar
 */
export func parse(text as string) {
    def cal as Calendar init calendar();
    def events as list of Event init [];
    def inEvent as bool init false;
    # Depth of a nested sub-component (VALARM, ...) inside the current VEVENT.
    # While > 0 the sub-component's properties are skipped so its DESCRIPTION /
    # SUMMARY don't clobber the enclosing event's.
    def skipDepth as int init 0;
    def uid as string init "";
    def summary as string init "";
    def description as string init "";
    def location as string init "";
    def startStr as string init "";
    def endStr as string init "";
    def stampStr as string init "";
    for (def line in splitLines(unfold($text))) {
        if ($line == "") {
            continue;
        }
        def colon as int init strings.indexOf($line, ":");
        if ($colon < 0) {
            continue;
        }
        def name as string init propName(strings.substring($line, 0, $colon));
        def value as string init strings.substring($line, $colon + 1, len($line));
        if ($name == "BEGIN" and strings.upper($value) == "VEVENT") {
            $inEvent = true;
            $skipDepth = 0;
            $uid = "";
            $summary = "";
            $description = "";
            $location = "";
            $startStr = "";
            $endStr = "";
            $stampStr = "";
            continue;
        }
        # A sub-component opened inside the event (BEGIN:VALARM, ...): enter skip
        # mode so its properties don't overwrite the event's, until its END.
        if ($inEvent and $name == "BEGIN") {
            $skipDepth = $skipDepth + 1;
            continue;
        }
        if ($inEvent and $skipDepth > 0 and $name == "END") {
            $skipDepth = $skipDepth - 1;
            continue;
        }
        if ($name == "END" and strings.upper($value) == "VEVENT") {
            $inEvent = false;
            if (not ($startStr == "")) {
                if ($endStr == "") {
                    $endStr = $startStr;
                }
                def ev as Event init event($uid, parseDateTime($startStr), parseDateTime($endStr), $summary);
                $ev = describe($ev, $description);
                $ev = locate($ev, $location);
                if (not ($stampStr == "")) {
                    $ev.stamp = parseDateTime($stampStr);
                }
                $events[] = $ev;
            }
            continue;
        }
        if ($inEvent and $skipDepth == 0) {
            if ($name == "UID") {
                $uid = unescapeText($value);
            } elseif ($name == "SUMMARY") {
                $summary = unescapeText($value);
            } elseif ($name == "DESCRIPTION") {
                $description = unescapeText($value);
            } elseif ($name == "LOCATION") {
                $location = unescapeText($value);
            } elseif ($name == "DTSTART") {
                $startStr = $value;
            } elseif ($name == "DTEND") {
                $endStr = $value;
            } elseif ($name == "DTSTAMP") {
                $stampStr = $value;
            }
        } elseif ($name == "PRODID") {
            $cal.prodid = unescapeText($value);
        }
    }
    $cal.events = $events;
    return $cal;
}
