// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package httpdlib

import (
	"bufio"
	"bytes"
	"net"
	"net/http"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestContentLengthMaxInt64: a declared Content-Length of MaxInt64 must not
// wrap the read limit (contentLength+1) to MinInt64. Before the fix the body
// read nothing and the program got an empty body; now an over-cap body is 413.
func TestContentLengthMaxInt64(t *testing.T) {
	ResetForTest()
	srvV, err := listenWithFn(noCtx, []Value{interpreter.StringVal("127.0.0.1:0"), optionsVal(1024, 0)})
	if err != nil {
		t.Fatalf("listenWith: %v", err)
	}
	defer shutdownFn(noCtx, []Value{srvV})
	addrV, _ := addressFn(noCtx, []Value{srvV})
	handled := make(chan int, 1)
	serveOnce(srvV, func(req Value) {
		b, _ := bodyFn(noCtx, []Value{req})
		handled <- len(b.Bytes)
		_, _ = respondFn(noCtx, []Value{req, interpreter.IntVal(200), interpreter.StringVal("ok")})
	})
	c, err := net.Dial("tcp", addrV.Str)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	req := "POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 9223372036854775807\r\n\r\n"
	if _, err := c.Write(append([]byte(req), bytes.Repeat([]byte("x"), 2048)...)); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	resp.Body.Close()
	select {
	case n := <-handled:
		t.Fatalf("program received a %d-byte body for a 2048-byte over-cap request (status %d)", n, resp.StatusCode)
	default:
	}
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413", resp.StatusCode)
	}
}
