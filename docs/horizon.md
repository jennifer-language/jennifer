# Beyond 1.0.0 - idea collection

Jennifer's near-term target is a rich, dependable set of language
features, libraries, and modules - enough to tag a **1.0.0**. That work is
tracked in [milestones.md](milestones.md). This file collects ideas for
*after* 1.0.0: directions worth recording so the design doesn't foreclose
them, none committed to a timeline.

Two kinds:

- **[Drafts](#drafts)** - concrete, already-shaped directions, grouped by theme
  and ordered roughly easiest-first. Each has a design; it just has no schedule.
- **[Loose ideas](#loose-ideas)** - a grab-bag of smaller or vaguer
  possibilities, loosely grouped, jotted down when they come up so they are not
  lost.

Nothing here is a commitment. An idea graduates into
[milestones.md](milestones.md) if and when it earns a slot.

## Drafts

Shaped directions, **grouped by theme** and ordered roughly **easiest to
implement first** - language tweaks and additive libraries near the top, the big
runtime and ecosystem restructures near the bottom. Each has a design; it just
has no schedule.

Each carries a stable **`DRAFT#`** handle so it can be referenced and later
graduated into a numbered milestone (graduating a `DRAFT#` gives it an `M`-number
in `milestones.md`). Handles are **assigned once and retired on graduation** -
never reused, and **never renumbered when the list is reordered** - so a reference
stays valid for the life
of the idea (which is why the numbers below run out of sequence). `DRAFT#` is
deliberately *not* a milestone number; an idea only gets an `M`-number when it
graduates into [milestones.md](milestones.md).

Each draft also states its **Requires** - the `DRAFT#` handles and/or milestones
(`M`-numbers) that must land first, or *none* - so its blockers are explicit
before it is scheduled.

### Concurrency

#### DRAFT#25 - Go-side concurrent HTTP serve loop

The `web` framework dispatches each request into an entry-program handler by name
through `meta.callMain`, and `web.serveOn` runs that dispatch in a `.j` loop. Once
`web` handles requests concurrently (each wrapped in a `spawn`), an optional
refinement is to move only the **accept + bounded-worker-pool loop** into Go, as
an `httpd` primitive (e.g. `httpd.serveLoop(server, handlerName)`, or a small
engine-owned pool) that calls `Interpreter.CallHostWith` per request. Routing,
middleware, and `web.Context` stay in `.j`.

Why it might pay: a worker pool with a tunable ceiling is more natural in Go than
expressed as `.j` `spawn`s; back-pressure / queue-depth limits live where the
sockets already are (`httpd` already bounds `maxInFlight`); and the hot accept
loop stops paying tree-walker overhead per request. Why it is only a refinement,
not a fix: a handler is still a `.j` method invoked on the shared host
interpreter, so making that invocation race-safe is the actual prerequisite and
does the real work - this only relocates the loop. Keep `web` a `.j` module
either way: dogfooding is a first-class goal, and with no closures a handler is
always a by-name entry method, so a Go rewrite of the router would buy little and
lose test surface.

### Embedding, WASM, and sandboxing

#### DRAFT#1 - Public interpreter API for third-party embedding

Extract the interpreter core out from under `internal/` and expose a
documented Go-side surface so external programs can embed Jennifer. Today
`internal/interpreter`, `internal/parser`, `internal/lexer`, and
`internal/lib/*` are unreachable from any module that isn't
`jennifer-lang.dev/jennifer` - Go's `internal/` visibility rule is not a
convention, it's a compile-time barrier. No submodule / require / replace
workaround exists; embedding is impossible without a restructure.

It comes ahead of the WASM runtime (`DRAFT#2`) because a Go-side embedding
API is a strictly smaller change (repository restructure, no new external
dependency), it unblocks the most immediate embedding scenarios (scripting
slot in a Go host, LSP / formatter tooling, test harnesses), and it does
not foreclose WASM (`DRAFT#3`) - a plugin surface can layer on the same
`pkg/` facade once Wazero (or similar) is in play.

**Concretely.** Add a `pkg/` top-level (working name; the final path
settles at the start of this work):

- `pkg/interpreter` re-exports `Interpreter`, `Value`, error types, and the
  `Install(in *Interpreter)` registration API that every stdlib library
  already uses. The `internal/` packages stay as the implementation; `pkg/`
  is the stable facade with semver-covered surface once we ship 1.0.
- `pkg/lib/*` re-exports each shipped library (`convert`, `math`,
  `strings`, ...) so a host can install the ones it wants and leave out the
  rest. Non-breaking for the current CLI - `cmd/jennifer` picks up the same
  `Install` calls, just through `pkg/lib` shims instead of directly.
- Documented pluggable interfaces for the host-provided facilities the
  OS-touching libraries currently reach for:
  - `io.Writer` for `io.printf` output (already a `*Interpreter` field;
    formalize as an interface).
  - `io.Reader` for `io.readLine` / `io.readBytes` / `io.readChars` stdin.
  - `Clock` for `time.now()` / `time.local()` / `time.sleep` (the `nowFunc`
    / `sleepFunc` test hooks in `internal/lib/time` are the shape).
  - `Rand` for `math.rand*` / `lists.shuffle` (the shared random source).
  - Filesystem / network / process hooks left as future work - a host
    wanting those either installs the stdlib libraries as-is (accepting the
    Go `os` / `net` dependencies) or ships its own shims. A documented
    registration pattern is the deliverable; the shims themselves are
    per-host and out of scope here.

**Stdlib-backed defaults.** Each pluggable interface carries a working
default so `pkg/interpreter.New()` plus `pkg/lib/io.Install(in)` produces a
running interpreter without every embedder wiring up seven interfaces
first. `Clock` defaults to Go's `time.Now`, `Rand` to a `math/rand`
source, `io.Writer` to `os.Stdout`, `io.Reader` to `os.Stdin`. Hosts
override only what they need. A `no-os` embedder replaces every default; a
Slack-bot embedder swaps just `io.Writer` for its outgoing-message pipe and
leaves the rest.

**Boundary rules at the Install site.** Three explicit error paths so hosts
get loud, positioned failures instead of subtle misbehaviour:

- **Duplicate library `Install` at the Go level is rejected**, mirroring
  how a duplicate `use NAME;` errors at the Jennifer level (the
  duplicate-`use` rule, lifted). A host installing `pkg/lib/math` and then
  its own shim that also claims the `math` namespace fails at the second
  `Install` call, not silently overlaid.
- **`Install` and pluggable-interface setters are frozen once `Run()`
  starts.** Attempts to call `Install`, `SetClock`, `SetOut`, or friends
  after the interpreter has begun executing produce a positioned "cannot
  configure interpreter mid-run" error at the Go call site. The interpreter
  can then trust its host bindings for the rest of the run without
  defensive re-checks.
- **Host implementations are trusted at the interface boundary.** The
  interpreter uses whatever `Clock.Now()` or `Rand.Int63()` returns without
  validation - a broken host implementation is the host's problem, not the
  interpreter's. Stated so hosts don't expect defensive checks that aren't
  there and so downstream bug reports are triaged to the correct side of
  the API boundary.

**Non-goals.**

- A hosted no-`os` build target. Even with this restructure, the shipping
  stdlib libraries lean on Go's `os` / `net` / `time` packages. A truly
  bare-metal or `no-os` embedding can only use the pure-value libraries
  (`convert`, `math`, `strings`, `lists`, `maps`, `hash`, `crc`,
  `encoding`, `regex`) plus whatever host-provided shims the embedder wires
  up. That's a design constraint on the embedder, not a milestone on
  Jennifer's side.
- Semver freezing the public API. Jennifer stays pre-1.0 through this work;
  it documents what's exported and how libraries plug in, but breaking
  changes to that surface remain allowed until 1.0.0.

**Motivation.** Third-party embedding has multiple concrete consumers
already imagined: scripting-language slot in a Go application, tooling that
needs direct AST / interpreter access (LSP, formatter integrations, syntax
highlighters), test harnesses that want to drive `.j` programs from Go,
config-DSL runtimes, plugin systems for game engines and similar. None of
them require an OS-free build; all of them need the `internal/` -> `pkg/`
restructure. The `Install` pattern already works this way - every stdlib
library is a `pkg.Install(in)` call. The missing piece is visibility, plus
documented hooks for the pieces of host state currently exposed only as
package-level test vars.

**Requires:** none - a self-contained restructure of the current codebase.
Best sequenced once the core library / module surface has settled, so the
`pkg/` facade is stable, but nothing blocks it.

#### DRAFT#2 - WASM runtime embedding

Wazero or similar inside the interpreter binary. TinyGo-size cost evaluated
honestly before commitment. Without it, no WASM libraries (`DRAFT#3`).

**Requires:** none (embeds Wazero directly).

#### DRAFT#3 - WASM libraries

If the WASM runtime (`DRAFT#2`) ships, sandboxed plugins via
`use wasm:libname;`. Each library its own piece.

**Requires:** `DRAFT#2` (the runtime) and `DRAFT#1` (the plugin surface
layers on its `pkg/` facade).

#### DRAFT#11 - Sandbox

Restricted-capability execution.

**Requires:** none hard; relates to `DRAFT#1` (embedding) and `DRAFT#3` (WASM
isolation).

### Interpreter internals

#### DRAFT#17 - Bytecode execution model

The tree-walker re-walks the AST and re-resolves shapes on every pass through a
hot loop; that per-operation dispatch + Value-copy overhead is the throughput
ceiling for CPU-bound `.j`. Compile the resolved AST to a linear **bytecode** (or
a register form) executed by a tight dispatch loop - a new pipeline stage between
resolve and run - so a loop body is decoded once and then dispatched without
re-walking the tree. This is the big structural lever, and the big effort.

- **Same semantics, same discipline.** Value semantics and the tagged-union
  `Value` stay exactly as they are; only how operations are sequenced and
  dispatched changes. Held to the same TinyGo-clean, reflect-free rule as the
  current evaluator, and to strict behaviour parity: the `spawn` snapshot,
  `defer` order, positioned errors, and the call-depth guard must all survive the
  rewrite, with the existing test suite as the conformance oracle.
- **Sequenced after the cheap win.** what remains for a VM
  is the residual CPU-bound `.j` (recursion, business-logic loops) no Go
  primitive covers. Pursued only when benchmark shows that residual is
  a real workload's bottleneck - not on spec.
- **Composes with arena.** The per-frame arena allocator is
  an independent memory-side optimization that pairs naturally here, since a
  bytecode VM restructures allocation anyway - but neither depends on the other.

Copy-on-write for compound Values is **not** part of this: it was tried
(shared-marker COW, reverted as inert) and the write-through variant is rejected
for reintroducing shared mutable state - see
[technical/rejected.md](technical/rejected.md).

### Project and governance

#### DRAFT#14 - Project governance, licensing, and contribution policy

The rules for *how the project is run and how outside contributions are taken* -
organizational, not code. Untouched while the project is solo (one author,
`Copyright (C) 2026 mplx <jennifer@mplx.dev>`, `LGPL-3.0-only`, no outside PRs), but
it must be settled **before the first external contribution is merged**: several
of the choices are hard to reverse once other people's copyrightable work is in
the tree. The open questions, roughly by urgency:

- **Copyright-holder model.** Under distributed copyright (the default, no
  paperwork) every non-trivial contributor automatically holds copyright in their
  patch, so the tree becomes a mosaic of holders and any future relicensing needs
  each one's agreement. The alternatives are a **CLA** (contributor grants the
  project a broad license, keeps their own copyright) or an **assignment / CAA**
  (contributor transfers copyright to a single holder) - both consolidate the
  rights but add contributor friction, and assignment needs an entity to hold
  them. This is the decision that is expensive to undo.
- **The copyright *notice*.** Whether headers stay per-author (`(C) <name>`) or
  move to a collective label (`(C) The Jennifer Authors`, defined by git
  history). The trap to avoid: a two-file `AUTHORS` (holders) / `CONTRIBUTORS`
  (credit) split only carries information when a **work-for-hire** contributor
  exists (employer holds copyright, individual is merely credited); for an
  all-volunteer project the two lists are identical, so the split is pointless.
  Either keep no enumerated holder file (the collective label refers to git
  history) or consolidate ownership via CLA / assignment.
- **Relicensing headroom.** LGPL already lets anyone embed / link Jennifer
  without permission, so ordinary use never needs a contributor's sign-off. The
  only thing distributed copyright forecloses is issuing a *different* license -
  e.g. a commercial embedding exception for a deep-embedded `jennifer-tiny`
  target that cannot meet LGPL's static-relink terms. If keeping that option open
  matters (embedding is a first-class goal), a CLA is the tool; if "LGPL-only
  forever" is acceptable, distributed copyright is fine and the constraint never
  bites.
- **Contribution mechanics.** `CONTRIBUTING.md`, the sign-off mechanism (a
  lightweight **DCO** `Signed-off-by` line, which asserts "I have the right to
  submit this" without a license grant, vs a full CLA-bot, which also grants
  one - the choice follows from the relicensing decision above), a code of
  conduct, and the PR / review workflow.
- **Project governance.** Who decides (BDFL vs a maintainer group), how commit
  rights are granted (judgment and sustained involvement, never an LOC or
  commit-count threshold - metrics are a bad proxy and get gamed), and a
  `MAINTAINERS` file once more than one decision-maker exists. Being listed as a
  contributor confers no authority; credit and governance are separate.
- **Name / mark.** Whether the "Jennifer" / `jennifer-lang` identity needs any
  trademark-style usage policy (forks, the deck registry) or stays informal.

The license itself stays **`LGPL-3.0-only`** unless a deliberate relicensing
decision above changes it; this draft is about the *process and ownership* around
it, not a license change.

**Requires:** none (organizational, independent of the codebase). Socially
paired with the M19.8 org move and triggered by the first external contribution,
but no code prerequisite. Not legal advice - the chosen model should get a real
legal review before it is published.

## Loose ideas

A grab-bag, loosely grouped and recorded when it comes up.

### Language sugar

- **Explicit map-to-struct conversion.** A spelled-out, validating way to
  turn a `json.Value` object (or a homogeneous `map of string to T`) into a
  typed struct - the sanctioned counterpart to the *rejected* implicit
  coercion (see [technical/rejected.md](technical/rejected.md)). Deferred:
  once JSON is destructured through `json.Value` accessors, the by-hand
  rebuild covers the need, so a one-call form is a convenience, not a
  blocker. Two candidate shapes, decided on consistency not brevity - a
  `convert.toStruct($map, "Point")` library call (a two-arg, stringly-typed
  outlier in the otherwise one-arg `convert.toX` family, or else not
  self-contained if it reads the binding's declared type) versus a
  `Point{ ..$map }` struct-literal spread (names its type statically, at the
  cost of new literal syntax). Either way strict: every declared field
  present with a matching type, recursing into nested structs / lists /
  maps, value-semantic, no partial fills or defaults.
- **Decimal / bignum / money math.** A Go-backed arbitrary-precision base-10
  `decimal` library (over `math/big`) with `from` / `add` / `sub` / `mul` /
  `div` / `round` / `compare` and an opaque `Decimal` value (the `KindObject`
  shape `json.Value` uses) - exact money arithmetic with no float rounding, kept
  a **library handle** rather than a new primitive so the core `int` / `float`
  model is untouched.

### Library completions

- **Locale-aware string collation.** `strings.fold(s)` ships as the lightweight
  answer for accent-insensitive sort / search keys: strip common Latin
  diacritics (`Österreich` -> `Osterreich`, `ß` -> `ss`), pair it with
  `lists.sortBy` for a "good enough" locale-ish order. What it deliberately is
  *not* is full Unicode collation - the Unicode Collation Algorithm (UCA) with
  per-locale CLDR tailorings, where German phonebook order puts `ö` at `oe`,
  Swedish sorts `ö` *after* `z`, and primary / secondary / tertiary weights
  separate base letter from case and accent. That is a data-heavy `collate`
  library (the scope of ICU / `golang.org/x/text/collate`), a real project best
  parked here until a concrete need justifies the CLDR data or the dependency;
  `strings.fold` covers the common Western-European case in the meantime.
- **`encoding` - the harder codecs.** The single-byte character codecs and
  binary-to-text formats all shipped; the deferred remainder, picked up only
  when a real program needs one: variable-width Asian encodings
  (`Shift-JIS`, `Big5`, `GB2312`, `GBK`, `GB18030`, `EUC-JP`, `EUC-KR`) -
  each a state machine with variant / ambiguity edge cases, a whole piece
  apiece; `UTF-16` / `UTF-16LE` / `UTF-16BE` / `UTF-32` (BOM, surrogate
  pairs, endianness); and `UTF-7` (mail-transport - though
  `quoted-printable` already shipped as a general codec).
- **Password hashing (Argon2id / bcrypt / scrypt).** The modern default for
  password *storage*, deferred out of the `crypto` library because it lives
  in `golang.org/x/crypto` (a dependency, unlike the stdlib KDFs `crypto`
  ships) and wants its own surface distinct from the KDFs: a self-describing
  `crypto.hashPassword(pw) -> string` (`$argon2id$...`) plus a constant-time
  `crypto.verifyPassword(pw, hash) -> bool`. Added when password storage is
  a concrete need, taking the `x/crypto` dependency then - crypto is the one
  place the dependency-free stance bends, since you never hand-roll it.
- **`time`: IANA / DST zones.** Real zone names (`Europe/Berlin`) with
  historically-correct daylight-saving resolution, added to the `time`
  **system library** - not a hand-maintained `.j` data map. A `.j` map is
  the wrong shape: abbreviations (`CST` is US Central *and* China Standard
  *and* Cuba Standard) don't identify a zone, and the real model is
  offset-per-(zone, instant) over a transition history that ships several
  updates a year. Back it with Go's `time.LoadLocation` + the embeddable
  `time/tzdata` (or the host's `/usr/share/zoneinfo`), so the database is
  the toolchain's problem and resolution is correct at any instant.
  Standard-`jennifer` only: TinyGo's `time` can't load zones, so
  `jennifer-tiny` stays fixed-offset (a build-tag split like `net`). Level 1
  first - an offset-at-instant resolver (`time.offsetAt(name, $t)` /
  `time.zoneFor(name, $t) -> time.Zone`) that leaves the
  `time.Time {nanos, offset}` model untouched (the snapshot is fixed, so
  DST-crossing arithmetic must re-resolve); Level 2 - a zone-carrying
  `time.Time` with DST-correct arithmetic - is a larger, optional follow-up
  needing a Go-backed zone handle.
- **`label`: embed a bitmap image in the job.** Today `label.image` references
  an image already stored on the printer (by name). The heavier alternative is
  to embed the bitmap in the rendered job so a logo travels with the label and
  needs no pre-loading: convert a source image (PNG / mono bitmap) to each
  dialect's raster - cab embedded-ASCII image data, ZPL `^GF` graphic field -
  which needs image decoding plus 1-bit dithering / thresholding. That is a real
  raster-conversion capability (a Go-side helper or an `image` library), not the
  pure-text `.j` the rest of the module is, so it is a separate piece of work
  rather than another encoder branch. Until then, `label.image` (by reference)
  covers the stored-logo case.
- **SQLite (`sql` engine backend).** SQLite stays parked here, and it is 
  worth being precise about *why*, because it is not the reason it first looks like.
  SQLite is **also** just a pure-Go `database/sql` driver - `modernc.org/sqlite`,
  registered with the same one-line `import _` as `go-sql-driver/mysql` or
  `pgx`, cross-compiling cleanly like any pure-Go package (the cgo
  `mattn/go-sqlite3`, which *does* break static / cross-compile / TinyGo and
  needs a C toolchain, is rejected in its favor). So integration effort and
  API are identical to the shipped drivers; SQLite is in every practical
  sense "just a third driver" for the same library, sharing its surface and
  opaque `sql.Row` result shape.
  The one real difference is **weight**. `modernc.org/sqlite` is the entire
  SQLite C source transpiled to Go plus `modernc.org/libc` (a Go libc
  reimplementation) - multiple MB of generated code, versus the
  hand-written, few-hundred-KB protocol clients already shipped. Baking that into
  every default `jennifer` bloats the binary for the many users who only ever
  touch a network database. That, and only that, is why SQLite is gated as a
  **build-tag opt-in** (`-tags sqlite`), surfaced as a `jennifer-full`
  release artifact - a build *variant* of the default binary, not a third
  supported brand. The binary ladder becomes `jennifer-tiny` (DBs stubbed) ⊂
  `jennifer` (MySQL + Postgres) ⊂ `jennifer-full` (+ SQLite). The dependency
  break from "libraries stay dependency-free" is already accepted;
  SQLite adds size, not a new principle.
  **TinyGo** is the one place SQLite is categorically worse, and it is
  architectural, not a build choice: `modernc.org/sqlite`'s libc emulation
  (unsafe, goroutines, syscall-level memory management) cannot compile under
  TinyGo, and no TinyGo-compatible SQLite exists. Unlike a wire-protocol
  database - which could in principle be reimplemented as pure `.j` over a
  net-enabled tiny rebuild - SQLite has no wire protocol and so can *never*
  reach the embeddable binary. That is the genuinely ironic gap: a local,
  file-based store is exactly what a minimal embedded target would most want,
  and it is the one database that binary can't have with current tooling.
- **FCGI.** `use FCGI as web;` library when `net` and `httpd` mature. Lets
  Jennifer host CGI / FastCGI workloads end-to-end.
- **i18n (CLDR formatting).** Locale-aware case folding, collation, number /
  date formatting, and BiDi - the CLDR-data-backed half of i18n, distinct from
  the message catalogs + `%name%` interpolation already shipped as the `intl`
  library (`M20.4`). Gated on the CLDR-data binary-size question (likely an
  optional library after the WASM runtime, so locale tables aren't baked into
  every build).

### Runtime and tooling

- **`tinygo_devtools` build tag.** The dev subcommands (`tokens` / `ast` /
  `fmt` / `lint` / `profile` / `test`) are `!tinygo` for binary *size*, not
  compatibility - they are TinyGo-clean Go. A
  `//go:build !tinygo || tinygo_devtools` constraint (stub as
  `tinygo && !tinygo_devtools`) plus a `make build-tinygo-dev` target would
  let them run under the actual TinyGo runtime - e.g. to `profile` a
  TinyGo-specific perf or stack issue in situ. Pairs with the depth metric
  above: together they are "TinyGo runtime introspection." Deferred -
  build-tag complexity across ~6 files and a larger dev-tiny binary, for a
  diagnostic reached for only occasionally.
- **Build-time library selection.** Choose which system (Go) libraries are
  baked into a binary at compile time. Motivated by `jennifer-tiny` size (an
  embedded target needing only `io` + `math` shouldn't carry `net` / `regex`
  / `hash`) and by opt-in niche Go libraries that don't merit defaulting.
  The install point is already consolidated - every entry path (`run` /
  `repl` / `profile` / `test` and the test harnesses) calls
  `internal/stdlib.InstallAll`, so a library is one line there - and that is
  the seam a build-tag scheme would cut along: gate each entry behind
  `//go:build lib_net` (or a `minimal` / `full` profile) and grow
  `make build-minimal` / `make build TAGS=...`, exactly like the existing
  `!tinygo` dev-tool split. **Compile-time only** - Go's `plugin` package is
  Linux/macOS-only and unsupported by TinyGo, and dynamic linking
  contradicts `jennifer-tiny`'s no-hosted-runtime goal, so PHP-style
  loadable `.so` extensions are out. Two caveats to design for: (1) a
  trimmed build breaks the "any `.j` runs on any binary" portability promise
  (`use net;` becomes a runtime error), so the default build stays full and
  trimmed builds are an explicit opt-out - ideally with a `meta`-level "is
  library X present?" query for graceful degradation; (2) CI grows a couple
  of profiles (default / minimal), not 2^N. Complementary to, not a
  substitute for, the module system: `.j`-level extensibility (community /
  uncommon libraries writable in Jennifer) is the module system's job with
  zero binary cost; build-time selection is only for the curated Go-level
  core.
- **Binary AST cache (`.jc` files).** Pre-parsed loading for big programs
  and embedded scripting hosts. Its own effort when it lands - file-format
  design, versioning, and TinyGo-safe serialization are enough work to merit
  dedicated treatment. The text JSON form via `jennifer ast` is the
  placeholder until then.
- **Compile to Go (AOT).** Transpile the resolved AST to Go source and let the
  Go toolchain build a native binary - the pragmatic ahead-of-time path, reusing
  the existing lexer / parser / resolver front-end. It wins where a from-scratch
  native backend (an LLVM one, say) loses: Go's GC, goroutine scheduler, and the
  **entire Go-hosted stdlib** come for free, so the huge re-hosting cost (`net` /
  TLS / `crypto` / `sql` / `regex` / ...) never appears. The mappings are natural
  - value semantics to struct copies, `spawn` to a goroutine over a snapshot,
  `defer` to `defer`, `throw` to error returns. The catch is the language's
  dynamism (untyped method returns, `meta.call` by name, opaque `json.Value`): a
  naive transpile keeps boxed tagged-union values, so it beats the tree-walker
  but is not C-speed without type-specialisation work. Relates to `DRAFT#17` (the
  in-process bytecode VM is the cheaper dispatch win) and `DRAFT#2` (WASM, the
  other compiled target); a hand-rolled LLVM backend is the most expensive, least
  differentiated version of this idea and stays a non-goal. Aside:
  `jennifer-tiny` already reaches native through LLVM transitively - TinyGo is an
  LLVM-based Go compiler.
- **Advanced scheduling knobs.** CPU affinity, work-stealing pool sizing,
  NUMA awareness, `GOMAXPROCS`-equivalent runtime tuning. Runtime-config
  surface for the spawn scheduler, not new language features. Ships when a
  real use case forces it (the default - "let Go's scheduler decide" -
  handles every workload we've imagined so far).
- **Profiler: heap-per-position metric.** Out of scope for now: `--allocs`
  already proxies value-copy churn, and true per-position RSS needs
  `runtime.ReadMemStats` sampling, which is coarse under TinyGo.

### Wild ideas

- **Inline assembler.**
