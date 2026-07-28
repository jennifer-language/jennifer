# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

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

# Attach a binary file and extract it back out. attachmentBytes carries raw
# bytes (here a tiny "PNG" with a 0xff byte no UTF-8 decode would survive).
def raw as bytes;
$raw[] = 0x89;
$raw[] = 0x50;
$raw[] = 0x4E;
$raw[] = 0x47;
$raw[] = 0xff;
$raw[] = 0x00;
def withFile as mime.Part init mime.multipart(
    "mixed",
    "=_mixed_7",
    [$plain, mime.attachmentBytes("logo.png", "image/png", $raw)]);

# Round-trip through the wire, then pull the parts apart with the accessors.
def parsed as mime.Part init mime.parse(mime.encode($withFile));
io.printf("=== attachments ===\n");
for (def att in mime.attachments($parsed)) {
    io.printf(
        "  %s (%s), %d bytes\n",
        mime.filename($att),
        mime.contentType($att),
        len(mime.data($att)));
}
io.printf("=== text bodies ===\n");
for (def tb in mime.textBodies($parsed)) {
    io.printf("  %s: %s\n", mime.contentType($tb), mime.body($tb));
}

# A received message may declare a non-UTF-8 charset; parse decodes body per the
# Content-Type charset (here Latin-1, quoted-printable) instead of forcing UTF-8.
def latin1 as mime.Part init mime.parse(
    "Content-Type: text/plain; charset=iso-8859-1\r\n" +
        "Content-Transfer-Encoding: quoted-printable\r\n\r\ncaf=E9 r=E9sum=E9\r\n");
io.printf("=== charset decode ===\n  latin-1 body: %s\n", mime.body($latin1));

# A non-ASCII attachment filename is written and read in RFC 2231 extended form.
def doc as mime.Part init mime.attachment("rapport-café.txt", "text/plain", "Le résumé du café.");
io.printf("=== rfc 2231 filename ===\n");
io.printf("  header:     %s\n", mime.headerValue($doc, "Content-Disposition"));
io.printf("  round-trip: %s\n", mime.filename(mime.parse(mime.encode($doc))));
