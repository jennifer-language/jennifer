// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

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

// fakeRedisKv serves the RESP2 commands the kvstore redis backend uses -
// SET (with a trailing EX/NX ignored), GET, DEL, EXPIRE, and INCR (create-at-1) -
// so a session round-trip and a ratelimit window both run over the redis
// dispatch. Values here are base64 / integer text (no binary), so a line-framed
// bulk reader suffices.
func fakeRedisKv(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	kv := map[string]string{}
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
	for {
		args, ok := readCmd()
		if !ok || len(args) == 0 {
			return
		}
		switch strings.ToUpper(args[0]) {
		case "AUTH", "SELECT", "EXPIRE":
			// EXPIRE: report success when the key exists.
			if strings.ToUpper(args[0]) == "EXPIRE" {
				n := 0
				if _, present := kv[args[1]]; present {
					n = 1
				}
				fmt.Fprintf(conn, ":%d\r\n", n)
			} else {
				fmt.Fprint(conn, "+OK\r\n")
			}
		case "SET":
			kv[args[1]] = args[2] // trailing EX / NX ignored
			fmt.Fprint(conn, "+OK\r\n")
		case "GET":
			if v, present := kv[args[1]]; present {
				fmt.Fprintf(conn, "$%d\r\n%s\r\n", len(v), v)
			} else {
				fmt.Fprint(conn, "$-1\r\n")
			}
		case "DEL":
			n := 0
			if _, present := kv[args[1]]; present {
				delete(kv, args[1])
				n = 1
			}
			fmt.Fprintf(conn, ":%d\r\n", n)
		case "INCR":
			cur, _ := strconv.Atoi(kv[args[1]]) // "" -> 0
			cur++
			kv[args[1]] = strconv.Itoa(cur)
			fmt.Fprintf(conn, ":%d\r\n", cur)
		case "QUIT":
			fmt.Fprint(conn, "+OK\r\n")
			return
		default:
			fmt.Fprint(conn, "+OK\r\n")
		}
	}
}

// A .j program drives session and ratelimit over the redis backend (via the
// kvstore selector) against the fake, proving the redis dispatch (SET EX / GET /
// DEL / EXPIRE / INCR): a session round-trips a json.Value, and a fixed-window
// limiter allows up to the limit then denies.
func TestKvstoreRedisBackend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go fakeRedisKv(ln)

	abs := func(name string) string {
		p, e := filepath.Abs(filepath.Join("..", "..", "modules", name))
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use json;
import %q as session;
import %q as ratelimit;
import %q as kvstore;
import %q as redis;
import %q as transport;
def rc as redis.Session init redis.connect(redis.Options{host: "127.0.0.1", port: %d, security: transport.Security.None, user: "", password: "", db: 0});
def st as kvstore.Store init kvstore.redisStore($rc);

# session over redis
def id as string init session.create($st, 60);
def d as json.Value init json.set(json.map(), "/user", "ada");
session.save($st, $id, $d, 60);
testing.assertEqual(json.asString(session.load($st, $id), "/user"), "ada");
testing.assertTrue(session.touch($st, $id, 120));
testing.assertTrue(session.destroy($st, $id));

# ratelimit over redis (INCR + EXPIRE)
def lim as ratelimit.Limiter init ratelimit.fixedWindow($st, 2, 3600);
def a as ratelimit.Result init ratelimit.check($lim, "ip:a");
testing.assertTrue($a.allowed);
def b as ratelimit.Result init ratelimit.check($lim, "ip:a");
testing.assertTrue($b.allowed);
def c as ratelimit.Result init ratelimit.check($lim, "ip:a");
testing.assertFalse($c.allowed);
redis.quit($rc);`, abs("session.j"), abs("ratelimit.j"), abs("kvstore.j"), abs("redis.j"), abs("transport.j"), port)
	progPath := filepath.Join(dir, "redisbackend.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("kvstore redis-backend program failed with code %d", code)
	}
}
