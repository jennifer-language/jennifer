// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package statslib

import (
	"math"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// floatMatrix builds a list of float rows.
func floatMatrix(rows ...[]float64) interpreter.Value {
	xs := make([]interpreter.Value, len(rows))
	for i, r := range rows {
		xs[i] = floatList(r...)
	}
	return interpreter.Value{Kind: interpreter.KindList, List: xs}
}

// TestCorrelationExtremeMagnitudes: sx*sy overflowed to +Inf (r = 0) for data
// near 1e150, and underflowed to "zero variance" near 1e-170.
func TestCorrelationExtremeMagnitudes(t *testing.T) {
	want := math.Sqrt(300.0 / 301) // r of [0, 2, 1] and [0, 2, 1.1]
	for _, s := range []float64{1, 1e150, 1e-170, 1e300} {
		x, y := floatList(0, 2*s, s), floatList(0, 2*s, 1.1*s)
		v, err := correlationFn(interpreter.BuiltinCtx{}, []interpreter.Value{x, y})
		if err != nil || !near(v.Float, want, 1e-12) {
			t.Errorf("scale %g: correlation = %v, %v; want %v", s, v.Float, err, want)
		}
	}
	reg, err := linearRegressionFn(interpreter.BuiltinCtx{}, []interpreter.Value{floatList(0, 2e150, 1e150), floatList(0, 2e150, 1.1e150)})
	if err != nil {
		t.Fatal(err)
	}
	if r := field(t, reg, "r").Float; !near(r, want, 1e-12) {
		t.Errorf("linearRegression r = %v, want %v", r, want)
	}
}

// TestMultipleRegressionFloatCollinear: x2 = 3*x1 up to rounding is singular;
// the absolute 1e-300 pivot test returned an arbitrary split of the slope.
func TestMultipleRegressionFloatCollinear(t *testing.T) {
	X := floatMatrix([]float64{0.1, 0.3}, []float64{0.2, 0.6}, []float64{0.3, 0.9}, []float64{0.7, 2.1}, []float64{0.5, 1.5})
	_, err := multipleRegressionFn(interpreter.BuiltinCtx{}, []interpreter.Value{X, floatList(1, 2, 3, 7, 5.5)})
	if err == nil || !strings.Contains(err.Error(), "singular") {
		t.Errorf("got %v, want a singular-design error", err)
	}
	// Columns in very different units still solve: y = 1 + 2e-8*x1 + 3e8*x2.
	X = floatMatrix([]float64{1e8, 1e-8}, []float64{2e8, 1e-8}, []float64{1e8, 2e-8}, []float64{3e8, 2e-8}, []float64{2e8, 3e-8}, []float64{4e8, 1e-8})
	if _, err := multipleRegressionFn(interpreter.BuiltinCtx{}, []interpreter.Value{X, floatList(6, 8, 9, 13, 14, 12)}); err != nil {
		t.Errorf("scaled design: %v", err)
	}
}

// relErr is |got - want| / |want|.
func relErr(got, want float64) float64 { return math.Abs(got-want) / math.Abs(want) }

// TestLargeParameterDensities pins tPdf, binomialPmf and poissonPmf against
// mpmath (50 digits). The lgamma-difference forms cancelled for a large
// parameter: tPdf(0, 1e15) was 8.66 (a density above 1), poissonPmf(1e15,
// 1e15) 8.9 times too large, binomialPmf(5e11, 1e12, 0.5) off by 1.7e-3.
func TestLargeParameterDensities(t *testing.T) {
	ctx := interpreter.BuiltinCtx{}
	for _, c := range [][3]float64{ // t, df, density
		{0, 1e15, 0.3989422804014326},
		{1.5, 1e12, 0.12951759566587756},
		{30, 1e8, 1.4766266253398494e-196},
		{4, 1e5, 0.00013390484863400854},
		{1000, 10, 1.230401076418158e-28},
		{0.3, 0.5, 0.23819425165813704},
		{1.5, 1, 0.09794150344116637},
	} {
		v, err := tPdfFn(ctx, []interpreter.Value{sf(c[0]), sf(c[1])})
		if err != nil || relErr(v.Float, c[2]) > 1e-13 {
			t.Errorf("tPdf(%v, %v) = %v, %v; want %v", c[0], c[1], v.Float, err, c[2])
		}
	}
	for _, c := range []struct {
		k, n    int64
		p, want float64
	}{
		{500_000_000_000, 1_000_000_000_000, 0.5, 7.978845608026659e-07},
		{300_000_000, 1_000_000_000, 0.3, 2.7529632778422573e-05},
		{10, 1_000_000_000, 1e-8, 0.1251100363466835},
		{0, 1_000_000_000_000, 1e-13, 0.9048374180359551},
		{3, 10, 0.3, 0.266827932},
		{2, 5, 0.999999, 9.999980000872668e-18},
	} {
		v, err := binomialPmfFn(ctx, []interpreter.Value{si(c.k), si(c.n), sf(c.p)})
		if err != nil || relErr(v.Float, c.want) > 1e-13 {
			t.Errorf("binomialPmf(%d, %d, %v) = %v, %v; want %v", c.k, c.n, c.p, v.Float, err, c.want)
		}
	}
	for _, c := range []struct {
		k            int64
		lambda, want float64
	}{
		{1_000_000_000_000, 1e12, 3.9894228040139945e-07},
		{1_000_000_000_000_000, 1e15, 1.2615662610100799e-08},
		{1_000_000_000_000, 1.000001e12, 2.4197080517598433e-07},
		{1_000_000, 1e6, 0.00039894224715624404},
		{20, 2.5, 3.0685934366680707e-12},
		{5, 1e-3, 8.325004165278126e-18},
	} {
		v, err := poissonPmfFn(ctx, []interpreter.Value{si(c.k), sf(c.lambda)})
		if err != nil || relErr(v.Float, c.want) > 1e-13 {
			t.Errorf("poissonPmf(%d, %v) = %v, %v; want %v", c.k, c.lambda, v.Float, err, c.want)
		}
	}
}

// TestTQuantileFarTail pins tQuantile against mpmath. The lower tail was
// inverted as 1 - cdf(-t) against 1 - p, so every p below ~1e-16 collapsed:
// tQuantile(1e-20, 10) was -108.24.
func TestTQuantileFarTail(t *testing.T) {
	for _, c := range [][3]float64{ // p, df, quantile
		{1e-300, 1, -3.1830988618379066e+299},
		{1e-10, 3, -2225.7692846830932},
		{1e-20, 10, -256.43469931852619},
		{1e-20, 1000, -9.4670448152559242},
		{1e-100, 1e6, -21.275865985490619},
		{0.975, 10, 2.2281388519862742},
		{0.999999, 3, 103.29946777942897},
	} {
		v, err := tQuantileFn(interpreter.BuiltinCtx{}, []interpreter.Value{sf(c[0]), sf(c[1])})
		if err != nil || relErr(v.Float, c[2]) > 2e-13 {
			t.Errorf("tQuantile(%v, %v) = %v, %v; want %v", c[0], c[1], v.Float, err, c[2])
		}
	}
}
