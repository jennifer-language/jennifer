// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package netlib_test

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// TestRecvFromReusesBuffer proves net.recvFrom does not allocate a 64 KiB
// receive buffer per call: 500 small datagrams cost far less than 500 x 64 KiB.
func TestRecvFromReusesBuffer(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector makes sync.Pool drop buffers at random")
	}
	const calls = 500
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	out, runErr := runProg(t, fmt.Sprintf(`
		use io; use net; use convert;
use encoding;
		def a as net.UDPSocket init net.listenUDP("127.0.0.1:0");
		def b as net.UDPSocket init net.listenUDP("127.0.0.1:0");
		def dest as string init net.address($b);
		def msg as bytes init encoding.encode("utf-8", "0123456789abcdef");
		def total as int init 0;
		for (def i in 0..%d) {
			net.sendTo($a, $dest, $msg);
			def dg as net.Datagram init net.recvFrom($b, 1024);
			$total = $total + len($dg.data);
		}
		io.printf("%%d", $total);
		net.close($a);
		net.close($b);
	`, calls))
	runtime.ReadMemStats(&after)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if out != fmt.Sprint(calls*16) {
		t.Fatalf("got %q, want %d bytes received", out, calls*16)
	}
	got := after.TotalAlloc - before.TotalAlloc
	t.Logf("%d recvFrom calls allocated %d bytes (%d per call)", calls, got, got/calls)
	if got > calls*16<<10 {
		t.Fatalf("%d recvFrom calls allocated %d bytes; the receive buffer must be reused", calls, got)
	}
}

// TestRecvFromWakesOnCancel: task.cancel wakes a recvFrom blocked in a spawned
// task at once ("task cancelled"), as it does a TCP read, instead of leaving it
// to its read deadline ("timeout"); the socket stays usable afterwards.
func TestRecvFromWakesOnCancel(t *testing.T) {
	start := time.Now()
	out, runErr := runProg(t, `
		use io; use net; use task; use convert;
use encoding;
		def s as net.UDPSocket init net.listenUDP("127.0.0.1:0");
		net.setReadDeadline($s, 5000);
		def t as task of int init spawn {
			def dg as net.Datagram init net.recvFrom($s, 16);
			return len($dg.data);
		};
		task.cancel($t);
		try {
			task.wait($t);
			io.printf("returned;");
		} catch (e) {
			io.printf("%s;", $e.kind);
		}
		net.sendTo($s, net.address($s), encoding.encode("utf-8", "hi"));
		io.printf("%d", len(net.recvFrom($s, 16).data));
		net.close($s);
	`)
	if runErr != nil {
		t.Fatalf("run: %v (out=%q)", runErr, out)
	}
	if out != "cancelled;2" {
		t.Fatalf("got %q, want %q", out, "cancelled;2")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %v: the cancel did not wake recvFrom", d)
	}
}

// TestPerCallCapsAreKindLimit: a count over the 256 MiB per-call cap of
// readN / readBytes / recvFrom is kind "limit", not "runtime".
func TestPerCallCapsAreKindLimit(t *testing.T) {
	out, runErr := runProg(t, `
		use io; use net;
		def l as net.Listener init net.listen("127.0.0.1:0");
		def c as net.Conn init net.connect(net.address($l));
		def u as net.UDPSocket init net.listenUDP("127.0.0.1:0");
		def big as int init 1 << 40;
		try { net.readN($c, $big); } catch (e) { io.printf("%s;", $e.kind); }
		try { net.readBytes($c, $big); } catch (e) { io.printf("%s;", $e.kind); }
		try { net.recvFrom($u, $big); } catch (e) { io.printf("%s", $e.kind); }
		net.close($c); net.close($l); net.close($u);
	`)
	if runErr != nil {
		t.Fatalf("run: %v (out=%q)", runErr, out)
	}
	if out != "limit;limit;limit" {
		t.Fatalf("got %q", out)
	}
}
