// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package linalglib implements Jennifer's `linalg` library: linear algebra over
// the language's own value types, the companion to `stats`. Vectors are a
// `list of float` and matrices a `list of list of float` - idiomatic,
// value-semantic, and consistent with the rest of the language (a Go-backed
// opaque matrix handle is a future escape hatch if big-matrix throughput ever
// demands it). Algorithms are implemented directly (Gaussian / Gauss-Jordan
// elimination for solve / determinant / inverse), so the library is pure Go
// stdlib and TinyGo-clean - no `gonum`.
//
// Strict, like `math` / `stats`: a dimension mismatch, a non-rectangular matrix,
// a singular `inverse` / `solve`, or a non-finite result is a catchable error
// rather than a NaN.
package linalglib

import (
	"fmt"
	"math"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the Jennifer name programs `use` to enable these functions and
// the namespace prefix at call sites.
const LibraryName = "linalg"

// Install registers the linalg builtins.
func Install(in *interpreter.Interpreter) {
	// Vectors (list of float).
	in.RegisterNamespaced(LibraryName, "dot", dotFn)
	in.RegisterNamespaced(LibraryName, "cross", crossFn)
	in.RegisterNamespaced(LibraryName, "distance", distanceFn)
	in.RegisterNamespaced(LibraryName, "normalize", normalizeFn)
	// Elementwise / scalar ops, polymorphic over a vector or a matrix.
	in.RegisterNamespaced(LibraryName, "norm", normFn)
	in.RegisterNamespaced(LibraryName, "scale", scaleFn)
	in.RegisterNamespaced(LibraryName, "add", addFn)
	in.RegisterNamespaced(LibraryName, "sub", subFn)
	// Matrices (list of list of float).
	in.RegisterNamespaced(LibraryName, "matmul", matmulFn)
	in.RegisterNamespaced(LibraryName, "transpose", transposeFn)
	in.RegisterNamespaced(LibraryName, "trace", traceFn)
	in.RegisterNamespaced(LibraryName, "determinant", determinantFn)
	in.RegisterNamespaced(LibraryName, "inverse", inverseFn)
	in.RegisterNamespaced(LibraryName, "solve", solveFn)
	in.RegisterNamespaced(LibraryName, "identity", identityFn)
	in.RegisterNamespaced(LibraryName, "zeros", zerosFn)
	in.RegisterNamespaced(LibraryName, "shape", shapeFn)
}

// isMatrixArg reports whether v looks like a matrix (a list whose first element
// is itself a list) rather than a vector. An empty list reads as an empty
// vector. This is what lets `norm` / `scale` / `add` / `sub` / `matmul` accept
// either shape.
func isMatrixArg(v interpreter.Value) bool {
	return v.Kind == interpreter.KindList && len(v.List) > 0 && v.List[0].Kind == interpreter.KindList
}

// exceedsElementBudget reports whether an r-by-c value (c = 1 for a vector) would
// exceed MaxMatrixElements. Each dimension is checked before the product, so a
// dimension at or below the cap keeps r*c within int64 range (no overflow). This
// is the single bound every linalg entry point shares: the constructors check it
// before allocating from an integer argument, and the vector / matrix readers
// check it before an oversized input is rebuilt as a 272-byte-per-cell Value tree
// - which also transitively bounds the O(n^3) routines, since a bounded element
// count bounds the dimension.
func exceedsElementBudget(r, c int64) bool {
	max := int64(limits.MaxMatrixElements)
	return r > max || c > max || r*c > max
}

// ---- argument readers ----

// vector validates that args[i] is a non-empty `list of int|float` and returns
// its float64 view.
func vector(name string, args []interpreter.Value, i int) ([]float64, error) {
	v := args[i]
	if v.Kind != interpreter.KindList {
		return nil, fmt.Errorf("linalg.%s: argument must be a vector (list of float), got %s", name, v.Kind)
	}
	if exceedsElementBudget(int64(len(v.List)), 1) {
		return nil, fmt.Errorf("linalg.%s: vector too large (%d exceeds the %d-element limit)", name, len(v.List), limits.MaxMatrixElements)
	}
	out := make([]float64, len(v.List))
	for j, e := range v.List {
		f, ok := e.AsFloat()
		if !ok {
			return nil, fmt.Errorf("linalg.%s: vector element %d must be int or float, got %s", name, j, e.Kind)
		}
		out[j] = f
	}
	return out, nil
}

// matrix validates that args[i] is a rectangular, non-empty
// `list of list of int|float` and returns its float64 view.
func matrix(name string, args []interpreter.Value, i int) ([][]float64, error) {
	v := args[i]
	if v.Kind != interpreter.KindList {
		return nil, fmt.Errorf("linalg.%s: argument must be a matrix (list of list of float), got %s", name, v.Kind)
	}
	if len(v.List) == 0 {
		return nil, fmt.Errorf("linalg.%s: matrix has no rows", name)
	}
	out := make([][]float64, len(v.List))
	cols := -1
	for r, rowv := range v.List {
		if rowv.Kind != interpreter.KindList {
			return nil, fmt.Errorf("linalg.%s: matrix row %d must be a list, got %s", name, r, rowv.Kind)
		}
		if cols == -1 {
			cols = len(rowv.List)
			if cols == 0 {
				return nil, fmt.Errorf("linalg.%s: matrix rows are empty", name)
			}
			// Bound the element count once the shape is known, so an oversized
			// input matrix is a positioned, catchable error rather than a fatal
			// OOM when its 272-byte-per-cell Value tree is rebuilt.
			if exceedsElementBudget(int64(len(v.List)), int64(cols)) {
				return nil, fmt.Errorf("linalg.%s: matrix too large (%dx%d exceeds the %d-element limit)", name, len(v.List), cols, limits.MaxMatrixElements)
			}
		} else if len(rowv.List) != cols {
			return nil, fmt.Errorf("linalg.%s: matrix is not rectangular (row 0 has %d columns, row %d has %d)", name, cols, r, len(rowv.List))
		}
		row := make([]float64, cols)
		for c, e := range rowv.List {
			f, ok := e.AsFloat()
			if !ok {
				return nil, fmt.Errorf("linalg.%s: matrix element [%d][%d] must be int or float, got %s", name, r, c, e.Kind)
			}
			row[c] = f
		}
		out[r] = row
	}
	return out, nil
}

// requireSquare returns the dimension n of a square matrix, or an error.
func requireSquare(name string, m [][]float64) (int, error) {
	if len(m) != len(m[0]) {
		return 0, fmt.Errorf("linalg.%s: matrix must be square, got %dx%d", name, len(m), len(m[0]))
	}
	return len(m), nil
}

// ---- result builders (strict: non-finite -> catchable error) ----

func isFinite(r float64) bool { return !math.IsNaN(r) && !math.IsInf(r, 0) }

func nonFinite(name string) (interpreter.Value, error) {
	return interpreter.Null(), fmt.Errorf("linalg.%s: result is undefined or infinite (magnitudes overflow the computation)", name)
}

// scalarVal wraps a computed float, rejecting a non-finite result.
func scalarVal(name string, r float64) (interpreter.Value, error) {
	if !isFinite(r) {
		return nonFinite(name)
	}
	return interpreter.FloatVal(r), nil
}

// vectorVal builds a `list of float`, rejecting a non-finite element.
func vectorVal(name string, v []float64) (interpreter.Value, error) {
	cells := make([]interpreter.Value, len(v))
	for i, f := range v {
		if !isFinite(f) {
			return nonFinite(name)
		}
		cells[i] = interpreter.FloatVal(f)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeFloat), cells), nil
}

// matrixVal builds a `list of list of float`, rejecting a non-finite element.
func matrixVal(name string, m [][]float64) (interpreter.Value, error) {
	rowType := parser.ListType(parser.PrimitiveType(parser.TypeFloat))
	rows := make([]interpreter.Value, len(m))
	for i, r := range m {
		cells := make([]interpreter.Value, len(r))
		for j, f := range r {
			if !isFinite(f) {
				return nonFinite(name)
			}
			cells[j] = interpreter.FloatVal(f)
		}
		rows[i] = interpreter.ListVal(parser.PrimitiveType(parser.TypeFloat), cells)
	}
	return interpreter.ListVal(rowType, rows), nil
}

// ---- vectors ----

func dotFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("linalg.dot expects 2 arguments (vector, vector), got %d", len(args))
	}
	a, err := vector("dot", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	b, err := vector("dot", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(a) != len(b) {
		return interpreter.Null(), fmt.Errorf("linalg.dot: vectors must be the same length (%d vs %d)", len(a), len(b))
	}
	s := 0.0
	for i := range a {
		s += a[i] * b[i]
	}
	return scalarVal("dot", s)
}

// normFn returns the Euclidean (L2) norm of a vector, or the Frobenius norm of a
// matrix (the square root of the sum of squared entries).
func normFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("linalg.norm expects 1 argument (vector or matrix), got %d", len(args))
	}
	if isMatrixArg(args[0]) {
		m, err := matrix("norm", args, 0)
		if err != nil {
			return interpreter.Null(), err
		}
		s := 0.0
		for _, row := range m {
			s += sumSquares(row)
		}
		return scalarVal("norm", math.Sqrt(s))
	}
	v, err := vector("norm", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	return scalarVal("norm", math.Sqrt(sumSquares(v)))
}

// normalizeFn returns the unit vector v / norm(v). The zero vector has no
// direction and is a catchable error.
func normalizeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("linalg.normalize expects 1 argument (vector), got %d", len(args))
	}
	v, err := vector("normalize", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	n := math.Sqrt(sumSquares(v))
	// A non-finite norm means the magnitudes overflowed; dividing by +Inf would
	// silently yield a bogus all-zero "unit vector", so reject it (strict, and
	// consistent with linalg.norm) rather than return a wrong answer.
	if !isFinite(n) {
		return nonFinite("normalize")
	}
	if n == 0 {
		return interpreter.Null(), fmt.Errorf("linalg.normalize: cannot normalize the zero vector (it has no direction)")
	}
	out := make([]float64, len(v))
	for i := range v {
		out[i] = v[i] / n
	}
	return vectorVal("normalize", out)
}

func distanceFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("linalg.distance expects 2 arguments (vector, vector), got %d", len(args))
	}
	a, b, err := twoVectors("distance", args)
	if err != nil {
		return interpreter.Null(), err
	}
	s := 0.0
	for i := range a {
		d := a[i] - b[i]
		s += d * d
	}
	return scalarVal("distance", math.Sqrt(s))
}

// scaleFn multiplies every element of a vector or a matrix by a scalar.
func scaleFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("linalg.scale expects 2 arguments (vector or matrix, scalar), got %d", len(args))
	}
	s, ok := args[1].AsFloat()
	if !ok {
		return interpreter.Null(), fmt.Errorf("linalg.scale: scalar must be int or float, got %s", args[1].Kind)
	}
	if isMatrixArg(args[0]) {
		m, err := matrix("scale", args, 0)
		if err != nil {
			return interpreter.Null(), err
		}
		out := make([][]float64, len(m))
		for i, row := range m {
			out[i] = make([]float64, len(row))
			for j, f := range row {
				out[i][j] = f * s
			}
		}
		return matrixVal("scale", out)
	}
	v, err := vector("scale", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	out := make([]float64, len(v))
	for i := range v {
		out[i] = v[i] * s
	}
	return vectorVal("scale", out)
}

func addFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return elementwise("add", args, func(x, y float64) float64 { return x + y })
}

func subFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	return elementwise("sub", args, func(x, y float64) float64 { return x - y })
}

func crossFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("linalg.cross expects 2 arguments (vector, vector), got %d", len(args))
	}
	a, err := vector("cross", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	b, err := vector("cross", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(a) != 3 || len(b) != 3 {
		return interpreter.Null(), fmt.Errorf("linalg.cross: both vectors must have length 3 (got %d and %d)", len(a), len(b))
	}
	out := []float64{
		a[1]*b[2] - a[2]*b[1],
		a[2]*b[0] - a[0]*b[2],
		a[0]*b[1] - a[1]*b[0],
	}
	return vectorVal("cross", out)
}

// ---- matrices ----

// matmulFn is the general product. Both operands matrices -> the matrix product;
// a matrix times a vector -> the vector `M v` (v as a column); a vector times a
// matrix -> the vector `v M` (v as a row). Two vectors are ambiguous - `dot` is
// the tool for that.
func matmulFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("linalg.matmul expects 2 arguments (matrix or vector, matrix or vector), got %d", len(args))
	}
	leftMat := isMatrixArg(args[0])
	rightMat := isMatrixArg(args[1])
	switch {
	case leftMat && rightMat:
		a, err := matrix("matmul", args, 0)
		if err != nil {
			return interpreter.Null(), err
		}
		b, err := matrix("matmul", args, 1)
		if err != nil {
			return interpreter.Null(), err
		}
		// a is m x n, b is n x p; the inner dimensions must match.
		if len(a[0]) != len(b) {
			return interpreter.Null(), fmt.Errorf("linalg.matmul: inner dimensions must match (left is %dx%d, right is %dx%d)", len(a), len(a[0]), len(b), len(b[0]))
		}
		m, n, p := len(a), len(a[0]), len(b[0])
		out := make([][]float64, m)
		for i := 0; i < m; i++ {
			out[i] = make([]float64, p)
			for j := 0; j < p; j++ {
				s := 0.0
				for k := 0; k < n; k++ {
					s += a[i][k] * b[k][j]
				}
				out[i][j] = s
			}
		}
		return matrixVal("matmul", out)

	case leftMat: // matrix x vector -> vector (v as a column)
		a, err := matrix("matmul", args, 0)
		if err != nil {
			return interpreter.Null(), err
		}
		v, err := vector("matmul", args, 1)
		if err != nil {
			return interpreter.Null(), err
		}
		if len(a[0]) != len(v) {
			return interpreter.Null(), fmt.Errorf("linalg.matmul: matrix columns (%d) must match vector length (%d)", len(a[0]), len(v))
		}
		out := make([]float64, len(a))
		for i := range a {
			s := 0.0
			for k := range v {
				s += a[i][k] * v[k]
			}
			out[i] = s
		}
		return vectorVal("matmul", out)

	case rightMat: // vector x matrix -> vector (v as a row)
		v, err := vector("matmul", args, 0)
		if err != nil {
			return interpreter.Null(), err
		}
		b, err := matrix("matmul", args, 1)
		if err != nil {
			return interpreter.Null(), err
		}
		if len(v) != len(b) {
			return interpreter.Null(), fmt.Errorf("linalg.matmul: vector length (%d) must match matrix rows (%d)", len(v), len(b))
		}
		p := len(b[0])
		out := make([]float64, p)
		for j := 0; j < p; j++ {
			s := 0.0
			for k := range v {
				s += v[k] * b[k][j]
			}
			out[j] = s
		}
		return vectorVal("matmul", out)

	default: // vector x vector
		return interpreter.Null(), fmt.Errorf("linalg.matmul of two vectors is ambiguous; use linalg.dot for the dot product")
	}
}

func transposeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("linalg.transpose expects 1 argument (matrix), got %d", len(args))
	}
	a, err := matrix("transpose", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	rows, cols := len(a), len(a[0])
	out := make([][]float64, cols)
	for j := 0; j < cols; j++ {
		out[j] = make([]float64, rows)
		for i := 0; i < rows; i++ {
			out[j][i] = a[i][j]
		}
	}
	return matrixVal("transpose", out)
}

func determinantFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("linalg.determinant expects 1 argument (matrix), got %d", len(args))
	}
	a, err := matrix("determinant", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if _, err := requireSquare("determinant", a); err != nil {
		return interpreter.Null(), err
	}
	return scalarVal("determinant", determinant(a))
}

func inverseFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("linalg.inverse expects 1 argument (matrix), got %d", len(args))
	}
	a, err := matrix("inverse", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	if _, err := requireSquare("inverse", a); err != nil {
		return interpreter.Null(), err
	}
	inv, err := inverse(a)
	if err != nil {
		return interpreter.Null(), err
	}
	return matrixVal("inverse", inv)
}

func solveFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("linalg.solve expects 2 arguments (matrix, vector), got %d", len(args))
	}
	a, err := matrix("solve", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	n, err := requireSquare("solve", a)
	if err != nil {
		return interpreter.Null(), err
	}
	b, err := vector("solve", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	if len(b) != n {
		return interpreter.Null(), fmt.Errorf("linalg.solve: right-hand side length %d must match the %dx%d matrix", len(b), n, n)
	}
	x, err := solve(a, b)
	if err != nil {
		return interpreter.Null(), err
	}
	return vectorVal("solve", x)
}

func identityFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("linalg.identity expects 1 argument (n), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("linalg.identity: n must be int, got %s", args[0].Kind)
	}
	n := args[0].Int
	if n < 1 {
		return interpreter.Null(), fmt.Errorf("linalg.identity: n must be >= 1, got %d", n)
	}
	// Bound the allocation: identity(n) materialises n*n elements. Reject a
	// dimension that would size make() past the cap, turning an uncatchable OOM
	// into a positioned, catchable error.
	if exceedsElementBudget(n, n) {
		return interpreter.Null(), fmt.Errorf("linalg.identity: matrix too large (%dx%d exceeds the %d-element limit)", n, n, limits.MaxMatrixElements)
	}
	out := make([][]float64, n)
	for i := int64(0); i < n; i++ {
		out[i] = make([]float64, n)
		out[i][i] = 1
	}
	return matrixVal("identity", out)
}

// shapeFn returns [rows, cols] of a matrix as a `list of int`.
func shapeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("linalg.shape expects 1 argument (matrix), got %d", len(args))
	}
	a, err := matrix("shape", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeInt), []interpreter.Value{
		interpreter.IntVal(int64(len(a))),
		interpreter.IntVal(int64(len(a[0]))),
	}), nil
}

// traceFn returns the trace (sum of the main-diagonal entries) of a square matrix.
func traceFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("linalg.trace expects 1 argument (matrix), got %d", len(args))
	}
	m, err := matrix("trace", args, 0)
	if err != nil {
		return interpreter.Null(), err
	}
	n, err := requireSquare("trace", m)
	if err != nil {
		return interpreter.Null(), err
	}
	s := 0.0
	for i := 0; i < n; i++ {
		s += m[i][i]
	}
	return scalarVal("trace", s)
}

// zerosFn builds a `rows x cols` matrix of zeros (the companion to identity).
func zerosFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("linalg.zeros expects 2 arguments (rows, cols), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindInt || args[1].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("linalg.zeros: rows and cols must be int")
	}
	rows, cols := args[0].Int, args[1].Int
	if rows < 1 || cols < 1 {
		return interpreter.Null(), fmt.Errorf("linalg.zeros: rows and cols must be >= 1, got %dx%d", rows, cols)
	}
	// Bound the allocation the same way identity does: reject dimensions that
	// would size make() past the cap (a thin 1 x N / N x 1 shape too).
	if exceedsElementBudget(rows, cols) {
		return interpreter.Null(), fmt.Errorf("linalg.zeros: matrix too large (%dx%d exceeds the %d-element limit)", rows, cols, limits.MaxMatrixElements)
	}
	out := make([][]float64, rows)
	for i := range out {
		out[i] = make([]float64, cols)
	}
	return matrixVal("zeros", out)
}

// ---- shared helpers ----

func sumSquares(v []float64) float64 {
	s := 0.0
	for _, f := range v {
		s += f * f
	}
	return s
}

func twoVectors(name string, args []interpreter.Value) ([]float64, []float64, error) {
	a, err := vector(name, args, 0)
	if err != nil {
		return nil, nil, err
	}
	b, err := vector(name, args, 1)
	if err != nil {
		return nil, nil, err
	}
	if len(a) != len(b) {
		return nil, nil, fmt.Errorf("linalg.%s: vectors must be the same length (%d vs %d)", name, len(a), len(b))
	}
	return a, b, nil
}

// elementwise applies op pairwise over two vectors or two matrices (both
// operands must be the same shape category and the same dimensions).
func elementwise(name string, args []interpreter.Value, op func(x, y float64) float64) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("linalg.%s expects 2 arguments (both vectors or both matrices), got %d", name, len(args))
	}
	if isMatrixArg(args[0]) || isMatrixArg(args[1]) {
		a, err := matrix(name, args, 0)
		if err != nil {
			return interpreter.Null(), err
		}
		b, err := matrix(name, args, 1)
		if err != nil {
			return interpreter.Null(), err
		}
		if len(a) != len(b) || len(a[0]) != len(b[0]) {
			return interpreter.Null(), fmt.Errorf("linalg.%s: matrices must have the same shape (%dx%d vs %dx%d)", name, len(a), len(a[0]), len(b), len(b[0]))
		}
		out := make([][]float64, len(a))
		for i := range a {
			out[i] = make([]float64, len(a[i]))
			for j := range a[i] {
				out[i][j] = op(a[i][j], b[i][j])
			}
		}
		return matrixVal(name, out)
	}
	a, b, err := twoVectors(name, args)
	if err != nil {
		return interpreter.Null(), err
	}
	out := make([]float64, len(a))
	for i := range a {
		out[i] = op(a[i], b[i])
	}
	return vectorVal(name, out)
}

// copyMatrix returns a deep copy of m (the elimination routines mutate their
// working copy, never the caller's input).
func copyMatrix(m [][]float64) [][]float64 {
	out := make([][]float64, len(m))
	for i, r := range m {
		out[i] = append([]float64(nil), r...)
	}
	return out
}

// determinant computes det(a) by Gaussian elimination with partial pivoting. A
// singular matrix returns 0 (a valid, finite result - only inverse / solve treat
// singularity as an error).
func determinant(a [][]float64) float64 {
	m := copyMatrix(a)
	n := len(m)
	det := 1.0
	for i := 0; i < n; i++ {
		p := i
		for r := i + 1; r < n; r++ {
			if math.Abs(m[r][i]) > math.Abs(m[p][i]) {
				p = r
			}
		}
		if m[p][i] == 0 {
			return 0
		}
		if p != i {
			m[i], m[p] = m[p], m[i]
			det = -det
		}
		det *= m[i][i]
		for r := i + 1; r < n; r++ {
			f := m[r][i] / m[i][i]
			for c := i; c < n; c++ {
				m[r][c] -= f * m[i][c]
			}
		}
	}
	return det
}

// errSingular is the sentinel a caller turns into a positioned error.
var errSingular = fmt.Errorf("matrix is singular (not invertible)")

// solve solves a x = b by Gauss-Jordan elimination with partial pivoting.
func solve(a [][]float64, b []float64) ([]float64, error) {
	n := len(a)
	// Augmented [a | b].
	m := make([][]float64, n)
	for i := 0; i < n; i++ {
		m[i] = append(append([]float64(nil), a[i]...), b[i])
	}
	for i := 0; i < n; i++ {
		p := i
		for r := i + 1; r < n; r++ {
			if math.Abs(m[r][i]) > math.Abs(m[p][i]) {
				p = r
			}
		}
		if m[p][i] == 0 {
			return nil, fmt.Errorf("linalg.solve: %v", errSingular)
		}
		m[i], m[p] = m[p], m[i]
		for r := 0; r < n; r++ {
			if r == i {
				continue
			}
			f := m[r][i] / m[i][i]
			for c := i; c <= n; c++ {
				m[r][c] -= f * m[i][c]
			}
		}
	}
	x := make([]float64, n)
	for i := 0; i < n; i++ {
		x[i] = m[i][n] / m[i][i]
	}
	return x, nil
}

// inverse computes a^-1 by Gauss-Jordan elimination on the augmented [a | I].
func inverse(a [][]float64) ([][]float64, error) {
	n := len(a)
	m := make([][]float64, n)
	for i := 0; i < n; i++ {
		m[i] = make([]float64, 2*n)
		copy(m[i], a[i])
		m[i][n+i] = 1
	}
	for i := 0; i < n; i++ {
		p := i
		for r := i + 1; r < n; r++ {
			if math.Abs(m[r][i]) > math.Abs(m[p][i]) {
				p = r
			}
		}
		if m[p][i] == 0 {
			return nil, fmt.Errorf("linalg.inverse: %v", errSingular)
		}
		m[i], m[p] = m[p], m[i]
		piv := m[i][i]
		for c := 0; c < 2*n; c++ {
			m[i][c] /= piv
		}
		for r := 0; r < n; r++ {
			if r == i {
				continue
			}
			f := m[r][i]
			for c := 0; c < 2*n; c++ {
				m[r][c] -= f * m[i][c]
			}
		}
	}
	inv := make([][]float64, n)
	for i := 0; i < n; i++ {
		inv[i] = append([]float64(nil), m[i][n:]...)
	}
	return inv, nil
}
