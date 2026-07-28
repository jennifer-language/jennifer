# Rejected features

Proposals that were considered and explicitly turned down. Recorded here so
the same ideas don't come back as fresh suggestions next session.

## `finally` clause (`try { } finally { }`)

Considered as the cleanup construct, alongside `defer`, when resource-holding
libraries (`fs`, `term`, the planned `sql`) made "release this no matter how the
block exits" a real need.

Rejected in favor of `defer` (which shipped) because:

- **Java's footgun.** A `return` or `throw` inside a `finally` silently swallows
  the try's exception or overrides its return value - a notorious, hard-to-spot
  bug. `defer CALL(args);` takes a *single call*, not a block, so that hazard is
  unrepresentable rather than merely discouraged.
- **`defer` co-locates acquire and release** (`open` then `defer close` on the
  next line) and scales to N resources by LIFO unwinding, where `finally` nests
  into pyramids and separates open (top) from close (bottom).
- **It fits the language.** With value semantics and handle-into-registry
  resources, `defer`'s "capture args now, run the call at block exit" needs no
  closures (which Jennifer lacks); the model falls out of value semantics.
- **One-obvious-way.** Shipping both `finally` and `defer` would be two
  constructs for one job. Student familiarity is `finally`'s only real edge, and
  `defer` is trivially teachable ("cleanup that runs when the block exits").

See [M20.7](../milestones.md) for the `defer` design.

## Increment / decrement (`++`/`--`)

Considered: postfix `$i++` and prefix `++$i`.

Rejected because:

- The pre/post distinction is a real footgun - the two forms differ only
  in expression context, which is exactly where bugs hide. Swift removed
  `++`/`--` in version 3 (2016) for this exact reason.
- The savings are tiny (three characters) and only apply to `+1` / `-1`.
- Python rejected them from the start and the language hasn't suffered.
- `$i = $i + 1;` is verbose but unambiguous; the readability cost is small.

## Compound assignment (`+=`, `-=`, `*=`, `/=`, `//=`, `%=`)

Considered as an alternative to `++`/`--`.

Rejected because:

- Several operators to add and remember for marginal ergonomic gain over
  `$x = $x + E;`.
- Slippery slope: would we also need a string-concat `+=`? An `and=`?
  Where does the family end?
- Keeping a single assignment shape (`$x = EXPR;`) makes source code uniform
  and matches Jennifer's "one way to do each thing" stance.

## Chained assignment (`$x = $y = 0;`)

Considered as a shorthand for setting several existing variables to one value,
alongside the other assignment sugar above.

Rejected because:

- **Same family, same verdict.** It is one more "assignment sugar" for marginal
  payoff, so declining it keeps company with `++`/`--` and the `+=` family and
  preserves the single assignment shape (`$x = EXPR;`).
- **The common case is already covered.** Jennifer variables are born with a
  value (`def x as int init 0;`), so initializing several to the same value is a
  declaration concern, not an assignment one. Chaining would only help
  *re-assigning* several already-declared variables at once - a narrow case that
  `$x = 0; $y = 0;` handles in one extra line.
- **Assignment is a statement, not an expression.** `$y = 0` has no value, so
  `$x = $y = 0;` does not parse today; supporting it is a real grammar addition,
  not free sugar. The two ways to add it both lose: the safe Python-style form
  (special `lvalue (= lvalue)* = EXPR;` grammar, RHS evaluated once) is pure sugar
  for the above non-need, and the C-style form (assignment-as-expression) is a
  bigger semantic change that reopens the `if ($x = 0)` typo-for-`==` footgun
  Jennifer avoids by having no assignment-expression at all.

The chosen solution: **initialize at declaration (`def ... init`) or assign per
line (`$x = 0; $y = 0;`).** (Note: had it existed, value semantics would give
`$x = $y = $someList` independent copies, sidestepping Python's shared-alias
surprise - a cleaner behavior, but still not enough to justify the sugar.)

## Ternary operator (`cond ? a : b`)

Considered as a way to write expression-position conditional
selection without bouncing through an intermediate variable:

```jennifer
# verbose - status quo
def grade as string;
if ($score >= 90) { $grade = "A"; } else { $grade = "B"; }

# what a ternary would let us write
def grade as string init $score >= 90 ? "A" : "B";
```

Python's `a if c else b` and the C-family `c ? a : b` cover the
same need.

Rejected because:

- **Parallel API to `if`/`else`.** Stance #1 is strict in Jennifer
  (`++` was rejected even though it's syntactically distinct from
  `$i = $i + 1`). "Pick value A or B based on a condition" is one
  operation; `if`/`else` is the canonical spelling. Adding a second
  syntax form would be the same kind of parallel API that `++` and
  `+=` were rejected for.
- **Closest stance neighbor agrees.** Go is the only mainstream
  language built on a "small, explicit" stance comparable to
  Jennifer's, and Go's designers rejected ternary explicitly: "The
  reason `?:` is absent from Go is that the language's designers
  had seen the operation used too often to create impenetrably
  complex expressions. The if-else form, although longer, is
  unquestionably clearer." Real Go code lives without it; Jennifer
  programs will too.
- **Nesting is the footgun.** `a ? b : c ? d : e ? f : g` is
  parseable but unreadable. Once shipped, the ternary will end up
  in code like this and there's no way to take it back. Rejecting
  upfront saves us the migration.
- **Verbose form has search/step affordances.** A multi-line
  `if`/`else` is grep-friendly, debuggable, and editable line by
  line. The condensed expression form is none of these. For one
  saved line of source across the whole program, the cost is too
  high.
- **Escape hatch already exists.** A user who really wants
  ternary-shaped code can write a one-line helper
  `func pick(c as bool, a as int, b as int) { if ($c) { return $a; } return $b; }`.
  Both arguments evaluate eagerly (no short-circuit), but for the
  cases where ternary is genuinely better this is fine.

The "make `if` itself an expression" alternative (Rust-style:
`def x init if (c) { 1 } else { 2 };`) was also considered and
deferred indefinitely. It's the cleaner long-term answer if
expression-position conditionals ever become genuinely needed -
it extends an existing construct rather than introducing a new
operator - but it's a much larger change (blocks have to evaluate
to their last expression; type-checking gets harder; `if` without
`else` and `if` containing `return`/`exit` need defined semantics).
We don't owe ourselves that complexity for "save one line of
source" ergonomics.

## Range literal syntax (`[1..9]`)

Considered as a shorthand for constructing a `list of int`
sequence:

```jennifer
# what range literal would let us write
def xs as list of int init [1..9];
for (def i in [1..len($items)]) { ... }
```

Borrowed from Haskell / Kotlin / Ruby's syntactic form.

Rejected because:

- **Parallel API to the `[1, 2, 3]` list literal.** Two syntaxes
  for "construct a `list of int`" - same family as `++`, `+=`,
  and ternary. Stance #1 has rejected every previous instance.
- **Hides materialization cost (stance #2).** `[1..big_number]`
  silently allocates a million-element list. The explicit
  `for (def i init 1; $i <= n; ...)` loop iterates without
  materializing, and the cost shows up at the call site instead
  of being buried in a two-character `..` operator. A library
  function call (`lists.range(...)`) makes the allocation
  visible too; the literal form does not.
- **New single-purpose operator.** Jennifer hasn't introduced an
  operator that works in exactly one context before; every other
  operator (`+ - * / // % & | ^ ~ << >> < > <= >= == and or not`)
  works across multiple types or contexts. `..` only does
  integer ranges. Once it ships we'd have to defend "why doesn't
  `[1.0..9.0]` work? why not `["a".."z"]`?" - each extension
  another design discussion.
- **Pattern set elsewhere.** Jennifer ships `lists.head` /
  `lists.tail` instead of Python `xs[:n]` / `xs[n:]` slicing
  syntax; `strings.substring` instead of `s[i:j]`; index-write
  `$xs[i] = v;` instead of pythonic `$xs[i:i+1] = [v]`. Library
  functions over syntax for collection operations is the
  established Jennifer pattern.

The chosen rule: ship `lists.range(start, end)` (M15.0) as the
canonical way to allocate an integer sequence. Use site reads as
"this allocates a list" instead of hiding behind two characters
of punctuation. See
[milestones.md > M15](../milestones.md#m15---foundational-libraries--first-public-release-compacted)
(M15.0).

## printf data-transformation modifiers

Considered during the M7 format-verb-modifier design: extending the
modifier list with options that transform the *value* rather than its
*visual representation*. Examples from the original draft:

- `%s|case=upper|lower|title|snake|kebab|camel|pascal|leet`
- `%s|slice=START:END`
- `%s|md=italic|bold|code|strike|header1|header2|...|link(URL)|...`
- `%s|md=table(...)` and the wider `%a|json=*`/`%a|xml=*`/`%a|yaml=*`
  family - serialisation modifiers for the aggregate verb. `%a`
  itself shipped in M11 with presentation-only modifiers (`sep`,
  `kv`, `open`, `close`, `depth`, `null=skip`); the
  `json=`/`xml=`/`yaml=` family remains rejected as data
  transformation that belongs in dedicated libraries.
- `null=sql` (SQL-specific spelling of `NULL`)

Rejected for the modifier system because:

- **Mission creep.** The guiding rule for printf modifiers is "shape
  the printed representation, not the value." `%d|base=2` is presentation
  (the int 5 becomes the glyph sequence `101`). `%s|case=upper` is data
  transformation (`"abc"` becomes `"ABC"` - a different string value).
  Once one transform is in, every string/number/aggregate manipulation
  becomes a candidate modifier and the format-string spec swallows the
  rest of the standard library.
- **Parallel API to the libraries.** `%s|case=upper` is already
  `strings.upper($s)`. Two ways to do the same thing breaks Jennifer's "one
  way per thing" stance and means every future string helper has to
  decide whether to ship as a function, a modifier, or both.
- **Domain leakage.** `null=sql` picks one application domain to bake
  into the formatter. `null=literal("NULL")` is already general -
  letting the user spell their own NULL keeps SQL, CSV, JSON, and any
  future format out of the printf spec.
- **`md=*` is a separate library.** Markdown rendering belongs in a
  future `markdown` library that returns strings, so the result
  composes with `printf`, `sprintf`, string concat, file writing -
  anywhere a string goes. Folding it into `%s` modifiers would lock
  markdown output to print sites.

## printf literal-pipe lookahead

Considered during M7 as a way to soften the breaking change to pre-M7
format strings: treat the `|` after a verb as a literal whenever the
*next* byte isn't a lowercase letter, so `"%s|%s"` would keep working
because `|%` isn't a key start.

Rejected because the rule is context-sensitive in exactly the wrong
direction. A user who writes `"%s|fill text"` (intending literal `|`)
would suddenly hit a parse error because `fill` is a valid-looking
key, while `"%s|9 lives"` would silently keep working because `9`
isn't a letter. The footgun moves around with whatever word follows
the verb.

The chosen rule is the strict one: `|` immediately after a verb
always starts a modifier list. To write a literal `|` in that
position, double it (`||`), parallel to the `%%` escape for a
literal `%`. The rule is uniform and easy to remember; the migration
cost was small (five test strings in this repo).

## Verbatim print builtin (`io.print` / `io.println`)

Considered: a plain `io.print(s)` / `io.println(s)` that writes a
string with no format interpretation - the safe primitive for "just
emit this string," since a dynamic value passed as `io.printf(s)`
misparses any `%` it contains (a generated password, user input, or
file bytes containing `%c` reads as an unknown verb, `%s` as a missing
argument).

Rejected because:

- It is a second way to do a job `printf` already covers. Any string
  prints with `printf("%s", s)`, so `print(s)` is convenience sugar
  over that - exactly the "no two `printf` flavors for the same job"
  case stance 1 (one way per thing) rules out.
- Keeping a single output entry point (`printf` / `sprintf` /
  `eprintf`) keeps the surface uniform; a reader never wonders which
  printer a program uses.
- The counter-argument - that a verbatim printer is a *distinct*
  primitive (no format language at all) rather than a printf flavor -
  is real but does not clear stance 1's bar: the observable job ("put
  this string on stdout") is the same, and the language prefers one
  canonical spelling even when a shorthand would be safer.

The accepted trade-off: `printf("%s", s)` is the **mandatory** idiom
for any dynamic or untrusted string, and `printf(s)` on such a value
is a latent bug. Documented in [io.md](../libraries/io.md) so the
footgun is called out at the source rather than papered over with a
second builtin.

## Methods on structs

Considered during M15.5 (`time` library) planning: let structs
declare methods that receive `self` implicitly, so accessors and
small operations on a struct value read as `$t.year()` instead of
`time.year($t)`. The trigger was the time library's many calendar
accessors; the same shape would later cover `hash.Stream.update`,
`os.Process.kill`, and every other library that holds onto state
behind a struct.

Rejected because:

- **It's the start of object orientation, not a syntactic shortcut.**
  Methods carry an implicit `self` binding and re-open the question
  of polymorphism, dispatch (single? double?), inheritance vs
  composition, interfaces / traits, visibility modifiers, and
  constructors. Once any one of those is shipped the others
  become a "why not also" thread. Jennifer is procedural with
  value types; the small, explicit shape is the language's
  identity, not a placeholder for an OO upgrade.
- **It invalidates every shipped library's call shape.** Today
  `lists.push($xs, x)`, `maps.has($m, k)`, `strings.upper($s)`,
  `hash.update($s, $b)`, `os.run($argv)` are all
  `lib.verb(receiver, args...)`. Adding `$receiver.verb(args...)`
  alongside would force each library to choose - or worse, ship
  both spellings and create the parallel-API problem stance #1
  rejects. Picking method-form everywhere would mean rewriting
  every library and every example.
- **Stances #1 and #2 already cover the ergonomics complaint.**
  "One way per thing" - if methods exist the function form
  becomes the second way. "Explicit over implicit" - the
  function call shows the library name at the call site; the
  method form hides it behind dispatch on `$receiver`'s type.
  The cost of `time.year($t)` vs `$t.year()` is one extra word;
  the cost of OO is a different language.

The chosen rule: structs have field access only. Operations on
struct values live as functions in the owning library
(`time.year($t)`, `hash.update($s, $b)`, `os.kill($p)`). If
ergonomics ever genuinely justify a receiver syntax, the
language change is large enough to merit its own milestone with
its own rejected.md trail of what the OO surface would *not*
include.

## `os.exit(n)`

Considered during the M11 / M15.1 planning: ship process exit as
both the language statement `exit EXPR;` (M11) and as a library
function `os.exit(n)` (planned for M15.1). The argument for keeping
both was that the language statement might be redefined under a
future embedding (a WASM sandbox, a host application driving the
interpreter) to mean "return from the interpreter," while
`os.exit(n)` would always mean "kill the host process" with no
possibility of redefinition. Same argument as C's `exit()` vs
`_exit()`.

Rejected because:

- **They collapse to the same thing today.** On a hosted OS the two
  forms have identical observable behaviour - same exit code, same
  stdout flush, same termination. Two spellings for one behaviour
  violates Jennifer's "one way per thing" stance immediately, in
  exchange for a divergence that hasn't been needed yet.
- **The embedding case is hypothetical.** A WASM-sandbox build and
  a host-driven embedding are long-horizon items; designing the
  public API for them now locks in a duplicate that the actual
  embeddings may not even want (an embedded host might redefine
  `exit EXPR;` *and* `os.exit(n)` the same way, leaving the
  distinction useless but still in the language).
- **The statement form is the right home.** Process exit is
  control-flow, parallel to `return;`: terminating execution from
  any reachable point. Wrapping the same primitive in a library
  function would read as "call into the OS" when it's actually
  "stop running." The statement form keeps the intent visible.

The chosen rule: `exit EXPR;` (and bare `exit;`) is the only
spelling. If a future embedding does need a "always kill the host"
escape hatch, it ships then with a name that says what it does
(`os.kill()`, `os.hardExit()`), not as a near-duplicate of the
existing statement.

## Implicit `use NAME;` fallback chain (M8+)

Considered during the M8-and-beyond roadmap discussion: have
`use http;` search the system libraries first, then any installed
WASM libraries, then a `http.j` on the file-import path, taking the
first one found. Same call-site spelling regardless of where the
implementation actually lives.

Rejected because:

- **It violates "explicit over implicit."** Two programs with the
  same source text would resolve `use http;` to different
  implementations depending on what's installed in the environment.
  At the call site (`http.get(...)`) the reader has no way to tell
  which `http` is in scope.
- **Silent precedence shifts break things.** Installing a WASM
  `http` package would shadow a (slower / different-flavoured)
  system `http`. The user's program would change behaviour with no
  source edit and no visible diff.
- **Debuggability suffers.** "Why does `http.get` behave this way?"
  becomes "which `http` did the resolver pick this time?" - a
  question that depends on the environment, not the code.

The chosen rule is explicit prefixes - the load source is visible
at the `use` site:

- `use net;` → system library only.
- `use wasm:libname;` → WASM library (when that milestone lands).
- `import "path/foo.j";` → file import (textual splice today;
  module-aware when M17 lands).

Users who genuinely want one entry point can write a tiny
Jennifer-coded shim that picks an implementation explicitly;
that's a per-program decision, not a language-wide default.

## FFI as a single milestone

Considered: a dedicated "FFI" milestone covering everything users
typically mean by foreign function interface - calling C libraries,
calling Go libraries, integrating with OS APIs, embedding Jennifer
in host applications, reusing existing ecosystems.

Rejected because it's three different problems wearing one name,
and lumping them together obscures that two are already addressed
and the third doesn't fit:

- **"Call Go libraries" is already the library mechanism.** Every
  `<pkg>.Install(in)` call registers Go functions as Jennifer
  builtins. That *is* FFI in everything but name. Anyone wanting
  to extend Jennifer with Go code writes a topic library today;
  no new surface needed.
- **"Integrate with OS APIs" is the topic-library job.** `os`,
  `fs`, `net`, and friends wrap Go's stdlib (which wraps the
  OS). Adding more OS surface means filling out those libraries,
  not inventing an FFI keyword.
- **"Reuse existing ecosystems / call C libraries" lands through
  WASM (M19), not cgo.** TinyGo's cgo support is partial on
  hosted targets and absent on WASI / baremetal; small-footprint
  embedding targets typically lack a userspace libc to link
  against at all. The WASM runtime milestone already plans
  sandboxed module loading, which sidesteps the ABI /
  marshalling / ownership fight entirely.
- **"Embed Jennifer in host applications" is a real gap - but
  it's a Go-side polish job, not a language feature.** It gets
  its own placeholder in the Long horizon list under
  "Host-embedding API", separate from FFI as conventionally
  meant.

The chosen rule: no FFI milestone. Three named homes instead -
topic libraries (Go + OS surface), WASM (M19, third-party
ecosystems), and a future host-embedding API milestone (driving
the interpreter from Go).

## References, interior mutability, shared mutable state

Considered as escape hatches from the deep-copy cost that
stance #5 (value semantics for collections) imposes on graph
algorithms, large data structures, and zero-copy workflows.
Three different shapes, considered together because they all
relax the same invariant:

- **Mutable references.** A `&xs` form that lets a callee
  write through to the caller's binding (Go / C++ semantics).
- **Interior mutability.** A wrapper type (Rust's `Cell` /
  `RefCell`) carrying a mutable cell behind an otherwise-immutable
  value, settable from anywhere holding the wrapper.
- **Shared mutable state.** A first-class "shared list" or
  "shared map" kind whose writes are visible to every binding
  that points at the same underlying storage.

Rejected because:

- **They break the local-reasoning guarantee that value
  semantics is the whole point of.** Today a reader of
  `func f(xs as list of int)` knows the caller's binding cannot
  be mutated by `f` - no aliasing, no surprises. Any of the
  three above forces every reader to ask "does this callee
  have a writable handle on my data?" at every call site. The
  cost is paid by every program, not just the ones that need
  the optimization.
- **Rust gets away with this only because the borrow checker
  pays the bill.** Aliased mutation in Rust is sound because
  the type system rejects programs where two writable handles
  coexist. Jennifer has no such checker and the language's
  whole point is local readability without a type-system tax;
  adding aliased mutation without the checker is just C
  semantics with prettier syntax.
- **Pre-1.0 softening is a one-way door.** Once any of these
  ships, the "no aliasing" guarantee is gone for every future
  reader. Even gating them behind a keyword (`shared`, `&mut`)
  forces every other Jennifer programmer to learn the
  aliased-mutation rules to read other people's code.
- **The performance gap is recoverable without semantic
  change.** Copy-on-write, arena allocation, and read-only
  slice views (`xs[1..5]` as a non-owning window that errors
  on assignment) close most of the gap that motivates the
  proposals. Those are interpreter-internal optimizations -
  they preserve "no aliasing" at the user level and are
  recorded as the "Performance & memory" placeholder in the
  Long horizon list. Graph algorithms specifically can use the
  M13.1 struct mechanism with explicit ID-keyed maps
  (`map of int to Node`) for parent / child links, which is
  the conventional fix in value-typed languages.
- **Shared state for concurrency belongs to M16.0.** The
  concurrency milestone already plans channel / queue / spawn
  primitives that handle the cross-task communication case
  without exposing shared mutable values to single-threaded
  code.

The chosen rule: stance #5 stands without exception. The
"Performance & memory" Long horizon entry covers the
optimizations that close the cost gap; users who genuinely
need aliased mutation are using the wrong language.

## Auto-invoked module setup hook (M17)

Considered while designing the module system (M17). A module
could declare a magic `func setup()` (or `init`) that the
loader calls automatically the first time the module is
imported, giving a defined place for one-time initialization.

Rejected because:

- **Redundant with `def const ... init EXPR;`.** Value-producing
  one-time work is already expressible: `export def const TABLE
  as list of int init buildTable();` where `buildTable` is a
  private module `func` that runs a loop and returns the result.
  The initializer runs exactly once at load (M17's run-once
  semantics), so a separate hook buys nothing for the common
  case.
- **A pure-side-effect hook reintroduces the footgun M17 removed.**
  M17 modules are declarations-only with no mutable top-level
  state, so `setup()` could only do I/O or seed a shared source -
  exactly the "import does work" surprise the declarations-only
  rule is meant to eliminate (the lesson behind Python's "imports
  should be cheap").
- **Cuts against "no required entry point."** Jennifer
  deliberately has no auto-invoked entry point; an auto-called
  module hook is the same idea by another name.
- **Stance #1 (one way).** `def const ... init ...;` already runs
  code once at load; a hook is a second spelling for "run at
  import."

The chosen rule: a module that genuinely needs imperative setup
exports an ordinary function the consumer calls explicitly
(`points.prepare();`) - more explicit (stance #2) and it keeps
the work off the import path. No new language surface, no
auto-invocation.

## Directory-as-module and cross-file re-export (M17)

Considered for multi-file modules: let a directory be a module,
imported by directory name (`import "bigmod" as bigmod;`)
resolving to a conventional entry file, and/or let several files
in the module each `export` with the module's public surface
being their union (a cross-file re-export / package model, as in
Python packages, Go packages, or ES module re-exports).

Rejected because:

- **`include`-assembly already covers it with one mechanism.** A
  multi-file module is a single entry `.j` file that `include`s
  its parts (subdirectories allowed); the splice shares one
  module scope and one `export` surface, and the consumer imports
  just the entry file. That is the M10 textual-splice mechanism
  plus M17.0's declarations-only-and-export rule, with no new
  surface. See M17.1.
- **Stance #1 (one way).** Directory-as-module would be a second,
  parallel way to compose a module on top of include-assembly,
  for the same outcome.
- **Re-export reopens a closed question.** Cross-file re-export is
  the same "reach a name from elsewhere and republish it" shape as
  the `from FOO import BAR` symbol import M17.2 already turned
  down; adding it here would reintroduce that surface by another
  name.
- **Relaxing must-end-in-`.j` for directory imports** removes the
  lexical marker that keeps import strings unambiguous, for an
  ergonomic gain include-assembly already delivers.

The chosen rule: multi-file modules assemble via `include` behind
one entry file (M17.1). Directory-as-module and cross-file
re-export can be revisited as their own milestone if real-world
module sizes ever make include-assembly unwieldy.

## Mandatory `public`/`private` keyword on module declarations (M17.4)

Considered as a stronger form of M17.4's export model: instead of
private-by-default with a single `export` marker, require an
explicit `public` or `private` keyword on every top-level `def
const`, `def struct`, and `func` in a module (omitting it would
be a parse error), on the "explicit over implicit" (stance #2)
argument.

Rejected because:

- **Stance #2 is already satisfied by `export`.** The property
  that must be explicit is the module's public surface - "what
  does this expose?" - and `export` answers it by grep
  (`grep '^export' mod.j` is the whole public API). The only
  implicit fact left is "unmarked means private," a one-line
  documented rule, not a hidden surprise.
- **Private-by-default is the fail-safe direction.** A forgotten
  marker keeps a name internal; there is no path where omission
  leaks a name into the public surface, which is the failure mode
  the M17.3/M17.4 supply-chain-hygiene argument cares about.
- **Every cited language uses marker-for-public, default-private.**
  Rust `pub`, TypeScript `export`, Go capitalization, Java
  package-private + `public` - none requires a visibility keyword
  on every declaration. The ecosystem converged on exactly the
  chosen model.
- **Cheaper script-to-module promotion.** Promoting a script means
  adding `export` only to the names being committed to a public
  API (M17.4's deliberate "I now have a public API" moment).
  Mandatory keywords would force annotating every top-level name
  on promotion, a noisier diff for no safety gain.
- **Stance #1 (one way).** Mandatory `public`/`private` doubles the
  visibility vocabulary; `export`-or-nothing is one axis, one
  token. (This is also why the parallel `public` synonym for
  `export` and a standalone `private` marker are rejected - see
  M17.4.)

## Implicit map-to-struct coercion at binding boundaries (M16.9 `json` typed decode)

Considered: letting a `map of string to V` value materialize a struct at
a typed binding *implicitly* - `def p as Point init json.decode(s);`
silently filling `Point`'s fields from the decoded object, erroring on
missing / unknown / wrong-type fields. It was the original M16.9 shape for
typed JSON decode. **Only the implicit form is rejected here** - an
*explicit* map-to-struct conversion (a spelled-out call or a struct-literal
spread, where the reader sees the conversion at the call site) is the
sanctioned path, deferred to Long horizon in milestones.md.

Rejected because:

- **Jennifer does no coercion at binding boundaries, anywhere.** The
  boundary (`MatchesDeclared`, at def-init, assignment, params, field
  writes) is strict: even `def x as float init 5;` errors - you write
  `5.0`. A map-to-struct coercion would be the first exception, and a
  cross-cutting one (it fires wherever a struct type meets a map value),
  softening a stance the rest of the language holds uniformly.
- **`json.decode` can't see the caller's declared type** (builtins don't
  receive it), so the coercion could only live in the interpreter's
  binding path - it is a language feature, not a library detail, and
  would apply to every map, not just decoded ones.
- **The explicit rebuild is short and self-documenting.**
  `def p as Point init Point{ x: $m["x"], y: $m["y"] };` names the schema
  at the call site, matching Jennifer's write-it-out style; the encode
  direction (`json.encode($p)`) is unaffected.

So `json.decode` returns generic Values (objects to `map of string to
V`); typed targets are rebuilt by hand, or - once specced - through the
*explicit* map-to-struct conversion (deferred; Long horizon). See M16.9 in
[milestones.md](../milestones.md) and
[libraries/json.md](../libraries/json.md).


## A language `any` / top type (M16.16 `json.Value`)

Considered: a general `any` type keyword - a top type every value matches -
as the home for heterogeneous data, its concrete driver being mixed-shape
JSON (`def x as any;`, `list of any`, `map of string to any`, with a
runtime-checked extraction back into concrete slots). **Rejected in favour
of confining the dynamism to an opaque, destructure-only `json.Value` tree**
(M16.16).

Rejected because:

- **It is a language-wide type-system opt-out.** An `any` value is usable
  at every expression site - operators dispatch on the runtime kind, so
  `def x as any init 5; def y as any init 3; $x + $y;` just works. A
  beginner can declare everything `any` and sail past the type system
  entirely; the strict, teachable identity ("you always declare what you
  store") is gutted to serve one library's need.
- **The friction fixes are worse than the disease.** Making `any` opaque
  (rejecting it at every operator so each use demands an explicit narrow)
  claws the strictness back, but only by bolting a whole second set of
  narrowing rules onto the type system - heavy machinery for what is
  really a per-source problem.
- **The dynamism is per-source, not universal.** Heterogeneous data comes
  from specific boundaries (a decoded JSON document; later, a DB row).
  Each earns its own labelled, opaque, destructure-only type -
  `json.Value`, a future `sql.Row` - so the escape hatch stays visible and
  local, never a shared language feature.

So there is no `any` keyword; heterogeneous JSON lives in `json.Value`,
walked with explicit accessors. See M16.16 in
[milestones.md](../milestones.md).

## printf i18n / string translation in format strings (M20.4 `intl`)

The proposal: since stance 3 makes `printf` about *presentation*, and
translating `"hello"` to `"hallo"` presents the same meaning in another form,
extend `(s)printf` to look up translations - a format-string-level translation layer.

Rejected because it fails the stance's own test - "shape how a value is
**rendered**" (kept) vs "transform the value itself" (a library call, like
`upper` / markdown rendering):

- **It is substitution, not rendering.** `%d|base=2` renders 255 as
  `11111111` - the same value, a pure function of value + modifier, no state.
  Translation *replaces* the string with a different one fetched from a
  catalog keyed by the current locale; the output has no mechanical relation
  to the input. That is transformation, sitting next to `upper` and markdown
  rendering, which the stance already makes library calls.
- **It smuggles in global state.** `printf` modifiers are pure; translation
  needs an ambient locale + loaded catalogs, violating stance 2 (explicit)
  and stance 7 (namespaced, no globals), and it couples `printf` to `intl`,
  killing the orthogonality stance 3 exists to protect.
- **Even gettext keeps it out of printf.** The canonical `_()` design
  translates the *format string* first, then formats the result:
  `io.printf(intl.tr("Hello, %s"), name)`. Translation and formatting already
  compose cleanly, so there is no gap for a `printf` extension to fill.

So translation stays `intl.tr()` (M20.4), composing with `printf` as above.
Locale-aware *value* formatting (a number with the locale's grouping /
decimal, a date in the locale's order) genuinely is presentation and is *not*
rejected - it is a separate, open `printf` question for later.

## Write-through copy-on-write for compound Values (M19.2)

Value semantics rest on eager deep copies at every store site (`def` /
assignment / parameter binding / spawn snapshot), so no two live bindings ever
share a compound backing and the mutation sites can grow a binding's own
backing in place - append-in-a-loop stays amortised O(N). An earlier attempt
added a copy-on-write layer on top: a `Value.shared *bool` marker set by
`Share()` on every `VarExpr` read and honoured by `Ensure()` at each mutation
site, meaning to defer the deep copy until an actually-aliased value is
mutated. It shipped **inert** - `Share()` has a value receiver and
`Environment.Get` / `GetAt` return the binding's `Value` by value, so the flag
was set on a throwaway copy and never reached the stored binding; `Ensure`
never detached. Correctness never depended on it, and every compound read
heap-allocated a `*bool` that went nowhere. It was removed.

The **write-through** version that would make COW actually fire - store
`*Binding` (or otherwise let the marker propagate back to the stored value) so
a mutation site genuinely detaches an aliased backing - was considered and
rejected:

- **The eager-copy invariant already makes aliasing rare.** Because every store
  copies, the only values that are ever aliased are transient rvalues that get
  copied again at the next store. A binding almost never holds a value that
  another live binding also holds, so the deferred-copy path COW optimises is
  nearly never taken - it would add machinery for a case that eager copies have
  already designed away.
- **It complicates the hot path it claims to help.** Propagating the marker
  means threading `*Binding` through `Environment` reads and every element /
  field slot, turning simple by-value reads into pointer bookkeeping and
  reintroducing shared mutable state the interpreter is otherwise free of.
- **The measured win is on append-in-a-loop, which already works.** In-place
  growth of a binding's own backing (safe precisely because nothing else
  aliases it) gives the O(N) append without any marker at all.

So Jennifer keeps eager copies plus one narrow optimisation - a fresh list /
map / struct literal RHS is already private, so the binding site skips the
redundant whole-value copy (`rhsFreshLiteral`). If profiling ever shows real
aliasing-heavy workloads paying for redundant copies, refcounted COW can be
revisited then.

## Sandboxing the stdlib, removing TLS `skipVerify`, and `SecureString` (security review)

A bot-generated security audit flagged the standard library's host access -
`fs` reading / writing any path, `net.connect` dialing any address, `sql.open`
to any DSN, `os.run` executing programs, `meta.call` invoking a function by name
- as CRITICAL / HIGH "path traversal / SSRF / RCE / arbitrary-dispatch"
vulnerabilities, and proposed removing or gating them (a `SetRoot` fs jail, a
dial policy, an exec allow-list, removing the TLS `skipVerify` option, a
zeroable `SecureString`).

These are **not vulnerabilities**; they are the intended surface of a
general-purpose language, exactly as Python's `open` / `socket` / `subprocess` /
`getattr` are. There is no trust boundary being crossed: the script *is* the
trusted party. A script's ability to read a file is a capability, not a defect.
Correspondingly:

- **No fs jail / dial policy / exec allow-list by default.** Restricting the
  stdlib to sandbox *untrusted* scripts is a real, separate goal - it is the
  planned capability sandbox, parked as `horizon.md` DRAFT#11 - not a fix for a
  bug in the trusted-script model. The genuine module-level injection / DoS bugs
  the same review found (where untrusted *data*, not untrusted *code*, reaches a
  wire) are fixed in M21.7; the recursion-crash robustness gap in M21.8.
- **The TLS `skipVerify` option stays.** Certificate verification is **on by
  default**; `net.TLSOptions.skipVerify` is an explicit, documented opt-out for
  self-signed / development endpoints (with `caCert` PEM-pinning as the
  narrower alternative) - the same role as Go's `InsecureSkipVerify`, `curl -k`,
  or Python's `verify=False`. Removing it would break legitimate use and is not
  how any mainstream TLS client behaves.
- **No `SecureString` zeroable-secret type.** Jennifer strings are immutable and
  garbage-collected, so an in-place wipe is not expressible, and no mainstream
  language zeroes secret strings in the interpreter. The pragmatic protection
  (owner-only file permissions for on-disk secrets) is M21.9, not a new type.

The review's one sound architectural point - that the security model should be
*stated* - is adopted: M21.7 adds a `SECURITY.md` / security-model doc making the
"scripts have full host access; sandbox untrusted ones" contract explicit.

## A `modsupport` / `modulecompanion` catch-all library (M21.10)

Considered while planning M21.10 (byte-oriented throughput): a single Go system
library - working names `modsupport` / `modulecompanion` - that would house the
performance-critical inner routines pulled out of various `.j` modules (mime
boundary scanning, http body accumulation, redis / imap framing, ...), even the
module-specific ones, so a slow `.j` hot loop could be replaced by one Go call in
a shared home.

Rejected because:

- **It is organized by the wrong axis.** The stdlib is topic-based (I/O to
  `io` / `fs` / `net`, byte / number / string transforms to
  `binary` / `math` / `strings`, ...). A "perf helpers for modules" library is
  organized by *who calls it* and *why*, not by topic - the classic `util` /
  `common` grab-bag that every mature codebase regrets, accreting unrelated
  functions with no coherent mental model.
- **The premise is mostly false.** The slow routines are rarely module-specific:
  mime scanning, http accumulation, and redis / imap framing all reduce to a
  small set of *general* byte primitives (`readAll` / `concat` / `slice` /
  `find`) that belong in a topic library (`binary`) and serve every module plus
  user code. That is M21.10.2, and it is the correct granularity.
- **It leaks module internals into the global public surface.** A
  `modsupport.mimeScanBoundary` builtin would appear in the cheatsheet and be
  callable by any program, yet mean anything only inside mime - an implementation
  detail published as API.
- **It unions dependencies and TinyGo build cost.** One library housing many
  modules' helpers pulls in the union of their Go dependencies; a program using
  only mime would compile redis's helpers too, unless the grab-bag is itself
  build-tag-split (complexity stacked on complexity). Topic libraries keep
  dependencies localized.
- **It invites speculative accretion.** "This might be slow, put it in
  `modsupport`" is exactly the unmeasured optimization the M21.10.1 benchmark
  exists to prevent.

The chosen rule: a hot `.j` routine that needs Go goes to a **topic library** as
a general primitive (`binary` for `concat` / `slice` / `find` / `readAll`,
`encoding` for codecs); a module whose *core* is inherently Go work is **promoted
to a Go library** (the `json` / `toml` / `xml` / `yaml` precedent - "a char
parser belongs in Go"); a genuinely module-specific, irreducible, *measured* hot
path folds into the nearest topic library or ships as a build-tag-split helper
co-located with that concern - never a global grab-bag. Only routines that
measure hot in M21.10.1 move.

## Multiple returns / destructuring

Considered as an ergonomic shorthand (it lived in the horizon idea collection):
let a method return several values (`return $a, $b;`) and a `def` / assignment
bind them positionally (`def q as int, r as int init divmod($a, $b);`,
`$x, $y = $y, $x;`), optionally extended to struct / list destructuring.

Rejected in favor of **structs** - which the language already has - because:

- **Structs already cover it, and read better.** A `def struct` with named fields
  is the sanctioned way to return several values, and the names are
  self-documenting (`$d.quotient` / `$d.remainder`, not "the second one"). A
  struct return also stays a single typed `Value`, so it composes everywhere -
  stored, passed, nested, printed - whereas a multi-return only works at the bind
  site. Adding multi-return would be a *second* way to do what structs already
  do, which the "one obvious way" / minimalism stances push back on.
- **The dominant motivation does not apply here.** Multiple returns earn their
  keep in languages that return errors as a value (`(value, error)`). Jennifer
  raises errors with `throw` and handles them with `try` / `catch`, so errors are
  never returned - that removes the biggest use case and leaves only small
  anonymous tuples (divmod, min / max), exactly what a two-field struct expresses.
- **It is not free plumbing, and the cost is Jennifer-specific.** Methods declare
  no return type or arity (a body may `return $a, $b;` on one path and
  `return $c;` on another), so return arity would become a *runtime* contract: a
  call site would newly have to care how many values came back, and a
  single-vs-multi mismatch would be a positioned runtime error. Keeping the
  tagged-union `Value` model clean would also force a statement-position-only
  restriction (no `f(g())` nesting) plus a slice-carrying `ReturnSignal` and a
  parallel call-eval path - real machinery for a narrow convenience structs
  already serve.

The chosen solution: **return a struct.**

```jennifer
def struct DivMod { quotient as int, remainder as int };
func divmod(a as int, b as int) {
    return DivMod{ quotient: $a // $b, remainder: $a % $b };
}
def d as DivMod init divmod(17, 5);   # $d.quotient == 3, $d.remainder == 2
```

## Per-algorithm digest / checksum shortcuts (`hash.sha256(b)`, `crc.crc32(b)`)

Considered once digit-bearing identifiers (M22.2) made the names spellable:
per-algorithm one-shot methods - `hash.md5(b)` / `sha1(b)` / `sha256(b)` /
`sha384(b)` / `sha512(b)` and `crc.crc32(b)` / `crc64(b)` - beside the existing
`hash.compute(b, algo)` / `crc.compute(b, algo)`.

Rejected on **stance #1 (one way per thing)** because:

- **They are a parallel API for the same job.** `hash.sha256(b)` and
  `hash.compute(b, "sha256")` produce the identical bytes; that is exactly the
  two-ways redundancy the stance rejects (its own examples are `++` / `+=` /
  two `printf` flavors).
- **The rest of the family is irreducibly algorithm-as-value, so the shortcuts
  would make it *less* consistent, not more.** `hash.hmac(_, _, algo)`,
  `crypto.pbkdf2(_, _, _, _, algo)`, and `crypto.hkdf(_, _, _, _, algo)` all take
  the algorithm as a runtime string and cannot cleanly go static (SHA-1 vs
  SHA-256 vs SHA-512 across three functions is a large surface, and the algorithm
  is *negotiated* - SCRAM, JWT `alg`, TLS suites pick it on the wire). Splitting
  only `compute` would leave the crypto/hash surface half per-method, half
  algo-string.
- **"Runtime algorithm => use `if()`" breaks the one real caller.** `sasl.j`
  threads a server-negotiated `$s.algo` through `compute`, `hmac`, *and*
  `pbkdf2` together; forcing a per-method dispatch ladder there (while `hmac` /
  `pbkdf2` beside it still take the string) is worse on every axis.

The chosen form: the algorithm is always a value, uniformly -
`hash.compute(b, algo)`, `hash.hmac(k, m, algo)`, `crypto.pbkdf2(..., algo)`,
`crypto.hkdf(..., algo)` - so a fixed algorithm is a string literal and a
negotiated one is a variable, with no new surface and no dispatch ladder.

## Parallel sequence-number + UID verbs in `imap`

**Rejected.** M23.3 briefly shipped a UID-addressed twin for every
sequence-number `imap` verb (`fetch` + `uidFetch`, `search` + `uidSearch`,
`copy` + `uidCopy`, `move` + `uidMove`, and so on - twenty verbs for ten
operations). Collapsed to **UID-only addressing** (each verb sends its `UID`
form and takes a UID) on **stance #1 (one way per thing)**:

- **The twins are a parallel API for the same job.** `fetch(session, n)` and
  `uidFetch(session, uid)` retrieve a message; they differ only in which
  identifier space the `int` names. That is the two-ways redundancy the stance
  rejects.
- **It also violated stance #2 (explicit over implicit).** With both, a bare
  `fetch($s, 42)` hid *which* id space `42` was in - a sequence number and a UID
  are different numbers for the same message, so mixing them is a silent bug.
- **Sequence-number addressing is the footgun the milestone existed to fix.** An
  `EXPUNGE` renumbers sequence numbers, silently breaking "fetch only what's new
  since last run" - the exact correctness problem M23.3 set out to solve. Keeping
  it as a co-equal addressing scheme preserved the trap.

The chosen form: one addressing scheme, UID, which is stable across expunges and
sessions. Sequence numbers survive only where the server *emits* them as data -
the `selectFolder` count and IDLE `EXISTS` / `EXPUNGE` push numbers - never as an
addressing input a caller hands back in. A session-level `useUid` mode (the
imapclient approach) was also considered and rejected: it fixes stance #1 but
doubles down on stance #2's hidden-mode problem.
