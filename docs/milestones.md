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
  moved here from `strings`. See [libraries/core.md](libraries/core.md)
  and [technical/cli.md > Version injection](technical/cli.md#version-injection).
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
  aliasing end-to-end. Expands in M13.1.

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

The `list`/`map` types shipped in M6 with literals, indexing, and
iteration but no manipulation helpers. M9 adds two namespaced
libraries plus a small piece of language sugar for the common
append case.

- **`lists` library** (`use lists;`, namespaced). `lists.push`,
  `lists.pop`, `lists.first`, `lists.last`, `lists.head`,
  `lists.tail`, `lists.reverse`, `lists.sort`, `lists.contains`,
  `lists.concat`, `lists.slice`. Every function returns a **new
  list** - the input is never mutated, callers write
  `$xs = lists.push($xs, item);`. `sort` accepts numeric, string, or
  bool elements (mixed int/float promotes; other mixes error).
  Comparator-based sort is deferred until methods are first-class
  values.
- **`maps` library** (`use maps;`, namespaced). `maps.keys`,
  `maps.values`, `maps.has`, `maps.delete`, `maps.merge`. Same
  value-semantics shape. `maps.delete` of a missing key errors
  (strict at boundaries, matching `$m[missing]`); `maps.merge`
  layers the second arg over the first.
- **BREAKING:** `has()` moved out of `core` and into `maps` as
  `maps.has(m, key)`. Programs that called bare `has(...)` now
  need `use maps;` and the qualified form. Rationale: `has` was
  the only non-polymorphic name in core - it only ever worked on
  maps - so it fits the M8 rule that domain-specific names live in
  their domain library. `len` stays in core because it really is
  polymorphic across string / list / map.
- **Namespaced, not flat.** Both libraries collide with `strings`
  (`contains`) and future libraries on common verbs (`parse`,
  `delete`, ...). Per M8's library-author rule, domain libraries
  with collision-prone names take a namespace; flat APIs stay
  reserved for the five essentials.
- **`core.has` stays map-only.** List membership goes through
  `lists.contains($xs, item)`, haystack-first like
  `strings.contains` - PHP's `in_array(needle, haystack)` order is
  deliberately not adopted.
- **Sugar: `$xs[] = item;`** - write-only target meaning "the
  position just past the end of the list". Equivalent to
  `$xs = lists.push($xs, item);` but cheaper to write. Reads of
  `$xs[]` and chained forms like `$xs[0][]` are parse errors;
  non-list targets error at runtime. New AST node `AppendStmt`;
  fmt re-emits the form unchanged.

See:
- [libraries/lists.md](libraries/lists.md) / [libraries/maps.md](libraries/maps.md) -
  function-by-function reference.
- [libraries/index.md](libraries/index.md) - catalog and the
  flat-vs-namespaced rule the M9 design follows.
- [user-guide/imports.md](user-guide/imports.md) and
  [user-guide/types-and-values.md > The `$xs[]` append sugar](user-guide/types-and-values.md#the-xs-append-sugar) -
  user-facing reference for `use lists;` / `use maps;` and the
  append form.
- [user-guide/style-guide.md](user-guide/style-guide.md) -
  no-whitespace-inside-`[]` style rule applies to the append form
  too.
- [technical/grammar.md](technical/grammar.md) - EBNF for
  `appendStmt`; AST table entry for `AppendStmt`.

---

The next phase splits into three arcs: Phase A finishes the
language so libraries have something to stand on, Phase B ships
the foundational libraries that every Jennifer program needs,
Phase C ships I/O libraries, and Phase D ships the higher-level
ecosystem (Jennifer-coded libraries, the module system that
unblocks them, crypto, a server). Phase E (WASM and specialised
domains) is the long horizon.

The library milestones use sub-numbering (M13.1, M13.2, ...) so
each library ships and is reviewed independently. This is the
first time we use sub-milestones; the practice is justified
because each library is small enough to land in a single sitting
once the language foundation is in place.

---

**Phase A: language completion (M10-M12).** These three milestones
close the biggest daily-use gaps and add the foundational types every
later library needs.

## M10 - Control-flow completion

- `break;` and `continue;` inside `while`/`for`/`for-each`/`repeat`.
- `repeat { } until (cond);` post-test loop. New keywords `repeat`
  and `until`. `until` makes condition inversion explicit, matching
  the word-operator style (`and`/`or`/`not`); `do { } while ...`
  considered and rejected.
- `exit;` and `exit EXPR;`. Process exit; integer expression sets
  the OS exit code on top-level execution. Distinct from `return`
  (which stays method-scoped).
- Bundled: printf `%s|align=center` while the modifier table is
  being touched anyway.

## M11 - Bytes and bit operators

- New primitive type `bytes` - **mutable** byte sequence with
  `list`-like semantics. Indexing yields `int` in `[0, 255]`;
  index-assign writes `$b[i] = 0xff;` (out-of-range writes are
  positioned runtime errors). Append via the same `$b[] = byte;`
  sugar M9 introduces for lists. **Value semantics** like lists:
  `$a = $b;` deep-copies, so passing bytes into a method can't
  surprise the caller. `const` is deep. `len($b)` returns the
  count. Rune-aware ops stay in `strings`; byte-level ops live
  here. (Mutable was chosen over immutable to fit real
  buffer-shaped workflows - I/O, hashing-while-streaming,
  in-place encrypt/decrypt - without forcing an allocate-new
  loop for each transformation.)
- New `bytes <-> string` codecs (UTF-8 by default; lossless
  re-encoding lives in the future `encoding` library).
- New operators `& | ^ ~ << >>` on `int` - promoted to syntax,
  not library calls. Library form would parallel arithmetic
  (`bitand` vs `+`) and violate "one way per thing."
- **Non-decimal integer literals**: hex `0xff` / `0xDEAD_BEEF`,
  octal `0o755`, binary `0b1010_0110`. All three lex as `int`
  (same kind, same operators); `_` accepted as a digit separator
  in any position except leading or trailing. Decimal literals
  also gain the `_` separator (`1_000_000`). Lexer-only change,
  no new runtime semantics. Lands here because byte and bit work
  without hex is the wrong default for the milestone.
- Resolves the M7-deferred stdin builtins that waited on the
  binary representation: `io.readBytes(n) -> bytes` (exact n;
  partial result at EOF, then `eof()` is true) and `io.readChars(n)
  -> string` (n runes, decoded from stdin's UTF-8). M7's `eof()`
  composes with both unchanged.

## M12 - Structs / records

- New `def struct Name { field as type, ... };` syntax (working
  name; revisited at start of M12).
- Literals: `Name{ field: expr, ... }`. Field access: `$p.field`.
  Value semantics like lists/maps - `def $q as Name init $p;`
  copies. `const` is deep.
- Unblocks every library that wants to return composite data
  (file info, time values, network endpoints, http request /
  response).

---

**Phase B: foundational libraries (M13.x).** Small, frequently-used
libraries grouped under M13 with sub-numbering; each sub-milestone
ships one library independently.

## M13.1 - `os`

Expands the M8 demo slice (`os.platform()`, `os.getEnv(name)`,
`os.JENNIFER_LF`, `os.JENNIFER_OS` already ship there). Adds:
process args (`os.args -> list of string`), exit code
(`os.exit(n)`), and the rest of the `JENNIFER_*` constants
(`os.JENNIFER_BUILD`, `os.JENNIFER_PLATFORM`,
`os.JENNIFER_ARCHITECTURE`).

## M13.2 - `random`

Non-crypto pseudo-random. `random.rand()` returns float in
`[0, 1)`. `random.randInt(lo, hi)`. `random.seed(n)` for
reproducibility. Crypto-grade random ships separately in M17.

## M13.3 - `time`

New `time` value type. `time.now()`, formatting, parsing,
arithmetic on durations. Larger than other M13 entries because
the type is new; may split into M13.3.x if scope demands.

## M13.4 - `hash` and `crc`

Common digests over `bytes` (MD5, SHA-1, SHA-256, CRC32,
CRC64). Pure compute, no dependencies. Crypto-relevant primitives
that don't require key material live here; key-based crypto goes
in M17.

## M13.5 - `encoding`

`encoding.isAscii`, `encoding.lenBytes`, `encoding.lenRunes`,
`encoding.toBytes($s, $codec)`, `encoding.toString($bytes,
$codec)`, hex / base64 helpers. Operates at the `bytes <->
string` boundary.

---

**Phase C: I/O libraries (M14.x).** System libraries that touch the
OS or do significant compute.

## M14.1 - `fs`

File I/O. `fs.read`, `fs.write` for `string` and `bytes`,
`fs.stat` returning a struct, directory walk. Requires M11
(bytes) and M12 (structs). Brings the M7-deferred file handles
and any non-stdin input source (`fs.open(path) -> handle`,
`handle.readLine()`, etc.) under one library.

## M14.2 - `net`

Sockets. Plain TCP and UDP first; TLS deferred to its own
sub-milestone. The first place a real `bytes` round-trip
matters.

## M14.3 - `regex`

Regular expressions over `string`. Large standalone milestone;
pure string processing, no other dependencies.

---

**Phase D: higher-level and Jennifer-coded libraries (M15-M18).**

## M15 - Module system for Jennifer-coded libraries

Real modules so `.j` libraries get namespaces, scope, and
explicit exports. Lands the form M8 deferred
(`import "templateengine.j" as ninja;` with a real module
boundary; today `import "x.j";` is a textual splice with no
namespace). File imports stay as textual splice for inline
composition (same source file); library distribution moves to
modules.

## M16.x - Jennifer-coded essentials

Built atop the existing system libraries. Sub-milestones in
priority order:

- **M16.1 - `http`** (client) - atop `net`.
- **M16.2 - `json`** - data interchange ubiquity.
- **M16.3 - `csv`** - simple, useful early.
- **M16.4 - `yaml`, `xml`, `markdown`, `pretty`** - one or more
  sub-milestones depending on scope when planned.

## M17 - `crypto`

Symmetric and asymmetric primitives, key derivation,
crypto-grade random. System library; TinyGo-safe primitives
only. Hashes already shipped in M13.4.

## M18 - `httpd`

Pure Jennifer HTTP server atop `net`. The point where Jennifer
becomes useful for serving content.

---

**Phase E: WASM and specialised domains (M19+).** Not committed to a
timeline; recorded so the design doesn't foreclose them.

## M19 - WASM runtime embedding

Wazero or similar inside the interpreter binary. TinyGo-size
cost evaluated honestly before commitment. Without M19, no WASM
libraries.

## M20.x - WASM libraries

If M19 ships, sandboxed plugins via `use wasm:libname;`. Each
library a sub-milestone.

## M21+ - Specialised domains

Each domain its own milestone with sub-milestones as needed:

- **ML.** Vector, matrix, stats, ML primitives.
- **Bioinformatics.** Sequence alignment (Smith-Waterman,
  Needleman-Wunsch), FASTA/FASTQ parsers, molecule structures.
- **Sandbox.** Restricted-capability execution.

Ordered when demand surfaces. WASM libraries (M20.x) may cover
some of this space first.

---

## Path to 1.0.0 distribution (parallel track)

Not milestone-gated; items can be picked up between language
milestones. The earlier this track makes Jennifer visible, the
sooner breaking changes feed into a real audience before 1.0.0
locks the API.

- **CI.** GitHub Actions running `go test ./...` and
  `tinygo build` on every push and PR.
- **Tagged releases.** Prebuilt binaries on GitHub releases.
  Linux/amd64 first, then arm64; macOS and Windows after
  cross-platform support lands.
- **Debian package** (`.deb`). Hosted from a GitHub release
  artifact initially.
- **Arch.** AUR `jennifer-bin` (prebuilt) and `jennifer-git`
  (source) packages.
- **Documentation site.** GitHub Pages driven from `docs/`
  (mdBook or similar; needs a design pass).
- **Optional later:** Homebrew tap, Snap, Nix package.

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
- **`fmt` comment preservation.** Today the lexer drops `#` and
  `/* */` comments; the formatter would need to carry them as
  tokens (or attach them to following nodes) and weave them back
  in. Deferred from M5.
- **`fmt` blank-line preservation or auto-insertion.** Either keep
  user blank lines as a side channel during lexing, or insert them
  automatically between logical groups (imports vs. methods vs.
  top-level code, method-to-method). Deferred from M5.
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
