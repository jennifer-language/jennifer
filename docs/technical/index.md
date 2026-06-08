# Jennifer Interpreter - Technical Documentation

Internals of the Jennifer interpreter. This directory is split by topic;
each page is small enough to read in one sitting.

## Design stances

The interpreter's shape follows from these language-design decisions.
When the implementation seems unusual - the `def` site bare-name rule,
the strict no-truthiness check at every `if`/`while`/`for`, deep-copy
on list/map assignment, the per-topic library directories - the
reasons trace back to one of the seven below. The same list appears in
[README.md](../../README.md) and [../user-guide/](../user-guide/index.md).

1. **One way per thing.** Reject sugar that creates parallel APIs (no
   `++`/`--`, no `+=`, no two `printf` flavors for the same job). One
   canonical form is easier to read than three convenient ones.
2. **Explicit over implicit.** Sigils mark use-site references (`$x`),
   `def` carries the type, libraries are imported per topic
   (`use io;`; nothing auto-loads except `core`), conditions must be
   `bool` (no truthiness), conversions are spelled out (`int(v)`,
   `float(v)`). Nothing important hides.
3. **Presentation, not transformation, in format strings.** `printf`
   verb modifiers shape how a value is rendered (`%d|base=2`,
   `%f|prec=4`). Transforming the value itself (`upper`, `substring`,
   markdown rendering) is a library call. Keeps `printf` small and
   orthogonal to the rest of the standard library.
4. **Strict at boundaries.** Undefined math, missing map keys,
   out-of-bounds reads, and type mismatches are positioned runtime
   errors. No NaN, no silent garbage.
5. **Value semantics for collections.** Lists and maps copy on
   assignment and on parameter binding - no aliasing. `const` is deep:
   it rejects both rebinding and content mutation at any depth.
6. **No shadowing.** A name binds once in any visible scope. Inner
   scopes inherit outer bindings but cannot redeclare them.
7. **Topic-based, opt-in libraries.** The standard library is split by
   topic, never bundled. Every library except the small auto-loaded
   `core` is enabled explicitly with `use NAME;`.

## Contents

- [Lexer](lexer.md) - hand-written scanner, token kinds, position tracking,
  identifier rules.
- [Grammar and parser](grammar.md) - the EBNF the parser accepts, plus the
  AST node table and parser implementation notes.
- [Preprocessor](preprocessor.md) - how `import "file.j";` is expanded
  before the parser runs.
- [Interpreter](interpreter.md) - runtime values, scoped environment,
  execution model, library/builtins, error model.
- [CLI](cli.md) - subcommands (`run`, `repl`, `tokens`, `ast`, `fmt`,
  `version`, `help`), the REPL, the inspection dumps, the formatter,
  and version injection.
- [Testing](testing.md) - which packages test what.
- [File map](filemap.md) - one-line description of every file in the
  repository.
- [Rejected features](rejected.md) - proposals that were turned down,
  with the reasoning so they don't come back.
- [TinyGo notes](tinygo.md) - the constraints TinyGo imposes and how the
  codebase respects them.

## Pipeline

The compiler/interpreter is a five-stage pipeline:

```
   source (string)
       │
       ▼
   ┌────────┐
   │ lexer  │   internal/lexer
   └────────┘
       │ []Token
       ▼
   ┌──────────────┐
   │ preprocessor │   internal/preproc   (splices file imports)
   └──────────────┘
       │ []Token
       ▼
   ┌────────┐
   │ parser │   internal/parser
   └────────┘
       │ *Program (AST)
       ▼
   ┌─────────────┐
   │ interpreter │   internal/interpreter + internal/lib/io (and other libs)
   └─────────────┘
       │
       ▼
     stdout / runtime error
```

The CLI lives in `cmd/jennifer/main.go` and orchestrates these stages.
For the user-facing language reference (types, control flow, libraries),
see [../user-guide/](../user-guide/index.md). For the style rules `jennifer
fmt` enforces, see [../user-guide/style-guide.md](../user-guide/style-guide.md).
