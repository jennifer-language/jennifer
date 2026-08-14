# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * Data plotting to SVG: turn numbers into a self-contained `<svg>` chart string
 * you can write to a `.svg` file or drop into HTML. A unified `chart` renders
 * one or more `Series` (line / points / both / area, solid or dashed, with
 * optional error bars and marker shapes) with a legend; `line` / `scatter` /
 * `bar` / `bars` / `histogram` are focused wrappers. `bars` groups or stacks
 * multiple series and supports negative (diverging) values. Charts support log
 * scales, a date axis, fonts, configurable margins, positioned legends,
 * reference lines, data labels, and native `<title>` hover tooltips. `save`
 * writes the SVG to a file. Pure Jennifer over `math` and `time` / `fs` /
 * `strings` / `lists` / `convert`; both binaries; the visual companion to the
 * `stats` / `ml` numeric stack.
 * @module plot
 * @example
 * import "plot.j" as plot;
 * def opts as plot.Options init plot.defaults();
 * $opts.title = "growth";
 * def a as plot.Series init plot.series("actual", [1.0, 2.0, 3.0], [1.0, 4.0, 9.0]);
 * def b as plot.Series init plot.series("model", [1.0, 2.0, 3.0], [1.2, 3.6, 9.4]);
 * plot.save(plot.chart([$a, $b], $opts), "growth.svg");
 */
use io;
use math;
use time;
use fs;
use convert;
use strings;
use lists;

# --- reference lines -----------------------------------------------

/**
 * A reference line drawn across the plot at a constant value on one axis - a
 * threshold, target, or marker.
 * @field axis {string} "y" for a horizontal line, "x" for a vertical one
 * @field value {float} the data value the line sits at
 * @field label {string} an optional caption drawn by the line ("" = none)
 * @field color {string} the line colour ("" = a default red)
 * @field dash {bool} dashed (true) or solid
 */
export def struct RefLine { axis as string, value as float, label as string, color as string, dash as bool };

/**
 * A horizontal reference line at y = value (a threshold / target).
 * @param value {float} the y value
 * @param label {string} an optional caption ("" = none)
 * @return {RefLine} the reference line
 */
export func hline(value as float, label as string) {
    return RefLine{axis: "y", value: $value, label: $label, color: "", dash: true};
}

/**
 * A vertical reference line at x = value (a marker in time / along x).
 * @param value {float} the x value
 * @param label {string} an optional caption ("" = none)
 * @return {RefLine} the reference line
 */
export func vline(value as float, label as string) {
    return RefLine{axis: "x", value: $value, label: $label, color: "", dash: true};
}

# --- options + series ----------------------------------------------

/**
 * Chart options: canvas size, captions, colours, fonts, margins, axis modes,
 * legend placement, data labels, tooltips, and reference lines. Copy
 * `defaults()` and set the fields you want.
 * @field width {int} canvas width in pixels
 * @field height {int} canvas height in pixels
 * @field title {string} chart title, centred at the top ("" = none)
 * @field xLabel {string} x-axis caption ("" = none)
 * @field yLabel {string} y-axis caption, rotated ("" = none)
 * @field color {string} the default data colour (single-series line / scatter / bar)
 * @field background {string} the canvas background colour
 * @field fontFamily {string} the CSS font-family for all text
 * @field fontSize {int} the base tick-label size; the title and captions scale off it
 * @field grid {bool} draw gridlines (false = short tick marks instead)
 * @field legend {bool} draw a legend for named / multiple series
 * @field legendPos {string} "top-right" / "top-left" / "bottom-right" / "bottom-left"
 * @field xLog {bool} log10 x scale (values must be positive)
 * @field yLog {bool} log10 y scale (values must be positive)
 * @field xDate {bool} treat x values as Unix seconds and label them as dates
 * @field dateFormat {string} strftime pattern for date labels ("" = chosen by tick spacing)
 * @field marginLeft {int} left margin (room for y labels)
 * @field marginRight {int} right margin
 * @field marginTop {int} top margin (room for the title)
 * @field marginBottom {int} bottom margin (room for x labels)
 * @field barMode {string} "grouped" or "stacked" for a multi-series `bars` chart
 * @field barLabels {bool} draw the value above each grouped bar
 * @field hover {bool} attach `<title>` tooltips to marks (native browser hover)
 * @field refLines {list of RefLine} reference lines drawn over the data
 */
export def struct Options {
    width as int,
    height as int,
    title as string,
    xLabel as string,
    yLabel as string,
    color as string,
    background as string,
    fontFamily as string,
    fontSize as int,
    grid as bool,
    legend as bool,
    legendPos as string,
    xLog as bool,
    yLog as bool,
    xDate as bool,
    dateFormat as string,
    marginLeft as int,
    marginRight as int,
    marginTop as int,
    marginBottom as int,
    barMode as string,
    barLabels as bool,
    hover as bool,
    refLines as list of RefLine
};

def const ML as int init 55;
def const MR as int init 18;
def const MT as int init 38;
def const MB as int init 46;

/**
 * Sensible defaults: a 640x400 canvas, no title / labels, blue data on white, a
 * sans-serif 11px base font, gridlines and a top-right legend on, linear scales,
 * grouped bars, tooltips on, no data labels, no reference lines.
 * @return {Options} the default options
 */
export func defaults() {
    return Options{width: 640, height: 400, title: "", xLabel: "", yLabel: "", color: "#3366cc", background: "#ffffff", fontFamily: "sans-serif", fontSize: 11, grid: true, legend: true, legendPos: "top-right", xLog: false, yLog: false, xDate: false, dateFormat: "", marginLeft: ML, marginRight: MR, marginTop: MT, marginBottom: MB, barMode: "grouped", barLabels: false, hover: true, refLines: []};
}

/**
 * One plotted data series. For line / scatter / area `xs` and `ys` are the
 * coordinates; for `bars` only `ys` (the per-category values) is used. `yErr`,
 * when the same length as `ys`, draws symmetric error bars (linear y only);
 * `shape` picks the scatter marker.
 * @field name {string} the legend label ("" = not listed)
 * @field xs {list of float} the x coordinates (unused by `bars`)
 * @field ys {list of float} the y coordinates / bar values
 * @field color {string} the series colour ("" = auto from the palette)
 * @field mark {string} "line" / "points" / "both" / "area"
 * @field dash {bool} draw the line dashed
 * @field yErr {list of float} symmetric +/- error per point ([] = none)
 * @field shape {string} marker for points: "circle" / "square" / "triangle" / "diamond"
 */
export def struct Series { name as string, xs as list of float, ys as list of float, color as string, mark as string, dash as bool, yErr as list of float, shape as string };

/**
 * A line series (mark "line"). Set `.mark` / `.color` / `.dash` / `.yErr` /
 * `.shape` on the result.
 * @param name {string} the legend label
 * @param xs {list of float} the x coordinates
 * @param ys {list of float} the y coordinates
 * @return {Series} the series
 */
export func series(name as string, xs as list of float, ys as list of float) {
    return Series{name: $name, xs: $xs, ys: $ys, color: "", mark: "line", dash: false, yErr: [], shape: "circle"};
}

/**
 * A points (scatter) series (mark "points").
 * @param name {string} the legend label
 * @param xs {list of float} the x coordinates
 * @param ys {list of float} the y coordinates
 * @return {Series} the series
 */
export func points(name as string, xs as list of float, ys as list of float) {
    return Series{name: $name, xs: $xs, ys: $ys, color: "", mark: "points", dash: false, yErr: [], shape: "circle"};
}

/**
 * Convert a `list of int` to a `list of float`, since the chart functions take
 * float data and an int list does not match a float parameter.
 * @param xs {list of int} the integers
 * @return {list of float} the same values as floats
 */
export func floats(xs as list of int) {
    def out as list of float init [];
    for (def x in $xs) {
        $out[] = convert.toFloat($x);
    }
    return $out;
}

# --- geometry + palette (private) ----------------------------------

def struct Geom { x0 as int, y0 as int, x1 as int, y1 as int };

func geomOf(opts as Options) {
    return Geom{x0: $opts.marginLeft, y0: $opts.marginTop, x1: $opts.width - $opts.marginRight, y1: $opts.height - $opts.marginBottom};
}

func paletteColor(i as int) {
    def pal as list of string init ["#3366cc", "#dc3912", "#109618", "#ff9900", "#990099", "#0099c6", "#dd4477", "#66aa00"];
    return $pal[$i % len($pal)];
}

func sx(g as Geom, v as float, lo as float, hi as float, isLog as bool) {
    # tfm is inlined here (and in sy) instead of called: these two are the
    # per-point geometry hot path, and a linear axis makes the call a no-op.
    def tlo as float init $lo;
    def thi as float init $hi;
    if ($isLog) {
        $tlo = math.log10($lo);
        $thi = math.log10($hi);
    }
    if ($thi == $tlo) {
        return convert.toFloat($g.x0 + $g.x1) / 2.0;
    }
    def tv as float init $v;
    if ($isLog) {
        $tv = math.log10($v);
    }
    return convert.toFloat($g.x0) + ($tv - $tlo) / ($thi - $tlo) * convert.toFloat($g.x1 - $g.x0);
}

func sy(g as Geom, v as float, lo as float, hi as float, isLog as bool) {
    def tlo as float init $lo;
    def thi as float init $hi;
    if ($isLog) {
        $tlo = math.log10($lo);
        $thi = math.log10($hi);
    }
    if ($thi == $tlo) {
        return convert.toFloat($g.y0 + $g.y1) / 2.0;
    }
    def tv as float init $v;
    if ($isLog) {
        $tv = math.log10($v);
    }
    return convert.toFloat($g.y0) + ($thi - $tv) / ($thi - $tlo) * convert.toFloat($g.y1 - $g.y0);
}

# --- numeric + string helpers (private) ----------------------------

func minOf(xs as list of float) {
    def m as float init $xs[0];
    for (def v in $xs) {
        if ($v < $m) {
            $m = $v;
        }
    }
    return $m;
}

func maxOf(xs as list of float) {
    def m as float init $xs[0];
    for (def v in $xs) {
        if ($v > $m) {
            $m = $v;
        }
    }
    return $m;
}

func num(x as float) {
    # A magnitude at or beyond 2^63 overflows math.round (int range); render it in
    # the float's own compact scientific form instead of crashing (DF-002).
    def r as int init 0;
    try {
        $r = math.round($x);
    } catch (e) {
        return convert.toString($x);
    }
    if (math.abs($x - convert.toFloat($r)) < 0.001) {
        return convert.toString($r);
    }
    return io.sprintf("%f|prec=2", $x);
}

func svgEsc(s as string) {
    # Fast path: most strings (colours, tooltips) carry no special char, and the
    # three replaces below would otherwise scan and reallocate for each one.
    # indexOf skips the allocation when there is nothing to escape.
    if (strings.indexOf($s, "&") < 0 and strings.indexOf($s, "<") < 0 and
        strings.indexOf($s, ">") < 0) {
        return $s;
    }
    def out as string init strings.replace($s, "&", "&amp;");
    $out = strings.replace($out, "<", "&lt;");
    $out = strings.replace($out, ">", "&gt;");
    return $out;
}

# attrEsc escapes a string for use inside a double-quoted SVG attribute value:
# svgEsc plus the double-quote, so a user-supplied colour / font-family / label
# can never break out of the attribute and inject markup (DF-001). Applied to
# every attribute slot fed a caller-controlled value; a palette / literal colour
# passes through unchanged.
func attrEsc(s as string) {
    if (strings.indexOf($s, "&") < 0 and strings.indexOf($s, "<") < 0 and
        strings.indexOf($s, ">") < 0 and strings.indexOf($s, '"') < 0) {
        return $s;
    }
    return strings.replace(svgEsc($s), '"', "&quot;");
}

# capName bounds a per-mark tooltip string so a pathological series name cannot
# amplify output by (name-length x mark-count) (DF-004).
def const MAX_TOOLTIP as int init 64;

func capName(s as string) {
    if (len($s) > MAX_TOOLTIP) {
        return strings.substring($s, 0, MAX_TOOLTIP) + "...";
    }
    return $s;
}

# The upper bound on histogram buckets: one int argument drives both the bucket
# list allocation and the bar-element count, so it is capped to keep a stray
# large value from a fatal, uncatchable OOM (DF-003).
def const MAX_BINS as int init 10000;

# titleChild returns a <title> child (a native browser tooltip) or "" when hover
# is off or the text is empty.
func titleChild(hover as bool, text as string) {
    if ($hover and $text != "") {
        return '<title>' + svgEsc($text) + '</title>';
    }
    return "";
}

# elemWithTitle emits a shape element, self-closing when it has no <title> child.
func elemWithTitle(tag as string, attrs as string, title as string) {
    if ($title == "") {
        return '<' + $tag + ' ' + $attrs + '/>';
    }
    return '<' + $tag + ' ' + $attrs + '>' + $title + '</' + $tag + '>';
}

func lineEl(x1 as float, y1 as float, x2 as float, y2 as float, stroke as string, w as string) {
    return '<line x1="' + num($x1) + '" y1="' + num($y1) + '" x2="' + num($x2) + '" y2="' + num($y2) + '" stroke="' + attrEsc($stroke) + '" stroke-width="' + $w + '"/>';
}

func labelText(opts as Options, x as float, y as float, anchor as string, size as int, weight as string, fill as string, s as string) {
    return '<text x="' + num($x) + '" y="' + num($y) + '" text-anchor="' + $anchor + '" font-family="' + attrEsc($opts.fontFamily) + '" font-size="' + convert.toString($size) + '" font-weight="' + $weight + '" fill="' + attrEsc($fill) + '">' + svgEsc($s) + '</text>';
}

# markerEl draws a scatter marker, with an optional tooltip. cxs / cys are the
# centre coordinates already run through num() by the caller - the marker loop
# would otherwise format each point twice (once for the line, once here).
func markerEl(
    shape as string,
    cxs as string,
    cys as string,
    cx as float,
    cy as float,
    r as float,
    col as string,
    title as string) {
    if ($shape == "square") {
        return elemWithTitle(
            "rect",
            'x="' + num($cx - $r) + '" y="' + num($cy - $r) + '" width="' + num($r * 2.0) +
                '" height="' + num($r * 2.0) + '" fill="' + attrEsc($col) + '"',
            $title);
    }
    if ($shape == "triangle") {
        def p as string init $cxs + ',' + num($cy - $r) + ' ' + num($cx - $r) + ',' +
            num($cy + $r) + ' ' + num($cx + $r) + ',' + num($cy + $r);
        return elemWithTitle("polygon", 'points="' + $p + '" fill="' + attrEsc($col) + '"', $title);
    }
    if ($shape == "diamond") {
        def p as string init $cxs + ',' + num($cy - $r) + ' ' + num($cx + $r) + ',' + $cys + ' ' +
            $cxs + ',' + num($cy + $r) + ' ' + num($cx - $r) + ',' + $cys;
        return elemWithTitle("polygon", 'points="' + $p + '" fill="' + attrEsc($col) + '"', $title);
    }
    return elemWithTitle(
        "circle",
        'cx="' + $cxs + '" cy="' + $cys + '" r="' + num($r) + '" fill="' + attrEsc($col) + '"',
        $title);
}

func barRect(x as float, yTop as float, w as float, h as float, col as string, title as string) {
    return elemWithTitle("rect", 'x="' + num($x) + '" y="' + num($yTop) + '" width="' + num($w) + '" height="' + num($h) + '" fill="' + attrEsc($col) + '"', $title);
}

# --- axes ----------------------------------------------------------

def struct Axis { ticks as list of float, labels as list of string, lo as float, hi as float, isLog as bool };

func niceNum(x as float, round as bool) {
    def expv as int init math.floor(math.log10($x));
    def base as float init math.pow(10.0, convert.toFloat($expv));
    def f as float init $x / $base;
    def nf as float init 10.0;
    if ($round) {
        if ($f < 1.5) {
            $nf = 1.0;
        } elseif ($f < 3.0) {
            $nf = 2.0;
        } elseif ($f < 7.0) {
            $nf = 5.0;
        }
    } else {
        if ($f <= 1.0) {
            $nf = 1.0;
        } elseif ($f <= 2.0) {
            $nf = 2.0;
        } elseif ($f <= 5.0) {
            $nf = 5.0;
        }
    }
    return $nf * $base;
}

func niceTicks(lo as float, hi as float, count as int) {
    def top as float init $hi;
    if ($top <= $lo) {
        # Expand a flat range so the span is non-zero. At a large magnitude a
        # `+ 1.0` bump is below the float ULP and collapses back to `lo` (feeding
        # log10(0) downstream), so scale the bump to the magnitude (DF-002).
        def bump as float init 1.0;
        if (math.abs($lo) > 1000.0) {
            $bump = math.abs($lo) * 0.001;
        }
        $top = $lo + $bump;
    }
    def spacing as float init niceNum(niceNum($top - $lo, false) / convert.toFloat($count - 1), true);
    def niceMin as float init convert.toFloat(math.floor($lo / $spacing)) * $spacing;
    def niceMax as float init convert.toFloat(math.ceil($top / $spacing)) * $spacing;
    def ticks as list of float init [];
    def t as float init $niceMin;
    def guard as int init 0;
    while ($t <= $niceMax + $spacing * 0.001 and $guard < 1000) {
        $ticks[] = $t;
        $t = $t + $spacing;
        $guard = $guard + 1;
    }
    return $ticks;
}

func linearAxis(dmin as float, dmax as float) {
    def ticks as list of float init niceTicks($dmin, $dmax, 6);
    def labels as list of string init [];
    for (def i in 0..len($ticks)) {
        $labels[] = num($ticks[$i]);
    }
    return Axis{ticks: $ticks, labels: $labels, lo: $ticks[0], hi: $ticks[len($ticks) - 1], isLog: false};
}

func logAxis(dmin as float, dmax as float) {
    if ($dmin <= 0.0) {
        throw Error{kind: "plot", message: "plot: a log axis needs strictly positive values", file: "", line: 0, col: 0};
    }
    def loExp as int init math.floor(math.log10($dmin));
    def hiExp as int init math.ceil(math.log10($dmax));
    if ($hiExp <= $loExp) {
        $hiExp = $loExp + 1;
    }
    def ticks as list of float init [];
    def labels as list of string init [];
    for (def k in $loExp..($hiExp + 1)) {
        def v as float init math.pow(10.0, convert.toFloat($k));
        $ticks[] = $v;
        $labels[] = num($v);
    }
    return Axis{ticks: $ticks, labels: $labels, lo: math.pow(10.0, convert.toFloat($loExp)), hi: math.pow(10.0, convert.toFloat($hiExp)), isLog: true};
}

func dateStep(span as int, count as int) {
    def steps as list of int init [1, 2, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200, 10800, 21600, 43200, 86400, 172800, 604800, 2592000, 7776000, 31536000, 63072000];
    def target as int init $span // $count;
    for (def i in 0..len($steps)) {
        if ($steps[$i] >= $target) {
            return $steps[$i];
        }
    }
    return $steps[len($steps) - 1];
}

func datePattern(step as int) {
    if ($step < 3600) {
        return "%H:%M";
    }
    if ($step < 86400) {
        return "%m-%d %H:%M";
    }
    if ($step < 2592000) {
        return "%Y-%m-%d";
    }
    return "%Y-%m";
}

func dateAxis(dmin as float, dmax as float, count as int, fmt as string) {
    def loSec as int init convert.toInt($dmin);
    def hiSec as int init convert.toInt($dmax);
    def span as int init $hiSec - $loSec;
    if ($span <= 0) {
        $span = 1;
    }
    def step as int init dateStep($span, $count);
    def pattern as string init $fmt;
    if ($pattern == "") {
        $pattern = datePattern($step);
    }
    def first as int init (($loSec + $step - 1) // $step) * $step;
    def ticks as list of float init [];
    def labels as list of string init [];
    def t as int init $first;
    def guard as int init 0;
    while ($t <= $hiSec and $guard < 1000) {
        $ticks[] = convert.toFloat($t);
        $labels[] = time.format(time.fromUnix($t), $pattern);
        $t = $t + $step;
        $guard = $guard + 1;
    }
    return Axis{ticks: $ticks, labels: $labels, lo: $dmin, hi: $dmax, isLog: false};
}

func makeAxis(dmin as float, dmax as float, isLog as bool, isDate as bool, fmt as string) {
    if ($isDate) {
        return dateAxis($dmin, $dmax, 6, $fmt);
    }
    if ($isLog) {
        return logAxis($dmin, $dmax);
    }
    return linearAxis($dmin, $dmax);
}

# --- frame + axis drawing (private) --------------------------------

func frameCommon(g as Geom, opts as Options) {
    def out as list of string init [];
    $out[] = '<rect x="0" y="0" width="' + convert.toString($opts.width) + '" height="' + convert.toString($opts.height) + '" fill="' + attrEsc($opts.background) + '"/>';
    $out[] = '<rect x="' + convert.toString($g.x0) + '" y="' + convert.toString($g.y0) + '" width="' + convert.toString($g.x1 - $g.x0) + '" height="' + convert.toString($g.y1 - $g.y0) + '" fill="none" stroke="#cccccc" stroke-width="1"/>';
    if ($opts.title != "") {
        $out[] = labelText($opts, convert.toFloat($opts.width // 2), 22.0, "middle", $opts.fontSize + 4, "bold", "#222222", $opts.title);
    }
    if ($opts.xLabel != "") {
        $out[] = labelText($opts, convert.toFloat(($g.x0 + $g.x1) // 2), convert.toFloat($opts.height - 8), "middle", $opts.fontSize + 1, "normal", "#333333", $opts.xLabel);
    }
    if ($opts.yLabel != "") {
        def cy as int init ($g.y0 + $g.y1) // 2;
        $out[] = '<text x="14" y="' + convert.toString($cy) + '" text-anchor="middle" font-family="' + attrEsc($opts.fontFamily) + '" font-size="' + convert.toString($opts.fontSize + 1) + '" fill="#333333" transform="rotate(-90 14 ' + convert.toString($cy) + ')">' + svgEsc($opts.yLabel) + '</text>';
    }
    return $out;
}

func drawYAxis(g as Geom, opts as Options, ax as Axis) {
    def out as list of string init [];
    for (def i in 0..len($ax.ticks)) {
        def y as float init sy($g, $ax.ticks[$i], $ax.lo, $ax.hi, $ax.isLog);
        if ($opts.grid) {
            $out[] = lineEl(convert.toFloat($g.x0), $y, convert.toFloat($g.x1), $y, "#eeeeee", "1");
        } else {
            $out[] = lineEl(convert.toFloat($g.x0 - 4), $y, convert.toFloat($g.x0), $y, "#999999", "1");
        }
        $out[] = labelText($opts, convert.toFloat($g.x0 - 6), $y + 4.0, "end", $opts.fontSize, "normal", "#333333", $ax.labels[$i]);
    }
    return $out;
}

func drawXAxis(g as Geom, opts as Options, ax as Axis) {
    def out as list of string init [];
    for (def i in 0..len($ax.ticks)) {
        def x as float init sx($g, $ax.ticks[$i], $ax.lo, $ax.hi, $ax.isLog);
        if ($opts.grid) {
            $out[] = lineEl($x, convert.toFloat($g.y0), $x, convert.toFloat($g.y1), "#eeeeee", "1");
        } else {
            $out[] = lineEl($x, convert.toFloat($g.y1), $x, convert.toFloat($g.y1 + 4), "#999999", "1");
        }
        $out[] = labelText($opts, $x, convert.toFloat($g.y1 + 16), "middle", $opts.fontSize, "normal", "#333333", $ax.labels[$i]);
    }
    return $out;
}

func svgWrap(opts as Options, body as list of string) {
    def head as string init '<svg xmlns="http://www.w3.org/2000/svg" width="' + convert.toString($opts.width) + '" height="' + convert.toString($opts.height) + '" viewBox="0 0 ' + convert.toString($opts.width) + ' ' + convert.toString($opts.height) + '">';
    def parts as list of string init [$head];
    $parts = lists.concat($parts, $body);
    $parts[] = '</svg>';
    return strings.join($parts, "\n") + "\n";
}

# --- series + legend + reference lines (private) -------------------

func dashAttr(dash as bool, pattern as string) {
    if ($dash) {
        return ' stroke-dasharray="' + $pattern + '"';
    }
    return "";
}

# pointTitle is the tooltip text for a data point: "name: y" for a named series,
# else "(x, y)".
func pointTitle(s as Series, x as float, y as float) {
    if ($s.name != "") {
        return capName($s.name) + ": " + num($y);
    }
    return "(" + num($x) + ", " + num($y) + ")";
}

func errorBars(g as Geom, s as Series, col as string, xa as Axis, ya as Axis) {
    def out as list of string init [];
    for (def i in 0..len($s.ys)) {
        def x as float init sx($g, $s.xs[$i], $xa.lo, $xa.hi, $xa.isLog);
        def e as float init $s.yErr[$i];
        def yHi as float init sy($g, $s.ys[$i] + $e, $ya.lo, $ya.hi, false);
        def yLo as float init sy($g, $s.ys[$i] - $e, $ya.lo, $ya.hi, false);
        $out[] = lineEl($x, $yLo, $x, $yHi, $col, "1");
        $out[] = lineEl($x - 3.0, $yLo, $x + 3.0, $yLo, $col, "1");
        $out[] = lineEl($x - 3.0, $yHi, $x + 3.0, $yHi, $col, "1");
    }
    return $out;
}

func drawSeries(g as Geom, s as Series, col as string, xa as Axis, ya as Axis, opts as Options) {
    # One pass over the points computes each coordinate pair once and formats it
    # once; the "both" / "area" marks used to re-run sx/sy and num() in a second
    # loop, doubling the per-point work (the hot path of every line chart).
    def out as list of string init [];
    def n as int init len($s.xs);
    def haveLine as bool init $s.mark == "line" or $s.mark == "both" or $s.mark == "area";
    def havePts as bool init $s.mark == "points" or $s.mark == "both";
    def coords as list of string init [];
    def markers as list of string init [];
    if ($haveLine) {
        for (def i in 0..$n) {
            def cx as float init sx($g, $s.xs[$i], $xa.lo, $xa.hi, $xa.isLog);
            def cy as float init sy($g, $s.ys[$i], $ya.lo, $ya.hi, $ya.isLog);
            def cxs as string init num($cx);
            def cys as string init num($cy);
            $coords[] = $cxs + ',' + $cys;
            if ($havePts) {
                def mt as string init "";
                if ($opts.hover) {
                    $mt = titleChild(true, pointTitle($s, $s.xs[$i], $s.ys[$i]));
                }
                $markers[] = markerEl($s.shape, $cxs, $cys, $cx, $cy, 3.5, $col, $mt);
            }
        }
        if ($s.mark == "area") {
            def base as float init sy($g, $ya.lo, $ya.lo, $ya.hi, $ya.isLog);
            def poly as list of string init [
                num(sx($g, $s.xs[0], $xa.lo, $xa.hi, $xa.isLog)) + ',' + num($base)
            ];
            $poly = lists.concat($poly, $coords);
            $poly[] = num(sx($g, $s.xs[$n - 1], $xa.lo, $xa.hi, $xa.isLog)) + ',' + num($base);
            $out[] = '<polygon fill="' + attrEsc($col) +
                '" fill-opacity="0.25" stroke="none" points="' + strings.join($poly, " ") + '"/>';
        }
        $out[] = '<polyline fill="none" stroke="' + attrEsc($col) + '" stroke-width="2"' +
            dashAttr($s.dash, "6,4") + ' points="' + strings.join($coords, " ") + '">' +
            titleChild($opts.hover, $s.name) + '</polyline>';
    } elseif ($havePts) {
        for (def i in 0..$n) {
            def cx as float init sx($g, $s.xs[$i], $xa.lo, $xa.hi, $xa.isLog);
            def cy as float init sy($g, $s.ys[$i], $ya.lo, $ya.hi, $ya.isLog);
            def cxs as string init num($cx);
            def cys as string init num($cy);
            def mt as string init "";
            if ($opts.hover) {
                $mt = titleChild(true, pointTitle($s, $s.xs[$i], $s.ys[$i]));
            }
            $markers[] = markerEl($s.shape, $cxs, $cys, $cx, $cy, 3.5, $col, $mt);
        }
    }
    if (len($markers) > 0) {
        $out = lists.concat($out, $markers);
    }
    if (len($s.yErr) == len($s.ys) and len($s.yErr) > 0 and not $ya.isLog) {
        $out = lists.concat($out, errorBars($g, $s, $col, $xa, $ya));
    }
    return $out;
}

func wantsLegend(data as list of Series) {
    if (len($data) > 1) {
        return true;
    }
    if (len($data) == 1 and $data[0].name != "") {
        return true;
    }
    return false;
}

func legendBox(g as Geom, data as list of Series, opts as Options) {
    def out as list of string init [];
    def maxLen as int init 0;
    for (def i in 0..len($data)) {
        if (len($data[$i].name) > $maxLen) {
            $maxLen = len($data[$i].name);
        }
    }
    def lw as int init 28 + convert.toInt(convert.toFloat($maxLen) * convert.toFloat($opts.fontSize) * 0.6);
    def lh as int init len($data) * ($opts.fontSize + 7) + 8;
    def bx as int init $g.x1 - $lw - 6;
    def by as int init $g.y0 + 8;
    if ($opts.legendPos == "top-left") {
        $bx = $g.x0 + 6;
    } elseif ($opts.legendPos == "bottom-right") {
        $by = $g.y1 - $lh - 6;
    } elseif ($opts.legendPos == "bottom-left") {
        $bx = $g.x0 + 6;
        $by = $g.y1 - $lh - 6;
    }
    $out[] = '<rect x="' + convert.toString($bx) + '" y="' + convert.toString($by) + '" width="' + convert.toString($lw) + '" height="' + convert.toString($lh) + '" fill="#ffffff" fill-opacity="0.85" stroke="#cccccc"/>';
    for (def i in 0..len($data)) {
        def col as string init $data[$i].color;
        if ($col == "") {
            $col = paletteColor($i);
        }
        def rowY as int init $by + 8 + $i * ($opts.fontSize + 7);
        $out[] = '<rect x="' + convert.toString($bx + 8) + '" y="' + convert.toString($rowY) + '" width="' + convert.toString($opts.fontSize) + '" height="' + convert.toString($opts.fontSize) + '" fill="' + attrEsc($col) + '"/>';
        $out[] = labelText($opts, convert.toFloat($bx + 12 + $opts.fontSize), convert.toFloat($rowY + $opts.fontSize - 1), "start", $opts.fontSize, "normal", "#333333", $data[$i].name);
    }
    return $out;
}

func drawRefLines(g as Geom, opts as Options, xa as Axis, ya as Axis, numericX as bool) {
    def out as list of string init [];
    for (def i in 0..len($opts.refLines)) {
        def r as RefLine init $opts.refLines[$i];
        def col as string init $r.color;
        if ($col == "") {
            $col = "#cc0000";
        }
        def dash as string init dashAttr($r.dash, "5,4");
        if ($r.axis == "y" and not ($ya.isLog and $r.value <= 0.0)) {
            def y as float init sy($g, $r.value, $ya.lo, $ya.hi, $ya.isLog);
            $out[] = '<line x1="' + convert.toString($g.x0) + '" y1="' + num($y) + '" x2="' + convert.toString($g.x1) + '" y2="' + num($y) + '" stroke="' + attrEsc($col) + '" stroke-width="1"' + $dash + '/>';
            if ($r.label != "") {
                $out[] = labelText($opts, convert.toFloat($g.x1 - 4), $y - 3.0, "end", $opts.fontSize, "normal", $col, $r.label);
            }
        } elseif ($r.axis == "x" and $numericX and not ($xa.isLog and $r.value <= 0.0)) {
            def x as float init sx($g, $r.value, $xa.lo, $xa.hi, $xa.isLog);
            $out[] = '<line x1="' + num($x) + '" y1="' + convert.toString($g.y0) + '" x2="' + num($x) + '" y2="' + convert.toString($g.y1) + '" stroke="' + attrEsc($col) + '" stroke-width="1"' + $dash + '/>';
            if ($r.label != "") {
                $out[] = labelText($opts, $x + 3.0, convert.toFloat($g.y0 + 10), "start", $opts.fontSize, "normal", $col, $r.label);
            }
        }
    }
    return $out;
}

# --- charts (exported) ---------------------------------------------

func requireXY(kind as string, xs as list of float, ys as list of float) {
    if (len($xs) == 0 or len($xs) != len($ys)) {
        throw Error{kind: "plot", message: "plot." + $kind + ": xs and ys must be non-empty and the same length", file: "", line: 0, col: 0};
    }
}

/**
 * Render one or more `Series` on shared axes, with a legend for named series.
 * Each series draws per its `mark` (line / points / both / area), with optional
 * dashing, marker shapes, and error bars. The domain spans every series, its
 * error bars, and any reference-line value on a linear axis.
 * @param data {list of Series} the series to plot (each non-empty, xs / ys equal length)
 * @param opts {Options} chart options
 * @return {string} a complete SVG document
 * @throws {Error} kind "plot" when data is empty, a series is malformed, or a log axis sees a non-positive value
 */
export func chart(data as list of Series, opts as Options) {
    if (len($data) == 0) {
        throw Error{kind: "plot", message: "plot.chart: no series to plot", file: "", line: 0, col: 0};
    }
    # Validate the first series before seeding the extrema from its [0] elements,
    # so a malformed first series gives the friendly message, not a raw index
    # error (DF-005). The per-series loop re-checks every series.
    requireXY("chart", $data[0].xs, $data[0].ys);
    def xmin as float init $data[0].xs[0];
    def xmax as float init $data[0].xs[0];
    def ymin as float init $data[0].ys[0];
    def ymax as float init $data[0].ys[0];
    for (def si in 0..len($data)) {
        requireXY("chart", $data[$si].xs, $data[$si].ys);
        def a as float init minOf($data[$si].xs);
        def b as float init maxOf($data[$si].xs);
        if ($a < $xmin) {
            $xmin = $a;
        }
        if ($b > $xmax) {
            $xmax = $b;
        }
        for (def j in 0..len($data[$si].ys)) {
            def lo as float init $data[$si].ys[$j];
            def hi as float init $data[$si].ys[$j];
            if (len($data[$si].yErr) == len($data[$si].ys) and not $opts.yLog) {
                $lo = $lo - $data[$si].yErr[$j];
                $hi = $hi + $data[$si].yErr[$j];
            }
            if ($lo < $ymin) {
                $ymin = $lo;
            }
            if ($hi > $ymax) {
                $ymax = $hi;
            }
        }
    }
    for (def i in 0..len($opts.refLines)) {
        def r as RefLine init $opts.refLines[$i];
        if ($r.axis == "y" and not $opts.yLog) {
            if ($r.value < $ymin) {
                $ymin = $r.value;
            }
            if ($r.value > $ymax) {
                $ymax = $r.value;
            }
        } elseif ($r.axis == "x" and not $opts.xLog and not $opts.xDate) {
            if ($r.value < $xmin) {
                $xmin = $r.value;
            }
            if ($r.value > $xmax) {
                $xmax = $r.value;
            }
        }
    }
    def g as Geom init geomOf($opts);
    def xa as Axis init makeAxis($xmin, $xmax, $opts.xLog, $opts.xDate, $opts.dateFormat);
    def ya as Axis init makeAxis($ymin, $ymax, $opts.yLog, false, "");
    def out as list of string init frameCommon($g, $opts);
    $out = lists.concat($out, drawYAxis($g, $opts, $ya));
    $out = lists.concat($out, drawXAxis($g, $opts, $xa));
    for (def si in 0..len($data)) {
        def col as string init $data[$si].color;
        if ($col == "") {
            $col = paletteColor($si);
        }
        $out = lists.concat($out, drawSeries($g, $data[$si], $col, $xa, $ya, $opts));
    }
    $out = lists.concat($out, drawRefLines($g, $opts, $xa, $ya, true));
    if ($opts.legend and wantsLegend($data)) {
        $out = lists.concat($out, legendBox($g, $data, $opts));
    }
    return svgWrap($opts, $out);
}

/**
 * A line chart of a single series. A convenience over `chart`.
 * @param xs {list of float} the x coordinates
 * @param ys {list of float} the y coordinates
 * @param opts {Options} chart options
 * @return {string} a complete SVG document
 * @throws {Error} kind "plot" when xs / ys are empty or unequal length
 */
export func line(xs as list of float, ys as list of float, opts as Options) {
    requireXY("line", $xs, $ys);
    def s as Series init Series{name: "", xs: $xs, ys: $ys, color: $opts.color, mark: "line", dash: false, yErr: [], shape: "circle"};
    return chart([$s], $opts);
}

/**
 * A scatter plot of a single series. A convenience over `chart`.
 * @param xs {list of float} the x coordinates
 * @param ys {list of float} the y coordinates
 * @param opts {Options} chart options
 * @return {string} a complete SVG document
 * @throws {Error} kind "plot" when xs / ys are empty or unequal length
 */
export func scatter(xs as list of float, ys as list of float, opts as Options) {
    requireXY("scatter", $xs, $ys);
    def s as Series init Series{name: "", xs: $xs, ys: $ys, color: $opts.color, mark: "points", dash: false, yErr: [], shape: "circle"};
    return chart([$s], $opts);
}

# barSpan returns [posSum, negSum] of one category across the series.
func barSpan(data as list of Series, i as int) {
    def pos as float init 0.0;
    def neg as float init 0.0;
    for (def si in 0..len($data)) {
        def v as float init $data[$si].ys[$i];
        if ($v >= 0.0) {
            $pos = $pos + $v;
        } else {
            $neg = $neg + $v;
        }
    }
    return [$pos, $neg];
}

# barGrouped draws one category's series bars side by side, from the zero
# baseline (up for positive values, down for negative).
func barGrouped(g as Geom, data as list of Series, ya as Axis, opts as Options, label as string, cx as float, slot as float, i as int) {
    def out as list of string init [];
    def m as int init len($data);
    def baseline as float init sy($g, 0.0, $ya.lo, $ya.hi, false);
    def gw as float init $slot * 0.7;
    def sub as float init $gw / convert.toFloat($m);
    for (def si in 0..$m) {
        def col as string init $data[$si].color;
        if ($col == "") {
            $col = paletteColor($si);
        }
        def v as float init $data[$si].ys[$i];
        def yv as float init sy($g, $v, $ya.lo, $ya.hi, false);
        def yTop as float init $baseline;
        def h as float init $yv - $baseline;
        if ($yv < $baseline) {
            $yTop = $yv;
            $h = $baseline - $yv;
        }
        def bx as float init $cx - $gw / 2.0 + $sub * convert.toFloat($si);
        def nm as string init $data[$si].name;
        if ($nm == "") {
            $nm = $label;
        }
        $out[] = barRect($bx, $yTop, $sub * 0.9, $h, $col, titleChild($opts.hover, capName($nm) + ": " + num($v)));
        if ($opts.barLabels) {
            def ly as float init $yTop - 3.0;
            if ($v < 0.0) {
                $ly = $yTop + $h + convert.toFloat($opts.fontSize);
            }
            $out[] = labelText($opts, $bx + $sub * 0.45, $ly, "middle", $opts.fontSize - 1, "normal", "#333333", num($v));
        }
    }
    return $out;
}

# barStacked draws one category's series stacked from zero: positive segments up,
# negative segments down.
func barStacked(g as Geom, data as list of Series, ya as Axis, opts as Options, label as string, cx as float, slot as float, i as int) {
    def out as list of string init [];
    def gw as float init $slot * 0.7;
    def accPos as float init 0.0;
    def accNeg as float init 0.0;
    for (def si in 0..len($data)) {
        def col as string init $data[$si].color;
        if ($col == "") {
            $col = paletteColor($si);
        }
        def v as float init $data[$si].ys[$i];
        def top as float init 0.0;
        def h as float init 0.0;
        if ($v >= 0.0) {
            def yTop as float init sy($g, $accPos + $v, $ya.lo, $ya.hi, false);
            $top = $yTop;
            $h = sy($g, $accPos, $ya.lo, $ya.hi, false) - $yTop;
            $accPos = $accPos + $v;
        } else {
            def yTop as float init sy($g, $accNeg, $ya.lo, $ya.hi, false);
            $top = $yTop;
            $h = sy($g, $accNeg + $v, $ya.lo, $ya.hi, false) - $yTop;
            $accNeg = $accNeg + $v;
        }
        def nm as string init $data[$si].name;
        if ($nm == "") {
            $nm = $label;
        }
        $out[] = barRect($cx - $gw / 2.0, $top, $gw, $h, $col, titleChild($opts.hover, capName($nm) + ": " + num($v)));
    }
    return $out;
}

/**
 * A multi-series bar chart: one group of bars per labelled category. Each
 * `Series` contributes its `ys` (one value per category) as a colour; series
 * are drawn side by side ("grouped", `opts.barMode`) or on top of each other
 * ("stacked"). Values may be negative (bars diverge from a zero baseline). A
 * named series is listed in the legend.
 * @param labels {list of string} the category labels, drawn under each group
 * @param data {list of Series} the series (each `ys` the same length as `labels`)
 * @param opts {Options} chart options
 * @return {string} a complete SVG document
 * @throws {Error} kind "plot" when labels / data are empty or a series length differs
 */
export func bars(labels as list of string, data as list of Series, opts as Options) {
    if (len($labels) == 0 or len($data) == 0) {
        throw Error{kind: "plot", message: "plot.bars: labels and data must be non-empty", file: "", line: 0, col: 0};
    }
    def n as int init len($labels);
    for (def si in 0..len($data)) {
        if (len($data[$si].ys) != $n) {
            throw Error{kind: "plot", message: "plot.bars: every series must have one value per label", file: "", line: 0, col: 0};
        }
    }
    def stacked as bool init $opts.barMode == "stacked";
    def yMax as float init 0.0;
    def yMin as float init 0.0;
    for (def i in 0..$n) {
        def span as list of float init barSpan($data, $i);
        if ($stacked) {
            if ($span[0] > $yMax) {
                $yMax = $span[0];
            }
            if ($span[1] < $yMin) {
                $yMin = $span[1];
            }
        } else {
            for (def si in 0..len($data)) {
                def v as float init $data[$si].ys[$i];
                if ($v > $yMax) {
                    $yMax = $v;
                }
                if ($v < $yMin) {
                    $yMin = $v;
                }
            }
        }
    }
    if ($yMax == $yMin) {
        $yMax = $yMin + 1.0;
    }
    def g as Geom init geomOf($opts);
    def ya as Axis init linearAxis($yMin, $yMax);
    def out as list of string init frameCommon($g, $opts);
    $out = lists.concat($out, drawYAxis($g, $opts, $ya));
    def slot as float init convert.toFloat($g.x1 - $g.x0) / convert.toFloat($n);
    for (def i in 0..$n) {
        def cx as float init convert.toFloat($g.x0) + $slot * (convert.toFloat($i) + 0.5);
        if ($stacked) {
            $out = lists.concat($out, barStacked($g, $data, $ya, $opts, $labels[$i], $cx, $slot, $i));
        } else {
            $out = lists.concat($out, barGrouped($g, $data, $ya, $opts, $labels[$i], $cx, $slot, $i));
        }
        $out[] = labelText($opts, $cx, convert.toFloat($g.y1 + 16), "middle", $opts.fontSize, "normal", "#333333", $labels[$i]);
    }
    $out = lists.concat($out, drawRefLines($g, $opts, $ya, $ya, false));
    if ($opts.legend and wantsLegend($data)) {
        $out = lists.concat($out, legendBox($g, $data, $opts));
    }
    return svgWrap($opts, $out);
}

/**
 * A single-series vertical bar chart: one bar per labelled category, from zero
 * (negative values diverge below). A convenience over `bars`.
 * @param labels {list of string} the category labels
 * @param values {list of float} the bar heights
 * @param opts {Options} chart options
 * @return {string} a complete SVG document
 * @throws {Error} kind "plot" when labels / values are empty or unequal length
 */
export func bar(labels as list of string, values as list of float, opts as Options) {
    if (len($labels) == 0 or len($labels) != len($values)) {
        throw Error{kind: "plot", message: "plot.bar: labels and values must be non-empty and the same length", file: "", line: 0, col: 0};
    }
    def s as Series init Series{name: "", xs: [], ys: $values, color: $opts.color, mark: "line", dash: false, yErr: [], shape: "circle"};
    return bars($labels, [$s], $opts);
}

/**
 * A histogram of `data` into `bins` equal-width buckets over the data range,
 * drawing the bucket counts as adjacent bars.
 * @param data {list of float} the samples
 * @param bins {int} the number of buckets (>= 1)
 * @param opts {Options} chart options
 * @return {string} a complete SVG document
 * @throws {Error} kind "plot" when data is empty or bins < 1
 */
export func histogram(data as list of float, bins as int, opts as Options) {
    if (len($data) == 0 or $bins < 1 or $bins > MAX_BINS) {
        throw Error{kind: "plot", message: "plot.histogram: data must be non-empty and bins in 1.." + convert.toString(MAX_BINS), file: "", line: 0, col: 0};
    }
    def lo as float init minOf($data);
    def hi as float init maxOf($data);
    if ($hi <= $lo) {
        $hi = $lo + 1.0;
    }
    def bw as float init ($hi - $lo) / convert.toFloat($bins);
    def counts as list of int init [];
    for (def i in 0..$bins) {
        $counts[] = 0;
    }
    for (def v in $data) {
        def idx as int init math.floor(($v - $lo) / $bw);
        if ($idx >= $bins) {
            $idx = $bins - 1;
        }
        if ($idx < 0) {
            $idx = 0;
        }
        $counts[$idx] = $counts[$idx] + 1;
    }
    def cmax as int init 0;
    for (def c in $counts) {
        if ($c > $cmax) {
            $cmax = $c;
        }
    }
    def g as Geom init geomOf($opts);
    def ya as Axis init linearAxis(0.0, convert.toFloat($cmax));
    def xa as Axis init linearAxis($lo, $hi);
    def out as list of string init frameCommon($g, $opts);
    $out = lists.concat($out, drawYAxis($g, $opts, $ya));
    $out = lists.concat($out, drawXAxis($g, $opts, $xa));
    for (def i in 0..$bins) {
        def left as float init sx($g, $lo + convert.toFloat($i) * $bw, $xa.lo, $xa.hi, false);
        def right as float init sx($g, $lo + convert.toFloat($i + 1) * $bw, $xa.lo, $xa.hi, false);
        def top as float init sy($g, convert.toFloat($counts[$i]), $ya.lo, $ya.hi, false);
        def tt as string init num($lo + convert.toFloat($i) * $bw) + ".." + num($lo + convert.toFloat($i + 1) * $bw) + ": " + convert.toString($counts[$i]);
        $out[] = barRect($left + 0.5, $top, $right - $left - 1.0, convert.toFloat($g.y1) - $top, $opts.color, titleChild($opts.hover, $tt));
    }
    $out = lists.concat($out, drawRefLines($g, $opts, $xa, $ya, true));
    return svgWrap($opts, $out);
}

# --- output --------------------------------------------------------

/**
 * Write an SVG chart string to a file, returning the path. A convenience over
 * `fs.writeString` so `plot.save(plot.line(...), "chart.svg")` reads in one line.
 * (Jennifer values have no methods, so it is `plot.save($svg, path)`, not
 * `$svg.save(path)`.)
 * @param svg {string} the SVG document (from any chart function)
 * @param path {string} the destination file path
 * @return {string} the path written
 */
export func save(svg as string, path as string) {
    fs.writeString($path, $svg);
    return $path;
}
