# `stats` - descriptive statistics

The `stats` library computes descriptive statistics over a `list of int` or
`list of float`. It is pure-value and dependency-free (Go stdlib only), so both
binaries build it.

```jennifer
use stats;
def xs as list of int init [2, 4, 4, 4, 5, 5, 7, 9];
stats.mean($xs);              # 5.0
stats.median($xs);           # 4.5
stats.stddev($xs);           # 2.0
stats.percentile($xs, 90);   # 7.6
```

## Functions

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `stats.mean(xs)`             | `float`        | Arithmetic mean. Empty list errors.                                            |
| `stats.median(xs)`          | `float`        | Middle value (mean of the two middles for an even count). Empty errors.         |
| `stats.mode(xs)`            | element kind   | Most frequent element (first-reached wins a tie). Empty errors.                 |
| `stats.variance(xs)`        | `float`        | **Population** variance (sum of squared deviations / n). Empty errors.          |
| `stats.stddev(xs)`          | `float`        | Population standard deviation = `sqrt(variance)`. Empty errors.                  |
| `stats.sampleVariance(xs)`  | `float`        | **Sample** variance (÷ n-1, Bessel-corrected). Needs >= 2 elements.             |
| `stats.sampleStddev(xs)`    | `float`        | Sample standard deviation = `sqrt(sampleVariance)`. Needs >= 2 elements.         |
| `stats.mad(xs)`             | `float`        | Median absolute deviation - a robust (outlier-resistant) spread. Empty errors.  |
| `stats.skewness(xs)`        | `float`        | Population skewness (3rd standardized moment); `0` symmetric, `> 0` right tail.  |
| `stats.kurtosis(xs)`        | `float`        | **Excess** kurtosis (4th moment - 3); `0` for a normal, `> 0` heavy tails.       |
| `stats.percentile(xs, p)`   | `float`        | The `p`-th percentile (`p` in `[0, 100]`), linear interpolation. See below.      |
| `stats.quartiles(xs)`       | `list of float`| `[Q1, Q2, Q3]` = `[p25, p50, p75]`. Empty errors.                               |
| `stats.iqr(xs)`             | `float`        | Interquartile range = `p75 - p25`. Empty errors.                                |
| `stats.min(xs)`             | element kind   | Smallest element. Empty errors.                                                 |
| `stats.max(xs)`             | element kind   | Largest element. Empty errors.                                                  |
| `stats.range(xs)`           | element kind   | The spread `max - min` (int for an all-int list, overflow-checked). Empty errors. |
| `stats.sum(xs)`             | `int` / `float`| Sum: `int` for an all-int list (overflow errors), `float` otherwise. `sum([])` is `0`. |
| `stats.geometricMean(xs)`   | `float`        | Geometric mean (n-th root of the product). Every element must be `> 0`.          |
| `stats.harmonicMean(xs)`    | `float`        | Harmonic mean (`n` / sum of reciprocals). Every element must be `> 0`.           |
| `stats.weightedMean(xs, ws)`| `float`        | Weighted mean `sum(x*w) / sum(w)`. Equal-length; weights must not sum to `0`.    |
| `stats.zscore(xs)`          | `list of float`| Each element standardized to `(x - mean) / stddev` (population). Constant list errors. |
| `stats.modes(xs)`           | `list`         | Every modal value (all tied for the top frequency), in first-seen order; keeps kind. |
| `stats.correlation(xs, ys)` | `float`        | Pearson correlation of two equal-length lists (>= 2 elements). See below.        |
| `stats.covariance(xs, ys)`  | `float`        | **Population** covariance (÷ n), the units-aware companion to `correlation`.     |
| `stats.sampleCovariance(xs, ys)` | `float`   | **Sample** covariance (÷ n-1), the companion to `sampleVariance`. Needs >= 2.    |
| `stats.describe(xs)`        | `stats.Summary`| One-call `{count, min, q1, median, mean, q3, max, stddev}`. See below.           |

Each function accepts a `list of int` or a `list of float` (an `int` element
promotes to `float` for the real-valued reductions). A non-numeric element, or a
non-list argument, is a positioned error.

## Kind-preserving vs. real-valued

The reductions that are inherently real-valued - `mean`, `median`, `variance`,
`stddev`, `percentile`, `correlation` - always return `float`. The selections that
pick an element - `min`, `max`, `mode` - and `sum` preserve the input kind: over a
`list of int` they return an `int`, over a `list of float` a `float`.

```jennifer
stats.min([2, 4, 9]);        # 2   (int)
stats.min([2.0, 4.0]);       # 2.0 (float)
stats.sum([1, 2, 3]);        # 6   (int, overflow-checked)
stats.sum([1.5, 2.5]);       # 4.0 (float)
```

## Population vs. sample

`variance` / `stddev` are **population** statistics (divide by `n`) - the list is
treated as the whole data set, matching NumPy's default. `sampleVariance` /
`sampleStddev` are the **sample** (Bessel-corrected, `÷ n-1`) forms for
inferential use, and need at least two elements. `covariance` follows the
population convention (`÷ n`) to match `variance`; the normalized `correlation`
divides out the standard deviations, so its `n` factor cancels and the
population/sample distinction does not matter there. The companion
[`linalg`](linalg.md) library does linear algebra over the same value types;
further ML primitives atop the two are on the [horizon](../horizon.md).

## Percentile

`stats.percentile(xs, p)` sorts the data and interpolates linearly between the two
nearest ranks (NumPy's default method): `p = 0` is the minimum, `p = 100` the
maximum, and `p = 50` equals `median`. `p` may be an `int` or `float`; a value
outside `[0, 100]`, or an empty list, is an error.

## Correlation

`stats.correlation(xs, ys)` is the Pearson correlation coefficient in `[-1, 1]`:
`1.0` for a perfect positive linear relationship, `-1.0` for a perfect negative
one, `0.0` for none. The two lists must be the same length with at least two
elements, and neither may have zero variance (a constant list) - all of which are
catchable errors. `stats.covariance(xs, ys)` is the raw, units-aware version (the
numerator, `÷ n`).

## Quartiles, IQR, and z-scores

`stats.quartiles(xs)` returns `[Q1, Q2, Q3]` (`[p25, p50, p75]`) as a `list of
float`; `stats.iqr(xs)` is `p75 - p25`, useful for spread and outlier detection.
`stats.range(xs)` is the full spread `max - min` (kind-preserving). `stats.zscore(xs)`
standardizes every element to `(x - mean) / stddev` (population), returning a `list
of float` - a constant list has no z-score and errors.

## Geometric and harmonic means

`stats.geometricMean(xs)` (the n-th root of the product, computed via logs so a
long list does not overflow) suits growth rates and ratios; `stats.harmonicMean(xs)`
(`n` / sum of reciprocals) suits averaging rates. Both require every element to be
strictly positive. `stats.weightedMean(xs, weights)` averages `xs` weighted by
`weights` (same length; the weights must not sum to zero).

## Shape

`stats.skewness(xs)` is the population skewness (the standardized third moment):
`0` for a symmetric distribution, positive when the right tail is longer.
`stats.kurtosis(xs)` is the **excess** kurtosis (the standardized fourth moment
minus 3), so a normal distribution is `0` and heavier-than-normal tails are
positive. Both need at least two elements and a non-constant list.
`stats.mad(xs)` (median absolute deviation) and `stats.modes(xs)` (every modal
value, versus `mode`'s single one) round out the robust / multimodal summaries.

## `describe`

`stats.describe(xs)` computes a whole summary in one pass and returns a
**`stats.Summary`** struct - `count` (`int`) plus `min`, `q1`, `median`, `mean`,
`q3`, `max`, and `stddev` (population, all `float`):

```jennifer
def s as stats.Summary init stats.describe($data);
io.printf("median %v, IQR %v\n", $s.median, $s.q3 - $s.q1);
```

## Strictness

Following `math`'s stance, a non-finite or mathematically undefined result is a
catchable error rather than a `NaN` or `Inf`: an empty list (for every reduction
except `sum`), a percentile `p` outside `[0, 100]`, mismatched or too-short
`correlation` inputs, a zero-variance `correlation`, and an int `sum` that
overflows `int64` all raise a positioned error you can `try` / `catch`. The same
applies when an input's *magnitudes* overflow the computation - a `list of float`
with values near the float64 ceiling (`~1.8e308`) can push an intermediate sum or
power to `+/-Inf` (and thence `NaN`); every real-valued reduction rejects a
non-finite result instead of returning it, so a `NaN` never escapes into the type
system.
