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
| M19.7 | `@scope/package` resolution (vendored decks) | A leading `@` is a vendored-deck reference expanded by one function (`resolveVendor`): the `@` swaps in the vendor root and a reference not ending in `.j` gets the package-named entry appended, so `@claude/bitcoin`, `@claude/bitcoin/`, and `@claude/bitcoin/utils.j` all reduce to a plain absolute path (after which run-once cache + M19.5 path identity are untouched). The entry is `<package>.j`, so `moduleStem` gives the package name and the display namespace / default alias fall out (`import "@claude/bitcoin/"` binds `bitcoin.`); two same-package decks across scopes are distinct types, colliding only on the default alias (resolve with `as`). Vendor root via `FindVendorRoot`: `--vendor` > `JENNIFER_VENDOR` > nearest `vendor/` above the program. Path safety: `@` front-only, no `.`/`..`, the file must stay inside the deck; a missing root is a guided error. The `jvc` manager over this stays DRAFT#12. |
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
| M21.13 | Windows installer | An Inno Setup script (`packaging/windows/jennifer.iss`) built by a `windows-latest` CI job into `jennifer-<ver>-setup.exe`: per-user (no admin), adds to `PATH`, bundles the system modules + sets `JENNIFER_SYSMODDIR`, opt-in `.j` association, Apps & Features uninstaller. Unsigned, best-effort **unsupported** build. `scripts/build-windows-installer.sh` recreates it locally via Wine. Promoting Windows to *supported* is the follow-on, `M25.1`. |

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
| M22.7 | `graphql` client module | Thin GraphQL client over `http` / `rest`: `client(endpoint)` + `bearer` / `basic` / `header` / `withCA` / `insecure` builders, `query(c, query, variables) -> json.Value` (POST `{query, variables}`; result under `/data`). Gets the GraphQL convention right - a non-empty top-level `errors` array is an **HTTP 200**, not a non-2xx - raising a `graphql` error with the joined messages; a non-2xx also raises. `queryNamed` / `tryQueryNamed` add an `operationName`; `tryQuery` / `tryQueryNamed` return the raw envelope (no raise on GraphQL errors) for structured-error handling via exported `hasErrors` / `errorMessages` + `json` accessors. POSTs the endpoint **verbatim** via `http.requestTls` (`rest`'s `joinUrl` would append a trailing slash). The GraphQL dependency `DRAFT#24` (Unraid) consumes. |
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

**Planned.** The first batch of [horizon](horizon.md) drafts graduated into a
scheduled track: a CLI ergonomic, the language's biggest expressiveness feature, a
concurrency-coordination layer, libraries - each already
design-shaped in the horizon collection and now committed. Five sub-milestones.
Each carries the standard discipline (spec + grammar EBNF / PEG + editor
highlighters where a language feature lands, a Go package +
`internal/stdlib.InstallAll` line + `docs/libraries/` reference + cheatsheet where
a library lands, both binaries build, and the full test close-out).

### M24.1 - run profiles / `--env` CLI flag (compacted)

**Done.** `jennifer run --env=prod script.j` (or `--env prod`) is a run-profile
flag that **sets `JENNIFER_ENV=prod` before `Run`** - identical to `JENNIFER_ENV=prod
jennifer run script.j`, no interpreter change, so the module side stays pure `.j`
(the `dotenv` `.env.<profile>` selection from `M22.18` is the first consumer, dogfood-
verified end to end). A `cmd/jennifer`-only change: `parseRunArgs` takes the flag
ahead of the script path, `validRunProfile` allowlists the label
(`[A-Za-z0-9_-]`, 1-64, hand-rolled to stay TinyGo-clean, mirroring dotenv's
`validProfile` and blocking a `.env.<profile>` traversal), then `os.Setenv` (an
explicit flag overrides an inherited `JENNIFER_ENV`; a token after the file stays a
program arg). Both binaries honor it. Pinned by `main_test.go`; documented in
`tooling.md` + `cli.md` + a `dotenv.md` cross-link. Graduated `DRAFT#26`.
**Requires:** `M22.18`; no interpreter dependency.

### M24.2 - first-class functions (compacted)

**Done (Tier 1).** Closed the **single largest expressiveness gap**: a function
can now be held in a value (type `func`), so callbacks stop being string-name
dispatch and `lists` gets a higher-order layer. A function value is **immutable,
not a pointer** (holding one aliases nothing mutable), so it does not touch the
value-semantics stance; the syntax reuses the call-vs-name shape the parser
already peeks for - a **bare method name** in expression position *is* the value,
a name followed by `(` is a call (no `&NAME` sigil). New `KindFunc` Value
(`Fn *parser.MethodDef`; `DeepCopy`'s default shares the immutable pointer;
`bindParamValue` treats it as a no-copy scalar) + `TypeFunc` (the `func` keyword in
`parseType`). A bare method name lands as a `ConstRefExpr` the interpreter turns
into a function value (the resolver already deferred such names to runtime); a new
`CallValueExpr` postfix-`(` node calls through any function-valued expression
(`$f(x)`, `$fns[0](x)`, `makeAdder(1)(2)`). The named-call dispatch stays **inline**
in `evalCall` (the hot path - the helper is too large for Go to inline, so routing
every call through it measurably slowed recursion); the byte-identical logic lives
in a `callUserMethod` helper the dynamic path uses, kept in lock-step. Arity /
kind checks, value-semantics arg copies, and the catchable call-depth guard are
identical whether a call is static or through a value (recursion through a function
value still trips the guard). `BuiltinCtx.Invoke` (backed by an unexported `interp`
ref, threading the caller's depth counter) is the callback bridge for the
higher-order **`lists`** layer: `map` (generic result, element-typed at the bind),
`filter` / `find` / `any` / `all` (bool callback; `find` errors catchably on no
match), `reduce`, `sortBy` (key extractor, decorate-sort-undecorate so the key runs
O(n); guarded against an empty list). `fmt` hugs `(` after `$var` / `)` / `]` (all
prior parse errors, so no existing program reflows). **Deferred:** Tier 2
(anonymous closures with by-value capture) and migrating the existing string-name
dispatch in `web` / `testing` / `meta` onto function values - a separate refactor;
the *capability* is delivered. Graduated `DRAFT#18`. **Requires:** none hard;
relates to a future bytecode VM (`DRAFT#17`) and the embedding API (`DRAFT#1`).
Pinned by `internal/interpreter/funcvalue_test.go` (incl. the empty-list crash
regression an audit surfaced); `examples/functions.j` + golden; both toolchains.

### M24.3 - concurrency coordination: cancellation, timeouts, channels (compacted)

**Done.** Gave `spawn` / `task` the coordination it lacked - cancel a task, bound
a wait, and stream between goroutines - and retired the exit-time hang from an
unobserved non-terminating `spawn`. Graduated `DRAFT#21`. **Requires:** none hard
(builds on `spawn` / `task`).

**Cancellation + timeouts.** `TaskState.Cancelled` (atomic) + an
`Environment.cancel *TaskState` set on the spawn snapshot root, reached by every
frame via `env.root`. A shared `loopCheckpoint(env, node)` replaced the five
per-loop `diagReq` polls (while / C-for / for-each / range-for / repeat): it
services the SIGUSR1 diagnostic poll and, inside a cancelled spawn, raises a
**catchable** "task cancelled" so the loop stops at a safe point - one atomic-nil
check per iteration, nil (never fires) on the main goroutine, loop-only so the
`evalCall` hot path is untouched. `task.cancel($t)`; `task.cancelled()` is a
**non-raising** poll (threaded via `BuiltinCtx.Cancel *TaskState`);
`task.waitTimeout` / `task.waitAnyTimeout` are throw-on-timeout bounded waits (a
too-large `ms` that would overflow the ns `time.Duration` is a catchable error,
not a silent instant timeout). A clean partial result uses `try` / `catch` inside
the body. **Rejected** (racy): a `CancelAcked` "poll suppresses the auto-raise"
scheme - a parent `task.cancel` can land before the body's first poll, so the
top-of-loop checkpoint fires before any ack; the auto-raise-is-the-mechanism model
is race-free instead.

**Channels.** `channel of T` + the `channel` library (`internal/lib/channel`):
`make(capacity)` / `send` / `recv` / `close` / `select` (fan-in) / `len` /
`capacity`. A `KindChannel` Value shares a `*ChannelState` pointer (like `task`,
not the spec's integer-registry - simpler, same sharing), so a copy (incl. the
spawn snapshot's `DeepCopy`) refers to the one Go `chan Value` while **`send`
deep-copies the value in** - conduit shared, data copied (no-shared-mutable-state
holds) - and validates the value against the channel's `T` at the send site.
`send`-on-closed / double-`close` are **catchable** errors, not Go panics
(atomic-CAS `CloseOnce`; `recover` on the send/close race); `recv` on a
closed+drained channel throws catchably (drain via `try` / `catch`). `select`
returns the received **value** (not an index - a receive is destructive, no
multiple-return), over the `reflect.Select` `task.waitAny` uses. `channel` is a
**contextual keyword**: lexed as an IDENT, a type only in `channel of T` (the `of`
disambiguates from a struct name). Reserving it broke `modules/amqp.j` (which uses
`channel` as a field / param); the contextual form keeps every program valid *and*
is simpler (`channel.send` / `use channel;` route through the IDENT paths). A
blocked channel op is not at a loop checkpoint, so it is not cancellable and a
channel with no counterpart **hangs** (Go's deadlock detector never fires - the
SIGUSR1 goroutine stays live); close the channel to unblock.

**Concurrency hardening** (post-audit). `channel.make` caps the buffer at
`limits.MaxChannelCapacity` (1<<20 std / 1<<16 tiny) with a catchable error - a
`chan Value` (272 B/slot) allocates eagerly, so an unbounded capacity was a
multi-GB OOM below Go's recoverable `makechan` panic (same class as
`MaxRangeElements`); a `recover` stays as backstop. `TaskState`/`ChannelState`
`ElemTyp` became `atomic.Pointer[parser.Type]` stamped set-once via CompareAndSwap
- a generic channel held in a `list` and bound by two spawns concurrently
otherwise raced on the plain field (confirmed under `-race`).

Pinned by `channel_test.go` (8 tests incl. value-semantics, concurrent-bind
`-race` guard, capacity-ceiling + huge-capacity regressions, identifier-still-works)
+ `task_cancel_test.go` (8 tests incl. the ms-overflow guard);
`examples/cancellation.j` / `channels.j` + goldens; both toolchains.
**Deferred:** cancellable channel ops and index-returning `select`.

### M24.4 - `stats` library (compacted)

**Done.** A `stats` system library (`internal/lib/stats`): 26 descriptive
statistics over `list of int` / `list of float` - central tendency (incl.
geometric / harmonic / weighted means, `modes`), spread (population + `sample*`
Bessel n-1, `range`, `iqr`, `mad`), shape (`skewness`, excess `kurtosis`), order
statistics (`percentile` / `quartiles` / `min` / `max`), `sum`, `zscore`, bivariate
`correlation` / `covariance` / `sampleCovariance`, and `describe` -> a
`stats.Summary` struct. Real-valued reductions return `float`; the selections and
`sum` / `range` preserve the input kind (overflow-checked); `quartiles` / `zscore`
return `list of float`. `variance` / `stddev` / `covariance` and the moments are
population (÷ n, NumPy default), `sample*` ÷ n-1, `kurtosis` excess. Strict like
`math`: any undefined result (empty list, bad percentile, zero-variance
bivariate/shape input, int-sum overflow, non-positive geo/harmonic input,
constant `zscore`) is a catchable error, not a NaN. Pure Go stdlib, TinyGo-clean.
Pinned by `statslib_test.go`; `examples/stats.j` + golden. Graduated `DRAFT#5`.
**Requires:** none. Its `linalg` companion is `M24.6`; the numerical-inference
piece (probability / regression / confidence intervals) is `M24.11`.

### M24.5 - `ml` predictive / classical machine learning (compacted)

**Done.** A `ml` **core** system library for classical / predictive machine
learning on tabular data (scikit-learn-lite), over `stats` / `linalg`.
Deliberately core, not a deck (classical algorithms are stable native-numeric
primitives), and explicitly **not** a deep-learning framework (tensors / autodiff
/ deep-net training are native GPU / C++; "run a pre-trained model" is already
`http` / `os.run`). A **fit / predict** shape: a fit function returns an opaque
`ml.Model` handle (registry-backed, immutable, read-only-shareable), applied with
`predict` / `transform` / `predictProba` / `free`. Models: regression
(`linearRegression` / `ridge` / `lasso` coordinate-descent, plus `kNNRegressor` /
`decisionTreeRegressor` / `randomForestRegressor`), classifiers (`kNN`,
`naiveBayes`, `logisticRegression` binary + multiclass one-vs-rest, `decisionTree`
CART/Gini, `randomForest`), `kMeans` (k-means++), `pca` (Jacobi
eigendecomposition), and `standardScaler` / `minMaxScaler`. Introspection reads
the learned parameters (`coefficients` / `intercept` / `centroids` / `components`
/ `explainedVariance` / `featureImportances`). Selection / preprocessing:
`trainTestSplit` -> `ml.Split`, `kFold` -> list of `ml.Fold`,
`polynomialFeatures`. Metrics: `accuracy` / `precision` / `recall` / `f1` (+
positive label) / `confusionMatrix` / `rocAuc` (tie-aware) / `logLoss` / `rmse` /
`mse` / `mae` / `r2`. X is a `list of list of float/int`, y a
`list of float/int`. Random models draw from
`math`'s `randSeed`-able source (reproducibility, not secrecy - a seed is not a
secret, so not `crypto`). Strict: a degenerate input is a catchable error, and
the cost-driving hyper-parameters are bounded (tree depth / forest size / iters /
epochs / kFold work) so a runaway value errors instead of a stack overflow /
hang / OOM. Pure Go stdlib, TinyGo-clean, both binaries; pinned against known
results.

### M24.6 - `linalg` library (compacted)

**Done.** A `linalg` system library (`internal/lib/linalg`): linear algebra over
Jennifer's own value types, the companion to `stats`. **Vectors** are
`list of float` - `dot`, `distance`, `cross` (3-vectors), `normalize`;
**matrices** are `list of list of float` - `transpose`, `trace`, `determinant`,
`inverse`, `solve`, `identity`, `zeros`, `shape`. Four ops are **polymorphic**
over a vector or a matrix (`norm` = L2 / Frobenius, `scale`, `add`, `sub`); and
`matmul` dispatches on operand shape (matrix*matrix -> matrix; matrix*vector or
vector*matrix -> vector; vector*vector errors, `dot` is the tool for that).
Algorithms are direct (Gaussian / Gauss-Jordan for solve / determinant /
inverse) - no `gonum` - so pure Go stdlib, TinyGo-clean, both binaries. A matrix
is a plain nested list for v1 (idiomatic, value-semantic; a Go-backed opaque
handle is the noted future escape hatch for big-matrix throughput). Strict like
`math` / `stats`: a dimension mismatch, a non-rectangular matrix, a singular
`inverse` / `solve`, the zero vector to `normalize`, or a non-finite (overflow)
result is a catchable error, not a NaN; every vector / matrix (the `identity` /
`zeros` constructors and the operation inputs alike) is bounded by
`limits.MaxMatrixElements` - sized in 272-byte Value cells like MaxChannelCapacity,
since a `linalg` value is always fully materialised - so an oversize dimension is a
catchable error rather than an uncatchable OOM, which also bounds the O(n^3)
routines. Eigenvalues / decompositions (LU / QR / Cholesky) / rank stay on the
horizon as advanced `linalg` the numerical-inference (`M24.11`) work would build
on.

### M24.7 - `asn1` library (compacted)

**Done.** An `asn1` system library (`internal/lib/asn1`): ASN.1 BER decode / DER
encode, the byte-level enabler for the LDAP / SNMP clients (later X.509 / PKCS).
Hand-rolled in Go - Go's `encoding/asn1` is DER-only + reflect-bound - so no
dependency: `decode` reads full **BER** (indefinite lengths, high-tag-number
identifiers), `encode` emits canonical **DER**. Mirrors the opaque-value
`KindObject` shape (`json` / `toml` / `xml` / `yaml`): `decode` yields an opaque
`asn1.Value` (element tree - class / tag / constructed / content / children)
walked by `(node, pointer)` accessors whose pointer tokens are **child indices**
(`typeOf` / `tagClass` / `tagNumber` / `isConstructed` / `get` / `has` / `length`
/ `asInt` / `asBool` / `asString` / `asBytes` / `asOid` / `isNull`); built with
typed constructors (`integer` / `enumerated` / `boolean` / `null` / `octetString`
/ `utf8String` / `printableString` / `ia5String` / `oid` / `sequence` / `set`,
plus `tagged` EXPLICIT / `retag` IMPLICIT context tagging) and serialised by
`encode`. Strict: malformed input and wrong-type leaf reads are catchable errors;
a decode-node budget (sized in ~1.5 KB Value cells) + nesting cap turn a decode
bomb into a catchable error, not an OOM / stack overflow; long-form lengths
accumulate width-independently and tag numbers are bounded so build / decode stay
symmetric. Encoder output byte-verified against Go's `encoding/asn1`; pinned by
`asn1_test.go`; `examples/asn1.j` + golden. Pure Go stdlib, TinyGo-clean, both
binaries.

### M24.8 - `snmp` client + agent (compacted)

**Done.** An SNMP v1 / v2c **client and agent** (`modules/snmp.j`), the simpler
ASN.1-over-the-wire protocol: compact PDUs, UDP, community-string auth, no SASL. A
`.j` module - the BER byte-crunching stays in `asn1` (`M24.7`), the transport in
`net` UDP. *Client:* `client` / `clientWith` -> `Client`, then `get` / `getNext` /
`set` / subtree `walk` return a `list of Varbind` `{oid, type, value, number}`
typed by SNMP value type (universal integer / octetString / oid / null;
`[APPLICATION]` counter32 / gauge32 / timeTicks / ipAddress / counter64 / opaque
via `asn1.retag`; `[CONTEXT]` noSuchObject / noSuchInstance / endOfMibView); each
exchange checks the request-id, bounds the wait with a deadline + retries, and
normalises any failure to one `snmp`-kind error. *Agent (server / hardware
simulator):* `agent(community, version, bindings)` + `serve` / `serveOn` answer
GET / GETNEXT / SET for a MIB (GETNEXT in numeric OID order so a client `walk`
traverses it; wrong-community / malformed datagrams dropped). The codec and
dispatch are factored pure (network-free), so the overlay round-trips every value
type and agent path without a socket; the live send/recv and a self-contained
client<->agent loop run over loopback UDP in the Go suite, and the client was
verified against real hardware. No SNMPv3 / USM, traps, or GETBULK. Module
discipline (`snmp_test.j` 100%, doc, two demos, `cmd/jennifer/snmp_test.go`,
catalog rows); default `jennifer` binary only (`net`).

### M24.9 - `ldap` client and directory server (compacted)

**Done.** An LDAP v3 client and lightweight directory server (RFC 4511)
(`modules/ldap.j`) on `asn1` BER + `net`, LDAPS / StartTLS via the shared
`transport.Security` enum; client and server share the private codec, so both
live in one module (the `snmp` `M24.8` precedent). *Client:* `connect(address,
security)` -> `Conn`, then simple `bind` / SASL-SCRAM `bindSasl` (via `sasl`)
returning `Result{code, matchedDn, message}`; `search` (`SCOPE_BASE` / `ONE` /
`SUB`) with an RFC 4515 `parseFilter` or the constructors (`equals` / `present` /
`greaterOrEqual` / `lessOrEqual` / `substrings` / `allOf` / `anyOf` / `negate`),
`searchPaged` (Active Directory's paged control); writes `add` / `modify` (`change`
+ `MOD_ADD` / `MOD_DELETE` / `MOD_REPLACE`) / `delete` / `modifyDn` /
`passwordModify` (RFC 3062); a non-UTF-8 attribute value (AD `objectGUID`) comes
back base64. *Server:* a directory of `entry` / `group` records answers simple bind
(`userPassword` via `password`'s `ssha` / `sha` / `ssha256` / `sha256` / `pbkdf2` /
plaintext schemes) and filtered search (an evaluator over the same `asn1` tree,
`userPassword` withheld unless requested) - read-only over the wire but mutable
from code (`addEntry` / `modifyEntry` / `deleteEntry` / `setAttribute`) through a
shared `kv` store (`directory` in-memory, `openDirectory` file-backed and
persistent), enough to back an auth portal such as Authelia (worked config in the
doc). The codec + filter + directory logic is factored pure, so the overlay
(`ldap_test.j`) round-trips requests / responses without a socket; the live path
plus a Go fake for the writes the read-only server rejects run in the Go suite
(`TestLdapDirectory` / `TestLdapWriteOps`, plus `TestLdapServerRobustness` for
malformed-request handling). AD Kerberos / GSSAPI SASL is out of scope (simple
bind over TLS); referrals are avoided by binding a specific DC / Global Catalog.
Default `jennifer` binary only (`net`).

### M24.10 - `math` foundations + special functions (compacted)

**Done.** Fill the two `math` gaps a batteries-included language should not carry
into 1.0, both folded into the existing `math`. **Everyday:** trigonometry (`sin`
/ `cos` / `tan` + inverses + `atan2`), hyperbolic (`sinh` / `cosh` / `tanh` +
inverses), exponentials / logarithms (`exp` / `expm1` / `ln` / `log10` / `log2` /
`log1p` + arbitrary-base `log`), `cbrt` / `hypot` / `sign` / `trunc`, and
combinatorics (`factorial` / `comb` / `perm` / `gcd` / `lcm`, exact-int with
overflow errors), plus `TAU`. **Special:** `erf` / `erfc`, `gamma` / `lgamma`,
`beta` / `lbeta`, and the regularized incomplete gamma (`regGammaP` / `regGammaQ`)
and beta (`regBetaI`) - the CDF engine `M24.11` needs, hand-rolled by the standard
series / continued-fraction algorithms and verified to machine precision. Strict
throughout: a domain error, an overflow, or a non-converged / non-finite CDF is a
catchable error, never a NaN (the CDF builtins guard the hand-rolled results and
clamp into `[0, 1]`). Go stdlib for the base functions; TinyGo-clean, both
binaries.

### M24.11 - distributions + inferential `stats` (compacted)

**Done.** The classical numerical-inference layer (the `scipy.stats` / Excel
surface) folded into `stats`, built on `M24.10`'s `math` special functions.
**No separate `prob` library** - distributions live with the descriptive stats
and inference they are used with (as in `scipy.stats` / R's `dnorm` / `pnorm`),
so `use stats;` gives the whole toolkit. Two new `stats` layers:
- **Distributions** (`distributions.go`) - normal / t / chi-square / F /
  binomial / Poisson, flat R-style names (`normalCdf` / `normalQuantile` /
  `tCdf` / `binomialPmf` / ...): pdf/pmf, cdf (reusing `math`'s exported
  `RegularizedGammaP` / `RegularizedIncBeta`), quantile (Acklam probit +
  bracketed bisection), and Box-Muller `normalSample` on `math`'s random source.
- **Inference** (`inference.go`) - `linearRegression` / `multipleRegression`
  (self-contained Gauss-Jordan solve, no `linalg` dependency), `confidenceInterval`,
  `proportionCi` (Wald / Wilson / exact **Clopper-Pearson**), `tTest` / `tTest2`
  / `chiSquareTest` / `fTest` / `anova`, and `histogram` (Excel `FREQUENCY`).
  Results are `stats.Regression` / `Interval` / `Test` structs.

Strict throughout: every p-value / CDF is finite-guarded and clamped to
`[0, 1]`, and a degenerate / out-of-domain / overflowing input (zero variance,
singular design, negative observed count, magnitudes past the float64 ceiling)
is a catchable error, never a NaN. Pinned against scipy reference values;
pure-value, TinyGo-clean, both binaries.

### M24.12 - scientific-notation float literals (compacted)

**Done.** Float literals accept an `[eE][+-]?digits` exponent (`6.022e23`,
`1.6e-19`, `2.5E8`, `1e10`) - the exponent makes a literal a **float** even with
no fraction (`1e10` is a float; `1e` a positioned lex error), takes no `_`
separators (the mantissa still does), and is decimal-only (`0xe5` keeps `e` as a
hex digit). A lexer-only change (`readNumber` scans the exponent; the parser's
`strconv.ParseFloat` reads the lexeme); additive / non-breaking. Strict at the
magnitude edge: overflow (`1e400`) is a positioned parse error, never `Inf`;
underflow (`1e-400`) rounds to a finite `0.0` (the check bans the non-finite, not
a finite zero). On stance #1 it is admitted for the same reason `0xff` / `0o755`
carry domain intent (not a parallel API), and it closes a round-trip gap - the
interpreter already prints `1e+21` but could not read it back. Reasoning in
`design-decisions.md`.

### M24.13 - `uri` module + `encoding` percent / form codecs (compacted)

**Done.** URL handling, factored so the byte-level encoding lives in the
`encoding` system library and the URL semantics live in a shared `.j` module,
and every module that reinvented query-string building routes through them.

- **`encoding` gains two binary-to-text codecs.** `"uri-percent"` is RFC 3986
  percent-encoding (unreserved set `A-Za-z0-9-._~` literal, space `%20`,
  uppercase hex, strict on a malformed `%` escape); `"uri-form"` is the
  `application/x-www-form-urlencoded` variant (identical but space `+`, so a
  literal `+` encodes `%2B`). Both plug into the existing `toText` / `fromText`
  format table (hand-written, no `net/url`), so they inherit the exact-name,
  catchable-error contract of the rest of the library. Named `uri-*` (not a bare
  `percent` / `form`) so the codec table keeps the genuine binary-to-text
  encodings distinct from the web ones, and to match RFC 3986's own vocabulary -
  percent-encoding is defined by the URI RFC, which standardises on "URI", not
  "URL".
- **`uri` module (`modules/uri.j`).** Pure Jennifer over `strings` + `encoding` +
  `convert` (no Go, no network -> **both binaries**). `parse(raw)` -> `Uri`
  (`scheme` / `user` / `host` / `port` / `path` / `query` / `fragment`, absent =
  ""; IPv6 literal keeps its brackets with the port split out) and `build(u)`
  back (verbatim, no re-encode, so `parse` -> `build` round-trips); `encode` /
  `decode` (`uri-percent`) and `encodeForm` / `decodeForm` (`uri-form`);
  `buildQuery` / `parseQuery` between a `map of string to string` and a query
  string (form-encoded, the query-string convention); and `resolve(base, ref)`
  applying RFC 3986 section 5 relative-reference resolution (with the section
  5.2.4 `remove_dot_segments` algorithm). 100% `uri_test.j` overlay.
- **Consumers de-duplicated.** Seven modules hand-rolled a percent / form
  byte-loop; all now delegate. `influxdb`, `totp`, and `prometheus` use
  `uri.encode` (RFC 3986, their bytes - incl. `%20` for a space - unchanged);
  `rest.queryString`, `gotify.formBody`, `oauth.formBody`, and `telegram.formEncode`
  use `uri.encodeForm` / `uri.buildQuery`. Two encoders are deliberately **not**
  folded in: `s3`'s AWS SigV4 encoder (byte-critical to the signature, a different
  reserved set), and `web`'s request-body `percentDecode` (intentionally *lenient*
  per OM-013 - it must not throw a 500 on a client's malformed `%` escape or
  non-UTF-8 field, whereas the `uri` / `encoding` path is strict).
- **Pre-1.0 break.** `rest`'s query strings now form-encode a space as `+`
  (was `%20`), matching Go `url.Values` / JS `URLSearchParams` / Python
  `urlencode`; both are decoded identically by any conformant server. The
  affected overlay assertions were updated in the same change.
- Deliverables: `docs/modules/uri.md` + index / `SUMMARY.md` rows,
  `examples/modules/uri_demo.j`, a `JENNIFER.md` bullet, the `encoding.md` +
  cheatsheet codec rows, and the four consumers' updated overlays. On stance #1
  (one obvious way): a single percent/form implementation in `encoding`, one URL
  module over it, no per-module reinvention.

## M25 - multiplatform: promote macOS / Windows to supported

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

### M25.1 - Windows: promote to supported

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

### M25.2 - macOS: promote to supported

**Planned.** The parallel case: the same "already ships unsupported, promote it"
shape as `M25.1`, and simpler - macOS lacks even the module-path blocker Windows
has (the POSIX exe-relative default resolves cleanly), and its separators / EOL /
`$HOME` match Linux. A `macos-latest` CI test job running `go test ./...` verifies
correctness (reusing the per-OS osinfo golden strategy from `M25.1`); once green,
move darwin/amd64 + darwin/arm64 out of the `build-unsupported` matrix into the
supported set and drop the "unsupported" labelling. **Requires:** none.

---

## Requirements for 1.0.0 stable

- **Cross-build for macOS / Windows.** The `M25` multiplatform track
  (`M25.1` Windows, `M25.2` macOS) does this; ships as soon as it lands.
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
