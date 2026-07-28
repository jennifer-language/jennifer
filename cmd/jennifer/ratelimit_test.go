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

// A .j program driving the ratelimit module over the memcache backend (via the
// kvstore selector) against the in-process memcached server asserts the
// fixed-window dispatch: the first `limit` hits on a key are allowed and the next
// is denied (with a positive retry-after), remaining counts down to 0, and a
// different key has its own independent budget. The algorithm itself is verified
// deterministically in the overlay; this proves the memcache plumbing. A large
// window avoids straddling a window boundary mid-test.
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
	kvstoreMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "kvstore.j"))
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
import %q as kvstore;
import %q as memcache;
def mc as memcache.Session init memcache.connect(memcache.Options{host: "127.0.0.1", port: %d});
def st as kvstore.Store init kvstore.memcacheStore($mc);
def lim as ratelimit.Limiter init ratelimit.fixedWindow($st, 3, 3600);
def r1 as ratelimit.Result init ratelimit.check($lim, "ip:a");
testing.assertTrue($r1.allowed);
testing.assertEqual($r1.remaining, 2);
def r2 as ratelimit.Result init ratelimit.check($lim, "ip:a");
testing.assertTrue($r2.allowed);
def r3 as ratelimit.Result init ratelimit.check($lim, "ip:a");
testing.assertTrue($r3.allowed);
testing.assertEqual($r3.remaining, 0);
def r4 as ratelimit.Result init ratelimit.check($lim, "ip:a");
testing.assertFalse($r4.allowed);
testing.assertTrue($r4.retryAfter > 0);
# a different key has its own budget
def rb as ratelimit.Result init ratelimit.check($lim, "ip:b");
testing.assertTrue($rb.allowed);
testing.assertEqual($rb.remaining, 2);
memcache.quit($mc);`, ratelimitMod, kvstoreMod, memcacheMod, port)
	progPath := filepath.Join(dir, "rl.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("ratelimit program failed with code %d", code)
	}
}
