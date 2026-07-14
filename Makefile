# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

# Build pipeline:
#   1. scripts/gen-version.sh writes internal/version/version_gen.go from the
#      current git state (gitignored; regenerated on every build).
#   2. The toolchain compiles the binary picking up that file via an init()
#      that sets Version.
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
# We use codegen rather than `-ldflags -X` because TinyGo 0.41 silently
# ignores -X. Codegen works identically on both toolchains.

.PHONY: build build-tinygo build-go test clean version gen-version

# Default: build both binaries so the user always has both variants
# side by side for local A/B comparison.
build: build-go build-tinygo

# Extra `go build` flags. Empty by default so a dev `make build` keeps
# symbols and paths for debugging; a packager passes `-trimpath` (and any
# strip flags) to drop the local build path from the released binary, e.g.
# `make build GOBUILDFLAGS=-trimpath` (the AUR -git PKGBUILD does this).
GOBUILDFLAGS ?=

# Standard Go toolchain binary - the default `jennifer`. Fast iteration,
# full host-feature support.
build-go: gen-version
	go build $(GOBUILDFLAGS) -o jennifer ./cmd/jennifer

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
# -stack-size=2mb: TinyGo's default goroutine stack (~8KB) overflows on
# Jennifer's tree-walking recursive evaluator. Each Jennifer-level call
# adds many Go-stack frames (execBlock + evalCall + evalExpr + ...), so
# even the serial fib(23) in examples/benchmark.j needs hundreds of KB -
# and at 1MB it sat right at the edge (fib(23) fit bare but overflowed
# nested in one more call frame, e.g. inside benchFib / an io.printf arg).
# 2MB clears the whole example suite (serial + parallel fib) with headroom;
# bump it further if a future workload needs deeper recursion.
build-tinygo: gen-version
	tinygo build -o jennifer-tiny -scheduler=tasks -stack-size=2mb ./cmd/jennifer

test:
	go test ./...

clean:
	rm -f jennifer jennifer-tiny internal/version/version_gen.go

# Regenerate the version-init file from git state. Always runs; the .PHONY
# declaration above ensures make doesn't skip it on rebuild.
gen-version:
	@sh scripts/gen-version.sh

# Print the version string that the next build would embed.
version:
	@sh scripts/version.sh
