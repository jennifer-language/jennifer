# Jennifer Programming Language

**Milestone 14**

Jennifer is a small, experimental, interpreted programming language.
The interpreter is written in Go and ships as two binaries:
**`jennifer`** (built with [TinyGo](https://tinygo.org/), small and
embeddable; some host features like `os/exec` aren't implemented by
the TinyGo runtime yet) and **`jennifer-go`** (built with the
standard Go toolchain, full host-feature surface; what you reach for
during development). `make build` produces both side by side. Source
files use the `.j` extension.

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

Seven design stances shape every feature in Jennifer. They are
deliberately uncompromising - "convenience" is rejected when it
creates parallel ways to do the same thing or hides what the code
does. See [docs/design-stances.md](docs/design-stances.md) for the
full table and rationale; the same canonical list is referenced from
the user-guide, the technical docs, and `CLAUDE.md`.

## Quick start

```sh
# Build both binaries (`jennifer` TinyGo + `jennifer-go` standard Go).
# `make build-tinygo` and `make build-go` each produce one alone.
make build

# Run a program on the shipping binary
./jennifer run examples/hello.j        # prints "42"

# Or on the dev binary (full host features - e.g. os.run / os.spawn)
./jennifer-go run examples/hello.j     # prints "42"
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

Tests run under the standard Go toolchain because TinyGo's `testing`
support is partial. After non-trivial changes, smoke-test both
binaries (`make build` produces them) since a few standard-library
features behave differently under the TinyGo runtime - see
[docs/technical/tinygo.md](docs/technical/tinygo.md) for the
current restriction list.

## License

LGPL-3.0-only. See [LICENSE.md](LICENSE.md).
