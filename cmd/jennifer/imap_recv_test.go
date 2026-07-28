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
	"strings"
	"testing"
	"time"
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
		// The client addresses messages by UID, so real commands are "UID FETCH
		// ...", "UID SEARCH ...": dispatch on the subcommand after the UID prefix.
		if cmd == "UID" && len(fields) >= 3 {
			cmd = strings.ToUpper(fields[2])
		}
		switch cmd {
		case "SELECT":
			fmt.Fprintf(conn, "* 2 EXISTS\r\n* 0 RECENT\r\n%s OK SELECT completed\r\n", tag)
		case "SEARCH":
			fmt.Fprintf(conn, "* SEARCH 1 2\r\n%s OK SEARCH completed\r\n", tag)
		case "FETCH":
			if strings.Contains(strings.ToUpper(line), "INTERNALDATE") {
				// Both messages report the same arrival instant, for the
				// sub-day since/before client-refinement test.
				fmt.Fprintf(conn, "* 1 FETCH (INTERNALDATE \"15-Jan-2026 12:00:00 +0000\")\r\n%s OK FETCH completed\r\n", tag)
			} else {
				fmt.Fprintf(conn, "* 1 FETCH (BODY[] {%d}\r\n%s)\r\n%s OK FETCH completed\r\n",
					len(msg), msg, tag)
			}
		case "LIST":
			fmt.Fprintf(conn, "* LIST (\\HasNoChildren) \"/\" \"INBOX\"\r\n"+
				"* LIST (\\HasChildren) \"/\" \"Archive\"\r\n%s OK LIST completed\r\n", tag)
		case "STATUS":
			fmt.Fprintf(conn, "* STATUS \"INBOX\" (MESSAGES 2 RECENT 0 UNSEEN 1 UIDNEXT 3 UIDVALIDITY 42)\r\n%s OK STATUS completed\r\n", tag)
		case "APPEND":
			// Literal-continuation flow: parse {N}, request the literal, consume
			// exactly N bytes plus its trailing CRLF, then complete.
			n := 0
			if i := strings.LastIndex(line, "{"); i >= 0 {
				fmt.Sscanf(line[i:], "{%d}", &n)
			}
			fmt.Fprintf(conn, "+ OK send the message\r\n")
			if n > 0 {
				_, _ = io.CopyN(io.Discard, r, int64(n))
			}
			_, _ = r.ReadString('\n') // the CRLF after the literal
			fmt.Fprintf(conn, "%s OK [APPENDUID 42 3] APPEND completed\r\n", tag)
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
use time;
use lists;
import %q as imap;
def o as imap.Options init imap.Options{host: "127.0.0.1", port: %d, security: "none", user: "u", pass: "p", auth: ""};
def s as imap.Session init imap.connect($o);
testing.assertEqual(imap.selectFolder($s, "INBOX"), 2);
def nums as list of int init imap.search($s, imap.criteria());
testing.assertEqual(len($nums), 2);
testing.assertEqual($nums[1], 2);
# Client-side regex filter over the candidates: the fetched header's Subject is
# "Café", so a matching pattern keeps both and a non-matching one drops all.
def hit as imap.Criteria init imap.criteria();
$hit.subjectRegex = "Caf";
testing.assertEqual(len(imap.search($s, $hit)), 2);
def miss as imap.Criteria init imap.criteria();
$miss.subjectRegex = "ZZZ-no-match";
testing.assertEqual(len(imap.search($s, $miss)), 0);
# Sub-day since: the day-granular server search returns candidates, then the
# client refines by INTERNALDATE (the mock's messages arrived 2026-01-15T12:00Z).
def early as imap.Criteria init imap.criteria();
$early.since = time.fromIso("2026-01-15T10:00:00Z");   # before arrival -> both kept
testing.assertEqual(len(imap.search($s, $early)), 2);
def late as imap.Criteria init imap.criteria();
$late.since = time.fromIso("2026-01-15T14:00:00Z");    # after arrival -> both dropped
testing.assertEqual(len(imap.search($s, $late)), 0);
# LIST: enumerate folders (name / delimiter / flags).
def boxes as list of imap.Folder init imap.folders($s, "*");
testing.assertEqual(len($boxes), 2);
testing.assertEqual($boxes[0].name, "INBOX");
testing.assertEqual($boxes[0].delimiter, "/");
testing.assertEqual($boxes[1].name, "Archive");
testing.assertTrue(lists.contains($boxes[1].flags, "\\HasChildren"));
# STATUS: counts without selecting.
def st as imap.Status init imap.status($s, "INBOX");
testing.assertEqual($st.messages, 2);
testing.assertEqual($st.unseen, 1);
testing.assertEqual($st.uidvalidity, 42);
# APPEND: upload a message (the literal-continuation flow); no throw = OK.
imap.append($s, "Sent", "Subject: hi\r\nFrom: me@x\r\n\r\nbody\r\n");
imap.appendWith($s, "Drafts", "\\Draft", "Subject: draft\r\n\r\nunfinished\r\n");
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

// fakeIMAPIdle serves the login / SELECT / CAPABILITY prelude, then the RFC 2177
// IDLE dialogue: on `<tag> IDLE` it answers a `+ idling` continuation and then
// pushes two untagged notifications (`* 5 EXISTS`, `* 1 EXPUNGE`); on the bare
// `DONE` line (which carries no tag) it answers the fixed-tag IDLE completion.
// The continuation and each push go out as separate writes with a short gap so
// the client's line reader (which discards anything past the first CRLF in a
// chunk) sees each on its own read, exercising the real push framing.
func fakeIMAPIdle(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	fmt.Fprintf(conn, "* OK IMAP4rev1 ready\r\n")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		trimmed := strings.TrimRight(line, "\r\n")
		// DONE is a bare, untagged line that ends the IDLE command; answer with
		// the fixed command tag the module uses (imap.j's `TAG`).
		if strings.ToUpper(trimmed) == "DONE" {
			fmt.Fprintf(conn, "JEN OK IDLE terminated\r\n")
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		tag, cmd := fields[0], strings.ToUpper(fields[1])
		switch cmd {
		case "SELECT":
			fmt.Fprintf(conn, "* 2 EXISTS\r\n* 0 RECENT\r\n%s OK SELECT completed\r\n", tag)
		case "CAPABILITY":
			fmt.Fprintf(conn, "* CAPABILITY IMAP4rev1 IDLE\r\n%s OK CAPABILITY completed\r\n", tag)
		case "IDLE":
			// Enter IDLE, then push mailbox changes as distinct writes so each
			// arrives on its own client read.
			fmt.Fprintf(conn, "+ idling\r\n")
			time.Sleep(100 * time.Millisecond)
			fmt.Fprintf(conn, "* 5 EXISTS\r\n")
			time.Sleep(100 * time.Millisecond)
			fmt.Fprintf(conn, "* 1 EXPUNGE\r\n")
		case "LOGOUT":
			fmt.Fprintf(conn, "* BYE\r\n%s OK LOGOUT completed\r\n", tag)
			return
		default: // LOGIN, ...
			fmt.Fprintf(conn, "%s OK %s completed\r\n", tag, cmd)
		}
	}
}

// A .j program drives the imap IDLE surface against the in-process server:
// gate on supportsIdle, enter idle, read the EXISTS push via pollNotification
// (a bounded wait that lands inside its window) and the EXPUNGE push via a
// blocking receiveNotification, then leave IDLE cleanly with done. Assertions
// run through `testing.assert*`, so any mismatch fails loadForTest.
func TestImapIdle(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go fakeIMAPIdle(ln)

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
# CAPABILITY gate: the server advertises IDLE.
testing.assertTrue(imap.supportsIdle($s));
# Enter IDLE and read the server's pushes as typed notifications.
imap.idle($s);
# A bounded poll returns the first push (EXISTS 5) once it arrives in-window.
def a as imap.Notification init imap.pollNotification($s, 2000);
testing.assertEqual($a.kind, "exists");
testing.assertEqual($a.number, 5);
# A blocking receive returns the next push (EXPUNGE 1).
def b as imap.Notification init imap.receiveNotification($s);
testing.assertEqual($b.kind, "expunge");
testing.assertEqual($b.number, 1);
# Leave IDLE cleanly and log out.
imap.done($s);
imap.logout($s);`, imapMod, port)
	progPath := filepath.Join(dir, "idle.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("imap idle program failed with code %d", code)
	}
}
