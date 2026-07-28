#!/usr/bin/env bash
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# publish.sh - publish the Jennifer formula to the Homebrew tap from the latest
# GitHub release. Copy this into your `homebrew-tap` clone and run it there (see
# packaging/homebrew/README.md). It resolves the newest release tag, computes the
# source-tarball sha256, renders Formula/jennifer.rb from the canonical template,
# then commits and pushes. A clean no-op if unchanged.

set -euo pipefail

# Always work from this script's directory (the tap clone).
cd "$(dirname "$(readlink -f "$0")")"

# Refuse to run outside a homebrew tap clone - e.g. if run in place from the
# jennifer-lang repo, where a push would go to the wrong remote.
origin_url="$(git remote get-url origin 2>/dev/null || true)"
if [[ "$origin_url" != *homebrew-* ]]; then
    echo "error: run this inside your homebrew-tap clone (origin is '${origin_url:-none}')." >&2
    echo "       copy packaging/homebrew/publish.sh into the tap clone and run it there." >&2
    exit 1
fi

repo="jennifer-language/jennifer"

echo "==> Resolving the newest release (including pre-releases)..."
# Capture first, then match from a here-string: piping curl into `grep -m1`
# closes the pipe early and trips pipefail. The /releases/latest/ alias excludes
# pre-releases (every 0.x tag is a pre-release), so use the newest-first list.
releases_json="$(curl -fsSL "https://api.github.com/repos/${repo}/releases?per_page=1")"
tag="$(grep -m1 '"tag_name"' <<<"$releases_json" | cut -d'"' -f4)"
if [[ -z "$tag" ]]; then
    echo "error: could not determine the newest release tag from the GitHub API" >&2
    exit 1
fi
echo "    newest release: ${tag}"

url="https://github.com/${repo}/archive/refs/tags/${tag}.tar.gz"

echo "==> Downloading source tarball and computing sha256..."
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fsSL -o "$tmp/src.tar.gz" "$url"
if command -v sha256sum >/dev/null 2>&1; then
    sha="$(sha256sum "$tmp/src.tar.gz" | cut -d' ' -f1)"
else
    sha="$(shasum -a 256 "$tmp/src.tar.gz" | cut -d' ' -f1)"
fi
echo "    sha256: ${sha}"

# The canonical formula (template) lives in the jennifer-lang repo. Prefer a
# sibling checkout; otherwise fetch it from the release tag.
template=""
if [[ -f ../jennifer-lang/packaging/homebrew/jennifer.rb ]]; then
    template="../jennifer-lang/packaging/homebrew/jennifer.rb"
    echo "==> Using template from sibling jennifer-lang checkout."
else
    echo "==> Fetching formula template from ${tag}..."
    curl -fsSL -o "$tmp/jennifer.rb" \
        "https://raw.githubusercontent.com/${repo}/${tag}/packaging/homebrew/jennifer.rb"
    template="$tmp/jennifer.rb"
fi

mkdir -p Formula
# Render the real url + sha256 into the formula. The url line drives the
# Homebrew-inferred version, so nothing else needs substituting.
sed -E \
    -e "s#^  url \".*\"#  url \"${url}\"#" \
    -e "s#^  sha256 \".*\"#  sha256 \"${sha}\"#" \
    "$template" > Formula/jennifer.rb

# Guard against a bad render before committing.
if ! grep -q "refs/tags/${tag}.tar.gz" Formula/jennifer.rb; then
    echo "error: rendered Formula/jennifer.rb does not carry the ${tag} url" >&2
    exit 1
fi
if grep -q 'sha256 "0000000000' Formula/jennifer.rb; then
    echo "error: rendered Formula/jennifer.rb still has the placeholder sha256" >&2
    exit 1
fi

if git diff --quiet -- Formula/jennifer.rb; then
    echo "==> Formula already up to date for ${tag}; nothing to do."
    exit 0
fi

echo "==> Committing and pushing..."
git add Formula/jennifer.rb
git commit -m "jennifer ${tag}"
git push
echo "==> Done. Users can now: brew install jennifer-language/tap/jennifer"
