# Jennifer - Milestones

Development is split into milestones. Each milestone produces a *working*
interpreter that runs a strictly larger subset of the language.

> **Note on compaction.** Older milestone entries are periodically **compacted**
> (condensed to a short summary, sometimes grouped) to keep this file readable.
> The compacted text records *what* shipped, not the implementation design. For
> the detailed design, rationale, and evolution of any milestone, see the **git
> history** (the commits for that milestone), the authoritative record of how it
> was built.

---

## M1 - End-to-end MVP

Smallest vertical slice that proves the pipeline (source → tokens →
preprocessed tokens → AST → result):

- Types: `int`, `string`
- `def x as int init 5;`, `$var` references, function defs (zero-arg, top
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

## M3 - Functions with parameters and return values

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
- **REPL** - `jennifer repl`, persistent globals/functions/imports across
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
- [user-guide/imports.md > Namespaced libraries and aliasing](user-guide/imports.md#namespaced-calls-and-aliasing) -
  user-facing reference for `use NAME [as ALIAS];` and qualified
  calls.
- [user-guide/style-guide.md > Namespaced calls](user-guide/style-guide.md#namespaced-calls) -
  spacing convention around `.`.
- [technical/grammar.md](technical/grammar.md) - EBNF for
  `qualifiedCall` / `qualifiedConstRef` and the `use ... as ...`
  shape; AST table entries for the new nodes.
- [technical/interpreter.md > Namespaced libraries (M8)](technical/interpreter.md#namespaced-libraries) -
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
  comparator-based sort is deferred until functions are first-class.
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
  non-polymorphic name in core; `len` stays because it spans
  string / list / map.
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
  outside a loop or across a function-call boundary is a positioned
  runtime error. `continue` in C-style `for` still runs the step
  before re-checking the condition (matches C/Go).
- **`repeat { } until (cond);`** post-test loop. New keywords
  `repeat` and `until`; `do { } while ...` considered and rejected
  because the inverted condition is the whole point of switching
  to `until`.
- **`exit;` / `exit EXPR;`** terminate the whole program (exit code 0
  / EXPR-as-int). Distinct from `return` (function-scoped): skips every
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
| M15.7 | `encoding` | Introspection (`isAscii` / `lenBytes` / `lenRunes`), binary-to-text `toText` / `fromText` (`hex` / `base64` / `base64-url`), character `encode` / `decode` / `codecs` (`ascii` / `iso-8859-1` / `windows-1252` / `ebcdic`). The cross-kind UTF-8 pair stays in `convert` (M12); `encoding` owns the codec proliferation. Exact-match codec / format names (the original alias / case-normalisation layer was later dropped, stance #2); Windows-1252's five undefined positions reject symmetrically. The long-tail single-byte codecs shipped later in [M16.15](#m16---io-libraries-and-developer-tooling). |
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
| M16.4 | `testing` (primitives) | The irreducible system side a `.j` test framework needs: `testing.run(name)` invokes a zero-arg user function via `Interpreter.CallByName`, times it, classifies failures into a `testing.Result`; `results`/`reset` (mutex-guarded); `report` (text / TAP / JUnit). The one place `exit` is caught (Go-level, so language `try`/`catch` still can't). |
| M16.5 | interpreter performance pass | Five sub-parts, behaviour unchanged: **.1** shared-marker COW on compound Values (append-in-a-loop O(N^2) -> amortised O(N)); **.2** parse-time lexical slot resolution ((Depth,Slot) coordinates + a `slots` slice; undefined/shadowing promoted to parse-time errors); **.3** pooled + pre-resolved + slot-bound function-call frames; **.4** namespaced-call / comparison / arg-bind / root-cache fast paths; **.5** compile-time constant folding + a `Share()` scalar fast path. Numbers in [tinygo.md](technical/tinygo.md). |
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
| M17.3 | module scope + namespacing | Declarations-only top level (`checkModuleDeclarationsOnly`: only `def const` / `def struct` / `func` / `use` / `import`; scripts keep both). `loadModuleImports` binds each alias (the `as NAME`, or the file stem) into `moduleAliases`, collision-checked against library prefixes. Consumer resolution rides the qualified-reference eval layer: `evalQualifiedCall` / `evalQualifiedConst` dispatch `alias.fn(args)` into the module's own interpreter via `CallByNameWith` (args evaluated in the consumer, body run against the module's globals + functions) and read `alias.CONST`. `use` non-transitivity, run-once sharing, and `-race` safety all follow from the fresh-sub-interpreter-per-module model. |
| M17.4 | exports + visibility | `export` publishes a top-level `func` / `def struct` / `def const`; unmarked names stay private (reaching `mod.helper` is a positioned "not exported" error), and `export` in a `run` script is rejected. `checkReferentialClosure` rejects an exported field / parameter typed as a *private* module struct (library / namespaced types cross freely). **Cross-module struct identity** = boundary translation (`retagStructs`): a module's structs are bare inside it and re-tagged to `(module-stem, name)` as a value crosses out to an importer and back, so `def p as mod.Point`, `mod.Point{...}`, field reads, and pass-back all type-check while `a.Point` / `b.Point` stay distinct; the retag also tags the element-type metadata a `list` / `map` carries (`retagType`). A co-located `MODULE_test.j` overlay (a token splice in `jennifer test`) runs white-box tests against the module's private names. |
| M17.5 | `ansi` module | First module built on the system (pure `.j`, one `use os;` across the boundary; a real dogfood of import / export / resolution). Exports `color` / `bgColor` / `style` (bold / dim / italic / underline / reverse) / `rgb` truecolor / `strip`, plus per-colour and per-style shortcuts (`ansi.red(s)`, `ansi.bold(s)`, ...). The ESC byte is built from a one-byte `bytes`; `strip` uses `regex`; unknown names `throw`. Stateless + TTY-aware: `enabled()` re-reads `NO_COLOR` / `FORCE_COLOR` / `os.isTerminal("stdout")` per call (no toggle state; degrades to always-on when `os.isTerminal` is absent). Colour is a string wrapper, so a `%s|color=` printf modifier was rejected in its favour. |
| M17.6 | `semver` module | Strict [SemVer 2.0.0](https://semver.org) as a second pure-`.j` reference module (`use strings` / `convert` / `regex`, so both binaries), and the base a future `jvc` package manager needs. Exports `Version{major, minor, patch, prerelease, build}` + `parse` (throws on invalid) / `isValid` / `toString`; `compare` / `lt` / `eq` / `gt` (full SemVer precedence: numeric core, prerelease ranks below release, build ignored); `isStable` (`0.y.z` unstable by convention) / `isPrerelease`; `incMajor` / `incMinor` / `incPatch`; and `sort` (own pass over `compare`, since `lists.sort` is scalar-only). Strict - a loose `1.2.3.4` is rejected. `parse` uses the anchored RE2 pattern with named groups; precedence + sort are hand-written (the algorithmic dogfood). Building it surfaced + fixed the M17.4 `retagType` gap (a consumer `list of semver.Version` handed back into a module `list of Version`). Range / constraint matching (`^1.2.0`, `>=1.0.0`, `~1.2.3`) deferred to `jvc`. |


## M18 - Jennifer-coded modules

Built atop the existing system libraries. Forty sub-milestones (with their
nested parts) shipped as pure-Jennifer **modules** under `modules/` (the
directory introduced in M17), not compiled into the interpreter binary, except
where noted as a Go **system library**. Each carries the standard discipline: a
100%-passing `*_test.j` overlay, a `cmd/jennifer/*_test.go` integration test, a `docs/modules/*.md` reference, an
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

- **`net.setReadDeadline` / `net.setWriteDeadline`** - separate read and write
  deadlines for socket timeouts (M18.13, first shipped as a combined `setDeadline`;
  extended to UDP sockets for `ntp`, M18.27; split into independent read/write
  calls so a read timeout cannot block a later write).
- **`io.eprintf`** - the stdout-`printf` twin that writes to stderr (a new
  `Interpreter.Err` / `BuiltinCtx.Err` writer), the stderr sink `log` builds on
  (M18.22).
- **`toml`** and **`httpd`** - two new Go **system libraries** (a char-by-char
  TOML parser and a `net/http` server engine both belong in Go, not a `.j`
  module); M18.8 / M18.9.1.
- **`meta.callMain` / `meta.definedMain`** - resolve a function against the entry
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
| M19.5 | module struct identity: canonical path | Module structs were tagged only by the file **stem**, so two modules sharing a basename (`a/util.j`, `b/util.j`, or two `@scope/package` decks) produced identical `(namespace, name)` identity and a foreign struct passed the other's type check. Identity is now keyed by the module's canonical (resolved) **path**: `Value` and `parser.Type` gained a `ModPath` field that `Equal` / `MatchesDeclared` compare alongside `StructNS` (which stays the stem, for display, so `%v` still reads `benchmark.Point`); the boundary retag + function-parameter stamping thread the path. Two imports of the same file stay one type; different files differ - no import error. |
| M19.6 | `.j` code coverage | `jennifer test --coverage[=text|json]` reports statement coverage by reusing the profiler's per-position hits (no second counting path): a statement profiler runs from top-level init through every test function, and `renderCoverage` intersects those hits with the AST's executable positions (`statementPositions`, mirroring `execStmt`'s recording). Scoped to the tested program's files (an imported-but-only-ran module does not skew it), per-file + total; `text` names the never-executed positions, `json` owns stdout (the human report moves to stderr). The plain `jennifer test` path is unchanged (a nil collector). An HTML view is a later `htmlwriter` consumer. |
| M19.7 | `@scope/package` resolution (vendored decks) | A leading `@` is a vendored-deck reference expanded by one function (`resolveVendor`): the `@` swaps in the vendor root and a reference not ending in `.j` gets the package-named entry appended, so `@jennifer/bitcoin`, `@jennifer/bitcoin/`, and `@jennifer/bitcoin/utils.j` all reduce to a plain absolute path (after which run-once cache + M19.5 path identity are untouched). The entry is `<package>.j`, so `moduleStem` gives the package name and the display namespace / default alias fall out (`import "@jennifer/bitcoin/"` binds `bitcoin.`); two same-package decks across scopes are distinct types, colliding only on the default alias (resolve with `as`). Vendor root via `FindVendorRoot`: `--vendor` > `JENNIFER_VENDOR` > nearest `vendor/` above the program. Path safety: `@` front-only, no `.`/`..`, the file must stay inside the deck; a missing root is a guided error. The `jvc` manager over this is M26. |
| M19.8 | relocation: `jennifer-language` org + vanity path | A one-time mechanical relocation, no language / interpreter behavior change: the repo moves to a `jennifer-language` GitHub org, and (separately) the Go module path moves **off** GitHub to a vanity path `jennifer-lang.dev/jennifer` served by a `go-import` meta page, so module identity is host-independent. Two distinct targets: the **Go module path** -> the vanity domain (`go.mod` + 112 `.go` imports rewritten; the meta page maps it to the org repo, plus a `go-source` tag for pkg.go.dev); **human-facing URLs** -> `github.com/jennifer-language/jennifer` (the `-lang`.dev vs `-language` org spelling is deliberate; the redirect keeps old links working, but canonical in-tree URLs are updated). Metadata / CI / packaging swept until `grep -rn 'mplx/jennifer-lang'` is empty; the first-party deck scope placeholder flips `@mplx/` -> `@jennifer/` (doc-only until `jvc`). |
| M19.9 | audit-driven correctness + hardening | A systematic severity-ordered sweep of ~190 findings from a full bug / performance audit of `internal/` + `modules/`, each fix with a regression test (Go test or `_test.j` overlay). **Crash / safety:** OS-entropy RNG seeding (predictable UUIDs / session ids / passwords fixed; `math.randSeed` stays the deterministic opt-in), `json` / `toml` decode nesting caps, a `try`-body scope fix (a throw-skipped `def` reads undefined, not null), `tengine` recursion guards, `archive` zip-slip + aggregate-decompression caps, a JSON-pointer overflow guard, the two interpreter `-race` races + REPL-vs-spawn table mutation. **Correctness:** lvalue writes re-fetch their root after the RHS, index / append stamp the element type, path-keyed module structs, `barcode` (Code 128 stop / EAN check digit / Code 39 `*` / QR mask 3), `pdfwriter` WinAnsi / Info encoding, a full `toml` conformance pass, `http` / `web` (CSRF / cookie / CORS / ETag) hardening, quote-aware `vcard` / `ical`. **Performance:** the O(N^2) accumulation patterns retired (map hash index, `json` object decode, wire-framing reads in `redis` / `amqp` / `mqtt` / `mikrotik` / `imap` / `websocket`, list-join builders in `csv` / `barcode` / `influxdb` / `jsonl` / `statsd`, GF(256) inline). **Lifecycle:** `os.release` + capped child output, non-blocking `net.eof` + mutex-guarded `net.Conn`, `httpd` admission bounds / must-respond timeout / TLS-1.2 floor / safe unix-socket unlink, per-stream mutex + a `discard` verb on hash / crc / compress streams, `lint` descending into `spawn` / `repeat`. Plus **six coordinated pre-1.0 strictness breaks** (each with tests + operator / scoping docs): `%` is floored (Python; `-7 % 3 == 2`, `7 % -3 == -2`); integer arithmetic overflow is a positioned error, not a silent wrap; a duplicate map-literal key is an error; mixed `int` / `float` comparison is exact (no lossy promotion, so `9007199254740993 == 9007199254740992.0` is `false`); a function may not share a top-level var / const name (no-shadowing both directions); reading a constant with the `$` sigil (`$MAX`) is a parse error. |


## M20 - system libraries

Go **system libraries** (cryptographic primitives, plus formats too heavy or
too reflect-bound for a Jennifer-coded `.j` module - the `json` pattern,
[M16.9](#m16---io-libraries-and-developer-tooling)), plus two cleanup keywords (`defer` / `errdefer`) and
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
| M20.7  | `defer` / `errdefer` | `defer CALL(args);` - single call, args snapshotted, block-scoped LIFO, runs on every exit path, never crosses the function / `spawn` boundary (no `finally` - rejected). `errdefer` (Zig-style) fires only on a propagating error; adopted by the seven connect-then-handshake modules. Paired with the new `fs.sync`. |
| M20.8  | `serial` / `spi` / `iic` / `gpio` | Device I/O over Linux `/dev` + `ioctl` (`x/sys/unix`), build-tag split `linux && !tinygo`: three buses + character-device GPIO (reusing the sysfs [M18.11](#m18---jennifer-coded-modules) module's pin-keyed shape). I2C is `iic` (letters-only). Shared plumbing in `internal/lib/devio`; ioctl struct layouts pinned to the kernel ABI by size assertions. |
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
| M21.13 | Windows installer | An Inno Setup script (`packaging/windows/jennifer.iss`) built by a `windows-latest` CI job into `jennifer-<ver>-setup.exe`: per-user (no admin), adds to `PATH`, bundles the system modules + sets `JENNIFER_SYSMODDIR`, opt-in `.j` association, Apps & Features uninstaller. Unsigned, best-effort **unsupported** build. `scripts/build-windows-installer.sh` recreates it locally via Wine. Promoting Windows to *supported* is the follow-on, `M31.1`. |

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
  is the throughput library, and M21.13 is packaging - which is why this bucket
  is the general catch-all rather than an M18 module run.
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
| M22.2 | digits in identifiers | Relaxed letters-only to **letter-initial** `[A-Za-z][A-Za-z0-9]*` (still no `_`, <= 64); constants `[A-Z][A-Z0-9]*(_[A-Z][A-Z0-9]*)*` (digits within a chunk). **Additive / non-breaking** - so `sha256` / `SHA256` / `HTTP2` / `SCRAM_SHA256` legal, `AES_256` still illegal (write `AES256`). One `IDENT` token, so it applies uniformly (vars / params / functions / struct type **and field** names / `use as` / `import as` aliases); constants the one separately-lexed class. Graduated `DRAFT#20`; unlocks the M22.3 renames. |
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
| M24.2  | first-class functions | Closed the largest expressiveness gap - a function held in a value (type `func`), **immutable, not a pointer** (so the value-semantics stance is untouched); a **bare function name** in expression position *is* the value, a name + `(` is a call (no `&NAME` sigil). New `KindFunc` / `TypeFunc`; a `CallValueExpr` postfix-`(` node calls any function-valued expression (`$f(x)`, `$fns[0](x)`, `makeAdder(1)(2)`). Enables the higher-order `lists` layer (`map` / `filter` / `reduce` / `find` / `any` / `all` / `sortBy`, via `BuiltinCtx.Invoke`). **Deferred:** Tier 2 closures + migrating string-name dispatch. Graduated `DRAFT#18`. |
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

**Done.** Parameter binding deep-copies compound arguments (`list`, `map`,
struct, `bytes`) to uphold value semantics; when the callee never writes the
parameter, a read-only alias to the caller's backing is observationally identical
to a copy, so the parameter is bound by alias. The "borrow" half of
copy-on-write, narrower and more clearly sound than the shared-marker COW that was
tried and reverted as inert (see [technical/rejected.md](technical/rejected.md)):
a borrowed parameter is never written, so it never has to detach. No syntax, no
`.j` changes - a pure interpreter optimization. Mechanism:
[technical/interpreter.md](technical/interpreter.md).

| M#    | Topic | Summary |
| ----- | ----- | ------- |
| M25.1 | borrow where no mutable global exists | `Resolve` runs `markBorrowableParams`: a write-scan of each body sets `Param.Borrow` for a never-written parameter of a borrow-safe type (a flat `list` / `map`, a struct, `bytes`; a `list of list` copies). "Never written" alone is not sound - an argument aliasing a mutable global the body mutates would observe the write - so `bindArg` aliases only in a script with no mutable top-level `def` (`entryGlobalsImmutable`). Removed accidental quadratics: the `markdown` block collectors went linear (a 160-chapter book: **5.1 s -> 0.85 s**). Pinned by `internal/parser/borrow_test.go` and `internal/interpreter/borrow_test.go`. |
| M25.2 | per-function escape analysis | Borrow in scripts that do hold mutable globals, per function: `computeEntryGlobalSafe` scans each function for a local hazard (a non-local write, a func-value call, a module call, a callback builtin from the `jCallbackBuiltins` denylist, an unresolved call; unknown nodes count as hazards) and propagates it along call edges, stamping hazard-free functions `GlobalSafe`. A read-only helper in such a script goes linear. Pinned by `globalsafe_internal_test.go` and `borrow_test.go`. |

**Since revised.** M25.1 also borrowed in modules ("declarations-only, so no
mutable global"), which was unsound: a module method can reach `meta.callMain` and
mutate a host global its argument aliases. Modules now rely on the M25.2 analysis.
M28.19 closed two further holes and records the current gate,
`i.entryGlobalsImmutable || (m.GlobalSafe && !spawnReentry)`, plus argument
snapshots. Nested collections (a `list of list`) still copy; M30.3 plans their
borrow.

---

## M26 - `jvc` package manager (decks)

**Done.** `jvc` (jennifer vendor console) is a standalone project
(`jennifer-language/jvc`, docs at <https://jvc.jennifer-lang.dev/>) with its own
releases. The interpreter's packages carry no copy; only the container image
bundles it.

**Bundled modules and decks.** Niche or product-specific bundled modules
graduate out into decks (archetype: `gotify`); language-fundamental modules stay
bundled. A move changes the import, so it ships **both ways** (bundled + deck,
the bundled copy `@deprecated`) before the bundled copy goes: in any later minor
release before 1.0, at the next major from 1.0 on. The first batch (`gotify`,
`slack`, `discord`, `telegram`) and the metrics clients (`prometheus`,
`statsd`, `influxdb`) leave in 0.28.0 (`M30.1`). *New* service
integrations ship as decks from the start; `M27` collects them.

| Sub | Feature | Notes |
|---|---|---|
| M26.1 | the package manager | jvc `0.1.0`, deck-spec `0.4.1`, written in Jennifer. A `deck.toml` manifest (YAML / JSON accepted); `jvc install` resolves and vendors into the `vendor/` tree the `M19.7` `@scope/package` form reads; `camcorder.lock` pins the result. Decks are sha256-verified `.tar.gz` archives or commit-pinned git sources; `[engines]` / `[conflicts]` checked across the graph; `jvc app install` puts programs on `PATH`; publishing runs a lint / overlay / docblock gate. Client of `registry.jennifer-lang.dev`. Naming conventions live in jvc's docs. |
| M26.2 | distribution | jvc ships its own `.deb`, Arch package, tarball and `jvc app install <url>`; it runs on the default `jennifer` only (needs `http` / `os.run` / `archive`) and depends on the interpreter at its `[engines]` floor, with the `M24.20` version pragma failing a too-old engine at read time. The interpreter side added only the Arch `provides=("jennifer=${pkgver}")`. The `slim` image bundles jvc (fetched at `ARG JVC_REF`, a release tag, staged in its `.deb` layout, checked by `jvc version` at build); `alpine` too, behind an `sh` wrapper (busybox `env` lacks `-S`); `static` does not (no `/usr/bin/env`: vendor in `slim`, copy into `static`). No `git` in the image; git-sourced decks need a derived image. |
| M26.3 | release gate | `release.yml`'s `jvc` job runs jvc's overlays, lint and a launcher smoke test against the `linux/amd64` release tarball; publish requires it. It reads the Dockerfile's `JVC_REF`, so image and gate share one pin. The pin is a jvc release known-green against the RC: an unintended break is a red here, an intended one moves the pin in the same batch (or the release waits). Needs `git` and a writable temp. jvc is the widest integration test: `toml` / `json` / `semver` / `archive` / `http` / `fs` / `os` / `path` / `hash` / `encoding` / `docblock` / `flatdb`, `exec`, and the module system. |
| M26.4 | core stays manager-agnostic | The core reads no manifest or lockfile, so jvc is endorsed, not required (deck-spec `0.4.1` section 5 agrees); a `@scope/deck/` import only resolves under `vendor/` (`M19.7`). The binding engine check is the per-file pragma at every import seam: `version` refuses a too-old engine, `capability` (`net` / `exec` / `sql`) a deck tiny cannot run. `term` / `serial` / `spi` / `i2c` / `gpio` / `crypto` RSA-ECDSA are not capabilities; their tiny stubs fail at first call. An import-time gate for them is deferred until a miss bites. |

---

## M27 - candidate decks (deck ecosystem)

**Planned.** A parking lot of deck ideas - third-party service and integration
clients that ship as **decks** (installed with `jvc`, M26) rather than core
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

**Moved.** The deck sections M27.1 to M27.10 (self-hosted infrastructure,
public / SaaS APIs, daily helpers, bioinformatics, forensic genetics, NGS,
data-format tooling, money, statistical data files, healthcare interop) live
in [decks.md](decks.md) since 2026-11-15, with their IDs unchanged. The
milestones here are about the language, the interpreter and the standard
library; a core change a deck needs is still an M28 entry here.

## M28 - 0.26.0

The 0.26.0 release: everything below is done except the release notes and the
bugs, which the tag waits for. Numbers are stable IDs, not positions. On
2026-11-15 the open M28 entries moved to the release milestones M29 (0.27.0)
and M30 (0.28.0), each keeping its old number as `(was M28.n)`; the former
M29 and M30 are now M31 and M32. A new entry takes the next free number of
the release it is planned for. Retired: M28.35, merged into M30.6 (was
M28.26).

#### M28.2 - non-blocking / timed terminal read

**Done.** `term.readByteTimeout(ms) -> int` returns the byte, `-1` at EOF, or `-2`
when `ms` pass (`0` polls); the blocking `term.readByte` stays the simple path.
Rather than `poll(2)` or termios `VMIN` / `VTIME`, a per-reader pump bounds the
wait over any `io.Reader` (so injected-input tests exercise it): a timed-out read
stays pending and its byte is the next one any read returns, so nothing is lost
and raw mode is untouched. `screen.nextKey` reads the escape-disambiguation byte
with a 50 ms timeout, so a lone `Escape` arrives as `"escape"` and no longer
swallows the next keypress. Stub under `jennifer-tiny`, refused in the REPL like
`readByte`. Not delivered: a pending pump read still holds stdin, so handing the
terminal back to line-mode `io.readLine` mid-program is unchanged; no
`term.pollByte` sugar.

#### M28.3 - append sugar for any place expression

**Done.** `[] =` closes any lvalue chain the indexed / field assignments accept:
`$obj.field[] = v`, `$xs[i][] = v`, `$obj.a[k].b[] = v`. `AppendStmt.Target` is
now the chain (a bare `VarExpr` or an `IndexExpr` / `FieldAccessExpr` rooted on
one); `execAppend` walks it with the same lvalue steps as `execIndexAssign` /
`execFieldAssign` and appends to the list or bytes it reaches in place, so a list
in a struct field fills in amortised O(N) instead of the O(N^2)
`$obj.field = lists.push($obj.field, v)` copy loop. Element-type checked (runtime
and lint L111), const-rejected at the root, `[]` without `=` is still a
write-only parse error. Additive: it only accepts input that was a parse error.

#### M28.9 - static call-site checking in `jennifer lint`

**Done.** `jennifer lint` surfaces the decidable subset of the runtime's call and
binding type errors, emitting **only where a type is certain** so the checks stay
false-positive-free enough for a CI gate (the shipped modules, overlays and
examples lint clean under them).

- **`L109` arity** for same-file functions, an imported module's exported functions,
  and library builtins (optional / variadic counts from the signature table).
- **`L110` argument type** and **`L111` binding type** (`def ... init`, `$x = ...`,
  struct field writes and literal fields, element writes, `$xs[] = ...`), over a
  small static typer (`internal/lint/types.go`): literals, declared bindings
  (for-each iterators included), field / index / slice reads, operators, struct
  and enum literals, library calls, library constants and struct fields. Types
  compare as `MatchesDeclared` does; a literal collection is checked element by
  element; a library parameter's element type only against literal elements
  (builtins check elements one by one).
- **Cross-module.** Lint resolves `import`s as `run` does and reads the
  module's exported surface, translating its types into the importing file's
  terms.
- **Library signatures.** A declarative side table, `internal/libsig` (one
  `name(T1, T2?, T3...) R` line per builtin), never stricter than the builtin and
  held in lock-step with `InstallAll` by a coverage test; the placement is
  recorded in [design-decisions.md](technical/design-decisions.md).

A user function declares no return type by design, so `f(g())` with `g` a user
function stays unknown and is skipped (inferring a return type, or an optional
annotation, is a separate decision).

---

#### M28.11 - `httpd` reads the body before taking an in-flight slot

**Done.** The handler took one of the `maxInFlight` admission slots and then read
the body under a fixed 60 s deadline, so `maxInFlight` connections each sending a
header and trickling the body held every slot and stalled the whole server
(health checks included) for the cost of a few sockets. The body is now read
first, and the slot is taken only once it has arrived. Body memory is bounded by
bytes actually buffered instead: a per-server `maxInFlight x maxBodyBytes` budget,
reserved as each buffer grows (512 bytes, doubling), so a trickling upload holds
about what it has sent; a body that would pass the budget is answered `503` with
`Retry-After`. The body read gets a 10 s per-read idle limit and an overall
timeout, now a per-server `httpd.Options.bodyTimeoutMs` (default 60 s).
**Breaking:** a body that does not arrive in time is answered `408 Request
Timeout` (it was `400`); a request beyond the buffer budget gets `503` instead of
waiting for a slot.

#### M28.12 - `html.escape` escapes quotes

**Done.** The public `html.escape` replaced only `&` `<` `>`, so its obvious use -
a value spliced into `value="..."` by hand - was an attribute injection (a
reflected XSS in a downstream app's search page). It now also escapes `"` and `'`
(`&quot;` / `&#39;`), making it safe in text and in a quoted attribute value.
`render` keeps a private text-only escaper, so rendered text nodes keep their
quotes and output is unchanged. **Breaking:** `html.escape` output for a string
with quotes changes (the same text once unescaped).

#### M28.13 - linear `json` / `toml` / `yaml` list building

**Done.** Every `append` copied the whole list, so building a list of N elements
one append at a time was O(N^2) (20,000 appends took 5.3 s). An end `append` now
grows the list's backing array in place under a one-time atomic tail claim on the
container: only the newest list built by the append path may extend it, every
earlier handle keeps seeing exactly its own elements, and a chain of appends is
amortised O(1) (20,000 in 27 ms). `list(xs)` / `map(m)` also take an optional
Jennifer list / map and convert it in one step, the linear way to build a map.
Additive.

#### M28.14 - `httpd` hardening pass

**Done.** An audit of the engine, each finding confirmed against a running server
and pinned by a regression test:

- A request without a body no longer reserves body buffer, so it is served while
  uploads hold the whole budget (a health check got `503` before); a body with a
  declared `Content-Length` is buffered at exactly that size.
- `serveDir` / `serveDirEtag` answer `404` for dotfiles (`.env`, `.git/`), except
  `.well-known/`, and for a directory without an `index.html`, which Go's file
  server would list; `httpd.Options.dirListing` opts back into listings.
- The engine answers `400` to a decoded path with a `.` or `..` segment
  (`/a%2F..%2Fb`), which a route parameter could otherwise carry into a file path.
- `setHeader` rejects an invalid name or a value with a control character, and
  `etag` a tag with a quote, space or control character, instead of `net/http`
  silently dropping or rewriting them.
- `respond` takes a final status (`200`-`999`); a 1xx produced a broken response.
- `listenTLS` / `listenTLSWith` accept the `unix:` prefix like `listen`.
- `shutdown` closes connections still busy after its 5 s grace period.

**Breaking:** requests with dot segments, dotfiles and unindexed directories under
`serveDir`, invalid headers / tags and 1xx statuses that used to pass now fail.

#### M28.15 - `httpd` follow-ups

**Done.** The smaller findings of the same audit:

- A `unix:` socket is bound inside a private `0700` directory, set to `0660`
  there and hard-linked into place, so it is never connectable under a looser
  mode (with a permissive umask it was `0777` until the chmod after bind).
- `shutdown` closes the listener itself, so the address is free when it returns
  (straight after `listen`, a re-listen on the same address failed).
- `httpd.query` parses the query string once per request, not on every call.
- The response write timeout is a per-server `httpd.Options.writeTimeoutMs`
  (default 5 minutes, which was fixed).
- `serveFileEtag` / `serveDirEtag` hash on the connection's goroutine instead of
  in the program's handler, and concurrent requests for the same uncached file
  share one hash.

Additive.

#### M28.16 - `php2shell`-class request hardening

**Done.** An audit against the PHP `php2shell` bugs (CVE-2026-109618 /
CVE-2026-109619: out-of-memory during request parsing that runs before the
application, leaving state a later request trusts). Go's memory safety rules out
the use-after-free itself; three analogs were real:

- Request headers are capped at 64 KiB (`net/http` allowed 1 MiB), with
  `httpd.Options.maxHeaderBytes` to change it (at most 1 MiB); past it the client
  gets `431`. Headers sit outside the body budget, and a Cookie header parsed
  into pairs grew about 950x in memory, so four header-only requests reached
  3.8 GB.
- `web.serveOn` runs at most `httpd.maxInFlight(srv)` (new) handlers at once.
  The engine frees a slot when it answers `503` after the respond timeout, but
  the worker kept running, so slow handlers under load piled up without limit.
- A `kv.openFile` mutation whose file write fails leaves memory unchanged. It
  changed memory first, so a `session.destroy` that could not be written removed
  the session in memory and resurrected it after a restart.

Additive.

#### M28.19 - borrow soundness and argument snapshots

**Done.** An audit of M25 against adversarial programs found two holes in the
per-function borrow (M25.2) and one older argument bug:

- A later argument could change an earlier, borrowed one: in `f($g, mut())` the
  analysis checked only the callee's body, so `mut()` wrote `$g` in place after
  it was evaluated and `f` read 99 instead of 1. Module and builtin calls had the
  same problem without any borrow, because they evaluate every argument before
  copying any (`m.first($g, mut())` gave 99; `lists.contains($g, mut())` gave
  `true` for a value only `mut()` put there). A load-time pass
  (`stampArgSnapshots`) now marks each argument that aliases a global and is
  followed by an argument that may mutate globals; a marked argument is never
  borrowed and is copied right after evaluation.
- A spawned task that re-enters the host (module call -> `meta.callMain`, a
  callback builtin, a func-value call) writes the live host globals. The
  analysis skipped spawn bodies as snapshot-only, so a borrowed parameter could
  change under its reader, through a spawn in the callee or a task started
  elsewhere and ordered by a channel (1099 instead of 1001). Any spawn in the run
  that can re-enter now turns the `GlobalSafe` widening off for the run.
- The analysis ran only when some parameter was borrowable, which also disabled
  the borrow of local `def`s: a parameterless helper doing `def q ... init $big`
  copied (22.6 s, now 34 ms). It now runs for every program with mutable
  globals, and its fixpoint is a worklist over reverse call edges (a
  10000-function chain loads in 0.12 s instead of 1.5 s).

Additive.

#### M28.22 - in-place int / bool fast path

**Done.** Profiling the two slowest v1 benchmark rows showed no allocation, GC
or scheduler time on either build: the time went into moving `Value` (a ~272-byte
struct) through the evaluator, which returns one from every node and passes two
by value into `evalComparison` / `evalArithmetic`. `intfast.go` reads int
literals, resolved int variables, `list` / `bytes` elements and int arithmetic in
place, answers int comparisons and `and` / `or` / `not` over them, and assigns a
resolved `int` / `bool` binding in place; anything it does not handle exactly
falls back to the general evaluator, so errors keep their message and position.
Two TinyGo-specific costs went with it: a `posFor` interface call per binary /
unary operation, and a range over an always-empty `vars` map on every frame
release. On the reference machine (`--suite all`, `+169` against `+170`)
`primes` went from 17.1 s to 3.6 s (Go) and 14.7 s to 2.7 s (TinyGo), the
per-byte scan from 6.1 s to 0.86 s and 4.1 s to 0.72 s, the v1 serial total from
25.3 s to 6.3 s and 21.2 s to 5.6 s, and the whole run from 34.3 s to 10.2 s and
42.2 s to 13.4 s of wall clock.

Additive.

#### M28.24 - release notes for tagged releases

**Planned.** Every tagged release gets real release notes. Today the
`Compose release notes` step in `.github/workflows/release.yml` writes only the
binary overview, test results, and artifact list, with a "What's new" section
that says there is no per-release changelog and points at this file. Keep the
notes in the repository, one file per release (e.g. `docs/releases/X.Y.Z.md`),
written before the tag: new features, **breaking changes** with migration
notes (pre-1.0 breaks happen at every milestone, so they must be called out
where users look), fixes, and the milestones the release closes. The release
workflow embeds the tag's file in place of the placeholder and fails the
release if it is missing, so a tag cannot ship without notes. `RELEASE.md`'s
"Before tagging" checklist gains the step; the notes are also published on the
docs site.

#### M28.34 - `xml` namespace resolution

**Done.** Shipped as planned, plus one addition found while doing it:
`xml.decode(s[, ns])` takes an optional prefix map that seeds the bindings in
scope at the root, because a fragment cut out of a larger document (the `geo`
deck's GPX / KML scanner decodes one `<trkpt>` at a time) has an undeclared
prefix under the strict rule. `feed` reads Atom in the root element's
namespace (so a default-namespace, a prefixed, and a declaration-less Atom
feed all parse) and Dublin Core by URI. Measured cost: an element grows by 24
bytes and an attribute by 16; a worst-case document of many small elements
goes from 12.6 to 14.9 times its text size. The plan as written:

`xml` kept namespace prefixes verbatim (`kml:Document`, and
`xmlns` declarations are plain attributes), so a program can only match a name
by the prefix one particular document happened to choose. That breaks on real
input: a KML or Atom file that binds its namespace as the **default**
(`xmlns="..."`, no prefix) and one that binds it to `kml:` hold the same
elements under different names, and `feed` reads `dc:creator` literally, so an
RSS feed that binds Dublin Core to another prefix loses its author. Namespaces
in XML 1.0 (third edition) identify a name by **(namespace URI, local name)**;
this milestone makes that pair readable and matchable, without changing what
existing code sees.

- **Resolved at decode, stored per node.** `decode` resolves every element and
  attribute name against the `xmlns` / `xmlns:p` declarations in scope and
  stores the namespace URI beside the name (a string sharing the input, like the
  name). A sub-tree handle from `get` / `findAll` / `children` has no parent, so
  resolution cannot wait until read time. In-scope declarations are kept as a
  shared, immutable chain of scope frames, one allocated only where an element
  declares something, so `lookupPrefix` works on a sub-tree too. Rules from the
  spec: an unprefixed **element** takes the default namespace, an unprefixed
  **attribute** has none; `xmlns=""` undeclares the default; `xml:` is bound
  implicitly to `http://www.w3.org/XML/1998/namespace` and `xmlns:` is
  reserved.
- **Strict, like the rest of `xml`.** A prefix with no declaration in scope, a
  binding of the reserved `xml` / `xmlns` prefixes to another URI, an empty
  `xmlns:p=""`, and two attributes that expand to the same (URI, local name) are
  decode errors with line / column. This is the one break: a document that is
  well-formed XML but not namespace-well-formed decodes today and errors after.
  Pre-1.0, so allowed; called out in the release notes.
- **Read surface (additive).** `xml.localName(node)` (`Document` for
  `kml:Document`), `xml.namespace(node)` (the element's URI, `""` for none),
  `xml.lookupPrefix(node, prefix)` (the URI bound at that node, `""` for the
  default namespace's prefix, an error when unbound), `xml.attrNS(node, uri,
  local)` / `xml.hasAttrNS(node, uri, local)`. `xml.tag`, `xml.attr`,
  `xml.attrs` keep returning the qualified names as written, so no existing
  read changes meaning.
- **Paths: a prefix map, not the document's prefixes.** `xml.get` / `findAll` /
  `has` take an optional `ns as map of string to string`, as XPath / lxml do.
  With it, a `p:name` step matches elements whose URI is `ns["p"]` and local
  name is `name` (an unknown `p` is an error), and an unprefixed step matches
  the URI under the `""` key (no namespace when the key is absent). So
  `xml.findAll($kml, "k:Document/k:Placemark", {"k": KML})` reads both the
  default-namespace and the `kml:`-prefixed file. Without the map, paths keep
  today's verbatim matching. One shortcut for scripts: a `*:name` step matches
  the local name in any namespace (or none), with or without a map.
- **Building and encoding.** `xml.elementNS(uri, qname)` and `xml.setAttrNS(node,
  uri, qname, value)` build namespaced nodes. `encode` keeps decoded prefixes
  and `xmlns` attributes exactly as read (a decode / encode round-trip is
  unchanged), and adds an `xmlns:p` declaration on a built element only when its
  URI is not already bound to that prefix in scope. `xml.element(name)` stays
  the verbatim, namespace-unaware form.
- **Memory.** One URI string per element and attribute plus the scope pointer,
  about 24 bytes an element; the node budget and nesting cap are unchanged.
  Hand-rolled over the existing scanner, so TinyGo-clean.
- **Users.** `feed` matches Dublin Core and Atom by URI instead of by prefix.
  The M30.11 streaming reader exposes the same
  `localName` / `namespace` on its events, so both land with one model.
- **Tests and docs.** A conformance subset of the W3C XML namespaces tests
  (resolution, undeclaring, reserved prefixes, duplicate expanded attributes);
  default-namespace vs prefixed documents read by one `ns`-map path; sub-tree
  `lookupPrefix`; built-element encoding; round-trip unchanged. `xml.md` replaces
  its "namespaces are lexical" note and the `localName` helper recipe with the
  new surface; the cheatsheet, libsig, `JENNIFER.md` and the spec catalog in
  CLAUDE.md follow.

**Requires:** M20.2 (`xml` library). Pairs with M30.11 (the streaming reader).

#### M28.37 - deep values: tiny crash and recursive copy

**Planned.** Found while fixing the formats libraries (2026-11-07; outside any
library). A struct nested about 1000 levels deep (`$n = N{kids: [$n]}` in a
loop) and then copied into a list or map literal (`def m init {"a": $n}`)
crashes `jennifer-tiny` intermittently - `slice out of range`, a nil
dereference, `close of closed channel`; 3 of 8 runs - with no library call
involved. The default binary survives that depth but overflows the Go stack in
the recursive `Value.DeepCopy` at about 100,000 levels. Neither is catchable.

- Reproduce on both binaries from the repro script (one struct type, a build
  loop, a literal copy), under `-race` on the default binary; find the shared
  state tiny corrupts (the closed channel points at a concurrent path, not only
  stack depth) and fix it.
- Bound value depth where values are built: a literal or assignment that would
  nest past `limits.MaxNestingDepth` (1000, 64 on tiny) is a catchable `limit`
  error, as decoding and the format encoders already do; or make `DeepCopy`
  and the other recursive walks (equality, retag, encode) iterative. Measure
  both; the cap is the smaller change.
- Tests: the repro on both binaries, a depth just under and just over the cap,
  and `==` / copy / `json.encode` of a deep value.

#### M28.38 - `term` and `io` share one stdin reader

**Planned.** A timed-out `term.readByteTimeout` leaves a goroutine blocked in
`io.ReadFull` on stdin. Only later `term` reads collect the byte it gets; a
following `io.readLine` / `io.readBytes` reads the same reader concurrently - a
data race, and the next byte goes to whichever read wins. `term.md` now
documents the hazard ("Reading keys"); nothing prevents it.

- One reader layer owns stdin: a single pump goroutine fills a buffer, and
  `term` key reads (with or without timeout) and every `io` stdin read take
  from that buffer, so a timed-out read loses nothing and no two readers touch
  the descriptor.
- `io` keeps its line and byte semantics; the layer is invisible except that
  mixing `term` and `io` reads becomes defined. Raw mode stays `term`'s.
- `jennifer-tiny`: the same layer if TinyGo's scheduler allows a blocking pump,
  otherwise `term`'s timeout is documented as unsupported there.
- Tests: a timed-out `term` read followed by `io.readLine` sees the byte in
  order, under `-race`; EOF and closed stdin reach both readers.

#### M28.40 - `crypto`: one function per operation, block modes, RSA-OAEP

**Done.** Breaking, in 0.26.0. `crypto` named some operations after one fixed
algorithm (`encrypt` was AES-256-GCM with a random nonce, `sign` / `verify` /
`signKeypair` Ed25519) and put the algorithm in the function name elsewhere
(`rsaSign`, `x25519`, `mlkemEncapsulate`), and it could not speak a protocol
someone else defined (Hoymiles' AES-128-GCM with its own nonce and AAD, Midea's
AES-128-ECB and unpadded CBC, SNMPv3's AES-CFB, files written with AES-CBC or
RSA-OAEP). Now there is one function per operation, the algorithm its first
argument, so a new algorithm is a new name:

| Operation | Functions | Algorithms |
| --- | --- | --- |
| encryption | `encrypt(algo, plaintext, key, nonce, aad)` / `decrypt` | `"aes-gcm"` (key 16/24/32, nonce 12, tag appended) |
| block modes, no MAC | `blockEncrypt(algo, data, key, iv)` / `blockDecrypt` | `"aes-ecb-nopad"` / `-pkcs7`, `"aes-cbc-nopad"` / `-pkcs7`, `"aes-ctr"`, `"aes-cfb"` |
| signatures | `sign(algo, message, privateKey)` / `verify(algo, message, signature, publicKey)` | `"ed25519"`, `"rsa-sha256"` / 384 / 512, `"ecdsa-sha256"` / 384 / 512 |
| keys | `keypair(algo)` / `publicKey(algo, privateKey)` | `"ed25519"`, `"x25519"`, `"mlkem768"` (raw bytes), `"rsa2048"` / 3072 / 4096, `"p256"` / `"p384"` / `"p521"` (PEM) |
| key agreement | `agree(algo, privateKey, peerPublic)` | `"x25519"` |
| KEM | `encapsulate(algo, publicKey)` / `decapsulate(algo, ciphertext, privateKey)` | `"mlkem768"` |
| key wrapping | `wrap(algo, plaintext, publicKey)` / `unwrap` | `"rsa-oaep-sha1"`, `"rsa-oaep-sha256"` |

- **Removed:** the two-argument `encrypt` / `decrypt`, `signKeypair`, the
  Ed25519-only `sign` / `verify`, `x25519Keypair` / `x25519` / `x25519Public`,
  `mlkem*`, `rsaSign` / `rsaVerify` / `ecdsaSign` / `ecdsaVerify`,
  `rsaGenerateKey` / `ecGenerateKey`. Unchanged: `randBytes`, `randInt`, `hkdf`,
  `pbkdf2`, `jwkPublic`, `jwkToPem`, `csr`, `mtwei*`.
- **Strict:** an unknown name lists the known ones; wrong key, nonce or IV
  sizes, an IV for ECB, unaligned `-nopad` data, a bad padding and a key of the
  wrong form or type for the algorithm are errors; a failed `decrypt` or
  `unwrap` is one error whatever the cause (no oracle). `publicKey` checks the
  key against the name (a 2048-bit key for `"rsa3072"` is an error). An Ed25519
  signature of the wrong length now verifies false instead of throwing.
- **No encrypt-then-MAC function:** formats MAC different inputs, so
  `crypto.md` gives the recipe (`hash.hmac` over IV and ciphertext, checked with
  `hash.equal` before decrypting); the convenience layer is
  `@deckyard/sealbox` (M27.7, [decks.md](decks.md)).
- **Builds:** `cryptolib_ops.go` holds the operations, AES, Ed25519 and X25519
  (both binaries); RSA, ECDSA, OAEP and ML-KEM helpers stay in the `*_std.go`
  files, and the `*_tiny.go` stubs name the algorithm in their error.
- **Tests:** known-answer vectors from independent implementations: the
  published AES-GCM cases, FIPS-197 / `openssl enc` for every block mode and
  key size, RFC 8032 and RFC 7748, and OpenSSL / Node fixtures
  (`internal/lib/crypto/testdata/kat`) for RSA, ECDSA and OAEP. Go-made RSA and
  ECDSA signatures, OAEP ciphertexts and generated keys were checked by hand
  with OpenSSL and Node in the other direction. A box in the old `encrypt`
  layout opens with the new `decrypt`.
- **Callers:** `jwt` (an algorithm map: `RS256` -> `"rsa-sha256"`, `ES384` ->
  `"ecdsa-sha384"`, `EdDSA` -> `"ed25519"`), `acme`, the examples and demos,
  the registry (forge credentials keep the `nonce || ciphertext || tag`
  layout, and a box made by the 0.25 interpreter still opens, under test), the
  website's crypto example and feature card, and the routeros certificates
  guide. Docs: `crypto.md` rewritten by operation, the cheatsheet, the library
  index, the TinyGo and file-map notes, and `JENNIFER.md`.

#### M28.41 - uniform argument order: the algorithm first

**Done.** Breaking, in 0.26.0. Standard-library functions that take an
algorithm, format or codec name put it in different places (last in
`hash.compute(b, algo)`, in the middle in `compress.pack(b, algo, level?)`,
first in `hash.stream(algo)`). All of them now take the name first, by the
rule in the style guide's "Function signatures" (summarised in
`JENNIFER.md`): a handle first, then the name, then the subject, then the
other parameters, optional ones last. Every caller was ours, so the order was
fixed once, before 1.0.

- **Libraries:**

  | Before | After |
  |---|---|
  | `hash.compute(b, algo)` | `hash.compute(algo, b)` |
  | `hash.hmac(key, message, algo)` | `hash.hmac(algo, message, key)` |
  | `crc.compute(b, algo)` | `crc.compute(algo, b)` |
  | `compress.pack(b, algo, level?)` | `compress.pack(algo, b, level?)` |
  | `compress.unpack(b, algo, maxBytes?)` | `compress.unpack(algo, b, maxBytes?)` |
  | `encoding.toText(b, codec)` / `fromText(s, codec)` | `encoding.toText(codec, b)` / `fromText(codec, s)` |
  | `encoding.encode(s, codec)` / `decode(b, codec)` | `encoding.encode(codec, s)` / `decode(codec, b)` |
  | `convert.bytesFromString(s, codec)` / `stringFromBytes(b, codec)` | `convert.bytesFromString(codec, s)` / `stringFromBytes(codec, b)` |
  | `archive.pack(entries, format)` / `unpack(b, format)` | `archive.pack(format, entries)` / `unpack(format, b)` |
  | `archive.unpackWith(b, format, options)` | `archive.unpackWith(format, b, options)` |
  | `images.encode(img, format, quality?)` | `images.encode(format, img, quality?)` |
  | `crypto.hkdf(secret, salt, info, length, algo)` | `crypto.hkdf(algo, secret, salt, info, length)` |
  | `crypto.pbkdf2(password, salt, iterations, keyLen, algo)` | `crypto.pbkdf2(algo, password, salt, iterations, keyLen)` |
  | `binary.readInt(b, offset, size, order)` (and `readUint`, `readFloat`) | `binary.readInt(order, b, offset, size)` |
  | `binary.readInts(b, offset, count, size, order, stride?)` (and `readFloats`) | `binary.readInts(order, b, offset, count, size, stride?)` |
  | `stats.proportionCi(successes, n, level, method)` | `stats.proportionCi(method, successes, n, level)` |
  | `testing.report(results, format)` | `testing.report(format, results)` |
  | `crypto.rsaSign(pem, message, algo)` (and `ecdsaSign`) | `crypto.rsaSign(algo, message, pem)` |
  | `crypto.rsaVerify(pem, message, signature, algo)` (and `ecdsaVerify`) | `crypto.rsaVerify(algo, message, signature, pem)` |

- **Modules:** `jwt.sign(alg, claims, key)` and every `jwt.verify*` /
  `signWithKid` / `jwksKeys` with `alg` first; `barcode.encode(symbology,
  data, opts)`; `doctree.render(format, doc)`; `feed.build(format, f)`;
  `log.new(format, level)` / `toStderr` / `toFile(format, level, path)`;
  `ldap.password(scheme, plain)`; `sasl.scramStart(algo, user)`;
  `acme.challenge(kind, authz)`; `oauth.jwtAssertion(alg, ...)`;
  `webhook.timestampedSign(algo, encoding, secret, body, timestamp)` /
  `timestampedVerify(algo, encoding, ...)`. A handle stays first:
  `ldap.bindSasl(conn, algo, user, password)`, `label.barcode(label, type, x,
  y, opts, data)`.
- **Stricter where a swap would be silent:** `log.new` / `toStderr` /
  `toFile` reject an unknown format or level and `toSyslog` an unknown level
  (before, anything unknown fell back to text / info), and `sasl.scramStart`
  rejects an unknown algorithm before anything is sent.
- **Removed:** `crypto.hmacEqual`, a duplicate of `hash.equal` (stance #1).
- **Errors:** an old-order call fails with the libraries' ordinary errors (a
  type error, or "unknown codec" where both arguments are strings); no
  order-change hint and no "Changed in" notes before 1.0.
- **Migration:** a one-off script (not committed) moved the arguments in
  `jennifer` (Go tests, modules, examples, docs, docblock examples,
  `JENNIFER.md`) and in 133 files across the website, the apps, the
  `@jennifer/*` decks and the `@deckyard/*` decks, each repo with its own
  `commit.txt` line. `go test ./...`, both builds, fmt, lint, the docblock
  checks, every deck's `check-all.sh` and the app suites pass on it. What it
  got wrong (aliases inherited by test overlays and includes, swaps that are
  not idempotent) is the input to `jennifer fix`, DRAFT#31 in `horizon.md`.
- **Left for the 0.26.0 release:** each deck's two releases from M28.42 (a
  patch release capped at `<0.26.0`, then a minor release from `>=0.26.0`),
  and the apps after the interpreter tag: their CI runs on the released
  interpreter until then. The decks' local `JENNIFER.md` copies were resynced
  at `afd9207`.

#### M28.42 - upper bounds for `engines.jennifer`, enforced

**Done.** A deck states the oldest interpreter it runs on, and once a breaking
release lands (M28.41 in 0.26.0) its author also knows the newest. Before this,
nothing enforced `[engines]`: jvc's resolver ignored it and the version pragma
took a floor only. Now an interpreter range has exactly two forms, `>=X.Y.Z`
and `>=X.Y.Z <A.B.C` (exclusive bound, above the floor), everywhere it is
written, and both ends count. A development build (`dev`, any `-dev` version)
passes every floor and fails every upper bound: it counts as newer than any
release, since `main` already carries the next release's breaks.

- **Interpreter:** `# pragma-jennifer-version: >=0.25.0 <0.26.0` fails at load
  with "requires jennifer >=0.25.0 <0.26.0, but this build is 0.26.0".
  `reqcheck.ParseVersionRange` is shared by the read-time check and lint L303
  (caret, tilde, `||`, `<=` and a bound not above the floor are errors);
  whitespace is not significant and `jennifer fmt` writes the canonical form;
  `version.Below` / `version.Compare` back the bound.
- **jvc:** a new `enginerange` module (`parse`, `admits`, `runsOn`). The
  resolver (`resolveGraphOn` with a `Target{engine, version}`) drops versions
  the local interpreter cannot run, as it drops yanked ones, so the newest
  version that fits both the constraints and the interpreter wins; when none
  does, the error lists each candidate with its range. The interpreter is
  `$JVC_JENNIFER` when set, else the one running jvc; an unknown version skips
  the check with a note. A `camcorder.lock` pin the interpreter cannot run stops
  `jvc install` before anything is fetched ("locked @scope/deck X needs jennifer
  ...; run `jvc update`"). The manifest check, `jvc engine` and `jvc publish`
  accept the two forms only, and publish warns when the local interpreter is
  outside the range.
- **Registry:** `deckadmin --engines` and the forge publish path (`POST
  /publish`) accept the two forms only for `jennifer` / `jennifer-tiny`; it
  already served each version's engines in the deck record, so the HTTP API is
  unchanged. `enginerange.j` and `resolver.j` stay identical between jvc and the
  registry.
- **Deck template:** `deck-lib.sh` reads and validates the range,
  `inject-version.sh` writes it into every pragma, `check-headers.sh` requires
  each pragma to equal it; `AGENTS.md` has the two-release recipe for a breaking
  interpreter release (a capped patch release from the last tag, then a minor
  release from `main`). Synced to all 33 decks; floor-only decks are unchanged.
- **Docs:** the pragma section of the user guide, `JENNIFER.md`, jvc's
  `manifest.md` / `deck-spec.md` / `cli.md`, the registry's `reference/cli.md`.

#### M28.43 - canonical-only base32 / base64 decoding

**Done.** Breaking, in 0.26.0. `encoding.fromText` with `"base32"`,
`"base32-hex"`, `"base64"` and `"base64-url"` skipped CR and LF anywhere and
ignored the unused low bits of the last character (Go's decoders do both), so
many strings decoded to the same bytes: 16 spellings of one Fernet token
opened, and a token used as a replay or revocation key could be respelled
past the check (found reviewing `@deckyard/sealbox`, M27.7 in
[decks.md](decks.md)). Now only the canonical spelling decodes: a line
break, missing or extra `=` padding, or unused bits set in the last character
are errors (a line break names its byte offset), checked by re-encoding the
result. Line-wrapped input (MIME,
PEM) is unwrapped by the caller first, as `mime` already did.
Quoted-printable keeps its documented RFC 2045 leniency; `hex` still accepts
both cases. `encoding.md` and `JENNIFER.md` updated; the Go tests pin the
rejected and accepted forms.

#### M28.46 - read and write deadlines on serial ports

**Done.** `serial.setReadDeadline(port, ms)` / `serial.setWriteDeadline(port,
ms)` arm a deadline `ms` from now (`0` clears it), as `net.setReadDeadline` /
`setWriteDeadline` do; a read or write past it throws a catchable
`Error{kind: "timeout"}` (`serial.read: read timed out`) and the deadline stays
until reset. The port fd was already on Go's poller (opened `O_NONBLOCK`, never
`f.Fd()`), so `os.File` deadlines apply without `VMIN` / `VTIME`. `task.cancel`
now wakes a task blocked in `serial.read` (the deadline moves into the past and
the user's deadline is restored afterwards), and a port closed under a read is
`Error{kind: "closed"}`, matching `net`. Found while writing
`@deckyard/astm`, whose LIS01 timers needed a reader task and a channel per
link because `serial.read` blocked without bound. Pinned by
`internal/lib/serial/seriallib_deadline_test.go` (pty pairs). Stub under
`jennifer-tiny` and off Linux, like the rest of `serial`.

#### M28.47 - TLS options for oauth

**Done.** `oauth.Config` gained `tls` (`http.TlsOptions`): every request made
with a config (token, device, poll, refresh, code, assertion grants) goes out
through `http.requestWithTls` with it, so a self-hosted provider whose
certificate an in-house CA signed works with `caCert` instead of the system's
roots. `discoverTls` / `discoverMetadataTls` fetch the metadata with the
options and return a `Config` carrying them; `discover` / `discoverMetadata`
are the zero-value case. The reply bounds (1 MiB, 15 s per read, 60 s in all)
are unchanged. A `Config` literal after `use oauth;` may leave `tls` out
(zero value: full verification), so `@jennifer/oauthext` and
`@deckyard/fhir` pass their gates unchanged. Pinned by
`testTlsOptionsReachTheTokenEndpoint` (a loopback `httpd.listenTLS` endpoint
signed by a test CA: refused without `caCert`, a token with it, and through
`discoverTls`), which fails when `fetch` drops the options.

#### M28.48 - channel.recvTimeout

**Done.** `channel.recvTimeout(ch, ms)` is `recv` bounded by a deadline: the
next value, or a catchable `Error{kind: "timeout"}` (`channel.recvTimeout:
timed out after N ms`). A timeout takes nothing - a value sent later stays
for the next receive - so no value is lost to a race; `0` polls; a closed
and drained channel throws as `recv`; `task.cancel` wakes it
(`ChannelState.RecvTimeout`, beside `Recv`). It replaces spawning a `recv`
task, `task.waitTimeout`, then `task.cancel` + `task.wait` to rescue a value
that raced the cancel, as `@deckyard/astm`'s link did per byte. Pinned by
`internal/lib/channel/channellib_timeout_test.go` (the race detector is
clean); index-returning `select` (M24.3) stays open.

#### M28.49 - for-each over bytes

**Done.** `for (def b in $data)` walks a `bytes` value, binding each byte as
an `int` (0 to 255); it iterates a snapshot like a list (writes in the body
do not change later iterations), and `break` / `continue` / `return` work
as for a list. It used to fail only at run time ("for-each requires a list
or map, got bytes") while lint passed it, so byte loops took an index. Lint
types the loop variable `int` (L111 flags `def s as string init $b`). A
string stays refused - bytes, code points or graphemes would be a silent
choice - and the message now names what is accepted ("a list, map or
bytes"); the guide points at `strings.chars` / `convert.bytesFromString`.
Pinned by `internal/interpreter/foreach_bytes_test.go` and
`TestBindingTypes/a_bytes_loop_binds_ints`.

#### M28.50 - L104 accepts error-shaped structs

**Done.** `throw` takes any value and the guide calls `Error` a convention,
but L104 warned on every throw that was not `Error` itself, so an error that
carries data (an HTTP status, a vendor code) went into the message text and
was parsed back out (`@deckyard/fhir`'s `statusOf`). L104 now accepts any
struct with `kind` and `message` string fields - of this file, of a loaded
imported module, or, for a module that did not load, a literal that names
both - as a literal, a variable of its type, or the result of a function
that always returns one. A thrown string, a struct missing either field or
with a non-string `kind`, and an enum variant are still flagged. The guide
shows the pattern: dispatch on `kind`, read the extra fields only under a
kind the struct alone uses (`convert.typeOf` says `"struct"` for every
struct). Pinned by eight new `TestThrowNonError` cases.

#### M28.51 - error structs in rest, graphql, jsonrpc, mcp, oauth

**Done.** The modules that wrap a remote service put its status and codes in
the message text only, so a caller parsed them back out. Each now throws one
struct for every error of its kind, so the kinds are unchanged and the fields
are safe to read once the kind matches:

- `rest.HttpError` (`"rest"`): `status`, `headers`, `body` of a non-2xx where
  `getJson` / `paginate` / `paginateCursor` needed JSON.
- `graphql.GraphqlError` (`"graphql"`): `reason` (`graphql` / `http` /
  `json`), `status`, `codes` (each `extensions.code`), `errors` (the array).
- `jsonrpc.RpcError` (`"jsonrpc"`): `reason` (`rpc` / `transport` /
  `protocol`), `status`, `code`, `data`.
- `mcp.McpError` (`"mcp"`, stdio): `reason` (`rpc` / `launch` / `exit` /
  `noreply`), `code`, `data`, `exitCode`; over HTTP the client throws
  `jsonrpc.RpcError`.
- `oauth.OAuthError` (`"oauth"`): `errorCode` (`invalid_grant`,
  `access_denied`, ...), `description`, `status` from a token or device
  endpoint, the device flow and a refused redirect; `""` / 0 otherwise.

Messages are unchanged. `interpreter.ClassifyError` now reads `kind` and
`message` from any such struct, so `testing.assertThrows` / `run` and the test
report see the kind a catch handler sees (they said `"error"` before). The
bundled `telegram` is deprecated (M30.1), so its `error_code` /
`retry_after` go into `@jennifer/telegram` instead.

#### M28.52 - error structs in smtp, ldap, amqp, acme, soap, webdav, mqtt

**Done.** The second group of M28.51, the same way (one struct per module for
every error of its kind, kinds and messages unchanged):

- `smtp.SmtpError`: `code`, `enhancedCode` (RFC 3463, `5.1.1`), `command`,
  `temporary` (4xx).
- `ldap.LdapError`: the result `code`, `matchedDn`, `diagnostic` of a refused
  StartTLS, a failed SASL bind or search (`bind` still returns its `Result`).
- `amqp.AmqpError`: a broker `Close`'s `scope`, `replyCode`, `replyText`,
  `classId` / `methodId`.
- `acme.AcmeError`: `status`, the RFC 7807 `problemType` / `detail` /
  `problem`, and `retryAfter` (seconds).
- `soap.SoapError`: `status`, and `isFault` with the parsed `fault` from `call`.
- `webdav.WebdavError`: `method`, `path`, `status`, and a 207's failed `member`.
- `mqtt.MqttError`: the CONNACK `connackCode` (named in the message) and a
  rejected subscription's `topic`.

The deprecated `influxdb` changed in `@jennifer/influxdb` instead
(`InfluxdbError`: `status`, a 2.x `code`).

Lint typed every catch binding as `Error`, so a handler passing `$e` to a
parameter of a module's struct type got L110; the binding is now untyped, as
the interpreter binds a non-`Error` value, and L104 accepts rethrowing it.
**Compatibility:** a parameter typed `Error` no longer accepts what these
modules throw (`argument 1 must be Error, got struct`), including a
`web.onError` handler declared `func(e as Error)`: for that one, `web` now
hands an error struct over as an `Error` with its kind and message (M28.53). Two module tests had such a
helper (`soap_test`, `webdav_test`).

#### M28.53 - error structs in snmp, mikrotik, imap, redis, s3, pop

**Done.** The third group of M28.51 / M28.52:

- `snmp.SnmpError`: the agent's `errorStatus`, its `statusName`, `errorIndex`.
- `mikrotik.MikrotikError`: the `reply` (`trap` / `fatal`), the trap's
  `category`, the router's `routerMessage`.
- `imap.ImapError`: `status` (`NO` / `BAD`), the RFC 5530 `responseCode`
  (`AUTHENTICATIONFAILED`, `TRYCREATE`, ...), `text`.
- `redis.RedisError`: the error reply's `code` (`WRONGTYPE`, `NOAUTH`, `MOVED`).
- `s3.S3Error` and a new `s3.errorOf(resp)`: the object verbs return the
  response, so `errorOf` reads S3's XML error (`code`, `requestId`, `status`);
  a refused `createMultipartUpload` now throws it.
- `pop.PopError`: `command`, the RFC 2449 / 3206 `responseCode`, `text`. Its
  local guards keep their kind `"pop"` (the others are `"pop3"`), as the tests
  pinned it; both kinds are `PopError`.

The deprecated bundled `telegram`, `influxdb` and `prometheus` got the same
change as their `@jennifer` decks (`TelegramError`, `InfluxdbError`,
`PrometheusError` with `errorType` / `status`), each adapted to its copy, since
the two have drifted. `web.onError` converts an error-shaped struct that
escapes a route into an `Error` (kind and message) before calling the hook,
so a handler typed `func(e as Error)` keeps working; the fields are in the
stderr line. `mikrotik` has no deck yet: the planned
`@jennifer/mikrotik` must start from the module with `MikrotikError`.

#### M28.54 - a reached cap is kind "limit"

**Done.** `net.readAll`'s cap, and `http`'s body, chunked-framing and header
caps, threw a runtime / `"http"` error that callers told from a failed
exchange by matching `exceeds the` / `body exceeds ` in the message (`http`
itself, `oauth`, `@deckyard/fhir`, `jellyfin`, `virtualboxws`). They are kind
`"limit"` now, messages unchanged, and those callers match the kind (the
decks keep the message match as a fallback until their floor rises).

#### M28.55 - generic kinds across the bundled modules

**Done.** An audit of all 93 modules (M28.54's follow-up):

- **Caps are `"limit"`.** 24 received-data caps and processing budgets moved
  from the module's kind: `imap`, `pop`, `redis`, `ldap`, `mqtt` (receive
  caps), `websocket` (handshake, frame, message), `http` (chunk line, 1xx
  responses), `transport.checkReceiveSize` (a negative size stays
  `"transport"`), the `snmp` / `webdav` walks, `font`, `html`, `pdf` and
  `tengine` budgets. Validation of caller input (an IDNA label, a semver
  number, a memcached key, a barcode's capacity, ...) keeps its kind.
- **`"closed"` and `"cancelled"`.** `net.readN`'s "connection closed after N
  of M bytes" is kind `"closed"` (it was a plain runtime error that `imap` and
  `mqtt` matched by text); a cancelled task's "task cancelled" is kind
  `"cancelled"`, message unchanged, so text matchers in decks keep working;
  `oauth` matches the kind.
- **Three more structs:** `ntp.NtpError` (`kissCode`), `memcache.MemcacheError`
  (`reply`, `text`), `zmq.ZmqError` (`peerReason`).
- **Docs:** the control-flow guide has an "Error kinds" table of the generic
  kinds (`runtime`, `value`, `limit`, `timeout`, `closed`, `cancelled`), and
  `JENNIFER.md` lists them.
- **Missed at first** (found auditing the apps): the `markdown`, `myst` and
  `rst` document budgets (depth, nodes, source size, table cells) and `font`'s
  nesting caps were still the module's kind; `font`'s malformed-font wrapper
  turned every `"limit"` into `"font"`, so its budgets never reached a caller
  as `"limit"`; `jsonrpc`, `soap`, `telegram` (and its deck) and `webdav`
  wrapped `http`'s body cap in their own kind. All pass `"limit"` on now.

#### M28.56 - a thrown module struct keeps its identity

**Done.** A value returned by a module crosses the boundary outward with its
module identity (`alias.Struct`), but a thrown one did not: a caught
`a.HttpError` stayed a bare `HttpError`, so it could not be declared as
`a.HttpError` nor passed back to a function of `a` taking it ("struct
"HttpError" is not a.HttpError (a consumer-defined struct cannot impersonate a
module struct)"). Found by `@deckyard/escl`, whose `scanpdf` rethrows an
`EsclError` through a helper. A module call now retags the thrown value like
the returned one (`dispatchModuleMethod`, `14fad43`).
Decks whose floor admits older interpreters rebuild such a struct from its
fields instead.

#### M28.57 - the decks follow the error conventions

**Done.** The audit of M28.55 over the decks:

- **`@jennifer`:** telegram, influxdb and prometheus have their structs (with
  the bundled copies, M28.53); discord, gotify and slack return the response,
  statsd never hears back, so nothing to convert. oauthext's docs name
  `oauth.OAuthError`.
- **`@deckyard`, structs** (kind and messages unchanged, `status` = HTTP
  status): `ForgejoError`, `ProxmoxError`, `VmwareError`, `SynologyError`
  (+ DSM `code`), `VirtualboxError` (`reason`, `code`, `exitCode`),
  `VirtualboxwsError` (`reason`, `code`), `EsclError` (+ `retryAfterMs`),
  `KoofrError` (+ `code`), `SeafileError`, `SteamError`, `TmdbError`
  (+ `statusCode`), `UnraidError` (+ GraphQL `codes`); fhir and jellyfin had
  `HttpError`. Every match on the deck's own message text (`HTTP 404`,
  `HTTP 4xx` retries, `(code 403)`, `VBOX_E_*` regexes, "no answer after")
  reads a field now; `errorCode(e)` in the VirtualBox decks takes the struct.
- **`"limit"` and `"cancelled"`:** reached caps in 20 decks (paging,
  received-data and parsing budgets: astm, hl7, escl, abif, geo, sheets,
  jsonpath, ...) are kind `"limit"`; synology's task wait is `"timeout"`;
  astm, fhir, jellyfin and virtualboxws match a cancelled task by kind. Each
  kind check keeps its old message match as a commented fallback until the
  deck's floor rises.

#### M28.58 - `golang.org/x/crypto`: sealed box, password hashing, SHA-3

**Done.** `crypto` and `hash` take their first dependency outside Go's
standard library, `golang.org/x/crypto` (v0.59.0; `x/sys`, `x/term`,
`x/text` and `x/sync` moved up with it), because crypto is never
hand-rolled. Measured cost: +216 KiB on `jennifer` (+0.8 %), +188 KiB on
`jennifer-tiny` (+1.4 %); everything below builds and runs on both.

- **`crypto.encrypt` / `decrypt`:** `"chacha20-poly1305"` (RFC 8439, nonce
  12) and `"xchacha20-poly1305"` (nonce 24, safe with random nonces).
- **`crypto.wrap` / `unwrap`:** `"nacl-sealedbox"`, libsodium's
  `crypto_box_seal` over raw X25519 keys (low-order public keys refused).
  GitHub's Actions secrets take values in this form, so
  `@deckyard/github` can gain `setSecret` once its floor carries this.
- **Password hashing** (graduated from the horizon idea):
  `crypto.hashPassword(algo, password) -> string` for `"argon2id"` (RFC 9106
  second recommendation, 64 MiB / 3 / 4), `"bcrypt"` (cost 12, over 72
  bytes an error) and `"scrypt"` (N 2^17, r 8, p 1), as PHC strings
  (bcrypt's `$2a$`); `crypto.verifyPassword(password, hash) -> bool`,
  constant-time, reads `$2b$` / `$2y$` and other libraries' PHC hashes, and
  refuses a malformed hash or one over the work caps.
- **Raw KDFs** for formats that fix their parameters (age):
  `crypto.argon2id(password, salt, time, memoryKiB, threads, keyLen)` and
  `crypto.scrypt(password, salt, n, r, p, keyLen)`, capped at 1 GiB of
  memory (Argon2id 16 passes and 64 lanes, scrypt `p` 16).
- **`hash`:** `"sha3-224"` / `"sha3-256"` / `"sha3-384"` / `"sha3-512"`
  (Go standard library) and `"blake2b-256"` / `"blake2b-384"` /
  `"blake2b-512"` / `"blake2s-256"`, in `compute`, `hmac` and `stream`.
- Tests use vectors from other implementations (libsodium via PyNaCl,
  argon2-cffi, Python's bcrypt and hashlib), so they show interoperability.
- **Not taken:** the SSH client (`x/crypto/ssh`, about +620 KiB more) waits
  for its own design; SHAKE and keyed BLAKE2 wait for a user.

#### M28.59 - `http.send` keeps only safe headers across origins

**Done.** A redirect to a different origin dropped only `Authorization`,
`Cookie` and `Proxy-Authorization`, so a key sent in any other header
(`PRIVATE-TOKEN`, `X-API-Key`, `x-goog-api-key`, `api-key`) and a caller-set
`Host` went on to the new host. The hop now keeps an allowlist instead:
`Accept*`, `Cache-Control`, `Content-Language`, `Content-Type`, `If-*`,
`Pragma`, `Range` and `User-Agent`. `allowCrossOriginRedirect` still keeps
everything. Found while writing `@deckyard/gitlab`, which follows its
redirects itself.

#### M28.60 - `soap` passes timeout and closed on

**Done.** `soap.send` rewrapped every transport failure but `"limit"` into
its own kind, so a deadline or a connection closed under the call reached a
caller as a `SoapError`, against the generic-kind rule (M28.55); found while
validating `@deckyard/vies`, whose timeouts arrived without a code. It now
keeps `"timeout"` and `"closed"` (message `soap: POST <endpoint>: ...`, the
endpoint's query left out) and rethrows `"cancelled"` as it is; tested with
a silent server and one that cuts its answer short.

#### M28.65 - transport errors have their own kind

**Done.** `net` threw a refused connection, a reset, a failed lookup and a
failed TLS handshake as kind `"runtime"`, and `http` threw a server closing
before any byte of its answer as `"http"`, the kind of malformed framing, so
no caller could tell "never reached the server, safe to repeat" from "may
have been carried out" without matching messages (the AI decks retried every
kind they did not know).

- **`net.NetError{kind: "net", message, op, reason, sent, replied}`** for
  every socket failure: `op` the step (`connect`, `handshake`, `read`,
  `write`, `accept`, `listen`, `lookup`), `reason` `refused` / `reset` (a
  broken pipe or aborted connection too) / `unreachable` / `dns` / `tls` /
  `closed` / `other`, from the OS error number (the Winsock numbers on
  Windows), `*net.DNSError` and the TLS and x509 error types; `connectTLS`
  names `connect` or `handshake` by where `tls.Dial` stopped. Messages are
  unchanged; `"timeout"` and `"closed"` stay; a malformed address stays
  `"runtime"`. The tiny build has no network stack, so it throws its usual
  error and never a `NetError`.
- **`"net"` is a generic kind.** It is always a `NetError`, so its fields are
  safe to read once the kind matches, and a module that rewraps what it
  catches passes it on unchanged, as it does `"limit"`; the control-flow
  guide's table says so. Lint's L104 accepts throwing a library error struct.
- **`http` adds the exchange facts:** `sent` (the whole request was written)
  and `replied` (some of the answer arrived) on every `NetError` after the
  dial. A close before any byte is a `NetError` of reason `closed`, `sent`
  true, `replied` false, rather than kind `"closed"` plus a field: `"closed"`
  is a plain `Error` raised by `net`, `serial` and many modules, so a field on
  it would not be safe to read after matching the kind, while under `"net"`
  it sits with the reset-before-reply it resembles. A close after part of the
  head, or before a chunked body's last chunk, is now kind `"closed"` (it was
  `"http"`), as a short `Content-Length` body already was; malformed framing
  stays `"http"`.
- **The bundled modules** pass `"net"` on where they rewrapped it: `amqp`
  (a reset was "closed mid-frame"), `jsonrpc` (an `RpcError` "transport"),
  `oauth` ("the request to X failed"), `soap`, `snmp`, `webdav` and
  `telegram` (whose `e as Error` helpers would have refused a `NetError`),
  `ntp`; `mqtt`, `imap` and `zmq` already did. A decoder cap (M28.68) now
  passes as `"limit"` through `jsonrpc`, `mcp`, `oauth`, `soap`, `webdav`,
  `graphql`, `resque`, `session`, `telegram`, `influxdb` and `prometheus`.
  Each is tested against a loopback fake that refuses or resets
  (`net.abort`, M28.71).
- **Decks** that retry on `"runtime"`, or rewrap every kind but the generic
  ones, need `"net"` added: the AI decks (openai, anthropic, gemini), the
  code-host decks (github, gitlab, forgejo, codehost), proxmox, vmware,
  synology, seafile, koofr, unraid, steam, tmdb, docker, frigate, jellyfin,
  fhir, virtualboxws, escl, onvif, vies, hl7, astm, telegram and influxdb.
  A helper taking the caught error `as Error` rejects a `NetError`.

#### M28.68 - document decoder caps throw "limit"; `json.decodeWith`

**Done.** The nesting and node caps of `json`, `toml`, `yaml` and `xml`
(and `asn1`, the `yaml` input-size cap, and the shared write surface's
"result nests deeper" / "more than N nodes") threw kind `"runtime"`, against
M28.54. They are `"limit"` now, messages unchanged: a Go error can carry its
kind (`limits.CapError`, kept through `%w` wrapping by the interpreter's
builtin-error path). `json.decodeWith(text, json.DecodeOptions{maxDepth,
maxNodes})` decodes under lower caps (0 keeps a default; above the default or
negative is an error), since 2M nodes can take some 300 MB from 6 MB of
text (found by the AI decks, which decode untrusted answers).

#### M28.69 - `http.send` honours an HTTP-date `Retry-After`

**Done.** `send` read only delay-seconds. It now also reads an HTTP-date:
the IMF-fixdate and the RFC 850 and asctime forms RFC 9110 has recipients
accept (names case-sensitive; a two-digit year more than 50 years ahead is
the previous century), parsed with `time`. A date in the past waits 0, a
malformed value counts as absent, and the wait is still clamped to 30 s; a
date past 2262, where `time` ends, is past that cap anyway.

#### M28.70 - one body copy less in `http`

**Done.** A `Content-Length` body was appended to the buffer by an
assignment (a copy), then cut to its length even when nothing needed
cutting (a second copy); a chunked body was assigned back after decoding (a
third). Both paths return their result directly now, and the body is cut out
of the raw response once. On a 20 MiB response from a loopback server,
`jennifer profile --allocs` shows the eager copies inside `http` drop from
20.0 MiB to none on both paths (the uncounted cut copy goes too), and peak
RSS of the `Content-Length` download drops from 179 MiB to 159 MiB.
`readToClose` and `readChunked` already join once.

#### M28.71 - `net.abort(conn)`

**Done.** `net.abort` closes a TCP connection with a reset (`SO_LINGER` 0,
then close; a TLS connection at its TCP layer), so a test or a fake server
can produce the failure a crashed peer causes, which the decks could not
test. The tiny build throws its no-network-stack error. Tested with a
loopback server that aborts and a client whose read is a `NetError` of
reason `reset`, through `net` and through `http`.

#### M28.72 - `jennifer-tiny` corrupts struct field names (geo)

**Done.** Running `@deckyard/geo`'s test overlays on `jennifer-tiny`
(the template's `tools/check-tiny.sh`) gave garbage field names (`unknown
field "\xa0\xdfv..." in struct "Document"`) in four KML / KMZ tests, then
TinyGo's "Bad complex descriptor" and a core dump. Not a struct or string
bug: a stack overflow. Under TinyGo, LLVM inlines the single-call-site
handlers into `evalExpr` and `execStmtRaw`, whose frames grow to about
25 KB each, so a recursion with nested loops costs about 130 KB of stack per
Jennifer call. geo's over-nested KML walk passed the fixed 4 MB stack before
`MaxCallDepth`'s 120 units, and TinyGo does not trap an overflow, so it
wrote over heap memory. `jennifer-tiny` now also measures the stack: each
call check reads TinyGo's stack pointer (`stackguard_tiny.go`) and throws
the same catchable `"limit"` "call stack too deep" error once a chain has
used 3 MB; the default binary is unchanged. `examples/deep_recursion.j` (a
heavy runaway recursion, caught, then a struct still intact) runs in
`TestExamplesTinyBinary`; the old binary panics on it. geo on tiny: 363 of
365 pass. The other tiny differences are by design: `json` / `xml` nesting
is capped at 64 and calls at 120 units (`limits/nesting_tiny.go`, sized to
the 4 MB stack), and over-deep `xml` / `json` throws kind `"limit"`
(M28.68); geo's "runtime" kind for an over-nested KML came from the
corrupted run. Its two remaining failures are those caps: geo nests up to
100 and rewraps json's `"limit"` as `"geo"` (a deck fix). Shrinking the
TinyGo frames (`//go:noinline` on the handlers) would allow deeper
recursion; not done.

#### M28.73 - generic kinds in jsonrpc, oauth, amqp, mqtt, imap

**Done.** The last rewraps of M28.55's generic kinds: `jsonrpc` turned
`"timeout"`, `"closed"` and `"cancelled"` into an `RpcError` "transport" and
rebuilt `"limit"`; it rethrows them as `http` threw them now. `oauth`'s 1 MiB
body cap and idle deadline were an `OAuthError`, a reply cut short too: they
pass on as `"limit"`, `"timeout"` and `"closed"`, and its 60 s request
deadline throws `"timeout"` itself (found by `task.poll`, no longer by
matching the message). `amqp` ("closed mid-frame"), `mqtt` ("closed
mid-packet") and `imap` ("closed before the command completed") re-tagged a
peer closing mid-read with their own kind; it is `"closed"` now. Each is
tested against a loopback server that closes early or stalls.

## M29 - 0.27.0

Hardening, platform reach, small libraries, diagnostics and the value model.
Entries keep their former M28 number as `(was M28.n)`.

#### M29.1 - unify resource-exhaustion errors under `kind: "limit"` (was M28.7)

**Planned.** A caught cap breach should tell a caller "your input was too big / too
deep" apart from "something inside me is broken" - so a library walking a
user-supplied tree can re-raise it in its own vocabulary and a deck can keep its
`kind` contract without first imposing its own limit. The `"limit"` kind and the
boundary mechanism already exist (the call-depth cap and `archive.unpack` /
`unpackWith` raise it); this sweep moves the remaining size / depth / count caps -
which today raise a generic `kind: "runtime"` - onto it, each with a regression
test asserting the kind.

Scope, highest-value first:

- **Tier A - input-driven decode / expansion bombs** (the clearest win, untrusted
  input): `json` / `toml` / `yaml` / `xml` nesting caps, the `yaml` alias-bomb node
  budget, `asn1` depth + node-count, `compress` decompressed-size, `intl`
  translation amplification, and the `net.readAll` / `readBytes` / `readN` /
  `recvFrom` byte caps.
- **Tier B - explicit size / allocation caps** (caller asked for too much):
  `binary.make`, `channel.make`, `crypto.randBytes` / `hkdf` / `pbkdf2` (size +
  work), `devio` read, `fs.readBytes`, the `io` printf field cap, `linalg` matrix /
  vector element cap.
- **Tier C - handle-leak guards** (`fs` / `net` / `httpd` / `compress` "too many
  open ...", the `httpd` buffer budget): deliberately **out of scope** - these
  signal a lifecycle leak, not oversized data, so they stay `"runtime"` (revisit if
  a distinct lifecycle kind is ever wanted).

- **Mechanism.** A shared helper so each site is one call, not per-builtin
  boilerplate: a library returns a sentinel/typed cap error (the `archive` `capError`
  pattern) and a common boundary translator raises `interpreter.RaiseError("limit",
  ...)` with the call-site position - or a direct `raiseLimit(ctx, msg)` where the
  builtin has `ctx`. Interpreter-core caps set `runtimeError.Kind = "limit"` directly
  (as the call-depth sites do). Pure error-kind reclassification: no message or
  control-flow change, each paired with a `kind == "limit"` test.

**Requires:** none hard (the `"limit"` kind + `RaiseError` boundary shipped with the
archive / call-depth work). Breaking only for code that catches these and branches on
`kind == "runtime"` (pre-1.0, documented per site).

#### M29.2 - bounded request parsing in Go (was M28.17)

**Planned.** The `.j` request parsers turn small input pieces into single values
(up to 272 bytes each): `multipart.extractParam` scans a part header as a list of
characters, `multipart.parse` has no cap on part count, header size or name
length, `web.parseForm` splits the body on `&` with no pair limit, and
`web.parseCookie` splits the Cookie header on `;`. Measured peaks: a 1 MiB
multipart part header 1.6 GB, three concurrent 3 MB `&&&...` POSTs 5.9 GB. The
form path runs before any route or login check once `web.csrfCheck` is
middleware. Move form, multipart and cookie parsing into Go (`url.ParseQuery`;
`mime/multipart` with header-size, part-count and name-length limits), exposed to
`web` / `multipart` as library calls returning ordinary maps / lists, so parse
memory stays a small multiple of the input. Until then the bound is the 10 MiB
body cap and the 64 KiB header cap.

#### M29.3 - per-task memory budget (was M28.18)

**Planned.** Nothing sets a Go memory limit, and the catchable `"limit"` guard
(`MaxCopyChainBytes`) only counts copies along a recursive call chain, so one
request that builds a huge value runs the process out of memory: a fatal Go
error or an OOM kill that ends every request in flight. Two parts: at startup
set `debug.SetMemoryLimit` from the cgroup / host limit (reusing
`httpdlib_memlimit.go`'s detection) so the GC works harder before the kill; and a
per-goroutine allocation budget for spawn roots, charged on list / map / bytes /
string growth like the copy-chain counter, so a runaway request raises a
catchable `"limit"` error in its own worker instead of taking the server down.
Touches the interpreter's allocation paths, so it needs its own benchmarks.

#### M29.4 - install paths set at build time (was M28.62)

**Planned.** The interpreter assumes an FHS layout: the system module
directory defaults to `/usr/share/jennifer/modules` (`internal/module/
sysmoddir.go`, `compileDefaultSysmoddir`), the docs, man page and syntax
files are packaged under `/usr/share`, and the shebang the docs recommend is
`#!/usr/bin/env -S jennifer run`. A platform whose prefix is elsewhere
(a self-contained userland under an app's data directory, `/opt`,
`$HOME/.local`, a Nix store path) gets a binary that finds no modules
unless `JENNIFER_SYSMODDIR` is set on every run.

- **One build-time prefix.** `make PREFIX=<dir>` (default `/usr`) and the
  release scripts write the default sysmoddir into the generated file
  `scripts/gen-version.sh` already emits (`init()`, not `-ldflags -X`,
  because TinyGo ignores `-X`), as `<PREFIX>/share/jennifer/modules`. The
  comment at `compileDefaultSysmoddir` already promises this override; the
  build does not do it yet. `jennifer version -v` prints the prefix and
  where each path came from (flag, environment, build).
- **Every other built-in path follows it:** the docs directory
  (`JENNIFER.md`, which decks sync from), and any future data path. No Go
  code names `/usr`, `/etc` or `/tmp` directly (an audit; today only the
  sysmoddir does). Temporary files go through `os.tempDir`, which honours
  `$TMPDIR` (Go's `os.TempDir`), and the docs say so.
- **Shebangs:** the docs mention that `/usr/bin/env` is not universal and
  that an absolute interpreter path (`#!<PREFIX>/bin/jennifer run`) works
  wherever `env` does not; `jvc app install` writes launchers with the
  resolved interpreter path rather than `env` when `/usr/bin/env` is
  absent.
- Tests: a build with `PREFIX=/opt/jennifer` resolves its modules there and
  reports it; the default build is byte-identical to today's. The
  packaging (Debian, Arch, Homebrew, OpenWrt, Docker) keeps `/usr` (or its
  own prefix) and passes it explicitly. Additive.

#### M29.5 - DNS without `/etc/resolv.conf` (was M28.63)

**Planned.** Release binaries are built with `CGO_ENABLED=0`, so name
resolution is Go's pure resolver, which reads `/etc/resolv.conf` and
`/etc/hosts`. On a system without them (some minimal containers, and
userlands whose `/etc` lives under another prefix, see M29.4) every
`http`, `net`, `smtp` and deck call fails at the lookup with no hint why.

- **Resolution order:** `JENNIFER_RESOLV_CONF` (a file path) if set, then
  `<PREFIX>/etc/resolv.conf` from M29.4 when it exists, then
  `/etc/resolv.conf`; the same for `hosts`. The chosen files are reported
  by `jennifer version -v`.
- **A clear error when none exists:** kind `"net"` with reason `"dns"`
  (M28.65) and a message naming the files looked for and the variable,
  instead of Go's "no such host" on a loopback default.
- **No silent public fallback.** Go's default of `127.0.0.1:53` /
  `[::1]:53` when no config exists stays; the interpreter never invents a
  public resolver (privacy, and corporate networks).
- `net.resolve(host) -> list of string` and `net.resolveWith(host,
  servers)` for programs that need a lookup against named servers (also
  useful for diagnostics).
- Implemented with a custom `net.Resolver` (`PreferGo`, `Dial` pointing at
  the configured servers) parsed from the chosen file, since Go's own
  config path is not settable. Tested against a loopback DNS fake in `.j`
  (UDP) and a container without `/etc/resolv.conf`. Default binary only
  for the custom resolver; the tiny build keeps the system behaviour.

#### M29.6 - more release architectures (was M28.64)

**Planned.** Releases ship `linux/amd64`, `linux/arm64`, `darwin/amd64`,
`darwin/arm64`, `windows/amd64` and `windows/386` (`release.yml`). Small
boards, older phones and tablets, and 32-bit x86 machines are left out,
though Go and TinyGo target them without cgo.

- **Add** `linux/arm` (GOARM 7, the common 32-bit ARM; GOARM 6 if the
  Raspberry Pi Zero class is wanted), `linux/386`, `linux/riscv64`, and the
  `android/arm64` and `android/amd64` targets (Go's Android targets link
  against Bionic conventions; which build without cgo is checked first,
  `android/arm` and `android/386` need cgo and an NDK and stay out unless
  that changes).
- Each gets both binaries where TinyGo supports the target, the release
  tarball layout, checksums, and a smoke test under QEMU in CI (as `arm64`
  already has): `jennifer version`, the module test subset that needs no
  network, and a DNS lookup against a loopback fake (M29.5).
- 32-bit targets: the interpreter's `int` is 64-bit everywhere (Go
  `int64`), so behaviour is unchanged; the size limits (`MaxInt`-based
  caps, slice lengths) are reviewed for 32-bit `int` in Go's own types.
- The packaging that consumes the tarballs (OpenWrt's `goarch` mapping,
  Debian, Docker's platforms) is extended to the new names.
- Depends on M29.4 for the Android-style prefix and M29.5 for name
  resolution there.

#### M29.7 - `strings` cutset / affix trim (was M28.5)

**Planned.** `trim` / `trimLeft` / `trimRight` strip Unicode whitespace only, so
there is no way to strip a chosen trailing character - writing a fixed-precision
number without trailing zeros, stripping a `\r` from a CRLF line where leading /
trailing spaces are significant, peeling a known prefix. Requested in three deck
reports; each hand-rolled a loop.

- **Add** `strings.trimPrefix(s, prefix)` / `strings.trimSuffix(s, suffix)` (exact
  affixes, the most-used Go pair) and the cutset family `trimSet(s, cutset)` /
  `trimLeftSet` / `trimRightSet` (strip any rune in `cutset`).
- **One way per thing:** the existing `trim*` stay whitespace-only (their canonical
  job); the new names are distinct operations, not an overload - no arity change to
  the current trio (consistent with how `toInt` took a *parameter* rather than
  spawning `parseHex`-style siblings).

**Requires:** none (additive `strings` surface).

#### M29.8 - bulk number reads from `json` (was M28.66)

**Planned.** Reading a JSON array of numbers into a `list of float` costs
one `json.asFloat(v, "/i")` call per element (about 1.4-2.4 us each, found
in `@deckyard/openai` and `@deckyard/gemini`: 3,072-dimension embeddings,
1,000 texts, 3M calls, most of an 8-9 s run). Statistics, metrics and
geodata decks pay the same.

- `json.asFloats(v, ptr) -> list of float` and `json.asInts(v, ptr) ->
  list of int`: the array at `ptr`, every element a number (ints accepted
  by `asFloats`, as `asFloat` does), else a `json` error naming the first
  bad index; `asInts` refuses a non-integral number. One Go call, no
  per-element pointer formatting.
- `json.asStrings(v, ptr) -> list of string` for the same reason (tag
  lists, ids).
- Signatures in `internal/libsig/json.go`, docs, tests (mixed arrays,
  empty arrays, nested pointers, a 1M-element array against the loop).
  Additive; the decks then use it, which is a deck change each.
- Related: M29.11 (a smaller `Value`) decides the memory such a list costs.

#### M29.9 - scope-rule and binder diagnostics (was M28.8)

**Planned.** Three small, independent refinements at the edges of the scope rules.

- **A parameter may shadow a global (decide: forbid, or formally allow).** The
  no-shadowing rule rejects a `def` local, a for-each iterator, a `catch` binding,
  and a `match` binder that collides with a visible global, but a function parameter
  is not checked - `def x ...; func f(x as int) { ... }` is accepted and `$x` inside
  `f` silently means the parameter. The behaviour is well-defined (innermost binding
  wins, and the borrow scan relies on that), so this is an inconsistency, not a
  correctness bug. The decision is which way to make it consistent: **forbid** it (as
  the sibling binders are) or **formally allow** it (a parameter may shadow a global,
  innermost-wins, narrowing the written rule to local-over-local). Evidence leans
  toward allowing - several shipped examples name handler parameters (`token` /
  `user` / `params` / `srv` / `driver` / `data`) that coincidentally match a global,
  so a ban is a pre-1.0 break rejecting a natural, unambiguous pattern. Either way
  the fix is a few lines in `resolveMethod` (the function resolver) plus a one-direction spec wording change,
  so the rule and the implementation agree.
- **A read-only `match` binder reports the wrong rule on mutation.** An enum payload
  binder (`when Circle(c) { ... }`) is read-only - it binds a copy - but is
  registered as an ordinary non-const untyped variable, so an attempt to reassign or
  mutate it is caught late with a message that does not describe why: `$c.r = 99`
  reports "cannot mutate contents of constant" and `$c = ...` reports "cannot assign
  enum to `<invalid>` variable" (the `<invalid>` is the binder's absent declared type
  leaking into the text). Registering it as a constant is wrong - a binder is
  referenced with the `$` sigil like a variable, and the constant machinery would
  demand the bare form - so it needs a distinct **read-only variable** flag on the
  binding (a `slotInfo` bit plus an assign-time check), yielding one accurate message
  ("the match binder `c` is read-only; its payload is a copy"). This adds a third
  binding mode (mutable / `const` / read-only-binder) the resolver and interpreter
  thread consistently.
- **A hoisted function can read a global before its `def` has run.** Top-level
  statements execute in source order and functions are hoisted, so a function called
  above the `def` of a global it references reads the pre-sized zero slot (a `null`)
  rather than the eventual value - and for a `def const` the empty slot is
  misclassified, emitting the wrong hint ("`MAX` is a variable; use `$MAX`"). The
  behaviour is order-dependent by design, but a cleaner outcome is an explicit "used
  before its definition" diagnostic naming the later `def`'s line, which needs a
  per-global "initialised yet?" flag checked on read. The largest of the three and
  the smallest payoff (a rare, order-dependent footgun); recorded mainly so the
  misleading constant hint is not forgotten.

**Requires:** none. The first two are self-contained resolver / interpreter changes
(the first also needs a one-line spec decision); the third is additive init-order
tracking.

---

#### M29.10 - compile-time validation of literal `printf` format strings (was M28.10)

**Planned.** `io.printf` / `sprintf` / `eprintf` parse their format string at
runtime, so a malformed verb or modifier in a string literal fails on the line that
runs rather than when the file is checked: an unknown verb (`%x`), a modifier the
verb does not take (`%d|prec=2`), a bad value (`%d|base=3`), a duplicate key, or a
verb count that does not match the argument count all surface only when that line
executes - possibly on a rarely-taken error path.

When the format argument is a string literal the whole spec is statically known, so
the same `io`/`format.go` parser that runs at call time can run at check time. Two
shippable shapes: a `jennifer lint` rule that re-parses each literal format and
reports a bad verb, modifier, or argument count at its source position (additive,
false-positive-free - a non-literal format is simply not checked); or, stronger, a
parse-time error for a statically-known format whose spec cannot parse (the
"undefined results error" stance applied to a format string). The lint form is the
lower-risk first step and turns a whole class of runtime surprise into a pre-commit
finding.

**Requires:** none hard; builds on the existing `io`/`format.go` spec parser and the
`jennifer lint` framework.

#### M29.11 - a smaller `Value` (was M28.45)

**Planned.** Every value is one `interpreter.Value` struct with a field for
each kind: `Kind`, `Bool`, `Int`, `Float`, `Str`, four slices (`List`, `Map`,
`Bytes`, `Fields`), four strings (`Variant`, `StructName`, `StructNS`,
`ModPath`) and eight pointers (types, task, object, function, channel), about
270 bytes on amd64 whatever it holds. A list element is a `Value`, so a list
of short strings costs roughly 650 bytes per element in practice. Found in the
decks: a 1 MiB HL7 or ASTM transmission of empty fields took 1.1 GB, 65,530
one-field ASTM records 656 MB (each record a struct carrying its delimiters),
a 4.8 MB FHIR bundle about 100 MB once decoded; a `list of float` or `list
of int` built by appending peaks at about 1 KB per element (921,600 floats:
0.9-1 GB RSS, measured 2026-11-15), which is what makes `@deckyard/gemini`'s
`embedMany` of 2,000 x 3,072 dimensions take 2.7 GB; each deck now caps what it
parses (`MAX_FIELDS`, `MAX_RECORDS`, `MAX_SEGMENTS`, a byte cap over search
pages) at well under its input limit, to stay inside a few hundred MB.

Move the rarely used parts out of line: struct identity (`StructName`,
`StructNS`, `ModPath`, `Variant`) behind one pointer to a shared, immutable
type descriptor; the collection element types the same way; the handle kinds
(task, channel, object, function) into one pointer field. That leaves the
kind, one scalar word, a string and one slice inline, about a quarter of the
size, with value semantics unchanged. It touches every constructor and
`Kind` switch, so it needs the benchmark suite before and after, and it sets
the representation `pkg/interpreter` will publish (see `horizon.md`).

#### M29.12 - `internal/runtime`: the value layer below the evaluator (was M28.39)

**Planned.** The libraries are welded to the evaluator: the 102 files in
`internal/lib` that import `internal/interpreter` use about 40 of its
symbols (`Value`, `Null`, `BuiltinCtx` and the constructors far ahead), but
importing them pulls in the whole tree-walker. Move what they use into
`internal/runtime`, with the evaluator above it. This is step 1 of
`DRAFT#1` in [horizon.md](horizon.md): internal only, no public API.

- **Moves to `internal/runtime`:** `Value`, its kinds and constructors,
  equality, deep copy and display (most of `value.go`); errors and
  positions (`RaiseError`, `KindedError`, `ClassifyError`, `ErrorSignal`);
  the builtin registry; task / channel / waker state; `BuiltinCtx`.
- **`Host` interface:** the calls a builtin makes back into the engine
  (`Invoke`, `CallByName`, `Cancel`, `MethodParams`, the call-depth chain)
  go through an interface the evaluator implements. Only callbacks use it;
  `Value` operations stay concrete calls.
- **Types out of the parser:** `Value.ElemTyp` / `KeyTyp` / `ValTyp` are
  `*parser.Type`, and the libraries call `parser.PrimitiveType` 174 times.
  The type descriptors move to a package below both the parser and the
  runtime.
- **Opaque function values:** `Value.Fn` (`*parser.MethodDef`) and
  `Value.FnHome` (`*Interpreter`) become one function reference the engine
  fills in, so the runtime holds no AST.
- **Staged:** type aliases at the old names (`interpreter.Value =
  runtime.Value`, ...) keep every package building while the libraries
  move over, one commit per library group; remove the aliases at the end.
- **Checks:** no behaviour change, so the existing suite is the oracle:
  `go test ./...`, the module overlays, the example goldens and the TinyGo
  goldens. `examples/benchmark.j --suite all` on the reference machine shows
  no regression; a `Host` call on a hot path would show up there.
  `go list -deps ./internal/lib/...` no longer lists
  `internal/interpreter`.

Gains: library tests run without `interpreter.New()`; `DRAFT#17` (a VM) and
compile-to-Go each become another `Host` instead of a change inside the
evaluator.

Not this milestone: the public `pkg/` facade (`DRAFT#1` step 2, after
`Value` is settled), any change to `Value`'s layout or semantics.

**Requires:** M30.3, M30.5, M30.6, M29.14, M30.8, M28.37, which reshape
`Value` and the evaluator; land them first so the move does not conflict
with them.

#### M29.13 - string positions without a rescan on non-ASCII text (was M28.44)

**Planned.** `strings.indexOf(s, sub, from)` and slicing `$s[a..b]` take rune
positions. ASCII text has a fast path (`TestOneByteRunePath`) and a string's
rune count is cached (`strRunes`), but once the text holds one non-ASCII rune,
each call converts rune positions to byte offsets by walking the string from
its start. The usual scanning loop (find the next delimiter from the last
one, slice the piece between) is then quadratic. Found in the decks:
`@deckyard/hl7` decoded escape sequences that way, and a 200 KB field of
`é\H\` took 45 s (40,000 escapes; the same count in ASCII took 1 s);
`@deckyard/astm` took 1.8 s for 10,000. Both now split on the escape
character instead (`strings.split`), which every such loop has to know to
do.

Remember the last rune-to-byte position on the string value (a one-entry
cursor next to `strRunes`, valid while the string is unchanged), so a call at
or after it resumes the walk from there; a forward scan then costs O(n) in
all. Measure with the 40,000-escape repro and a `strings.indexOf` loop over a
mixed-script text; the `strings` and slicing tests cover the semantics.

#### M29.14 - cheaper calls into another module (was M28.28)

**Planned.** A call into another module costs more than a local call: 200,000
calls of a one-line function take 115 ms locally and 169 ms across a module
boundary, and 142 ms against 310 ms with a two-field struct argument. Thin
wrapper modules then cost more than the work they wrap; inlining one made a
FASTQ reader 23% faster. `dispatchModuleMethod` allocates an argument slice per
call, walks each argument for impostor structs (`findImpostorStruct`), retags
struct arguments inward and the result outward (`retagStructs`, which
reallocates a module struct's field array on every crossing even when only its
own identity changes), and binds the parameters from that slice rather than
while evaluating the arguments, as a local call does.

Profile the struct case with pprof first, then: a fast path for a flat module
struct (no nested module struct or declared type naming one) that rewrites the
identity without walking or reallocating more than the struct itself; a
combined single pass for the impostor check and the inward retag; and binding
each argument into the call frame as it is evaluated, as `evalCall` does. Measure with
the 200,000-call repro on both builds; the cross-module tests in
`module_test.go` and `funcvalue_home_test.go` cover the semantics.

Those steps make a crossing cheaper, but a value that *holds* module structs
still costs time proportional to its size on every call: `retagStructs` rebuilds
every struct, list and map on the way to each one, in both directions, whether
the callee reads the value or not. A module function taking its own "handle"
struct (a parsed tree, an index) pays it per call: 200 calls of a one-lookup
`has` on a 6400-node tree take 745 ms against 1 ms for the same code in one file,
and `@deckyard/mitoleaf`'s tree accessors pay about 5.4 ms a call on its
6409-node tree. The structural fix is **one identity on both sides**: resolve a
module's bare struct names to `(ModPath, name)` when the module is loaded, so a
struct built inside the module already carries the identity its importers use,
and type checks on both sides compare that identity. Nothing then needs retagging,
and the per-call walk, the impostor scan's job (a consumer struct can no longer
share the identity) and the outward retag of results all go. It touches struct
construction, `stampModuleStructType` / `resolveDeclaredStructNS`, the
parameter and field type checks, `zeroStructFor` / `zeroEnumFor`, `crossRetag`
and module constants. (A lazy "already retagged" marker does not help: every
crossing flips the identity, so the marker never matches on the next call.)
`jennifer profile --allocs` reports these rebuilds per call site in its
*module-boundary rebuilds* section, and the v4 `module handle call` row of
`examples/benchmark.j` measures them.

## M30 - 0.28.0

The removal of the deprecated bundled modules, copy elision, and streaming
and binary formats. Entries keep their former M28 number as `(was M28.n)`.

#### M30.1 - remove the deprecated bundled modules (0.28.0) (was M28.32)

**Planned, for 0.28.0.** Nine single-vendor clients graduate into decks (see
M26): four notification clients, three metrics clients and two MikroTik
clients. Each deck lives in
the `jennifer-language` organization with the module's surface, so migrating
changes only the import:

| Module | Deck | Import |
|---|---|---|
| `gotify` | [`deck-gotify`](https://github.com/jennifer-language/deck-gotify) | `import "@jennifer/gotify/" as gotify;` |
| `slack` | [`deck-slack`](https://github.com/jennifer-language/deck-slack) | `import "@jennifer/slack/" as slack;` |
| `discord` | [`deck-discord`](https://github.com/jennifer-language/deck-discord) | `import "@jennifer/discord/" as discord;` |
| `telegram` | [`deck-telegram`](https://github.com/jennifer-language/deck-telegram) | `import "@jennifer/telegram/" as telegram;` |
| `prometheus` | [`deck-prometheus`](https://github.com/jennifer-language/deck-prometheus) | `import "@jennifer/prometheus/" as prometheus;` |
| `statsd` | [`deck-statsd`](https://github.com/jennifer-language/deck-statsd) | `import "@jennifer/statsd/" as statsd;` |
| `influxdb` | [`deck-influxdb`](https://github.com/jennifer-language/deck-influxdb) | `import "@jennifer/influxdb/" as influxdb;` |
| `mikrotik` | [`deck-mikrotik`](https://github.com/jennifer-language/deck-mikrotik) | `import "@jennifer/mikrotik/" as mikrotik;` |
| `mactelnet` | [`deck-mactelnet`](https://github.com/jennifer-language/deck-mactelnet) | `import "@jennifer/mactelnet/" as mactelnet;` |

`gotify` is the archetype and the first-party sample deck. All nine are
deprecated from 0.26.0: a module-level `@deprecated` in `modules/<name>.j`, a
notice in `docs/modules/<name>.md` and the catalogs. Each deck carries its
module's `_test.j` overlay and replaces the Go test with `.j` tests against a
loopback `httpd` server (or UDP socket, `statsd`; a fake RouterOS API or
MAC-Telnet peer for the MikroTik pair), as `deck-gotify` does. The decks keep
their modules' floors, `>=0.25.0` for the metrics decks and `deck-mikrotik`,
`>=0.26.0` for `deck-mactelnet`, so they run on the releases that still bundle
the modules. `@deckyard/routeros` imports the bundled `mikrotik`; it moves to
`@jennifer/mikrotik` (a `[decks]` entry) in a release before 0.28.0. All nine
ship through the 0.27 releases and
are removed on the development line right after 0.27.0 is tagged, so 0.28.0 is
the first release without them.

Removal deletes, per module, `modules/<name>.j`, `modules/<name>_test.j`,
`examples/modules/<name>_demo.j` and `docs/modules/<name>.md`, plus the Go
tests `cmd/jennifer/gotify_test.go`, `cmd/jennifer/telegram_test.go`,
`cmd/jennifer/chatnotify_test.go` (`TestSlackWebhook`, `TestDiscordWebhook`),
`cmd/jennifer/prometheus_test.go`, `cmd/jennifer/statsd_test.go`,
`cmd/jennifer/influxdb_test.go`, `cmd/jennifer/mikrotik_test.go` and
`cmd/jennifer/mactelnet_test.go`; drops their rows and entries from
`docs/modules/index.md`, `modules/README.md`, `docs/SUMMARY.md`,
`docs/technical/filemap.md` and `JENNIFER.md`; and rewords the remaining
mentions (`README.md`, `docs/introduction.md`, the MAC-Telnet links in
`docs/libraries/crypto.md` and `docs/libraries/net.md`) to point at the decks.
`http.requestRawBody` / `requestRawBodyTls`, added for `telegram` uploads,
stay, as does `multipart`, which `web` also imports; so do the EC-SRP
primitives `crypto.mtwei*` and `net.bindToDevice`, which `deck-mactelnet`
needs. Breaking: an import of
any of the nine `.j` files fails to resolve from 0.28.0, so the 0.28.0
release notes (M28.24) carry the migration (`deck.toml` + `jvc install` + the
`@jennifer/` import). Allowed before 1.0 (0.x may break in any release); from
1.0 on, a module moving to a deck stays bundled and deprecated until the next
major release, per M26.

#### M30.2 - value-semantics copy elision (return / const read) (was M28.6)

**Planned.** Value semantics deep-copy a compound on every store, which is correct
but compounds: reading 50,000 points into a nested `Document` copies the point list
~7 times (out of the reader, into each literal, into each enclosing list), and a
`def const ... as bytes` probe profiled **11,486 eager copies** of an immutable
value. `GOGC=20` barely moves peak RSS (1.23 -> 0.93 GB), confirming retention from
copying rather than collectable garbage. The read-only-parameter borrow (note 24)
already removes the argument-binding copy; this extends that reasoning to two more
hand-off shapes, keeping value semantics observably intact (a borrow of an
immutable source is identical to a copy).

- **`const` compound read.** A `def x as T init SOME_CONST` copies a deeply-immutable
  value for no semantic reason. Elide it where the target cannot mutate the shared
  backing (the borrow analysis already in place), or mark const-sourced bindings
  copy-on-write.
- **Return-then-store hand-off.** A value returned and immediately stored
  (`def xs init readSegment(...)` then `$segs[] = Segment{points: $xs}`) has one live
  reference throughout; an escape analysis narrow enough to catch `return $local`
  removes most of the seven copies without weakening any guarantee.
- **Document the existing guarantee** the geo report verified as a positive: a large
  `bytes` argument is *not* deep-copied (note-24 borrow) - state it as a contract, not
  an observation.

**Requires:** the note-24 borrow/escape machinery. Pin value-semantics parity with
the `value_alias` stress tests (note 10) for every elision.

#### M30.3 - borrow nested collections (was M28.21)

**Planned.** A parameter or local of a nested collection type (`list of list of
float`, `map of string to list of int`) never borrows, because
`stampDeclaredType` stamps the inner element type into each nested element, which
would write into the caller's shared backing. A matrix helper over a 20000 x 10
matrix called 2000 times takes 41 s copying. Skip re-stamping elements already
carrying the declared type (a read-only check on the shared backing) and allow
the borrow when every element already matches, falling back to copy otherwise.
Needs alias-stress coverage in `value_alias_test.go`.

A deck hit it since: `@deckyard/forensicgenetics` pedigree peeling passed its
messages as a `list of list of float` and copied 2.3 GiB on a 300-generation
chain, which fell to about nothing once the messages were wrapped in a struct (a
struct holding the same nested value is borrowed). The every-element check above
is O(n) per call - cheaper than the copy, but not free, and checking only one
representative element per level would be unsound (elements from `json.decode`
or appends need not carry the same stamp). A cheaper sound form: on the borrow
path `bindArg` stamps only the top-level value - its `ElemTyp` / `KeyTyp` /
`ValTyp` live in the callee's own copy of the `Value` header, so that write is
private - and skips the recursion into the elements, which is the only part that
writes into the caller's backing. A borrowed parameter is never written through,
so the nested stamps exist only to type-check writes it cannot make; a value that
escapes (passed on to a writing callee, stored, returned) is copied and stamped
at that binding as today. Before choosing it, check that nothing on a read path
needs the nested stamps (passing the value on to another borrowed parameter,
`typeOf`-style introspection, encoding an empty inner list). The v4
`nested read param` row of `examples/benchmark.j` measures the gap against its
`wrapped nested read` control.

#### M30.4 - statement results without a `Value` (was M28.23)

**Planned.** After M28.22, statement dispatch is the bulk of a tight loop: in
`primes`, `execStmt` / `execStmts` / `execStmtRaw` / `execBlock` / `execWhile`
hold 75% of the flat time on Go and 52% of the samples on TinyGo. The cause is
the same as M28.22's: every statement returns a `blockResult`, which embeds a
whole `Value` (the `return` value) next to its flow flags and position, so each
statement copies ~320 bytes up through four levels of calls even though almost
none of them return anything. Shrink `blockResult` to the flow flags and
position, and carry a `return` value out of band - on the function's call frame,
or through a pointer the call site passes down - so the common statement returns
a few words. The change is local to `interpreter.go` (15 functions return
`blockResult`; 4 sites read its value) but touches every statement executor, the
`defer` / `errdefer` teardown in `finishFrame`, `try` / `catch`, and `return`
from a `spawn` body. Measure with `examples/benchmark.j` (`primes`, the per-byte
scan, `fib`) on both builds; the existing tests cover the flow semantics.

#### M30.5 - move a parameter into a local on its last use (was M28.25)

**Planned.** The accumulator `func merge(doc as Document, f as Feature) { def out
as Document init $doc; $out.features[] = $f; return $out; }`, called as
`$doc = merge($doc, $f)` in a loop, is O(n^2): the local `def` deep-copies the
whole accumulator on every call. Measured inside a function: 2,000 calls 1.2 s,
8,000 calls 15.3 s, against 7 ms / 33 ms for mutating `$doc` directly (which the
existing `MoveArg` + `ReturnsOwned` path already makes O(1) per call). A GeoJSON
reader written this way took 8.6 s for 2,000 features. `L205` now flags the shape;
this makes it fast instead.

When `def x as T init $p` is the **last use** of parameter `p` in the method body,
move `p`'s value into `x` instead of copying it. Sound only when the frame owns
`p`'s value, so: the resolver treats such a parameter as written (it loses
`Param.Borrow`, so it is bound by copy, or by move when the caller's `MoveArg`
applies), and marks the `def` as a move (`DefineStmt.Move`, like
`VarExpr.Move` / `markMoveableAssign`). `execDefine` then takes the value without
`eagerCopy`. With `$v = merge($v, ...)` on a function local, the argument arrives
by move and the local takes it by move: zero copies per iteration. "Last use"
means no later read of `$p` on any path, the `def` not inside a `try` (the same
restriction `MoveArg` has) and not inside a loop body that could reach it again. Needs alias-stress cases
in `value_alias_test.go` (a moved-from parameter is never read; a borrowed
parameter is never moved) and the benchmark above as a regression check.

#### M30.6 - owned builtin results: no copy at the binding site (was M28.26)

**Planned.** A binding site (`def x init ...`, `$x = ...`) deep-copies its
right-hand side unless it can prove the value private: `rhsFreshLiteral`
(list / map / struct literals, ranges, slices) and `ownedCallResult` (user and
module functions marked `ReturnsOwned`). A builtin call matches neither, so a
result its Go function has just built, and nothing else holds, is copied again:

- `def ch as list of string init strings.chars($s)` on a 100,000-rune string:
  50 calls take 539 ms bound against 201 ms used directly. The same holds for
  `strings.split`, `convert.bytesFromString`, `fs.readBytes`, `maps.keys` and
  most other builtins that build their result; in the Deckyard decks these are
  60-80% of all copies `profile --allocs` reports.
- `binary.readFloats` of 1M values: 30 ms used inline, 92 ms bound with `def`,
  and twice the peak memory (`profile --allocs`: 259 MiB for the list, 287 MiB
  for `binary.split` of 16 MiB into 1M parts).
- Struct results: `def m as regex.Match init regex.find(RE, $s)` deep-copies
  the `Match` (its fields, the `groups` list and the `groupsNamed` map, about
  20 `Value` cells, 5.4 KiB). Over 30,000 matches of a five-group SemVer
  pattern that is 241 ms bound against 130 ms with `regex.find(...).start` in
  expression position, and 155.6 MiB of copies. It is the dominant cost in
  jvc's resolver (309 MiB and about a third of the wall clock over 80 decks x
  25 versions, through `semver.parse`, which binds its match this way).

The fix:

- **Registration:** a builtin opts in with an owned-result flag at
  registration (`RegisterNamespacedOwned` or an option on the existing call),
  mirrored by a marker on its `internal/libsig` line so the two cannot drift.
  `ownedCallResult` also accepts a `QualifiedCallExpr` stamped to such a
  builtin; both binding sites already consult it, so nothing else in the
  evaluator changes.
- **Audit per builtin:** a builtin may be marked only if its result never
  shares backing with an argument or interpreter state. Not marked until
  checked: anything that can return an argument or part of one (`lists.first`
  and `lists.last` hand back an element; a sort or merge may hand back its
  input when there is nothing to do), and getters returning stored values.
  Handle structs (`hash` / `crc` / `compress` streams, `fs.File`, `kv.Store`,
  ...) need no flag: they share state by design and are one small field to
  copy.
- **Candidates:** the measured builtins above; the list / map / bytes builders
  in `strings`, `lists` (`range`, `concat`, `slice`, `map`, `filter`,
  `sortBy`), `binary`, `convert`, `fs`, `encoding`, `compress` and `regex`;
  the `json` / `yaml` / `toml` decoders; and the struct builders: `regex.find`
  / `findAll`, `os.run` / `wait` (`os.Result`), `stats.describe`, `time`
  constructors and arithmetic, `fs.stat` / `walk`, `net.recvFrom`,
  `crypto.signKeypair` / `mlkemEncapsulate`, `archive.unpack` (a list of
  `archive.Entry`).
- **Tests:** each marked builtin gets an aliasing test that mutates the bound
  result and checks the argument (and the reverse), and fails with the copy
  removed, as `binary`'s does. The `strings.chars`, `binary.readFloats` and
  `regex.find` repros become `--allocs` regressions. A debug switch that
  copies anyway and compares is optional.
- **Order:** `binary` first (already audited and tested), then the decoders,
  `convert` and `regex` (jvc's resolver), then the rest by measured benefit
  (`profile --allocs` on the module and deck suites).

Not this milestone: copy-on-write or reference counting for compound values in
general (an M28-sized redesign of value semantics), and the module-boundary
rebuild of deck structs (M29.14). M28.35 was merged into this entry.

#### M30.7 - capture only the locals a `spawn` body reads (was M28.27)

**Planned.** `computeSpawnCaptures` narrows the **globals** a `spawn` snapshot
copies to those the body reads, but the **locals** frame is still copied
whole. A `spawn` inside a function therefore deep-copies every local in scope,
used or not: 200 spawns that use nothing, with a 5,000-entry map local in
scope, take 450 ms. In `@deckyard/ngs`, a worker loop that spawns from the
function holding the job list and the results gathered so far is quadratic
(1,000 jobs 1.42 s, 0.17 s once the spawn moves into a one-parameter helper).

Locals are simpler than globals: a function the body calls cannot see the
caller's locals, so the set is just the variable names the body (and any
nested `spawn`) reads, found by the same name-based walk; no call-graph
fixpoint is needed. `snapshotForSpawn` then copies only those bindings into
the locals frame. A body the walk cannot bound (an unrecognised node) falls
back to the whole frame, as for globals. Pin it in
`spawncapture_internal_test.go` and with a value-semantics test that a local
mutated after launch, used or not, never shows inside the task.

#### M30.8 - borrow through named callbacks (was M28.36)

**Planned.** In a program with a mutable top-level `def`, a parameter is
borrowed only in a function the M25.2 analysis stamps `GlobalSafe`, and a call to
any `jCallbackBuiltins` builtin (`lists.map`, `filter`, `reduce`, `find`, `any`,
`all`, `sortBy`, `meta.call`, `meta.callMain`, the `testing` runners) marks the
caller unsafe outright, as does every function that reaches it. So a function
that ranks with `lists.sortBy(xs, key)` copies every compound argument on every
call even when `key` is a named function that writes nothing and the argument is
a local. With one unused top-level `int` in the file, 200 calls of such a
function over a 2000-element `list of P` copy 726 MiB (1.0 s); without the line,
nothing (0.02 s). Decks hit it: `@deckyard/forensicgenetics` `search` copied its
whole profile database per call (64 MiB at 2000 profiles; on `jennifer-tiny` at
5000 profiles `search` went from 25.5 s to 14.1 s after switching to
`lists.sort` on plain keys), and `@deckyard/mitoleaf` reached `lists.sortBy`
through `lineage.mtHaplotype` and copied its tree per `distance` call (36 ms a
call, 10.5 ms without `sortBy`). Both decks now carry the workaround.

When the func-value argument of a callback builtin is a bare function name (a
`ConstRefExpr` resolving to a `MethodDef`), treat the builtin call as a named
call of that function in `globalHazardScan.qualifiedCall`: the caller is then
`GlobalSafe` exactly when that function is. `meta.call` / `callMain` (dispatch by
string) and func values held in variables, fields or parameters stay fail-closed.
A wider option to weigh alongside: an argument whose root is the caller's own
local (not itself a borrowed parameter) cannot alias a global, so it could be
borrowed whatever the callee reaches; that needs a "may alias a global" bit
carried through borrowed bindings. Pin it in `globalsafe_internal_test.go` and
`borrow_test.go`, including a named key that *does* write a global (must still
copy). The v4 `sortBy callee read` row of `examples/benchmark.j` measures it.

#### M30.9 - streaming HTTP responses and server-sent events (was M28.29)

**Planned.** `http` reads every response whole before returning it, so a
response that arrives over time cannot be consumed as it arrives. That blocks
server-sent events (SSE, `text/event-stream`), which the LLM decks (`openai`,
`anthropic`, `gemini`), Firebase's Realtime Database listeners and MCP's streamable
HTTP transport all stream over, and it also blocks any large download processed
as it comes in. Two layers, both pure `.j` over `net`:

- **`http` streaming responses.** `http.openStream(method, url, headers, body,
  options) -> http.Stream` sends the request, reads the status line and headers,
  and returns with the body unread: `status` and `headers` are on the handle, so
  a caller checks for a non-2xx before reading. `http.readStream(stream) ->
  http.StreamRead{data as bytes, done as bool, stream as Stream}` returns the next
  piece of the body (one chunk under chunked framing, up to a buffer size under
  `Content-Length` or close-delimited framing), and `http.closeStream` ends it
  early. The handle threads its state forward the way `exchange` threads a
  `Session` (`$s = $r.stream;`): the connection, the framing mode, the bytes
  still due, and any body bytes over-read with the headers. Takes the existing
  `http.Options` (TLS, the cap on the total body, and the per-read idle
  timeout, which is what bounds a stalled stream). Redirects are not followed
  (as with `exchange`); a 3xx comes back like any status.
- **An `sse` module** for the event-stream format (WHATWG HTML, "Server-sent
  events"): `sse.open(method, url, headers, body, options) -> sse.Stream` over
  `http.openStream` (sends `Accept: text/event-stream`, rejects another
  content type), `sse.next(stream) -> sse.Read{event as Event, done as bool,
  stream as Stream}`, `sse.close`, and `sse.Event{event, data, id, retry}`. The
  parser follows the spec exactly: `CR`, `LF` and `CRLF` line ends, a leading
  BOM dropped, `:` comment lines (keep-alives) skipped, multi-line `data` joined
  with `\n`, an event dispatched on a blank line, `event` defaulting to
  `"message"`, `retry` taken only when all digits, and the last `id` kept.
  `sse.parse(text) -> list of Event` covers a body already read whole, and
  `sse.format(event) -> string` serializes one for a server to send. A method
  and body are parameters because the LLM APIs stream from a `POST` with a JSON
  body. Reconnection stays with the caller (`sse.lastEventId` gives the value
  for a `Last-Event-ID` header): EventSource-style automatic reconnect only fits
  `GET` streams and would hide failures.

Stopping a stream from another task works by closing it: a read blocked on the
socket fails with `Error{kind: "closed"}`, while `task.cancel` cannot interrupt
a blocked read. Default binary only, like `http`. Serving a stream is a
separate step: `httpd` answers each request with one complete body, so a
streaming `respond` would be an `httpd` addition of its own; `sse.format` is
ready for it. Tests run against a fake server written in `.j` on `net.listen`
that writes chunked event frames with pauses between them, covering a frame
split across reads, a CRLF split between two chunks, keep-alive comments, an
idle-timeout breach and early close. A new module, so it ships with the full
module set: `modules/sse_test.j`, `docs/modules/sse.md`, an index row, the
`SUMMARY.md` entry, `examples/modules/sse_demo.j`, `modules/README.md` and a
`JENNIFER.md` bullet. Additive.

`sse` is a core module rather than an `@jennifer/` deck because core needs
it: the `mcp` module is stateless today ("no SSE, no sessions"), and MCP's
streamable HTTP transport, the follow-up this unblocks, sends server-to-client
messages over SSE. A core module cannot import a deck. Adding that transport
to `mcp` (the client reading a streamed reply, plus the session handling the
transport defines) is a separate step once this lands; `docs/modules/mcp.md`
points here.

#### M30.10 - streaming request bodies (was M28.67)

**Planned.** `http` takes a request body as a whole `string` or `bytes`, so
an upload holds the file in memory, and in practice about three times its
size (the caller's buffer, the framed request, the write). Found in
`@deckyard/gemini`'s Files API upload (a 200 MB file peaked at 609 MB RSS)
and relevant to `webdav`, `s3`, the `download` deck's peers and any
multipart upload.

- `http.requestFromFile(method, url, headers, path, options) ->
  Response`: the body is read from `path` in chunks and written to the
  socket as it goes, with `Content-Length` from the file size (no chunked
  encoding, which many upload endpoints refuse). The file is checked to be
  a regular file and not to change size while sent (an error if it does).
- `http.requestFromParts(method, url, headers, parts, options)` for bodies
  built from pieces (`list of BodyPart{data as bytes, path as string}`),
  which covers multipart uploads without concatenating a file into memory;
  `multipart.buildParts` returns such a list.
- Redirects stay unfollowed for these (a streamed body cannot be replayed
  without reading the file again; documented).
- Pairs with M30.9 (streaming responses), which is the download half.
  Default binary only, like `http`. Tested with a 1 GiB file against a
  loopback `httpd` and a peak-RSS bound in CI.

#### M30.11 - streaming `xml` decode (was M28.4)

**Planned.** `xml.decode` builds a full DOM up front - every element a `KindObject`
wrapping a `{tag, attrs, children}` map, every node / attr / text a 280-byte
interpreter `Value` - so the tree runs ~150x the source and is all live at once: a
10 MB GPX measures ~1.5 GB RSS, and 3 copies do not fit in 4 GB. Reported twice
(the geo deck wrote a ~900-line hand scanner to avoid it); the whole XML family is a
long flat list of small records, the exact shape a pull parser handles in constant
memory. Keep `xml.decode` for small documents; add a streaming reader for large ones.

- **Primitive (pull loop, no callbacks - the M23.1 shape).**
  `xml.reader(bytes) -> xml.Reader` (operates on `bytes`, so a caller that scanned a
  buffer hands it over without a string copy, leaning on the no-copy `bytes`-arg
  guarantee), then `xml.next(reader) -> xml.Event` yielding start / text / end / eof.
  Memory is O(depth) + the source, not O(document).
- **`xml.Event`** is small (`kind`, `tag`, `text`, `depth`); attributes read through
  the existing `xml.attr` / `xml.attrs` accessors (no new vocabulary).
- **Subtree iterator (the ergonomic layer):** `xml.nextElement(reader, name) ->
  xml.Value` yields one small per-element DOM at a time (the "iterparse" shape), so
  `for each trkpt` holds O(one trackpoint), reusing every existing accessor.
- **Local-name matching.** The reader matches `trkpt` / `lat` on a `gpx:`-prefixed
  real-world file (today `xml.attr` *raises* on `gpx:lat`), closing the namespace
  pain the geo report also hit.
- **Handle** is an integer-registry entry (the `net.Conn` / `fs.File` pattern), closed
  with the polymorphic `close`; hand-rolled over the existing position-based scanner,
  so TinyGo-clean. Malformed markup is a positioned catchable error.

**Requires:** M20.2 (`xml` library). Follow-on: the same DOM blowup is shared by
`json` / `toml` / `yaml` (all 280-byte-`Value` trees); a streaming decode there is a
later generalization, not part of this.

#### M30.12 - `cbor` library (was M28.30)

**Planned.** CBOR (RFC 8949) as a system library, designed like `json` /
`toml` / `yaml`: `cbor.decode(bytes) -> cbor.Value` and `cbor.encode(v) ->
bytes` over the shared `internal/lib/doctree` tree, which brings the whole
read / walk / write surface (`typeOf` with `"missing"`, `get` / `has` / `keys`
/ `length` / `as*`, `isNull`, `isDatetime` / `asDatetime`, `map` / `list` +
`set` / `insert` / `append` / `remove` / `move`) addressed by JSON Pointer. A
core library, not a deck: a general, stable IETF format, and the one WebAuthn /
passkeys, COSE (RFC 9052) and CWT are built on, so core auth code may need it.
Go rather than a `.j` module, because a byte-by-byte decoder in `.j` is the
slow path the copy reports measured. Hand-rolled with no dependency (the common
Go CBOR packages are reflection-based), TinyGo-clean, both binaries.

Mapping onto the tree, strict at the boundary:

- **Integer map keys.** The tree's keys are strings, but CBOR maps (COSE keys
  most of all: `{1: 2, 3: -7, -1: 1}`) use integer keys. The tree records a
  key kind per member (string or int, in a parallel slice that only CBOR and
  MessagePack trees allocate, so `json` / `toml` / `yaml` pay nothing); an int
  key is addressed by its decimal pointer token (`"/3"`), `keys` lists it in
  decimal, and `encode` writes it back as an integer. `cbor.keyKinds(v[, ptr])
  -> list of string` reports `"string"` / `"int"` per member, and
  `cbor.setIntKey(v, ptr, key, value)` adds an int-keyed member (the shared
  `set` creates string keys). A map holding both `1` and `"1"`, or a key of
  any other type, is a decode error.
- **Integers** outside int64 (major type 0 above 2^63 - 1, major type 1 below
  -2^63) are an error, as are bignum tags 2 / 3 whose value does not fit int64.
- **Floats:** half, single and double precision decode to `float`; `encode`
  picks the shortest width that keeps the value exactly. A non-finite float
  decodes to a node `typeOf` reports as `float` but `asFloat` rejects, as
  `toml` / `yaml` already do.
- **Byte strings** become the tree's `bytes` kind; text strings must be valid
  UTF-8.
- **Tags:** 0 (RFC 3339 text) and 1 (epoch seconds) become `datetime`; any
  other tag is a decode error in v1, naming the tag number.
- **Simple values:** `false` / `true` / `null` map; `undefined` and the other
  simple values are an error.
- **Indefinite lengths** are decoded; `encode` always writes definite lengths.
- **`encode` is deterministic** (RFC 8949 section 4.2.1 core deterministic
  encoding: shortest forms, map keys sorted by their encoded bytes), which is
  what COSE signs and verifies against, and makes equal values encode to equal
  bytes.
- **Decode limits** like `asn1`'s: a node budget and a nesting cap, and a
  declared length is checked against the remaining input before anything is
  allocated, so a hostile length field is a catchable error, not an
  out-of-memory.

Tested against the RFC 8949 Appendix A examples (both directions) plus
round-trip, deterministic-order and limit cases. A new library, so the usual
set: `docs/libraries/cbor.md`, the library index and cheatsheet rows, an
`internal/libsig/cbor.go` line per builtin, `user-guide/imports.md` and
`technical/interpreter.md` links, the catalog in the spec and a `JENNIFER.md`
bullet. Additive.

#### M30.13 - `msgpack` library (was M28.31)

**Planned.** MessagePack as a system library on the same tree, after `cbor`
(which lands the int-key support it reuses): `msgpack.decode(bytes) ->
msgpack.Value`, `msgpack.encode(v) -> bytes`, the shared surface, plus
`keyKinds` / `setIntKey` as in `cbor`. Used by Neovim's RPC, fluentd and
MessagePack-RPC. A separate library from `cbor`, as `json` / `toml` / `yaml`
are separate: two formats with their own wire rules. The same boundary rules:
`uint 64` above int64 is an error; `float 32` / `float 64` decode to `float`
and `encode` uses `float 32` only when exact; `bin` becomes `bytes` and `str`
must be valid UTF-8; the timestamp extension (type -1, all three widths)
becomes `datetime`, and any other extension type is a decode error naming it;
integer keys as in `cbor`, other key types an error. `encode` picks the
smallest format for each value, and keeps map order as given (MessagePack
defines no canonical order). Same decode limits as `cbor`. Tested against the
spec's format table in both directions plus round-trip and limit cases. The
same new-library set as `cbor`. Additive.

#### M30.14 - `protobuf` wire library (was M28.61)

**Planned.** Protocol Buffers' binary wire format as a system library, the
layer every protobuf user needs and the one that must be fast. Wanted by the
Hoymiles DTU and ESPHome native API (device decks), Prometheus remote write,
binary OTLP, Meshtastic, and the descriptor-driven `protobuf` deck (M27.7 in [decks.md](decks.md));
gRPC is a separate, later question (HTTP/2, see `horizon.md`). Today nothing
reads it: `binary` has no varint, and `jennifer profile` hand-encodes its
pprof output in Go. Go rather than `.j`: a byte-by-byte varint loop in `.j` is
the slow path, and the codec is small. Hand-rolled with no dependency (not
`google.golang.org/protobuf`, which is reflection- and codegen-based and
large), TinyGo-clean, both binaries.

Schema-free, by field number:

- `protobuf.decode(data) -> list of Field` with `Field{number as int, wire as
  string, value as int, data as bytes}`: one entry per field occurrence, in
  wire order (repeated fields and unknown fields stay visible). `wire` is
  `"varint"`, `"fixed64"`, `"bytes"` (length-delimited) or `"fixed32"`; the
  value is in `value` (varint, fixed32/64 raw bits) or `data`. Groups (wire
  types 3 / 4, deprecated since proto2) are an error naming the field.
- `protobuf.encode(fields) -> bytes` writes them back in the given order;
  `decode(encode(f)) == f`.
- Typed readers and makers for the scalar mappings, so callers never
  bit-twiddle: `asSint` (zigzag), `asUint64` (refuses values above int64:
  Jennifer has no unsigned 64-bit), `asBool`, `asFloat` (fixed32 bits),
  `asDouble`, `asString` (valid UTF-8 or an error), `asPacked(field, kind) ->
  list of int` / `list of float` for packed repeated scalars; and
  `varintField(n, v)`, `sintField`, `fixed32Field`, `fixed64Field`,
  `floatField`, `doubleField`, `boolField`, `stringField`, `bytesField`,
  `messageField(n, fields)`, `packedField(n, kind, values)` to build
  `Field`s (one `*Field` suffix, so none is named after a type keyword).
- `protobuf.find(fields, number) -> list of Field` and `first(fields,
  number)` for the usual lookup; a nested message is `decode(field.data)`.
- Varint helpers for formats that frame with them outside protobuf
  (`varintBytes(v) -> bytes`, `readVarint(data, offset) -> VarintRead{value,
  next}`), and length-delimited message streams (`readDelimited(data, offset)`,
  `delimited(message)`), which ESPHome and many socket protocols use.

Strict at the boundary, with decode limits like `cbor`'s: a varint longer
than 10 bytes, a field number of 0 or above 2^29 - 1, a truncated value, and a
length past the end of the input are `"protobuf"` errors; a length is checked
against the remaining input before anything is allocated; a total field-count
budget and the caller's input size bound the work, and exceeding the budget
is kind `"limit"`. Nesting is the caller's (each `decode` is one level), so
there is no recursion to bound here. Errors are one `ProtobufError{kind,
message, offset}` (the byte offset of the failing field) per the error rules.

Tested against byte strings produced by `protoc --encode` for every scalar
type, packed and unpacked repeated fields, negative int32 / int64 (10-byte
varints), zigzag extremes, unknown fields and the limit cases, plus
round-trips. The usual new-library set: `docs/libraries/protobuf.md`, the
library index and cheatsheet rows, an `internal/libsig/protobuf.go` line per
builtin, the catalog in the spec and a `JENNIFER.md` bullet. `jennifer
profile` may then encode its pprof output through the same codec. Additive.

#### M30.15 - `net` Unix domain sockets and file-descriptor passing (was M28.33)

**Planned.** `net` speaks TCP and UDP only; Unix domain sockets are on its
"left out" list, and only `httpd` can listen on one. That keeps every local
service behind a socket file out of reach: the Docker / Podman API
(`/var/run/docker.sock`), D-Bus, systemd's `sd_notify` and socket activation,
`ssh-agent`, and a Wayland compositor. Several of those also hand open file
descriptors across the socket (`SCM_RIGHTS`), which no Jennifer program can
send or receive today. Two layers, both in the `net` library (Go):

- **Unix sockets on the existing handles.** The `unix:` address prefix
  `httpd` already uses selects a stream socket: `net.connect("unix:/path")`
  returns a `net.Conn` and `net.listen("unix:/path")` a `net.Listener`, so
  `readBytes` / `readN` / `readAll` / `writeBytes`, the deadlines, `eof`,
  `accept` and `close` work unchanged. `unix:@name` is the Linux abstract
  namespace. `listen` follows `httpd`'s rules: the socket is created `0660`
  regardless of umask, a stale socket file is cleared only after confirming
  nothing listens on it, and `close` on the listener unlinks the file it
  created. Datagrams use `unixgram:` with the UDP surface: `net.listenUDP`
  binds (`"unixgram:"` alone is an unbound sender), and `net.sendTo` /
  `net.recvFrom` take a `unixgram:` peer, which is what `sd_notify` needs.
  `net.address` returns the path. `net.peerCred($conn) -> net.Cred{pid, uid,
  gid}` reads `SO_PEERCRED` so a server can check who connected.
- **File descriptors.** A new handle `net.Fd{id as int}`, shared between
  copies like `net.Conn`, holds one open descriptor. `net.fdOf(handle)`
  duplicates the descriptor behind an `fs.File`, `net.Conn`, `net.Listener` or
  `net.UDPSocket` (the original stays open). `net.writeWithFds($conn, b, fds as
  list of net.Fd)` sends bytes plus descriptors in one message;
  `net.readWithFds($conn, n) -> net.FdRead{data as bytes, fds as list of
  net.Fd}` receives them, with every received descriptor close-on-exec and
  registered as a handle. `net.fdToFile(fd, mode) -> fs.File`,
  `net.fdToConn(fd) -> net.Conn` and `net.fdToListener(fd) -> net.Listener`
  adopt one into the usual surface, and `net.closeFd` releases it.
  `net.inheritedFd(n) -> net.Fd` adopts descriptor `3 + n` for socket
  activation (checking `LISTEN_PID` against the process). Limits: at most 253
  descriptors per message (the kernel's `SCM_MAX_FD`), an error before
  sending; a message that arrives with more descriptors than fit, or on a
  plain `readBytes`, has them closed by the kernel, never leaked into the
  process; a truncated control message is a catchable error.

Linux first, other Unixes where Go's `syscall` has the same calls; on Windows
`AF_UNIX` stream sockets work but descriptor passing is a catchable error.
`jennifer-tiny` stubs both like the rest of `net`. Needs the `net` capability,
as today. Tests: a `.j` program listens on a socket in a temp dir, a spawned
task connects, and the pair exchanges bytes, a datagram over `unixgram:`, a
temp file's descriptor (read back through `fdToFile`) and a listener's
descriptor (accepted on through `fdToListener`); plus the stale-socket,
permission, `SCM_MAX_FD` and abstract-namespace cases. Docs: a "Unix domain
sockets" and a "Passing file descriptors" section in `docs/libraries/net.md`
(dropping the "left out" line), `internal/libsig/net.go` lines, cheatsheet
rows and a `JENNIFER.md` bullet. Additive.

Out of scope, each a step of its own once this lands: `http` over a Unix
socket (an `http.Options` socket path, for the Docker API), a `dbus` module,
and shared memory (`memfd_create` plus a mapped buffer), which a Wayland
client needs for its pixel buffers on top of descriptor passing.

#### M30.16 - runtime `.j` module loader (`plugin` library) (was M28.1)

**Planned.** Load a `.j` module from a path computed at run time and dispatch into
it, so a plugin needs no `os.run` + stdio bridge - the three current loaders resolve
before execution from a string literal (`use` compiles a library in; `include`
splices tokens in preproc; `import` resolves at `Run()` start), so no
runtime-chosen module can be loaded today. This is a system library over the
existing M17 `loadModule`, not a new keyword: `include` / `import` are compile /
load-time (token splice, parse-time type stamping, pre-execution resolution),
whereas a runtime load is side-effecting and takes the handle-returning shape of
`fs.open` / `kv.open`.

- **Surface.** `plugin.load(path) -> plugin.Module` (resolve + run once, cached);
  `plugin.func(m, name) -> func`; `plugin.call(m, name, args...)`;
  `plugin.const(m, name)`; `plugin.has(m, name)`;
  `plugin.exports(m) -> list of string`.
- **Dispatch.** `plugin.func` returns a `func` value whose `FnHome` runs the body
  in the module's context with argument / return struct retagging; `plugin.call`
  is the by-name wrapper (`meta.call` path).
- **Reuse.** `loadModule` supplies run-once caching, the `@vendor` / search-path
  sandbox (`module.Resolve`), and the M24.20 version / capability pragma gates
  unchanged (a `net` plugin fails to load on `jennifer-tiny`). Run-once, no unload
  or reload.
- **Interpreter deltas.** Expose `loadModule` through `BuiltinCtx`; skip the
  static-import compile-time steps (`bindModuleAlias`, `resolveDeclaredTypesOnce`).
  Add a mutex to the module registry (`cache` / `stack` / `resolved`) - lock-free
  today, correct only because imports load single-threaded before any `spawn`,
  which a runtime load breaks.
- **Type boundary.** A plugin's struct types are unknown at parse time, so the host
  cannot write `def x as plugin.T`. Contract: a shared interface module, imported
  normally, declares the struct types and the plugin implements funcs over them
  (values retag at the boundary); primitives, collections, and opaque handles cross
  without a shared declaration.
- **Trust.** Loading `.j` runs code; no new sandbox (untrusted code stays
  `DRAFT#11`), and path resolution is the `import` sandbox.

**Requires:** M17 (`loadModule`), M19.7 (`@vendor` resolver), M24.20
(version / capability pragma), `FnHome` func values. Open: handle as an integer
registry (`kv.Store` pattern, survives `spawn` / boundaries) vs `KindObject`;
runtime load inside a `spawn` vs main-goroutine-only; a stricter `plugin.load` path
allowlist than `import`.

#### M30.17 - host re-entry from spawned tasks (was M28.20)

**Planned.** A spawned task that calls a module function which calls
`meta.callMain` runs the host function against the live host globals while the
main program keeps running: a data race the race detector reports, with or
without borrow. `hostMu` serializes `callMain` callers against each other (which
is why concurrent `web` handlers are safe, the main program being parked in
`web.serveOn`), but not against the main program. Options: hold `hostMu` while
the main program runs statements whenever host re-entry from a spawn is possible
(serializing it with those tasks), give each such task a host snapshot like a
plain spawn (changing what `callMain` from a task can observe and mutate), or
make it a documented rule that `callMain` from a task is only valid while the
main program is blocked. Needs a decision, then the matching tests under `-race`.

## M31 - multiplatform: promote macOS / Windows to supported

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
green - none blocks a release. Flatpak's sandbox suits desktop apps, not a
CLI that reads host files and runs host commands, and AppImage bundles shared
libraries a static binary does not have; the "unpack anywhere and run" case is
the exe-relative module directory in `M31.1`.

### M31.1 - Windows: promote to supported

**Planned.** The **Windows** track is a handful of concrete gaps, not a rewrite:

- **Exe-relative module directory, on every platform.** `compileDefaultSysmoddir`
  bakes one POSIX path (`/usr/share/jennifer/modules`,
  `internal/module/sysmoddir.go`) into every binary, so the modules a release
  archive ships are found only through `JENNIFER_SYSMODDIR`: an unpacked Linux
  or macOS tarball runs, but `import "name.j";` fails, and on Windows the path is
  meaningless. Add a layer that looks next to the binary, resolved from
  `os.Executable()` through `filepath.EvalSymlinks` (a Homebrew or `/usr/local/bin`
  symlink points into the real prefix):
  - `<exe dir>/share/jennifer/modules`, the release archive layout (`jennifer`
    and `share/` side by side);
  - `<exe dir>/../share/jennifer/modules`, a prefix install (`/usr/local`,
    `/opt/jennifer`, Homebrew).

  Precedence: `--sysmoddir` > `JENNIFER_SYSMODDIR` > exe-relative (the first
  directory that exists) > the compile-time default. The binary's own modules
  come before `/usr/share`, so a newer archive unpacked on a host with an older
  distro package does not load the older modules; for a distro install the
  `../share` candidate is `/usr/share/jennifer/modules` anyway. Report the layer
  as a new `Source` (`exe`) in `meta` and `jennifer version -v`. The `M21.13`
  installer's `JENNIFER_SYSMODDIR` stays the explicit override. `jennifer-tiny`
  takes the same layer where TinyGo provides `os.Executable`, else keeps the
  compile-time default. Tests: a relocated archive (binary and `share/` copied to
  a temp directory) resolves its modules with no env var; a symlinked binary
  resolves through the link; an env var still wins. This makes AppImage
  unnecessary for the "unpack anywhere" case.
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
  is verified (the exec suite self-skips off Linux; the osinfo golden is
  the known failure the item above resolves); once green, move windows/amd64 out of
  the `build-unsupported` matrix into the supported set and drop the "unsupported"
  labelling for that arch.

**Requires:** none.

### M31.2 - macOS: promote to supported

**Planned.** The parallel case: the same "already ships unsupported, promote it"
shape as `M31.1`, and simpler: the exe-relative module directory from `M31.1`
covers the tarball and Homebrew layouts, and its separators / EOL / `$HOME`
match Linux. A `macos-latest` CI test job running `go test ./...` verifies
correctness (reusing the per-OS osinfo golden strategy from `M31.1`); once green,
move darwin/amd64 + darwin/arm64 out of the `build-unsupported` matrix into the
supported set and drop the "unsupported" labelling. **Requires:** none.

---

## M32 - project governance, licensing, and contribution policy

**Planned. A hard requirement for 1.0.0 stable** (also listed under
[Requirements for 1.0.0 stable](#requirements-for-100-stable)). The rules for
*how the project is run and how outside contributions are taken* - organizational,
not code. Untouched while the project is solo (one author, `Copyright (C) 2026
mplx <mplx@jennifer-lang.dev>`, `LGPL-3.0-only`, no outside PRs), but it must be settled
**before the first external contribution is merged**: several of the choices are
hard to reverse once other people's copyrightable work is in the tree. The open
questions, roughly by urgency:

- **Copyright-holder model.** Under distributed copyright (the default, no
  paperwork) every non-trivial contributor automatically holds copyright in their
  patch, so the tree becomes a mosaic of holders and any future relicensing needs
  each one's agreement. The alternatives are a **CLA** (contributor grants the
  project a broad license, keeps their own copyright) or an **assignment / CAA**
  (contributor transfers copyright to a single holder) - both consolidate the
  rights but add contributor friction, and assignment needs an entity to hold
  them. This is the decision that is expensive to undo.
- **The copyright *notice*.** Whether headers stay per-author (`(C) <name>`) or
  move to a collective label (`(C) The Jennifer Authors`, defined by git
  history). The trap to avoid: a two-file `AUTHORS` (holders) / `CONTRIBUTORS`
  (credit) split only carries information when a **work-for-hire** contributor
  exists (employer holds copyright, individual is merely credited); for an
  all-volunteer project the two lists are identical, so the split is pointless.
  Either keep no enumerated holder file (the collective label refers to git
  history) or consolidate ownership via CLA / assignment.
- **Relicensing headroom.** LGPL already lets anyone embed / link Jennifer
  without permission, so ordinary use never needs a contributor's sign-off. The
  only thing distributed copyright forecloses is issuing a *different* license -
  e.g. a commercial embedding exception for a deep-embedded `jennifer-tiny`
  target that cannot meet LGPL's static-relink terms. If keeping that option open
  matters (embedding is a first-class goal), a CLA is the tool; if "LGPL-only
  forever" is acceptable, distributed copyright is fine and the constraint never
  bites.
- **Contribution mechanics.** `CONTRIBUTING.md`, the sign-off mechanism (a
  lightweight **DCO** `Signed-off-by` line, which asserts "I have the right to
  submit this" without a license grant, vs a full CLA-bot, which also grants
  one - the choice follows from the relicensing decision above), a code of
  conduct, and the PR / review workflow.
- **Project governance.** Who decides (BDFL vs a maintainer group), how commit
  rights are granted (judgment and sustained involvement, never an LOC or
  commit-count threshold - metrics are a bad proxy and get gamed), and a
  `MAINTAINERS` file once more than one decision-maker exists. Being listed as a
  contributor confers no authority; credit and governance are separate.
- **Name / mark.** Whether the "Jennifer" / `jennifer-lang` identity needs any
  trademark-style usage policy (forks, the deck registry) or stays informal.

The license itself stays **`LGPL-3.0-only`** unless a deliberate relicensing
decision above changes it; this milestone is about the *process and ownership*
around it, not a license change. Not legal advice - the chosen model should get a
real legal review before it is published.

**Requires:** none (organizational, independent of the codebase). Socially paired
with the M19.8 org move and triggered by the first external contribution, but no
code prerequisite - which is why it is the one 1.0.0 requirement that can be
settled at any time before the first outside PR.

---

## Requirements for 1.0.0 stable

- **Project governance, licensing, and contribution policy** - the `M32`
  work: the copyright-holder model, copyright notice, relicensing headroom,
  `CONTRIBUTING.md` / sign-off, governance, and name / mark. A **hard
  requirement** for 1.0.0, and it must be settled before the first external
  contribution is merged (whichever comes first).
- **Cross-build for macOS / Windows.** The `M31` multiplatform track
  (`M31.1` Windows, `M31.2` macOS) does this; ships as soon as it lands.
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

Ideas *beyond* 1.0.0 - embedding, a WASM runtime, specialised-domain
libraries, and smaller possibilities - live in the
[beyond-1.0.0 idea collection](horizon.md), so this file stays focused on the
road to 1.0.0.
