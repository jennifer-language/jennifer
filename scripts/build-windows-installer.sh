#!/usr/bin/env bash
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# Recreate the Windows installer (jennifer-<version>-setup.exe) locally on Linux
# via Wine + Inno Setup - the same artifact the release CI builds on a
# windows-latest runner from packaging/windows/jennifer.iss. It cross-compiles a
# fresh windows/amd64 jennifer.exe, provisions Inno Setup under a Wine prefix
# (downloaded once, then cached), compiles the .iss, and writes the installer to
# dist/. This is a best-effort, UNSUPPORTED build; the installer is unsigned.
#
# Usage:
#   scripts/build-windows-installer.sh
#
# Requirements:
#   - Go toolchain (the GOOS=windows cross-compile is pure Go, no cross C).
#   - wine, to run the Inno Setup compiler (Linux has no native iscc) - unless
#     you point ISCC at a native compiler.
#   - curl, to fetch Inno Setup the first time (Wine path only).
#
# Environment (all optional):
#   APP_VERSION   Installer version label and filename (default: scripts/version.sh
#                 with the +build metadata stripped, e.g. 0.20.0-dev).
#   OUT_DIR       Where to leave the installer (default: <repo>/dist). The .iss
#                 always writes to <repo>/dist; a different OUT_DIR gets a copy.
#   ISCC          Path to a native Inno Setup compiler (ISCC.exe / iscc). If set,
#                 Wine is not used.
#   WINEPREFIX    Wine prefix to create/reuse (default:
#                 ${XDG_CACHE_HOME:-~/.cache}/jennifer-innosetup-wine). Persisted
#                 across runs, so Inno Setup is downloaded only once.
#   INNO_VERSION  Inno Setup version to fetch under Wine (default 6.7.3).
#
# Output:
#   <OUT_DIR>/jennifer-<APP_VERSION>-setup.exe  (+ .sha256)

set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/.." && pwd)
cd "$repo"

INNO_VERSION=${INNO_VERSION:-6.7.3}
OUT_DIR=${OUT_DIR:-$repo/dist}

die() { echo "error: $*" >&2; exit 1; }

command -v go >/dev/null 2>&1 || die "go toolchain not found"

# Temp files to remove on exit (the cross-compiled exe, and the downloaded Inno
# Setup installer if we fetch one). The installer bundles jennifer.exe, so we do
# not keep the standalone copy in the tree.
cleanup_files=("$repo/jennifer.exe")
cleanup() { rm -f "${cleanup_files[@]}"; }
trap cleanup EXIT

# --- version -----------------------------------------------------------------
"$repo/scripts/gen-version.sh" >/dev/null   # stamp version_gen.go so jennifer.exe reports the real version
version=$("$repo/scripts/version.sh")
APP_VERSION=${APP_VERSION:-${version%%+*}}  # strip +build metadata for the installer label / filename
echo ">> version: $version   (installer AppVersion: $APP_VERSION)"

# --- cross-compile jennifer.exe at the repo root (the .iss references ..\..\jennifer.exe) ---
echo ">> building windows/amd64 jennifer.exe"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
    go build -trimpath -ldflags="-s -w" -o "$repo/jennifer.exe" ./cmd/jennifer

# --- locate / provision the Inno Setup compiler ------------------------------
if [ -n "${ISCC:-}" ]; then
    echo ">> using native Inno Setup: $ISCC"
    run_iscc() { "$ISCC" "$@"; }
elif command -v iscc >/dev/null 2>&1; then
    echo ">> using iscc from PATH"
    run_iscc() { iscc "$@"; }
else
    command -v wine >/dev/null 2>&1 || \
        die "wine not found (needed to run the Inno Setup compiler on Linux); install wine, or set ISCC to a native compiler"
    export WINEPREFIX=${WINEPREFIX:-${XDG_CACHE_HOME:-$HOME/.cache}/jennifer-innosetup-wine}
    export WINEARCH=win64
    export WINEDEBUG=${WINEDEBUG:--all}
    export WINEDLLOVERRIDES=${WINEDLLOVERRIDES:-mscoree,mshtml=}
    iscc_exe="$WINEPREFIX/drive_c/Program Files (x86)/Inno Setup 6/ISCC.exe"
    if [ ! -f "$iscc_exe" ]; then
        echo ">> provisioning Inno Setup $INNO_VERSION under Wine ($WINEPREFIX)"
        command -v curl >/dev/null 2>&1 || die "curl not found (needed to download Inno Setup)"
        wineboot -i >/dev/null 2>&1 || true
        tag="is-${INNO_VERSION//./_}"
        url="https://github.com/jrsoftware/issrc/releases/download/${tag}/innosetup-${INNO_VERSION}.exe"
        tmp=$(mktemp --suffix=.exe)
        cleanup_files+=("$tmp")
        curl -fsSL -o "$tmp" "$url" || die "failed to download Inno Setup from $url"
        wine "$tmp" /VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SP- >/dev/null 2>&1 || \
            die "Inno Setup silent install failed"
        [ -f "$iscc_exe" ] || die "ISCC.exe not found after install: $iscc_exe"
    else
        echo ">> using cached Inno Setup under Wine ($WINEPREFIX)"
    fi
    run_iscc() { wine "$iscc_exe" "$@"; }
fi

# --- compile the installer ---------------------------------------------------
echo ">> compiling packaging/windows/jennifer.iss"
run_iscc "/DAppVersion=$APP_VERSION" 'packaging\windows\jennifer.iss' || \
    die "Inno Setup compile failed (see output above)"

# --- locate output (the .iss writes to <repo>/dist) + checksum ---------------
setup="$repo/dist/jennifer-$APP_VERSION-setup.exe"
[ -f "$setup" ] || die "expected installer not found: $setup"

mkdir -p "$OUT_DIR"
if [ "$OUT_DIR" != "$repo/dist" ]; then
    cp "$setup" "$OUT_DIR/"
    setup="$OUT_DIR/jennifer-$APP_VERSION-setup.exe"
fi
( cd "$(dirname "$setup")" && sha256sum "$(basename "$setup")" > "$(basename "$setup").sha256" )

echo ""
echo ">> done:"
echo "   $setup"
echo "   $(cat "$setup.sha256")"
