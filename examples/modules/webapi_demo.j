# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# webapi_demo.j - a JSON API over the `web` module using `webapi`: a versioned
# mount with a root alias, a public route, a bearer route with scopes + request
# validation + rate limiting, an error envelope, and pagination. It starts a
# server on an ephemeral port, drives itself as its own client, then shuts down.
#
#     jennifer run examples/modules/webapi_demo.j

use io;
use httpd;
use task;
use json;
use kv;
use convert;
import "../../modules/web.j" as web;
import "../../modules/webapi.j" as webapi;
import "../../modules/http.j" as http;
import "../../modules/validate.j" as validate;

# An in-process counter store the rate limiter increments (atomic; safe across
# the concurrently-served requests).
def store as kv.Store init kv.open();

# --- the app's own handlers (entry-program methods, dispatched by name) ------

# verifyToken is the authenticator: it turns a bearer token into an Identity. A
# real deployment would verify a JWT or look the token up; here two fixed tokens
# stand in. The module never learns how a token is checked.
func verifyToken(token as string) {
    if ($token == "admin-token") {
        return webapi.Identity{ok: true, subject: "u-admin", display: "admin", scopes: ["publish"]};
    }
    def none as webapi.Identity;
    return $none;
}

# countHit is the rate limiter: true while the per-key count is within the limit.
func countHit(key as string, limit as int) {
    kv.add($store, $key, "0", 0);
    return kv.incr($store, $key, 1) <= $limit;
}

func getDeck(ctx as web.Context) {
    webapi.sendJson($ctx, 200, json.set(json.map(), "/deck", web.param($ctx, "name")));
}

func listDecks(ctx as web.Context) {
    def pg as webapi.Page init webapi.page($ctx, 2, 50);
    def items as json.Value init json.list();
    def i as int init 0;
    while ($i < $pg.limit) {
        $items = json.append($items, "", "deck-" + convert.toString($pg.offset + $i));
        $i = $i + 1;
    }
    webapi.sendPage($ctx, $items, $pg, 231);
}

func publish(ctx as web.Context) {
    def who as webapi.Identity init webapi.identity($api, $ctx);
    def data as map of string to string init webapi.validated($api, $ctx);
    def doc as json.Value init json.set(json.map(), "/published", $data["tag"]);
    $doc = json.set($doc, "/by", $who.subject);
    webapi.sendJson($ctx, 201, $doc);
}

# The one guard shim: it binds the built Api into web's middleware chain. It is an
# entry-program handler (so it can see the global `$api`), which webapi.install
# wires as a `before` middleware - the honest wiring, since a func value crossing
# the module boundary could not capture the Api.
func apiGuard(ctx as web.Context) {
    return webapi.guard($api, $ctx);
}

# --- build the API -----------------------------------------------------------

def api as webapi.Api init webapi.new();
$api = webapi.mount($api, 1, "/v1");
$api = webapi.alias($api, 1);
$api = webapi.authenticator($api, "verifyToken");
$api = webapi.limiter($api, "countHit");

$api = webapi.get($api, "/deck/:name", "getDeck", webapi.public());
$api = webapi.get($api, "/decks", "listDecks", webapi.public());
$api = webapi.post($api, "/publish", "publish", webapi.Spec{
    summary: "publish a deck version",
    auth: webapi.Auth.Bearer,
    scopes: ["publish"],
    rules: {"tag": [validate.required(), validate.maxLen(16)]},
    rateLimit: 2,
    produces: webapi.Produces.Json
});
$api = webapi.feature($api, "publish-deck");

def app as web.App init web.new();
$app = webapi.install($api, $app, "apiGuard");

# The discovery document is derived from the route table, so it never drifts.
io.printf("discovery: %s\n", json.encode(webapi.discovery($api, "jennifer-registry", "1.0")));

# --- run + self-drive --------------------------------------------------------

def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def base as string init "http://" + httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };

def h as map of string to string init {};
def admin as map of string to string init {"Authorization": "Bearer admin-token"};
def body as string init '{"tag":"v2.1.0"}';

def deck as http.Response init http.get($base + "/v1/deck/routeros", $h);
io.printf("GET  /v1/deck/routeros   -> %d %s\n", $deck.status, $deck.body);

def rootDeck as http.Response init http.get($base + "/deck/routeros", $h);
io.printf("GET  /deck/routeros      -> %d (root alias)\n", $rootDeck.status);

def page as http.Response init http.get($base + "/v1/decks?limit=3&offset=9", $h);
io.printf("GET  /v1/decks           -> %d %s\n", $page.status, $page.body);

def anon as http.Response init http.post($base + "/v1/publish", "application/json", $body, $h);
io.printf("POST /v1/publish (anon)  -> %d %s\n", $anon.status, $anon.body);

def bad as http.Response init http.post($base + "/v1/publish", "application/json", '{}', $admin);
io.printf("POST /v1/publish (bad)   -> %d %s\n", $bad.status, $bad.body);

def okr as http.Response init http.post($base + "/v1/publish", "application/json", $body, $admin);
io.printf("POST /v1/publish (ok)    -> %d %s\n", $okr.status, $okr.body);

def dupe as http.Response init http.post($base + "/v1/publish", "application/json", $body, $admin);
io.printf("POST /v1/publish (2nd)   -> %d\n", $dupe.status);

def limited as http.Response init http.post($base + "/v1/publish", "application/json", $body, $admin);
io.printf("POST /v1/publish (3rd)   -> %d %s (rate limited)\n", $limited.status, $limited.body);

httpd.shutdown($srv);
task.wait($server);
io.printf("server stopped cleanly\n");
