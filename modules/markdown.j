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
import "./pdf.j" as pdf;
use strings;
use regex;
use convert;
use lists;
use math;

# The inline span kinds and block kinds, as sum types: the renderers `match` on
# them, so adding a kind surfaces every place that must handle it (both the HTML
# and the ANSI path) instead of silently falling through a string compare.
def enum SpanKind { Text, Code, Strong, Em, Link, Image };
def enum BlockKind { Paragraph, Heading, Code, List, Table, Quote, Rule, Html };

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
func ruleBlock() {
    return Block{
        kind: BlockKind.Rule,
        level: 0,
        text: "",
        lang: "",
        ordered: false,
        items: [],
        headings: [],
        aligns: [],
        rows: [],
        children: []
    };
}
func htmlBlock(text as string) {
    return Block{
        kind: BlockKind.Html,
        level: 0,
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
# matchLinkLabel finds the `]` that closes a link / image label opened by the `[`
# at `open`, balancing nested `[...]` and skipping code spans (a `]` inside
# backticks is not a delimiter), so a label that mentions bracket syntax
# (`$xs[]`, `argv[0]`) or contains a nested bracket pair parses correctly.
# Returns the index of the closing `]`, or -1 if the label never closes.
func matchLinkLabel(cs as list of string, open as int, n as int) {
    def depth as int init 1;
    def i as int init $open + 1;
    while ($i < $n) {
        def c as string init $cs[$i];
        if ($c == "`") {
            # Skip a code span so brackets inside it are literal.
            def j as int init $i + 1;
            def closed as bool init false;
            while ($j < $n) {
                if ($cs[$j] == "`") {
                    $closed = true;
                    break;
                }
                $j = $j + 1;
            }
            if ($closed) {
                $i = $j + 1;
                continue;
            }
        } elseif ($c == "[") {
            $depth = $depth + 1;
        } elseif ($c == "]") {
            $depth = $depth - 1;
            if ($depth == 0) {
                return $i;
            }
        }
        $i = $i + 1;
    }
    return -1;
}

# isAutolinkUri reports whether an angle-bracket span is a URI autolink: an RFC
# 3986 scheme (letter then letters / digits / `+` / `.` / `-`) followed by `:`.
# The caller has already established there is no whitespace or `<` inside.
func isAutolinkUri(inner as string) {
    return regex.matches("^[a-zA-Z][a-zA-Z0-9+.\\-]*:", $inner);
}

# isAutolinkEmail reports whether an angle-bracket span is an email autolink
# (`user@host.tld`), rendered as a `mailto:` link.
func isAutolinkEmail(inner as string) {
    return regex.matches("^[^@ \t]+@[^@ \t]+\\.[^@ \t]+$", $inner);
}

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
            def kb as int init matchLinkLabel($cs, $i + 1, $n);
            def kp as int init -1;
            if ($kb >= 0 and $kb + 1 < $n and $cs[$kb + 1] == "(") {
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
        # link: [text](url) - the label is matched with a bracket counter (so a
        # `]` inside the label or a code span does not end it), the `)` via the
        # precomputed array.
        def linkEnd as int init -1;
        def rb as int init -1;
        def rp as int init -1;
        if ($c == "[") {
            def kb as int init matchLinkLabel($cs, $i, $n);
            def kp as int init -1;
            if ($kb >= 0 and $kb + 1 < $n and $cs[$kb + 1] == "(") {
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
        # autolink: <scheme:...> or <user@host> - a `<` with no whitespace to the
        # next `>` and either a URI scheme or an email inside.
        def autoEnd as int init -1;
        def autoUrl as string init "";
        def autoText as string init "";
        if ($c == "<") {
            def j as int init $i + 1;
            def ok as bool init true;
            while ($j < $n) {
                def cj as string init $cs[$j];
                if ($cj == ">") {
                    break;
                }
                if ($cj == "<" or $cj == " " or $cj == "\t") {
                    $ok = false;
                    break;
                }
                $j = $j + 1;
            }
            if ($ok and $j < $n and $cs[$j] == ">") {
                def inner as string init strings.join(lists.slice($cs, $i + 1, $j), "");
                if (isAutolinkUri($inner)) {
                    $autoUrl = $inner;
                    $autoText = $inner;
                    $autoEnd = $j + 1;
                } elseif (isAutolinkEmail($inner)) {
                    $autoUrl = "mailto:" + $inner;
                    $autoText = $inner;
                    $autoEnd = $j + 1;
                }
            }
        }
        if ($autoEnd >= 0) {
            if ($i > $bufStart) {
                $spans[] = span(SpanKind.Text, strings.join(lists.slice($cs, $bufStart, $i), ""), "");
            }
            $spans[] = Span{kind: SpanKind.Link, text: $autoText, url: $autoUrl, title: ""};
            $i = $autoEnd;
            $bufStart = $i;
            continue;
        }
        $i = $i + 1;
    }
    if ($n > $bufStart) {
        $spans[] = span(SpanKind.Text, strings.join(lists.slice($cs, $bufStart, $n), ""), "");
    }
    # Spans are returned raw (entities not yet decoded); `spanToPublic` decodes at
    # the leaves so a nested re-parse never double-decodes, and Code spans keep
    # their entities literal (CommonMark keeps entities raw in code).
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
# isThematicBreak reports whether a line is a thematic break: three or more of the
# same `-`, `*`, or `_`, separated only by spaces / tabs (CommonMark). Tested before
# the list rule so `- - -` reads as a rule, not three empty items.
func isThematicBreak(trimmed as string) {
    def compact as string init strings.replace(strings.replace($trimmed, " ", ""), "\t", "");
    if (len($compact) < 3) {
        return false;
    }
    def first as string init strings.substring($compact, 0, 1);
    if (not ($first == "-" or $first == "*" or $first == "_")) {
        return false;
    }
    for (def ch in strings.chars($compact)) {
        if (not ($ch == $first)) {
            return false;
        }
    }
    return true;
}

# isHtmlBlockOpen reports whether a line opens a raw HTML block: `<` (after up to a
# little leading space) followed by `!` (declaration / comment), `/` (closing tag),
# or a tag name whose next character is whitespace, `>`, or `/`. The tag-name test
# deliberately fails on `:` so an autolink (`<https://...>`) is not mistaken for a
# block - it stays inline prose (see the autolink scanner).
func isHtmlBlockOpen(line as string) {
    def t as string init strings.trimLeft($line);
    if (not strings.startsWith($t, "<") or len($t) < 2) {
        return false;
    }
    def cs as list of string init strings.chars($t);
    def c2 as string init $cs[1];
    if ($c2 == "!" or $c2 == "/") {
        return true;
    }
    if (not isAsciiLetter($c2)) {
        return false;
    }
    def i as int init 1;
    def n as int init len($cs);
    while ($i < $n and isTagNameChar($cs[$i])) {
        $i = $i + 1;
    }
    if ($i >= $n) {
        return true;
    }
    def nx as string init $cs[$i];
    return $nx == " " or $nx == "\t" or $nx == ">" or $nx == "/";
}

# isAsciiLetter / isTagNameChar classify the characters of an HTML tag name.
func isAsciiLetter(c as string) {
    return ($c >= "a" and $c <= "z") or ($c >= "A" and $c <= "Z");
}
func isTagNameChar(c as string) {
    return isAsciiLetter($c) or ($c >= "0" and $c <= "9") or $c == "-";
}

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
    if (isThematicBreak($trimmed)) {
        return "rule";
    }
    if (isHtmlBlockOpen($line)) {
        return "html";
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
    text as string,
    contentCol as int
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
        return ItemInfo{indent: -1, ordered: false, text: "", contentCol: -1};
    }
    # The content column (in characters) is where the item text begins, i.e. the
    # whole line minus its text suffix - the indent a soft-wrapped continuation
    # line must reach to belong to this item.
    return ItemInfo{
        indent: leadingWidth($line),
        ordered: strings.endsWith($m.groups[1], "."),
        text: $m.groups[2],
        contentCol: len($line) - len($m.groups[2])
    };
}

# leadCharCount counts the leading space / tab characters of a line - the
# character-column at which its content starts, matched against an item's
# `contentCol` to decide a lazy continuation.
func leadCharCount(line as string) {
    def w as int init 0;
    for (def ch in strings.chars($line)) {
        if ($ch == " " or $ch == "\t") {
            $w = $w + 1;
        } else {
            return $w;
        }
    }
    return $w;
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
                break;
            }
            # A non-blank line indented to the item's content column is a lazy
            # continuation of the current item's text (a soft-wrapped line), not a
            # new block - absorb it so one item does not split into two lists with a
            # stranded paragraph. A blank line above already ended the item, which
            # is what keeps an indented code block out. A fence / heading / quote
            # (non-"plain") still ends the list.
            if (len($items) > 0 and leadCharCount($lines[$j]) >= $base.contentCol and
                lineType(strings.trim($lines[$j]), $lines[$j]) == "plain") {
                def last as int init len($items) - 1;
                $items[$last] = $items[$last] + " " + strings.trim($lines[$j]);
                $j = $j + 1;
                continue;
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
        # Indented code block: a non-blank line indented four or more columns that
        # does not continue an open paragraph (the document start, or a blank line,
        # precedes it - which is what distinguishes it from a lazy continuation).
        if (len($para) == 0 and len(strings.trim($line)) > 0 and leadingWidth($line) >= 4) {
            def ics as BlockScan init collectIndentedCode($lines, $i);
            $blocks[] = $ics.block;
            $i = $ics.next;
            continue;
        }
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
        if ($lt == "rule") {
            $blocks[] = ruleBlock();
            $i = $i + 1;
            continue;
        }
        if ($lt == "html") {
            def hs as BlockScan init collectHtml($lines, $i);
            $blocks[] = $hs.block;
            $i = $hs.next;
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
# stripCodeIndent removes up to four leading columns (four spaces, or one tab) from
# an indented-code line, preserving the rest of its whitespace verbatim.
func stripCodeIndent(line as string) {
    def cs as list of string init strings.chars($line);
    def removed as int init 0;
    def i as int init 0;
    def n as int init len($cs);
    while ($i < $n and $removed < 4) {
        if ($cs[$i] == " ") {
            $removed = $removed + 1;
            $i = $i + 1;
        } elseif ($cs[$i] == "\t") {
            $removed = $removed + 4;
            $i = $i + 1;
        } else {
            break;
        }
    }
    return strings.join(lists.slice($cs, $i, $n), "");
}

# collectIndentedCode gathers a run of four-space-indented lines into a code block,
# stripping the four-column indent from each. Interior blank lines are kept when a
# still-indented line follows; a non-indented non-blank line, or a trailing blank
# run, ends the block.
func collectIndentedCode(lines as list of string, start as int) {
    def out as list of string init [];
    def n as int init len($lines);
    def j as int init $start;
    while ($j < $n) {
        if (len(strings.trim($lines[$j])) == 0) {
            def k as int init $j + 1;
            while ($k < $n and len(strings.trim($lines[$k])) == 0) {
                $k = $k + 1;
            }
            if ($k < $n and leadingWidth($lines[$k]) >= 4) {
                $out[] = "";
                $j = $j + 1;
                continue;
            }
            break;
        }
        if (leadingWidth($lines[$j]) < 4) {
            break;
        }
        $out[] = stripCodeIndent($lines[$j]);
        $j = $j + 1;
    }
    return BlockScan{block: codeBlockNode(strings.join($out, "\n"), ""), next: $j};
}

# collectHtml gathers a raw HTML block: every line from the opener up to (not
# including) the next blank line, kept verbatim.
func collectHtml(lines as list of string, start as int) {
    def out as list of string init [];
    def n as int init len($lines);
    def j as int init $start;
    while ($j < $n and len(strings.trim($lines[$j])) > 0) {
        $out[] = $lines[$j];
        $j = $j + 1;
    }
    return BlockScan{block: htmlBlock(strings.join($out, "\n")), next: $j};
}

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

# inlineChildren re-parses the inner text of a styled span (strong / emphasis /
# link label) and returns the resulting inline nodes, so markup nests: a link
# inside `**...**`, an `*emphasis*` inside a link label, code inside bold. The
# recursion strictly shrinks the text at each level (the delimiters are gone), and
# `depth` bounds it as a safety net - at the floor the remaining text is emitted
# flat, so the walk always terminates.
func inlineChildren(text as string, depth as int) {
    if ($depth <= 0) {
        return [textNode(html.unescape($text))];
    }
    def out as list of Node init [];
    for (def sp in parseInline($text)) {
        $out[] = spanToPublic($sp, $depth - 1);
    }
    return $out;
}

# spanToPublic maps one inline Span to a public inline Node, decoding HTML entities
# at the leaves (text, and a link / image url + title) and keeping Code spans
# literal (CommonMark keeps entities raw in code). Strong / emphasis / link content
# is re-parsed via `inlineChildren` so inline markup nests. The `match` is over a
# Span, so the resolver checks it covers every SpanKind.
func spanToPublic(sp as Span, depth as int) {
    match ($sp.kind) {
        when Text {
            return textNode(html.unescape($sp.text));
        }
        when Code {
            def n as Node init nodeOf("codespan");
            $n.text = $sp.text;
            return $n;
        }
        when Strong {
            def n as Node init nodeOf("strong");
            $n.children = inlineChildren($sp.text, $depth);
            return $n;
        }
        when Em {
            def n as Node init nodeOf("emphasis");
            $n.children = inlineChildren($sp.text, $depth);
            return $n;
        }
        when Link {
            def n as Node init nodeOf("link");
            $n.url = html.unescape($sp.url);
            $n.title = html.unescape($sp.title);
            $n.children = inlineChildren($sp.text, $depth);
            return $n;
        }
        when Image {
            def n as Node init nodeOf("image");
            $n.url = html.unescape($sp.url);
            $n.title = html.unescape($sp.title);
            $n.text = html.unescape($sp.text);
            return $n;
        }
    }
}

# INLINE_MAX_DEPTH bounds inline nesting (bold in a link in emphasis ...); deeper
# markup is emitted as flat text. Well above any hand-written document's nesting.
def const INLINE_MAX_DEPTH as int init 8;

func inlineToPublic(spans as list of Span) {
    def out as list of Node init [];
    for (def sp in $spans) {
        $out[] = spanToPublic($sp, INLINE_MAX_DEPTH);
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
        when Rule {
            return nodeOf("thematic_break");
        }
        when Html {
            # A lone `<!-- pagebreak -->` comment is a page-break directive; any
            # other raw HTML block passes through as an html_block node.
            if (strings.trim($b.text) == "<!-- pagebreak -->") {
                return nodeOf("page_break");
            }
            def n as Node init nodeOf("html_block");
            $n.text = $b.text;
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
 * A page-break node. Placed in a document tree (typically between the blocks of a
 * hand-assembled book), it makes `renderPdf` / `toPdf` start the following content
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
            return html.element("strong", [], inlineNodesToHtml($n.children));
        }
        when "emphasis" {
            return html.element("em", [], inlineNodesToHtml($n.children));
        }
        when "link" {
            def attrs as list of html.Attr init [];
            $attrs[] = html.attr("href", safeHref($n.url));
            if (not ($n.title == "")) {
                $attrs[] = html.attr("title", $n.title);
            }
            return html.element("a", $attrs, inlineNodesToHtml($n.children));
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
        when "thematic_break" {
            def noKids as list of html.Node init [];
            return html.element("hr", [], $noKids);
        }
        when "html_block" {
            return html.raw($n.text);
        }
        when "page_break" {
            # A page break has no HTML representation; emit nothing.
            return html.raw("");
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
            return ansi.bold(inlineChildrenToAnsi($n.children));
        }
        when "emphasis" {
            return ansi.italic(inlineChildrenToAnsi($n.children));
        }
        when "link" {
            return ansi.underline(inlineChildrenToAnsi($n.children)) + " (" + $n.url + ")";
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
        when "thematic_break" {
            return ansi.dim(strings.repeat("-", 40));
        }
        when "html_block" {
            # Raw HTML has no terminal representation; drop it.
            return "";
        }
        when "page_break" {
            return "";
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
    producer as string
};

/**
 * The default options: US Letter, 54-point margins, the Helvetica family for body /
 * bold / italic / headings, Courier for code, 11-point body. Copy and tweak fields
 * (value semantics) to customise.
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
        producer: ""
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
    font as string
};

# The running layout: the accumulated Document, the Page being filled, the pen `y`
# (the top of the next line, decreasing down the page), and the current left `x` /
# content `width` (widened / narrowed by lists and quotes).
def struct Layout {
    doc as pdf.Document,
    page as pdf.Page,
    y as int,
    x as int,
    width as int,
    opts as PdfOptions
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

func newLayout(opts as PdfOptions) {
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
    return Layout{
        doc: $doc,
        page: pdf.page($opts.pageWidth, $opts.pageHeight),
        y: $opts.pageHeight - $opts.margin,
        x: $opts.margin,
        width: $opts.pageWidth - 2 * $opts.margin,
        opts: $opts
    };
}

# flushPage finalises the current page into the document and starts a fresh one,
# preserving the current indent (x / width) so a block split across a page break
# keeps its column.
func flushPage(state as Layout) {
    $state.doc = pdf.addPage($state.doc, $state.page);
    $state.page = pdf.page($state.opts.pageWidth, $state.opts.pageHeight);
    $state.y = $state.opts.pageHeight - $state.opts.margin;
    return $state;
}

# ensureSpace starts a new page when `needed` points would not fit above the bottom
# margin - but only when the page already holds content, so an over-tall single line
# still gets placed (overflow) instead of looping.
func ensureSpace(state as Layout, needed as int) {
    def top as int init $state.opts.pageHeight - $state.opts.margin;
    if ($state.y - $needed < $state.opts.margin and $state.y < $top) {
        return flushPage($state);
    }
    return $state;
}

# --- inline layout (styled word wrapping) --------------------------

# fontForInline picks the font for one inline node's kind (text / link / image ->
# body, strong -> bold, emphasis -> italic, codespan -> mono).
func fontForInline(node as Node, opts as PdfOptions) {
    match (typeOf($node)) {
        when "strong" { return $opts.boldFont; }
        when "emphasis" { return $opts.italicFont; }
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
    return text($node);
}

# inlineWords flattens inline nodes into styled words (split on spaces), each tagged
# with the font its run renders in.
func inlineWords(nodes as list of Node, opts as PdfOptions) {
    def out as list of IWord init [];
    for (def n in $nodes) {
        def f as string init fontForInline($n, $opts);
        def t as string init mdSanitize($opts, inlineText($n));
        for (def w in strings.split($t, " ")) {
            if (len($w) > 0) {
                $out[] = IWord{text: $w, font: $f};
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
        def ww as float init pdf.measureText($w.font, $size, $w.text);
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

# placeInlineLines draws already-packed styled lines at the current pen, advancing y
# and paginating per line.
func placeInlineLines(state as Layout, lines as list of list of IWord, size as int) {
    def lh as int init lineH($size);
    def sp as int init roundPt(pdf.measureText($state.opts.bodyFont, $size, " "));
    for (def line in $lines) {
        $state = ensureSpace($state, $lh);
        def baseline as int init $state.y - $size;
        def cx as int init $state.x;
        def first as bool init true;
        for (def w in $line) {
            if (not $first) {
                $cx = $cx + $sp;
            }
            $first = false;
            $state.page = pdf.text($state.page, $cx, $baseline, $w.font, $size, $w.text);
            $cx = $cx + roundPt(pdf.measureText($w.font, $size, $w.text));
        }
        $state.y = $state.y - $lh;
    }
    return $state;
}

# placePlainLines draws single-font wrapped lines (headings, code) at the current pen.
func placePlainLines(state as Layout, lines as list of string, font as string, size as int, xOffset as int) {
    def lh as int init lineH($size);
    for (def line in $lines) {
        $state = ensureSpace($state, $lh);
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
    def htext as string init mdSanitize($state.opts, text($node));
    def lines as list of string init pdf.wrapText($state.opts.headingFont, $size, $htext, $state.width);
    def blockH as int init len($lines) * lineH($size);
    $state = ensureSpace($state, $blockH);
    # Record a bookmark for this heading when its level is within the option's
    # bookmark depth. The page it lands on is the one being built (its index once
    # added is the current page count), and `y` is already in PDF coordinates.
    if ($state.opts.bookmarkLevel > 0 and $lvl <= $state.opts.bookmarkLevel) {
        $state.doc = pdf.bookmark($state.doc, len($state.doc.pages), $state.y, $htext, $lvl);
    }
    # Optional shaded background bar behind the heading (drawn before the text; the
    # colour is reset to black so the text and later content stay black).
    def bg as Fill init headingBg($state.opts, $lvl);
    if ($bg.on) {
        $state.page = pdf.color($state.page, $bg.r, $bg.g, $bg.b);
        $state.page = pdf.rect($state.page, $state.x - 3, $state.y - $blockH + 2, $state.width + 6, $blockH + 2, true);
        $state.page = pdf.color($state.page, 0, 0, 0);
    }
    return placePlainLines($state, $lines, $state.opts.headingFont, $size, 0);
}

func renderParagraph(state as Layout, node as Node) {
    def words as list of IWord init inlineWords(children($node), $state.opts);
    def lines as list of list of IWord init packLines($words, $state.opts.bodySize, $state.width, $state.opts);
    return placeInlineLines($state, $lines, $state.opts.bodySize);
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
        $state = ensureSpace($state, $blockH);
        def top as int init $state.y + 4;
        if ($fill.on) {
            $state.page = pdf.color($state.page, $fill.r, $fill.g, $fill.b);
            $state.page = pdf.rect($state.page, $state.x - 3, $top - $blockH, $state.width + 6, $blockH, true);
            $state.page = pdf.color($state.page, 0, 0, 0);
        }
        if ($border.on) {
            $state.page = pdf.color($state.page, $border.r, $border.g, $border.b);
            $state.page = pdf.rect($state.page, $state.x - 3, $top - $blockH, $state.width + 6, $blockH, false);
            $state.page = pdf.color($state.page, 0, 0, 0);
        }
    }
    return placePlainLines($state, $lines, $state.opts.monoFont, $state.opts.bodySize, 6);
}

func renderList(state as Layout, node as Node, depth as int) {
    def ordered as bool init attr($node, "ordered") == "true";
    def markerW as int init 18;
    def idx as int init 1;
    for (def item in children($node)) {
        def marker as string init "-";
        if ($ordered) {
            $marker = convert.toString($idx) + ".";
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
        $state = ensureSpace($state, lineH($state.opts.bodySize));
        def baseline as int init $state.y - $state.opts.bodySize;
        $state.page = pdf.text($state.page, $state.x, $baseline, $state.opts.bodyFont, $state.opts.bodySize, $marker);
        # Item text flows in a column indented past the marker, starting on that line.
        def savedX as int init $state.x;
        def savedW as int init $state.width;
        $state.x = $savedX + $markerW;
        $state.width = $savedW - $markerW;
        def words as list of IWord init inlineWords($inlineKids, $state.opts);
        def lines as list of list of IWord init packLines($words, $state.opts.bodySize, $state.width, $state.opts);
        if (len($lines) == 0) {
            $state.y = $state.y - lineH($state.opts.bodySize);
        } else {
            $state = placeInlineLines($state, $lines, $state.opts.bodySize);
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
func columnEdges(rows as list of Node, ncols as int, avail as int, font as string, size as int, pad as int, unenc as string) {
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
                def txt as string init pdf.toWinAnsi(text($cells[$c]), $unenc);
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
func wrapRow(cells as list of Node, ncols as int, edges as list of int, pad as int, font as string, size as int, unenc as string) {
    def lines as list of list of string init [];
    def maxLines as int init 1;
    def c as int init 0;
    while ($c < $ncols) {
        def txt as string init "";
        if ($c < len($cells)) {
            $txt = pdf.toWinAnsi(text($cells[$c]), $unenc);
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
# markdown alignment ("right" / "center" / left).
func cellX(cellLeft as int, colW as int, pad as int, align as string, font as string, size as int, line as string) {
    if ($align == "right") {
        return $cellLeft + $colW - $pad - roundPt(pdf.measureText($font, $size, $line));
    }
    if ($align == "center") {
        return $cellLeft + ($colW - roundPt(pdf.measureText($font, $size, $line))) // 2;
    }
    return $cellLeft + $pad;
}

# renderTable draws a GFM table as a ruled grid. The per-cell wrapping (`wrapRow`) and
# per-line x placement (`cellX`) are extracted as pure helpers that take no `Layout`,
# so the row loop mutates one `Layout` frame in place - no full-state copy per row.
func renderTable(state as Layout, node as Node) {
    def rows as list of Node init children($node);
    if (len($rows) == 0) {
        return $state;
    }
    def ncols as int init len(children($rows[0]));
    if ($ncols == 0) {
        return $state;
    }
    def size as int init $state.opts.bodySize;
    def pad as int init $state.opts.tablePad;
    def lh as int init cellLineH($size);
    def edges as list of int init columnEdges($rows, $ncols, $state.width, $state.opts.bodyFont, $size, $pad, $state.opts.unencodable);
    def r as int init 0;
    for (def row in $rows) {
        def cells as list of Node init children($row);
        def font as string init $state.opts.bodyFont;
        if ($r == 0) {
            $font = $state.opts.boldFont;
        }
        def rc as RowCells init wrapRow($cells, $ncols, $edges, $pad, $font, $size, $state.opts.unencodable);
        def rowH as int init $rc.maxLines * $lh + 2 * $pad;
        $state = ensureSpace($state, $rowH);
        def rowTop as int init $state.y;
        def rowBot as int init $state.y - $rowH;
        # Optional shaded background behind the header row (drawn first, then the cell
        # text and grid on top; colour reset to black).
        if ($r == 0 and $state.opts.tableHeaderFill.on) {
            def hf as Fill init $state.opts.tableHeaderFill;
            $state.page = pdf.color($state.page, $hf.r, $hf.g, $hf.b);
            $state.page = pdf.rect($state.page, $state.x, $rowBot, $state.width, $rowH, true);
            $state.page = pdf.color($state.page, 0, 0, 0);
        }
        # Cell text, aligned per the column's markdown alignment.
        def c as int init 0;
        while ($c < $ncols) {
            def align as string init "left";
            if ($c < len($cells)) {
                $align = attr($cells[$c], "align");
            }
            def cellLeft as int init $state.x + $edges[$c];
            def colW as int init $edges[$c + 1] - $edges[$c];
            def ly as int init $rowTop - $pad - $size;
            for (def ln in $rc.lines[$c]) {
                $state.page = pdf.text($state.page, cellX($cellLeft, $colW, $pad, $align, $font, $size, $ln), $ly, $font, $size, $ln);
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
            doc: pdf.document(),
            page: pdf.page($state.opts.pageWidth, $state.opts.pageHeight),
            y: $state.y,
            x: $savedX + $indent,
            width: $savedW - $indent,
            opts: $state.opts
        };
        for (def child in children($node)) {
            $probe = renderBlock($probe, $child, $depth + 1);
        }
        if (len($probe.doc.pages) == 0) {
            def top as int init $state.y + 2;
            def blockH as int init ($state.y - $probe.y) + 4;
            if ($fill.on) {
                $state.page = pdf.color($state.page, $fill.r, $fill.g, $fill.b);
                $state.page = pdf.rect($state.page, $savedX, $top - $blockH, $savedW, $blockH, true);
                $state.page = pdf.color($state.page, 0, 0, 0);
            }
            if ($rule.on) {
                $state.page = pdf.color($state.page, $rule.r, $rule.g, $rule.b);
                $state.page = pdf.rect($state.page, $savedX, $top - $blockH, 3, $blockH, true);
                $state.page = pdf.color($state.page, 0, 0, 0);
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
func renderRule(state as Layout) {
    def h as int init lineH($state.opts.bodySize);
    $state = ensureSpace($state, $h);
    def midY as int init $state.y - $h // 2;
    $state.page = pdf.color($state.page, 128, 128, 128);
    $state.page = pdf.rect($state.page, $state.x, $midY, $state.width, 1, true);
    $state.page = pdf.color($state.page, 0, 0, 0);
    $state.y = $state.y - $h;
    return $state;
}

func renderBlock(state as Layout, node as Node, depth as int) {
    match (typeOf($node)) {
        when "heading" { $state = renderHeading($state, $node); }
        when "paragraph" { $state = renderParagraph($state, $node); }
        when "list" { $state = renderList($state, $node, $depth); }
        when "code" { $state = renderCode($state, $node); }
        when "table" { $state = renderTable($state, $node); }
        when "quote" { $state = renderQuote($state, $node, $depth); }
        when "thematic_break" { $state = renderRule($state); }
        when "page_break" {
            # Start a fresh page; no trailing gap.
            return flushPage($state);
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
    def state as Layout init newLayout($opts);
    for (def block in children($doc)) {
        $state = renderBlock($state, $block, 0);
    }
    $state.doc = pdf.addPage($state.doc, $state.page);
    return $state.doc;
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

/**
 * Render a Markdown string to PDF bytes with the given options.
 * @param md {string} the Markdown source
 * @param opts {PdfOptions} the page geometry and fonts
 * @return {bytes} the PDF document
 */
export func toPdfWith(md as string, opts as PdfOptions) {
    return renderPdf(parse($md), $opts);
}

/**
 * Render a Markdown string to PDF bytes with the default options.
 * @param md {string} the Markdown source
 * @return {bytes} the PDF document
 */
export func toPdf(md as string) {
    return toPdfWith($md, pdfDefaults());
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
