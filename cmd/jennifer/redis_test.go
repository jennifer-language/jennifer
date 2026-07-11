// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeRedis accepts one connection and serves a minimal in-memory key store
// over RESP2: it parses each command (a `*N` array of `$len` bulk strings) and
// replies with the matching RESP type, exercising the client's encoder and its
// simple-string / integer / bulk / nil / array decoders on a real socket.
func fakeRedis(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	store := map[string]string{}
	readCmd := func() ([]string, bool) {
		header, err := r.ReadString('\n')
		if err != nil || len(header) == 0 || header[0] != '*' {
			return nil, false
		}
		n, err := strconv.Atoi(strings.TrimRight(header[1:], "\r\n"))
		if err != nil {
			return nil, false
		}
		args := make([]string, 0, n)
		for i := 0; i < n; i++ {
			if _, err := r.ReadString('\n'); err != nil { // $len line
				return nil, false
			}
			arg, err := r.ReadString('\n')
			if err != nil {
				return nil, false
			}
			args = append(args, strings.TrimRight(arg, "\r\n"))
		}
		return args, true
	}
	bulk := func(s string, ok bool) string {
		if !ok {
			return "$-1\r\n"
		}
		return fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
	}
	for {
		args, ok := readCmd()
		if !ok || len(args) == 0 {
			return
		}
		switch strings.ToUpper(args[0]) {
		case "AUTH", "SELECT":
			fmt.Fprint(conn, "+OK\r\n")
		case "PING":
			fmt.Fprint(conn, "+PONG\r\n")
		case "SET":
			store[args[1]] = args[2]
			fmt.Fprint(conn, "+OK\r\n")
		case "GET":
			v, present := store[args[1]]
			fmt.Fprint(conn, bulk(v, present))
		case "INCR":
			n, _ := strconv.Atoi(store[args[1]])
			n++
			store[args[1]] = strconv.Itoa(n)
			fmt.Fprintf(conn, ":%d\r\n", n)
		case "DEL":
			n := 0
			if _, present := store[args[1]]; present {
				n = 1
				delete(store, args[1])
			}
			fmt.Fprintf(conn, ":%d\r\n", n)
		case "EXISTS":
			n := 0
			if _, present := store[args[1]]; present {
				n = 1
			}
			fmt.Fprintf(conn, ":%d\r\n", n)
		case "KEYS":
			var b strings.Builder
			fmt.Fprintf(&b, "*%d\r\n", len(store))
			for k := range store {
				b.WriteString(bulk(k, true))
			}
			fmt.Fprint(conn, b.String())
		case "QUIT":
			fmt.Fprint(conn, "+OK\r\n")
			return
		default:
			fmt.Fprint(conn, "+OK\r\n")
		}
	}
}

// A .j program driving the redis client against an in-process RESP server
// asserts what it gets back (AUTH + SELECT on connect, PING, a SET/GET
// round-trip, INCR, EXISTS on present vs missing keys, KEYS count, DEL count);
// a mismatch throws and fails loadForTest. Runs the real net dialogue in CI
// with no Redis install.
func TestRedisCommands(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go fakeRedis(ln)

	redisMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "redis.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as redis;
def o as redis.Options init redis.Options{host: "127.0.0.1", port: %d, security: "none", user: "u", password: "p", db: 1};
def s as redis.Session init redis.connect($o);
testing.assertEqual(redis.ping($s), "PONG");
redis.set($s, "greeting", "hello");
testing.assertEqual(redis.get($s, "greeting"), "hello");
testing.assertEqual(redis.get($s, "missing"), "");
testing.assertEqual(redis.incr($s, "n"), 1);
testing.assertEqual(redis.incr($s, "n"), 2);
testing.assertTrue(redis.exists($s, "greeting"));
testing.assertFalse(redis.exists($s, "nope"));
testing.assertEqual(len(redis.keys($s, "*")), 2);
testing.assertEqual(redis.del($s, "greeting"), 1);
testing.assertEqual(redis.del($s, "greeting"), 0);
redis.quit($s);`, redisMod, port)
	progPath := filepath.Join(dir, "cmds.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("redis command program failed with code %d", code)
	}
}
