// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// A .j program driving the ratelimit module against the in-process memcached
// server (fakeMemcached, shared with the memcache test) asserts the fixed-window
// logic: the first `limit` hits on a key are allowed and the next is denied,
// `remaining` counts down to 0, and a different key has its own independent
// budget (proving the incr-then-add window is per key). A mismatch throws and
// fails loadForTest.
func TestRatelimit(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go fakeMemcached(ln)

	ratelimitMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "ratelimit.j"))
	if err != nil {
		t.Fatal(err)
	}
	memcacheMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "memcache.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as ratelimit;
import %q as memcache;
def mc as memcache.Session init memcache.connect(memcache.Options{host: "127.0.0.1", port: %d});
# three hits allowed per 60s window on this key
testing.assertEqual(ratelimit.remaining($mc, "ip:a", 3), 3);
testing.assertTrue(ratelimit.allow($mc, "ip:a", 3, 60));
testing.assertTrue(ratelimit.allow($mc, "ip:a", 3, 60));
testing.assertEqual(ratelimit.remaining($mc, "ip:a", 3), 1);
testing.assertTrue(ratelimit.allow($mc, "ip:a", 3, 60));
testing.assertFalse(ratelimit.allow($mc, "ip:a", 3, 60));
testing.assertEqual(ratelimit.remaining($mc, "ip:a", 3), 0);
# a different key has its own budget
testing.assertEqual(ratelimit.remaining($mc, "ip:b", 3), 3);
testing.assertTrue(ratelimit.allow($mc, "ip:b", 3, 60));
testing.assertEqual(ratelimit.remaining($mc, "ip:b", 3), 2);
memcache.quit($mc);`, ratelimitMod, memcacheMod, port)
	progPath := filepath.Join(dir, "rl.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("ratelimit program failed with code %d", code)
	}
}
