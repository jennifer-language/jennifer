// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestGraphql drives the graphql module's live request path against a local
// httpd server: a successful query returns the decoded response (data under
// /data); a GraphQL execution error (HTTP 200 with a top-level errors array)
// raises a "graphql" error carrying the joined messages; and a non-2xx HTTP
// status raises a "graphql" error with the status and body. The stub server
// switches its reply on a marker in the posted query string.
func TestGraphql(t *testing.T) {
	graphqlMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "graphql.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use httpd;
use task;
use json;
use strings;
use convert;
import %q as graphql;

# Stub GraphQL server: pick the reply from a marker in the request body.
func serve(srv as httpd.Server) {
    while (true) {
        try {
            def req as httpd.Request init httpd.accept($srv);
            def body as string init convert.stringFromBytes(httpd.body($req), "utf-8");
            if (strings.contains($body, "causeHttp")) {
                httpd.respond($req, 500, "upstream exploded");
            } elseif (strings.contains($body, "\"operationName\":\"PickB\"")) {
                # Confirms the operationName field reached the wire.
                httpd.respond($req, 200, '{"data":{"picked":"B"}}');
            } elseif (strings.contains($body, "structErr")) {
                # A structured error: partial data + an errors entry with extensions.code.
                httpd.respond($req, 200, '{"data":{"x":null},"errors":[{"message":"denied","extensions":{"code":"FORBIDDEN"}}]}');
            } elseif (strings.contains($body, "causeErr")) {
                httpd.respond($req, 200, '{"errors":[{"message":"field x failed"},{"message":"and y"}]}');
            } else {
                httpd.respond($req, 200, '{"data":{"hello":"world"}}');
            }
        } catch (acceptErr) {
            return;
        }
    }
}

def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def addr as string init httpd.address($srv);
def server as task of null init spawn { serve($srv); };

def c as graphql.Client init graphql.header(graphql.client("http://" + $addr), "X-Client", "jennifer");

# 1. Success: the full response comes back, data is under /data.
def ok as json.Value init graphql.query($c, '{ hello }', json.map());
testing.assertEqual(json.asString($ok, "/data/hello"), "world");

# 2. GraphQL errors (HTTP 200 + errors array) -> a graphql error with the joined
#    messages, NOT trusting the 200 status.
def gotGqlErr as bool init false;
try {
    graphql.query($c, "causeErr", json.map());
} catch (e) {
    $gotGqlErr = true;
    testing.assertEqual($e.kind, "graphql");
    testing.assertTrue(strings.contains($e.message, "field x failed"));
    testing.assertTrue(strings.contains($e.message, "and y"));
}
testing.assertTrue($gotGqlErr);

# 3. HTTP error (non-2xx) -> a graphql error carrying the status and body.
def gotHttpErr as bool init false;
try {
    graphql.query($c, "causeHttp", json.map());
} catch (e) {
    $gotHttpErr = true;
    testing.assertEqual($e.kind, "graphql");
    testing.assertTrue(strings.contains($e.message, "500"));
}
testing.assertTrue($gotHttpErr);

# 4. queryNamed sends operationName - the stub only returns picked=B when it saw
#    "operationName":"PickB" on the wire.
def named as json.Value init graphql.queryNamed($c, 'query A { a } query B { b }', json.map(), "PickB");
testing.assertEqual(json.asString($named, "/data/picked"), "B");

# 5. tryQuery does NOT raise on GraphQL errors - it returns the envelope so the
#    caller reads partial data + structured error fields (extensions.code).
def env as json.Value init graphql.tryQuery($c, "structErr", json.map());
testing.assertTrue(graphql.hasErrors($env));
testing.assertTrue(strings.contains(graphql.errorMessages($env), "denied"));
testing.assertEqual(json.asString($env, "/errors/0/extensions/code"), "FORBIDDEN");
testing.assertTrue(json.isNull($env, "/data/x"));   # partial data preserved

# 6. tryQuery still raises on a non-2xx HTTP status (no GraphQL envelope to return).
def tryHttpErr as bool init false;
try {
    graphql.tryQuery($c, "causeHttp", json.map());
} catch (e) {
    $tryHttpErr = true;
    testing.assertEqual($e.kind, "graphql");
}
testing.assertTrue($tryHttpErr);

httpd.shutdown($srv);
task.wait($server);
`, graphqlMod)

	progPath := filepath.Join(dir, "gql.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("graphql program failed with code %d", code)
	}
}
