// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package httpdlib

import (
	"crypto/tls"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func optsVal(fields ...interpreter.StructField) Value {
	return interpreter.NamespacedStructVal(LibraryName, "Options", fields)
}

func field(name string, v Value) interpreter.StructField {
	return interpreter.StructField{Name: name, Value: v}
}

// TestListenWithTLS (M29.48): httpd.Options.certPem / keyPem make listenWith
// serve TLS; one without the other, or a certificate in both places for
// listenTLSWith, is refused.
func TestListenWithTLS(t *testing.T) {
	ResetForTest()
	certPEM, keyPEM := selfSignedPEM(t)
	srv, err := listenWithFn(noCtx, []Value{interpreter.StringVal("127.0.0.1:0"), optsVal(
		field("certPem", interpreter.BytesVal(certPEM)),
		field("keyPem", interpreter.BytesVal(keyPEM)))})
	if err != nil {
		t.Fatalf("listenWith TLS: %v", err)
	}
	defer shutdownFn(noCtx, []Value{srv})
	addr, _ := addressFn(noCtx, []Value{srv})
	serveOnce(srv, func(req Value) {
		_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("tls")})
	})
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	resp, err := client.Get("https://" + addr.Str() + "/")
	if err != nil {
		t.Fatalf("GET over TLS: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "tls" || resp.TLS == nil {
		t.Errorf("body = %q, TLS = %v", b, resp.TLS != nil)
	}

	_, err = listenWithFn(noCtx, []Value{interpreter.StringVal("127.0.0.1:0"), optsVal(field("certPem", interpreter.BytesVal(certPEM)))})
	if err == nil || !strings.Contains(err.Error(), "set both httpd.Options.certPem and keyPem") {
		t.Errorf("cert without key: %v", err)
	}
	_, err = listenTLSWithFn(noCtx, []Value{interpreter.StringVal("127.0.0.1:0"), interpreter.BytesVal(certPEM), interpreter.BytesVal(keyPEM),
		optsVal(field("certPem", interpreter.BytesVal(certPEM)), field("keyPem", interpreter.BytesVal(keyPEM)))})
	if err == nil || !strings.Contains(err.Error(), "leave httpd.Options.certPem and keyPem empty") {
		t.Errorf("listenTLSWith with a certificate in the options: %v", err)
	}
}

// TestEtagOption (M29.48): httpd.Options.etag makes serveFile and serveDir
// send a content ETag and answer a matching If-None-Match with 304; without
// it they send none.
func TestEtagOption(t *testing.T) {
	ResetForTest()
	dir := t.TempDir()
	content := []byte("<h1>cached</h1>")
	for _, name := range []string{"index.html", "a.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wantTag := sha256Tag(content)
	plain, plainAddr := startServer(t)
	defer shutdownFn(noCtx, []Value{plain})
	tagged, err := listenWithFn(noCtx, []Value{interpreter.StringVal("127.0.0.1:0"), optsVal(field("etag", interpreter.BoolVal(true)))})
	if err != nil {
		t.Fatal(err)
	}
	defer shutdownFn(noCtx, []Value{tagged})
	taggedAddr, _ := addressFn(noCtx, []Value{tagged})
	for _, c := range []struct {
		name    string
		srv     Value
		addr    string
		serve   func(req Value)
		wantTag string
		path    string
	}{
		{"serveFile plain", plain, plainAddr, func(req Value) {
			_, _ = serveFileFn(noCtx, []Value{req, interpreter.StringVal(filepath.Join(dir, "index.html"))})
		}, "", "/"},
		{"serveFile etag", tagged, taggedAddr.Str(), func(req Value) {
			_, _ = serveFileFn(noCtx, []Value{req, interpreter.StringVal(filepath.Join(dir, "index.html"))})
		}, wantTag, "/"},
		{"serveDir etag", tagged, taggedAddr.Str(), func(req Value) {
			_, _ = serveDirFn(noCtx, []Value{req, interpreter.StringVal(dir)})
		}, wantTag, "/a.txt"},
	} {
		serveOnce(c.srv, c.serve)
		req, _ := http.NewRequest("GET", "http://"+c.addr+c.path, nil)
		req.Header.Set("If-None-Match", wantTag)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if got := resp.Header.Get("ETag"); got != c.wantTag {
			t.Errorf("%s: ETag %q, want %q", c.name, got, c.wantTag)
		}
		if wantStatus := map[bool]int{true: 304, false: 200}[c.wantTag != ""]; resp.StatusCode != wantStatus {
			t.Errorf("%s: status %d, want %d", c.name, resp.StatusCode, wantStatus)
		}
	}
}

// TestClosedServerAndRequest (M29.46): accept after shutdown, any call on a
// shut-down server and a call on an answered request are kind "closed"; a
// second shutdown does nothing.
func TestClosedServerAndRequest(t *testing.T) {
	ResetForTest()
	srv, addr := startServer(t)
	got := make(chan Value, 1)
	serveOnce(srv, func(req Value) {
		_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("ok")})
		got <- req
	})
	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	req := <-got
	kindOf := func(err error) string {
		kind, _, _, _, _ := interpreter.ClassifyError(err)
		return kind
	}
	_, err = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("again")})
	if k := kindOf(err); k != "closed" {
		t.Errorf("respond to an answered request: kind %q (%v)", k, err)
	}
	time.Sleep(50 * time.Millisecond) // the handler unregisters the request
	_, err = methodFn(noCtx, []Value{req})
	if k := kindOf(err); k != "closed" {
		t.Errorf("method of a finished request: kind %q (%v)", k, err)
	}
	accepted := make(chan error, 1)
	go func() {
		_, err := acceptFn(noCtx, []Value{srv})
		accepted <- err
	}()
	time.Sleep(50 * time.Millisecond)
	if _, err := shutdownFn(noCtx, []Value{srv}); err != nil {
		t.Fatal(err)
	}
	if k := kindOf(<-accepted); k != "closed" {
		t.Errorf("accept woken by shutdown: kind %q", k)
	}
	if _, err := shutdownFn(noCtx, []Value{srv}); err != nil {
		t.Errorf("second shutdown: %v", err)
	}
	for name, fn := range map[string]interpreter.Builtin{"accept": acceptFn, "address": addressFn, "maxInFlight": maxInFlightFn} {
		_, err := fn(noCtx, []Value{srv})
		if k := kindOf(err); k != "closed" {
			t.Errorf("%s after shutdown: kind %q (%v)", name, k, err)
		}
	}
}
