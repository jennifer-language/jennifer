#!/usr/bin/env bash
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# publish-git.sh - sync the jennifer-git recipe from the repo, refresh its -git
# pkgver from upstream, and publish it to the AUR. Copy this into your
# jennifer-git AUR clone as `publish.sh` and run it there (see RELEASE.md). It
# copies PKGBUILD + jennifer.install from the sibling jennifer-lang checkout,
# clones the source so pkgver() can run, regenerates .SRCINFO, then commits and
# pushes PKGBUILD + jennifer.install + .SRCINFO. When neither the recipe nor the
# computed pkgver has changed it is a clean no-op.

set -euo pipefail

# Always work from this script's directory (the AUR clone).
cd "$(dirname "$(readlink -f "$0")")"

# Refuse to run outside an AUR clone - e.g. if run in place from the
# jennifer-lang repo, where a push would go to the wrong remote.
origin_url="$(git remote get-url origin 2>/dev/null || true)"
if [[ "$origin_url" != *aur.archlinux.org* ]]; then
    echo "error: run this inside the jennifer-git AUR clone (origin is '${origin_url:-none}')." >&2
    echo "       copy packaging/arch/publish-git.sh into your jennifer-git clone as publish.sh." >&2
    exit 1
fi

if [[ ! -f PKGBUILD ]]; then
    echo "error: no PKGBUILD in $(pwd)" >&2
    exit 1
fi

# Keep the PKGBUILD in sync with its canonical copy in the jennifer-lang repo
# (a sibling checkout), when that checkout is present. Unlike the -bin flow -
# which fetches a release-generated PKGBUILD-bin with substituted hashes - the
# -git recipe is static (pkgver() computes the version at build time), so there
# is nothing release-specific to fetch; the recipe just needs to travel from the
# repo to the AUR. Copying it here is what makes an in-tree PKGBUILD-git edit
# actually reach users, instead of silently staying on the last-published recipe.
src_pkgbuild="../jennifer-lang/packaging/arch/PKGBUILD-git"
if [[ -f "$src_pkgbuild" ]]; then
    echo "==> Syncing PKGBUILD from jennifer-lang (packaging/arch/PKGBUILD-git)..."
    cp "$src_pkgbuild" PKGBUILD
fi

# Keep the install hook in sync with its canonical copy in the jennifer-lang
# repo (a sibling checkout), when that checkout is present.
src_install="../jennifer-lang/packaging/arch/jennifer.install"
if [[ -f "$src_install" ]]; then
    echo "==> Syncing jennifer.install from jennifer-lang..."
    cp "$src_install" jennifer.install
fi

echo "==> Cloning upstream source and computing pkgver (makepkg -od)..."
# -o: download + extract only (no build); -d: skip the dependency check, so
# go / tinygo need not be installed just to run pkgver(). makepkg writes the
# computed pkgver back into PKGBUILD.
makepkg -od

pkgver="$(grep '^pkgver=' PKGBUILD | cut -d= -f2)"
echo "==> pkgver = ${pkgver}"

echo "==> Regenerating .SRCINFO..."
makepkg --printsrcinfo > .SRCINFO

# Stage only the recipe files - never src/ or built packages.
git add PKGBUILD jennifer.install .SRCINFO

if git diff --cached --quiet; then
    echo "==> Already current (no pkgver change); nothing to publish."
    exit 0
fi

echo "==> Committing and pushing to the AUR..."
git commit -m "Update to ${pkgver}"
git push origin HEAD:master

echo "==> Published jennifer-git ${pkgver} to the AUR."
