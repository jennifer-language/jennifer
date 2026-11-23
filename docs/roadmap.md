# Jennifer - Roadmap

Where Jennifer is going: the planned releases up to 1.0.0, the governance and
licensing work, and what 1.0.0 requires. What has shipped is in
[milestones](milestones.md), ideas beyond 1.0.0 in the
[horizon](horizon.md), and deck plans in [decks](decks.md).

Each milestone from M28 on is one release. Its entries are numbered `Mx.y`;
numbers are stable IDs, never reused, and an entry moved to another release
gets a new number and keeps `(was Mx.y)`. Each entry starts with **Planned**
or **Done**; an entry done during a cycle stays here, marked **Done**, until its
release is tagged and the section moves to [milestones](milestones.md).

| Milestone | Release | What |
|---|---|---|
| - | 0.27.0 | no milestone: what was done since 0.26.0, with the M29 entries marked "ships in 0.27.0" ([CHANGELOG](../CHANGELOG.md)) |
| [M29](#m29---0280) | 0.28.0 | hardening, platform reach, small libraries, diagnostics, the value model, the conventions survey, removal of the deprecated bundled modules and names |
| [M30](#m30---0290) | 0.29.0 | copy elision, streaming and binary formats, function signatures, grapheme clusters |
| [M31](#m31---multiplatform-promote-macos--windows-to-supported) | - | macOS and Windows promoted to supported platforms |
| [M32](#m32---project-governance-licensing-and-contribution-policy) | before the first outside contribution | governance, licensing, CLA vs. DCO, contribution policy |
| [1.0.0](#requirements-for-100-stable) | 1.0.0 | the requirements for a stable release |

---

## M29 - 0.28.0

Hardening, platform reach, small libraries, diagnostics and the value model,
and the removal of the deprecated bundled modules and names promised for
0.28.0 (M29.33, M29.34). Entries marked "ships in 0.27.0" were done early and
are released with 0.27.0, a release without a milestone; they leave this file
when it is tagged. Entries keep their former M28 number as `(was M28.n)`.

#### M29.1 - unify resource-exhaustion errors under `kind: "limit"` (was M28.7)

**Done, ships in 0.27.0.** Mostly done in 0.26.0: the decoder caps (M28.54, M28.68), the
explicit size caps (`binary`, `asn1.encode`, `crypto.randBytes`, `regex`,
`strings.repeat`), the open-stream caps of `hash`, `crc` and `compress`, and the
per-call caps of `net`, `io`, `fs`, `lists`, `linalg`, `ml` and `os.spawn` now
throw `"limit"` (M28.78). Still `"runtime"`: materialising a range into a list
(`0..n`), `channel.make` capacity, the `hkdf` / `pbkdf2` size and work caps,
`devio` reads, the `io` printf field cap, and `intl.tr`; and the handle-count
caps below that 0.26.0 did not reach (`net` connections / listeners / UDP
sockets, `httpd` servers, `sql` handles, `term.makeRaw`). A caught
cap breach should tell a caller "your input was too big / too
deep" apart from "something inside me is broken" - so a library walking a
user-supplied tree can re-raise it in its own vocabulary and a deck can keep its
`kind` contract without first imposing its own limit. The `"limit"` kind and the
boundary mechanism already exist (the call-depth cap and `archive.unpack` /
`unpackWith` raise it); this sweep moves the remaining size / depth / count caps -
which today raise a generic `kind: "runtime"` - onto it, each with a regression
test asserting the kind.

Scope, highest-value first:

- **Tier A - input-driven decode / expansion bombs** (the clearest win, untrusted
  input): `json` / `toml` / `yaml` / `xml` nesting caps, the `yaml` alias-bomb node
  budget, `asn1` depth + node-count, `compress` decompressed-size, `intl`
  translation amplification, and the `net.readAll` / `readBytes` / `readN` /
  `recvFrom` byte caps.
- **Tier B - explicit size / allocation caps** (caller asked for too much):
  `binary.make`, `channel.make`, `crypto.randBytes` / `hkdf` / `pbkdf2` (size +
  work), `devio` read, `fs.readBytes`, the `io` printf field cap, `linalg` matrix /
  vector element cap.
- **Tier C - handle-leak guards** (`fs` / `net` / `httpd` / `compress` "too many
  open ...", the `httpd` buffer budget): planned as out of scope (a lifecycle
  leak, not oversized data). **Reversed in 0.26.0:** the `fs`, `os.spawn`,
  `hash`, `crc` and `compress` handle caps already throw `"limit"`, so the rest
  follow for one rule - `net` connections, listeners and UDP sockets, `httpd`
  servers, `sql` handles, `term.makeRaw` - each with a test at the cap.

- **Mechanism.** A shared helper so each site is one call, not per-builtin
  boilerplate: a library returns a sentinel/typed cap error (the `archive` `capError`
  pattern) and a common boundary translator raises `interpreter.RaiseError("limit",
  ...)` with the call-site position - or a direct `raiseLimit(ctx, msg)` where the
  builtin has `ctx`. Interpreter-core caps set `runtimeError.Kind = "limit"` directly
  (as the call-depth sites do). Pure error-kind reclassification: no message or
  control-flow change, each paired with a `kind == "limit"` test.

**Requires:** none hard (the `"limit"` kind + `RaiseError` boundary shipped with the
archive / call-depth work). Breaking only for code that catches these and branches on
`kind == "runtime"` (pre-1.0, documented per site).

#### M29.2 - bounded request parsing in Go (was M28.17)

**Done, ships in 0.27.0.** The `.j` request parsers turn small input pieces into single values
(up to 272 bytes each): `multipart.extractParam` scans a part header as a list of
characters, `multipart.parse` has no cap on part count, header size or name
length, `web.parseForm` splits the body on `&` with no pair limit, and
`web.parseCookie` splits the Cookie header on `;`. Measured peaks: a 1 MiB
multipart part header 1.6 GB, three concurrent 3 MB `&&&...` POSTs 5.9 GB. The
form path runs before any route or login check once `web.csrfCheck` is
middleware. Move form, multipart and cookie parsing into Go (`url.ParseQuery`;
`mime/multipart` with header-size, part-count and name-length limits), exposed to
`web` / `multipart` as library calls returning ordinary maps / lists, so parse
memory stays a small multiple of the input. Until then the bound is the 10 MiB
body cap and the 64 KiB header cap.

#### M29.3 - per-task memory budget (was M28.18)

**Planned.** Nothing sets a Go memory limit, and the catchable `"limit"` guard
(`MaxCopyChainBytes`) only counts copies along a recursive call chain, so one
request that builds a huge value runs the process out of memory: a fatal Go
error or an OOM kill that ends every request in flight. Two parts: at startup
set `debug.SetMemoryLimit` from the cgroup / host limit (reusing
`httpdlib_memlimit.go`'s detection) so the GC works harder before the kill; and a
per-goroutine allocation budget for spawn roots, charged on list / map / bytes /
string growth like the copy-chain counter, so a runaway request raises a
catchable `"limit"` error in its own worker instead of taking the server down.
Touches the interpreter's allocation paths, so it needs its own benchmarks.

#### M29.4 - install paths set at build time (was M28.62)

**Planned.** The interpreter assumes an FHS layout: the system module
directory defaults to `/usr/share/jennifer/modules` (`internal/module/
sysmoddir.go`, `compileDefaultSysmoddir`), the docs, man page and syntax
files are packaged under `/usr/share`, and the shebang the docs recommend is
`#!/usr/bin/env -S jennifer run`. A platform whose prefix is elsewhere
(a self-contained userland under an app's data directory, `/opt`,
`$HOME/.local`, a Nix store path) gets a binary that finds no modules
unless `JENNIFER_SYSMODDIR` is set on every run.

- **One build-time prefix.** `make PREFIX=<dir>` (default `/usr`) and the
  release scripts write the default sysmoddir into the generated file
  `scripts/gen-version.sh` already emits (`init()`, not `-ldflags -X`,
  because TinyGo ignores `-X`), as `<PREFIX>/share/jennifer/modules`. The
  comment at `compileDefaultSysmoddir` already promises this override; the
  build does not do it yet. `jennifer version -v` prints the prefix and
  where each path came from (flag, environment, build).
- **Every other built-in path follows it:** the docs directory
  (`JENNIFER.md`, which decks sync from), and any future data path. No Go
  code names `/usr`, `/etc` or `/tmp` directly (an audit; today only the
  sysmoddir does). Temporary files go through `os.tempDir`, which honours
  `$TMPDIR` (Go's `os.TempDir`), and the docs say so.
- **Shebangs:** the docs mention that `/usr/bin/env` is not universal and
  that an absolute interpreter path (`#!<PREFIX>/bin/jennifer run`) works
  wherever `env` does not; `jvc app install` writes launchers with the
  resolved interpreter path rather than `env` when `/usr/bin/env` is
  absent.
- Tests: a build with `PREFIX=/opt/jennifer` resolves its modules there and
  reports it; the default build is byte-identical to today's. The
  packaging (Debian, Arch, Homebrew, OpenWrt, Docker) keeps `/usr` (or its
  own prefix) and passes it explicitly. Additive.

#### M29.5 - DNS without `/etc/resolv.conf` (was M28.63)

**Planned.** Release binaries are built with `CGO_ENABLED=0`, so name
resolution is Go's pure resolver, which reads `/etc/resolv.conf` and
`/etc/hosts`. On a system without them (some minimal containers, and
userlands whose `/etc` lives under another prefix, see M29.4) every
`http`, `net`, `smtp` and deck call fails at the lookup with no hint why.

- **Resolution order:** `JENNIFER_RESOLV_CONF` (a file path) if set, then
  `<PREFIX>/etc/resolv.conf` from M29.4 when it exists, then
  `/etc/resolv.conf`; the same for `hosts`. The chosen files are reported
  by `jennifer version -v`.
- **A clear error when none exists:** kind `"net"` with reason `"dns"`
  (M28.65) and a message naming the files looked for and the variable,
  instead of Go's "no such host" on a loopback default.
- **No silent public fallback.** Go's default of `127.0.0.1:53` /
  `[::1]:53` when no config exists stays; the interpreter never invents a
  public resolver (privacy, and corporate networks).
- `net.resolve(host) -> list of string` and `net.resolveWith(host,
  servers)` for programs that need a lookup against named servers (also
  useful for diagnostics).
- Implemented with a custom `net.Resolver` (`PreferGo`, `Dial` pointing at
  the configured servers) parsed from the chosen file, since Go's own
  config path is not settable. Tested against a loopback DNS fake in `.j`
  (UDP) and a container without `/etc/resolv.conf`. Default binary only
  for the custom resolver; the tiny build keeps the system behaviour.

#### M29.6 - more release architectures (was M28.64)

**Planned.** Releases ship `linux/amd64`, `linux/arm64`, `darwin/amd64`,
`darwin/arm64`, `windows/amd64` and `windows/386` (`release.yml`). Small
boards, older phones and tablets, and 32-bit x86 machines are left out,
though Go and TinyGo target them without cgo.

- **Add** `linux/arm` (GOARM 7, the common 32-bit ARM; GOARM 6 if the
  Raspberry Pi Zero class is wanted), `linux/386`, `linux/riscv64`, and the
  `android/arm64` and `android/amd64` targets (Go's Android targets link
  against Bionic conventions; which build without cgo is checked first,
  `android/arm` and `android/386` need cgo and an NDK and stay out unless
  that changes).
- Each gets both binaries where TinyGo supports the target, the release
  tarball layout, checksums, and a smoke test under QEMU in CI (as `arm64`
  already has): `jennifer version`, the module test subset that needs no
  network, and a DNS lookup against a loopback fake (M29.5).
- 32-bit targets: the interpreter's `int` is 64-bit everywhere (Go
  `int64`), so behaviour is unchanged; the size limits (`MaxInt`-based
  caps, slice lengths) are reviewed for 32-bit `int` in Go's own types.
- The packaging that consumes the tarballs (OpenWrt's `goarch` mapping,
  Debian, Docker's platforms) is extended to the new names.
- Depends on M29.4 for the Android-style prefix and M29.5 for name
  resolution there.

#### M29.7 - `strings` cutset / affix trim (was M28.5)

**Done, ships in 0.27.0.** `trim` / `trimLeft` / `trimRight` strip Unicode whitespace only, so
there is no way to strip a chosen trailing character - writing a fixed-precision
number without trailing zeros, stripping a `\r` from a CRLF line where leading /
trailing spaces are significant, peeling a known prefix. Requested in three deck
reports; each hand-rolled a loop.

- **Add** `strings.trimPrefix(s, prefix)` / `strings.trimSuffix(s, suffix)` (exact
  affixes, the most-used Go pair) and the cutset family `trimSet(s, cutset)` /
  `trimLeftSet` / `trimRightSet` (strip any rune in `cutset`).
- **One way per thing:** the existing `trim*` stay whitespace-only (their canonical
  job); the new names are distinct operations, not an overload - no arity change to
  the current trio (consistent with how `toInt` took a *parameter* rather than
  spawning `parseHex`-style siblings).

**Requires:** none (additive `strings` surface).

#### M29.8 - bulk number reads from `json` (was M28.66)

**Done, ships in 0.27.0.** Reading a JSON array of numbers into a `list of float` costs
one `json.asFloat(v, "/i")` call per element (about 1.4-2.4 us each, found
in `@deckyard/openai` and `@deckyard/gemini`: 3,072-dimension embeddings,
1,000 texts, 3M calls, most of an 8-9 s run). Statistics, metrics and
geodata decks pay the same.

- `json.asFloats(v, ptr) -> list of float` and `json.asInts(v, ptr) ->
  list of int`: the array at `ptr`, every element a number (ints accepted
  by `asFloats`, as `asFloat` does), else a `json` error naming the first
  bad index; `asInts` refuses a non-integral number. One Go call, no
  per-element pointer formatting.
- `json.asStrings(v, ptr) -> list of string` for the same reason (tag
  lists, ids).
- Signatures in `internal/libsig/json.go`, docs, tests (mixed arrays,
  empty arrays, nested pointers, a 1M-element array against the loop).
  Additive; the decks then use it, which is a deck change each.
- Related: M29.11 (a smaller `Value`) decides the memory such a list costs.
- Shipped in the shared document layer (`internal/lib/doctree`), so `toml`
  and `yaml` have the three readers too, as their "same surface, name for
  name" promises. A bad element is a runtime error naming its index, like
  `asFloat` / `asInt` / `asString`.

#### M29.9 - scope-rule and binder diagnostics (was M28.8)

**Planned.** Three small, independent refinements at the edges of the scope rules.

- **A parameter may shadow a global (decide: forbid, or formally allow).** The
  no-shadowing rule rejects a `def` local, a for-each iterator, a `catch` binding,
  and a `match` binder that collides with a visible global, but a function parameter
  is not checked - `def x ...; func f(x as int) { ... }` is accepted and `$x` inside
  `f` silently means the parameter. The behaviour is well-defined (innermost binding
  wins, and the borrow scan relies on that), so this is an inconsistency, not a
  correctness bug. The decision is which way to make it consistent: **forbid** it (as
  the sibling binders are) or **formally allow** it (a parameter may shadow a global,
  innermost-wins, narrowing the written rule to local-over-local). Evidence leans
  toward allowing - several shipped examples name handler parameters (`token` /
  `user` / `params` / `srv` / `driver` / `data`) that coincidentally match a global,
  so a ban is a pre-1.0 break rejecting a natural, unambiguous pattern. Either way
  the fix is a few lines in `resolveMethod` (the function resolver) plus a one-direction spec wording change,
  so the rule and the implementation agree.
- **A read-only `match` binder reports the wrong rule on mutation.** An enum payload
  binder (`when Circle(c) { ... }`) is read-only - it binds a copy - but is
  registered as an ordinary non-const untyped variable, so an attempt to reassign or
  mutate it is caught late with a message that does not describe why: `$c.r = 99`
  reports "cannot mutate contents of constant" and `$c = ...` reports "cannot assign
  enum to `<invalid>` variable" (the `<invalid>` is the binder's absent declared type
  leaking into the text). Registering it as a constant is wrong - a binder is
  referenced with the `$` sigil like a variable, and the constant machinery would
  demand the bare form - so it needs a distinct **read-only variable** flag on the
  binding (a `slotInfo` bit plus an assign-time check), yielding one accurate message
  ("the match binder `c` is read-only; its payload is a copy"). This adds a third
  binding mode (mutable / `const` / read-only-binder) the resolver and interpreter
  thread consistently.
- **A hoisted function can read a global before its `def` has run.** Top-level
  statements execute in source order and functions are hoisted, so a function called
  above the `def` of a global it references reads the pre-sized zero slot (a `null`)
  rather than the eventual value - and for a `def const` the empty slot is
  misclassified, emitting the wrong hint ("`MAX` is a variable; use `$MAX`"). The
  behaviour is order-dependent by design, but a cleaner outcome is an explicit "used
  before its definition" diagnostic naming the later `def`'s line, which needs a
  per-global "initialised yet?" flag checked on read. The largest of the three and
  the smallest payoff (a rare, order-dependent footgun); recorded mainly so the
  misleading constant hint is not forgotten.

**Requires:** none. The first two are self-contained resolver / interpreter changes
(the first also needs a one-line spec decision); the third is additive init-order
tracking.

---

#### M29.10 - compile-time validation of literal `printf` format strings (was M28.10)

**Planned.** `io.printf` / `sprintf` / `eprintf` parse their format string at
runtime, so a malformed verb or modifier in a string literal fails on the line that
runs rather than when the file is checked: an unknown verb (`%x`), a modifier the
verb does not take (`%d|prec=2`), a bad value (`%d|base=3`), a duplicate key, or a
verb count that does not match the argument count all surface only when that line
executes - possibly on a rarely-taken error path.

When the format argument is a string literal the whole spec is statically known, so
the same `io`/`format.go` parser that runs at call time can run at check time. Two
shippable shapes: a `jennifer lint` rule that re-parses each literal format and
reports a bad verb, modifier, or argument count at its source position (additive,
false-positive-free - a non-literal format is simply not checked); or, stronger, a
parse-time error for a statically-known format whose spec cannot parse (the
"undefined results error" stance applied to a format string). The lint form is the
lower-risk first step and turns a whole class of runtime surprise into a pre-commit
finding.

**Requires:** none hard; builds on the existing `io`/`format.go` spec parser and the
`jennifer lint` framework.

#### M29.11 - a smaller `Value` (was M28.45)

**Planned.** Every value is one `interpreter.Value` struct with a field for
each kind: `Kind`, `Bool`, `Int`, `Float`, `Str`, four slices (`List`, `Map`,
`Bytes`, `Fields`), four strings (`Variant`, `StructName`, `StructNS`,
`ModPath`) and eight pointers (types, task, object, function, channel), about
270 bytes on amd64 whatever it holds. A list element is a `Value`, so a list
of short strings costs roughly 650 bytes per element in practice. Found in the
decks: a 1 MiB HL7 or ASTM transmission of empty fields took 1.1 GB, 65,530
one-field ASTM records 656 MB (each record a struct carrying its delimiters),
a 4.8 MB FHIR bundle about 100 MB once decoded; a `list of float` or `list
of int` built by appending peaks at about 1 KB per element (921,600 floats:
0.9-1 GB RSS, measured 2026-11-15), which is what makes `@deckyard/gemini`'s
`embedMany` of 2,000 x 3,072 dimensions take 2.7 GB; each deck now caps what it
parses (`MAX_FIELDS`, `MAX_RECORDS`, `MAX_SEGMENTS`, a byte cap over search
pages) at well under its input limit, to stay inside a few hundred MB.

Move the rarely used parts out of line: struct identity (`StructName`,
`StructNS`, `ModPath`, `Variant`) behind one pointer to a shared, immutable
type descriptor; the collection element types the same way; the handle kinds
(task, channel, object, function) into one pointer field. That leaves the
kind, one scalar word, a string and one slice inline, about a quarter of the
size, with value semantics unchanged. It touches every constructor and
`Kind` switch, so it needs the benchmark suite before and after, and it sets
the representation `pkg/interpreter` will publish (see `horizon.md`).

#### M29.13 - string positions without a rescan on non-ASCII text (was M28.44)

**Planned.** `strings.indexOf(s, sub, from)` and slicing `$s[a..b]` take rune
positions. ASCII text has a fast path (`TestOneByteRunePath`) and a string's
rune count is cached (`strRunes`), but once the text holds one non-ASCII rune,
each call converts rune positions to byte offsets by walking the string from
its start. The usual scanning loop (find the next delimiter from the last
one, slice the piece between) is then quadratic. Found in the decks:
`@deckyard/hl7` decoded escape sequences that way, and a 200 KB field of
`é\H\` took 45 s (40,000 escapes; the same count in ASCII took 1 s);
`@deckyard/astm` took 1.8 s for 10,000. Both now split on the escape
character instead (`strings.split`), which every such loop has to know to
do.

Remember the last rune-to-byte position on the string value (a one-entry
cursor next to `strRunes`, valid while the string is unchanged), so a call at
or after it resumes the walk from there; a forward scan then costs O(n) in
all. Measure with the 40,000-escape repro and a `strings.indexOf` loop over a
mixed-script text; the `strings` and slicing tests cover the semantics.

#### M29.14 - cheaper calls into another module (was M28.28)

**Planned.** A call into another module costs more than a local call: 200,000
calls of a one-line function take 115 ms locally and 169 ms across a module
boundary, and 142 ms against 310 ms with a two-field struct argument. Thin
wrapper modules then cost more than the work they wrap; inlining one made a
FASTQ reader 23% faster. `dispatchModuleMethod` allocates an argument slice per
call, walks each argument for impostor structs (`findImpostorStruct`), retags
struct arguments inward and the result outward (`retagStructs`, which
reallocates a module struct's field array on every crossing even when only its
own identity changes), and binds the parameters from that slice rather than
while evaluating the arguments, as a local call does.

Profile the struct case with pprof first, then: a fast path for a flat module
struct (no nested module struct or declared type naming one) that rewrites the
identity without walking or reallocating more than the struct itself; a
combined single pass for the impostor check and the inward retag; and binding
each argument into the call frame as it is evaluated, as `evalCall` does. Measure with
the 200,000-call repro on both builds; the cross-module tests in
`module_test.go` and `funcvalue_home_test.go` cover the semantics.

Those steps make a crossing cheaper, but a value that *holds* module structs
still costs time proportional to its size on every call: `retagStructs` rebuilds
every struct, list and map on the way to each one, in both directions, whether
the callee reads the value or not. A module function taking its own "handle"
struct (a parsed tree, an index) pays it per call: 200 calls of a one-lookup
`has` on a 6400-node tree take 745 ms against 1 ms for the same code in one file,
and `@deckyard/mitoleaf`'s tree accessors pay about 5.4 ms a call on its
6409-node tree. The structural fix is **one identity on both sides**: resolve a
module's bare struct names to `(ModPath, name)` when the module is loaded, so a
struct built inside the module already carries the identity its importers use,
and type checks on both sides compare that identity. Nothing then needs retagging,
and the per-call walk, the impostor scan's job (a consumer struct can no longer
share the identity) and the outward retag of results all go. It touches struct
construction, `stampModuleStructType` / `resolveDeclaredStructNS`, the
parameter and field type checks, `zeroStructFor` / `zeroEnumFor`, `crossRetag`
and module constants. (A lazy "already retagged" marker does not help: every
crossing flips the identity, so the marker never matches on the next call.)
`jennifer profile --allocs` reports these rebuilds per call site in its
*module-boundary rebuilds* section, and the v4 `module handle call` row of
`examples/benchmark.j` measures them.

A second measured case is a streaming cursor. `@deckyard/sheets`' `RowReader`
holds the sheet's XML bytes, the shared string table and the date styles, and
the calling shape is `while (hasRow($r)) { $r = nextRow($r); }`. Reading 694
rows of a 17,350-cell Excel file (a 2026-11-23 report against 0.3.0) rebuilt
the cursor three times per row at the boundary (`hasRow` in, `nextRow` in and
out), about 10 MiB per site over the read, on top of the 715 MiB `nextRow`
copied inside the deck (`def out as RowReader init $r`). The deck now keeps
the cursor's large parts in `bytes`, whose copy is one block move (M30.2 has
the copy itself); the boundary rebuilds remain, and a cursor whose caller
passes it back on every step pays them until the identities match. Measured on
0.4.0 (a second report, 2026-11-23): five crossings per row in the usual loop
(`hasRow`, `nextRow` in and out, `rowNumber`, `rowValues`), about 15 KB each
for the cursor's maps and the row, 53 MiB over 694 rows: the largest cost left
in a streaming read.

#### M29.15 - `html.parse` without a per-character copy

**Planned.** `html.parse` turns the whole source into a list of characters
before tokenizing, so a document costs several times its size in memory: a
36 MB EPUB's navigation document (mahabharata, found validating
`@deckyard/epub`) needs about 100 MB, which exhausts `jennifer-tiny`'s
memory guard and is wasteful on the default build too. Tokenize over the
string with positions and slices instead (the string-position work of
M29.13 helps), keeping the parse result and its caps unchanged; measure
before and after with `jennifer profile --allocs` on a large page, and run
the epub deck's 73-book comparison as a regression check.

#### M29.16 - telling a wrong return type from a fault inside a callback

**Planned.** A module that calls a caller's `func` value and needs a
particular result type (a guard must return `bool`, a hook a `json.Value`)
cannot tell "the callback returned the wrong type" from "the callback itself
failed" without matching messages: both are kind `"runtime"`, and Jennifer
has no untyped local or `any` to hold the result and inspect it first.
`@deckyard/fsm` works around it by checking the caught error's source
position (inside fsm.j means the assignment failed), which is correct but
indirect. Pick one of:

- **Typed func values checked at the call:** a parameter typed with a
  signature (`guard as func(string, json.Value) -> bool`) makes the
  interpreter check the result when the value is called and throw a
  distinct error kind (e.g. `"type"`) or a `reason` on the error.
- **A structured reason on type errors:** every "value of type X where Y
  was declared" error carries `reason: "type"` (and the two type names), so
  a caller branches on a field instead of a message.

Either lets fsm drop the position check (a deck change after the release
that ships it). Decide which fits the type system before implementing.

#### M29.17 - local halves of network modules on `jennifer-tiny`

**Planned.** A module that imports a network module carries its `net`
capability, so it does not load on `jennifer-tiny` at all, even for the parts
that need no network. Found in the 0.26.0 pre-release validation (M28.78):
`kvstore` imports `memcache` and `redis`, so its in-process and file stores fail
to load on tiny, and `session` and `ratelimit`, which import `kvstore`, fail with
it. `s3.presign` and `webhook.sign` / `verify` are pure computation, but `s3` and
`webhook` import `http`. The docs mark all of these `[full]` since 0.26.0.

- Decide the mechanism:
  - capability pragmas per function instead of per file, checked at call time on
    tiny;
  - or a lazy import that loads the network module at first use;
  - or splitting each module into a local part and a network part (`kvstore` and
    `kvstore_net`, `webhook` signing apart from delivery).

  Splitting needs no interpreter change, but it changes imports for users.
- Whichever is chosen, a network call on tiny stays a clear "needs capability
  net" error at the point of use.
- Tests: on `jennifer-tiny-dev`, `kvstore.inProcessStore`, `session` and
  `ratelimit` over it, `s3.presign` and `webhook.sign` / `verify` load and run.
  The network halves still refuse.
- Docs: the `[full]` markers in JENNIFER.md and the TinyGo column of the module
  indexes follow the result.

#### M29.18 - `doctree.renderPdf` without a document rebuild per bookmark

**Planned.** `renderPdfDoc` (`modules/doctree.j`, the block loop) calls
`$outdoc = pdf.bookmark($outdoc, ...)` once for every bookmarked heading. `pdf.Document` is the `pdf` module's own struct, so every
call rebuilds the whole document at the module boundary, in and out, outline
and embedded image bytes included. The outline grows with every call, so the
cost is quadratic: a 200-slide cantrip deck makes 400 rebuilds (about 55 MiB);
at 2,000 slides the bookmarks take 8 s of a 32 s build. Found validating
cantrip 0.1.0.

- Collect the `OutlineEntry` list and attach it once, the way the same loop
  already hands pages over with `pdf.addPages`, or hand them to `pdf` once at the end
  (a `pdf.bookmarks(doc, list of Bookmark)` taking a list, or set them in the
  final `Document` before the one call that builds it). One crossing per
  document instead of one per page.
- Look for the same pattern elsewhere in `doctree`'s PDF path (`addFont`,
  `addImage`, per-page calls) and batch those too.
- M29.14 makes module calls cheaper in general; this entry removes the
  per-page calls whether or not M29.14 lands first.
- Test: `profile --allocs` on a generated 500-page document shows no
  per-page `pdf.Document` rebuild; the output PDF is byte-identical.

#### M29.19 - `io.printf` `%f`: zero fill and digit grouping

**Done, ships in 0.27.0.** `%f` has `pad`, `align`, `prec` and `sign`, but neither `fill=0` nor
`group` / `sep`, which `%d` has. So C's `%+010.3f` has no Jennifer form: the
nearest, `%f|prec=3|pad=10|sign=always`, pads with spaces, and `fill` is
refused (`option "fill" not valid for verb %f`).

- `fill=0` on `%f`: zero-pad between the sign and the digits, as `%d` does;
  `%f|prec=3|pad=10|fill=0|sign=always` prints `+00003.142`. Like `%d`, it
  requires `align=right`.
- `group` / `sep` on `%f`, grouping the integer part only (`1,234,567.89`).
- `%e`-style `sci=true` with `fill=0` follows C's rule (zero-pad the mantissa).
- Docs: the `%f` modifier table in `docs/libraries/io.md` and JENNIFER.md's
  printf line; libsig unchanged (modifiers are parsed at run time).
- Tests: positive, negative, zero, `-0.0`, NaN and infinity with `fill=0`; a
  width smaller than the number; grouping with `prec`.

#### M29.20 - `doctree` PDF with embedded TrueType fonts

**Planned.** `doctree.PdfOptions` takes only standard-14 font names
(`bodyFont`, `boldFont`, `italicFont`, `monoFont`, `headingFont`). Those use
WinAnsi (cp1252), so any other character prints as `?` without a warning: an
arrow, an emoji, Greek, Cyrillic, CJK. Grimoire's PDF books and cantrip's slide
PDFs inherit this; the academy refuses such characters until it is fixed. The
`pdf` module already embeds TrueType (`loadFont`, `addFont`, `textUnicode`, a
Type0 font subset to the glyphs used); `doctree` does not use that path.

- `PdfOptions` gains embedded faces: a `pdf.LoadedFont` per role (body, bold,
  italic, mono, heading), each optional. A role with a face is drawn with
  `textUnicode` and measured with the face's own advance widths, so line
  breaking, justification and code wrapping stay exact; a role without one
  keeps its standard-14 font.
- A fallback face list per role, for a glyph the first face lacks (e.g.
  DejaVu Sans, then a symbol font); a glyph no face has is drawn as U+FFFD or
  `?` and reported.
- Never silent: a character that cannot be drawn is reported to the caller (a
  list in the render result, or a `warn` callback in `PdfOptions`), with its
  position, so cantrip and grimoire can warn.
- Bookmarks and document metadata (title, author) are written as UTF-16 text
  strings, so non-Latin titles work in the outline too.
- Ships no font: the caller supplies the `.ttf` bytes. Docs name DejaVu as the
  tested choice.
- Tests: a document with Greek, Cyrillic, an arrow and CJK (with a CJK face)
  renders the right glyphs (`pdftotext` round trip in the test where
  available, else the content stream's glyph ids); width measurement matches
  the face; the report lists an unsupported glyph; the standard-14 path
  produces byte-identical output to before.

#### M29.21 - the built-in libraries raise their own kind for bad input data

**Planned.** Every Go library raises `"runtime"` for whatever goes wrong,
except the generic kinds (`"limit"`, `"timeout"`, `"closed"`, `"net"`,
`"cancelled"`). So malformed JSON from a request, a corrupt gzip stream and a
tampered ciphertext (`crypto.decrypt`: "authentication failed") are the same
kind as an index out of range, though the control-flow guide defines
`"runtime"` as a fault in the program and says most kinds are named after
their library (its `"json"` example is not true today). A handler cannot tell
"the input was bad" from "my code is wrong" without reading the message,
which the project forbids. The `.j` modules and decks already do this
(`totp`, `oauth`, `acme`, `@deckyard/sealbox`); the libraries they build on
do not. Raised by a reader of the crypto talk.

- The rule: input **data** the program did not write itself, read and found
  bad (decoding, parsing, decompressing, decrypting, verifying) throws the
  library's own kind: `"json"`, `"xml"`, `"toml"`, `"yaml"`, `"asn1"`,
  `"encoding"`, `"compress"`, `"archive"`, `"images"`, `"crypto"`, `"time"`
  (parsing), `"uuid"`, ... Misuse stays `"runtime"`: a wrong argument type or
  count, an unknown algorithm name, a key or nonce of the wrong length.
- An audit of every `internal/lib/*` error path decides each one and records
  the kind in `internal/libsig/<lib>.go` and `docs/libraries/<lib>.md`; a
  test per library pins the kind of each data error.
- Messages do not change. `crypto.decrypt`, `crypto.unwrap` and opening a
  sealed box keep one vague message for every cause, now kind `"crypto"`.
- The generic kinds keep priority: a cap is `"limit"` whatever library hits it
  (M29.1), a network failure stays `net.NetError`.
- A plain `Error`, no struct: none of these carries data a caller needs.
- Migration: grep the bundled modules, `examples/`, the decks and the academy
  talks for catches of `"runtime"` around these calls (and for wrappers that
  rewrap them, which must keep passing the generic kinds on); JENNIFER.md's
  error section and the guide's kind table list the new kinds.

#### M29.22 - `fs.readSecretBytes`: refuse a secret others can read

**Done, ships in 0.27.0.** Reading a key, token or password file safely was left to user code:
the crypto talk checks `fs.stat($path).mode & 0o077` by hand, and
`acme.connect`, `jwt` and `@deckyard/sealbox` take key bytes, so they never
see the file. OpenSSH refuses a private key with loose permissions; Jennifer
now offers the same in one call. Raised by a reader of the crypto talk.

- `fs.readSecretBytes(path) -> bytes` and `fs.readSecretString(path) -> string`
  open the file (non-blocking, so a FIFO does not wait for a writer) and
  check the **open handle**, so the file read is the file checked. A refusal
  is kind `"fs"` (ahead of M29.21) with a message naming the path:
  - anything but a regular file after following links;
  - on a writable filesystem: any group or other permission bit
    (`mode & 0o077`, with the mode and the fix, `run chmod 600 <path>`), or
    an owner other than the effective user or root;
  - on a read-only mount (Linux `fstatfs` `ST_RDONLY` on the handle): only a
    group or other **write** bit. Container runtimes mount secrets read-only
    (Docker / Compose `/run/secrets`, Kubernetes secret volumes) with modes
    and owners the strict rule refused: Swarm `0444` root, Kubernetes `0644`,
    a Compose file owned by the host user. Nothing in the container can
    replace such a file, and the mount's operator chose who reads it.
    Detecting a container instead (`/.dockerenv`, cgroups) was rejected as
    guesswork, and a second, laxer function as a choice users get wrong.
    Checked in Docker as root and as uid 1000, on both binaries.
- Over 1 MiB is kind `"limit"`. A missing or unreadable file is the ordinary
  `fs` error.
- On a platform without POSIX permission bits every call is refused (kind
  `"fs"`) rather than read unchecked, until M31.
- `internal/lib/fs/readsecret*.go`, `internal/libsig/fs.go`,
  `docs/libraries/fs.md` ("Reading a secret"), the `fs` line in JENNIFER.md.
  Tests: 0600, 0400 and a symlink to one read; 0640, 0604, 0620, 0610 and
  0644 are refused with the exact message; a directory and a FIFO are refused;
  on a fake read-only mount 0444 / 0644 root and 0600 / 0640 of another owner
  read, the same refused on a writable one, 0666 / 0620 / 0602 / 0464 refused;
  over 1 MiB is `"limit"`, exactly 1 MiB reads; another user's file is
  refused (on a fake `FileInfo`, since a test cannot chown without root).
- The `acme`, `jwt` and `oauth` docs read their keys and secrets with it.
  Left for after the tag: the web app and crypto talks (pinned to 0.26.0,
  which lacks it).

#### M29.23 - the conventions survey: stance 8, checked

**Planned.** Done: the catalogue, the survey tool
(`scripts/conventions-survey.j`), and the first round of findings, from 42
to 0 ([conventions-findings](technical/conventions-findings.md)). Open: the
rules the survey cannot test yet, and the checks that stay. Stance 8 ("one convention everywhere",
[conventions](technical/conventions.md)) asks that the same situation look and
behave the same in every library, module and deck. Readers notice the
exceptions first: `crypto.decrypt` throwing `"runtime"` where `totp` and
`oauth` throw their own kind, a talk loading a key file by hand that its own
checklist says to check. Inconsistencies found after 1.0 can only be fixed
with a major release, so the survey runs now. Raised by readers of the
academy talks.

- **The catalogue first.** `docs/technical/conventions.md` writes down the
  rules that were scattered over JENNIFER.md, the modules index, the deck
  template's AGENTS.md and `deckyard/CLAUDE.md`: names (`xWith`, `xBytes`,
  `is` / `has`, units in names, `allowInsecure...`), argument order, options
  structs, error kinds and structs, caps, text versus bytes, security
  defaults, documentation. Each rule says how it is checked.
- **A survey tool** that lists every deviation, mechanically where it can:
  the built-in signatures in `internal/libsig` (44 libraries), the exports of
  the bundled modules (80) and of the `@jennifer` decks (10), read from their
  docblocks and cheatsheets. Decks of other scopes, `@deckyard` included, are
  outside the project and the survey. Per rule: name
  patterns, parameter names and order, the kinds each function throws (from
  the per-library kind tests of M29.21 and the docs), caps without `"limit"`.
  The output is a findings list, one line per deviation, kept in the
  repository so its shrinking is visible.
- **Checks that stay.** A rule the survey can test becomes a gate: a Go test
  over `internal/libsig` for the built-ins, a lint rule where a user's or a
  deck author's code can break it (so third-party decks get it too), and a
  shared check in the deck template, synced to every deck on the shared layer.
- **Fixes, spread over releases.** A rename keeps the old name `@deprecated`
  for one minor release (the path the bundled modules took to decks); a kind
  change is a breaking change, named in the CHANGELOG. M29.21 (the built-in
  libraries' kinds) and M29.22 (`fs.readSecretBytes`) are the first two slices.
  A deliberate exception is recorded in
  [design-decisions.md](technical/design-decisions.md) and named in the
  catalogue.
- **JENNIFER.md** gets the stance and the catalogue's rules in compact form
  once they are settled.
- **Everywhere the stances are counted** (was "seven"): the README, the
  website, the docs and the stances talk, which has a slide for stance 8
  (done).
- **The first round** (2026-11-20), 42 findings:
  - kept, as [recorded exceptions](technical/design-decisions.md#core-names-spelled-like-a-keyword):
    12 core names spelled like a keyword (`lists.map`, `json.map`, `fs.list`,
    `strings.repeat`, ...), `mcp` throwing `jsonrpc.RpcError` from its
    transport, and `pop`'s two kinds;
  - renamed, the old name `@deprecated` until 0.28.0 (M29.34): `orm.hasOne` /
    `hasMany` to `oneToOne` / `oneToMany`; `validate.isInt` / `isFloat` /
    `isBool` to `integer` / `number` / `boolean`, **and their rule kinds**
    (`Failure.rule` and `localize` keys change); `insecure` (rest, graphql,
    mikrotik) and `withInsecure` (soap, webdav) to `withSkipVerify`; soap and
    webdav `withCaCert` to `withCA`;
  - renamed with no old name, **breaking** for code that names the field:
    `memcache.Session.timeout`, `redis.Session.timeout` and
    `s3.Client.timeout` to `timeoutMs`; `totp.Options.period` to `periodSec`;
    `oauth.DeviceAuth.interval` to `intervalSec`. Parameters (positional, not
    breaking): `ttl` to `ttlSec` in `kvstore` and `session`, `timeout` to
    `timeoutSec` in `telegram.getUpdates` (module and deck);
  - fixed: `uri` checks percent escapes itself instead of reading the cause
    out of `encoding`'s message;
  - the `@jennifer/mikrotik` and `@jennifer/telegram` decks got the same
    changes (their next minor release);
  - academy talks pinned to 0.26.0 still use the old names (`crypto`'s
    `$o.period`, `key-value`'s `$db.timeout`); they move with the talks.
- Not yet tested mechanically: argument order, the kind of each data error
  (M29.21's tests), caps without `"limit"`, struct-name casing (`http.TlsOptions`
  beside `net.TLSOptions`), and the documentation rules.

#### M29.24 - `time.format`: numbers without zero padding

**Done, ships in 0.27.0.** Every numeric strftime code pads (`%d` is `05`), so the US long
date "Friday, November 5, 2027" has no layout: Rosetta Code's `Date format`
solution formats `%A` and `%B` and then splices in `time.day($t)` by hand.
How a number is printed is presentation, so it belongs in the layout, not in a
transform deck.

- **Add** `printf`'s modifier syntax to the numeric codes, with `printf`'s `%d`
  vocabulary: a code is a `%d` with a default `pad` and `fill=0` (`%d`, `%m`,
  `%H`, `%I`, `%M`, `%S` pad to 2, `%j` to 3, `%Y` to 4), and `pad` sets the
  minimum width. `%d|pad=0` prints the day unpadded:
  `time.format($t, "%A, %B %d|pad=0, %Y")`. Not glibc's `%-d` or C's `%e`:
  one modifier syntax across `printf` and `time`, not a second flag
  language (M7's reasoning).
- `printf`'s rules come with it: a `|` right after a code always starts a
  modifier list, `||` is a literal `|`, a bare value ends at the first
  character outside `[A-Za-z0-9_]` (so the `,` above ends `pad=0`), and an
  unknown modifier or one the code does not take is an error. Breaking for
  a layout with a literal `|` right after a code; none in the workspace has
  one (checked 2026-11-21). CHANGELOG notes it.
- Only `pad` for now. `fill` stays `0` (a space-padded day has no use case
  yet), and the name codes (`%A`, `%B`, `%a`, `%b`, `%p`) and `%z` take no
  modifiers: case or truncation would change the value, which `printf` keeps
  out of modifiers too.
- `time.parse` accepts the same layouts: a code with `pad=0` reads one or more
  digits up to the code's default width.
- Docs: the format-code table in `docs/libraries/time.md`, with a pointer to
  the modifier rules in `docs/libraries/io.md`.
- Tests: single- and double-digit values for each code, `%j|pad=0` on day 1
  and day 366, `pad` above the default width, parse round-trips, `||`, and
  `%A|pad=0` / `%d|fill=0` / `%d|bogus=1` refused.

**Requires:** none.

#### M29.25 - `lists.replicate` (and `strings.replicate`)

**Done, ships in 0.27.0.** A list of `n` equal elements is built with an append loop (a
sieve's `list of bool`, a zeroed row of a grid); `binary.make(n, fill)` does it
for `bytes` only.

- **Add** `lists.replicate(xs, n)`: `n` copies of `xs` concatenated, as
  `strings.repeat(s, n)` does for strings. A filled list is
  `lists.replicate([false], $n)`, a repeating pattern `lists.replicate([1, 0], 4)`.
  `n` must be non-negative; a result over the `lists.concat` cap (4194304
  elements, 1048576 on `jennifer-tiny`) is kind `"limit"`, checked before
  anything is built.
- Elements are deep copies: `lists.replicate([[0, 0]], 3)` is three independent
  rows.
- Docs: `docs/libraries/lists.md`, JENNIFER.md's `lists` line,
  `internal/libsig/lists.go`.
- Shipped as `replicate`, not `repeat`: `repeat` is a keyword, and the naming
  rule allows no new keyword spelling. `strings.repeat` (a recorded
  exception until then) got the same name, `strings.replicate`, and is
  deprecated, removed in 0.28.0 (M29.35).

**Requires:** none.

#### M29.26 - `decimal`: a digit cap big integers fit under

**Done, ships in 0.27.0.** `decimal` refuses any value over 100,000 digits (`maxDigits` in
`internal/lib/decimal/arith.go`), so Rosetta Code's `Arbitrary-precision
integers (included)`, 5^4^3^2 with 183,231 digits, throws `"limit"` and cannot
be posted. The cap keeps a `pow` or a parsed exponent (`"1e100000000"`) from
allocating without bound; it does not need to be this low. Measured with Go's
`math/big` on the default build: 5^262144 takes 2 ms and its `toString` 14
ms; a 1,000,000-digit multiply 59 ms and its `toString` 53 ms, at about 415 KB
per value.

- **Raise** `maxDigits` to 1,000,000 on the default build. The cap stays
  exact and shared (parse, every result, `scale`), so every value's
  `toString` still parses back.
- Measure division, `sqrt`-style rounding and `toString` at the new cap on
  `jennifer-tiny` (TinyGo's `math/big`, the 32 MiB heap); keep 100,000 there
  if they are slow, and document the per-build value (as the other caps are).
- No per-call or global setting: one cap per build, like `json`'s defaults.
- Docs: `docs/libraries/decimal.md` (the limit line), JENNIFER.md's
  `decimal` entry ("over 100,000 digits is kind `"limit"`").
- Tests: 5^262144's first and last 20 digits and length; a result one digit
  over the cap throws `"limit"` before it is built; parse at the cap.

**Requires:** none.

#### M29.27 - `lists.sortWith`: sort by a comparator

**Done, ships in 0.27.0.** `lists.sortBy(xs, keyFn)` sorts by one key, and a key must be a
number, string or bool. An order on two keys, or one key descending and the
next ascending, has no form: Rosetta Code's `Sort using a custom comparator`
(descending length, then case-insensitive ascending) builds a zero-padded
string key (`999999 - len`, then the folded word). A comparator says it
directly.

- **Add** `lists.sortWith(xs, cmp)`: `cmp(a, b)` returns an `int`, negative
  when `a` goes first, positive when `b` does, 0 when they are equal. Stable,
  so equal elements keep their order. A non-`int` result is a runtime error
  naming the call, as `sortBy`'s non-comparable key is.
- A comparator that is not a consistent order (`cmp(a, b)` and `cmp(b, a)`
  both negative) gives some permutation of the input, never a crash or a
  lost element; documented, not detected.
- Elements go in as copies, as for `sortBy` (`ctx.Invoke(fn, x.Copy())`).
  Cost: O(n log n) calls of a Jennifer function, so `sortBy` stays the
  faster choice for a single key; the docs say which to use.
- Docs: `docs/libraries/lists.md` ("Sort" and the higher-order table),
  JENNIFER.md's `lists` line, `internal/libsig/lists.go`.
- Tests: two-key order, descending, stability, an empty and a one-element
  list (no call), a throwing comparator, a non-`int` result.

**Requires:** none. Without closures (DRAFT#28) a comparator cannot capture a
parameter (a column number); that is the closure gap, not this entry's.

#### M29.28 - `match` destructures an indexed subject

**Done, ships in 0.27.0.** An arm that binds a payload (`when Node(nd)`) needs the
subject's enum type when the program loads, to find the variant and type the
binder. The resolver's `subjectType` (`internal/parser/resolver.go`) knows a
variable, a constant, and a field read through a local struct, and nothing
else, so `match ($queue[$head])` over a `list of Tree` is refused ("this
`match` subject is not statically an enum type; assign the subject to a
variable first"). Rosetta Code's `Tree traversal` needs a `def` line for it.
The type is already known: it is the declared element type.

- **Add** `IndexExpr` to `subjectType`: a target of type `list of T` gives
  `T`, `map of K to V` gives `V`, recursively, so `$grid[$i][$j]` and
  `$byName["x"].tree` (index, then field) resolve too. A `string` or
  `bytes` target gives no enum and keeps today's error.
- **Two copies.** `checkSpawnEnums` has its own `subjectType` closure (the
  types-only walk of `spawn` bodies) with the same three cases. Make them one
  function over a lookup, so a `match` inside a `spawn` resolves the same
  subjects as one outside it.
- Out of scope, and still refused with the same hint: a call result (a
  function declares no return type; a declared one, M30.21, makes it
  possible) and a field of another module's struct (its definition is not
  resolver-visible).
- Lint and the borrow analysis read the tagged arms, not the subject, so they
  need no change; check that a `match` on an index still evaluates its
  subject once.
- Docs: the `match` section of JENNIFER.md and
  `docs/user-guide/control-flow.md` (which subjects can be destructured); the
  error's hint unchanged.
- Tests: list, map, nested list, index then field, inside and outside a
  `spawn`; exhaustiveness checked for an indexed subject; a string index
  and a call result still refused.

**Requires:** none.

#### M29.29 - `web` answers 400 for a malformed request body

**Planned.** An error that escapes a handler or middleware reaches `web`'s
safety net, which answers 500 and logs it. A client that sends a malformed
body therefore gets a server error: `web.bodyJson` on bad JSON, and
`web.multipartForm` on a broken multipart body, throw and end in a 500
(`multipart.parse` throws kind `"multipart"`, `json.decode` `"runtime"`).
`web.form` has been strict since 0.27.0 (M29.2: its lenient OM-013 decoding
went, as a public lenient mode would break the stances), so a malformed form
body is a 500 too, and `web.csrfCheck` reads the form as middleware: one
malformed POST is a 500 on every form route. The right answer is a 400.

- **A client-fault error.** `web` throws `web.WebError{kind: "web", message,
  status}` (the module's struct for every error of its kind, as
  `rest.HttpError`) when the request body cannot be read: malformed
  urlencoded form, JSON, or multipart, and a body that is not UTF-8 where text
  is required. `status` is 400 (413 stays `httpd`'s for an oversized body).
- **The safety net answers the status.** A `web.WebError` that escapes the
  handler is answered with its status and a short plain-text reason, without
  the 500 error dump; the `onError` hook still sees it. Any other error stays a
  500. A handler that wants its own response catches the error as before.
- **`web.form` / `formValue`** wrap `encoding.parseForm`'s `"encoding"` error
  (and a body that is not UTF-8) into the `WebError` above, so a browser or
  script sending one gets a 400, and `csrfCheck` refuses such a request with
  400.
- `web.bodyJson` and `web.multipartForm` wrap their decoder's malformed-input
  error into the `WebError`; generic kinds (`"limit"` first) pass on
  unchanged, and the safety net answers a `"limit"` from a body parser with
  413 rather than 500.
- **Breaking:** code that caught `"encoding"` from `form`, or `"runtime"` /
  `"multipart"` from `bodyJson` / `multipartForm`, sees `"web"`. Named in the
  CHANGELOG.
- Callers to check: `webapi.j`'s form helpers, the academy `web-app` and
  `docker` talk examples, apps using `web.form`.
- Docs: `docs/modules/web.md` (errors, the safety net), JENNIFER.md's `web`
  entry, the error-kinds table if `web` is listed.
- Tests: each body kind malformed -> 400 with the reason; a `WebError` caught
  in the handler answers as the handler says; `onError` sees it; a non-UTF-8
  form and a bare `%` answered 400; a valid form unchanged; `csrfCheck` on a
  malformed body; a `"limit"` -> 413.

**Requires:** M29.2 (shipped in 0.27.0).

#### M29.30 - profiling a program that does not exit: `--out` and signals

**Done.** `jennifer profile` writes its report when the program ends
normally. A server never does: a signal ends the process before the profiler
writes anything, and `web.serveOn` has no shutdown of its own. Profiling the
registry (2026-11-21) needed a patched copy that caught SIGINT and called
`httpd.shutdown`.

- **`--out FILE`** writes the report to a file instead of stdout, so the
  program's own output and the report never mix.
- **SIGINT / SIGTERM** under `jennifer profile`: stop running, write the
  report, exit with the code the signal would give. A program that catches the
  signal itself (`os.catchSignal`) keeps it; the profiler then writes at its
  exit as now.
- **SIGUSR1** (already reserved for the interpreter's diagnostics dump) also
  writes a snapshot of the profile so far to `FILE.<n>` and keeps running, so
  a server can be profiled per load phase.
- Docs: `docs/technical/cli_profile.md`, the `os.md` SIGUSR1 note.
- Tests: a server loop profiled and stopped with SIGINT writes a complete
  report; SIGUSR1 twice gives two snapshots, the second a superset; a program
  catching SIGINT is not cut short.

**Requires:** none.

#### M29.31 - profile rows a reader can place: functions, source text, `--top`

**Done.** A table row is a position (`/usr/share/jennifer/modules/web.j:1426:21`):
no function name, no source, every module path absolute. Reading the
registry's profile meant looking each hot line up by hand.

- Each row names its **function** (or `<top level>`) and shows the **source
  line**, trimmed; paths are relative to the working directory, a system
  module shown as `<sys>/web.j`.
- **`--by=function`** rolls the statement rows up per function (hits, self,
  cum), the view most questions start from; `--by=statement` stays the default.
- **`--top N`** limits the table; `--focus PATTERN` keeps the rows whose path
  or function matches.
- pprof output carries the function names already given to the table, so `go
  tool pprof -top` agrees with it.
- Docs and tests: `cli_profile.md`; golden tables for a small program.

**Requires:** none.

#### M29.32 - profile: waiting apart from working

**Done.** Times are wall-clock, so a statement blocked in a builtin
(`httpd.accept`, `channel.recv`, `task.wait`, `time.sleep`, a `net` read) shows
its wait as self time: in the registry's profile the accept loop and a sleep
loop were the two "hottest" lines. With concurrent tasks, CUM also adds up the
time of every worker (10 s of load showed as 68 s).

- A **wait** column: time spent inside builtins that block, left out of self.
  `--by=function` gets it too. As built, the blocking builtins are a fixed
  table (`blockingBuiltins` in `profiling.go`: `time.sleep`, the `channel`
  and `task` waits, the `net` reads and accepts, `serial.read`,
  `httpd.accept`) rather than the cancellation checkpoints: on the main task
  those builtins never reach a checkpoint. A module's own network wait, or
  `meta.callMain` waiting for the host lock, still counts as self.
- The header states the wall time of the run and the number of tasks
  profiled, and CUM is labelled as summed over tasks.
- Docs and tests: `cli_profile.md` ("Reading a parallel profile"); a program
  that sleeps 1 s in a loop shows the time as wait, not self.

**Requires:** none. Composes with M29.31.

#### M29.33 - remove the deprecated bundled modules (was M30.1, M28.32)

**Done.** Nine single-vendor clients graduate into decks (see
M26): four notification clients, three metrics clients and two MikroTik
clients. Each deck lives in
the `jennifer-language` organization with the module's surface, so migrating
changes only the import:

| Module | Deck | Import |
|---|---|---|
| `gotify` | [`deck-gotify`](https://github.com/jennifer-language/deck-gotify) | `import "@jennifer/gotify/" as gotify;` |
| `slack` | [`deck-slack`](https://github.com/jennifer-language/deck-slack) | `import "@jennifer/slack/" as slack;` |
| `discord` | [`deck-discord`](https://github.com/jennifer-language/deck-discord) | `import "@jennifer/discord/" as discord;` |
| `telegram` | [`deck-telegram`](https://github.com/jennifer-language/deck-telegram) | `import "@jennifer/telegram/" as telegram;` |
| `prometheus` | [`deck-prometheus`](https://github.com/jennifer-language/deck-prometheus) | `import "@jennifer/prometheus/" as prometheus;` |
| `statsd` | [`deck-statsd`](https://github.com/jennifer-language/deck-statsd) | `import "@jennifer/statsd/" as statsd;` |
| `influxdb` | [`deck-influxdb`](https://github.com/jennifer-language/deck-influxdb) | `import "@jennifer/influxdb/" as influxdb;` |
| `mikrotik` | [`deck-mikrotik`](https://github.com/jennifer-language/deck-mikrotik) | `import "@jennifer/mikrotik/" as mikrotik;` |
| `mactelnet` | [`deck-mactelnet`](https://github.com/jennifer-language/deck-mactelnet) | `import "@jennifer/mactelnet/" as mactelnet;` |

`gotify` is the archetype and the first-party sample deck. All nine are
deprecated from 0.26.0: a module-level `@deprecated` in `modules/<name>.j`, a
notice in `docs/modules/<name>.md` and the catalogs. Each deck carries its
module's `_test.j` overlay and replaces the Go test with `.j` tests against a
loopback `httpd` server (or UDP socket, `statsd`; a fake RouterOS API or
MAC-Telnet peer for the MikroTik pair), as `deck-gotify` does. All nine decks
set the floor `>=0.26.0`, the release that deprecates the modules, so they run
on the releases that still bundle them. `@deckyard/routeros` imports the
bundled `mikrotik`; it moves to `@jennifer/mikrotik` (a `[decks]` entry) in a
release before 0.28.0. All nine ship through the 0.27 releases and are
removed on the development line right after 0.27.0 is tagged, so 0.28.0 is
the first release without them.

Removal deletes, per module, `modules/<name>.j`, `modules/<name>_test.j`,
`examples/modules/<name>_demo.j` and `docs/modules/<name>.md`, plus the Go
tests `cmd/jennifer/gotify_test.go`, `cmd/jennifer/telegram_test.go`,
`cmd/jennifer/chatnotify_test.go` (`TestSlackWebhook`, `TestDiscordWebhook`),
`cmd/jennifer/prometheus_test.go`, `cmd/jennifer/statsd_test.go`,
`cmd/jennifer/influxdb_test.go`, `cmd/jennifer/mikrotik_test.go` and
`cmd/jennifer/mactelnet_test.go`; drops their rows and entries from
`docs/modules/index.md`, `modules/README.md`, `docs/SUMMARY.md`,
`docs/technical/filemap.md` and `JENNIFER.md`; and rewords the remaining
mentions (`README.md`, `docs/introduction.md`, the MAC-Telnet links in
`docs/libraries/crypto.md` and `docs/libraries/net.md`) to point at the decks.
`http.requestRawBody` / `requestRawBodyTls`, added for `telegram` uploads,
stay, as does `multipart`, which `web` also imports; so do the EC-SRP
primitives `crypto.mtwei*` and `net.bindToDevice`, which `deck-mactelnet`
needs. Breaking: an import of
any of the nine `.j` files fails to resolve from 0.28.0, so the 0.28.0
release notes (M28.24) carry the migration (`deck.toml` + `jvc install` + the
`@jennifer/` import). Allowed before 1.0 (0.x may break in any release); from
1.0 on, a module moving to a deck stays bundled and deprecated until the next
major release, per M26.

#### M29.34 - remove the stance-8 deprecated names (was M30.18)

**Done.** The 0.27.0 conventions survey (M29.23) renamed thirteen names and
kept the old ones `@deprecated` for one minor release: `orm.hasOne` /
`hasMany`, `validate.isInt` / `isFloat` / `isBool`, `rest` / `graphql` /
`mikrotik` `insecure`, `soap` / `webdav` `withInsecure` and `withCaCert`. They
go in 0.28.0; the survey's "Deprecated names still shipped" section is then
empty of the bundled names, and lists whatever the next round deprecates (it
still shows `@jennifer/mikrotik.insecure`, which goes in that deck's next
minor release). `mikrotik`'s `insecure` went with the module (M29.33).

#### M29.35 - remove the slice duplicates and the one-value printf

**Done.** The builtins and call shapes are gone, and lint L302
reports a leftover call with what to write instead (L114 and L109 leave those
calls to it). Removing them showed that the borrow analysis's callback table
(`jCallbackBuiltins`) still named `testing.runWith` / `assertThrowsWith`, not
the `runArgs` / `assertThrowsArgs` that replaced them in 0.27.0, so a function
reaching `.j` code through the new names could be judged unable to write a
global; the table names them now. Deprecated in 0.27.0 (lint L301, the survey's
"Deprecated names still shipped"): `strings.substring`, `lists.slice` /
`head` / `tail` / `pop` and `binary.slice`, whose job the language's slices do
(`$s[a..b]`, `$xs[..n]`, `$xs[len($xs) - n..]`, M21.11) with the same rune
indexing and strict bounds; and `io.printf` / `sprintf` / `eprintf` with one
non-string value, a verbatim print under another name (rejected.md): write
`"%v"`; `strings.repeat`, renamed `strings.replicate` (a keyword
spelling, M29.25); and `testing.runWith` / `assertThrowsWith`, renamed
`runArgs` / `assertThrowsArgs`; and `convert.bytesFromString` /
`stringFromBytes`, a UTF-8-only copy of `encoding.encode` / `decode`, which
take `"utf-8"` and the UTF-16 / UTF-32 forms since 0.27.0 (stance 1); and
`convert.toString(n, radix)`, the digits `"%d|base="` prints (stance 3);
`crypto.hkdf` / `pbkdf2` / `argon2id` / `scrypt`, one function per algorithm
where the rest of `crypto` has one per operation (now `crypto.deriveKey`,
stance 8); and the two-bound `lists.range(a, b)`, which is `a..b` (or the step
`-1` written out). Found by the stance review of 2026-11-21.

- Remove the fifteen builtins, the radix form of `toString` and the two-bound
  `lists.range` and the one-value shape (a single argument is then
  always the format, and a non-string one is a runtime error naming `"%v"`).
- Lint: L302 (removed API) reports a call to any of them, with the slice to
  write; L301 entries go.
- Migrate the callers first, mechanically. Done in 0.27.0 for the bundled
  modules (213 calls, and 499 of the convert pair), the examples and the
  docs; the nine deprecated modules keep the pair and go with it. Done on
  2026-11-23 for `decks/`, `deckyard/`, `apps/`, the rosetta solutions, the
  website and the academy's tools and talks (about 2,700 calls in 71
  repositories, `jennifer lint` 0.27.0 reporting no L301 after it); each
  raised its floor to `>=0.27.0`, which `encoding.encode("utf-8", ...)` and
  the other replacements need, so a deck ships it in its first release
  after the 0.27.0 tag. Left as written: the academy talks pinned to 0.26.0
  (the `release-0-26` talk among them), which move with their talks. The
  removal itself is all that remains.
- The Go tests that embed Jennifer source (`internal/`, about 100 calls)
  move with the removal.
- Docs: the deprecation rows go from the library pages and the cheatsheet;
  JENNIFER.md's slicing note keeps one sentence.

**Requires:** none.

#### M29.36 - `sql`: verified TLS by default, errors a caller can test

**Planned.** Two gaps the stance review found (2026-11-21).

- **TLS.** Nothing requires it: go-sql-driver/mysql defaults to `tls=false`,
  so the password crosses in plaintext, and pgx to `sslmode=prefer`, which
  falls back to plaintext without verifying. The catalogue says credentials
  need TLS unless an option opts out. `sql.open` requires verified TLS (MySQL
  `tls=true` or a registered CA, Postgres `sslmode=verify-full`) and refuses a
  DSN that weakens it unless `sql.openWith(driver, dsn, sql.Options{
  allowInsecure})`; a local Unix socket is exempt.
- **Errors.** Every failure is `"runtime"`: a refused connection (where `net`
  says `"net"`), the client query timeout, and server errors with no SQLSTATE
  or error number, so a unique-key violation can only be spotted by its
  message. Add `sql.SqlError{kind: "sql", message, code, sqlState}` for server
  errors, `"timeout"` for deadlines, and pass a transport failure on as
  `net.NetError`.
- While there: one driver name each (`mysql`, `postgres`; drop the
  `mariadb` / `postgresql` aliases), `asString` validates UTF-8, `asFloat`
  refuses NaN and infinity, `asBool` takes only 0 / 1, and the process-wide
  `setQueryTimeout` becomes `Options.queryTimeoutMs`.
- **Breaking** (TLS refusal, kinds, aliases); named in the CHANGELOG. Tests
  against the CI database containers.

**Requires:** none.

#### M29.37 - `net`: a default deadline for every connect and lookup

**Planned.** The catalogue says every network call has a default deadline.
`net.connect` / `connectTLS` / `startTLS` without `timeoutMs` (or with 0) have
none, and `net.lookup` / `reverseLookup` cannot be bounded at all; the `http`
module adds its own `DEFAULT_TIMEOUT_MS` to make up for it.

- A default connect and handshake deadline (30 s), `0` meaning the default
  and an explicit option for none; `timeoutMs` on the lookups with the same
  default. Timeouts throw `"timeout"`.
- The options move into a struct while the signature changes:
  `net.connectWith(addr, net.ConnectOptions{timeoutMs, tls})`, retiring the
  type-sniffing optional arguments of `connectTLS` (see M29.45).
- Docs: `net.md`; tests with an unroutable address and a stalled handshake.

**Requires:** none.

#### M29.38 - `kv.incr` on an absent key

**Planned.** `kv.incr` returns -1 for an absent key, but a signed delta makes
-1 a real count as well (a decrement to -1 and an absent key read the same).
Throw for an absent key, as `getValue` does, with `has` as the test; the
module stores built on `kv` (`kvstore`, `session`, `ratelimit`) follow.
**Breaking** for code reading -1.

**Requires:** none.

#### M29.39 - `ml` models as values

**Planned.** An `ml.Model` is a handle into a per-run registry, so copies
share one entry and `ml.free` on a copy kills the original (verified
2026-11-21), against value semantics and `ml.md`'s "safe to share across
value-copies". Make a model an immutable value the garbage collector owns,
like `json.Value`; deprecate `ml.free` (a no-op, then removed in 0.29.0).
While the signatures change, the hyper-parameters (`logisticRegression`'s
learning rate and epochs, the trees' `nTrees` / `maxDepth`, `kMeans`'s
`maxIter`) move from chained optional arguments into options structs
(`randomForestWith(X, y, ml.ForestOptions{...})`, zero = default).

**Requires:** none.

#### M29.40 - integers outside int64 in `json`, `toml` and `yaml`

**Planned.** One situation, three behaviours (verified 2026-11-21): `json`
reads `12345678901234567891` as a float and silently loses digits, `toml`
throws, `yaml` keeps an unquoted one as a string. All three throw the data
error (the library's kind, M29.21) naming the fix: quote the number, or read
it with a new `asDecimal(v[, ptr])` accessor that keeps every digit. Also:
`asFloat` / `asFloats` on an int above 2^53 throw when the float does not
round-trip, instead of rounding.

**Requires:** M29.21 for the kind; none for the behaviour.

#### M29.41 - `compress` refuses data after the stream

**Planned.** `compress.unpack("zlib" | "deflate", ...)` ignores bytes after
the end of the stream and silently drops a second concatenated stream, while
`"gzip"` reads every member and refuses trailing junk (verified 2026-11-21).
Throw on unread input after the stream for zlib and deflate; document gzip's
multi-member rule. Tests: trailing junk, two streams, an exact stream.

**Requires:** none.

#### M29.42 - strict quoted-printable

**Planned.** `encoding.fromText("quoted-printable", ...)` keeps a malformed
`=XX` escape as literal text (verified 2026-11-21), while base32 and base64
accept only canonical text. Refuse a malformed escape; decide whether hex
accepts only lowercase (canonical) as base64 does, or record why it takes
both cases.

**Requires:** none.

#### M29.43 - `http`: one request function

**Planned.** `http.send(method, url, headers, body, Options)` with zero
`Options` is a one-shot request, and `Options` holds `timeoutMs`, `maxBytes`
and `tls`, so `requestWith`, `requestTls` and `requestWithTls` are second ways
to call it. `requestRawBody*` and `requestBytes*` exist because `send` has no
bytes form.

- Add `http.sendBytes` (a bytes body and a `BytesResponse`); deprecate
  `requestWith`, `requestTls`, `requestWithTls`, `requestRawBody`,
  `requestRawBodyTls`, `requestBytes` and `requestWithBytes` (removed in
  0.29.0). `request` and the verb shortcuts (`get`, `post`, ...) stay.
- `http.TlsOptions` becomes `net.TLSOptions`, ending the field copy, and the
  docblock's "in addition to the system roots" is corrected (a `caCert`
  replaces them, as in `net`).

**Requires:** none.

#### M29.44 - one shape for a caller-set cap

**Planned.** Three shapes for "the caller sets a cap" (stance review,
2026-11-21): `archive.unpackWith(..., UnpackOptions)` where 0 is the default
and a negative value means unlimited; `compress.unpack(..., maxBytes)` and
`images.decode(..., maxPixels)` as positional optional ints that refuse
values below 1; and `net.readAll(maxBytes < 0)` / `http` `maxBytes` as
unlimited. The catalogue says a negative count throws.

- One rule, written into `conventions.md`: an options struct (`xWith`),
  zero = the default, a negative value throws, and "unlimited" is
  spelled `MaxInt` where a caller truly needs it.
- `compress.unpackWith` / `images.decodeWith` with options; the positional
  forms deprecated. `archive`'s negative = unlimited and its `"tgz"` alias
  (two names for `"tar.gz"`) go.

**Requires:** none.

#### M29.45 - timeouts: the shapes a wait may take

**Planned.** "Bound this wait" has five shapes today: a separate
`xTimeout(ms)` function (`task`, `channel`), an optional trailing `timeoutMs`
(`net.connect`, `readAll`, `readN`), stateful `setRead/WriteDeadline` (`net`,
`serial`), an options field (`httpd`, `http`), and a global setter
(`sql.setQueryTimeout`); `time.sleep` is the only wait taking a
`time.Duration`. Negative and oversized values are handled five ways too.

- Write the allowed shapes into `conventions.md` (an options field for a
  call that has options, `xTimeout(ms)` for a blocking primitive, deadlines
  for streams) and convert the outliers: `net` (M29.37), `sql` (M29.36),
  `term.readByteTimeout`'s `-1` / `-2` sentinels to `"timeout"` / `"closed"`,
  `fs.watch` / `os.spawn` optional arguments to options structs.
- Timeouts for `channel.select`, `task.waitAll` and `os.wait`, so no one
  spawns a waiter to bound them.
- Decide `time.sleep(ms)` against `Duration`, and make negative or oversized
  values throw everywhere (record `time.sleep(0)` returning at once).

**Requires:** M29.36, M29.37 for their parts.

#### M29.46 - handles: one rule for use after close

**Planned.** `net` throws `"closed"` for a closed handle; `fs`, `kv`,
`serial`, `sql`, `i2c` and `httpd` throw `"runtime"`, so `httpd.accept` after
`shutdown` cannot be told from a failure the way `net.accept` can. And
`close` is idempotent in `sql` and `kv` but throws on a second call in `fs`,
`net`, `term` and `channel`, which matters for `defer`.

- Every use of a closed or consumed handle (a finalized `hash` / `crc` /
  `compress` stream included) throws `"closed"`.
- One rule for a second `close`, written into `conventions.md` (idempotent is
  kinder to `defer`), applied everywhere.
- `net.eof` (a 20 ms guess, named like the exact `fs.eof`) deprecated:
  an empty `readBytes` is the answer.

**Requires:** none.

#### M29.47 - decoders: the same options and accessors

**Planned.** `json.decodeWith(s, DecodeOptions{maxDepth, maxNodes})` lowers
the caps; `toml`, `yaml`, `xml` and `asn1` have fixed caps only, and `xml`
takes namespaces as a second positional map.

- `decodeWith` with a shared `DecodeOptions` for `toml`, `yaml`, `xml` (with
  `namespaces`) and `asn1`; the two-argument `xml.decode` deprecated.
- `yaml.asBytes` for a `!!binary` node, which today cannot be read.
- `toml.asDatetime` refuses a local date, time or date-time (no offset)
  instead of guessing UTC; `typeOf` names the local forms and an accessor
  returns their fields.
- `toml.decode`'s messages get the `toml.decode:` prefix, the column, and
  "unexpected end of input" for EOF.

**Requires:** none.

#### M29.48 - second ways that remain

**Planned.** Smaller duplicates from the stance review (2026-11-21), each
deprecated in favour of the one way, removed in 0.29.0:

- `stats.multipleRegression` -> `ml.linearRegression` (same numbers; stats
  keeps simple regression with its inference);
- `convert.toInt(float)` truncating -> accept only an integral float, so the
  caller picks `math.trunc` / `round` / `floor` / `ceil`;
- `os.hasFlag` / `os.flag` (half a parser, `--port=80` silently `""`) -> the
  `args` module;
- the `gpio` module -> the `gpio` library (same API; the module is sysfs,
  which the kernel deprecates), aligning chip selection first;
- `httpd.serveFileEtag` / `serveDirEtag` -> an `etag` option;
  `listenTLS` / `listenTLSWith` -> certificate fields in `httpd.Options`;
- `decimal.toString` -> `convert.toString` / `%v`; `images.rgba` -> the
  `images.Color` literal.

**Requires:** none.

#### M29.49 - hardware libraries: errors, caps and handles alike

**Planned.**

- `serial` with parity on reads a byte with a parity error as `0x00`, which a
  real NUL also is: throw a `"serial"` error (or report it with `PARMRK`).
- `i2c.read` / `readReg` / `write` / `writeReg` silently clamp to i2c-dev's
  8192 bytes, while `writeRead` and `spi.transfer` throw `"limit"`: throw
  `"limit"` everywhere.
- `spi.openWith(path, spi.Options{mode, speedHz, bitsPerWord})` like
  `serial.openWith`, replacing `open` + `configure`; `gpio` gets a chip handle
  (`gpio.open(path) -> gpio.Chip`) instead of the process-wide `gpio.chip` and
  `JENNIFER_GPIO_CHIP`.
- `serial.write` returns `null` like `net.writeBytes` and `fs.writeBytes`
  (the count is always the length).

**Requires:** none.

#### M29.50 - outside text is validated UTF-8

**Planned.** The catalogue returns outside text as a string only after UTF-8
validation (as `fs.readString` and `httpd` do). `os.getEnv`, `os.ARGS`,
`fs.list` (and `walk`, `readlink`, `realpath`) and `sql.asString` pass
invalid bytes through (verified with `\xff` names and values). Validate,
naming the byte offset, and add bytes forms where a caller needs the raw
value (`os.getEnvBytes`, `sql.asBytes` exists).

**Requires:** none.

#### M29.51 - `crypto`: names and types that say what they do

**Planned.**

- `"rsa-sha256"` / `384` / `512` in `sign` / `verify` hide PKCS#1 v1.5
  padding, while `wrap` spells `"rsa-oaep-sha256"`: rename to
  `"rsa-pkcs1-sha256"` (old names deprecated), leaving room for
  `"rsa-pss-sha256"`.
- `jwkToPem` returns a `string`, but `verify` and every other PEM argument
  take `bytes`: return `bytes`.
- One type for a password: `hashPassword` / `verifyPassword` take `string`,
  the KDFs `bytes`. Pick one (bytes, as the KDFs) and convert at the edge.
- `hkdf` / `pbkdf2` accept the `hash` names minus `md5` as their docs say
  (`sha384`, `sha3-*`, `blake2*`), or the docs list the three they take.
- Weak algorithms are offered by name for interop, never as a default, and
  tagged "(weak)" in every unknown-name list, as `hash` tags md5 / sha1;
  record the rule in `design-decisions.md`.

**Requires:** none.

#### M29.52 - small consistency fixes from the stance review

**Planned.** Each small, each found 2026-11-21:

- `strings.replace` with an empty `old` inserts between every rune; refuse
  it, as `count` and `split` do.
- `regex.find` returns a sentinel `Match` (`start == -1`) while `lists.find`
  throws on no match: align, or record why.
- `strings` and `convert` messages without the `lib.fn:` prefix (`count():`,
  `toInt("abc"):`) get it.
- `intl`: one locale-normalisation rule for `tr` / `setLocale` /
  `formatNumber`; `setLocale` refuses an unknown locale; misuse is
  `"runtime"`.
- `meta.hasCapability` throws on an unknown name and documents `"sql"`.
- `time` names: `fromSeconds` (a Duration) beside `fromUnix` (a Time), and
  `Millis` / `Milliseconds` in one API; one spelling each.
- `kv.openFile` loses writes across processes silently where `flatdb.save`
  refuses a lost update: detect a changed file on flush, or record it.
- `httpd.serveDir` follows a symlink out of `root`: contain the resolved path
  by default, `Options.followSymlinks` to opt in.
- `%f|prec` rounds the binary float while `intl.formatNumber` rounds the
  shortest decimal (`1002.675`: `1002.67` against `1,002.68`): align, or
  document the difference.

**Requires:** none.

#### M29.53 - `profile --allocs` counts the copies into a literal

**Done.** The allocation profile misses a whole class of copies: a value
put into a struct, list, map or enum literal is deep-copied by
`literalElementValue` (`internal/interpreter/interpreter.go`) with a plain
`v.Copy()`, which no profiler hook sees. `Cur{names: $c.names, at: $c.at + 1}`
with 17,500 strings in `names` costs 1.2 ms a call and shows nothing, while
the same copy spelled `def out as Cur init $c` is reported. Optimising
`@deckyard/sheets`' row cursor against the profile, the literal form looked
like it had removed a 3.6 GiB copy; it had only hidden it.

Record the copy where it happens (route it through `eagerCopy` with the
literal's position, or record at the `evalStructLit` / `evalListLit` /
`evalMapLit` / enum call sites), and check the other unrecorded `Copy()`
sites the same way: lvalue writes, the throw snapshot, `defer` arguments, the
catch binding, slice elements and the environment snapshot. A test per site
that a profiled program reports it.

**Requires:** none.

#### M29.54 - `binary`: writing numbers as well as reading them

**Planned.** `binary` decodes fixed-width numbers natively (`readInt`,
`readUint`, `readFloat`, the bulk `readInts` / `readFloats`) but has nothing to
encode one, so every program that writes a binary format builds its bytes one
at a time in `.j`: `@deckyard/sheets` carries a hand-written `le32` for its ZIP
writer and its packed shared-string table, and the deck ZIP, WAV and protocol
writers each carry their own. That costs the dozen interpreted operations per
value the `read*` docs warn about, and every copy is a place to get the sign or
the byte order wrong. Raised by a user.

The writers mirror the readers, by the stance-8 rules (format name first, then
the subject; builders return the new value; a value that does not fit throws
rather than being truncated):

| Function | Returns | Mirrors |
|---|---|---|
| `binary.fromInt(order, n, size)` | `bytes`: `n` in `size` bytes (1, 2, 4, 8), two's complement | `readInt` |
| `binary.fromUint(order, n, size)` | `bytes`: `n` unsigned in `size` bytes; size 8 takes 0 up to the int maximum | `readUint` |
| `binary.fromFloat(order, x, size)` | `bytes`: IEEE 754 in 4 or 8 bytes | `readFloat` |
| `binary.fromInts(order, xs, size)` / `fromFloats(order, xs, size)` | `bytes`: every element, back to back | `readInts` / `readFloats` |
| `binary.setInt(order, b, offset, n, size)` / `setUint` / `setFloat` | `bytes`: `b` with the value written at `offset` (`$b = binary.setUint("le", $b, 4, $len, 4);`) | `readInt` / `readUint` / `readFloat` |

- **Names.** `from*` as `convert.fromCodepoint` and `time.fromUnix` build one
  value from another; `set*` as `json.set` and `xml.setAttr` return the value
  changed. Not `write*`: in the libraries a write is I/O (`fs.writeBytes`).
- **Errors.** `order` other than `"le"` / `"be"`, a size the type does not
  have, an `n` outside the size's range (`fromUint(..., 256, 1)`, a negative
  unsigned), an offset or span past the end of `b`, and a NaN or infinity (as
  `readFloat` refuses on reading) are `"runtime"`, misuse in a built-in, with
  the range in the message. A float that does not fit 4 bytes (beyond the
  float32 range) throws rather than becoming an infinity; within it, it is
  rounded to the nearest float32 as IEEE 754 does. A bulk result over the
  allocation cap is `"limit"`.
- Signatures in `internal/libsig/binary.go`; `docs/libraries/binary.md` gets a
  "Writing numbers" section beside "Numbers"; tests round-trip every size and
  order against the readers, pin each refusal, and check `setUint` leaves the
  rest of `b` alone. Then `@deckyard/sheets` drops its `le16` / `le32` (and the
  other decks theirs) in their first release on a floor that has them.

**Requires:** none.

#### M29.55 - release every deck before 0.28.0

**Planned.** M29.35 removes builtins that the decks' latest releases still
call, and a consumer installs a release, not `main`: a program using one of
them stops working the day 0.28.0 is released. `main` of every deck was
migrated on 2026-11-23 (no removed call left, floor `>=0.27.0`); the releases
were not. 57 decks (11 `@jennifer`, 1 `@mplx`, 45 `@deckyard`) have a removed call
in code, not only in comments, in their newest `N.N.N` tag.

| Deck | Release | Next |
|---|---|---|
| `@jennifer/discord` | 0.1.0 | 0.2.0 |
| `@jennifer/gotify` | 0.3.0 | 0.4.0 |
| `@jennifer/influxdb` | 0.2.0 | 0.3.0 |
| `@jennifer/mactelnet` | 0.1.0 | 0.2.0 |
| `@jennifer/mikrotik` | 0.2.0 | 0.3.0 |
| `@jennifer/oauthext` | 0.2.0 | 0.3.0 |
| `@jennifer/prometheus` | 0.3.0 | 0.4.0 |
| `@jennifer/slack` | 0.1.0 | 0.2.0 |
| `@jennifer/statsd` | 0.3.0 | 0.4.0 |
| `@jennifer/telegram` | 0.3.0 | 0.4.0 |
| `@jennifer/texttransform` | 0.3.0 | 0.4.0 |
| `@mplx/clispinner` | 0.2.0 | 0.3.0 |
| `@deckyard/abif` | 0.2.0 | 0.3.0 |
| `@deckyard/anthropic` | 0.1.0 | 0.2.0 |
| `@deckyard/astm` | 0.2.0 | 0.3.0 |
| `@deckyard/bioinformatics` | 0.4.0 | 0.5.0 |
| `@deckyard/codehost` | 0.3.0 | 0.4.0 |
| `@deckyard/d64` | 0.2.0 | 0.3.0 |
| `@deckyard/docker` | 0.2.0 | 0.3.0 |
| `@deckyard/epub` | 0.2.0 | 0.3.0 |
| `@deckyard/escl` | 0.3.1 | 0.4.0 |
| `@deckyard/fhir` | 0.3.0 | 0.4.0 |
| `@deckyard/forensicgenetics` | 0.6.0 | 0.7.0 |
| `@deckyard/forgejo` | 0.4.0 | 0.5.0 |
| `@deckyard/frigate` | 0.4.0 | 0.5.0 |
| `@deckyard/fsm` | 0.2.0 | 0.3.0 |
| `@deckyard/gemini` | 0.1.0 | 0.2.0 |
| `@deckyard/geo` | 0.2.0 | 0.3.0 |
| `@deckyard/git` | 0.2.0 | 0.3.0 |
| `@deckyard/github` | 0.4.0 | 0.5.0 |
| `@deckyard/gitlab` | 0.5.0 | 0.6.0 |
| `@deckyard/hl7` | 0.2.0 | 0.3.0 |
| `@deckyard/jellyfin` | 0.2.0 | 0.3.0 |
| `@deckyard/jsonpath` | 0.2.0 | 0.3.0 |
| `@deckyard/jsonschema` | 0.2.0 | 0.3.0 |
| `@deckyard/koofr` | 0.4.0 | 0.5.0 |
| `@deckyard/magick` | 0.2.0 | 0.3.0 |
| `@deckyard/mitoleaf` | 0.3.0 | 0.4.0 |
| `@deckyard/money` | 0.2.0 | 0.3.0 |
| `@deckyard/mp3` | 0.2.0 | 0.3.0 |
| `@deckyard/ngs` | 0.3.0 | 0.4.0 |
| `@deckyard/onvif` | 0.3.0 | 0.4.0 |
| `@deckyard/openai` | 0.1.0 | 0.2.0 |
| `@deckyard/proxmox` | 0.2.0 | 0.3.0 |
| `@deckyard/routeros` | 0.5.0 | 0.6.0 |
| `@deckyard/seafile` | 0.2.0 | 0.3.0 |
| `@deckyard/sealbox` | 0.1.0 | 0.2.0 |
| `@deckyard/statfiles` | 0.2.0 | 0.3.0 |
| `@deckyard/steam` | 0.2.0 | 0.3.0 |
| `@deckyard/synology` | 0.2.0 | 0.3.0 |
| `@deckyard/tmdb` | 0.2.0 | 0.3.0 |
| `@deckyard/unraid` | 0.3.0 | 0.4.0 |
| `@deckyard/vies` | 0.3.0 | 0.4.0 |
| `@deckyard/virtualbox` | 0.1.0 | 0.2.0 |
| `@deckyard/virtualboxws` | 0.3.0 | 0.4.0 |
| `@deckyard/vmware` | 0.3.0 | 0.4.0 |
| `@deckyard/zmachine` | 0.2.0 | 0.3.0 |

- "Next" is the version on `main`: a minor release (the floor moved to
  `>=0.27.0`, so not a patch). The bump is prepared in each deck's
  `commit.txt` and passes its gate; discord, slack and d64 were already
  bumped and only need the tag.
- Each release goes through the usual tag, push to both remotes and
  `jvc publish`.
- The check before 0.28.0 is tagged: no deck's newest tag calls a removed
  builtin outside a comment, and the registry serves those tags.
- The `@jennifer` decks that replaced bundled modules (M29.33) are on the
  list too; their next release is also the first one users need once the
  modules are gone.

**Requires:** M29.35.

## M30 - 0.29.0

The removal of the deprecated bundled modules, copy elision, streaming and
binary formats, function signatures (declared return types, library and module
functions as values) and grapheme clusters. Entries keep their former M28
number as `(was M28.n)`.

#### M30.2 - value-semantics copy elision (return / const read) (was M28.6)

**Planned.** Value semantics deep-copy a compound on every store, which is correct
but compounds: reading 50,000 points into a nested `Document` copies the point list
~7 times (out of the reader, into each literal, into each enclosing list), and a
`def const ... as bytes` probe profiled **11,486 eager copies** of an immutable
value. `GOGC=20` barely moves peak RSS (1.23 -> 0.93 GB), confirming retention from
copying rather than collectable garbage. The read-only-parameter borrow (note 24)
already removes the argument-binding copy; this extends that reasoning to two more
hand-off shapes, keeping value semantics observably intact (a borrow of an
immutable source is identical to a copy).

- **`const` compound read.** A `def x as T init SOME_CONST` copies a deeply-immutable
  value for no semantic reason. Elide it where the target cannot mutate the shared
  backing (the borrow analysis already in place), or mark const-sourced bindings
  copy-on-write.
- **Return-then-store hand-off.** A value returned and immediately stored
  (`def xs init readSegment(...)` then `$segs[] = Segment{points: $xs}`) has one live
  reference throughout; an escape analysis narrow enough to catch `return $local`
  removes most of the seven copies without weakening any guarantee.
- **A field read into a literal.** `Cur{names: $c.names, at: $c.at + 1}`, the
  way to build a changed copy of a parameter the function never writes,
  deep-copies every field it reads, because `literalElementValue` copies all
  but fresh literals and moved variables. A row cursor carrying a 17,500-string
  table paid 1.2 ms a row for it (`@deckyard/sheets`, 2026-11-23), and moved
  its table into `bytes`, whose copy is one block move. Reading a field off a
  borrowed parameter into a literal that is then returned has one live
  reference, like the return hand-off above. (Profiling it needs M29.53.)
- **Document the existing guarantee** the geo report verified as a positive: a large
  `bytes` argument is *not* deep-copied (note-24 borrow) - state it as a contract, not
  an observation.

**Requires:** the note-24 borrow/escape machinery. Pin value-semantics parity with
the `value_alias` stress tests (note 10) for every elision.

#### M30.3 - borrow nested collections (was M28.21)

**Planned.** A parameter or local of a nested collection type (`list of list of
float`, `map of string to list of int`) never borrows, because
`stampDeclaredType` stamps the inner element type into each nested element, which
would write into the caller's shared backing. A matrix helper over a 20000 x 10
matrix called 2000 times takes 41 s copying. Skip re-stamping elements already
carrying the declared type (a read-only check on the shared backing) and allow
the borrow when every element already matches, falling back to copy otherwise.
Needs alias-stress coverage in `value_alias_test.go`.

A deck hit it since: `@deckyard/forensicgenetics` pedigree peeling passed its
messages as a `list of list of float` and copied 2.3 GiB on a 300-generation
chain, which fell to about nothing once the messages were wrapped in a struct (a
struct holding the same nested value is borrowed). The every-element check above
is O(n) per call - cheaper than the copy, but not free, and checking only one
representative element per level would be unsound (elements from `json.decode`
or appends need not carry the same stamp). A cheaper sound form: on the borrow
path `bindArg` stamps only the top-level value - its `ElemTyp` / `KeyTyp` /
`ValTyp` live in the callee's own copy of the `Value` header, so that write is
private - and skips the recursion into the elements, which is the only part that
writes into the caller's backing. A borrowed parameter is never written through,
so the nested stamps exist only to type-check writes it cannot make; a value that
escapes (passed on to a writing callee, stored, returned) is copied and stamped
at that binding as today. Before choosing it, check that nothing on a read path
needs the nested stamps (passing the value on to another borrowed parameter,
`typeOf`-style introspection, encoding an empty inner list). The v4
`nested read param` row of `examples/benchmark.j` measures the gap against its
`wrapped nested read` control.

#### M30.4 - statement results without a `Value` (was M28.23)

**Planned.** After M28.22, statement dispatch is the bulk of a tight loop: in
`primes`, `execStmt` / `execStmts` / `execStmtRaw` / `execBlock` / `execWhile`
hold 75% of the flat time on Go and 52% of the samples on TinyGo. The cause is
the same as M28.22's: every statement returns a `blockResult`, which embeds a
whole `Value` (the `return` value) next to its flow flags and position, so each
statement copies ~320 bytes up through four levels of calls even though almost
none of them return anything. Shrink `blockResult` to the flow flags and
position, and carry a `return` value out of band - on the function's call frame,
or through a pointer the call site passes down - so the common statement returns
a few words. The change is local to `interpreter.go` (15 functions return
`blockResult`; 4 sites read its value) but touches every statement executor, the
`defer` / `errdefer` teardown in `finishFrame`, `try` / `catch`, and `return`
from a `spawn` body. Measure with `examples/benchmark.j` (`primes`, the per-byte
scan, `fib`) on both builds; the existing tests cover the flow semantics.

#### M30.5 - move a parameter into a local on its last use (was M28.25)

**Planned.** The accumulator `func merge(doc as Document, f as Feature) { def out
as Document init $doc; $out.features[] = $f; return $out; }`, called as
`$doc = merge($doc, $f)` in a loop, is O(n^2): the local `def` deep-copies the
whole accumulator on every call. Measured inside a function: 2,000 calls 1.2 s,
8,000 calls 15.3 s, against 7 ms / 33 ms for mutating `$doc` directly (which the
existing `MoveArg` + `ReturnsOwned` path already makes O(1) per call). A GeoJSON
reader written this way took 8.6 s for 2,000 features. `L205` now flags the shape;
this makes it fast instead.

When `def x as T init $p` is the **last use** of parameter `p` in the method body,
move `p`'s value into `x` instead of copying it. Sound only when the frame owns
`p`'s value, so: the resolver treats such a parameter as written (it loses
`Param.Borrow`, so it is bound by copy, or by move when the caller's `MoveArg`
applies), and marks the `def` as a move (`DefineStmt.Move`, like
`VarExpr.Move` / `markMoveableAssign`). `execDefine` then takes the value without
`eagerCopy`. With `$v = merge($v, ...)` on a function local, the argument arrives
by move and the local takes it by move: zero copies per iteration. "Last use"
means no later read of `$p` on any path, the `def` not inside a `try` (the same
restriction `MoveArg` has) and not inside a loop body that could reach it again. Needs alias-stress cases
in `value_alias_test.go` (a moved-from parameter is never read; a borrowed
parameter is never moved) and the benchmark above as a regression check.

#### M30.6 - owned builtin results: no copy at the binding site (was M28.26)

**Planned.** A binding site (`def x init ...`, `$x = ...`) deep-copies its
right-hand side unless it can prove the value private: `rhsFreshLiteral`
(list / map / struct literals, ranges, slices) and `ownedCallResult` (user and
module functions marked `ReturnsOwned`). A builtin call matches neither, so a
result its Go function has just built, and nothing else holds, is copied again:

- `def ch as list of string init strings.chars($s)` on a 100,000-rune string:
  50 calls take 539 ms bound against 201 ms used directly. The same holds for
  `strings.split`, `convert.bytesFromString`, `fs.readBytes`, `maps.keys` and
  most other builtins that build their result; in the Deckyard decks these are
  60-80% of all copies `profile --allocs` reports.
- `binary.readFloats` of 1M values: 30 ms used inline, 92 ms bound with `def`,
  and twice the peak memory (`profile --allocs`: 259 MiB for the list, 287 MiB
  for `binary.split` of 16 MiB into 1M parts).
- Struct results: `def m as regex.Match init regex.find(RE, $s)` deep-copies
  the `Match` (its fields, the `groups` list and the `groupsNamed` map, about
  20 `Value` cells, 5.4 KiB). Over 30,000 matches of a five-group SemVer
  pattern that is 241 ms bound against 130 ms with `regex.find(...).start` in
  expression position, and 155.6 MiB of copies. It is the dominant cost in
  jvc's resolver (309 MiB and about a third of the wall clock over 80 decks x
  25 versions, through `semver.parse`, which binds its match this way).

The fix:

- **Registration:** a builtin opts in with an owned-result flag at
  registration (`RegisterNamespacedOwned` or an option on the existing call),
  mirrored by a marker on its `internal/libsig` line so the two cannot drift.
  `ownedCallResult` also accepts a `QualifiedCallExpr` stamped to such a
  builtin; both binding sites already consult it, so nothing else in the
  evaluator changes.
- **Audit per builtin:** a builtin may be marked only if its result never
  shares backing with an argument or interpreter state. Not marked until
  checked: anything that can return an argument or part of one (`lists.first`
  and `lists.last` hand back an element; a sort or merge may hand back its
  input when there is nothing to do), and getters returning stored values.
  Handle structs (`hash` / `crc` / `compress` streams, `fs.File`, `kv.Store`,
  ...) need no flag: they share state by design and are one small field to
  copy.
- **Candidates:** the measured builtins above; the list / map / bytes builders
  in `strings`, `lists` (`range`, `concat`, `slice`, `map`, `filter`,
  `sortBy`), `binary`, `convert`, `fs`, `encoding`, `compress` and `regex`;
  the `json` / `yaml` / `toml` decoders; and the struct builders: `regex.find`
  / `findAll`, `os.run` / `wait` (`os.Result`), `stats.describe`, `time`
  constructors and arithmetic, `fs.stat` / `walk`, `net.recvFrom`,
  `crypto.signKeypair` / `mlkemEncapsulate`, `archive.unpack` (a list of
  `archive.Entry`).
- **Tests:** each marked builtin gets an aliasing test that mutates the bound
  result and checks the argument (and the reverse), and fails with the copy
  removed, as `binary`'s does. The `strings.chars`, `binary.readFloats` and
  `regex.find` repros become `--allocs` regressions. A debug switch that
  copies anyway and compares is optional.
- **Order:** `binary` first (already audited and tested), then the decoders,
  `convert` and `regex` (jvc's resolver), then the rest by measured benefit
  (`profile --allocs` on the module and deck suites).

Not this milestone: copy-on-write or reference counting for compound values in
general (an M28-sized redesign of value semantics), and the module-boundary
rebuild of deck structs (M29.14). M28.35 was merged into this entry.

#### M30.7 - capture only the locals a `spawn` body reads (was M28.27)

**Planned.** `computeSpawnCaptures` narrows the **globals** a `spawn` snapshot
copies to those the body reads, but the **locals** frame is still copied
whole. A `spawn` inside a function therefore deep-copies every local in scope,
used or not: 200 spawns that use nothing, with a 5,000-entry map local in
scope, take 450 ms. In `@deckyard/ngs`, a worker loop that spawns from the
function holding the job list and the results gathered so far is quadratic
(1,000 jobs 1.42 s, 0.17 s once the spawn moves into a one-parameter helper).

Locals are simpler than globals: a function the body calls cannot see the
caller's locals, so the set is just the variable names the body (and any
nested `spawn`) reads, found by the same name-based walk; no call-graph
fixpoint is needed. `snapshotForSpawn` then copies only those bindings into
the locals frame. A body the walk cannot bound (an unrecognised node) falls
back to the whole frame, as for globals. Pin it in
`spawncapture_internal_test.go` and with a value-semantics test that a local
mutated after launch, used or not, never shows inside the task.

#### M30.8 - borrow through named callbacks (was M28.36)

**Planned.** In a program with a mutable top-level `def`, a parameter is
borrowed only in a function the M25.2 analysis stamps `GlobalSafe`, and a call to
any `jCallbackBuiltins` builtin (`lists.map`, `filter`, `reduce`, `find`, `any`,
`all`, `sortBy`, `meta.call`, `meta.callMain`, the `testing` runners) marks the
caller unsafe outright, as does every function that reaches it. So a function
that ranks with `lists.sortBy(xs, key)` copies every compound argument on every
call even when `key` is a named function that writes nothing and the argument is
a local. With one unused top-level `int` in the file, 200 calls of such a
function over a 2000-element `list of P` copy 726 MiB (1.0 s); without the line,
nothing (0.02 s). Decks hit it: `@deckyard/forensicgenetics` `search` copied its
whole profile database per call (64 MiB at 2000 profiles; on `jennifer-tiny` at
5000 profiles `search` went from 25.5 s to 14.1 s after switching to
`lists.sort` on plain keys), and `@deckyard/mitoleaf` reached `lists.sortBy`
through `lineage.mtHaplotype` and copied its tree per `distance` call (36 ms a
call, 10.5 ms without `sortBy`). Both decks now carry the workaround.

When the func-value argument of a callback builtin is a bare function name (a
`ConstRefExpr` resolving to a `MethodDef`), treat the builtin call as a named
call of that function in `globalHazardScan.qualifiedCall`: the caller is then
`GlobalSafe` exactly when that function is. `meta.call` / `callMain` (dispatch by
string) and func values held in variables, fields or parameters stay fail-closed.
A wider option to weigh alongside: an argument whose root is the caller's own
local (not itself a borrowed parameter) cannot alias a global, so it could be
borrowed whatever the callee reaches; that needs a "may alias a global" bit
carried through borrowed bindings. Pin it in `globalsafe_internal_test.go` and
`borrow_test.go`, including a named key that *does* write a global (must still
copy). The v4 `sortBy callee read` row of `examples/benchmark.j` measures it.

#### M30.9 - streaming HTTP responses and server-sent events (was M28.29)

**Planned.** `http` reads every response whole before returning it, so a
response that arrives over time cannot be consumed as it arrives. That blocks
server-sent events (SSE, `text/event-stream`), which the LLM decks (`openai`,
`anthropic`, `gemini`), Firebase's Realtime Database listeners and MCP's streamable
HTTP transport all stream over, and it also blocks any large download processed
as it comes in. Two layers, both pure `.j` over `net`:

- **`http` streaming responses.** `http.openStream(method, url, headers, body,
  options) -> http.Stream` sends the request, reads the status line and headers,
  and returns with the body unread: `status` and `headers` are on the handle, so
  a caller checks for a non-2xx before reading. `http.readStream(stream) ->
  http.StreamRead{data as bytes, done as bool, stream as Stream}` returns the next
  piece of the body (one chunk under chunked framing, up to a buffer size under
  `Content-Length` or close-delimited framing), and `http.closeStream` ends it
  early. The handle threads its state forward the way `exchange` threads a
  `Session` (`$s = $r.stream;`): the connection, the framing mode, the bytes
  still due, and any body bytes over-read with the headers. Takes the existing
  `http.Options` (TLS, the cap on the total body, and the per-read idle
  timeout, which is what bounds a stalled stream). Redirects are not followed
  (as with `exchange`); a 3xx comes back like any status.
- **An `sse` module** for the event-stream format (WHATWG HTML, "Server-sent
  events"): `sse.open(method, url, headers, body, options) -> sse.Stream` over
  `http.openStream` (sends `Accept: text/event-stream`, rejects another
  content type), `sse.next(stream) -> sse.Read{event as Event, done as bool,
  stream as Stream}`, `sse.close`, and `sse.Event{event, data, id, retry}`. The
  parser follows the spec exactly: `CR`, `LF` and `CRLF` line ends, a leading
  BOM dropped, `:` comment lines (keep-alives) skipped, multi-line `data` joined
  with `\n`, an event dispatched on a blank line, `event` defaulting to
  `"message"`, `retry` taken only when all digits, and the last `id` kept.
  `sse.parse(text) -> list of Event` covers a body already read whole, and
  `sse.format(event) -> string` serializes one for a server to send. A method
  and body are parameters because the LLM APIs stream from a `POST` with a JSON
  body. Reconnection stays with the caller (`sse.lastEventId` gives the value
  for a `Last-Event-ID` header): EventSource-style automatic reconnect only fits
  `GET` streams and would hide failures.

Stopping a stream from another task works by closing it: a read blocked on the
socket fails with `Error{kind: "closed"}`, while `task.cancel` cannot interrupt
a blocked read. Default binary only, like `http`. Serving a stream is a
separate step: `httpd` answers each request with one complete body, so a
streaming `respond` would be an `httpd` addition of its own; `sse.format` is
ready for it. Tests run against a fake server written in `.j` on `net.listen`
that writes chunked event frames with pauses between them, covering a frame
split across reads, a CRLF split between two chunks, keep-alive comments, an
idle-timeout breach and early close. A new module, so it ships with the full
module set: `modules/sse_test.j`, `docs/modules/sse.md`, an index row, the
`SUMMARY.md` entry, `examples/modules/sse_demo.j`, `modules/README.md` and a
`JENNIFER.md` bullet. Additive.

`sse` is a core module rather than an `@jennifer/` deck because core needs
it: the `mcp` module is stateless today ("no SSE, no sessions"), and MCP's
streamable HTTP transport, the follow-up this unblocks, sends server-to-client
messages over SSE. A core module cannot import a deck. Adding that transport
to `mcp` (the client reading a streamed reply, plus the session handling the
transport defines) is a separate step once this lands; `docs/modules/mcp.md`
points here.

#### M30.10 - streaming request bodies (was M28.67)

**Planned.** `http` takes a request body as a whole `string` or `bytes`, so
an upload holds the file in memory, and in practice about three times its
size (the caller's buffer, the framed request, the write). Found in
`@deckyard/gemini`'s Files API upload (a 200 MB file peaked at 609 MB RSS)
and relevant to `webdav`, `s3`, the `download` deck's peers and any
multipart upload.

- `http.requestFromFile(method, url, headers, path, options) ->
  Response`: the body is read from `path` in chunks and written to the
  socket as it goes, with `Content-Length` from the file size (no chunked
  encoding, which many upload endpoints refuse). The file is checked to be
  a regular file and not to change size while sent (an error if it does).
- `http.requestFromParts(method, url, headers, parts, options)` for bodies
  built from pieces (`list of BodyPart{data as bytes, path as string}`),
  which covers multipart uploads without concatenating a file into memory;
  `multipart.buildParts` returns such a list.
- Redirects stay unfollowed for these (a streamed body cannot be replayed
  without reading the file again; documented).
- Pairs with M30.9 (streaming responses), which is the download half.
  Default binary only, like `http`. Tested with a 1 GiB file against a
  loopback `httpd` and a peak-RSS bound in CI.

#### M30.11 - streaming `xml` decode (was M28.4)

**Planned.** `xml.decode` builds a full DOM up front - every element a `KindObject`
wrapping a `{tag, attrs, children}` map, every node / attr / text a 280-byte
interpreter `Value` - so the tree runs ~150x the source and is all live at once: a
10 MB GPX measures ~1.5 GB RSS, and 3 copies do not fit in 4 GB. Reported twice
(the geo deck wrote a ~900-line hand scanner to avoid it); the whole XML family is a
long flat list of small records, the exact shape a pull parser handles in constant
memory. Keep `xml.decode` for small documents; add a streaming reader for large ones.

- **Primitive (pull loop, no callbacks - the M23.1 shape).**
  `xml.reader(bytes) -> xml.Reader` (operates on `bytes`, so a caller that scanned a
  buffer hands it over without a string copy, leaning on the no-copy `bytes`-arg
  guarantee), then `xml.next(reader) -> xml.Event` yielding start / text / end / eof.
  Memory is O(depth) + the source, not O(document).
- **`xml.Event`** is small (`kind`, `tag`, `text`, `depth`); attributes read through
  the existing `xml.attr` / `xml.attrs` accessors (no new vocabulary).
- **Subtree iterator (the ergonomic layer):** `xml.nextElement(reader, name) ->
  xml.Value` yields one small per-element DOM at a time (the "iterparse" shape), so
  `for each trkpt` holds O(one trackpoint), reusing every existing accessor.
- **Local-name matching.** The reader matches `trkpt` / `lat` on a `gpx:`-prefixed
  real-world file (today `xml.attr` *raises* on `gpx:lat`), closing the namespace
  pain the geo report also hit.
- **Handle** is an integer-registry entry (the `net.Conn` / `fs.File` pattern), closed
  with the polymorphic `close`; hand-rolled over the existing position-based scanner,
  so TinyGo-clean. Malformed markup is a positioned catchable error.

**Requires:** M20.2 (`xml` library). Follow-on: the same DOM blowup is shared by
`json` / `toml` / `yaml` (all 280-byte-`Value` trees); a streaming decode there is a
later generalization, not part of this.

#### M30.12 - `cbor` library (was M28.30)

**Planned.** CBOR (RFC 8949) as a system library, designed like `json` /
`toml` / `yaml`: `cbor.decode(bytes) -> cbor.Value` and `cbor.encode(v) ->
bytes` over the shared `internal/lib/doctree` tree, which brings the whole
read / walk / write surface (`typeOf` with `"missing"`, `get` / `has` / `keys`
/ `length` / `as*`, `isNull`, `isDatetime` / `asDatetime`, `map` / `list` +
`set` / `insert` / `append` / `remove` / `move`) addressed by JSON Pointer. A
core library, not a deck: a general, stable IETF format, and the one WebAuthn /
passkeys, COSE (RFC 9052) and CWT are built on, so core auth code may need it.
Go rather than a `.j` module, because a byte-by-byte decoder in `.j` is the
slow path the copy reports measured. Hand-rolled with no dependency (the common
Go CBOR packages are reflection-based), TinyGo-clean, both binaries.

Mapping onto the tree, strict at the boundary:

- **Integer map keys.** The tree's keys are strings, but CBOR maps (COSE keys
  most of all: `{1: 2, 3: -7, -1: 1}`) use integer keys. The tree records a
  key kind per member (string or int, in a parallel slice that only CBOR and
  MessagePack trees allocate, so `json` / `toml` / `yaml` pay nothing); an int
  key is addressed by its decimal pointer token (`"/3"`), `keys` lists it in
  decimal, and `encode` writes it back as an integer. `cbor.keyKinds(v[, ptr])
  -> list of string` reports `"string"` / `"int"` per member, and
  `cbor.setIntKey(v, ptr, key, value)` adds an int-keyed member (the shared
  `set` creates string keys). A map holding both `1` and `"1"`, or a key of
  any other type, is a decode error.
- **Integers** outside int64 (major type 0 above 2^63 - 1, major type 1 below
  -2^63) are an error, as are bignum tags 2 / 3 whose value does not fit int64.
- **Floats:** half, single and double precision decode to `float`; `encode`
  picks the shortest width that keeps the value exactly. A non-finite float
  decodes to a node `typeOf` reports as `float` but `asFloat` rejects, as
  `toml` / `yaml` already do.
- **Byte strings** become the tree's `bytes` kind; text strings must be valid
  UTF-8.
- **Tags:** 0 (RFC 3339 text) and 1 (epoch seconds) become `datetime`; any
  other tag is a decode error in v1, naming the tag number.
- **Simple values:** `false` / `true` / `null` map; `undefined` and the other
  simple values are an error.
- **Indefinite lengths** are decoded; `encode` always writes definite lengths.
- **`encode` is deterministic** (RFC 8949 section 4.2.1 core deterministic
  encoding: shortest forms, map keys sorted by their encoded bytes), which is
  what COSE signs and verifies against, and makes equal values encode to equal
  bytes.
- **Decode limits** like `asn1`'s: a node budget and a nesting cap, and a
  declared length is checked against the remaining input before anything is
  allocated, so a hostile length field is a catchable error, not an
  out-of-memory.

Tested against the RFC 8949 Appendix A examples (both directions) plus
round-trip, deterministic-order and limit cases. A new library, so the usual
set: `docs/libraries/cbor.md`, the library index and cheatsheet rows, an
`internal/libsig/cbor.go` line per builtin, `user-guide/imports.md` and
`technical/interpreter.md` links, the catalog in the spec and a `JENNIFER.md`
bullet. Additive.

#### M30.13 - `msgpack` library (was M28.31)

**Planned.** MessagePack as a system library on the same tree, after `cbor`
(which lands the int-key support it reuses): `msgpack.decode(bytes) ->
msgpack.Value`, `msgpack.encode(v) -> bytes`, the shared surface, plus
`keyKinds` / `setIntKey` as in `cbor`. Used by Neovim's RPC, fluentd and
MessagePack-RPC. A separate library from `cbor`, as `json` / `toml` / `yaml`
are separate: two formats with their own wire rules. The same boundary rules:
`uint 64` above int64 is an error; `float 32` / `float 64` decode to `float`
and `encode` uses `float 32` only when exact; `bin` becomes `bytes` and `str`
must be valid UTF-8; the timestamp extension (type -1, all three widths)
becomes `datetime`, and any other extension type is a decode error naming it;
integer keys as in `cbor`, other key types an error. `encode` picks the
smallest format for each value, and keeps map order as given (MessagePack
defines no canonical order). Same decode limits as `cbor`. Tested against the
spec's format table in both directions plus round-trip and limit cases. The
same new-library set as `cbor`. Additive.

#### M30.14 - `protobuf` wire library (was M28.61)

**Planned.** Protocol Buffers' binary wire format as a system library, the
layer every protobuf user needs and the one that must be fast. Wanted by the
Hoymiles DTU and ESPHome native API (device decks), Prometheus remote write,
binary OTLP, Meshtastic, and the descriptor-driven `protobuf` deck (M27.7 in [decks.md](decks.md));
gRPC is a separate, later question (HTTP/2, see `horizon.md`). Today nothing
reads it: `binary` has no varint, and `jennifer profile` hand-encodes its
pprof output in Go. Go rather than `.j`: a byte-by-byte varint loop in `.j` is
the slow path, and the codec is small. Hand-rolled with no dependency (not
`google.golang.org/protobuf`, which is reflection- and codegen-based and
large), TinyGo-clean, both binaries.

Schema-free, by field number:

- `protobuf.decode(data) -> list of Field` with `Field{number as int, wire as
  string, value as int, data as bytes}`: one entry per field occurrence, in
  wire order (repeated fields and unknown fields stay visible). `wire` is
  `"varint"`, `"fixed64"`, `"bytes"` (length-delimited) or `"fixed32"`; the
  value is in `value` (varint, fixed32/64 raw bits) or `data`. Groups (wire
  types 3 / 4, deprecated since proto2) are an error naming the field.
- `protobuf.encode(fields) -> bytes` writes them back in the given order;
  `decode(encode(f)) == f`.
- Typed readers and makers for the scalar mappings, so callers never
  bit-twiddle: `asSint` (zigzag), `asUint64` (refuses values above int64:
  Jennifer has no unsigned 64-bit), `asBool`, `asFloat` (fixed32 bits),
  `asDouble`, `asString` (valid UTF-8 or an error), `asPacked(field, kind) ->
  list of int` / `list of float` for packed repeated scalars; and
  `varintField(n, v)`, `sintField`, `fixed32Field`, `fixed64Field`,
  `floatField`, `doubleField`, `boolField`, `stringField`, `bytesField`,
  `messageField(n, fields)`, `packedField(n, kind, values)` to build
  `Field`s (one `*Field` suffix, so none is named after a type keyword).
- `protobuf.find(fields, number) -> list of Field` and `first(fields,
  number)` for the usual lookup; a nested message is `decode(field.data)`.
- Varint helpers for formats that frame with them outside protobuf
  (`varintBytes(v) -> bytes`, `readVarint(data, offset) -> VarintRead{value,
  next}`), and length-delimited message streams (`readDelimited(data, offset)`,
  `delimited(message)`), which ESPHome and many socket protocols use.

Strict at the boundary, with decode limits like `cbor`'s: a varint longer
than 10 bytes, a field number of 0 or above 2^29 - 1, a truncated value, and a
length past the end of the input are `"protobuf"` errors; a length is checked
against the remaining input before anything is allocated; a total field-count
budget and the caller's input size bound the work, and exceeding the budget
is kind `"limit"`. Nesting is the caller's (each `decode` is one level), so
there is no recursion to bound here. Errors are one `ProtobufError{kind,
message, offset}` (the byte offset of the failing field) per the error rules.

Tested against byte strings produced by `protoc --encode` for every scalar
type, packed and unpacked repeated fields, negative int32 / int64 (10-byte
varints), zigzag extremes, unknown fields and the limit cases, plus
round-trips. The usual new-library set: `docs/libraries/protobuf.md`, the
library index and cheatsheet rows, an `internal/libsig/protobuf.go` line per
builtin, the catalog in the spec and a `JENNIFER.md` bullet. `jennifer
profile` may then encode its pprof output through the same codec. Additive.

#### M30.15 - `net` Unix domain sockets and file-descriptor passing (was M28.33)

**Planned.** `net` speaks TCP and UDP only; Unix domain sockets are on its
"left out" list, and only `httpd` can listen on one. That keeps every local
service behind a socket file out of reach: the Docker / Podman API
(`/var/run/docker.sock`), D-Bus, systemd's `sd_notify` and socket activation,
`ssh-agent`, and a Wayland compositor. Several of those also hand open file
descriptors across the socket (`SCM_RIGHTS`), which no Jennifer program can
send or receive today. Two layers, both in the `net` library (Go):

- **Unix sockets on the existing handles.** The `unix:` address prefix
  `httpd` already uses selects a stream socket: `net.connect("unix:/path")`
  returns a `net.Conn` and `net.listen("unix:/path")` a `net.Listener`, so
  `readBytes` / `readN` / `readAll` / `writeBytes`, the deadlines, `eof`,
  `accept` and `close` work unchanged. `unix:@name` is the Linux abstract
  namespace. `listen` follows `httpd`'s rules: the socket is created `0660`
  regardless of umask, a stale socket file is cleared only after confirming
  nothing listens on it, and `close` on the listener unlinks the file it
  created. Datagrams use `unixgram:` with the UDP surface: `net.listenUDP`
  binds (`"unixgram:"` alone is an unbound sender), and `net.sendTo` /
  `net.recvFrom` take a `unixgram:` peer, which is what `sd_notify` needs.
  `net.address` returns the path. `net.peerCred($conn) -> net.Cred{pid, uid,
  gid}` reads `SO_PEERCRED` so a server can check who connected.
- **File descriptors.** A new handle `net.Fd{id as int}`, shared between
  copies like `net.Conn`, holds one open descriptor. `net.fdOf(handle)`
  duplicates the descriptor behind an `fs.File`, `net.Conn`, `net.Listener` or
  `net.UDPSocket` (the original stays open). `net.writeWithFds($conn, b, fds as
  list of net.Fd)` sends bytes plus descriptors in one message;
  `net.readWithFds($conn, n) -> net.FdRead{data as bytes, fds as list of
  net.Fd}` receives them, with every received descriptor close-on-exec and
  registered as a handle. `net.fdToFile(fd, mode) -> fs.File`,
  `net.fdToConn(fd) -> net.Conn` and `net.fdToListener(fd) -> net.Listener`
  adopt one into the usual surface, and `net.closeFd` releases it.
  `net.inheritedFd(n) -> net.Fd` adopts descriptor `3 + n` for socket
  activation (checking `LISTEN_PID` against the process). Limits: at most 253
  descriptors per message (the kernel's `SCM_MAX_FD`), an error before
  sending; a message that arrives with more descriptors than fit, or on a
  plain `readBytes`, has them closed by the kernel, never leaked into the
  process; a truncated control message is a catchable error.

Linux first, other Unixes where Go's `syscall` has the same calls; on Windows
`AF_UNIX` stream sockets work but descriptor passing is a catchable error.
`jennifer-tiny` stubs both like the rest of `net`. Needs the `net` capability,
as today. Tests: a `.j` program listens on a socket in a temp dir, a spawned
task connects, and the pair exchanges bytes, a datagram over `unixgram:`, a
temp file's descriptor (read back through `fdToFile`) and a listener's
descriptor (accepted on through `fdToListener`); plus the stale-socket,
permission, `SCM_MAX_FD` and abstract-namespace cases. Docs: a "Unix domain
sockets" and a "Passing file descriptors" section in `docs/libraries/net.md`
(dropping the "left out" line), `internal/libsig/net.go` lines, cheatsheet
rows and a `JENNIFER.md` bullet. Additive.

Out of scope, each a step of its own once this lands: `http` over a Unix
socket (an `http.Options` socket path, for the Docker API), a `dbus` module,
and shared memory (`memfd_create` plus a mapped buffer), which a Wayland
client needs for its pixel buffers on top of descriptor passing.

#### M30.16 - runtime `.j` module loader (`plugin` library) (was M28.1)

**Planned.** Load a `.j` module from a path computed at run time and dispatch into
it, so a plugin needs no `os.run` + stdio bridge - the three current loaders resolve
before execution from a string literal (`use` compiles a library in; `include`
splices tokens in preproc; `import` resolves at `Run()` start), so no
runtime-chosen module can be loaded today. This is a system library over the
existing M17 `loadModule`, not a new keyword: `include` / `import` are compile /
load-time (token splice, parse-time type stamping, pre-execution resolution),
whereas a runtime load is side-effecting and takes the handle-returning shape of
`fs.open` / `kv.open`.

- **Surface.** `plugin.load(path) -> plugin.Module` (resolve + run once, cached);
  `plugin.func(m, name) -> func`; `plugin.call(m, name, args...)`;
  `plugin.const(m, name)`; `plugin.has(m, name)`;
  `plugin.exports(m) -> list of string`.
- **Dispatch.** `plugin.func` returns a `func` value whose `FnHome` runs the body
  in the module's context with argument / return struct retagging; `plugin.call`
  is the by-name wrapper (`meta.call` path).
- **Reuse.** `loadModule` supplies run-once caching, the `@vendor` / search-path
  sandbox (`module.Resolve`), and the M24.20 version / capability pragma gates
  unchanged (a `net` plugin fails to load on `jennifer-tiny`). Run-once, no unload
  or reload.
- **Interpreter deltas.** Expose `loadModule` through `BuiltinCtx`; skip the
  static-import compile-time steps (`bindModuleAlias`, `resolveDeclaredTypesOnce`).
  Add a mutex to the module registry (`cache` / `stack` / `resolved`) - lock-free
  today, correct only because imports load single-threaded before any `spawn`,
  which a runtime load breaks.
- **Type boundary.** A plugin's struct types are unknown at parse time, so the host
  cannot write `def x as plugin.T`. Contract: a shared interface module, imported
  normally, declares the struct types and the plugin implements funcs over them
  (values retag at the boundary); primitives, collections, and opaque handles cross
  without a shared declaration.
- **Trust.** Loading `.j` runs code; no new sandbox (untrusted code stays
  `DRAFT#11`), and path resolution is the `import` sandbox.

**Requires:** M17 (`loadModule`), M19.7 (`@vendor` resolver), M24.20
(version / capability pragma), `FnHome` func values. Open: handle as an integer
registry (`kv.Store` pattern, survives `spawn` / boundaries) vs `KindObject`;
runtime load inside a `spawn` vs main-goroutine-only; a stricter `plugin.load` path
allowlist than `import`.

#### M30.17 - host re-entry from spawned tasks (was M28.20)

**Planned.** A spawned task that calls a module function which calls
`meta.callMain` runs the host function against the live host globals while the
main program keeps running: a data race the race detector reports, with or
without borrow. `hostMu` serializes `callMain` callers against each other (which
is why concurrent `web` handlers are safe, the main program being parked in
`web.serveOn`), but not against the main program. Options: hold `hostMu` while
the main program runs statements whenever host re-entry from a spawn is possible
(serializing it with those tasks), give each such task a host snapshot like a
plain spawn (changing what `callMain` from a task can observe and mutate), or
make it a documented rule that `callMain` from a task is only valid while the
main program is blocked. Needs a decision, then the matching tests under `-race`.

#### M30.19 - `strings.graphemes`: user-perceived characters

**Planned.** Strings index by code point, so `strings.chars("e\u0301")` is two
elements and reversing or truncating a string can split an accent from its
letter, a flag into two regional indicators, or a ZWJ emoji family into its
people. `bytes` is the finer level, not the coarser one: "é" written as `e` +
U+0301 is 3 bytes, 2 runes and 1 grapheme, and 👍🏽 is 8 bytes, 2 runes and 1
grapheme. Combining marks alone can be kept today with
`regex.findAll('\P{M}\p{M}*', s)` (Rosetta Code's `Reverse a string` does), but
that is not a grapheme cluster: it splits emoji sequences, flags and
Hangul syllables built from jamo.

- **Add** `strings.graphemes(s) -> list of string`: the extended grapheme
  clusters of Unicode UAX #29, in order. With it, `len(strings.graphemes(s))`
  is the user-perceived length, and reversing or cutting a list of clusters
  keeps each one whole.
- The break rules need the `Grapheme_Cluster_Break` and
  `Extended_Pictographic` properties, which Go's `unicode` package does not
  carry. A table generated from the Unicode data files (pinned to one Unicode
  version, regenerated by a `go generate` step) rather than a new module
  dependency. Weigh the table's size on `jennifer-tiny`.
- Tests: the Unicode `GraphemeBreakTest.txt` cases for the pinned version.
- Docs: `docs/libraries/strings.md`, JENNIFER.md's `strings` line,
  `internal/libsig/strings.go`.

Title case, capitalizing and swapping case stay out of `strings`: they change
the value by rules (small words, locale, apostrophes) rather than being
primitives, and belong with `@mplx/texttransform`'s transforms.

**Requires:** none.

#### M30.20 - library and module functions as values

**Planned.** A bare user function name is a `func` value (`lists.map($xs,
double)`), but a library or module function is not: `math.sin` as a value is
"math.sin is a function; call it as math.sin(...)", and `semver.isValid` is
"module "semver" has no constant "isValid"". Both parse as a qualified
constant read. Rosetta Code's `Function composition` wraps `math.sin` and
`math.asin` in one-line functions to pass them; a deck's exported function
cannot be handed to `lists.map` or `lists.filter` without the same wrapper.

- **A qualified name not followed by `(`** that names a library builtin
  (`math.sin`) or an exported module function (`semver.isValid`,
  `texttransform.t`) is a `func` value. The resolver decides it where it now
  reports the error: a name that is a function becomes a function value, an
  unknown name stays an error.
- A module function value runs in its module, as a module-handed `func`
  value already does (`FnHome`), so its structs keep their identity.
- A builtin value checks its arguments when called, as a direct call does;
  a variadic builtin (`io.printf`) is a value too, called with any count.
- Value semantics are unchanged: a `func` is immutable, and two values of
  the same function compare equal.
- Docs: the `func` paragraph of JENNIFER.md ("a bare function name ... is
  the value"), `docs/user-guide/functions.md`, the error-message
  change in CHANGELOG.
- Tests: `math.sin` / `strings.upper` / a module function through
  `lists.map`, stored in a list and called, passed across a module
  boundary; `$f == math.sin`; a misspelt name still an error at load.

**Requires:** none (`M24.2`'s `func` values).

#### M30.21 - declared return types

**Planned.** Parameters and `def`s are typed; a function's result is not. The
caller's `def` checks it, so a wrong result is reported in the caller's file,
not at the `return` that produced it, and nothing before the call knows what
comes back:

- `jennifer lint`'s call checks (M28.9: L110 argument type, L111 binding
  type) skip every call to a user function;
- `match` cannot destructure a call result (M29.28 keeps it refused);
- typed `func` values (one of M29.16's options) and generic functions
  (DRAFT#33) have no way to state the result;
- the bundled modules state 1,228 return types in `@return {type}` tags that
  nothing checks.

- **Add** an optional return type, spelled as everything else is typed:
  `func parse(s as string) as Tree { ... }`. `as null` declares a function
  that returns nothing; a `return EXPR;` with a value is then an error at
  load.
- The interpreter checks the value at every `return` (and at the end of a
  body that falls off it, which returns `null`) with the check a `def`
  does; an error names the function, the declared and the actual type, at
  the `return`.
- `jennifer lint`: L110 / L111 cover calls to a function with a declared
  type; a new rule reports an exported module function without one, so the
  public surface of modules and decks is typed first; another one checks a
  docblock's `@return {type}` against the declaration. `jennifer fmt` keeps
  the type on the `func` line.
- Docgen (grimoire, `gen-module-docs.j`) takes the type from the signature
  when there is one.
- Type the bundled modules' exported functions in this release, from their
  `@return` tags.
- One type only: several values still travel in a struct (multiple returns
  stay rejected, [rejected.md](technical/rejected.md)).
- **Optional for now.** Whether a return type becomes required is decided at
  the 1.0.0 cut, once the modules and decks are typed: required is a
  breaking change for every untyped function.
- Docs: JENNIFER.md's Functions section, `docs/user-guide/functions.md`,
  the grammar files (`docs/technical/grammar_ebnf.md`, `grammar_peg.md`),
  `docs/technical/cli_lint.md`.
- Tests: each `return` path checked, the fall-off `null`, `as null` with a
  value refused at load, a module function's struct return type across the
  boundary, recursion, a `func` value of a typed function, lint on typed and
  untyped calls.

**Requires:** none. Unblocks M29.16's typed-`func` option and call results
in M29.28; DRAFT#33 uses the same spelling for `as T`.

#### M30.22 - a sampling profiler

**Planned.** The statement profile times every statement, and the timing is
part of the cost: interpreted code looks far more expensive than time spent in
Go builtins. On the registry (2026-11-21) it put routing at about 60% of a
request; removing most of that routing made the unprofiled server 2-45%
faster. A profile should rank by what the program really spends.

- **`jennifer profile --sample`**: no per-statement timing; every few
  milliseconds (default 10, `--rate`) the Jennifer call stack of each running
  task is recorded. Report the share of samples per function and per line,
  with the same table, `--by`, `--top` and pprof output as the statement
  profile (M29.31), and waits apart (M29.32).
- Overhead target: under 5% on the benchmark suite, so a profiled server
  behaves like an unprofiled one.
- The call stack the interpreter already keeps for the SIGUSR1 dump and error
  positions is the source; no instrumentation of the tree walk.
- `jennifer-tiny`: with the `tinygo_devtools` tag, if the scheduler allows a
  sampler; otherwise documented as default-build only.
- Docs and tests: `cli_profile.md` (when to use which); a CPU-bound loop and a
  builtin-bound loop each attributed within a few percent of their measured
  share.

**Requires:** M29.31 (table and `--by`); M29.32 for waits.

#### M30.23 - `jennifer debug`: breakpoints and single-stepping

**Planned.** There is no way to stop a program and look at it: `repl`,
`tokens`, `ast` and `profile --format=trace` show code or timing, not state.
The interpreter already hooks every statement and call for the profiler,
which is where a debugger stops.

- **`jennifer debug prog.j [args]`**, a terminal debugger: breakpoints by
  `file:line` and by function, `continue`, `step` (into calls), `next` (over),
  `finish`; `print EXPR` evaluates any expression in the current frame with
  the REPL's evaluator; `locals`, `stack`, `tasks` (the running `spawn`s) and
  `catch` (stop where an error is thrown, optionally by kind).
- Value semantics make inspection simple: a printed value is the value, there
  is no alias to explain. `print` never changes state; assignment from the
  debugger is left out at first.
- Concurrency: a breakpoint pauses the task that hits it; `tasks` lists the
  others, which keep running unless `pause all` (to decide when implementing:
  stopping every goroutine at a checkpoint is what the SIGUSR1 dump and
  cancellation already do).
- Cost without breakpoints: one branch per statement, like the profiler's
  hook when off; measured on the benchmark suite.
- Modules and decks: breakpoints by module path; system modules shown as
  `<sys>/web.j`.
- `jennifer-tiny`: with the `tinygo_devtools` tag only.
- Docs: a "Debugging" page in the user guide, `docs/technical/` for the engine.
- Tests: a scripted session (commands on stdin) per command; breakpoints in
  a module, in a `spawn` body, on a throw.

**Requires:** none. The engine is what DRAFT#35 (Debug Adapter Protocol)
builds on.

#### M30.24 - `internal/runtime`: the value layer below the evaluator (was M29.12, M28.39)

**Planned.** The libraries are welded to the evaluator: the 102 files in
`internal/lib` that import `internal/interpreter` use about 40 of its
symbols (`Value`, `Null`, `BuiltinCtx` and the constructors far ahead), but
importing them pulls in the whole tree-walker. Move what they use into
`internal/runtime`, with the evaluator above it. This is step 1 of
`DRAFT#1` in [horizon.md](horizon.md): internal only, no public API.

- **Moves to `internal/runtime`:** `Value`, its kinds and constructors,
  equality, deep copy and display (most of `value.go`); errors and
  positions (`RaiseError`, `KindedError`, `ClassifyError`, `ErrorSignal`);
  the builtin registry; task / channel / waker state; `BuiltinCtx`.
- **`Host` interface:** the calls a builtin makes back into the engine
  (`Invoke`, `CallByName`, `Cancel`, `MethodParams`, the call-depth chain)
  go through an interface the evaluator implements. Only callbacks use it;
  `Value` operations stay concrete calls.
- **Types out of the parser:** `Value.ElemTyp` / `KeyTyp` / `ValTyp` are
  `*parser.Type`, and the libraries call `parser.PrimitiveType` 174 times.
  The type descriptors move to a package below both the parser and the
  runtime.
- **Opaque function values:** `Value.Fn` (`*parser.MethodDef`) and
  `Value.FnHome` (`*Interpreter`) become one function reference the engine
  fills in, so the runtime holds no AST.
- **Staged:** type aliases at the old names (`interpreter.Value =
  runtime.Value`, ...) keep every package building while the libraries
  move over, one commit per library group; remove the aliases at the end.
- **Checks:** no behaviour change, so the existing suite is the oracle:
  `go test ./...`, the module overlays, the example goldens and the TinyGo
  goldens. `examples/benchmark.j --suite all` on the reference machine shows
  no regression; a `Host` call on a hot path would show up there.
  `go list -deps ./internal/lib/...` no longer lists
  `internal/interpreter`.

Gains: library tests run without `interpreter.New()`; `DRAFT#17` (a VM) and
compile-to-Go each become another `Host` instead of a change inside the
evaluator.

Not this milestone: the public `pkg/` facade (`DRAFT#1` step 2, after
`Value` is settled), any change to `Value`'s layout or semantics.

**Requires:** M30.3, M30.5, M30.6, M29.14, M30.8, M28.37, which reshape
`Value` and the evaluator; land them first so the move does not conflict
with them. Moved from M29 (2026-11-23): four of them are M30 entries, so it
cannot land in 0.28.0.

## M31 - multiplatform: promote macOS / Windows to supported

Linux is the only *supported* platform, but best-effort **unsupported** macOS /
Windows binaries (the standard-Go `jennifer`, via cross-compile) already ship each
release - so the work is not "add the ports" but **promoting them to supported**,
which graduates the "cross-build for macOS / Windows" 1.0.0 distribution
requirement. A portability audit found the surface small: separators / EOL /
`$HOME` / temp are already `runtime.GOOS`-derived (`internal/lib/os/oslib.go`),
`os/exec` keys on `runtime.Compiler != "tinygo"` (not GOOS, so it is enabled on
Windows), and signals + the four Linux-only hardware libs (`serial` / `spi` /
`i2c` / `gpio`) stub cleanly on non-Linux (`*_other.go`). **Extra distribution
packaging** (a Homebrew tap, Snap, Nix flake, Flatpak / AppImage) stays a
per-format nice-to-have, shipped only when a user asks and a maintainer keeps it
green - none blocks a release. Flatpak's sandbox suits desktop apps, not a
CLI that reads host files and runs host commands, and AppImage bundles shared
libraries a static binary does not have; the "unpack anywhere and run" case is
the exe-relative module directory in `M31.1`.

### M31.1 - Windows: promote to supported

**Planned.** The **Windows** track is a handful of concrete gaps, not a rewrite:

- **Exe-relative module directory, on every platform.** `compileDefaultSysmoddir`
  bakes one POSIX path (`/usr/share/jennifer/modules`,
  `internal/module/sysmoddir.go`) into every binary, so the modules a release
  archive ships are found only through `JENNIFER_SYSMODDIR`: an unpacked Linux
  or macOS tarball runs, but `import "name.j";` fails, and on Windows the path is
  meaningless. Add a layer that looks next to the binary, resolved from
  `os.Executable()` through `filepath.EvalSymlinks` (a Homebrew or `/usr/local/bin`
  symlink points into the real prefix):
  - `<exe dir>/share/jennifer/modules`, the release archive layout (`jennifer`
    and `share/` side by side);
  - `<exe dir>/../share/jennifer/modules`, a prefix install (`/usr/local`,
    `/opt/jennifer`, Homebrew).

  Precedence: `--sysmoddir` > `JENNIFER_SYSMODDIR` > exe-relative (the first
  directory that exists) > the compile-time default. The binary's own modules
  come before `/usr/share`, so a newer archive unpacked on a host with an older
  distro package does not load the older modules; for a distro install the
  `../share` candidate is `/usr/share/jennifer/modules` anyway. Report the layer
  as a new `Source` (`exe`) in `meta` and `jennifer version -v`. The `M21.13`
  installer's `JENNIFER_SYSMODDIR` stays the explicit override. `jennifer-tiny`
  takes the same layer where TinyGo provides `os.Executable`, else keeps the
  compile-time default. Tests: a relocated archive (binary and `share/` copied to
  a temp directory) resolves its modules with no env var; a symlinked binary
  resolves through the link; an env var still wins. This makes AppImage
  unnecessary for the "unpack anywhere" case.
- **Per-OS golden strategy.** `examples/expected/osinfo.txt` is the sole
  platform-pinned golden (pins `linux` / `amd64` / `/` / `:`, compared byte-exact in
  `cmd/jennifer/examples_test.go`); add per-OS expected-file selection or a
  `runtime.GOOS`-gated skip for the osinfo canary (already flagged at
  `examples/osinfo.j`).
- **`fs.chmod` / `fs.chown` on Windows.** Define the Windows behaviour (a friendly
  catchable error is acceptable; the `chown` test is already Linux-gated), and
  document that signal-based graceful shutdown is limited on Windows
  (`signal_other.go` stubs `os.catchSignal`).
- **A `windows-latest` CI test job** running `go test ./...` so Windows correctness
  is verified (the exec suite self-skips off Linux; the osinfo golden is
  the known failure the item above resolves); once green, move windows/amd64 out of
  the `build-unsupported` matrix into the supported set and drop the "unsupported"
  labelling for that arch.

**Requires:** none.

### M31.2 - macOS: promote to supported

**Planned.** The parallel case: the same "already ships unsupported, promote it"
shape as `M31.1`, and simpler: the exe-relative module directory from `M31.1`
covers the tarball and Homebrew layouts, and its separators / EOL / `$HOME`
match Linux. A `macos-latest` CI test job running `go test ./...` verifies
correctness (reusing the per-OS osinfo golden strategy from `M31.1`); once green,
move darwin/amd64 + darwin/arm64 out of the `build-unsupported` matrix into the
supported set and drop the "unsupported" labelling. **Requires:** none.

---

## M32 - project governance, licensing, and contribution policy

**Planned. A hard requirement for 1.0.0 stable** (also listed under
[Requirements for 1.0.0 stable](#requirements-for-100-stable)). The rules for
*how the project is run and how outside contributions are taken* - organizational,
not code.
It must be settled
**before the first external contribution is merged**: several of the choices are
hard to reverse once other people's copyrightable work is in the tree. The open
questions, roughly by urgency:

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
decision above changes it; this milestone is about the *process and ownership*
around it, not a license change. Not legal advice - the chosen model should get a
real legal review before it is published.

**CLA vs. DCO.** The sign-off decision from "Contribution mechanics", worked
through for this project. **Recommendation: the DCO (Developer Certificate of
Origin 1.1) in every repository the project runs, and no CLA**, with one
license decision taken first (below).

- *What each is.* The DCO is a `Signed-off-by: Name <email>` trailer on every
  commit (`git commit -s`): the contributor certifies they may submit the work
  under the license of the file it lands in. No extra grant, no paperwork, no
  bot; copyright stays distributed and inbound = outbound. A CLA is a signed
  agreement that grants a named recipient a broad license, usually including
  the right to relicense. It needs that recipient, a signing service, a record
  per contributor, and contributors increasingly decline CLAs that let one
  party relicense their work.
- *Why the DCO here.*
  - The one relicensing need this milestone names is an exception for a
    statically linked, embedded `jennifer-tiny` that cannot meet LGPL's relink
    terms. That needs no CLA: while mplx is the only copyright holder, a
    static-linking exception can be added to the license now (LGPL-3.0 plus a
    linking exception, as several embedded libraries ship), and every later
    contribution arrives under it. A CLA would only add the right to relicense
    arbitrarily later, which nothing here plans. **Decide the exception before
    the first outside merge**: after it, adding one needs every contributor.
  - There is no entity to receive a CLA. A CLA to a private person asks for
    trust a foundation would not need, and moving it to an entity later is
    paperwork per contributor.
  - Two forges with no mirroring: CLA services are GitHub apps, Forgejo has
    none. A DCO check is a short script over `git log` that runs the same in
    both CI systems and locally.
  - Code moves between repositories: bundled modules become `@jennifer/*`
    decks and fixes are ported both ways until the bundled copy is removed. A
    CLA on `jennifer` alone would leave code ported in from a DCO-only deck
    uncovered; one rule everywhere keeps its provenance uniform.
- *Effects on the repositories.*
  - **`jennifer-language`** (`jennifer`, `website`, `academy`, `apps/*`,
    `decks/*`, `profile`): one contribution page in the org's `.github`
    repository, linked from each repo's README. A `check-signoff` step fails a
    pull request whose commits lack a sign-off matching the commit author.
    Decks get it through the shared CI layer (`deck-template`, kind `shared`,
    synced out); `jennifer`, `website`, `academy` and the apps add it to their
    workflows on both forges. Maintainers sign off too, so `commit.txt` lines
    become `git commit -s -m "..."`. An assistant's `Co-Authored-By` trailer is
    credit, never a sign-off: only the person who submits certifies the DCO.
  - **`academy`**: keeps its contributor declaration, which covers more than
    the DCO (rights to images and text, CC BY 4.0, the content policy,
    responsibility); the sign-off is added so every repository has one rule,
    and it covers the MIT-0 code samples.
  - **`@mplx`** (`openconnect-saml`, `deck-clispinner`, MIT): personal
    repositories outside the policy; using the same check is recommended,
    since clispinner follows the deck conventions anyway.
  - **`deckyard`**: run by the project, so the same rule and the same check,
    through the shared layer. Its decks may use any OSI licence (its
    CONTRIBUTING); the DCO certifies against each deck's own license, which
    a CLA bound to one license holder would not fit.
  - **Third-party scopes in the registry** (`@acme/...`): outside the policy.
    The project holds no rights in them and asks for none; the registry
    records the declared license and does not vet it. Vendoring copies them
    into a consumer's tree under their own license. Code copied from such a
    deck into a project repository must be under a compatible license, which
    the contributor certifies with clause (b) of the DCO.
- *Still to decide before adopting it:* the exact exception text (legal
  review); whether the sign-off email must match the commit author
  (recommended: yes); whether a pseudonym is accepted, as many DCO projects
  now allow.

**Requires:** none (organizational, independent of the codebase). Socially paired
with the M19.8 org move and triggered by the first external contribution, but no
code prerequisite - which is why it is the one 1.0.0 requirement that can be
settled at any time before the first outside PR.

---

## Requirements for 1.0.0 stable

- **One convention everywhere** - stance 8: the conventions survey (`M29.23`)
  has no open finding in the built-in libraries, the bundled modules and the
  `@jennifer` decks, every rule it can test is a gate, and each remaining
  exception is recorded in the catalogue. A **hard requirement**: after 1.0 a
  fix is a major release.
- **Project governance, licensing, and contribution policy** - the `M32`
  work: the copyright-holder model, copyright notice, relicensing headroom,
  `CONTRIBUTING.md` / sign-off, governance, and name / mark. A **hard
  requirement** for 1.0.0, and it must be settled before the first external
  contribution is merged (whichever comes first).
- **Cross-build for macOS / Windows.** The `M31` multiplatform track
  (`M31.1` Windows, `M31.2` macOS) does this; ships as soon as it lands.
- **Real apt repository** (replacing the "GitHub Release
  artifact" install of the M15.8 `.deb`) if user demand
  warrants the maintenance.
- **Container image (OCI).** Done - `.github/workflows/docker.yml`
  builds and pushes multi-arch (`linux/amd64` + `linux/arm64`)
  `ghcr.io/<owner>/jennifer` images on each release tag: a Debian-slim
  default (`:latest` / `:<ver>`, full host features) and a distroless
  `:static` variant. The image bundles both binaries and the system
  modules under the compile-default module dir, so a bare
  `import "name.j";` resolves with no env var. See `packaging/docker/`.
- **Homebrew tap (macOS, best-effort unsupported).** Available -
  `packaging/homebrew/jennifer.rb` builds the standard `jennifer` from
  source (so Intel + Apple Silicon work with no Gatekeeper friction) and
  bakes the version and module path via `-ldflags -X`, so a bare
  `import "name.j";` resolves with no env var. Published to the
  `jennifer-language/tap` tap by a manual `publish.sh` (the AUR convention;
  no CI secret). The `-UNSUPPORTED` macOS tarball stays alongside it.

The extra Linux / macOS distribution formats (Homebrew, Snap,
Nix, Flatpak, AppImage, ...) are not requirements; they live in
the [horizon idea collection](horizon.md) and ship when there's
user demand and a maintainer willing to keep one green.

---

## Long horizon

Ideas *beyond* 1.0.0 - embedding, a WASM runtime, specialised-domain
libraries, and smaller possibilities - live in the
[beyond-1.0.0 idea collection](horizon.md), so this file stays focused on the
road to 1.0.0.
