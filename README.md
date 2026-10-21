<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="site/logo/jennifer-wordmark-dark.svg">
    <img src="site/logo/jennifer-wordmark.svg" alt="Jennifer" width="420">
  </picture>
</p>

# Jennifer Programming Language

Jennifer is an interpreted programming language.

Jennifer is **batteries-included**:
**[41 built-in libraries](docs/libraries/index.md) and
[74 distributable modules](docs/modules/index.md) - over 2,000 functions,
constants, and types** cover what real tools need. Talk to a database with the
[SQL](docs/libraries/sql.md) library (MySQL/MariaDB/Galera and PostgreSQL,
parameterized and injection-safe) or map rows to structs with the
[ORM](docs/modules/orm.md). Read and write [JSON](docs/libraries/json.md),
[YAML](docs/libraries/yaml.md), [TOML](docs/libraries/toml.md), and
[XML](docs/libraries/xml.md); match text with full
[regular expressions](docs/libraries/regex.md). For the web there is an
[HTTP/S client](docs/modules/http.md) and [REST](docs/modules/rest.md) layer to
call out, and a built-in [HTTP server](docs/libraries/httpd.md) and
[web framework](docs/modules/web.md) to serve. Email: [SMTP](docs/modules/smtp.md)
to send, [POP3](docs/modules/pop.md) / [IMAP](docs/modules/imap.md) to receive.
Caches and stores: [Redis](docs/modules/redis.md) and
[memcached](docs/modules/memcache.md) clients. Security:
[AES-256-GCM, Ed25519, HKDF / PBKDF2](docs/libraries/crypto.md), and
[JWT](docs/modules/jwt.md). The [Model Context Protocol](docs/modules/mcp.md)
module exposes your functions as tools / resources / prompts to an AI host
(Claude, an IDE, ...) or calls another MCP server as a client, validated end to
end against the official MCP SDK. Lightweight
[concurrency](docs/user-guide/concurrency.md) is built into the language via
`spawn` and the [task](docs/libraries/task.md) library. More niche needs are
covered too: [Gotify](docs/modules/gotify.md) push, strict
[SemVer](docs/modules/semver.md), [S3](docs/modules/s3.md) object storage,
[vCard](docs/modules/vcard.md) / [iCalendar](docs/modules/ical.md),
[MQTT](docs/modules/mqtt.md) and [AMQP](docs/modules/amqp.md) messaging,
[ACME](docs/modules/acme.md) certificate issuance,
[TOTP](docs/modules/totp.md) two-factor codes, and more.

The interpreter and its built-in libraries ship as **one self-contained static
binary** - about 17 MB with everything, about 5 MB for the embeddable build - with
no separate runtime and no dependency tree to resolve. The modules ship as plain
`.j` source files (installed beside the binary and resolved through the module
path), readable and editable with no compile step. Third-party **decks**
(Jennifer's packages) are opt-in and vendored into your project's own tree - no
global install, no daemon - with the `jvc` package manager to resolve and pin
them, and imported as `import "@scope/package/";`.

Jennifer also suits **teaching and learning**: an interactive
[REPL](docs/technical/cli_repl.md), an
[easy-to-read grammar](docs/technical/grammar.md), and
[token and AST dumps](docs/technical/cli_inspect.md) for studying language
design, plus a built-in [linter](docs/technical/cli_lint.md),
[profiler](docs/technical/cli_profile.md), and
[test support](docs/technical/cli_test.md). Its
[strict, explicit design](docs/design-stances.md) - conditions must be
`bool`, conversions are spelled out, names never shadow, and errors are
positioned - turns a mistake into a clear message instead of a silent
surprise.

The interpreter is written in Go and ships as two binaries:
**`jennifer`** (standard Go toolchain, full host-feature surface, the default)
and **`jennifer-tiny`** (built with [TinyGo](https://tinygo.org/), smaller and
embeddable; missing `os/exec` and the network stack). `make build` produces
both side by side. Source files use the `.j` extension.

Jennifer's supported platform is **Linux**. Best-effort, **unsupported**
macOS (Intel and Apple Silicon) and Windows (64- and 32-bit) binaries are
published alongside each release - see [Install](#install) for the caveats.

## Which binary?

Same source, same language; pick by use case:

- **`jennifer`** (standard Go toolchain, the default): full
  host-feature surface, competitive on single-thread compute and the
  reliable choice for multi-core parallel `spawn` (it wins the
  end-to-end wall clock whenever parallelism is in play), supports
  `os.run` / `os.spawn` / the whole `net` library. Use this unless
  you need the constrained variant.
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
commands from a tarball, the container image (Docker / Podman), the Windows
installer, and platform-specific notes.

## Editor support & coding with an AI assistant

Syntax highlighting for your editor lives in [editors/](editors/): a
drop-in for Vim / Neovim, a VS Code extension, a Sublime Text syntax, a
TextMate grammar for Zed and other TextMate-based editors, and a highlight.js
definition for static sites.

AI assistants have no built-in knowledge of Jennifer, so the repo ships
[JENNIFER.md](JENNIFER.md), a self-contained language reference. Drop it into
your project and point your assistant at it ("we code in Jennifer, see
JENNIFER.md") and it writes correct `.j` instead of guessing
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

Seven design stances shape every feature in Jennifer. "Convenience" is
rejected when it creates parallel ways to do the same thing or hides
what the code does. See [docs/design-stances.md](docs/design-stances.md) for the
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

The full docs are built with [Grimoire](https://grimoire.jennifer-lang.dev/)
and served at [jennifer-lang.dev](https://jennifer-lang.dev/)
(published from `main` on every push). The same content also reads
fine inside the GitHub file tree:

- [docs/user-guide/](docs/user-guide/index.md) - language tutorial
  and reference split by topic: installing, first program, syntax,
  types and values, functions, control flow, concurrency, imports,
  tooling, style guide, best practices, performance, examples.
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
