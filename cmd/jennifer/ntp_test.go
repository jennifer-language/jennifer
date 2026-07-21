// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ntpTimestamp encodes a Unix second as an 8-byte NTP timestamp (seconds since
// 1900, zero fraction).
func ntpTimestamp(unixSec int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint32(b[0:4], uint32(unixSec+2208988800))
	return b
}

// TestNtpQuery drives ntp.queryWith against a fake UDP NTP server that answers
// one request with a fixed transmit time, and checks the module decodes it.
func TestNtpQuery(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	addr := pc.LocalAddr().String()
	const serverUnix = int64(1700000000)

	go func() {
		buf := make([]byte, 48)
		n, peer, rerr := pc.ReadFrom(buf)
		if rerr != nil || n < 48 {
			return
		}
		resp := make([]byte, 48)
		resp[0] = 0x24                              // LI=0, VN=4, Mode=4 (server)
		resp[1] = 1                                 // stratum 1
		copy(resp[24:32], buf[40:48])               // originate = client's transmit
		copy(resp[32:40], ntpTimestamp(serverUnix)) // receive
		copy(resp[40:48], ntpTimestamp(serverUnix)) // transmit
		_, _ = pc.WriteTo(resp, peer)
	}()

	ntpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "ntp.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`import %q as ntp;
use time;
use testing;
def const ADDR as string init %q;

func testQuery() {
    def r as ntp.Result init ntp.queryWith(ADDR, 2000);
    testing.assertEqual(time.unix($r.serverTime), 1700000000);
}
`, ntpMod, addr)
	progPath := filepath.Join(dir, "prog.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	in, code := loadForTest(progPath)
	if in == nil || code != testExitPass {
		t.Fatalf("loadForTest failed: code %d", code)
	}
	if _, err := in.CallByName("testQuery"); err != nil {
		t.Errorf("testQuery failed: %v", err)
	}
}

// TestNtpTimeout confirms a query against a silent server throws (kind "ntp")
// rather than hanging, thanks to the UDP receive deadline.
func TestNtpTimeout(t *testing.T) {
	// A bound-but-silent UDP socket: it never replies.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	addr := pc.LocalAddr().String()

	ntpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "ntp.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`import %q as ntp;
use testing;
def const ADDR as string init %q;

func testTimeout() {
    def threw as bool init false;
    try {
        ntp.queryWith(ADDR, 300);
    } catch (e) {
        $threw = true;
    }
    testing.assertTrue($threw);
}
`, ntpMod, addr)
	progPath := filepath.Join(dir, "prog.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		in, code := loadForTest(progPath)
		if in == nil || code != testExitPass {
			t.Errorf("loadForTest failed: code %d", code)
			return
		}
		if _, err := in.CallByName("testTimeout"); err != nil {
			t.Errorf("testTimeout failed: %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ntp.queryWith did not time out (UDP receive deadline not honoured)")
	}
}
