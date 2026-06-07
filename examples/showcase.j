// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>
//
// showcase.j - exercises every Jennifer language feature and every
// standard-library function that ships at M5. Used as a golden
// integration test by cmd/jennifer/examples_test.go.
//
// JENNIFER_VERSION (from the auto-loaded `core` library) is
// intentionally NOT printed - its value depends on git state and would
// make the golden file non-deterministic. We exercise the constant via
// typeOf() instead.

use io;
use convert;
use math;
use strings;
import "showcase/helpers.j";

// --- Constants: simple, underscored, library-provided ---
def const MAX as int init 5;
def const MAX_RETRIES as int init 3;
def const HTTP_OK as int init 200;
def const PI_APPROX as float init 3.14;

// --- Variable definitions: explicit init and zero-value ---
def x as int init 10;
def y as float init 2.5;
def name as string init "Jennifer";
def flag as bool init true;
def nothing as null;
def empty as int;

printf("=== variables ===\n");
printf("x=%d y=%f name=%s flag=%t nothing=%v empty=%d\n", $x, $y, $name, $flag, $nothing, $empty);

// --- Arithmetic, including the Python-3 / vs div distinction ---
printf("=== arithmetic ===\n");
printf("10 + 5 = %d\n", 10 + 5);
printf("10 - 5 = %d\n", 10 - 5);
printf("10 * 5 = %d\n", 10 * 5);
printf("10 / 4 = %f\n", 10 / 4);
printf("10 div 4 = %d\n", 10 div 4);
printf("10 %% 3 = %d\n", 10 % 3);
printf("-x = %d\n", -$x);
printf("int + float = %f\n", 3 + 0.5);

// --- Comparison and logical operators ---
printf("=== comparison and logic ===\n");
printf("5 < 10  = %t\n", 5 < 10);
printf("5 == 5  = %t\n", 5 == 5);
printf("5 >= 5  = %t\n", 5 >= 5);
printf("true and false = %t\n", true and false);
printf("true or  false = %t\n", true or false);
printf("not false      = %t\n", not false);

// --- String concatenation ---
printf("=== string concat ===\n");
def greeting as string init "Hello, " + $name + "!";
printf("%s\n", $greeting);

// --- Control flow ---
printf("=== if / elseif / else ===\n");
if ($x > 0) {
    printf("x is positive\n");
} elseif ($x == 0) {
    printf("x is zero\n");
} else {
    printf("x is negative\n");
}

printf("=== while ===\n");
def i as int init 1;
while ($i <= 3) {
    printf("  while i=%d\n", $i);
    $i = $i + 1;
}

printf("=== for ===\n");
for (def j as int init 0; $j < 3; $j = $j + 1) {
    printf("  for j=%d\n", $j);
}

// --- Methods + recursion + file-imported helper ---
printf("=== methods ===\n");
printf("fact(5) = %d\n", fact(5));
printf("%s\n", greet($name));

// --- io.sprintf ---
printf("=== sprintf ===\n");
def line as string init sprintf("[%d:%s]", 42, "hi");
printf("%s\n", $line);

// --- convert library ---
printf("=== convert ===\n");
printf("int(3.7)       = %d\n", int(3.7));
printf("int(\"42\")      = %d\n", int("42"));
printf("float(5)       = %f\n", float(5));
printf("string(42)     = %s\n", string(42));
printf("bool(1)        = %t\n", bool(1));
printf("bool(\"true\")   = %t\n", bool("true"));
printf("typeOf(3.14)   = %s\n", typeOf(3.14));
printf("typeOf(\"x\")    = %s\n", typeOf("x"));
printf("typeOf(true)   = %s\n", typeOf(true));
printf("typeOf(null)   = %s\n", typeOf(null));

// --- math library ---
printf("=== math ===\n");
printf("abs(-7)        = %d\n", abs(-7));
printf("abs(-3.5)      = %f\n", abs(-3.5));
printf("min(3, 7)      = %d\n", min(3, 7));
printf("max(3.5, 2.5)  = %f\n", max(3.5, 2.5));
printf("sqrt(16)       = %f\n", sqrt(16));
printf("pow(2, 10)     = %f\n", pow(2, 10));
printf("floor(3.7)     = %d\n", floor(3.7));
printf("ceil(3.2)      = %d\n", ceil(3.2));
printf("round(2.5)     = %d\n", round(2.5));
printf("round(-2.5)    = %d\n", round(-2.5));

printf("=== math constants ===\n");
printf("typeOf(PI)     = %s\n", typeOf(PI));
printf("typeOf(E)      = %s\n", typeOf(E));

// --- strings library ---
printf("=== strings ===\n");
def sample as string init "Hello, World";
printf("len           = %d\n", len($sample));
printf("upper         = %s\n", upper($sample));
printf("lower         = %s\n", lower($sample));
printf("contains      = %t\n", contains($sample, "World"));
printf("startsWith    = %t\n", startsWith($sample, "Hello"));
printf("endsWith      = %t\n", endsWith($sample, "World"));
printf("indexOf       = %d\n", indexOf($sample, "World"));
printf("indexOf miss  = %d\n", indexOf($sample, "zzz"));
printf("trim          = [%s]\n", trim("  ab  "));
printf("trimLeft      = [%s]\n", trimLeft("  ab  "));
printf("trimRight     = [%s]\n", trimRight("  ab  "));
printf("replace       = %s\n", replace($sample, "World", "Jennifer"));
printf("repeat        = %s\n", repeat("ab", 3));
printf("substring 0..5 = %s\n", substring($sample, 0, 5));
printf("substring 7..  = %s\n", substring($sample, 7));

// --- core (auto-loaded): prove JENNIFER_VERSION is wired without baking its value into the golden ---
printf("=== core ===\n");
printf("typeOf(JENNIFER_VERSION) = %s\n", typeOf(JENNIFER_VERSION));

// --- Constants in expressions ---
printf("=== constants ===\n");
printf("MAX=%d MAX_RETRIES=%d HTTP_OK=%d PI_APPROX=%f\n", MAX, MAX_RETRIES, HTTP_OK, PI_APPROX);

printf("=== done ===\n");
