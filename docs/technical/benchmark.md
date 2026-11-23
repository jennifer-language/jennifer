# Benchmarks

Single-binary throughput for the two builds - `jennifer` (standard Go, the
default) and `jennifer-tiny` (TinyGo, the constrained variant) - measured with
`examples/benchmark.j`.

Reference numbers are from `examples/benchmark.j` on an **AMD Ryzen 5
7600X3D** (6 cores, 12 threads; bare metal, desktop idle) - the machine the
suite prints in its own header, from `os.NCPU` plus a `/proc/cpuinfo` read done
in Jennifer. The numbers are machine-specific; the **ratios and the shape** of
the comparison are the stable part. Every figure below comes from one
`--suite all` run of each binary at version `0.25.0-dev+265.abf83f98` (v1 to v4 in
one process, under GNU `time`), so the cross-build comparisons are
like for like, and so is the memory / wall-clock table. Both binaries were built
on the reference machine with **Go 1.27.4** (`jennifer`) and **TinyGo 0.44.0** on
Go 1.27.4 and LLVM 22.1.4 (`jennifer-tiny`); the toolchain is part of what is
measured.

The suite is **versioned** (`--suite v1` / `v2` / `v3` / `v4` / `all`) so its
reference series stays comparable over time. **v1** is the interpreter-throughput
workload; **v2** is the numeric-library workload (`stats` / `linalg` / `math` /
`ml`); **v3** is the value-semantics workload (copies, in-place updates, borrowed
reads, spawn snapshots); **v4** is the borrow-gap workload (the read-only
arguments the borrow still copies). v2, v3 and v4 are reported in their own
sections below. Each version's workloads are frozen once they have published
reference numbers.

The v1 suite has two sections. The **serial** section is single-threaded by
design; the **parallel** section fans out to `PARALLEL_WORKERS = 4` spawn tasks
per workload. The measured interpreter has value semantics with eager copies,
lexical slot resolution, parse-time constant folding, the advisory map hash
index, per-frame allocation elimination (slot-backed bindings), read-only
borrow, the in-place int / bool fast path, and the `binary` bulk-byte
primitives, so append-in-a-loop is amortised O(N) with in-place growth, keyed
map access is O(1), a call frame allocates nothing per binding, integer loops
move no `Value`s, and a byte scan can drop into a single Go call.

> These are reference-machine figures, recorded from a supplied run, **not**
> produced in CI - benchmark timings are too machine- and load-sensitive to pin
> in a test. Regenerate them on the reference machine after a perf-relevant
> change (`jennifer run examples/benchmark.j --suite all --format json`); do not
> run the suite on other hardware for documentation numbers.

## Suite v1 - `jennifer` (standard-Go binary, default)

```
=== Jennifer benchmark suite ===
build:    go
version:  0.25.0-dev+265.abf83f98
cpu:      AMD Ryzen 5 7600X3D 6-Core Processor (12 cores)
platform: linux/amd64
host:     bare metal (no VM / container hints)

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           39
primes up to LIMIT                   100000            1         3491
newton sqrt batch                     10000        10000          175
monte carlo pi                       500000       500000          608
list sort/reverse/slice               10000          500          625
struct list build+read                10000        10000           24
string join                           10000        10000            7
map insert+read                       10000        10000           35
byte scan naive (per-byte)           500000           10          860
byte scan binary.indexOf (Go)        500000           10            0
----------------------------------------------------------------------
total                                                            5864

----------------------------------------------------------------------
Parallel comparison (workers = 4, scheduler = go)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                     3491         1293         2.70
newton sqrt batch                       175           45         3.89
monte carlo pi                          608          173         3.51
fib(N) x workers                        156           42         3.71
----------------------------------------------------------------------
```

Go runs the serial section single-threaded while its concurrent GC works on other
cores, then reaches multi-core throughput in the parallel section (four
`spawn` workers on separate OS threads).

## Suite v1 - `jennifer-tiny` (TinyGo binary)

```
=== Jennifer benchmark suite ===
build:    tinygo
version:  0.25.0-dev+265.abf83f98
cpu:      AMD Ryzen 5 7600X3D 6-Core Processor (1 cores)
platform: linux/amd64
host:     bare metal (no VM / container hints)

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           30
primes up to LIMIT                   100000            1         2657
newton sqrt batch                     10000        10000          172
monte carlo pi                       500000       500000          596
list sort/reverse/slice               10000          500         1237
struct list build+read                10000        10000           30
string join                           10000        10000           12
map insert+read                       10000        10000           50
byte scan naive (per-byte)           500000           10          674
byte scan binary.indexOf (Go)        500000           10            1
----------------------------------------------------------------------
total                                                            5459

----------------------------------------------------------------------
Parallel comparison (workers = 4, scheduler = tinygo)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                     2657         2788         0.95
newton sqrt batch                       172          184         0.93
monte carlo pi                          596          541         1.10
fib(N) x workers                        120          131         0.92
----------------------------------------------------------------------
```

`os.NCPU` reports `1` here: the usable parallelism of the cooperative
`-scheduler=tasks` build, not the machine's 12 threads. Every goroutine
runs on one OS thread, so the parallel column hovers at ~1.0 by design, dipping
slightly below on rows where spawn setup has no parallel payoff to offset it.

## v1 per-workload comparison (serial section)

Ratios are `tiny_ms / go_ms`; > 1.0 means `jennifer-tiny` is slower, < 1.0
(shown in *italics*) means it is faster.

| Workload                       | tiny (ms) | go (ms) | Ratio  | Where the time goes                                                            |
| ------------------------------ | --------- | ------- | ------ | ------------------------------------------------------------------------------ |
| `fib(N) recursive`             |        30 |      39 | *0.8x* | Call-heavy dispatch; TinyGo's tighter loop edges it. Both effectively free (sub-50 ms) thanks to the per-frame allocation elimination. |
| `primes up to LIMIT`           |      2657 |    3491 | *0.8x* | The long integer loop, the single biggest row. TinyGo leads it by ~0.8 s.      |
| `newton sqrt batch`            |       172 |     175 | 1.0x   | Float arithmetic + dispatch; a tie.                                            |
| `monte carlo pi`               |       596 |     608 | 1.0x   | Float arithmetic + RNG calls; a tie.                                           |
| `list sort/reverse/slice`      |      1237 |     625 | 2.0x   | Allocation-heavy. Go's concurrent GC absorbs the sort/reverse/slice churn far better than TinyGo's simpler collector - Go's largest serial win (~0.6 s). |
| `struct list build+read`       |        30 |      24 | 1.3x   | Append hot loop is O(1); both effectively free (sub-40 ms).                    |
| `string join`                  |        12 |       7 | 1.7x   | Build-up-a-string pattern is O(1); both free (sub-20 ms).                      |
| `map insert+read`              |        50 |      35 | 1.4x   | The advisory map hash index keeps keyed insert+read O(1); both sub-50 ms.      |
| `byte scan naive (per-byte)`   |       674 |     860 | *0.8x* | A per-byte tree-walker scan of ~500 KB x 10 - the cost the `binary` library exists to remove. TinyGo leads it by ~0.2 s. |
| `byte scan binary.indexOf (Go)`|         1 |       0 | -      | The same scan through one `binary.indexOf` call (Go `bytes.Index`, SIMD). Both effectively **0 ms** - see below. |
| **total**                      |      5459 |    5864 | *0.9x* | TinyGo posts the lower serial total (~7%), carried by `primes` and the naive byte scan. |

TinyGo takes the serial section (5.5 s vs 5.9 s, ~7% lower) on its tighter
dispatch loop: it leads `primes` (-0.8 s), the naive `byte scan` (-0.2 s) and
`fib`, and ties `newton` and `monte carlo`. Go wins the allocation-heavy `list`
row by ~0.6 s, twice as fast as TinyGo there, and the small `struct` /
`string` / `map` rows (all sub-50 ms). **Without the two byte-scan rows the
totals are 4784 ms (TinyGo) vs 5004 ms (Go)**, so TinyGo's serial lead is not
only the byte scan. (The milliseconds shift run to run; what holds is that the
builds trade wins by workload shape - TinyGo the tight compute loops, Go the
allocation churn and the parallel section below.)

## The bulk-byte rows

The two `byte scan` rows are the `binary` library's throughput demonstration:
the same search for a 6-byte needle at the **end** of a ~500 KB buffer, 10 times
(so every scan traverses the whole buffer).

- **Naive, per byte** (a `.j` loop comparing bytes one at a time): 860 ms (Go) /
  674 ms (TinyGo). This is the per-byte tree-walker cost byte-oriented code pays.
- **`binary.indexOf`** (one call into Go's assembly/SIMD `bytes.Index`): **0 ms**
  (Go) / 1 ms (TinyGo).

The Go row reads `0` because it is sub-millisecond: ~500 KB x 10 = ~5 MB
scanned at ~20 GB/s takes a couple hundred microseconds, and the suite reports
whole milliseconds (TinyGo's `1` is the same work, just at the 1 ms rounding
line). Pushing the per-byte loop into one Go call is a ~3,500x speedup on this
workload.

## v1 parallel section

Speedup is `serial_ms / par_ms`; > 1.0 means the four-worker version beat serial.
Go gets real multi-core speedup; TinyGo's cooperative scheduler stays at ~1.0 by
design.

| Workload             | Go serial (ms) | Go par (ms) | Go speedup | TinyGo serial (ms) | TinyGo par (ms) | TinyGo speedup |
| -------------------- | -------------- | ----------- | ---------- | ------------------ | --------------- | -------------- |
| `primes up to LIMIT` |           3491 |        1293 |   **2.70** |               2657 |            2788 |      0.95      |
| `newton sqrt batch`  |            175 |          45 |   **3.89** |                172 |             184 |      0.93      |
| `monte carlo pi`     |            608 |         173 |   **3.51** |                596 |             541 |      1.10      |
| `fib(N) x workers`   |            156 |          42 |   **3.71** |                120 |             131 |      0.92      |

Go reaches 2.7x-3.9x on four workers; the evenly split rows land at 3.5x-3.9x,
~3-12% under a linear 4.0 for spawn and scheduling overhead. `primes` scales
worst, and that is the partition, not contention: trial division costs about
`sqrt(n)` per number, so the worker holding the top quarter of the range
(75 001-100 000) does ~35% of the work and caps the speedup near 2.85x; the same
overhead takes it to the measured 2.70x. `jennifer-tiny` pins the cooperative
scheduler, so `spawn` there is concurrency without multi-core throughput: its
column sits at ~1.0. Use the default binary when parallel throughput matters.

**The parallel section reverses the end-to-end picture.** TinyGo has the lower
*serial* total (5.5 s vs 5.9 s), but Go crunches the parallel section in
**~1.6 s** (1293 + 45 + 173 + 42) where TinyGo takes **~3.6 s** (no
parallelism). On the wall clock the full `--suite all` run finished in
**10.70 s on Go vs 17.23 s on TinyGo** (see the memory table below): a lower
single-thread total does not mean a faster end-to-end run once any `spawn`
parallelism is in play.

## Suite v2 - numeric libraries (`stats` / `linalg` / `math` / `ml`)

The v2 workloads exercise the numeric stack: the Go-side compute plus the
Jennifer <-> Go marshaling of scalars and matrices. Both builds run it serially
(no parallel section). These blocks are from the `--suite all` run, so v2 runs
after v1 in the same process.

```
=== Jennifer benchmark suite (v2 within --suite all, jennifer) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
stats normal cdf                      50000        50000           73
math regGammaP                        50000        50000           86
linalg matmul NxN                        30          300           24
linalg solve NxN                         30          300           10
ml linreg fit+predict                   500          200           22
ml kMeans cluster                       500           40            6
----------------------------------------------------------------------
total                                                             221
```

```
=== Jennifer benchmark suite (v2 within --suite all, jennifer-tiny) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
stats normal cdf                      50000        50000           96
math regGammaP                        50000        50000          103
linalg matmul NxN                        30          300           23
linalg solve NxN                         30          300           11
ml linreg fit+predict                   500          200           28
ml kMeans cluster                       500           40            6
----------------------------------------------------------------------
total                                                             267
```

Go is ahead on v2 (221 ms vs 267 ms, ~17% lower), most of it on the two
distribution rows (40 of the 46 ms) and the rest on `ml linreg`; `matmul`,
`linalg solve` and `ml kMeans` are within 1 ms. In absolute terms the gap is
~46 ms over the whole suite. The distribution rows (`stats normal cdf`,
`math regGammaP`) dominate the total (159 of 221 ms on Go, 199 of 267 ms on
TinyGo) because they are the interpreter-heavy ones: 50 000
tree-walked iterations each, one Go call per iteration. The `linalg` / `ml` rows
are cheap despite doing matrix work: the compute is one Go call over a matrix
built once outside the timed loop, so the per-iteration cost is mostly list
marshaling, and the Go-side algorithms (matmul, Gaussian solve, normal-equations
fit) are far below the dispatch cost. The tree-walker overhead is in the
per-call loop around the numeric libraries, not the math.

## Suite v3 - value semantics

v1 and v2 are dominated by dispatch loops and Go-side compute, so they barely
register what the value model costs. v3 measures that cost directly. Every row
moves a `Grid` - a struct
whose 10 000-element list is the expensive part to copy (`VS_CELLS`, a 100x100
grid, between an 80x24 and a 244x110 terminal) - or a list of small nested
structs, through one path of the value model:

| Workload                    | What it isolates                                                                  |
| --------------------------- | --------------------------------------------------------------------------------- |
| `owned update g = f(g)`     | `$g = gridSet($g, ...)` in a function: the argument is dead after the call, so the update may run in place. 20 000 calls. |
| `copy-then-write update`    | the defensive idiom `def out as Grid init $g; $out.cells[i] = v; return $out;`, 2 000 calls: what the interpreter makes of the local copy. |
| `snapshot + write (detach)` | the old value is kept alive across a write, so every one of 2 000 iterations must copy: the price of one full detach, the baseline for the rows above. |
| `borrowed read param`       | a `Grid` passed to a function that only reads it, 20 000 calls: the read-only borrow. |
| `list append helper`        | `$xs = appendTo($xs, v)`, the accumulator helper lint L205 names, growing a list to 10 000. |
| `nested field write`        | `$units[$j].pos.x = ...` and `$units[$j].hp = ...` over 1 000 structs, 50 passes: deep path writes into a list of structs. |
| `spawn capture`             | 200 tasks, each spawned and waited on in turn, that read one cell of a `Grid` in the enclosing scope: the cost of the scope snapshot `spawn` takes. One task at a time, because hundreds alive at once make `jennifer-tiny`'s per-task stack reservation the whole cost. |

Read the rows against each other rather than alone. The owned-update and
borrowed-read rows should cost a small fraction of the snapshot row; when one
of them climbs toward it, the interpreter is copying where it does not need to.
(`benchmark.j` itself spawns workers and holds mutable globals, so the
`borrowed read param` row is also the canary for anything that turns the
read-only borrow off run-wide. It passes a struct, so it does not cover the
shapes the borrow skips; v4 does.)

Two properties keep the series honest. **Every row checks its result** (the
final checksum, and for the snapshot row that each snapshot still holds its
pre-write value) and stops the run when it is wrong, so a build that got faster
by breaking copy semantics fails instead of posting a better number. **The
workloads use only long-standing language features** (full `Error` literals,
complete struct literals), so every interpreter in the series can run the same
`examples/benchmark.j --suite v3`.

```
=== Jennifer benchmark suite (v3 within --suite all, jennifer) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
owned update g = f(g)                 10000        20000           36
copy-then-write update                10000         2000          481
snapshot + write (detach)             10000         2000          399
borrowed read param                   10000        20000           20
list append helper                    10000        10000           13
nested field write                     1000           50          125
spawn capture                         10000          200           43
----------------------------------------------------------------------
total                                                            1117
```

```
=== Jennifer benchmark suite (v3 within --suite all, jennifer-tiny) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
owned update g = f(g)                 10000        20000           30
copy-then-write update                10000         2000         1252
snapshot + write (detach)             10000         2000         1265
borrowed read param                   10000        20000           17
list append helper                    10000        10000           12
nested field write                     1000           50          160
spawn capture                         10000          200          368
----------------------------------------------------------------------
total                                                            3104
```

Per operation on the reference machine (Go / TinyGo), and the cross-build ratio
`tiny_ms / go_ms` as in the v1 table:

| Workload                    | per operation        | Ratio  | What it shows                                                     |
| --------------------------- | -------------------- | ------ | ----------------------------------------------------------------- |
| `owned update g = f(g)`     | 1.8 / 1.5 us a call  | *0.8x* | Runs in place: about **110x cheaper than one copy** of the grid on Go, ~420x on TinyGo. |
| `copy-then-write update`    | 0.24 / 0.63 ms a call | 2.6x  | Costs about the same as the forced copy below: the defensive local copy is one full copy per call, so it is the idiom to avoid. |
| `snapshot + write (detach)` | 0.20 / 0.63 ms a copy | 3.2x  | The price of one full copy of a 10 000-cell grid: the baseline.   |
| `borrowed read param`       | 1.0 / 0.9 us a call  | *0.9x* | No copy: a read-only parameter is borrowed.                       |
| `list append helper`        | 1.3 / 1.2 us a call  | *0.9x* | Runs in place: the L205 accumulator is linear when called as `$xs = appendTo($xs, v)`. |
| `nested field write`        | 2.5 / 3.2 us a step  | 1.3x   | Two deep path writes per step (50 passes over 1 000 units), no copy of the list. |
| `spawn capture`             | 0.22 / 1.8 ms a task | 8.6x   | On Go, about one grid copy per task: the snapshot is a full copy. TinyGo adds roughly 1.2 ms of per-task overhead on top. |
| **total**                   |                      | 2.8x   |                                                                   |

The value model behaves as designed on both builds: every path that may avoid a
copy does (owned update, borrowed read, append helper, nested write), and the
two paths that must copy cost exactly one copy each. Where the builds differ is
the copy itself and task start-up. Since the previous reference run
(`+107.33ed417`) a full copy on Go is about twice as fast (0.47 ms to 0.20 ms a
grid), while TinyGo's improved less (0.76 ms to 0.63 ms), so a copy is now
~2.6-3.2x slower on TinyGo. Each `spawn` also costs more under TinyGo's
cooperative scheduler with a fixed 4 MB stack per task (see
[tinygo.md](tinygo.md), which also covers the cost of many tasks alive at once).
Both TinyGo gaps are wider in a `--suite all` run than in a standalone
`--suite v3` run: after v1 and v2 the TinyGo heap is larger, and its simpler
collector pays for that on every allocation-heavy row.

## Suite v4 - borrow gaps

v3's `borrowed read param` row passes a struct, the shape the read-only borrow
handles. v4 covers the three shapes it does not handle yet, each a function that
only reads one cell of a large argument, so a build that borrows runs the row in
microseconds and a build that copies pays one full copy per call:

| Workload               | What it isolates |
| ---------------------- | ---------------- |
| `nested read param`    | a 100 x 100 `list of list of int` passed to a function that reads one cell, 2 000 calls: a nested-collection parameter is never borrowed (M30.3). |
| `wrapped nested read`  | the same matrix inside a struct, 2 000 calls: the control, and the workaround - a struct parameter is borrowed. |
| `sortBy callee read`   | a v3 `Grid` passed to a function that reads one cell and also calls `lists.sortBy` with a named key, 2 000 calls: in a program with mutable globals, a callback builtin anywhere below a function turns its borrow off (M30.8). |
| `module handle call`   | a 10 000-node tree struct declared in `examples/benchmark/tree.j`, passed to that module's one-lookup `has`, 200 calls: the module-boundary rebuild of every argument holding the module's own structs (M29.14). |

Read `nested read param` against `wrapped nested read`: the same work, and the
ratio is the cost of the copy. The `sortBy` row reads against v3's
`borrowed read param` the same way. When one of the gaps closes, its row drops
to the control's level and the suite records it. `jennifer profile --allocs
examples/benchmark.j --suite v4` shows the copies (eager copies) and the
rebuilds (module-boundary rebuilds) by call site.

```
=== Jennifer benchmark suite (v4 within --suite all, jennifer) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
nested read param                     10000         2000          688
wrapped nested read                   10000         2000            2
sortBy callee read                    10000         2000          456
module handle call                    10000          200          672
----------------------------------------------------------------------
total                                                            1818
```

```
=== Jennifer benchmark suite (v4 within --suite all, jennifer-tiny) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
nested read param                     10000         2000         1527
wrapped nested read                   10000         2000            2
sortBy callee read                    10000         2000         1476
module handle call                    10000          200         1590
----------------------------------------------------------------------
total                                                            4595
```

| Workload               | per call (Go / TinyGo) | Ratio  | What it shows |
| ---------------------- | ---------------------- | ------ | ------------- |
| `nested read param`    | 0.34 / 0.76 ms         | 2.2x   | One full copy of the 100 x 100 matrix per call, ~340x the wrapped control on Go. A copy of 100 inner lists costs more than v3's flat 10 000-cell grid (0.20 ms). |
| `wrapped nested read`  | ~1 / ~1 us             | -      | Borrowed: the same read through a struct parameter is effectively free (both rows round to 2 ms). |
| `sortBy callee read`   | 0.23 / 0.74 ms         | 3.2x   | About one v3 grid copy per call, against ~1.0 us for v3's `borrowed read param`: the callback builtin turns the borrow off. |
| `module handle call`   | 3.4 / 8.0 ms           | 2.4x   | The module-boundary rebuild of the 10 000-node tree: per call, the costliest row in v3 and v4, and the one row that did not get faster on Go since `+107`. |
| **total**              |                        | 2.5x   | |

All three gaps cost a full copy or rebuild on both builds; no row is close to
its control yet. The two copy rows halved on Go with the faster copy (v3); the
rebuild row did not. TinyGo pays 2.2x-3.2x more for each, in line with its
slower copies in v3.

## Memory, page faults, and wall clock

GNU `time` on the `--suite all` run (v1 + v2 + v3 + v4) on the reference
machine, version `0.25.0-dev+265.abf83f98`:

| Metric                | `jennifer` (Go)  | `jennifer-tiny` (TinyGo) |
| --------------------- | ---------------- | ------------------------ |
| wall clock (real)     | 10.70 s          | 17.23 s                  |
| user / system         | 19.62 s / 0.84 s | 17.07 s / 0.10 s         |
| CPU                   | 191%             | 99%                      |
| peak resident (RSS)   | ~81 MB           | ~216 MB                  |
| minor page faults     | ~302,100         | ~65,800                  |
| major page faults     | 0                | 0                        |

- **Go finishes faster end-to-end** (10.70 s vs 17.23 s) while spending more
  CPU-seconds doing it (19.6 s user at 191% CPU). The gap between `user` and
  `real` is the concurrent GC on other cores plus the four `spawn` workers running
  in parallel during the parallel section. TinyGo's `user ~= real` at 99% CPU
  shows single-thread execution: the cooperative scheduler runs every goroutine
  on one OS thread.
- **TinyGo uses ~2.7x the peak RSS** (~216 MB vs ~81 MB). Its cooperative
  scheduler reserves each goroutine's full `-stack-size` (4 MB, sized above the
  catchable call-depth cap) up front, so spawned workers hold reserved stack
  while they are alive, and its simpler collector lets the heap grow further
  between collections during the v3 copy rows and the v4 copies and rebuilds.
  Go grows goroutine stacks on demand from ~8 KB, so its footprint stays small
  and flat.
- **Go takes ~4.6x the minor page faults** (~302k vs ~66k), most likely
  from the v3 copy rows and the v4 copies and rebuilds (each a full grid,
  matrix or tree): Go keeps a small heap and
  hands pages back between collections, so it touches fresh pages again, where
  TinyGo holds on to the larger heap it already touched. Neither build takes any
  major faults.

## Picking a binary

- **Single-thread compute:** the two builds trade wins by workload shape. TinyGo
  leads the tight compute loops (`primes`, `fib`, the naive byte scan) and ties
  `newton` and `monte carlo`; Go leads the allocation-heavy `list` row (2x) and
  the small structural rows. Which build posts the lower serial *total* shifts
  run to run.
- **End-to-end wall clock / any `spawn` parallelism:** the default **`jennifer`**,
  the only build with multi-core throughput (2.7x-3.9x here); it finishes the
  whole suite faster despite a higher serial total.
- **Footprint:** `jennifer` for a small, on-demand memory profile (~81 MB peak);
  `jennifer-tiny` trades ~2.7x the RSS for its smaller *binary* and embeddability,
  not for a smaller runtime footprint.
- **Value-heavy work:** copies cost ~2.6-3.2x more on `jennifer-tiny`, `spawn`
  snapshots ~8.6x, module-boundary rebuilds ~2.4x; the copy-avoiding paths
  (owned update, borrowed read, append helper) cost the same or less.
- **Byte-oriented work:** reach for the `binary` library (and `net.readAll` /
  `readN`) on either build - the `binary.indexOf` row shows a per-byte loop
  collapsing to effectively free.
- **Numeric work:** `stats` / `linalg` / `math` / `ml` cost about the same on
  both builds (Go ~17% lower on v2 here, ~46 ms in absolute terms); the per-call
  dispatch loop dominates, not the Go-side math.
