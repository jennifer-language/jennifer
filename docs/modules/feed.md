# `feed` - RSS 2.0 and Atom 1.0 syndication

Import with `import "feed.j" as feed;`. Build and parse web syndication feeds -
**both RSS 2.0 and Atom 1.0 through one module** (design stance 1: not separate
`rss` / `atom` modules). The format is chosen when you `build` and detected from
the root element when you `parse`, so the same value-semantic `Feed` shape
serves a feed reader, podcast client, news aggregator, or changelog-to-feed
generator.

Parsing rides the [`xml`](../libraries/xml.md) library (entities, CDATA, Atom's
namespaces); building emits escaped XML through it. Dates go through
[`time`](../libraries/time.md). `fetch` pulls a feed over the [`http`](http.md)
module.

```jennifer
import "feed.j" as feed;

def f as feed.Feed init feed.add(
    feed.feed("My Blog", "https://example.org"),
    feed.entry("Hello", "https://example.org/1"));
def rss as string init feed.build($f, "rss");     # or "atom"
def back as feed.Feed init feed.parse($rss);       # format auto-detected
```

Runnable: [`examples/modules/feed_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/feed_demo.j).

## The `Feed`, `Entry` and `Enclosure` structs

All three are value-semantic, so the builders return a fresh copy.

`feed.Feed { title as string, link as string, updated as time.Time, entries as list of Entry, author as string, categories as list of string }`

`feed.Entry { title as string, link as string, id as string, published as time.Time, updated as time.Time, summary as string, content as string, author as string, categories as list of string, enclosure as Enclosure }`

`feed.Enclosure { url as string, length as int, type as string }`

A field that maps to different names per format: `id` is RSS `guid` / Atom `id`;
`summary` is RSS `description` / Atom `summary`; `content` is Atom-only (RSS
carries the one description, kept in `summary`). `author` is RSS `<author>` /
`<dc:creator>` (channel `<managingEditor>`) and Atom `<author><name>`;
`categories` are RSS `<category>` / Atom `<category term>`; `enclosure` is the
attached media file (RSS `<enclosure>` / Atom `<link rel="enclosure">`), the
podcast episode's audio. An **unset date is the Unix epoch** and is omitted when
building; an **empty enclosure `url`** means no enclosure and is likewise
omitted.

## Building

| Call                         | Returns   | Notes                                                     |
| ---------------------------- | --------- | -------------------------------------------------------- |
| `feed.feed(title, link)`     | `Feed`    | A new empty feed (no entries, unset date).               |
| `feed.feedUpdated(f, t)`     | `Feed`    | A copy with the feed's `updated` instant set.            |
| `feed.add(f, e)`             | `Feed`    | A copy with `e` appended to the entries.                 |
| `feed.entry(title, link)`    | `Entry`   | A new entry (empty id / summary / content, unset dates). |
| `feed.entryId(e, id)`        | `Entry`   | A copy with the stable id set.                           |
| `feed.entryPublished(e, t)`  | `Entry`   | A copy with the published instant set.                   |
| `feed.entryUpdated(e, t)`    | `Entry`   | A copy with the updated instant set.                     |
| `feed.entrySummary(e, s)`    | `Entry`   | A copy with the summary set.                             |
| `feed.entryContent(e, c)`    | `Entry`   | A copy with the full content set (Atom).                 |
| `feed.entryAuthor(e, author)`| `Entry`   | A copy with the item author set.                         |
| `feed.entryCategory(e, category)` | `Entry` | A copy with `category` appended to the item's tags.   |
| `feed.entryEnclosure(e, url, length, type)` | `Entry` | A copy with a media enclosure (podcast file) set. |
| `feed.feedAuthor(f, author)` | `Feed`    | A copy with the feed author set.                         |
| `feed.feedCategory(f, category)` | `Feed` | A copy with `category` appended to the feed's tags.      |
| `feed.hasEnclosure(e)`       | `bool`    | Whether the entry has a media enclosure (non-empty url). |
| `feed.build(f, format)`      | `string`  | Render to `"rss"` (RSS 2.0) or `"atom"` (Atom 1.0). Unknown format errors. |

RSS uses RFC 822 dates (`pubDate` / `lastBuildDate`); Atom uses RFC 3339
(`published` / `updated`). All text is XML-escaped, so `&`, `<`, and `>` in
titles or summaries round-trip.

### Podcasts and metadata

`entryEnclosure` attaches the media file that makes a feed a **podcast**: RSS
emits `<enclosure url length type/>`, Atom `<link rel="enclosure" href length
type/>`, and parse reads either back into the `Enclosure` struct. The item's
own page link (Atom's alternate `<link>`) stays separate from the enclosure, so
`entry.link` is the episode page and `entry.enclosure.url` the audio. `author`
and `categories` round-trip on both the feed and each entry; on parse an RSS
author falls back to `<dc:creator>` when the native `<author>` is absent, and a
blank / non-numeric enclosure `length` degrades to `0` rather than failing.

```jennifer
def ep as feed.Entry init feed.entryEnclosure(
    feed.entryAuthor(feed.entry("Episode 1", "https://show.example/1"), "Jane Host"),
    "https://show.example/1.mp3", 12345678, "audio/mpeg");
def show as feed.Feed init feed.feedCategory(
    feed.feedAuthor(feed.feed("My Show", "https://show.example"), "Jane Host"),
    "Technology");
def rss as string init feed.build(feed.add($show, $ep), "rss");
```

## Parsing

| Call               | Returns  | Notes                                                              |
| ------------------ | -------- | ----------------------------------------------------------------- |
| `feed.parse(text)` | `Feed`   | Parse feed XML, detecting RSS vs Atom from the root element.       |
| `feed.kind(text)`  | `string` | `"rss"` or `"atom"` - the detected format (errors if neither).     |
| `feed.fetch(url)`  | `Feed`   | Fetch over HTTP (`http.get`) and parse. Non-2xx / transport errors are catchable. Needs the `http` module (default binary). |

Parsing is **lenient about dates**: a malformed timestamp degrades to the epoch
sentinel rather than failing the whole feed, and a malformed RSS document that
omits `<channel>` is still read. A document whose root is neither `<rss>` nor
`<feed>` is a catchable error.

## Safety for untrusted feeds

Feeds come from URLs you do not control, so the untrusted-input paths are
hardened:

- **Deeply-nested XML** is capped by the `xml` library's shared nesting limit
  (a catchable error, not a stack overflow) - see
  [xml](../libraries/xml.md).
- **Entity-expansion ("billion laughs")** is impossible: `xml` decodes only the
  five predefined entities and numeric character references, never a DTD's
  custom entities.
- **Oversized bodies** are bounded: `feed.fetch` reads through `http`, which
  caps a response body (64 MiB) so a hostile server cannot drive the interpreter
  to OOM - the fetch fails with a catchable error instead.
- Parsing is **linear** in the document size; a large feed is slow, not
  quadratic.

For a fully-hostile source, prefer `feed.parse` on a body you have already
size-bounded, and wrap the call in `try` / `catch`.

## Platforms

Building and parsing are pure Jennifer over `xml` + `time` and run on **both
binaries**. `feed.fetch` needs `http` (hence `net`), so it works on the default
`jennifer` and returns a friendly network error on `jennifer-tiny`.
