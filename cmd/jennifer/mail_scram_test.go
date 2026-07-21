// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/pbkdf2"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	gohash "hash"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests stand up an in-process mail server that speaks the real
// server-side of CRAM-MD5 and SCRAM, computed here independently of the
// Jennifer `sasl` module, and run each client against it. A client that
// authenticates (the program exits 0) proves it wired the mechanism correctly:
// the SCRAM server verifies the client's proof, and the client verifies the
// server's signature. The auto (`auth: "auto"`) case additionally checks that
// the client picked the strongest advertised mechanism: the servers advertise a
// full set but reject the weak fallback, so success means SCRAM was chosen, and
// the chosen-mechanism channel pins it to SCRAM-SHA-256.

const mailUser = "user"
const mailPass = "pencil"

// advertised is the mechanism set the servers offer to an auto-negotiating
// client (strongest is SCRAM-SHA-256).
var advertised = []string{"PLAIN", "LOGIN", "CRAM-MD5", "SCRAM-SHA-1", "SCRAM-SHA-256"}

// wireHash picks the hash from a SCRAM mechanism name on the wire.
func wireHash(s string) func() gohash.Hash {
	if strings.Contains(strings.ToUpper(s), "SCRAM-SHA-256") {
		return sha256.New
	}
	return sha1.New
}

// wireMech normalises a SCRAM mechanism name for the chosen-mechanism channel.
func wireMech(s string) string {
	if strings.Contains(strings.ToUpper(s), "SCRAM-SHA-256") {
		return "SCRAM-SHA-256"
	}
	return "SCRAM-SHA-1"
}

func hmacSum(nh func() gohash.Hash, key, msg []byte) []byte {
	m := hmac.New(nh, key)
	m.Write(msg)
	return m.Sum(nil)
}

func plainSum(nh func() gohash.Hash, data []byte) []byte {
	h := nh()
	h.Write(data)
	return h.Sum(nil)
}

func xorBytes(a, b []byte) []byte {
	out := make([]byte, len(a))
	for i := range a {
		out[i] = a[i] ^ b[i]
	}
	return out
}

func saslAttr(msg, key string) string {
	for _, p := range strings.Split(msg, ",") {
		if strings.HasPrefix(p, key+"=") {
			return p[len(key)+1:]
		}
	}
	return ""
}

// scramSrv is the server half of one SCRAM exchange.
type scramSrv struct {
	nh          func() gohash.Hash
	bare        string
	serverFirst string
	salt        []byte
	iters       int
}

func startScram(nh func() gohash.Hash, clientFirstB64 string) (*scramSrv, string, error) {
	cf, err := base64.StdEncoding.DecodeString(clientFirstB64)
	if err != nil {
		return nil, "", err
	}
	s := string(cf)
	if !strings.HasPrefix(s, "n,,") {
		return nil, "", fmt.Errorf("client-first missing gs2 header: %q", s)
	}
	bare := s[3:]
	cnonce := saslAttr(bare, "r")
	if cnonce == "" {
		return nil, "", fmt.Errorf("client-first has no nonce")
	}
	salt := []byte("0123456789abcdef")
	iters := 4096
	serverFirst := fmt.Sprintf("r=%sSRVNONCE,s=%s,i=%d", cnonce, base64.StdEncoding.EncodeToString(salt), iters)
	return &scramSrv{nh: nh, bare: bare, serverFirst: serverFirst, salt: salt, iters: iters}, serverFirst, nil
}

// finish verifies the client-final proof and returns the server-final (v=...).
func (s *scramSrv) finish(clientFinalB64 string) (string, error) {
	cf, err := base64.StdEncoding.DecodeString(clientFinalB64)
	if err != nil {
		return "", err
	}
	cfStr := string(cf)
	pIdx := strings.Index(cfStr, ",p=")
	if pIdx < 0 {
		return "", fmt.Errorf("client-final has no proof: %q", cfStr)
	}
	finalNoProof := cfStr[:pIdx]
	proofB64 := cfStr[pIdx+3:]
	authMessage := s.bare + "," + s.serverFirst + "," + finalNoProof
	salted, err := pbkdf2.Key(s.nh, mailPass, s.salt, s.iters, s.nh().Size())
	if err != nil {
		return "", err
	}
	clientKey := hmacSum(s.nh, salted, []byte("Client Key"))
	storedKey := plainSum(s.nh, clientKey)
	clientSig := hmacSum(s.nh, storedKey, []byte(authMessage))
	proof, err := base64.StdEncoding.DecodeString(proofB64)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(plainSum(s.nh, xorBytes(proof, clientSig)), storedKey) {
		return "", fmt.Errorf("client proof did not verify")
	}
	serverKey := hmacSum(s.nh, salted, []byte("Server Key"))
	serverSig := hmacSum(s.nh, serverKey, []byte(authMessage))
	return "v=" + base64.StdEncoding.EncodeToString(serverSig), nil
}

// cramExpected is the reference CRAM-MD5 response for a raw challenge.
func cramExpected(rawChallenge string) string {
	m := hmac.New(md5.New, []byte(mailPass))
	m.Write([]byte(rawChallenge))
	return base64.StdEncoding.EncodeToString([]byte(mailUser + " " + hex.EncodeToString(m.Sum(nil))))
}

const cramRawChallenge = "<12345.67890@server.example>"

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func readLine(r *bufio.Reader) string {
	line, _ := r.ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}

// note records the mechanism the client chose (non-blocking, for the auto case).
func note(ch chan<- string, mech string) {
	if ch == nil {
		return
	}
	select {
	case ch <- mech:
	default:
	}
}

// runMailAuthTest runs a .j program against `serve` and asserts it exits 0.
func runMailAuthTest(t *testing.T, module, prog string, serve func(*testing.T, net.Conn)) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		serve(t, conn)
	}()

	mod, err := filepath.Abs(filepath.Join("..", "..", "modules", module))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.j")
	full := fmt.Sprintf(prog, mod, ln.Addr().(*net.TCPAddr).Port)
	if err := os.WriteFile(path, []byte(full), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(path); code != testExitPass {
		t.Fatalf("%s auth program failed with code %d", module, code)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("server did not finish")
	}
}

// ---- SMTP server (334 continuations) --------------------------------------

func smtpScram(t *testing.T, conn net.Conn, r *bufio.Reader, authLine string) bool {
	cf := authLine[strings.LastIndex(authLine, " ")+1:]
	srv, sf, err := startScram(wireHash(authLine), cf)
	if err != nil {
		t.Errorf("smtp scram start: %v", err)
		fmt.Fprintf(conn, "535 no\r\n")
		return false
	}
	fmt.Fprintf(conn, "334 %s\r\n", b64(sf))
	serverFinal, err := srv.finish(readLine(r))
	if err != nil {
		t.Errorf("smtp scram: %v", err)
		fmt.Fprintf(conn, "535 auth failed\r\n")
		return false
	}
	fmt.Fprintf(conn, "334 %s\r\n", b64(serverFinal))
	if ack := readLine(r); ack != "=" {
		t.Errorf("smtp scram: expected \"=\" ack, got %q", ack)
		fmt.Fprintf(conn, "535 no\r\n")
		return false
	}
	fmt.Fprintf(conn, "235 ok\r\n")
	return true
}

// smtpServe reacts to whatever mechanism the client uses. It advertises the
// full set on EHLO and rejects PLAIN, so an auto client that fell back instead
// of choosing SCRAM would fail. `chosen` records the mechanism (auto case).
func smtpServe(chosen chan<- string) func(*testing.T, net.Conn) {
	return func(t *testing.T, conn net.Conn) {
		r := bufio.NewReader(conn)
		fmt.Fprintf(conn, "220 ready\r\n")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			l := strings.TrimRight(line, "\r\n")
			up := strings.ToUpper(l)
			switch {
			case strings.HasPrefix(up, "EHLO"):
				fmt.Fprintf(conn, "250-hi\r\n250 AUTH %s\r\n", strings.Join(advertised, " "))
			case up == "AUTH CRAM-MD5":
				note(chosen, "CRAM-MD5")
				fmt.Fprintf(conn, "334 %s\r\n", b64(cramRawChallenge))
				if got := readLine(r); got != cramExpected(cramRawChallenge) {
					t.Errorf("smtp CRAM response = %q, want %q", got, cramExpected(cramRawChallenge))
					fmt.Fprintf(conn, "535 auth failed\r\n")
					return
				}
				fmt.Fprintf(conn, "235 ok\r\n")
			case strings.HasPrefix(up, "AUTH SCRAM-"):
				note(chosen, wireMech(l))
				if !smtpScram(t, conn, r, l) {
					return
				}
			case strings.HasPrefix(up, "AUTH PLAIN"), strings.HasPrefix(up, "AUTH LOGIN"):
				note(chosen, "PLAIN/LOGIN")
				fmt.Fprintf(conn, "535 weak mechanism refused\r\n")
			case up == "QUIT":
				fmt.Fprintf(conn, "221 bye\r\n")
				return
			case up == "DATA":
				fmt.Fprintf(conn, "354 go\r\n")
				for {
					dl, derr := r.ReadString('\n')
					if derr != nil || dl == ".\r\n" {
						break
					}
				}
				fmt.Fprintf(conn, "250 queued\r\n")
			default:
				fmt.Fprintf(conn, "250 ok\r\n")
			}
		}
	}
}

const smtpAuthProg = `import %q as smtp;
def o as smtp.Options init smtp.Options{host: "127.0.0.1", port: %d, security: "none", clientName: "t", user: "user", pass: "pencil", auth: "MECHNAME", allowInsecureAuth: true};
smtp.send($o, "user@example.com", ["you@example.com"], "Subject: Hi\r\n\r\nbody");`

func TestSmtpSaslMechanisms(t *testing.T) {
	for _, mech := range []string{"cram", "scram-sha-1", "scram-sha-256"} {
		t.Run(mech, func(t *testing.T) {
			prog := strings.ReplaceAll(smtpAuthProg, "MECHNAME", mech)
			runMailAuthTest(t, "smtp.j", prog, smtpServe(nil))
		})
	}
	t.Run("auto", func(t *testing.T) {
		chosen := make(chan string, 1)
		prog := strings.ReplaceAll(smtpAuthProg, "MECHNAME", "auto")
		runMailAuthTest(t, "smtp.j", prog, smtpServe(chosen))
		if got := <-chosen; got != "SCRAM-SHA-256" {
			t.Errorf("auto-selected %q, want SCRAM-SHA-256", got)
		}
	})
}

// ---- POP3 server ("+ " continuations, initial-response SCRAM) -------------

func popScram(t *testing.T, conn net.Conn, r *bufio.Reader, authLine string) bool {
	cf := authLine[strings.LastIndex(authLine, " ")+1:]
	srv, sf, err := startScram(wireHash(authLine), cf)
	if err != nil {
		t.Errorf("pop scram start: %v", err)
		fmt.Fprintf(conn, "-ERR no\r\n")
		return false
	}
	fmt.Fprintf(conn, "+ %s\r\n", b64(sf))
	serverFinal, err := srv.finish(readLine(r))
	if err != nil {
		t.Errorf("pop scram: %v", err)
		fmt.Fprintf(conn, "-ERR auth failed\r\n")
		return false
	}
	fmt.Fprintf(conn, "+ %s\r\n", b64(serverFinal))
	if ack := readLine(r); ack != "" {
		t.Errorf("pop scram: expected empty ack, got %q", ack)
		fmt.Fprintf(conn, "-ERR no\r\n")
		return false
	}
	fmt.Fprintf(conn, "+OK authenticated\r\n")
	return true
}

// popServe advertises mechanisms via CAPA and rejects USER (the fallback).
func popServe(chosen chan<- string) func(*testing.T, net.Conn) {
	return func(t *testing.T, conn net.Conn) {
		r := bufio.NewReader(conn)
		fmt.Fprintf(conn, "+OK ready\r\n")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			l := strings.TrimRight(line, "\r\n")
			up := strings.ToUpper(l)
			switch {
			case up == "CAPA":
				fmt.Fprintf(conn, "+OK caps\r\nSASL %s\r\n.\r\n", strings.Join(advertised, " "))
			case up == "AUTH CRAM-MD5":
				note(chosen, "CRAM-MD5")
				fmt.Fprintf(conn, "+ %s\r\n", b64(cramRawChallenge))
				if got := readLine(r); got != cramExpected(cramRawChallenge) {
					t.Errorf("pop CRAM response = %q, want %q", got, cramExpected(cramRawChallenge))
					fmt.Fprintf(conn, "-ERR auth failed\r\n")
					return
				}
				fmt.Fprintf(conn, "+OK authenticated\r\n")
			case strings.HasPrefix(up, "AUTH SCRAM-"):
				note(chosen, wireMech(l))
				if !popScram(t, conn, r, l) {
					return
				}
			case strings.HasPrefix(up, "USER "):
				note(chosen, "USER")
				fmt.Fprintf(conn, "-ERR USER/PASS refused\r\n")
			case up == "STAT":
				fmt.Fprintf(conn, "+OK 0 0\r\n")
			case up == "QUIT":
				fmt.Fprintf(conn, "+OK bye\r\n")
				return
			default:
				fmt.Fprintf(conn, "+OK\r\n")
			}
		}
	}
}

const popAuthProg = `import %q as pop;
def o as pop.Options init pop.Options{host: "127.0.0.1", port: %d, security: "none", user: "user", pass: "pencil", auth: "MECHNAME"};
def s as pop.Session init pop.connect($o);
def n as int init pop.count($s);
pop.quit($s);`

func TestPopSaslMechanisms(t *testing.T) {
	for _, mech := range []string{"cram", "scram-sha-1", "scram-sha-256"} {
		t.Run(mech, func(t *testing.T) {
			prog := strings.ReplaceAll(popAuthProg, "MECHNAME", mech)
			runMailAuthTest(t, "pop.j", prog, popServe(nil))
		})
	}
	t.Run("auto", func(t *testing.T) {
		chosen := make(chan string, 1)
		prog := strings.ReplaceAll(popAuthProg, "MECHNAME", "auto")
		runMailAuthTest(t, "pop.j", prog, popServe(chosen))
		if got := <-chosen; got != "SCRAM-SHA-256" {
			t.Errorf("auto-selected %q, want SCRAM-SHA-256", got)
		}
	})
}

// ---- IMAP server ("+ " continuations, non-initial-response SCRAM) ---------

func imapScram(t *testing.T, conn net.Conn, r *bufio.Reader, mechName, tag string) bool {
	fmt.Fprintf(conn, "+ \r\n")
	srv, sf, err := startScram(wireHash(mechName), readLine(r))
	if err != nil {
		t.Errorf("imap scram start: %v", err)
		fmt.Fprintf(conn, "%s NO no\r\n", tag)
		return false
	}
	fmt.Fprintf(conn, "+ %s\r\n", b64(sf))
	serverFinal, err := srv.finish(readLine(r))
	if err != nil {
		t.Errorf("imap scram: %v", err)
		fmt.Fprintf(conn, "%s NO auth failed\r\n", tag)
		return false
	}
	fmt.Fprintf(conn, "+ %s\r\n", b64(serverFinal))
	if ack := readLine(r); ack != "" {
		t.Errorf("imap scram: expected empty ack, got %q", ack)
		fmt.Fprintf(conn, "%s NO no\r\n", tag)
		return false
	}
	fmt.Fprintf(conn, "%s OK authenticated\r\n", tag)
	return true
}

// imapServe advertises AUTH= mechanisms via CAPABILITY and rejects LOGIN.
func imapServe(chosen chan<- string) func(*testing.T, net.Conn) {
	return func(t *testing.T, conn net.Conn) {
		r := bufio.NewReader(conn)
		fmt.Fprintf(conn, "* OK ready\r\n")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			l := strings.TrimRight(line, "\r\n")
			fields := strings.Fields(l)
			if len(fields) < 2 {
				continue
			}
			tag, cmd := fields[0], strings.ToUpper(fields[1])
			switch {
			case cmd == "CAPABILITY":
				caps := "* CAPABILITY IMAP4rev1"
				for _, m := range advertised {
					if m == "PLAIN" || m == "LOGIN" {
						continue
					}
					caps += " AUTH=" + m
				}
				fmt.Fprintf(conn, "%s\r\n%s OK done\r\n", caps, tag)
			case cmd == "AUTHENTICATE" && len(fields) >= 3 && strings.ToUpper(fields[2]) == "CRAM-MD5":
				note(chosen, "CRAM-MD5")
				fmt.Fprintf(conn, "+ %s\r\n", b64(cramRawChallenge))
				if got := readLine(r); got != cramExpected(cramRawChallenge) {
					t.Errorf("imap CRAM response = %q, want %q", got, cramExpected(cramRawChallenge))
					fmt.Fprintf(conn, "%s NO auth failed\r\n", tag)
					return
				}
				fmt.Fprintf(conn, "%s OK authenticated\r\n", tag)
			case cmd == "AUTHENTICATE" && len(fields) >= 3:
				note(chosen, wireMech(fields[2]))
				if !imapScram(t, conn, r, fields[2], tag) {
					return
				}
			case cmd == "LOGIN":
				note(chosen, "LOGIN")
				fmt.Fprintf(conn, "%s NO LOGIN refused\r\n", tag)
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
}

const imapAuthProg = `import %q as imap;
def o as imap.Options init imap.Options{host: "127.0.0.1", port: %d, security: "none", user: "user", pass: "pencil", auth: "MECHNAME"};
def s as imap.Session init imap.connect($o);
def n as int init imap.selectMailbox($s, "INBOX");
imap.logout($s);`

func TestImapSaslMechanisms(t *testing.T) {
	for _, mech := range []string{"cram", "scram-sha-1", "scram-sha-256"} {
		t.Run(mech, func(t *testing.T) {
			prog := strings.ReplaceAll(imapAuthProg, "MECHNAME", mech)
			runMailAuthTest(t, "imap.j", prog, imapServe(nil))
		})
	}
	t.Run("auto", func(t *testing.T) {
		chosen := make(chan string, 1)
		prog := strings.ReplaceAll(imapAuthProg, "MECHNAME", "auto")
		runMailAuthTest(t, "imap.j", prog, imapServe(chosen))
		if got := <-chosen; got != "SCRAM-SHA-256" {
			t.Errorf("auto-selected %q, want SCRAM-SHA-256", got)
		}
	})
}

// ---- POP3 APOP ------------------------------------------------------------

// TestPopApop covers POP3 APOP: the server greets with a timestamp, and the
// client proves the shared secret with MD5(timestamp+password). Explicit
// (auth: "apop") and auto (CAPA offers no SASL, so auto falls to APOP).
func TestPopApop(t *testing.T) {
	const timestamp = "<1896.697170952@server.example>"
	want := fmt.Sprintf("%x", md5.Sum([]byte(timestamp+mailPass)))
	serve := func(t *testing.T, conn net.Conn) {
		r := bufio.NewReader(conn)
		fmt.Fprintf(conn, "+OK POP3 ready %s\r\n", timestamp)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			l := strings.TrimRight(line, "\r\n")
			up := strings.ToUpper(l)
			switch {
			case up == "CAPA":
				// No SASL advertised, so auto falls back to APOP.
				fmt.Fprintf(conn, "+OK caps\r\nUSER\r\n.\r\n")
			case strings.HasPrefix(l, "APOP "):
				fields := strings.Fields(l)
				if len(fields) != 3 || fields[1] != mailUser || fields[2] != want {
					t.Errorf("APOP line = %q, want \"APOP %s %s\"", l, mailUser, want)
					fmt.Fprintf(conn, "-ERR auth failed\r\n")
					return
				}
				fmt.Fprintf(conn, "+OK maildrop ready\r\n")
			case up == "STAT":
				fmt.Fprintf(conn, "+OK 0 0\r\n")
			case up == "QUIT":
				fmt.Fprintf(conn, "+OK bye\r\n")
				return
			default:
				fmt.Fprintf(conn, "+OK\r\n")
			}
		}
	}
	for _, mech := range []string{"apop", "auto"} {
		t.Run(mech, func(t *testing.T) {
			prog := strings.ReplaceAll(popAuthProg, "MECHNAME", mech)
			runMailAuthTest(t, "pop.j", prog, serve)
		})
	}
}
