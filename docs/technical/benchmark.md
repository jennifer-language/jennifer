# Benchmarks

Single-binary throughput for the two builds - `jennifer` (standard Go, the
default) and `jennifer-tiny` (TinyGo, the constrained variant) - measured with
`examples/benchmark.j`.

Reference numbers are from `examples/benchmark.j` on an **AMD Ryzen 5
7600X3D** (6 cores, 12 threads; bare metal, desktop idle) - the machine the
suite prints in its own header, from `os.NCPU` plus a `/proc/cpuinfo` read done
in Jennifer. The numbers are machine-specific; the **ratios and the shape** of
the comparison are the stable part. Every figure below comes from one
`--suite all` run of each binary at version `0.24.0-dev+170.be4bfbb` (v1, v2 and
v3 in one process, under GNU `/bin/time -v`), so the cross-build comparisons are
like for like, and so is the memory / wall-clock table. Both binaries were built
on the reference machine with **Go 1.27.2** (`jennifer`) and **TinyGo 0.43.0** on
Go 1.27.2 and LLVM 22.1.4 (`jennifer-tiny`); the toolchain is part of what is
measured.

The suite is **versioned** (`--suite v1` / `v2` / `v3` / `all`) so its reference
series stays comparable over time. **v1** is the interpreter-throughput workload;
**v2** is the numeric-library workload (`stats` / `linalg` / `math` / `ml`);
**v3** is the value-semantics workload (copies, in-place updates, borrowed reads,
spawn snapshots). v2 and v3 are reported in their own sections below. Each
version's workloads are frozen once they have published reference numbers.

The v1 suite has two sections. The **serial** section is single-threaded by
design; the **parallel** section fans out to `PARALLEL_WORKERS = 4` spawn tasks
per workload. The interpreter build is the current one - eager-copy value
semantics, lexical slot resolution, parse-time constant folding, the advisory map
hash index, per-frame allocation elimination (slot-backed bindings), read-only
borrow, the in-place int / bool fast path, and the `binary` bulk-byte
primitives - so append-in-a-loop is amortised O(N) with in-place growth, keyed
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
version:  0.24.0-dev+170.be4bfbb
cpu:      AMD Ryzen 5 7600X3D 6-Core Processor (12 cores)
platform: linux/amd64
host:     bare metal (no VM / container hints)

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           42
primes up to LIMIT                   100000            1         3562
newton sqrt batch                     10000        10000          175
monte carlo pi                       500000       500000          595
list sort/reverse/slice               10000          500          987
struct list build+read                10000        10000           24
string join                           10000        10000            8
map insert+read                       10000        10000           33
byte scan naive (per-byte)           500000           10          864
byte scan binary.indexOf (Go)        500000           10            0
----------------------------------------------------------------------
total                                                            6290

----------------------------------------------------------------------
Parallel comparison (workers = 4, scheduler = go)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                     3562         1229         2.90
newton sqrt batch                       175           48         3.65
monte carlo pi                          595          168         3.54
fib(N) x workers                        168           41         4.10
----------------------------------------------------------------------
```

Go runs the serial section single-threaded while its concurrent GC works on other
cores, then reaches multi-core throughput in the parallel section (four
`spawn` workers on separate OS threads).

## Suite v1 - `jennifer-tiny` (TinyGo binary)

```
=== Jennifer benchmark suite ===
build:    tinygo
version:  0.24.0-dev+170.be4bfbb
cpu:      AMD Ryzen 5 7600X3D 6-Core Processor (1 cores)
platform: linux/amd64
host:     bare metal (no VM / container hints)

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           28
primes up to LIMIT                   100000            1         2694
newton sqrt batch                     10000        10000          151
monte carlo pi                       500000       500000          536
list sort/reverse/slice               10000          500         1410
struct list build+read                10000        10000           30
string join                           10000        10000           14
map insert+read                       10000        10000           49
byte scan naive (per-byte)           500000           10          719
byte scan binary.indexOf (Go)        500000           10            2
----------------------------------------------------------------------
total                                                            5633

----------------------------------------------------------------------
Parallel comparison (workers = 4, scheduler = tinygo)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                     2694         2676         1.01
newton sqrt batch                       151          161         0.94
monte carlo pi                          536          488         1.10
fib(N) x workers                        112          124         0.90
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
| `fib(N) recursive`             |        28 |      42 | *0.7x* | Call-heavy dispatch; TinyGo's tighter loop edges it. Both effectively free (sub-50 ms) thanks to the per-frame allocation elimination. |
| `primes up to LIMIT`           |      2694 |    3562 | *0.8x* | The long integer loop, the single biggest row. TinyGo leads it by ~0.9 s.      |
| `newton sqrt batch`            |       151 |     175 | *0.9x* | Float arithmetic + dispatch; TinyGo slightly ahead.                            |
| `monte carlo pi`               |       536 |     595 | *0.9x* | Float arithmetic + RNG calls; TinyGo slightly ahead.                           |
| `list sort/reverse/slice`      |      1410 |     987 | 1.4x   | Allocation-heavy. Go's concurrent GC absorbs the sort/reverse/slice churn better than TinyGo's simpler collector - Go's largest serial win (~420 ms). |
| `struct list build+read`       |        30 |      24 | 1.3x   | Append hot loop is O(1); both effectively free (sub-40 ms).                    |
| `string join`                  |        14 |       8 | 1.8x   | Build-up-a-string pattern is O(1); both free (sub-20 ms).                      |
| `map insert+read`              |        49 |      33 | 1.5x   | The advisory map hash index keeps keyed insert+read O(1); both sub-50 ms.      |
| `byte scan naive (per-byte)`   |       719 |     864 | *0.8x* | A per-byte tree-walker scan of ~500 KB x 10 - the cost the `binary` library exists to remove. TinyGo leads it by ~0.15 s. |
| `byte scan binary.indexOf (Go)`|         2 |       0 | -      | The same scan through one `binary.indexOf` call (Go `bytes.Index`, SIMD). Both effectively **0 ms** - see below. |
| **total**                      |      5633 |    6290 | *0.9x* | TinyGo posts the lower serial total (~10%), carried mainly by `primes`.        |

TinyGo takes the serial section (5.6 s vs 6.3 s, ~10% lower) on its tighter
dispatch loop: it leads every compute row - `primes` (-0.9 s), the naive
`byte scan` (-0.15 s), `monte carlo`, `newton` and `fib`. Go wins the
allocation-heavy `list` row (+420 ms) and the small structural rows (`struct` /
`string` / `map`, all sub-50 ms). **Without the two byte-scan rows the totals are
4912 ms (TinyGo) vs 5426 ms (Go)**, so TinyGo's serial lead is not the byte
scan. (The milliseconds shift run to run; what holds is that the builds trade
wins by workload shape - TinyGo the tight compute loops, Go the allocation churn
and the parallel section below.)

## The bulk-byte rows

The two `byte scan` rows are the `binary` library's throughput demonstration:
the same search for a 6-byte needle at the **end** of a ~500 KB buffer, 10 times
(so every scan traverses the whole buffer).

- **Naive, per byte** (a `.j` loop comparing bytes one at a time): 864 ms (Go) /
  719 ms (TinyGo). This is the per-byte tree-walker cost byte-oriented code pays.
- **`binary.indexOf`** (one call into Go's assembly/SIMD `bytes.Index`): **0 ms**
  (Go) / 2 ms (TinyGo).

The Go row reads `0` because it is sub-millisecond: ~500 KB x 10 = ~5 MB
scanned at ~20 GB/s takes a couple hundred microseconds, and the suite reports
whole milliseconds (TinyGo's `2` is the same work, just above the 1 ms rounding
line). Pushing the per-byte loop into one Go call is a ~3,500x speedup on this
workload.

## v1 parallel section

Speedup is `serial_ms / par_ms`; > 1.0 means the four-worker version beat serial.
Go gets real multi-core speedup; TinyGo's cooperative scheduler stays at ~1.0 by
design.

| Workload             | Go serial (ms) | Go par (ms) | Go speedup | TinyGo serial (ms) | TinyGo par (ms) | TinyGo speedup |
| -------------------- | -------------- | ----------- | ---------- | ------------------ | --------------- | -------------- |
| `primes up to LIMIT` |           3562 |        1229 |   **2.90** |               2694 |            2676 |      1.01      |
| `newton sqrt batch`  |            175 |          48 |   **3.65** |                151 |             161 |      0.94      |
| `monte carlo pi`     |            595 |         168 |   **3.54** |                536 |             488 |      1.10      |
| `fib(N) x workers`   |            168 |          41 |   **4.10** |                112 |             124 |      0.90      |

Go reaches 2.9x-4.1x on four workers (the 4.10 is run-to-run noise around a
linear 4.0). `primes` scales worst, and that is the partition, not contention:
trial division costs about `sqrt(n)` per number, so the worker holding the top
quarter of the range (75 001-100 000) does ~35% of the work and caps the speedup
near 2.85x. `jennifer-tiny` pins the cooperative scheduler, so `spawn` there is
concurrency without multi-core throughput: its column sits at ~1.0. Use the
default binary when parallel throughput matters.

**The parallel section reverses the end-to-end picture.** TinyGo has the lower
*serial* total (5.6 s vs 6.3 s), but Go crunches the parallel section in
**~1.5 s** (1229 + 48 + 168 + 41) where TinyGo takes **~3.4 s** (no
parallelism). On the wall clock the full `--suite all` run finished in
**10.16 s on Go vs 13.44 s on TinyGo** (see the memory table below): a lower
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
stats normal cdf                      50000        50000           68
math regGammaP                        50000        50000           81
linalg matmul NxN                        30          300           23
linalg solve NxN                         30          300            6
ml linreg fit+predict                   500          200           23
ml kMeans cluster                       500           40            7
----------------------------------------------------------------------
total                                                             208
```

```
=== Jennifer benchmark suite (v2 within --suite all, jennifer-tiny) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
stats normal cdf                      50000        50000           82
math regGammaP                        50000        50000          102
linalg matmul NxN                        30          300           27
linalg solve NxN                         30          300            5
ml linreg fit+predict                   500          200           27
ml kMeans cluster                       500           40            5
----------------------------------------------------------------------
total                                                             248
```

Go is ahead on v2 (208 ms vs 248 ms, ~16% lower) and wins every row but the two
smallest, `linalg solve` (TinyGo 5 ms vs 6 ms) and `ml kMeans` (5 ms vs 7 ms);
in absolute terms the gap is ~40 ms over the whole suite. The distribution rows
(`stats normal cdf`, `math regGammaP`) dominate the total (149 of 208 ms on Go,
184 of 248 ms on TinyGo) because they are the interpreter-heavy ones: 50 000
tree-walked iterations each, one Go call per iteration. The `linalg` / `ml` rows
are cheap despite doing matrix work: the compute is one Go call over a matrix
built once outside the timed loop, so the per-iteration cost is mostly list
marshaling, and the Go-side algorithms (matmul, Gaussian solve, normal-equations
fit) are far below the dispatch cost. The tree-walker overhead is in the
per-call loop around the numeric libraries, not the math.

## Suite v3 - value semantics

v1 and v2 are dominated by dispatch loops and Go-side compute, so they barely
register what the value model costs. v3 measures that cost directly. Every row moves a `Grid` - a struct
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
read-only borrow off run-wide.)

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
owned update g = f(g)                 10000        20000           35
copy-then-write update                10000         2000          907
snapshot + write (detach)             10000         2000          895
borrowed read param                   10000        20000           25
list append helper                    10000        10000           13
nested field write                     1000           50          130
spawn capture                         10000          200           97
----------------------------------------------------------------------
total                                                            2102
```

```
=== Jennifer benchmark suite (v3 within --suite all, jennifer-tiny) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
owned update g = f(g)                 10000        20000           36
copy-then-write update                10000         2000         1705
snapshot + write (detach)             10000         2000         1726
borrowed read param                   10000        20000           16
list append helper                    10000        10000           21
nested field write                     1000           50          155
spawn capture                         10000          200          375
----------------------------------------------------------------------
total                                                            4034
```

Per operation on the reference machine (Go / TinyGo), and the cross-build ratio
`tiny_ms / go_ms` as in the v1 table:

| Workload                    | per operation        | Ratio  | What it shows                                                     |
| --------------------------- | -------------------- | ------ | ----------------------------------------------------------------- |
| `owned update g = f(g)`     | 1.8 / 1.8 us a call  | 1.0x   | Runs in place: about **250x cheaper than one copy** of the grid.  |
| `copy-then-write update`    | 0.45 / 0.85 ms a call | 1.9x  | Costs the same as the forced copy below: the defensive local copy is one full copy per call, so it is the idiom to avoid. |
| `snapshot + write (detach)` | 0.45 / 0.86 ms a copy | 1.9x  | The price of one full copy of a 10 000-cell grid: the baseline.   |
| `borrowed read param`       | 1.3 / 0.8 us a call  | *0.6x* | No copy: a read-only parameter is borrowed.                       |
| `list append helper`        | 1.3 / 2.1 us a call  | 1.6x   | Runs in place: the L205 accumulator is linear when called as `$xs = appendTo($xs, v)`. |
| `nested field write`        | 2.6 / 3.1 us a step  | 1.2x   | Two deep path writes per step (50 passes over 1 000 units), no copy of the list. |
| `spawn capture`             | 0.49 / 1.9 ms a task | 3.9x   | On Go, about one grid copy per task: the snapshot is a full copy. TinyGo adds roughly 1.4 ms of per-task overhead on top. |
| **total**                   |                      | 1.9x   |                                                                   |

The value model behaves as designed on both builds: every path that may avoid a
copy does (owned update, borrowed read, append helper, nested write), and the
two paths that must copy cost exactly one copy each. Where the builds differ is
the copy itself and task start-up. A copy is ~1.9x slower on TinyGo here, and
each `spawn` costs more under its cooperative scheduler with a fixed 4 MB stack
per task (see [tinygo.md](tinygo.md), which also covers the cost of many tasks
alive at once). Both TinyGo gaps are wider in a `--suite all` run than in a
standalone `--suite v3` run (which has measured ~1.3x on the copy rows and ~2.7x
on `spawn capture`): after v1 and v2 the TinyGo heap is larger, and its simpler
collector pays for that on every allocation-heavy row.

## Memory, page faults, and wall clock

GNU `/bin/time -v` on the `--suite all` run (v1 + v2 + v3) on the reference
machine, version `0.24.0-dev+170.be4bfbb`:

| Metric                | `jennifer` (Go)  | `jennifer-tiny` (TinyGo) |
| --------------------- | ---------------- | ------------------------ |
| wall clock (real)     | 10.16 s          | 13.44 s                  |
| user / system         | 16.12 s / 0.38 s | 13.32 s / 0.07 s         |
| CPU                   | 162%             | 99%                      |
| peak resident (RSS)   | ~50 MB           | ~136 MB                  |
| minor page faults     | ~114,600         | ~39,800                  |
| major page faults     | 0                | 0                        |
| context switches (voluntary / involuntary) | ~89,100 / ~1,800 | 1 / ~530 |

- **Go finishes faster end-to-end** (10.16 s vs 13.44 s) while spending more
  CPU-seconds doing it (16.1 s user at 162% CPU). The gap between `user` and
  `real` is the concurrent GC on other cores plus the four `spawn` workers running
  in parallel during the parallel section. TinyGo's `user ~= real` at 99% CPU
  (and a single voluntary context switch) shows single-thread execution: the
  cooperative scheduler runs every goroutine on one OS thread.
- **TinyGo uses ~2.7x the peak RSS** (~136 MB vs ~50 MB). Its cooperative
  scheduler reserves each goroutine's full `-stack-size` (4 MB, sized above the
  catchable call-depth cap) up front, so spawned workers hold reserved stack
  while they are alive, and its simpler collector lets the heap grow further
  between collections during the v3 copy rows. Go grows goroutine stacks on
  demand from ~8 KB, so its footprint stays small and flat.
- **Go takes ~2.9x the minor page faults** (~114.6k vs ~39.8k), most likely
  from the v3 copy rows (each a full 10 000-cell grid): Go keeps a small heap and
  hands pages back between collections, so it touches fresh pages again, where
  TinyGo holds on to the larger heap it already touched. Neither build takes any
  major faults.

## Picking a binary

- **Single-thread compute:** the two builds trade wins by workload shape. TinyGo
  leads the tight compute loops (`primes`, `fib`, `newton`, `monte carlo`, the
  naive byte scan); Go leads the allocation-heavy `list` row and the small
  structural rows. Which build posts the lower serial *total* shifts run to run.
- **End-to-end wall clock / any `spawn` parallelism:** the default **`jennifer`**,
  the only build with multi-core throughput (2.9x-4.1x here); it finishes the
  whole suite faster despite a higher serial total.
- **Footprint:** `jennifer` for a small, on-demand memory profile (~50 MB peak);
  `jennifer-tiny` trades ~2.7x the RSS for its smaller *binary* and embeddability,
  not for a smaller runtime footprint.
- **Value-heavy work:** copies and `spawn` snapshots cost ~1.9x (copies) to ~3.9x
  (spawn) more on `jennifer-tiny`; the copy-avoiding paths (owned update,
  borrowed read) cost the same or less.
- **Byte-oriented work:** reach for the `binary` library (and `net.readAll` /
  `readN`) on either build - the `binary.indexOf` row shows a per-byte loop
  collapsing to effectively free.
- **Numeric work:** `stats` / `linalg` / `math` / `ml` cost about the same on
  both builds (Go ~16% lower on v2 here, ~40 ms in absolute terms); the per-call
  dispatch loop dominates, not the Go-side math.
