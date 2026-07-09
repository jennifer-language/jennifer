#!/bin/sh
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
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

# Vim / Neovim syntax highlighting. /usr/share/vim/vimfiles is on Vim's
# default runtimepath, so `.j` files highlight with no user setup.
install -m 0644 "$REPO_ROOT/editors/vim/syntax/jennifer.vim" \
    "$STAGE/usr/share/vim/vimfiles/syntax/jennifer.vim"
install -m 0644 "$REPO_ROOT/editors/vim/ftdetect/jennifer.vim" \
    "$STAGE/usr/share/vim/vimfiles/ftdetect/jennifer.vim"

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
    https://github.com/mplx/jennifer-lang/releases for full notes.

 -- developer@mplx.eu <developer@mplx.eu>  $(date -R)
EOF
gzip -9n "$CHANGELOG"

# DEBIAN/control: substitute the version + arch placeholders and
# emit the resolved file. Source control file uses placeholders so
# both arches can build from the same metadata.
sed \
    -e "s/^Architecture: amd64 arm64$/Architecture: $ARCH/" \
    "$PKG_DIR/debian/control" \
    | awk '
        # Drop the Source/VCS/Standards lines that only apply at
        # build-from-source level; the binary control file is a
        # subset.
        /^Source:|^Vcs-|^Standards-Version:/ { next }
        # Insert the Version field right after Package.
        /^Package: / { print; print "Version: '"$VERSION"'"; next }
        { print }
    ' > "$STAGE/DEBIAN/control"

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
