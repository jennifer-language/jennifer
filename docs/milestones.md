# Jennifer - Milestones

Development is split into milestones. Each milestone produces a *working*
interpreter that runs a strictly larger subset of the language.

> **Note on compaction.** Older milestone entries are periodically **compacted** -
> condensed to a short summary (and sometimes grouped) -
> to keep this file readable and its length in check. The
> compacted text records *what* shipped, not the full implementation design. For
> the detailed design, rationale, and step-by-step evolution of any milestone,
> **dig into the git history** (the commits for that milestone) - that is the
> authoritative record of how it was built.

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

## M13-M13.2 - structs and catchable errors (compacted)

**All done.** The composite-data milestone, batched in dependency order: M13.1
ships the struct mechanism, M13.2 the recoverable-error story built on it (the
canonical error value is a struct), together unblocking composite library
returns. Design detail: git history, the `user-guide`
([types-and-values](user-guide/types-and-values.md#structs),
[control-flow](user-guide/control-flow.md#try-catch-throw)) and
[technical/interpreter.md](technical/interpreter.md) docs, and
`examples/structs.j` / `examples/trycatch.j` (plus the matching `showcase.j`
sections).

| M#    | Topic | Summary |
| ----- | ----- | ------- |
| M13.1 | structs / records | `def struct Name { field as type, ... };` at top level (hoisted before the first statement; duplicate names error in `Run`, silently redefine in the REPL). Literals `Name{ field: expr, ... }` with every field required; `def x as Name;` zero-fills, recursing nested struct fields. Field read `$p.field` / write `$p.field = ...;`; lvalue chains mix `[index]` and `.field` freely (`$L.from.x = 5;`, `$bag.items[0] = 99;`) through one shared index/field walker. Value semantics like lists / maps; `const` deep at any depth. Strict at boundaries - unknown struct type, missing / unknown field at a literal, field-type mismatch on write, and field access on a non-struct are all positioned errors. Runtime: a `KindStruct` tagged-union value. |
| M13.2 | `try` / `catch` / `throw` | Catchable errors (keywords `try` / `catch` / `throw`). `try { body } catch (NAME) { handler }` binds the thrown value to `$NAME` in a fresh per-handler scope; `throw EXPR;` raises any value, convention the auto-hoisted `Error{kind, message, file, line, col}` struct. The runtime errors today's builtins / ops raise (out-of-bounds, missing key, type mismatch) are wrapped into `Error` on entry to the catch (`kind` defaults to `"runtime"` until a site opts into a tag); `throw $err;` in a catch re-raises to the enclosing `try`. **Not** catchable: `exit` (propagates through `try`), and `return` / `break` / `continue` (control flow, flow through `try` unchanged). No `finally`, no typed catch in v1. Internals: an `ErrorSignal` sentinel parallels `ExitSignal`, `runtimeError.Kind` threads the tag, and the `Error` struct is reserved (user code may not redefine it). |

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

## M15 - foundational libraries + first public release (compacted)

**All done.** Nine sub-milestones - two language (M15.2, M15.4), the rest library
/ tooling / release - that built out the foundational stdlib and shipped the
first public release. Design detail: git history and the linked per-library
docs. Two API patterns established here recur across later libraries:

- **Codec-table shape** - the algorithm / format / codec is a string argument
  (`hash.compute(b, algo)`, `crc.compute(b, algo)`, `encoding.encode(s, codec)`,
  `encoding.toText(b, format)`), collapsing parallel verbs into one (stance #1)
  and sidestepping the letters-only identifier rule that rejects `hash.md5(...)`.
- **Integer-handle struct for opaque resources** - a namespaced struct with a
  single `id as int` indexing a Go-side map (`os.Process`, `hash.Stream`,
  `crc.Stream`).

| M#    | Topic | Summary |
| ----- | ----- | ------- |
| M15.0 | existing-library extensions | `lists.shuffle(xs)` (Fisher-Yates, respects `math.randSeed`) and `lists.range(start, end[, step])` (half-open; the single-arg form is deliberately omitted, stance #2). |
| M15.1 | `os` reshape + `meta` | Immutable per-run host facts became uppercase constants (`PLATFORM` / `ARCH` / `EOL` / `DIRSEP` / `PATHSEP` / `ARGS`), operations stay functions (`getEnv` / `hasFlag` / `flag`); dropped the `JENNIFER_` prefix. New `meta` library for interpreter identity (`VERSION` / `BUILD`); the CLI forwards trailing args to `os.ARGS` (script path at index 0). Breaking renames (`JENNIFER_VERSION` -> `meta.VERSION`, `os.platform()` -> `os.PLATFORM`, `os.JENNIFER_LF` -> `os.EOL`), old names now plain "undefined" errors. |
| M15.2 | library-provided namespaced structs (language) | `def x as lib.Name;` type syntax + `lib.Name{field: ...}` literals + the Go `Interpreter.RegisterNamespacedStruct` API, reusing M13.1's value-semantics / deep-const / strict-boundary machinery (only resolution differs). User code can't register structs (Go-side only); no methods-on-structs / inheritance. Unblocks `os.Result`/`Process`, `time.*`, the `hash`/`crc` streams, later `fs`/`net`. |
| M15.3 | `os` external-program execution | First consumer of M15.2. `os.Result{exitCode, stdout, stderr}` + `os.Process{pid}`; `os.run(argv) -> Result` (blocking), `os.spawn(argv) -> Process` (non-blocking) + `wait` / `poll` / `kill`. `argv` is always `list of string` (no shell parsing - use `["sh", "-c", $cmd]` explicitly). Non-zero exit codes are values, not errors. First user-visible two-binary split: `jennifer-tiny` has no `os/exec`, so it returns a friendly "use the default `jennifer` binary" error. |
| M15.4 | `len` built-in, `core` removed (language) | Promoted `len(EXPR)` from the auto-loaded `core` library to a reserved keyword + primary expression (polymorphic over string / list / map / bytes). Deleted `internal/lib/core/`; `use core;` returns a friendly migration error pointing at the built-in and `meta.VERSION`/`BUILD`. Every library now lives behind a `use NAME;` (stance #2, no exceptions). |
| M15.5 | `time` | Instants, durations, fixed-offset zones, strftime format / parse, ISO 8601 round-trip. Structs `time.Time{nanos, offset}` (private fields), `time.Duration{nanos}`, `time.Zone{offset, name}` (public, so an IANA/DST companion can build them). Granularity is a formatting property, not a type; Unix timestamps are constructor/accessor pairs, not a type. IANA/DST stay out of the fixed-offset core (a Go-backed extension, not a `.j` data map). Three parts: core + Unix + calendar + 1-based-ISO-weekday + arithmetic; strftime (`%Y %m %d %H %M %S %z %a %A %b %B %j %u %%`) + `time.zone`/`inZone` + `time.UTC` const alongside `time.utc()` + `time.iso`/`fromIso`; and `examples/benchmark.j` (the TinyGo-vs-Go suite; the sieve became trial-division since value-semantic list mutation makes a sieve O(N^2)). |
| M15.6 | `hash` + `crc` | Two parallel codec-table libraries: `hash` (crypto-style digests `md5` / `sha1` / `sha256`), `crc` (`crc32` IEEE / `crc64` ECMA) - the split keeps "transport integrity" vs "content addressing" visible at the import line (mirrors Go's `crypto/*` vs `hash/crc*`). Output is raw `bytes`. `compute(b, algo)` one-shot + `stream(algo)` / `update` / `finalize` (the integer-handle pattern). No convenience wrappers like `hash.md5String` - compose `convert` + `encoding` (stance #1). Struct hashing deferred. |
| M15.7 | `encoding` | Introspection (`isAscii` / `lenBytes` / `lenRunes`), binary-to-text `toText` / `fromText` (`hex` / `base64` / `base64-url`), character `encode` / `decode` / `codecs` (`ascii` / `iso-8859-1` / `windows-1252` / `ebcdic`). The cross-kind UTF-8 pair stays in `convert` (M12); `encoding` owns the codec proliferation. Exact-match codec / format names (the original alias / case-normalisation layer was later dropped, stance #2); Windows-1252's five undefined positions reject symmetrically. The long-tail single-byte codecs shipped later in [M16.15](#m16---io-libraries-and-developer-tooling-compacted). |
| M15.8 | distribution + first public release | Packaging / CI / release only, no language change. **CI**: PR gate (`go vet` + `gofmt` + `go test ./...` + `make build` + per-binary smoke + em-dash scan); release on bare-semver tags cross-compiling `linux/{amd64,arm64}`, QEMU-smoke-testing the non-native arch, running the benchmark, publishing a draft Release. **Packaging** under `packaging/{debian,arch,mime,man}/` (`.deb` via `build-deb.sh` with man pages + `text/x-jennifer` MIME; AUR `PKGBUILD-bin` / `-git`, auto-filled by the pipeline). **Docs site** via pinned mdBook 0.5.3 -> GitHub Pages. Conventions kept: bare semver tags (no `v` prefix), no top-level `LICENSE` (LGPL text in `packaging/debian/copyright` + a README link). Deferred (not gated): the macOS / Windows cross-build and a real apt repo stay in [Requirements for 1.0.0 stable](#requirements-for-100-stable). |

---

## M16 - I/O libraries and developer tooling (compacted)

**All done.** Phase C - system libraries that touch the OS or do significant
compute, opened by the `spawn` concurrency primitive (M16.0) the I/O libraries
build on, then a lint / profile / test developer-tooling trio and a run of
self-contained data libraries. Design detail: git history and the linked
per-library / per-tool docs.

| M#     | Topic | Summary |
| ------ | ----- | ------- |
| M16.0 | lightweight concurrency | `spawn { ... }` (block primary expression), `task of T` (new compound kind), the `task` library (`wait` / `poll` / `discard` / `waitAll` / `waitAny`). Goroutine-backed but race-free by construction: `snapshotForSpawn` deep-copies a globals+locals snapshot at launch; tasks share only the `TaskState` pointer (the one value-semantics carve-out). A per-run registry loud-fails unobserved task errors at exit (an undiscarded non-terminating spawn hangs at exit - the documented trade-off); `task.wait` re-raises a body error at the wait site; `waitAny` is the runtime's only `reflect.Select`. TinyGo builds with `-stack-size=4mb -scheduler=tasks`. |
| M16.1 | `fs` | Blocking filesystem I/O composed with `spawn` (no `*Async`): whole-file `read`/`write`/`append` (String/Bytes), metadata (`exists`/`isFile`/`isDir`/`stat` -> `fs.Stat`), dir ops with the two-verb recursion split (`mkdir`/`mkdirAll`, `remove`/`removeAll`, `rename`/`list`/`walk`), buffered `fs.File` handles (`open`/`readLine`/.../`close`, `eof` peeks a byte). Path- vs handle-form verbs dispatch on the first-arg kind; `fs.File` shares state across copies (handle carve-out). |
| M16.2 | `net` | TCP (`connect`/`listen`/`accept`/`readBytes`/`writeBytes`), UDP (`listenUDP`/`sendTo`/`recvFrom`), DNS (`lookup`/`reverseLookup`); polymorphic `close`/`address` over three handle registries; blocking calls compose with `spawn` (accept-loop-per-connection). Build-tag split: `jennifer-tiny` returns friendly-error stubs (no netdev in TinyGo). |
| M16.3 | `regex` | RE2 (Go `regexp`, linear-time) over strings: `matches`/`find`/`findAll`/`replace`/`split`/`escape`; `regex.Match{text,start,end,groups,groupsNamed}` (rune indices, `start=-1` = no match); implicit 128-entry LRU pattern cache. Both binaries. |
| M16.4 | `testing` (primitives) | The irreducible system side a `.j` test framework needs: `testing.run(name)` invokes a zero-arg user method via `Interpreter.CallByName`, times it, classifies failures into a `testing.Result`; `results`/`reset` (mutex-guarded); `report` (text / TAP / JUnit). The one place `exit` is caught (Go-level, so language `try`/`catch` still can't). |
| M16.5 | interpreter performance pass | Five sub-parts, behaviour unchanged: **.1** shared-marker COW on compound Values (append-in-a-loop O(N^2) -> amortised O(N)); **.2** parse-time lexical slot resolution ((Depth,Slot) coordinates + a `slots` slice; undefined/shadowing promoted to parse-time errors); **.3** pooled + pre-resolved + slot-bound method-call frames; **.4** namespaced-call / comparison / arg-bind / root-cache fast paths; **.5** compile-time constant folding + a `Share()` scalar fast path. Numbers in [tinygo.md](technical/tinygo.md). |
| M16.6 | lint | `jennifer lint` flags compile-legal-but-suspect patterns: grouped IDs (**L0nn** source errors / **L1nn** correctness / **L2nn** style / **L3nn** lifecycle), `# lint-disable[-file]: IDS` suppression, `--checks` / `.jennifer-lint` config, human / JSON / GitHub output (a JSON pipeline stays valid even on a source error); exit 0/1/2. `!tinygo`. |
| M16.7 | profile | `jennifer profile` attributes work to `.j` source positions (what `go tool pprof` can't): a statement profile (hit count + self/cumulative wall-clock) and an `--allocs` value-copy profile; table / pprof (hand-encoded gzipped protobuf) / Chrome-trace output; program output to stderr so the profile owns stdout. `!tinygo`. |
| M16.8 | testing framework consolidation | An assertion vocabulary on M16.4 (`assertEqual` ... `assertThrows`, throwing `Error{kind:"assertion"}` at the call site), `CallByNameWith`/`runWith` arg dispatch, and the `jennifer test` subcommand (`test*` discovery or `--filter`, `setUp`/`tearDown`, `--isolated` per-test subprocess, text/TAP/JUnit, exit 0/1/2). Builtins can now raise a catchable error via `interpreter.RaiseError`. |
| M16.9 | `json` | Hand-rolled RFC 8259 encode/decode onto the tagged-union Value (no `encoding/json`, no reflect). `encode`/`encodePretty`/`decode`; structs and `map of string to V` -> objects, `bytes` -> base64, integral numbers -> `int` else `float`. Also closed a type hole: a generic collection (a fresh literal or decode result) is validated entry-by-entry against the declared element type at every binding boundary. Decode's return shape was later superseded by M16.16 (`json.Value`). |
| M16.10 | `uuid` | RFC 9562: `generate("v4")` (random) / `generate("v7")` (time-ordered) - the version a string arg since identifiers were letters-only - plus `parse`/`isValid`/`version` and constant `NIL`. Randomness was later repointed from `math` to `crypto`'s crypto-grade source (M20.1), making v4/v7 unguessable; M22.3 renamed `generate("v4")` -> `v4()`. |
| M16.11 | `compress` | Byte-stream size reduction (distinct from `encoding`'s representation codecs): `pack`/`unpack` for `gzip`/`zlib`/`deflate` with an optional `fast`/`default`/`best` level, plus a streaming `compress.Stream`. Go `compress/*`, TinyGo-clean. |
| M16.12 | `archive` | tar / zip containers over `bytes` (no `fs`, value-semantic): `pack`/`unpack` (verbs shared with `compress`) for `tar`/`zip`/`tar.gz`; a bundle is a `list of archive.Entry{name,data,mode,mtime}`. Go `archive/tar`+`archive/zip`. |
| M16.13 | `os.isTerminal` | `os.isTerminal("stdout"/"stderr"/"stdin")` -> bool (the ANSI-colour gate) via the char-device mode bit (`os.ModeCharDevice`) - pure stdlib (keeps `x/term` CLI-scoped), TinyGo-clean; an unstattable stream reports `false`. |
| M16.14 | `net` TLS | `net.connectTLS(address)` (implicit) and `net.startTLS(conn)` (in-place STARTTLS upgrade), both yielding the transport-agnostic `net.Conn`. Cert verification on by default, `net.TLSOptions{skipVerify, caCert}` opt-out. Go `crypto/tls` on the `!tinygo` build (stubbed on tiny). |
| M16.15 | `encoding` completion | `toText`/`fromText` gained `quoted-printable`, `base32`/`base32-hex`, `ascii85`, `z85`; the full ISO-8859-{1..16} / Windows-{1250..1258} single-byte codecs, generated from the Unicode mapping files (`gen_codecs.go` -> `codecs_gen.go`) so only `ascii`/`ebcdic` stay hand-written. Exact-match codec/format names (the normalisation layer was dropped, stance #2). |
| M16.16 | `json.Value` | The strict home for heterogeneous JSON without a language top type: `json.decode` returns an opaque `json.Value` - the first `KindObject` (the opaque sibling of `KindStruct`: discriminated by `(namespace, name)`, minted only by a library, rejecting operators / `[i]` / `.field`). `convert.typeOf` -> `"object"`, `convert.objectType` -> `"json.Value"`. Reads + non-mutating writes share JSON Pointer (RFC 6901): `typeOf`/`get`/`has`/`keys`/`length`/`as*`/`isNull` and `map`/`list`/`set`/`insert`/`append`/`remove`/`move` (strict / no-vivify, `-` end-marker), node types in `list`/`map` vocabulary; a displayer renders a handle as its JSON. `json.decode`'s return type changed (a pre-1.0 break); the decoder's number grammar tightened to json.org. No `any` keyword (rationale in [rejected.md](technical/rejected.md)). |

---

**Phase D: higher-level and Jennifer-coded libraries (M17-M20).**

## M17 - module system for Jennifer-coded libraries (compacted)

**All done.** Jennifer-coded libraries get their own namespace, scope, and
explicit `export`s via a real **module boundary** (`import "x.j" as x;`, a parser
+ interpreter feature) beside the textual `include "x.j";` splice (a preprocessor
operation for composing one module from several files). Settled cross-cutting
decisions (turned-down alternatives in [rejected.md](technical/rejected.md)): each
module is its own resolution context (own `use` set, namespace + export tables);
the module top level is **declarations-only** (no mutable state, so `spawn`
capture is unaffected); the one global `Error` stands (modules add
distinctly-named error structs, never redefine it); **private by default**, a
leading `export` publishes (no `public` / `private` keyword); multi-file modules
assemble via `include` behind one entry file (no directory-as-module); a module
needs a filesystem (an FS-less `jennifer-tiny` host fails with the ordinary
search-path error). Design detail: git history,
[imports.md](user-guide/imports.md), [interpreter.md](technical/interpreter.md).

| M#    | Topic | Summary |
| ----- | ----- | ------- |
| M17.1 | source tree + resolution | `internal/module` `Classify` + `Resolve` map an import path (local `./` / `../`, absolute `/`, or a bare name walked on the search path) to a canonical absolute path, rejecting an ambiguous name (found in two search dirs) and a not-found. The system module dir resolves `--sysmoddir` > `JENNIFER_SYSMODDIR` > compile default (surfaced as `meta.SYSMODDIR`; a named-but-missing dir refuses to start, the compile default is best-effort); `-I DIR` (repeatable) appends after it. `jennifer version -v` reports the layers. |
| M17.2 | import statement + loader | `import "path.j" [as NAME];` is a real statement (`ModuleImportStmt` - the preprocessor passes it through, the parser builds the node). The loader runs each module in a fresh sub-interpreter sharing one `moduleReg`, so **run-once** (cached by canonical path), **depth-first post-order init**, and **cycle detection** (erroring with every edge named) all fall out of the recursion. Load-time errors (a parse error or a throwing `def const` init) aren't catchable (`import` is a declaration, not an expression). `fmt` / `ast` / `tokens` round-trip an `import` line. |
| M17.3 | module scope + namespacing | Declarations-only top level (`checkModuleDeclarationsOnly`: only `def const` / `def struct` / `func` / `use` / `import`; scripts keep both). `loadModuleImports` binds each alias (the `as NAME`, or the file stem) into `moduleAliases`, collision-checked against library prefixes. Consumer resolution rides the qualified-reference eval layer: `evalQualifiedCall` / `evalQualifiedConst` dispatch `alias.fn(args)` into the module's own interpreter via `CallByNameWith` (args evaluated in the consumer, body run against the module's globals + methods) and read `alias.CONST`. `use` non-transitivity, run-once sharing, and `-race` safety all follow from the fresh-sub-interpreter-per-module model. |
| M17.4 | exports + visibility | `export` publishes a top-level `func` / `def struct` / `def const`; unmarked names stay private (reaching `mod.helper` is a positioned "not exported" error), and `export` in a `run` script is rejected. `checkReferentialClosure` rejects an exported field / parameter typed as a *private* module struct (library / namespaced types cross freely). **Cross-module struct identity** = boundary translation (`retagStructs`): a module's structs are bare inside it and re-tagged to `(module-stem, name)` as a value crosses out to an importer and back, so `def p as mod.Point`, `mod.Point{...}`, field reads, and pass-back all type-check while `a.Point` / `b.Point` stay distinct; the retag also tags the element-type metadata a `list` / `map` carries (`retagType`). A co-located `MODULE_test.j` overlay (a token splice in `jennifer test`) runs white-box tests against the module's private names. |
| M17.5 | `ansi` module | First module built on the system (pure `.j`, one `use os;` across the boundary; a real dogfood of import / export / resolution). Exports `color` / `bgColor` / `style` (bold / dim / italic / underline / reverse) / `rgb` truecolor / `strip`, plus per-colour and per-style shortcuts (`ansi.red(s)`, `ansi.bold(s)`, ...). The ESC byte is built from a one-byte `bytes`; `strip` uses `regex`; unknown names `throw`. Stateless + TTY-aware: `enabled()` re-reads `NO_COLOR` / `FORCE_COLOR` / `os.isTerminal("stdout")` per call (no toggle state; degrades to always-on when `os.isTerminal` is absent). Colour is a string wrapper, so a `%s|color=` printf modifier was rejected in its favour. |
| M17.6 | `semver` module | Strict [SemVer 2.0.0](https://semver.org) as a second pure-`.j` reference module (`use strings` / `convert` / `regex`, so both binaries), and the base a future `jvc` package manager needs. Exports `Version{major, minor, patch, prerelease, build}` + `parse` (throws on invalid) / `isValid` / `toString`; `compare` / `lt` / `eq` / `gt` (full SemVer precedence: numeric core, prerelease ranks below release, build ignored); `isStable` (`0.y.z` unstable by convention) / `isPrerelease`; `incMajor` / `incMinor` / `incPatch`; and `sort` (own pass over `compare`, since `lists.sort` is scalar-only). Strict - a loose `1.2.3.4` is rejected. `parse` uses the anchored RE2 pattern with named groups; precedence + sort are hand-written (the algorithmic dogfood). Building it surfaced + fixed the M17.4 `retagType` gap (a consumer `list of semver.Version` handed back into a module `list of Version`). Range / constraint matching (`^1.2.0`, `>=1.0.0`, `~1.2.3`) deferred to `jvc`. |


## M18.x - Jennifer-coded modules

Built atop the existing system libraries. Each one ships as a Jennifer
**module** under `modules/` (the directory introduced in M17); none of
them are compiled into the interpreter binary. Sub-milestones in priority
order.

### M18.1-M18.40 - shipped modules (compacted)

**All done.** Forty sub-milestones (with their nested parts) shipped as
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
| M18.27     | `ntp`                   | SNTP network-time client over UDP: `query` / `queryWith` -> `Result` (server time + clock offset + round-trip delay). |
| M18.28     | `statsd`                | fire-and-forget StatsD metrics over UDP (`count` / `gauge` / `timing` / `set`); the push counterpart to `prometheus`. |
| M18.29     | `influxdb`              | InfluxDB 1.x client on `http`: line-protocol `Point` builders + `write`; `query` -> parsed `Series`. |
| M18.30     | `slack` / `discord`     | incoming-webhook chat notifiers on `http`: plain `send` + Block Kit / embed builders (`sendMessage`). |
| M18.31     | `telegram`              | Telegram Bot API on `http`: `sendMessage` / `sendPhoto` / `getMe`, `getUpdates` long-poll (stateful receive loop). |
| M18.32     | `websocket`             | RFC 6455 client over `net` (`ws://` / `wss://`): handshake + masked `send` / `receive` (auto-pong, fragment reassembly). |
| M18.33     | `amqp`                  | AMQP 0-9-1 client for RabbitMQ over `net`: handshake, `declareQueue`, `publish`, `get` (Basic.Get pull), `ack`. |
| M18.34     | `multipart`             | `multipart/form-data` (RFC 7578) build + parse (binary-safe); `web.multipartForm` pairs it with `web`. |
| M18.35     | `pdfwriter`             | generate PDF documents (text / lines / rects, Standard-14 fonts, FlateDecode via `compress`); byte-identical output. |
| M18.36     | `bloom` / `ringbuffer`  | data structures: a Bloom filter (probabilistic set) + a fixed-capacity ring buffer (bounded FIFO). |
| M18.37     | `tengine`               | a `text/template`-subset engine over a `json.Value` tree (`if` / `range` / `with` / pipes / layout inheritance). |
| M18.38     | `barcode`               | QR (Reed-Solomon over GF(256), masking, versions 1-10) + 1D (`code128` / `code39` / `ean13` / `ean8` / `itf`); SVG / PNG / terminal. |
| M18.39     | `mikrotik`              | MikroTik RouterOS API client over `net`: sentence-based binary framing, `talk` / `print` / `run`, plaintext + MD5 login. |
| M18.40     | `password`              | password generate / validate / score against a policy `Schema`; entropy-based complexity (non-crypto RNG). |

**Enabling changes** these modules pulled into the system side (each documented
under its library):

- **`net.setDeadline`** - a read/write deadline for socket timeouts (M18.13; later
  extended to UDP sockets for `ntp`, M18.27).
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


## M19 - cross-cutting tooling (compacted)

**All done.** The catch-all bucket for interpreter / tooling work belonging to
neither the M18 `.j` modules nor the M20 Go system libraries: M19.1-M19.5 a
correctness / performance hardening pass over the core + libraries; M19.6 the
coverage tool; M19.7 the `@scope/package` vendored-module resolver; M19.8 the
one-time org / vanity-path relocation; M19.9 the audit-driven hardening pass. No
`reflect`, no TinyGo-cleanliness break. Design detail: git history and the linked
docs.

| M#    | Topic | Summary |
| ----- | ----- | ------- |
| M19.1 | interpreter concurrency-safety | Both interpreter data races fixed, each pinned by a `-race` stress test. `snapshotForSpawn` snapshots the launching goroutine's own root frame (`effectiveGlobal(env)`), not the live `i.global`, so a nested spawn no longer races the main goroutine's global writes. Declared struct types are stamped once, single-threaded, before any statement (`resolveDeclaredTypesOnce`, after `loadModuleImports`) with a `parser.Type.Resolved` marker, so the per-execution re-resolve is a read-only no-op (also fixes a latent aliased-library-struct "canonical is aliased" rejection). Error timing unchanged. |
| M19.2 | value representation cleanup | Removed the inert copy-on-write machinery (`Value.shared`, `Share()`, `Ensure()`, `ensureCOW`, the per-`VarExpr`-read `Share()`); the mutation sites grow the binding's own backing in place, reads return the binding directly. Value semantics rest (as before) on eager deep copies at every store site; the write-through alternative is in `rejected.md`. Dead COW reporting stripped from the `--allocs` profiler. A fresh list / map / struct literal RHS is already private, so `execDefine` / `execAssign` skip the redundant copy (`rhsFreshLiteral`), proven by a profiler-backed test. |
| M19.3 | runtime perf: maps + call/loop hot path | Maps gained an advisory hash index (`Value.mapIdx`, encoded scalar key -> position) guarded by a `len(mapIdx) == len(Map)` stamp, so `$m[$k] = $v` over N keys is O(N) not O(N^2) while insertion order + value semantics are untouched (any stale / duplicate-key / non-hashable case fails the stamp and falls to the linear scan; a 100k-key build + for-each runs sub-second where the quadratic path took minutes). Plus: `execForEach` / `execFor` borrow frames from `envPool`; `DefineAt` skips the shadow walk on the slot path; `Run` pre-sizes `i.global`'s slots; the three mutation sites write the root binding through `(Depth, Slot)`; `lists.reverse`/`head`/`tail`/`slice`/`concat` shallow-copy instead of deep-copying an argument they overwrite. |
| M19.4 | resource lifecycle + numeric strictness | `os.spawn` handles keyed by a monotonic id, not the OS pid (a recycled pid can't alias a handle or misroute `wait`/`poll`/`kill`); the reaper drains captured buffers to strings and drops the live `*bytes.Buffer`s (idempotent `wait` / `poll`-after-`wait` preserved). Numeric strictness: `convert.toInt` and `math.floor`/`ceil`/`round` reject NaN / +/-Inf / out-of-int64 floats; `math.abs(MinInt64)` errors; the `toml` decoder errors on an int past int64 (`json` keeps its deliberate fallback); the most-negative int literal `-9223372036854775808` parses to `MinInt64` (folded at unary-minus with a 2^63 range check). |
| M19.5 | module struct identity: canonical path | Module structs were tagged only by the file **stem**, so two modules sharing a basename (`a/util.j`, `b/util.j`, or two `@scope/package` decks) produced identical `(namespace, name)` identity and a foreign struct passed the other's type check. Identity is now keyed by the module's canonical (resolved) **path**: `Value` and `parser.Type` gained a `ModPath` field that `Equal` / `MatchesDeclared` compare alongside `StructNS` (which stays the stem, for display, so `%v` still reads `benchmark.Point`); the boundary retag + method-parameter stamping thread the path. Two imports of the same file stay one type; different files differ - no import error. |
| M19.6 | `.j` code coverage | `jennifer test --coverage[=text|json]` reports statement coverage by reusing the profiler's per-position hits (no second counting path): a statement profiler runs from top-level init through every test method, and `renderCoverage` intersects those hits with the AST's executable positions (`statementPositions`, mirroring `execStmt`'s recording). Scoped to the tested program's files (an imported-but-only-ran module does not skew it), per-file + total; `text` names the never-executed positions, `json` owns stdout (the human report moves to stderr). The plain `jennifer test` path is unchanged (a nil collector). An HTML view is a later `htmlwriter` consumer. |
| M19.7 | `@scope/package` resolution (vendored decks) | A leading `@` is a vendored-deck reference expanded by one function (`resolveVendor`): the `@` swaps in the vendor root and a reference not ending in `.j` gets the package-named entry appended, so `@claude/bitcoin`, `@claude/bitcoin/`, and `@claude/bitcoin/utils.j` all reduce to a plain absolute path (after which run-once cache + M19.5 path identity are untouched). The entry is `<package>.j`, so `moduleStem` gives the package name and the display namespace / default alias fall out (`import "@claude/bitcoin/"` binds `bitcoin.`); two same-package decks across scopes are distinct types, colliding only on the default alias (resolve with `as`). Vendor root via `FindVendorRoot`: `--vendor` > `JENNIFER_VENDOR` > nearest `vendor/` above the program. Path safety: `@` front-only, no `.`/`..`, the file must stay inside the deck; a missing root is a guided error. The `jvc` manager over this stays DRAFT#12. |
| M19.8 | relocation: `jennifer-language` org + vanity path | A one-time mechanical relocation, no language / interpreter behavior change: the repo moves to a `jennifer-language` GitHub org, and (separately) the Go module path moves **off** GitHub to a vanity path `jennifer-lang.dev/jennifer` served by a `go-import` meta page, so module identity is host-independent. Two distinct targets: the **Go module path** -> the vanity domain (`go.mod` + 112 `.go` imports rewritten; the meta page maps it to the org repo, plus a `go-source` tag for pkg.go.dev); **human-facing URLs** -> `github.com/jennifer-language/jennifer` (the `-lang`.dev vs `-language` org spelling is deliberate; the redirect keeps old links working, but canonical in-tree URLs are updated). Metadata / CI / packaging swept until `grep -rn 'mplx/jennifer-lang'` is empty; the first-party deck scope placeholder flips `@mplx/` -> `@jennifer/` (doc-only until `jvc`). |
| M19.9 | audit-driven correctness + hardening | A systematic severity-ordered sweep of ~190 findings from a full bug / performance audit of `internal/` + `modules/`, each fix with a regression test (Go test or `_test.j` overlay). **Crash / safety:** OS-entropy RNG seeding (predictable UUIDs / session ids / passwords fixed; `math.randSeed` stays the deterministic opt-in), `json` / `toml` decode nesting caps, a `try`-body scope fix (a throw-skipped `def` reads undefined, not null), `tengine` recursion guards, `archive` zip-slip + aggregate-decompression caps, a JSON-pointer overflow guard, the two interpreter `-race` races + REPL-vs-spawn table mutation. **Correctness:** lvalue writes re-fetch their root after the RHS, index / append stamp the element type, path-keyed module structs, `barcode` (Code 128 stop / EAN check digit / Code 39 `*` / QR mask 3), `pdfwriter` WinAnsi / Info encoding, a full `toml` conformance pass, `http` / `web` (CSRF / cookie / CORS / ETag) hardening, quote-aware `vcard` / `ical`. **Performance:** the O(N^2) accumulation patterns retired (map hash index, `json` object decode, wire-framing reads in `redis` / `amqp` / `mqtt` / `mikrotik` / `imap` / `websocket`, list-join builders in `csv` / `barcode` / `influxdb` / `jsonl` / `statsd`, GF(256) inline). **Lifecycle:** `os.release` + capped child output, non-blocking `net.eof` + mutex-guarded `net.Conn`, `httpd` admission bounds / must-respond timeout / TLS-1.2 floor / safe unix-socket unlink, per-stream mutex + a `discard` verb on hash / crc / compress streams, `lint` descending into `spawn` / `repeat`. Plus **six coordinated pre-1.0 strictness breaks** (each with tests + operator / scoping docs): `%` is floored (Python; `-7 % 3 == 2`, `7 % -3 == -2`); integer arithmetic overflow is a positioned error, not a silent wrap; a duplicate map-literal key is an error; mixed `int` / `float` comparison is exact (no lossy promotion, so `9007199254740993 == 9007199254740992.0` is `false`); a method may not share a top-level var / const name (no-shadowing both directions); reading a constant with the `$` sigil (`$MAX`) is a parse error. |


## M20 - system libraries (compacted)

Go **system libraries** (cryptographic primitives, plus formats too heavy or
too reflect-bound for a Jennifer-coded `.j` module - the `json` pattern,
[M16.9](#m16---io-libraries-and-developer-tooling-compacted)), plus two cleanup keywords (`defer` / `errdefer`) and
Unix-signal support that the libraries needed. **All done.** Ten sub-milestones,
each shipped with the standard discipline: a `cmd/jennifer` / `*_test.go` suite,
a `docs/libraries/*.md` reference, cheatsheet + `JENNIFER.md` entries, and a
runnable example. Per-function detail lives in
[docs/libraries/](libraries/index.md); this table is the milestone-number index.

| M#     | Library / feature | Surface |
| ------ | ----------------- | ------- |
| M20.1  | `crypto`          | Security primitives above `hash`, Go stdlib only: crypto-grade `randBytes` / `randInt` (rejection-sampled, unseedable), constant-time `hmacEqual`, key derivation `hkdf` / `pbkdf` (`algo` sha1/256/512). Repointed `uuid`'s random source here, so v4 / v7 are unguessable. |
| M20.2  | `xml`             | Hand-rolled XML encode / decode over an opaque `xml.Value` (`KindObject`) element tree; read (`tag` / `text` / `attr` / `children`) + XPath-style `get` / `findAll` / `has` (`name` / `name[k]` / `*`) + build (`element` / `setAttr` / `setText` / `append`). No `encoding/xml`. |
| M20.3  | `yaml`            | YAML 1.2 over `yaml.Value`, the same read / walk / write surface as `json` / `toml` + `asDatetime`; `decode` / `decodeAll`, anchors / aliases by value, `<<` merge keys. Backed by `gopkg.in/yaml.v3` (the one config parser that earns a dep); pre-parse depth + node-budget guards. |
| M20.4  | `intl`            | Message catalogs + locale-aware `tr(key[, params])` (`{name}` interpolation, locale -> base -> default -> key fallback); `load` / `setLocale` / `locale`. A system library (global mutable state + O(1) Go-map lookup); single-pass, output-capped interpolation. |
| M20.5  | `term`            | Terminal host control for TUIs: raw mode (`makeRaw` / `restore`, single-use `State`), `size` -> `Size{rows, cols}`, `readByte`. Over `golang.org/x/term`, build-tag split (stub on `jennifer-tiny`); refused in the REPL. |
| M20.6  | signals           | Cooperative Unix signals: `SIGUSR1` -> live interpreter diagnostics (dump-and-continue); `os.catchSignal` / `os.gotSignal` (opt-in `int` / `term` / `hup` / `usr2`) for graceful shutdown; CLI terminal-restore-on-abort. Establishes the checkpoint hook the loop-cancellation follow-on will build on. |
| M20.7  | `defer` / `errdefer` | `defer CALL(args);` - single call, args snapshotted, block-scoped LIFO, runs on every exit path, never crosses the method / `spawn` boundary (no `finally` - rejected). `errdefer` (Zig-style) fires only on a propagating error; adopted by the seven connect-then-handshake modules. Paired with the new `fs.sync`. |
| M20.8  | `serial` / `spi` / `iic` / `gpio` | Device I/O over Linux `/dev` + `ioctl` (`x/sys/unix`), build-tag split `linux && !tinygo`: three buses + character-device GPIO (reusing the sysfs [M18.11](#m1811---gpio-module) module's pin-keyed shape). I2C is `iic` (letters-only). Shared plumbing in `internal/lib/devio`; ioctl struct layouts pinned to the kernel ABI by size assertions. |
| M20.9  | `sql`             | Relational client over `database/sql`: MySQL / MariaDB (`go-sql-driver/mysql`) + PostgreSQL (`jackc/pgx`), both pure-Go. `open` -> `Connection`, `query` / `exec`, pull cursor + typed `as*` accessors, `begin` / `commit` / `rollback`, prepared statements. Values bind **only** through placeholders. The first heavyweight library-layer dependency ([design-decisions.md](technical/design-decisions.md)); build-tag split, default binary only. |
| M20.10 | `crypto` AE + signatures | AES-256-GCM `encrypt` / `decrypt` (32-byte key, nonce prepended, AEAD-only) + Ed25519 `signKeypair` / `sign` / `verify` (`crypto.Keypair`); added `sha384` to `hash`. All Go stdlib, TinyGo-clean. Safe-by-construction: AEAD-only, one algorithm per verb, length-validated keys, internal nonce. Out: password hashing, raw block modes, RSA / ECDSA, x509, AAD. |

Cross-cutting additions these pulled into the language / system side (each
documented under its library):

- **`defer` / `errdefer`** (M20.7) - the deterministic-cleanup keywords; the
  `finally` rejection and the `printf`-for-translation rejection (M20.4) are
  recorded in [rejected.md](technical/rejected.md).
- **`fs.sync`** (M20.7) - fsync a write / append handle to the device, distinct
  from `close`'s reach-the-OS: durability you check, versus cleanup you defer.
- **`sha384` / `sha512`** (M20.10) - the SHA-2 digests filled out across
  `hash.compute` / `hmac` / streaming; MD5 / SHA-1 stay checksum-only.
- **The nesting cap** (`internal/limits.MaxNestingDepth`) shared by the language
  parser and the `json` / `toml` / `xml` decoders - build-tag split so it stays
  below `jennifer-tiny`'s fixed-stack crash point (see
  [technical/tinygo.md](technical/tinygo.md)).
- **The `SIGUSR1` diagnostics checkpoint hook** (M20.6) - the loop-iteration /
  call checkpoint the deferred cooperative loop-cancellation follow-on
  (interruptible loops, REPL Ctrl-C, terminating-signal terminal restore) builds
  on.

## M21 - general backlog (compacted)

The catch-all bucket: milestones that fit no other track - not a Jennifer-coded
module (M18), not interpreter / tooling work (M19), not a Go system library
(M20), and not a beyond-1.0.0 idea (those live in the [horizon
collection](horizon.md)). Anything worth recording with no natural home landed
here, and graduated out once a cluster grew big enough to earn its own bucket.
**All done** - thirteen sub-milestones across Jennifer-coded modules, security
hardening, interpreter / language work, byte throughput, and Windows packaging.
Each shipped module carries the standard discipline (a 100%-passing `*_test.j`
overlay, a `cmd/jennifer/*_test.go` integration test, `docs/modules/*.md` +
catalog + `JENNIFER.md` entries, a runnable demo); per-item detail lives in those
docs - this table is the milestone-number index.

| M#     | Topic | Summary |
| ------ | ----- | ------- |
| M21.1  | `screen` module | Terminal UI. Output-only layer (both binaries): a value-semantic cell `Buffer` (`text` / `textColor` / `box` / `fill` / `hline` / `vline`), ANSI control builders, and a flicker-free `render` / `diff` paint loop (repaints only changed row-runs). Interactive layer (`term`, default binary): a pure `decodeKey(seq) -> Key` (printable / arrows / nav / F1-F12 / ctrl-* / alt-*) plus `nextKey` / `begin` / `end` / `size` over raw mode. 0-based coords; drawing past an edge is clipped. The curses / bubbletea subset. |
| M21.2  | `feed` module | RSS 2.0 + Atom 1.0 build **and** parse in one module (format chosen on `build`, sniffed on `parse`); a value-semantic `Feed` of `Entry` plus `fetch(url)`. Rides `xml` (parse / escaped build) and `time` (RFC 822 / 3339 dates). Hardened for untrusted feeds: `xml`'s nesting cap + no-custom-entity decode, lenient dates, and a new 64 MiB `http` body cap closing an unbounded-body OOM. |
| M21.3  | `jwt` module | JWT (RFC 7519) `sign` / `verify` / `decode` / `header`, claims a `json.Value`; ten algorithms (HS* / RS* / ES* / EdDSA). `verify` pins the expected alg (blocks algorithm-confusion), enforces `exp` / `nbf`, HMAC-compares constant-time. Brought M20.10's deferred asymmetric crypto due: extends `crypto` with `rsaSign` / `rsaVerify` / `ecdsaSign` / `ecdsaVerify` over PEM keys (build-tag split - RS* / ES* need the default binary; HS* / EdDSA on both). |
| M21.4  | `acme` module | ACME (RFC 8555) client over `http` + `json`: directory / account / `order` / `authorization` / `challenge` (HTTP-01 `keyAuthorization` + DNS-01 `dnsRecord` math) / `accept` / `finalize`(CSR) / `downloadCertificate`. Per-request JWS + a fresh anti-replay nonce; CA problem docs surface as a catchable `Error`. Second `crypto` expansion: `rsaGenerateKey` / `ecGenerateKey`, `jwkPublic` (RFC 7638 thumbprint), `csr` (PKCS#10). Default binary only. |
| M21.5  | `orm` module | **Data Mapper** over `sql` (value-semantic, method-less structs rule out Active Record): repository CRUD keyed off an explicit `orm.Schema` (no reflection), a non-mutating functional query builder (`from` / `where` / `orderBy` / `limit` / `join` -> `toSql`) with per-dialect placeholders (mysql / postgres backend selector, injection-safe), records as `map of string to string`, plus a `createTable` DDL emitter. Enabling `sql` change: `toDriverArgs` spreads a single `list` argument (Jennifer has no spread). |
| M21.6  | `font` module | Pure-`.j` TrueType / SFNT parser (no Go - `bytes` + bitwise + `fs`, both binaries): `parse` / `open` -> `Font`, then `unitsPerEm` / `name` / `advance` / `glyphPath` (SVG `d`) / `glyph` (contours). Parses `head` / `cmap` (fmt 4 + 12) / `maxp` / `hhea` / `hmtx` / `loca` / `glyf` (simple **and** composite) / `name`. Closed the dogfood gap: `scripts/genwordmark.j` reproduces the wordmark byte-for-byte without fontTools. |
| M21.7  | injection / DoS hardening | Real wire-boundary fixes, each with a regression test: CRLF / control rejection in `smtp` (envelope + EHLO), `http` (method), `websocket` (handshake URL), `web` (cookie `Path` / `Domain`); `jwt.verify` rejects an unsupported `crit`; `acme` control-char escaping + validated nonce; `websocket` 64 MiB message cap (refused before allocation, `1009` close). Shipped the **security model** (`SECURITY.md` + `security-model.md`): full host access is by design, untrusted *data on a wire* is the bug class, the untrusted-code sandbox is `DRAFT#11`. |
| M21.8  | call-depth limit + profiler metric | Deep recursion that overflowed the Go stack (fatal crash / `jennifer-tiny` segfault) now raises a catchable "call stack too deep" - Python's `RecursionError` analogue. The counter lives on the per-goroutine root env (spawn-safe); the cap is build-tag split in `internal/limits` (10000 default / 48 tiny, whose stack rose 2 -> 4 MiB). Feeds a `jennifer profile` max-call-depth metric. |
| M21.9  | network + credential hardening | Sensible-default / optional checks, one regression test each: an optional `timeoutMs` on `net.connect` / `connectTLS` / `startTLS`, `fs.chmod` / `chown`, `web` `Secure` cookies, `smtp` no-cleartext-auth + anti-downgrade STARTTLS check + envelope validation, `oauth` 0600 token file, `jwt.verifyWith` (iss / aud), 64 MiB received-data caps across the network clients, `amqp` AMQPS. |
| M21.10 | byte-oriented throughput | The `binary` library (`concat` / `slice` / `indexOf` / `contains` / `split` / `startsWith` / `endsWith` - the byte counterpart to `strings`, value-semantic, TinyGo-clean) plus `net.readAll` / `readN` (bulk reads with catchable size / close-mid-frame caps). Reworked `http` / `mqtt` / `imap` onto bulk reads; a `binary.indexOf` benchmark fixture. |
| M21.11 | range syntax (`..`) | Half-open `lo..hi` (int bounds), three materializing / value-semantic uses: list construction (`0..n`), lazy for-each (`for i in 0..n`, no list built), and slicing (`$xs[a..b]` + open forms, over list / bytes / string). `lo > hi` a positioned error; materialisation bounded by a catchable `limits.MaxRangeElements` (int64-span-overflow-safe, not the uncatchable `makeslice` panic). New `RangeExpr` / `SliceExpr` AST; `fmt` emits `..` tight. |
| M21.12 | per-frame allocation elimination | A call / block frame now does **no** per-binding or per-call heap allocation: a slot-backed binding (`Slot >= 0`) writes only the pooled `slots` slice (the identifier travels in `Binding.Name`; the rare name-based readers scan the small slot slice via `lookupLocal`), and `evalCall` binds args interleaved with no intermediate `[]Value`. Recursive fib ~300k -> ~59 allocs/op; Go's minor page faults fell ~7x. Value semantics and the `vars` fallback (REPL) intact; guarded by `TestFrameAllocationsStayLow`. |
| M21.13 | Windows installer | An Inno Setup script (`packaging/windows/jennifer.iss`) built by a `windows-latest` CI job into `jennifer-<ver>-setup.exe`: per-user (no admin), adds to `PATH`, bundles the system modules + sets `JENNIFER_SYSMODDIR`, opt-in `.j` association, Apps & Features uninstaller. Unsigned, best-effort **unsupported** build. `scripts/build-windows-installer.sh` recreates it locally via Wine. Promoting Windows to *supported* is the follow-on, horizon `DRAFT#22`. |

Cross-cutting threads:

- **The `crypto` asymmetric surface** grew here, not in M20: M21.3 added
  `rsaSign` / `rsaVerify` / `ecdsaSign` / `ecdsaVerify` (PEM; PKCS#1 v1.5 / JOSE
  R||S) for JWT interop, and M21.4 added `rsaGenerateKey` / `ecGenerateKey` /
  `jwkPublic` / `csr` for ACME - all build-tag split off the TinyGo build
  (`crypto/x509` is absent there), so the asymmetric verbs are default-binary
  only while the symmetric primitives and Ed25519 stay on both.
- **A 64 MiB received-data cap** became the standard DoS guard across the network
  clients (`http` / `redis` / `pop` / `imap` / `mqtt` / `websocket`, in M21.2 /
  M21.7 / M21.9), so an attacker-declared length or an unbounded stream fails
  catchably instead of OOMing.
- **Not all of M21 is modules.** M21.8 / M21.11 / M21.12 are interpreter and
  language work (the call-depth guard, range syntax, the allocation model), M21.10
  is the throughput library, and M21.13 is packaging - which is exactly why this
  bucket is the general catch-all rather than an M18 module run.
- **The `sql` list-spread** (M21.5) - `toDriverArgs` spreading a single `list`
  argument into the placeholder sequence - is the one small language-adjacent
  enabler this track pulled in, for runtime-shaped parameterized queries.

---

## M22 - additional libraries and language refinements

Post-backlog work that belongs to neither the M20 system-library set nor the M21
backlog: focused standard-library and module additions or enhancements, and small
language cleanups, each landing when the need is concrete. A new library entry
ships the usual surface (a Go package under `internal/lib/`, a line in
`internal/stdlib.InstallAll`, a `docs/libraries/` reference, cheatsheet rows, a
`JENNIFER.md` bullet); a `.j` module change ships its `*_test.j` overlay, its
`docs/modules/` doc, and a `JENNIFER.md` bullet.

### M22.1-M22.3 - `path` library, digit identifiers, library renames (compacted)

**All done.** The first three M22 items: one new library, one language-rule
relaxation, and the breaking renames the relaxation unlocked. Per-item surface
detail lives in `docs/libraries/`, `JENNIFER.md`, and (for the rejected shortcut)
`docs/technical/rejected.md`; this table is the milestone index.

| M#    | Topic | Summary |
| ----- | ----- | ------- |
| M22.1 | `path` library | OS-aware path manipulation over Go `path/filepath`, the pure-string counterpart to `fs`'s I/O (so `.j` stops hand-rolling separator splits / hardcoding `/`). `use path;` -> `base` / `dir` / `ext` / `stem` / `join` (variadic) / `clean` / `isAbs` / `split` (`-> [dir, file]`). Its own library, not a `basename` on `fs`: path logic is string work that never touches disk, and it has 5+ functions (mirrors Go's `path/filepath` vs `os`/`io`). Manipulation subset only - `Abs` / `Glob` / `Walk` / `EvalSymlinks` excluded (they need the disk, belong with `fs`), so **no build-tag split**, TinyGo-clean, both binaries. Uses the host separator (portable; pairs with `os.DIRSEP`). Explicitly **not** a filename sanitizer - OS-aware `base` won't strip a foreign `\` on Linux. |
| M22.2 | digits in identifiers | Relaxed the letters-only rule to allow interior / trailing digits, keeping identifiers **letter-initial** (graduated `DRAFT#20`); **additive, non-breaking** (every prior identifier stays valid). Regular idents `[A-Za-z][A-Za-z0-9]*` (still no `_`, <= 64); constants `[A-Z][A-Z0-9]*(_[A-Z][A-Z0-9]*)*` (ALLCAPS, digits within a chunk, `_` still constants-only) - so `sha256` / `SHA256` / `HTTP2` / `SCRAM_SHA256` are legal, `AES_256` still illegal (write `AES256`). Applies **uniformly** to every letter-initial identifier (the lexer emits one `IDENT` and can't scope digits to method names): variables, params, methods, struct type **and field** names (`x1` / `ipv4` / `md5` fields), library / `use as` / `import as` aliases; constants are the one separately-lexed class; paths / map keys are strings and already had digits. Letter-initial keeps lexing unambiguous (digit-first is always a number). Touched the lexer + constant validator + spec + `JENNIFER.md` + grammar EBNF / PEG + naming guidance + every editor highlighter. Ends the euphemism tax; the breaking renames land as M22.3. |
| M22.3 | library renames | The euphemism-dropping renames the digit rule unlocked, landed together pre-1.0 (each semver-breaking, no deprecation window): `use iic;` -> `use i2c;`; `uuid.generate("v4")` / `generate("v7")` -> `uuid.v4()` / `uuid.v7()` (the version is a real method; `generate` removed); `crypto.pbkdf(...)` -> `crypto.pbkdf2(...)`. **Not renamed:** `binary` (named for the reserved `bytes` keyword, not a digit) and `intl` (modeled on JS `Intl`, not `i18n`). One batch updated each library, its `*_test.j` overlay + Go integration test, `docs/`, `JENNIFER.md`, the cheatsheet, and every caller. |

**Rejected in M22.3 (stance #1):** per-algorithm digest shortcuts
(`hash.sha256(b)` / `crc.crc32(b)`) were briefly added beside `compute(b, algo)`,
then removed - they are a parallel API for one job, and the family neighbours
(`hash.hmac`, `crypto.pbkdf2` / `hkdf`) are irreducibly algorithm-as-value
(SCRAM / JWT / TLS negotiate the hash; `sasl.j` threads a runtime `$s.algo`
through all of them). So `compute(b, algo)` stays the single canonical form and
the algorithm is always a value across the whole hash / crypto family. Full
reasoning in `docs/technical/rejected.md`.

### M22.4 - `match` statement (multi-way value dispatch, compacted)

**Done.** A multi-way branch - `match (EXPR) { when V [, V ...] { } ... else { } }` -
so a chain of `if` / `elseif` over one subject reads as a single dispatch.
Keywords `match` / `when` (reserved - a pre-1.0 break, renamed collisions in
`totp.j` / `feed_test.j`) chosen so it can *grow* into pattern matching (`M22.5`
adds enum-variant patterns to the same `when`, no new keyword).
**Semantics** (v1, value dispatch only): the parenthesized subject is evaluated
**once** and compared to each arm's values by strict `==` (`Value.Equal`, the
operator's path); the first matching arm wins; an arm's values short-circuit
left-to-right; **no fall-through**, and `match` is **not a `break` target**
(`break` / `continue` act on the enclosing loop, so Go's "break breaks the switch"
trap is unrepresentable); optional `else` last; no-match-no-`else` is a no-op;
each arm is its own scope; a statement, not an expression. **`fmt`** lays it out
as a flat case list (like `switch`/`case` everywhere) - each `when` / `else` on
its own line at the arm indent, **not** cuddling the previous `}` (a `match` arm
is a case, unlike an `if`'s `} else {` tail); wrapped value lists align under the
first value. **Implementation:** new `MatchStmt{Subject, []MatchArm{Values, Body},
Else}` AST, `execMatch` runs the first matching arm via `execBlock`; the header
`{` ambiguity (`when Name {` vs a `Name{...}` struct literal) is resolved by the
parser's `noStructLit` flag - set across a `when` value list, re-enabled inside
`(` / `[` / call / list / map bodies (Go's `exprLev` rule) - so a composite-literal
value takes parens (`when (Point{x:1}) {`). `ast` gained the node cases, `tokens` /
`profile` / `repl` needed none (generic handling), four editor highlighters + the
docs bundle updated. Full spec in CLAUDE.md; verified across behavior /
fmt-idempotence / `-race` / a golden example.

### M22.5 - sum types (enums) + pattern `match`

**Done** (graduated from `DRAFT#19`; the payoff `M22.4`'s `match` was designed
to grow into). Structs model a *record*; there is no way to model "one of N
variants," and the interpreter's own `Value` is already a tagged union - this just
exposes that shape to the language, with `match` as the way to consume it.

**Shipped as designed, with two deliberate refinements decided during
implementation:**

- **Naming is case-agnostic; no PascalCase / camelCase convention is enforced.**
  An early sketch leaned on capitalisation to tell an enum construction
  (`Shape.Empty`) apart from a namespace member (`bio.lower`) at parse time. That
  was rejected as a parser shortcut that burdens a *teaching* language with an
  incidental "start lowercase here, uppercase there" rule for no user benefit.
  Instead, `Prefix.Member` is resolved **at eval from the tables** - is `Prefix`
  an enum? then `Member` is a variant; otherwise it is a namespace / module
  member. Enum and variant names may be spelled any way. The **only** convention
  in play is the one the language already teaches: an all-`UPPERCASE` name is a
  *constant*, so an enum *type* named all-caps would read as a constant in
  `Name.member` position (name types normally and it never comes up). A bare
  `namespace.fn` (a call missing its `()`) now gets a "that's a function, call it
  as `namespace.fn(...)`" error at eval instead of a parse-time
  "must be uppercase".
- **Pattern matching covers a local / same-module enum subject.** Exhaustiveness
  and variant-pattern resolution run in the parse-time resolver, which sees the
  program's own enums; matching an *imported* module enum from the importing
  program is not auto-detected (the owning module matches its own enums, and
  callers dispatch through a module function). Cross-module **identity,
  construction, value semantics, and zero values** all work.

The `Value` gained a `KindEnum` kind (a `(ns, name)` type + a `Variant` tag +
the payload `Fields`), mirroring `KindStruct` for copy / equality / deep-const /
`MatchesDeclared` / module retagging; the parser reuses the `StructLit` node for
braced construction (with a third segment for the cross-module form) and defers
the payload-less bare form to eval; the resolver tracks binding types so an
enum-typed `match` subject drives pattern arms + the exhaustiveness check; `fmt`
reflows an enum like a struct def; a golden `examples/enum.j` and lexer / parser /
interpreter / cross-module tests pin it.

**Declaration + construction:**

```jennifer
def enum Shape {
    Circle { r as float },
    Rect { w as float, h as float },
    Empty
};
def s as Shape init Shape.Circle{ r: 2.0 };   # or Shape.Empty
```

- **`def enum Name { Variant [ { field as type, ... } ], ... };`** at top level,
  hoisted before the first statement like `def struct`. Each variant is
  payload-less or carries named fields (a mini-struct). Construction mirrors
  struct literals: `Name.Variant{ ... }` / `Name.Variant` (payload-less).
- Cross-module enum identity keyed by canonical path, exactly like module structs
  (`ALIAS.Enum` / `ALIAS.Enum.Variant{...}`).

**Consumption - extends `M22.4`'s `match` / `when` with variant patterns:**

```jennifer
match ($s) {
    when Circle(c) { io.printf("area %f\n", math.PI * $c.r * $c.r); }
    when Rect(rc)  { io.printf("area %f\n", $rc.w * $rc.h); }
    when Empty     { io.printf("empty\n"); }
}
```

- A pattern arm `when Variant(bind) { block }` binds the variant's payload into a
  **fresh per-arm scope** (`$bind` is the mini-struct), reusing exactly the arm
  scope machinery `M22.4` already builds - value arms and pattern arms coexist,
  disambiguated at resolve time by whether the arm head is an enum variant.
- **Exhaustiveness** for an enum subject is checked at **resolve time**: every
  variant must be covered, or an `else` arm present, else a positioned parse
  error. (Value-`match` over `int` / `string` stays non-exhaustive - there is no
  finite variant set to check.) This is the strict, positioned-error stance
  applied to control flow; a forgotten variant is a compile error, not a silent
  no-op.

**Implementation surface.** A new `Value` kind **`KindEnum`** mirroring
`KindStruct` almost exactly - a `(namespace, name)` discriminant (which variant)
plus a `Fields` payload - so it inherits value semantics, deep `const`,
`Copy` / `DeepCopy`, and `MatchesDeclared` with little new machinery, and stays
tagged-union / reflect-free (TinyGo-clean, no frame-pool concern). Parser: the
`def enum` declaration and `Name.Variant{...}` construction, plus the pattern-arm
grammar in `match` (payload slots resolved into the arm block scope via
`borrowBlockEnv`). Resolver: the exhaustiveness check. Interpreter: `execMatch`
extended to test the active variant and bind its payload. Plus grammar EBNF / PEG,
the spec (`CLAUDE.md`, `JENNIFER.md`), the four editor highlighters, the
`types-and-values` + `control-flow` user-guide pages, and lexer / parser /
interpreter tests with a golden example.

**Requires:** **`M22.4`** (the `match` / `when` construct pattern arms extend).
Relates to `DRAFT#18` (first-class functions - a `match` result plus function
values together give the functional-core idioms).

### M22.6 - TLS options for `http` / `rest`

**Done.** Let the `http` client (and `rest` on top) reach an HTTPS server with
a self-signed or private-CA certificate, by threading TLS options through to
`net.connectTLS`. Today `http` always full-verifies - `modules/http.j` calls
`net.connectTLS($addr, DEFAULT_TIMEOUT_MS)` with no options - so it cannot talk to
the self-signed certs that LAN appliances (Proxmox, Synology, an internal service)
ship by default. This is the shared enabler for that whole "manage my own
infrastructure" family. It needs **no interpreter or system-library change**:
`net.connectTLS` already accepts `net.TLSOptions{ skipVerify, caCert }` (`M16.14`),
so the work is purely plumbing it through the two `.j` modules.

**Shape:**

- **`http`** carries TLS options on the request path: an `http.TlsOptions` struct
  (`{ skipVerify as bool, caCert as bytes }`, mirroring `net.TLSOptions`) plus
  options-taking send variants `http.requestTls(method, url, headers, body, tls)`
  and `http.requestWithTls(..., timeoutMs, maxBytes, tls)` (`http` is
  function-based, with no `Request` struct to hang a field on). `https://` with no
  options - `request` and the verb shortcuts, which pass the zero `TlsOptions` -
  behaves exactly as today.
- **`rest`** exposes it per-client (a `rest.Client` already bundles base URL +
  default headers): a `tls as http.TlsOptions` field, a `rest.client(baseUrl)`
  constructor (needed since the added required field breaks the old bare
  `Client{baseUrl, headers}` literal), and builders mirroring `rest.withHeader` -
  `rest.insecure(client)` (skip verification) and `rest.withCA(client, pem)`
  (trust a PEM CA) - applied to every request.
- **Secure by default.** Verification stays **on** unless explicitly relaxed;
  `skipVerify` is opt-in with a MITM-risk note, `caCert` is the safer path (trust
  the appliance's own CA). Matches `net.TLSOptions` semantics and the security
  model (opt-in, documented, for a trusted-LAN endpoint - never a default).

**Files:** `modules/http.j` + `modules/rest.j` (struct / builders / thread to
`net.connectTLS`), their `*_test.j` overlays (TLS-builder shape tests),
`docs/modules/http.md` + `rest.md`, `JENNIFER.md` bullets, the `rest_demo.j`
constructor update. Default-binary-only (net-backed), so no TinyGo concern.
**Verified:** Go integration tests (`cmd/jennifer/http_tls_test.go`) drive both
surfaces against a self-signed loopback - the default request refuses the
untrusted cert, `skipVerify` accepts it, and `withCA` trusts the server's own
cert - plus a full Jennifer `httpd.listenTLS` round-trip. A real end-to-end
test, not just a shape check.

Dogfoods **M22.9**: `rest.Client.tls` is a module struct (`http.TlsOptions`) used
as a struct field across the `main -> rest -> http` boundary.

### M22.7 - `graphql` (GraphQL client module)

**Planned.** A thin GraphQL client `.j` module: build a request against one
endpoint, POST `{ "query": ..., "variables": ... }`, and read the JSON response.
GraphQL is a stable, general protocol (GitHub, GitLab, Shopify, Hasura - and the
immediate motivation, Unraid's official API - all speak it), so unlike a
vendor-specific NAS client (`DRAFT#24`) it belongs in **core `modules/` as a peer
to `http` / `rest`**, not a deck. It is a `.j` module, not a Go library: the query
is an opaque string the caller supplies (no need to parse GraphQL syntax - that is
the server's job) and the response is JSON the `json` library already handles, so
there is no per-byte hot path.

**Shape:** a `graphql.Client` layered on a `rest.Client` (endpoint URL + auth
header, and - via `M22.6` - TLS options for a self-signed host), plus
`graphql.query(client, query, variables) -> json.Value`. The one
GraphQL-specific wrinkle to get right: **a GraphQL error is an HTTP 200 with an
`errors` array in the body**, not a non-2xx status - so `query` must inspect the
payload and raise a positioned `Error` (kind `"graphql"`) carrying the server's
messages, rather than trusting the status line. `variables` is a `json.Value` /
`map`; a mutation is just a query string, so no separate verb is required (a
`mutation` alias can be added if it reads better).

**Files:** `modules/graphql.j` + `modules/graphql_test.j` (100%),
`docs/modules/graphql.md`, a `JENNIFER.md` bullet, a demo. Default-binary-only
(net-backed via `http` / `rest`). **Requires:** builds on `http` / `rest` +
`json`; `M22.6` for self-signed endpoints (the homelab case). It is the GraphQL
dependency the Unraid client in `DRAFT#24` consumes.

### M22.8 - self-referential struct guard

**Done.** A struct that contains itself **by value** - directly
(`def struct Node { v as int, next as Node }`) or mutually (A holds a B that
holds an A) - has no finite zero value under Jennifer's value semantics (there is
no null / pointer struct field to terminate it). It used to be accepted at
declaration and then **fatally stack-overflow** (an uncatchable Go crash, past
the call-depth cap) the moment its zero value or a literal was built. This turns
that into a **positioned, actionable error at hoist time**:

```
struct "Node" cannot contain itself by value (field "next" is "Node"); a by-value
struct cycle has no finite zero value - use `list of Node` (or a `map`) for
recursive data
```

- `Interpreter.checkStructCycles` runs once **after** all top-level structs are
  hoisted (so mutual cycles across the whole set are visible), at both entry
  points (`Run` and `EvalInteractive`); module structs are checked via their
  sub-interpreter's `Run`. A DFS with gray/black colouring over each struct's
  **direct** struct-typed fields finds a back-edge.
- Only a **direct, local user-struct** field is a by-value edge. Recursion
  *through* a `list` / `map` / `task` field is allowed (their zero is
  empty / a handle - finite), and a module- or library-struct field can never
  cycle back to a local struct. So `def struct Tree { v as int, kids as list of
  Tree };` and ordinary nesting (`Line { from as Point, to as Point }`) stay
  legal - the guard only rejects the genuinely-infinite shapes.
- Pinned by `TestStructSelfReferenceRejected` (direct, mutual, allowed
  list-recursion, allowed nesting).

**Adjacent gap (now fixed):** a **module** struct used as a *struct field type*
was a separate cross-module field-identity bug, fixed in `M22.9` (below).

### M22.9 - module structs as struct fields (cross-module field identity)

**Done.** A module struct used as a **struct field type** now type-checks. It was
previously rejected with "expects geo.Point, got struct" even though the same
module struct works fine as a variable / parameter type (`def p as geo.Point`).
Two related manifestations, both fixed:

- **A main-program struct with a module-struct field**
  (`def struct Line { from as geo.Point };`): `resolveDeclaredTypesOnce` (which
  stamps the module's `(stem, path)` identity onto declared types after
  `loadModuleImports`) never walked **struct field types** - only variable /
  parameter types and `list` / `map` elements. So the field kept its unresolved
  alias namespace while the field value carried the module identity, and the
  check mismatched. Fix: also stamp each hoisted struct's field types in that
  pass (recursing into `list` / `map` elements, so `pts as list of geo.Point`
  resolves too). The `StructDef` pointers are shared with `i.structs`, so this
  stamps the hoisted defs.
- **A module struct whose own field references a sibling module struct**
  (`Seg { a as Point }`), used across the import boundary: the module's field
  types are bare (module-internal) but the boundary-crossed field values carry
  the module's `(ns, path)` identity. Fix: retag the bare own-struct field types
  to the module identity at the boundary check - at both struct-literal
  construction and field assignment - via the existing `retagType` helper (which
  already handles the same retag for `list` / `map` element types).

Value semantics, chained lvalues into a nested module-struct field
(`$L.from.x = 5`), and passing a nested field back into a module function all
work; a wrong-typed field value is still rejected. Pinned by
`TestModuleStructAsFieldTypeInMainStruct` and
`TestModuleStructWithSiblingStructField`.

### M22.10 - byte-capable `http` download

**Done.** The `http` client could not fetch a binary payload (a `.tar.gz`, an
image): every response body was built as `convert.stringFromBytes(_, "utf-8")`,
which is strict (throws on non-UTF-8 by the "strict at boundaries" stance), so
`http.request` on a gzip URL threw `stringFromBytes: input is not valid UTF-8`.
The whole stack **up to that last step was already byte-exact** (socket read,
Content-Length trim, chunked `dechunk` all operate on `bytes`), so the fix was an
additive byte path, not a rewrite:

- **`http.BytesResponse`** - the byte-safe twin of `Response`, with `body as
  bytes` instead of `string`.
- **`http.requestBytes` / `requestWithBytes` / `getBytes`** - return a
  `BytesResponse` with the raw body. `requestWithBytes` carries the explicit
  timeout / `maxBytes` (negative = unbounded, for a large archive) / `TlsOptions`.
- Internally, `parseResponse` split into `parseRaw` (returns the byte-exact
  `BytesResponse` core) plus a thin text decoder, and `sendCore` now returns the
  raw wire bytes; the text verbs are unchanged (still throw on a non-UTF-8 body,
  by design). `rest` stays text/JSON-only.

Verified end-to-end in `cmd/jennifer/http_bytes_test.go` (a gzip served over
loopback downloads with matching sha256 and unpacks back, while the text `get`
refuses the same body) plus a `parseRaw`-keeps-binary overlay test.

### M22.11-M22.13 - Hardening

**All done.** The three grouped milestones from two security / robustness audits.
Every fix ships with a test (a module `*_test.j` overlay case or a Go table /
integration test). **Per-finding detail (severity, reproducer, code sites) lives
in the two report files**; this table is the milestone index, and the reasoned
non-literal choices follow it.

| M#     | Theme | Findings -> what shipped |
| ------ | ----- | ------------------------ |
| M22.11 | injection & output-encoding | `OM-002` orm identifier/operator allowlists + quoted idents; `OM-006` imap/pop `rejectControl` (CRLF); `OM-007` statsd metric name/value validation; `OM-011` htmlwriter tag/attr name check + exported `safeUrl` (markdown's `safeHref` delegates to it); `OM-012` tengine no-auto-escape SECURITY warning; `OF-003` json.encode HTML-escapes `< > & U+2028 U+2029` as `\u00xx`; `OF-008` new `hash.equal` (constant-time); `OM-020` http.parseUrl splits `?`/`#` before the authority; `OM-021` dotenv env-name validation; `OM-010` ipnet folds v4-mapped `::ffff:0:0/96` to a v4 `Address` (new `unmap`). |
| M22.12 | network / resource / path robustness | `OM-004` memcache/mikrotik/amqp 64 MiB cap on server-declared lengths; `OM-005` mikrotik connect+read timeouts + cleartext-default warning; `OM-017` memcache/label connect timeouts; `OF-005` net.readAll defaults to a 256 MiB cap (negative = unlimited); `OF-009` net.startTLS snapshots state under `s.mu` + holds `s.readMu` across the handshake; `OF-007` bounded handle registries (sql/fs/os/compress/net) + sql 30 s query/exec deadline + `sql.CloseAll` CLI teardown; `OF-013` archive per-entry size budget (no `uint64` wrap); `OF-004` httpd.serveDir rejects `\` + re-verifies containment; `OF-011` httpd chmods a `unix:` socket to 0660; `OF-012` documented the serveFile/serveDir symlink follow; `OF-010` sql redacts the DSN password from errors; `OF-014` json/toml/yaml write-API depth guard (bounds the tree at construction, covering encode / `%v` / `==` / `DeepCopy`); `OM-014` flatdb.save preserves the target's mode (0600 default) + removes the temp file on a failed rename. |
| M22.13 | web framework | `OM-008` web.sessionId trusts only a minted-UUID-shaped cookie (re-mints otherwise) + new `web.renewSession`; `OM-009` new `web.onError` hook, else a swallowed handler error goes to stderr; `OM-013` web.form/percentDecode decode leniently + csrfCheck gates the form fallback on content type; `OM-019` a `HEAD` request is served by the matching `GET` route; `OF-017` httpd stops its two per-request timers (`NewTimer` + `defer Stop`). |

**Reasoned non-literal choices** (the fix chosen over the literal one, with why):

- **`OM-003`** (web serial handling, HIGH) - kept **serial** for v1 and documented
  loudly, not a `spawn` pool: dispatch runs through `meta.callMain` into the entry
  program, which shares one root `Environment` (the `callDepth` counter and
  globals), so concurrent handlers would race on interpreter state. Per-handler
  concurrency needs interpreter-level work first.
- **`OF-006`** (unrestricted `meta.call`, a design note) - shipped the prominent
  "match untrusted input against a program-defined allowlist first" warning + a
  worked example in `meta.md`, not a new registration primitive (a two-line `.j`
  allowlist already makes the safe path easy; `web` only dispatches
  author-registered route-handler names, never request data).
- **`OM-012`** took the tengine SECURITY warning, not an auto-escape mode (keeps
  the advertised `text/template` semantics); **`OF-003`** the inline `\u00xx`
  escape, not a separate `strings.escapeHtml`.
- **`OM-010`** folds only the well-defined `::ffff:0:0/96` mapped form; the
  deprecated IPv4-compatible `::a.b.c.d` is left as v6 (ambiguous with `::1`).
- **`OF-007`** applied the full bounds + deadline + teardown to `sql` (the worst
  instance) and registry bounds to `fs` / `os` / `compress` / `net`; httpd requests
  already self-unregister and its listeners are OS-fd-bounded. `OF-013` stays
  defence in depth (the extraction-time `readCapped` budget is authoritative).

### M22.14 - `imap` criteria-based search

**Done.** `imap.search(session)` (which was `SEARCH ALL`, redundant with the
mailbox count) is now `imap.search(session, criteria)` - a **breaking** signature
change (pre-1.0), where an empty `imap.criteria()` reproduces the old behaviour.
The new `imap.Criteria` struct filters as a **hybrid**: server-side fields map to
one IMAP `SEARCH` (substring on `subject`/`from`/`to`/`text`, a `since` / `before`
day-granular date range as `time.Time` values, flag state, size - all ANDed, one
round-trip, no bodies), and client-side fields refine the returned candidates by
fetching only their headers / structure (`subjectRegex` / `fromRegex` on the
decoded header - what IMAP substring search can't do - and `hasAttachments` via a
`BODYSTRUCTURE` disposition heuristic, no body download). Criteria strings can't
inject a command: substrings go through `quoteArg` (control-checked + escaped +
quoted), dates are rendered from `time.Time` (a controlled `dd-Mon-yyyy`, no
user string), sizes are ints, and `command()` control-checks the whole line. The
`since`/`before` fields are plain `time.Time` (a zero value = unset, rendered
internally with `time.format` - no bespoke date type). IMAP `SEARCH` is
day-granular, so a bound carrying a **time-of-day** is transparently refined to
the exact instant client-side (against each candidate's `INTERNALDATE`, the clock
`SEARCH` filters on; the `BEFORE` server date widens by a day so its own day's
earlier messages survive for the refinement) - no extra parameter, a midnight
bound stays pure server-side. `imap.criteria()` builds an empty filter; pure
builders (`buildSearchCommand` / `bodyStructureShowsAttachment` /
`parseInternalDate` / `hasTimeOfDay`) are white-box-tested in the overlay, the
client refinement end-to-end in the Go suite. Follow-ups noted in `imap.md`:
non-ASCII `SEARCH` strings, UID search. Pure `.j` (adds a `use time;`,
TinyGo-clean), both binaries build.

### M22.15 - `imap` folder browsing, APPEND, and folder rename

**Done.** Rounded the `imap` client from read-plus-manage into a complete
read / browse / manage / **save** client by adding the core IMAP4rev1 commands it
was missing, plus a user-facing terminology rename:

- **`folders(session, pattern)`** (`LIST`) - enumerate folders as `imap.Folder`
  (`name`, `delimiter`, `flags`), so a program can discover the folder tree
  instead of hardcoding `"INBOX"`. **`status(session, folder)`** (`STATUS`) -
  message / unseen / recent / uidnext / uidvalidity counts **without** selecting
  (folder badges, "new mail?" polls). Both parse via pure, overlay-tested helpers
  (`parseListLine` / `parseStatus`).
- **`append(session, folder, message)`** / **`appendWith(..., flags, ...)`**
  (`APPEND`) - upload a full RFC 5322 message (save to Sent, store a Draft),
  closing the read-only-ish gap. Uses the synchronizing-literal continuation flow
  (`writeLine` the `{N}` head, wait for `+`, send the byte-counted body, read the
  tagged completion); the mailbox is quoted and the head control-checked, the body
  is an opaque literal.
- **Terminology: "mailbox" -> "folder"** across the user-facing surface (a pre-1.0
  breaking rename, since consistency forbids `selectMailbox` beside a `Folder`
  struct): `selectMailbox` -> `selectFolder`, `createMailbox` -> `createFolder`,
  the `Mailbox` struct -> `Folder`, and the `mailbox` parameters -> `folder`.
  "Folder" is what every mail client and user calls these; the IMAP-spec term is
  noted once in the docs for discoverability. `list` was unavailable (reserved
  type keyword), so the LIST verb is `folders`.

End-to-end LIST / STATUS / both APPEND variants are covered by the Go IMAP mock
(which grew a literal-continuation `APPEND` responder). Pure `.j`, both binaries
build.

---

### M22.16 - core hardening sweep

**Done.** A batch of small correctness / resource / performance residuals in
the `internal/` tree - each self-contained and independent of the module work,
landed as one reviewed pass:

- **`sql` cursor deadline** - the query timeout currently governs the whole cursor
  lifetime, so a `sql.next` loop reading a large table for longer than the deadline
  dies mid-iteration and looks like a database failure. Split the two concerns: a
  short deadline on acquiring the connection / issuing the statement, and either no
  deadline or a caller-settable one on reading the cursor (`sql.setQueryTimeout(ms)`
  or an `open` option), with error text that names the client as the source.
- **`httpd` registry bound** - the server (and per-request) registries are the one
  instance of the registry pattern the earlier hardening pass left unbounded; add
  the same `maxServers` pre-insert check and catchable "too many open" error the
  five other registries already carry.
- **`hash` md5 / sha1 labelling** - mark md5 and sha1 non-cryptographic in the
  `algoList` error text, the `docs/libraries/hash.md` algorithm table, and the
  `hash.hmac` doc comment, so a checksum use is not confused with a security one
  (both stay available for legacy interop).
- **write-depth guard cost** - `json` / `yaml` / `toml` re-scan the entire result
  after every `set` / `insert` / `append` to enforce the nesting cap; have the
  write helpers compare the depth of the node they touched (they already walk the
  pointer path) instead, with a subtree-only scan for `move`, and fold the three
  identical `exceedsDepth` copies into one shared helper.
- **`include` expansion cap** - per-path cycle detection (correct for diamonds)
  lets a chain that includes the previous file twice expand to 2^n tokens, so ~30
  tiny files exhaust memory; cap total spliced tokens (or include depth) with a
  positioned preprocessor error. Untrusted *code* is outside the threat model, but
  this is an easy accident in a generated-source pipeline.
- **`printf` field cap** - the field cap is a generous `1 << 20`; lower it to a few
  thousand (terminal layout never needs a megabyte-wide field) and replace the
  `for width(n) < pad { n++ }` O(Pad) spin with the closed form.
- **decoder error positions** - `json` / `xml` recompute line / column by
  rescanning from offset 0 on every error; O(n) once per failed decode today, a
  trap only if a future change accumulates diagnostics. Track position
  incrementally so it does not get rediscovered.

---

### M22.17 - web hardening (concurrent dispatch)

**Planned.** Turn the `web` framework from strictly-serial into safely-concurrent
request handling, and fix the error-reporting hook a module can never feed a typed
`Error`. Interpreter-first: the module-level change depends on core work, so the
order below is mandatory.

The naive fix - wrap `handleOne` in a `spawn` - was **reproduced to be unsafe**.
Under `go build -race`, a `/fast` request issued 300 ms into a 2 s handler does
drop from ~1700 ms to ~3 ms, but the race detector reports a data race on the
call-depth counter (`root.callDepth++`). That counter is meant to live on the
per-goroutine root env, but a spawned worker dispatching a handler through
`meta.callMain` re-roots the handler frame at the shared host (and shared module)
global, so concurrent workers mutate one shared `callDepth`. The M23 note that
`web` stays serial *because concurrent dispatch races on shared interpreter state*
is therefore correct as stated; the constraint is removed here properly, not
papered over:

1. **`Error` across `meta.callMain`** - the retag path stamps a module's struct
   arguments with the module identity, but `Error` is auto-injected into every
   interpreter, so `isOwnStructName("Error")` is true and a module's `Error`
   arrives stamped with the module path, unbindable to an entry-program `as Error`
   parameter. Track a module's *declared* structs separately from the
   auto-registered `Error` and retag only the former, so a module can hand an
   `Error` back across the boundary. (Regression test: a module that returns and
   accepts an `Error` across `meta.callMain` in both directions.)
2. **Race-safe concurrent host / module dispatch** - the prerequisite for spawning
   handlers: move `callDepth` off the shared root env (goroutine-local), then audit
   every other write to a shared `i.global` / module global reachable from a
   concurrently-dispatched handler (global `def` / assign, the `CallExpr.Method` /
   `Fn` node caches, the map hash-index, profiler / diag fields), and gate the
   whole change behind a concurrent-handler stress test run under `-race`.
3. **`web` spawn-per-request** - only after (2), wrap per-request handling in a
   `spawn` (errors caught inside the spawn so nothing escapes as an unwaited-task
   error; concurrency already bounded by `httpd`'s `maxInFlight`). Update the
   header comment, `docs/modules/web.md`, `modules/README.md`, and `JENNIFER.md`,
   all of which currently assert serial handling in bold, and reconcile the M23
   web non-goal note. The 1700 ms -> 3 ms probe becomes the regression gate.
4. **`web.onError` fail-safe** - depends on (1). Today registering an error handler
   *replaces* the working stderr fallback and, because the `Error` is unbindable,
   loses the error entirely - a deployment that follows the docs gets *less*
   diagnostic output than one that ignores the hook. Report to stderr *in addition
   to* calling the handler, correct the doc comment that promises a typed `Error`,
   and add a real end-to-end test (register a handler, request a throwing route,
   assert the handler observed the error's `kind` / `message`) - the current test
   asserts only that the name was stored, which is why the dead hook shipped green.

The reproduction that the spawn-per-request fix races on the shared call-depth
counter gets a reasoning record in `docs/technical/design-decisions.md` when the
milestone lands, since the concurrent-dispatch design turns on it.

---

### M22.18 - `dotenv` layering, profiles, interpolation

**Planned.** Grow `dotenv` from a single-file `parse` / `read` / `load` into a
layered loader with environment profiles, `${VAR}` interpolation, and multi-line
values - the dotenv-flow / Rails / Next.js shape, built with Jennifer's
explicit-over-implicit stance and a strict security posture. Pure `.j` module
work; **no interpreter change** in this cut (a CLI `--env` flag is parked beyond
1.0.0 as `docs/horizon.md` `DRAFT#26`). Consolidates here the `${VAR}`
interpolation + multi-line items that were parked in `M23.8`.

**Layered load.** Read, later overriding earlier, skipping absent files, from a
single explicit base directory (`dir`, usually `os.cwd()`):
`.env` -> `.env.local` -> `.env.<profile>` -> `.env.<profile>.local`. Two
independent precedence axes:

- **Profile label** (which `<profile>`): `JENNIFER_ENV` env var, else `""` (no
  profile - meaning load the base files only, *not* a `.env.default` file).
- **Value** (which wins, highest first): a **pre-existing OS env var** (never
  overwritten - real env wins) > `.env.<profile>.local` > `.env.<profile>` >
  `.env.local` > `.env`.

**Surface** (exported):

- `parse(text)` - enhanced with `${VAR}` interpolation + multi-line (below);
  single-file `read(path)` / `load(path)` stay as the primitives, also enhanced.
- `readCascade(dir, profile)` - the file layers merged into a map, **no env
  mutation** (the explicit path).
- `resolve(dir, profile)` - the **effective** map: `readCascade` overlaid by the
  real OS env (real env wins), for programs that read the map instead of
  `os.getEnv`.
- `loadCascade(dir, profile)` - `readCascade` then `os.setEnv(k, v)` **only for
  keys not already set** in the OS env (real env wins). Returns the file map.
- `autoload(dir)` - convenience: `loadCascade(dir, <JENNIFER_ENV>)`, documented as
  reading that one env var.

**Interpolation.** `${VAR}` (`VAR` shaped like `validEnvName`) resolves against
earlier-parsed keys (this file + earlier cascade files) -> real OS env -> empty
string. **Backward-reference only**, so cycles are impossible by construction (no
detection needed). It expands in unquoted and double-quoted values; single-quoted
values are fully literal (`'${X}'` stays literal). **No `$(...)` / backtick
command substitution** - those are plain characters; this is the hard security
line (a `.env` value can never execute a command). Multi-line double-quoted values
span physical lines (the parser tracks quote state across lines instead of the
current line-by-line split; an unterminated quote is a positioned error).

**Security posture**

- **Strict profile validation** - the label is matched against
  `^[A-Za-z0-9_-]{1,64}$` before building `.env.<profile>`, so a `JENNIFER_ENV`
  from an untrusted upstream cannot path-traverse (`../../etc/x`, `prod/../y`).
  `path.base` is not a sanitizer; validate up front. Builds on the existing
  `validEnvName` (`OM-021`).
- **Fixed base directory, no search / no walk-up** - loading from one explicit
  `dir` is what closes the DLL-hijack class (a walked-up "nearest `.env`" would let
  an attacker plant a file in a parent dir). `.env.local` overriding `.env` is safe
  because they share that developer-controlled directory.
- **Real env wins** - a committed file cannot clobber a deployment's real secret
  (a pre-1.0 behavior change from today's unconditional-override `load`; called out
  in the docs). To force-override, use `readCascade` + your own `os.setEnv` loop.
- Consistent with the security model: `.env*` files are **trusted local input**
  (the developer controls the working dir); dotenv is not a sandbox.

**Deliverables:** `modules/dotenv.j` + a 100% `modules/dotenv_test.j`, a Go
integration test (cascade order + real-env-wins + traversal rejection + multi-line
+ interpolation), `docs/modules/dotenv.md`, `modules/README.md` row, the
`JENNIFER.md` bullet, and `examples/modules/dotenv_demo.j`. Both binaries build
(pure `.j` over `fs` / `strings` / `os` / `path` / `regex`).

---

## M23 - module improvements

**Planned.** M22 lifted a handful of modules (notably `imap` and `http`); M23
generalizes that across the module ecosystem. A survey of all 63 `.j` modules
found the gaps **cluster into cross-cutting themes** rather than scattering per
module, so the sub-milestones are organized by theme: a shared pattern (a
receive loop, a persistent connection, a backend selector) is built once and
applied to every module that needs it. Rough priority is value x
how-many-users-hit-it. Each sub-milestone ships the usual per-module close-out (a
100%-passing `*_test.j` overlay, updated `docs/modules/` + `JENNIFER.md`, both
binaries build). **Deliberate non-goals** (not module bugs): `web` per-request
concurrency (an interpreter-level job - see M22.13), `password` hashing (needs
`x/crypto`), and fully-typed `orm` rows (awaits a language feature; the
query-builder half is in scope). Sub-milestones may grow their own sub-numbering
as they land.

### M23.1 - streaming / server-push read loops

**Planned.** The biggest structural theme: several protocol clients are
one-request/one-reply and cannot express an unsolicited server message, which
breaks the headline use case of each. Build one cooperative receive-loop-over-`net`
pattern (poll at a safe point, no callbacks; the app opts into a `spawn` for
concurrency), then apply it:
- **`redis`** - `SUBSCRIBE` / `PSUBSCRIBE` + a message-receive loop (and
  `PUBLISH`); plus pipelining (batch commands, read N replies) and `MULTI`/`EXEC`;
  `SCAN` to retire the production-unsafe `KEYS`.
- **`amqp`** - `Basic.Consume` (server-pushed delivery) beside the existing
  `Basic.Get`; exchange declaration + queue binding (direct/topic/fanout);
  message properties + `Basic.Nack`/requeue + publisher confirms.
- **`mqtt`** - QoS-1 (PUBACK handshake) publish/subscribe, a robust subscribe
  loop, retained messages + Last-Will, and auto-reconnect / session resumption.
- **`mikrotik`** - command tagging (`.tag`) + `/listen` for streaming commands
  (`/interface/monitor`, `/ping`) and `/cancel`; and clear the read deadline
  `readN` arms after it returns, so a later read / write does not inherit a stale
  one.
- **`imap`** - `IDLE` push new-mail notification (shares the loop; the rest of
  the mail work lives in M23.3).

### M23.2 - connection reuse / persistent sessions

**Planned.** Every call currently pays a fresh handshake. Add persistent,
reusable connections:
- **`http`** - keep-alive / connection reuse (a `Client` holding a persistent
  socket per host) so a request loop to one host reconnects once, not N times;
  automatic 3xx **redirect following**; a **cookie jar** (preserving multiple
  `Set-Cookie`); a Basic-auth helper; and **retry / backoff** on 429 / 5xx.
  `rest` inherits all of it and adds a per-request timeout + Link-header / cursor
  **pagination** iterator.
- **`smtp`** - a persistent `Session` (connect + EHLO + auth once, `send` many),
  so a queue of N messages pays one TLS+auth handshake instead of N.

### M23.3 - stable-identity verbs (cross-session correctness)

**Planned.** `imap` and `pop` both key off volatile sequence numbers, silently
breaking "fetch only what's new since last run":
- **`imap`** - `UID FETCH` / `SEARCH` / `STORE` / `COPY` (stable identifiers that
  survive expunge), native `MOVE` (RFC 6851), and ranged / partial `FETCH` for
  large bodies. Pairs with the `IDLE` from M23.1.
- **`pop`** - `UIDL` (per-message unique id -> leave-on-server / skip-seen),
  `TOP` (headers-only preview), `RSET` / `NOOP`.

### M23.4 - byte-exact binary values

**Planned.** `redis` and `memcache` decode bulk values as UTF-8, so an arbitrary
binary value (a serialized blob, a compressed payload) is not round-tripped. One
shared design across both: `bytes`-valued get/set variants (or make the existing
verbs byte-safe) so a non-UTF-8 value stores and loads exactly. Fold in the
adjacent low-cost verbs while here - `memcache` `gets` / `cas` + multi-key `get`,
and `redis` typed hash / list / set helpers.

### M23.5 - selectable backends (stance #1)

**Planned.** `session` and `ratelimit` are memcache-only, which contradicts the
"one module, one selectable backend" stance:
- **`session`** - a backend selector (memcache / redis / in-process) behind one
  API; richer-than-`map of string to string` values via `json.Value`.
- **`ratelimit`** - a pluggable backend plus a **sliding-window / token-bucket**
  algorithm option beyond the current fixed-window (which lets a 2x burst
  straddle a window boundary), and reset-time / retry-after reporting so a caller
  can build a compliant `429`.

### M23.6 - format & coverage completeness

**Planned.** The deepest per-module gaps, where a module handles the easy case
but not the real one. The broadest sub-milestone (likely to grow its own
sub-numbering as pieces land):
- **`barcode`** - DataMatrix, UPC-A/E, Code93, GS1-128 symbologies; QR numeric /
  alphanumeric modes + versions 11-40; a human-readable text line under 1D
  barcodes. Reconcile the mismatch where `label` advertises `datamatrix` that the
  image side lacks.
- **`pdfwriter`** - raster image embedding (PNG / JPEG XObjects), embedded /
  subset TrueType fonts (Unicode / CJK body text), and text layout (word-wrap,
  width measurement, alignment).
- **`ical`** - recurrence (`RRULE` / `RDATE` / `EXDATE`), all-day `DATE` +
  `TZID` / `VTIMEZONE`, and VTODO / VALARM / ORGANIZER / ATTENDEE.
- **`orm`** - `OR` / `IN` / aggregates / `GROUP BY`, column projection, and
  relations / joins as first-class (the SELECT-only, AND-only builder blocks
  ordinary queries). Re-validate identifiers where the SQL is *rendered*
  (`toSql` / `createTable` / insert / update), not only in the constructors, so a
  raw `Schema` / `Query` struct literal cannot inject.
- **`bucket`** - presigned URL generation (SigV4 query-signing), multipart +
  `bytes` bodies (currently `string`, capped at 5 GB in memory), and content-type
  / metadata / `HEAD` / copy.
- **`ipnet`** - subnet math (split / aggregate / iterate / host-count / next-IP /
  first-last-usable), classification predicates (isPrivate / isLoopback /
  isMulticast / isLinkLocal / isGlobal), and network-in-network overlap; and fix
  the regression that now rejects every IPv4-mapped CIDR (`::ffff:0:0/96`) - derive
  the max prefix from the literal and translate the prefix on the v4 fold. (The
  regression breaks existing deny-lists at startup, so it wants fixing ahead of
  the rest of the `ipnet` work.)
- **`font`** - CFF / OpenType (`OTTO`) outlines, kerning + `OS/2` metrics
  (cap-height, ascender/descender/line-gap).
- **`feed`** - `<enclosure>` (podcast media, its advertised use case), author +
  categories.
- **`mime`** - honour a text-body `charset` on decode (currently forced to
  UTF-8), and RFC 2231 extended / continued filenames (`filename*=`).
- **`vcard`** - `TYPE` parameters (work / home), a full `N` (prefix / middle /
  suffix), and common fields (PHOTO / BDAY / NICKNAME / ...).
- **`markdown`** - blockquotes, images, and nested lists.

### M23.7 - observability completeness

**Planned.** The metrics story is half-built:
- **`prometheus`** - histogram + summary types (`_bucket` / `_sum` / `_count`,
  quantiles), the most-used metric type, without which latency / SLO
  instrumentation is impossible; optional per-sample timestamp + a Pushgateway
  grouping-key helper.
- **`statsd`** - sample rate (`|@0.1`), DogStatsD tags (`|#k:v`), float values,
  and packet batching (newline-pack several metrics per datagram); and validate
  the metric `prefix` on the same wire line the metric name is already checked on.
- **`influxdb`** - 2.x / 3.x support (token auth, org / bucket params,
  `/api/v2/write`, Flux) alongside the current 1.x.

### M23.8 - ergonomic papercuts + notifier richness

**Planned.** A sweep of cheap, high-value-per-effort wins across many modules:
- **`log`** - a child / context logger (`with(fields)` binding persistent fields)
  and a `fatal` level (log-then-exit).
- **`cron`** - named months / weekdays (`JAN`-`DEC`, `MON`-`SUN`) and the
  `@daily` / `@hourly` / `@reboot` / ... nickname macros.
- **`docblock`** - update the identifier regexes for the M22.2 digit rule (so a
  `func v4` / a `@param x2` / a const `SHA256` parse).
- **`jwt`** - JWKS / `kid`-based key resolution and clock-skew leeway.
- **`totp`** - a secret-generation helper and a configurable skew window; export
  the internal HOTP; and document that replay defence (single-use tracking) and
  brute-force rate limiting are the caller's responsibility (RFC 6238 5.2).
- **`bloom`** - an optimal-sizing constructor (`bloom.optimal(n, fpr)`),
  serialize / deserialize, and union / merge.
- **`csv` / `jsonl`** - streaming reader / writer handles; `csv` dialect options
  (quote char, comment prefix, trim); plus CSV formula-injection (CWE-1236)
  mitigation - a `formatSafe` that prefixes a leading `= + - @` / tab / CR field
  with `'`, and a doc note that plain export is unsafe for spreadsheet targets.
- **`gotify`** - `extras` (markdown content-type, click actions).
- **`htmlwriter`** - boolean / valueless attributes (`disabled`, `checked`).
- **`flatdb`** - clear the two residual lint warnings (a `lint-disable: L103` with
  a reason on the best-effort cleanup catch; drop the unused test local).
- **`webhook`** - replay-protected timestamped signing schemes (Stripe `t=,v1=`,
  Slack `v0=`) + base64 / SHA-1 variants.
- **notifier richness** (the meatier end of the bundle) - `discord` embed fields /
  footer / author + username / avatar override; `telegram` inline keyboards +
  `callback_query` handling + local-file upload (and redact the bot token, which
  the API forces into the URL path, from logs / error messages); `slack`
  `context` / `fields` / `actions` blocks.

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
- **Container image (OCI).** Done - `.github/workflows/docker.yml`
  builds and pushes multi-arch (`linux/amd64` + `linux/arm64`)
  `ghcr.io/<owner>/jennifer` images on each release tag: a Debian-slim
  default (`:latest` / `:<ver>`, full host features) and a distroless
  `:static` variant. The image bundles both binaries and the system
  modules under the compile-default module dir, so a bare
  `import "name.j";` resolves with no env var. See `packaging/docker/`.
- **Homebrew tap (macOS, best-effort unsupported).** Available -
  `packaging/homebrew/jennifer.rb` builds the standard `jennifer` from
  source (so Intel + Apple Silicon work with no Gatekeeper friction) and
  bakes the version and module path via `-ldflags -X`, so a bare
  `import "name.j";` resolves with no env var. Published to the
  `jennifer-language/tap` tap by a manual `publish.sh` (the AUR convention;
  no CI secret). The `-UNSUPPORTED` macOS tarball stays alongside it.

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
