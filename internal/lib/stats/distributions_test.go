// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package statslib

import (
	"math"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func sf(x float64) interpreter.Value { return interpreter.FloatVal(x) }
func si(n int64) interpreter.Value   { return interpreter.IntVal(n) }
func ss(s string) interpreter.Value  { return interpreter.Value{Kind: interpreter.KindString, Str: s} }

// callFail asserts the builtin rejects its arguments.
func callFail(t *testing.T, fn interpreter.Builtin, args ...interpreter.Value) {
	t.Helper()
	if _, err := fn(interpreter.BuiltinCtx{}, args); err == nil {
		t.Fatalf("expected an error, got none")
	}
}

// field reads a named field from a struct-kind Value.
func field(t *testing.T, v interpreter.Value, name string) interpreter.Value {
	t.Helper()
	if v.Kind != interpreter.KindStruct {
		t.Fatalf("expected struct, got %s", v.Kind)
	}
	for _, f := range v.Fields {
		if f.Name == name {
			return f.Value
		}
	}
	t.Fatalf("no field %q", name)
	return interpreter.Null()
}

func near(got, want, tol float64) bool { return math.Abs(got-want) <= tol }

// TestDistributionsReference pins the distribution surface to scipy reference
// values (pdf / pmf, cdf, quantile) across normal, t, chi-square, F, binomial,
// and Poisson.
func TestDistributionsReference(t *testing.T) {
	cases := []struct {
		name string
		fn   interpreter.Builtin
		args []interpreter.Value
		want float64
		tol  float64
	}{
		// Normal(0,1).
		{"normalPdf", normalPdfFn, []interpreter.Value{sf(0), sf(0), sf(1)}, 0.3989422804014327, 1e-12},
		{"normalCdf", normalCdfFn, []interpreter.Value{sf(1.96), sf(0), sf(1)}, 0.9750021048517795, 1e-12},
		{"normalCdf@0", normalCdfFn, []interpreter.Value{sf(0), sf(0), sf(1)}, 0.5, 1e-12},
		{"normalQuantile", normalQuantileFn, []interpreter.Value{sf(0.975), sf(0), sf(1)}, 1.959963984540054, 1e-9},
		// Student's t.
		{"tCdf", tCdfFn, []interpreter.Value{sf(2.0), sf(10)}, 0.9633059826146299, 1e-10},
		{"tCdf@0", tCdfFn, []interpreter.Value{sf(0), sf(5)}, 0.5, 1e-12},
		{"tQuantile", tQuantileFn, []interpreter.Value{sf(0.975), sf(10)}, 2.2281388519649385, 1e-8},
		{"tPdf", tPdfFn, []interpreter.Value{sf(0), sf(10)}, 0.3891083839660307, 1e-10},
		// Chi-square.
		{"chiSquareCdf", chiSquareCdfFn, []interpreter.Value{sf(3.841458820694124), sf(1)}, 0.95, 1e-9},
		{"chiSquareQuantile", chiSquareQuantileFn, []interpreter.Value{sf(0.95), sf(1)}, 3.841458820694124, 1e-7},
		{"chiSquareCdf@df4", chiSquareCdfFn, []interpreter.Value{sf(4.0), sf(4)}, 0.5939941502901616, 1e-10},
		// F.
		{"fCdf", fCdfFn, []interpreter.Value{sf(1.0), sf(10), sf(10)}, 0.5, 1e-10},
		{"fQuantile", fQuantileFn, []interpreter.Value{sf(0.95), sf(3), sf(10)}, 3.7082648734963563, 1e-6},
		// Binomial(10, 0.5).
		{"binomialPmf", binomialPmfFn, []interpreter.Value{si(5), si(10), sf(0.5)}, 0.24609375, 1e-12},
		{"binomialCdf", binomialCdfFn, []interpreter.Value{si(5), si(10), sf(0.5)}, 0.623046875, 1e-10},
		// Poisson(3).
		{"poissonPmf", poissonPmfFn, []interpreter.Value{si(2), sf(3.0)}, 0.22404180765538775, 1e-12},
		{"poissonCdf", poissonCdfFn, []interpreter.Value{si(2), sf(3.0)}, 0.42319008112684353, 1e-10},
	}
	for _, c := range cases {
		got := call(t, c.fn, c.args...)
		if got.Kind != interpreter.KindFloat || !near(got.Float, c.want, c.tol) {
			t.Errorf("%s = %v, want %v (tol %g)", c.name, got.Float, c.want, c.tol)
		}
	}
}

// TestQuantileRoundTrip verifies quantile(cdf(x)) == x across the continuous
// distributions (the inversion is self-consistent).
func TestQuantileRoundTrip(t *testing.T) {
	// chi-square df=7 at x=5.
	p := call(t, chiSquareCdfFn, sf(5.0), sf(7))
	x := call(t, chiSquareQuantileFn, sf(p.Float), sf(7))
	if !near(x.Float, 5.0, 1e-6) {
		t.Errorf("chiSquare round-trip: %v, want 5", x.Float)
	}
	// t df=15 at t=1.3.
	p = call(t, tCdfFn, sf(1.3), sf(15))
	x = call(t, tQuantileFn, sf(p.Float), sf(15))
	if !near(x.Float, 1.3, 1e-6) {
		t.Errorf("t round-trip: %v, want 1.3", x.Float)
	}
	// F (5,12) at x=2.7.
	p = call(t, fCdfFn, sf(2.7), sf(5), sf(12))
	x = call(t, fQuantileFn, sf(p.Float), sf(5), sf(12))
	if !near(x.Float, 2.7, 1e-6) {
		t.Errorf("F round-trip: %v, want 2.7", x.Float)
	}
}

// TestDistributionDomains checks the strict domain rejections.
func TestDistributionDomains(t *testing.T) {
	callFail(t, normalPdfFn, sf(0), sf(0), sf(0))      // sd = 0
	callFail(t, normalCdfFn, sf(0), sf(0), sf(-1))     // sd < 0
	callFail(t, normalQuantileFn, sf(0), sf(0), sf(1)) // p = 0
	callFail(t, normalQuantileFn, sf(1), sf(0), sf(1)) // p = 1
	callFail(t, tCdfFn, sf(0), sf(0))                  // df = 0
	callFail(t, chiSquareCdfFn, sf(-1), sf(3))         // x < 0
	callFail(t, fQuantileFn, sf(1.5), sf(3), sf(3))    // p > 1
	callFail(t, binomialPmfFn, si(2), si(3), sf(1.5))  // p out of [0,1]
	callFail(t, poissonPmfFn, si(2), sf(0))            // lambda <= 0
	// Boundary values that must NOT error: k out of range -> 0.
	if v := call(t, binomialPmfFn, si(11), si(10), sf(0.5)); v.Float != 0 {
		t.Errorf("binomialPmf(11,10,.5) = %v, want 0", v.Float)
	}
}

// TestRegression checks OLS on an exact line and multiple regression on an
// exactly-consistent system.
func TestRegression(t *testing.T) {
	// y = 2x + 1 exactly.
	rg := call(t, linearRegressionFn, floatList(1, 2, 3, 4, 5), floatList(3, 5, 7, 9, 11))
	if !near(field(t, rg, "slope").Float, 2, 1e-9) || !near(field(t, rg, "intercept").Float, 1, 1e-9) {
		t.Errorf("linearRegression slope/intercept = %v/%v, want 2/1", field(t, rg, "slope").Float, field(t, rg, "intercept").Float)
	}
	if !near(field(t, rg, "r2").Float, 1, 1e-12) {
		t.Errorf("r2 = %v, want 1", field(t, rg, "r2").Float)
	}
	// Multiple regression, y = 1 + 2*x1 + 3*x2 exactly.
	X := interpreter.Value{Kind: interpreter.KindList, List: []interpreter.Value{
		floatList(1, 1), floatList(2, 1), floatList(1, 2), floatList(3, 2), floatList(2, 3),
	}}
	// y: (1,1)->6 (2,1)->8 (1,2)->9 (3,2)->13 (2,3)->14
	beta := call(t, multipleRegressionFn, X, floatList(6, 8, 9, 13, 14))
	want := []float64{1, 2, 3}
	for i, w := range want {
		if !near(beta.List[i].Float, w, 1e-6) {
			t.Errorf("multipleRegression coeff %d = %v, want %v", i, beta.List[i].Float, w)
		}
	}
	// Singular design (collinear predictors) errors.
	Xs := interpreter.Value{Kind: interpreter.KindList, List: []interpreter.Value{
		floatList(1, 2), floatList(2, 4), floatList(3, 6), floatList(4, 8),
	}}
	callFail(t, multipleRegressionFn, Xs, floatList(1, 2, 3, 4))
	// Zero-variance regressor errors.
	callFail(t, linearRegressionFn, floatList(2, 2, 2, 2), floatList(1, 2, 3, 4))
}

// TestInferenceReference pins the inference results to hand / scipy values.
func TestInferenceReference(t *testing.T) {
	data := floatList(2, 4, 4, 4, 5, 5, 7, 9) // mean 5, sample sd 2
	// 95% CI for the mean: 5 +/- 2.365*2/sqrt(8) = [3.2125, 6.7875].
	ci := call(t, confidenceIntervalFn, data, sf(0.95))
	if !near(field(t, ci, "lower").Float, 3.2125120817637924, 1e-7) || !near(field(t, ci, "upper").Float, 6.787487918236208, 1e-7) {
		t.Errorf("CI95 = [%v, %v]", field(t, ci, "lower").Float, field(t, ci, "upper").Float)
	}
	// One-sample t-test vs mu=3: t=2.6458, p=0.03315.
	tt := call(t, tTestFn, data, sf(3.0))
	if !near(field(t, tt, "statistic").Float, 2.6457513110645907, 1e-9) || !near(field(t, tt, "pValue").Float, 0.0331455, 1e-6) {
		t.Errorf("tTest t=%v p=%v", field(t, tt, "statistic").Float, field(t, tt, "pValue").Float)
	}
	// chi-square goodness of fit: stat=20, df=3, p=0.00016974.
	cs := call(t, chiSquareTestFn, floatList(10, 20, 30, 40), floatList(25, 25, 25, 25))
	if !near(field(t, cs, "statistic").Float, 20, 1e-9) || !near(field(t, cs, "pValue").Float, 0.00016974243555, 1e-9) {
		t.Errorf("chiSquareTest stat=%v p=%v", field(t, cs, "statistic").Float, field(t, cs, "pValue").Float)
	}
	// One-way ANOVA: F=13, p=0.0065918.
	av := call(t, anovaFn, interpreter.Value{Kind: interpreter.KindList, List: []interpreter.Value{
		floatList(1, 2, 3), floatList(2, 3, 4), floatList(5, 6, 7),
	}})
	if !near(field(t, av, "statistic").Float, 13, 1e-9) || !near(field(t, av, "pValue").Float, 0.0065918, 1e-6) {
		t.Errorf("anova F=%v p=%v", field(t, av, "statistic").Float, field(t, av, "pValue").Float)
	}
}

// TestInferenceStrictness pins the DF-stats2 audit fixes: invalid or
// overflowing inputs to the tests are catchable errors, never a plausible-
// looking p-value or an Inf statistic.
func TestInferenceStrictness(t *testing.T) {
	// A negative observed count is invalid data - rejected (F-stats2-2).
	callFail(t, chiSquareTestFn, floatList(-5, 15), floatList(10, 10))
	// A zero observed count is valid (a legitimate empty category).
	call(t, chiSquareTestFn, floatList(0, 20), floatList(10, 10))
	// Magnitude overflow lands on the precise "overflow" error, and no test
	// embeds a non-finite statistic (F-stats2-1).
	for _, tc := range []struct {
		fn   interpreter.Builtin
		args []interpreter.Value
	}{
		{chiSquareTestFn, []interpreter.Value{floatList(1e308, 1), floatList(1, 1)}},
		{tTestFn, []interpreter.Value{floatList(1, 2, 1e308), sf(0)}},
		{tTest2Fn, []interpreter.Value{floatList(1, 2, 1e308), floatList(1, 2, 3)}},
		{fTestFn, []interpreter.Value{floatList(1, 2, 1e308), floatList(1, 2, 3)}},
		{anovaFn, []interpreter.Value{interpreter.Value{Kind: interpreter.KindList, List: []interpreter.Value{
			floatList(1, 2, 1e308), floatList(1, 2, 3),
		}}}},
	} {
		callFail(t, tc.fn, tc.args...)
	}
}

// TestProportionCi checks the three proportion-interval methods for 8/10 at 95%,
// including the Clopper-Pearson exact bounds and the all-successes edge.
func TestProportionCi(t *testing.T) {
	wald := call(t, proportionCiFn, si(8), si(10), sf(0.95), ss("wald"))
	if !near(field(t, wald, "lower").Float, 0.5520819870781755, 1e-9) {
		t.Errorf("wald lower = %v", field(t, wald, "lower").Float)
	}
	wilson := call(t, proportionCiFn, si(8), si(10), sf(0.95), ss("wilson"))
	if !near(field(t, wilson, "lower").Float, 0.4901624715366419, 1e-9) || !near(field(t, wilson, "upper").Float, 0.9433178485456249, 1e-9) {
		t.Errorf("wilson = [%v, %v]", field(t, wilson, "lower").Float, field(t, wilson, "upper").Float)
	}
	cp := call(t, proportionCiFn, si(8), si(10), sf(0.95), ss("clopper-pearson"))
	if !near(field(t, cp, "lower").Float, 0.4439045, 1e-6) || !near(field(t, cp, "upper").Float, 0.9747893, 1e-6) {
		t.Errorf("clopper-pearson = [%v, %v]", field(t, cp, "lower").Float, field(t, cp, "upper").Float)
	}
	// All successes: Clopper-Pearson upper is exactly 1, lower > 0.
	edge := call(t, proportionCiFn, si(10), si(10), sf(0.95), ss("clopper-pearson"))
	if field(t, edge, "upper").Float != 1 || field(t, edge, "lower").Float <= 0 {
		t.Errorf("cp(10/10) = [%v, %v], want lower>0 upper=1", field(t, edge, "lower").Float, field(t, edge, "upper").Float)
	}
	// Unknown method errors.
	callFail(t, proportionCiFn, si(8), si(10), sf(0.95), ss("bayes"))
}

// TestHistogram checks bin counting (last bin closed on the right) and the
// ascending-edges guard.
func TestHistogram(t *testing.T) {
	h := call(t, histogramFn, floatList(1, 1.5, 2, 2.5, 3, 3.5, 4), floatList(1, 2, 3, 4))
	want := []int64{2, 2, 3} // [1,2)->2, [2,3)->2, [3,4]->3
	if len(h.List) != 3 {
		t.Fatalf("histogram len %d, want 3", len(h.List))
	}
	for i, w := range want {
		if h.List[i].Int != w {
			t.Errorf("bin %d = %d, want %d", i, h.List[i].Int, w)
		}
	}
	// Values outside [first, last] edge are not counted.
	h2 := call(t, histogramFn, floatList(-5, 0.5, 5, 100), floatList(1, 2, 3))
	if h2.List[0].Int != 0 || h2.List[1].Int != 0 {
		t.Errorf("out-of-range counting: %v", h2.List)
	}
	// Non-ascending edges error.
	callFail(t, histogramFn, floatList(1, 2, 3), floatList(3, 2, 1))
}
