# Jennifer - User Guide

Jennifer is an interpreted programming language with structs, enums and
value semantics, lightweight concurrency, a topic-based standard library,
and a set of bundled modules written in Jennifer.

This guide covers the whole language. Run `jennifer version` to see which
build you are on.

## Design stances

Eight decisions shape every feature and explain why the language looks the
way it does. Read the [design stances](../design-stances.md) before the
topical chapters.

## Contents

- [Installing & running](installing.md) - building the binary, running a
  program, the interactive REPL, the inspection and formatting commands.
- [Your first program](first-program.md) - a one-screen `hello.j` and a
  walkthrough of what each line does.
- [Editor & AI support](tooling.md) - syntax highlighting for your editor,
  and the drop-in `JENNIFER.md` that lets an AI assistant write Jennifer.
- [Syntax](syntax.md) - tokens, comments, identifier rules.
- [Types and values](types-and-values.md) - the primitive types,
  variables and constants, scoping rules, and the compound types
  `list` and `map`.
- [Functions](functions.md) - declaring functions, parameters, return values,
  recursion, hoisting, naming rules.
- [Control flow](control-flow.md) - operators, precedence, `if` / `while`
  / `for` / for-each, `match`, `try` / `catch`, `defer`.
- [Concurrency](concurrency.md) - `spawn`, `task of T`, channels, the
  loud-fail contract.
- [Imports](imports.md) - `use LIB;` for libraries, `include` and
  `import "file.j";` for source files, the requirement header.
- [Examples](examples.md) - small programs that exercise the language
  end-to-end.
- [Style guide](style-guide.md) - the canonical source style that
  `jennifer fmt` produces; the spacing/indentation rules, the names
  convention, the `[]` and `{}` literal layout.
- [Best practices](best-practices.md) - `jennifer lint`, doc comments,
  and code-smell heuristics with the reasoning behind each.
- [Performance](performance.md) - the few patterns that turn a
  quadratic hot path into a linear one (append-and-join over `+` in a
  loop, hoisting per-element calls out of hot loops), and what the
  interpreter already optimizes for you.

## Related

- [docs/libraries/](../libraries/index.md) - one reference page per
  standard library (`io`, `convert`, `math`, `strings`, ...).
- [docs/modules/](../modules/index.md) - the Jennifer-coded modules that
  ship with the interpreter, brought in with `import` (`ansi`, `csv`,
  `mime`, `redis`, ...).
- [docs/technical/](../technical/index.md) - interpreter internals for
  contributors.
- [docs/glossary.md](../glossary.md) - project terminology: where two
  words could name the same concept (library vs module, list vs array),
  the one the project uses.
- [docs/milestones.md](../milestones.md) - what's shipped;
  [docs/roadmap.md](../roadmap.md) - what's planned.
