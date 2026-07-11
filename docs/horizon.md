# Beyond 1.0.0 - idea collection

Jennifer's near-term target is a rich, dependable set of language
features, libraries, and modules - enough to tag a **1.0.0**. That work is
tracked in [milestones.md](milestones.md). This file collects ideas for
*after* 1.0.0: directions worth recording so the design doesn't foreclose
them, none committed to a timeline.

Two kinds:

- **[Drafts](#drafts)** - concrete, already-shaped
  directions (embedding, WASM, specialised-domain libraries). Each has a
  design; it just has no schedule.
- **[Loose ideas](#loose-ideas)** - a grab-bag of smaller or vaguer
  possibilities, jotted down when they come up so they are not lost.

Nothing here is a commitment. An idea graduates into
[milestones.md](milestones.md) if and when it earns a slot.

## Drafts

Embedding, WASM, and specialised domains. Recorded so the design doesn't
foreclose them, roughly ordered by the size of the structural change each
is: the embedding restructure is a single-repo refactor; the WASM runtime
brings in a whole new dependency; the specialised-domain libraries are
indefinite-in-count families.

Each carries a stable **`DRAFT#`** handle so it can be referenced and later
graduated into a numbered milestone (e.g. "shift `DRAFT#9` to `M20.6`").
Handles are assigned once and **retired on graduation** - never reused, and
never renumbered when the list is reordered - so a reference stays valid for
the life of the idea. `DRAFT#` is deliberately *not* a milestone number; an
idea only gets an `M`-number when it graduates into
[milestones.md](milestones.md).

Each draft also states its **Requires** - the `DRAFT#` handles and/or
milestones (`M`-numbers) that must land first, or *none* - so its blockers
are explicit before it is scheduled.

### DRAFT#1 - Public interpreter API for third-party embedding

Extract the interpreter core out from under `internal/` and expose a
documented Go-side surface so external programs can embed Jennifer. Today
`internal/interpreter`, `internal/parser`, `internal/lexer`, and
`internal/lib/*` are unreachable from any module that isn't
`github.com/mplx/jennifer-lang` - Go's `internal/` visibility rule is not a
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

### DRAFT#2 - WASM runtime embedding

Wazero or similar inside the interpreter binary. TinyGo-size cost evaluated
honestly before commitment. Without it, no WASM libraries (`DRAFT#3`).

**Requires:** none (embeds Wazero directly).

### DRAFT#3 - WASM libraries

If the WASM runtime (`DRAFT#2`) ships, sandboxed plugins via
`use wasm:libname;`. Each library its own piece.

**Requires:** `DRAFT#2` (the runtime) and `DRAFT#1` (the plugin surface
layers on its `pkg/` facade).

### Specialised domains

Each domain its own effort with sub-pieces as needed:

- **ML.**
  - **`DRAFT#5` `stats`** - descriptive statistics over `list of
    int|float`: mean, median, mode, variance, stddev, percentile, min / max
    / sum, correlation. Pure-value, TinyGo-clean; the highest-value,
    simplest piece, so first. **Requires:** none.
  - **`DRAFT#6` `linalg`** - vectors as `list of float` (dot, norm, cross,
    scale, add / sub) and matrices as `list of list of float` (matmul,
    transpose, determinant, inverse, solve, identity). Algorithms
    implemented directly - no `gonum`, too large a dependency. Matrices stay
    `list of list of float` for v1 (idiomatic and value-semantic); a
    Go-backed matrix handle is the noted future escape hatch when big-matrix
    performance demands it. **Requires:** none.
  - **`DRAFT#7` ML primitives** - atop `stats` / `linalg`, when demand
    surfaces. **Requires:** `DRAFT#5` + `DRAFT#6`.
- **`DRAFT#8` Bioinformatics.** Sequence alignment (Smith-Waterman,
  Needleman-Wunsch), FASTA/FASTQ parsers, molecule structures.
  **Requires:** none.
- **Encoding / binary protocols.**
  - **`DRAFT#9` `asn1`** - ASN.1 BER/DER encode/decode, as a **Go system
    library**.
    Byte-level binary parsing belongs in Go, not `.j` (the `json` lesson: a
    char-by-char parser in the interpreter pays overhead per byte). This is
    the *enabler* for a family of binary protocols and PKI formats - LDAP,
    SNMP, X.509, PKCS. Go's stdlib `encoding/asn1` is DER-only, so the full
    BER that LDAP / SNMP use (indefinite lengths, alternative encodings)
    needs either a BER dependency (e.g. `go-asn1-ber`) or a hand-rolled BER
    codec in Go. **Requires:** none (it is the enabler for the rest of this
    group).
  - **`DRAFT#10` `ldap` / `snmp` (layered on `asn1` + `net`).** With `asn1` doing the
    byte crunching in Go and `net` providing TCP/UDP + TLS (`connectTLS` /
    `startTLS` already cover LDAPS / StartTLS), the protocol orchestration
    (bind, build request, iterate results) is not per-byte hot and can live
    in a `.j` module or a thin Go library. SNMP is the natural first client
    (simpler PDUs, UDP, no SASL); LDAP adds controls + SASL (SCRAM builds on
    the `crypto` library). A pure-`.j` implementation of the BER layer
    itself is explicitly *not* the plan. Existing pure implementations (e.g.
    PHP FreeDSx) are a protocol reference, not a port target - their heavy
    OO shape does not map to Jennifer's value-semantic structs.
    **Requires:** `DRAFT#9` (`asn1`) and the shipped `net` library; LDAP's
    SASL / SCRAM path additionally needs `crypto` (`M20.1`).
- **`DRAFT#11` Sandbox.** Restricted-capability execution.
  **Requires:** none hard; relates to `DRAFT#1` (embedding) and `DRAFT#3`
  (WASM isolation).

Ordered when demand surfaces. The WASM libraries idea (`DRAFT#3`) may cover
some of this space first.

### DRAFT#12 - `jvc` package manager (decks)

A package manager for Jennifer, in the shape of PHP's Composer (or Rust's
Cargo): declare dependencies in a manifest, `jvc install` resolves and
fetches them, and the app imports what it pulled. Installing an app becomes
`git clone` + `jvc install`, and `jvc update` advances within the declared
constraints.

- **Packages are "decks".** A deck is a distributable, versioned bundle of
  `.j` modules, published to a public **deck repository / registry**
  (provided later) that `jvc` resolves and fetches from - packagist-style; a
  deck can also come straight from a git URL.
- **Installed under the app root.** `jvc` writes decks into a project-local
  `vendor/` (working name; `deck/` is the alternative) directory, which
  becomes one more **module search root** - the module resolver already walks
  a search path (system dir, then `-I` dirs), so this is an added entry, not
  a new resolution model. Nothing is global; each app owns its decks, checked
  out beside it.
- **Import syntax - the `@`-sigil (leading proposal).** A deck reference is
  `@vendor/deck`, e.g. `import @mplx/supercms as cms;`. The `/` separates
  vendor from deck (the npm / Composer / Go convention - reads as a
  hierarchy, and unlike `_` or `=` collides with nothing; `_` is also a
  non-starter since Jennifer names are letters-only). The **vendor
  namespaces the registry** for global uniqueness; the **call-site namespace
  is the deck name** (`supercms.`), with `as` to rename on collision - the
  same shape as `use X as Y`.
  - **Version is jvc's job by default.** Plain `import @mplx/supercms;` uses
    the version jvc resolved (declared in `deck.toml`, pinned in
    `camcorder.lock`); 99% of scripts write exactly that, version transparent.
  - **A per-import selector is the opt-in for side-by-side versions.** Since
    `deck.toml` can install several versions at once, a script may choose one
    at the import - **resolved against the installed versions, never
    triggering a fetch** (fetching is jvc's job): `@mplx/supercms=1.2.3`
    (exact), `@mplx/supercms>=1.2.3` (and `~` / `^` / other `semver`
    operators - a constraint over what's installed), or
    `@mplx/supercms#cefa234` (a git commit). This lets one script in a repo
    pin `=1.x` while another pins `=2.x`; two versions in the *same* file
    just take distinct `as` aliases (both otherwise bind `supercms.`). An
    unsatisfiable selector is a positioned error pointing at `jvc install`,
    not a silent download.
  - **Cost:** this is **new grammar** - the lexer must read an `@`-deck
    reference (slash + `semver` operator + `#commit`) as one token up to `;`.
    The lower-magic fallback stays on the table: the existing string-path
    resolver over the `vendor/` search root
    (`import "mplx/supercms/main.j" as cms;`) needs no grammar change but
    loses the inline version selector and the visual distinctiveness.
- **`deck.toml` is the manifest** (TOML - so it needs the `toml` library). It
  declares the app's required decks and version constraints
  (`bitcoin = ">=1.2.0"`), and `jvc` produces a lockfile -
  **`camcorder.lock`** - pinning the exact resolved versions (content hash
  per deck for integrity) so `git clone` + `jvc install` is reproducible.
- **Environments.** `deck.toml` separates dependency sets by section - e.g.
  `[prod]` and `[dev]` - so `jvc install --prod` (working spelling) installs
  one set. Two modes to design: **taxative** (the section is the exact,
  exclusive set for that environment) vs **additive** (a base set plus the
  section's extra decks), the additive mode enabling optional / opt-in
  dependencies loaded only in some environments.
- **`jvc` owns the lifecycle:** dependency resolution (semver constraint
  solving across the graph), downloading, `jvc update` (advance to the
  newest constraint-satisfying versions, rewrite `camcorder.lock`), integrity
  pinning, and the publish flow to the registry.

**Migrating bundled modules out to decks.** Once decks exist, some modules
that ship bundled today should graduate *out* of the interpreter's module
set into decks - the niche or product-specific ones, not the
language-fundamental ones. `gotify` is the archetype: a single-product
push-notification integration, tied to no language constraint and not
broadly needed - a classic deck, not something every install should carry.
Moving it changes how scripts import it, so it is a **breaking change** and
follows semver discipline after 1.0.0:

- **Within 1.x, ship it both ways** - the bundled `gotify.j` *and* a
  `@gotify` deck - so nothing breaks. The bundled copy is marked
  **deprecated** (a `@deprecated` docblock tag), and developers re-point
  imports from `gotify.j` to the deck at their own pace.
- The two copies may **drift** in features across 1.x, but neither ever
  breaks; which one a script uses is the user's choice.
- The bundled `gotify.j` is **removed in 2.0.0** - the major, breaking
  release where such removals are allowed.

The same path applies to any other bundled module that turns out to be
deck-shaped rather than standard-library-shaped.

A whole track of its own. **Requires:** the `toml` library (`M18.8`, for
`deck.toml`); builds on the shipped module system (the search-root
mechanism), the `semver` module (constraint solving), and `http` / git
(fetching decks). The public deck **registry** is separate infrastructure,
provided later; the import convention above is the one language-surface
question to settle.

## Loose ideas

A grab-bag, recorded when it comes up.

- **Extra distribution packaging.** Beyond the Linux `.deb` (shipped in
  M15.8) and the two distribution *requirements* for 1.0.0 stable
  (cross-build for macOS / Windows, a real apt repository), the additional
  package formats are nice-to-haves, each shipped only when a user asks and a
  maintainer will keep it green: a **Homebrew tap** (macOS), a **Snap**
  package, a **Nix flake** / Nix package, and **Flatpak** / **AppImage** or
  any other Linux distribution format. None blocks a release; none is a 1.0.0
  requirement.
- **Cross-platform support.** Linux is the only *supported* platform, but
  best-effort **unsupported** macOS and Windows binaries (the standard-Go
  `jennifer`, via cross-compile) already ship with each release - so the work
  here is not "add the ports" but *promoting them to supported* (real CI
  coverage, per-OS golden strategy, the platform-specific edge cases). When
  touching filesystem, paths, line endings, or process behavior, prefer
  portable stdlib helpers (`path/filepath`, not hardcoded `/`); avoid
  Linux-only assumptions so those binaries stay genuinely portable, not just
  compile-clean.
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
- **Password hashing (Argon2id / bcrypt / scrypt).** The modern default for
  password *storage*, deferred out of the `crypto` library because it lives
  in `golang.org/x/crypto` (a dependency, unlike the stdlib KDFs `crypto`
  ships) and wants its own surface distinct from the KDFs: a self-describing
  `crypto.hashPassword(pw) -> string` (`$argon2id$...`) plus a constant-time
  `crypto.verifyPassword(pw, hash) -> bool`. Added when password storage is
  a concrete need, taking the `x/crypto` dependency then - crypto is the one
  place the dependency-free stance bends, since you never hand-roll it.
- **`encoding` - the harder codecs.** The single-byte character codecs and
  binary-to-text formats all shipped; the deferred remainder, picked up only
  when a real program needs one: variable-width Asian encodings
  (`Shift-JIS`, `Big5`, `GB2312`, `GBK`, `GB18030`, `EUC-JP`, `EUC-KR`) -
  each a state machine with variant / ambiguity edge cases, a whole piece
  apiece; `UTF-16` / `UTF-16LE` / `UTF-16BE` / `UTF-32` (BOM, surrogate
  pairs, endianness); and `UTF-7` (mail-transport - though
  `quoted-printable` already shipped as a general codec).
- **FCGI.** `use FCGI as web;` library when `net` and `httpd` mature. Lets
  Jennifer host CGI / FastCGI workloads end-to-end.
- **Inline assembler.**
- **Binary AST cache (`.jc` files).** Pre-parsed loading for big programs
  and embedded scripting hosts. Its own effort when it lands - file-format
  design, versioning, and TinyGo-safe serialization are enough work to merit
  dedicated treatment. The text JSON form via `jennifer ast` is the
  placeholder until then.
- **Profiler: max-call-depth metric.** Have `jennifer profile` track
  Jennifer call depth (bump in `evalCall`, drop on return) and report the
  max reached, per source position and overall. Names stack-limit problems
  directly - the recursion-depth-vs-`-stack-size` headroom that the
  recursive `fib` in `examples/benchmark.j` exercises on `jennifer-tiny`.
  Small and additive to the existing hit-count / wall-clock / `--allocs`
  collector; deferred because stack limits are diagnosable by hand today.
  Heap-per-position stays out of scope (`--allocs` already proxies copy
  churn; true RSS needs `runtime.ReadMemStats` sampling, coarse under
  TinyGo).
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
- **Relational databases (`sql`).** Postponed pending a design discussion. A
  system library - a storage engine and SQL planner can't be interpreted,
  and wire-protocol auth needs crypto - driver-agnostic over Go's
  `database/sql`, SQLite first, then MySQL / PostgreSQL; standard-Go only,
  stubbed on `jennifer-tiny`. The questions that gate it: the SQLite driver
  (cgo `mattn/go-sqlite3`, which breaks static / cross-compile / TinyGo, vs
  pure-Go `modernc.org/sqlite`, multi-MB); whether to accept the first
  heavyweight dependency in the library layer (a break from "libraries stay
  dependency-free") and, if so, gate it as a build-tag opt-in (per the note
  above); and the result-row shape - an opaque `sql.Row` accessor type
  mirroring `json.Value` vs a typed struct via the deferred map-to-struct
  conversion. Values bind only through `?` placeholders (injection safety).
  Contrast the text-protocol stores `redis` / `memcache`, which are pure
  Jennifer over `net` and need none of this.
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
- **`io.lines() -> list of string`.** Slurp the whole stdin into a list.
  Additive on top of the streaming `readLine()` + `eof()` idiom; nice-to-have
  for tiny scripts, not blocking.
- **i18n.** Locale-aware case folding, collation, number / date formatting,
  BiDi. Gated on the CLDR-data binary-size question (likely an optional
  library shipped after the WASM runtime, so locale tables aren't baked into
  every build).
- **Advanced scheduling knobs.** CPU affinity, work-stealing pool sizing,
  NUMA awareness, `GOMAXPROCS`-equivalent runtime tuning. Runtime-config
  surface for the spawn scheduler, not new language features. Ships when a
  real use case forces it (the default - "let Go's scheduler decide" -
  handles every workload we've imagined so far).
- **Performance & memory.** Interpreter-internal optimizations that preserve
  stance #5 (value semantics) at the user level: copy-on-write for lists /
  maps / bytes / structs (share underlying storage until a write splits it),
  per-frame arena allocation, and read-only slice views (`xs[1..5]` as a
  non-owning window that errors on assignment). Strictly optimizations - no
  user-visible aliasing or mutation rules change. Stance-breaking variants
  (mutable references, interior mutability, shared mutable state) are turned
  down in
  [technical/rejected.md](technical/rejected.md#references-interior-mutability-shared-mutable-state).
  Best landed once the language is settled and the interpreter doesn't churn
  under it.
