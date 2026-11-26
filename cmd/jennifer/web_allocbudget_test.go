// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestWebAllocBudget is the end-to-end gate for web.allocBudget (M29.3): a
// handler that builds a value past the per-request budget is answered 500 (the
// server declining to spend more on the request, not the 413 of a client
// sending too much), a handler can catch the "limit" and answer itself, helper
// tasks a handler spawns count toward its request, a parser cap under the
// budget is still 413, and the server keeps answering the other requests,
// including ones running while a request trips.
func TestWebAllocBudget(t *testing.T) {
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
use math;
use convert;
import %q as web;
import %q as http;

def observed as string init "none";

func recordError(e as Error) {
    $observed = $e.kind;
}

func showObserved(ctx as web.Context) {
    web.text($ctx, 200, $observed);
}

func grow() {
    def xs as list of string init [];
    while (true) {
        $xs[] = "x";
    }
    return len($xs);
}

func big(ctx as web.Context) {
    grow();
    web.text($ctx, 200, "unreachable");
}

func caught(ctx as web.Context) {
    try {
        grow();
    } catch (e) {
        web.text($ctx, 503, $e.kind);
        return null;
    }
    web.text($ctx, 200, "unreachable");
}

# A helper task the handler spawns shares the request's budget.
func helper(ctx as web.Context) {
    def t as task of int init spawn { return grow(); };
    task.wait($t);
    web.text($ctx, 200, "unreachable");
}

func small(ctx as web.Context) {
    def xs as list of int init [];
    for (def i in 0..100) {
        $xs[] = $i;
    }
    web.text($ctx, 200, convert.toString(len($xs)) + " " + convert.toString(task.allocatedBytes() > 0));
}

func echoForm(ctx as web.Context) {
    web.text($ctx, 200, web.formValue($ctx, "a"));
}

def app as web.App init web.new();
$app = web.allocBudget($app, 4000000);
$app = web.onError($app, recordError);
$app = web.get($app, "/big", big);
$app = web.get($app, "/caught", caught);
$app = web.get($app, "/helper", helper);
$app = web.get($app, "/small", small);
$app = web.get($app, "/observed", showObserved);
$app = web.post($app, "/form", echoForm);

def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def base as string init "http://" + httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };
def h as map of string to string init {};
def r as http.Response;

$r = http.get($base + "/small", $h);
testing.assertEqual($r.status, 200);
testing.assertEqual($r.body, "100 true");

$r = http.get($base + "/big", $h);
testing.assertEqual($r.status, 500);
$r = http.get($base + "/observed", $h);
testing.assertEqual($r.body, "limit");

$r = http.get($base + "/caught", $h);
testing.assertEqual($r.status, 503);
testing.assertEqual($r.body, "limit");

$r = http.get($base + "/helper", $h);
testing.assertEqual($r.status, 500);

# Other requests, while one trips and after, are answered.
def bigs as list of task of int init [];
for (def i in 0..4) {
    $bigs[] = spawn { return http.get($base + "/big", $h).status; };
}
for (def i in 0..20) {
    $r = http.get($base + "/small", $h);
    testing.assertEqual($r.status, 200);
}
for (def b in task.waitAll($bigs)) {
    testing.assertEqual($b, 500);
}

# A parser cap under the budget is still the client's 413.
def many as string init "";
for (def i in 0..10001) {
    $many = $many + "a=1&";
}
$r = http.post($base + "/form", "application/x-www-form-urlencoded", $many, $h);
testing.assertEqual($r.status, 413);

# 0 or a negative budget is a misconfiguration.
def bad as bool init false;
try {
    web.allocBudget($app, 0);
} catch (e) {
    def we as web.WebError init $e;
    $bad = $we.kind == "web";
}
testing.assertTrue($bad);
# math.MAX_INT removes the budget.
testing.assertEqual(web.allocBudget($app, math.MAX_INT).allocBudget, 0);

httpd.shutdown($srv);
task.wait($server);
`, webMod, httpMod)

	progPath := filepath.Join(dir, "app.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("web allocation-budget program failed with code %d", code)
	}
}
