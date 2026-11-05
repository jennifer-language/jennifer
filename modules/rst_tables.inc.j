# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0
#
# rst_tables.inc.j - grid tables and simple tables for the rst module. Included
# (textual splice) into rst.j; it declares no `use` of its own and relies on the
# includer's imports and helpers. Private to the rst module.
#
# Both kinds become a doctree `table` of `row`s of `cell`s. A cell holds blocks
# (it is parsed like any other body), unwrapped to its inline children when it
# is a single paragraph. A spanning cell carries `colspan` / `rowspan` attrs and
# the table its header row count in `headerRows`. A table that cannot be read is
# kept as a literal block of its source, with an error diagnostic.

# A grid table's top border, and the `+===+` line that ends its header rows.
def const GRID_TOP_RE as string init '^\+(?:-+\+)+$';
def const GRID_BORDER_RE as string init '^\+(?:[-=]+\+)+$';
def const GRID_HEAD_SEP_RE as string init '^\+(?:=+\+)+$';
# A simple table's border: two or more runs of `=` (one run is a section
# adornment). Interior borders may have fewer runs; a `-` line under a row
# marks that row's column spans.
def const SIMPLE_TOP_RE as string init '^=+(?: +=+)+$';
def const SIMPLE_BORDER_RE as string init '^=+(?: +=+)*$';
def const SIMPLE_SPAN_RE as string init '^-+(?: +-+)*$';
# Tables bigger than this many cells are refused (an "rst" error), so a hostile
# document cannot make the cell scan run without bound.
def const MAX_TABLE_CELLS as int init 20000;

# One grid-table cell: the corners of its box, in table coordinates.
def struct GridCell {
    top as int,
    left as int,
    bottom as int,
    right as int
};

# The bottom-right corner scanCell found for a top-left one, or ok false.
def struct CellScan {
    ok as bool,
    bottom as int,
    right as int
};

# A column of a simple table: where it starts and (exclusive) ends.
def struct Span {
    start as int,
    stop as int
};

# A simple-table row while it is read: its text lines, the source index of its
# first line, and the column groups its cells cover (from a `-` span line, or
# one column each).
def struct SimpleRow {
    lines as list of string,
    first as int,
    groups as list of Span
};

# tableFallback keeps a table that could not be read as a literal block of its
# source, after an error diagnostic, so no content disappears.
func tableFallback(lines as list of string, i as int, next as int, base as int, msg as string) {
    def code as doctree.Node init mk("code");
    $code.text = strings.join($lines[$i..$next], "\n");
    $code.attrs = {"class": "literal-block"};
    return Step{nodes: [diagNode("error", $base + $i, $msg), $code], next: $next};
}

# cellNode parses a cell's text lines (already cut out of the table) as body
# blocks: dedented by their common indent, blank lines around them dropped.
func cellNode(text as list of string, line as int, depth as int) {
    def minInd as int init -1;
    def first as int init -1;
    def last as int init -1;
    for (def k in 0..len($text)) {
        if (len($text[$k]) == 0) {
            continue;
        }
        if ($first < 0) {
            $first = $k;
        }
        $last = $k;
        def ind as int init indentOf($text[$k]);
        if ($minInd < 0 or $ind < $minInd) {
            $minInd = $ind;
        }
    }
    def cell as doctree.Node init mk("cell");
    if ($first < 0) {
        return $cell;
    }
    def body as list of string init [];
    for (def k in $first..$last + 1) {
        if (len($text[$k]) == 0) {
            $body[] = "";
        } else {
            $body[] = $text[$k][$minInd..];
        }
    }
    $cell.children = compactCell(parseBlocks($body, $line + $first, $depth + 1, false));
    return $cell;
}

# compactCell unwraps a cell that is one paragraph to its inline children (the
# way docutils' HTML writer prints a simple cell); anything else stays blocks.
func compactCell(kids as list of doctree.Node) {
    def real as int init 0;
    def para as int init -1;
    for (def k in 0..len($kids)) {
        if ($kids[$k].kind == "rst_diag") {
            continue;
        }
        $real = $real + 1;
        if ($kids[$k].kind == "paragraph" and len($kids[$k].attrs) == 0) {
            $para = $k;
        }
    }
    if ($real != 1 or $para < 0) {
        return $kids;
    }
    def out as list of doctree.Node init [];
    for (def k in 0..len($kids)) {
        if ($k == $para) {
            for (def c in $kids[$k].children) {
                $out[] = $c;
            }
        } else {
            $out[] = $kids[$k];
        }
    }
    return $out;
}

# spanCell sets a cell's colspan / rowspan attrs when they are above 1.
func spanCell(cell as doctree.Node, cols as int, rows as int) {
    def c as doctree.Node init $cell;
    if ($cols > 1) {
        $c.attrs["colspan"] = "{$cols}";
    }
    if ($rows > 1) {
        $c.attrs["rowspan"] = "{$rows}";
    }
    return $c;
}

func tableOf(rows as list of doctree.Node, headerRows as int) {
    def t as doctree.Node init withChildren("table", $rows);
    $t.attrs = {"headerRows": "{$headerRows}"};
    return $t;
}

# --- grid tables -------------------------------------------------------

# gridTable reads the grid table starting at the `+---+` line i: the lines up to
# the first blank one, which must form a rectangle closed by a border line.
# Cells are found the way docutils finds them - from each top-left corner, scan
# right along the top edge to a `+` from which an edge runs down, then left
# along the bottom and back up - so a cell may span rows and columns.
func gridTable(lines as list of string, i as int, base as int, depth as int) {
    def n as int init len($lines);
    def j as int init $i;
    while ($j < $n and len($lines[$j]) > 0 and
        (strings.startsWith($lines[$j], "+") or strings.startsWith($lines[$j], "|"))) {
        $j = $j + 1;
    }
    def width as int init len($lines[$i]);
    for (def k in $i..$j) {
        def l as string init $lines[$k];
        def end as string init $l[len($l) - 1..];
        if (len($l) != $width or ($end != "+" and $end != "|")) {
            return tableFallback(
                $lines,
                $i,
                $j,
                $base,
                "Malformed table: the lines are not all the same width.");
        }
    }
    if ($j - $i < 3 or not regex.matches(GRID_BORDER_RE, $lines[$j - 1])) {
        return tableFallback(
            $lines,
            $i,
            $j,
            $base,
            "Malformed table: no bottom table border found.");
    }
    def g as list of list of string init [];
    def headSep as int init -1;
    for (def k in $i..$j) {
        def l as string init $lines[$k];
        if ($k > $i and $k < $j - 1 and regex.matches(GRID_HEAD_SEP_RE, $l)) {
            if ($headSep >= 0) {
                return tableFallback(
                    $lines,
                    $i,
                    $j,
                    $base,
                    "Malformed table: multiple head/body row separators.");
            }
            $headSep = $k - $i;
            $l = strings.replace($l, "=", "-");
        }
        $g[] = strings.chars($l);
    }
    def cells as list of GridCell init scanGrid($g);
    if (len($cells) == 0) {
        return tableFallback(
            $lines,
            $i,
            $j,
            $base,
            "Malformed table: the cells do not fit together.");
    }
    return oneStep(gridStructure($g, $cells, $headSep, $base + $i, $depth), $j);
}

# scanGrid finds every cell of a grid (docutils' GridTableParser.parse_table):
# corners are taken in (top, left) order, `done[c]` is the last grid row column
# c is filled to. An empty result means the grid is malformed.
func scanGrid(g as list of list of string) {
    def bottom as int init len($g) - 1;
    def right as int init len($g[0]) - 1;
    def done as list of int init [];
    for (def c in 0..$right + 1) {
        $done[] = -1;
    }
    def tops as list of int init [0];
    def lefts as list of int init [0];
    def cells as list of GridCell init [];
    while (len($tops) > 0) {
        # Take the smallest corner; a table has few pending corners at a time.
        def best as int init 0;
        for (def k in 1..len($tops)) {
            if ($tops[$k] < $tops[$best] or
                ($tops[$k] == $tops[$best] and $lefts[$k] < $lefts[$best])) {
                $best = $k;
            }
        }
        def top as int init $tops[$best];
        def left as int init $lefts[$best];
        def lastK as int init len($tops) - 1;
        $tops[$best] = $tops[$lastK];
        $lefts[$best] = $lefts[$lastK];
        $tops = $tops[..$lastK];
        $lefts = $lefts[..$lastK];
        if ($top == $bottom or $left == $right or $top <= $done[$left]) {
            continue;
        }
        def cs as CellScan init scanRight($g, $top, $left, $bottom, $right);
        if (not $cs.ok) {
            continue;
        }
        for (def c in $left..$cs.right) {
            if ($done[$c] != $top - 1) {
                return [];
            }
            $done[$c] = $cs.bottom - 1;
        }
        $cells[] = GridCell{top: $top, left: $left, bottom: $cs.bottom, right: $cs.right};
        if (len($cells) > MAX_TABLE_CELLS) {
            rstFail("table too large (over {MAX_TABLE_CELLS} cells)");
        }
        $tops[] = $top;
        $lefts[] = $cs.right;
        $tops[] = $cs.bottom;
        $lefts[] = $left;
    }
    for (def c in 0..$right) {
        if ($done[$c] != $bottom - 1) {
            return [];
        }
    }
    return $cells;
}

func noCell() {
    return CellScan{ok: false, bottom: 0, right: 0};
}

# scanRight follows a cell's top edge from its top-left corner; at each `+` it
# tries the edge down from there, so a cell spanning columns skips the inner `+`
# whose edge does not reach a bottom corner.
func scanRight(g as list of list of string, top as int, left as int, bottom as int, right as int) {
    for (def c in $left + 1..$right + 1) {
        def ch as string init $g[$top][$c];
        if ($ch == "+") {
            def b as int init scanDown($g, $top, $left, $c, $bottom);
            if ($b >= 0) {
                return CellScan{ok: true, bottom: $b, right: $c};
            }
        } elseif ($ch != "-") {
            return noCell();
        }
    }
    return noCell();
}

# scanDown follows a cell's right edge; it returns the row of the bottom-right
# corner whose bottom and left edges close the box, or -1.
func scanDown(g as list of list of string, top as int, left as int, right as int, bottom as int) {
    for (def r in $top + 1..$bottom + 1) {
        def ch as string init $g[$r][$right];
        if ($ch == "+") {
            if (scanLeft($g, $top, $left, $r, $right)) {
                return $r;
            }
        } elseif ($ch != "|") {
            return -1;
        }
    }
    return -1;
}

# scanLeft checks a cell's bottom edge back to its bottom-left corner, then its
# left edge up to the top.
func scanLeft(g as list of list of string, top as int, left as int, bottom as int, right as int) {
    def c as int init $right - 1;
    while ($c > $left) {
        def ch as string init $g[$bottom][$c];
        if ($ch != "+" and $ch != "-") {
            return false;
        }
        $c = $c - 1;
    }
    if ($g[$bottom][$left] != "+") {
        return false;
    }
    def r as int init $bottom - 1;
    while ($r > $top) {
        def ch as string init $g[$r][$left];
        if ($ch != "+" and $ch != "|") {
            return false;
        }
        $r = $r - 1;
    }
    return true;
}

# gridStructure turns the found cells into rows: the distinct top / bottom rows
# and left / right columns of all the cells are the table's row and column
# boundaries, and a cell spans the boundaries between its edges.
func gridStructure(
    g as list of list of string,
    cells as list of GridCell,
    headSep as int,
    line as int,
    depth as int) {
    def rowB as list of bool init [];
    for (def r in 0..len($g)) {
        $rowB[] = false;
    }
    def colB as list of bool init [];
    for (def c in 0..len($g[0])) {
        $colB[] = false;
    }
    for (def cell in $cells) {
        $rowB[$cell.top] = true;
        $rowB[$cell.bottom] = true;
        $colB[$cell.left] = true;
        $colB[$cell.right] = true;
    }
    def rowIdx as list of int init boundaryIndex($rowB);
    def colIdx as list of int init boundaryIndex($colB);
    def rows as list of doctree.Node init [];
    def cur as int init -1;
    def row as doctree.Node init mk("row");
    for (def cell in $cells) {
        if ($rowIdx[$cell.top] != $cur) {
            if ($cur >= 0) {
                $rows[] = $row;
            }
            $row = mk("row");
            $cur = $rowIdx[$cell.top];
        }
        def text as list of string init [];
        for (def r in $cell.top + 1..$cell.bottom) {
            $text[] = strings.trimRight(strings.join($g[$r][$cell.left + 1..$cell.right], ""));
        }
        def node as doctree.Node init cellNode($text, $line + $cell.top + 1, $depth);
        $row.children[] = spanCell(
            $node,
            $colIdx[$cell.right] - $colIdx[$cell.left],
            $rowIdx[$cell.bottom] - $rowIdx[$cell.top]);
    }
    $rows[] = $row;
    def headerRows as int init 0;
    if ($headSep >= 0) {
        $headerRows = $rowIdx[$headSep];
    }
    return tableOf($rows, $headerRows);
}

# boundaryIndex numbers the boundary positions in a flag list: entry k is how
# many boundaries come before position k (so a boundary's own index).
func boundaryIndex(flags as list of bool) {
    def out as list of int init [];
    def count as int init 0;
    for (def f in $flags) {
        $out[] = $count;
        if ($f) {
            $count = $count + 1;
        }
    }
    return $out;
}

# --- simple tables -----------------------------------------------------

# simpleTable reads the simple table whose top border is line i. It ends at a
# border line followed by a blank line (or the end); a border before that ends
# the header rows. Columns come from the top border's runs of `=`; text in the
# last column may run past its border. A line whose first column is blank
# continues the row above, and a `-` line under a row groups its columns into
# spanning cells.
func simpleTable(lines as list of string, i as int, base as int, depth as int) {
    def n as int init len($lines);
    def end as int init -1;
    def seps as list of int init [];
    def j as int init $i + 1;
    while ($j < $n) {
        if (len($lines[$j]) > 0 and regex.matches(SIMPLE_BORDER_RE, $lines[$j])) {
            if ($j + 1 >= $n or len($lines[$j + 1]) == 0) {
                $end = $j;
                break;
            }
            $seps[] = $j;
        }
        $j = $j + 1;
    }
    if ($end < 0) {
        def stop as int init $i + 1;
        while ($stop < $n and len($lines[$stop]) > 0) {
            $stop = $stop + 1;
        }
        return tableFallback(
            $lines,
            $i,
            $stop,
            $base,
            "Malformed table: no bottom table border found.");
    }
    if (len($seps) > 1) {
        return tableFallback(
            $lines,
            $i,
            $end + 1,
            $base,
            "Malformed table: multiple head/body row separators.");
    }
    def headSep as int init -1;
    if (len($seps) == 1) {
        $headSep = $seps[0];
    }
    def cols as list of Span init columnsOf($lines[$i]);
    def rows as list of SimpleRow init [];
    def pendingBlank as bool init false;
    for (def k in $i + 1..$end) {
        def l as string init $lines[$k];
        if ($k == $headSep) {
            $pendingBlank = false;
            continue;
        }
        if (len($l) == 0) {
            $pendingBlank = true;
            continue;
        }
        def nr as int init len($rows);
        if (regex.matches(SIMPLE_SPAN_RE, $l) and $nr > 0) {
            def groups as list of Span init spanGroups($l, $cols);
            if (len($groups) == 0) {
                return tableFallback(
                    $lines,
                    $i,
                    $end + 1,
                    $base,
                    "Malformed table: a column span underline does not match the columns.");
            }
            $rows[$nr - 1].groups = $groups;
            $pendingBlank = false;
            continue;
        }
        def lead as string init "";
        if (len($l) > $cols[0].start) {
            $lead = strings.trim($l[$cols[0].start..minInt(len($l), $cols[0].stop)]);
        }
        if ($nr > 0 and $lead == "" and
            ($k < $headSep or $headSep < 0 or $rows[$nr - 1].first > $headSep)) {
            if ($pendingBlank) {
                $rows[$nr - 1].lines[] = "";
            }
            $rows[$nr - 1].lines[] = $l;
        } else {
            $rows[] = SimpleRow{lines: [$l], first: $k, groups: oneColumnEach($cols)};
        }
        $pendingBlank = false;
    }
    def out as list of doctree.Node init [];
    def headerRows as int init 0;
    for (def r in $rows) {
        if (marginText($r, $cols)) {
            return tableFallback(
                $lines,
                $i,
                $end + 1,
                $base,
                "Malformed table: text in a column margin.");
        }
        $out[] = simpleRowNode($r, $cols, $base, $depth);
        if ($headSep >= 0 and $r.first < $headSep) {
            $headerRows = $headerRows + 1;
        }
    }
    if (len($out) * len($cols) > MAX_TABLE_CELLS) {
        rstFail("table too large (over {MAX_TABLE_CELLS} cells)");
    }
    return oneStep(tableOf($out, $headerRows), $end + 1);
}

func minInt(a as int, b as int) {
    if ($a < $b) {
        return $a;
    }
    return $b;
}

# columnsOf reads the columns off a border line: each run of `=` is one.
func columnsOf(border as string) {
    def out as list of Span init [];
    def cs as list of string init strings.chars($border);
    def k as int init 0;
    while ($k < len($cs)) {
        if ($cs[$k] != "=") {
            $k = $k + 1;
            continue;
        }
        def from as int init $k;
        while ($k < len($cs) and $cs[$k] == "=") {
            $k = $k + 1;
        }
        $out[] = Span{start: $from, stop: $k};
    }
    return $out;
}

func oneColumnEach(cols as list of Span) {
    def out as list of Span init [];
    for (def k in 0..len($cols)) {
        $out[] = Span{start: $k, stop: $k + 1};
    }
    return $out;
}

# spanGroups reads a `-` span line into column groups (column index ranges).
# Each run of `-` must start at a column's start and end at a column's end (the
# last column's end is open). An empty result means it does not line up.
func spanGroups(line as string, cols as list of Span) {
    def out as list of Span init [];
    def last as int init len($cols) - 1;
    for (def run in columnsOf(strings.replace($line, "-", "="))) {
        def a as int init -1;
        def b as int init -1;
        for (def k in 0..len($cols)) {
            if ($cols[$k].start == $run.start) {
                $a = $k;
            }
            if ($cols[$k].stop == $run.stop or ($k == $last and $run.stop >= $cols[$k].stop)) {
                $b = $k;
            }
        }
        if ($a < 0 or $b < $a) {
            return [];
        }
        $out[] = Span{start: $a, stop: $b + 1};
    }
    def next as int init 0;
    for (def gr in $out) {
        if ($gr.start != $next) {
            return [];
        }
        $next = $gr.stop;
    }
    if ($next != len($cols)) {
        return [];
    }
    return $out;
}

# groupText cuts one row line down to a column group's text: from its first
# column's start to its last column's end (the end of the line in the last
# column).
func groupText(l as string, cols as list of Span, gr as Span) {
    def from as int init $cols[$gr.start].start;
    if (len($l) <= $from) {
        return "";
    }
    if ($gr.stop == len($cols)) {
        return strings.trimRight($l[$from..]);
    }
    return strings.trimRight($l[$from..minInt(len($l), $cols[$gr.stop - 1].stop)]);
}

# marginText is true when a row has text between its cells, where only spaces
# may go.
func marginText(r as SimpleRow, cols as list of Span) {
    for (def l in $r.lines) {
        for (def k in 0..len($r.groups) - 1) {
            def from as int init $cols[$r.groups[$k].stop - 1].stop;
            def upto as int init minInt(len($l), $cols[$r.groups[$k + 1].start].start);
            if ($from < $upto and strings.trim($l[$from..$upto]) != "") {
                return true;
            }
        }
        if (len($l) > 0 and $cols[0].start > 0 and
            strings.trim($l[..minInt(len($l), $cols[0].start)]) != "") {
            return true;
        }
    }
    return false;
}

func simpleRowNode(r as SimpleRow, cols as list of Span, base as int, depth as int) {
    def row as doctree.Node init mk("row");
    for (def gr in $r.groups) {
        def text as list of string init [];
        for (def l in $r.lines) {
            $text[] = groupText($l, $cols, $gr);
        }
        $row.children[] = spanCell(
            cellNode($text, $base + $r.first, $depth),
            $gr.stop - $gr.start,
            1);
    }
    return $row;
}
