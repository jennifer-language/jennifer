# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

# First-class function values: a bare method name is a value you can store,
# pass, and call. This drives the higher-order `lists` helpers.

use io;
use lists;

func double(n as int) { return $n * 2; }
func isEven(n as int) { return $n % 2 == 0; }
func add(a as int, b as int) { return $a + $b; }
func neg(n as int) { return 0 - $n; }

# A bare method name in expression position is the function value.
def f as func init double;
io.printf("call through a variable: %d\n", $f(21));

# Pass a function value to a method and call it inside.
func applyTo(fn as func, x as int) { return $fn($x); }
io.printf("passed as an argument: %d\n", applyTo(double, 20));

# The higher-order layer over `lists`, each taking a `func`.
def xs as list of int init [1, 2, 3, 4, 5];
io.printf("map double:  %v\n", lists.map($xs, double));
io.printf("filter even: %v\n", lists.filter($xs, isEven));
io.printf("reduce sum:  %d\n", lists.reduce($xs, add, 0));
io.printf("find even:   %d\n", lists.find($xs, isEven));
io.printf("any/all even: %t %t\n", lists.any($xs, isEven), lists.all($xs, isEven));
io.printf("sortBy desc: %v\n", lists.sortBy($xs, neg));

# Sort a list of structs by a field, via a one-line key accessor.
def struct Person { name as string, age as int };
func ageOf(p as Person) { return $p.age; }
def people as list of Person init [
    Person{name: "cy", age: 30},
    Person{name: "al", age: 25},
    Person{name: "bo", age: 40}
];
def byAge as list of Person init lists.sortBy($people, ageOf);
for (def p in $byAge) {
    io.printf("%s(%d) ", $p.name, $p.age);
}
io.printf("\n");
