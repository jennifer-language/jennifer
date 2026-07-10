# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# benchmark.j - small Jennifer benchmark suite. Two purposes:
#
#   1. Demonstrate the `time` library: `time.now()`,
#      `time.sub(end, start)`, `time.milliseconds(d)`.
#   2. Provide a side-by-side workload between the default `jennifer`
#      (standard Go) and the constrained `jennifer-tiny` (TinyGo) so
#      programmers (and embedding evaluators) can see where the two
#      binaries diverge on the same machine.
#
# `--format text` (default) prints the human table; `--format json`
# emits one JSON object (for a performance database, e.g. the `flatdb`
# module).
#
# Not part of the golden-file test suite (output is timing-dependent
# and host-dependent). Tune the workload sizes at the top so each
# block finishes in O(seconds), not minutes, on a modern laptop.
#
# The report and data-row lines are intentionally wide (aligned output
# and struct literals), so line-length is silenced file-wide.
# lint-disable-file: L203

use io;
use time;
use math;
use lists;
use maps;
use strings;
use convert;
use meta;
use task;
use os;
use fs;
use json;

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
def const PARALLEL_WORKERS as int init 4;

# --- Helpers -------------------------------------------------------

# fib is intentionally the textbook exponential recursive form so the
# interpreter dispatch loop dominates the cost.
func fib(n as int) {
    if ($n < 2) { return $n; }
    return fib($n - 1) + fib($n - 2);
}

# printRow renders one column-aligned timing row. The pad/align
# modifiers come from io's format-verb mini-language. `base` is
# the per-workload size constant (FIB_N, PRIME_LIMIT, ...); `iters`
# is the outer-loop iteration count (often 1 for "single sweep"
# workloads).
func printRow(name as string, base as int, iters as int, ms as int) {
    io.printf("%s|pad=30|align=left %d|pad=12|align=right %d|pad=12|align=right %d|pad=12|align=right\n",   # lint-disable: L203
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
        lists.slice($zs, 0, LIST_SIZE // 2);
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
    strings.join($parts, ",");
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

# --- Parallel workloads (spawn / task) -----------------------------
# Each parallel variant partitions the same workload across N
# `spawn` workers and joins with `task.waitAll`. The serial
# baseline above is the apples-to-apples comparison; speedup =
# serial_ms / parallel_ms.

# Count primes in [lo, hi] by trial division. Used as the spawn body
# for benchPrimesParallel; called once per worker over a sub-range.
func primesInRange(lo as int, hi as int) {
    def count as int init 0;
    def n as int init $lo;
    if ($n < 2) { $n = 2; }
    while ($n <= $hi) {
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
    return $count;
}

func benchPrimesParallel() {
    def start as time.Time init time.now();
    def workers as list of task of int init [];
    def per as int init PRIME_LIMIT // PARALLEL_WORKERS;
    def w as int init 0;
    while ($w < PARALLEL_WORKERS) {
        def lo as int init $w * $per + 1;
        def hi as int init ($w + 1) * $per;
        if ($w == PARALLEL_WORKERS - 1) {
            $hi = PRIME_LIMIT;
        }
        $workers[] = spawn { return primesInRange($lo, $hi); };
        $w = $w + 1;
    }
    def counts as list of int init task.waitAll($workers);
    def primeCount as int init 0;
    for (def c in $counts) {
        $primeCount = $primeCount + $c;
    }
    def gap as time.Duration init time.sub(time.now(), $start);
    return time.milliseconds($gap);
}

# Newton iteration sub-range: target values from lo (inclusive) to
# hi (exclusive). 30 inner iterations each, matching the serial
# benchmark.
func newtonRange(lo as int, hi as int) {
    def i as int init $lo;
    while ($i < $hi) {
        def target as float init convert.toFloat($i);
        def guess as float init $target;
        def j as int init 0;
        while ($j < 30) {
            $guess = ($guess + $target / $guess) / 2.0;
            $j = $j + 1;
        }
        $i = $i + 1;
    }
    return 0;
}

func benchNewtonParallel() {
    def start as time.Time init time.now();
    def workers as list of task of int init [];
    def per as int init NEWTON_ITERS // PARALLEL_WORKERS;
    def w as int init 0;
    while ($w < PARALLEL_WORKERS) {
        def lo as int init $w * $per + 1;
        def hi as int init ($w + 1) * $per + 1;
        if ($w == PARALLEL_WORKERS - 1) {
            $hi = NEWTON_ITERS + 1;
        }
        $workers[] = spawn { return newtonRange($lo, $hi); };
        $w = $w + 1;
    }
    task.waitAll($workers);
    def gap as time.Duration init time.sub(time.now(), $start);
    return time.milliseconds($gap);
}

# Monte-Carlo pi worker: throws independent random samples, returns
# the number inside the quarter-circle. Each worker uses its own
# seed offset so the streams don't correlate.
func monteCarloWorker(seed as int, throws as int) {
    math.randSeed($seed);
    def i as int init 0;
    def inside as int init 0;
    while ($i < $throws) {
        def x as float init math.rand();
        def y as float init math.rand();
        if ($x * $x + $y * $y <= 1.0) {
            $inside = $inside + 1;
        }
        $i = $i + 1;
    }
    return $inside;
}

func benchMonteCarloParallel() {
    def start as time.Time init time.now();
    def workers as list of task of int init [];
    def per as int init MONTECARLO_THROWS // PARALLEL_WORKERS;
    def w as int init 0;
    while ($w < PARALLEL_WORKERS) {
        def seed as int init 42 + $w;
        def throws as int init $per;
        if ($w == PARALLEL_WORKERS - 1) {
            $throws = MONTECARLO_THROWS - $per * (PARALLEL_WORKERS - 1);
        }
        $workers[] = spawn { return monteCarloWorker($seed, $throws); };
        $w = $w + 1;
    }
    task.waitAll($workers);
    def gap as time.Duration init time.sub(time.now(), $start);
    return time.milliseconds($gap);
}

# fib done as PARALLEL_WORKERS independent calls; deliberately simple
# fan-out so the spawn / waitAll overhead is the variable, not the
# partitioning logic. Equivalent serial cost is fib(N) repeated N
# times where N = PARALLEL_WORKERS.
func benchFibParallel() {
    def start as time.Time init time.now();
    def workers as list of task of int init [];
    def w as int init 0;
    while ($w < PARALLEL_WORKERS) {
        $workers[] = spawn { return fib(FIB_N); };
        $w = $w + 1;
    }
    task.waitAll($workers);
    def gap as time.Duration init time.sub(time.now(), $start);
    return time.milliseconds($gap);
}

# printSpeedupRow renders one row of the serial-vs-parallel
# comparison table. The speedup is rendered as a float with 2
# decimals so even sub-second variation is visible.
func printSpeedupRow(name as string, serialMs as int, parallelMs as int) {
    def ratio as float init 0.0;
    if ($parallelMs > 0) {
        $ratio = convert.toFloat($serialMs) / convert.toFloat($parallelMs);
    }
    io.printf("%s|pad=30|align=left %d|pad=12|align=right %d|pad=12|align=right %f|prec=2|pad=12|align=right\n",   # lint-disable: L203
        $name, $serialMs, $parallelMs, $ratio);
}

# cpuModel returns the CPU brand string. The OS-specific probing lives here
# in Jennifer, not the interpreter: `os` stays portable (it only exposes the
# stdlib `os.NCPU`), and this reads Linux's /proc/cpuinfo via `fs` when it is
# present. Off Linux (or if the field is absent) it returns "unknown".
func cpuModel() {
    def path as string init "/proc/cpuinfo";
    if (not fs.exists($path)) {
        return "unknown";
    }
    def lines as list of string init strings.split(fs.readString($path), "\n");
    for (def line in $lines) {
        if (strings.startsWith($line, "model name")) {
            def colon as int init strings.indexOf($line, ":");
            if ($colon >= 0) {
                return strings.trim(strings.substring($line, $colon + 1, len($line)));
            }
        }
    }
    return "unknown";
}

# --- Report model + output formats --------------------------------
# The run is captured in structs so it can be emitted either as the
# human table (--format text, the default) or as one JSON object
# (--format json) - the latter is what a future `flatdb` performance
# database would ingest, keyed on cpu / platform / arch / build.

def struct Bench { name as string, base as int, iters as int, timeMs as int };
def struct Par { name as string, serialMs as int, parMs as int, speedup as float };
def struct Report {
    schema as string,
    build as string,
    version as string,
    cpu as string,
    ncpu as int,
    platform as string,
    arch as string,
    workers as int,
    totalMs as int,
    workloads as list of Bench,
    parallel as list of Par
};

# speedup is serial/parallel wall-clock, 0.0 when the parallel run was
# unmeasurably fast (avoids a divide-by-zero).
func speedup(serialMs as int, parMs as int) {
    if ($parMs > 0) {
        return convert.toFloat($serialMs) / convert.toFloat($parMs);
    }
    return 0.0;
}

func emitText(r as Report) {
    io.printf("=== Jennifer benchmark suite ===\n");
    io.printf("build:    %s\n", $r.build);
    io.printf("version:  %s\n", $r.version);
    io.printf("cpu:      %s (%d cores)\n", $r.cpu, $r.ncpu);
    io.printf("platform: %s/%s\n", $r.platform, $r.arch);
    io.printf("\n");

    printDivider();
    io.printf("%s|pad=30|align=left %s|pad=12|align=right %s|pad=12|align=right %s|pad=12|align=right\n",   # lint-disable: L203
        "Workload", "base", "iters", "time_ms");
    printDivider();
    for (def w in $r.workloads) {
        printRow($w.name, $w.base, $w.iters, $w.timeMs);
    }
    printDivider();
    io.printf("%s|pad=30|align=left %s|pad=12|align=right %s|pad=12|align=right %d|pad=12|align=right\n",   # lint-disable: L203
        "total", "", "", $r.totalMs);

    io.printf("\n");
    printDivider();
    io.printf("Parallel comparison (workers = %d, scheduler = %s)\n", $r.workers, $r.build);
    printDivider();
    io.printf("%s|pad=30|align=left %s|pad=12|align=right %s|pad=12|align=right %s|pad=12|align=right\n",   # lint-disable: L203
        "Workload", "serial_ms", "par_ms", "speedup");
    printDivider();
    for (def p in $r.parallel) {
        io.printf("%s|pad=30|align=left %d|pad=12|align=right %d|pad=12|align=right %f|prec=2|pad=12|align=right\n",   # lint-disable: L203
            $p.name, $p.serialMs, $p.parMs, $p.speedup);
    }
    printDivider();
    return;
}

func emitJson(r as Report) {
    io.printf("%s\n", json.encode($r));
    return;
}

# --- Driver --------------------------------------------------------

# Serial section: run every workload, collecting its time.
def msFib as int init benchFib();
def msPrimes as int init benchPrimes();
def msNewton as int init benchNewton();
def msMonteCarlo as int init benchMonteCarlo();
def msListCopy as int init benchListCopy();
def msStructList as int init benchStructList();
def msStringJoin as int init benchStringJoin();
def msMapChurn as int init benchMapChurn();

def workloads as list of Bench init [];
$workloads[] = Bench{name: "fib(N) recursive", base: FIB_N, iters: 1, timeMs: $msFib};
$workloads[] = Bench{name: "primes up to LIMIT", base: PRIME_LIMIT, iters: 1, timeMs: $msPrimes};
$workloads[] = Bench{name: "newton sqrt batch", base: NEWTON_ITERS, iters: NEWTON_ITERS, timeMs: $msNewton};
$workloads[] = Bench{name: "monte carlo pi", base: MONTECARLO_THROWS, iters: MONTECARLO_THROWS, timeMs: $msMonteCarlo};
$workloads[] = Bench{name: "list sort/reverse/slice", base: LIST_SIZE, iters: LIST_COPY_REPS, timeMs: $msListCopy};
$workloads[] = Bench{name: "struct list build+read", base: STRUCT_LIST_SIZE, iters: STRUCT_LIST_SIZE, timeMs: $msStructList};
$workloads[] = Bench{name: "string join", base: STRING_JOIN_SIZE, iters: STRING_JOIN_SIZE, timeMs: $msStringJoin};
$workloads[] = Bench{name: "map insert+read", base: MAP_CHURN_SIZE, iters: MAP_CHURN_SIZE, timeMs: $msMapChurn};

def total as int init 0;
for (def w in $workloads) {
    $total = $total + $w.timeMs;
}

# Parallel section: re-run the workloads that fan out over PARALLEL_WORKERS
# spawn tasks, measured against the serial baselines above. For fib the
# fan-out runs PARALLEL_WORKERS copies, so its serial reference is the serial
# time times the worker count (perfect scaling would give PARALLEL_WORKERS).
def msPrimesPar as int init benchPrimesParallel();
def msNewtonPar as int init benchNewtonParallel();
def msMonteCarloPar as int init benchMonteCarloParallel();
def msFibPar as int init benchFibParallel();
def fibSerial as int init $msFib * PARALLEL_WORKERS;

def spPrimes as float init speedup($msPrimes, $msPrimesPar);
def spNewton as float init speedup($msNewton, $msNewtonPar);
def spMonte as float init speedup($msMonteCarlo, $msMonteCarloPar);
def spFib as float init speedup($fibSerial, $msFibPar);

def parallel as list of Par init [];
$parallel[] = Par{name: "primes up to LIMIT", serialMs: $msPrimes, parMs: $msPrimesPar, speedup: $spPrimes};
$parallel[] = Par{name: "newton sqrt batch", serialMs: $msNewton, parMs: $msNewtonPar, speedup: $spNewton};
$parallel[] = Par{name: "monte carlo pi", serialMs: $msMonteCarlo, parMs: $msMonteCarloPar, speedup: $spMonte};
$parallel[] = Par{name: "fib(N) x workers", serialMs: $fibSerial, parMs: $msFibPar, speedup: $spFib};

def report as Report init Report{
    schema: "jennifer-benchmark/1",
    build: meta.BUILD,
    version: meta.VERSION,
    cpu: cpuModel(),
    ncpu: os.NCPU,
    platform: os.PLATFORM,
    arch: os.ARCH,
    workers: PARALLEL_WORKERS,
    totalMs: $total,
    workloads: $workloads,
    parallel: $parallel
};

# --format json emits one JSON object (for a perf database); anything else
# (or absent) prints the human table.
if (os.flag("--format") == "json") {
    emitJson($report);
} else {
    emitText($report);
}
