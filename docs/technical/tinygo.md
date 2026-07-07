# TinyGo notes

Jennifer ships as two binaries built from the same source:

- `jennifer` - default, built with the standard Go toolchain.
  Full host-feature surface: file I/O, `os/exec`, network stack,
  everything.
- `jennifer-tiny` - constrained variant, built with TinyGo. Smaller
  binary, embeddable; missing `os/exec` and the network stack (see
  [TinyGo restrictions](#tinygo-restrictions) below).

`make build` produces both. Use `make build-go` or `make
build-tinygo` for just one; both regenerate the version file
before compiling.

**The language is written to stay TinyGo-clean** even though the
default binary is standard-Go. The `jennifer-tiny` build sits in
CI so any change that breaks TinyGo compatibility surfaces
immediately. A few constraints shape the implementation:

- **No `reflect`-heavy code.** Tagged-union `Value` instead of
  interfaces with type assertions in hot paths. `task.waitAny`
  uses `reflect.Select` (verified under TinyGo); other new
  reflect uses need justification.
- **No `text/template`.** Not needed yet; would drag in fragile
  runtime paths under TinyGo.
- **No `encoding/json` for in-binary serialization.** The
  reflect-based marshaler is fragile under TinyGo, so the AST
  JSON emitter is hand-rolled (see
  [CLI > Inspection](cli.md#inspection-tokens-and-ast)).
- **Goroutines are allowed** (used since M16.0 for `spawn`),
  but need `-stack-size=1mb` under TinyGo - see
  [the goroutine-stack note below](#m160--tinygo-goroutine-stack).
- **No `-ldflags "-X package.var=value"`.** TinyGo 0.41 silently
  ignores the `-X` directive. Use the codegen path
  (`scripts/gen-version.sh` -> `internal/version/version_gen.go`)
  for build-time string injection. See
  [CLI > Version injection](cli.md#version-injection).
- **No hard dependencies on a hosted runtime.** `jennifer-tiny`
  targets embedded systems, minimal containers, and small-footprint
  scripting hosts where ambient stdin, dynamic linking, and a full
  hosted runtime are not guaranteed.
- **`testing` runs under regular `go test`.** TinyGo's `testing`
  support is partial; we develop and verify with `go test ./...`.

Verify both builds after non-trivial changes:

```sh
make build
./jennifer      run examples/hello.j   # default (standard Go); full host features
./jennifer-tiny run examples/hello.j   # constrained (TinyGo); no os/exec, no net
```

## TinyGo restrictions

A few standard-library features depend on TinyGo runtime support
that isn't there today. Calls into them from `jennifer-tiny` error
with a friendly Jennifer-level message pointing the user at the
default `jennifer` binary. The default binary always supports the
full surface.

| Library | Affected names                                        | What happens on `jennifer-tiny`                                                                       |
| ------- | ----------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| `os`    | `os.run`, `os.spawn`, `os.wait`, `os.poll`, `os.kill` | Runtime error pointing at the default `jennifer` binary. TinyGo's `os/exec` syscalls aren't implemented yet. |
| `net`   | Every entry point (TCP, UDP, DNS)                     | Runtime error pointing at the default `jennifer` binary. TinyGo 0.41 needs a netdev driver at runtime (not registered) and has no `net.ListenPacket` for UDP. Build-tag split: `netlib_tinygo.go` returns friendly errors. |

The constants and the env / argv / flag helpers in `os`
(`os.PLATFORM`, `os.ARCH`, `os.EOL`, `os.DIRSEP`, `os.PATHSEP`,
`os.ARGS`, `os.getEnv`, `os.hasFlag`, `os.flag`) all work fully
on both binaries. Every other shipped library (`io`, `convert`,
`math`, `strings`, `lists`, `maps`, `meta`, `time`, `hash`,
`crc`, `encoding`, `task`, `fs`, `regex`, `testing`) has full
TinyGo support.

**M16.0 / TinyGo goroutine stack**. Jennifer's tree-walking
evaluator wraps each Jennifer-level call in many Go-stack frames
(`execBlock` + `evalCall` + `evalExpr` + ...), so even a
modest-depth recursion (fib 23) easily exceeds TinyGo's default
goroutine stack of ~8KB and segfaults. The Makefile passes
`-stack-size=1mb` to `tinygo build` for `jennifer-tiny` so it
can run recursive `spawn` bodies (and the parallel section of
`examples/benchmark.j`). The default `jennifer` binary doesn't
need this - Go's goroutine stacks grow automatically.

**M16.0 / TinyGo scheduler**. TinyGo's runtime scheduler is
cooperative and (as of 0.41) does not exploit multi-core
hardware: every goroutine runs on the same OS thread.
`spawn` works correctly under TinyGo - the semantics, the
loud-fail, the registry - but **parallel speedups on
`jennifer-tiny` will be close to 1.0**. The default `jennifer`
binary uses Go's regular scheduler and does benefit from multiple
cores. The benchmark output reports the scheduler name in the
parallel-section header so the speedup numbers can be read in
context.

Future library work will grow the restrictions table if further
TinyGo runtime gaps surface. Each new gap lands with the same
friendly-message pattern.

## Single-binary benchmark results

Reference numbers from `examples/benchmark.j` run as a single
process on an **AMD Ryzen 5 7600X3D** (6 cores, 12 threads;
Linux + KDE Plasma desktop active during the run, total CPU load
low). Interpreter build is the current M16.5.1 shared-marker
COW variant - the append-in-a-loop pattern that dominated the
pre-M16.5 numbers (see the historical row further below) is now
amortised O(1). The serial section is single-threaded by
design; the parallel section fans out to `PARALLEL_WORKERS = 4`
spawn tasks per workload and prints serial-vs-parallel
speedups.

### `jennifer` (standard-Go binary, default)

```
=== Jennifer benchmark suite ===
build:   go
version: 0.15.0-dev (M16.5.1)

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           89
primes up to LIMIT                   100000            1        21067
newton sqrt batch                     10000        10000          337
monte carlo pi                       500000       500000         1054
list sort/reverse/slice               10000          500         1406
struct list build+read                10000        10000           36
string join                           10000        10000           11
map insert+read                       10000        10000         1197
----------------------------------------------------------------------
total                                                           25197

Parallel comparison (workers = 4, scheduler = go)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                    21067         7940         2.65
newton sqrt batch                       337           94         3.59
monte carlo pi                         1054          338         3.12
fib(N) x workers                        356          111         3.21
----------------------------------------------------------------------

user   50.60 s
sys     0.53 s
real   33.69 s     (151% CPU)
```

`user + sys` (51.1 s) exceeds `real` (33.7 s) by ~17 s - that
gap is Go's concurrent GC running on a second CPU during the
serial section, plus the four spawn workers running in
parallel during the parallel section. Sys time is small (0.5 s)
because Go's runtime coordinates goroutines with cheap in-process
sync primitives.

### `jennifer-tiny` (TinyGo binary)

```
=== Jennifer benchmark suite ===
build:   tinygo
version: 0.15.0-dev (M16.5.1)

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1          415
primes up to LIMIT                   100000            1        42421
newton sqrt batch                     10000        10000          779
monte carlo pi                       500000       500000         2509
list sort/reverse/slice               10000          500         1003
struct list build+read                10000        10000           60
string join                           10000        10000           19
map insert+read                       10000        10000         1690
----------------------------------------------------------------------
total                                                           48896

Parallel comparison (workers = 4, scheduler = tinygo)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                    42421        62359         0.68
newton sqrt batch                       779         1087         0.72
monte carlo pi                         2509         3443         0.73
fib(N) x workers                       1660         1646         1.01
----------------------------------------------------------------------

user  125.69 s
sys    91.29 s     (!)
real  117.44 s     (184% CPU)
```

**Sys time explodes on `jennifer-tiny`** (91.3 s vs 0.5 s on
the Go build) even though TinyGo's GC is non-concurrent. The
serial section spends almost no time in the kernel; the sys
column is the parallel section paying its bill. Four spawn
workers on TinyGo's cooperative scheduler thrash the runtime's
park/unpark path - every `task.wait` becomes a futex round trip,
and the runtime's `runtime.futexsleep` / `futexwakeup` pair
isn't tuned for this contention rate. The kernel time is real:
`taskset -c 0` (pinning to one CPU) would cut it, but the
underlying issue is TinyGo's runtime not being tuned for
goroutine-coordination throughput. This is exactly why the
parallel-section speedups are below 1.0 for three of four
workloads - the runtime is spending its wall-clock in
scheduler overhead, not user code.

### Per-workload comparison (serial section)

Ratios are `tiny_ms / go_ms`; > 1.0 means `jennifer-tiny` is slower.

| Workload                  | tiny (ms) | go (ms) | Ratio    | Where the time goes                                                            |
| ------------------------- | --------- | ------- | -------- | ------------------------------------------------------------------------------ |
| `fib(N) recursive`        |       415 |      89 | **4.7x** | Tight interpreter dispatch loop; TinyGo's runtime call overhead dominates.     |
| `primes up to LIMIT`      |     42421 |   21067 | **2.0x** | Same dispatch loop, more iterations; the gap stabilises.                       |
| `newton sqrt batch`       |       779 |     337 | **2.3x** | Float arithmetic + dispatch; same shape as primes.                             |
| `monte carlo pi`          |      2509 |    1054 | **2.4x** | Float arithmetic + RNG calls; identical pattern.                               |
| `list sort/reverse/slice` |      1003 |    1406 | *0.7x*   | Allocation-heavy; TinyGo's simpler GC beats Go's concurrent GC at this scale.  |
| `struct list build+read`  |        60 |      36 | 1.7x     | Post-M16.5.1: append hot loop is O(1). Both binaries are effectively free.     |
| `string join`             |        19 |      11 | 1.7x     | Post-M16.5.1: build-up-a-string pattern is O(1). Both binaries are free.       |
| `map insert+read`         |      1690 |    1197 | **1.4x** | Go's runtime map implementation outperforms TinyGo's at this churn rate.       |
| **total**                 |     48896 |   25197 | **1.9x** | Compute-bound average wins for Go; alloc-heavy workloads are cheap on both.    |

**Pre-M16.5.1 comparison, same workloads:** the `struct list build+read`
row measured 10752 ms (tiny) / 12126 ms (go); `string join` was
4189 / 3916; `map insert+read` was 9768 / 7010. Every one of
those workloads was dominated by the compound-write O(N^2)
deep-copy pattern that shared-marker COW eliminated. The
2x-3x reduction in the serial totals
(74.5 s -> 48.9 s tiny, 45.5 s -> 25.2 s go) comes almost
entirely from those three workloads.

### Parallel section

Ratios in the "speedup" column are `serial_ms / par_ms`; > 1.0
means the four-worker parallel version was faster than the
single-threaded serial version on the same workload.

| Workload             | Go serial (ms) | Go par (ms) | Go speedup | TinyGo serial (ms) | TinyGo par (ms) | TinyGo speedup |
| -------------------- | -------------- | ----------- | ---------- | ------------------ | --------------- | -------------- |
| `primes up to LIMIT` |          21067 |        7940 |   **2.65** |              42421 |           62359 |     *0.68*     |
| `newton sqrt batch`  |            337 |          94 |   **3.59** |                779 |            1087 |     *0.72*     |
| `monte carlo pi`     |           1054 |         338 |   **3.12** |               2509 |            3443 |     *0.73*     |
| `fib(N) x workers`   |            356 |         111 |   **3.21** |               1660 |            1646 |     1.01       |

The default `jennifer` binary lands close to the 4x ceiling for
four workers on numeric workloads (3.1x-3.6x). `jennifer-tiny`
is *slower than serial* on the same workloads - the runtime
cost of coordinating four cooperative goroutines outweighs the
work being distributed. This is the practical face of the
"TinyGo has no multi-core scheduler" caveat: `spawn` is
functionally correct on `jennifer-tiny` (values, error paths,
loud-fail all work) but throughput-negative under contention.

The pattern that matters when picking a binary:

- **CPU-bound interpreter dispatch (recursive / numeric loops)**:
  `jennifer` wins by 2-5x on the serial section. The TinyGo
  runtime's slower per-call overhead shows here because the
  work is "the same handful of `Value` operations, repeated."
- **Allocation-heavy workloads (lists, structs)**: post-M16.5.1
  both binaries are essentially free on these. The historical
  10-second bars are gone.
- **Stdlib-dominated workloads (strings, maps)**: `jennifer`
  edges ahead on map churn where Go's runtime map is more
  aggressive about reseating buckets.
- **Parallel workloads**: use `jennifer`. `jennifer-tiny`'s
  scheduler contention makes `spawn` performance-negative
  under four-way fan-out; reserve concurrency for the default
  binary.
- **Wall-clock vs CPU time**: Go uses extra cores for GC and
  for spawn workers, so `real` can be 30%+ shorter than
  `user`. TinyGo's `sys` column ballooning to ~90 s in the
  parallel section is the runtime paying for futex-based
  goroutine coordination.

The benchmark's serial section is single-threaded by design (the
milestone-stated goal is to stress the evaluator dispatch loop,
not parallelism). **M16.0 shipped the multi-core parallel
counterparts** and their numbers appear in the parallel table
above - see
[milestones.md > M16.0](../milestones.md#m160---lightweight-concurrency).
