// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package mllib

import (
	"fmt"
	"math"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// scalerModel is a fitted feature scaler: standardize (z-score) or min-max.
type scalerModel struct {
	center []float64 // mean, or min
	scale  []float64 // stddev, or (max - min); a zero scale is stored as 1
	nf     int
}

func (m *scalerModel) numFeatures() int { return m.nf }
func (m *scalerModel) transformOne(x []float64) []float64 {
	out := make([]float64, len(x))
	for j := range x {
		out[j] = (x[j] - m.center[j]) / m.scale[j]
	}
	return out
}

func (r *registry) standardScalerFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return r.fitScaler("standardScaler", args, true)
}
func (r *registry) minMaxScalerFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return r.fitScaler("minMaxScaler", args, false)
}

// fitScaler computes per-feature center/scale. standard: (mean, stddev);
// minmax: (min, max-min). A zero scale (a constant feature) is stored as 1 so
// the transform maps it to 0 rather than dividing by zero.
func (r *registry) fitScaler(name string, args []interpreter.Value, standard bool) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("ml.%s expects 1 argument (X), got %d", name, len(args))
	}
	x, err := matrix(name, args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	d := len(x[0])
	center := make([]float64, d)
	scale := make([]float64, d)
	for j := 0; j < d; j++ {
		col := make([]float64, len(x))
		for i := range x {
			col[i] = x[i][j]
		}
		if standard {
			center[j] = mean(col)
			scale[j] = math.Sqrt(popVar(col))
		} else {
			lo, hi := col[0], col[0]
			for _, v := range col {
				lo, hi = math.Min(lo, v), math.Max(hi, v)
			}
			center[j] = lo
			scale[j] = hi - lo
		}
		if scale[j] == 0 || !isFinite(scale[j]) {
			scale[j] = 1
		}
	}
	return r.store(&scalerModel{center: center, scale: scale, nf: d}), nil
}

// --- polynomial feature expansion ---

// cappedBinom returns C(n, k), capped: it stops and returns a value above
// maxPolyFeatures as soon as the running product exceeds it, so a huge count
// never overflows or allocates.
func cappedBinom(n, k int) int64 {
	if k < 0 || k > n {
		return 0
	}
	if k > n-k {
		k = n - k
	}
	res := int64(1)
	for i := 1; i <= k; i++ {
		res = res * int64(n-k+i) / int64(i)
		if res > maxPolyFeatures {
			return res
		}
	}
	return res
}

// polynomialFeaturesFn expands X to all monomials up to `degree` (combinations
// with replacement), with a leading bias column of 1 - the same shape as
// scikit's PolynomialFeatures. It is stateless (the expansion depends only on
// the column count and degree), so the same call applies to train and test.
func polynomialFeaturesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("ml.polynomialFeatures expects 2 arguments (X, degree), got %d", len(args))
	}
	x, err := matrix("polynomialFeatures", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindInt || args[1].Int < 1 || args[1].Int > maxPolyDegree {
		return interpreter.Null(), fmt.Errorf("ml.polynomialFeatures: degree must be in [1, %d]", maxPolyDegree)
	}
	degree := int(args[1].Int)
	d := len(x[0])
	// Output width = C(d+degree, degree) (all monomials incl. the bias). Reject
	// a tiny input that would explode into a huge matrix - both by column count
	// and by total cells (rows * columns), so many rows of a moderate width
	// cannot OOM under the column cap alone.
	width := cappedBinom(d+degree, degree)
	if width > maxPolyFeatures {
		return interpreter.Null(), fmt.Errorf("ml.polynomialFeatures: %d features at degree %d exceed the %d output-column limit", d, degree, maxPolyFeatures)
	}
	if int64(len(x))*width > maxPolyCells {
		return interpreter.Null(), fmt.Errorf("ml.polynomialFeatures: %d rows x %d columns exceed the %d output-cell limit", len(x), width, maxPolyCells)
	}
	// Enumerate non-decreasing feature-index multisets of each size 1..degree.
	var combos [][]int
	for deg := 1; deg <= degree; deg++ {
		var gen func(start int, cur []int)
		gen = func(start int, cur []int) {
			if len(cur) == deg {
				combos = append(combos, append([]int{}, cur...))
				return
			}
			for f := start; f < d; f++ {
				gen(f, append(cur, f))
			}
		}
		gen(0, nil)
	}
	out := make([][]float64, len(x))
	for i, row := range x {
		r := make([]float64, 1+len(combos))
		r[0] = 1 // bias
		for c, combo := range combos {
			p := 1.0
			for _, f := range combo {
				p *= row[f]
			}
			if !isFinite(p) {
				return interpreter.Null(), fmt.Errorf("ml.polynomialFeatures: a product overflows to a non-finite value (feature magnitudes too large for degree %d)", degree)
			}
			r[1+c] = p
		}
		out[i] = r
	}
	return floatMat(out), nil
}

// --- train/test split ---

func (r *registry) trainTestSplitFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("ml.trainTestSplit expects 3 arguments (X, y, testFraction), got %d", len(args))
	}
	x, err := matrix("trainTestSplit", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	y, err := vector("trainTestSplit", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(x) != len(y) {
		return interpreter.Null(), fmt.Errorf("ml.trainTestSplit: X has %d rows but y has %d labels", len(x), len(y))
	}
	frac, ok := args[2].AsFloat()
	if !ok || frac <= 0 || frac >= 1 {
		return interpreter.Null(), fmt.Errorf("ml.trainTestSplit: testFraction must be in (0, 1)")
	}
	n := len(x)
	perm := make([]int, n)
	for i := range perm {
		perm[i] = i
	}
	// Fisher-Yates shuffle from the shared random source (respects randSeed).
	for i := n - 1; i > 0; i-- {
		j := randIntN(i + 1)
		perm[i], perm[j] = perm[j], perm[i]
	}
	nTest := int(math.Round(frac * float64(n)))
	if nTest < 1 {
		nTest = 1
	}
	if nTest > n-1 {
		nTest = n - 1
	}
	testIdx, trainIdx := perm[:nTest], perm[nTest:]
	gather := func(idx []int) ([][]float64, []float64) {
		gx := make([][]float64, len(idx))
		gy := make([]float64, len(idx))
		for i, id := range idx {
			gx[i], gy[i] = x[id], y[id]
		}
		return gx, gy
	}
	trX, trY := gather(trainIdx)
	teX, teY := gather(testIdx)
	return interpreter.NamespacedStructVal(LibraryName, "Split", []interpreter.StructField{
		{Name: "trainX", Value: floatMat(trX)},
		{Name: "trainY", Value: floatVec(trY)},
		{Name: "testX", Value: floatMat(teX)},
		{Name: "testY", Value: floatVec(teY)},
	}), nil
}

// --- k-fold cross-validation index sets ---

func kFoldFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("ml.kFold expects 2 arguments (nSamples, k), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindInt || args[1].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("ml.kFold: nSamples and k must be ints")
	}
	n, k := int(args[0].Int), int(args[1].Int)
	if k < 2 || k > n {
		return interpreter.Null(), fmt.Errorf("ml.kFold: k must be in [2, nSamples] (got k=%d, n=%d)", k, n)
	}
	// kFold takes bare ints, so bound nSamples*k (the total index storage) to
	// stop two small arguments from allocating a huge result.
	if args[0].Int > maxFoldWork || args[0].Int*args[1].Int > maxFoldWork {
		return interpreter.Null(), fmt.Errorf("ml.kFold: nSamples*k = %d exceeds the %d limit (too many index sets)", args[0].Int*args[1].Int, maxFoldWork)
	}
	// Contiguous folds over 0..n-1 (shuffle the data yourself with
	// trainTestSplit-style logic if you need randomized folds).
	folds := make([]interpreter.Value, k)
	base, extra := n/k, n%k
	start := 0
	for f := 0; f < k; f++ {
		sz := base
		if f < extra {
			sz++
		}
		var test, train []int64
		for i := 0; i < n; i++ {
			if i >= start && i < start+sz {
				test = append(test, int64(i))
			} else {
				train = append(train, int64(i))
			}
		}
		folds[f] = interpreter.NamespacedStructVal(LibraryName, "Fold", []interpreter.StructField{
			{Name: "trainIdx", Value: intVec(train)},
			{Name: "testIdx", Value: intVec(test)},
		})
		start += sz
	}
	return interpreter.ListVal(parser.NamespacedStructType(LibraryName, "Fold"), folds), nil
}
