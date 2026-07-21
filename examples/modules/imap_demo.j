# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The imap module (modules/imap.j, IMAP4rev1) with mime (modules/mime.j): select a mailbox, fetch its messages, and parse each.
 * By default it targets a local IMAP server on 127.0.0.1:2143; with none running it prints the connection error rather than failing. Point host / port / user / pass at a real mailbox to fetch real mail. Needs the default `jennifer` binary (`jennifer-tiny` has no network stack).
 * @module imap_demo
 */
use io;
import "../../modules/imap.j" as imap;
import "../../modules/mime.j" as mime;

def opts as imap.Options init imap.Options{host: "127.0.0.1", port: 2143,
    security: "none", user: "demo", pass: "demo", auth: ""};

try {
    def msgs as list of string init imap.fetchAll($opts, "INBOX");
    io.printf("fetched %d message(s) from INBOX:\n", len($msgs));
    for (def raw in $msgs) {
        def m as mime.Part init mime.parse($raw);
        io.printf("  from %s | subject: %s\n", mime.headerValue($m, "From"),
            mime.headerValue($m, "Subject"));
    }
} catch (e) {
    io.printf("no IMAP server at %s:%d (%s)\n", $opts.host, $opts.port, $e.message);
}
