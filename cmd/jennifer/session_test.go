// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// A .j program driving the session module over the memcache backend (via the
// kvstore selector) against the in-process memcached server asserts the whole
// lifecycle: a fresh session loads empty, saved structured data (a nested object
// plus a non-ASCII value, proving the json.Value surface and base64 wrap) loads
// back, touch reports present vs absent, an unknown ID loads empty, and destroy
// removes it (true then false). A mismatch throws and fails loadForTest.
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
use json;
import %q as session;
import %q as kvstore;
import %q as memcache;
def mc as memcache.Session init memcache.connect(memcache.Options{host: "127.0.0.1", port: %d});
def st as kvstore.Store init kvstore.memcacheStore($mc);
def id as string init session.create($st, 60);
testing.assertEqual(json.length(session.load($st, $id), ""), 0);
def d as json.Value init json.map();
$d = json.set($d, "/user", "ada");
$d = json.set($d, "/name", "José");
$d = json.set($d, "/prefs", json.map());
$d = json.set($d, "/prefs/theme", "dark");
session.save($st, $id, $d, 60);
def back as json.Value init session.load($st, $id);
testing.assertEqual(json.asString($back, "/user"), "ada");
testing.assertEqual(json.asString($back, "/name"), "José");
testing.assertEqual(json.asString($back, "/prefs/theme"), "dark");
testing.assertTrue(session.touch($st, $id, 120));
testing.assertEqual(json.length(session.load($st, "no-such-session"), ""), 0);
testing.assertFalse(session.touch($st, "no-such-session", 60));
testing.assertTrue(session.destroy($st, $id));
testing.assertFalse(session.destroy($st, $id));
memcache.quit($mc);`, sessionMod, kvstoreMod, memcacheMod, port)
	progPath := filepath.Join(dir, "sess.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("session lifecycle program failed with code %d", code)
	}
}
