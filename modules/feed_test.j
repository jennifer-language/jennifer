# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
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
func nowStamp() {
    return time.fromIso("2026-07-20T14:30:00Z");
}

# sample builds a one-entry feed with every field set.
func sample() {
    def e as Entry init entryContent(
        entrySummary(
            entryUpdated(
                entryPublished(
                    entryId(entry("First Post", "https://example.org/1"), "id-1"),
                    nowStamp()),
                nowStamp()),
            "A summary & <tag>"),
        "Full body");
    return add(feedUpdated(feed("My Blog", "https://example.org"), nowStamp()), $e);
}

func testFeedConstructorDefaults() {
    def f as Feed init feed("T", "L");
    testing.assertEqual($f.title, "T");
    testing.assertEqual($f.link, "L");
    testing.assertEqual(len($f.entries), 0);
    testing.assertFalse(isSet($f.updated)); # unset by default
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
    testing.assertEqual($base.id, ""); # original untouched
    testing.assertEqual($withId.id, "x");
    def f as Feed init feed("t", "l");
    def added as Feed init add($f, $base);
    testing.assertEqual(len($f.entries), 0); # original untouched
    testing.assertEqual(len($added.entries), 1);
}

func testKindDetection() {
    testing.assertEqual(kind(build("rss", sample())), "rss");
    testing.assertEqual(kind(build("atom", sample())), "atom");
}

func testBuildRejectsUnknownFormat() {
    testing.assertThrows("buildJson", "value");
}
func buildJson() {
    build("json", sample());
}

func testParseRejectsNonFeed() {
    testing.assertThrows("parseHtml", "value");
}
func parseHtml() {
    parse("<html><body>nope</body></html>");
}

func testBuildRssStructure() {
    def out as string init build("rss", sample());
    testing.assertContains($out, "<rss");
    testing.assertContains($out, "<channel>");
    testing.assertContains($out, "<item>");
    testing.assertContains($out, "<guid>id-1</guid>");
    testing.assertContains($out, "<pubDate>");
}

func testBuildAtomStructure() {
    def out as string init build("atom", sample());
    testing.assertContains($out, "<feed");
    testing.assertContains($out, "<entry>");
    testing.assertContains($out, "<id>id-1</id>");
    testing.assertContains($out, "href=\"https://example.org/1\"");
    testing.assertContains($out, "<content>");
}

func testRssRoundTrip() {
    def f as Feed init parse(build("rss", sample()));
    testing.assertEqual($f.title, "My Blog");
    testing.assertEqual($f.link, "https://example.org");
    testing.assertEqual(len($f.entries), 1);
    def e as Entry init $f.entries[0];
    testing.assertEqual($e.title, "First Post");
    testing.assertEqual($e.link, "https://example.org/1");
    testing.assertEqual($e.id, "id-1");
    testing.assertEqual($e.summary, "A summary & <tag>"); # entities survive
    testing.assertEqual(time.iso($e.published), "2026-07-20T14:30:00Z");
}

func testAtomRoundTrip() {
    def f as Feed init parse(build("atom", sample()));
    testing.assertEqual($f.title, "My Blog");
    testing.assertEqual($f.link, "https://example.org");
    testing.assertEqual(time.iso($f.updated), "2026-07-20T14:30:00Z");
    def e as Entry init $f.entries[0];
    testing.assertEqual($e.title, "First Post");
    testing.assertEqual($e.link, "https://example.org/1"); # from href attribute
    testing.assertEqual($e.id, "id-1");
    testing.assertEqual($e.content, "Full body");
    testing.assertEqual(time.iso($e.published), "2026-07-20T14:30:00Z");
    testing.assertEqual(time.iso($e.updated), "2026-07-20T14:30:00Z");
}

func testEscapingRoundTrip() {
    def e as Entry init entrySummary(entry("Tom & Jerry <fun>", "l"), "a < b && c > d");
    def f as Feed init add(feed("F", "L"), $e);
    def rss as Entry init parse(build("rss", $f)).entries[0];
    testing.assertEqual($rss.title, "Tom & Jerry <fun>");
    testing.assertEqual($rss.summary, "a < b && c > d");
    def atom as Entry init parse(build("atom", $f)).entries[0];
    testing.assertEqual($atom.title, "Tom & Jerry <fun>");
}

func testUnsetDatesOmitted() {
    def f as Feed init add(feed("F", "L"), entry("e", "l")); # no dates set
    def rss as string init build("rss", $f);
    testing.assertFalse(containsStr($rss, "<pubDate>"));
    testing.assertFalse(containsStr($rss, "<lastBuildDate>"));
    def atom as string init build("atom", $f);
    testing.assertFalse(containsStr($atom, "<updated>"));
    testing.assertFalse(containsStr($atom, "<published>"));
    # And round-trip leaves them unset.
    testing.assertFalse(isSet(parse($rss).entries[0].published));
}

# containsStr: a small substring predicate (the module does not `use strings`,
# so the test imports it below).
func containsStr(haystack as string, needle as string) {
    return strings.contains($haystack, $needle);
}
use strings;

func testRssDateFormat() {
    testing.assertEqual(rssDate(nowStamp()), "Mon, 20 Jul 2026 14:30:00 +0000");
}

func testParseRssDateBothForms() {
    testing.assertEqual(
        time.iso(parseRssDate("Mon, 20 Jul 2026 14:30:00 +0000")),
        "2026-07-20T14:30:00Z");
    # Bare form (no weekday) also parses.
    testing.assertEqual(
        time.iso(parseRssDate("20 Jul 2026 14:30:00 +0000")),
        "2026-07-20T14:30:00Z");
}

func rssBadDate() {
    parseRssDate("not a date");
}
func atomBadDate() {
    parseAtomDate("garbage");
}
func testParseDateStrict() {
    # A malformed date is an error, not a silent epoch.
    testing.assertThrows("rssBadDate", "value");
    testing.assertThrows("atomBadDate", "value");
}

# The RFC 822 forms RSS feeds use: GMT / UT / Z and North American zone names,
# a 1-digit day, no seconds, no day name, a 2-digit year, any case.
func testParseRssDateRfc822Forms() {
    def want as int init 1136214245; # 2006-01-02T15:04:05Z
    for (def d in [
        "Mon, 02 Jan 2006 15:04:05 GMT",
        "Mon, 02 Jan 2006 15:04:05 UT",
        "Mon, 02 Jan 2006 15:04:05 Z",
        "Mon, 2 Jan 2006 15:04:05 +0000",
        "Mon, 02 Jan 2006 10:04:05 EST",
        "Mon, 02 Jan 2006 08:04:05 PDT",
        "Mon,02 Jan 2006 15:04:05 GMT",
        "02 Jan 06 15:04:05 GMT",
        "  mon, 02 JAN 2006 15:04:05 gmt  "
    ]) {
        testing.assertEqual(time.unix(parseRssDate($d)), $want);
    }
    testing.assertEqual(
        time.iso(parseRssDate("Mon, 02 Jan 2006 15:04 +0000")),
        "2006-01-02T15:04:00Z");
    testing.assertEqual(time.iso(parseRssDate("1 Jan 99 00:00 GMT")), "1999-01-01T00:00:00Z");
}

func badRssDate(d as string) {
    try {
        parseRssDate($d);
    } catch (e) {
        return $e.kind;
    }
    return "";
}
func testParseRssDateRefusesBadDates() {
    for (def d in [
        "",
        "Mon, 30 Feb 2006 15:04:05 GMT",
        "Mon, 02 Jan 2006 24:00:00 GMT",
        "Mon, 02 Jan 2006 15:04:05",
        "Mon, 02 Jan 2006 15:04:05 A",
        "Mon, 02 Jan 2006 15:04:05 +9999",
        "Xyz, 02 Jan 2006 15:04:05 GMT",
        "02 Jan 206 15:04:05 GMT",
        "2006-01-02T15:04:05Z"
    ]) {
        testing.assertEqual(badRssDate($d), "value");
    }
}

# parseKind parses a document and returns the kind it threw ("" when none).
func parseKind(doc as string) {
    try {
        parse($doc);
    } catch (e) {
        return $e.kind;
    }
    return "";
}

func testABadDateFailsTheParse() {
    def rss as string init "<rss version=\"2.0\"><channel><title>C</title><item><title>i</title>" +
        "<pubDate>yesterday</pubDate></item></channel></rss>";
    testing.assertEqual(parseKind($rss), "value");
    def atom as string init "<feed xmlns=\"http://www.w3.org/2005/Atom\"><title>F</title>" +
        "<entry><title>E</title><updated>2024-01-02</updated></entry></feed>";
    testing.assertEqual(parseKind($atom), "value");
    # an absent date is unset, not an error
    def plain as string init "<rss version=\"2.0\"><channel><title>C</title>" +
        "<item><title>i</title>" +
        "</item></channel></rss>";
    testing.assertFalse(isSet(parse($plain).entries[0].published));
}

# An Atom type="xhtml" construct is the markup inside its XHTML <div>.
func testAtomXhtmlContent() {
    def doc as string init "<feed xmlns=\"http://www.w3.org/2005/Atom\"><title type=\"xhtml\">" +
        "<div xmlns=\"http://www.w3.org/1999/xhtml\">A <i>B</i></div></title>" +
        "<entry><title>E</title>" +
        "<content type=\"xhtml\"><div xmlns=\"http://www.w3.org/1999/xhtml\" class=\"a&gt;b\">" +
        "<p>Hello &amp; <b>world</b></p></div></content>" +
        "<summary type=\"html\">&lt;p&gt;hi&lt;/p&gt;</summary></entry></feed>";
    def f as Feed init parse($doc);
    testing.assertEqual($f.title, "A <i>B</i>");
    testing.assertEqual($f.entries[0].content, "<p>Hello &amp; <b>world</b></p>");
    testing.assertEqual($f.entries[0].summary, "<p>hi</p>");
    def empty as string init "<feed xmlns=\"http://www.w3.org/2005/Atom\"><entry><title>E</title>" +
        "<content type=\"xhtml\"><div xmlns=\"http://www.w3.org/1999/xhtml\"/></content>" +
        "</entry></feed>";
    testing.assertEqual(parse($empty).entries[0].content, "");
    # no div, a div outside XHTML, or text beside the div is malformed
    for (def body in [
        "<content type=\"xhtml\">plain</content>",
        "<content type=\"xhtml\"><div>x</div></content>",
        "<content type=\"xhtml\">t<div xmlns=\"http://www.w3.org/1999/xhtml\">x</div></content>"
    ]) {
        testing.assertEqual(
            parseKind("<feed xmlns=\"http://www.w3.org/2005/Atom\"><entry>" + $body +
                "</entry></feed>"),
            "value");
    }
}

func testBadEnclosureLengthFailsTheParse() {
    for (def length in ["-5", "abc", "", "1.5"]) {
        def doc as string init "<rss version=\"2.0\"><channel><title>C</title>" +
            "<item><title>i</title>" +
            "<enclosure url=\"https://x/a.mp3\" length=\"" + $length +
            "\" type=\"audio/mpeg\"/></item>" +
            "</channel></rss>";
        testing.assertEqual(parseKind($doc), "value");
    }
}

func testRssNeedsAChannel() {
    testing.assertEqual(
        parseKind("<rss version=\"2.0\"><title>T</title><item><title>i</title></item></rss>"),
        "value");
}

func testParseAtomDate() {
    testing.assertEqual(time.iso(parseAtomDate("2026-07-20T14:30:00Z")), "2026-07-20T14:30:00Z");
}

func testTextOfAndKindOf() {
    def root as xml.Value init xml.decode("<rss><channel><title>Hi</title></channel></rss>");
    testing.assertEqual(textOf(xml.get($root, "channel"), "title", {}), "Hi");
    testing.assertEqual(textOf(xml.get($root, "channel"), "missing", {}), "");
    testing.assertEqual(kindOf($root), "rss");
    testing.assertEqual(
        kindOf(xml.decode("<feed xmlns=\"http://www.w3.org/2005/Atom\"></feed>")),
        "atom");
}

func testAtomLink() {
    def e as xml.Value init xml.decode("<entry><link href=\"http://x/1\"/></entry>");
    testing.assertEqual(atomLink($e, {}), "http://x/1");
    testing.assertEqual(atomLink(xml.decode("<entry></entry>"), {}), "");
}

func testEmptyFeedRoundTrips() {
    def f as Feed init feed("Empty", "https://x");
    testing.assertEqual(len(parse(build("rss", $f)).entries), 0);
    testing.assertEqual(len(parse(build("atom", $f)).entries), 0);
}

# ---- enclosure, author, categories ----

# A podcast-style feed with author, categories and a media enclosure.
func podcast() {
    def e as Entry init entryEnclosure(
        entryCategory(
            entryCategory(entryAuthor(entry("Ep 1", "https://p.example/1"), "Jane Host"), "Tech"),
            "News"),
        "https://p.example/1.mp3",
        12345678,
        "audio/mpeg");
    def f as Feed init feedCategory(
        feedAuthor(feed("Show", "https://p.example"), "Jane Host"),
        "Technology");
    return add($f, $e);
}

func testPodcastRssRoundTrip() {
    def out as string init build("rss", podcast());
    testing.assertContains($out, "<managingEditor>Jane Host</managingEditor>");
    testing.assertContains($out, "<author>Jane Host</author>");
    testing.assertContains($out, "url=\"https://p.example/1.mp3\"");
    testing.assertContains($out, "length=\"12345678\"");
    def f as Feed init parse($out);
    testing.assertEqual($f.author, "Jane Host");
    testing.assertEqual($f.categories[0], "Technology");
    def e as Entry init $f.entries[0];
    testing.assertEqual($e.author, "Jane Host");
    testing.assertEqual(len($e.categories), 2);
    testing.assertEqual($e.categories[0], "Tech");
    testing.assertEqual($e.enclosure.url, "https://p.example/1.mp3");
    testing.assertEqual($e.enclosure.length, 12345678);
    testing.assertEqual($e.enclosure.type, "audio/mpeg");
    testing.assertTrue(hasEnclosure($e));
}

func testPodcastAtomRoundTrip() {
    def out as string init build("atom", podcast());
    testing.assertContains($out, "<name>Jane Host</name>");
    testing.assertContains($out, "term=\"Technology\"");
    testing.assertContains($out, "rel=\"enclosure\"");
    def f as Feed init parse($out);
    testing.assertEqual($f.author, "Jane Host");
    def e as Entry init $f.entries[0];
    testing.assertEqual($e.author, "Jane Host");
    testing.assertEqual(len($e.categories), 2);
    testing.assertEqual($e.enclosure.url, "https://p.example/1.mp3");
    testing.assertEqual($e.enclosure.length, 12345678);
    # the item's alternate link is the page URL, not the enclosure media URL
    testing.assertEqual($e.link, "https://p.example/1");
}

# RSS author falls back to Dublin Core <dc:creator> when <author> is absent.
func testRssAuthorDcCreatorFallback() {
    def x as string init "<rss version=\"2.0\" xmlns:dc=\"http://purl.org/dc/elements/1.1/\"><channel>" +
        "<title>C</title><item><title>i</title><dc:creator>Bob</dc:creator></item></channel></rss>";
    testing.assertEqual(parse($x).entries[0].author, "Bob");
}

# A missing enclosure length is 0 (unknown).
func testEnclosureBlankLength() {
    def x as string init "<rss version=\"2.0\"><channel><title>C</title><item><title>i</title><enclosure url=\"http://x/a.mp3\" type=\"audio/mpeg\"/></item></channel></rss>";
    def e as Entry init parse($x).entries[0];
    testing.assertEqual($e.enclosure.length, 0);
    testing.assertEqual($e.enclosure.url, "http://x/a.mp3");
}

# Unset author / categories / enclosure emit nothing and parse back empty.
func testOptionalFieldsOmitted() {
    def out as string init build("rss", add(feed("F", "L"), entry("e", "l")));
    testing.assertFalse(containsStr($out, "managingEditor"));
    testing.assertFalse(containsStr($out, "enclosure"));
    def e as Entry init parse($out).entries[0];
    testing.assertEqual($e.author, "");
    testing.assertEqual(len($e.categories), 0);
    testing.assertFalse(hasEnclosure($e));
}

func testNewBuildersValueSemantic() {
    def base as Entry init entry("t", "l");
    testing.assertEqual(len(entryCategory($base, "x").categories), 1);
    testing.assertEqual(len($base.categories), 0); # original untouched
    testing.assertEqual(entryAuthor($base, "A").author, "A");
    testing.assertEqual($base.author, "");
}

# An empty-string category is skipped on build (as parse drops it), so it does
# not emit a hollow <category></category> and the round-trip count is exact.
func testEmptyCategorySkippedOnBuild() {
    def f as Feed init add(feed("F", "L"), entryCategory(entry("e", "l"), ""));
    def out as string init build("rss", $f);
    testing.assertFalse(containsStr($out, "<category>"));
    testing.assertEqual(len(parse($out).entries[0].categories), 0);
}

# Feed-level and item-level metadata never leak across the nesting boundary.
func testFeedItemMetadataIsolation() {
    def x as string init "<rss version=\"2.0\"><channel><title>C</title>" +
        "<managingEditor>Boss</managingEditor>" +
        "<item><title>i</title><author>Writer</author><category>ItemCat</category></item>" +
        "</channel></rss>";
    def f as Feed init parse($x);
    testing.assertEqual($f.author, "Boss"); # channel author, not the item's
    testing.assertEqual(len($f.categories), 0); # item's category does not leak up
    testing.assertEqual($f.entries[0].author, "Writer");
    testing.assertEqual($f.entries[0].categories[0], "ItemCat");
}

# Feeds are read by namespace, not by the prefixes a document chose: Dublin Core
# under any prefix, and Atom as a default namespace or under a prefix. A <feed>
# outside the Atom namespace is not an Atom feed.
func testNamespacesByUri() {
    def rss as string init "<rss xmlns:creator=\"http://purl.org/dc/elements/1.1/\"><channel><title>T</title>" +
        "<item><title>I</title><creator:creator>ann</creator:creator></item></channel></rss>";
    testing.assertEqual(parse($rss).entries[0].author, "ann");
    def atom as string init "http://www.w3.org/2005/Atom";
    for (def doc in [
        "<feed xmlns=\"{$atom}\"><title>F</title><entry><title>E</title></entry></feed>",
        "<a:feed xmlns:a=\"{$atom}\"><a:title>F</a:title><a:entry><a:title>E</a:title></a:entry></a:feed>"
    ]) {
        def f as Feed init parse($doc);
        testing.assertEqual(kind($doc), "atom");
        testing.assertEqual($f.title, "F");
        testing.assertEqual($f.entries[0].title, "E");
    }
    testing.assertEqual(
        parseKind("<feed><title>F</title><entry><title>E</title></entry></feed>"),
        "value");
    testing.assertEqual(
        parseKind("<feed xmlns=\"http://example.org/x\"><title>F</title></feed>"),
        "value");
}
