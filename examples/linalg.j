# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

# Linear algebra over Jennifer's own value types with the `linalg` library.
# Vectors are a `list of float`; matrices a `list of list of float`.

use io;
use linalg;

# --- vectors ---
def u as list of float init [1.0, 2.0, 2.0];
def v as list of float init [3.0, 0.0, 4.0];

io.printf("dot(u, v)      = %v\n", linalg.dot($u, $v));
io.printf("norm(u)        = %v\n", linalg.norm($u));
io.printf("normalize(u)   = %v\n", linalg.normalize($u));
io.printf("distance(u, v) = %v\n", linalg.distance($u, $v));
io.printf("u + v          = %v\n", linalg.add($u, $v));
io.printf("u - v          = %v\n", linalg.sub($u, $v));
io.printf("2 * u          = %v\n", linalg.scale($u, 2.0));
io.printf("x cross y      = %v\n", linalg.cross([1.0, 0.0, 0.0], [0.0, 1.0, 0.0]));

# --- matrices ---
def a as list of list of float init [[4.0, 7.0], [2.0, 6.0]];
def b as list of list of float init [[1.0, 0.0], [1.0, 1.0]];

io.printf("shape(a)       = %v\n", linalg.shape($a));
io.printf("transpose(a)   = %v\n", linalg.transpose($a));
io.printf("trace(a)       = %v\n", linalg.trace($a));
io.printf("det(a)         = %v\n", linalg.determinant($a));
io.printf("a + b          = %v\n", linalg.add($a, $b));
io.printf("2 * a          = %v\n", linalg.scale($a, 2.0));
io.printf("a * b          = %v\n", linalg.matmul($a, $b));
io.printf("a * [1, 1]     = %v\n", linalg.matmul($a, [1.0, 1.0]));
io.printf("frobenius(a)   = %v\n", linalg.norm($a));
io.printf("zeros(2, 3)    = %v\n", linalg.zeros(2, 3));
io.printf("inverse(a)     = %v\n", linalg.inverse($a));

# Solve the linear system  x + y = 3,  x - y = 1  ->  x = 2, y = 1.
def coeffs as list of list of float init [[1.0, 1.0], [1.0, -1.0]];
io.printf("solve          = %v\n", linalg.solve($coeffs, [3.0, 1.0]));

# The strict stance: a singular matrix has no inverse - a catchable error.
try {
    linalg.inverse([[1.0, 2.0], [2.0, 4.0]]);
} catch (e) {
    io.printf("singular       = %v\n", $e.message);
}
