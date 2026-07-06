# TinyGo notes

The interpreter ships as a TinyGo binary named `jennifer`.
`make build` produces both binaries side by side:
the TinyGo `jennifer` (shipping) and the standard-Go `jennifer-go`
(dev/full-feature). To produce only one, use `make build-tinygo`
or `make build-go`. All three regenerate the version file before
compiling.

A few constraints shape the implementation:

- **No `reflect`-heavy code.** Tagged-union `Value` instead of interfaces
  with type assertions in hot paths.
- **No `text/template`, no goroutines in the interpreter core.** Not
  needed yet, but worth not introducing accidentally.
- **No `encoding/json` for in-binary serialization.** The reflect-based
  marshaler is fragile under TinyGo, so the AST JSON emitter is
  hand-rolled (see [CLI > Inspection](cli.md#inspection-tokens-and-ast)).
- **No `-ldflags "-X package.var=value"`.** TinyGo 0.41 silently ignores
  the `-X` directive. Use the codegen path
  (`scripts/gen-version.sh` -> `internal/version/version_gen.go`) for
  build-time string injection. See [CLI > Version injection](cli.md#version-injection).
- **No hard dependencies on a hosted runtime.** A long-term goal is to
  embed the interpreter into the **McFly OS** kernel (also TinyGo), so
  ambient stdin, dynamic linking, and the like should not be assumed.
- **`testing` runs under regular `go test`.** TinyGo's `testing` support
  is partial; we develop and verify with `go test ./...`.

Verify both builds after non-trivial changes:

```sh
make build
./jennifer run examples/hello.j     # TinyGo binary
./jennifer-go run examples/hello.j  # Go binary (full host features)
```

## TinyGo restrictions

A few standard-library features depend on TinyGo runtime support
that isn't there yet. When called from the `jennifer` (TinyGo)
binary they error with a friendly Jennifer-level message pointing
the user at `jennifer-go`. The standard-Go binary always supports
the full surface.

| Library | Affected names                                        | What happens on TinyGo                                                                       |
| ------- | ----------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| `os`    | `os.run`, `os.spawn`, `os.wait`, `os.poll`, `os.kill` | Runtime error pointing at `jennifer-go`. TinyGo's `os/exec` syscalls aren't implemented yet. |
| `net`   | Every entry point (TCP, UDP, DNS)                     | Runtime error pointing at `jennifer-go`. TinyGo 0.41 needs a netdev driver at runtime (not registered) and has no `net.ListenPacket` for UDP. Build-tag split: `netlib_tinygo.go` returns friendly errors. |

The constants and the env / argv / flag helpers in `os`
(`os.PLATFORM`, `os.ARCH`, `os.EOL`, `os.DIRSEP`, `os.PATHSEP`,
`os.ARGS`, `os.getEnv`, `os.hasFlag`, `os.flag`) all work fully on
both binaries. Every other shipped library (`io`, `convert`,
`math`, `strings`, `lists`, `maps`, `meta`, `time`, `hash`,
`crc`, `encoding`, `task`, `fs`) has full TinyGo support.

**M16.0 / TinyGo goroutine stack**. Jennifer's tree-walking
evaluator wraps each Jennifer-level call in many Go-stack frames
(`execBlock` + `evalCall` + `evalExpr` + ...), so even a
modest-depth recursion (fib 23) easily exceeds TinyGo's default
goroutine stack of ~8KB and segfaults. The Makefile passes
`-stack-size=1mb` to `tinygo build` for `jennifer` so the shipping
binary can run recursive `spawn` bodies (and the parallel section
of `examples/benchmark.j`). `jennifer-go` doesn't need this -
Go's goroutine stacks grow automatically.

**M16.0 / TinyGo scheduler**. TinyGo's runtime scheduler is
cooperative and (as of 0.41) does not exploit multi-core
hardware: every goroutine runs on the same OS thread.
`spawn` works correctly under TinyGo - the semantics, the
loud-fail, the registry - but **parallel speedups will be close
to 1.0**. The standard-Go binary `jennifer-go` uses Go's regular
scheduler and does benefit from multiple cores. The benchmark
output reports the scheduler name in the parallel-section header
so the speedup numbers can be read in context.

Future library work in `fs` (M16.1) and `net` (M16.2) will hit the
same boundary and will land with the same friendly-message pattern.
The table will grow as those ship.

## Single-binary benchmark results

Reference numbers from `examples/benchmark.j` run as a single
process on an **AMD Ryzen 5 7600X3D** (6 cores, 12 threads;
Linux + KDE Plasma desktop active during the run, total CPU load
low). The serial section of the script is single-threaded; the
only place a second CPU enters the picture is Go's concurrent
garbage collector, which the table below illustrates. The
parallel section (added M16.0) fans out to `PARALLEL_WORKERS`
spawn tasks per workload and prints serial-vs-parallel speedups
in a separate table; see the driver output for those numbers.
The reference dump below captures only the serial section.

### `jennifer` (TinyGo binary)

```
=== Jennifer benchmark suite ===
build:   tinygo
version: 0.14.0-dev+8.ce22fcc

----------------------------------------------------------------------
Workload                             base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                       23            1          435
primes up to LIMIT                 100000            1        44876
newton sqrt batch                   10000        10000          783
monte carlo pi                     500000       500000         2362
list sort/reverse/slice             10000          500         1293
struct list build+read              10000        10000        10752
string join                         10000        10000         4189
map insert+read                     10000        10000         9768
----------------------------------------------------------------------
total                                                         74458

real    1m14.468s
user    1m13.846s
sys     0m0.203s
```

`real` and `user` track within 1% of each other; `sys` is
negligible. TinyGo's GC is non-concurrent, so the run is
single-core start to finish. CPU saturation is one logical core,
the rest of the machine is idle.

### `jennifer-go` (standard-Go binary)

```
=== Jennifer benchmark suite ===
build:   go
version: 0.14.0-dev+8.ce22fcc

----------------------------------------------------------------------
Workload                             base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                       23            1           84
primes up to LIMIT                 100000            1        19431
newton sqrt batch                   10000        10000          323
monte carlo pi                     500000       500000          994
list sort/reverse/slice             10000          500         1635
struct list build+read              10000        10000        12126
string join                         10000        10000         3916
map insert+read                     10000        10000         7010
----------------------------------------------------------------------
total                                                         45519

real    0m45.529s
user    0m59.677s
sys     0m6.586s
```

`user + sys` (66.3 s) exceeds `real` (45.5 s) by ~21 s - that
gap is Go's concurrent GC running on a second CPU while the main
interpreter goroutine continues. The script's program logic is
still single-threaded; the second core is consumed by the
runtime, not by user-visible parallelism. Sys time is also much
higher (6.6 s vs 0.2 s) because the concurrent GC issues more
memory-management syscalls.

### Per-workload comparison

Ratios are `tinygo_ms / go_ms`; > 1.0 means TinyGo is slower.

| Workload                  | TinyGo (ms) | Go (ms) | Ratio   | Where the time goes                                                            |
| ------------------------- | ----------- | ------- | ------- | ------------------------------------------------------------------------------ |
| `fib(N) recursive`        |         435 |      84 | **5.2x** | Tight interpreter dispatch loop; TinyGo's runtime call overhead dominates.    |
| `primes up to LIMIT`      |       44876 |   19431 | **2.3x** | Same dispatch loop, more iterations; the gap stabilises.                       |
| `newton sqrt batch`       |         783 |     323 | **2.4x** | Float arithmetic + dispatch; same shape as primes.                             |
| `monte carlo pi`          |        2362 |     994 | **2.4x** | Float arithmetic + RNG calls; identical pattern.                               |
| `list sort/reverse/slice` |        1293 |    1635 | *0.8x*   | Allocation-heavy; TinyGo's simpler GC beats Go's concurrent GC at this scale. |
| `struct list build+read`  |       10752 |   12126 | *0.9x*   | Same story - the alloc path dominates over dispatch.                           |
| `string join`             |        4189 |    3916 | 1.1x    | Roughly equal; bottleneck is the stdlib's UTF-8 / strings paths.               |
| `map insert+read`         |        9768 |    7010 | **1.4x** | Go's runtime map implementation outperforms TinyGo's at this churn rate.       |
| **total**                 |       74458 |   45519 | **1.6x** | Compute-bound average wins for Go; alloc-heavy workloads favour TinyGo.       |

The pattern that matters when picking a binary:

- **CPU-bound interpreter dispatch (recursive / numeric loops)**:
  Go wins by 2-5x. The TinyGo runtime's slower per-call overhead
  shows here because the work is "the same handful of `Value`
  operations, repeated."
- **Allocation-heavy workloads (lists, structs)**: TinyGo
  matches or beats Go. Go's concurrent GC has extra bookkeeping
  that doesn't pay back at the working-set sizes used here;
  TinyGo's simpler collector spends fewer cycles per allocation.
- **Stdlib-dominated workloads (strings, maps)**: roughly equal,
  with Go pulling ahead on map churn where its runtime map
  implementation is more aggressive about reseating buckets.
- **Wall-clock vs CPU time**: Go uses a second core for GC, so
  `real` can be 30%+ shorter than `user`. TinyGo stays
  single-core. If you constrained both to one CPU (e.g.
  `taskset -c 0`) the gap would narrow.

The benchmark is single-threaded by design (the milestone-stated
goal is to stress the evaluator dispatch loop, not parallelism).
**M16.0 ships multi-core counterparts to each workload** so
reviewers can see how much speedup the new `spawn` primitive
actually delivers on each shape - see
[milestones.md > M16.0](../milestones.md#m160---lightweight-concurrency).
