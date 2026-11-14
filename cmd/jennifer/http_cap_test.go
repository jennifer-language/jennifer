// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// rawHTTPServer answers every connection with the bytes respond returns for it,
// written in the pieces given (with a short pause between pieces, so a client
// read sees a piece boundary), then closes the connection. It returns the base
// URL; the listener closes with the test.
func rawHTTPServer(t *testing.T, pieces ...string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadString('\n')
					if err != nil || line == "\r\n" {
						break
					}
				}
				for i, p := range pieces {
					if i > 0 {
						time.Sleep(30 * time.Millisecond)
					}
					if _, err := c.Write([]byte(p)); err != nil {
						return
					}
				}
			}(c)
		}
	}()
	return "http://" + ln.Addr().String() + "/"
}

// capProg is a program that fetches url with maxBytes and checks the outcome:
// a body of wantLen bytes, or (wantLen < 0) an error of kind "limit" containing
// wantErr - a cap, told apart from a failed exchange by its kind.
func capProg(url string, maxBytes, wantLen int, wantErr string) string {
	return fmt.Sprintf(`
def msg as string init "";
def n as int init -1;
try {
    def r as http.Response init http.requestWith("GET", %q, {}, "", 5000, %d);
    $n = len($r.body);
} catch (e) {
    $msg = $e.kind + ": " + $e.message;
}
if (%d >= 0) {
    testing.assertEqual($msg, "");
    testing.assertEqual($n, %d);
} else {
    testing.assertContains($msg, "limit: http: ");
    testing.assertContains($msg, %q);
}
`, url, maxBytes, wantLen, wantLen, wantErr)
}

// bigHeaders is a head padded with headers well past the body sizes the tests
// use, so a cap that still counted header bytes would fail every case.
func bigHeaders(framing string) string {
	return "HTTP/1.1 200 OK\r\n" + framing + "X-Pad: " + strings.Repeat("p", 4000) + "\r\n\r\n"
}

func chunked(body string, size int) string {
	var b strings.Builder
	for len(body) > 0 {
		n := size
		if n > len(body) {
			n = len(body)
		}
		fmt.Fprintf(&b, "%x\r\n%s\r\n", n, body[:n])
		body = body[n:]
	}
	b.WriteString("0\r\n\r\n")
	return b.String()
}

// maxBytes caps the body alone, for every framing: a body of exactly maxBytes
// passes however large the headers, and one byte more fails.
func TestHttpMaxBytesCountsBodyOnly(t *testing.T) {
	body100 := strings.Repeat("a", 100)
	body101 := strings.Repeat("a", 101)
	cases := []struct {
		name    string
		pieces  []string
		wantLen int
	}{
		{"content-length exact", []string{bigHeaders("Content-Length: 100\r\n") + body100}, 100},
		{"content-length over", []string{bigHeaders("Content-Length: 101\r\n") + body101}, -1},
		{"chunked exact", []string{bigHeaders("Transfer-Encoding: chunked\r\n"), chunked(body100, 7)}, 100},
		{"chunked over", []string{bigHeaders("Transfer-Encoding: chunked\r\n"), chunked(body101, 7)}, -1},
		// To-close bodies, read with the head (the whole budget used before
		// net.readAll runs) and after it.
		{"to-close exact, one write", []string{"HTTP/1.1 200 OK\r\n\r\n" + body100}, 100},
		{"to-close over, one write", []string{"HTTP/1.1 200 OK\r\n\r\n" + body101}, -1},
		{"to-close exact, body later", []string{bigHeaders(""), body100}, 100},
		{"to-close over, body later", []string{bigHeaders(""), body101}, -1},
		{"to-close exact then one more", []string{"HTTP/1.1 200 OK\r\n\r\n" + body100, "b"}, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			url := rawHTTPServer(t, c.pieces...)
			runHTTPProg(t, "cap.j", capProg(url, 100, c.wantLen, "response body exceeds 100 bytes"))
		})
	}
}

// The head has its own 1 MiB cap whatever maxBytes says, an unlimited body
// included, and its own message.
func TestHttpHeadersHaveTheirOwnCap(t *testing.T) {
	huge := "HTTP/1.1 200 OK\r\nX-Pad: " + strings.Repeat("p", 1<<20) + "\r\nContent-Length: 2\r\n\r\nok"
	for _, maxBytes := range []int{0, -1, 1 << 30} {
		t.Run(fmt.Sprint(maxBytes), func(t *testing.T) {
			url := rawHTTPServer(t, huge)
			runHTTPProg(t, "head.j", capProg(url, maxBytes, -1, "response headers exceed 1048576 bytes"))
		})
	}
	// A large but legal head (64 KiB of cookies) passes a tiny body cap.
	cookies := ""
	for i := 0; i < 16; i++ {
		cookies += fmt.Sprintf("Set-Cookie: c%d=%s\r\n", i, strings.Repeat("v", 4000))
	}
	url := rawHTTPServer(t, "HTTP/1.1 200 OK\r\n"+cookies+"Content-Length: 2\r\n\r\nok")
	runHTTPProg(t, "cookies.j", capProg(url, 2, 2, ""))
}

// Under a body cap, a chunked body's framing (here a flood of trailers) is
// bounded too, so a server cannot stream without end behind a small cap. An
// unlimited body gets no framing bound.
func TestHttpChunkedFramingIsBounded(t *testing.T) {
	trailers := strings.Repeat("X-T: "+strings.Repeat("t", 4000)+"\r\n", 300)
	resp := "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n" + trailers + "\r\n"
	url := rawHTTPServer(t, resp)
	runHTTPProg(t, "framing.j", capProg(url, 100, -1, "chunked framing exceeds 1048576 bytes"))
	url = rawHTTPServer(t, resp)
	runHTTPProg(t, "framing-unlimited.j", capProg(url, -1, 5, ""))
}
