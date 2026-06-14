# TinyGo notes

The interpreter ships as a TinyGo binary named `jennifer`.
`make build` produces both binaries side by side:
the TinyGo `jennifer` (shipping) and the standard-Go `jennifer-go`
(dev/full-feature). To produce only one, use `make build-tinygo`
or `make build-go`. All three regenerate the version file before
compiling.

A few constraints shape the implementation:

- **No `reflect`-heavy code.** Tagged-union `Value` instead of interfaces
  with type assertions in hot paths.
- **No `text/template`, no goroutines in the interpreter core.** Not
  needed yet, but worth not introducing accidentally.
- **No `encoding/json` for in-binary serialization.** The reflect-based
  marshaler is fragile under TinyGo, so the AST JSON emitter is
  hand-rolled (see [CLI > Inspection](cli.md#inspection-tokens-and-ast)).
- **No `-ldflags "-X package.var=value"`.** TinyGo 0.41 silently ignores
  the `-X` directive. Use the codegen path
  (`scripts/gen-version.sh` -> `internal/version/version_gen.go`) for
  build-time string injection. See [CLI > Version injection](cli.md#version-injection).
- **No hard dependencies on a hosted runtime.** A long-term goal is to
  embed the interpreter into the **McFly OS** kernel (also TinyGo), so
  ambient stdin, dynamic linking, and the like should not be assumed.
- **`testing` runs under regular `go test`.** TinyGo's `testing` support
  is partial; we develop and verify with `go test ./...`.

Verify both builds after non-trivial changes:

```sh
make build
./jennifer run examples/hello.j     # TinyGo binary
./jennifer-go run examples/hello.j  # Go binary (full host features)
```

## TinyGo restrictions

A few standard-library features depend on TinyGo runtime support
that isn't there yet. When called from the `jennifer` (TinyGo)
binary they error with a friendly Jennifer-level message pointing
the user at `jennifer-go`. The standard-Go binary always supports
the full surface.

| Library | Affected names                                        | What happens on TinyGo                                                                       |
| ------- | ----------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| `os`    | `os.run`, `os.spawn`, `os.wait`, `os.poll`, `os.kill` | Runtime error pointing at `jennifer-go`. TinyGo's `os/exec` syscalls aren't implemented yet. |

The constants and the env / argv / flag helpers in `os`
(`os.PLATFORM`, `os.ARCH`, `os.EOL`, `os.DIRSEP`, `os.PATHSEP`,
`os.ARGS`, `os.getEnv`, `os.hasFlag`, `os.flag`) all work fully on
both binaries. Every other shipped library (`io`, `convert`, `math`,
`strings`, `lists`, `maps`, `meta`, `core`) has full TinyGo
support.

Future library work in `fs` (M16.1) and `net` (M16.2) will hit the
same boundary and will land with the same friendly-message pattern.
The table will grow as those ship.
