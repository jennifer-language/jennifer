#!/bin/sh
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# Assemble a `.deb` from already-built binaries. Run after
# `make build` has produced ./jennifer and ./jennifer-tiny (or from
# CI after the cross-compile step). The packaging metadata in
# packaging/debian/ is copied into the staging directory; the
# binaries, man pages, MIME definition, license, and changelog
# stub all land at the standard FHS locations.
#
# Usage:
#   scripts/build-deb.sh <version> <arch> <out-dir>
#
# Arguments:
#   <version>  - Debian-style version string (e.g. 0.14.0,
#                0.14.0~dev+5.g1023204). Project convention is bare
#                semver git tags (no `v` prefix), so the release
#                pipeline passes the tag straight through; dev
#                builds use a ~dev pre-release form so version-sort
#                stays correct.
#   <arch>     - Debian architecture: amd64 or arm64.
#   <out-dir>  - Directory to write the resulting .deb into.
#
# Expects in CWD:
#   ./jennifer       - standard Go binary (default, full-featured)
#   ./jennifer-tiny  - TinyGo binary (constrained; no os/exec, no net)
#
# Output:
#   <out-dir>/jennifer_<version>_<arch>.deb
#   <out-dir>/jennifer_<version>_<arch>.deb.sha256

set -eu

if [ $# -ne 3 ]; then
    echo "usage: $0 <version> <arch> <out-dir>" >&2
    exit 2
fi

VERSION="$1"
ARCH="$2"
OUT="$3"

case "$ARCH" in
    amd64|arm64) ;;
    *)
        echo "unsupported arch: $ARCH (want amd64 or arm64)" >&2
        exit 2
        ;;
esac

if ! command -v dpkg-deb >/dev/null 2>&1; then
    echo "dpkg-deb not found; install dpkg-dev (Debian/Ubuntu) or run on a Debian host" >&2
    exit 2
fi

for f in ./jennifer ./jennifer-tiny; do
    if [ ! -f "$f" ]; then
        echo "missing required input: $f" >&2
        exit 2
    fi
done

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PKG_DIR="$REPO_ROOT/packaging"

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

# FHS layout under the staging root.
mkdir -p "$STAGE/usr/bin"
mkdir -p "$STAGE/usr/share/man/man1"
mkdir -p "$STAGE/usr/share/mime/packages"
mkdir -p "$STAGE/usr/share/doc/jennifer"
mkdir -p "$STAGE/usr/share/vim/vimfiles/syntax"
mkdir -p "$STAGE/usr/share/vim/vimfiles/ftdetect"
mkdir -p "$STAGE/usr/share/nvim/site/syntax"
mkdir -p "$STAGE/usr/share/nvim/site/ftdetect"
mkdir -p "$STAGE/DEBIAN"

# Binaries.
install -m 0755 ./jennifer "$STAGE/usr/bin/jennifer"
install -m 0755 ./jennifer-tiny "$STAGE/usr/bin/jennifer-tiny"

# Man pages, gzipped per Debian policy (max compression).
gzip -9n -c "$PKG_DIR/man/jennifer.1"      > "$STAGE/usr/share/man/man1/jennifer.1.gz"
gzip -9n -c "$PKG_DIR/man/jennifer-tiny.1" > "$STAGE/usr/share/man/man1/jennifer-tiny.1.gz"

# Shared MIME definition.
install -m 0644 "$PKG_DIR/mime/jennifer.xml" "$STAGE/usr/share/mime/packages/jennifer.xml"

# Bash completion, at the Debian-policy location (file named after the
# command). jennifer-tiny symlinks to it so bash-completion lazy-loads
# the same script for both binaries.
mkdir -p "$STAGE/usr/share/bash-completion/completions"
install -m 0644 "$PKG_DIR/completions/jennifer.bash" \
    "$STAGE/usr/share/bash-completion/completions/jennifer"
ln -sf jennifer "$STAGE/usr/share/bash-completion/completions/jennifer-tiny"

# Vim / Neovim syntax highlighting, so `.j` files highlight with no user
# setup. Vim and Neovim have separate runtimepaths: /usr/share/vim/vimfiles
# for Vim, /usr/share/nvim/site for Neovim (on its default runtimepath via
# XDG_DATA_DIRS). The same syntax file works for both, so install a copy in
# each - dropping it only under vimfiles leaves Neovim without highlighting.
install -m 0644 "$REPO_ROOT/editors/vim/syntax/jennifer.vim" \
    "$STAGE/usr/share/vim/vimfiles/syntax/jennifer.vim"
install -m 0644 "$REPO_ROOT/editors/vim/ftdetect/jennifer.vim" \
    "$STAGE/usr/share/vim/vimfiles/ftdetect/jennifer.vim"
install -m 0644 "$REPO_ROOT/editors/vim/syntax/jennifer.vim" \
    "$STAGE/usr/share/nvim/site/syntax/jennifer.vim"
install -m 0644 "$REPO_ROOT/editors/vim/ftdetect/jennifer.vim" \
    "$STAGE/usr/share/nvim/site/ftdetect/jennifer.vim"

# Sublime Text / bat syntax. Unlike Vim/Neovim (read from the runtimepath at
# startup), bat compiles syntaxes into a per-user cache, so this can't
# auto-activate from a system path. It ships here for discovery; the user
# activates it once (see editors/README.md): copy to
# `$(bat --config-dir)/syntaxes/` and run `bat cache --build`.
mkdir -p "$STAGE/usr/share/jennifer/syntaxes"
install -m 0644 "$REPO_ROOT/editors/sublime/jennifer.sublime-syntax" \
    "$STAGE/usr/share/jennifer/syntaxes/jennifer.sublime-syntax"

# Jennifer-coded library modules: bare `import "name.j";` resolves under the
# system module directory. Ship the modules and their include-partials, not
# the *_test.j overlays (development-only).
mkdir -p "$STAGE/usr/share/jennifer/modules"
for m in "$REPO_ROOT"/modules/*.j; do
    case "$m" in *_test.j) continue ;; esac
    install -m 0644 "$m" "$STAGE/usr/share/jennifer/modules/"
done

# Language reference for coding assistants (also a human quick-reference).
install -m 0644 "$REPO_ROOT/JENNIFER.md" "$STAGE/usr/share/doc/jennifer/JENNIFER.md"

# Documentation: copyright (required) + changelog (Debian compresses
# the upstream changelog with gzip).
install -m 0644 "$PKG_DIR/debian/copyright" "$STAGE/usr/share/doc/jennifer/copyright"

# Generate a minimal Debian changelog entry for this version. The
# real upstream changelog is the release notes on GitHub; this is
# the Debian-package-specific record per Debian policy 4.4.
CHANGELOG="$STAGE/usr/share/doc/jennifer/changelog.Debian"
cat > "$CHANGELOG" <<EOF
jennifer ($VERSION) unstable; urgency=low

  * Upstream release $VERSION. See
    https://github.com/jennifer-language/jennifer/releases for full notes.

 -- mplx <jennifer@mplx.dev>  $(date -R)
EOF
gzip -9n "$CHANGELOG"

# DEBIAN/control: a single-paragraph *binary* control, Package first. The
# source control (packaging/debian/control) is source-style - a Source
# paragraph, a blank line, then the binary Package paragraph - so a plain
# field filter would leave two paragraphs and the first (Section / Priority
# / Maintainer / Homepage) would have no Package field, which dpkg-deb
# rejects. Reduce it to the binary form: emit Package / Version /
# Architecture at the top, then pass the remaining fields through, dropping
# the source-only fields (Source / Vcs-* / Standards-Version), the original
# Package / Architecture lines, and every blank line (so it stays one
# paragraph). Description continuation lines (leading space, or " .") are
# not blank, so they survive.
awk -v ver="$VERSION" -v arch="$ARCH" '
    BEGIN {
        print "Package: jennifer"
        print "Version: " ver
        print "Architecture: " arch
    }
    /^Source:|^Vcs-|^Standards-Version:|^Package:|^Architecture:/ { next }
    # debhelper substitution variables (e.g. `Depends: ${misc:Depends}`)
    # are expanded by dpkg-buildpackage, not this direct dpkg-deb build, so
    # drop any line still carrying one. The binaries are static (CGO off),
    # so there are no shared-library dependencies to declare anyway.
    /\$\{/ { next }
    /^[[:space:]]*$/ { next }
    { print }
' "$PKG_DIR/debian/control" > "$STAGE/DEBIAN/control"

# Hook scripts, made executable per Debian requirements.
install -m 0755 "$PKG_DIR/debian/postinst" "$STAGE/DEBIAN/postinst"
install -m 0755 "$PKG_DIR/debian/postrm" "$STAGE/DEBIAN/postrm"

# md5sums for dpkg's bookkeeping (recommended, not required).
(
    cd "$STAGE"
    find usr -type f -print0 | xargs -0 md5sum > DEBIAN/md5sums
)

# Build the .deb.
mkdir -p "$OUT"
DEB="$OUT/jennifer_${VERSION}_${ARCH}.deb"
dpkg-deb --root-owner-group --build "$STAGE" "$DEB"

# Sidecar checksum so users can verify the download.
(cd "$OUT" && sha256sum "$(basename "$DEB")" > "$(basename "$DEB").sha256")

echo "built $DEB"
