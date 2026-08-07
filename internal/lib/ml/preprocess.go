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
