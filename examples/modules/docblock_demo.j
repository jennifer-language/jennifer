#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Extract structured docs from Jennifer source.
 * Parses an embedded sample module and prints the documentation docblock found in it: the module preamble, each documented construct, and any diagnostics (here, a deliberately drifted @param). Self-contained; runs on either binary.
 * @module docblock_demo
 */
use io;
import "../../modules/docblock.j" as docblock;

def source as string init "/**
 * A tiny geometry module.
 * @module geometry
 * @author edv
 * @version 1.0
 */

/**
 * Distance between two points.
 * @param ax {float} first x
 * @param ay {float} first y
 * @param bx {float} second x
 * @param by {float} second y
 * @return {float} the Euclidean distance
 */
export func distance(ax as float, ay as float, bx as float, by as float) {
    return 0.0;
}

/**
 * A point in the plane.
 * @field x {float} the x coordinate
 * @field y {float} the y coordinate
 */
export def struct Point { x as float, y as float };

/**
 * Ratio of a circle's circumference to its diameter.
 * @deprecated use math.PI
 */
def const PI as float init 3.14159;

/**
 * Drifted docs: names a parameter that is not there.
 * @param typo {float} oops
 */
func drifted(real as float) {
    return;
}
";

def doc as docblock.FileDoc init docblock.parse($source);

io.printf("module: %s\n", $doc.module.summary);
io.printf("  by %s, version %s\n\n", $doc.module.author, $doc.module.version);

io.printf("functions:\n");
for (def f in $doc.funcs) {
    io.printf("  %s  (exported=%t, %d params -> %s)\n",
        $f.name, $f.exported, len($f.params), $f.returns.type);
    io.printf("    %s\n", $f.summary);
}

io.printf("structs:\n");
for (def s in $doc.structs) {
    io.printf("  %s  (%d fields)\n", $s.name, len($s.fields));
}

io.printf("constants:\n");
for (def c in $doc.consts) {
    io.printf("  %s as %s", $c.name, $c.type);
    if ($c.deprecated != "") {
        io.printf("  [deprecated: %s]", $c.deprecated);
    }
    io.printf("\n");
}

io.printf("diagnostics:\n");
for (def d in $doc.diagnostics) {
    io.printf("  [%s] line %d: %s\n", $d.severity, $d.line, $d.message);
}
