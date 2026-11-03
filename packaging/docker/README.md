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
| `ghcr.io/jennifer-language/jennifer:alpine` | `alpine` | `alpine` | Small, with `/bin/sh` and `apk`: a build base. |
| `ghcr.io/jennifer-language/jennifer:<version>-alpine` | `alpine` | `alpine` | Pinned alpine. |
| `ghcr.io/jennifer-language/jennifer:static` | `static` | `distroless/static` | Minimal, no shell: a runtime image, not a build base. |
| `ghcr.io/jennifer-language/jennifer:<version>-static` | `static` | `distroless/static` | Pinned static. |
| `ghcr.io/jennifer-language/jennifer:dev` | `slim` | `debian:trixie-slim` | Latest commit on `main` (also `:main`). Moving tag, rebuilt every push; `meta.VERSION` is `dev`. |

The release tags above (`:latest`, `:<version>`, `:alpine`, `:static`, ...) are
built on a version tag; `:dev` / `:main` are built on every push to `main` and
overwrite the previous one, for trying the bleeding edge. `:dev` is slim-only (no
per-commit alpine or static build).

Every variant ships both binaries (`/usr/bin/jennifer` and
`/usr/bin/jennifer-tiny`), the system modules, and runs as a non-root user with
`WORKDIR /work`. The `slim` and `alpine` variants also bundle
[`jvc`](https://jvc.jennifer-lang.dev/), the deck manager (see below).

**Which variant.**

- **`slim`** has a full Debian userland: `os.run` / `os.spawn` can shell out,
  `ca-certificates` is present so `net` / `http` TLS verifies peers, and `apt-get`
  adds packages in a derived image. The default, and the most compatible base.
- **`alpine`** is the small image that is still a **build base**: Alpine's
  `/bin/sh` and `apk`, `ca-certificates`, and `jvc`, so a `FROM ...:alpine`
  Dockerfile can `RUN` shell steps and `apk add` what it needs (switch to
  `USER root` for that, then back to `USER jennifer`). The binaries are static,
  so musl does not affect them; a program that `os.run`s a tool gets the busybox
  version unless the image adds the full one.
- **`static`** is distroless (~15-25MB, minimal attack surface) and has **no
  `/bin/sh`** and no package manager. It is a **runtime image to copy an app
  into**, not a build base: a shell-form `RUN` step in a derived image fails, and
  `os.run` of external programs raises a runtime error. Use it for
  pure-interpreter or web-serving workloads, as the last stage of a multi-stage
  build.

A derived `static` image can still run Jennifer at build time, because the
**exec form** of `RUN` needs no shell:

```dockerfile
FROM ghcr.io/jennifer-language/jennifer:static
COPY . /work
RUN ["/usr/bin/jennifer", "run", "build.j"]
CMD ["run", "app.j"]
```

Anything else - packages, `git`, shell scripts, `jvc install` - belongs in a
`slim` or `alpine` build stage whose result is copied into `static` (see the
two-stage example under [Decks with `jvc`](#decks-with-jvc)), or use `alpine` as
the base outright.

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

## Decks with `jvc`

The `slim` and `alpine` images carry [`jvc`](https://jvc.jennifer-lang.dev/) at
`/usr/bin/jvc` (laid out as jvc's own `.deb` does: `/usr/share/jvc/`, minus its
test overlays; on `alpine`, `/usr/bin/jvc` is a small `sh` wrapper, since busybox
`env` has no `-S` for the launcher's shebang), so an app image can vendor its decks at build time. `jvc` is a
separate project, fetched at build time from its repository: the `JVC_REF` build
arg names the branch, tag, or commit, and is `main` until jvc publishes releases.
The image build runs `jvc version` and fails if the bundled copy cannot start.

```dockerfile
# Vendor the decks in the slim image, then run on the minimal one.
FROM ghcr.io/jennifer-language/jennifer:latest AS deps
COPY --chown=jennifer:jennifer deck.toml camcorder.lock ./
RUN jvc install

FROM ghcr.io/jennifer-language/jennifer:static
COPY . /work
COPY --from=deps /work/vendor /work/vendor
CMD ["run", "app.j"]
```

From the command line, override the entrypoint:
`docker run --rm -v "$PWD:/work" --entrypoint jvc ghcr.io/jennifer-language/jennifer install`.

Installing decks from a registry needs only HTTPS. `git` is not in the image, so
a deck taken from a git remote (`[sources]` in `deck.toml`) needs an image that
adds it (`apt-get install git` as root in a derived `slim` image, `apk add git`
in a derived `alpine` one). The `static` image does not carry `jvc` (it has no
`/usr/bin/env` for the launcher's shebang); use the two-stage build above, with
`slim` or `alpine` as the first stage.

## Build locally

Build context is the **repo root** (the Dockerfile needs the Go/TinyGo sources).
Requires Docker with [Buildx](https://docs.docker.com/go/buildx/) (BuildKit) for
the `$BUILDPLATFORM` cross-compile:

```sh
# Default slim image for the host arch.
docker buildx build --load -f packaging/docker/Dockerfile --target slim \
  --build-arg VERSION=dev -t jennifer:dev .

# The small alpine variant.
docker buildx build --load -f packaging/docker/Dockerfile --target alpine \
  --build-arg VERSION=dev -t jennifer:dev-alpine .

# The minimal static variant.
docker buildx build --load -f packaging/docker/Dockerfile --target static \
  --build-arg VERSION=dev -t jennifer:dev-static .

# Multi-arch (produces a manifest; push instead of --load).
docker buildx build --platform linux/amd64,linux/arm64 \
  -f packaging/docker/Dockerfile --target slim \
  --build-arg VERSION=dev -t youruser/jennifer:dev --push .
```

Without a `VERSION` build arg the binaries report `dev`; `--build-arg JVC_REF=<ref>`
bundles a different jvc branch, tag, or commit. The build stage
cross-compiles both binaries on the build platform (Go and TinyGo both honour
`GOARCH`), so multi-arch needs no emulation to compile - only the `slim` stage's
small `apt-get` step and the `alpine` stage's `apk` step run under QEMU for a
foreign arch.
