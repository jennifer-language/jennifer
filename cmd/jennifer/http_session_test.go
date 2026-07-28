// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// A .j program drives a persistent http.Session against an in-process server:
// two exchanges reuse one TCP connection (keep-alive) and a cookie set on the
// first response is replayed on the second. The server counts new connections
// via ConnState, so the test proves the socket was actually reused (conns == 1),
// not just that both requests succeeded.
func TestHttpSession(t *testing.T) {
	var mu sync.Mutex
	conns := 0
	reqCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/count", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqCount++
		n := reqCount
		mu.Unlock()
		if n == 1 {
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "abc"})
		}
		cv := ""
		if c, err := r.Cookie("sid"); err == nil {
			cv = c.Value
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "n=%d cookie=%s", n, cv)
	})
	srv := httptest.NewUnstartedServer(mux)
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			mu.Lock()
			conns++
			mu.Unlock()
		}
	}
	srv.Start()
	defer srv.Close()

	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as http;
def opts as http.Options init http.defaultOptions();
def s as http.Session init http.connect(%q, $opts);
def x1 as http.Exchange init http.exchange($s, "GET", "/count", {}, "");
$s = $x1.session;
testing.assertEqual($x1.response.status, 200);
testing.assertContains($x1.response.body, "n=1");
def x2 as http.Exchange init http.exchange($s, "GET", "/count", {}, "");
$s = $x2.session;
testing.assertContains($x2.response.body, "n=2");
testing.assertContains($x2.response.body, "cookie=abc");
http.close($s);`, httpMod, srv.URL)
	progPath := filepath.Join(dir, "session.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("http session program failed with code %d", code)
	}
	mu.Lock()
	got := conns
	mu.Unlock()
	if got != 1 {
		t.Fatalf("keep-alive reuse: opened %d connections, want 1", got)
	}
}

// A .j program drives http.send's request policy: it follows a 302 redirect to
// a final resource, converts a POST that gets a 303 into a bodyless GET, and
// retries a 503 that then succeeds. All against Go's real HTTP server.
func TestHttpSendPolicy(t *testing.T) {
	var mu sync.Mutex
	flaky := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/redir", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dest", http.StatusFound) // 302
	})
	mux.HandleFunc("/submit", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dest", http.StatusSeeOther) // 303 -> GET
	})
	mux.HandleFunc("/dest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "arrived %s", r.Method)
	})
	mux.HandleFunc("/flaky", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		flaky++
		n := flaky
		mu.Unlock()
		if n == 1 {
			http.Error(w, "busy", http.StatusServiceUnavailable) // 503
			return
		}
		fmt.Fprint(w, "ok")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as http;
def base as string init %q;

def follow as http.Options;
$follow.maxRedirects = 3;
def r1 as http.Response init http.send("GET", $base + "/redir", {}, "", $follow);
testing.assertEqual($r1.status, 200);
testing.assertEqual($r1.body, "arrived GET");

# a POST that gets a 303 follows as a bodyless GET
def r2 as http.Response init http.send("POST", $base + "/submit", {"Content-Type": "text/plain"}, "hello", $follow);
testing.assertEqual($r2.body, "arrived GET");

# a 503 that then succeeds is retried
def retryOpts as http.Options;
$retryOpts.maxRetries = 2;
$retryOpts.backoffMs = 1;
def r3 as http.Response init http.send("GET", $base + "/flaky", {}, "", $retryOpts);
testing.assertEqual($r3.status, 200);
testing.assertEqual($r3.body, "ok");`, httpMod, srv.URL)
	progPath := filepath.Join(dir, "policy.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("http send-policy program failed with code %d", code)
	}
}
