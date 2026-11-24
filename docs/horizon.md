# Beyond 1.0.0 - idea collection

Jennifer's near-term target is a rich, dependable set of language
features, libraries, and modules - enough to tag a **1.0.0**. That work is
tracked in the [roadmap](roadmap.md). This file collects ideas for
*after* 1.0.0: directions worth recording so the design doesn't foreclose
them, none committed to a timeline.

Two kinds:

- **[Drafts](#drafts)** - concrete, already-shaped directions, grouped by theme
  and ordered roughly easiest-first. Each has a design; it just has no schedule.
- **[Loose ideas](#loose-ideas)** - a grab-bag of smaller or vaguer
  possibilities, loosely grouped, jotted down when they come up so they are not
  lost.

Nothing here is a commitment. An idea graduates into the
[roadmap](roadmap.md) when it earns a slot.

## Drafts

Ordered roughly easiest first: language tweaks and additive libraries near the
top, the big runtime and ecosystem restructures near the bottom.

Each carries a stable **`DRAFT#`** handle so it can be referenced and later
graduated into a numbered milestone. Handles are **assigned once and retired on
graduation**, never reused and **never renumbered when the list is reordered**,
so a reference stays valid for the life of the idea (hence the numbers below
run out of sequence). `DRAFT#` is not a milestone number; an idea gets an
`M`-number only when it graduates into the [roadmap](roadmap.md).

Each draft also states its **Requires** - the `DRAFT#` handles and/or milestones
(`M`-numbers) that must land first, or *none* - so its blockers are explicit
before it is scheduled.

### Language

#### DRAFT#28 - Anonymous function literals with value capture (Tier 2 of first-class functions)

`func` values already ship (`M24.2`): a function held in an immutable value,
passed / returned / stored / called, powering the higher-order `lists` layer. What
is missing - the "Tier 2" `M24.2` deferred - is the **anonymous function literal**
(an inline lambda) and **capture** of enclosing bindings. Today a one-off transform
must be hoisted to a named top-level function
(`func dbl(n as int) { return $n * 2; }` just to write `lists.map($xs, dbl)`), and
a middleware or handler cannot close over state, which is why the module ecosystem
routes so much through **by-name dispatch** (`web` handlers, `args.dispatch`,
`webapi`'s authenticator / limiter names + its `guard` shim). Both the ergonomic
complaint ("no closures") and those internal workarounds are the same gap.

**The design that fits: value capture, not reference capture.** A literal
`func(n as int) { return $n * $factor; }` in expression position evaluates to a
`func` value whose captured bindings (`$factor`) are a **deep-copy snapshot taken
at the moment the literal is evaluated** - exactly what `spawn` already does to its
enclosing scope. Captured bindings are immutable snapshots, never live references,
so no aliasing is introduced and value semantics hold unchanged. This is the only
flavor consistent with the language; the reference-capturing (JS / Python) flavor
is rejected (see [rejected.md](technical/rejected.md)) because a live env pointer
breaks value semantics and the frame-pool / borrow / spawn-snapshot optimizations
documented as safe *because* there are no such captures.

**It reuses machinery that already exists.** Capture is `snapshotForSpawn`'s
deep-copy, so the value-semantics-capture path is built. A closure body is a
resolver / borrow-analysis **hazard boundary** the same way a `spawn` body is (skip
descent, treat as opaque), and a captured closure is **excluded from the frame
pool** the same way a spawn snapshot is - the carve-outs are the ones the
concurrency work already established, applied to a new node. Across a module
boundary a closure carries its defining interpreter like `FnHome`
(implementation-note 25) already lets a `func` value do, so a captured closure
resolves its own namespaces.

**The one teachable surprise** is the snapshot: a captured variable is frozen at
creation, so a later mutation of the outer variable is invisible inside the closure
- the identical mental model `spawn` already teaches, deliberately consistent
rather than the JS / Python live-binding behavior.

**What it buys, beyond inline lambdas.** Real currying captures
(`makeAdder(1)(2)` today is call-through-expression syntax, not capture - this is
what would make it capture). And it lets the module ecosystem replace by-name
dispatch with closures that close over their state - a `webapi` guard could capture
the `Api` directly instead of a name plus a one-line shim, `args.dispatch` could
take closures - simplifying those APIs. Whether to *migrate* those modules is a
separate follow-on, not part of this draft.

Syntax and semantics to settle at graduation: the literal spelling
(`func(params) { body }`, mirroring a named `func` declaration without the name),
whether parameter types are required (yes, for consistency with functions), no
declared return type (consistent), and how `defer` / `spawn` inside a closure body
behave.

**Requires:** `M24.2` (the `func` value, `KindFunc`, and `CallValueExpr` this
extends). Reuses the `spawn` snapshot and the resolver / borrow carve-outs already
in place; no new dependency.

#### DRAFT#32 - Variadic user functions

Built-ins take a variable number of arguments (`io.printf`, `path.join`,
`os.run`'s optional stdin, `meta.call`); a user function cannot. Its
arguments are fixed in number and type, so a function that wants "any number
of these" takes a list (`sum([1, 2, 3])` instead of `sum(1, 2, 3)`), and a
deck cannot offer a `printf`-like API at all: `@mplx/texttransform` ships a
`t(value, spec)` transform that composes with the real `io.sprintf` for
exactly this reason.

**Typed variadics fit the language.** A last parameter
`func sum(...xs as list of int)` collects the remaining arguments into a
`list of int` (zero or more); inside the body it is an ordinary list, so
value semantics, borrowing and the frame pool are unchanged. Each argument
is checked against the element type at the call site, as any argument is,
and `jennifer lint`'s arity check (L109) already understands variadic
counts for built-ins through the `libsig` table. Spreading an existing list
into the call (`sum(...$xs)`) is the matching sugar.

**Mixed types are the hard part.** `printf` is useful because its
arguments differ in type, and Jennifer has no `any` (rejected, see
[rejected.md](technical/rejected.md) "A language `any` / top type"), and
lists are homogeneous. Ways to get there, to weigh at graduation:

- **Homogeneous only.** Ship `...xs as list of T` and stop; a mixed call
  stays a list of an enum the caller builds. Simple, and no type-system
  change.
- **A sum type as the element.** `...args as list of Arg`, where `Arg` is a
  user enum (`Arg.Int{v: 1}`); the call site writes the wrapping, so this
  is mostly sugar for building the list.
- **Implicit wrapping into a library enum.** A built-in `fmt.Value` enum
  (one variant per primitive) that the call site fills in automatically for
  a `...args as list of fmt.Value` parameter. This is what a user-level
  `printf` needs, but it is an implicit conversion, which the language
  otherwise avoids.

Settle at graduation: the spelling (`...name as list of T`, last parameter
only), whether a variadic may follow optional parameters (there are none
today), how `func` values and `meta.call` pass variadics, and the
cross-module rebuild cost of a list of a module's own structs.

**Requires:** none for the homogeneous form; the mixed form needs a
decision against the `any` rejection first.

#### DRAFT#33 - Generic functions (type parameters)

The built-ins are generic: `lists.sort`, `lists.map`, `lists.reverse` and
`len` take a list of any element type. A user function cannot be. Every
parameter names one concrete type, so an algorithm over "a list of things
that can be ordered" is written once per element type, or for one type
only. Rosetta Code's `Sorting algorithms/Quicksort` solution in
`jennifer-language/rosetta` sorts `list of int` and nothing else, where the
Go, Rust and Haskell entries sort any ordered type. The same holds for
binary search, a max / min helper, a stack or queue, and a `.j` module's
containers: `ringbuffer` stores strings only and tells callers to
serialize anything else.

**Type parameters, bound per call.** A function declares its type
parameters, each with an optional constraint, and uses them in its
parameter types and in the types of the body's own `def`s:

```text
func quicksort[T ordered](xs as list of T) {
    if (len($xs) <= 1) {
        return $xs;
    }
    def pivot as T init $xs[len($xs) // 2];
    def less as list of T;
    ...
}

def words as list of string init quicksort(["pear", "apple", "fig"]);
def nums as list of int init quicksort([3, 1, 2]);
```

Each call binds `T` from its arguments: the first argument whose parameter
type mentions `T` fixes it, and every other occurrence must match that type
exactly (no int / float promotion across a binding). A call where no
argument binds `T` (only an empty `[]`) names it: `quicksort[int]([])`.
There is still no declared return type; the caller's `def` checks the
result, as for every function.

**This is not `any`.** The rejected top type (see
[rejected.md](technical/rejected.md) "A language `any` / top type") lets one
variable hold values of different types and dispatches operators on
whatever arrives. A type parameter stands for one concrete type per call:
`list of T` stays homogeneous, a `def x as T` holds only that type, and
inside the body `T` allows only what its constraint names. "You always
declare what you store" still holds; the declaration says "the type the
caller chose".

**A small, closed set of constraints, no user-defined interfaces.** Methods
on structs are rejected, so there are no traits to hang constraints on.
The language's operators define the constraints instead:

- *none*: store, copy, pass, return, put in a list or map value, and
  `len` / iterate / index when the parameter type is `list of T`.
- `ordered`: adds `<` `<=` `>` `>=`, so `int`, `float` or `string` (the
  types the ordering operators accept).
- `number`: adds arithmetic, so `int` or `float`.

The body is checked against the constraint when the program loads: `<` on
an unconstrained `T` is a load-time error naming the constraint it needs,
as an unknown struct field is today, not a runtime error when the line
first runs. `lists.sort`'s `bool` ordering stays a library feature; the
operators do not order `bool`.

**It fits the interpreter.** Parameter types are checked at each call
already, so binding `T` is one more step at the same place: the call
resolves `T`, and `def x as T` / `list of T` in the body resolve through
the call frame. A `func` value of a generic function binds at each call
through the value, since the `func` type carries no signature. Across a
module boundary `T` binds to the caller's type, a consumer's struct keeping
its `alias.Struct` identity; the rebuild cost for a module's own structs
is unchanged.

**What it buys.** One quicksort, merge sort or binary search for every
ordered type (Rosetta Code has dozens of `Sorting algorithms/*` tasks), a
`max` / `clamp` / `sum` over `int` and `float` in user code, and modules
that hold any element type. With [`DRAFT#28`](#draft28---anonymous-function-literals-with-value-capture-tier-2-of-first-class-functions)
a generic higher-order helper (`compose`, `memoize`) becomes writable at
all.

Settle at graduation:

- the spelling: `func f[T ordered](...)` (Go's form; brackets are free in
  a function header) against `func f(...) for T ordered`;
- whether `==` / `!=` need a constraint, which depends on every value kind
  comparing (`func`, `task` and `channel` values);
- map keys of type `T` (which key types maps allow today);
- `lint`, `jennifer fmt` and the `libsig` table, and how docblocks write a
  type parameter (`@param xs {list of T}`);
- generic structs and enums (`def struct Stack[T] { items as list of T }`,
  used as `Stack of int` like `list of int`), which would let `ringbuffer`
  and an Option / Result enum be generic. A follow-on, not part of this
  draft: functions come first.

**Requires:** none. Composes with `DRAFT#28` and `DRAFT#32` (a typed
variadic `...xs as list of T`) but needs neither.

#### DRAFT#34 - Struct literals that leave fields out (user and deck structs)

**Open: to be discussed before any design is chosen.**

A struct literal names every field: there are no field defaults, so a reader
sees the whole value and a forgotten field is a load-time error. Library
structs and the bundled modules' structs are exempt and may leave fields
out, because they change under the program on upgrade (reasoning in
[design-decisions.md](technical/design-decisions.md) "A library struct
literal may leave fields out"). User structs and every other module's
structs, decks included, keep the full rule.

Two places where the rule costs more than it catches:

- **Options structs.** Jennifer has no optional or named parameters, so the
  idiom is a struct of options. Its caller cannot write only the options it
  changes: `SortOptions{reverse: true}` fails for the missing fields, so a
  module offers a `defaults()` function and the caller assigns fields one by
  one (Rosetta Code's `Optional parameters` and `Named parameters` show
  both). The literal that reads best is the one that is refused.
- **Decks.** A deck is vendored, but a consumer updates it on its own
  schedule, as it updates the interpreter. A deck that adds a field to an
  exported struct breaks every consumer literal of it, at load, for a
  change that is no break for code that reads the struct: the reason the
  library exemption exists, one level up. Today every added field is a
  breaking (major) release of the deck.

Directions to weigh:

- **Keep the rule** and document the `defaults()` + assignment idiom as
  the options pattern; decks bump their major version for an added field.
- **Field defaults in the declaration** (`reverse as bool init false`): a
  literal may leave out a field that has one, a field without one stays
  required. The author decides per field, the reader finds the default at
  the type, and "forgot a field" is still caught where no default exists.
- **Extend the exemption to deck structs** (a module under `vendor/`), as
  for bundled modules: decided by where the file lives, not by syntax.
  Fixes the upgrade break, not the options idiom.
- **An update literal** (`SortOptions{...defaults(), reverse: true}`):
  copy a value and override some fields. Explicit about where the rest
  comes from; no defaults in the type.

These combine (defaults for the options idiom, the vendor exemption for
upgrades). Settle with them: lint `L113`'s rules, how `jennifer fmt` and the
docblock `@field` tags show a default, whether a default may be any
constant expression or only a literal, and what `def x as T;` (the zero
value) means once a type has defaults.

**Requires:** none.

### Concurrency

#### DRAFT#25 - Go-side concurrent HTTP serve loop

The `web` framework dispatches each request into an entry-program handler `func`
value (called in its home context), and `web.serveOn` runs that dispatch in a
`.j` loop. Once
`web` handles requests concurrently (each wrapped in a `spawn`), an optional
refinement is to move only the **accept + bounded-worker-pool loop** into Go, as
an `httpd` primitive (e.g. `httpd.serveLoop(server, handlerName)`, or a small
engine-owned pool) that calls `Interpreter.CallHostWith` per request. Routing,
middleware, and `web.Context` stay in `.j`.

Why it might pay: a worker pool with a tunable ceiling is more natural in Go than
expressed as `.j` `spawn`s; back-pressure / queue-depth limits live where the
sockets already are (`httpd` already bounds `maxInFlight`); and the hot accept
loop stops paying tree-walker overhead per request. Why it is only a refinement,
not a fix: a handler is still a `.j` function invoked on the shared host
interpreter, so making that invocation race-safe is the actual prerequisite and
does the real work - this only relocates the loop. Keep `web` a `.j` module
either way: dogfooding is a first-class goal, and with no closures a handler is
always a named top-level function, so a Go rewrite of the router would buy little
and lose test surface.

### Tooling

#### DRAFT#31 - `jennifer fix`: rewrite code across a breaking change

After 1.0, a breaking API change ships with a rewrite rule, and `jennifer
fix` applies it to a user's code, as `go fix` does. Before 1.0 every caller
is in the project's own repos, so M28.41 (the name-first argument order) was
migrated with a one-off script instead; its problems are the design input
here.

- **Rules per release.** Each release that breaks an API carries a table of
  rules: rename a function (`crypto.hmacEqual` -> `hash.equal`), reorder its
  arguments by arity, or replace a call with a snippet. A rule applies only
  to code whose declared floor (`# pragma-jennifer-version` or the
  manifest's `engines.jennifer`) is below the release that introduced it,
  and `fix` raises the floor afterwards. That makes a run idempotent: an
  argument reorder applied twice is not, and the one-off script needed
  heuristics (string literals in the name slot) to avoid swapping a call
  back.
- **Resolved through the parser, not text.** Namespaces come from `use`, and
  module aliases from `import ... as`, including the imports a test overlay
  (`x_test.j`) and an `include`d file inherit from their parent: missing
  those was the script's most common miss. Calls inside strings are never
  touched. Moving arguments needs each node's end position, which the AST
  does not record today (only starts).
- **Beyond code.** Docblock examples and the Jennifer code blocks in
  Markdown (fenced `jennifer` blocks and inline code) are rewritten too,
  reported separately so a reviewer checks prose that names the old form.
- **Output.** `--diff` by default, `-w` to write; then `jennifer fmt` on the
  touched files. A call the rules cannot place (wrong arity, an ambiguous
  name) is listed with its position, never guessed.

**Requires:** none. Worth having before the first breaking change after
1.0.

#### DRAFT#35 - Debug Adapter Protocol: debugging from the editor

`jennifer debug` (M30.23) is a terminal debugger. Editors speak the Debug
Adapter Protocol (DAP, JSON over stdio): one adapter gives VS Code, Zed,
Helix, Neovim and others breakpoints in the gutter, stepping, the variables
and call-stack views and a debug console, without editor-specific code.

**The design that fits:** `jennifer debug --dap` runs the same engine as the
terminal debugger behind a DAP front end: `initialize`, `launch` (program,
args, sysmoddir, vendor), `setBreakpoints` / `setFunctionBreakpoints` /
`setExceptionBreakpoints` (by error kind), `threads` (the main program and
each `spawn`), `stackTrace`, `scopes` / `variables` (locals, globals; a struct,
list or map expands lazily), `evaluate` (the REPL evaluator), `continue` /
`next` / `stepIn` / `stepOut` / `pause`. The adapter is part of the
interpreter, so no separate install and no version skew.

**What it needs beyond M30.23:** a variable reference for each compound value
shown (stable for one stop, as DAP requires), source references for system
modules (sent by content when the editor cannot open the path), and the
editors' launch configuration: a VS Code extension snippet and Zed / Helix
debugger config, next to the planned tree-sitter grammar in
`jennifer/editors/`.

Settle when graduating: conditional breakpoints and logpoints, attaching to a
running server (a port, off by default) versus launch only, and whether
`setVariable` is offered at all.

**Requires:** M30.23.

### Distribution

#### DRAFT#30 - `jennifer pack`: single-file executables

Ship a program as one executable without a Go toolchain:
`jennifer pack app.j -o app` copies an interpreter binary and appends a
payload. At startup the binary looks for a trailer (magic + length) at the
end of its own executable; when there is one, it runs the payload instead of
the CLI. This covers most of what "compile it" is asked for (one file to
deploy, no install, no module path), with the interpreter's semantics,
errors and both builds unchanged.

- **Payload.** The entry program and every file it reaches, resolved at pack
  time: `import`ed and `include`d files, bundled modules and vendored decks.
  Imports resolve inside the payload only, never against the host's module
  directory, so the app does not change when the host's install does. Source
  text at first; the binary AST cache (`.jc`, see
  [Runtime and tooling](#runtime-and-tooling)) can replace it later.
- **Checked at pack time.** The version and capability pragmas of every
  packed file are checked against the target binary, so a `net` program
  packed into `jennifer-tiny` fails then, not at first run.
- **Cross-platform.** `--runtime PATH` packs into another platform's
  `jennifer` or `jennifer-tiny` binary.
- **CLI.** A packed binary takes no interpreter flags: every argument reaches
  `os.ARGS`.
- **Not obfuscation.** The source is readable in the file. Appended data also
  invalidates code signatures (macOS, Windows Authenticode), so sign after
  packing.

**Requires:** none. Shares the `jennifer build` command family with
build-time library selection and compile-to-Go (both under
[Runtime and tooling](#runtime-and-tooling)); design the command once.

#### Termux packages (Android)

Jennifer scripts on an Android phone or tablet through
[Termux](https://termux.dev), the terminal and Linux userland app. Three
steps, in order:

1. **Our own Termux packages.** CI builds `.deb` packages for Termux's four
   architectures (aarch64, arm, x86_64, i686) with its prefix
   (`/data/data/com.termux/files/usr`), shipped as release assets or from a
   small apt repository. **Requires:** M29.4 (install paths set at build
   time), M29.5 (DNS without `/etc/resolv.conf`) and M29.6 (more release
   architectures), plus a test on a real device.
2. **The Termux User Repository (TUR,
   [termux-user-repository/tur](https://github.com/termux-user-repository/tur)).**
   The community repository for packages the official one does not take; a
   `build.sh` in the Termux build system, built from the release tarball.
   Its own rules are to be read before submitting.
3. **The official repository** (`termux/termux-packages`), later. Its
   policy (`CONTRIBUTING.md`, "Packaging policy") asks for "active,
   well-known projects", an OSI licence, under 100 MiB, no duplicate
   functionality, a package request issue first, then an `addpkg(main/
   jennifer)` pull request; AI assistance must be disclosed with an
   `Assisted-by:` trailer. Jennifer meets everything but "well-known" today,
   so this waits for a user base (realistically after 1.0).

A `termux` deck wrapping the `termux-api` commands (notifications, sensors,
clipboard, battery, location; capability `exec`) would make it worth having.

### Embedding, WASM, and sandboxing

#### DRAFT#1 - Public interpreter API for third-party embedding

Extract the interpreter core out from under `internal/` and expose a
documented Go-side surface so external programs can embed Jennifer. Today
`internal/interpreter`, `internal/parser`, `internal/lexer`, and
`internal/lib/*` are unreachable from any module that isn't
`jennifer-lang.dev/jennifer` - Go's `internal/` visibility rule is a
compile-time barrier, and no submodule / require / replace workaround exists;
embedding is impossible without a restructure.

It comes ahead of the WASM runtime (`DRAFT#2`) because a Go-side embedding
API is a strictly smaller change (repository restructure, no new external
dependency), it unblocks the most immediate embedding scenarios (scripting
slot in a Go host, LSP / formatter tooling, test harnesses), and it does
not foreclose WASM (`DRAFT#3`) - a plugin surface can layer on the same
`pkg/` facade once Wazero (or similar) is in play.

**Layering first.** A facade that re-exports `internal/` would leave the
libraries welded to the evaluator. Split the evaluator instead, in two steps:

1. **`internal/runtime`** (scheduled as M30.24 in
   the [roadmap](roadmap.md)), a refactor with no public promise. It holds what
   the libraries use: `Value`, its kinds and constructors, equality, deep
   copy and display (most of `value.go`); errors and positions; the builtin
   registry; task / channel / waker primitives; and `BuiltinCtx`, with
   re-entry (`Invoke`, `CallByName`, `Cancel`, `MethodParams`) behind a
   **`Host` interface**. The evaluator (`interpreter.go`, environments,
   module loading, `spawn` capture) stays above it and implements `Host`. The
   About 100 library files that import `interpreter` use about 40 of its symbols,
   mostly `Value` / `Null` / `BuiltinCtx` / the constructors, so most of the
   change is a rename; type aliases at the old names let it land in stages.
   Two couplings need design:
   - **Types live in the parser.** `Value.ElemTyp` / `KeyTyp` / `ValTyp` are
     `*parser.Type`, and the libraries call `parser.PrimitiveType` about 150 times.
     The type descriptors move to their own package below both the parser and
     the runtime, so the runtime does not carry the front end.
   - **Function values point into the AST.** `Value.Fn` is a
     `*parser.MethodDef` and `Value.FnHome` an `*Interpreter`. The runtime
     gets an opaque function reference that each engine fills in.

   Only callbacks go through `Host`; `Value` operations stay concrete calls,
   because they are the hot path. Library tests can then run without
   `interpreter.New()`.
2. **`pkg/`**, the public surface below: a thin layer over
   `internal/runtime` and the libraries.

Step 1 also serves `DRAFT#17` (a VM is a second `Host`) and compile-to-Go
(generated code is a third). Land it after the M28 items that reshape
`Value`, so the rename does not collide with them.

**Concretely.** Add a `pkg/` top-level (working name; the final path
settles at the start of this work):

- `pkg/interpreter` re-exports `Interpreter`, `Value`, error types, and the
  `Install(in *Interpreter)` registration API that every stdlib library
  already uses, from `internal/runtime` and the evaluator. The `internal/`
  packages stay as the implementation; `pkg/` is the stable facade with
  semver-covered surface once we ship 1.0. What it publishes of `Value` is
  the accessor API of M29.11: constructors (`IntVal`, `StringVal`, `ListVal`,
  `GenericListVal`, `MapVal`, `StructValOf`, `InternIdentity`) and getters
  (`v.Int()`, `v.List()`, `v.Identity()`, ...). The packed fields stay
  unexported, so the layout can change again without breaking embedders.
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

- **Duplicate library `Install` at the Go level is rejected** (a repeated
  `use NAME;` is a no-op, but a second library claiming a namespace is a
  conflict). A host installing `pkg/lib/math` and then its own shim that also
  claims the `math` namespace fails at the second `Install` call instead of
  being silently overlaid.
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
Step 1 is M30.24, after the entries that change `Value` (M30.3,
M30.5, M30.6, M29.14, M30.8, M28.37); step 2 once the core library / module
surface has settled, so the `pkg/` facade is stable.

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
- **Sequenced after the cheap win.** Once hot paths have moved into Go primitives, what remains for a VM
  is the residual CPU-bound `.j` (recursion, business-logic loops) no Go
  primitive covers. Pursued only when a benchmark shows that residual is
  a real workload's bottleneck - not on spec.
- **Composes with arena.** The per-frame arena allocator is
  an independent memory-side optimization that pairs naturally here, since a
  bytecode VM restructures allocation anyway - but neither depends on the other.
- **Another `Host`.** With `DRAFT#1` step 1 (`internal/runtime`) done, the VM
  implements the runtime's `Host` interface next to the tree-walker, and the
  libraries do not change. Without it, the VM has to live inside the
  evaluator package.
- **Expect a modest gain.** The tree-walker already resolves slots, folds
  constants at parse time and has the in-place int / bool fast path, the
  usual first wins of a VM, and Go has no computed goto, so dispatch is a
  `switch`. Estimated 1.5-3x on loop-heavy `.j`, not 10x. Before committing,
  write a throwaway VM loop for `fib` and `primes` from `examples/benchmark.j`
  and compare it with the tree-walker and with a hand-translated Go version
  (the compile-to-Go floor, see [Runtime and tooling](#runtime-and-tooling)).
- **Its IR feeds compile-to-Go.** The lowered form (resolved slots, explicit
  copies and borrows, error positions, depth accounting) is the input a Go
  code generator wants. Built first, it makes that transpiler a smaller,
  later step.

Copy-on-write for compound Values is **not** part of this: it was tried
(shared-marker COW, reverted as inert) and the write-through variant is rejected
for reintroducing shared mutable state - see
[technical/rejected.md](technical/rejected.md).

**Requires:** none hard; `DRAFT#1` step 1 (`internal/runtime`) first keeps the
VM outside the evaluator package.

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
- **`encoding` - the harder codecs.** The single-byte character codecs, the
  UTF-16 / UTF-32 family and the binary-to-text formats ship; the remainder,
  picked up only when a real program needs one: variable-width Asian encodings
  (`Shift-JIS`, `Big5`, `GB2312`, `GBK`, `GB18030`, `EUC-JP`, `EUC-KR`), each
  a state machine with variant / ambiguity edge cases, a whole piece apiece;
  and `UTF-7` (mail transport; `quoted-printable` already ships as a general
  codec).
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
  which needs image decoding plus 1-bit dithering / thresholding. The `images`
  library covers that part (`decode`, `fit`, `dither`, `pixels` -> RGBA rows),
  so what is left is packing those rows into each dialect's raster encoding,
  an encoder branch per dialect. Until then, `label.image` (by reference)
  covers the stored-logo case.
- **SQLite (`sql` engine backend).** SQLite stays parked here, and the reason
  is not integration effort. SQLite has a pure-Go `database/sql` driver, `modernc.org/sqlite`,
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
  reach the embeddable binary - although a local, file-based store is what a
  minimal embedded target would most want.
- **FastCGI.** A `fcgi` library beside `httpd`, so Jennifer can host CGI /
  FastCGI workloads end to end.
- **gRPC (HTTP/2).** Protobuf itself is planned (the M30.14 wire library and
  the M27.7 `protobuf` deck in [decks.md](decks.md)); gRPC is not, because it runs over HTTP/2:
  binary framing, HPACK header compression, per-stream flow control and
  trailers carrying the status, none of which `http` (HTTP/1.1) has. That is
  a core transport of its own, weeks of work and a large test surface, and
  it is what M23.11 meant by choosing JSON-RPC over gRPC. Two cheaper steps
  come first and need no HTTP/2: **gRPC-Web** and the **Connect** protocol,
  both of which carry protobuf (or ProtoJSON) over HTTP/1.1 POSTs, so a deck
  could call Connect / gRPC-Web services on M30.14 plus the `protobuf` deck.
  Full gRPC unblocks Firestore realtime listeners (the `firebase` idea
  below), most Google Cloud streaming APIs and in-cluster services; it would
  need M30.9-style streaming reads as well, for server streaming.
- **i18n (CLDR formatting).** Locale-aware case folding, collation, number /
  date formatting, and BiDi - the CLDR-data-backed half of i18n, distinct from
  the message catalogs + `%name%` interpolation already shipped as the `intl`
  library (`M20.4`). Gated on the CLDR-data binary-size question (likely an
  optional library after the WASM runtime, so locale tables aren't baked into
  every build).

### Decks

Deck ideas that are not planned yet are in [decks.md](decks.md) ("Ideas"),
next to the deck roadmap (M27).

### Runtime and tooling

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
  of profiles (default / minimal), not 2^N. `meta.hasCapability` already
  answers the same question for the `net` / `exec` / `sql` capabilities. Complementary to, not a
  substitute for, the module system: `.j`-level extensibility (community /
  uncommon libraries writable in Jennifer) is the module system's job with
  zero binary cost; build-time selection is only for the curated Go-level
  core.
- **Foreign-function interface (user-reported: "Jennifer has no FFI").** A
  dedicated FFI was turned down as one feature (see "FFI as a single milestone"
  in [rejected.md](technical/rejected.md)): calling Go is what every system
  library already is, OS surface belongs in the topic libraries, C ecosystems
  are meant to arrive sandboxed through WASM (`DRAFT#2` / `DRAFT#3`), and
  driving Jennifer from a host is `DRAFT#1`. The complaint still names a real
  gap: **native code a user writes cannot reach a stock binary** without
  forking the interpreter, and WASM is not there yet. Two cheap answers short of
  an FFI keyword: (1) **out-of-tree Go libraries in a custom build** - a
  documented registration hook (an `Install(in)` the build picks up by import,
  `xcaddy`-style) so `jennifer build --with example.com/mylib` produces an
  interpreter that also carries `use mylib;`, compile-time only (Go's `plugin`
  package and `dlopen` are out for the reasons in the build-time library
  selection entry above, and so is cgo for `jennifer-tiny`); (2) a **subprocess
  bridge** - a documented line-delimited JSON protocol over `os.spawn` stdin /
  stdout, plus a small helper module, so any language's code is callable today
  at process-boundary cost. (1) shares its build plumbing with build-time library
  selection and its registration surface with `DRAFT#1`; (2) needs nothing new
  in the interpreter. When users ask for FFI, check which of the four they mean
  before designing anything.
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
  dynamism (untyped function returns, `meta.call` by name, opaque `json.Value`): a
  naive transpile keeps boxed tagged-union values, so it beats the tree-walker
  but is not C-speed without type-specialisation work. Relates to `DRAFT#17` (the
  in-process bytecode VM is the cheaper dispatch win) and `DRAFT#2` (WASM, the
  other compiled target); a hand-rolled LLVM backend is the most expensive, least
  differentiated version of this idea and stays a non-goal. (`jennifer-tiny`
  already reaches native through LLVM transitively: TinyGo is an LLVM-based Go
  compiler.)
  - **Cost is parity, not syntax.** Generated code must keep catchable,
    positioned runtime errors (overflow, index, key) and the call-depth guard:
    native Go recursion overflows the stack fatally, and `jennifer-tiny` runs on
    a fixed 4 MB stack without `recover()`, so it needs its own depth counter
    and an error check after almost every operation. It must also repeat the
    M28 copy elision (borrow, move on last use, owned results), or it is
    correct but slow. Every later language change then lands twice, and
    profile / coverage / `jennifer test` / lint / the REPL stay interpreter-only.
    The golden examples and module overlays are the conformance suite.
  - **Blocked by `internal/`.** A generated program outside the
    `jennifer-lang.dev/jennifer` module cannot import `internal/...`, not even
    `internal/runtime`, so this needs `DRAFT#1` step 2, the public `pkg/` (or a
    build inside a copy of the source tree). Step 1 gives the generated code
    its target: it implements `Host` over the runtime and calls the libraries
    directly. The user
    also needs a Go or TinyGo toolchain and the runtime sources at build time.
  - **Sequencing.** After `DRAFT#1` and `DRAFT#17`, generating Go from the
    bytecode IR (resolved slots, explicit copies) rather than from the AST.
    After the 1.0 language freeze, not while M28 still changes the value
    model. Before committing, hand-translate `fib` and `primes` from
    `examples/benchmark.j` into boxed-`Value` Go and time them against the
    interpreter.
  - **Side effects.** TinyGo's WASM targets would give Jennifer-to-WASM nearly
    for free (`DRAFT#2`); a compiled binary has to carry the capability
    pragmas' checks; direct calls let the Go linker drop unused libraries
    (build-time library selection). Single-file deployment alone is
    `DRAFT#30`, far cheaper.
- **Advanced scheduling knobs.** CPU affinity, work-stealing pool sizing,
  NUMA awareness, `GOMAXPROCS`-equivalent runtime tuning. Runtime-config
  surface for the spawn scheduler, not new language features. Ships when a
  real use case forces it (the default - "let Go's scheduler decide" -
  handles every workload considered so far).
- **Profiler: heap-per-position metric.** Out of scope for now: `--allocs`
  already proxies value-copy churn, and true per-position RSS needs
  `runtime.ReadMemStats` sampling, which is coarse under TinyGo.

### Wild ideas

- **Inline assembler.**
