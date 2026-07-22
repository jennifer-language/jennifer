# Benchmarks

Single-binary throughput for the two builds - `jennifer` (standard Go, the
default) and `jennifer-tiny` (TinyGo, the constrained variant) - measured with
`examples/benchmark.j`.

Reference numbers are from `examples/benchmark.j` (version
`0.20.0-dev+24.05b91d9`) on an **AMD Ryzen 5 7600X3D** (6 cores, 12 threads;
bare metal, desktop idle) - the machine the suite prints in its own header, from
`os.NCPU` plus a `/proc/cpuinfo` read done in Jennifer. The numbers are
machine-specific; the **ratios and the shape** of the comparison are the stable
part, not the absolute milliseconds.

The suite has two sections. The **serial** section is single-threaded by design;
the **parallel** section fans out to `PARALLEL_WORKERS = 4` spawn tasks per
workload. The interpreter build is the current one - eager-copy value semantics,
lexical slot resolution, parse-time constant folding, the advisory map hash
index, per-frame allocation elimination (slot-backed bindings), and the `binary`
bulk-byte primitives - so append-in-a-loop is amortised O(N) with in-place
growth, keyed map access is O(1), a call frame allocates nothing per binding, and
a byte scan can drop into a single Go call.

> These are reference-machine figures, recorded from a supplied run, **not**
> produced in CI - benchmark timings are too machine- and load-sensitive to pin
> in a test. Regenerate them on the reference machine after a perf-relevant
> change; do not run the suite on other hardware for documentation numbers.

## `jennifer` (standard-Go binary, default)

```
=== Jennifer benchmark suite ===
build:    go
version:  0.20.0-dev+24.05b91d9
cpu:      AMD Ryzen 5 7600X3D 6-Core Processor (12 cores)
platform: linux/amd64
host:     bare metal (no VM / container hints)

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           51
primes up to LIMIT                   100000            1        16091
newton sqrt batch                     10000        10000          271
monte carlo pi                       500000       500000          758
list sort/reverse/slice               10000          500          942
struct list build+read                10000        10000           27
string join                           10000        10000           10
map insert+read                       10000        10000           34
byte scan naive (per-byte)           500000           10         5749
byte scan binary.indexOf (Go)        500000           10            0
----------------------------------------------------------------------
total                                                           23933

Parallel comparison (workers = 4, scheduler = go)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                    16091         5711         2.82
newton sqrt batch                       271           66         4.11
monte carlo pi                          758          212         3.58
fib(N) x workers                        204           59         3.46
----------------------------------------------------------------------

real   30.0s   user 42.3s   sys 0.1s   (141% CPU)
```

`user` (42.3s) exceeds `real` (30.0s) by ~12s - that gap is Go's concurrent GC
running on other cores during the serial section, plus the four spawn workers
running truly in parallel during the parallel section. Sys time is negligible
(0.1s) because Go coordinates goroutines with cheap in-process primitives.

## `jennifer-tiny` (TinyGo binary)

```
=== Jennifer benchmark suite ===
build:    tinygo
version:  0.20.0-dev+24.05b91d9
cpu:      AMD Ryzen 5 7600X3D 6-Core Processor (1 cores)
platform: linux/amd64
host:     bare metal (no VM / container hints)

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           54
primes up to LIMIT                   100000            1        15675
newton sqrt batch                     10000        10000          265
monte carlo pi                       500000       500000          803
list sort/reverse/slice               10000          500         1363
struct list build+read                10000        10000           33
string join                           10000        10000           12
map insert+read                       10000        10000           39
byte scan naive (per-byte)           500000           10         4404
byte scan binary.indexOf (Go)        500000           10            2
----------------------------------------------------------------------
total                                                           22650

Parallel comparison (workers = 4, scheduler = tinygo)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                    15675        15984         0.98
newton sqrt batch                       265          279         0.95
monte carlo pi                          803          805         1.00
fib(N) x workers                        216          233         0.93
----------------------------------------------------------------------

real   40.0s   user 39.7s   sys 0.1s   (99% CPU)
```

`user ~= real` (39.7s vs 40.0s, 99% CPU) confirms single-thread execution: the
cooperative `-scheduler=tasks` build runs every goroutine on one OS thread, so
the parallel section is concurrency without multi-core throughput. `os.NCPU`
reports `1` here - honest about the scheduler's usable parallelism, not the 12
threads the machine has. The parallel column hovers at ~1.0 by design, dipping
slightly below on some rows (spawn setup with no parallel payoff to offset it).

## Per-workload comparison (serial section)

Ratios are `tiny_ms / go_ms`; > 1.0 means `jennifer-tiny` is slower, < 1.0 means
it is faster.

| Workload                       | tiny (ms) | go (ms) | Ratio  | Where the time goes                                                            |
| ------------------------------ | --------- | ------- | ------ | ------------------------------------------------------------------------------ |
| `fib(N) recursive`             |        54 |      51 | 1.1x   | Call-heavy dispatch. Down ~30% on both builds (from 83 / 73) with the per-frame allocation elimination. Go a hair ahead. |
| `primes up to LIMIT`           |     15675 |   16091 | *0.9x* | The long numeric dispatch loop; TinyGo still leads, but by 416 ms now (was ~1.6 s). |
| `newton sqrt batch`            |       265 |     271 | *1.0x* | Float arithmetic + dispatch; effectively tied, TinyGo a hair ahead.            |
| `monte carlo pi`               |       803 |     758 | 1.1x   | Float arithmetic + RNG calls; Go now edges it (was tied).                      |
| `list sort/reverse/slice`      |      1363 |     942 | *1.4x* | Allocation-heavy. The row **flipped**: TinyGo led it before (871 vs 977); now Go's GC handles the churn better and TinyGo's simpler collector pays more. The one clear Go win in the serial section. |
| `struct list build+read`       |        33 |      27 | 1.2x   | Append hot loop is O(1); both effectively free (sub-40 ms).                    |
| `string join`                  |        12 |      10 | 1.2x   | Build-up-a-string pattern is O(1); both free (sub-15 ms).                      |
| `map insert+read`              |        39 |      34 | 1.1x   | The advisory map hash index keeps keyed insert+read O(1); both sub-40 ms, Go a hair ahead. |
| `byte scan naive (per-byte)`   |      4404 |    5749 | *0.8x* | A per-byte tree-walker scan of ~500 KB x 10 - the cost the `binary` library exists to remove. TinyGo's tighter dispatch wins it by 1.3 s. |
| `byte scan binary.indexOf (Go)`|         2 |       0 | -      | The same scan through one `binary.indexOf` call (Go `bytes.Index`, SIMD). Both effectively **0 ms** - see below. |
| **total**                      |     22650 |   23933 | *0.9x* | TinyGo posts the lower serial total, but the margin now lives almost entirely in the naive byte-scan row. |

The two binaries stay close, and the story is more nuanced than a single total.
TinyGo's ~5% lower serial total (22.7 s vs 23.9 s) is carried by two rows where
its tighter dispatch loop wins big - `primes` (-416 ms) and the naive
`byte scan` (-1345 ms) - which together more than cover the rows Go wins.
**Strip the two new byte-scan rows and the totals are a dead heat**: 18244 ms
(TinyGo) vs 18184 ms (Go), Go marginally ahead. On the classic workloads the two
builds are now essentially tied, where TinyGo used to hold a clearer ~9-13%
lead.

Two shifts stand out versus the previous reference run:

- **`fib` dropped ~30% on both builds** (TinyGo 83 -> 54, Go 73 -> 51). `fib` is
  the most call-heavy workload, so it is the clearest read on the per-frame
  allocation elimination (slot-backed bindings): a recursive call now allocates
  nothing per binding.
- **The `list` row flipped to Go.** It was TinyGo's before (871 vs 977); now Go
  leads it decisively (942 vs 1363). Go's concurrent GC absorbs the
  allocation-heavy sort/reverse/slice churn better than TinyGo's simpler
  collector at this scale, and it is now the one workload where Go clearly wins
  the serial section.

## The bulk-byte rows

The two `byte scan` rows are the `binary` library's throughput demonstration:
the same search for a 6-byte needle at the **end** of a ~500 KB buffer, 10 times
(so every scan traverses the whole buffer).

- **Naive, per byte** (a `.j` loop comparing bytes one at a time): 5749 ms (Go) /
  4404 ms (TinyGo). This is the per-byte tree-walker cost byte-oriented code used
  to pay.
- **`binary.indexOf`** (one call into Go's assembly/SIMD `bytes.Index`): **0 ms**
  (Go) / 2 ms (TinyGo).

The Go row reads `0` because it is genuinely sub-millisecond - ~500 KB x 10 =
~5 MB scanned at ~20 GB/s finishes in a couple hundred microseconds, and the
suite reports whole milliseconds, so it floors to `0` (TinyGo's `2` is the same
work, just above the 1 ms rounding line). That is the point: pushing a per-byte
loop into one Go call is a ~40,000x speedup on this workload.

## Parallel section

Speedup is `serial_ms / par_ms`; > 1.0 means the four-worker version beat serial.
Go gets real multi-core speedup; TinyGo's cooperative scheduler stays at ~1.0 by
design.

| Workload             | Go serial (ms) | Go par (ms) | Go speedup | TinyGo serial (ms) | TinyGo par (ms) | TinyGo speedup |
| -------------------- | -------------- | ----------- | ---------- | ------------------ | --------------- | -------------- |
| `primes up to LIMIT` |          16091 |        5711 |   **2.82** |              15675 |           15984 |      0.98      |
| `newton sqrt batch`  |            271 |          66 |   **4.11** |                265 |             279 |      0.95      |
| `monte carlo pi`     |            758 |         212 |   **3.58** |                803 |             805 |      1.00      |
| `fib(N) x workers`   |            204 |          59 |   **3.46** |                216 |             233 |      0.93      |

Go reaches real multi-core speedup (2.8x-4.1x on four workers). `jennifer-tiny`
pins the cooperative scheduler, so `spawn` there is concurrency without
multi-core throughput: its column sits at ~1.0, a touch under on rows where the
spawn setup has no parallel payoff to hide behind. Use the default binary when
parallel throughput matters.

**This is where the serial-total lead reverses.** TinyGo has the lower *serial*
total (22.7 s vs 23.9 s), but Go finishes the *whole suite* in far less
wall-clock time: `real` is **30.0 s for Go vs 40.0 s for TinyGo**. The parallel
section is why - Go crunches it multi-core in ~6 s where TinyGo takes ~16 s (no
parallelism), a ~10 s swing that more than erases TinyGo's ~1.3 s serial edge.
Lower single-thread compute does not mean a faster end-to-end run once any
`spawn` parallelism is in play.

## Memory and page faults

Same machine, GNU `/bin/time` on the same runs (per-workload timings within noise
of the tables above):

| Metric                | `jennifer` (Go) | `jennifer-tiny` (TinyGo) |
| --------------------- | --------------- | ------------------------ |
| peak resident (RSS)   | ~41 MB          | ~123 MB                  |
| minor page faults     | ~6,672          | ~22,542                  |
| CPU                   | 141%            | 99%                      |

The two runtimes trade opposite resources, and both sides of that trade moved
since the previous run.

- **TinyGo uses ~3.0x the peak RSS** (~123 MB vs ~41 MB). Its cooperative
  scheduler reserves each goroutine's full `-stack-size` up front, and that stack
  was raised from 2 MB to 4 MB (to sit above the catchable call-depth cap), so
  the four parallel `spawn` workers now hold ~16 MB of reserved stack between
  them. Go grows goroutine stacks on demand from ~8 KB, so its footprint stays
  small and flat.
- **The page-fault relationship reversed.** Go now churns *far fewer* minor
  faults (~6.7k) than TinyGo (~22.5k) - the opposite of earlier builds, where
  Go's concurrent GC faulted ~3x more than TinyGo. The driver is the per-frame
  allocation elimination: removing the per-binding and per-call heap traffic from
  the hot path cut Go's allocation churn dramatically (its minor faults fell from
  ~49.6k in the prior reference run to ~6.7k), and with it the GC page activity
  that used to dominate. TinyGo's faults rose over the same window (byte-scan
  buffers plus the larger reserved stacks), so the two crossed over.

So the trade today: TinyGo buys competitive-to-leading single-thread dispatch
with a larger, flatter memory footprint and now the higher page-fault count; Go
buys a small footprint, sharply reduced allocation churn, and the only real
multi-core parallelism, at 141% CPU during the parallel section.

## Picking a binary

- **Single-thread compute:** roughly a tie now. TinyGo still leads the long tight
  numeric loops (`primes`, `newton`) and the naive byte scan; Go leads the
  allocation-heavy `list` row and the small structural rows. Excluding the
  byte-scan rows the serial totals are within 0.3%.
- **End-to-end wall clock / any `spawn` parallelism:** the default **`jennifer`**,
  every time - it is the only build with real multi-core throughput (2.8x-4.1x
  here) and finishes the suite in 30 s vs 40 s.
- **Footprint:** `jennifer` for a small, on-demand memory profile (~41 MB peak);
  `jennifer-tiny` trades ~3x the RSS for its smaller *binary* and embeddability,
  not for a smaller runtime footprint.
- **Byte-oriented work:** reach for the `binary` library (and `net.readAll` /
  `readN`) on either build - the `binary.indexOf` row shows a per-byte loop
  collapsing to effectively free.
