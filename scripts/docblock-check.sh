#!/bin/sh
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# docblock-check.sh - run the `docblock` parser over a .j file or a directory
# tree and report each construct's doc coverage plus any diagnostics (a @param
# / @field that names nothing real, an undocumented parameter, an orphaned doc
# comment). Exits 0 when every file is clean, 1 when any diagnostics are found -
# so it drops into a pre-commit hook or CI step.
#
#   scripts/docblock-check.sh path/to/file.j
#   scripts/docblock-check.sh modules/
#
# Uses the built `jennifer` binary. Override with JENNIFER=/path/to/jennifer;
# otherwise it looks for ./jennifer at the repo root, then `jennifer` on PATH.

set -eu

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/.." && pwd)
docblock="$repo/modules/docblock.j"

usage() {
    echo "usage: $0 <file.j | directory>" >&2
    echo "  runs docblock.parse over the target; exit 1 if any diagnostics" >&2
    exit 2
}

[ $# -eq 1 ] || usage
case "$1" in
    -h | --help) usage ;;
esac
target=$1

if [ ! -e "$target" ]; then
    echo "error: no such file or directory: $target" >&2
    exit 2
fi
if [ ! -f "$docblock" ]; then
    echo "error: docblock module not found at $docblock" >&2
    exit 2
fi

# Locate a jennifer binary.
if [ -n "${JENNIFER:-}" ]; then
    jennifer=$JENNIFER
elif [ -x "$repo/jennifer" ]; then
    jennifer="$repo/jennifer"
elif command -v jennifer >/dev/null 2>&1; then
    jennifer=jennifer
else
    echo "error: no jennifer binary (set JENNIFER=... or run 'make build')" >&2
    exit 2
fi

# A .j checker that parses os.ARGS[1] and reports it; exit 1 on diagnostics.
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT INT TERM
checker="$tmpdir/docblock-check.j"
cat > "$checker" <<EOF
use io;
use fs;
use os;
import "$docblock" as db;

def path as string init os.ARGS[1];
def d as db.FileDoc init db.parse(fs.readString(\$path));
def n as int init len(\$d.diagnostics);
if (\$n == 0) {
    io.printf("ok   %s  (%d func, %d struct, %d const)\n",
        \$path, len(\$d.funcs), len(\$d.structs), len(\$d.consts));
    exit 0;
}
io.printf("WARN %s  (%d diagnostic)\n", \$path, \$n);
for (def g in \$d.diagnostics) {
    io.printf("       line %d: %s\n", \$g.line, \$g.message);
}
exit 1;
EOF

# Collect target files.
if [ -d "$target" ]; then
    files=$(find "$target" -type f -name '*.j' | sort)
else
    files=$target
fi
if [ -z "$files" ]; then
    echo "no .j files found under $target" >&2
    exit 0
fi

total=0
withdiag=0
for f in $files; do
    total=$((total + 1))
    if "$jennifer" run "$checker" "$f"; then
        :
    else
        withdiag=$((withdiag + 1))
    fi
done

echo
echo "docblock: checked $total file(s), $withdiag with diagnostics"
[ "$withdiag" -eq 0 ]
