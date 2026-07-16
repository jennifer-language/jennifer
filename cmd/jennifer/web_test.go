// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// A .j program builds a small app with the web framework, serves it on an
// ephemeral port in a spawned task, and acts as its own client. It asserts the
// full round-trip: an exact-route text response, a `:param` route returning
// JSON with the captured parameter, a middleware-set response header, and a 404
// for an unmatched path. This exercises the whole chain - httpd engine, route
// matching, and cross-module handler dispatch via meta.callMain (a web.Context
// built inside the module reaching a handler defined in this entry program).
func TestWebFrameworkRoundTrip(t *testing.T) {
	webMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "web.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use httpd;
use task;
use json;
import %q as web;
import %q as http;

func showHome(ctx as web.Context) { web.text($ctx, 200, "home"); }
func showUser(ctx as web.Context) {
    def out as json.Value init json.map();
    $out = json.set($out, "/id", web.param($ctx, "id"));
    $out = json.set($out, "/method", web.method($ctx));
    web.sendJson($ctx, 200, $out);
}
func tagHeader(ctx as web.Context) { web.setHeader($ctx, "X-Powered-By", "jennifer/web"); return true; }

def app as web.App init web.new();
$app = web.before($app, "tagHeader");
$app = web.get($app, "/", "showHome");
$app = web.get($app, "/users/:id", "showUser");

def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def addr as string init httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };

def h as map of string to string init {};
def home as http.Response init http.get("http://" + $addr + "/", $h);
testing.assertEqual($home.status, 200);
testing.assertEqual($home.body, "home");
testing.assertEqual(http.header($home, "X-Powered-By"), "jennifer/web");

def user as http.Response init http.get("http://" + $addr + "/users/42", $h);
testing.assertEqual($user.status, 200);
def doc as json.Value init json.decode($user.body);
testing.assertEqual(json.asString($doc, "/id"), "42");
testing.assertEqual(json.asString($doc, "/method"), "GET");

def miss as http.Response init http.get("http://" + $addr + "/nope", $h);
testing.assertEqual($miss.status, 404);

httpd.shutdown($srv);
task.wait($server);`, webMod, httpMod)

	progPath := filepath.Join(dir, "app.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("web framework program failed with code %d", code)
	}
}

// TestWebMiddlewareThrowKeepsServing proves a throwing middleware is contained
// to its own request (answered 500) and does NOT shut the server down: a
// following normal request still succeeds. Before the fix, a middleware throw
// propagated to serveOn's catch-all, which read any error as a shutdown signal.
func TestWebMiddlewareThrowKeepsServing(t *testing.T) {
	webMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "web.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use httpd;
use task;
import %q as web;
import %q as http;

# A middleware that throws only for the /boom path; returns true otherwise.
func guard(ctx as web.Context) {
    if (web.path($ctx) == "/boom") {
        throw Error{kind: "test", message: "middleware boom", file: "", line: 0, col: 0};
    }
    return true;
}
func showHome(ctx as web.Context) { web.text($ctx, 200, "home"); }

def app as web.App init web.new();
$app = web.before($app, "guard");
$app = web.get($app, "/", "showHome");
$app = web.get($app, "/boom", "showHome");

def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def addr as string init httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };

def h as map of string to string init {};
# The throwing middleware request is contained: it returns 500, not a dropped
# connection.
def boom as http.Response init http.get("http://" + $addr + "/boom", $h);
testing.assertEqual($boom.status, 500);
# The server is still up: a normal request afterwards still succeeds.
def home as http.Response init http.get("http://" + $addr + "/", $h);
testing.assertEqual($home.status, 200);
testing.assertEqual($home.body, "home");

httpd.shutdown($srv);
task.wait($server);`, webMod, httpMod)

	progPath := filepath.Join(dir, "app.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("web middleware-throw program failed with code %d", code)
	}
}

// TestWebCookiesAndSession drives the cookie + session-id surface end to end: a
// handler resolves the session id via web.sessionId (minting + Set-Cookie on
// first use), counting hits in an app-owned store keyed by the id. The .j
// program acts as its own client: a first cookieless GET mints a session
// (counter 1, HttpOnly Set-Cookie), a second GET replaying that cookie hits the
// same session (counter 2), and a third cookieless GET starts a fresh one
// (counter 1). Proves web owns only the id cookie while the store stays the
// app's.
func TestWebCookiesAndSession(t *testing.T) {
	webMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "web.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use httpd;
use task;
use convert;
use maps;
use strings;
import %q as web;
import %q as http;

# The app owns its session store (here a simple in-program map keyed by id);
# web only manages the id cookie.
def store as map of string to int init {};

func hit(ctx as web.Context) {
    def id as string init web.sessionId($ctx, "sid");
    def n as int init 0;
    if (maps.has($store, $id)) { $n = $store[$id]; }
    $n = $n + 1;
    $store[$id] = $n;
    web.text($ctx, 200, convert.toString($n));
}

def app as web.App init web.new();
$app = web.get($app, "/hit", "hit");
def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def addr as string init httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };

def h as map of string to string init {};
def first as http.Response init http.get("http://" + $addr + "/hit", $h);
testing.assertEqual($first.status, 200);
testing.assertEqual($first.body, "1");
def sc as string init http.header($first, "Set-Cookie");
testing.assertTrue(strings.startsWith($sc, "sid="));
testing.assertTrue(strings.contains($sc, "HttpOnly"));

# Replay the cookie: same session, counter advances.
def pair as string init strings.substring($sc, 0, strings.indexOf($sc, ";"));
def hc as map of string to string init {};
$hc["Cookie"] = $pair;
def second as http.Response init http.get("http://" + $addr + "/hit", $hc);
testing.assertEqual($second.body, "2");

# No cookie: a fresh session starts over.
def third as http.Response init http.get("http://" + $addr + "/hit", $h);
testing.assertEqual($third.body, "1");

httpd.shutdown($srv);
task.wait($server);`, webMod, httpMod)

	progPath := filepath.Join(dir, "sess.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("web cookie/session program failed with code %d", code)
	}
}

// TestWebCors drives the CORS policy end to end: with web.cors set, a normal GET
// carries the Access-Control-Allow-Origin header, and an OPTIONS preflight is
// answered 204 with the configured Allow-Methods (before any route runs). Also
// exercises cross-module zero-value construction (`def o as web.CorsOptions;`).
func TestWebCors(t *testing.T) {
	webMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "web.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use httpd;
use task;
import %q as web;
import %q as http;

func ok(ctx as web.Context) { web.text($ctx, 200, "ok"); }

def opts as web.CorsOptions;
$opts.allowOrigin = "*";
$opts.allowMethods = "GET, POST, OPTIONS";
$opts.allowHeaders = "Content-Type";
$opts.maxAge = 600;

def app as web.App init web.new();
$app = web.cors($app, $opts);
$app = web.get($app, "/", "ok");

def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def addr as string init httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };

def h as map of string to string init {};
def get as http.Response init http.get("http://" + $addr + "/", $h);
testing.assertEqual($get.status, 200);
testing.assertEqual($get.body, "ok");
testing.assertEqual(http.header($get, "Access-Control-Allow-Origin"), "*");

# A genuine CORS preflight carries Origin + Access-Control-Request-Method.
def ph as map of string to string init {"Origin": "http://example.com", "Access-Control-Request-Method": "POST"};
def pre as http.Response init http.request("OPTIONS", "http://" + $addr + "/", $ph, "");
testing.assertEqual($pre.status, 204);
testing.assertEqual(http.header($pre, "Access-Control-Allow-Methods"), "GET, POST, OPTIONS");
testing.assertEqual(http.header($pre, "Access-Control-Max-Age"), "600");

# A plain OPTIONS (no preflight headers) is NOT hijacked as CORS: it routes
# normally, so with no OPTIONS handler registered it 404s.
def plain as http.Response init http.request("OPTIONS", "http://" + $addr + "/", $h, "");
testing.assertEqual($plain.status, 404);

httpd.shutdown($srv);
task.wait($server);`, webMod, httpMod)

	progPath := filepath.Join(dir, "cors.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("web cors program failed with code %d", code)
	}
}

// TestWebEtag drives the conditional-GET helper: a first GET returns 200 with an
// ETag header; a second GET replaying that ETag in If-None-Match is answered
// 304 with an empty body (web.etag short-circuits before the handler's body).
func TestWebEtag(t *testing.T) {
	webMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "web.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use httpd;
use task;
import %q as web;
import %q as http;

func page(ctx as web.Context) {
    if (web.etag($ctx, "v1")) { return; }
    web.text($ctx, 200, "hello");
}

def app as web.App init web.new();
$app = web.get($app, "/", "page");
def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def addr as string init httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };

def h as map of string to string init {};
def first as http.Response init http.get("http://" + $addr + "/", $h);
testing.assertEqual($first.status, 200);
testing.assertEqual($first.body, "hello");
testing.assertEqual(http.header($first, "ETag"), "\"v1\"");

def hc as map of string to string init {};
$hc["If-None-Match"] = "\"v1\"";
def second as http.Response init http.get("http://" + $addr + "/", $hc);
testing.assertEqual($second.status, 304);
testing.assertEqual(len($second.body), 0);

httpd.shutdown($srv);
task.wait($server);`, webMod, httpMod)

	progPath := filepath.Join(dir, "etag.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("web etag program failed with code %d", code)
	}
}

// TestWebAuth drives the server-side auth helpers: a Basic-gated route returns
// 401 + a WWW-Authenticate challenge with no credentials and 200 with the right
// ones (built by the .j client via encoding), and a route echoes the extracted
// bearer token. The credential check stays app code; web only parses the header.
func TestWebAuth(t *testing.T) {
	webMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "web.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use httpd;
use task;
use strings;
use convert;
use encoding;
import %q as web;
import %q as http;

func secret(ctx as web.Context) {
    def cred as web.BasicCredentials init web.basicAuth($ctx);
    if ($cred.present and $cred.user == "admin" and $cred.password == "s3cret") {
        web.text($ctx, 200, "welcome " + $cred.user);
        return;
    }
    web.setHeader($ctx, "WWW-Authenticate", "Basic realm=\"demo\"");
    web.text($ctx, 401, "unauthorized");
}
func token(ctx as web.Context) { web.text($ctx, 200, "token=" + web.bearerToken($ctx)); }

def app as web.App init web.new();
$app = web.get($app, "/secret", "secret");
$app = web.get($app, "/token", "token");
def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def addr as string init httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };

def none as map of string to string init {};
def denied as http.Response init http.get("http://" + $addr + "/secret", $none);
testing.assertEqual($denied.status, 401);
testing.assertTrue(strings.contains(http.header($denied, "WWW-Authenticate"), "Basic"));

def encoded as string init encoding.toText(convert.bytesFromString("admin:s3cret", "utf-8"), "base64");
def auth as map of string to string init {};
$auth["Authorization"] = "Basic " + $encoded;
def okResp as http.Response init http.get("http://" + $addr + "/secret", $auth);
testing.assertEqual($okResp.status, 200);
testing.assertEqual($okResp.body, "welcome admin");

def bt as map of string to string init {};
$bt["Authorization"] = "Bearer xyz.tok";
def tokResp as http.Response init http.get("http://" + $addr + "/token", $bt);
testing.assertEqual($tokResp.body, "token=xyz.tok");

httpd.shutdown($srv);
task.wait($server);`, webMod, httpMod)

	progPath := filepath.Join(dir, "auth.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("web auth program failed with code %d", code)
	}
}

// TestWebFormAndResponses drives the request/response helpers: a form POST read
// via web.formValue, a JSON POST read via web.bodyJson, an html response
// (text/html), a redirect (302 + Location), and remoteAddr. The .j program is
// its own client.
func TestWebFormAndResponses(t *testing.T) {
	webMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "web.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use httpd;
use task;
use strings;
use convert;
use json;
import %q as web;
import %q as http;

func submit(ctx as web.Context) { web.text($ctx, 200, "hi " + web.formValue($ctx, "name")); }
func api(ctx as web.Context) {
    def doc as json.Value init web.bodyJson($ctx);
    web.text($ctx, 200, "x=" + convert.toString(json.asInt($doc, "/x")));
}
func page(ctx as web.Context) { web.html($ctx, 200, "<h1>ok</h1>"); }
func old(ctx as web.Context) { web.redirect($ctx, 302, "/new"); }
func ip(ctx as web.Context) { web.text($ctx, 200, web.remoteAddr($ctx)); }

def app as web.App init web.new();
$app = web.post($app, "/submit", "submit");
$app = web.post($app, "/api", "api");
$app = web.get($app, "/page", "page");
$app = web.get($app, "/old", "old");
$app = web.get($app, "/ip", "ip");
def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def addr as string init httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };
def base as string init "http://" + $addr;
def h as map of string to string init {};

def form as http.Response init http.post($base + "/submit", "application/x-www-form-urlencoded", "name=Jane+Doe", $h);
testing.assertEqual($form.body, "hi Jane Doe");

def apiResp as http.Response init http.post($base + "/api", "application/json", "{\"x\":5}", $h);
testing.assertEqual($apiResp.body, "x=5");

def pageResp as http.Response init http.get($base + "/page", $h);
testing.assertTrue(strings.contains(http.header($pageResp, "Content-Type"), "text/html"));

def red as http.Response init http.get($base + "/old", $h);
testing.assertEqual($red.status, 302);
testing.assertEqual(http.header($red, "Location"), "/new");

def ipResp as http.Response init http.get($base + "/ip", $h);
testing.assertTrue(strings.startsWith($ipResp.body, "127.0.0.1"));

httpd.shutdown($srv);
task.wait($server);`, webMod, httpMod)

	progPath := filepath.Join(dir, "reqresp.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("web form/response program failed with code %d", code)
	}
}

// TestWebMultipartForm drives a multipart/form-data upload through
// web.multipartForm: the .j client builds a form (a field + a file) with the
// multipart module and POSTs it; the handler parses it back with
// web.multipartForm and echoes a summary. This also exercises cross-module
// struct identity - a multipart.Part flows entry -> web -> multipart and back.
func TestWebMultipartForm(t *testing.T) {
	webMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "web.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	mpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "multipart.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use httpd;
use task;
use convert;
import %q as web;
import %q as http;
import %q as multipart;

func upload(ctx as web.Context) {
    def items as list of multipart.Part init web.multipartForm($ctx);
    def out as string init "";
    for (def p in $items) {
        if (multipart.isFile($p)) {
            $out = $out + "file:" + $p.name + "=" + $p.filename + "(" + convert.toString(len($p.data)) + ");";
        } else {
            $out = $out + "field:" + $p.name + "=" + multipart.text($p) + ";";
        }
    }
    web.text($ctx, 200, $out);
}

def app as web.App init web.new();
$app = web.post($app, "/upload", "upload");
def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def addr as string init httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };
def base as string init "http://" + $addr;

def parts as list of multipart.Part init [
    multipart.field("name", "Jane Doe"),
    multipart.file("doc", "a.txt", "text/plain", convert.bytesFromString("hello", "utf-8"))
];
def form as multipart.Built init multipart.buildWith($parts, "TESTBND");
def h as map of string to string init {};
def resp as http.Response init http.post($base + "/upload", $form.contentType, convert.stringFromBytes($form.body, "utf-8"), $h);
testing.assertEqual($resp.body, "field:name=Jane Doe;file:doc=a.txt(5);");

httpd.shutdown($srv);
task.wait($server);`, webMod, httpMod, mpMod)

	progPath := filepath.Join(dir, "multipart.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("web multipart program failed with code %d", code)
	}
}

// TestWebCsrf drives the CSRF flow: a GET mints a token (returned in the body,
// set in the csrf cookie); a guarded POST replaying the token in X-CSRF-Token
// plus the cookie is accepted, and a POST with neither is rejected 403.
func TestWebCsrf(t *testing.T) {
	webMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "web.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use httpd;
use task;
use strings;
import %q as web;
import %q as http;

def const SECRET as string init "topsecret";

func mint(ctx as web.Context) { web.text($ctx, 200, web.csrfToken($ctx, SECRET)); }
func submit(ctx as web.Context) {
    if (web.csrfCheck($ctx, SECRET)) {
        web.text($ctx, 200, "ok");
        return;
    }
    web.text($ctx, 403, "forbidden");
}

def app as web.App init web.new();
$app = web.get($app, "/form", "mint");
$app = web.post($app, "/submit", "submit");
def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def addr as string init httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };
def base as string init "http://" + $addr;
def none as map of string to string init {};

def minted as http.Response init http.get($base + "/form", $none);
testing.assertEqual($minted.status, 200);
def token as string init $minted.body;
def sc as string init http.header($minted, "Set-Cookie");
def pair as string init strings.substring($sc, 0, strings.indexOf($sc, ";"));

def h as map of string to string init {};
$h["X-CSRF-Token"] = $token;
$h["Cookie"] = $pair;
def okResp as http.Response init http.post($base + "/submit", "text/plain", "", $h);
testing.assertEqual($okResp.status, 200);
testing.assertEqual($okResp.body, "ok");

def denied as http.Response init http.post($base + "/submit", "text/plain", "", $none);
testing.assertEqual($denied.status, 403);

httpd.shutdown($srv);
task.wait($server);`, webMod, httpMod)

	progPath := filepath.Join(dir, "csrf.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("web csrf program failed with code %d", code)
	}
}

// TestWebWildcard drives wildcard routes live: a `/static/*path` route captures
// the nested remainder, and a `/*page` catch-all (registered last) is the SPA
// fallback for everything else, including "/".
func TestWebWildcard(t *testing.T) {
	webMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "web.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use httpd;
use task;
import %q as web;
import %q as http;

func files(ctx as web.Context) { web.text($ctx, 200, "path=" + web.param($ctx, "path")); }
func fallback(ctx as web.Context) { web.text($ctx, 200, "spa:" + web.param($ctx, "page")); }

def app as web.App init web.new();
$app = web.get($app, "/static/*path", "files");
$app = web.get($app, "/*page", "fallback");
def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def addr as string init httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };
def base as string init "http://" + $addr;
def h as map of string to string init {};

def asset as http.Response init http.get($base + "/static/css/app.css", $h);
testing.assertEqual($asset.body, "path=css/app.css");

def spa as http.Response init http.get($base + "/some/deep/route", $h);
testing.assertEqual($spa.body, "spa:some/deep/route");

def root as http.Response init http.get($base + "/", $h);
testing.assertEqual($root.body, "spa:");

httpd.shutdown($srv);
task.wait($server);`, webMod, httpMod)

	progPath := filepath.Join(dir, "wild.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("web wildcard program failed with code %d", code)
	}
}
