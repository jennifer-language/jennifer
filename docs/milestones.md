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

### M18.1-M18.26 - shipped modules (compacted)

**All done.** Twenty-six sub-milestones (with their nested parts) shipped as
pure-Jennifer `modules/` (except where noted as a Go **system library**), each
with the standard discipline: a 100%-passing `*_test.j` overlay, a
`cmd/jennifer/*_test.go` integration test, a `docs/modules/*.md` reference, an
`examples/modules/*_demo.j`, and catalog / README / `JENNIFER.md` entries.
Per-function detail lives in [docs/modules/](modules/index.md); this table is
the milestone-number index (numbers were assigned in rough priority order).

| M#         | Module(s)               | Surface                                                                                          |
| ---------- | ----------------------- | ------------------------------------------------------------------------------------------------ |
| M18.1      | `csv`                   | RFC 4180 parse / format (+ `*With` for any delimiter), header-keyed `toRecords` / `fromRecords`. |
| M18.2      | `htmlwriter`            | build an HTML element tree and render escaped HTML5 (`element` / `text` / `raw` / `render`).     |
| M18.3      | `markdown`              | Markdown -> HTML.                                                                                 |
| M18.4.1/.7 | `mime`                  | RFC 5322 / 2045 message build + parse, incl. RFC 2047 encoded-words.                             |
| M18.4.2/.4 | `smtp` / `pop` / `imap` | mail send + POP3 / IMAP receive over `net` (plaintext / STARTTLS / implicit TLS).                |
| M18.4.5/.6 | `sasl` / `idna`         | SASL auth encoders (incl. XOAUTH2); Punycode / IDNA domains.                                     |
| M18.5      | `redis`                 | RESP2 client over `net`.                                                                          |
| M18.5.1    | `resque`                | Resque-wire-compatible background jobs on `redis`.                                                |
| M18.6      | `memcache`              | memcached text-protocol client over `net`.                                                        |
| M18.6.1/.2 | `session` / `ratelimit` | server-side sessions + fixed-window rate limiting on `memcache`.                                  |
| M18.7      | `http`                  | HTTP/1.1 client over `net` (`https://` via TLS).                                                  |
| M18.7.1/.3 | `gotify` / `rest` / `oauth` | push notifications; ergonomic REST layer; OAuth2 get-a-token - all on `http`.                 |
| M18.8      | `toml` (**library**) | RFC TOML 1.0 encode / decode; opaque `toml.Value`, JSON-Pointer walk. TinyGo-clean.              |
| M18.9.1    | `httpd` (**library**)| HTTP/1.1 server engine over `net/http`; pull-loop `accept` / `respond`.                          |
| M18.9.2    | `web` + `jennifer serve`| `.j` routing framework over `httpd` (routes by handler name, `:param`, middleware, `web.Context`), dispatched by `meta.callMain`; `serve` runs / `--watch`-reloads a program. |
| M18.10     | `flatdb`                | file-backed JSON document store over `json` + `fs`; JSON-Pointer query / edit; crash-atomic save.|
| M18.11     | `gpio`                  | Linux GPIO over the sysfs / character-device interface.                                          |
| M18.12     | `docblock`              | the Jennifer doc-comment format + parser (`FileDoc` tree, drift diagnostics).                    |
| M18.13     | `mqtt`                  | MQTT 3.1.1 pub / sub client over `net`.                                                           |
| M18.14     | `prometheus`            | metrics exposition (produce) + retrieval (query the HTTP API).                                   |
| M18.15     | `label`                 | industrial label printing: build / render (ZPL + cab JScript) / emit pipeline.                   |
| M18.16     | `web` cookies + sessions| cookie helpers + cookie-keyed sessions on the `web` framework.                                   |
| M18.17     | `totp`                  | RFC 6238 TOTP: `generate` / `verify` / `uri`. Over `hash.hmac` + `encoding` + `time`.            |
| M18.18     | `webhook`               | GitHub `X-Hub-Signature-256` HMAC `sign` / `verify` (pure) + `send` (over `http`).               |
| M18.19     | `bucket`                | S3-compatible object storage over `http` (AWS SigV4): `connect` / `get` / `put` / `delete` / `listObjects`. One module for AWS S3 + MinIO / R2 / B2. |
| M18.20     | `dotenv`                | `.env` config: `parse` / `read` / `load` (into env via `os.setEnv`). Over `fs` + `strings` + `os`. |
| M18.21     | `cron`                  | parse cron expressions; `next(schedule, after)` / `matches`. A calculator over `time`.           |
| M18.22     | `log`                   | leveled structured logging (`debug`..`error`; text / logfmt / json) to stdout / stderr / file / RFC 5424 syslog. |
| M18.23     | `ical`                  | iCalendar (RFC 5545) build + parse: a `Calendar` of `VEVENT`s, escaped + line-folded, dates through `time`. |
| M18.24     | `vcard`                 | vCard (RFC 6350) contacts build + parse; shares the content-line codec (`ical_vcard_shared.j`) with `ical`. |
| M18.25     | `jsonl`                 | JSON Lines (NDJSON): `encode` / `decode` + whole-file + streaming `Reader`, over `json` + `fs`.   |
| M18.26     | `ipnet`                 | IPv4 / IPv6 addresses + CIDR math: `parseAddress` / `toString` (RFC 5952) / `parse` / `contains` / `netmask` / `broadcast`. |

**Enabling changes** these modules pulled into the system side (each documented
under its library):

- **`net.setDeadline`** - a read/write deadline for socket timeouts (M18.13).
- **`io.eprintf`** - the stdout-`printf` twin that writes to stderr (a new
  `Interpreter.Err` / `BuiltinCtx.Err` writer), the stderr sink `log` builds on
  (M18.22).
- **`toml`** and **`httpd`** - two new Go **system libraries** (a char-by-char
  TOML parser and a `net/http` server engine both belong in Go, not a `.j`
  module); M18.8 / M18.9.1.
- **`meta.callMain` / `meta.definedMain`** - resolve a method against the entry
  program (retagging module-own struct args across the boundary), the capability
  the `web` framework dispatches handlers through (M18.9.2).
- **`hash.hmac`** (RFC 2104) and the **`sha512`** digest - the HMAC primitive
  `totp` / `webhook` build on (and that `jwt` / SigV4 will reuse).

### M18.27 - `ntp` module (network time)

**Done.** A simple SNTP client (RFC 4330 / 5905): `ntp.query(host) -> Result`
(port 123, 5s timeout) and `ntp.queryWith(address, timeoutMs) -> Result` over
`net` UDP. `Result` carries `serverTime as time.Time`, `offset as
time.Duration` (server minus local clock, from the four-timestamp SNTP formula),
and `delay as time.Duration` (round-trip). The 48-byte NTP packet is packed /
unpacked with `bytes` and the bitwise operators, and the NTP epoch (seconds
since 1900) converts through `time`. Query-only - it measures the offset and
hands it back, it does not discipline the clock or run as a daemon. Named `ntp`
(not `sntp`) because SNTP is wire-identical to NTP and `ntp` is the discoverable,
idiomatic name (cf. `beevik/ntp`, `ntplib`). Needs the default binary (`net`).

Enabling change (like `io.eprintf` before it): **`net.setDeadline` now accepts a
`net.UDPSocket`** as well as a `net.Conn` (a datagram `PacketConn` honours
`SetDeadline` too), so a `recvFrom` can be bounded by a timeout - the SNTP client
fails fast on a lost reply instead of hanging. Validated end to end against a
real NTP server (sub-millisecond LAN offset). No other new prereq.

### M18.28 - `statsd` module (metrics)

**Done.** A fire-and-forget StatsD client over `net` UDP. `statsd.client(host)`
(default port 8125) / `statsd.clientWith(address, prefix)` open a `Client`
(sending socket + agent address + optional metric-name prefix), then
`count` / `increment` / `decrement` (counter `c`), `gauge` (`g`), `timing`
(`ms`), and `set` (`s`) each format one `[prefix.]name:value|type` line and
send one datagram; `close` releases the socket. The push counterpart to the
pull-based `prometheus` - UDP means no reply and no error when no agent is
listening, so a metric costs one datagram and never blocks. Integer counter /
gauge values; sample rates and Datadog tags are deliberately out of this
version. The pure name / line formatting is unit-tested in the
`modules/statsd_test.j` overlay; the live wire output is verified end to end
against a real UDP loopback listener in `cmd/jennifer/statsd_test.go`. Needs the
default binary (`net`). No new prereq.

### M18.29 - `influxdb` module (time-series)

**Done.** An InfluxDB 1.x client over the `http` module. `client` /
`clientWith` open a `Client` (base URL + database + optional Basic auth). A
`Point` is built with value-semantic builders - `point` / `tag` / `field`
(float) / `intField` / `stringField` / `boolField` / `at` / `atTime` - each
returning a fresh point; field types ride as pre-rendered line-protocol
fragments so one point mixes float / int / string / bool despite Jennifer's
homogeneous maps. `line` renders one line-protocol line (throws if a point has
no fields), and `write(client, points)` posts them to `/write` at nanosecond
precision. `query(client, influxql) -> Result` runs InfluxQL against `/query`
and parses the tabular JSON into a `list of Series` (name, tag set, columns,
stringified rows) - the same read-a-parsed-result shape as `prometheus`'s
retrieval half. Line-protocol escaping is automatic; a request failure or a
per-statement query error throws `Error{kind: "influxdb"}`. Over `http` +
`json` + `time` + `encoding` (Basic-auth base64). The pure line-protocol
rendering and query-JSON parsing are unit-tested in the
`modules/influxdb_test.j` overlay; the live write / query round-trip (body,
auth header, URL encoding) is verified against a fake HTTP server in
`cmd/jennifer/influxdb_test.go`. Needs the default binary. Prereq: `http`
(M18.7).

### M18.30 - `slack` + `discord` modules (chat notifiers)

**Done.** Two incoming-webhook push clients, siblings of `gotify`, delivered
together as separate modules (distinct payload shapes). Each has a plain
`send(webhookUrl, text)` and a value-semantic rich-message builder that
assembles a JSON payload from pre-rendered fragments (so nested, heterogeneous
structures build under Jennifer's homogeneous maps), rendered by `render` and
posted by `sendMessage`:

- **`slack`** - `send` posts `{"text": ...}`; the builder assembles Block Kit
  blocks (`message` / `text` / `section` / `header` / `divider`). Slack answers
  200 "ok".
- **`discord`** - `send` posts `{"content": ...}`; the builder assembles embeds
  (`message` / `content` / `embed(m, title, description, color)`, `color` a
  decimal RGB int). Discord answers 204.

All text is JSON-escaped through the `json` library (`json.encode` on a string
value), so quotes / newlines are safe. Over `http` + `json`. The pure payload
rendering is unit-tested in `modules/slack_test.j` / `modules/discord_test.j`;
the live webhook POST bodies (plain + rich) are asserted against a fake HTTP
server in `cmd/jennifer/chatnotify_test.go`. Need the default binary. Prereq:
`http` (M18.7).

### M18.31 - `telegram` module (bot API)

**Done.** A Telegram Bot API client over the `http` module + `json`. `bot` /
`botWith` open a `Bot` (token + API base URL, the base overridable for a
self-hosted Bot API server or a test endpoint). Send verbs `sendMessage` /
`sendMessageWith` (parse mode) / `sendPhoto` (by URL or file id) /
`sendChatAction`, plus `getMe` for identity, each POST form-encoded params to
`baseUrl/bot<token>/<method>`, verify the `{"ok": true}` envelope (an API error
throws `Error{kind: "telegram"}` with the server description), and parse the
`result` into a struct (`Message` / `User`). `getUpdates(bot, offset, timeout)`
long-polls (HTTP read bounded a few seconds past `timeout`) and returns a `list
of Update` - the stateful receive loop the one-shot notifiers lack: the caller
advances `offset` past each `updateId` and checks `Update.hasMessage`. `chatId`
is a 64-bit `int` (channel ids are large / negative). The pure form encoding and
response parsing are unit-tested in the `modules/telegram_test.j` overlay; the
live getMe / sendMessage / getUpdates and the error-throw path are verified
against a fake Bot API server in `cmd/jennifer/telegram_test.go`. Needs the
default binary. Prereq: `http` (M18.7).

### M18.32 - `websocket` module (WebSocket client)

An RFC 6455 WebSocket **client** over `net`: the HTTP `Upgrade` handshake (the
`Sec-WebSocket-Accept` key is SHA-1 + base64, both in hand), then framed `send` /
`receive` with client-side masking, ping / pong, and close - binary framing over
`net` + `hash` (SHA-1) + `encoding` + bitwise. Needs the default binary. A
server-side upgrade would need an `httpd` connection-hijack hook (a separate,
larger piece). No new prereq.

### M18.33 - `amqp` module (RabbitMQ)

An AMQP 0-9-1 client over `net` (RabbitMQ and compatible brokers): the
connection / channel handshake, `publish`, and `consume`, with the binary
frame / method encoding built from `bytes` and bitwise ops. The **largest**
protocol module attempted - much bigger than `mqtt` - so a candidate to
reassess as a Go library if the tree-walker becomes the bottleneck. Needs the
default binary. No new prereq.

### M18.34 - `multipart` module (form-data)

Build and parse `multipart/form-data` (RFC 7578) bodies - the file-upload
counterpart to `mime`'s email multipart: `build(parts) -> (contentType, bytes)`
and `parse(contentType, body) -> list of Part` (fields + files). Over `strings` +
`bytes`; pairs with `web` for handling uploads. Pure `.j`, both binaries. No new
prereq.

### M18.35 - `pdfwriter` module (PDF documents)

**Done.** Generate simple PDF documents - text, lines, rectangles - the way
`htmlwriter` / `label` generate their formats: value-semantic `document()` /
`page(w, h)` / `text` / `line` / `rect` / `color` / `addPage` builders, document
metadata (`info(doc, key, value)` -> the PDF Info dictionary; `Producer` defaults
to "Jennifer pdfwriter", `pdfDate(t)` formats a PDF date string), and
`render(doc) -> bytes` writing the PDF 1.7 object / xref structure by hand (no
stdlib PDF) - catalog, page tree, one FlateDecode-compressed content stream per
page (via `compress`, `zlib`), shared standard-14 Type1 font objects, the Info
dictionary, a byte-offset xref, and the trailer. The standard-14 base fonts (an
unknown font throws `Error{kind: "pdfwriter"}`); coordinates are PDF points
(ints, origin bottom-left), colour is 0-255 RGB. Output is **byte-identical**
run-to-run and across both binaries - no timestamp is auto-stamped (opt into one
via `info` + `pdfDate`), so a rendered PDF is safe to assert against a golden in
an automated test - and validates clean under `qpdf --check` (the Go suite runs
`qpdf` + `pdftotext` + `pdfinfo` when present, plus always-on byte-level checks).
Embedded fonts and images are follow-ons; a writer, not a reader. Over `bytes` +
`strings` + `lists` + `convert` + `compress` + `time`. Pure `.j`, both binaries.
The higher-level layer (font metrics, table / flow layout, Markdown -> PDF) is
`DRAFT#13`. No new prereq.

### M18.36 - `bloom` + `ringbuffer` modules (data structures)

Two pure data-structure utilities delivered together: a **Bloom filter**
(`bloom.new(size, hashes)` / `add` / `mightContain`) over `hash` + `bytes`, and a
fixed-capacity **ring buffer** (`ringbuffer.new(cap)` / `push` / `pop`,
overwrite-oldest) over `list`. Value-semantic, both binaries. No new prereq.

Each of M18.17-M18.36 ships the usual module discipline: a 100%-passing
`modules/X_test.j` overlay, a Go integration test where a network path exists,
`docs/modules/X.md` + a catalog / `SUMMARY.md` / `modules/README.md` /
`JENNIFER.md` entry, and a runnable `examples/modules/X_demo.j`. The `hash.hmac`
primitive that `totp` / `webhook` / `bucket` build on already shipped (a small
`hash` library addition, no milestone). The paired-module entries (M18.30,
M18.36) ship two small modules each.

### M18.37 - `tengine` module (text template engine)

**Done.** A lightweight-CMS text template engine - a subset of Go's
`text/template` (the Go / Hugo style) - rendered directly over a `json.Value`
data tree (dotted paths resolve as JSON Pointers; `$` is the root, `$var` a
variable), with no compile step (it re-scans block bodies). `newSet` /
`add(set, name, src)` (pulling `{{ define }}` blocks into their own entries) /
`render(set, entry, data)`. Actions: value output; `if` / `else if` / `else`
with the functions `eq` / `ne` / `lt` / `le` / `gt` / `ge` / `and` / `or` / `not`
(parenthesised nesting); `range` (with an optional `$i, $e :=` bind) and `with`,
each with `else`; `{{ $x := PIPE }}` variables; `define` / `template` / `block`
layout inheritance; `{{/* comments */}}`; `{{- -}}` whitespace-trim markers; and
output pipes `upper` / `lower` / `title` / `trim` / `html` / `urlize` /
`default` / `truncate` / `join` / `len` / `printf`. Not auto-escaped (a `text/template`, not
`html/template`). Over the compiled-in `json` / `strings` / `lists` / `maps` /
`convert`, so both binaries. Contributed as a Jennifer-coded module, refactored
to the module conventions, and extended to the CMS feature set (conditionals with
comparison / boolean functions, `$` root, variables, range index, and the
`default` / `truncate` / `join` / `len` / `printf` pipes, plus the `title` pipe
and trim markers). No new prereq.

### M18.38 - `barcode` module (barcode / QR generation)

Generate scannable codes as **images**, not printer commands - the complement to
`label` (which emits printer-native barcode *commands* for the printer to render;
this module renders the code itself, for the web, email, PDFs, on-screen display,
or embedding as a bitmap into a `label.image`). **One** module with the symbology
as a selector (stance 1, like `label`'s dialects), covering both 1D and 2D:

- **Encode.** `barcode.encode(data, symbology, opts) -> Symbol` builds a
  device-independent representation - a black/white module **matrix** for the 2D
  codes, a run of **bar widths** for the 1D codes. Symbologies: **2D** `qr`
  (Reed-Solomon over GF(256), the four EC levels `L`/`M`/`Q`/`H`, automatic
  version selection and data-mask scoring); **1D** `code128`, `ean13`, `ean8`,
  `itf` (Interleaved 2 of 5), `code39` (bit-pattern / check-digit tables). The
  1D set matches `label`'s, so the two modules speak the same symbology names.
  `datamatrix` is a follow-on (a second 2D code once QR is proven).
- **Render.** From a `Symbol`: `barcode.svg(symbol, opts) -> string` (the primary
  output - resolution-independent, embeds directly in HTML / email / the docs);
  `barcode.png(symbol, opts) -> bytes` (a monochrome PNG hand-encoded over
  `compress` for the zlib IDAT and `crc` for the chunk CRC-32s - no image
  library); `barcode.terminal(symbol) -> string` (Unicode half-block art for the
  CLI / REPL); and `barcode.matrix(symbol) -> list of list of bool` (the raw
  cells, e.g. to feed `label.image` or a caller's own renderer). `opts` carries
  the module size / bar height, the quiet-zone width, and the foreground /
  background colours (for SVG / PNG).

Pure `.j`, so **both binaries** - it builds only on the compiled-in `compress`
(zlib), `crc` (CRC-32), `encoding` (the numeric / alphanumeric packing), `bytes`,
and bitwise operators; no `net`, no new Go code, no third-party dependency. QR's
Reed-Solomon and mask scoring are the substantial `.j` work (a dogfood of the
byte / bitwise surface); a typical QR is a few hundred codewords, well within the
tree-walker's budget.

The GF(256) / Reed-Solomon math lives in **its own file** inside the module -
`barcode.j` `include`s `barcode_ecc.j` (the multi-file pattern `label.j` uses for
`label_zpl.j` / `label_cab.j`) - so it is cleanly isolated but **private**: not
an exported public surface. This keeps the layering pointing downward (a caller
never reaches through the whole renderer for the field math) while making the
math trivially promotable. The **trigger** to promote it: a genuine second
consumer *outside* `barcode` (a standalone Data Matrix / Aztec / PDF417 module,
or erasure coding). Data Matrix, the only near-term reuser, lives *inside* this
module and shares the file directly, so nothing needs exporting yet. When a real
outside consumer appears, extract `barcode_ecc.j` into a cohesive `ecc.j` module
(Reed-Solomon over GF(256)) that `barcode` then imports - **not** an export bolted
onto `barcode`, and **not** a `crypto` / `galois` library (Reed-Solomon is forward
error correction, not cryptography, and GF arithmetic alone is a building block,
not a user-facing topic). A Go `ecc` library is the further escape hatch only if
the `.j` math ever proves too slow (pure Go, ~60 lines, no third-party dep).

Discipline: a 100%-passing `modules/barcode_test.j` overlay pinned against known
vectors (QR matrices for a few payloads / EC levels, 1D bar patterns, and a PNG
byte-signature check), a `cmd/jennifer/barcode_test.go` (decode a generated PNG /
matrix to confirm it scans, or at least round-trips the structure),
`docs/modules/barcode.md`, an `examples/modules/barcode_demo.j`, and the catalog /
`SUMMARY.md` / `README.md` / `JENNIFER.md` entries. Prereq: `compress` (M16.11)
and `crc` (M15.6), both shipped. No general image library (a deliberate
non-goal - the only raster need is a monochrome bitmap, which the PNG encoder
covers in ~50 lines; revisit a general `image` module only if a broader need
appears).

### M18.39 - `mikrotik` module (RouterOS API client)

Connect to a MikroTik RouterOS device and run commands over its **binary API**,
not SSH. SSH is deliberately out of scope: a real SSH client needs key exchange,
host-key verification, and cipher / MAC negotiation - the whole `crypto` surface
(M20.1) plus a heavy `golang.org/x/crypto/ssh` dependency, against the
dependency-free + TinyGo-clean stance. The RouterOS API is the purpose-built,
crypto-optional door, and it is plain TCP.

- **Transport.** RouterOS serves the API on plain TCP **8728** (`net.connect`)
  and on **8729** for api-ssl (`net.connectTLS`, already shipped) - the same
  transport-agnostic `net.Conn`, so plaintext and TLS share the read / write
  path. The wire protocol is **sentence-based**: a sentence is a run of
  length-prefixed **words** terminated by a zero-length word. The length prefix
  is MikroTik's variable-length scheme (1 byte below `0x80`, then the
  `0x8000` / `0xC0...` / `0xE0...` / `0xF0`-flagged 2-5 byte forms) - pure
  bit-twiddling over `bytes`, the same class of hand-rolled framing `mqtt`
  already does. Words: command `/interface/print`, attributes `=name=value`,
  queries `?name=value`, API tags `.tag=N`.
- **Surface.** `mikrotik.connect(opts) -> Session` (host / port / user /
  password / tls, logging in as part of the connect); `mikrotik.talk(session,
  command, attrs) -> list of (map of string to string)` (the general call - each
  `!re` reply sentence's `=key=value` words fold into one row map);
  `mikrotik.print(session, path)` sugar for a `"/.../print"` read;
  `mikrotik.run(session, path, attrs)` for `add` / `set` / `remove`; and
  `mikrotik.close(session)`. A `!trap` / `!fatal` reply throws
  `Error{kind: "mikrotik"}`; `!done` ends the exchange.
- **Login needs no crypto.** RouterOS 6.43+ accepts a plaintext `/login` with
  `=name=` / `=password=` (confidentiality comes from api-ssl, exactly like the
  mail clients over TLS), so the primary path adds no prereq. The pre-6.43 MD5
  challenge-response is an **optional fallback** and even that only needs
  `hash.compute(b, "md5")`, already compiled in.

Deps: `net` (+ `net.connectTLS`), `hash` (MD5 fallback), `bytes`, the bitwise
operators, `convert`, `strings` - all shipped, **no new prereq**. Net-backed, so
**partial** on `jennifer-tiny` (the no-network stub), like every `net` module.

Scope for v1: the API-only backend covering both RouterOS v6 and v7. The v7
**REST API** (HTTP + JSON) is a possible *second backend* under stance 1, but
deferred - it is a different, stateless request / response shape that does not
share the binary API's session model, so v1 ships the binary API cleanly and a
REST backend is revisited only on a concrete need (the same "add the second way
when a real consumer appears" discipline the `barcode` ECC file uses). Query
(`?`) words and `.tag`-multiplexed concurrent commands are likewise a follow-on
once the synchronous `talk` path is proven.

Discipline: a 100%-passing `modules/mikrotik_test.j` overlay against a fake API
server (word length-prefix encode / decode round-trips, sentence framing, reply
parsing, `!trap` -> `Error`) - the wire codec is pure and testable without a
device; a `cmd/jennifer/mikrotik_test.go` driving a fake Go TCP server through a
login + `talk` exchange; `docs/modules/mikrotik.md`; an
`examples/modules/mikrotik_demo.j`; and the catalog / `SUMMARY.md` / `README.md`
/ `JENNIFER.md` entries. Prereq: `net` (M16.2) and `hash` (M15.6), both shipped.

### M18.40 - `password` module (generate / validate / score)

**Done.** Password generation, validation, and complexity scoring against a
policy `Schema` (character classes, a length range, per-class minimum counts, a
symbol set, exclude-ambiguous-glyphs). `password.schema()` is a strong default;
copy-on-write builders (`withLength` / `withClasses` / `withMinimums` /
`withSymbolSet` / `withoutAmbiguous`) each return a fresh `Schema`.
`generate(schema) -> string` produces a conforming password (a length in
`[minLength, maxLength]`, the per-class minimums guaranteed, the rest filled
from the enabled alphabet, then shuffled; throws `Error{kind: "password"}` on an
infeasible schema). `validate(schema, pw) -> Report{valid, reasons}` checks
length + per-class minimums (minimums, not a whitelist), listing the failed
rules. `complexity(pw) -> Strength{length, classes, poolSize, entropy, label}`
estimates bits as `length * log2(poolSize)` (a `binaryLog` helper - `math` has
no `log`) and bands them very weak / weak / reasonable / strong / very strong. A
disabled class overrides a leftover minimum (the enable bool is authoritative).

Randomness draws from `math`'s shared, seedable, **non-crypto** RNG (the `uuid`
precedent): documented as unsuitable for high-value secrets until it repoints at
crypto-grade random when M20.1 `crypto` lands - a one-function change, no surface
change. Pure `.j` over `math` / `strings` / `lists` / `convert`, so it runs on
**both binaries**. Discipline: a 100%-passing `modules/password_test.j` overlay
(generation-conforms property tests with a seeded RNG, plus deterministic
validate / complexity / `binaryLog` cases), a `TestShippedPasswordOverlayPasses`
guard, `docs/modules/password.md`, an `examples/modules/password_demo.j`, and the
catalog / `SUMMARY.md` / `README.md` / `JENNIFER.md` entries. No new prereq.

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
`TaskState.Observed` atomic, the numeric-conversion and read-length caps, the
cross-module struct-definition lookup so `def x as alias.Struct;` zero-value
construction and `$x.field = ...` writes resolve an imported module struct)
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
- **Message authentication (HMAC) already shipped as `hash.hmac`.** HMAC
  is a hash construction, so it lives in the `hash` library (RFC 2104,
  Go `crypto/hmac`, TinyGo-clean) rather than here - it unblocks request
  signing, webhook verification, JWT HS256, and TOTP without a crypto
  library. What can still land here is a constant-time
  `crypto.hmacEqual` for MAC comparison (verification today recomputes
  and compares the full digest). The KDFs below build on HMAC (PBKDF2 is
  iterated HMAC; HKDF and SASL SCRAM are HMAC-based).
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

### M20.6 - hardware buses (`serial` / `spi` / `iic`)

Device-bus I/O libraries for embedded / single-board-computer hosts - the
syscall-backed siblings of the sysfs-backed `gpio` module
([M18.11](#m1811---gpio-module)). Each needs `ioctl` / `termios`, which `.j` and
plain `fs` cannot reach, so they are Go **system libraries**, not modules:

- **`serial`** - open a serial port (`/dev/ttyUSB0`, `/dev/ttyAMA0`), configure
  it (baud rate, data bits, parity, stop bits via `termios`), then `read` /
  `write` / `close`. Setting the baud rate is a `termios` `ioctl`, unreachable
  from `fs` - which is what forces a library.
- **`spi`** - open a SPI device (`/dev/spidev0.0`), set mode / speed, and
  `transfer(bytes) -> bytes` (full-duplex), via the `SPI_IOC_MESSAGE` `ioctl`.
- **`iic`** - the I2C bus (`/dev/i2c-1`): select a slave address and `read` /
  `write` register bytes via the `I2C_SLAVE` `ioctl`. Named **`iic`** (Inter-IC)
  rather than `i2c` because a library namespace is letters-only (no digit, like
  `bucket` not `s3`); candidates `iic` / `twi` / `wire`, settled at build time.

Build-tag split like `net` / `os`: the real implementation on Linux (the
supported platform), a friendly-error stub elsewhere. Default binary; a
`jennifer-tiny` rebuilt for a specific board could include them (TinyGo's
`machine` package is a different, microcontroller-level API - these target the
Linux `/dev` + `ioctl` interface). Together with `gpio` they complete the SBC
I/O story.

### M20.7 - `sql` (MySQL / MariaDB + PostgreSQL)

A relational-database client library over Go's `database/sql`, shipping the
two **client-server** engines: MySQL / MariaDB (`go-sql-driver/mysql`) and
PostgreSQL (`jackc/pgx`), both **pure-Go** drivers (no cgo, so cross-compile
and the best-effort macOS / Windows artifacts stay clean). SQLite - the one
*embedded* engine - is deliberately **not** here; it needs a multi-MB
dependency and cannot build under TinyGo, so it stays a build-tag opt-in
parked in [horizon](horizon.md) (the `jennifer-full` variant).

**Why a Go library and not a `.j` module over `net`.** MySQL and Postgres are
open TCP wire protocols, so a client *is* writable in pure Jennifer - the same
shape as `redis` / `memcache` / `imap`, and the auth crypto it needs (SHA-1,
SHA-256, `hash.hmac`, PBKDF2 as iterated HMAC) already ships. The deciding
factor is not performance: a database client is **latency-bound** (network
round-trip + server execution dominate; client-side decode only becomes the
cost center when streaming 10^5+ rows, a bulk workload Jennifer is not the
right tool for regardless of driver), and the COW shared-marker protocol
already makes materializing a large result set amortized O(N). The deciding
factor is **correctness maturity**: the mature drivers have absorbed a decade
of protocol long-tail - every auth plugin (MySQL `caching_sha2_password`
full-auth / the RSA path, Postgres SCRAM-SHA-256), charset handling, NULL
semantics, multi-result-sets, server-version quirks - that a hand-rolled `.j`
client would re-derive one edge case at a time. For databases users depend on
daily, that maturity is worth the dependency. Going Go also makes the auth
crypto the driver's problem, not the language's, so this needs nothing from
[M20.1 `crypto`](#m201---crypto).

**The deliberate dependency break.** These are the **first heavyweight
dependencies in the library layer** - a conscious exception to the
dependency-free discipline `CLAUDE.md` states for the library layer (the
only third-party dependency today is CLI-scoped `golang.org/x/term`). Both
drivers are pure-Go, but they are real dependency trees. The exception gets a
reasoning record in [technical/design-decisions.md](technical/design-decisions.md)
when it lands - the sanctioned home for a feature that ships despite appearing
to cut against project doctrine - justified as above, not slid into.

**Build / TinyGo.** Build-tag split exactly like `net` / `httpd`:
`sqllib_std.go` (`//go:build !tinygo`) imports `database/sql` + drivers;
`sqllib_tiny.go` (`//go:build tinygo`) registers `use sql;` and returns a
friendly positioned "not available on this build" error. TinyGo never
compiles the driver imports, so the language stays TinyGo-clean and the
interpreter core is untouched. On stock `jennifer-tiny` the engines are
unavailable anyway (no net driver), consistent with every net-backed module.

**Surface.** Integer-handle-into-a-registry like `fs` / `net`:
`sql.open(driver, dsn) -> Connection`, `sql.query(conn, sql, params...) ->
Rows`, `sql.exec(conn, sql, params...) -> Result` (affected-rows /
last-insert-id), prepared statements, `sql.begin` / `commit` / `rollback`
transactions, `sql.close`. **Values bind only through placeholders**
(injection safety) - `database/sql` abstracts the per-driver spelling (`?`
for MySQL, `$1` for Postgres). The result-row shape is an opaque `sql.Row`
`KindObject` mirroring `json.Value` (foreshadowed in interpreter note 18),
walked by accessors; a typed-struct path waits on the deferred map-to-struct
conversion. A `.j` `postgres.j` module (Postgres has the clean protocol, no
RSA gap) remains a possible later *optional* dependency-free alternative and
language stress-test - not the primary path.

**Builds on.** The [M21.5 `orm` module](#m215---orm-module) is layered on this
library (its hard prerequisite) and inherits the `sql.Row` caveat above - its
typed-row ergonomics wait on the same deferred map-to-struct conversion, so it
graduates in stages.

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

### M21.3 - `jwt` module (JSON Web Tokens)

A JWT (RFC 7519) module. The HMAC algorithms (HS256 / HS384 / HS512) need only
the shipped `hash.hmac` and could stand alone, but the module targets the full
common surface - including the asymmetric **RS256 / ES256** that OAuth / OIDC
rely on - so it is parked here **gated on [M20.1 `crypto`](#m201---crypto)** for
the public-key signing / verification, and graduates into the M18 module track
when `crypto` lands. Surface: `jwt.sign(claims, key, alg)`, `jwt.verify(token,
key) -> claims` (checks the signature and the `exp` / `nbf` time claims), and
`jwt.decode(token) -> claims` (read without verifying). Over `hash.hmac` (+
`crypto` for RS / ES), `encoding` (base64url), `json`, and `time`. **`jwt_auth`**
is not a separate module: it is this module used as a `web.before` middleware
that pulls the bearer token from the `Authorization` header, `jwt.verify`s it,
and rejects on failure (`func requireJwt(ctx) { ...; return true; }`) - shipped
as a snippet in the demo / docs, not its own surface. Discipline as usual: a
`modules/jwt_test.j` overlay (sign / verify round-trips and tampered-token
rejection), docs, catalog, and demo.

### M21.4 - `acme` module (Let's Encrypt / ACME)

An ACME (RFC 8555) client - obtain and renew TLS certificates from Let's Encrypt
and compatible CAs: account registration, an order plus HTTP-01 / DNS-01
challenge, CSR submission, and certificate download, over `http` + `json`. Parked
here **gated on [M20.1 `crypto`](#m201---crypto)**: ACME requests are JWS-signed
with an account key (RS256 / ES256) and the flow needs CSR generation - both
asymmetric-crypto operations. Composes with `web` / `httpd` to serve the HTTP-01
challenge. Graduates into the M18 module track when `crypto` lands. Needs the
default binary. Discipline as usual.

### M21.5 - `orm` module

A relational mapper layered over the [M20.7 `sql`](#m207---sql-mysql--mariadb--postgresql)
library (its **hard prerequisite** - no `sql`, no `orm`), and a good stress-test
of how far the module system stretches. Jennifer's semantics dictate the shape,
and it is **Data Mapper, not Active Record**: structs are value-semantic and
carry no methods, and a module is declarations-only with no mutable state, so a
row object cannot know how to `save()` / `delete()` itself and there is no place
to hold identity-map or dirty-tracking state. The pattern is therefore a
**repository / table-gateway** - the caller passes a record and a schema to
module functions: `orm.insert(conn, schema, record)`, `orm.find(conn, schema,
id) -> record`, `orm.update` / `orm.delete`, `orm.all(conn, query)`.

Three constraints shape the surface:

- **Explicit schema descriptor (no reflection).** `.j` cannot introspect a
  struct's fields, so the caller declares the mapping once - an `orm.Schema`
  (table name, column-to-field list, primary key, per-column type) built with a
  small constructor. This is the module's central object; everything keys off it.
- **Non-mutating functional query builder**, mirroring the `json.Value` write
  surface (`set` / `insert` / `append` returning fresh handles) rather than
  method chaining (values have no methods): `orm.where(orm.from(schema), "age",
  ">", 18)` returns a new `orm.Query`, composed functionally through
  `orm.where` / `orm.orderBy` / `orm.limit` / `orm.join`, then rendered to
  **parameterized** SQL - values bind only through placeholders (injection
  safety inherited from `sql`). A small **dialect** layer (placeholder spelling
  `?` vs `$1`, identifier quoting, `LIMIT` / `OFFSET`, `RETURNING`) is a backend
  selector on one module, not parallel modules (stance 1 / the one-module rule).
- **Row-to-struct mapping is partly gated.** `sql.query` yields an opaque
  `sql.Row`; turning it into a *typed* struct wants the deferred **explicit
  map-to-struct conversion** ([horizon](horizon.md)). So `orm` ships in two
  steps: first a `map of string to V` (or by-hand field extraction via the
  schema) row form that needs nothing new, then the typed-struct return once
  map-to-struct lands.

Transactions come straight from `sql.begin` / `commit` / `rollback`. Kept out of
v1: relations beyond a plain `join` (has-many / belongs-to eager loading wants
object identity and lazy proxies Jennifer does not have), and migrations (a thin
`orm.createTable(schema)` DDL emitter is a plausible follow-on, full migration
tooling is its own module). Needs the default binary. Discipline as usual: a
`modules/orm_test.j` overlay, docs, catalog, and demo - with the overlay split
so the **query-builder-to-SQL** surface (pure string generation) is covered 100%
offline, and live CRUD sits behind a DB-service-gated integration test rather
than the unit overlay.

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
