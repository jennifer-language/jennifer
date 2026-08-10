#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * A self-contained GraphQL client demo: it starts a tiny local stub server (so
 * the example runs with no network), then uses the `graphql` module to run a
 * successful query and a query the server answers with a GraphQL `errors` array
 * (an HTTP 200), showing that `graphql.query` raises a typed `graphql` error
 * rather than trusting the 200 status. Against a real endpoint you would just
 * `graphql.client("https://host/graphql")` and skip the stub server.
 * @module graphql_demo
 */
use io;
use httpd;
use task;
use json;
use strings;
use convert;
import "../../modules/graphql.j" as graphql;

# A stub GraphQL server: a real query gets data; anything mentioning "boom" gets
# a GraphQL errors array (still HTTP 200, the GraphQL convention).
func serve(srv as httpd.Server) {
    while (true) {
        try {
            def req as httpd.Request init httpd.accept($srv);
            def body as string init convert.stringFromBytes(httpd.body($req), "utf-8");
            if (strings.contains($body, "boom")) {
                httpd.respond($req, 200, '{"errors":[{"message":"no such field: boom"}]}');
            } else {
                httpd.respond($req, 200, '{"data":{"viewer":{"login":"octocat"}}}');
            }
        } catch (acceptErr) {
            return;
        }
    }
}

def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def addr as string init httpd.address($srv);
def server as task of null init spawn {
    serve($srv);
};

def gql as graphql.Client init graphql.header(graphql.client("http://" + $addr), "X-Demo", "1");

# A successful query - read the result from under /data.
def resp as json.Value init graphql.query($gql, '{ viewer { login } }', json.map());
io.printf("viewer login = %s\n", json.asString($resp, "/data/viewer/login"));

# A query the server rejects with a GraphQL errors array (HTTP 200) - caught as a
# typed `graphql` error, not mistaken for success.
try {
    graphql.query($gql, '{ boom }', json.map());
    io.printf("unexpected success\n");
} catch (e) {
    io.printf("caught %s: %s\n", $e.kind, $e.message);
}

httpd.shutdown($srv);
task.wait($server);
