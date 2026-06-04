# Jennifer - Milestones

Development is split into milestones. Each milestone produces a *working*
interpreter that runs a strictly larger subset of the language.

---

## M1 - End-to-end MVP

**Status:** done.

The smallest possible vertical slice that proves the pipeline:
source → tokens → preprocessed tokens → AST → result.

**Language subset:**

- Types: `int`, `string` only
- `def x as int init 5;` (the `init` clause is required in M1)
- `$var` references
- Arithmetic: `+ - * /` and `%` on ints; parenthesised grouping
- `printf("text")` and `printf($var)` - single argument, no format specifiers
- `use stdlib;` (library import)
- `import "file.j";` (file import - textual splice; works anywhere, including
  inside a block; circular-import detection; subdirectories supported via the
  string path)
- Method definitions (zero-arg, top-level only)
- Comments: `//` and `/* */`

**Post-M1/M2 syntax adjustments (kept here for historical clarity):**

- The `app()` entry-method requirement was dropped: top-level statements run
  in source order, methods are just hoisted callables.
- `define` was originally a synonym for `def`. It has been removed; only
  `def` remains, and methods use a new `func` keyword.
- At the def site, names are bare identifiers (no `$` sigil). The `$` is
  reserved for use-site references to mutable variables.
- Imports split into two keywords: `use NAME;` for system libraries (originally
  `import NAME;`) and `import "PATH.j";` for files (originally
  `import NAME.j;` with an identifier path). The new string-literal path
  enables subdirectories, hyphens, and underscores.

**What lands beyond the bare MVP:**

- Source-context caret in error messages (`file: error at L:C` + the offending
  line + caret)
- Golden-file integration test that walks `examples/*.j`
- TinyGo build target verified

**Exit criterion:** `./jennifer run examples/hello.j` prints `42`.

---

## M2 - Types, constants, scoping, control flow

**Status:** done.

**Decision (resolved at start of M2):** uninitialized `def x as T;` gives
`$x` the zero value of `T` (`0`, `0.0`, `""`, `false`, `null`).

Rounds out the "ordinary" feature set the spec calls for.

**New types and literals:**

- `float`, `null`, `bool` types
- Literals `null`, `true`, `false`
- `float` literals: `3.14`, `0.5`

**Variable system:**

- `def x as int;` (uninitialized → zero value of the type)
- `def const NAME as TYPE init VALUE;` - constants; assignment-after-init
  is an error
- Name-rule enforcement: variable names `[A-Za-z]{1,64}`, constant names
  `[A-Z]{1,64}`
- Nested block scoping with the full visibility/no-shadowing rules
- Assignment statement: `$x = EXPR;`

**Operators:**

- Comparison: `< > <= >= ==` → `bool`
- `+` for string concatenation
- `int` ↔ `float` promotion in mixed arithmetic (result is `float`)
- Escape-sequence parsing inside `'...'` strings (currently only `"..."`)

**Control flow:**

- `if (cond) { } elseif (cond) { } else { }`
- `while (cond) { }`
- `for (init; cond; step) { }`
- All conditions must be `bool` (no implicit truthiness)

**New AST nodes:** `FloatLit`, `NullLit`, `BoolLit`, `ConstDefineStmt`,
`AssignStmt`, `IfStmt`, `WhileStmt`, `ForStmt`, `CompareExpr`.

**Decision required at start of M2:** semantics of uninitialized `def`
(recommend: zero value of the declared type - `0`, `0.0`, `""`, `false`).

**New tests:** scope tests (inner reads outer; inner cannot re-declare; const
cannot be reassigned), full arithmetic/comparison matrices, programs like
`fizzbuzz.j` and `fib.j`.

---

## M3 - Methods with parameters and return values

**Status:** done.

- `func name(a as int, b as string) { ... }` - parameter parsing
- Argument passing - by value (parameters bind in a fresh call frame whose
  parent is the global env)
- `return;` and `return EXPR;`
- Type-checking at the call site: argument count and declared type both checked
- Method calls inside expressions, recursion (free once methods call methods)
- `sprintf(...)` returns a formatted string instead of writing
- `printf` / `sprintf` accept Go-style format strings with verbs `%d %f %s %t %v %%`
- `examples/factorial.j` added as a recursion smoke test with golden output

---

## M4 - Polish & ergonomics

**Status:** not started.


- **Better errors:** line/column on every error, source snippet with caret
- **REPL:** `jennifer repl` reusing the existing lexer/parser/interpreter
- **Formatter:** `jennifer fmt` - re-emit the AST as canonical source
- **Logical operators:** `and`, `or`, `not` - only if their absence becomes painful
- **Type-conversion functions:** stdlib builtins `string(v)`, `int(v)`, `float(v)`, `bool(v)`.
  Explicit casts only - no implicit coercion in `+` or comparisons. Each errors on
  impossible conversions (e.g. `int("abc")`). Roughly:
  - `string(any)` -> `string` (uses `Value.Display()`)
  - `int(int|float|string|bool)` -> `int` (truncate float; parse string; true=1/false=0)
  - `float(int|float|string)` -> `float`
  - `bool(bool|int)` -> `bool` (nonzero int is true) - or restrict to bool to stay strict
- **Arrays:** the original spec teased them; significant lift, essentially its own milestone

---

## Future directions (post-M4)

Long-term goals - not committed to a milestone yet, but the code should not
foreclose them.

- **Cross-platform support.** Today Jennifer targets Linux only. Windows and
  macOS are planned. When touching filesystem, paths, line endings, or
  process behavior, prefer portable stdlib helpers (`path/filepath`, not
  hardcoded `/`); avoid Linux-only assumptions.
- **macflyos kernel integration.** A long-term goal is to embed the Jennifer
  interpreter into **macflyos**, an experimental OS also
  written in TinyGo. This reinforces the TinyGo-friendliness discipline: no
  `reflect`-heavy code, no goroutines in the core, no heavy stdlib
  dependencies, and no hard dependencies on a hosted runtime (ambient stdin,
  network, dynamic linking).
