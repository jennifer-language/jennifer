# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * Exercises the `xml` library: decode a document, walk it with the accessors
 * and an XPath-style path, then build one from scratch and encode it. Every
 * output is deterministic, so it doubles as a golden test.
 * @module xml
 */

use io;
use xml;

# A small catalog, with an attribute, nested elements, and an entity.
def doc as string init "<catalog>" +
    "<book id=\"a1\"><title>Go &amp; XML</title><price>39</price></book>" +
    "<book id=\"a2\"><title>More</title></book>" + "</catalog>";

def root as xml.Value init xml.decode($doc);
io.printf("root tag: %s\n", xml.tag($root));
io.printf("books: %d\n", len(xml.findAll($root, "book")));

# Walk each book by path; read an attribute and child text.
def books as list of xml.Value init xml.findAll($root, "book");
for (def bk in $books) {
    io.printf("  book %s -> %s\n", xml.attr($bk, "id"), xml.text(xml.get($bk, "title")));
}

# Path indexing and has().
io.printf("book[1] price: %s\n", xml.text(xml.get($root, "book[1]/price")));
io.printf("has book/title: %t\n", xml.has($root, "book/title"));
io.printf("has book/isbn:  %t\n", xml.has($root, "book/isbn"));

# Build a document and serialize it (attribute + text are escaped).
def note as xml.Value init xml.setText(
    xml.setAttr(xml.element("note"), "level", "info"),
    "1 < 2 & ok");
def page as xml.Value init xml.append(xml.element("page"), $note);
io.printf("built: %s\n", xml.encode($page));
io.printf("pretty:\n%s\n", xml.encodePretty($root));

# Namespaces: two KML files, one with a default namespace and one with a kml:
# prefix, read by one path through a prefix map of your own choosing.
def const KML as string init "http://www.opengis.net/kml/2.2";
def plainKml as string init "<kml xmlns=\"{KML}\"><Document><name>A</name></Document></kml>";
def prefixedKml as string init "<k:kml xmlns:k=\"{KML}\">" +
    "<k:Document><k:name>B</k:name></k:Document></k:kml>";
for (def src in [$plainKml, $prefixedKml]) {
    def kml as xml.Value init xml.decode($src);
    def d as xml.Value init xml.get($kml, "g:Document", {"g": KML});
    io.printf(
        "%s: local %s, name %s\n",
        xml.tag($d),
        xml.localName($d),
        xml.text(xml.get($d, "*:name")));
}

# A fragment cut out of a larger document decodes against its root's bindings,
# and encode declares what it needs.
def frag as xml.Value init xml.decode("<k:Placemark><k:name>C</k:name></k:Placemark>", {"k": KML});
io.printf("fragment ns: %s\n", xml.namespace($frag));
io.printf("fragment: %s\n", xml.encode($frag));

# Build namespaced elements; the declaration is added where it is needed.
def mark as xml.Value init xml.setAttrNS(
    xml.elementNS(KML, "kml:Placemark"),
    "http://www.w3.org/XML/1998/namespace",
    "xml:lang",
    "en");
io.printf("built ns: %s\n", xml.encode($mark));
