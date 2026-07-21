# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The mime module (modules/mime.j): build a multipart message, serialize it, then parse it back.
 * @module mime_demo
 */
use io;
import "../../modules/mime.j" as mime;

# Build a multipart/alternative message with a plain and an HTML part.
def plain as mime.Part init mime.text("text/plain", "Hello, monde: café.");
def html as mime.Part init mime.text("text/html", "<p>Hello, <b>monde</b>: café.</p>");
def parts as list of mime.Part init [];
$parts[] = $plain;
$parts[] = $html;

def msg as mime.Part init mime.multipart("alternative", "=_boundary_42", $parts);
$msg = mime.withHeader($msg, "From", mime.address("Ada Lovelace", "ada@example.com"));
$msg = mime.withHeader($msg, "To", mime.address("", "team@example.com"));
$msg = mime.withHeader($msg, "Subject", "Bonjour");

def wire as string init mime.encode($msg);
io.printf("=== encoded message ===\n%s\n", $wire);

# Parse it back and walk the tree.
def back as mime.Part init mime.parse($wire);
io.printf("=== parsed ===\n");
io.printf("from:    %s\n", mime.headerValue($back, "From"));
io.printf("subject: %s\n", mime.headerValue($back, "Subject"));
io.printf("type:    %s\n", mime.contentType($back));
for (def part in mime.parts($back)) {
    io.printf("  part %s: %s\n", mime.contentType($part), mime.body($part));
}
