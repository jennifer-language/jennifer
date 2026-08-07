# `stats` - statistics: descriptive, distributions, and inference

The `stats` library is the whole statistical toolkit: descriptive statistics
over a `list of int` or `list of float`, the probability **distributions** those
build on, and the classical **inference** layer (regression, confidence
intervals, hypothesis tests). It is pure-value and dependency-free (Go stdlib
only, the distribution CDFs reusing `math`'s special functions), so both binaries
build it. There is deliberately no separate `prob` library - distributions live
with the descriptive stats and inference they are used with, as in `scipy.stats`
or R's base `dnorm` / `pnorm` / `qnorm`.

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
| `stats.variance(xs)`        | `float`        | **Population** variance σ² (Σ(x - μ)² / n). Empty errors.                       |
| `stats.stddev(xs)`          | `float`        | Population standard deviation σ = √variance. Empty errors.                       |
| `stats.sampleVariance(xs)`  | `float`        | **Sample** variance s² (÷ n-1, Bessel-corrected). Needs >= 2 elements.          |
| `stats.sampleStddev(xs)`    | `float`        | Sample standard deviation s = √sampleVariance (the "empirical" SD). Needs >= 2. |
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
| `stats.covariance(xs, ys)`  | `float`        | **Population** covariance σ_xy (÷ n), the units-aware companion to `correlation`.|
| `stats.sampleCovariance(xs, ys)` | `float`   | **Sample** covariance s_xy (÷ n-1), the companion to `sampleVariance`. Needs >= 2.|
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

## Population vs. sample (σ vs s)

`variance` / `stddev` are **population** statistics (σ², σ - divide by `n`) - the
list is treated as the whole data set, matching NumPy's default. `sampleVariance`
/ `sampleStddev` are the **sample** (s², s - Bessel-corrected, `÷ (n-1)`) forms
for inferential use, and need at least two elements:

```text
population   σ² = Σ(x - μ)² / n           σ = √σ²
sample       s² = Σ(x - x̄)² / (n - 1)     s = √s²
```

where μ (population) / x̄ (sample) is the mean. The sample form `s` is what some
sources call the **empirical** standard deviation; Jennifer names the two forms
explicitly (`stddev` vs `sampleStddev`) rather than by that ambiguous label.
`covariance` follows the population convention (σ_xy, `÷ n`) to match `variance`;
`sampleCovariance` is the sample form (s_xy, `÷ (n-1)`). The normalized
`correlation` divides out the standard deviations, so its `n` factor cancels and
the population/sample distinction does not matter there. The companion
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

## Distributions

The probability distributions inference is built on, with flat R-style names.
Every function returns a `float`. The continuous CDFs reduce to `math`'s
regularized incomplete gamma / beta; the quantiles (inverse CDFs) invert them
numerically. Sampling draws on `math`'s shared random source (so `math.randSeed`
makes it reproducible).

| Call | Meaning |
| ---- | ------- |
| `stats.normalPdf(x, mean, sd)` / `normalCdf` / `normalQuantile(p, mean, sd)` / `normalSample(mean, sd)` | Normal (Gaussian) N(μ, σ²). `sd > 0`. |
| `stats.tPdf(x, df)` / `tCdf(x, df)` / `tQuantile(p, df)` | Student's *t*. `df > 0`. |
| `stats.chiSquareCdf(x, df)` / `chiSquareQuantile(p, df)` | Chi-square (χ²). `x >= 0`, `df > 0`. |
| `stats.fCdf(x, df1, df2)` / `fQuantile(p, df1, df2)` | *F* distribution. `x >= 0`, dfs `> 0`. |
| `stats.binomialPmf(k, n, p)` / `binomialCdf(k, n, p)` | Binomial. `k`, `n` ints; `0 <= p <= 1`. |
| `stats.poissonPmf(k, lambda)` / `poissonCdf(k, lambda)` | Poisson. `k` int; `lambda > 0`. |

A quantile probability must be in the open interval `(0, 1)` (the boundaries map
to infinity); an out-of-domain parameter is a catchable error, never a `NaN`.

```jennifer
use stats;
stats.normalCdf(1.96, 0, 1);        # 0.975...
stats.normalQuantile(0.975, 0, 1);  # 1.959...
stats.tQuantile(0.975, 10);         # 2.228... (t critical value)
stats.binomialPmf(5, 10, 0.5);      # 0.2461
```

## Inference

Regression, confidence intervals, and hypothesis tests. Results come back as
small structs (`stats.Regression`, `stats.Interval`, `stats.Test`).

| Call | Returns | Meaning |
| ---- | ------- | ------- |
| `stats.linearRegression(xs, ys)` | `Regression` | Simple OLS: `slope`, `intercept`, `r`, `r2`, `stdErr` (of the slope), `pValue`, `n`. |
| `stats.multipleRegression(X, ys)` | `list of float` | Coefficients `[intercept, b1, b2, ...]` for the design rows `X` (normal-equations solve). |
| `stats.confidenceInterval(data, level)` | `Interval` | t-based CI for the mean at confidence `level` in `(0, 1)`. |
| `stats.proportionCi(successes, n, level, method)` | `Interval` | Binomial-proportion interval; `method` is `"wald"`, `"wilson"`, or `"clopper-pearson"` (exact). |
| `stats.tTest(data, mu)` | `Test` | One-sample t-test of the mean against `mu`. |
| `stats.tTest2(a, b)` | `Test` | Two-sample Welch t-test (unequal variances). |
| `stats.chiSquareTest(observed, expected)` | `Test` | Chi-square goodness-of-fit; `expected` counts must be positive. |
| `stats.fTest(a, b)` | `Test` | Two-sided F-test for equal variances. |
| `stats.anova(groups)` | `Test` | One-way ANOVA over a `list of` groups. |
| `stats.histogram(data, binEdges)` | `list of int` | Bin counts (Excel `FREQUENCY`); `k+1` ascending edges give `k` bins, the last closed on the right. |

A `stats.Test` carries `statistic`, `df1`, `df2` (0 for a single-df test like the
t or chi-square; both set for an F / ANOVA), and `pValue`. A degenerate input
(zero variance, a singular design, a non-positive expected count, a negative
observed count, or magnitudes that overflow the computation) is a catchable
error.

A few boundary and numerical notes:

- **Proportion intervals at the extremes.** For `proportionCi` at `successes == 0`
  or `successes == n`, the `"wald"` and `"wilson"` formulas collapse to a
  degenerate point interval (`[0, 0]` / `[1, 1]`) - the well-known boundary defect
  of those methods. Use `"clopper-pearson"` (the exact interval) when a boundary
  count matters; it yields a proper one-sided bound there.
- **Regression conditioning.** `linearRegression` / `multipleRegression` solve the
  normal equations by Gaussian elimination (the same approach as `linalg.solve`).
  That is fast and exact for well-conditioned data, but numerically weak for
  near-collinear predictors or extreme scales; the coefficients can lose precision
  before the design is flagged singular. Center / scale the predictors when they
  span very different magnitudes.
- **Cost.** `multipleRegression` is `O(n*k²)` to form the system and `O(k³)` to
  solve it, for `k` predictors - fine for the modest tabular data a tree-walker
  handles, not a many-thousand-column design.

```jennifer
use stats;
use io;

def r as stats.Regression init stats.linearRegression([1, 2, 3, 4, 5], [2.1, 3.9, 6.2, 7.8, 10.1]);
io.printf("y = %.2fx + %.2f (r2 %.3f, p %.4f)\n", $r.slope, $r.intercept, $r.r2, $r.pValue);

def ci as stats.Interval init stats.proportionCi(8, 10, 0.95, "clopper-pearson");
io.printf("95%% CI: [%.3f, %.3f]\n", $ci.lower, $ci.upper);

def t as stats.Test init stats.tTest([5.1, 4.9, 5.2, 4.8, 5.0], 4.5);
io.printf("t = %.3f, p = %.4f\n", $t.statistic, $t.pValue);
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
