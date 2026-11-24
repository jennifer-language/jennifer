# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

# Build pipeline:
#   1. scripts/gen-version.sh writes internal/version/version_gen.go from the
#      current git state and PREFIX (gitignored; regenerated on every build).
#   2. The toolchain compiles the binary picking up that file via an init()
#      that sets Version and Prefix.
#
# `make build` produces both binaries because Jennifer has a deliberate
# two-binary story:
#   - `jennifer`       - standard Go toolchain. Full host-feature surface;
#                        the default binary users install and reach for.
#   - `jennifer-tiny`  - TinyGo build. Small binary, embeddable; some host
#                        features missing (no `os/exec`, no netdev stack)
#                        surface as friendly "not available in
#                        jennifer-tiny; use the plain jennifer binary"
#                        runtime errors.
#
# The `build-go` and `build-tinygo` targets each produce one of the two.
# The language still stays TinyGo-clean (both binaries built in CI); the
# name flip only reflects which one distributors ship as default.
#
# We use codegen rather than `-ldflags -X` because TinyGo before 0.43.0
# silently ignores -X, and 0.41.1 is still a supported (CI-tested) toolchain.
# Codegen works identically on every toolchain.

.PHONY: build build-tinygo build-tinygo-dev build-go test clean version gen-version docker

# Default: build both binaries so the user always has both variants
# side by side for local A/B comparison.
build: build-go build-tinygo

# Extra `go build` flags. Empty by default so a dev `make build` keeps
# symbols and paths for debugging; a packager passes `-trimpath` (and any
# strip flags) to drop the local build path from the released binary, e.g.
# `make build GOBUILDFLAGS=-trimpath` (the AUR -git PKGBUILD does this).
GOBUILDFLAGS ?=

# A pure-Go build, as the release archives, packages and images are: no cgo,
# so the binary is static and needs no system library, not even libc (Go's own
# DNS resolver replaces the libc one). Without it a toolchain whose default is
# cgo-on links libc and libresolv, and the AUR -git package ships a dynamic
# binary. `make build CGO_ENABLED=1` restores the cgo build.
CGO_ENABLED ?= 0

# Standard Go toolchain binary - the default `jennifer`. Fast iteration,
# full host-feature support.
build-go: gen-version
	CGO_ENABLED=$(CGO_ENABLED) go build $(GOBUILDFLAGS) -o jennifer ./cmd/jennifer

# TinyGo constrained binary. Embeddable; smaller; missing os/exec + net.
#
# -scheduler=tasks: pin the cooperative single-thread scheduler. `spawn`
# gives concurrency without multi-core parallelism under jennifer-tiny;
# this is deliberate. An unpinned build picks up a threads-capable
# default that segfaults on recursive spawn bodies (the parallel fib
# block in examples/benchmark.j), because -stack-size below does not
# govern OS-thread stacks the way it governs the cooperative scheduler's
# goroutine stacks. Real multi-core on jennifer-tiny is a separate piece
# of work, not a default flip.
#
# -stack-size=4mb: TinyGo's default goroutine stack (~8KB) overflows on
# Jennifer's tree-walking recursive evaluator. Each Jennifer-level call
# adds many Go-stack frames (execBlock + evalCall + evalExpr + ...), so
# even the serial fib(23) in examples/benchmark.j needs hundreds of KB -
# and at 1MB it sat right at the edge (fib(23) fit bare but overflowed
# nested in one more call frame, e.g. inside benchFib / an io.printf arg).
# 4MB clears the whole example suite (serial + parallel fib) with wide
# headroom and sets the ceiling the catchable call-depth cap sits below
# (internal/limits.MaxCallDepth = 120 on tinygo, counting calls, blocks and
# expression descents) and the stack-bytes guard (maxStackBytes = 3 MB in
# internal/interpreter/stackguard_tiny.go, which also assumes a 4 MB stack):
# a heavy recursive body can pass 4 MB below 120 units, so the bytes guard is
# what trips first there (M28.72). Bump them together if a future
# workload needs deeper recursion. Never shrink it without re-measuring and
# lowering the caps: TinyGo does not reliably trap an overflow, so past the
# floor a run can continue on corrupted values instead of crashing (at 1mb a
# map-building recursion did from 10 levels on). Smaller stacks would also make
# many live tasks cheaper to start (docs/technical/tinygo.md), but not safely.
#
# The build goes through scripts/build-tinygo.sh, which SKIPS gracefully (exit 0)
# when the toolchain cannot build jennifer-tiny at all - so `make build` still
# succeeds on a host where only the standard binary is buildable (e.g. Go 1.27
# with a TinyGo older than 0.43.0, which cannot link it: tinygo#5652). It gates the skip on a
# probe build, so a real TinyGo-cleanliness regression in Jennifer still fails.
# The scheduler/stack flags stay here as the single source (TINYGO_FLAGS, shared
# with build-tinygo-dev); the script forwards them to both the probe and the
# real build, and applies the same two values itself when a caller names
# neither (scripts/build-tinygo.sh keeps its defaults equal to these).
TINYGO_FLAGS = -scheduler=tasks -stack-size=4mb
build-tinygo: gen-version
	sh scripts/build-tinygo.sh jennifer-tiny ./cmd/jennifer $(TINYGO_FLAGS)

# jennifer-tiny-dev: the same TinyGo build with the `tinygo_devtools` tag, which
# compiles in the development subcommands (tokens, ast, fmt, lint, profile,
# test) that jennifer-tiny stubs out. Same flags, so the same runtime, call-depth
# caps and stack; only the subcommands differ. For CI (a deck's test overlays
# run on the constrained runtime) and for profiling a TinyGo-only issue in situ;
# the container images carry it, the release archives and packages do not. Not
# part of `make build`.
build-tinygo-dev: gen-version
	sh scripts/build-tinygo.sh jennifer-tiny-dev ./cmd/jennifer $(TINYGO_FLAGS) -tags tinygo_devtools

test:
	go test ./...

clean:
	rm -f jennifer jennifer-tiny jennifer-tiny-dev internal/version/version_gen.go

# Install prefix the binaries are built for (M29.4): the system module
# directory defaults to $(PREFIX)/share/jennifer/modules, the docs to
# $(PREFIX)/share/doc/jennifer, and the resolver looks for resolv.conf / hosts
# in $(PREFIX)/etc before /etc (the FHS prefix /usr keeps them in /etc). Built
# into the binary by gen-version below, so `make build PREFIX=/opt/jennifer`
# finds its modules there without JENNIFER_SYSMODDIR. An absolute path; the
# packaging passes its own (Debian, Arch, OpenWrt, Docker: /usr).
PREFIX ?= /usr
# make exports a command-line variable to every recipe's environment, which
# expands it (a $(shell ...) in it would run); gen-version passes it instead.
unexport PREFIX

# Regenerate the version-init file from git state and PREFIX. Always runs; the
# .PHONY declaration above ensures make doesn't skip it on rebuild. PREFIX goes
# through the environment unexpanded ($(value ...)), so no character in it
# reaches the recipe's shell; gen-version.sh validates it.
gen-version: export JENNIFER_PREFIX = $(value PREFIX)
gen-version:
	@sh scripts/gen-version.sh

# Print the version string that the next build would embed.
version:
	@sh scripts/version.sh

# Build the container image locally (host arch) via Buildx / BuildKit, which the
# packaging/docker/Dockerfile needs for the $BUILDPLATFORM cross-compile. Builds
# the `slim` variant by default and tags it jennifer:<version>; override with
# `make docker DOCKER_TARGET=static DOCKER_TAG=my/jennifer:dev`. Multi-arch
# publishing is CI's job (.github/workflows/docker.yml).
DOCKER_TARGET ?= slim
DOCKER_TAG ?= jennifer:$(shell sh scripts/version.sh)
docker:
	docker buildx build --load -f packaging/docker/Dockerfile \
		--target $(DOCKER_TARGET) \
		--build-arg VERSION=$$(sh scripts/version.sh) \
		-t $(DOCKER_TAG) .
