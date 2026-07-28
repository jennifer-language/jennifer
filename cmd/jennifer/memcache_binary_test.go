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

// fakeMemcachedCas serves the byte-exact + CAS + multi-key surface: set / cas
// (length-prefixed data blocks read by count, so a binary value round-trips),
// multi-key get, and gets (which appends a CAS token to the VALUE line). Values
// are kept as raw bytes with a per-key CAS version that bumps on each store.
func fakeMemcachedCas(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	store := map[string][]byte{}
	cas := map[string]int{}
	casCtr := 0
	readData := func(n int) ([]byte, bool) {
		data := make([]byte, n)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, false
		}
		r.Discard(2) // trailing CRLF after the data block
		return data, true
	}
	writeValues := func(keys []string, withCas bool) {
		var sb strings.Builder
		for _, k := range keys {
			v, ok := store[k]
			if !ok {
				continue
			}
			if withCas {
				fmt.Fprintf(&sb, "VALUE %s 0 %d %d\r\n%s\r\n", k, len(v), cas[k], v)
			} else {
				fmt.Fprintf(&sb, "VALUE %s 0 %d\r\n%s\r\n", k, len(v), v)
			}
		}
		sb.WriteString("END\r\n")
		fmt.Fprint(conn, sb.String())
	}
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
		case "set":
			n, _ := strconv.Atoi(parts[4])
			data, ok := readData(n)
			if !ok {
				return
			}
			store[parts[1]] = data
			casCtr++
			cas[parts[1]] = casCtr
			fmt.Fprint(conn, "STORED\r\n")
		case "cas":
			n, _ := strconv.Atoi(parts[4])
			casId, _ := strconv.Atoi(parts[5])
			data, ok := readData(n)
			if !ok {
				return
			}
			if _, present := store[parts[1]]; !present {
				fmt.Fprint(conn, "NOT_FOUND\r\n")
			} else if cas[parts[1]] != casId {
				fmt.Fprint(conn, "EXISTS\r\n")
			} else {
				store[parts[1]] = data
				casCtr++
				cas[parts[1]] = casCtr
				fmt.Fprint(conn, "STORED\r\n")
			}
		case "get":
			writeValues(parts[1:], false)
		case "gets":
			writeValues(parts[1:], true)
		case "quit":
			return
		default:
			fmt.Fprint(conn, "ERROR\r\n")
		}
	}
}

// A .j program drives the byte-exact get/set, multi-key get, and gets/cas
// check-and-set against the mock: a binary value (NUL / CR / LF / 0xFF)
// round-trips through setBytes/getBytes, getMulti returns only the present keys,
// and a cas succeeds with a fresh token, reports "exists" on a stale one, and
// "not_found" on a missing key.
func TestMemcacheBinaryCasMulti(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go fakeMemcachedCas(ln)

	mcMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "memcache.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as memcache;
def mc as memcache.Session init memcache.connect(memcache.Options{host: "127.0.0.1", port: %d});

# byte-exact round-trip: NUL, CR, LF, 0xFF
def blob as bytes;
$blob[] = 0;
$blob[] = 13;
$blob[] = 10;
$blob[] = 255;
$blob[] = 66;
memcache.setBytes($mc, "bin", $blob, 0);
def got as bytes init memcache.getBytes($mc, "bin");
testing.assertEqual(len($got), 5);
testing.assertEqual($got[1], 13);
testing.assertEqual($got[2], 10);
testing.assertEqual($got[3], 255);
testing.assertEqual(len(memcache.getBytes($mc, "nope")), 0);

# multi-key get: only present keys appear
memcache.set($mc, "a", "1", 0);
memcache.set($mc, "b", "2", 0);
def m as map of string to string init memcache.getMulti($mc, ["a", "b", "missing"]);
testing.assertEqual(len($m), 2);
testing.assertEqual($m["a"], "1");
testing.assertEqual($m["b"], "2");

# gets + cas
def it as memcache.Item init memcache.gets($mc, "a");
testing.assertTrue($it.found);
testing.assertEqual($it.value, "1");
testing.assertEqual(memcache.cas($mc, "a", "1b", 0, $it.cas), "stored");
# the old token is now stale -> exists
testing.assertEqual(memcache.cas($mc, "a", "x", 0, $it.cas), "exists");
# a missing key -> not_found
def gone as memcache.Item init memcache.gets($mc, "ghost");
testing.assertFalse($gone.found);
testing.assertEqual(memcache.cas($mc, "ghost", "y", 0, 999), "not_found");
memcache.quit($mc);`, mcMod, port)
	progPath := filepath.Join(dir, "binary.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("memcache binary/cas program failed with code %d", code)
	}
}
