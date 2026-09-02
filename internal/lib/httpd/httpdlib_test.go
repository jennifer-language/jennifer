// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package httpdlib

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// sha256Tag is the quoted strong ETag contentEtag produces for the given bytes.
func sha256Tag(b []byte) string {
	sum := sha256.Sum256(b)
	return "\"" + hex.EncodeToString(sum[:]) + "\""
}

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

// TestRespondRejectsBadStatus verifies an out-of-range status is a boundary
// error, not a net/http WriteHeader panic.
func TestRespondRejectsBadStatus(t *testing.T) {
	rs := &reqState{done: make(chan struct{}), status: 200}
	id := registerReq(rs)
	defer unregisterReq(id)
	req := makeRequest(id)
	if _, err := respondFn(noCtx, []Value{req, interpreter.IntVal(0), interpreter.StringVal("x")}); err == nil {
		t.Error("expected error for status 0")
	}
	if _, err := respondFn(noCtx, []Value{req, interpreter.IntVal(1000), interpreter.StringVal("x")}); err == nil {
		t.Error("expected error for status 1000")
	}
}

// TestRequestScratch round-trips the per-request scratch store: an unset key
// reads back "", a set key reads back its value, and a second set overwrites.
// This is the primitive web.csrfToken uses to memoize its token per request.
func TestRequestScratch(t *testing.T) {
	rs := &reqState{done: make(chan struct{}), status: 200}
	id := registerReq(rs)
	defer unregisterReq(id)
	req := makeRequest(id)

	// Absent key -> "".
	got, err := requestValueFn(noCtx, []Value{req, interpreter.StringVal("k")})
	if err != nil {
		t.Fatalf("requestValue (absent): %v", err)
	}
	if got.Kind != interpreter.KindString || got.Str != "" {
		t.Fatalf("absent key: want empty string, got %v %q", got.Kind, got.Str)
	}

	// Set then read back.
	if _, err := setRequestValueFn(noCtx, []Value{req, interpreter.StringVal("k"), interpreter.StringVal("v1")}); err != nil {
		t.Fatalf("setRequestValue: %v", err)
	}
	got, err = requestValueFn(noCtx, []Value{req, interpreter.StringVal("k")})
	if err != nil {
		t.Fatalf("requestValue (set): %v", err)
	}
	if got.Str != "v1" {
		t.Fatalf("after set: want %q, got %q", "v1", got.Str)
	}

	// Overwrite.
	if _, err := setRequestValueFn(noCtx, []Value{req, interpreter.StringVal("k"), interpreter.StringVal("v2")}); err != nil {
		t.Fatalf("setRequestValue (overwrite): %v", err)
	}
	got, _ = requestValueFn(noCtx, []Value{req, interpreter.StringVal("k")})
	if got.Str != "v2" {
		t.Fatalf("after overwrite: want %q, got %q", "v2", got.Str)
	}
}

// TestServeDirRejectsBackslash verifies OF-004: a request path carrying a
// backslash (a Windows separator that path.Clean's slash-only cleaning leaves
// intact but filepath.Join would resolve above root) is answered 400, not served.
func TestServeDirRejectsBackslash(t *testing.T) {
	rs := &reqState{
		done:   make(chan struct{}),
		status: 200,
		r:      &http.Request{URL: &url.URL{Path: `/..\..\secret.txt`}},
	}
	id := registerReq(rs)
	defer unregisterReq(id)
	req := makeRequest(id)
	if _, err := serveDirFn(noCtx, []Value{req, interpreter.StringVal("/tmp/root")}); err != nil {
		t.Fatalf("serveDir returned an error instead of a 400 answer: %v", err)
	}
	<-rs.done
	if rs.status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (400)", rs.status, http.StatusBadRequest)
	}
	if rs.useServeFile {
		t.Error("a backslash path must not reach ServeFile")
	}
}

// TestSetCookieMultiple verifies that several Set-Cookie response headers are
// each preserved (emitted with Header().Add, not collapsed by Set) - the
// property the web module's cookie support relies on.
func TestSetCookieMultiple(t *testing.T) {
	ResetForTest()
	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	serveOnce(srv, func(req Value) {
		_, _ = setHeaderFn(noCtx, []Value{req, interpreter.StringVal("Set-Cookie"), interpreter.StringVal("sid=abc; HttpOnly")})
		_, _ = setHeaderFn(noCtx, []Value{req, interpreter.StringVal("Set-Cookie"), interpreter.StringVal("theme=dark")})
		_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("ok")})
	})

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if cookies := resp.Header["Set-Cookie"]; len(cookies) != 2 {
		t.Fatalf("Set-Cookie count = %d, want 2 (%v)", len(cookies), cookies)
	}
}

// respond must copy a bytes body at the boundary: the socket write happens
// later, on the handler goroutine, so handing it the caller's backing lets a
// post-respond `$buf[i] = ...` mutation race the write (and corrupt the
// response). The Jennifer-visible symptom is a violation of value semantics.
func TestRespondCopiesBytesBody(t *testing.T) {
	ResetForTest()
	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	serveOnce(srv, func(req Value) {
		buf := interpreter.BytesVal([]byte("hello"))
		_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), buf})
		// Simulates `$buf[0] = 88;` right after httpd.respond returns.
		buf.Bytes[0] = 'X'
	})

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello" {
		t.Errorf("body = %q, want %q (post-respond mutation must not reach the wire)", string(body), "hello")
	}
}

// A request body over the buffering cap must be REJECTED (413), not silently
// truncated: a truncated-but-complete-looking body defeats body-signature
// verification and smuggles content past inspection. A body exactly at the
// cap still goes through whole.
func TestOversizeBodyRejectedNotTruncated(t *testing.T) {
	ResetForTest()
	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	// One byte over the cap: the handler must answer 413 on its own (the
	// pull loop never sees the request, so no serveOnce here).
	over := bytes.Repeat([]byte("x"), int(defaultMaxBodyBytes)+1)
	resp, err := http.Post("http://"+addr+"/", "application/octet-stream", bytes.NewReader(over))
	if err != nil {
		t.Fatalf("POST over-cap: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("over-cap status = %d, want %d", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}

	// Exactly at the cap: handed to the program complete.
	var gotLen int64
	serveOnce(srv, func(req Value) {
		b, err := bodyFn(noCtx, []Value{req})
		if err == nil {
			gotLen = int64(len(b.Bytes))
		}
		_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("ok")})
	})
	atCap := bytes.Repeat([]byte("y"), int(defaultMaxBodyBytes))
	resp2, err := http.Post("http://"+addr+"/", "application/octet-stream", bytes.NewReader(atCap))
	if err != nil {
		t.Fatalf("POST at-cap: %v", err)
	}
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Errorf("at-cap status = %d, want 200", resp2.StatusCode)
	}
	if gotLen != defaultMaxBodyBytes {
		t.Errorf("at-cap body length seen by the program = %d, want %d", gotLen, defaultMaxBodyBytes)
	}
}

// optionsVal builds a httpd.Options struct value for the listenWith tests.
func optionsVal(maxBody, inFlight int64) Value {
	return interpreter.NamespacedStructVal(LibraryName, "Options", []interpreter.StructField{
		{Name: "maxBodyBytes", Value: interpreter.IntVal(maxBody)},
		{Name: "maxInFlight", Value: interpreter.IntVal(inFlight)},
	})
}

// resolveServerLimits is the memory guard: 0 selects a default, a negative or
// over-budget value is rejected, and the product is computed overflow-safely.
func TestResolveServerLimits(t *testing.T) {
	cases := []struct {
		name             string
		maxBody, inFlt   int64
		wantBody         int64
		wantInFlt        int
		wantErrSubstring string // "" = expect success
	}{
		{"both-default", 0, 0, defaultMaxBodyBytes, defaultMaxInFlight, ""},
		// Raising the body at the default concurrency is bounded by the budget:
		// 16 MiB * 256 = exactly 4 GiB is allowed (the ceiling is inclusive).
		{"raise-body-at-ceiling", 16 << 20, 0, 16 << 20, defaultMaxInFlight, ""},
		// A big body needs a matching concurrency cut: 100 MiB * 40 = 4000 MiB.
		{"repartition-for-big-body", 100 << 20, 40, 100 << 20, 40, ""},
		{"repartition", 26 << 20, 100, 26 << 20, 100, ""},
		{"body-default-when-zero", 0, 50, defaultMaxBodyBytes, 50, ""},
		{"negative-body", -1, 0, 0, 0, "maxBodyBytes must be >= 0"},
		{"negative-inflight", 0, -5, 0, 0, "maxInFlight must be >= 0"},
		{"inflight-over-ceiling", 0, maxInFlightCeiling + 1, 0, 0, "exceeds the ceiling"},
		{"single-body-over-budget", defaultBufferBudget + 1, 1, 0, 0, "on its own"},
		{"product-over-budget", 3 << 30, 256, 0, 0, "worst-case buffered memory exceeds"},
		// A huge body with a bounded inFlight must be rejected without an int64
		// multiply overflow (the single-body check fires first).
		{"overflow-guarded", 1 << 62, maxInFlightCeiling, 0, 0, "on its own"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotBody, gotInFlt, err := resolveServerLimits(tc.maxBody, tc.inFlt)
			if tc.wantErrSubstring != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (body=%d inflight=%d)", tc.wantErrSubstring, gotBody, gotInFlt)
				}
				if !strings.Contains(err.Error(), tc.wantErrSubstring) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErrSubstring)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotBody != tc.wantBody || gotInFlt != tc.wantInFlt {
				t.Fatalf("resolveServerLimits = (%d, %d), want (%d, %d)", gotBody, gotInFlt, tc.wantBody, tc.wantInFlt)
			}
		})
	}
}

// The guard's product check must not overflow int64 when the operator raises
// the budget very high: a (maxBody, inFlight) whose true product exceeds the
// budget but whose int64 multiply would wrap must still be rejected.
func TestResolveServerLimitsNoOverflowAtHighBudget(t *testing.T) {
	ResetForTest()
	defer ResetForTest()
	// Raise the budget to 1<<62. maxBody 1<<50 is under it (passes the
	// single-body check), but 1<<50 * 65536 = 1<<66 overflows int64 - and is
	// genuinely far over the 1<<62 budget, so it must be rejected.
	bufferBudget.Store(1 << 62)
	if _, _, err := resolveServerLimits(1<<50, maxInFlightCeiling); err == nil {
		t.Fatal("a product that overflows int64 must still be rejected, not wrap past the guard")
	}
	// A pair whose product is exactly the raised budget is allowed (inclusive).
	if _, _, err := resolveServerLimits(1<<46, 1<<16); err != nil {
		t.Fatalf("product == budget should be allowed: %v", err)
	}
}

// A server opened with httpd.listenWith enforces ITS OWN body cap, not the
// package default: here a cap lowered to 1 KiB rejects a 2 KiB body (which the
// default 10 MiB server would accept) and admits a sub-cap body whole. Proves
// the per-server limit threads all the way to the handler.
func TestListenWithEnforcesPerServerBodyCap(t *testing.T) {
	ResetForTest()
	srvV, err := listenWithFn(noCtx, []Value{interpreter.StringVal("127.0.0.1:0"), optionsVal(1024, 0)})
	if err != nil {
		t.Fatalf("listenWith: %v", err)
	}
	defer shutdownFn(noCtx, []Value{srvV})
	addrV, _ := addressFn(noCtx, []Value{srvV})
	addr := addrV.Str

	// Over the 1 KiB per-server cap: 413 on its own, and the message names the
	// actual limit so it is self-explaining.
	over := bytes.Repeat([]byte("x"), 2048)
	resp, err := http.Post("http://"+addr+"/", "application/octet-stream", bytes.NewReader(over))
	if err != nil {
		t.Fatalf("POST over-cap: %v", err)
	}
	msg, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("over-cap status = %d, want %d", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}
	if !strings.Contains(string(msg), "1024") {
		t.Errorf("413 body %q should name the 1024-byte limit", strings.TrimSpace(string(msg)))
	}

	// Under the cap: handed to the program complete.
	var gotLen int64
	serveOnce(srvV, func(req Value) {
		b, err := bodyFn(noCtx, []Value{req})
		if err == nil {
			gotLen = int64(len(b.Bytes))
		}
		_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("ok")})
	})
	under := bytes.Repeat([]byte("y"), 512)
	resp2, err := http.Post("http://"+addr+"/", "application/octet-stream", bytes.NewReader(under))
	if err != nil {
		t.Fatalf("POST under-cap: %v", err)
	}
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Errorf("under-cap status = %d, want 200", resp2.StatusCode)
	}
	if gotLen != 512 {
		t.Errorf("under-cap body length seen by the program = %d, want 512", gotLen)
	}
}

// The memory guard rejects an over-budget httpd.Options at listen time, and no
// server (nor its listening socket) leaks into the registry when it does.
func TestListenWithGuardRejectsOverBudget(t *testing.T) {
	ResetForTest()
	before := serverCount()
	// 3 GiB body at the default 256 concurrency = ~768 GiB worst case, far over
	// the 4 GiB ceiling.
	_, err := listenWithFn(noCtx, []Value{interpreter.StringVal("127.0.0.1:0"), optionsVal(3<<30, 0)})
	if err == nil {
		t.Fatal("expected listenWith to reject an over-budget Options, got nil")
	}
	if !strings.Contains(err.Error(), "worst-case buffered memory exceeds") {
		t.Errorf("error %q should explain the memory-budget rejection", err.Error())
	}
	if got := serverCount(); got != before {
		t.Errorf("a rejected listenWith leaked a server: count %d -> %d", before, got)
	}
}

// httpd.setMaxBufferBudget raises (or lowers) the guard ceiling: a config the
// default 4 GiB budget rejects becomes allowed after the operator raises the
// budget to match the host's RAM, and a below-floor budget is refused.
func TestSetMaxBufferBudget(t *testing.T) {
	ResetForTest()
	defer ResetForTest() // restore the default for later tests

	// 5 GiB body is over the default 4 GiB budget on its own.
	if _, _, err := resolveServerLimits(5<<30, 1); err == nil {
		t.Fatal("5 GiB body should be rejected under the default 4 GiB budget")
	}

	// Raise the budget to 8 GiB (an 8 GiB VPS); now 5 GiB x 1 fits.
	if _, err := setMaxBufferBudgetFn(noCtx, []Value{interpreter.IntVal(8 << 30)}); err != nil {
		t.Fatalf("setMaxBufferBudget(8 GiB): %v", err)
	}
	gotBody, gotInFlt, err := resolveServerLimits(5<<30, 1)
	if err != nil {
		t.Fatalf("5 GiB body should be allowed after raising the budget: %v", err)
	}
	if gotBody != 5<<30 || gotInFlt != 1 {
		t.Fatalf("resolveServerLimits = (%d, %d), want (%d, 1)", gotBody, gotInFlt, int64(5<<30))
	}

	// A below-floor budget is refused (would reject every listenWith).
	if _, err := setMaxBufferBudgetFn(noCtx, []Value{interpreter.IntVal(1024)}); err == nil {
		t.Fatal("setMaxBufferBudget below the floor should error")
	}

	// An absurd fat-finger above the sanity ceiling is refused, not stored.
	if _, err := setMaxBufferBudgetFn(noCtx, []Value{interpreter.IntVal(9000000000000000000)}); err == nil {
		t.Fatal("setMaxBufferBudget above the sanity ceiling should error")
	}
	// budgetFromLimit caps at the ceiling even for an absurd detected limit.
	if got := budgetFromLimit(1<<60, 1.0); got != maxBufferBudget {
		t.Errorf("budgetFromLimit(1<<60, 1.0) = %d, want ceiling %d", got, int64(maxBufferBudget))
	}
}

// The cgroup / meminfo parsers that back setMaxBufferBudgetFromRAM.
func TestMemLimitParsers(t *testing.T) {
	// /proc/meminfo -> bytes.
	if got, ok := parseMemTotalBytes("MemFree: 100 kB\nMemTotal:   16384000 kB\nBuffers: 1 kB\n"); !ok || got != 16384000*1024 {
		t.Errorf("parseMemTotalBytes = (%d, %v), want (%d, true)", got, ok, int64(16384000)*1024)
	}
	if _, ok := parseMemTotalBytes("Buffers: 1 kB\n"); ok {
		t.Error("parseMemTotalBytes should fail when MemTotal is absent")
	}

	// cgroup v2 memory.max: "max" = unlimited, a number = a cap.
	if _, ok := parseCgroupV2MemoryMax("max\n"); ok {
		t.Error(`parseCgroupV2MemoryMax("max") should report no limit`)
	}
	if got, ok := parseCgroupV2MemoryMax(" 536870912 \n"); !ok || got != 536870912 {
		t.Errorf("parseCgroupV2MemoryMax = (%d, %v), want (536870912, true)", got, ok)
	}

	// cgroup v1 limit_in_bytes: a near-int64-max sentinel = unlimited.
	if _, ok := parseCgroupV1Limit("9223372036854771712\n"); ok {
		t.Error("parseCgroupV1Limit should treat the sentinel as unlimited")
	}
	if got, ok := parseCgroupV1Limit("268435456"); !ok || got != 268435456 {
		t.Errorf("parseCgroupV1Limit = (%d, %v), want (268435456, true)", got, ok)
	}

	// /proc/self/cgroup path extraction.
	if p, ok := cgroupV2Path("0::/system.slice/app.service\n"); !ok || p != "/system.slice/app.service" {
		t.Errorf("cgroupV2Path = (%q, %v)", p, ok)
	}
	if p, ok := cgroupV1MemoryPath("9:cpu,cpuacct:/x\n8:memory:/system.slice/app\n"); !ok || p != "/system.slice/app" {
		t.Errorf("cgroupV1MemoryPath = (%q, %v)", p, ok)
	}
	if _, ok := cgroupV1MemoryPath("9:cpu,cpuacct:/x\n"); ok {
		t.Error("cgroupV1MemoryPath should fail when no memory controller line is present")
	}
}

// budgetFromLimit applies the fraction and floors at one default body.
func TestBudgetFromLimit(t *testing.T) {
	if got := budgetFromLimit(8<<30, 0.5); got != 4<<30 {
		t.Errorf("budgetFromLimit(8 GiB, 0.5) = %d, want %d", got, int64(4)<<30)
	}
	if got := budgetFromLimit(8<<30, 1.0); got != 8<<30 {
		t.Errorf("budgetFromLimit(8 GiB, 1.0) = %d, want %d", got, int64(8)<<30)
	}
	// A tiny limit floors at one default body rather than going below it.
	if got := budgetFromLimit(1<<20, 0.5); got != minBufferBudget {
		t.Errorf("budgetFromLimit(1 MiB, 0.5) = %d, want floor %d", got, int64(minBufferBudget))
	}
}

// setMaxBufferBudgetFromRAM is opt-in: it validates the fraction, and on a Linux
// host it sets a sane positive budget (>= floor) and returns the bytes it chose.
func TestSetMaxBufferBudgetFromRAM(t *testing.T) {
	ResetForTest()
	defer ResetForTest()

	// Fraction out of range is rejected without touching the budget.
	for _, bad := range []Value{interpreter.FloatVal(0), interpreter.FloatVal(1.5), interpreter.FloatVal(-0.1)} {
		if _, err := setMaxBufferBudgetFromRAMFn(noCtx, []Value{bad}); err == nil {
			t.Errorf("fraction %v should be rejected", bad)
		}
	}

	// On this Linux host, detection should succeed and set a floored, positive
	// budget no larger than the detected machine limit.
	limit, derr := detectMemoryLimitBytes()
	res, err := setMaxBufferBudgetFromRAMFn(noCtx, []Value{interpreter.FloatVal(0.5)})
	if derr != nil {
		// Non-Linux / unusual host: the builtin must error cleanly, not panic.
		if err == nil {
			t.Skip("no machine memory limit detectable here; builtin returned without error")
		}
		t.Skipf("memory limit not detectable on this host: %v", derr)
	}
	if err != nil {
		t.Fatalf("setMaxBufferBudgetFromRAM(0.5): %v", err)
	}
	if res.Kind != interpreter.KindInt || res.Int < minBufferBudget || res.Int > limit {
		t.Fatalf("budget = %v (%s), want an int in [%d, %d]", res, res.Kind, int64(minBufferBudget), limit)
	}
	if bufferBudget.Load() != res.Int {
		t.Fatalf("live budget %d != returned %d", bufferBudget.Load(), res.Int)
	}
}

// A request that is accepted but never answered (the program threw between
// accept and respond) must not park the handler goroutine and client forever:
// the engine answers 500 after respondTimeout and unparks.
func TestUnansweredRequestTimesOut(t *testing.T) {
	ResetForTest()
	old := respondTimeout
	respondTimeout = 150 * time.Millisecond
	defer func() { respondTimeout = old }()

	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	// Accept the request but deliberately never respond.
	go func() {
		_, _ = acceptFn(noCtx, []Value{srv})
	}()

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("unanswered request status = %d, want 500", resp.StatusCode)
	}
}

// A client that trickles its request body must not hold an admission slot
// forever: the body read runs under bodyReadTimeout and answers 400 when it
// expires (body-read Slowloris protection).
func TestSlowBodyTimesOut(t *testing.T) {
	ResetForTest()
	old := bodyReadTimeout
	bodyReadTimeout = 200 * time.Millisecond
	defer func() { bodyReadTimeout = old }()

	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	// Raw client: send headers declaring a body, then stall mid-body.
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	fmt.Fprintf(c, "POST / HTTP/1.1\r\nHost: %s\r\nContent-Length: 1000\r\n\r\npartial", addr)

	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("stalled body status = %d, want 400", resp.StatusCode)
	}
}

// A request that is never pulled off the accept queue (the program is stuck
// between listen and accept) must not park its handler goroutine forever; it
// answers 503 after respondTimeout.
func TestNeverAcceptedRequestTimesOut(t *testing.T) {
	ResetForTest()
	old := respondTimeout
	respondTimeout = 150 * time.Millisecond
	defer func() { respondTimeout = old }()

	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	// No acceptFn call at all: the reqs channel is never drained.
	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("never-accepted request status = %d, want 503", resp.StatusCode)
	}
}

// A client that stops reading the response must not pin its handler (and its
// admission slot) forever: the per-response write deadline aborts the stalled
// write. Without it, a large body to a non-reading client blocks in w.Write
// until the connection dies on its own.
func TestSlowReadResponseTimesOut(t *testing.T) {
	ResetForTest()
	old := writeTimeout
	writeTimeout = 300 * time.Millisecond
	defer func() { writeTimeout = old }()

	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	// Body larger than the combined kernel send + receive buffers (wmem_max +
	// tcp_rmem max, ~36 MiB on this host), so the write genuinely blocks on a
	// non-reading client rather than being absorbed - only then does the
	// deadline get exercised.
	big := strings.Repeat("x", 128<<20)
	serveOnce(srv, func(req Value) {
		_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal(big)})
	})

	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	fmt.Fprintf(c, "GET / HTTP/1.1\r\nHost: %s\r\n\r\n", addr)

	// Do NOT read while the server writes: the kernel buffers fill (~36 MiB)
	// and the write blocks. Past writeTimeout the deadline must fire, aborting
	// the write and closing the connection. Wait well past the deadline, then
	// drain: with the fix the socket holds the buffered prefix followed by EOF
	// (server closed), so the drain finishes fast. Without it the server is
	// still blocked in w.Write, the connection stays open, and the drain blocks
	// on the read deadline instead.
	time.Sleep(1500 * time.Millisecond) // >> writeTimeout (300ms)
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	start := time.Now()
	buf := make([]byte, 64<<10)
	var readErr error
	for {
		_, readErr = c.Read(buf)
		if readErr != nil {
			break
		}
	}
	elapsed := time.Since(start)
	if os.IsTimeout(readErr) {
		t.Errorf("server did not abort the stalled write: drain hit the read deadline after %v (connection stayed open)", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Errorf("drain ran %v (want fast EOF); server likely did not close", elapsed)
	}
}

// An idle keep-alive connection must be closed by the server after
// idleTimeout: ReadHeaderTimeout does not bound the gap between requests, so
// without IdleTimeout a client could hold a goroutine + fd open forever by
// sending one request then idling (connection-exhaustion Slowloris).
func TestIdleKeepAliveTimesOut(t *testing.T) {
	ResetForTest()
	oldIdle := idleTimeout
	idleTimeout = 300 * time.Millisecond
	defer func() { idleTimeout = oldIdle }()

	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	serveOnce(srv, func(req Value) {
		_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("hi")})
	})

	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	// One complete request/response, then go idle (send nothing more).
	fmt.Fprintf(c, "GET / HTTP/1.1\r\nHost: %s\r\n\r\n", addr)
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// The idle connection must be closed by the server well inside this bound.
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	start := time.Now()
	_, readErr := br.Read(make([]byte, 1))
	if os.IsTimeout(readErr) {
		t.Errorf("idle connection was not closed: read hit its deadline after %v", time.Since(start))
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("idle close took %v (want ~idleTimeout)", elapsed)
	}
}

// TestEtagMatches pins the RFC 7232 If-None-Match parsing the engine does for
// httpd.etag: exact quoted, bare-tag fallback, "*", comma list, and the W/ weak
// prefix a cache/proxy may add; a non-match and an empty header return false.
func TestEtagMatches(t *testing.T) {
	cases := []struct {
		inm    string
		want   bool
		reason string
	}{
		{`"v1"`, true, "exact quoted"},
		{`v1`, true, "bare-tag fallback"},
		{`*`, true, "star matches any"},
		{`"v0", "v1"`, true, "comma list contains it"},
		{`W/"v1"`, true, "weak prefix stripped"},
		{`"v2"`, false, "different tag"},
		{``, false, "empty header"},
		{`"v10"`, false, "no substring false-positive"},
	}
	for _, c := range cases {
		if got := etagMatches(c.inm, `"v1"`, "v1"); got != c.want {
			t.Errorf("etagMatches(%q) = %v, want %v (%s)", c.inm, got, c.want, c.reason)
		}
	}
}

// TestEtagConditionalGET drives httpd.etag end to end: the first GET (no
// validator) gets the full body plus an ETag, and a second GET carrying that
// ETag as If-None-Match gets a bodyless 304 - and the handler sees etag return
// true so it stops.
func TestEtagConditionalGET(t *testing.T) {
	ResetForTest()
	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	// First request: no If-None-Match, so etag returns false and the handler
	// sends the full body.
	var firstRet Value
	var wg sync.WaitGroup
	wg.Add(1)
	serveOnce(srv, func(req Value) {
		defer wg.Done()
		firstRet, _ = etagFn(noCtx, []Value{req, interpreter.StringVal("v1")})
		if !firstRet.Bool {
			_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("full body")})
		}
	})
	resp, err := http.Get("http://" + addr + "/asset")
	if err != nil {
		t.Fatalf("GET 1: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	wg.Wait()
	if firstRet.Bool {
		t.Errorf("first etag() = true, want false (no If-None-Match)")
	}
	if resp.StatusCode != 200 || string(body) != "full body" {
		t.Errorf("first response = %d %q, want 200 %q", resp.StatusCode, string(body), "full body")
	}
	if et := resp.Header.Get("ETag"); et != `"v1"` {
		t.Errorf("first ETag = %q, want %q", et, `"v1"`)
	}

	// Second request: If-None-Match carries the tag, so etag claims a 304 and
	// returns true; the handler stops without sending a body.
	var secondRet Value
	wg.Add(1)
	serveOnce(srv, func(req Value) {
		defer wg.Done()
		secondRet, _ = etagFn(noCtx, []Value{req, interpreter.StringVal("v1")})
		if !secondRet.Bool {
			_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("full body")})
		}
	})
	req, _ := http.NewRequest("GET", "http://"+addr+"/asset", nil)
	req.Header.Set("If-None-Match", `"v1"`)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET 2: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	wg.Wait()
	if !secondRet.Bool {
		t.Errorf("second etag() = false, want true (If-None-Match matched)")
	}
	if resp2.StatusCode != http.StatusNotModified {
		t.Errorf("second status = %d, want 304", resp2.StatusCode)
	}
	if len(body2) != 0 {
		t.Errorf("304 carried a body %q, want empty", string(body2))
	}
	if et := resp2.Header.Get("ETag"); et != `"v1"` {
		t.Errorf("304 ETag = %q, want %q (validator belongs on a 304)", et, `"v1"`)
	}
}

// TestServeFileEtag: serveFileEtag sets a content-hash ETag, and a
// conditional GET carrying it gets a bodyless 304 (via http.ServeContent).
func TestServeFileEtag(t *testing.T) {
	ResetForTest()
	dir := t.TempDir()
	content := []byte("<h1>cached</h1>")
	fpath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(fpath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	wantTag := sha256Tag(content)

	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	serveOnce(srv, func(req Value) {
		_, _ = serveFileEtagFn(noCtx, []Value{req, interpreter.StringVal(fpath)})
	})
	resp, err := http.Get("http://" + addr + "/whatever")
	if err != nil {
		t.Fatalf("GET 1: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != string(content) {
		t.Errorf("first = %d %q, want 200 %q", resp.StatusCode, string(body), string(content))
	}
	if et := resp.Header.Get("ETag"); et != wantTag {
		t.Errorf("ETag = %q, want %q (quoted sha256)", et, wantTag)
	}

	serveOnce(srv, func(req Value) {
		_, _ = serveFileEtagFn(noCtx, []Value{req, interpreter.StringVal(fpath)})
	})
	req, _ := http.NewRequest("GET", "http://"+addr+"/whatever", nil)
	req.Header.Set("If-None-Match", wantTag)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET 2: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotModified {
		t.Errorf("conditional GET status = %d, want 304", resp2.StatusCode)
	}
	if len(body2) != 0 {
		t.Errorf("304 carried a body %q", string(body2))
	}
}

// TestServeFileEtagInvalidatesOnEdit: editing the file (new bytes + a later
// mtime) yields a new ETag - the mtime+size-keyed cache does not serve a stale
// digest.
func TestServeFileEtagInvalidatesOnEdit(t *testing.T) {
	ResetForTest()
	dir := t.TempDir()
	fpath := filepath.Join(dir, "a.txt")
	v1 := []byte("one")
	if err := os.WriteFile(fpath, v1, 0o644); err != nil {
		t.Fatal(err)
	}
	if tag := contentEtag(fpath); tag != sha256Tag(v1) {
		t.Fatalf("tag v1 = %q, want %q", tag, sha256Tag(v1))
	}
	// Rewrite with different bytes and force a later mtime so the change is
	// visible even if the two writes land in the same filesystem-time tick.
	v2 := []byte("two!!")
	if err := os.WriteFile(fpath, v2, 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(fpath, future, future); err != nil {
		t.Fatal(err)
	}
	if tag := contentEtag(fpath); tag != sha256Tag(v2) {
		t.Errorf("tag v2 = %q, want %q (stale cache not invalidated)", tag, sha256Tag(v2))
	}
}

// TestServeDirEtag: serveDirEtag maps the request path under root and sets a
// content ETag, while keeping serveDir's traversal guard (a backslash is 400).
func TestServeDirEtag(t *testing.T) {
	ResetForTest()
	dir := t.TempDir()
	content := []byte("body{}")
	if err := os.WriteFile(filepath.Join(dir, "style.css"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	serveOnce(srv, func(req Value) {
		_, _ = serveDirEtagFn(noCtx, []Value{req, interpreter.StringVal(dir)})
	})
	resp, err := http.Get("http://" + addr + "/style.css")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != string(content) {
		t.Errorf("response = %d %q, want 200 %q", resp.StatusCode, string(body), string(content))
	}
	if et := resp.Header.Get("ETag"); et != sha256Tag(content) {
		t.Errorf("ETag = %q, want %q", et, sha256Tag(content))
	}
}
