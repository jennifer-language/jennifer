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
graduated into a numbered milestone (e.g. "shift `DRAFT#9` to `M21.9`").
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
- **Installed into the `vendor/` tree M19.7 resolves.** `jvc` writes decks into
  the project-local `vendor/` tree that the interpreter already addresses through
  the `@scope/package` import form and vendor-root discovery shipped in
  [M19.7](milestones.md) - so a hand-populated `vendor/` imports before `jvc`
  exists, and `jvc` is just the manager layered over that resolver. Nothing is
  global; each app owns its decks beside it.
- **The one remaining language-surface question: inline version selectors.**
  M19.7 resolves `import "@jennifer/supercms/" as cms;` (the trailing `/` expands to
  the package-named entry `supercms/supercms.j`) against whatever is installed.
  `jvc` supplies the *default* - plain `import @jennifer/supercms;` takes
  the version `jvc` resolved (declared in `deck.toml`, pinned in the lockfile),
  version-transparent, which is what almost every script wants. The opt-in for
  side-by-side versions is a **per-import selector** matched against the installed
  set (never triggering a fetch): `@jennifer/supercms=1.2.3` (exact), `>=1.2.3` / `~`
  / `^` (a `semver` constraint over what is installed), or `#cefa234` (a git
  commit); one script can pin `=1.x` while another pins `=2.x`, and two versions
  in one file take distinct `as` aliases. An unsatisfiable selector errors
  pointing at `jvc install`, not a silent download. **Cost:** the selector is
  **new grammar** (the lexer reads `@vendor/deck` + `semver` op + `#commit` as one
  token up to `;`); the plain M19.7 string-path form is the no-new-grammar
  fallback that loses only the inline selector.
- **`deck.toml` manifest + lockfile.** `deck.toml` (TOML, so it needs the `toml`
  library) declares required decks and constraints (`bitcoin = ">=1.2.0"`), and
  `jvc` produces a **`camcorder.lock`** pinning exact resolved versions (content
  hash per deck) so `git clone` + `jvc install` is reproducible. Dependency sets
  split by section (`[prod]` / `[dev]`, `jvc install --prod`), with a **taxative**
  (the section is the exact set) vs **additive** (base plus the section's extras)
  mode still to design.
- **`jvc` owns the lifecycle:** dependency resolution (semver constraint
  solving across the graph), downloading, `jvc update` (advance to the
  newest constraint-satisfying versions, rewrite `camcorder.lock`), integrity
  pinning, and the publish flow to the registry.

**Migrating bundled modules out to decks.** Once decks exist, niche or
product-specific modules that ship bundled today should graduate *out* into
decks (the archetype is `gotify` - a single-product push integration every
install need not carry); language-fundamental modules stay bundled. Moving one
changes its import, so it is a breaking change under semver: within 1.x ship it
**both ways** (bundled + `@`-deck) with the bundled copy marked `@deprecated` so
imports re-point at their own pace, let the two drift without breaking, and
**remove the bundled copy in 2.0.0**.

A whole track of its own. **Requires:** [M19.7](milestones.md) (the
`@scope/package` resolver + vendor root the scheme rests on); the `toml` library
(M18.8, for `deck.toml`); the shipped module system and the `semver` module
(constraint solving - its `satisfies` / `maxSatisfying` / `minSatisfying` /
`validRange` range surface is the resolver primitive); and `http` / git
(fetching). The public deck **registry** is separate infrastructure, provided
later.

### DRAFT#13 - Higher-level PDF: font metrics, layout, and Markdown -> PDF

A three-phase build on top of the shipped `pdfwriter` module (M18.35), taking it
from "place text and shapes at coordinates" to "flow a document". The phases are
strictly ordered because each depends on the one before; they land as separate
milestones when graduated, but share one draft handle because they are one arc.

**Phase 1 - font metrics (the keystone).** Today `pdfwriter` can place text but
cannot **measure** it, so there is no way to wrap a paragraph, auto-size a table
column, or align text - all of which need the rendered width of a string. Add
the **standard-14 AFM width tables** (public Adobe Font Metrics: per-glyph
advance widths, in 1/1000 em, for WinAnsiEncoding) and a
`pdfwriter.stringWidth(font, size, text) -> int` that sums them. Courier is
monospace (600 units/glyph, trivial); Helvetica and Times need the real
per-character tables, generated into an included `.j` data file the way the
`encoding` codecs were generated from the Unicode mapping files
(`gen_*.go` -> `*_gen.j`). First payoff: `textAligned(pg, x, y, font, size,
align, str)` for left / center / right placement. Nothing in the later phases is
expressible without this.

**Phase 2 - table and flow layout.** The layer that removes the manual
cell-positioning pain (the ergonomic win people reach for TCPDF's `writeHTML`
tables to get - but as a typed API, not an HTML subset). On top of `stringWidth`:
`table(columns, rows, options)` (auto column sizing, in-cell word wrap, borders,
a header row, page-break across rows); `paragraph(pg, text, x, y, width, font,
size) -> int` (wrapped flowing text that returns the y it ended at, so callers
stack blocks); and `heading` / list helpers.

**Phase 3 - Markdown -> PDF.** The markup-driven document story, and the reason
it is Markdown rather than HTML: Jennifer ships a `markdown` parser (GFM tables,
headings, lists, emphasis) but no HTML *parser* (`htmlwriter` only builds HTML),
so the cheap, ergonomic path to "write markup, get a PDF" is to drive the Phase-2
layout layer from the existing Markdown parse rather than write a quirky
HTML-subset parser (Markdown tables are also easier to author than HTML tables).
A prerequisite to verify first: whether the `markdown` module exposes a reusable
parse tree or only renders straight to a string - if the latter, it needs a small
refactor to surface the intermediate document model. An HTML-subset front-end
(TCPDF-style) stays a *later* option, only worth it for consuming pre-existing
HTML, and it would sit on the same layout foundation.

Stays pure `.j` (static data + lookups + layout math), both binaries.
**Requires:** none hard for Phase 1 (builds on the shipped `pdfwriter` module,
M18.35); Phase 2 requires Phase 1; Phase 3 requires Phase 2 and the shipped
`markdown` module (plus possibly a parse-tree surface on it).

### DRAFT#14 - Project governance, licensing, and contribution policy

The rules for *how the project is run and how outside contributions are taken* -
organizational, not code. Untouched while the project is solo (one author,
`Copyright (C) 2026 <developer@mplx.eu>`, `LGPL-3.0-only`, no outside PRs), but
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

### Interpreter throughput ceiling - graduated to M21.10

The tree-walker's per-operation overhead (the throughput ceiling that caps
byte-oriented `.j`: `mime` scanning ~8 s/MiB, `http.readToEOF` ~1 min/64 MiB)
graduated to `milestones.md` **M21.10**, scoped into measured sub-milestones - a
throughput benchmark harness, a bulk `bytes` primitive, pushing one hot
byte-crunching path into Go, and (the large lever, allowed to defer past 1.0.0) a
bytecode execution model. The push-to-Go direction composes with `DRAFT#9`
`asn1`; the compiled core relates to `DRAFT#1` embedding.

## Loose ideas

A grab-bag, recorded when it comes up.

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
- **Profiler: max-call-depth metric.** Graduated to `milestones.md` M21.8
  (paired with the interpreter's catchable call-depth limit, since both ride the
  same `evalCall` depth counter).
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
- **SQLite (`sql` engine backend).** The client-server half of relational
  support - MySQL / MariaDB + PostgreSQL - is committed as
  [M20.9](milestones.md) (a `sql` system library over Go's `database/sql`,
  pure-Go drivers). SQLite stays parked here, and it is worth being precise
  about *why*, because it is not the reason it first looks like. SQLite is
  **also** just a pure-Go `database/sql` driver - `modernc.org/sqlite`,
  registered with the same one-line `import _` as `go-sql-driver/mysql` or
  `pgx`, cross-compiling cleanly like any pure-Go package (the cgo
  `mattn/go-sqlite3`, which *does* break static / cross-compile / TinyGo and
  needs a C toolchain, is rejected in its favor). So integration effort and
  API are identical to M20.9's two drivers; SQLite is in every practical
  sense "just a third driver" for the same library, sharing its surface and
  opaque `sql.Row` result shape.
  The one real difference is **weight**. `modernc.org/sqlite` is the entire
  SQLite C source transpiled to Go plus `modernc.org/libc` (a Go libc
  reimplementation) - multiple MB of generated code, versus the
  hand-written, few-hundred-KB protocol clients M20.9 ships. Baking that into
  every default `jennifer` bloats the binary for the many users who only ever
  touch a network database. That, and only that, is why SQLite is gated as a
  **build-tag opt-in** (`-tags sqlite`), surfaced as a `jennifer-full`
  release artifact - a build *variant* of the default binary, not a third
  supported brand. The binary ladder becomes `jennifer-tiny` (DBs stubbed) ⊂
  `jennifer` (MySQL + Postgres) ⊂ `jennifer-full` (+ SQLite). The dependency
  break from "libraries stay dependency-free" is already accepted at M20.9;
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
  Because SQLite is really just another driver, the only open call is timing:
  fold it into M20.9 behind the `-tags sqlite` gate, or keep it deferred here
  until the `jennifer-full` variant earns its place in the release / CI /
  packaging matrix. Contrast the text-protocol stores `redis` / `memcache`,
  pure Jennifer over `net`, which need none of this.
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
