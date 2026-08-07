# Benchmarks

Single-binary throughput for the two builds - `jennifer` (standard Go, the
default) and `jennifer-tiny` (TinyGo, the constrained variant) - measured with
`examples/benchmark.j`.

Reference numbers are from `examples/benchmark.j` (version
`0.23.0-dev+16.f857e8a`) on an **AMD Ryzen 5 7600X3D** (6 cores, 12 threads;
bare metal, desktop idle) - the machine the suite prints in its own header, from
`os.NCPU` plus a `/proc/cpuinfo` read done in Jennifer. The numbers are
machine-specific; the **ratios and the shape** of the comparison are the stable
part, not the absolute milliseconds.

The suite is **versioned** (`--suite v1` / `v2` / `all`) so its reference series
stays comparable over time. **v1** is the interpreter-throughput workload;
**v2** is the numeric-library workload (`stats` / `linalg` / `math` / `ml`),
reported in its own section below. Each version's workloads are frozen once they
have published reference numbers.

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
version:  0.23.0-dev+16.f857e8a
cpu:      AMD Ryzen 5 7600X3D 6-Core Processor (12 cores)
platform: linux/amd64
host:     bare metal (no VM / container hints)

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           57
primes up to LIMIT                   100000            1        18342
newton sqrt batch                     10000        10000          309
monte carlo pi                       500000       500000          821
list sort/reverse/slice               10000          500         1075
struct list build+read                10000        10000           30
string join                           10000        10000           11
map insert+read                       10000        10000           37
byte scan naive (per-byte)           500000           10         6442
byte scan binary.indexOf (Go)        500000           10            0
----------------------------------------------------------------------
total                                                           27124

Parallel comparison (workers = 4, scheduler = go)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                    18342         6543         2.80
newton sqrt batch                       309           74         4.18
monte carlo pi                          821          232         3.54
fib(N) x workers                        228           63         3.62
----------------------------------------------------------------------
```

Go runs the serial section single-threaded while its concurrent GC works on other
cores, then reaches genuine multi-core throughput in the parallel section (four
`spawn` workers on separate OS threads).

## Suite v1 - `jennifer-tiny` (TinyGo binary)

```
=== Jennifer benchmark suite ===
build:    tinygo
version:  0.23.0-dev+16.f857e8a
cpu:      AMD Ryzen 5 7600X3D 6-Core Processor (1 cores)
platform: linux/amd64
host:     bare metal (no VM / container hints)

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           53
primes up to LIMIT                   100000            1        15451
newton sqrt batch                     10000        10000          270
monte carlo pi                       500000       500000          818
list sort/reverse/slice               10000          500         1620
struct list build+read                10000        10000           33
string join                           10000        10000           18
map insert+read                       10000        10000           45
byte scan naive (per-byte)           500000           10         4286
byte scan binary.indexOf (Go)        500000           10            2
----------------------------------------------------------------------
total                                                           22596

Parallel comparison (workers = 4, scheduler = tinygo)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                    15451        15261         1.01
newton sqrt batch                       270          283         0.95
monte carlo pi                          818          845         0.97
fib(N) x workers                        212          229         0.93
----------------------------------------------------------------------
```

`os.NCPU` reports `1` here - honest about the cooperative `-scheduler=tasks`
build's usable parallelism, not the 12 threads the machine has. Every goroutine
runs on one OS thread, so the parallel column hovers at ~1.0 by design, dipping
slightly below on rows where spawn setup has no parallel payoff to offset it.

## v1 per-workload comparison (serial section)

Ratios are `tiny_ms / go_ms`; > 1.0 means `jennifer-tiny` is slower, < 1.0
(shown in *italics*) means it is faster.

| Workload                       | tiny (ms) | go (ms) | Ratio  | Where the time goes                                                            |
| ------------------------------ | --------- | ------- | ------ | ------------------------------------------------------------------------------ |
| `fib(N) recursive`             |        53 |      57 | *0.9x* | Call-heavy dispatch; TinyGo's tighter loop edges it. Both effectively free (sub-60 ms) thanks to the per-frame allocation elimination. |
| `primes up to LIMIT`           |     15451 |   18342 | *0.8x* | The long numeric dispatch loop, the single biggest row. TinyGo leads it by ~2.9 s this run. |
| `newton sqrt batch`            |       270 |     309 | *0.9x* | Float arithmetic + dispatch; TinyGo ahead.                                     |
| `monte carlo pi`               |       818 |     821 | 1.0x   | Float arithmetic + RNG calls; a dead heat.                                     |
| `list sort/reverse/slice`      |      1620 |    1075 | 1.5x   | Allocation-heavy. Go's concurrent GC absorbs the sort/reverse/slice churn better than TinyGo's simpler collector - the one clear Go win in the serial section. |
| `struct list build+read`       |        33 |      30 | 1.1x   | Append hot loop is O(1); both effectively free (sub-40 ms).                    |
| `string join`                  |        18 |      11 | 1.6x   | Build-up-a-string pattern is O(1); both free (sub-20 ms).                      |
| `map insert+read`              |        45 |      37 | 1.2x   | The advisory map hash index keeps keyed insert+read O(1); both sub-50 ms.      |
| `byte scan naive (per-byte)`   |      4286 |    6442 | *0.7x* | A per-byte tree-walker scan of ~500 KB x 10 - the cost the `binary` library exists to remove. TinyGo's tighter dispatch wins it by ~2.2 s. |
| `byte scan binary.indexOf (Go)`|         2 |       0 | -      | The same scan through one `binary.indexOf` call (Go `bytes.Index`, SIMD). Both effectively **0 ms** - see below. |
| **total**                      |     22596 |   27124 | *0.8x* | TinyGo posts the lower serial total (~17%), carried mainly by `primes` and the naive byte-scan row. |

TinyGo takes the serial section clearly this run (22.6 s vs 27.1 s, ~17% lower),
where its tighter dispatch loop leads the long numeric workloads - `primes`
(-2.9 s), the naive `byte scan` (-2.2 s), plus `newton` and `fib`. Go's win is
the allocation-heavy `list` row (+545 ms) and the tiny structural rows
(`struct` / `string` / `map`, all sub-50 ms). **Strip the two byte-scan rows and
the classic-workload totals are 18308 ms (TinyGo) vs 20682 ms (Go)** - TinyGo
still ahead, so this run's serial lead is not just the byte scan. (The absolute
milliseconds shift run to run; the durable signal is that the two builds trade
wins by workload shape - TinyGo the tight compute loops, Go the allocation churn
and the parallel section below.)

## The bulk-byte rows

The two `byte scan` rows are the `binary` library's throughput demonstration:
the same search for a 6-byte needle at the **end** of a ~500 KB buffer, 10 times
(so every scan traverses the whole buffer).

- **Naive, per byte** (a `.j` loop comparing bytes one at a time): 6442 ms (Go) /
  4286 ms (TinyGo). This is the per-byte tree-walker cost byte-oriented code used
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
| `primes up to LIMIT` |          18342 |        6543 |   **2.80** |              15451 |           15261 |      1.01      |
| `newton sqrt batch`  |            309 |          74 |   **4.18** |                270 |             283 |      0.95      |
| `monte carlo pi`     |            821 |         232 |   **3.54** |                818 |             845 |      0.97      |
| `fib(N) x workers`   |            228 |          63 |   **3.62** |                212 |             229 |      0.93      |

Go reaches real multi-core speedup (2.8x-4.2x on four workers). `jennifer-tiny`
pins the cooperative scheduler, so `spawn` there is concurrency without
multi-core throughput: its column sits at ~1.0, a touch under on rows where the
spawn setup has no parallel payoff to hide behind. Use the default binary when
parallel throughput matters.

**The parallel section reverses the end-to-end picture.** TinyGo has the lower
*serial* total (22.6 s vs 27.1 s), but Go crunches the parallel section in
**~6.9 s** (6543 + 74 + 232 + 63) where TinyGo takes **~16.6 s** (no
parallelism). On the wall clock the full `--suite all` run finishes in
**34.35 s on Go vs 39.43 s on TinyGo** (see below): a lower single-thread total
does not mean a faster end-to-end run once any `spawn` parallelism is in play.

## Suite v2 - numeric libraries (`stats` / `linalg` / `math` / `ml`)

The v2 workloads exercise the numeric stack: the Go-side compute plus the
Jennifer <-> Go marshaling of scalars and matrices. Both builds run it serially
(no parallel section).

```
=== Jennifer benchmark suite (--suite v2, jennifer) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
stats normal cdf                      50000        50000           94
math regGammaP                        50000        50000          108
linalg matmul NxN                        30          300           25
linalg solve NxN                         30          300            7
ml linreg fit+predict                   500          200           25
ml kMeans cluster                       500           40            9
----------------------------------------------------------------------
total                                                             268
```

```
=== Jennifer benchmark suite (--suite v2, jennifer-tiny) ===
----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
stats normal cdf                      50000        50000           91
math regGammaP                        50000        50000          116
linalg matmul NxN                        30          300           24
linalg solve NxN                         30          300            4
ml linreg fit+predict                   500          200           28
ml kMeans cluster                       500           40            5
----------------------------------------------------------------------
total                                                             268
```

The two builds are effectively tied on v2 (268 ms each). The distribution rows
(`stats normal cdf`, `math regGammaP`) dominate the total because they are the
interpreter-heavy ones - 50 000 tree-walked iterations each, one Go call per
iteration. The `linalg` / `ml` rows are cheap despite doing real matrix work: the
compute is one Go call over a matrix built once outside the timed loop, so the
per-iteration cost is mostly the list marshaling, and the Go-side algorithms
(matmul, Gaussian solve, normal-equations fit) are far below the dispatch cost.
The takeaway: the numeric libraries carry their weight in Go; the tree-walker
overhead is in the per-call loop around them, not the math.

## Memory, page faults, and wall clock

GNU `/bin/time` on the full `--suite all` run (v1 + v2) on the reference machine:

| Metric                | `jennifer` (Go)  | `jennifer-tiny` (TinyGo) |
| --------------------- | ---------------- | ------------------------ |
| wall clock (real)     | 34.35 s          | 39.43 s                  |
| user / system         | 48.50 s / 0.11 s | 39.21 s / 0.03 s         |
| CPU                   | 141%             | 99%                      |
| peak resident (RSS)   | ~44 MB           | ~123 MB                  |
| minor page faults     | ~26,800          | ~22,700                  |
| major page faults     | 0                | 0                        |

- **Go finishes faster end-to-end** (34.35 s vs 39.43 s) while spending more
  CPU-seconds doing it (48.5 s user at 141% CPU). The gap between `user` and
  `real` is the concurrent GC on other cores plus the four `spawn` workers running
  truly in parallel during the parallel section. TinyGo's `user ~= real` at 99%
  CPU confirms single-thread execution: the cooperative scheduler runs every
  goroutine on one OS thread.
- **TinyGo uses ~2.8x the peak RSS** (~123 MB vs ~44 MB). Its cooperative
  scheduler reserves each goroutine's full `-stack-size` (4 MB, sized above the
  catchable call-depth cap) up front, so the four parallel `spawn` workers hold
  ~16 MB of reserved stack between them. Go grows goroutine stacks on demand from
  ~8 KB, so its footprint stays small and flat.
- **Minor page faults are comparable** this run (~26.8k Go, ~22.7k TinyGo), Go
  slightly higher - the allocation churn of the four parallel workers plus the v2
  numeric workloads (matrix / model allocation) landing on Go's growable heap.
  Neither build takes any major faults.

## Picking a binary

- **Single-thread compute:** the two builds trade wins by workload shape. TinyGo
  leads the long tight numeric loops (`primes`, `newton`) and the naive byte
  scan; Go leads the allocation-heavy `list` row and the small structural rows.
  Which build posts the lower serial *total* shifts run to run.
- **End-to-end wall clock / any `spawn` parallelism:** the default **`jennifer`**,
  every time - it is the only build with real multi-core throughput (2.8x-4.2x
  here) and finishes the whole suite faster despite a higher serial total.
- **Footprint:** `jennifer` for a small, on-demand memory profile (~44 MB peak);
  `jennifer-tiny` trades ~3x the RSS for its smaller *binary* and embeddability,
  not for a smaller runtime footprint.
- **Byte-oriented work:** reach for the `binary` library (and `net.readAll` /
  `readN`) on either build - the `binary.indexOf` row shows a per-byte loop
  collapsing to effectively free.
- **Numeric work:** `stats` / `linalg` / `math` / `ml` cost the same on both
  builds (v2 is a tie); the per-call dispatch loop dominates, not the Go-side
  math.
