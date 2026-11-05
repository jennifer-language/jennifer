# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0
#
# The line loop and the inline scan are single long functions by design.
# lint-disable-file: L201
#
# markdown_inline.inc.j - the inline phase of the markdown parser. Included
# (textual splice) into markdown.j; it declares no `use` of its own and relies
# on the includer's imports and helpers. Private to the markdown module.
#
# CommonMark's inline algorithm. One left-to-right scan turns the text into
# tokens: runs of text, finished nodes (code spans, autolinks, breaks), and
# delimiter runs (`*`, `_`, `~`, `=`, `^`) and brackets that may still become
# markup. A `]` that completes a link wraps the tokens since its `[`, and
# `processEmphasis` pairs delimiter runs with CommonMark's rules (flanking, the
# rule of three). Pairings are recorded on the tokens as tags to open and
# close; `buildInline` then nests the nodes in one pass. Nothing is rebuilt per
# match, so the work stays linear in the text.
#
# Extensions beside CommonMark's `*` / `_`: `~~strike~~` and `~sub~`, `==mark==`
# and `^sup^` use the same delimiter machinery (a subscript or superscript may
# not contain whitespace), and a `{...}` attribute list right after a link or
# image sets its attributes. Raw inline HTML becomes an `html_inline` node
# (markup inside a tag is not parsed); the renderer escapes it unless raw HTML
# is allowed.

# One inline token. `kind` is "text", "node" (a finished `node`), "delim" (a
# delimiter run of `ch`, `len0` long, `remain` of it unused), "open" (a
# bracket that became a link / image: `node` is its template) or "close".
def struct Tok {
    kind as string,
    text as string,
    node as doctree.Node,
    ch as string,
    len0 as int,
    remain as int,
    canOpen as bool,
    canClose as bool,
    opens as list of doctree.Node,
    closes as int
};

# An open `[` or `![` on the bracket stack: its token, whether it may still
# open a link, how deep the delimiter stack was then, and where its text
# starts.
def struct Bracket {
    tok as int,
    image as bool,
    active as bool,
    dsLen as int,
    pos as int
};

# The inline scanner's state.
def struct IS {
    toks as list of Tok,
    ds as list of int,
    alive as list of bool,
    brackets as list of Bracket,
    nbr as int
};

def struct LabelScan {
    ok as bool,
    label as string,
    end as int
};

def struct DestScan {
    ok as bool,
    url as string,
    end as int
};

def struct TitleScan {
    ok as bool,
    title as string,
    end as int
};

# An inline link's `(destination "title")` after its label.
def struct LinkTail {
    ok as bool,
    url as string,
    title as string,
    end as int
};

def const ASCII_PUNCT as string init '!"#$%&' + "'" + '()*+,-./:;<=>?@[\]^_`{|}~';
# Inline nesting deeper than this is emitted flat, a safety net far past any
# real document.
def const INLINE_MAX_DEPTH as int init 32;
def const URI_AUTOLINK_RE as string init '^<([A-Za-z][A-Za-z0-9+.-]{1,31}:[^\s<>]*)>$';
def const EMAIL_AUTOLINK_RE as string init "^<([a-zA-Z0-9.!#$%&'*+/=?^_`\{|\}~-]+@[a-zA-Z0-9]" +
    '(?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*)>$';
def const INLINE_TAG_RE as string init '^(?:<[A-Za-z][A-Za-z0-9-]*(?:\s+[A-Za-z_:][A-Za-z0-9_.:-]*' +
    '(?:\s*=\s*(?:[^\s"' + "'" + '=<>`]+|' + "'[^']*'" +
    '|"[^"]*"))?)*\s*/?>|</[A-Za-z][A-Za-z0-9-]*\s*>' +
    '|<![A-Za-z][^>]*>)$';

func isAsciiPunct(c as string) {
    return len($c) == 1 and strings.contains(ASCII_PUNCT, $c);
}

func isSpaceChar(c as string) {
    return $c == " " or $c == "\t" or $c == "\n" or $c == "\r" or $c == " " or
        $c == "\u000C" or regex.matches('^\pZ$', $c);
}

func isPunctChar(c as string) {
    return isAsciiPunct($c) or regex.matches('^[\pP\pS]$', $c);
}

# unescapeText applies backslash escapes and entity references to text that
# is not inline-parsed further: a link destination or title, a code fence's
# info string.
func unescapeText(s as string) {
    if (not strings.contains($s, "\\")) {
        return decodeEntities($s);
    }
    def cs as list of string init strings.chars($s);
    def out as list of string init [];
    def k as int init 0;
    while ($k < len($cs)) {
        if ($cs[$k] == "\\" and $k + 1 < len($cs) and isAsciiPunct($cs[$k + 1])) {
            # An escaped `&` must not start an entity, so it is held as a
            # marker until entities are decoded.
            if ($cs[$k + 1] == "&") {
                $out[] = "";
            } else {
                $out[] = $cs[$k + 1];
            }
            $k = $k + 2;
            continue;
        }
        $out[] = $cs[$k];
        $k = $k + 1;
    }
    return strings.replace(decodeEntities(strings.join($out, "")), "", "&");
}

# scanLabel reads a link label `[...]` at j: no unescaped brackets inside, at
# most 999 characters.
func scanLabel(cs as list of string, j as int) {
    def bad as LabelScan init LabelScan{ok: false, label: "", end: $j};
    def n as int init len($cs);
    def k as int init $j + 1;
    while ($k < $n and $cs[$k] != "]") {
        if ($cs[$k] == "[") {
            return $bad;
        }
        if ($cs[$k] == "\\" and $k + 1 < $n) {
            $k = $k + 1;
        }
        $k = $k + 1;
        if ($k - $j > 1000) {
            return $bad;
        }
    }
    if ($k >= $n) {
        return $bad;
    }
    return LabelScan{ok: true, label: strings.join($cs[$j + 1..$k], ""), end: $k + 1};
}

# scanDest reads a link destination at j: `<...>` or a run without spaces
# whose parentheses balance. `allowEmpty` admits a bare `<>`.
func scanDest(cs as list of string, j as int, allowEmpty as bool) {
    def bad as DestScan init DestScan{ok: false, url: "", end: $j};
    def n as int init len($cs);
    if ($j < $n and $cs[$j] == "<") {
        def k as int init $j + 1;
        while ($k < $n and $cs[$k] != ">") {
            if ($cs[$k] == "\n" or $cs[$k] == "<") {
                return $bad;
            }
            if ($cs[$k] == "\\" and $k + 1 < $n) {
                $k = $k + 1;
            }
            $k = $k + 1;
        }
        if ($k >= $n) {
            return $bad;
        }
        return DestScan{
            ok: true,
            url: unescapeText(strings.join($cs[$j + 1..$k], "")),
            end: $k + 1
        };
    }
    def k as int init $j;
    def depth as int init 0;
    while ($k < $n) {
        def c as string init $cs[$k];
        if ($c == "\\" and $k + 1 < $n and isAsciiPunct($cs[$k + 1])) {
            $k = $k + 2;
            continue;
        }
        if ($c == " " or $c == "\t" or $c == "\n" or $c < " ") {
            break;
        }
        if ($c == "(") {
            $depth = $depth + 1;
            if ($depth > 32) {
                return $bad;
            }
        } elseif ($c == ")") {
            if ($depth == 0) {
                break;
            }
            $depth = $depth - 1;
        }
        $k = $k + 1;
    }
    if ($k == $j or $depth != 0) {
        return $bad;
    }
    return DestScan{ok: true, url: unescapeText(strings.join($cs[$j..$k], "")), end: $k};
}

# scanTitle reads a link title at j: `"..."`, `'...'` or `(...)`.
func scanTitle(cs as list of string, j as int) {
    def bad as TitleScan init TitleScan{ok: false, title: "", end: $j};
    def n as int init len($cs);
    if ($j >= $n) {
        return $bad;
    }
    def open as string init $cs[$j];
    def close as string init $open;
    if ($open == "(") {
        $close = ")";
    } elseif ($open != "\"" and $open != "'") {
        return $bad;
    }
    def k as int init $j + 1;
    while ($k < $n and $cs[$k] != $close) {
        if ($open == "(" and $cs[$k] == "(") {
            return $bad;
        }
        if ($cs[$k] == "\n" and $k + 1 < $n and $cs[$k + 1] == "\n") {
            return $bad;
        }
        if ($cs[$k] == "\\" and $k + 1 < $n) {
            $k = $k + 1;
        }
        $k = $k + 1;
    }
    if ($k >= $n) {
        return $bad;
    }
    return TitleScan{ok: true, title: unescapeText(strings.join($cs[$j + 1..$k], "")), end: $k + 1};
}

func skipWs(cs as list of string, p as int) {
    def j as int init $p;
    while ($j < len($cs) and ($cs[$j] == " " or $cs[$j] == "\t" or $cs[$j] == "\n")) {
        $j = $j + 1;
    }
    return $j;
}

# linkTail reads `(destination "title")` where cs[p] is the `(`.
func linkTail(cs as list of string, p as int) {
    def bad as LinkTail init LinkTail{ok: false, url: "", title: "", end: $p};
    def n as int init len($cs);
    def j as int init skipWs($cs, $p + 1);
    if ($j < $n and $cs[$j] == ")") {
        return LinkTail{ok: true, url: "", title: "", end: $j + 1};
    }
    def d as DestScan init scanDest($cs, $j, true);
    if (not $d.ok) {
        return $bad;
    }
    def k as int init skipWs($cs, $d.end);
    def title as string init "";
    if ($k > $d.end and $k < $n and ($cs[$k] == "\"" or $cs[$k] == "'" or $cs[$k] == "(")) {
        def t as TitleScan init scanTitle($cs, $k);
        if (not $t.ok) {
            return $bad;
        }
        $title = $t.title;
        $k = skipWs($cs, $t.end);
    }
    if ($k >= $n or $cs[$k] != ")") {
        return $bad;
    }
    return LinkTail{ok: true, url: $d.url, title: $title, end: $k + 1};
}

# --- tokens -------------------------------------------------------------

func textTok(s as string) {
    def t as Tok;
    $t.kind = "text";
    $t.text = $s;
    return $t;
}

func nodeTok(n as doctree.Node) {
    def t as Tok;
    $t.kind = "node";
    $t.node = $n;
    return $t;
}

# flush moves the pending text cs[from..to] into a text token.
func flush(sc as IS, cs as list of string, from as int, upto as int) { # lint-disable: L205
    if ($upto > $from) {
        $sc.toks[] = textTok(strings.join($cs[$from..$upto], ""));
    }
    return $sc;
}

# delimTok builds the token for a delimiter run, with CommonMark's flanking
# rules deciding whether it can open and close.
func delimTok(ch as string, len0 as int, before as string, after as string) {
    def t as Tok;
    $t.kind = "delim";
    $t.ch = $ch;
    $t.len0 = $len0;
    $t.remain = $len0;
    def bs as bool init isSpaceChar($before);
    def as2 as bool init isSpaceChar($after);
    def bp as bool init isPunctChar($before);
    def ap as bool init isPunctChar($after);
    def left as bool init not $as2 and (not $ap or $bs or $bp);
    def right as bool init not $bs and (not $bp or $as2 or $ap);
    if ($ch == "_") {
        $t.canOpen = $left and (not $right or $bp);
        $t.canClose = $right and (not $left or $ap);
    } else {
        $t.canOpen = $left;
        $t.canClose = $right;
    }
    return $t;
}

# isDelimRun is true for the delimiter runs this parser pairs: `*` and `_` of
# any length, `~` of one or two, `==` and a single `^`.
func isDelimRun(ch as string, n as int) {
    match ($ch) {
        when "*", "_" { return true; }
        when "~" { return $n <= 2; }
        when "=" { return $n == 2; }
        when "^" { return $n == 1; }
        else { return false; }
    }
}

# --- emphasis -----------------------------------------------------------

# compatible applies CommonMark's rule of three (for `*` / `_`) and the
# same-length rule (for `~`) to an opener / closer pair.
func compatible(o as Tok, c as Tok) {
    if ($o.ch == "~") {
        return $o.len0 == $c.len0;
    }
    if ($o.ch != "*" and $o.ch != "_") {
        return true;
    }
    if (($o.canClose or $c.canOpen) and ($o.len0 + $c.len0) % 3 == 0 and
        not ($o.len0 % 3 == 0 and $c.len0 % 3 == 0)) {
        return false;
    }
    return true;
}

# spanHasSpace is true when the tokens strictly between two token indices hold
# whitespace or anything but text (a subscript / superscript holds plain,
# space-free text).
func spanHasSpace(toks as list of Tok, from as int, upto as int) {
    for (def k in $from + 1..$upto) {
        if ($toks[$k].kind != "text" or regex.matches('\s', $toks[$k].text)) {
            return true;
        }
    }
    return false;
}

func emphasisNode(ch as string, count as int) {
    match ($ch) {
        when "~" { if ($count == 2) {
            return nodeOf("strikethrough");
        }
        return nodeOf("subscript"); }
        when "=" { return nodeOf("highlight"); }
        when "^" { return nodeOf("superscript"); }
        else { if ($count == 2) {
            return nodeOf("strong");
        }
        return nodeOf("emphasis"); }
    }
}

# processEmphasis pairs the delimiter runs above `bottom` on the delimiter
# stack (CommonMark's process_emphasis), recording each pair as an open tag on
# the opener and a close on the closer, then retires them all.
func processEmphasis(sc as IS, bottom as int) { # lint-disable: L205
    def floors as map of string to int init {};
    def cur as int init $bottom;
    while ($cur < len($sc.ds)) {
        if (not $sc.alive[$cur]) {
            $cur = $cur + 1;
            continue;
        }
        def ct as int init $sc.ds[$cur];
        if (not $sc.toks[$ct].canClose) {
            $cur = $cur + 1;
            continue;
        }
        def ch as string init $sc.toks[$ct].ch;
        def key as string init "{$ch}{$sc.toks[$ct].len0 % 3}{$sc.toks[$ct].canOpen}";
        def floor as int init $bottom;
        if (maps.has($floors, $key) and $floors[$key] > $floor) {
            $floor = $floors[$key];
        }
        def found as int init -1;
        def j as int init $cur - 1;
        while ($j >= $floor) {
            if ($sc.alive[$j]) {
                def ot as int init $sc.ds[$j];
                if ($sc.toks[$ot].ch == $ch and $sc.toks[$ot].canOpen and
                    compatible($sc.toks[$ot], $sc.toks[$ct])) {
                    if (($ch != "^" and $ch != "~") or $sc.toks[$ot].len0 == 2 or
                        not spanHasSpace($sc.toks, $ot, $ct)) {
                        $found = $j;
                        break;
                    }
                }
            }
            $j = $j - 1;
        }
        if ($found < 0) {
            $floors[$key] = $cur;
            if (not $sc.toks[$ct].canOpen) {
                $sc.alive[$cur] = false;
            }
            $cur = $cur + 1;
            continue;
        }
        def ot2 as int init $sc.ds[$found];
        def count as int init 1;
        if ($ch == "*" or $ch == "_") {
            if ($sc.toks[$ot2].remain >= 2 and $sc.toks[$ct].remain >= 2) {
                $count = 2;
            }
        } elseif ($ch == "~" or $ch == "=") {
            $count = $sc.toks[$ot2].len0;
        }
        def tag as doctree.Node init emphasisNode($ch, $count);
        def opens as list of doctree.Node init [$tag];
        for (def o in $sc.toks[$ot2].opens) {
            $opens[] = $o;
        }
        $sc.toks[$ot2].opens = $opens;
        $sc.toks[$ct].closes = $sc.toks[$ct].closes + 1;
        for (def m in $found + 1..$cur) {
            $sc.alive[$m] = false;
        }
        $sc.toks[$ot2].remain = $sc.toks[$ot2].remain - $count;
        $sc.toks[$ct].remain = $sc.toks[$ct].remain - $count;
        if ($sc.toks[$ot2].remain == 0) {
            $sc.alive[$found] = false;
        }
        if ($sc.toks[$ct].remain == 0) {
            $sc.alive[$cur] = false;
            $cur = $cur + 1;
        }
    }
    for (def m in $bottom..len($sc.ds)) {
        $sc.alive[$m] = false;
    }
    return $sc;
}

# --- the scan -----------------------------------------------------------

# backtickRuns lists the maximal runs of backticks in the text as parallel
# start / length lists.
def struct Runs {
    starts as list of int,
    lens as list of int
};

func backtickRuns(cs as list of string, ps as list of int) {
    def r as Runs;
    def k as int init 0;
    while ($k < len($ps)) {
        if ($cs[$ps[$k]] != "`") {
            $k = $k + 1;
            continue;
        }
        def s as int init $ps[$k];
        def e as int init $s;
        while ($e < len($cs) and $cs[$e] == "`") {
            $e = $e + 1;
        }
        $r.starts[] = $s;
        $r.lens[] = $e - $s;
        while ($k < len($ps) and $ps[$k] < $e) {
            $k = $k + 1;
        }
    }
    return $r;
}

# codeSpanText normalises a code span's content: line endings become spaces,
# and one space comes off each end when both ends have one (and it is not all
# spaces).
func codeSpanText(s as string) {
    def t as string init strings.replace($s, "\n", " ");
    if (len($t) >= 2 and strings.startsWith($t, " ") and strings.endsWith($t, " ") and
        strings.trim($t) != "") {
        return $t[1..len($t) - 1];
    }
    return $t;
}

# parseInline turns a block's raw text into inline nodes. `refs` holds the
# document's link reference definitions.
func parseInline(s as string, refs as map of string to list of string) {
    if (not regex.matches('[\\`*_~=^!\[\]<&\n]', $s)) {
        if ($s == "") {
            return [];
        }
        return [textNode($s)];
    }
    def cs as list of string init strings.chars($s);
    def n as int init len($cs);
    def ps as list of int init positionsOf($s, '[\\`*_~=^!\[\]<&\n]');
    def runs as Runs init backtickRuns($cs, $ps);
    # Backtick runs grouped by length, each list ascending, with a cursor per
    # length: closers are found in one forward pass however many openers fail.
    def byLen as map of string to list of int init {};
    for (def r in 0..len($runs.starts)) {
        def lk as string init "{$runs.lens[$r]}";
        if (not maps.has($byLen, $lk)) {
            $byLen[$lk] = [];
        }
        $byLen[$lk][] = $r;
    }
    def cursor as map of string to int init {};
    def gts as list of int init positionsOf($s, '>');
    def cg as int init 0;
    def sc as IS;
    def i as int init 0;
    def buf as int init 0;
    def ai as int init 0;
    $ps[] = $n;
    while ($i < $n) {
        while ($ps[$ai] < $i) {
            $ai = $ai + 1;
        }
        if ($ps[$ai] > $i) {
            $i = $ps[$ai];
            if ($i >= $n) {
                break;
            }
        }
        def c as string init $cs[$i];
        if ($c == "\\") {
            if ($i + 1 < $n and $cs[$i + 1] == "\n") {
                $sc = flush($sc, $cs, $buf, $i);
                $sc.toks[] = nodeTok(nodeOf("line_break"));
                $i = skipSpaces($cs, $i + 2);
                $buf = $i;
                continue;
            }
            if ($i + 1 < $n and isAsciiPunct($cs[$i + 1])) {
                $sc = flush($sc, $cs, $buf, $i);
                $sc.toks[] = textTok($cs[$i + 1]);
                $i = $i + 2;
                $buf = $i;
                continue;
            }
            $i = $i + 1;
            continue;
        }
        if ($c == "`") {
            def e as int init $i;
            while ($e < $n and $cs[$e] == "`") {
                $e = $e + 1;
            }
            def l as int init $e - $i;
            def lk as string init "{$l}";
            def close as int init -1;
            if (maps.has($byLen, $lk)) {
                def p as int init 0;
                if (maps.has($cursor, $lk)) {
                    $p = $cursor[$lk];
                }
                def cnt as int init len($byLen[$lk]);
                while ($p < $cnt and $runs.starts[$byLen[$lk][$p]] < $e) {
                    $p = $p + 1;
                }
                $cursor[$lk] = $p;
                if ($p < $cnt) {
                    $close = $runs.starts[$byLen[$lk][$p]];
                }
            }
            if ($close < 0) {
                $i = $e;
                continue;
            }
            $sc = flush($sc, $cs, $buf, $i);
            def cn as doctree.Node init nodeOf("codespan");
            $cn.text = codeSpanText(strings.join($cs[$e..$close], ""));
            $sc.toks[] = nodeTok($cn);
            $i = $close + $l;
            $buf = $i;
            continue;
        }
        if ($c == "*" or $c == "_" or $c == "~" or $c == "=" or $c == "^") {
            def e as int init $i;
            while ($e < $n and $cs[$e] == $c) {
                $e = $e + 1;
            }
            if (not isDelimRun($c, $e - $i)) {
                $i = $e;
                continue;
            }
            $sc = flush($sc, $cs, $buf, $i);
            def before as string init "\n";
            if ($i > 0) {
                $before = $cs[$i - 1];
            }
            def after as string init "\n";
            if ($e < $n) {
                $after = $cs[$e];
            }
            def dt as Tok init delimTok($c, $e - $i, $before, $after);
            $sc.toks[] = $dt;
            if ($dt.canOpen or $dt.canClose) {
                $sc.ds[] = len($sc.toks) - 1;
                $sc.alive[] = true;
            } else {
                $sc.toks[len($sc.toks) - 1] = textTok(strings.join($cs[$i..$e], ""));
            }
            $i = $e;
            $buf = $i;
            continue;
        }
        if ($c == "!" or $c == "[") {
            def image as bool init $c == "!";
            if ($image and ($i + 1 >= $n or $cs[$i + 1] != "[")) {
                $i = $i + 1;
                continue;
            }
            $sc = flush($sc, $cs, $buf, $i);
            def w as int init 1;
            if ($image) {
                $w = 2;
            }
            $sc.toks[] = textTok(strings.join($cs[$i..$i + $w], ""));
            def br as Bracket init Bracket{
                tok: len($sc.toks) - 1,
                image: $image,
                active: true,
                dsLen: len($sc.ds),
                pos: $i + $w
            };
            if ($sc.nbr < len($sc.brackets)) {
                $sc.brackets[$sc.nbr] = $br;
            } else {
                $sc.brackets[] = $br;
            }
            $sc.nbr = $sc.nbr + 1;
            $i = $i + $w;
            $buf = $i;
            continue;
        }
        if ($c == "]") {
            if ($sc.nbr == 0) {
                $i = $i + 1;
                continue;
            }
            def br as Bracket init $sc.brackets[$sc.nbr - 1];
            $sc.nbr = $sc.nbr - 1;
            if (not $br.active) {
                $i = $i + 1;
                continue;
            }
            # A footnote reference `[^label]` to a defined footnote (refs holds
            # the defined labels under a `^` key) is one node.
            if (not $br.image and $br.pos < $i and $cs[$br.pos] == "^") {
                def fkey as string init "^" + normLabel(strings.join($cs[$br.pos + 1..$i], ""));
                if (maps.has($refs, $fkey)) {
                    def fr as doctree.Node init nodeOf("footnote_ref");
                    $fr.attrs = {"md-label": $refs[$fkey][0]};
                    $sc = flush($sc, $cs, $buf, $i);
                    $sc.toks[$br.tok] = nodeTok($fr);
                    for (def q in $br.tok + 1..len($sc.toks)) {
                        $sc.toks[$q] = textTok("");
                    }
                    for (def q in $br.dsLen..len($sc.ds)) {
                        $sc.alive[$q] = false;
                    }
                    $i = $i + 1;
                    $buf = $i;
                    continue;
                }
            }
            def url as string init "";
            def title as string init "";
            def end as int init -1;
            if ($i + 1 < $n and $cs[$i + 1] == "(") {
                def lt as LinkTail init linkTail($cs, $i + 1);
                if ($lt.ok) {
                    $url = $lt.url;
                    $title = $lt.title;
                    $end = $lt.end;
                }
            }
            if ($end < 0 and $i - $br.pos <= 999) {
                def key as string init normLabel(strings.join($cs[$br.pos..$i], ""));
                def after as int init $i + 1;
                if ($i + 1 < $n and $cs[$i + 1] == "[") {
                    def lab as LabelScan init scanLabel($cs, $i + 1);
                    if ($lab.ok) {
                        $after = $lab.end;
                        if (strings.trim($lab.label) != "") {
                            $key = normLabel($lab.label);
                        }
                    }
                }
                if ($key != "" and maps.has($refs, $key)) {
                    $url = $refs[$key][0];
                    $title = $refs[$key][1];
                    $end = $after;
                }
            }
            if ($end < 0) {
                $i = $i + 1;
                continue;
            }
            $sc = flush($sc, $cs, $buf, $i);
            $sc = processEmphasis($sc, $br.dsLen);
            def tmpl as doctree.Node init nodeOf("link");
            if ($br.image) {
                $tmpl = nodeOf("image");
            }
            $tmpl.url = $url;
            $tmpl.title = $title;
            def ae as int init attrBraceEnd($cs, $end, $n);
            if ($ae >= 0) {
                $tmpl.attrs = parseAttrList(strings.join($cs[$end + 1..$ae], ""));
                $end = $ae + 1;
            }
            $sc.toks[$br.tok].kind = "open";
            $sc.toks[$br.tok].node = $tmpl;
            def closeT as Tok;
            $closeT.kind = "close";
            $sc.toks[] = $closeT;
            if (not $br.image) {
                # No links inside a link: the `[` openers below go inactive.
                for (def b in 0..$sc.nbr) {
                    if (not $sc.brackets[$b].image) {
                        $sc.brackets[$b].active = false;
                    }
                }
            }
            $i = $end;
            $buf = $i;
            continue;
        }
        if ($c == "<") {
            while ($cg < len($gts) and $gts[$cg] < $i) {
                $cg = $cg + 1;
            }
            if ($cg >= len($gts)) {
                $i = $i + 1;
                continue;
            }
            def gt as int init $gts[$cg];
            def win as string init strings.join($cs[$i..$gt + 1], "");
            def um as regex.Match init regex.find(URI_AUTOLINK_RE, $win);
            def em as regex.Match init regex.find(EMAIL_AUTOLINK_RE, $win);
            if ($um.start == 0 or $em.start == 0) {
                $sc = flush($sc, $cs, $buf, $i);
                def ln as doctree.Node init nodeOf("link");
                if ($um.start == 0) {
                    $ln.url = $um.groups[0];
                    $ln.children = [textNode($um.groups[0])];
                } else {
                    $ln.url = "mailto:" + $em.groups[0];
                    $ln.children = [textNode($em.groups[0])];
                }
                $sc.toks[] = nodeTok($ln);
                $i = $gt + 1;
                $buf = $i;
                continue;
            }
            def hend as int init rawHtmlEnd($s, $win, $i);
            # A quoted attribute value may hold a `>`: try the next few `>` too.
            def more as int init $cg + 1;
            while ($hend == $i and $more < len($gts) and $more <= $cg + 8 and
                strings.startsWith($win, "<") and not strings.startsWith($win, "<!")) {
                $hend = rawHtmlEnd($s, strings.join($cs[$i..$gts[$more] + 1], ""), $i);
                $more = $more + 1;
            }
            if ($hend > $i) {
                # Raw HTML: one html_inline node, its markup not parsed (the
                # renderer escapes it unless raw HTML is allowed).
                $sc = flush($sc, $cs, $buf, $i);
                def hn as doctree.Node init nodeOf("html_inline");
                $hn.text = strings.join($cs[$i..$hend], "");
                $sc.toks[] = nodeTok($hn);
                $i = $hend;
                $buf = $i;
                continue;
            }
            $i = $i + 1;
            continue;
        }
        if ($c == "&") {
            def lim as int init math.min($n, $i + 34);
            def semi as int init -1;
            for (def q in $i + 1..$lim) {
                if ($cs[$q] == ";") {
                    $semi = $q;
                    break;
                }
            }
            if ($semi > $i + 1) {
                def name as string init strings.join($cs[$i + 1..$semi], "");
                def ch2 as string init "";
                if (regex.matches(
                    '^(?:#[0-9]{1,7}|#[xX][0-9a-fA-F]{1,6}|[A-Za-z][A-Za-z0-9]{1,31})$',
                    $name)) {
                    $ch2 = entityChar($name);
                    if ($ch2 == "" and strings.startsWith($name, "#")) {
                        $ch2 = "�";
                    }
                }
                if ($ch2 != "") {
                    $sc = flush($sc, $cs, $buf, $i);
                    $sc.toks[] = textTok($ch2);
                    $i = $semi + 1;
                    $buf = $i;
                    continue;
                }
            }
            $i = $i + 1;
            continue;
        }
        if ($c == "\n") {
            # Trailing spaces end the line; two or more make a hard break.
            def tail as int init $i;
            while ($tail > $buf and $cs[$tail - 1] == " ") {
                $tail = $tail - 1;
            }
            def hard as bool init $i - $tail >= 2;
            $sc = flush($sc, $cs, $buf, $tail);
            if ($hard) {
                $sc.toks[] = nodeTok(nodeOf("line_break"));
            } else {
                $sc.toks[] = textTok(" ");
            }
            $i = skipSpaces($cs, $i + 1);
            $buf = $i;
            continue;
        }
        $i = $i + 1;
    }
    $sc = flush($sc, $cs, $buf, $n);
    $sc = processEmphasis($sc, 0);
    return buildInline($sc.toks);
}

# rawHtmlEnd is the end of the raw inline HTML (a tag, comment, processing
# instruction, declaration or CDATA section) starting at i, or i when there is
# none. `win` is the text from i to the next `>`.
func rawHtmlEnd(s as string, win as string, i as int) {
    if (strings.startsWith($win, "<!--")) {
        if (strings.startsWith($win, "<!-->") or strings.startsWith($win, "<!--->")) {
            return $i + len($win);
        }
        def e as int init strings.indexOf($s, "-->", $i + 4);
        if ($e < 0) {
            return $i;
        }
        return $e + 3;
    }
    if (strings.startsWith($win, "<?")) {
        def e as int init strings.indexOf($s, "?>", $i + 2);
        if ($e < 0) {
            return $i;
        }
        return $e + 2;
    }
    if (strings.startsWith($win, "<![CDATA[")) {
        def e as int init strings.indexOf($s, "]]>", $i + 9);
        if ($e < 0) {
            return $i;
        }
        return $e + 3;
    }
    if (regex.matches(INLINE_TAG_RE, $win)) {
        return $i + len($win);
    }
    return $i;
}

# --- building the nodes -----------------------------------------------

# buildInline nests the tokens' nodes: an "open" or a delimiter's opens push
# a node, a "close" or a delimiter's closes pop one into its parent. An image
# pops to its alt text.
func buildInline(toks as list of Tok) {
    def stk as list of doctree.Node init [nodeOf("root")];
    def d as int init 1;
    def skipped as int init 0;
    for (def t in $toks) {
        if ($t.kind == "text") {
            # Appended in place on the stack top; adjacent text merges.
            if ($t.text == "") {
                continue;
            }
            def lt as int init len($stk[$d - 1].children) - 1;
            if ($lt >= 0 and $stk[$d - 1].children[$lt].kind == "text") {
                $stk[$d - 1].children[$lt].text = $stk[$d - 1].children[$lt].text + $t.text;
            } else {
                $stk[$d - 1].children[] = textNode($t.text);
            }
            continue;
        }
        if ($t.kind == "node") {
            $stk[$d - 1].children[] = $t.node;
            continue;
        }
        def pops as int init $t.closes;
        if ($t.kind == "close") {
            $pops = 1;
        }
        for (def q in 0..$pops) {
            if ($skipped > 0) {
                $skipped = $skipped - 1;
                continue;
            }
            if ($d <= 1) {
                break;
            }
            def done as doctree.Node init $stk[$d - 1];
            $d = $d - 1;
            if ($done.kind == "image") {
                $done.text = doctree.text($done);
                $done.children = [];
            }
            $stk[$d - 1].children[] = $done;
        }
        if ($t.kind == "delim" and $t.remain > 0) {
            def rt as string init strings.repeat($t.ch, $t.remain);
            def lr as int init len($stk[$d - 1].children) - 1;
            if ($lr >= 0 and $stk[$d - 1].children[$lr].kind == "text") {
                $stk[$d - 1].children[$lr].text = $stk[$d - 1].children[$lr].text + $rt;
            } else {
                $stk[$d - 1].children[] = textNode($rt);
            }
        }
        def pushes as list of doctree.Node init $t.opens;
        if ($t.kind == "open") {
            $pushes = [$t.node];
        }
        for (def o in $pushes) {
            if ($d > INLINE_MAX_DEPTH) {
                $skipped = $skipped + 1;
                continue;
            }
            if ($d < len($stk)) {
                $stk[$d] = $o;
            } else {
                $stk[] = $o;
            }
            $d = $d + 1;
        }
    }
    return $stk[0].children;
}
