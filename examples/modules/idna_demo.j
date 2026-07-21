# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The idna module (modules/idna.j): convert internationalized domain names to their ASCII-compatible (xn--) form and back.
 * Run: jennifer run examples/modules/idna_demo.j
 * @module idna_demo
 */
use io;
import "../../modules/idna.j" as idna;

def domains as list of string init ["münchen.de", "bücher.example", "café.fr",
    "señor.correos.es", "example.com"];

io.printf("unicode  ->  ascii (xn--)  ->  round-trip\n");
for (def d in $domains) {
    def ace as string init idna.toAscii($d);
    io.printf("%s  ->  %s  ->  %s\n", $d, $ace, idna.toUnicode($ace));
}
