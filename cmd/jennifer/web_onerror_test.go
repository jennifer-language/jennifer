// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestWebOnErrorObservesTypedError is the end-to-end gate for the web.onError
// hook. A throwing route hands the thrown Error to the registered onError
// handler, which binds it `as Error` (only possible once Error crosses
// meta.callMain intact) and records its kind/message; a second route reports
// what was recorded. The old test asserted only that the handler *name* was
// stored, which is how a dead hook (unbindable Error, handler never observing
// anything) shipped green. This asserts the handler actually observed the
// error's typed fields.
func TestWebOnErrorObservesTypedError(t *testing.T) {
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

def observedKind as string init "none";
def observedMsg as string init "none";

func boom(ctx as web.Context) {
    throw Error{ kind: "web", message: "kaboom", file: "", line: 0, col: 0 };
}

# The onError hook: binds the thrown value as Error (the step-1 fix) and records
# its typed fields. If Error were still stamped with the module identity across
# meta.callMain, this parameter would fail to bind and nothing would be recorded.
func recordError(e as Error) {
    $observedKind = $e.kind;
    $observedMsg = $e.message;
}

func showObserved(ctx as web.Context) {
    web.text($ctx, 200, $observedKind + ":" + $observedMsg);
}

def app as web.App init web.new();
$app = web.onError($app, "recordError");
$app = web.get($app, "/boom", "boom");
$app = web.get($app, "/observed", "showObserved");

def srv as httpd.Server init httpd.listen("127.0.0.1:0");
def addr as string init httpd.address($srv);
def server as task of null init spawn { web.serveOn($app, $srv); };

def h as map of string to string init {};

# The throwing route answers 500; the onError hook records the typed error.
def r1 as http.Response init http.get("http://" + $addr + "/boom", $h);
testing.assertEqual($r1.status, 500);

# The reporter route reflects what the hook observed - proof it bound Error.
def r2 as http.Response init http.get("http://" + $addr + "/observed", $h);
testing.assertEqual($r2.body, "web:kaboom");

httpd.shutdown($srv);
task.wait($server);
`, webMod, httpMod)

	progPath := filepath.Join(dir, "app.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("web onError program failed with code %d", code)
	}
}
