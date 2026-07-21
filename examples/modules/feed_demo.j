# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

# feed_demo.j - build a syndication feed once and emit it as both RSS 2.0 and
# Atom 1.0, then parse each back to show the format is auto-detected and the
# round-trip preserves the data. Pure (no network); `feed.fetch(url)` pulls a
# live feed over the http module.
#
#     jennifer run examples/modules/feed_demo.j

use io;
use time;
import "../../modules/feed.j" as feed;

# Build a feed with two entries. Dates go through `time`.
def built as time.Time init time.fromIso("2026-07-20T12:00:00Z");
def f as feed.Feed init feed.feedUpdated(feed.feed("Jennifer Release Notes",
    "https://jennifer-language.dev"), $built);

def one as feed.Entry init feed.entrySummary(
    feed.entryPublished(
        feed.entryId(feed.entry("0.19 ships the screen module",
            "https://jennifer-language.dev/notes/0-19"), "note-0-19"),
        time.fromIso("2026-07-20T10:00:00Z")),
    "Terminal UIs land: a cell buffer, a diff renderer & key decoding.");
def two as feed.Entry init feed.entrySummary(
    feed.entryPublished(
        feed.entryId(feed.entry("0.18 adds the feed module",
            "https://jennifer-language.dev/notes/0-18"), "note-0-18"),
        time.fromIso("2026-07-13T10:00:00Z")),
    "RSS 2.0 and Atom 1.0, one module, format detected on parse.");

$f = feed.add(feed.add($f, $one), $two);

# Emit both formats from the same feed.
def rss as string init feed.build($f, "rss");
def atom as string init feed.build($f, "atom");
io.printf("built RSS  (%d bytes, kind=%s)\n", len($rss), feed.kind($rss));
io.printf("built Atom (%d bytes, kind=%s)\n", len($atom), feed.kind($atom));

io.printf("\n--- Atom document ---\n%s\n", $atom);

# Parse each back; the format is detected from the root element.
def fromRss as feed.Feed init feed.parse($rss);
def fromAtom as feed.Feed init feed.parse($atom);
io.printf("\nparsed RSS : \"%s\", %d entries, updated %s\n",
    $fromRss.title, len($fromRss.entries), time.iso($fromRss.updated));
io.printf("parsed Atom: \"%s\", %d entries, updated %s\n",
    $fromAtom.title, len($fromAtom.entries), time.iso($fromAtom.updated));

io.printf("\nlatest entry: \"%s\" (%s)\n  %s\n",
    $fromAtom.entries[0].title,
    time.format($fromAtom.entries[0].published, "%Y-%m-%d"),
    $fromAtom.entries[0].summary);
