// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package mllib implements Jennifer's `ml` library: classical / predictive
// machine learning on tabular data - the scikit-learn-lite core companion to
// `stats` / `linalg`. Models follow a fit / predict shape: a `fit` function
// (`ml.kMeans`, `ml.linearRegression`, ...) trains and returns an opaque
// `ml.Model` value, and `ml.predict` / `ml.transform` apply it. A fitted model
// is an immutable native value the garbage collector owns, so copies (in
// structs, lists, `spawn`ed tasks) share it safely. Data is a `list of list of float/int` (rows of features) and
// labels a `list of float/int`.
//
// Pure Go stdlib (native loops over modest tabular data; large datasets stay a
// native-tool job), TinyGo-clean, both binaries. Not a deep-learning framework -
// tensors / autodiff / deep-net training are out of scope by design.
package mllib

import (
	"fmt"
	"math"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	mathlib "jennifer-lang.dev/jennifer/internal/lib/math"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the Jennifer name programs `use` to enable these functions.
const LibraryName = "ml"

// Resource ceilings on the cost-driving hyper-parameters. They bound recursion
// depth, iteration counts, and allocation so a runaway parameter (accidental or
// hostile) is a catchable error, not a stack overflow / hang / OOM. All are far
// above any useful setting.
const (
	maxTreeDepth      = 64        // tree recursion depth (2^64 leaves is already absurd)
	maxForestTrees    = 1000      // trees in a random forest
	maxKMeansIter     = 10000     // Lloyd iterations (converges in << 100)
	maxLogisticEpochs = 1_000_000 // gradient-descent passes
	maxPolyDegree     = 8         // polynomialFeatures degree
	maxPolyFeatures   = 100_000   // polynomialFeatures output width (small input * high degree -> huge output)
	maxClasses        = 100       // one-vs-rest logistic classes (guards continuous y mistaken for labels)
)

// maxFoldWork caps kFold's nSamples*k index cells. Each is a ~272-byte Value,
// so half of limits.MaxRangeElements is ~570 MiB of result (~140 MiB on
// jennifer-tiny).
const maxFoldWork = limits.MaxRangeElements / 2

// maxPolyCells caps polynomialFeatures' rows*cols output cells (few columns but
// many rows -> OOM); each cell is a Value, as in maxFoldWork.
const maxPolyCells = limits.MaxRangeElements / 2

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

// modelValue is the native payload of an ml.Model: the fitted model itself.
// A fitted model never changes after its fit function returns, so every copy
// of the value shares the payload safely (across tasks too) and the garbage
// collector frees it once no copy is left.
type modelValue struct{ m any }

// EqualNative: a model equals itself and its copies. Two separate fits are
// different models even on the same data (a forest or k-means fit draws from
// the random source), so equality is identity of the fitted model.
func (v *modelValue) EqualNative(other interpreter.NativeObject) bool {
	o, ok := other.(*modelValue)
	return ok && o.m == v.m
}

func (v *modelValue) DisplayNative() string {
	if v.m == nil {
		return "<ml.Model unfitted>"
	}
	return "<ml.Model " + modelKind(v.m) + ">"
}

// modelKind names a fitted model's type for display and messages.
func modelKind(m any) string {
	switch v := m.(type) {
	case *linearModel:
		return "linear"
	case *knnModel:
		if v.regress {
			return "kNNRegressor"
		}
		return "kNN"
	case *naiveBayesModel:
		return "naiveBayes"
	case *logisticModel, *logisticMultiModel:
		return "logisticRegression"
	case *kMeansModel:
		return "kMeans"
	case *pcaModel:
		return "pca"
	case *treeModel:
		if v.regress {
			return "decisionTreeRegressor"
		}
		return "decisionTree"
	case *forestModel:
		if v.regress {
			return "randomForestRegressor"
		}
		return "randomForest"
	case *scalerModel:
		return "scaler"
	}
	return "model"
}

// storeModel wraps a fitted model as an `ml.Model` value.
func storeModel(m any) interpreter.Value {
	return interpreter.NativeObjectVal(LibraryName, "Model", &modelValue{m: m})
}

// resolveModel pulls the fitted model out of an `ml.Model` argument. The zero
// value (`def m as ml.Model;`) holds no model and is refused.
func resolveModel(fnName string, v interpreter.Value) (any, error) {
	nv, ok := v.AsNativeObject(LibraryName, "Model")
	if !ok {
		return nil, fmt.Errorf("ml.%s: argument must be an ml.Model, got %s", fnName, v.Kind)
	}
	m := nv.(*modelValue).m
	if m == nil {
		return nil, fmt.Errorf("ml.%s: the ml.Model is not fitted (a zero value); fit one with ml.linearRegression, ml.kMeans, ...", fnName)
	}
	return m, nil
}

// Install registers the ml library.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedObject(LibraryName, "Model", nil)
	in.SetNativeObjectZero(LibraryName, "Model", &modelValue{})

	// Fit functions - each trains and returns an ml.Model handle.
	in.RegisterNamespaced(LibraryName, "linearRegression", linearRegressionFn)
	in.RegisterNamespaced(LibraryName, "ridge", ridgeFn)
	in.RegisterNamespaced(LibraryName, "lasso", lassoFn)
	in.RegisterNamespaced(LibraryName, "kNN", kNNFn)
	in.RegisterNamespaced(LibraryName, "kNNRegressor", kNNRegressorFn)
	in.RegisterNamespaced(LibraryName, "naiveBayes", naiveBayesFn)
	in.RegisterNamespaced(LibraryName, "logisticRegression", logisticRegressionFn)
	in.RegisterNamespaced(LibraryName, "logisticRegressionWith", logisticRegressionWithFn)
	in.RegisterNamespaced(LibraryName, "kMeans", kMeansFn)
	in.RegisterNamespaced(LibraryName, "kMeansWith", kMeansWithFn)
	in.RegisterNamespaced(LibraryName, "pca", pcaFn)
	in.RegisterNamespaced(LibraryName, "decisionTree", decisionTreeFn)
	in.RegisterNamespaced(LibraryName, "decisionTreeRegressor", decisionTreeRegressorFn)
	in.RegisterNamespaced(LibraryName, "decisionTreeWith", decisionTreeWithFn)
	in.RegisterNamespaced(LibraryName, "decisionTreeRegressorWith", decisionTreeRegressorWithFn)
	in.RegisterNamespaced(LibraryName, "randomForest", randomForestFn)
	in.RegisterNamespaced(LibraryName, "randomForestRegressor", randomForestRegressorFn)
	in.RegisterNamespaced(LibraryName, "randomForestWith", randomForestWithFn)
	in.RegisterNamespaced(LibraryName, "randomForestRegressorWith", randomForestRegressorWithFn)
	in.RegisterNamespaced(LibraryName, "standardScaler", standardScalerFn)
	in.RegisterNamespaced(LibraryName, "minMaxScaler", minMaxScalerFn)

	// Apply a fitted model.
	in.RegisterNamespaced(LibraryName, "predict", predictFn)
	in.RegisterNamespaced(LibraryName, "transform", transformFn)
	in.RegisterNamespaced(LibraryName, "predictProba", predictProbaFn)
	in.RegisterNamespaced(LibraryName, "free", freeFn)

	// Introspection - read the learned parameters.
	in.RegisterNamespaced(LibraryName, "coefficients", coefficientsFn)
	in.RegisterNamespaced(LibraryName, "intercept", interceptFn)
	in.RegisterNamespaced(LibraryName, "centroids", centroidsFn)
	in.RegisterNamespaced(LibraryName, "components", componentsFn)
	in.RegisterNamespaced(LibraryName, "explainedVariance", explainedVarianceFn)
	in.RegisterNamespaced(LibraryName, "featureImportances", featureImportancesFn)

	// Model selection / preprocessing.
	in.RegisterNamespaced(LibraryName, "trainTestSplit", trainTestSplitFn)
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

// registerStructs registers the plain-data result structs (split, confusion)
// and the fit options.
func registerStructs(in *interpreter.Interpreter) {
	intT := parser.PrimitiveType(parser.TypeInt)
	in.RegisterNamespacedStruct(LibraryName, "LogisticOptions", []parser.StructField{
		{Name: "learningRate", Type: parser.PrimitiveType(parser.TypeFloat)},
		{Name: "epochs", Type: intT},
	})
	in.RegisterNamespacedStruct(LibraryName, "TreeOptions", []parser.StructField{
		{Name: "maxDepth", Type: intT},
	})
	in.RegisterNamespacedStruct(LibraryName, "ForestOptions", []parser.StructField{
		{Name: "nTrees", Type: intT},
		{Name: "maxDepth", Type: intT},
	})
	in.RegisterNamespacedStruct(LibraryName, "KMeansOptions", []parser.StructField{
		{Name: "maxIter", Type: intT},
	})
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

// --- options structs ---

// fitOptions is a fit function's options struct (ml.LogisticOptions, ...):
// a zero field means the default, a negative one throws, and a count past its
// cap throws "limit".
type fitOptions struct {
	fn     string
	fields []interpreter.StructField
}

// options checks that v is an ml.<typ> and returns its fields.
func options(fn string, v interpreter.Value, typ string) (fitOptions, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != typ {
		return fitOptions{}, fmt.Errorf("ml.%s: argument 3 must be an ml.%s, got %s", fn, typ, v.Kind)
	}
	return fitOptions{fn: fn, fields: v.Fields}, nil
}

func (o fitOptions) field(name string) interpreter.Value {
	for _, f := range o.fields {
		if f.Name == name {
			return f.Value
		}
	}
	return interpreter.IntVal(0)
}

// count reads an int field: 0 -> def, negative -> error, past max -> "limit".
func (o fitOptions) count(name string, def, max int) (int, error) {
	v := o.field(name)
	if v.Kind != interpreter.KindInt {
		return 0, fmt.Errorf("ml.%s: %s must be int, got %s", o.fn, name, v.Kind)
	}
	switch {
	case v.Int == 0:
		return def, nil
	case v.Int < 0:
		return 0, fmt.Errorf("ml.%s: %s must not be negative, got %d", o.fn, name, v.Int)
	case v.Int > int64(max):
		return 0, limits.Capf("ml.%s: %s %d exceeds the limit of %d", o.fn, name, v.Int, max)
	}
	return int(v.Int), nil
}

// float reads a float field: 0 -> def, negative or not finite -> error.
func (o fitOptions) float(name string, def float64) (float64, error) {
	f, ok := o.field(name).AsFloat()
	switch {
	case !ok:
		return 0, fmt.Errorf("ml.%s: %s must be float", o.fn, name)
	case f == 0:
		return def, nil
	case f < 0 || !isFinite(f):
		return 0, fmt.Errorf("ml.%s: %s must be a positive finite number, got %g", o.fn, name, f)
	}
	return f, nil
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

func predictFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("ml.predict expects 2 arguments (model, X), got %d", len(args))
	}
	m, err := resolveModel("predict", args[0])
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
			li, err := labelToInt("ml.predict", v)
			if err != nil {
				return interpreter.Null(), err
			}
			out[i] = li
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

func transformFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("ml.transform expects 2 arguments (model, X), got %d", len(args))
	}
	m, err := resolveModel("transform", args[0])
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

func predictProbaFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("ml.predictProba expects 2 arguments (model, X), got %d", len(args))
	}
	m, err := resolveModel("predictProba", args[0])
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

// freeFn is deprecated (M29.39, removed in 0.29.0): a model is a value the
// garbage collector frees, so there is nothing to release. It checks its
// argument and returns null.
func freeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("ml.free expects 1 argument (model), got %d", len(args))
	}
	if _, ok := args[0].AsNativeObject(LibraryName, "Model"); !ok {
		return interpreter.Null(), fmt.Errorf("ml.free: argument must be an ml.Model, got %s", args[0].Kind)
	}
	return interpreter.Null(), nil
}

// isFinite reports whether r is neither NaN nor infinite.
func isFinite(r float64) bool { return !math.IsNaN(r) && !math.IsInf(r, 0) }

// labelToInt rounds a class label (a user-supplied float) to int64, rejecting a
// value outside the int64 range instead of Go's silent, platform-defined
// out-of-range conversion (which turns a 1e300 label into MinInt64 - a wrong
// result rather than an error). Rounding matches the prior behaviour for
// in-range labels; fn names the caller for the error text.
func labelToInt(fn string, v float64) (int64, error) {
	r := math.Round(v)
	if math.IsNaN(r) || math.IsInf(r, 0) || r >= 9223372036854775808.0 || r < -9223372036854775808.0 {
		return 0, fmt.Errorf("%s: class label %g is out of the integer range", fn, v)
	}
	return int64(r), nil
}

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
