# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# showcase.j - exercises every Jennifer language feature and every
# standard-library function that ships through M8. Used as a golden
# integration test by cmd/jennifer/examples_test.go.
#
# JENNIFER_VERSION (from the auto-loaded `core` library) is
# intentionally NOT printed - its value depends on git state and would
# make the golden file non-deterministic. We exercise the constant via
# typeOf() instead.

use io;
use convert;
use math;
use strings;
use lists;
use maps;
use os;
import "showcase/helpers.j";

# --- Constants: simple, underscored, library-provided ---
def const MAX as int init 5;
def const MAX_RETRIES as int init 3;
def const HTTP_OK as int init 200;
def const PI_APPROX as float init 3.14;

# --- Variable definitions: explicit init and zero-value ---
def x as int init 10;
def y as float init 2.5;
def name as string init "Jennifer";
def flag as bool init true;
def nothing as null;
def empty as int;

printf("=== variables ===\n");
printf("x=%d y=%f name=%s flag=%t nothing=%v empty=%d\n", $x, $y, $name, $flag, $nothing, $empty);

# --- Arithmetic, including the Python-3 / vs // distinction ---
printf("=== arithmetic ===\n");
printf("10 + 5 = %d\n", 10 + 5);
printf("10 - 5 = %d\n", 10 - 5);
printf("10 * 5 = %d\n", 10 * 5);
printf("10 / 4 = %f\n", 10 / 4);
printf("10 // 4 = %d\n", 10 // 4);
printf("10 %% 3 = %d\n", 10 % 3);
printf("-x = %d\n", -$x);
printf("int + float = %f\n", 3 + 0.5);

# --- Comparison and logical operators ---
printf("=== comparison and logic ===\n");
printf("5 < 10  = %t\n", 5 < 10);
printf("5 == 5  = %t\n", 5 == 5);
printf("5 >= 5  = %t\n", 5 >= 5);
printf("true and false = %t\n", true and false);
printf("true or  false = %t\n", true or false);
printf("not false      = %t\n", not false);

# --- String concatenation ---
printf("=== string concat ===\n");
def greeting as string init "Hello, " + $name + "!";
printf("%s\n", $greeting);

# --- Control flow ---
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

# --- Methods + recursion + file-imported helper ---
printf("=== methods ===\n");
printf("fact(5) = %d\n", fact(5));
printf("%s\n", greet($name));

# --- io.sprintf ---
printf("=== sprintf ===\n");
def line as string init sprintf("[%d:%s]", 42, "hi");
printf("%s\n", $line);

# --- convert library ---
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

# --- math library ---
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

# --- strings library ---
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
printf("split         = %s\n", join(split("a,b,c", ","), "|"));
printf("chars count   = %d\n", len(chars("héllo")));
printf("join          = %s\n", join(["x", "y", "z"], "-"));

# --- os library (M8: first namespaced library) ---
#
# Every name lives behind the `os.` prefix. The actual *values* of
# os.platform() / os.JENNIFER_OS / os.JENNIFER_LF depend on the host
# OS (Jennifer ships Linux-only today, but Windows and macOS are on
# the roadmap), so we only assert their runtime *kinds* here - same
# pattern that keeps JENNIFER_VERSION out of the golden file. The
# platform-pinned demo lives in examples/osinfo.j. `os.getEnv` of a
# deliberately unset variable returns the empty string portably.
printf("=== os ===\n");
printf("typeOf(os.platform())  = %s\n", typeOf(os.platform()));
printf("typeOf(os.JENNIFER_OS) = %s\n", typeOf(os.JENNIFER_OS));
printf("typeOf(os.JENNIFER_LF) = %s\n", typeOf(os.JENNIFER_LF));
printf("getEnv unset           = [%s]\n", os.getEnv("JENNIFER_SHOWCASE_NONEXISTENT_VAR"));

# --- lists ---
printf("=== lists ===\n");
def xs as list of int init [10, 20, 30];
printf("xs = [%d, %d, %d]\n", $xs[0], $xs[1], $xs[2]);
printf("len(xs) = %d\n", len($xs));
$xs[1] = 99;
printf("after $xs[1] = 99: %d\n", $xs[1]);
def grid as list of list of int init [[1, 2], [3, 4]];
$grid[0][1] = 9;
printf("grid[0] = [%d, %d]\n", $grid[0][0], $grid[0][1]);

printf("=== for-each list ===\n");
for (def elem in $xs) {
    printf("  %d\n", $elem);
}

# --- maps ---
printf("=== maps ===\n");
def scores as map of string to int init {"alice": 90, "bob": 80};
printf("alice=%d bob=%d\n", $scores["alice"], $scores["bob"]);
printf("len(scores) = %d\n", len($scores));
$scores["carol"] = 70;
printf("has alice = %t, has dave = %t\n", maps.has($scores, "alice"), maps.has($scores, "dave"));

printf("=== for-each map (insertion order) ===\n");
for (def who in $scores) {
    printf("  %s=%d\n", $who, $scores[$who]);
}

# --- M9: lists library + $xs[] append sugar ---
printf("=== lists ===\n");
def ns as list of int init [3, 1, 4, 1, 5];
$ns[] = 9;
$ns[] = 2;
printf("after append: len=%d last=%d\n", len($ns), lists.last($ns));
def sorted as list of int init lists.sort($ns);
for (def v in $sorted) { printf("%d ", $v); }
printf("\n");
def reversed as list of int init lists.reverse($sorted);
printf("first(reversed)=%d last(reversed)=%d\n", lists.first($reversed), lists.last($reversed));
printf("contains 4 = %t, contains 99 = %t\n",
    lists.contains($ns, 4), lists.contains($ns, 99));
def slice as list of int init lists.slice($sorted, 1, 4);
for (def v in $slice) { printf("%d ", $v); }
printf("\n");
def joined as list of int init lists.concat([1, 2], [3, 4]);
printf("concat len=%d\n", len($joined));
def front as list of int init lists.head($sorted, 2);
def back as list of int init lists.tail($sorted, 2);
printf("head=[%d,%d] tail=[%d,%d]\n", $front[0], $front[1], $back[0], $back[1]);
$ns = lists.pop($ns);
printf("after pop: len=%d last=%d\n", len($ns), lists.last($ns));

# --- M9: maps library ---
printf("=== maps lib ===\n");
def points as map of string to int init {"alice": 90, "bob": 80, "carol": 70};
def names as list of string init maps.keys($points);
def vals as list of int init maps.values($points);
for (def n in $names) { printf("%s ", $n); }
printf("\n");
for (def v in $vals) { printf("%d ", $v); }
printf("\n");
def smaller as map of string to int init maps.delete($points, "bob");
printf("after delete: len=%d has(bob)=%t\n", len($smaller), maps.has($smaller, "bob"));
def merged as map of string to int init maps.merge($points, {"dave": 60, "alice": 100});
printf("merged: alice=%d dave=%d\n", $merged["alice"], $merged["dave"]);

# --- value semantics + deep const ---
printf("=== value semantics ===\n");
def src as list of int init [1, 2, 3];
def dst as list of int init [0];
$dst = $src;
$dst[0] = 99;
printf("src[0]=%d dst[0]=%d\n", $src[0], $dst[0]);

# --- core (auto-loaded): prove JENNIFER_VERSION is wired without baking its value into the golden ---
printf("=== core ===\n");
printf("typeOf(JENNIFER_VERSION) = %s\n", typeOf(JENNIFER_VERSION));

# --- Constants in expressions ---
printf("=== constants ===\n");
printf("MAX=%d MAX_RETRIES=%d HTTP_OK=%d PI_APPROX=%f\n", MAX, MAX_RETRIES, HTTP_OK, PI_APPROX);

# --- printf modifiers (M7) ---
/* Per-verb pipe modifiers shape the rendered representation:
   presentation only, never data transformation. Each example below
   exercises one or two modifiers so the golden output stays compact.
   This is a block comment - the line comments elsewhere in this
   file use the # form; both are valid (block comments don't nest). */
printf("=== printf modifiers ===\n");
printf("%%s pad/align:  [%s|pad=8|align=right]\n", "hi");
printf("%%s max:        [%s|max=3]\n", "abcdef");
printf("%%s quote:      %s|mode=quote\n", "say \"hi\"");
printf("%%d base=2:     %d|base=2\n", 42);
printf("%%d base=16:    %d|base=16\n", 255);
printf("%%d group/sep:  %d|group=3|sep=,\n", 1234567);
printf("%%d sign+fill:  %d|pad=5|fill=0|sign=always\n", 42);
printf("%%f prec/trim:  %f|prec=4|trim=true\n", 3.14);
printf("%%f sci:        %f|sci=true|prec=2\n", 0.00123);
printf("%%t case=upper: %t|case=upper\n", true);
printf("%%t case=title: %t|case=title\n", false);
def maybe as null;
printf("null=literal: [%s|null=literal(\"N/A\")|pad=6|align=right]\n", $maybe);
# `||` is the literal-pipe escape after a verb, parallel to `%%`.
printf("|| escape:    %s||then\n", "X");

# --- empty container literals (M6) ---
#
# `[]` / `{}` are valid literals; their element/key/value type comes
# from the declared variable type at the def site.
def emptyList as list of int init [];
def emptyMap as map of string to int init {};
printf("=== empty containers ===\n");
printf("len(emptyList) = %d, len(emptyMap) = %d\n", len($emptyList), len($emptyMap));

printf("=== done ===\n");
