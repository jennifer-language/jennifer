# Container image

A multi-stage [`Dockerfile`](Dockerfile) that packages the Jennifer interpreter
as an OCI image. The default `jennifer` binary is the full host-feature build
(net, TLS, `os/exec`), and `jennifer serve` / the `web` + `httpd` libraries make
the image a web-app runtime as well, so a container is a natural way to run
Jennifer in CI, a Kubernetes job, or as a `FROM` base.

Modules install to `/usr/share/jennifer/modules`, which is Jennifer's
compile-default module directory, so a bare `import "name.j";` resolves inside
the image with **no environment variable**.

## Variants and tags

Published to GHCR on each release tag by
[`.github/workflows/docker.yml`](../../.github/workflows/docker.yml), multi-arch
(`linux/amd64` + `linux/arm64`):

| Tag | Stage | Base | Notes |
|-----|-------|------|-------|
| `ghcr.io/jennifer-language/jennifer:latest` | `slim` | `debian:trixie-slim` | Default. Full host features. |
| `ghcr.io/jennifer-language/jennifer:<version>` | `slim` | `debian:trixie-slim` | Pinned version. |
| `ghcr.io/jennifer-language/jennifer:<major>.<minor>` | `slim` | `debian:trixie-slim` | Floating minor. |
| `ghcr.io/jennifer-language/jennifer:static` | `static` | `distroless/static` | Minimal, no shell. |
| `ghcr.io/jennifer-language/jennifer:<version>-static` | `static` | `distroless/static` | Pinned static. |
| `ghcr.io/jennifer-language/jennifer:dev` | `slim` | `debian:trixie-slim` | Latest commit on `main` (also `:main`). Moving tag, rebuilt every push; `meta.VERSION` is `dev`. |

The release tags above (`:latest`, `:<version>`, `:static`, ...) are built on a
version tag; `:dev` / `:main` are built on every push to `main` and overwrite the
previous one, for trying the bleeding edge. `:dev` is slim-only (no static
per-commit build).

Both variants ship both binaries (`/usr/bin/jennifer` and
`/usr/bin/jennifer-tiny`), the system modules, and run as a non-root user with
`WORKDIR /work`.

**`slim` vs `static`.** The `slim` image has a full Debian userland: `os.run` /
`os.spawn` can shell out and `ca-certificates` is present so `net` / `http` TLS
verifies peers. The `static` image is distroless (~15-25MB, minimal attack
surface) but has **no `/bin/sh`**, so `os.run` of external programs raises a
runtime error there - use it for pure-interpreter or web-serving workloads.

## Run

```sh
# Run a script from the current directory (mounted at /work).
docker run --rm -v "$PWD:/work" ghcr.io/jennifer-language/jennifer run app.j

# Pipe a program on stdin.
echo 'use io; io.printf("hi\n");' | docker run --rm -i ghcr.io/jennifer-language/jennifer run -

# Interactive REPL (the default command).
docker run --rm -it ghcr.io/jennifer-language/jennifer

# Serve a web app (the port is whatever the program passes to web.run / httpd).
docker run --rm -p 8080:8080 -v "$PWD:/work" ghcr.io/jennifer-language/jennifer run server.j
```

`ENTRYPOINT` is `jennifer`, so the arguments after the image name are Jennifer's
own subcommands (`run`, `repl`, `version`, ...). The default command is `repl`.

## Build locally

Build context is the **repo root** (the Dockerfile needs the Go/TinyGo sources).
Requires Docker with [Buildx](https://docs.docker.com/go/buildx/) (BuildKit) for
the `$BUILDPLATFORM` cross-compile:

```sh
# Default slim image for the host arch.
docker buildx build --load -f packaging/docker/Dockerfile --target slim \
  --build-arg VERSION=dev -t jennifer:dev .

# The minimal static variant.
docker buildx build --load -f packaging/docker/Dockerfile --target static \
  --build-arg VERSION=dev -t jennifer:dev-static .

# Multi-arch (produces a manifest; push instead of --load).
docker buildx build --platform linux/amd64,linux/arm64 \
  -f packaging/docker/Dockerfile --target slim \
  --build-arg VERSION=dev -t youruser/jennifer:dev --push .
```

Without a `VERSION` build arg the binaries report `dev`. The build stage
cross-compiles both binaries on the build platform (Go and TinyGo both honour
`GOARCH`), so multi-arch needs no emulation to compile - only the `slim` stage's
small `apt-get` step runs under QEMU for a foreign arch.
