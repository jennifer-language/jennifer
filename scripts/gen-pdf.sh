#!/bin/sh
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# Renders the built mdBook site to a single PDF via headless Chrome/Chromium.
# Run AFTER `mdbook build`: it reads book-output/print.html (the whole book on
# one page, produced by mdBook's HTML backend) and prints it to a PDF. Used by
# the docs and release CI pipelines. The book-output tree is gitignored, so the
# PDF is a build artifact and is never committed.
#
# Usage: scripts/gen-pdf.sh [output.pdf]   (default: book-output/jennifer-manual.pdf)
set -eu

OUT="${1:-book-output/jennifer-manual.pdf}"
PRINT_HTML="book-output/print.html"

if [ ! -f "$PRINT_HTML" ]; then
	echo "gen-pdf: $PRINT_HTML missing - run 'mdbook build' first" >&2
	exit 1
fi

CHROME=""
for c in chromium chromium-browser google-chrome google-chrome-stable chrome; do
	if command -v "$c" >/dev/null 2>&1; then
		CHROME="$c"
		break
	fi
done
if [ -z "$CHROME" ]; then
	echo "gen-pdf: no chromium / chrome found on PATH" >&2
	exit 1
fi

# The docs site defaults to a dark theme (book.toml sets ayu for both the light
# and dark slots), so print.html renders dark and the PDF prints on black. Force
# mdBook's built-in "light" theme for the PDF only: rewrite the baked-in theme
# class and the JS default-theme constants on a throwaway copy of print.html. The
# site itself keeps its dark theme.
LIGHT_HTML="${PRINT_HTML%.html}-light.html"
sed \
	-e 's/class="ayu /class="light /' \
	-e 's/default_light_theme = "[^"]*"/default_light_theme = "light"/' \
	-e 's/default_dark_theme = "[^"]*"/default_dark_theme = "light"/' \
	"$PRINT_HTML" > "$LIGHT_HTML"

# --headless=new: current headless mode. --no-sandbox: required as root in CI.
# --no-pdf-header-footer: no browser-added date/URL chrome on each page.
"$CHROME" --headless=new --no-sandbox --disable-gpu --no-pdf-header-footer \
	--print-to-pdf="$OUT" "file://$(pwd)/$LIGHT_HTML" >/dev/null 2>&1

rm -f "$LIGHT_HTML"
echo "gen-pdf: wrote $OUT"
