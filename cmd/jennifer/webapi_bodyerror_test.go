// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runCapturingStderr runs a program file and returns its exit code and what it
// wrote to stderr.
func runCapturingStderr(t *testing.T, path string) (int, string) {
	t.Helper()
	oldErr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	done := make(chan []byte)
	go func() {
		out, _ := io.ReadAll(r)
		done <- out
	}()
	code := runFile(path, nil, "")
	w.Close()
	os.Stderr = oldErr
	return code, string(<-done)
}

// TestWebapiBodyErrors: webapi reads a body only for the Content-Type it
// declares (a JSON body with a `%` passes the rules, a multipart upload is not
// parsed as a form), and a body that cannot be read reaches web's safety net:
// 400 (413 past a cap), not logged, while a handler fault is still the 500
// envelope and logged.
func TestWebapiBodyErrors(t *testing.T) {
	mods := map[string]string{}
	for _, m := range []string{"web", "webapi", "http", "validate"} {
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
use strings;
use encoding;
use binary;
import %q as web;
import %q as webapi;
import %q as http;
import %q as validate;

func create(ctx as web.Context) {
    def d as map of string to string init webapi.validated($api, $ctx);
    web.text($ctx, 200, "name=" + $d["name"]);
}

func rawJson(ctx as web.Context) {
    def v as json.Value init web.bodyJson($ctx);
    web.text($ctx, 200, json.encode($v));
}

func boom(ctx as web.Context) {
    throw Error{kind: "runtime", message: "a handler fault"};
}

def api as webapi.Api init webapi.new();
$api = webapi.mount($api, 1, "/v1");
def spec as webapi.Spec init webapi.public();
$spec.rules = {"name": [validate.required()]};
$api = webapi.post($api, "/items", create, $spec);
$api = webapi.post($api, "/raw", rawJson, webapi.public());
$api = webapi.post($api, "/boom", boom, webapi.public());
func apiGuard(ctx as web.Context) { return webapi.guard($api, $ctx); }
def app as web.App init web.new();
$app = webapi.install($api, $app, apiGuard);
def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def base as string init "http://" + httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };
def h as map of string to string init {};
def r as http.Response;

# A JSON body is read as JSON only: a "%%" is not a form escape.
$r = http.post($base + "/v1/items", "application/json", '{"name": "50%% off"}', $h);
testing.assertEqual($r.status, 200);
testing.assertEqual($r.body, "name=50%% off");
$r = http.post($base + "/v1/items", "application/json; charset=utf-8", '{"name": "a"}', $h);
testing.assertEqual($r.body, "name=a");
# A form body is read as a form.
$r = http.post($base + "/v1/items", "application/x-www-form-urlencoded", "name=x%%20y", $h);
testing.assertEqual($r.status, 200);
testing.assertEqual($r.body, "name=x y");
# A multipart upload with non-UTF-8 bytes is not parsed as a form.
def mp as bytes init binary.join([
    encoding.encode("utf-8", "--B\r\nContent-Disposition: form-data; name=\"f\"; filename=\"a.bin\"\r\n\r\n"),
    encoding.fromText("hex", "ff00fe"),
    encoding.encode("utf-8", "\r\n--B--\r\n")]);
$r = http.requestRawBody("POST", $base + "/v1/items?name=q",
    {"Content-Type": "multipart/form-data; boundary=B"}, $mp, 5000, 0);
testing.assertEqual($r.status, 200);
testing.assertEqual($r.body, "name=q");

# A body that cannot be read is web's 400, on a rules route and in a handler.
$r = http.post($base + "/v1/items", "application/json", '{"name": ', $h);
testing.assertEqual($r.status, 400);
testing.assertTrue(strings.startsWith($r.body, "400 web.bodyJson: malformed request body: "));
$r = http.post($base + "/v1/items", "application/x-www-form-urlencoded", "name=%%zz", $h);
testing.assertEqual($r.status, 400);
$r = http.post($base + "/v1/raw", "application/json", '{"a": ', $h);
testing.assertEqual($r.status, 400);
# Past a parser cap: 413.
def many as string init "";
for (def i in 0..10001) {
    $many = $many + "a=1&";
}
$r = http.post($base + "/v1/items", "application/x-www-form-urlencoded", $many, $h);
testing.assertEqual($r.status, 413);

# A handler fault is still the 500 envelope.
$r = http.post($base + "/v1/boom", "text/plain", "", $h);
testing.assertEqual($r.status, 500);
testing.assertEqual($r.body, '{"error":"internal server error"}');

httpd.shutdown($srv);
task.wait($server);
`, mods["web"], mods["webapi"], mods["http"], mods["validate"])

	progPath := filepath.Join(dir, "app.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stderr := runCapturingStderr(t, progPath)
	if code != 0 {
		t.Fatalf("webapi body-error program failed with code %d; stderr:\n%s", code, stderr)
	}
	if n := strings.Count(stderr, "web: unhandled handler error:"); n != 1 || !strings.Contains(stderr, "a handler fault") {
		t.Fatalf("want exactly the handler fault logged, got:\n%s", stderr)
	}
}
