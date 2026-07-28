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

// fakeIMAPUid serves the UID-addressed dialogue (UID SEARCH / FETCH / STORE /
// COPY / MOVE, plus a ranged FETCH and native MOVE) and records every command
// line it received, so the test can assert the exact wire form the client sent.
// It dispatches on the command content (not fields[1]) so a "UID FETCH ..." is
// routed by its FETCH nature, not the leading "UID" token.
func fakeIMAPUid(ln net.Listener, captured chan<- []string) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	fmt.Fprintf(conn, "* OK IMAP4rev1 ready\r\n")
	var cmds []string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			captured <- cmds
			return
		}
		trimmed := strings.TrimRight(line, "\r\n")
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		cmds = append(cmds, trimmed)
		tag := fields[0]
		up := strings.ToUpper(trimmed)
		switch {
		case strings.Contains(up, "SELECT"):
			fmt.Fprintf(conn, "* 2 EXISTS\r\n* 0 RECENT\r\n%s OK SELECT completed\r\n", tag)
		case strings.Contains(up, "SEARCH"):
			// UID SEARCH reports stable UIDs, not sequence numbers.
			fmt.Fprintf(conn, "* SEARCH 101 102\r\n%s OK SEARCH completed\r\n", tag)
		case strings.Contains(up, "FETCH") && strings.Contains(up, "(FLAGS)"):
			fmt.Fprintf(conn, "* 1 FETCH (UID 101 FLAGS (\\Seen $cl_1))\r\n%s OK FETCH completed\r\n", tag)
		case strings.Contains(up, "FETCH") && strings.Contains(up, "HEADER.FIELDS"):
			hdr := "Subject: Hi\r\n\r\n"
			fmt.Fprintf(conn, "* 1 FETCH (UID 101 BODY[HEADER.FIELDS (SUBJECT)] {%d}\r\n%s)\r\n%s OK FETCH completed\r\n", len(hdr), hdr, tag)
		case strings.Contains(up, "FETCH") && strings.Contains(up, "<"):
			part := "01234"
			fmt.Fprintf(conn, "* 1 FETCH (UID 101 BODY[]<0> {%d}\r\n%s)\r\n%s OK FETCH completed\r\n", len(part), part, tag)
		case strings.Contains(up, "FETCH"):
			body := "Subject: Full\r\nFrom: a@x\r\n\r\nthe uid body\r\n"
			fmt.Fprintf(conn, "* 1 FETCH (UID 101 BODY[] {%d}\r\n%s)\r\n%s OK FETCH completed\r\n", len(body), body, tag)
		case strings.Contains(up, "STORE"), strings.Contains(up, "COPY"), strings.Contains(up, "MOVE"):
			fmt.Fprintf(conn, "%s OK completed\r\n", tag)
		case strings.Contains(up, "LOGOUT"):
			fmt.Fprintf(conn, "* BYE\r\n%s OK LOGOUT completed\r\n", tag)
			captured <- cmds
			return
		default: // LOGIN, ...
			fmt.Fprintf(conn, "%s OK completed\r\n", tag)
		}
	}
}

// A .j program exercises the UID-addressed verbs and asserts both the parsed
// results (UID SEARCH numbers, a UID-fetched body, a ranged fetch, UID flags)
// and, via the captured command log, the exact wire commands (UID FETCH / STORE /
// COPY / MOVE and the native seq MOVE + ranged FETCH).
func TestImapUidVerbs(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	captured := make(chan []string, 1)
	go fakeIMAPUid(ln, captured)

	imapMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "imap.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as imap;
def o as imap.Options init imap.Options{host: "127.0.0.1", port: %d, security: "none", user: "u", pass: "p", auth: ""};
def s as imap.Session init imap.connect($o);
testing.assertEqual(imap.selectFolder($s, "INBOX"), 2);

# UID SEARCH -> stable UIDs
def uids as list of int init imap.uidSearch($s, imap.criteria());
testing.assertEqual(len($uids), 2);
testing.assertEqual($uids[0], 101);
testing.assertEqual($uids[1], 102);

# UID FETCH -> body by stable id
def body as string init imap.uidFetch($s, 101);
testing.assertContains($body, "the uid body");

# ranged FETCH (partial body)
def part as string init imap.fetchPartial($s, 1, 0, 5);
testing.assertEqual($part, "01234");

# UID FETCH headers + flags
def hdrs as string init imap.uidFetchHeaders($s, 101, "SUBJECT");
testing.assertContains($hdrs, "Subject: Hi");
def fl as string init imap.uidFlags($s, 101);
testing.assertContains($fl, "\\Seen");

# UID STORE / COPY / MOVE and native seq MOVE (no throw = OK)
imap.uidAddFlags($s, 101, "\\Deleted");
imap.uidRemoveFlags($s, 101, "$cl_1");
imap.uidCopy($s, 101, "Archive");
imap.uidMove($s, 102, "Archive");
imap.move($s, 1, "Archive");
imap.logout($s);`, imapMod, port)
	progPath := filepath.Join(dir, "uid.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("imap uid program failed with code %d", code)
	}

	got := <-captured
	joined := strings.Join(got, "\n")
	for _, want := range []string{
		"UID SEARCH",
		"UID FETCH 101 BODY.PEEK[]",
		"FETCH 1 BODY.PEEK[]<0.5>",
		"UID FETCH 101 BODY.PEEK[HEADER.FIELDS (SUBJECT)]",
		"UID FETCH 101 (FLAGS)",
		"UID STORE 101 +FLAGS.SILENT (\\Deleted)",
		"UID STORE 101 -FLAGS.SILENT ($cl_1)",
		"UID COPY 101 \"Archive\"",
		"UID MOVE 102 \"Archive\"",
		"MOVE 1 \"Archive\"",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected the client to send %q; commands were:\n%s", want, joined)
		}
	}
}
