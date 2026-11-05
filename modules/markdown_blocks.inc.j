# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0
#
# The line loop and the inline scan are single long functions by design.
# lint-disable-file: L201
#
# markdown_blocks.inc.j - the block phase of the markdown parser. Included
# (textual splice) into markdown.j; it declares no `use` of its own and relies
# on the includer's imports and helpers. Private to the markdown module.
#
# This is CommonMark's parsing strategy: each line is matched against the
# blocks still open (a quote needs its `>`, a list item its indentation), then
# checked for new block starts, and otherwise added to the open paragraph (a
# lazy continuation line when some container did not match). Containers hold
# any blocks, so a list item can carry paragraphs, code and quotes.
#
# Open blocks live on a stack of fixed slots (`open`, `depth` of them in use):
# popping moves the depth back rather than slicing the list, so closing a block
# never copies the document. A closed block becomes a doctree node appended to
# its parent's `kids`. Paragraph, heading and table-cell text is not parsed
# yet: it is kept as an `md_inline` placeholder, because a link reference may
# be defined after its use, and replaced once the whole tree exists.

# An open block. Which fields matter depends on `kind`: "document", "quote",
# "list", "item", "deflist", "desc" and "fndef" (a footnote definition) are
# containers; "paragraph", "fence",
# "icode", "html" and "table" are leaves that collect `lines`.
def struct OB {
    kind as string,
    kids as list of doctree.Node,
    lines as list of string,
    offset as int,
    ordered as bool,
    marker as string,
    start as int,
    loose as bool,
    blank as bool,
    fenceChar as string,
    fenceLen as int,
    fenceIndent as int,
    info as string,
    htmlType as int,
    aligns as list of string
};

# The block parser's state: the open-block stack, the link reference
# definitions found so far (normalised label -> [url, title]) and the footnote
# definitions (normalised label -> the `footnote` node, kept out of the tree
# until the references number them).
def struct BP {
    open as list of OB,
    depth as int,
    refs as map of string to list of string,
    notes as map of string to doctree.Node
};

# A list marker read off a line: the content offset (columns from the start of
# the text it was read from), the text after it, and whether it opens empty.
def struct ListScan {
    ok as bool,
    ordered as bool,
    marker as string,
    start as int,
    offset as int,
    rest as string,
    empty as bool
};

# An ATX heading read off a line.
def struct AtxScan {
    ok as bool,
    level as int,
    text as string
};

# A link reference definition read off the start of a paragraph.
def struct RefScan {
    ok as bool,
    label as string,
    url as string,
    title as string,
    end as int
};

# The block-level HTML tag names (CommonMark's HTML block type 6).
def const HTML_BLOCK_TAGS as list of string init [
    "address",
    "article",
    "aside",
    "base",
    "basefont",
    "blockquote",
    "body",
    "caption",
    "center",
    "col",
    "colgroup",
    "dd",
    "details",
    "dialog",
    "dir",
    "div",
    "dl",
    "dt",
    "fieldset",
    "figcaption",
    "figure",
    "footer",
    "form",
    "frame",
    "frameset",
    "h1",
    "h2",
    "h3",
    "h4",
    "h5",
    "h6",
    "head",
    "header",
    "hr",
    "html",
    "iframe",
    "legend",
    "li",
    "link",
    "main",
    "menu",
    "menuitem",
    "nav",
    "noframes",
    "ol",
    "optgroup",
    "option",
    "p",
    "param",
    "search",
    "section",
    "summary",
    "table",
    "tbody",
    "td",
    "tfoot",
    "th",
    "thead",
    "title",
    "tr",
    "track",
    "ul"
];

# An HTML open or closing tag alone on its line (HTML block type 7).
def const HTML_TAG_LINE_RE as string init '^(?:<[A-Za-z][A-Za-z0-9-]*(?:\s+[A-Za-z_:][A-Za-z0-9_.:-]*' +
    '(?:\s*=\s*(?:[^\s"' + "'" + '=<>`]+|' + "'[^']*'" +
    '|"[^"]*"))?)*\s*/?>|</[A-Za-z][A-Za-z0-9-]*\s*>)\s*$';

func newOB(kind as string) {
    def o as OB;
    $o.kind = $kind;
    return $o;
}

func isLeafKind(kind as string) {
    return $kind == "paragraph" or $kind == "fence" or $kind == "icode" or $kind == "html" or
        $kind == "table";
}

func isBlankLine(s as string) {
    return len(strings.trim($s)) == 0;
}

# indentOf counts leading spaces (tabs in the indentation are expanded first).
func indentOf(s as string) {
    def m as regex.Match init regex.find('^ *', $s);
    return $m.end;
}

# expandLead expands the tabs in a line's leading whitespace to 4-column tab
# stops, counting columns from `col` (where the text sits after any container
# markers). Tabs past the indentation are content and stay as they are.
func expandLead(s as string, col as int) {
    if (not strings.contains($s, "\t")) {
        return $s;
    }
    def cs as list of string init strings.chars($s);
    def out as list of string init [];
    def c as int init $col;
    def k as int init 0;
    while ($k < len($cs) and ($cs[$k] == " " or $cs[$k] == "\t")) {
        if ($cs[$k] == "\t") {
            def w as int init 4 - ($c % 4);
            $out[] = strings.repeat(" ", $w);
            $c = $c + $w;
        } else {
            $out[] = " ";
            $c = $c + 1;
        }
        $k = $k + 1;
    }
    return strings.join($out, "") + strings.join($cs[$k..], "");
}

# dropCols removes up to n leading spaces.
func dropCols(s as string, n as int) {
    def k as int init 0;
    def cs as list of string init strings.chars($s);
    while ($k < $n and $k < len($cs) and $cs[$k] == " ") {
        $k = $k + 1;
    }
    return strings.join($cs[$k..], "");
}

# --- line classification --------------------------------------------

func isThematicBreak(trimmed as string) {
    def compact as string init strings.replace(strings.replace($trimmed, " ", ""), "\t", "");
    if (len($compact) < 3) {
        return false;
    }
    def first as string init $compact[..1];
    if (not ($first == "-" or $first == "*" or $first == "_")) {
        return false;
    }
    return $compact == strings.repeat($first, len($compact));
}

# scanAtx reads an ATX heading (`## Title ##`) from text with its indentation
# already removed.
func scanAtx(t as string) {
    def none as AtxScan init AtxScan{ok: false, level: 0, text: ""};
    def m as regex.Match init regex.find('^(#{1,6})(?:[ \t]+|$)', $t);
    if ($m.start != 0) {
        return $none;
    }
    def body as string init strings.trim($t[$m.end..]);
    # A closing run of `#` goes when a space precedes it (or it is all there is).
    def closing as regex.Match init regex.find('(?:^|[ \t]+)#+$', $body);
    if ($closing.start >= 0) {
        $body = strings.trimRight($body[..$closing.start]);
    }
    return AtxScan{ok: true, level: len($m.groups[0]), text: $body};
}

# fenceOpen reads an opening code fence (3+ backticks or tildes); the info
# string of a backtick fence may not contain a backtick.
func fenceOpen(t as string) {
    def m as regex.Match init regex.find('^(`{3,}|~{3,})(.*)$', $t);
    if ($m.start != 0) {
        return "";
    }
    if (strings.startsWith($m.groups[0], "`") and strings.contains($m.groups[1], "`")) {
        return "";
    }
    return $m.groups[0];
}

func isFenceClose(rest as string, ch as string, n as int) {
    def ind as int init indentOf($rest);
    if ($ind > 3) {
        return false;
    }
    def t as string init strings.trimRight($rest[$ind..]);
    return len($t) >= $n and $t == strings.repeat($ch, len($t));
}

# htmlStart is the CommonMark HTML block type (1-7) a line opens, or 0.
func htmlStart(t as string) {
    def low as string init strings.lower($t);
    if (regex.matches('^<(?:script|pre|style|textarea)(?:[\s>]|$)', $low)) {
        return 1;
    }
    if (strings.startsWith($t, "<!--")) {
        return 2;
    }
    if (strings.startsWith($t, "<?")) {
        return 3;
    }
    if (regex.matches('^<![A-Za-z]', $t)) {
        return 4;
    }
    if (strings.startsWith($t, "<![CDATA[")) {
        return 5;
    }
    def tm as regex.Match init regex.find('^</?([A-Za-z][A-Za-z0-9-]*)(?:[\s>]|/>|$)', $t);
    if ($tm.start == 0 and lists.contains(HTML_BLOCK_TAGS, strings.lower($tm.groups[0]))) {
        return 6;
    }
    if (regex.matches(HTML_TAG_LINE_RE, $t)) {
        return 7;
    }
    return 0;
}

# htmlEnds is true when a line holds the end condition of an HTML block of
# type 1-5.
func htmlEnds(kind as int, line as string) {
    match ($kind) {
        when 1 { return regex.matches('(?i)</(?:script|pre|style|textarea)>', $line); }
        when 2 { return strings.contains($line, "-->"); }
        when 3 { return strings.contains($line, "?>"); }
        when 4 { return strings.contains($line, ">"); }
        when 5 { return strings.contains($line, "]]>"); }
        else { return false; }
    }
}

# listMarker reads a list item marker (`-`, `*`, `+`, `1.`, `1)`) at the start
# of `rest` (up to three spaces in).
func listMarker(rest as string) {
    def none as ListScan init ListScan{
        ok: false,
        ordered: false,
        marker: "",
        start: 0,
        offset: 0,
        rest: "",
        empty: false
    };
    def ind as int init indentOf($rest);
    if ($ind > 3) {
        return $none;
    }
    def t as string init $rest[$ind..];
    def m as regex.Match init regex.find('^(?:([-*+])|([0-9]{1,9})([.)]))(?:[ \t]|$)', $t);
    if ($m.start != 0) {
        return $none;
    }
    def out as ListScan init $none;
    $out.ok = true;
    def mlen as int init 1;
    if ($m.groups[0] != "") {
        $out.marker = $m.groups[0];
    } else {
        $out.ordered = true;
        $out.marker = $m.groups[2];
        $out.start = convert.toInt($m.groups[1]);
        $mlen = len($m.groups[1]) + 1;
    }
    def after as string init expandLead($t[$mlen..], $ind + $mlen);
    if (isBlankLine($after)) {
        $out.empty = true;
        $out.offset = $ind + $mlen + 1;
        $out.rest = "";
        return $out;
    }
    def sp as int init indentOf($after);
    if ($sp >= 5) {
        # Content that starts five or more spaces in is an indented code block
        # one space after the marker.
        $out.offset = $ind + $mlen + 1;
        $out.rest = $after[1..];
        return $out;
    }
    $out.offset = $ind + $mlen + $sp;
    $out.rest = $after[$sp..];
    return $out;
}

# defMarker is true for a definition line (`: text`) of a definition list.
func defMarker(t as string) {
    return $t == ":" or strings.startsWith($t, ": ") or strings.startsWith($t, ":\t");
}

# startsBlock is true when a line (indentation included) would open a block
# other than a paragraph; a definition list ends at such a line.
func startsBlock(rest as string) {
    def ind as int init indentOf($rest);
    if ($ind >= 4) {
        return false;
    }
    def t as string init $rest[$ind..];
    if (strings.startsWith($t, ">") or scanAtx($t).ok or fenceOpen($t) != "" or isThematicBreak($t)) {
        return true;
    }
    def h as int init 0;
    if (strings.startsWith($t, "<")) {
        $h = htmlStart($t);
    }
    return ($h > 0 and $h < 7) or listMarker($rest).ok;
}

# --- the stack ------------------------------------------------------

# pushBlock opens a block as a child of the deepest open block. Adding a child
# to an item (or definition) after a blank line makes it loose; a new item in
# a list after a blank line makes the list loose.
func pushBlock(st as BP, ob as OB) { # lint-disable: L205
    if ($st.depth > MAX_DOC_DEPTH) {
        mdFail("document nesting too deep (over {MAX_DOC_DEPTH} levels)");
    }
    $st = noteChild($st);
    if ($st.depth < len($st.open)) {
        $st.open[$st.depth] = $ob;
    } else {
        $st.open[] = $ob;
    }
    $st.depth = $st.depth + 1;
    return $st;
}

# noteChild records that the deepest open block is about to get a child: a
# pending blank line in front of it makes an item or definition loose.
func noteChild(st as BP) { # lint-disable: L205
    def p as int init $st.depth - 1;
    if ($st.open[$p].blank) {
        def pk as string init $st.open[$p].kind;
        if ($pk == "list" or
            (($pk == "item" or $pk == "desc" or $pk == "fndef") and
            (len($st.open[$p].kids) > 0))) {
            $st.open[$p].loose = true;
        }
        $st.open[$p].blank = false;
    }
    return $st;
}

# addNode appends a finished node (a heading, a rule) to the deepest open
# block.
func addNode(st as BP, n as doctree.Node) { # lint-disable: L205
    $st = noteChild($st);
    $st.open[$st.depth - 1].kids[] = $n;
    return $st;
}

# closeTop closes the deepest open block: it becomes doctree node(s) in its
# parent. A block that ended on a blank line passes the blank up, so the next
# sibling sees it.
func closeTop(st as BP) { # lint-disable: L205
    def t as int init $st.depth - 1;
    def kind as string init $st.open[$t].kind;
    def blank as bool init $st.open[$t].blank;
    def loose as bool init $st.open[$t].loose;
    $st.depth = $t;
    def nodes as list of doctree.Node init [];
    if ($kind == "paragraph") {
        def raw as string init strings.join($st.open[$t].lines, "\n");
        $st = takeRefs($st, $raw);
        $nodes = paragraphNodes($raw);
    } elseif ($kind == "fndef") {
        def label as string init $st.open[$t].marker;
        def key as string init normLabel($label);
        if (not maps.has($st.notes, $key)) {
            def fn as doctree.Node init nodeOf("footnote");
            $fn.attrs = {"md-label": $label};
            $fn.children = $st.open[$t].kids;
            $st.notes[$key] = $fn;
        }
    } else {
        $nodes = finishBlock($st.open[$t]);
    }
    def p as int init $t - 1;
    if ($kind == "item" and $loose) {
        $st.open[$p].loose = true;
    }
    for (def n in $nodes) {
        $st.open[$p].kids[] = $n;
    }
    if ($blank and $kind != "paragraph" and $kind != "quote") {
        $st.open[$p].blank = true;
    }
    return $st;
}

# closeTo closes open blocks until `keep` is the deepest one.
func closeTo(st as BP, keep as int) { # lint-disable: L205
    while ($st.depth - 1 > $keep) {
        $st = closeTop($st);
    }
    return $st;
}

# prepare makes the deepest matched container ready for a new block of the
# given kind: deeper blocks close, and a list (which holds only items) or a
# definition list (which holds only terms and definitions) closes too.
func prepare(st as BP, container as int, kind as string) { # lint-disable: L205
    $st = closeTo($st, $container);
    while (true) {
        def ck as string init $st.open[$st.depth - 1].kind;
        if ($ck == "list" and $kind != "item") {
            $st = closeTop($st);
        } elseif ($ck == "deflist" and $kind != "paragraph" and $kind != "desc") {
            $st = closeTop($st);
        } else {
            break;
        }
    }
    return $st;
}

# --- finishing blocks ----------------------------------------------

func inlineHolder(text as string) {
    def n as doctree.Node init nodeOf("md_inline");
    $n.text = $text;
    return $n;
}

func withInline(kind as string, text as string) {
    def n as doctree.Node init nodeOf($kind);
    $n.children = [inlineHolder($text)];
    return $n;
}

# headingNode builds a heading, peeling a trailing `{#id .class}` attribute
# list off its text.
func headingNode(level as int, text as string) {
    def hs as AttrSplit init splitTrailingAttrs($text);
    def n as doctree.Node init withInline("heading", $hs.text);
    $n.level = $level;
    $n.attrs = $hs.attrs;
    return $n;
}

# paragraphNodes is what a closing paragraph becomes. Its raw text, once the
# link reference definitions are taken off the front, is a paragraph - unless
# it is the first block of an item (a task marker) or of a quote (an alert
# marker), which the container reads when it closes.
func paragraphNodes(raw as string) {
    def text as string init strings.trim(restAfterRefs($raw));
    if ($text == "") {
        return [];
    }
    return [withInline("paragraph", $text)];
}

# finishBlock turns a closed block other than a paragraph into its nodes.
func finishBlock(ob as OB) {
    match ($ob.kind) {
        when "fence" {
            def n as doctree.Node init nodeOf("code");
            $n.text = strings.join($ob.lines, "\n");
            def info as string init unescapeText(strings.trim($ob.info));
            def sp as int init strings.indexOf($info, " ");
            $n.lang = $info;
            if ($sp >= 0) {
                $n.lang = $info[..$sp];
            }
            if ($info != "") {
                $n.attrs["info"] = $info;
            }
            return [$n];
        }
        when "icode" {
            def ls as list of string init $ob.lines;
            def k as int init len($ls);
            while ($k > 0 and isBlankLine($ls[$k - 1])) {
                $k = $k - 1;
            }
            def n as doctree.Node init nodeOf("code");
            $n.text = strings.join($ls[..$k], "\n");
            return [$n];
        }
        when "html" {
            def text as string init strings.join($ob.lines, "\n");
            if (strings.trim($text) == "<!-- pagebreak -->") {
                return [nodeOf("page_break")];
            }
            def n as doctree.Node init nodeOf("html_block");
            $n.text = $text;
            return [$n];
        }
        when "table" { return [tableNode($ob)]; }
        when "quote" { return [quoteNode($ob)]; }
        when "item" { return [itemNode($ob)]; }
        when "list" { return [listNode($ob)]; }
        when "desc" {
            def n as doctree.Node init nodeOf("def_desc");
            $n.children = $ob.kids;
            if (len($ob.kids) == 1 and $ob.kids[0].kind == "paragraph") {
                $n.children = $ob.kids[0].children;
            }
            return [$n];
        }
        when "deflist" { return deflistNodes($ob); }
        else { return $ob.kids; }
    }
}

func tableNode(ob as OB) {
    def n as doctree.Node init nodeOf("table");
    def cols as int init len($ob.aligns);
    def rows as list of doctree.Node init [];
    for (def line in $ob.lines) {
        def cells as list of string init splitCells($line);
        def row as doctree.Node init nodeOf("row");
        for (def c in 0..$cols) {
            def cell as doctree.Node init nodeOf("cell");
            $cell.align = alignOf($ob.aligns, $c);
            if ($c < len($cells) and $cells[$c] != "") {
                $cell.children = [inlineHolder($cells[$c])];
            }
            $row.children[] = $cell;
        }
        $rows[] = $row;
    }
    $n.children = $rows;
    return $n;
}

# quoteNode is a block quote, or an admonition when its first paragraph opens
# with an alert marker (`[!NOTE] Optional title`): the marker line is
# structure, not content, so it goes.
func quoteNode(ob as OB) {
    def n as doctree.Node init nodeOf("quote");
    $n.children = $ob.kids;
    if (len($ob.kids) == 0 or $ob.kids[0].kind != "paragraph") {
        return $n;
    }
    def raw as string init $ob.kids[0].children[0].text;
    def nl as int init strings.indexOf($raw, "\n");
    def first as string init $raw;
    def body as string init "";
    if ($nl >= 0) {
        $first = $raw[..$nl];
        $body = strings.trim($raw[$nl + 1..]);
    }
    def al as Alert init alertMarker($first);
    if ($al.kind == "") {
        return $n;
    }
    def adm as doctree.Node init nodeOf("admonition");
    $adm.lang = $al.kind;
    $adm.title = $al.title;
    def kids as list of doctree.Node init [];
    if ($body != "") {
        $kids[] = withInline("paragraph", $body);
    }
    for (def k in 1..len($ob.kids)) {
        $kids[] = $ob.kids[$k];
    }
    $adm.children = $kids;
    return $adm;
}

# itemNode is a list item; a first paragraph opening with `[ ]` / `[x]` makes
# it a task item (the marker goes, `task` / `checked` attrs record it).
func itemNode(ob as OB) {
    def n as doctree.Node init nodeOf("item");
    $n.children = $ob.kids;
    if (len($ob.kids) == 0 or $ob.kids[0].kind != "paragraph") {
        return $n;
    }
    def raw as string init $ob.kids[0].children[0].text;
    def tm as regex.Match init regex.find('^\[([ xX])\](?:[ \t]+|$)', $raw);
    if ($tm.start != 0) {
        return $n;
    }
    $n.attrs["task"] = "true";
    $n.attrs["checked"] = "false";
    if ($tm.groups[0] != " ") {
        $n.attrs["checked"] = "true";
    }
    def rest as string init $raw[$tm.end..];
    if ($rest == "") {
        $n.children = $ob.kids[1..];
    } else {
        $n.children[0].children[0].text = $rest;
    }
    return $n;
}

# listNode is a list; in a tight one each item's paragraphs are unwrapped to
# their inline content (the compact form a renderer prints without <p>).
func listNode(ob as OB) {
    def n as doctree.Node init nodeOf("list");
    $n.ordered = $ob.ordered;
    if ($ob.ordered and $ob.start != 1) {
        $n.attrs["start"] = "{$ob.start}";
    }
    if ($ob.loose) {
        $n.children = $ob.kids;
        return $n;
    }
    def items as list of doctree.Node init [];
    for (def item in $ob.kids) {
        def kids as list of doctree.Node init [];
        for (def c in $item.children) {
            if ($c.kind == "paragraph") {
                for (def ic in $c.children) {
                    $kids[] = $ic;
                }
            } else {
                $kids[] = $c;
            }
        }
        def it as doctree.Node init $item;
        $it.children = $kids;
        $items[] = $it;
    }
    $n.children = $items;
    return $n;
}

# deflistNodes groups a definition list's children: runs of terms and
# definitions become definition lists, and a paragraph that never got a
# definition (a candidate term) stays a paragraph between them.
func deflistNodes(ob as OB) {
    def out as list of doctree.Node init [];
    def cur as list of doctree.Node init [];
    for (def k in $ob.kids) {
        if ($k.kind == "def_term" or $k.kind == "def_desc") {
            $cur[] = $k;
            continue;
        }
        if (len($cur) > 0) {
            def dl as doctree.Node init nodeOf("definition_list");
            $dl.children = $cur;
            $out[] = $dl;
            $cur = [];
        }
        $out[] = $k;
    }
    if (len($cur) > 0) {
        def dl as doctree.Node init nodeOf("definition_list");
        $dl.children = $cur;
        $out[] = $dl;
    }
    return $out;
}

# --- link reference definitions --------------------------------------

# normLabel is a reference label's matching form: case-folded, whitespace runs
# collapsed, trimmed.
func normLabel(s as string) {
    # Lowercasing is close to Unicode case folding; the one fold it misses in
    # practice is the sharp s, whose capital `ẞ` folds to `ss`.
    return strings.replace(strings.lower(strings.trim(regex.replace('\s+', $s, " "))), "ß", "ss");
}

# takeRefs records the link reference definitions at the start of a
# paragraph's raw text (the first definition of a label wins).
func takeRefs(st as BP, raw as string) { # lint-disable: L205
    if (not strings.startsWith(strings.trimLeft($raw), "[")) {
        return $st;
    }
    def cs as list of string init strings.chars($raw);
    def p as int init 0;
    while (true) {
        def r as RefScan init refDefAt($cs, $p);
        if (not $r.ok) {
            break;
        }
        def key as string init normLabel($r.label);
        if (not maps.has($st.refs, $key)) {
            $st.refs[$key] = [$r.url, $r.title];
        }
        $p = $r.end;
    }
    return $st;
}

# restAfterRefs is a paragraph's raw text without the definitions at its
# start.
func restAfterRefs(raw as string) {
    if (not strings.startsWith(strings.trimLeft($raw), "[")) {
        return $raw;
    }
    def cs as list of string init strings.chars($raw);
    def p as int init 0;
    while (true) {
        def r as RefScan init refDefAt($cs, $p);
        if (not $r.ok) {
            break;
        }
        $p = $r.end;
    }
    return strings.join($cs[$p..], "");
}

func skipSpaces(cs as list of string, p as int) {
    def j as int init $p;
    while ($j < len($cs) and ($cs[$j] == " " or $cs[$j] == "\t")) {
        $j = $j + 1;
    }
    return $j;
}

# refDefAt reads one link reference definition (`[label]: url "title"`) at
# position p, which must be a line start.
func refDefAt(cs as list of string, p as int) {
    def bad as RefScan init RefScan{ok: false, label: "", url: "", title: "", end: $p};
    def n as int init len($cs);
    def j as int init skipSpaces($cs, $p);
    if ($j - $p > 3 or $j >= $n or $cs[$j] != "[") {
        return $bad;
    }
    def lab as LabelScan init scanLabel($cs, $j);
    if (not $lab.ok or $lab.end >= $n or $cs[$lab.end] != ":" or strings.trim($lab.label) == "" or
        strings.startsWith($lab.label, "^")) {
        return $bad;
    }
    $j = skipSpaces($cs, $lab.end + 1);
    if ($j < $n and $cs[$j] == "\n") {
        $j = skipSpaces($cs, $j + 1);
    }
    def d as DestScan init scanDest($cs, $j, true);
    if (not $d.ok) {
        return $bad;
    }
    def afterDest as int init $d.end;
    # An optional title, after whitespace (it may start on the next line).
    def k as int init skipSpaces($cs, $afterDest);
    if ($k < $n and $cs[$k] == "\n") {
        $k = skipSpaces($cs, $k + 1);
    }
    if ($k > $afterDest and $k < $n) {
        def t as TitleScan init scanTitle($cs, $k);
        if ($t.ok) {
            def e as int init skipSpaces($cs, $t.end);
            if ($e >= $n or $cs[$e] == "\n") {
                return RefScan{
                    ok: true,
                    label: $lab.label,
                    url: $d.url,
                    title: $t.title,
                    end: lineEnd($cs, $e)
                };
            }
        }
    }
    def e2 as int init skipSpaces($cs, $afterDest);
    if ($e2 < $n and $cs[$e2] != "\n") {
        return $bad;
    }
    return RefScan{ok: true, label: $lab.label, url: $d.url, title: "", end: lineEnd($cs, $e2)};
}

func lineEnd(cs as list of string, e as int) {
    if ($e < len($cs)) {
        return $e + 1;
    }
    return $e;
}

# --- the line loop ---------------------------------------------------

# blockParse builds the block tree of a whole document.
func blockParse(src as string) {
    def st as BP;
    $st.refs = {};
    $st.notes = {};
    $st.open = [newOB("document")];
    $st.depth = 1;
    def s as string init strings.replace(strings.replace($src, "\r\n", "\n"), "\r", "\n");
    $s = strings.replace($s, "\u0000", "�");
    def lines as list of string init strings.split($s, "\n");
    if (len($lines) > 0 and $lines[len($lines) - 1] == "") {
        $lines = $lines[..len($lines) - 1];
    }
    for (def line in $lines) {
        $st = blockLine($st, expandLead($line, 0));
    }
    $st = closeTo($st, 0);
    return $st;
}

# blockLine adds one source line to the tree.
func blockLine(st as BP, line as string) { # lint-disable: L205
    def rest as string init $line;
    def col as int init 0;
    def top as int init $st.depth - 1;
    def leaf as int init -1;
    if (isLeafKind($st.open[$top].kind)) {
        $leaf = $top;
    }
    def contTop as int init $top;
    if ($leaf >= 0) {
        $contTop = $top - 1;
    }
    # 1. Which open containers does the line continue?
    def last as int init 0;
    def k as int init 1;
    while ($k <= $contTop) {
        def kind as string init $st.open[$k].kind;
        def blank as bool init isBlankLine($rest);
        def ok as bool init true;
        if ($kind == "quote") {
            def ind as int init indentOf($rest);
            if ($ind <= 3 and $ind < len($rest) and $rest[$ind..$ind + 1] == ">") {
                $col = $col + $ind + 1;
                $rest = $rest[$ind + 1..];
                $rest = expandLead($rest, $col);
                if (strings.startsWith($rest, " ")) {
                    $rest = $rest[1..];
                    $col = $col + 1;
                }
            } else {
                $ok = false;
            }
        } elseif ($kind == "item" or $kind == "desc" or $kind == "fndef") {
            def off as int init $st.open[$k].offset;
            if ($blank) {
                if ($kind == "item" and len($st.open[$k].kids) == 0 and $k == $top) {
                    $ok = false;
                }
                $rest = "";
            } elseif (indentOf($rest) >= $off) {
                $rest = $rest[$off..];
                $col = $col + $off;
            } else {
                $ok = false;
            }
        } elseif ($kind == "deflist") {
            if (not $blank and indentOf($rest) == 0 and not defMarker($rest) and startsBlock($rest)) {
                $ok = false;
            }
        }
        if (not $ok) {
            break;
        }
        $last = $k;
        $k = $k + 1;
    }
    def allMatched as bool init $last == $contTop;
    def blank as bool init isBlankLine($rest);
    # 2. A leaf that takes the line as it is.
    if ($leaf >= 0 and $allMatched) {
        def lk as string init $st.open[$leaf].kind;
        if ($lk == "fence") {
            if (isFenceClose($rest, $st.open[$leaf].fenceChar, $st.open[$leaf].fenceLen)) {
                $st = closeTop($st);
                return $st;
            }
            $st.open[$leaf].lines[] = dropCols($rest, $st.open[$leaf].fenceIndent);
            return $st;
        }
        if ($lk == "html") {
            if ($blank and $st.open[$leaf].htmlType >= 6) {
                $st = closeTop($st);
                $st.open[$st.depth - 1].blank = true;
                return $st;
            }
            $st.open[$leaf].lines[] = $rest;
            if (htmlEnds($st.open[$leaf].htmlType, $rest)) {
                $st = closeTop($st);
            }
            return $st;
        }
        if ($lk == "icode") {
            if ($blank) {
                $st.open[$leaf].lines[] = dropCols($rest, 4);
                $st.open[$leaf].blank = true;
                return $st;
            }
            if (indentOf($rest) >= 4) {
                $st.open[$leaf].lines[] = $rest[4..];
                $st.open[$leaf].blank = false;
                return $st;
            }
            $st = closeTop($st);
            $leaf = -1;
        }
    }
    if ($leaf >= 0 and not $allMatched) {
        def lk2 as string init $st.open[$leaf].kind;
        if ($lk2 == "fence" or $lk2 == "html" or $lk2 == "icode") {
            $st = closeTo($st, $last);
            $leaf = -1;
        }
    }
    # 3. New block starts.
    def paraOpen as bool init $leaf >= 0 and $st.open[$leaf].kind == "paragraph";
    def paraMatched as bool init $paraOpen and $allMatched;
    def container as int init $last;
    def started as bool init false;
    while (not $blank) {
        def ind as int init indentOf($rest);
        if ($ind >= 4) {
            if ($paraOpen and not $started) {
                break;
            }
            $st = prepare($st, $container, "icode");
            def ic as OB init newOB("icode");
            $ic.lines = [$rest[4..]];
            $st = pushBlock($st, $ic);
            return $st;
        }
        def t as string init $rest[$ind..];
        def c0 as string init $t[..1];
        if ($c0 == ">") {
            $st = prepare($st, $container, "quote");
            $st = pushBlock($st, newOB("quote"));
            $container = $st.depth - 1;
            $col = $col + $ind + 1;
            $rest = expandLead($t[1..], $col);
            if (strings.startsWith($rest, " ")) {
                $rest = $rest[1..];
                $col = $col + 1;
            }
            $blank = isBlankLine($rest);
            $started = true;
            $paraOpen = false;
            $paraMatched = false;
            continue;
        }
        if ($c0 == "#") {
            def atx as AtxScan init scanAtx($t);
            if ($atx.ok) {
                $st = prepare($st, $container, "heading");
                $st = addNode($st, headingNode($atx.level, $atx.text));
                return $st;
            }
        }
        if ($c0 == "`" or $c0 == "~") {
            def fence as string init fenceOpen($t);
            if ($fence != "") {
                $st = prepare($st, $container, "fence");
                def fo as OB init newOB("fence");
                $fo.fenceChar = $fence[..1];
                $fo.fenceLen = len($fence);
                $fo.fenceIndent = $ind;
                $fo.info = $t[len($fence)..];
                $st = pushBlock($st, $fo);
                return $st;
            }
        }
        if ($c0 == "<") {
            def ht as int init htmlStart($t);
            if ($ht > 0 and not ($ht == 7 and $paraOpen)) {
                $st = prepare($st, $container, "html");
                def ho as OB init newOB("html");
                $ho.htmlType = $ht;
                $ho.lines = [$rest];
                $st = pushBlock($st, $ho);
                if (htmlEnds($ht, $rest)) {
                    $st = closeTop($st);
                }
                return $st;
            }
        }
        if ($paraMatched and regex.matches('^(?:=+|-+)[ \t]*$', $t)) {
            def raw as string init strings.join($st.open[$leaf].lines, "\n");
            def text as string init strings.trim(restAfterRefs($raw));
            if ($text != "") {
                $st = takeRefs($st, $raw);
                $st.depth = $leaf;
                def lvl as int init 2;
                if ($c0 == "=") {
                    $lvl = 1;
                }
                $st = addNode($st, headingNode($lvl, $text));
                return $st;
            }
        }
        # The cheap shape test first: most continuation lines are prose.
        if ($paraMatched and regex.matches('^[ \t|:-]+$', $t) and isDelimiterRow($t)) {
            def pls as list of string init $st.open[$leaf].lines;
            def header as string init $pls[len($pls) - 1];
            def aligns as list of string init parseAligns($t);
            if (strings.contains($header, "|") or strings.contains($t, "|")) {
                if (len(splitCells($header)) == len($aligns)) {
                    $st.open[$leaf].lines = $pls[..len($pls) - 1];
                    if (len($st.open[$leaf].lines) > 0) {
                        $st = closeTop($st);
                    } else {
                        $st.depth = $leaf;
                    }
                    def tob as OB init newOB("table");
                    $tob.aligns = $aligns;
                    $tob.lines = [$header];
                    $st = pushBlock($st, $tob);
                    return $st;
                }
            }
        }
        if (isThematicBreak($t)) {
            $st = prepare($st, $container, "hr");
            $st = addNode($st, nodeOf("thematic_break"));
            return $st;
        }
        def fm as regex.Match init regex.find('^\[\^([^\]\s]+)\]:', $t);
        if ($fm.start == 0) {
            $st = prepare($st, $container, "fndef");
            def fo2 as OB init newOB("fndef");
            $fo2.marker = $fm.groups[0];
            $fo2.offset = $ind + 4;
            $st = pushBlock($st, $fo2);
            $container = $st.depth - 1;
            $col = $col + $ind + $fm.end;
            $rest = expandLead($t[$fm.end..], $col);
            if (strings.startsWith($rest, " ")) {
                $rest = $rest[1..];
                $col = $col + 1;
            }
            $blank = isBlankLine($rest);
            $started = true;
            $paraOpen = false;
            $paraMatched = false;
            continue;
        }
        def lm as ListScan init listMarker($rest);
        # Only a list's first item is limited when it interrupts a paragraph;
        # a sibling of an open list's items is not.
        def sibling as bool init $st.open[$container].kind == "list";
        if ($lm.ok and
            (not $paraOpen or $started or $sibling or
            (not $lm.empty and (not $lm.ordered or $lm.start == 1)))) {
            $st = prepare($st, $container, "item");
            def ck as string init $st.open[$st.depth - 1].kind;
            def same as bool init $ck == "list" and
                $st.open[$st.depth - 1].ordered == $lm.ordered and
                $st.open[$st.depth - 1].marker == $lm.marker;
            if ($ck == "list" and not $same) {
                $st = closeTop($st);
            }
            if (not $same) {
                def lo as OB init newOB("list");
                $lo.ordered = $lm.ordered;
                $lo.marker = $lm.marker;
                $lo.start = $lm.start;
                $st = pushBlock($st, $lo);
            }
            def io as OB init newOB("item");
            $io.offset = $lm.offset;
            $st = pushBlock($st, $io);
            $container = $st.depth - 1;
            $col = $col + $lm.offset;
            $rest = $lm.rest;
            $blank = isBlankLine($rest);
            $started = true;
            $paraOpen = false;
            $paraMatched = false;
            continue;
        }
        if (defMarker($t) and $ind == 0) {
            def termOk as bool init $paraMatched;
            def inList as bool init $st.open[$container].kind == "deflist";
            if ($termOk) {
                # The paragraph's last line is the term; any lines before it stay
                # a paragraph of their own.
                def pls as list of string init $st.open[$leaf].lines;
                def term as string init strings.trim($pls[len($pls) - 1]);
                $st.open[$leaf].lines = $pls[..len($pls) - 1];
                if (len($st.open[$leaf].lines) > 0) {
                    $st = closeTop($st);
                } else {
                    $st.depth = $leaf;
                }
                if ($st.open[$st.depth - 1].kind != "deflist") {
                    $st = pushBlock($st, newOB("deflist"));
                }
                $st = addNode($st, withInline("def_term", $term));
                $inList = true;
            } elseif ($inList) {
                $st = closeTo($st, $container);
            }
            if ($inList) {
                def dsp as int init indentOf(expandLead($t[1..], $col + 1));
                def dofs as int init 1 + math.max(1, math.min($dsp, 4));
                def dob as OB init newOB("desc");
                $dob.offset = $ind + $dofs;
                $st = pushBlock($st, $dob);
                $container = $st.depth - 1;
                $rest = dropCols(expandLead($t[1..], $col + 1), $dofs - 1);
                $col = $col + $dofs;
                $blank = isBlankLine($rest);
                $started = true;
                $paraOpen = false;
                $paraMatched = false;
                continue;
            }
        }
        break;
    }
    # 4. Text: a blank line, a paragraph continuation, a table row or a new
    # paragraph.
    if ($blank) {
        if (not $started) {
            $st = closeTo($st, $last);
            # A blank line inside a block quote is not one between list items or
            # item children, so a quote records none (as commonmark.js).
            if ($st.open[$st.depth - 1].kind == "deflist" or
                $st.open[$st.depth - 1].kind == "list" or
                $st.open[$st.depth - 1].kind == "item" or $st.open[$st.depth - 1].kind == "desc") {
                $st.open[$st.depth - 1].blank = true;
            }
        }
        return $st;
    }
    if (not $started and $paraOpen) {
        if ($allMatched or lazyAllowed($st, $last, $contTop)) {
            $st.open[$leaf].lines[] = strings.trimLeft($rest);
            return $st;
        }
    }
    if (not $started and $leaf >= 0 and $allMatched and $st.open[$leaf].kind == "table") {
        $st.open[$leaf].lines[] = $rest;
        return $st;
    }
    $st = prepare($st, $container, "paragraph");
    def po as OB init newOB("paragraph");
    $po.lines = [strings.trimLeft($rest)];
    $st = pushBlock($st, $po);
    return $st;
}

# lazyAllowed is true when a line may continue a paragraph lazily past the
# unmatched containers: quotes and list items allow it, a definition does not
# (its next unindented line is a new term).
func lazyAllowed(st as BP, last as int, contTop as int) {
    for (def k in $last + 1..$contTop + 1) {
        if ($st.open[$k].kind == "desc" or $st.open[$k].kind == "deflist") {
            return false;
        }
    }
    return true;
}
