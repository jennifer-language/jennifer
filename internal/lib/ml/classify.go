// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package mllib

import (
	"fmt"
	"math"
	"sort"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// --- k-nearest-neighbours (classifier) ---

type knnModel struct {
	x  [][]float64
	y  []float64 // class labels
	k  int
	nf int
}

func (m *knnModel) numFeatures() int { return m.nf }
func (m *knnModel) intLabels() bool  { return true }
func (m *knnModel) predictOne(x []float64) float64 {
	type nb struct {
		d float64
		y float64
	}
	nbs := make([]nb, len(m.x))
	for i := range m.x {
		nbs[i] = nb{euclid2(x, m.x[i]), m.y[i]}
	}
	sort.Slice(nbs, func(a, b int) bool { return nbs[a].d < nbs[b].d })
	votes := map[float64]int{}
	best, bestN := nbs[0].y, 0
	for i := 0; i < m.k && i < len(nbs); i++ {
		votes[nbs[i].y]++
		if v := votes[nbs[i].y]; v > bestN {
			bestN, best = v, nbs[i].y
		}
	}
	return best
}

func (r *registry) kNNFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("ml.kNN expects 3 arguments (X, y, k), got %d", len(args))
	}
	x, y, err := fitData("kNN", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if args[2].Kind != interpreter.KindInt || args[2].Int < 1 {
		return interpreter.Null(), fmt.Errorf("ml.kNN: k must be a positive int")
	}
	k := int(args[2].Int)
	if k > len(x) {
		return interpreter.Null(), fmt.Errorf("ml.kNN: k=%d exceeds the %d training rows", k, len(x))
	}
	return r.store(&knnModel{x: x, y: y, k: k, nf: len(x[0])}), nil
}

// --- Gaussian naive Bayes ---

type naiveBayesModel struct {
	classes []float64
	priors  []float64
	means   [][]float64 // [class][feature]
	vars    [][]float64
	nf      int
}

func (m *naiveBayesModel) numFeatures() int { return m.nf }
func (m *naiveBayesModel) intLabels() bool  { return true }
func (m *naiveBayesModel) predictOne(x []float64) float64 {
	best, bestLL := m.classes[0], math.Inf(-1)
	for c := range m.classes {
		ll := math.Log(m.priors[c])
		for j := 0; j < m.nf; j++ {
			v := m.vars[c][j]
			d := x[j] - m.means[c][j]
			ll += -0.5*math.Log(2*math.Pi*v) - d*d/(2*v)
		}
		if ll > bestLL {
			bestLL, best = ll, m.classes[c]
		}
	}
	return best
}

func (r *registry) naiveBayesFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("ml.naiveBayes expects 2 arguments (X, y), got %d", len(args))
	}
	x, y, err := fitData("naiveBayes", args)
	if err != nil {
		return interpreter.Null(), err
	}
	classes := uniqueSorted(y)
	nf := len(x[0])
	m := &naiveBayesModel{classes: classes, nf: nf,
		priors: make([]float64, len(classes)),
		means:  make([][]float64, len(classes)),
		vars:   make([][]float64, len(classes))}
	// A tiny variance floor keeps a zero-variance feature from dividing by zero
	// (Gaussian NB's standard epsilon smoothing).
	globalVar := 0.0
	for j := 0; j < nf; j++ {
		col := make([]float64, len(x))
		for i := range x {
			col[i] = x[i][j]
		}
		globalVar += popVar(col)
	}
	eps := 1e-9 * globalVar / float64(nf)
	if eps <= 0 {
		eps = 1e-9
	}
	for ci, c := range classes {
		var rows [][]float64
		for i := range x {
			if y[i] == c {
				rows = append(rows, x[i])
			}
		}
		m.priors[ci] = float64(len(rows)) / float64(len(x))
		m.means[ci] = make([]float64, nf)
		m.vars[ci] = make([]float64, nf)
		for j := 0; j < nf; j++ {
			col := make([]float64, len(rows))
			for i := range rows {
				col[i] = rows[i][j]
			}
			m.means[ci][j] = mean(col)
			m.vars[ci][j] = popVar(col) + eps
		}
	}
	return r.store(m), nil
}

// --- logistic regression (binary) ---

type logisticModel struct {
	w  []float64
	b  float64
	nf int
}

func (m *logisticModel) numFeatures() int { return m.nf }
func (m *logisticModel) intLabels() bool  { return true }
func (m *logisticModel) probaOne(x []float64) float64 {
	z := m.b
	for i, wi := range m.w {
		z += wi * x[i]
	}
	return sigmoid(z)
}
func (m *logisticModel) predictOne(x []float64) float64 {
	if m.probaOne(x) >= 0.5 {
		return 1
	}
	return 0
}

func sigmoid(z float64) float64 {
	if z >= 0 {
		return 1 / (1 + math.Exp(-z))
	}
	e := math.Exp(z)
	return e / (1 + e)
}

func (r *registry) logisticRegressionFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) < 2 || len(args) > 4 {
		return interpreter.Null(), fmt.Errorf("ml.logisticRegression expects 2 to 4 arguments (X, y [, lr [, epochs]]), got %d", len(args))
	}
	x, y, err := fitData("logisticRegression", args)
	if err != nil {
		return interpreter.Null(), err
	}
	for _, v := range y {
		if v != 0 && v != 1 {
			return interpreter.Null(), fmt.Errorf("ml.logisticRegression: labels must be 0 or 1 (binary), got %s", interpreter.DisplayFloat(v))
		}
	}
	lr := 0.1
	epochs := 1000
	if len(args) >= 3 {
		if f, ok := args[2].AsFloat(); ok && f > 0 {
			lr = f
		} else {
			return interpreter.Null(), fmt.Errorf("ml.logisticRegression: lr must be a positive number")
		}
	}
	if len(args) == 4 {
		if args[3].Kind != interpreter.KindInt || args[3].Int < 1 || args[3].Int > maxLogisticEpochs {
			return interpreter.Null(), fmt.Errorf("ml.logisticRegression: epochs must be in [1, %d]", maxLogisticEpochs)
		}
		epochs = int(args[3].Int)
	}
	nf := len(x[0])
	w := make([]float64, nf)
	b := 0.0
	n := float64(len(x))
	for e := 0; e < epochs; e++ {
		gw := make([]float64, nf)
		gb := 0.0
		for i := range x {
			z := b
			for j := 0; j < nf; j++ {
				z += w[j] * x[i][j]
			}
			err := sigmoid(z) - y[i]
			gb += err
			for j := 0; j < nf; j++ {
				gw[j] += err * x[i][j]
			}
		}
		b -= lr * gb / n
		for j := 0; j < nf; j++ {
			w[j] -= lr * gw[j] / n
		}
	}
	for _, wj := range w {
		if !isFinite(wj) {
			return interpreter.Null(), fmt.Errorf("ml.logisticRegression: training diverged (try a smaller lr or scaled features)")
		}
	}
	return r.store(&logisticModel{w: w, b: b, nf: nf}), nil
}

// --- shared helpers ---

func mean(fs []float64) float64 {
	s := 0.0
	for _, f := range fs {
		s += f
	}
	return s / float64(len(fs))
}

func popVar(fs []float64) float64 {
	m := mean(fs)
	s := 0.0
	for _, f := range fs {
		d := f - m
		s += d * d
	}
	return s / float64(len(fs))
}

// uniqueSorted returns the distinct values of y in ascending order.
func uniqueSorted(y []float64) []float64 {
	seen := map[float64]bool{}
	var out []float64
	for _, v := range y {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Float64s(out)
	return out
}
