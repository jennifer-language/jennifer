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

// fakeResqueRedis accepts one connection and serves the small RESP2 command set
// the resque module needs - SADD / SMEMBERS (the queue registry), RPUSH / LPOP
// (the queues and the failed list), and LLEN - over an in-memory store, so the
// module's enqueue / reserve / introspection path runs on a real socket in CI.
func fakeResqueRedis(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	lists := map[string][]string{}
	sets := map[string]map[string]bool{}
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
	bulk := func(s string) string { return fmt.Sprintf("$%d\r\n%s\r\n", len(s), s) }
	for {
		args, ok := readCmd()
		if !ok || len(args) == 0 {
			return
		}
		switch strings.ToUpper(args[0]) {
		case "SADD":
			if sets[args[1]] == nil {
				sets[args[1]] = map[string]bool{}
			}
			added := 0
			if !sets[args[1]][args[2]] {
				added = 1
				sets[args[1]][args[2]] = true
			}
			fmt.Fprintf(conn, ":%d\r\n", added)
		case "SMEMBERS":
			var b strings.Builder
			fmt.Fprintf(&b, "*%d\r\n", len(sets[args[1]]))
			for m := range sets[args[1]] {
				b.WriteString(bulk(m))
			}
			fmt.Fprint(conn, b.String())
		case "RPUSH":
			lists[args[1]] = append(lists[args[1]], args[2])
			fmt.Fprintf(conn, ":%d\r\n", len(lists[args[1]]))
		case "LPOP":
			l := lists[args[1]]
			if len(l) == 0 {
				fmt.Fprint(conn, "$-1\r\n")
			} else {
				fmt.Fprint(conn, bulk(l[0]))
				lists[args[1]] = l[1:]
			}
		case "LLEN":
			fmt.Fprintf(conn, ":%d\r\n", len(lists[args[1]]))
		case "QUIT":
			fmt.Fprint(conn, "+OK\r\n")
			return
		default:
			fmt.Fprint(conn, "+OK\r\n")
		}
	}
}

// A .j program driving the resque module against the in-process server asserts
// the full producer / consumer path: enqueue registers queues and pushes
// envelopes, introspection counts them, reserve pops in priority order and
// decodes args, fail lands on the failed list, and a drained queue yields an
// empty Job. A mismatch throws and fails loadForTest.
func TestResqueJobs(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go fakeResqueRedis(ln)

	resqueMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "resque.j"))
	if err != nil {
		t.Fatal(err)
	}
	redisMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "redis.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as resque;
import %q as redis;
def db as redis.Session init redis.connect(redis.Options{host: "127.0.0.1", port: %d, security: "none", user: "", password: "", db: 0});
resque.enqueue($db, "email", "SendWelcome", ["user@example.com", "en"]);
resque.enqueue($db, "email", "SendReceipt", ["order-42"]);
resque.enqueue($db, "high", "Ping", []);
testing.assertEqual(len(resque.queues($db)), 2);
testing.assertEqual(resque.size($db), 3);
testing.assertEqual(resque.queueLength($db, "email"), 2);
def first as resque.Job init resque.reserve($db, ["high", "email"]);
testing.assertEqual($first.queue, "high");
testing.assertEqual($first.class, "Ping");
testing.assertEqual(len($first.args), 0);
def second as resque.Job init resque.reserve($db, ["high", "email"]);
testing.assertEqual($second.queue, "email");
testing.assertEqual($second.class, "SendWelcome");
testing.assertEqual($second.args[0], "user@example.com");
testing.assertEqual($second.args[1], "en");
resque.fail($db, $second, "boom");
testing.assertEqual(resque.size($db), 1);
def drained as resque.Job init resque.reserve($db, ["high"]);
testing.assertEqual(len($drained.class), 0);
redis.quit($db);`, resqueMod, redisMod, port)
	progPath := filepath.Join(dir, "jobs.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("resque job program failed with code %d", code)
	}
}
