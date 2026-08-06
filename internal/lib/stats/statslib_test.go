// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package statslib

import (
	"math"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func intList(ns ...int64) interpreter.Value {
	xs := make([]interpreter.Value, len(ns))
	for i, n := range ns {
		xs[i] = interpreter.IntVal(n)
	}
	return interpreter.Value{Kind: interpreter.KindList, List: xs}
}

func floatList(fs ...float64) interpreter.Value {
	xs := make([]interpreter.Value, len(fs))
	for i, f := range fs {
		xs[i] = interpreter.FloatVal(f)
	}
	return interpreter.Value{Kind: interpreter.KindList, List: xs}
}

func call(t *testing.T, fn interpreter.Builtin, args ...interpreter.Value) interpreter.Value {
	t.Helper()
	v, err := fn(interpreter.BuiltinCtx{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return v
}

func wantFloat(t *testing.T, v interpreter.Value, want float64) {
	t.Helper()
	if v.Kind != interpreter.KindFloat {
		t.Fatalf("expected float, got %s", v.Kind)
	}
	if math.Abs(v.Float-want) > 1e-9 {
		t.Errorf("got %v, want %v", v.Float, want)
	}
}

// TestReductions covers mean / median / variance / stddev / percentile on a known
// data set (the classic [2,4,4,4,5,5,7,9]: mean 5, pop-variance 4, stddev 2).
func TestReductions(t *testing.T) {
	xs := intList(2, 4, 4, 4, 5, 5, 7, 9)
	wantFloat(t, call(t, meanFn, xs), 5.0)
	wantFloat(t, call(t, medianFn, xs), 4.5)
	wantFloat(t, call(t, varianceFn, xs), 4.0)
	wantFloat(t, call(t, stddevFn, xs), 2.0)
	wantFloat(t, call(t, percentileFn, xs, interpreter.IntVal(50)), 4.5)
	wantFloat(t, call(t, percentileFn, xs, interpreter.IntVal(90)), 7.6)
	wantFloat(t, call(t, percentileFn, xs, interpreter.IntVal(0)), 2.0)
	wantFloat(t, call(t, percentileFn, xs, interpreter.IntVal(100)), 9.0)
}

// TestSelectionsPreserveKind: min / max / mode / sum keep the element kind.
func TestSelectionsPreserveKind(t *testing.T) {
	xs := intList(2, 4, 4, 4, 5, 5, 7, 9)
	if v := call(t, minFn, xs); v.Kind != interpreter.KindInt || v.Int != 2 {
		t.Errorf("min = %v, want int 2", v)
	}
	if v := call(t, maxFn, xs); v.Kind != interpreter.KindInt || v.Int != 9 {
		t.Errorf("max = %v, want int 9", v)
	}
	if v := call(t, modeFn, xs); v.Kind != interpreter.KindInt || v.Int != 4 {
		t.Errorf("mode = %v, want int 4", v)
	}
	if v := call(t, sumFn, xs); v.Kind != interpreter.KindInt || v.Int != 40 {
		t.Errorf("sum = %v, want int 40", v)
	}
	// Float list: sum stays float, min preserves float.
	fs := floatList(1.5, 2.5, 3.0)
	if v := call(t, sumFn, fs); v.Kind != interpreter.KindFloat || math.Abs(v.Float-7.0) > 1e-9 {
		t.Errorf("float sum = %v, want float 7.0", v)
	}
	if v := call(t, minFn, fs); v.Kind != interpreter.KindFloat {
		t.Errorf("float min kind = %s, want float", v.Kind)
	}
}

// TestCorrelation: perfect positive / negative / with promotion.
func TestCorrelation(t *testing.T) {
	wantFloat(t, call(t, correlationFn, floatList(1, 2, 3, 4), floatList(2, 4, 6, 8)), 1.0)
	wantFloat(t, call(t, correlationFn, floatList(1, 2, 3, 4), floatList(8, 6, 4, 2)), -1.0)
	// int lists promote to float
	wantFloat(t, call(t, correlationFn, intList(1, 2, 3), intList(1, 2, 3)), 1.0)
}

// TestSumOverflowCatchable: an int sum that overflows int64 is an error, not a
// silent wrap.
func TestSumOverflowCatchable(t *testing.T) {
	if _, err := sumFn(interpreter.BuiltinCtx{}, []interpreter.Value{intList(math.MaxInt64, 1)}); err == nil {
		t.Fatal("expected an integer-overflow error")
	}
}

// TestErrors: the boundary rejections are all errors, not panics or NaN.
func TestErrors(t *testing.T) {
	empty := intList()
	cases := []struct {
		name string
		fn   func() (interpreter.Value, error)
	}{
		{"mean empty", func() (interpreter.Value, error) { return meanFn(interpreter.BuiltinCtx{}, []interpreter.Value{empty}) }},
		{"median empty", func() (interpreter.Value, error) {
			return medianFn(interpreter.BuiltinCtx{}, []interpreter.Value{empty})
		}},
		{"mode empty", func() (interpreter.Value, error) { return modeFn(interpreter.BuiltinCtx{}, []interpreter.Value{empty}) }},
		{"min empty", func() (interpreter.Value, error) { return minFn(interpreter.BuiltinCtx{}, []interpreter.Value{empty}) }},
		{"percentile out of range", func() (interpreter.Value, error) {
			return percentileFn(interpreter.BuiltinCtx{}, []interpreter.Value{intList(1, 2, 3), interpreter.IntVal(150)})
		}},
		{"correlation length mismatch", func() (interpreter.Value, error) {
			return correlationFn(interpreter.BuiltinCtx{}, []interpreter.Value{intList(1, 2, 3), intList(1, 2)})
		}},
		{"correlation zero variance", func() (interpreter.Value, error) {
			return correlationFn(interpreter.BuiltinCtx{}, []interpreter.Value{intList(1, 1, 1), intList(1, 2, 3)})
		}},
		{"non-numeric element", func() (interpreter.Value, error) {
			return meanFn(interpreter.BuiltinCtx{}, []interpreter.Value{{Kind: interpreter.KindList, List: []interpreter.Value{interpreter.StringVal("x")}}})
		}},
		{"not a list", func() (interpreter.Value, error) {
			return meanFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(5)})
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := c.fn(); err == nil {
				t.Errorf("expected an error")
			}
		})
	}
}

// TestNonFiniteResultsRejected: a finite input whose magnitudes overflow the
// intermediate computation must yield a catchable error, not a NaN / Inf that
// could escape into the type system and corrupt comparisons.
func TestNonFiniteResultsRejected(t *testing.T) {
	huge := math.Pow(2, 1023) // 8.988e307, finite but near the float64 ceiling
	big3 := floatList(huge, huge, huge)
	two := floatList(huge, -huge)
	single := []struct {
		name string
		fn   interpreter.Builtin
		arg  interpreter.Value
	}{
		// Sum-based reductions overflow their intermediates on huge magnitudes.
		// (percentile / quartiles / median are convex combinations bounded by the
		// input range, so they stay finite here - their guard is defensive.)
		{"mean", meanFn, big3}, {"variance", varianceFn, big3}, {"stddev", stddevFn, big3},
		{"skewness", skewnessFn, two}, {"kurtosis", kurtosisFn, two},
		{"zscore", zscoreFn, big3}, {"describe", describeFn, big3},
		{"sum", sumFn, big3},
	}
	for _, c := range single {
		if _, err := c.fn(interpreter.BuiltinCtx{}, []interpreter.Value{c.arg}); err == nil {
			t.Errorf("%s over huge magnitudes should error (non-finite result), got none", c.name)
		}
	}
	// Two-list forms.
	if _, err := correlationFn(interpreter.BuiltinCtx{}, []interpreter.Value{big3, big3}); err == nil {
		t.Error("correlation over huge magnitudes should error")
	}
	if _, err := weightedMeanFn(interpreter.BuiltinCtx{}, []interpreter.Value{floatList(8.9), floatList(huge)}); err == nil {
		t.Error("weightedMean with an overflowing numerator should error")
	}
	// A normal input still succeeds (no false positives).
	if _, err := meanFn(interpreter.BuiltinCtx{}, []interpreter.Value{intList(1, 2, 3)}); err != nil {
		t.Errorf("mean of a normal list should succeed, got %v", err)
	}
}

// TestSumEmptyIsZero: the sum of an empty list is int 0 (well-defined).
func TestSumEmptyIsZero(t *testing.T) {
	if v := call(t, sumFn, intList()); v.Kind != interpreter.KindInt || v.Int != 0 {
		t.Errorf("sum([]) = %v, want int 0", v)
	}
}

// TestSampleVsPopulation: sample variance/stddev use n-1, population use n.
func TestSampleVsPopulation(t *testing.T) {
	xs := intList(2, 4, 4, 4, 5, 5, 7, 9)                        // sum sq dev = 32, n = 8
	wantFloat(t, call(t, varianceFn, xs), 4.0)                   // 32/8
	wantFloat(t, call(t, sampleVarianceFn, xs), 32.0/7.0)        // 32/7
	wantFloat(t, call(t, sampleStddevFn, xs), math.Sqrt(32.0/7)) // sqrt(32/7)
	// Sample variants need >= 2 elements.
	if _, err := sampleVarianceFn(interpreter.BuiltinCtx{}, []interpreter.Value{intList(5)}); err == nil {
		t.Error("sampleVariance of one element should error")
	}
}

// TestRangePreservesKind: range = max - min, int for an int list.
func TestRangePreservesKind(t *testing.T) {
	if v := call(t, rangeFn, intList(2, 9, 5)); v.Kind != interpreter.KindInt || v.Int != 7 {
		t.Errorf("range = %v, want int 7", v)
	}
	if v := call(t, rangeFn, floatList(1.5, 4.0)); v.Kind != interpreter.KindFloat || math.Abs(v.Float-2.5) > 1e-9 {
		t.Errorf("float range = %v, want float 2.5", v)
	}
}

// TestCovariance: population covariance of a perfectly linear pair.
func TestCovariance(t *testing.T) {
	wantFloat(t, call(t, covarianceFn, floatList(1, 2, 3, 4), floatList(2, 4, 6, 8)), 2.5)
}

// TestQuartilesIqrZscore: shape helpers returning lists / an interquartile range.
func TestQuartilesIqrZscore(t *testing.T) {
	xs := intList(2, 4, 4, 4, 5, 5, 7, 9)
	wantFloat(t, call(t, iqrFn, xs), 1.5) // p75 5.5 - p25 4.0
	q := call(t, quartilesFn, xs)
	if q.Kind != interpreter.KindList || len(q.List) != 3 {
		t.Fatalf("quartiles = %v, want a 3-element list", q)
	}
	wantFloat(t, q.List[0], 4.0)
	wantFloat(t, q.List[1], 4.5)
	wantFloat(t, q.List[2], 5.5)
	z := call(t, zscoreFn, xs)
	if z.Kind != interpreter.KindList || len(z.List) != 8 {
		t.Fatalf("zscore = %v, want an 8-element list", z)
	}
	wantFloat(t, z.List[0], -1.5) // (2-5)/2
	// A constant list has no z-score.
	if _, err := zscoreFn(interpreter.BuiltinCtx{}, []interpreter.Value{intList(3, 3, 3)}); err == nil {
		t.Error("zscore of a constant list should error")
	}
}

// TestSkewnessKurtosis: sign of skew, excess kurtosis (normal -> 0), constant errors.
func TestSkewnessKurtosis(t *testing.T) {
	// A right-skewed set (one large outlier) has positive skewness.
	if v := call(t, skewnessFn, intList(1, 2, 2, 3, 100)); v.Float <= 0 {
		t.Errorf("skewness = %v, want positive", v.Float)
	}
	// A symmetric set has ~zero skewness.
	wantFloat(t, call(t, skewnessFn, floatList(1, 2, 3, 4, 5)), 0.0)
	// Excess kurtosis of a uniform-ish symmetric small set is negative (platykurtic).
	if v := call(t, kurtosisFn, floatList(1, 2, 3, 4, 5)); v.Float >= 0 {
		t.Errorf("excess kurtosis = %v, want negative for this set", v.Float)
	}
	for _, fn := range []interpreter.Builtin{skewnessFn, kurtosisFn} {
		if _, err := fn(interpreter.BuiltinCtx{}, []interpreter.Value{intList(3, 3, 3)}); err == nil {
			t.Error("shape stat of a constant list should error")
		}
	}
}

// TestSampleCovarianceMadWeightedModes: the remaining additions.
func TestSampleCovarianceMadWeightedModes(t *testing.T) {
	// Sample covariance = population * n/(n-1); pop cov of the linear pair is 2.5.
	wantFloat(t, call(t, sampleCovarianceFn, floatList(1, 2, 3, 4), floatList(2, 4, 6, 8)), 2.5*4/3)
	// mad([1,2,2,3,3,3,100]): median 3, |dev| median = 1.
	wantFloat(t, call(t, madFn, intList(1, 2, 2, 3, 3, 3, 100)), 1.0)
	// weighted mean of [90,80,70] with weights [3,1,1] = 420/5 = 84.
	wantFloat(t, call(t, weightedMeanFn, floatList(90, 80, 70), floatList(3, 1, 1)), 84.0)
	if _, err := weightedMeanFn(interpreter.BuiltinCtx{}, []interpreter.Value{floatList(1, 2), floatList(1, -1)}); err == nil {
		t.Error("weightedMean with zero-sum weights should error")
	}
	// modes returns every tied value in first-encountered order, preserving kind.
	m := call(t, modesFn, intList(2, 2, 5, 5, 7))
	if m.Kind != interpreter.KindList || len(m.List) != 2 || m.List[0].Int != 2 || m.List[1].Int != 5 {
		t.Errorf("modes = %v, want int list [2, 5]", m)
	}
}

// TestDescribe: the one-call Summary struct.
func TestDescribe(t *testing.T) {
	d := call(t, describeFn, intList(2, 4, 4, 4, 5, 5, 7, 9))
	if d.Kind != interpreter.KindStruct || d.StructName != "Summary" || d.StructNS != "stats" {
		t.Fatalf("describe = %v, want a stats.Summary struct", d)
	}
	fields := map[string]interpreter.Value{}
	for _, f := range d.Fields {
		fields[f.Name] = f.Value
	}
	if fields["count"].Kind != interpreter.KindInt || fields["count"].Int != 8 {
		t.Errorf("count = %v, want int 8", fields["count"])
	}
	wantFloat(t, fields["min"], 2.0)
	wantFloat(t, fields["median"], 4.5)
	wantFloat(t, fields["mean"], 5.0)
	wantFloat(t, fields["max"], 9.0)
	wantFloat(t, fields["stddev"], 2.0)
	if _, err := describeFn(interpreter.BuiltinCtx{}, []interpreter.Value{intList()}); err == nil {
		t.Error("describe of an empty list should error")
	}
}

// TestGeometricHarmonicMean: values and the positivity requirement.
func TestGeometricHarmonicMean(t *testing.T) {
	wantFloat(t, call(t, geometricMeanFn, floatList(1, 4, 16)), 4.0)    // (1*4*16)^(1/3) = 4
	wantFloat(t, call(t, harmonicMeanFn, floatList(1, 2, 4)), 12.0/7.0) // 3/(1+0.5+0.25)
	for _, fn := range []interpreter.Builtin{geometricMeanFn, harmonicMeanFn} {
		if _, err := fn(interpreter.BuiltinCtx{}, []interpreter.Value{floatList(1, -2, 3)}); err == nil {
			t.Error("mean of a non-positive element should error")
		}
	}
}
