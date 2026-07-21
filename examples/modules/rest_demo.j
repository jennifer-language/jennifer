#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Talk to a JSON REST API with the rest module.
 * Needs a REST server and the default jennifer binary (rest builds on the http client, which uses net). Point base at any JSON API. With no server reachable it prints the connection error rather than failing.
 * @module rest_demo
 */
use io;
use json;
import "../../modules/rest.j" as rest;
import "../../modules/http.j" as http;

def api as rest.Client init rest.Client{baseUrl: "http://127.0.0.1:8080",
    headers: {"Authorization": rest.bearer("demo-token")}};

try {
    # create a resource from a JSON value
    def created as rest.Response init rest.postJson($api, "/users",
        json.decode("{\"name\":\"ada\"}"));
    io.printf("POST /users -> %d\n", $created.status);

    # read one back and decode it
    def user as json.Value init rest.getJson($api, "/users/1", {});
    io.printf("name -> %s\n", json.asString($user, "/name"));

    # a GET with a query string
    def hits as json.Value init rest.getJson($api, "/search", {"q": "ada lovelace"});
    io.printf("search echoed -> %s\n", json.asString($hits, "/q"));
} catch (e) {
    io.printf("no REST server at %s (%s)\n", $api.baseUrl, $e.message);
}
