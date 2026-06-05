# `meta` - interpreter introspection

Enable with `use meta;`. Exposes information about the running Jennifer
interpreter itself. Today there is one constant - `VERSION` - but the
library is the natural home for any future build/host/platform info.

```jennifer
use io;
use meta;

printf("running Jennifer %s\n", VERSION);
```

## Constants

| Name      | Kind     | Value                                                  |
|-----------|----------|--------------------------------------------------------|
| `VERSION` | string   | The interpreter's build version (see format below).    |

### `VERSION` string format

The build pipeline derives `VERSION` from `git describe --tags --long`:

| Repository state                          | `VERSION` value                       |
|-------------------------------------------|---------------------------------------|
| HEAD is exactly on a semver tag           | `"0.4.0"`                             |
| HEAD is N commits past the most recent tag | `"0.4.0-dev+2.1023204"`              |
| No tags exist yet                         | `"0.0.0-dev+<N>.<shortsha>"`          |
| Built without git (or outside a repo)     | `"dev"`                               |

The `dev+` prefix is intentional: any non-tagged build is a development
build, and the `N.shortsha` suffix lets you trace which commit produced it.

## Build flow

`VERSION` is set at build time by a small codegen step. The Makefile runs
`scripts/gen-version.sh` before `tinygo build` / `go build`, writing a
generated `internal/version/version_gen.go` whose `init()` assigns
`version.Version` to the string from `scripts/version.sh`. The `meta`
library then mirrors that into the interpreter as the `VERSION` constant.

You don't need to run the codegen step manually if you build via `make
build` (TinyGo) or `make build-go` (Go). A bare `go test ./...` skips
codegen and uses the default `"dev"` baked into `version.go`.

Codegen rather than `go build -ldflags -X` is used because TinyGo 0.41
silently ignores `-X`. The generated file is `.gitignore`d.
