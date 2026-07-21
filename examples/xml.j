# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Exercises the `xml` library: decode a document, walk it with the accessors
 * and an XPath-style path, then build one from scratch and encode it. Every
 * output is deterministic, so it doubles as a golden test.
 * @module xml
 */

use io;
use xml;

# A small catalog, with an attribute, nested elements, and an entity.
def doc as string init "<catalog>"
    + "<book id=\"a1\"><title>Go &amp; XML</title><price>39</price></book>"
    + "<book id=\"a2\"><title>More</title></book>"
    + "</catalog>";

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
    xml.setAttr(xml.element("note"), "level", "info"), "1 < 2 & ok");
def page as xml.Value init xml.append(xml.element("page"), $note);
io.printf("built: %s\n", xml.encode($page));
io.printf("pretty:\n%s\n", xml.encodePretty($root));
