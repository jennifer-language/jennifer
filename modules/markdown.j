# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

# A hand-rolled CommonMark parser: its line loop and inline scanner
# legitimately run past the L201 statement-count limit. Every other lint check
# stays active.
# lint-disable-file: L201

/**
 * A CommonMark reader. `parse` turns Markdown into a `doctree` document tree
 * (`doctree.Node`), which the doctree module walks and renders to HTML, XHTML,
 * styled terminal text and PDF; `toHtml` / `toXhtml` / `toAnsi` / `toPdf` here
 * are shortcuts from Markdown text. It also authors Markdown text (header /
 * style / link / list / codeBlock / table) and aligns table source
 * (`tablePretty`).
 *
 * The parser follows CommonMark's block and inline algorithms: block quotes
 * and list items are containers holding any blocks, with lazy continuation and
 * tight / loose lists; ATX and setext headings; indented and fenced code; HTML
 * blocks; thematic breaks; link reference definitions; emphasis by the
 * delimiter rules; code spans; inline, reference and autolinks; images;
 * backslash escapes; entities; hard and soft line breaks. Extensions: GFM
 * tables, task lists and `~~strikethrough~~`; `~sub~`, `^sup^`, `==mark==`;
 * definition lists (`term` then `: definition`); GitHub alerts (`> [!NOTE]`)
 * as admonitions; `{#id .class key="v"}` attribute lists on headings, links
 * and images. Raw inline HTML is recognised but kept as text. Pure Jennifer.
 * @module markdown
 * @example
 * import "markdown.j" as markdown;
 * io.printf("%s\n", markdown.toHtml("# Hi\n\nA **bold** word.\n\n> a quote"));
 * io.printf("%s\n", markdown.toAnsi("- one\n  - nested\n- two"));
 */
import "./doctree.j" as doctree;
use strings;
use regex;
use convert;
use lists;
use maps;
use math;

# positionsOf returns the rune index of every match of pattern in s, ascending.
# pattern must match exactly one character: the positions are the running sum of
# the pieces regex.split leaves between matches, which is cheaper than a Match
# struct per hit from regex.findAll.
func positionsOf(s as string, pattern as string) {
    def parts as list of string init regex.split($pattern, $s);
    def out as list of int init [];
    def pos as int init 0;
    for (def k in 0..len($parts) - 1) {
        $pos = $pos + len($parts[$k]);
        $out[] = $pos;
        $pos = $pos + 1;
    }
    return $out;
}

# AttrSplit is a heading's text with a trailing `{...}` attribute list peeled off.
def struct AttrSplit {
    text as string,
    attrs as map of string to string
};

# attrTokens splits an attribute-list body on whitespace, keeping a quoted value
# (`key="a b"`) as one token.
func attrTokens(body as string) {
    def cs as list of string init strings.chars($body);
    def n as int init len($cs);
    def toks as list of string init [];
    def cur as list of string init [];
    def inQuote as bool init false;
    for (def i in 0..$n) {
        def c as string init $cs[$i];
        if ($c == "\"") {
            $inQuote = not $inQuote;
            $cur[] = $c;
        } elseif (($c == " " or $c == "\t") and not $inQuote) {
            if (len($cur) > 0) {
                $toks[] = strings.join($cur, "");
                $cur = [];
            }
        } else {
            $cur[] = $c;
        }
    }
    if (len($cur) > 0) {
        $toks[] = strings.join($cur, "");
    }
    return $toks;
}

# parseAttrList parses an attribute-list body (`#id .class key="value"`): `#` sets
# id, `.` appends to a space-joined class, `key=value` (quotes optional) sets a
# named attribute. Returns the attrs map (empty when nothing valid is found).
func parseAttrList(body as string) {
    def attrs as map of string to string init {};
    def classes as list of string init [];
    for (def t in attrTokens($body)) {
        if (strings.startsWith($t, "#") and len($t) > 1) {
            $attrs["id"] = $t[1..];
        } elseif (strings.startsWith($t, ".") and len($t) > 1) {
            $classes[] = $t[1..];
        } else {
            def eq as int init strings.indexOf($t, "=");
            if ($eq > 0) {
                def k as string init $t[..$eq];
                def v as string init $t[$eq + 1..];
                if (len($v) >= 2 and strings.startsWith($v, "\"") and strings.endsWith($v, "\"")) {
                    $v = $v[1..len($v) - 1];
                }
                $attrs[$k] = $v;
            }
        }
    }
    if (len($classes) > 0) {
        $attrs["class"] = strings.join($classes, " ");
    }
    return $attrs;
}

# attrBraceEnd: when cs[j] opens an inline attribute list ("{"), the index of its
# closing "}" (no nesting - a stray inner "{" voids it), else -1.
func attrBraceEnd(cs as list of string, j as int, n as int) {
    if ($j >= $n or $cs[$j] != '{') {
        return -1;
    }
    for (def k in $j + 1..$n) {
        if ($cs[$k] == '}') {
            return $k;
        }
        if ($cs[$k] == '{') {
            return -1;
        }
    }
    return -1;
}

# splitTrailingAttrs peels a trailing `{...}` attribute list off a heading's text
# (`## H {#id .cls}`). Returns the text without it and the parsed attrs; with no
# valid trailing list the text is unchanged and attrs is empty.
func splitTrailingAttrs(s as string) {
    def none as AttrSplit init AttrSplit{text: $s, attrs: {}};
    def cs as list of string init strings.chars(strings.trimRight($s));
    def n as int init len($cs);
    if ($n == 0 or $cs[$n - 1] != '}') {
        return $none;
    }
    def open as int init -1;
    for (def k as int init $n - 2; $k >= 0 and $open < 0; $k = $k - 1) {
        if ($cs[$k] == '{') {
            $open = $k;
        }
    }
    if ($open < 0) {
        return $none;
    }
    def attrs as map of string to string init parseAttrList(strings.join(
        $cs[$open + 1..$n - 1],
        ""));
    if (len(maps.keys($attrs)) == 0) {
        return $none;
    }
    def before as string init strings.trimRight(strings.join($cs[..$open], ""));
    return AttrSplit{text: $before, attrs: $attrs};
}

# splitCells splits one table row into trimmed cell strings, dropping an
# optional leading / trailing `|` and treating `\|` as a literal pipe.
func splitCells(row as string) {
    # Without a backslash no pipe is escaped: a plain split does it natively.
    if (not strings.contains($row, "\\")) {
        def t as string init strings.trim($row);
        if (strings.startsWith($t, "|")) {
            $t = $t[1..];
        }
        if (strings.endsWith($t, "|")) {
            $t = $t[..len($t) - 1];
        }
        def fast as list of string init [];
        for (def part in strings.split($t, "|")) {
            $fast[] = strings.trim($part);
        }
        return $fast;
    }
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
    def buf as list of string init [];
    def i as int init $start;
    while ($i < $end) {
        def c as string init $cs[$i];
        if ($c == "\\" and $i + 1 < $end and $cs[$i + 1] == "|") {
            $buf[] = "|";
            $i = $i + 2;
            continue;
        }
        if ($c == "|") {
            $cells[] = strings.trim(strings.join($buf, ""));
            $buf = [];
            $i = $i + 1;
            continue;
        }
        $buf[] = $c;
        $i = $i + 1;
    }
    $cells[] = strings.trim(strings.join($buf, ""));
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

# looksLikeTable reports whether `row` opens a table: a pipe-bearing header row
# over a delimiter row of the same column count. Takes the two lines rather than
# the whole list plus an index - a caller does this once per plain line, so
# binding the entire line list by value here (Jennifer's value semantics) made
# the parse quadratic in document length. `next` is "" at end of document, which
# is not a delimiter row, so a trailing pipe row correctly does not open a table.
func looksLikeTable(row as string, next as string) {
    if (not strings.contains($row, "|")) {
        return false;
    }
    if (not isDelimiterRow($next)) {
        return false;
    }
    return len(splitCells($row)) == len(parseAligns($next));
}

# The five admonition kinds, spelled as they are written. GitHub calls these
# alerts; the rest of the world calls the box they produce an admonition. A
# sixth word is a quotation that happens to start with a bracket, which is what
# every renderer that has never heard of the syntax makes of all of them.
def const ALERT_KINDS as list of string init ["note", "tip", "important", "warning", "caution"];

# What an admonition marker line opens: the kind, and the custom title that
# followed it on the same line. An empty kind means the line is not a marker.
def struct Alert {
    kind as string,
    title as string
};

# alertMarker reads `[!NOTE]`, optionally followed by a title, off the first line
# of a blockquote.
#
# This runs while the source is still lines, which is the whole reason the parser
# does it rather than a consumer: by the time the paragraph exists, the line
# break after the marker has been folded into a space and `> [!NOTE] Mind the
# gap` is indistinguishable from a marker with a body on the next line.
#
# The marker has to be the whole of what precedes the title, and the kind has to
# be one of the five: `[!NOTE]:` opening a quotation is prose, and so is
# `[!NOTES]`.
func alertMarker(line as string) {
    def none as Alert init Alert{kind: "", title: ""};
    def trimmed as string init strings.trim($line);
    if (not strings.startsWith($trimmed, "[!")) {
        return $none;
    }
    def close as int init strings.indexOf($trimmed, "]");
    if ($close < 3) {
        return $none;
    }
    def kind as string init strings.lower($trimmed[2..$close]);
    if (not lists.contains(ALERT_KINDS, $kind)) {
        return $none;
    }
    def rest as string init $trimmed[$close + 1..];
    if ($rest != "" and not strings.startsWith($rest, " ")) {
        return $none;
    }
    return Alert{kind: $kind, title: strings.trim($rest)};
}

# --- limits and errors ----------------------------------------------

# The nesting-depth cap and total-node budget that turn a pathological document
# (deeply nested quotes / lists, or a huge input) into a catchable "limit" error
# rather than an unbounded recursion or allocation.
def const MAX_DOC_DEPTH as int init 100;
def const MAX_DOC_NODES as int init 200000;

func mdFail(msg as string) {
    throw Error{kind: "markdown", message: $msg};
}

# mdLimit raises a reached budget (nesting depth, node count): kind "limit",
# the generic kind for a cap, with the message as mdFail would write it.
func mdLimit(msg as string) {
    throw Error{kind: "limit", message: $msg};
}

# nodeOf returns a zero-valued doctree.Node with only its kind set; callers fill the fields
# that kind uses (value semantics - each call is a fresh node).
func nodeOf(kind as string) {
    def n as doctree.Node;
    $n.kind = $kind;
    return $n;
}

func textNode(s as string) {
    def n as doctree.Node init nodeOf("text");
    $n.text = $s;
    return $n;
}

# The HTML 4 named entities (name -> Unicode code point), plus `apos`: the
# Latin-1, Greek, math and typographic set ordinary prose uses. Not HTML5's
# full ~2200-entry table; an unknown name stays literal text. Stored as code
# points (not literal characters) so this source stays ASCII; decodeEntities
# turns them into the actual character.
def const NAMED_ENTITIES as map of string to int init {
    "Aacute": 193,
    "aacute": 225,
    "Acirc": 194,
    "acirc": 226,
    "acute": 180,
    "AElig": 198,
    "aelig": 230,
    "Agrave": 192,
    "agrave": 224,
    "alefsym": 8501,
    "Alpha": 913,
    "alpha": 945,
    "amp": 38,
    "and": 8743,
    "ang": 8736,
    "apos": 39,
    "Aring": 197,
    "aring": 229,
    "asymp": 8776,
    "Atilde": 195,
    "atilde": 227,
    "Auml": 196,
    "auml": 228,
    "bdquo": 8222,
    "Beta": 914,
    "beta": 946,
    "brvbar": 166,
    "bull": 8226,
    "cap": 8745,
    "Ccedil": 199,
    "ccedil": 231,
    "cedil": 184,
    "cent": 162,
    "Chi": 935,
    "chi": 967,
    "circ": 710,
    "clubs": 9827,
    "cong": 8773,
    "copy": 169,
    "crarr": 8629,
    "cup": 8746,
    "curren": 164,
    "Dagger": 8225,
    "dagger": 8224,
    "dArr": 8659,
    "darr": 8595,
    "deg": 176,
    "Delta": 916,
    "delta": 948,
    "diams": 9830,
    "divide": 247,
    "Eacute": 201,
    "eacute": 233,
    "Ecirc": 202,
    "ecirc": 234,
    "Egrave": 200,
    "egrave": 232,
    "empty": 8709,
    "emsp": 8195,
    "ensp": 8194,
    "Epsilon": 917,
    "epsilon": 949,
    "equiv": 8801,
    "Eta": 919,
    "eta": 951,
    "ETH": 208,
    "eth": 240,
    "Euml": 203,
    "euml": 235,
    "euro": 8364,
    "exist": 8707,
    "fnof": 402,
    "forall": 8704,
    "frac12": 189,
    "frac14": 188,
    "frac34": 190,
    "frasl": 8260,
    "Gamma": 915,
    "gamma": 947,
    "ge": 8805,
    "gt": 62,
    "hArr": 8660,
    "harr": 8596,
    "hearts": 9829,
    "hellip": 8230,
    "Iacute": 205,
    "iacute": 237,
    "Icirc": 206,
    "icirc": 238,
    "iexcl": 161,
    "Igrave": 204,
    "igrave": 236,
    "image": 8465,
    "infin": 8734,
    "int": 8747,
    "Iota": 921,
    "iota": 953,
    "iquest": 191,
    "isin": 8712,
    "Iuml": 207,
    "iuml": 239,
    "Kappa": 922,
    "kappa": 954,
    "Lambda": 923,
    "lambda": 955,
    "lang": 9001,
    "laquo": 171,
    "lArr": 8656,
    "larr": 8592,
    "lceil": 8968,
    "ldquo": 8220,
    "le": 8804,
    "lfloor": 8970,
    "lowast": 8727,
    "loz": 9674,
    "lrm": 8206,
    "lsaquo": 8249,
    "lsquo": 8216,
    "lt": 60,
    "macr": 175,
    "mdash": 8212,
    "micro": 181,
    "middot": 183,
    "minus": 8722,
    "Mu": 924,
    "mu": 956,
    "nabla": 8711,
    "nbsp": 160,
    "ndash": 8211,
    "ne": 8800,
    "ni": 8715,
    "not": 172,
    "notin": 8713,
    "nsub": 8836,
    "Ntilde": 209,
    "ntilde": 241,
    "Nu": 925,
    "nu": 957,
    "Oacute": 211,
    "oacute": 243,
    "Ocirc": 212,
    "ocirc": 244,
    "OElig": 338,
    "oelig": 339,
    "Ograve": 210,
    "ograve": 242,
    "oline": 8254,
    "Omega": 937,
    "omega": 969,
    "Omicron": 927,
    "omicron": 959,
    "oplus": 8853,
    "or": 8744,
    "ordf": 170,
    "ordm": 186,
    "Oslash": 216,
    "oslash": 248,
    "Otilde": 213,
    "otilde": 245,
    "otimes": 8855,
    "Ouml": 214,
    "ouml": 246,
    "para": 182,
    "part": 8706,
    "permil": 8240,
    "perp": 8869,
    "Phi": 934,
    "phi": 966,
    "Pi": 928,
    "pi": 960,
    "piv": 982,
    "plusmn": 177,
    "pound": 163,
    "Prime": 8243,
    "prime": 8242,
    "prod": 8719,
    "prop": 8733,
    "Psi": 936,
    "psi": 968,
    "quot": 34,
    "radic": 8730,
    "rang": 9002,
    "raquo": 187,
    "rArr": 8658,
    "rarr": 8594,
    "rceil": 8969,
    "rdquo": 8221,
    "real": 8476,
    "reg": 174,
    "rfloor": 8971,
    "Rho": 929,
    "rho": 961,
    "rlm": 8207,
    "rsaquo": 8250,
    "rsquo": 8217,
    "sbquo": 8218,
    "Scaron": 352,
    "scaron": 353,
    "sdot": 8901,
    "sect": 167,
    "shy": 173,
    "Sigma": 931,
    "sigma": 963,
    "sigmaf": 962,
    "sim": 8764,
    "spades": 9824,
    "sub": 8834,
    "sube": 8838,
    "sum": 8721,
    "sup": 8835,
    "sup1": 185,
    "sup2": 178,
    "sup3": 179,
    "supe": 8839,
    "szlig": 223,
    "Tau": 932,
    "tau": 964,
    "there4": 8756,
    "Theta": 920,
    "theta": 952,
    "thetasym": 977,
    "thinsp": 8201,
    "THORN": 222,
    "thorn": 254,
    "tilde": 732,
    "times": 215,
    "trade": 8482,
    "Uacute": 218,
    "uacute": 250,
    "uArr": 8657,
    "uarr": 8593,
    "Ucirc": 219,
    "ucirc": 251,
    "Ugrave": 217,
    "ugrave": 249,
    "uml": 168,
    "upsih": 978,
    "Upsilon": 933,
    "upsilon": 965,
    "Uuml": 220,
    "uuml": 252,
    "weierp": 8472,
    "Xi": 926,
    "xi": 958,
    "Yacute": 221,
    "yacute": 253,
    "yen": 165,
    "Yuml": 376,
    "yuml": 255,
    "Zeta": 918,
    "zeta": 950,
    "zwj": 8205,
    "zwnj": 8204
};

# entityChar decodes one entity name (the text between `&` and `;`) to its
# character, or "" when it is not a recognised numeric or common-named reference.
# A numeric reference out of the Unicode range (or a surrogate) is treated as
# unrecognised, so a crafted `&#...;` can never produce an invalid value.
func entityChar(name as string) {
    if (len($name) == 0) {
        return "";
    }
    if (strings.startsWith($name, "#")) {
        def rest as string init $name[1..];
        def cp as int init -1;
        try {
            if (strings.startsWith($rest, "x") or strings.startsWith($rest, "X")) {
                $cp = convert.toInt($rest[1..], 16);
            } else {
                $cp = convert.toInt($rest, 10);
            }
        } catch (e) {
            return "";
        }
        if ($cp < 0 or $cp > 1114111) {
            return "";
        }
        if ($cp == 0) {
            # CommonMark: a NUL reference is the replacement character.
            return "\uFFFD";
        }
        try {
            return convert.fromCodepoint($cp);
        } catch (e2) {
            return "";
        }
    }
    if (maps.has(NAMED_ENTITIES, $name)) {
        return convert.fromCodepoint(NAMED_ENTITIES[$name]);
    }
    return "";
}

# decodeEntities replaces every recognised HTML entity reference in `s` with its
# actual character, in one left-to-right pass over the raw text. Decoding to the
# character (rather than passing the reference through) is what keeps it both
# correct and safe: a reference is resolved once here, and the renderer re-escapes
# the result, so `&#60;` becomes `&lt;` (never an injected `<`) and a plain text
# `&copy;` becomes the character (not the double-escaped `&amp;copy;`). Processing
# the raw text in a single pass keeps `&amp;copy;` (an escaped ampersand followed
# by literal text) distinct from `&copy;` (an entity). An unrecognised reference
# is left verbatim (its `&` escapes normally).
func decodeEntities(s as string) {
    if (not strings.contains($s, "&")) {
        return $s;
    }
    def cs as list of string init strings.chars($s);
    def n as int init len($cs);
    def out as list of string init [];
    def i as int init 0;
    while ($i < $n) {
        if ($cs[$i] == "&") {
            # entity names are short: look for the terminating `;` in a bounded window.
            def semi as int init -1;
            def limit as int init $i + 32;
            if ($limit > $n) {
                $limit = $n;
            }
            def j as int init $i + 1;
            while ($j < $limit and $semi < 0) {
                if ($cs[$j] == ";") {
                    $semi = $j;
                }
                $j = $j + 1;
            }
            def decoded as string init "";
            if ($semi > $i + 1) {
                $decoded = entityChar(strings.join($cs[$i + 1..$semi], ""));
            }
            if ($decoded == "") {
                $out[] = "&";
                $i = $i + 1;
            } else {
                $out[] = $decoded;
                $i = $semi + 1;
            }
        } else {
            $out[] = $cs[$i];
            $i = $i + 1;
        }
    }
    return strings.join($out, "");
}

include "./markdown_inline.inc.j";
include "./markdown_blocks.inc.j";

# resolveInline replaces every md_inline placeholder under a node with the
# parsed inline nodes, now that all link reference definitions are known.
func resolveInline(n as doctree.Node, refs as map of string to list of string, depth as int) {
    if ($depth > MAX_DOC_DEPTH) {
        mdLimit("document nesting too deep (over {MAX_DOC_DEPTH} levels)");
    }
    if (len($n.children) == 0) {
        return $n;
    }
    def kids as list of doctree.Node init [];
    for (def c in $n.children) {
        if ($c.kind == "md_inline") {
            for (def x in parseInline($c.text, $refs)) {
                $kids[] = $x;
            }
        } else {
            $kids[] = resolveInline($c, $refs, $depth + 1);
        }
    }
    # Built from the fields rather than copied: a copy of `n` would copy the
    # subtree it is about to replace.
    return doctree.Node{
        kind: $n.kind,
        level: $n.level,
        text: $n.text,
        lang: $n.lang,
        ordered: $n.ordered,
        url: $n.url,
        title: $n.title,
        align: $n.align,
        attrs: $n.attrs,
        children: $kids
    };
}

# The footnote numbering pass: the number each label got (in order of its
# first reference) and how many references each has had.
def struct Notes {
    order as list of string,
    num as map of string to int,
    seen as map of string to int
};

# numberRefs gives each footnote reference under a node its number, link and
# id: footnotes are numbered by first reference, a second reference to the same
# note gets an id of its own (`fnref-1-2`).
func numberRefs(n as doctree.Node, nt as Notes) { # lint-disable: L205
    if (len($n.children) == 0) {
        return $nt;
    }
    for (def c in $n.children) {
        if ($c.kind == "footnote_ref") {
            def key as string init normLabel($c.attrs["md-label"]);
            if (not maps.has($nt.num, $key)) {
                $nt.order[] = $key;
                $nt.num[$key] = len($nt.order);
                $nt.seen[$key] = 0;
            }
            $nt.seen[$key] = $nt.seen[$key] + 1;
        } else {
            $nt = numberRefs($c, $nt);
        }
    }
    return $nt;
}

func refId(num as int, k as int) {
    if ($k <= 1) {
        return "fnref-{$num}";
    }
    return "fnref-{$num}-{$k}";
}

func withKidsOf(n as doctree.Node, kids as list of doctree.Node) {
    return doctree.Node{
        kind: $n.kind,
        level: $n.level,
        text: $n.text,
        lang: $n.lang,
        ordered: $n.ordered,
        url: $n.url,
        title: $n.title,
        align: $n.align,
        attrs: $n.attrs,
        children: $kids
    };
}

# placeFootnotes numbers the footnote references (GFM: by first reference),
# links each to its note and back, and appends the referenced notes to the
# document in that order; a note that is never referenced is dropped. A
# reference inside a note numbers the notes it reaches in turn.
func placeFootnotes(
    doc as doctree.Node,
    notes as map of string to doctree.Node,
    refs as map of string to list of string) {
    def nt as Notes;
    $nt = numberRefs($doc, $nt);
    def bodies as list of doctree.Node init [];
    def k as int init 0;
    while ($k < len($nt.order)) {
        def key as string init $nt.order[$k];
        def body as doctree.Node init resolveInline($notes[$key], $refs, 1);
        $nt = numberRefs($body, $nt);
        $bodies[] = $body;
        $k = $k + 1;
    }
    # Second pass: ids per reference, counted again in document order.
    def counts as Notes init $nt;
    $counts.seen = {};
    for (def key in $nt.order) {
        $counts.seen[$key] = 0;
    }
    def out as list of doctree.Node init [];
    def linked as LinkPass init linkPass($doc, $counts);
    $out = $linked.node.children;
    $counts = $linked.nt;
    for (def j in 0..len($bodies)) {
        def num as int init $j + 1;
        def lp as LinkPass init linkPass($bodies[$j], $counts);
        $counts = $lp.nt;
        def note as doctree.Node init withKidsOf($lp.node, $lp.node.children);
        $note.attrs = {"label": "{$num}", "id": "fn-{$num}", "backref": "fnref-{$num}"};
        $out[] = $note;
    }
    return withKidsOf($doc, $out);
}

# A node with its footnote references linked, and the counts after it.
def struct LinkPass {
    node as doctree.Node,
    nt as Notes
};

# linkPass is linkRefs with the per-label reference counts carried through, so
# the second reference to a note gets the id `fnref-1-2`.
func linkPass(n as doctree.Node, nt as Notes) {
    if (len($n.children) == 0) {
        return LinkPass{node: $n, nt: $nt};
    }
    def cur as Notes init $nt;
    def kids as list of doctree.Node init [];
    for (def c in $n.children) {
        if ($c.kind == "footnote_ref") {
            def key as string init normLabel($c.attrs["md-label"]);
            def num as int init $cur.num[$key];
            $cur.seen[$key] = $cur.seen[$key] + 1;
            def r as doctree.Node init nodeOf("footnote_ref");
            $r.text = "{$num}";
            $r.url = "#fn-{$num}";
            $r.attrs = {"id": refId($num, $cur.seen[$key])};
            $kids[] = $r;
        } else {
            def sub as LinkPass init linkPass($c, $cur);
            $cur = $sub.nt;
            $kids[] = $sub.node;
        }
    }
    return LinkPass{node: withKidsOf($n, $kids), nt: $cur};
}

# countNodes sums the nodes in a subtree. The depth cap already bounds recursion
# depth, so this cannot itself overflow; it enforces the node budget.
func countNodes(n as doctree.Node) {
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
 * @return {doctree.Node} the document root node
 */
export func parse(md as string) {
    def st as BP init blockParse($md);
    def doc as doctree.Node init nodeOf("document");
    $doc.children = $st.open[0].kids;
    def refs as map of string to list of string init $st.refs;
    for (def key in maps.keys($st.notes)) {
        $refs["^" + $key] = [$st.notes[$key].attrs["md-label"], ""];
    }
    $doc = resolveInline($doc, $refs, 0);
    if (len($st.notes) > 0) {
        $doc = placeFootnotes($doc, $st.notes, $refs);
    }
    if (countNodes($doc) > MAX_DOC_NODES) {
        mdLimit("document too large (over {MAX_DOC_NODES} nodes)");
    }
    return $doc;
}

# alignOf returns the alignment for column `i`, or "none" past the list end.
func alignOf(aligns as list of string, i as int) {
    if ($i < len($aligns)) {
        return $aligns[$i];
    }
    return "none";
}

# --- rendering (exported) -------------------------------------------

/**
 * Options for HTML rendering.
 * @field allowRawHtml {bool} pass raw HTML blocks through verbatim (trusted input
 *   only). Default (zero value) is false: raw HTML is escaped, so untrusted
 *   Markdown cannot inject scripts or event handlers.
 */
export def struct HtmlOptions {
    allowRawHtml as bool
};

/**
 * Render Markdown to an HTML string (block elements concatenated, no
 * indentation). **Safe by default:** raw HTML in the source (an HTML block or
 * an inline tag) is escaped and rendered as literal text, so passing
 * untrusted Markdown (a README, a user comment) cannot inject a `<script>` or an
 * event-handler attribute. Everything the renderer builds is already escaped
 * (text, link URLs via an allow-list, image attributes). To pass raw HTML through
 * for **trusted** input, use `toHtmlWith` with `allowRawHtml: true`.
 * @param md {string} the Markdown source
 * @return {string} the rendered HTML
 */
export func toHtml(md as string) {
    return doctree.render("html", parse($md));
}

/**
 * Render Markdown to a well-formed XHTML string: void elements self-close
 * (`<hr />`) and boolean attributes expand, so the output is an XHTML / EPUB
 * content document. Raw HTML in the source is escaped, exactly as `toHtml`;
 * otherwise identical.
 * @param md {string} the Markdown source
 * @return {string} the rendered XHTML
 */
export func toXhtml(md as string) {
    return doctree.render("xhtml", parse($md));
}

/**
 * Render Markdown to HTML with explicit options. The only option today is
 * `allowRawHtml`: set it true to emit raw HTML (blocks and inline tags) verbatim
 * for trusted Markdown, or false (the default `toHtml` behavior) to escape it.
 * @param md {string} the Markdown source
 * @param opts {HtmlOptions} the render options
 * @return {string} the rendered HTML
 */
export func toHtmlWith(md as string, opts as HtmlOptions) {
    # A zero value, not a literal: a literal of another module's struct must name
    # every field unless that module is installed in the system module dir.
    def html as doctree.HtmlOptions;
    $html.allowRawHtml = $opts.allowRawHtml;
    return doctree.renderHtml(parse($md), $html);
}

/**
 * Render Markdown to styled terminal text, blocks separated by a blank line.
 * @param md {string} the Markdown source
 * @return {string} the rendered terminal text
 */
export func toAnsi(md as string) {
    return doctree.render("ansi", parse($md));
}

# --- PDF (exported) -------------------------------------------------

/**
 * Render a Markdown string to PDF bytes with the given options.
 * @param md {string} the Markdown source
 * @param opts {PdfOptions} the page geometry and fonts
 * @return {bytes} the PDF document
 */
export func toPdfWith(md as string, opts as doctree.PdfOptions) {
    return doctree.renderPdf(parse($md), $opts);
}

/**
 * Render a Markdown string to PDF bytes with the default options.
 * @param md {string} the Markdown source
 * @return {bytes} the PDF document
 */
export func toPdf(md as string) {
    return toPdfWith($md, doctree.pdfDefaults());
}

# --- authoring: build Markdown source (exported) -------------------
#
# These produce Markdown *text*, the inverse of toHtml / toAnsi, so a program
# can assemble a document and (round-trip) render it. The text is inserted
# literally: a caller passing Markdown metacharacters is responsible for
# escaping them.

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

# fail raises a catchable value error from an authoring helper.
func fail(msg as string) {
    throw Error{kind: "value", message: $msg};
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
    def out as list of string init [];
    for (def item in $items) {
        $out[] = "- " + $item;
    }
    return strings.join($out, "\n");
}

/**
 * Render an ordered list, `1. item` upward.
 * @param items {list of string} the list items
 * @return {string} the Markdown numbered list
 */
export func numbered(items as list of string) {
    def out as list of string init [];
    def i as int init 1;
    for (def item in $items) {
        $out[] = "{$i}. {$item}";
        $i = $i + 1;
    }
    return strings.join($out, "\n");
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
    def parts as list of string init ["|"];
    def c as int init 0;
    while ($c < $cols) {
        def v as string init "";
        if ($c < len($cells)) {
            $v = $cells[$c];
        }
        $parts[] = " " + cellText($v) + " |";
        $c = $c + 1;
    }
    return strings.join($parts, "");
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
    def parts as list of string init ["|"];
    def c as int init 0;
    while ($c < $cols) {
        def a as string init "";
        if ($c < len($aligns)) {
            $a = $aligns[$c];
        }
        $parts[] = " " + alignSep($a) + " |";
        $c = $c + 1;
    }
    return strings.join($parts, "");
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
    def lines as list of string init [tableRow($headings, $cols), alignRow($aligns, $cols)];
    for (def row in $rows) {
        $lines[] = tableRow($row, $cols);
    }
    return strings.join($lines, "\n");
}

# A table's source reformatted, and the line after it.
def struct TablePretty {
    lines as list of string,
    next as int
};

# --- prettify: align table source columns (exported) ---------------

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

# srcLen is a cell's re-emitted source width (pipes re-escaped, newline to
# space).
func srcLen(cell as string) {
    return len(cellText($cell));
}

# sourceWidths is the per-column source width: the widest cell, at least 3 (so
# the delimiter has room for a colon and dashes).
func sourceWidths(headings as list of string, rows as list of list of string, cols as int) {
    def widths as list of int init [];
    def i as int init 0;
    while ($i < $cols) {
        def w as int init 3;
        if ($i < len($headings)) {
            $w = math.max($w, srcLen($headings[$i]));
        }
        for (def row in $rows) {
            if ($i < len($row)) {
                $w = math.max($w, srcLen($row[$i]));
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
    def parts as list of string init ["|"];
    def i as int init 0;
    while ($i < $cols) {
        def text as string init "";
        if ($i < len($cells)) {
            $text = cellText($cells[$i]);
        }
        $parts[] = " " + padCell($text, len($text), $widths[$i], alignOf($aligns, $i)) + " |";
        $i = $i + 1;
    }
    return strings.join($parts, "");
}

# prettyDelim renders the padded `| :--- | ---: |` delimiter row.
func prettyDelim(widths as list of int, aligns as list of string, cols as int) {
    def parts as list of string init ["|"];
    def i as int init 0;
    while ($i < $cols) {
        $parts[] = " " + delimCell($widths[$i], alignOf($aligns, $i)) + " |";
        $i = $i + 1;
    }
    return strings.join($parts, "");
}

# prettyTableAt reformats the table at line `i` into aligned source lines.
func prettyTableAt(lines as list of string, i as int) {
    def headings as list of string init splitCells($lines[$i]);
    def aligns as list of string init parseAligns($lines[$i + 1]);
    def rows as list of list of string init [];
    def j as int init $i + 2;
    while ($j < len($lines) and len(strings.trim($lines[$j])) > 0 and
        strings.contains($lines[$j], "|")) {
        $rows[] = splitCells($lines[$j]);
        $j = $j + 1;
    }
    def cols as int init len($headings);
    def widths as list of int init sourceWidths($headings, $rows, $cols);
    def out as list of string init [];
    $out[] = prettyRow($headings, $widths, $aligns, $cols);
    $out[] = prettyDelim($widths, $aligns, $cols);
    for (def row in $rows) {
        $out[] = prettyRow($row, $widths, $aligns, $cols);
    }
    return TablePretty{lines: $out, next: $j};
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
        def next as string init "";
        if ($i + 1 < $n) {
            $next = $lines[$i + 1];
        }
        if (looksLikeTable($lines[$i], $next)) {
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
