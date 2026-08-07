// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package mllib

import (
	"fmt"
	"math"
	"sort"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// twoVectors reads two equal-length numeric-list arguments (yTrue, yPred).
func twoVectors(name string, args []interpreter.Value) ([]float64, []float64, error) {
	if len(args) < 2 {
		return nil, nil, fmt.Errorf("ml.%s expects at least 2 list arguments", name)
	}
	a, err := vector(name, args, 0)
	if err != nil {
		return nil, nil, err
	}
	b, err := vector(name, args, 1)
	if err != nil {
		return nil, nil, err
	}
	if len(a) != len(b) {
		return nil, nil, fmt.Errorf("ml.%s: the two lists must have equal length (%d vs %d)", name, len(a), len(b))
	}
	if len(a) == 0 {
		return nil, nil, fmt.Errorf("ml.%s: lists are empty", name)
	}
	return a, b, nil
}

func floatResult(name string, r float64) (interpreter.Value, error) {
	if !isFinite(r) {
		return interpreter.Null(), fmt.Errorf("ml.%s: result is undefined or infinite", name)
	}
	return interpreter.FloatVal(r), nil
}

// --- classification metrics ---

func accuracyFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	yt, yp, err := twoVectors("accuracy", args)
	if err != nil {
		return interpreter.Null(), err
	}
	correct := 0
	for i := range yt {
		if yt[i] == yp[i] {
			correct++
		}
	}
	return interpreter.FloatVal(float64(correct) / float64(len(yt))), nil
}

// binaryCounts tallies TP / FP / FN for a given positive label.
func binaryCounts(yt, yp []float64, pos float64) (tp, fp, fn int) {
	for i := range yt {
		predPos := yp[i] == pos
		actualPos := yt[i] == pos
		switch {
		case predPos && actualPos:
			tp++
		case predPos && !actualPos:
			fp++
		case !predPos && actualPos:
			fn++
		}
	}
	return
}

// posLabelArg reads an optional third argument as the positive class label
// (default 1).
func posLabelArg(args []interpreter.Value) (float64, error) {
	if len(args) < 3 {
		return 1, nil
	}
	p, ok := args[2].AsFloat()
	if !ok {
		return 0, fmt.Errorf("positive label must be int or float, got %s", args[2].Kind)
	}
	return p, nil
}

func precisionFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	yt, yp, err := twoVectors("precision", args)
	if err != nil {
		return interpreter.Null(), err
	}
	pos, err := posLabelArg(args)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("ml.precision: %v", err)
	}
	tp, fp, _ := binaryCounts(yt, yp, pos)
	if tp+fp == 0 {
		return interpreter.FloatVal(0), nil // no positive predictions
	}
	return interpreter.FloatVal(float64(tp) / float64(tp+fp)), nil
}

func recallFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	yt, yp, err := twoVectors("recall", args)
	if err != nil {
		return interpreter.Null(), err
	}
	pos, err := posLabelArg(args)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("ml.recall: %v", err)
	}
	tp, _, fn := binaryCounts(yt, yp, pos)
	if tp+fn == 0 {
		return interpreter.FloatVal(0), nil // no actual positives
	}
	return interpreter.FloatVal(float64(tp) / float64(tp+fn)), nil
}

func f1Fn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	yt, yp, err := twoVectors("f1", args)
	if err != nil {
		return interpreter.Null(), err
	}
	pos, err := posLabelArg(args)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("ml.f1: %v", err)
	}
	tp, fp, fn := binaryCounts(yt, yp, pos)
	if 2*tp+fp+fn == 0 {
		return interpreter.FloatVal(0), nil
	}
	return interpreter.FloatVal(2 * float64(tp) / float64(2*tp+fp+fn)), nil
}

func confusionMatrixFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	yt, yp, err := twoVectors("confusionMatrix", args)
	if err != nil {
		return interpreter.Null(), err
	}
	// Labels = sorted union of true and predicted.
	all := append(append([]float64{}, yt...), yp...)
	labels := uniqueSorted(all)
	idx := map[float64]int{}
	for i, l := range labels {
		idx[l] = i
	}
	m := make([][]int64, len(labels))
	for i := range m {
		m[i] = make([]int64, len(labels))
	}
	for i := range yt {
		m[idx[yt[i]]][idx[yp[i]]]++
	}
	li := make([]int64, len(labels))
	for i, l := range labels {
		li[i] = int64(math.Round(l))
	}
	return interpreter.NamespacedStructVal(LibraryName, "Confusion", []interpreter.StructField{
		{Name: "labels", Value: intVec(li)},
		{Name: "matrix", Value: intMat(m)},
	}), nil
}

// rocAucFn computes the binary ROC-AUC from true 0/1 labels and predicted
// scores via the rank (Mann-Whitney U) statistic, with tie-aware average ranks.
func rocAucFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	yt, scores, err := twoVectors("rocAuc", args)
	if err != nil {
		return interpreter.Null(), err
	}
	var nPos, nNeg int
	for _, v := range yt {
		switch v {
		case 1:
			nPos++
		case 0:
			nNeg++
		default:
			return interpreter.Null(), fmt.Errorf("ml.rocAuc: true labels must be 0 or 1, got %s", interpreter.DisplayFloat(v))
		}
	}
	if nPos == 0 || nNeg == 0 {
		return interpreter.Null(), fmt.Errorf("ml.rocAuc: need both positive and negative examples")
	}
	// Average ranks (1-based) of the scores, ties shared.
	order := make([]int, len(scores))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return scores[order[a]] < scores[order[b]] })
	ranks := make([]float64, len(scores))
	for i := 0; i < len(order); {
		j := i
		for j < len(order) && scores[order[j]] == scores[order[i]] {
			j++
		}
		avg := float64(i+j+1) / 2 // mean of ranks (i+1 .. j), 1-based
		for t := i; t < j; t++ {
			ranks[order[t]] = avg
		}
		i = j
	}
	sumPos := 0.0
	for i := range yt {
		if yt[i] == 1 {
			sumPos += ranks[i]
		}
	}
	auc := (sumPos - float64(nPos)*float64(nPos+1)/2) / (float64(nPos) * float64(nNeg))
	return floatResult("rocAuc", auc)
}

// --- regression metrics ---

func rmseFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	yt, yp, err := twoVectors("rmse", args)
	if err != nil {
		return interpreter.Null(), err
	}
	s := 0.0
	for i := range yt {
		d := yt[i] - yp[i]
		s += d * d
	}
	return floatResult("rmse", math.Sqrt(s/float64(len(yt))))
}

func mseFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	yt, yp, err := twoVectors("mse", args)
	if err != nil {
		return interpreter.Null(), err
	}
	s := 0.0
	for i := range yt {
		d := yt[i] - yp[i]
		s += d * d
	}
	return floatResult("mse", s/float64(len(yt)))
}

// logLossFn is the binary cross-entropy between 0/1 labels and predicted
// probabilities, with the probabilities clipped away from 0 and 1 so the log
// stays finite.
func logLossFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	yt, p, err := twoVectors("logLoss", args)
	if err != nil {
		return interpreter.Null(), err
	}
	const eps = 1e-15
	s := 0.0
	for i := range yt {
		if yt[i] != 0 && yt[i] != 1 {
			return interpreter.Null(), fmt.Errorf("ml.logLoss: true labels must be 0 or 1, got %s", interpreter.DisplayFloat(yt[i]))
		}
		pi := p[i]
		if pi < 0 || pi > 1 {
			return interpreter.Null(), fmt.Errorf("ml.logLoss: probability %d must be in [0, 1], got %s", i, interpreter.DisplayFloat(pi))
		}
		// Clip away from the exact endpoints so log(0) / log(1) stay finite.
		if pi < eps {
			pi = eps
		} else if pi > 1-eps {
			pi = 1 - eps
		}
		s += yt[i]*math.Log(pi) + (1-yt[i])*math.Log(1-pi)
	}
	return floatResult("logLoss", -s/float64(len(yt)))
}

func maeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	yt, yp, err := twoVectors("mae", args)
	if err != nil {
		return interpreter.Null(), err
	}
	s := 0.0
	for i := range yt {
		s += math.Abs(yt[i] - yp[i])
	}
	return floatResult("mae", s/float64(len(yt)))
}

func r2Fn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	yt, yp, err := twoVectors("r2", args)
	if err != nil {
		return interpreter.Null(), err
	}
	m := mean(yt)
	var ssRes, ssTot float64
	for i := range yt {
		dr := yt[i] - yp[i]
		dt := yt[i] - m
		ssRes += dr * dr
		ssTot += dt * dt
	}
	if ssTot == 0 {
		return interpreter.Null(), fmt.Errorf("ml.r2: true values have zero variance (R^2 is undefined)")
	}
	return floatResult("r2", 1-ssRes/ssTot)
}
