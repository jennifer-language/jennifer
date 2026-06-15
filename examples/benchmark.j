# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# benchmark.j - small Jennifer benchmark suite. Two purposes:
#
#   1. Demonstrate the M15.5 `time` library: `time.now()`,
#      `time.sub(end, start)`, `time.milliseconds(d)`.
#   2. Provide a side-by-side workload between the shipping `jennifer`
#      (TinyGo) and the development `jennifer-go` (standard Go) so
#      programmers (and macflyos-embedding evaluators) can see where
#      the two binaries diverge on the same machine.
#
# Not part of the golden-file test suite (output is timing-dependent
# and host-dependent). Tune the workload sizes at the top so each
# block finishes in O(seconds), not minutes, on a modern laptop.

use io;
use time;
use math;
use lists;
use maps;
use strings;
use convert;
use meta;

# --- Tunables ------------------------------------------------------
# Bump these if the suite runs too fast to see meaningful numbers, or
# trim them if a slow machine sits on a block for too long. The
# `base` column in the output shows the value used.
def const FIB_N as int init 23;
def const PRIME_LIMIT as int init 100000;
def const NEWTON_ITERS as int init 10000;
def const MONTECARLO_THROWS as int init 500000;
def const LIST_COPY_REPS as int init 500;
def const LIST_SIZE as int init 10000;
def const STRUCT_LIST_SIZE as int init 10000;
def const STRING_JOIN_SIZE as int init 10000;
def const MAP_CHURN_SIZE as int init 10000;

# --- Helpers -------------------------------------------------------

# fib is intentionally the textbook exponential recursive form so the
# interpreter dispatch loop dominates the cost.
func fib(n as int) {
    if ($n < 2) { return $n; }
    return fib($n - 1) + fib($n - 2);
}

# printRow renders one column-aligned timing row. The pad/align
# modifiers come from io's M7 format-verb mini-language. `base` is
# the per-workload size constant (FIB_N, PRIME_LIMIT, ...); `iters`
# is the outer-loop iteration count (often 1 for "single sweep"
# workloads).
func printRow(name as string, base as int, iters as int, ms as int) {
    io.printf("%s|pad=30|align=left %d|pad=12|align=right %d|pad=12|align=right %d|pad=12|align=right\n",
        $name, $base, $iters, $ms);
}

func printDivider() {
    io.printf("%s\n", strings.repeat("-", 70));
}

# --- Workloads -----------------------------------------------------
# Each workload returns its elapsed wall-clock milliseconds. Doing
# the timing inside the function keeps the top level uncluttered and
# isolates each measurement from the others.

func benchFib() {
    def start as time.Time init time.now();
    fib(FIB_N);
    def gap as time.Duration init time.sub(time.now(), $start);
    return time.milliseconds($gap);
}

# Trial-division prime count up to LIMIT. A Sieve of Eratosthenes
# would do less arithmetic, but value-semantic lists deep-copy on
# every `$s[i] = false;` write, which turns the sieve into an O(N^2)
# tour of the heap. Trial division stays on scalars and stresses the
# evaluator dispatch loop, which is the milestone's stated goal.
func benchPrimes() {
    def start as time.Time init time.now();
    def count as int init 0;
    def n as int init 2;
    while ($n <= PRIME_LIMIT) {
        def isPrime as bool init true;
        def d as int init 2;
        while ($d * $d <= $n) {
            if ($n % $d == 0) {
                $isPrime = false;
            }
            $d = $d + 1;
        }
        if ($isPrime) {
            $count = $count + 1;
        }
        $n = $n + 1;
    }
    def gap as time.Duration init time.sub(time.now(), $start);
    return time.milliseconds($gap);
}

# Newton's iteration for sqrt: x' = (x + N/x) / 2. Hand-rolled rather
# than calling math.sqrt so we stress the float arithmetic path.
func benchNewton() {
    def start as time.Time init time.now();
    def i as int init 1;
    while ($i <= NEWTON_ITERS) {
        def target as float init convert.toFloat($i);
        def guess as float init $target;
        def j as int init 0;
        while ($j < 30) {
            $guess = ($guess + $target / $guess) / 2.0;
            $j = $j + 1;
        }
        $i = $i + 1;
    }
    def gap as time.Duration init time.sub(time.now(), $start);
    return time.milliseconds($gap);
}

# Monte-Carlo pi: throw N points into the unit square, count how many
# fall inside the unit quarter-circle. Seeded so the result is
# reproducible across runs of the same binary (timings still vary).
func benchMonteCarlo() {
    math.randSeed(42);
    def start as time.Time init time.now();
    def i as int init 0;
    def inside as int init 0;
    while ($i < MONTECARLO_THROWS) {
        def x as float init math.rand();
        def y as float init math.rand();
        if ($x * $x + $y * $y <= 1.0) {
            $inside = $inside + 1;
        }
        $i = $i + 1;
    }
    def gap as time.Duration init time.sub(time.now(), $start);
    return time.milliseconds($gap);
}

# Chain three non-mutating list operations. Each call deep-copies its
# input by value semantics, so the per-binding Value.Copy cost
# dominates.
func benchListCopy() {
    def xs as list of int init lists.range(0, LIST_SIZE);
    def start as time.Time init time.now();
    def i as int init 0;
    while ($i < LIST_COPY_REPS) {
        def ys as list of int init lists.sort($xs);
        def zs as list of int init lists.reverse($ys);
        def ws as list of int init lists.slice($zs, 0, LIST_SIZE // 2);
        $i = $i + 1;
    }
    def gap as time.Duration init time.sub(time.now(), $start);
    return time.milliseconds($gap);
}

# A user-defined struct exercised through assignment (which deep-copies).
def struct Point { x as int, y as int };

func benchStructList() {
    def start as time.Time init time.now();
    def points as list of Point init [];
    def i as int init 0;
    while ($i < STRUCT_LIST_SIZE) {
        $points[] = Point{ x: $i, y: $i * 2 };
        $i = $i + 1;
    }
    # Read each back so we exercise the struct deep-copy on read.
    def sum as int init 0;
    def j as int init 0;
    while ($j < STRUCT_LIST_SIZE) {
        def p as Point init $points[$j];
        $sum = $sum + $p.x + $p.y;
        $j = $j + 1;
    }
    def gap as time.Duration init time.sub(time.now(), $start);
    return time.milliseconds($gap);
}

func benchStringJoin() {
    def start as time.Time init time.now();
    def ns as list of int init lists.range(0, STRING_JOIN_SIZE);
    def parts as list of string init [];
    for (def n in $ns) {
        $parts[] = convert.toString($n);
    }
    def joined as string init strings.join($parts, ",");
    def gap as time.Duration init time.sub(time.now(), $start);
    return time.milliseconds($gap);
}

func benchMapChurn() {
    def start as time.Time init time.now();
    def m as map of string to int init {};
    def i as int init 0;
    while ($i < MAP_CHURN_SIZE) {
        def k as string init strings.repeat("k", 1) + convert.toString($i);
        $m[$k] = $i;
        $i = $i + 1;
    }
    # Read each back to exercise the hashed lookup path.
    def sum as int init 0;
    def j as int init 0;
    while ($j < MAP_CHURN_SIZE) {
        def k as string init strings.repeat("k", 1) + convert.toString($j);
        $sum = $sum + $m[$k];
        $j = $j + 1;
    }
    def gap as time.Duration init time.sub(time.now(), $start);
    return time.milliseconds($gap);
}

# --- Driver --------------------------------------------------------

io.printf("=== Jennifer benchmark suite ===\n");
io.printf("build:   %s\n", meta.BUILD);
io.printf("version: %s\n", meta.VERSION);
io.printf("\n");

printDivider();
io.printf("%s|pad=30|align=left %s|pad=12|align=right %s|pad=12|align=right %s|pad=12|align=right\n",
    "Workload", "base", "iters", "time_ms");
printDivider();

def total as int init 0;

def msFib as int init benchFib();
printRow("fib(N) recursive", FIB_N, 1, $msFib);
$total = $total + $msFib;

def msPrimes as int init benchPrimes();
printRow("primes up to LIMIT", PRIME_LIMIT, 1, $msPrimes);
$total = $total + $msPrimes;

def msNewton as int init benchNewton();
printRow("newton sqrt batch", NEWTON_ITERS, NEWTON_ITERS, $msNewton);
$total = $total + $msNewton;

def msMonteCarlo as int init benchMonteCarlo();
printRow("monte carlo pi", MONTECARLO_THROWS, MONTECARLO_THROWS, $msMonteCarlo);
$total = $total + $msMonteCarlo;

def msListCopy as int init benchListCopy();
printRow("list sort/reverse/slice", LIST_SIZE, LIST_COPY_REPS, $msListCopy);
$total = $total + $msListCopy;

def msStructList as int init benchStructList();
printRow("struct list build+read", STRUCT_LIST_SIZE, STRUCT_LIST_SIZE, $msStructList);
$total = $total + $msStructList;

def msStringJoin as int init benchStringJoin();
printRow("string join", STRING_JOIN_SIZE, STRING_JOIN_SIZE, $msStringJoin);
$total = $total + $msStringJoin;

def msMapChurn as int init benchMapChurn();
printRow("map insert+read", MAP_CHURN_SIZE, MAP_CHURN_SIZE, $msMapChurn);
$total = $total + $msMapChurn;

printDivider();
io.printf("%s|pad=30|align=left %s|pad=12|align=right %s|pad=12|align=right %d|pad=12|align=right\n",
    "total", "", "", $total);
