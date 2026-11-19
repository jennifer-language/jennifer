// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestWebapiRoundTrip drives the webapi module end-to-end against a real server
// the .j program starts itself: it exercises the guard's whole chain - public
// routes under a version and its root alias, bearer auth (401 no/bad token, 403
// wrong scope), request validation (422 with failures), success (201),
// rate limiting (429, counted before validation), pagination, a throwing
// handler (500 envelope, onError still called), Deprecation / Sunset headers,
// and one authenticator call per request. The authenticator and limiter are
// entry-program func values, and one guard shim binds the built Api into web
// as every route's handler - the honest no-closures wiring.
func TestWebapiRoundTrip(t *testing.T) {
	mods := map[string]string{}
	for _, m := range []string{"web", "webapi", "http", "validate", "ratelimit"} {
		p, err := filepath.Abs(filepath.Join("..", "..", "modules", m+".j"))
		if err != nil {
			t.Fatal(err)
		}
		mods[m] = p
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use httpd;
use task;
use json;
use kv;
use strings;
use convert;
import %q as web;
import %q as webapi;
import %q as http;
import %q as validate;
import %q as ratelimit;

def store as kv.Store init kv.open();
kv.set($store, "authCalls", "0", 0);

func verifyToken(token as string) {
    kv.incr($store, "authCalls", 1);
    if ($token == "admin-token") {
        return webapi.Identity{ok: true, subject: "u-admin", display: "admin", scopes: ["publish", "read"]};
    }
    if ($token == "reader-token") {
        return webapi.Identity{ok: true, subject: "u-reader", display: "reader", scopes: ["read"]};
    }
    def none as webapi.Identity;
    return $none;
}

# countHit counts per key and limit (one bucket per route limit) and denies past
# the limit with a fixed retryAfter, so the test can assert the Retry-After
# header the guard derives from it.
func countHit(key as string, limit as int) {
    def bucket as string init $key + "|" + convert.toString($limit);
    kv.add($store, $bucket, "0", 0);
    def n as int init kv.incr($store, $bucket, 1);
    if ($n <= $limit) {
        return ratelimit.Result{allowed: true, remaining: $limit - $n, retryAfter: 0, resetSeconds: 60};
    }
    return ratelimit.Result{allowed: false, remaining: 0, retryAfter: 42, resetSeconds: 42};
}

# clientKey keys an anonymous request on a test header when present (standing in
# for a trusted proxy's forwarding header), else the client host.
func clientKey(ctx as web.Context) {
    def c as string init web.header($ctx, "X-Test-Client");
    if ($c != "") {
        return "client:" + $c;
    }
    return web.remoteHost($ctx);
}

func ping(ctx as web.Context) {
    webapi.sendJson($ctx, 200, json.set(json.map(), "/ok", true));
}

func getDeck(ctx as web.Context) {
    webapi.sendJson($ctx, 200, json.set(json.map(), "/deck", web.param($ctx, "name")));
}

func listDecks(ctx as web.Context) {
    def pg as webapi.Page init webapi.page($ctx, 2, 5);
    def items as json.Value init json.list();
    def i as int init 0;
    while ($i < $pg.limit) {
        $items = json.append($items, "", "item" + convert.toString($pg.offset + $i));
        $i = $i + 1;
    }
    webapi.sendPage($ctx, $items, $pg, 10);
}

func publish(ctx as web.Context) {
    def who as webapi.Identity init webapi.identity($api, $ctx);
    def data as map of string to string init webapi.validated($api, $ctx);
    def doc as json.Value init json.set(json.map(), "/tag", $data["tag"]);
    $doc = json.set($doc, "/by", $who.subject);
    webapi.sendJson($ctx, 201, $doc);
}

func boom(ctx as web.Context) {
    throw Error{kind: "db", message: "secret connection string"};
}

func login(ctx as web.Context) {
    webapi.sendJson($ctx, 200, json.set(json.map(), "/ok", true));
}

func failed(err as Error) {
    kv.set($store, "hooked", $err.kind, 0);
}

func apiGuard(ctx as web.Context) { return webapi.guard($api, $ctx); }

def api as webapi.Api init webapi.new();
$api = webapi.mount($api, 1, "/v1");
$api = webapi.alias($api, 1);
$api = webapi.mount($api, 0, "/v0");
$api = webapi.deprecatedSince($api, 0, "2026-01-01");
$api = webapi.deprecate($api, 0, "2027-06-30T12:00:00+02:00");
$api = webapi.authenticator($api, verifyToken);
$api = webapi.limiter($api, countHit);
$api = webapi.limitKey($api, clientKey);
$api = webapi.get($api, "/ping", ping, webapi.Spec{
    summary: "anonymous, rate limited",
    auth: webapi.Auth.None,
    scopes: [],
    rules: {},
    rateLimit: 2,
    produces: webapi.Produces.Json
});
$api = webapi.get($api, "/deck/:name", getDeck, webapi.public());
$api = webapi.get($api, "/boom", boom, webapi.public());
$api = webapi.post($api, "/login", login, webapi.Spec{
    summary: "public, validated, rate limited",
    auth: webapi.Auth.None,
    scopes: [],
    rules: {"name": [validate.required()]},
    rateLimit: 2,
    produces: webapi.Produces.Json
});
$api = webapi.get($api, "/decks", listDecks, webapi.public());
$api = webapi.get($api, "/secret", ping, webapi.Spec{
    summary: "bearer-protected GET",
    auth: webapi.Auth.Bearer,
    scopes: ["publish"],
    rules: {},
    rateLimit: 0,
    produces: webapi.Produces.Json
});
$api = webapi.post($api, "/publish", publish, webapi.Spec{
    summary: "publish a deck version",
    auth: webapi.Auth.Bearer,
    scopes: ["publish"],
    rules: {"tag": [validate.required(), validate.maxLen(8)]},
    rateLimit: 3,
    produces: webapi.Produces.Json
});

def app as web.App init web.new();
$app = webapi.install($api, $app, apiGuard);
$app = webapi.onError($app, failed);

def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def base as string init "http://" + httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };

def h as map of string to string init {};
def adminH as map of string to string init {"Authorization": "Bearer admin-token"};
def readerH as map of string to string init {"Authorization": "Bearer reader-token"};
def badH as map of string to string init {"Authorization": "Bearer nope"};
def jbody as string init '{"tag":"v1"}';

# public route under the version and the root alias
def d1 as http.Response init http.get($base + "/v1/deck/foo", $h);
testing.assertEqual($d1.status, 200);
testing.assertTrue(strings.contains($d1.body, "foo"));
testing.assertEqual(http.get($base + "/deck/foo", $h).status, 200);

# pagination: limit clamped to maxLimit 5, offset/total echoed
def pgResp as http.Response init http.get($base + "/v1/decks?limit=3&offset=6", $h);
testing.assertEqual($pgResp.status, 200);
def pgd as json.Value init json.decode($pgResp.body);
testing.assertEqual(json.asInt($pgd, "/total"), 10);
testing.assertEqual(json.asInt($pgd, "/limit"), 3);
testing.assertEqual(json.asInt($pgd, "/offset"), 6);
def pgClamp as json.Value init json.decode(http.get($base + "/v1/decks?limit=999", $h).body);
testing.assertEqual(json.asInt($pgClamp, "/limit"), 5);
# a malformed or out-of-range limit / offset is a 400 envelope from the guard,
# not a silent default (and not a 500 from the handler)
for (def q in ["limit=abc", "limit=0", "limit=-1", "offset=-5", "offset=x", "offset=99999999999999999999"]) {
    def badPage as http.Response init http.get($base + "/v1/decks?" + $q, $h);
    testing.assertEqual($badPage.status, 400);
    testing.assertTrue(strings.contains($badPage.body, "failures"));
}

# HEAD is served by the GET route, so it is guarded by the GET route's spec
testing.assertEqual(http.get($base + "/v1/secret", $h).status, 401);
testing.assertEqual(http.request("HEAD", $base + "/v1/secret", $h, "").status, 401);
testing.assertEqual(http.request("HEAD", $base + "/v1/secret", $adminH, "").status, 200);

# bearer route: no token -> 401 with challenge
def noTok as http.Response init http.post($base + "/v1/publish", "application/json", $jbody, $h);
testing.assertEqual($noTok.status, 401);
testing.assertTrue(strings.contains($noTok.body, "error"));

# bad token -> 401
testing.assertEqual(http.post($base + "/v1/publish", "application/json", $jbody, $badH).status, 401);

# valid token but missing scope -> 403
testing.assertEqual(http.post($base + "/v1/publish", "application/json", $jbody, $readerH).status, 403);

# admin token but invalid body (missing required tag) -> 422 with failures
def v422 as http.Response init http.post($base + "/v1/publish", "application/json", '{}', $adminH);
testing.assertEqual($v422.status, 422);
testing.assertTrue(strings.contains($v422.body, "failures"));
testing.assertTrue(strings.contains($v422.body, "tag"));

# admin token + valid body -> 201 (rate hit 2: the 422 above counted), and the
# handler's webapi.identity reuses the guard's identity: one authenticator call
def callsBefore as int init kv.incr($store, "authCalls", 0);
def ok1 as http.Response init http.post($base + "/v1/publish", "application/json", $jbody, $adminH);
testing.assertEqual($ok1.status, 201);
testing.assertTrue(strings.contains($ok1.body, "u-admin"));
testing.assertEqual(kv.incr($store, "authCalls", 0), $callsBefore + 1);

# rate limit is 3 per subject: hit 3 passes, hit 4 -> 429
testing.assertEqual(http.post($base + "/v1/publish", "application/json", $jbody, $adminH).status, 201);
def limited as http.Response init http.post($base + "/v1/publish", "application/json", $jbody, $adminH);
testing.assertEqual($limited.status, 429);
testing.assertEqual(http.header($limited, "Retry-After"), "42");

# anonymous: every http.get is a new connection from a new source port, so the
# key must be the client host, not host:port - the third request is limited
testing.assertEqual(http.get($base + "/v1/ping", $h).status, 200);
testing.assertEqual(http.get($base + "/v1/ping", $h).status, 200);
def pingLimited as http.Response init http.get($base + "/v1/ping", $h);
testing.assertEqual($pingLimited.status, 429);
testing.assertEqual(http.header($pingLimited, "Retry-After"), "42");

# the key function: another client key gets its own bucket
def otherH as map of string to string init {"X-Test-Client": "b"};
testing.assertEqual(http.get($base + "/v1/ping", $otherH).status, 200);
testing.assertEqual(http.get($base + "/v1/ping", $otherH).status, 200);
testing.assertEqual(http.get($base + "/v1/ping", $otherH).status, 429);

# requests failing validation are counted too: a login flood is limited
def floodH as map of string to string init {"X-Test-Client": "flood"};
testing.assertEqual(http.post($base + "/v1/login", "application/json", '{}', $floodH).status, 422);
testing.assertEqual(http.post($base + "/v1/login", "application/json", '{}', $floodH).status, 422);
testing.assertEqual(http.post($base + "/v1/login", "application/json", '{}', $floodH).status, 429);
testing.assertEqual(
    http.post($base + "/v1/login", "application/json", '{"name":"a"}', $floodH).status, 429);

# a throwing handler: a 500 in the envelope, without the error's details, and
# web's onError hook still gets the error
def boomR as http.Response init http.get($base + "/v1/boom", $h);
testing.assertEqual($boomR.status, 500);
testing.assertEqual(json.asString(json.decode($boomR.body), "/error"), "internal server error");
testing.assertFalse(strings.contains($boomR.body, "secret"));
testing.assertEqual(kv.get($store, "hooked"), "db");

# a deprecated version: Deprecation (RFC 9745) and Sunset (RFC 8594) headers;
# the current version carries neither
def old as http.Response init http.get($base + "/v0/deck/foo", $h);
testing.assertEqual($old.status, 200);
testing.assertEqual(http.header($old, "Deprecation"), "@1767225600");
testing.assertEqual(http.header($old, "Sunset"), "Wed, 30 Jun 2027 10:00:00 GMT");
def cur as http.Response init http.get($base + "/v1/deck/foo", $h);
testing.assertEqual(http.header($cur, "Deprecation"), "");
testing.assertEqual(http.header($cur, "Sunset"), "");

httpd.shutdown($srv);
task.wait($server);`, mods["web"], mods["webapi"], mods["http"], mods["validate"], mods["ratelimit"])

	progPath := filepath.Join(dir, "apiapp.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("webapi round-trip program failed with code %d", code)
	}
}
