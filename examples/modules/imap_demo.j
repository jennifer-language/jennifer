# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The imap module (modules/imap.j, IMAP4rev1) with mime (modules/mime.j): select a folder, fetch its messages, and parse each.
 * By default it targets a local IMAP server on 127.0.0.1:2143; with none running it prints the connection error rather than failing. Point host / port / user / pass at a real account to fetch real mail. Needs the default `jennifer` binary (`jennifer-tiny` has no network stack).
 * @module imap_demo
 */
use io;
import "../../modules/imap.j" as imap;
import "../../modules/transport.j" as transport;
import "../../modules/mime.j" as mime;

def opts as imap.Options init imap.Options{
    host: "127.0.0.1",
    port: 2143,
    security: transport.Security.None,
    user: "demo",
    pass: "demo",
    auth: ""
};

try {
    def s as imap.Session init imap.connect($opts);
    imap.selectFolder($s, "INBOX");
    # search returns stable UIDs (survive an expunge) - the basis for "process
    # only what is new": persist these UIDs, skip them next run.
    def uids as list of int init imap.search($s, imap.criteria());
    io.printf("INBOX has %d message(s):\n", len($uids));
    for (def uid in $uids) {
        def m as mime.Part init imap.fetchMessage($s, $uid);
        io.printf(
            "  uid %d | from %s | subject: %s\n",
            $uid,
            mime.headerValue($m, "From"),
            mime.headerValue($m, "Subject"));
    }
    imap.logout($s);
} catch (e) {
    io.printf("no IMAP server at %s:%d (%s)\n", $opts.host, $opts.port, $e.message);
}
