# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

# The PDF layout engine and the HTML / ANSI renderers legitimately run past the
# L201 statement-count limit in a few places. Every other lint check stays active.
# lint-disable-file: L201

/**
 * A format-neutral document tree and its renderers. A `Node` is one element of
 * a document - a heading, a paragraph, a list, a table, a link, a run of text -
 * with its children; a tree of them renders to HTML, XHTML, styled terminal
 * text (ANSI) or a paginated PDF. Readers build trees (`markdown.parse`,
 * `rst.parse`, `myst.parse`), programs build them by hand, and the accessors (`typeOf` /
 * `children` / `text` / `level` / `attr` / `get` / `findAll` / `has`) walk and
 * inspect them - the same vocabulary as `xml` / `html`.
 *
 * Rendering is safe by default: text is escaped, link and image URLs go through
 * an allow-list (`javascript:` becomes `#`), attribute names outside a small
 * allow-list are dropped, and raw HTML blocks are escaped unless `renderHtml` is
 * asked to pass them through.
 * @module doctree
 * @example
 * import "doctree.j" as doctree;
 * def doc as doctree.Node init doctree.Node{kind: "document", children: [
 *     doctree.Node{kind: "heading", level: 1, children: [doctree.Node{kind: "text", text: "Hi"}]}
 * ]};
 * io.printf("%s\n", doctree.render($doc, "html"));
 */
import "./html.j" as html;
import "./ansi.j" as ansi;
import "./pdf.j" as pdf;
use strings;
use regex;
use convert;
use lists;
use maps;
use math;
use encoding;

/**
 * One element of a document tree. Its `kind` is a block - `"document"`,
 * `"heading"`, `"paragraph"`, `"code"`, `"list"` (of `"item"`s),
 * `"definition_list"` (of `"def_term"` / `"def_desc"`), `"table"` (of `"row"`s of
 * `"cell"`s), `"quote"`, `"admonition"`, `"thematic_break"`, `"html_block"`,
 * `"page_break"`, `"line_block"` (of `"line"`s), `"footnote"` / `"citation"`,
 * `"figure"` (an image, `"caption"`, `"legend"`), `"topic"`, `"rubric"`,
 * `"container"` - or an inline - `"text"`, `"codespan"`, `"strong"`, `"emphasis"`,
 * `"strikethrough"`, `"highlight"`, `"subscript"`, `"superscript"`, `"link"`,
 * `"image"`, `"line_break"`, `"html_inline"`, `"footnote_ref"` / `"citation_ref"`,
 * `"span"`, `"cite"`, `"abbr"`. An `item`, `def_desc`, `cell`, footnote or
 * admonition may hold blocks as well as inline nodes.
 * @field kind {string} the node kind
 * @field level {int} a heading's level (1-6); 0 otherwise
 * @field text {string} literal text (a `text` / `codespan` / `code` node, an
 *   `image`'s alt, a `footnote_ref` / `citation_ref`'s label); "" for a container
 *   (read its content with the `text` accessor)
 * @field lang {string} a fenced `code` block's language ("" if none)
 * @field ordered {bool} whether a `list` is ordered
 * @field url {string} a `link` / `image` target, or a note reference's `#id`
 * @field title {string} a `link` / `image` title, a `topic`'s title, or an
 *   `abbr`'s expansion ("" if none)
 * @field align {string} a table `cell`'s alignment ("left" / "right" / "center" / "")
 * @field attrs {map of string to string} extra named attributes: a heading /
 *   link / image / code's `{#id .class key="value"}` attribute list (`id` /
 *   `class` / any key, on any node in a hand-built tree), a fenced block's `info`
 *   string, a task `item`'s `task` / `checked`, an ordered `list`'s `start` and
 *   `enumtype` (`1` / `a` / `A` / `i` / `I`), a `table`'s `headerRows`, a `cell`'s
 *   `colspan` / `rowspan`, a footnote's `label` and `backref`. Read with
 *   `attr(node, name)`.
 * @field children {list of Node} the child nodes
 */
export def struct Node {
    kind as string,
    level as int,
    text as string,
    lang as string,
    ordered as bool,
    url as string,
    title as string,
    align as string,
    attrs as map of string to string,
    children as list of Node
};

# treeFail throws a catchable doctree error.
func treeFail(msg as string) {
    throw Error{kind: "doctree", message: $msg};
}

# nodeOf returns a zero-valued Node with only its kind set; callers fill the fields
# that kind uses (value semantics - each call is a fresh node).
func nodeOf(kind as string) {
    def n as Node;
    $n.kind = $kind;
    return $n;
}

func textNode(s as string) {
    def n as Node init nodeOf("text");
    $n.text = $s;
    return $n;
}

/**
 * A page-break node. Placed in a document tree (typically between the blocks of a
 * hand-assembled book), it makes `renderPdf` start the following content
 * on a fresh page - a lever independent of the level-one-heading page break. The
 * HTML and ANSI renderers ignore it. In Markdown source, a lone
 * `<!-- pagebreak -->` comment parses to the same node.
 * @return {Node} a page_break node
 */
export func pageBreak() {
    return nodeOf("page_break");
}

/**
 * The kind of a node (`"document"`, `"heading"`, `"link"`, `"text"`, ...).
 * @param node {Node} the node
 * @return {string} the node kind
 */
export func typeOf(node as Node) {
    return $node.kind;
}

/**
 * A node's direct children.
 * @param node {Node} the node
 * @return {list of Node} the child nodes
 */
export func children(node as Node) {
    return $node.children;
}

/**
 * A heading's level (1-6); 0 for any other node.
 * @param node {Node} the node
 * @return {int} the heading level
 */
export func level(node as Node) {
    return $node.level;
}

/**
 * The text content of a node: a leaf's own literal text, else the concatenation of
 * its descendants' text - so `text` of a heading (or any container) is its full
 * flattened text, handy for a table of contents.
 * @param node {Node} the node
 * @return {string} the flattened text
 */
export func text(node as Node) {
    if (len($node.children) == 0) {
        return $node.text;
    }
    def out as list of string init [];
    for (def c in $node.children) {
        $out[] = text($c);
    }
    return strings.join($out, "");
}

/**
 * A named string attribute of a node: `"href"` / `"title"` (a `link` / `image`),
 * `"lang"` (a `code` block), `"kind"` and `"title"` (an `admonition`: the alert
 * kind, and the custom title or ""), `"align"` (a table `cell`), `"ordered"`
 * ("true" / "false", a `list`), or `"level"` (a heading, as a string). Returns ""
 * for an absent attribute.
 * @param node {Node} the node
 * @param name {string} the attribute name
 * @return {string} the attribute value, or ""
 */
export func attr(node as Node, name as string) {
    match ($name) {
        when "href", "url" { return $node.url; }
        when "title" { return $node.title; }
        when "lang", "language" { return $node.lang; }
        when "kind" {
            # The alert kind of an admonition, and only that: on any other node
            # `kind` is not a meaningful attribute (a code block's language is
            # `lang`, which shares the field but answers a different question).
            if ($node.kind == "admonition") {
                return $node.lang;
            }
            return "";
        }
        when "align" { return $node.align; }
        when "ordered" { if ($node.ordered) {
            return "true";
        }
        return "false"; }
        when "level" { return "{$node.level}"; }
        else {
            # Any other name falls back to the extra-attributes map: `id` / `class`
            # and custom `key="value"` from an attribute list, a fenced block's
            # `info`, a task item's `task` / `checked`, a footnote ref's `label` /
            # `number`. Absent -> "".
            if (maps.has($node.attrs, $name)) {
                return $node.attrs[$name];
            }
            return "";
        }
    }
}

# An XPath-ish selector step: a kind name (or "*") plus an optional 1-based index.
def struct MdStep {
    name as string,
    index as int
};

# mdParseSteps splits a "/"-separated selector ("list/item") into steps.
func mdParseSteps(selector as string) {
    def steps as list of MdStep init [];
    for (def raw in strings.split($selector, "/")) {
        def s as string init strings.trim($raw);
        if (len($s) > 0) {
            def name as string init $s;
            def idx as int init 0;
            def lb as int init strings.indexOf($s, "[");
            if ($lb >= 0 and strings.endsWith($s, "]")) {
                $name = $s[..$lb];
                def inner as string init $s[$lb + 1..len($s) - 1];
                try {
                    $idx = convert.toInt($inner);
                } catch (e) {
                    $idx = 0;
                }
            }
            $steps[] = MdStep{name: strings.lower($name), index: $idx};
        }
    }
    return $steps;
}

/**
 * Every node matching a `/`-separated `selector` relative to `node`: each step is a
 * kind name, `*` (any kind), or `name[k]` (the k-th such child, 1-based). Steps match
 * direct children.
 * @param node {Node} the node to search under
 * @param selector {string} the selector path (e.g. "list/item")
 * @return {list of Node} the matching nodes (empty if none)
 */
export func findAll(node as Node, selector as string) {
    def frontier as list of Node init [$node];
    for (def step in mdParseSteps($selector)) {
        def next as list of Node init [];
        for (def parent in $frontier) {
            def matchNum as int init 0;
            for (def child in $parent.children) {
                if ($step.name == "*" or $child.kind == $step.name) {
                    $matchNum = $matchNum + 1;
                    if ($step.index == 0 or $step.index == $matchNum) {
                        $next[] = $child;
                    }
                }
            }
        }
        $frontier = $next;
    }
    return $frontier;
}

/**
 * The first node matching `selector` (see `findAll`), or an empty node (`typeOf` "")
 * if there is no match.
 * @param node {Node} the node to search under
 * @param selector {string} the selector path
 * @return {Node} the first match, or an empty node
 */
export func get(node as Node, selector as string) {
    def all as list of Node init findAll($node, $selector);
    if (len($all) > 0) {
        return $all[0];
    }
    return nodeOf("");
}

/**
 * Whether any node matches `selector` (see `findAll`).
 * @param node {Node} the node to search under
 * @param selector {string} the selector path
 * @return {bool} true if at least one node matches
 */
export func has(node as Node, selector as string) {
    return len(findAll($node, $selector)) > 0;
}

# --- HTML helpers shared by the node renderer ----------------------

# wrapEl wraps text in a simple element (`html.text` escapes the content).
func wrapEl(tag as string, text as string) {
    def kids as list of html.Node init [];
    $kids[] = html.text($text);
    return html.element($tag, [], $kids);
}

# safeHref returns url when it is safe in an href or src - http / https / mailto
# or a scheme-less reference - and "#" otherwise (javascript:, data:, ...). The
# allowlist is html.safeUrl's, so the policy has one home.
func safeHref(url as string) {
    return html.safeUrl($url);
}

# mdAttrSafe gates which custom `{key=value}` attributes from a `{...}` list reach
# the HTML output. Rendering is safe by default, so only a fixed allowlist of
# presentational / semantic names (plus the data-* and aria-* families) is
# emitted. Everything else is dropped - crucially the `on*` event handlers and
# `style`, which pass html.attr's name-shape check yet would let untrusted
# Markdown forge a live event handler (`![x](u){onerror="..."}`) or inject CSS.
func mdAttrSafe(name as string) {
    # The name must be a valid HTML attribute name (ASCII letter start, then
    # letters / digits / hyphen) - the exact shape html.attr enforces. An
    # untrusted `{...}` key like `data-x"y` or `data-é` passed the data- / aria-
    # prefix check but then made html.attr throw, aborting the render; it is dropped
    # here instead. Checked on the original name the renderer would emit.
    if (not regex.matches("^[A-Za-z][A-Za-z0-9-]*$", $name)) {
        return false;
    }
    def n as string init strings.lower($name);
    if (strings.startsWith($n, "data-") or strings.startsWith($n, "aria-")) {
        return true;
    }
    def allow as list of string init [
        "id",
        "class",
        "title",
        "width",
        "height",
        "align",
        "dir",
        "lang",
        "role",
        "target",
        "rel",
        "colspan",
        "rowspan",
        "scope",
        "headers"
    ];
    return lists.contains($allow, $n);
}

# mdAttrsToHtml renders a node's attribute-list attrs (id / class / custom key=val
# from a `{...}`) as html attributes, id then class then the rest by key. Used by
# the heading / link / image renderers; other renderers ignore these attrs. Custom
# keys are filtered through mdAttrSafe so a `{...}` list cannot introduce an unsafe
# attribute into the safe-by-default output.
func mdAttrsToHtml(node as Node) {
    def out as list of html.Attr init [];
    if (maps.has($node.attrs, "id")) {
        $out[] = html.attr("id", $node.attrs["id"]);
    }
    if (maps.has($node.attrs, "class")) {
        $out[] = html.attr("class", $node.attrs["class"]);
    }
    for (def k in maps.keys($node.attrs)) {
        if (not ($k == "id") and not ($k == "class") and mdAttrSafe($k)) {
            $out[] = html.attr($k, $node.attrs[$k]);
        }
    }
    return $out;
}

# imageNode builds an `<img>`: the src through the same scheme allowlist as a
# link href, the alt and title as escaped attributes.
func imageNode(alt as string, url as string, title as string, extra as list of html.Attr) {
    def attrs as list of html.Attr init [];
    $attrs[] = html.attr("src", safeHref($url));
    $attrs[] = html.attr("alt", $alt);
    if (not ($title == "")) {
        $attrs[] = html.attr("title", $title);
    }
    for (def a in $extra) {
        $attrs[] = $a;
    }
    def noKids as list of html.Node init [];
    return html.element("img", $attrs, $noKids);
}

# alignOf returns the alignment for column `i`, or "none" past the list end.
func alignOf(aligns as list of string, i as int) {
    if ($i < len($aligns)) {
        return $aligns[$i];
    }
    return "none";
}

# --- shared tree helpers (block kinds, attributes, lists, tables) ----

# The node kinds that render as blocks. A container whose children mix inline
# and block nodes (an `item`, a `def_desc`, a `cell`, a `footnote`) renders each
# child by this test, so a list item can hold paragraphs, code and nested lists
# as well as a run of inline text.
def const BLOCK_KINDS as list of string init [
    "heading",
    "paragraph",
    "code",
    "list",
    "definition_list",
    "table",
    "quote",
    "admonition",
    "thematic_break",
    "html_block",
    "page_break",
    "line_block",
    "footnote",
    "citation",
    "figure",
    "topic",
    "rubric",
    "container"
];

func isBlockKind(kind as string) {
    return lists.contains(BLOCK_KINDS, $kind);
}

func hasBlockChild(n as Node) {
    for (def c in $n.children) {
        if (isBlockKind($c.kind)) {
            return true;
        }
    }
    return false;
}

# blockify groups each run of inline children into a paragraph, so a renderer
# that lays out blocks one after another (ANSI, PDF) can take mixed content.
func blockify(ns as list of Node) {
    def out as list of Node init [];
    def run as list of Node init [];
    for (def c in $ns) {
        if (isBlockKind($c.kind)) {
            if (len($run) > 0) {
                def p as Node init nodeOf("paragraph");
                $p.children = $run;
                $out[] = $p;
                $run = [];
            }
            $out[] = $c;
        } else {
            $run[] = $c;
        }
    }
    if (len($run) > 0) {
        def p as Node init nodeOf("paragraph");
        $p.children = $run;
        $out[] = $p;
    }
    return $out;
}

# plainText is a node's text for a renderer that can only draw a flat string
# (a PDF table cell): block children are joined by a space, so two paragraphs in
# a cell do not run together the way `text` (which joins with "") would.
func plainText(n as Node) {
    if (not hasBlockChild($n)) {
        return text($n);
    }
    def parts as list of string init [];
    for (def c in $n.children) {
        $parts[] = plainText($c);
    }
    return strings.join($parts, " ");
}

# withNodeAttrs adds a node's own attributes (`id`, `class`, the allowlisted
# rest - see mdAttrSafe) to the attributes a renderer built for its element. A
# `class` on both is merged into one ("admonition admonition-note extra"), and
# `id` / `class` come first. A node with no attributes returns `base` as is.
func withNodeAttrs(base as list of html.Attr, node as Node) {
    def extra as list of html.Attr init mdAttrsToHtml($node);
    if (len($extra) == 0) {
        return $base;
    }
    def cls as string init "";
    def id as string init "";
    def rest as list of html.Attr init [];
    for (def a in $base) {
        if ($a.name == "class") {
            $cls = $a.value;
        } else {
            $rest[] = $a;
        }
    }
    for (def a in $extra) {
        if ($a.name == "class") {
            if ($cls == "") {
                $cls = $a.value;
            } else {
                $cls = $cls + " " + $a.value;
            }
        } elseif ($a.name == "id") {
            $id = $a.value;
        } else {
            $rest[] = $a;
        }
    }
    def out as list of html.Attr init [];
    if ($id != "") {
        $out[] = html.attr("id", $id);
    }
    if ($cls != "") {
        $out[] = html.attr("class", $cls);
    }
    for (def a in $rest) {
        $out[] = $a;
    }
    return $out;
}

# smallIntAttr reads a non-negative integer attribute (a list's `start`, a
# table's `headerRows`), or `dflt` when it is absent or not a plain number - a
# hand-built tree cannot make a renderer throw through a malformed attribute.
func smallIntAttr(n as Node, name as string, dflt as int) {
    if (not maps.has($n.attrs, $name)) {
        return $dflt;
    }
    def v as string init $n.attrs[$name];
    if (not regex.matches('^[0-9]{1,9}$', $v)) {
        return $dflt;
    }
    return convert.toInt($v);
}

# listEnumType is an ordered list's numbering style: "1" (the default), "a" /
# "A" (letters) or "i" / "I" (roman numerals), from the `enumtype` attribute.
func listEnumType(n as Node) {
    def t as string init attr($n, "enumtype");
    if ($t == "a" or $t == "A" or $t == "i" or $t == "I") {
        return $t;
    }
    return "1";
}

# romanOf writes 1..3999 as a lower-case roman numeral ("" outside that range).
func romanOf(n as int) {
    if ($n < 1 or $n > 3999) {
        return "";
    }
    def vals as list of int init [1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1];
    def syms as list of string init [
        "m",
        "cm",
        "d",
        "cd",
        "c",
        "xc",
        "l",
        "xl",
        "x",
        "ix",
        "v",
        "iv",
        "i"
    ];
    def out as list of string init [];
    def rest as int init $n;
    def i as int init 0;
    while ($rest > 0) {
        while ($rest >= $vals[$i]) {
            $out[] = $syms[$i];
            $rest = $rest - $vals[$i];
        }
        $i = $i + 1;
    }
    return strings.join($out, "");
}

# lettersOf writes n >= 1 as bijective base-26 letters: a..z, aa, ab, ...
func lettersOf(n as int) {
    def out as list of string init [];
    def rest as int init $n;
    while ($rest > 0) {
        def d as int init ($rest - 1) % 26;
        $out[] = convert.fromCodepoint(97 + $d);
        $rest = ($rest - 1) // 26;
    }
    return strings.join(lists.reverse($out), "");
}

# enumMarker is the number an ordered-list item shows in the ANSI and PDF
# renderers (HTML leaves it to the browser through `start` / `type`). A value
# outside what letters or numerals can write falls back to digits.
func enumMarker(enumType as string, n as int) {
    if ($n >= 1) {
        match ($enumType) {
            when "a" { return lettersOf($n); }
            when "A" { return strings.upper(lettersOf($n)); }
            when "i" {
                if ($n <= 3999) {
                    return romanOf($n);
                }
            }
            when "I" {
                if ($n <= 3999) {
                    return strings.upper(romanOf($n));
                }
            }
        }
    }
    return "{$n}";
}

# headerRowCount is how many leading rows of a table are its header: the
# `headerRows` attribute when set (0 for a table without a header), else 1 (the
# GFM shape).
func headerRowCount(n as Node) {
    return smallIntAttr($n, "headerRows", 1);
}

# gridRows lays a table's rows out as a rectangular grid for the renderers that
# draw fixed columns (ANSI, PDF): a cell with `colspan` / `rowspan` keeps its
# content in its first slot and the slots it covers become empty cells, so the
# columns line up. HTML keeps the spans themselves. Each grid row is padded to
# the widest row with empty cells. A span is capped at 1000, far past any table
# that renders legibly, so a hand-built tree cannot make this allocate without
# bound.
func gridRows(table as Node) {
    def grid as list of list of Node init [];
    # pending[c] is how many more rows column c is covered by a rowspan above.
    def pending as list of int init [];
    def blank as Node init nodeOf("cell");
    def width as int init 0;
    for (def row in $table.children) {
        def out as list of Node init [];
        def col as int init 0;
        for (def cell in $row.children) {
            # Skip the slots a rowspan from an earlier row still covers.
            while ($col < len($pending) and $pending[$col] > 0) {
                $out[] = $blank;
                $pending[$col] = $pending[$col] - 1;
                $col = $col + 1;
            }
            def cs as int init smallIntAttr($cell, "colspan", 1);
            def rs as int init smallIntAttr($cell, "rowspan", 1);
            if ($cs < 1) {
                $cs = 1;
            }
            if ($cs > 1000) {
                $cs = 1000;
            }
            if ($rs < 1) {
                $rs = 1;
            }
            if ($rs > 1000) {
                $rs = 1000;
            }
            # The cell fills its first slot; the rest of its span is empty.
            $out[] = $cell;
            for (def k in 1..$cs) {
                $out[] = $blank;
            }
            while (len($pending) < $col + $cs) {
                $pending[] = 0;
            }
            for (def k in 0..$cs) {
                $pending[$col + $k] = $rs - 1;
            }
            $col = $col + $cs;
        }
        # Trailing slots still covered from above.
        while ($col < len($pending)) {
            if ($pending[$col] > 0) {
                $pending[$col] = $pending[$col] - 1;
            }
            $out[] = $blank;
            $col = $col + 1;
        }
        if (len($out) > $width) {
            $width = len($out);
        }
        $grid[] = $out;
    }
    for (def r in 0..len($grid)) {
        while (len($grid[$r]) < $width) {
            $grid[$r][] = $blank;
        }
    }
    return $grid;
}

# gridTable rebuilds a table node from gridRows, so the ANSI and PDF table code
# keeps working on plain rows of cells.
func gridTable(table as Node) {
    def out as Node init $table;
    def rows as list of Node init [];
    for (def cells in gridRows($table)) {
        def r as Node init nodeOf("row");
        $r.children = $cells;
        $rows[] = $r;
    }
    $out.children = $rows;
    return $out;
}

# hasSpans reports whether any cell of a table spans rows or columns.
func hasSpans(table as Node) {
    for (def row in $table.children) {
        for (def cell in $row.children) {
            if (maps.has($cell.attrs, "colspan") or maps.has($cell.attrs, "rowspan")) {
                return true;
            }
        }
    }
    return false;
}

# noteLabel is how a footnote or citation shows its label: in brackets.
func noteLabel(n as Node) {
    def label as string init attr($n, "label");
    if ($n.kind == "footnote_ref" or $n.kind == "citation_ref") {
        $label = $n.text;
    }
    return "[" + $label + "]";
}

# indentLines prefixes every line of s with `prefix`. Lines collect and join
# once: growing a string with `+` per line is O(N^2) over the block's size.
func indentLines(s as string, prefix as string) {
    def out as list of string init [];
    def first as bool init true;
    for (def line in strings.split($s, "\n")) {
        if (not $first) {
            $out[] = "\n";
        }
        $first = false;
        $out[] = $prefix + $line;
    }
    return strings.join($out, "");
}

# isWideRune approximates the Unicode East-Asian Width "W"/"F" categories plus
# the common emoji blocks - the code points a terminal renders two columns wide.
func isWideRune(cp as int) {
    # The ranges below span 0x1100..0x3FFFD; every script below Han / fullwidth
    # (Latin, Greek, Cyrillic, etc. - the common case) exits on the first guard
    # instead of evaluating the whole range chain.
    if ($cp < 0x1100 or $cp > 0x3FFFD) {
        return false;
    }
    return ($cp >= 0x1100 and $cp <= 0x115F) or ($cp >= 0x2E80 and $cp <= 0x303E) or
        ($cp >= 0x3041 and $cp <= 0x33FF) or ($cp >= 0x3400 and $cp <= 0x4DBF) or
        ($cp >= 0x4E00 and $cp <= 0x9FFF) or ($cp >= 0xA000 and $cp <= 0xA4CF) or
        ($cp >= 0xAC00 and $cp <= 0xD7A3) or ($cp >= 0xF900 and $cp <= 0xFAFF) or
        ($cp >= 0xFE30 and $cp <= 0xFE4F) or ($cp >= 0xFF00 and $cp <= 0xFF60) or
        ($cp >= 0xFFE0 and $cp <= 0xFFE6) or ($cp >= 0x1F300 and $cp <= 0x1FAFF) or
        ($cp >= 0x20000 and $cp <= 0x3FFFD);
}

# displayWidth is the terminal column width of s: East-Asian wide / fullwidth
# runes occupy two columns, so a table built from CJK / emoji cells aligns
# instead of running one column short per wide rune. Pure ASCII (the common
# cell) returns its rune count directly - one Go check, no per-rune loop.
func displayWidth(s as string) {
    if (encoding.isAscii(convert.bytesFromString($s, "utf-8"))) {
        return len($s);
    }
    def w as int init 0;
    for (def ch in strings.chars($s)) {
        if (isWideRune(convert.toCodepoint($ch))) {
            $w = $w + 2;
        } else {
            $w = $w + 1;
        }
    }
    return $w;
}

# padCell pads `styled` (whose visible width is `plain`) to `width` per
# alignment.
func padCell(styled as string, plain as int, width as int, align as string) {
    def gap as int init $width - $plain;
    if ($gap <= 0) {
        return $styled;
    }
    if ($align == "right") {
        return strings.repeat(" ", $gap) + $styled;
    }
    if ($align == "center") {
        def left as int init $gap // 2;
        return strings.repeat(" ", $left) + $styled + strings.repeat(" ", $gap - $left);
    }
    return $styled + strings.repeat(" ", $gap);
}

# ansiDivider renders the `---+---` rule under the header row.
func ansiDivider(widths as list of int) {
    def parts as list of string init [];
    def i as int init 0;
    while ($i < len($widths)) {
        $parts[] = strings.repeat("-", $widths[$i]);
        $i = $i + 1;
    }
    return strings.join($parts, "-+-");
}

# inlineNodeToHtml maps one inline Node to an html node (the inline model is flat,
# so a `strong` / `emphasis` / `link` carries its text in a single `text` child).
func inlineNodeToHtml(n as Node, allowRaw as bool) {
    match ($n.kind) {
        when "text" { return html.text($n.text); }
        when "html_inline" {
            # Raw inline HTML from the source: escaped unless the caller trusts
            # the document (allowRawHtml), like an html_block.
            if ($allowRaw) {
                return html.raw($n.text);
            }
            return html.text($n.text);
        }
        when "line_break" {
            def noKids as list of html.Node init [];
            return html.element("br", [], $noKids);
        }
        when "codespan" {
            def codeText as list of html.Node init [html.text($n.text)];
            return html.element("code", withNodeAttrs([], $n), $codeText);
        }
        when "footnote_ref", "citation_ref" {
            # A reference to a footnote or citation: a bracketed label linking to
            # the note, carrying its own `id` so the note can link back.
            def refAttrs as list of html.Attr init [
                html.attr("class", strings.replace($n.kind, "_ref", "-reference")),
                html.attr("href", safeHref($n.url))
            ];
            if ($n.kind == "footnote_ref") {
                $refAttrs[] = html.attr("role", "doc-noteref");
            }
            def refText as list of html.Node init [html.text(noteLabel($n))];
            return html.element("a", withNodeAttrs($refAttrs, $n), $refText);
        }
        when "span" {
            return html.element(
                "span",
                withNodeAttrs([], $n),
                inlineNodesToHtml($n.children, $allowRaw));
        }
        when "cite" {
            return html.element(
                "cite",
                withNodeAttrs([], $n),
                inlineNodesToHtml($n.children, $allowRaw));
        }
        when "abbr" {
            def abbrAttrs as list of html.Attr init [];
            if ($n.title != "") {
                $abbrAttrs[] = html.attr("title", $n.title);
            }
            return html.element(
                "abbr",
                withNodeAttrs($abbrAttrs, $n),
                inlineNodesToHtml($n.children, $allowRaw));
        }
        when "strong" {
            return html.element("strong", [], inlineNodesToHtml($n.children, $allowRaw));
        }
        when "emphasis" { return html.element("em", [], inlineNodesToHtml($n.children, $allowRaw)); }
        when "strikethrough" {
            return html.element("del", [], inlineNodesToHtml($n.children, $allowRaw));
        }
        when "highlight" {
            return html.element("mark", [], inlineNodesToHtml($n.children, $allowRaw));
        }
        when "subscript" {
            return html.element("sub", [], inlineNodesToHtml($n.children, $allowRaw));
        }
        when "superscript" {
            return html.element("sup", [], inlineNodesToHtml($n.children, $allowRaw));
        }
        when "link" {
            def attrs as list of html.Attr init [];
            $attrs[] = html.attr("href", safeHref($n.url));
            if (not ($n.title == "")) {
                $attrs[] = html.attr("title", $n.title);
            }
            # `{.class key="v"}` attributes from an attribute list.
            for (def a in mdAttrsToHtml($n)) {
                $attrs[] = $a;
            }
            return html.element("a", $attrs, inlineNodesToHtml($n.children, $allowRaw));
        }
        when "image" { return imageNode($n.text, $n.url, $n.title, mdAttrsToHtml($n)); }
        else { return html.text(text($n)); }
    }
}

func inlineNodesToHtml(ns as list of Node, allowRaw as bool) {
    def out as list of html.Node init [];
    for (def c in $ns) {
        $out[] = inlineNodeToHtml($c, $allowRaw);
    }
    return $out;
}

# listNodeToHtml renders a `list` node: each `item`'s children are inline nodes plus
# an optional nested `list` (rendered as a child `<ul>` / `<ol>` inside the `<li>`).
func listNodeToHtml(n as Node, allowRaw as bool) {
    def lis as list of html.Node init [];
    for (def item in $n.children) {
        def kids as list of html.Node init [];
        def liAttrs as list of html.Attr init [];
        # GFM task list item: a disabled checkbox before the content, and a
        # `task-list-item` class on the <li> (what GFM and its stylesheets use).
        if (attr($item, "task") == "true") {
            $liAttrs[] = html.attr("class", "task-list-item");
            def box as list of html.Attr init [
                html.attr("type", "checkbox"),
                html.boolAttr("disabled")
            ];
            if (attr($item, "checked") == "true") {
                $box[] = html.boolAttr("checked");
            }
            $kids[] = html.element("input", $box, []);
            $kids[] = html.text(" ");
        }
        for (def c in mixedToHtml($item.children, $allowRaw)) {
            $kids[] = $c;
        }
        $lis[] = html.element("li", withNodeAttrs($liAttrs, $item), $kids);
    }
    if ($n.ordered) {
        # A list that does not count from 1, or counts in letters / numerals,
        # says so through the HTML `start` / `type` attributes.
        def olAttrs as list of html.Attr init [];
        def start as int init smallIntAttr($n, "start", 1);
        if ($start != 1) {
            $olAttrs[] = html.attr("start", "{$start}");
        }
        def et as string init listEnumType($n);
        if ($et != "1") {
            $olAttrs[] = html.attr("type", $et);
        }
        return html.element("ol", withNodeAttrs($olAttrs, $n), $lis);
    }
    return html.element("ul", withNodeAttrs([], $n), $lis);
}

# mixedToHtml renders a container's children, each as a block or an inline node
# by its kind, so an item / description / cell can hold paragraphs and code as
# well as inline text.
func mixedToHtml(ns as list of Node, allowRaw as bool) {
    def out as list of html.Node init [];
    for (def c in $ns) {
        if (isBlockKind($c.kind)) {
            $out[] = nodeToHtml($c, $allowRaw);
        } else {
            $out[] = inlineNodeToHtml($c, $allowRaw);
        }
    }
    return $out;
}

# tableCellToHtml builds a `th` / `td` with the cell's content and an `align`
# attribute (omitted when unset / "none"); `colspan` / `rowspan` and the other
# allowlisted attributes come from the cell's attrs.
func tableCellToHtml(tag as string, cell as Node, allowRaw as bool) {
    def attrs as list of html.Attr init [];
    if (not ($cell.align == "none") and not ($cell.align == "")) {
        $attrs[] = html.attr("align", $cell.align);
    }
    return html.element($tag, withNodeAttrs($attrs, $cell), mixedToHtml($cell.children, $allowRaw));
}

func tableRowToHtml(tag as string, row as Node, allowRaw as bool) {
    def tds as list of html.Node init [];
    for (def cell in $row.children) {
        $tds[] = tableCellToHtml($tag, $cell, $allowRaw);
    }
    return html.element("tr", withNodeAttrs([], $row), $tds);
}

# capitalise upper-cases the first rune of a word, which is all the five kind
# names need: they are ASCII and one word each.
func capitalise(word as string) {
    if ($word == "") {
        return "";
    }
    return strings.upper($word[..1]) +
        $word[1..];
}

# alertLabel is what an admonition is called: the author's own title when the
# marker carried one, else the kind's name. The renderers that draw chrome in
# one language use this; `renderPdfDoc` takes a translation table instead, since
# the caller laying out a book knows what language it is in.
func alertLabel(n as Node) {
    if ($n.title != "") {
        return $n.title;
    }
    return capitalise($n.lang);
}

func tableNodeToHtml(n as Node, allowRaw as bool) {
    def head as list of html.Node init [];
    def body as list of html.Node init [];
    def headRows as int init headerRowCount($n);
    def r as int init 0;
    for (def row in $n.children) {
        if ($r < $headRows) {
            $head[] = tableRowToHtml("th", $row, $allowRaw);
        } else {
            $body[] = tableRowToHtml("td", $row, $allowRaw);
        }
        $r = $r + 1;
    }
    def parts as list of html.Node init [];
    # A table's `title` (an rST list-table / csv-table title) is its caption.
    if ($n.title != "") {
        def capText as list of html.Node init [html.text($n.title)];
        $parts[] = html.element("caption", [], $capText);
    }
    # A table declared without a header (`headerRows` "0") has no <thead>.
    if ($headRows > 0) {
        $parts[] = html.element("thead", [], $head);
    }
    $parts[] = html.element("tbody", [], $body);
    return html.element("table", withNodeAttrs([], $n), $parts);
}

# lineBlockToHtml renders a line block: each `line` a <div class="line"> (an
# empty one holds a <br> so it keeps its height), a nested `line_block` an
# indented block of its own.
func lineBlockToHtml(n as Node, allowRaw as bool) {
    def kids as list of html.Node init [];
    for (def c in $n.children) {
        if ($c.kind == "line_block") {
            $kids[] = lineBlockToHtml($c, $allowRaw);
        } else {
            def lineKids as list of html.Node init inlineNodesToHtml($c.children, $allowRaw);
            if (len($lineKids) == 0) {
                def noKids as list of html.Node init [];
                $lineKids[] = html.element("br", [], $noKids);
            }
            def lineAttrs as list of html.Attr init [html.attr("class", "line")];
            $kids[] = html.element("div", withNodeAttrs($lineAttrs, $c), $lineKids);
        }
    }
    def attrs as list of html.Attr init [html.attr("class", "line-block")];
    return html.element("div", withNodeAttrs($attrs, $n), $kids);
}

# noteToHtml renders a footnote or citation as an <aside>: the bracketed label
# first (a link back to the first reference when `backref` names one), then the
# note's content.
func noteToHtml(n as Node, allowRaw as bool) {
    def labelKids as list of html.Node init [];
    def backref as string init attr($n, "backref");
    if ($backref != "") {
        def backAttrs as list of html.Attr init [
            html.attr("role", "doc-backlink"),
            html.attr("href", safeHref("#" + $backref))
        ];
        def backText as list of html.Node init [html.text(noteLabel($n))];
        $labelKids[] = html.element("a", $backAttrs, $backText);
    } else {
        $labelKids[] = html.text(noteLabel($n));
    }
    def labelAttrs as list of html.Attr init [html.attr("class", "label")];
    def kids as list of html.Node init [html.element("span", $labelAttrs, $labelKids)];
    for (def c in mixedToHtml($n.children, $allowRaw)) {
        $kids[] = $c;
    }
    def attrs as list of html.Attr init [html.attr("class", $n.kind)];
    if ($n.kind == "footnote") {
        $attrs[] = html.attr("role", "doc-footnote");
    }
    return html.element("aside", withNodeAttrs($attrs, $n), $kids);
}

# figureToHtml renders a figure: its image, then a <figcaption> holding the
# caption paragraph and any legend.
func figureToHtml(n as Node, allowRaw as bool) {
    def kids as list of html.Node init [];
    def capKids as list of html.Node init [];
    for (def c in $n.children) {
        match ($c.kind) {
            when "caption" {
                $capKids[] = html.element(
                    "p",
                    withNodeAttrs([], $c),
                    inlineNodesToHtml($c.children, $allowRaw));
            }
            when "legend" {
                def legAttrs as list of html.Attr init [html.attr("class", "legend")];
                $capKids[] = html.element(
                    "div",
                    withNodeAttrs($legAttrs, $c),
                    mixedToHtml($c.children, $allowRaw));
            }
            else {
                if (isBlockKind($c.kind)) {
                    $kids[] = nodeToHtml($c, $allowRaw);
                } else {
                    $kids[] = inlineNodeToHtml($c, $allowRaw);
                }
            }
        }
    }
    if (len($capKids) > 0) {
        $kids[] = html.element("figcaption", [], $capKids);
    }
    return html.element("figure", withNodeAttrs([], $n), $kids);
}

# nodeToHtml renders one block Node to an html node.
func nodeToHtml(n as Node, allowRaw as bool) {
    match ($n.kind) {
        when "heading" {
            def tag as string init "h{$n.level}";
            return html.element($tag, mdAttrsToHtml($n), inlineNodesToHtml($n.children, $allowRaw));
        }
        when "paragraph" {
            return html.element(
                "p",
                withNodeAttrs([], $n),
                inlineNodesToHtml($n.children, $allowRaw));
        }
        when "code" {
            def codeKids as list of html.Node init [];
            $codeKids[] = html.text($n.text);
            # CommonMark: the language becomes `class="language-<lang>"`, which is
            # what web highlighters key on. No language -> a bare <code>.
            def codeAttrs as list of html.Attr init [];
            if ($n.lang != "") {
                $codeAttrs[] = html.attr("class", "language-" + $n.lang);
            }
            def pre as list of html.Node init [];
            $pre[] = html.element("code", $codeAttrs, $codeKids);
            return html.element("pre", withNodeAttrs([], $n), $pre);
        }
        when "list" { return listNodeToHtml($n, $allowRaw); }
        when "definition_list" {
            def dkids as list of html.Node init [];
            for (def c in $n.children) {
                if ($c.kind == "def_term") {
                    $dkids[] = html.element(
                        "dt",
                        withNodeAttrs([], $c),
                        inlineNodesToHtml($c.children, $allowRaw));
                } else {
                    $dkids[] = html.element(
                        "dd",
                        withNodeAttrs([], $c),
                        mixedToHtml($c.children, $allowRaw));
                }
            }
            return html.element("dl", withNodeAttrs([], $n), $dkids);
        }
        when "table" { return tableNodeToHtml($n, $allowRaw); }
        when "quote" {
            def kids as list of html.Node init [];
            for (def cb in $n.children) {
                $kids[] = nodeToHtml($cb, $allowRaw);
            }
            return html.element("blockquote", withNodeAttrs([], $n), $kids);
        }
        when "line_block" { return lineBlockToHtml($n, $allowRaw); }
        when "footnote", "citation" { return noteToHtml($n, $allowRaw); }
        when "figure" { return figureToHtml($n, $allowRaw); }
        when "topic" {
            def kids as list of html.Node init [];
            if ($n.title != "") {
                def titleAttrs as list of html.Attr init [html.attr("class", "topic-title")];
                def titleText as list of html.Node init [html.text($n.title)];
                $kids[] = html.element("p", $titleAttrs, $titleText);
            }
            for (def c in mixedToHtml($n.children, $allowRaw)) {
                $kids[] = $c;
            }
            def attrs as list of html.Attr init [html.attr("class", "topic")];
            return html.element("aside", withNodeAttrs($attrs, $n), $kids);
        }
        when "rubric" {
            def attrs as list of html.Attr init [html.attr("class", "rubric")];
            return html.element(
                "p",
                withNodeAttrs($attrs, $n),
                inlineNodesToHtml($n.children, $allowRaw));
        }
        when "container" {
            return html.element("div", withNodeAttrs([], $n), mixedToHtml($n.children, $allowRaw));
        }
        when "admonition" {
            # The class names are the conventional ones, so a stylesheet written
            # for another generator's callouts already matches. Nothing here
            # ships a stylesheet: the markup carries the kind, and what it looks
            # like is the page's business.
            def kids as list of html.Node init [];
            def labelText as list of html.Node init [html.text(alertLabel($n))];
            def titleAttrs as list of html.Attr init [html.attr("class", "admonition-title")];
            $kids[] = html.element("p", $titleAttrs, $labelText);
            for (def cb in $n.children) {
                $kids[] = nodeToHtml($cb, $allowRaw);
            }
            def attrs as list of html.Attr init [
                html.attr("class", "admonition admonition-" + $n.lang)
            ];
            return html.element("div", withNodeAttrs($attrs, $n), $kids);
        }
        when "thematic_break" {
            def noKids as list of html.Node init [];
            return html.element("hr", withNodeAttrs([], $n), $noKids);
        }
        when "html_block" {
            # SECURITY: raw HTML from the source is escaped by default, so
            # untrusted Markdown (a README, a comment) cannot inject a `<script>`
            # or an event-handler attribute (`<img onerror=...>`). A trusted
            # caller opts back in to verbatim passthrough with
            # `toHtmlWith(md, HtmlOptions{allowRawHtml: true})`.
            if ($allowRaw) {
                return html.raw($n.text);
            }
            return html.text($n.text);
        }
        when "page_break" {
            # A page break has no HTML representation; emit nothing.
            return html.raw("");
        }
        else { return inlineNodeToHtml($n, $allowRaw); }
    }
}

# inlineNodeToAnsi maps one inline Node to terminal-styled text (styling suppresses
# itself when stdout is not a TTY, so piped output is plain).
func inlineNodeToAnsi(n as Node) {
    match ($n.kind) {
        when "text", "html_inline" { return $n.text; }
        when "line_break" { return "\n"; }
        when "codespan" { return ansi.cyan($n.text); }
        when "strong" { return ansi.bold(inlineChildrenToAnsi($n.children)); }
        when "emphasis" { return ansi.italic(inlineChildrenToAnsi($n.children)); }
        when "strikethrough" { return ansi.strike(inlineChildrenToAnsi($n.children)); }
        when "highlight" { return ansi.reverse(inlineChildrenToAnsi($n.children)); }
        when "subscript" { return inlineChildrenToAnsi($n.children); }
        when "superscript" { return inlineChildrenToAnsi($n.children); }
        when "link" {
            return ansi.underline(inlineChildrenToAnsi($n.children)) + " (" + $n.url + ")";
        }
        when "image" { return ansi.dim("[image] ") + $n.text + " (" + $n.url + ")"; }
        when "footnote_ref", "citation_ref" { return ansi.dim(noteLabel($n)); }
        when "cite" { return ansi.italic(inlineChildrenToAnsi($n.children)); }
        when "span", "abbr" { return inlineChildrenToAnsi($n.children); }
        else { return text($n); }
    }
}

func inlineChildrenToAnsi(ns as list of Node) {
    def out as list of string init [];
    for (def c in $ns) {
        $out[] = inlineNodeToAnsi($c);
    }
    return strings.join($out, "");
}

# cellAnsi is a table cell's terminal text. A cell holding blocks (a grid table
# from `rst`) is flattened onto one line: the table draws one line per row.
func cellAnsi(cell as Node) {
    if (not hasBlockChild($cell)) {
        return inlineChildrenToAnsi($cell.children);
    }
    def parts as list of string init [];
    for (def b in blockify($cell.children)) {
        $parts[] = strings.replace(nodeToAnsi($b), "\n", " ");
    }
    return strings.join($parts, " ");
}

# cellVisWidthNode is the visible terminal width of a cell (styling stripped).
func cellVisWidthNode(cell as Node) {
    return displayWidth(ansi.strip(cellAnsi($cell)));
}

# colWidthsNode is the max visible width of each column over every row.
func colWidthsNode(table as Node) {
    def widths as list of int init [];
    if (len($table.children) == 0) {
        return $widths;
    }
    for (def cell in $table.children[0].children) {
        $widths[] = cellVisWidthNode($cell);
    }
    def r as int init 1;
    while ($r < len($table.children)) {
        def c as int init 0;
        for (def cell in $table.children[$r].children) {
            # Widen column c in place (columns past the header count are ignored),
            # keeping the accumulator in this frame rather than copying it per cell.
            def w as int init cellVisWidthNode($cell);
            if ($c < len($widths) and $w > $widths[$c]) {
                $widths[$c] = $w;
            }
            $c = $c + 1;
        }
        $r = $r + 1;
    }
    return $widths;
}

# ansiRowNode renders one ` | `-separated row padded to the column widths; a
# header row is bold.
func ansiRowNode(row as Node, widths as list of int, bold as bool) {
    def cells as list of string init [];
    def i as int init 0;
    while ($i < len($widths)) {
        def styled as string init "";
        def vis as int init 0;
        def align as string init "none";
        if ($i < len($row.children)) {
            def cell as Node init $row.children[$i];
            $styled = cellAnsi($cell);
            $vis = displayWidth(ansi.strip($styled));
            $align = $cell.align;
        }
        if ($bold) {
            $styled = ansi.bold($styled);
        }
        $cells[] = padCell($styled, $vis, $widths[$i], $align);
        $i = $i + 1;
    }
    return strings.join($cells, " | ");
}

func tableNodeToAnsi(n as Node) {
    # A table always carries its header row from `parse`; a hand-built table with no
    # rows renders as empty rather than indexing past the end.
    if (len($n.children) == 0) {
        return "";
    }
    # Spanning cells are laid out on a plain grid first: the terminal draws
    # fixed columns, so a span keeps its text in its first slot. The table is
    # passed on (borrowed), not copied into a local.
    if (hasSpans($n)) {
        return gridToAnsi(gridTable($n), $n.title);
    }
    return gridToAnsi($n, $n.title);
}

# gridToAnsi draws a span-free table as text rows under a bold title.
func gridToAnsi(t as Node, title as string) {
    def headRows as int init headerRowCount($t);
    def widths as list of int init colWidthsNode($t);
    def rows as list of string init [];
    if ($title != "") {
        $rows[] = ansi.bold($title);
    }
    def r as int init 0;
    while ($r < len($t.children)) {
        $rows[] = ansiRowNode($t.children[$r], $widths, $r < $headRows);
        # The rule goes under the last header row (none for a headerless table).
        if ($r == $headRows - 1) {
            $rows[] = ansiDivider($widths);
        }
        $r = $r + 1;
    }
    return strings.join($rows, "\n");
}

# inlineItemToAnsi renders the text of a list item that holds only inline
# content and nested lists: the inline parts on the marker's line, each nested
# list on the lines under it, one level deeper.
func inlineItemToAnsi(item as Node, depth as int) {
    def inlineParts as list of string init [];
    def nested as string init "";
    for (def c in $item.children) {
        if ($c.kind == "list") {
            $nested = "\n" + listNodeToAnsi($c, $depth + 1);
        } else {
            $inlineParts[] = inlineNodeToAnsi($c);
        }
    }
    return strings.join($inlineParts, "") + $nested;
}

# listNodeToAnsi renders a `list` node at nesting `depth` (2 spaces of indent per
# level), recursing into each item's nested sub-list under it. Rendered items
# collect and join once: growing `out` with `+` per item is O(N^2) over a long list.
func listNodeToAnsi(n as Node, depth as int) {
    def out as list of string init [];
    def idx as int init smallIntAttr($n, "start", 1);
    def et as string init listEnumType($n);
    def first as bool init true;
    def pad as string init strings.repeat("  ", $depth + 1);
    for (def item in $n.children) {
        if (not $first) {
            $out[] = "\n";
        }
        $first = false;
        def marker as string init "- ";
        if ($n.ordered) {
            $marker = enumMarker($et, $idx) + ". ";
        }
        # GFM task list item: show the checkbox after the bullet ("- [x] done").
        if (attr($item, "task") == "true") {
            if (attr($item, "checked") == "true") {
                $marker = $marker + "[x] ";
            } else {
                $marker = $marker + "[ ] ";
            }
        }
        if (hasNonListBlock($item)) {
            $out[] = blockItemToAnsi($item, $pad, $marker, $depth);
        } else {
            $out[] = $pad + $marker + inlineItemToAnsi($item, $depth);
        }
        $idx = $idx + 1;
    }
    return strings.join($out, "");
}

# hasNonListBlock reports whether an item holds a block other than a nested
# list (a loose item's paragraphs, a code block, a quote), which takes the block
# layout below instead of the inline one.
func hasNonListBlock(n as Node) {
    for (def c in $n.children) {
        if (isBlockKind($c.kind) and $c.kind != "list") {
            return true;
        }
    }
    return false;
}

# blockItemToAnsi lays out a list item that holds blocks: the first block on the
# marker's line, every later line indented to the item's text column, and a
# nested list at its own depth.
func blockItemToAnsi(item as Node, pad as string, marker as string, depth as int) {
    def col as string init strings.repeat(" ", len($pad) + len($marker));
    def parts as list of string init [];
    for (def b in blockify($item.children)) {
        if ($b.kind == "list") {
            $parts[] = listNodeToAnsi($b, $depth + 1);
        } else {
            $parts[] = indentLines(nodeToAnsi($b), $col);
        }
    }
    def body as string init strings.join($parts, "\n");
    # The first line takes the marker in place of the indent. A nested list
    # that opens the item starts on the line below the marker.
    if (strings.startsWith($body, $col)) {
        return $pad + $marker + $body[len($col)..];
    }
    return $pad + strings.trimRight($marker) + "\n" + $body;
}

# blocksToAnsi renders mixed content as blocks one per line, for the containers
# that hold them (a description, a footnote, a topic).
func blocksToAnsi(ns as list of Node) {
    def parts as list of string init [];
    for (def b in blockify($ns)) {
        $parts[] = nodeToAnsi($b);
    }
    return strings.join($parts, "\n");
}

# lineBlockToAnsi renders a line block, a nested one indented four columns.
func lineBlockToAnsi(n as Node) {
    def lines as list of string init [];
    for (def c in $n.children) {
        if ($c.kind == "line_block") {
            $lines[] = indentLines(lineBlockToAnsi($c), "    ");
        } else {
            $lines[] = inlineChildrenToAnsi($c.children);
        }
    }
    return strings.join($lines, "\n");
}

# figurePartToAnsi renders one child of a `figure`: the caption in italics, the
# legend's blocks, and the image or any other child as the block or inline node
# it is.
func figurePartToAnsi(c as Node) {
    match ($c.kind) {
        when "caption" { return ansi.italic(inlineChildrenToAnsi($c.children)); }
        when "legend" { return blocksToAnsi($c.children); }
    }
    if (isBlockKind($c.kind)) {
        return nodeToAnsi($c);
    }
    return inlineNodeToAnsi($c);
}

# nodeToAnsi renders one block Node to terminal text.
func nodeToAnsi(n as Node) {
    match ($n.kind) {
        when "table" { return tableNodeToAnsi($n); }
        when "heading" { return ansi.bold(inlineChildrenToAnsi($n.children)); }
        when "code" { return ansi.dim(indentLines($n.text, "    ")); }
        when "list" { return listNodeToAnsi($n, 0); }
        when "definition_list" {
            def parts as list of string init [];
            for (def c in $n.children) {
                if ($c.kind == "def_term") {
                    $parts[] = ansi.bold(inlineChildrenToAnsi($c.children));
                } elseif (hasBlockChild($c)) {
                    $parts[] = indentLines(blocksToAnsi($c.children), "    ");
                } else {
                    $parts[] = "    " + inlineChildrenToAnsi($c.children);
                }
            }
            return strings.join($parts, "\n");
        }
        when "quote" {
            def parts as list of string init [];
            for (def cb in $n.children) {
                $parts[] = nodeToAnsi($cb);
            }
            return indentLines(strings.join($parts, "\n"), ansi.dim("> "));
        }
        when "admonition" {
            # A terminal has no panel to draw, so the label carries the weight:
            # bold, above the quotation it opened.
            def parts as list of string init [ansi.bold(alertLabel($n))];
            for (def cb in $n.children) {
                $parts[] = nodeToAnsi($cb);
            }
            return indentLines(strings.join($parts, "\n"), ansi.dim("> "));
        }
        when "paragraph" { return inlineChildrenToAnsi($n.children); }
        when "thematic_break" { return ansi.dim(strings.repeat("-", 40)); }
        when "html_block" {
            # Raw HTML has no terminal representation; drop it.
            return "";
        }
        when "page_break" { return ""; }
        when "line_block" { return lineBlockToAnsi($n); }
        when "footnote", "citation" {
            # The label hangs in front of the note, its text indented past it.
            def label as string init noteLabel($n) + " ";
            def body as string init indentLines(
                blocksToAnsi($n.children),
                strings.repeat(" ", len($label)));
            return ansi.dim($label) + $body[len($label)..];
        }
        when "figure" {
            def parts as list of string init [];
            for (def c in $n.children) {
                $parts[] = figurePartToAnsi($c);
            }
            return strings.join($parts, "\n");
        }
        when "topic" {
            def body as string init blocksToAnsi($n.children);
            if ($n.title == "") {
                return $body;
            }
            return ansi.bold($n.title) + "\n" + $body;
        }
        when "rubric" { return ansi.bold(inlineChildrenToAnsi($n.children)); }
        when "container" {
            def parts as list of string init [];
            for (def b in blockify($n.children)) {
                $parts[] = nodeToAnsi($b);
            }
            return strings.join($parts, "\n\n");
        }
        else { return inlineChildrenToAnsi($n.children); }
    }
}

# TERMINAL_UNSAFE matches the characters a terminal acts on instead of printing:
# C0 controls other than tab and newline, DEL, and C1 (U+0080-U+009F, which
# includes the 8-bit CSI / OSC introducers).
def const TERMINAL_UNSAFE as string init '[\x00-\x08\x0b-\x1f\x7f-\x{9f}]';

# terminalSafe returns n with every string field of it and its descendants
# cleaned of TERMINAL_UNSAFE characters (each becomes U+FFFD), so source text
# can't emit its own escape sequences through the ANSI renderer.
func terminalSafe(n as Node) {
    $n.text = terminalText($n.text);
    $n.url = terminalText($n.url);
    $n.title = terminalText($n.title);
    $n.lang = terminalText($n.lang);
    for (def k in maps.keys($n.attrs)) {
        $n.attrs[$k] = terminalText($n.attrs[$k]);
    }
    for (def i in 0..len($n.children)) {
        $n.children[$i] = terminalSafe($n.children[$i]);
    }
    return $n;
}

# hasTerminalUnsafe reports whether any string field of n or its descendants
# holds a TERMINAL_UNSAFE character: a read-only walk, so a clean tree is not
# copied.
func hasTerminalUnsafe(n as Node) {
    if (regex.matches(TERMINAL_UNSAFE, $n.text + $n.url + $n.title + $n.lang)) {
        return true;
    }
    if (len($n.attrs) > 0) {
        if (regex.matches(TERMINAL_UNSAFE, strings.join(maps.values($n.attrs), ""))) {
            return true;
        }
    }
    for (def i in 0..len($n.children)) {
        if (hasTerminalUnsafe($n.children[$i])) {
            return true;
        }
    }
    return false;
}

# terminalText replaces each TERMINAL_UNSAFE character of s with U+FFFD.
func terminalText(s as string) {
    if ($s == "") {
        return $s;
    }
    return regex.replace(TERMINAL_UNSAFE, $s, "\uFFFD");
}

# htmlBlocksToString renders a block list to an HTML string. `allowRaw` controls
# whether a raw `html_block` is emitted verbatim (trusted) or escaped (the safe
# default) - see nodeToHtml's html_block arm.
func htmlBlocksToString(blocks as list of Node, allowRaw as bool, xhtml as bool) {
    def nodes as list of html.Node init [];
    for (def c in $blocks) {
        $nodes[] = nodeToHtml($c, $allowRaw);
    }
    if ($xhtml) {
        return html.renderAllXhtml($nodes);
    }
    return html.renderAll($nodes);
}

/**
 * Render a document tree (a `parse` result, or a hand-built one) to a string.
 * `format` is `"html"` (block elements concatenated, no indentation), `"xhtml"`
 * (the same, but well-formed XML for an EPUB / XHTML content document: void
 * elements self-close and boolean attributes expand), or `"ansi"` (styled
 * terminal text, blocks separated by a blank line; control characters in the
 * source text other than tab and newline become U+FFFD). A `"document"` node renders
 * its children; any other node renders as a single block. An unknown format is a
 * catchable `"doctree"` error.
 * @param doc {Node} the document (or block) node to render
 * @param format {string} "html", "xhtml", or "ansi"
 * @return {string} the rendered document
 */
export func render(doc as Node, format as string) {
    if ($doc.kind == "document") {
        return renderBlocks($doc.children, $format);
    }
    return renderBlocks([$doc], $format);
}

# renderBlocks renders top-level blocks to a format. A separate function so a
# document's children are passed (borrowed), not copied into a local.
func renderBlocks(blocks as list of Node, format as string) {
    if ($format == "html") {
        return htmlBlocksToString($blocks, false, false);
    }
    if ($format == "xhtml") {
        return htmlBlocksToString($blocks, false, true);
    }
    if ($format == "ansi") {
        def parts as list of string init [];
        def first as bool init true;
        for (def c in $blocks) {
            if (not $first) {
                $parts[] = "\n\n";
            }
            $first = false;
            if (hasTerminalUnsafe($c)) {
                $parts[] = nodeToAnsi(terminalSafe($c));
            } else {
                $parts[] = nodeToAnsi($c);
            }
        }
        return strings.join($parts, "");
    }
    treeFail("unknown render format \"" + $format + "\" (want \"html\", \"xhtml\", or \"ansi\")");
}

/**
 * Options for `renderHtml`.
 * @field allowRawHtml {bool} pass `html_block` nodes through verbatim (trusted
 *   input only). Default (zero value) is false: raw HTML is escaped.
 * @field xhtml {bool} write well-formed XHTML (void elements self-close, boolean
 *   attributes expand) instead of HTML
 */
export def struct HtmlOptions {
    allowRawHtml as bool,
    xhtml as bool
};

/**
 * Render a document (or a single block) to HTML with explicit options. `render`
 * with `"html"` / `"xhtml"` is this with the default options.
 * @param doc {Node} the document (or block) node to render
 * @param opts {HtmlOptions} the render options
 * @return {string} the rendered HTML
 */
export func renderHtml(doc as Node, opts as HtmlOptions) {
    def blocks as list of Node init [$doc];
    if ($doc.kind == "document") {
        $blocks = $doc.children;
    }
    return htmlBlocksToString($blocks, $opts.allowRawHtml, $opts.xhtml);
}

# --- PDF rendering, through pdf (exported) -----------------------

# --- options -------------------------------------------------------

/**
 * A fill colour (0-255 RGB) that may be off. `on: false` means "no fill" (draw
 * nothing); an `on: true` fill paints a background behind a heading or a table
 * header row. Build one with `gray` / `rgb`, or `noFill` for none.
 * @field on {bool} whether the fill is applied at all
 * @field r {int} red 0-255
 * @field g {int} green 0-255
 * @field b {int} blue 0-255
 */
export def struct Fill {
    on as bool,
    r as int,
    g as int,
    b as int
};

/**
 * The style for a heading level - currently just a `background` fill painted behind
 * the heading text (a shaded bar). A struct so more style knobs can be added later.
 * @field background {Fill} the background fill (off for none)
 */
export def struct HeadingStyle {
    background as Fill
};

/**
 * No fill (a transparent background).
 * @return {Fill} an off fill
 */
export func noFill() {
    return Fill{on: false, r: 0, g: 0, b: 0};
}

/**
 * A grey fill of the given 0-255 level (0 black, 255 white).
 * @param level {int} the grey level, 0-255
 * @return {Fill} the fill
 */
export func gray(level as int) {
    return Fill{on: true, r: $level, g: $level, b: $level};
}

/**
 * An RGB fill (each component 0-255).
 * @param r {int} red 0-255
 * @param g {int} green 0-255
 * @param b {int} blue 0-255
 * @return {Fill} the fill
 */
export func rgb(r as int, g as int, b as int) {
    return Fill{on: true, r: $r, g: $g, b: $b};
}

/**
 * A heading style with the given background fill.
 * @param background {Fill} the background fill (off for none)
 * @return {HeadingStyle} the style
 */
export func headingStyle(background as Fill) {
    return HeadingStyle{background: $background};
}

/**
 * Page geometry and fonts for a render. `margin` is applied to all four sides;
 * `bodyFont` / `boldFont` / `italicFont` / `monoFont` are standard-14 font names for
 * body text and its inline styles, `headingFont` for headings. `bodySize` is the
 * body point size; heading sizes derive from the heading level.
 * @field pageWidth {int} page width in points (Letter 612, A4 595)
 * @field pageHeight {int} page height in points (Letter 792, A4 842)
 * @field margin {int} margin on every side, in points
 * @field bodyFont {string} standard-14 font for body text
 * @field boldFont {string} standard-14 font for `**strong**`
 * @field italicFont {string} standard-14 font for `*emphasis*`
 * @field monoFont {string} standard-14 font for inline / block code
 * @field headingFont {string} standard-14 font for headings
 * @field bodySize {int} body text point size
 * @field tablePad {int} padding inside a table cell, in points (table density)
 * @field tableHeaderFill {Fill} background fill behind a table's header row (off for none)
 * @field headingStyles {list of HeadingStyle} per-level heading style; index 0 is h1,
 *   1 is h2, and so on (missing / off entries render with no background)
 * @field title {string} PDF document Title metadata ("" = unset)
 * @field author {string} PDF document Author metadata ("" = unset)
 * @field subject {string} PDF document Subject metadata ("" = unset)
 * @field keywords {string} PDF document Keywords metadata ("" = unset)
 * @field bookmarkLevel {int} bookmark headings up to this level (0 = none; 2 = h1 + h2)
 * @field unencodable {string} substitute for a character the standard-14 fonts cannot
 *   encode ("?" default; "" drops it)
 * @field codeFill {Fill} background fill behind a code block (off for none)
 * @field codeBorder {Fill} border stroke around a code block (off for none)
 * @field quoteFill {Fill} background fill behind a blockquote (off for none)
 * @field quoteRule {Fill} colour of the vertical bar down a blockquote's left edge (off for none)
 * @field creator {string} PDF document Creator metadata ("" = unset)
 * @field producer {string} PDF document Producer metadata ("" = keep the pdf default)
 * @field admonitionLabels {map of string to string} what to call each admonition
 *   kind - `{"note": "Hinweis", ...}` - for a book laid out in a language other
 *   than English. A kind that is missing is called by its own name; a marker
 *   that carried a title is called by that, since the title is the author's own
 *   words rather than the renderer's.
 * @field images {map of string to pdf.Image} pictures to draw, keyed by the image URL
 *   exactly as written in the source (empty = every `![alt](url)` stays `[alt]` text);
 *   load each with `pdf.loadImage`, each with a unique resource name
 * @field imageDpi {int} pixels-to-points scale for a drawn image (default 96)
 */
export def struct PdfOptions {
    pageWidth as int,
    pageHeight as int,
    margin as int,
    bodyFont as string,
    boldFont as string,
    italicFont as string,
    monoFont as string,
    headingFont as string,
    bodySize as int,
    tablePad as int,
    tableHeaderFill as Fill,
    headingStyles as list of HeadingStyle,
    title as string,
    author as string,
    subject as string,
    keywords as string,
    bookmarkLevel as int,
    unencodable as string,
    codeFill as Fill,
    codeBorder as Fill,
    quoteFill as Fill,
    quoteRule as Fill,
    creator as string,
    producer as string,
    admonitionLabels as map of string to string,
    images as map of string to pdf.Image,
    imageDpi as int
};

/**
 * The default options: US Letter, 54-point margins, the Helvetica family for body /
 * bold / italic / headings, Courier for code, 11-point body. Copy and tweak fields
 * (value semantics) to customise.
 *
 * `images` is empty by default, so `![alt](url)` renders as its `[alt]` text (as
 * before). To draw pictures, load each with `pdf.loadImage(name, bytes)` and put
 * it in `images` keyed by the image URL exactly as it appears in the Markdown
 * source (`markdown` never touches the filesystem - the caller supplies the
 * bytes); each image `name` must be unique. `imageDpi` (default 96) converts a
 * picture's pixel size to points; a picture wider than the text column shrinks to
 * it, keeping aspect ratio. Only a paragraph that is a single image is drawn; an
 * image inline in a sentence keeps its `[alt]` text.
 * @return {PdfOptions} the default options
 */
export func pdfDefaults() {
    return PdfOptions{
        pageWidth: 612,
        pageHeight: 792,
        margin: 54,
        bodyFont: "Helvetica",
        boldFont: "Helvetica-Bold",
        italicFont: "Helvetica-Oblique",
        monoFont: "Courier",
        headingFont: "Helvetica-Bold",
        bodySize: 11,
        tablePad: 4,
        tableHeaderFill: noFill(),
        headingStyles: [],
        title: "",
        author: "",
        subject: "",
        keywords: "",
        bookmarkLevel: 0,
        unencodable: "?",
        codeFill: noFill(),
        codeBorder: noFill(),
        quoteFill: noFill(),
        quoteRule: noFill(),
        creator: "",
        producer: "",
        admonitionLabels: {},
        images: {},
        imageDpi: 96
    };
}

# mdSanitize replaces any character the PDF standard-14 fonts cannot encode
# (outside WinAnsi) with the configured substitute, so one out-of-range glyph
# (a smart arrow, an emoji) can never abort a whole render. A clean string is
# returned unchanged (the common case). Applied at every point text enters the
# PDF layout - so measurement and drawing both see WinAnsi-safe text.
func mdSanitize(opts as PdfOptions, s as string) {
    return pdf.toWinAnsi($s, $opts.unencodable);
}

# --- internal layout state -----------------------------------------

# One styled word: the word text plus the standard-14 font it renders in. All words
# on a line share a point size, so widths add up without per-word size bookkeeping.
def struct IWord {
    text as string,
    font as string,
    width as float
};

# The running layout: the page being filled, the pen `y` (the top of the next
# line, decreasing down the page), the current left `x` / content `width`
# (widened / narrowed by lists and quotes). The finished document is NOT
# threaded through here - `done` collects the pages a call finalised (flushed)
# so the caller can fold them into the document, instead of deep-copying the
# whole growing `pdf.Document` at every render-function boundary. `pageNo` is
# the count of finished pages (the 0-based index of the page being built, what
# a bookmark points at); `bks` collects any bookmarks the subtree produced.
def struct Layout {
    page as pdf.Page,
    y as int,
    x as int,
    width as int,
    opts as PdfOptions,
    pageNo as int,
    done as list of pdf.Page,
    bks as list of pdf.OutlineEntry,
    # URL -> a byte-free copy of the registered image (name + pixel dims only).
    # The real bytes live once in the document's `images`; carrying only this
    # light copy keeps the per-block Layout deep-copy off the image bytes.
    placements as map of string to pdf.Image
};

# roundPt rounds a measured (float) width to whole points for integer placement.
func roundPt(v as float) {
    return math.round($v);
}

# lineH is the line advance for a point size (1.4x, the usual single-spaced ratio).
func lineH(size as int) {
    return ($size * 7) // 5;
}

# blockGap is the vertical space left after each block.
func blockGap(opts as PdfOptions) {
    return ($opts.bodySize * 3) // 5;
}

# usableHeight is the tallest a block can be on a page: the full height less the
# top and bottom margins. An image is capped to this so it always fits one page.
func usableHeight(opts as PdfOptions) {
    return $opts.pageHeight - 2 * $opts.margin;
}

# pxToPoints converts a pixel length to whole points at the given dpi (72 pt per
# inch). A non-positive dpi falls back to 96 so a caller that left imageDpi unset
# on a hand-built PdfOptions never divides by zero.
func pxToPoints(px as int, dpi as int) {
    def d as int init $dpi;
    if ($d <= 0) {
        $d = 96;
    }
    return $px * 72 // $d;
}

# headingSize maps a heading level (1-6) to a point size.
func headingSize(level as int, opts as PdfOptions) {
    match ($level) {
        when 1 { return 22; }
        when 2 { return 17; }
        when 3 { return 14; }
        when 4 { return 13; }
        when 5 { return 12; }
        else { return $opts.bodySize; }
    }
}

# buildDoc creates the document carrying the option's metadata (each unset "" is
# left out), for renderPdfDoc to fold the laid-out pages into.
func buildDoc(opts as PdfOptions) {
    def doc as pdf.Document init pdf.document();
    if ($opts.title != "") {
        $doc = pdf.info($doc, "Title", $opts.title);
    }
    if ($opts.author != "") {
        $doc = pdf.info($doc, "Author", $opts.author);
    }
    if ($opts.subject != "") {
        $doc = pdf.info($doc, "Subject", $opts.subject);
    }
    if ($opts.keywords != "") {
        $doc = pdf.info($doc, "Keywords", $opts.keywords);
    }
    if ($opts.creator != "") {
        $doc = pdf.info($doc, "Creator", $opts.creator);
    }
    if ($opts.producer != "") {
        $doc = pdf.info($doc, "Producer", $opts.producer);
    }
    return $doc;
}

func newLayout(opts as PdfOptions) {
    return Layout{
        page: pdf.page($opts.pageWidth, $opts.pageHeight),
        y: $opts.pageHeight - $opts.margin,
        x: $opts.margin,
        width: $opts.pageWidth - 2 * $opts.margin,
        opts: $opts,
        pageNo: 0,
        done: [],
        bks: [],
        placements: {}
    };
}

# flushPage finalises the current page (handing it up in `done` for the caller
# to fold into the document) and starts a fresh one, preserving the current
# indent (x / width) so a block split across a page break keeps its column.
func flushPage(state as Layout) {
    $state.done[] = $state.page;
    $state.page = pdf.page($state.opts.pageWidth, $state.opts.pageHeight);
    $state.y = $state.opts.pageHeight - $state.opts.margin;
    $state.pageNo = $state.pageNo + 1;
    return $state;
}

# needsBreak reports whether `needed` points no longer fit above the bottom margin
# on a page that already holds content (an over-tall line on a fresh page is
# placed anyway, so it cannot loop). Callers then `flushPage`; a bool test keeps
# the Layout from being copied on every call that does not break.
func needsBreak(state as Layout, needed as int) {
    def top as int init $state.opts.pageHeight - $state.opts.margin;
    return $state.y - $needed < $state.opts.margin and $state.y < $top;
}

# --- inline layout (styled word wrapping) --------------------------

# fontForInline picks the font for one inline node's kind (text / link / image ->
# body, strong -> bold, emphasis -> italic, codespan -> mono).
func fontForInline(node as Node, opts as PdfOptions) {
    match (typeOf($node)) {
        when "strong" { return $opts.boldFont; }
        when "emphasis", "cite" { return $opts.italicFont; }
        when "codespan" { return $opts.monoFont; }
        else { return $opts.bodyFont; }
    }
}

# inlineText is one inline node's display text; an image renders its alt in brackets.
func inlineText(node as Node) {
    if (typeOf($node) == "image") {
        def alt as string init text($node);
        if (len($alt) == 0) {
            return "[image]";
        }
        return "[" + $alt + "]";
    }
    if (typeOf($node) == "footnote_ref" or typeOf($node) == "citation_ref") {
        return noteLabel($node);
    }
    return text($node);
}

# inlineWords flattens inline nodes into styled words (split on spaces), each tagged
# with the font its run renders in.
func inlineWords(nodes as list of Node, opts as PdfOptions) {
    def out as list of IWord init [];
    for (def n in $nodes) {
        # A hard line break is a sentinel word packLines turns into a line end.
        if (typeOf($n) == "line_break") {
            $out[] = IWord{text: "\n", font: $opts.bodyFont, width: 0.0};
            continue;
        }
        def f as string init fontForInline($n, $opts);
        def t as string init mdSanitize($opts, inlineText($n));
        for (def w in strings.split($t, " ")) {
            if (len($w) > 0) {
                $out[] = IWord{text: $w, font: $f, width: 0.0};
            }
        }
    }
    return $out;
}

# packLines greedily packs styled words into lines that fit `maxWidth` at `size`.
func packLines(words as list of IWord, size as int, maxWidth as int, opts as PdfOptions) {
    def lines as list of list of IWord init [];
    def cur as list of IWord init [];
    def curW as float init 0.0;
    def sp as float init pdf.measureText($opts.bodyFont, $size, " ");
    for (def w in $words) {
        if ($w.text == "\n") {
            $lines[] = $cur;
            def empty as list of IWord init [];
            $cur = $empty;
            $curW = 0.0;
            continue;
        }
        def ww as float init pdf.measureText($w.font, $size, $w.text);
        # Cache the measured width on the word so placeInlineLines does not
        # re-measure it (every word was measured twice - wrap, then place).
        $w.width = $ww;
        def add as float init $ww;
        if (len($cur) > 0) {
            $add = $ww + $sp;
        }
        if (len($cur) > 0 and $curW + $add > $maxWidth) {
            $lines[] = $cur;
            def fresh as list of IWord init [$w];
            $cur = $fresh;
            $curW = $ww;
        } else {
            $cur[] = $w;
            $curW = $curW + $add;
        }
    }
    if (len($cur) > 0) {
        $lines[] = $cur;
    }
    return $lines;
}

# Packed carries packLines' output: a struct parameter is borrowed, a bare
# `list of list` would be copied on every call.
def struct Packed {
    lines as list of list of IWord
};

# placeInlineLines draws already-packed styled lines at the current pen, advancing y
# and paginating per line.
func placeInlineLines(state as Layout, lines as Packed, size as int) {
    def lh as int init lineH($size);
    def sp as int init roundPt(pdf.measureText($state.opts.bodyFont, $size, " "));
    for (def line in $lines.lines) {
        if (needsBreak($state, $lh)) {
            $state = flushPage($state);
        }
        def baseline as int init $state.y - $size;
        def cx as int init $state.x;
        def first as bool init true;
        for (def w in $line) {
            if (not $first) {
                $cx = $cx + $sp;
            }
            $first = false;
            $state.page = pdf.text($state.page, $cx, $baseline, $w.font, $size, $w.text);
            $cx = $cx + roundPt($w.width);
        }
        $state.y = $state.y - $lh;
    }
    return $state;
}

# placePlainLines draws single-font wrapped lines (headings, code) at the current pen.
func placePlainLines(
    state as Layout,
    lines as list of string,
    font as string,
    size as int,
    xOffset as int) {
    def lh as int init lineH($size);
    for (def line in $lines) {
        if (needsBreak($state, $lh)) {
            $state = flushPage($state);
        }
        def baseline as int init $state.y - $size;
        $state.page = pdf.text($state.page, $state.x + $xOffset, $baseline, $font, $size, $line);
        $state.y = $state.y - $lh;
    }
    return $state;
}

# --- block renderers ------------------------------------------------

# headingBg returns the background fill configured for a heading level (off if the
# level has no configured style).
func headingBg(opts as PdfOptions, level as int) {
    if ($level - 1 < len($opts.headingStyles)) {
        return $opts.headingStyles[$level - 1].background;
    }
    return noFill();
}

func renderHeading(state as Layout, node as Node) {
    def lvl as int init level($node);
    # A level-1 heading starts a fresh page (unless the page is still empty), so each
    # top-level section begins on its own page.
    if ($lvl == 1 and $state.y < $state.opts.pageHeight - $state.opts.margin) {
        $state = flushPage($state);
    }
    def size as int init headingSize($lvl, $state.opts);
    def raw as string init text($node);
    def htext as string init mdSanitize($state.opts, $raw);
    def lines as list of string init pdf.wrapText(
        $state.opts.headingFont,
        $size,
        $htext,
        $state.width);
    def blockH as int init len($lines) * lineH($size);
    if (needsBreak($state, $blockH)) {
        $state = flushPage($state);
    }
    # Record a bookmark for this heading when its level is within the option's
    # bookmark depth. The page it lands on is the one being built (`pageNo`, its
    # 0-based index once added), and `y` is already in PDF coordinates.
    #
    # The bookmark takes the *unsanitised* heading. A bookmark is read by the
    # viewer rather than drawn in a font, so `pdf` writes it as UTF-16BE and it
    # carries any script; the WinAnsi fold above exists because the standard-14
    # fonts cannot draw those glyphs, which is a constraint on the page and not
    # on the sidebar. A Cyrillic heading now reads as itself in the outline even
    # where the page beneath it shows the substitute.
    if ($state.opts.bookmarkLevel > 0 and $lvl <= $state.opts.bookmarkLevel) {
        $state.bks[] = pdf.OutlineEntry{title: $raw, page: $state.pageNo, y: $state.y, level: $lvl};
    }
    # Optional shaded background bar behind the heading (drawn before the text; the
    # colour is reset to black so the text and later content stay black).
    def bg as Fill init headingBg($state.opts, $lvl);
    if ($bg.on) {
        $state.page = fillRect(
            $state.page,
            $state.x - 3,
            $state.y - $blockH + 2,
            $state.width + 6,
            $blockH + 2,
            $bg);
    }
    # Read what the call needs from $state first: an argument after $state that
    # reads it again keeps $state from moving into the call, so it would be copied.
    def font as string init $state.opts.headingFont;
    $state = placePlainLines($state, $lines, $font, $size, 0);
    return $state;
}

func renderParagraph(state as Layout, node as Node) {
    def size as int init $state.opts.bodySize;
    def words as list of IWord init inlineWords(children($node), $state.opts);
    def lines as Packed init Packed{lines: packLines($words, $size, $state.width, $state.opts)};
    $state = placeInlineLines($state, $lines, $size);
    return $state;
}

# renderDefList lays out a definition list: each term in bold, each definition
# flowed in an indented column beneath it.
func renderDefList(state as Layout, node as Node) {
    def indent as int init 18;
    for (def c in children($node)) {
        def size as int init $state.opts.bodySize;
        if (typeOf($c) == "def_term") {
            def words as list of IWord init inlineWords(children($c), $state.opts);
            for (def k in 0..len($words)) {
                $words[$k].font = $state.opts.boldFont;
            }
            def lines as Packed init Packed{
                lines: packLines($words, $size, $state.width, $state.opts)
            };
            $state = placeInlineLines($state, $lines, $size);
        } elseif (hasBlockChild($c)) {
            # A description holding blocks lays them out in the indented column.
            $state = renderBlocksIn($state, children($c), $indent, 0);
        } else {
            def savedX as int init $state.x;
            def savedW as int init $state.width;
            $state.x = $savedX + $indent;
            $state.width = $savedW - $indent;
            def words as list of IWord init inlineWords(children($c), $state.opts);
            def lines as Packed init Packed{
                lines: packLines($words, $size, $state.width, $state.opts)
            };
            $state = placeInlineLines($state, $lines, $size);
            $state.x = $savedX;
            $state.width = $savedW;
        }
    }
    return $state;
}

func renderCode(state as Layout, node as Node) {
    def raw as list of string init strings.split(text($node), "\n");
    # Fold each code line to the code column so a long line (a full command, a
    # URL) wraps onto the page instead of being clipped at the right margin.
    # Sanitize first so the fold measures exactly the glyphs that will be drawn.
    def colW as int init $state.width - 6;
    def lines as list of string init [];
    for (def l in $raw) {
        def s as string init mdSanitize($state.opts, $l);
        for (def piece in pdf.foldLine($state.opts.monoFont, $state.opts.bodySize, $s, $colW)) {
            $lines[] = $piece;
        }
    }
    # Optional background fill / border behind the whole block (drawn before the
    # text). Both default to noFill(), so the default output is unchanged. The
    # block is kept together on one page so the panel frames all of it.
    def fill as Fill init $state.opts.codeFill;
    def border as Fill init $state.opts.codeBorder;
    if ($fill.on or $border.on) {
        def blockH as int init len($lines) * lineH($state.opts.bodySize) + 8;
        if (needsBreak($state, $blockH)) {
            $state = flushPage($state);
        }
        def top as int init $state.y + 4;
        if ($fill.on) {
            $state.page = fillRect(
                $state.page,
                $state.x - 3,
                $top - $blockH,
                $state.width + 6,
                $blockH,
                $fill);
        }
        if ($border.on) {
            $state.page = strokeRect(
                $state.page,
                $state.x - 3,
                $top - $blockH,
                $state.width + 6,
                $blockH,
                $border);
        }
    }
    def mono as string init $state.opts.monoFont;
    def size as int init $state.opts.bodySize;
    $state = placePlainLines($state, $lines, $mono, $size, 6);
    return $state;
}

func renderList(state as Layout, node as Node, depth as int) {
    def ordered as bool init attr($node, "ordered") == "true";
    def markerW as int init 18;
    def idx as int init smallIntAttr($node, "start", 1);
    def et as string init listEnumType($node);
    for (def item in children($node)) {
        def marker as string init "-";
        def mw as int init $markerW;
        if ($ordered) {
            $marker = enumMarker($et, $idx) + ".";
            # A marker wider than the column ("100.", "xxxviii.") widens it so it
            # never runs into the text; every narrower one keeps the 18pt column.
            def need as int init roundPt(pdf.measureText(
                $state.opts.bodyFont,
                $state.opts.bodySize,
                $marker)) + 2;
            if ($need > $mw) {
                $mw = $need;
            }
        }
        # GFM task list item: the checkbox stands in for the bullet; widen the
        # marker column so "[x]" fits.
        if (attr($item, "task") == "true") {
            $marker = "[ ]";
            if (attr($item, "checked") == "true") {
                $marker = "[x]";
            }
            $mw = 30;
        }
        if (hasNonListBlock($item)) {
            # An item holding blocks: the marker on the first line, the blocks
            # laid out in the column past it.
            if (needsBreak($state, lineH($state.opts.bodySize))) {
                $state = flushPage($state);
            }
            $state.page = pdf.text(
                $state.page,
                $state.x,
                $state.y - $state.opts.bodySize,
                $state.opts.bodyFont,
                $state.opts.bodySize,
                $marker);
            $state = renderBlocksIn($state, children($item), $mw, $depth);
            $idx = $idx + 1;
            continue;
        }
        # Separate the item's inline content from any nested sub-list.
        def inlineKids as list of Node init [];
        def nested as list of Node init [];
        for (def c in children($item)) {
            if (typeOf($c) == "list") {
                $nested[] = $c;
            } else {
                $inlineKids[] = $c;
            }
        }
        # Place the marker on the item's first line.
        if (needsBreak($state, lineH($state.opts.bodySize))) {
            $state = flushPage($state);
        }
        def baseline as int init $state.y - $state.opts.bodySize;
        $state.page = pdf.text(
            $state.page,
            $state.x,
            $baseline,
            $state.opts.bodyFont,
            $state.opts.bodySize,
            $marker);
        # Item text flows in a column indented past the marker, starting on that line.
        def savedX as int init $state.x;
        def savedW as int init $state.width;
        $state.x = $savedX + $mw;
        $state.width = $savedW - $mw;
        def words as list of IWord init inlineWords($inlineKids, $state.opts);
        def lines as Packed init Packed{
            lines: packLines($words, $state.opts.bodySize, $state.width, $state.opts)
        };
        if (len($lines.lines) == 0) {
            $state.y = $state.y - lineH($state.opts.bodySize);
        } else {
            def bodySize as int init $state.opts.bodySize;
            $state = placeInlineLines($state, $lines, $bodySize);
        }
        for (def nl in $nested) {
            $state = renderList($state, $nl, $depth + 1);
        }
        $state.x = $savedX;
        $state.width = $savedW;
        $idx = $idx + 1;
    }
    return $state;
}

# The wrapped lines of one table row plus the tallest cell's line count.
def struct RowCells {
    lines as list of list of string,
    maxLines as int
};

# cellLineH is the line advance inside a table cell - tighter than prose leading
# (1.2x) so a multi-line cell stays compact.
func cellLineH(size as int) {
    return ($size * 6) // 5;
}

# maxWordWidth is the width of the widest single (space-delimited) word in a string -
# the minimum a column must be so that word does not overflow its cell (wrapText
# cannot break inside a word).
func maxWordWidth(txt as string, font as string, size as int) {
    def m as int init 1;
    for (def w in strings.split($txt, " ")) {
        def ww as int init roundPt(pdf.measureText($font, $size, $w));
        if ($ww > $m) {
            $m = $ww;
        }
    }
    return $m;
}

# columnEdges sizes the columns to their content. Each column first claims a minimum
# (its widest word plus padding, so nothing overflows into the next column), then the
# remaining width is shared in proportion to the columns' natural (full-cell) widths -
# so a long "Description" gets the extra room without starving a short "Type". Returns
# ncols+1 cumulative x-offsets (0 .. avail), relative to the table's left.
func columnEdges(
    rows as list of Node,
    ncols as int,
    avail as int,
    font as string,
    size as int,
    pad as int,
    unenc as string) {
    def nat as list of int init [];
    def mins as list of int init [];
    def c as int init 0;
    while ($c < $ncols) {
        $nat[] = 1;
        $mins[] = 1;
        $c = $c + 1;
    }
    for (def row in $rows) {
        def cells as list of Node init children($row);
        $c = 0;
        while ($c < $ncols) {
            if ($c < len($cells)) {
                def txt as string init pdf.toWinAnsi(plainText($cells[$c]), $unenc);
                def w as int init roundPt(pdf.measureText($font, $size, $txt)) + 2 * $pad;
                if ($w > $nat[$c]) {
                    $nat[$c] = $w;
                }
                def mw as int init maxWordWidth($txt, $font, $size) + 2 * $pad;
                if ($mw > $mins[$c]) {
                    $mins[$c] = $mw;
                }
            }
            $c = $c + 1;
        }
    }
    def natTotal as int init 0;
    def minTotal as int init 0;
    $c = 0;
    while ($c < $ncols) {
        $natTotal = $natTotal + $nat[$c];
        $minTotal = $minTotal + $mins[$c];
        $c = $c + 1;
    }
    def extra as int init $avail - $minTotal;
    def edges as list of int init [0];
    def x as int init 0;
    $c = 0;
    while ($c < $ncols) {
        def w as int init $mins[$c] + ($extra * $nat[$c]) // $natTotal;
        if ($extra < 0) {
            # Even the minimums exceed the page: shrink them proportionally (best
            # effort - a table this wide cannot fit without clipping).
            $w = ($avail * $mins[$c]) // $minTotal;
        }
        if ($c == $ncols - 1) {
            $w = $avail - $x;
        }
        $x = $x + $w;
        $edges[] = $x;
        $c = $c + 1;
    }
    return $edges;
}

# wrapRow word-wraps each cell of a row to its column (from `edges`), padding short
# rows to `ncols` empty cells, and reports the tallest cell so the row can be sized.
func wrapRow(
    cells as list of Node,
    ncols as int,
    edges as list of int,
    pad as int,
    font as string,
    size as int,
    unenc as string) {
    def lines as list of list of string init [];
    def maxLines as int init 1;
    def c as int init 0;
    while ($c < $ncols) {
        def txt as string init "";
        if ($c < len($cells)) {
            $txt = pdf.toWinAnsi(plainText($cells[$c]), $unenc);
        }
        def colW as int init $edges[$c + 1] - $edges[$c];
        def wl as list of string init pdf.wrapText($font, $size, $txt, $colW - 2 * $pad);
        if (len($wl) == 0) {
            def one as list of string init [""];
            $wl = $one;
        }
        $lines[] = $wl;
        if (len($wl) > $maxLines) {
            $maxLines = len($wl);
        }
        $c = $c + 1;
    }
    return RowCells{lines: $lines, maxLines: $maxLines};
}

# cellX returns the x for one wrapped line within its column, honouring the cell's
# alignment ("right" / "center" / left).
func cellX(
    cellLeft as int,
    colW as int,
    pad as int,
    align as string,
    font as string,
    size as int,
    line as string) {
    if ($align == "right") {
        return $cellLeft + $colW - $pad - roundPt(pdf.measureText($font, $size, $line));
    }
    if ($align == "center") {
        return $cellLeft + ($colW - roundPt(pdf.measureText($font, $size, $line))) // 2;
    }
    return $cellLeft + $pad;
}

# renderTable draws a table as a ruled grid. The per-cell wrapping (`wrapRow`) and
# per-line x placement (`cellX`) are extracted as pure helpers that take no `Layout`,
# so the row loop mutates one `Layout` frame in place - no full-state copy per row.
func renderTable(state as Layout, node as Node) {
    # Spanning cells are laid out on a plain grid (a span keeps its text in its
    # first slot), since the columns below are fixed. The table is passed on
    # (borrowed), not copied into a local.
    if (hasSpans($node)) {
        $state = renderGrid($state, gridTable($node));
        return $state;
    }
    $state = renderGrid($state, $node);
    return $state;
}

# renderGrid lays out a span-free table.
func renderGrid(state as Layout, t as Node) {
    def headRows as int init headerRowCount($t);
    if (len($t.children) == 0) {
        return $state;
    }
    def ncols as int init len($t.children[0].children);
    if ($ncols == 0) {
        return $state;
    }
    def size as int init $state.opts.bodySize;
    def pad as int init $state.opts.tablePad;
    def lh as int init cellLineH($size);
    def edges as list of int init columnEdges(
        $t.children,
        $ncols,
        $state.width,
        $state.opts.bodyFont,
        $size,
        $pad,
        $state.opts.unencodable);
    def r as int init 0;
    for (def row in $t.children) {
        def font as string init $state.opts.bodyFont;
        if ($r < $headRows) {
            $font = $state.opts.boldFont;
        }
        def rc as RowCells init wrapRow(
            $row.children,
            $ncols,
            $edges,
            $pad,
            $font,
            $size,
            $state.opts.unencodable);
        def rowH as int init $rc.maxLines * $lh + 2 * $pad;
        if (needsBreak($state, $rowH)) {
            $state = flushPage($state);
        }
        def rowTop as int init $state.y;
        def rowBot as int init $state.y - $rowH;
        # Optional shaded background behind the header row (drawn first, then the cell
        # text and grid on top; colour reset to black).
        if ($r < $headRows and $state.opts.tableHeaderFill.on) {
            def hf as Fill init $state.opts.tableHeaderFill;
            $state.page = fillRect($state.page, $state.x, $rowBot, $state.width, $rowH, $hf);
        }
        # Cell text, aligned per the column's markdown alignment.
        def c as int init 0;
        while ($c < $ncols) {
            def align as string init "left";
            if ($c < len($row.children)) {
                $align = attr($row.children[$c], "align");
            }
            def cellLeft as int init $state.x + $edges[$c];
            def colW as int init $edges[$c + 1] - $edges[$c];
            def ly as int init $rowTop - $pad - $size;
            for (def ln in $rc.lines[$c]) {
                $state.page = pdf.text(
                    $state.page,
                    cellX($cellLeft, $colW, $pad, $align, $font, $size, $ln),
                    $ly,
                    $font,
                    $size,
                    $ln);
                $ly = $ly - $lh;
            }
            $c = $c + 1;
        }
        # Grid: the top border once, a bottom border per row, and column verticals.
        def gridRight as int init $state.x + $state.width;
        if ($r == 0) {
            $state.page = pdf.line($state.page, $state.x, $rowTop, $gridRight, $rowTop);
        }
        $state.page = pdf.line($state.page, $state.x, $rowBot, $gridRight, $rowBot);
        def vc as int init 0;
        while ($vc <= $ncols) {
            def vx as int init $state.x + $edges[$vc];
            $state.page = pdf.line($state.page, $vx, $rowTop, $vx, $rowBot);
            $vc = $vc + 1;
        }
        $state.y = $rowBot;
        $r = $r + 1;
    }
    return $state;
}

# pdfAlertLabel is what this document calls an admonition kind: the author's own
# title first, then the caller's table, then the kind's name. The table is how a
# book in another language gets "Hinweis" without the module knowing any German.
func pdfAlertLabel(node as Node, opts as PdfOptions) {
    if ($node.title != "") {
        return $node.title;
    }
    if (maps.has($opts.admonitionLabels, $node.lang)) {
        return $opts.admonitionLabels[$node.lang];
    }
    return capitalise($node.lang);
}

# titled returns the admonition with its label prepended as a bold paragraph, so
# the quotation renderer can draw it without knowing what it is. Nodes are
# values, so this is a fresh one and the caller's tree is untouched.
func titled(node as Node, opts as PdfOptions) {
    def label as Node init nodeOf("strong");
    $label.children = [textNode(pdfAlertLabel($node, $opts))];
    def para as Node init nodeOf("paragraph");
    $para.children = [$label];
    def kids as list of Node init [$para];
    for (def child in $node.children) {
        $kids[] = $child;
    }
    def out as Node init $node;
    $out.children = $kids;
    return $out;
}

func renderQuote(state as Layout, node as Node, depth as int) {
    def indent as int init 16;
    def savedX as int init $state.x;
    def savedW as int init $state.width;
    def fill as Fill init $state.opts.quoteFill;
    def rule as Fill init $state.opts.quoteRule;
    # With a fill / rule configured, measure the quote first (render its children
    # into a throwaway layout with a fresh page) so the background can be painted
    # behind the text - PDF paints in stream order, so the fill must precede the
    # text. Only paint when the quote fits on the current page; a page-spanning
    # quote falls back to no background (a single rectangle could not frame it).
    if ($fill.on or $rule.on) {
        def probe as Layout init Layout{
            page: pdf.page($state.opts.pageWidth, $state.opts.pageHeight),
            y: $state.y,
            x: $savedX + $indent,
            width: $savedW - $indent,
            opts: $state.opts,
            pageNo: 0,
            done: [],
            bks: [],
            placements: $state.placements
        };
        for (def child in children($node)) {
            $probe = renderBlock($probe, $child, $depth + 1);
        }
        if (len($probe.done) == 0) {
            def top as int init $state.y + 2;
            def blockH as int init ($state.y - $probe.y) + 4;
            if ($fill.on) {
                $state.page = fillRect(
                    $state.page,
                    $savedX,
                    $top - $blockH,
                    $savedW,
                    $blockH,
                    $fill);
            }
            if ($rule.on) {
                $state.page = fillRect($state.page, $savedX, $top - $blockH, 3, $blockH, $rule);
            }
        }
    }
    $state.x = $savedX + $indent;
    $state.width = $savedW - $indent;
    for (def child in children($node)) {
        $state = renderBlock($state, $child, $depth + 1);
    }
    $state.x = $savedX;
    $state.width = $savedW;
    return $state;
}

# renderBlock dispatches one block node and leaves a gap after it.
# renderRule draws a thematic break as a thin grey rule across the content column.
# fillRect paints a filled rectangle in (r, g, b) on a page and resets the colour
# to black, so text and later content stay black. strokeRect is the same for an
# outlined rectangle. Both operate on the page (not the whole layout, which would
# copy the finished-pages list), and fold the repeated colour / rect / reset
# triplet that every shaded box in the renderer used.
func fillRect(pg as pdf.Page, x as int, y as int, w as int, h as int, c as Fill) {
    def out as pdf.Page init pdf.color($pg, $c.r, $c.g, $c.b);
    $out = pdf.rect($out, $x, $y, $w, $h, true);
    return pdf.color($out, 0, 0, 0);
}

func strokeRect(pg as pdf.Page, x as int, y as int, w as int, h as int, c as Fill) {
    def out as pdf.Page init pdf.color($pg, $c.r, $c.g, $c.b);
    $out = pdf.rect($out, $x, $y, $w, $h, false);
    return pdf.color($out, 0, 0, 0);
}

func renderRule(state as Layout) {
    def h as int init lineH($state.opts.bodySize);
    if (needsBreak($state, $h)) {
        $state = flushPage($state);
    }
    def midY as int init $state.y - $h // 2;
    $state.page = fillRect(
        $state.page,
        $state.x,
        $midY,
        $state.width,
        1,
        Fill{on: true, r: 128, g: 128, b: 128});
    $state.y = $state.y - $h;
    return $state;
}

# renderImage draws a supplied picture for a single-image paragraph, scaled to
# fit the text column and one page (aspect ratio kept). When no image was supplied
# for the node's URL - or the supplied one has no positive size - it falls back to
# the bracketed [alt] text, exactly as an inline image renders, so a missing image
# degrades to a caption rather than a blank.
func renderImage(state as Layout, node as Node) {
    if (not maps.has($state.placements, $node.url)) {
        $state = renderImageFallback($state, $node);
        return $state;
    }
    def img as pdf.Image init $state.placements[$node.url];
    if ($img.width <= 0 or $img.height <= 0) {
        $state = renderImageFallback($state, $node);
        return $state;
    }
    def w as int init pxToPoints($img.width, $state.opts.imageDpi);
    def h as int init pxToPoints($img.height, $state.opts.imageDpi);
    # Never wider than the text column (keep aspect ratio).
    if ($w > $state.width) {
        $w = $state.width;
        $h = $img.height * $w // $img.width;
    }
    # Never taller than a full page's usable height (keep aspect ratio), so an
    # over-tall picture fits one page instead of overflowing forever.
    def maxH as int init usableHeight($state.opts);
    if ($h > $maxH) {
        $h = $maxH;
        $w = $img.width * $h // $img.height;
    }
    # Break to a fresh page when it will not fit below the current position; a
    # freshly-started (empty) page just takes it, since it has been capped to fit.
    if (needsBreak($state, $h)) {
        $state = flushPage($state);
    }
    # drawImage places by the lower-left corner; $state.y is the current top.
    $state.page = pdf.drawImage($state.page, $img, $state.x, $state.y - $h, $w, $h);
    $state.y = $state.y - $h;
    return $state;
}

# renderImageFallback renders a bare image node as a one-line paragraph, so a URL
# with no supplied bytes keeps the [alt] text (the same run inlineText produces).
func renderImageFallback(state as Layout, node as Node) {
    def para as Node init nodeOf("paragraph");
    $para.children = [$node];
    $state = renderParagraph($state, $para);
    return $state;
}

# renderBlocksIn lays out mixed content (inline runs become paragraphs) in a
# column indented `indent` points, as a list item, a description or a footnote
# holds it. The container leaves its own gap after it, so the last inner block's
# gap is taken back.
func renderBlocksIn(state as Layout, ns as list of Node, indent as int, depth as int) {
    def savedX as int init $state.x;
    def savedW as int init $state.width;
    $state.x = $savedX + $indent;
    $state.width = $savedW - $indent;
    def gapped as bool init false;
    for (def b in blockify($ns)) {
        $state = renderBlock($state, $b, $depth + 1);
        $gapped = typeOf($b) != "page_break";
    }
    if ($gapped) {
        $state.y = $state.y + blockGap($state.opts);
    }
    $state.x = $savedX;
    $state.width = $savedW;
    return $state;
}

# wrapAll returns a paragraph whose inline children all sit inside one `kind`
# node (strong / emphasis), so the whole run draws in that node's font.
func wrapAll(ns as list of Node, kind as string) {
    def inner as Node init nodeOf($kind);
    $inner.children = $ns;
    def para as Node init nodeOf("paragraph");
    $para.children = [$inner];
    return $para;
}

# renderLineBlock draws a line block one line to a row, no gap between lines; an
# empty line keeps its height and a nested line block is indented.
func renderLineBlock(state as Layout, node as Node) {
    for (def c in children($node)) {
        if (typeOf($c) == "line_block") {
            def savedX as int init $state.x;
            def savedW as int init $state.width;
            $state.x = $savedX + 18;
            $state.width = $savedW - 18;
            $state = renderLineBlock($state, $c);
            $state.x = $savedX;
            $state.width = $savedW;
        } elseif (len(children($c)) == 0) {
            if (needsBreak($state, lineH($state.opts.bodySize))) {
                $state = flushPage($state);
            }
            $state.y = $state.y - lineH($state.opts.bodySize);
        } else {
            def para as Node init nodeOf("paragraph");
            $para.children = children($c);
            $state = renderParagraph($state, $para);
        }
    }
    return $state;
}

# renderNote draws a footnote or citation: its bracketed label hanging in the
# margin of the first line, the note's content in the column past it.
func renderNote(state as Layout, node as Node, depth as int) {
    def label as string init mdSanitize($state.opts, noteLabel($node));
    def size as int init $state.opts.bodySize;
    def lw as int init roundPt(pdf.measureText($state.opts.bodyFont, $size, $label)) + 6;
    if ($lw < 24) {
        $lw = 24;
    }
    if (needsBreak($state, lineH($state.opts.bodySize))) {
        $state = flushPage($state);
    }
    $state.page = pdf.text(
        $state.page,
        $state.x,
        $state.y - $state.opts.bodySize,
        $state.opts.bodyFont,
        $state.opts.bodySize,
        $label);
    if (len(children($node)) == 0) {
        $state.y = $state.y - lineH($state.opts.bodySize);
        return $state;
    }
    $state = renderBlocksIn($state, children($node), $lw, $depth);
    return $state;
}

# renderFigure draws a figure: the picture (or its [alt] text), the caption in
# italics, then the legend.
func renderFigure(state as Layout, node as Node, depth as int) {
    for (def c in children($node)) {
        match (typeOf($c)) {
            when "image" { $state = renderImage($state, $c); }
            when "caption" {
                $state.y = $state.y - blockGap($state.opts) // 2;
                $state = renderParagraph($state, wrapAll(children($c), "emphasis"));
            }
            when "legend" {
                $state.y = $state.y - blockGap($state.opts) // 2;
                $state = renderBlocksIn($state, children($c), 0, $depth);
            }
            else {
                if (isBlockKind(typeOf($c))) {
                    $state = renderBlock($state, $c, $depth + 1);
                } else {
                    def para as Node init nodeOf("paragraph");
                    $para.children = [$c];
                    $state = renderParagraph($state, $para);
                }
            }
        }
    }
    return $state;
}

func renderBlock(state as Layout, node as Node, depth as int) {
    match (typeOf($node)) {
        when "heading" { $state = renderHeading($state, $node); }
        when "paragraph" {
            # A paragraph that is a single image on its own line becomes a drawn
            # picture (when the caller supplied bytes for its URL); every other
            # paragraph - including one with an image mid-sentence - flows as text,
            # so an inline image keeps its [alt] run.
            def kids as list of Node init children($node);
            if (len($kids) == 1 and typeOf($kids[0]) == "image") {
                $state = renderImage($state, $kids[0]);
            } else {
                $state = renderParagraph($state, $node);
            }
        }
        when "list" { $state = renderList($state, $node, $depth); }
        when "definition_list" { $state = renderDefList($state, $node); }
        when "code" { $state = renderCode($state, $node); }
        when "table" {
            # A table's `title` is a bold line above it.
            if ($node.title != "") {
                $state = renderParagraph($state, wrapAll([textNode($node.title)], "strong"));
            }
            $state = renderTable($state, $node);
        }
        when "quote" { $state = renderQuote($state, $node, $depth); }
        # An admonition is a quotation with a title, and print has had that
        # shape for centuries. Giving the label to `renderQuote` as an ordinary
        # first child means it is measured with the rest, so the panel covers it
        # and a page-spanning callout falls back the same way a quotation does.
        when "admonition" {
            def panel as Node init titled($node, $state.opts);
            $state = renderQuote($state, $panel, $depth);
        }
        when "thematic_break" { $state = renderRule($state); }
        when "line_block" { $state = renderLineBlock($state, $node); }
        when "footnote", "citation" { $state = renderNote($state, $node, $depth); }
        when "figure" { $state = renderFigure($state, $node, $depth); }
        # A topic is drawn like an admonition: a titled quotation.
        when "topic" {
            def t as Node init $node;
            if ($node.title != "") {
                $t = titled($node, $state.opts);
            }
            $state = renderQuote($state, $t, $depth);
        }
        when "rubric" { $state = renderParagraph($state, wrapAll(children($node), "strong")); }
        when "container" {
            $state = renderBlocksIn($state, children($node), 0, $depth);
            # renderBlocksIn took back the inner gap; the container's own follows.
        }
        when "page_break" {
            # Start a fresh page; no trailing gap.
            $state = flushPage($state);
            return $state;
        }
        else { return $state; }
    }
    $state.y = $state.y - blockGap($state.opts);
    return $state;
}

# --- entry points ---------------------------------------------------

/**
 * Lay a parsed (or hand-built / transformed) `markdown` document tree out to a
 * `pdf.Document`, ready for any document-level work - a running header or footer
 * (`pdf.setHeader` / `pdf.setFooter`, with `%page%` / `%pages%`), extra `pdf.info`
 * metadata, more bookmarks - before it is serialised with `pdf.render`. This is
 * the seam `renderPdf` renders through; use it directly when you need the document
 * itself (a page number in a book's footer needs the total page count, which only
 * exists once the whole document is laid out).
 * @param doc {Node} the document root node from `parse`
 * @param opts {PdfOptions} the page geometry and fonts
 * @return {pdf.Document} the laid-out document, not yet serialised
 */
export func renderPdfDoc(doc as Node, opts as PdfOptions) {
    def outdoc as pdf.Document init buildDoc($opts);

    # Register every supplied image on the document up front, so `render` wires
    # each into every page's /XObject dict and any page may reference it. Build a
    # light placement lookup (each image with its byte fields stripped) to thread
    # through the layout: `drawImage` needs only the resource name, and copying
    # image bytes on every per-block Layout copy would be O(blocks * image bytes).
    # Iteration is in the images map's insertion order (a language guarantee), so
    # registration - and thus the resource numbering `render` assigns - is
    # deterministic regardless of how the caller built the map.
    def placements as map of string to pdf.Image init {};
    def seenNames as map of string to bool init {};
    def emptyBytes as bytes;
    for (def url in $opts.images) {
        def img as pdf.Image init $opts.images[$url];
        if (maps.has($seenNames, $img.name)) {
            treeFail("doctree: PdfOptions.images has two images named '" + $img.name +
                "'; each image needs a unique resource name (from pdf.loadImage)");
        }
        $seenNames[$img.name] = true;
        $outdoc = pdf.addImage($outdoc, $img);
        def light as pdf.Image init $img;
        $light.data = $emptyBytes;
        $light.smask = $emptyBytes;
        $light.hasSmask = false;
        $placements[$url] = $light;
    }

    # Thread the placement lookup, not the byte-heavy images map, through layout.
    def slim as PdfOptions init $opts;
    $slim.images = {};
    def state as Layout init newLayout($slim);
    $state.placements = $placements;
    # Collect every finalised page into a local list and attach them all with one
    # pdf.addPages at the end: pdf.addPage copies the whole document (all prior
    # pages' content streams) per call, so adding them one-by-one here was
    # O(pages^2) over a long report. Bookmarks stay interleaved - they only push a
    # small outline entry and the document carries no pages during the loop, so a
    # bookmark copy is cheap - and the page NUMBER each references is independent
    # of when the page is attached.
    def allPages as list of pdf.Page init [];
    for (def block in children($doc)) {
        $state = renderBlock($state, $block, 0);
        for (def p in $state.done) {
            $allPages[] = $p;
        }
        $state.done = [];
        for (def b in $state.bks) {
            $outdoc = pdf.bookmark($outdoc, $b.page, $b.y, $b.title, $b.level);
        }
        $state.bks = [];
    }
    $allPages[] = $state.page;
    $outdoc = pdf.addPages($outdoc, $allPages);
    return $outdoc;
}

/**
 * Render a parsed (or hand-built / transformed) `markdown` document tree to PDF bytes
 * with the given options. A one-liner over `renderPdfDoc` + `pdf.render`; call those
 * two directly when you need to touch the `pdf.Document` in between.
 * @param doc {Node} the document root node from `parse`
 * @param opts {PdfOptions} the page geometry and fonts
 * @return {bytes} the PDF document
 */
export func renderPdf(doc as Node, opts as PdfOptions) {
    return pdf.render(renderPdfDoc($doc, $opts));
}

