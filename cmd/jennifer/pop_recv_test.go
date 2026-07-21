// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakePOP3 accepts one connection and serves a fixed two-message mailbox over a
// minimal POP3 dialogue. Message 2's body carries a byte-stuffed leading-dot
// line ("..dotted"), so the client's un-stuffing is exercised on a real socket.
func fakePOP3(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	fmt.Fprintf(conn, "+OK POP3 server ready\r\n")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		up := strings.ToUpper(strings.TrimRight(line, "\r\n"))
		switch {
		case up == "STAT":
			fmt.Fprintf(conn, "+OK 2 34\r\n")
		case up == "LIST":
			fmt.Fprintf(conn, "+OK 2 messages\r\n1 20\r\n2 14\r\n.\r\n")
		case up == "RETR 1":
			fmt.Fprintf(conn, "+OK 20 octets\r\nSubject: One\r\n\r\nbody one\r\n.\r\n")
		case up == "RETR 2":
			// ".dotted" byte-stuffed to "..dotted" on the wire.
			fmt.Fprintf(conn, "+OK 14 octets\r\nSubject: Two\r\n\r\n..dotted\r\n.\r\n")
		case up == "RETR 3":
			// A multi-byte UTF-8 body. Reading it as per-chunk strings (or
			// counting runes) corrupts the accented characters.
			fmt.Fprintf(conn, "+OK 30 octets\r\nSubject: Trois\r\n\r\ncafé résumé\r\n.\r\n")
		case up == "QUIT":
			fmt.Fprintf(conn, "+OK bye\r\n")
			return
		default: // USER, PASS, DELE, NOOP, ...
			fmt.Fprintf(conn, "+OK\r\n")
		}
	}
}

// A .j program driving the pop client against an in-process POP3 server
// asserts what it receives (message count, bodies, un-stuffed dot line, LIST
// sizes); a mismatch throws and fails loadForTest. Runs the real net dialogue
// in CI with no external server.
func TestPop3Receive(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go fakePOP3(ln)

	popMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "pop.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as pop;
def o as pop.Options init pop.Options{host: "127.0.0.1", port: %d, security: "none", user: "u", pass: "p", auth: ""};
def s as pop.Session init pop.connect($o);
testing.assertEqual(pop.count($s), 2);
def msgA as string init pop.retrieve($s, 1);
testing.assertContains($msgA, "Subject: One");
testing.assertContains($msgA, "body one");
def msgB as string init pop.retrieve($s, 2);
testing.assertContains($msgB, ".dotted");
def msgC as string init pop.retrieve($s, 3);
testing.assertContains($msgC, "café résumé");
def szs as list of int init pop.sizes($s);
testing.assertEqual(len($szs), 2);
testing.assertEqual($szs[0], 20);
pop.quit($s);`, popMod, port)
	progPath := filepath.Join(dir, "recv.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("pop receive program failed with code %d", code)
	}
}
