// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package mllib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// treeNode is a CART node: an internal split (feature < threshold) or a leaf
// (label set, children nil).
type treeNode struct {
	feature   int
	threshold float64
	label     float64
	left      *treeNode
	right     *treeNode
}

func (t *treeNode) predict(x []float64) float64 {
	for t.left != nil {
		if x[t.feature] < t.threshold {
			t = t.left
		} else {
			t = t.right
		}
	}
	return t.label
}

type treeModel struct {
	root *treeNode
	nf   int
}

func (m *treeModel) numFeatures() int               { return m.nf }
func (m *treeModel) intLabels() bool                { return true }
func (m *treeModel) predictOne(x []float64) float64 { return m.root.predict(x) }

// gini is the Gini impurity of a label multiset.
func gini(y []float64) float64 {
	if len(y) == 0 {
		return 0
	}
	counts := map[float64]int{}
	for _, v := range y {
		counts[v]++
	}
	imp := 1.0
	n := float64(len(y))
	for _, c := range counts {
		p := float64(c) / n
		imp -= p * p
	}
	return imp
}

// majority returns the most frequent label (smallest label wins a tie, for
// determinism).
func majority(y []float64) float64 {
	counts := map[float64]int{}
	for _, v := range y {
		counts[v]++
	}
	best, bestN := y[0], 0
	for _, v := range uniqueSorted(y) {
		if counts[v] > bestN {
			bestN, best = counts[v], v
		}
	}
	return best
}

// buildTree grows a CART classifier recursively. featSubset, when non-nil, is
// the random feature subset a random-forest tree considers at each split.
func buildTree(x [][]float64, y []float64, depth, maxDepth int, featSubset []int) *treeNode {
	if depth >= maxDepth || gini(y) == 0 || len(y) < 2 {
		return &treeNode{label: majority(y)}
	}
	feats := featSubset
	if feats == nil {
		feats = make([]int, len(x[0]))
		for i := range feats {
			feats[i] = i
		}
	}
	bestGini, bestFeat, bestThr := gini(y), -1, 0.0
	parentN := float64(len(y))
	for _, f := range feats {
		// Candidate thresholds: midpoints between consecutive sorted values.
		vals := make([]float64, len(x))
		for i := range x {
			vals[i] = x[i][f]
		}
		uniq := uniqueSorted(vals)
		for t := 0; t+1 < len(uniq); t++ {
			thr := (uniq[t] + uniq[t+1]) / 2
			var ly, ry []float64
			for i := range x {
				if x[i][f] < thr {
					ly = append(ly, y[i])
				} else {
					ry = append(ry, y[i])
				}
			}
			if len(ly) == 0 || len(ry) == 0 {
				continue
			}
			w := (float64(len(ly))*gini(ly) + float64(len(ry))*gini(ry)) / parentN
			if w < bestGini {
				bestGini, bestFeat, bestThr = w, f, thr
			}
		}
	}
	if bestFeat == -1 {
		return &treeNode{label: majority(y)}
	}
	var lx, rx [][]float64
	var ly, ry []float64
	for i := range x {
		if x[i][bestFeat] < bestThr {
			lx = append(lx, x[i])
			ly = append(ly, y[i])
		} else {
			rx = append(rx, x[i])
			ry = append(ry, y[i])
		}
	}
	return &treeNode{
		feature:   bestFeat,
		threshold: bestThr,
		left:      buildTree(lx, ly, depth+1, maxDepth, featSubset),
		right:     buildTree(rx, ry, depth+1, maxDepth, featSubset),
	}
}

func (r *registry) decisionTreeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return interpreter.Null(), fmt.Errorf("ml.decisionTree expects 2 or 3 arguments (X, y [, maxDepth]), got %d", len(args))
	}
	x, y, err := fitData("decisionTree", args)
	if err != nil {
		return interpreter.Null(), err
	}
	maxDepth := 8
	if len(args) == 3 {
		if args[2].Kind != interpreter.KindInt || args[2].Int < 1 || args[2].Int > maxTreeDepth {
			return interpreter.Null(), fmt.Errorf("ml.decisionTree: maxDepth must be in [1, %d]", maxTreeDepth)
		}
		maxDepth = int(args[2].Int)
	}
	return r.store(&treeModel{root: buildTree(x, y, 0, maxDepth, nil), nf: len(x[0])}), nil
}

// --- random forest ---

type forestModel struct {
	trees []*treeNode
	nf    int
}

func (m *forestModel) numFeatures() int { return m.nf }
func (m *forestModel) intLabels() bool  { return true }
func (m *forestModel) predictOne(x []float64) float64 {
	votes := make([]float64, len(m.trees))
	for i, t := range m.trees {
		votes[i] = t.predict(x)
	}
	return majority(votes)
}

func (r *registry) randomForestFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) < 2 || len(args) > 4 {
		return interpreter.Null(), fmt.Errorf("ml.randomForest expects 2 to 4 arguments (X, y [, nTrees [, maxDepth]]), got %d", len(args))
	}
	x, y, err := fitData("randomForest", args)
	if err != nil {
		return interpreter.Null(), err
	}
	nTrees, maxDepth := 10, 8
	if len(args) >= 3 {
		if args[2].Kind != interpreter.KindInt || args[2].Int < 1 || args[2].Int > maxForestTrees {
			return interpreter.Null(), fmt.Errorf("ml.randomForest: nTrees must be in [1, %d]", maxForestTrees)
		}
		nTrees = int(args[2].Int)
	}
	if len(args) == 4 {
		if args[3].Kind != interpreter.KindInt || args[3].Int < 1 || args[3].Int > maxTreeDepth {
			return interpreter.Null(), fmt.Errorf("ml.randomForest: maxDepth must be in [1, %d]", maxTreeDepth)
		}
		maxDepth = int(args[3].Int)
	}
	n, d := len(x), len(x[0])
	// sqrt(d) features per split, at least 1 (the standard classification rule).
	mtry := int(sqrtInt(d))
	if mtry < 1 {
		mtry = 1
	}
	trees := make([]*treeNode, nTrees)
	for t := 0; t < nTrees; t++ {
		// Bootstrap sample (sampling with replacement) + a per-tree feature subset.
		bx := make([][]float64, n)
		by := make([]float64, n)
		for i := 0; i < n; i++ {
			s := randIntN(n)
			bx[i], by[i] = x[s], y[s]
		}
		feats := sampleFeatures(d, mtry)
		trees[t] = buildTree(bx, by, 0, maxDepth, feats)
	}
	return r.store(&forestModel{trees: trees, nf: d}), nil
}

// sqrtInt returns floor(sqrt(n)) as a float without importing math into the hot
// callers (kept local for readability).
func sqrtInt(n int) float64 {
	x := float64(n)
	if x <= 0 {
		return 0
	}
	g := x
	for i := 0; i < 40; i++ {
		g = 0.5 * (g + x/g)
	}
	return g
}

// sampleFeatures picks `m` distinct feature indices out of d (partial
// Fisher-Yates) from the shared random source.
func sampleFeatures(d, m int) []int {
	idx := make([]int, d)
	for i := range idx {
		idx[i] = i
	}
	for i := 0; i < m; i++ {
		j := i + randIntN(d-i)
		idx[i], idx[j] = idx[j], idx[i]
	}
	return idx[:m]
}
