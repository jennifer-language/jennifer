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
  but need `-stack-size=4mb` under TinyGo - see
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
`-stack-size=4mb` to `tinygo build` for `jennifer-tiny` so it
can run recursive `spawn` bodies (and the parallel section of
`examples/benchmark.j`). The default `jennifer` binary doesn't
need this - Go's goroutine stacks grow automatically.

That fixed 4 MB stack also sets a hard ceiling on how deeply the
tree-walker can recurse over *nested data*: a deeply-nested source
literal, or a deeply-nested `json` / `toml` / `xml` document, drives
one recursive descent per level, and the interpreter has no
`recover()`, so an overflow is a fatal crash rather than a catchable
error. Every recursive-descent parser (the language parser and the
three hand-rolled decoders) therefore enforces a shared nesting cap,
`internal/limits.MaxNestingDepth`. It is build-tag split: 1000 on the
default binary (growable stack), and 64 on `jennifer-tiny`, which sits
below the depth where the heaviest shape (a nested map literal)
overflows the stack (at the earlier 2 MB it survived 96 and segfaulted
near 128; the 4 MB stack roughly doubles that floor, so 64 has even more
margin). Exceeding the cap is a positioned parse error / catchable decode
error on both binaries. `yaml` keeps its own pre-parse guard (it is
backed by a Go dependency, not a hand-rolled descent).

The same 4 MB ceiling limits how deep *Jennifer method calls* can nest at
runtime - each call stacks many Go frames in the tree-walker - so the
interpreter enforces a second, sibling cap in the same package,
`internal/limits.MaxCallDepth`, in `evalCall`. It too is build-tag split, but
lower than the nesting cap because a call frame is heavier than one nesting
step: 10000 on the default binary (a heavy recursive body crashes Go's growable
stack near 50k), and 48 on `jennifer-tiny` (whose stack was raised from 2 MB to
4 MB for this, on which a fib-shaped or heavy body segfaults near depth 75, while
the deepest recursion a shipped example reaches - `examples/benchmark.j`'s serial
`fib(23)` - is depth 24). Exceeding it
is a catchable "call stack too deep" runtime
error - the analogue of Python's `RecursionError` - instead of a segfault; it
guards `spawn` bodies on their own goroutines too.

**TinyGo scheduler**. `jennifer-tiny` pins the cooperative
single-thread scheduler (`-scheduler=tasks` in the Makefile).
`spawn` works fully (semantics, loud-fail, registry), but every
goroutine shares one OS thread, so it gives concurrency without
multi-core parallelism: **parallel speedups stay close to 1.0**,
and `-stack-size=4mb` reliably covers recursive `spawn` bodies.
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

## Benchmarks

Single-binary throughput - `jennifer` (Go) vs `jennifer-tiny` (TinyGo) across the
serial and parallel workloads of `examples/benchmark.j`, plus the memory / page-fault
trade - lives in its own doc: [benchmark.md](benchmark.md).
