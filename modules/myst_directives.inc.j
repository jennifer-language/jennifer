# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0
#
# myst_directives.inc.j - the directives of the myst module. Included (textual
# splice) into myst.j; it declares no `use` of its own and relies on the
# includer's imports and helpers. Private to the myst module.
#
# markdown reads a directive as a fenced code block whose language is `{name}`;
# `directiveNodes` takes that block apart (the argument after the name, the
# `:option:` lines or `---` options block, the content) and builds doctree
# nodes for it. Content that is prose is parsed as MyST again, one level
# deeper. Every directive takes `:class:` (added to the node's classes) and
# `:name:` (a label: the node gets an id and references reach it). An unknown
# option is reported and ignored; an unknown directive is kept as a literal
# block of its source and reported.

def const ADMONITIONS as list of string init [
    "attention",
    "caution",
    "danger",
    "error",
    "hint",
    "important",
    "note",
    "tip",
    "warning",
    "seealso"
];
def const IMAGE_OPTIONS as list of string init [
    "alt",
    "height",
    "width",
    "scale",
    "align",
    "target",
    "loading"
];
def const TABLE_OPTIONS as list of string init [
    "widths",
    "width",
    "align",
    "stub-columns",
    "header-rows"
];
def const CODE_OPTIONS as list of string init [
    "linenos",
    "number-lines",
    "lineno-start",
    "emphasize-lines",
    "caption",
    "dedent",
    "force"
];

# A directive taken apart.
def struct Directive {
    name as string,
    arg as string,
    options as map of string to string,
    content as list of string,
    contentLine as int,
    line as int,
    source as string
};

# readDirective takes a `{name}` code block apart. The argument is the rest of
# the info string; options are `:key: value` lines at the top of the content,
# or a YAML-ish `---` block (`key: value` lines).
func readDirective(code as doctree.Node) {
    def d as Directive;
    $d.name = strings.lower($code.lang[1..len($code.lang) - 1]);
    def info as string init "";
    if (maps.has($code.attrs, "info")) {
        $info = $code.attrs["info"];
    }
    def lm as regex.Match init regex.find(LINE_MARK + '([0-9]+)' + LINE_MARK, $info);
    if ($lm.start >= 0) {
        $d.line = convert.toInt($lm.groups[0]);
        $info = $info[..$lm.start] + $info[$lm.end..];
    }
    def sp as int init strings.indexOf($info, " ");
    if ($sp >= 0) {
        $d.arg = strings.trim($info[$sp + 1..]);
    }
    $d.options = {};
    def lines as list of string init strings.split($code.text, "\n");
    if ($code.text == "") {
        $lines = [];
    }
    def k as int init 0;
    if (len($lines) > 0 and strings.trim($lines[0]) == "---") {
        def j as int init 1;
        while ($j < len($lines) and strings.trim($lines[$j]) != "---") {
            def om as regex.Match init regex.find('^\s*([A-Za-z0-9_-]+)\s*:\s*(.*)$', $lines[$j]);
            if ($om.start == 0) {
                $d.options[strings.lower($om.groups[0])] = unquote(strings.trim($om.groups[1]));
            }
            $j = $j + 1;
        }
        if ($j < len($lines)) {
            $k = $j + 1;
        }
    } else {
        while ($k < len($lines)) {
            def om2 as regex.Match init regex.find('^:([^:\s][^:]*):(?:\s+(.*))?$', $lines[$k]);
            if ($om2.start != 0) {
                break;
            }
            $d.options[strings.lower($om2.groups[0])] = strings.trim($om2.groups[1]);
            $k = $k + 1;
        }
    }
    $d.content = $lines[$k..];
    $d.contentLine = $d.line + 1 + $k;
    $d.source = $code.text;
    return $d;
}

func unquote(v as string) {
    if (len($v) >= 2 and
        (strings.startsWith($v, "\"") and strings.endsWith($v, "\"") or
        strings.startsWith($v, "'") and strings.endsWith($v, "'"))) {
        return $v[1..len($v) - 1];
    }
    return $v;
}

func hasContent(d as Directive) {
    for (def l in $d.content) {
        if (strings.trim($l) != "") {
            return true;
        }
    }
    return false;
}

func trimmedContent(content as list of string) {
    def a as int init 0;
    def b as int init len($content);
    while ($a < $b and strings.trim($content[$a]) == "") {
        $a = $a + 1;
    }
    while ($b > $a and strings.trim($content[$b - 1]) == "") {
        $b = $b - 1;
    }
    return $content[$a..$b];
}

func opt(d as Directive, name as string) {
    if (maps.has($d.options, $name)) {
        return $d.options[$name];
    }
    return "";
}

func classNames(s as string) {
    def out as list of string init [];
    for (def w in regex.split('\s+', strings.trim($s))) {
        def c as string init regex.replace('[^a-z0-9]+', strings.lower($w), "-");
        $c = regex.replace('^[^a-z]+', $c, "");
        $c = regex.replace('-+$', $c, "");
        if ($c != "") {
            $out[] = $c;
        }
    }
    return strings.join($out, " ");
}

func addClass(node as doctree.Node, cls as string) {
    def n as doctree.Node init $node;
    if ($cls == "") {
        return $n;
    }
    if (maps.has($n.attrs, "class") and $n.attrs["class"] != "") {
        $n.attrs["class"] = $n.attrs["class"] + " " + $cls;
    } else {
        $n.attrs["class"] = $cls;
    }
    return $n;
}

func withoutOption(d as Directive, name as string) {
    def out as Directive init $d;
    def opts as map of string to string init {};
    for (def k in maps.keys($d.options)) {
        if ($k != $name) {
            $opts[$k] = $d.options[$k];
        }
    }
    $out.options = $opts;
    return $out;
}

# contentBlocks parses a directive's content as MyST, one level deeper.
func contentBlocks(d as Directive, opts as Options, depth as int) {
    return blocks(strings.join($d.content, "\n"), $d.contentLine, $opts, $depth + 1);
}

# directiveError reports a directive that could not be built and keeps its
# source as a literal block.
func directiveError(d as Directive, msg as string) {
    def code as doctree.Node init mk("code");
    def head as string init '```{' + $d.name + '}';
    if ($d.arg != "") {
        $head = $head + " " + $d.arg;
    }
    def body as string init strings.trimRight($d.source);
    if ($body == "") {
        $code.text = $head + "\n```";
    } else {
        $code.text = $head + "\n" + $body + "\n```";
    }
    $code.attrs = {"class": "literal-block"};
    return [diagNode("error", $d.line, $msg), $code];
}

# finish wraps up a built directive: unknown options are reported, `:class:`
# goes on the node and `:name:` labels it.
func finish(d as Directive, node as doctree.Node, allowed as list of string) {
    def out as list of doctree.Node init [];
    for (def k in maps.keys($d.options)) {
        if ($k != "class" and $k != "name" and not lists.contains($allowed, $k)) {
            $out[] = diagNode(
                "error",
                $d.line,
                "Unknown option \"" + $k + "\" for the \"" + $d.name + "\" directive; ignored.");
        }
    }
    def n as doctree.Node init addClass($node, classNames(opt($d, "class")));
    if (opt($d, "name") != "") {
        $n = labelNode($n, opt($d, "name"));
    }
    $out[] = $n;
    return $out;
}

# directiveNodes builds the nodes for one directive block.
func directiveNodes(code as doctree.Node, opts as Options, depth as int) {
    def d as Directive init readDirective($code);
    if (lists.contains(ADMONITIONS, $d.name) or $d.name == "admonition") {
        return admonitionNodes($d, $opts, $depth);
    }
    match ($d.name) {
        when "code", "code-block", "sourcecode", "code-cell" { return codeNodes($d); }
        when "math" { return mathNodes($d); }
        when "image" { return imageNodes($d); }
        when "figure" { return figureNodes($d, $opts, $depth); }
        when "topic", "sidebar" { return topicNodes($d, $opts, $depth); }
        when "rubric" {
            if ($d.arg == "") {
                return directiveError($d, "The \"rubric\" directive needs its text.");
            }
            return finish($d, withChildren("rubric", [textOf($d.arg)]), []);
        }
        when "epigraph", "highlights", "pull-quote" {
            if (not hasContent($d)) {
                return directiveError(
                    $d,
                    "The \"" + $d.name + "\" directive is empty; content is required.");
            }
            def q as doctree.Node init withChildren("quote", contentBlocks($d, $opts, $depth));
            $q.attrs = {"class": $d.name};
            return finish($d, $q, []);
        }
        when "container" {
            def c as doctree.Node init withChildren("container", contentBlocks($d, $opts, $depth));
            if ($d.arg != "") {
                $c.attrs = {"class": classNames($d.arg)};
            }
            return finish($d, $c, []);
        }
        when "raw" { return rawNodes($d); }
        when "table" { return tableNodes($d, $opts, $depth); }
        when "list-table" { return listTableNodes($d, $opts, $depth); }
        when "csv-table" { return csvTableNodes($d, $opts, $depth); }
        when "contents", "toc" {
            if ($depth > 0) {
                return directiveError(
                    $d,
                    "The \"contents\" directive is only supported at the top level of a document.");
            }
            def p as doctree.Node init mk("myst_contents");
            $p.title = $d.arg;
            if (regex.matches('^[0-9]+$', opt($d, "depth"))) {
                $p.attrs["depth"] = opt($d, "depth");
            }
            if (maps.has($d.options, "local")) {
                $p.attrs["local"] = "true";
            }
            return finish($d, $p, ["depth", "local", "backlinks"]);
        }
        when "eval-rst", "eval_rst" { return evalRst($d, $opts); }
        when "include", "literalinclude" {
            return directiveError(
                $d,
                "The \"" + $d.name + "\" directive is not supported (myst reads no files).");
        }
        else { return directiveError($d, "Unknown directive type \"" + $d.name + "\"."); }
    }
}

func admonitionNodes(d as Directive, opts as Options, depth as int) {
    def node as doctree.Node init mk("admonition");
    $node.lang = $d.name;
    def content as list of string init $d.content;
    if ($d.name == "admonition") {
        if ($d.arg == "") {
            return directiveError($d, "The \"admonition\" directive needs a title.");
        }
        $node.title = $d.arg;
        $node.lang = slugOf($d.arg);
    } elseif ($d.arg != "") {
        # A specific admonition's first line is the start of its content.
        def lead as list of string init [$d.arg];
        for (def l in $content) {
            $lead[] = $l;
        }
        $content = $lead;
    }
    if ($d.name == "seealso") {
        $node.title = "See also";
    }
    def dd as Directive init $d;
    $dd.content = $content;
    if (not hasContent($dd)) {
        return directiveError(
            $d,
            "The \"" + $d.name + "\" admonition is empty; content is required.");
    }
    $node.children = contentBlocks($dd, $opts, $depth);
    return finish($d, $node, []);
}

func topicNodes(d as Directive, opts as Options, depth as int) {
    if ($d.arg == "") {
        return directiveError($d, "The \"" + $d.name + "\" directive needs a title.");
    }
    def node as doctree.Node init withChildren("topic", contentBlocks($d, $opts, $depth));
    $node.title = $d.arg;
    if ($d.name == "sidebar") {
        $node.attrs = {"class": "sidebar"};
    }
    return finish($d, $node, ["subtitle"]);
}

func codeNodes(d as Directive) {
    def node as doctree.Node init mk("code");
    $node.text = strings.join(trimmedContent($d.content), "\n");
    $node.lang = strings.lower(strings.trim($d.arg));
    return finish($d, $node, CODE_OPTIONS);
}

# mathNodes makes each blank-line-separated block of a math directive a code
# block with class `math`; `:label:` labels the first one for `{eq}`.
func mathNodes(d as Directive) {
    def parts as list of string init [];
    def cur as list of string init [];
    for (def l in $d.content) {
        if (strings.trim($l) == "") {
            if (len($cur) > 0) {
                $parts[] = strings.join($cur, "\n");
                $cur = [];
            }
            continue;
        }
        $cur[] = $l;
    }
    if (len($cur) > 0) {
        $parts[] = strings.join($cur, "\n");
    }
    if ($d.arg != "") {
        $parts = lists.concat([$d.arg], $parts);
    }
    if (len($parts) == 0) {
        return directiveError($d, "The \"math\" directive is empty; content is required.");
    }
    def out as list of doctree.Node init [];
    for (def k in 0..len($parts)) {
        def node as doctree.Node init mk("code");
        $node.text = $parts[$k];
        $node.attrs = {"class": "math"};
        def dd as Directive init $d;
        if ($k > 0) {
            $dd.options = {};
        } elseif (opt($d, "label") != "") {
            $dd = withoutOption($d, "label");
            $node = labelNode($node, opt($d, "label"));
        }
        for (def x in finish($dd, $node, ["nowrap"])) {
            $out[] = $x;
        }
    }
    return $out;
}

func rawNodes(d as Directive) {
    if ($d.arg == "") {
        return directiveError($d, "The \"raw\" directive needs an output format.");
    }
    if (opt($d, "file") != "" or opt($d, "url") != "") {
        return directiveError(
            $d,
            "The \"raw\" directive's :file: and :url: options are not supported (myst reads no files).");
    }
    if (not lists.contains(regex.split('\s+', strings.lower($d.arg)), "html")) {
        return [];
    }
    def node as doctree.Node init mk("html_block");
    $node.text = strings.join(trimmedContent($d.content), "\n");
    return finish($d, $node, ["encoding"]);
}

# imageOf is the image (or the linked image, with `:target:`) a directive
# names; `align` becomes an `align-*` class.
func imageOf(d as Directive) {
    def img as doctree.Node init mk("image");
    $img.url = regex.replace('\s+', $d.arg, "");
    $img.text = $img.url;
    if (opt($d, "alt") != "") {
        $img.text = opt($d, "alt");
    }
    def attrs as map of string to string init {};
    for (def k in ["width", "height"]) {
        if (opt($d, $k) != "") {
            $attrs[$k] = opt($d, $k);
        }
    }
    def classes as list of string init [];
    if (opt($d, "class") != "") {
        $classes[] = classNames(opt($d, "class"));
    }
    if (opt($d, "align") != "") {
        $classes[] = "align-" + classNames(opt($d, "align"));
    }
    if (len($classes) > 0) {
        $attrs["class"] = strings.join($classes, " ");
    }
    $img.attrs = $attrs;
    if (opt($d, "target") == "") {
        return $img;
    }
    def l as doctree.Node init withChildren("link", [$img]);
    $l.url = opt($d, "target");
    return $l;
}

func imageNodes(d as Directive) {
    if ($d.arg == "") {
        return directiveError($d, "The \"image\" directive needs a URI.");
    }
    return finish(
        withoutOption($d, "class"),
        withChildren("paragraph", [imageOf($d)]),
        IMAGE_OPTIONS);
}

# figureNodes is an image with an optional caption (the first paragraph of its
# content) and legend (the rest).
func figureNodes(d as Directive, opts as Options, depth as int) {
    if ($d.arg == "") {
        return directiveError($d, "The \"figure\" directive needs a URI.");
    }
    def fig as doctree.Node init withChildren("figure", [imageOf(withoutOption($d, "class"))]);
    def body as list of doctree.Node init contentBlocks($d, $opts, $depth);
    def k as int init 0;
    while ($k < len($body) and $body[$k].kind == "myst_diag") {
        $fig.children[] = $body[$k];
        $k = $k + 1;
    }
    if ($k < len($body) and $body[$k].kind == "paragraph") {
        $fig.children[] = withChildren("caption", $body[$k].children);
        $k = $k + 1;
    }
    if ($k < len($body)) {
        $fig.children[] = withChildren("legend", $body[$k..]);
    }
    def classes as list of string init [];
    if (opt($d, "figclass") != "") {
        $classes[] = classNames(opt($d, "figclass"));
    }
    if (opt($d, "class") != "") {
        $classes[] = classNames(opt($d, "class"));
    }
    if (opt($d, "align") != "") {
        $classes[] = "align-" + classNames(opt($d, "align"));
    }
    if (len($classes) > 0) {
        $fig.attrs = {"class": strings.join($classes, " ")};
    }
    if (opt($d, "figwidth") != "") {
        $fig.attrs["width"] = opt($d, "figwidth");
    }
    def allowed as list of string init lists.concat(["figwidth", "figclass"], IMAGE_OPTIONS);
    return finish(withoutOption($d, "class"), $fig, $allowed);
}

# --- tables ---------------------------------------------------------

func headerRowsOpt(d as Directive) {
    def v as string init opt($d, "header-rows");
    if ($v == "") {
        return 0;
    }
    if (not regex.matches('^[0-9]+$', $v)) {
        return -1;
    }
    return convert.toInt($v);
}

# tableExtras: the argument is the table's caption, `align` a class.
func tableExtras(t as doctree.Node, d as Directive) {
    def out as doctree.Node init $t;
    if ($d.arg != "") {
        $out.title = $d.arg;
    }
    if (opt($d, "align") != "") {
        $out = addClass($out, "align-" + classNames(opt($d, "align")));
    }
    return $out;
}

# tableNodes is the `table` directive: a caption around the Markdown table in
# its content.
func tableNodes(d as Directive, opts as Options, depth as int) {
    def found as doctree.Node init mk("");
    def out as list of doctree.Node init [];
    for (def b in contentBlocks($d, $opts, $depth)) {
        if ($b.kind == "table" and $found.kind == "") {
            $found = $b;
        } elseif ($b.kind == "myst_diag") {
            $out[] = $b;
        }
    }
    if ($found.kind == "") {
        return directiveError($d, "The \"table\" directive needs a table in its content.");
    }
    for (def x in finish($d, tableExtras($found, $d), TABLE_OPTIONS)) {
        $out[] = $x;
    }
    return $out;
}

func tableOf(rows as list of doctree.Node, headerRows as int) {
    def t as doctree.Node init withChildren("table", $rows);
    $t.attrs = {"headerRows": "{$headerRows}"};
    return $t;
}

func listTableNodes(d as Directive, opts as Options, depth as int) {
    def bad as string init "The \"list-table\" directive needs a uniform two-level bullet list.";
    def lst as list of doctree.Node init [];
    for (def b in contentBlocks($d, $opts, $depth)) {
        if ($b.kind != "myst_diag") {
            $lst[] = $b;
        }
    }
    if (len($lst) != 1 or $lst[0].kind != "list" or $lst[0].ordered) {
        return directiveError($d, $bad);
    }
    def rows as list of doctree.Node init [];
    def width as int init -1;
    for (def item in $lst[0].children) {
        def sub as doctree.Node init mk("");
        for (def c in $item.children) {
            if ($c.kind == "list") {
                $sub = $c;
            }
        }
        if ($sub.kind != "list" or $sub.ordered) {
            return directiveError($d, $bad);
        }
        def row as doctree.Node init mk("row");
        for (def cell in $sub.children) {
            $row.children[] = withChildren("cell", $cell.children);
        }
        if ($width >= 0 and len($row.children) != $width) {
            return directiveError($d, $bad + " Its rows have different numbers of cells.");
        }
        $width = len($row.children);
        $rows[] = $row;
    }
    def hr as int init headerRowsOpt($d);
    if ($hr < 0 or $hr > len($rows)) {
        return directiveError(
            $d,
            "The \"list-table\" directive's :header-rows: is not a row count of the table.");
    }
    return finish($d, tableExtras(tableOf($rows, $hr), $d), TABLE_OPTIONS);
}

func csvTableNodes(d as Directive, opts as Options, depth as int) {
    if (opt($d, "file") != "" or opt($d, "url") != "") {
        return directiveError(
            $d,
            "The \"csv-table\" directive's :file: and :url: options are not supported (myst reads no files).");
    }
    def delim as string init ",";
    match (strings.lower(opt($d, "delim"))) {
        when "", "comma" { $delim = ","; }
        when "tab" { $delim = "\t"; }
        when "space" { $delim = " "; }
        else { $delim = opt($d, "delim"); }
    }
    if (len($delim) != 1) {
        return directiveError($d, "The \"csv-table\" directive's :delim: must be one character.");
    }
    def dia as csv.Dialect init csv.dialect($delim);
    def all as list of list of string init [];
    def headN as int init 0;
    try {
        if (opt($d, "header") != "") {
            $all = csv.parseDialect(opt($d, "header"), $dia);
            $headN = len($all);
        }
        for (def r in csv.parseDialect(strings.join(trimmedContent($d.content), "\n"), $dia)) {
            $all[] = $r;
        }
    } catch (e) {
        return directiveError($d, "The \"csv-table\" directive's data is not valid CSV.");
    }
    if (len($all) == $headN) {
        return directiveError($d, "The \"csv-table\" directive is empty; content is required.");
    }
    def width as int init 0;
    for (def r in $all) {
        if (len($r) > $width) {
            $width = len($r);
        }
    }
    def rows as list of doctree.Node init [];
    for (def r in $all) {
        def row as doctree.Node init mk("row");
        for (def c in 0..$width) {
            def text as string init "";
            if ($c < len($r)) {
                $text = strings.trim($r[$c]);
            }
            def cell as doctree.Node init mk("cell");
            if ($text != "") {
                def kids as list of doctree.Node init markdown.parse($text).children;
                if (len($kids) == 1 and $kids[0].kind == "paragraph") {
                    $kids = $kids[0].children;
                }
                $cell.children = lowerInline($kids);
            }
            $row.children[] = $cell;
        }
        $rows[] = $row;
    }
    def hr as int init headerRowsOpt($d);
    if ($hr < 0) {
        return directiveError(
            $d,
            "The \"csv-table\" directive's :header-rows: is not a row count of the table.");
    }
    def allowed as list of string init lists.concat(
        TABLE_OPTIONS,
        ["header", "delim", "quote", "keepspace", "escape", "encoding"]);
    return finish($d, tableExtras(tableOf($rows, $headN + $hr), $d), $allowed);
}

# evalRst reads its content as reStructuredText with the rst module; rST's own
# problems come back as this document's, at their lines here.
func evalRst(d as Directive, opts as Options) {
    def src as string init strings.join($d.content, "\n");
    def ro as rst.Options;
    $ro.allowRawHtml = $opts.allowRawHtml;
    $ro.noTitlePromotion = true;
    def out as list of doctree.Node init [];
    for (def pd in rst.check($src)) {
        $out[] = diagNode($pd.severity, $d.contentLine + $pd.line - 1, "eval-rst: " + $pd.message);
    }
    for (def c in rst.parseWith($src, $ro).children) {
        $out[] = $c;
    }
    return $out;
}

# --- the table of contents ------------------------------------------

def struct TocEntry {
    level as int,
    id as string,
    text as string
};

def struct TocBuild {
    node as doctree.Node,
    next as int
};

func hasContents(doc as doctree.Node) {
    for (def c in $doc.children) {
        if ($c.kind == "myst_contents") {
            return true;
        }
    }
    return false;
}

# contentsPass gives every top-level heading without an id one (a slug), then
# replaces each contents placeholder with a `topic` (class `contents`) of links
# to the sections - all of them, or with `:local:` those of the section it is
# in.
func contentsPass(doc as doctree.Node) {
    def kids as list of doctree.Node init $doc.children;
    def ids as map of string to bool init {};
    for (def k in 0..len($kids)) {
        if ($kids[$k].kind != "heading") {
            continue;
        }
        if (maps.has($kids[$k].attrs, "id")) {
            $ids[$kids[$k].attrs["id"]] = true;
            continue;
        }
        def base as string init slugOf(doctree.text($kids[$k]));
        def id as string init $base;
        def n as int init 2;
        while (maps.has($ids, $id)) {
            $id = "{$base}-{$n}";
            $n = $n + 1;
        }
        $ids[$id] = true;
        $kids[$k].attrs["id"] = $id;
    }
    def out as list of doctree.Node init [];
    for (def k in 0..len($kids)) {
        if ($kids[$k].kind != "myst_contents") {
            $out[] = $kids[$k];
            continue;
        }
        def entries as list of TocEntry init tocEntries($kids, $k);
        if (len($entries) == 0) {
            continue;
        }
        def maxDepth as int init 0;
        if (maps.has($kids[$k].attrs, "depth")) {
            $maxDepth = convert.toInt($kids[$k].attrs["depth"]);
        }
        def topic as doctree.Node init withChildren(
            "topic",
            [tocBuild($entries, 0, 1, $maxDepth).node]);
        def local as bool init maps.has($kids[$k].attrs, "local");
        $topic.title = $kids[$k].title;
        if ($topic.title == "" and not $local) {
            $topic.title = "Contents";
        }
        $topic.attrs = {"class": "contents"};
        if (maps.has($kids[$k].attrs, "class")) {
            $topic.attrs["class"] = "contents " + $kids[$k].attrs["class"];
        }
        $out[] = $topic;
    }
    def d2 as doctree.Node init $doc;
    $d2.children = $out;
    return $d2;
}

func tocEntries(kids as list of doctree.Node, at as int) {
    def local as bool init maps.has($kids[$at].attrs, "local");
    def from as int init 0;
    def stop as int init 0;
    if ($local) {
        $from = $at + 1;
        for (def k in 0..$at) {
            if ($kids[$k].kind == "heading") {
                $stop = $kids[$k].level;
            }
        }
    }
    def out as list of TocEntry init [];
    for (def k in $from..len($kids)) {
        def h as doctree.Node init $kids[$k];
        if ($h.kind != "heading") {
            continue;
        }
        if ($local and $h.level <= $stop) {
            break;
        }
        $out[] = TocEntry{level: $h.level, id: $h.attrs["id"], text: doctree.text($h)};
    }
    return $out;
}

func tocBuild(entries as list of TocEntry, k as int, depth as int, maxDepth as int) {
    def lvl as int init $entries[$k].level;
    def items as list of doctree.Node init [];
    def j as int init $k;
    while ($j < len($entries) and $entries[$j].level >= $lvl) {
        def e as TocEntry init $entries[$j];
        def link as doctree.Node init withChildren("link", [textOf($e.text)]);
        $link.url = "#" + $e.id;
        def item as doctree.Node init withChildren("item", [$link]);
        $j = $j + 1;
        if ($j < len($entries) and $entries[$j].level > $lvl) {
            def sub as TocBuild init tocBuild($entries, $j, $depth + 1, $maxDepth);
            if ($maxDepth == 0 or $depth < $maxDepth) {
                $item.children[] = $sub.node;
            }
            $j = $sub.next;
        }
        $items[] = $item;
    }
    return TocBuild{node: withChildren("list", $items), next: $j};
}
