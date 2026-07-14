#!/usr/bin/env bash
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# publish-bin.sh - publish jennifer-bin to the AUR from the latest GitHub
# release. Copy this into your jennifer-bin AUR clone as `publish.sh` and run it
# there (see RELEASE.md). It fetches the release-ready PKGBUILD-bin (real pkgver
# + sha256sums) from the newest release's assets, syncs the install hook,
# regenerates .SRCINFO, then commits and pushes. A clean no-op if unchanged.

set -euo pipefail

# Always work from this script's directory (the AUR clone).
cd "$(dirname "$(readlink -f "$0")")"

# Refuse to run outside an AUR clone - e.g. if run in place from the
# jennifer-lang repo, where a push would go to the wrong remote.
origin_url="$(git remote get-url origin 2>/dev/null || true)"
if [[ "$origin_url" != *aur.archlinux.org* ]]; then
    echo "error: run this inside the jennifer-bin AUR clone (origin is '${origin_url:-none}')." >&2
    echo "       copy packaging/arch/publish-bin.sh into your jennifer-bin clone as publish.sh." >&2
    exit 1
fi

repo="mplx/jennifer-lang"

echo "==> Resolving the newest release (including pre-releases)..."
# Capture the response first, then match from a here-string. Piping curl
# straight into `grep -m1` makes grep close the pipe early, which kills curl
# with a write error (23) and trips `pipefail`. We cannot use the
# /releases/latest/ alias either: every 0.x tag ships as a pre-release, and that
# alias excludes pre-releases (it 404s until 1.0.0). The API list is
# newest-first and includes pre-releases, so [0] is the current release.
releases_json="$(curl -fsSL "https://api.github.com/repos/${repo}/releases?per_page=1")"
tag="$(grep -m1 '"tag_name"' <<<"$releases_json" | cut -d'"' -f4)"
if [[ -z "$tag" ]]; then
    echo "error: could not determine the newest release tag from the GitHub API" >&2
    exit 1
fi

echo "==> Fetching PKGBUILD-bin from release ${tag}..."
# -f: fail loudly on an HTTP error; -L: follow redirects.
curl -fL -o PKGBUILD "https://github.com/${repo}/releases/download/${tag}/PKGBUILD-bin"

# Guard against a truncated / wrong download before doing anything with it.
if ! grep -q '^pkgname=jennifer-bin' PKGBUILD; then
    echo "error: fetched PKGBUILD is not jennifer-bin (bad download, or no release yet)" >&2
    exit 1
fi

# The install hook is not a release asset, so sync it from the sibling
# jennifer-lang checkout (its canonical home), when that checkout is present.
src_install="../jennifer-lang/packaging/arch/jennifer.install"
if [[ -f "$src_install" ]]; then
    echo "==> Syncing jennifer.install from jennifer-lang..."
    cp "$src_install" jennifer.install
fi
if [[ ! -f jennifer.install ]]; then
    echo "error: jennifer.install is missing and no ../jennifer-lang checkout to copy it from" >&2
    echo "       (the -bin PKGBUILD references install=jennifer.install)" >&2
    exit 1
fi

pkgver="$(grep '^pkgver=' PKGBUILD | cut -d= -f2)"
pkgrel="$(grep '^pkgrel=' PKGBUILD | cut -d= -f2)"
echo "==> jennifer-bin ${pkgver}-${pkgrel}"

echo "==> Regenerating .SRCINFO..."
makepkg --printsrcinfo > .SRCINFO

# Stage only the recipe files - never src/ or built packages.
git add PKGBUILD jennifer.install .SRCINFO

if git diff --cached --quiet; then
    echo "==> Already current (release unchanged); nothing to publish."
    exit 0
fi

echo "==> Committing and pushing to the AUR..."
git commit -m "Update to ${pkgver}-${pkgrel}"
# HEAD:master works whether the local branch is main or master, and creates the
# AUR's master branch on the first push.
git push origin HEAD:master

echo "==> Published jennifer-bin ${pkgver}-${pkgrel} to the AUR."
