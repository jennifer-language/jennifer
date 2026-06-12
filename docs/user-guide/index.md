# Jennifer - User Guide

Jennifer is a small, experimental, interpreted programming language. This
guide covers everything you can do in Jennifer today. Run
`jennifer version` to see which build you're on; the language history
lives in [docs/milestones.md](../milestones.md).

## Design stances

A handful of decisions shape every feature in Jennifer. Read them
before the topical chapters - they explain why the language looks the
way it does and rule out the "but why don't you just..." reflex. The
same list appears in [README.md](../../README.md) and
[../technical/](../technical/index.md).

1. **One way per thing.** Reject sugar that creates parallel APIs (no
   `++`/`--`, no `+=`, no two `printf` flavors for the same job). One
   canonical form is easier to read than three convenient ones.
2. **Explicit over implicit.** Sigils mark use-site references (`$x`),
   `def` carries the type, libraries are imported per topic
   (`use io;`; nothing auto-loads except `core`), conditions must be
   `bool` (no truthiness), conversions are spelled out (`convert.toInt(v)`,
   `convert.toFloat(v)`). Nothing important hides.
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

- [Installing & running](installing.md) - building the binary, running a
  program, the interactive REPL, the inspection and formatting commands.
- [Your first program](first-program.md) - a one-screen `hello.j` and a
  walkthrough of what each line does.
- [Syntax](syntax.md) - tokens, comments, identifier rules.
- [Types and values](types-and-values.md) - the primitive types,
  variables and constants, scoping rules, and the compound types
  `list` and `map`.
- [Methods](methods.md) - declaring methods, parameters, return values,
  recursion, hoisting, the no-shadowing rule for builtins.
- [Control flow](control-flow.md) - operators, precedence, `if` / `while`
  / `for` / for-each.
- [Imports](imports.md) - `use LIB;` for libraries, `import "file.j";` for
  source files, the library catalog.
- [Examples](examples.md) - small programs that exercise the language
  end-to-end.
- [Style guide](style-guide.md) - the canonical source style that
  `jennifer fmt` produces; the spacing/indentation rules, the names
  convention, the `[]` and `{}` literal layout.
- [Best practices](best-practices.md) - stylistic guidance and
  code-smell heuristics for writing Jennifer that ages well. Not
  enforced by the language; click through if you want the "why".

## Related

- [docs/libraries/](../libraries/index.md) - one reference page per
  standard library (`io`, `convert`, `math`, `strings`, `core`).
- [docs/technical/](../technical/index.md) - interpreter internals for
  contributors.
- [docs/glossary.md](../glossary.md) - canonical project terminology.
  When two words could plausibly name the same concept (function vs
  method, library vs module, list vs array), this page picks the one
  the project uses everywhere.
- [docs/milestones.md](../milestones.md) - what's shipped, what's
  planned.
