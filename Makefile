# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

# Build pipeline:
#   1. scripts/gen-version.sh writes internal/version/version_gen.go from the
#      current git state (gitignored; regenerated on every build).
#   2. The toolchain (TinyGo by default, or Go via `make build-go`) compiles
#      the binary picking up that file via an init() that sets Version.
#
# We use codegen rather than `-ldflags -X` because TinyGo 0.41 silently
# ignores -X. Codegen works identically on both toolchains.

.PHONY: build build-go test clean version gen-version

build: gen-version
	tinygo build -o jennifer ./cmd/jennifer

# Convenience target for fast iteration during development.
build-go: gen-version
	go build -o jennifer ./cmd/jennifer

test:
	go test ./...

clean:
	rm -f jennifer internal/version/version_gen.go

# Regenerate the version-init file from git state. Always runs; the .PHONY
# declaration above ensures make doesn't skip it on rebuild.
gen-version:
	@sh scripts/gen-version.sh

# Print the version string that the next build would embed.
version:
	@sh scripts/version.sh
