# Jennifer Interpreter - Technical Documentation

Internals of the Jennifer interpreter. This directory is split by topic;
each page is small enough to read in one sitting.

## Design stances

The interpreter's shape follows from seven language-design decisions.
When the implementation seems unusual - the `def`-site bare-name rule,
the strict no-truthiness check at every `if`/`while`/`for`, deep-copy
on list/map assignment, the per-topic library directories - the
reasons trace back to one of them.

See [docs/design-stances.md](../design-stances.md) for the canonical
table.

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
- [Design decisions](design-decisions.md) - features that ship despite
  looking like a stance violation at first glance; the reasoning that
  shows they aren't one.
- [TinyGo notes](tinygo.md) - the constraints TinyGo imposes and how the
  codebase respects them.
- [Glossary](../glossary.md) - canonical project terminology
  (function/method, library/module, list/array, ...). The terms
  this file uses match it.

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
