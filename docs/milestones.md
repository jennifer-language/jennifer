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
doesn't implement `os/exec`, so the constrained `jennifer-tiny`
binary returns a friendly "use the default `jennifer` binary" error
instead of panicking - first place the two-binary story becomes
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

**Status:** done.

Ships `spawn { ... }` (block primary expression), `task of T`
(new compound type kind), and the `task` library (`wait`, `poll`,
`discard`, `waitAll`, `waitAny`). Goroutine-backed concurrency on
top of Jennifer's value-semantics-capture, which makes data
races impossible by construction.

- **`spawn { ... }` runs concurrently; `task of T` wraps the
  result.** Body's `return EXPR;` becomes the task's value; bare
  `return;` yields null. A task carries either a value or an
  error after its body finishes.
- **Value-semantics capture.** `snapshotForSpawn` builds a
  two-frame snapshot at launch: a globals frame (deep copy of
  `i.global`) and a locals frame (deep copy of every non-global
  binding visible at the spawn site) chained on top. The body
  runs against the locals frame; user-method calls inside the
  spawn parent through `effectiveGlobal(env)` and land on the
  globals frame, so the no-shadowing rule treats spawn-local
  scoping the same as serial scoping. Tasks are the **one
  exception** to value semantics: copying a `task of T` shares
  the underlying `TaskState` pointer (multiple handles must
  observe one in-flight goroutine, not clone it).
- **Loud-fail at exit.** Per-run task registry on the
  interpreter; `spawn` appends, `wait` (success or rethrow) /
  `discard` / `waitAll` flip the observed bit. CLI walks the
  slice after `Run` returns and prints each unobserved error
  to stderr, bumping the exit code. The scan blocks on each
  unobserved task's `Done` channel - a non-terminating
  `spawn { while (true) { ... } }` without `task.discard` hangs
  at exit (documented footgun, trade-off for "no silent
  drops").
- **Errors compose with M13.2 try/catch.** `task.wait` re-raises
  a body error as a positioned runtime error at the wait site;
  enclosing `try/catch` catches it normally. `exit EXPR;` inside
  a spawn still exits the whole program (uncatchable, same as
  outside spawn). `break`/`continue` outside an enclosing loop
  inside the body surface as positioned runtime errors via
  `unhandledLoopFlowError` - loop flow doesn't cross the spawn
  boundary.
- **`waitAny` uses `reflect.Select`** over a
  `[]reflect.SelectCase` built from the task list - the only
  reflect call in the runtime; verified under both compilers.
- **TinyGo trade-offs.**
  - `tinygo build -stack-size=1mb` is now in the Makefile.
    Jennifer's tree-walking evaluator wraps each Jennifer-level
    call in many Go-stack frames; the TinyGo default (~8KB) is
    far too small for any recursive spawn body.
  - TinyGo's runtime is cooperative single-threaded as of 0.41,
    so parallel speedups on `jennifer-tiny` will be close to 1.0;
    the default `jennifer` (standard Go) reaches real multi-core
    speedup on the parallel benchmark.
- **What was deferred** (none of these blocked M16.0 ship;
  picked up later if a forcing function appears): channels,
  mutexes, cancellation, timeouts, `select`,
  `context.Context`, structured-concurrency blocks. Each is
  documented inline in user-guide/concurrency.md so the
  decisions stay visible.
- **`examples/benchmark.j` parallel section.** Multi-core
  variants for primes, newton, monte-carlo, and a
  PARALLEL_WORKERS-fanout of `fib(N)`. Driver prints
  `serial_ms / par_ms / speedup` per workload, plus the
  scheduler name in the header so the numbers can be read in
  context (TinyGo's cooperative scheduler gives sub-1.0
  speedups; Go gives >1.0).

See:
- [user-guide/concurrency.md](user-guide/concurrency.md) -
  user-facing tour: model, value-semantics capture, patterns,
  loud-fail contract, what's deferred.
- [libraries/task.md](libraries/task.md) - reference for the
  five `task.*` builtins, error propagation, worked examples.
- [technical/interpreter.md > Concurrency](technical/interpreter.md#concurrency-m160) -
  goroutine mapping, two-frame snapshot, `effectiveGlobal`,
  task registry, CLI integration.
- [technical/tinygo.md > TinyGo restrictions](technical/tinygo.md#tinygo-restrictions) -
  the `-stack-size=1mb` Makefile flag and the TinyGo scheduler
  note.

## M16.1 - `fs`

**Status:** done.

Ships blocking filesystem I/O built on M16.0's spawn-and-compose
model: whole-file reads and writes, metadata, directory
operations, buffered file handles for line-oriented reads. No
`*Async` duplication - non-blocking use composes with `spawn`.

- **One-shot ops.** `fs.readString(path)` / `readBytes(path)` for
  whole-file reads; `writeString` / `writeBytes` for
  overwrite-or-create; `appendString` / `appendBytes` for
  append. Invalid UTF-8 in `readString` is a boundary error.
- **Metadata.** `exists` / `isFile` / `isDir` return `bool`
  without erroring on missing paths (permission errors still
  surface). `fs.stat(path) -> fs.Stat` errors on missing.
- **`fs.Stat`.** `path`, `size`, `isDir`, `mtimeNanos`, `mode`.
  `mtimeNanos` is `int` (Unix nanoseconds), not `time.Time`, so
  `fs` stays decoupled from `time` at the Go-package level;
  users compose `time.fromUnixNanos($stat.mtimeNanos)`
  explicitly. `size` is `-1` for directories.
- **Directory ops.** `mkdir` / `mkdirAll` and `remove` /
  `removeAll` ship as **two verbs each** (no-footguns stance:
  the recursive form is grep-visible at the call site).
  `rename`, `list` (sorted, non-recursive), `walk`
  (depth-first, sorted, includes root, skips symlinks).
- **Handles.** `fs.open(path, mode) -> fs.File{id as int}` with
  `mode` in `"read"` / `"write"` / `"append"` (codec-table
  shape). `readLine` / `readChars` / `readBytes($f, n)` /
  `writeString` / `writeBytes` / `eof` / `close`. Handle
  registry pattern from M15.6 (`map[int64]*handleState`
  guarded by a mutex). `fs.eof` peeks one byte ahead through
  the buffered reader so the canonical
  `while (not fs.eof($f))` loop terminates cleanly on files
  ending with `\n`.
- **Polymorphic verbs.** `fs.readBytes`, `fs.writeString`,
  `fs.writeBytes` dispatch on the first-arg kind - string
  means path form, `fs.File` means handle form. Keeps the
  surface compact without magic.
- **Value-semantics carve-out.** `fs.File` handles share
  underlying state between copies via the integer id (same
  discipline as M16.0's `task of T`). Every other type keeps
  whole-value semantics.
- **Concurrency composition.** `spawn { return fs.readString(...); }`
  gives non-blocking use in one line; `task.waitAll` fans in
  multiple parallel reads. Under the default `jennifer` this
  parallelises across cores; under `jennifer-tiny` (TinyGo,
  cooperative single-threaded) it's correct-but-sequential.
- **What's deferred** (recorded so the design stays visible):
  streaming line iterator (`for (def line in fs.lines(path))`),
  `fs.copy` / `fs.chmod`, symlink ops, `fs.stat($f)` on an open
  handle, watch / notify (inotify), temp file / dir helpers,
  symlink-following in `fs.walk`.

See:
- [libraries/fs.md](libraries/fs.md) - reference doc with
  surface tables, worked examples, error surface, and the
  spawn-composition callout.
- [user-guide/imports.md](user-guide/imports.md) - `fs` row.
- [user-guide/concurrency.md](user-guide/concurrency.md) -
  the `spawn`-and-compose story `fs` builds on.

## M16.2 - `net`

**Status:** done.

Ships TCP + UDP sockets and DNS lookups on top of M16.0's
`spawn` composition model. The default `jennifer` binary
(standard Go) carries the full surface; `jennifer-tiny`
(TinyGo) ships friendly-error stubs at every entry point via
build-tag split.

- **TCP.** `net.connect(address) -> net.Conn`,
  `net.listen(address) -> net.Listener`,
  `net.accept($listener) -> net.Conn`,
  `net.readBytes($conn, n)` (blocks for at least one byte;
  returns up to n whatever's available; sticky EOF on close),
  `net.writeBytes($conn, b)`, `net.eof($conn)`,
  `net.address($conn)` peer, `net.address($listener)` local
  bound address.
- **UDP.** `net.listenUDP(address) -> net.UDPSocket`,
  `net.sendTo($sock, peer, bytes)`,
  `net.recvFrom($sock, n) -> net.Datagram{data, peer}`.
  Unconnected only - the same socket doubles as client and
  server via bind-to-`:0`.
- **DNS.** `net.lookup(host) -> list of string` (forward,
  Go's `LookupHost`), `net.reverseLookup(ip) -> list of string`
  (reverse, `LookupAddr`).
- **Polymorphic verbs.** `net.close($h)` and `net.address($h)`
  dispatch on the struct tag over Conn / Listener /
  UDPSocket. Boundary errors when passed the wrong shape.
- **Structs.** `net.Conn{id}`, `net.Listener{id}`,
  `net.UDPSocket{id}`, `net.Datagram{data, peer}`. Handle
  registries use the M15.6 integer-id pattern; three separate
  registries so wrong-type calls surface at the boundary.
- **Naming.** `net.connect`, not `net.dial` - plain-English
  verbs over Go idioms.
- **Value-semantics carve-out.** `net.Conn` / `net.Listener` /
  `net.UDPSocket` handles share underlying state between
  copies via the integer id. Same discipline as M16.0
  `task of T` and M16.1 `fs.File`.
- **Concurrency composition.** Blocking calls compose with
  `spawn` for non-blocking use; the accept-loop-with-
  spawn-per-connection pattern in the docs is the workhorse
  case.
- **TinyGo build-tag split.** `netlib_std.go` (`!tinygo`)
  implements the full surface via Go's `net` package;
  `netlib_tinygo.go` (`tinygo`) returns a friendly runtime
  error at every entry point. TinyGo 0.41 requires a netdev
  driver at runtime (Jennifer doesn't register one) and
  lacks `net.ListenPacket` for UDP; the stubs make the
  limitation visible at the call site rather than surfacing
  cryptic runtime errors. Same pattern as M15.3 `os.run` on
  TinyGo.
- **`examples/net.j`.** A tiny in-process TCP echo: spawn
  server, main-flow client, round-trip a payload. Uses `:0`
  + `net.address($listener)` to discover the bound port.
- **What's deferred** (recorded so the design stays visible):
  TLS, Unix domain sockets, socket options (SO_REUSEADDR /
  KEEPALIVE / NODELAY), timeouts / deadlines, DNS record-type
  helpers (`lookupMX`, `lookupTXT`, `lookupSRV`), explicit
  IPv6 control.

See:
- [libraries/net.md](libraries/net.md) - reference doc with
  TCP + UDP + DNS surface, address helpers, error surface,
  and the TinyGo note.
- [user-guide/imports.md](user-guide/imports.md) - `net` row.
- [technical/tinygo.md](technical/tinygo.md) - the netdev row
  in the restrictions table.
- [user-guide/concurrency.md](user-guide/concurrency.md) - the
  `spawn`-and-compose story `net` builds on.

## M16.3 - `regex`

**Status:** done.

Ships six verbs over `string` using Go's `regexp` package
(RE2 syntax). Pure string processing; no other library
dependencies; full TinyGo support.

- **Six verbs.** `regex.matches(pattern, s) -> bool`,
  `regex.find(pattern, s) -> regex.Match`,
  `regex.findAll(pattern, s) -> list of regex.Match`,
  `regex.replace(pattern, s, replacement) -> string` (`$1` and
  `${name}` in the replacement expand to captures),
  `regex.split(pattern, s) -> list of string`,
  `regex.escape(s) -> string` (metacharacter escape for
  literal matching).
- **`regex.Match`.** Fields `text`, `start`, `end`, `groups`
  (positional captures), `groupsNamed` (map for
  `(?P<name>...)` captures). `start`/`end` are rune indices,
  matching `strings.substring` and the rest of the string
  surface.
- **No-match sentinel.** `regex.find` returns a Match with
  `start=-1` when nothing matches (no nullable return types
  in the language today).
- **Implicit LRU cache** (128 entries) of compiled patterns
  keyed by the pattern string. Hot loops get compile-once
  behaviour without user bookkeeping. An explicit
  `regex.compile` + `regex.Pattern` handle would ship as
  M16.3.x if a benchmark ever demands it.
- **RE2 syntax.** No backreferences, no lookahead/lookbehind,
  guaranteed linear-time matching. Invalid patterns surface
  at the call site with the pattern quoted.
- **No build-tag split needed.** Go's `regexp` package works
  under TinyGo. Full surface in both binaries.
- **`examples/regex.j`.** Walks the whole surface: predicate,
  positional groups, named groups, no-match sentinel, replace
  with `$1` and `${name}`, split, escape round-trip.
- **What's deferred** (recorded so the design stays visible):
  `regex.compile` + Pattern handle, regex-over-bytes, streaming
  iterator, global-flag args (use `(?i)` in the pattern),
  `regex.count` (compose with `len(findAll)`), the RE2-forbidden
  features (backreferences, lookaround).

See:
- [libraries/regex.md](libraries/regex.md) - reference doc
  with surface tables, syntax cheat sheet, and worked
  examples for named groups + escape.
- [user-guide/imports.md](user-guide/imports.md) - `regex`
  row.

## M16.4 - `testing` (system-library primitives)

**Status:** done.

Ships the irreducible system-side surface a Jennifer-coded test
framework needs: `testing.run(name)` invokes a user method by
name, `testing.results()` / `reset()` manage the accumulator,
and `testing.report(results, format)` renders to text / TAP /
JUnit. The `.j`-side assertion vocabulary + CLI harness ship
in M18.x on top of these primitives.

- **`testing.run(name)`.** Looks up a zero-arg user method via
  the new `Interpreter.CallByName` primitive, times the run in
  Go, catches every failure mode into a `testing.Result`, and
  appends to the process-wide accumulator. Runs even when the
  named method doesn't exist (records `errorKind="unknown"`).
- **`testing.Result`.** `name`, `ms`, `passed`, `errorKind`,
  `errorMessage`, `file`, `line`, `col`. `errorKind` mirrors
  the try/catch strings (`"runtime"`, thrown-Error's `kind`,
  ...) plus a new `"exit"` value.
- **Exit interception.** `testing.run` is the ONE place in the
  language where `exit` is caught. Language-level `try`/`catch`
  deliberately does NOT catch exit; the runner catches it at
  the Go level so a runaway `exit` in a test body stays scoped
  to the runner without weakening the language guarantee. This
  carve-out is why the primitive can't live in `.j`.
- **Three report formats** via `testing.report(results, format)`:
  `"text"` (human terminal), `"tap"` (Test Anything Protocol
  v14), `"junit"` (JUnit XML). Codec-table shape - format
  strings are case-sensitive, matching `hash.compute` /
  `encoding.toText` / `fs.open`.
- **New interpreter helpers.** `Interpreter.CallByName(name)`
  invokes a zero-arg user method by string; `MethodNames()`
  enumerates every defined top-level method (for future
  test-discovery-by-prefix in the .j harness);
  `interpreter.ClassifyError(err)` extracts
  `(kind, message, file, line, col)` from any of the three
  interpreter sentinels (`*runtimeError`, `*ErrorSignal`,
  `*ExitSignal`) into the uniform tuple the library serialises.
- **Process-wide accumulator** guarded by a mutex, so
  `spawn { testing.run(...) }` from concurrent tasks doesn't
  race. Suites that share a process (or the fmt round-trip test
  path) should `testing.reset()` at the top of the run.
- **What's deferred** (recorded so the design stays visible):
  subprocess isolation for divergent tests (a future
  `--isolated` flag on the M18.x harness re-invokes
  `jennifer run testfile.j --testing-single name` per test
  via `os.spawn`), setup / teardown / fixtures, skip / xfail,
  parameterised tests, test discovery by prefix (primitive is
  there via `MethodNames`; policy lives in M18.x).

See:
- [libraries/testing.md](libraries/testing.md) - reference doc
  with worked failure-mode examples and the three-format
  walkthrough.
- [user-guide/imports.md](user-guide/imports.md) - `testing`
  row.
- The M18.5 `testing` module (Jennifer-coded assertions and
  suite driver built on these primitives).

## M16.5 - Interpreter performance pass

Closes the largest gaps the benchmark suite exposes
(see [technical/tinygo.md > Single-binary benchmark
results](technical/tinygo.md#single-binary-benchmark-results))
before Phase D's Jennifer-coded libraries (json, csv, http,
testing.j, ...) land on top. Those libraries are mostly tight
parsing loops and recursive descent; landing M16.5 first means
they ship at native-feeling speed instead of needing apology
paragraphs in the per-library docs.

**Where the cycles go.** The pre-M16.5 numbers from
`examples/benchmark.j` pointed at three distinct bottlenecks,
each fixable independently. The first is now closed by M16.5.1;
the other two remain as separate sub-milestones. Pre-M16.5
timings quoted below are from the historical run captured in
[technical/tinygo.md](technical/tinygo.md#per-workload-comparison-serial-section)
under "Pre-M16.5.1 comparison, same workloads."

1. **Value-semantic copy-on-write on every compound write.**
   **Closed by M16.5.1.** `$xs[] = item`, `$xs[i] = v`,
   `$m[k] = v`, `$p.field = v` used to deep-copy the root
   binding before mutating - O(N^2) for a sequence of N
   appends. Was the standout cost in `struct list build+read`
   (pre-M16.5: 10.7 s -> post: 0.06 s), `map insert+read`
   (9.8 s -> 1.7 s), and `string join` (4.2 s -> 0.02 s) on
   TinyGo. See M16.5.1 below.
2. **Name-based variable resolution at every reference.** Each
   `$n` reference walks the Environment chain by name; in a tight
   loop touching `$n`, `$d`, `$count` the resolver re-traverses
   the same chain on every iteration. Now the standout tax on
   `primes up to LIMIT` (42.4 s) and the rest of the
   compute-bound block. Addressed by M16.5.2.
3. **Method-call frame churn.** Every call allocates a fresh
   `Environment`, binds parameters one by one through map
   operations, and pays the method-table lookup at the call
   site. `fib(23)` runs at 4.7x the standard-Go cost - the
   recursion depth is small (~28 K calls), the body is trivial,
   the gap is dispatch overhead. Addressed by M16.5.3.

The user-visible behaviour stays unchanged. Value semantics still
hold, just implemented without the literal whole-tree copy on
every write. Lexical scoping still holds, just resolved at parse
time. Methods still bind by name at the source level; the
lookup is just done once.

### M16.5.1 - Refcount-based copy-on-mutation for `Value`

**Status:** done (shared-marker COW variant).

Ships the shared-marker copy-on-write protocol on compound
`Value`s. Cheaper than full refcount, correct where it matters,
and removes the O(N^2) tax on the "grow a list one item at a
time" pattern.

- **`Value.shared *bool` marker.** nil for freshly-constructed
  compounds and scalars. `Share()` sets it to a `*bool = true`
  when `evalExpr` returns a variable reference. Any downstream
  storage (Define, Assign, param binding, list/map element,
  struct field) sees the value as potentially aliased.
- **`Ensure()` at mutation sites.** `execAppend`,
  `execIndexAssign`, `execFieldAssign` all call
  `binding.Value.Ensure()` before mutating. If shared, Ensure
  DeepCopies to a private backing. Otherwise (append hot loop,
  where the binding is the only reference), Ensure returns v
  as-is and the mutation happens in place - no allocation.
- **`Copy()` stays as the public deep-copy** for libraries
  whose pattern is "Copy then mutate freely" (`lists.shuffle`,
  `lists.reverse`). Historical name preserved so no library
  needed changes. `snapshotForSpawn` calls `DeepCopy()`
  explicitly for value-semantics capture across goroutine
  boundaries.
- **Impact.** The append-in-a-loop pattern (`$xs[] = i` inside
  a while) dropped from ~35 s on 10K items (pre-M16.5.1) to
  ~40 ms on `jennifer-tiny` (TinyGo) - roughly 1000x. In the
  full benchmark suite (`examples/benchmark.j`), the three
  workloads dominated by the compound-COW pattern collapsed
  correspondingly: `struct list build+read` 10.7 s -> 0.06 s,
  `string join` 4.2 s -> 0.02 s, `map insert+read` 9.8 s ->
  1.7 s (all on `jennifer-tiny`). Serial total: 74.5 s ->
  48.9 s on tiny, 45.5 s -> 25.2 s on the default `jennifer`
  binary. The M15.5.3 benchmark note about swapping a Sieve
  of Eratosthenes for trial division no longer applies; the
  write pattern is now cheap enough.
- **Alias correctness.** The plan-required alias-stress suite
  is `internal/interpreter/value_alias_test.go` - 15 tests
  covering direct list aliasing, map aliasing, struct
  aliasing, structs containing lists, lists of structs, nested
  compounds, chained field writes, bytes append, function-arg
  mutation, and the hot-loop append pattern. Anyone touching
  mutation code must add coverage there.
- **Trade-off recorded**: the marker is one-directional (once
  true, never flips back). Pessimistic - a Value that was ever
  aliased detaches on next mutation even after the alias goes
  out of scope. A full refcounted COW would fix that; not
  scheduled.

See:
- [technical/interpreter.md > Value semantics](technical/interpreter.md#value-semantics) -
  the shared-marker + Share/Ensure/DeepCopy protocol in full.
- `internal/interpreter/value_alias_test.go` - the alias
  correctness suite.

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

Expected impact (baseline: M16.5.1 numbers in
[technical/tinygo.md](technical/tinygo.md#per-workload-comparison-serial-section)):
- `primes up to LIMIT`: 42.4 s -> ~15-20 s (2-3x) on tiny;
  21.1 s -> ~7-10 s on the default `jennifer` binary.
- `newton`, `monte carlo`: similar 2x range on both binaries.
- `fib`: contributes to the M16.5.3 win below.

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

Expected impact (baseline M16.5.1: `fib(23)` = 415 ms tiny /
89 ms go):
- `fib(23)` on `jennifer-tiny`: 415 ms -> ~80-150 ms (3-5x),
  in striking distance of the default `jennifer` binary's
  89 ms.
- `jennifer` `fib(23)` similarly shrinks; the ratio narrows
  to near-1x on both binaries.

### Combined target

Re-running `examples/benchmark.j` against the M16.5-final
`jennifer-tiny` binary should yield a serial total in the
15-25 s range (down from 48.9 s post-M16.5.1, and 74.5 s
pre-M16.5) and put TinyGo within ~1.5x of the default
`jennifer` binary on every workload shape - that's the floor
set by the TinyGo-vs-stdlib runtime gap and the right target
to aim for. The parallel section is a separate story and
belongs to a possible future "TinyGo parallel scheduler"
sub-milestone; the current runtime turns 4-way fan-out into
a *slowdown* (see
[technical/tinygo.md > Parallel section](technical/tinygo.md#parallel-section)),
which no M16.5.x work touches. Update
`technical/tinygo.md`'s tables with the post-M16.5 numbers
as the closing deliverable.

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
that ship to the system module directory
(`/usr/share/jennifer/modules/`) and are loadable without
recompilation. The path follows the FHS: `/usr/share` is for
architecture-independent, read-only data, which is exactly
what shipped `.j` modules are. `/var/lib` would be wrong -
FHS reserves it for *variable* state that the running system
modifies. The directory exists from M17.1; the modules shipped
in M18.x land directly in it.

The system module directory is baked into the interpreter at
build time via `-DJENNIFER_SYSMODDIR=/usr/share/jennifer/modules`
(propagated through the Makefile's codegen path in the same
shape as `internal/version/version_gen.go`, since TinyGo's
0.41 `-ldflags -X` is a no-op - see implementation-note 7 in
`CLAUDE.md`). Two runtime overrides let non-standard installs
(NixOS store paths, Homebrew cellars, local development
builds) point elsewhere without rebuilding:

- **Environment variable `JENNIFER_SYSMODDIR`** - checked
  first so distro packages that bake in a path can still be
  redirected by a shell wrapper without touching the binary.
- **CLI flag `--sysmoddir=PATH`** - overrides both the
  environment and the compile-time default; applies only to
  the current invocation. Useful for `jennifer run` inside a
  Nix shell or a per-project scratch tree.

Resolution precedence: `--sysmoddir` > `JENNIFER_SYSMODDIR` >
compile-time default. The chosen path is stored on
`Interpreter` at construction so M17.3 module resolution can
consult it without re-parsing argv. Validation is
asymmetric: `--sysmoddir` and `JENNIFER_SYSMODDIR` are
checked at `Run()` entry (the user named a specific path;
silently ignoring it would be the wrong default) and refuse
to start on a missing or non-directory target. The
compile-time default is best-effort - it stays uncomplaining
until a module import actually needs it, so
`jennifer run myscript.j` still works from a fresh developer
checkout where `/usr/share/jennifer/modules/` isn't
installed yet. See [M17.3](#m173---module-resolution) for
the resolution-time behaviour that ties this together.

**The effective sysmoddir is inspectable at two surfaces**
so a debugger, a bug report, or a Jennifer program can see
which of the three layers won:

- `meta.SYSMODDIR` - a string constant on the `meta`
  namespace (M15.1) alongside `meta.VERSION` and
  `meta.BUILD`. Programs can read it, print it, or gate
  behaviour on it; matches the interpreter-self-identity
  role `meta` already owns.
- `jennifer version -v` - the CLI's extended version output
  prints `VERSION`, `BUILD`, and `SYSMODDIR`, each tagged
  with the layer that supplied it (`cli`, `env`, or
  `compile`). Aimed at humans and bug reports.

Both shapes point at the same resolved path; the CLI form
is the one printed inside the "please file a bug" prompt so
non-standard installs never have to guess.

## M17.2 - `import "foo.j" as foo;` syntax

The real module boundary. The imported file's exported
top-level names live behind the `foo.` prefix at the call
site, the same shape as a namespaced system library.
Aliasing rules match `use NAME as ALIAS;` (alias replaces
the canonical name; bare form reserves the file-stem as the
prefix). Reuses the namespacing machinery shipped in M10 so
call-site resolution is already settled.

**Collisions rejected at `Run()` entry.** Two imports
aliased to the same prefix fail with a duplicate-alias
error, mirroring M10's rejection of a duplicate `use NAME;`.
A module prefix that would shadow (or be shadowed by) an
already-imported system library's prefix also fails - the
second import is the one flagged, whichever shape it takes.

**Parse errors in an imported file surface as positioned
errors** rooted in the imported file (path + line + col),
consistent with the existing file-import preprocessor's
error propagation.

## M17.3 - Module resolution

Two disjoint import shapes, distinguished at the character
level so a reader can tell "local file" apart from "system
module" without leaving the source line:

- `import "./foo.j" as foo;` (or `../`) is a **local path
  import**. Always resolves relative to the importing file's
  directory. No search path is consulted. This is the only
  way to import a file from the current tree.
- `import "/opt/mymods/foo.j" as foo;` (leading `/`) is an
  **absolute path import**. Lands at exactly the named file
  and bypasses every search path, including the system
  module directory. Rare in practice - reserved for the
  cases where the user really does want to name a specific
  file on disk without indirection.
- `import "foo.j" as foo;` (no `./`, no `../`, no leading `/`)
  is a **module import**. The search path is walked in order:
  first the system module directory
  ([M17.1](#m171---source-tree-separation);
  `--sysmoddir` > `JENNIFER_SYSMODDIR` > compile-time default),
  then any `-I DIR` command-line directories in the order
  given. The current source file's directory is NEVER
  consulted for a module import. If nothing matches, the
  import errors with the ordered list of paths that were
  searched.

**Why the two shapes are disjoint.** A single "find this file
somewhere" syntax that searches the working directory first is
a supply-chain attack vector - the Jennifer equivalent of DLL
hijacking or `PYTHONPATH`-style import shadowing. An attacker
who can drop a file into the working directory of the target
program can silently replace what looks like a system module
reference (`import "sample.j" as s;`) with their own code, and
the source of that program tells the reader nothing about
which of the two just happened. Mandating `./` for local files
makes intent visible: `"./sample.j"` is always the file next
to the importing source, `"sample.j"` is always a module the
distro (or `-I`) has vetted. The two forms can never collide
because the `./` prefix is a lexical marker at the string
level.

**Precedence within the module search path**
(system > `-I`, not the other way around) prevents a stale or
malicious `-I` entry from shadowing a distro-shipped module
that a program depends on. `-I` still works for local
development (drop a module tree under `~/proj/modules/`, pass
`-I ~/proj/modules`), and for testing patched modules against
a distro binary, but it can only add names, not override
existing ones. If a program *needs* to override a system
module (rare - usually the right answer is to patch and
rebuild the interpreter or use `--sysmoddir` to point at a
whole alternate tree), the escape hatch is
`--sysmoddir=~/proj/modules` for that invocation, which is a
noisy, deliberate choice.

**Duplicate module names across `-I` directories are a hard
error.** If the user passes `-I a -I b` and both directories
contain `foo.j`, the first `import "foo.j" as foo;` fails at
`Run()` entry with both matching paths named. This mirrors
the batch-mode rejection of a duplicate `use NAME;` and rules
out silent first-match-wins shadowing between two entries the
user themselves added. Duplicates between the system directory
and an `-I` entry are also errors under the same rule (system
still wins if the user forces resolution via
`--sysmoddir`, but a stray `-I` overlay is loud, not silent).

**Missing paths are strict only when the user names them.**
`--sysmoddir` and `JENNIFER_SYSMODDIR` are validated at `Run()`
entry: if either points at a nonexistent or non-directory
target, the program refuses to start (the user asked for a
specific location, so silently ignoring it would be the wrong
default). The compile-time default is treated as
best-effort: if `/usr/share/jennifer/modules/` doesn't exist
(developer machine, `go build` without `make install`), that
is fine until the first module import actually looks in it,
at which point the resolution error names the same missing
path in its ordered list. This keeps `jennifer run
myscript.j` working from a fresh checkout even when the
distro tree isn't installed - the failure only fires when a
module import genuinely needs the system dir.

No implicit fallback to system libraries in either shape - a
`use NAME;` always means a Go-built system library,
`import "..." as ...;` always means a Jennifer-coded module
file. Matches the explicit-prefix rule recorded in
[rejected.md > Implicit `use NAME;` fallback chain](technical/rejected.md#implicit-use-name-fallback-chain-m8).

## M17.4 - Module-level exports

Top-level `def`, `def const`, `def struct`, and `func` are
**private by default**; a leading `export` marker publishes
the name into the module's public namespace. One marker,
one direction - no `public`/`private` dual sugar. Follows
the lineage of Rust `pub`, TypeScript `export`, and Java's
package-private + `public` split.

Rationale (stance 2: explicit over implicit). The public
API surface of a module has to be visible in the module's
source without knowing a default rule. A leading `export`
answers "what does this module expose?" by grep. A helper
accidentally left unmarked stays internal rather than
silently leaking into the public surface - the same
supply-chain hygiene argument that motivates M17.3's
mandatory `./` prefix for local file imports. Cost: every
intended-public name carries one extra token; worth paying
for the stance-2 alignment.

Accessing a private name from outside the module is a
positioned error at the call site
(`foo.helper: 'helper' is not exported from module 'foo'`).
Struct exports carry the whole struct type, including its
fields - there is no field-level visibility knob.

**Visibility is a module concept, not a script concept.**
Scripts (`.j` files run directly via `jennifer run`) carry
no `export` markers, and the absence isn't an inconsistency
with the module rule - it's the same rule applied to a
structurally different situation. Visibility only becomes
meaningful when a file has an external caller: a script has
no importer, so its "public API" is the empty set by
construction, and a marker would be noise. A module has
importers by definition, so the marker declares which names
cross the boundary. Same pattern every language that takes
visibility seriously ships (Rust `pub`, TypeScript `export`,
Java's package-private + `public`, Go's capitalization);
none considers the asymmetry with unimported entry points
an inconsistency.

A useful side effect: a script promoted to a module needs
`export` markers added deliberately. That is the "I now
commit to a public API" moment, and forcing it to be a
deliberate edit prevents a helper that happened to live in
the file from silently leaking into the module's API
surface when the file is first imported. Same supply-chain
hygiene reasoning as M17.3's mandatory `./` prefix.

No `private` marker and no `public` synonym for `export` -
"no marker" already means private, and stance 1 rules out
two spellings for the same thing.

Ships last because it's the only piece that touches parser
grammar - M17.1 through M17.3 are tooling and resolution.

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
  both forms so we don't ship a renderer. Unknown
  `--format` values are rejected at argv parse (positioned
  error naming the valid choices), not deferred to output
  time.
- **Allocation tracking.** Optional second mode
  (`--allocs`) that surfaces the M16.5.1 shared-marker COW
  hot spots: `Ensure()` calls that actually detached a
  shared backing (the site where value semantics turned a
  logical mutation into a real copy) and `DeepCopy()`
  invocations from `snapshotForSpawn` (the frame captured
  when a `spawn` block launches). Jennifer's value
  semantics make every detachment load-bearing; surfacing
  "this struct's Ensure detaches on 40 frames per call" is
  exactly the kind of insight that compiled-language
  allocation profilers give. Same output formats as the
  statement profile. `--allocs` is a mode switch, not a
  layer over the statement profile: `--allocs` alongside
  `--format=trace` is rejected at argv parse rather than
  producing an ambiguous mixed stream. If a future
  workload needs both, a separate `--allocs=both` or a
  side-by-side `--profile-and-allocs` shape lands then.
- **TinyGo-friendliness.** Profiling code lives behind the
  `profile` subcommand and a build tag so it doesn't bloat
  the shipping `jennifer` binary. Minimal-footprint embedding
  builds get the profiler-free variant; desktop / dev builds
  get both.
- **Out of scope for v1.** No source-step debugger (its
  own milestone if/when it lands).
- **May absorb future developer-tooling work.** Source-level
  debugger hooks, runtime introspection, and any other
  `jennifer <tool>` subcommand that instruments the
  evaluator can ship as sub-milestones under M17.5.x rather
  than getting their own top-level slots. M17.5.1 (linting)
  is the first such sub-milestone.

### M17.5.1 - `jennifer lint`

A `jennifer lint <prog.j>` subcommand that reports patterns
which are compile-legal but stylistically or semantically
suspect. Distinct from `jennifer fmt` (which normalises
lexical shape) and from the parser (which rejects the
outright illegal): the linter's slot is "the code parses, it
runs, and something about it is still worth flagging."

**Why bother when the parser already catches so much.**
Undefined variables, wrong types at declared sites, arity
mismatches, const reassignment, no-shadowing, const-name
rule violations, and (after M16.5.2) unused locals are all
parse-time or first-execution errors. A linter that only
duplicated those wouldn't earn its keep. What's left:
dead-code detection past a `return` / `throw` / `exit` /
`break` / `continue`, empty catch blocks that swallow
errors, `try` bodies that can't throw, `throw` sites that
don't carry an `Error` struct (convention violation),
methods whose block-nesting depth or statement count
exceeds a threshold, always-true / always-false
comparisons a reader can spot statically, and (over time)
deprecation warnings as language features change.

**Check set for v1** (each check gets a stable ID so
suppression comments and CI configuration are portable):

- `L001 unused-local` - a `def NAME as T init ...;` whose
  binding is never referenced. Parse-time analysis on top
  of M16.5.2's scope pass; runs cheap.
- `L002 dead-code-after-terminator` - statements
  syntactically unreachable because a preceding sibling
  is `return;` / `throw ...;` / `exit ...;` /
  `break;` / `continue;`.
- `L003 empty-catch` - `catch (e) { }` with no body. The
  handler receives the error and discards it silently -
  almost always a mistake; if it's deliberate, the
  suppression comment makes the intent explicit.
- `L004 throw-non-error` - a `throw` whose expression
  isn't an `Error{...}` struct literal or a variable of
  type `Error`. The convention (documented in M13.2 and
  `docs/user-guide/control-flow.md`) is that user code
  throws `Error`; catches downstream assume the field
  shape. Bare-value throws break that contract.
- `L005 method-too-long` - method body exceeds a
  configurable statement count (default 60). Same
  "one concern per method" rationale as
  [style-guide.md > Loops](../user-guide/style-guide.md#loops)
  applies to methods.
- `L006 nesting-too-deep` - block-nesting exceeds a
  configurable depth (default 4). Same shape as
  `list of list of list of list of ...` being flagged
  as a smell in the style guide.
- `L007 constant-condition` - `if (true)`, `if (false)`,
  `while (true)` (unless followed by `break` inside),
  `if ($x == $x)`. Runtime consequences vary; the
  static pattern is almost always a bug or leftover
  scaffold.
- `L008 deprecation` - a family, not a single rule.
  Populated by future milestones when they retire an
  API surface. Empty on M17.5.1 landing; grows over
  time. Each entry names the successor. When a
  milestone retires a name, it lands the corresponding
  `L008` sub-rule in the same release the old form
  still runs, so the deprecation warning is available
  the moment the replacement ships.

**Suppression.** A `# lint-disable: L003` line-trailing
comment suppresses the specified check on that statement.
A `# lint-disable-file: L005, L006` at file head
suppresses across the whole file. No blanket "disable
all" - suppression names IDs, so a review can grep for
what was silenced. The lexer already carries trivia
tokens (M14), so the linter reads suppressions off the
same token stream it walks for checks.

**Output shapes** (same rationale as M17.5's profile
`--format` flag - one measurement, multiple consumption
targets):

- Human-readable default (positioned diagnostics with
  source-context carets, matching the existing
  parse-error and runtime-error rendering in
  `cmd/jennifer/main.go`).
- `--format=json` machine-readable stream for editor
  integration (LSPs, CI annotations). One JSON object
  per finding: `{id, file, line, col, message, severity}`.
- `--format=github` GitHub Actions annotations
  (`::error file=…,line=…,col=…::message`). Cheap to
  add; matches `go test`'s convention.

**Exit code semantics.** `0` = clean (no findings above
the configured severity floor). `1` = findings at or
above the floor. `2` = the linter itself failed
(parse error, IO failure). Same triaging shape as
`gofmt -l` and `shellcheck`.

**Configuration.** A single `--checks=IDS` flag on the
CLI covers per-run tuning; a leading `!` on an entry means
"exclude" (`--checks=!L005,!L006` runs everything except
those two; `--checks=L001,L002` runs only those two).
One flag, one direction - stance 1 rules out shipping
`--check` and `--exclude` as inverted sugar for the same
job. A `.jennifer-lint` file at the source-tree root
(optional) covers per-project defaults using the same
`IDS` / `!IDS` format, one entry per line, comments
start with `#`. No YAML, no TOML - matches the
small-tooling philosophy of the rest of the CLI.

**Unknown check IDs are always errors** - in
`--checks`, in `.jennifer-lint`, and in
`# lint-disable:` suppression comments. Silently
ignoring a typo would defeat the auditability the ID
scheme buys. A retired ID that used to exist errors with
a positioned message naming the successor when the
retirement landed an `L008` sub-rule, or just naming
the removal when it didn't.

**TinyGo-friendliness.** Same story as the profiler:
lint checks live behind the `lint` subcommand and a
build tag so they don't bloat the shipping
`jennifer-tiny` binary. Minimal-footprint embedding
builds get the lint-free variant; desktop / dev builds
get both. Shares the scope-analysis pass with M16.5.2
where the checks overlap (`L001 unused-local`), so the
linter doesn't duplicate the analyser it needs.

**Out of scope for v1.** Auto-fixing (the linter reports;
the human decides), whole-project / cross-module
analysis (per-file for v1; module-graph analysis after
M17 modules land), and any check that would need type
inference beyond what `def NAME as TYPE` already gives.

## M17.6 - Public interpreter API for third-party embedding

Extract the interpreter core out from under `internal/` and
expose a documented Go-side surface so external programs can
embed Jennifer. Today `internal/interpreter`,
`internal/parser`, `internal/lexer`, and `internal/lib/*` are
unreachable from any module that isn't `github.com/mplx/jennifer-lang`
- Go's `internal/` visibility rule is not a convention, it's
a compile-time barrier. No submodule / require / replace
workaround exists; embedding is impossible without a
restructure.

**Concretely.** Add a `pkg/` top-level (working name; the
final path settles at start of M17.6):

- `pkg/interpreter` re-exports `Interpreter`, `Value`, error
  types, and the `Install(in *Interpreter)` registration API
  that every stdlib library already uses. The `internal/`
  packages stay as the implementation; `pkg/` is the stable
  facade with semver-covered surface once we ship 1.0.
- `pkg/lib/*` re-exports each shipped library (`convert`,
  `math`, `strings`, ...) so a host can install the ones it
  wants and leave out the rest. Non-breaking for the current
  CLI - `cmd/jennifer` picks up the same `Install` calls,
  just through `pkg/lib` shims instead of directly.
- Documented pluggable interfaces for the host-provided
  facilities the OS-touching libraries currently reach for:
  - `io.Writer` for `io.printf` output (already a
    `*Interpreter` field; formalize as an interface).
  - `io.Reader` for `io.readLine` / `io.readBytes` /
    `io.readChars` stdin.
  - `Clock` for `time.now()` / `time.local()` / `time.sleep`
    (the `nowFunc` / `sleepFunc` test hooks in
    `internal/lib/time` are the shape).
  - `Rand` for `math.rand*` / `lists.shuffle` (the shared
    random source).
  - Filesystem / network / process hooks left as future
    work - a host wanting those either installs the
    stdlib libraries as-is (accepting the Go `os` /
    `net` dependencies) or ships its own shims. A
    documented registration pattern is the deliverable;
    the shims themselves are per-host and out of scope
    for M17.6.

**Stdlib-backed defaults.** Each pluggable interface
carries a working default so `pkg/interpreter.New()`
plus `pkg/lib/io.Install(in)` produces a running
interpreter without every embedder wiring up seven
interfaces first. `Clock` defaults to Go's `time.Now`,
`Rand` to a `math/rand` source, `io.Writer` to
`os.Stdout`, `io.Reader` to `os.Stdin`. Hosts override
only what they need. A `no-os` embedder replaces every
default; a Slack-bot embedder swaps just `io.Writer`
for its outgoing-message pipe and leaves the rest.

**Boundary rules at the Install site.** Three explicit
error paths so hosts get loud, positioned failures
instead of subtle misbehaviour:

- **Duplicate library `Install` at the Go level is
  rejected**, mirroring how a duplicate `use NAME;`
  errors at the Jennifer level (M10 rule, lifted). A
  host installing `pkg/lib/math` and then its own
  shim that also claims the `math` namespace fails at
  the second `Install` call, not silently overlaid.
- **`Install` and pluggable-interface setters are
  frozen once `Run()` starts.** Attempts to call
  `Install`, `SetClock`, `SetOut`, or friends after
  the interpreter has begun executing produce a
  positioned "cannot configure interpreter mid-run"
  error at the Go call site. The interpreter can then
  trust its host bindings for the rest of the run
  without defensive re-checks.
- **Host implementations are trusted at the interface
  boundary.** The interpreter uses whatever `Clock.Now()`
  or `Rand.Int63()` returns without validation - a
  broken host implementation is the host's problem, not
  the interpreter's. Stated so hosts don't expect
  defensive checks that aren't there and so downstream
  bug reports are triaged to the correct side of the
  API boundary.

**Non-goals.**
- A hosted no-`os` build target. Even after M17.6, the
  shipping stdlib libraries lean on Go's `os` / `net` /
  `time` packages. A truly bare-metal or `no-os` embedding
  can only use the pure-value libraries (`convert`, `math`,
  `strings`, `lists`, `maps`, `hash`, `crc`, `encoding`,
  `regex`) plus whatever host-provided shims the embedder
  wires up. That's a design constraint on the embedder, not
  a milestone on Jennifer's side.
- Semver freezing the public API. Jennifer stays pre-1.0
  through the M17.x series; M17.6 documents what's exported
  and how libraries plug in, but breaking changes to that
  surface remain allowed until 1.0.0 (per the stability
  section in `README.md` and `CLAUDE.md`).

**Motivation.** Third-party embedding has multiple concrete
consumers already imagined: scripting-language slot in a Go
application, tooling that needs direct AST / interpreter
access (LSP, formatter integrations, syntax highlighters),
test harnesses that want to drive `.j` programs from Go,
config-DSL runtimes, plugin systems for game engines and
similar. None of them require an OS-free build; all of them
need the `internal/` -> `pkg/` restructure.

The `Install` pattern already works this way - every stdlib
library is a `pkg.Install(in)` call. The missing piece is
visibility, plus documented hooks for the pieces of host
state currently exposed only as package-level test vars.

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
- **FCGI.** `use FCGI as web;` library when `net` and `httpd`
  mature. Lets Jennifer host CGI / FastCGI workloads end-to-end.
- **Inline assembler.**
- **Binary AST cache (`.jc` files).** Pre-parsed loading for big
  programs and embedded scripting hosts. Its own milestone when
  it lands - file-format design, versioning, and TinyGo-safe
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
- **Host-embedding API.** *Promoted to a numbered milestone -
  see [M17.6](#m176---public-interpreter-api-for-third-party-embedding).*
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
