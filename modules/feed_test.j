# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# feed_test.j - white-box tests for feed.j. Run with:
#
#     jennifer test modules/feed_test.j
#
# The overlay splices feed.j in front of this file, so the tests reach its
# private helpers (rssDate, parseRssDate, parseAtomDate, textOf, kindOf, epoch,
# isSet) by bare identifier as well as its exported surface. Build / parse
# round-trips and date handling are pure; the networked fetch is covered by the
# Go test (cmd/jennifer/feed_test.go).
use testing;

# A fixed instant used across the round-trip tests.
func when() {
    return time.fromIso("2026-07-20T14:30:00Z");
}

# sample builds a one-entry feed with every field set.
func sample() {
    def e as Entry init entryContent(
        entrySummary(
            entryUpdated(
                entryPublished(
                    entryId(entry("First Post", "https://example.org/1"), "id-1"),
                    when()),
                when()),
            "A summary & <tag>"),
        "Full body");
    return add(feedUpdated(feed("My Blog", "https://example.org"), when()), $e);
}

func testFeedConstructorDefaults() {
    def f as Feed init feed("T", "L");
    testing.assertEqual($f.title, "T");
    testing.assertEqual($f.link, "L");
    testing.assertEqual(len($f.entries), 0);
    testing.assertFalse(isSet($f.updated));            # unset by default
}

func testEntryConstructorDefaults() {
    def e as Entry init entry("Title", "Link");
    testing.assertEqual($e.title, "Title");
    testing.assertEqual($e.id, "");
    testing.assertEqual($e.summary, "");
    testing.assertEqual($e.content, "");
    testing.assertFalse(isSet($e.published));
}

func testBuildersAreValueSemantic() {
    def base as Entry init entry("t", "l");
    def withId as Entry init entryId($base, "x");
    testing.assertEqual($base.id, "");                 # original untouched
    testing.assertEqual($withId.id, "x");
    def f as Feed init feed("t", "l");
    def added as Feed init add($f, $base);
    testing.assertEqual(len($f.entries), 0);           # original untouched
    testing.assertEqual(len($added.entries), 1);
}

func testKindDetection() {
    testing.assertEqual(kind(build(sample(), "rss")), "rss");
    testing.assertEqual(kind(build(sample(), "atom")), "atom");
}

func testBuildRejectsUnknownFormat() {
    testing.assertThrows("buildJson", "value");
}
func buildJson() { build(sample(), "json"); }

func testParseRejectsNonFeed() {
    testing.assertThrows("parseHtml", "value");
}
func parseHtml() { parse("<html><body>nope</body></html>"); }

func testBuildRssStructure() {
    def out as string init build(sample(), "rss");
    testing.assertContains($out, "<rss");
    testing.assertContains($out, "<channel>");
    testing.assertContains($out, "<item>");
    testing.assertContains($out, "<guid>id-1</guid>");
    testing.assertContains($out, "<pubDate>");
}

func testBuildAtomStructure() {
    def out as string init build(sample(), "atom");
    testing.assertContains($out, "<feed");
    testing.assertContains($out, "<entry>");
    testing.assertContains($out, "<id>id-1</id>");
    testing.assertContains($out, "href=\"https://example.org/1\"");
    testing.assertContains($out, "<content>");
}

func testRssRoundTrip() {
    def f as Feed init parse(build(sample(), "rss"));
    testing.assertEqual($f.title, "My Blog");
    testing.assertEqual($f.link, "https://example.org");
    testing.assertEqual(len($f.entries), 1);
    def e as Entry init $f.entries[0];
    testing.assertEqual($e.title, "First Post");
    testing.assertEqual($e.link, "https://example.org/1");
    testing.assertEqual($e.id, "id-1");
    testing.assertEqual($e.summary, "A summary & <tag>");     # entities survive
    testing.assertEqual(time.iso($e.published), "2026-07-20T14:30:00Z");
}

func testAtomRoundTrip() {
    def f as Feed init parse(build(sample(), "atom"));
    testing.assertEqual($f.title, "My Blog");
    testing.assertEqual($f.link, "https://example.org");
    testing.assertEqual(time.iso($f.updated), "2026-07-20T14:30:00Z");
    def e as Entry init $f.entries[0];
    testing.assertEqual($e.title, "First Post");
    testing.assertEqual($e.link, "https://example.org/1");   # from href attribute
    testing.assertEqual($e.id, "id-1");
    testing.assertEqual($e.content, "Full body");
    testing.assertEqual(time.iso($e.published), "2026-07-20T14:30:00Z");
    testing.assertEqual(time.iso($e.updated), "2026-07-20T14:30:00Z");
}

func testEscapingRoundTrip() {
    def e as Entry init entrySummary(entry("Tom & Jerry <fun>", "l"), "a < b && c > d");
    def f as Feed init add(feed("F", "L"), $e);
    def rss as Entry init parse(build($f, "rss")).entries[0];
    testing.assertEqual($rss.title, "Tom & Jerry <fun>");
    testing.assertEqual($rss.summary, "a < b && c > d");
    def atom as Entry init parse(build($f, "atom")).entries[0];
    testing.assertEqual($atom.title, "Tom & Jerry <fun>");
}

func testUnsetDatesOmitted() {
    def f as Feed init add(feed("F", "L"), entry("e", "l"));    # no dates set
    def rss as string init build($f, "rss");
    testing.assertFalse(containsStr($rss, "<pubDate>"));
    testing.assertFalse(containsStr($rss, "<lastBuildDate>"));
    def atom as string init build($f, "atom");
    testing.assertFalse(containsStr($atom, "<updated>"));
    testing.assertFalse(containsStr($atom, "<published>"));
    # And round-trip leaves them unset.
    testing.assertFalse(isSet(parse($rss).entries[0].published));
}

# containsStr: a small substring predicate (strings library isn't imported by
# the module, so the test defines its own).
func containsStr(haystack as string, needle as string) {
    def hay as list of string init strings.chars($haystack);
    def nee as list of string init strings.chars($needle);
    if (len($nee) == 0) {
        return true;
    }
    for (def i as int init 0; $i + len($nee) <= len($hay); $i = $i + 1) {
        def match as bool init true;
        for (def j as int init 0; $j < len($nee); $j = $j + 1) {
            if ($hay[$i + $j] != $nee[$j]) {
                $match = false;
            }
        }
        if ($match) {
            return true;
        }
    }
    return false;
}
use strings;

func testRssDateFormat() {
    testing.assertEqual(rssDate(when()), "Mon, 20 Jul 2026 14:30:00 +0000");
}

func testParseRssDateBothForms() {
    testing.assertEqual(time.iso(parseRssDate("Mon, 20 Jul 2026 14:30:00 +0000")),
        "2026-07-20T14:30:00Z");
    # Bare form (no weekday) also parses.
    testing.assertEqual(time.iso(parseRssDate("20 Jul 2026 14:30:00 +0000")),
        "2026-07-20T14:30:00Z");
}

func testParseDateLenient() {
    # A malformed date degrades to the epoch sentinel, not an error.
    testing.assertFalse(isSet(parseRssDate("not a date")));
    testing.assertFalse(isSet(parseAtomDate("garbage")));
}

func testParseAtomDate() {
    testing.assertEqual(time.iso(parseAtomDate("2026-07-20T14:30:00Z")),
        "2026-07-20T14:30:00Z");
}

func testTextOfAndKindOf() {
    def root as xml.Value init xml.decode("<rss><channel><title>Hi</title></channel></rss>");
    testing.assertEqual(textOf(xml.get($root, "channel"), "title"), "Hi");
    testing.assertEqual(textOf(xml.get($root, "channel"), "missing"), "");
    testing.assertEqual(kindOf($root), "rss");
    testing.assertEqual(kindOf(xml.decode("<feed></feed>")), "atom");
}

func testAtomLink() {
    def e as xml.Value init xml.decode("<entry><link href=\"http://x/1\"/></entry>");
    testing.assertEqual(atomLink($e), "http://x/1");
    testing.assertEqual(atomLink(xml.decode("<entry></entry>")), "");
}

func testEmptyFeedRoundTrips() {
    def f as Feed init feed("Empty", "https://x");
    testing.assertEqual(len(parse(build($f, "rss")).entries), 0);
    testing.assertEqual(len(parse(build($f, "atom")).entries), 0);
}
