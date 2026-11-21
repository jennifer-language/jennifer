# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# ical_test.j - white-box tests for ical.j. Run with:
#
#     jennifer test modules/ical_test.j
#
# The overlay splices ical.j in front of this file, so the tests reach its
# private helpers (escapeText, unescapeText, fold, unfold, formatDateTime,
# parseDt, propName) by bare identifier as well as its exported surface.
# ical.j already `use`s strings / lists / time, so the overlay only adds testing.
use encoding;
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
    def long as string init "SUMMARY:" + strings.replicate("x", 200);
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

func testParseDtForms() {
    testing.assertTrue(time.equal(
        parseDt("20240615T130000Z", false, 1, "X"),
        at("2024-06-15T13:00:00Z")));
    testing.assertTrue(time.equal(
        parseDt("20240615T130000", false, 1, "X"),
        at("2024-06-15T13:00:00Z")));
    testing.assertTrue(time.equal(parseDt("20240615", true, 1, "X"), at("2024-06-15T00:00:00Z")));
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

# --- all-day / TZID / recurrence / VALARM / attendees / VTODO ---

func testAllDayRoundTrip() {
    def ev as Event init withAllDay(
        event("h", at("2024-06-15T00:00:00Z"), at("2024-06-16T00:00:00Z"), "Holiday"),
        true);
    def text as string init encode(add(calendar(), $ev));
    testing.assertContains($text, "DTSTART;VALUE=DATE:20240615\r\n");
    def back as Event init parse($text).events[0];
    testing.assertTrue($back.allDay);
}

func testTzidRoundTrip() {
    def ev as Event init withZone(
        event("z", at("2024-06-15T13:00:00Z"), at("2024-06-15T14:00:00Z"), "Local"),
        "Europe/London");
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
    def ev as Event init recur(
        event("r", at("2024-06-15T13:00:00Z"), at("2024-06-15T14:00:00Z"), "Standup"),
        "FREQ=WEEKLY;COUNT=3");
    def back as Event init parse(encode(add(calendar(), $ev))).events[0];
    testing.assertEqual($back.rrule, "FREQ=WEEKLY;COUNT=3");
}

func testOccurrencesWeekly() {
    def ev as Event init recur(
        event("r", at("2024-06-15T13:00:00Z"), at("2024-06-15T14:00:00Z"), "S"),
        rule("WEEKLY", 1, 3));
    def occ as list of time.Time init occurrences($ev, 10);
    testing.assertEqual(len($occ), 3);
    testing.assertTrue(time.equal($occ[0], at("2024-06-15T13:00:00Z")));
    testing.assertTrue(time.equal($occ[2], at("2024-06-29T13:00:00Z")));
}

# A MONTHLY instance on a day the month lacks is skipped and not counted (RFC
# 5545 3.3.10), never clamped: Jan 31 -> Mar 31 -> May 31 -> Jul 31.
func testOccurrencesMonthlySkipsInvalidDates() {
    def ev as Event init recur(
        event("m", at("2024-01-31T09:00:00Z"), at("2024-01-31T09:00:00Z"), "M"),
        rule("MONTHLY", 1, 4));
    def occ as list of time.Time init occurrences($ev, 10);
    testing.assertEqual(len($occ), 4);
    testing.assertTrue(time.equal($occ[1], at("2024-03-31T09:00:00Z")));
    testing.assertTrue(time.equal($occ[2], at("2024-05-31T09:00:00Z")));
    testing.assertTrue(time.equal($occ[3], at("2024-07-31T09:00:00Z")));
    def leap as Event init recur(
        event("y", at("2024-02-29T09:00:00Z"), at("2024-02-29T09:00:00Z"), "Y"),
        rule("YEARLY", 1, 2));
    def yo as list of time.Time init occurrences($leap, 10);
    testing.assertEqual(len($yo), 2);
    testing.assertTrue(time.equal($yo[1], at("2028-02-29T09:00:00Z")));
}

func testOccurrencesExdateAndRdate() {
    def ev as Event init recur(
        event("d", at("2024-06-01T08:00:00Z"), at("2024-06-01T08:00:00Z"), "D"),
        rule("DAILY", 1, 5));
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
    def ev as Event init addAlarm(
        event("a", at("2024-06-15T13:00:00Z"), at("2024-06-15T14:00:00Z"), "Meet"),
        alarm("DISPLAY", "-PT15M", "Reminder"));
    $ev = describe($ev, "Event body");
    def back as Event init parse(encode(add(calendar(), $ev))).events[0];
    testing.assertEqual($back.description, "Event body");
    testing.assertEqual(len($back.alarms), 1);
    testing.assertEqual($back.alarms[0].action, "DISPLAY");
    testing.assertEqual($back.alarms[0].trigger, "-PT15M");
    testing.assertEqual($back.alarms[0].description, "Reminder");
}

func testOrganizerAttendeeRoundTrip() {
    def ev as Event init withOrganizer(
        event("o", at("2024-06-15T13:00:00Z"), at("2024-06-15T14:00:00Z"), "Sync"),
        "mailto:boss@x.com");
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
    def ev as Event init addAttendee(
        event("x", at("2024-06-15T13:00:00Z"), at("2024-06-15T14:00:00Z"), "M"),
        attendee("mailto:b@x", "Bob: The 3rd", "REQ"));
    def text as string init encode(add(calendar(), $ev));
    testing.assertContains($text, "ATTENDEE;CN=\"Bob: The 3rd\";ROLE=REQ:mailto:b@x\r\n");
    def back as Event init parse($text).events[0];
    testing.assertEqual($back.attendees[0].cn, "Bob: The 3rd");
    testing.assertEqual($back.attendees[0].address, "mailto:b@x");
}

func testVtodoRoundTrip() {
    def td as Todo init withStatus(
        withDue(todo("t1", at("2024-06-15T13:00:00Z"), "Ship it"), at("2024-06-20T17:00:00Z")),
        "NEEDS-ACTION");
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

# A parameter value holding a quote or a line break cannot break out of its
# quoted string or line: RFC 6868 caret-encodes them, and paramValue decodes.
func testParamValueCannotInject() {
    def cn as string init "Bob\";ROLE=CHAIR;X=\"y";
    def line as string init emitLine("ATTENDEE;CN=" + quoteParam($cn), "mailto:b@x.org");
    # The quote is caret-encoded, so the `;` stay inside one quoted value.
    testing.assertFalse(strings.contains($line, "Bob\";"));
    def nameSection as string init $line[..strings.indexOf($line, ":mailto")];
    testing.assertEqual(paramValue($nameSection, "CN"), $cn);
    testing.assertEqual(paramValue($nameSection, "ROLE"), "");
    def multi as string init emitLine("X;CN=" + quoteParam("a\nb^c"), "v");
    testing.assertFalse(strings.contains($multi, "\n"));
    testing.assertEqual(paramValue($multi[..strings.indexOf($multi, ":v")], "CN"), "a\nb^c");
}

# A long line folds into 75-octet physical lines on rune boundaries.
func testFoldLongLine() {
    def folded as string init fold("X:" + strings.replicate("é", 200));
    for (def part in strings.split($folded, "\r\n")) {
        testing.assertTrue(len(encoding.encode("utf-8", $part)) <= 75);
    }
    testing.assertEqual(unfold($folded), "X:" + strings.replicate("é", 200));
}

# --- strict parsing: IcalError ----------------------------------------------

# calText wraps VEVENT properties (one per line, LF-separated) in a calendar;
# the properties start on line 5.
func calText(props as string) {
    def body as string init "BEGIN:VCALENDAR\nVERSION:2.0\nBEGIN:VEVENT\nUID:u\n" + $props +
        "END:VEVENT\nEND:VCALENDAR\n";
    return strings.replace($body, "\n", "\r\n");
}

# parseFault returns "kind@line" of the error parse throws, or "ok".
func parseFault(text as string) {
    try {
        parse($text);
    } catch (e) {
        if ($e.kind == "ical") {
            def ie as IcalError init $e;
            return "ical@{$ie.line}";
        }
        return $e.kind;
    }
    return "ok";
}

# occurrencesFault returns the kind of the error occurrences throws, or "ok".
func occurrencesFault(rrule as string, max as int) {
    def ev as Event init recur(
        event("r", at("2024-01-31T10:00:00Z"), at("2024-01-31T11:00:00Z"), "R"),
        $rrule);
    try {
        occurrences($ev, $max);
    } catch (e) {
        return $e.kind;
    }
    return "ok";
}

func testParseRejectsStructuralFaults() {
    testing.assertEqual(parseFault(""), "ical@0");
    testing.assertEqual(parseFault("hello world"), "ical@1");
    testing.assertEqual(parseFault("BEGIN:VEVENT\r\nUID:u\r\nEND:VEVENT\r\n"), "ical@1");
    testing.assertEqual(parseFault("UID:u\r\n"), "ical@1");
    testing.assertEqual(parseFault(calText("DTSTART:20240615T130000Z\nNO COLON HERE\n")), "ical@6");
    # missing END: reported at the unclosed BEGIN
    testing.assertEqual(
        parseFault("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:u\r\nEND:VEVENT\r\n"),
        "ical@1");
    # mismatched END
    testing.assertEqual(
        parseFault("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"),
        "ical@3");
    # a VEVENT inside a VEVENT
    testing.assertEqual(parseFault(calText("BEGIN:VEVENT\nEND:VEVENT\n")), "ical@5");
    # a property after the calendar
    testing.assertEqual(parseFault(calText("") + "X-AFTER:1\r\n"), "ical@7");
}

func testParseFoldedLineNumbers() {
    # The bad line starts on physical line 7, after a value folded over lines 5-6.
    testing.assertEqual(parseFault(calText("SUMMARY:a\n b\nDTSTART:2024\n")), "ical@7");
}

func testParseRejectsBadEscapes() {
    testing.assertEqual(parseFault(calText("DTSTART:20240615T130000Z\nSUMMARY:a\\qb\n")), "ical@6");
    testing.assertEqual(parseFault(calText("DTSTART:20240615T130000Z\nSUMMARY:ab\\\n")), "ical@6");
    testing.assertEqual(
        parse(calText("DTSTART:20240615T130000Z\nSUMMARY:a\\Nb\\\\c\n")).events[0].summary,
        "a\nb\\c");
}

func testParseBadDatesAreIcalErrors() {
    testing.assertEqual(parseFault(calText("DTSTART:20240231T130000Z\n")), "ical@5");
    testing.assertEqual(parseFault(calText("DTSTART:20240615T250000Z\n")), "ical@5");
    testing.assertEqual(parseFault(calText("DTSTART:hello\n")), "ical@5");
    testing.assertEqual(parseFault(calText("DTSTART:20240615T130000+0200\n")), "ical@5");
    testing.assertEqual(parseFault(calText("DTSTART;VALUE=DATE:20241315\n")), "ical@5");
    testing.assertEqual(parseFault(calText("DTSTART;VALUE=DATE:20240615T130000\n")), "ical@5");
    testing.assertEqual(
        parseFault(calText("DTSTART:20240615T130000Z\nEXDATE:20240616T130000Z,junk\n")),
        "ical@6");
    testing.assertEqual(
        parseFault(calText("DTSTART:20240615T130000Z\nRDATE;VALUE=PERIOD:20240616T130000Z/PT1H\n")),
        "ical@6");
}

func testParseDuration() {
    def ev as Event init parse(calText("DTSTART:20240615T130000Z\nDURATION:PT1H30M\n")).events[0];
    testing.assertTrue(time.equal($ev.end, at("2024-06-15T14:30:00Z")));
    $ev = parse(calText("DTSTART:20240615T130000Z\nDURATION:P1DT2H\n")).events[0];
    testing.assertTrue(time.equal($ev.end, at("2024-06-16T15:00:00Z")));
    $ev = parse(calText("DTSTART;VALUE=DATE:20240615\nDURATION:P2W\n")).events[0];
    testing.assertTrue(time.equal($ev.end, at("2024-06-29T00:00:00Z")));
    testing.assertEqual(
        parseFault(calText("DTSTART:20240615T130000Z\nDURATION:-PT1H\n")),
        "ical@6");
    testing.assertEqual(
        parseFault(calText("DTSTART:20240615T130000Z\nDURATION:PT1H5S\n")),
        "ical@6");
    testing.assertEqual(parseFault(calText("DTSTART:20240615T130000Z\nDURATION:P\n")), "ical@6");
    testing.assertEqual(
        parseFault(calText("DTSTART:20240615T130000Z\nDURATION:P1W2D\n")),
        "ical@6");
    testing.assertEqual(
        parseFault(calText("DTSTART:20240615T130000Z\nDURATION:PT9999999999H\n")),
        "ical@6");
}

func testParseEndChecks() {
    testing.assertEqual(
        parseFault(calText("DTSTART:20240615T130000Z\nDTEND:20240615T140000Z\nDURATION:PT1H\n")),
        "ical@7");
    testing.assertEqual(
        parseFault(calText("DTSTART:20240615T130000Z\nDTEND:20240614T130000Z\n")),
        "ical@6");
    testing.assertEqual(
        parseFault(calText("DTSTART;VALUE=DATE:20240615\nDTEND:20240616T000000Z\n")),
        "ical@6");
}

func testParseAllDayWithoutEndLastsOneDay() {
    def ev as Event init parse(calText("DTSTART;VALUE=DATE:20240615\n")).events[0];
    testing.assertTrue($ev.allDay);
    testing.assertTrue(time.equal($ev.end, at("2024-06-16T00:00:00Z")));
}

func testParseChecksRruleSyntax() {
    testing.assertEqual(parseFault(calText("DTSTART:20240615T130000Z\nRRULE:FOO=BAR\n")), "ical@6");
    testing.assertEqual(parseFault(calText("DTSTART:20240615T130000Z\nRRULE:COUNT=2\n")), "ical@6");
    # BY* parts are valid RRULE syntax and round-trip, though occurrences rejects them.
    testing.assertEqual(
        parse(calText("DTSTART:20240615T130000Z\nRRULE:FREQ=WEEKLY;BYDAY=MO,WE\n")).events[0].rrule,
        "FREQ=WEEKLY;BYDAY=MO,WE");
}

func testParseErrorIsIcalError() {
    def caught as bool init false;
    try {
        parse(calText("DTSTART:20240615T130000Z\nDTEND:x\n"));
    } catch (e) {
        $caught = true;
        def ie as IcalError init $e;
        testing.assertEqual($ie.kind, "ical");
        testing.assertEqual($ie.line, 6);
        testing.assertTrue(strings.startsWith($ie.message, "line 6: DTEND"));
    }
    testing.assertTrue($caught);
}

# --- recurrence: frequencies, validation, limits ----------------------------

func testOccurrencesSubDaily() {
    def ev as Event init recur(
        event("s", at("2024-01-31T10:00:00Z"), at("2024-01-31T10:00:00Z"), "S"),
        "FREQ=HOURLY;INTERVAL=2;COUNT=3");
    def occ as list of time.Time init occurrences($ev, 10);
    testing.assertEqual(len($occ), 3);
    testing.assertTrue(time.equal($occ[2], at("2024-01-31T14:00:00Z")));
    $ev = recur($ev, "FREQ=MINUTELY;COUNT=2");
    testing.assertTrue(time.equal(occurrences($ev, 10)[1], at("2024-01-31T10:01:00Z")));
    $ev = recur($ev, "FREQ=SECONDLY;COUNT=2");
    testing.assertTrue(time.equal(occurrences($ev, 10)[1], at("2024-01-31T10:00:01Z")));
}

func testOccurrencesRejectsBadRules() {
    for (def rr in [
        "FREQ=DAILY;INTERVAL=0;COUNT=3",
        "FREQ=DAILY;INTERVAL=-1",
        "FREQ=DAILY;COUNT=-3",
        "FREQ=DAILY;COUNT=abc",
        "FREQ=DAILY;COUNT=99999999999999999999",
        "FREQ=DAILY;UNTIL=garbage",
        "FREQ=DAILY;UNTIL=20240203",
        "FREQ=FORTNIGHTLY",
        "COUNT=3",
        "FOO=BAR",
        "FREQ=DAILY;;COUNT=2",
        "FREQ=DAILY;FREQ=WEEKLY",
        "FREQ=DAILY;COUNT=3;UNTIL=20240203T000000Z",
        "FREQ=WEEKLY;BYDAY=MO,WE",
        "FREQ=MONTHLY;BYMONTHDAY=-1",
        "FREQ=WEEKLY;WKST=XX"
    ]) {
        testing.assertEqual(occurrencesFault($rr, 10), "ical");
    }
    testing.assertEqual(occurrencesFault("FREQ=WEEKLY;WKST=MO;COUNT=2", 10), "ok");
}

func testOccurrencesUntilInclusive() {
    def ev as Event init recur(
        event("u", at("2024-01-31T10:00:00Z"), at("2024-01-31T10:00:00Z"), "U"),
        "FREQ=DAILY;UNTIL=20240202T100000Z");
    testing.assertEqual(len(occurrences($ev, 10)), 3);
}

func testOccurrencesLimit() {
    testing.assertEqual(occurrencesFault("FREQ=MINUTELY", 200000), "limit");
    testing.assertEqual(occurrencesFault("FREQ=HOURLY;COUNT=150000", 150000), "limit");
}

func testOccurrencesTimeOverflowIsIcalError() {
    testing.assertEqual(occurrencesFault("FREQ=DAILY;INTERVAL=999999999", 5), "ical");
    testing.assertEqual(occurrencesFault("FREQ=YEARLY;INTERVAL=999999999", 5), "ical");
}

# DTSTART is always part of the set, even with RDATEs and no RRULE; RDATEs merge
# in date order without duplicates.
func testOccurrencesRdatesSortedWithStart() {
    def ev as Event init event("r", at("2024-01-10T10:00:00Z"), at("2024-01-10T11:00:00Z"), "R");
    $ev = addRdate($ev, at("2024-01-05T10:00:00Z"));
    def occ as list of time.Time init occurrences($ev, 10);
    testing.assertEqual(len($occ), 2);
    testing.assertTrue(time.equal($occ[0], at("2024-01-05T10:00:00Z")));
    testing.assertTrue(time.equal($occ[1], at("2024-01-10T10:00:00Z")));
    $ev = recur($ev, "FREQ=DAILY;COUNT=2");
    $ev = addRdate($ev, at("2024-01-11T10:00:00Z"));
    $occ = occurrences($ev, 10);
    testing.assertEqual(len($occ), 3);
    testing.assertTrue(time.equal($occ[2], at("2024-01-11T10:00:00Z")));
    testing.assertEqual(len(occurrences($ev, 2)), 2);
}
