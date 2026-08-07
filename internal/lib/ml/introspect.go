// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Model introspection: read the parameters a fitted model learned. Without
// these a model is a black box you can only predict with; scikit-learn exposes
// the same via `.coef_` / `.intercept_` / `.cluster_centers_` / `.components_` /
// `.feature_importances_`.
package mllib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// coefficientsFn returns the learned coefficients: a `list of float` for a
// linear / ridge / lasso / binary-logistic model, or a `list of list of float`
// (one row per class) for multiclass logistic.
func (r *registry) coefficientsFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	m, err := r.resolve1("coefficients", args)
	if err != nil {
		return interpreter.Null(), err
	}
	switch v := m.(type) {
	case *linearModel:
		return finiteVec("coefficients", v.coef)
	case *logisticModel:
		return finiteVec("coefficients", v.w)
	case *logisticMultiModel:
		return finiteMat("coefficients", v.w)
	}
	return interpreter.Null(), fmt.Errorf("ml.coefficients: this model has no coefficients")
}

// interceptFn returns the model intercept / bias: a `float` for a linear or
// binary-logistic model, a `list of float` (per class) for multiclass logistic.
func (r *registry) interceptFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	m, err := r.resolve1("intercept", args)
	if err != nil {
		return interpreter.Null(), err
	}
	switch v := m.(type) {
	case *linearModel:
		return finiteScalar("intercept", v.intercept)
	case *logisticModel:
		return finiteScalar("intercept", v.b)
	case *logisticMultiModel:
		return finiteVec("intercept", v.b)
	}
	return interpreter.Null(), fmt.Errorf("ml.intercept: this model has no intercept")
}

// centroidsFn returns a k-means model's cluster centres (`list of list of float`).
func (r *registry) centroidsFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	m, err := r.resolve1("centroids", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if v, okk := m.(*kMeansModel); okk {
		return finiteMat("centroids", v.centroids)
	}
	return interpreter.Null(), fmt.Errorf("ml.centroids: not a k-means model")
}

// componentsFn returns a PCA model's principal axes (`list of list of float`,
// one row per component).
func (r *registry) componentsFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	m, err := r.resolve1("components", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if v, okk := m.(*pcaModel); okk {
		return finiteMat("components", v.components)
	}
	return interpreter.Null(), fmt.Errorf("ml.components: not a PCA model")
}

// explainedVarianceFn returns a PCA model's per-component explained-variance
// ratios (`list of float`, summing to <= 1) - used to choose nComponents.
func (r *registry) explainedVarianceFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	m, err := r.resolve1("explainedVariance", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if v, okk := m.(*pcaModel); okk {
		return finiteVec("explainedVariance", v.explained)
	}
	return interpreter.Null(), fmt.Errorf("ml.explainedVariance: not a PCA model")
}

// featureImportancesFn returns the Gini feature importances of a decision tree
// or random forest (`list of float`, summing to 1).
func (r *registry) featureImportancesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	m, err := r.resolve1("featureImportances", args)
	if err != nil {
		return interpreter.Null(), err
	}
	var imp []float64
	switch v := m.(type) {
	case *treeModel:
		imp = make([]float64, v.nf)
		accumulateImportance(v.root, imp)
	case *forestModel:
		imp = make([]float64, v.nf)
		for _, t := range v.trees {
			accumulateImportance(t, imp)
		}
	default:
		return interpreter.Null(), fmt.Errorf("ml.featureImportances: not a tree / forest model")
	}
	total := 0.0
	for _, v := range imp {
		total += v
	}
	if total > 0 {
		for i := range imp {
			imp[i] /= total
		}
	}
	return finiteVec("featureImportances", imp)
}

// resolve1 resolves the single ml.Model argument of an accessor.
func (r *registry) resolve1(name string, args []interpreter.Value) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("ml.%s expects 1 argument (model), got %d", name, len(args))
	}
	return r.resolve(name, args[0])
}

// finiteVec / finiteMat build a value from stored model parameters, but re-check
// finiteness at the read boundary rather than trusting the fit-time invariant -
// so a model's internals can never leak a NaN/Inf through an accessor even if a
// future fit path forgot to validate.
func finiteVec(name string, fs []float64) (interpreter.Value, error) {
	for _, f := range fs {
		if !isFinite(f) {
			return interpreter.Null(), fmt.Errorf("ml.%s: model contains a non-finite value", name)
		}
	}
	return floatVec(fs), nil
}

func finiteMat(name string, m [][]float64) (interpreter.Value, error) {
	for _, row := range m {
		for _, f := range row {
			if !isFinite(f) {
				return interpreter.Null(), fmt.Errorf("ml.%s: model contains a non-finite value", name)
			}
		}
	}
	return floatMat(m), nil
}

func finiteScalar(name string, f float64) (interpreter.Value, error) {
	if !isFinite(f) {
		return interpreter.Null(), fmt.Errorf("ml.%s: model contains a non-finite value", name)
	}
	return interpreter.FloatVal(f), nil
}
