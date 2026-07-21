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

// fakeIMAP accepts one connection and serves a minimal IMAP4rev1 dialogue for a
// two-message INBOX, returning the FETCH body as a `{N}` literal (the byte count
// the client must read exactly) so literal handling is exercised on a real
// socket. It echoes the client's command tag in each tagged completion.
func fakeIMAP(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	fmt.Fprintf(conn, "* OK IMAP4rev1 ready\r\n")
	// A multi-byte UTF-8 body: the {N} literal count is a BYTE count, which
	// differs from the rune count here ("café"/"résumé"), so a rune-indexed
	// reader under-reads the literal and swallows the protocol trailer.
	msg := "Subject: Café\r\nFrom: alice@example.com\r\n\r\nthe café résumé body\r\n"
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		fields := strings.Fields(strings.TrimRight(line, "\r\n"))
		if len(fields) < 2 {
			continue
		}
		tag, cmd := fields[0], strings.ToUpper(fields[1])
		switch cmd {
		case "SELECT":
			fmt.Fprintf(conn, "* 2 EXISTS\r\n* 0 RECENT\r\n%s OK SELECT completed\r\n", tag)
		case "SEARCH":
			fmt.Fprintf(conn, "* SEARCH 1 2\r\n%s OK SEARCH completed\r\n", tag)
		case "FETCH":
			fmt.Fprintf(conn, "* 1 FETCH (BODY[] {%d}\r\n%s)\r\n%s OK FETCH completed\r\n",
				len(msg), msg, tag)
		case "LOGOUT":
			fmt.Fprintf(conn, "* BYE\r\n%s OK LOGOUT completed\r\n", tag)
			return
		default: // LOGIN, STARTTLS, ...
			fmt.Fprintf(conn, "%s OK %s completed\r\n", tag, cmd)
		}
	}
}

// A .j program driving the imap client against an in-process IMAP server
// asserts the message count, SEARCH numbers, and a FETCH body read out of a
// `{N}` literal; a mismatch fails loadForTest. Runs the real net dialogue
// (tagged responses + literals) in CI with no external server.
func TestImapReceive(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go fakeIMAP(ln)

	imapMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "imap.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as imap;
def o as imap.Options init imap.Options{host: "127.0.0.1", port: %d, security: "none", user: "u", pass: "p", auth: ""};
def s as imap.Session init imap.connect($o);
testing.assertEqual(imap.selectMailbox($s, "INBOX"), 2);
def nums as list of int init imap.search($s);
testing.assertEqual(len($nums), 2);
testing.assertEqual($nums[1], 2);
def body as string init imap.fetch($s, 1);
testing.assertContains($body, "Subject: Café");
testing.assertContains($body, "the café résumé body");
imap.logout($s);`, imapMod, port)
	progPath := filepath.Join(dir, "recv.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("imap receive program failed with code %d", code)
	}
}
