// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package mllib

import (
	"fmt"
	"math"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// kMeansModel holds the learned centroids; predict assigns the nearest one.
type kMeansModel struct {
	centroids [][]float64
	nf        int
}

func (m *kMeansModel) numFeatures() int { return m.nf }
func (m *kMeansModel) intLabels() bool  { return true }
func (m *kMeansModel) predictOne(x []float64) float64 {
	best, bestD := 0, math.Inf(1)
	for c, cen := range m.centroids {
		if d := euclid2(x, cen); d < bestD {
			bestD, best = d, c
		}
	}
	return float64(best)
}

func (r *registry) kMeansFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return interpreter.Null(), fmt.Errorf("ml.kMeans expects 2 or 3 arguments (X, k [, maxIter]), got %d", len(args))
	}
	x, err := matrix("kMeans", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("ml.kMeans: k must be an int")
	}
	k := int(args[1].Int)
	if k < 1 || k > len(x) {
		return interpreter.Null(), fmt.Errorf("ml.kMeans: k must be in [1, rows] (got k=%d, rows=%d)", k, len(x))
	}
	maxIter := 100
	if len(args) == 3 {
		if args[2].Kind != interpreter.KindInt || args[2].Int < 1 || args[2].Int > maxKMeansIter {
			return interpreter.Null(), fmt.Errorf("ml.kMeans: maxIter must be in [1, %d]", maxKMeansIter)
		}
		maxIter = int(args[2].Int)
	}
	cen, err := kMeansFit(x, k, maxIter)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("ml.kMeans: %v", err)
	}
	return r.store(&kMeansModel{centroids: cen, nf: len(x[0])}), nil
}

// kMeansFit runs Lloyd's algorithm with k-means++ seeding, drawing from `math`'s
// shared random source (so `math.randSeed` makes it reproducible).
func kMeansFit(x [][]float64, k, maxIter int) ([][]float64, error) {
	n, d := len(x), len(x[0])
	centroids := kmeansppSeed(x, k)
	assign := make([]int, n)
	for iter := 0; iter < maxIter; iter++ {
		changed := false
		for i, row := range x {
			best, bestD := 0, math.Inf(1)
			for c, cen := range centroids {
				if dd := euclid2(row, cen); dd < bestD {
					bestD, best = dd, c
				}
			}
			if best != assign[i] {
				changed = true
			}
			assign[i] = best
		}
		// Recompute centroids as the mean of their members.
		sums := make([][]float64, k)
		counts := make([]int, k)
		for c := range sums {
			sums[c] = make([]float64, d)
		}
		for i, row := range x {
			c := assign[i]
			counts[c]++
			for j := 0; j < d; j++ {
				sums[c][j] += row[j]
			}
		}
		for c := 0; c < k; c++ {
			if counts[c] == 0 {
				// Empty cluster: reseed it to a random point so k clusters persist.
				copy(centroids[c], x[randIntN(n)])
				changed = true
				continue
			}
			for j := 0; j < d; j++ {
				centroids[c][j] = sums[c][j] / float64(counts[c])
			}
		}
		if !changed {
			break
		}
	}
	for _, cen := range centroids {
		for _, v := range cen {
			if !isFinite(v) {
				return nil, fmt.Errorf("input magnitudes overflow the computation")
			}
		}
	}
	return centroids, nil
}

// kmeansppSeed picks k initial centroids by the k-means++ rule: the first
// uniformly at random, each next with probability proportional to its squared
// distance from the nearest already-chosen centroid.
func kmeansppSeed(x [][]float64, k int) [][]float64 {
	n, d := len(x), len(x[0])
	centroids := make([][]float64, k)
	centroids[0] = append([]float64{}, x[randIntN(n)]...)
	dist := make([]float64, n)
	for i := range dist {
		dist[i] = euclid2(x[i], centroids[0])
	}
	for c := 1; c < k; c++ {
		sum := 0.0
		for _, dd := range dist {
			sum += dd
		}
		centroids[c] = make([]float64, d)
		if sum == 0 { // all points identical / already covered
			copy(centroids[c], x[randIntN(n)])
		} else {
			target := randFloat() * sum
			acc, pick := 0.0, n-1
			for i, dd := range dist {
				acc += dd
				if acc >= target {
					pick = i
					break
				}
			}
			copy(centroids[c], x[pick])
		}
		for i := range dist {
			if nd := euclid2(x[i], centroids[c]); nd < dist[i] {
				dist[i] = nd
			}
		}
	}
	return centroids
}
