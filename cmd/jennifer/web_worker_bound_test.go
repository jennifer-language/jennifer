// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// web.serveOn runs at most the server's maxInFlight workers at once. The engine
// frees a request's slot when it answers 503 after the respond timeout, while
// the worker handling it keeps running; without the bound, slow handlers under
// load kept accepting and spawning workers past the limit. Here the server
// allows 2 in flight with a 100 ms respond timeout, and 8 requests hit an 800 ms
// handler: no more than 2 handlers may ever run concurrently.
func TestWebWorkersBoundedByMaxInFlight(t *testing.T) {
	webMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "web.j"))
	if err != nil {
		t.Fatal(err)
	}
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	prog := fmt.Sprintf(`use testing;
use httpd;
use task;
use time;
use kv;
import %q as web;
import %q as http;

def store as kv.Store init kv.open();

func slow(ctx as web.Context) {
    def n as int init kv.incr($store, "active", 1);
    if (kv.incr($store, "peak", 0) < $n) {
        kv.set($store, "peak", "{$n}", 0);
    }
    time.sleep(time.fromMilliseconds(800));
    kv.incr($store, "active", -1);
    web.text($ctx, 200, "done");
}

kv.set($store, "active", "0", 0);
kv.set($store, "peak", "0", 0);
def app as web.App init web.new();
$app = web.get($app, "/slow", slow);
def srv as httpd.Server init httpd.listenWith("127.0.0.1:0", httpd.Options{maxInFlight: 2, respondTimeoutMs: 100});
def addr as string init httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };

def reqs as list of task of http.Response init [];
for (def i in 0..8) {
    $reqs[] = spawn { return http.get("http://" + $addr + "/slow", {}); };
}
task.waitAll($reqs);
# Let every worker that started finish before reading the peak.
time.sleep(time.fromMilliseconds(1200));
testing.assertTrue(kv.incr($store, "peak", 0) <= 2);
testing.assertTrue(kv.incr($store, "peak", 0) >= 1);
httpd.shutdown($srv);
task.wait($server);
`, webMod, httpMod)

	progPath := filepath.Join(t.TempDir(), "app.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("worker bound probe failed with code %d", code)
	}
}
