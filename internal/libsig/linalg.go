// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package libsig

func init() {
	// Vectors and matrices accept int and float elements mixed, so arguments
	// are `list of any`; a matrix is a list of rows.
	add("linalg", `
dot(list of any, list of any) float
cross(list of any, list of any) list of float
distance(list of any, list of any) float
normalize(list of any) list of float
norm(list of any) float
scale(list of any, int|float) list of float|list of list of float
add(list of any, list of any) list of float|list of list of float
sub(list of any, list of any) list of float|list of list of float
matmul(list of any, list of any) list of float|list of list of float
transpose(list of any) list of list of float
trace(list of any) float
determinant(list of any) float
inverse(list of any) list of list of float
solve(list of any, list of any) list of float
identity(int) list of list of float
zeros(int, int) list of list of float
shape(list of any) list of int
`)
}
