# TinyGo notes

Jennifer ships as two binaries built from the same source:

- `jennifer` - default, built with the standard Go toolchain.
  Full host-feature surface: file I/O, `os/exec`, network stack,
  everything.
- `jennifer-tiny` - constrained variant, built with TinyGo. Smaller
  binary, embeddable; the stock build ships without `os/exec` or a
  network stack (a build choice, not a hard TinyGo limit - see
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
  [CLI > Inspection](cli_inspect.md#inspection-tokens-and-ast)).
- **Goroutines are allowed** (used for `spawn`),
  but need `-stack-size=2mb` under TinyGo - see
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
| `os`    | `os.run`, `os.spawn`, `os.wait`, `os.poll`, `os.kill` | Runtime error pointing at the default `jennifer` binary. The `os/exec` subprocess surface: unimplemented in TinyGo on host targets, and absent by nature on embedded / WASM. Not the same "recompile" story as `net` - see the note below. |
| `net`   | Every entry point (TCP, UDP, DNS)                     | Runtime error pointing at the default `jennifer` binary. Our stock `jennifer-tiny` registers no netdev driver, so `net` is stubbed. Build-tag split: `netlib_tinygo.go` returns friendly errors. Not a hard TinyGo limit - see the note below. |
| `httpd` | Every entry point (`listen`, `accept`, `respond`, ...) | Runtime error pointing at the default `jennifer` binary. The HTTP/1.1 server engine is over Go `net/http`, so it is stubbed for the same reason as `net` (no netdev driver): `httpdlib_tinygo.go` returns friendly errors, and a rebuild with a network stack restores it. |
| `term`  | Every entry point (`makeRaw`, `restore`, `size`, `readByte`) | Runtime error pointing at the default `jennifer` binary. Terminal control needs `golang.org/x/term` (which the tiny build excludes) *and* a controlling TTY (which a minimal / embedded target may not have). Build-tag split like `net`: `termlib_tinygo.go` returns friendly errors. |
| `serial` | Every entry point (`open`, `read`, `write`, ...) | Runtime error pointing at the default `jennifer` binary on Linux. Serial-port termios I/O is a Linux `/dev` + ioctl feature over `golang.org/x/sys/unix`, which the tiny build does not carry. Build-tag split `linux && !tinygo` (real) / else (stub). |
| `spi`   | Every entry point (`open`, `configure`, `transfer`, `close`) | Runtime error pointing at the default `jennifer` binary on Linux. `SPI_IOC_MESSAGE` ioctl; same build-tag split as `serial`. |
| `iic`   | Every entry point (`open`, `read`, `write`, `readReg`, ...) | Runtime error pointing at the default `jennifer` binary on Linux. `I2C_SLAVE` ioctl; same build-tag split as `serial`. |
| `gpio`  | Every entry point (`setup`, `read`, `write`, `release`, `chip`) | Runtime error pointing at the default `jennifer` binary on Linux. The `/dev/gpiochipN` GPIO v2 line ioctls; same build-tag split as `serial`. (The sysfs-backed `gpio` **module** is the portable default and runs on both binaries.) |
| `sql`   | Every entry point (`open`, `query`, `exec`, ...) | Runtime error pointing at the default `jennifer` binary. The MySQL / Postgres drivers are heavyweight dependency trees that TinyGo does not compile, so `sqllib_tiny.go` stubs the whole surface (they are unreachable on stock `jennifer-tiny` anyway - no network stack). Build-tag split like `net`: `sqllib_std.go` (`!tinygo`) imports the drivers, `sqllib_tiny.go` (`tinygo`) returns friendly errors. |
| `crypto` | **Only** the RSA / ECDSA surface: `rsaSign` / `rsaVerify` / `ecdsaSign` / `ecdsaVerify`, plus `rsaGenerateKey` / `ecGenerateKey` / `jwkPublic` / `csr` | Runtime error pointing at the default `jennifer` binary. These pull in `crypto/rsa`, `crypto/ecdsa`, and `crypto/x509` (PEM parsing / CSR), which are off the TinyGo build; `cryptolib_asym_tiny.go` / `cryptolib_acme_tiny.go` stub them. The rest of `crypto` - random, `hmacEqual`, HKDF / PBKDF2, AES-GCM, and Ed25519 `sign` / `verify` - runs on **both** binaries. (So JWT's HS\* / EdDSA work on `jennifer-tiny` but RS\* / ES\* do not, and `acme` needs the default binary.) |

### `net` on TinyGo is a build choice, not a hard limit

The "no network" state is a property of the **stock `jennifer-tiny`
build**, not of TinyGo itself. TinyGo compiles most of `net.Dial` /
`net.Listen`; what it does not do on a default target is register a
**netdev driver** at runtime (the pluggable network device interface its
`net` package dials through), and our stock build ships none - so we
compile the `tinygo`-tagged `netlib_tinygo.go` stub that returns a
friendly error instead of failing cryptically deep in Go's `net`.

Anyone who needs networking on the tiny binary can restore it by
**rebuilding with a network stack**: target (or link in) a registered
netdev driver - or a net-capable target such as one exposing a host
socket layer - and drop the `tinygo` build tag on `net` so the real
implementation compiles in. With a network stack present, `net` and
**every net-backed module** (`smtp`, `pop`, `imap`, `redis`, `resque`,
`memcache`, `session`, `ratelimit`, ...) run on `jennifer-tiny` too. So
read "needs the default `jennifer` binary" as "needs a build that
includes a network stack" - the stock `jennifer` has one, the stock
`jennifer-tiny` does not. (UDP is the one genuinely thinner spot:
`net.ListenPacket` is not part of TinyGo's surface today, so a rebuild
covers TCP / DNS more readily than UDP.)

### `os/exec` on TinyGo is a platform limit, not a switch

The `os` restriction is narrower than it looks: it is only the **`os/exec`
subprocess surface** - `os.run` / `os.spawn` / `os.wait` / `os.poll` /
`os.kill`. Everything else in `os` (env, args, flags, the `PLATFORM` /
`ARCH` / `EOL` / `DIRSEP` / `PATHSEP` / `ARGS` values) works fully on both
binaries.

Do **not** read the `net` note above as applying here. `net` needs a
pluggable *driver* you can supply; `os/exec` needs a whole **host operating
system with a process model** - fork/exec, a process table, executables on a
filesystem. There is no component to link in. Two cases:

- **Host-OS TinyGo target (Linux / macOS / Windows):** a TinyGo
  **standard-library maturity gap** - the `os/exec` fork/exec path is not
  implemented yet. If TinyGo upstream adds it, a host-targeted
  `jennifer-tiny` could gain `os.run` / `os.spawn`; that is upstream work,
  not a rebuild switch on our side.
- **Embedded / bare-metal / WASM / WASI targets** (what `jennifer-tiny`
  exists for): there is **no process model at all** - nothing to fork, no
  other programs, no `exec` syscall. So the subprocess surface is
  *fundamentally inapplicable*, a hard platform limit rather than a missing
  piece. It stays unavailable there, permanently.

This also fits the deployment target: minimal containers and embedded
scripting hosts generally should *not* shell out to external processes (no
shell, no other executables), so the restriction aligns with where
`jennifer-tiny` runs rather than fighting it. In short: `net` = a driver you
can supply and rebuild around; `os/exec` = a host capability that is a TinyGo
gap on host targets and simply absent on embedded / WASM.

The constants and the env / argv / flag helpers in `os`
(`os.PLATFORM`, `os.ARCH`, `os.EOL`, `os.DIRSEP`, `os.PATHSEP`,
`os.ARGS`, `os.getEnv`, `os.hasFlag`, `os.flag`) all work fully
on both binaries. Every shipped library except the three stubbed ones above
(`net`, `httpd`, `term`) and the `os/exec` slice of `os` has **full TinyGo
support on both binaries** - `io`, `convert`, `math`, `strings`, `lists`,
`maps`, `meta`, `time`, `hash`, `crc`, `crypto`, `compress`, `archive`,
`encoding`, `json`, `toml`, `xml`, `yaml`, `intl`, `task`, `fs`, `regex`,
`testing`, and `uuid`. `yaml` is the one carrying a third-party dependency
(`gopkg.in/yaml.v3`), verified to build *and* run under TinyGo 0.41; every
other library is Go standard library or hand-rolled.

**Development subcommands are default-binary only.** `jennifer-tiny`
is a run-only interpreter: `run` and `repl` execute Jennifer source,
but the development subcommands `tokens`, `ast`, `fmt`, `lint`, `profile`, and `test` are
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
`-stack-size=2mb` to `tinygo build` for `jennifer-tiny` so it
can run recursive `spawn` bodies (and the parallel section of
`examples/benchmark.j`). The default `jennifer` binary doesn't
need this - Go's goroutine stacks grow automatically.

That fixed 2 MB stack also sets a hard ceiling on how deeply the
tree-walker can recurse over *nested data*: a deeply-nested source
literal, or a deeply-nested `json` / `toml` / `xml` document, drives
one recursive descent per level, and the interpreter has no
`recover()`, so an overflow is a fatal crash rather than a catchable
error. Every recursive-descent parser (the language parser and the
three hand-rolled decoders) therefore enforces a shared nesting cap,
`internal/limits.MaxNestingDepth`. It is build-tag split: 1000 on the
default binary (growable stack), and 64 on `jennifer-tiny`, which sits
below the depth where the heaviest shape (a nested map literal)
overflows the 2 MB stack (empirically it survives 96 and segfaults near
128). Exceeding the cap is a positioned parse error / catchable decode
error on both binaries. `yaml` keeps its own pre-parse guard (it is
backed by a Go dependency, not a hand-rolled descent).

**TinyGo scheduler**. `jennifer-tiny` pins the cooperative
single-thread scheduler (`-scheduler=tasks` in the Makefile).
`spawn` works fully (semantics, loud-fail, registry), but every
goroutine shares one OS thread, so it gives concurrency without
multi-core parallelism: **parallel speedups stay close to 1.0**,
and `-stack-size=2mb` reliably covers recursive `spawn` bodies.
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
`make build` on linux/amd64 (Go 1.26.5, TinyGo 0.41.1, unstripped).
The absolute numbers move with toolchain version and platform; the
ratio is the stable part.

| Binary          | Size                        |
| --------------- | --------------------------- |
| `jennifer`      | ~21.6 MB (21,594,881 bytes) |
| `jennifer-tiny` | ~8.3 MB (8,282,216 bytes)   |

`jennifer-tiny` comes in at ~38% of the default binary (well under half
the size). Most of that gap is TinyGo's smaller runtime versus the
standard Go runtime, plus the network-, `os/exec`-, and
database-driver-backed libraries the tiny build stubs out; the run-only
trim (excluding the
`tokens` / `ast` / `fmt` / `lint` / `profile` / `test` development subcommands) shaves an
incremental slice on top. The gap has widened as the default binary
grew (the database drivers and crypto surface land only on the standard
build), so the ratio drifts with each library the tiny build stubs.

These are unstripped `make build` (dev) sizes. Release builds strip:
the Go binary adds `-trimpath -ldflags "-s -w"` (down to ~14.8 MB, a
~31% cut) and the TinyGo binary adds `-no-debug` (down to ~3.4 MB, a
~59% cut). Shipped artifacts are therefore well under the dev numbers
above; dev builds keep symbols for debugging.

## Single-binary benchmark results

Reference numbers from `examples/benchmark.j` (version
`0.19.0-dev+19.d8e8107`) on an **AMD Ryzen 5 7600X3D** (6 cores, 12
threads; desktop active, load low) - the machine the suite prints in its
own header, from `os.NCPU` plus a `/proc/cpuinfo` read done in Jennifer.
The serial section is single-threaded by design; the parallel section fans
out to `PARALLEL_WORKERS = 4` spawn tasks per workload. The interpreter
build is the current one (eager-copy value semantics, lexical slot
resolution, parse-time constant folding, and the advisory map hash index),
so append-in-a-loop is amortised O(N) with in-place growth and keyed
map access is O(1).

### `jennifer` (standard-Go binary, default)

```
=== Jennifer benchmark suite ===
build:    go
version:  0.19.0-dev+19.d8e8107
cpu:      AMD Ryzen 5 7600X3D 6-Core Processor (12 cores)
platform: linux/amd64

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           73
primes up to LIMIT                   100000            1        16645
newton sqrt batch                     10000        10000          300
monte carlo pi                       500000       500000          850
list sort/reverse/slice               10000          500          977
struct list build+read                10000        10000           27
string join                           10000        10000           12
map insert+read                       10000        10000           38
----------------------------------------------------------------------
total                                                           18922

Parallel comparison (workers = 4, scheduler = go)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                    16645         5481         3.04
newton sqrt batch                       300           75         4.00
monte carlo pi                          850          288         2.95
fib(N) x workers                        292           96         3.04
----------------------------------------------------------------------

user   37.24s
sys     0.19s
real   24.87s     (150% CPU)
```

`user + sys` (37.4s) exceeds `real` (24.9s) by ~12.6s - that gap is Go's
concurrent GC running on a second CPU during the serial section, plus the
four spawn workers running in parallel during the parallel section. Sys
time is tiny (0.2s) because Go's runtime coordinates goroutines with cheap
in-process sync primitives.

### `jennifer-tiny` (TinyGo binary)

```
=== Jennifer benchmark suite ===
build:    tinygo
version:  0.19.0-dev+19.d8e8107
cpu:      AMD Ryzen 5 7600X3D 6-Core Processor (1 cores)
platform: linux/amd64

----------------------------------------------------------------------
Workload                               base        iters      time_ms
----------------------------------------------------------------------
fib(N) recursive                         23            1           83
primes up to LIMIT                   100000            1        15032
newton sqrt batch                     10000        10000          260
monte carlo pi                       500000       500000          851
list sort/reverse/slice               10000          500          871
struct list build+read                10000        10000           32
string join                           10000        10000           10
map insert+read                       10000        10000           44
----------------------------------------------------------------------
total                                                           17183

Parallel comparison (workers = 4, scheduler = tinygo)
----------------------------------------------------------------------
Workload                          serial_ms       par_ms      speedup
----------------------------------------------------------------------
primes up to LIMIT                    15032        15025         1.00
newton sqrt batch                       260          260         1.00
monte carlo pi                          851          862         0.99
fib(N) x workers                        332          280         1.19
----------------------------------------------------------------------

user   33.51s
sys     0.02s
real   33.62s     (99% CPU)
```

This is the pinned build (`-scheduler=tasks -stack-size=2mb`): the whole
suite completes, serial and parallel. `os.NCPU` reports `1` here - honest
about the cooperative single-thread scheduler's usable parallelism, not
the 12 threads the machine has. The parallel column hovers at ~1.0 by
design - `spawn` under that scheduler is concurrency, not multi-core
throughput - and `user ~= real` at 99% CPU confirms the single-thread
execution (contrast Go's 150% above). The 2MB stack (up from 1MB) is what
lets the serial recursive `fib` run at all: at 1MB it fit bare but
overflowed nested one call frame deeper, inside `benchFib`.

### Per-workload comparison (serial section)

Ratios are `tiny_ms / go_ms`; > 1.0 means `jennifer-tiny` is slower, < 1.0 means it is faster.

| Workload                  | tiny (ms) | go (ms) | Ratio  | Where the time goes                                                            |
| ------------------------- | --------- | ------- | ------ | ------------------------------------------------------------------------------ |
| `fib(N) recursive`        |        83 |      73 | 1.1x   | Tight interpreter dispatch loop; Go a hair ahead.                              |
| `primes up to LIMIT`      |     15032 |   16645 | *0.9x* | Long numeric dispatch loop; TinyGo pulls ahead at scale - and this row is most of the aggregate lead. |
| `newton sqrt batch`       |       260 |     300 | *0.9x* | Float arithmetic + dispatch; TinyGo ahead.                                     |
| `monte carlo pi`          |       851 |     850 | 1.0x   | Float arithmetic + RNG calls; effectively tied (1 ms).                         |
| `list sort/reverse/slice` |       871 |     977 | *0.9x* | Allocation-heavy; TinyGo's simpler GC beats Go's concurrent GC at this scale.  |
| `struct list build+read`  |        32 |      27 | 1.2x   | Append hot loop is O(1). Both binaries are effectively free (sub-40ms).        |
| `string join`             |        10 |      12 | *0.8x* | Build-up-a-string pattern is O(1); both free in absolute terms (sub-15ms).     |
| `map insert+read`         |        44 |      38 | 1.2x   | The advisory map hash index makes keyed insert+read O(1): both now sub-50ms, a ~30x cut from the pre-index build; Go a hair ahead. |
| **total**                 |     17183 |   18922 | *0.9x* | TinyGo posts the lower serial total; `primes` is most of the margin.           |

The two binaries stay close: TinyGo posts the lower serial total (17.2s vs
18.9s, ~9% faster), a narrower gap than the older ~13% as both builds
improved - the map hash index alone cut `map insert+read` ~30x, to sub-50ms
on both. The `primes` row alone is 1613 ms in TinyGo's favour (15032 vs
16645), which is most of the 1739 ms total gap; on the sum of every other
workload TinyGo is still ~126 ms ahead, so it now edges Go nearly
everywhere except the tiny structural rows. TinyGo wins the long tight
numeric loops (`primes`, `newton`), allocation-heavy `list`, and `string
join`; Go wins `fib`, the sub-40ms `struct` row, the now-tiny `map
insert+read` (by 6 ms), and ties `monte carlo`. The "longest dispatch loop
favours TinyGo, and it's big enough to tip the total" rule still holds.

### Parallel section

Speedup is `serial_ms / par_ms`; > 1.0 means the four-worker version beat
serial. Go gets real multi-core speedup; TinyGo's cooperative scheduler
stays at ~1.0 by design.

| Workload             | Go serial (ms) | Go par (ms) | Go speedup | TinyGo serial (ms) | TinyGo par (ms) | TinyGo speedup |
| -------------------- | -------------- | ----------- | ---------- | ------------------ | --------------- | -------------- |
| `primes up to LIMIT` |          16645 |        5481 |   **3.04** |              15032 |           15025 |      1.00      |
| `newton sqrt batch`  |            300 |          75 |   **4.00** |                260 |             260 |      1.00      |
| `monte carlo pi`     |            850 |         288 |   **2.95** |                851 |             862 |      0.99      |
| `fib(N) x workers`   |            292 |          96 |   **3.04** |                332 |             280 |      1.19      |

Go reaches real multi-core speedup (2.95x-4.00x on four workers).
`jennifer-tiny` pins the cooperative scheduler, so `spawn` there is
concurrency without multi-core throughput (~1.0 by design); use the default
binary when parallel throughput matters.

**This is where the serial-total lead reverses.** TinyGo has the lower
*serial* total (17.2s vs 18.9s), but Go finishes the *whole suite* in less
wall-clock time: `real` is **24.9s for Go vs 33.6s for TinyGo**. The
parallel section is why - Go crunches it in ~5.9s (four workers, real
speedup) where TinyGo takes ~16.4s (no parallelism), and that ~10.5s swing
more than erases TinyGo's ~1.7s serial edge. Lower single-thread compute
time does not mean a faster end-to-end run once any `spawn` parallelism is
in play.

Picking a binary, in short: TinyGo leads single-thread dispatch on the long
numeric loops and posts the lower serial total, but that lead lives mostly
in `primes`; both are essentially free on the small structural workloads;
Go wins the end-to-end wall clock whenever `spawn` parallelism is involved,
and is the only choice for real multi-core throughput; TinyGo trades that
for a larger resident-memory footprint (below).

### Memory and page faults

Same machine, `/bin/time` (GNU time) on an equivalent run (per-workload
timings within noise of the tables above):

| Metric              | `jennifer` (Go) | `jennifer-tiny` (TinyGo) |
| ------------------- | --------------- | ------------------------ |
| peak resident (RSS) | ~40 MB          | ~96 MB                   |
| minor page faults   | ~49,600         | ~16,100                  |
| CPU                 | 150%            | 99%                      |

The two runtimes trade opposite resources. **TinyGo uses ~2.4x the peak
RSS** - its cooperative scheduler reserves each goroutine's full
`-stack-size` up front (the four parallel `spawn` workers each hold a 2MB
stack, ~8MB before the interpreter's own data), where Go grows goroutine
stacks on demand from ~8KB. **Go, in turn, churns ~3x the page faults**
(~49.6k vs ~16.1k): its concurrent GC allocates and reclaims pages across
cores - the same activity behind its 150% CPU and the `user` >> `real` gap.
That ratio has fallen sharply from earlier builds' ~16x as the map / list
optimizations cut allocation churn. TinyGo's simpler GC touches far fewer
pages and runs single-threaded at 99% CPU. So TinyGo buys lower
single-thread compute and low GC churn with a larger, flatter memory
footprint; Go buys a smaller footprint and real multi-core parallelism with
heavier GC activity.
