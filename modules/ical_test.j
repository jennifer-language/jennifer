# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# ical_test.j - white-box tests for ical.j. Run with:
#
#     jennifer test modules/ical_test.j
#
# The overlay splices ical.j in front of this file, so the tests reach its
# private helpers (escapeText, unescapeText, fold, unfold, formatDateTime,
# parseDateTime, propName) by bare identifier as well as its exported surface.
# ical.j already `use`s strings / lists / time, so the overlay only adds testing.
use testing;

func at(iso as string) {
    return time.fromIso($iso);
}

func sampleEvent() {
    def ev as Event init event(
        "evt-1@example.com",
        at("2024-06-15T13:00:00Z"),
        at("2024-06-15T14:00:00Z"),
        "Launch; party, cake");
    $ev = describe($ev, "Line one\nLine two with, comma");
    $ev = locate($ev, "Room 5");
    return $ev;
}

# --- escaping (private) -----------------------------------------------------

func testEscapeText() {
    testing.assertEqual(escapeText("a,b"), "a\\,b");
    testing.assertEqual(escapeText("a;b"), "a\\;b");
    testing.assertEqual(escapeText("a\nb"), "a\\nb");
    testing.assertEqual(escapeText("a\\b"), "a\\\\b");
}

func testUnescapeText() {
    testing.assertEqual(unescapeText("a\\,b"), "a,b");
    testing.assertEqual(unescapeText("a\\;b"), "a;b");
    testing.assertEqual(unescapeText("x\\ny"), "x\ny");
    testing.assertEqual(unescapeText("p\\\\q"), "p\\q");
}

func testEscapeRoundTrips() {
    def s as string init "semis; commas, backslash \\ and\na newline";
    testing.assertEqual(unescapeText(escapeText($s)), $s);
}

# --- folding (private) ------------------------------------------------------

func testFoldShortUnchanged() {
    testing.assertEqual(fold("SUMMARY:short line"), "SUMMARY:short line");
}

func testFoldLongWraps() {
    def long as string init "SUMMARY:" + strings.repeat("x", 200);
    def folded as string init fold($long);
    # A wrapped line carries CRLF + space continuations, and unfolding restores it.
    testing.assertTrue(strings.contains($folded, "\r\n "));
    testing.assertEqual(unfold($folded), $long);
}

func testUnfoldTabAndLf() {
    testing.assertEqual(unfold("abc\r\n def"), "abcdef");
    testing.assertEqual(unfold("abc\r\n\tdef"), "abcdef");
    testing.assertEqual(unfold("abc\n def"), "abcdef");
}

# --- date-time (private) ----------------------------------------------------

func testFormatDateTimeUtc() {
    testing.assertEqual(formatDateTime(at("2024-06-15T13:00:00Z")), "20240615T130000Z");
}

func testFormatDateTimeNormalisesToUtc() {
    # A +01:00 wall-clock of 14:00 is 13:00 UTC.
    testing.assertEqual(formatDateTime(at("2024-06-15T14:00:00+01:00")), "20240615T130000Z");
}

func testParseDateTimeForms() {
    testing.assertTrue(time.equal(parseDateTime("20240615T130000Z"), at("2024-06-15T13:00:00Z")));
    testing.assertTrue(time.equal(parseDateTime("20240615T130000"), at("2024-06-15T13:00:00Z")));
    testing.assertTrue(time.equal(parseDateTime("20240615"), at("2024-06-15T00:00:00Z")));
}

# --- property name parsing (private) ----------------------------------------

func testPropName() {
    testing.assertEqual(propName("DTSTART"), "DTSTART");
    testing.assertEqual(propName("DTSTART;VALUE=DATE-TIME"), "DTSTART");
    testing.assertEqual(propName("dtstart"), "DTSTART");
}

# --- encode (exported) ------------------------------------------------------

func testEncodeStructure() {
    def cal as Calendar init add(calendar(), sampleEvent());
    def text as string init encode($cal);
    testing.assertTrue(strings.startsWith($text, "BEGIN:VCALENDAR\r\n"));
    testing.assertTrue(strings.contains($text, "VERSION:2.0\r\n"));
    testing.assertTrue(strings.contains($text, "BEGIN:VEVENT\r\n"));
    testing.assertTrue(strings.contains($text, "UID:evt-1@example.com\r\n"));
    testing.assertTrue(strings.contains($text, "SUMMARY:Launch\\; party\\, cake\r\n"));
    testing.assertTrue(strings.contains($text, "END:VCALENDAR\r\n"));
}

func testEncodeOmitsEmptyOptionalFields() {
    def ev as Event init event("u", at("2024-06-15T13:00:00Z"), at("2024-06-15T14:00:00Z"), "Bare");
    def text as string init encode(add(calendar(), $ev));
    testing.assertFalse(strings.contains($text, "DESCRIPTION"));
    testing.assertFalse(strings.contains($text, "LOCATION"));
}

func testCustomProdid() {
    def cal as Calendar init calendarWith("-//Acme//Cal//EN");
    testing.assertTrue(strings.contains(encode($cal), "PRODID:-//Acme//Cal//EN\r\n"));
}

# --- parse + round-trip (exported) ------------------------------------------

func testRoundTrip() {
    def cal as Calendar init add(calendar(), sampleEvent());
    def back as Calendar init parse(encode($cal));
    testing.assertEqual($back.prodid, "-//Jennifer//ical//EN");
    testing.assertEqual(len($back.events), 1);
    def r as Event init $back.events[0];
    testing.assertEqual($r.uid, "evt-1@example.com");
    testing.assertEqual($r.summary, "Launch; party, cake");
    testing.assertEqual($r.description, "Line one\nLine two with, comma");
    testing.assertEqual($r.location, "Room 5");
    testing.assertTrue(time.equal($r.start, at("2024-06-15T13:00:00Z")));
    testing.assertTrue(time.equal($r.end, at("2024-06-15T14:00:00Z")));
    testing.assertTrue(time.equal($r.stamp, at("2024-06-15T13:00:00Z")));
}

func testParseTwoEvents() {
    def cal as Calendar init calendar();
    $cal = add($cal, event("a", at("2024-01-01T10:00:00Z"), at("2024-01-01T11:00:00Z"), "One"));
    $cal = add($cal, event("b", at("2024-02-02T10:00:00Z"), at("2024-02-02T11:00:00Z"), "Two"));
    def back as Calendar init parse(encode($cal));
    testing.assertEqual(len($back.events), 2);
    testing.assertEqual($back.events[0].summary, "One");
    testing.assertEqual($back.events[1].uid, "b");
}

func testParseIgnoresParameters() {
    def src as string init "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:x\r\nDTSTART;VALUE=DATE-TIME:20240615T130000Z\r\nDTEND:20240615T140000Z\r\nSUMMARY:Hi\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n";
    def cal as Calendar init parse($src);
    testing.assertEqual(len($cal.events), 1);
    testing.assertEqual($cal.events[0].summary, "Hi");
    testing.assertTrue(time.equal($cal.events[0].start, at("2024-06-15T13:00:00Z")));
}

# A nested VALARM carries its own DESCRIPTION / SUMMARY; those must not
# overwrite the enclosing VEVENT's fields (real Google / Outlook exports embed
# alarms). The parser skips a sub-component's properties until its END.
func testParseIgnoresNestedValarm() {
    def src as string init "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:x\r\nDTSTART:20240615T130000Z\r\nDTEND:20240615T140000Z\r\nSUMMARY:Real Summary\r\nDESCRIPTION:Real Description\r\nBEGIN:VALARM\r\nACTION:DISPLAY\r\nDESCRIPTION:Reminder popup\r\nSUMMARY:Alarm summary\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n";
    def cal as Calendar init parse($src);
    testing.assertEqual(len($cal.events), 1);
    testing.assertEqual($cal.events[0].summary, "Real Summary");
    testing.assertEqual($cal.events[0].description, "Real Description");
}

func testParseSkipsEventWithoutStart() {
    def src as string init "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:x\r\nSUMMARY:No start\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n";
    testing.assertEqual(len(parse($src).events), 0);
}

func testParseDefaultsEndToStart() {
    def src as string init "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:x\r\nDTSTART:20240615T130000Z\r\nSUMMARY:Point\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n";
    def cal as Calendar init parse($src);
    testing.assertEqual(len($cal.events), 1);
    testing.assertTrue(time.equal($cal.events[0].end, $cal.events[0].start));
}

func testParseFoldedLine() {
    # A DESCRIPTION folded across two physical lines unfolds to one value.
    def src as string init "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:x\r\nDTSTART:20240615T130000Z\r\nDTEND:20240615T140000Z\r\nDESCRIPTION:first part \r\n and second part\r\nSUMMARY:Folded\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n";
    def cal as Calendar init parse($src);
    testing.assertEqual($cal.events[0].description, "first part and second part");
}

# --- M23.6: all-day / TZID / recurrence / VALARM / attendees / VTODO ---

func testAllDayRoundTrip() {
    def ev as Event init withAllDay(event("h", at("2024-06-15T00:00:00Z"), at("2024-06-16T00:00:00Z"), "Holiday"), true);
    def text as string init encode(add(calendar(), $ev));
    testing.assertContains($text, "DTSTART;VALUE=DATE:20240615\r\n");
    def back as Event init parse($text).events[0];
    testing.assertTrue($back.allDay);
}

func testTzidRoundTrip() {
    def ev as Event init withZone(event("z", at("2024-06-15T13:00:00Z"), at("2024-06-15T14:00:00Z"), "Local"), "Europe/London");
    def text as string init encode(add(calendar(), $ev));
    # a TZID value is floating (no trailing Z) and carries the zone parameter
    testing.assertContains($text, "DTSTART;TZID=Europe/London:20240615T130000\r\n");
    def back as Event init parse($text).events[0];
    testing.assertEqual($back.tzid, "Europe/London");
}

func testRuleBuilder() {
    testing.assertEqual(rule("weekly", 1, 3), "FREQ=WEEKLY;COUNT=3");
    testing.assertEqual(rule("DAILY", 2, 0), "FREQ=DAILY;INTERVAL=2");
    testing.assertEqual(rule("MONTHLY", 1, 0), "FREQ=MONTHLY");
}

func testRecurrenceRoundTrip() {
    def ev as Event init recur(event("r", at("2024-06-15T13:00:00Z"), at("2024-06-15T14:00:00Z"), "Standup"), "FREQ=WEEKLY;COUNT=3");
    def back as Event init parse(encode(add(calendar(), $ev))).events[0];
    testing.assertEqual($back.rrule, "FREQ=WEEKLY;COUNT=3");
}

func testOccurrencesWeekly() {
    def ev as Event init recur(event("r", at("2024-06-15T13:00:00Z"), at("2024-06-15T14:00:00Z"), "S"), rule("WEEKLY", 1, 3));
    def occ as list of time.Time init occurrences($ev, 10);
    testing.assertEqual(len($occ), 3);
    testing.assertTrue(time.equal($occ[0], at("2024-06-15T13:00:00Z")));
    testing.assertTrue(time.equal($occ[2], at("2024-06-29T13:00:00Z")));
}

# MONTHLY clamps a too-large day per month, computed from DTSTART (not the
# previous clamped instant): Jan 31 -> Feb 29 -> Mar 31 -> Apr 30.
func testOccurrencesMonthlyClamp() {
    def ev as Event init recur(event("m", at("2024-01-31T09:00:00Z"), at("2024-01-31T09:00:00Z"), "M"), rule("MONTHLY", 1, 4));
    def occ as list of time.Time init occurrences($ev, 10);
    testing.assertEqual(len($occ), 4);
    testing.assertTrue(time.equal($occ[1], at("2024-02-29T09:00:00Z")));
    testing.assertTrue(time.equal($occ[2], at("2024-03-31T09:00:00Z")));
    testing.assertTrue(time.equal($occ[3], at("2024-04-30T09:00:00Z")));
}

func testOccurrencesExdateAndRdate() {
    def ev as Event init recur(event("d", at("2024-06-01T08:00:00Z"), at("2024-06-01T08:00:00Z"), "D"), rule("DAILY", 1, 5));
    $ev = addExdate($ev, at("2024-06-03T08:00:00Z")); # remove day 3
    $ev = addRdate($ev, at("2024-06-20T08:00:00Z")); # add an extra
    def occ as list of time.Time init occurrences($ev, 20);
    # 5 daily - 1 excluded + 1 rdate = 5
    testing.assertEqual(len($occ), 5);
    testing.assertTrue(time.equal($occ[4], at("2024-06-20T08:00:00Z")));
}

# A VALARM inside a VEVENT is parsed into the event's alarms (and its DESCRIPTION
# still does not clobber the event's).
func testValarmRoundTrip() {
    def ev as Event init addAlarm(event("a", at("2024-06-15T13:00:00Z"), at("2024-06-15T14:00:00Z"), "Meet"), alarm("DISPLAY", "-PT15M", "Reminder"));
    $ev = describe($ev, "Event body");
    def back as Event init parse(encode(add(calendar(), $ev))).events[0];
    testing.assertEqual($back.description, "Event body");
    testing.assertEqual(len($back.alarms), 1);
    testing.assertEqual($back.alarms[0].action, "DISPLAY");
    testing.assertEqual($back.alarms[0].trigger, "-PT15M");
    testing.assertEqual($back.alarms[0].description, "Reminder");
}

func testOrganizerAttendeeRoundTrip() {
    def ev as Event init withOrganizer(event("o", at("2024-06-15T13:00:00Z"), at("2024-06-15T14:00:00Z"), "Sync"), "mailto:boss@x.com");
    $ev = addAttendee($ev, attendee("mailto:bob@x.com", "Bob", "REQ-PARTICIPANT"));
    def text as string init encode(add(calendar(), $ev));
    testing.assertContains($text, "ATTENDEE;CN=Bob;ROLE=REQ-PARTICIPANT:mailto:bob@x.com\r\n");
    def back as Event init parse($text).events[0];
    testing.assertEqual($back.organizer, "mailto:boss@x.com");
    testing.assertEqual($back.attendees[0].cn, "Bob");
    testing.assertEqual($back.attendees[0].role, "REQ-PARTICIPANT");
}

# An attendee CN containing a `:` is quoted, so it does not truncate the line at
# the wrong colon (an unquoted CN colon otherwise corrupts the address).
func testAttendeeCnQuotedWhenSpecial() {
    def ev as Event init addAttendee(event("x", at("2024-06-15T13:00:00Z"), at("2024-06-15T14:00:00Z"), "M"), attendee("mailto:b@x", "Bob: The 3rd", "REQ"));
    def text as string init encode(add(calendar(), $ev));
    testing.assertContains($text, "ATTENDEE;CN=\"Bob: The 3rd\";ROLE=REQ:mailto:b@x\r\n");
    def back as Event init parse($text).events[0];
    testing.assertEqual($back.attendees[0].cn, "Bob: The 3rd");
    testing.assertEqual($back.attendees[0].address, "mailto:b@x");
}

func testVtodoRoundTrip() {
    def td as Todo init withStatus(withDue(todo("t1", at("2024-06-15T13:00:00Z"), "Ship it"), at("2024-06-20T17:00:00Z")), "NEEDS-ACTION");
    def cal as Calendar init addTodo(calendar(), $td);
    def text as string init encode($cal);
    testing.assertContains($text, "BEGIN:VTODO\r\n");
    testing.assertContains($text, "DUE:20240620T170000Z\r\n");
    def back as Calendar init parse($text);
    testing.assertEqual(len($back.todos), 1);
    testing.assertEqual($back.todos[0].summary, "Ship it");
    testing.assertEqual($back.todos[0].status, "NEEDS-ACTION");
    testing.assertTrue($back.todos[0].hasDue);
    testing.assertTrue(time.equal($back.todos[0].due, at("2024-06-20T17:00:00Z")));
}

# A VTIMEZONE component is parsed and skipped, so an event after it still parses.
func testVtimezoneSkipped() {
    def src as string init "BEGIN:VCALENDAR\r\nBEGIN:VTIMEZONE\r\nTZID:Europe/London\r\nBEGIN:STANDARD\r\nTZOFFSETTO:+0000\r\nEND:STANDARD\r\nEND:VTIMEZONE\r\nBEGIN:VEVENT\r\nUID:x\r\nDTSTART:20240615T130000Z\r\nSUMMARY:After TZ\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n";
    def cal as Calendar init parse($src);
    testing.assertEqual(len($cal.events), 1);
    testing.assertEqual($cal.events[0].summary, "After TZ");
}
