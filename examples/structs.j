# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Introduce structs.
 * A struct ("record" in some traditions) gives a name to a fixed set of
 * typed fields. Use it whenever a multi-value bundle would otherwise
 * be a map indexed by string keys.
 * Defined once at the top level with `def struct Name { field as type, ... };`,
 * then constructed via `Name{ field: expr, ... }`. All fields must be
 * named at construction time (no implicit defaults at the literal level);
 * the uninitialised form `def x as Name;` gives every field its declared
 * zero.
 * Field access reads via `$v.field` and writes via `$v.field = expr;`.
 * Like lists and maps, structs are value types: assignment and parameter
 * binding copy.
 * @module structs
 */

use io;

/**
 * A point in the 2D integer plane.
 * @field x {int} the x coordinate
 * @field y {int} the y coordinate
 */
def struct Point { x as int, y as int };

# Constructed literal.
def origin as Point init Point{ x: 0, y: 0 };
def p as Point init Point{ x: 3, y: 4 };
io.printf("origin = %v\n", $origin);
io.printf("p = %v\n", $p);

# Field read.
io.printf("p.x = %d, p.y = %d\n", $p.x, $p.y);

# Field write.
$p.x = 30;
io.printf("after $p.x = 30: %v\n", $p);

# Value semantics: q is an independent copy of p.
def q as Point init $p;
$q.y = 99;
io.printf("p = %v, q = %v\n", $p, $q);

# Zero-init: every field gets its declared zero.
def z as Point;
io.printf("zero = %v\n", $z);

# Nested struct: a Line is two Points.
/**
 * A line segment between two points.
 * @field from {Point} the start point
 * @field to {Point} the end point
 */
def struct Line { from as Point, to as Point };
def L as Line init Line{ from: Point{ x: 0, y: 0 }, to: Point{ x: 10, y: 20 } };
io.printf("L = %v\n", $L);
io.printf("L.to.x = %d\n", $L.to.x);

# Chained field write reaches into the nested struct.
$L.from.x = 5;
io.printf("after $L.from.x = 5: %v\n", $L);
