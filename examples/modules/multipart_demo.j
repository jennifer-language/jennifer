#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The multipart module (modules/multipart.j): build and parse
 * multipart/form-data. Assemble a form of two text fields and a file, show the
 * encoded body, then parse it back. Pure `.j`; runs on both binaries.
 * Run: jennifer run examples/modules/multipart_demo.j
 * @module multipart_demo
 */
use io;
use convert;
import "../../modules/multipart.j" as multipart;

def parts as list of multipart.Part init [
    multipart.field("title", "quarterly report"),
    multipart.field("author", "alice"),
    multipart.file("attachment", "note.txt", "text/plain",
        convert.bytesFromString("see attached", "utf-8"))
];

def form as multipart.Built init multipart.buildWith($parts, "----DemoBoundary");
io.printf("Content-Type: %s\n\n", $form.contentType);
io.printf("%s\n", convert.stringFromBytes($form.body, "utf-8"));

io.printf("--- parsed back ---\n");
def back as list of multipart.Part init multipart.parse($form.contentType, $form.body);
for (def p in $back) {
    if (multipart.isFile($p)) {
        io.printf("file  %s -> %s (%s, %d bytes)\n", $p.name, $p.filename, $p.contentType, len($p.data));
    } else {
        io.printf("field %s = %s\n", $p.name, multipart.text($p));
    }
}
