# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

# A MyST reader: its line pre-pass and directive dispatcher legitimately run
# past the L201 statement-count limit. Every other lint check stays active.
# lint-disable-file: L201

/**
 * A MyST (Markedly Structured Text) reader. MyST is CommonMark plus the
 * directives and roles of reStructuredText, as Sphinx and Jupyter Book write it.
 * `parse` turns MyST source into a `doctree` document tree (`doctree.Node`), the
 * tree `markdown.parse` and `rst.parse` build, so the same renderers serve it:
 * `toHtml` / `toXhtml` / `toAnsi` here, and `doctree.render` /
 * `doctree.renderPdf` / `doctree.findAll` on the tree.
 *
 * Everything `markdown` reads (CommonMark, GFM tables, task lists,
 * strikethrough, footnotes, ...) plus: directives in backtick fences
 * (```` ```{note} ````) or colon fences (`:::{note}`), with `:option:` lines or a
 * `---` options block; roles (``{sub}`2` ``); targets (`(label)=`) and
 * references to them (``{ref}`label` ``, ``{ref}`text <label>` ``, `{numref}`,
 * `{eq}`, `{doc}`); `%` comments; `+++` block breaks; front matter; and dollar
 * math (`$x$`, `$$ ... $$`). Directives: the admonitions, `code` /
 * `code-block`, `math`, `image`, `figure`, `topic`, `sidebar`, `rubric`,
 * `epigraph`, `container`, `raw`, `table`, `list-table`, `csv-table`,
 * `contents` and `eval-rst` (rST content, read by the `rst` module).
 *
 * Parsing is lenient: an unknown directive is kept as a literal block, an
 * unknown role as text, and `check` reports both as `Diagnostic`s;
 * `Options{strict: true}` turns the first problem into a catchable `"myst"`
 * error. There is no file I/O: `include` and `:file:` options are refused.
 * @module myst
 * @example
 * import "myst.j" as myst;
 * io.printf("%s\n", myst.toHtml('```{note}\nMind the *gap*.\n```\n\nH{sub}`2`O'));
 */
import "./doctree.j" as doctree;
import "./markdown.j" as markdown;
import "./rst.j" as rst;
import "./csv.j" as csv;
use strings;
use regex;
use convert;
use lists;
use maps;

/**
 * Options for `parseWith` and `toHtmlWith`. The zero value is the default.
 * @field strict {bool} throw the first warning or error as a `"myst"` error
 *   instead of recovering from it
 * @field allowRawHtml {bool} pass raw HTML through when rendering (trusted
 *   input only); escaped by default
 * @field noDollarMath {bool} read `$...$` and `$$...$$` as plain text rather
 *   than math
 * @field noColonFence {bool} read `:::` lines as plain text rather than
 *   directive fences
 */
export def struct Options {
    strict as bool,
    allowRawHtml as bool,
    noDollarMath as bool,
    noColonFence as bool
};

/**
 * One problem found in a document.
 * @field severity {string} `"warning"` (recovered from, likely unintended) or
 *   `"error"` (markup that could not be read as written)
 * @field line {int} the 1-based source line it was found on; 0 when not known
 * @field message {string} what went wrong
 */
export def struct Diagnostic {
    severity as string,
    line as int,
    message as string
};

# Directive nesting deeper than this is a "myst" error.
def const MAX_DEPTH as int init 50;
# Each directive opener carries its source line in its info string, between
# two of these private-use characters, until the lowering pass reads it.
def const LINE_MARK as string init "";
def const ROLE_RE as string init '\{([A-Za-z][A-Za-z0-9_:+.-]*)\}$';
def const TARGET_RE as string init '^\(([^()\s][^()]*)\)=(?:\s+|$)';
def const FENCE_RE as string init '^(`{3,}|~{3,})(.*)$';
def const COLON_RE as string init '^(:{3,})\{([^}\s]+)\}(.*)$';

# The parsed document and the problems found on the way.
def struct Result {
    doc as doctree.Node,
    diags as list of Diagnostic
};

include "./myst_directives.inc.j";

func mystFail(msg as string) {
    throw Error{kind: "myst", message: $msg};
}

func mk(kind as string) {
    def n as doctree.Node;
    $n.kind = $kind;
    return $n;
}

func textOf(s as string) {
    def n as doctree.Node init mk("text");
    $n.text = $s;
    return $n;
}

func withChildren(kind as string, kids as list of doctree.Node) {
    def n as doctree.Node init mk($kind);
    $n.children = $kids;
    return $n;
}

# diagNode is a problem kept in the tree where it was found and lifted out by
# `run`: the severity rides in `lang`, the line in `level`.
func diagNode(severity as string, line as int, message as string) {
    def n as doctree.Node init mk("myst_diag");
    $n.lang = $severity;
    $n.level = $line;
    $n.text = $message;
    return $n;
}

# --- the line pre-pass -----------------------------------------------

# frontMatterEnd is the index of the line closing a front-matter block that
# opens the document (`---` ... `---` or `...`), or -1.
func frontMatterEnd(lines as list of string) {
    if (len($lines) == 0 or strings.trimRight($lines[0]) != "---") {
        return -1;
    }
    for (def k in 1..len($lines)) {
        def t as string init strings.trimRight($lines[$k]);
        if ($t == "---" or $t == "...") {
            return $k;
        }
    }
    return -1;
}

func longestTickRun(lines as list of string) {
    def most as int init 0;
    for (def l in $lines) {
        def m as regex.Match init regex.find('^\s*(`+)', $l);
        if ($m.start == 0 and len($m.groups[0]) > $most) {
            $most = len($m.groups[0]);
        }
    }
    return $most;
}

# tickWrap writes text as a code span, with a backtick fence longer than any
# run inside it.
func tickWrap(s as string) {
    def most as int init 0;
    for (def m in regex.findAll('`+', $s)) {
        if (len($m.text) > $most) {
            $most = len($m.text);
        }
    }
    def fence as string init strings.repeat("`", $most + 1);
    if (strings.startsWith($s, "`") or strings.endsWith($s, "`")) {
        return $fence + " " + $s + " " + $fence;
    }
    return $fence + $s + $fence;
}

# inlineMath rewrites `$x$` (and `$$x$$`) on one line as the `{math}` role,
# leaving code spans and escaped dollars alone. An opening `$` may not be
# followed by a space, a closing one not preceded by a space nor followed by a
# digit, so prices (`$5 and $10`) stay text.
func inlineMath(line as string) {
    if (not strings.contains($line, "$")) {
        return $line;
    }
    def cs as list of string init strings.chars($line);
    def n as int init len($cs);
    def out as list of string init [];
    def i as int init 0;
    while ($i < $n) {
        def c as string init $cs[$i];
        if ($c == "\\" and $i + 1 < $n) {
            $out[] = $c;
            $out[] = $cs[$i + 1];
            $i = $i + 2;
            continue;
        }
        if ($c == "`") {
            def e as int init $i;
            while ($e < $n and $cs[$e] == "`") {
                $e = $e + 1;
            }
            def tick as string init strings.join($cs[$i..$e], "");
            def close as int init strings.indexOf($line, $tick, $e);
            if ($close < 0) {
                $close = $e - len($tick);
            }
            def upto as int init $close + len($tick);
            for (def q in $i..$upto) {
                $out[] = $cs[$q];
            }
            $i = $upto;
            continue;
        }
        if ($c != "$") {
            $out[] = $c;
            $i = $i + 1;
            continue;
        }
        def w as int init 1;
        if ($i + 1 < $n and $cs[$i + 1] == "$") {
            $w = 2;
        }
        def body as int init $i + $w;
        def found as int init -1;
        if ($body < $n and $cs[$body] != " " and $cs[$body] != "$") {
            def j as int init $body;
            while ($j < $n) {
                if ($cs[$j] == "\\") {
                    $j = $j + 2;
                    continue;
                }
                if ($cs[$j] == "$") {
                    def ok as bool init $cs[$j - 1] != " ";
                    if ($w == 2) {
                        $ok = $ok and $j + 1 < $n and $cs[$j + 1] == "$";
                    } else {
                        $ok = $ok and not ($j + 1 < $n and regex.matches('^[0-9]$', $cs[$j + 1]));
                    }
                    if ($ok) {
                        $found = $j;
                    }
                    break;
                }
                $j = $j + 1;
            }
        }
        if ($found < 0) {
            $out[] = $c;
            $i = $i + 1;
            continue;
        }
        $out[] = '{math}' + tickWrap(strings.join($cs[$body..$found], ""));
        $i = $found + $w;
    }
    return strings.join($out, "");
}

# preprocess rewrites the MyST syntax markdown does not read into syntax it
# does: colon fences into backtick fences, `$$` blocks into `{math}` fences,
# `$x$` into the `{math}` role; it drops `%` comments and front matter, makes a
# `+++` a thematic break, and stamps each directive opener with its source line
# (`base` is the line number of the first line).
func preprocess(src as string, base as int, opts as Options) {
    def lines as list of string init strings.split(strings.replace($src, "\r\n", "\n"), "\n");
    def out as list of string init [];
    def n as int init len($lines);
    def i as int init 0;
    if ($base == 1) {
        def fm as int init frontMatterEnd($lines);
        if ($fm > 0) {
            for (def k in 0..$fm + 1) {
                $out[] = "";
            }
            $i = $fm + 1;
        }
    }
    def fenceCh as string init "";
    def fenceLen as int init 0;
    while ($i < $n) {
        def line as string init $lines[$i];
        def t as string init strings.trimLeft($line);
        def ind as string init $line[..len($line) - len($t)];
        if ($fenceCh != "") {
            $out[] = $line;
            if (regex.matches('^' + regex.escape($fenceCh) + '{' + "{$fenceLen}" + ',}\s*$', $t)) {
                $fenceCh = "";
            }
            $i = $i + 1;
            continue;
        }
        def fm2 as regex.Match init regex.find(FENCE_RE, $t);
        if ($fm2.start == 0) {
            $fenceCh = $fm2.groups[0][..1];
            $fenceLen = len($fm2.groups[0]);
            if ($fenceCh == "`" and strings.startsWith($fm2.groups[1], '{')) {
                $out[] = $line + " " + LINE_MARK + "{$base + $i}" + LINE_MARK;
            } else {
                $out[] = $line;
            }
            $i = $i + 1;
            continue;
        }
        def cm as regex.Match init regex.find(COLON_RE, $t);
        if ($cm.start == 0 and not $opts.noColonFence) {
            def want as int init len($cm.groups[0]);
            def j as int init $i + 1;
            while ($j < $n and
                not regex.matches('^:{' + "{$want}" + ',}\s*$', strings.trim($lines[$j]))) {
                $j = $j + 1;
            }
            def body as list of string init $lines[$i + 1..$j];
            def ticks as string init strings.repeat("`", math2Max(3, longestTickRun($body) + 1));
            $out[] = $ind + $ticks + '{' + $cm.groups[1] + '}' + $cm.groups[2] + " " + LINE_MARK +
                "{$base + $i}" + LINE_MARK;
            for (def b in $body) {
                $out[] = $b;
            }
            $out[] = $ind + $ticks;
            $i = $j + 1;
            continue;
        }
        if (len($ind) <= 3 and
            ($t == "%" or strings.startsWith($t, "% ") or strings.startsWith($t, "%"))) {
            $i = $i + 1;
            continue;
        }
        if (len($ind) <= 3 and regex.matches('^\+\+\+(?:\s.*)?$', $t)) {
            $out[] = $ind + "***";
            $i = $i + 1;
            continue;
        }
        if (not $opts.noDollarMath and len($ind) <= 3 and strings.startsWith($t, "$$")) {
            def single as regex.Match init regex.find('^\$\$(.+?)\$\$\s*(?:\(([^()]+)\))?\s*$', $t);
            def mathLines as list of string init [];
            def label as string init "";
            def next as int init $i + 1;
            if ($single.start == 0) {
                $mathLines = [$single.groups[0]];
                $label = $single.groups[1];
            } else {
                def first as string init strings.trim($t[2..]);
                if ($first != "") {
                    $mathLines[] = $first;
                }
                def j as int init $i + 1;
                def closed as bool init false;
                while ($j < $n) {
                    def tj as string init strings.trim($lines[$j]);
                    def endm as regex.Match init regex.find('^(.*?)\$\$\s*(?:\(([^()]+)\))?$', $tj);
                    if ($endm.start == 0) {
                        if ($endm.groups[0] != "") {
                            $mathLines[] = $endm.groups[0];
                        }
                        $label = $endm.groups[1];
                        $closed = true;
                        break;
                    }
                    $mathLines[] = $lines[$j];
                    $j = $j + 1;
                }
                if (not $closed) {
                    $out[] = $line;
                    $i = $i + 1;
                    continue;
                }
                $next = $j + 1;
            }
            $out[] = $ind + '```{math} ' + LINE_MARK + "{$base + $i}" + LINE_MARK;
            if ($label != "") {
                $out[] = $ind + ":label: " + $label;
                $out[] = "";
            }
            for (def ml in $mathLines) {
                $out[] = $ind + $ml;
            }
            $out[] = $ind + "```";
            $i = $next;
            continue;
        }
        if (not $opts.noDollarMath) {
            $out[] = inlineMath($line);
        } else {
            $out[] = $line;
        }
        $i = $i + 1;
    }
    return strings.join($out, "\n");
}

func math2Max(a as int, b as int) {
    if ($a > $b) {
        return $a;
    }
    return $b;
}

# --- lowering the markdown tree ---------------------------------------

func isBlockNode(n as doctree.Node) {
    return lists.contains(
        [
            "paragraph",
            "heading",
            "code",
            "list",
            "item",
            "quote",
            "admonition",
            "definition_list",
            "def_term",
            "def_desc",
            "table",
            "row",
            "cell",
            "thematic_break",
            "html_block",
            "page_break",
            "footnote",
            "topic",
            "figure",
            "caption",
            "legend",
            "rubric",
            "container",
            "line_block",
            "myst_contents"
        ],
        $n.kind);
}

# targetOf reads a `(label)=` target off the start of a paragraph: the label,
# and whether anything follows it in the paragraph.
def struct TargetScan {
    label as string,
    rest as bool
};

func targetOf(p as doctree.Node) {
    def none as TargetScan init TargetScan{label: "", rest: false};
    if ($p.kind != "paragraph" or len($p.children) == 0 or $p.children[0].kind != "text") {
        return $none;
    }
    def m as regex.Match init regex.find(TARGET_RE, $p.children[0].text);
    if ($m.start != 0) {
        return $none;
    }
    return TargetScan{
        label: strings.trim($m.groups[0]),
        rest: len($p.children) > 1 or $m.end < len($p.children[0].text)
    };
}

func dropTarget(p as doctree.Node) {
    def out as doctree.Node init $p;
    def m as regex.Match init regex.find(TARGET_RE, $out.children[0].text);
    $out.children[0].text = $out.children[0].text[$m.end..];
    if ($out.children[0].text == "") {
        $out.children = $out.children[1..];
    }
    return $out;
}

# labelNode marks a node as the target of a label: its id (the label itself
# when it is a valid id, else a slug of it) and the label for references.
func labelNode(n as doctree.Node, label as string) {
    def id as string init $label;
    if (not regex.matches('^[A-Za-z][A-Za-z0-9_:.-]*$', $id)) {
        $id = slugOf($label);
    }
    $n.attrs["id"] = $id;
    $n.attrs["myst-label"] = $label;
    return $n;
}

# lowerKids lowers a list of children: blocks one by one (directives built,
# targets attached), runs of inline nodes through the role pass.
func lowerKids(kids as list of doctree.Node, opts as Options, depth as int) {
    def out as list of doctree.Node init [];
    def pending as string init "";
    def inl as list of doctree.Node init [];
    for (def k in $kids) {
        if (not isBlockNode($k)) {
            $inl[] = $k;
            continue;
        }
        if (len($inl) > 0) {
            for (def x in lowerInline($inl)) {
                $out[] = $x;
            }
            $inl = [];
        }
        def node as doctree.Node init $k;
        def ts as TargetScan init targetOf($node);
        if ($ts.label != "") {
            if (not $ts.rest) {
                $pending = $ts.label;
                continue;
            }
            $node = labelNode(dropTarget($node), $ts.label);
        }
        def built as list of doctree.Node init [];
        if ($node.kind == "code" and regex.matches('^\{[^}]+\}$', $node.lang)) {
            $built = directiveNodes($node, $opts, $depth);
        } else {
            $built = [lowerNode($node, $opts, $depth)];
        }
        for (def b in $built) {
            if ($pending != "" and $b.kind != "myst_diag") {
                $out[] = labelNode($b, $pending);
                $pending = "";
            } else {
                $out[] = $b;
            }
        }
    }
    if (len($inl) > 0) {
        for (def x in lowerInline($inl)) {
            $out[] = $x;
        }
    }
    if ($pending != "") {
        $out[] = diagNode(
            "warning",
            0,
            "Target \"(" + $pending + ")=\" has nothing after it to label.");
    }
    return $out;
}

func lowerNode(n as doctree.Node, opts as Options, depth as int) {
    if (len($n.children) == 0) {
        return $n;
    }
    return rebuilt($n, $n.attrs, lowerKids($n.children, $opts, $depth));
}

# rebuilt is `n` with new attributes and children, built from n's fields: a
# copy of `n` would deep-copy the whole subtree only to drop its children.
func rebuilt(n as doctree.Node, attrs as map of string to string, kids as list of doctree.Node) {
    return doctree.Node{
        kind: $n.kind,
        level: $n.level,
        text: $n.text,
        lang: $n.lang,
        ordered: $n.ordered,
        url: $n.url,
        title: $n.title,
        align: $n.align,
        attrs: $attrs,
        children: $kids
    };
}

# lowerInline turns `{role}` + a code span into the role's node, recursing
# into inline containers (emphasis, links, ...).
func lowerInline(ns as list of doctree.Node) {
    def out as list of doctree.Node init [];
    def n as int init len($ns);
    def i as int init 0;
    while ($i < $n) {
        def c as doctree.Node init $ns[$i];
        if ($c.kind == "text" and $i + 1 < $n and $ns[$i + 1].kind == "codespan") {
            def m as regex.Match init regex.find(ROLE_RE, $c.text);
            if ($m.start >= 0) {
                if ($m.start > 0) {
                    $out[] = textOf($c.text[..$m.start]);
                }
                for (def r in roleNodes($m.groups[0], $ns[$i + 1].text)) {
                    $out[] = $r;
                }
                $i = $i + 2;
                continue;
            }
        }
        if (len($c.children) > 0) {
            $c.children = lowerInline($c.children);
        }
        $out[] = $c;
        $i = $i + 1;
    }
    return $out;
}

# explicitTarget splits a role's content `text <target>` into its two parts
# (text "" when there is no explicit title).
def struct RoleTarget {
    text as string,
    target as string
};

func explicitTarget(s as string) {
    def m as regex.Match init regex.find('^(.*\S)\s*<([^<>]+)>$', strings.trim($s));
    if ($m.start == 0) {
        return RoleTarget{text: $m.groups[0], target: strings.trim($m.groups[1])};
    }
    return RoleTarget{text: "", target: strings.trim($s)};
}

func spanOf(kind as string, text as string) {
    return withChildren($kind, [textOf($text)]);
}

func classed(n as doctree.Node, cls as string) {
    $n.attrs["class"] = $cls;
    return $n;
}

# roleNodes is what a role becomes. Cross-references are placeholders
# (`myst_ref`) until every label in the document is known.
func roleNodes(name as string, content as string) {
    match ($name) {
        when "emphasis" { return [spanOf("emphasis", $content)]; }
        when "strong" { return [spanOf("strong", $content)]; }
        when "sub", "subscript" { return [spanOf("subscript", $content)]; }
        when "sup", "superscript" { return [spanOf("superscript", $content)]; }
        when "literal", "code", "samp", "file", "command" {
            def c as doctree.Node init mk("codespan");
            $c.text = $content;
            return [$c];
        }
        when "kbd" {
            def k as doctree.Node init mk("codespan");
            $k.text = $content;
            return [classed($k, "kbd")];
        }
        when "math" {
            def mth as doctree.Node init mk("codespan");
            $mth.text = $content;
            return [classed($mth, "math")];
        }
        when "guilabel", "menuselection" { return [classed(spanOf("span", $content), $name)]; }
        when "abbr" {
            def a as doctree.Node init withChildren("abbr", [textOf($content)]);
            def am as regex.Match init regex.find('^(.*\S)\s*\(([^()]*)\)$', $content);
            if ($am.start == 0) {
                $a.children = [textOf($am.groups[0])];
                $a.title = $am.groups[1];
            }
            return [$a];
        }
        when "pep" {
            def pl as doctree.Node init withChildren(
                "link",
                [textOf("PEP " + strings.trim($content))]);
            $pl.url = "https://peps.python.org/pep-" + padNumber(strings.trim($content), 4) + "/";
            return [$pl];
        }
        when "rfc" {
            def rl as doctree.Node init withChildren(
                "link",
                [textOf("RFC " + strings.trim($content))]);
            $rl.url = "https://datatracker.ietf.org/doc/html/rfc" + strings.trim($content);
            return [$rl];
        }
        when "ref", "numref", "eq", "doc", "download", "term" {
            def rt as RoleTarget init explicitTarget($content);
            def r as doctree.Node init mk("myst_ref");
            $r.lang = $name;
            $r.text = $rt.target;
            $r.title = $rt.text;
            return [$r];
        }
        else {
            return [
                diagNode("warning", 0, "Unknown role \"" + $name + "\"; kept as text."),
                textOf('{' + $name + '}' + tickWrap($content))
            ];
        }
    }
}

func padNumber(s as string, width as int) {
    if (not regex.matches('^[0-9]+$', $s) or len($s) >= $width) {
        return $s;
    }
    return strings.repeat("0", $width - len($s)) + $s;
}

func slugOf(title as string) {
    def s as string init regex.replace('[^a-z0-9]+', strings.lower($title), "-");
    $s = regex.replace('^[^a-z]+', $s, "");
    $s = regex.replace('-+$', $s, "");
    if ($s == "") {
        return "section";
    }
    return $s;
}

# --- parsing a body of MyST --------------------------------------------

# blocks parses MyST source into block nodes, with diagnostics left in place
# as placeholders. `base` is the source line of the first line.
func blocks(src as string, base as int, opts as Options, depth as int) {
    if ($depth > MAX_DEPTH) {
        mystFail("document nests too deep (over {MAX_DEPTH} directive levels)");
    }
    def md as string init preprocess($src, $base, $opts);
    def doc as doctree.Node init markdown.parse($md);
    return lowerKids($doc.children, $opts, $depth);
}

# --- references ------------------------------------------------------

# A label's target: its id and the text a reference shows by default.
def struct Target {
    id as string,
    title as string
};

func collectLabels(n as doctree.Node, into as map of string to Target) { # lint-disable: L205
    def out as map of string to Target init $into;
    for (def c in $n.children) {
        if (maps.has($c.attrs, "myst-label")) {
            def key as string init strings.lower($c.attrs["myst-label"]);
            if (not maps.has($out, $key)) {
                $out[$key] = Target{id: $c.attrs["id"], title: labelTitle($c)};
            }
        }
        if (len($c.children) > 0) {
            $out = collectLabels($c, $out);
        }
    }
    return $out;
}

# labelTitle is the text a reference to a node shows: a heading's text, a
# figure's caption, a table's title, else nothing (the label itself is used).
func labelTitle(n as doctree.Node) {
    if ($n.kind == "heading") {
        return doctree.text($n);
    }
    if ($n.kind == "table" and $n.title != "") {
        return $n.title;
    }
    if ($n.kind == "figure") {
        for (def c in $n.children) {
            if ($c.kind == "caption") {
                return doctree.text($c);
            }
        }
    }
    return "";
}

# resolveRefs replaces each myst_ref with a link to its target (or, for an
# unknown label, its text and a warning), and drops the bookkeeping attrs.
func resolveRefs(n as doctree.Node, labels as map of string to Target) {
    if (len($n.children) == 0) {
        return cleanAttrs($n);
    }
    def kids as list of doctree.Node init [];
    for (def c in $n.children) {
        if ($c.kind == "myst_ref") {
            for (def x in refLink($c, $labels)) {
                $kids[] = $x;
            }
        } else {
            $kids[] = resolveRefs($c, $labels);
        }
    }
    return rebuilt($n, cleanedAttrs($n.attrs), $kids);
}

func cleanAttrs(n as doctree.Node) {
    if (not maps.has($n.attrs, "myst-label")) {
        return $n;
    }
    $n.attrs = cleanedAttrs($n.attrs);
    return $n;
}

# cleanedAttrs is attrs without the `myst-label` bookkeeping key.
func cleanedAttrs(attrs as map of string to string) {
    if (not maps.has($attrs, "myst-label")) {
        return $attrs;
    }
    def out as map of string to string init {};
    for (def k in maps.keys($attrs)) {
        if ($k != "myst-label") {
            $out[$k] = $attrs[$k];
        }
    }
    return $out;
}

func refLink(r as doctree.Node, labels as map of string to Target) {
    if ($r.lang == "doc" or $r.lang == "download") {
        def text as string init $r.title;
        if ($text == "") {
            $text = $r.text;
        }
        def dl as doctree.Node init withChildren("link", [textOf($text)]);
        $dl.url = $r.text;
        return [$dl];
    }
    def key as string init strings.lower($r.text);
    if (not maps.has($labels, $key)) {
        def shown as string init $r.title;
        if ($shown == "") {
            $shown = $r.text;
        }
        return [
            diagNode(
                "warning",
                0,
                "Unknown target \"" + $r.text + "\" in a " + '{' + $r.lang + '}' + " reference."),
            textOf($shown)
        ];
    }
    def t as Target init $labels[$key];
    def text as string init $r.title;
    if ($text == "") {
        $text = $t.title;
    }
    if ($text == "" and $r.lang == "eq") {
        $text = "(" + $r.text + ")";
    }
    if ($text == "") {
        $text = $r.text;
    }
    def l as doctree.Node init withChildren("link", [textOf($text)]);
    $l.url = "#" + $t.id;
    return [$l];
}

# --- diagnostics ----------------------------------------------------

func gatherDiags(n as doctree.Node) {
    def out as list of Diagnostic init [];
    for (def c in $n.children) {
        if ($c.kind == "myst_diag") {
            $out[] = Diagnostic{severity: $c.lang, line: $c.level, message: $c.text};
        } elseif (len($c.children) > 0) {
            for (def d in gatherDiags($c)) {
                $out[] = $d;
            }
        }
    }
    return $out;
}

func stripDiags(n as doctree.Node) {
    if (len($n.children) == 0) {
        return $n;
    }
    def kids as list of doctree.Node init [];
    for (def c in $n.children) {
        if ($c.kind != "myst_diag") {
            $kids[] = stripDiags($c);
        }
    }
    return rebuilt($n, $n.attrs, $kids);
}

# lineOfRef finds the first source line naming a role's target, for a
# reference whose diagnostic has no line of its own.
func fillLines(diags as list of Diagnostic, src as string) { # lint-disable: L205
    def lines as list of string init strings.split($src, "\n");
    for (def k in 0..len($diags)) {
        if ($diags[$k].line != 0) {
            continue;
        }
        def m as regex.Match init regex.find('"([^"]+)"', $diags[$k].message);
        if ($m.start < 0) {
            continue;
        }
        for (def j in 0..len($lines)) {
            if (strings.contains($lines[$j], $m.groups[0])) {
                $diags[$k].line = $j + 1;
                break;
            }
        }
    }
    return $diags;
}

func sortDiags(ds as list of Diagnostic) { # lint-disable: L205
    if (len($ds) < 2) {
        return $ds;
    }
    for (def k in 1..len($ds)) {
        def d as Diagnostic init $ds[$k];
        def j as int init $k - 1;
        while ($j >= 0 and $ds[$j].line > $d.line) {
            $ds[$j + 1] = $ds[$j];
            $j = $j - 1;
        }
        $ds[$j + 1] = $d;
    }
    return $ds;
}

# run parses a document, resolves its references and contents, and collects
# its diagnostics.
func run(src as string, opts as Options) {
    def doc as doctree.Node init withChildren("document", blocks($src, 1, $opts, 0));
    if (hasContents($doc)) {
        $doc = contentsPass($doc);
    }
    def labels as map of string to Target init {};
    $labels = collectLabels($doc, $labels);
    $doc = resolveRefs($doc, $labels);
    def diags as list of Diagnostic init sortDiags(fillLines(gatherDiags($doc), $src));
    if (len($diags) > 0) {
        $doc = stripDiags($doc);
    }
    if ($opts.strict and len($diags) > 0) {
        mystFail("line {$diags[0].line}: {$diags[0].message}");
    }
    return Result{doc: $doc, diags: $diags};
}

func defaultOptions() {
    def o as Options;
    return $o;
}

/**
 * Parse MyST into a document tree: a `doctree.Node` whose `typeOf` is
 * `"document"`, walked with `doctree.findAll` / `doctree.text` and friends and
 * rendered with `doctree.render` / `doctree.renderPdf`. Problems are recovered
 * from (see `check`); directives nested too deep are a catchable `"myst"`
 * error.
 * @param src {string} the MyST source
 * @return {doctree.Node} the document root node
 */
export func parse(src as string) {
    return run($src, defaultOptions()).doc;
}

/**
 * Parse MyST with explicit options.
 * @param src {string} the MyST source
 * @param opts {Options} the parse options
 * @return {doctree.Node} the document root node
 * @throws {Error} kind `"myst"` on the first problem when `opts.strict` is set
 */
export func parseWith(src as string, opts as Options) {
    return run($src, $opts).doc;
}

/**
 * The problems in a document, in source order: unknown directives, roles,
 * options and reference targets, and directives that could not be built. An
 * empty list means the document is clean.
 * @param src {string} the MyST source
 * @return {list of Diagnostic} the problems found
 */
export func check(src as string) {
    return run($src, defaultOptions()).diags;
}

/**
 * Render MyST to HTML. Safe by default: raw HTML is escaped and link targets go
 * through `doctree`'s URL filter.
 * @param src {string} the MyST source
 * @return {string} the rendered HTML
 */
export func toHtml(src as string) {
    return doctree.render("html", parse($src));
}

/**
 * Render MyST to well-formed XHTML (void elements self-close).
 * @param src {string} the MyST source
 * @return {string} the rendered XHTML
 */
export func toXhtml(src as string) {
    return doctree.render("xhtml", parse($src));
}

/**
 * Render MyST to HTML with explicit options.
 * @param src {string} the MyST source
 * @param opts {Options} the parse and render options
 * @return {string} the rendered HTML
 * @throws {Error} kind `"myst"` on the first problem when `opts.strict` is set
 */
export func toHtmlWith(src as string, opts as Options) {
    def html as doctree.HtmlOptions;
    $html.allowRawHtml = $opts.allowRawHtml;
    return doctree.renderHtml(parseWith($src, $opts), $html);
}

/**
 * Render MyST to styled terminal text.
 * @param src {string} the MyST source
 * @return {string} the rendered text, with ANSI styling
 */
export func toAnsi(src as string) {
    return doctree.render("ansi", parse($src));
}

/**
 * The front matter of a document: the YAML between the `---` lines that open
 * it, without those lines, or "" when it has none. Decode it with `yaml`.
 * @param src {string} the MyST source
 * @return {string} the front matter's YAML text
 */
export func frontMatter(src as string) {
    def lines as list of string init strings.split(strings.replace($src, "\r\n", "\n"), "\n");
    def end as int init frontMatterEnd($lines);
    if ($end < 0) {
        return "";
    }
    return strings.join($lines[1..$end], "\n");
}
