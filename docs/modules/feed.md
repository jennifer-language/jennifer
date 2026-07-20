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

## The `Feed` and `Entry` structs

Both are value-semantic, so the builders return a fresh copy.

`feed.Feed { title as string, link as string, updated as time.Time, entries as list of Entry }`

`feed.Entry { title as string, link as string, id as string, published as time.Time, updated as time.Time, summary as string, content as string }`

A field that maps to different names per format: `id` is RSS `guid` / Atom `id`;
`summary` is RSS `description` / Atom `summary`; `content` is Atom-only (RSS
carries the one description, kept in `summary`). An **unset date is the Unix
epoch** and is omitted when building.

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
| `feed.build(f, format)`      | `string`  | Render to `"rss"` (RSS 2.0) or `"atom"` (Atom 1.0). Unknown format errors. |

RSS uses RFC 822 dates (`pubDate` / `lastBuildDate`); Atom uses RFC 3339
(`published` / `updated`). All text is XML-escaped, so `&`, `<`, and `>` in
titles or summaries round-trip.

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
