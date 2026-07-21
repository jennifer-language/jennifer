// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// xoauth2SASL is the reference SASL XOAUTH2 initial response, computed
// independently of the Jennifer `sasl` module.
func xoauth2SASL(user, token string) string {
	raw := "user=" + user + "\x01auth=Bearer " + token + "\x01\x01"
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

const xoUser = "me@gmail.com"
const xoToken = "ya29.A0ExampleAccessToken"

// runXoauth2Test runs a .j program against an in-process server (`serve`) that
// captures the base64 the client sends after `prefix`, and asserts it equals
// the reference XOAUTH2 response. This verifies each mail client wires
// sasl.bearer into the right auth command.
func runXoauth2Test(t *testing.T, module, prefix, prog string, serve func(net.Conn, chan<- string)) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	captured := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		serve(conn, captured)
	}()

	mod, err := filepath.Abs(filepath.Join("..", "..", "modules", module))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	full := fmt.Sprintf(prog, mod, ln.Addr().(*net.TCPAddr).Port)
	path := filepath.Join(dir, "xo.j")
	if err := os.WriteFile(path, []byte(full), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(path); code != testExitPass {
		t.Fatalf("%s xoauth2 program failed with code %d", module, code)
	}

	select {
	case got := <-captured:
		want := prefix + xoauth2SASL(xoUser, xoToken)
		if got != want {
			t.Errorf("auth line = %q, want %q", got, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the auth command")
	}
}

func TestSmtpXoauth2(t *testing.T) {
	serve := func(conn net.Conn, captured chan<- string) {
		r := bufio.NewReader(conn)
		fmt.Fprintf(conn, "220 ready\r\n")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			trimmed := strings.TrimRight(line, "\r\n")
			up := strings.ToUpper(trimmed)
			switch {
			case strings.HasPrefix(trimmed, "AUTH XOAUTH2 "):
				captured <- trimmed
				fmt.Fprintf(conn, "235 accepted\r\n")
			case strings.HasPrefix(up, "EHLO"):
				fmt.Fprintf(conn, "250 ok\r\n")
			case up == "DATA":
				fmt.Fprintf(conn, "354 go\r\n")
				for {
					dl, err := r.ReadString('\n')
					if err != nil || dl == ".\r\n" {
						break
					}
				}
				fmt.Fprintf(conn, "250 queued\r\n")
			case up == "QUIT":
				fmt.Fprintf(conn, "221 bye\r\n")
				return
			default:
				fmt.Fprintf(conn, "250 ok\r\n")
			}
		}
	}
	prog := `import %q as smtp;
def o as smtp.Options init smtp.Options{host: "127.0.0.1", port: %d, security: "none", clientName: "t", user: "me@gmail.com", pass: "ya29.A0ExampleAccessToken", auth: "xoauth2", allowInsecureAuth: true};
smtp.send($o, "me@gmail.com", ["you@example.com"], "Subject: Hi\r\n\r\nbody");`
	runXoauth2Test(t, "smtp.j", "AUTH XOAUTH2 ", prog, serve)
}

func TestPopXoauth2(t *testing.T) {
	serve := func(conn net.Conn, captured chan<- string) {
		r := bufio.NewReader(conn)
		fmt.Fprintf(conn, "+OK ready\r\n")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			trimmed := strings.TrimRight(line, "\r\n")
			switch {
			case strings.HasPrefix(trimmed, "AUTH XOAUTH2 "):
				captured <- trimmed
				fmt.Fprintf(conn, "+OK\r\n")
			case strings.ToUpper(trimmed) == "STAT":
				fmt.Fprintf(conn, "+OK 0 0\r\n")
			case strings.ToUpper(trimmed) == "QUIT":
				fmt.Fprintf(conn, "+OK bye\r\n")
				return
			default:
				fmt.Fprintf(conn, "+OK\r\n")
			}
		}
	}
	prog := `import %q as pop;
def o as pop.Options init pop.Options{host: "127.0.0.1", port: %d, security: "none", user: "me@gmail.com", pass: "ya29.A0ExampleAccessToken", auth: "xoauth2"};
def s as pop.Session init pop.connect($o);
def n as int init pop.count($s);
pop.quit($s);`
	runXoauth2Test(t, "pop.j", "AUTH XOAUTH2 ", prog, serve)
}

func TestImapXoauth2(t *testing.T) {
	serve := func(conn net.Conn, captured chan<- string) {
		r := bufio.NewReader(conn)
		fmt.Fprintf(conn, "* OK ready\r\n")
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
			switch {
			case cmd == "AUTHENTICATE" && len(fields) >= 4:
				// "TAG AUTHENTICATE XOAUTH2 <b64>"
				captured <- "AUTHENTICATE XOAUTH2 " + fields[3]
				fmt.Fprintf(conn, "%s OK authenticated\r\n", tag)
			case cmd == "SELECT":
				fmt.Fprintf(conn, "* 0 EXISTS\r\n%s OK done\r\n", tag)
			case cmd == "LOGOUT":
				fmt.Fprintf(conn, "* BYE\r\n%s OK done\r\n", tag)
				return
			default:
				fmt.Fprintf(conn, "%s OK done\r\n", tag)
			}
		}
	}
	prog := `import %q as imap;
def o as imap.Options init imap.Options{host: "127.0.0.1", port: %d, security: "none", user: "me@gmail.com", pass: "ya29.A0ExampleAccessToken", auth: "xoauth2"};
def s as imap.Session init imap.connect($o);
def n as int init imap.selectMailbox($s, "INBOX");
imap.logout($s);`
	runXoauth2Test(t, "imap.j", "AUTHENTICATE XOAUTH2 ", prog, serve)
}
