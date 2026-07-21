// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package httpdlib

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
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
	over := bytes.Repeat([]byte("x"), int(maxBodyBytes)+1)
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
	atCap := bytes.Repeat([]byte("y"), int(maxBodyBytes))
	resp2, err := http.Post("http://"+addr+"/", "application/octet-stream", bytes.NewReader(atCap))
	if err != nil {
		t.Fatalf("POST at-cap: %v", err)
	}
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Errorf("at-cap status = %d, want 200", resp2.StatusCode)
	}
	if gotLen != maxBodyBytes {
		t.Errorf("at-cap body length seen by the program = %d, want %d", gotLen, maxBodyBytes)
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
