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

// fakeRedisTyped serves RESP2 with **byte-count** framing (unlike fakeRedis,
// which reads bulk args as CRLF-terminated lines and so cannot carry a binary
// value). It reads each argument by its `$len`, so a SET value containing NUL /
// CR / LF round-trips, and it implements the hash / list / set commands the
// typed helpers use, keeping every value as raw bytes.
func fakeRedisTyped(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	kv := map[string][]byte{}
	hashes := map[string]map[string][]byte{}
	lists := map[string][][]byte{}
	sets := map[string]map[string]bool{}

	readCmd := func() ([][]byte, bool) {
		header, err := r.ReadString('\n')
		if err != nil || len(header) == 0 || header[0] != '*' {
			return nil, false
		}
		n, err := strconv.Atoi(strings.TrimRight(header[1:], "\r\n"))
		if err != nil {
			return nil, false
		}
		args := make([][]byte, 0, n)
		for i := 0; i < n; i++ {
			lenLine, err := r.ReadString('\n') // $len\r\n
			if err != nil || len(lenLine) == 0 || lenLine[0] != '$' {
				return nil, false
			}
			m, err := strconv.Atoi(strings.TrimRight(lenLine[1:], "\r\n"))
			if err != nil {
				return nil, false
			}
			buf := make([]byte, m+2) // value + trailing CRLF, read by count
			if _, err := io.ReadFull(r, buf); err != nil {
				return nil, false
			}
			args = append(args, buf[:m])
		}
		return args, true
	}
	bulk := func(b []byte, present bool) string {
		if !present {
			return "$-1\r\n"
		}
		return fmt.Sprintf("$%d\r\n%s\r\n", len(b), b)
	}
	array := func(items [][]byte) string {
		var sb strings.Builder
		fmt.Fprintf(&sb, "*%d\r\n", len(items))
		for _, it := range items {
			sb.WriteString(bulk(it, true))
		}
		return sb.String()
	}
	for {
		args, ok := readCmd()
		if !ok || len(args) == 0 {
			return
		}
		s := func(i int) string { return string(args[i]) }
		switch strings.ToUpper(s(0)) {
		case "AUTH", "SELECT":
			fmt.Fprint(conn, "+OK\r\n")
		case "SET":
			kv[s(1)] = args[2]
			fmt.Fprint(conn, "+OK\r\n")
		case "GET":
			v, present := kv[s(1)]
			fmt.Fprint(conn, bulk(v, present))
		case "HSET":
			if hashes[s(1)] == nil {
				hashes[s(1)] = map[string][]byte{}
			}
			_, existed := hashes[s(1)][s(2)]
			hashes[s(1)][s(2)] = args[3]
			n := 1
			if existed {
				n = 0
			}
			fmt.Fprintf(conn, ":%d\r\n", n)
		case "HGET":
			v, present := hashes[s(1)][s(2)]
			fmt.Fprint(conn, bulk(v, present))
		case "HGETALL":
			var items [][]byte
			for k, v := range hashes[s(1)] {
				items = append(items, []byte(k), v)
			}
			fmt.Fprint(conn, array(items))
		case "HDEL":
			n := 0
			if _, present := hashes[s(1)][s(2)]; present {
				delete(hashes[s(1)], s(2))
				n = 1
			}
			fmt.Fprintf(conn, ":%d\r\n", n)
		case "RPUSH":
			lists[s(1)] = append(lists[s(1)], args[2])
			fmt.Fprintf(conn, ":%d\r\n", len(lists[s(1)]))
		case "LPUSH":
			lists[s(1)] = append([][]byte{args[2]}, lists[s(1)]...)
			fmt.Fprintf(conn, ":%d\r\n", len(lists[s(1)]))
		case "LLEN":
			fmt.Fprintf(conn, ":%d\r\n", len(lists[s(1)]))
		case "LRANGE":
			l := lists[s(1)]
			lo, _ := strconv.Atoi(s(2))
			hi, _ := strconv.Atoi(s(3))
			if hi < 0 {
				hi = len(l) + hi
			}
			var items [][]byte
			for i := lo; i <= hi && i < len(l); i++ {
				items = append(items, l[i])
			}
			fmt.Fprint(conn, array(items))
		case "LPOP":
			l := lists[s(1)]
			if len(l) == 0 {
				fmt.Fprint(conn, "$-1\r\n")
			} else {
				fmt.Fprint(conn, bulk(l[0], true))
				lists[s(1)] = l[1:]
			}
		case "SADD":
			if sets[s(1)] == nil {
				sets[s(1)] = map[string]bool{}
			}
			n := 0
			if !sets[s(1)][s(2)] {
				sets[s(1)][s(2)] = true
				n = 1
			}
			fmt.Fprintf(conn, ":%d\r\n", n)
		case "SREM":
			n := 0
			if sets[s(1)][s(2)] {
				delete(sets[s(1)], s(2))
				n = 1
			}
			fmt.Fprintf(conn, ":%d\r\n", n)
		case "SMEMBERS":
			var items [][]byte
			for m := range sets[s(1)] {
				items = append(items, []byte(m))
			}
			fmt.Fprint(conn, array(items))
		case "SISMEMBER":
			n := 0
			if sets[s(1)][s(2)] {
				n = 1
			}
			fmt.Fprintf(conn, ":%d\r\n", n)
		case "SCARD":
			fmt.Fprintf(conn, ":%d\r\n", len(sets[s(1)]))
		case "QUIT":
			fmt.Fprint(conn, "+OK\r\n")
			return
		default:
			fmt.Fprint(conn, "+OK\r\n")
		}
	}
}

// A .j program drives the byte-exact get/set and the typed hash / list / set
// helpers against the byte-framing mock: a binary value (NUL / CR / LF / 0xFF)
// round-trips through setBytes/getBytes, and the hash / list / set verbs return
// the expected shapes.
func TestRedisBinaryAndTyped(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go fakeRedisTyped(ln)

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

# byte-exact round-trip: a value with NUL, CR, LF, 0xFF
def blob as bytes;
$blob[] = 0;
$blob[] = 13;
$blob[] = 10;
$blob[] = 255;
$blob[] = 65;
redis.setBytes($s, "bin", $blob);
def got as bytes init redis.getBytes($s, "bin");
testing.assertEqual(len($got), 5);
testing.assertEqual($got[0], 0);
testing.assertEqual($got[1], 13);
testing.assertEqual($got[2], 10);
testing.assertEqual($got[3], 255);
testing.assertEqual($got[4], 65);
testing.assertEqual(len(redis.getBytes($s, "nope")), 0);

# hash
redis.hset($s, "h", "f1", "v1");
redis.hset($s, "h", "f2", "v2");
testing.assertEqual(redis.hget($s, "h", "f1"), "v1");
def all as map of string to string init redis.hgetAll($s, "h");
testing.assertEqual(len($all), 2);
testing.assertEqual($all["f2"], "v2");
testing.assertEqual(redis.hdel($s, "h", "f1"), 1);

# list
testing.assertEqual(redis.rpush($s, "l", "a"), 1);
testing.assertEqual(redis.rpush($s, "l", "b"), 2);
testing.assertEqual(redis.lpush($s, "l", "z"), 3);
testing.assertEqual(redis.llen($s, "l"), 3);
def items as list of string init redis.lrange($s, "l", 0, -1);
testing.assertEqual(len($items), 3);
testing.assertEqual($items[0], "z");
testing.assertEqual(redis.lpop($s, "l"), "z");

# set
testing.assertEqual(redis.sadd($s, "st", "m1"), 1);
redis.sadd($s, "st", "m2");
testing.assertEqual(redis.scard($s, "st"), 2);
testing.assertTrue(redis.sismember($s, "st", "m1"));
testing.assertFalse(redis.sismember($s, "st", "zzz"));
testing.assertEqual(len(redis.smembers($s, "st")), 2);
testing.assertEqual(redis.srem($s, "st", "m1"), 1);
redis.quit($s);`, redisMod, filepath.Join(filepath.Dir(redisMod), "transport.j"), port)
	progPath := filepath.Join(dir, "binary.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("redis binary/typed program failed with code %d", code)
	}
}
