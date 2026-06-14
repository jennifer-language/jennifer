# `meta` - interpreter-self-identity constants

Enable with `use meta;`. Holds facts about the running Jennifer
interpreter itself - the build version, which Go toolchain compiled
it, and similar information that programs typically log for bug
reports, embed in error messages, or branch on for build-specific
behaviour.

This is distinct from `os` (which is about the *host environment* -
operating system, CPU architecture, environment variables) and from
the rest of the standard library (which is about user data).

```jennifer
use io;
use meta;

io.printf("Jennifer %s (%s build)\n",
    meta.VERSION, meta.BUILD);
```

## Constants

| Name           | Kind   | Value                                                              |
| -------------- | ------ | ------------------------------------------------------------------ |
| `meta.VERSION` | string | The interpreter's build version. See format below.                 |
| `meta.BUILD`   | string | Which Go toolchain compiled the interpreter: `"go"` or `"tinygo"`. |

### `VERSION` string format

The build pipeline derives `meta.VERSION` from
`git describe --tags --long`:

| Repository state                           | `meta.VERSION` value         |
| ------------------------------------------ | ---------------------------- |
| HEAD is exactly on a semver tag            | `"0.4.0"`                    |
| HEAD is N commits past the most recent tag | `"0.4.0-dev+2.1023204"`      |
| No tags exist yet                          | `"0.0.0-dev+<N>.<shortsha>"` |
| Built without git (or outside a repo)      | `"dev"`                      |

The `dev+` prefix is intentional: any non-tagged build is a development
build, and the `N.shortsha` suffix lets you trace which commit produced it.

### `BUILD` values

`meta.BUILD` distinguishes which Go variant compiled the
interpreter binary. Useful because TinyGo has subtly different runtime
behaviour from standard Go (different GC, different scheduler tuning,
different stdlib subset) - a program that needs build-specific behaviour
or just wants to log "which interpreter is this for the bug report" can
branch on this value.

| Value      | Meaning                                       |
| ---------- | --------------------------------------------- |
| `"go"`     | Built with the standard Go toolchain (`gc`)   |
| `"tinygo"` | Built with TinyGo (the shipping binary today) |

`make build` produces both binaries: the shipping `jennifer` binary
(TinyGo, `meta.BUILD == "tinygo"`) and `jennifer-go` (standard Go,
`meta.BUILD == "go"`). `make build-tinygo` and `make build-go`
produce only one each. `go run` against the source also reports
`"go"`. If a future alternative compiler shows up, its identifier
passes through directly rather than being normalised - so the
constant always reports honestly
what built the binary.

## Build flow

`meta.VERSION` is set at build time by a small codegen step.


The Makefile runs `scripts/gen-version.sh` before `tinygo build` /
`go build`, writing a generated `internal/version/version_gen.go` whose
`init()` assigns `version.Version` to the string from
`scripts/version.sh`. The `meta` library then mirrors that into the
interpreter as the `meta.VERSION` constant.

You don't need to run the codegen step manually if you build via `make
build` (TinyGo) or `make build-go` (Go). A bare `go test ./...` skips
codegen and uses the default `"dev"` baked into `version.go`.

Codegen rather than `go build -ldflags -X` is used because TinyGo 0.41
silently ignores `-X`. The generated file is `.gitignore`d.

## Roadmap

`meta` is a new library and intentionally small. Future candidates if
they earn their slot: build time, git SHA (separated from the version
string), REPL-vs-script mode, runtime GC stats, scheduler diagnostics.
The library exists in part to give those a natural home when they
land.

See also: [os.md](os.md), [index.md](index.md).
