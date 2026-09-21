#!/bin/sh
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# Assemble an OpenWRT `.ipk` from an already-built static `jennifer`
# binary. Run after the cross-compile step has produced ./jennifer in
# CWD (the release pipeline builds it CGO_ENABLED=0, so it is a fully
# static, libc-independent ELF that runs on OpenWRT's musl the same as
# on glibc). No OpenWRT SDK is needed: an `.ipk` is an `ar` archive of
# debian-binary + control.tar.gz + data.tar.gz, which this script builds
# with stock `ar` / `tar` / `gzip`.
#
# The payload is deliberately minimal - the interpreter plus the .j
# module library - not the full desktop bundle the .deb / tarball carry
# (man pages, MIME, vim/nvim/sublime syntax): those waste flash on a
# router and have no consumer there. jennifer-tiny is not shipped either
# (it is currently unbuildable upstream, and one interpreter is what an
# OpenWRT target wants); when it returns it is the better fit here and
# gets its own package.
#
# The install paths match the binary's compile-time defaults:
#   /usr/bin/jennifer                 - the interpreter
#   /usr/share/jennifer/modules/*.j   - the system module dir
#     (internal/module.compileDefaultSysmoddir), so bare
#     `import "name.j";` resolves with no --sysmoddir / JENNIFER_SYSMODDIR.
#
# Usage:
#   scripts/build-ipk.sh <version> <arch> <out-dir> [openwrt-arch]
#
# Arguments:
#   <version>      - bare semver, matching the git tag (e.g. 0.14.1).
#   <arch>         - amd64 or arm64 (the release matrix arch), mapped to
#                    the default OpenWRT architecture string below.
#   <out-dir>      - directory to write the resulting .ipk into.
#   [openwrt-arch] - optional override for the OpenWRT `Architecture:`
#                    field (e.g. aarch64_cortex-a53, arm_cortex-a7_neon-vfpv4).
#                    Because the binary is static and CPU-family- (not
#                    subtarget-) specific, the default generic string plus
#                    `opkg install --force-architecture` covers other
#                    subtargets; override here to stamp an exact one.
#
# Expects in CWD:
#   ./jennifer  - standard Go binary, built CGO_ENABLED=0 (static).
#
# Output:
#   <out-dir>/jennifer_<version>_<openwrt-arch>.ipk
#   <out-dir>/jennifer_<version>_<openwrt-arch>.ipk.sha256

set -eu

if [ $# -lt 3 ] || [ $# -gt 4 ]; then
    echo "usage: $0 <version> <arch> <out-dir> [openwrt-arch]" >&2
    exit 2
fi

VERSION="$1"
ARCH="$2"
OUT="$3"
OWRT_ARCH="${4:-}"

# Map the release matrix arch to the default OpenWRT architecture string.
# x86_64 is stable across x86-64 OpenWRT targets; aarch64_generic is the
# lowest-common-denominator ARMv8 string (a generic ARMv8 static binary runs
# on every aarch64 subtarget - see the README on --force-architecture).
case "$ARCH" in
    amd64) DEFAULT_OWRT_ARCH=x86_64 ;;
    arm64) DEFAULT_OWRT_ARCH=aarch64_generic ;;
    *)
        echo "unsupported arch: $ARCH (want amd64 or arm64)" >&2
        exit 2
        ;;
esac
[ -n "$OWRT_ARCH" ] || OWRT_ARCH="$DEFAULT_OWRT_ARCH"

for t in ar tar gzip sha256sum du; do
    if ! command -v "$t" >/dev/null 2>&1; then
        echo "required tool not found: $t" >&2
        exit 2
    fi
done

if [ ! -f ./jennifer ]; then
    echo "missing required input: ./jennifer (build it CGO_ENABLED=0 first)" >&2
    exit 2
fi

# Resolve the output dir to an absolute path now: the assembly step cd's into a
# temp staging dir, after which a relative <out-dir> would no longer resolve.
mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PKG_DIR="$REPO_ROOT/packaging"

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

DATA="$STAGE/data"
CTRL="$STAGE/control"
mkdir -p "$DATA/usr/bin" "$DATA/usr/share/jennifer/modules" \
         "$DATA/usr/share/doc/jennifer" "$CTRL"

# The interpreter.
install -m 0755 ./jennifer "$DATA/usr/bin/jennifer"

# Jennifer-coded library modules at the compile-default system module dir,
# minus the *_test.j overlays (development-only). Small text files.
for m in "$REPO_ROOT"/modules/*.j; do
    case "$m" in *_test.j) continue ;; esac
    install -m 0644 "$m" "$DATA/usr/share/jennifer/modules/"
done

# License / copyright notice (LGPL attribution travels with the binary).
install -m 0644 "$PKG_DIR/debian/copyright" "$DATA/usr/share/doc/jennifer/copyright"

# Installed-Size in bytes (opkg reports it and uses it for the free-space check).
ISIZE="$(du -sb "$DATA" | cut -f1)"

# control: fill the template's @VERSION@ / @ARCH@ / @ISIZE@ placeholders.
sed \
    -e "s/@VERSION@/$VERSION/" \
    -e "s/@ARCH@/$OWRT_ARCH/" \
    -e "s/@ISIZE@/$ISIZE/" \
    "$PKG_DIR/openwrt/control" > "$CTRL/control"
chmod 0644 "$CTRL/control"

# control.tar.gz and data.tar.gz: root-owned, deterministic member metadata so
# the .ipk is reproducible. Paths are ./-relative, as opkg expects.
( cd "$CTRL" && tar --numeric-owner --owner=0 --group=0 \
    --mtime='@0' -czf "$STAGE/control.tar.gz" ./control )
( cd "$DATA" && tar --numeric-owner --owner=0 --group=0 \
    --mtime='@0' -czf "$STAGE/data.tar.gz" ./ )

# debian-binary: the format-version stamp (same "2.0" as a .deb; opkg reads it).
printf '2.0\n' > "$STAGE/debian-binary"

IPK="$OUT/jennifer_${VERSION}_${OWRT_ARCH}.ipk"
rm -f "$IPK"
# ar archive, deterministic (D): debian-binary, then the two tarballs. Modern
# opkg (libarchive) extracts members by name, order-independent.
( cd "$STAGE" && ar rcD "$IPK" debian-binary control.tar.gz data.tar.gz )

# Sidecar checksum so users can verify the download.
( cd "$OUT" && sha256sum "$(basename "$IPK")" > "$(basename "$IPK").sha256" )

echo "built $IPK (Architecture: $OWRT_ARCH, Installed-Size: $ISIZE)"
