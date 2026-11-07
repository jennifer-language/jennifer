// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package netlib_test

import (
	"fmt"
	"runtime"
	"testing"
)

// TestRecvFromReusesBuffer proves net.recvFrom does not allocate a 64 KiB
// receive buffer per call: 500 small datagrams cost far less than 500 x 64 KiB.
func TestRecvFromReusesBuffer(t *testing.T) {
	const calls = 500
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	out, runErr := runProg(t, fmt.Sprintf(`
		use io; use net; use convert;
		def a as net.UDPSocket init net.listenUDP("127.0.0.1:0");
		def b as net.UDPSocket init net.listenUDP("127.0.0.1:0");
		def dest as string init net.address($b);
		def msg as bytes init convert.bytesFromString("0123456789abcdef", "utf-8");
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
