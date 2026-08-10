# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0

# A hand-rolled Markdown parser: its inline scanner legitimately runs past the
# L201 statement-count limit. Every other lint check stays active.
# lint-disable-file: L201

/**
 * A lightweight Markdown renderer for a small CommonMark subset: ATX headings,
 * bold / italic emphasis, inline code, links, images, fenced code blocks,
 * unordered / ordered lists (**nested** by indentation), blockquotes (nesting
 * recursively), and GFM tables. Renders to HTML (through the `html`
 * module, so escaping is handled for you) and to styled terminal text (through
 * the `ansi` module). It also authors Markdown text (header / style / link /
 * list / codeBlock / table). Pure Jennifer; line-oriented block parsing with a
 * small inline scanner. Not full CommonMark: inline spans do not nest (the
 * content of `**...**`, `` `...` ``, a link, or an image alt is plain text), and
 * there is no thematic break, setext heading, or reference-link support. A
 * link / image URL cannot contain an unescaped `)` (the scanner closes on the
 * first one).
 * @module markdown
 * @example
 * import "markdown.j" as markdown;
 * io.printf("%s\n", markdown.toHtml("# Hi\n\nA **bold** word.\n\n> a quote"));
 * io.printf("%s\n", markdown.toAnsi("- one\n  - nested\n- two"));
 */
import "./html.j" as html;
import "./ansi.j" as ansi;
use strings;
use regex;
use convert;
use lists;

# The inline span kinds and block kinds, as sum types: the renderers `match` on
# them, so adding a kind surfaces every place that must handle it (both the HTML
# and the ANSI path) instead of silently falling through a string compare.
def enum SpanKind { Text, Code, Strong, Em, Link, Image };
def enum BlockKind { Paragraph, Heading, Code, List, Table, Quote };

# An inline span: a run of Text, or an emphasised / code / link / image span.
# Link and Image spans carry the target in `url` (and an optional `title`); the
# others leave both "".
def struct Span {
    kind as SpanKind,
    text as string,
    url as string,
    title as string
};

# A block: a Heading (with `level`), a Paragraph or Code block (in `text`), a
# List (`ordered` plus `items`, and a parallel `children` sub-list per item for
# nesting), a Table (`headings` + `aligns` + `rows`), or a Quote (its inner
# blocks in `children`). `children` is a `list of Block` (recursive - allowed
# because a `list` field has a finite zero), holding a List's nested sub-lists
# or a Quote's inner blocks.
def struct Block {
    kind as BlockKind,
    level as int,
    text as string,
    lang as string,
    ordered as bool,
    items as list of string,
    headings as list of string,
    aligns as list of string,
    rows as list of list of string,
    children as list of Block
};

# A parsed block plus the line index to resume scanning at (the collect helpers'
# return, since a Jennifer function returns a single value).
def struct BlockScan {
    block as Block,
    next as int
};

# A parsed fenced code block plus the line index to resume scanning at
# (a helper's return, since Jennifer functions return a single value and
# value semantics rule out appending into a caller's list).
def struct Fence {
    code as string,
    lang as string,
    next as int
};

# A parsed table block plus the line index to resume scanning at.
def struct TableScan {
    block as Block,
    next as int
};

# The reformatted lines of one table plus the line index to resume at.
def struct TablePretty {
    lines as list of string,
    next as int
};

# --- span + block constructors (private) ---------------------------

func span(kind as SpanKind, text as string, url as string) {
    return Span{kind: $kind, text: $text, url: $url, title: ""};
}

# linkSpan builds a link span carrying an optional title (from `[t](url "t")`).
func linkSpan(text as string, url as string, title as string) {
    return Span{kind: SpanKind.Link, text: $text, url: $url, title: $title};
}

func paraBlock(lines as list of string) {
    def joined as string init strings.join($lines, " ");
    return Block{
        kind: BlockKind.Paragraph,
        level: 0,
        text: $joined,
        lang: "",
        ordered: false,
        items: [],
        headings: [],
        aligns: [],
        rows: [],
        children: []
    };
}

# listBlockFull builds a List block with its item texts and a parallel
# `children` list (one entry per item: a nested sub-list block, or an empty List
# when the item has no nesting).
func listBlockFull(items as list of string, children as list of Block, ordered as bool) {
    return Block{
        kind: BlockKind.List,
        level: 0,
        text: "",
        lang: "",
        ordered: $ordered,
        items: $items,
        headings: [],
        aligns: [],
        rows: [],
        children: $children
    };
}

# emptyListFor is the "no nested list" placeholder attached to an item until (and
# unless) a more-indented sub-list replaces it. Its empty `items` marks it absent.
func emptyListFor(ordered as bool) {
    def none as list of Block init [];
    def noItems as list of string init [];
    return listBlockFull($noItems, $none, $ordered);
}

# quoteBlock wraps the inner blocks of a blockquote.
func quoteBlock(children as list of Block) {
    return Block{
        kind: BlockKind.Quote,
        level: 0,
        text: "",
        lang: "",
        ordered: false,
        items: [],
        headings: [],
        aligns: [],
        rows: [],
        children: $children
    };
}

func tableBlock(
    headings as list of string,
    aligns as list of string,
    rows as list of list of string) {
    return Block{
        kind: BlockKind.Table,
        level: 0,
        text: "",
        lang: "",
        ordered: false,
        items: [],
        headings: $headings,
        aligns: $aligns,
        rows: $rows,
        children: []
    };
}

func codeBlockNode(text as string, lang as string) {
    return Block{
        kind: BlockKind.Code,
        level: 0,
        text: $text,
        lang: $lang,
        ordered: false,
        items: [],
        headings: [],
        aligns: [],
        rows: [],
        children: []
    };
}

# --- inline scanner (private) --------------------------------------

# isFlankSpace reports whether c is whitespace, for CommonMark emphasis
# flanking (a delimiter run flanked by whitespace on the inside does not open /
# close emphasis).
func isFlankSpace(c as string) {
    return $c == " " or $c == "\t" or $c == "\n" or $c == "\r";
}

# nextIndexArray returns an int list `nxt` of length len(cs)+1 where nxt[j] is
# the index of the first `target` at or after j, or len(cs) if there is none.
# One backward pass, so the inline scanner can resolve "where does this marker
# close?" in O(1) instead of re-scanning to the end at every position (which
# made a run of unmatched markers O(N^2)).
func nextIndexArray(cs as list of string, target as string) {
    def n as int init len($cs);
    def nxt as list of int init [];
    def k as int init 0;
    while ($k <= $n) {
        $nxt[] = $n;
        $k = $k + 1;
    }
    def j as int init $n - 1;
    while ($j >= 0) {
        if ($cs[$j] == $target) {
            $nxt[$j] = $j;
        } else {
            $nxt[$j] = $nxt[$j + 1];
        }
        $j = $j - 1;
    }
    return $nxt;
}

# nextPairArray is nextIndexArray for a doubled marker: nxt[j] is the first
# index of a `target target` pair at or after j, or len(cs) if none.
func nextPairArray(cs as list of string, target as string) {
    def n as int init len($cs);
    def nxt as list of int init [];
    def k as int init 0;
    while ($k <= $n) {
        $nxt[] = $n;
        $k = $k + 1;
    }
    def j as int init $n - 2;
    while ($j >= 0) {
        if ($cs[$j] == $target and $cs[$j + 1] == $target) {
            $nxt[$j] = $j;
        } else {
            $nxt[$j] = $nxt[$j + 1];
        }
        $j = $j - 1;
    }
    return $nxt;
}

# NOTE: the precomputed next-index arrays are read by direct indexing
# (`$arr[$from]`) at the marker sites below, never passed into a helper -
# handing a list to a function deep-copies it under Jennifer's value semantics,
# which would reintroduce the O(N^2) the arrays exist to remove.

# parseInline scans a line of text into spans. Markers: `` ` `` for code,
# `**` for strong, `*` for emphasis, and `[text](url)` for a link. Span
# content is not re-scanned (no nesting).
func parseInline(s as string) {
    def spans as list of Span init [];
    def cs as list of string init strings.chars($s);
    def n as int init len($cs);
    # Precompute, in one pass each, where every marker next occurs, so a run of
    # unmatched `[`, `*`, or `` ` `` costs O(N) total rather than O(N) per
    # position (the old forward re-scan was O(N^2) on adversarial input).
    def nBacktick as list of int init nextIndexArray($cs, "`");
    def nStar as list of int init nextIndexArray($cs, "*");
    def nDblStar as list of int init nextPairArray($cs, "*");
    def nRbrack as list of int init nextIndexArray($cs, "]");
    def nRparen as list of int init nextIndexArray($cs, ")");
    def i as int init 0;
    # The pending plain-text run is s[bufStart:i]; slicing it with substring
    # (Go-side, linear) instead of accumulating rune-by-rune keeps a long text
    # run from being O(N^2) to build.
    def bufStart as int init 0;
    while ($i < $n) {
        def c as string init $cs[$i];
        # inline code: `code`
        def bt as int init -1;
        if ($c == "`") {
            def k as int init $nBacktick[$i + 1];
            if ($k < $n) {
                $bt = $k;
            }
        }
        if ($bt >= 0) {
            if ($i > $bufStart) {
                $spans[] = span(SpanKind.Text, strings.join(lists.slice($cs, $bufStart, $i), ""), "");
            }
            $spans[] = span(SpanKind.Code, strings.join(lists.slice($cs, $i + 1, $bt), ""), "");
            $i = $bt + 1;
            $bufStart = $i;
            continue;
        }
        # strong: **text** (flanking: no whitespace just inside the markers, so
        # a space-flanked `**` is literal, not a delimiter).
        def dbl as int init -1;
        if ($c == "*" and $i + 1 < $n and $cs[$i + 1] == "*" and $i + 2 < $n and
            not isFlankSpace($cs[$i + 2])) {
            def k as int init $nDblStar[$i + 2];
            if ($k < $n and not isFlankSpace($cs[$k - 1])) {
                $dbl = $k;
            }
        }
        if ($dbl >= 0) {
            if ($i > $bufStart) {
                $spans[] = span(SpanKind.Text, strings.join(lists.slice($cs, $bufStart, $i), ""), "");
            }
            $spans[] = span(SpanKind.Strong, strings.join(lists.slice($cs, $i + 2, $dbl), ""), "");
            $i = $dbl + 2;
            $bufStart = $i;
            continue;
        }
        # emphasis: *text* (flanking: the char right after the opening `*` and
        # right before the closing `*` must be non-space, so "3 * 4 * 5" is not
        # italicized).
        def em as int init -1;
        if ($c == "*" and $i + 1 < $n and not isFlankSpace($cs[$i + 1])) {
            def k as int init $nStar[$i + 1];
            if ($k < $n and not isFlankSpace($cs[$k - 1])) {
                $em = $k;
            }
        }
        if ($em >= 0) {
            if ($i > $bufStart) {
                $spans[] = span(SpanKind.Text, strings.join(lists.slice($cs, $bufStart, $i), ""), "");
            }
            $spans[] = span(SpanKind.Em, strings.join(lists.slice($cs, $i + 1, $em), ""), "");
            $i = $em + 1;
            $bufStart = $i;
            continue;
        }
        # image: ![alt](url) - like a link, but opened by `!` before the `[`.
        def imgEnd as int init -1;
        def irb as int init -1;
        def irp as int init -1;
        if ($c == "!" and $i + 1 < $n and $cs[$i + 1] == "[") {
            def kb as int init $nRbrack[$i + 2];
            def kp as int init -1;
            if ($kb < $n and $kb + 1 < $n and $cs[$kb + 1] == "(") {
                $kp = $nRparen[$kb + 2];
            }
            if ($kp >= 0 and $kp < $n) {
                $irb = $kb;
                $irp = $kp;
                $imgEnd = $kp + 1;
            }
        }
        if ($imgEnd >= 0) {
            if ($i > $bufStart) {
                $spans[] = span(SpanKind.Text, strings.join(lists.slice($cs, $bufStart, $i), ""), "");
            }
            def altText as string init strings.join(lists.slice($cs, $i + 2, $irb), "");
            def imgDest as string init strings.join(lists.slice($cs, $irb + 2, $irp), "");
            $spans[] = imageSpanFrom($altText, $imgDest);
            $i = $imgEnd;
            $bufStart = $i;
            continue;
        }
        # link: [text](url) - resolved via the precomputed `]` and `)` arrays.
        def linkEnd as int init -1;
        def rb as int init -1;
        def rp as int init -1;
        if ($c == "[") {
            def kb as int init $nRbrack[$i + 1];
            def kp as int init -1;
            if ($kb < $n and $kb + 1 < $n and $cs[$kb + 1] == "(") {
                $kp = $nRparen[$kb + 2];
            }
            if ($kp >= 0 and $kp < $n) {
                $rb = $kb;
                $rp = $kp;
                $linkEnd = $kp + 1;
            }
        }
        if ($linkEnd >= 0) {
            if ($i > $bufStart) {
                $spans[] = span(SpanKind.Text, strings.join(lists.slice($cs, $bufStart, $i), ""), "");
            }
            # Slice the (short) text and destination here - lists.slice on the
            # rune list is O(slice length) regardless of position, unlike
            # strings.substring which walks from the string start (O(offset),
            # so O(N^2) across many links).
            def linkText as string init strings.join(lists.slice($cs, $i + 1, $rb), "");
            def linkDest as string init strings.join(lists.slice($cs, $rb + 2, $rp), "");
            $spans[] = linkSpanFrom($linkText, $linkDest);
            $i = $linkEnd;
            $bufStart = $i;
            continue;
        }
        $i = $i + 1;
    }
    if ($n > $bufStart) {
        $spans[] = span(SpanKind.Text, strings.join(lists.slice($cs, $bufStart, $n), ""), "");
    }
    return $spans;
}

# A link / image destination split into a URL and an optional title.
def struct Dest {
    url as string,
    title as string
};

# splitDest parses a `[...](url "title")` destination: the URL, then an optional
# space-separated quoted title (a single pair of surrounding `"` or `'` stripped),
# so the title lands in its own attribute rather than in the href / src.
func splitDest(rawDest as string) {
    def dest as string init strings.trim($rawDest);
    def sp as int init strings.indexOf($dest, " ");
    if ($sp < 0) {
        return Dest{url: $dest, title: ""};
    }
    def url as string init strings.substring($dest, 0, $sp);
    def rawTitle as string init strings.trim(strings.substring($dest, $sp + 1, len($dest)));
    if (len($rawTitle) >= 2 and
        (strings.startsWith($rawTitle, "\"") and strings.endsWith($rawTitle, "\"") or
        strings.startsWith($rawTitle, "'") and strings.endsWith($rawTitle, "'"))) {
        $rawTitle = strings.substring($rawTitle, 1, len($rawTitle) - 1);
    }
    return Dest{url: $url, title: $rawTitle};
}

# linkSpanFrom builds the link span for a `[text](url)` from the already-sliced
# (short) link text and destination.
func linkSpanFrom(text as string, rawDest as string) {
    def d as Dest init splitDest($rawDest);
    return linkSpan($text, $d.url, $d.title);
}

# imageSpan builds an image span carrying alt text, a URL, and an optional title.
func imageSpan(alt as string, url as string, title as string) {
    return Span{kind: SpanKind.Image, text: $alt, url: $url, title: $title};
}

# imageSpanFrom builds the image span for `![alt](url "title")`, splitting the
# destination exactly like a link.
func imageSpanFrom(alt as string, rawDest as string) {
    def d as Dest init splitDest($rawDest);
    return imageSpan($alt, $d.url, $d.title);
}

# --- block parser (private) ----------------------------------------

# lineType classifies a source line: "blank", "fence", "quote", "heading", "ul"
# (unordered item), "ol" (ordered item), or "plain".
func lineType(trimmed as string, line as string) {
    if (len($trimmed) == 0) {
        return "blank";
    }
    if (strings.startsWith($trimmed, "```")) {
        return "fence";
    }
    if (strings.startsWith($trimmed, ">")) {
        return "quote";
    }
    if (regex.matches("^(#\{1,6\})[ \t]+", $line)) {
        return "heading";
    }
    if (regex.matches("^[ \t]*[-*+][ \t]+", $line)) {
        return "ul";
    }
    if (regex.matches("^[ \t]*[0-9]+\\.[ \t]+", $line)) {
        return "ol";
    }
    return "plain";
}

# splitCells splits one table row into trimmed cell strings, dropping an
# optional leading / trailing `|` and treating `\|` as a literal pipe.
func splitCells(row as string) {
    def cells as list of string init [];
    def cs as list of string init strings.chars(strings.trim($row));
    def n as int init len($cs);
    def start as int init 0;
    def end as int init $n;
    if ($n > 0 and $cs[0] == "|") {
        $start = 1;
    }
    if ($end > $start and $cs[$end - 1] == "|") {
        $end = $end - 1;
    }
    def buf as string init "";
    def i as int init $start;
    while ($i < $end) {
        def c as string init $cs[$i];
        if ($c == "\\" and $i + 1 < $end and $cs[$i + 1] == "|") {
            $buf = $buf + "|";
            $i = $i + 2;
            continue;
        }
        if ($c == "|") {
            $cells[] = strings.trim($buf);
            $buf = "";
            $i = $i + 1;
            continue;
        }
        $buf = $buf + $c;
        $i = $i + 1;
    }
    $cells[] = strings.trim($buf);
    return $cells;
}

# cellAlign reads a delimiter cell (`:---`, `---:`, `:---:`, `---`) as an
# alignment name.
func cellAlign(cell as string) {
    def t as string init strings.trim($cell);
    def left as bool init strings.startsWith($t, ":");
    def right as bool init strings.endsWith($t, ":");
    if ($left and $right) {
        return "center";
    }
    if ($right) {
        return "right";
    }
    if ($left) {
        return "left";
    }
    return "none";
}

# parseAligns reads the per-column alignments from a delimiter row.
func parseAligns(delim as string) {
    def out as list of string init [];
    for (def cell in splitCells($delim)) {
        $out[] = cellAlign($cell);
    }
    return $out;
}

# isDelimiterRow reports whether a line is a table delimiter row: every cell is
# an optional-colon run of dashes.
func isDelimiterRow(s as string) {
    def t as string init strings.trim($s);
    if (not strings.contains($t, "-")) {
        return false;
    }
    for (def cell in splitCells($t)) {
        if (not regex.matches("^:?-+:?$", strings.trim($cell))) {
            return false;
        }
    }
    return true;
}

# looksLikeTable reports whether lines[i] opens a table: a pipe-bearing header
# row over a delimiter row of the same column count.
func looksLikeTable(lines as list of string, i as int) {
    if ($i + 1 >= len($lines)) {
        return false;
    }
    if (not strings.contains($lines[$i], "|")) {
        return false;
    }
    if (not isDelimiterRow($lines[$i + 1])) {
        return false;
    }
    return len(splitCells($lines[$i])) == len(parseAligns($lines[$i + 1]));
}

# tableFrom parses the table opening at line `i` (header, delimiter, then data
# rows until a blank or pipe-less line) into a block plus the resume index.
func tableFrom(lines as list of string, i as int) {
    def headings as list of string init splitCells($lines[$i]);
    def aligns as list of string init parseAligns($lines[$i + 1]);
    def rows as list of list of string init [];
    def n as int init len($lines);
    def j as int init $i + 2;
    while ($j < $n and len(strings.trim($lines[$j])) > 0 and strings.contains($lines[$j], "|")) {
        $rows[] = splitCells($lines[$j]);
        $j = $j + 1;
    }
    return TableScan{block: tableBlock($headings, $aligns, $rows), next: $j};
}

# One list line parsed into its indent (columns; -1 = not a list line), its
# ordered flag, and its content text.
def struct ItemInfo {
    indent as int,
    ordered as bool,
    text as string
};

# leadingWidth counts a line's leading indentation in columns (a tab is 4).
func leadingWidth(line as string) {
    def w as int init 0;
    for (def ch in strings.chars($line)) {
        if ($ch == " ") {
            $w = $w + 1;
        } elseif ($ch == "\t") {
            $w = $w + 4;
        } else {
            return $w;
        }
    }
    return $w;
}

# listItemAt parses a list line into its indent / ordered flag / content; an
# indent of -1 marks a non-list line.
func listItemAt(line as string) {
    def m as regex.Match init regex.find("^([ \t]*)([-*+]|[0-9]+\\.)[ \t]+(.*)$", $line);
    if ($m.start < 0) {
        return ItemInfo{indent: -1, ordered: false, text: ""};
    }
    return ItemInfo{
        indent: leadingWidth($line),
        ordered: strings.endsWith($m.groups[1], "."),
        text: $m.groups[2]
    };
}

# mergeLists concatenates two List blocks into one (items + parallel children),
# keeping the first list's ordered flag. Used only to fold a second, inconsistently
# indented nested run into an item's existing sub-list instead of dropping it.
func mergeLists(a as Block, b as Block) {
    def items as list of string init [];
    def children as list of Block init [];
    for (def it in $a.items) {
        $items[] = $it;
    }
    for (def it in $b.items) {
        $items[] = $it;
    }
    for (def ch in $a.children) {
        $children[] = $ch;
    }
    for (def ch in $b.children) {
        $children[] = $ch;
    }
    return listBlockFull($items, $children, $a.ordered);
}

# collectList parses a whole (possibly nested) list starting at line `start`. It
# gathers sibling items at the first item's indent; a more-indented run becomes a
# nested list attached to the preceding item's `children` slot; a less-indented
# line or a marker-type flip ends the list. Returns the List block + resume index.
func collectList(lines as list of string, start as int) {
    def base as ItemInfo init listItemAt($lines[$start]);
    def items as list of string init [];
    def children as list of Block init [];
    def n as int init len($lines);
    def j as int init $start;
    while ($j < $n) {
        def info as ItemInfo init listItemAt($lines[$j]);
        if ($info.indent < 0) {
            # Not a list line. A blank line is interior (loose list, or a gap
            # before a nested list) only if a list item at >= base indent follows.
            if (len(strings.trim($lines[$j])) == 0) {
                def k as int init $j + 1;
                while ($k < $n and len(strings.trim($lines[$k])) == 0) {
                    $k = $k + 1;
                }
                if ($k < $n and listItemAt($lines[$k]).indent >= $base.indent) {
                    $j = $k;
                    continue;
                }
            }
            break;
        }
        if ($info.indent < $base.indent) {
            break;
        }
        if ($info.indent > $base.indent) {
            # A deeper run nests under the last item. Normally it replaces the
            # placeholder; if the item already has a nested list (inconsistent
            # sub-indentation put a second deeper run under it), merge rather than
            # overwrite, so no items are silently dropped.
            def sub as BlockScan init collectList($lines, $j);
            if (len($children) > 0) {
                def last as int init len($children) - 1;
                if (len($children[$last].items) == 0) {
                    $children[$last] = $sub.block;
                } else {
                    $children[$last] = mergeLists($children[$last], $sub.block);
                }
            }
            $j = $sub.next;
            continue;
        }
        # Same level: a sibling. A marker-type flip ends this list.
        if (not ($info.ordered == $base.ordered)) {
            break;
        }
        $items[] = $info.text;
        $children[] = emptyListFor($base.ordered);
        $j = $j + 1;
    }
    return BlockScan{block: listBlockFull($items, $children, $base.ordered), next: $j};
}

# stripQuoteMarker removes one leading `>` (and an optional following space) from
# a blockquote line, after trimming up to three leading spaces.
func stripQuoteMarker(line as string) {
    def t as string init strings.trimLeft($line);
    if (strings.startsWith($t, "> ")) {
        return strings.substring($t, 2, len($t));
    }
    if (strings.startsWith($t, ">")) {
        return strings.substring($t, 1, len($t));
    }
    return $t;
}

# collectQuote gathers a run of `>`-prefixed lines, strips the marker, and parses
# the inner text recursively - so a blockquote holds real blocks (paragraphs,
# lists, nested quotes). Returns the Quote block + resume index.
func collectQuote(lines as list of string, start as int) {
    def inner as list of string init [];
    def n as int init len($lines);
    def j as int init $start;
    while ($j < $n and strings.startsWith(strings.trim($lines[$j]), ">")) {
        $inner[] = stripQuoteMarker($lines[$j]);
        $j = $j + 1;
    }
    return BlockScan{block: quoteBlock(parseLines($inner)), next: $j};
}

# parseBlocks splits Markdown text into a list of blocks, line by line.
func parseBlocks(md as string) {
    return parseLines(strings.split($md, "\n"));
}

# parseLines is parseBlocks over an already-split line list, so a blockquote can
# recurse on its stripped inner lines without re-splitting. The paragraph-flush
# guard closes an open paragraph before a line that does not continue it; list
# and quote runs are consumed whole by their collectors.
func parseLines(lines as list of string) {
    def blocks as list of Block init [];
    def n as int init len($lines);
    def para as list of string init [];
    def i as int init 0;
    while ($i < $n) {
        def line as string init $lines[$i];
        def lt as string init lineType(strings.trim($line), $line);
        # A table opens on a pipe row over a delimiter row; it reads as "plain"
        # but needs the two-line lookahead lineType cannot do.
        def isTable as bool init ($lt == "plain") and looksLikeTable($lines, $i);
        # A paragraph continues only across a plain, non-table line.
        if ((not ($lt == "plain") or $isTable) and len($para) > 0) {
            $blocks[] = paraBlock($para);
            $para = [];
        }
        if ($lt == "blank") {
            $i = $i + 1;
            continue;
        }
        if ($lt == "fence") {
            def f as Fence init collectFence($lines, $i);
            $blocks[] = codeBlockNode($f.code, $f.lang);
            $i = $f.next;
            continue;
        }
        if ($lt == "quote") {
            def qs as BlockScan init collectQuote($lines, $i);
            $blocks[] = $qs.block;
            $i = $qs.next;
            continue;
        }
        if ($lt == "heading") {
            def hm as regex.Match init regex.find("^(#\{1,6\})[ \t]+(.*)$", $line);
            $blocks[] = headingBlock(len($hm.groups[0]), $hm.groups[1]);
            $i = $i + 1;
            continue;
        }
        if ($lt == "ul" or $lt == "ol") {
            def ls as BlockScan init collectList($lines, $i);
            $blocks[] = $ls.block;
            $i = $ls.next;
            continue;
        }
        if ($isTable) {
            def ts as TableScan init tableFrom($lines, $i);
            $blocks[] = $ts.block;
            $i = $ts.next;
            continue;
        }
        $para[] = strings.trim($line);
        $i = $i + 1;
    }
    if (len($para) > 0) {
        $blocks[] = paraBlock($para);
    }
    return $blocks;
}

func headingBlock(level as int, text as string) {
    return Block{
        kind: BlockKind.Heading,
        level: $level,
        text: $text,
        lang: "",
        ordered: false,
        items: [],
        headings: [],
        aligns: [],
        rows: [],
        children: []
    };
}

# collectFence gathers the fenced code block opening at line `open` and returns
# its content plus the line index just past the closing fence.
func collectFence(lines as list of string, open as int) {
    def n as int init len($lines);
    # Record the opening fence length; a shorter run of backticks inside the
    # block is content, not a terminator (only a fence of equal-or-greater
    # length closes it).
    def trimmedOpen as string init strings.trim($lines[$open]);
    def openLen as int init fenceLen($trimmedOpen);
    # The info string follows the opening backtick run; its first word is the
    # language (```python -> "python"). The rest (rare "```python extra") is dropped.
    def info as string init strings.trim(strings.substring($trimmedOpen, $openLen, len($trimmedOpen)));
    def lang as string init $info;
    def sp as int init strings.indexOf($info, " ");
    if ($sp >= 0) {
        $lang = strings.substring($info, 0, $sp);
    }
    def parts as list of string init [];
    def j as int init $open + 1;
    while ($j < $n) {
        if (fenceLen(strings.trim($lines[$j])) >= $openLen) {
            break;
        }
        $parts[] = $lines[$j];
        $j = $j + 1;
    }
    def code as string init strings.join($parts, "\n");
    if ($j < $n) {
        return Fence{code: $code, lang: $lang, next: $j + 1};
    }
    return Fence{code: $code, lang: $lang, next: $j};
}

# fenceLen returns the number of leading backticks in a trimmed line when it is
# a code fence (3 or more), else 0.
func fenceLen(trimmed as string) {
    def cs as list of string init strings.chars($trimmed);
    def k as int init 0;
    while ($k < len($cs) and $cs[$k] == "`") {
        $k = $k + 1;
    }
    if ($k >= 3) {
        return $k;
    }
    return 0;
}

# --- public document tree (reader) ---------------------------------

/**
 * A node in a parsed Markdown document tree. The tree is walked with the reader
 * accessors (`typeOf` / `children` / `text` / `level` / `attr` / `get` / `findAll` /
 * `has`), the same family vocabulary as `xml` / `html`. A node's `kind` is one of the
 * block kinds `"document"` / `"heading"` / `"paragraph"` / `"code"` / `"list"` /
 * `"item"` / `"table"` / `"row"` / `"cell"` / `"quote"`, or the inline kinds
 * `"text"` / `"codespan"` / `"strong"` / `"emphasis"` / `"link"` / `"image"`.
 * @field kind {string} the node kind
 * @field level {int} a heading's level (1-6); 0 otherwise
 * @field text {string} literal text (a `text` / `codespan` / `code` node, or an
 *   `image`'s alt); "" for a container (read its content with the `text` accessor)
 * @field lang {string} a fenced `code` block's language ("" if none)
 * @field ordered {bool} whether a `list` is ordered
 * @field url {string} a `link` / `image` target
 * @field title {string} a `link` / `image` title ("" if none)
 * @field align {string} a table `cell`'s alignment ("left" / "right" / "center" / "")
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
    children as list of Node
};

# The nesting-depth cap and total-node budget that turn a pathological document
# (deeply nested quotes / lists, or a huge input) into a catchable "markdown" error
# rather than an unbounded recursion or allocation.
def const MAX_DOC_DEPTH as int init 100;
def const MAX_DOC_NODES as int init 200000;

func mdFail(msg as string) {
    throw Error{kind: "markdown", message: $msg, file: "", line: 0, col: 0};
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

# spanToPublic maps one inline Span to a public inline Node. The `match` is over a
# Span, so the resolver checks it covers every SpanKind.
func spanToPublic(sp as Span) {
    match ($sp.kind) {
        when Text {
            return textNode($sp.text);
        }
        when Code {
            def n as Node init nodeOf("codespan");
            $n.text = $sp.text;
            return $n;
        }
        when Strong {
            def n as Node init nodeOf("strong");
            $n.children = [textNode($sp.text)];
            return $n;
        }
        when Em {
            def n as Node init nodeOf("emphasis");
            $n.children = [textNode($sp.text)];
            return $n;
        }
        when Link {
            def n as Node init nodeOf("link");
            $n.url = $sp.url;
            $n.title = $sp.title;
            $n.children = [textNode($sp.text)];
            return $n;
        }
        when Image {
            def n as Node init nodeOf("image");
            $n.url = $sp.url;
            $n.title = $sp.title;
            $n.text = $sp.text;
            return $n;
        }
    }
}

func inlineToPublic(spans as list of Span) {
    def out as list of Node init [];
    for (def sp in $spans) {
        $out[] = spanToPublic($sp);
    }
    return $out;
}

# cellNodePublic builds a table "cell" node with its alignment and inline content.
func cellNodePublic(text as string, align as string) {
    def n as Node init nodeOf("cell");
    $n.align = $align;
    $n.children = inlineToPublic(parseInline($text));
    return $n;
}

# rowNodePublic builds a "row" of exactly `cols` "cell" nodes (a short source row
# pads with empty cells, a long one truncates), so every table row is rectangular
# against the header - the shape the renderers assume.
func rowNodePublic(cells as list of string, aligns as list of string, cols as int) {
    def n as Node init nodeOf("row");
    def kids as list of Node init [];
    def i as int init 0;
    while ($i < $cols) {
        def cellText as string init "";
        if ($i < len($cells)) {
            $cellText = $cells[$i];
        }
        $kids[] = cellNodePublic($cellText, alignOf($aligns, $i));
        $i = $i + 1;
    }
    $n.children = $kids;
    return $n;
}

func tableToPublic(b as Block) {
    def n as Node init nodeOf("table");
    def cols as int init len($b.headings);
    def kids as list of Node init [];
    $kids[] = rowNodePublic($b.headings, $b.aligns, $cols);
    for (def r in $b.rows) {
        $kids[] = rowNodePublic($r, $b.aligns, $cols);
    }
    $n.children = $kids;
    return $n;
}

func listToPublic(b as Block, depth as int) {
    def n as Node init nodeOf("list");
    $n.ordered = $b.ordered;
    def kids as list of Node init [];
    def i as int init 0;
    for (def itemText in $b.items) {
        def item as Node init nodeOf("item");
        def ikids as list of Node init inlineToPublic(parseInline($itemText));
        # The parallel `children` entry is a nested sub-list (an empty List when the
        # item has no nesting); attach it as a child of the item when non-empty.
        if ($i < len($b.children)) {
            def sub as Block init $b.children[$i];
            if (len($sub.items) > 0) {
                $ikids[] = blockToPublic($sub, $depth + 1);
            }
        }
        $item.children = $ikids;
        $kids[] = $item;
        $i = $i + 1;
    }
    $n.children = $kids;
    return $n;
}

# blockToPublic maps one internal Block to a public Node, recursing (under a depth
# cap) into a list's nested sub-lists and a quote's inner blocks. The `match` is over
# a Block, so the resolver checks it covers every BlockKind.
func blockToPublic(b as Block, depth as int) {
    if ($depth > MAX_DOC_DEPTH) {
        mdFail("document nesting too deep (over " + convert.toString(MAX_DOC_DEPTH) + " levels)");
    }
    match ($b.kind) {
        when Heading {
            def n as Node init nodeOf("heading");
            $n.level = $b.level;
            $n.children = inlineToPublic(parseInline($b.text));
            return $n;
        }
        when Paragraph {
            def n as Node init nodeOf("paragraph");
            $n.children = inlineToPublic(parseInline($b.text));
            return $n;
        }
        when Code {
            def n as Node init nodeOf("code");
            $n.text = $b.text;
            $n.lang = $b.lang;
            return $n;
        }
        when List {
            return listToPublic($b, $depth);
        }
        when Table {
            return tableToPublic($b);
        }
        when Quote {
            def n as Node init nodeOf("quote");
            def kids as list of Node init [];
            for (def cb in $b.children) {
                $kids[] = blockToPublic($cb, $depth + 1);
            }
            $n.children = $kids;
            return $n;
        }
    }
}

# countNodes sums the nodes in a subtree. The depth cap already bounds recursion
# depth, so this cannot itself overflow; it enforces the node budget.
func countNodes(n as Node) {
    def total as int init 1;
    for (def c in $n.children) {
        $total = $total + countNodes($c);
    }
    return $total;
}

/**
 * Parse Markdown into a document tree, walked by the reader accessors (`typeOf` /
 * `children` / `text` / `level` / `attr` / `get` / `findAll` / `has`) - so a caller
 * can inspect or transform a document (pull the headings for a table of contents,
 * rewrite links, convert to another format) before `render`ing it. The returned node
 * is the document root (its `typeOf` is `"document"`); its children are the top-level
 * blocks. A document that nests too deep or holds too many nodes is a catchable
 * `"markdown"` error.
 * @param md {string} the Markdown source
 * @return {Node} the document root node
 */
export func parse(md as string) {
    def kids as list of Node init [];
    for (def b in parseBlocks($md)) {
        $kids[] = blockToPublic($b, 1);
    }
    def doc as Node init nodeOf("document");
    $doc.children = $kids;
    if (countNodes($doc) > MAX_DOC_NODES) {
        mdFail("document too large (over " + convert.toString(MAX_DOC_NODES) + " nodes)");
    }
    return $doc;
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
    def out as string init "";
    for (def c in $node.children) {
        $out = $out + text($c);
    }
    return $out;
}

/**
 * A named string attribute of a node: `"href"` / `"title"` (a `link` / `image`),
 * `"lang"` (a `code` block), `"align"` (a table `cell`), `"ordered"` ("true" /
 * "false", a `list`), or `"level"` (a heading, as a string). Returns "" for an
 * absent attribute.
 * @param node {Node} the node
 * @param name {string} the attribute name
 * @return {string} the attribute value, or ""
 */
export func attr(node as Node, name as string) {
    match ($name) {
        when "href", "url" {
            return $node.url;
        }
        when "title" {
            return $node.title;
        }
        when "lang", "language" {
            return $node.lang;
        }
        when "align" {
            return $node.align;
        }
        when "ordered" {
            if ($node.ordered) {
                return "true";
            }
            return "false";
        }
        when "level" {
            return convert.toString($node.level);
        }
        else {
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
                $name = strings.substring($s, 0, $lb);
                def inner as string init strings.substring($s, $lb + 1, len($s) - 1);
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

# safeHref returns url unchanged when it is safe to place in an href - an
# http/https/mailto URL or a scheme-less relative reference - and "#"
# otherwise. It neutralizes script-executing schemes (javascript:, data:,
# vbscript:, ...) that untrusted markdown (a blog comment) could smuggle into a
# link. Whitespace and control characters are stripped before the scheme is
# read, because browsers ignore them inside a scheme (so "java\tscript:" and
# "  javascript:" would otherwise execute).
# safeHref returns a URL safe for an href (http / https / mailto, else "#").
# The scheme allowlist lives in html (html.safeUrl) so the anti-XSS policy
# has one home; this thin wrapper keeps the local call sites readable.
func safeHref(url as string) {
    return html.safeUrl($url);
}

func linkNode(text as string, url as string, title as string) {
    def attrs as list of html.Attr init [];
    $attrs[] = html.attr("href", safeHref($url));
    if (not ($title == "")) {
        $attrs[] = html.attr("title", $title);
    }
    def kids as list of html.Node init [];
    $kids[] = html.text($text);
    return html.element("a", $attrs, $kids);
}

# imageNode builds an `<img>` void element. The src runs through the same
# scheme allowlist as a link href (so `![x](javascript:...)` is neutralized),
# and the alt text is an escaped attribute.
func imageNode(alt as string, url as string, title as string) {
    def attrs as list of html.Attr init [];
    $attrs[] = html.attr("src", safeHref($url));
    $attrs[] = html.attr("alt", $alt);
    if (not ($title == "")) {
        $attrs[] = html.attr("title", $title);
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

/**
 * Render Markdown to an HTML string (block elements concatenated, no
 * indentation).
 * @param md {string} the Markdown source
 * @return {string} the rendered HTML
 */
export func toHtml(md as string) {
    return render(parse($md), "html");
}

# --- ANSI helpers shared by the node renderer ----------------------

# indentLines prefixes every line of s with `prefix`.
func indentLines(s as string, prefix as string) {
    def out as string init "";
    def first as bool init true;
    for (def line in strings.split($s, "\n")) {
        if (not $first) {
            $out = $out + "\n";
        }
        $first = false;
        $out = $out + $prefix + $line;
    }
    return $out;
}

# isWideRune approximates the Unicode East-Asian Width "W"/"F" categories plus
# the common emoji blocks - the code points a terminal renders two columns wide.
func isWideRune(cp as int) {
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
# instead of running one column short per wide rune.
func displayWidth(s as string) {
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

# widenAt grows column `c`'s width to at least `w` (columns past the header
# count are ignored).
func widenAt(widths as list of int, c as int, w as int) {
    if ($c < len($widths) and $w > $widths[$c]) {
        $widths[$c] = $w;
    }
    return $widths;
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
    def out as string init "";
    def i as int init 0;
    while ($i < len($widths)) {
        if ($i > 0) {
            $out = $out + "-+-";
        }
        $out = $out + strings.repeat("-", $widths[$i]);
        $i = $i + 1;
    }
    return $out;
}

/**
 * Render Markdown to styled terminal text, blocks separated by a blank line.
 * @param md {string} the Markdown source
 * @return {string} the rendered terminal text
 */
export func toAnsi(md as string) {
    return render(parse($md), "ansi");
}

# --- rendering from the public tree (exported) ---------------------
#
# `render` walks a public Node tree - a `parse` result or a hand-built one - so a
# document can be inspected / transformed, then rendered. `toHtml` / `toAnsi` are
# thin wrappers over `render(parse(md), ...)`, so build and parse share one model.

# inlineNodeToHtml maps one inline Node to an html node (the inline model is flat,
# so a `strong` / `emphasis` / `link` carries its text in a single `text` child).
func inlineNodeToHtml(n as Node) {
    match ($n.kind) {
        when "text" {
            return html.text($n.text);
        }
        when "codespan" {
            return wrapEl("code", $n.text);
        }
        when "strong" {
            return wrapEl("strong", text($n));
        }
        when "emphasis" {
            return wrapEl("em", text($n));
        }
        when "link" {
            return linkNode(text($n), $n.url, $n.title);
        }
        when "image" {
            return imageNode($n.text, $n.url, $n.title);
        }
        else {
            return html.text(text($n));
        }
    }
}

func inlineNodesToHtml(ns as list of Node) {
    def out as list of html.Node init [];
    for (def c in $ns) {
        $out[] = inlineNodeToHtml($c);
    }
    return $out;
}

# listNodeToHtml renders a `list` node: each `item`'s children are inline nodes plus
# an optional nested `list` (rendered as a child `<ul>` / `<ol>` inside the `<li>`).
func listNodeToHtml(n as Node) {
    def lis as list of html.Node init [];
    for (def item in $n.children) {
        def kids as list of html.Node init [];
        for (def c in $item.children) {
            if ($c.kind == "list") {
                $kids[] = nodeToHtml($c);
            } else {
                $kids[] = inlineNodeToHtml($c);
            }
        }
        $lis[] = html.element("li", [], $kids);
    }
    if ($n.ordered) {
        return html.element("ol", [], $lis);
    }
    return html.element("ul", [], $lis);
}

# tableCellToHtml builds a `th` / `td` with the cell's inline content and an `align`
# attribute (omitted when unset / "none").
func tableCellToHtml(tag as string, cell as Node) {
    def attrs as list of html.Attr init [];
    if (not ($cell.align == "none") and not ($cell.align == "")) {
        $attrs[] = html.attr("align", $cell.align);
    }
    return html.element($tag, $attrs, inlineNodesToHtml($cell.children));
}

func tableRowToHtml(tag as string, row as Node) {
    def tds as list of html.Node init [];
    for (def cell in $row.children) {
        $tds[] = tableCellToHtml($tag, $cell);
    }
    return html.element("tr", [], $tds);
}

func tableNodeToHtml(n as Node) {
    def head as list of html.Node init [];
    def body as list of html.Node init [];
    def r as int init 0;
    for (def row in $n.children) {
        if ($r == 0) {
            $head[] = tableRowToHtml("th", $row);
        } else {
            $body[] = tableRowToHtml("td", $row);
        }
        $r = $r + 1;
    }
    def parts as list of html.Node init [];
    $parts[] = html.element("thead", [], $head);
    $parts[] = html.element("tbody", [], $body);
    return html.element("table", [], $parts);
}

# nodeToHtml renders one block Node to an html node.
func nodeToHtml(n as Node) {
    match ($n.kind) {
        when "heading" {
            def tag as string init "h" + convert.toString($n.level);
            return html.element($tag, [], inlineNodesToHtml($n.children));
        }
        when "paragraph" {
            return html.element("p", [], inlineNodesToHtml($n.children));
        }
        when "code" {
            def codeKids as list of html.Node init [];
            $codeKids[] = html.text($n.text);
            def pre as list of html.Node init [];
            $pre[] = html.element("code", [], $codeKids);
            return html.element("pre", [], $pre);
        }
        when "list" {
            return listNodeToHtml($n);
        }
        when "table" {
            return tableNodeToHtml($n);
        }
        when "quote" {
            def kids as list of html.Node init [];
            for (def cb in $n.children) {
                $kids[] = nodeToHtml($cb);
            }
            return html.element("blockquote", [], $kids);
        }
        else {
            return inlineNodeToHtml($n);
        }
    }
}

# inlineNodeToAnsi maps one inline Node to terminal-styled text (styling suppresses
# itself when stdout is not a TTY, so piped output is plain).
func inlineNodeToAnsi(n as Node) {
    match ($n.kind) {
        when "text" {
            return $n.text;
        }
        when "codespan" {
            return ansi.cyan($n.text);
        }
        when "strong" {
            return ansi.bold(text($n));
        }
        when "emphasis" {
            return ansi.italic(text($n));
        }
        when "link" {
            return ansi.underline(text($n)) + " (" + $n.url + ")";
        }
        when "image" {
            return ansi.dim("[image] ") + $n.text + " (" + $n.url + ")";
        }
        else {
            return text($n);
        }
    }
}

func inlineChildrenToAnsi(ns as list of Node) {
    def out as string init "";
    for (def c in $ns) {
        $out = $out + inlineNodeToAnsi($c);
    }
    return $out;
}

# cellVisWidthNode is the visible terminal width of a cell (styling stripped).
func cellVisWidthNode(cell as Node) {
    return displayWidth(ansi.strip(inlineChildrenToAnsi($cell.children)));
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
            $widths = widenAt($widths, $c, cellVisWidthNode($cell));
            $c = $c + 1;
        }
        $r = $r + 1;
    }
    return $widths;
}

# ansiRowNode renders one ` | `-separated row padded to the column widths; the
# header row is bold.
func ansiRowNode(row as Node, widths as list of int, bold as bool) {
    def out as string init "";
    def i as int init 0;
    while ($i < len($widths)) {
        if ($i > 0) {
            $out = $out + " | ";
        }
        def styled as string init "";
        def vis as int init 0;
        def align as string init "none";
        if ($i < len($row.children)) {
            def cell as Node init $row.children[$i];
            $styled = inlineChildrenToAnsi($cell.children);
            $vis = displayWidth(ansi.strip($styled));
            $align = $cell.align;
        }
        if ($bold) {
            $styled = ansi.bold($styled);
        }
        $out = $out + padCell($styled, $vis, $widths[$i], $align);
        $i = $i + 1;
    }
    return $out;
}

func tableNodeToAnsi(n as Node) {
    # A table always carries its header row from `parse`; a hand-built table with no
    # rows renders as empty rather than indexing past the end.
    if (len($n.children) == 0) {
        return "";
    }
    def widths as list of int init colWidthsNode($n);
    def out as string init ansiRowNode($n.children[0], $widths, true);
    $out = $out + "\n" + ansiDivider($widths);
    def r as int init 1;
    while ($r < len($n.children)) {
        $out = $out + "\n" + ansiRowNode($n.children[$r], $widths, false);
        $r = $r + 1;
    }
    return $out;
}

# listNodeToAnsi renders a `list` node at nesting `depth` (2 spaces of indent per
# level), recursing into each item's nested sub-list under it.
func listNodeToAnsi(n as Node, depth as int) {
    def out as string init "";
    def idx as int init 1;
    def first as bool init true;
    def pad as string init strings.repeat("  ", $depth + 1);
    for (def item in $n.children) {
        if (not $first) {
            $out = $out + "\n";
        }
        $first = false;
        def marker as string init "- ";
        if ($n.ordered) {
            $marker = convert.toString($idx) + ". ";
        }
        def inlineStr as string init "";
        def nested as string init "";
        for (def c in $item.children) {
            if ($c.kind == "list") {
                $nested = "\n" + listNodeToAnsi($c, $depth + 1);
            } else {
                $inlineStr = $inlineStr + inlineNodeToAnsi($c);
            }
        }
        $out = $out + $pad + $marker + $inlineStr + $nested;
        $idx = $idx + 1;
    }
    return $out;
}

# nodeToAnsi renders one block Node to terminal text.
func nodeToAnsi(n as Node) {
    match ($n.kind) {
        when "table" {
            return tableNodeToAnsi($n);
        }
        when "heading" {
            return ansi.bold(inlineChildrenToAnsi($n.children));
        }
        when "code" {
            return ansi.dim(indentLines($n.text, "    "));
        }
        when "list" {
            return listNodeToAnsi($n, 0);
        }
        when "quote" {
            def inner as string init "";
            def first as bool init true;
            for (def cb in $n.children) {
                if (not $first) {
                    $inner = $inner + "\n";
                }
                $first = false;
                $inner = $inner + nodeToAnsi($cb);
            }
            return indentLines($inner, ansi.dim("> "));
        }
        when "paragraph" {
            return inlineChildrenToAnsi($n.children);
        }
        else {
            return inlineChildrenToAnsi($n.children);
        }
    }
}

/**
 * Render a document tree (a `parse` result, or a hand-built one) to a string.
 * `format` is `"html"` (block elements concatenated, no indentation) or `"ansi"`
 * (styled terminal text, blocks separated by a blank line). A `"document"` node
 * renders its children; any other node renders as a single block. An unknown format
 * is a catchable `"markdown"` error.
 * @param doc {Node} the document (or block) node to render
 * @param format {string} "html" or "ansi"
 * @return {string} the rendered document
 */
export func render(doc as Node, format as string) {
    def blocks as list of Node init $doc.children;
    if (not ($doc.kind == "document")) {
        $blocks = [$doc];
    }
    if ($format == "html") {
        def nodes as list of html.Node init [];
        for (def c in $blocks) {
            $nodes[] = nodeToHtml($c);
        }
        return html.renderAll($nodes);
    }
    if ($format == "ansi") {
        def out as string init "";
        def first as bool init true;
        for (def c in $blocks) {
            if (not $first) {
                $out = $out + "\n\n";
            }
            $first = false;
            $out = $out + nodeToAnsi($c);
        }
        return $out;
    }
    mdFail("unknown render format \"" + $format + "\" (want \"html\" or \"ansi\")");
}

# --- authoring: build Markdown source (exported) -------------------
#
# These produce Markdown *text*, the inverse of toHtml / toAnsi, so a program
# can assemble a document and (round-trip) render it. The text is inserted
# literally: a caller passing Markdown metacharacters is responsible for
# escaping them.

# fail raises a catchable value error from an authoring helper.
func fail(msg as string) {
    throw Error{kind: "value", message: $msg, file: "", line: 0, col: 0};
}

# headerLevel maps an "h1".."h6" tag to its heading depth, or throws.
func headerLevel(level as string) {
    if ($level == "h1") {
        return 1;
    }
    if ($level == "h2") {
        return 2;
    }
    if ($level == "h3") {
        return 3;
    }
    if ($level == "h4") {
        return 4;
    }
    if ($level == "h5") {
        return 5;
    }
    if ($level == "h6") {
        return 6;
    }
    fail("markdown.header: level must be h1..h6, got " + $level);
}

/**
 * Render an ATX heading.
 * @param level {string} the heading depth, "h1".."h6"
 * @param text {string} the heading text
 * @return {string} the Markdown heading line
 * @throws {Error} when level is not "h1".."h6"
 */
export func header(level as string, text as string) {
    return strings.repeat("#", headerLevel($level)) + " " + $text;
}

/**
 * Wrap text in an inline emphasis span.
 * @param kind {string} the emphasis, "bold" / "italic" / "code"
 * @param text {string} the text to wrap
 * @return {string} the emphasised Markdown span
 * @throws {Error} when kind is not "bold" / "italic" / "code"
 */
export func style(kind as string, text as string) {
    match ($kind) {
        when "bold" { return "**" + $text + "**"; }
        when "italic" { return "*" + $text + "*"; }
        when "code" { return "`" + $text + "`"; }
        else { fail("markdown.style: kind must be bold|italic|code, got " + $kind); }
    }
}

/**
 * Render an inline link `[text](url)`.
 * @param text {string} the link text
 * @param url {string} the link target
 * @return {string} the Markdown link
 */
export func link(text as string, url as string) {
    return "[" + $text + "](" + $url + ")";
}

/**
 * Render an unordered list, one `- item` per line.
 * @param items {list of string} the list items
 * @return {string} the Markdown bullet list
 */
export func bullets(items as list of string) {
    def out as string init "";
    def first as bool init true;
    for (def item in $items) {
        if (not $first) {
            $out = $out + "\n";
        }
        $first = false;
        $out = $out + "- " + $item;
    }
    return $out;
}

/**
 * Render an ordered list, `1. item` upward.
 * @param items {list of string} the list items
 * @return {string} the Markdown numbered list
 */
export func numbered(items as list of string) {
    def out as string init "";
    def i as int init 1;
    def first as bool init true;
    for (def item in $items) {
        if (not $first) {
            $out = $out + "\n";
        }
        $first = false;
        $out = $out + convert.toString($i) + ". " + $item;
        $i = $i + 1;
    }
    return $out;
}

/**
 * Render a fenced code block around verbatim text.
 * @param text {string} the verbatim code
 * @return {string} the fenced Markdown code block
 */
export func codeBlock(text as string) {
    return "```\n" + $text + "\n```";
}

# cellText makes a string safe inside a table cell: a pipe is escaped and a
# newline (which would break the row) becomes a space.
func cellText(s as string) {
    def out as string init strings.replace($s, "|", "\\|");
    return strings.replace($out, "\n", " ");
}

# tableRow renders one `| a | b |` row, padded / truncated to `cols` columns.
func tableRow(cells as list of string, cols as int) {
    def out as string init "|";
    def c as int init 0;
    while ($c < $cols) {
        def v as string init "";
        if ($c < len($cells)) {
            $v = $cells[$c];
        }
        $out = $out + " " + cellText($v) + " |";
        $c = $c + 1;
    }
    return $out;
}

# alignSep renders one column's separator cell from its alignment.
func alignSep(a as string) {
    if ($a == "left") {
        return ":---";
    }
    if ($a == "right") {
        return "---:";
    }
    if ($a == "center") {
        return ":---:";
    }
    if ($a == "" or $a == "none") {
        return "---";
    }
    fail("markdown.table: align must be left|right|center|none, got " + $a);
}

# alignRow renders the `| :--- | ---: |` separator row under the header.
func alignRow(aligns as list of string, cols as int) {
    def out as string init "|";
    def c as int init 0;
    while ($c < $cols) {
        def a as string init "";
        if ($c < len($aligns)) {
            $a = $aligns[$c];
        }
        $out = $out + " " + alignSep($a) + " |";
        $c = $c + 1;
    }
    return $out;
}

/**
 * Render a GFM table. Columns follow `headings`: a short row is padded with
 * empty cells, extra cells are dropped. Pipes and newlines in a cell are made
 * safe.
 * @param headings {list of string} the column headings
 * @param aligns {list of string} the per-column alignment ("left" / "right" / "center" / "none"; `[]` for all-default)
 * @param rows {list of list of string} the data rows, each a list of cell strings
 * @return {string} the GFM table source
 * @throws {Error} when an align value is not "left" / "right" / "center" / "none"
 */
export func table(
    headings as list of string,
    aligns as list of string,
    rows as list of list of string) {
    def cols as int init len($headings);
    def out as string init tableRow($headings, $cols);
    $out = $out + "\n" + alignRow($aligns, $cols);
    for (def row in $rows) {
        $out = $out + "\n" + tableRow($row, $cols);
    }
    return $out;
}

# --- prettify: align table source columns (exported) ---------------

# maxInt is the larger of two ints.
func maxInt(a as int, b as int) {
    if ($a > $b) {
        return $a;
    }
    return $b;
}

# srcLen is a cell's re-emitted source width (pipes re-escaped, newline to
# space).
func srcLen(cell as string) {
    return len(cellText($cell));
}

# sourceWidths is the per-column source width: the widest cell, at least 3 (so
# the delimiter has room for a colon and dashes).
func sourceWidths(b as Block, cols as int) {
    def widths as list of int init [];
    def i as int init 0;
    while ($i < $cols) {
        def w as int init 3;
        if ($i < len($b.headings)) {
            $w = maxInt($w, srcLen($b.headings[$i]));
        }
        for (def row in $b.rows) {
            if ($i < len($row)) {
                $w = maxInt($w, srcLen($row[$i]));
            }
        }
        $widths[] = $w;
        $i = $i + 1;
    }
    return $widths;
}

# delimCell renders one aligned delimiter cell filling `width`.
func delimCell(width as int, align as string) {
    if ($align == "left") {
        return ":" + strings.repeat("-", $width - 1);
    }
    if ($align == "right") {
        return strings.repeat("-", $width - 1) + ":";
    }
    if ($align == "center") {
        return ":" + strings.repeat("-", $width - 2) + ":";
    }
    return strings.repeat("-", $width);
}

# prettyRow renders one padded `| a | b |` source row to the column widths.
func prettyRow(
    cells as list of string,
    widths as list of int,
    aligns as list of string,
    cols as int) {
    def out as string init "|";
    def i as int init 0;
    while ($i < $cols) {
        def text as string init "";
        if ($i < len($cells)) {
            $text = cellText($cells[$i]);
        }
        $out = $out + " " + padCell($text, len($text), $widths[$i], alignOf($aligns, $i)) + " |";
        $i = $i + 1;
    }
    return $out;
}

# prettyDelim renders the padded `| :--- | ---: |` delimiter row.
func prettyDelim(widths as list of int, aligns as list of string, cols as int) {
    def out as string init "|";
    def i as int init 0;
    while ($i < $cols) {
        $out = $out + " " + delimCell($widths[$i], alignOf($aligns, $i)) + " |";
        $i = $i + 1;
    }
    return $out;
}

# prettyTableAt reformats the table at line `i` into aligned source lines.
func prettyTableAt(lines as list of string, i as int) {
    def ts as TableScan init tableFrom($lines, $i);
    def b as Block init $ts.block;
    def cols as int init len($b.headings);
    def widths as list of int init sourceWidths($b, $cols);
    def out as list of string init [];
    $out[] = prettyRow($b.headings, $widths, $b.aligns, $cols);
    $out[] = prettyDelim($widths, $b.aligns, $cols);
    for (def row in $b.rows) {
        $out[] = prettyRow($row, $widths, $b.aligns, $cols);
    }
    return TablePretty{lines: $out, next: $ts.next};
}

/**
 * Reformat every GFM table in Markdown text so its source columns line up
 * (padded cells, aligned delimiters), leaving all other lines exactly as
 * written. The handcraft-then-prettify workflow, in one call.
 * @param md {string} the Markdown source
 * @return {string} the source with its tables aligned
 */
export func tablePretty(md as string) {
    def lines as list of string init strings.split($md, "\n");
    def out as list of string init [];
    def n as int init len($lines);
    def i as int init 0;
    while ($i < $n) {
        if (looksLikeTable($lines, $i)) {
            def tp as TablePretty init prettyTableAt($lines, $i);
            for (def line in $tp.lines) {
                $out[] = $line;
            }
            $i = $tp.next;
            continue;
        }
        $out[] = $lines[$i];
        $i = $i + 1;
    }
    return strings.join($out, "\n");
}
