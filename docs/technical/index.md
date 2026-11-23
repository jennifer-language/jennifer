# Jennifer Interpreter - Technical Documentation

Internals of the Jennifer interpreter, split by topic.

## Design stances

The interpreter's shape follows from eight language-design decisions
([design stances](../design-stances.md)). Unusual-looking choices - the
`def`-site bare-name rule, the strict no-truthiness check at every
`if`/`while`/`for`, value semantics for lists and maps, the per-topic
library directories - trace back to one of them.

## Contents

- [Lexer](lexer.md) - hand-written scanner, token kinds, position tracking,
  identifier rules.
- [Grammar and parser](grammar.md) - the EBNF the parser accepts, plus the
  AST node table and parser implementation notes.
- [Preprocessor](preprocessor.md) - how `include "file.j";` is spliced
  before the parser runs (`use` and `import` pass through unchanged).
- [Interpreter](interpreter.md) - runtime values, scoped environment,
  execution model, library/builtins, error model.
- [CLI](cli.md) - subcommands (`run`, `repl`, `tokens`, `ast`, `fmt`,
  `lint`, `profile`, `test`, `serve`, `version`, `help`), each with its own page
  ([REPL](cli_repl.md), [Inspection](cli_inspect.md),
  [Formatter](cli_fmt.md), [Linter](cli_lint.md),
  [Profiler](cli_profile.md), [Test runner](cli_test.md)) linked from the
  index, plus version injection.
- [Testing](testing.md) - which packages test what.
- [File map](filemap.md) - one-line descriptions of the repository's
  files, grouped by area.
- [Rejected features](rejected.md) - proposals that were turned down,
  with the reasoning so they don't come back.
- [Conventions](conventions.md) - the rules of stance 8: names, argument
  order, options, error kinds, caps and security defaults that every
  library, module and deck follows, and how each is checked.
- [Design decisions](design-decisions.md) - features that ship despite
  looking like a stance violation at first glance; the reasoning that
  shows they aren't one.
- [TinyGo notes](tinygo.md) - the constraints TinyGo imposes and how the
  codebase respects them.
- [Benchmarks](benchmark.md) - single-binary throughput (Go vs TinyGo),
  parallelism, and the memory / page-fault trade, from the reference machine.
- [Security model](security-model.md) - what a Jennifer script can do by
  design, what counts as a real vulnerability, and how to run untrusted code.
- [Glossary](../glossary.md) - canonical project terminology
  (function/method, library/module, list/array, ...); these pages use
  its terms.

## Pipeline

The interpreter is a five-stage pipeline (scope analysis sits between
parse and evaluation):

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
   │ preprocessor │   internal/preproc   (splices `include`s)
   └──────────────┘
       │ []Token
       ▼
   ┌────────┐
   │ parser │   internal/parser
   └────────┘
       │ *Program (AST)
       ▼
   ┌──────────┐
   │ resolver │   internal/parser (Resolve)
   └──────────┘    scope pass: numbers slots per frame,
       │           annotates every reference with (Depth, Slot),
       │           reports undefined-variable / shadowing errors
       │           before execution. Idempotent; the REPL skips
       │           it and falls back to name-based lookup.
       │ resolved *Program
       ▼
   ┌─────────────┐
   │ interpreter │   internal/interpreter + internal/lib/io (and other libs)
   └─────────────┘
       │
       ▼
     stdout / runtime error
```

The CLI (`cmd/jennifer/main.go`) runs these stages; `Interpreter.Run`
calls the resolver. For the user-facing language reference (types, control flow, libraries),
see [../user-guide/](../user-guide/index.md). For the style rules `jennifer
fmt` enforces, see [../user-guide/style-guide.md](../user-guide/style-guide.md).
