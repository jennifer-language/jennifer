# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The pop module (modules/pop.j, POP3) with mime (modules/mime.j): fetch a mailbox and parse each message.
 * By default it targets a local POP3 server on 127.0.0.1:2110; with none running it prints the connection error rather than failing. Point host / port / user / pass at a real mailbox to fetch real mail. Needs the default `jennifer` binary (`jennifer-tiny` has no network stack).
 * @module pop_demo
 */
use io;
use maps;
import "../../modules/pop.j" as pop;
import "../../modules/mime.j" as mime;

def opts as pop.Options init pop.Options{
    host: "127.0.0.1",
    port: 2110,
    security: "none",
    user: "demo",
    pass: "demo",
    auth: ""
};

try {
    # Leave-on-server: fetch by stable UIDL id, previewing headers with TOP, and
    # never delete - so re-running only shows what is new (given a persisted set).
    def seen as map of string to bool init {}; # a real app would load this from disk
    def s as pop.Session init pop.connect($opts);
    for (def m in pop.uidl($s)) {
        if (maps.has($seen, $m.id)) {
            continue;
        }
        def preview as mime.Part init mime.parse(pop.top($s, $m.number, 0)); # headers only
        io.printf(
            "  [%s] from %s | subject: %s\n",
            $m.id,
            mime.headerValue($preview, "From"),
            mime.headerValue($preview, "Subject"));
        $seen[$m.id] = true;
    }
    pop.quit($s); # no deleteMessage: the mailbox is left intact
} catch (e) {
    io.printf("no POP3 server at %s:%d (%s)\n", $opts.host, $opts.port, $e.message);
}
