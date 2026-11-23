// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestWebMalformedBody is the end-to-end gate for M29.29: a request body that
// cannot be read is the client's fault, answered 400 by the safety net (a
// web.WebError), a body past a parser cap 413, and neither is the 500 a
// handler fault gets. A handler that catches the WebError answers as it says;
// the onError hook sees it as an Error of kind "web".
func TestWebMalformedBody(t *testing.T) {
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
use strings;
use convert;
use encoding;
import %q as web;
import %q as http;

def observed as string init "none";

func recordError(e as Error) {
    $observed = $e.kind;
}

func showObserved(ctx as web.Context) {
    web.text($ctx, 200, $observed);
}

func echoJson(ctx as web.Context) {
    def v as json.Value init web.bodyJson($ctx);
    web.text($ctx, 200, json.encode($v));
}

func echoForm(ctx as web.Context) {
    web.text($ctx, 200, web.formValue($ctx, "a"));
}

func countParts(ctx as web.Context) {
    web.text($ctx, 200, convert.toString(len(web.multipartForm($ctx))));
}

# A handler that catches the WebError answers as it chooses.
func ownAnswer(ctx as web.Context) {
    try {
        web.bodyJson($ctx);
        web.text($ctx, 200, "ok");
    } catch (e) {
        def we as web.WebError init $e;
        web.text($ctx, 422, $we.kind + " " + convert.toString($we.status));
    }
}

func csrfGate(ctx as web.Context) {
    if (not web.csrfCheck($ctx, "s3cret")) {
        web.text($ctx, 403, "forbidden");
        return false;
    }
    return true;
}

func boom(ctx as web.Context) {
    throw Error{kind: "runtime", message: "a handler fault"};
}

def app as web.App init web.new();
$app = web.onError($app, recordError);
$app = web.post($app, "/json", echoJson);
$app = web.post($app, "/form", echoForm);
$app = web.post($app, "/parts", countParts);
$app = web.post($app, "/own", ownAnswer);
$app = web.post($app, "/boom", boom);
$app = web.get($app, "/observed", showObserved);

def guarded as web.App init web.new();
$guarded = web.before($guarded, csrfGate);
$guarded = web.post($guarded, "/form", echoForm);

def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def base as string init "http://" + httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };
def srv2 as httpd.Server init httpd.listen("127.0.0.1:0");
def base2 as string init "http://" + httpd.address($srv2);
def server2 as task of null init spawn { web.serveOn($guarded, $srv2); };

def h as map of string to string init {};
def formCT as string init "application/x-www-form-urlencoded";
def r as http.Response;

# Valid bodies are unchanged.
$r = http.post($base + "/json", "application/json", "[1,2]", $h);
testing.assertEqual($r.status, 200);
testing.assertEqual($r.body, "[1,2]");
$r = http.post($base + "/form", $formCT, "a=x%%20y", $h);
testing.assertEqual($r.status, 200);
testing.assertEqual($r.body, "x y");

# Each malformed body kind is a 400 naming the reason.
$r = http.post($base + "/json", "application/json", "[1,", $h);
testing.assertEqual($r.status, 400);
testing.assertTrue(strings.startsWith($r.body, "400 web.bodyJson: malformed request body: "));
$r = http.post($base + "/form", $formCT, "a=%%zz", $h);
testing.assertEqual($r.status, 400);
testing.assertTrue(strings.startsWith($r.body, "400 web.form: malformed request body: "));
$r = http.post($base + "/form", $formCT, "a=100%%", $h);
testing.assertEqual($r.status, 400);
$r = http.requestRawBody("POST", $base + "/form", {"Content-Type": $formCT}, encoding.fromText("hex", "613dff"), 5000, 0);
testing.assertEqual($r.status, 400);
$r = http.requestRawBody("POST", $base + "/json", {"Content-Type": "application/json"}, encoding.fromText("hex", "22ff22"), 5000, 0);
testing.assertEqual($r.status, 400);
$r = http.post($base + "/parts", "multipart/form-data; boundary=B", "--Bx\r\n--B--", $h);
testing.assertEqual($r.status, 400);
testing.assertTrue(strings.startsWith($r.body, "400 web.multipartForm: malformed request body: "));

# The onError hook sees the client fault as kind "web".
$r = http.get($base + "/observed", $h);
testing.assertEqual($r.body, "web");

# A caught WebError is answered as the handler says.
$r = http.post($base + "/own", "application/json", "[1,", $h);
testing.assertEqual($r.status, 422);
testing.assertEqual($r.body, "web 400");

# Past a parser cap: 413.
def many as string init "";
for (def i in 0..10001) {
    $many = $many + "a=1&";
}
$r = http.post($base + "/form", $formCT, $many, $h);
testing.assertEqual($r.status, 413);
testing.assertEqual($r.body, "413 content too large\n");

# A handler fault is still a 500.
$r = http.post($base + "/boom", "text/plain", "", $h);
testing.assertEqual($r.status, 500);

# csrfCheck on a malformed form body refuses with 400, not 500.
$r = http.post($base2 + "/form", $formCT, "csrf=%%zz", $h);
testing.assertEqual($r.status, 400);

httpd.shutdown($srv);
task.wait($server);
httpd.shutdown($srv2);
task.wait($server2);
`, webMod, httpMod)

	progPath := filepath.Join(dir, "app.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("web malformed-body program failed with code %d", code)
	}
}
