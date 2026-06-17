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
#   - `jennifer`     - TinyGo build. The shipping interpreter; small binary,
#                      embeddable; some host features missing (today: no
#                      `os/exec`, so `os.run` / `os.spawn` etc. return a
#                      friendly limitation error).
#   - `jennifer-go`  - standard Go toolchain. Full host-feature surface;
#                      what you reach for during development and on any
#                      machine with the host runtime available.
#
# The `build-tinygo` and `build-go` targets each produce one of the two.
#
# We use codegen rather than `-ldflags -X` because TinyGo 0.41 silently
# ignores -X. Codegen works identically on both toolchains.

.PHONY: build build-tinygo build-go test clean version gen-version

# Default: build both binaries so the user always has a Go-toolchain binary
# (`jennifer-go`) alongside the TinyGo shipping binary (`jennifer`).
build: build-tinygo build-go

# TinyGo shipping binary. Embeddable; the smaller of the two.
#
# -stack-size=1mb: TinyGo's default goroutine stack (~8KB) overflows on
# Jennifer's tree-walking recursive evaluator. Each Jennifer-level call
# adds many Go-stack frames (execBlock + evalCall + evalExpr + ...), so
# a depth-24 fib spawn already needs hundreds of KB. 1MB matches Go's
# initial-stack ballpark and runs the full example suite (including the
# parallel fib block in examples/benchmark.j) without segfaulting; bump
# it if a future workload needs deeper recursion.
build-tinygo: gen-version
	tinygo build -o jennifer -stack-size=1mb ./cmd/jennifer

# Standard Go toolchain binary. Fast iteration, full host-feature support.
# Named with a `-go` suffix so `jennifer` always refers to the shipping
# (TinyGo) build and a side-by-side install never overwrites it.
build-go: gen-version
	go build -o jennifer-go ./cmd/jennifer

test:
	go test ./...

clean:
	rm -f jennifer jennifer-go internal/version/version_gen.go

# Regenerate the version-init file from git state. Always runs; the .PHONY
# declaration above ensures make doesn't skip it on rebuild.
gen-version:
	@sh scripts/gen-version.sh

# Print the version string that the next build would embed.
version:
	@sh scripts/version.sh
