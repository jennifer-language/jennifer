# Benchmarks

Single-binary throughput for the two builds - `jennifer` (standard Go, the
default) and `jennifer-tiny` (TinyGo, the constrained variant) - measured with
`examples/benchmark.j`.

Reference numbers are from `examples/benchmark.j` on an **AMD Ryzen 5
7600X3D** (6 cores, 12 threads; bare metal, desktop idle) - the machine the
suite prints in its own header, from `os.NCPU` plus a `/proc/cpuinfo` read done
in Jennifer. The numbers are machine-specific; the **ratios and the shape** of
the comparison are the stable part, not the absolute milliseconds. The suite v1,
v2 and v3 runs of both binaries are from version `0.24.0-dev+149.3222bd4`, so the
cross-build comparisons below are like for like, and so is the memory /
wall-clock table. Both binaries were built on the reference machine with **Go
1.27.2** (`jennifer`) and **TinyGo 0.43.0** on Go 1.27.2 and LLVM 22.1.4
(`jennifer-tiny`); the toolchain is part of what is measured, so a comparison
with an older series compares toolchains as well as interpreter versions.

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
hash index, per-frame allocation elimination (slot-backed bindings), and the
`binary` bulk-byte primitives - so append-in-a-loop is amortised O(N) with
in-place growth, keyed map access is O(1), a call frame allocates nothing per
binding, and a byte scan can drop into a single Go call.

> These are reference-machine figures, recorded from a supplied run, **not**
> produced in CI - benchmark timings are too machine- and load-sensitive to pin
> in a test. Regenerate them on the reference machine after a perf-relevant
> change (`jennifer run examples/benchmark.j --suite all --format json`); do not
> run the suite on other hardware for documentation numbers.

## Suite v1 - `jennifer` (standard-Go binary, default)

```
=== Jennifer benchmark suite ===
build:    go
version:  0.24.0-dev+149.3222bd4
cpu:      AMD Ryzen 5 7600X3D 6-Core Processor (12 cores)
platform: linux/amd64
host:     bare metal (no VM / container hints)

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           58
primes up to LIMIT                   100000            1        17096
newton sqrt batch                     10000        10000          283
monte carlo pi                       500000       500000          768
list sort/reverse/slice               10000          500          974
struct list build+read                10000        10000           30
string join                           10000        10000           10
map insert+read                       10000        10000           39
byte scan naive (per-byte)           500000           10         6061
byte scan binary.indexOf (Go)        500000           10            0
----------------------------------------------------------------------
total                                                           25319

----------------------------------------------------------------------
Parallel comparison (workers = 4, scheduler = go)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                    17096         6137         2.79
newton sqrt batch                       283           76         3.72
monte carlo pi                          768          225         3.41
fib(N) x workers                        232           59         3.93
----------------------------------------------------------------------
```

Go runs the serial section single-threaded while its concurrent GC works on other
cores, then reaches genuine multi-core throughput in the parallel section (four
`spawn` workers on separate OS threads).

## Suite v1 - `jennifer-tiny` (TinyGo binary)

```
=== Jennifer benchmark suite ===
build:    tinygo
version:  0.24.0-dev+149.3222bd4
cpu:      AMD Ryzen 5 7600X3D 6-Core Processor (1 cores)
platform: linux/amd64
host:     bare metal (no VM / container hints)

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           53
primes up to LIMIT                   100000            1        14951
newton sqrt batch                     10000        10000          278
monte carlo pi                       500000       500000          822
list sort/reverse/slice               10000          500         1067
struct list build+read                10000        10000           35
string join                           10000        10000           11
map insert+read                       10000        10000           43
byte scan naive (per-byte)           500000           10         4099
byte scan binary.indexOf (Go)        500000           10            2
----------------------------------------------------------------------
total                                                           21361

----------------------------------------------------------------------
Parallel comparison (workers = 4, scheduler = tinygo)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                    14951        14954         1.00
newton sqrt batch                       278          291         0.96
monte carlo pi                          822          796         1.03
fib(N) x workers                        212          224         0.95
----------------------------------------------------------------------
```

`os.NCPU` reports `1` here - honest about the cooperative `-scheduler=tasks`
build's usable parallelism, not the 12 threads the machine has. Every goroutine
runs on one OS thread, so the parallel column hovers at ~1.0 by design, dipping
slightly below on rows where spawn setup has no parallel payoff to offset it.

## v1 per-workload comparison (serial section)

Ratios are `tiny_ms / go_ms`; > 1.0 means `jennifer-tiny` is slower, < 1.0
(shown in *italics*) means it is faster. Both columns are from
`0.24.0-dev+149.3222bd4`.

| Workload                       | tiny (ms) | go (ms) | Ratio  | Where the time goes                                                            |
| ------------------------------ | --------- | ------- | ------ | ------------------------------------------------------------------------------ |
| `fib(N) recursive`             |        53 |      58 | *0.9x* | Call-heavy dispatch; TinyGo's tighter loop edges it. Both effectively free (sub-60 ms) thanks to the per-frame allocation elimination. |
| `primes up to LIMIT`           |     14951 |   17096 | *0.9x* | The long numeric dispatch loop, the single biggest row. TinyGo leads it by ~2.1 s. |
| `newton sqrt batch`            |       278 |     283 | 1.0x   | Float arithmetic + dispatch; a dead heat.                                      |
| `monte carlo pi`               |       822 |     768 | 1.1x   | Float arithmetic + RNG calls; Go slightly ahead.                               |
| `list sort/reverse/slice`      |      1067 |     974 | 1.1x   | Allocation-heavy. Go's concurrent GC absorbs the sort/reverse/slice churn better than TinyGo's simpler collector - Go's largest serial win, though a narrow one (~90 ms). |
| `struct list build+read`       |        35 |      30 | 1.2x   | Append hot loop is O(1); both effectively free (sub-40 ms).                    |
| `string join`                  |        11 |      10 | 1.1x   | Build-up-a-string pattern is O(1); both free (sub-20 ms).                      |
| `map insert+read`              |        43 |      39 | 1.1x   | The advisory map hash index keeps keyed insert+read O(1); both sub-50 ms.      |
| `byte scan naive (per-byte)`   |      4099 |    6061 | *0.7x* | A per-byte tree-walker scan of ~500 KB x 10 - the cost the `binary` library exists to remove. TinyGo's tighter dispatch wins it by ~2.0 s. |
| `byte scan binary.indexOf (Go)`|         2 |       0 | -      | The same scan through one `binary.indexOf` call (Go `bytes.Index`, SIMD). Both effectively **0 ms** - see below. |
| **total**                      |     21361 |   25319 | *0.8x* | TinyGo posts the lower serial total (~16%), carried mainly by `primes` and the naive byte-scan row. |

TinyGo takes the serial section clearly (21.4 s vs 25.3 s, ~16% lower), where
its tighter dispatch loop leads the long numeric workloads - `primes` (-2.1 s)
and the naive `byte scan` (-2.0 s), plus `fib`. Go's wins are the
allocation-heavy `list` row (+93 ms), `monte carlo` (+54 ms) and the tiny
structural rows (`struct` / `string` / `map`, all sub-50 ms); `newton` is a dead
heat. **Strip the two byte-scan rows and the classic-workload totals are
17260 ms (TinyGo) vs 19258 ms (Go)** - TinyGo still ahead, so its serial lead is
not just the byte scan. (The absolute milliseconds shift run to run; the durable
signal is that the two builds trade wins by workload shape - TinyGo the tight
compute loops, Go the allocation churn and the parallel section below.)

## The bulk-byte rows

The two `byte scan` rows are the `binary` library's throughput demonstration:
the same search for a 6-byte needle at the **end** of a ~500 KB buffer, 10 times
(so every scan traverses the whole buffer).

- **Naive, per byte** (a `.j` loop comparing bytes one at a time): 6061 ms (Go) /
  4099 ms (TinyGo). This is the per-byte tree-walker cost byte-oriented code used
  to pay.
- **`binary.indexOf`** (one call into Go's assembly/SIMD `bytes.Index`): **0 ms**
  (Go) / 2 ms (TinyGo).

The Go row reads `0` because it is genuinely sub-millisecond - ~500 KB x 10 =
~5 MB scanned at ~20 GB/s finishes in a couple hundred microseconds, and the
suite reports whole milliseconds, so it floors to `0` (TinyGo's `2` is the same
work, just above the 1 ms rounding line). That is the point: pushing a per-byte
loop into one Go call is a ~40,000x speedup on this workload.

## v1 parallel section

Speedup is `serial_ms / par_ms`; > 1.0 means the four-worker version beat serial.
Go gets real multi-core speedup; TinyGo's cooperative scheduler stays at ~1.0 by
design.

| Workload             | Go serial (ms) | Go par (ms) | Go speedup | TinyGo serial (ms) | TinyGo par (ms) | TinyGo speedup |
| -------------------- | -------------- | ----------- | ---------- | ------------------ | --------------- | -------------- |
| `primes up to LIMIT` |          17096 |        6137 |   **2.79** |              14951 |           14954 |      1.00      |
| `newton sqrt batch`  |            283 |          76 |   **3.72** |                278 |             291 |      0.96      |
| `monte carlo pi`     |            768 |         225 |   **3.41** |                822 |             796 |      1.03      |
| `fib(N) x workers`   |            232 |          59 |   **3.93** |                212 |             224 |      0.95      |

Go reaches real multi-core speedup (2.8x-3.9x on four workers). `jennifer-tiny`
pins the cooperative scheduler, so `spawn` there is concurrency without
multi-core throughput: its column sits at ~1.0, a touch under on rows where the
spawn setup has no parallel payoff to hide behind. Use the default binary when
parallel throughput matters.

**The parallel section reverses the end-to-end picture.** TinyGo has the lower
*serial* total (21.4 s vs 25.3 s), but Go crunches the parallel section in
**~6.5 s** (6137 + 76 + 225 + 59) where TinyGo takes **~16.3 s** (no
parallelism). On the wall clock the v1 run of `0.24.0-dev+149` finished in
**31.83 s on Go vs 37.65 s on TinyGo** (`time`; 45.25 s vs 37.55 s user, so
~142% CPU against ~100%), and the full `--suite all` run (v1 + v2) in 32.15 s vs 39.37 s
(see below): a lower single-thread total does not mean a faster end-to-end run
once any `spawn` parallelism is in play.

## Suite v2 - numeric libraries (`stats` / `linalg` / `math` / `ml`)

The v2 workloads exercise the numeric stack: the Go-side compute plus the
Jennifer <-> Go marshaling of scalars and matrices. Both builds run it serially
(no parallel section).

```
=== Jennifer benchmark suite (--suite v2, jennifer) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
stats normal cdf                      50000        50000           82
math regGammaP                        50000        50000           94
linalg matmul NxN                        30          300           21
linalg solve NxN                         30          300            6
ml linreg fit+predict                   500          200           20
ml kMeans cluster                       500           40            6
----------------------------------------------------------------------
total                                                             229
```

```
=== Jennifer benchmark suite (--suite v2, jennifer-tiny) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
stats normal cdf                      50000        50000           98
math regGammaP                        50000        50000          112
linalg matmul NxN                        30          300           26
linalg solve NxN                         30          300            4
ml linreg fit+predict                   500          200           24
ml kMeans cluster                       500           40            7
----------------------------------------------------------------------
total                                                             271
```

Go is ahead on v2 (229 ms vs 271 ms, ~15% lower) and wins every row but
`linalg solve`, where TinyGo's 4 ms beats Go's 6 ms; in absolute terms the gap is
~40 ms over the whole suite, and each process exits in well under a second (0.25 s
Go, 0.30 s TinyGo, `time`). The distribution rows (`stats normal cdf`,
`math regGammaP`) dominate the total (176 of 229 ms on Go, 210 of 271 ms on
TinyGo) because they are the interpreter-heavy ones: 50 000 tree-walked
iterations each, one Go call per iteration. The `linalg` / `ml` rows are cheap despite doing real matrix work: the
compute is one Go call over a matrix built once outside the timed loop, so the
per-iteration cost is mostly the list marshaling, and the Go-side algorithms
(matmul, Gaussian solve, normal-equations fit) are far below the dispatch cost.
The takeaway: the numeric libraries carry their weight in Go; the tree-walker
overhead is in the per-call loop around them, not the math.

Run as part of `--suite all`, after the v1 section in the same process, v2
comes in at 236 ms on Go but 314 ms on TinyGo (the `stats`, `math`, `matmul`
and `linreg` rows all slower), so the gap widens to ~25%. The blocks above are
the standalone `--suite v2` runs; the TinyGo figure in particular depends on
what ran before it in the process.

## Suite v3 - value semantics

v1 and v2 are dominated by dispatch loops and Go-side compute, so they barely
register what the value model costs: the copy-avoidance work in
`0.24.0-dev+149` moved v1 by a few percent while a copy-heavy program got about a
third faster. v3 measures that cost directly. Every row moves a `Grid` - a struct
whose 10 000-element list is the expensive part to copy (`VS_CELLS`, a 100x100
grid, between an 80x24 and a 244x110 terminal) - or a list of small nested
structs, through one path of the value model:

| Workload                    | What it isolates                                                                  |
| --------------------------- | --------------------------------------------------------------------------------- |
| `owned update g = f(g)`     | `$g = gridSet($g, ...)` in a method: the argument is dead after the call, so the update may run in place. 20 000 calls. |
| `copy-then-write update`    | the defensive idiom `def out as Grid init $g; $out.cells[i] = v; return $out;`, 2 000 calls: what the interpreter makes of the local copy. |
| `snapshot + write (detach)` | the old value is kept alive across a write, so every one of 2 000 iterations must copy: the price of one full detach, the baseline for the rows above. |
| `borrowed read param`       | a `Grid` passed to a function that only reads it, 20 000 calls: the read-only borrow. |
| `list append helper`        | `$xs = appendTo($xs, v)`, the accumulator helper lint L205 names, growing a list to 10 000. |
| `nested field write`        | `$units[$j].pos.x = ...` and `$units[$j].hp = ...` over 1 000 structs, 50 passes: deep path writes into a list of structs. |
| `spawn capture`             | 200 tasks, each spawned and waited on in turn, that read one cell of a `Grid` in the enclosing scope: the cost of the scope snapshot `spawn` takes. One task at a time, because hundreds alive at once make `jennifer-tiny`'s per-task stack reservation the whole cost. |

Read the rows against each other rather than alone. The owned-update and
borrowed-read rows should cost a small fraction of the snapshot row; when one
of them climbs toward it, the interpreter has started copying where it did not
need to. On an interpreter from before owned updates (`0.24.0-dev+127`), the
owned-update and list-append rows copy on every call and take seconds where
`0.24.0-dev+149` takes tens of milliseconds, and the copy-then-write row costs
about two copies per call instead of one.

Two properties keep the series honest. **Every row checks its result** (the
final checksum, and for the snapshot row that each snapshot still holds its
pre-write value) and stops the run when it is wrong, so a build that got faster
by breaking copy semantics fails instead of posting a better number. **The
workloads use only long-standing language features** (full `Error` literals,
complete struct literals), so an older interpreter can run the current
`examples/benchmark.j --suite v3` and extend the series backwards.

```
=== Jennifer benchmark suite (--suite v3, jennifer) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
owned update g = f(g)                 10000        20000           41
copy-then-write update                10000         2000          873
snapshot + write (detach)             10000         2000          851
borrowed read param                   10000        20000           25
list append helper                    10000        10000           16
nested field write                     1000           50          131
spawn capture                         10000          200           93
----------------------------------------------------------------------
total                                                            2030
```

```
=== Jennifer benchmark suite (--suite v3, jennifer-tiny) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
owned update g = f(g)                 10000        20000           47
copy-then-write update                10000         2000         1165
snapshot + write (detach)             10000         2000         1124
borrowed read param                   10000        20000           25
list append helper                    10000        10000           13
nested field write                     1000           50          147
spawn capture                         10000          200          254
----------------------------------------------------------------------
total                                                            2775
```

Per operation on the reference machine (Go / TinyGo), and the cross-build ratio
`tiny_ms / go_ms` as in the v1 table:

| Workload                    | per operation        | Ratio  | What it shows                                                     |
| --------------------------- | -------------------- | ------ | ----------------------------------------------------------------- |
| `owned update g = f(g)`     | 2.1 / 2.4 us a call  | 1.1x   | Runs in place: about **200x cheaper than one copy** of the grid.  |
| `copy-then-write update`    | 0.44 / 0.58 ms a call | 1.3x  | Costs the same as the forced copy below: the defensive local copy is one full copy per call, so it is the idiom to avoid. |
| `snapshot + write (detach)` | 0.43 / 0.56 ms a copy | 1.3x  | The price of one full copy of a 10 000-cell grid: the baseline.   |
| `borrowed read param`       | 1.3 / 1.3 us a call  | 1.0x   | No copy: a read-only parameter is borrowed.                       |
| `list append helper`        | 1.6 / 1.3 us a call  | *0.8x* | Runs in place: the L205 accumulator is linear when called as `$xs = appendTo($xs, v)`. |
| `nested field write`        | 2.6 / 2.9 us a step  | 1.1x   | Two deep path writes per step (50 passes over 1 000 units), no copy of the list. |
| `spawn capture`             | 0.47 / 1.27 ms a task | 2.7x  | On Go, about one grid copy per task: the snapshot is a full copy. TinyGo adds roughly 0.7 ms of per-task overhead on top. |
| **total**                   |                      | 1.4x   |                                                                   |

The value model behaves as designed on both builds: every path that may avoid a
copy does (owned update, borrowed read, append helper, nested write), and the
two paths that must copy cost exactly one copy each. Where the builds differ is
the copy itself, ~30% slower on TinyGo, and task start-up, where TinyGo's
cooperative scheduler costs more per `spawn` (see [tinygo.md](tinygo.md), which
also covers the cost of many tasks alive at once).

GNU `/bin/time -v` on the two runs: `jennifer` 2.11 s wall at 194% CPU (the
concurrent GC works on other cores during the copy rows), 38.8 MB peak RSS, ~37,800
minor page faults; `jennifer-tiny` 2.85 s at 98% CPU, 102.2 MB peak RSS, ~16,000
minor page faults.

## Memory, page faults, and wall clock

GNU `/bin/time -v` on the full `--suite all` run on the reference machine,
version `0.24.0-dev+149.3222bd4`. It was taken before v3 existed, so it covers
v1 + v2; `--suite all` now runs v3 as well, which adds a few seconds:

| Metric                | `jennifer` (Go)  | `jennifer-tiny` (TinyGo) |
| --------------------- | ---------------- | ------------------------ |
| wall clock (real)     | 32.15 s          | 39.37 s                  |
| user / system         | 45.53 s / 0.10 s | 39.22 s / 0.04 s         |
| CPU                   | 141%             | 99%                      |
| peak resident (RSS)   | ~45 MB           | ~116 MB                  |
| minor page faults     | ~20,000          | ~18,200                  |
| major page faults     | 0                | 0                        |

- **Go finishes faster end-to-end** (32.15 s vs 39.37 s) while spending more
  CPU-seconds doing it (45.5 s user at 141% CPU). The gap between `user` and
  `real` is the concurrent GC on other cores plus the four `spawn` workers running
  truly in parallel during the parallel section. TinyGo's `user ~= real` at 99%
  CPU confirms single-thread execution: the cooperative scheduler runs every
  goroutine on one OS thread.
- **TinyGo uses ~2.6x the peak RSS** (~116 MB vs ~45 MB). Its cooperative
  scheduler reserves each goroutine's full `-stack-size` (4 MB, sized above the
  catchable call-depth cap) up front, so the four parallel `spawn` workers hold
  ~16 MB of reserved stack between them. Go grows goroutine stacks on demand from
  ~8 KB, so its footprint stays small and flat.
- **Minor page faults are comparable** (~20.0k Go, ~18.2k TinyGo), TinyGo
  slightly lower - Go's growable heap takes the allocation churn of the four
  parallel workers and the v2 numeric workloads (matrix / model allocation). Both
  builds fault less than on `0.23.0-dev+16` (~26.8k Go, ~22.7k TinyGo), which
  was built with an older Go and TinyGo, so the drop is not the interpreter's
  alone. Neither build takes any major faults.

## Picking a binary

- **Single-thread compute:** the two builds trade wins by workload shape. TinyGo
  leads the long tight numeric loops (`primes`, `fib`) and the naive byte
  scan; Go leads the allocation-heavy `list` row, `monte carlo` and the small
  structural rows.
  Which build posts the lower serial *total* shifts run to run.
- **End-to-end wall clock / any `spawn` parallelism:** the default **`jennifer`**,
  every time - it is the only build with real multi-core throughput (2.8x-3.9x
  here) and finishes the whole suite faster despite a higher serial total.
- **Footprint:** `jennifer` for a small, on-demand memory profile (~45 MB peak);
  `jennifer-tiny` trades ~2.6x the RSS for its smaller *binary* and embeddability,
  not for a smaller runtime footprint.
- **Byte-oriented work:** reach for the `binary` library (and `net.readAll` /
  `readN`) on either build - the `binary.indexOf` row shows a per-byte loop
  collapsing to effectively free.
- **Numeric work:** `stats` / `linalg` / `math` / `ml` cost about the same on
  both builds (Go ~15% lower on v2, ~40 ms in absolute terms); the per-call
  dispatch loop dominates, not the Go-side math.
