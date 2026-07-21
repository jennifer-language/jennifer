# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Exercises every Jennifer language feature and every standard-library surface.
 * Used as a golden integration test by cmd/jennifer/examples_test.go.
 * meta.VERSION is intentionally NOT printed - its value depends on
 * git state and would make the golden file non-deterministic. Same
 * for time.now() (wall clock), fs paths that depend on cwd, and
 * net addresses that depend on the kernel's ephemeral-port pick.
 * Everything printed below is deliberately deterministic across
 * runs and hosts.
 * @module showcase
 */

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
io.printf("x=%d y=%f name=%s flag=%t nothing=%v empty=%d\n", $x, $y, $name, $flag, $nothing, $empty);   # lint-disable: L203

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

# --- os library ---
#
# Every name lives behind the `os.` prefix. Immutable per-run host
# facts are uppercase constants; `getEnv` is a real function. The
# actual values of PLATFORM / ARCH / EOL depend on the host OS,
# so we only assert their runtime *kinds* here - the
# platform-pinned demo lives in examples/osinfo.j. `os.getEnv` of a
# deliberately unset variable returns the empty string portably.
io.printf("=== os ===\n");
io.printf("convert.typeOf(os.PLATFORM)    = %s\n", convert.typeOf(os.PLATFORM));
io.printf("convert.typeOf(os.ARCH)        = %s\n", convert.typeOf(os.ARCH));
io.printf("convert.typeOf(os.EOL)         = %s\n", convert.typeOf(os.EOL));
io.printf("convert.typeOf(os.DIRSEP)      = %s\n", convert.typeOf(os.DIRSEP));
io.printf("convert.typeOf(os.PATHSEP)     = %s\n", convert.typeOf(os.PATHSEP));
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

# --- lists library + $xs[] append sugar ---
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

# --- maps library ---
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

# --- meta: prove meta.VERSION + meta.BUILD are wired without
# baking either value into the golden (both depend on build state).
use meta;
io.printf("=== meta ===\n");
io.printf("convert.typeOf(meta.VERSION) = %s\n", convert.typeOf(meta.VERSION));
io.printf("convert.typeOf(meta.BUILD)   = %s\n", convert.typeOf(meta.BUILD));

# --- os: ARGS + flag inspection. Type-check, don't print
# actual values (vary by host / how the program was launched).
io.printf("=== os ARGS + flags ===\n");
io.printf("convert.typeOf(os.ARGS)            = %s\n", convert.typeOf(os.ARGS));
io.printf("convert.typeOf(os.hasFlag(\"x\"))    = %s\n", convert.typeOf(os.hasFlag("--definitely-not-a-real-flag")));   # lint-disable: L203
io.printf("convert.typeOf(os.flag(\"x\"))       = %s\n", convert.typeOf(os.flag("--definitely-not-a-real-flag")));   # lint-disable: L203

# --- Constants in expressions ---
io.printf("=== constants ===\n");
io.printf("MAX=%d MAX_RETRIES=%d HTTP_OK=%d PI_APPROX=%f\n", MAX, MAX_RETRIES, HTTP_OK, PI_APPROX);

# --- printf modifiers ---
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

# --- empty container literals ---
#
# `[]` / `{}` are valid literals; their element/key/value type comes
# from the declared variable type at the def site.
def emptyList as list of int init [];
def emptyMap as map of string to int init {};
io.printf("=== empty containers ===\n");
io.printf("len(emptyList) = %d, len(emptyMap) = %d\n", len($emptyList), len($emptyMap));

# --- break / continue / repeat...until ---
io.printf("=== control flow ===\n");

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

# --- printf %s|align=center ---
io.printf("=== align=center ===\n");
io.printf("[%s|pad=8|align=center]\n", "hi");
io.printf("[%s|pad=9|align=center]\n", "hi");

# --- printf %a aggregate verb ---
io.printf("=== aggregate %%a ===\n");
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

# --- non-decimal literals + digit separators ---
io.printf("=== literals ===\n");
io.printf("0xff           = %d\n", 0xff);
io.printf("0xDEAD_BEEF    = %d\n", 0xDEAD_BEEF);
io.printf("0o755          = %d\n", 0o755);
io.printf("0b1010_0110    = %d\n", 0b1010_0110);
io.printf("1_000_000      = %d\n", 1_000_000);

# --- bit operators ---
io.printf("=== bit ops ===\n");
io.printf("0xff & 0x0f    = %d|base=16\n", 0xff & 0x0f);
io.printf("0x0f | 0xf0    = %d|base=16\n", 0x0f | 0xf0);
io.printf("0xff ^ 0xaa    = %d|base=16\n", 0xff ^ 0xaa);
io.printf("~0             = %d\n", ~0);
io.printf("1 << 8         = %d\n", 1 << 8);
io.printf("256 >> 1       = %d\n", 256 >> 1);

# --- bytes type ---
io.printf("=== bytes ===\n");
def msg as bytes init convert.bytesFromString("Hi!", "utf-8");
io.printf("len            = %d\n", len($msg));
io.printf("msg[0]         = %d (H)\n", $msg[0]);
io.printf("display        = %v\n", $msg);
$msg[0] = 0x68;
$msg[] = 0x0a;
io.printf("after mutation = %v\n", $msg);
def roundTripped as string init convert.stringFromBytes($msg, "utf-8");
io.printf("round-trip     = %s", $roundTripped);

# --- structs (records) ---
#
# A struct names a fixed set of typed fields. Literals construct
# (every field required), `.field` reads, `.field = ...;` writes.
# Value semantics: assignment copies. Nested structs work and the
# lvalue chain reaches through them. A dedicated walkthrough lives
# in examples/structs.j.
def struct Point { x as int, y as int };
def struct Line { from as Point, to as Point };

io.printf("=== structs ===\n");
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

# --- try / catch / throw ---
#
# `try { ... } catch (err) { ... }` runs the body; any `throw EXPR;`
# inside it, or any runtime error (out-of-bounds, missing key, etc.),
# binds the thrown value to `$err` in the handler. Convention is to
# throw an `Error` struct - the runtime auto-defines that struct shape.
# A dedicated walkthrough lives in examples/trycatch.j.
io.printf("=== try/catch ===\n");

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

# --- time (instants, durations, zones, strftime) ---
#
# time.now() is wall-clock and would drift the golden; we exercise
# the deterministic surface (fromUnix + accessors + arithmetic +
# format + iso) and type-check the moving parts.
use time;

io.printf("=== time ===\n");
def epoch as time.Time init time.fromUnix(0);
io.printf("epoch UTC       = %s\n", time.format($epoch, "%Y-%m-%d %H:%M:%S"));
io.printf("epoch year      = %d\n", time.year($epoch));
io.printf("epoch weekday   = %d\n", time.weekday($epoch));
def hourSpan as time.Duration init time.fromMinutes(60);
def afterEpoch as time.Time init time.add($epoch, $hourSpan);
io.printf("epoch+1h iso    = %s\n", time.iso($afterEpoch));
def gap as time.Duration init time.sub($afterEpoch, $epoch);
io.printf("gap minutes     = %d\n", time.minutes($gap));
io.printf("epoch<after     = %t\n", time.before($epoch, $afterEpoch));
io.printf("typeOf now()    = %s\n", convert.typeOf(time.now()));
io.printf("typeOf UTC      = %s\n", convert.typeOf(time.UTC));

# --- hash + crc, rendered through encoding ---
#
# hash.compute + crc.compute return raw bytes; encoding.toText(_,"hex")
# turns them into printable digests. This chains three libraries in
# one line and gives a stable golden.
use hash;
use crc;
use encoding;

io.printf("=== hash / crc ===\n");
def digestIn as bytes init convert.bytesFromString("abc", "utf-8");
io.printf("md5(abc)     = %s\n", encoding.toText(hash.compute($digestIn, "md5"), "hex"));
io.printf("sha1(abc)    = %s\n", encoding.toText(hash.compute($digestIn, "sha1"), "hex"));
io.printf("sha256(abc)  = %s\n", encoding.toText(hash.compute($digestIn, "sha256"), "hex"));
io.printf("crc32(abc)   = %s\n", encoding.toText(crc.compute($digestIn, "crc32"), "hex"));
io.printf("crc64(abc)   = %s\n", encoding.toText(crc.compute($digestIn, "crc64"), "hex"));

# Streamed digest matches the one-shot form.
def sha as hash.Stream init hash.stream("sha256");
hash.update($sha, convert.bytesFromString("a", "utf-8"));
hash.update($sha, convert.bytesFromString("bc", "utf-8"));
io.printf("sha256 stream = %s\n", encoding.toText(hash.finalize($sha), "hex"));

# --- encoding introspection + binary-to-text + codecs ---
io.printf("=== encoding ===\n");
io.printf("base64(abc)  = %s\n", encoding.toText($digestIn, "base64"));
def qpbytes as bytes init convert.bytesFromString("a=b=c", "utf-8");
io.printf("qprintable   = %s\n", encoding.toText($qpbytes, "quoted-printable"));
io.printf("hex round    = %s\n", convert.stringFromBytes(encoding.fromText("616263", "hex"), "utf-8"));   # lint-disable: L203
io.printf("isAscii(abc) = %t\n", encoding.isAscii($digestIn));
io.printf("lenRunes     = %d\n", encoding.lenRunes(convert.bytesFromString("café", "utf-8")));
def latin as bytes init encoding.encode("café", "iso-8859-1");
io.printf("iso-8859-1 hex = %s\n", encoding.toText($latin, "hex"));
io.printf("iso-8859-1 dec = %s\n", encoding.decode($latin, "iso-8859-1"));

# --- task (spawn a computation, wait for the value) ---
#
# spawn { ... } evaluates immediately to a `task of T` handle; the
# body runs concurrently. task.wait re-raises the body's error as
# a positioned runtime error if it failed. task.waitAll collects
# a list of results in list order.
use task;

io.printf("=== task ===\n");
def one as task of int init spawn { return 21 + 21; };
io.printf("wait single  = %d\n", task.wait($one));
def many as list of task of int init [
    spawn { return 1; },
    spawn { return 2; },
    spawn { return 3; }
];
io.printf("waitAll      = %a\n", task.waitAll($many));

# --- fs (whole-file round-trip in a unique scratch dir) ---
#
# fs.makeTempDir hands back a fresh, uniquely-named directory (created
# atomically, so parallel runs never collide); the demo writes into it
# and removes it, so the output is identical wherever it was launched.
use fs;

def scratch as string init fs.makeTempDir("", "showcase-");
fs.writeString($scratch + "/hi.txt", "hello, fs\n");
io.printf("=== fs ===\n");
io.printf("read back    = %s", fs.readString($scratch + "/hi.txt"));
io.printf("isFile       = %t\n", fs.isFile($scratch + "/hi.txt"));
io.printf("isDir        = %t\n", fs.isDir($scratch));
def stat as fs.Stat init fs.stat($scratch + "/hi.txt");
io.printf("stat size    = %d\n", $stat.size);
fs.removeAll($scratch);
io.printf("cleanup      = %t\n", not fs.exists($scratch));

# --- net (in-process TCP echo) ---
#
# The default `jennifer` binary has full networking; `jennifer-tiny`
# has no netdev driver and every net.* call surfaces a friendly
# error. try/catch keeps the demo cross-binary; the golden captures
# the standard-Go success path (which is what `go test` uses).
use net;

io.printf("=== net ===\n");
try {
    def listener as net.Listener init net.listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    def server as task of null init spawn {
        def conn as net.Conn init net.accept($listener);
        def raw as bytes init net.readBytes($conn, 8);
        net.writeBytes($conn, $raw);
        net.close($conn);
        return null;
    };
    def client as net.Conn init net.connect($addr);
    net.writeBytes($client, convert.bytesFromString("net-demo", "utf-8"));
    def echoed as bytes init net.readBytes($client, 8);
    net.close($client);
    task.wait($server);
    net.close($listener);
    io.printf("echo replied = %s\n", convert.stringFromBytes($echoed, "utf-8"));
} catch (err) {
    io.printf("net unavailable (%s)\n", $err.kind);
}

# --- regex (matches, find + groups, replace, split) ---
use regex;

io.printf("=== regex ===\n");
io.printf("matches      = %t\n", regex.matches("^\\d+$", "42"));
def hit as regex.Match init regex.find("(\\w+):(\\d+)", "port:8080");
io.printf("find         = %s (%s / %s)\n", $hit.text, $hit.groups[0], $hit.groups[1]);
def named as regex.Match init regex.find("(?P<key>\\w+)=(?P<val>\\d+)", "n=7");
io.printf("named group  = key=%s val=%s\n",
    $named.groupsNamed["key"], $named.groupsNamed["val"]);
io.printf("replace      = %s\n", regex.replace("\\d+", "port 8080 host 22", "###"));
io.printf("split        = %a\n", regex.split("\\s+", "a  b  c"));
io.printf("escape       = %s\n", regex.escape("1+2=(3)"));

# --- json (encode; decode yields an opaque json.Value the accessors walk) ---
use json;

io.printf("=== json ===\n");
io.printf("encode       = %s\n", json.encode({"name": "jen", "nums": [1, 2, 3], "ok": true}));
def parsed as json.Value init json.decode("{\"x\": 7, \"y\": 8}");
io.printf("decode x+y   = %d\n", json.asInt($parsed, "/x") + json.asInt($parsed, "/y"));

# --- uuid (RFC 9562; values are random / time-based, so assert facts) ---
use uuid;

io.printf("=== uuid ===\n");
io.printf("v4 version   = %d\n", uuid.version(uuid.generate("v4")));
io.printf("v7 version   = %d\n", uuid.version(uuid.generate("v7")));
io.printf("valid        = %t\n", uuid.isValid(uuid.generate("v4")));
io.printf("parse bytes  = %d\n", len(uuid.parse(uuid.generate("v4"))));
io.printf("NIL          = %s\n", uuid.NIL);

# --- crypto (crypto-grade random, constant-time compare, KDFs) ---
# randBytes / randInt are unpredictable, so assert facts, not values;
# pbkdf / hkdf are deterministic functions, so their output is stable.
use crypto;

io.printf("=== crypto ===\n");
def cpw as bytes init convert.bytesFromString("pw", "utf-8");
def csalt as bytes init convert.bytesFromString("salt", "utf-8");
def cinfo as bytes;
io.printf("randBytes len= %d\n", len(crypto.randBytes(16)));
io.printf("randInt range= %t\n", crypto.randInt(1, 6) >= 1 and crypto.randInt(1, 6) <= 6);
io.printf("hmacEqual    = %t / %t\n", crypto.hmacEqual($cpw, $cpw), crypto.hmacEqual($cpw, $csalt));
io.printf("pbkdf        = %s\n",
    encoding.toText(crypto.pbkdf($cpw, $csalt, 1, 16, "sha256"), "hex"));
io.printf("hkdf len     = %d\n", len(crypto.hkdf($cpw, $csalt, $cinfo, 32, "sha256")));

# --- compress (byte-stream pack / unpack) ---
# Packed bytes vary with the zlib version, so assert the round-trip and
# that a repetitive input shrinks, not the exact compressed bytes.
use compress;

io.printf("=== compress ===\n");
def craw as bytes init convert.bytesFromString(strings.repeat("compress me ", 50), "utf-8");
def crt as bytes init compress.unpack(compress.pack($craw, "gzip"), "gzip");
io.printf("gzip rt ok   = %t\n",
    convert.stringFromBytes($crt, "utf-8") == strings.repeat("compress me ", 50));
io.printf("shrank       = %t\n", len(compress.pack($craw, "gzip")) < len($craw));

# --- archive (tar / zip containers over bytes) ---
use archive;

io.printf("=== archive ===\n");
def aentry as archive.Entry init archive.Entry{
    name: "a.txt", data: convert.bytesFromString("alpha", "utf-8"), mode: 0o644, mtime: 1700000000
};
def aback as list of archive.Entry init archive.unpack(archive.pack([$aentry], "tar"), "tar");
io.printf("entries=%d name=%s data=%s\n",
    len($aback), $aback[0].name, convert.stringFromBytes($aback[0].data, "utf-8"));

# --- toml (opaque toml.Value, JSON-Pointer accessors, like json) ---
use toml;

io.printf("=== toml ===\n");
def tdoc as toml.Value init toml.decode("title = \"jen\"\n[server]\nport = 8080\n");
io.printf("title=%s port=%d\n", toml.asString($tdoc, "/title"), toml.asInt($tdoc, "/server/port"));

# --- xml (opaque xml.Value, element tree, XPath-style paths) ---
use xml;

io.printf("=== xml ===\n");
def xdoc as xml.Value init xml.decode("<book id=\"1\"><title>Go &amp; XML</title></book>");
io.printf("tag=%s id=%s title=%s\n",
    xml.tag($xdoc), xml.attr($xdoc, "id"), xml.text(xml.get($xdoc, "title")));

# --- yaml (opaque yaml.Value, same accessors as json / toml) ---
use yaml;

io.printf("=== yaml ===\n");
def ydoc as yaml.Value init yaml.decode("name: jen\nnums: [1, 2, 3]\n");
io.printf("name=%s nums=%d\n",
    yaml.asString($ydoc, "/name"), yaml.length(yaml.get($ydoc, "/nums")));

# --- intl (message catalogs + locale-aware translation) ---
use intl;

io.printf("=== intl ===\n");
intl.load("en", {"hi": "Hello, {name}!"});
intl.load("de", {"hi": "Hallo, {name}!"});
intl.setLocale("de");
io.printf("tr=%s\n", intl.tr("hi", {"name": "Welt"}));

# --- testing (name-based dispatch, per-process accumulator) ---
#
# testing.run invokes a zero-arg method by name, catches user throws
# and runtime errors (including exit), and appends a testing.Result
# to a process-wide accumulator. We define one passing and one
# failing test, run both, and count outcomes. testing.report renders
# text / TAP / JUnit XML - see examples/testing.j for those.
use testing;

func showcasePasses() {
    return;
}

func showcaseFails() {
    throw Error{kind: "assertion", message: "demo", file: "", line: 0, col: 0};
}

testing.reset();
testing.run("showcasePasses");
testing.run("showcaseFails");
def outcomes as list of testing.Result init testing.results();
def pass as int init 0;
def fail as int init 0;
for (def r in $outcomes) {
    if ($r.passed) {
        $pass = $pass + 1;
    } else {
        $fail = $fail + 1;
    }
}
io.printf("=== testing ===\n");
io.printf("ran=%d passed=%d failed=%d\n", len($outcomes), $pass, $fail);

io.printf("=== done ===\n");
