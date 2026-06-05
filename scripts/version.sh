#!/bin/sh
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# Computes the project version from git tags. Output goes to stdout.
#
#   - If HEAD is exactly on a semver tag: prints the tag (e.g. "0.4.0").
#   - If HEAD is N commits past the most recent tag:
#       prints "<tag>-dev+<N>.<shortsha>" (e.g. "0.4.0-dev+2.1023204").
#   - If no tags exist yet but the repo has commits:
#       prints "0.0.0-dev+<N>.<shortsha>".
#   - If git isn't available or this isn't a repo: prints "dev".
#
# Consumed by the Makefile to (re)generate internal/version/version_gen.go
# before each build. We don't use `go build -ldflags -X` because TinyGo
# 0.41 silently ignores the -X directive; codegen works on both toolchains.

set -eu

if ! command -v git >/dev/null 2>&1; then
    echo "dev"
    exit 0
fi
if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "dev"
    exit 0
fi

# `git describe --tags --long --always` formats:
#   - "<tag>-<N>-g<shortsha>"  when at least one tag exists
#   - "<shortsha>"             when no tags exist yet
desc=$(git describe --tags --long --always 2>/dev/null || echo "")
if [ -z "$desc" ]; then
    echo "dev"
    exit 0
fi

case "$desc" in
    *-*-g*)
        # Has a tag. Split off the trailing "-<N>-g<sha>" suffix.
        sha=${desc##*-g}
        without_sha=${desc%-g*}
        n=${without_sha##*-}
        tag=${without_sha%-*}
        if [ "$n" = "0" ]; then
            echo "$tag"
        else
            echo "${tag}-dev+${n}.${sha}"
        fi
        ;;
    *)
        # No tag in history; describe printed just the short SHA.
        n=$(git rev-list --count HEAD 2>/dev/null || echo "0")
        echo "0.0.0-dev+${n}.${desc}"
        ;;
esac
