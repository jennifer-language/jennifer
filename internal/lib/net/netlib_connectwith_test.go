// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

// Tests for net.ConnectOptions (M29.37): connectWith, startTLSWith,
// lookupWith / reverseLookupWith, the default deadline, and the deprecated
// timeout shapes that keep working until 0.29.0.

package netlib_test

import (
	"fmt"
	stdnet "net"
	"strings"
	"testing"
	"time"

	netlib "jennifer-lang.dev/jennifer/internal/lib/net"
)

// stalledListener accepts connections and never sends a byte, so a TLS
// handshake against it stalls until its deadline. Returns its address.
func stalledListener(t *testing.T) string {
	t.Helper()
	l, err := stdnet.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	t.Cleanup(func() { close(done); l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { <-done; c.Close() }()
		}
	}()
	return l.Addr().String()
}

// echoListener answers each connection by echoing what it reads.
func echoListener(t *testing.T) string {
	t.Helper()
	l, err := stdnet.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				buf := make([]byte, 64)
				n, err := c.Read(buf)
				if err != nil {
					return
				}
				_, _ = c.Write(buf[:n])
			}()
		}
	}()
	return l.Addr().String()
}

// kindOf runs body inside try and prints the caught kind, or "ok".
func kindOf(t *testing.T, body string) string {
	t.Helper()
	out, err := runProg(t, `
		use io;
		use net;
		try { `+body+` io.printf("ok"); } catch (e) { io.printf("%s", $e.kind); }
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return out
}

func within(t *testing.T, start time.Time, limit time.Duration) {
	t.Helper()
	if el := time.Since(start); el > limit {
		t.Fatalf("took %v, want under %v", el, limit)
	}
}

// TestConnectWithUnroutableTimesOut: a connect to an address that drops the
// SYN gives up at timeoutMs with kind "timeout". A sandbox without a route
// answers "unreachable" at once instead, which proves nothing here.
func TestConnectWithUnroutableTimesOut(t *testing.T) {
	start := time.Now()
	got := kindOf(t, `def c as net.Conn init net.connectWith("10.255.255.1:9", net.ConnectOptions{timeoutMs: 200});`)
	if got == "net" {
		t.Skip("no route to 10.255.255.1 here: the connect failed at once")
	}
	if got != "timeout" {
		t.Fatalf("kind = %q, want timeout", got)
	}
	within(t, start, 5*time.Second)
}

// TestConnectWithStalledHandshakeTimesOut: with tls set, timeoutMs bounds the
// handshake too; a peer that accepts and never answers is "timeout".
func TestConnectWithStalledHandshakeTimesOut(t *testing.T) {
	addr := stalledListener(t)
	start := time.Now()
	got := kindOf(t, fmt.Sprintf(`def c as net.Conn init net.connectWith(%q, net.ConnectOptions{timeoutMs: 200, tls: true});`, addr))
	if got != "timeout" {
		t.Fatalf("kind = %q, want timeout", got)
	}
	within(t, start, 5*time.Second)
}

// TestDefaultDeadlineApplies: timeoutMs 0, connectTLS without options and
// startTLS all take the default deadline (lowered here from 30 s).
func TestDefaultDeadlineApplies(t *testing.T) {
	netlib.SetDefaultTimeoutForTest(t, 200*time.Millisecond)
	addr := stalledListener(t)
	for _, body := range []string{
		fmt.Sprintf(`def c as net.Conn init net.connectWith(%q, net.ConnectOptions{tls: true});`, addr),
		fmt.Sprintf(`def c as net.Conn init net.connectTLS(%q);`, addr),
		fmt.Sprintf(`def c as net.Conn init net.connect(%q); net.startTLS($c);`, strings.Replace(addr, "127.0.0.1", "localhost", 1)),
		fmt.Sprintf(`def c as net.Conn init net.connect(%q); net.startTLSWith($c, net.ConnectOptions{});`, strings.Replace(addr, "127.0.0.1", "localhost", 1)),
	} {
		start := time.Now()
		if got := kindOf(t, body); got != "timeout" {
			t.Errorf("%s: kind = %q, want timeout", body, got)
		}
		within(t, start, 5*time.Second)
	}
}

// TestDefaultDeadlineEndsWithHandshake: the default bounds the connect only;
// a read after it waits longer than the default without timing out.
func TestDefaultDeadlineEndsWithHandshake(t *testing.T) {
	netlib.SetDefaultTimeoutForTest(t, 100*time.Millisecond)
	l, err := stdnet.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		time.Sleep(400 * time.Millisecond)
		_, _ = c.Write([]byte("late"))
	}()
	got := kindOf(t, fmt.Sprintf(`def c as net.Conn init net.connect(%q);
		def b as bytes init net.readBytes($c, 4);
		if (len($b) != 4) { throw Error{kind: "short", message: "short read"}; }`, l.Addr().String()))
	if got != "ok" {
		t.Fatalf("kind = %q, want ok", got)
	}
}

// TestConnectWithNegativeTimeoutThrows: a negative timeoutMs is "runtime".
func TestConnectWithNegativeTimeoutThrows(t *testing.T) {
	for _, body := range []string{
		`def c as net.Conn init net.connectWith("127.0.0.1:1", net.ConnectOptions{timeoutMs: -1});`,
		`def a as list of string init net.lookupWith("localhost", net.ConnectOptions{timeoutMs: -5});`,
		`def c as net.Conn init net.connect("127.0.0.1:1", -1);`,
	} {
		if got := kindOf(t, body); got != "runtime" {
			t.Errorf("%s: kind = %q, want runtime", body, got)
		}
	}
	_, err := runProg(t, `use net; def c as net.Conn init net.connectWith("127.0.0.1:1", net.ConnectOptions{timeoutMs: -1});`)
	if err == nil || !strings.Contains(err.Error(), "timeoutMs must be >= 0") {
		t.Fatalf("error = %v, want the timeoutMs rule", err)
	}
}

// TestConnectWithMaxIntIsNoDeadline: the largest int asks for no deadline; a
// connect to a live listener works, and so does a handshake with none.
func TestConnectWithMaxIntIsNoDeadline(t *testing.T) {
	addr := echoListener(t)
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use net;
		use encoding;
		def c as net.Conn init net.connectWith(%q, net.ConnectOptions{timeoutMs: 9223372036854775807});
		net.writeBytes($c, encoding.encode("utf-8", "hi"));
		io.printf("%%s", encoding.decode("utf-8", net.readBytes($c, 2)));
		net.close($c);
	`, addr))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "hi" {
		t.Fatalf("got %q, want hi", out)
	}
}

// TestConnectWithOptionsType: the second argument must be a ConnectOptions.
func TestConnectWithOptionsType(t *testing.T) {
	_, err := runProg(t, `use net; def o as net.TLSOptions; def c as net.Conn init net.connectWith("127.0.0.1:1", $o);`)
	if err == nil || !strings.Contains(err.Error(), "must be a net.ConnectOptions, got net.TLSOptions") {
		t.Fatalf("error = %v, want the options type", err)
	}
}

// TestLookupWith: lookupWith and reverseLookupWith answer like lookup and
// reverseLookup within their deadline.
func TestLookupWith(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use net;
		def o as net.ConnectOptions init net.ConnectOptions{timeoutMs: 5000};
		def a as list of string init net.lookupWith("localhost", $o);
		io.printf("%d;", len($a));
		def b as list of string init net.lookupWith("localhost", net.ConnectOptions{});
		io.printf("%d", len($b));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out == "" || strings.HasPrefix(out, "0;") || strings.HasSuffix(out, ";0") {
		t.Fatalf("got %q, want two non-zero counts", out)
	}
	if got := kindOf(t, `def n as list of string init net.reverseLookupWith("127.0.0.1", net.ConnectOptions{timeoutMs: 5000});`); got != "ok" && got != "net" {
		t.Fatalf("reverseLookupWith kind = %q, want ok (or net without a PTR record)", got)
	}
}

// TestLookupTimesOut: a lookup whose name server never answers is "timeout"
// at timeoutMs, and at the default for lookup / reverseLookup.
func TestLookupTimesOut(t *testing.T) {
	netlib.SetStalledResolverForTest(t)
	netlib.SetDefaultTimeoutForTest(t, 200*time.Millisecond)
	for _, body := range []string{
		`def a as list of string init net.lookupWith("stalled.jennifer-lang.dev", net.ConnectOptions{timeoutMs: 200});`,
		`def a as list of string init net.lookup("stalled.jennifer-lang.dev");`,
		`def a as list of string init net.reverseLookup("192.0.2.7");`,
	} {
		start := time.Now()
		if got := kindOf(t, body); got != "timeout" {
			t.Errorf("%s: kind = %q, want timeout", body, got)
		}
		within(t, start, 5*time.Second)
	}
}

// TestDeprecatedTimeoutShapesWork: the deprecated trailing timeouts keep
// working until 0.29.0.
func TestDeprecatedTimeoutShapesWork(t *testing.T) {
	addr := stalledListener(t)
	for _, body := range []string{
		fmt.Sprintf(`def c as net.Conn init net.connectTLS(%q, 200);`, addr),
		fmt.Sprintf(`def o as net.TLSOptions; def c as net.Conn init net.connectTLS(%q, $o, 200);`, addr),
		fmt.Sprintf(`def c as net.Conn init net.connect(%q, 200); net.startTLS($c, 200);`, strings.Replace(addr, "127.0.0.1", "localhost", 1)),
		fmt.Sprintf(`def c as net.Conn init net.connect(%q); net.readN($c, 4, 200);`, addr),
		fmt.Sprintf(`def c as net.Conn init net.connect(%q); net.readAll($c, 100, 200);`, addr),
	} {
		start := time.Now()
		if got := kindOf(t, body); got != "timeout" {
			t.Errorf("%s: kind = %q, want timeout", body, got)
		}
		within(t, start, 5*time.Second)
	}
	if got := kindOf(t, fmt.Sprintf(`def c as net.Conn init net.connect(%q); if (net.eof($c)) { throw Error{kind: "eof", message: "eof"}; }`, addr)); got != "ok" {
		t.Errorf("net.eof on an open connection: %q, want ok", got)
	}
}

// scriptedServer accepts one connection and runs serve on it.
func scriptedServer(t *testing.T, serve func(c stdnet.Conn)) string {
	t.Helper()
	l, err := stdnet.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		serve(c)
	}()
	return l.Addr().String()
}

// trickle sends n bytes, one every gap, then closes.
func trickle(n int, gap time.Duration) func(stdnet.Conn) {
	return func(c stdnet.Conn) {
		for i := 0; i < n; i++ {
			time.Sleep(gap)
			if _, err := c.Write([]byte{'x'}); err != nil {
				return
			}
		}
	}
}

// TestIdleTimeoutStalledPeer: under setIdleTimeout a read that sees no byte
// for ms is "timeout", for readBytes, readN and readAll alike.
func TestIdleTimeoutStalledPeer(t *testing.T) {
	for _, read := range []string{"net.readBytes($c, 4);", "net.readN($c, 4);", "net.readAll($c);"} {
		addr := stalledListener(t)
		start := time.Now()
		got := kindOf(t, fmt.Sprintf(`def c as net.Conn init net.connect(%q);
			net.setIdleTimeout($c, 150);
			%s`, addr, read))
		if got != "timeout" {
			t.Errorf("%s: kind = %q, want timeout", read, got)
		}
		within(t, start, 5*time.Second)
	}
}

// TestIdleTimeoutSteadySender: a sender that sends a byte every ms/2.5 is
// never cut, though the whole transfer takes several times ms.
func TestIdleTimeoutSteadySender(t *testing.T) {
	addr := scriptedServer(t, trickle(10, 80*time.Millisecond))
	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use net;
		def c as net.Conn init net.connect(%q);
		net.setIdleTimeout($c, 200);
		io.printf("%%d;", len(net.readN($c, 6)));
		io.printf("%%d", len(net.readAll($c)));
	`, addr))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "6;4" {
		t.Fatalf("got %q, want 6;4", out)
	}
}

// TestIdleTimeoutCleared: 0 and the largest int clear the idle timeout.
func TestIdleTimeoutCleared(t *testing.T) {
	for _, ms := range []string{"0", "9223372036854775807"} {
		addr := scriptedServer(t, trickle(1, 300*time.Millisecond))
		got := kindOf(t, fmt.Sprintf(`def c as net.Conn init net.connect(%q);
			net.setIdleTimeout($c, 100);
			net.setIdleTimeout($c, %s);
			if (len(net.readBytes($c, 1)) != 1) { throw Error{kind: "short", message: "no byte"}; }`, addr, ms))
		if got != "ok" {
			t.Errorf("cleared with %s: kind = %q, want ok", ms, got)
		}
	}
	addr := stalledListener(t)
	if got := kindOf(t, fmt.Sprintf(`def c as net.Conn init net.connect(%q); net.setIdleTimeout($c, -1);`, addr)); got != "runtime" {
		t.Errorf("negative: kind = %q, want runtime", got)
	}
}

// TestIdleTimeoutWithReadDeadline: an idle timeout and a read deadline both
// apply, and whichever passes first fires: a steady sender is cut by the
// deadline, a stall by the idle timeout before a later deadline. The deadline
// still holds for a plain read afterwards.
func TestIdleTimeoutWithReadDeadline(t *testing.T) {
	addr := scriptedServer(t, trickle(20, 50*time.Millisecond))
	start := time.Now()
	got := kindOf(t, fmt.Sprintf(`def c as net.Conn init net.connect(%q);
		net.setIdleTimeout($c, 200);
		net.setReadDeadline($c, 300);
		net.readN($c, 20);`, addr))
	if got != "timeout" {
		t.Fatalf("steady sender under a 300 ms deadline: kind = %q, want timeout", got)
	}
	if el := time.Since(start); el > 900*time.Millisecond {
		t.Fatalf("deadline fired after %v, want about 300 ms", el)
	}
	addr = stalledListener(t)
	start = time.Now()
	got = kindOf(t, fmt.Sprintf(`def c as net.Conn init net.connect(%q);
		net.setReadDeadline($c, 5000);
		net.setIdleTimeout($c, 150);
		net.readBytes($c, 1);`, addr))
	if got != "timeout" {
		t.Fatalf("stall under idle 150 / deadline 5000: kind = %q, want timeout", got)
	}
	within(t, start, 2*time.Second)
}
