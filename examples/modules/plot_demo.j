# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The plot module (modules/plot.j): render data to SVG charts. Writes a
 * multi-series chart with a reference line, grouped and stacked bars, an
 * area + dashed chart, a histogram, a log-scale chart, and a date-axis chart
 * into the OS temp directory (via plot.save).
 * Run: jennifer run examples/modules/plot_demo.j
 * @module plot_demo
 */
use io;
use os;
use path;
use convert;
import "../../modules/plot.j" as plot;

def dir as string init os.tempDir();
def names as list of string init [];
def svgs as list of string init [];

# 1. Two series with a legend, a dashed target line, square markers, and error bars.
def multiOpts as plot.Options init plot.defaults();
$multiOpts.title = "measured vs model";
$multiOpts.xLabel = "x";
$multiOpts.yLabel = "y";
$multiOpts.legendPos = "top-left";
$multiOpts.refLines = [plot.hline(20.0, "target")];
def measured as plot.Series init plot.points("measured", [1.0, 2.0, 3.0, 4.0, 5.0], [1.1, 4.2, 8.8, 16.4, 24.7]);
$measured.yErr = [0.6, 0.8, 1.0, 1.2, 1.4];
$measured.shape = "square";
def model as plot.Series init plot.series("model", [1.0, 2.0, 3.0, 4.0, 5.0], [1.0, 4.0, 9.0, 16.0, 25.0]);
$names[] = "multi";
$svgs[] = plot.chart([$measured, $model], $multiOpts);

# 1b. Diverging bars (positive and negative) with value labels.
def netOpts as plot.Options init plot.defaults();
$netOpts.title = "monthly net (diverging + labels)";
$netOpts.barLabels = true;
$netOpts.legend = false;
def net as plot.Series init plot.series("net", [], [5.0, -3.0, 8.0, -2.0, 6.0]);
$names[] = "diverging";
$svgs[] = plot.bars(["Jan", "Feb", "Mar", "Apr", "May"], [$net], $netOpts);

# 2. Grouped and 3. stacked multi-series bars.
def north as plot.Series init plot.series("north", [], [12.0, 19.0, 7.0, 15.0]);
def south as plot.Series init plot.series("south", [], [8.0, 14.0, 11.0, 20.0]);
def groupedOpts as plot.Options init plot.defaults();
$groupedOpts.title = "sales by region (grouped)";
$names[] = "grouped";
$svgs[] = plot.bars(["Q1", "Q2", "Q3", "Q4"], [$north, $south], $groupedOpts);
def stackedOpts as plot.Options init plot.defaults();
$stackedOpts.title = "sales by region (stacked)";
$stackedOpts.barMode = "stacked";
$names[] = "stacked";
$svgs[] = plot.bars(["Q1", "Q2", "Q3", "Q4"], [$north, $south], $stackedOpts);

# 4. An area fill under one series and a dashed second series, wider left margin.
def areaS as plot.Series init plot.series("cumulative", [1.0, 2.0, 3.0, 4.0, 5.0], [2.0, 5.0, 9.0, 12.0, 18.0]);
$areaS.mark = "area";
def dashedS as plot.Series init plot.series("budget", [1.0, 2.0, 3.0, 4.0, 5.0], [3.0, 6.0, 9.0, 12.0, 15.0]);
$dashedS.dash = true;
def areaOpts as plot.Options init plot.defaults();
$areaOpts.title = "area + dashed";
$areaOpts.marginLeft = 70;
$names[] = "area";
$svgs[] = plot.chart([$areaS, $dashedS], $areaOpts);

# 5. A histogram of a small sample into 5 buckets.
def sample as list of float init [1.0, 2.0, 2.0, 3.0, 3.0, 3.0, 4.0, 4.0, 5.0, 2.0, 3.0, 3.0, 4.0, 4.0, 3.0];
def histOpts as plot.Options init plot.defaults();
$histOpts.title = "sample distribution";
$names[] = "histogram";
$svgs[] = plot.histogram($sample, 5, $histOpts);

# 6. A log-scale y axis (exponential growth reads as a straight line).
def logOpts as plot.Options init plot.defaults();
$logOpts.title = "log-scale growth";
$logOpts.yLog = true;
$names[] = "log";
$svgs[] = plot.line([1.0, 2.0, 3.0, 4.0, 5.0], [8.0, 40.0, 200.0, 1000.0, 5000.0], $logOpts);

# 7. A date x axis - x values are Unix seconds, ticks labelled as dates.
def dateOpts as plot.Options init plot.defaults();
$dateOpts.title = "daily readings";
$dateOpts.xDate = true;
def day as int init 86400;
def base as int init 1718452800;
def times as list of float init [];
def values as list of float init [11.0, 14.0, 9.0, 17.0, 13.0, 20.0, 16.0];
for (def i in 0..len($values)) {
    $times[] = convert.toFloat($base + $i * $day);
}
$names[] = "date";
$svgs[] = plot.line($times, $values, $dateOpts);

for (def i in 0..len($names)) {
    def p as string init plot.save($svgs[$i], path.join($dir, "jennifer-" + $names[$i] + ".svg"));
    io.printf("wrote %s (%d bytes)\n", $p, len($svgs[$i]));
}
