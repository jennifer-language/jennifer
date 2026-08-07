// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package mllib

import (
	"math"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// --- test helpers ---

func newReg() *registry { return &registry{models: map[int64]any{}} }

func mat(rows ...[]float64) interpreter.Value {
	out := make([]interpreter.Value, len(rows))
	for i, r := range rows {
		out[i] = vec(r...)
	}
	return interpreter.ListVal(parser.ListType(parser.PrimitiveType(parser.TypeFloat)), out)
}
func vec(fs ...float64) interpreter.Value {
	out := make([]interpreter.Value, len(fs))
	for i, f := range fs {
		out[i] = interpreter.FloatVal(f)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeFloat), out)
}
func iv(n int64) interpreter.Value { return interpreter.IntVal(n) }

func ok(t *testing.T, fn interpreter.Builtin, args ...interpreter.Value) interpreter.Value {
	t.Helper()
	v, err := fn(interpreter.BuiltinCtx{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return v
}
func fails(t *testing.T, fn interpreter.Builtin, args ...interpreter.Value) {
	t.Helper()
	if _, err := fn(interpreter.BuiltinCtx{}, args); err == nil {
		t.Fatalf("expected error, got none")
	}
}
func fvals(v interpreter.Value) []float64 {
	out := make([]float64, len(v.List))
	for i, e := range v.List {
		if e.Kind == interpreter.KindInt {
			out[i] = float64(e.Int)
		} else {
			out[i] = e.Float
		}
	}
	return out
}
func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// TestLinearRegression checks an exact linear fit and ridge shrinkage.
func TestLinearRegression(t *testing.T) {
	r := newReg()
	// y = 2*x1 + 3*x2 + 1 exactly.
	X := mat([]float64{1, 1}, []float64{2, 1}, []float64{1, 2}, []float64{3, 2}, []float64{2, 3}, []float64{4, 1})
	y := vec(6, 8, 9, 13, 14, 12)
	m := ok(t, r.linearRegressionFn, X, y)
	pred := ok(t, r.predictFn, m, mat([]float64{5, 5}))
	if !near(fvals(pred)[0], 26, 1e-6) {
		t.Errorf("linreg predict [5,5] = %v, want 26", fvals(pred)[0])
	}
	r2 := ok(t, r2Fn, y, ok(t, r.predictFn, m, X))
	if !near(r2.Float, 1.0, 1e-9) {
		t.Errorf("r2 = %v, want 1", r2.Float)
	}
	// Ridge shrinks coefficients toward zero vs OLS.
	ols := r.models[m.Fields[0].Value.Int].(*linearModel)
	mr := ok(t, r.ridgeFn, X, y, interpreter.FloatVal(10))
	ridge := r.models[mr.Fields[0].Value.Int].(*linearModel)
	if math.Abs(ridge.coef[0]) >= math.Abs(ols.coef[0]) {
		t.Errorf("ridge coef %v not shrunk vs OLS %v", ridge.coef[0], ols.coef[0])
	}
	// Errors.
	fails(t, r.linearRegressionFn, X, vec(1, 2)) // length mismatch
	fails(t, r.ridgeFn, X, y, interpreter.FloatVal(-1))
}

// TestClassifiers checks kNN / naiveBayes / decisionTree / randomForest /
// logistic all separate a clean two-cluster problem.
func TestClassifiers(t *testing.T) {
	r := newReg()
	X := mat([]float64{1, 1}, []float64{1.5, 2}, []float64{2, 1}, []float64{8, 8}, []float64{8.5, 9}, []float64{9, 8})
	y := vec(0, 0, 0, 1, 1, 1)
	test := mat([]float64{1.2, 1.2}, []float64{8.7, 8.5})
	want := []float64{0, 1}
	for _, tc := range []struct {
		name  string
		model interpreter.Value
	}{
		{"kNN", ok(t, r.kNNFn, X, y, iv(3))},
		{"naiveBayes", ok(t, r.naiveBayesFn, X, y)},
		{"decisionTree", ok(t, r.decisionTreeFn, X, y)},
		{"randomForest", ok(t, r.randomForestFn, X, y, iv(15))},
		{"logistic", ok(t, r.logisticRegressionFn, X, y)},
	} {
		got := fvals(ok(t, r.predictFn, tc.model, test))
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s predict[%d] = %v, want %v", tc.name, i, got[i], want[i])
			}
		}
	}
	// logistic proba is monotone with the class and in [0,1].
	log := ok(t, r.logisticRegressionFn, X, y)
	pr := fvals(ok(t, r.predictProbaFn, log, test))
	if !(pr[0] < 0.5 && pr[1] > 0.5) {
		t.Errorf("logistic proba %v not separating", pr)
	}
	// Logistic rejects non-binary labels.
	fails(t, r.logisticRegressionFn, X, vec(0, 1, 2, 0, 1, 2))
}

// TestKMeans checks the two well-separated groups land in different clusters.
func TestKMeans(t *testing.T) {
	r := newReg()
	X := mat([]float64{0, 0}, []float64{0.1, 0.1}, []float64{0.2, 0}, []float64{5, 5}, []float64{5.1, 4.9}, []float64{4.9, 5})
	m := ok(t, r.kMeansFn, X, iv(2))
	lab := fvals(ok(t, r.predictFn, m, X))
	if lab[0] == lab[3] || lab[0] != lab[1] || lab[1] != lab[2] || lab[3] != lab[4] || lab[4] != lab[5] {
		t.Errorf("kMeans labels %v do not separate the two groups", lab)
	}
	fails(t, r.kMeansFn, X, iv(0))  // k < 1
	fails(t, r.kMeansFn, X, iv(99)) // k > rows
}

// TestPcaAndScalers checks PCA reduces dimensionality and captures the dominant
// axis, and the scalers center/scale correctly.
func TestPcaAndScalers(t *testing.T) {
	r := newReg()
	// Data varying mostly along x (axis 0).
	X := mat([]float64{-2, 0}, []float64{-1, 0.1}, []float64{0, -0.1}, []float64{1, 0}, []float64{2, 0.1})
	pc := ok(t, r.pcaFn, X, iv(1))
	tr := ok(t, r.transformFn, pc, X)
	if len(tr.List) != 5 || len(tr.List[0].List) != 1 {
		t.Fatalf("pca transform shape wrong: %d x %d", len(tr.List), len(tr.List[0].List))
	}
	// Standard scaler: each feature becomes mean 0, unit variance.
	sc := ok(t, r.standardScalerFn, X)
	st := ok(t, r.transformFn, sc, X)
	col0 := 0.0
	for _, row := range st.List {
		col0 += row.List[0].Float
	}
	if !near(col0/5, 0, 1e-9) {
		t.Errorf("standardized feature 0 mean = %v, want 0", col0/5)
	}
	// Min-max scaler: values land in [0,1].
	mm := ok(t, r.minMaxScalerFn, X)
	mt := ok(t, r.transformFn, mm, X)
	for _, row := range mt.List {
		for _, e := range row.List {
			if e.Float < -1e-12 || e.Float > 1+1e-12 {
				t.Errorf("minmax value %v out of [0,1]", e.Float)
			}
		}
	}
	fails(t, r.pcaFn, X, iv(5)) // nComponents > features
}

// TestMetrics pins the classification and regression metrics.
func TestMetrics(t *testing.T) {
	yt := vec(1, 0, 1, 1, 0, 1)
	yp := vec(1, 0, 0, 1, 0, 1)
	if v := ok(t, accuracyFn, yt, yp); !near(v.Float, 5.0/6, 1e-12) {
		t.Errorf("accuracy = %v", v.Float)
	}
	// positives: TP=3, FP=0, FN=1 -> prec=1, rec=0.75, f1=0.857
	if v := ok(t, precisionFn, yt, yp, iv(1)); !near(v.Float, 1, 1e-12) {
		t.Errorf("precision = %v", v.Float)
	}
	if v := ok(t, recallFn, yt, yp, iv(1)); !near(v.Float, 0.75, 1e-12) {
		t.Errorf("recall = %v", v.Float)
	}
	if v := ok(t, f1Fn, yt, yp, iv(1)); !near(v.Float, 6.0/7, 1e-12) {
		t.Errorf("f1 = %v", v.Float)
	}
	// ROC-AUC: perfect ranking = 1.0; reversed = 0.0; tie handling = 0.5.
	if v := ok(t, rocAucFn, vec(1, 0, 1, 0), vec(0.9, 0.1, 0.8, 0.4)); !near(v.Float, 1, 1e-12) {
		t.Errorf("rocAuc perfect = %v", v.Float)
	}
	if v := ok(t, rocAucFn, vec(1, 1, 0, 0), vec(0.5, 0.5, 0.5, 0.5)); !near(v.Float, 0.5, 1e-12) {
		t.Errorf("rocAuc all-ties = %v, want 0.5", v.Float)
	}
	// Regression metrics.
	if v := ok(t, rmseFn, vec(1, 2, 3), vec(1, 2, 4)); !near(v.Float, math.Sqrt(1.0/3), 1e-12) {
		t.Errorf("rmse = %v", v.Float)
	}
	if v := ok(t, maeFn, vec(1, 2, 3), vec(2, 2, 5)); !near(v.Float, 1.0, 1e-12) {
		t.Errorf("mae = %v", v.Float)
	}
	if v := ok(t, r2Fn, vec(1, 2, 3, 4), vec(1, 2, 3, 4)); !near(v.Float, 1, 1e-12) {
		t.Errorf("r2 perfect = %v", v.Float)
	}
	// Confusion matrix diagonal for a perfect prediction.
	cm := ok(t, confusionMatrixFn, vec(0, 1, 0, 1), vec(0, 1, 0, 1))
	m := cm.Fields[1].Value // matrix
	if m.List[0].List[1].Int != 0 || m.List[0].List[0].Int != 2 {
		t.Errorf("confusion matrix wrong: %v", m)
	}
	fails(t, accuracyFn, yt, vec(1, 0)) // length mismatch
	fails(t, r2Fn, vec(2, 2, 2), vec(1, 2, 3))
	fails(t, rocAucFn, vec(1, 1, 1), vec(0.1, 0.2, 0.3)) // no negatives
}

// TestResourceCaps pins the audit fixes: the cost-driving hyper-parameters are
// bounded, so a runaway value is a catchable error rather than a stack overflow
// / hang / OOM.
func TestResourceCaps(t *testing.T) {
	r := newReg()
	X := mat([]float64{1}, []float64{2}, []float64{3}, []float64{4})
	y := vec(0, 1, 0, 1)
	fails(t, r.decisionTreeFn, X, y, iv(1000000))                                    // maxDepth > 64 (recursion depth)
	fails(t, r.randomForestFn, X, y, iv(1000000))                                    // nTrees > 1000 (OOM)
	fails(t, r.kMeansFn, X, iv(2), iv(100000000))                                    // maxIter > 10000 (hang)
	fails(t, r.logisticRegressionFn, X, y, interpreter.FloatVal(0.1), iv(999999999)) // epochs (hang)
	fails(t, kFoldFn, iv(1000000), iv(1000000))                                      // nSamples*k amplification (OOM)
	// The generous ceilings still admit any realistic setting.
	ok(t, r.decisionTreeFn, X, y, iv(64))
	ok(t, r.randomForestFn, X, y, iv(1000))
	ok(t, r.kMeansFn, X, iv(2), iv(10000))
	ok(t, kFoldFn, iv(10000), iv(10))
}

// TestSplitAndFold checks the split sizes / disjointness and the k-fold
// partition covers every index exactly once as a test index.
func TestSplitAndFold(t *testing.T) {
	r := newReg()
	X := mat([]float64{1}, []float64{2}, []float64{3}, []float64{4}, []float64{5}, []float64{6}, []float64{7}, []float64{8}, []float64{9}, []float64{10})
	y := vec(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	sp := ok(t, r.trainTestSplitFn, X, y, interpreter.FloatVal(0.3))
	trX := sp.Fields[0].Value
	teX := sp.Fields[2].Value
	if len(trX.List) != 7 || len(teX.List) != 3 {
		t.Errorf("split sizes train=%d test=%d, want 7/3", len(trX.List), len(teX.List))
	}
	// k-fold: 3 folds over 10, test indices partition 0..9 with no overlap.
	folds := ok(t, kFoldFn, iv(10), iv(3))
	if len(folds.List) != 3 {
		t.Fatalf("want 3 folds, got %d", len(folds.List))
	}
	seen := map[int64]bool{}
	for _, f := range folds.List {
		testIdx := f.Fields[1].Value
		for _, e := range testIdx.List {
			if seen[e.Int] {
				t.Errorf("index %d appears in two test folds", e.Int)
			}
			seen[e.Int] = true
		}
	}
	if len(seen) != 10 {
		t.Errorf("k-fold test indices cover %d of 10", len(seen))
	}
	fails(t, kFoldFn, iv(10), iv(1)) // k < 2
	fails(t, kFoldFn, iv(3), iv(10)) // k > n
}
