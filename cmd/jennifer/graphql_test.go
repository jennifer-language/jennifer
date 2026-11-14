// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

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
            def body as string init convert.stringFromBytes("utf-8", httpd.body($req));
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
    # a GraphqlError: the reason, the status, and the server's errors
    testing.assertEqual($e.reason, "graphql");
    testing.assertEqual($e.status, 200);
    testing.assertEqual($e.codes, ["", ""]);
    testing.assertEqual(json.asString($e.errors, "/1/message"), "and y");
}
testing.assertTrue($gotGqlErr);

# 2b. The codes of a raised error, from extensions.code.
def gotCode as bool init false;
try {
    graphql.query($c, "structErr", json.map());
} catch (e) {
    $gotCode = true;
    testing.assertEqual($e.codes, ["FORBIDDEN"]);
    testing.assertEqual(json.asString($e.errors, "/0/extensions/code"), "FORBIDDEN");
}
testing.assertTrue($gotCode);

# 3. HTTP error (non-2xx) -> a graphql error carrying the status and body.
def gotHttpErr as bool init false;
try {
    graphql.query($c, "causeHttp", json.map());
} catch (e) {
    $gotHttpErr = true;
    testing.assertEqual($e.kind, "graphql");
    testing.assertTrue(strings.contains($e.message, "500"));
    testing.assertEqual($e.reason, "http");
    testing.assertEqual($e.status, 500);
    testing.assertEqual(len($e.codes), 0);
    testing.assertTrue(json.isNull($e.errors, ""));
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

// TestGraphqlPolicy drives the request policy end to end: withRetries repeats
// a query answered 503 but never a mutation; the Accept header names the
// GraphQL-over-HTTP media type; a 400 that carries an errors envelope raises
// its messages with their codes (and tryQuery returns it); and a non-GraphQL
// error body is cut short and flattened in the message.
func TestGraphqlPolicy(t *testing.T) {
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

# The stub counts the flaky requests itself and reports the counts, with the
# last Accept header, when asked for "stats".
func serve(srv as httpd.Server) {
    def queries as int init 0;
    def mutations as int init 0;
    def accept as string init "";
    while (true) {
        try {
            def req as httpd.Request init httpd.accept($srv);
            def body as string init convert.stringFromBytes("utf-8", httpd.body($req));
            if (strings.contains($body, "flakyQ")) {
                $queries = $queries + 1;
                $accept = httpd.header($req, "Accept");
                httpd.respond($req, 503, "busy");
            } elseif (strings.contains($body, "flakyM")) {
                $mutations = $mutations + 1;
                httpd.respond($req, 503, "busy");
            } elseif (strings.contains($body, "stats")) {
                httpd.respond($req, 200, '{"data":{"q":' + convert.toString($queries) + ',"m":' + convert.toString($mutations) + ',"accept":' + json.encode($accept) + '}}');
            } elseif (strings.contains($body, "badQuery")) {
                httpd.respond($req, 400, '{"errors":[{"message":"Cannot query field nope","extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}]}');
            } else {
                httpd.respond($req, 502, "<html>\r\n" + strings.repeat("x", 1000));
            }
        } catch (acceptErr) {
            return;
        }
    }
}

def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def server as task of null init spawn { serve($srv); };
def c as graphql.Client init graphql.withBackoff(graphql.withRetries(graphql.client("http://" + httpd.address($srv)), 2), 1);

func fails(c as graphql.Client, doc as string) {
    try {
        graphql.query($c, $doc, json.map());
    } catch (e) {
        return $e.message;
    }
    return "";
}

# 1. A query is retried (1 + 2 attempts), a mutation sent once.
testing.assertTrue(strings.contains(fails($c, '{ flakyQ }'), "HTTP 503"));
testing.assertTrue(strings.contains(fails($c, 'mutation { flakyM }'), "HTTP 503"));
def stats as json.Value init graphql.query($c, '{ stats }', json.map());
testing.assertEqual(json.asInt($stats, "/data/q"), 3);
testing.assertEqual(json.asInt($stats, "/data/m"), 1);

# 2. The Accept header asks for the GraphQL-over-HTTP media type first.
testing.assertTrue(strings.startsWith(json.asString($stats, "/data/accept"), "application/graphql-response+json"));

# 3. A 400 with an errors envelope raises its message and code; tryQuery
#    returns the envelope.
testing.assertEqual(fails($c, '{ badQuery }'), "graphql: HTTP 400: Cannot query field nope [GRAPHQL_VALIDATION_FAILED]");
def env as json.Value init graphql.tryQuery($c, '{ badQuery }', json.map());
testing.assertEqual(graphql.errorCodes($env), ["GRAPHQL_VALIDATION_FAILED"]);

# 4. Any other error body is flattened and cut short.
def msg as string init fails($c, '{ proxy }');
testing.assertTrue(strings.startsWith($msg, "graphql: HTTP 502: <html> xxx"));
testing.assertTrue(len($msg) < 240);

httpd.shutdown($srv);
task.wait($server);
`, graphqlMod)

	progPath := filepath.Join(dir, "gqlpolicy.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("graphql policy program failed with code %d", code)
	}
}
