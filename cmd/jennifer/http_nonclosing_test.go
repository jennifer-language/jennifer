// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestHttpOneShotNonClosingServer is the regression test for the one-shot
// request path ignoring response framing (bug-http-report.md). A server that
// frames its response correctly (Content-Length or chunked) but ignores the
// client's Connection: close and holds the socket open must not make
// http.requestWith block until the idle timeout: the client must return as soon
// as the framing says the body is complete.
//
// Every *closing* test server (Go net/http, Python http.server, a Jennifer
// httpd) masks this bug, because EOF arrives and even a read-to-EOF reader
// completes; only a non-closing peer - a Cisco ASA gateway, some proxies -
// exposes it. Both framing styles are covered because the one-shot path used to
// frame neither.
func TestHttpOneShotNonClosingServer(t *testing.T) {
	const body = "<hello/>"
	cases := []struct {
		name  string
		reply string
	}{
		{
			name: "content-length",
			reply: fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/xml\r\n"+
				"Content-Length: %d\r\nConnection: Keep-Alive\r\n\r\n%s",
				len(body), body),
		},
		{
			name: "chunked",
			reply: fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/xml\r\n"+
				"Transfer-Encoding: chunked\r\nConnection: Keep-Alive\r\n\r\n"+
				"%x\r\n%s\r\n0\r\n\r\n", len(body), body),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			go func() {
				for {
					c, err := ln.Accept()
					if err != nil {
						return
					}
					go func(c net.Conn) {
						// Drain the request head, reply with correct framing, then
						// DELIBERATELY hold the socket open past the test window
						// (ignore Connection: close) - the behaviour a Cisco
						// appliance / a keep-alive proxy shows. If framing works
						// the client has already returned.
						br := bufio.NewReader(c)
						for {
							line, err := br.ReadString('\n')
							if err != nil {
								c.Close()
								return
							}
							if line == "\r\n" {
								break
							}
						}
						fmt.Fprint(c, tc.reply)
						time.Sleep(30 * time.Second)
						c.Close()
					}(c)
				}
			}()

			httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			url := "http://" + ln.Addr().String() + "/x"
			// A generous 5s idle timeout: if the fix works the request returns in
			// milliseconds; if it does not it blocks and the select below fires
			// first, failing the test as a hang rather than a caught timeout.
			prog := fmt.Sprintf(`use testing;
import %q as http;
def r as http.Response init http.requestWith("GET", %q, {}, "", 5000, 0);
testing.assertEqual($r.status, 200);
testing.assertEqual($r.body, %q);`, httpMod, url, body)
			progPath := filepath.Join(dir, "nonclosing.j")
			if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
				t.Fatal(err)
			}

			done := make(chan int, 1)
			go func() {
				_, code := loadForTest(progPath)
				done <- code
			}()
			select {
			case code := <-done:
				if code != testExitPass {
					t.Fatalf("one-shot request against non-closing %s server failed with code %d", tc.name, code)
				}
			case <-time.After(3 * time.Second):
				t.Fatalf("one-shot request against non-closing %s server hung (response framing ignored)", tc.name)
			}
		})
	}
}
