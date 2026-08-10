# `meta` - interpreter identity and reflection

Enable with `use meta;`. Holds facts about the running Jennifer
interpreter itself - the build version, which Go toolchain compiled
it, and similar information that programs typically log for bug
reports, embed in error messages, or branch on for build-specific
behaviour - plus a small **reflection** surface for invoking a
top-level method by a runtime string name (`meta.call` / `meta.defined`
and their entry-program siblings `meta.callMain` / `meta.definedMain`).

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
| `meta.VERSION`   | string | The interpreter's build version. See format below.                 |
| `meta.BUILD`     | string | Which Go toolchain compiled the interpreter: `"go"` or `"tinygo"`. |
| `meta.SYSMODDIR` | string | The resolved system module directory (where bare `import`s look). Resolved from `--sysmoddir` > `JENNIFER_SYSMODDIR` > the compile-time default; `jennifer version -v` shows the layers. |
| `meta.CAPABILITIES` | list of string | The host capabilities this build was compiled with, sorted: `["exec", "net", "sql"]` on the standard `jennifer`, `[]` on `jennifer-tiny`. The same set the `# pragma-jennifer-capability:` header is checked against. |

## Capability query

| Signature | Returns | Description |
| --------- | ------- | ----------- |
| `meta.hasCapability(name)` | bool | Whether this build includes the named host capability (`"net"` for TCP/UDP/TLS/DNS and the network-backed libraries, `"exec"` for `os/exec` external processes). Lets a script branch instead of failing in a stub. |

```jennifer
use meta;
use io;
if (meta.hasCapability("net")) {
    io.printf("networking available\n");
}
io.printf("built with: %v\n", meta.CAPABILITIES);
```

A module can *require* a capability declaratively with a header pragma
(`# pragma-jennifer-capability: net`), which refuses the module at read time on a
build that lacks it - see [imports](../user-guide/imports.md#requirement-header).

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

| Value      | Meaning                                                            |
| ---------- | ------------------------------------------------------------------ |
| `"go"`     | Built with the standard Go toolchain (`gc`) - the default `jennifer` binary |
| `"tinygo"` | Built with TinyGo - the constrained `jennifer-tiny` binary         |

`make build` produces both binaries: the default `jennifer` (standard
Go, `meta.BUILD == "go"`) and `jennifer-tiny` (TinyGo, `meta.BUILD ==
"tinygo"`). `make build-go` and `make build-tinygo` produce only one
each. `go run` against the source also reports `"go"`. If a future
alternative compiler shows up, its identifier passes through directly
rather than being normalised - so the constant always reports honestly
what built the binary.

## Reflection - calling a method by name

Jennifer has no first-class functions: a bare call `greet(...)` is
resolved at compile time, so you cannot dispatch on a name computed at
runtime. `meta.call` closes that gap - it invokes a top-level user
method by a runtime string, the general form of what `testing.run` does
for tests.

| Call | Returns | |
| ---- | ------- | - |
| `meta.call(name, args...)` | the method's return value | Invoke the method `name` with the given arguments (arity + declared types checked, as at a normal call site). |
| `meta.defined(name)` | bool | Whether a method `name` exists - validate a name before calling it. |
| `meta.callMain(name, args...)` | the method's return value | Like `call`, but resolves against the **entry program's** methods. |
| `meta.definedMain(name)` | bool | Like `defined`, against the entry program. |

```jennifer
use io;
use meta;

func greet(name as string) { return "hi " + $name; }

io.printf("%s\n", meta.call("greet", "ada"));   # hi ada
io.printf("%t\n", meta.defined("nope"));        # false
```

Unlike `testing.run`, `meta.call` is transparent: it does not catch
`exit`, and every sentinel a normal call can raise - a runtime error, a
thrown `Error`, `exit` - propagates to the caller, catchable with
`try` / `catch`.

### Security: never dispatch on untrusted input

`meta.call` / `meta.callMain` invoke **any** top-level method by name, with no
built-in restriction. Passing a name that comes from request data, a URL, a
message body, or any other untrusted source is a remote-code-execution hole - it
reaches `deleteAllUsers`, `runMigration`, or any internal helper you never meant
to expose. `meta.defined` answers "does it exist", which is **not** an
authorization check.

Always match untrusted input against a **program-defined allowlist** first, and
call only a name that is on it:

```jennifer
use meta;
use lists;

# The only names an outside caller may reach.
def const ACTIONS as list of string init ["listItems", "showItem"];

func handleAction(name as string, arg as string) {
    if (not lists.contains(ACTIONS, $name)) {
        throw Error{kind: "app", message: "unknown action: " + $name, file: "", line: 0, col: 0};
    }
    return meta.call($name, $arg);      # safe: $name is proven to be on the allowlist
}
```

A framework that maps requests to handlers must register the handler names up
front and dispatch only registered ones - which is exactly what the
[`web`](../modules/web.md) module does (routes are declared with `web.get` /
`web.post`; the request's own path never becomes a method name). Follow the same
discipline whenever you build dispatch on `meta.call`.

### `callMain` / `definedMain` - reaching the entry program

Modules run on isolated sub-interpreters, so a `meta.call` *inside a
module* reaches that module's own methods, not the program that imported
it. The `*Main` variants cross that boundary: they resolve against the
**entry program's** top-level methods. This is what lets a framework
module dispatch to handlers the application defined - the
[`web`](../modules/web.md) module registers routes against handler names
and calls them with `meta.callMain`. Called from the entry program
itself, `callMain` / `definedMain` coincide with the plain forms (the
entry program is its own host). Struct arguments are re-tagged across the
boundary automatically, so a module can pass one of its own struct values
(e.g. a `web.Context`) to an entry-program handler that declares it.

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
