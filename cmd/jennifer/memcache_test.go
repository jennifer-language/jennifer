// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeMemcached accepts one connection and serves an in-memory store over
// memcached's classic text protocol - set / add (with a length-prefixed data
// block), get (VALUE ... END), delete, incr / decr, and touch - so the client's
// storage-header framing and line / value parsing run on a real socket in CI.
func fakeMemcached(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	store := map[string]string{}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		parts := strings.Fields(strings.TrimRight(line, "\r\n"))
		if len(parts) == 0 {
			return
		}
		switch parts[0] {
		case "set", "add":
			key := parts[1]
			n, _ := strconv.Atoi(parts[4])
			data := make([]byte, n)
			if _, err := io.ReadFull(r, data); err != nil {
				return
			}
			r.Discard(2) // trailing CRLF after the data block
			if parts[0] == "add" {
				if _, ok := store[key]; ok {
					fmt.Fprint(conn, "NOT_STORED\r\n")
					continue
				}
			}
			store[key] = string(data)
			fmt.Fprint(conn, "STORED\r\n")
		case "get":
			if v, ok := store[parts[1]]; ok {
				fmt.Fprintf(conn, "VALUE %s 0 %d\r\n%s\r\nEND\r\n", parts[1], len(v), v)
			} else {
				fmt.Fprint(conn, "END\r\n")
			}
		case "delete":
			if _, ok := store[parts[1]]; ok {
				delete(store, parts[1])
				fmt.Fprint(conn, "DELETED\r\n")
			} else {
				fmt.Fprint(conn, "NOT_FOUND\r\n")
			}
		case "incr", "decr":
			v, ok := store[parts[1]]
			if !ok {
				fmt.Fprint(conn, "NOT_FOUND\r\n")
				continue
			}
			cur, _ := strconv.Atoi(v)
			d, _ := strconv.Atoi(parts[2])
			if parts[0] == "incr" {
				cur += d
			} else {
				cur -= d
				if cur < 0 {
					cur = 0
				}
			}
			store[parts[1]] = strconv.Itoa(cur)
			fmt.Fprintf(conn, "%d\r\n", cur)
		case "touch":
			if _, ok := store[parts[1]]; ok {
				fmt.Fprint(conn, "TOUCHED\r\n")
			} else {
				fmt.Fprint(conn, "NOT_FOUND\r\n")
			}
		case "quit":
			return
		default:
			fmt.Fprint(conn, "ERROR\r\n")
		}
	}
}

// A .j program driving the memcache client against the in-process server
// asserts the whole command surface: a set/get round-trip, a miss returning "",
// add's store-if-absent (true then false), incr / decr on a counter (and -1 on
// an absent key), touch on present vs absent keys, and delete (true then false,
// then a confirming miss). A mismatch throws and fails loadForTest.
func TestMemcacheCommands(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go fakeMemcached(ln)

	mcMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "memcache.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as memcache;
def mc as memcache.Session init memcache.connect(memcache.Options{host: "127.0.0.1", port: %d});
memcache.set($mc, "greeting", "hello", 60);
testing.assertEqual(memcache.get($mc, "greeting"), "hello");
testing.assertEqual(memcache.get($mc, "missing"), "");
# A multi-byte value: "café" is 5 bytes / 4 runes. The value must round-trip
# byte-exact - a rune-indexed reader mis-slices the trailing CRLF into it.
memcache.set($mc, "accent", "café", 60);
testing.assertEqual(memcache.get($mc, "accent"), "café");
testing.assertTrue(memcache.add($mc, "fresh", "1", 60));
testing.assertFalse(memcache.add($mc, "greeting", "again", 60));
memcache.set($mc, "counter", "10", 0);
testing.assertEqual(memcache.incr($mc, "counter", 5), 15);
testing.assertEqual(memcache.decr($mc, "counter", 3), 12);
testing.assertEqual(memcache.incr($mc, "nope", 1), -1);
testing.assertTrue(memcache.touch($mc, "greeting", 120));
testing.assertFalse(memcache.touch($mc, "ghost", 120));
testing.assertTrue(memcache.delete($mc, "greeting"));
testing.assertFalse(memcache.delete($mc, "greeting"));
testing.assertEqual(memcache.get($mc, "greeting"), "");
memcache.quit($mc);`, mcMod, port)
	progPath := filepath.Join(dir, "cmds.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("memcache command program failed with code %d", code)
	}
}
