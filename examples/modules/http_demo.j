#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# http_demo.j - make HTTP requests with the http module.
#
# Needs an HTTP server and the default `jennifer` binary (the module uses
# `net`). Point `base` at any server - a local dev server on 127.0.0.1:8080, or
# a public URL like http://example.com - then:
#
#     jennifer run examples/modules/http_demo.j
#
# With no server reachable it prints the connection error rather than failing.
# Not a golden test (it needs a live server); it demonstrates the surface.
use io;
import "../../modules/http.j" as http;

def base as string init "http://127.0.0.1:8080";

try {
    # a GET, reading status / headers / body
    def r as http.Response init http.get($base + "/", {});
    io.printf("GET / -> %d %s\n", $r.status, $r.statusText);
    io.printf("content-type: %s\n", http.header($r, "Content-Type"));
    io.printf("body (%d bytes):\n%s\n", len($r.body), $r.body);

    # a POST with a JSON body and an auth header
    def sent as http.Response init http.post($base + "/items", "application/json",
        "{\"name\":\"ada\"}", {"Authorization": "Bearer demo-token"});
    io.printf("POST /items -> %d\n", $sent.status);
} catch (e) {
    io.printf("no HTTP server at %s (%s)\n", $base, $e.message);
}
