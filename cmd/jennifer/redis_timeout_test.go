// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// TestRedisTimeout proves the read timeout on a net-based protocol client: a
// server that accepts the connection but never answers a command must make the
// client fail (a catchable error) rather than block a worker forever. The same
// net.setDeadline mechanism backs memcache / smtp / pop / imap / mqtt. The .j
// program lowers Session.timeout, wraps the command in try / catch, and asserts
// it was caught; a watchdog fails the test if the command ever hangs.
func TestRedisTimeout(t *testing.T) {
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
				time.Sleep(3 * time.Second)
				c.Close()
			}(c)
		}
	}()
	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)

	redisMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "redis.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as redis;
def opts as redis.Options init redis.Options{host: %q, port: %d, security: "", user: "", password: "", db: 0};
def s as redis.Session init redis.connect($opts);
$s.timeout = 250;
def caught as bool init false;
try {
    def r as redis.Reply init redis.command($s, ["PING"]);
} catch (e) {
    $caught = true;
}
testing.assertTrue($caught);`, redisMod, host, port)
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
			t.Fatalf("redis timeout program failed with code %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("redis command did not time out (read deadline not enforced)")
	}
}
