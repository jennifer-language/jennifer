// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package httpdlib

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// A zero-value httpd.Server / httpd.Request (id 0) is refused, not resolved to
// the first live server or request.
func TestZeroValueHandlesRefused(t *testing.T) {
	ResetForTest()
	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})
	got := make(chan Value, 1)
	release := make(chan struct{})
	serveOnce(srv, func(req Value) {
		got <- req
		<-release
		_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("ok")})
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		if resp, err := http.Get("http://" + addr + "/"); err == nil {
			resp.Body.Close()
		}
	}()
	<-got
	zeroSrv, zeroReq := makeServer(0), makeRequest(0)
	if v, err := addressFn(noCtx, []Value{zeroSrv}); err == nil || !strings.Contains(err.Error(), "unset") {
		t.Errorf("address(zero Server) = %q, %v; want an unset-handle error", v.Str, err)
	}
	if v, err := methodFn(noCtx, []Value{zeroReq}); err == nil || !strings.Contains(err.Error(), "unset") {
		t.Errorf("method(zero Request) = %q, %v; want an unset-handle error", v.Str, err)
	}
	if v, _ := isOpenFn(noCtx, []Value{zeroReq}); v.Bool {
		t.Error("isOpen(zero Request) = true")
	}
	if _, err := shutdownFn(noCtx, []Value{zeroSrv}); err == nil {
		t.Error("shutdown(zero Server) succeeded")
	}
	close(release)
	<-done
	if _, err := addressFn(noCtx, []Value{srv}); err != nil {
		t.Errorf("the live server was affected: %v", err)
	}
}

// An accepted request answered within the shutdown grace period gets the
// program's answer, not an immediate 503.
func TestShutdownGraceForAcceptedRequest(t *testing.T) {
	ResetForTest()
	srv, addr := startServer(t)
	accepted := make(chan Value, 1)
	serveOnce(srv, func(req Value) { accepted <- req })
	type result struct {
		status int
		body   string
		err    error
	}
	res := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/")
		if err != nil {
			res <- result{err: err}
			return
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		res <- result{status: resp.StatusCode, body: string(b)}
	}()
	req := <-accepted
	shut := make(chan struct{})
	go func() {
		defer close(shut)
		_, _ = shutdownFn(noCtx, []Value{srv})
	}()
	time.Sleep(300 * time.Millisecond)
	if _, err := respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("late but in grace")}); err != nil {
		t.Errorf("respond during the grace period: %v", err)
	}
	r := <-res
	if r.err != nil || r.status != 200 || r.body != "late but in grace" {
		t.Errorf("client got %d %q (%v), want 200 with the program's body", r.status, r.body, r.err)
	}
	select {
	case <-shut:
	case <-time.After(3 * time.Second):
		t.Error("shutdown did not return after the request was answered")
	}
}

// After the grace period an accepted, unanswered request is released: shutdown
// returns and the handler stops waiting (it does not park until the respond
// timeout).
func TestShutdownGraceExpires(t *testing.T) {
	ResetForTest()
	old := shutdownGrace
	shutdownGrace = 200 * time.Millisecond
	defer func() { shutdownGrace = old }()
	srv, addr := startServer(t)
	accepted := make(chan Value, 1)
	serveOnce(srv, func(req Value) { accepted <- req })
	go func() {
		c := &http.Client{Timeout: 3 * time.Second}
		if resp, err := c.Get("http://" + addr + "/"); err == nil {
			resp.Body.Close()
		}
	}()
	req := <-accepted
	start := time.Now()
	_, _ = shutdownFn(noCtx, []Value{srv})
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("shutdown took %v with a 200 ms grace", d)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		v, _ := isOpenFn(noCtx, []Value{req})
		if !v.Bool {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("request still open 2 s after the grace period")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("x")}); err == nil {
		t.Error("respond after the grace period succeeded")
	}
}

// The dirListing listing leaves out dot entries, as serveDir refuses to serve
// them.
func TestDirListingHidesDotfiles(t *testing.T) {
	ResetForTest()
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(filepath.Join(sub, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"a.txt", ".env", ".htpasswd"} {
		if err := os.WriteFile(filepath.Join(sub, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts := interpreter.NamespacedStructVal(LibraryName, "Options", []interpreter.StructField{
		{Name: "dirListing", Value: interpreter.BoolVal(true)},
	})
	srv, err := listenWithFn(noCtx, []Value{interpreter.StringVal("127.0.0.1:0"), opts})
	if err != nil {
		t.Fatal(err)
	}
	defer shutdownFn(noCtx, []Value{srv})
	serveLoop(srv, func(req Value) { _, _ = serveDirFn(noCtx, []Value{req, interpreter.StringVal(dir)}) })
	addrV, _ := addressFn(noCtx, []Value{srv})
	get := func(p string) (int, string) {
		t.Helper()
		c := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		resp, err := c.Get("http://" + addrV.Str + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	status, body := get("/sub/")
	if status != 200 || !strings.Contains(body, "a.txt") {
		t.Fatalf("GET /sub/ = %d %q, want 200 listing a.txt", status, body)
	}
	for _, hidden := range []string{".env", ".htpasswd", ".git"} {
		if strings.Contains(body, hidden) {
			t.Errorf("listing shows %s: %q", hidden, body)
		}
	}
	if status, _ := get("/sub"); status != http.StatusMovedPermanently {
		t.Errorf("GET /sub = %d, want the 301 to /sub/", status)
	}
	if status, body := get("/sub/a.txt"); status != 200 || body != "x" {
		t.Errorf("GET /sub/a.txt = %d %q", status, body)
	}
}
