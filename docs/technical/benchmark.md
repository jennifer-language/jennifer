# Benchmarks

Single-binary throughput for the two builds - `jennifer` (standard Go, the
default) and `jennifer-tiny` (TinyGo, the constrained variant) - measured with
`examples/benchmark.j`.

Reference numbers are from `examples/benchmark.j` on an **AMD Ryzen 5
7600X3D** (6 cores, 12 threads; bare metal, desktop idle) - the machine the
suite prints in its own header, from `os.NCPU` plus a `/proc/cpuinfo` read done
in Jennifer. The numbers are machine-specific; the **ratios and the shape** of
the comparison are the stable part. Every figure below comes from one
`--suite all` run of each binary at version `0.27.0-dev+60.8fbf5a01` (v1 to v4 in
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
version:  0.27.0-dev+60.8fbf5a01
cpu:      AMD Ryzen 5 7600X3D 6-Core Processor (12 cores)
platform: linux/amd64
host:     bare metal (no VM / container hints)

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           26
primes up to LIMIT                   100000            1         2387
newton sqrt batch                     10000        10000           91
monte carlo pi                       500000       500000          368
list sort/reverse/slice               10000          500          316
struct list build+read                10000        10000           14
string join                           10000        10000            5
map insert+read                       10000        10000           22
byte scan naive (per-byte)           500000           10          628
byte scan binary.indexOf (Go)        500000           10            0
----------------------------------------------------------------------
total                                                            3857

----------------------------------------------------------------------
Parallel comparison (workers = 4, scheduler = go)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                     2387          969         2.46
newton sqrt batch                        91           25         3.64
monte carlo pi                          368          133         2.77
fib(N) x workers                        104           28         3.71
----------------------------------------------------------------------
```

Go runs the serial section single-threaded while its concurrent GC works on other
cores, then reaches multi-core throughput in the parallel section (four
`spawn` workers on separate OS threads).

## Suite v1 - `jennifer-tiny` (TinyGo binary)

```
=== Jennifer benchmark suite ===
build:    tinygo
version:  0.27.0-dev+60.8fbf5a01
cpu:      AMD Ryzen 5 7600X3D 6-Core Processor (1 cores)
platform: linux/amd64
host:     bare metal (no VM / container hints)

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           15
primes up to LIMIT                   100000            1         1722
newton sqrt batch                     10000        10000           60
monte carlo pi                       500000       500000          273
list sort/reverse/slice               10000          500          375
struct list build+read                10000        10000           13
string join                           10000        10000            3
map insert+read                       10000        10000           17
byte scan naive (per-byte)           500000           10          521
byte scan binary.indexOf (Go)        500000           10            2
----------------------------------------------------------------------
total                                                            3001

----------------------------------------------------------------------
Parallel comparison (workers = 4, scheduler = tinygo)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                     1722         1697         1.01
newton sqrt batch                        60           66         0.91
monte carlo pi                          273          248         1.10
fib(N) x workers                         60           61         0.98
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
| `fib(N) recursive`             |        15 |      26 | *0.6x* | Call-heavy dispatch; both effectively free.                                    |
| `primes up to LIMIT`           |      1722 |    2387 | *0.7x* | The long integer loop, the single biggest row. TinyGo leads it by ~0.7 s.      |
| `newton sqrt batch`            |        60 |      91 | *0.7x* | Float arithmetic + dispatch.                                                   |
| `monte carlo pi`               |       273 |     368 | *0.7x* | Float arithmetic + RNG calls.                                                  |
| `list sort/reverse/slice`      |       375 |     316 | 1.2x   | Allocation-heavy: Go's concurrent GC absorbs the sort/reverse/slice churn better; Go's only clear serial win. |
| `struct list build+read`       |        13 |      14 | *0.9x* | Append hot loop is O(1); both effectively free.                                |
| `string join`                  |         3 |       5 | *0.6x* | Build-up-a-string pattern is O(1); both free.                                  |
| `map insert+read`              |        17 |      22 | *0.8x* | The advisory map hash index keeps keyed insert+read O(1).                      |
| `byte scan naive (per-byte)`   |       521 |     628 | *0.8x* | A per-byte tree-walker scan of ~500 KB x 10 - the cost the `binary` library exists to remove. |
| `byte scan binary.indexOf (Go)`|         2 |       0 | -      | The same scan through one `binary.indexOf` call (Go `bytes.Index`, SIMD). Both effectively **0 ms** - see below. |
| **total**                      |      3001 |    3857 | *0.8x* | TinyGo posts the lower serial total (~22%).                                    |

TinyGo takes the serial section (3.0 s vs 3.9 s) on its tighter dispatch loop
and leads every compute row; Go wins only the allocation-heavy `list` row.
**Without the two byte-scan rows the totals are 2478 ms (TinyGo) vs 3229 ms
(Go)**, so TinyGo's lead is not the byte scan. (The milliseconds shift run to
run; what holds is that the builds trade wins by workload shape - TinyGo the
tight compute loops, Go the allocation churn and the parallel section below.)

## The bulk-byte rows

The two `byte scan` rows are the `binary` library's throughput demonstration:
the same search for a 6-byte needle at the **end** of a ~500 KB buffer, 10 times
(so every scan traverses the whole buffer).

- **Naive, per byte** (a `.j` loop comparing bytes one at a time): 628 ms (Go) /
  521 ms (TinyGo). This is the per-byte tree-walker cost byte-oriented code pays.
- **`binary.indexOf`** (one call into Go's assembly/SIMD `bytes.Index`): **0 ms**
  (Go) / 2 ms (TinyGo).

The Go row reads `0` because it is sub-millisecond: ~500 KB x 10 = ~5 MB
scanned at ~20 GB/s takes a couple hundred microseconds, and the suite reports
whole milliseconds (TinyGo's `2` is the same work at its rounding line).
Pushing the per-byte loop into one Go call is a ~2,500x speedup on this
workload.

## v1 parallel section

Speedup is `serial_ms / par_ms`; > 1.0 means the four-worker version beat serial.
Go gets real multi-core speedup; TinyGo's cooperative scheduler stays at ~1.0 by
design.

| Workload             | Go serial (ms) | Go par (ms) | Go speedup | TinyGo serial (ms) | TinyGo par (ms) | TinyGo speedup |
| -------------------- | -------------- | ----------- | ---------- | ------------------ | --------------- | -------------- |
| `primes up to LIMIT` |           2387 |         969 |   **2.46** |               1722 |            1697 |      1.01      |
| `newton sqrt batch`  |             91 |          25 |   **3.64** |                 60 |              66 |      0.91      |
| `monte carlo pi`     |            368 |         133 |   **2.77** |                273 |             248 |      1.10      |
| `fib(N) x workers`   |            104 |          28 |   **3.71** |                 60 |              61 |      0.98      |

Go reaches 2.5x-3.7x on four workers. `primes` scales worst, and that is the
partition, not contention: trial division costs about `sqrt(n)` per number, so
the worker holding the top quarter of the range (75 001-100 000) does ~35% of
the work and caps the speedup near 2.85x; spawn and scheduling overhead take it
to the measured 2.46x. `monte carlo` (2.77x) has the most per-iteration builtin
calls and so the most GC and allocation contention between workers.
`jennifer-tiny` pins the cooperative scheduler, so `spawn` there is
concurrency without multi-core throughput: its column sits at ~1.0. Use the
default binary when parallel throughput matters.

**The parallel section narrows the end-to-end picture.** TinyGo has the lower
*serial* total (3.0 s vs 3.9 s), but Go crunches the parallel section in
**~1.2 s** (969 + 25 + 133 + 28) where TinyGo takes **~2.1 s** (no
parallelism). On the wall clock the full `--suite all` run finished in
**6.13 s on Go vs 6.55 s on TinyGo** (see the memory table below).

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
stats normal cdf                      50000        50000           40
math regGammaP                        50000        50000           58
linalg matmul NxN                        30          300           11
linalg solve NxN                         30          300            8
ml linreg fit+predict                   500          200           14
ml kMeans cluster                       500           40            7
----------------------------------------------------------------------
total                                                             138
```

```
=== Jennifer benchmark suite (v2 within --suite all, jennifer-tiny) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
stats normal cdf                      50000        50000           32
math regGammaP                        50000        50000           62
linalg matmul NxN                        30          300           18
linalg solve NxN                         30          300            4
ml linreg fit+predict                   500          200           14
ml kMeans cluster                       500           40            6
----------------------------------------------------------------------
total                                                             136
```

The two builds tie on v2 (138 ms vs 136 ms); no row differs by more than
8 ms. The distribution rows (`stats normal cdf`, `math regGammaP`) dominate
the total (98 of 138 ms on Go, 94 of 136 ms on TinyGo) because they are the
interpreter-heavy ones: 50 000 tree-walked iterations each, one Go call per
iteration. The `linalg` / `ml` rows are cheap despite doing matrix work: the
compute is one Go call over a matrix built once outside the timed loop, so the
per-iteration cost is mostly list marshaling, and the Go-side algorithms
(matmul, Gaussian solve, normal-equations fit) are far below the dispatch cost.
The tree-walker overhead is in the per-call loop around the numeric libraries,
not the math.

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
owned update g = f(g)                 10000        20000           22
copy-then-write update                10000         2000          194
snapshot + write (detach)             10000         2000          188
borrowed read param                   10000        20000           14
list append helper                    10000        10000            8
nested field write                     1000           50           72
spawn capture                         10000          200           21
----------------------------------------------------------------------
total                                                             519
```

```
=== Jennifer benchmark suite (v3 within --suite all, jennifer-tiny) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
owned update g = f(g)                 10000        20000           14
copy-then-write update                10000         2000          218
snapshot + write (detach)             10000         2000          230
borrowed read param                   10000        20000            7
list append helper                    10000        10000            4
nested field write                     1000           50           66
spawn capture                         10000          200          172
----------------------------------------------------------------------
total                                                             711
```

Per operation on the reference machine (Go / TinyGo), and the cross-build ratio
`tiny_ms / go_ms` as in the v1 table:

| Workload                    | per operation        | Ratio  | What it shows                                                     |
| --------------------------- | -------------------- | ------ | ----------------------------------------------------------------- |
| `owned update g = f(g)`     | 1.1 / 0.7 us a call  | *0.6x* | Runs in place: about **85x cheaper than one copy** of the grid on Go, ~160x on TinyGo. |
| `copy-then-write update`    | 0.10 / 0.11 ms a call | 1.1x  | Costs about the same as the forced copy below: the defensive local copy is one full copy per call, so it is the idiom to avoid. |
| `snapshot + write (detach)` | 0.09 / 0.12 ms a copy | 1.2x  | The price of one full copy of a 10 000-cell grid: the baseline.   |
| `borrowed read param`       | 0.7 / 0.4 us a call  | *0.5x* | No copy: a read-only parameter is borrowed.                       |
| `list append helper`        | 0.8 / 0.4 us a call  | *0.5x* | Runs in place: the L205 accumulator is linear when called as `$xs = appendTo($xs, v)`. |
| `nested field write`        | 1.4 / 1.3 us a step  | *0.9x* | Two deep path writes per step (50 passes over 1 000 units), no copy of the list. |
| `spawn capture`             | 0.11 / 0.86 ms a task | 8.2x  | On Go, about one grid copy per task: the snapshot is a full copy. TinyGo adds roughly 0.75 ms of per-task overhead on top. |
| **total**                   |                      | 1.4x   |                                                                   |

The value model behaves as designed on both builds: every path that may avoid a
copy does (owned update, borrowed read, append helper, nested write), and the
two paths that must copy cost exactly one copy each. With the 48-byte value
(M29.11) a full copy of the grid costs about 0.09 ms on Go and 0.12 ms on
TinyGo, so the copy rows are now within ~1.2x of each other across builds. The
remaining gap is task start-up: each `spawn` costs more under TinyGo's
cooperative scheduler with a fixed 4 MB stack per task (see
[tinygo.md](tinygo.md), which also covers the cost of many tasks alive at once).

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
| `module handle call`   | a 10 000-node tree struct declared in `examples/benchmark/tree.j`, passed to that module's one-lookup `has`, 200 calls: a module call with a large argument, which since M29.14 crosses without a rebuild. |

Read `nested read param` against `wrapped nested read`: the same work, and the
ratio is the cost of the copy. The `sortBy` row reads against v3's
`borrowed read param` the same way. When one of the gaps closes, its row drops
to the control's level and the suite records it. `jennifer profile --allocs
examples/benchmark.j --suite v4` shows the copies (eager copies) by call
site.

```
=== Jennifer benchmark suite (v4 within --suite all, jennifer) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
nested read param                     10000         2000          199
wrapped nested read                   10000         2000            1
sortBy callee read                    10000         2000          193
module handle call                    10000          200            0
----------------------------------------------------------------------
total                                                             393
```

```
=== Jennifer benchmark suite (v4 within --suite all, jennifer-tiny) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
nested read param                     10000         2000          308
wrapped nested read                   10000         2000            0
sortBy callee read                    10000         2000          241
module handle call                    10000          200            0
----------------------------------------------------------------------
total                                                             549
```

| Workload               | per call (Go / TinyGo) | Ratio  | What it shows |
| ---------------------- | ---------------------- | ------ | ------------- |
| `nested read param`    | 0.10 / 0.15 ms         | 1.5x   | One full copy of the 100 x 100 matrix per call, ~200x the wrapped control on Go. |
| `wrapped nested read`  | <1 / <1 us             | -      | Borrowed: the same read through a struct parameter is effectively free. |
| `sortBy callee read`   | 0.10 / 0.12 ms         | 1.2x   | About one v3 grid copy per call, against ~0.7 us for v3's `borrowed read param`: the callback builtin turns the borrow off. |
| `module handle call`   | <2.5 / <2.5 us         | -      | A module call with a 10 000-node tree argument: no rebuild at the boundary (M29.14), so the 200 calls round to 0 ms on both builds. |
| **total**              |                        | 1.4x   | |

The two copy gaps (M30.3, M30.8) still cost one full copy per call on both
builds; the module-boundary row has reached its control. TinyGo pays 1.2x-1.5x
more per copy, in line with v3.

## Memory, page faults, and wall clock

GNU `time` on the `--suite all` run (v1 + v2 + v3 + v4) on the reference
machine, version `0.27.0-dev+60.8fbf5a01`:

| Metric                | `jennifer` (Go)  | `jennifer-tiny` (TinyGo) |
| --------------------- | ---------------- | ------------------------ |
| wall clock (real)     | 6.13 s           | 6.55 s                   |
| user / system         | 10.20 s / 0.44 s | 6.48 s / 0.05 s          |
| CPU                   | 173%             | 99%                      |
| peak resident (RSS)   | ~28 MB           | ~104 MB                  |
| minor page faults     | ~171,400         | ~24,700                  |
| major page faults     | 0                | 0                        |

- **Go finishes slightly faster end-to-end** (6.13 s vs 6.55 s) while spending
  more CPU-seconds doing it (10.2 s user at 173% CPU). The gap between `user`
  and `real` is the concurrent GC on other cores plus the four `spawn` workers
  running in parallel during the parallel section. TinyGo's `user ~= real` at
  99% CPU shows single-thread execution: the cooperative scheduler runs every
  goroutine on one OS thread.
- **TinyGo uses ~3.7x the peak RSS** (~104 MB vs ~28 MB). Its cooperative
  scheduler reserves each goroutine's full `-stack-size` (4 MB, sized above the
  catchable call-depth cap) up front, so spawned workers hold reserved stack
  while they are alive, and its simpler collector lets the heap grow further
  between collections. Go grows goroutine stacks on demand from ~8 KB, so its
  footprint stays small and flat.
- **Go takes ~6.9x the minor page faults** (~171k vs ~25k): Go keeps a small
  heap and hands pages back between collections, so it touches fresh pages
  again, where TinyGo holds on to the larger heap it already touched. Neither
  build takes any major faults.

### Against the 0.27.0 release

The same `--suite all` run of the 0.27.0 release binaries (its own
`examples/benchmark.j`; the v1 to v4 workloads are frozen, so the rows are
the same work), on the same machine and toolchain:

| Suite / metric | Go 0.27.0 | Go now | TinyGo 0.27.0 | TinyGo now |
| --- | --- | --- | --- | --- |
| v1 serial total | 6374 ms | 3857 ms | 6565 ms | 3001 ms |
| v1 parallel section | 1665 ms | 1155 ms | 3800 ms | 2072 ms |
| v2 total | 253 ms | 138 ms | 284 ms | 136 ms |
| v3 total | 1195 ms | 519 ms | 4052 ms | 711 ms |
| v4 total | 1916 ms | 393 ms | 5619 ms | 549 ms |
| wall clock | 11.54 s | 6.13 s | 20.53 s | 6.55 s |
| user CPU | 22.16 s | 10.20 s | 20.17 s | 6.48 s |
| peak RSS | ~79 MB | ~28 MB | ~263 MB | ~104 MB |
| minor page faults | ~285,900 | ~171,400 | ~93,300 | ~24,700 |

The 48-byte value (M29.11) accounts for most of it: smaller copies (v3, v4: a
full grid copy went from ~0.20 to ~0.09 ms on Go and from ~0.85 to ~0.12 ms on
TinyGo), less allocation and GC work (v1's `list` row 647 to 316 ms on Go and
2003 to 375 ms on TinyGo; v2) and about a third of the memory. The one identity
per module struct (M29.14) takes the v4 `module handle call` row from 712 ms
(Go) and 1659 ms (TinyGo) to zero. TinyGo gains the most: its simpler collector
paid for every oversized value, so the whole run is 3.1x faster there, against
1.9x on Go.

## Picking a binary

- **Single-thread compute:** `jennifer-tiny` leads every compute row of v1
  (~0.6x-0.8x the time) and posts the lower serial total; Go leads the
  allocation-heavy `list` row (1.2x).
- **End-to-end wall clock / any `spawn` parallelism:** the default **`jennifer`**,
  the only build with multi-core throughput (2.5x-3.7x here); it finishes the
  whole suite slightly faster despite a higher serial total.
- **Footprint:** `jennifer` for a small, on-demand memory profile (~28 MB peak);
  `jennifer-tiny` trades ~3.7x the RSS for its smaller *binary* and
  embeddability, not for a smaller runtime footprint.
- **Value-heavy work:** copies cost ~1.1-1.5x more on `jennifer-tiny`, `spawn`
  snapshots ~8x; the copy-avoiding paths (owned update, borrowed read, append
  helper) cost the same or less.
- **Byte-oriented work:** reach for the `binary` library (and `net.readAll` /
  `readN`) on either build - the `binary.indexOf` row shows a per-byte loop
  collapsing to effectively free.
- **Numeric work:** `stats` / `linalg` / `math` / `ml` cost the same on both
  builds (138 ms vs 136 ms); the per-call dispatch loop dominates, not the
  Go-side math.
