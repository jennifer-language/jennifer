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
server). Everything beyond a 1.0.0 - embedding, WASM, and
specialised domains - lives in the
[beyond-1.0.0 idea collection](horizon.md).

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

**Deferred out of this milestone** (not gated on it): the
cross-build for macOS / Windows and a real apt repository stay in
[Requirements for 1.0.0 stable](#requirements-for-100-stable);
the extra distribution formats (Homebrew, Snap, Nix, Flatpak,
AppImage) moved to the [horizon idea collection](horizon.md).

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
  LDAP client reuses. SASL / SCRAM is a *consumer* of crypto
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
Bodies are held decoded as text; binary (`bytes`) bodies are deferred
(RFC 2047 encoded-words for non-ASCII headers followed in M18.4.7). Reference doc
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

### M18.4.7 - `mime` RFC 2047 encoded-words

**Done.** Non-ASCII header values now cross the wire as RFC 2047
encoded-words instead of raw 8-bit bytes. Added to the `mime` module
(`modules/mime.j`): `encodeWord(text)` renders one or more UTF-8 base64
encoded-words (`=?UTF-8?B?...?=`), split on rune boundaries under the
75-character limit and folded with CRLF + space; `decodeWord(value)` decodes
every encoded-word in a header value (both B and Q, dropping the whitespace
that separates adjacent words as a reader should, and leaving a word that
fails to decode verbatim so `parse` never crashes). The two are applied
**automatically and symmetrically**: `encode` encodes a non-ASCII `Subject` /
`Comments` and the display-name half of an address header (`From` / `To` /
`Cc` / `Bcc` / `Reply-To` / `Sender`), leaving the `<addr>` alone, and
`mime.address` encodes a non-ASCII name; `parse` decodes those same headers
back to text. So a `Bericht aus München` subject goes out conformant and a
fetched `=?UTF-8?B?...?=` reads back as text through `imap` / `pop`. Charset
decode routes UTF-8 through `convert` and `us-ascii` / `iso-8859-*` /
`windows-*` through `encoding`. Pure text, no new host dependency (adds
`use regex` for token scanning); TinyGo-clean. Tested in the overlay
(`modules/mime_test.j`): encode/decode round-trips, B and Q decode, adjacent-
word whitespace collapse, surrounding-text preservation, folding, the
auto-encode-on-`encode` / auto-decode-on-`parse` path, and address-name
encoding. Reference doc [docs/modules/mime.md](modules/mime.md). Out of scope
still: binary bodies, and per-mailbox name encoding in a multi-address line
(encode each name with `mime.address` when building it).

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

### M18.5.1 - `resque` module (background jobs)

**Done.** A `resque` module (`modules/resque.j`) on top of `redis` + `json`:
schedule background jobs onto named queues and process them from a worker later,
Resque wire-compatible (queues are Redis lists at `resque:queue:NAME`, the
registry a set at `resque:queues`, a job the JSON envelope
`{"class":"WorkerName","args":[...]}`), so a Ruby-resque / php-resque worker can
process Jennifer's jobs and vice versa. Surface over an existing `redis.Session`
(no new transport): `enqueue(session, queue, class, args)` (SADD registry +
RPUSH envelope), `reserve(session, queues)` -> `Job{queue, class, args}` (LPOP
the first non-empty queue in the caller's priority order; empty `Job` when
drained), `queueLength` / `queues` / `size`, `fail(session, job, message)`.
`args` is a `list of string` (Ruby-resque positional; a non-string arg from
another producer still reserves as its string form). The worker's `class`
dispatch stays user code (a `.j` module can't call a method by dynamic name).
Tested: the JSON envelope encode / decode and key builders in the overlay
(`modules/resque_test.j`, 100%) and the full enqueue / reserve / introspect /
fail path against an in-process RESP server in the Go suite (`TestResqueJobs`).
Reference doc [docs/modules/resque.md](modules/resque.md); demo
`examples/modules/resque_demo.j`. Deferred (kept out of the basics): blocking
`BLPOP` reserve, a fully Resque-compatible failure record, delayed jobs,
retries, and a configurable namespace.

A `resque` module on top of `redis` ([M18.5](#m185---redis-module)) + `json`:
schedule background jobs onto named queues now, process them from a worker
later. Deliberately Resque **wire-compatible**, so a job Jennifer enqueues can
be processed by a Ruby-resque / php-resque worker and vice versa - the layout
is a small, stable de-facto standard:

- **Enqueue** - `SADD resque:queues NAME` then `RPUSH resque:queue:NAME <json>`,
  the JSON envelope `{"class":"WorkerName","args":[...]}`.
- **Reserve** - `LPOP resque:queue:NAME` across the caller's queue list in
  priority order (FIFO within a queue), returning a `Job{queue, class, args}`
  (an empty `Job` when every queue is drained).
- **Introspect** - `queueLength` / `queues` / `size`.
- **Fail** - push a simplified failure record to the `failed` list.

Surface (over an existing `redis.Session`, no new transport): `enqueue(session,
queue, class, args)`, `reserve(session, queues)` -> `Job`, `queueLength` /
`queues` / `size`, `fail(session, job, message)`. **args is a `list of
string`** - it maps exactly to Ruby-resque's *positional* args
(`perform(a, b)`); the php-resque single-hash convention (`args:[{...}]`, plus
its `id` / `queue_time` envelope fields) is documented for that ecosystem.

The **worker loop is user code**: `reserve`, then dispatch on `job.class`
(a Jennifer module can't call a method by dynamic name - that is the
interpreter's `testing.run` primitive, not exposed to `.j` - and keeping
dispatch in user code keeps the module small). Two caveats are inherent to
Resque, not added here: the `class` string is resolved on the *worker's* side
(the runtime that pops the job must have a class by that name), and the default
`resque:` key namespace must match on both ends.

No new host dependency (pure `.j` over `redis` + `json`), so it fits right
after `redis`. Deferred to a later pass (kept out of the basics): blocking
`reserve` (`BLPOP` with a timeout, so a worker sleeps instead of poll-looping),
a fully Resque-compatible `failed`-queue record (`failed_at` / `exception` /
`backtrace` / `worker`), scheduled / delayed jobs, and retries. Discipline as
usual: `modules/resque_test.j` overlay (JSON-envelope shape as pure helpers;
the networked enqueue / reserve round-trip against the in-process RESP server
in a Go test), `docs/modules/resque.md`, catalog + `SUMMARY` + `README` +
`JENNIFER.md` entries, and `examples/modules/resque_demo.j`.

### M18.6 - `memcache` module

**Done.** A `memcache` module (`modules/memcache.j`): a client for memcached's
classic text protocol over `net`. `connect` opens a session; `set` / `add`
(store-if-absent, `-> bool`) store a value with an `exptime`-second TTL;
`get` returns the string value (`""` when absent / expired); `delete` /
`touch` return whether the key existed; `incr` / `decr` are atomic counters
returning the new value (`-1` when the key is absent, since memcached will not
create it); `quit` closes. A protocol-error reply (`ERROR` / `CLIENT_ERROR` /
`SERVER_ERROR`) throws a catchable `Error` (kind `"memcache"`). The reader
handles the line replies plus `get`'s length-prefixed `VALUE ... END` block;
values are read as UTF-8 text (byte-exact for ASCII / UTF-8, the common case;
the same documented limitation as `redis`). `add` + a TTL and the
`incr`-then-`add` shape are exactly the primitives the two reference modules
([M18.6.1](#m1861---session-module-on-memcache) /
[M18.6.2](#m1862---ratelimit-module-on-memcache)) build on. Tested: the
storage-header byte-length framing in the overlay (`modules/memcache_test.j`,
100%) and the full command surface against an in-process memcached server in
the Go suite (`TestMemcacheCommands`), so CI needs no memcached install.
Reference doc [docs/modules/memcache.md](modules/memcache.md); demo
`examples/modules/memcache_demo.j`.

A client for the `memcached` server's text protocol (`set` / `get` /
`delete` / `incr` / `decr`, plus `add` (store only if absent) and `touch`
(re-arm expiry); replies `STORED` / `VALUE ... END`) over `net`. Every
store carries an **expiration** (`exptime`, the per-key TTL memcached is
built around). Pure Jennifer, plaintext (memcached rarely uses TLS);
values are `bytes` / `string`. Named `memcache` for the client / protocol;
it talks to a `memcached` daemon. Two reference modules build on it -
[M18.6.1 `session`](#m1861---session-module-on-memcache) and
[M18.6.2 `ratelimit`](#m1862---ratelimit-module-on-memcache) - and the
`httpd` server ([M18.9](#m189---httpd)) uses both.

#### M18.6.1 - `session` module (on `memcache`)

**Done.** A `session` module (`modules/session.j`) on `memcache` + `uuid` +
`json`: `create(mc, ttl)` mints a UUID v4 ID and stores an empty session,
`load(mc, id)` returns the `map of string to string` (empty when absent /
expired), `save(mc, id, data, ttl)` writes it and slides the expiry,
`touch(mc, id, ttl)` re-arms the TTL, `destroy(mc, id)` removes it - all under a
`sess:ID` key. The data map is stored **base64-wrapped JSON**: `json.encode`
keeps raw UTF-8, and memcached's value read is only byte-exact for ASCII, so the
base64 wrap makes every value ASCII on the wire and any UTF-8 session value
(`"José"`) round-trips exactly - proven in both test tiers. Volatile by nature
(a cache of soft state, not a store of record); IDs are non-crypto UUID v4 (fine
for a cache key, documented). Tested: the base64 + JSON encode / decode round
trip (incl. non-ASCII and the ASCII-blob property), key building, and JSON
Pointer escaping in the overlay (`modules/session_test.j`, 100%); the full
create / load / save / touch / destroy lifecycle against an in-process memcached
server in the Go suite (`TestSessionLifecycle`). A PHP-session-compatible layout
stays a follow-on. Reference doc [docs/modules/session.md](modules/session.md);
demo `examples/modules/session_demo.j`.

A server-side session store on `memcache`, the canonical memcached use (it
is what PHP's memcached session handler does). A session is a `map of string
to string` held under a `sess:ID` key with a sliding TTL, so it expires on
its own when idle - exactly what memcached's per-key `exptime` gives for
free. Threads three existing pieces together the way `resque` threads
`redis` + `json`: `memcache` (store + TTL), `uuid` (`generate("v4")` for the
session ID), and `json` (encode the map as the stored value). Surface over
an existing `memcache` connection: `create(mc, ttl) -> id` (mint an ID,
store an empty session), `load(mc, id) -> map of string to string` (empty
map when absent / expired), `save(mc, id, data, ttl)`, `touch(mc, id, ttl)`
(re-arm the expiry without rewriting, via `memcache.touch`), and
`destroy(mc, id)`. Volatile by nature (memcached evicts under pressure);
that trade-off is documented. A PHP-session-compatible key / serialization
layout - so a Jennifer app and a PHP app could share a session - is noted as
a follow-on, not built here (PHP uses its own serialize format, not JSON).
Prereq: M18.6 `memcache` (+ `uuid`, `json`, both shipped). Full discipline:
`modules/session_test.j` overlay (the JSON round-trip and key building as
pure helpers; the networked create / load / save / destroy round-trip
against an in-process memcached fake in a Go test), `docs/modules/session.md`,
catalog + `SUMMARY` + `README` + `JENNIFER.md` entries, and
`examples/modules/session_demo.j`.

#### M18.6.2 - `ratelimit` module (on `memcache`)

**Done.** A `ratelimit` module (`modules/ratelimit.j`) on `memcache`:
`allow(mc, key, limit, window)` records a hit with atomic `incr` and reports
whether it is within `limit` for the current window; `remaining(mc, key,
limit)` reports the budget left. The window starts at the first hit - an absent
counter is created via `add` carrying the window's TTL, and since a later
`incr` does not re-arm the expiry, the counter dies exactly `window` seconds
later, a clean fixed window with nothing to reap. The `incr`-then-`add` pair
closes the create race (only one `add` wins; the loser re-`incr`s, so no hit is
lost). Tested: the budget arithmetic (`withinLimit`, `remainingFrom` incl. the
clamp-at-0) in the overlay (`modules/ratelimit_test.j`, 100%); the full
allow / deny / remaining path over the window, and per-key independence, against
an in-process memcached server in the Go suite (`TestRatelimit`). Fixed window
only (sliding window / token bucket deferred). Reference doc
[docs/modules/ratelimit.md](modules/ratelimit.md); demo
`examples/modules/ratelimit_demo.j`.

A fixed-window rate limiter on `memcache` - the sharpest demonstration of
memcached's distinctive strength, **atomic `incr` + TTL**. `allow` does
`INCR key`; when the counter is newly 1 it arms the window expiry
(`exptime` = the window), and the call denies once the count passes the
limit; the key expires on its own at the window's end, so there is nothing
to reap. Surface over an existing `memcache` connection: `allow(mc, key,
limit, window) -> bool` and `remaining(mc, key, limit) -> int`. Small by
design (its value is showing *why* one reaches for memcached over a plain
map), entirely real (API throttling, login-attempt caps). A fixed window is
the honest scope; sliding-window / token-bucket variants are a later add.
Prereq: M18.6 `memcache`. Full discipline: `modules/ratelimit_test.j`
overlay (the incr / window / deny logic against an in-process memcached fake
in a Go test; pure helpers for the arithmetic), `docs/modules/ratelimit.md`,
catalog + `SUMMARY` + `README` + `JENNIFER.md` entries, and
`examples/modules/ratelimit_demo.j`.

### M18.7 - `http` module

**Done.** An `http` module (`modules/http.j`): an HTTP/1.1 client over `net`.
`request(method, url, headers, body)` - method-agnostic, plus `get` / `post` /
`put` / `patch` / `delete` / `head` / `options` shortcuts and a
case-insensitive `header` reader - returns a `Response`
(`status`, `statusText`, lowercased `headers` map, `body`). `http://` connects
in the clear, `https://` via `net.connectTLS` ([M16.14](#m1614---net-tls)). It
sends `Connection: close` and reads the whole response, decoding **both**
framings - Content-Length and Transfer-Encoding: chunked - at the byte level, so
the body is byte-exact: the complete body bytes are de-chunked / length-trimmed
and decoded as one UTF-8 unit (text bodies round-trip exactly; a binary body
raises rather than corrupts - a `bytes` accessor is a follow-on). A non-2xx
status is a normal `Response`, not an error; redirects are returned, not
followed. Tested: URL parsing, request building, and response parsing including
chunked decoding in the overlay (`modules/http_test.j`, 100%); the full GET /
JSON / POST-with-body-and-headers / chunked / 404 path against a real in-process
`net/http` server in the Go suite (`TestHttpClient`). Groups with the `httpd`
server ([M18.9](#m189---httpd)), which can share this request / response
parsing. Reference doc [docs/modules/http.md](modules/http.md); demo
`examples/modules/http_demo.j`.

### M18.7.1 - `gotify` module on top of `http` module

**Done.** A `gotify` module (`modules/gotify.j`) on top of `http`: a
value-semantic `Config{url, token}` and `push(cfg, title, message, priority)`
that POSTs the message form (`application/x-www-form-urlencoded`) to
`cfg.url + "/message"` with an `X-Gotify-Key` header and returns the raw
`http.Response` - a 200 on success, a bad token surfaced as a 4xx value rather
than a crash. Stateless / declarations-only (the caller holds the `Config`, no
module state). Includes a per-byte `application/x-www-form-urlencoded` encoder
(unreserved literal, space -> `+`, else `%XX` over UTF-8). Tested: the URL /
form encoding (incl. reserved characters and UTF-8) in the overlay
(`modules/gotify_test.j`, 100%); the full push against a fake Gotify server in
the Go suite (`TestGotifyPush`) - header, form round-trip, 200, and 401 on a bad
token. The demo reads `GOTIFY_URL` / `GOTIFY_TOKEN` from the environment (never
committed). Reference doc [docs/modules/gotify.md](modules/gotify.md); demo
`examples/modules/gotify_demo.j`.

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

**Done.** A `rest` module (`modules/rest.j`): an ergonomic REST layer over
`http` + `json`, pure `.j` composition (no host capability of its own). A
value-semantic `Client{baseUrl, headers}` plus verbs `get` / `delete`
(with a query map) and `post` / `put` / `patch` (raw body), each returning a
`rest.Response{status, headers, body}`; JSON wrappers `getJson` (-> `json.Value`)
/ `postJson` / `putJson` / `patchJson` encode / decode through `json` and set
`Content-Type`. Base-URL joining collapses to one slash (no double slashes),
query strings are percent-encoded from a map, and auth is a header -
`bearer(token)` / `basic(user, pass)` (base64 via `encoding`) / `withHeader`.
Stateless / declarations-only: the caller threads the `Client`, auth in its
headers; a stateful cookie-jar session stays deferred to `http` (a module holds
no mutable handle). A 4xx / 5xx is a `Response` value, not a crash (`getJson`
does throw on a non-JSON body). Tested: URL joining, query building / encoding,
and Bearer / Basic auth in the overlay (`modules/rest_test.j`, 100%); a full
CRUD round-trip (`postJson` create -> `getJson` read -> `putJson` / `patchJson`
update -> `delete` -> 404, a query-string GET, and 401 on a bad token) against
an in-process REST server in the Go suite (`TestRestCrud`). Reference doc
[docs/modules/rest.md](modules/rest.md); demo `examples/modules/rest_demo.j`.

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

**Done (first tier).** An `oauth` module (`modules/oauth.j`): a generic OAuth2
client over `http` + `json` + `time`, shipping the flows that need no extra
dependency - **Client Credentials**, **Refresh Token**, and the **Device
Authorization Grant** (`deviceStart` -> `deviceWait`, honouring `interval` /
`slow_down`, no local redirect server). `Config` presets `google` /
`microsoft` fill the endpoints; a `Token` carries `expiresAt` (from
`expires_in`) with `isExpired` (30s skew) and `refresh` preserving the refresh
token when the server omits it; `save` / `load` persist a token as JSON via
`fs`. Tokens feed `sasl.bearer` (XOAUTH2) so `oauth` + `sasl` + the mail
clients compose into Google / Microsoft 365 mail. A token-endpoint error
throws a catchable `Error` (kind `"oauth"`). Tested: the form encoder, token
parsing (with the clock injected), poll classification, and expiry check in
the overlay (`modules/oauth_test.j`, 100%); the full client-credentials /
device / refresh / error round-trip against a mock OAuth2 endpoint - and the
token driving `sasl` - in the Go suite (`TestOauthFlows`). Still deferred (as
planned, dependency-gated): **Authorization Code + PKCE** (needs `httpd` +
crypto-grade random) and the **service-account JWT assertion** (needs
`crypto`). Reference doc [docs/modules/oauth.md](modules/oauth.md); demo
`examples/modules/oauth_demo.j`.

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
- **Authorization Code + PKCE:** needs `httpd` (M18.9) to catch the redirect
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

### M18.8 - `toml` system library

**Done.** A Go **system library** (not a `.j` module), **designed like `json`**
([M16.9](#m169---json) + [M16.16](#m1616---jsonvalue)) end to end - the same
opaque-`Value` model and the same **read / walk / write** surface, syntax
name-for-name, so a program that reads, walks, and builds JSON does the same
with TOML. Why a library, not a module: TOML is parsed **char by char**
(belongs in Go - the `json` lesson, a per-byte parser in the interpreter is
too slow), and its opaque handle is a `KindObject`, which **only a Go
library can mint** (`interpreter.ObjectVal` / `RegisterNamespacedObject`),
never a `.j` module.

Surface, mirroring `json`:

- **`toml.decode(text) -> toml.Value`** - parse to an opaque `toml.Value`
  (a `KindObject`, the sibling of `json.Value`; `convert.typeOf` ->
  `"object"`, `convert.objectType` -> `"toml.Value"`).
- **Walk** - `(node, pointer)` accessors reported in `list` / `map`
  vocabulary, the **same names as `json`**: `toml.typeOf` / `get` / `has` /
  `keys` / `length` / `asInt` / `asFloat` / `asString` / `asBool`, plus
  **`toml.asDatetime`** (RFC 3339, backed by `time.Time`) for TOML's native
  date-times - the one type JSON lacks. The **path** is **JSON Pointer**
  (RFC 6901, `/server/ports/0`, `~0` / `~1` escaping): TOML has no
  standardized document-pointer of its own, and a dotted path would be
  ambiguous when a key itself contains a `.` (a quoted key `"a.b"`), so the
  addressing reuses `json`'s - identical `/`-syntax, so a program that walks
  JSON walks TOML unchanged.
- **Write** - a non-mutating write surface, fresh handle per call (strict /
  no-vivify, `-` end-marker), mirroring `json`'s: `toml.map` / `toml.list`
  constructors + `toml.set` / `insert` / `append` / `remove` / `move`.
- **`toml.encode(v)` / `toml.encodePretty(v) -> string`** - render a
  `toml.Value` (or a native `map` / `list` / scalar) back to TOML text, so
  encode / decode round-trips.

TOML's typed, section-oriented grammar - `[table]`, `[[array of tables]]`,
`key = value`, dotted keys, inline tables, multiline strings, date-times -
maps onto this model; the typed numbers and date-times are exactly why TOML
is the config format Jennifer ships (`yaml` / `xml` are the heavier M20
siblings, [M20.2](#m202---xml) / [M20.3](#m203---yaml)). Installed through
`internal/stdlib.InstallAll` like every library; TinyGo-clean (pure text,
no `net` / `os`). Sequenced here, before `httpd`
([M18.9](#m189---httpd)), because the server reads its configuration
from TOML - it is the one Go **system library** in the otherwise
Jennifer-coded M18 run, placed for that dependency (it also belongs to the
M20 data-format library family).

Discipline is the **library** shape, not the module shape:
`internal/lib/toml/` (parser + encoder + `toml.Value` object registration
via `RegisterNamespacedObject`), Go unit tests
(`internal/lib/toml/toml_test.go`), a row in `docs/libraries/index.md` and a
`docs/libraries/cheatsheet.md` row per builtin, and a reference doc
`docs/libraries/toml.md`. That doc must include a section **contrasting INI
with TOML** and stating plainly why `.ini` is not a supported format: INI
has **no real standard** (parsers disagree on comments, quoting, nesting),
is **flat** (one level of `[section]`, no arrays of tables or nested
tables), and is **untyped** (every value a bare string - no clear `int` /
`float` / `bool` / datetime). TOML fixes all three, so it is the one config
format Jennifer ships; `.ini` stays out (documented, not silently missing),
a deferred tiny cousin only if real demand surfaces.

### M18.9 - `httpd`

**Status:** done (both sub-milestones shipped). The point where Jennifer
becomes useful for *serving* content. It splits
into a Go **engine** and a Jennifer **framework**, mirroring the split we
already run twice - `testing` system-primitive + `.j` test framework, and
`net` system-primitive + the `http` / `smtp` / `imap` client modules. The
engine is a system library because parsing HTTP per request in the
tree-walker is the wrong place for the hot path; Go's `net/http` already
handles keep-alive, chunked transfer, TLS / HTTP-2, timeouts, and graceful
shutdown correctly and fast. (The *client* `http.j` stays a `.j` module
over `net` - one response, low volume - for the same reason inverted: a
server multiplexes many concurrent requests and wants the Go engine.)
(Formerly the standalone M20.)

#### M18.9.1 - `httpd` system library (engine)

**Done.** A Go-backed HTTP/1.1 server library (`use httpd;`) wrapping `net/http`.
Build-tag split like `net`: the full engine on the default `jennifer`, a
friendly-error stub on `jennifer-tiny` (no netdev in TinyGo's runtime).
HTTP/2 comes for free over TLS from `net/http`; plaintext stays HTTP/1.1.

**Pull-loop request model.** Jennifer has no first-class functions, so a
handler cannot be a callback handed to Go. Instead the engine accepts and
parses concurrently on Go's side and hands the interpreter one request at
a time to process:

```jennifer
use httpd;
def srv init httpd.listen(":8080");
while (true) {
    def req init httpd.accept($srv);          # blocks for the next request
    httpd.respond($req, 200, "hello\n");       # sends the response
}
```

This keeps the two concurrency worlds cleanly separated: Go owns the I/O
concurrency (accept, parse, keep-alive), and the `.j` side opts into
parallelism with its *own* `spawn` value-semantics snapshot when it wants
per-request concurrency - the interpreter is never re-entered from a Go
handler goroutine, so none of the spawn-snapshot machinery is at risk. A
callback / route-registration engine was **rejected** for exactly this
reason (record it in `docs/technical/rejected.md`).

Surface (indicative): `httpd.listen` / `httpd.listenTLS(addr, cert, key)`
-> a `httpd.Server` handle; `httpd.accept($srv) -> httpd.Request`;
`httpd.Request` accessors (method, path, query, headers, body `bytes`,
remote address); `httpd.respond($req, status, body)` and a
header/streaming form; `httpd.serveFile` / `httpd.serveDir` over `fs`;
`httpd.shutdown($srv)` for graceful drain; read/idle/write timeout and
max-body knobs. Handles use the integer-into-a-registry pattern (shared
state across copies) like `fs` / `net` / `os.Process`.

#### M18.9.2 - `web` framework module + `jennifer serve`

**Done.** The dispatch-by-name that module isolation otherwise blocks is
granted by a new, explicit reflection capability: `meta.callMain` /
`meta.definedMain` resolve against the **entry program's** methods (backed by
an `Interpreter.host` reference set in `loadModule` and `CallHostWith`, which
retags a module's own struct args across the boundary). So `web.run($app,
addr)` owns the accept loop and dispatches a matched route to a handler the
application defined, passing it a `web.Context`.

The ergonomic "fun-to-have" layer: a `.j` module (`import "web.j";`) built
on the `httpd` engine, dogfooding the libraries we have shipped. Because it
is Jennifer code it does its own **name-based handler dispatch** with
`CallByNameWith` (the mechanism `testing.run` already uses), so the engine
never calls back into the interpreter - the framework pulls a request with
`httpd.accept`, matches it against registered routes, and invokes the named
handler method itself.

```jennifer
import "web.j";
def app init web.new();
web.get($app, "/users/:id", "showUser");     # dispatch by method name
web.use($app, "logRequests");                  # middleware by name
web.run($app, ":8080");
```

Surface: routing with path params (`/users/:id`) and method dispatch, a
middleware chain, a custom not-found handler, and a `web.Context` carrying
request accessors (`method` / `path` / `query` / `header` / `body` / `param`)
and response helpers (`text`, `sendJson` via `json`, `respond`, `setHeader`,
`serveFile`). `web.run($app, addr)` owns the accept loop (`web.serveOn($app,
srv)` to hold the server handle).

`web` stays deliberately **dependency-light**: it imports only `httpd`,
`meta`, `json`, and the collection libraries, so it forces no store or config
format on an app. The heavier pieces are **opt-in companions the app composes
in**, not things `web` itself wires: an app reads its own configuration with
[`toml`](#m188---toml-system-library), adds cookie-keyed server-side sessions
with [`session`](#m1861---session-module-on-memcache) (a `Set-Cookie` id whose
data lives in `memcache`), and throttles per client with
[`ratelimit`](#m1862---ratelimit-module-on-memcache) (`allow` check, `429` on
deny) - each imported by the app and used from a handler through the request.
So a real serving stack demonstrates config, sessions, and rate limiting end to
end without baking a `memcache` dependency into every `web` app.

**`jennifer serve app.j`** is the Hugo-style convenience: it boots a `web`
app from a file (with reload), the closest analog to `hugo server`. It
lives in the default binary alongside `run` (net-backed, so absent from
`jennifer-tiny`), and is the CLI face of the framework module.

### M18.10 - `flatdb` module

**Done.** A file-backed JSON document store as a `.j` module - the "embed a small
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

**Done.** Raspberry-Pi (and any Linux SBC) GPIO as a **pure `.j` module** over
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

**Done.** A doc-comment parser: read Jennifer source and return the documentation
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

### M18.13 - `mqtt` module (+ `net.setDeadline`)

**Done.** An MQTT client (pub/sub messaging over TCP / TLS), as a `.j` module over
`net` - the same "protocol clients are modules, `net` is the transport"
line the other network clients (`redis`, `smtp`, `imap`, `resque`) already
follow. MQTT is the most **binary** and most **asynchronous** protocol
attempted in `.j`, but it stays expressible: packets are a 1-byte type, a
varint remaining-length, then a length-prefixed payload, all built and
parsed with Jennifer's bitwise operators (`& | ^ ~ << >>`, int-only) and
`bytes` (`$b[] =` append, int-returning `$b[i]` reads) over
`net.readBytes` / `net.writeBytes` - the same bit-arithmetic `idna`
already does for Punycode. Two deliverables:

**`net.setDeadline($conn, ms)` (the `net` enabler, ships first).** A read
/ write deadline on a `net.Conn` (Go `SetDeadline`), already on the `net`
roadmap. It is what makes MQTT's asynchronous **subscribe** path clean: a
single-threaded poll-with-timeout loop (read a packet, or time out and
send a keepalive PINGREQ when idle) instead of leaning on a `spawn`ed
reader / pinger. Part of `net`'s `!tinygo` build (a friendly stub on
`jennifer-tiny`, like the rest of `net`); a positioned error on a closed
conn; a timed-out read is a distinguishable, catchable outcome, not a
crash. Documented in [net.md](libraries/net.md) (retiring the "Timeouts /
deadlines: compose with `spawn` for a first cut" deferral note).

**`mqtt.j` (the module).** Basics-first, MQTT 3.1.1: `connect` (CONNECT /
CONNACK, keepalive, optional `mqtts` via `net.connectTLS`), `publish`
(QoS 0 - a synchronous build-and-write), `subscribe` + `poll` / `receive`
(QoS 0, dispatch on topic in user code), `ping`, `disconnect`. A received
message is a `Message { topic, payload }`; publish payloads are `bytes` /
`string`. Deferred to a later pass (kept out of the basics): QoS 1/2
handshakes (PUBACK / PUBREC / PUBREL / PUBCOMP with persistent packet-ID
state), retained messages and the will, auto-reconnect / session
resumption, and MQTT 5 properties. If full QoS 1/2 + high-throughput
processing ever makes the tree-walker the bottleneck, a hand-rolled Go
library (build-tag split like `net`, since there is no stdlib MQTT and the
project avoids third-party deps) is the fallback - but the pub/sub basics
belong in a module. Prereq: `net.setDeadline` (above). Discipline as
usual: `modules/mqtt_test.j` overlay (packet encode / decode - the varint
remaining-length, CONNECT / PUBLISH / SUBSCRIBE framing - as pure helpers;
the networked connect / publish / subscribe round-trip against an
in-process MQTT-broker fake in a Go test), `docs/modules/mqtt.md`, a
catalog row, a `SUMMARY.md` entry, a `modules/README.md` entry, a
`JENNIFER.md` bullet, and `examples/modules/mqtt_demo.j`.

### M18.14 - `prometheus` module (metrics)

A `prometheus` module in two halves with different prerequisites, so the
first can land well before the second. Both are text / HTTP orchestration
over existing capabilities - a module, like the other format and client
modules.

#### M18.14.1 - exposition (produce metrics)

**No blockers - buildable now.** The Prometheus text exposition format is
pure text (`# HELP name text`, `# TYPE name counter`, then
`name{label="value"} value [timestamp]` sample lines), which a `.j` module
renders directly. Deliberately **transport-agnostic**: the module builds a
metric set and renders the string; how it reaches Prometheus is the
caller's choice, and two of the three routes need nothing new:

- **node_exporter textfile collector** - render and write `*.prom` files to
  a directory (via `fs`) for node_exporter to pick up. The "provide data
  for nodes" path, buildable today.
- **Pushgateway** - POST the rendered text (needs the `http` client,
  M18.7).
- **A `/metrics` scrape endpoint** - serve the rendered text from an HTTP
  handler (needs `httpd`, M18.9). The module stays server-agnostic.

Surface: a `Metric { name, help, type, samples }` and a
`Sample { labels as map of string to string, value as float }`; builders
`counter(name, help)` / `gauge(name, help)`, `observe(metric, labels,
value)` (returns a new metric, value-semantic), and `render(metrics ->
list of Metric) -> string`. Strict where the format is strict: a metric
name must match `[a-zA-Z_:][a-zA-Z0-9_:]*`, label values escape `\`, `"`,
and newline, and `# HELP` text escapes `\` and newline. Scope: `counter`
and `gauge` first (the two that cover most custom metrics); `histogram`
and `summary` (buckets / quantiles, the `_bucket` / `_sum` / `_count`
child series) are a documented follow-on. Pure `.j` over `strings` /
`maps` / `convert`; TinyGo-clean. Prereq: none.

#### M18.14.2 - retrieval (query metrics)

A read client for Prometheus's HTTP query API: `query(base, promql)`
(instant, `/api/v1/query`) and `queryRange(base, promql, start, end, step)`
(`/api/v1/query_range`), returning the parsed result set (`resultType` plus
`metric` label maps and `value` / `values` samples). HTTP GET + JSON parse,
so it is built on the `http` module (M18.7) + `json` - it cannot land
before `http`. Prereq: M18.7 `http`.

Each sub-milestone ships the usual discipline: a `*_test.j` overlay (the
exposition renderer's format / escaping as pure helpers; the query client's
JSON-result parsing against a canned response, with the networked path in a
Go test once `http` exists), `docs/modules/prometheus.md`, a catalog row, a
`SUMMARY.md` entry, a `modules/README.md` entry, a `JENNIFER.md` bullet, and
a runnable `examples/modules/prometheus_demo.j` (textfile-collector output
for .1).

### M18.15 - `label` module (label printing)

Print to industrial label printers. **One** module - one way to describe and
print a label (design stance 1) - with the printer **language as a
selectable backend**, not a module per language. A device-independent
`Label` is built once, then rendered / printed in a chosen dialect; adding a
dialect is a new encoder plus a selector string, with no change to the build
API, so the module extends to further printer languages later without
fragmenting the surface.

A deliberate **three-stage pipeline** - build, render, emit - each stage a
plain value handed to the next, so any stage can be swapped independently:

**Stage 1 - build (device-independent).** A `Label` is a physical
description in **millimetres** (printer-independent), not device dots:
`label.new(width, height)` then value-semantic field builders `text(label,
x, y, opts, content)`, `barcode(label, x, y, type, data)` (Code 128 / EAN /
QR / Interleaved 2 of 5 (ITF) - the main reason these printers exist), `box(label, x, y, w, h,
thickness)`, and `quantity(label, n)`. ITF is numeric-only and even-length
(the interleaving pairs digits), so the encoder pads / rejects odd input; it
is the standard shipping-carton symbology (ITF-14). Each returns a new `Label`
(value semantics, like the other builders).

**Stage 2 - render (to the target language).** `render(label, device) ->
string` turns the label into the command stream for a chosen dialect,
returning it as a plain value. A `Device { dialect, dpi }` names the dialect
(`"zpl"` / `"cab"`) and, for raster languages, the printer `dpi` used to
convert millimetres to dots (cab JScript is millimetre-native and ignores
it); `dpi` and any future device settings (darkness, speed) live here, not
mixed with a destination. Render is pure text - TinyGo-clean, no `net`.

**Stage 3 - emit (transport-agnostic).** The rendered string is yours to
send anywhere: over `net` to the raw print port (`:9100`, JetDirect / RAW),
written to a file or a USB device node via `fs`, stored in a database, or
dumped to stdout for inspection. The module ships only the thin common-case
convenience `send(host, port, rendered)` (the `net` `:9100` path, so it is
the sole part that needs `net`); every other destination is just the caller
using the string. Keeping emit out of render is what makes the same label
printable, saveable, and testable without a printer attached.

**Dialects.**

- **`"zpl"` (Zebra Programming Language) - the first backend, buildable
  now.** ZPL II is a public, stable, widely-used standard (the dominant
  label language), and cab Squix printers support it too - so this one
  dialect already drives cab hardware as well as Zebra and the rest.
  Encodes the caret / tilde stream - `^XA` start, `^FO` origin, `^A0` font,
  `^FD`/`^FS` data, `^BY`/`^BC` (with `^BQ` for QR and `^B2` for
  Interleaved 2 of 5 / ITF) barcodes, `^GB` box, `^PQ` quantity, `^XZ` end -
  converting millimetres to dots via the target dpi. From the public ZPL II
  reference.
- **`"cab"` (cab JScript) - the second backend (M18.15.1).** The native
  language of cab printers (cab is a German industrial-printer maker).
  The dialect string is `"cab"`, not `"jscript"` (which reads as JavaScript);
  it emits cab's JScript. From cab's *JScript Programming Manual*:
  millimetre units, `J` new label, `S` size, `T` text,  `B` barcode,
  `G` graphics, `A` print (quantity). Lands as a second encoder once the
  build API and dialect dispatch are proven on ZPL - no build-API change.

Out of scope: Brother's ESC/P is raster / bitmap rather than a field
command language, so it does not fit this vector-field model and is not a
planned dialect.

Discipline: a `modules/label_test.j` overlay (each dialect's exact command
output and the mm-to-dots conversion / escaping as pure helpers; the `:9100`
send captured by an in-process fake printer in a Go test),
`docs/modules/label.md`, a catalog row, a `SUMMARY.md` entry, a
`modules/README.md` entry, a `JENNIFER.md` bullet, and a runnable
`examples/modules/label_demo.j` (rendering the same label in both dialects).
Prereq: none for the module + ZPL dialect (pure `.j`; `print` uses `net`);
the cab dialect (M18.15.1) is a follow-on encoder.

### M18.16 - `web` framework enhancements: cookies and sessions

`httpd` deliberately owns no routing, cookies, or sessions - those live in the
`web` module above it. `web` already ships routing, `:param` capture, a
middleware chain, a custom not-found handler, and a `web.Context`, but
**cookies and sessions are not yet implemented** - this milestone closes that
gap so the framework matches what the layer above the engine is described as
providing. `web` stays **dependency-light** and keeps **one way to do each
thing** with a **selectable backend** where a real choice exists (design
stance 1).

#### M18.16.1 - Cookies

First-class cookie access on the `web.Context`, pure HTTP-header work over
`httpd` (no new dependency):

- `web.cookie(ctx, name) -> string` parses the request `Cookie` header and
  returns the named value ("" if absent).
- `web.setCookie(ctx, name, value, opts)` emits a `Set-Cookie` response header.
  `opts` is a `web.CookieOptions` struct - `path`, `domain`, `maxAge` (int
  seconds; `0` omits the attribute, a negative value expires the cookie now),
  `httpOnly`, `secure`, `sameSite` (`"Lax"` / `"Strict"` / `"None"` / `""`). A
  zero-value `CookieOptions` is the common case (a session cookie scoped to the
  current path).

Cookie parse / format are pure `.j` helpers (header splitting, attribute
rendering, value quoting), unit-tested in the overlay without a live server.

#### M18.16.2 - Sessions

Cookie-keyed sessions built on M18.16.1. A module top level is
declarations-only, so `web` cannot hold mutable session state itself: it
manages the session-**id** cookie and reads / writes the data through a
**store handle** the app supplies - the integer-handle-into-a-registry pattern,
whose state is shared across value copies, carried on the `App`. The store is a
**selectable backend**:

- **`session` / `memcache`** (opt-in, the reference backend) - the existing
  [`session`](#m1861---session-module-on-memcache) module, for multi-process
  serving. Pulled in only when the app opts into sessions, so a `web` app that
  never touches them keeps its dependency surface at `httpd` + `meta` + `json` +
  the collection libraries.

Surface:

- `web.withSessions(app, store, cookieName) -> App` registers a store on the
  App and names the id cookie (default `"sid"`).
- `web.session(ctx) -> session handle` returns the current request's session -
  minting a new id plus a `Set-Cookie` on first use, resolving the existing id
  cookie otherwise - then the handler reads / writes through the `session`
  module's own verbs. Calling it on an App with no store registered is a
  positioned `web`-kind error with a "call web.withSessions first" hint.

Stateless signed-cookie sessions (the data in the cookie, no server store) need
real HMAC and so wait on the `crypto` library - parked in
[horizon.md](horizon.md), not this milestone.

Middleware already ships (`web.before` + the chain); this milestone leaves it
as is. Discipline: extend `modules/web_test.j` (the cookie parse / format
helpers and the session-id round trip against an in-process fake store),
refresh `docs/modules/web.md` and `examples/modules/web_demo.j` (a
login / counter-in-session demo), and update the `httpd.md` scope note and the
`JENNIFER.md` `web` bullet. Prereq: M18.9.2 (`web`) and, for the memcache
backend, M18.6.1 (`session`).

## M19 - cross-cutting tooling

The catch-all bucket for milestones that improve the interpreter or its
tooling but belong to neither the Jennifer-coded modules of M18 nor the Go
system libraries of M20. Numbered sub-entries land here as needs arise.
M19.1-M19.5 are a correctness / performance hardening pass over the
interpreter core and libraries (races, dead code, algorithmic complexity,
resource bounds, module identity); M19.6 is the coverage tool. None of the
hardening work needs `reflect` or breaks TinyGo-cleanliness. The smallest,
localized crash / correctness fixes (out-of-range `httpd.respond` status,
truncated-toml-date-time panic, `io.sprintf("%d", MinInt64)`, `math.randInt`
span overflow, `floorDiv` large-quotient garbage, the constant-folder's
above-2^53 comparison divergence, the missing stream-registry mutexes, the
`TaskState.Observed` atomic, the numeric-conversion and read-length caps)
land as they surface rather than waiting on a milestone; each ships with a
regression test.

### M19.1 - Interpreter concurrency-safety

Two data races in the interpreter core that the race detector catches and that
can crash a program using nested `spawn`. Both are small, localized fixes plus
a `go test -race` nested-spawn stress test.

- **Nested-spawn global snapshot race.** `snapshotForSpawn`
  (`internal/interpreter/interpreter.go`) always iterates `i.global.vars` - the
  live top-level frame - regardless of which goroutine launches the `spawn`. A
  `spawn` nested inside a `spawn` body snapshots on a background goroutine while
  the main goroutine keeps defining / assigning globals: Go's fatal "concurrent
  map iteration and map write" (uncatchable, takes the interpreter down).
  Snapshot from `effectiveGlobal(env)` (the launching goroutine's own root
  frame - inside a task, the outer spawn's snapshot) instead of the live
  `i.global`. This is also more correct: a nested spawn captures its enclosing
  scope, not the main goroutine's live globals.
- **Runtime AST mutation.** `resolveDeclaredStructNS`
  (`internal/interpreter/module.go`) writes `t.StructNS = <canonical>` into the
  shared AST node every time a `def x as alias.Struct;` statement executes (once
  per loop iteration, say). The same `def` run concurrently from a spawn body
  and the main goroutine is a write-write race on the AST node. Resolve declared
  module-struct types once, at load / resolve time, not per execution.

**Acceptance.** A nested-spawn stress program that mutates globals on the main
goroutine while inner spawns launch runs clean under `go test -race`; a
`def alias.Struct` in a loop body inside a spawn is race-free.

### M19.2 - Value representation cleanup

The copy-on-write marker protocol added for the append-in-a-loop optimization
is **inert**: `Value.Share()` has a value receiver, and `Environment.Get` /
`GetAt` return the binding's `Value` by value, so the `shared` flag is set on a
throwaway copy and never reaches the stored binding. `Ensure()` / `ensureCOW`
therefore never detach (the whole Share / Ensure machinery is dead code), and
every read of a compound variable heap-allocates a fresh `*bool` that goes
nowhere - pure overhead in hot loops. Correctness never depended on the
protocol: it rests entirely on the eager deep copies at every binding site
(`def` / assignment / parameter bind) and on builtins copying before they
mutate.

- **Delete the dead protocol.** Remove `Share()` / `Ensure()` / `ensureCOW`,
  the `Value.shared` field, and the per-read `Share()` call, and document the
  eager-copy invariant that actually provides value semantics. The write-through
  alternative (store `*Binding` so the marker propagates and mutation sites
  genuinely detach) was considered and rejected - aliasing-heavy code is rare
  precisely because we eager-copy, so the complexity buys little; record it in
  `docs/technical/rejected.md`.
- **Stop double-copying literals.** `evalListLit` / `evalMapLit` `Copy()` every
  element of a freshly-built literal, then `execDefine` / `execAssign` eager-copy
  the whole result again. A fresh literal (or a call result) cannot alias a
  binding - only Var / Index / Field reads can - so the binding site can skip the
  copy for non-aliasing RHS shapes, removing two full deep copies per literal
  binding.

Shrinking `Value` itself (moving the compound payload behind one pointer so the
scalar case stays small) is **deferred**: a large cross-cutting churn, and the
map hash index (M19.3) is the bigger algorithmic win. Revisit only if benchmarks
still show `Value` copying dominating after M19.3.

**Acceptance.** The Share / Ensure machinery and the `shared` field are gone,
the alias-stress tests (`value_alias_test.go`) still pass (value semantics
intact), and a compound-var read in a hot loop no longer allocates; a literal
`def` binding does one deep copy, not three.

### M19.3 - Runtime performance: maps and the call / loop hot path

The biggest algorithmic issue in the runtime plus the call- and loop-overhead
batch. None need `reflect` or break TinyGo-cleanliness.

- **Maps as association lists (biggest win).** `Value.Map` is a `[]MapEntry`;
  index reads (`indexInto`) and writes (`writeIndexedSlot`) linear-scan with a
  recursive `Value.Equal` per entry, and map equality is O(n*m) - building a map
  with `$m[$k] = $v` in a loop is quadratic. Maintain a side hash index
  (`map[string]int` over a canonical scalar-key encoding) alongside the ordered
  slice (insertion order stays a language guarantee), falling back to the linear
  scan for the rare non-hashable key.
- **Call / loop hot path.** `execForEach` allocates a fresh `NewEnvironmentSized`
  (new name map) per iteration and `execFor` an unpooled header env - both should
  borrow from `envPool` like `execBlock`. `DefineAt` re-runs `existsInChain` and
  mirrors every binding into `e.vars` on the resolver-verified slot fast path
  (skip the chain walk when `slot >= 0`, defer the name-map mirror to rare name
  lookups). `Run` builds `i.global` without pre-sizing from `prog.NumGlobals`,
  growing the slot slice one-at-a-time (O(n^2) for n globals). `execIndexAssign` /
  `execAppend` / `execFieldAssign` re-fetch the root binding by name though the
  root `VarExpr` already carries `(Depth, Slot)`. `lists.reverse` / `head` /
  `tail` / `slice` / `concat` deep-copy the whole argument then immediately
  replace the copied slice - the first copy is pure waste.

**Acceptance.** A map-heavy build (`$m[$k] = $v` over N keys) is near-linear, not
quadratic; a call-heavy and a loop-heavy benchmark improve measurably on the
reference machine; every existing test (incl. `value_alias_test.go` and the map
ordering tests) still passes.

### M19.4 - Resource lifecycle and numeric strictness

- **`os.spawn` handle lifecycle.** `internal/lib/os/exec.go` keys process
  handles by OS PID and never deletes them: every `processState` (with buffered
  stdout / stderr) is retained for the interpreter's life, and after the reaper
  `Wait()`s a process the freed PID can be reused, so a later `os.spawn`
  overwrites the entry and `os.wait` / `poll` / `kill` on the old handle hits the
  wrong process. Key handles by a monotonic internal id (like `net` / `fs` /
  `httpd`) and delete on reap.
- **Numeric strictness.** `convert.toInt` and `math.floor` / `ceil` / `round`
  do an unchecked `int64(v.Float)`, platform-defined garbage for NaN, Inf, and
  out-of-int64-range values - contradicting the `math` library's documented
  strict stance (mathematically-undefined results error, never yield garbage).
  Reject NaN / Inf / out-of-range with positioned errors; special-case
  `math.abs(MinInt64)` (currently returns a negative). The toml decoder degrades
  an integer literal past int64 to a lossy float - TOML 1.0 says integer overflow
  is an error, so make it one (json keeps its deliberate lossy-float fallback).
- **Most-negative int literal.** `-9223372036854775808` (and
  `0x8000000000000000`) is a parse error because the magnitude is parsed with
  `ParseInt` before the unary minus applies. Parse magnitudes with `ParseUint`
  and range-check against 2^63 under a leading minus.

The uncapped-allocation issues (caller-supplied `make([]byte, n)` in `net` /
`fs` reads, and unbounded `io.ReadAll` of decompressed `compress` / `archive`
streams - a zip-bomb sink) are fixed **immediately**, with fixed sensible
defaults mirroring httpd's `maxBodyBytes`, ahead of this milestone.

**Acceptance.** A recycled-PID scenario signals the right process (or errors)
and the handle table does not grow without bound; `convert.toInt(NaN)` /
`math.floor(1e300)` error; `math.abs(MinInt64)` errors; `-9223372036854775808`
parses; a too-large toml integer is a decode error.

### M19.5 - Module struct identity: reject stem collisions

Module struct values are tagged by the imported file's **stem** (`moduleStem`),
so two modules whose files share a stem
(`import "a/util.j" as u1; import "b/util.j" as u2;`) produce values with
identical `(namespace, name)` identity, and `moduleByNS` resolves a stem to
whichever module Go's nondeterministic map iteration hits first - a same-named
struct from an unrelated module passes the other's type check. **Detect stem
collisions at import time and error** (the "fail loud" choice), rather than
re-keying struct identity by canonical path (a larger change kept in reserve;
record it in `docs/technical/rejected.md`).

**Acceptance.** Importing two modules with the same file stem is a positioned
error at load; single-stem programs are unaffected; the module test suite still
passes.

### M19.6 - `.j` code coverage

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

### M20.5 - `term`

A `term` system library exposing the terminal host capabilities a TUI needs
and pure `.j` cannot reach: **raw mode** (`makeRaw` / `restore` - unbuffered,
no-echo input), **terminal size** (`size -> rows, cols`), and raw single-key
reads from stdin. It reuses `golang.org/x/term` - already a repository
dependency scoped to the REPL's line editor (`cmd/jennifer/lineedit.go`) - so
it largely exposes a capability the interpreter already exercises. Build-tag
split like `net` / `os`: a friendly-error stub on `jennifer-tiny` (embedded /
minimal targets may have no controlling TTY). This is the **enabler for
interactive TUIs**; the pure-ANSI screen control, key decoding, and rendering
sit in the [M21.1](#m211---screen--tui-module) `screen` / `tui` module on top.
Output-only TUIs (dashboards, progress bars) need neither this library nor raw
mode - just `ansi` + `os.isTerminal`.

## M21 - general backlog (catch-all)

The general holding area for milestones that fit no other bucket - not a
Jennifer-coded module (M18), not interpreter / tooling work (M19), not a Go
system library (M20), and not a beyond-1.0.0 idea (embedding, WASM, and the
rest live in the [horizon collection](horizon.md)). It is the top-level
counterpart to M19's tooling bucket:
anything worth recording that has no natural home lands here as a numbered
sub-entry, and graduates out into its own bucket once a cluster grows enough to
deserve one.

### M21.1 - `screen` / `tui` module

Jennifer's terminal-UI answer - a `.j` module for terminal user interfaces,
since there is no GUI medium-term, so the terminal is the interactive surface.
Layered so the output-only subset ships with no host dependency:

- **Stage 1 (no prerequisite).** Pure-ANSI screen control over `ansi`: cursor
  movement, clear, alternate-screen buffer, hide / show cursor, box-drawing,
  and a screen buffer + render / diff loop. Enables **output-only TUIs** - live
  dashboards, progress bars, spinners, self-updating tables (the `rich`-style
  subset). Pure strings; TinyGo-clean.
- **Stage 2 (needs [M20.5](#m205---term)).** A key-event decoder (parse the raw
  byte stream - `ESC [ A` -> Up, and so on) plus an event loop over `term`'s
  raw mode / size / key reads. Enables **interactive TUIs** - menus, forms, key
  navigation (the `curses` / `bubbletea` subset).

Positioned as an *explicit* terminal UI, not a GUI framework. Named `screen`
or `tui` (settled at build time). Parked in the general backlog for now; it can
graduate into the M18 module track when built.

### M21.2 - `feed` module (RSS + Atom)

A `feed` module for web syndication feeds: **build and parse** both RSS 2.0
and Atom 1.0. **One** module, format selected on build and detected on parse
(design stance 1 - not separate `rss` / `atom` modules); a feed is a value-
semantic `Feed` (title, link, updated, entries) of `Entry` (title, link, id,
published / updated, summary, content). It composes across the stack: `http`
fetches a feed, `feed` parses it, `time` handles the RFC 3339 / RFC 822 dates -
a feed reader, podcast client, news aggregator, or changelog-to-feed generator
in a few lines.

Parked here because it is **gated on [M20.2 `xml`](#m202---xml)**: RSS / Atom
parsing needs a real XML parser (entities, CDATA, and Atom's XML namespaces),
which is exactly why `xml` is a system library rather than a hand-rolled `.j`
scanner - so feed *parsing* rides it. Feed *building* (emitting escaped XML)
could predate `xml`, but build and parse ship together on the same layer. Also
uses `http` (fetch) and `time` (dates); pure `.j` otherwise. It graduates into
the M18 module track once `xml` lands. Discipline as usual: a
`modules/feed_test.j` overlay (build / parse round-trips and date handling for
both formats as pure helpers; a networked fetch-and-parse against an in-process
server in a Go test), `docs/modules/feed.md`, a catalog row, a `SUMMARY.md`
entry, a `modules/README.md` entry, a `JENNIFER.md` bullet, and a runnable
`examples/modules/feed_demo.j`.

---

## Requirements for 1.0.0 stable

The core CI + release + packaging items that used to live here
were promoted into M15.8 (the last step before Phase C). What
stays here are the distribution requirements for a stable 1.0.0
that aren't themselves milestones - they can land any time and
don't block any feature milestone:

- **Cross-build for macOS / Windows.** Waits on the
  platform-portability work in the [horizon ideas](horizon.md); ships as
  soon as that lands.
- **Real apt repository** (replacing the "GitHub Release
  artifact" install of the M15.8 `.deb`) if user demand
  warrants the maintenance.

The extra Linux / macOS distribution formats (Homebrew, Snap,
Nix, Flatpak, AppImage, ...) are not requirements; they live in
the [horizon idea collection](horizon.md) and ship when there's
user demand and a maintainer willing to keep one green.

---

## Long horizon

Ideas for development *beyond* 1.0.0 - embedding, a WASM runtime,
specialised-domain libraries, and a grab-bag of smaller possibilities -
live in their own collection, kept out of the near-term plan so this file
stays focused on the road to 1.0.0. See the
[beyond-1.0.0 idea collection](horizon.md).
