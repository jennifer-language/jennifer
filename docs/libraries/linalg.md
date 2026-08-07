# `linalg` - linear algebra

The `linalg` library does linear algebra over Jennifer's own value types - the
companion to [`stats`](stats.md). **Vectors** are a `list of float` and
**matrices** a `list of list of float`, so the data is idiomatic,
value-semantic, and interchangeable with the rest of the language (no opaque
handle to construct or unwrap). It is pure-value and dependency-free (Go stdlib
only, algorithms implemented directly - no `gonum`), so both binaries build it.

```jennifer
use linalg;
def a as list of list of float init [[4.0, 7.0], [2.0, 6.0]];
linalg.determinant($a);              # 10.0
linalg.inverse($a);                  # [[0.6, -0.7], [-0.2, 0.4]]
linalg.solve([[1.0, 1.0], [1.0, -1.0]], [3.0, 1.0]);  # [2.0, 1.0]  (x+y=3, x-y=1)
linalg.dot([1.0, 2.0, 3.0], [4.0, 5.0, 6.0]);         # 32.0
```

## Vectors (`list of float`)

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `linalg.dot(a, b)`      | `float`        | Dot product. Vectors must be the same length. |
| `linalg.distance(a, b)` | `float`        | Euclidean distance `norm(a - b)`. Same length.  |
| `linalg.cross(a, b)`    | `list of float`| 3-D cross product. Both vectors must have length 3. |
| `linalg.normalize(v)`   | `list of float`| The unit vector `v / norm(v)`. The zero vector errors. |

## Matrices (`list of list of float`)

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `linalg.transpose(m)`     | `list of list of float`| Rows become columns.                          |
| `linalg.trace(m)`         | `float`                | Sum of the main-diagonal entries. Square only. |
| `linalg.determinant(m)`   | `float`                | Determinant via Gaussian elimination. Square only; a singular matrix is `0`. |
| `linalg.inverse(m)`       | `list of list of float`| Inverse via Gauss-Jordan. Square, non-singular - a singular matrix errors. |
| `linalg.solve(a, b)`      | `list of float`        | Solve `a x = b` for `x`. `a` square `n x n`, `b` length `n`; singular errors. |
| `linalg.identity(n)`      | `list of list of float`| The `n x n` identity matrix (`n >= 1`).       |
| `linalg.zeros(rows, cols)`| `list of list of float`| A `rows x cols` matrix of zeros (`rows`, `cols` `>= 1`). |
| `linalg.shape(m)`         | `list of int`          | `[rows, cols]` of a matrix.                    |

## Vector-or-matrix operations

Four operations are **polymorphic** - each takes either a vector or a matrix and
returns the matching shape:

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `linalg.norm(x)`      | `float`        | Euclidean (L2) norm of a **vector**, or the **Frobenius** norm of a matrix (`sqrt` of the sum of squared entries). |
| `linalg.scale(x, s)`  | same shape as `x` | Every element of a vector or matrix times the scalar `s`. |
| `linalg.add(a, b)`    | same shape as the inputs | Element-wise sum of two vectors or two matrices (same shape). |
| `linalg.sub(a, b)`    | same shape as the inputs | Element-wise difference of two vectors or two matrices. |

`linalg.matmul(a, b)` is the general product, dispatched on the operand shapes:

| Operands | Result | Meaning |
| -------- | ------ | ------- |
| matrix `x` matrix | `list of list of float` | Matrix product. The inner dimensions must match (`m x n` by `n x p`). |
| matrix `x` vector | `list of float`         | `M v`, the vector `v` treated as a column. Matrix columns must match the vector length. |
| vector `x` matrix | `list of float`         | `v M`, the vector `v` treated as a row. The vector length must match the matrix rows. |
| vector `x` vector | *error* | Ambiguous - use `linalg.dot` for the dot product. |

A vector or matrix element may be an `int` or a `float` (an `int` promotes to
`float`). A matrix must be **rectangular** - every row the same length - and
non-empty; anything else is a positioned error.

## Matrices are a plain nested list

There is no dedicated matrix type: a matrix is exactly a `list of list of float`,
built and read with the language's own list syntax and value semantics. That
keeps the data first-class (pass it to `stats`, iterate it, slice it) at the cost
of the per-element boxing a Go-backed dense matrix would avoid - a fine trade at
the sizes a scripting language handles. A Go-backed opaque matrix handle is the
noted future escape hatch if big-matrix throughput ever demands it.

## Strictness

Following `math` / `stats`, `linalg` never returns a `NaN` or `Inf`: a shape or
dimension problem and a numerically undefined result are both catchable errors.

- A **dimension mismatch** (`dot` / `add` / `sub` / `distance` on unequal lengths,
  `matmul` on non-conforming inner dimensions, `solve` on a right-hand side that
  does not match the matrix), a **non-square** matrix where one is required
  (`determinant` / `inverse` / `solve`), a **non-rectangular** matrix, a `cross`
  on a non-3 vector, and an empty vector or matrix all raise a positioned error.
- A **singular** matrix has no inverse and no unique solution, so `inverse` and
  `solve` raise an error (`determinant` returns `0`, its correct value).
- When the input *magnitudes* overflow the computation - values near the float64
  ceiling (`~1.8e308`) can push an intermediate product or sum to `+/-Inf` (and
  thence `NaN`) - the non-finite result is rejected rather than returned, so a
  `NaN` never escapes into the type system.
- A vector or matrix is bounded to a fixed **element count** (large but far below
  the point where materialising it would exhaust memory): a constructor called
  with an oversized dimension (`identity(100000)`, `zeros(100000, 100000)`) and an
  operation handed an oversized input both raise a positioned, catchable error
  rather than a fatal out-of-memory crash. This ceiling also keeps the `O(n^3)`
  routines (`matmul` / `inverse` / `solve`) bounded to a small, finite amount of
  work.

```jennifer
use linalg;
try {
    linalg.inverse([[1.0, 2.0], [2.0, 4.0]]);   # rows are linearly dependent
} catch (e) {
    io.printf("%v\n", $e.message);               # linalg.inverse: matrix is singular (not invertible)
}
```

## See also

[`stats`](stats.md) for descriptive statistics over a `list`; `linalg` is its
linear-algebra companion. Further ML primitives atop the two are on the
[horizon](../horizon.md).
