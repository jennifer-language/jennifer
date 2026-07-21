# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# cron_test.j - white-box tests for cron.j. Run with:
#
#     jennifer test modules/cron_test.j
#
# The overlay splices cron.j in first, so these tests reach its private helpers
# (parseField, parseTerm, dayMatches, fields) and the exported surface by bare
# identifier. cron.j already `use`s time / strings / convert / lists, so the
# overlay only adds testing.
use testing;

# at builds a UTC time.Time from an ISO string (convenience for the tests).
func at(iso as string) {
    return time.fromIso($iso);
}

func testParseFields() {
    def s as Schedule init parse("30 9 * * 1-5");
    testing.assertEqual(len($s.minutes), 1);
    testing.assertEqual($s.minutes[0], 30);
    testing.assertEqual($s.hours[0], 9);
    testing.assertEqual(len($s.weekdays), 5);        # Mon..Fri (ISO 1-5)
    testing.assertEqual($s.weekdays[0], 1);
    testing.assertTrue($s.domStar);
    testing.assertFalse($s.dowStar);
}

func testRangesStepsLists() {
    def s as Schedule init parse("0,30 */6 1-3 * *");
    testing.assertEqual(len($s.minutes), 2);
    testing.assertEqual($s.minutes[1], 30);
    testing.assertEqual(len($s.hours), 4);           # 0,6,12,18
    testing.assertEqual($s.hours[3], 18);
    testing.assertEqual(len($s.daysOfMonth), 3);     # 1,2,3
    testing.assertEqual($s.daysOfMonth[2], 3);
}

func testSundayAliases() {
    testing.assertEqual(parse("0 0 * * 0").weekdays[0], 7);   # cron 0 -> ISO 7
    testing.assertEqual(parse("0 0 * * 7").weekdays[0], 7);   # cron 7 -> ISO 7
}

func testMatches() {
    def s as Schedule init parse("30 9 * * 1-5");
    testing.assertTrue(matches($s, at("2026-03-16T09:30:00+00:00")));    # Monday 09:30
    testing.assertFalse(matches($s, at("2026-03-16T09:31:00+00:00")));   # wrong minute
    testing.assertFalse(matches($s, at("2026-03-15T09:30:00+00:00")));   # Sunday
    testing.assertTrue(matches($s, at("2026-03-16T09:30:45+00:00")));    # seconds ignored
}

func testNextWeekday() {
    def s as Schedule init parse("30 9 * * 1-5");
    # from Saturday 10:30 -> Monday 09:30
    testing.assertEqual(time.iso(next($s, at("2026-03-14T10:30:00+00:00"))), "2026-03-16T09:30:00Z");
}

func testNextStep() {
    def s as Schedule init parse("*/15 * * * *");
    testing.assertEqual(time.iso(next($s, at("2026-03-14T10:31:00+00:00"))), "2026-03-14T10:45:00Z");
    # at an exact matching minute, "at or after" returns that minute
    testing.assertEqual(time.iso(next($s, at("2026-03-14T10:45:00+00:00"))), "2026-03-14T10:45:00Z");
}

func testNextMonthRollover() {
    def s as Schedule init parse("0 0 1 * *");
    testing.assertEqual(time.iso(next($s, at("2026-03-14T10:30:00+00:00"))), "2026-04-01T00:00:00Z");
}

func testDomOrDowRule() {
    # "13th OR any Friday" - both fields restricted, so either matches
    def s as Schedule init parse("0 0 13 * 5");
    testing.assertTrue(matches($s, at("2026-03-20T00:00:00+00:00")));    # a Friday
    testing.assertTrue(matches($s, at("2026-02-13T00:00:00+00:00")));    # the 13th (a Friday too)
    testing.assertTrue(matches($s, at("2026-01-13T00:00:00+00:00")));    # the 13th (a Tuesday)
    testing.assertFalse(matches($s, at("2026-03-19T00:00:00+00:00")));   # Thursday, not the 13th
}

func testKeepsOffset() {
    # next preserves the input's zone offset
    def s as Schedule init parse("0 12 * * *");
    testing.assertEqual(time.iso(next($s, at("2026-03-14T08:00:00+02:00"))), "2026-03-14T12:00:00+02:00");
}

# --- error cases ------------------------------------------------------------

func badFieldCount() {
    parse("30 9 * *");     # only 4 fields
}

func badRange() {
    parse("99 9 * * *");   # minute 99 out of range
}

func badStep() {
    parse("*/0 * * * *");  # zero step
}

func testErrors() {
    testing.assertThrows("badFieldCount", "cron");
    testing.assertThrows("badRange", "cron");
    testing.assertThrows("badStep", "cron");
}

func testHelpers() {
    testing.assertEqual(len(fields("  30   9 * * 1  ")), 5);       # collapses whitespace
    testing.assertEqual(len(parseTerm("1-5", 0, 59, "minute")), 5);
    testing.assertEqual(len(parseTerm("*/20", 0, 59, "minute")), 3);   # 0,20,40
    def s as Schedule init parse("0 0 * * *");
    testing.assertTrue(dayMatches($s, at("2026-03-14T00:00:00+00:00")));   # both day fields *
}
