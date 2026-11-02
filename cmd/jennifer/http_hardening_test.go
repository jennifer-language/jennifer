// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// runHTTPProg writes a .j program that imports the http module as `http` and
// runs it under loadForTest; a failed assertion in the program fails the test.
func runHTTPProg(t *testing.T, name, body string) {
	t.Helper()
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	prog := fmt.Sprintf("use testing;\nuse strings;\nuse convert;\nimport %q as http;\n%s", httpMod, body)
	progPath := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("%s failed with code %d", name, code)
	}
}

// An https:// server that redirects to a plain http:// one: http.send refuses
// the hop by default (the next request would go out unencrypted) and follows it
// only with allowInsecureRedirect set.
func TestHttpRefusesHttpsToHttpRedirect(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "plain")
	}))
	defer plain.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/landed", http.StatusFound)
	}))
	defer secure.Close()

	runHTTPProg(t, "downgrade.j", fmt.Sprintf(`
def o as http.Options;
$o.maxRedirects = 3;
$o.tls.skipVerify = true;
def msg as string init "";
try {
    http.send("GET", %[1]q, {}, "", $o);
} catch (e) {
    $msg = $e.kind + ": " + $e.message;
}
testing.assertContains($msg, "http: ");
testing.assertContains($msg, "refused redirect from https:// to http://");

$o.allowInsecureRedirect = true;
def r as http.Response init http.send("GET", %[1]q, {}, "", $o);
testing.assertEqual($r.body, "plain");
`, secure.URL+"/start"))
}

// A server that drains the request body slowly (2 MiB per 100 ms, behind a
// small receive buffer) makes a 32 MiB upload take about 1.6 s while data
// keeps flowing. timeoutMs is an idle timeout on the write side too, so an
// 800 ms timeout must not cut the upload off. (The client send buffer, at most
// 4 MiB, drains in about 0.2 s after the last write, well inside the timeout.)
func TestHttpUploadTimeoutIsIdle(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			buf := make([]byte, 2<<20)
			total := 0
			for {
				n, err := io.ReadFull(r.Body, buf)
				total += n
				if err != nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			fmt.Fprintf(w, "%d", total)
		}),
	}
	go srv.Serve(smallBufListener{ln})
	defer srv.Close()

	runHTTPProg(t, "upload.j", fmt.Sprintf(`
def body as bytes init convert.bytesFromString(strings.repeat("x", 33554432), "utf-8");
def r as http.Response init http.requestRawBody("POST", %q, {"Content-Type": "application/octet-stream"}, $body, 800, 0);
testing.assertEqual($r.status, 200);
testing.assertEqual($r.body, "33554432");
`, "http://"+ln.Addr().String()+"/upload"))
}

// smallBufListener shrinks each accepted connection's receive buffer so the
// kernel cannot absorb the whole upload and the client's writes really block.
type smallBufListener struct{ net.Listener }

func (l smallBufListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		if tc, ok := c.(*net.TCPConn); ok {
			tc.SetReadBuffer(64 << 10)
		}
	}
	return c, err
}
