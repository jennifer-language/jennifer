// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

//go:build !tinygo

package httpdlib

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

// noCtx is a zero BuiltinCtx; the httpd builtins ignore it.
var noCtx interpreter.BuiltinCtx

// startServer listens on an ephemeral port and returns the Server handle value
// and its bound address.
func startServer(t *testing.T) (Value, string) {
	t.Helper()
	srv, err := listenFn(noCtx, []Value{interpreter.StringVal("127.0.0.1:0")})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addrV, err := addressFn(noCtx, []Value{srv})
	if err != nil {
		t.Fatalf("address: %v", err)
	}
	return srv, addrV.Str
}

// serveOnce runs one accept + handler(req) pass in a goroutine.
func serveOnce(srv Value, handler func(req Value)) {
	go func() {
		req, err := acceptFn(noCtx, []Value{srv})
		if err != nil {
			return
		}
		handler(req)
	}()
}

func TestRespondRoundTrip(t *testing.T) {
	ResetForTest()
	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	serveOnce(srv, func(req Value) {
		_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(201), interpreter.StringVal("hello\n")})
	})

	resp, err := http.Get("http://" + addr + "/greet")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 201 {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	if string(body) != "hello\n" {
		t.Errorf("body = %q, want %q", string(body), "hello\n")
	}
}

func TestRequestAccessors(t *testing.T) {
	ResetForTest()
	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	var gotMethod, gotPath, gotQuery, gotHeader, gotBody string
	var wg sync.WaitGroup
	wg.Add(1)
	serveOnce(srv, func(req Value) {
		defer wg.Done()
		m, _ := methodFn(noCtx, []Value{req})
		p, _ := pathFn(noCtx, []Value{req})
		q, _ := queryFn(noCtx, []Value{req, interpreter.StringVal("x")})
		h, _ := headerFn(noCtx, []Value{req, interpreter.StringVal("X-Test")})
		b, _ := bodyFn(noCtx, []Value{req})
		gotMethod, gotPath, gotQuery, gotHeader = m.Str, p.Str, q.Str, h.Str
		gotBody = string(b.Bytes)
		_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("ok")})
	})

	req, _ := http.NewRequest("POST", "http://"+addr+"/users/42?x=1", strings.NewReader("payload"))
	req.Header.Set("X-Test", "yes")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	wg.Wait()

	if gotMethod != "POST" {
		t.Errorf("method = %q", gotMethod)
	}
	if gotPath != "/users/42" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery != "1" {
		t.Errorf("query x = %q", gotQuery)
	}
	if gotHeader != "yes" {
		t.Errorf("header X-Test = %q", gotHeader)
	}
	if gotBody != "payload" {
		t.Errorf("body = %q", gotBody)
	}
}

func TestSetHeaderAndByteBody(t *testing.T) {
	ResetForTest()
	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	serveOnce(srv, func(req Value) {
		_, _ = setHeaderFn(noCtx, []Value{req, interpreter.StringVal("Content-Type"), interpreter.StringVal("application/json")})
		_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.BytesVal([]byte("{\"ok\":true}"))})
	})

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "{\"ok\":true}" {
		t.Errorf("body = %q", string(body))
	}
}

func TestServeFile(t *testing.T) {
	ResetForTest()
	dir := t.TempDir()
	fpath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(fpath, []byte("<h1>hi</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	serveOnce(srv, func(req Value) {
		_, _ = serveFileFn(noCtx, []Value{req, interpreter.StringVal(fpath)})
	})

	resp, err := http.Get("http://" + addr + "/whatever")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "<h1>hi</h1>" {
		t.Errorf("body = %q", string(body))
	}
}

func TestDoubleRespondErrors(t *testing.T) {
	ResetForTest()
	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	var secondErr error
	var wg sync.WaitGroup
	wg.Add(1)
	serveOnce(srv, func(req Value) {
		defer wg.Done()
		_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("first")})
		_, secondErr = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("second")})
	})
	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	wg.Wait()
	if secondErr == nil {
		t.Error("expected an error on the second respond")
	}
}

func TestAcceptAfterShutdown(t *testing.T) {
	ResetForTest()
	srv, _ := startServer(t)
	if _, err := shutdownFn(noCtx, []Value{srv}); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	// The server is unregistered, so accept reports it is gone.
	if _, err := acceptFn(noCtx, []Value{srv}); err == nil {
		t.Error("expected accept to error after shutdown")
	}
}

func TestShutdownUnblocksAccept(t *testing.T) {
	ResetForTest()
	srv, _ := startServer(t)

	done := make(chan error, 1)
	go func() {
		_, err := acceptFn(noCtx, []Value{srv})
		done <- err
	}()
	// Give the accept goroutine time to park on the channel.
	time.Sleep(20 * time.Millisecond)
	if _, err := shutdownFn(noCtx, []Value{srv}); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Error("expected the parked accept to error on shutdown")
		}
	case <-time.After(2 * time.Second):
		t.Error("accept did not unblock on shutdown")
	}
}

// TestUnixSocketListen serves over a Unix domain socket (httpd.listen("unix:..."))
// and drives it with an http.Client that dials the socket - the nginx
// reverse-proxy path.
func TestUnixSocketListen(t *testing.T) {
	ResetForTest()
	sock := filepath.Join(t.TempDir(), "httpd.sock")
	srv, err := listenFn(noCtx, []Value{interpreter.StringVal("unix:" + sock)})
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer shutdownFn(noCtx, []Value{srv})

	// The bound address is the socket path.
	addrV, _ := addressFn(noCtx, []Value{srv})
	if addrV.Str != sock {
		t.Errorf("address = %q, want %q", addrV.Str, sock)
	}

	serveOnce(srv, func(req Value) {
		_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("over-unix")})
	})

	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sock)
		},
	}}
	resp, err := client.Get("http://unix/whatever")
	if err != nil {
		t.Fatalf("GET over unix: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "over-unix" {
		t.Errorf("body = %q", string(body))
	}
}
