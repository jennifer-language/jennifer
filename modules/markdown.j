# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# markdown.j - a lightweight Markdown renderer for a small CommonMark subset:
# ATX headings, bold / italic emphasis, inline code, links, fenced code
# blocks, unordered / ordered lists, and GFM tables. Renders to HTML (through
# the `htmlwriter` module, so escaping is handled for you) and to styled
# terminal text (through the `ansi` module). It also authors Markdown text
# (header / style / link / list / codeBlock / table). Pure Jennifer;
# line-oriented block parsing with a small inline scanner.
#
#     import "markdown.j" as markdown;
#     io.printf("%s\n", markdown.toHtml("# Hi\n\nA **bold** word."));
#     io.printf("%s\n", markdown.toAnsi("- one\n- two"));
#
# Not full CommonMark: inline spans do not nest (the content of `**...**`,
# `` `...` ``, and a link is plain text), and there is no blockquote, thematic
# break, image, or reference-link support. Documents are small, so the
# per-line interpreter overhead is a non-issue.
import "./htmlwriter.j" as html;
import "./ansi.j" as ansi;
use strings;
use regex;
use convert;

# An inline span: a run of "text", or an emphasised / code / link span. Link
# spans carry the target in `url`; the others leave it "".
def struct Span {
    kind as string,
    text as string,
    url as string
};

# A block: a heading (with `level`), a paragraph or code block (in `text`), a
# list (`ordered` plus `items`), or a table (`headings` + `aligns` + `rows`,
# each cell an inline-text string).
def struct Block {
    kind as string,
    level as int,
    text as string,
    ordered as bool,
    items as list of string,
    headings as list of string,
    aligns as list of string,
    rows as list of list of string
};

# A parsed fenced code block plus the line index to resume scanning at
# (a helper's return, since Jennifer functions return a single value and
# value semantics rule out appending into a caller's list).
def struct Fence {
    code as string,
    next as int
};

# A parsed table block plus the line index to resume scanning at.
def struct TableScan {
    block as Block,
    next as int
};

# --- span + block constructors (private) ---------------------------

func span(kind as string, text as string, url as string) {
    return Span{kind: $kind, text: $text, url: $url};
}

func paraBlock(lines as list of string) {
    def joined as string init strings.join($lines, " ");
    return Block{kind: "paragraph", level: 0, text: $joined, ordered: false,
        items: [], headings: [], aligns: [], rows: []};
}

func listBlock(items as list of string, ordered as bool) {
    return Block{kind: "list", level: 0, text: "", ordered: $ordered,
        items: $items, headings: [], aligns: [], rows: []};
}

func tableBlock(headings as list of string, aligns as list of string,
        rows as list of list of string) {
    return Block{kind: "table", level: 0, text: "", ordered: false, items: [],
        headings: $headings, aligns: $aligns, rows: $rows};
}

func codeBlockNode(text as string) {
    return Block{kind: "code", level: 0, text: $text, ordered: false,
        items: [], headings: [], aligns: [], rows: []};
}

# --- inline scanner (private) --------------------------------------

# findChar returns the index of target in cs at or after `from`, or -1.
func findChar(cs as list of string, target as string, from as int) {
    def j as int init $from;
    while ($j < len($cs)) {
        if ($cs[$j] == $target) {
            return $j;
        }
        $j = $j + 1;
    }
    return -1;
}

# findDouble returns the index of the first `target target` pair at or after
# `from`, or -1.
func findDouble(cs as list of string, target as string, from as int) {
    def j as int init $from;
    while ($j + 1 < len($cs)) {
        if ($cs[$j] == $target and $cs[$j + 1] == $target) {
            return $j;
        }
        $j = $j + 1;
    }
    return -1;
}

# sliceStr concatenates cs[start:end] into a string.
func sliceStr(cs as list of string, start as int, end as int) {
    def out as string init "";
    def j as int init $start;
    while ($j < $end) {
        $out = $out + $cs[$j];
        $j = $j + 1;
    }
    return $out;
}

# parseInline scans a line of text into spans. Markers: `` ` `` for code,
# `**` for strong, `*` for emphasis, and `[text](url)` for a link. Span
# content is not re-scanned (no nesting).
func parseInline(s as string) {
    def spans as list of Span init [];
    def cs as list of string init strings.chars($s);
    def n as int init len($cs);
    def i as int init 0;
    def buf as string init "";
    while ($i < $n) {
        def c as string init $cs[$i];
        # inline code: `code`
        def bt as int init -1;
        if ($c == "`") {
            $bt = findChar($cs, "`", $i + 1);
        }
        if ($bt >= 0) {
            if (len($buf) > 0) {
                $spans[] = span("text", $buf, "");
                $buf = "";
            }
            $spans[] = span("code", sliceStr($cs, $i + 1, $bt), "");
            $i = $bt + 1;
            continue;
        }
        # strong: **text**
        def dbl as int init -1;
        if ($c == "*" and $i + 1 < $n and $cs[$i + 1] == "*") {
            $dbl = findDouble($cs, "*", $i + 2);
        }
        if ($dbl >= 0) {
            if (len($buf) > 0) {
                $spans[] = span("text", $buf, "");
                $buf = "";
            }
            $spans[] = span("strong", sliceStr($cs, $i + 2, $dbl), "");
            $i = $dbl + 2;
            continue;
        }
        # emphasis: *text*
        def em as int init -1;
        if ($c == "*") {
            $em = findChar($cs, "*", $i + 1);
        }
        if ($em >= 0) {
            if (len($buf) > 0) {
                $spans[] = span("text", $buf, "");
                $buf = "";
            }
            $spans[] = span("em", sliceStr($cs, $i + 1, $em), "");
            $i = $em + 1;
            continue;
        }
        # link: [text](url)
        def linkEnd as int init linkAt($cs, $i);
        if ($linkEnd >= 0) {
            if (len($buf) > 0) {
                $spans[] = span("text", $buf, "");
                $buf = "";
            }
            $spans[] = linkSpanAt($cs, $i);
            $i = $linkEnd;
            continue;
        }
        $buf = $buf + $c;
        $i = $i + 1;
    }
    if (len($buf) > 0) {
        $spans[] = span("text", $buf, "");
    }
    return $spans;
}

# linkAt returns the index just past a `[text](url)` starting at `i`, or -1 if
# there is no well-formed link there.
func linkAt(cs as list of string, i as int) {
    def n as int init len($cs);
    if (not ($cs[$i] == "[")) {
        return -1;
    }
    def rb as int init findChar($cs, "]", $i + 1);
    if ($rb < 0 or $rb + 1 >= $n or not ($cs[$rb + 1] == "(")) {
        return -1;
    }
    def rp as int init findChar($cs, ")", $rb + 2);
    if ($rp < 0) {
        return -1;
    }
    return $rp + 1;
}

# linkSpanAt builds the link span for a `[text](url)` known to start at `i`.
func linkSpanAt(cs as list of string, i as int) {
    def rb as int init findChar($cs, "]", $i + 1);
    def rp as int init findChar($cs, ")", $rb + 2);
    return span("link", sliceStr($cs, $i + 1, $rb), sliceStr($cs, $rb + 2, $rp));
}

# --- block parser (private) ----------------------------------------

# lineType classifies a source line: "blank", "fence", "heading", "ul"
# (unordered item), "ol" (ordered item), or "plain".
func lineType(trimmed as string, line as string) {
    if (len($trimmed) == 0) {
        return "blank";
    }
    if (strings.startsWith($trimmed, "```")) {
        return "fence";
    }
    if (regex.matches("^(#{1,6})[ \t]+", $line)) {
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

# parseBlocks splits Markdown text into a list of blocks, line by line. The two
# flush guards at the top close an open paragraph or list before a line that
# does not continue it, so each block type's handler only appends.
func parseBlocks(md as string) {
    def blocks as list of Block init [];
    def lines as list of string init strings.split($md, "\n");
    def n as int init len($lines);
    def para as list of string init [];
    def items as list of string init [];
    def ordered as bool init false;
    def inList as bool init false;
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
        # A list continues only across a same-type item.
        def cont as bool init ($lt == "ul" and not $ordered) or ($lt == "ol" and $ordered);
        if ($inList and not $cont) {
            $blocks[] = listBlock($items, $ordered);
            $items = [];
            $inList = false;
        }
        if ($lt == "blank") {
            $i = $i + 1;
            continue;
        }
        if ($lt == "fence") {
            def f as Fence init collectFence($lines, $i);
            $blocks[] = codeBlockNode($f.code);
            $i = $f.next;
            continue;
        }
        if ($lt == "heading") {
            def hm as regex.Match init regex.find("^(#{1,6})[ \t]+(.*)$", $line);
            $blocks[] = headingBlock(len($hm.groups[0]), $hm.groups[1]);
            $i = $i + 1;
            continue;
        }
        if ($lt == "ul" or $lt == "ol") {
            def m as regex.Match init regex.find("^[ \t]*(?:[-*+]|[0-9]+\\.)[ \t]+(.*)$", $line);
            $ordered = $lt == "ol";
            $inList = true;
            $items[] = $m.groups[0];
            $i = $i + 1;
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
    if ($inList) {
        $blocks[] = listBlock($items, $ordered);
    }
    return $blocks;
}

func headingBlock(level as int, text as string) {
    return Block{kind: "heading", level: $level, text: $text, ordered: false,
        items: [], headings: [], aligns: [], rows: []};
}

# collectFence gathers the fenced code block opening at line `open` and returns
# its content plus the line index just past the closing fence.
func collectFence(lines as list of string, open as int) {
    def n as int init len($lines);
    def code as string init "";
    def first as bool init true;
    def j as int init $open + 1;
    while ($j < $n and not strings.startsWith(strings.trim($lines[$j]), "```")) {
        if (not $first) {
            $code = $code + "\n";
        }
        $first = false;
        $code = $code + $lines[$j];
        $j = $j + 1;
    }
    if ($j < $n) {
        return Fence{code: $code, next: $j + 1};
    }
    return Fence{code: $code, next: $j};
}

# --- HTML rendering, through htmlwriter (exported) -----------------

# inlineToNodes maps spans to htmlwriter nodes; html.text escapes text content
# and html.attr escapes the link target.
func inlineToNodes(spans as list of Span) {
    def nodes as list of html.Node init [];
    for (def sp in $spans) {
        if ($sp.kind == "text") {
            $nodes[] = html.text($sp.text);
        } elseif ($sp.kind == "code") {
            $nodes[] = wrapEl("code", $sp.text);
        } elseif ($sp.kind == "strong") {
            $nodes[] = wrapEl("strong", $sp.text);
        } elseif ($sp.kind == "em") {
            $nodes[] = wrapEl("em", $sp.text);
        } elseif ($sp.kind == "link") {
            $nodes[] = linkNode($sp.text, $sp.url);
        }
    }
    return $nodes;
}

func wrapEl(tag as string, text as string) {
    def kids as list of html.Node init [];
    $kids[] = html.text($text);
    return html.element($tag, [], $kids);
}

func linkNode(text as string, url as string) {
    def attrs as list of html.Attr init [];
    $attrs[] = html.attr("href", $url);
    def kids as list of html.Node init [];
    $kids[] = html.text($text);
    return html.element("a", $attrs, $kids);
}

# alignOf returns the alignment for column `i`, or "none" past the list end.
func alignOf(aligns as list of string, i as int) {
    if ($i < len($aligns)) {
        return $aligns[$i];
    }
    return "none";
}

# cellNode builds a `th` / `td` with inline-parsed content and an `align`
# attribute (omitted for "none").
func cellNode(tag as string, text as string, align as string) {
    def attrs as list of html.Attr init [];
    if (not ($align == "none")) {
        $attrs[] = html.attr("align", $align);
    }
    return html.element($tag, $attrs, inlineToNodes(parseInline($text)));
}

# tableRowNode builds one `tr` of `tag` cells (`th` for the header, `td` for
# data), exactly `cols` columns: a short row pads with empty cells, a long one
# is truncated.
func tableRowNode(tag as string, cells as list of string, aligns as list of string, cols as int) {
    def tds as list of html.Node init [];
    def i as int init 0;
    while ($i < $cols) {
        def text as string init "";
        if ($i < len($cells)) {
            $text = $cells[$i];
        }
        $tds[] = cellNode($tag, $text, alignOf($aligns, $i));
        $i = $i + 1;
    }
    return html.element("tr", [], $tds);
}

# tableNode renders a table block as `<table><thead>...<tbody>...`.
func tableNode(b as Block) {
    def cols as int init len($b.headings);
    def head as list of html.Node init [];
    $head[] = tableRowNode("th", $b.headings, $b.aligns, $cols);
    def body as list of html.Node init [];
    for (def row in $b.rows) {
        $body[] = tableRowNode("td", $row, $b.aligns, $cols);
    }
    def parts as list of html.Node init [];
    $parts[] = html.element("thead", [], $head);
    $parts[] = html.element("tbody", [], $body);
    return html.element("table", [], $parts);
}

# blockToNode renders one block to an htmlwriter node.
func blockToNode(b as Block) {
    if ($b.kind == "table") {
        return tableNode($b);
    }
    if ($b.kind == "heading") {
        def tag as string init "h" + convert.toString($b.level);
        return html.element($tag, [], inlineToNodes(parseInline($b.text)));
    }
    if ($b.kind == "code") {
        def codeKids as list of html.Node init [];
        $codeKids[] = html.text($b.text);
        def pre as list of html.Node init [];
        $pre[] = html.element("code", [], $codeKids);
        return html.element("pre", [], $pre);
    }
    if ($b.kind == "list") {
        def lis as list of html.Node init [];
        for (def item in $b.items) {
            $lis[] = html.element("li", [], inlineToNodes(parseInline($item)));
        }
        if ($b.ordered) {
            return html.element("ol", [], $lis);
        }
        return html.element("ul", [], $lis);
    }
    return html.element("p", [], inlineToNodes(parseInline($b.text)));
}

# toHtml renders Markdown to an HTML string (block elements concatenated, no
# indentation).
export func toHtml(md as string) {
    def nodes as list of html.Node init [];
    for (def b in parseBlocks($md)) {
        $nodes[] = blockToNode($b);
    }
    return html.renderAll($nodes);
}

# --- ANSI rendering, through ansi (exported) -----------------------

# inlineToAnsi maps spans to terminal-styled text (styling suppresses itself
# when stdout is not a TTY, so piped output is plain).
func inlineToAnsi(spans as list of Span) {
    def out as string init "";
    for (def sp in $spans) {
        if ($sp.kind == "text") {
            $out = $out + $sp.text;
        } elseif ($sp.kind == "code") {
            $out = $out + ansi.cyan($sp.text);
        } elseif ($sp.kind == "strong") {
            $out = $out + ansi.bold($sp.text);
        } elseif ($sp.kind == "em") {
            $out = $out + ansi.italic($sp.text);
        } elseif ($sp.kind == "link") {
            $out = $out + ansi.underline($sp.text) + " (" + $sp.url + ")";
        }
    }
    return $out;
}

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

# cellVisWidth is the visible width of a rendered cell (styling stripped, so
# escape codes do not count).
func cellVisWidth(text as string) {
    return len(ansi.strip(inlineToAnsi(parseInline($text))));
}

# widenAt grows column `c`'s width to at least `w` (columns past the header
# count are ignored).
func widenAt(widths as list of int, c as int, w as int) {
    if ($c < len($widths) and $w > $widths[$c]) {
        $widths[$c] = $w;
    }
    return $widths;
}

# colWidths is the max visible width of each column over the header and rows.
func colWidths(b as Block) {
    def widths as list of int init [];
    def i as int init 0;
    while ($i < len($b.headings)) {
        $widths[] = cellVisWidth($b.headings[$i]);
        $i = $i + 1;
    }
    for (def row in $b.rows) {
        def c as int init 0;
        while ($c < len($row)) {
            $widths = widenAt($widths, $c, cellVisWidth($row[$c]));
            $c = $c + 1;
        }
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

# ansiRow renders one ` | `-separated row padded to the column widths; the
# header row is bold.
func ansiRow(cells as list of string, aligns as list of string,
        widths as list of int, bold as bool) {
    def out as string init "";
    def i as int init 0;
    while ($i < len($widths)) {
        if ($i > 0) {
            $out = $out + " | ";
        }
        def text as string init "";
        if ($i < len($cells)) {
            $text = $cells[$i];
        }
        def styled as string init inlineToAnsi(parseInline($text));
        if ($bold) {
            $styled = ansi.bold($styled);
        }
        $out = $out + padCell($styled, cellVisWidth($text), $widths[$i], alignOf($aligns, $i));
        $i = $i + 1;
    }
    return $out;
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

# tableToAnsi renders a table block as aligned terminal columns.
func tableToAnsi(b as Block) {
    def widths as list of int init colWidths($b);
    def out as string init ansiRow($b.headings, $b.aligns, $widths, true);
    $out = $out + "\n" + ansiDivider($widths);
    for (def row in $b.rows) {
        $out = $out + "\n" + ansiRow($row, $b.aligns, $widths, false);
    }
    return $out;
}

# blockToAnsi renders one block to terminal text.
func blockToAnsi(b as Block) {
    if ($b.kind == "table") {
        return tableToAnsi($b);
    }
    if ($b.kind == "heading") {
        return ansi.bold(inlineToAnsi(parseInline($b.text)));
    }
    if ($b.kind == "code") {
        return ansi.dim(indentLines($b.text, "    "));
    }
    if ($b.kind == "list") {
        def out as string init "";
        def idx as int init 1;
        def first as bool init true;
        for (def item in $b.items) {
            if (not $first) {
                $out = $out + "\n";
            }
            $first = false;
            def marker as string init "- ";
            if ($b.ordered) {
                $marker = convert.toString($idx) + ". ";
            }
            $out = $out + "  " + $marker + inlineToAnsi(parseInline($item));
            $idx = $idx + 1;
        }
        return $out;
    }
    return inlineToAnsi(parseInline($b.text));
}

# toAnsi renders Markdown to styled terminal text, blocks separated by a blank
# line.
export func toAnsi(md as string) {
    def out as string init "";
    def first as bool init true;
    for (def b in parseBlocks($md)) {
        if (not $first) {
            $out = $out + "\n\n";
        }
        $first = false;
        $out = $out + blockToAnsi($b);
    }
    return $out;
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

# header renders an ATX heading; `level` is "h1".."h6".
export func header(level as string, text as string) {
    return strings.repeat("#", headerLevel($level)) + " " + $text;
}

# style wraps text in an inline emphasis: "bold", "italic", or "code".
export func style(kind as string, text as string) {
    if ($kind == "bold") {
        return "**" + $text + "**";
    }
    if ($kind == "italic") {
        return "*" + $text + "*";
    }
    if ($kind == "code") {
        return "`" + $text + "`";
    }
    fail("markdown.style: kind must be bold|italic|code, got " + $kind);
}

# link renders an inline link `[text](url)`.
export func link(text as string, url as string) {
    return "[" + $text + "](" + $url + ")";
}

# bullets renders an unordered list, one `- item` per line.
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

# numbered renders an ordered list, `1. item` upward.
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

# codeBlock renders a fenced code block around verbatim text.
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

# table renders a GFM table from column `headings`, per-column `aligns`
# ("left" / "right" / "center" / "none"; `[]` for all-default), and `rows`
# (each a list of cell strings). Columns follow `headings`: a short row is
# padded with empty cells, extra cells are dropped. Pipes and newlines in a
# cell are made safe.
export func table(headings as list of string, aligns as list of string,
        rows as list of list of string) {
    def cols as int init len($headings);
    def out as string init tableRow($headings, $cols);
    $out = $out + "\n" + alignRow($aligns, $cols);
    for (def row in $rows) {
        $out = $out + "\n" + tableRow($row, $cols);
    }
    return $out;
}
