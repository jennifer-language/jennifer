# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# plot_test.j - white-box tests for plot.j. Run with:
#
#     jennifer test modules/plot_test.j
#
# The overlay splices plot.j in front of this file, so the tests reach its
# private helpers (niceNum, niceTicks, minOf, maxOf, num, svgEsc) by bare
# identifier as well as its exported chart functions.
use testing;
use strings;
use fs;
use os;
use path;

# occurrences counts how many times needle appears in haystack.
func occurrences(haystack as string, needle as string) {
    return len(strings.split($haystack, $needle)) - 1;
}

func testDefaults() {
    def o as Options init defaults();
    testing.assertEqual($o.width, 640);
    testing.assertEqual($o.height, 400);
    testing.assertEqual($o.title, "");
}

func testFloatsConverts() {
    def f as list of float init floats([1, 2, 3]);
    testing.assertEqual(len($f), 3);
    testing.assertEqual($f[1], 2.0);
}

func testMinMax() {
    testing.assertEqual(minOf([3.0, 1.0, 2.0]), 1.0);
    testing.assertEqual(maxOf([3.0, 1.0, 2.0]), 3.0);
}

func testNiceNum() {
    testing.assertEqual(niceNum(2.0, true), 2.0);
    testing.assertEqual(niceNum(1.0, true), 1.0);
    testing.assertEqual(niceNum(0.9, true), 1.0);
    testing.assertEqual(niceNum(6.0, true), 5.0);
}

func testNiceTicksZeroToTen() {
    def t as list of float init niceTicks(0.0, 10.0, 6);
    testing.assertEqual(len($t), 6);
    testing.assertEqual($t[0], 0.0);
    testing.assertEqual($t[1], 2.0);
    testing.assertEqual($t[5], 10.0);
}

func testNiceTicksDegenerate() {
    # A flat range must not divide by zero; it expands to a unit span.
    def t as list of float init niceTicks(5.0, 5.0, 6);
    testing.assertTrue(len($t) >= 2);
}

func testNumFormatting() {
    testing.assertEqual(num(3.0), "3");
    testing.assertEqual(num(10.0), "10");
    testing.assertEqual(num(1.5), "1.50");
}

func testSvgEscape() {
    testing.assertEqual(svgEsc("a & b < c > d"), "a &amp; b &lt; c &gt; d");
}

func testLineWellFormed() {
    def o as Options init defaults();
    $o.title = "growth <n>";
    def svg as string init line([1.0, 2.0, 3.0], [1.0, 4.0, 9.0], $o);
    testing.assertTrue(strings.startsWith($svg, "<svg xmlns="));
    testing.assertTrue(strings.contains($svg, 'viewBox="0 0 640 400"'));
    testing.assertTrue(strings.contains($svg, "<polyline"));
    testing.assertTrue(strings.endsWith($svg, "</svg>" + "\n"));
    # the title is escaped
    testing.assertTrue(strings.contains($svg, "growth &lt;n&gt;"));
}

func testScatterOneCirclePerPoint() {
    def svg as string init scatter([1.0, 2.0, 3.0, 4.0], [2.0, 3.0, 1.0, 5.0], defaults());
    testing.assertEqual(occurrences($svg, "<circle"), 4);
}

func testBarOneRectPerBarPlusFrame() {
    # frameCommon draws 2 rects (background + border); each bar adds one.
    def svg as string init bar(["a", "b", "c"], [3.0, 7.0, 5.0], defaults());
    testing.assertEqual(occurrences($svg, "<rect"), 5);
    testing.assertTrue(strings.contains($svg, ">a</text>"));
}

func testHistogramBarCountMatchesBins() {
    def data as list of float init [1.0, 1.0, 2.0, 2.0, 2.0, 3.0, 9.0, 9.0];
    def svg as string init histogram($data, 4, defaults());
    # 2 frame rects + 4 bin bars
    testing.assertEqual(occurrences($svg, "<rect"), 6);
}

func testLineRejectsMismatchedLengths() {
    def threw as bool init false;
    try {
        line([1.0], [1.0, 2.0], defaults());
    } catch (e) {
        $threw = true;
    }
    testing.assertTrue($threw);
}

func testHistogramRejectsZeroBins() {
    def threw as bool init false;
    try {
        histogram([1.0, 2.0], 0, defaults());
    } catch (e) {
        $threw = true;
    }
    testing.assertTrue($threw);
}

# --- multi-series + legend ---

func testChartMultiSeriesTwoLinesAndLegend() {
    def a as Series init series("actual", [1.0, 2.0, 3.0], [1.0, 4.0, 9.0]);
    def b as Series init series("model", [1.0, 2.0, 3.0], [2.0, 3.0, 8.0]);
    def svg as string init chart([$a, $b], defaults());
    testing.assertEqual(occurrences($svg, "<polyline"), 2);
    testing.assertTrue(strings.contains($svg, ">actual</text>"));
    testing.assertTrue(strings.contains($svg, ">model</text>"));
    testing.assertTrue(strings.contains($svg, 'fill-opacity="0.85"'));
}

func testSeriesMarkBoth() {
    def s as Series init series("s", [1.0, 2.0, 3.0], [1.0, 2.0, 3.0]);
    $s.mark = "both";
    def svg as string init chart([$s], defaults());
    testing.assertEqual(occurrences($svg, "<polyline"), 1);
    testing.assertEqual(occurrences($svg, "<circle"), 3);
}

func testPointsConstructor() {
    def s as Series init points("p", [1.0], [1.0]);
    testing.assertEqual($s.mark, "points");
}

func testPaletteDistinctAndWraps() {
    testing.assertTrue(paletteColor(0) != paletteColor(1));
    testing.assertEqual(paletteColor(0), paletteColor(8));
}

func testSingleUnnamedSeriesNoLegend() {
    def svg as string init line([1.0, 2.0], [1.0, 2.0], defaults());
    testing.assertFalse(strings.contains($svg, 'fill-opacity="0.85"'));
}

# --- log scales ---

func testLogAxisDecades() {
    def ax as Axis init logAxis(1.0, 1000.0);
    testing.assertTrue($ax.isLog);
    testing.assertEqual($ax.lo, 1.0);
    testing.assertEqual($ax.hi, 1000.0);
    testing.assertEqual($ax.ticks[0], 1.0);
    testing.assertEqual($ax.ticks[len($ax.ticks) - 1], 1000.0);
}

func testLogAxisRejectsNonPositive() {
    def threw as bool init false;
    try {
        logAxis(0.0, 100.0);
    } catch (e) {
        $threw = true;
    }
    testing.assertTrue($threw);
}

func testYLogChartRenders() {
    def o as Options init defaults();
    $o.yLog = true;
    def svg as string init line([1.0, 2.0, 3.0], [10.0, 100.0, 1000.0], $o);
    testing.assertTrue(strings.contains($svg, "<polyline"));
    testing.assertTrue(strings.contains($svg, ">100</text>"));
}

# --- date axis ---

func testDateAxisLabels() {
    # 2024-06-15 12:00 UTC .. one day later
    def ax as Axis init dateAxis(1718452800.0, 1718539200.0, 6, "");
    testing.assertTrue(len($ax.ticks) >= 2);
    testing.assertTrue(strings.contains($ax.labels[0], ":") or strings.contains($ax.labels[0], "-"));
}

func testXDateChartUsesDateLabels() {
    def o as Options init defaults();
    $o.xDate = true;
    $o.dateFormat = "%Y-%m-%d";
    def svg as string init line([1718452800.0, 1718539200.0, 1718625600.0], [1.0, 2.0, 3.0], $o);
    testing.assertTrue(strings.contains($svg, "2024-06-"));
}

# --- fonts + grid ---

func testFontOptionsApplied() {
    def o as Options init defaults();
    $o.fontFamily = "Georgia, serif";
    $o.fontSize = 14;
    def svg as string init line([1.0, 2.0], [1.0, 2.0], $o);
    testing.assertTrue(strings.contains($svg, 'font-family="Georgia, serif"'));
    testing.assertTrue(strings.contains($svg, 'font-size="14"'));
}

func testGridToggleOff() {
    def o as Options init defaults();
    $o.grid = false;
    def withGrid as string init line([1.0, 2.0, 3.0], [1.0, 2.0, 3.0], defaults());
    def noGrid as string init line([1.0, 2.0, 3.0], [1.0, 2.0, 3.0], $o);
    testing.assertTrue(occurrences($withGrid, "#eeeeee") > occurrences($noGrid, "#eeeeee"));
}

# --- grouped / stacked bars ---

func testGroupedBarsRectCount() {
    def a as Series init series("a", [], [3.0, 5.0]);
    def b as Series init series("b", [], [4.0, 2.0]);
    def o as Options init defaults();
    $o.legend = false;
    def svg as string init bars(["c1", "c2"], [$a, $b], $o);
    # 2 frame rects + 2 categories x 2 series = 6
    testing.assertEqual(occurrences($svg, "<rect"), 6);
    testing.assertEqual(occurrences($svg, "<polyline"), 0);
}

func testStackedBarsRenders() {
    def a as Series init series("a", [], [3.0, 5.0]);
    def b as Series init series("b", [], [4.0, 2.0]);
    def o as Options init defaults();
    $o.legend = false;
    $o.barMode = "stacked";
    def svg as string init bars(["c1", "c2"], [$a, $b], $o);
    testing.assertTrue(strings.startsWith($svg, "<svg xmlns="));
    testing.assertEqual(occurrences($svg, "<rect"), 6);
}

func testBarsLegendFromNames() {
    def a as Series init series("north", [], [3.0]);
    def b as Series init series("south", [], [4.0]);
    def svg as string init bars(["q1"], [$a, $b], defaults());
    testing.assertTrue(strings.contains($svg, ">north</text>"));
    testing.assertTrue(strings.contains($svg, ">south</text>"));
}

func testBarsRejectsRaggedSeries() {
    def a as Series init series("a", [], [1.0, 2.0]);
    def b as Series init series("b", [], [3.0]);
    def threw as bool init false;
    try {
        bars(["x", "y"], [$a, $b], defaults());
    } catch (e) {
        $threw = true;
    }
    testing.assertTrue($threw);
}

# --- reference lines ---

func testHorizontalRefLine() {
    def o as Options init defaults();
    $o.refLines = [hline(5.0, "target")];
    def svg as string init line([1.0, 2.0, 3.0], [1.0, 4.0, 9.0], $o);
    testing.assertTrue(strings.contains($svg, 'stroke="#cc0000"'));
    testing.assertTrue(strings.contains($svg, 'stroke-dasharray="5,4"'));
    testing.assertTrue(strings.contains($svg, ">target</text>"));
}

func testRefLineConstructors() {
    def h as RefLine init hline(3.0, "h");
    def v as RefLine init vline(7.0, "v");
    testing.assertEqual($h.axis, "y");
    testing.assertEqual($v.axis, "x");
    testing.assertTrue($h.dash);
}

# --- area fill + dashed lines ---

func testAreaFillPolygon() {
    def s as Series init series("s", [1.0, 2.0, 3.0], [1.0, 3.0, 2.0]);
    $s.mark = "area";
    def svg as string init chart([$s], defaults());
    testing.assertEqual(occurrences($svg, "<polygon"), 1);
    testing.assertEqual(occurrences($svg, "<polyline"), 1);
}

func testDashedLine() {
    def s as Series init series("s", [1.0, 2.0, 3.0], [1.0, 2.0, 3.0]);
    $s.dash = true;
    def svg as string init chart([$s], defaults());
    testing.assertTrue(strings.contains($svg, 'stroke-dasharray="6,4"'));
}

# --- configurable margins ---

func testConfigurableMargins() {
    def o as Options init defaults();
    $o.marginLeft = 100;
    def svg as string init line([1.0, 2.0], [1.0, 2.0], $o);
    # the plot border rect sits at the left margin
    testing.assertTrue(strings.contains($svg, '<rect x="100" y='));
}

# --- save ---

func testSaveWritesFile() {
    def p as string init path.join(os.tempDir(), "jennifer-plot-test.svg");
    def svg as string init line([1.0, 2.0], [1.0, 2.0], defaults());
    def ret as string init save($svg, $p);
    testing.assertEqual($ret, $p);
    testing.assertTrue(fs.exists($p));
    testing.assertEqual(fs.readString($p), $svg);
    fs.remove($p);
}

# --- negative / diverging bars ---

func testNegativeBarsDiverge() {
    def s as Series init series("x", [], [5.0, -3.0, 2.0]);
    def o as Options init defaults();
    $o.legend = false;
    def svg as string init bars(["a", "b", "c"], [$s], $o);
    # no invalid negative-height rects, and the y-axis includes a negative tick
    testing.assertFalse(strings.contains($svg, 'height="-'));
    testing.assertTrue(strings.contains($svg, ">-2</text>"));
}

func testStackedNegativeBarsRender() {
    def a as Series init series("a", [], [4.0, -2.0]);
    def b as Series init series("b", [], [-1.0, 3.0]);
    def o as Options init defaults();
    $o.barMode = "stacked";
    def svg as string init bars(["p", "q"], [$a, $b], $o);
    testing.assertTrue(strings.startsWith($svg, "<svg xmlns="));
    testing.assertFalse(strings.contains($svg, 'height="-'));
}

# --- legend positioning ---

func testLegendPosTopLeft() {
    def a as Series init series("a", [1.0, 2.0], [1.0, 2.0]);
    def b as Series init series("b", [1.0, 2.0], [2.0, 3.0]);
    def o as Options init defaults();
    $o.legendPos = "top-left";
    def svg as string init chart([$a, $b], $o);
    # legend box at the left margin: x0(55)+6 = 61, y0(38)+8 = 46
    testing.assertTrue(strings.contains($svg, '<rect x="61" y="46"'));
}

# --- error bars ---

func testErrorBarsHelper() {
    def s as Series init points("m", [1.0], [10.0]);
    $s.yErr = [2.0];
    def g as Geom init Geom{x0: 55, y0: 38, x1: 620, y1: 354};
    def ax as Axis init linearAxis(0.0, 20.0);
    def out as list of string init errorBars($g, $s, "#000000", $ax, $ax);
    testing.assertEqual(len($out), 3);
}

func testErrorBarsExtendDomain() {
    # y+err reaches 22, so the axis tops out above 20 (a ">25" tick appears).
    def s as Series init points("m", [1.0, 2.0, 3.0], [10.0, 20.0, 15.0]);
    $s.yErr = [1.0, 2.0, 1.5];
    def svg as string init chart([$s], defaults());
    testing.assertTrue(strings.contains($svg, ">25</text>"));
}

# --- hover tooltips ---

func testHoverTitlesOn() {
    def s as Series init points("m", [1.0, 2.0], [3.0, 4.0]);
    testing.assertTrue(strings.contains(chart([$s], defaults()), "<title>"));
}

func testHoverTitlesOff() {
    def s as Series init points("m", [1.0, 2.0], [3.0, 4.0]);
    def o as Options init defaults();
    $o.hover = false;
    testing.assertFalse(strings.contains(chart([$s], $o), "<title>"));
}

# --- data labels ---

func testBarDataLabels() {
    def s as Series init series("x", [], [7.0, 3.0]);
    def o as Options init defaults();
    $o.barLabels = true;
    $o.legend = false;
    def svg as string init bars(["a", "b"], [$s], $o);
    # 7 and 3 are not axis ticks here, so these come from the bar labels
    testing.assertTrue(strings.contains($svg, ">7</text>"));
    testing.assertTrue(strings.contains($svg, ">3</text>"));
}

# --- marker shapes ---

func testMarkerSquare() {
    def s as Series init points("m", [1.0, 2.0, 3.0], [1.0, 2.0, 3.0]);
    $s.shape = "square";
    def svg as string init chart([$s], defaults());
    testing.assertEqual(occurrences($svg, "<circle"), 0);
    testing.assertTrue(occurrences($svg, "<rect") >= 3);
}

func testMarkerTriangle() {
    def s as Series init points("m", [1.0, 2.0], [1.0, 2.0]);
    $s.shape = "triangle";
    def svg as string init chart([$s], defaults());
    testing.assertEqual(occurrences($svg, "<polygon"), 2);
    testing.assertEqual(occurrences($svg, "<circle"), 0);
}

# --- DF-plot-report hardening ---

func testDF001AttrEscHelper() {
    testing.assertEqual(attrEsc('a"b<c>&'), "a&quot;b&lt;c&gt;&amp;");
}

func testDF001InjectionNeutralized() {
    def o as Options init defaults();
    $o.color = 'red"><script>x()</script><a b="';
    $o.background = 'w"><script>y()</script><a b="';
    $o.fontFamily = 's"><script>z()</script><a b="';
    def svg as string init line([1.0, 2.0], [1.0, 2.0], $o);
    testing.assertFalse(strings.contains($svg, "<script>"));
    testing.assertTrue(strings.contains($svg, "&quot;"));
}

func testDF002NumScientificFallback() {
    # math.round overflows int range at 1e19; num falls back to scientific.
    testing.assertEqual(num(1e19), "1e+19");
}

func testDF002ExtremeMagnitudeRenders() {
    def svg as string init line([1.0, 2.0, 3.0], [1e19, 2e19, 3e19], defaults());
    testing.assertTrue(strings.startsWith($svg, "<svg xmlns="));
    testing.assertTrue(strings.contains($svg, "e+"));
}

func testDF002ConstantHugeRenders() {
    # A ULP-collapsed range must not feed log10(0).
    def svg as string init line([1.0, 2.0, 3.0], [1e19, 1e19, 1e19], defaults());
    testing.assertTrue(strings.startsWith($svg, "<svg xmlns="));
}

func testDF003HistogramBinsCapped() {
    def threw as bool init false;
    try {
        histogram([1.0, 2.0], 999999, defaults());
    } catch (e) {
        $threw = true;
    }
    testing.assertTrue($threw);
}

func testDF004CapNameHelper() {
    testing.assertEqual(len(capName(strings.repeat("x", 5000))), MAX_TOOLTIP + 3);
    testing.assertEqual(capName("short"), "short");
}

func testDF004TooltipNameCapped() {
    # Legend off, so the only place the long name could appear is the per-mark
    # tooltips - which are capped. (The legend shows a name once, not per mark,
    # so it is not the quadratic amplifier DF-004 targets.)
    def o as Options init defaults();
    $o.legend = false;
    def s as Series init points(strings.repeat("A", 5000), [1.0, 2.0], [1.0, 2.0]);
    def svg as string init chart([$s], $o);
    testing.assertFalse(strings.contains($svg, strings.repeat("A", 200)));
    testing.assertTrue(strings.contains($svg, "..."));
}

func testDF005EmptyFirstSeriesFriendly() {
    def threw as bool init false;
    def msg as string init "";
    try {
        chart([series("a", [], [])], defaults());
    } catch (e) {
        $threw = true;
        $msg = $e.message;
    }
    testing.assertTrue($threw);
    testing.assertTrue(strings.contains($msg, "non-empty"));
}

func testDF006RefLineLogSkipped() {
    def o as Options init defaults();
    $o.yLog = true;
    $o.refLines = [hline(0.0, "z")];
    def svg as string init line([1.0, 2.0], [10.0, 100.0], $o);
    testing.assertTrue(strings.startsWith($svg, "<svg xmlns="));
}
