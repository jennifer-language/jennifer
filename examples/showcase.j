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
# convert.typeOf() instead.

use io;
use convert;
use math;
use strings;
use lists;
use maps;
use os;
include "showcase/helpers.j";

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

io.printf("=== variables ===\n");
io.printf("x=%d y=%f name=%s flag=%t nothing=%v empty=%d\n", $x, $y, $name, $flag, $nothing, $empty);

# --- Arithmetic, including the Python-3 / vs // distinction ---
io.printf("=== arithmetic ===\n");
io.printf("10 + 5 = %d\n", 10 + 5);
io.printf("10 - 5 = %d\n", 10 - 5);
io.printf("10 * 5 = %d\n", 10 * 5);
io.printf("10 / 4 = %f\n", 10 / 4);
io.printf("10 // 4 = %d\n", 10 // 4);
io.printf("10 %% 3 = %d\n", 10 % 3);
io.printf("-x = %d\n", -$x);
io.printf("int + float = %f\n", 3 + 0.5);

# --- Comparison and logical operators ---
io.printf("=== comparison and logic ===\n");
io.printf("5 < 10  = %t\n", 5 < 10);
io.printf("5 == 5  = %t\n", 5 == 5);
io.printf("5 >= 5  = %t\n", 5 >= 5);
io.printf("true and false = %t\n", true and false);
io.printf("true or  false = %t\n", true or false);
io.printf("not false      = %t\n", not false);

# --- String concatenation ---
io.printf("=== string concat ===\n");
def greeting as string init "Hello, " + $name + "!";
io.printf("%s\n", $greeting);

# --- Control flow ---
io.printf("=== if / elseif / else ===\n");
if ($x > 0) {
    io.printf("x is positive\n");
} elseif ($x == 0) {
    io.printf("x is zero\n");
} else {
    io.printf("x is negative\n");
}

io.printf("=== while ===\n");
def i as int init 1;
while ($i <= 3) {
    io.printf("  while i=%d\n", $i);
    $i = $i + 1;
}

io.printf("=== for ===\n");
for (def j as int init 0; $j < 3; $j = $j + 1) {
    io.printf("  for j=%d\n", $j);
}

# --- Methods + recursion + file-imported helper ---
io.printf("=== methods ===\n");
io.printf("fact(5) = %d\n", fact(5));
io.printf("%s\n", greet($name));

# --- io.sprintf ---
io.printf("=== sprintf ===\n");
def line as string init io.sprintf("[%d:%s]", 42, "hi");
io.printf("%s\n", $line);

# --- convert library ---
io.printf("=== convert ===\n");
io.printf("convert.toInt(3.7)       = %d\n", convert.toInt(3.7));
io.printf("convert.toInt(\"42\")      = %d\n", convert.toInt("42"));
io.printf("convert.toFloat(5)       = %f\n", convert.toFloat(5));
io.printf("convert.toString(42)     = %s\n", convert.toString(42));
io.printf("convert.toBool(1)        = %t\n", convert.toBool(1));
io.printf("convert.toBool(\"true\")   = %t\n", convert.toBool("true"));
io.printf("convert.typeOf(3.14)   = %s\n", convert.typeOf(3.14));
io.printf("convert.typeOf(\"x\")    = %s\n", convert.typeOf("x"));
io.printf("convert.typeOf(true)   = %s\n", convert.typeOf(true));
io.printf("convert.typeOf(null)   = %s\n", convert.typeOf(null));

# --- math library ---
io.printf("=== math ===\n");
io.printf("math.abs(-7)        = %d\n", math.abs(-7));
io.printf("math.abs(-3.5)      = %f\n", math.abs(-3.5));
io.printf("math.min(3, 7)      = %d\n", math.min(3, 7));
io.printf("math.max(3.5, 2.5)  = %f\n", math.max(3.5, 2.5));
io.printf("math.sqrt(16)       = %f\n", math.sqrt(16));
io.printf("math.pow(2, 10)     = %f\n", math.pow(2, 10));
io.printf("math.floor(3.7)     = %d\n", math.floor(3.7));
io.printf("math.ceil(3.2)      = %d\n", math.ceil(3.2));
io.printf("math.round(2.5)     = %d\n", math.round(2.5));
io.printf("math.round(-2.5)    = %d\n", math.round(-2.5));

io.printf("=== math constants ===\n");
io.printf("convert.typeOf(math.PI) = %s\n", convert.typeOf(math.PI));
io.printf("convert.typeOf(math.E)  = %s\n", convert.typeOf(math.E));

# --- strings library ---
io.printf("=== strings ===\n");
def sample as string init "Hello, World";
io.printf("len           = %d\n", len($sample));
io.printf("upper         = %s\n", strings.upper($sample));
io.printf("lower         = %s\n", strings.lower($sample));
io.printf("contains      = %t\n", strings.contains($sample, "World"));
io.printf("startsWith    = %t\n", strings.startsWith($sample, "Hello"));
io.printf("endsWith      = %t\n", strings.endsWith($sample, "World"));
io.printf("indexOf       = %d\n", strings.indexOf($sample, "World"));
io.printf("indexOf miss  = %d\n", strings.indexOf($sample, "zzz"));
io.printf("trim          = [%s]\n", strings.trim("  ab  "));
io.printf("trimLeft      = [%s]\n", strings.trimLeft("  ab  "));
io.printf("trimRight     = [%s]\n", strings.trimRight("  ab  "));
io.printf("replace       = %s\n", strings.replace($sample, "World", "Jennifer"));
io.printf("repeat        = %s\n", strings.repeat("ab", 3));
io.printf("substring 0..5 = %s\n", strings.substring($sample, 0, 5));
io.printf("substring 7..  = %s\n", strings.substring($sample, 7));
io.printf("split         = %s\n", strings.join(strings.split("a,b,c", ","), "|"));
io.printf("chars count   = %d\n", len(strings.chars("héllo")));
io.printf("join          = %s\n", strings.join(["x", "y", "z"], "-"));

# --- os library (M8) ---
#
# Every name lives behind the `os.` prefix. The actual *values* of
# os.platform() / os.JENNIFER_OS / os.JENNIFER_LF depend on the host
# OS (Jennifer ships Linux-only today, but Windows and macOS are on
# the roadmap), so we only assert their runtime *kinds* here - same
# pattern that keeps JENNIFER_VERSION out of the golden file. The
# platform-pinned demo lives in examples/osinfo.j. `os.getEnv` of a
# deliberately unset variable returns the empty string portably.
io.printf("=== os ===\n");
io.printf("convert.typeOf(os.platform())  = %s\n", convert.typeOf(os.platform()));
io.printf("convert.typeOf(os.JENNIFER_OS) = %s\n", convert.typeOf(os.JENNIFER_OS));
io.printf("convert.typeOf(os.JENNIFER_LF) = %s\n", convert.typeOf(os.JENNIFER_LF));
io.printf("getEnv unset           = [%s]\n", os.getEnv("JENNIFER_SHOWCASE_NONEXISTENT_VAR"));

# --- lists ---
io.printf("=== lists ===\n");
def xs as list of int init [10, 20, 30];
io.printf("xs = [%d, %d, %d]\n", $xs[0], $xs[1], $xs[2]);
io.printf("len(xs) = %d\n", len($xs));
$xs[1] = 99;
io.printf("after $xs[1] = 99: %d\n", $xs[1]);
def grid as list of list of int init [[1, 2], [3, 4]];
$grid[0][1] = 9;
io.printf("grid[0] = [%d, %d]\n", $grid[0][0], $grid[0][1]);

io.printf("=== for-each list ===\n");
for (def elem in $xs) {
    io.printf("  %d\n", $elem);
}

# --- maps ---
io.printf("=== maps ===\n");
def scores as map of string to int init {"alice": 90, "bob": 80};
io.printf("alice=%d bob=%d\n", $scores["alice"], $scores["bob"]);
io.printf("len(scores) = %d\n", len($scores));
$scores["carol"] = 70;
io.printf("has alice = %t, has dave = %t\n", maps.has($scores, "alice"), maps.has($scores, "dave"));

io.printf("=== for-each map (insertion order) ===\n");
for (def who in $scores) {
    io.printf("  %s=%d\n", $who, $scores[$who]);
}

# --- M9: lists library + $xs[] append sugar ---
io.printf("=== lists ===\n");
def ns as list of int init [3, 1, 4, 1, 5];
$ns[] = 9;
$ns[] = 2;
io.printf("after append: len=%d last=%d\n", len($ns), lists.last($ns));
def sorted as list of int init lists.sort($ns);
for (def v in $sorted) { io.printf("%d ", $v); }
io.printf("\n");
def reversed as list of int init lists.reverse($sorted);
io.printf("first(reversed)=%d last(reversed)=%d\n", lists.first($reversed), lists.last($reversed));
io.printf("contains 4 = %t, contains 99 = %t\n",
    lists.contains($ns, 4), lists.contains($ns, 99));
def slice as list of int init lists.slice($sorted, 1, 4);
for (def v in $slice) { io.printf("%d ", $v); }
io.printf("\n");
def joined as list of int init lists.concat([1, 2], [3, 4]);
io.printf("concat len=%d\n", len($joined));
def front as list of int init lists.head($sorted, 2);
def back as list of int init lists.tail($sorted, 2);
io.printf("head=[%d,%d] tail=[%d,%d]\n", $front[0], $front[1], $back[0], $back[1]);
$ns = lists.pop($ns);
io.printf("after pop: len=%d last=%d\n", len($ns), lists.last($ns));

# lists.range: half-open. End is exclusive so `range(0, n)`
# matches the valid 0-based indices into an n-element list,
# `concat(range(a, b), range(b, c))` composes cleanly into
# `range(a, c)`, and stepping always "stops before end" without
# the user having to reason about exact divisibility. For a
# closed counting range write `lists.range(1, N + 1)`.
io.printf("range(0,5)     = %a\n", lists.range(0, 5));
io.printf("range(1,5)     = %a\n", lists.range(1, 5));
io.printf("range(0,9,3)   = %a\n", lists.range(0, 9, 3));
io.printf("range(1,9,3)   = %a\n", lists.range(1, 9, 3));
io.printf("range(5,5)     = %a\n", lists.range(5, 5));
io.printf("range(10,1,-3) = %a\n", lists.range(10, 1, -3));

# Composability: partitioning a range at any midpoint reassembles
# byte-for-byte. concat(range(2, 7), range(7, 12)) == range(2, 12).
def left as list of int init lists.range(2, 7);
def right as list of int init lists.range(7, 12);
def joinedRange as list of int init lists.concat($left, $right);
def whole as list of int init lists.range(2, 12);
io.printf("partitioned    = %a\n", $joinedRange);
io.printf("whole          = %a\n", $whole);

# lists.shuffle: non-mutating Fisher-Yates. randSeed makes runs
# deterministic so this output is stable.
math.randSeed(1);
io.printf("shuffle = %a\n", lists.shuffle([1, 2, 3, 4, 5]));

# --- M9: maps library ---
io.printf("=== maps lib ===\n");
def points as map of string to int init {"alice": 90, "bob": 80, "carol": 70};
def names as list of string init maps.keys($points);
def vals as list of int init maps.values($points);
for (def n in $names) { io.printf("%s ", $n); }
io.printf("\n");
for (def v in $vals) { io.printf("%d ", $v); }
io.printf("\n");
def smaller as map of string to int init maps.delete($points, "bob");
io.printf("after delete: len=%d has(bob)=%t\n", len($smaller), maps.has($smaller, "bob"));
def merged as map of string to int init maps.merge($points, {"dave": 60, "alice": 100});
io.printf("merged: alice=%d dave=%d\n", $merged["alice"], $merged["dave"]);

# --- value semantics + deep const ---
io.printf("=== value semantics ===\n");
def src as list of int init [1, 2, 3];
def dst as list of int init [0];
$dst = $src;
$dst[0] = 99;
io.printf("src[0]=%d dst[0]=%d\n", $src[0], $dst[0]);

# --- core (auto-loaded): prove JENNIFER_VERSION is wired without baking its value into the golden ---
io.printf("=== core ===\n");
io.printf("convert.typeOf(JENNIFER_VERSION) = %s\n", convert.typeOf(JENNIFER_VERSION));

# --- Constants in expressions ---
io.printf("=== constants ===\n");
io.printf("MAX=%d MAX_RETRIES=%d HTTP_OK=%d PI_APPROX=%f\n", MAX, MAX_RETRIES, HTTP_OK, PI_APPROX);

# --- printf modifiers (M7) ---
/* Per-verb pipe modifiers shape the rendered representation:
   presentation only, never data transformation. Each example below
   exercises one or two modifiers so the golden output stays compact.
   This is a block comment - the line comments elsewhere in this
   file use the # form; both are valid (block comments don't nest). */
io.printf("=== printf modifiers ===\n");
io.printf("%%s pad/align:  [%s|pad=8|align=right]\n", "hi");
io.printf("%%s max:        [%s|max=3]\n", "abcdef");
io.printf("%%s quote:      %s|mode=quote\n", "say \"hi\"");
io.printf("%%d base=2:     %d|base=2\n", 42);
io.printf("%%d base=16:    %d|base=16\n", 255);
io.printf("%%d group/sep:  %d|group=3|sep=,\n", 1234567);
io.printf("%%d sign+fill:  %d|pad=5|fill=0|sign=always\n", 42);
io.printf("%%f prec/trim:  %f|prec=4|trim=true\n", 3.14);
io.printf("%%f sci:        %f|sci=true|prec=2\n", 0.00123);
io.printf("%%t case=upper: %t|case=upper\n", true);
io.printf("%%t case=title: %t|case=title\n", false);
def maybe as null;
io.printf("null=literal: [%s|null=literal(\"N/A\")|pad=6|align=right]\n", $maybe);
# `||` is the literal-pipe escape after a verb, parallel to `%%`.
io.printf("|| escape:    %s||then\n", "X");

# --- empty container literals (M6) ---
#
# `[]` / `{}` are valid literals; their element/key/value type comes
# from the declared variable type at the def site.
def emptyList as list of int init [];
def emptyMap as map of string to int init {};
io.printf("=== empty containers ===\n");
io.printf("len(emptyList) = %d, len(emptyMap) = %d\n", len($emptyList), len($emptyMap));

# --- M11: break / continue / repeat...until ---
io.printf("=== M11 control flow ===\n");

# break leaves the innermost loop early
for (def k as int init 0; $k < 10; $k = $k + 1) {
    if ($k == 4) { break; }
    io.printf("%d ", $k);
}
io.printf("\n");

# continue skips one iteration; the step (k = k + 1) still runs.
for (def k as int init 0; $k < 5; $k = $k + 1) {
    if ($k == 2) { continue; }
    io.printf("%d ", $k);
}
io.printf("\n");

# repeat...until runs the body at least once, then checks `until`.
def count as int init 0;
repeat {
    io.printf("%d ", $count);
    $count = $count + 1;
} until ($count >= 3);
io.printf("\n");

# --- M11: printf %s|align=center ---
io.printf("=== M11 align=center ===\n");
io.printf("[%s|pad=8|align=center]\n", "hi");
io.printf("[%s|pad=9|align=center]\n", "hi");

# --- M11: printf %a aggregate verb ---
io.printf("=== M11 aggregate %%a ===\n");
def nums as list of int init [1, 2, 3];
io.printf("default        %a\n", $nums);
io.printf("custom sep     %a|sep=\" | \"\n", $nums);
io.printf("brackets       %a|open=\"<\"|close=\">\"\n", $nums);
def matrix as list of list of int init [[1, 2], [3, 4]];
io.printf("nested         %a\n", $matrix);
io.printf("depth=1        %a|depth=1\n", $matrix);
def scoreMap as map of string to int init {"a": 1, "b": 2};
io.printf("map            %a\n", $scoreMap);
io.printf("map kv style   %a|kv=\"=\"|sep=\" \"\n", $scoreMap);

# --- M12: non-decimal literals + digit separators ---
io.printf("=== M12 literals ===\n");
io.printf("0xff           = %d\n", 0xff);
io.printf("0xDEAD_BEEF    = %d\n", 0xDEAD_BEEF);
io.printf("0o755          = %d\n", 0o755);
io.printf("0b1010_0110    = %d\n", 0b1010_0110);
io.printf("1_000_000      = %d\n", 1_000_000);

# --- M12: bit operators ---
io.printf("=== M12 bit ops ===\n");
io.printf("0xff & 0x0f    = %d|base=16\n", 0xff & 0x0f);
io.printf("0x0f | 0xf0    = %d|base=16\n", 0x0f | 0xf0);
io.printf("0xff ^ 0xaa    = %d|base=16\n", 0xff ^ 0xaa);
io.printf("~0             = %d\n", ~0);
io.printf("1 << 8         = %d\n", 1 << 8);
io.printf("256 >> 1       = %d\n", 256 >> 1);

# --- M12: bytes type ---
io.printf("=== M12 bytes ===\n");
def msg as bytes init convert.bytesFromString("Hi!", "utf-8");
io.printf("len            = %d\n", len($msg));
io.printf("msg[0]         = %d (H)\n", $msg[0]);
io.printf("display        = %v\n", $msg);
$msg[0] = 0x68;
$msg[] = 0x0a;
io.printf("after mutation = %v\n", $msg);
def roundTripped as string init convert.stringFromBytes($msg, "utf-8");
io.printf("round-trip     = %s", $roundTripped);

# --- M13.1: structs (records) ---
#
# A struct names a fixed set of typed fields. Literals construct
# (every field required), `.field` reads, `.field = ...;` writes.
# Value semantics: assignment copies. Nested structs work and the
# lvalue chain reaches through them. A dedicated walkthrough lives
# in examples/structs.j.
def struct Point { x as int, y as int };
def struct Line { from as Point, to as Point };

io.printf("=== M13.1 structs ===\n");
def p as Point init Point{ x: 3, y: 4 };
io.printf("p              = %v\n", $p);
io.printf("p.x p.y        = %d %d\n", $p.x, $p.y);
$p.x = 30;
io.printf("after $p.x=30  = %v\n", $p);
def q as Point init $p;
$q.y = 99;
io.printf("copied + edited = p=%v q=%v\n", $p, $q);
def L as Line init Line{ from: Point{ x: 0, y: 0 }, to: Point{ x: 10, y: 20 } };
$L.from.x = 5;
io.printf("nested write   = %v\n", $L);

# --- M13.2: try / catch / throw ---
#
# `try { ... } catch (err) { ... }` runs the body; any `throw EXPR;`
# inside it, or any runtime error (out-of-bounds, missing key, etc.),
# binds the thrown value to `$err` in the handler. Convention is to
# throw an `Error` struct - the runtime auto-defines that struct shape.
# A dedicated walkthrough lives in examples/trycatch.j.
io.printf("=== M13.2 try/catch ===\n");

# User throw, caught.
try {
    throw Error{kind: "demo", message: "user", file: "", line: 0, col: 0};
} catch (err) {
    io.printf("user thrown  = %s / %s\n", $err.kind, $err.message);
}

# Runtime error caught uniformly.
def shortList as list of int init [1, 2];
try {
    def bad as int init $shortList[99];
} catch (err) {
    io.printf("runtime err  = kind=%s\n", $err.kind);
}

io.printf("=== done ===\n");
