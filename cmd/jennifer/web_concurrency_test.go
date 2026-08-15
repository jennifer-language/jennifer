// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestWebConcurrentRequestsDoNotHeadOfLineBlock is the regression gate for
// spawn-per-request serving: a slow handler must not stall the requests behind
// it. A background request occupies a worker in a ~1 s handler; a fast request
// issued 150 ms later must return in a few ms, not wait ~850 ms behind the slow
// one. Serial handling (the old behaviour) blocks the fast request until the
// slow handler finishes; concurrent handling answers it immediately. The 500 ms
// threshold cleanly separates the two regimes.
//
// This is also the concurrent-dispatch smoke test: two handlers run at once,
// both reaching the entry program via meta.callMain, so it doubles as a
// -race check on the whole web dispatch path.
func TestWebConcurrentRequestsDoNotHeadOfLineBlock(t *testing.T) {
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
use time;
import %q as web;
import %q as http;

func slowHandler(ctx as web.Context) {
    time.sleep(time.fromMilliseconds(1000));
    web.text($ctx, 200, "slow");
}
func fastHandler(ctx as web.Context) { web.text($ctx, 200, "fast"); }

def app as web.App init web.new();
$app = web.get($app, "/slow", slowHandler);
$app = web.get($app, "/fast", fastHandler);

def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def addr as string init httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };

def h as map of string to string init {};

# Occupy a worker with the slow request in the background.
def slowReq as task of http.Response init spawn { return http.get("http://" + $addr + "/slow", $h); };

# Let the slow request reach its handler before timing the fast one.
time.sleep(time.fromMilliseconds(150));

def t0 as time.Time init time.now();
def fast as http.Response init http.get("http://" + $addr + "/fast", $h);
def elapsedMs as int init time.milliseconds(time.sub(time.now(), $t0));
testing.assertEqual($fast.status, 200);
testing.assertEqual($fast.body, "fast");
# Serial handling would block the fast request behind the slow handler (~850ms
# remaining); concurrent handling answers in a few ms.
testing.assertTrue($elapsedMs < 500);

# Drain the slow request and shut the server down cleanly.
def slowResp as http.Response init task.wait($slowReq);
testing.assertEqual($slowResp.body, "slow");
httpd.shutdown($srv);
task.wait($server);
`, webMod, httpMod)

	progPath := filepath.Join(dir, "app.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("web concurrency probe failed with code %d", code)
	}
}
