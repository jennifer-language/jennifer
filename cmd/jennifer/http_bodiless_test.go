// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// serveRawHeld answers each request on a connection with the next reply from
// replies (one per request, in order) and holds the connection open afterwards,
// as a keep-alive server does. A client that waits for body bytes a reply does
// not carry therefore blocks instead of seeing EOF.
func serveRawHeld(t *testing.T, replies []string) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				for _, reply := range replies {
					for {
						line, err := br.ReadString('\n')
						if err != nil {
							return
						}
						if line == "\r\n" {
							break
						}
					}
					fmt.Fprint(c, reply)
				}
				time.Sleep(30 * time.Second)
			}(c)
		}
	}()
	return ln
}

// runHTTPProgram runs a .j program (with `http` imported) and fails the test if
// it does not pass within the window - a hang means a body was waited for.
func runHTTPProgram(t *testing.T, body string) {
	t.Helper()
	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	progPath := filepath.Join(t.TempDir(), "bodiless.j")
	prog := fmt.Sprintf("use testing;\nimport %q as http;\n%s", httpMod, body)
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
			t.Fatalf("program failed with code %d", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request hung: the client waited for a body the response does not have")
	}
}

// A response to HEAD, and every 1xx / 204 / 304, has no body whatever its
// Content-Length or Transfer-Encoding says (RFC 9110). The client must read only
// the head - not wait for the advertised length, and not check the byte cap
// against it - and must skip an interim 1xx to reach the final response.
func TestHttpBodilessResponses(t *testing.T) {
	big := "HTTP/1.1 200 OK\r\nContent-Length: 540354\r\n\r\n"
	cases := []struct {
		name, method, reply, check string
		maxBytes                   int
	}{
		{"HEAD with a large Content-Length", "HEAD", big,
			`testing.assertEqual($r.status, 200); testing.assertEqual($r.body, ""); testing.assertEqual(http.header($r, "content-length"), "540354");`, 0},
		{"HEAD under a small byte cap", "HEAD", big,
			`testing.assertEqual($r.status, 200); testing.assertEqual($r.body, "");`, 1024},
		{"HEAD advertising chunked", "HEAD", "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n",
			`testing.assertEqual($r.status, 200); testing.assertEqual($r.body, "");`, 0},
		{"304 with a Content-Length", "GET", "HTTP/1.1 304 Not Modified\r\nContent-Length: 1234\r\n\r\n",
			`testing.assertEqual($r.status, 304); testing.assertEqual($r.body, "");`, 0},
		{"204 without framing headers", "GET", "HTTP/1.1 204 No Content\r\n\r\n",
			`testing.assertEqual($r.status, 204); testing.assertEqual($r.body, "");`, 0},
		{"100 Continue before the final response", "GET",
			"HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello",
			`testing.assertEqual($r.status, 200); testing.assertEqual($r.body, "hello");`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ln := serveRawHeld(t, []string{tc.reply})
			defer ln.Close()
			url := "http://" + ln.Addr().String() + "/x"
			runHTTPProgram(t, fmt.Sprintf(
				"def r as http.Response init http.requestWith(%q, %q, {}, \"\", 5000, %d);\n%s",
				tc.method, url, tc.maxBytes, tc.check))
		})
	}
	t.Run("http.head", func(t *testing.T) {
		ln := serveRawHeld(t, []string{big})
		defer ln.Close()
		runHTTPProgram(t, fmt.Sprintf(
			"def r as http.Response init http.head(%q, {});\ntesting.assertEqual($r.status, 200);",
			"http://"+ln.Addr().String()+"/x"))
	})
}

// On a keep-alive session a HEAD answer must leave the connection in step: had
// the client consumed the next response as the HEAD's "body" (or blocked on
// it), the GET that follows would read the wrong bytes.
func TestHttpSessionHeadThenGet(t *testing.T) {
	ln := serveRawHeld(t, []string{
		"HTTP/1.1 200 OK\r\nContent-Length: 11\r\n\r\n",
		"HTTP/1.1 200 OK\r\nContent-Length: 11\r\n\r\nhello world",
	})
	defer ln.Close()
	base := "http://" + ln.Addr().String()
	runHTTPProgram(t, strings.Join([]string{
		fmt.Sprintf("def s as http.Session init http.connect(%q, http.defaultOptions());", base),
		`def h as http.Exchange init http.exchange($s, "HEAD", "/f", {}, "");`,
		`testing.assertEqual($h.response.status, 200);`,
		`testing.assertEqual(len($h.response.body), 0);`,
		`def g as http.Exchange init http.exchange($h.session, "GET", "/f", {}, "");`,
		`testing.assertEqual($g.response.status, 200);`,
		`testing.assertEqual($g.response.body, "hello world");`,
		`http.close($g.session);`,
	}, "\n"))
}
