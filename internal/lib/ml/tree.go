// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package mllib

import (
	"fmt"
	"sort"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// treeNode is a CART node: an internal split (feature < threshold, children set)
// or a leaf (value set). `n` and `decrease` record the sample count and the
// impurity decrease at a split, for feature-importance accounting.
type treeNode struct {
	feature   int
	threshold float64
	value     float64 // leaf prediction (class label or mean target)
	n         int
	decrease  float64
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
	return t.value
}

type treeModel struct {
	root    *treeNode
	nf      int
	regress bool
}

func (m *treeModel) numFeatures() int               { return m.nf }
func (m *treeModel) intLabels() bool                { return !m.regress }
func (m *treeModel) predictOne(x []float64) float64 { return m.root.predict(x) }

// gini is the Gini impurity of a label multiset (classification criterion).
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

// varImpurity is the population variance of the targets (regression criterion).
func varImpurity(y []float64) float64 {
	if len(y) < 2 {
		return 0
	}
	return popVar(y)
}

// majority returns the most frequent label (smallest label wins a tie).
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

// buildTree grows a CART node recursively for either criterion (impurity) and
// leaf rule (leaf). featSubset, when non-nil, is the random feature subset a
// random-forest tree considers at each split.
func buildTree(x [][]float64, y []float64, depth, maxDepth int, featSubset []int,
	impurity func([]float64) float64, leaf func([]float64) float64, regress bool) *treeNode {
	node := &treeNode{n: len(y)}
	parentImp := impurity(y)
	if depth >= maxDepth || parentImp == 0 || len(y) < 2 {
		node.value = leaf(y)
		return node
	}
	feats := featSubset
	if feats == nil {
		feats = make([]int, len(x[0]))
		for i := range feats {
			feats[i] = i
		}
	}
	bestFeat, bestThr := bestSplit(x, y, feats, parentImp, regress)
	if bestFeat == -1 {
		node.value = leaf(y)
		return node
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
	node.feature = bestFeat
	node.threshold = bestThr
	parentN := float64(len(y))
	// Impurity decrease (weighted by samples), for feature importance.
	node.decrease = parentN*parentImp - float64(len(ly))*impurity(ly) - float64(len(ry))*impurity(ry)
	node.left = buildTree(lx, ly, depth+1, maxDepth, featSubset, impurity, leaf, regress)
	node.right = buildTree(rx, ry, depth+1, maxDepth, featSubset, impurity, leaf, regress)
	return node
}

// bestSplit finds the (feature, threshold) with the lowest weighted child
// impurity that also beats the parent impurity, or (-1, 0) if none does. Each
// feature is sorted once and swept in order with running impurity - running
// per-class counts for Gini, running sum and sum-of-squares for variance - so a
// node costs O(features * n log n) instead of re-partitioning the whole node for
// every candidate threshold (the O(features * unique * n), ~O(d * n^2) cost the
// per-threshold search had). Thresholds are midpoints of adjacent distinct
// feature values and the first lowest-scoring split wins, matching the search it
// replaces.
func bestSplit(x [][]float64, y []float64, feats []int, parentImp float64, regress bool) (int, float64) {
	n := len(y)
	parentN := float64(n)
	bestScore, bestFeat, bestThr := parentImp, -1, 0.0

	// Classification sweeps per-class counts; index the labels seen at this node.
	var classOf map[float64]int
	nClasses := 0
	if !regress {
		classOf = make(map[float64]int, 8)
		for _, v := range y {
			if _, ok := classOf[v]; !ok {
				classOf[v] = nClasses
				nClasses++
			}
		}
	}

	order := make([]int, n)
	cntL := make([]int, nClasses)
	cntR := make([]int, nClasses)
	for _, f := range feats {
		ff := f
		for i := range order {
			order[i] = i
		}
		sort.Slice(order, func(a, b int) bool { return x[order[a]][ff] < x[order[b]][ff] })

		if regress {
			// gain is tracked as the weighted child variance; move one sample from
			// the right side to the left at each step, updating sum / sum-of-squares.
			var sumL, sqL, sumR, sqR float64
			for _, v := range y {
				sumR += v
				sqR += v * v
			}
			nL := 0
			for i := 0; i+1 < n; i++ {
				oi := order[i]
				v := y[oi]
				sumL += v
				sqL += v * v
				sumR -= v
				sqR -= v * v
				nL++
				// A split is valid only between two distinct feature values.
				if x[oi][ff] == x[order[i+1]][ff] {
					continue
				}
				nR := n - nL
				// nSide * variance(side) = sumSq - sum^2 / nSide.
				wImp := ((sqL - sumL*sumL/float64(nL)) + (sqR - sumR*sumR/float64(nR))) / parentN
				if wImp < bestScore {
					bestScore, bestFeat, bestThr = wImp, ff, (x[oi][ff]+x[order[i+1]][ff])/2
				}
			}
			continue
		}

		for c := range cntL {
			cntL[c] = 0
			cntR[c] = 0
		}
		var sqL, sqR float64
		for _, v := range y {
			cntR[classOf[v]]++
		}
		for _, c := range cntR {
			sqR += float64(c) * float64(c)
		}
		nL := 0
		for i := 0; i+1 < n; i++ {
			oi := order[i]
			c := classOf[y[oi]]
			// (k+1)^2 - k^2 = 2k+1 on the left; (k-1)^2 - k^2 = 1-2k on the right.
			sqL += float64(2*cntL[c] + 1)
			cntL[c]++
			sqR += float64(1 - 2*cntR[c])
			cntR[c]--
			nL++
			if x[oi][ff] == x[order[i+1]][ff] {
				continue
			}
			nR := n - nL
			// nSide * gini(side) = nSide - sumSq(counts) / nSide, so the weighted
			// child Gini is parentN - sqL/nL - sqR/nR, all over parentN.
			wImp := (parentN - sqL/float64(nL) - sqR/float64(nR)) / parentN
			if wImp < bestScore {
				bestScore, bestFeat, bestThr = wImp, ff, (x[oi][ff]+x[order[i+1]][ff])/2
			}
		}
	}
	return bestFeat, bestThr
}

// treeCriteria returns the (impurity, leaf) pair for a classification or
// regression tree.
func treeCriteria(regress bool) (func([]float64) float64, func([]float64) float64) {
	if regress {
		return varImpurity, mean
	}
	return gini, majority
}

// accumulateImportance sums each feature's impurity decrease over the tree.
func accumulateImportance(root *treeNode, imp []float64) {
	if root == nil || root.left == nil {
		return
	}
	imp[root.feature] += root.decrease
	accumulateImportance(root.left, imp)
	accumulateImportance(root.right, imp)
}

func (r *registry) decisionTreeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return r.fitTree("decisionTree", args, false)
}
func (r *registry) decisionTreeRegressorFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return r.fitTree("decisionTreeRegressor", args, true)
}

func (r *registry) fitTree(name string, args []interpreter.Value, regress bool) (interpreter.Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return interpreter.Null(), fmt.Errorf("ml.%s expects 2 or 3 arguments (X, y [, maxDepth]), got %d", name, len(args))
	}
	x, y, err := fitData(name, args)
	if err != nil {
		return interpreter.Null(), err
	}
	maxDepth := 8
	if len(args) == 3 {
		if args[2].Kind != interpreter.KindInt || args[2].Int < 1 || args[2].Int > maxTreeDepth {
			return interpreter.Null(), fmt.Errorf("ml.%s: maxDepth must be in [1, %d]", name, maxTreeDepth)
		}
		maxDepth = int(args[2].Int)
	}
	imp, leaf := treeCriteria(regress)
	return r.store(&treeModel{root: buildTree(x, y, 0, maxDepth, nil, imp, leaf, regress), nf: len(x[0]), regress: regress}), nil
}

// --- random forest ---

type forestModel struct {
	trees   []*treeNode
	nf      int
	regress bool
}

func (m *forestModel) numFeatures() int { return m.nf }
func (m *forestModel) intLabels() bool  { return !m.regress }
func (m *forestModel) predictOne(x []float64) float64 {
	votes := make([]float64, len(m.trees))
	for i, t := range m.trees {
		votes[i] = t.predict(x)
	}
	if m.regress {
		return mean(votes)
	}
	return majority(votes)
}

func (r *registry) randomForestFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return r.fitForest("randomForest", args, false)
}
func (r *registry) randomForestRegressorFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return r.fitForest("randomForestRegressor", args, true)
}

func (r *registry) fitForest(name string, args []interpreter.Value, regress bool) (interpreter.Value, error) {
	if len(args) < 2 || len(args) > 4 {
		return interpreter.Null(), fmt.Errorf("ml.%s expects 2 to 4 arguments (X, y [, nTrees [, maxDepth]]), got %d", name, len(args))
	}
	x, y, err := fitData(name, args)
	if err != nil {
		return interpreter.Null(), err
	}
	nTrees, maxDepth := 10, 8
	if len(args) >= 3 {
		if args[2].Kind != interpreter.KindInt || args[2].Int < 1 || args[2].Int > maxForestTrees {
			return interpreter.Null(), fmt.Errorf("ml.%s: nTrees must be in [1, %d]", name, maxForestTrees)
		}
		nTrees = int(args[2].Int)
	}
	if len(args) == 4 {
		if args[3].Kind != interpreter.KindInt || args[3].Int < 1 || args[3].Int > maxTreeDepth {
			return interpreter.Null(), fmt.Errorf("ml.%s: maxDepth must be in [1, %d]", name, maxTreeDepth)
		}
		maxDepth = int(args[3].Int)
	}
	n, d := len(x), len(x[0])
	mtry := int(sqrtInt(d))
	if mtry < 1 {
		mtry = 1
	}
	imp, leaf := treeCriteria(regress)
	trees := make([]*treeNode, nTrees)
	for t := 0; t < nTrees; t++ {
		bx := make([][]float64, n)
		by := make([]float64, n)
		for i := 0; i < n; i++ {
			s := randIntN(n)
			bx[i], by[i] = x[s], y[s]
		}
		trees[t] = buildTree(bx, by, 0, maxDepth, sampleFeatures(d, mtry), imp, leaf, regress)
	}
	return r.store(&forestModel{trees: trees, nf: d, regress: regress}), nil
}

// sqrtInt returns sqrt(n) as a float (local Newton iteration).
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
