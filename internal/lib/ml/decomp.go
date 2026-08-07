// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package mllib

import (
	"fmt"
	"math"
	"sort"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// pcaModel projects centered features onto the top principal components.
type pcaModel struct {
	mean       []float64
	components [][]float64 // [component][feature]
	nf         int
}

func (m *pcaModel) numFeatures() int { return m.nf }
func (m *pcaModel) transformOne(x []float64) []float64 {
	out := make([]float64, len(m.components))
	for c, comp := range m.components {
		s := 0.0
		for j := range comp {
			s += comp[j] * (x[j] - m.mean[j])
		}
		out[c] = s
	}
	return out
}

func (r *registry) pcaFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("ml.pca expects 2 arguments (X, nComponents), got %d", len(args))
	}
	x, err := matrix("pca", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("ml.pca: nComponents must be an int")
	}
	nc := int(args[1].Int)
	d := len(x[0])
	if nc < 1 || nc > d {
		return interpreter.Null(), fmt.Errorf("ml.pca: nComponents must be in [1, features] (got %d, features=%d)", nc, d)
	}
	// Center, build the covariance matrix, eigendecompose (Jacobi), take the top.
	mu := make([]float64, d)
	for _, row := range x {
		for j := 0; j < d; j++ {
			mu[j] += row[j]
		}
	}
	for j := 0; j < d; j++ {
		mu[j] /= float64(len(x))
	}
	cov := make([][]float64, d)
	for i := range cov {
		cov[i] = make([]float64, d)
	}
	for _, row := range x {
		for i := 0; i < d; i++ {
			for j := 0; j < d; j++ {
				cov[i][j] += (row[i] - mu[i]) * (row[j] - mu[j])
			}
		}
	}
	den := float64(len(x) - 1)
	if den <= 0 {
		den = 1
	}
	for i := 0; i < d; i++ {
		for j := 0; j < d; j++ {
			cov[i][j] /= den
		}
	}
	vals, vecs := jacobiEigen(cov)
	// Sort components by descending eigenvalue.
	idx := make([]int, d)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return vals[idx[a]] > vals[idx[b]] })
	comps := make([][]float64, nc)
	for c := 0; c < nc; c++ {
		col := idx[c]
		comps[c] = make([]float64, d)
		for i := 0; i < d; i++ {
			comps[c][i] = vecs[i][col]
			if !isFinite(comps[c][i]) {
				return interpreter.Null(), fmt.Errorf("ml.pca: eigendecomposition failed (non-finite result)")
			}
		}
	}
	return r.store(&pcaModel{mean: mu, components: comps, nf: d}), nil
}

// jacobiEigen returns the eigenvalues and eigenvectors of a symmetric matrix by
// the cyclic Jacobi rotation method. vecs[i][j] is component i of eigenvector j.
func jacobiEigen(a [][]float64) ([]float64, [][]float64) {
	n := len(a)
	// Work on a copy; v accumulates the rotations (the eigenvectors).
	m := make([][]float64, n)
	v := make([][]float64, n)
	for i := range m {
		m[i] = append([]float64{}, a[i]...)
		v[i] = make([]float64, n)
		v[i][i] = 1
	}
	for sweep := 0; sweep < 100; sweep++ {
		off := 0.0
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				off += m[i][j] * m[i][j]
			}
		}
		if off < 1e-30 {
			break
		}
		for p := 0; p < n; p++ {
			for q := p + 1; q < n; q++ {
				if math.Abs(m[p][q]) < 1e-300 {
					continue
				}
				theta := (m[q][q] - m[p][p]) / (2 * m[p][q])
				t := math.Copysign(1, theta) / (math.Abs(theta) + math.Sqrt(theta*theta+1))
				c := 1 / math.Sqrt(t*t+1)
				s := t * c
				for i := 0; i < n; i++ {
					mip, miq := m[i][p], m[i][q]
					m[i][p] = c*mip - s*miq
					m[i][q] = s*mip + c*miq
				}
				for i := 0; i < n; i++ {
					mpi, mqi := m[p][i], m[q][i]
					m[p][i] = c*mpi - s*mqi
					m[q][i] = s*mpi + c*mqi
				}
				for i := 0; i < n; i++ {
					vip, viq := v[i][p], v[i][q]
					v[i][p] = c*vip - s*viq
					v[i][q] = s*vip + c*viq
				}
			}
		}
	}
	vals := make([]float64, n)
	for i := 0; i < n; i++ {
		vals[i] = m[i][i]
	}
	return vals, v
}
