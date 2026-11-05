# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0
#
# rst_inline.inc.j - the inline markup scanner of the rst module. Included
# (textual splice) into rst.j; it declares no `use` of its own and relies on the
# includer's imports (`strings`, `regex`, `convert`, the `doctree` alias) and helpers
# (`mk`, `textOf`, `withChildren`, `diagNode`). Private to the rst module.
#
# rST inline markup is recognised by context, not by delimiters alone: a
# start-string must follow whitespace or one of a few punctuation characters and
# be followed by non-whitespace, and an end-string the mirror image. Inline
# markup does not nest. The scanner visits only the candidate characters, found
# in one native regex pass; plain text between them is copied as a slice, and
# checked once for standalone URLs and email addresses.
#
# References come out as `rst_ref` placeholders (see rst_refs.inc.j), resolved
# once the whole document has been read.

# What may come right before an inline markup start-string (besides whitespace
# and the start of the text), and right after an end-string (besides whitespace
# and the end of the text). ASCII plus the common Unicode quotes and dashes.
def const START_PRECEDERS as string init '-:/"([{<' +
    "'‘“«‹‐‑‒–—";
def const END_FOLLOWERS as string init '-:/".,;!?)]}>\' +
    "'’”»›‐‑‒–—";
# A start-string right after an opener must not be followed by its closer
# (`"*"` is a quoted asterisk, not emphasis).
def const OPENERS as string init '"([{<' + "'‘“«‹";
def const CLOSERS as string init '")]}>' + "'’”»›";

# The characters that can begin inline markup, a reference or an escape.
def const MARKUP_CHARS_RE as string init '[*`\\_\[|:]';
# A simple reference name at the end of a run of text (`name` in `name_`):
# words joined by single `-`, `.`, `_`, `+` or `:`.
def const REFNAME_TAIL_RE as string init '(?:[\pL\pN]+[-._+:])*[\pL\pN]+$';
def const ROLE_PREFIX_RE as string init '^:([A-Za-z0-9]+(?:[-_+:.][A-Za-z0-9]+)*):`';
def const ROLE_SUFFIX_RE as string init '^:([A-Za-z0-9]+(?:[-_+:.][A-Za-z0-9]+)*):';
def const NOTE_LABEL_RE as string init '^(?:[0-9]+|#|\*|#?[\pL\pN]+(?:[-._+:][\pL\pN]+)*)$';
# Standalone hyperlinks: a URI with a known scheme, or an email address. The
# last character may not be trailing punctuation.
def const LINK_RE as string init '(?:(?:https?|ftps?|sftp|file|news|nntp|telnet|irc|git|ssh)://' +
    '[^\s<>"\x60]*[^\s<>"\x60.,;:!?)\]}\x27]|mailto:[^\s<>"@]+@[^\s<>"]*[^\s<>".,;:!?)\]}\x27]' +
    '|[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+)';

# One recognised (or rejected) piece of inline markup at a position. A rejected
# start-string has ok false and `next` past it, so it is kept as text. `from`
# is where the markup really starts when that is before the candidate character
# (a `name_` reference is found at its `_`). `severity` / `warn` carry a problem.
def struct InlineScan {
    ok as bool,
    node as doctree.Node,
    from as int,
    next as int,
    severity as string,
    warn as string
};

func noScan(next as int) {
    return InlineScan{ok: false, node: mk("text"), from: -1, next: $next, severity: "", warn: ""};
}

func gotScan(node as doctree.Node, next as int) {
    return InlineScan{ok: true, node: $node, from: -1, next: $next, severity: "", warn: ""};
}

func badScan(next as int, severity as string, warn as string) {
    return InlineScan{
        ok: false,
        node: mk("text"),
        from: -1,
        next: $next,
        severity: $severity,
        warn: $warn
    };
}

# refNode builds an rst_ref placeholder. `key` is unique in the document (the
# source line and the column in the joined paragraph), which lets the reference
# pass pair anonymous references and auto-numbered footnotes by position.
func refNode(
    lang as string,
    name as string,
    url as string,
    kids as list of doctree.Node,
    raw as string,
    key as string) {
    def n as doctree.Node init withChildren("rst_ref", $kids);
    $n.lang = $lang;
    $n.text = $name;
    $n.url = $url;
    $n.attrs = {"key": $key, "raw": $raw, "suffix": ""};
    return $n;
}

# inlineNodes parses a run of text (a paragraph's lines joined with spaces) into
# inline nodes. `line` is the source line the text starts on, for diagnostics.
func inlineNodes(text as string, line as int) {
    def marks as list of regex.Match init regex.findAll(MARKUP_CHARS_RE, $text);
    if (len($marks) == 0) {
        return plainNodes($text);
    }
    def cs as list of string init strings.chars($text);
    def n as int init len($cs);
    def out as list of doctree.Node init [];
    def buf as list of string init [];
    def i as int init 0;
    for (def m in $marks) {
        def p as int init $m.start;
        if ($p < $i) {
            # Already consumed, by an escape or by markup that started earlier.
            continue;
        }
        if ($cs[$p] == "_" and not ($p + 1 < $n and $cs[$p + 1] == "`")) {
            # A `name_` reference is only seen at its `_`: the name is the tail
            # of the plain text run before it.
            def nr as InlineScan init scanNameRef($cs, $p, $i, $line);
            if ($nr.ok) {
                $buf[] = strings.join($cs[$i..$nr.from], "");
                $out = flushText($out, $buf);
                $buf = [];
                $out[] = $nr.node;
                $i = $nr.next;
            }
            continue;
        }
        if ($p > $i) {
            $buf[] = strings.join($cs[$i..$p], "");
        }
        if ($cs[$p] == "\\") {
            # An escaped character is literal; an escaped space disappears.
            if ($p + 1 < $n and $cs[$p + 1] != " ") {
                $buf[] = $cs[$p + 1];
            }
            $i = $p + 2;
            continue;
        }
        def sc as InlineScan init scanAt($cs, $p, $line);
        if ($sc.warn != "") {
            $out = flushText($out, $buf);
            $buf = [];
            $out[] = diagNode($sc.severity, $line, $sc.warn);
        }
        if (not $sc.ok) {
            $buf[] = strings.join($cs[$p..$sc.next], "");
            $i = $sc.next;
            continue;
        }
        $out = flushText($out, $buf);
        $buf = [];
        $out[] = $sc.node;
        $i = $sc.next;
    }
    if ($i < $n) {
        $buf[] = strings.join($cs[$i..$n], "");
    }
    $out = flushText($out, $buf);
    return $out;
}

# flushText appends the pending plain text (with any standalone links in it) to
# the output. Call it as `$out = flushText($out, $buf);`.
func flushText(out as list of doctree.Node, buf as list of string) { # lint-disable: L205
    if (len($buf) == 0) {
        return $out;
    }
    for (def x in plainNodes(strings.join($buf, ""))) {
        $out[] = $x;
    }
    return $out;
}

# plainNodes turns plain text into text nodes, with standalone URIs and email
# addresses as links. A link must start where inline markup could.
func plainNodes(text as string) {
    if ($text == "") {
        return [];
    }
    if (not (strings.contains($text, ":") or strings.contains($text, "@"))) {
        return [textOf($text)];
    }
    def out as list of doctree.Node init [];
    def at as int init 0;
    for (def m in regex.findAll(LINK_RE, $text)) {
        if ($m.start > 0 and not linkMayStart($text[$m.start - 1..$m.start])) {
            continue;
        }
        if ($m.start > $at) {
            $out[] = textOf($text[$at..$m.start]);
        }
        def url as string init $m.text;
        if (not strings.contains($url, ":")) {
            $url = "mailto:" + $url;
        }
        $out[] = linkNode($url, [textOf($m.text)]);
        $at = $m.end;
    }
    if ($at < len($text)) {
        $out[] = textOf($text[$at..]);
    }
    return $out;
}

func linkMayStart(before as string) {
    return $before == " " or strings.contains(START_PRECEDERS, $before);
}

# scanAt dispatches on the candidate character at p.
func scanAt(cs as list of string, p as int, line as int) {
    match ($cs[$p]) {
        when "*", "`" { return scanMarkup($cs, $p, $line); }
        when "_" { return scanInlineTarget($cs, $p); }
        when "[" { return scanNoteRef($cs, $p, $line); }
        when "|" { return scanSubstRef($cs, $p, $line); }
        when ":" { return scanRolePrefix($cs, $p, $line); }
    }
    return noScan($p + 1);
}

func keyOf(line as int, p as int) {
    return "{$line}:{$p}";
}

# startString is the inline start-string at i: `**`, `*`, ``` `` ``` or `` ` ``.
func startString(cs as list of string, i as int) {
    def c as string init $cs[$i];
    if ($i + 1 < len($cs) and $cs[$i + 1] == $c) {
        return $c + $c;
    }
    return $c;
}

# startAllowed applies the start-string context rules to a start-string of
# length sl at i: preceded by the start of the text, whitespace or a
# START_PRECEDERS character; followed by non-whitespace; and not a quoted
# start-string (an opener before, its closer after).
func startAllowed(cs as list of string, i as int, sl as int) {
    def n as int init len($cs);
    if ($i + $sl >= $n) {
        return false;
    }
    def after as string init $cs[$i + $sl];
    if ($after == " ") {
        return false;
    }
    if ($i == 0) {
        return true;
    }
    def before as string init $cs[$i - 1];
    if ($before == " ") {
        return true;
    }
    if (not strings.contains(START_PRECEDERS, $before)) {
        return false;
    }
    def o as int init strings.indexOf(OPENERS, $before);
    return $o < 0 or strings.indexOf(CLOSERS, $after) != $o;
}

# endAllowed applies the end-string context rules at an end-string that starts
# at j and is followed by position f: not preceded by whitespace, and followed by
# the end of the text, whitespace or an END_FOLLOWERS character.
func endAllowed(cs as list of string, j as int, f as int) {
    if ($cs[$j - 1] == " ") {
        return false;
    }
    return followAllowed($cs, $f);
}

func followAllowed(cs as list of string, f as int) {
    if ($f >= len($cs)) {
        return true;
    }
    def after as string init $cs[$f];
    return $after == " " or strings.contains(END_FOLLOWERS, $after);
}

# scanMarkup reads the emphasis, strong, literal or interpreted text whose
# start-string is at i.
func scanMarkup(cs as list of string, i as int, line as int) {
    def n as int init len($cs);
    def start as string init startString($cs, $i);
    def sl as int init len($start);
    if (not startAllowed($cs, $i, $sl)) {
        return noScan($i + $sl);
    }
    if ($start == "`") {
        return scanInterpreted($cs, $i, $i + 1, "", $line);
    }
    def literal as bool init $start == "``";
    def j as int init $i + $sl + 1;
    while ($j + $sl <= $n) {
        def c as string init $cs[$j];
        if ($c == "\\" and not $literal) {
            $j = $j + 2;
            continue;
        }
        if ($c == $start[0..1] and strings.join($cs[$j..$j + $sl], "") == $start and
            endAllowed($cs, $j, $j + $sl)) {
            return gotScan(simpleMarkup($start, $cs[$i + $sl..$j]), $j + $sl);
        }
        $j = $j + 1;
    }
    return badScan(
        $i + $sl,
        "warning",
        "Inline " + markupName($start) + " start-string without end-string.");
}

func markupName(start as string) {
    match ($start) {
        when "**" { return "strong"; }
        when "*" { return "emphasis"; }
        when "``" { return "literal"; }
    }
    return "interpreted text or phrase reference";
}

func simpleMarkup(start as string, raw as list of string) {
    match ($start) {
        when "**" { return withChildren("strong", [textOf(unescapeChars($raw))]); }
        when "*" { return withChildren("emphasis", [textOf(unescapeChars($raw))]); }
    }
    def code as doctree.Node init mk("codespan");
    $code.text = strings.join($raw, "");
    return $code;
}

# scanInterpreted reads interpreted text or a phrase reference whose content
# starts at a (just past the opening backtick), for markup that started at i
# (the backtick itself, or the `:role:` before it). The closing backtick may be
# followed by a reference suffix (`_` / `__`) or, with no prefix role, by a
# suffix role (`:role:`).
func scanInterpreted(cs as list of string, i as int, a as int, role as string, line as int) {
    def n as int init len($cs);
    def j as int init $a + 1;
    while ($j < $n) {
        if ($cs[$j] == "\\") {
            $j = $j + 2;
            continue;
        }
        if ($cs[$j] == "`") {
            def f as int init refSuffixEnd($cs, $j + 1);
            def g as int init $f;
            if ($f == $j + 1 and $role == "") {
                $g = roleSuffixEnd($cs, $f);
            }
            if (endAllowed($cs, $j, $g)) {
                return interpreted($cs, $i, $a, $j, $f, $g, $role, $line);
            }
        }
        $j = $j + 1;
    }
    return badScan(
        $a,
        "warning",
        "Inline interpreted text or phrase reference start-string without end-string.");
}

# refSuffixEnd steps over a `_` or `__` reference suffix after a closing backtick.
func refSuffixEnd(cs as list of string, f as int) {
    def k as int init $f;
    if ($k < len($cs) and $cs[$k] == "_") {
        $k = $k + 1;
        if ($k < len($cs) and $cs[$k] == "_") {
            $k = $k + 1;
        }
    }
    return $k;
}

# window joins up to `width` characters from position p, for a regex that
# anchors there.
func window(cs as list of string, p as int, width as int) {
    def e as int init $p + $width;
    if ($e > len($cs)) {
        $e = len($cs);
    }
    return strings.join($cs[$p..$e], "");
}

# roleSuffixEnd steps over a suffix role (`:role:`) after a closing backtick at
# f - 1, or returns f when there is none.
func roleSuffixEnd(cs as list of string, f as int) {
    if ($f >= len($cs) or $cs[$f] != ":") {
        return $f;
    }
    def m as regex.Match init regex.find(ROLE_SUFFIX_RE, window($cs, $f, 70));
    if ($m.start != 0) {
        return $f;
    }
    return $f + len($m.text);
}

# interpreted builds the node for interpreted text whose content is cs[a..j],
# whose closing backtick is at j, whose reference suffix ends at f and whose
# suffix role (if any) ends at g.
func interpreted(
    cs as list of string,
    i as int,
    a as int,
    j as int,
    f as int,
    g as int,
    role as string,
    line as int) {
    def raw as list of string init $cs[$a..$j];
    def suffix as string init strings.join($cs[$j + 1..$f], "");
    if ($suffix == "") {
        def r as string init $role;
        if ($g > $f) {
            # A suffix role, `text`:role:.
            $r = strings.join($cs[$f + 1..$g - 1], "");
        }
        def rn as InlineScan init roleNode($r, $raw);
        $rn.next = $g;
        return $rn;
    }
    if ($role != "") {
        return badScan(
            $g,
            "error",
            "Mismatch: both interpreted text role prefix and reference suffix.");
    }
    def raws as string init strings.join($cs[$i..$f], "");
    return gotScan(phraseRef(strings.join($raw, ""), $suffix, $raws, $line, keyOf($line, $i)), $f);
}

# phraseRef builds a phrase reference: `text`_, `text <uri>`_ (an embedded URI),
# or `text <name_>`_ (an embedded alias).
func phraseRef(content as string, suffix as string, raw as string, line as int, key as string) {
    def text as string init $content;
    def target as string init "";
    if (strings.endsWith($content, ">")) {
        def lt as int init strings.lastIndexOf($content, "<");
        if ($lt == 0 or ($lt > 0 and $content[$lt - 1..$lt] == " ")) {
            $text = strings.trim($content[..$lt]);
            $target = regex.replace('\s+', $content[$lt + 1..len($content) - 1], "");
        }
    }
    def label as string init unescapeText($text);
    def lang as string init refLang("named", $suffix);
    def name as string init normName($label);
    def uri as string init "";
    if ($target != "" and strings.endsWith($target, "_") and not strings.endsWith($target, "\\_")) {
        $lang = "alias";
        $name = normName(stripTicks($target[..len($target) - 1]));
        if ($label == "") {
            $label = $name;
        }
    } elseif ($target != "") {
        $lang = refLang("embedded", $suffix);
        $uri = unescapeText($target);
        if ($label == "") {
            $label = $uri;
        }
        $name = normName($label);
    }
    def r as doctree.Node init refNode($lang, $name, $uri, [textOf($label)], $raw, $key);
    $r.level = $line;
    return $r;
}

# refLang is the reference kind for a base kind and its suffix: `__` makes a
# named reference anonymous and keeps an embedded URI from defining a target.
func refLang(base as string, suffix as string) {
    if ($suffix != "__") {
        return $base;
    }
    if ($base == "named") {
        return "anon";
    }
    return "embedded_anon";
}

# roleNode applies an interpreted text role to its content.
func roleNode(role as string, raw as list of string) {
    def content as string init unescapeChars($raw);
    match (strings.lower($role)) {
        when "", "title-reference", "title", "t" {
            return gotScan(withChildren("cite", [textOf($content)]), 0);
        }
        when "emphasis" { return gotScan(withChildren("emphasis", [textOf($content)]), 0); }
        when "strong" { return gotScan(withChildren("strong", [textOf($content)]), 0); }
        when "literal", "code" { return gotScan(codeOf($content, ""), 0); }
        when "math" { return gotScan(codeOf($content, "math"), 0); }
        when "subscript", "sub" { return gotScan(withChildren("subscript", [textOf($content)]), 0); }
        when "superscript", "sup" {
            return gotScan(withChildren("superscript", [textOf($content)]), 0);
        }
        when "abbreviation", "ab", "abbr", "acronym", "ac" { return gotScan(abbrOf($content), 0); }
        when "pep-reference", "pep" { return numberedLink("PEP", $content, 9999); }
        when "rfc-reference", "rfc" { return numberedLink("RFC", $content, 99999); }
    }
    return badScan(0, "error", "Unknown interpreted text role \"" + $role + "\".");
}

func codeOf(text as string, cls as string) {
    def code as doctree.Node init mk("codespan");
    $code.text = $text;
    if ($cls != "") {
        $code.attrs = {"class": $cls};
    }
    return $code;
}

# abbrOf builds an abbreviation; a trailing "(expansion)" becomes its title, the
# way Sphinx's :abbr: role reads it.
func abbrOf(content as string) {
    def m as regex.Match init regex.find('^(.*\S)\s*\((.+)\)$', $content);
    if ($m.start == 0) {
        def a as doctree.Node init withChildren("abbr", [textOf($m.groups[0])]);
        $a.title = $m.groups[1];
        return $a;
    }
    return withChildren("abbr", [textOf($content)]);
}

# numberedLink is the :pep: / :rfc: role: a link to the numbered document.
func numberedLink(kind as string, content as string, max as int) {
    if (not regex.matches('^[0-9]{1,5}$', $content) or convert.toInt($content) > $max) {
        return badScan(
            0,
            "error",
            $kind + " number must be a number from 0 to {$max}; \"" + $content + "\" is invalid.");
    }
    def num as int init convert.toInt($content);
    def url as string init "https://datatracker.ietf.org/doc/html/rfc{$num}";
    if ($kind == "PEP") {
        def digits as string init "{$num}";
        def padded as string init $digits;
        if (len($digits) < 4) {
            $padded = strings.repeat("0", 4 - len($digits)) + $digits;
        }
        $url = "https://peps.python.org/pep-" + $padded + "/";
    }
    return gotScan(linkNode($url, [textOf($kind + " {$num}")]), 0);
}

# scanNameRef reads a `name_` / `name__` reference whose underscore is at p;
# `i` is where the current plain run starts (the name cannot reach before it).
func scanNameRef(cs as list of string, p as int, i as int, line as int) {
    def n as int init len($cs);
    def f as int init $p + 1;
    if ($f < $n and $cs[$f] == "_") {
        $f = $f + 1;
    }
    if (($f < $n and $cs[$f] == "_") or not followAllowed($cs, $f) or $p == $i) {
        return noScan($p + 1);
    }
    def m as regex.Match init regex.find(REFNAME_TAIL_RE, strings.join($cs[$i..$p], ""));
    if ($m.start < 0) {
        return noScan($p + 1);
    }
    def q as int init $i + $m.start;
    if ($q > 0 and not linkMayStart($cs[$q - 1])) {
        return noScan($p + 1);
    }
    def suffix as string init strings.join($cs[$p..$f], "");
    def lang as string init refLang("named", $suffix);
    def r as doctree.Node init refNode(
        $lang,
        normName($m.text),
        "",
        [textOf($m.text)],
        $m.text + $suffix,
        keyOf($line, $q));
    $r.level = $line;
    def sc as InlineScan init gotScan($r, $f);
    $sc.from = $q;
    return $sc;
}

# scanInlineTarget reads an inline target, _`text`, at p.
func scanInlineTarget(cs as list of string, p as int) {
    if (not startAllowed($cs, $p, 2)) {
        return noScan($p + 1);
    }
    def j as int init $p + 3;
    while ($j < len($cs)) {
        if ($cs[$j] == "\\") {
            $j = $j + 2;
            continue;
        }
        if ($cs[$j] == "`" and endAllowed($cs, $j, $j + 1)) {
            def t as doctree.Node init mk("rst_itarget");
            $t.text = unescapeChars($cs[$p + 2..$j]);
            return gotScan($t, $j + 1);
        }
        $j = $j + 1;
    }
    return badScan($p + 1, "warning", "Inline target start-string without end-string.");
}

# scanNoteRef reads a footnote or citation reference, `[label]_`, at p.
func scanNoteRef(cs as list of string, p as int, line as int) {
    def n as int init len($cs);
    if (not startAllowed($cs, $p, 1)) {
        return noScan($p + 1);
    }
    def k as int init $p + 1;
    while ($k < $n and $k - $p <= 100 and $cs[$k] != "]" and $cs[$k] != " ") {
        $k = $k + 1;
    }
    if ($k + 1 >= $n or $cs[$k] != "]" or $cs[$k + 1] != "_" or not followAllowed($cs, $k + 2)) {
        return noScan($p + 1);
    }
    def label as string init strings.join($cs[$p + 1..$k], "");
    if (not regex.matches(NOTE_LABEL_RE, $label)) {
        return noScan($p + 1);
    }
    def lang as string init "citation";
    if (regex.matches('^([0-9]+|#.*|\*)$', $label)) {
        $lang = "footnote";
    }
    def r as doctree.Node init refNode(
        $lang,
        $label,
        "",
        [],
        "[" + $label + "]_",
        keyOf($line, $p));
    $r.level = $line;
    return gotScan($r, $k + 2);
}

# scanSubstRef reads a substitution reference, `|name|` (optionally `|name|_` or
# `|name|__`), at p.
func scanSubstRef(cs as list of string, p as int, line as int) {
    def n as int init len($cs);
    if (not startAllowed($cs, $p, 1)) {
        return noScan($p + 1);
    }
    def k as int init $p + 2;
    while ($k < $n and $cs[$k] != "|") {
        $k = $k + 1;
    }
    if ($k >= $n or $cs[$k - 1] == " ") {
        return noScan($p + 1);
    }
    def f as int init refSuffixEnd($cs, $k + 1);
    if (not followAllowed($cs, $f)) {
        return noScan($p + 1);
    }
    def name as string init regex.replace('\s+', unescapeChars($cs[$p + 1..$k]), " ");
    def raw as string init strings.join($cs[$p..$f], "");
    def r as doctree.Node init refNode("subst", $name, "", [], $raw, keyOf($line, $p));
    $r.level = $line;
    $r.attrs["suffix"] = strings.join($cs[$k + 1..$f], "");
    return gotScan($r, $f);
}

# scanRolePrefix reads `:role:` followed by interpreted text, at p.
func scanRolePrefix(cs as list of string, p as int, line as int) {
    if ($p + 1 >= len($cs) or not regex.matches('^[A-Za-z0-9]$', $cs[$p + 1])) {
        return noScan($p + 1);
    }
    def m as regex.Match init regex.find(ROLE_PREFIX_RE, window($cs, $p, 70));
    if ($m.start != 0 or not startAllowed($cs, $p, len($m.text))) {
        return noScan($p + 1);
    }
    return scanInterpreted($cs, $p, $p + len($m.text), $m.groups[0], $line);
}

# unescapeChars joins markup content with its backslash escapes applied: an
# escaped character stands for itself, an escaped space disappears.
func unescapeChars(raw as list of string) {
    def out as list of string init [];
    def k as int init 0;
    while ($k < len($raw)) {
        if ($raw[$k] == "\\" and $k + 1 < len($raw)) {
            if ($raw[$k + 1] != " ") {
                $out[] = $raw[$k + 1];
            }
            $k = $k + 2;
            continue;
        }
        $out[] = $raw[$k];
        $k = $k + 1;
    }
    return strings.join($out, "");
}

# stripTicks drops the backticks around a quoted reference name.
func stripTicks(s as string) {
    return regex.replace('^`+|`+$', $s, "");
}

func unescapeText(s as string) {
    if (not strings.contains($s, "\\")) {
        return $s;
    }
    return unescapeChars(strings.chars($s));
}
