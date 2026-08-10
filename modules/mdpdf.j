# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0

/**
 * Render a Markdown document to a laid-out PDF ("write markup, get a PDF"): parse
 * with the `markdown` module, then flow the block tree onto paginated pages with the
 * `pdf` layout primitives (`measureText` / `wrapText` / `text` / `line`). A heading
 * becomes sized bold text, a paragraph a word-wrapped block with per-run font styling
 * (bold / italic / inline code), a list indented markers (ordered / unordered,
 * nested), a GFM table a ruled grid (per-column alignment, bold header), a fenced
 * code block a monospaced block, and a blockquote indented content. Blocks paginate:
 * a new page starts when the next line would cross the bottom margin.
 *
 * The layout runs over `markdown.parse`'s public tree, so a caller can transform the
 * document (drop a section, rewrite links) before rendering. Uses the standard-14
 * PDF fonts, so text is best kept to the Latin-1 range; an embedded Unicode font is
 * a future refinement. Pure Jennifer over `markdown` + `pdf`; both binaries.
 *
 * @module mdpdf
 * @example
 * import "mdpdf.j" as mdpdf;
 * def out as bytes init mdpdf.render("# Title\n\nA **bold** paragraph.\n");
 * # write the bytes to a file with fs.writeBytes(...)
 */

import "./markdown.j" as markdown;
import "./pdf.j" as pdf;

use strings;
use convert;
use math;

# --- options -------------------------------------------------------

/**
 * Page geometry and fonts for a render. `margin` is applied to all four sides;
 * `bodyFont` / `boldFont` / `italicFont` / `monoFont` are standard-14 font names for
 * body text and its inline styles, `headingFont` for headings. `bodySize` is the
 * body point size; heading sizes derive from the heading level.
 * @field pageWidth {int} page width in points (Letter 612, A4 595)
 * @field pageHeight {int} page height in points (Letter 792, A4 842)
 * @field margin {int} margin on every side, in points
 * @field bodyFont {string} standard-14 font for body text
 * @field boldFont {string} standard-14 font for `**strong**`
 * @field italicFont {string} standard-14 font for `*emphasis*`
 * @field monoFont {string} standard-14 font for inline / block code
 * @field headingFont {string} standard-14 font for headings
 * @field bodySize {int} body text point size
 */
export def struct Options {
    pageWidth as int,
    pageHeight as int,
    margin as int,
    bodyFont as string,
    boldFont as string,
    italicFont as string,
    monoFont as string,
    headingFont as string,
    bodySize as int
};

/**
 * The default options: US Letter, 72-point margins, the Helvetica family for body /
 * bold / italic / headings, Courier for code, 11-point body. Copy and tweak fields
 * (value semantics) to customise.
 * @return {Options} the default options
 */
export func defaults() {
    return Options{
        pageWidth: 612,
        pageHeight: 792,
        margin: 72,
        bodyFont: "Helvetica",
        boldFont: "Helvetica-Bold",
        italicFont: "Helvetica-Oblique",
        monoFont: "Courier",
        headingFont: "Helvetica-Bold",
        bodySize: 11
    };
}

# --- internal layout state -----------------------------------------

# One styled word: the word text plus the standard-14 font it renders in. All words
# on a line share a point size, so widths add up without per-word size bookkeeping.
def struct IWord {
    text as string,
    font as string
};

# The running layout: the accumulated Document, the Page being filled, the pen `y`
# (the top of the next line, decreasing down the page), and the current left `x` /
# content `width` (widened / narrowed by lists and quotes).
def struct Layout {
    doc as pdf.Document,
    page as pdf.Page,
    y as int,
    x as int,
    width as int,
    opts as Options
};

# roundPt rounds a measured (float) width to whole points for integer placement.
func roundPt(v as float) {
    return math.round($v);
}

# lineH is the line advance for a point size (1.4x, the usual single-spaced ratio).
func lineH(size as int) {
    return ($size * 7) // 5;
}

# blockGap is the vertical space left after each block.
func blockGap(opts as Options) {
    return ($opts.bodySize * 3) // 5;
}

# headingSize maps a heading level (1-6) to a point size.
func headingSize(level as int, opts as Options) {
    match ($level) {
        when 1 { return 22; }
        when 2 { return 17; }
        when 3 { return 14; }
        when 4 { return 13; }
        when 5 { return 12; }
        else { return $opts.bodySize; }
    }
}

func newLayout(opts as Options) {
    return Layout{
        doc: pdf.document(),
        page: pdf.page($opts.pageWidth, $opts.pageHeight),
        y: $opts.pageHeight - $opts.margin,
        x: $opts.margin,
        width: $opts.pageWidth - 2 * $opts.margin,
        opts: $opts
    };
}

# flushPage finalises the current page into the document and starts a fresh one,
# preserving the current indent (x / width) so a block split across a page break
# keeps its column.
func flushPage(state as Layout) {
    $state.doc = pdf.addPage($state.doc, $state.page);
    $state.page = pdf.page($state.opts.pageWidth, $state.opts.pageHeight);
    $state.y = $state.opts.pageHeight - $state.opts.margin;
    return $state;
}

# ensureSpace starts a new page when `needed` points would not fit above the bottom
# margin - but only when the page already holds content, so an over-tall single line
# still gets placed (overflow) instead of looping.
func ensureSpace(state as Layout, needed as int) {
    def top as int init $state.opts.pageHeight - $state.opts.margin;
    if ($state.y - $needed < $state.opts.margin and $state.y < $top) {
        return flushPage($state);
    }
    return $state;
}

func finish(state as Layout) {
    $state.doc = pdf.addPage($state.doc, $state.page);
    return pdf.render($state.doc);
}

# --- inline layout (styled word wrapping) --------------------------

# fontForInline picks the font for one inline node's kind (text / link / image ->
# body, strong -> bold, emphasis -> italic, codespan -> mono).
func fontForInline(node as markdown.Node, opts as Options) {
    match (markdown.typeOf($node)) {
        when "strong" { return $opts.boldFont; }
        when "emphasis" { return $opts.italicFont; }
        when "codespan" { return $opts.monoFont; }
        else { return $opts.bodyFont; }
    }
}

# inlineText is one inline node's display text; an image renders its alt in brackets.
func inlineText(node as markdown.Node) {
    if (markdown.typeOf($node) == "image") {
        def alt as string init markdown.text($node);
        if (len($alt) == 0) {
            return "[image]";
        }
        return "[" + $alt + "]";
    }
    return markdown.text($node);
}

# inlineWords flattens inline nodes into styled words (split on spaces), each tagged
# with the font its run renders in.
func inlineWords(nodes as list of markdown.Node, opts as Options) {
    def out as list of IWord init [];
    for (def n in $nodes) {
        def f as string init fontForInline($n, $opts);
        for (def w in strings.split(inlineText($n), " ")) {
            if (len($w) > 0) {
                $out[] = IWord{text: $w, font: $f};
            }
        }
    }
    return $out;
}

# packLines greedily packs styled words into lines that fit `maxWidth` at `size`.
func packLines(words as list of IWord, size as int, maxWidth as int, opts as Options) {
    def lines as list of list of IWord init [];
    def cur as list of IWord init [];
    def curW as float init 0.0;
    def sp as float init pdf.measureText($opts.bodyFont, $size, " ");
    for (def w in $words) {
        def ww as float init pdf.measureText($w.font, $size, $w.text);
        def add as float init $ww;
        if (len($cur) > 0) {
            $add = $ww + $sp;
        }
        if (len($cur) > 0 and $curW + $add > $maxWidth) {
            $lines[] = $cur;
            def fresh as list of IWord init [$w];
            $cur = $fresh;
            $curW = $ww;
        } else {
            $cur[] = $w;
            $curW = $curW + $add;
        }
    }
    if (len($cur) > 0) {
        $lines[] = $cur;
    }
    return $lines;
}

# placeInlineLines draws already-packed styled lines at the current pen, advancing y
# and paginating per line.
func placeInlineLines(state as Layout, lines as list of list of IWord, size as int) {
    def lh as int init lineH($size);
    def sp as int init roundPt(pdf.measureText($state.opts.bodyFont, $size, " "));
    for (def line in $lines) {
        $state = ensureSpace($state, $lh);
        def baseline as int init $state.y - $size;
        def cx as int init $state.x;
        def first as bool init true;
        for (def w in $line) {
            if (not $first) {
                $cx = $cx + $sp;
            }
            $first = false;
            $state.page = pdf.text($state.page, $cx, $baseline, $w.font, $size, $w.text);
            $cx = $cx + roundPt(pdf.measureText($w.font, $size, $w.text));
        }
        $state.y = $state.y - $lh;
    }
    return $state;
}

# placePlainLines draws single-font wrapped lines (headings, code) at the current pen.
func placePlainLines(state as Layout, lines as list of string, font as string, size as int, xOffset as int) {
    def lh as int init lineH($size);
    for (def line in $lines) {
        $state = ensureSpace($state, $lh);
        def baseline as int init $state.y - $size;
        $state.page = pdf.text($state.page, $state.x + $xOffset, $baseline, $font, $size, $line);
        $state.y = $state.y - $lh;
    }
    return $state;
}

# --- block renderers ------------------------------------------------

func renderHeading(state as Layout, node as markdown.Node) {
    def size as int init headingSize(markdown.level($node), $state.opts);
    def lines as list of string init pdf.wrapText($state.opts.headingFont, $size, markdown.text($node), $state.width);
    return placePlainLines($state, $lines, $state.opts.headingFont, $size, 0);
}

func renderParagraph(state as Layout, node as markdown.Node) {
    def words as list of IWord init inlineWords(markdown.children($node), $state.opts);
    def lines as list of list of IWord init packLines($words, $state.opts.bodySize, $state.width, $state.opts);
    return placeInlineLines($state, $lines, $state.opts.bodySize);
}

func renderCode(state as Layout, node as markdown.Node) {
    def lines as list of string init strings.split(markdown.text($node), "\n");
    return placePlainLines($state, $lines, $state.opts.monoFont, $state.opts.bodySize, 6);
}

func renderList(state as Layout, node as markdown.Node, depth as int) {
    def ordered as bool init markdown.attr($node, "ordered") == "true";
    def markerW as int init 18;
    def idx as int init 1;
    for (def item in markdown.children($node)) {
        def marker as string init "-";
        if ($ordered) {
            $marker = convert.toString($idx) + ".";
        }
        # Separate the item's inline content from any nested sub-list.
        def inlineKids as list of markdown.Node init [];
        def nested as list of markdown.Node init [];
        for (def c in markdown.children($item)) {
            if (markdown.typeOf($c) == "list") {
                $nested[] = $c;
            } else {
                $inlineKids[] = $c;
            }
        }
        # Place the marker on the item's first line.
        $state = ensureSpace($state, lineH($state.opts.bodySize));
        def baseline as int init $state.y - $state.opts.bodySize;
        $state.page = pdf.text($state.page, $state.x, $baseline, $state.opts.bodyFont, $state.opts.bodySize, $marker);
        # Item text flows in a column indented past the marker, starting on that line.
        def savedX as int init $state.x;
        def savedW as int init $state.width;
        $state.x = $savedX + $markerW;
        $state.width = $savedW - $markerW;
        def words as list of IWord init inlineWords($inlineKids, $state.opts);
        def lines as list of list of IWord init packLines($words, $state.opts.bodySize, $state.width, $state.opts);
        if (len($lines) == 0) {
            $state.y = $state.y - lineH($state.opts.bodySize);
        } else {
            $state = placeInlineLines($state, $lines, $state.opts.bodySize);
        }
        for (def nl in $nested) {
            $state = renderList($state, $nl, $depth + 1);
        }
        $state.x = $savedX;
        $state.width = $savedW;
        $idx = $idx + 1;
    }
    return $state;
}

# The wrapped lines of one table row plus the tallest cell's line count.
def struct RowCells {
    lines as list of list of string,
    maxLines as int
};

# wrapRow word-wraps each cell of a row into `colW`, padding short rows to `ncols`
# empty cells, and reports the tallest cell so the row height can be sized.
func wrapRow(cells as list of markdown.Node, ncols as int, colW as int, pad as int, font as string, size as int) {
    def lines as list of list of string init [];
    def maxLines as int init 1;
    def c as int init 0;
    while ($c < $ncols) {
        def txt as string init "";
        if ($c < len($cells)) {
            $txt = markdown.text($cells[$c]);
        }
        def wl as list of string init pdf.wrapText($font, $size, $txt, $colW - 2 * $pad);
        if (len($wl) == 0) {
            def one as list of string init [""];
            $wl = $one;
        }
        $lines[] = $wl;
        if (len($wl) > $maxLines) {
            $maxLines = len($wl);
        }
        $c = $c + 1;
    }
    return RowCells{lines: $lines, maxLines: $maxLines};
}

# cellX returns the x for one wrapped line within its column, honouring the cell's
# markdown alignment ("right" / "center" / left).
func cellX(cellLeft as int, colW as int, pad as int, align as string, font as string, size as int, line as string) {
    if ($align == "right") {
        return $cellLeft + $colW - $pad - roundPt(pdf.measureText($font, $size, $line));
    }
    if ($align == "center") {
        return $cellLeft + ($colW - roundPt(pdf.measureText($font, $size, $line))) // 2;
    }
    return $cellLeft + $pad;
}

# renderTable draws a GFM table as a ruled grid. The per-cell wrapping (`wrapRow`) and
# per-line x placement (`cellX`) are extracted as pure helpers that take no `Layout`,
# so the row loop mutates one `Layout` frame in place - no full-state copy per row.
func renderTable(state as Layout, node as markdown.Node) {
    def rows as list of markdown.Node init markdown.children($node);
    if (len($rows) == 0) {
        return $state;
    }
    def ncols as int init len(markdown.children($rows[0]));
    if ($ncols == 0) {
        return $state;
    }
    def size as int init $state.opts.bodySize;
    def pad as int init 4;
    def colW as int init $state.width // $ncols;
    def r as int init 0;
    for (def row in $rows) {
        def cells as list of markdown.Node init markdown.children($row);
        def font as string init $state.opts.bodyFont;
        if ($r == 0) {
            $font = $state.opts.boldFont;
        }
        def rc as RowCells init wrapRow($cells, $ncols, $colW, $pad, $font, $size);
        def rowH as int init $rc.maxLines * lineH($size) + 2 * $pad;
        $state = ensureSpace($state, $rowH);
        def rowTop as int init $state.y;
        def rowBot as int init $state.y - $rowH;
        def gridRight as int init $state.x + $ncols * $colW;
        # Cell text, aligned per the column's markdown alignment.
        def c as int init 0;
        while ($c < $ncols) {
            def align as string init "left";
            if ($c < len($cells)) {
                $align = markdown.attr($cells[$c], "align");
            }
            def cellLeft as int init $state.x + $c * $colW;
            def ly as int init $rowTop - $pad - $size;
            for (def ln in $rc.lines[$c]) {
                $state.page = pdf.text($state.page, cellX($cellLeft, $colW, $pad, $align, $font, $size, $ln), $ly, $font, $size, $ln);
                $ly = $ly - lineH($size);
            }
            $c = $c + 1;
        }
        # Grid: the top border once, a bottom border per row, and column verticals.
        if ($r == 0) {
            $state.page = pdf.line($state.page, $state.x, $rowTop, $gridRight, $rowTop);
        }
        $state.page = pdf.line($state.page, $state.x, $rowBot, $gridRight, $rowBot);
        def vc as int init 0;
        while ($vc <= $ncols) {
            def vx as int init $state.x + $vc * $colW;
            $state.page = pdf.line($state.page, $vx, $rowTop, $vx, $rowBot);
            $vc = $vc + 1;
        }
        $state.y = $rowBot;
        $r = $r + 1;
    }
    return $state;
}

func renderQuote(state as Layout, node as markdown.Node, depth as int) {
    def indent as int init 16;
    def savedX as int init $state.x;
    def savedW as int init $state.width;
    $state.x = $savedX + $indent;
    $state.width = $savedW - $indent;
    for (def child in markdown.children($node)) {
        $state = renderBlock($state, $child, $depth + 1);
    }
    $state.x = $savedX;
    $state.width = $savedW;
    return $state;
}

# renderBlock dispatches one block node and leaves a gap after it.
func renderBlock(state as Layout, node as markdown.Node, depth as int) {
    match (markdown.typeOf($node)) {
        when "heading" { $state = renderHeading($state, $node); }
        when "paragraph" { $state = renderParagraph($state, $node); }
        when "list" { $state = renderList($state, $node, $depth); }
        when "code" { $state = renderCode($state, $node); }
        when "table" { $state = renderTable($state, $node); }
        when "quote" { $state = renderQuote($state, $node, $depth); }
        else { return $state; }
    }
    $state.y = $state.y - blockGap($state.opts);
    return $state;
}

# --- entry points ---------------------------------------------------

/**
 * Render a parsed (or hand-built / transformed) `markdown` document tree to PDF bytes
 * with the given options.
 * @param doc {markdown.Node} the document root node from `markdown.parse`
 * @param opts {Options} the page geometry and fonts
 * @return {bytes} the PDF document
 */
export func renderTree(doc as markdown.Node, opts as Options) {
    def state as Layout init newLayout($opts);
    for (def block in markdown.children($doc)) {
        $state = renderBlock($state, $block, 0);
    }
    return finish($state);
}

/**
 * Render a Markdown string to PDF bytes with the given options.
 * @param md {string} the Markdown source
 * @param opts {Options} the page geometry and fonts
 * @return {bytes} the PDF document
 */
export func renderWith(md as string, opts as Options) {
    return renderTree(markdown.parse($md), $opts);
}

/**
 * Render a Markdown string to PDF bytes with the default options.
 * @param md {string} the Markdown source
 * @return {bytes} the PDF document
 */
export func render(md as string) {
    return renderWith($md, defaults());
}
