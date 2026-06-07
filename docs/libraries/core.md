# `core` - auto-loaded structural builtins

The `core` library is special: it is **automatically available** in every
Jennifer program. You do **not** write `use core;` - the interpreter
pre-imports it at startup. Writing `use core;` is a runtime error to
keep source files honest about which libraries the program actively
invokes.

```jennifer
use io;

printf("running Jennifer %s\n", JENNIFER_VERSION);
```

(no `use core;` needed; `JENNIFER_VERSION` is already in scope.)

## Why `core` is special

Jennifer's library discipline is "nothing for free": every standard
library is opt-in via `use NAME;`. `core` is the deliberate exception,
reserved for a tiny handful of *structural* builtins that every program
needs without ceremony - things that are more like language operators
than library functions. Reserve membership carefully; almost everything
new belongs in `io`, `math`, `convert`, `strings`, or a new
domain-library, not here.

## Constants

| Name               | Kind     | Value                                                  |
|--------------------|----------|--------------------------------------------------------|
| `JENNIFER_VERSION` | string   | The interpreter's build version (see format below).    |

The `JENNIFER_` prefix follows the precedent set by `PHP_VERSION` and
`RUBY_VERSION` (and similar from other languages). It also leaves the
unprefixed namespace clean for host/environment constants
(`PLATFORM`, `OSNAME`, `ARCH`, ...) when those land.

### `JENNIFER_VERSION` string format

The build pipeline derives `JENNIFER_VERSION` from
`git describe --tags --long`:

| Repository state                          | `JENNIFER_VERSION` value              |
|-------------------------------------------|---------------------------------------|
| HEAD is exactly on a semver tag           | `"0.4.0"`                             |
| HEAD is N commits past the most recent tag | `"0.4.0-dev+2.1023204"`              |
| No tags exist yet                         | `"0.0.0-dev+<N>.<shortsha>"`          |
| Built without git (or outside a repo)     | `"dev"`                               |

The `dev+` prefix is intentional: any non-tagged build is a development
build, and the `N.shortsha` suffix lets you trace which commit produced it.

## Build flow

`JENNIFER_VERSION` is set at build time by a small codegen step. The
Makefile runs `scripts/gen-version.sh` before `tinygo build` /
`go build`, writing a generated `internal/version/version_gen.go` whose
`init()` assigns `version.Version` to the string from
`scripts/version.sh`. The `core` library then mirrors that into the
interpreter as the `JENNIFER_VERSION` constant.

You don't need to run the codegen step manually if you build via `make
build` (TinyGo) or `make build-go` (Go). A bare `go test ./...` skips
codegen and uses the default `"dev"` baked into `version.go`.

Codegen rather than `go build -ldflags -X` is used because TinyGo 0.41
silently ignores `-X`. The generated file is `.gitignore`d.

## Roadmap

Pass 2 of this M5 cleanup migrates `len` from the `strings` library into
`core`, where it becomes polymorphic on strings, lists, and maps once
lists and maps land in M6.

Future candidates for `core` membership (none committed yet):
`PLATFORM`, `BUILDTIME`, `has($m, key)` once maps exist. Each one needs
to clear the bar of "universally needed structural primitive that's more
like an operator than a library function."

See also: [../user-guide.md](../user-guide.md), [../technical/interpreter.md](../technical/interpreter.md#builtins-and-libraries), [index.md](index.md).
