# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The pop module (modules/pop.j, POP3) with mime (modules/mime.j): fetch a mailbox and parse each message.
 * By default it targets a local POP3 server on 127.0.0.1:2110; with none running it prints the connection error rather than failing. Point host / port / user / pass at a real mailbox to fetch real mail. Needs the default `jennifer` binary (`jennifer-tiny` has no network stack).
 * @module pop_demo
 */
use io;
import "../../modules/pop.j" as pop;
import "../../modules/mime.j" as mime;

def opts as pop.Options init pop.Options{host: "127.0.0.1", port: 2110,
    security: "none", user: "demo", pass: "demo", auth: ""};

try {
    def msgs as list of string init pop.fetchAll($opts);
    io.printf("fetched %d message(s):\n", len($msgs));
    for (def raw in $msgs) {
        def m as mime.Part init mime.parse($raw);
        io.printf("  from %s | subject: %s\n", mime.headerValue($m, "From"),
            mime.headerValue($m, "Subject"));
    }
} catch (e) {
    io.printf("no POP3 server at %s:%d (%s)\n", $opts.host, $opts.port, $e.message);
}
