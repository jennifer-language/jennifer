// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A .j program driving the http client against a real in-process net/http
// server asserts the whole path: a GET with a Content-Length body, a JSON body,
// a POST whose body and Content-Type reach the handler (echoed back via
// headers), a chunked (Transfer-Encoding) response the client must de-chunk, and
// a 404 status. A mismatch throws and fails loadForTest. Runs against Go's
// actual HTTP implementation, no external server.
func TestHttpClient(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "hello world")
	})
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	})
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Method", r.Method)
		w.Header().Set("X-Ctype", r.Header.Get("Content-Type"))
		w.Write(body)
	})
	mux.HandleFunc("/chunked", func(w http.ResponseWriter, r *http.Request) {
		// no Content-Length + Flush -> Go sends Transfer-Encoding: chunked
		fl := w.(http.Flusher)
		fmt.Fprint(w, "chunk-one ")
		fl.Flush()
		fmt.Fprint(w, "chunk-two")
		fl.Flush()
	})
	mux.HandleFunc("/notfound", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
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
def hello as http.Response init http.get($base + "/hello", {});
testing.assertEqual($hello.status, 200);
testing.assertEqual($hello.body, "hello world");
testing.assertContains(http.header($hello, "Content-Type"), "text/plain");
def j as http.Response init http.get($base + "/json", {});
testing.assertEqual($j.body, '{"ok":true}');
def echo as http.Response init http.post($base + "/echo", "application/json", '{"x":1}', {});
testing.assertEqual($echo.body, '{"x":1}');
testing.assertEqual(http.header($echo, "X-Method"), "POST");
testing.assertEqual(http.header($echo, "X-Ctype"), "application/json");
def patched as http.Response init http.patch($base + "/echo", "application/json", '{"y":2}', {});
testing.assertEqual($patched.body, '{"y":2}');
testing.assertEqual(http.header($patched, "X-Method"), "PATCH");
def opts as http.Response init http.options($base + "/echo", {});
testing.assertEqual(http.header($opts, "X-Method"), "OPTIONS");
def chunked as http.Response init http.get($base + "/chunked", {});
testing.assertEqual($chunked.body, "chunk-one chunk-two");
def nf as http.Response init http.get($base + "/notfound", {});
testing.assertEqual($nf.status, 404);`, httpMod, srv.URL)
	progPath := filepath.Join(dir, "client.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("http client program failed with code %d", code)
	}
}

// TestHttpLargeChunkedMultiRead drives the client against a large chunked
// response (100 KB across 200 flushed chunks) so the de-chunk read loop must
// iterate over many partial reads - the multi-read path a small single-read
// chunked body never reaches. It pins the rolling-tail reassembly (a per-read
// `bytes + bytes` here is a runtime type error that breaks every multi-read
// chunked body).
func TestHttpLargeChunkedMultiRead(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		fl := w.(http.Flusher)
		for i := 0; i < 200; i++ {
			fmt.Fprint(w, strings.Repeat("x", 500))
			fl.Flush()
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	// Exercise BOTH body readers on a large chunked response: the one-shot
	// `get` (readToEOF) and a keep-alive session `exchange` (readOneRaw) - the
	// latter is where a per-read `bytes + bytes` broke every multi-read chunked
	// body.
	prog := fmt.Sprintf(`use testing;
use strings;
import %q as http;
def want as string init strings.repeat("x", 100000);
def r as http.Response init http.get(%q, {});
testing.assertEqual($r.status, 200);
testing.assertEqual($r.body, $want);
def opts as http.Options init http.defaultOptions();
def s as http.Session init http.connect(%q, $opts);
def x as http.Exchange init http.exchange($s, "GET", "/big", {}, "");
testing.assertEqual($x.response.status, 200);
testing.assertEqual(len($x.response.body), 100000);
testing.assertEqual($x.response.body, $want);
http.close($x.session);`, httpMod, srv.URL+"/big", srv.URL)
	progPath := filepath.Join(dir, "bigchunked.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("http large-chunked program failed with code %d", code)
	}
}
