// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A fake MAC-Telnet server (legacy MD5 path) on a loopback UDP socket, enough to
// drive the module's full state machine end to end: session start -> begin auth
// -> 16-byte salt -> MD5 password verify -> end-of-auth banner -> command echo.
// It speaks the same wire format as a RouterOS device: client packets carry the
// session key at offset 14, the server's replies put it at offset 16.

const (
	mtPtypeSessionStart = 0
	mtPtypeData         = 1
	mtPtypeAck          = 2
	mtPtypeEnd          = 255

	mtCpBeginAuth = 0
	mtCpPassSalt  = 1
	mtCpPassword  = 2
	mtCpEndAuth   = 9
)

// srvHeader builds a 22-byte server->client header (session key at offset 16,
// client type at 14 - the mirror of the client layout).
func srvHeader(ptype byte, seskey uint16, counter uint32) []byte {
	h := make([]byte, 22)
	h[0] = 1
	h[1] = ptype
	h[14] = 0x00
	h[15] = 0x15
	binary.BigEndian.PutUint16(h[16:18], seskey)
	binary.BigEndian.PutUint32(h[18:22], counter)
	return h
}

// mtCtrl frames one control block: magic, type, 4-byte big-endian length, data.
func mtCtrl(cptype byte, data []byte) []byte {
	b := []byte{0x56, 0x34, 0x12, 0xff, cptype}
	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(len(data)))
	b = append(b, l[:]...)
	return append(b, data...)
}

// mtControls scans a DATA payload for the control blocks the server cares about.
func mtControls(payload []byte) (beginAuth bool, password []byte, hasPassword bool) {
	off := 0
	magic := []byte{0x56, 0x34, 0x12, 0xff}
	for off < len(payload) {
		if off+9 <= len(payload) && bytes.Equal(payload[off:off+4], magic) {
			ctype := payload[off+4]
			clen := int(binary.BigEndian.Uint32(payload[off+5 : off+9]))
			if off+9+clen > len(payload) {
				clen = len(payload) - off - 9
			}
			data := payload[off+9 : off+9+clen]
			if ctype == mtCpBeginAuth {
				beginAuth = true
			}
			if ctype == mtCpPassword {
				password = append([]byte(nil), data...)
				hasPassword = true
			}
			off += 9 + clen
		} else {
			break // trailing plain data
		}
	}
	return beginAuth, password, hasPassword
}

// serveFakeMactelnet answers one MAC-Telnet session over conn using the MD5 auth
// path, verifying the password against want. It returns after an END packet or a
// read timeout.
func serveFakeMactelnet(conn net.PacketConn, wantPassword string) {
	salt := []byte("mactelnet16bsalt") // exactly 16 bytes -> selects MD5 auth
	var serverOut uint32
	buf := make([]byte, 2048)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		if n < 22 {
			continue
		}
		pkt := append([]byte(nil), buf[:n]...)
		ptype := pkt[1]
		seskey := binary.BigEndian.Uint16(pkt[14:16]) // client layout
		counter := binary.BigEndian.Uint32(pkt[18:22])

		switch ptype {
		case mtPtypeSessionStart:
			_, _ = conn.WriteTo(srvHeader(mtPtypeAck, seskey, 0), addr)
		case mtPtypeData:
			payload := pkt[22:]
			// Always acknowledge a DATA packet.
			_, _ = conn.WriteTo(srvHeader(mtPtypeAck, seskey, counter+uint32(len(payload))), addr)

			beginAuth, password, hasPassword := mtControls(payload)
			var body []byte
			switch {
			case beginAuth:
				body = mtCtrl(mtCpPassSalt, salt)
			case hasPassword:
				sum := md5.Sum(append(append([]byte{0}, []byte(wantPassword)...), salt...))
				expect := append([]byte{0}, sum[:]...)
				if !bytes.Equal(password, expect) {
					// Failure: a plaindata message, then end the session (no
					// end-of-auth), the way RouterOS refuses a bad login.
					fail := append(srvHeader(mtPtypeData, seskey, serverOut), []byte("Login failed\r\n")...)
					_, _ = conn.WriteTo(fail, addr)
					serverOut += uint32(len("Login failed\r\n"))
					_, _ = conn.WriteTo(srvHeader(mtPtypeEnd, seskey, 0), addr)
					continue
				}
				body = append(mtCtrl(mtCpEndAuth, nil), []byte("logged in\r\n")...)
			default:
				body = []byte("OK\r\n") // echo any console command
			}
			reply := append(srvHeader(mtPtypeData, seskey, serverOut), body...)
			_, _ = conn.WriteTo(reply, addr)
			serverOut += uint32(len(body))
		case mtPtypeEnd:
			return
		}
	}
}

// TestMactelnetSession drives connect -> recv (banner) -> send -> recv (echo) ->
// close through modules/mactelnet.j against the fake MD5 server, over loopback
// (JENNIFER_MACTELNET_TARGET redirects the module's broadcast to the server).
func TestMactelnetSession(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	go serveFakeMactelnet(pc, "test")

	target := pc.LocalAddr().String()
	t.Setenv("JENNIFER_MACTELNET_TARGET", target)

	mod, err := filepath.Abs(filepath.Join("..", "..", "modules", "mactelnet.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
use strings;
import %q as mactelnet;
def s as mactelnet.Session init mactelnet.connect("lo", "aa:bb:cc:dd:ee:ff", "admin", "test");
def banner as string init mactelnet.recv($s, 800);
testing.assertTrue(strings.contains($banner, "logged in"));
mactelnet.send($s, "/ip address print\r\n");
def out as string init mactelnet.recv($s, 800);
testing.assertTrue(strings.contains($out, "OK"));
mactelnet.close($s);`, mod)
	progPath := filepath.Join(dir, "mt.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("mactelnet program failed with code %d", code)
	}
}

// TestMactelnetWrongPasswordFails - a bad password never reaches the logged-in
// banner, so connect fails (the server answers with a login-failed message and
// no end-of-auth).
func TestMactelnetWrongPasswordFails(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	go serveFakeMactelnet(pc, "correct-horse")

	t.Setenv("JENNIFER_MACTELNET_TARGET", pc.LocalAddr().String())
	mod, err := filepath.Abs(filepath.Join("..", "..", "modules", "mactelnet.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`use testing;
import %q as mactelnet;
def threw as bool init false;
try {
    def s as mactelnet.Session init mactelnet.connect("lo", "aa:bb:cc:dd:ee:ff", "admin", "wrong");
    mactelnet.close($s);
} catch (e) {
    $threw = true;
    testing.assertEqual($e.kind, "mactelnet");
}
testing.assertTrue($threw);`, mod)
	progPath := filepath.Join(dir, "mt.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("mactelnet wrong-password program failed with code %d", code)
	}
}
