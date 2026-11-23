// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package netlib_test

import (
	"fmt"
	"strings"
	"testing"
)

// TestConnectRefusedIsNetError: a refused connection is a net.NetError (kind
// "net", op "connect", reason "refused") that a program can declare as one, its
// message the OS text it had as a runtime error.
func TestConnectRefusedIsNetError(t *testing.T) {
	addr := pickListenerAddr(t) // closed again: nothing listens there
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use net;
		try {
			def c as net.Conn init net.connect(%q, 2000);
			io.printf("connected\n");
		} catch (e) {
			def ne as net.NetError init $e;
			io.printf("%%s %%s %%s %%v %%v %%v\n", $ne.kind, $ne.op, $ne.reason, $ne.sent, $ne.replied,
				$ne.message == "net.connect: %s: dial tcp %s: connect: connection refused");
		}
	`, addr, addr, addr))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "net connect refused false false true\n" {
		t.Errorf("got %q", out)
	}
}

// TestAbortResetsPeer: net.abort closes with a reset, so the peer's next read
// fails with a NetError of reason "reset" (a net.close would read as EOF).
func TestAbortResetsPeer(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use net;
		use task;
		use convert;
use encoding;

		def listener as net.Listener init net.listen("127.0.0.1:0");
		def addr as string init net.address($listener);
		def server as task of null init spawn {
			def c as net.Conn init net.accept($listener);
			net.readN($c, 5);
			net.abort($c);
			return null;
		};
		def client as net.Conn init net.connect($addr);
		net.writeBytes($client, encoding.encode("utf-8", "hello"));
		task.wait($server);
		try {
			def got as bytes init net.readBytes($client, 10);
			io.printf("read %d bytes\n", len($got));
		} catch (e) {
			io.printf("%s %s %s\n", $e.kind, $e.op, $e.reason);
		}
		net.close($client);
		net.close($listener);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "net read reset\n" {
		t.Errorf("got %q, want %q", out, "net read reset\n")
	}
}

// TestAbortClosedConn (M29.46): aborting or closing a handle already closed
// does nothing, any other use is kind "closed"; aborting a listener is a
// runtime error naming net.Conn.
func TestAbortClosedConn(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use net;
		def listener as net.Listener init net.listen("127.0.0.1:0");
		def c as net.Conn init net.connect(net.address($listener));
		def u as net.UDPSocket init net.listenUDP("127.0.0.1:0");
		net.close($c);
		net.abort($c);
		net.close($c);
		net.close($u);
		net.close($u);
		try {
			net.writeBytes($c, net.readBytes($c, 1));
		} catch (e) {
			io.printf("%s\n", $e.kind);
		}
		try {
			net.recvFrom($u, 10);
		} catch (e) {
			io.printf("%s\n", $e.kind);
		}
		def l2 as net.Listener init net.listen("127.0.0.1:0");
		net.close($l2);
		net.close($l2);
		try {
			net.accept($l2);
		} catch (e) {
			io.printf("%s\n", $e.kind);
		}
		net.abort($listener);
	`)
	if out != "closed\nclosed\nclosed\n" {
		t.Errorf("got %q, want %q", out, "closed\nclosed\nclosed\n")
	}
	if err == nil || !strings.Contains(err.Error(), "net.abort: argument must be a net.Conn") {
		t.Errorf("abort of a listener: got %v", err)
	}
}
