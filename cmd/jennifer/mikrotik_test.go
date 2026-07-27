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
)

// --- RouterOS API broker-side wire helpers (stdlib only) --------------------

func rosEncodeLen(n int) []byte {
	switch {
	case n < 0x80:
		return []byte{byte(n)}
	case n < 0x4000:
		return []byte{byte(n>>8) | 0x80, byte(n)}
	case n < 0x200000:
		return []byte{byte(n>>16) | 0xc0, byte(n >> 8), byte(n)}
	case n < 0x10000000:
		return []byte{byte(n>>24) | 0xe0, byte(n >> 16), byte(n >> 8), byte(n)}
	default:
		return []byte{0xf0, byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
	}
}

func rosReadLen(r *bufio.Reader) (int, error) {
	b0, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	switch {
	case b0&0x80 == 0:
		return int(b0), nil
	case b0&0xc0 == 0x80:
		b1, err := r.ReadByte()
		return int(b0&0x3f)<<8 | int(b1), err
	case b0&0xe0 == 0xc0:
		buf := make([]byte, 2)
		_, err := io.ReadFull(r, buf)
		return int(b0&0x1f)<<16 | int(buf[0])<<8 | int(buf[1]), err
	case b0&0xf0 == 0xe0:
		buf := make([]byte, 3)
		_, err := io.ReadFull(r, buf)
		return int(b0&0x0f)<<24 | int(buf[0])<<16 | int(buf[1])<<8 | int(buf[2]), err
	default:
		buf := make([]byte, 4)
		_, err := io.ReadFull(r, buf)
		return int(buf[0])<<24 | int(buf[1])<<16 | int(buf[2])<<8 | int(buf[3]), err
	}
}

func rosReadSentence(r *bufio.Reader) ([]string, error) {
	var words []string
	for {
		n, err := rosReadLen(r)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return words, nil
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		words = append(words, string(buf))
	}
}

func rosWriteSentence(w io.Writer, words ...string) {
	for _, word := range words {
		w.Write(rosEncodeLen(len(word)))
		w.Write([]byte(word))
	}
	w.Write(rosEncodeLen(0))
}

// hasWord reports whether words contains an exact match for w.
func hasWord(words []string, w string) bool {
	for _, x := range words {
		if x == w {
			return true
		}
	}
	return false
}

// serveMikrotik answers a plaintext login, then handles print / add / unknown
// commands with canned reply sentences, exercising the client's login, talk,
// run, !trap, and query-word (talkQuery / printWhere) paths.
func serveMikrotik(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)

	login, err := rosReadSentence(r)
	if err != nil || len(login) == 0 || login[0] != "/login" {
		return
	}
	rosWriteSentence(conn, "!done") // modern plaintext login success (no challenge)

	for {
		cmd, err := rosReadSentence(r)
		if err != nil {
			return
		}
		if len(cmd) == 0 {
			continue
		}
		switch {
		case strings.Contains(cmd[0], "print"):
			// A query word filtering on running=true returns only the matching
			// row - proving query words reach the server via talkQuery / printWhere.
			rosWriteSentence(conn, "!re", "=name=ether1", "=type=ether", "=running=true")
			if !hasWord(cmd, "?running=true") {
				rosWriteSentence(conn, "!re", "=name=ether2", "=type=ether", "=running=false")
			}
			rosWriteSentence(conn, "!done")
		case strings.HasPrefix(cmd[0], "/ip/address/add"):
			rosWriteSentence(conn, "!done", "=ret=*5")
		default:
			rosWriteSentence(conn, "!trap", "=message=no such command")
			rosWriteSentence(conn, "!done")
		}
	}
}

// TestMikrotikTalk drives the mikrotik client through connect (login) / print /
// run / a !trap error against a fake RouterOS API server, exercising the word
// length codec, sentence framing, and reply parsing on the wire.
func TestMikrotikTalk(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go serveMikrotik(ln)

	addr := ln.Addr().(*net.TCPAddr)
	mtMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "mikrotik.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as mikrotik;
def s as mikrotik.Session init mikrotik.connect(mikrotik.withPort(mikrotik.options("127.0.0.1", "admin", "pw"), %d));

def rows as list of map of string to string init mikrotik.print($s, "/interface");
testing.assertEqual(len($rows), 2);
testing.assertEqual($rows[0]["name"], "ether1");
testing.assertEqual($rows[0]["running"], "true");
testing.assertEqual($rows[1]["running"], "false");

def filtered as list of map of string to string init mikrotik.printWhere($s, "/interface", ["?running=true"]);
testing.assertEqual(len($filtered), 1);
testing.assertEqual($filtered[0]["name"], "ether1");

def attrs as map of string to string init {};
$attrs["address"] = "10.0.0.1/24";
def newId as string init mikrotik.run($s, "/ip/address/add", $attrs);
testing.assertEqual($newId, "*5");

def threw as bool init false;
try {
    mikrotik.talk($s, "/bad/command", {});
} catch (e) {
    $threw = true;
    testing.assertEqual($e.kind, "mikrotik");
}
testing.assertTrue($threw);

mikrotik.close($s);`, mtMod, addr.Port)
	progPath := filepath.Join(dir, "mt.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("mikrotik program failed with code %d", code)
	}
}

// rosWordValue returns the value following the first word in words carrying the
// given prefix (e.g. ".tag=" or "=tag="), or "" when none is present.
func rosWordValue(words []string, prefix string) string {
	for _, w := range words {
		if strings.HasPrefix(w, prefix) {
			return w[len(prefix):]
		}
	}
	return ""
}

// rosHasTag reports whether any word is a `.tag=<id>` API tag word (the marker
// of a tagged / streaming command like /listen).
func rosHasTag(words []string) bool {
	for _, w := range words {
		if strings.HasPrefix(w, ".tag=") {
			return true
		}
	}
	return false
}

// serveMikrotikStream answers a plaintext login, then models one streaming
// command: a sentence carrying a `.tag=<id>` word (a `/listen`) is answered by
// pushing two tagged `!re` data sentences (no `!done` - the stream stays open),
// and a later `/cancel` naming that tag is answered with a `!trap` (category
// "interrupted") followed by a `!done`, both echoing the tag. This exercises the
// client's listen / receiveReply / cancel streaming path on the wire.
func serveMikrotikStream(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)

	login, err := rosReadSentence(r)
	if err != nil || len(login) == 0 || login[0] != "/login" {
		return
	}
	rosWriteSentence(conn, "!done") // modern plaintext login success

	for {
		cmd, err := rosReadSentence(r)
		if err != nil {
			return
		}
		if len(cmd) == 0 {
			continue
		}
		switch {
		case cmd[0] == "/cancel":
			// End the stream the way a real router does: an "interrupted"
			// trap for the streaming command, then its !done, both tagged.
			tagWord := ".tag=" + rosWordValue(cmd, "=tag=")
			rosWriteSentence(conn, "!trap", "=category=interrupted", "=message=interrupted", tagWord)
			rosWriteSentence(conn, "!done", tagWord)
		case rosHasTag(cmd):
			// A tagged streaming command: push two data replies, echoing the
			// tag on each so the client can correlate them.
			tagWord := ".tag=" + rosWordValue(cmd, ".tag=")
			rosWriteSentence(conn, "!re", "=name=ether1", "=rx=100", tagWord)
			rosWriteSentence(conn, "!re", "=name=ether1", "=rx=200", tagWord)
		default:
			rosWriteSentence(conn, "!trap", "=message=no such command")
			rosWriteSentence(conn, "!done")
		}
	}
}

// TestMikrotikListenStream drives the mikrotik client's streaming surface -
// listen (start a tagged streaming command) / receiveReply (pull each pushed
// !re for the tag) / cancel (stop it) - against a fake RouterOS API server that
// pushes two tagged data sentences and answers /cancel with an interrupted trap
// plus !done.
func TestMikrotikListenStream(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go serveMikrotikStream(ln)

	addr := ln.Addr().(*net.TCPAddr)
	mtMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "mikrotik.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as mikrotik;
def s as mikrotik.Session init mikrotik.connect(mikrotik.withPort(mikrotik.options("127.0.0.1", "admin", "pw"), %d));

def tag as string init mikrotik.listen($s, "/interface/monitor", {});
testing.assertTrue(len($tag) > 0);

def r1 as map of string to string init mikrotik.receiveReply($s, $tag);
testing.assertEqual($r1["name"], "ether1");
testing.assertEqual($r1["rx"], "100");

def r2 as map of string to string init mikrotik.receiveReply($s, $tag);
testing.assertEqual($r2["name"], "ether1");
testing.assertEqual($r2["rx"], "200");

mikrotik.cancel($s, $tag);

mikrotik.close($s);`, mtMod, addr.Port)
	progPath := filepath.Join(dir, "mtstream.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("mikrotik stream program failed with code %d", code)
	}
}
