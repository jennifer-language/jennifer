# Jennifer - Milestones

Development is split into milestones. Each milestone produces a *working*
interpreter that runs a strictly larger subset of the language.

---

## M1 - End-to-end MVP

**Status:** done.

Smallest vertical slice that proves the pipeline (source → tokens →
preprocessed tokens → AST → result):

- Types: `int`, `string`
- `def x as int init 5;`, `$var` references, method defs (zero-arg, top
  level), `import "file.j";`, `use io;`, single-arg `printf`
- Arithmetic `+ - * / %` on ints; comments `#` and `/* */`
- Source-context caret in error messages
- Golden-file integration test and TinyGo build verified

**Exit criterion:** `./jennifer run examples/hello.j` prints `42`.

---

## M2 - Types, constants, scoping, control flow

**Status:** done.

Rounds out the "ordinary" feature set:

- New types `float`, `null`, `bool` with literals `3.14`, `null`, `true`,
  `false`
- Uninitialized `def x as T;` gives `T`'s zero value
- `def const NAME as TYPE init VALUE;` (reassignment is an error)
- Nested block scoping; inner scopes cannot redeclare visible names
- Assignment statement `$x = EXPR;`
- Comparison `< > <= >= ==`, `+` for string concat, `int`↔`float`
  promotion
- Escape parsing in `'...'` strings (previously only `"..."`)
- Control flow: `if`/`elseif`/`else`, `while`, `for`, all requiring
  `bool` conditions (no implicit truthiness)

---

## M3 - Methods with parameters and return values

**Status:** done.

- `func name(a as int, b as string) { ... }` with typed parameters,
  by-value argument passing, call-site arity + type checks
- `return;` and `return EXPR;`; recursion works
- `sprintf` and format verbs `%d %f %s %t %v %%` for both `printf` and
  `sprintf`
- The omnibus `stdlib` retired in favor of topic-based libraries; `io`
  is the first.

---

## M4 - Polish & ergonomics

**Status:** done.

- Logical operators `and`, `or`, `not` (word-based, short-circuit)
- Unary minus
- Python-3 division: `/` always returns `float`; new `div` keyword for 
  floor division (`//` is taken by line comments)
- Floats always display with a decimal (`5.0`, not `5`) so the type
  stays visible
- New libraries (all `use`-gated): [`convert`](libraries/convert.md),
  [`math`](libraries/math.md), [`strings`](libraries/strings.md)
- Interpreter gained `RegisterConst` so libraries can expose constants
  (`PI`, `E`).

---

## M5 - Interpreter improvements

**Status:** done.

- **Cross-file error sources** - errors raised inside an imported `.j`
  display the line from the imported file. See
  [technical/interpreter.md > Errors and positions](technical/interpreter.md#errors-and-positions-cross-file).
- **REPL** - `jennifer repl`, persistent globals/methods/imports across
  inputs, multi-line input via brace balancing, expression results
  printed. See [technical/cli.md > REPL](technical/cli.md#repl-cmdjenniferreplgo).
- **REPL line editor** - cursor keys, Home/End, word motions, Ctrl+W /
  Ctrl+U / Ctrl+K, in-memory history (Up/Down), Ctrl+C cancel. Non-TTY
  stdin falls back to plain line reading. See
  [technical/cli.md > Line editor](technical/cli.md#line-editor-cmdjenniferlineeditgo-cmdjenniferhistorygo).
- **Auto-loaded `core` library** - new library kind, pre-imported at
  startup; writing `use core;` is a runtime error. Contents:
  `JENNIFER_VERSION` (a `git describe`-derived build-version constant)
  and `len` (polymorphic over strings now; lists/maps in M6). `len`
  moved here from `strings`. See [libraries/core.md](libraries/core.md)
  and [technical/cli.md > Version injection](technical/cli.md#version-injection).
- **Formatter** - `jennifer fmt` re-emits canonical source per
  [user-guide/style-guide.md](user-guide/style-guide.md). Token-level walker so file imports and
  user-written parentheses survive. See
  [technical/cli.md > Formatter](technical/cli.md#formatter-cmdjenniferfmtgo).
- **Inspection subcommands** - `jennifer tokens <file>` dumps the lexer
  output; `jennifer ast <file>` dumps the preprocessed AST as JSON.
  See [technical/cli.md > Inspection](technical/cli.md#inspection-tokens-and-ast).
- **Underscore-in-constants** - constant names became `[A-Z]+(_[A-Z]+)*`,
  enabling `MAX_RETRIES` and the `JENNIFER_VERSION` rename. See
  [technical/lexer.md > Identifier rule](technical/lexer.md#identifier-rule).
- **Documentation overhaul** - `docs/technical.md` split into
  `docs/technical/<topic>.md`; `docs/lib_*.md` moved to
  `docs/libraries/`; new `docs/user-guide/style-guide.md`.

---

## M6 - Lists and maps

**Status:** done.

Two new compound types - `list` and `map` - plus the strings library
functions deferred until compound types existed.

- **Syntax**: `def xs as list of int init [1, 2, 3];`,
  `def m as map of string to int init {"a": 1};`. Index read/write
  `$xs[i]`, `$m["k"]`, chains `$g[i][j]`. Iteration via
  `for (def x in $coll) { ... }` (new keyword `in`). New tokens
  `[ ] :` and keywords `list`, `map`, `of`, `to`, `in`.
- **Semantics**: value-typed (copy on assignment and on
  function-parameter binding; no aliasing); `const` is deep
  (`$NUMS[0] = ...` is a runtime error if `NUMS` is `const`);
  out-of-bounds list reads/writes and missing map keys are positioned
  runtime errors; map iteration is insertion-order deterministic.
- **Type system**: `parser.Type` became a recursive struct
  (`Element`, `KeyType`, `ValType` `*Type` slots), so nesting like
  `list of list of int` falls out without depth cap. 3+ levels is a
  documented code smell.
- **Stdlib**: `core.len` extended to lists and maps; `core.has(m, key)`
  for membership tests; `strings.split`, `strings.chars`,
  `strings.join` finished.
- **Tooling**: formatter handles `[...]` / `{...}` per
  [user-guide/style-guide.md](user-guide/style-guide.md) (no inner padding, space after `,`/`:`,
  block-vs-map disambiguation via a small brace stack); AST JSON
  emitter handles `ListLit`, `MapLit`, `IndexExpr`, `IndexAssignStmt`,
  `ForEachStmt`.

See [user-guide/types-and-values.md > Lists and maps](user-guide/types-and-values.md#lists-and-maps) for
the user-facing tour, and
[technical/grammar.md](technical/grammar.md) /
[technical/interpreter.md](technical/interpreter.md) for the
implementation contract.

---

## M7 - printf modifiers, stdin input, comment/division swap

**Status:** done.

A breaking syntax change to free up `//` for integer division and to
allow shebangs, the long-promised format-verb modifier system, and
the first stdin-reading builtins.

- **Comments and integer division** (**BREAKING**). Line comments
  moved from `//` to `#`, freeing `//` for floor division (Python 3
  shape). `div` keyword removed. A Jennifer file can now begin with
  `#!/usr/bin/env -S jennifer run`.
- **`(s)printf` format-verb modifiers.** Each format verb except `%v`
  accepts a pipe-separated, order-independent flag list:
  `%verb[|key=value]*`. Modifiers shape *presentation* only - data
  transformations (`case=upper` on strings, markdown rendering, etc.)
  are explicitly out of scope; libraries do that work. Verbs gained:
  `pad`/`max`/`align`/`mode` (`%s`); `pad`/`fill`/`align`/`base`/
  `sign`/`group`/`sep` (`%d`); `prec`/`trim`/`sci`/`pad`/`align`/
  `sign` (`%f`); `case` (`%t`); shared `null=empty|null|literal(...)`
  across all four typed verbs. `%v` deliberately takes none.
- **Format-string breaking change.** `|` immediately after a verb
  now starts a modifier list. Pre-M7 strings with `|` as a literal
  separator (`"%d|%d"`) need either a different separator or the
  `||` escape (parallels `%%`).
- **`io` stdin input.** New builtins `readLine()`,
  `readLine(prompt)`, `eof()` - one-line-at-a-time reads with an
  explicit EOF predicate (`while (not eof()) { ... }`). Refuses
  inside the REPL since the line editor owns stdin.
- **Internals.** Builtin signature changed from
  `func(out io.Writer, args)` to `func(ctx BuiltinCtx, args)` so
  stdin and the REPL flag are plumbed symmetrically with stdout.
  Mechanical refactor across the ~30 existing builtins.

See:
- [libraries/io.md](libraries/io.md) - full modifier and input reference.
- [technical/lexer.md](technical/lexer.md) and
  [technical/grammar.md](technical/grammar.md) - the comment / division
  syntax change.
- [technical/rejected.md](technical/rejected.md) - what the modifier
  system deliberately doesn't do (data transformations, `%a`
  aggregate, `null=sql`/`null=skip`) and why the literal-pipe
  lookahead alternative was turned down.
- [technical/interpreter.md > Builtins and libraries](technical/interpreter.md#builtins-and-libraries) - the `BuiltinCtx` signature.

---

## M8 - System library namespacing

The flat-namespace model is fine for the essential libraries today
(`io`, `convert`, `math`, `strings`, plus the auto-loaded `core`) -
they're small, names are carefully chosen, collisions are unlikely. It will not scale to domain
libraries like `regex`, `net`, `bio`, or `crypto`, where the chance of
two libraries shipping a `len` / `parse` / `encode` is high. M8 settles
the policy before the first domain library lands.

**Decision (to confirm at start of M8):** hybrid model. Essential
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
- **Aliasing is a rename, not an addition.**
  ```jennifer
  use bio as b;
  printf("%d\n", b.translateLen($seq));
  bio.translateLen($seq);   # error: unknown namespace `bio`
  ```
  After `use bio as b;`, only `b.` resolves. `bio.` errors with a
  "did you mean `b`?" hint. Matches Python's `import foo as bar`
  shadowing of `foo`; matches Jennifer's "one way per thing" stance.
  Adds an `AsName` to `ImportStmt`.
- **Namespace name as a reserved identifier.** A bare `use bio;`
  reserves `bio` as a namespace-prefix identifier - `func bio() {}`
  is then rejected with `shadows imported namespace 'bio'`. With
  `use bio as b;`, only `b` is reserved, freeing `bio` as a regular
  method name. Style guide will note "don't reuse a library's
  canonical name even when aliased - it's confusing"; soft rule,
  not enforced.
- **Migration of existing libraries:** none. All five essentials stay
  flat. The change is additive; existing source keeps working
  unchanged.
- **No-shadowing rule:** scoped to a namespace. `bio.len` does NOT
  collide with `strings.len`. A user method named `translate` collides
  with `bio.translate` only if it's referenced bare; it does not
  conflict with the qualified form. Document the resolution rule
  carefully in `docs/technical/interpreter.md`.

### Deferred from M8

- **File-import aliasing** (`import "templateengine.j" as jinja;`).
  Today `import "file.j";` is a textual splice at the preprocessor
  level - the imported file's defs/methods become globals, with no
  module boundary. Adding `as jinja` implies namespaced user methods,
  which Jennifer doesn't have any infrastructure for. Either
  (a) prefix-splice (rewrite tokens so `render` becomes
  `jinja.render`; fragile under cross-file references), or (b) real
  modules with per-file environments and explicit exports - its own
  milestone. M8's system-library namespacing works because builtins
  are registered through a Go API that already has a `lib` tag;
  file imports have no equivalent and would need to invent one. File
  imports stay unnamespaced until a dedicated modules milestone.

**Test strategy.** No real namespaced library is shipped in M8 (all
five essentials stay flat), so tests register **synthetic** namespaced
builtins inline through the public Go API
(`in.RegisterNamespaced("bio", "translate", fn)`). The parser sees a
qualified call as `bio.translate(...)` regardless of whether the
namespace points at real code, so this covers the full pipeline
without a stub package on disk. Concretely:

- **Parser:** `QualifiedCallExpr` parses `IDENT.IDENT(...)`; table-driven
  cases include qualified constant refs (`bio.STOPS`) and rejection of
  qualified-assign (`bio.x = 1` is a parse error).
- **Interpreter:** register synthetic namespaced builtins, then
  exercise lookup hits, lookup misses, `use bio as b;` makes `b.`
  resolve and `bio.` error with a "did you mean `b`?" hint, the
  namespace-identifier reservation rule (`func bio() {}` errors after
  bare `use bio;`, passes after aliased `use bio as b;`), `use`-gating
  before any qualified call is allowed, and dot-separated constants.
- **Formatter:** round-trip a program containing qualified calls;
  verify no space around `.`.
- **AST JSON dump:** emit `QualifiedCallExpr` for a synthetic source.
- **REPL:** `EvalInteractive` path through a qualified call; multi-line
  continuation crossing a `.` boundary.
- **End-to-end:** one test in `cmd/jennifer/examples_test.go` (or a
  new `namespace_test.go`) registers a test-only namespaced library
  and runs a small source string; no `examples/*.j` golden file
  required.
- **Regression:** the five shipping essentials act as the "namespaces
  don't break flat builtins" sanity suite - their existing tests
  must keep passing unchanged.

**Docs to write at end of M8:**

- Update `docs/libraries/index.md` with the namespace policy and a
  rule for library authors ("is your library essential or domain?
  if domain, set a namespace").
- Update `docs/technical/grammar.md` EBNF.
- Update `docs/technical/interpreter.md` Builtins-and-libraries
  section to describe the lookup rule.
- New section in `docs/user-guide/style-guide.md`: how to write namespaced calls
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

## M9 - Collection operations

The `list`/`map` types shipped in M6 with literals, indexing, and
iteration but no manipulation helpers. M9 adds two opt-in libraries
that cover the rest:

- **`lists` library** (`use lists;`): `push`, `pop`, `first`, `last`,
  `head`, `tail`, `reverse`, `sort`, `contains`, `concat`, `slice`.
  All functions return a **new list** (no mutation through reference;
  matches Jennifer's value semantics) - callers write
  `$xs = push($xs, item);`. `sort` works on primitive element types;
  comparator-based sort is deferred until methods are first-class
  values.
- **`maps` library** (`use maps;`): `keys`, `values`, `delete`,
  `merge`. Same pattern - functions return a new map, no in-place
  mutation. `delete($m, key)` returns a shrunk map; `merge($a, $b)`
  returns a new map with `$b` overlaying `$a`.

**Sugar: `$xs[] = item;` syntax-level append.** For the 80% case of
"build a list by appending", introduce a new write target where `[]`
means "the position just past the end". Mechanically the same as
`push($xs, item)` but cheaper to write. Reuses index-assign machinery
(the binding gets rewritten to the extended list); no new keyword. The
parser learns to accept `LBRACKET RBRACKET` as a special index in
write position. Reads of `$xs[]` are not valid (no obvious meaning).

**`core.has` stays map-only**, as already documented. List
membership is `lists.contains($xs, item)` - matches the
`strings.contains($s, $sub)` shape (haystack first, needle second; PHP's
`in_array($needle, $haystack)` argument order is famously confusing
and deliberately not adopted).

**Naming convention** (resolved at start of M9): flat names within
each library following the existing `strings.contains` /
`strings.startsWith` style. No `array_*` PHP-style prefix. Once M8
namespacing lands, qualified `lists.contains($xs, x)` form becomes
available; until then, `use lists;` + bare `contains` works.

**Why now rather than ship with M6**: M6 already pushed a lot of new
machinery (parser, runtime, type stamping, formatter rules). Punting
helpers gave us time to confirm the value-semantics + return-new-list
shape against real example code (the showcase, wordcount). With that
validated, M9 is mostly stdlib registration: each function is one Go
helper. The `$xs[] = item;` sugar is the only language-level change.

**Implementation notes:**

- Each library function is a builtin: `Register("lists", "push", pushFn)`.
- `pushFn` takes `(list, item) -> list of T` - copies the input list,
  appends the item, returns it. The caller's assignment writes the
  result back; the original input is untouched. Cost: O(n) per push,
  acceptable at Jennifer's scale; copy-on-write is a future optimization.
- The `$xs[] = item;` sugar parses as a new AST node `AppendStmt {
  Target *VarExpr, Value Expr }`. Interpreter walks identically to
  IndexAssignStmt but writes one slot past the end.
- `sort` uses Go's `sort.Slice` with a kind-aware less function; mixed
  types in one list error (kind already enforced at element-write
  time, so this should never happen in practice).

**Docs to update:**

- `docs/libraries/lists.md` (new file) - covers every function with
  signature, semantics, example, and the "returns new list" rule.
- `docs/libraries/maps.md` (new file) - same pattern.
- `docs/libraries/index.md` - add the two new entries to the catalog.
- `docs/user-guide/imports.md` - mention `lists` / `maps` in the libraries
  table; cover the `$xs[] = ...;` sugar in the Lists and maps section.
- `docs/technical/grammar.md` - new EBNF for the append form and
  new `AppendStmt` AST node.
- `docs/user-guide/style-guide.md` - no whitespace inside `$xs[]` (consistent
  with `$xs[0]`).
- `examples/showcase.j` and/or `examples/wordcount.j` - exercise
  push/pop/sort/contains so the goldens cover the new surface.

**Exit criterion:** the showcase exercises `push`, `pop`, `sort`,
`contains`, and the `$xs[]` sugar end-to-end; round-trips through
`fmt` are stable; `lists` and `maps` libraries both show up in
`jennifer help`'s "available libraries" error list.

---

## M10 - Domain libraries

- **File library** (`use fs;`)
- **OS library** (`use os;`)
- **Regex library** (`use regex;`)
- **Network library** (`use net;`)
- **Crypto library** (`use crypto;`)
- **Random library** (`use random;`)

All M10 libraries are domain libraries and ship with namespaces
(per M8): `fs.read`, `os.getenv`, `regex.match`, `net.dial`,
`crypto.sha256`. This is the first real test of the namespacing
convention - if any of these prove awkward (`crypto.sha256` reads
fine; `fs.read` collides with the urge to call it `readFile`), revisit
in early M10 before too many call sites lock in.

---

## Future directions

### Mid-term goals

- **CI/CD:** Github Actions pipeline for automatic testing and release
- **Packages** for Debian and Archlinux
- **Pages** - create github pages

### Long-term goals

Not committed to a milestone yet, but the code should not foreclose them.

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

### Deferred from M5

- **Comment preservation in `fmt`.** Lexer would need to carry `#` and
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

### Deferred from M7

- **Byte reads from stdin** (`readBytes(n)`) - waits on `fs.read` to
  settle the binary-data representation (new `bytes` primitive vs.
  `list of int` with elements in `[0, 255]`). `eof()` is already
  byte-level and will compose with whichever shape wins.
- **`readChars(n)`** - separate design from byte reads; defer for
  the same reason.
- **`lines()` returning the whole stream as a list** - additive,
  not required by the streaming `readLine()` + `eof()` idiom.
- **File handles and non-stdin sources** - owned by a future `fs`
  library.
