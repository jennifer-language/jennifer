// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package mllib

import (
	"math"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// noPanic fails the test instead of crashing the binary when f panics.
func noPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("panic: %v", p)
		}
	}()
	f()
}

// TestTreeSplitAdjacentAndHugeValues: the midpoint of two adjacent floats
// rounds to the lower one and (a+b)/2 of two huge values overflows; either
// left a child empty and majority(nil) panicked.
func TestTreeSplitAdjacentAndHugeValues(t *testing.T) {
	for _, tc := range [][2]float64{
		{1.0, math.Nextafter(1, 2)},
		{1e308, 1.7e308},
		{-1.7e308, -1e308},
		{5e-324, 1e-323},
	} {
		a, b := tc[0], tc[1]
		X := mat([]float64{a}, []float64{b})
		for _, fit := range []string{"decisionTree", "decisionTreeRegressor"} {
			noPanic(t, func() {
				r := newReg()
				fn := r.decisionTreeFn
				if fit == "decisionTreeRegressor" {
					fn = r.decisionTreeRegressorFn
				}
				m := ok(t, fn, X, vec(0, 1))
				got := fvals(ok(t, r.predictFn, m, X))
				if got[0] != 0 || got[1] != 1 {
					t.Errorf("%s on [%v, %v]: predict = %v, want [0 1]", fit, a, b, got)
				}
			})
		}
	}
	if thr := splitThreshold(1, math.Nextafter(1, 2)); !(thr > 1) {
		t.Errorf("splitThreshold(1, next) = %v, not above 1", thr)
	}
}

// TestTreeRegressorTargetOffset: a step 0 -> 1 is found whatever offset the
// targets carry (the running sum-of-squares cancelled at 1e9).
func TestTreeRegressorTargetOffset(t *testing.T) {
	for _, off := range []float64{0, 1e9, 1e12} {
		rows := make([][]float64, 40)
		ys := make([]float64, 40)
		for i := range rows {
			rows[i] = []float64{float64(i)}
			if i >= 20 {
				ys[i] = 1
			}
			ys[i] += off
		}
		r := newReg()
		m := ok(t, r.decisionTreeRegressorFn, mat(rows...), vec(ys...), iv(1))
		got := fvals(ok(t, r.predictFn, m, mat([]float64{0}, []float64{39})))
		if got[0] != off || got[1] != off+1 {
			t.Errorf("offset %g: predict = %v, want [%v %v]", off, got, off, off+1)
		}
	}
}

// TestRandomForestPerSplitFeatures: each split draws its own feature subset,
// so one deep tree over 4 features (mtry 2) uses more than 2 features. With a
// subset drawn once per tree at most 2 features ever get importance.
func TestRandomForestPerSplitFeatures(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	rows := make([][]float64, 300)
	ys := make([]float64, 300)
	for i := range rows {
		rows[i] = []float64{rng.Float64(), rng.Float64(), rng.Float64(), rng.Float64()}
		ys[i] = float64(rng.Intn(2))
	}
	r := newReg()
	m := ok(t, r.randomForestFn, mat(rows...), vec(ys...), iv(1), iv(12))
	used := 0
	for _, f := range fvals(ok(t, r.featureImportancesFn, m)) {
		if f > 0 {
			used++
		}
	}
	if used <= 2 {
		t.Errorf("one tree used %d of 4 features, want > 2 (per-split subsets)", used)
	}
}

// TestPCAScaleInvariantComponents: the Jacobi stop is relative, so data scaled
// by 1e-10 has the same components (the absolute 1e-30 bound left the tiny
// covariance unrotated: [[0, 1]]).
func TestPCAScaleInvariantComponents(t *testing.T) {
	base := [][]float64{{1, 2}, {2, 4.1}, {3, 5.9}, {4, 8.2}, {5, 9.7}}
	comp := func(scale float64) []float64 {
		rows := make([][]float64, len(base))
		for i, r := range base {
			rows[i] = []float64{r[0] * scale, r[1] * scale}
		}
		r := newReg()
		m := ok(t, r.pcaFn, mat(rows...), iv(1))
		c := fvals(ok(t, r.componentsFn, m).List[0])
		if c[0] < 0 {
			c[0], c[1] = -c[0], -c[1]
		}
		return c
	}
	want := comp(1)
	if !(want[0] > 0.4 && want[1] > 0.85) {
		t.Fatalf("unscaled component %v, want about [0.45 0.89]", want)
	}
	for _, s := range []float64{1e-10, 1e-150, 1e100} {
		got := comp(s)
		if !near(got[0], want[0], 1e-9) || !near(got[1], want[1], 1e-9) {
			t.Errorf("scale %g: component %v, want %v", s, got, want)
		}
	}
}

// TestLinearRegressionFloatCollinear: x2 = 3*x1 up to rounding is singular
// (the absolute 1e-300 pivot test fitted an arbitrary split), while a design
// with columns of very different units still fits.
func TestLinearRegressionFloatCollinear(t *testing.T) {
	r := newReg()
	X := mat([]float64{0.1, 0.3}, []float64{0.2, 0.6}, []float64{0.3, 0.9}, []float64{0.7, 2.1}, []float64{0.5, 1.5})
	y := vec(1, 2, 3, 7, 5.5)
	for _, fn := range []interpreter.Builtin{r.linearRegressionFn, func(c interpreter.BuiltinCtx, a []interpreter.Value) (interpreter.Value, error) {
		return r.ridgeFn(c, append(a, interpreter.FloatVal(0)))
	}} {
		_, err := fn(interpreter.BuiltinCtx{}, []interpreter.Value{X, y})
		if err == nil || !strings.Contains(err.Error(), "singular") {
			t.Errorf("collinear design: got %v, want a singular error", err)
		}
	}
	// y = 2e-8*x1 + 3e8*x2 + 1 with x1 ~ 1e8 and x2 ~ 1e-8.
	Xs := mat([]float64{1e8, 1e-8}, []float64{2e8, 1e-8}, []float64{1e8, 2e-8}, []float64{3e8, 2e-8}, []float64{2e8, 3e-8}, []float64{4e8, 1e-8})
	ys := vec(6, 8, 9, 13, 14, 12)
	m := ok(t, r.linearRegressionFn, Xs, ys)
	got := fvals(ok(t, r.predictFn, m, mat([]float64{5e8, 5e-8})))[0]
	if !near(got, 26, 1e-6) {
		t.Errorf("scaled design predict = %v, want 26", got)
	}
}

// TestPolynomialFeaturesCellCap: rows*cols past maxPolyCells is refused before
// building the output (the 2e7 cap allowed ~6 GB of Values).
func TestPolynomialFeaturesCellCap(t *testing.T) {
	const d, width = 13, 560 // degree 3 over 13 features, with the bias column
	rows := make([][]float64, limits.MaxRangeElements/2/width+1)
	for i := range rows {
		rows[i] = make([]float64, d)
		for j := range rows[i] {
			rows[i][j] = float64(i + j)
		}
	}
	X := mat(rows...)
	refuses(t, "output-cell limit", 64<<20, func() error {
		_, err := polynomialFeaturesFn(interpreter.BuiltinCtx{}, []interpreter.Value{X, iv(3)})
		return err
	})
}

// TestKNNNearestMatchesSort: the heap selection returns the same neighbours
// as sorting every distance (ties by training-row order).
func TestKNNNearestMatchesSort(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	n := 500
	x := make([][]float64, n)
	for i := range x {
		// Coarse grid values make distance ties common.
		x[i] = []float64{float64(rng.Intn(10)), float64(rng.Intn(10))}
	}
	for _, k := range []int{1, 3, 7, n} {
		m := &knnModel{x: x, y: make([]float64, n), k: k, nf: 2}
		for q := 0; q < 50; q++ {
			query := []float64{float64(rng.Intn(10)), float64(rng.Intn(10))}
			want := make([]neighbour, n)
			for i := range x {
				want[i] = neighbour{euclid2(query, x[i]), i}
			}
			sort.SliceStable(want, func(a, b int) bool { return want[a].d < want[b].d })
			got := m.nearest(query)
			if len(got) != k {
				t.Fatalf("k=%d: got %d neighbours", k, len(got))
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("k=%d query %v: neighbour %d = %v, want %v", k, query, i, got[i], want[i])
				}
			}
		}
	}
}

// BenchmarkKNNPredict: 5000 queries against 5000 training rows, k = 5.
func BenchmarkKNNPredict(b *testing.B) {
	rng := rand.New(rand.NewSource(3))
	x := make([][]float64, 5000)
	for i := range x {
		x[i] = []float64{rng.Float64(), rng.Float64(), rng.Float64(), rng.Float64()}
	}
	m := &knnModel{x: x, y: make([]float64, len(x)), k: 5, nf: 4}
	for i := 0; i < b.N; i++ {
		for _, q := range x {
			m.predictOne(q)
		}
	}
}
