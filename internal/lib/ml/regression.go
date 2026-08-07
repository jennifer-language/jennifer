// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package mllib

import (
	"fmt"
	"math"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// linearModel is ordinary-least-squares / ridge linear regression: a coefficient
// per feature plus an intercept.
type linearModel struct {
	coef      []float64 // one per feature
	intercept float64
	nf        int
}

func (m *linearModel) numFeatures() int { return m.nf }
func (m *linearModel) intLabels() bool  { return false }
func (m *linearModel) predictOne(x []float64) float64 {
	s := m.intercept
	for i, c := range m.coef {
		s += c * x[i]
	}
	return s
}

// solveSPD solves the symmetric system a*x = b by Gauss-Jordan elimination with
// partial pivoting, returning false if singular. (The normal-equations matrix is
// symmetric positive-(semi)definite; this is the same algorithm linalg.solve
// runs.)
func solveSPD(a [][]float64, b []float64) ([]float64, bool) {
	n := len(b)
	m := make([][]float64, n)
	for i := range m {
		m[i] = append(append([]float64{}, a[i]...), b[i])
	}
	for col := 0; col < n; col++ {
		piv := col
		for r := col + 1; r < n; r++ {
			if math.Abs(m[r][col]) > math.Abs(m[piv][col]) {
				piv = r
			}
		}
		if math.Abs(m[piv][col]) < 1e-300 {
			return nil, false
		}
		m[col], m[piv] = m[piv], m[col]
		for r := 0; r < n; r++ {
			if r == col {
				continue
			}
			f := m[r][col] / m[col][col]
			for c := col; c <= n; c++ {
				m[r][c] -= f * m[col][c]
			}
		}
	}
	x := make([]float64, n)
	for i := 0; i < n; i++ {
		x[i] = m[i][n] / m[i][i]
	}
	return x, true
}

// fitLinear solves the (ridge-)regularized normal equations for a design matrix
// with a leading intercept column. alpha == 0 is plain OLS; alpha > 0 adds
// alpha to the diagonal of every coefficient except the intercept (standard
// ridge, which does not penalize the bias).
func fitLinear(x [][]float64, y []float64, alpha float64) (*linearModel, error) {
	nrow := len(x)
	k := len(x[0]) // features
	p := k + 1     // + intercept
	if nrow <= p {
		return nil, fmt.Errorf("need more rows than coefficients (%d rows, %d features + intercept)", nrow, k)
	}
	dtd := make([][]float64, p)
	dty := make([]float64, p)
	for i := range dtd {
		dtd[i] = make([]float64, p)
	}
	design := func(i, j int) float64 {
		if j == 0 {
			return 1
		}
		return x[i][j-1]
	}
	for r := 0; r < nrow; r++ {
		for a := 0; a < p; a++ {
			da := design(r, a)
			dty[a] += da * y[r]
			for b := 0; b < p; b++ {
				dtd[a][b] += da * design(r, b)
			}
		}
	}
	if alpha > 0 {
		for j := 1; j < p; j++ { // skip the intercept term
			dtd[j][j] += alpha
		}
	}
	beta, ok := solveSPD(dtd, dty)
	if !ok {
		return nil, fmt.Errorf("the design is singular (collinear features?)")
	}
	for _, b := range beta {
		if !isFinite(b) {
			return nil, fmt.Errorf("input magnitudes overflow the computation")
		}
	}
	return &linearModel{coef: beta[1:], intercept: beta[0], nf: k}, nil
}

func (r *registry) linearRegressionFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("ml.linearRegression expects 2 arguments (X, y), got %d", len(args))
	}
	x, y, err := fitData("linearRegression", args)
	if err != nil {
		return interpreter.Null(), err
	}
	m, err := fitLinear(x, y, 0)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("ml.linearRegression: %v", err)
	}
	return r.store(m), nil
}

func (r *registry) ridgeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("ml.ridge expects 3 arguments (X, y, alpha), got %d", len(args))
	}
	x, y, err := fitData("ridge", args)
	if err != nil {
		return interpreter.Null(), err
	}
	alpha, ok := args[2].AsFloat()
	if !ok {
		return interpreter.Null(), fmt.Errorf("ml.ridge: alpha must be int or float, got %s", args[2].Kind)
	}
	if alpha < 0 {
		return interpreter.Null(), fmt.Errorf("ml.ridge: alpha must be non-negative, got %s", interpreter.DisplayFloat(alpha))
	}
	m, err := fitLinear(x, y, alpha)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("ml.ridge: %v", err)
	}
	return r.store(m), nil
}
