// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestHttpTimeout proves the http idle timeout: a server that accepts the
// connection but never responds must make the client fail (a catchable "read
// timed out" error) rather than block forever - the resource-exhaustion class
// this guards against. The .j program wraps the request in try / catch and
// asserts it was caught; if the timeout did not work the read would hang and the
// test would time out instead of passing.
func TestHttpTimeout(t *testing.T) {
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
			// Accept but never write a response; hold it briefly, then drop.
			go func(c net.Conn) {
				time.Sleep(3 * time.Second)
				c.Close()
			}(c)
		}
	}()

	httpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	url := "http://" + ln.Addr().String() + "/"
	prog := fmt.Sprintf(`use testing;
import %q as http;
def caught as bool init false;
try {
    def r as http.Response init http.requestWith("GET", %q, {}, "", 250, 0);
} catch (e) {
    $caught = true;
}
testing.assertTrue($caught);`, httpMod, url)
	progPath := filepath.Join(dir, "timeout.j")
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
			t.Fatalf("http timeout program failed with code %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request did not time out (idle timeout not enforced)")
	}
}
