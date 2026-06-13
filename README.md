# Jennifer Programming Language

**Milestone 14**

Jennifer is a small, experimental, interpreted programming language. The
interpreter is written in Go and the shipping binary is produced with
[TinyGo](https://tinygo.org/). Source files use the `.j` extension.

This project exists primarily as a learning exercise: how to design a
language and build an interpreter end-to-end (lexer → preprocessor → parser →
tree-walking evaluator → stdlib).

Jennifer currently targets **Linux**; Windows and macOS support is planned.

## Stability

Jennifer is **pre-1.0**. While the major version stays at `0.x.y`,
**anything can change at any time** - syntax, semantics, library names,
function signatures, file formats. We aim for best-effort stability
between minor versions but make no guarantees: a milestone may rename a
keyword, retype a builtin, or restructure the standard library when a
better design is found. Pin to a specific version if you need
reproducibility; expect to migrate when you upgrade.

Starting with **`1.0.0`**, Jennifer will follow [Semantic
Versioning](https://semver.org/): breaking changes only on a major
version bump, additive features on minor, fixes on patch.

## Design stances

The decisions below shape every feature in Jennifer. They are
deliberately uncompromising - "convenience" is rejected when it
creates parallel ways to do the same thing or hides what the code
does. The same list appears in [docs/user-guide/](docs/user-guide/index.md)
and [docs/technical/](docs/technical/index.md).

1. **One way per thing.** Reject sugar that creates parallel APIs (no
   `++`/`--`, no `+=`, no two `printf` flavors for the same job). One
   canonical form is easier to read than three convenient ones.
2. **Explicit over implicit.** Sigils mark use-site references (`$x`),
   `def` carries the type, libraries are imported per topic
   (`use io;`; nothing auto-loads except `core`), conditions must be
   `bool` (no truthiness), conversions are spelled out
   (`convert.toInt(v)`, `convert.toFloat(v)`). Nothing important hides.
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

## Quick start

```sh
# Build
tinygo build -o jennifer ./cmd/jennifer

# Run a program
./jennifer run examples/hello.j     # prints "42"
```

A first program:

```jennifer
use io;

def x as int init 21;
printf($x + $x);
```

## Documentation

- [docs/user-guide/](docs/user-guide/index.md) - language tutorial and
  reference split by topic: installing, first program, syntax, types
  and values, methods, control flow, imports, examples.
- [docs/technical/](docs/technical/index.md) - interpreter internals split
  by topic: lexer, grammar/parser, preprocessor, interpreter, CLI, testing,
  file map, rejected features, TinyGo notes.
- [docs/milestones.md](docs/milestones.md) - what's implemented, what's
  coming, and the rationale behind the order.

## Testing

```sh
go test ./...
```

Development tests run under regular Go because TinyGo's test runner is
limited; the shipping binary is built with TinyGo.

## License

LGPL-3.0-only. See [LICENSE.md](LICENSE.md).
