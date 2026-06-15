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
  moved here from `strings`. (M15.4 later promoted `len` to a
  language built-in and deleted `core`; see M15.4 for the
  migration.) Version injection details at
  [technical/cli.md](technical/cli.md#version-injection).
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

**Status:** done.

A hybrid namespace model so domain libraries can ship without
polluting the bare-name pool, plus the first real namespaced
library (`os`) so the machinery has a non-synthetic exercise.

- **Hybrid model.** Essential libraries (`io`, `convert`, `math`,
  `strings`, auto-loaded `core`) stay flat - their builtins are
  bare names. Domain libraries register through a new namespaced
  API (`RegisterNamespaced` / `RegisterNamespacedConst`) and are
  addressed by `prefix.name(...)` / `prefix.NAME`. The library's
  name doubles as the namespace prefix.
- **Qualified calls and constants.** New AST nodes
  `QualifiedCallExpr` and `QualifiedConstRefExpr`; parsed as
  `IDENT "." IDENT` (then `(` decides). Lookup is keyed by
  `(namespace, name)` and gated by `use lib;`.
- **`use NAME as ALIAS;` aliasing.** Optional `as` clause on
  `use`. Rename-not-addition: after `use bio as b;` only `b.`
  resolves, `bio.foo()` errors with a "did you mean `b`?" hint;
  the canonical name `bio` is freed for ordinary identifier use.
  Matches Python's `import foo as bar`. Aliasing is rejected for
  flat libraries (`use math as m;` errors as meaningless).
- **Namespace prefix is a reserved identifier.** After bare
  `use bio;`, `func bio() {}` errors with `shadows imported
  namespace 'bio'`. After `use bio as b;`, only `b` is reserved.
- **No migration.** The change is purely additive; all five flat
  essentials continue to work unchanged.
- **Demo library `os` (minimal slice).** First namespaced
  library: `os.platform() -> string`, `os.getEnv(name) -> string`,
  `os.JENNIFER_LF`, `os.JENNIFER_OS`. Two functions plus two
  constants - enough to exercise namespaced zero-arg calls,
  namespaced calls with arguments, namespaced constants, and
  aliasing end-to-end. Expands in M15.1.

See:
- [libraries/os.md](libraries/os.md) - the shipping demo library.
- [libraries/index.md](libraries/index.md) - flat vs namespaced
  policy and the rule for library authors.
- [user-guide/imports.md > Namespaced libraries and aliasing](user-guide/imports.md#namespaced-libraries-and-aliasing) -
  user-facing reference for `use NAME [as ALIAS];` and qualified
  calls.
- [user-guide/style-guide.md > Namespaced calls](user-guide/style-guide.md#namespaced-calls) -
  spacing convention around `.`.
- [technical/grammar.md](technical/grammar.md) - EBNF for
  `qualifiedCall` / `qualifiedConstRef` and the `use ... as ...`
  shape; AST table entries for the new nodes.
- [technical/interpreter.md > Namespaced libraries (M8)](technical/interpreter.md#namespaced-libraries-m8) -
  registration API, `nsPrefixes` / `nsAliasedAway` resolution
  tables, no-shadowing rule for namespace prefixes.

---

## M9 - Collection operations

**Status:** done.

Two new namespaced libraries cover the M6-deferred list/map
manipulation helpers, a small append sugar shortens the common
write pattern, and two follow-on breaking changes tidy up the
flat-vs-namespaced split.

- **`lists` library** (`use lists;`, namespaced). `lists.push`,
  `lists.pop`, `lists.first`, `lists.last`, `lists.head`,
  `lists.tail`, `lists.reverse`, `lists.sort`, `lists.contains`,
  `lists.concat`, `lists.slice`. Non-mutating - every function
  returns a new list. `sort` accepts numeric, string, or bool
  elements (mixed int/float promotes; other mixes error);
  comparator-based sort is deferred until methods are first-class.
- **`maps` library** (`use maps;`, namespaced). `maps.keys`,
  `maps.values`, `maps.has`, `maps.delete`, `maps.merge`. Same
  shape. `maps.delete` of a missing key errors (strict at
  boundaries, matching `$m[missing]`); `maps.merge` layers the
  second arg over the first.
- **Sugar: `$xs[] = item;`** - write-only target meaning "just past
  the end of the list". Equivalent to
  `$xs = lists.push($xs, item);`. Reads of `$xs[]` and chained
  forms (`$xs[0][]`) are parse errors; non-list targets error at
  runtime. New AST node `AppendStmt`.
- **BREAKING:** `has()` moved from `core` to `maps` as
  `maps.has(m, key)`. Bare `has(...)` callers now need
  `use maps;` and the qualified form. `has` was the only
  non-polymorphic name in core; `len` stays because it genuinely
  spans string / list / map.
- **BREAKING:** `strings` library moved from flat to namespaced.
  `upper(s)` → `strings.upper(s)`, `contains(s, sub)` →
  `strings.contains(s, sub)`, etc. across all 15 functions.
  `use strings;` itself is unchanged. The M8 library-author rule
  named exactly these collision-prone verbs (`contains`, `split`,
  `replace`, `join`); acting on it now keeps callers off the wrong
  shape before more libraries arrive. After M9 the remaining flat
  libraries are `io`, `convert`, `math`, and auto-loaded `core`.

See:
- [libraries/lists.md](libraries/lists.md) /
  [libraries/maps.md](libraries/maps.md) - function reference for
  each new library.
- [libraries/strings.md](libraries/strings.md) - now namespaced
  (M9 migration note at top).
- [libraries/index.md](libraries/index.md) - updated flat-vs-namespaced
  catalog and the library-author rule.
- [user-guide/imports.md](user-guide/imports.md) and
  [user-guide/types-and-values.md > The `$xs[]` append sugar](user-guide/types-and-values.md#the-xs-append-sugar) -
  user-facing reference.
- [technical/grammar.md](technical/grammar.md) - EBNF and AST entry
  for `AppendStmt`.

---

The next phase splits into four arcs after two architectural
prerequisites: M10 lands the namespace-first library architecture
that the rest of the standard library will be built on; Phase A
(M11-M13) finishes the language so libraries have something to
stand on; M14 closes the lexer-side gap (`fmt` losing comments
and shebangs) so the first wave of struct-using libraries can
ship with doc-comments intact; Phase B (M15.x) ships the
foundational libraries that every Jennifer program needs,
finishing with **M15.8 - the first public release** (CI, prebuilt
binaries, .deb / pacman / AUR packaging); Phase C (M16.x) ships
I/O libraries on top of the now-released foundation; Phase D
(M17-M20) ships the higher-level ecosystem (Jennifer-coded
libraries, the module system that unblocks them, crypto, a
server). Phase E (WASM and specialised domains) is the long
horizon.

The library milestones use sub-numbering (M15.1, M15.2, ...) so
each library ships and is reviewed independently. This is the
first time we use sub-milestones; the practice is justified
because each library is small enough to land in a single sitting
once the language foundation is in place.

---

## M10 - Namespace-first library architecture

**Status:** done.

A pre-language-completion API-shape correction: every library is
now namespaced, with bare-name globals reserved as a narrow
`core`-only exception. Small implementation surface, large API
shape; pre-1.0 is the window for this kind of change.

- **BREAKING:** `io`, `math`, `convert` migrate to
  namespaced-only. `printf(x)` → `io.printf(x)`,
  `sqrt(x)` → `math.sqrt(x)`, etc. The "io is special, keep
  it flat" alternative was considered and rejected at kickoff
  to keep a uniform "every call carries its library name"
  rule. `strings`, `lists`, `maps`, `os` were already
  namespaced (M9/M8).
- **BREAKING:** `convert`'s four conversion callees are renamed
  to `convert.toInt`, `convert.toFloat`, `convert.toString`,
  `convert.toBool` so they don't collide with the type
  keywords (`int`, `float`, `string`, `bool`); `convert.typeOf`
  keeps its name. The `to`-prefix also reads as English
  ("convert to int") at the call site.
- **BREAKING:** file-splice keyword `import` → `include`.
  `include "x.j";` is the textual splice; the `import`
  keyword is reserved for the M17 module system and produces
  a migration-hint error today. Mixing-mistake diagnostics
  updated.
- **BREAKING for embedders:** registration API renamed.
  `Register` / `RegisterConst` → `RegisterGlobal` /
  `RegisterGlobalConst`, making their role explicit ("expose
  this name globally"). The namespaced API
  (`RegisterNamespaced` / `RegisterNamespacedConst`) keeps
  its name and is the recommended default. Per-library
  storage (`globalFnsByLib`, `globalConstsByLib`) so two
  libraries with the same global name can no longer silently
  overwrite each other at Install time; the resolution map
  is populated by `processImports` when a library activates.
- **`math` absorbs the planned non-crypto random helpers**:
  `math.rand()`, `math.randInt(lo, hi)`, `math.randSeed(n)`.
  Three functions don't justify their own library under the
  new threshold (next bullet); pseudo-random fits `math`'s
  pure-numeric charter. The crypto-grade variant still ships
  in M19 `crypto`. The originally planned M14.2 `random`
  library is removed.
- **`core` is the only library publishing bare-name globals.**
  `len` and `JENNIFER_VERSION` only - no `core.len` /
  `core.JENNIFER_VERSION` qualified form, because shipping the
  same name two ways violates stance #1. `core` is the
  auto-loaded escape hatch, and its asymmetric exposure is
  the whole point.
- **Three globals-publishing rules in `processImports`**, all
  forward-looking (inert today since `core` is the only
  globals-publishing library and can't be `use`d):
  1. Duplicate `use` of a globals-publishing library is
     rejected (`library 'X' already in scope`); REPL no-ops a
     repeat.
  2. `use X as Y;` where `X` has globals but no namespaced
     names is rejected as meaningless.
  3. Two active libraries publishing the same global name are
     rejected at the second `use`
     (`library "B" collides with already-active library "A"
     on global "VER"`). The pre-M10 flat-only-alias-meaningless
     check is removed for the general case but kept as rule 2.
- **Library-author guidance updated.** The
  `docs/libraries/index.md` "flat vs namespaced" framing is
  retired; the new policy is "every library is namespaced;
  only `core` ships globals via `RegisterGlobal`." The
  "deserves its own library" threshold is raised from M8's
  "3+" to **"5+ functions or constants"**: anything smaller
  folds into the most-related existing library. The non-crypto
  random helpers (3 functions) are the first case the new rule
  caught.

See:
- [libraries/io.md](libraries/io.md),
  [libraries/math.md](libraries/math.md),
  [libraries/convert.md](libraries/convert.md) - migrated library
  references.
- [libraries/index.md](libraries/index.md) - retired
  flat-vs-namespaced framing; new library-author policy and
  5+ threshold.
- [user-guide/imports.md](user-guide/imports.md) - `use` and
  `include` keyword reference; namespaced-call and aliasing
  rules.
- [user-guide/types-and-values.md](user-guide/types-and-values.md) -
  `convert.toInt` / `convert.toFloat` example placement in the
  "explicit conversions" section.
- [technical/rejected.md](technical/rejected.md) - "Methods
  on structs" (M14.3 trigger; recorded here in M10's wake
  because M10's review touched the same call-shape question)
  and other related rejected alternatives.

No new language features land here - that's M11.

---

**Phase A: language completion (M11-M13).** These three milestones
close the biggest daily-use gaps and add the foundational types every
later library needs.

## M11 - Control-flow completion

**Status:** done.

Closes the biggest daily-use gap in the language and rounds out the
printf modifier table at the same time. Five new keywords (`break`,
`continue`, `repeat`, `until`, `exit`) and two new printf features.

- **`break;` / `continue;`** in every loop kind
  (`while`/`for`/`for-each`/`repeat`). Innermost loop only; misuse
  outside a loop or across a method-call boundary is a positioned
  runtime error. `continue` in C-style `for` still runs the step
  before re-checking the condition (matches C/Go).
- **`repeat { } until (cond);`** post-test loop. New keywords
  `repeat` and `until`; `do { } while ...` considered and rejected
  because the inverted condition is the whole point of switching
  to `until`.
- **`exit;` / `exit EXPR;`** terminate the whole program (exit code 0
  / EXPR-as-int). Distinct from `return` (method-scoped): skips every
  caller frame and remaining top-level statement. Implemented as an
  `ExitSignal` sentinel error the CLI translates into the OS exit
  status.
- **Bundled: printf `%s|align=center`** rounds out the align set.
  Rejected on every other typed verb (centred numbers break columnar
  output).
- **Bundled: printf `%a` aggregate verb** for lists and maps
  (deferred from M7; unblocked by M6 + M9). Modifiers: `sep`, `kv`,
  `open`, `close`, `depth=N`, `null=skip`. The modifier-list parser
  was extended with a `"..."` quoted-value form (`%a|sep=", "`) so
  values can contain spaces / reserved characters; standard
  `\n \r \t \\ \"` escapes.
- **Post-dot name relaxation.** Reserved words read as identifiers in
  the name slot of a qualified call (`strings.repeat`,
  `lists.break` if anyone wrote one), preserving the `strings.repeat`
  library function after `repeat` was reserved as a loop keyword.

See:
- [user-guide/control-flow.md](user-guide/control-flow.md) -
  `repeat`/`until`, `break`/`continue` scope rules, `exit` vs
  `return`.
- [libraries/io.md](libraries/io.md) - `%a` modifier table,
  `%s|align=center` example, quoted modifier values.
- [technical/rejected.md](technical/rejected.md) - `%a|json=*` /
  `%a|xml=*` / `%a|yaml=*` (serialisation modifiers stayed rejected
  even after `%a` itself shipped) and the
  `do { } while` shape for the post-test loop.

## M12 - Bytes and bit operators

**Status:** done.

Adds the buffer-shaped primitive and the bit-twiddling vocabulary
the standard library needs for hashing, encoding, crypto, and
network code in later milestones.

- **New primitive type `bytes`** - mutable byte sequence; value
  semantics on assignment / parameter binding; deep-const. Reads
  yield `int` in `[0, 255]`; writes accept the same range and
  reject anything else. Append via the existing M9 `$b[] = byte;`
  sugar. `len($b)` returns the byte count.
- **New `convert.bytesFromString(s, codec)` and
  `convert.stringFromBytes(b, codec)`** - bytes ↔ string codecs.
  Only `"utf-8"` today (further codecs ship in M15.7 `encoding`).
  Invalid UTF-8 input is an error - no silent replacement
  characters.
- **Bit operators on `int`**: `& | ^ ~ << >>`. Python-style
  precedence (comparison < `|` < `^` < `&` < shifts < `+ -`),
  so `$x & 0xff == 0` parses as `($x & 0xff) == 0`. `~` is
  bitwise NOT. Shifts are arithmetic; negative count rejected;
  count >= 64 saturates to 0 / -1. `^` ships as a primitive
  operator (CPU primitive with unique algebraic properties -
  same justification `-` has against being composable from `+`
  and unary `-`).
- **Non-decimal integer literals**: hex `0xff`, octal `0o755`,
  binary `0b1010_0110`. `_` accepted between digits in any base
  (including decimal `1_000_000` and float mantissas). Never
  adjacent to the prefix or another `_`. Lexer-only change.
- **Resolves M7-deferred stdin builtins**:
  `io.readBytes(n) -> bytes` (exact n; partial at EOF then
  `io.eof()` becomes true) and `io.readChars(n) -> string`
  (n runes, UTF-8 decoded). Both compose with M7's `io.eof()`
  unchanged.

See:
- [user-guide/types-and-values.md](user-guide/types-and-values.md) -
  `bytes` type, value semantics, index-write rules.
- [libraries/convert.md](libraries/convert.md) - codec functions,
  UTF-8 strictness.
- [libraries/io.md](libraries/io.md) - `io.readBytes`,
  `io.readChars`.
- [user-guide/control-flow.md](user-guide/control-flow.md) -
  bit-operator precedence table.
- [user-guide/syntax.md](user-guide/syntax.md) - non-decimal
  literals + digit separator.

## M13 - Structs and catchable errors

**Status:** done.

The composite-data milestone, batched into two sub-milestones in
dependency order: M13.1 ships the struct mechanism, M13.2 ships
the error-handling design that uses it. Together they unblock
every library that wants composite returns and give the language
a recoverable-error story.

## M13.1 - Structs / records

**Status:** done.

- `def struct Name { field as type, ... };` at top level
  (hoisted before the first statement; duplicate names error in
  `Run`, silently redefine in the REPL).
- Literals `Name{ field: expr, ... }` with every field required;
  `def x as Name;` (no init) zero-fills, recursing through
  nested struct fields.
- Field read `$p.field`, write `$p.field = ...;`. Lvalue chains
  mix `[index]` and `.field` freely (`$L.from.x = 5;`,
  `$bag.items[0] = 99;`); index-assign and field-assign share
  one walker.
- Value semantics like lists/maps; `const` deep at any depth.
- Strict at boundaries: unknown struct type, missing / unknown
  field at literal, field-type mismatch on write, field access
  on a non-struct value are all positioned errors.

See:
- [user-guide/types-and-values.md](user-guide/types-and-values.md#structs) -
  language angle.
- [technical/interpreter.md](technical/interpreter.md#structs-m131) -
  runtime details (`KindStruct`, hoisting, unified lvalue walker).
- [technical/grammar.md](technical/grammar.md) - `structDef`,
  `structLit`, `fieldAssign`, mixed-tail lvalues.
- `examples/structs.j` standalone; `examples/showcase.j`
  `=== M13.1 structs ===` section.

## M13.2 - `try` / `catch` / `throw`

**Status:** done.

Catchable errors. New keywords `try`, `catch`, `throw`. Depends
on M13.1 because the canonical error value is a struct.

- `try { body } catch (NAME) { handler }` runs the body and, on
  a catchable error, binds the thrown value to `$NAME` in a
  fresh per-handler scope.
- `throw EXPR;` raises any value; convention is the auto-hoisted
  `Error{kind, message, file, line, col}` struct.
- Runtime errors (out-of-bounds, missing key, type mismatch,
  etc.) are wrapped into the canonical `Error` shape on entry
  to the catch (`kind` defaults to `"runtime"` until sites
  opt in to specific tags); user code catches both kinds
  uniformly. `throw $err;` inside a catch re-raises to the
  enclosing `try`.
- **Not** catchable: `exit` (program-level escape, propagates
  through `try`); `return` / `break` / `continue` (control
  flow, flow through `try` unchanged).
- No `finally` and no typed catch in v1.
- Internals: `ErrorSignal` sentinel parallels `ExitSignal`;
  `runtimeError.Kind` field threads the symbolic tag; user code
  may not redefine the auto-hoisted `Error` struct.

See:
- [user-guide/control-flow.md](user-guide/control-flow.md#try-catch-throw) -
  language angle.
- [technical/interpreter.md](technical/interpreter.md#catchable-errors-m132) -
  runtime details (`ErrorSignal`, wrapping, flow passthrough).
- [technical/grammar.md](technical/grammar.md) - `tryStmt`,
  `throwStmt`.
- `examples/trycatch.j` standalone; `examples/showcase.j`
  `=== M13.2 try/catch ===` section.

---

## M14 - Lexer comment + blank-line preservation

**Status:** done.

Closes the two M5-deferred items (`fmt drops comments`, `fmt
drops blank lines`). No language change - the runtime still
never sees comments.

- Lexer emits trivia tokens (`TOKEN_COMMENT_LINE`,
  `TOKEN_COMMENT_BLOCK`, `TOKEN_COMMENT_SHEBANG`,
  `TOKEN_BLANK_LINE`). Shebang on line 1 col 1 is its own kind;
  runs of blank lines collapse to one.
- Preprocessor and parser strip trivia at entry; `jennifer fmt`
  walks the raw lexer stream and re-emits trivia via a dedicated
  `emitTrivia` path that doesn't disturb the surrounding state
  machine.
- Block comments nest via depth counter; unterminated comments
  error at the outermost `/*`.
- Token-level over AST-level: the original spec proposed
  AST-attached `LeadingComments` / `TrailingComment` slots and a
  `jennifer ast --with-comments` flag - dropped in favour of the
  simpler token-level path. Add them back if a future doc
  generator needs structured per-statement attachment.

See:
- [user-guide/style-guide.md](user-guide/style-guide.md#comments) -
  Comments section (block comments nest; inline-comment spacing
  exception).
- [technical/lexer.md](technical/lexer.md#comments) - trivia
  emission, shebang detection, nesting depth counter.
- [technical/cli.md](technical/cli.md) - `fmt`'s trivia re-emission.

---

**Phase B: foundational libraries (M15.x).** Small,
frequently-used libraries grouped under M15 with sub-numbering.
The leading M15.0 slot is the "wrap-up of existing libraries"
(extensions to M8 / M9 / M10 libraries that depend on language
features added since); later slots ship a new library each.
M15.8 closes the phase by making the result installable before
Phase C starts adding I/O on top.

## M15 - foundational libraries + first public release

**Status:** done. All nine sub-milestones shipped. Three are
language work (M15.2, M15.4), the rest are library / tooling /
release work. Two recurring patterns surfaced in the shipped
APIs and are worth remembering:
- **Codec-table shape** (algorithm/format/codec passed as a
  string argument). Used by `hash.compute(b, algo)`,
  `crc.compute(b, algo)`, `encoding.encode(s, codec)`,
  `encoding.toText(b, format)`. Originally adopted because
  Jennifer's letters-only identifier rule rejects digits in
  method names (so `hash.md5(...)` won't parse), but it also
  honours stance #1 by collapsing parallel verbs into one.
- **Integer-handle struct for opaque resources** (M15.2's
  namespaced-struct mechanism + a single `id as int` field
  indexing into a Go-side map). Used by `os.Process`,
  `hash.Stream`, `crc.Stream`.

### M15.0 - existing-library extensions

**Done.** Two extensions to the M9 `lists` library that needed
post-M14 language features: `lists.shuffle(xs)` (Fisher-Yates,
respects `math.randSeed`) and `lists.range(start, end[, step])`
(half-open, deliberate single-arg omission per stance #2). See
[libraries/lists.md](libraries/lists.md#shuffle) and
[technical/design-decisions.md > Half-open ranges](technical/design-decisions.md#half-open-ranges)
for the half-vs-closed-range rationale.

### M15.1 - `os` + `meta` (process metadata)

**Done.** Reshapes the M8-era `os` surface around one rule:
immutable per-run host facts are uppercase constants
(`PLATFORM`, `ARCH`, `EOL`, `DIRSEP`, `PATHSEP`, `ARGS`),
operations are functions (`getEnv`, `hasFlag`, `flag`). Drops
the `JENNIFER_` prefix that only made sense for bare-global use,
and introduces a new `meta` library for interpreter-self-identity
constants (`VERSION`, `BUILD`). CLI forwards trailing args to
`os.ARGS` (script path at index 0). Breaking renames
(`JENNIFER_VERSION` -> `meta.VERSION`, `os.platform()` ->
`os.PLATFORM`, `os.JENNIFER_OS` -> `os.PLATFORM`,
`os.JENNIFER_LF` -> `os.EOL`); old names now produce plain
"undefined" errors with no rename-hint. See
[libraries/os.md](libraries/os.md) and
[libraries/meta.md](libraries/meta.md).

### M15.2 - Language: library-provided namespaced struct types

**Done.** Language work slotted inside Phase B because the next
wave of libraries (M15.3 `os.{Result,Process}`, M15.5 `time.*`,
M15.6 `hash.Stream`/`crc.Stream`, future M16.1 `fs`, M16.2 `net`)
all need their own struct types and M13.1 only handled bare-IDENT
names. Adds `def x as lib.Name;` type syntax,
`lib.Name{field: ...}` literals, and the Go-side
`Interpreter.RegisterNamespacedStruct` API. Reuses M13.1's value
semantics, deep-`const`, and strict-boundary machinery; only the
resolution path differs. User code can't register structs (Go-side
only); methods on structs and inheritance stay out of scope. See
[technical/interpreter.md > Library-provided namespaced structs](technical/interpreter.md#library-provided-namespaced-structs-m152).

### M15.3 - `os` external-program execution

**Done.** First library to consume the M15.2
namespaced-struct mechanism. Surface: `os.Result {exitCode,
stdout, stderr}` + `os.Process {pid}` as the public types;
`os.run(argv) -> Result` blocking, `os.spawn(argv) -> Process`
non-blocking, `os.wait/poll/kill(p)` for handle ops. `argv` is
always `list of string` (no shell parsing; explicit
`["sh", "-c", $cmd]` for that hop). Non-zero exit codes are
values, not errors. **TinyGo limitation**: TinyGo's runtime
doesn't implement `os/exec`, so the shipping `jennifer` binary
returns a friendly "rebuild with `jennifer-go`" error instead of
panicking - first place the two-binary story becomes
user-visible. See
[libraries/os.md > External programs](libraries/os.md#external-programs)
and `examples/exec.j`.

### M15.4 - Language: `len` built-in, `core` removed

**Done.** Promoted `len(EXPR)` from the auto-loaded `core` library
to a reserved keyword + language primary expression (polymorphic
over string / list / map / bytes). Deleted `internal/lib/core/`
entirely; `use core;` now returns a friendly migration error
pointing at the built-in and at `meta.VERSION` / `meta.BUILD`.
Stance #2 ("explicit over implicit") now applies uniformly: every
library name lives behind a `use NAME;` prefix, no exceptions. See
[technical/design-decisions.md > len is a language built-in](technical/design-decisions.md#len-is-a-language-built-in-not-a-library).

### M15.5 - `time`

**Done.** One opt-in library spanning instants, durations,
fixed-offset zones, strftime format/parse, and ISO 8601
round-trip. Three namespaced structs: `time.Time {nanos, offset}`
(fields private API), `time.Duration {nanos}`, and
`time.Zone {offset, name}` (fields public, so the M18.4
`timezones.j` companion can build them). Granularity (date-only
vs time-of-day-only) is a property of formatting, not the value
type. Unix timestamps are constructor / accessor pairs, not a
separate type. IANA names and DST are deferred to M18.4. Three
sub-milestones: **M15.5.1** core type + Unix + calendar + 1-based
ISO weekday + arithmetic / comparison; **M15.5.2** strftime
format/parse (chosen over Go's reference-time style for
familiarity; v1 verbs `%Y %m %d %H %M %S %z %a %A %b %B %j %u
%%`) + `time.zone(offset, name)` + `time.inZone` + the `time.UTC`
constant coexisting with the `time.utc()` function via
case-sensitive lookup + `time.iso` / `time.fromIso` RFC 3339
round-trip; **M15.5.3** the `examples/benchmark.j` side-by-side
TinyGo-vs-Go workload suite (eight workloads; the original spec's
"Sieve of Eratosthenes" became trial-division because Jennifer's
value-semantic list mutation turns the sieve into O(N^2)). See
[libraries/time.md](libraries/time.md),
`examples/time{,-format,benchmark}.j`.

### M15.6 - `hash` and `crc`

**Done.** Two opt-in libraries with parallel surfaces: `hash`
for cryptographic-style digests (`"md5"`, `"sha1"`, `"sha256"`),
`crc` for non-cryptographic checksums (`"crc32"` IEEE, `"crc64"`
with Go's `crc64.ECMA` polynomial). Output is raw `bytes`
(big-endian 4 / 8 bytes for CRC, natural width for hash). The
split keeps "transport integrity" vs "content addressing"
visible at the import line and matches Go's `crypto/*` vs
`hash/crc*` arrangement. Both libraries ship the codec-table
shape: `compute(b, algo)` one-shot, `stream(algo)` +
`update($s, $b)` + `finalize($s) -> bytes` for chunked input.
Streaming reuses the
[integer-handle pattern from `os.Process`](#m153---os-external-program-execution).
No convenience wrappers like `hash.md5String` or `hash.computeHex`
(stance #1; users compose `convert.bytesFromString` and
`encoding.toText`). Struct hashing deferred (needs stable byte
serialization, its own design problem). See
[libraries/hash.md](libraries/hash.md),
[libraries/crc.md](libraries/crc.md), `examples/hash.j`.

### M15.7 - `encoding`

**Done.** Three-part surface: introspection
(`isAscii`/`lenBytes`/`lenRunes`), binary-to-text
(`toText`/`fromText` for `"hex"`/`"base64"`/`"base64-url"`), and
character codecs (`encode`/`decode`/`codecs`). The cross-kind
UTF-8 pair stays in `convert` (M12); `encoding` owns the
table-based codec proliferation. Four codecs ship: `"ascii"`,
`"latin-1"` (alias `"iso-8859-1"`), `"windows-1252"` (alias
`"cp1252"`), `"ebcdic"` (alias `"ibm-1047"`). The spec's per-format
verbs (`encoding.hex`, `encoding.base64`, ...) consolidated into
the codec-table pair to dodge the same digit-in-identifier rule
M15.6 hit. Codec names normalise case-insensitively (strip
`-`/`_`/spaces); format strings stay case-sensitive (smaller set).
Windows-1252's five canonically-undefined positions (0x81, 0x8D,
0x8F, 0x90, 0x9D) reject symmetrically on encode and decode.
Long-tail single-byte codecs (ISO-8859-{2..16},
Windows-{1250,1251,1253..1258}) parked in
[M24+](#m24---encoding-long-tail-codecs). See
[libraries/encoding.md](libraries/encoding.md),
`examples/encoding.j`.

### M15.8 - distribution + first public release

**Done.** Packaging / CI / release-engineering only; no
`.j`-source language change. Four sub-phases:

- **CI** (`.github/workflows/test.yml`, `release.yml`). PR gate
  runs `go vet` + `gofmt` + `go test ./...` + `make build` +
  per-binary smoke run + repo-wide em-dash scan. Release
  triggers on bare-semver tags (`[0-9]*.[0-9]*.[0-9]*`, no `v`
  prefix per project convention), cross-compiles both binaries
  for `linux/{amd64,arm64}` from one runner, QEMU-smoke-tests
  the non-native arch, runs the benchmark on amd64 so release
  notes carry fresh numbers, publishes a draft Release.
- **Packaging** under `packaging/{debian,arch,mime,man}/`.
  `scripts/build-deb.sh` produces the `.deb` (binaries +
  gzipped man pages + shared `text/x-jennifer` MIME definition
  + `update-mime-database` hooks). AUR ships `PKGBUILD-bin`
  (release tarball) and `PKGBUILD-git` (source-tracking) with
  a shared `.install` hook. Release pipeline auto-fills
  `PKGBUILD-bin` (real `pkgver` + real `sha256sums_*`) as a
  release asset so the AUR push is a one-step `curl`. The
  `.pacman` standalone artefact from the original spec was
  dropped - `PKGBUILD-bin` + `makepkg` covers the same need.
- **Docs site** via mdBook 0.5.3 (pinned, fetched via direct
  curl from `rust-lang/mdBook` releases - no third-party
  action). `book.toml` at repo root, `src = "docs"`,
  `docs/SUMMARY.md` maps the existing tree into five parts,
  `docs/introduction.md` is the docs-site landing
  (README stays GitHub-repo-focused). `.github/workflows/docs.yml`
  publishes to GitHub Pages on every push to `main`.
- **User-facing install docs**. README gained "Which binary?" +
  "Install" sections with one command per path. `installing.md`
  restructured to put package paths first; build-from-source
  positioned as the developer path. `RELEASE.md` at the repo
  root documents the steps CI can't do (AUR SSH push,
  draft-publish review, pre-tag readiness checks).

**Conventions established (worth keeping)**:

- Bare semver tags (`0.14.1`, no `v` prefix); all pipelines pass
  the tag straight through.
- No top-level `LICENSE` file in v1 - the LGPL-3.0 text ships
  inline in `packaging/debian/copyright` (the form distros
  actually consume) + README links to gnu.org's canonical URL.

**One-time manual setup** before the first push to `main`: in
GitHub repo Settings -> Pages, set "Source" to "GitHub Actions"
so `docs.yml` can deploy.

**Stayed on the "Path to 1.0.0 distribution" parallel track**
(post-M15.8 polish, not gated on this milestone): Homebrew tap,
Snap, Nix flake, real apt repository, cross-build for macOS /
Windows (waits on platform-portability work first).

---

**Phase C: I/O libraries (M16.x).** System libraries that touch the
OS or do significant compute. Phase C opens with a language
addition (M16.0 - concurrency primitives) because every I/O library
in the phase wants to know whether spawned work and async waits
are available; the I/O libraries themselves (M16.1+) ship in the
same phase atop that foundation.

## M16.0 - Lightweight concurrency

Adds `spawn { ... }` and a `task of T` value type so I/O can
proceed without blocking the whole program. Lands as a
prerequisite of M16.1-M16.3 (any of which can use it for
non-blocking calls) and a hard requirement for M20 `httpd` (a
single-connection web server isn't a server). The decision to
ship goroutine-style concurrency rather than `async`/`await` or
raw OS threads is recorded with the reasoning at
[technical/rejected.md > async/await coloring](technical/rejected.md#asyncawait-function-coloring)
once the milestone work begins; the short version is "Jennifer's
value semantics already provide the isolation that
async-await / borrow-checking exist to enforce."

**Why now.** Jennifer's value-semantics decision (M6 lists/maps,
M12 bytes, M13.1 structs - all copied on assignment and
parameter binding) means a spawned block cannot accidentally
share mutable state with the parent. Every captured variable is
deep-copied at spawn time, identical to a function call. The
hardest problem in shared-memory concurrency - data races on
mutable state - cannot happen by construction. We get that
property for free from a decision we made for unrelated reasons,
which is what makes "ship concurrency" a smaller surface than it
would be in a reference-semantics language.

**Syntax.**

```jennifer
# spawn { ... } runs the body in a separate goroutine. Variables
# captured from the enclosing scope are deep-copied at spawn time,
# same semantics as a method call. The block's `return EXPR;`
# becomes the task's result; bare `return;` produces null.
def t as task of int init spawn {
    return computeStuff();
};

# task.wait blocks until the spawned block finishes and returns the
# value. If the block threw an error, task.wait re-throws it in the
# waiter's frame.
def result as int init task.wait($t);
```

**New type kind: `task of T`.** A compound type that wraps a
pending or completed computation. Constructed only by `spawn`;
read only via the `task` library. Value semantics: a
`task of T` value is small (essentially a handle), and copying
it shares the same underlying task (single result, multiple
waiters get the same value or the same re-thrown error - "the
task" exists once even if the handle moves). This is the **one
exception** to Jennifer's "no shared references" rule, and it's
necessary - a task by definition represents a single underlying
operation. The exception is contained: `task of T` values can
only be acted on through the `task.*` API, which itself is
side-effect-careful.

**New keyword: `spawn`.** Statement-position only - same shape
as `if`/`while`/`for`/`repeat`. The spawn block is a fresh
scope; the spawn body runs concurrently with the rest of the
program.

**Value-semantics capture.** Variables referenced inside
`spawn { ... }` from the enclosing scope are deep-copied into
the spawned frame at the moment of spawn, same as how a method
call deep-copies its arguments. The spawned block sees its own
copy; mutations don't propagate back; the parent's bindings are
untouched:

```jennifer
def xs as list of int init [1, 2, 3];
def t as task of null init spawn {
    $xs[0] = 999;       # mutates the spawned frame's copy
    return null;
};
task.wait($t);
# $xs in the parent is still [1, 2, 3].
```

**Error propagation.** Errors thrown inside a `spawn` block are
captured by the task. Calling `task.wait` re-throws them in the
waiter's frame, where ordinary `try`/`catch` (M13.2) handles
them:

```jennifer
def t as task of int init spawn {
    throw Error{kind: "boom", message: "...", file: "", line: 0, col: 0};
};
try {
    def n as int init task.wait($t);
} catch (err) {
    io.printf("caught: %s\n", $err.message);
}
```

A task whose error was never waited on **silently drops** the
error when GC'd. (Jennifer has no finalizers; surfacing dropped
errors would require one.) Users who care about every error
should always `wait` or `discard`. Recommended idiom for
fire-and-forget: explicit `task.discard($t);` so the intent is
visible at the call site.

**`task` library (built-in, shipped with M16.0).**

| Function                        | Effect                                                                                                                 |
| ------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `task.wait(t) -> T`             | Block until `t` completes; return its value or re-throw its error.                                                     |
| `task.poll(t) -> bool`          | Non-blocking check: true if `t` has completed (value or error available).                                              |
| `task.discard(t)`               | Mark `t` as fire-and-forget; suppresses dropped-error logging if it's ever added.                                      |
| `task.waitAll(ts) -> list of T` | Wait for every task in `$ts`; return their results in order. Re-throws the first error encountered (others discarded). |
| `task.waitAny(ts) -> int`       | Wait until any task in `$ts` completes; return its index. Caller then `task.wait`s that one.                           |

`waitAll` / `waitAny` cover the most common patterns (parallel
map-fold and "first to respond wins"). Anything more complex
(timeouts, racing N tasks with cancellation of losers, etc.)
ships in a `task.x` sub-milestone if a real use case forces it.

**Multi-core / parallelism.** Spawn blocks compile to goroutines
and Go's scheduler runs them across every available core by
default - so on hosted builds (Linux / macOS / Windows desktop)
M16.0 *is* multi-core parallelism, not just concurrency.
Data-race-freedom holds by construction because value-semantics
capture deep-copies every variable at spawn time; the data races
that drive most parallel-programming bugs cannot happen.

Two target classes need an explicit fallback:

- **Single-threaded TinyGo targets** (WASI, baremetal embedded)
  and **macflyos** have no OS threads to schedule across. The
  interpreter ships a **`--cooperative` mode** for these builds
  in which `spawn` blocks run on a single OS thread with a
  cooperative scheduler; the same source program runs without
  modification, just sequentially. Semantics remain identical -
  the only observable difference is wall-clock behavior.
- **Per-goroutine interpreter state stays single-threaded.**
  Each `spawn` walks the AST sequentially in its own goroutine;
  making the tree-walker itself internally parallel
  (lock-free environments, concurrent scope chains) is a
  different and much harder project with near-zero payoff for
  the workloads Jennifer targets and is deliberately out of
  scope.

Finer-grained scheduling control (CPU affinity, work-stealing
pools, NUMA awareness, `GOMAXPROCS`-equivalent tuning) is
recorded in the Long horizon list - those are advanced-runtime
knobs, not language features, and the same "ships when a real
use case forces it" rule applies.

**What's NOT in v1 (deliberate; defer until a forcing function appears).**

- **Channels** (`chan of T`, `chan.send`, `chan.recv`). Tasks
  cover the futures use case; channels cover pipelines /
  fan-in / fan-out. The pipeline patterns can be expressed with
  lists of tasks and `waitAll` until that proves clumsy. Add
  channels in a follow-on sub-milestone if/when real programs
  need them.
- **Mutexes / locks.** Value-semantics capture removes the
  shared-mutable-state-needs-locking case. If users want a
  shared counter, they spawn a goroutine that owns it and
  communicate via tasks (actor-style).
- **Cancellation.** Famously hard to do right (Go added
  `context.Context` years after goroutines). Spawned blocks
  cannot be killed; they run to completion. Tasks that take a
  long time and may need stopping should check a flag the
  parent sets via shared state - but since Jennifer has no
  shared state, the pattern is "use a quick task plus a
  user-managed sentinel value." Concrete API ships when a real
  use case surfaces.
- **Timeouts.** Same family as cancellation. Workaround:
  `task.waitAny([$work, $timer])` where `$timer` is a task
  that spawns and sleeps. Wait for whichever returns first.
- **Go-style `select`.** Multi-channel select belongs with
  channels.
- **`context.Context` propagation.** Belongs with cancellation.

**Interaction with existing language features.**

- **`exit` inside a spawn**: terminates the whole program (same
  as outside spawn). The spawn keyword does not create a frame
  that catches `exit`; it's still the global escape hatch.
- **`return` inside a spawn**: returns from the spawn block,
  producing the task's value. Does not return from the
  enclosing method. (Symmetric with how `return` inside a method
  body returns from the method, not from the enclosing
  top-level.)
- **`break` / `continue` inside a spawn**: error at parse time
  - they make no sense without an enclosing loop, and the
    spawn's lexical block does not transparently see loops in
    the parent.
- **`throw` inside a spawn**: captured by the task and re-thrown
  on `wait`, as documented above.
- **try/catch crossing a spawn boundary**: a `try` in the
  parent does **not** catch errors raised inside an
  unwaited-on `spawn`. The `task.wait($t)` site is where errors
  enter the catchable channel.

**REPL interaction.** A `spawn` from a REPL input runs
concurrently; subsequent inputs can `task.wait` on it. The line
editor owns stdin (same M5 rule as `io.readLine`), so spawned
blocks must not read from stdin while the REPL is interactive.

**Internals.**

- `spawn { body }` lowers to a Go goroutine that runs the body
  with a deep-copied frame. The goroutine sends `{value, error}`
  to a one-shot Go channel; the `task of T` value wraps that
  channel.
- `task.wait` blocks on the channel; on receive, returns the
  value or wraps the error and routes it through the same
  `ErrorSignal` machinery M13.2 introduces. The waiter's frame
  position is what `catch` blocks see.
- TinyGo's goroutine implementation depends on the target;
  Linux native (the shipping target today) uses preemptive
  scheduling on top of OS threads, so `spawn` can give real
  parallelism on multi-core systems. WASM targets use
  cooperative scheduling. The McFly OS embedding inherits
  whatever scheduling its TinyGo build configures.
- Tests: a synthetic interpreter helper installs a "spawn that
  runs the body inline" mode so deterministic tests don't
  depend on goroutine scheduling.

**Library impact.**

- **M16.1 `fs`** can ship blocking read/write that callers wrap
  in `spawn` for non-blocking. No `fs.readAsync(...)`
  duplication API needed - the user composes with `spawn`.
- **M16.2 `net`** likewise. Accept-loop / per-connection servers
  spawn a task per connection.
- **M19 `crypto`** doesn't need concurrency directly but the
  M19 hash/crypto helpers can be composed under `spawn` for
  parallel hashing of large inputs.
- **M20 `httpd`** is the headline consumer. The accept loop
  becomes `while (true) { def conn init net.accept($listener); spawn { handle($conn); } }`.

**Benchmark suite extension.** `examples/benchmark.j` is
currently single-core only - `time` of the user/real columns
under `tinygo` build sits within a few percent of each other
(see [technical/tinygo.md > Single-binary benchmark
results](technical/tinygo.md#single-binary-benchmark-results)).
M16.0 ships with the suite extended so each existing workload
has a multi-core counterpart that fans the same work across N
`spawn`'d tasks and joins. The driver runs both single-core and
multi-core variants side by side and shows the speedup ratio,
so reviewers can see at a glance how much parallelism actually
materialises for each workload shape (CPU-bound, allocation-
heavy, IO-bound). This is the headline programmer-facing
artifact for the milestone.

See:
- [user-guide/concurrency.md](user-guide/concurrency.md) (new
  doc at M16.0 implementation time) - the user-facing tour with
  worked examples (parallel fetch, request handler, fire-and-forget
  logging).
- [libraries/task.md](libraries/task.md) (new) - the `task.*`
  function reference.
- [technical/interpreter.md > Concurrency](technical/interpreter.md#concurrency) (new
  subsection) - goroutine mapping, frame copying, error
  routing, test-only inline mode.

## M16.1 - `fs`

File I/O. `fs.read`, `fs.write` for `string` and `bytes`,
`fs.stat` returning a struct, directory walk. Requires M11
(bytes) and M13.1 (structs). Brings the M7-deferred file handles
and any non-stdin input source (`fs.open(path) -> handle`,
`handle.readLine()`, etc.) under one library.

## M16.2 - `net`

Sockets. Plain TCP and UDP first; TLS deferred to its own
sub-milestone. The first place a real `bytes` round-trip
matters. Ships blocking calls; non-blocking use composes through
M16.0 `spawn` rather than duplicating each call as
`net.acceptAsync`, `net.readAsync`, etc.

## M16.3 - `regex`

Regular expressions over `string`. Large standalone milestone;
pure string processing, no other dependencies.

## M16.4 - `testing` (system-library primitives)

Irreducible system-side surface a Jennifer-coded test framework
needs: a run-by-name primitive, a per-process result
accumulator, and a format dispatcher for human-readable / TAP /
JUnit-XML output. The `.j` half (assertion vocabulary, suite
organisation, CLI filtering) ships in M18.x as a separate
sub-milestone built on top of these primitives - this layer is
the minimum that has to live in the interpreter.

**Why a system library at all.** A pure `.j` test runner would
have to take "the test body" as a value and call it. Jennifer
has no function references / first-class methods today, so a
`.j` runner can't say `testing.run("myTest", myTest)`. The
interpreter already does method-name lookup at call sites for
`prefix.fn()`; this milestone exposes that lookup as one
builtin so a Jennifer-coded module can dispatch user methods by
name without each runner reinventing the indirection.

**Surface.**

- `def struct testing.Result { name as string, ms as int,
  passed as bool, errorKind as string, errorMessage as string,
  file as string, line as int, col as int };` - one entry
  per test run. An empty `errorKind` denotes a pass; on
  failure the fields mirror the auto-hoisted `Error` struct
  (M13.2) so the failure positions are usable.
- `testing.run(name as string) -> testing.Result` - looks up
  the named top-level method, calls it inside an implicit
  `try { ... } catch (e) { ... }`. Times the run via the
  `time` library (M15.5). If the call throws an `Error` the
  result carries its fields. An `exit` inside the test is
  also captured here - this is the one place in the language
  where `exit` is interceptable, the test-runner exception
  (cleanly scoped: the runner sees it as a special failure,
  but it does not propagate to the surrounding program).
  Appended to the per-process accumulator.
- `testing.results() -> list of testing.Result` - read the
  accumulator (returns a copy).
- `testing.reset() -> null` - clear the accumulator between
  independent runs.
- `testing.report(results, format as string) -> string` -
  codec-table dispatch matching the
  [`encoding.toText`](#m157---encoding) shape:
  - `"text"` - human terminal output: pass / fail per test,
    failure context, totals, timings.
  - `"tap"` - Test Anything Protocol v14, machine-readable,
    works with `prove` and most CI harnesses.
  - `"junit"` - JUnit XML, the ubiquitous CI input format.

  Format strings stay case-sensitive (small set), matching
  the M15.7 precedent.

**Subprocess isolation deferred.** A runaway `exit` or
infinite loop in one test would in principle kill the in-process
runner; in practice the `exit` capture above plus a future
`--isolated` flag (re-invoking `jennifer run testfile.j
--testing-single name` per test via M15.3 `os.spawn`)
gives complete isolation when the user opts in. The building
blocks ship already; the runner CLI wiring lives with the
`.j` module's CLI surface in M18.x.

**Dependencies:** M13.2 (`try`/`catch`/`throw` + Error struct),
M15.5 (`time` for run duration), M13.1 structs, M15.2
library-provided namespaced struct types.

**See also:** the M18.5 `testing` module (Jennifer-coded
assertions and suite driver built on these primitives).

## M16.5 - Interpreter performance pass

Closes the largest gaps the benchmark suite exposes
(see [technical/tinygo.md > Single-binary benchmark
results](technical/tinygo.md#single-binary-benchmark-results))
before Phase D's Jennifer-coded libraries (json, csv, http,
testing.j, ...) land on top. Those libraries are mostly tight
parsing loops and recursive descent; landing M16.5 first means
they ship at native-feeling speed instead of needing apology
paragraphs in the per-library docs.

**Where the cycles go today.** The numbers from
`examples/benchmark.j` point at three distinct bottlenecks, each
fixable independently:

1. **Value-semantic copy-on-write on every compound write.**
   `$xs[] = item`, `$xs[i] = v`, `$m[k] = v`, `$p.field = v` all
   deep-copy the root binding before mutating. For a sequence of
   N appends that's O(N^2) - the standout cost in
   `struct list build+read` (10.7 s), `map insert+read` (9.8 s),
   and `string join` (4.2 s) on TinyGo.
2. **Name-based variable resolution at every reference.** Each
   `$n` reference walks the Environment chain by name; in a tight
   loop touching `$n`, `$d`, `$count` the resolver re-traverses
   the same chain on every iteration. Big tax on `primes up to
   LIMIT` (44.9 s) and the rest of the compute-bound block.
3. **Method-call frame churn.** Every call allocates a fresh
   `Environment`, binds parameters one by one through map
   operations, and pays the method-table lookup at the call
   site. `fib(23)` shows up at 5.2x the standard-Go cost - the
   recursion depth is small (~28 K calls), the body is trivial,
   the gap is dispatch overhead.

The user-visible behaviour stays unchanged. Value semantics still
hold, just implemented without the literal whole-tree copy on
every write. Lexical scoping still holds, just resolved at parse
time. Methods still bind by name at the source level; the
lookup is just done once.

### M16.5.1 - Refcount-based copy-on-mutation for `Value`

The biggest single win. Add a refcount to compound `Value`
payloads (or to a shared wrapper around the underlying slice /
map / struct-fields backing store). On assignment / parameter
binding, bump the refcount instead of deep-copying. On mutation
(`$xs[] = `, `$xs[i] = `, `$m[k] = `, `$p.field = `), check
the refcount: refcount == 1 means no aliases exist, mutate in
place; refcount > 1 means split (deep-copy this binding) before
mutating.

The refactor touches:

- `internal/interpreter/value.go` - refcount field on the
  compound payloads (KindList, KindMap, KindStruct, KindBytes).
  Currently `Value.Copy()` is the single deep-copy entry point;
  it becomes "bump refcount" with the deep-copy reserved for
  the mutation-site split.
- `internal/interpreter/interpreter.go` - `execAppend`,
  `execIndexAssign`, `execFieldAssign` all rewritten to use
  the split protocol. Parameter binding (`evalCall`), assignment
  (`Assign`), and for-each iteration variables also need their
  Copy()-sites reviewed.
- Test strategy: a dedicated `value_alias_test.go` that
  constructs aliased compound `Value` graphs and asserts each
  mutation site preserves the no-aliasing invariant
  (mutating through one binding doesn't visibly affect another).
  Run as part of `go test ./...`. Stress-cover the corner cases
  (lists inside maps, structs containing lists, nested struct
  fields, bytes inside structs) before considering the milestone
  done.

Expected impact (from the M16.5 planning analysis):
- `struct list build+read`: ~10752 ms → ~1500-2000 ms (5-7x)
- `map insert+read`: ~9768 ms → ~1500-2500 ms (4-6x)
- `string join`: ~4189 ms → ~600-900 ms (5-7x)
- `list sort/reverse/slice`: smaller direct effect (sort /
  reverse / slice return fresh lists anyway).

Highest-risk sub-milestone too: aliasing bugs are the
classic mode-of-failure for refcounted COW. The alias-stress
test harness is non-negotiable - lands as part of M16.5.1.

### M16.5.2 - Lexical slot resolution at parse time

Resolve every variable / constant reference to a
`(frame-depth, slot-index)` pair at parse time rather than a
name-based lookup at run time. Inside any block, slot indices
are dense and stable; the runtime lookup becomes an array
index instead of a map walk.

The refactor touches:

- `internal/parser/parser.go` - add a scope analyser pass that
  numbers definitions and annotates references with their slot.
  New AST fields on `VarExpr` / `ConstRefExpr` etc. (or new
  resolved-variant AST nodes).
- `internal/interpreter/interpreter.go` - `Environment`
  storage becomes a per-frame slice indexed by slot rather
  than a map keyed by name; `evalExpr`'s var-ref case becomes
  `frames[depth].slots[idx]`.
- A small grammar / scope-analysis test suite covering
  shadowing rejection, scope-chain walk for for-each iteration
  variables, and method-body-sees-globals semantics. The
  user-visible behaviour stays the same; the scope analyser
  just catches "undefined variable" at parse time instead of
  runtime.

Side benefit beyond performance: undefined-variable errors
surface at parse time rather than first-execution, which is
strictly better for the developer experience.

Expected impact:
- `primes up to LIMIT`: ~44876 ms → ~15-20 s (2-3x)
- `newton`, `monte carlo`: similar 2x range
- `fib`: contributes to the M16.5.3 win below

### M16.5.3 - Method call frame optimization

Three independent moves that compound:

- **Pool environments.** Currently every call allocates a fresh
  `Environment`; under recursive workloads this churns the
  allocator hard. A free-list of recycled environments,
  cleared on push and pushed back on pop, eliminates the
  allocation.
- **Pre-resolve callees at parse time.** Method calls today
  look up the callee name in `i.methods` on every call. With
  M16.5.2 in place we can resolve method references at parse
  time too (top-level methods form a fixed set after hoisting),
  so the runtime call goes straight to the resolved function
  pointer.
- **Slot-based parameter binding.** Under M16.5.2 parameters
  are slots; binding becomes an array write per parameter,
  not a map operation.

All three depend on M16.5.2 (slot resolution), so they ship
together at the end of the performance pass.

Expected impact:
- `fib(23)`: drops to ~80-150 ms (3-5x), in striking distance
  of `jennifer-go`'s 84 ms.

### Combined target

Re-running `examples/benchmark.j` against the M16.5-final
TinyGo binary should yield a total time in the 25-30 s range
(down from ~75 s) and put TinyGo within ~1.5x of `jennifer-go`
on every workload shape - that's the floor set by the
TinyGo-vs-stdlib runtime gap and the right target to aim for.
Update `technical/tinygo.md`'s tables with the post-M16.5
numbers as the closing deliverable.

**Stance check.** None of these changes alter user-visible
semantics:
- Value semantics still hold (the user can't observe aliasing;
  mutations through one binding never affect another).
- Lexical scoping still holds (`def` still introduces a fresh
  binding; redeclaration still rejects).
- Method calls still resolve by name at the source level; the
  pre-resolution just caches the resolved target.

So the optimisation pass is invisible to existing Jennifer
programs and to the docs - it changes performance, not
behaviour. Existing tests and examples should pass unchanged.

---

**Phase D: higher-level and Jennifer-coded libraries (M17-M20).**

## M17 - Module system for Jennifer-coded libraries

Real modules so Jennifer-coded libraries get namespaces, scope,
and explicit exports. Adopts "**module**" as the canonical
wording for a distributable `.j` library (matches Python, ES2015,
Rust); "bundle" deliberately not used because it overloads the
JS build-output term. The `include "x.j";` keyword (renamed in
M10) stays as the textual-splice form for inline composition
inside one source tree; library distribution moves to modules
via `import`.

Split into four sub-milestones so each piece ships standalone
and lands in dependency order: source-tree layout first, then
the import syntax, then resolution, then exports. M18.x cannot
start until all four ship.

## M17.1 - Source tree separation

A new top-level `modules/` directory holds Jennifer-coded
modules; system (Go-built) libraries stay in `internal/lib/*/`.
The split is load-bearing for distro packaging: system
libraries are baked into the interpreter binary, so a Debian
package only ships `/usr/bin/jennifer`; modules are data files
that ship to a system module dir
(`/usr/share/jennifer/modules/` or `/var/lib/jennifer/modules/`,
decided at packaging time) and are loadable without
recompilation. The directory exists from M17.1; the modules
shipped in M18.x land directly in it.

## M17.2 - `import "modules/foo.j" as foo;` syntax

The real module boundary. The imported file's top-level names
live behind the `foo.` prefix at the call site, the same shape
as a namespaced system library. Aliasing rules match
`use NAME as ALIAS;` (alias replaces the canonical name; bare
form reserves the file-stem as the prefix). Reuses the
namespacing machinery shipped in M10 so call-site resolution is
already settled.

## M17.3 - Module resolution

A search path with explicit precedence: the current source
file's directory first, then any `-I` directories passed on
the command line, then the system module dir. No implicit
fallback to system libraries - a `use NAME;` always means a
system library, `import "..." as ...;` always means a module
file. Matches the explicit-prefix rule recorded in
[rejected.md > Implicit `use NAME;` fallback chain](technical/rejected.md#implicit-use-name-fallback-chain-m8).

## M17.4 - Module-level exports

Top-level `def` and `func` are exported by default; a leading
`private` marker (or similar; settled at start of M17.4) hides
them. Same visibility rule top-to-bottom; no per-file export
list. Ships last because it's the only piece that touches
parser grammar - M17.1 through M17.3 are tooling and
resolution.

## M17.5 - Developer tooling: profiling

A `jennifer profile <prog.j>` subcommand that instruments the
evaluator to attribute work back to Jennifer source positions -
the gap left by `go tool pprof`, which profiles the interpreter
binary, not the .j program running inside it. Lands between M17
(modules) and M18.x (Jennifer-coded libraries) so library
authors can profile their code from the moment they start
writing it; deliberately not under M16.x because it isn't an
I/O library and doesn't fit the phase grouping.

- **Statement / call profile.** The default mode. Per source
  position (file:line:col) record hit count and cumulative
  wall-clock time spent in that statement / expression.
  Output formats: a flat human-readable table by default,
  pprof-compatible binary via `--format=pprof`, Chrome-trace
  JSON via `--format=trace`. Existing flamegraph tooling
  (`go tool pprof`, https://www.speedscope.app/) consumes
  both forms so we don't ship a renderer.
- **Allocation tracking.** Optional second mode
  (`--allocs`) that counts `Value.Copy()` and Value-build
  sites per source position. Jennifer's value semantics
  make every copy load-bearing; surfacing "this struct
  passes through 40 frames per call" is exactly the
  kind of insight that compiled-language allocation
  profilers give. Same output formats as the statement
  profile.
- **TinyGo-friendliness.** Profiling code lives behind the
  `profile` subcommand and a build tag so it doesn't bloat
  the shipping `jennifer` binary. The macflyos embedding
  target gets the profiler-free build; desktop / dev
  builds get both.
- **Out of scope for v1.** No source-step debugger (its
  own milestone if/when it lands).
- **May absorb future developer-tooling work.** Source-level
  debugger hooks, runtime introspection, and any other
  `jennifer <tool>` subcommand that instruments the
  evaluator can ship as sub-milestones under M17.5.x rather
  than getting their own top-level slots.

## M18.x - Jennifer-coded modules

Built atop the existing system libraries. Each one ships as a
Jennifer **module** under `modules/` (the directory introduced in
M17); none of them are compiled into the interpreter binary.
Sub-milestones in priority order:

- **M18.1 - `timezones`** - IANA-name + DST companion to the
  fixed-offset core in M15.5.2. A pure-Jennifer map of zone
  names to `time.Zone` values (with seasonal ranges where
  applicable) and a small resolver helper
  (`timezones.zoneFor(name, $t) -> time.Zone`) that picks the
  right offset for a given instant. A build-time script
  regenerates the map from the host's tzdata before shipping,
  so the data is auditable as source. Keeps zone policy out
  of the interpreter binary.
- **M18.2 - `http`** (client) - atop `net`.
- **M18.3 - `json`** - data interchange ubiquity.
- **M18.4 - `csv`** - simple, useful early.
- **M18.5 - `testing`** - the Jennifer-coded half of the test
  framework, on top of the M16.4 primitives. Assertion
  vocabulary built on `throw` so each mismatch produces an
  `Error` the M16.4 runner catches uniformly:
  `assertEqual`, `assertNotEqual`, `assertTrue` / `assertFalse`,
  `assertContains` (substring, list element, map key), and
  `assertThrows(name, kind)` that runs another method by name
  and asserts it throws an Error of the given kind. Suite
  driver: pass a list of test-method names, the module calls
  `testing.run` for each, looks up `setUp` / `tearDown` by
  convention if present, and finishes by calling
  `testing.report` against the chosen format. CLI surface:
  `--filter pattern` to run a subset, `--format text|tap|junit`
  for the report shape, `--isolated` to opt into the
  per-test-subprocess mode (M15.3 `os.spawn` + the M16.4
  `--testing-single` flag). The cycle is closed: programs
  written against `testing.j` can be tested in turn by the
  same framework.
- **M18.6 - `yaml`, `xml`, `markdown`, `pretty`** - one or more
  sub-milestones depending on scope when planned.

## M19 - `crypto`

Symmetric and asymmetric primitives, key derivation,
crypto-grade random. System library; TinyGo-safe primitives
only. Hashes already shipped in M15.6.

## M20 - `httpd`

Pure-Jennifer HTTP server atop `net`. Ships as a module under
`modules/httpd.j` (same packaging shape as the M18 modules), not
baked into the interpreter. The point where Jennifer becomes
useful for serving content. Depends on **M16.0 concurrency**
(per-connection handlers run in `spawn` blocks) and M16.2 `net`
(the underlying TCP listener).

---

**Phase E: WASM and specialised domains (M21+).** Not committed to a
timeline; recorded so the design doesn't foreclose them.

## M21 - WASM runtime embedding

Wazero or similar inside the interpreter binary. TinyGo-size
cost evaluated honestly before commitment. Without M21, no WASM
libraries.

## M22.x - WASM libraries

If M21 ships, sandboxed plugins via `use wasm:libname;`. Each
library a sub-milestone.

## M23+ - Specialised domains

Each domain its own milestone with sub-milestones as needed:

- **ML.** Vector, matrix, stats, ML primitives.
- **Bioinformatics.** Sequence alignment (Smith-Waterman,
  Needleman-Wunsch), FASTA/FASTQ parsers, molecule structures.
- **Sandbox.** Restricted-capability execution.

Ordered when demand surfaces. WASM libraries (M22.x) may cover
some of this space first.

## M24+ - encoding long-tail codecs

The remaining single-byte codecs the original M15.7 spec
listed, parked here so they're picked up only when a real
Jennifer program asks for them. The codec-table infrastructure
shipped with M15.7 - each new entry is just a 256-entry table
plus its alias list.

- **ISO-8859-{2..16}** (15 codecs): Central / Eastern European,
  Cyrillic, Arabic, Greek, Hebrew, Turkish, Nordic, Celtic,
  South-Eastern European. Canonical alias form `"iso-8859-N"`.
- **Windows-{1250, 1251, 1253..1258}** (8 codecs): Microsoft's
  Central, Cyrillic, Greek, Turkish, Hebrew, Arabic, Baltic,
  Vietnamese code pages. Canonical alias form `"windows-N"`
  with `"cpN"` accepted.

**What stays out (deferred further).**

- Variable-width Asian encodings: `Shift-JIS`, `Big5`, `GB2312`,
  `GBK`, `GB18030`, `EUC-JP`, `EUC-KR`. Each is a state-machine
  implementation with multiple variants and ambiguity edge
  cases; one or two of these is a whole milestone of work, not
  a row in a table.
- `UTF-16` / `UTF-16LE` / `UTF-16BE` and `UTF-32` - byte-order
  marks, surrogate pair handling, endianness.
- `UTF-7`, `quoted-printable`, mail-transport hacks - belong
  with `mail` / `mime` library work, not core encoding.

---

## Path to 1.0.0 distribution (parallel track)

The core CI + release + packaging items that used to live here
were promoted into M15.8 (the last step before Phase C). What
stays on this parallel track is the post-M15.8 polish - items
that can land any time and don't block any milestone:

- **Homebrew tap** for macOS users.
- **Snap** package.
- **Nix flake** / Nix package.
- **Cross-build for macOS / Windows.** Waits on the
  platform-portability work in the Long-horizon list; ships as
  soon as that lands.
- **Real apt repository** (replacing the "GitHub Release
  artifact" install of the M15.8 `.deb`) if user demand
  warrants the maintenance.
- **Snap / Flatpak**, **AppImage**, or any other Linux
  distribution format Jennifer users actually ask for.

Each of these ships when there's user demand and a maintainer
willing to keep it green; they're not blocking anything.

---

## Long horizon (recorded, not scheduled)

- **Cross-platform support.** Today Jennifer targets Linux only.
  Windows and macOS are planned. When touching filesystem, paths,
  line endings, or process behavior, prefer portable stdlib
  helpers (`path/filepath`, not hardcoded `/`); avoid Linux-only
  assumptions.
- **McFly OS kernel integration.** Embed the Jennifer interpreter
  into **McFly OS**, an experimental OS also written in TinyGo.
  Reinforces the TinyGo-friendliness discipline: no
  `reflect`-heavy code, no goroutines in the core, no heavy
  stdlib dependencies, and no hard dependencies on a hosted
  runtime (ambient stdin, network, dynamic linking).
- **FCGI.** `use FCGI as web;` library when `net` and `httpd`
  mature. Lets Jennifer host CGI / FastCGI workloads end-to-end.
- **Inline assembler.**
- **Binary AST cache (`.jc` files).** Pre-parsed loading for big
  programs and McFly OS embedding. Its own milestone when it
  lands - file-format design, versioning, and TinyGo-safe
  serialization are enough work to merit dedicated treatment. The
  text JSON form via `jennifer ast` is the placeholder until then.
  Deferred from M5.
- **`io.lines() -> list of string`.** Slurp the whole stdin into a
  list. Additive on top of the streaming `readLine()` + `eof()`
  idiom; nice-to-have for tiny scripts, not blocking. Deferred from
  M7.
- **i18n.** Locale-aware case folding, collation, number / date
  formatting, BiDi. Gated on the CLDR-data binary-size question
  (likely an optional library shipped after the M19 WASM runtime
  so locale tables aren't baked into every build).
- **Host-embedding API.** A stable, documented Go-side surface
  for driving the interpreter from a host program (passing
  values in / out, surfacing positioned errors, hooking stdout
  / stdin). Distinct from FFI as conventionally meant - see
  [technical/rejected.md](technical/rejected.md#ffi-as-a-single-milestone)
  for why that framing was turned down. Likely Phase D/E timing
  (around M18-M19), since macflyos embedding and any
  serious host-driven use case need it.
- **Advanced scheduling knobs.** CPU affinity, work-stealing
  pool sizing, NUMA awareness, `GOMAXPROCS`-equivalent runtime
  tuning. Runtime-config surface for the M16.0 spawn scheduler,
  not new language features. Ships when a real use case forces
  it (the M16.0 default - "let Go's scheduler decide" -
  handles every workload we've imagined so far).
- **Performance & memory.** Interpreter-internal optimizations
  that preserve stance #5 (value semantics) at the user level:
  copy-on-write for lists / maps / bytes / structs (share
  underlying storage until a write splits it), per-frame arena
  allocation, and read-only slice views (`xs[1..5]` as a
  non-owning window that errors on assignment). Strictly
  optimizations - no user-visible aliasing or mutation rules
  change. Stance-breaking variants (mutable references,
  interior mutability, shared mutable state) are turned down in
  [technical/rejected.md](technical/rejected.md#references-interior-mutability-shared-mutable-state).
  Best landed post-M21 once the language is settled and the
  interpreter doesn't churn under it.
