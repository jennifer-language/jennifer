#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The ical module (modules/ical.j): build a VCALENDAR with two events, encode it
 * to RFC 5545 text (escaped, folded, CRLF), then parse it back and read fields.
 * Run: jennifer run examples/modules/ical_demo.j
 * @module ical_demo
 */
use io;
use time;
import "../../modules/ical.j" as ical;

# A weekly recurring standup, with an organizer, an attendee, and a 5-minute
# reminder alarm. Times go through `time`.
def standup as ical.Event init ical.event(
    "standup-2024-06-17@team",
    time.fromIso("2024-06-17T09:00:00Z"),
    time.fromIso("2024-06-17T09:15:00Z"),
    "Daily standup");
$standup = ical.recur($standup, ical.rule("WEEKLY", 1, 4));
$standup = ical.withOrganizer($standup, "mailto:lead@team.example");
$standup = ical.addAttendee($standup, ical.attendee("mailto:dev@team.example", "Dev", "REQ-PARTICIPANT"));
$standup = ical.addAlarm($standup, ical.alarm("DISPLAY", "-PT5M", "Standup in 5 minutes"));

def launch as ical.Event init ical.event(
    "launch-2024-06-20@team",
    time.fromIso("2024-06-20T14:00:00Z"),
    time.fromIso("2024-06-20T15:30:00Z"),
    "Product launch; all-hands");
$launch = ical.describe($launch, "Demo, Q&A, and cake.\nBring laptops.");
$launch = ical.locate($launch, "Room 5");

# An all-day company holiday.
def holiday as ical.Event init ical.withAllDay(
    ical.event("holiday-2024-07-04@team",
        time.fromIso("2024-07-04T00:00:00Z"), time.fromIso("2024-07-05T00:00:00Z"), "Company Holiday"),
    true);

def cal as ical.Calendar init ical.calendar();
$cal = ical.add($cal, $standup);
$cal = ical.add($cal, $launch);
$cal = ical.add($cal, $holiday);

# A to-do with a due date.
$cal = ical.addTodo($cal, ical.withStatus(
    ical.withDue(ical.todo("ship-v1@team", time.fromIso("2024-06-17T09:00:00Z"), "Ship v1"),
        time.fromIso("2024-06-30T17:00:00Z")),
    "NEEDS-ACTION"));

def text as string init ical.encode($cal);
io.printf("=== encoded iCalendar ===\n%s\n", $text);

# Parse it back and walk the events.
def back as ical.Calendar init ical.parse($text);
io.printf("=== parsed %d events, %d to-dos (prodid %s) ===\n", len($back.events), len($back.todos), $back.prodid);
for (def ev in $back.events) {
    io.printf("- %s  %s -> %s\n", $ev.summary, time.iso($ev.start), time.iso($ev.end));
    if ($ev.location != "") {
        io.printf("    at: %s\n", $ev.location);
    }
}

# Expand the recurring standup into its concrete dates.
io.printf("=== standup occurrences ===\n");
for (def occ in ical.occurrences($back.events[0], 10)) {
    io.printf("  %s\n", time.iso($occ));
}
