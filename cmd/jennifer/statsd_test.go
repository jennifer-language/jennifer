// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestStatsdEmits drives the statsd verbs against a real UDP loopback listener
// and checks each one puts the expected `metric:value|type` line on the wire.
// Each `.j` method sends exactly one datagram, read back and asserted in order.
func TestStatsdEmits(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	addr := pc.LocalAddr().String()

	statsdMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "statsd.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`import %q as statsd;
def const ADDR as string init %q;

func send(prefix as string) {
    def c as statsd.Client init statsd.clientWith(ADDR, $prefix);
    return $c;
}

func testCount() { def c as statsd.Client init send(""); statsd.count($c, "errors", 3); statsd.close($c); }
func testIncrement() { def c as statsd.Client init send(""); statsd.increment($c, "hits"); statsd.close($c); }
func testDecrement() { def c as statsd.Client init send(""); statsd.decrement($c, "hits"); statsd.close($c); }
func testGauge() { def c as statsd.Client init send(""); statsd.gauge($c, "queue.depth", 7); statsd.close($c); }
func testTiming() { def c as statsd.Client init send(""); statsd.timing($c, "response", 42); statsd.close($c); }
func testSet() { def c as statsd.Client init send(""); statsd.set($c, "users", "u123"); statsd.close($c); }
func testPrefixed() { def c as statsd.Client init send("web"); statsd.increment($c, "requests"); statsd.close($c); }
`, statsdMod, addr)
	progPath := filepath.Join(dir, "prog.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	in, code := loadForTest(progPath)
	if in == nil || code != testExitPass {
		t.Fatalf("loadForTest failed: code %d", code)
	}

	cases := []struct {
		method string
		want   string
	}{
		{"testCount", "errors:3|c"},
		{"testIncrement", "hits:1|c"},
		{"testDecrement", "hits:-1|c"},
		{"testGauge", "queue.depth:7|g"},
		{"testTiming", "response:42|ms"},
		{"testSet", "users:u123|s"},
		{"testPrefixed", "web.requests:1|c"},
	}
	buf := make([]byte, 512)
	for _, c := range cases {
		if _, err := in.CallByName(c.method); err != nil {
			t.Errorf("%s failed: %v", c.method, err)
			continue
		}
		if err := pc.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatal(err)
		}
		n, _, rerr := pc.ReadFrom(buf)
		if rerr != nil {
			t.Errorf("%s: no datagram received: %v", c.method, rerr)
			continue
		}
		if got := string(buf[:n]); got != c.want {
			t.Errorf("%s: wire line = %q, want %q", c.method, got, c.want)
		}
	}
}
