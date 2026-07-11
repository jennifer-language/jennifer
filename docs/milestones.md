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
  in M20.1 `crypto`. The originally planned M14.2 `random`
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
`time.Zone {offset, name}` (fields public, so an IANA / DST
companion can build them). Granularity (date-only
vs time-of-day-only) is a property of formatting, not the value
type. Unix timestamps are constructor / accessor pairs, not a
separate type. IANA names and DST are out of the fixed-offset
core - a Go-backed `time`-library extension, not a `.j` data map
(see the Long-horizon "`time`: IANA / DST zones" entry). Three
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
`"iso-8859-1"`, `"windows-1252"`, `"ebcdic"`. The spec's per-format
verbs (`encoding.hex`, `encoding.base64`, ...) consolidated into
the codec-table pair to dodge the same digit-in-identifier rule
M15.6 hit. Codec names and format strings are exact-match (the one
canonical spelling only; the original alias / case normalisation was
later dropped as a strictness lift, stance #2).
Windows-1252's five canonically-undefined positions (0x81, 0x8D,
0x8F, 0x90, 0x9D) reject symmetrically on encode and decode.
Long-tail single-byte codecs (ISO-8859-{2..16},
Windows-{1250,1251,1253..1258}) later shipped in
[M16.15](#m1615---encoding-completion), generated from the Unicode
mapping files. See
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

## M16 - I/O libraries and developer tooling

**Status:** done. Phase C: system libraries that touch the OS or do
significant compute, opened by a concurrency primitive (M16.0) that the
I/O libraries build on, then a developer-tooling trio (lint / profile /
test) and a run of self-contained data libraries. All sub-milestones
shipped; git history holds the full per-milestone specs.

### M16.0 - Lightweight concurrency

**Done.** `spawn { ... }` (block primary expression), `task of T` (new
compound kind), and the `task` library (`wait` / `poll` / `discard` /
`waitAll` / `waitAny`). Goroutine-backed but data-race-free by
construction: `snapshotForSpawn` deep-copies a two-frame globals+locals
snapshot at launch (tasks are the one carve-out from value semantics -
copies share the `TaskState` pointer). A per-run registry loud-fails
unobserved task errors at exit and bumps the exit code (a non-terminating
undiscarded spawn hangs at exit - the documented trade-off). `task.wait`
re-raises a body error at the wait site for `try`/`catch`; `waitAny` is
the runtime's only `reflect.Select`. The Makefile passes
`-stack-size=2mb -scheduler=tasks` to TinyGo. See
[concurrency.md](user-guide/concurrency.md), [task.md](libraries/task.md).

### M16.1 - `fs`

**Done.** Blocking filesystem I/O composed with `spawn` (no `*Async`
variants): whole-file `read`/`write`/`append` (String/Bytes), metadata
(`exists`/`isFile`/`isDir`/`stat` -> `fs.Stat`), directory ops with the
two-verb recursion split (`mkdir`/`mkdirAll`, `remove`/`removeAll`, plus
`rename`/`list`/`walk`), and buffered `fs.File` handles
(`open`/`readLine`/.../`close`; `eof` peeks one byte). Path- vs
handle-form verbs dispatch on first-arg kind; `fs.File` shares state
across copies (the handle carve-out from value semantics). See
[fs.md](libraries/fs.md).

### M16.2 - `net`

**Done.** TCP (`connect`/`listen`/`accept`/`readBytes`/`writeBytes`),
UDP (`listenUDP`/`sendTo`/`recvFrom`), and DNS
(`lookup`/`reverseLookup`); polymorphic `close`/`address` over three
handle registries. Blocking calls compose with `spawn`
(accept-loop-per-connection). Build-tag split: `jennifer-tiny` returns
friendly-error stubs (no netdev in TinyGo). See [net.md](libraries/net.md).

### M16.3 - `regex`

**Done.** RE2 (Go `regexp`, linear-time) over strings:
`matches`/`find`/`findAll`/`replace`/`split`/`escape`;
`regex.Match{text,start,end,groups,groupsNamed}` with rune indices and a
`start=-1` no-match sentinel; an implicit 128-entry LRU pattern cache.
Full surface in both binaries. See [regex.md](libraries/regex.md).

### M16.4 - `testing` (system-library primitives)

**Done.** The irreducible system-side surface a `.j` test framework needs:
`testing.run(name)` invokes a zero-arg user method via the new
`Interpreter.CallByName`, times it, and classifies every failure mode
into a `testing.Result`; `results`/`reset` manage a mutex-guarded
accumulator; `report` renders text / TAP / JUnit. The one place `exit` is
caught (at the Go level, so language `try`/`catch` still cannot). See
[testing.md](libraries/testing.md).

### M16.5 - Interpreter performance pass

**Done.** Five sub-milestones, user-visible behaviour unchanged: **.1**
shared-marker copy-on-write on compound Values (append-in-a-loop O(N^2)
-> amortised O(N)); **.2** parse-time lexical slot resolution
((Depth,Slot) coordinates + a `slots` slice, undefined/shadowing promoted
to parse-time errors); **.3** pooled + pre-resolved + slot-bound
method-call frames; **.4** namespaced-call / comparison / arg-bind /
root-cache fast paths; **.5** compile-time constant folding plus a
`Share()` scalar fast path. Numbers in [tinygo.md](technical/tinygo.md).

### M16.6 - Developer tooling: linting

**Done.** `jennifer lint` flags compile-legal-but-suspect patterns:
grouped IDs (**L0nn** source errors, always on / **L1nn** correctness /
**L2nn** style / **L3nn** lifecycle), `# lint-disable[-file]: IDS`
suppression, `--checks` / `.jennifer-lint` config, and human / JSON /
GitHub output (source errors render in the chosen format, so a JSON
pipeline stays valid); exit 0/1/2. `!tinygo`. See
[cli_lint.md](technical/cli_lint.md).

### M16.7 - Developer tooling: profiling

**Done.** `jennifer profile` attributes work to `.j` source positions
(what `go tool pprof` cannot): a statement profile (hit count +
self/cumulative wall-clock) and an `--allocs` value-copy profile; table /
pprof (hand-encoded gzipped protobuf) / Chrome-trace output; program
output goes to stderr so the profile owns stdout. `!tinygo`. See
[cli_profile.md](technical/cli_profile.md).

### M16.8 - Testing framework consolidation

**Done.** An assertion vocabulary on M16.4's primitives (`assertEqual`
... `assertThrows`, throwing `Error{kind:"assertion"}` at the call site),
`CallByNameWith`/`runWith` argument dispatch, and the `jennifer test`
subcommand (`test*` discovery or `--filter`, `setUp`/`tearDown`,
`--isolated` per-test subprocess, text/TAP/JUnit, exit 0/1/2). Builtins
can now raise a catchable Jennifer error via `interpreter.RaiseError`. See
[cli_test.md](technical/cli_test.md).

### M16.9 - `json`

**Done.** Hand-rolled RFC 8259 encode/decode onto the tagged-union Value
(no `encoding/json`, no reflect). `encode`/`encodePretty`/`decode`;
structs and `map of string to V` map to objects, `bytes` to base64,
numbers to `int` when integral else `float`. Also closed a type hole: a
generic collection (a fresh literal or decode result) is validated entry
by entry against the declared element type at every binding boundary.
Decode's return shape was later superseded by
[M16.16](#m1616---jsonvalue). See [json.md](libraries/json.md).

### M16.10 - `uuid`

**Done.** RFC 9562: `uuid.generate("v4")` (random) / `generate("v7")`
(time-ordered), the version tag a string argument (identifiers are
letters-only), plus `parse`/`isValid`/`version` and constant `NIL`.
Randomness draws from `math`'s shared seedable RNG (documented
non-crypto; swaps when M20.1 `crypto` lands). See [uuid.md](libraries/uuid.md).

### M16.11 - `compress`

**Done.** Byte-stream size reduction (distinct from `encoding`'s
representation codecs): `pack`/`unpack` for `"gzip"`/`"zlib"`/`"deflate"`
with an optional `"fast"`/`"default"`/`"best"` level, plus a streaming
`compress.Stream` handle. Go `compress/*`, TinyGo-clean. See
[compress.md](libraries/compress.md).

### M16.12 - `archive`

**Done.** tar / zip containers over `bytes` (no `fs`, value-semantic):
`pack`/`unpack` (verbs shared with `compress`) for
`"tar"`/`"zip"`/`"tar.gz"`; a bundle is a
`list of archive.Entry{name,data,mode,mtime}`. Go
`archive/tar`+`archive/zip`. See [archive.md](libraries/archive.md).

### M16.13 - `os.isTerminal`

**Done.** `os.isTerminal(stream)` (`"stdout"`/`"stderr"`/`"stdin"`) ->
bool, the gate for ANSI colour, via the character-device mode bit
(`os.ModeCharDevice`) - pure stdlib (keeps `x/term` CLI-scoped),
TinyGo-clean; an unstattable stream reports `false`. See
[os.md](libraries/os.md).

### M16.14 - `net` TLS

**Done.** `net.connectTLS(address)` (implicit TLS) and
`net.startTLS(conn)` (in-place STARTTLS upgrade), both yielding the
transport-agnostic `net.Conn`. Certificate verification is on by default,
with a `net.TLSOptions{skipVerify as bool, caCert as bytes}` opt-out. Go
`crypto/tls` on the `!tinygo` build (stubbed on `jennifer-tiny`). See
[net.md](libraries/net.md).

### M16.15 - `encoding` completion

**Done.** Filled out `encoding`: `toText`/`fromText` gained
`quoted-printable`, `base32`/`base32-hex`, `ascii85`, and `z85`; the full
ISO-8859-{1..16} / Windows-{1250..1258} single-byte codecs, generated
from the Unicode mapping files (`gen_codecs.go` -> `codecs_gen.go`) so
only `ascii`/`ebcdic` stay hand-written. Format and codec names are
exact-match (the normalisation layer was dropped as a strictness lift,
stance #2). See [encoding.md](libraries/encoding.md).

### M16.16 - `json.Value`

**Done.** The strict home for heterogeneous JSON without a language top
type: `json.decode` returns an opaque `json.Value` - the first
`KindObject` (the opaque sibling of `KindStruct`: discriminated by
`(namespace, name)`, minted only by a library, rejecting operators /
`[i]` / `.field`). `convert.typeOf` reports `"object"`,
`convert.objectType` the specific `"json.Value"`. Reads and non-mutating
writes share JSON Pointer (RFC 6901) addressing -
`typeOf`/`get`/`has`/`keys`/`length`/`as*`/`isNull` and
`map`/`list`/`set`/`insert`/`append`/`remove`/`move` (strict / no-vivify,
`-` end-marker), with node types in `list`/`map` vocabulary - and a
displayer hook renders a handle as its JSON. `json.decode`'s return type
changed (a pre-1.0 break) and the decoder's number grammar was tightened
to json.org. No `any` keyword (rationale in
[rejected.md](technical/rejected.md)). See [json.md](libraries/json.md).

---

**Phase D: higher-level and Jennifer-coded libraries (M17-M20).**

## M17 - Module system for Jennifer-coded libraries

**Status:** done. All six sub-milestones shipped. Jennifer-coded libraries
now get their own namespace, scope, and explicit `export`s via a real
**module boundary** (`import "x.j" as x;`, parser + interpreter) that lives
beside the textual `include "x.j";` splice (preprocessor, for composing one
module out of several files). The settled, cross-cutting decisions
(turned-down alternatives in [rejected.md](technical/rejected.md)): `import`
is a parser + interpreter feature, not a preprocessor splice; each module is
its own resolution context (own `use` set, namespace + export tables); the
module top level is **declarations-only** - no mutable module state, so
`spawn` capture is unaffected; the one global `Error` stands (modules add
distinctly-named error structs, never redefine it); **private by default**,
a leading `export` publishes a name (no `public` / `private` keyword);
multi-file modules assemble via `include` behind one entry file (no
directory-as-module, no cross-file re-export); modules need a filesystem
(an FS-less `jennifer-tiny` host fails with the ordinary search-path error).
User-facing reference: [imports.md](user-guide/imports.md) +
[modules/index.md](modules/index.md). M18.x builds on top.

### M17.1 - Source tree and resolution

**Done.** Path resolution in `internal/module`: `Classify` + `Resolve` map
an import path (local `./` / `../`, absolute `/`, or a bare name walked on
the search path) to a canonical absolute path, rejecting a name found in
two search dirs and a not-found. The system module directory resolves
`--sysmoddir` > `JENNIFER_SYSMODDIR` > compile-time default (surfaced as
`meta.SYSMODDIR`; a named CLI / env dir that is missing or not a directory
refuses to start, the compile default is best-effort), and `-I DIR`
(repeatable) appends to the search path after it. `jennifer version -v`
reports the resolved layers. See [cli.md](technical/cli.md).

### M17.2 - Import statement and loader

**Done.** `import "path.j" [as NAME];` is a real statement
(`ModuleImportStmt`) - the preprocessor passes it through, the parser
builds the node. The loader (`internal/interpreter/module.go`) runs each
module in a fresh sub-interpreter sharing one `moduleReg` (run-once cache
by canonical path, in-progress load stack, search path), so **run-once**,
**depth-first post-order init**, and **cycle detection** (erroring with
every edge named) all fall out of the recursion. Load-time errors (a parse
error or a throwing `def const` initializer) fail the program and are not
catchable - an `import` is a declaration, not an expression, so it cannot
sit in a `try` / block. `jennifer fmt` / `ast` / `tokens` round-trip an
`import` line. See [imports.md](user-guide/imports.md).

### M17.3 - Module scope and namespacing

**Done.** A module top level is **declarations-only**
(`checkModuleDeclarationsOnly`: only `def const` / `def struct` / `func` /
`use` / `import`; a mutable `def` or free-standing statement is a
positioned load-time error - scripts keep both). `loadModuleImports` binds
each alias (the `as NAME`, or the file stem) into `moduleAliases`,
collision-checked against library prefixes. Consumer resolution rides the
qualified-reference eval layer: `evalQualifiedCall` / `evalQualifiedConst`
dispatch `alias.fn(args)` into the module's own interpreter via
`CallByNameWith` (arguments evaluated in the consumer, body run against the
module's globals + methods) and read `alias.CONST` from its scope. `use`
non-transitivity, run-once sharing, and `-race` safety all follow from the
fresh-sub-interpreter-per-module model - a module holds only immutable
constants and read-only methods. See [interpreter.md](technical/interpreter.md).

### M17.4 - Exports and visibility

**Done.** `export` (a keyword) publishes a top-level `func` / `def struct` /
`def const`; unmarked names stay private (reaching `mod.helper` from
outside is a positioned "not exported from module" error), and `export` in
a `jennifer run` script is rejected (module vs script by entry, via the
`isModule` flag). `checkReferentialClosure` rejects an exported struct
field or exported function parameter typed as a *private* module struct;
library / namespaced types cross freely. **Cross-module struct identity**
is boundary translation (`retagStructs`): a module's structs are bare
inside it and re-tagged to `(module-stem, name)` as a value crosses out to
an importer and back on the way in, so `def p as mod.Point`,
`mod.Point{...}`, field reads, and pass-back all type-check while `a.Point`
and `b.Point` stay distinct. The retag walks values *and* the collection
type tags a `list` / `map` carries (`retagType`), so a
`list of mod.Point` handed back into a module reads as its bare
`list of Point` parameter. A co-located `MODULE_test.j` overlay (a token
splice in `jennifer test`) runs white-box tests against the module's
private names.

### M17.5 - `ansi` module

**Done.** `modules/ansi.j` - terminal styling as explicit string wrappers,
the first module built on the system (pure Jennifer, one `use os;`
dependency across the boundary; a real end-to-end dogfood of `import` /
`export` / resolution). Exports `color` / `bgColor` / `style` (bold / dim /
italic / underline / reverse) / `rgb` truecolor / `strip`, plus per-colour
and per-style shortcuts (`ansi.red(s)`, `ansi.bold(s)`, ...). The ESC byte
is built from a one-byte `bytes` (no string-literal escape for it); `strip`
uses `regex`; unknown colour / style names `throw`. Stateless and
TTY-aware: `enabled()` re-reads `NO_COLOR` / `FORCE_COLOR` /
`os.isTerminal("stdout")` per call, so redirected output stays clean and no
toggle state is stored (honours the no-mutable-state rule); degrades to
always-on when `os.isTerminal` is absent. Colour is a string wrapper, so a
`%s|color=` printf modifier is rejected in its favour
([rejected.md](technical/rejected.md)). Demo
`examples/modules/ansi_demo.j`; coverage
`internal/interpreter/module_ansi_test.go` + a white-box
`modules/ansi_test.j` overlay reading ansi's private tables; reference doc
[modules/ansi.md](modules/ansi.md).

### M17.6 - `semver` module

**Done.** `modules/semver.j` - strict [SemVer 2.0.0](https://semver.org) as
a second pure-Jennifer reference module (no system dependency beyond
`use strings; use convert; use regex;`, so it runs on both binaries), and
the foundation the future `jvc` package manager
([Long horizon](#long-horizon-recorded-not-scheduled)) needs since
`install gotify>=1.0.0` is semver comparison at its core. Exports
`Version { major, minor, patch, prerelease, build }` plus `parse` (throws
on invalid) / `isValid` / `toString` (round-trips `parse`); `compare` /
`lt` / `eq` / `gt` (SemVer precedence: numeric core, a prerelease ranks
*below* its release, prerelease fields compared numeric-below-alphanumeric,
build metadata ignored); `isStable` (no prerelease and `major >= 1`;
`0.y.z` is unstable by convention) / `isPrerelease`; `incMajor` / `incMinor`
/ `incPatch` (reset the lower parts, clear pre + build); and `sort` (own
pass over `compare`, since `lists.sort` is scalar-only). Strict, not a loose
parser: a looser N-segment form (`1.2.3.4`) has no defined ordering and is
rejected. `parse` matches the canonical anchored SemVer RE2 pattern with
named groups (`regex.find` + `groupsNamed`); the precedence comparison and
sort are hand-written Jennifer - the algorithmic dogfood. `meta.VERSION` is
valid strict semver, so the module parses Jennifer's own version. Range /
constraint matching (`^1.2.0`, `>=1.0.0`, `~1.2.3`) is the harder parser and
is deferred to (or just before) `jvc`. Building the demo surfaced and fixed
a latent M17.4 boundary gap: passing a consumer-typed `list of semver.Version`
back into a module `list of Version` parameter (`semver.sort`) failed because
the retag re-tagged the struct element *values* but not the list's
element-*type* metadata - now covered by `retagType` (regression
`TestModuleListOfStructCrossesBoundary`). Demo
`examples/modules/semver_demo.j`; white-box `modules/semver_test.j` overlay
(12 tests); reference doc [modules/semver.md](modules/semver.md).

## M18.x - Jennifer-coded modules

Built atop the existing system libraries. Each one ships as a Jennifer
**module** under `modules/` (the directory introduced in M17); none of
them are compiled into the interpreter binary. Sub-milestones in priority
order.

### M18.1 - `csv` module

**Done.** RFC 4180 comma-separated values as a pure-Jennifer module
(`modules/csv.j`) over `strings` and `maps`. A quoting-aware hand-written
scanner: `parse` / `format` for standard comma-delimited text and
`parseWith` / `formatWith` for any single-character delimiter (TSV and
friends), quoting a field only when it carries the delimiter, a quote, or a
line break and doubling embedded quotes; records split on `LF` / `CRLF` /
bare `CR` and re-emit with `LF`. `toRecords` / `fromRecords` pair a header
row with data rows as header-keyed `map of string to string` records. Type
inference and streaming are out of scope (every field is a `string`; the
caller converts and slurps). Reference doc
[docs/modules/csv.md](modules/csv.md); overlay `modules/csv_test.j` (100%);
demo `examples/modules/csv_demo.j`.

### M18.2 - `htmlwriter` module

**Done.** An HTML builder / serializer as a pure-Jennifer module
(`modules/htmlwriter.j`) over `strings` and `lists`, imported `as html`.
An element tree of `Node` values - `element(tag, attrs, children)` /
`text(s)` / `raw(s)` constructors plus `attr(name, value)` - renders through
`render` / `renderAll` to correctly-escaped HTML5: text nodes escape
`&` / `<` / `>`, attribute values also escape `"`, `raw` passes verbatim,
and the void elements (`br` / `img` / ...) emit no closing tag and drop
children. The public `escape` exposes the text-context escaper. A *writer*,
not a parser: no dependency on the system `xml` library (M20.2) -
serialization is string orchestration - and named `htmlwriter` (not `html`)
to leave room for a separate HTML parser later. Reference doc
[docs/modules/htmlwriter.md](modules/htmlwriter.md); overlay
`modules/htmlwriter_test.j` (100%); demo
`examples/modules/htmlwriter_demo.j`.

Building it surfaced and fixed a module-boundary bug: a declared
`list of alias.Struct` (or `map of K to alias.Struct`) whose importer alias
differs from the module file stem left the element type tagged with the
alias while values crossed the boundary tagged with the stem, so appending a
module value into the list failed the element-type check. Declared-type
namespace resolution now recurses into list / map / task element types and
is idempotent across re-execution (a `def` inside a loop). The same fix
covers aliased library structs in collections (`use os as o; def xs as list
of o.Result`).

### M18.3 - `markdown` module

**Done.** A lightweight Markdown renderer (`modules/markdown.j`) for a small
CommonMark subset - ATX headings, bold / italic emphasis, inline code,
links, fenced code blocks, ordered / unordered lists, and GFM tables - with
two output targets: `toHtml` renders through the `htmlwriter` module (M18.2)
(tables to `<table>` with per-column alignment, ANSI tables as aligned
terminal columns), so
escaping is automatic and the markup can't be malformed, and `toAnsi`
renders through the `ansi` module (self-suppressing off a TTY). The inverse
authoring helpers (`header` / `style` / `link` / `bullets` / `numbered` /
`codeBlock`, plus `table` for GFM tables) build Markdown text, so a program
can assemble and round-trip a document, and `tablePretty` reformats
handcrafted table source so its columns line up. Line-oriented
block parsing (a `lineType` classifier plus per-kind handlers) over a small
non-nesting inline scanner; documents are small, so per-line interpreter
overhead is a non-issue. It is the first module that imports sibling modules,
which surfaced that `jennifer test` did not enable the module system - now it
does (local imports resolve relative to the test file, bare names through the
default system module dir), so a module that imports other modules is
testable through its overlay. Deliberately not full CommonMark: inline spans
do not nest, and blockquotes / thematic breaks / images / reference links are
out. Reference doc [docs/modules/markdown.md](modules/markdown.md);
overlay `modules/markdown_test.j` (100%); demo
`examples/modules/markdown_demo.j`.

### M18.4 - mail suite (SMTP / POP3 / IMAP + MIME)

SMTP / IMAP / POP3 clients plus MIME (RFC 5322 headers, multipart, 7bit /
8bit / quoted-printable / base64 transfer encodings). **Pure Jennifer**:
the protocols are line-oriented command/response state machines and MIME
is header + boundary structure - exactly the text orchestration a `.j`
module does well - with the heavy lifting delegated to system libraries.
Two system prerequisites, because neither can be pure Jennifer:

- **net TLS ([M16.14](#m1614---net-tls), system).** Mail is almost always
  encrypted (implicit TLS on 465 / 993 / 995, STARTTLS on 587 / 143 /
  110), and TLS is cryptographic - it must be the host's
  (`net.connectTLS` / `net.startTLS`), never interpreted `.j`.
- **quoted-printable in `encoding`
  ([M16.15](#m1615---encoding-completion), system).** The MIME transfer
  codec, alongside the base64 the module also leans on.

Split into sub-milestones, MIME first: it is the reusable message-structure
foundation the protocol clients build on, and unlike them it is pure text,
so it is 100% overlay-testable. The network clients depend on `net` (so
they are default-binary-only; `jennifer-tiny` stubs `net`), and their
round-trips cannot run in the offline CI overlay - they are verified by a
demo / manual send-and-fetch against a real server, with the protocol logic
(command build, response parse) unit-tested offline.

- **SASL auth, ordering-aware.** SMTP AUTH / IMAP / POP3 login is SASL,
  which is pure message orchestration (a good `.j` fit, not a crypto
  primitive). The crypto-free mechanisms - `PLAIN`, `LOGIN`, `XOAUTH2`
  (base64 only, already in `encoding`) - ship with the clients and cover
  most real-world mail. The challenge-response mechanisms (`SCRAM-SHA-256`,
  `CRAM-MD5`) need HMAC / PBKDF2 from **M20.1 `crypto`**, so they land with
  / after M20.1, factored into a small shared `sasl` `.j` module a later
  LDAP client (M24+) reuses. SASL / SCRAM is a *consumer* of crypto
  primitives, never part of M20.1 itself.

### M18.4.1 - `mime` module

**Done.** Build and parse MIME messages as a pure-Jennifer module
(`modules/mime.j`) over `strings` / `convert` / `encoding` - no networking,
TinyGo-clean, so it runs on both binaries. A `Part` is a leaf (headers plus
a decoded-text `body` with a transfer `encoding`) or a multipart container
(headers plus child `parts` under a `boundary`). Build with `text` /
`attachment` / `multipart` / `withHeader`, serialize with `encode` (CRLF
lines; 7bit when ASCII, quoted-printable for non-ASCII text, base64 wrapped
at 76 columns for attachments), parse back with `parse` (header unfolding,
recursive boundary split, transfer-decode), and read with `headerValue` /
`body` / `parts` / `contentType`; `address` formats an RFC 5322 mailbox.
Bodies are held decoded as text: binary (`bytes`) bodies and RFC 2047
encoded-words for non-ASCII headers are deferred. Reference doc
[docs/modules/mime.md](modules/mime.md); overlay `modules/mime_test.j`
(100%); demo `examples/modules/mime_demo.j`.

### M18.4.2 - `smtp` (send)

**Done.** SMTP send client (`modules/smtp.j`) over `net`: `smtp.send(opts,
from, recipients, message)` runs the RFC 5321 dialogue - connect per
`Options.security` (`"none"` / `"starttls"` / `"tls"`), `EHLO`, STARTTLS
upgrade via `net.startTLS`, `AUTH PLAIN`, `MAIL FROM` / `RCPT TO` / `DATA`
(CRLF-normalised, dot-stuffed) / `QUIT` - throwing a catchable `Error` (kind
`"smtp"`) on rejection. The message is any string, typically from
`mime.encode`. Uses `net`, so it is the first default-binary-only module
(`jennifer-tiny` stubs the network stack; a send there raises a friendly
error). Testing three ways: the pure protocol logic (reply-code parsing incl.
multi-line `250-`, `AUTH PLAIN` base64, dot-stuffing) in the overlay
(`modules/smtp_test.j`, 100%); the full networked dialogue end to end against
an in-process fake SMTP server in the Go suite (`TestSmtpSendDialogue`, so it
runs in CI without an external server); and a live send verified against a
local SMTP daemon. `AUTH LOGIN` / `XOAUTH2` follow; the challenge-response
mechanisms land with M20.1 `crypto`. A non-ASCII host or envelope address
(an IDN domain, a non-ASCII local part) throws a clear error rather than
sending a misrouted address, until IDNA lands (M18.4.6). Reference doc
[docs/modules/smtp.md](modules/smtp.md); demo
`examples/modules/smtp_demo.j`.

### M18.4.3 - `pop` (POP3 receive)

**Done.** POP3 receive client (`modules/pop.j`) over `net`: a stateful
session - `pop.connect(opts)` (greet, optional STLS, `USER` / `PASS`), then
`stat` / `count` / `sizes` / `retrieve(n)` / `deleteMessage(n)` / `quit`, with
`fetchAll(opts)` for the common case - reading the `+OK` / `-ERR` status
dialogue and dot-terminated multi-line responses (un-stuffing a doubled
leading dot). Retrieved messages are raw strings for `mime.parse`; a `-ERR`
throws a catchable `Error` (kind `"pop3"`). Named `pop`, not `pop3`: a
Jennifer namespace is letters-only, so a digit can't be a call prefix (POP v3
is the only one in use; Ruby's `net/pop` makes the same choice). Uses `net`,
so default-binary-only, with the same IDN loud-fail guard as `smtp` (until
M18.4.6). Tested: pure parsers (status, `STAT`, `LIST` sizes, dot-terminator /
un-stuffing) in the overlay (`modules/pop_test.j`, 100%); the full session end
to end against an in-process fake POP3 server in the Go suite
(`TestPop3Receive`). Reference doc [docs/modules/pop.md](modules/pop.md); demo
`examples/modules/pop_demo.j`.

### M18.4.4 - `imap` (receive)

**Done.** IMAP4rev1 receive client (`modules/imap.j`) over `net`, a useful
reading subset: `imap.connect(opts)` (greeting, optional STARTTLS, `LOGIN`),
then `selectMailbox(name)` (returns the `EXISTS` count), `search()`
(`SEARCH ALL` sequence numbers), `fetch(n)` (`FETCH n BODY.PEEK[]`, the whole
message), `logout`, with `fetchAll(opts, mailbox)` for the common case. The
two IMAP mechanics are handled: tagged commands / untagged `*` responses (a
single fixed tag, safe for a synchronous client; `NO` / `BAD` throws `Error`
kind `"imap"`), and `{N}` **literals** read by byte count rather than by line,
so a `FETCH` body is returned intact. Literals are read as 7-bit / ASCII
(MIME keeps mail ASCII); raw 8-bit is not yet byte-exact. Retrieved messages
are strings for `mime.parse`; default-binary-only, with the same IDN guard.
Tested: pure parsers (tag detection, literal length / extraction, `EXISTS` /
`SEARCH`, quoting, tagged `OK` / `NO`) in the overlay (`modules/imap_test.j`,
100%); the full session with literals end to end against an in-process fake
IMAP server in the Go suite (`TestImapReceive`). Out of scope: partial fetch,
`STORE` / `COPY` / `APPEND` / `EXPUNGE`, mailbox management, `IDLE`, SASL
`AUTHENTICATE`. Reference doc [docs/modules/imap.md](modules/imap.md); demo
`examples/modules/imap_demo.j`.

With this the mail suite's clients are complete (`mime` + `smtp` + `pop` +
`imap`), and so are the shared pieces `sasl` and `idna` - **M18.4 is done**.

### M18.4.5 - `sasl` (auth mechanisms, incl. XOAUTH2)

**Done.** A shared `sasl` module (`modules/sasl.j`) hosting the crypto-free
SASL mechanisms as pure encoders (base64, no networking, TinyGo-clean):
`sasl.plain(user, pass)`, the two `sasl.login*` steps, and `sasl.bearer(user,
token)` - the `base64("user=" ... "\x01auth=Bearer " token "\x01\x01")` string
that authenticates to Google / Microsoft 365 (both have retired password
auth). Named `bearer`, not `xoauth2`, because a Jennifer method name is
letters-only; the wire mechanism name "XOAUTH2" is a string the client sends.
The `smtp` / `pop` / `imap` clients gained an explicit `Options.auth` mechanism
(`""` auto / `"plain"` / `"login"` / `"xoauth2"`, token in `pass`) and run the
mechanism-specific wire dialogue (SMTP `AUTH`, IMAP `AUTHENTICATE`, POP3
`AUTH`) around these encoders, replacing their inline auth. This ships the
**use-a-token** half of OAuth2: given an access token, mail works with the big
providers today; the token itself comes from the generic `oauth` client
(M18.7.3). The `\x01` byte a Jennifer string has no escape for is built from
`bytes` (or `convert.fromCodepoint(1)` once M18.4.6 lands). Tested: encoders
against independent references in the overlay (`modules/sasl_test.j`, 100%);
each client's XOAUTH2 command captured and matched end to end against an
in-process server in the Go suite (`TestSmtpXoauth2` / `TestPopXoauth2` /
`TestImapXoauth2`). Challenge-response (`SCRAM-SHA-256`, `CRAM-MD5`) joins here
once `crypto` (M20.1) lands; a later LDAP client reuses the module. Reference
doc [docs/modules/sasl.md](modules/sasl.md).

### M18.4.6 - `idna` (internationalized domains)

**Done.** An `idna` module (`modules/idna.j`): `toAscii` / `toUnicode` over a
Punycode (RFC 3492) bootstring core, pure Jennifer, TinyGo-clean - so a
domain like `münchen.de` goes on the wire as `xn--mnchen-3ya.de` and back,
label by label, with ASCII labels lowercased. The mail clients apply
`idna.toAscii` to the connection host and to the **domain** part of each SMTP
envelope address, so an internationalized recipient is delivered instead of
throwing; a non-ASCII **local part** still errors (it needs SMTPUTF8, a later
step - full IDNA2008 nameprep / mapping tables and the SMTPUTF8 send path are
out of scope here). Enabler shipped: `convert.toCodepoint(char)` /
`convert.fromCodepoint(n)` (rune to / from its integer code point), which the
bootstring arithmetic needs and is reusable for any Unicode algorithm.
Reusable beyond mail: URL hosts, DNS tools, any IDN. Tested: Punycode against
known vectors and round-trips in the overlay (`modules/idna_test.j`, 100%);
the mail clients' `asciiEnvelope` domain-encode / local-part-reject in their
overlays. Reference doc [docs/modules/idna.md](modules/idna.md); demo
`examples/modules/idna_demo.j`.

### M18.5 - `redis` module

**Done.** A `redis` module (`modules/redis.j`): a RESP2 client over `net`.
Commands go out as RESP arrays of bulk strings (byte-counted lengths); replies
parse through a pure, buffer-driven decoder (`parseComplete`) that handles
simple strings, errors, integers, bulk strings (nil `$-1`), and recursive
arrays, reporting an incomplete buffer so `readReply` reads until a whole reply
has arrived. Typed helpers (`get` / `set` / `del` / `exists` / `incr` / `decr`
/ `keys` / `ping`) keep the common path fully typed; a generic
`command(session, args)` returns the raw `Reply` (`kind` / `str` / `num` /
`items`), walked with accessors the same way a `json.Value` is
([M16.16](#m1616---jsonvalue)). `connect` does optional `AUTH [user] password`
and `SELECT db`; a `-ERR` reply throws a catchable `Error` (kind `"redis"`).
Plaintext and `"tls"` (rediss, via net TLS) transports. Bulk values are read as
UTF-8 text (byte-exact for ASCII / UTF-8; binary values want base64 until a
byte-native read lands). Tested: the RESP encoder / decoder in the overlay
(`modules/redis_test.j`, 100%) and the full networked session against an
in-process RESP server in the Go suite (`TestRedisCommands`), so CI needs no
Redis install. Reference doc [docs/modules/redis.md](modules/redis.md); demo
`examples/modules/redis_demo.j`.

A Redis client over `net`. RESP2 framing (`+OK`, `$len`, `*count`,
`:int`, `-ERR`) parses cleanly in `.j`; commands go out as RESP arrays.
Plaintext first (trusted network / localhost); `rediss://` TLS is a later
add via net TLS ([M16.14](#m1614---net-tls)). `AUTH [user] password` is a
plain command, so no `crypto` dependency. Typed per-command helpers
(`get -> string`, `incr -> int`, `lrange -> list`) keep the common path
fully typed; a generic `command(...)` returning the raw reply would use
an opaque `redis.Reply` walked with accessors, the same pattern as
`json.Value` ([M16.16](#m1616---jsonvalue)). No hard prerequisites (just
`net`), so it can land ahead of `mail`.

### M18.6 - `memcache` module

A client for the `memcached` server's text protocol (`set` / `get` /
`delete` / `incr` / `decr`; replies `STORED` / `VALUE ... END`) over
`net`. Pure Jennifer, plaintext (memcached rarely uses TLS); values are
`bytes` / `string`. Named `memcache` for the client / protocol; it talks
to a `memcached` daemon.

### M18.7 - `http` module

A client over `net`. HTTPS needs net TLS ([M16.14](#m1614---net-tls)).
Groups with the `httpd` server below; the two can share HTTP request /
response parsing.

### M18.7.1 - `gotify` module on top of `http` module

A tiny real-world module built on the M18.7 `http` client: push a
notification to a [Gotify](https://gotify.net) server. It is the second
reference module (after M17.5 `ansi`), and the first that crosses the
module boundary into a *network* dependency - small enough to read in one
sitting, and the headline example that makes the http client tangible.

- **Surface.** `export def struct Config { url as string, token as string };`
  plus `export func push(cfg as Config, title as string, message as string,
  priority as int)`. `push` POSTs `title` / `message` / `priority` as
  `application/x-www-form-urlencoded` to `cfg.url + "/message"` with an
  `X-Gotify-Key: cfg.token` header - Gotify's
  [push-message](https://gotify.net/docs/pushmsg) contract, where
  [priority](https://gotify.net/docs/priority) is a plain int. Returns the
  `http` response so the caller can check the status.
- **Stateless, declarations-only.** No `init()` that stashes url + token:
  M17.3 forbids mutable module state. The caller holds a value-semantic
  `Config` and passes it per call, the same shape as the `time` / `hash`
  structs. Usage: `def g as gotify.Config init gotify.Config{ url: $url,
  token: $tok }; gotify.push($g, "Title", "Body", 5);`.
- **Built on `http.post`, not hand-rolled.** The module is ~15 lines
  because M18.7 owns the HTTP framing, TLS (HTTPS via M16.14), and form
  encoding; a version predating the http client would hand-roll HTTP/1.1
  over `net.connectTLS` and be thrown away when M18.7 landed, so it is
  deliberately sequenced after it. Ships as `modules/gotify.j`, resolved
  through the module search path like any other module.

**Acceptance.** `import "gotify.j" as gotify;` resolves and exports
`Config` + `push`; a `push` to a running Gotify server returns a 2xx and
the message appears in the server's feed; a bad token surfaces the server's
4xx as a value, not a crash. The URL and token are supplied by the caller
and never committed - the example reads them from the environment and the
docs use placeholders.

### M18.7.2 - `rest` module

The ergonomic REST layer over the M18.7 `http` client - a genuine
library-sized `.j` module (a step up from `gotify`'s one endpoint), and the
proof that the module system carries a real utility written in the language
itself. It is a **module, not a system library**, because it needs no host
capability of its own: everything hard is already owned by `http` (verbs,
headers, TLS, body framing) and `json` ([M16.9](#m169---json)); `rest` is
pure composition over the two, so putting it in Go would duplicate `http`
and break the dependency-free / TinyGo-clean stances for no gain. The split
is deliberate: `http` is the transport primitive (Go, needs `net` / TLS),
`rest` is the convenience layer (`.j`).

- **Surface.** A value-semantic client plus JSON-aware verbs:
  `export def struct Client { baseUrl as string, headers as map of string
  to string };` and `export func get/post/put/patch/delete(c as Client,
  path as string, ...)` returning a `Response` struct
  `{ status as int, headers as map of string to string, body as string }`.
  JSON convenience wrappers - `rest.getJson(c, path) -> json.Value`,
  `rest.postJson(c, path, body as json.Value)` - encode / decode through
  `json` and set `Content-Type`. Base-URL joining, query-string building,
  and `Authorization` (Bearer / Basic, the latter via `encoding` base64)
  are string / map work.
- **Stateless, declarations-only.** Like `gotify`, the caller holds the
  `Client` value and threads it per call (M17.3 forbids mutable module
  state); auth lives in `Client.headers`. A stateful cookie-jar *session*
  is deliberately out of scope for v1 - it would need either a
  functionally-threaded jar or an `http`-side handle (a system library may
  hold stateful handles like `net.Conn`; a module may not), so it is
  deferred to `http` rather than dragging `rest` into Go.
- **Built on `http` + `json`, not hand-rolled.** No sockets, no TLS, no
  parsing in this module - it calls `http` for transport and `json` for
  bodies. Ships as `modules/rest.j`, resolved through the module search
  path.

**Acceptance.** `import "rest.j" as rest;` resolves and exports `Client` +
the verb functions; a full CRUD round-trip against a test server
(`postJson` create -> `getJson` read -> `put`/`patch` update -> `delete`)
returns the expected statuses and decoded bodies; a 4xx / 5xx is reported
as a `Response` value (status inspectable), not a crash; base-URL joining
and query params compose without double slashes.

### M18.7.3 - `oauth` module (generic OAuth2 client)

The **get-a-token** half of OAuth2 (the *use-a-token* half is `sasl`
XOAUTH2, M18.4.5). A generic OAuth2 client - not email-specific; any
OAuth2-protected API - built on `http` + `json`, with `hash` for PKCE S256.
Acquires and refreshes access tokens; provider presets for Google and
Microsoft 365 (endpoints + scopes) make mail the headline consumer, its
tokens feeding `sasl.xoauth2`. Flows tier by dependency, so they land in
order:

- **No extra deps (ship first):** Client Credentials, Refresh Token, and the
  **Device Authorization Grant** - the last a natural fit for a CLI /
  embeddable runtime (no local redirect server; show the user a URL + code,
  poll the token endpoint). These need only `http` + `json`.
- **Authorization Code + PKCE:** needs `httpd` (M18.8) to catch the redirect
  and crypto-grade random for the PKCE verifier (its security rests on
  verifier entropy, so `math.rand` will not do) - lands after `httpd` and
  `crypto` (M20.1).
- **Service-account JWT assertion (Google):** RSA-signed client assertion,
  so it waits on `crypto` (M20.1) too.

Token refresh / expiry handling and a small on-disk token store (via `fs`)
round it out. Stateless / declarations-only like the other `http` consumers:
the caller holds the token value and threads it.

**Acceptance.** A device-flow + refresh round-trip against a mock OAuth2
token endpoint yields an access token, refreshes it when expired, and the
token drives `sasl.xoauth2` into a successful IMAP `AUTHENTICATE XOAUTH2`
against a mock server; a token-endpoint error surfaces as a catchable
`Error`, not a crash.

### M18.8 - `httpd` module

A pure-Jennifer HTTP server atop `net`, shipped as a module (same shape
as the other M18 modules), not baked into the interpreter - the point
where Jennifer becomes useful for serving content. Per-connection
handlers run in `spawn` blocks (depends on **M16.0** concurrency) over the
`net` TCP listener (**M16.2**); it can share HTTP request / response
parsing with the M18.7 client. (Formerly the standalone M20.)

### M18.9 - `toml` module

A `.j` module: TOML's regular, line / section-oriented grammar
(`[table]`, `key = value`) maps cleanly to `map` / `list` / `time.Time` -
the `.j`-friendliest of the config formats (unlike `yaml` / `xml`, which
go to M20 as system libraries). Covers the grammar corners: multiline
strings, inline tables, arrays of tables. (`.ini` is a deferred tiny
cousin only if demand surfaces - no real standard, ambiguous quoting /
typing.)

### M18.10 - `flatdb` module

A file-backed JSON document store as a `.j` module - the "embed a small
store" need, built from parts that already exist: `json`
([M16.9](#m169---json)) for the data and `fs` for the file. Load once,
query and mutate in memory, write back atomically. It is deliberately *not*
a database engine; the honest description is "a small JSON store with
crash-atomic whole-file replace," and that framing goes in the docs so no
one mistakes it for OLTP.

- **Handle-as-value, not a connection.** A module holds no mutable state
  and `spawn` deep-copies scope, so the store cannot be a shared open
  connection. It is a value the caller holds: `export def struct DB { path
  as string, data as json.Value };` (a library type in an exported field is
  fine - the referential-closure check concerns only *module* structs).
  `flatdb.open(path) -> DB` loads the file (empty doc if absent); mutating
  verbs return a fresh `DB` (`flatdb.set(db, pointer, value)`,
  `flatdb.remove(db, pointer)`); readers do not (`flatdb.get(db, pointer)`,
  `flatdb.has(db, pointer)`); `flatdb.save(db)` writes it back. Addressing
  reuses `json`'s JSON-Pointer model, so `flatdb` is a thin file-lifecycle +
  ergonomics layer over the `json` write surface, not a re-implementation.
- **Atomic replace.** `save` writes to a sibling temp file and `fs.rename`s
  it over the target, so a reader ever sees the whole old file or the whole
  new one, never a torn write. Single-writer by construction (whole-file).
- **What it is and isn't (ACID, honestly).** *Atomicity*: whole-file, via
  temp + rename. *Consistency*: application-level. *Isolation*: none - one
  process, reload-the-whole-file, no concurrent transactions. *Durability*:
  the rename is atomic, but flush-to-disk is OS-buffered unless `fs` later
  grows an `fsync` verb. So: crash-atomic snapshotting of small data, not a
  transactional engine. Real databases are a different milestone track -
  *clients* over `net` (`redis` M18.5; `postgres` / `mysql` would be wire
  protocols, no CGo), never a homegrown ACID engine.
- **First use case: a benchmark database.** `examples/benchmark.j
  --format json` emits one self-describing JSON record per run (schema,
  `build` / `version`, `cpu` / `ncpu` / `platform` / `arch`, and the serial
  + parallel timings). Feeding that into a `flatdb` store - append a record
  per run, keyed on `(cpu, platform, arch, build)` - turns the ad-hoc
  reference numbers (today pinned by hand in `technical/tinygo.md`) into a
  queryable performance history across machines, OSes, and interpreter
  builds. It is the natural first real workload for `flatdb`: small,
  append-mostly, human-readable, and exactly the shape a JSON store serves
  well. The `--format json` producer already ships; the ingest side (a
  small `flatdb.append`-style helper, or a `bench-collect.j`) lands with the
  module.

**Naming.** `flatdb` (flat-file DB) over the earlier working name `store`
and over `simpledb`: it names the shape (one flat JSON file) rather than
overselling "database."

**Acceptance.** `open` of a missing path yields an empty store; `set` /
`get` / `remove` round-trip through a JSON Pointer; `save` then a fresh
`open` returns the same data; an interrupted `save` (temp file present, no
rename) leaves the original intact; the docs state plainly it is not
transactional.

### M18.11 - `gpio` module

Raspberry-Pi (and any Linux SBC) GPIO as a **pure `.j` module** over
sysfs - the physical-computing / IoT-teaching use case, with no core
changes, no system library, and no platform build-tag. `/sys/class/gpio`
is plain files, so `fs` is the whole backend: export a pin, set its
direction, read / write its value. "Blink an LED from a five-line script"
is the headline, and it competes with `RPi.GPIO` / `gpiozero` on the
simplicity of the language itself.

- **Why a module, not a system library.** Imports are static (`use` /
  `import` resolve before execution, uncatchable, block-illegal), so there
  is no "check then conditionally import." The portability seam is instead
  *which module file is on the search path* (Go uses build tags; Jennifer
  uses the module file). A program writes one uniform line - `import
  "gpio.j" as gpio;` - and the deployment supplies the right `gpio.j`: the
  real sysfs module on a Pi, an emulator (blink a console cell) elsewhere.
  A capability like this being *absent* off-Linux (rather than a stubbed
  system library that errors mid-call) is the right call for a
  *platform*-bound feature: stub when the same source must load in both
  binaries (why `net` stubs on `jennifer-tiny` - a *toolchain* axis); be
  absent when the capability is genuinely platform-bound (a *platform*
  axis), so `use`-ing it fails at the top with a clear message rather than
  three calls deep. Neither `try`/`catch` nor an `isDefined()` reflection
  primitive is needed or wanted here; both fight the static-import model.
- **Surface (pin-keyed, stateless).** sysfs derives every path from the pin
  number, so no handle is needed: `gpio.setup(pin as int, direction as
  string)` exports the pin and sets `"in"` / `"out"`; `gpio.write(pin as
  int, value as int)`; `gpio.read(pin as int) -> int`; `gpio.release(pin as
  int)` unexports. A missing `/sys/class/gpio` (not a Pi, sysfs disabled)
  is a clear positioned error, not a crash - `fs.exists` gate, the same
  fail-soft as `os.isTerminal`.
- **Testability.** Real sysfs needs root and hardware, so the module reads
  its base directory from a constant (default `/sys/class/gpio`) that tests
  point at a temp-dir mock; the round-trip is verified against the mock in
  CI, real pins manually on a Pi.

**sysfs default, chardev as the escape valve.** sysfs GPIO is deprecated
(the kernel prefers the `/dev/gpiochip` character device and can compile
sysfs out). The bet is that sysfs stays available on the hobbyist Pi
kernels this targets; the module keeps its API stable so that **if sysfs is
removed - or `fs` is ever dropped from `jennifer-tiny` - the backend can be
swapped for a `/dev/gpiochip` ioctl system library (Linux build-tag,
`syscall` or an `x/sys/unix` carve-out) with no change to `.j` scripts.**
The pure-module form is the default because it costs the language nothing;
the system library is the future-proofing, taken only when forced.

**Acceptance.** Against a mock sysfs tree: `setup` writes `export` and
`direction`; `write` / `read` round-trip a pin's `value`; `release` writes
`unexport`; a missing base directory errors positionally. On a real Pi, a
`gpio.setup(17, "out")` + `gpio.write(17, 1)` script lights an LED, and the
same script with an emulator `gpio.j` on the search path runs on a laptop.

### M18.12 - `docblock` module

A doc-comment parser: read Jennifer source and return the documentation
embedded in it as structured values. Two deliverables - a **blessed
doc-comment format** (documented under `docs/` as "Jennifer doc comments")
and **`docblock.j`**, the pure-`.j` module that parses it. The module
produces data; it does not render. Turning parsed docs into HTML is a
separate consumer built later on `htmlwriter` (M18.2), so `docblock` stays
a small, self-contained parser with no output dependency.

**The format.** A doc comment opens with exactly `/**` (a plain `/*` block
comment stays invisible); its body is a summary line, an optional
description, and `@`-tags. It must immediately precede the construct it
documents - `func`, `def struct`, `def const`, or, when it precedes none,
the file preamble (module doc). `export` is read from the construct
keyword, not a tag. Tags: `@param name {type} desc` and `@field name
{type} desc` (one per parameter / field), `@return {type} desc`, `@throws
{type} desc`, the universals `@since @deprecated @see @example @internal`,
and the preamble set `@module @author @version @license`. Types are
written **verbatim in Jennifer's own syntax** inside mandatory `{ }`
(`{int}`, `{list of int}`, `{map of string to list of int}`,
`{json.Value}`) - no invented notation, and the braces make RE2 extraction
unambiguous. There is no `mixed` / `any` pseudo-type: Jennifer has no top
type, so an opaque value documents as `json.Value` or a named struct.

**The result is typed data, not a tag bag.** Jennifer has no sum types and
no `any`, so heterogeneous collections are modelled as parallel typed lists
plus fixed-field structs: a `FileDoc { module, funcs, structs, consts,
diagnostics }`, a `FuncDoc { name, exported, summary, description, params,
returns, throws, examples, since, deprecated, see, internal }`, and the
leaves `ParamDoc` / `ReturnDoc` / `ThrowDoc` / `StructDoc` / `ConstDoc` /
`ModuleDoc` / `Diagnostic { severity, line, message }`. The module
**reports, never enforces**: mismatches surface as `Diagnostic` values and
the caller decides what is fatal.

**Implementation.** Pure `.j` over `regex` and `strings`. Doc-comment
boundaries are found by a char-level `/*` / `*/` depth scan that skips
string literals (mirroring the lexer, which already emits nesting-correct
block-comment trivia) - not a fragile "delimiter alone on its line" rule.
Association to a construct assumes conventionally-laid-out source; the
reference layout is `jennifer fmt` output (fmt preserves doc comments and
normalises layout deterministically), which the docs name as the canonical
normaliser. The one validation that earns its keep is the
signature cross-check: `@param` / `@field` names and count are matched
against the real declaration (Jennifer types carry no commas or parens, so
a parameter list splits cleanly), catching the commonest doc bug - docs
drifting from the code. Whole-body analysis (for example "a `@return` on a
function that never returns a value") is out of scope: it needs the AST,
and the module has only the text.

**Future-proofed, not future-built.** The format is designed so a later
backend can discover constructs from a `jennifer tokens` / `ast` trivia
feed instead of re-scanning text, with no change to the format itself; and
because `@example` bodies are kept runnable, a `jennifer test` doctest hook
that executes them is a natural follow-on. Neither is part of this
milestone.

**Acceptance.** `docblock.parse(source)` on a file mixing an exported
function, an exported struct, a constant, and a module preamble returns a
`FileDoc` with each construct's summary, description, and tags populated
and `exported` set from the keyword; a `@param` whose name is not a real
parameter (and a parameter with no `@param`) each produce a `Diagnostic`;
a doc comment that precedes no construct and no preamble is reported
orphaned rather than silently dropped; nested `/* */` inside a body and a
`/**` inside a string literal are both handled correctly. Ships with the
`docblock_test.j` overlay (100%), `docs/modules/docblock.md`, a
`docs/modules/index.md` row, a `SUMMARY.md` entry, a runnable
`examples/modules/docblock_demo.j`, and a `modules/README.md` entry.

## M19 - cross-cutting tooling

The catch-all bucket for milestones that improve the interpreter or its
tooling but belong to neither the Jennifer-coded modules of M18 nor the Go
system libraries of M20. Numbered sub-entries land here as needs arise.

### M19.1 - `.j` code coverage

Teach `jennifer test` to report which lines of the code under test actually
ran. The profiler (`jennifer profile`) already records per-position hit
counts, so the raw signal exists; this milestone surfaces it as coverage: a
per-file and total percentage, the list of never-executed positions, and a
machine-readable form a CI job or an editor can consume. It reuses the
profiler's instrumentation rather than adding a second counting path, and is
independent of any renderer: a plain-text summary is the baseline output.
Educationally it closes the loop the REPL / linter / profiler / test-runner
set opened - a learner sees not just that tests pass but what the tests
miss.

- **Surface.** `jennifer test --coverage[=FORMAT]` runs the suite with the
  profiler's counters live over the tested file(s) and emits a coverage
  report next to the test report. Default `text` (per-file and total
  percent); plus a machine-readable form for tooling.
- **Reuse, do not duplicate.** Coverage is a second consumer of the
  profiler's per-position hit data, not a new instrumentation pass.
- **No renderer dependency.** Text is the baseline; an HTML coverage view
  would be a later consumer built on `htmlwriter` (M18.2), not part of this
  milestone.

**Acceptance.** The coverage report over a file whose tests exercise some
but not all of its methods shows below 100 percent and names the unexecuted
positions; a suite that touches every position shows 100 percent; the
machine-readable form parses; the plain `jennifer test` path (no
`--coverage`) is byte-for-byte unchanged.

## M20 - system libraries

Go **system libraries**: cryptographic primitives, plus formats too heavy
or too reflect-bound for a Jennifer-coded `.j` module (the `json` pattern,
[M16.9](#m169---json)). Members below; more land as M20.x as needs arise.

### M20.1 - `crypto`

Symmetric and asymmetric primitives, key derivation, crypto-grade random.
System library; TinyGo-safe primitives only. Hashes already shipped in
M15.6.

- **Swap `uuid`'s random source.** `uuid` (M16.10) draws its v4 / v7
  randomness from `math`'s shared non-crypto RNG (seedable, predictable -
  documented). When crypto-grade random lands here, repoint
  `uuidlib.randByte` at it so `uuid.v4` is unguessable; the change is one
  function, no surface change. Until then `uuid` must not be used for
  security tokens.
- **Message authentication (HMAC).** `crypto.hmac(key, data, algo) ->
  bytes` over the `hash` algorithms, plus a constant-time
  `crypto.hmacEqual` for verification. Go standard library
  (`crypto/hmac`), no dependency, TinyGo-clean - and the shared
  foundation the KDFs below build on (PBKDF2 is iterated HMAC; HKDF and
  SASL SCRAM are HMAC-based). Needed directly all the time too: request
  signing, webhook verification, JWT HS256, TOTP.
- **Key derivation (stdlib, no dependency).** `HKDF` - derive keys from a
  high-entropy secret - and `PBKDF2-HMAC-SHA256` - derive a key from a
  password (salt + iteration count). Both come from the Go standard
  library (`crypto/hkdf`, `crypto/pbkdf2`, stdlib since Go 1.24), so they
  add no dependency and stay TinyGo-clean. Shape: `crypto.hkdf(...)` /
  `crypto.pbkdf2(password, salt, iterations, keyLen) -> bytes`.
- **Password hashing is out of scope here.** Argon2id (and bcrypt /
  scrypt) moved to the Long-horizon list: they need the `x/crypto`
  dependency and their own `hashPassword` / `verifyPassword` surface.

### M20.2 - `xml`

Hand-rolled like `json` (Go's `encoding/xml` is reflect-heavy, so
TinyGo-hostile). A genuinely complex tree - attributes + ordered,
possibly-duplicated children + mixed text + namespaces + entities - whose
byte-level parsing is too slow in `.j`. Also the natural mirror target for
the `json.Value` read / write vocabulary
([M16.16](#m1616---jsonvalue)): the same opaque-handle plus path-addressed
accessor shape (an XPath-style path dialect in place of JSON Pointer).

### M20.3 - `yaml`

A system library because full YAML - anchors / aliases, flow *and* block
styles, implicit typing, multi-document streams - is impractical in `.j`
and has no Go stdlib. Unlike `xml`, that means a **Go dependency** (e.g.
`gopkg.in/yaml.v3`): the one place a config parser earns one, since a
hand-rolled full YAML is a project of its own. Verify TinyGo-cleanliness
of the dependency, and fall back to a documented subset if it won't build
there.

### M20.4 - `i18n`

Message catalogs and locale-aware translation. A **system library**, not a
`.j` module, for two independent reasons: it needs **global mutable state**
(the current locale plus loaded catalogs), which a declarations-only module
cannot hold; and it needs **performance** - Jennifer's `map` is a linear-scan
`[]MapEntry`, so a large catalog looked up per call would be O(n), whereas the
library holds catalogs in a Go `map[string]string` (O(1)). The
`map of string to string` a caller passes is fine as the *load* interface (a
one-time ingest); the per-lookup scan is what the library avoids.

Surface: `i18n.load(lang, catalog)` (a `map of string to string`, built from
`json` today or `yaml` here at M20.3, or a literal), `i18n.setLocale(lang)` /
`i18n.locale()`, `i18n.tr(key)` (translate in the current locale; fallback
locale -> default lang -> the key itself, so a missing key is visible), and
`i18n.tr(key, params)` for named interpolation (`"Hello, {name}"`).
Pluralization (CLDR per-language plural rules) and an `i18n.loadFile(path)`
convenience are follow-ons.

No gettext-style `_()`: `_` is not a valid Jennifer method name (letters-only;
`_` is reserved for constant-name separators), and a bare `tr()` builtin does
not clear the `len` promotion bar (translation is not useful to nearly every
program). Ambient-global `_()` is also exactly what stances 2 (explicit) and 7
(namespaced, no globals) rule out - so the call is `i18n.tr("key")` (or
`use i18n as t; t.tr("key")`). Extending `printf` for translation is rejected
(see [technical/rejected.md](technical/rejected.md)): translation is content
substitution from stateful external data, not presentation of the value in
hand. Locale-aware *value* formatting (number / date grouping) is a separate,
open `printf` question.

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
- **Encoding / binary protocols.**
  - **`asn1`** - ASN.1 BER/DER encode/decode, as a **Go system
    library**. Byte-level binary parsing belongs in Go, not `.j` (the
    `json` lesson: a char-by-char parser in the interpreter pays
    overhead per byte). This is the *enabler* for a family of binary
    protocols and PKI formats - LDAP, SNMP, X.509, PKCS. Go's stdlib
    `encoding/asn1` is DER-only, so the full BER that LDAP / SNMP use
    (indefinite lengths, alternative encodings) needs either a BER
    dependency (e.g. `go-asn1-ber`) or a hand-rolled BER codec in Go.
  - **`ldap` / `snmp` (layered on `asn1` + `net`).** With `asn1` doing
    the byte crunching in Go and `net` providing TCP/UDP + TLS
    (`connectTLS` / `startTLS` already cover LDAPS / StartTLS), the
    protocol orchestration (bind, build request, iterate results) is
    not per-byte hot and can live in a `.j` module or a thin Go library.
    SNMP is the natural first client (simpler PDUs, UDP, no SASL); LDAP
    adds controls + SASL (SCRAM builds on M20.1 `crypto`). A pure-`.j`
    implementation of the BER layer itself is explicitly *not* the plan.
    Existing pure implementations (e.g. PHP FreeDSx) are a protocol
    reference, not a port target - their heavy OO shape does not map to
    Jennifer's value-semantic structs.
- **Sandbox.** Restricted-capability execution.

Ordered when demand surfaces. WASM libraries (M23.x) may cover
some of this space first.

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
  `jennifer-tiny` stays fixed-offset (a build-tag split like `net`).
  Level 1 first - an offset-at-instant resolver
  (`time.offsetAt(name, $t)` / `time.zoneFor(name, $t) -> time.Zone`) that
  leaves the `time.Time {nanos, offset}` model untouched (the snapshot is
  fixed, so DST-crossing arithmetic must re-resolve); Level 2 - a
  zone-carrying `time.Time` with DST-correct arithmetic - is a larger,
  optional follow-up needing a Go-backed zone handle.
- **Password hashing (Argon2id / bcrypt / scrypt).** The modern default
  for password *storage*, deferred out of M20.1 `crypto` because it lives in
  `golang.org/x/crypto` (a dependency, unlike the stdlib KDFs M20.1 ships)
  and wants its own surface distinct from the KDFs: a self-describing
  `crypto.hashPassword(pw) -> string` (`$argon2id$...`) plus a
  constant-time `crypto.verifyPassword(pw, hash) -> bool`. Added when
  password storage is a concrete need, taking the `x/crypto` dependency
  then - crypto is the one place the dependency-free stance bends, since
  you never hand-roll it.
- **`encoding` - the harder codecs.** The single-byte character codecs and
  binary-to-text formats all shipped (M16.15); the deferred remainder,
  picked up only when a real program needs one: variable-width Asian
  encodings (`Shift-JIS`, `Big5`, `GB2312`, `GBK`, `GB18030`, `EUC-JP`,
  `EUC-KR`) - each a state machine with variant / ambiguity edge cases, a
  whole milestone apiece; `UTF-16` / `UTF-16LE` / `UTF-16BE` / `UTF-32` (BOM,
  surrogate pairs, endianness); and `UTF-7` (mail-transport - though
  `quoted-printable` already shipped as a general codec).
- **Module package manager + registry (`jvc`).** A CLI package tool plus a
  public registry (packagist-style) so a developer can declare and install
  third-party `.j` modules: `jvc install gotify>=1.0.0` resolves a version
  constraint against the registry and downloads into a project-local
  `./vendor` directory, which becomes one more module search root - the M17
  resolver already walks a search path (system dir, then `-I` dirs), so
  `vendor/` is an added entry, not a new resolution model. Needs a manifest
  + lockfile format, semver constraint solving, a registry service and
  publish flow, and integrity pinning (content hash per version). A whole
  track of its own; the module system (M17) is the prerequisite,
  `semver` ([M17.6](#m176---semver-module)) supplies the version
  comparison and constraint core it resolves against, and `gotify`
  ([M18.7.1](#m1871---gotify-module-on-top-of-http-module)) is the first module worth
  publishing through it.
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
  zero binary cost; build-time selection is only for the curated Go-level core.
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
  shape - an opaque `sql.Row` accessor type mirroring `json.Value`
  (M16.16) vs a typed struct via the deferred map-to-struct conversion.
  Values bind only
  through `?` placeholders (injection safety). Contrast the
  text-protocol stores redis / memcache (M18.5 / M18.6), which are pure
  Jennifer over `net` and need none of this.
- **Explicit map-to-struct conversion.** A spelled-out, validating way to
  turn a `json.Value` object (or a homogeneous `map of string to T`) into
  a typed struct - the sanctioned counterpart to the *rejected* implicit
  coercion (see [technical/rejected.md](technical/rejected.md)). Deferred
  from the M16.x line: once JSON is destructured through `json.Value`
  accessors, the by-hand rebuild covers the need, so a one-call form is a
  convenience, not a blocker. Two candidate shapes, decided on consistency
  not brevity - a `convert.toStruct($map, "Point")` library call (a
  two-arg, stringly-typed outlier in the otherwise one-arg `convert.toX`
  family, or else not self-contained if it reads the binding's declared
  type) versus a `Point{ ..$map }` struct-literal spread (names its type
  statically, at the cost of new literal syntax). Either way strict: every
  declared field present with a matching type, recursing into nested
  structs / lists / maps, value-semantic, no partial fills or defaults.
- **`io.lines() -> list of string`.** Slurp the whole stdin into a
  list. Additive on top of the streaming `readLine()` + `eof()`
  idiom; nice-to-have for tiny scripts, not blocking. Deferred from
  M7.
- **i18n.** Locale-aware case folding, collation, number / date
  formatting, BiDi. Gated on the CLDR-data binary-size question
  (likely an optional library shipped after the M22 WASM runtime
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
