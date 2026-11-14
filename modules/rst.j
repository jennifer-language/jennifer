# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

# A hand-rolled reStructuredText parser: its block dispatcher and inline scanner
# legitimately run past the L201 statement-count limit. Every other lint check
# stays active.
# lint-disable-file: L201

/**
 * A reStructuredText (rST) reader. `parse` turns rST source into a `doctree`
 * document tree (`doctree.Node`), the same tree `markdown.parse` builds, so one
 * set of renderers serves both formats: `toHtml` / `toXhtml` / `toAnsi` here,
 * and `doctree.render` / `doctree.renderPdf` / `doctree.findAll` on the tree.
 *
 * Covered: section titles (levels from the order the underline / overline styles
 * first appear), transitions, paragraphs, `::` literal blocks (indented and
 * quoted), doctest blocks, block quotes with attributions, line blocks, bullet
 * lists, enumerated lists (arabic, alphabetic and roman, `1.` / `1)` / `(1)`,
 * auto-numbered `#.`), definition lists with classifiers, field lists, option
 * lists, grid and simple tables (with row and column spans) and comments.
 * A lone top-level title is promoted to the document title
 * (and a lone subtitle under it), and a field list at the top becomes the
 * bibliographic `docinfo`. Inline: `*emphasis*`, `**strong**`, ``` ``literal`` ```,
 * interpreted text with roles (`emphasis`, `strong`, `literal`, `code`, `math`,
 * `sub`, `sup`, `title-reference` - the default -, `abbreviation`, `pep`, `rfc`),
 * inline targets and backslash escapes, with rST's rules for where markup may
 * start and end.
 *
 * Directives: the admonitions (`note`, `warning`, ... and `admonition`), `code`
 * / `code-block` / `sourcecode`, `math`, `image`, `figure`, `topic`, `sidebar`,
 * `rubric`, `epigraph` / `highlights` / `pull-quote`, `container`, `compound`,
 * `class`, `raw` (HTML, escaped unless allowed), `contents`, `list-table` and
 * `csv-table` (inline data), each with `:class:` and `:name:`. `include` and
 * every other way of reading a file are refused.
 *
 * References: named (`name_`, `` `a phrase`_ ``), anonymous (`name__`), with an
 * embedded URI (`` `text <https://...>`_ ``) or alias (`` `text <name_>`_ ``),
 * to external, internal (`.. _name:`) and indirect targets and to section
 * titles; standalone URLs and email addresses; footnotes (`[1]_`, `[#]_`,
 * `[#label]_`, `[*]_`) and citations (`[CIT2002]_`), with links back; and
 * substitutions (`|name|`, `|name|_`) defined with `replace`, `unicode` or
 * `image`. A reference to an unknown target stays as its source text and is
 * reported by `check`.
 *
 * Parsing is lenient: malformed markup is kept as text and never throws.
 * `check` reports what was wrong as `Diagnostic`s; `Options{strict: true}` turns
 * the first warning or error into a catchable `"rst"` error. A document that
 * nests too deep or holds too many nodes is a kind `"limit"` error. There is no
 * file I/O: nothing in a document can read from the host.
 * @module rst
 * @example
 * import "rst.j" as rst;
 * io.printf("%s\n", rst.toHtml("Title\n=====\n\nSome *emphasis* and ``code``."));
 * io.printf("%s\n", rst.toAnsi("- one\n- two\n\n  continued"));
 */
import "./doctree.j" as doctree;
import "./csv.j" as csv;
use strings;
use regex;
use convert;
use lists;
use maps;

/**
 * Options for `parseWith` and `toHtmlWith`. The zero value is the default.
 * @field strict {bool} throw the first warning or error as an `"rst"` error
 *   instead of recovering from it
 * @field allowRawHtml {bool} pass raw HTML through when rendering (trusted
 *   input only); escaped by default
 * @field initialHeadingLevel {int} the HTML level of the top heading (1-6; 0
 *   means 1). Deeper sections follow from it, capped at 6
 * @field noTitlePromotion {bool} keep a lone top-level title as an ordinary
 *   section heading instead of promoting it to the document title
 */
export def struct Options {
    strict as bool,
    allowRawHtml as bool,
    initialHeadingLevel as int,
    noTitlePromotion as bool
};

/**
 * One problem found in a document.
 * @field severity {string} `"warning"` (recovered from, likely unintended) or
 *   `"error"` (markup that could not be read as written)
 * @field line {int} the 1-based source line it was found on
 * @field message {string} what went wrong
 */
export def struct Diagnostic {
    severity as string,
    line as int,
    message as string
};

# The nesting-depth cap, total-node budget and input size cap that turn a
# pathological document into a catchable "rst" error rather than unbounded
# recursion or allocation. The depth and node figures match the markdown module's.
def const MAX_DEPTH as int init 100;
def const MAX_NODES as int init 200000;
def const MAX_SOURCE as int init 16000000;

# The characters a section adornment, a transition or a quoted literal block may
# be drawn with: every printable 7-bit ASCII punctuation character.
def const ADORNMENT as string init '!"#$%&()*+,-./:;<=>?@[\]^_`{|}~' + "'";

# The bibliographic fields docinfo keeps, and the two that become topics.
def const DOCINFO_TOPICS as list of string init ["dedication", "abstract"];

def const BULLET_RE as string init '^([-*+\x{2022}\x{2023}\x{2043}])( +|$)';
def const ENUM_PARENS_RE as string init '^\(([0-9]+|[a-zA-Z]|[ivxlcdmIVXLCDM]+|#)\)( +|$)';
def const ENUM_RE as string init '^([0-9]+|[a-zA-Z]|[ivxlcdmIVXLCDM]+|#)([.)])( +|$)';
def const LINE_BLOCK_RE as string init '^\|( +|$)';
def const DOCTEST_RE as string init '^>>>( |$)';
def const ATTRIBUTION_RE as string init '^(---?|\x{2014}) *([^ -].*)$';
def const TARGET_RE as string init '^\.\. +_';
def const ANON_TARGET_RE as string init '^__( +|$)';
def const FOOTNOTE_DEF_RE as string init '^\.\. +\[([^\]\s]+)\]( +|$)';
def const SUBST_DEF_RE as string init '^\.\. +\|([^|\s](?:[^|]*[^|\s])?)\| +' +
    '([A-Za-z0-9]+(?:[-_+:.][A-Za-z0-9]+)*) ?::( +|$)';
def const OPTION_LINE_RE as string init '^:([^:\s][^:]*):(?: +(.*))?$';
def const DIRECTIVE_RE as string init '^\.\. +([A-Za-z0-9]+(?:[-_+:.][A-Za-z0-9]+)*) ?::( +|$)';
def const ROMAN_RE as string init '^m{0,4}(cm|cd|d?c{0,3})(xc|xl|l?x{0,3})(ix|iv|v?i{0,3})$';
# One option of an option list (`-a`, `--long`, `/V`, `+x`), with an optional
# argument after a space or `=`.
def const OPTION_RE as string init '(?:-[A-Za-z0-9]|--[A-Za-z0-9][A-Za-z0-9_-]*' +
    '|/[A-Za-z0-9]+|\+[A-Za-z0-9])' + '(?:[ =](?:<[^<>]+>|[A-Za-z][A-Za-z0-9_-]*))?';

# The result of one block-level step: the nodes it produced and the index of the
# first line it did not consume.
def struct Step {
    nodes as list of doctree.Node,
    next as int
};

# An indented block cut out of the line list, dedented. `next` is the first line
# after it (trailing blank lines excluded); `blankFinish` says whether a blank
# line or the end of input follows it, which a list or quote needs to end well.
def struct IndentBlock {
    lines as list of string,
    next as int,
    blankFinish as bool
};

# What sits at a line when it is read as a section title or a transition.
def struct TitleScan {
    found as bool,
    transition as bool,
    text as string,
    style as string,
    next as int,
    warn as string
};

# An enumerated-list marker: its format ("period" / "rparen" / "parens"), the
# enumerator text, and the column the item text starts at.
def struct EnumScan {
    ok as bool,
    format as string,
    text as string,
    end as int
};

# A field-list marker: the field name and the column the body starts at.
def struct FieldScan {
    ok as bool,
    name as string,
    end as int
};

# One line of a line block, before nesting: its indent and its text.
def struct LineEntry {
    indent as int,
    text as string
};

# A nested line block built from the entries starting at some index.
def struct LineBuild {
    node as doctree.Node,
    next as int
};

# A hyperlink target block read apart: anonymous or not, the name as written,
# and what follows the colon.
def struct TargetScan {
    ok as bool,
    anon as bool,
    name as string,
    rest as string
};

# A directive block split into its `:name: value` options and its content lines.
def struct DirectiveBody {
    options as map of string to string,
    content as list of string
};

# The parsed document and the problems found on the way.
def struct Result {
    doc as doctree.Node,
    diags as list of Diagnostic
};

include "./rst_inline.inc.j";
include "./rst_refs.inc.j";
include "./rst_tables.inc.j";
include "./rst_directives.inc.j";

func rstFail(msg as string) {
    throw Error{kind: "rst", message: $msg};
}

# rstLimit raises a reached budget (source size, nesting depth, node or table
# cell count): kind "limit", the generic kind for a cap.
func rstLimit(msg as string) {
    throw Error{kind: "limit", message: $msg};
}

# Nodes are built from a zero value, not a struct literal: a literal of another
# module's struct must name every field unless that module is installed in the
# system module dir, which the in-tree tests do not have.
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

# diagNode is a placeholder for a problem, kept in the tree where it was found
# (a declarations-only module has nowhere else to collect it) and lifted out by
# `run` before the tree is returned. The severity rides in `lang` and the line in
# `level`.
func diagNode(severity as string, line as int, message as string) {
    def n as doctree.Node init mk("rst_diag");
    $n.lang = $severity;
    $n.level = $line;
    $n.text = $message;
    return $n;
}

func oneStep(node as doctree.Node, next as int) {
    return Step{nodes: [$node], next: $next};
}

func noStep(next as int) {
    return Step{nodes: [], next: $next};
}

func indentOf(line as string) {
    return len($line) - len(strings.trimLeft($line));
}

# --- input normalisation -------------------------------------------

# normalize splits the source into lines the parser can index by column: the
# byte-order mark goes, CR LF and lone CR become LF, tabs expand to 8-column
# stops, form feeds and vertical tabs become spaces, and trailing whitespace is
# stripped (so a whitespace-only line is "").
func normalize(src as string) {
    if (len($src) > MAX_SOURCE) {
        rstLimit("document too large (over {MAX_SOURCE} characters)");
    }
    def s as string init $src;
    if (strings.startsWith($s, "\uFEFF")) {
        $s = $s[1..];
    }
    $s = strings.replace($s, "\r\n", "\n");
    $s = strings.replace($s, "\r", "\n");
    $s = strings.replace($s, "\u000C", " ");
    $s = strings.replace($s, "\u000B", " ");
    def out as list of string init [];
    for (def line in strings.split($s, "\n")) {
        if (strings.contains($line, "\t")) {
            $out[] = strings.trimRight(expandTabs($line));
        } else {
            $out[] = strings.trimRight($line);
        }
    }
    return $out;
}

func expandTabs(line as string) {
    def out as list of string init [];
    def col as int init 0;
    for (def c in strings.chars($line)) {
        if ($c == "\t") {
            def pad as int init 8 - ($col % 8);
            $out[] = strings.repeat(" ", $pad);
            $col = $col + $pad;
        } else {
            $out[] = $c;
            $col = $col + 1;
        }
    }
    return strings.join($out, "");
}

# --- line classification -------------------------------------------

# adornChar returns the punctuation character a line is drawn with when the
# whole line is one repeated character (a section adornment or a transition),
# else "".
func adornChar(line as string) {
    if (len($line) == 0) {
        return "";
    }
    def c as string init $line[0..1];
    if (not strings.contains(ADORNMENT, $c)) {
        return "";
    }
    if ($line != strings.repeat($c, len($line))) {
        return "";
    }
    return $c;
}

# titleAt reads the line at i as a section title (overline + title + underline,
# or title + underline) or a transition. A short underline still makes a title
# (with a warning) once it is 4 characters long; a shorter one is ordinary text.
func titleAt(lines as list of string, i as int) {
    def none as TitleScan init TitleScan{
        found: false,
        transition: false,
        text: "",
        style: "",
        next: $i,
        warn: ""
    };
    def n as int init len($lines);
    def line as string init $lines[$i];
    def c0 as string init adornChar($line);
    if ($c0 != "") {
        if ($i + 2 < $n and len($lines[$i + 1]) > 0 and adornChar($lines[$i + 2]) == $c0) {
            def text as string init strings.trim($lines[$i + 1]);
            def warn as string init "";
            if ($lines[$i + 2] != $line) {
                $warn = "Title overline & underline mismatch.";
            } elseif (len($line) < len($text)) {
                $warn = "Title overline too short.";
            }
            return TitleScan{
                found: true,
                transition: false,
                text: $text,
                style: "o" + $c0,
                next: $i + 3,
                warn: $warn
            };
        }
        if (len($line) >= 4 and ($i + 1 >= $n or len($lines[$i + 1]) == 0)) {
            return TitleScan{
                found: true,
                transition: true,
                text: "",
                style: "",
                next: $i + 1,
                warn: ""
            };
        }
        return $none;
    }
    if ($i + 1 >= $n) {
        return $none;
    }
    def c1 as string init adornChar($lines[$i + 1]);
    if ($c1 == "") {
        return $none;
    }
    def title as string init strings.trim($line);
    def warn as string init "";
    if (len($lines[$i + 1]) < len($title)) {
        if (len($lines[$i + 1]) < 4) {
            return $none;
        }
        $warn = "Title underline too short.";
    }
    return TitleScan{
        found: true,
        transition: false,
        text: $title,
        style: "u" + $c1,
        next: $i + 2,
        warn: $warn
    };
}

# indentedBlock cuts out the indented block starting at lines[start]: the lines
# that are blank or indented past column 0, up to the first non-blank line at
# column 0, without trailing blank lines, dedented by their smallest indent.
# With firstIndent >= 0 the first line is a list item's, field's or directive's
# first line: it is cut at that column instead and takes no part in the minimum.
func indentedBlock(lines as list of string, start as int, firstIndent as int) {
    def n as int init len($lines);
    def from as int init $start;
    if ($firstIndent >= 0) {
        $from = $start + 1;
    }
    def minInd as int init -1;
    def last as int init $from;
    def j as int init $from;
    while ($j < $n) {
        def line as string init $lines[$j];
        if (len($line) > 0) {
            def ind as int init indentOf($line);
            if ($ind == 0) {
                break;
            }
            if ($minInd < 0 or $ind < $minInd) {
                $minInd = $ind;
            }
            $last = $j + 1;
        }
        $j = $j + 1;
    }
    def out as list of string init [];
    if ($firstIndent >= 0) {
        def first as string init $lines[$start];
        if (len($first) > $firstIndent) {
            $out[] = $first[$firstIndent..];
        } else {
            $out[] = "";
        }
    }
    for (def k in $from..$last) {
        def line as string init $lines[$k];
        if (len($line) == 0) {
            $out[] = "";
        } else {
            $out[] = $line[$minInd..];
        }
    }
    def blankFinish as bool init $last >= $n or len($lines[$last]) == 0;
    return IndentBlock{lines: $out, next: $last, blankFinish: $blankFinish};
}

# --- the block parser ----------------------------------------------

# parseBlocks parses a run of lines that starts at column 0 (a whole document, or
# the dedented content of a list item, quote or field) into block nodes. `base` is
# the source line number of lines[0], for diagnostics. Section titles are only
# read at the top level (`top`); anywhere else they are reported and kept as text.
# A section heading carries its nesting depth in `level` here; `promote` maps
# depths to heading levels once the whole document is known.
func parseBlocks(lines as list of string, base as int, depth as int, top as bool) {
    if ($depth > MAX_DEPTH) {
        rstLimit("document nests too deep (over {MAX_DEPTH} levels)");
    }
    def out as list of doctree.Node init [];
    def styles as list of string init [];
    def cur as int init 0;
    def n as int init len($lines);
    def i as int init 0;
    while ($i < $n) {
        def line as string init $lines[$i];
        if (len($line) == 0) {
            $i = $i + 1;
            continue;
        }
        if (indentOf($line) > 0) {
            def q as Step init blockQuote($lines, $i, $base, $depth);
            for (def x in $q.nodes) {
                $out[] = $x;
            }
            $i = $q.next;
            continue;
        }
        def ts as TitleScan init titleAt($lines, $i);
        if ($ts.found and not $top) {
            $out[] = diagNode("error", $base + $i, "Unexpected section title or transition.");
            if (not $ts.transition) {
                $out[] = withChildren("paragraph", inlineNodes($ts.text, $base + $i));
            }
            $i = $ts.next;
            continue;
        }
        if ($ts.found and $ts.transition) {
            $out[] = mk("thematic_break");
            $i = $ts.next;
            continue;
        }
        if ($ts.found) {
            if ($ts.warn != "") {
                $out[] = diagNode("warning", $base + $i, $ts.warn);
            }
            def idx as int init lists.indexOf($styles, $ts.style);
            def lvl as int init $idx + 1;
            if ($idx < 0) {
                $styles[] = $ts.style;
                $lvl = len($styles);
            }
            if ($lvl > $cur + 1) {
                $out[] = diagNode("error", $base + $i, "Title level inconsistent.");
            }
            $cur = $lvl;
            def h as doctree.Node init withChildren("heading", inlineNodes($ts.text, $base + $i));
            $h.level = $lvl;
            $out[] = $h;
            $i = $ts.next;
            continue;
        }
        def st as Step init bodyBlock($lines, $i, $base, $depth);
        for (def x in $st.nodes) {
            $out[] = $x;
        }
        $i = $st.next;
    }
    if (hasKind($out, "rst_class")) {
        $out = applyClasses($out);
    }
    return $out;
}

# bodyBlock parses the one body element that starts at the column-0 line i: an
# explicit markup block, a list, a line block, a doctest block, a definition list
# or a paragraph.
func bodyBlock(lines as list of string, i as int, base as int, depth as int) {
    def line as string init $lines[$i];
    if ($line == ".." or strings.startsWith($line, ".. ")) {
        return explicitBlock($lines, $i, $base, $depth);
    }
    if (regex.matches(ANON_TARGET_RE, $line)) {
        def ablk as IndentBlock init indentedBlock($lines, $i, 3);
        return oneStep(targetNode("__: " + strings.join($ablk.lines, " "), $base + $i), $ablk.next);
    }
    if (regex.matches(GRID_TOP_RE, $line)) {
        return gridTable($lines, $i, $base, $depth);
    }
    if (regex.matches(SIMPLE_TOP_RE, $line)) {
        return simpleTable($lines, $i, $base, $depth);
    }
    if (regex.matches(BULLET_RE, $line)) {
        return bulletList($lines, $i, $base, $depth);
    }
    def es as EnumScan init scanEnum($line);
    if ($es.ok and enumStartsList($lines, $i, $es)) {
        return enumList($lines, $i, $base, $depth);
    }
    if (scanField($line).ok) {
        return fieldList($lines, $i, $base, $depth);
    }
    if (optionStart($lines, $i)) {
        return optionList($lines, $i, $base, $depth);
    }
    if (regex.matches(LINE_BLOCK_RE, $line)) {
        return lineBlock($lines, $i, $base, $depth);
    }
    if (regex.matches(DOCTEST_RE, $line)) {
        return doctestBlock($lines, $i);
    }
    return textBlock($lines, $i, $base, $depth);
}

# --- paragraphs, literal blocks, definition lists -------------------

# textBlock reads a run of text lines: a definition list when the first line is a
# one-line term with an indented definition right under it, else a paragraph. A
# paragraph ending in `::` introduces the literal block that follows it.
func textBlock(lines as list of string, i as int, base as int, depth as int) {
    def n as int init len($lines);
    if ($i + 1 < $n and len($lines[$i + 1]) > 0 and indentOf($lines[$i + 1]) > 0) {
        return definitionList($lines, $i, $base, $depth);
    }
    def j as int init $i;
    while ($j < $n and len($lines[$j]) > 0 and indentOf($lines[$j]) == 0) {
        $j = $j + 1;
    }
    def text as string init strings.join($lines[$i..$j], " ");
    def literal as bool init strings.endsWith($text, "::");
    if ($literal) {
        $text = literalMarkerText($text);
    }
    def out as list of doctree.Node init [];
    if ($text != "") {
        $out[] = withChildren("paragraph", inlineNodes($text, $base + $i));
    }
    if (not $literal) {
        if ($j < $n and len($lines[$j]) > 0) {
            $out[] = diagNode("error", $base + $j, "Unexpected indentation.");
        }
        return Step{nodes: $out, next: $j};
    }
    def lit as Step init literalBlock($lines, $j, $base);
    for (def x in $lit.nodes) {
        $out[] = $x;
    }
    return Step{nodes: $out, next: $lit.next};
}

# literalMarkerText applies the `::` rules to a paragraph's text: a lone `::`
# leaves no paragraph, `text ::` drops the marker, and `text::` keeps one colon.
func literalMarkerText(text as string) {
    if ($text == "::") {
        return "";
    }
    def body as string init $text[..len($text) - 2];
    if (strings.endsWith($body, " ")) {
        return strings.trimRight($body);
    }
    return $text[..len($text) - 1];
}

# literalBlock reads the literal block a `::` paragraph introduces, from line j
# (just past the paragraph): an indented block, kept verbatim but dedented, or a
# quoted block of unindented lines that all start with the same punctuation.
func literalBlock(lines as list of string, j as int, base as int) {
    def n as int init len($lines);
    def k as int init $j;
    while ($k < $n and len($lines[$k]) == 0) {
        $k = $k + 1;
    }
    if ($k < $n and indentOf($lines[$k]) > 0) {
        def blk as IndentBlock init indentedBlock($lines, $k, -1);
        def code as doctree.Node init mk("code");
        $code.text = strings.join($blk.lines, "\n");
        $code.attrs = {"class": "literal-block"};
        return oneStep($code, $blk.next);
    }
    if ($k < $n and $k > $j and strings.contains(ADORNMENT, $lines[$k][0..1])) {
        return quotedLiteral($lines, $k, $base);
    }
    return oneStep(diagNode("warning", $base + $k, "Literal block expected; none found."), $j);
}

func quotedLiteral(lines as list of string, k as int, base as int) {
    def n as int init len($lines);
    def q as string init $lines[$k][0..1];
    def parts as list of string init [];
    def e as int init $k;
    while ($e < $n and len($lines[$e]) > 0 and strings.startsWith($lines[$e], $q)) {
        $parts[] = $lines[$e];
        $e = $e + 1;
    }
    def code as doctree.Node init mk("code");
    $code.text = strings.join($parts, "\n");
    $code.attrs = {"class": "literal-block"};
    if ($e < $n and len($lines[$e]) > 0) {
        return Step{
            nodes: [$code, diagNode("error", $base + $e, "Inconsistent literal block quoting.")],
            next: $e
        };
    }
    return oneStep($code, $e);
}

# definitionList reads definition list items: a one-line term (with optional
# ` : classifier`s) and its indented definition. Items may be separated by blank
# lines; the list ends at the first line that is not another term.
func definitionList(lines as list of string, i as int, base as int, depth as int) {
    def kids as list of doctree.Node init [];
    def j as int init $i;
    def next as int init $i;
    while (isDefinitionStart($lines, $j)) {
        $kids[] = definitionTerm($lines[$j], $base + $j);
        def blk as IndentBlock init indentedBlock($lines, $j + 1, -1);
        $kids[] = withChildren(
            "def_desc",
            parseBlocks($blk.lines, $base + $j + 1, $depth + 1, false));
        $next = $blk.next;
        def k as int init skipBlank($lines, $next);
        if (isDefinitionStart($lines, $k)) {
            $j = $k;
            continue;
        }
        if (not $blk.blankFinish) {
            $kids[] = diagNode(
                "warning",
                $base + $next,
                "Definition list ends without a blank line; unexpected unindent.");
        }
        break;
    }
    $kids = compactDescs($kids);
    def dl as doctree.Node init withChildren("definition_list", $kids);
    return oneStep($dl, $next);
}

func isDefinitionStart(lines as list of string, j as int) {
    def n as int init len($lines);
    if ($j + 1 >= $n or len($lines[$j]) == 0 or indentOf($lines[$j]) > 0) {
        return false;
    }
    return len($lines[$j + 1]) > 0 and indentOf($lines[$j + 1]) > 0;
}

func skipBlank(lines as list of string, j as int) {
    def k as int init $j;
    while ($k < len($lines) and len($lines[$k]) == 0) {
        $k = $k + 1;
    }
    return $k;
}

# definitionTerm builds a `def_term`: the term, then each classifier after a
# ` : ` as a delimiter span and a classifier span (docutils' HTML shape).
func definitionTerm(line as string, lineNo as int) {
    def parts as list of string init regex.split(' +: +', $line);
    def kids as list of doctree.Node init inlineNodes($parts[0], $lineNo);
    for (def cls in $parts[1..]) {
        def delim as doctree.Node init withChildren("span", [textOf(":")]);
        $delim.attrs = {"class": "classifier-delimiter"};
        $kids[] = $delim;
        $kids[] = textOf(" ");
        def span as doctree.Node init withChildren("span", inlineNodes($cls, $lineNo));
        $span.attrs = {"class": "classifier"};
        $kids[] = $span;
    }
    return withChildren("def_term", $kids);
}

# --- block quotes ----------------------------------------------------

# blockQuote reads the indented block at line i as one or more block quotes: an
# attribution (`-- Author`) closes a quote, and anything after it starts the next.
func blockQuote(lines as list of string, i as int, base as int, depth as int) {
    def blk as IndentBlock init indentedBlock($lines, $i, -1);
    def out as list of doctree.Node init [];
    def q as list of string init $blk.lines;
    def qbase as int init $base + $i;
    while (len($q) > 0) {
        def a as int init attributionAt($q);
        if ($a < 0) {
            $out[] = withChildren("quote", parseBlocks($q, $qbase, $depth + 1, false));
            break;
        }
        def e as int init $a + 1;
        while ($e < len($q) and len($q[$e]) > 0) {
            $e = $e + 1;
        }
        def kids as list of doctree.Node init parseBlocks($q[..$a], $qbase, $depth + 1, false);
        $kids[] = attribution($q[$a..$e], $qbase + $a);
        $out[] = withChildren("quote", $kids);
        $q = $q[$e..];
        $qbase = $qbase + $e;
        def k as int init skipBlank($q, 0);
        $q = $q[$k..];
        $qbase = $qbase + $k;
    }
    if (not $blk.blankFinish) {
        $out[] = diagNode(
            "warning",
            $base + $blk.next,
            "Block quote ends without a blank line; unexpected unindent.");
    }
    return Step{nodes: $out, next: $blk.next};
}

# attributionAt finds the first attribution line in a dedented quote: a column-0
# line starting with `--`, `---` or an em dash, right after a blank line. -1 if
# there is none.
func attributionAt(q as list of string) {
    for (def k in 1..len($q)) {
        if (len($q[$k - 1]) == 0 and regex.matches(ATTRIBUTION_RE, $q[$k])) {
            return $k;
        }
    }
    return -1;
}

func attribution(alines as list of string, lineNo as int) {
    def parts as list of string init [];
    for (def l in $alines) {
        $parts[] = strings.trim($l);
    }
    def m as regex.Match init regex.find(ATTRIBUTION_RE, strings.join($parts, " "));
    def kids as list of doctree.Node init [textOf("\u2014")];
    for (def x in inlineNodes($m.groups[1], $lineNo)) {
        $kids[] = $x;
    }
    def p as doctree.Node init withChildren("paragraph", $kids);
    $p.attrs = {"class": "attribution"};
    return $p;
}

# --- bullet and enumerated lists --------------------------------------

func bulletList(lines as list of string, i as int, base as int, depth as int) {
    def n as int init len($lines);
    def bullet as string init $lines[$i][0..1];
    def items as list of doctree.Node init [];
    def j as int init $i;
    def next as int init $i;
    def diags as list of doctree.Node init [];
    while ($j < $n) {
        def m as regex.Match init regex.find(BULLET_RE, $lines[$j]);
        if ($m.start != 0 or $m.groups[0] != $bullet) {
            break;
        }
        def blk as IndentBlock init indentedBlock($lines, $j, $m.end);
        $items[] = withChildren("item", parseBlocks($blk.lines, $base + $j, $depth + 1, false));
        $next = $blk.next;
        def k as int init skipBlank($lines, $next);
        if ($k < $n and strings.startsWith($lines[$k], $bullet) and
            regex.matches(BULLET_RE, $lines[$k])) {
            $j = $k;
            continue;
        }
        if (not $blk.blankFinish) {
            $diags[] = diagNode(
                "warning",
                $base + $next,
                "Bullet list ends without a blank line; unexpected unindent.");
        }
        break;
    }
    $items = compactItems($items);
    def lst as doctree.Node init withChildren("list", $items);
    def out as list of doctree.Node init [$lst];
    for (def d in $diags) {
        $out[] = $d;
    }
    return Step{nodes: $out, next: $next};
}

# scanEnum reads an enumerator (`1.`, `a)`, `(iv)`, `#.`) at the start of a line.
func scanEnum(line as string) {
    def m as regex.Match init regex.find(ENUM_PARENS_RE, $line);
    if ($m.start == 0) {
        return EnumScan{ok: true, format: "parens", text: $m.groups[0], end: $m.end};
    }
    $m = regex.find(ENUM_RE, $line);
    if ($m.start == 0) {
        def format as string init "period";
        if ($m.groups[1] == ")") {
            $format = "rparen";
        }
        return EnumScan{ok: true, format: $format, text: $m.groups[0], end: $m.end};
    }
    return EnumScan{ok: false, format: "", text: "", end: 0};
}

# enumSequence names the numbering an enumerator belongs to: "arabic",
# "loweralpha", "upperalpha", "lowerroman", "upperroman", or "#" (auto). A list
# already under way keeps its own sequence where the text fits it; otherwise a
# lone `i` / `I` is roman and any other single letter alphabetic (docutils' rule).
func enumSequence(text as string, expected as string) {
    if ($text == "#") {
        return "#";
    }
    if ($expected != "" and inSequence($expected, $text)) {
        return $expected;
    }
    if ($text == "i") {
        return "lowerroman";
    }
    if ($text == "I") {
        return "upperroman";
    }
    for (def seq in ["arabic", "loweralpha", "upperalpha", "lowerroman", "upperroman"]) {
        if (inSequence($seq, $text)) {
            return $seq;
        }
    }
    return "";
}

func inSequence(seq as string, text as string) {
    match ($seq) {
        when "arabic" { return regex.matches('^[0-9]+$', $text); }
        when "loweralpha" { return regex.matches('^[a-z]$', $text); }
        when "upperalpha" { return regex.matches('^[A-Z]$', $text); }
        when "lowerroman" { return regex.matches('^[ivxlcdm]+$', $text); }
        when "upperroman" { return regex.matches('^[IVXLCDM]+$', $text); }
    }
    return false;
}

# enumOrdinal is the number an enumerator stands for, or -1 when it is not a
# valid one (a malformed roman numeral, a number too large to be sensible).
func enumOrdinal(seq as string, text as string) {
    match ($seq) {
        when "arabic" { if (len($text) > 9) {
            return -1;
        }
        return convert.toInt($text); }
        when "loweralpha" { return convert.toCodepoint($text) - 96; }
        when "upperalpha" { return convert.toCodepoint($text) - 64; }
        when "lowerroman", "upperroman" { return romanValue(strings.lower($text)); }
    }
    return -1;
}

func romanValue(s as string) {
    if (not regex.matches(ROMAN_RE, $s)) {
        return -1;
    }
    def values as map of string to int init {
        "i": 1,
        "v": 5,
        "x": 10,
        "l": 50,
        "c": 100,
        "d": 500,
        "m": 1000
    };
    def cs as list of string init strings.chars($s);
    def total as int init 0;
    for (def k in 0..len($cs)) {
        def v as int init $values[$cs[$k]];
        if ($k + 1 < len($cs) and $values[$cs[$k + 1]] > $v) {
            $total = $total - $v;
        } else {
            $total = $total + $v;
        }
    }
    return $total;
}

func romanText(n as int) {
    def vals as list of int init [1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1];
    def syms as list of string init [
        "m",
        "cm",
        "d",
        "cd",
        "c",
        "xc",
        "l",
        "xl",
        "x",
        "ix",
        "v",
        "iv",
        "i"
    ];
    def out as list of string init [];
    def rest as int init $n;
    for (def k in 0..len($vals)) {
        while ($rest >= $vals[$k]) {
            $out[] = $syms[$k];
            $rest = $rest - $vals[$k];
        }
    }
    return strings.join($out, "");
}

# enumText writes the enumerator for an ordinal in a sequence and format ("" when
# the ordinal has no such enumerator, like the 27th letter).
func enumText(ordinal as int, seq as string, format as string) {
    def t as string init "";
    match ($seq) {
        when "#" { $t = "#"; }
        when "arabic" { $t = "{$ordinal}"; }
        when "loweralpha", "upperalpha" {
            if ($ordinal < 1 or $ordinal > 26) {
                return "";
            }
            $t = convert.fromCodepoint($ordinal + 96);
            if ($seq == "upperalpha") {
                $t = strings.upper($t);
            }
        }
        when "lowerroman" { $t = romanText($ordinal); }
        when "upperroman" { $t = strings.upper(romanText($ordinal)); }
    }
    match ($format) {
        when "parens" { return "(" + $t + ")"; }
        when "rparen" { return $t + ")"; }
    }
    return $t + ".";
}

# enumStartsList decides whether an enumerator at line i really opens a list
# item, the way docutils does: the next line must be blank, indented, the end of
# input, or the next enumerator - so a sentence that happens to start with "A."
# stays a paragraph.
func enumStartsList(lines as list of string, i as int, es as EnumScan) {
    def seq as string init enumSequence($es.text, "");
    def ord as int init 1;
    if ($seq != "#") {
        $ord = enumOrdinal($seq, $es.text);
    }
    if ($seq == "" or $ord < 0) {
        return false;
    }
    if ($i + 1 >= len($lines)) {
        return true;
    }
    def next as string init $lines[$i + 1];
    if (len($next) == 0 or indentOf($next) > 0) {
        return true;
    }
    def seqNext as string init $seq;
    if ($seq == "#") {
        $seqNext = "arabic";
    }
    def want as string init enumText($ord + 1, $seqNext, $es.format);
    if ($want != "" and strings.startsWith($next, $want)) {
        return true;
    }
    return strings.startsWith($next, enumText(0, "#", $es.format));
}

func enumList(lines as list of string, i as int, base as int, depth as int) {
    def n as int init len($lines);
    def first as EnumScan init scanEnum($lines[$i]);
    def format as string init $first.format;
    def seq as string init enumSequence($first.text, "");
    def auto as bool init $seq == "#";
    def ord as int init 1;
    if ($auto) {
        $seq = "arabic";
    } else {
        $ord = enumOrdinal($seq, $first.text);
    }
    def start as int init $ord;
    def items as list of doctree.Node init [];
    def diags as list of doctree.Node init [];
    def j as int init $i;
    def next as int init $i;
    while (true) {
        def es as EnumScan init scanEnum($lines[$j]);
        def blk as IndentBlock init indentedBlock($lines, $j, $es.end);
        $items[] = withChildren("item", parseBlocks($blk.lines, $base + $j, $depth + 1, false));
        $next = $blk.next;
        def k as int init skipBlank($lines, $next);
        if ($k < $n and enumContinues($lines, $k, $format, $seq, $auto, $ord)) {
            $ord = $ord + 1;
            $j = $k;
            continue;
        }
        if (not $blk.blankFinish) {
            $diags[] = diagNode(
                "warning",
                $base + $next,
                "Enumerated list ends without a blank line; unexpected unindent.");
        }
        break;
    }
    $items = compactItems($items);
    def lst as doctree.Node init withChildren("list", $items);
    $lst.ordered = true;
    def attrs as map of string to string init {};
    if ($start != 1) {
        $attrs["start"] = "{$start}";
    }
    def et as string init enumTypeAttr($seq);
    if ($et != "1") {
        $attrs["enumtype"] = $et;
    }
    $lst.attrs = $attrs;
    def out as list of doctree.Node init [$lst];
    for (def d in $diags) {
        $out[] = $d;
    }
    return Step{nodes: $out, next: $next};
}

# enumContinues says whether line k holds the next item of an enumerated list:
# the same format, the same sequence (or `#`, in an auto list only `#`), the next
# ordinal, and a valid item by enumStartsList's test.
func enumContinues(
    lines as list of string,
    k as int,
    format as string,
    seq as string,
    auto as bool,
    ord as int) {
    def es as EnumScan init scanEnum($lines[$k]);
    if (not $es.ok or $es.format != $format) {
        return false;
    }
    def s as string init enumSequence($es.text, $seq);
    if ($s == "#") {
        return enumStartsList($lines, $k, $es);
    }
    if ($auto or $s != $seq or enumOrdinal($s, $es.text) != $ord + 1) {
        return false;
    }
    return enumStartsList($lines, $k, $es);
}

func enumTypeAttr(seq as string) {
    match ($seq) {
        when "loweralpha" { return "a"; }
        when "upperalpha" { return "A"; }
        when "lowerroman" { return "i"; }
        when "upperroman" { return "I"; }
    }
    return "1";
}

# compactItems drops the paragraph wrapper from the items of a simple list (every
# item one paragraph, optionally followed by a compact nested list), so the HTML
# reads `<li>text</li>` the way docutils writes a simple list. It edits the list
# in place: call it as `$items = compactItems($items);`.
func compactItems(items as list of doctree.Node) { # lint-disable: L205
    for (def item in $items) {
        if (not isSimpleItem($item)) {
            return $items;
        }
    }
    for (def k in 0..len($items)) {
        if (len($items[$k].children) > 0) {
            def kids as list of doctree.Node init $items[$k].children[0].children;
            if (len($items[$k].children) == 2) {
                $kids[] = $items[$k].children[1];
            }
            $items[$k].children = $kids;
        }
    }
    return $items;
}

func isSimpleItem(item as doctree.Node) {
    def n as int init len($item.children);
    if ($n == 0) {
        return true;
    }
    if ($item.children[0].kind != "paragraph") {
        return false;
    }
    if ($n == 1) {
        return true;
    }
    return $n == 2 and $item.children[1].kind == "list" and isCompactList($item.children[1]);
}

# isCompactList is true for a list compactItems already unwrapped: no item holds
# a paragraph.
func isCompactList(lst as doctree.Node) {
    for (def item in $lst.children) {
        for (def c in $item.children) {
            if ($c.kind == "paragraph") {
                return false;
            }
        }
    }
    return true;
}

# compactDescs is compactItems for the children of a definition / field / option
# list: when every definition is a single paragraph, the paragraphs are
# unwrapped. Call it as `$kids = compactDescs($kids);`.
func compactDescs(kids as list of doctree.Node) { # lint-disable: L205
    for (def c in $kids) {
        if ($c.kind == "def_desc" and
            not (len($c.children) == 1 and $c.children[0].kind == "paragraph")) {
            return $kids;
        }
    }
    for (def k in 0..len($kids)) {
        if ($kids[$k].kind == "def_desc") {
            $kids[$k].children = $kids[$k].children[0].children;
        }
    }
    return $kids;
}

# --- field lists and option lists -------------------------------------

# scanField reads a field marker `:name:` at the start of a line: the name may
# hold escaped colons and colons not followed by a space or a backtick, and must
# not start with a space or end with one.
func scanField(line as string) {
    if (len($line) < 3 or $line[0..1] != ":") {
        return FieldScan{ok: false, name: "", end: 0};
    }
    def cs as list of string init strings.chars($line);
    if ($cs[1] == " " or $cs[1] == ":") {
        return FieldScan{ok: false, name: "", end: 0};
    }
    def n as int init len($cs);
    def k as int init 1;
    while ($k < $n) {
        def c as string init $cs[$k];
        if ($c == "\\") {
            $k = $k + 2;
            continue;
        }
        if ($c == ":" and $k + 1 < $n and $cs[$k + 1] == "`") {
            # `:role:`text`` opens interpreted text, not a field.
            return FieldScan{ok: false, name: "", end: 0};
        }
        if ($c == ":" and $cs[$k - 1] != " " and ($k + 1 == $n or $cs[$k + 1] == " ")) {
            def end as int init $k + 1;
            while ($end < $n and $cs[$end] == " ") {
                $end = $end + 1;
            }
            return FieldScan{ok: true, name: strings.join($cs[1..$k], ""), end: $end};
        }
        $k = $k + 1;
    }
    return FieldScan{ok: false, name: "", end: 0};
}

func fieldList(lines as list of string, i as int, base as int, depth as int) {
    def n as int init len($lines);
    def kids as list of doctree.Node init [];
    def j as int init $i;
    def next as int init $i;
    while (true) {
        def fs as FieldScan init scanField($lines[$j]);
        $kids[] = withChildren("def_term", inlineNodes($fs.name, $base + $j));
        def blk as IndentBlock init indentedBlock($lines, $j, $fs.end);
        $kids[] = withChildren("def_desc", parseBlocks($blk.lines, $base + $j, $depth + 1, false));
        $next = $blk.next;
        def k as int init skipBlank($lines, $next);
        if ($k < $n and scanField($lines[$k]).ok) {
            $j = $k;
            continue;
        }
        if (not $blk.blankFinish) {
            $kids[] = diagNode(
                "warning",
                $base + $next,
                "Field list ends without a blank line; unexpected unindent.");
        }
        break;
    }
    $kids = compactDescs($kids);
    def dl as doctree.Node init withChildren("definition_list", $kids);
    $dl.attrs = {"class": "field-list"};
    return oneStep($dl, $next);
}

# optionMatch matches an option-list item line: one or more options joined by
# ", ", then the description after two or more spaces, or nothing (the
# description is then the indented block below).
func optionMatch(line as string) {
    return regex.find('^(' + OPTION_RE + '(?:, ' + OPTION_RE + ')*)(?:  +(.*))?$', $line);
}

func optionStart(lines as list of string, i as int) {
    def first as string init $lines[$i][0..1];
    if (not ($first == "-" or $first == "/" or $first == "+")) {
        return false;
    }
    def m as regex.Match init optionMatch($lines[$i]);
    if ($m.start != 0) {
        return false;
    }
    if ($m.groups[1] != "") {
        return true;
    }
    return $i + 1 < len($lines) and len($lines[$i + 1]) > 0 and indentOf($lines[$i + 1]) > 0;
}

func optionList(lines as list of string, i as int, base as int, depth as int) {
    def n as int init len($lines);
    def kids as list of doctree.Node init [];
    def j as int init $i;
    def next as int init $i;
    while (true) {
        def m as regex.Match init optionMatch($lines[$j]);
        $kids[] = optionTerm($m.groups[0]);
        def cut as int init len($lines[$j]) - len($m.groups[1]);
        def blk as IndentBlock init indentedBlock($lines, $j, $cut);
        $kids[] = withChildren("def_desc", parseBlocks($blk.lines, $base + $j, $depth + 1, false));
        $next = $blk.next;
        def k as int init skipBlank($lines, $next);
        if ($k < $n and optionStart($lines, $k)) {
            $j = $k;
            continue;
        }
        if (not $blk.blankFinish) {
            $kids[] = diagNode(
                "warning",
                $base + $next,
                "Option list ends without a blank line; unexpected unindent.");
        }
        break;
    }
    $kids = compactDescs($kids);
    def dl as doctree.Node init withChildren("definition_list", $kids);
    $dl.attrs = {"class": "option-list"};
    return oneStep($dl, $next);
}

# optionTerm is the `def_term` of an option-list item: each option as a code span
# with class "option", separated by ", ".
func optionTerm(group as string) {
    def kids as list of doctree.Node init [];
    for (def opt in strings.split($group, ", ")) {
        if (len($kids) > 0) {
            $kids[] = textOf(", ");
        }
        def c as doctree.Node init mk("codespan");
        $c.text = $opt;
        $c.attrs = {"class": "option"};
        $kids[] = $c;
    }
    return withChildren("def_term", $kids);
}

# --- line blocks and doctest blocks -------------------------------------

# lineBlock reads `| ` lines (and their indented continuation lines) into a
# `line_block`; a line indented further than its neighbours nests a block.
func lineBlock(lines as list of string, i as int, base as int, depth as int) {
    def n as int init len($lines);
    def entries as list of LineEntry init [];
    def j as int init $i;
    def prevIndent as int init 0;
    while ($j < $n) {
        def line as string init $lines[$j];
        if (regex.matches(LINE_BLOCK_RE, $line)) {
            def rest as string init $line[1..];
            if (len($rest) == 0) {
                $entries[] = LineEntry{indent: $prevIndent, text: ""};
            } else {
                def sp as int init indentOf($rest);
                $prevIndent = $sp - 1;
                $entries[] = LineEntry{indent: $prevIndent, text: $rest[$sp..]};
            }
        } elseif (len($line) > 0 and indentOf($line) > 0 and len($entries) > 0) {
            def last as int init len($entries) - 1;
            $entries[$last].text = $entries[$last].text + " " + strings.trim($line);
        } else {
            break;
        }
        $j = $j + 1;
    }
    def minIndent as int init $entries[0].indent;
    for (def e in $entries) {
        if ($e.indent < $minIndent) {
            $minIndent = $e.indent;
        }
    }
    def lb as LineBuild init buildLines($entries, 0, $minIndent, $base + $i, $depth);
    if ($j < $n and len($lines[$j]) > 0) {
        return Step{
            nodes: [
                $lb.node,
                diagNode("warning", $base + $j, "Line block ends without a blank line.")
            ],
            next: $j
        };
    }
    return oneStep($lb.node, $j);
}

func buildLines(
    entries as list of LineEntry,
    start as int,
    indent as int,
    lineNo as int,
    depth as int) {
    if ($depth > MAX_DEPTH) {
        rstLimit("document nests too deep (over {MAX_DEPTH} levels)");
    }
    def kids as list of doctree.Node init [];
    def k as int init $start;
    while ($k < len($entries) and $entries[$k].indent >= $indent) {
        def e as LineEntry init $entries[$k];
        if ($e.indent > $indent) {
            def inner as LineBuild init buildLines($entries, $k, $e.indent, $lineNo, $depth + 1);
            $kids[] = $inner.node;
            $k = $inner.next;
            continue;
        }
        $kids[] = withChildren("line", inlineNodes($e.text, $lineNo + $k));
        $k = $k + 1;
    }
    return LineBuild{node: withChildren("line_block", $kids), next: $k};
}

func doctestBlock(lines as list of string, i as int) {
    def j as int init $i;
    while ($j < len($lines) and len($lines[$j]) > 0) {
        $j = $j + 1;
    }
    def code as doctree.Node init mk("code");
    $code.text = strings.join($lines[$i..$j], "\n");
    $code.lang = "pycon";
    $code.attrs = {"class": "doctest-block"};
    return oneStep($code, $j);
}

# --- explicit markup ------------------------------------------------------

# explicitBlock reads a `..` block: a hyperlink target, a footnote, a citation,
# a substitution definition, a directive, or else a comment (dropped).
func explicitBlock(lines as list of string, i as int, base as int, depth as int) {
    def line as string init $lines[$i];
    if ($line == "..") {
        if ($i + 1 >= len($lines) or len($lines[$i + 1]) == 0) {
            return noStep($i + 1);
        }
        return noStep(indentedBlock($lines, $i, 2).next);
    }
    def blk as IndentBlock init indentedBlock($lines, $i, 3);
    if (regex.matches(TARGET_RE, $line)) {
        def t as doctree.Node init targetNode(strings.join($blk.lines, " "), $base + $i);
        if ($t.kind == "rst_target") {
            return oneStep($t, $blk.next);
        }
        return noStep($blk.next);
    }
    def fm as regex.Match init regex.find(FOOTNOTE_DEF_RE, $line);
    if ($fm.start == 0 and regex.matches(NOTE_LABEL_RE, $fm.groups[0])) {
        return noteBlock($lines, $i, $base, $depth, $fm);
    }
    def sm as regex.Match init regex.find(SUBST_DEF_RE, $line);
    if ($sm.start == 0) {
        return substBlock($lines, $i, $base, $sm, $blk);
    }
    def m as regex.Match init regex.find(DIRECTIVE_RE, $line);
    if ($m.start == 0) {
        return directiveBlock($lines, $i, $blk, $m, $base, $depth);
    }
    return noStep($blk.next);
}

# parseTarget splits a target block's text (`_name: uri`, `` _`a: b`: uri ``,
# `__: uri`) into its name and the rest. A name holds escaped colons, or any
# colon when it is quoted in backticks.
func parseTarget(s as string) {
    def bad as TargetScan init TargetScan{ok: false, anon: false, name: "", rest: ""};
    if (strings.startsWith($s, "__:")) {
        return TargetScan{ok: true, anon: true, name: "", rest: $s[3..]};
    }
    if (strings.startsWith($s, "_`")) {
        def close as int init strings.indexOf($s, "`:", 2);
        if ($close <= 2) {
            return $bad;
        }
        return TargetScan{ok: true, anon: false, name: $s[2..$close], rest: $s[$close + 2..]};
    }
    def cs as list of string init strings.chars($s);
    def k as int init 1;
    while ($k < len($cs)) {
        if ($cs[$k] == "\\") {
            $k = $k + 2;
            continue;
        }
        if ($cs[$k] == ":" and ($k + 1 == len($cs) or $cs[$k + 1] == " ")) {
            break;
        }
        $k = $k + 1;
    }
    if ($k >= len($cs) or $k == 1) {
        return $bad;
    }
    return TargetScan{ok: true, anon: false, name: unescapeChars($cs[1..$k]), rest: $s[$k + 1..]};
}

# targetNode builds the rst_target placeholder for a target block's text, or a
# plain "text" node when the text is not a well-formed target (it is then a
# comment). The URI's whitespace is removed, so a long one may wrap; a refname
# ending in `_` makes the target indirect, and no URI at all makes it internal.
func targetNode(s as string, line as int) {
    def ts as TargetScan init parseTarget(strings.trim($s));
    if (not $ts.ok) {
        return mk("text");
    }
    def rest as string init strings.trim($ts.rest);
    def t as doctree.Node init mk("rst_target");
    $t.text = normName($ts.name);
    $t.title = $ts.name;
    $t.level = $line;
    if ($rest == "") {
        if ($ts.anon) {
            return mk("text");
        }
        $t.lang = "internal";
        return $t;
    }
    def indirect as bool init strings.endsWith($rest, "_") and
        not strings.endsWith($rest, "\\_") and
        (strings.startsWith($rest, "`") or not regex.matches('\s', $rest));
    if ($indirect) {
        $t.lang = "indirect";
        $t.url = normName(stripTicks($rest[..len($rest) - 1]));
    } else {
        $t.lang = "external";
        $t.url = unescapeText(regex.replace('\s+', $rest, ""));
    }
    if ($ts.anon) {
        $t.lang = "anon_" + $t.lang;
    }
    return $t;
}

# noteBlock reads a footnote (`.. [1]`, `.. [#]`, `.. [#name]`, `.. [*]`) or a
# citation (`.. [name]`) and its body. Labels and numbers are settled by the
# reference pass; the parser records what kind of label was written.
func noteBlock(lines as list of string, i as int, base as int, depth as int, m as regex.Match) {
    def label as string init $m.groups[0];
    def kind as string init "citation";
    def name as string init $label;
    if (regex.matches('^[0-9]+$', $label)) {
        $kind = "manual";
    } elseif ($label == "#") {
        $kind = "auto";
    } elseif ($label == "*") {
        $kind = "symbol";
    } elseif (strings.startsWith($label, "#")) {
        $kind = "autolabel";
        $name = $label[1..];
    }
    def blk as IndentBlock init indentedBlock($lines, $i, $m.end);
    def node as doctree.Node init withChildren(
        "footnote",
        parseBlocks($blk.lines, $base + $i, $depth + 1, false));
    if ($kind == "citation") {
        $node.kind = "citation";
    }
    $node.level = $base + $i;
    $node.attrs = {"rst-kind": $kind, "rst-name": $name, "rst-key": "{$base + $i}:d"};
    return oneStep($node, $blk.next);
}

# directiveBody splits a directive's lines after its first one into the leading
# `:name: value` option lines and the content after them.
func directiveBody(rest as list of string) {
    def opts as map of string to string init {};
    def k as int init 0;
    while ($k < len($rest)) {
        def m as regex.Match init regex.find(OPTION_LINE_RE, $rest[$k]);
        if ($m.start != 0) {
            break;
        }
        $opts[strings.lower($m.groups[0])] = strings.trim($m.groups[1]);
        $k = $k + 1;
    }
    return DirectiveBody{options: $opts, content: $rest[$k..]};
}

func joinNonBlank(first as string, rest as list of string) {
    def parts as list of string init [];
    if ($first != "") {
        $parts[] = $first;
    }
    for (def l in $rest) {
        if (len($l) > 0) {
            $parts[] = strings.trim($l);
        }
    }
    return strings.join($parts, " ");
}

# substBlock reads a substitution definition, `.. |name| directive:: ...`. The
# directives a substitution can use here are `replace` (inline text), `unicode`
# (characters by code point) and `image`.
func substBlock(
    lines as list of string,
    i as int,
    base as int,
    m as regex.Match,
    blk as IndentBlock) {
    def name as string init $m.groups[0];
    def directive as string init strings.lower($m.groups[1]);
    def line as int init $base + $i;
    def first as string init strings.trim($lines[$i][$m.end..]);
    def body as DirectiveBody init directiveBody($blk.lines[1..]);
    def kids as list of doctree.Node init [];
    match ($directive) {
        when "replace" {
            def text as string init joinNonBlank($first, $blk.lines[1..]);
            if ($text == "") {
                return oneStep(
                    diagNode(
                        "error",
                        $line,
                        "Substitution definition \"" + $name + "\" empty or invalid."),
                    $blk.next);
            }
            $kids = inlineNodes($text, $line);
        }
        when "unicode" { $kids = [textOf(unicodeText(joinNonBlank($first, $body.content)))]; }
        when "image" {
            $kids = [substImage($name, joinNonBlank($first, $body.content), $body.options, $line)];
        }
        else {
            return oneStep(
                diagNode(
                    "error",
                    $line,
                    "Substitution definition \"" + $name + "\" uses the unsupported directive \"" +
                        $directive + "\"."),
                $blk.next);
        }
    }
    def s as doctree.Node init withChildren("rst_subst", $kids);
    $s.text = $name;
    $s.level = $line;
    return oneStep($s, $blk.next);
}

# substImage is the image a substitution stands for, linked when it has a
# `:target:` (a URI, or a reference name ending in `_`).
func substImage(name as string, uri as string, opts as map of string to string, line as int) {
    def img as doctree.Node init mk("image");
    $img.url = regex.replace('\s+', $uri, "");
    $img.text = $name;
    if (maps.has($opts, "alt")) {
        $img.text = $opts["alt"];
    }
    def attrs as map of string to string init {};
    for (def k in ["width", "height", "class"]) {
        if (maps.has($opts, $k)) {
            $attrs[$k] = $opts[$k];
        }
    }
    $img.attrs = $attrs;
    if (not maps.has($opts, "target")) {
        return $img;
    }
    def target as string init $opts["target"];
    if (strings.endsWith($target, "_")) {
        def ref as string init normName(stripTicks($target[..len($target) - 1]));
        def r as doctree.Node init refNode("named", $ref, "", [$img], $target, "{$line}:s");
        $r.level = $line;
        return $r;
    }
    return linkNode(regex.replace('\s+', $target, ""), [$img]);
}

# unicodeText is the text of a `unicode` substitution: each code given as
# `U+00A9`, `0xA9`, `xA9`, `\xA9`, `uA9`, `\uA9`, `&#xA9;` or `&#169;` becomes
# that character, other words stand for themselves, spaces are dropped, and `..`
# starts a comment.
func unicodeText(s as string) {
    def out as list of string init [];
    for (def tok in regex.split('\s+', strings.trim($s))) {
        if ($tok == "..") {
            break;
        }
        $out[] = unicodeToken($tok);
    }
    return strings.join($out, "");
}

func unicodeToken(tok as string) {
    def code as int init -1;
    def hm as regex.Match init regex.find('^(?:0x|x|\\x|U\+|u|\\u|&#x)([0-9a-fA-F]+);?$', $tok);
    if ($hm.start == 0 and len($hm.groups[0]) <= 6) {
        $code = convert.toInt($hm.groups[0], 16);
    }
    def dm as regex.Match init regex.find('^(?:&#)?([0-9]{1,7});?$', $tok);
    if ($code < 0 and $dm.start == 0) {
        $code = convert.toInt($dm.groups[0]);
    }
    if ($code < 0 or $code > 1114111 or ($code >= 55296 and $code <= 57343)) {
        return $tok;
    }
    return convert.fromCodepoint($code);
}

# unknownDirective reports a directive this module does not implement and keeps
# its source as a literal block, so no content silently disappears.
func unknownDirective(lines as list of string, i as int, next as int, name as string, base as int) {
    def code as doctree.Node init mk("code");
    $code.text = strings.join($lines[$i..$next], "\n");
    $code.attrs = {"class": "literal-block"};
    return Step{
        nodes: [diagNode("error", $base + $i, "Unknown directive type \"" + $name + "\"."), $code],
        next: $next
    };
}

# --- whole-document transforms ------------------------------------------

# firstContent is the index of the first node at or after k that is not a
# diagnostic placeholder, or -1.
func firstContent(kids as list of doctree.Node, k as int) {
    for (def j in $k..len($kids)) {
        if (not isPlaceholder($kids[$j].kind)) {
            return $j;
        }
    }
    return -1;
}

func countDepth(kids as list of doctree.Node, depth as int) {
    def c as int init 0;
    for (def k in $kids) {
        if ($k.kind == "heading" and $k.level == $depth) {
            $c = $c + 1;
        }
    }
    return $c;
}

# promote applies the whole-document transforms to the top-level blocks: the
# document title and subtitle, heading levels from section depths, heading ids,
# and the docinfo field list. It edits the list in place: call it as
# `$kids = promote($kids, $opts);`.
func promote(kids as list of doctree.Node, opts as Options) { # lint-disable: L205
    def t as int init -1;
    def s as int init -1;
    if (not $opts.noTitlePromotion) {
        def f as int init firstContent($kids, 0);
        if ($f >= 0 and $kids[$f].kind == "heading" and $kids[$f].level == 1 and
            countDepth($kids, 1) == 1) {
            $t = $f;
            def g as int init firstContent($kids, $t + 1);
            if ($g >= 0 and $kids[$g].kind == "heading" and $kids[$g].level == 2 and
                countDepth($kids, 2) == 1) {
                $s = $g;
            }
        }
    }
    def offset as int init 0;
    if ($opts.initialHeadingLevel > 1) {
        $offset = $opts.initialHeadingLevel - 1;
    }
    def shift as int init 0;
    if ($s >= 0) {
        $shift = 1;
    }
    def ids as map of string to bool init {};
    def clamped as bool init false;
    for (def k in 0..len($kids)) {
        if ($kids[$k].kind != "heading") {
            continue;
        }
        def id as string init uniqueId(slugOf(doctree.text($kids[$k])), $ids);
        $ids[$id] = true;
        if ($k == $s) {
            $kids[$k].kind = "paragraph";
            $kids[$k].level = 0;
            $kids[$k].attrs = {"class": "subtitle", "id": $id};
            continue;
        }
        def lvl as int init $kids[$k].level - $shift + $offset;
        def attrs as map of string to string init {"id": $id};
        if ($k == $t) {
            $lvl = 1 + $offset;
            $attrs["class"] = "title";
        }
        if ($lvl > 6) {
            $clamped = true;
            $lvl = 6;
        }
        $kids[$k].level = $lvl;
        $kids[$k].attrs = $attrs;
    }
    if ($clamped) {
        $kids[] = diagNode(
            "warning",
            0,
            "Sections nest deeper than 6 heading levels; deeper ones render at level 6.");
    }
    $kids = docinfoPass($kids, $t, $s);
    return $kids;
}

# docinfoPass turns the field list that opens the document (after any title and
# subtitle) into the bibliographic docinfo; its `dedication` and `abstract`
# fields become topics after it, as docutils does. Call it as
# `$kids = docinfoPass($kids, ...);`.
func docinfoPass(kids as list of doctree.Node, t as int, s as int) { # lint-disable: L205
    def after as int init 0;
    if ($s >= 0) {
        $after = $s + 1;
    } elseif ($t >= 0) {
        $after = $t + 1;
    }
    def f as int init firstContent($kids, $after);
    if ($f < 0 or not isFieldList($kids[$f])) {
        return $kids;
    }
    def fields as list of doctree.Node init [];
    def topics as list of doctree.Node init [];
    def k as int init 0;
    while ($k + 1 < len($kids[$f].children)) {
        def name as string init strings.trim(doctree.text($kids[$f].children[$k]));
        if (lists.contains(DOCINFO_TOPICS, strings.lower($name))) {
            def topic as doctree.Node init withChildren(
                "topic",
                descBlocks($kids[$f].children[$k + 1]));
            $topic.title = $name;
            $topic.attrs = {"class": strings.lower($name)};
            $topics[] = $topic;
        } else {
            $fields[] = $kids[$f].children[$k];
            $fields[] = $kids[$f].children[$k + 1];
        }
        $k = $k + 2;
    }
    $kids[$f].children = $fields;
    $kids[$f].attrs = {"class": "docinfo"};
    if (len($topics) == 0) {
        return $kids;
    }
    # Rare: the topics go right after the docinfo, so the list is rebuilt once.
    def out as list of doctree.Node init [];
    for (def j in 0..len($kids)) {
        if ($j != $f or len($fields) > 0) {
            $out[] = $kids[$j];
        }
        if ($j == $f) {
            for (def tp in $topics) {
                $out[] = $tp;
            }
        }
    }
    return $out;
}

func isFieldList(n as doctree.Node) {
    return $n.kind == "definition_list" and maps.has($n.attrs, "class") and
        $n.attrs["class"] == "field-list";
}

# descBlocks returns a definition's content as blocks: a compacted definition
# (inline children) is wrapped back into a paragraph.
func descBlocks(desc as doctree.Node) {
    for (def c in $desc.children) {
        if (isBlockNode($c)) {
            return $desc.children;
        }
    }
    return [withChildren("paragraph", $desc.children)];
}

func isBlockNode(n as doctree.Node) {
    return lists.contains(
        [
            "paragraph",
            "code",
            "list",
            "quote",
            "definition_list",
            "line_block",
            "table",
            "thematic_break",
            "heading",
            "topic",
            "admonition",
            "container",
            "figure",
            "rubric",
            "html_block"
        ],
        $n.kind);
}

# slugOf makes an HTML id from a title the way docutils does: lowercase, runs of
# anything but a-z and 0-9 become one hyphen, and leading non-letters go.
func slugOf(title as string) {
    def s as string init regex.replace('[^a-z0-9]+', strings.lower($title), "-");
    $s = regex.replace('^[^a-z]+', $s, "");
    $s = regex.replace('-+$', $s, "");
    if ($s == "") {
        return "section";
    }
    return $s;
}

func uniqueId(slug as string, ids as map of string to bool) {
    if (not maps.has($ids, $slug)) {
        return $slug;
    }
    def k as int init 2;
    while (maps.has($ids, "{$slug}-{$k}")) {
        $k = $k + 1;
    }
    return "{$slug}-{$k}";
}

# gatherDiags collects the diagnostic placeholders under a node, in document
# order. Leaves are checked in place rather than visited with a call.
func gatherDiags(n as doctree.Node) {
    def out as list of Diagnostic init [];
    for (def k in 0..len($n.children)) {
        if ($n.children[$k].kind == "rst_diag") {
            $out[] = Diagnostic{
                severity: $n.children[$k].lang,
                line: $n.children[$k].level,
                message: $n.children[$k].text
            };
        } elseif (len($n.children[$k].children) > 0) {
            for (def d in gatherDiags($n.children[$k])) {
                $out[] = $d;
            }
        }
    }
    return $out;
}

# hasDiag is true when a diagnostic placeholder sits anywhere under a node.
func hasDiag(n as doctree.Node) {
    for (def c in $n.children) {
        if ($c.kind == "rst_diag" or (len($c.children) > 0 and hasDiag($c))) {
            return true;
        }
    }
    return false;
}

# stripDiags returns a copy of the node with every diagnostic placeholder
# removed. Only the paths that lead to a placeholder are rebuilt; the parameter
# is never written, so it is borrowed rather than copied.
func stripDiags(n as doctree.Node) {
    def kids as list of doctree.Node init [];
    for (def k in 0..len($n.children)) {
        if ($n.children[$k].kind == "rst_diag") {
            continue;
        }
        if (hasDiag($n.children[$k])) {
            $kids[] = stripDiags($n.children[$k]);
        } else {
            $kids[] = $n.children[$k];
        }
    }
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

# countNodes counts a node and everything under it, recursing only into the
# children that have children of their own.
func countNodes(n as doctree.Node) {
    def total as int init 1 + len($n.children);
    for (def k in 0..len($n.children)) {
        if (len($n.children[$k].children) > 0) {
            $total = $total + countNodes($n.children[$k]) - 1;
        }
    }
    return $total;
}

# defaultOptions is the zero Options value: lenient, raw HTML escaped, headings
# from level 1, title promotion on.
func defaultOptions() {
    def o as Options;
    return $o;
}

# parseTop parses the whole document and applies the document transforms.
func parseTop(src as string, opts as Options) {
    def kids as list of doctree.Node init parseBlocks(normalize($src), 1, 0, true);
    $kids = promote($kids, $opts);
    if (hasKind($kids, "rst_contents")) {
        $kids = contentsPass($kids);
    }
    return $kids;
}

# guardedParse is parseTop with a resource limit hit while parsing (the
# interpreter's own call-depth cap, reached before MAX_DEPTH on the constrained
# build) reported with the module's own message; it stays kind "limit". The `try` only
# wraps a `return`, so the tree is never copied on its way out.
func guardedParse(src as string, opts as Options) {
    try {
        return parseTop($src, $opts);
    } catch (e) {
        if ($e.kind == "limit") {
            rstLimit("document nests too deep (" + $e.message + ")");
        }
        throw $e;
    }
}

# sortDiags orders diagnostics by line, keeping the order of those on the same
# line (an insertion sort: a document has few).
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

# run parses a document, resolves its references, and collects its
# diagnostics.
func run(src as string, opts as Options) {
    def doc as doctree.Node init withChildren("document", guardedParse($src, $opts));
    def refs as Refs init newRefs();
    $refs = collect($refs, $doc);
    if ($refs.count > 0) {
        $refs = finishRefs($refs);
        $doc = withChildren("document", resolveKids($doc.children, $refs, 0));
    }
    def diags as list of Diagnostic init $refs.diags;
    for (def d in gatherDiags($doc)) {
        $diags[] = $d;
    }
    $diags = sortDiags($diags);
    if (len($diags) > 0) {
        $doc = stripDiags($doc);
    }
    if (countNodes($doc) > MAX_NODES) {
        rstLimit("document too large (over {MAX_NODES} nodes)");
    }
    if ($opts.strict and len($diags) > 0) {
        rstFail("line {$diags[0].line}: {$diags[0].message}");
    }
    return Result{doc: $doc, diags: $diags};
}

/**
 * Parse reStructuredText into a document tree: a `doctree.Node` whose `typeOf`
 * is `"document"`, walked with `doctree.findAll` / `doctree.text` and friends
 * and rendered with `doctree.render` / `doctree.renderPdf`. Problems in the
 * markup are recovered from (see `check`); a document that nests too deep or is
 * too large is a catchable kind `"limit"` error.
 * @param src {string} the rST source
 * @return {doctree.Node} the document root node
 */
export func parse(src as string) {
    return run($src, defaultOptions()).doc;
}

/**
 * Parse reStructuredText with explicit options.
 * @param src {string} the rST source
 * @param opts {Options} the parse options
 * @return {doctree.Node} the document root node
 * @throws {Error} kind `"rst"` on the first problem when `opts.strict` is set
 */
export func parseWith(src as string, opts as Options) {
    return run($src, $opts).doc;
}

/**
 * The problems in a document, in source order: markup that was read some other
 * way than it looks (`"warning"`) or could not be read as written (`"error"`).
 * An empty list means the document is clean.
 * @param src {string} the rST source
 * @return {list of Diagnostic} the problems found
 */
export func check(src as string) {
    return run($src, defaultOptions()).diags;
}

/**
 * Render reStructuredText to HTML. Safe by default: everything is escaped and
 * link targets go through `doctree`'s URL filter.
 * @param src {string} the rST source
 * @return {string} the rendered HTML
 */
export func toHtml(src as string) {
    return doctree.render("html", parse($src));
}

/**
 * Render reStructuredText to well-formed XHTML (void elements self-close).
 * @param src {string} the rST source
 * @return {string} the rendered XHTML
 */
export func toXhtml(src as string) {
    return doctree.render("xhtml", parse($src));
}

/**
 * Render reStructuredText to HTML with explicit options.
 * @param src {string} the rST source
 * @param opts {Options} the parse and render options
 * @return {string} the rendered HTML
 * @throws {Error} kind `"rst"` on the first problem when `opts.strict` is set
 */
export func toHtmlWith(src as string, opts as Options) {
    def html as doctree.HtmlOptions;
    $html.allowRawHtml = $opts.allowRawHtml;
    return doctree.renderHtml(parseWith($src, $opts), $html);
}

/**
 * Render reStructuredText to styled terminal text.
 * @param src {string} the rST source
 * @return {string} the rendered text, with ANSI styling
 */
export func toAnsi(src as string) {
    return doctree.render("ansi", parse($src));
}

/**
 * The document title: the text of the promoted title heading, or "" when the
 * document has none.
 * @param doc {doctree.Node} a document from `parse`
 * @return {string} the title text
 */
export func title(doc as doctree.Node) {
    for (def c in $doc.children) {
        if ($c.kind == "heading" and maps.has($c.attrs, "class") and $c.attrs["class"] == "title") {
            return doctree.text($c);
        }
    }
    return "";
}

/**
 * The bibliographic fields of a document (the field list at its top, such as
 * `:Author:` or `:Version:`), keyed by lowercased field name, each value the
 * field's flattened text. Empty when the document has no docinfo.
 * @param doc {doctree.Node} a document from `parse`
 * @return {map of string to string} field name to text
 */
export func docinfo(doc as doctree.Node) {
    def out as map of string to string init {};
    for (def c in $doc.children) {
        if ($c.kind == "definition_list" and maps.has($c.attrs, "class") and
            $c.attrs["class"] == "docinfo") {
            def k as int init 0;
            while ($k + 1 < len($c.children)) {
                def name as string init strings.lower(strings.trim(doctree.text($c.children[$k])));
                $out[$name] = strings.trim(doctree.text($c.children[$k + 1]));
                $k = $k + 2;
            }
        }
    }
    return $out;
}
