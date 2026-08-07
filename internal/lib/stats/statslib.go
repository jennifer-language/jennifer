// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package statslib implements Jennifer's `stats` library: descriptive statistics
// over a `list of int` or `list of float`. Pure-value and dependency-free (Go
// stdlib only), so both binaries build it.
//
// Real-valued reductions (mean / median / variance / stddev / skewness /
// kurtosis / percentile / correlation / ...) return `float`. Selections that pick
// an element (min / max / mode / range) return that element's kind; `sum` returns
// `int` for an all-int list (overflow-checked) or `float`; `quartiles` / `zscore`
// return `list of float`, `modes` a list of the element kind, and `describe` a
// `stats.Summary` struct. `variance` / `stddev` / `covariance` and the moments are
// POPULATION (divide by n, NumPy's default); the `sample*` forms use n-1.
// `kurtosis` is EXCESS (normal distribution -> 0). An undefined result (empty
// list, out-of-range percentile, zero-variance shape/bivariate input, int-sum
// overflow, non-positive geometric/harmonic input) is a catchable error, not a
// NaN.
package statslib

import (
	"fmt"
	"math"
	"sort"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// isFinite reports whether r is neither NaN nor infinite.
func isFinite(r float64) bool { return !math.IsNaN(r) && !math.IsInf(r, 0) }

// finiteResult wraps a computed float as a value, or - mirroring `math`'s strict
// stance - rejects a non-finite result (NaN or +/-Inf) as a catchable error. A
// finite input list whose magnitudes overflow the intermediate sums / powers
// (e.g. near 1.8e308) is the reachable trigger; without this a `stats` reduction
// could leak a NaN, which would then corrupt comparisons.
func finiteResult(name string, r float64) (interpreter.Value, error) {
	if !isFinite(r) {
		return interpreter.Null(), fmt.Errorf("stats.%s: result is undefined or infinite (input magnitudes overflow the computation)", name)
	}
	return interpreter.FloatVal(r), nil
}

// floatListVal wraps a slice of float64 as a `list of float` value, rejecting a
// non-finite element (see finiteResult).
func floatListVal(name string, fs []float64) (interpreter.Value, error) {
	for _, f := range fs {
		if !isFinite(f) {
			return interpreter.Null(), fmt.Errorf("stats.%s: result is undefined or infinite (input magnitudes overflow the computation)", name)
		}
	}
	return rawFloatListVal(fs), nil
}

// rawFloatListVal wraps a slice of float64 as a `list of float` value.
func rawFloatListVal(fs []float64) interpreter.Value {
	out := make([]interpreter.Value, len(fs))
	for i, f := range fs {
		out[i] = interpreter.FloatVal(f)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeFloat), out)
}

// percentileOf returns the p-th percentile (p in [0, 100]) of an
// ALREADY-SORTED slice by linear interpolation between the two nearest ranks.
func percentileOf(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 1 {
		return sorted[0]
	}
	rank := p / 100 * float64(n-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo] + frac*(sorted[hi]-sorted[lo])
}

// sortedCopy returns a sorted copy of fs (leaving the input untouched).
func sortedCopy(fs []float64) []float64 {
	s := append([]float64(nil), fs...)
	sort.Float64s(s)
	return s
}

// medianOf returns the median of a non-empty slice (a copy is sorted internally).
func medianOf(fs []float64) float64 {
	return percentileOf(sortedCopy(fs), 50)
}

// centralMoment returns the k-th central moment (1/n) * sum((x - mean)^k).
func centralMoment(fs []float64, mean float64, k int) float64 {
	s := 0.0
	for _, f := range fs {
		d := f - mean
		p := 1.0
		for j := 0; j < k; j++ {
			p *= d
		}
		s += p
	}
	return s / float64(len(fs))
}

// LibraryName is the Jennifer name programs `use` to enable these functions and
// the namespace prefix at call sites.
const LibraryName = "stats"

// Install registers the stats builtins.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "mean", meanFn)
	in.RegisterNamespaced(LibraryName, "median", medianFn)
	in.RegisterNamespaced(LibraryName, "mode", modeFn)
	in.RegisterNamespaced(LibraryName, "variance", varianceFn)
	in.RegisterNamespaced(LibraryName, "stddev", stddevFn)
	in.RegisterNamespaced(LibraryName, "percentile", percentileFn)
	in.RegisterNamespaced(LibraryName, "min", minFn)
	in.RegisterNamespaced(LibraryName, "max", maxFn)
	in.RegisterNamespaced(LibraryName, "sum", sumFn)
	in.RegisterNamespaced(LibraryName, "correlation", correlationFn)
	// Sample (Bessel-corrected) spread, additional summaries, and shape helpers.
	in.RegisterNamespaced(LibraryName, "sampleVariance", sampleVarianceFn)
	in.RegisterNamespaced(LibraryName, "sampleStddev", sampleStddevFn)
	in.RegisterNamespaced(LibraryName, "range", rangeFn)
	in.RegisterNamespaced(LibraryName, "covariance", covarianceFn)
	in.RegisterNamespaced(LibraryName, "iqr", iqrFn)
	in.RegisterNamespaced(LibraryName, "quartiles", quartilesFn)
	in.RegisterNamespaced(LibraryName, "zscore", zscoreFn)
	in.RegisterNamespaced(LibraryName, "geometricMean", geometricMeanFn)
	in.RegisterNamespaced(LibraryName, "harmonicMean", harmonicMeanFn)
	// Shape, robust spread, weighting, multimodality, and a one-call summary.
	in.RegisterNamespaced(LibraryName, "skewness", skewnessFn)
	in.RegisterNamespaced(LibraryName, "kurtosis", kurtosisFn)
	in.RegisterNamespaced(LibraryName, "sampleCovariance", sampleCovarianceFn)
	in.RegisterNamespaced(LibraryName, "mad", madFn)
	in.RegisterNamespaced(LibraryName, "weightedMean", weightedMeanFn)
	in.RegisterNamespaced(LibraryName, "modes", modesFn)
	in.RegisterNamespaced(LibraryName, "describe", describeFn)

	// Distributions (pdf / pmf, cdf, quantile, sample) - the probability layer,
	// built on `math`'s regularized incomplete gamma / beta. Flat R-style names.
	in.RegisterNamespaced(LibraryName, "normalPdf", normalPdfFn)
	in.RegisterNamespaced(LibraryName, "normalCdf", normalCdfFn)
	in.RegisterNamespaced(LibraryName, "normalQuantile", normalQuantileFn)
	in.RegisterNamespaced(LibraryName, "normalSample", normalSampleFn)
	in.RegisterNamespaced(LibraryName, "tPdf", tPdfFn)
	in.RegisterNamespaced(LibraryName, "tCdf", tCdfFn)
	in.RegisterNamespaced(LibraryName, "tQuantile", tQuantileFn)
	in.RegisterNamespaced(LibraryName, "chiSquareCdf", chiSquareCdfFn)
	in.RegisterNamespaced(LibraryName, "chiSquareQuantile", chiSquareQuantileFn)
	in.RegisterNamespaced(LibraryName, "fCdf", fCdfFn)
	in.RegisterNamespaced(LibraryName, "fQuantile", fQuantileFn)
	in.RegisterNamespaced(LibraryName, "binomialPmf", binomialPmfFn)
	in.RegisterNamespaced(LibraryName, "binomialCdf", binomialCdfFn)
	in.RegisterNamespaced(LibraryName, "poissonPmf", poissonPmfFn)
	in.RegisterNamespaced(LibraryName, "poissonCdf", poissonCdfFn)

	// Inference: regression, confidence intervals, hypothesis tests, histogram.
	in.RegisterNamespaced(LibraryName, "linearRegression", linearRegressionFn)
	in.RegisterNamespaced(LibraryName, "multipleRegression", multipleRegressionFn)
	in.RegisterNamespaced(LibraryName, "confidenceInterval", confidenceIntervalFn)
	in.RegisterNamespaced(LibraryName, "proportionCi", proportionCiFn)
	in.RegisterNamespaced(LibraryName, "tTest", tTestFn)
	in.RegisterNamespaced(LibraryName, "tTest2", tTest2Fn)
	in.RegisterNamespaced(LibraryName, "chiSquareTest", chiSquareTestFn)
	in.RegisterNamespaced(LibraryName, "fTest", fTestFn)
	in.RegisterNamespaced(LibraryName, "anova", anovaFn)
	in.RegisterNamespaced(LibraryName, "histogram", histogramFn)
	registerInferenceStructs(in)

	// stats.Summary is the struct describe() returns: the count plus the
	// five-number summary, the mean, and the population standard deviation.
	fl := parser.PrimitiveType(parser.TypeFloat)
	in.RegisterNamespacedStruct(LibraryName, "Summary", []parser.StructField{
		{Name: "count", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "min", Type: fl},
		{Name: "q1", Type: fl},
		{Name: "median", Type: fl},
		{Name: "mean", Type: fl},
		{Name: "q3", Type: fl},
		{Name: "max", Type: fl},
		{Name: "stddev", Type: fl},
	})
}

// numbers validates that args[i] is a list whose every element is int or float,
// returning the original values (so a selection can preserve int/float kind) and
// their float64 view (for arithmetic).
func numbers(name string, args []interpreter.Value, i int) ([]interpreter.Value, []float64, error) {
	v := args[i]
	if v.Kind != interpreter.KindList {
		return nil, nil, fmt.Errorf("stats.%s: argument must be a list of int or float, got %s", name, v.Kind)
	}
	raw := v.List
	fs := make([]float64, len(raw))
	for j, e := range raw {
		f, ok := e.AsFloat()
		if !ok {
			return nil, nil, fmt.Errorf("stats.%s: element %d must be int or float, got %s", name, j, e.Kind)
		}
		fs[j] = f
	}
	return raw, fs, nil
}

// arity1 checks that a single-list function received exactly one argument. The
// two-argument functions (correlation / covariance / weightedMean / ...) check
// their own count inline.
func arity1(name string, args []interpreter.Value) error {
	if len(args) != 1 {
		return fmt.Errorf("stats.%s expects 1 argument (list), got %d", name, len(args))
	}
	return nil
}

func meanOf(fs []float64) float64 {
	s := 0.0
	for _, f := range fs {
		s += f
	}
	return s / float64(len(fs))
}

// popVariance is the population variance (sum of squared deviations / n).
func popVariance(fs []float64) float64 {
	m := meanOf(fs)
	s := 0.0
	for _, f := range fs {
		d := f - m
		s += d * d
	}
	return s / float64(len(fs))
}

func meanFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("mean", args); err != nil {
		return interpreter.Null(), err
	}
	_, fs, err := numbers("mean", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.mean: list is empty")
	}
	return finiteResult("mean", meanOf(fs))
}

func medianFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("median", args); err != nil {
		return interpreter.Null(), err
	}
	_, fs, err := numbers("median", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.median: list is empty")
	}
	sorted := append([]float64(nil), fs...)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return finiteResult("median", sorted[n/2])
	}
	return finiteResult("median", (sorted[n/2-1]+sorted[n/2])/2)
}

// modeFn returns the most frequent element (by exact equality), preserving its
// int/float kind. On a tie the first element to reach the winning count wins.
func modeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("mode", args); err != nil {
		return interpreter.Null(), err
	}
	raw, fs, err := numbers("mode", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.mode: list is empty")
	}
	counts := make(map[float64]int, len(fs))
	bestIdx, bestCount := 0, 0
	for i, f := range fs {
		counts[f]++
		if counts[f] > bestCount {
			bestCount = counts[f]
			bestIdx = i
		}
	}
	return raw[bestIdx].Copy(), nil
}

func varianceFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("variance", args); err != nil {
		return interpreter.Null(), err
	}
	_, fs, err := numbers("variance", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.variance: list is empty")
	}
	return finiteResult("variance", popVariance(fs))
}

func stddevFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("stddev", args); err != nil {
		return interpreter.Null(), err
	}
	_, fs, err := numbers("stddev", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.stddev: list is empty")
	}
	return finiteResult("stddev", math.Sqrt(popVariance(fs)))
}

// percentileFn returns the p-th percentile (p in [0, 100]) by linear
// interpolation between the two nearest ranks (NumPy's default method).
func percentileFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("stats.percentile expects 2 arguments (list, p), got %d", len(args))
	}
	_, fs, err := numbers("percentile", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	p, ok := args[1].AsFloat()
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.percentile: p must be int or float, got %s", args[1].Kind)
	}
	if len(fs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.percentile: list is empty")
	}
	if p < 0 || p > 100 {
		return interpreter.Null(), fmt.Errorf("stats.percentile: p must be in [0, 100], got %s", interpreter.DisplayFloat(p))
	}
	return finiteResult("percentile", percentileOf(sortedCopy(fs), p))
}

// minFn / maxFn return the smallest / largest element, preserving its kind.
func minFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return extremum("min", args, false)
}

func maxFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return extremum("max", args, true)
}

func extremum(name string, args []interpreter.Value, wantMax bool) (interpreter.Value, error) {
	if err := arity1(name, args); err != nil {
		return interpreter.Null(), err
	}
	raw, fs, err := numbers(name, args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.%s: list is empty", name)
	}
	bestIdx := 0
	for i := 1; i < len(fs); i++ {
		if (wantMax && fs[i] > fs[bestIdx]) || (!wantMax && fs[i] < fs[bestIdx]) {
			bestIdx = i
		}
	}
	return raw[bestIdx].Copy(), nil
}

// sumFn returns the sum: int (overflow-checked) for an all-int list, else float.
// The sum of an empty list is int 0.
func sumFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("sum", args); err != nil {
		return interpreter.Null(), err
	}
	raw, fs, err := numbers("sum", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	allInt := true
	for _, e := range raw {
		if e.Kind != interpreter.KindInt {
			allInt = false
			break
		}
	}
	if allInt {
		var s int64
		for _, e := range raw {
			t := s + e.Int
			// Same-sign operands whose sum flips sign overflowed int64.
			if (s > 0 && e.Int > 0 && t < 0) || (s < 0 && e.Int < 0 && t >= 0) {
				return interpreter.Null(), fmt.Errorf("stats.sum: integer overflow")
			}
			s = t
		}
		return interpreter.IntVal(s), nil
	}
	s := 0.0
	for _, f := range fs {
		s += f
	}
	return finiteResult("sum", s)
}

// correlationFn returns the Pearson correlation coefficient of two equal-length
// lists (>= 2 elements, each with non-zero spread), in [-1, 1].
func correlationFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("stats.correlation expects 2 arguments (list, list), got %d", len(args))
	}
	_, xs, err := numbers("correlation", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	_, ys, err := numbers("correlation", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(xs) != len(ys) {
		return interpreter.Null(), fmt.Errorf("stats.correlation: lists must be the same length (%d vs %d)", len(xs), len(ys))
	}
	if len(xs) < 2 {
		return interpreter.Null(), fmt.Errorf("stats.correlation: need at least 2 elements, got %d", len(xs))
	}
	mx, my := meanOf(xs), meanOf(ys)
	var cov, sx, sy float64
	for i := range xs {
		dx, dy := xs[i]-mx, ys[i]-my
		cov += dx * dy
		sx += dx * dx
		sy += dy * dy
	}
	if sx == 0 || sy == 0 {
		return interpreter.Null(), fmt.Errorf("stats.correlation: undefined when a list has zero variance")
	}
	r := cov / math.Sqrt(sx*sy)
	// Clamp tiny floating-point excursions past the mathematical [-1, 1] range.
	if r > 1 {
		r = 1
	} else if r < -1 {
		r = -1
	}
	return finiteResult("correlation", r)
}

// sampleVarianceOf is the sample (Bessel-corrected) variance: sum of squared
// deviations / (n-1). The caller guarantees len(fs) >= 2.
func sampleVarianceOf(fs []float64) float64 {
	m := meanOf(fs)
	s := 0.0
	for _, f := range fs {
		d := f - m
		s += d * d
	}
	return s / float64(len(fs)-1)
}

// sampleVarianceFn / sampleStddevFn are the n-1 (unbiased, inferential) companions
// to the population `variance` / `stddev`; they need at least 2 elements.
func sampleVarianceFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("sampleVariance", args); err != nil {
		return interpreter.Null(), err
	}
	_, fs, err := numbers("sampleVariance", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) < 2 {
		return interpreter.Null(), fmt.Errorf("stats.sampleVariance: need at least 2 elements, got %d", len(fs))
	}
	return finiteResult("sampleVariance", sampleVarianceOf(fs))
}

func sampleStddevFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("sampleStddev", args); err != nil {
		return interpreter.Null(), err
	}
	_, fs, err := numbers("sampleStddev", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) < 2 {
		return interpreter.Null(), fmt.Errorf("stats.sampleStddev: need at least 2 elements, got %d", len(fs))
	}
	return finiteResult("sampleStddev", math.Sqrt(sampleVarianceOf(fs)))
}

// rangeFn returns max - min (the spread), preserving the input kind: int for an
// all-int list (overflow-checked), float otherwise. Empty list errors.
func rangeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("range", args); err != nil {
		return interpreter.Null(), err
	}
	raw, fs, err := numbers("range", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.range: list is empty")
	}
	loIdx, hiIdx := 0, 0
	for i := 1; i < len(fs); i++ {
		if fs[i] < fs[loIdx] {
			loIdx = i
		}
		if fs[i] > fs[hiIdx] {
			hiIdx = i
		}
	}
	allInt := true
	for _, e := range raw {
		if e.Kind != interpreter.KindInt {
			allInt = false
			break
		}
	}
	if allInt {
		hi, lo := raw[hiIdx].Int, raw[loIdx].Int
		d := hi - lo
		// hi >= lo, so overflow only when a large-positive minus a large-negative
		// wraps to a negative difference.
		if hi >= 0 && lo < 0 && d < 0 {
			return interpreter.Null(), fmt.Errorf("stats.range: integer overflow")
		}
		return interpreter.IntVal(d), nil
	}
	return finiteResult("range", fs[hiIdx]-fs[loIdx])
}

// covarianceFn returns the POPULATION covariance (sum of paired deviations / n) of
// two equal-length, non-empty lists - the raw, units-aware companion to the
// normalized `correlation` (which divides by the standard deviations). It matches
// `variance`'s population convention; `sampleVariance` has no covariance analogue.
func covarianceFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("stats.covariance expects 2 arguments (list, list), got %d", len(args))
	}
	_, xs, err := numbers("covariance", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	_, ys, err := numbers("covariance", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(xs) != len(ys) {
		return interpreter.Null(), fmt.Errorf("stats.covariance: lists must be the same length (%d vs %d)", len(xs), len(ys))
	}
	if len(xs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.covariance: lists are empty")
	}
	mx, my := meanOf(xs), meanOf(ys)
	var cov float64
	for i := range xs {
		cov += (xs[i] - mx) * (ys[i] - my)
	}
	return finiteResult("covariance", cov/float64(len(xs)))
}

// iqrFn returns the interquartile range: the 75th percentile minus the 25th.
func iqrFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("iqr", args); err != nil {
		return interpreter.Null(), err
	}
	_, fs, err := numbers("iqr", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.iqr: list is empty")
	}
	sorted := sortedCopy(fs)
	return finiteResult("iqr", percentileOf(sorted, 75)-percentileOf(sorted, 25))
}

// quartilesFn returns the three quartiles [Q1, Q2, Q3] = [p25, p50, p75] as a
// `list of float`.
func quartilesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("quartiles", args); err != nil {
		return interpreter.Null(), err
	}
	_, fs, err := numbers("quartiles", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.quartiles: list is empty")
	}
	sorted := sortedCopy(fs)
	return floatListVal("quartiles", []float64{
		percentileOf(sorted, 25),
		percentileOf(sorted, 50),
		percentileOf(sorted, 75),
	})
}

// zscoreFn standardizes each element to (x - mean) / stddev (population), returning
// a `list of float`. A constant list (zero stddev) has no defined z-score.
func zscoreFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("zscore", args); err != nil {
		return interpreter.Null(), err
	}
	_, fs, err := numbers("zscore", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.zscore: list is empty")
	}
	m := meanOf(fs)
	sd := math.Sqrt(popVariance(fs))
	if sd == 0 {
		return interpreter.Null(), fmt.Errorf("stats.zscore: undefined for a constant list (zero standard deviation)")
	}
	out := make([]float64, len(fs))
	for i, f := range fs {
		out[i] = (f - m) / sd
	}
	return floatListVal("zscore", out)
}

// geometricMeanFn returns the geometric mean (the n-th root of the product),
// computed via log-sum-exp so a long list does not overflow the product. Every
// element must be strictly positive.
func geometricMeanFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("geometricMean", args); err != nil {
		return interpreter.Null(), err
	}
	_, fs, err := numbers("geometricMean", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.geometricMean: list is empty")
	}
	sumLog := 0.0
	for i, f := range fs {
		if f <= 0 {
			return interpreter.Null(), fmt.Errorf("stats.geometricMean: every element must be > 0 (element %d is %s)", i, interpreter.DisplayFloat(f))
		}
		sumLog += math.Log(f)
	}
	return finiteResult("geometricMean", math.Exp(sumLog/float64(len(fs))))
}

// harmonicMeanFn returns the harmonic mean (n / sum of reciprocals). Every element
// must be strictly positive.
func harmonicMeanFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("harmonicMean", args); err != nil {
		return interpreter.Null(), err
	}
	_, fs, err := numbers("harmonicMean", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.harmonicMean: list is empty")
	}
	sumRecip := 0.0
	for i, f := range fs {
		if f <= 0 {
			return interpreter.Null(), fmt.Errorf("stats.harmonicMean: every element must be > 0 (element %d is %s)", i, interpreter.DisplayFloat(f))
		}
		sumRecip += 1 / f
	}
	return finiteResult("harmonicMean", float64(len(fs))/sumRecip)
}

// skewnessFn returns the population skewness (the standardized 3rd central moment,
// Fisher-Pearson g1): 0 for a symmetric distribution, positive for a right tail.
func skewnessFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("skewness", args); err != nil {
		return interpreter.Null(), err
	}
	_, fs, err := numbers("skewness", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) < 2 {
		return interpreter.Null(), fmt.Errorf("stats.skewness: need at least 2 elements, got %d", len(fs))
	}
	m := meanOf(fs)
	m2 := centralMoment(fs, m, 2)
	if m2 == 0 {
		return interpreter.Null(), fmt.Errorf("stats.skewness: undefined for a constant list (zero variance)")
	}
	m3 := centralMoment(fs, m, 3)
	return finiteResult("skewness", m3/(m2*math.Sqrt(m2)))
}

// kurtosisFn returns the **excess** kurtosis (standardized 4th central moment minus
// 3), so a normal distribution is 0; positive means heavier tails than normal.
func kurtosisFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("kurtosis", args); err != nil {
		return interpreter.Null(), err
	}
	_, fs, err := numbers("kurtosis", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) < 2 {
		return interpreter.Null(), fmt.Errorf("stats.kurtosis: need at least 2 elements, got %d", len(fs))
	}
	m := meanOf(fs)
	m2 := centralMoment(fs, m, 2)
	if m2 == 0 {
		return interpreter.Null(), fmt.Errorf("stats.kurtosis: undefined for a constant list (zero variance)")
	}
	m4 := centralMoment(fs, m, 4)
	return finiteResult("kurtosis", m4/(m2*m2)-3)
}

// sampleCovarianceFn is the sample (Bessel-corrected, ÷ n-1) covariance, the
// companion to sampleVariance. Equal-length lists, >= 2 elements.
func sampleCovarianceFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("stats.sampleCovariance expects 2 arguments (list, list), got %d", len(args))
	}
	_, xs, err := numbers("sampleCovariance", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	_, ys, err := numbers("sampleCovariance", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(xs) != len(ys) {
		return interpreter.Null(), fmt.Errorf("stats.sampleCovariance: lists must be the same length (%d vs %d)", len(xs), len(ys))
	}
	if len(xs) < 2 {
		return interpreter.Null(), fmt.Errorf("stats.sampleCovariance: need at least 2 elements, got %d", len(xs))
	}
	mx, my := meanOf(xs), meanOf(ys)
	var cov float64
	for i := range xs {
		cov += (xs[i] - mx) * (ys[i] - my)
	}
	return finiteResult("sampleCovariance", cov/float64(len(xs)-1))
}

// madFn returns the median absolute deviation: median(|x - median(xs)|), a robust
// (outlier-resistant) measure of spread.
func madFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("mad", args); err != nil {
		return interpreter.Null(), err
	}
	_, fs, err := numbers("mad", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.mad: list is empty")
	}
	med := medianOf(fs)
	dev := make([]float64, len(fs))
	for i, f := range fs {
		dev[i] = math.Abs(f - med)
	}
	return finiteResult("mad", medianOf(dev))
}

// weightedMeanFn returns the weighted mean sum(x_i * w_i) / sum(w_i). The value and
// weight lists must be the same non-empty length, and the weights must not sum to
// zero.
func weightedMeanFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("stats.weightedMean expects 2 arguments (values, weights), got %d", len(args))
	}
	_, xs, err := numbers("weightedMean", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	_, ws, err := numbers("weightedMean", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(xs) != len(ws) {
		return interpreter.Null(), fmt.Errorf("stats.weightedMean: values and weights must be the same length (%d vs %d)", len(xs), len(ws))
	}
	if len(xs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.weightedMean: lists are empty")
	}
	var num, den float64
	for i := range xs {
		num += xs[i] * ws[i]
		den += ws[i]
	}
	if den == 0 {
		return interpreter.Null(), fmt.Errorf("stats.weightedMean: weights sum to zero")
	}
	return finiteResult("weightedMean", num/den)
}

// modesFn returns every modal value (all elements tied for the highest frequency),
// in first-encountered order, preserving the input kind. `mode` returns just the
// first of these.
func modesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("modes", args); err != nil {
		return interpreter.Null(), err
	}
	raw, fs, err := numbers("modes", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.modes: list is empty")
	}
	counts := make(map[float64]int, len(fs))
	maxCount := 0
	for _, f := range fs {
		counts[f]++
		if counts[f] > maxCount {
			maxCount = counts[f]
		}
	}
	seen := make(map[float64]bool, len(counts))
	out := make([]interpreter.Value, 0)
	for i, f := range fs {
		if counts[f] == maxCount && !seen[f] {
			seen[f] = true
			out = append(out, raw[i].Copy())
		}
	}
	// A generic list (no recorded element type) is validated element-by-element at
	// the binding site, so a `list of int` result binds to `list of int`.
	return interpreter.Value{Kind: interpreter.KindList, List: out}, nil
}

// describeFn returns a stats.Summary struct: the count, the five-number summary
// (min / Q1 / median / Q3 / max), the mean, and the population standard deviation.
func describeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := arity1("describe", args); err != nil {
		return interpreter.Null(), err
	}
	_, fs, err := numbers("describe", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(fs) == 0 {
		return interpreter.Null(), fmt.Errorf("stats.describe: list is empty")
	}
	sorted := sortedCopy(fs)
	// The mean / stddev / percentile fields can overflow to +/-Inf (or NaN) on
	// magnitudes near 1.8e308; reject a non-finite summary rather than embedding it.
	stats := map[string]float64{
		"min":    sorted[0],
		"q1":     percentileOf(sorted, 25),
		"median": percentileOf(sorted, 50),
		"mean":   meanOf(fs),
		"q3":     percentileOf(sorted, 75),
		"max":    sorted[len(sorted)-1],
		"stddev": math.Sqrt(popVariance(fs)),
	}
	for _, v := range stats {
		if !isFinite(v) {
			return interpreter.Null(), fmt.Errorf("stats.describe: result is undefined or infinite (input magnitudes overflow the computation)")
		}
	}
	return interpreter.NamespacedStructVal("stats", "Summary", []interpreter.StructField{
		{Name: "count", Value: interpreter.IntVal(int64(len(fs)))},
		{Name: "min", Value: interpreter.FloatVal(stats["min"])},
		{Name: "q1", Value: interpreter.FloatVal(stats["q1"])},
		{Name: "median", Value: interpreter.FloatVal(stats["median"])},
		{Name: "mean", Value: interpreter.FloatVal(stats["mean"])},
		{Name: "q3", Value: interpreter.FloatVal(stats["q3"])},
		{Name: "max", Value: interpreter.FloatVal(stats["max"])},
		{Name: "stddev", Value: interpreter.FloatVal(stats["stddev"])},
	}), nil
}
