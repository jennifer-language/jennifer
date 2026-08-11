# `plot` - data plotting to SVG

Import with `import "plot.j" as plot;`. Turns numbers into a self-contained
`<svg>` chart string you can write to a `.svg` file or drop into HTML. A unified
`chart` renders one or more **series** (line, points, both, or filled area,
solid or dashed, with **error bars** and pickable **marker shapes**) on shared
axes with a positioned **legend**; `line` / `scatter` / `bar` / `bars` /
`histogram` are focused wrappers, and `bars` **groups or stacks** multiple
series (values may be negative - bars **diverge** from a zero baseline). Charts
support **log scales**, a **date axis**, **fonts**, configurable **margins**,
**reference lines**, **data labels** on bars, and native `<title>` **hover
tooltips**. `save` writes the SVG to a file. Pure Jennifer over `math` and
`time` / `fs` / `strings` / `lists` / `convert`; both binaries; the visual
companion to the `stats` / `ml` numeric stack.

```jennifer
import "plot.j" as plot;

def opts as plot.Options init plot.defaults();
$opts.title = "measured vs model";
$opts.refLines = [plot.hline(20.0, "target")];

def measured as plot.Series init plot.points("measured", [1.0, 2.0, 3.0], [1.1, 4.2, 8.8]);
$measured.yErr = [0.4, 0.5, 0.6];             # symmetric error bars
$measured.shape = "square";                    # marker shape
def model as plot.Series init plot.series("model", [1.0, 2.0, 3.0], [1.0, 4.0, 9.0]);
plot.save(plot.chart([$measured, $model], $opts), "fit.svg");
```

Runnable: [`examples/modules/plot_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/plot_demo.j).

## Surface

| Call                              | Returns         | Notes                                                                    |
| --------------------------------- | --------------- | ------------------------------------------------------------------------ |
| `plot.defaults()`                 | `Options`       | A 640x400 canvas, blue on white, sans-serif 11px, grid + top-right legend + tooltips on. |
| `plot.series(name, xs, ys)`       | `Series`        | A line series (set `.mark` / `.color` / `.dash` / `.yErr` / `.shape`).   |
| `plot.points(name, xs, ys)`       | `Series`        | A points (scatter) series.                                               |
| `plot.chart(series, opts)`        | `string`        | Render a `list of Series` on shared axes, with a legend for named series.|
| `plot.line(xs, ys, opts)`         | `string`        | One line series in `opts.color`.                                         |
| `plot.scatter(xs, ys, opts)`      | `string`        | One points series in `opts.color`.                                       |
| `plot.bar(labels, values, opts)`  | `string`        | A single-series bar chart, one bar per category (negatives diverge).     |
| `plot.bars(labels, series, opts)` | `string`        | A multi-series bar chart - **grouped** or **stacked** per `opts.barMode`.|
| `plot.histogram(data, bins, opts)`| `string`        | A histogram of `data` into `bins` equal-width buckets.                   |
| `plot.hline(value, label)`        | `RefLine`       | A horizontal reference line (append to `opts.refLines`).                 |
| `plot.vline(value, label)`        | `RefLine`       | A vertical reference line.                                               |
| `plot.floats(xs)`                 | `list of float` | Lift a `list of int` to `list of float`.                                 |
| `plot.save(svg, path)`            | `string`        | Write an SVG string to a file with `fs`; returns the path.               |

Chart data is `list of float` (use `plot.floats` or float literals). Each render
returns a complete SVG document. Empty / mismatched input, a ragged `bars`
series, or a non-positive value on a log axis throw a catchable
`Error{kind: "plot"}`; `histogram` caps `bins` at 10000. Jennifer values have no
methods, so saving is `plot.save($svg, path)`, not `$svg.save(path)`.

The output is safe to build from untrusted data: text is XML-escaped, and every
caller-supplied attribute value (`color` / `background` / `fontFamily`, series and
ref-line colours) is attribute-escaped, so a hostile string cannot break out and
inject markup. Per-mark tooltip names are truncated to keep output bounded, and
data magnitudes past the int range (>= 2^63) render in scientific notation rather
than failing.

## `Series`

`{ name, xs, ys, color, mark, dash, yErr, shape }` - `name` the legend label
(`""` = not listed); `xs` / `ys` the coordinates (`bars` uses only `ys`); `color`
`""` takes a palette colour; `mark` is `"line"`, `"points"`, `"both"`, or
`"area"`; `dash` draws the line dashed; `yErr`, when the same length as `ys`,
draws symmetric error bars (linear y axis only); `shape` picks the scatter marker
- `"circle"`, `"square"`, `"triangle"`, or `"diamond"`. Build with `plot.series`
(line) or `plot.points`, then set the fields.

## `Options`

`{ width, height, title, xLabel, yLabel, color, background, fontFamily, fontSize, grid, legend, legendPos, xLog, yLog, xDate, dateFormat, marginLeft, marginRight, marginTop, marginBottom, barMode, barLabels, hover, refLines }`

- **size / captions** - `width` / `height` px; `title` / `xLabel` / `yLabel`.
- **colours / fonts** - `color`, `background`; `fontFamily` and `fontSize`.
- **frame** - `grid` (gridlines vs. tick marks), the four `margin*` fields.
- **legend** - `legend` on/off and `legendPos` (`"top-right"` / `"top-left"` /
  `"bottom-right"` / `"bottom-left"`).
- **scales** - `xLog` / `yLog` (log10, decade ticks, positive only) and `xDate`
  / `dateFormat` (x = Unix seconds, calendar-boundary ticks via `time`).
- **bars** - `barMode` `"grouped"` / `"stacked"`; `barLabels` draws each grouped
  bar's value; negative values diverge below a zero baseline.
- **tooltips** - `hover` attaches a `<title>` to every mark for native browser
  hover text.
- **reference lines** - `refLines`, a `list of RefLine` from `hline` / `vline`.

## Scope

Data charts over `math` for the axis math. Possible follow-ons: a secondary
y-axis, per-series point size, and non-10 log bases. For graph / network diagrams
(nodes and edges) rather than data charts, see [`dot`](dot.md).
