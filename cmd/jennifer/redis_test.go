// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
import %q as transport;
def o as redis.Options init redis.Options{host: "127.0.0.1", port: %d, security: transport.Security.None, user: "u", password: "p", db: 1};
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
redis.quit($s);`, redisMod, filepath.Join(filepath.Dir(redisMod), "transport.j"), port)
	progPath := filepath.Join(dir, "cmds.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("redis command program failed with code %d", code)
	}
}

// fakeRedisPubSub accepts one connection and scripts the RESP2 reply shapes the
// redis module's newer verbs decode: a pub/sub push (preceded by its SUBSCRIBE
// confirmation, so the client exercises its confirmation-draining path), a
// two-command pipeline, and a SCAN `[cursor, [keys]]` reply. It is a permissive
// mock (it does not enforce Redis's subscribed-mode command restriction). It
// deliberately sends coalesced frames (the subscribe confirmation + the pushed
// message in one write, and back-to-back pipeline replies) so the test exercises
// the client's buffered reader, which must parse every reply out of one read
// rather than dropping the leftover.
func fakeRedisPubSub(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
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
	bulk := func(s string) string {
		return fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
	}
	for {
		args, ok := readCmd()
		if !ok || len(args) == 0 {
			return
		}
		switch strings.ToUpper(args[0]) {
		case "SUBSCRIBE":
			ch := args[1]
			// Send the subscribe confirmation (`*3 subscribe <chan> 1`) and the
			// pushed message COALESCED in a single write, so the test exercises the
			// client's buffered coalescing fix: a per-reply reader would drop the
			// message with the leftover here and then block.
			fmt.Fprintf(conn, "*3\r\n%s%s:1\r\n*3\r\n%s%s%s",
				bulk("subscribe"), bulk(ch),
				bulk("message"), bulk(ch), bulk("hello-payload"))
		case "ECHO":
			// One reply per pipelined command, written back-to-back with no pause
			// so the replies coalesce - the client's buffered pipeline read must
			// parse both, not drop the second.
			fmt.Fprint(conn, bulk(args[1]))
		case "SCAN":
			// A SCAN page: the next cursor (a bulk string) then the key array.
			fmt.Fprintf(conn, "*2\r\n%s*2\r\n%s%s", bulk("42"), bulk("alpha"), bulk("beta"))
		case "QUIT":
			fmt.Fprint(conn, "+OK\r\n")
			return
		default:
			fmt.Fprint(conn, "+OK\r\n")
		}
	}
}

// A .j program drives the redis module's M23.1 verbs against the scripted mock:
// it subscribes and reads a pushed message (asserting receiveMessage drains the
// SUBSCRIBE confirmation and returns the channel / payload), pipelines two
// commands (asserting both replies come back in order), and scans (asserting the
// cursor and keys parse from a `[cursor, [keys]]` reply). Each assertion is a
// testing.assert* inside the program, so a mismatch throws and fails loadForTest.
func TestRedisPubSubAndScan(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go fakeRedisPubSub(ln)

	redisMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "redis.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as redis;
import %q as transport;
def o as redis.Options init redis.Options{host: "127.0.0.1", port: %d, security: transport.Security.None, user: "", password: "", db: 0};
def s as redis.Session init redis.connect($o);

redis.subscribe($s, ["chan1"]);
def m as redis.Message init redis.receiveMessage($s);
testing.assertEqual($m.kind, "message");
testing.assertEqual($m.channel, "chan1");
testing.assertEqual($m.payload, "hello-payload");

def replies as list of redis.Reply init redis.pipeline($s, [["ECHO", "one"], ["ECHO", "two"]]);
testing.assertEqual(len($replies), 2);
testing.assertEqual($replies[0].str, "one");
testing.assertEqual($replies[1].str, "two");

def sr as redis.ScanResult init redis.scan($s, 0, "*", 10);
testing.assertEqual($sr.cursor, 42);
testing.assertEqual(len($sr.keys), 2);
testing.assertEqual($sr.keys[0], "alpha");
testing.assertEqual($sr.keys[1], "beta");

redis.quit($s);`, redisMod, filepath.Join(filepath.Dir(redisMod), "transport.j"), port)
	progPath := filepath.Join(dir, "pubsub.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("redis pub/sub program failed with code %d", code)
	}
}
