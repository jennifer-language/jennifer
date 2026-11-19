# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.26.0

/**
 * Web syndication feeds: build and parse both RSS 2.0 and Atom 1.0 through one
 * module. A feed is a value-semantic `Feed` (title, link, updated, entries) of
 * `Entry` (title, link, id, published / updated, summary, content). The format
 * is chosen when you `build` (`"rss"` or `"atom"`) and detected from the root
 * element when you `parse`, so the same `Feed` shape serves both - a feed
 * reader, podcast client, news aggregator, or changelog-to-feed generator.
 *
 * A call into the module rebuilds the Feed it is passed (module-boundary
 * retag), so append many entries with one `addAll`, not `add` in a loop.
 *
 * Parsing rides the `xml` library (entities, CDATA, Atom's namespaces);
 * building emits escaped XML through the same. Dates go through `time` (RFC 822
 * `pubDate` for RSS, RFC 3339 `updated` for Atom); an unset date is the Unix
 * epoch and is omitted on build. To read a feed off the web, `http.get` it and
 * `parse` the body; the module itself needs no network, so it loads on both
 * binaries.
 *
 * `parse` is strict: a document that is not an RSS 2.0 `<rss>` with a
 * `<channel>` or an Atom `<feed>` in the Atom namespace, a date that does not
 * parse, an enclosure `length` that is not a non-negative integer, or an Atom
 * `type="xhtml"` construct without its single XHTML `<div>` throws
 * `Error{kind: "value"}` (the `xml` library's own error for malformed XML).
 * @module feed
 * @example
 * import "feed.j" as feed;
 *
 * def f as feed.Feed init feed.add(feed.feed("News", "https://example.org"),
 *     feed.entry("Hello", "https://example.org/1"));
 * def xml as string init feed.build("atom", $f);
 * def back as feed.Feed init feed.parse($xml);
 */
use xml;
use time;
use convert;
use regex;
use strings;
use maps;

# DC_NAMESPACE is the Dublin Core elements namespace (RSS <dc:creator>).
def const DC_NAMESPACE as string init "http://purl.org/dc/elements/1.1/";
# ATOM_NAMESPACE is the namespace every Atom 1.0 element is in (RFC 4287 2).
def const ATOM_NAMESPACE as string init "http://www.w3.org/2005/Atom";
# XHTML_NAMESPACE is the namespace of an Atom xhtml construct's <div>.
def const XHTML_NAMESPACE as string init "http://www.w3.org/1999/xhtml";

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
    return Feed{
        title: $title,
        link: $link,
        updated: epoch(),
        entries: [],
        author: "",
        categories: []
    };
}
/**
 * A copy of `f` with its `updated` instant set.
 * @param f {Feed} the source feed
 * @param t {time.Time} the last-updated instant
 * @return {Feed} the updated feed
 */
export func feedUpdated(f as Feed, t as time.Time) {
    $f.updated = $t;
    return $f;
}
/**
 * A copy of `f` with its author set.
 * @param f {Feed} the source feed
 * @param author {string} the feed author (a name or email)
 * @return {Feed} the updated feed
 */
export func feedAuthor(f as Feed, author as string) {
    $f.author = $author;
    return $f;
}
/**
 * A copy of `f` with `category` appended to its category tags.
 * @param f {Feed} the source feed
 * @param category {string} the category / tag to add
 * @return {Feed} the feed with the category added
 */
export func feedCategory(f as Feed, category as string) {
    $f.categories[] = $category;
    return $f;
}
/**
 * A copy of `f` with `e` appended to its entries.
 * @param f {Feed} the source feed
 * @param e {Entry} the entry to append
 * @return {Feed} the feed with the entry added
 */
export func add(f as Feed, e as Entry) {
    $f.entries[] = $e;
    return $f;
}

/**
 * A copy of `f` with every entry in `entries` appended, in order. Prefer it to
 * `add` in a loop: each module call rebuilds the whole Feed.
 * @param f {Feed} the source feed
 * @param entries {list of Entry} the entries to append
 * @return {Feed} the feed with the entries added
 */
export func addAll(f as Feed, entries as list of Entry) {
    for (def e in $entries) {
        $f.entries[] = $e;
    }
    return $f;
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
    $e.id = $id;
    return $e;
}
/**
 * A copy of `e` with its published instant set.
 * @param e {Entry} the source entry
 * @param t {time.Time} the first-published instant
 * @return {Entry} the updated entry
 */
export func entryPublished(e as Entry, t as time.Time) {
    $e.published = $t;
    return $e;
}
/**
 * A copy of `e` with its updated instant set.
 * @param e {Entry} the source entry
 * @param t {time.Time} the last-updated instant
 * @return {Entry} the updated entry
 */
export func entryUpdated(e as Entry, t as time.Time) {
    $e.updated = $t;
    return $e;
}
/**
 * A copy of `e` with its summary set.
 * @param e {Entry} the source entry
 * @param s {string} the summary / description text
 * @return {Entry} the updated entry
 */
export func entrySummary(e as Entry, s as string) {
    $e.summary = $s;
    return $e;
}
/**
 * A copy of `e` with its full content set (Atom).
 * @param e {Entry} the source entry
 * @param c {string} the content text
 * @return {Entry} the updated entry
 */
export func entryContent(e as Entry, c as string) {
    $e.content = $c;
    return $e;
}
/**
 * A copy of `e` with its author set.
 * @param e {Entry} the source entry
 * @param author {string} the item author (a name or email)
 * @return {Entry} the updated entry
 */
export func entryAuthor(e as Entry, author as string) {
    $e.author = $author;
    return $e;
}
/**
 * A copy of `e` with `category` appended to its category tags.
 * @param e {Entry} the source entry
 * @param category {string} the category / tag to add
 * @return {Entry} the entry with the category added
 */
export func entryCategory(e as Entry, category as string) {
    $e.categories[] = $category;
    return $e;
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
    $e.enclosure = Enclosure{url: $url, length: $length, type: $type};
    return $e;
}

# ---- dates ----

# RSS uses RFC 822 (RFC 1123) dates; Atom uses RFC 3339 (time.iso).
func rssDate(t as time.Time) {
    return time.format($t, "%a, %d %b %Y %H:%M:%S %z");
}
# malformed is the error of a document that is not a well-formed feed.
func malformed(message as string) {
    return Error{kind: "value", message: "feed.parse: " + $message};
}

# RFC822_DATE is an RFC 822 date-time (section 5, as RFC 1123 widens it to a
# 4-digit year): an optional day name, a 1- or 2-digit day, the month, a 2- or
# 4-digit year, hh:mm with optional seconds, and a zone - a numeric offset, UT
# / GMT / Z, or a North American zone name. Names match case-insensitively
# (RFC 5322 3.3). The single-letter military zones other than Z are refused:
# RFC 1123 5.2.14 notes their signs were defined backwards.
def const RFC822_DATE as string init '(?i)^(?:(?:mon|tue|wed|thu|fri|sat|sun)\s*,\s*)?' +
    '([0-9]{1,2})\s+(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)\s+([0-9]{2}|[0-9]{4})\s+' +
    '([0-9]{2}):([0-9]{2})(?::([0-9]{2}))?\s+' +
    '([+-][0-9]{4}|ut|gmt|z|est|edt|cst|cdt|mst|mdt|pst|pdt)$';

# zoneOffset maps an RFC 822 zone to a numeric offset.
func zoneOffset(zone as string) {
    def z as string init strings.upper($zone);
    def named as map of string to string init {
        "UT": "+0000",
        "GMT": "+0000",
        "Z": "+0000",
        "EST": "-0500",
        "EDT": "-0400",
        "CST": "-0600",
        "CDT": "-0500",
        "MST": "-0700",
        "MDT": "-0600",
        "PST": "-0800",
        "PDT": "-0700"
    };
    if (maps.has($named, $z)) {
        return $named[$z];
    }
    return $z;
}

# parseRssDate reads an RFC 822 date (RSS 2.0 pubDate / lastBuildDate). A
# 2-digit year is read as RFC 5322 4.3 reads RFC 822's: 00-49 is 2000-2049,
# 50-99 is 1950-1999. A day name is checked for spelling, not against the
# date. Anything else - an impossible date or time included - throws.
func parseRssDate(s as string) {
    def m as regex.Match init regex.find(RFC822_DATE, strings.trim($s));
    if ($m.start < 0) {
        throw malformed("not an RFC 822 date: " + shownValue($s));
    }
    def day as string init $m.groups[0];
    if (len($day) == 1) {
        $day = "0" + $day;
    }
    def year as string init $m.groups[2];
    if (len($year) == 2) {
        if (convert.toInt($year) < 50) {
            $year = "20" + $year;
        } else {
            $year = "19" + $year;
        }
    }
    def sec as string init $m.groups[5];
    if ($sec == "") {
        $sec = "00";
    }
    def norm as string init $day + " " + $m.groups[1] + " " + $year + " " + $m.groups[3] + ":" +
        $m.groups[4] + ":" + $sec + " " + zoneOffset($m.groups[6]);
    try {
        return time.parse($norm, "%d %b %Y %H:%M:%S %z");
    } catch (e) {
        throw malformed("not a valid date: " + shownValue($s));
    }
}

# parseAtomDate reads an RFC 3339 date-time (Atom's Date construct); anything
# else throws.
func parseAtomDate(s as string) {
    try {
        return time.fromIso(strings.trim($s));
    } catch (e) {
        throw malformed("not an RFC 3339 date: " + shownValue($s));
    }
}

# dateOf reads the date element `name` of a node with `parseDate`'s format: the
# epoch sentinel when the element is absent, an error when it does not parse.
func dateOf(node as xml.Value, name as string, ns as map of string to string, rss as bool) {
    if (not xml.has($node, $name, $ns)) {
        return epoch();
    }
    def text as string init xml.text(xml.get($node, $name, $ns));
    if ($rss) {
        return parseRssDate($text);
    }
    return parseAtomDate($text);
}

# shownValue quotes a document value for an error message, cut to 64
# characters.
func shownValue(s as string) {
    if (len($s) > 64) {
        return '"' + $s[..64] + '..."';
    }
    return '"' + $s + '"';
}

# ---- xml build / read helpers ----

# el builds an element with a single text child.
func el(tag as string, content as string) {
    return xml.setText(xml.element($tag), $content);
}
# textOf returns the text of the first child element at `path`, or "" if absent.
func textOf(node as xml.Value, path as string, ns as map of string to string) {
    if (xml.has($node, $path, $ns)) {
        return xml.text(xml.get($node, $path, $ns));
    }
    return "";
}
# atomTextOf reads an Atom Text construct (RFC 4287 3.1) - title, summary,
# content: `type="text"` / `"html"` (or none) is the element's text, the
# HTML still escaped as markup; `type="xhtml"` is the serialised children of
# its single XHTML <div> (3.1.1.3), the div itself left out. "" when absent.
func atomTextOf(node as xml.Value, path as string, ns as map of string to string) {
    if (not xml.has($node, $path, $ns)) {
        return "";
    }
    def el as xml.Value init xml.get($node, $path, $ns);
    if (attrOr($el, "type") != "xhtml") {
        return xml.text($el);
    }
    def kids as list of xml.Value init xml.children($el);
    if (len($kids) != 1 or xml.localName($kids[0]) != "div" or
        xml.namespace($kids[0]) != XHTML_NAMESPACE or strings.trim(xml.text($el)) != "") {
        throw malformed("an xhtml <" + xml.localName($el) + "> must hold exactly one XHTML <div>");
    }
    return innerXml(xml.encode($kids[0]));
}

# innerXml cuts the start and end tag off an encoded element, leaving its
# content as written; a self-closing element has none. The start tag ends at
# the first ">" outside a quoted attribute value.
func innerXml(encoded as string) {
    def quote as string init "";
    def i as int init 0;
    def n as int init len($encoded);
    while ($i < $n) {
        def c as string init $encoded[$i..$i + 1];
        if ($quote != "") {
            if ($c == $quote) {
                $quote = "";
            }
        } elseif ($c == "\"" or $c == "'") {
            $quote = $c;
        } elseif ($c == ">") {
            break;
        }
        $i = $i + 1;
    }
    if ($encoded[$i - 1..$i] == "/") {
        return "";
    }
    def close as int init strings.lastIndexOf($encoded, "</");
    return $encoded[$i + 1..$close];
}

# atomLink returns the href of the node's first alternate <link> element (one
# without a rel, or rel="alternate"), or "". A rel="enclosure" link is a media
# attachment, not the item URL, so it is skipped here.
func atomLink(node as xml.Value, ns as map of string to string) {
    def links as list of xml.Value init xml.findAll($node, "link", $ns);
    for (def l in $links) {
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
func atomAuthorOf(node as xml.Value, ns as map of string to string) {
    if (xml.has($node, "author/name", $ns)) {
        return xml.text(xml.get($node, "author/name", $ns));
    }
    if (xml.has($node, "author", $ns)) {
        return xml.text(xml.get($node, "author", $ns));
    }
    return "";
}
# rssAuthorOf reads an RSS author, preferring the native <author> and falling
# back to Dublin Core <dc:creator> (matched by namespace through ns, so any
# prefix the feed binds Dublin Core to works);
# a channel-level <managingEditor> is a third fallback. "" when none present.
func rssAuthorOf(node as xml.Value, ns as map of string to string) {
    if (xml.has($node, "author", $ns)) {
        return xml.text(xml.get($node, "author", $ns));
    }
    if (xml.has($node, "dc:creator", $ns)) {
        return xml.text(xml.get($node, "dc:creator", $ns));
    }
    if (xml.has($node, "managingEditor", $ns)) {
        return xml.text(xml.get($node, "managingEditor", $ns));
    }
    return "";
}
# categoriesText reads RSS categories (<category>text</category>) as a list.
func categoriesText(node as xml.Value, ns as map of string to string) {
    def out as list of string init [];
    def cats as list of xml.Value init xml.findAll($node, "category", $ns);
    for (def c in $cats) {
        def t as string init xml.text($c);
        if (len($t) > 0) {
            $out[] = $t;
        }
    }
    return $out;
}
# categoriesTerm reads Atom categories (<category term="..."/>) as a list.
func categoriesTerm(node as xml.Value, ns as map of string to string) {
    def out as list of string init [];
    def cats as list of xml.Value init xml.findAll($node, "category", $ns);
    for (def c in $cats) {
        def t as string init attrOr($c, "term");
        if (len($t) > 0) {
            $out[] = $t;
        }
    }
    return $out;
}
# lengthOf reads an enclosure's `length` attribute: 0 when absent (unknown),
# else a non-negative decimal integer; anything else throws.
func lengthOf(enc as xml.Value) {
    if (not xml.hasAttr($enc, "length")) {
        return 0;
    }
    def s as string init strings.trim(xml.attr($enc, "length"));
    if (not regex.matches('^[0-9]{1,18}$', $s)) {
        throw malformed("enclosure length is not a non-negative integer: " + shownValue($s));
    }
    return convert.toInt($s);
}
# readEnclosureRss reads an RSS <enclosure url length type/> from an item.
func readEnclosureRss(item as xml.Value, ns as map of string to string) {
    if (not xml.has($item, "enclosure", $ns)) {
        return noEnclosure();
    }
    def enc as xml.Value init xml.get($item, "enclosure", $ns);
    return Enclosure{url: attrOr($enc, "url"), length: lengthOf($enc), type: attrOr($enc, "type")};
}
# readEnclosureAtom reads an Atom <link rel="enclosure" href length type/>.
func readEnclosureAtom(item as xml.Value, ns as map of string to string) {
    def links as list of xml.Value init xml.findAll($item, "link", $ns);
    for (def l in $links) {
        if (attrOr($l, "rel") == "enclosure") {
            return Enclosure{
                url: attrOr($l, "href"),
                length: lengthOf($l),
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
 * @param format {string} `"rss"` or `"atom"`
 * @param f {Feed} the feed to render
 * @return {string} the feed document as XML text
 * @throws {Error} when `format` is not `"rss"` or `"atom"`
 */
export func build(format as string, f as Feed) {
    if ($format == "rss") {
        return buildRss($f);
    }
    if ($format == "atom") {
        return buildAtom($f);
    }
    throw Error{
        kind: "value",
        message: "feed.build: unknown format: " + $format + " (want \"rss\" or \"atom\")"
    };
}

func buildRss(f as Feed) {
    def channel as xml.Value init xml.element("channel");
    $channel = xml.append($channel, el("title", $f.title));
    $channel = xml.append($channel, el("link", $f.link));
    if (len($f.author) > 0) {
        $channel = xml.append($channel, el("managingEditor", $f.author));
    }
    for (def cat in $f.categories) {
        if (len($cat) > 0) {
            $channel = xml.append($channel, el("category", $cat));
        }
    }
    if (isSet($f.updated)) {
        $channel = xml.append($channel, el("lastBuildDate", rssDate($f.updated)));
    }
    for (def e in $f.entries) {
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
        for (def cat in $e.categories) {
            if (len($cat) > 0) {
                $item = xml.append($item, el("category", $cat));
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
            def enc as xml.Value init xml.setAttr(
                xml.element("enclosure"),
                "url",
                $e.enclosure.url);
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
    for (def cat in $f.categories) {
        if (len($cat) > 0) {
            $root = xml.append($root, xml.setAttr(xml.element("category"), "term", $cat));
        }
    }
    if (isSet($f.updated)) {
        $root = xml.append($root, el("updated", time.iso($f.updated)));
    }
    for (def e in $f.entries) {
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
        for (def cat in $e.categories) {
            if (len($cat) > 0) {
                $item = xml.append($item, xml.setAttr(xml.element("category"), "term", $cat));
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
 * @throws {Error} when the XML is malformed or the root is neither (kind "value")
 */
export func kind(text as string) {
    def root as xml.Value init xml.decode($text);
    return kindOf($root);
}
# kindOf names the format by the root element: an RSS 2.0 <rss> is in no
# namespace, an Atom <feed> in the Atom one (RFC 4287 2).
func kindOf(root as xml.Value) {
    def tag as string init xml.localName($root);
    def ns as string init xml.namespace($root);
    if ($tag == "rss" and $ns == "") {
        return "rss";
    }
    if ($tag == "feed" and $ns == ATOM_NAMESPACE) {
        return "atom";
    }
    if ($tag == "feed") {
        throw malformed("not an Atom feed (<feed> must be in the " + ATOM_NAMESPACE +
            " namespace)");
    }
    throw malformed("not a feed (root element <" + $tag + ">, want <rss> or an Atom <feed>)");
}

/**
 * Parse feed XML into a `Feed`, detecting RSS vs Atom from the root element.
 * An absent date is the epoch sentinel; one that is there must parse (RFC 822
 * for RSS, RFC 3339 for Atom). An Atom `type="xhtml"` title / summary /
 * content is the markup inside its XHTML `<div>`.
 * @param text {string} the feed XML (RSS 2.0 or Atom 1.0)
 * @return {Feed} the parsed feed
 * @throws {Error} kind "value" for a document that is not a well-formed feed (see the module doc);
 * the `xml` library's error for malformed XML
 */
export func parse(text as string) {
    def root as xml.Value init xml.decode($text);
    if (kindOf($root) == "rss") {
        return parseRss($root);
    }
    return parseAtom($root);
}

func parseRss(root as xml.Value) {
    # RSS 2.0 elements are in no namespace; Dublin Core is matched by its URI,
    # whatever prefix the feed binds it to.
    def ns as map of string to string init {"dc": DC_NAMESPACE};
    # RSS 2.0 wraps everything in its one <channel>.
    if (len(xml.findAll($root, "channel", $ns)) != 1) {
        throw malformed("an RSS document needs exactly one <channel>");
    }
    def channel as xml.Value init xml.get($root, "channel", $ns);
    def updated as time.Time init epoch();
    if (xml.has($channel, "lastBuildDate", $ns)) {
        $updated = dateOf($channel, "lastBuildDate", $ns, true);
    } elseif (xml.has($channel, "pubDate", $ns)) {
        $updated = dateOf($channel, "pubDate", $ns, true);
    }
    def es as list of Entry init [];
    def items as list of xml.Value init xml.findAll($channel, "item", $ns);
    for (def it in $items) {
        def e as Entry init Entry{
            title: textOf($it, "title", $ns),
            link: textOf($it, "link", $ns),
            id: textOf($it, "guid", $ns),
            published: dateOf($it, "pubDate", $ns, true),
            updated: epoch(),
            summary: textOf($it, "description", $ns),
            content: "",
            author: rssAuthorOf($it, $ns),
            categories: categoriesText($it, $ns),
            enclosure: readEnclosureRss($it, $ns)
        };
        $es[] = $e;
    }
    return Feed{
        title: textOf($channel, "title", $ns),
        link: textOf($channel, "link", $ns),
        updated: $updated,
        entries: $es,
        author: rssAuthorOf($channel, $ns),
        categories: categoriesText($channel, $ns)
    };
}

func parseAtom(root as xml.Value) {
    # Read Atom elements in the Atom namespace (kindOf checked the root is in
    # it), as a default namespace or under any prefix.
    def ns as map of string to string init {"": ATOM_NAMESPACE};
    def es as list of Entry init [];
    def items as list of xml.Value init xml.findAll($root, "entry", $ns);
    for (def it in $items) {
        def e as Entry init Entry{
            title: atomTextOf($it, "title", $ns),
            link: atomLink($it, $ns),
            id: textOf($it, "id", $ns),
            published: dateOf($it, "published", $ns, false),
            updated: dateOf($it, "updated", $ns, false),
            summary: atomTextOf($it, "summary", $ns),
            content: atomTextOf($it, "content", $ns),
            author: atomAuthorOf($it, $ns),
            categories: categoriesTerm($it, $ns),
            enclosure: readEnclosureAtom($it, $ns)
        };
        $es[] = $e;
    }
    return Feed{
        title: atomTextOf($root, "title", $ns),
        link: atomLink($root, $ns),
        updated: dateOf($root, "updated", $ns, false),
        entries: $es,
        author: atomAuthorOf($root, $ns),
        categories: categoriesTerm($root, $ns)
    };
}
