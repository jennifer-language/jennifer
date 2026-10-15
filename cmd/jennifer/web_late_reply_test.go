// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A web handler slower than the server's respond timeout: the client gets 503
// at the per-server deadline (httpd.Options.respondTimeoutMs), web.isOpen tells
// the handler its reply can no longer be sent, and web notes the dropped reply
// in one line instead of logging an unhandled handler error.
func TestWebLateReplyAfterRespondTimeout(t *testing.T) {
	stderr := runSlowWebHandler(t, `    io.eprintf("open=%%t\n", web.isOpen($ctx));
    web.text($ctx, 200, "done\n");`)
	if !strings.Contains(stderr, "open=false") {
		t.Errorf("web.isOpen should be false after the timeout; stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "the client already got 503, so its reply was dropped") {
		t.Errorf("missing the dropped-reply note; stderr:\n%s", stderr)
	}
	if strings.Contains(stderr, "unhandled handler error") {
		t.Errorf("a late reply was logged as a handler error; stderr:\n%s", stderr)
	}
}

// A real error a handler throws after the timeout is still a handler failure:
// web reports it as usual instead of passing it off as a dropped reply.
func TestWebErrorAfterRespondTimeoutIsReported(t *testing.T) {
	stderr := runSlowWebHandler(t, `    throw Error{kind: "db", message: "db down", file: "", line: 0, col: 0};`)
	if !strings.Contains(stderr, "unhandled handler error") || !strings.Contains(stderr, "db down") {
		t.Errorf("the handler's own error was not reported; stderr:\n%s", stderr)
	}
	if strings.Contains(stderr, "so its reply was dropped") {
		t.Errorf("a real error was reported as a dropped reply; stderr:\n%s", stderr)
	}
}

// runSlowWebHandler serves one route whose handler sleeps past a 150 ms respond
// timeout and then runs tail, checks the client got 503, and returns stderr.
func runSlowWebHandler(t *testing.T, tail string) string {
	t.Helper()
	webMod, _ := filepath.Abs(filepath.Join("..", "..", "modules", "web.j"))
	httpMod, _ := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	path := filepath.Join(t.TempDir(), "late.j")
	src := fmt.Sprintf(`use io;
use time;
use httpd;
use task;
use testing;
import %q as web;
import %q as http;
func slow(ctx as web.Context) {
    time.sleep(time.fromMilliseconds(500));
`+tail+`
}
def app as web.App init web.new();
$app = web.get($app, "/", slow);
def srv as httpd.Server init httpd.listenWith("127.0.0.1:0",
    httpd.Options{maxBodyBytes: 0, maxInFlight: 0, respondTimeoutMs: 150});
def t as task of null init spawn {
    web.serveOn($app, $srv);
    return;
};
def r as http.Response init http.get("http://" + httpd.address($srv) + "/", {});
testing.assertEqual($r.status, 503);
time.sleep(time.fromMilliseconds(800));
httpd.shutdown($srv);
task.wait($t);
`, webMod, httpMod)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	oldErr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	code := runFile(path, nil, "")
	w.Close()
	os.Stderr = oldErr
	out, _ := io.ReadAll(r)
	stderr := string(out)
	if code != 0 {
		t.Fatalf("exit %d; stderr:\n%s", code, stderr)
	}
	return stderr
}
