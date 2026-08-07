// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Statistical inference for the `stats` library: regression, confidence
// intervals, and hypothesis tests built on the distributions in
// distributions.go. Pure-value and Go-stdlib-only (the multiple-regression
// solve is a self-contained Gauss-Jordan, no external linear-algebra
// dependency), so both binaries build it. Strict, like the rest of `stats`: a
// degenerate input (zero variance, a singular design, a non-positive expected
// count) is a catchable error, not a NaN.
package statslib

import (
	"fmt"
	"math"
	"sort"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// sampleVarOf is the Bessel-corrected (n-1) sample variance of a slice with
// at least two elements.
func sampleVarOf(fs []float64) float64 {
	m := meanOf(fs)
	s := 0.0
	for _, f := range fs {
		d := f - m
		s += d * d
	}
	return s / float64(len(fs)-1)
}

// matrixArg reads a `list of list of float/int` as a [][]float64, erroring on a
// non-list or a ragged / non-numeric row.
func matrixArg(name string, v interpreter.Value) ([][]float64, error) {
	if v.Kind != interpreter.KindList {
		return nil, fmt.Errorf("stats.%s: argument must be a list of rows, got %s", name, v.Kind)
	}
	rows := make([][]float64, len(v.List))
	for i, r := range v.List {
		if r.Kind != interpreter.KindList {
			return nil, fmt.Errorf("stats.%s: row %d must be a list, got %s", name, i, r.Kind)
		}
		row := make([]float64, len(r.List))
		for j, e := range r.List {
			f, ok := e.AsFloat()
			if !ok {
				return nil, fmt.Errorf("stats.%s: row %d element %d must be int or float, got %s", name, i, j, e.Kind)
			}
			row[j] = f
		}
		rows[i] = row
	}
	return rows, nil
}

// solveLinearSystem solves a*x = b (n by n) by Gauss-Jordan elimination with
// partial pivoting, returning false if the matrix is singular. Used for the
// multiple-regression normal equations - the same algorithm `linalg.solve`
// runs, kept local so `stats` needs no linear-algebra dependency.
func solveLinearSystem(a [][]float64, b []float64) ([]float64, bool) {
	n := len(b)
	m := make([][]float64, n)
	for i := range m {
		m[i] = append(append([]float64{}, a[i]...), b[i])
	}
	for col := 0; col < n; col++ {
		piv := col
		for r := col + 1; r < n; r++ {
			if math.Abs(m[r][col]) > math.Abs(m[piv][col]) {
				piv = r
			}
		}
		if math.Abs(m[piv][col]) < 1e-300 {
			return nil, false
		}
		m[col], m[piv] = m[piv], m[col]
		for r := 0; r < n; r++ {
			if r == col {
				continue
			}
			f := m[r][col] / m[col][col]
			for c := col; c <= n; c++ {
				m[r][c] -= f * m[col][c]
			}
		}
	}
	x := make([]float64, n)
	for i := 0; i < n; i++ {
		x[i] = m[i][n] / m[i][i]
	}
	return x, true
}

// twoSidedT is the two-sided p-value 2*(1 - F_t(|t|, df)) for a t statistic. It
// returns ok=false on a non-finite result, so a caller never embeds a NaN
// p-value.
func twoSidedT(t, df float64) (float64, bool) {
	v, ok := tCdfStd(math.Abs(t), df)
	if !ok {
		return 0, false
	}
	p := 2 * (1 - v)
	if !isFinite(p) {
		return 0, false
	}
	return p, true
}

// registerInferenceStructs registers the result structs the inference functions
// return. Called from Install.
func registerInferenceStructs(in *interpreter.Interpreter) {
	fl := parser.PrimitiveType(parser.TypeFloat)
	in.RegisterNamespacedStruct(LibraryName, "Regression", []parser.StructField{
		{Name: "n", Type: parser.PrimitiveType(parser.TypeInt)},
		{Name: "slope", Type: fl},
		{Name: "intercept", Type: fl},
		{Name: "r", Type: fl},
		{Name: "r2", Type: fl},
		{Name: "stdErr", Type: fl},
		{Name: "pValue", Type: fl},
	})
	in.RegisterNamespacedStruct(LibraryName, "Interval", []parser.StructField{
		{Name: "lower", Type: fl},
		{Name: "upper", Type: fl},
	})
	// A general hypothesis-test result: the statistic, its degrees of freedom
	// (df2 is 0 for a single-df test like the t or chi-square; both are set for
	// an F test / ANOVA), and the p-value.
	in.RegisterNamespacedStruct(LibraryName, "Test", []parser.StructField{
		{Name: "statistic", Type: fl},
		{Name: "df1", Type: fl},
		{Name: "df2", Type: fl},
		{Name: "pValue", Type: fl},
	})
}

func intervalVal(lower, upper float64) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "Interval", []interpreter.StructField{
		{Name: "lower", Value: interpreter.FloatVal(lower)},
		{Name: "upper", Value: interpreter.FloatVal(upper)},
	})
}

func testVal(stat, df1, df2, p float64) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "Test", []interpreter.StructField{
		{Name: "statistic", Value: interpreter.FloatVal(stat)},
		{Name: "df1", Value: interpreter.FloatVal(df1)},
		{Name: "df2", Value: interpreter.FloatVal(df2)},
		{Name: "pValue", Value: interpreter.FloatVal(p)},
	})
}

// --- Linear regression ---

func linearRegressionFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("stats.linearRegression expects 2 arguments (xs, ys), got %d", len(args))
	}
	_, xs, err := numbers("linearRegression", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	_, ys, err := numbers("linearRegression", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	n := len(xs)
	if n != len(ys) {
		return interpreter.Null(), fmt.Errorf("stats.linearRegression: xs and ys must have equal length (%d vs %d)", n, len(ys))
	}
	if n < 3 {
		return interpreter.Null(), fmt.Errorf("stats.linearRegression: need at least 3 points, got %d", n)
	}
	xbar, ybar := meanOf(xs), meanOf(ys)
	var sxx, sxy, syy float64
	for i := range xs {
		dx, dy := xs[i]-xbar, ys[i]-ybar
		sxx += dx * dx
		sxy += dx * dy
		syy += dy * dy
	}
	if !isFinite(sxx) || !isFinite(sxy) || !isFinite(syy) {
		return interpreter.Null(), fmt.Errorf("stats.linearRegression: input magnitudes overflow the computation")
	}
	if sxx == 0 {
		return interpreter.Null(), fmt.Errorf("stats.linearRegression: xs has zero variance (a vertical fit has no slope)")
	}
	if syy == 0 {
		return interpreter.Null(), fmt.Errorf("stats.linearRegression: ys has zero variance (no relationship to fit)")
	}
	slope := sxy / sxx
	intercept := ybar - slope*xbar
	r := sxy / math.Sqrt(sxx*syy)
	sse := syy - slope*sxy // residual sum of squares
	if sse < 0 {
		sse = 0 // guard tiny negative from cancellation
	}
	stdErr := math.Sqrt((sse / float64(n-2)) / sxx)
	tstat := slope / stdErr
	p, ok := twoSidedT(tstat, float64(n-2))
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.linearRegression: p-value did not converge")
	}
	fields := []interpreter.StructField{
		{Name: "n", Value: interpreter.IntVal(int64(n))},
		{Name: "slope", Value: interpreter.FloatVal(slope)},
		{Name: "intercept", Value: interpreter.FloatVal(intercept)},
		{Name: "r", Value: interpreter.FloatVal(r)},
		{Name: "r2", Value: interpreter.FloatVal(r * r)},
		{Name: "stdErr", Value: interpreter.FloatVal(stdErr)},
		{Name: "pValue", Value: interpreter.FloatVal(clamp01(p))},
	}
	for _, f := range fields {
		if f.Value.Kind == interpreter.KindFloat && !isFinite(f.Value.Float) {
			return interpreter.Null(), fmt.Errorf("stats.linearRegression: result is undefined or infinite")
		}
	}
	return interpreter.NamespacedStructVal(LibraryName, "Regression", fields), nil
}

// --- Multiple regression ---

func multipleRegressionFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("stats.multipleRegression expects 2 arguments (X, ys), got %d", len(args))
	}
	x, err := matrixArg("multipleRegression", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	_, ys, err := numbers("multipleRegression", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	nrow := len(x)
	if nrow != len(ys) {
		return interpreter.Null(), fmt.Errorf("stats.multipleRegression: X has %d rows but ys has %d", nrow, len(ys))
	}
	if nrow == 0 {
		return interpreter.Null(), fmt.Errorf("stats.multipleRegression: X is empty")
	}
	k := len(x[0]) // predictors
	for i, row := range x {
		if len(row) != k {
			return interpreter.Null(), fmt.Errorf("stats.multipleRegression: row %d has %d columns, expected %d", i, len(row), k)
		}
	}
	if nrow <= k+1 {
		return interpreter.Null(), fmt.Errorf("stats.multipleRegression: need more rows than coefficients (%d rows, %d predictors + intercept)", nrow, k)
	}
	// Design matrix with a leading intercept column of 1s -> normal equations
	// (D^T D) beta = D^T y, solved directly.
	p := k + 1
	dtd := make([][]float64, p)
	dty := make([]float64, p)
	for i := range dtd {
		dtd[i] = make([]float64, p)
	}
	design := func(i, j int) float64 {
		if j == 0 {
			return 1
		}
		return x[i][j-1]
	}
	for r := 0; r < nrow; r++ {
		for a := 0; a < p; a++ {
			da := design(r, a)
			dty[a] += da * ys[r]
			for b := 0; b < p; b++ {
				dtd[a][b] += da * design(r, b)
			}
		}
	}
	beta, ok := solveLinearSystem(dtd, dty)
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.multipleRegression: the design is singular (collinear predictors?)")
	}
	return floatListVal("multipleRegression", beta)
}

// --- Confidence interval for the mean ---

func confidenceIntervalFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("stats.confidenceInterval expects 2 arguments (data, level), got %d", len(args))
	}
	_, fs, err := numbers("confidenceInterval", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	level, ok := args[1].AsFloat()
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.confidenceInterval: level must be int or float, got %s", args[1].Kind)
	}
	if level <= 0 || level >= 1 {
		return interpreter.Null(), fmt.Errorf("stats.confidenceInterval: level must be in (0, 1), got %s", interpreter.DisplayFloat(level))
	}
	n := len(fs)
	if n < 2 {
		return interpreter.Null(), fmt.Errorf("stats.confidenceInterval: need at least 2 observations, got %d", n)
	}
	mean := meanOf(fs)
	se := math.Sqrt(sampleVarOf(fs) / float64(n))
	tcrit, ok := tQuantile((1+level)/2, float64(n-1))
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.confidenceInterval: critical value did not converge")
	}
	lower, upper := mean-tcrit*se, mean+tcrit*se
	if !isFinite(lower) || !isFinite(upper) {
		return interpreter.Null(), fmt.Errorf("stats.confidenceInterval: result is undefined or infinite")
	}
	return intervalVal(lower, upper), nil
}

// --- Proportion confidence interval ---

func proportionCiFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 4 {
		return interpreter.Null(), fmt.Errorf("stats.proportionCi expects 4 arguments (successes, n, level, method), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindInt || args[1].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("stats.proportionCi: successes and n must be ints")
	}
	x, n := args[0].Int, args[1].Int
	level, ok := args[2].AsFloat()
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.proportionCi: level must be int or float, got %s", args[2].Kind)
	}
	if args[3].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("stats.proportionCi: method must be a string")
	}
	method := args[3].Str
	if n <= 0 {
		return interpreter.Null(), fmt.Errorf("stats.proportionCi: n must be positive, got %d", n)
	}
	if x < 0 || x > n {
		return interpreter.Null(), fmt.Errorf("stats.proportionCi: successes must be in [0, n], got %d of %d", x, n)
	}
	if level <= 0 || level >= 1 {
		return interpreter.Null(), fmt.Errorf("stats.proportionCi: level must be in (0, 1), got %s", interpreter.DisplayFloat(level))
	}
	nf, xf := float64(n), float64(x)
	phat := xf / nf
	alpha := 1 - level
	z := stdNormalInv(1 - alpha/2)
	var lower, upper float64
	switch method {
	case "wald":
		half := z * math.Sqrt(phat*(1-phat)/nf)
		lower, upper = phat-half, phat+half
	case "wilson":
		z2 := z * z
		denom := 1 + z2/nf
		center := (phat + z2/(2*nf)) / denom
		half := (z * math.Sqrt(phat*(1-phat)/nf+z2/(4*nf*nf))) / denom
		lower, upper = center-half, center+half
	case "clopper-pearson":
		if x == 0 {
			lower = 0
		} else {
			lower, ok = betaQuantile(alpha/2, xf, nf-xf+1)
			if !ok {
				return interpreter.Null(), fmt.Errorf("stats.proportionCi: Clopper-Pearson lower bound did not converge")
			}
		}
		if x == n {
			upper = 1
		} else {
			upper, ok = betaQuantile(1-alpha/2, xf+1, nf-xf)
			if !ok {
				return interpreter.Null(), fmt.Errorf("stats.proportionCi: Clopper-Pearson upper bound did not converge")
			}
		}
	default:
		return interpreter.Null(), fmt.Errorf("stats.proportionCi: unknown method %q (use \"wald\", \"wilson\", or \"clopper-pearson\")", method)
	}
	// Guard before clamping: clamp01(NaN) would be NaN, so a non-finite bound
	// must error, not silently clamp to a bogus 0/1.
	if !isFinite(lower) || !isFinite(upper) {
		return interpreter.Null(), fmt.Errorf("stats.proportionCi: result is undefined or infinite")
	}
	return intervalVal(clamp01(lower), clamp01(upper)), nil
}

// --- One-sample t-test ---

func tTestFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("stats.tTest expects 2 arguments (data, mu), got %d", len(args))
	}
	_, fs, err := numbers("tTest", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	mu, ok := args[1].AsFloat()
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.tTest: mu must be int or float, got %s", args[1].Kind)
	}
	n := len(fs)
	if n < 2 {
		return interpreter.Null(), fmt.Errorf("stats.tTest: need at least 2 observations, got %d", n)
	}
	sv := sampleVarOf(fs)
	if !isFinite(sv) {
		return interpreter.Null(), fmt.Errorf("stats.tTest: input magnitudes overflow the computation")
	}
	if sv == 0 {
		return interpreter.Null(), fmt.Errorf("stats.tTest: data has zero variance")
	}
	df := float64(n - 1)
	t := (meanOf(fs) - mu) / math.Sqrt(sv/float64(n))
	if !isFinite(t) {
		return interpreter.Null(), fmt.Errorf("stats.tTest: input magnitudes overflow the computation")
	}
	p, ok := twoSidedT(t, df)
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.tTest: p-value did not converge")
	}
	return testVal(t, df, 0, clamp01(p)), nil
}

// --- Two-sample (Welch) t-test ---

func tTest2Fn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("stats.tTest2 expects 2 arguments (a, b), got %d", len(args))
	}
	_, a, err := numbers("tTest2", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	_, b, err := numbers("tTest2", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	na, nb := len(a), len(b)
	if na < 2 || nb < 2 {
		return interpreter.Null(), fmt.Errorf("stats.tTest2: each sample needs at least 2 observations (%d, %d)", na, nb)
	}
	va, vb := sampleVarOf(a), sampleVarOf(b)
	if !isFinite(va) || !isFinite(vb) {
		return interpreter.Null(), fmt.Errorf("stats.tTest2: input magnitudes overflow the computation")
	}
	if va == 0 && vb == 0 {
		return interpreter.Null(), fmt.Errorf("stats.tTest2: both samples have zero variance")
	}
	sa, sb := va/float64(na), vb/float64(nb)
	se := math.Sqrt(sa + sb)
	t := (meanOf(a) - meanOf(b)) / se
	// Welch-Satterthwaite degrees of freedom.
	df := (sa + sb) * (sa + sb) / (sa*sa/float64(na-1) + sb*sb/float64(nb-1))
	if !isFinite(t) || !isFinite(df) {
		return interpreter.Null(), fmt.Errorf("stats.tTest2: input magnitudes overflow the computation")
	}
	p, ok := twoSidedT(t, df)
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.tTest2: p-value did not converge")
	}
	return testVal(t, df, 0, clamp01(p)), nil
}

// --- Chi-square goodness-of-fit test ---

func chiSquareTestFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("stats.chiSquareTest expects 2 arguments (observed, expected), got %d", len(args))
	}
	_, obs, err := numbers("chiSquareTest", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	_, exp, err := numbers("chiSquareTest", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(obs) != len(exp) {
		return interpreter.Null(), fmt.Errorf("stats.chiSquareTest: observed and expected must have equal length (%d vs %d)", len(obs), len(exp))
	}
	if len(obs) < 2 {
		return interpreter.Null(), fmt.Errorf("stats.chiSquareTest: need at least 2 categories")
	}
	var stat float64
	for i := range obs {
		if obs[i] < 0 {
			return interpreter.Null(), fmt.Errorf("stats.chiSquareTest: observed count %d must be non-negative, got %s", i, interpreter.DisplayFloat(obs[i]))
		}
		if exp[i] <= 0 {
			return interpreter.Null(), fmt.Errorf("stats.chiSquareTest: expected count %d must be positive, got %s", i, interpreter.DisplayFloat(exp[i]))
		}
		d := obs[i] - exp[i]
		stat += d * d / exp[i]
	}
	if !isFinite(stat) {
		return interpreter.Null(), fmt.Errorf("stats.chiSquareTest: input magnitudes overflow the computation")
	}
	df := float64(len(obs) - 1)
	cdf, ok := chiSquareCdfStd(stat, df)
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.chiSquareTest: p-value did not converge")
	}
	return testVal(stat, df, 0, clamp01(1-cdf)), nil
}

// --- F-test for equality of variances ---

func fTestFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("stats.fTest expects 2 arguments (a, b), got %d", len(args))
	}
	_, a, err := numbers("fTest", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	_, b, err := numbers("fTest", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(a) < 2 || len(b) < 2 {
		return interpreter.Null(), fmt.Errorf("stats.fTest: each sample needs at least 2 observations")
	}
	va, vb := sampleVarOf(a), sampleVarOf(b)
	if !isFinite(va) || !isFinite(vb) {
		return interpreter.Null(), fmt.Errorf("stats.fTest: input magnitudes overflow the computation")
	}
	if va == 0 || vb == 0 {
		return interpreter.Null(), fmt.Errorf("stats.fTest: a sample has zero variance")
	}
	f := va / vb
	if !isFinite(f) {
		return interpreter.Null(), fmt.Errorf("stats.fTest: input magnitudes overflow the computation")
	}
	df1, df2 := float64(len(a)-1), float64(len(b)-1)
	cdf, ok := fCdfStd(f, df1, df2)
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.fTest: p-value did not converge")
	}
	// Two-sided: twice the smaller tail.
	p := 2 * math.Min(cdf, 1-cdf)
	return testVal(f, df1, df2, clamp01(p)), nil
}

// --- One-way ANOVA ---

func anovaFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("stats.anova expects 1 argument (list of groups), got %d", len(args))
	}
	groups, err := matrixArg("anova", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	k := len(groups)
	if k < 2 {
		return interpreter.Null(), fmt.Errorf("stats.anova: need at least 2 groups, got %d", k)
	}
	var total int
	var grandSum float64
	for i, g := range groups {
		if len(g) < 1 {
			return interpreter.Null(), fmt.Errorf("stats.anova: group %d is empty", i)
		}
		total += len(g)
		for _, v := range g {
			grandSum += v
		}
	}
	if total <= k {
		return interpreter.Null(), fmt.Errorf("stats.anova: need more observations than groups (%d obs, %d groups)", total, k)
	}
	grand := grandSum / float64(total)
	var ssb, ssw float64
	for _, g := range groups {
		gm := meanOf(g)
		ssb += float64(len(g)) * (gm - grand) * (gm - grand)
		for _, v := range g {
			ssw += (v - gm) * (v - gm)
		}
	}
	df1, df2 := float64(k-1), float64(total-k)
	if !isFinite(ssb) || !isFinite(ssw) {
		return interpreter.Null(), fmt.Errorf("stats.anova: input magnitudes overflow the computation")
	}
	if ssw == 0 {
		return interpreter.Null(), fmt.Errorf("stats.anova: zero within-group variance (F is undefined)")
	}
	f := (ssb / df1) / (ssw / df2)
	if !isFinite(f) {
		return interpreter.Null(), fmt.Errorf("stats.anova: input magnitudes overflow the computation")
	}
	cdf, ok := fCdfStd(f, df1, df2)
	if !ok {
		return interpreter.Null(), fmt.Errorf("stats.anova: p-value did not converge")
	}
	return testVal(f, df1, df2, clamp01(1-cdf)), nil
}

// --- Histogram (Excel FREQUENCY) ---

func histogramFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("stats.histogram expects 2 arguments (data, binEdges), got %d", len(args))
	}
	_, data, err := numbers("histogram", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	_, edges, err := numbers("histogram", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(edges) < 2 {
		return interpreter.Null(), fmt.Errorf("stats.histogram: need at least 2 bin edges, got %d", len(edges))
	}
	for i := 1; i < len(edges); i++ {
		if edges[i] <= edges[i-1] {
			return interpreter.Null(), fmt.Errorf("stats.histogram: bin edges must be strictly ascending (edge %d)", i)
		}
	}
	nbins := len(edges) - 1
	counts := make([]int64, nbins)
	for _, v := range data {
		// A value outside [edges[0], edges[last]] is not counted; the last bin
		// is closed on the right so the maximum edge value lands in it. Binary
		// search the (validated, ascending) edges - the bin is the last edge
		// <= v - so counting stays O(len(data) * log(bins)), not O(data*bins).
		if v < edges[0] || v > edges[nbins] {
			continue
		}
		b := sort.Search(len(edges), func(i int) bool { return edges[i] > v }) - 1
		if b >= nbins {
			b = nbins - 1 // v == the final edge lands in the closed last bin
		}
		counts[b]++
	}
	out := make([]interpreter.Value, nbins)
	for i, c := range counts {
		out[i] = interpreter.IntVal(c)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeInt), out), nil
}
