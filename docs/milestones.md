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
  printed. See [technical/cli_repl.md > REPL](technical/cli_repl.md#repl-cmdjenniferreplgo).
- **REPL line editor** - cursor keys, Home/End, word motions, Ctrl+W /
  Ctrl+U / Ctrl+K, in-memory history (Up/Down), Ctrl+C cancel. Non-TTY
  stdin falls back to plain line reading. See
  [technical/cli_repl.md > Line editor](technical/cli_repl.md#line-editor-cmdjenniferlineeditgo-cmdjenniferhistorygo).
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
  [technical/cli_fmt.md > Formatter](technical/cli_fmt.md#formatter-cmdjenniferfmtgo).
- **Inspection subcommands** - `jennifer tokens <file>` dumps the lexer
  output; `jennifer ast <file>` dumps the preprocessed AST as JSON.
  See [technical/cli_inspect.md > Inspection](technical/cli_inspect.md#inspection-tokens-and-ast).
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
- [technical/cli_fmt.md](technical/cli_fmt.md) - `fmt`'s trivia re-emission.

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
[M25+](#m25---encoding-long-tail-codecs). See
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
  - `tinygo build -stack-size=2mb` is now in the Makefile.
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
  the `-stack-size=2mb` Makefile flag and the TinyGo scheduler
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
  subprocess isolation for divergent tests (the `--isolated`
  flag on the M16.8 `jennifer test` subcommand re-invokes
  `jennifer run testfile.j --testing-single name` per test
  via `os.spawn`), setup / teardown / fixtures, skip / xfail,
  parameterised tests, test discovery by prefix (primitive is
  there via `MethodNames`; policy lives in M16.8).

See:
- [libraries/testing.md](libraries/testing.md) - reference doc
  with worked failure-mode examples and the three-format
  walkthrough.
- [user-guide/imports.md](user-guide/imports.md) - `testing`
  row.
- M16.8 (assertion vocabulary + `jennifer test` subcommand +
  `--testing-single` flag built on these primitives).

## M16.5 - Interpreter performance pass

**Status:** shipped as five sub-milestones (M16.5.1 through M16.5.5).

Closed the largest gaps `examples/benchmark.j` exposed before Phase D
(json, csv, http, testing.j, ...) lands on top, so those Jennifer-coded
libraries ship at native-feeling speed instead of needing apology
paragraphs in the per-library docs. User-visible behaviour is unchanged
- value semantics, lexical scoping, and by-name method resolution all
still hold; the implementation just avoids the whole-tree copy, the
name-map walk, and the per-call frame allocation. Pre-M16.5 baseline
numbers in
[technical/tinygo.md](technical/tinygo.md#per-workload-comparison-serial-section)
under "Pre-M16.5.1 comparison, same workloads."

**What shipped:**

- **M16.5.1 - Shared-marker copy-on-write on compound `Value`s.**
  `Value.shared *bool` marker + `Share()` at read sites +
  `Ensure()` at mutation sites; deep-copy only when the flag is
  set. Killed the O(N^2) tax on `$xs[] = item` in a loop -
  append-in-a-loop dropped from ~35s on 10K items to ~40ms on
  `jennifer-tiny`. Alias correctness pinned by
  `internal/interpreter/value_alias_test.go` (15 tests). See
  [technical/interpreter.md > Value semantics](technical/interpreter.md#value-semantics).
- **M16.5.2 - Lexical slot resolution at parse time.** Every
  variable / constant reference gets a `(Depth, Slot)`
  coordinate from `internal/parser/resolver.go` (runs from
  `Interpreter.Run`, idempotent, REPL bypasses). `Environment`
  gains a `slots []Binding` slice alongside the name map;
  `DefineAt` / `GetAt` / `AssignAt` are the fast paths and
  `Binding.Slot` keeps the two views in sync. Undefined-variable
  and shadowing errors promote to parse-time diagnostics.
  Deliberate carve-outs: spawn body unresolved (two-frame
  snapshot doesn't align), try body inline (execTry uses
  execStmts), for-header separate from body.
- **M16.5.3 - Method call frame optimization.** Package-level
  `sync.Pool` behind `borrowBlockEnv` / `releaseBlockEnv`
  recycles frames across every `execBlock` and `evalCall`.
  `CallExpr.Method *MethodDef` is pre-filled by the resolver so
  the runtime skips a hash lookup per user-method call.
  Parameters bind through `DefineAt` into pre-sized slots.
  Target: `fib(23)` on `jennifer-tiny` 415ms -> ~80-150ms
  (3-5x), in striking distance of the default `jennifer`
  binary's 89ms.
- **M16.5.4 - Namespaced-call + micro fast paths.**
  `QualifiedCallExpr.Fn` / `QualifiedConstRefExpr.Const`
  pre-filled by `Interpreter.resolveQualifiedRefs` (a second
  pass after `processImports`); three map hits per namespaced
  call collapse to one type assertion. `evalComparison` grew
  int-int / float-float fast paths mirroring `evalArithmetic`.
  `bindParamValue` skips `Copy` + stamp for scalar-Kind args.
  `effectiveGlobal` becomes an O(1) read via `Environment.root`.
- **M16.5.5 - Expression micro-optimizations.** `BinaryExpr` /
  `UnaryExpr` gained a `Folded Expr` field the resolver stamps
  when the whole subtree is a compile-time literal
  (`internal/parser/fold.go`; chains transitively so
  `((1+2)*3)+4` collapses to `IntLit(11)`). Runtime-erroring
  ops (div by zero, negative shift) stay unfolded so the
  runtime hits the same error at the same source position.
  `Value.Share()` grew a range-compare fast path so the
  inliner eliminates the call at scalar VarExpr reads.

**Combined target.** Re-running `examples/benchmark.j` against the
M16.5-final `jennifer-tiny` binary should yield a serial total in the
15-25s range (down from 48.9s post-M16.5.1, and 74.5s pre-M16.5)
and put TinyGo within ~1.5x of the default `jennifer` binary on every
workload shape - the floor set by the TinyGo-vs-stdlib runtime gap.
The parallel section is a separate story and belongs to a possible
future "TinyGo parallel scheduler" sub-milestone; the current runtime
turns 4-way fan-out into a *slowdown* (see
[technical/tinygo.md > Parallel section](technical/tinygo.md#parallel-section)),
which no M16.5.x work touches. Update `technical/tinygo.md`'s tables
with the post-M16.5 numbers as the closing deliverable, measured on
the reference Ryzen 5 7600X3D.

## M16.6 - Developer tooling: linting

`jennifer lint <prog.j>...` reports patterns that are compile-legal but
stylistically or semantically suspect - the slot between `jennifer fmt`
(lexical shape) and the parser (the outright illegal). Internals:
[technical/cli_lint.md](technical/cli_lint.md).

**Checks.** Stable IDs, grouped by concern - the leading digit is the
group: **L0nn** source errors, **L1nn** correctness, **L2nn** complexity
and style, **L3nn** API lifecycle.

- `L001 lex-error` / `L002 parse-error` / `L003 preproc-error` - the file
  couldn't be tokenized / parsed / spliced.
- `L004 invalid-directive` - a malformed or unknown-ID `# lint-disable`.
- `L101 unused-local` - a local `def` binding never read. Skips
  declarations inside a `spawn` body (inherits M16.5.2's resolver
  carve-out); reads inside a spawn still count.
- `L102 dead-code-after-terminator` - a statement after
  `return`/`throw`/`exit`/`break`/`continue`.
- `L103 empty-catch` - a `catch` with no body.
- `L104 throw-non-error` - a `throw` not statically an `Error` (an
  `Error{...}` literal or a `$var` declared `Error` pass; any other
  shape is flagged).
- `L105 constant-condition` - `if (true)`/`if (false)`, `while (true)`
  with no break/return/throw/exit escape, `if ($x == $x)`.
- `L201 method-too-long` - body over a statement threshold (default 60).
- `L202 nesting-too-deep` - block nesting over a depth threshold (default 4).
- `L203 line-too-long` - a source line over the column limit (default 100).
- `L301 deprecation` - reserved family, empty until an API is deprecated.
- `L302 removed-api` - use of a removed API (e.g. `use core;`).

The `L0nn` source errors are always on and not user-selectable (the file
doesn't parse, so there's nothing to configure); every `L1nn`/`L2nn`/`L3nn`
check is selectable via `--checks`.

**Suppression.** `# lint-disable: IDS` (trailing) silences on that line
(the line the finding anchors to - the `func` line for `L201`, the
block-introducer line for `L202`); `# lint-disable-file: IDS` silences
file-wide. No blanket disable-all - directives name IDs. Read off the
trivia token stream, since the parser strips comments. A malformed /
unknown-ID directive is continue-and-report: it becomes an `L004` finding
and suppresses nothing. A doubled marker (`## lint-disable: ...`) is an
ordinary comment.

**Config.** `--checks=IDS` (per run) or a `.jennifer-lint` file (per
project), one direction: all includes ("only these") or all `!excludes`
("everything except"); mixing errors. Unknown IDs are always an error;
naming an always-on `L0nn` in `--checks` is rejected. Messages are terse
(`unknown check ID "L999"`) - `jennifer lint --help` lists the catalog.

**Output.** `--format=human` (source-context carets, default),
`--format=json` (a JSON array of `{id,file,line,col,message,severity}`,
`[]` when empty, one array across all files), `--format=github` (Actions
annotations). Lex / preprocess / parse failures render as `L0nn` findings
in the chosen format, not stderr bail-outs, so a JSON pipeline always
gets valid output. Exit `0` clean / `1` findings at or above the warning
floor (a source error counts) / `2` an invocation failure with no source
position (bad flags, IO, bad config). CI lints `examples/*.j` as a gate.

**TinyGo.** Behind a build tag; the run-only `jennifer-tiny` omits it
(along with `tokens` / `ast` / `fmt` / `profile`).

**Out of scope for v1.** Auto-fixing, cross-module analysis (per-file;
revisited after M17), and checks needing type inference beyond
`def NAME as TYPE`.

## M16.7 - Developer tooling: profiling

`jennifer profile <prog.j>` runs the program with the evaluator
instrumented and attributes work back to Jennifer source positions - the
gap `go tool pprof` leaves (it profiles the interpreter binary, not the
.j program inside it). Program output goes to stderr so the profile owns
stdout. Internals: [technical/cli_profile.md](technical/cli_profile.md).

- **Statement profile** (default). Per source position: hit count and
  self / cumulative wall-clock time. Instruments statements and method
  calls, not every expression - a `time.Now()` around each literal read
  would swamp the profile.
- **Allocation profile** (`--allocs`). Value-semantics copies per
  position: eager copies (def / assignment / parameter binding of a
  compound value - where the real cost is), COW `Ensure()` detachments
  (kept for correctness but at or near zero for `.j` code, since every
  store copies eagerly and the append/index hot loop stays unshared), and
  `snapshotForSpawn` deep copies. A mode switch, not a layer on the
  statement profile. `examples/profile.j` exercises all three.
- **Formats.** `--format=table` (default), `--format=pprof` (gzipped
  protobuf, hand-encoded for zero deps; `go tool pprof` and
  speedscope.app read it), `--format=trace` (Chrome-trace of the call
  timeline). Unknown `--format` and `--allocs --format=trace`
  (allocation events have no timeline) are rejected at argv parse.
- **TinyGo.** Behind the `profile` subcommand + build tag; the run-only
  `jennifer-tiny` omits it.
- **Out of scope for v1.** A source-step debugger (its own milestone if
  it lands). Future evaluator-instrumenting tools can land as M16.7.x
  sub-milestones.

## M16.8 - Testing framework consolidation

Adds the assertion vocabulary and CLI orchestration on top of M16.4's
dispatch primitives (`testing.run` / `results` / `report`), so a suite
doesn't reduce to hand-written `throw Error{...}` plus a bespoke runner.
Internals: [technical/cli_test.md](technical/cli_test.md) and
[libraries/testing.md](libraries/testing.md).

**Assertions** (`testing` library, in both binaries). Six builtins that
reduce to `Value.Equal` / Kind dispatch in Go and, on failure, throw a
canonical `Error{kind: "assertion"}` positioned at the assertion call,
which `testing.run` catches and classifies: `assertEqual` /
`assertNotEqual` (deep structural equality), `assertTrue` /
`assertFalse` (require bool), `assertContains` (substring / list element
/ map key, by haystack kind), and `assertThrows(name, kind)` (calls the
named method and asserts it throws an `Error` of that kind). Native
speed - no per-call interpreter overhead.

**Arg-taking dispatch.** `Interpreter.CallByNameWith(name, args...)`
binds arguments to a method's parameters with the normal arity and
declared-type checks; zero-arg `CallByName` stays the compat entrypoint.
`testing.runWith(name, argsList)` mirrors `testing.run` for methods that
take arguments - for framework dispatchers, not the zero-arg test
methods the runner discovers.

**`jennifer test FILE.j`** (dev subcommand, `!tinygo`). Discovers
zero-arg methods by the `test*` convention (or `--filter=REGEX`), runs
each through the runner with optional `setUp` / `tearDown` hooks, and
reports in `--format=text|tap|junit`. `--isolated` runs each test in a
fresh interpreter subprocess (via `--testing-single METHOD`, the per-test
entry the parent fires), trading richer detail for clean state. Exit
`0` pass / `1` failures / `2` runner error, the `jennifer lint` shape.

**Enabling interpreter change.** Builtins can now raise a catchable
Jennifer error: a builtin returning `interpreter.RaiseError(kind, msg,
...)` - an `*ErrorSignal` wrapping the canonical `Error` - propagates
unwrapped instead of being flattened into a `runtimeError`, and
`BuiltinCtx` carries the call-site position so the error anchors there.

**Conventions.** Test entry points are zero-arg; the subject under test
takes any signature (called with concrete values in the body).
Table-driven tests are a body loop (`for (def c in $cases) {
testing.assertEqual(...); }`) - fail-fast, the first failing row throws
with its position. `examples/testsuite.j` is a runnable sample.

**Deferred to M17.** White-box testing overlays (a `MODULE_test.j` with
private-name visibility into a module) depend on the module system; they
land with [M17.4](#m174---exports-and-visibility), not here.

## M16.9 - `json`

Native JSON encode / decode mapping onto the tagged-union `Value`.
Promoted from the old Jennifer-coded plan (M18.3) to a Go system
library because JSON is foundational and performance-sensitive - a
char-by-char parser in `.j` pays interpreter overhead per byte.
Hand-rolled to stay TinyGo-clean (`encoding/json` is reflect-heavy,
the reason `astjson.go` is already hand-rolled).

- **Surface.** `json.encode(v) -> string`, `json.encodePretty(v) ->
  string` (2-space indent), `json.decode(s) -> Value`.
- **Value mapping.** `null` maps to null; `int` to an integer number
  and `float` to a number; `string`, `bool` map directly; `list` to
  array; `map of string to V` to object; struct to object (by field
  name); `bytes` to a base64 string. Non-string map keys and `task`
  values are encode errors.
- **Decode.** Produces generic Values (a number decodes to `int` when
  integral, else `float`; objects to `map of string to V`; arrays to
  `list of V`). **No map-to-struct coercion:** Jennifer does no coercion
  at binding boundaries (you write `5.0`, not `5`, for a float), so
  `json.decode` returns a map and a typed target is rebuilt explicitly -
  `def p as Point init Point{ x: $m["x"], y: $m["y"] };`. The encode
  direction is unaffected (`json.encode($p)` serializes a struct to an
  object directly). Rationale in
  [technical/rejected.md](technical/rejected.md).
- **Implementation.** Recursive-descent parser + emitter in
  `internal/lib/json`, no `reflect`, no `encoding/json`; RFC 8259,
  UTF-8, positioned errors (byte offset resolved to line / col within
  the JSON text). Decoded collections carry no recorded element type, so
  they are validated entry by entry against the declared `list` / `map`
  element type at the binding boundary (the same check a fresh literal
  gets): a homogeneous JSON collection binds to the matching typed
  target, a heterogeneous one matches no homogeneous type and is rejected
  - storing heterogeneous JSON awaits the planned `any` type.
- **Acceptance.** Round-trips every Value kind with a JSON image;
  malformed input yields a positioned error; both toolchains build.

## M16.10 - `uuid`

Generate and parse UUIDs. Small, self-contained, TinyGo-clean (16
bytes + version / variant bits + `8-4-4-4-12` hex).

- **Surface.** `uuid.generate(v) -> string`, where `v` is `"v4"`
  (random) or `"v7"` (time-ordered: 48-bit big-endian millisecond
  timestamp + random). The version tag is a **string argument**, not a
  `v4()` / `v7()` method - Jennifer identifiers are letters-only (no
  digits), so the variant lives in an argument, mirroring
  `hash.compute(b, "sha-256")`. Plus `uuid.parse(s) -> bytes` (16,
  validates format), `uuid.isValid(s) -> bool`, `uuid.version(s) -> int`
  (0 for NIL), constant `uuid.NIL`.
- **Random source.** Draws from `math`'s shared seedable RNG for now (v4
  predictability is documented); swaps to crypto-grade random when
  **M19 `crypto`** lands - a one-line change to `uuidlib.randByte`.
  `v7`'s timestamp is the wall clock (a package-local `nowFunc`, test-
  swappable).
- **Acceptance.** `generate("v4")` / `generate("v7")` are well-formed
  with correct version and variant bits; `parse` round-trips
  (case-insensitive); `generate("v7")` sorts by creation time; an
  unknown version tag errors; both toolchains build.

## M16.11 - `compress`

Byte-stream compression, `bytes` in / `bytes` out, plus streaming
handles for large data.

- **Not `encoding`.** `encoding` is the *representation* library:
  charset mapping and binary-to-text (hex / base64 / base32 /
  ascii85 / z85), reversible representations that don't reduce
  information (base64 even grows the data). Compression is
  entropy-based *size reduction* - a different operation with a
  streaming shape of its own. Go's stdlib draws the same line
  (`encoding/*` vs `compress/*`), which the routing mirrors.
- **Surface.** `compress.pack(b, algo [, level])` / `compress.unpack(b,
  algo)`, where `algo` is `"gzip"` / `"zlib"` / `"deflate"` and the
  optional `level` is `"fast"` / `"default"` / `"best"`. The algorithm is
  a string argument (matching `hash.compute` / `crc.compute`), and the
  `pack` / `unpack` verbs pair with `archive`'s. Streaming via the
  integer-handle pattern `hash` uses (`compress.stream(algo [, level])` /
  `update` / `finalize`).
- **Implementation.** Go `compress/flate` + `compress/gzip` +
  `compress/zlib` (pure Go; verify TinyGo-clean before relying on it).
- **Acceptance.** Round-trips each algorithm; `gzip` output is readable
  by `gzip(1)`; streaming matches one-shot; both toolchains build.

## M16.12 - `archive`

tar and zip container read / write over `bytes`, value-semantic so it
needs no `fs`.

- **Surface.** `archive.pack(entries, format) -> bytes` /
  `archive.unpack(b, format) -> list of Entry`, sharing the `pack` /
  `unpack` verbs with `compress` (byte streams there, file bundles
  here). `format` is `"tar"`, `"zip"`, or the gzip combo `"tar.gz"`
  (alias `"tgz"`), so the everyday `.tar.gz` case is one call rather
  than a `tar`-then-`compress` pair - `"tar.gz"` layers `compress`'s
  gzip ([M16.11](#m1611---compress)) over a tar internally. `Entry` is a
  struct `{ name, data as bytes, mode, mtime }`. An `fs`-integrated
  helper ("pack a directory") can layer on top later.
- **Implementation.** Go `archive/tar` (pure Go) + `archive/zip` (uses
  `compress/flate`, so depends on **M16.11**). No `fs` dependency in the
  core; the bytes-in/out shape keeps it TinyGo-safe.
- **Acceptance.** `pack` / `unpack` round-trip for tar, zip, and
  tar.gz; `tar(1)` / `unzip(1)` read the output; both toolchains build.

## M16.13 - `os.isTerminal`

A small `os` addition: is a given standard stream an interactive
terminal? The one host fact terminal styling needs - escape-code
generation is otherwise pure string work, so the styling itself lives in
the `ansi` module ([M17.5](#m175---ansi-first-reference-module)). Named
`isTerminal`, not the C-tradition `isatty`, to match the `is*` camelCase
family (`isAscii`, `isFile`, `isDir`).

- **Surface.** `os.isTerminal(stream) -> bool`, where `stream` is
  `"stdout"` / `"stderr"` / `"stdin"` (a string argument, like the rest
  of `os`). True only when that stream is an interactive terminal, not a
  pipe / file / CI.
- **Implementation.** The character-device mode bit (`os.File.Stat` +
  `os.ModeCharDevice`) - pure stdlib, so the library stays dependency-free
  (`golang.org/x/term` stays CLI-scoped) and TinyGo-clean. A pipe or a
  file redirect reports `false`, a terminal `true`; a stream that can't be
  stat'd reports `false`. Both binaries (on `jennifer-tiny`, an
  unstattable stream just falls through to the conservative `false`).
- **Acceptance.** `true` on an interactive TTY, `false` under a pipe /
  redirect; both toolchains build.

---

**Phase D: higher-level and Jennifer-coded libraries (M17-M20).**

## M16.14 - `any` type

The honest home for heterogeneous data. Jennifer's collections are
homogeneous (`list of T`, `map of K to V`) and every binding boundary is
strict (M16.9 closed the last hole - a generic literal / `json.decode`
result is now validated entry by entry, so a heterogeneous collection
matches no homogeneous type and can't be stored). That leaves genuine
"mixed shape" data - a JSON object with string, number, and array values;
a list of unrelated things - with nowhere to live. `any` is that place:
a type that every value matches.

- **Surface.** `any` is a type keyword, usable anywhere a type is:
  `def x as any;`, `def xs as list of any;`, `def m as map of string to
  any;`, a struct field (`def struct Row { cells as list of any };`),
  a parameter (`func f(v as any) {...}`). The zero value of `any` is
  `null`. Map *keys* stay concrete (they must be comparable and
  printable); `any` is a value-position type.
- **Semantics.** `any` matches every kind in `MatchesDeclared`, so any
  value binds to an `any` target and any value is a legal element of a
  `list of any` / `map of ... to any`. **The dynamism is explicit and
  one-directional:** a concrete value flows *into* `any` freely, but an
  `any` value flowing *out* into a concrete slot is a normal type
  mismatch and errors (strict at boundaries holds). You narrow an `any`
  with `convert.typeOf` and use it once its kind is known. No truthiness,
  no auto-coercion - reading an `any` gives a value of its real runtime
  kind, nothing more.
- **Type-system impact.** A `TypeAny` kind in `parser.Type`; the lexer /
  parser accept the `any` keyword in type position; `MatchesDeclared`
  gains a `TypeAny` arm returning true for every value; `ZeroFor(any)` is
  `null`; `stampDeclaredType` treats an `any` inner type as "leave the
  element's own type untouched" (an `any` collection records `any` as its
  element type and never rewrites element tags); `convert.typeOf` reports
  the value's actual kind, not `"any"`. Display and the AST-JSON emitter
  get the new kind.
- **`json` payoff.** `def m as map of string to any init json.decode(s);`
  becomes the honest way to hold an arbitrary decoded object;
  `$m["k"]` returns the real kind, narrowed with `typeOf`. The M16.9
  rebuild-into-a-struct idiom stays the way to get *static* types back.
- **Stance: explicit over implicit (#2).** `any` *appears* to violate the
  stance, but it serves it. The status quo it replaces is the *implicit*
  version - a `map of string to string` silently holding an `int`, type
  hidden. `any` makes the dynamism a written, greppable decision: you
  type `any` to say "the kind is decided at runtime," and you must narrow
  with `typeOf` before use - nothing important hides, and no value is
  silently coerced. It is the explicit escape hatch, not an implicit
  loosening, and it stays strict at boundaries (#4): `any`-into-concrete
  errors. A `design-decisions.md` reasoning record lands when it ships.
- **Rejected alternatives.** Implicit dynamic typing (a value with no
  declared type) - no; `any` must be written. Truthiness / auto-coercion
  of `any` at use sites - no; narrow explicitly. (Both already precluded
  by stances #2 and #4.)
- **Acceptance.** `def m as map of string to any init json.decode(s);`
  holds a heterogeneous object; each `$m[k]` reports its real kind via
  `typeOf`; an `any` value bound to a concrete-typed target errors;
  `def x as any;` zero-values to `null`; both toolchains build.

## M16.15 - explicit map-to-struct conversion

An explicit, validating way to turn a map - typically a `json.decode`
result held as `map of string to any` (M16.14) - into a typed struct. The
*implicit* form (`def p as Point init json.decode(s);` coercing at the
binding) is rejected; see
[technical/rejected.md](technical/rejected.md). This is the spelled-out
counterpart, so the conversion is visible at the call site. Depends on
M16.14 `any`, which is what lets an arbitrary decoded object be held
before conversion.

**Two candidate forms - not decided here; the choice hinges on the
consistency axis below.**

- **Library call - `convert.toStruct($map, "Point")`.** Routes into the
  existing `convert` type-conversion family; general (any map, not only
  decoded ones); the builtin looks the struct up in the registry and
  recurses into nested structs / lists.
- **Struct-literal spread - `Point{ ..$map }`.** Construction syntax that
  fills a `Point`'s fields from a map; the struct name is checked
  statically in literal position.

**The consistency question (decisive, not brevity).** The `convert.toX`
family is uniform: `convert.toInt(v)` / `toFloat(v)` / `toString(v)` /
`toBool(v)` each take a single value, name their target *in the function
name*, and return a self-contained, assignable value. `convert.toStruct`
cannot join that family cleanly:

- to stay self-contained (freely assignable, like the rest of `convert`)
  it needs the target as a **second, stringly-typed argument** (`"Point"`)
  - a two-arg outlier in a one-arg family, with the struct name unchecked
  until runtime; or
- to drop that argument it would have to read the **binding's declared
  type**, so `convert.toStruct($map)` would not be a self-contained
  expression (it wouldn't work outside a typed `def` / assignment) -
  unlike every other `convert.toX`, which *is* assignable anywhere.

The struct-literal spread sidesteps both problems (it isn't a `convert`
function and names its type statically), at the cost of new literal
syntax. We do **not** want a set of look-alike calls with divergent shapes
and arg orders - the trap PHP's array functions fell into - so this
uniformity axis is what decides the form.

- **Semantics (either form).** Strict, like every boundary: each declared
  field must be present in the map with a matching type; a missing field
  or a type mismatch is a positioned error. Recurses into nested structs,
  lists, and maps. Value semantics - the result is an independent struct;
  no partial fills, no defaults.
- **Open sub-questions (settle at implementation).** Whether an unexpected
  map key is an error or ignored; whether a partial spread
  (`Point{ x: 1, ..$map }`) is allowed; how an `any`-typed field is
  handled.
- **Acceptance.** A map / decoded object materializes a typed struct with
  full field validation; missing / mismatched fields (and, per the
  decision above, extra keys) error with a position; nested structs
  round-trip; both toolchains build.

## M16.16 - `net` TLS

Encrypted transport for `net`, so TLS-only protocols work (mail on
465 / 993 / 995, HTTPS). TLS is cryptographic and cannot be pure
Jennifer, so it lives in the system library - as a transport variant of
the existing socket, not a separate library.

- **Surface.** `net.connectTLS(host, port)` (implicit TLS, handshake on
  connect) and `net.startTLS(conn, host)` (upgrade a plaintext
  connection in place, for STARTTLS on 587 / 143 / 110). Both yield the
  same connection handle as `net.connect`, so `net.readBytes` /
  `writeBytes` / `close` / `address` work unchanged and callers stay
  transport-agnostic. `host` drives certificate / SNI verification.
- **Implementation.** Standard-Go `crypto/tls` over the existing TCP
  dial; part of `net`'s `!tinygo` build (`netlib_std.go`), stubbed on
  `jennifer-tiny` with the same friendly-error pattern as the rest of
  `net` (TinyGo 0.41 has no usable `crypto/tls` + netdev). Certificate
  verification is on by default; any skip-verify is a deliberate,
  explicitly-named argument, never the default.
- **Prerequisite for.** M18.2 `http` (HTTPS) and M18.6 `mail`.
- **Acceptance.** A handshake against a known-good endpoint round-trips
  bytes; `startTLS` upgrades an open connection; a bad certificate errors
  (positioned); the default binary builds and `jennifer-tiny` returns the
  friendly stub error.

## M16.17 - `encoding` quoted-printable

Add quoted-printable (RFC 2045) to `encoding`'s binary-to-text codecs,
the topical sibling of the base64 / hex already there. Needed by M18.6
`mail` (a MIME transfer encoding) and generally useful.

- **Surface.** A new format string for the existing verbs -
  `encoding.toText(b, "quoted-printable")` /
  `encoding.fromText(s, "quoted-printable")` - not a new function; it
  slots into the same dispatch as `"hex"` / `"base64"` / `"base64-url"`,
  under the existing codec-name normalisation (case, `-` / `_`).
- **Semantics.** Encode: printable ASCII stays literal; `=`, control,
  and 8-bit bytes become `=XX`; soft line breaks (`=` + CRLF) keep lines
  within the 76-column limit. Decode reverses it, tolerant of CRLF and
  LF soft breaks. Round-trips `bytes`.
- **Acceptance.** Canonical RFC 2045 vectors round-trip; the 76-column
  soft-wrap is applied on encode and unwound on decode; both toolchains
  build.

## M17 - Module system for Jennifer-coded libraries

Real modules so Jennifer-coded libraries get namespaces, scope, and
explicit exports. "Module" is the canonical term for a distributable
`.j` library (matching Python, ES2015, Rust). `include "x.j";` stays the
textual-splice form for composing one module out of several files;
`import "x.j" as x;` is the new cross-module boundary. Ships as four
dependency-ordered submilestones (M17.1 source tree and resolution,
M17.2 import statement and loader, M17.3 module scope and namespacing,
M17.4 exports and visibility); M18.x cannot start until all four land.

```jennifer
# points.j - a module
export def struct Point { x as int, y as int };

export func mid(a as Point, b as Point) {
    return Point{ x: ($a.x + $b.x) // 2, y: ($a.y + $b.y) // 2 };
}

func doubleCoord(v as int) { return $v * 2; }   # private: no export marker
```

```jennifer
# consumer.j
use io;
import "./points.j" as points;

def p as points.Point init points.Point{ x: 0, y: 10 };
io.printf("mid = %v\n", points.mid($p, points.Point{ x: 4, y: 6 }));
```

**Design basis** (the settled decisions the submilestones inherit;
turned-down alternatives live in
[rejected.md](technical/rejected.md)):

- `import` is a parser + interpreter feature, not a preprocessor splice
  like `include` (M17.2).
- Each module is its own resolution context: own `use` set, own
  namespace tables, own export table (M17.3).
- Modules are declarations-only, no mutable top-level state; `spawn`
  capture is unaffected because there is nothing mutable to capture
  (M17.3).
- One global `Error` (M13.2 stands); modules add distinctly-named error
  structs, never redefine `Error` (M17.3).
- Private by default; a leading `export` publishes a name. No
  `public`/`private` keyword (M17.4).
- Multi-file modules assemble via `include` behind one entry file; no
  directory-as-module, no cross-file re-export (M17.1).
- Modules need a filesystem; `jennifer-tiny` loads them where one
  exists, else fails with the normal search-path error (M17.1).

## M17.1 - Source tree and resolution

**Scope.** Where modules live on disk and how an import string resolves
to a file.

**Ships.**

- A top-level `modules/` directory for Jennifer-coded modules; system
  (Go-built) libraries stay in `internal/lib/*/`. Distro packaging ships
  modules to `/usr/share/jennifer/modules/` (FHS: read-only,
  arch-independent data), loadable without recompiling the interpreter.
- Three import shapes, classified by the *leading* token of the string:
  - `import "./f.j"` / `"../f.j"` - **local**: relative to the importing
    file's directory; no search path consulted.
  - `import "/abs/f.j"` - **absolute**: exactly that file; no search
    path.
  - `import "f.j"` - **module**: walk the search path (system module
    dir, then each `-I DIR` in order). The importing file's directory is
    never consulted.
- Subdirectories: a `/` anywhere but the leading position is an ordinary
  path component, so all three shapes accept `sub/f.j`.
- Multi-file modules: one entry file (`modules/bigmod/bigmod.j`)
  `include`s its parts (subdirs allowed); the splice is one module scope
  and one export surface; the consumer imports only the entry file.
- sysmoddir resolution: `--sysmoddir` > `JENNIFER_SYSMODDIR` >
  compile-time default (baked via the Makefile codegen path, since
  TinyGo ignores `-ldflags -X`). Inspectable through `meta.SYSMODDIR`
  (resolved after argv/env, not a static Install-time const) and
  `jennifer version -v` (each layer tagged `cli`/`env`/`compile`).

**Decisions.**

- Path shapes are disjoint at the character level so a reader tells
  "local file" from "vetted module" without leaving the line;
  working-directory-first search is a supply-chain footgun (rejected.md,
  implicit fallback chain). System modules win over `-I`; `-I` adds
  names, never overrides.
- Duplicate module names across `-I` dirs (or `-I` vs system) are a hard
  error at load, naming both paths.
- `--sysmoddir` / `JENNIFER_SYSMODDIR` are validated at Run() (missing
  or non-dir refuses to start); the compile-time default is best-effort,
  so a fresh checkout with no installed tree still runs scripts that
  import nothing.
- Bare form (`import "sub/bigmod.j";`, no `as`) reserves the file stem
  (`bigmod`) as the prefix.
- `../` in a local import is allowed - the supply-chain rule targets the
  consumer's working directory, not navigation within an
  author-controlled tree.
- No directory-as-module and no cross-file re-export (rejected.md); no
  package versioning / upgrade / signing (explicit non-goal: the distro
  or `-I` places module versions, the interpreter picks what it finds).
- TinyGo: resolution needs `fs` (present under TinyGo). Hosted
  `jennifer-tiny` loads modules; an FS-less host fails with the ordinary
  ordered-search-path error. A build-time `embed.FS` bundle is a
  deferred future option, not M17.

**Acceptance.** A module in `modules/bigmod/bigmod.j` that `include`s
`sub/extra.j` imports and runs under both `jennifer` and `jennifer-tiny`
on a hosted target via each of the three path shapes; `--sysmoddir`,
`JENNIFER_SYSMODDIR`, and the compile-time default resolve in that
precedence and surface identically through `meta.SYSMODDIR` and
`jennifer version -v`; duplicate `-I` names and a missing named
sysmoddir both error at Run().

## M17.2 - Import statement and loader

**Scope.** The `import "..." as NAME;` statement end to end: pipeline
placement, one-time init, ordering, cycles, error surfacing.

**Ships.**

- Preprocessor stops rejecting `import` and passes its tokens through
  (as it already does for `use`); `include` stays a preprocessor splice.
  The two keywords diverge cleanly by pipeline stage.
- Parser gains a `ModuleImportStmt` node (path + optional alias),
  distinct from the `use`-backed `ImportStmt`.
- Interpreter gains a module loader + cache. Loading a module lexes,
  preprocesses, parses, resolves, and executes it once against a fresh
  module scope, keyed by resolved absolute path.

**Decisions.**

- **Run-once.** A module inits exactly once per program run; later
  imports (any importer, any point) reuse the cached resolved AST and
  initialised namespace. Two aliases of one file share an instance; the
  same relative name from two directories are two modules (abspath key).
- **Init order is depth-first post-order.** A module fully inits before
  any module that imports it. Script imports A, A imports B, B imports C
  -> C, then B, then A, then the script body; each module's struct
  hoisting and `def const` initializers run once, on first reach.
- **Cycles are rejected; a module in a cycle never inits.** The loader
  tracks in-progress modules on the load stack; reaching an `import` of
  an in-progress module errors at load, before any initializer in the
  cycle runs, with `module cycle: A -> B -> C -> A` naming each edge. No
  Python-style partial init. Fix: factor a shared third module (Go's
  model). Mirrors the M10 include-cycle rejection.
- **Load-time errors are not catchable.** A `def const` initializer that
  throws during load fails the program at load, before the importer's
  body; `import` is a top-level declaration, not an expression, so
  `try`/`catch` cannot wrap it. Parse errors surface positioned in the
  imported file (path + line + col).
- `jennifer fmt` / `ast` / `tokens` preserve `import "..." as x;`
  textually, same as `include`.

**Acceptance.** A three-module chain inits in post-order exactly once
each (observable via a load-time `io.printf` in each `def const`
initializer); a re-import returns the cached instance without re-running
init; an A->B->C->A cycle errors at Run() naming every edge; a throwing
initializer fails at load and is not caught by a `try` around the
`import`; `jennifer fmt` round-trips an `import` line unchanged.

## M17.3 - Module scope and namespacing

**Scope.** What a module's top level may contain, how its names resolve,
and how it reaches other libraries and modules.

**Ships.**

- Per-module resolution context: each loaded module carries its own
  `nsPrefixes`, namespace tables, and private + export symbol tables in
  the module cache entry.
- Declarations-only top level. Exactly these forms; the three
  declaration forms take an optional leading `export`, the two import
  forms do not:
  - `def const NAME as TYPE init EXPR;` (export-able)
  - `def struct Name { ... };` (export-able)
  - `func name(...) { ... }` (export-able)
  - `use LIB [as ALIAS];`
  - `import "..." as NAME;`
- Consumer-side qualified resolution (`points.mid(...)`, `points.Point`)
  reuses the M10 namespacing machinery through the consumer's
  module-import table into the module's export table.

**Decisions.**

- **No mutable module state.** A mutable `def VAR ...;` at module top
  level errors, as does any free-standing top-level statement (bare
  expression, assignment, `if`/`while`/`for`/`repeat`). A `def const`
  initializer still runs once at load; "declarations-only" bounds
  statement forms, not initializer evaluation. Value-producing init is
  `export def const T init buildTable();` calling a private `func`.
- **`spawn` is unaffected.** With no mutable module state there is
  nothing new to capture; `snapshotForSpawn`'s two-frame model is
  unchanged and module constants (deep-immutable) are safe to reference.
- **`use` is not transitive.** A module's `use net;` gives the module
  `net.*`; the importer needs its own `use net;`. Prevents a module's
  implementation choices leaking into a consumer's namespace.
- **Bare type names resolve in the module's own type table.** Inside
  `points.j`, `func mid(a as Point, ...)` checks a value of identity
  `(points, Point)` against the module-local `Point`. A module names
  another module's type only through that module's prefix. Struct
  identity stays a `(namespace, name)` pair (M15.2).
- **One global `Error`.** M13.2's reserved `Error` is canonical across
  every module; user code never redefines it. Richer payloads are
  distinctly-named structs (`export def struct ParseError { ... };`);
  cross-module identity makes `a.ParseError` and `b.ParseError` distinct
  (rejected.md records the dropped per-module-`Error` idea).
- Scripts (run via `jennifer run`) keep top-level mutable `def` and
  free-standing statements - a script is a single execution context with
  no importer.

**Acceptance.** A mutable `def` or a free-standing statement at module
top level is a positioned parse error; a `def const` initializer runs
once at load; a module using `use net;` internally works while its
importer without `use net;` cannot call `net.*`; a struct made in a
module and passed back to a module `func` type-checks; a `spawn` body
calling a module `func` behaves identically to the serial call under
`-race`.

## M17.4 - Exports and visibility

**Scope.** Which top-level names cross the module boundary, and the
script-vs-module and test-overlay rules that follow.

**Ships.**

- `export` marker on top-level `def const` / `def struct` / `func`
  publishes the name; unmarked names are module-private. One marker, one
  direction.
- Referential-closure check: an exported struct field (or exported
  `func` parameter / return) whose type is a private struct errors at
  the export-annotation site.
- Cross-module struct identity `(module-prefix, name)` (extends M15.2).
- `MODULE_test.j` white-box test-overlay convention (pairs with M16.8
  `jennifer test`).

**Decisions.**

- **Private by default** (stance 2): the public API is the `export`-ed
  set, greppable in one pass; a forgotten marker stays internal (the
  fail-safe direction). No mandatory `public`/`private` keyword and no
  `public` synonym / standalone `private` marker (rejected.md).
- **Accessing a private name** from outside errors positioned at the
  call site (`foo.helper: 'helper' is not exported from module 'foo'`).
  No field-level visibility; an exported struct exports its whole shape.
- **Scripts vs modules by entry, not content.** `export` in a
  `jennifer run` script is a parse error ("script has no importers"); a
  module with zero exports loads fine and yields an empty namespace (any
  `NAME.x` at the importer errors as undefined). Promoting a script to a
  module is the deliberate "I now have a public API" moment.
- **Test overlays.** A co-located `MODULE_test.j` is spliced into
  `MODULE.j`'s scope *before* `parser.Resolve` runs over the combined
  file, so slot numbering covers both and the overlay reads private
  names by bare identifier. Black-box tests use `import "./MODULE.j";`
  instead. One overlay per module (Go's convention). Splicing after
  resolution would strand the overlay at `(-1, -1)` and force name-based
  fallback, so fold it in pre-resolve.

**Acceptance.** An unmarked helper is unreachable as `mod.helper` (with
the positioned error) while an `export`-ed name resolves; an exported
struct with a private-struct field errors at annotation; `export` in a
run script is a parse error; a module with no exports imports but every
`NAME.x` errors; a `MODULE_test.j` overlay reads `MODULE.j`'s private
names and runs under `jennifer test`. This submilestone touches parser
grammar (the `export` keyword), so it ships last; M17.1-M17.3 are
tooling, loading, and resolution.

## M17.5 - `ansi` (first reference module)

The first module built on the M17 module system - small, useful, pure
Jennifer, and a real dogfood of `import` / `export` / resolution.
Terminal styling as explicit string wrappers; escape-code generation is
pure string work, so no Go is needed.

- **Surface.** `ansi.color(s, name)` / `ansi.bgColor(s, name)`,
  `ansi.style(s, name)` (bold / dim / italic / underline / reverse),
  `ansi.rgb(s, r, g, b)` for truecolor, `ansi.strip(s)` to remove all
  escapes, plus convenience `ansi.red(s)` / `ansi.bold(s)` / ...
  shortcuts. `export`ed from a `modules/ansi.j`.
- **TTY-aware.** Gates styling on `os.isTerminal("stdout")`
  ([M16.13](#m1613---osisterminal)) so redirected output stays clean; a
  manual `ansi.enabled(flag)` override is available, and the module
  degrades to always-on if `os.isTerminal` is absent.
- **Why it is the reference module.** It exercises the whole module path
  end to end - a real `import`, `export`ed names, one system dependency
  (`use os;`) across the boundary - with code small enough to read in one
  sitting; a better M17 proof than a toy.
- **Not `printf` modifiers, not `io`.** Colour is a string wrapper, not
  I/O and not value formatting, so a `%s|color=` printf modifier is
  rejected in its favour ([technical/rejected.md](technical/rejected.md)
  entry when it lands).
- **Acceptance.** `import`s and resolves as a module; wrapping composes
  and nests; `strip` reverses it; styling suppresses when
  `os.isTerminal` is false.

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
- **M18.2 - `http`** (client) - atop `net`. HTTPS needs net TLS
  ([M16.16](#m1616---net-tls)).
- **M18.3 -** (`json` promoted to a Go system library, **M16.9** -
  foundational and performance-sensitive enough to earn native speed.)
- **M18.4 - `csv`** - simple, useful early.
- **M18.5 - `yaml`, `toml`, `xml`, `markdown`, `pretty`** - one or more
  sub-milestones depending on scope when planned. `toml` maps tables to
  `map`, arrays to `list`, and datetimes to `time.Time`; `.ini` is
  deferred as an optional tiny module only if demand surfaces (no real
  standard, ambiguous quoting / typing).
- **M18.6 - `mail`** - SMTP / IMAP / POP3 clients plus MIME (RFC 5322
  headers, multipart, 7bit / 8bit / quoted-printable / base64 transfer
  encodings). **Pure Jennifer**: the protocols are line-oriented
  command/response state machines and MIME is header + boundary
  structure - exactly the text orchestration a `.j` module does well -
  with the heavy lifting delegated to system libraries. Two system
  prerequisites, because neither can be pure Jennifer:
  - **net TLS ([M16.16](#m1616---net-tls), system).** Mail is almost
    always encrypted (implicit TLS on 465 / 993 / 995, STARTTLS on
    587 / 143 / 110), and TLS is cryptographic - it must be the host's
    (`net.connectTLS` / `net.startTLS`), never interpreted `.j`.
  - **quoted-printable in `encoding`
    ([M16.17](#m1617---encoding-quoted-printable), system).** The MIME
    transfer codec, alongside the base64 the module also leans on.
  With those in place the `mail` module stays pure Jennifer: connection
  dialogue, header parse/build, MIME-tree assembly and walk, and
  address / date formatting.
- **M18.7 - `redis`** - a Redis client over `net`. RESP2 framing
  (`+OK`, `$len`, `*count`, `:int`, `-ERR`) parses cleanly in `.j`;
  commands go out as RESP arrays. Plaintext first (trusted network /
  localhost); `rediss://` TLS is a later add via net TLS
  ([M16.16](#m1616---net-tls)). `AUTH [user] password` is a plain
  command, so no `crypto` dependency. Typed per-command helpers
  (`get -> string`, `incr -> int`, `lrange -> list`) keep the common
  path off `any`; a generic `command(...)` returning the raw reply is
  where `any` ([M16.14](#m1614---any-type)) helps. No hard prerequisites
  (just `net`), so it can land ahead of `mail`.
- **M18.8 - `memcache`** - a client for the `memcached` server's text
  protocol (`set` / `get` / `delete` / `incr` / `decr`; replies
  `STORED` / `VALUE ... END`) over `net`. Pure Jennifer, plaintext
  (memcached rarely uses TLS); values are `bytes` / `string`. Named
  `memcache` for the client / protocol; it talks to a `memcached`
  daemon.

(A Jennifer-coded `testing` module used to sit here as M18.5
- assertion vocabulary, `setUp` / `tearDown` orchestration,
`--filter` / `--format` / `--isolated` CLI. All of that moved
down into [M16.8](#m168---testing-framework-consolidation)
because the split was primitives-in-Jennifer atop
primitives-in-Go, spending implementation weight on
abstraction rather than capability. What remained after M16.8
- `assertApproxEqual($a, $b, $tol)`, `assertMatchesRegex`
composing on M16.3, table-driven test helpers - was
docs-section-sized rather than module-sized, so it lives as a
usage-patterns section in
[docs/libraries/testing.md](libraries/testing.md) instead of
as a numbered milestone. M18.6 renumbered up to M18.5.)

## M19 - `crypto`

Symmetric and asymmetric primitives, key derivation,
crypto-grade random. System library; TinyGo-safe primitives
only. Hashes already shipped in M15.6.

- **Swap `uuid`'s random source.** `uuid` (M16.10) draws its v4 /
  v7 randomness from `math`'s shared non-crypto RNG (seedable,
  predictable - documented). When crypto-grade random lands here,
  repoint `uuidlib.randByte` at it so `uuid.v4` is unguessable; the
  change is one function, no surface change. Until then `uuid` must
  not be used for security tokens.

## M20 - `httpd`

Pure-Jennifer HTTP server atop `net`. Ships as a module under
`modules/httpd.j` (same packaging shape as the M18 modules), not
baked into the interpreter. The point where Jennifer becomes
useful for serving content. Depends on **M16.0 concurrency**
(per-connection handlers run in `spawn` blocks) and M16.2 `net`
(the underlying TCP listener).

---

**Phase E: embedding, WASM, and specialised domains (M21+).** Not
committed to a timeline; recorded so the design doesn't foreclose
them. Ordered by the size of the structural change each one is:
M21's `internal/` -> `pkg/` restructure is a single-repo refactor;
M22's WASM runtime brings in a whole new dependency; M23.x/M24+
are indefinite-in-count library families.

## M21 - Public interpreter API for third-party embedding

Extract the interpreter core out from under `internal/` and
expose a documented Go-side surface so external programs can
embed Jennifer. Today `internal/interpreter`,
`internal/parser`, `internal/lexer`, and `internal/lib/*` are
unreachable from any module that isn't `github.com/mplx/jennifer-lang`
- Go's `internal/` visibility rule is not a convention, it's
a compile-time barrier. No submodule / require / replace
workaround exists; embedding is impossible without a
restructure.

Ships ahead of M22's WASM runtime because a Go-side embedding
API is a strictly smaller change (repository restructure, no
new external dependency), it unblocks the most immediate
embedding scenarios (scripting slot in a Go host, LSP /
formatter tooling, test harnesses), and it does not foreclose
M22 - a WASM plugin surface can layer on the same `pkg/`
facade once Wazero (or similar) is in play.

**Concretely.** Add a `pkg/` top-level (working name; the
final path settles at start of M21):

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
    for M21.

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
- A hosted no-`os` build target. Even after M21, the
  shipping stdlib libraries lean on Go's `os` / `net` /
  `time` packages. A truly bare-metal or `no-os` embedding
  can only use the pure-value libraries (`convert`, `math`,
  `strings`, `lists`, `maps`, `hash`, `crc`, `encoding`,
  `regex`) plus whatever host-provided shims the embedder
  wires up. That's a design constraint on the embedder, not
  a milestone on Jennifer's side.
- Semver freezing the public API. Jennifer stays pre-1.0
  through M21; the milestone documents what's exported and
  how libraries plug in, but breaking changes to that
  surface remain allowed until 1.0.0.

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

## M22 - WASM runtime embedding

Wazero or similar inside the interpreter binary. TinyGo-size
cost evaluated honestly before commitment. Without M22, no WASM
libraries.

## M23.x - WASM libraries

If M22 ships, sandboxed plugins via `use wasm:libname;`. Each
library a sub-milestone.

## M24+ - Specialised domains

Each domain its own milestone with sub-milestones as needed:

- **ML.**
  - **M24.1 - `stats`** - descriptive statistics over
    `list of int|float`: mean, median, mode, variance, stddev,
    percentile, min / max / sum, correlation. Pure-value,
    TinyGo-clean; the highest-value, simplest piece, so first.
  - **M24.2 - `linalg`** - vectors as `list of float` (dot, norm,
    cross, scale, add / sub) and matrices as `list of list of float`
    (matmul, transpose, determinant, inverse, solve, identity).
    Algorithms implemented directly - no `gonum`, too large a
    dependency. Matrices stay `list of list of float` for v1
    (idiomatic and value-semantic); a Go-backed matrix handle is the
    noted future escape hatch when big-matrix performance demands it.
  - **M24.3 - ML primitives** - atop `stats` / `linalg`, when demand
    surfaces.
- **Bioinformatics.** Sequence alignment (Smith-Waterman,
  Needleman-Wunsch), FASTA/FASTQ parsers, molecule structures.
- **Sandbox.** Restricted-capability execution.

Ordered when demand surfaces. WASM libraries (M23.x) may cover
some of this space first.

## M25+ - encoding long-tail codecs

The remaining `encoding` codecs, parked here so they're picked
up only when a real Jennifer program asks for them: the
single-byte character sets the original M15.7 spec listed, plus
a few binary-to-text additions. The character codec-table
infrastructure shipped with M15.7 - each new character-set entry
is just a 256-entry table plus its alias list.

- **Binary-to-text** (`toText` / `fromText`, joining today's
  `hex` / `base64` / `base64-url`): `base32` and `base32-hex`
  (RFC 4648), `ascii85` (Adobe / btoa, the `!`-`u` alphabet with
  `z` for an all-zero group), and `z85` (ZeroMQ Base85, the
  source-safe alphabet). `base32` and `ascii85` map onto Go's
  `encoding/base32` / `encoding/ascii85`; `z85` is a small
  hand-rolled table (no stdlib codec). Names normalise case and
  `-` / `_` like the existing entries.
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
- **Profiler: max-call-depth metric.** Have `jennifer profile` track
  Jennifer call depth (bump in `evalCall`, drop on return) and report
  the max reached, per source position and overall. Names stack-limit
  problems directly - the recursion-depth-vs-`-stack-size` headroom
  that the recursive `fib` in `examples/benchmark.j` exercises on
  `jennifer-tiny`. Small and additive to the existing hit-count /
  wall-clock / `--allocs` collector; deferred because stack limits are
  diagnosable by hand today. Heap-per-position stays out of scope
  (`--allocs` already proxies copy churn; true RSS needs
  `runtime.ReadMemStats` sampling, coarse under TinyGo).
- **`tinygo_devtools` build tag.** The dev subcommands (`tokens` /
  `ast` / `fmt` / `lint` / `profile` / `test`) are `!tinygo` for binary
  *size*, not compatibility - they are TinyGo-clean Go. A
  `//go:build !tinygo || tinygo_devtools` constraint (stub as
  `tinygo && !tinygo_devtools`) plus a `make build-tinygo-dev` target
  would let them run under the actual TinyGo runtime - e.g. to
  `profile` a TinyGo-specific perf or stack issue in situ. Pairs with
  the depth metric above: together they are "TinyGo runtime
  introspection." Deferred - build-tag complexity across ~6 files and a
  larger dev-tiny binary, for a diagnostic reached for only
  occasionally.
- **Build-time library selection.** Choose which system (Go) libraries
  are baked into a binary at compile time. Motivated by `jennifer-tiny`
  size (an embedded target needing only `io` + `math` shouldn't carry
  `net` / `regex` / `hash`) and by opt-in niche Go libraries that don't
  merit defaulting. The install point is already consolidated - every
  entry path (`run` / `repl` / `profile` / `test` and the test
  harnesses) calls `internal/stdlib.InstallAll`, so a library is one
  line there - and that is the seam a build-tag scheme would cut along:
  gate each entry behind `//go:build lib_net` (or a `minimal` / `full`
  profile) and grow `make build-minimal` / `make build TAGS=...`,
  exactly like the existing `!tinygo` dev-tool split. **Compile-time
  only** - Go's `plugin` package is Linux/macOS-only and unsupported by
  TinyGo, and dynamic linking contradicts `jennifer-tiny`'s
  no-hosted-runtime goal, so PHP-style loadable `.so` extensions are
  out. Two caveats to design for: (1) a trimmed build breaks the "any
  `.j` runs on any binary" portability promise (`use net;` becomes a
  runtime error), so the default build stays full and trimmed builds are
  an explicit opt-out - ideally with a `meta`-level "is library X
  present?" query for graceful degradation; (2) CI grows a couple of
  profiles (default / minimal), not 2^N. Complementary to, not a
  substitute for, the M17 module system: `.j`-level extensibility
  (community / uncommon libraries writable in Jennifer) is M17's job with
  zero binary cost; build-time selection is only for the curated
- **Relational databases (`sql`).** Postponed to M20+, pending a design
  discussion. A system library - a storage engine and SQL planner can't
  be interpreted, and wire-protocol auth needs crypto - driver-agnostic
  over Go's `database/sql`, SQLite first, then MySQL / PostgreSQL;
  standard-Go only, stubbed on `jennifer-tiny`. The questions that gate
  it: the SQLite driver (cgo `mattn/go-sqlite3`, which breaks static /
  cross-compile / TinyGo, vs pure-Go `modernc.org/sqlite`, multi-MB);
  whether to accept the first heavyweight dependency in the library
  layer (a break from "libraries stay dependency-free") and, if so, gate
  it as a build-tag opt-in (per the note above); and the result-row
  shape - `map of string to any` (a concrete motivator for `any`,
  M16.14) vs a typed struct via map-to-struct (M16.15). Values bind only
  through `?` placeholders (injection safety). Contrast the
  text-protocol stores redis / memcache (M18.7 / M18.8), which are pure
  Jennifer over `net` and need none of this.
  Go-level core.
- **`io.lines() -> list of string`.** Slurp the whole stdin into a
  list. Additive on top of the streaming `readLine()` + `eof()`
  idiom; nice-to-have for tiny scripts, not blocking. Deferred from
  M7.
- **i18n.** Locale-aware case folding, collation, number / date
  formatting, BiDi. Gated on the CLDR-data binary-size question
  (likely an optional library shipped after the M19 WASM runtime
  so locale tables aren't baked into every build).
- **Host-embedding API.** *Promoted to a numbered milestone -
  see [M21](#m21---public-interpreter-api-for-third-party-embedding).*
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
  Best landed post-M22 once the language is settled and the
  interpreter doesn't churn under it.
