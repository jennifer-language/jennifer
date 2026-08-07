// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package mllib implements Jennifer's `ml` library: classical / predictive
// machine learning on tabular data - the scikit-learn-lite core companion to
// `stats` / `linalg`. Models follow a fit / predict shape: a `fit` function
// (`ml.kMeans`, `ml.linearRegression`, ...) trains and returns an opaque
// `ml.Model` handle, and `ml.predict` / `ml.transform` apply it. A fitted model
// is immutable, so sharing a handle across value-copies and `spawn`ed tasks is
// safe (read-only). Data is a `list of list of float/int` (rows of features) and
// labels a `list of float/int`.
//
// Pure Go stdlib (native loops over modest tabular data; large datasets stay a
// native-tool job), TinyGo-clean, both binaries. Not a deep-learning framework -
// tensors / autodiff / deep-net training are out of scope by design.
package mllib

import (
	"fmt"
	"math"
	"sync"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	mathlib "jennifer-lang.dev/jennifer/internal/lib/math"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the Jennifer name programs `use` to enable these functions.
const LibraryName = "ml"

// Resource ceilings on the cost-driving hyper-parameters. They bound recursion
// depth, iteration counts, and allocation so a runaway parameter (accidental or
// hostile) is a catchable error, not a stack overflow / hang / OOM. All are far
// above any useful setting.
const (
	maxTreeDepth      = 64          // tree recursion depth (2^64 leaves is already absurd)
	maxForestTrees    = 1000        // trees in a random forest
	maxKMeansIter     = 10000       // Lloyd iterations (converges in << 100)
	maxLogisticEpochs = 1_000_000   // gradient-descent passes
	maxFoldWork       = 100_000_000 // kFold: cap nSamples*k index storage (bare ints -> huge output)
	maxPolyDegree     = 8           // polynomialFeatures degree
	maxPolyFeatures   = 100_000     // polynomialFeatures output width (small input * high degree -> huge output)
	maxPolyCells      = 20_000_000  // polynomialFeatures total output cells rows*cols (few cols but many rows -> OOM)
	maxClasses        = 100         // one-vs-rest logistic classes (guards continuous y mistaken for labels)
)

// predictor is a fitted model that maps a feature row to a scalar prediction.
// intLabels reports whether predictions are discrete class / cluster labels
// (returned as int) rather than a continuous value (returned as float).
type predictor interface {
	predictOne(x []float64) float64
	intLabels() bool
	numFeatures() int
}

// prober is an optionally-implemented interface for a classifier that can return
// a probability / score for the positive class (used by ROC-AUC).
type prober interface {
	probaOne(x []float64) float64
	numFeatures() int
}

// transformer is a fitted model that maps a feature row to a new feature row
// (scalers, PCA).
type transformer interface {
	transformOne(x []float64) []float64
	numFeatures() int
}

// registry holds the live fitted models keyed by integer handle, per interpreter
// (a fresh registry is closed over in Install, so nothing leaks across runs).
type registry struct {
	mu     sync.Mutex
	models map[int64]any
	nextID int64
}

// store registers a fitted model and returns its `ml.Model{id}` handle.
func (r *registry) store(m any) interpreter.Value {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.nextID
	r.nextID++
	r.models[id] = m
	return interpreter.NamespacedStructVal(LibraryName, "Model", []interpreter.StructField{
		{Name: "id", Value: interpreter.IntVal(id)},
	})
}

// resolve pulls the fitted model out of an `ml.Model` argument.
func (r *registry) resolve(fnName string, v interpreter.Value) (any, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != "Model" {
		return nil, fmt.Errorf("ml.%s: argument must be an ml.Model, got %s", fnName, v.Kind)
	}
	for _, f := range v.Fields {
		if f.Name == "id" && f.Value.Kind == interpreter.KindInt {
			r.mu.Lock()
			m, ok := r.models[f.Value.Int]
			r.mu.Unlock()
			if !ok {
				return nil, fmt.Errorf("ml.%s: unknown or freed ml.Model", fnName)
			}
			return m, nil
		}
	}
	return nil, fmt.Errorf("ml.%s: malformed ml.Model handle", fnName)
}

// Install registers the ml library with a fresh per-interpreter registry.
func Install(in *interpreter.Interpreter) {
	r := &registry{models: map[int64]any{}}
	in.RegisterNamespacedStruct(LibraryName, "Model", []parser.StructField{
		{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)},
	})

	// Fit functions - each trains and returns an ml.Model handle.
	in.RegisterNamespaced(LibraryName, "linearRegression", r.linearRegressionFn)
	in.RegisterNamespaced(LibraryName, "ridge", r.ridgeFn)
	in.RegisterNamespaced(LibraryName, "lasso", r.lassoFn)
	in.RegisterNamespaced(LibraryName, "kNN", r.kNNFn)
	in.RegisterNamespaced(LibraryName, "kNNRegressor", r.kNNRegressorFn)
	in.RegisterNamespaced(LibraryName, "naiveBayes", r.naiveBayesFn)
	in.RegisterNamespaced(LibraryName, "logisticRegression", r.logisticRegressionFn)
	in.RegisterNamespaced(LibraryName, "kMeans", r.kMeansFn)
	in.RegisterNamespaced(LibraryName, "pca", r.pcaFn)
	in.RegisterNamespaced(LibraryName, "decisionTree", r.decisionTreeFn)
	in.RegisterNamespaced(LibraryName, "decisionTreeRegressor", r.decisionTreeRegressorFn)
	in.RegisterNamespaced(LibraryName, "randomForest", r.randomForestFn)
	in.RegisterNamespaced(LibraryName, "randomForestRegressor", r.randomForestRegressorFn)
	in.RegisterNamespaced(LibraryName, "standardScaler", r.standardScalerFn)
	in.RegisterNamespaced(LibraryName, "minMaxScaler", r.minMaxScalerFn)

	// Apply a fitted model.
	in.RegisterNamespaced(LibraryName, "predict", r.predictFn)
	in.RegisterNamespaced(LibraryName, "transform", r.transformFn)
	in.RegisterNamespaced(LibraryName, "predictProba", r.predictProbaFn)
	in.RegisterNamespaced(LibraryName, "free", r.freeFn)

	// Introspection - read the learned parameters.
	in.RegisterNamespaced(LibraryName, "coefficients", r.coefficientsFn)
	in.RegisterNamespaced(LibraryName, "intercept", r.interceptFn)
	in.RegisterNamespaced(LibraryName, "centroids", r.centroidsFn)
	in.RegisterNamespaced(LibraryName, "components", r.componentsFn)
	in.RegisterNamespaced(LibraryName, "explainedVariance", r.explainedVarianceFn)
	in.RegisterNamespaced(LibraryName, "featureImportances", r.featureImportancesFn)

	// Model selection / preprocessing.
	in.RegisterNamespaced(LibraryName, "trainTestSplit", r.trainTestSplitFn)
	in.RegisterNamespaced(LibraryName, "kFold", kFoldFn)
	in.RegisterNamespaced(LibraryName, "polynomialFeatures", polynomialFeaturesFn)

	// Metrics (pure functions over label / value lists).
	in.RegisterNamespaced(LibraryName, "accuracy", accuracyFn)
	in.RegisterNamespaced(LibraryName, "precision", precisionFn)
	in.RegisterNamespaced(LibraryName, "recall", recallFn)
	in.RegisterNamespaced(LibraryName, "f1", f1Fn)
	in.RegisterNamespaced(LibraryName, "confusionMatrix", confusionMatrixFn)
	in.RegisterNamespaced(LibraryName, "rocAuc", rocAucFn)
	in.RegisterNamespaced(LibraryName, "rmse", rmseFn)
	in.RegisterNamespaced(LibraryName, "mse", mseFn)
	in.RegisterNamespaced(LibraryName, "mae", maeFn)
	in.RegisterNamespaced(LibraryName, "r2", r2Fn)
	in.RegisterNamespaced(LibraryName, "logLoss", logLossFn)

	registerStructs(in)
}

// registerStructs registers the plain-data result structs (split, confusion).
func registerStructs(in *interpreter.Interpreter) {
	fl := parser.PrimitiveType(parser.TypeFloat)
	matF := parser.ListType(parser.ListType(fl))
	vecF := parser.ListType(fl)
	in.RegisterNamespacedStruct(LibraryName, "Split", []parser.StructField{
		{Name: "trainX", Type: matF},
		{Name: "trainY", Type: vecF},
		{Name: "testX", Type: matF},
		{Name: "testY", Type: vecF},
	})
	vecI := parser.ListType(parser.PrimitiveType(parser.TypeInt))
	in.RegisterNamespacedStruct(LibraryName, "Confusion", []parser.StructField{
		{Name: "labels", Type: vecI},
		{Name: "matrix", Type: parser.ListType(vecI)},
	})
	in.RegisterNamespacedStruct(LibraryName, "Fold", []parser.StructField{
		{Name: "trainIdx", Type: vecI},
		{Name: "testIdx", Type: vecI},
	})
}

// --- argument helpers ---

// matrix reads args[i] as a `list of list of int/float` -> [][]float64,
// requiring a non-empty, rectangular matrix.
func matrix(name string, args []interpreter.Value, i int) ([][]float64, error) {
	v := args[i]
	if v.Kind != interpreter.KindList {
		return nil, fmt.Errorf("ml.%s: argument %d must be a list of rows, got %s", name, i+1, v.Kind)
	}
	if len(v.List) == 0 {
		return nil, fmt.Errorf("ml.%s: feature matrix is empty", name)
	}
	rows := make([][]float64, len(v.List))
	cols := -1
	for r, row := range v.List {
		if row.Kind != interpreter.KindList {
			return nil, fmt.Errorf("ml.%s: row %d must be a list, got %s", name, r, row.Kind)
		}
		if cols == -1 {
			cols = len(row.List)
			if cols == 0 {
				return nil, fmt.Errorf("ml.%s: rows must have at least one feature", name)
			}
		} else if len(row.List) != cols {
			return nil, fmt.Errorf("ml.%s: row %d has %d features, expected %d", name, r, len(row.List), cols)
		}
		fr := make([]float64, cols)
		for c, e := range row.List {
			f, ok := e.AsFloat()
			if !ok {
				return nil, fmt.Errorf("ml.%s: row %d feature %d must be int or float, got %s", name, r, c, e.Kind)
			}
			fr[c] = f
		}
		rows[r] = fr
	}
	return rows, nil
}

// vector reads args[i] as a `list of int/float` -> []float64.
func vector(name string, args []interpreter.Value, i int) ([]float64, error) {
	v := args[i]
	if v.Kind != interpreter.KindList {
		return nil, fmt.Errorf("ml.%s: argument %d must be a list, got %s", name, i+1, v.Kind)
	}
	out := make([]float64, len(v.List))
	for j, e := range v.List {
		f, ok := e.AsFloat()
		if !ok {
			return nil, fmt.Errorf("ml.%s: element %d must be int or float, got %s", name, j, e.Kind)
		}
		out[j] = f
	}
	return out, nil
}

// fitData reads (X, y) with matching row counts.
func fitData(name string, args []interpreter.Value) ([][]float64, []float64, error) {
	if len(args) < 2 {
		return nil, nil, fmt.Errorf("ml.%s expects at least 2 arguments (X, y)", name)
	}
	x, err := matrix(name, args, 0)
	if err != nil {
		return nil, nil, err
	}
	y, err := vector(name, args, 1)
	if err != nil {
		return nil, nil, err
	}
	if len(x) != len(y) {
		return nil, nil, fmt.Errorf("ml.%s: X has %d rows but y has %d labels", name, len(x), len(y))
	}
	return x, y, nil
}

// --- Value builders ---

func floatVec(fs []float64) interpreter.Value {
	out := make([]interpreter.Value, len(fs))
	for i, f := range fs {
		out[i] = interpreter.FloatVal(f)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeFloat), out)
}

func intVec(is []int64) interpreter.Value {
	out := make([]interpreter.Value, len(is))
	for i, n := range is {
		out[i] = interpreter.IntVal(n)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeInt), out)
}

func floatMat(m [][]float64) interpreter.Value {
	out := make([]interpreter.Value, len(m))
	for i, row := range m {
		out[i] = floatVec(row)
	}
	return interpreter.ListVal(parser.ListType(parser.PrimitiveType(parser.TypeFloat)), out)
}

func intMat(m [][]int64) interpreter.Value {
	out := make([]interpreter.Value, len(m))
	for i, row := range m {
		out[i] = intVec(row)
	}
	return interpreter.ListVal(parser.ListType(parser.PrimitiveType(parser.TypeInt)), out)
}

// --- predict / transform dispatch ---

func (r *registry) predictFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("ml.predict expects 2 arguments (model, X), got %d", len(args))
	}
	m, err := r.resolve("predict", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	p, ok := m.(predictor)
	if !ok {
		return interpreter.Null(), fmt.Errorf("ml.predict: this model does not predict labels/values (try ml.transform)")
	}
	x, err := matrix("predict", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(x[0]) != p.numFeatures() {
		return interpreter.Null(), fmt.Errorf("ml.predict: X has %d features, the model was fit on %d", len(x[0]), p.numFeatures())
	}
	if p.intLabels() {
		out := make([]int64, len(x))
		for i, row := range x {
			v := p.predictOne(row)
			if !isFinite(v) {
				return interpreter.Null(), fmt.Errorf("ml.predict: prediction is undefined or infinite")
			}
			out[i] = int64(math.Round(v))
		}
		return intVec(out), nil
	}
	out := make([]float64, len(x))
	for i, row := range x {
		v := p.predictOne(row)
		if !isFinite(v) {
			return interpreter.Null(), fmt.Errorf("ml.predict: prediction is undefined or infinite")
		}
		out[i] = v
	}
	return floatVec(out), nil
}

func (r *registry) transformFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("ml.transform expects 2 arguments (model, X), got %d", len(args))
	}
	m, err := r.resolve("transform", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	tr, ok := m.(transformer)
	if !ok {
		return interpreter.Null(), fmt.Errorf("ml.transform: this model does not transform features (try ml.predict)")
	}
	x, err := matrix("transform", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(x[0]) != tr.numFeatures() {
		return interpreter.Null(), fmt.Errorf("ml.transform: X has %d features, the model was fit on %d", len(x[0]), tr.numFeatures())
	}
	out := make([][]float64, len(x))
	for i, row := range x {
		res := tr.transformOne(row)
		for _, v := range res {
			if !isFinite(v) {
				return interpreter.Null(), fmt.Errorf("ml.transform: result is undefined or infinite")
			}
		}
		out[i] = res
	}
	return floatMat(out), nil
}

func (r *registry) predictProbaFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("ml.predictProba expects 2 arguments (model, X), got %d", len(args))
	}
	m, err := r.resolve("predictProba", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	pb, ok := m.(prober)
	if !ok {
		return interpreter.Null(), fmt.Errorf("ml.predictProba: this model does not produce probabilities")
	}
	x, err := matrix("predictProba", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(x[0]) != pb.numFeatures() {
		return interpreter.Null(), fmt.Errorf("ml.predictProba: X has %d features, the model was fit on %d", len(x[0]), pb.numFeatures())
	}
	out := make([]float64, len(x))
	for i, row := range x {
		v := pb.probaOne(row)
		if !isFinite(v) {
			return interpreter.Null(), fmt.Errorf("ml.predictProba: probability is undefined or infinite")
		}
		out[i] = v
	}
	return floatVec(out), nil
}

// freeFn drops a model from the registry, freeing its memory early.
func (r *registry) freeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("ml.free expects 1 argument (model), got %d", len(args))
	}
	if _, err := r.resolve("free", args[0]); err != nil {
		return interpreter.Null(), err
	}
	for _, f := range args[0].Fields {
		if f.Name == "id" && f.Value.Kind == interpreter.KindInt {
			r.mu.Lock()
			delete(r.models, f.Value.Int)
			r.mu.Unlock()
		}
	}
	return interpreter.Null(), nil
}

// isFinite reports whether r is neither NaN nor infinite.
func isFinite(r float64) bool { return !math.IsNaN(r) && !math.IsInf(r, 0) }

// euclid2 is the squared Euclidean distance between two equal-length vectors.
func euclid2(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		d := a[i] - b[i]
		s += d * d
	}
	return s
}

// randFloat / randIntN draw from `math`'s shared, `randSeed`-able source so ML
// runs are reproducible under `math.randSeed`.
func randFloat() float64 { return mathlib.SharedFloat64() }
func randIntN(n int) int { return int(mathlib.SharedIntN(int64(n))) }
