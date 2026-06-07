# Jennifer - Milestones

Development is split into milestones. Each milestone produces a *working*
interpreter that runs a strictly larger subset of the language.

---

## M1 - End-to-end MVP

**Status:** done.

The smallest possible vertical slice that proves the pipeline:
source → tokens → preprocessed tokens → AST → result.

**Language subset:**

- Types: `int`, `string` only
- `def x as int init 5;` (the `init` clause is required in M1)
- `$var` references
- Arithmetic: `+ - * /` and `%` on ints; parenthesised grouping
- `printf("text")` and `printf($var)` - single argument, no format specifiers
- `use io;` (library import)
- `import "file.j";` (file import - textual splice; works anywhere, including
  inside a block; circular-import detection; subdirectories supported via the
  string path)
- Method definitions (zero-arg, top-level only)
- Comments: `//` and `/* */`

**Post-M1/M2 syntax adjustments (kept here for historical clarity):**

- The `app()` entry-method requirement was dropped: top-level statements run
  in source order, methods are just hoisted callables.
- `define` was originally a synonym for `def`. It has been removed; only
  `def` remains, and methods use a new `func` keyword.
- At the def site, names are bare identifiers (no `$` sigil). The `$` is
  reserved for use-site references to mutable variables.
- Imports split into two keywords: `use NAME;` for system libraries (originally
  `import NAME;`) and `import "PATH.j";` for files (originally
  `import NAME.j;` with an identifier path). The new string-literal path
  enables subdirectories, hyphens, and underscores.

**What lands beyond the bare MVP:**

- Source-context caret in error messages (`file: error at L:C` + the offending
  line + caret)
- Golden-file integration test that walks `examples/*.j`
- TinyGo build target verified

**Exit criterion:** `./jennifer run examples/hello.j` prints `42`.

---

## M2 - Types, constants, scoping, control flow

**Status:** done.

**Decision (resolved at start of M2):** uninitialized `def x as T;` gives
`$x` the zero value of `T` (`0`, `0.0`, `""`, `false`, `null`).

Rounds out the "ordinary" feature set the spec calls for.

**New types and literals:**

- `float`, `null`, `bool` types
- Literals `null`, `true`, `false`
- `float` literals: `3.14`, `0.5`

**Variable system:**

- `def x as int;` (uninitialized → zero value of the type)
- `def const NAME as TYPE init VALUE;` - constants; assignment-after-init
  is an error
- Name-rule enforcement: variable names `[A-Za-z]{1,64}`, constant names
  `[A-Z]{1,64}`
- Nested block scoping with the full visibility/no-shadowing rules
- Assignment statement: `$x = EXPR;`

**Operators:**

- Comparison: `< > <= >= ==` → `bool`
- `+` for string concatenation
- `int` ↔ `float` promotion in mixed arithmetic (result is `float`)
- Escape-sequence parsing inside `'...'` strings (currently only `"..."`)

**Control flow:**

- `if (cond) { } elseif (cond) { } else { }`
- `while (cond) { }`
- `for (init; cond; step) { }`
- All conditions must be `bool` (no implicit truthiness)

**New AST nodes:** `FloatLit`, `NullLit`, `BoolLit`, `ConstDefineStmt`,
`AssignStmt`, `IfStmt`, `WhileStmt`, `ForStmt`, `CompareExpr`.

**Decision required at start of M2:** semantics of uninitialized `def`
(recommend: zero value of the declared type - `0`, `0.0`, `""`, `false`).

**New tests:** scope tests (inner reads outer; inner cannot re-declare; const
cannot be reassigned), full arithmetic/comparison matrices, programs like
`fizzbuzz.j` and `fib.j`.

---

## M3 - Methods with parameters and return values

**Status:** done.

- `func name(a as int, b as string) { ... }` - parameter parsing
- Argument passing - by value (parameters bind in a fresh call frame whose
  parent is the global env)
- `return;` and `return EXPR;`
- Type-checking at the call site: argument count and declared type both checked
- Method calls inside expressions, recursion (free once methods call methods)
- `sprintf(...)` returns a formatted string instead of writing
- `printf` / `sprintf` accept Go-style format strings with verbs `%d %f %s %t %v %%`
- `examples/factorial.j` added as a recursion smoke test with golden output

**Post-M3 adjustment:** the omnibus `stdlib` library was retired in favor
of topic-based libraries. `printf`/`sprintf` moved to a new `io` library
(`use io;`). Future builtins go in their own libraries (`math`, `strings`,
`convert`, etc.), all explicit `use` - no auto-loading.

---

## M4 - Polish & ergonomics

**Status:** done.

- **Logical operators:** `and`, `or`, `not` (word-based) with standard
  precedence and short-circuit `and`/`or`.
- **Unary minus.**
- **Python 3 division:** `/` always returns `float`; new `div` keyword for
  floor division (Pascal-style because `//` collides with line comments).
- **Float display fix:** floats always print with a decimal (`5.0`, not `5`)
  so the type stays visible.
- **`convert` library:** `int(v)`, `float(v)`, `string(v)`, `bool(v)`,
  `typeOf(v)`. Type-name calls (`int(...)` etc.) work in expression position
  because the parser allows TYPE tokens before `(`. Strict conversion
  semantics - `int("abc")` and `bool("maybe")` both error. `bool` is
  canonical-only across all source kinds (`bool(123)` errors).
- **`math` library:** `abs`, `min`, `max`, `sqrt`, `pow`, `floor`, `ceil`,
  `round` plus constants `PI`, `E`. Strict on undefined math (sqrt of
  negative, pow producing NaN/Inf rejected). `floor`/`ceil`/`round` return
  int. Interpreter gained `RegisterConst` so libraries can expose constants.
- **`strings` library:** `len`, `upper`, `lower`, `contains`, `startsWith`,
  `endsWith`, `indexOf`, `trim`, `trimLeft`, `trimRight`, `replace`, `repeat`,
  `substring`. Rune-based indexing throughout. `split`/`chars`/`join`
  deferred until arrays land. Also renamed `typeof` → `typeOf` in `convert`
  for camelCase consistency with the new multi-word names.

---

## M5 - Interpreter improvements

- **Better errors:** cross-file error sources - done. Every error type
  (`LexError`, `PreprocessError`, `ParseError`, `runtimeError`) carries the
  originating file and implements a small `Position()` interface. The CLI
  uses that interface to load the right file when printing the source
  snippet, so an error raised inside an imported `.j` displays the line
  from the imported file, not from the importing file.
- **REPL:** `jennifer repl` - done. Interactive read-eval-print loop that
  reuses the existing lexer/preproc/parser/interpreter. Globals, constants,
  methods, and library imports persist across inputs; a bare final
  expression prints its non-null value. Continuation lines are detected by
  counting `{`/`(` depth and requiring a `;`/`}` terminator, so multi-line
  method definitions and blocks work transparently. Methods can be
  redefined freely from one input to the next (`Run` still rejects
  duplicates; the REPL uses a new `EvalInteractive` entry point).
- **Build version + `core` library** - done. The Makefile regenerates
  `internal/version/version_gen.go` from `git describe --tags --long`
  before every build, so `jennifer help` / `jennifer version` always
  report the current commit. The `core` library exposes the same string
  to Jennifer programs as the `JENNIFER_VERSION` constant. Format: `"<tag>"` on
  a tag, `"<tag>-dev+<N>.<shortsha>"` past a tag, `"dev"` outside git.
  Codegen is used instead of `-ldflags -X` because TinyGo 0.41 silently
  ignores `-X`; codegen works on both toolchains.

  The library was originally named `meta` and required `use meta;`. As
  a late-M5 cleanup it was renamed to `core` and made auto-loaded -
  pre-imported by the interpreter in `New()`, with explicit `use core;`
  now a runtime error. (The constant itself was also renamed from
  `VERSION` to `JENNIFER_VERSION` once the M5 underscore-in-constants
  rule made that possible - follows the PHP_VERSION / RUBY_VERSION
  precedent and leaves the bare `VERSION` name free for future
  per-library use.) The rationale: `JENNIFER_VERSION` is structural and
  universally useful, and the same library will host other structural
  builtins like `len` (pass 2 of the cleanup moves it from `strings`)
  and eventually `has`, `PLATFORM`, etc. The "nothing for free" rule
  still holds for every other library.
- **Formatter:** `jennifer fmt` - done. Token-stream formatter that
  re-emits canonical source per [docs/stylespec.md](stylespec.md).
  Works at the lexer level (not AST) to preserve `import "file.j";`
  statements and user-written parentheses verbatim. Verified
  idempotent and behavior-preserving across all `examples/*.j`.
  Known v1 limitations: comments are dropped (lexer strips them) and
  blank lines aren't preserved or inserted between logical groups.
- **Inspection subcommands:** `jennifer tokens <file>` dumps the lexer
  output; `jennifer ast <file>` dumps the preprocessed AST as JSON
  (hand-rolled emitter, no `encoding/json`, so TinyGo stays clean).
  Both useful for understanding the pipeline and for tooling that
  wants to consume Jennifer programs as data.
- **REPL:** improve repl with history (cursor up/down scrolling)
- **Documentation:** improve documentation in /docs and make sure it's up-to-date with M5

### Deferred from M5

- **Comment preservation in `fmt`.** Lexer would need to carry `//` and
  `/* */` as tokens (or attach them to following nodes); the formatter
  would then weave them back in.
- **Blank-line preservation / auto-insertion in `fmt`.** Either keep
  user blank lines as a side channel during lexing, or insert them
  automatically between logical groups (imports vs methods vs top-level
  code, method-to-method).
- **Binary AST cache (`.jc` files).** Pre-parsed loading for big
  programs and McFly OS embedding. Its own milestone - file-format
  design, versioning, and TinyGo-safe serialization are enough work to
  merit dedicated treatment. The text JSON form via `jennifer ast` is
  the placeholder until then.

---

## M6 - Lists and maps

Two new compound types, plus the strings-library functions that were
deferred until compound types existed.

**Naming decision (resolved at start of M6):** the sequence type is
called `list`, not `array`. "List" describes an ordered sequence of
values; "array" carries fixed-size / contiguous-memory baggage from C/
Java/Go that doesn't match Jennifer's semantics. The map type is called
`map`. The pair reads as one vocabulary: "list of values, map of keys to
values." Beginners coming from Lisp/Scheme should note that Jennifer's
`list` is array-backed (Go slice underneath), not a linked list - O(1)
random access, but no O(1) prepend.

**Value semantics (resolved at start of M6):** lists and maps are
value-typed, like every scalar in Jennifer today. `$ys = $xs;` copies;
mutations to `$ys` do not affect `$xs`. Function parameters bind by
copy. Aliasing is impossible. This is the Pascal/Algol tradition, not
the Python/JS tradition.

Why: Jennifer is teachable, and reference semantics introduce "spooky
action at a distance" bugs (mutate `$ys`, find `$xs` changed) plus they
undermine `const` - any other binding holding the same list could
mutate through it. Value semantics removes both problems for free.

Cost: O(n) copy on every assignment and call. Acceptable at Jennifer's
educational scale; copy-on-write is available as a future optimization
that preserves the user-visible semantics.

Implications: `Value` stores lists/maps as owned data (not pointers),
the call-frame binder copies the value, and the AST-JSON emitter
serializes by value.

**`const` works for `list` and `map`, and constness is deep
(resolved at start of M6):**

```jennifer
def const PRIMES as list of int init [2, 3, 5, 7, 11];
printf("%d\n", $PRIMES[0]);   // 2 - reads are fine
$PRIMES = [9, 8];             // error: binding is const
$PRIMES[0] = 9;               // error: contents are const
```

Combined with the value-semantics decision above, `const` becomes a
real guarantee: the binding can't be repointed, the contents can't be
mutated, and the value can't escape through aliasing to a non-const
binding that would mutate it. The existing `IsConst` flag on every
binding carries through; the new check is at index-assignment time,
which errors with a positioned message when the target binding is
const. The same rule applies to map values (`$CONST_MAP["k"] = ...`).

**Language additions:**

- New value kinds: `KindList`, `KindMap`.
- New keywords: `list`, `map`, `of`, `to`, `in`.
- New tokens: `LBRACKET` (`[`), `RBRACKET` (`]`), `COLON` (`:`).
- New AST nodes: `ListLit`, `MapLit`, `IndexExpr` (for both `$xs[0]` and
  `$m["key"]`), `ForEachStmt` (for the `for (def x in $coll) { ... }`
  form). Existing `ForStmt` stays for the C-style three-part header.
- New parameterized `Type` representation, **recursive**. Today `Type`
  is a flat enum (`TypeInt`, `TypeFloat`, ...); M6 widens it to a
  struct that carries an element-type pointer for `list of T` and
  key/value-type pointers for `map of K to V`:

  ```go
  type Type struct {
      Kind    TypeKind  // TypeInt, ..., TypeList, TypeMap
      Element *Type     // for TypeList; nil otherwise
      KeyType *Type     // for TypeMap; nil otherwise
      ValType *Type     // for TypeMap; nil otherwise
  }
  ```

  Because `Element`, `KeyType`, and `ValType` are themselves `*Type`,
  nesting falls out naturally: `list of list of int`,
  `map of string to list of int`, `list of map of string to int` all
  work without special-casing. `parseType()` recurses through the
  `list of <TYPE>` and `map of <TYPE> to <TYPE>` productions. This is
  the most substantial structural change in the milestone and ripples
  into parser declarations, `Environment.Binding`, `MatchesDeclared`,
  the formatter, and the AST-JSON emitter.

**Nesting depth and style guidance (resolved at start of M6):** no
explicit depth cap. The recursive parser bottoms out at Go's stack
limit (~10K-100K levels) - far beyond anything any human will write,
and a clean stack-overflow if someone generates Jennifer source
programmatically. Inventing an arbitrary limit (8? 16?) would just
be the next surprise users hit, and the threshold is impossible to
defend.

What we ship instead: a style note in `docs/stylespec.md` saying
that nesting beyond 3 levels is a code smell - 1-2 levels is normal,
3 is uncommon, 4+ almost always wants a struct or a named type
instead. The advice points forward to structs whenever they land
(post-M9 namespacing and M10 domain libraries; tentatively M11). A
future fmt-time linter could nudge at depth >= 4; out of scope for
M6.
- New auto-loaded `core` library housing `len`. Pre-imported by the
  interpreter at startup; users never write `use core;`. See the
  "len" decision below.

**Syntax:**

```jennifer
def xs as list of int init [1, 2, 3];
def m as map of string to int init { "a": 1, "b": 2 };

printf("%d\n", $xs[0]);
printf("%d\n", $m["a"]);

$xs[1] = 42;
$m["c"] = 3;
```

**Semantics:**

- Lists: 0-indexed, mutable, dynamically grown. Out-of-bounds read or
  write is a positioned runtime error (no silent extension).
- Maps: arbitrary key type as long as it's hashable (`int`, `float`,
  `string`, `bool`); typically `string` keys are the path of least
  surprise. Reads of a missing key are a runtime error (no `null`
  fallback - keeps the type discipline tight); add a separate
  `has($m, key)` builtin or a key-test operator for the lookup case.

**Iteration (in scope for M6):** ships in M6, not a follow-up. Lists
and maps without iteration would be barely usable, and the syntax fits
the existing `for` shape cleanly:

```jennifer
for (def x in $xs) {
    printf("%v\n", $x);
}

for (def k in $m) {
    printf("%s -> %d\n", $k, $m[$k]);
}
```

New keyword: `in`. For maps, iterates **keys** (no tuple destructuring -
we don't have tuples). Users look up values explicitly via `$m[$k]`.
Map iteration order is **insertion order, deterministic** (not Go's
randomized default) so tests and the REPL are reproducible. The
underlying map representation will need to track insertion order, which
rules out `map[K]V` alone - probably a `struct { keys []K; values
map[K]V }` or similar.

**`len(...)` becomes an auto-loaded builtin (resolved at start of
M6):** `len` lives in a new `core` library that the interpreter
pre-imports at startup (`i.imported["core"] = true` in `New()`). Users
never write `use core;` - it's always available. Polymorphic on
strings, lists, and maps. Migration: `len` moves out of `strings`; the
`strings` library keeps only the string-specific functions. The
`core` library establishes "auto-loaded" as a special library kind -
reserve this carefully, it's the escape hatch from "nothing for free"
and should hold at most 2-3 things ever. `len` is the only inhabitant
for M6.

**Strings library completion (in scope for M6):**

`split`, `chars`, and `join` were deferred until compound types
existed. They land here:

- `split(s, sep) -> list of string`
- `chars(s) -> list of string` (one entry per Unicode code point)
- `join(parts, sep) -> string` where `parts` is `list of string`

**New tests:**

- List operations: literal, index read/write, out-of-bounds error,
  type mismatch on write, equality semantics (defer if scope is tight).
- Map operations: literal, key read/write, missing-key error, key-type
  mismatch.
- Integration: `split($s, ",")` and `join($parts, ", ")` round-trip on
  the showcase example.
- AST JSON: `ListLit`, `MapLit`, `IndexExpr` emit cleanly.
- Formatter: `[1, 2, 3]` and `{"a": 1}` survive a round-trip
  unchanged. **Whitespace rule (resolved at start of M6):** no padding
  inside `[ ]` or `{ }`, space after `,`, space after `:`. Matches the
  existing `(...)` rule for consistency. Empty literals are `[]` and
  `{}` (no inner space). Multi-line literals are allowed when values
  don't fit on one line; trailing comma in the multi-line form is
  permitted (not required).

**Docs to update at end of M6:**

- `docs/user-guide.md` - new "Lists and maps" section, plus one-liner
  clarification that `list` is array-backed (not Lisp-style) and that
  list/map values copy on assignment (value semantics).
- `docs/technical/lexer.md` - new tokens (`LBRACKET`, `RBRACKET`,
  `COLON`) and keywords (`list`, `map`, `of`, `to`, `in`) in the token
  table.
- `docs/technical/grammar.md` - new EBNF productions for list/map
  literals, parameterized types, index expressions, and the
  `for (def x in $coll)` iteration form.
- `docs/technical/interpreter.md` - new value kinds, the parameterized
  `Type` representation, semantics of bounds/key errors, value-copy
  semantics on assignment and function-parameter binding, and
  deterministic insertion-order map iteration.
- `docs/stylespec.md` - whitespace rules for `[...]` and `{...}`
  (no padding, space after `,` and `:`, trailing-comma allowed
  multi-line).
- `docs/libraries/strings.md` - `split` / `chars` / `join` defined.
  (`len` already moved to `core` in pass 2 of the M5 cleanup.)
- `docs/libraries/core.md` - extend the existing page to describe
  `len`'s list/map dispatch.
- `examples/showcase.j` - exercise lists, maps, iteration, and the
  three new strings functions; update `examples/expected/showcase.txt`.

**Exit criterion:** the showcase example exercises lists, maps, and
`split`/`chars`/`join` end-to-end; round-trips through `fmt` are
stable; AST JSON for the new node kinds validates; the REPL handles
list/map literals and indexed assignment correctly.

---

## M7 - printf modifier

- **(s)printf**: introduce format verb modifiers

---

## M8

- **CI/CD:** Github Actions pipeline for automatic testing and release

---

## M9 - Library namespacing

The flat-namespace model is fine for the essential libraries today
(`io`, `convert`, `math`, `strings`, plus the auto-loaded `core`) -
they're small, names are carefully chosen, collisions are unlikely. It will not scale to domain
libraries like `regex`, `net`, `bio`, or `crypto`, where the chance of
two libraries shipping a `len` / `parse` / `encode` is high. M9 settles
the policy before the first domain library lands.

**Decision (to confirm at start of M9):** hybrid model. Essential
libraries stay flat for ergonomics; domain libraries are prefixed
through a namespace tag set at registration time. Optional aliasing
lets the user shorten a namespace.

**Concrete plan:**

- **Library declares a namespace at install time.**
  ```go
  // internal/lib/io/iolib.go - essential, no namespace
  in.Register("io", "printf", printf)
  // internal/lib/bio/biolib.go - domain library
  in.RegisterNamespaced("bio", "translate", translate)
  ```
  Internally: extend `builtinEntry` with an optional `Namespace string`.
  Empty namespace = flat lookup (current behavior).
- **Syntax: qualified calls via `.`.**
  The lexer already reserves `TOKEN_DOT`. Add a grammar production:
  ```ebnf
  qualifiedCall = IDENT "." IDENT "(" [ args ] ")" ;
  ```
  `primary` accepts `qualifiedCall` alongside the existing `call`. A
  bare `IDENT` immediately followed by `.` and another `IDENT` and `(`
  is parsed as a qualified call; otherwise the existing call /
  constant-ref disambiguation applies.
- **New AST node:** `QualifiedCallExpr { Namespace, Callee, Args }`.
- **Lookup rule:** at the call site, the interpreter resolves
  `bio.translate(...)` as "find a builtin whose `Namespace == "bio"`
  and whose `Name == "translate"`, registered under a library the
  program has `use`d." A bare `translate(...)` finds only the
  flat-registered builtins (and user-defined methods).
- **Constants get the same treatment.** A namespaced library exposing
  a constant uses `bio.STOPS`; flat-registered constants like `PI`
  stay bare.
- **`use bio;` is unchanged at the source level** - the user still
  imports by library name. The namespace is whatever the library
  declared, not the library name necessarily (usually they'll match,
  but a library called `crypto_primitives` could expose itself as
  `crypto.` to be brief).
- **Optional aliasing (defer to M9.1 if scope is tight):**
  ```jennifer
  use bio as b;
  printf("%d\n", b.translateLen($seq));
  ```
  Adds an `AsName` to `ImportStmt`. Probably worth doing in the same
  milestone so the convention lands complete.
- **Migration of existing libraries:** none. All five essentials stay
  flat. The change is additive; existing source keeps working
  unchanged.
- **No-shadowing rule:** scoped to a namespace. `bio.len` does NOT
  collide with `strings.len`. A user method named `translate` collides
  with `bio.translate` only if it's referenced bare; it does not
  conflict with the qualified form. Document the resolution rule
  carefully in `docs/technical/interpreter.md`.

**Docs to write at end of M9:**

- Update `docs/libraries/index.md` with the namespace policy and a
  rule for library authors ("is your library essential or domain?
  if domain, set a namespace").
- Update `docs/technical/grammar.md` EBNF.
- Update `docs/technical/interpreter.md` Builtins-and-libraries
  section to describe the lookup rule.
- New section in `docs/stylespec.md`: how to write namespaced calls
  (`bio.translate($seq)`, no space around `.`, same as method-call
  paren-hugging).
- `cmd/jennifer/fmt.go` already preserves `TOKEN_DOT` verbatim;
  verify the formatter handles it cleanly with a small test.

**Exit criterion:** a synthetic example library can register itself
under a namespace, be `use`d, and be called via `lib.fn(...)`; the
parser, interpreter, REPL, AST JSON dump, and formatter all handle
the qualified form correctly; the five existing essential libraries
continue to work unchanged.

---

## M10 - Domain libraries

- **File library** (`use fs;`)
- **OS library** (`use os;`)
- **Regex library** (`use regex;`)
- **Network library** (`use net;`)
- **Crypto library** (`use crypto;`)

All M10 libraries are domain libraries and ship with namespaces
(per M9): `fs.read`, `os.getenv`, `regex.match`, `net.dial`,
`crypto.sha256`. This is the first real test of the namespacing
convention - if any of these prove awkward (`crypto.sha256` reads
fine; `fs.read` collides with the urge to call it `readFile`), revisit
in early M10 before too many call sites lock in.

---

## Future directions

Long-term goals - not committed to a milestone yet, but the code should not
foreclose them.

- **Cross-platform support.** Today Jennifer targets Linux only. Windows and
  macOS are planned. When touching filesystem, paths, line endings, or
  process behavior, prefer portable stdlib helpers (`path/filepath`, not
  hardcoded `/`); avoid Linux-only assumptions.
- **McFly OS kernel integration.** A long-term goal is to embed the Jennifer
  interpreter into **McFly OS**, an experimental OS also
  written in TinyGo. This reinforces the TinyGo-friendliness discipline: no
  `reflect`-heavy code, no goroutines in the core, no heavy stdlib
  dependencies, and no hard dependencies on a hosted runtime (ambient stdin,
  network, dynamic linking).
- **Inline Assembler**
