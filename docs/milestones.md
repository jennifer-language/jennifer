# Jennifer - Milestones

Development is split into milestones. Each milestone produces a *working*
interpreter that runs a strictly larger subset of the language.

Status legend:

- ✅ done
- 🚧 in progress
- ⬜ not started

---

## M1 - End-to-end MVP ✅

The smallest possible vertical slice that proves the pipeline:
source → tokens → preprocessed tokens → AST → result.

**Language subset:**

- Types: `int`, `string` only
- `define $x as int init 5;` (the `init` clause is required in M1)
- `$var` references
- Arithmetic: `+ - * /` and `%` on ints; parenthesised grouping
- `printf("text")` and `printf($var)` - single argument, no format specifiers
- `import stdlib;` (library import)
- `import file.j;` (file import - textual splice; works anywhere, including
  inside a block; circular-import detection)
- `def app() { ... }` (zero-arg, top-level only)
- `def` and `define` are synonyms (one `TOKEN_DEFINE`; parser disambiguates)
- Comments: `//` and `/* */`

**What lands beyond the bare MVP:**

- Source-context caret in error messages (`file: error at L:C` + the offending
  line + caret)
- Golden-file integration test that walks `examples/*.j`
- TinyGo build target verified

**Exit criterion:** `./jennifer run examples/hello.j` prints `42`.

---

## M2 - Types, constants, scoping, control flow ⬜

Rounds out the "ordinary" feature set the spec calls for.

**New types and literals:**

- `float`, `null`, `bool` types
- Literals `null`, `true`, `false`
- `float` literals: `3.14`, `0.5`

**Variable system:**

- `define $x as int;` (uninitialized → zero value of the type)
- `define const NAME as TYPE init VALUE;` - constants; assignment-after-init
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

**Decision required at start of M2:** semantics of uninitialized `define`
(recommend: zero value of the declared type - `0`, `0.0`, `""`, `false`).

**New tests:** scope tests (inner reads outer; inner cannot re-declare; const
cannot be reassigned), full arithmetic/comparison matrices, programs like
`fizzbuzz.j` and `fib.j`.

**Docs to update:** full grammar in `technical.md`, full type table and
scoping rules in `user-guide.md`.

---

## M3 - Methods with parameters and return values ⬜

- `def name(a as int, b as string) { ... }` - parameter parsing
- Argument passing - by value for scalars
- `return;` and `return EXPR;`
- Type-checking at the call site: argument count and declared type must match
- Method calls inside expressions, recursion (free once methods call methods)
- `sprintf(...)` - returns a formatted string instead of printing
- `printf` / `sprintf` with multiple arguments - format style: Go-like
  `%d %f %s` (familiar; easy to implement)

**New tests:** recursion (`fib`, `fact`), wrong-arity and wrong-type call
errors, `sprintf` output.

**Docs:** methods chapter in `user-guide.md`; calling convention + stack model
in `technical.md`.

---

## M4 - Polish & ergonomics ⬜

- **Better errors:** line/column on every error, source snippet with caret
- **REPL:** `jennifer repl` reusing the existing lexer/parser/interpreter
- **Formatter:** `jennifer fmt` - re-emit the AST as canonical source
- **Logical operators:** `and`, `or`, `not` - only if their absence becomes painful
- **Arrays:** the original spec teased them; significant lift, essentially its own milestone

---

## Future directions (post-M4) ⬜

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
