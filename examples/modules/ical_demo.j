#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The ical module (modules/ical.j): build a VCALENDAR with two events, encode it
 * to RFC 5545 text (escaped, folded, CRLF), then parse it back and read fields.
 * Run: jennifer run examples/modules/ical_demo.j
 * @module ical_demo
 */
use io;
use time;
import "../../modules/ical.j" as ical;

# Two events, one with a description and location. Times go through `time`.
def standup as ical.Event init ical.event(
    "standup-2024-06-17@team",
    time.fromIso("2024-06-17T09:00:00Z"),
    time.fromIso("2024-06-17T09:15:00Z"),
    "Daily standup");

def launch as ical.Event init ical.event(
    "launch-2024-06-20@team",
    time.fromIso("2024-06-20T14:00:00Z"),
    time.fromIso("2024-06-20T15:30:00Z"),
    "Product launch; all-hands");
$launch = ical.describe($launch, "Demo, Q&A, and cake.\nBring laptops.");
$launch = ical.locate($launch, "Room 5");

def cal as ical.Calendar init ical.calendar();
$cal = ical.add($cal, $standup);
$cal = ical.add($cal, $launch);

def text as string init ical.encode($cal);
io.printf("=== encoded iCalendar ===\n%s\n", $text);

# Parse it back and walk the events.
def back as ical.Calendar init ical.parse($text);
io.printf("=== parsed %d events (prodid %s) ===\n", len($back.events), $back.prodid);
for (def ev in $back.events) {
    io.printf("- %s  %s -> %s\n", $ev.summary, time.iso($ev.start), time.iso($ev.end));
    if ($ev.location != "") {
        io.printf("    at: %s\n", $ev.location);
    }
}
