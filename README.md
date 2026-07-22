<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="site/logo/jennifer-wordmark-dark.svg">
    <img src="site/logo/jennifer-wordmark.svg" alt="Jennifer" width="420">
  </picture>
</p>

# Jennifer Programming Language

Jennifer is a small, experimental, interpreted programming language.

But small is not bare. Jennifer is batteries-included: a broad standard
library and a growing set of distributable modules cover what real programs
actually need, so you build genuine tools, not toys. Text handling has full
[regular expressions](docs/libraries/regex.md); structured data flows through
[JSON](docs/libraries/json.md); email is a complete stack -
[SMTP](docs/modules/smtp.md) to send, [POP3](docs/modules/pop.md) and
[IMAP](docs/modules/imap.md) to receive; in-memory data stores come through
[Redis](docs/modules/redis.md) and [memcached](docs/modules/memcache.md)
clients; the web runs from an ergonomic [REST client](docs/modules/rest.md)
to turnkey integrations such as [Gotify](docs/modules/gotify.md) push
notifications; and lightweight
[concurrency](docs/user-guide/concurrency.md) is built into the language via
`spawn` and the [task](docs/libraries/task.md) library. Browse the full
[library catalog](docs/libraries/index.md) and
[module catalog](docs/modules/index.md) - both grow with every release.

It is also a natural fit for teaching and learning: an interactive
[REPL](docs/technical/cli_repl.md), an
[easy-to-read grammar](docs/technical/grammar.md), and
[token and AST dumps](docs/technical/cli_inspect.md) that make it ideal for
mastering language design, plus a built-in
[linter](docs/technical/cli_lint.md) and
[profiler](docs/technical/cli_profile.md) and full
[test support](docs/technical/cli_test.md). Its
[strict, explicit design](docs/design-stances.md) - conditions must be
`bool`, conversions are spelled out, names never shadow, and errors are
positioned - surfaces a mistake as a clear message instead of a silent
surprise, so a learner sees exactly what went wrong.

The interpreter is written in Go and ships as two binaries:
**`jennifer`** (built with the standard Go toolchain, full
host-feature surface - the default you install and reach for) and
**`jennifer-tiny`** (built with [TinyGo](https://tinygo.org/), smaller
and embeddable; missing `os/exec` and the network stack). `make
build` produces both side by side. Source files use the `.j`
extension.

Jennifer's supported platform is **Linux**. Best-effort, **unsupported**
macOS and Windows binaries (64- and 32-bit) are published alongside each
release - see [Install](#install) for the caveats.

## Which binary?

Same source, same language; pick by use case:

- **`jennifer`** (standard Go toolchain, the default): full
  host-feature surface, competitive on single-thread compute and the
  reliable choice for multi-core parallel `spawn` (it wins the
  end-to-end wall clock whenever parallelism is in play), supports
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
[docs/technical/benchmark.md](docs/technical/benchmark.md).

## Install

**Linux** is the supported platform. Best-effort **unsupported** macOS
and Windows binaries (the standard `jennifer` build only, no TinyGo) are
attached to each [release](https://github.com/jennifer-language/jennifer/releases);
read the caveats in
[docs/user-guide/installing.md](docs/user-guide/installing.md#macos--windows-unsupported)
before relying on them.

```sh
# Debian / Ubuntu - pick the .deb for your arch from the Releases
# page (https://github.com/jennifer-language/jennifer/releases) and:
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

## Editor support & coding with an AI assistant

Syntax highlighting for your editor lives in [editors/](editors/): a
true drop-in for Vim / Neovim, a TextMate grammar for VS Code /
Sublime / Zed, and a highlight.js definition for static sites.

And because Jennifer is new enough that an AI assistant has no built-in
knowledge of it, we ship [JENNIFER.md](JENNIFER.md) - a single,
self-contained language reference. Drop it into your project and point
your assistant at it ("we code in Jennifer, see JENNIFER.md, let's go")
and it writes correct `.j` from the start instead of guessing
Python-with-dollar-signs. Details in
[docs/user-guide/tooling.md](docs/user-guide/tooling.md).

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
[jennifer-lang.dev](https://jennifer-lang.dev/)
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
