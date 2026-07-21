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
	"time"
)

// fakeSMTP accepts one connection and speaks a minimal SMTP dialogue, capturing
// the DATA payload. The EHLO reply is deliberately multi-line so the client's
// reply parser is exercised on a real socket, not just in unit tests.
func fakeSMTP(ln net.Listener, captured chan<- string) {
	conn, err := ln.Accept()
	if err != nil {
		captured <- "ACCEPT ERROR: " + err.Error()
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	fmt.Fprintf(conn, "220 fake ESMTP ready\r\n")
	var data strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			captured <- "READ ERROR: " + err.Error()
			return
		}
		up := strings.ToUpper(strings.TrimRight(line, "\r\n"))
		switch {
		case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
			fmt.Fprintf(conn, "250-fake greets you\r\n250-PIPELINING\r\n250 OK\r\n")
		case up == "DATA":
			fmt.Fprintf(conn, "354 end with <CRLF>.<CRLF>\r\n")
			for {
				dl, err := r.ReadString('\n')
				if err != nil {
					captured <- "DATA READ ERROR: " + err.Error()
					return
				}
				if dl == ".\r\n" {
					break
				}
				// undo dot-stuffing (a body line beginning with '.').
				if strings.HasPrefix(dl, ".") {
					dl = dl[1:]
				}
				data.WriteString(dl)
			}
			fmt.Fprintf(conn, "250 queued\r\n")
		case up == "QUIT":
			fmt.Fprintf(conn, "221 bye\r\n")
			captured <- data.String()
			return
		default: // MAIL FROM, RCPT TO, ...
			fmt.Fprintf(conn, "250 OK\r\n")
		}
	}
}

// A .j program driving smtp.send against an in-process SMTP server delivers the
// message intact: this exercises the real net dialogue (EHLO multi-line reply,
// MAIL FROM / RCPT TO / DATA / QUIT) in CI, with no external server.
func TestSmtpSendDialogue(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	captured := make(chan string, 1)
	go fakeSMTP(ln, captured)

	smtpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "smtp.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`import %q as smtp;
def o as smtp.Options init smtp.Options{host: "127.0.0.1", port: %d, security: "none", clientName: "t", user: "", pass: "", auth: "", allowInsecureAuth: true};
def rcpts as list of string init ["to@example.com"];
smtp.send($o, "from@example.com", $rcpts, "Subject: Hi\r\n\r\nthe body\r\n.hidden dotline");`, smtpMod, port)
	progPath := filepath.Join(dir, "send.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	_, code := loadForTest(progPath)
	if code != testExitPass {
		t.Fatalf("smtp.send program failed with code %d", code)
	}

	select {
	case got := <-captured:
		// Headers, body, and the un-stuffed leading-dot line all arrive.
		for _, want := range []string{"Subject: Hi", "the body", ".hidden dotline"} {
			if !strings.Contains(got, want) {
				t.Errorf("captured message missing %q; got:\n%s", want, got)
			}
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the SMTP server to capture the message")
	}
}
