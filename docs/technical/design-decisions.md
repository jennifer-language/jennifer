# Design decisions

Decisions that ship in the language but look, at first glance, like
they conflict with one of Jennifer's seven design stances. Each entry
explains why the feature is *not* the kind of thing the stance was
written to reject. The negative counterpart is
[Rejected features](rejected.md): proposals that *were* turned down
because they really did clash with a stance.

When in doubt, the stances list in
[../user-guide/index.md](../user-guide/index.md) is authoritative for
users; this file is the reasoning record for maintainers.

## The `$xs[] = item;` append form

Stance #1 ("one way per thing") normally rejects sugar that creates a
parallel API. `$xs[] = item;` and `$xs = lists.push($xs, item);` do
compile to the same operation, so the form looks suspect under that
rule. It ships anyway because the three properties below set it apart
from the rejected `$i++` / `+=` family - the form is not a parallel
API, it's the index-write syntax growing one more legal position.

1. **`$xs[]` re-uses an existing operator slot; it is not a new
   operator.** `$xs[i] = item;` already targets a list position via
   the `[...]` index-write syntax. `$xs[] = item;` extends that same
   operator to one position the existing syntax didn't cover -
   "just past the end" - by passing an empty index. No new token is
   introduced. Compare `$i++`: that proposed a *new* operator (`++`)
   competing with the canonical `$i = $i + 1;`. The bracket form has
   no new token to learn, no precedence to memorize, and no parse
   rule that wouldn't exist anyway.
2. **Index-write semantics, not function-call semantics.**
   `$xs[i] = item;` mutates the binding's list in place.
   `$xs[] = item;` extends that in-place behaviour to the append
   position, where the function-call form
   (`$xs = lists.push($xs, item);`) needs an explicit reassignment to
   commit the new list back into the binding. So the bracket form
   isn't a "shortcut for `lists.push`" so much as the index-write
   syntax growing one more legal position. The two forms have
   genuinely different shapes: one is a write statement that mutates
   a binding, the other is an expression that returns a new list.
3. **Write-only; no expression-context footgun.** `$xs[]` cannot
   appear on the right-hand side of any expression - reading "the
   element just past the end" has no meaning and is rejected at
   parse time. `$i++`'s real problem was that pre/post forms differ
   only in expression context, which is where the bugs hid. `$xs[]`
   has no expression context to hide in, so the analogous footgun
   cannot exist.

What this means for `lists.push`: it stays in the language and is
canonical for any context that needs the post-append list as an
expression value (passing it into another call, chaining
transformations). The two spellings are not parallel APIs that do the
same thing in the same context; they fit different syntactic
positions - the bracket form for the in-place write statement, the
function form for the expression value. That's also why the same
argument doesn't license a `bytes.push` removal once `$b[] = byte;`
ships: any future code that needs "a new bytes value with this byte
appended" as an expression still wants the function form.

## XOR (`^`) as its own operator

Stance #1 ("one way per thing") would normally argue against shipping
an operator that's algebraically derivable from operators we already
have - XOR is `(a | b) & ~(a & b)` in terms of the other bitwise
primitives. It ships anyway because XOR is a CPU primitive with
unique algebraic properties that show up at every use site:

- **Self-inverse**: `$a ^ $a == 0`.
- **Round-trip**: `($a ^ $b) ^ $b == $a` - the canonical reversible
  transform (cheap obfuscation, parity bits, the classic in-place
  swap trick).
- **Bit-toggle**: `$flags ^ $mask` flips exactly the bits set in the
  mask, leaving the rest alone.

Forcing every XOR use site to write the three-operator composition
would be the `a - b` ≡ `a + (-b)` argument: we still ship `-` because
the composed form obscures the intent at every call site. Same logic
applies here.

## `len` is a language built-in, not a library

`len(EXPR)` is a reserved keyword and a primary expression in the
grammar, not a function in any library. Stance #2 ("explicit over
implicit") would normally argue that every name should be
explicitly imported - which is exactly what every library obeys
(`use io;`, `use math;`, etc.). The pre-M15.4 design had the
`core` library auto-loaded as the one exception to this rule, so
that `len` could be called without ceremony. M15.4 chose a
different answer: promote `len` to a language built-in so the
exception disappears, instead of preserving the auto-loaded
library.

The case for keeping `core` auto-loaded (the path we didn't take):

- Minimal language surface area (one stronger reserved word avoided).
- The auto-loaded library was already justified once; doubling down
  is cheaper than redesigning.
- A future library that wants the same exception ("polymorphic
  structural primitive every program needs") could be added the
  same way.

The case for the built-in (what we ship):

- **Stance #2 alignment is now uniform.** Every name a Jennifer
  program reaches for either lives in the language (operators,
  keywords, `len`) or behind an explicit `use lib;`. There is no
  third category. A reader can audit a `.j` file's imports and
  know every external name in scope.
- **No special-case library machinery.** `RegisterGlobal`,
  `globalFnsByLib`, the alias-meaningless-for-globals-only-lib
  rule, the "library 'core' is automatically available" rejection,
  the "skip core from the available-libs error message" filter -
  all of that infrastructure existed to support one auto-loaded
  library. With `len` promoted to a built-in, none of it is
  required.
- **Future polymorphic primitives have a clear home.** If `len`-like
  behaviour ever needs a sibling (e.g. a future `empty(v)`), the
  decision is the same: language built-in or topic library, not
  "expand the auto-loaded list."
- **No `core` to keep tightening.** M15.1 moved `JENNIFER_VERSION`
  out of `core` into `meta`; the charter discussion ("what
  qualifies for `core`?") had already started chipping at the
  exception. Removing `core` entirely closes the question instead
  of arguing it indefinitely.

Tradeoffs accepted:

- **Another reserved word.** `len` is now a keyword - users can't
  define `func len() {}`. The same restriction existed under the
  old model (the M5-era "shadows builtin" runtime check), just
  enforced one phase later.
- **Migration churn for any out-of-tree code.** Source that wrote
  `use core;` errors with a friendly migration hint; sources that
  defined `func len()` get a parse error pointing at the keyword
  rename. Pre-1.0 covers both.

`RegisterGlobal` / `RegisterGlobalConst` remain on the interpreter
as exported API for compatibility, but no shipping library calls
them; the in-tree consumer is gone. A later cleanup pass removes
the infrastructure once the M10 collision-rule tests that exercise
it migrate.

## Half-open ranges

`lists.range(start, end)` is half-open: `lists.range(1, 100)` returns
`[1, 2, ..., 99]` (99 elements; 100 excluded). The English-reading
stance Jennifer has applied to syntax (`repeat ... until`, word
operators, `as` / `init` / `in`) would argue for the closed form -
"from 1 to 100" in English includes 100. We deliberately don't extend
that stance to value-generating runtime operations.

The English-reading argument applies cleanly to *syntax*, which is
read once when learning the language. `lists.range` is a *runtime
operation* whose semantics are read at every use site, and the cost
of getting it wrong is paid every time the user composes ranges,
partitions an iteration, or aligns a range with indexing. The
half-open form makes those operations easier; the closed form makes
the function name read more naturally in isolation. We pick the
operation-friendly form.

The case for half-open:

- **Index alignment.** `lists.range(0, len($xs))` yields exactly the
  valid 0-based indices for an `len($xs)`-element list. Closed
  would force `lists.range(0, len($xs) - 1)` - the off-by-one trap
  the half-open form was invented to eliminate.
- **Composability.** `lists.concat(lists.range(a, b), lists.range(b, c))`
  is exactly `lists.range(a, c)`. Partitioning a range at any point
  composes cleanly with no duplication and no `+1` adjustment.
  Closed would either duplicate `b` or require
  `lists.range(b + 1, c)` on every partition.
- **Stepping uniformity.** Half-open stepping is always "emit while
  inside the open end" with no "did the step land?" question. The
  user never has to reason about whether `end - start` divides
  evenly by `step`.
- **Consistency with the rest of the stdlib.** `lists.slice`,
  `strings.substring`, and 0-based indexing are all half-open. A
  closed `range` would make it the only exception, forcing every
  user to remember the special case.
- **CS-tradition languages all picked half-open.** Python `range`,
  Go slice indexing, Rust `..`, C++ STL iterators `[begin, end)`,
  JavaScript libraries. The "natural-syntax languages picked
  closed" framing is misleading: Ruby ships *both* (`..` closed,
  `...` half-open), Swift ships *both* (`...`, `..<`), Kotlin
  ships *both* (`..`, `until`). When you can ship only one because
  of stance #1, half-open is the more general choice - closed is
  recoverable as `lists.range(start, end + 1)`, but the composition
  and index-alignment properties of half-open are not recoverable
  from closed.

The English-reading stance still wins for syntax (it costs nothing
at runtime), but it's the wrong tie-breaker for a value-generating
operation that's about behaviour, not prose. This entry exists to
record the call for future tie-breakers: when an operation's
semantics matter at every use site, the operation-friendly form
beats the prose-friendly form.

We deliberately don't ship a closed variant (`lists.rangeInclusive`
or similar) - stance #1 rejects parallel APIs, and the closed form
is recoverable as `lists.range(start, end + 1)` when the user wants
"count 1 to N inclusive." `lists.range(end)` with a single-arg
default-start form is also not shipped (stance #2: explicit over
implicit).

## The `sql` library's heavyweight driver dependencies

Jennifer states a dependency-free discipline for the library layer: every
standard library is pure-stdlib, the two carve-outs being `gopkg.in/yaml.v3`
(a parser too big to hand-roll) and the CLI-scoped `golang.org/x/term`. The
`sql` library breaks further: it takes two **real dependency trees** -
`go-sql-driver/mysql` and `jackc/pgx` - which looks like a clear violation.

It ships anyway, and the reasoning is deliberate, not slid in:

- **Both drivers are pure-Go.** No cgo, so static builds, cross-compilation, and
  the best-effort macOS / Windows artifacts stay clean. The one *embedded* engine
  that would need a multi-MB, TinyGo-hostile dependency - SQLite - is excluded and
  parked in [horizon](../horizon.md) behind a build tag, precisely to keep this
  line from being crossed casually.
- **The deciding factor is correctness maturity, not convenience.** MySQL and
  Postgres are open TCP wire protocols, so a client *is* writable in pure Jennifer
  (the same shape as `redis` / `imap`). But the mature drivers have absorbed a
  decade of protocol long-tail - every auth plugin (`caching_sha2_password` /
  the RSA path, SCRAM-SHA-256), charset handling, NULL semantics, multi-result
  sets, server-version quirks - that a hand-rolled `.j` client would re-derive one
  edge case at a time. For databases users depend on daily, that maturity is worth
  the dependency; the auth crypto becomes the driver's problem, not the language's.
- **It is not a performance concession.** A database client is latency-bound
  (network round-trip + server execution dominate); client-side decode is the cost
  center only when streaming 10^5+ rows, a bulk workload Jennifer is the wrong tool
  for regardless of driver. So this is not "reach for a fast Go library" - it is
  "reach for a *correct* one".
- **It is TinyGo-clean by construction.** The build-tag split (`sqllib_std.go`
  imports the drivers, `sqllib_tiny.go` stubs them) means `jennifer-tiny` never
  compiles the trees; the language stays TinyGo-clean and the interpreter core is
  untouched.

The precedent (`yaml`) took one pure-Go dependency for a parser too big to
hand-roll; `sql` extends the same judgment to a client too intricate to hand-roll
*correctly* for engines people trust with production data. The bar stays high: a
new heavyweight dependency needs this same "correctness maturity that a hand-roll
would re-derive slowly and get subtly wrong" justification, not mere convenience.

## Per-goroutine call-depth counter (concurrent `web` dispatch)

Turning the `web` framework from serial to concurrent request handling looked
like a one-line change: wrap each request in a `spawn`. The naive version worked
- a fast request issued mid-way through a slow handler dropped from ~1700 ms to
~3 ms - but `go test -race` reported a data race on the call-depth counter. The
fix that makes concurrent dispatch safe turns on understanding why.

The call-depth guard (the `RecursionError` analogue that raises a catchable
error before the Go goroutine stack overflows fatally) counts nested method
calls. It originally lived as a plain `int` on the per-goroutine **root env**
(`i.global` in serial code, a `spawn` snapshot's globals frame inside a spawn),
which is correct for ordinary spawn bodies: each snapshot is a distinct root, so
parallel spawns never share the counter.

Handler dispatch breaks that assumption. A handler is dispatched through
`meta.callMain` into the **entry program's** interpreter, and `CallMethodWith`
re-roots the handler frame at that interpreter's *shared* `host.global`. So the
counter a handler's nested calls bump is `host.global`'s single field - and
several `spawn`ed workers dispatching handlers into the one shared host all
increment it at once. The value-semantics guarantee that makes ordinary spawns
race-free ("each spawn deep-copies its scope") does **not** extend across the
`meta.callMain` boundary, because the whole point of that boundary is to reach
the *shared* entry program, not a copy.

The fix makes the counter a `*int` **threaded down the logical call chain**
rather than read off whatever root a frame happens to sit on. `evalCall` stamps
the caller frame's counter onto the callee frame (`callFrame.depth = env.depth`)
instead of reading `effectiveGlobal`'s shared one, so a chain shares one counter
while concurrent chains stay isolated.

The first cut of this minted a *fresh* counter at each cross-interpreter dispatch
entry (`CallMethodWith`) - which is race-free, but wrong in a subtler way that an
adversarial review caught: it **reset the depth at every boundary crossing**, so
recursion that bounces through a dispatch builtin (`meta.call` self-recursion, a
handler that re-dispatches itself via `meta.callMain`, a module method that calls
back into the host) never accumulated past a single crossing and grew the Go
stack unbounded - converting a **catchable** "call stack too deep" error into a
**fatal, unrecoverable** Go stack overflow (a whole-process crash, e.g. a web
handler taking down the entire server). A fresh counter per crossing is the wrong
knob precisely because the milestone's own step-2 note warned "must NOT reset
depth in a way that breaks meta.call recursion detection."

So the counter is instead **threaded across the dispatch boundary too**, not
reset. `dispatchModuleMethod` passes the consumer's `env.depth` into the module
interpreter's `callMethodWithDepth`; the `meta.call` / `meta.callMain` builtins
carry the caller's counter through `BuiltinCtx.Depth` into
`CallByNameWithDepth` / `CallHostWithDepth`; and those dispatch entries increment
it (a crossing adds Go frames like any call). Only a genuine root entry with no
caller chain - the CLI invoking the entry program, a `testing` harness - mints a
fresh counter. The caller's counter is always goroutine-local (a spawn worker's
own, or `i.global`'s for the main goroutine), so threading it keeps concurrent
chains isolated *and* lets a single logical chain accumulate across every
boundary. The result is strictly better than the pre-change reach: recursion that
bounces purely through `meta.call` (never guarded before, because those builtins
bypassed `evalCall`'s increment) is now catchable too.

Two consequences worth stating. First, the audit that gated the change (run
under `-race`): the *other* shared interpreter state a concurrently-dispatched
handler can reach is safe - the map hash-index is built only on an indexed
*write* (a read never mutates the shared value), the resolver's
`CallExpr.Method` / `Fn` caches are stamped single-threaded before any spawn and
only read afterwards, the profiler's collector is mutex-guarded (and off the
serve path anyway), and `diagReq` is atomic. Second, the one genuinely-unsafe
pattern is left to the user and documented, not papered over: a handler that
**writes** a shared top-level variable of the entry program races another
handler doing the same, because handler workers run against the same host
globals (no per-goroutine snapshot across `meta.callMain`). That is the
threads-sharing-memory hazard every language has; the guidance is to keep
per-request mutable state in the request/response or a synchronized store, not a
bare global a handler assigns.

## The `orm` module maps rows as `map of string to string`, relations via a side `Result`

The `orm` module is a **Data Mapper**: a row is a `map of string to string` keyed
by column name, not a user struct, and `orm.load` returns eager-loaded relations
in a separate `Result` holder rather than nesting them into the row map. Both look,
at first glance, like they fight stance #4 (rich, honest types) - why hand back
stringly-typed maps instead of typed records?

Because the language has **no struct reflection**. A generic mapper cannot
populate an arbitrary user struct field-by-field at runtime (there is no way to
enumerate a struct's fields or set one by name), so a typed-row API would have to
be hand-written per table by the caller - which is exactly what the caller does
today by reading `row["name"]` and rebuilding a typed value explicitly. The map is
the honest shape for "a row of unknown-at-compile-time columns": the database
coerces the string values to column types, and values bind only through
placeholders, so the safety story is intact. A typed-struct row form is recorded
as [rejected](rejected.md) (it needs the missing reflection), to revisit if the
language ever grows field reflection.

The **side `Result`** falls out of the same constraint. A `map of string to
string` is homogeneous - it cannot hold both scalar columns *and* child-row lists
without becoming a heterogeneous `map of string to any`, and Jennifer has no `any`
(that is the [json.Value](../libraries/json.md) opaque-tree decision restated). So
eager-loaded associations live in an identity-map-style `Result` read through
`orm.related` / `relatedOne`, keeping the row map a clean scalar record. It is the
same "no language-level top type; walk heterogeneous data with explicit
accessors" stance the `json` / `toml` / `yaml` libraries take.
