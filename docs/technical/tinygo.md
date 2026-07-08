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
- **Goroutines are allowed** (used for `spawn`),
  but need `-stack-size=1mb` under TinyGo - see
  [the goroutine-stack note below](#tinygo-goroutine-stack).
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

**Development subcommands are default-binary only.** `jennifer-tiny`
is a run-only interpreter: `run` and `repl` execute Jennifer source,
but the development subcommands `tokens`, `ast`, `fmt`, `lint`, and `profile` are
build-tag-excluded (`cmd/jennifer/devtools_tinygo.go`) and return a
friendly error pointing at the default `jennifer` binary. They pull in
lexer-dump, AST-JSON, formatter, and lint machinery that a
minimal-footprint embedding has no use for; build the standard-Go
`jennifer` binary for development work.

**TinyGo goroutine stack**. Jennifer's tree-walking
evaluator wraps each Jennifer-level call in many Go-stack frames
(`execBlock` + `evalCall` + `evalExpr` + ...), so even a
modest-depth recursion (fib 23) easily exceeds TinyGo's default
goroutine stack of ~8KB and segfaults. The Makefile passes
`-stack-size=1mb` to `tinygo build` for `jennifer-tiny` so it
can run recursive `spawn` bodies (and the parallel section of
`examples/benchmark.j`). The default `jennifer` binary doesn't
need this - Go's goroutine stacks grow automatically.

**TinyGo scheduler**. `jennifer-tiny` pins the cooperative
single-thread scheduler (`-scheduler=tasks` in the Makefile).
`spawn` works fully (semantics, loud-fail, registry), but every
goroutine shares one OS thread, so it gives concurrency without
multi-core parallelism: **parallel speedups stay close to 1.0**,
and `-stack-size=1mb` reliably covers recursive `spawn` bodies.
The pin is deliberate - the threads-capable default briefly showed
real multi-core speedups (161% CPU) but segfaulted on recursive
`spawn` bodies, because `-stack-size` doesn't govern OS-thread
stacks. Real multi-core on `jennifer-tiny` is separate future
work, not a default flip; the default `jennifer` binary already
reaches multi-core speedup via Go's scheduler.

Future library work will grow the restrictions table if further
TinyGo runtime gaps surface. Each new gap lands with the same
friendly-message pattern.

## Binary size

The constrained build is the smaller one, which is the point of
`jennifer-tiny` targeting minimal-footprint deployments. Sizes from
`make build` on linux/amd64 (Go 1.26.3, TinyGo 0.41.1, unstripped).
The absolute numbers move with toolchain version and platform; the
ratio is the stable part.

| Binary          | Size                        |
| --------------- | --------------------------- |
| `jennifer`      | ~7.0 MB (7,250,750 bytes)   |
| `jennifer-tiny` | ~4.5 MB (4,621,624 bytes)   |

`jennifer-tiny` comes in at ~64% of the default binary (about a
third smaller). Most of that gap is TinyGo's smaller runtime versus
the standard Go runtime; the run-only trim (excluding the
`tokens` / `ast` / `fmt` / `lint` / `profile` development subcommands) shaves an
incremental slice on top.

These are unstripped `make build` (dev) sizes. Release builds strip:
the Go binary adds `-trimpath -ldflags "-s -w"` (down to ~5.0 MB, a
third smaller) and the TinyGo binary adds `-no-debug` (down to
~1.8 MB, a ~60% cut). Shipped artifacts are therefore well under the
dev numbers above; dev builds keep symbols for debugging.

## Single-binary benchmark results

Reference numbers from `examples/benchmark.j` on an **AMD Ryzen 5
7600X3D** (6 cores, 12 threads; desktop active, load low). The
serial section is single-threaded by design; the parallel section
fans out to `PARALLEL_WORKERS = 4` spawn tasks per workload. The
interpreter build is the current one (shared-marker COW, lexical
slot resolution, parse-time constant folding), so append-in-a-loop
is amortised O(1).

### `jennifer` (standard-Go binary, default)

```
=== Jennifer benchmark suite ===
build:   go
version: 0.15.0-dev+21.582825d

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           71
primes up to LIMIT                   100000            1        18058
newton sqrt batch                     10000        10000          290
monte carlo pi                       500000       500000          858
list sort/reverse/slice               10000          500         1736
struct list build+read                10000        10000           32
string join                           10000        10000           10
map insert+read                       10000        10000         1187
----------------------------------------------------------------------
total                                                           22242

Parallel comparison (workers = 4, scheduler = go)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                    18058         6117         2.95
newton sqrt batch                       290           78         3.72
monte carlo pi                          858          288         2.98
fib(N) x workers                        284           90         3.16
----------------------------------------------------------------------

user   42.05s
sys     0.39s
real   28.82s     (147% CPU)
```

`user + sys` (42.4s) exceeds `real` (28.8s) by ~13.6s - that
gap is Go's concurrent GC running on a second CPU during the
serial section, plus the four spawn workers running in
parallel during the parallel section. Sys time is tiny (0.4s)
because Go's runtime coordinates goroutines with cheap in-process
sync primitives.

### `jennifer-tiny` (TinyGo binary)

```
=== Jennifer benchmark suite ===
build:   tinygo
version: 0.15.0-dev+21.582825d

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           83
primes up to LIMIT                   100000            1        13957
newton sqrt batch                     10000        10000          238
monte carlo pi                       500000       500000          882
list sort/reverse/slice               10000          500         1125
struct list build+read                10000        10000           32
string join                           10000        10000           14
map insert+read                       10000        10000         1653
----------------------------------------------------------------------
total                                                           17984

Parallel comparison (workers = 4, scheduler = tinygo)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                    13957         7179         1.94
newton sqrt batch                       238          136         1.75
monte carlo pi                          882          814         1.08
fib(N) x workers            <run crashed here - SIGSEGV, signal 11>
----------------------------------------------------------------------

user   38.18s
sys     4.24s     (crashed: SIGSEGV before completing)
real   26.32s     (161% CPU)
```

**This is a pre-pin measurement**, taken before the Makefile
pinned `-scheduler=tasks`. The 161% CPU, the positive parallel
speedups, and the `fib` SIGSEGV are all the unpinned
threads-scheduler story from [TinyGo scheduler](#tinygo-scheduler)
above (`fib` is the only deeply recursive `spawn` body; the
iterative workloads finished). The serial section completed and
its numbers stand; under the pinned cooperative scheduler the
parallel column returns to ~1.0, to be refreshed on the next run.

### Per-workload comparison (serial section)

Ratios are `tiny_ms / go_ms`; > 1.0 means `jennifer-tiny` is slower, < 1.0 means it is faster.

| Workload                  | tiny (ms) | go (ms) | Ratio  | Where the time goes                                                            |
| ------------------------- | --------- | ------- | ------ | ------------------------------------------------------------------------------ |
| `fib(N) recursive`        |        83 |      71 | 1.2x   | Tight interpreter dispatch loop; TinyGo's per-call overhead still shows, but the gap is now small. |
| `primes up to LIMIT`      |     13957 |   18058 | *0.8x* | Long numeric dispatch loop; TinyGo edges ahead at scale.                       |
| `newton sqrt batch`       |       238 |     290 | *0.8x* | Float arithmetic + dispatch; TinyGo slightly ahead.                            |
| `monte carlo pi`          |       882 |     858 | 1.0x   | Float arithmetic + RNG calls; effectively tied.                                |
| `list sort/reverse/slice` |      1125 |    1736 | *0.6x* | Allocation-heavy; TinyGo's simpler GC beats Go's concurrent GC at this scale.  |
| `struct list build+read`  |        32 |      32 | 1.0x   | Append hot loop is O(1). Both binaries are effectively free.                   |
| `string join`             |        14 |      10 | 1.4x   | Build-up-a-string pattern is O(1). Both binaries are free; Go a hair ahead.    |
| `map insert+read`         |      1653 |    1187 | 1.4x   | Go's runtime map implementation outperforms TinyGo's at this churn rate.       |
| **total**                 |     17984 |   22242 | *0.8x* | Compute- and alloc-heavy averages now favour TinyGo; Go leads only on small stdlib-churn workloads. |

The story has inverted from earlier builds: the two binaries are
within ~1.4x on every workload, and `jennifer-tiny` now posts the
lower serial total (18.0s vs 22.2s). The old "Go wins CPU-bound
work by 2-5x" gap is gone - eliminating the compound-write O(N^2)
deep-copy pattern (which once pushed the tiny total to ~49s) plus
the dispatch-path optimizations closed it.

Two things stand behind the inversion. The dispatch-path work
(constant folding, slot resolution, the `Share()` scalar fast
path, COW) cut per-call cost, and TinyGo's per-call overhead was
higher to start, so it gained *disproportionately* - its serial
total fell ~49s -> 18s (~2.7x) while Go moved only ~25s ->
22s. And tiny's lead is *concentrated*: of the 4.3s gap,
`primes` alone is ~4.1s (the tightest, longest dispatch loop),
with `list` adding most of the rest. Go still wins the
stdlib-churn workloads (`map insert+read`, `string join`), so
"tiny is faster" is real but narrow.

### Parallel section

Speedup is `serial_ms / par_ms`; > 1.0 means the four-worker
version beat serial. **The TinyGo columns are the same pre-pin
measurement** - multi-core speedups and a crashed `fib` row from
the unpinned build; under the pinned `-scheduler=tasks` they
return to ~1.0.

| Workload             | Go serial (ms) | Go par (ms) | Go speedup | TinyGo serial (ms) | TinyGo par (ms) | TinyGo speedup |
| -------------------- | -------------- | ----------- | ---------- | ------------------ | --------------- | -------------- |
| `primes up to LIMIT` |          18058 |        6117 |   **2.95** |              13957 |            7179 |     1.94*      |
| `newton sqrt batch`  |            290 |          78 |   **3.72** |                238 |             136 |     1.75*      |
| `monte carlo pi`     |            858 |         288 |   **2.98** |                882 |             814 |     1.08*      |
| `fib(N) x workers`   |            284 |          90 |   **3.16** |               crashed (SIGSEGV) | - | -             |

`*` pre-pin, threads-capable build; ~1.0 under the pinned cooperative scheduler.

Go reaches real multi-core speedup (2.95x-3.72x on four workers).
`jennifer-tiny` pins the cooperative scheduler, so `spawn` there
is concurrency without multi-core throughput (~1.0 by design); use
the default binary when parallel throughput matters.

Picking a binary, in short: the two are close on CPU-bound
dispatch (tiny can edge ahead on long numeric loops); both are
essentially free on allocation-heavy workloads (tiny's simpler GC
slightly ahead); Go leads on string/map churn; and Go is the
choice for parallel `spawn`.
