# Jennifer - Roadmap

Where Jennifer is going: the planned releases up to 1.0.0, the governance and
licensing work, what 1.0.0 requires, and the platform work (M31) that follows
it. What has shipped is in
[milestones](milestones.md), ideas beyond 1.0.0 in the
[horizon](horizon.md), and deck plans in [decks](decks.md).

Each milestone from M28 on is one release. Its entries are numbered `Mx.y`;
numbers are stable IDs, never reused, and an entry moved to another release
gets a new number and keeps `(was Mx.y)`. Each entry starts with **Planned**
or **Done**; an entry done during a cycle stays here, marked **Done**, until its
release is tagged and the section moves to [milestones](milestones.md).

| Milestone | Release | What |
|---|---|---|
| [M29](#m29---0280) | 0.28.0 | done; M29.55 moves to [milestones](milestones.md#m29---0280) with the tag |
| [M30](#m30---0290) | 0.29.0 | copy elision and in-place updates, streaming and binary formats, function signatures, grapheme clusters, the conventions survey |
| [M32](#m32---project-governance-licensing-and-contribution-policy) | before the first outside contribution | governance, licensing, CLA vs. DCO, contribution policy |
| [1.0.0](#requirements-for-100-stable) | 1.0.0 | the requirements for a stable release |
| [M31](#m31---multiplatform-promote-macos--windows-to-supported) | after 1.0.0 | macOS and Windows promoted to supported platforms |

---

## M29 - 0.28.0

The rest of M29 is done and summarised in
[milestones](milestones.md#m29---0280); M29.55 stays here until 0.28.0 is
tagged, and the rest of the conventions survey moved to M30.28.

#### M29.55 - release every deck before 0.28.0

**Done** (2026-11-26): the registry serves the "Next" release or a later one
for all 57 decks; all ten `@jennifer` names carry a "moved" record to
`@deckyard`. M29.35 removes builtins that the decks' latest releases still
call, and a consumer installs a release, not `main`: a program using one of
them stops working the day 0.28.0 is released. `main` of every deck was
migrated (no removed call left, floor `>=0.27.0`); the releases
were not. 57 decks (10 `@jennifer`, 2 `@mplx`, 45 `@deckyard`) have a removed call
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
| `@mplx/texttransform` | 0.3.0 | 0.4.0 |
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

**Every `@jennifer` deck moves to `@deckyard`.** The project ships the
language and its standard library; decks live in Deckyard, where others can
co-maintain them. The ten: discord, gotify, influxdb, mactelnet, mikrotik,
oauthext, prometheus, slack, statsd and telegram. `@jennifer` keeps no decks
(the scope table is in `decks.md`); the bundled `oauth` module points to
`@deckyard/oauthext` for provider presets. Each moving deck's first release
under the new name continues its version series (the next minor after its
last `@jennifer` release):

- Each repository moves to the `deckyard` organisation on GitHub and Forgejo;
  the manifest name, the imports (`@deckyard/<name>/`), the docs and the
  README install line change.
- Every version of a moved `@jennifer/*` deck is yanked: a lockfile that
  pins one still fetches it, a new resolution does not.
- **A moved deck.** The registry gets a per-deck `movedTo` record
  (`deckadmin move @jennifer/slack @deckyard/slack`), shown in the API and on
  the deck page; `jvc add` / `install` / `update` refuse a moved deck with a
  message naming the new one. No transparent alias: every import must change
  anyway, and two names would stay forever.
- `@deckyard/routeros` depends on `@deckyard/mikrotik`, not
  `@jennifer/mikrotik`.
- The 0.28.0 CHANGELOG, `docs/decks.md`, the deckyard profile README and
  every pointer to `@jennifer/<name>` in docs, academy talks and the website
  name the `@deckyard` deck. Released interpreters (0.26.0, 0.27.0) keep
  pointing at `@jennifer/<name>`, which the moved record answers.

**Requires:** M29.35.

## M30 - 0.29.0

Copy elision, streaming and binary formats, function signatures (declared
return types, library and module functions as values), grapheme clusters, and
the removal of what 0.28.0 deprecated (M30.25). Entries keep their former M28
or M29 number as `(was M28.n)`.

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
  table paid 1.2 ms a row for it (`@deckyard/sheets`), and moved
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
- typed `func` values (M29.16, now part of this entry) and generic functions
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
- **Typed `func` values** (from M29.16): a parameter typed with a signature,
  `guard as func(string, json.Value) as bool`, is checked when the value is
  called; a result of another type throws a kind of its own (not
  `"runtime"`), so a module tells a callback's wrong result from a fault
  inside it. `@deckyard/fsm` then drops its source-position check.
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

**Requires:** none. Unblocks call results in M29.28; DRAFT#33 uses the same
spelling for `as T`.

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
with them. Moved from M29: four of them are M30 entries, so it
cannot land in 0.28.0.

#### M30.25 - remove what 0.28.0 deprecated

**Planned.** Every name and call shape 0.28.0 deprecates (lint L301 reports
each, naming the replacement) is removed, moving it to L302:

- `time.fromUnixMillis` / `unixMillis` (M29.52);
- `net.connect(addr, ms)`, `connectTLS(..., ms)`, `startTLS(..., ms)`,
  `readAll` / `readN` with a trailing timeout (`setIdleTimeout` instead),
  `net.eof` (M29.37, M29.45, M29.46);
- the `sql` driver aliases `mariadb` / `postgresql` and `setQueryTimeout`
  (M29.36);
- `http.requestWith`, `requestTls`, `requestWithTls`, `requestRawBody`,
  `requestRawBodyTls`, `requestBytes`, `requestWithBytes`, `Options.tls` and
  `http.TlsOptions` (M29.43), moving the public `tls` fields of
  `oauth.Config`, `soap.Client` and `webdav.Client` to `net.TLSOptions`;
- `crypto`'s `"rsa-sha*"` names and `string` passwords (M29.51);
- the two-argument `xml.decode` (M29.47), the positional caps of
  `compress.unpack` / `images.decode` and `archive`'s `"tgz"` (M29.44);
- `ml.free` and the trailing ml hyper-parameters (M29.39); the frozen v2
  suite of `examples/benchmark.j` calls `ml.free` inside its timed code,
  so removing it needs a decision there (keep a no-op until the suite is
  retired, or end the v2 series);
  `stats.multipleRegression`, `convert.toInt(float)` truncation (then only
  an integral float), `os.hasFlag` / `os.flag`, `decimal.toString`
  (M29.48);
- `httpd.listenTLS` / `listenTLSWith` / `serveFileEtag` / `serveDirEtag`,
  the `gpio` module (M29.48); `spi.configure`, `gpio.chip`,
  `JENNIFER_GPIO_CHIP` and the pin-only `gpio` forms (M29.49);
- `images.rgba` (M29.48);
- `fs.watch(path, intervalMs)` and the `os.spawn` options map (M29.45).

Before the tag, the decks and apps are checked for each removed name, as
M29.55 did for 0.28.0.

**Requires:** the deprecations above, shipped in 0.28.0.

---

#### M30.26 - `crypto` RC4 for interop

**Planned.** PDF's standard security handler revisions 2 to 4 encrypt with
RC4, and many PDFs are encrypted with an empty user password only to carry
permissions, so a PDF reader (the `pdfread` deck idea in `decks.md`) cannot
read their text without it. `crypto` has no RC4.

- `"rc4"` in `crypto.blockEncrypt` / `blockDecrypt`, beside `"aes-ctr"`: a
  stream cipher, so both directions are the same keystream XOR. Key 1 to 256
  bytes (PDF uses 5 to 16), no IV (an `iv` argument is an error), any data
  length.
- Tagged "(weak)" in the docs and the algorithm table, like `md5` and `sha1`:
  for reading and writing legacy formats only; new encryption uses AES-GCM or
  the `sealbox` deck.
- Go's `crypto/rc4` (still in the standard library; check the TinyGo build).
- Tests: the RFC 6229 vectors (40-bit to 256-bit keys, offsets 0 and 1536),
  encrypt then decrypt round trip, key length 0 and 257 refused.

#### M30.27 - `compress` LZW as PDF and TIFF use it

**Planned.** PDF's `LZWDecode` filter (common in older files) and TIFF's LZW
compression use the variable-width MSB-first LZW with "early change": the code
width grows one code early. Go's `compress/lzw` has no early-change mode, so
it reads neither.

- `"lzw"` in `compress.unpack` / `unpackWith` / `pack`, MSB-first, 8-bit
  literals, clear code 256, end code 257, codes up to 12 bits. `pack` writes
  the early-change form.
- `earlyChange` in `compress.UnpackOptions` (`true` by default, as PDF's
  `/EarlyChange 1` and TIFF), `false` for the late-change variant.
- The output cap and `"limit"` of `unpackWith` apply; a code past the table, a
  missing end code or bytes after it are an error of kind `"compress"`.
- Implemented in Go in `internal/lib/compress` (no dependency); both builds.
- Tests: PDF 32000-1 section 7.4.4.2's example, TIFF 6.0 sample strips,
  round trips at the 9 / 10 / 11 / 12-bit boundaries with both settings, a
  table-full stream without a clear code, the cap.

#### M30.28 - the conventions survey: stance 8, checked (was M29.23)

**Planned.** Moved from 0.28.0. Done in 0.27.0 (see M29.23 in
[milestones](milestones.md#m29---0280)): the catalogue, the survey tool
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

#### M30.29 - in-place update for `$x = f($x, ...)`

**Planned.** A builder that returns its argument changed (`binary.setUint`,
`json.set`, `xml.setAttr`, `maps.delete`, a user function `func bump(c as
Counter) { $c.n = $c.n + 1; return $c; }`) copies the whole value, because the
argument might be shared, even when the caller replaces the only reference:

```jennifer excerpt
$b = binary.setUint("le", $b, 4, $len, 4);   # copies all of $b to write 4 bytes
$doc = json.set($doc, "/counter", $i);       # copies every key of $doc
```

Measured on a `main` build (2026-11-26), 1000 calls each:

| Value | Per call |
|---|---|
| `binary.setUint`, 4 bytes into 64 KiB / 1 MiB / 8 MiB | 0.11 / 0.33 / 2.5 ms |
| `json.set` of one key in a map of 1,024 / 16,384 / 131,072 keys | 0.03 / 0.35 / 0.98 ms |

So patching a ZIP header's sizes, filling a buffer field by field or updating
a counter in a large document is O(value) per call, and a loop of them is
quadratic. `binary.md` steers around it (`from*` plus `join`), and M30.6
removes only the second copy, at the binding site.

- **Ownership passing.** When an argument is the variable the call's result is
  assigned to (`$x = f($x, ...)`, also through a place expression: `$s.buf =
  binary.setUint("le", $s.buf, ...)`), and nothing else holds its backing (not
  borrowed by a running call, not captured by a `spawn` snapshot, not a
  `const`), the call takes the value instead of a copy, and the builtin or
  function may change it in place. The variable is dead between the call and
  the store, so nothing can observe the difference: value semantics hold.
- **Builtins** opt in per function (a flag like a user function's
  `ReturnsOwned`), starting with `binary.set*`, `json.set` / `remove`,
  `xml.setAttr` / `setAttrNS` and `maps.delete`.
- **User and module functions** get it through the existing borrow analysis: a
  parameter the function writes and returns is moved in rather than copied
  when the call site qualifies.
- **The model:** M28.3's `[] =`, which made appending in place linear by the
  same argument. Not an in-place API (`binary.patch(b)`): no built-in mutates
  its argument, and the stance-8 builder shape stays.
- **Tests:** the `value_alias` stress tests (note 10) for every qualifying
  shape, including an alias taken before the call, a `spawn` between, a
  borrowed parameter passed on, and a global read by the callee; the numbers
  above as a benchmark, expected O(change).

**Requires:** M30.6 (the binding-site copy), the note-24 borrow/escape
machinery.

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

- **One convention everywhere** - stance 8: the conventions survey (`M30.28`)
  has no open finding in the built-in libraries, the bundled modules and the
  `@jennifer` decks, every rule it can test is a gate, and each remaining
  exception is recorded in the catalogue. A **hard requirement**: after 1.0 a
  fix is a major release.
- **Project governance, licensing, and contribution policy** - the `M32`
  work: the copyright-holder model, copyright notice, relicensing headroom,
  `CONTRIBUTING.md` / sign-off, governance, and name / mark. A **hard
  requirement** for 1.0.0, and it must be settled before the first external
  contribution is merged (whichever comes first).
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

## M31 - multiplatform: promote macOS / Windows to supported

Linux is the only *supported* platform, but best-effort **unsupported** macOS /
Windows binaries (the standard-Go `jennifer`, via cross-compile) already ship each
release - so the work is not "add the ports" but **promoting them to supported**.
Planned after 1.0.0: the language and its conventions settle first, then
the platforms they run on; until then macOS and Windows stay best-effort
unsupported builds. A portability audit found the surface small: separators / EOL /
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

## Long horizon

Ideas *beyond* 1.0.0 - embedding, a WASM runtime, specialised-domain
libraries, and smaller possibilities - live in the
[beyond-1.0.0 idea collection](horizon.md), so this file stays focused on the
road to 1.0.0.
