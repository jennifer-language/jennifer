# Jennifer Programming Language

**Milestone 16.13**

Jennifer is a small, experimental, interpreted programming language.

The interpreter is written in Go and ships as two binaries:
**`jennifer`** (built with the standard Go toolchain, full
host-feature surface - the default you install and reach for) and
**`jennifer-tiny`** (built with [TinyGo](https://tinygo.org/), smaller
and embeddable; missing `os/exec` and the network stack). `make
build` produces both side by side. Source files use the `.j`
extension.

Jennifer currently targets **Linux**; Windows and macOS support is planned.

## Which binary?

Same source, same language; pick by use case:

- **`jennifer`** (standard Go toolchain, the default): full
  host-feature surface, faster on compute-bound code, supports
  `os.run` / `os.spawn` / the whole `net` library. This is what
  you want unless you have a specific reason to use the constrained
  variant.
- **`jennifer-tiny`** (TinyGo): smaller binary, embeddable in
  minimal-footprint deployments (embedded systems, minimal
  containers, small-footprint scripting hosts). Trade-off: no
  `os/exec` (TinyGo runtime gap) and no network stack (no netdev
  driver registered). Calls into those surfaces return a friendly
  runtime error pointing back at `jennifer`.

Both binaries install side by side. Benchmarks comparing the two on
the same workload set live in
[docs/technical/tinygo.md > Single-binary benchmark results](docs/technical/tinygo.md#single-binary-benchmark-results).

## Install

Linux only today; macOS/Windows are post-1.0 work.

```sh
# Debian / Ubuntu - pick the .deb for your arch from the Releases
# page (https://github.com/mplx/jennifer-lang/releases) and:
sudo dpkg -i jennifer_X.Y.Z_amd64.deb

# Arch (AUR) - prebuilt binary, fast install:
yay -S jennifer-bin
# Or build-from-source, tracks main:
yay -S jennifer-git

# Any Linux - tarball download + extract:
tar -xzf jennifer-X.Y.Z-linux-amd64.tar.gz
cd jennifer-X.Y.Z-linux-amd64
./jennifer version

# Build from source (developer path, any platform with Go + TinyGo):
make build
```

See [docs/user-guide/installing.md](docs/user-guide/installing.md)
for verified checksums, the full FHS layout, system-wide install
commands from a tarball, and platform-specific notes.

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
full table and rationale.

## Quick start (developer)

If you cloned the repo and want to build + iterate locally:

```sh
# Build both binaries side by side
make build
./jennifer      run examples/hello.j   # prints "42"  (standard Go, default)
./jennifer-tiny run examples/hello.j   # prints "42"  (TinyGo, constrained)

# Quick iteration without rebuilding
go run ./cmd/jennifer run examples/hello.j
```

A first program:

```jennifer
use io;

def x as int init 21;
io.printf("%d\n", $x + $x);
```

## Documentation

The full docs are served as an mdBook at
[mplx.github.io/jennifer-lang](https://mplx.github.io/jennifer-lang/)
(published from `main` on every push). The same content also reads
fine inside the GitHub file tree:

- [docs/user-guide/](docs/user-guide/index.md) - language tutorial
  and reference split by topic: installing, first program, syntax,
  types and values, methods, control flow, imports, examples.
- [docs/libraries/](docs/libraries/index.md) - per-library reference
  + alphabetical cheatsheet of every builtin.
- [docs/technical/](docs/technical/index.md) - interpreter internals:
  lexer, grammar / parser, preprocessor, interpreter, CLI, testing,
  TinyGo notes.
- [docs/milestones.md](docs/milestones.md) - what's implemented,
  what's coming, and the rationale behind the order.
- [RELEASE.md](RELEASE.md) - the maintainer-facing release checklist.

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

LGPL-3.0-only. The full license text and copyright information
ship inside the packaged distributions
([packaging/debian/copyright](packaging/debian/copyright) for the
`.deb`, the AUR packages reference the upstream license);
[gnu.org/licenses/lgpl-3.0.html](https://www.gnu.org/licenses/lgpl-3.0.html)
is the canonical text.
