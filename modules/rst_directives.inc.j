# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0
#
# rst_directives.inc.j - the block directives of the rst module. Included
# (textual splice) into rst.j; it declares no `use` of its own and relies on the
# includer's imports and helpers. Private to the rst module.
#
# `directiveBlock` reads a `.. name:: argument` block into a Directive (its
# argument, `:option:` lines and content) and builds the doctree nodes for it.
# Every directive takes `:class:` (added to the node's classes) and `:name:`
# (an internal target, so the node gets an id and `name_` links to it). An
# unknown option is reported and ignored; an unknown directive is kept as a
# literal block. Two directives leave placeholders for a later pass:
#
#   rst_class     `.. class::` with no content: `text` the classes, given to
#                 the next element in the same body by `applyClasses`
#   rst_contents  `.. contents::`: `title`, attrs `depth` / `local`, replaced
#                 by a table of contents once the headings are final
#                 (`contentsPass`)

def const ADMONITIONS as list of string init [
    "attention",
    "caution",
    "danger",
    "error",
    "hint",
    "important",
    "note",
    "tip",
    "warning"
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
def const CSV_OPTIONS as list of string init [
    "header",
    "delim",
    "quote",
    "keepspace",
    "escape",
    "file",
    "url",
    "encoding"
];
# Sphinx's options for `code-block`, accepted (and ignored) so Sphinx sources
# read without warnings.
def const CODE_OPTIONS as list of string init [
    "number-lines",
    "linenos",
    "lineno-start",
    "emphasize-lines",
    "caption",
    "dedent",
    "force"
];

# A directive block read apart. `content` is dedented; `contentLine` is the
# source line of content[0]; `next` the first line after the block.
def struct Directive {
    name as string,
    arg as string,
    options as map of string to string,
    content as list of string,
    contentLine as int,
    line as int,
    next as int
};

# readDirective splits a directive block. A directive that takes an argument
# reads it from the rest of the first line and the lines right under it (up to a
# blank or an option line); one that does not takes the first line's text as the
# start of its content. Options are the `:name: value` lines that follow.
func readDirective(
    lines as list of string,
    i as int,
    blk as IndentBlock,
    m as regex.Match,
    base as int,
    takesArg as bool) {
    def d as Directive;
    $d.name = strings.lower($m.groups[0]);
    $d.line = $base + $i;
    $d.next = $blk.next;
    def first as string init strings.trim($lines[$i][$m.end..]);
    def rest as list of string init $blk.lines[1..];
    def k as int init 0;
    if (not $takesArg and $first != "") {
        $d.content = [$first];
        for (def l in $rest) {
            $d.content[] = $l;
        }
        $d.contentLine = $base + $i;
        return $d;
    }
    def arg as string init $first;
    if ($takesArg) {
        while ($k < len($rest) and len($rest[$k]) > 0 and
            regex.find(OPTION_LINE_RE, $rest[$k]).start != 0) {
            $arg = strings.trim($arg + " " + strings.trim($rest[$k]));
            $k = $k + 1;
        }
    }
    $d.arg = $arg;
    def body as DirectiveBody init directiveBody($rest[$k..]);
    $d.options = $body.options;
    $d.content = $body.content;
    $d.contentLine = $base + $i + 1 + $k + len($body.options);
    return $d;
}

# withoutOption is the directive with one option taken out (the image
# directives put `:class:` on the image, not on its wrapper).
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

func hasContent(d as Directive) {
    for (def l in $d.content) {
        if (len($l) > 0) {
            return true;
        }
    }
    return false;
}

# trimmedContent is the content without the blank lines before and after it.
func trimmedContent(content as list of string) {
    def a as int init 0;
    def b as int init len($content);
    while ($a < $b and len($content[$a]) == 0) {
        $a = $a + 1;
    }
    while ($b > $a and len($content[$b - 1]) == 0) {
        $b = $b - 1;
    }
    return $content[$a..$b];
}

func contentBlocks(d as Directive, depth as int) {
    return parseBlocks($d.content, $d.contentLine, $depth + 1, false);
}

func opt(d as Directive, name as string) {
    if (maps.has($d.options, $name)) {
        return $d.options[$name];
    }
    return "";
}

# classNames normalises a `:class:` value the way docutils does: each
# whitespace-separated name lowercased, runs of anything but a-z and 0-9 made one
# hyphen, leading non-letters dropped.
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

# directiveError reports a directive that could not be built and keeps its
# source as a literal block.
func directiveError(lines as list of string, i as int, d as Directive, msg as string) {
    def code as doctree.Node init mk("code");
    $code.text = strings.join($lines[$i..$d.next], "\n");
    $code.attrs = {"class": "literal-block"};
    return Step{nodes: [diagNode("error", $d.line, $msg), $code], next: $d.next};
}

# finish wraps up a built directive: unknown options are reported, `:class:` is
# added to the node and `:name:` becomes an internal target in front of it.
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
    if (opt($d, "name") != "") {
        $out[] = targetNode("_`" + opt($d, "name") + "`:", $d.line);
    }
    $out[] = addClass($node, classNames(opt($d, "class")));
    return Step{nodes: $out, next: $d.next};
}

# directiveBlock builds the nodes for one directive block.
func directiveBlock(
    lines as list of string,
    i as int,
    blk as IndentBlock,
    m as regex.Match,
    base as int,
    depth as int) {
    def name as string init strings.lower($m.groups[0]);
    if (lists.contains(ADMONITIONS, $name)) {
        return admonitionDirective(
            $lines,
            $i,
            readDirective($lines, $i, $blk, $m, $base, false),
            $depth);
    }
    match ($name) {
        when "admonition" {
            return admonitionDirective(
                $lines,
                $i,
                readDirective($lines, $i, $blk, $m, $base, true),
                $depth);
        }
        when "code", "code-block", "sourcecode" {
            return codeDirective($lines, $i, readDirective($lines, $i, $blk, $m, $base, true));
        }
        when "math" {
            return mathDirective($lines, $i, readDirective($lines, $i, $blk, $m, $base, true));
        }
        when "image" {
            return imageDirective($lines, $i, readDirective($lines, $i, $blk, $m, $base, true));
        }
        when "figure" {
            return figureDirective(
                $lines,
                $i,
                readDirective($lines, $i, $blk, $m, $base, true),
                $depth);
        }
        when "topic", "sidebar" {
            return topicDirective(
                $lines,
                $i,
                readDirective($lines, $i, $blk, $m, $base, true),
                $depth);
        }
        when "rubric" {
            return rubricDirective($lines, $i, readDirective($lines, $i, $blk, $m, $base, true));
        }
        when "epigraph", "highlights", "pull-quote" {
            return quoteDirective(
                $lines,
                $i,
                readDirective($lines, $i, $blk, $m, $base, false),
                $depth);
        }
        when "container", "compound" {
            return containerDirective(
                $lines,
                $i,
                readDirective($lines, $i, $blk, $m, $base, true),
                $depth);
        }
        when "class" {
            return classDirective(
                $lines,
                $i,
                readDirective($lines, $i, $blk, $m, $base, true),
                $depth);
        }
        when "raw" {
            return rawDirective($lines, $i, readDirective($lines, $i, $blk, $m, $base, true));
        }
        when "contents" {
            return contentsDirective(
                $lines,
                $i,
                readDirective($lines, $i, $blk, $m, $base, true),
                $depth);
        }
        when "list-table" {
            return listTableDirective(
                $lines,
                $i,
                readDirective($lines, $i, $blk, $m, $base, true),
                $depth);
        }
        when "csv-table" {
            return csvTableDirective(
                $lines,
                $i,
                readDirective($lines, $i, $blk, $m, $base, true),
                $depth);
        }
        when "include" {
            def inc as Directive init readDirective($lines, $i, $blk, $m, $base, true);
            return directiveError(
                $lines,
                $i,
                $inc,
                "The \"include\" directive is not supported (rst reads no files).");
        }
        else { return unknownDirective($lines, $i, $blk.next, $m.groups[0], $base); }
    }
}

# --- admonitions, topics, quotes, containers -----------------------------

func admonitionDirective(lines as list of string, i as int, d as Directive, depth as int) {
    def node as doctree.Node init mk("admonition");
    $node.lang = $d.name;
    if ($d.name == "admonition") {
        if ($d.arg == "") {
            return directiveError($lines, $i, $d, "The \"admonition\" directive needs a title.");
        }
        $node.title = $d.arg;
        $node.lang = slugOf($d.arg);
    }
    if (not hasContent($d)) {
        return directiveError(
            $lines,
            $i,
            $d,
            "The \"" + $d.name + "\" admonition is empty; content is required.");
    }
    $node.children = contentBlocks($d, $depth);
    return finish($d, $node, []);
}

func topicDirective(lines as list of string, i as int, d as Directive, depth as int) {
    if ($d.arg == "") {
        return directiveError($lines, $i, $d, "The \"" + $d.name + "\" directive needs a title.");
    }
    if (not hasContent($d)) {
        return directiveError(
            $lines,
            $i,
            $d,
            "The \"" + $d.name + "\" directive is empty; content is required.");
    }
    def node as doctree.Node init withChildren("topic", contentBlocks($d, $depth));
    $node.title = $d.arg;
    if ($d.name == "sidebar") {
        $node.attrs = {"class": "sidebar"};
    }
    return finish($d, $node, ["subtitle"]);
}

func rubricDirective(lines as list of string, i as int, d as Directive) {
    if ($d.arg == "") {
        return directiveError($lines, $i, $d, "The \"rubric\" directive needs its text.");
    }
    return finish($d, withChildren("rubric", inlineNodes($d.arg, $d.line)), []);
}

# quoteDirective is `epigraph` / `highlights` / `pull-quote`: a block quote
# (with its attribution) carrying the directive's name as a class.
func quoteDirective(lines as list of string, i as int, d as Directive, depth as int) {
    if (not hasContent($d)) {
        return directiveError(
            $lines,
            $i,
            $d,
            "The \"" + $d.name + "\" directive is empty; content is required.");
    }
    def indented as list of string init [];
    for (def l in trimmedContent($d.content)) {
        if (len($l) == 0) {
            $indented[] = "";
        } else {
            $indented[] = "  " + $l;
        }
    }
    def q as Step init blockQuote($indented, 0, $d.contentLine, $depth);
    def node as doctree.Node init withChildren("quote", []);
    for (def x in $q.nodes) {
        if ($x.kind == "quote") {
            for (def c in $x.children) {
                $node.children[] = $c;
            }
        } else {
            $node.children[] = $x;
        }
    }
    $node.attrs = {"class": $d.name};
    return finish($d, $node, []);
}

func containerDirective(lines as list of string, i as int, d as Directive, depth as int) {
    if (not hasContent($d)) {
        return directiveError(
            $lines,
            $i,
            $d,
            "The \"" + $d.name + "\" directive is empty; content is required.");
    }
    def node as doctree.Node init withChildren("container", contentBlocks($d, $depth));
    if ($d.name == "compound") {
        $node.attrs = {"class": "compound"};
    } elseif ($d.arg != "") {
        $node.attrs = {"class": classNames($d.arg)};
    }
    return finish($d, $node, []);
}

# classDirective gives its classes to each block of its content, or with no
# content to the next element (an rst_class placeholder applyClasses resolves).
func classDirective(lines as list of string, i as int, d as Directive, depth as int) {
    def cls as string init classNames($d.arg);
    if ($cls == "") {
        return directiveError(
            $lines,
            $i,
            $d,
            "The \"class\" directive needs one or more class names.");
    }
    if (not hasContent($d)) {
        def p as doctree.Node init mk("rst_class");
        $p.text = $cls;
        $p.level = $d.line;
        return oneStep($p, $d.next);
    }
    def out as list of doctree.Node init [];
    for (def b in contentBlocks($d, $depth)) {
        if (isPlaceholder($b.kind) or $b.kind == "rst_diag") {
            $out[] = $b;
        } else {
            $out[] = addClass($b, $cls);
        }
    }
    return Step{nodes: $out, next: $d.next};
}

# applyClasses hands each rst_class placeholder's classes to the element after
# it in the same body (past targets and diagnostics). One with nothing after it
# is reported. Call it as `$kids = applyClasses($kids);`.
func applyClasses(kids as list of doctree.Node) { # lint-disable: L205
    def out as list of doctree.Node init [];
    def pending as string init "";
    def pendingLine as int init 0;
    for (def k in $kids) {
        if ($k.kind == "rst_class") {
            if ($pending != "") {
                $pending = $pending + " ";
            }
            $pending = $pending + $k.text;
            $pendingLine = $k.level;
            continue;
        }
        if ($pending != "" and not isPlaceholder($k.kind) and $k.kind != "rst_diag") {
            $out[] = addClass($k, $pending);
            $pending = "";
            continue;
        }
        $out[] = $k;
    }
    if ($pending != "") {
        $out[] = diagNode(
            "warning",
            $pendingLine,
            "The \"class\" directive has no element after it to apply to.");
    }
    return $out;
}

func hasKind(kids as list of doctree.Node, kind as string) {
    for (def k in $kids) {
        if ($k.kind == $kind) {
            return true;
        }
    }
    return false;
}

# --- literal content: code, math, raw ----------------------------------

func codeDirective(lines as list of string, i as int, d as Directive) {
    if (not hasContent($d)) {
        return directiveError(
            $lines,
            $i,
            $d,
            "The \"" + $d.name + "\" directive is empty; content is required.");
    }
    def node as doctree.Node init mk("code");
    $node.text = strings.join(trimmedContent($d.content), "\n");
    $node.lang = strings.lower(strings.trim($d.arg));
    return finish($d, $node, CODE_OPTIONS);
}

# mathDirective makes each blank-line-separated block of its content (and its
# argument, if any) a code block with class `math`; the TeX source is kept as
# written for a client-side renderer.
func mathDirective(lines as list of string, i as int, d as Directive) {
    def blocks as list of string init [];
    if ($d.arg != "") {
        $blocks[] = $d.arg;
    }
    def cur as list of string init [];
    for (def l in $d.content) {
        if (len($l) == 0) {
            if (len($cur) > 0) {
                $blocks[] = strings.join($cur, "\n");
                $cur = [];
            }
            continue;
        }
        $cur[] = $l;
    }
    if (len($cur) > 0) {
        $blocks[] = strings.join($cur, "\n");
    }
    if (len($blocks) == 0) {
        return directiveError(
            $lines,
            $i,
            $d,
            "The \"math\" directive is empty; content is required.");
    }
    def out as list of doctree.Node init [];
    for (def b in $blocks) {
        def node as doctree.Node init mk("code");
        $node.text = $b;
        $node.attrs = {"class": "math"};
        def st as Step init finish($d, $node, []);
        for (def x in $st.nodes) {
            $out[] = $x;
        }
        # Options apply once: the first block takes the name and the reports.
        $d.options = {"class": opt($d, "class")};
    }
    return Step{nodes: $out, next: $d.next};
}

# rawDirective passes `html` content through as an html_block, which the
# renderer escapes unless raw HTML is allowed. Content for other formats is
# dropped; reading it from a file or URL is refused.
func rawDirective(lines as list of string, i as int, d as Directive) {
    if ($d.arg == "") {
        return directiveError($lines, $i, $d, "The \"raw\" directive needs an output format.");
    }
    if (opt($d, "file") != "" or opt($d, "url") != "") {
        return directiveError(
            $lines,
            $i,
            $d,
            "The \"raw\" directive's :file: and :url: options are not supported (rst reads no files).");
    }
    if (not hasContent($d)) {
        return directiveError(
            $lines,
            $i,
            $d,
            "The \"raw\" directive is empty; content is required.");
    }
    def formats as list of string init regex.split('\s+', strings.lower(strings.trim($d.arg)));
    if (not lists.contains($formats, "html")) {
        return noStep($d.next);
    }
    def node as doctree.Node init mk("html_block");
    $node.text = strings.join(trimmedContent($d.content), "\n");
    return finish($d, $node, ["encoding"]);
}

# --- images and figures -------------------------------------------------

func imageDirective(lines as list of string, i as int, d as Directive) {
    if ($d.arg == "") {
        return directiveError($lines, $i, $d, "The \"image\" directive needs a URI.");
    }
    def img as doctree.Node init substImage(
        regex.replace('\s+', $d.arg, ""),
        $d.arg,
        imageOptions($d),
        $d.line);
    def node as doctree.Node init withChildren("paragraph", [$img]);
    # The classes belong on the image itself, so `finish` gets none to add.
    def rest as Directive init withoutOption($d, "class");
    return finish($rest, $node, IMAGE_OPTIONS);
}

# imageOptions is a directive's options as substImage takes them, with `align`
# folded into the class list.
func imageOptions(d as Directive) {
    def opts as map of string to string init {};
    for (def k in ["alt", "width", "height", "target"]) {
        if (maps.has($d.options, $k)) {
            $opts[$k] = $d.options[$k];
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
        $opts["class"] = strings.join($classes, " ");
    }
    return $opts;
}

# figureDirective is an image with an optional caption (the first paragraph of
# its content) and legend (the rest). Content that starts with an empty comment
# has a legend and no caption.
func figureDirective(lines as list of string, i as int, d as Directive, depth as int) {
    if ($d.arg == "") {
        return directiveError($lines, $i, $d, "The \"figure\" directive needs a URI.");
    }
    def img as doctree.Node init substImage(
        regex.replace('\s+', $d.arg, ""),
        $d.arg,
        imageOptions($d),
        $d.line);
    def fig as doctree.Node init withChildren("figure", [$img]);
    def body as list of string init trimmedContent($d.content);
    def noCaption as bool init len($body) > 0 and $body[0] == "..";
    def blocks as list of doctree.Node init contentBlocks($d, $depth);
    def k as int init 0;
    def lead as list of doctree.Node init [];
    while ($k < len($blocks) and (isPlaceholder($blocks[$k].kind) or $blocks[$k].kind == "rst_diag")) {
        $lead[] = $blocks[$k];
        $k = $k + 1;
    }
    for (def x in $lead) {
        $fig.children[] = $x;
    }
    if (not $noCaption and $k < len($blocks)) {
        if ($blocks[$k].kind != "paragraph") {
            return directiveError(
                $lines,
                $i,
                $d,
                "A figure's caption must be a paragraph or an empty comment.");
        }
        $fig.children[] = withChildren("caption", $blocks[$k].children);
        $k = $k + 1;
    }
    if ($k < len($blocks)) {
        $fig.children[] = withChildren("legend", $blocks[$k..]);
    }
    def classes as list of string init [];
    if (opt($d, "figclass") != "") {
        $classes[] = classNames(opt($d, "figclass"));
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
    def rest as Directive init withoutOption($d, "class");
    def allowed as list of string init ["figwidth", "figclass"];
    for (def o in IMAGE_OPTIONS) {
        $allowed[] = $o;
    }
    return finish($rest, $fig, $allowed);
}

# --- the table of contents ---------------------------------------------

# One heading as the table of contents sees it.
def struct TocEntry {
    level as int,
    id as string,
    text as string
};

def struct TocBuild {
    node as doctree.Node,
    next as int
};

func contentsDirective(lines as list of string, i as int, d as Directive, depth as int) {
    if ($depth > 0) {
        return directiveError(
            $lines,
            $i,
            $d,
            "The \"contents\" directive is only supported at the top level of a document.");
    }
    def p as doctree.Node init mk("rst_contents");
    $p.title = $d.arg;
    $p.level = $d.line;
    def depthOpt as string init opt($d, "depth");
    if ($depthOpt != "" and regex.matches('^[0-9]+$', $depthOpt)) {
        $p.attrs["depth"] = $depthOpt;
    }
    if (maps.has($d.options, "local")) {
        $p.attrs["local"] = "true";
    }
    def st as Step init finish($d, $p, ["depth", "local", "backlinks"]);
    return $st;
}

# contentsPass replaces each rst_contents placeholder in the top-level blocks
# with a `topic` (class `contents`) holding a nested list of links to the
# sections - all of them, or with `:local:` those inside the section the
# directive sits in. Runs after promote, so heading levels and ids are final.
func contentsPass(kids as list of doctree.Node) { # lint-disable: L205
    def out as list of doctree.Node init [];
    for (def k in 0..len($kids)) {
        if ($kids[$k].kind != "rst_contents") {
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
        $topic.title = $kids[$k].title;
        def local as bool init maps.has($kids[$k].attrs, "local");
        if ($topic.title == "" and not $local) {
            $topic.title = "Contents";
        }
        $topic.attrs = {"class": "contents"};
        if (maps.has($kids[$k].attrs, "class")) {
            $topic.attrs["class"] = "contents " + $kids[$k].attrs["class"];
        }
        if ($local) {
            $topic.attrs["class"] = $topic.attrs["class"] + " local";
        }
        $out[] = $topic;
    }
    return $out;
}

# tocEntries lists the section headings a contents placeholder at index at
# covers: every one but the document title, or with `local` the ones after it up
# to the end of the section it is in.
func tocEntries(kids as list of doctree.Node, at as int) {
    def local as bool init maps.has($kids[$at].attrs, "local");
    def from as int init 0;
    def stop as int init 0;
    if ($local) {
        $from = $at + 1;
        for (def k in 0..$at) {
            if ($kids[$k].kind == "heading" and not isTitle($kids[$k])) {
                $stop = $kids[$k].level;
            }
        }
    }
    def out as list of TocEntry init [];
    for (def k in $from..len($kids)) {
        def h as doctree.Node init $kids[$k];
        if ($h.kind != "heading" or isTitle($h)) {
            continue;
        }
        if ($local and $h.level <= $stop) {
            break;
        }
        def id as string init "";
        if (maps.has($h.attrs, "id")) {
            $id = $h.attrs["id"];
        }
        $out[] = TocEntry{level: $h.level, id: $id, text: doctree.text($h)};
    }
    return $out;
}

func isTitle(h as doctree.Node) {
    return maps.has($h.attrs, "class") and $h.attrs["class"] == "title";
}

# tocBuild builds the list for the entries from k on that sit at entries[k]'s
# level or deeper; a deeper run becomes a nested list under the item before it,
# kept only while `depth` stays within maxDepth (0 is unlimited).
func tocBuild(entries as list of TocEntry, k as int, depth as int, maxDepth as int) {
    def lvl as int init $entries[$k].level;
    def items as list of doctree.Node init [];
    def j as int init $k;
    while ($j < len($entries) and $entries[$j].level >= $lvl) {
        def e as TocEntry init $entries[$j];
        def link as doctree.Node init linkNode("#" + $e.id, [textOf($e.text)]);
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

# --- list tables and CSV tables ----------------------------------------

# tableExtras applies what list-table and csv-table share: the title as the
# table's caption, `align` as a class, `stub-columns` as a `stub` class on the
# leading cells of each row.
func tableExtras(t as doctree.Node, d as Directive) {
    def out as doctree.Node init $t;
    $out.title = $d.arg;
    if (opt($d, "align") != "") {
        $out = addClass($out, "align-" + classNames(opt($d, "align")));
    }
    def stubs as int init 0;
    if (regex.matches('^[0-9]+$', opt($d, "stub-columns"))) {
        $stubs = convert.toInt(opt($d, "stub-columns"));
    }
    if ($stubs == 0) {
        return $out;
    }
    for (def r in 0..len($out.children)) {
        for (def c in 0..minInt($stubs, len($out.children[$r].children))) {
            $out.children[$r].children[$c] = addClass($out.children[$r].children[$c], "stub");
        }
    }
    return $out;
}

# headerRowsOpt reads `:header-rows:`, which must be a number no larger than the
# table; -1 means it was not a number.
func headerRowsOpt(d as Directive, fallback as int) {
    def v as string init opt($d, "header-rows");
    if ($v == "") {
        return $fallback;
    }
    if (not regex.matches('^[0-9]+$', $v)) {
        return -1;
    }
    return convert.toInt($v);
}

# listTableDirective reads a uniform two-level bullet list: each item a row,
# each item of its sub-list a cell.
func listTableDirective(lines as list of string, i as int, d as Directive, depth as int) {
    def blocks as list of doctree.Node init [];
    def lead as list of doctree.Node init [];
    for (def b in contentBlocks($d, $depth)) {
        if ($b.kind == "rst_diag" or isPlaceholder($b.kind)) {
            $lead[] = $b;
        } else {
            $blocks[] = $b;
        }
    }
    def bad as string init "The \"list-table\" directive needs a uniform two-level bullet list.";
    if (len($blocks) != 1 or $blocks[0].kind != "list" or $blocks[0].ordered) {
        return directiveError($lines, $i, $d, $bad);
    }
    def rows as list of doctree.Node init [];
    def width as int init -1;
    for (def item in $blocks[0].children) {
        if (len($item.children) != 1 or $item.children[0].kind != "list" or
            $item.children[0].ordered) {
            return directiveError($lines, $i, $d, $bad);
        }
        def row as doctree.Node init mk("row");
        for (def sub in $item.children[0].children) {
            def cell as doctree.Node init withChildren("cell", $sub.children);
            $row.children[] = $cell;
        }
        if ($width >= 0 and len($row.children) != $width) {
            return directiveError(
                $lines,
                $i,
                $d,
                $bad + " Its rows have different numbers of cells.");
        }
        $width = len($row.children);
        $rows[] = $row;
    }
    def headerRows as int init headerRowsOpt($d, 0);
    if ($headerRows < 0 or $headerRows > len($rows)) {
        return directiveError(
            $lines,
            $i,
            $d,
            "The \"list-table\" directive's :header-rows: is not a row count of the table.");
    }
    def st as Step init finish($d, tableExtras(tableOf($rows, $headerRows), $d), TABLE_OPTIONS);
    def out as list of doctree.Node init $lead;
    for (def x in $st.nodes) {
        $out[] = $x;
    }
    return Step{nodes: $out, next: $st.next};
}

# csvDelim is the delimiter a `:delim:` value names: one character, or `tab` /
# `space` / `comma`.
func csvDelim(v as string) {
    match (strings.lower($v)) {
        when "", "comma" { return ","; }
        when "tab" { return "\t"; }
        when "space" { return " "; }
        else { return $v; }
    }
}

# csvRows parses CSV text into trimmed (unless keepspace) fields. It throws a
# `csv` error on malformed input.
func csvRows(text as string, d as Directive) {
    def dia as csv.Dialect init csv.dialect(csvDelim(opt($d, "delim")));
    if (opt($d, "quote") != "") {
        $dia.quote = opt($d, "quote");
    }
    def keep as bool init maps.has($d.options, "keepspace");
    # As docutils: short rows are padded later, and without keepspace a space
    # may follow the delimiter before a quoted field.
    $dia.width = -1;
    $dia.trim = not $keep;
    def out as list of list of string init [];
    for (def row in csv.parseDialect($text, $dia)) {
        def r as list of string init [];
        for (def f in $row) {
            if ($keep) {
                $r[] = $f;
            } else {
                $r[] = strings.trim($f);
            }
        }
        $out[] = $r;
    }
    return $out;
}

func safeCsvRows(text as string, d as Directive) {
    try {
        return csvRows($text, $d);
    } catch (e) {
        def none as list of list of string init [];
        return $none;
    }
}

# csvTableDirective reads its inline CSV content (and the `:header:` option's
# CSV) into a table; each field is parsed as body text. Short rows are padded.
func csvTableDirective(lines as list of string, i as int, d as Directive, depth as int) {
    if (opt($d, "file") != "" or opt($d, "url") != "") {
        return directiveError(
            $lines,
            $i,
            $d,
            "The \"csv-table\" directive's :file: and :url: options are not supported (rst reads no files).");
    }
    def delim as string init csvDelim(opt($d, "delim"));
    if (len($delim) != 1) {
        return directiveError(
            $lines,
            $i,
            $d,
            "The \"csv-table\" directive's :delim: must be one character.");
    }
    def body as list of string init trimmedContent($d.content);
    if (len($body) == 0) {
        return directiveError(
            $lines,
            $i,
            $d,
            "The \"csv-table\" directive is empty; content is required.");
    }
    def head as list of list of string init [];
    if (opt($d, "header") != "") {
        $head = safeCsvRows(opt($d, "header"), $d);
    }
    def data as list of list of string init safeCsvRows(strings.join($body, "\n"), $d);
    if (len($data) == 0) {
        return directiveError(
            $lines,
            $i,
            $d,
            "The \"csv-table\" directive's data is not valid CSV.");
    }
    def all as list of list of string init $head;
    for (def r in $data) {
        $all[] = $r;
    }
    def width as int init 0;
    for (def r in $all) {
        if (len($r) > $width) {
            $width = len($r);
        }
    }
    if (len($all) * $width > MAX_TABLE_CELLS) {
        rstFail("table too large (over {MAX_TABLE_CELLS} cells)");
    }
    def rows as list of doctree.Node init [];
    for (def r in $all) {
        def row as doctree.Node init mk("row");
        for (def c in 0..$width) {
            def text as string init "";
            if ($c < len($r)) {
                $text = $r[$c];
            }
            $row.children[] = cellNode(strings.split($text, "\n"), $d.contentLine, $depth);
        }
        $rows[] = $row;
    }
    def headerRows as int init headerRowsOpt($d, 0);
    if ($headerRows < 0 or $headerRows > len($data)) {
        return directiveError(
            $lines,
            $i,
            $d,
            "The \"csv-table\" directive's :header-rows: is not a row count of the table.");
    }
    def t as doctree.Node init tableOf($rows, len($head) + $headerRows);
    def allowed as list of string init [];
    for (def o in TABLE_OPTIONS) {
        $allowed[] = $o;
    }
    for (def o in CSV_OPTIONS) {
        $allowed[] = $o;
    }
    return finish($d, tableExtras($t, $d), $allowed);
}
