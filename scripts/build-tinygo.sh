#!/bin/sh
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# Build jennifer-tiny (the TinyGo binary), or skip gracefully when the available
# TinyGo/Go toolchain cannot build it - so a plain `make build` still succeeds on
# a host where only the standard `jennifer` is buildable. On Arch's Go 1.27 no
# TinyGo release yet produces a working jennifer-tiny: an older TinyGo rejects
# Go 1.27 at its version check ("requires go version 1.19 through 1.26, got
# go1.27"), and TinyGo 0.42 accepts Go 1.27 but fails to link with
# `duplicate symbol: tinygo_task_exit` (tinygo#5652).
#
# The skip is gated on a PROBE build of a trivial goroutine program with the same
# flags, NOT on swallowing the real build's errors. Both failure modes above
# break that probe too, whereas a TinyGo-cleanliness regression in Jennifer's own
# code does not (the trivial program still builds) - so a genuine regression
# still fails the build loudly, and CI (a capable TinyGo 0.41.1 + Go 1.26) keeps
# enforcing TinyGo-cleanliness through this same script.
#
# Usage:
#   scripts/build-tinygo.sh <output> <target-pkg> [tinygo build flags...]
#
# Exit status:
#   0   built <output>, OR skipped because the toolchain cannot build it
#   !=0 the toolchain is capable but the real build failed (a real regression)

set -eu

if [ $# -lt 2 ]; then
    echo "usage: $0 <output> <target-pkg> [tinygo build flags...]" >&2
    exit 2
fi

OUT="$1"
shift
PKG="$1"
shift
# Anything left in "$@" is tinygo build flags, e.g. -scheduler=tasks -stack-size=4mb.

if ! command -v tinygo >/dev/null 2>&1; then
    echo "build-tinygo: tinygo not found on PATH; skipping $OUT (the standard 'jennifer' is unaffected)." >&2
    exit 0
fi

# Probe: can this toolchain build a trivial goroutine program with these flags? A
# version-range rejection and the tasks-scheduler link regression (tinygo#5652)
# both fail here; a Jennifer-specific TinyGo-cleanliness break does not, so that
# still fails the real build below.
PROBE_DIR="$(mktemp -d)"
trap 'rm -rf "$PROBE_DIR"' EXIT
cat > "$PROBE_DIR/main.go" <<'EOF'
package main

func main() {
	done := make(chan int, 1)
	go func() { done <- 1 }()
	<-done
}
EOF

if ! probe_err="$(tinygo build "$@" -o "$PROBE_DIR/probe" "$PROBE_DIR/main.go" 2>&1)"; then
    {
        echo "build-tinygo: this TinyGo/Go toolchain cannot build $OUT; skipping it."
        echo "  $(tinygo version 2>/dev/null || echo 'tinygo version unknown')"
        echo "  No TinyGo release yet builds a working jennifer-tiny on Go 1.27 (tinygo#5652);"
        echo "  the standard 'jennifer' binary is unaffected. Probe error:"
        printf '%s\n' "$probe_err" | sed 's/^/    /'
    } >&2
    exit 0
fi

# Capable toolchain: build for real. A failure here is a genuine TinyGo-cleanliness
# regression in Jennifer and must fail the build (set -e propagates the status).
echo "build-tinygo: toolchain can build TinyGo; building $OUT" >&2
tinygo build "$@" -o "$OUT" "$PKG"
