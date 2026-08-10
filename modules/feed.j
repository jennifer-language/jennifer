# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0

/**
 * Web syndication feeds: build and parse both RSS 2.0 and Atom 1.0 through one
 * module. A feed is a value-semantic `Feed` (title, link, updated, entries) of
 * `Entry` (title, link, id, published / updated, summary, content). The format
 * is chosen when you `build` (`"rss"` or `"atom"`) and detected from the root
 * element when you `parse`, so the same `Feed` shape serves both - a feed
 * reader, podcast client, news aggregator, or changelog-to-feed generator.
 *
 * Parsing rides the `xml` library (entities, CDATA, Atom's namespaces);
 * building emits escaped XML through the same. Dates go through `time` (RFC 822
 * `pubDate` for RSS, RFC 3339 `updated` for Atom); an unset date is the Unix
 * epoch and is omitted on build. `fetch` pulls a feed over the `http` module.
 * @module feed
 * @example
 * import "feed.j" as feed;
 * def f as feed.Feed init feed.add(feed.feed("News", "https://example.org"),
 *     feed.entry("Hello", "https://example.org/1"));
 * def xml as string init feed.build($f, "atom");
 * def back as feed.Feed init feed.parse($xml);
 */
use xml;
use time;
use convert;
import "./http.j" as http;

/**
 * A media enclosure attached to an entry: the podcast / audio / video file. Maps
 * to RSS `<enclosure url length type>` and Atom `<link rel="enclosure">`. An
 * entry with an empty `url` has no enclosure (omitted on build).
 * @field url {string} the media URL
 * @field length {int} the media size in bytes (0 when unknown)
 * @field type {string} the MIME type (e.g. "audio/mpeg")
 */
export def struct Enclosure {
    url as string,
    length as int,
    type as string
};

/**
 * One feed item. `link` is the item URL, `id` its stable identifier (RSS
 * `guid` / Atom `id`), `summary` a short description (RSS `description` / Atom
 * `summary`) and `content` the full body (Atom `content`; RSS carries only the
 * one description, mapped to `summary`). `author` is the item author, `categories`
 * its tags, and `enclosure` an attached media file (podcast episode). An unset
 * date is the Unix epoch.
 * @field title {string} the entry title
 * @field link {string} the entry URL
 * @field id {string} the stable identifier (RSS guid / Atom id)
 * @field published {time.Time} the first-published instant (epoch if unset)
 * @field updated {time.Time} the last-updated instant (epoch if unset)
 * @field summary {string} a short summary / description
 * @field content {string} the full content (Atom only)
 * @field author {string} the item author (RSS author / dc:creator, Atom author/name)
 * @field categories {list of string} the item's category tags
 * @field enclosure {Enclosure} an attached media file (empty url = none)
 */
export def struct Entry {
    title as string,
    link as string,
    id as string,
    published as time.Time,
    updated as time.Time,
    summary as string,
    content as string,
    author as string,
    categories as list of string,
    enclosure as Enclosure
};

/**
 * A syndication feed: channel-level metadata plus its entries.
 * @field title {string} the feed title
 * @field link {string} the feed's site URL
 * @field updated {time.Time} the feed's last-build / updated instant (epoch if unset)
 * @field entries {list of Entry} the feed items, in document order
 * @field author {string} the feed author (RSS managingEditor, Atom author/name)
 * @field categories {list of string} the feed's category tags
 */
export def struct Feed {
    title as string,
    link as string,
    updated as time.Time,
    entries as list of Entry,
    author as string,
    categories as list of string
};

# epoch is the "unset date" sentinel: a date equal to it is omitted on build.
func epoch() {
    return time.fromUnix(0);
}
# isSet reports whether a date is a real instant (not the epoch sentinel).
func isSet(t as time.Time) {
    return time.unix($t) != 0;
}
# noEnclosure is the "no attached media" sentinel (empty url).
func noEnclosure() {
    return Enclosure{url: "", length: 0, type: ""};
}
/**
 * Report whether an entry carries a media enclosure (a non-empty enclosure url).
 * @param e {Entry} the entry to test
 * @return {bool} true when the entry has an attached media file
 */
export func hasEnclosure(e as Entry) {
    return len($e.enclosure.url) > 0;
}

# ---- value-semantic builders ----

/**
 * A new empty `Feed` with the given title and site link (no entries, unset date).
 * @param title {string} the feed title
 * @param link {string} the feed's site URL
 * @return {Feed} the new feed
 */
export func feed(title as string, link as string) {
    return Feed{title: $title, link: $link, updated: epoch(), entries: [], author: "", categories: []};
}
/**
 * A copy of `f` with its `updated` instant set.
 * @param f {Feed} the source feed
 * @param t {time.Time} the last-updated instant
 * @return {Feed} the updated feed
 */
export func feedUpdated(f as Feed, t as time.Time) {
    def out as Feed init $f;
    $out.updated = $t;
    return $out;
}
/**
 * A copy of `f` with its author set.
 * @param f {Feed} the source feed
 * @param author {string} the feed author (a name or email)
 * @return {Feed} the updated feed
 */
export func feedAuthor(f as Feed, author as string) {
    def out as Feed init $f;
    $out.author = $author;
    return $out;
}
/**
 * A copy of `f` with `category` appended to its category tags.
 * @param f {Feed} the source feed
 * @param category {string} the category / tag to add
 * @return {Feed} the feed with the category added
 */
export func feedCategory(f as Feed, category as string) {
    def out as Feed init $f;
    def cs as list of string init $out.categories;
    $cs[] = $category;
    $out.categories = $cs;
    return $out;
}
/**
 * A copy of `f` with `e` appended to its entries.
 * @param f {Feed} the source feed
 * @param e {Entry} the entry to append
 * @return {Feed} the feed with the entry added
 */
export func add(f as Feed, e as Entry) {
    def out as Feed init $f;
    def es as list of Entry init $out.entries;
    $es[] = $e;
    $out.entries = $es;
    return $out;
}

/**
 * A new `Entry` with the given title and link (empty id / summary / content,
 * unset dates).
 * @param title {string} the entry title
 * @param link {string} the entry URL
 * @return {Entry} the new entry
 */
export func entry(title as string, link as string) {
    return Entry{
        title: $title,
        link: $link,
        id: "",
        published: epoch(),
        updated: epoch(),
        summary: "",
        content: "",
        author: "",
        categories: [],
        enclosure: noEnclosure()
    };
}
/**
 * A copy of `e` with its stable id set.
 * @param e {Entry} the source entry
 * @param id {string} the identifier (RSS guid / Atom id)
 * @return {Entry} the updated entry
 */
export func entryId(e as Entry, id as string) {
    def out as Entry init $e;
    $out.id = $id;
    return $out;
}
/**
 * A copy of `e` with its published instant set.
 * @param e {Entry} the source entry
 * @param t {time.Time} the first-published instant
 * @return {Entry} the updated entry
 */
export func entryPublished(e as Entry, t as time.Time) {
    def out as Entry init $e;
    $out.published = $t;
    return $out;
}
/**
 * A copy of `e` with its updated instant set.
 * @param e {Entry} the source entry
 * @param t {time.Time} the last-updated instant
 * @return {Entry} the updated entry
 */
export func entryUpdated(e as Entry, t as time.Time) {
    def out as Entry init $e;
    $out.updated = $t;
    return $out;
}
/**
 * A copy of `e` with its summary set.
 * @param e {Entry} the source entry
 * @param s {string} the summary / description text
 * @return {Entry} the updated entry
 */
export func entrySummary(e as Entry, s as string) {
    def out as Entry init $e;
    $out.summary = $s;
    return $out;
}
/**
 * A copy of `e` with its full content set (Atom).
 * @param e {Entry} the source entry
 * @param c {string} the content text
 * @return {Entry} the updated entry
 */
export func entryContent(e as Entry, c as string) {
    def out as Entry init $e;
    $out.content = $c;
    return $out;
}
/**
 * A copy of `e` with its author set.
 * @param e {Entry} the source entry
 * @param author {string} the item author (a name or email)
 * @return {Entry} the updated entry
 */
export func entryAuthor(e as Entry, author as string) {
    def out as Entry init $e;
    $out.author = $author;
    return $out;
}
/**
 * A copy of `e` with `category` appended to its category tags.
 * @param e {Entry} the source entry
 * @param category {string} the category / tag to add
 * @return {Entry} the entry with the category added
 */
export func entryCategory(e as Entry, category as string) {
    def out as Entry init $e;
    def cs as list of string init $out.categories;
    $cs[] = $category;
    $out.categories = $cs;
    return $out;
}
/**
 * A copy of `e` with a media enclosure set (the podcast / audio / video file).
 * @param e {Entry} the source entry
 * @param url {string} the media URL
 * @param length {int} the media size in bytes (0 when unknown)
 * @param type {string} the MIME type (e.g. "audio/mpeg")
 * @return {Entry} the entry with the enclosure set
 */
export func entryEnclosure(e as Entry, url as string, length as int, type as string) {
    def out as Entry init $e;
    $out.enclosure = Enclosure{url: $url, length: $length, type: $type};
    return $out;
}

# ---- dates ----

# RSS uses RFC 822 (RFC 1123) dates; Atom uses RFC 3339 (time.iso).
func rssDate(t as time.Time) {
    return time.format($t, "%a, %d %b %Y %H:%M:%S %z");
}
# parseRssDate is lenient: it accepts the day-of-week-prefixed form and the
# bare form, returning the epoch sentinel when neither parses (real feeds carry
# malformed dates, and a feed must not fail to load over one bad timestamp).
func parseRssDate(s as string) {
    try {
        return time.parse($s, "%a, %d %b %Y %H:%M:%S %z");
    } catch (first) {
        try {
            return time.parse($s, "%d %b %Y %H:%M:%S %z");
        } catch (second) {
            return epoch();
        }
    }
}
func parseAtomDate(s as string) {
    try {
        return time.fromIso($s);
    } catch (e) {
        return epoch();
    }
}

# ---- xml build / read helpers ----

# el builds an element with a single text child.
func el(tag as string, content as string) {
    return xml.setText(xml.element($tag), $content);
}
# textOf returns the text of the first child element at `path`, or "" if absent.
func textOf(node as xml.Value, path as string) {
    if (xml.has($node, $path)) {
        return xml.text(xml.get($node, $path));
    }
    return "";
}
# atomLink returns the href of the node's first alternate <link> element (one
# without a rel, or rel="alternate"), or "". A rel="enclosure" link is a media
# attachment, not the item URL, so it is skipped here.
func atomLink(node as xml.Value) {
    def links as list of xml.Value init xml.findAll($node, "link");
    for (def i as int init 0; $i < len($links); $i = $i + 1) {
        def l as xml.Value init $links[$i];
        def rel as string init attrOr($l, "rel");
        if (($rel == "" or $rel == "alternate") and xml.hasAttr($l, "href")) {
            return xml.attr($l, "href");
        }
    }
    return "";
}
# attrOr returns an element's named attribute, or "" when absent.
func attrOr(node as xml.Value, name as string) {
    if (xml.hasAttr($node, $name)) {
        return xml.attr($node, $name);
    }
    return "";
}
# atomAuthor builds an Atom <author><name>...</name></author> element.
func atomAuthor(name as string) {
    return xml.append(xml.element("author"), el("name", $name));
}
# atomAuthorOf reads an Atom author name (<author><name>), tolerating a feed that
# puts the name directly in <author>; "" when absent.
func atomAuthorOf(node as xml.Value) {
    if (xml.has($node, "author/name")) {
        return xml.text(xml.get($node, "author/name"));
    }
    if (xml.has($node, "author")) {
        return xml.text(xml.get($node, "author"));
    }
    return "";
}
# rssAuthorOf reads an RSS author, preferring the native <author> and falling
# back to Dublin Core <dc:creator> (the xml library keeps prefixes verbatim);
# a channel-level <managingEditor> is a third fallback. "" when none present.
func rssAuthorOf(node as xml.Value) {
    if (xml.has($node, "author")) {
        return xml.text(xml.get($node, "author"));
    }
    if (xml.has($node, "dc:creator")) {
        return xml.text(xml.get($node, "dc:creator"));
    }
    if (xml.has($node, "managingEditor")) {
        return xml.text(xml.get($node, "managingEditor"));
    }
    return "";
}
# categoriesText reads RSS categories (<category>text</category>) as a list.
func categoriesText(node as xml.Value) {
    def out as list of string init [];
    def cats as list of xml.Value init xml.findAll($node, "category");
    for (def i as int init 0; $i < len($cats); $i = $i + 1) {
        def t as string init xml.text($cats[$i]);
        if (len($t) > 0) {
            $out[] = $t;
        }
    }
    return $out;
}
# categoriesTerm reads Atom categories (<category term="..."/>) as a list.
func categoriesTerm(node as xml.Value) {
    def out as list of string init [];
    def cats as list of xml.Value init xml.findAll($node, "category");
    for (def i as int init 0; $i < len($cats); $i = $i + 1) {
        def t as string init attrOr($cats[$i], "term");
        if (len($t) > 0) {
            $out[] = $t;
        }
    }
    return $out;
}
# parseLength converts an enclosure length attribute to an int, or 0 when it is
# empty or non-numeric (real feeds carry a blank or bad length).
func parseLength(s as string) {
    if (len($s) == 0) {
        return 0;
    }
    try {
        return convert.toInt($s);
    } catch (e) {
        return 0;
    }
}
# readEnclosureRss reads an RSS <enclosure url length type/> from an item.
func readEnclosureRss(item as xml.Value) {
    if (not xml.has($item, "enclosure")) {
        return noEnclosure();
    }
    def enc as xml.Value init xml.get($item, "enclosure");
    return Enclosure{
        url: attrOr($enc, "url"),
        length: parseLength(attrOr($enc, "length")),
        type: attrOr($enc, "type")
    };
}
# readEnclosureAtom reads an Atom <link rel="enclosure" href length type/>.
func readEnclosureAtom(item as xml.Value) {
    def links as list of xml.Value init xml.findAll($item, "link");
    for (def i as int init 0; $i < len($links); $i = $i + 1) {
        def l as xml.Value init $links[$i];
        if (attrOr($l, "rel") == "enclosure") {
            return Enclosure{
                url: attrOr($l, "href"),
                length: parseLength(attrOr($l, "length")),
                type: attrOr($l, "type")
            };
        }
    }
    return noEnclosure();
}

# ---- build ----

/**
 * Render `f` to feed XML in the named format: `"rss"` (RSS 2.0) or `"atom"`
 * (Atom 1.0). Values are XML-escaped; unset dates are omitted.
 * @param f {Feed} the feed to render
 * @param format {string} `"rss"` or `"atom"`
 * @return {string} the feed document as XML text
 * @throws {Error} when `format` is not `"rss"` or `"atom"`
 */
export func build(f as Feed, format as string) {
    if ($format == "rss") {
        return buildRss($f);
    }
    if ($format == "atom") {
        return buildAtom($f);
    }
    throw Error{
        kind: "value",
        message: "feed.build: unknown format: " + $format + " (want \"rss\" or \"atom\")",
        file: "",
        line: 0,
        col: 0
    };
}

func buildRss(f as Feed) {
    def channel as xml.Value init xml.element("channel");
    $channel = xml.append($channel, el("title", $f.title));
    $channel = xml.append($channel, el("link", $f.link));
    if (len($f.author) > 0) {
        $channel = xml.append($channel, el("managingEditor", $f.author));
    }
    for (def c as int init 0; $c < len($f.categories); $c = $c + 1) {
        if (len($f.categories[$c]) > 0) {
            $channel = xml.append($channel, el("category", $f.categories[$c]));
        }
    }
    if (isSet($f.updated)) {
        $channel = xml.append($channel, el("lastBuildDate", rssDate($f.updated)));
    }
    for (def i as int init 0; $i < len($f.entries); $i = $i + 1) {
        def e as Entry init $f.entries[$i];
        def item as xml.Value init xml.element("item");
        $item = xml.append($item, el("title", $e.title));
        if (len($e.link) > 0) {
            $item = xml.append($item, el("link", $e.link));
        }
        if (len($e.id) > 0) {
            $item = xml.append($item, el("guid", $e.id));
        }
        if (len($e.author) > 0) {
            $item = xml.append($item, el("author", $e.author));
        }
        for (def c as int init 0; $c < len($e.categories); $c = $c + 1) {
            if (len($e.categories[$c]) > 0) {
                $item = xml.append($item, el("category", $e.categories[$c]));
            }
        }
        if (isSet($e.published)) {
            $item = xml.append($item, el("pubDate", rssDate($e.published)));
        }
        def body as string init $e.summary;
        if (len($body) == 0) {
            $body = $e.content;
        }
        if (len($body) > 0) {
            $item = xml.append($item, el("description", $body));
        }
        if (hasEnclosure($e)) {
            def enc as xml.Value init xml.setAttr(xml.element("enclosure"), "url", $e.enclosure.url);
            $enc = xml.setAttr($enc, "length", convert.toString($e.enclosure.length));
            $enc = xml.setAttr($enc, "type", $e.enclosure.type);
            $item = xml.append($item, $enc);
        }
        $channel = xml.append($channel, $item);
    }
    def rss as xml.Value init xml.setAttr(xml.element("rss"), "version", "2.0");
    $rss = xml.append($rss, $channel);
    return xml.encode($rss);
}

func buildAtom(f as Feed) {
    def root as xml.Value init xml.setAttr(
        xml.element("feed"),
        "xmlns",
        "http://www.w3.org/2005/Atom");
    $root = xml.append($root, el("title", $f.title));
    $root = xml.append($root, xml.setAttr(xml.element("link"), "href", $f.link));
    if (len($f.author) > 0) {
        $root = xml.append($root, atomAuthor($f.author));
    }
    for (def c as int init 0; $c < len($f.categories); $c = $c + 1) {
        if (len($f.categories[$c]) > 0) {
            $root = xml.append($root, xml.setAttr(xml.element("category"), "term", $f.categories[$c]));
        }
    }
    if (isSet($f.updated)) {
        $root = xml.append($root, el("updated", time.iso($f.updated)));
    }
    for (def i as int init 0; $i < len($f.entries); $i = $i + 1) {
        def e as Entry init $f.entries[$i];
        def item as xml.Value init xml.element("entry");
        $item = xml.append($item, el("title", $e.title));
        if (len($e.link) > 0) {
            $item = xml.append($item, xml.setAttr(xml.element("link"), "href", $e.link));
        }
        if (len($e.id) > 0) {
            $item = xml.append($item, el("id", $e.id));
        }
        if (len($e.author) > 0) {
            $item = xml.append($item, atomAuthor($e.author));
        }
        for (def c as int init 0; $c < len($e.categories); $c = $c + 1) {
            if (len($e.categories[$c]) > 0) {
                $item = xml.append($item, xml.setAttr(xml.element("category"), "term", $e.categories[$c]));
            }
        }
        if (isSet($e.published)) {
            $item = xml.append($item, el("published", time.iso($e.published)));
        }
        if (isSet($e.updated)) {
            $item = xml.append($item, el("updated", time.iso($e.updated)));
        }
        if (len($e.summary) > 0) {
            $item = xml.append($item, el("summary", $e.summary));
        }
        if (len($e.content) > 0) {
            $item = xml.append($item, el("content", $e.content));
        }
        if (hasEnclosure($e)) {
            def enc as xml.Value init xml.setAttr(xml.element("link"), "rel", "enclosure");
            $enc = xml.setAttr($enc, "href", $e.enclosure.url);
            $enc = xml.setAttr($enc, "length", convert.toString($e.enclosure.length));
            $enc = xml.setAttr($enc, "type", $e.enclosure.type);
            $item = xml.append($item, $enc);
        }
        $root = xml.append($root, $item);
    }
    return xml.encode($root);
}

# ---- parse ----

/**
 * The feed format of `text`, by its root element: `"rss"` or `"atom"`.
 * @param text {string} the feed XML
 * @return {string} `"rss"` or `"atom"`
 * @throws {Error} when the XML is malformed or the root is neither
 */
export func kind(text as string) {
    def root as xml.Value init xml.decode($text);
    return kindOf($root);
}
func kindOf(root as xml.Value) {
    def tag as string init xml.tag($root);
    if ($tag == "rss") {
        return "rss";
    }
    if ($tag == "feed") {
        return "atom";
    }
    throw Error{
        kind: "value",
        message: "feed.parse: not a feed (root element <" + $tag + ">, want <rss> or <feed>)",
        file: "",
        line: 0,
        col: 0
    };
}

/**
 * Parse feed XML into a `Feed`, detecting RSS vs Atom from the root element.
 * Malformed dates degrade to the epoch sentinel rather than failing the parse.
 * @param text {string} the feed XML (RSS 2.0 or Atom 1.0)
 * @return {Feed} the parsed feed
 * @throws {Error} when the XML is malformed or the root is neither feed format
 */
export func parse(text as string) {
    def root as xml.Value init xml.decode($text);
    if (kindOf($root) == "rss") {
        return parseRss($root);
    }
    return parseAtom($root);
}

func parseRss(root as xml.Value) {
    # A well-formed RSS document wraps everything in <channel>; tolerate a
    # malformed feed that omits it by reading directly from the root.
    def channel as xml.Value init $root;
    if (xml.has($root, "channel")) {
        $channel = xml.get($root, "channel");
    }
    def updated as time.Time init epoch();
    if (xml.has($channel, "lastBuildDate")) {
        $updated = parseRssDate(textOf($channel, "lastBuildDate"));
    } elseif (xml.has($channel, "pubDate")) {
        $updated = parseRssDate(textOf($channel, "pubDate"));
    }
    def es as list of Entry init [];
    def items as list of xml.Value init xml.findAll($channel, "item");
    for (def i as int init 0; $i < len($items); $i = $i + 1) {
        def it as xml.Value init $items[$i];
        def e as Entry init Entry{
            title: textOf($it, "title"),
            link: textOf($it, "link"),
            id: textOf($it, "guid"),
            published: parseRssDate(textOf($it, "pubDate")),
            updated: epoch(),
            summary: textOf($it, "description"),
            content: "",
            author: rssAuthorOf($it),
            categories: categoriesText($it),
            enclosure: readEnclosureRss($it)
        };
        $es[] = $e;
    }
    return Feed{
        title: textOf($channel, "title"),
        link: textOf($channel, "link"),
        updated: $updated,
        entries: $es,
        author: rssAuthorOf($channel),
        categories: categoriesText($channel)
    };
}

func parseAtom(root as xml.Value) {
    def es as list of Entry init [];
    def items as list of xml.Value init xml.findAll($root, "entry");
    for (def i as int init 0; $i < len($items); $i = $i + 1) {
        def it as xml.Value init $items[$i];
        def e as Entry init Entry{
            title: textOf($it, "title"),
            link: atomLink($it),
            id: textOf($it, "id"),
            published: parseAtomDate(textOf($it, "published")),
            updated: parseAtomDate(textOf($it, "updated")),
            summary: textOf($it, "summary"),
            content: textOf($it, "content"),
            author: atomAuthorOf($it),
            categories: categoriesTerm($it),
            enclosure: readEnclosureAtom($it)
        };
        $es[] = $e;
    }
    return Feed{
        title: textOf($root, "title"),
        link: atomLink($root),
        updated: parseAtomDate(textOf($root, "updated")),
        entries: $es,
        author: atomAuthorOf($root),
        categories: categoriesTerm($root)
    };
}

# ---- fetch ----

/**
 * Fetch a feed over HTTP and parse it: `http.get` the URL, then `parse` the
 * body. Needs the `http` module (the default `jennifer`; a friendly network
 * error on `jennifer-tiny`).
 * @param url {string} the feed URL
 * @return {Feed} the fetched, parsed feed
 * @throws {Error} on a transport error, a non-2xx status, or malformed feed XML
 */
export func fetch(url as string) {
    def headers as map of string to string init {};
    def resp as http.Response init http.get($url, $headers);
    if ($resp.status < 200 or $resp.status >= 300) {
        throw Error{
            kind: "http",
            message: "feed.fetch: " + $url + " returned status " + convert.toString($resp.status),
            file: "",
            line: 0,
            col: 0
        };
    }
    return parse($resp.body);
}
