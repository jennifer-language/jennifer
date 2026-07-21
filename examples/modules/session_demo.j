#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Server-side sessions on memcached with the session module.
 * Needs a memcached server on 127.0.0.1:11211 and the default jennifer binary (session builds on the memcache module, which uses net). With no server running it prints the connection error rather than failing.
 * @module session_demo
 */
use io;
import "../../modules/session.j" as session;
import "../../modules/memcache.j" as memcache;

def opts as memcache.Options init memcache.Options{host: "127.0.0.1", port: 11211};

try {
    def mc as memcache.Session init memcache.connect($opts);

    # Start a session (30 minutes) and populate it.
    def id as string init session.create($mc, 1800);
    io.printf("created session %s\n", $id);

    def data as map of string to string init session.load($mc, $id);
    $data["user"] = "ada";
    $data["name"] = "José";           # non-ASCII survives (base64-wrapped)
    session.save($mc, $id, $data, 1800);

    # A later request loads it back.
    def back as map of string to string init session.load($mc, $id);
    io.printf("user -> %s\n", $back["user"]);
    io.printf("name -> %s\n", $back["name"]);

    # Keep it alive without rewriting, then end it.
    io.printf("touch  -> %t\n", session.touch($mc, $id, 1800));
    io.printf("destroy-> %t\n", session.destroy($mc, $id));
    io.printf("gone   -> %d entries\n", len(session.load($mc, $id)));

    memcache.quit($mc);
} catch (e) {
    io.printf("no memcached server at %s:%d (%s)\n", $opts.host, $opts.port, $e.message);
}
