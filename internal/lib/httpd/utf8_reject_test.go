// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package httpdlib

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// rawBytesRequest writes one raw HTTP/1.1 request and returns the status and body.
func rawBytesRequest(t *testing.T, addr, req string) (int, string) {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// TestServerRejectsInvalidUTF8: a request whose decoded path or query, or a
// header value, is not valid UTF-8 is answered 400 by the engine and never
// reaches accept; a valid request (percent-encoded UTF-8 included) still does.
// Before the fix these requests were handed to the program.
func TestServerRejectsInvalidUTF8(t *testing.T) {
	ResetForTest()
	srv, addr := startServer(t)
	defer shutdownFn(noCtx, []Value{srv})

	accepted := make(chan string, 8)
	go func() {
		for {
			req, err := acceptFn(noCtx, []Value{srv})
			if err != nil {
				return
			}
			p, perr := pathFn(noCtx, []Value{req})
			q, qerr := queryFn(noCtx, []Value{req, interpreter.StringVal("x")})
			h, herr := headerFn(noCtx, []Value{req, interpreter.StringVal("X-Test")})
			got := p.Str + "|" + q.Str + "|" + h.Str
			if perr != nil || qerr != nil || herr != nil {
				got = "error"
			}
			accepted <- got
			_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("ok")})
		}
	}()

	bad := []struct{ name, req, part string }{
		{"path %FF", "GET /a%FF HTTP/1.1\r\nHost: x\r\n\r\n", "path"},
		{"path raw byte", "GET /a\xff HTTP/1.1\r\nHost: x\r\n\r\n", "path"},
		{"query %FF", "GET /ok?x=%FF HTTP/1.1\r\nHost: x\r\n\r\n", "query"},
		{"query key %C3", "GET /ok?%C3=1 HTTP/1.1\r\nHost: x\r\n\r\n", "query"},
		{"query raw byte", "GET /ok?x=\xfe HTTP/1.1\r\nHost: x\r\n\r\n", "query"},
		{"latin-1 header", "GET /ok HTTP/1.1\r\nHost: x\r\nX-Test: caf\xe9\r\n\r\n", "header X-Test"},
		{"with body", "POST /a%FF HTTP/1.1\r\nHost: x\r\nContent-Length: 3\r\n\r\nabc", "path"},
	}
	for _, tc := range bad {
		status, body := rawBytesRequest(t, addr, tc.req)
		if status != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", tc.name, status)
		}
		if want := "bad request: " + tc.part + " is not valid UTF-8"; strings.TrimSpace(body) != want {
			t.Errorf("%s: body %q, want %q", tc.name, body, want)
		}
	}
	select {
	case got := <-accepted:
		t.Fatalf("an invalid request reached accept: %q", got)
	default:
	}

	status, _ := rawBytesRequest(t, addr, "GET /caf%C3%A9?x=%C3%A9+b&y=%2541 HTTP/1.1\r\nHost: x\r\nX-Test: caf\xc3\xa9\r\n\r\n")
	if status != http.StatusOK {
		t.Fatalf("valid request: status %d", status)
	}
	if got, want := <-accepted, "/café|é b|café"; got != want {
		t.Errorf("valid request: got %q, want %q", got, want)
	}
}
