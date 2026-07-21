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

// A .j program driving the session module against the in-process memcached
// server (fakeMemcached, shared with the memcache test) asserts the whole
// lifecycle: a fresh session loads empty, saved data (including a non-ASCII
// value, proving the base64 wrap) loads back, touch reports present vs absent,
// an unknown ID loads empty, and destroy removes it (true then false, then a
// confirming empty load). A mismatch throws and fails loadForTest.
func TestSessionLifecycle(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go fakeMemcached(ln)

	sessionMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "session.j"))
	if err != nil {
		t.Fatal(err)
	}
	memcacheMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "memcache.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as session;
import %q as memcache;
def mc as memcache.Session init memcache.connect(memcache.Options{host: "127.0.0.1", port: %d});
def id as string init session.create($mc, 60);
testing.assertEqual(len(session.load($mc, $id)), 0);
def d as map of string to string init {"user": "ada", "name": "José"};
session.save($mc, $id, $d, 60);
def back as map of string to string init session.load($mc, $id);
testing.assertEqual($back["user"], "ada");
testing.assertEqual($back["name"], "José");
testing.assertTrue(session.touch($mc, $id, 120));
testing.assertEqual(len(session.load($mc, "no-such-session")), 0);
testing.assertFalse(session.touch($mc, "no-such-session", 60));
testing.assertTrue(session.destroy($mc, $id));
testing.assertFalse(session.destroy($mc, $id));
testing.assertEqual(len(session.load($mc, $id)), 0);
memcache.quit($mc);`, sessionMod, memcacheMod, port)
	progPath := filepath.Join(dir, "sess.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("session lifecycle program failed with code %d", code)
	}
}
