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

- `func name(a as int, b as string) { ... }` with typed parameters,
  by-value argument passing, call-site arity + type checks
- `return;` and `return EXPR;`; recursion works
- `sprintf` and format verbs `%d %f %s %t %v %%` for both `printf` and
  `sprintf`
- The omnibus `stdlib` retired in favor of topic-based libraries; `io`
  is the first.

---

## M4 - Polish & ergonomics

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

## M10 - Namespace-first library architecture

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

## M11 - Control-flow completion

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

## M13-M13.2 - structs and catchable errors

The composite-data milestone, batched in dependency order: M13.1
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

## M15 - foundational libraries + first public release

Nine sub-milestones - two language (M15.2, M15.4), the rest library
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

## M16 - I/O libraries and developer tooling

System libraries that touch the OS or do significant
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

## M17 - module system for Jennifer-coded libraries

Jennifer-coded libraries get their own namespace, scope, and
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


## M18 - Jennifer-coded modules

Built atop the existing system libraries. Each one ships as a Jennifer
**module** under `modules/` (the directory introduced in M17); none of
them are compiled into the interpreter binary. Sub-milestones in priority
order.

Forty sub-milestones (with their nested parts) shipped as
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
| M18.19     | `s3`                    | S3-compatible object storage over `http` (AWS SigV4): `connect` / `get` / `put` / `delete` / `listObjects`. One module for AWS S3 + MinIO / R2 / B2. Renamed `bucket` -> `s3` once the M22.2 digit-identifier rule made `s3` a legal namespace (cf. M22.3's `iic` -> `i2c`); a pre-1.0 break, one batch across the module / overlay / Go test / demo / docs. |
| M18.20     | `dotenv`                | `.env` config: `parse` / `read` / `load` (into env via `os.setEnv`). Over `fs` + `strings` + `os`. |
| M18.21     | `cron`                  | parse cron expressions; `next(schedule, after)` / `matches`. A calculator over `time`.           |
| M18.22     | `log`                   | leveled structured logging (`debug`..`error`; text / logfmt / json) to stdout / stderr / file / RFC 5424 syslog. |
| M18.23     | `ical`                  | iCalendar (RFC 5545) build + parse: a `Calendar` of `VEVENT`s, escaped + line-folded, dates through `time`. |
| M18.24     | `vcard`                 | vCard (RFC 6350) contacts build + parse; shares the content-line codec (`ical_vcard_shared.inc.j`) with `ical`. |
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


## M19 - cross-cutting tooling

The catch-all bucket for interpreter / tooling work belonging to
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
| M19.7 | `@scope/package` resolution (vendored decks) | A leading `@` is a vendored-deck reference expanded by one function (`resolveVendor`): the `@` swaps in the vendor root and a reference not ending in `.j` gets the package-named entry appended, so `@claude/bitcoin`, `@claude/bitcoin/`, and `@claude/bitcoin/utils.j` all reduce to a plain absolute path (after which run-once cache + M19.5 path identity are untouched). The entry is `<package>.j`, so `moduleStem` gives the package name and the display namespace / default alias fall out (`import "@claude/bitcoin/"` binds `bitcoin.`); two same-package decks across scopes are distinct types, colliding only on the default alias (resolve with `as`). Vendor root via `FindVendorRoot`: `--vendor` > `JENNIFER_VENDOR` > nearest `vendor/` above the program. Path safety: `@` front-only, no `.`/`..`, the file must stay inside the deck; a missing root is a guided error. The `jvc` manager over this is M26. |
| M19.8 | relocation: `jennifer-language` org + vanity path | A one-time mechanical relocation, no language / interpreter behavior change: the repo moves to a `jennifer-language` GitHub org, and (separately) the Go module path moves **off** GitHub to a vanity path `jennifer-lang.dev/jennifer` served by a `go-import` meta page, so module identity is host-independent. Two distinct targets: the **Go module path** -> the vanity domain (`go.mod` + 112 `.go` imports rewritten; the meta page maps it to the org repo, plus a `go-source` tag for pkg.go.dev); **human-facing URLs** -> `github.com/jennifer-language/jennifer` (the `-lang`.dev vs `-language` org spelling is deliberate; the redirect keeps old links working, but canonical in-tree URLs are updated). Metadata / CI / packaging swept until `grep -rn 'mplx/jennifer-lang'` is empty; the first-party deck scope placeholder flips `@mplx/` -> `@jennifer/` (doc-only until `jvc`). |
| M19.9 | audit-driven correctness + hardening | A systematic severity-ordered sweep of ~190 findings from a full bug / performance audit of `internal/` + `modules/`, each fix with a regression test (Go test or `_test.j` overlay). **Crash / safety:** OS-entropy RNG seeding (predictable UUIDs / session ids / passwords fixed; `math.randSeed` stays the deterministic opt-in), `json` / `toml` decode nesting caps, a `try`-body scope fix (a throw-skipped `def` reads undefined, not null), `tengine` recursion guards, `archive` zip-slip + aggregate-decompression caps, a JSON-pointer overflow guard, the two interpreter `-race` races + REPL-vs-spawn table mutation. **Correctness:** lvalue writes re-fetch their root after the RHS, index / append stamp the element type, path-keyed module structs, `barcode` (Code 128 stop / EAN check digit / Code 39 `*` / QR mask 3), `pdfwriter` WinAnsi / Info encoding, a full `toml` conformance pass, `http` / `web` (CSRF / cookie / CORS / ETag) hardening, quote-aware `vcard` / `ical`. **Performance:** the O(N^2) accumulation patterns retired (map hash index, `json` object decode, wire-framing reads in `redis` / `amqp` / `mqtt` / `mikrotik` / `imap` / `websocket`, list-join builders in `csv` / `barcode` / `influxdb` / `jsonl` / `statsd`, GF(256) inline). **Lifecycle:** `os.release` + capped child output, non-blocking `net.eof` + mutex-guarded `net.Conn`, `httpd` admission bounds / must-respond timeout / TLS-1.2 floor / safe unix-socket unlink, per-stream mutex + a `discard` verb on hash / crc / compress streams, `lint` descending into `spawn` / `repeat`. Plus **six coordinated pre-1.0 strictness breaks** (each with tests + operator / scoping docs): `%` is floored (Python; `-7 % 3 == 2`, `7 % -3 == -2`); integer arithmetic overflow is a positioned error, not a silent wrap; a duplicate map-literal key is an error; mixed `int` / `float` comparison is exact (no lossy promotion, so `9007199254740993 == 9007199254740992.0` is `false`); a method may not share a top-level var / const name (no-shadowing both directions); reading a constant with the `$` sigil (`$MAX`) is a parse error. |


## M20 - system libraries

Go **system libraries** (cryptographic primitives, plus formats too heavy or
too reflect-bound for a Jennifer-coded `.j` module - the `json` pattern,
[M16.9](#m16---io-libraries-and-developer-tooling-compacted)), plus two cleanup keywords (`defer` / `errdefer`) and
Unix-signal support that the libraries needed. Ten sub-milestones,
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

## M21 - general backlog

The catch-all bucket: milestones that fit no other track - not a Jennifer-coded
module (M18), not interpreter / tooling work (M19), not a Go system library
(M20), and not a beyond-1.0.0 idea (those live in the [horizon
collection](horizon.md)). Anything worth recording with no natural home landed
here, and graduated out once a cluster grew big enough to earn its own bucket.
Thirteen sub-milestones across Jennifer-coded modules, security
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
| M21.13 | Windows installer | An Inno Setup script (`packaging/windows/jennifer.iss`) built by a `windows-latest` CI job into `jennifer-<ver>-setup.exe`: per-user (no admin), adds to `PATH`, bundles the system modules + sets `JENNIFER_SYSMODDIR`, opt-in `.j` association, Apps & Features uninstaller. Unsigned, best-effort **unsupported** build. `scripts/build-windows-installer.sh` recreates it locally via Wine. Promoting Windows to *supported* is the follow-on, `M28.1`. |

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

Post-backlog work belonging to neither the M20 system-library set nor the M21
catch-all: focused standard-library and `.j`-module additions or enhancements,
plus small language cleanups, each landing when the need was concrete. Eighteen
sub-milestones. Each shipped with the standard discipline (a
library adds a Go package + `internal/stdlib.InstallAll` line + `docs/libraries/`
reference + cheatsheet rows; a `.j` module ships a 100%-passing `*_test.j`
overlay + a `cmd/jennifer/*_test.go` integration test + `docs/modules/` doc +
catalog + `JENNIFER.md` bullet + a demo; a language feature updates the spec +
grammar EBNF/PEG + the editor highlighters). Per-item surface detail lives in
those docs; this table is the milestone-number index.

| M#    | Topic | Summary |
| ----- | ----- | ------- |
| M22.1 | `path` library | OS-aware path manipulation over Go `path/filepath`, the pure-string counterpart to `fs`'s I/O: `base` / `dir` / `ext` / `stem` / `join` (variadic) / `clean` / `isAbs` / `split`. Manipulation subset only (no disk-touching `Abs` / `Glob` / `Walk`), so **no build-tag split**, TinyGo-clean, both binaries. Host separator (portable). Explicitly **not** a filename sanitizer. |
| M22.2 | digits in identifiers | Relaxed letters-only to **letter-initial** `[A-Za-z][A-Za-z0-9]*` (still no `_`, <= 64); constants `[A-Z][A-Z0-9]*(_[A-Z][A-Z0-9]*)*` (digits within a chunk). **Additive / non-breaking** - so `sha256` / `SHA256` / `HTTP2` / `SCRAM_SHA256` legal, `AES_256` still illegal (write `AES256`). One `IDENT` token, so it applies uniformly (vars / params / methods / struct type **and field** names / `use as` / `import as` aliases); constants the one separately-lexed class. Graduated `DRAFT#20`; unlocks the M22.3 renames. |
| M22.3 | library renames | The breaking renames the digit rule unlocked, one pre-1.0 batch (no deprecation window): `use iic;` -> `use i2c;`; `uuid.generate("v4"/"v7")` -> `uuid.v4()` / `v7()` (`generate` removed); `crypto.pbkdf` -> `pbkdf2`. **Not renamed:** `binary` (the `bytes` keyword, not a digit), `intl` (JS `Intl`). Each batch updated the library, its overlay + Go test, docs, cheatsheet, and every caller. **Rejected** (stance #1): per-algorithm digest shortcuts (`hash.sha256` / `crc.crc32`) - the hash / crypto family is irreducibly algorithm-as-value (SCRAM / JWT / TLS negotiate the hash), so `compute(b, algo)` stays canonical (`rejected.md`). |
| M22.4 | `match` statement | Multi-way value dispatch `match (EXPR) { when V [, V ...] { } ... else { } }`: subject evaluated **once**, strict `==` (`Value.Equal`), first match wins, values short-circuit left-to-right, **no fall-through**, **not a `break` target** (`break` / `continue` act on the enclosing loop), optional `else` last, no-match-no-`else` is a no-op; a statement, each arm its own scope. Keywords `match` / `when` (pre-1.0 break). `fmt` lays arms out flat (each `when` / `else` on its own line, **not** cuddled). New `MatchStmt`; the header `{` ambiguity (`when Name {` vs a `Name{...}` literal) is resolved by the parser's `noStructLit` flag. Designed to grow into M22.5 patterns. |
| M22.5 | sum types (enums) + pattern `match` | `def enum Name { Variant [ { field as type, ... } ], ... };` (top-level, hoisted like `def struct`); `Name.Variant{...}` / `Name.Variant` construction; `match` gains variant patterns `when Variant(bind) { }` binding the payload into a fresh per-arm scope, with **exhaustiveness checked at resolve time** for a local / same-module enum subject. New `Value` `KindEnum` mirroring `KindStruct` (value semantics, deep-const, cross-module identity by canonical path, retag), tagged-union / reflect-free. **Case-agnostic** naming - `Prefix.Member` resolved from the tables at eval, not capitalisation, so no PascalCase rule is forced on a teaching language (only the pre-existing "ALL-CAPS is a constant" rule still applies). Graduated `DRAFT#19`. |
| M22.6 | TLS options for `http` / `rest` | Reach an HTTPS host with a self-signed / private-CA cert. `http.TlsOptions{skipVerify, caCert}` + send variants `http.requestTls` / `requestWithTls`; `rest.Client.tls` field + a `rest.client(baseUrl)` constructor (the added required field breaks the bare literal) + `rest.withCA(c, pem)` (preferred) / `rest.insecure(c)`. **Secure by default** (verification stays on unless explicitly relaxed). Pure `.j` plumbing to `net.connectTLS` (M16.14) - no interpreter or system-library change. Verified end-to-end against a self-signed loopback (`http_tls_test.go`). Dogfoods M22.9 (`http.TlsOptions` as a struct field across `main -> rest -> http`). |
| M22.7 | `graphql` client module | Thin GraphQL client over `http` / `rest`: `client(endpoint)` + `bearer` / `basic` / `header` / `withCA` / `insecure` builders, `query(c, query, variables) -> json.Value` (POST `{query, variables}`; result under `/data`). Gets the GraphQL convention right - a non-empty top-level `errors` array is an **HTTP 200**, not a non-2xx - raising a `graphql` error with the joined messages; a non-2xx also raises. `queryNamed` / `tryQueryNamed` add an `operationName`; `tryQuery` / `tryQueryNamed` return the raw envelope (no raise on GraphQL errors) for structured-error handling via exported `hasErrors` / `errorMessages` + `json` accessors. POSTs the endpoint **verbatim** via `http.requestTls` (`rest`'s `joinUrl` would append a trailing slash). The GraphQL dependency the M27 Unraid deck consumes. |
| M22.8 | self-referential struct guard | A struct containing itself **by value** (direct or mutual) has no finite zero value and used to fatally stack-overflow when its zero / a literal was built; `Interpreter.checkStructCycles` (a gray/black DFS over direct struct-typed fields, run after hoisting at both `Run` and `EvalInteractive`) now rejects it at hoist time with a positioned error pointing at `list of Self`. Recursion through a `list` / `map` / `task` field and ordinary nesting stay legal. |
| M22.9 | module structs as struct fields | A module struct used as a **struct field type** now type-checks (was rejected "expects geo.Point, got struct" though it worked as a variable type). `resolveDeclaredTypesOnce` now also stamps struct **field** types with the module's `(stem, path)` identity (recursing into `list` / `map` elements), and a module struct's own sibling-struct field types retag to the module identity at the boundary check (construction + field assignment, via `retagType`). Value semantics + chained lvalues into a nested module-struct field work. |
| M22.10 | byte-capable `http` download | `http` could not fetch a binary body (the response was always `convert.stringFromBytes(_, "utf-8")`, which throws on non-UTF-8). Added a byte path reusing the already byte-exact framing: `http.BytesResponse` (`body as bytes`) + `requestBytes` / `requestWithBytes` / `getBytes` (`parseResponse` split into a `parseRaw` byte core + a text decoder). Text verbs unchanged (still throw on non-UTF-8, by design); `rest` stays text / JSON. Pinned by `http_bytes_test.go` (a gzip round-trips with matching sha256). |
| M22.11 | hardening: injection & output-encoding | From two security / robustness audits (`internal/` + `modules/`; per-finding detail - severity, reproducer, sites - in the two report files). orm identifier / operator allowlists; imap / pop CRLF rejection; statsd metric validation; htmlwriter tag / attr checks + exported `safeUrl`; `json.encode` HTML-escaping (`< > & U+2028 U+2029`); a new constant-time `hash.equal`; `http.parseUrl` authority split; dotenv env-name validation (`OM-021`); `ipnet` v4-mapped `::ffff:0:0/96` fold (`unmap`). **Reasoned non-literal:** a tengine no-auto-escape SECURITY **warning**, not an auto-escape mode (`OM-012`, keeps `text/template` semantics); `OM-010` folds only the well-defined `::ffff:0:0/96` mapped form. |
| M22.12 | hardening: network / resource / path robustness | 64 MiB caps on server-declared lengths; connect / read timeouts + cleartext warnings; a `net.readAll` default cap; `net.startTLS` locking; bounded handle registries + a `sql` query deadline + DSN-password redaction; `archive` per-entry budget; `httpd.serveDir` traversal rejection + `unix:`-socket perms; the json / toml / yaml write-API depth guard. **Reasoned non-literal:** `OF-007` applied full bounds + deadline + teardown to `sql` (the worst instance) and registry bounds to `fs` / `os` / `compress` / `net`. |
| M22.13 | hardening: web framework | `web.sessionId` trusts only a minted-UUID-shaped cookie + `web.renewSession`; the `web.onError` hook (else stderr); lenient form / percent decoding + csrf content-type gate; a `HEAD` served by the matching `GET` route; httpd per-request timer cleanup. **Reasoned non-literal:** `OM-003` shipped **serial** for v1 with loud docs because concurrent dispatch raced shared interpreter state - **later removed properly in M22.17**; `OF-006` (unrestricted `meta.call`) shipped a docs allowlist pattern + `meta.md` example, not a new primitive (a two-line `.j` allowlist suffices; `web` only dispatches author-registered handler names). |
| M22.14 | `imap` criteria-based search | `imap.search(session)` -> `imap.search(session, criteria)` (breaking; an empty `imap.criteria()` = the old `SEARCH ALL`). `imap.Criteria` filters **hybrid**: server-side fields map to one IMAP `SEARCH` (substring on subject/from/to/text, a `since` / `before` day-range as `time.Time`, flags, size - all ANDed, one round-trip), client-side fields refine the candidates by fetching only headers / `BODYSTRUCTURE` (`subjectRegex` / `fromRegex`, `hasAttachments` heuristic). Injection-safe (`quoteArg` + control-checked line); a time-of-day bound is transparently refined client-side against each candidate's `INTERNALDATE`. Pure `.j`. |
| M22.15 | `imap` browse / APPEND / rename | Rounded `imap` into a full read / browse / manage / **save** client: `folders` (`LIST`) + `status` (`STATUS`, no select), `append` / `appendWith` (`APPEND` via the synchronizing-literal continuation flow). Terminology rename (pre-1.0 breaking): mailbox -> **folder** (`selectFolder` / `createFolder` / the `Folder` struct); the LIST verb is `folders` (`list` is a reserved type keyword). |
| M22.16 | core hardening sweep | Small `internal/` correctness / resource / performance residuals, one reviewed pass: `sql` cursor deadline split from the acquire / statement deadline (caller-settable `sql.setQueryTimeout`); `httpd` registry `maxServers` bound + catchable "too many open"; md5 / sha1 labelled **non-cryptographic**; the json / toml / yaml write-depth guard checks only the touched node (not a full re-scan) and folds three `exceedsDepth` copies into a shared helper; an `include` total-token cap (a diamond that re-includes the previous file expands 2^n); the `printf` field cap lowered from `1<<20` + closed-form padding; incremental json / xml decoder error positions. |
| M22.17 | web hardening (concurrent dispatch) | Turned `web` from strictly-serial into **safely-concurrent**, interpreter-first (four ordered steps, each with a gating test; reasoning in `design-decisions.md`). (1) `Error` crosses `meta.callMain` intact - track a module's *declared* structs separately from the auto-injected `Error`, retag only the former. (2) Race-safe dispatch - the call-depth counter became a per-chain `*int` threaded down the frames (fresh at each goroutine root, incremented at `evalCall` **and** every cross-boundary dispatch), fixing both the `-race` data race and a fatal re-entry Go-stack overflow; the rest of the reachable shared state (map hash-index reads, resolver caches, profiler, diag) audited clean. (3) spawn-per-request in `web` (errors caught inside; `task.discard` prunes; concurrency bounded by `httpd`). (4) `web.onError` fail-safe - always stderr **and** the hook, which now binds `as Error`. The 1700 ms -> 3 ms latency probe is the regression gate. Supersedes the M22.13 `OM-003` "web stays serial" note. |
| M22.18 | `dotenv` layering, profiles, interpolation | Grew `dotenv` into a layered loader: `readCascade` / `resolve` / `loadCascade` / `autoload` merge `.env` -> `.env.local` -> `.env.<profile>` -> `.env.<profile>.local` from one fixed `dir` (no walk-up, closing the file-hijack class), with a **real OS env var always winning** over a file value; profile from `JENNIFER_ENV` (empty = base files only). Enhanced `parse`: **backward-reference** `${VAR}` (unquoted + double-quoted, resolving earlier keys -> real OS env -> "", so cycles are impossible; no `$(...)` command substitution), multi-line double-quoted values (positioned unterminated-quote error). Strict profile validation `^[A-Za-z0-9_-]{1,64}$` (no traversal). Single-file `load` keeps unconditional-override (the primitive); the cascade loaders are real-env-wins. `os.getEnv(k) != ""` is the "already set" test (no `os.hasEnv`; `""` counts as unset). Consolidated the `${VAR}` / multi-line items parked in M23.8. |

## M23 - module improvements

**Done.** M22 lifted a handful of modules (`imap`, `http`); M23 generalized that
across the module ecosystem. A survey of all 63 `.j` modules found the gaps
**cluster into cross-cutting themes** rather than scattering per module, so the
work was organized by theme - a shared pattern (a receive loop, a persistent
connection, a backend selector) built once and applied to every module that needs
it. Fifteen sub-milestones. Each shipped the standard per-module discipline (a
100%-passing `*_test.j` overlay, a `cmd/jennifer/*_test.go` integration test where
a live server applies, `docs/modules/` + catalog + `JENNIFER.md` entries, a demo,
both binaries build); per-item surface detail lives in those docs, and the biggest
sub-milestones keep their own reference docs. This table is the milestone-number
index. **Deliberate non-goals:** `password` hashing (needs `x/crypto`) and
fully-typed `orm` rows (awaits struct reflection).

| M#     | Topic | Summary |
| ------ | ----- | ------- |
| M23.1  | streaming / server-push read loops | One cooperative receive-loop-over-`net` shape - a blocking `receive*` (+ timeout-bounded `poll`), no callbacks, the app opting into concurrency via `spawn` - across `redis` (pub/sub + one-round-trip `pipeline` + `multi`/`exec` + `scan`), `amqp` (`Basic.Consume` + exchanges + publisher confirms), `mqtt` (QoS-1 + retained + Last-Will + `reconnect`), `mikrotik` (`.tag`-correlated + `/listen`), `imap` (RFC 2177 `IDLE`). Wire framing factored into pure encode/parse (100% overlay); live loops on mock-server Go tests. Fixed `redis` coalesced-frame buffering and `imap` stale-deadline. Residual: a value-semantic `Session` can't retain a cross-call buffer (a buffered `net` reader is the general fix). |
| M23.2  | connection reuse / persistent sessions | Reusable connections so a loop stops re-handshaking: `http.Session` (reused `net.Conn` + cookie jar, framed one-response reader) + a policy `http.send` over `http.Options` (3xx redirects, cookie jar, 429/5xx retry+backoff); `rest.Client` routes through it (`tls` folded into `Options`, pre-1.0 break; `paginate` / `paginateCursor` walk every page); `smtp` split into `open` / `sendOn` / `close` so N messages pay one TLS+auth handshake. Pinned by keep-alive / cookie / pagination / session-reuse Go tests. |
| M23.3  | stable-identity verbs | Volatile sequence numbers broke "fetch only what's new": `imap` went **UID-only** (every verb sends its `UID` form + `search` returns UIDs + atomic `move` + ranged `fetchPartial`; pre-1.0 break, a seq+UID twin set rejected on stance #1), `pop` added `uidl` -> `MessageId` + `top` / `reset` / `noop` (additive). Pinned by exact-wire-command fake-server tests. |
| M23.4  | byte-exact binary values | `redis` / `memcache` threw on a non-UTF-8 bulk value; added `bytes`-valued `setBytes` / `getBytes` (text verbs unchanged, still strict-throw) plus typed `redis` hash/list/set helpers and `memcache` `getMulti` / `gets` / `cas`. NUL/CR/LF/0xFF byte-count-framed round-trip tests. |
| M23.5  | selectable backends | `session` / `ratelimit` were memcache-only; added the **`kv`** system library (in-process per-key-TTL store, integer handle shared across `spawn`, `openFile` persisted) and the **`kvstore`** module selector (`Store` a sum-type enum `Memcache`/`Redis`/`Local`, exhaustiveness-matched), and moved `session` (values now a `json.Value`) and `ratelimit` (`fixedWindow` / `slidingWindow` -> `Result`) onto it (both pre-1.0 break). Interpreter fix: a module enum as a struct-field type across a boundary. |
| M23.6  | format & coverage completeness | The broadest track (13 pieces) - the deepest per-module gaps. `ipnet` subnet math + `scope` classifier; `orm` ordinary-query surface (`select` / aggregate / `join` / `groupBy`+`having`, render-time allowlist re-check); `markdown` images / blockquotes / nested lists; `vcard` full `N` / `TYPE`; `ical` recurrence + `TZID` + `VTODO`/`VALARM`; `mime` charset-on-decode + RFC 2231; `feed` enclosures + author/categories; `s3` presign + byte bodies + multipart; `barcode` UPC / code93 / DataMatrix + QR 11-40; `font` a CFF/OTTO backend + an O(1)-per-query fix; `pdfwriter` embedded TrueType fonts + image XObjects + text layout. Several pre-1.0 struct-shape breaks; each validated against an independent reference. |
| M23.7  | observability completeness | `prometheus` histogram + summary types (cumulative buckets, nearest-rank quantiles, exact text format, `observeAt` / `pushgatewayPath`); `statsd` `*Rate` / `*Tagged` / `*Float` verbs + a `Batch` datagram packer + a control-char-validated `prefix`; `influxdb` 2.x / 3.x via a `Version` enum (Flux `queryFlux`, token redaction). Each validated against an independent reference parser. |
| M23.8  | ergonomic papercuts + notifier richness | Cheap high-value wins across 15 modules: `log` child logger + `fatal`; `cron` named months/weekdays + `@`-macros; `jwt` `verifyLeeway` / `verifyWithKeys` / `verifyJwks`; `totp` `generateSecret` / `hotp` / `verifyWindow`; `bloom` `optimal` / serialize; `csv` `formatSafe` + `Dialect` + streaming; richer `discord` / `telegram` / `slack` / `gotify` messages; `webhook` replay-protected signing; and more. Core fall-out: `http.requestRawBody` / `requestRawBodyTls` (a byte request body, for telegram uploads). |
| M23.9  | `fmt`: shape-aware wrapping + raw-literal fidelity | Rebuilt `jennifer fmt` (token-stream, no AST): width-aware wrapping (one element/arg per line past 100 cols, struct/map over 6 members, calls hug `)`, inline single-statement `when` arms, operator-chain fill-break), tight Go-style literal spacing, and **raw-literal fidelity** via a lexer `Token.Raw` (digit separators / base prefix / quote style / embedded newlines survive verbatim). `-w` is atomic + self-verifying (re-lexes its output, refuses to corrupt). Corpus reflowed (~700 -> ~215 over-limit lines). Pre-1.0 break (canonical output changed); `TestFmtPreservesTokenStream` proves a format changes only whitespace. |
| M23.10 | interactive stdin for `os.run` / `os.spawn` | `os.run(argv, stdin)` feeds an optional trailing `string` / `bytes` to the child's stdin then closes it (variadic - one-arg calls unchanged), output drained into the 16 MiB-capped buffers - **deadlock-free by construction**. Unblocks a stateless filter / subprocess exchange (M23.13's `mcp.connectStdio` built on it). Deferred: interactive streaming pipes on a spawned `Process`. |
| M23.11 | `jsonrpc` module | JSON-RPC 2.0 client + server, pure `.j` over `json` + `http`. Client `call` / `notify` (`json.Value` params/results, every failure a unified catchable `Error`); a transport-agnostic `handle(requestBody)` runs the whole protocol (single / notification / batch / reserved codes) dispatching each `method` to a top-level `func` by name via `meta.callMain`; a thrown handler yields a generic `-32603` (detail stays server-side). Chosen over gRPC (protobuf + HTTP/2 + codegen would be a heavy Go library, not a `.j` module). |
| M23.12 | `match` / `enum` adoption | Applied `match` / `enum` to the genuine closed-variant-set cases (open sets stayed strings): non-breaking `match` in 14 modules; a non-breaking private enum (`markdown`); breaking API enums (`htmlwriter.NodeKind`, `prometheus.MetricType`, `barcode.SymbolKind`, `orm.Dialect` / `ColumnKind`, `totp.Algorithm`); and a shared `transport.Security{None,Tls,Starttls}` replacing the stringly-typed `security` field on six socket clients (the mail three accept all modes, the other three reject `Starttls`). Exhaustiveness-checked. |
| M23.13 | `mcp` module (Model Context Protocol, stateless) | A server exposing tools / resources / prompts (`server` / `addTool` / `addResource` / `addPrompt`, `handle` / `serveStdio`) plus a client (HTTP `connect` / stdio `connectStdio` over `os.run`) sharing one call surface; MCP is JSON-RPC 2.0 so the client reuses `jsonrpc.call`. `tools/call` is **allow-listed** (only a registered handler; a thrown handler yields a generic message). Validated end-to-end against the official MCP Python SDK. Not planned: the stateful Streamable-HTTP transport (needs an SSE push primitive `httpd` lacks). |
| M23.14 | raw single-quoted string literals | Breaking split so each delimiter does one job: `"..."` stays cooked (escapes processed), `'...'` becomes **raw** (verbatim to the next `'`, spanning newlines - a free heredoc); embed a `'` via the cooked form (`"it's"`). No `r"..."` prefix (rejected - the delimiter is the mode). Lexer-confined (`raw := quote == '\''`); `src` is `[]rune` so multibyte content is exact; `Token.Raw` round-trips (incl. multi-line). Migration a no-op; docs / grammar / four editor highlighters updated. |
| M23.15 | `orm`: a batteries-included data-mapper ORM | Lifted `orm` from a thin query builder to a full ORM (still Data Mapper): a `Session` unit-of-work + column-attribute DDL builders (.1); migrations split into the sibling **`sqlmigrate`** module (.1b); associations + `joinRelation` (.2); eager loading in a fixed 1+R queries (.3); a write path - `upsert` / `insertMany` / `insertReturning` / `updateWhere` / `deleteWhere` / `save` (.4); finders + `whereNull` / `whereBetween` / `distinct` / `page` (.5). Rows stay `map of string to string` (no reflection), relations attach via a side `Result` (no `any`); `whereRaw` / typed-struct mapping / Active Record rejected. Keeps its own `docs/modules/orm.md` + `sqlmigrate.md`. |

Cross-cutting threads:

- **Not all of M23 is modules.** M23.9 (`fmt`), M23.10 (`os.run` stdin), and
  M23.14 (raw single-quoted strings) are tooling / interpreter / language work; the
  rest is the module ecosystem.
- **Build once, apply everywhere** was the payoff of organizing by theme: a shared
  receive-loop shape (M23.1) and a shared backend selector (the `kv` library +
  `kvstore` enum, M23.5), each built once and threaded through every module that
  needs it.
- **`match` / `enum`** (M22.4 / M22.5) landed across the ecosystem in M23.12, and
  the largest sub-milestone - M23.15's `orm` + the new `sqlmigrate` - is a full
  batteries-included ORM in its own right.

---

## M24 - language, concurrency, and libraries

**Done.** The first batch of [horizon](horizon.md) drafts graduated into a
scheduled track, then grew into the broadest single milestone: a CLI ergonomic,
the language's biggest expressiveness feature (first-class functions), a
concurrency-coordination layer, a numeric / ML library stack, two ASN.1 protocol
clients, and a run of library / language refinements. Twenty-three sub-milestones.
Each shipped with the standard discipline - spec + grammar EBNF/PEG + editor
highlighters where a language feature lands; a Go package +
`internal/stdlib.InstallAll` line + `docs/libraries/` reference + cheatsheet where
a library lands; a 100%-passing `*_test.j` overlay + `cmd/jennifer/*_test.go` +
`docs/modules/` + catalog + `JENNIFER.md` + demo where a `.j` module lands; both
binaries build; the full test close-out. Per-item surface detail lives in those
docs - this table is the milestone-number index.

| M#     | Topic | Summary |
| ------ | ----- | ------- |
| M24.1  | run profiles / `--env` flag | `jennifer run --env=prod script.j` sets `JENNIFER_ENV=prod` before `Run` - identical to the env var, no interpreter change (`dotenv`'s `.env.<profile>` selection is the first consumer). `cmd/jennifer`-only: `parseRunArgs` + a `validRunProfile` allowlist (`[A-Za-z0-9_-]`, 1-64, blocks a `.env.<profile>` traversal); an explicit flag overrides an inherited env. Both binaries. Graduated `DRAFT#26`. |
| M24.2  | first-class functions | Closed the largest expressiveness gap - a function held in a value (type `func`), **immutable, not a pointer** (so the value-semantics stance is untouched); a **bare method name** in expression position *is* the value, a name + `(` is a call (no `&NAME` sigil). New `KindFunc` / `TypeFunc`; a `CallValueExpr` postfix-`(` node calls any function-valued expression (`$f(x)`, `$fns[0](x)`, `makeAdder(1)(2)`). Enables the higher-order `lists` layer (`map` / `filter` / `reduce` / `find` / `any` / `all` / `sortBy`, via `BuiltinCtx.Invoke`). **Deferred:** Tier 2 closures + migrating string-name dispatch. Graduated `DRAFT#18`. |
| M24.3  | concurrency coordination | Gave `spawn` / `task` cancellation, bounded waits, and channels. `task.cancel` / `cancelled` - cooperative, observed at a shared `loopCheckpoint` (a catchable "task cancelled" at loop safe points; one atomic-nil check per iteration, hot path untouched); `waitTimeout` / `waitAnyTimeout` throw on timeout. `channel of T` + the `channel` library (`make` / `send` / `recv` / `close` / `select` / `len` / `capacity`): a `KindChannel` shares a `*ChannelState` while `send` deep-copies the value in (conduit shared, data copied), and `channel` is a **contextual keyword**. Buffer capacity capped (OOM guard); `send`-on-closed / double-close catchable. Graduated `DRAFT#21`. **Deferred:** cancellable channel ops, index-returning `select`. |
| M24.4  | `stats` library | 26 descriptive statistics over `list of int` / `list of float` - central tendency (geo / harmonic / weighted means, `modes`), spread (population + `sample*` Bessel, `iqr`, `mad`), shape (`skewness` / excess `kurtosis`), order (`percentile` / `quartiles`), `sum` / `zscore`, bivariate (`correlation` / `covariance`), `describe` -> `stats.Summary`. Strict like `math` (an undefined result is a catchable error, never a NaN); real reductions -> `float`, selections preserve the input kind. Pure Go, TinyGo-clean. Graduated `DRAFT#5`. |
| M24.5  | `ml` library | A classical / predictive ML core (scikit-learn-lite) over `stats` / `linalg`, **not** deep learning. Fit/predict: a fit fn -> an opaque `ml.Model` handle applied with `predict` / `transform` / `predictProba` / `free`. Regression (linear / ridge / lasso / kNN / tree / forest), classifiers (kNN / naiveBayes / logistic / CART tree / forest), `kMeans`, `pca`, scalers; metrics (`accuracy` / `precision` / `recall` / `f1` / `rocAuc` / `rmse` / `r2` / ...); `trainTestSplit` / `kFold` / `polynomialFeatures`. Random draws from `math`'s seedable source; hyper-parameters bounded (a runaway value -> catchable). Pure Go, both binaries. |
| M24.6  | `linalg` library | Linear algebra over Jennifer's value types, the `stats` companion. Vectors (`list of float`): `dot` / `distance` / `cross` / `normalize`; matrices (`list of list of float`): `transpose` / `trace` / `determinant` / `inverse` / `solve` / `identity` / `zeros` / `shape`; polymorphic `norm` / `scale` / `add` / `sub`; `matmul` dispatches on operand shape. Direct algorithms (Gaussian / Gauss-Jordan), no `gonum`. Strict (dimension mismatch / singular / non-finite -> catchable); every value bounded by `MaxMatrixElements`. Pure Go, both binaries. |
| M24.7  | `asn1` library | ASN.1 BER decode / DER encode, the byte enabler for LDAP / SNMP. Hand-rolled (Go's `encoding/asn1` is DER-only + reflect-bound). An opaque `asn1.Value` element tree (like `json` / `xml`) walked by `(node, pointer)` accessors whose tokens are child indices (`tagClass` / `tagNumber` / `asInt` / `asOid` / ...); typed constructors (`integer` / `oid` / `sequence` / `tagged` EXPLICIT / `retag` IMPLICIT). A decode-node budget + nesting cap turn a bomb into a catchable error; output byte-verified against Go's `encoding/asn1`. Pure Go, both binaries. |
| M24.8  | `snmp` client + agent | SNMP v1 / v2c **client and agent** (`modules/snmp.j`) over `asn1` + `net` UDP. Client `get` / `getNext` / `set` / `walk` -> `list of Varbind` typed by SNMP type (counter32 / gauge32 / timeTicks via `asn1.retag`); request-id check, deadline + retries. Agent `agent` / `serve` answers GET / GETNEXT / SET for a MIB (GETNEXT in numeric OID order so a `walk` traverses it). Codec factored pure (socketless overlay); live loopback + real-hardware verified. No v3 / traps / GETBULK. Default binary only. |
| M24.9  | `ldap` client + directory server | LDAP v3 (RFC 4511) client and lightweight directory server over `asn1` BER + `net`, LDAPS / StartTLS via `transport.Security`. Client `bind` / SASL `bindSasl`, `search` (RFC 4515 `parseFilter` or constructors) / `searchPaged`, writes `add` / `modify` / `delete` / `modifyDn` / `passwordModify`. Server answers simple bind (`password` schemes) + filtered search - read-only over the wire, mutable from code via a shared `kv` store (in-memory or file-backed), enough to back Authelia. Codec factored pure. Default binary only. |
| M24.10 | `math` foundations + special fns | Filled the `math` gaps for 1.0, folded in. **Everyday:** trig (+ inverses, `atan2`), hyperbolic, exp / log (`exp` / `ln` / `log2` / arbitrary-base `log`), `cbrt` / `hypot` / `sign`, combinatorics (`factorial` / `comb` / `perm` / `gcd` / `lcm`, exact-int overflow-checked), `TAU`. **Special:** `erf` / `gamma` / `lgamma` / `beta` + the regularized incomplete gamma (`regGammaP` / `regGammaQ`) / beta (`regBetaI`) the CDF engine needs, hand-rolled to machine precision. Strict (domain / overflow / non-finite -> catchable). Go stdlib base; both binaries. |
| M24.11 | distributions + inferential `stats` | The classical numerical-inference layer (the scipy.stats / Excel surface) folded into `stats` on M24.10's specials - **no separate `prob` library**. Distributions (normal / t / chi-square / F / binomial / Poisson): pdf/pmf, cdf, quantile (Acklam + bisection), Box-Muller `normalSample`. Inference: `linearRegression` / `multipleRegression`, `confidenceInterval`, `proportionCi` (Wald / Wilson / Clopper-Pearson), `tTest` / `chiSquareTest` / `fTest` / `anova`, `histogram` -> `Regression` / `Interval` / `Test` structs. Every p-value finite-guarded, clamped `[0, 1]`; a degenerate input -> catchable. Pinned to scipy; both binaries. |
| M24.12 | scientific-notation float literals | Float literals accept an `[eE][+-]?digits` exponent (`6.022e23`, `1e10` - the exponent makes a literal a float even with no fraction), no `_` in the exponent, decimal-only (`0xe5` keeps `e` a hex digit). Lexer-only, additive / non-breaking. Strict at the edge: overflow (`1e400`) a positioned parse error (never `Inf`); underflow (`1e-400`) rounds to a finite `0.0`. Reasoned in `design-decisions.md`. |
| M24.13 | `uri` module + `encoding` codecs | URL handling factored - byte encoding in `encoding`, URL semantics in a `.j` module. `encoding` gains `"uri-percent"` (RFC 3986) + `"uri-form"` (form-urlencoded, space `+`) `toText` / `fromText` codecs. The `uri` module (pure `.j`, both binaries): `parse` -> `Uri` / `build` (round-trips), `encode` / `decode` / `encodeForm` / `decodeForm`, `buildQuery` / `parseQuery`, RFC 3986 `resolve(base, ref)`. Seven modules de-duplicated onto them. **Pre-1.0 break:** `rest` query strings now form-encode a space as `+`. |
| M24.14 | `args` CLI parser module | A declarative `argparse`-style parser (the "write your tools in Jennifer" gap `os.flag` / `os.ARGS` left). A value-semantic `Parser`: typed optional flags (`flag` / `intFlag` / `boolFlag` / `countFlag` `-vvv` / `listFlag`), `nargs` positionals, subcommands, `version`; `args.parse($p, os.ARGS)` -> a `Result` with typed accessors. Normalises `--flag=value`, bundled shorts, `--` end-of-flags; an error is a catchable `Error{kind:"args"}` (not `exit(2)`), `-h` / `--version` set `done`. Pure `.j`, both binaries. 32-test overlay. |
| M24.15 | `validate` module | Declarative validation of a `map of string to string` -> a structured failure list. Value-semantic rules (`required` / `isInt` / `min` / `maxLen` / `pattern` / `email` / `url` / `datetime` / `oneOf` / `password` / `custom` `func` predicate + `withMessage`), grouped per field; `validate.check` -> `list of Failure`, plus `ok` / `messages` / `byField` / `localize` (i18n `%param%` markers). An absent / blank field passes every rule but `required`. Pure `.j` over `regex` / `uri` / `time` / `password`; both binaries. 16-test overlay. |
| M24.16 | module suite hardening | A security / robustness pass whose audit findings collapsed to systemic root causes, each fixed once across its cluster (~35 fixes / ~24 modules, both binaries clean), remotely-exploitable first. Received-data caps (shared `transport.checkReceiveSize`); injection (`label` digits-only command class, CRLF / control stripping in `log` / `ical` / `vcard`, `barcode` colour); a cross-origin redirect credential leak (`http.send` drops `Authorization` / `Cookie` behind `allowCrossOriginRedirect`); typed error propagation; numeric clamps; correctness (`semver.satisfies(v, garbage)` -> `false`). |
| M24.17 | `html` module (rebrand + parser) | Fold HTML building and parsing into one bare-named `html`. **Rebrand** `htmlwriter` -> `html` (pre-1.0 break; a format module is named for the format, like `xml` / `json`). A tolerant hand-rolled `parse` (pure `.j`, no `x/net/html`) producing the **same transparent `Node`** the writers do (build / parse round-trip): void / self-closing / unquoted-boolean attrs, mismatched-nesting auto-close, comments, DOCTYPE, raw `script`, entities; depth + node budget. Added `parse` / `attrOf` / `hasAttr` + XPath-ish `get` / `findAll` / `has`. Both binaries; 23-test overlay. |
| M24.18 | `markdown` reader | Gave `markdown` reading (the consistency move M24.17 set up). `markdown.parse(md)` -> a public `Node` (a string `kind`, recursive over `children`) walked by the `xml` / `html` vocabulary (`typeOf` / `children` / `text` / `level` / `attr` / `get` / `findAll` / `has`); the private `Block` / `Span` model is converted, inline spans eagerly expanded (a link's `href` walkable), the fence language captured. **One model:** `toHtml` / `toAnsi` became `render(parse(md), ...)` wrappers (old renderers deleted; byte-identical output). Nesting cap + node budget. Pure `.j`, both binaries; +16 overlay tests. |
| M24.19 | string interpolation | Cooked-string interpolation `"total: {$sum}, up {strings.upper($x)}"` - sugar over concat + `convert.toString`. Only a cooked `"..."` interpolates, each `{expr}` one expression (a statement / empty `{}` is a parse error); a raw `'...'` never (the off form); a literal brace is `\{` / `\}`; `f"..."` rejected. **Part 1** moved the two `{name}`-template consumers (`intl.tr`, `validate`) to a `%name%` marker. **Part 2** added the lexer `TOKEN_STRING_INTERP` (`Parts`) + the parser sub-lex / sub-parse of each slot -> `InterpStringExpr`. The whole toolchain treats a slot as real code (resolver / `lint` `L204` / `profile`); every literal-brace cooked string was migrated (prefer raw). Graduated the horizon entry; both binaries. |
| M24.20 | module version + capability header | A per-file `# pragma-jennifer-<key>: <value>` header stating the minimum interpreter version + required host capabilities, checked at **read time** (the lightweight sibling of the M26 `jvc` / `deck.toml` constraints, no package manager). `version` (`>=0.25.0`, a min-compare via `version.AtLeast`; any dev build bypasses) + `capability` (`net` / `exec` / `sql`, from a build-tag-split set, queryable as `meta.CAPABILITIES` / `hasCapability`). Enforced per file at three first-read seams (CLI / `include` / `loadModule`), with bounded diagnostics; a malformed or duplicate-`version` directive is a hard error. Ships an `L303` lint + `fmt` canonicalisation. Every module carries a `>=0.24.0` floor; `capability:` only where mandatory (an optional backend stays ungated). No interpreter-core change. |
| M24.21 | Markdown -> PDF (`markdown.toPdf`) | The markup-driven document story for `pdf`, **folded into `markdown`** as a third target beside `toHtml` / `toAnsi`: `toPdf(md)` / `toPdfWith(md, opts)` / `renderPdf(doc, opts)` lay a document out to a paginated PDF over `pdf`'s layout primitives. A flow engine threads a value-semantic `Layout` through per-block renderers (heading, word-wrapped paragraph with per-run fonts, nested lists, ruled GFM table, code, blockquote), paginating at the bottom margin; `PdfOptions` sets size / margins / standard-14 fonts. Folded in (reasoned in `design-decisions.md`) at ~2x markdown import time / +~6 MB RSS; needed M24.18's parse tree. Graduated `DRAFT#13`. Pure `.j`, both binaries; +15 overlay tests. Dogfooded by `gen-module-docs.j` -> `jennifer-module-api.pdf`. |
| M24.22 | `dot` module | Graphviz DOT graph description: build a graph of nodes and edges with attributes (`digraph` / `graph`, `node` / `nodeWith`, `edge` / `edgeWith`, `graphAttr` / `nodeAttr` / `edgeAttr`) and `render` it to `.dot` text for an external Graphviz tool to lay out (`dot -Tsvg`). Value-semantic builders; DOT-escaped strings; emits the description only (graph layout is Graphviz's job, deliberately not reimplemented). Pure `.j` over `strings` / `lists`, both binaries. |
| M24.23 | `plot` module | Data plotting to SVG: a unified `chart(series, opts)` renders one or more `Series` (line / points / both / area, dashed, error bars, marker shapes) on shared axes with a positioned legend; `line` / `scatter` / `bar` / `bars` (grouped / stacked / diverging) / `histogram` wrap it. Automatic "nice" ticks, log scales, a `time`-labelled date axis, reference lines (`hline` / `vline`), fonts / margins, data labels, `<title>` hover tooltips, and `save`. The visual companion to `stats` / `ml`; pure `.j` over `math` / `time` / `fs` / `strings` / `lists` / `convert`, both binaries. |

Cross-cutting threads:

- **Graduated horizon drafts.** M24 drained the first scheduled batch from the
  [horizon collection](horizon.md): `DRAFT#26` (M24.1 `--env`), `DRAFT#18` (M24.2
  first-class functions), `DRAFT#21` (M24.3 concurrency coordination), `DRAFT#5`
  (M24.4 `stats`), and `DRAFT#13` (M24.21 Markdown -> PDF), plus the
  string-interpolation entry (M24.19).
- **A numeric / statistical stack** landed together: `stats` (M24.4) + `linalg`
  (M24.6) + `ml` (M24.5) + the `math` special functions (M24.10) + the
  distributions / inference layer folded back into `stats` (M24.11) - each strict
  (an undefined result is a catchable error, never a NaN) and bounded by a
  materialised-value cap.
- **An ASN.1 protocol stack:** the hand-rolled `asn1` byte layer (M24.7) enabled
  the `snmp` (M24.8) and `ldap` (M24.9) clients-and-servers, each factoring its
  codec pure so the overlay round-trips without a socket.
- **Consistency across the format modules.** The `html` rebrand + parser (M24.17)
  and the `markdown` reader (M24.18) gave both modules the same build-and-parse
  `Node` model as `xml` / `json`, which M24.21's `toPdf` then consumed.
- **Not all of M24 is libraries.** M24.2 (first-class functions), M24.3
  (cancellation / channels), M24.12 (scientific-notation literals), and M24.19
  (string interpolation) are language / interpreter work; M24.1 is a CLI flag;
  M24.16 / M24.20 are a cross-suite hardening pass and a read-time guard.

---

## M25 - read-only-parameter borrow (compound copy elision)

Parameter binding deep-copies compound arguments (`list`, `map`, struct) to
uphold value semantics. When the callee **never writes the parameter** - no
`$p = ...`, `$p[i] = ...`, `$p[] = ...`, `$p.f = ...` anywhere in its body - that
copy is pure waste: a read-only alias to the caller's backing is observationally
identical to a copy, so the parameter can be bound by alias. This is the standard
value-semantic-language optimization (the "borrow" half of copy-on-write),
narrower and more clearly sound than the shared-marker COW that was tried and
reverted as inert (see [technical/rejected.md](technical/rejected.md)): it never
detaches-on-write, because a borrowed parameter is by construction never written.
No `.j` changes and no new syntax - a pure interpreter optimization. Split into
the borrow itself (M25.1) and the escape analysis that widens where it is sound
(M25.2). Independent of the horizon bytecode-VM direction but composes with it.

### M25.1 - borrow in modules and globals-free scripts

**Done.** The borrow, enabled in the two contexts where it is sound without a
whole-program analysis.

- **The soundness subtlety.** "Never writes the parameter" alone is *not*
  sufficient: if the argument aliases a mutable global and the body mutates that
  global, a borrowed read observes the mutation where a copy would not
  (`def g ...; func f(p) { $g[0] = 9; return $p[0]; } f($g)`). The synchronous
  call model rules out the *caller* changing the backing mid-call, but not the
  *callee* changing it through a global alias. Borrow is therefore enabled only
  where no mutable global can exist to alias: (a) a **module**, whose top level is
  declarations-only (and whose cross-boundary arguments are copies anyway); and
  (b) a **single-file script that declares no mutable top-level `def`** - checked
  once by `hasMutableTopLevelGlobal`, so a script's own read-only helpers benefit
  without being moved into a module. A script *with* a mutable global keeps the
  copying bind (M25.2 is what lifts that).
- **Mechanism.** Extends the existing **scalar** copy-elision in `bindParamValue`
  to compound Kinds - the "mutation-safety proof" that elision path was documented
  as requiring. `Resolve` runs `markBorrowableParams` per method: a statement-level
  write-scan of the body (through nested control flow; no-shadowing makes a written
  name unambiguously the parameter; `spawn` bodies mutate their own snapshot so are
  correctly skipped) sets `Param.Borrow` when the parameter is never written and
  its type is *borrow-safe* - a compound whose `stampDeclaredType` does not recurse
  into element backing (a flat `list`/`map`, or a struct/bytes; a `list of list`
  falls back to copy so the stamp can't mutate the shared backing).
  `Interpreter.bindArg` aliases when
  `Param.Borrow && (i.isModule || i.entryGlobalsImmutable)`, else copies as before.
- **What it buys.** It removes a class of accidental quadratics transparently. The
  `markdown` block collectors (`collectFence` / `collectQuote` / `collectList` /
  `tableFrom`, all `lines as list of string`) deep-copied the whole line list once
  per block (lines x blocks); parsing a 160-chapter synthetic book as one document
  drops from **5.1 s to 0.85 s** and turns from super-linear (2.9x per input
  doubling) to linear (2.0x). The `countNodes` budget walk borrows its `Node`
  argument, and a globals-free script's own read-only helpers go linear the same
  way. The `.j`-level workaround does not pay off: a count-during-construction
  rewrite of the `countNodes` guard measured a **wash** (9.5 s vs 9.6 s on a
  100k-node document), because eliminating the read-only walk forces threading the
  count back through the builders, re-introducing an equal volume of copies. No
  reference-semantic handle a pure `.j` module can hang a large list on exists, so
  the copy is only removable below the language.
- **Discipline.** TinyGo-clean, reflect-free, strict behaviour parity, race-clean.
  `internal/parser/borrow_test.go` pins the write-scan and type gate (a false
  positive would alias a mutated parameter); `internal/interpreter/borrow_test.go`
  and `borrow_internal_test.go` pin the soundness gate (a script with a mutable
  global keeps copy semantics; a globals-free script and a module borrow) and the
  value-semantics parity (borrowed reads correct, caller values intact, returned
  borrows copied at the receiver).

### M25.2 - per-function escape analysis for scripts with mutable globals

**Planned.** M25.1 gates a whole script all-or-nothing on "does it declare any
mutable top-level global". M25.2 replaces that with a per-function decision:
borrow a never-written parameter in any function proven not to mutate a global
**transitively**, even in a script that has mutable globals elsewhere. A
call-graph fixpoint marks each user method "may mutate a global" - seeded by a
direct write to an outer-scope binding and propagated through its callees, with
unanalysable targets (a `func`-value call, `meta.call` / `meta.callMain`) treated
conservatively as "may mutate"; module and builtin calls are safe (module
isolation; builtins do not touch `.j` globals). Composes with M25.1 unchanged -
same `Param.Borrow` flag and `bindArg` path, only the enabling gate widens from
"whole script is globals-immutable" to "this function mutates no global".

---

## M26 - `jvc` package manager (decks)

**Planned.** A package manager for Jennifer, in the shape of PHP's Composer (or Rust's
Cargo): declare dependencies in a manifest, `jvc install` resolves and
fetches them, and the app imports what it pulled. Installing an app becomes
`git clone` + `jvc install`, and `jvc update` advances within the declared
constraints.

- **Packages are "decks".** A deck is a distributable, versioned bundle of
  `.j` modules, published to a public **deck repository / registry**
  (provided later) that `jvc` resolves and fetches from - packagist-style; a
  deck can also come straight from a git URL.
- **Naming conventions.** A deck has two separate identities. Its **canonical
  name** is the `@vendor/deckname` scope in `deck.toml` (what imports and jvc key
  on) - kept clean, with no "deck" word: an official deck is `@jennifer/routeros`,
  imported `import "@jennifer/routeros/";` -> `routeros.*`. Its **GitHub repo
  name** is cosmetic (jvc reads `deck.toml`, not the repo name), so the "deck"
  marker lives there and only there:
  - **Official** decks (in the `jennifer-language` org) use a **`deck-` prefix** -
    `jennifer-language/deck-routeros`. The prefix is the "*not core, but official
    deck*" statement: it clusters the decks and keeps the org's top level readable
    (core `jennifer` / `homebrew-tap` vs `deck-*`). The org already supplies
    "jennifer", so `deck-` is the useful signal (deck vs core).
  - **Community** decks (any account) can be named anything - jvc only needs it
    configured - but the **suggested** form is a **`jennifer-` prefix**,
    `alice/jennifer-routeros`: on a personal account there is no Jennifer
    namespace, so "which ecosystem" is the useful signal instead.
  - **All** deck repos carry the GitHub **topic `jennifer-deck`** for discovery,
    which works regardless of the repo name.
- **Installed into the `vendor/` tree M19.7 resolves.** `jvc` writes decks into
  the project-local `vendor/` tree that the interpreter already addresses through
  the `@scope/package` import form and vendor-root discovery shipped in
  M19.7 - so a hand-populated `vendor/` imports before `jvc`
  exists, and `jvc` is just the manager layered over that resolver. Nothing is
  global; each app owns its decks beside it.
- **The one remaining language-surface question: inline version selectors.**
  M19.7 resolves `import "@jennifer/supercms/" as cms;` (the trailing `/` expands to
  the package-named entry `supercms/supercms.j`) against whatever is installed.
  `jvc` supplies the *default* - plain `import @jennifer/supercms;` takes
  the version `jvc` resolved (declared in `deck.toml`, pinned in the lockfile),
  version-transparent, which is what almost every script wants. The opt-in for
  side-by-side versions is a **per-import selector** matched against the installed
  set (never triggering a fetch): `@jennifer/supercms=1.2.3` (exact), `>=1.2.3` / `~`
  / `^` (a `semver` constraint over what is installed), or `#cefa234` (a git
  commit); one script can pin `=1.x` while another pins `=2.x`, and two versions
  in one file take distinct `as` aliases. An unsatisfiable selector errors
  pointing at `jvc install`, not a silent download. **Cost:** the selector is
  **new grammar** (the lexer reads `@vendor/deck` + `semver` op + `#commit` as one
  token up to `;`); the plain M19.7 string-path form is the no-new-grammar
  fallback that loses only the inline selector.
- **`deck.toml` manifest + lockfile.** `deck.toml` (TOML, so it needs the `toml`
  library) declares required decks and constraints (`bitcoin = ">=1.2.0"`), and
  `jvc` produces a **`camcorder.lock`** pinning exact resolved versions (content
  hash per deck) so `git clone` + `jvc install` is reproducible. Dependency sets
  split by section (`[prod]` / `[dev]`, `jvc install --prod`), with a **taxative**
  (the section is the exact set) vs **additive** (base plus the section's extras)
  mode still to design.
- **`jvc` owns the lifecycle:** dependency resolution (semver constraint
  solving across the graph), downloading, `jvc update` (advance to the
  newest constraint-satisfying versions, rewrite `camcorder.lock`), integrity
  pinning, and the publish flow to the registry.

**Migrating bundled modules out to decks.** Once decks exist, niche or
product-specific modules that ship bundled today should graduate *out* into
decks (the archetype is `gotify` - a single-product push integration every
install need not carry); language-fundamental modules stay bundled. Moving one
changes its import, so it is a breaking change under semver: within 1.x ship it
**both ways** (bundled + `@`-deck) with the bundled copy marked `@deprecated` so
imports re-point at their own pace, let the two drift without breaking, and
**remove the bundled copy in 2.0.0**. Conversely, *new* third-party service
integrations ship **as decks from the start** rather than as core modules (core
stays general primitives; specific-vendor clients live in the ecosystem) - the
`M27` candidate-deck ecosystem collects the list (GitLab, GitHub, Steam,
TheMovieDB, Jellyfin, Frigate, RouterOS, ...).

**Requires:** the public deck **registry** (separate infrastructure, provided
later); its language-side prerequisites - the `@scope/package` resolver + vendor
root (`M19.7`), `toml`, the module system, and `semver`'s range surface - have
shipped.

---

## M27 - candidate decks (deck ecosystem)

**Planned.** A running parking lot of deck ideas - third-party service and integration
clients that, once `jvc` / decks (M26) land, ship as **decks** rather than core
`modules/`. The rule: core stays *general primitives* (protocols, formats,
infrastructure - `http`, `graphql`, `csv`); a client for one *specific vendor or
service* lives in the ecosystem as a deck (independently versioned,
community-maintainable, so vendor API churn never touches the core). This list is
demand-driven and open-ended, a collection not a commitment.

Most are thin clients over `http` / `rest` + `json` (plus `xml` where a vendor
returns XML, and the `graphql` module where the API is GraphQL). Each is a
login / token step, a generic `call(path, params) -> json.Value`, and a handful of
conveniences; a fat typed wrapper is explicitly **not** the plan - these APIs are
enormous and firmware-versioned, so a thin client ages far better.

### M27.1 - self-hosted infrastructure

A LAN appliance, usually a **self-signed** cert. Per-vendor maturity differs and
should set the order, not the vendor:

- **`routeros`** - a *full* RouterOS abstraction layer (MikroTik), well beyond the
  small bundled `mikrotik.j` API client. Already in progress; the likely **first
  published deck**.
- **`proxmox`** - Proxmox VE: documented REST / JSON, API-token header auth
  (`PVEAPIToken=...`). Clean.
- **`vmware`** - vCenter / vSphere (vCSA): the vSphere Automation **REST** API
  (JSON, session auth), clean and tractable like Proxmox. A standalone **ESXi**
  host is the messier case - historically only the SOAP / VMOMI Web Services API
  (XML / SOAP, heavier), with just partial REST on 8.x - so vCenter is the target,
  a bare ESXi host best-effort.
- **`synology`** - DSM: the cleanest NAS API - a documented Web API
  (`SYNO.API.Auth` login -> session `sid`, JSON responses).
- **`unraid`** - the newer official API is **GraphQL** over HTTP with an API key.
- **`qnap`** - QTS: the messiest - much of the useful surface is undocumented,
  reverse-engineered CGI with XML responses and a legacy hashed-password auth;
  firmware-fragile and hard to keep green. Lowest priority, on real need only.
- **`ugreen`** - NASync / UGOS Pro: **no official API** - only the internal
  token-based API the web GUI uses (log in for a token, then GET), reverse-
  engineered by the community (a working Home Assistant integration exists). Same
  keep-green risk as `qnap`; UGOS is new and evolving, so revisit if UGREEN ever
  ships official docs.
- **`jellyfin`** - the self-hosted media server's REST API (API-key / token auth).
- **`frigate`** - the Frigate NVR REST API (events / config / recordings), often
  paired with its MQTT feed (the `mqtt` module).

### M27.2 - public / SaaS APIs

- **`gitlab`** / **`github`** - dev-platform clients; both expose REST **and**
  GraphQL, token auth.
- **`steam`** - the Steam Web API (JSON, `?key=` auth); parts of the surface are
  community-reverse-engineered, which is exactly why it belongs in a deck, not core.
- **`themoviedb`** - TMDB's clean, well-documented REST JSON API (bearer / key auth).

### M27.3 - daily helpers

A grab-bag of small, everyday helper decks - the little utilities a script reaches
for often, each a tiny pure-`.j` deck.

- **cli spinners** - a small deck of terminal progress spinners for a long-running
  task: a catalogue of spinner styles (dots / line / bar / braille / arc / ...),
  each individually **colourable** (through `ansi`) and driven at its **own speed**
  (frame interval). Self-suppresses when stdout is not a TTY (`os.isTerminal`), so
  piped output stays clean.

### M27.4 - bioinformatics

A sequence-manipulation deck for DNA / RNA / protein, modelled on the
[Sequence Manipulation Suite](https://stothardresearch.ca/sequence-manipulation-suite/)
(SMS) tool catalogue - the classic, comprehensive reference for this surface.
Almost all of it is pure string / list / map work, so it is a natural
**pure-`.j` deck** (leaning on `strings`, `regex`, `lists`, `maps`, `math`, and
`stats`, no Go), dogfooding the language and community-maintainable as tables and
algorithms are added. Grouped SMS-style:

- **Transforms** - `complement`, `reverseComplement`, `reverse`, `transcribe`
  (DNA <-> RNA), `translate` (codon table), six-frame translation, `splitCodons`,
  amino-acid `oneToThree` / `threeToOne`.
- **Composition & properties** - `gcContent`, base / amino-acid composition,
  `dnaStats` / `proteinStats` summaries, `molecularWeight` (DNA / RNA / protein),
  `meltingTemp` (Wallace + nearest-neighbour), `isoelectricPoint` (pI), `gravy`
  (hydropathy), `codonUsage`. The thermodynamic / pI / nearest-neighbour formulas
  want `exp` / `ln` / `log`, so they pull in `M24.10` (`math` foundations).
- **Search** - exact and IUPAC-ambiguity pattern find (via `regex` character
  classes), fuzzy search (n mismatches), ORF finder (six frames), CpG islands,
  restriction-site search and `restrictionDigest` (fragments) over a bundled
  enzyme table.
- **Formats** - FASTA parse / write (a `Record{id, description, sequence}` list),
  FASTQ reads (sequence + quality), format conversion, `filterDna` /
  `filterProtein` (strip non-sequence characters).
- **Manipulation** - `randomDna` / `randomProtein` (length + optional
  composition), composition-preserving `shuffle`, point `mutate` (rate), range /
  sliding-window extraction.
- **Comparison** - pairwise `alignGlobal` (Needleman-Wunsch) / `alignLocal`
  (Smith-Waterman) with percent `identity` / `similarity`, plus `hammingDistance`
  / `editDistance`. Alignment is the one `O(nm)` hot loop - fine in `.j` for the
  small sequences a deck user handles; a candidate Go primitive only if
  whole-genome throughput is ever needed.
- **Reference data** - the standard genetic code + alternative codon tables, IUPAC
  ambiguity codes, per-residue property tables (MW / pKa / hydropathy), and a
  common-enzyme table - all plain `.j` maps a community can extend.

FASTA / FASTQ I/O and alignment cover the "molecule structures" the original note
gestured at; PDB / 3-D structure parsing stays out of v1 (a much larger, separate
effort).

### M27.5 - forensic / statistical genetics

The statistical-genetics sibling of the sequence deck (M27.4): it works on
**profiles** (an unordered allele pair per autosomal STR locus, plus uniparental
**haplotypes** for mtDNA / Y-STR) and reference frequency / count data, not on
sequences, so it shares no code and no audience with SMS. Pure `.j` (probability
arithmetic over allele-frequency maps, no Go, TinyGo-clean), leaning on `math`
and on `xml` / text parsing to ingest a published frequency table.

- **Match probabilities** - Hardy-Weinberg single-locus genotype frequencies
  (homozygote `p^2`, heterozygote `2 pa pb`), with the NRC II theta / Fst
  sub-population correction (Balding-Nichols), multiplied across loci:
  `randomMatchProbability` (RMP), the single-source match `LR = 1 / RMP`, and
  `cpi` / `cpe` (combined probability of inclusion / exclusion) for mixtures. A
  minimum-allele-frequency floor (`5 / 2N`, so the table keeps each locus's `N`)
  handles rare or unobserved alleles.
- **Lineage markers (mtDNA / Y-STR)** - mitochondrial and Y-chromosome markers are
  haploid, non-recombining, and uniparentally inherited, so match probability is
  **not** Hardy-Weinberg but a direct **haplotype count**: frequency `k / N` in a
  reference database, with a **Clopper-Pearson** exact-binomial upper bound as the
  conservative estimate (which pulls in the `beta` distribution `M24.11` adds to `stats`).
  Deliberately **database-independent**, so the deck supplies the estimator
  (`haplotypeFrequency(k, N)` / `lineageMatchProbability`) and the caller feeds the
  count from whatever database they queried. mtDNA adds a haplotype coded as
  differences from the **rCRS** (e.g. `263G 315.1C`) plus the substantive `align`
  step that renders a raw sequence into that standard nomenclature; Y-STR is a
  per-locus repeat-count vector with exact / single-step comparison.
- **Kinship likelihood ratios** - relationships encoded as IBD coefficients
  (kappa0 / kappa1 / kappa2: parent-child `(0, 1, 0)`, full sibs
  `(1/4, 1/2, 1/4)`, half-sib / avuncular / grandparent `(1/2, 1/2, 0)`, first
  cousins `(3/4, 1/4, 0)`, ...). `kinshipLR(a, b, relationship, db)` tests one
  relationship against another; `paternityIndex(mother, child, allegedFather,
  db)` gives a combined paternity index (CPI) and `probabilityOfPaternity`
  (`W = CPI / (CPI + 1)`), with a stepwise STR mutation model to survive a lone
  inconsistency. General pedigrees (beyond pairs / trios) need a peeling engine -
  **Elston-Stewart** - the one substantial algorithm, the Familias / `forrel`-style
  capability.

Mixture deconvolution (multi-contributor, drop-in / drop-out - the EuroForMix
space) is a larger, later effort layered on this base.

A concrete frequency source to wire in first, as a sample: the ENFSI STR reference
database **[STRidER](https://strider.online/frequencies)** - a
`loadFrequencies(xml, "strider")` over the `xml` library, with its
[formulae page](https://strider.online/formulae) and online calculator as the
exact-conventions spec and a validation target.

### M27.6 - NGS / high-throughput sequencing

Unlike the pure-`.j` decks above, NGS breaks the model on two axes - **scale**
(FASTQ.gz files run 10s of GB, BAM 100s, so everything **streams**, never
load-into-memory) and **compute** (alignment / assembly / variant calling are
heavily SIMD-optimised C a tree-walker is ~100-1000x too slow to replace). So the
deck is deliberately the **glue and light-I/O layer**, not the heavy engine, with
a different posture from the pure-`.j` decks: **Go-backed streaming parsers** (the
decompress-and-parse hot loop in Go, the `net.readAll` / `binary` pattern) with
the per-record logic in `.j`. In scope:

- **Streaming format I/O** - FASTQ (gzipped, over `compress` + `fs` handles) and
  the tab-delimited **SAM** / **VCF** / **BED** / **GFF** / **GTF** text formats:
  parse / filter / convert as a stream, so a 50 GB file never lands in memory.
- **QC + preprocessing** (FastQC / fastp-lite) - per-base quality and read-length
  / GC distributions, adapter detection, quality / adapter trimming, length
  filtering, subsampling: one streaming pass over FASTQ.
- **Interval ops** (bedtools-lite) - overlap / intersect / merge over BED / GFF
  features.
- **Pipeline orchestration** - the real sweet spot: NGS is fundamentally
  `bwa | samtools | gatk` glued together, and Jennifer already has `os.run` /
  `os.spawn` + `spawn` concurrency, so it can drive the standard tools, parse
  their output, manage intermediate files, fan out over samples in parallel, and
  handle errors - a Snakemake-lite in a real language.

Out of scope (wrap and pipe the native tool, do not reimplement): read
**alignment** (BWA / Bowtie2 / minimap2), de-novo **assembly** (SPAdes), **variant
calling** (GATK), and heavy **BAM / CRAM** handling - which additionally needs
**BGZF** (blocked gzip for random access), a `compress` gap and a Go addition
whose standalone value is limited without the downstream compute the deck omits.

---

## M28 - multiplatform: promote macOS / Windows to supported

Linux is the only *supported* platform, but best-effort **unsupported** macOS /
Windows binaries (the standard-Go `jennifer`, via cross-compile) already ship each
release - so the work is not "add the ports" but **promoting them to supported**,
which graduates the "cross-build for macOS / Windows" 1.0.0 distribution
requirement. A portability audit found the surface small: separators / EOL /
`$HOME` / temp are already `runtime.GOOS`-derived (`internal/lib/os/oslib.go`),
`os/exec` keys on `runtime.Compiler != "tinygo"` (not GOOS, so it is enabled on
Windows), and signals + the four Linux-only hardware libs (`serial` / `spi` /
`i2c` / `gpio`) stub cleanly on non-Linux (`*_other.go`). **Extra distribution
packaging** (a Homebrew tap, Snap, Nix flake, Flatpak / AppImage) stays a
per-format nice-to-have, shipped only when a user asks and a maintainer keeps it
green - none blocks a release.

### M28.1 - Windows: promote to supported

**Planned.** The **Windows** track is a handful of concrete gaps, not a rewrite:

- **Exe-relative module default.** `compileDefaultSysmoddir` bakes one hardcoded
  POSIX path (`/usr/share/jennifer/modules`, `internal/module/sysmoddir.go`) into a
  Windows binary; give it a Windows-native, exe-relative default
  (`<dir(os.Executable())>\share\jennifer\modules`) via a build-tag split
  (`sysmoddir_windows.go` / `_unix.go`), so a portable-zip user's `import "name.j";`
  resolves with no env var (the `M21.13` installer's `JENNIFER_SYSMODDIR` stays the
  explicit override; precedence unchanged).
- **Per-OS golden strategy.** `examples/expected/osinfo.txt` is the sole
  platform-pinned golden (pins `linux` / `amd64` / `/` / `:`, compared byte-exact in
  `cmd/jennifer/examples_test.go`); add per-OS expected-file selection or a
  `runtime.GOOS`-gated skip for the osinfo canary (already flagged at
  `examples/osinfo.j`).
- **`fs.chmod` / `fs.chown` on Windows.** Define the Windows behaviour (a friendly
  catchable error is acceptable; the `chown` test is already Linux-gated), and
  document that signal-based graceful shutdown is limited on Windows
  (`signal_other.go` stubs `os.catchSignal`).
- **A `windows-latest` CI test job** running `go test ./...` so Windows correctness
  is actually verified (the exec suite self-skips off Linux; the osinfo golden is
  the known failure the item above resolves); once green, move windows/amd64 out of
  the `build-unsupported` matrix into the supported set and drop the "unsupported"
  labelling for that arch.

**Requires:** none.

### M28.2 - macOS: promote to supported

**Planned.** The parallel case: the same "already ships unsupported, promote it"
shape as `M28.1`, and simpler - macOS lacks even the module-path blocker Windows
has (the POSIX exe-relative default resolves cleanly), and its separators / EOL /
`$HOME` match Linux. A `macos-latest` CI test job running `go test ./...` verifies
correctness (reusing the per-OS osinfo golden strategy from `M28.1`); once green,
move darwin/amd64 + darwin/arm64 out of the `build-unsupported` matrix into the
supported set and drop the "unsupported" labelling. **Requires:** none.

---

## Requirements for 1.0.0 stable

- **Cross-build for macOS / Windows.** The `M28` multiplatform track
  (`M28.1` Windows, `M28.2` macOS) does this; ships as soon as it lands.
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
