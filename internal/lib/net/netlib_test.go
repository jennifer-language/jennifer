// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

// Tests for the standard-Go net implementation. Skipped under
// TinyGo because the surface is stub-only there and every call
// returns the same friendly error - no round-trip semantics to
// verify.

package netlib_test

import (
	"bytes"
	"fmt"
	stdnet "net"
	"strings"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/convert"
	fslib "jennifer-lang.dev/jennifer/internal/lib/fs"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	netlib "jennifer-lang.dev/jennifer/internal/lib/net"
	tasklib "jennifer-lang.dev/jennifer/internal/lib/task"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// runProg parses + runs a Jennifer program with io + task + fs +
// net installed. Wipes the net registries between runs so a
// leaked handle can't leak an id into the next test.
func runProg(t *testing.T, src string) (string, error) {
	t.Helper()
	netlib.ResetForTest()
	fslib.ResetForTest()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	convert.Install(in)
	tasklib.Install(in)
	fslib.Install(in)
	netlib.Install(in)
	runErr := in.Run(prog)
	_ = in.UnwaitedTaskErrors()
	return buf.String(), runErr
}

// pickListenerAddr binds a listener on an ephemeral port outside
// Jennifer so the test can predict the address to hand to
// net.connect / net.listenUDP without racing.
func pickListenerAddr(t *testing.T) string {
	t.Helper()
	l, err := stdnet.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pickListenerAddr: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

// TestTCPEchoWithFixedAddress uses a pre-picked ephemeral port so
// both the server-side listen and the client-side connect know the
// address up front. The server is a Go goroutine (not a Jennifer
// spawn) because we're testing the client half from Jennifer.
func TestTCPEchoWithFixedAddress(t *testing.T) {
	addr := pickListenerAddr(t)
	l, err := stdnet.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("stdnet.Listen: %v", err)
	}
	defer l.Close()

	go func() {
		c, aErr := l.Accept()
		if aErr != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 5)
		if _, rErr := c.Read(buf); rErr != nil {
			return
		}
		_, _ = c.Write(buf)
	}()

	out, runErr := runProg(t, fmt.Sprintf(`
		use io;
		use net;

		def c as net.Conn init net.connect(%q);
		net.writeBytes($c, convert.bytesFromString("hello", "utf-8"));
		def reply as bytes init net.readBytes($c, 5);
		net.close($c);
		use convert;
		io.printf("%%s", convert.stringFromBytes($reply, "utf-8"));
	`, addr))
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if out != "hello" {
		t.Errorf("got %q, want %q", out, "hello")
	}
}

// TestTCPFullJenniferServerAndClient wires the whole thing inside
// Jennifer: spawn a listener that accepts one connection and
// writes back "ok", then the main flow connects and reads.
func TestTCPFullJenniferServerAndClient(t *testing.T) {
	// Use a Go-picked port so the client knows where to dial. The
	// Jennifer server binds that same port via net.listen.
	addr := pickListenerAddr(t)

	out, err := runProg(t, fmt.Sprintf(`
		use io;
		use net;
		use task;
		use convert;

		def listener as net.Listener init net.listen(%q);
		def server as task of null init spawn {
			def c as net.Conn init net.accept($listener);
			net.writeBytes($c, convert.bytesFromString("ok", "utf-8"));
			net.close($c);
			return null;
		};

		def client as net.Conn init net.connect(%q);
		def reply as bytes init net.readBytes($client, 2);
		net.close($client);

		task.wait($server);
		net.close($listener);
		io.printf("%%s", convert.stringFromBytes($reply, "utf-8"));
	`, addr, addr))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "ok" {
		t.Errorf("got %q, want %q", out, "ok")
	}
}

// net.eof must NOT block on an open connection where no data is pending (it is
// the local side's turn to write): a bare Peek(1) would deadlock. The server
// here accepts and stays silent; eof must return false promptly.
func TestTCPEOFDoesNotBlockOnOpenIdleConn(t *testing.T) {
	addr := pickListenerAddr(t)
	l, err := stdnet.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	accepted := make(chan stdnet.Conn, 1)
	go func() {
		c, aErr := l.Accept()
		if aErr != nil {
			return
		}
		accepted <- c // keep the connection open (never write, never close)
	}()

	done := make(chan struct{})
	var out string
	var runErr error
	go func() {
		out, runErr = runProg(t, fmt.Sprintf(`
			use io;
			use net;
			def c as net.Conn init net.connect(%q);
			io.printf("eof=%%t\n", net.eof($c));
			net.close($c);
		`, addr))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("net.eof blocked on an open idle connection (deadlock)")
	}
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if out != "eof=false\n" {
		t.Errorf("got %q, want %q", out, "eof=false\n")
	}
	if c := <-accepted; c != nil {
		c.Close()
	}
}

// TestTCPEOFAfterServerClose exercises the sticky-EOF flow. Server
// writes 3 bytes then closes; client reads them, then a follow-up
// read returns a partial (empty) result and net.eof flips true.
func TestTCPEOFAfterServerClose(t *testing.T) {
	addr := pickListenerAddr(t)
	l, err := stdnet.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	go func() {
		c, aErr := l.Accept()
		if aErr != nil {
			return
		}
		_, _ = c.Write([]byte("abc"))
		_ = c.Close()
	}()

	out, runErr := runProg(t, fmt.Sprintf(`
		use io;
		use net;
		use convert;

		def c as net.Conn init net.connect(%q);
		def payload as bytes init net.readBytes($c, 3);
		io.printf("payload=%%s eof=%%t\n", convert.stringFromBytes($payload, "utf-8"), net.eof($c));
		def rest as bytes init net.readBytes($c, 5);
		io.printf("rest_len=%%d eof=%%t\n", len($rest), net.eof($c));
		net.close($c);
	`, addr))
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	want := "payload=abc eof=true\nrest_len=0 eof=true\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestUDPWithPickedAddresses uses raw Go PacketConns to pick two
// ephemeral UDP ports the Jennifer code can send between.
func TestUDPWithPickedAddresses(t *testing.T) {
	// Reserve two ephemeral UDP ports.
	p1, err := stdnet.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr1 := p1.LocalAddr().String()
	_ = p1.Close()

	p2, err := stdnet.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr2 := p2.LocalAddr().String()
	_ = p2.Close()

	out, runErr := runProg(t, fmt.Sprintf(`
		use io;
		use net;
		use convert;

		def a as net.UDPSocket init net.listenUDP(%q);
		def b as net.UDPSocket init net.listenUDP(%q);

		net.sendTo($a, %q, convert.bytesFromString("ping", "utf-8"));
		def dg as net.Datagram init net.recvFrom($b, 1024);
		io.printf("data=%%s\n", convert.stringFromBytes($dg.data, "utf-8"));
		io.printf("peer=%%s\n", $dg.peer);

		net.close($a);
		net.close($b);
	`, addr1, addr2, addr2))
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if !strings.HasPrefix(out, "data=ping\n") {
		t.Errorf("got %q, want data=ping prefix", out)
	}
	if !strings.Contains(out, "peer=127.0.0.1:") {
		t.Errorf("peer address missing from %q", out)
	}
}

// TestDNSLookupLocalhost exercises net.lookup on a name that always
// resolves. We check for either the v4 loopback or the v6 loopback
// so the test survives whichever resolver order the host uses.
func TestDNSLookupLocalhost(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use net;
		def ips as list of string init net.lookup("localhost");
		io.printf("%d", len($ips));
	`)
	if err != nil {
		// DNS resolution might be misconfigured in the test env; skip
		// rather than failing loudly, per docs/technical/testing.md.
		t.Skipf("net.lookup('localhost') skipped: %v", err)
	}
	if out == "0" {
		t.Errorf("expected at least one IP for localhost, got empty list")
	}
}

// TestReverseLookupLoopback exercises net.reverseLookup on 127.0.0.1.
func TestReverseLookupLoopback(t *testing.T) {
	out, err := runProg(t, `
		use io;
		use net;
		def names as list of string init net.reverseLookup("127.0.0.1");
		io.printf("%d", len($names));
	`)
	if err != nil {
		t.Skipf("net.reverseLookup skipped: %v", err)
	}
	if out == "0" {
		t.Errorf("expected at least one name for 127.0.0.1")
	}
}

// TestClosePolymorphic - the single close verb accepts three kinds.
func TestClosePolymorphic(t *testing.T) {
	_, err := runProg(t, `
		use net;
		def l as net.Listener init net.listen("127.0.0.1:0");
		def u as net.UDPSocket init net.listenUDP("127.0.0.1:0");
		net.close($l);
		net.close($u);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestCloseRejectsWrongType - passing a non-net struct to net.close
// errors with the actionable message.
func TestCloseRejectsWrongType(t *testing.T) {
	_, err := runProg(t, `
		use net;
		use fs;
		def f as fs.File init fs.open("/dev/null", "read");
		net.close($f);
	`)
	if err == nil {
		t.Fatal("expected boundary error for wrong-type close")
	}
	if !strings.Contains(err.Error(), "net.Conn") {
		t.Errorf("error should list net kinds: %v", err)
	}
}

// TestConnectUnknownHost exercises the address-not-found boundary.
func TestConnectUnknownHost(t *testing.T) {
	_, err := runProg(t, `
		use net;
		def c as net.Conn init net.connect("nonexistent.invalid:9999");
	`)
	if err == nil {
		t.Fatal("expected boundary error")
	}
	if !strings.Contains(err.Error(), "net.connect") {
		t.Errorf("error should be positioned at net.connect: %v", err)
	}
}

// TestUseAfterClose - close then read.
func TestUseAfterClose(t *testing.T) {
	addr := pickListenerAddr(t)
	l, err := stdnet.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		c, _ := l.Accept()
		if c != nil {
			_ = c.Close()
		}
	}()

	_, runErr := runProg(t, fmt.Sprintf(`
		use net;
		def c as net.Conn init net.connect(%q);
		net.close($c);
		def x as bytes init net.readBytes($c, 1);
	`, addr))
	if runErr == nil {
		t.Fatal("expected use-after-close error")
	}
	if !strings.Contains(runErr.Error(), "not open") {
		t.Errorf("error should mention 'not open': %v", runErr)
	}
}

// TestSetDeadlineTimesOut - a server that accepts but stays silent, so a
// short read deadline elapses and readBytes fails with the distinguishable
// "timed out" message (catchable in .j), not a crash.
func TestSetDeadlineTimesOut(t *testing.T) {
	addr := pickListenerAddr(t)
	l, err := stdnet.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	done := make(chan struct{})
	go func() {
		c, aErr := l.Accept()
		if aErr != nil {
			return
		}
		<-done // hold the connection open, sending nothing
		_ = c.Close()
	}()
	defer close(done)

	_, runErr := runProg(t, fmt.Sprintf(`
		use net;
		def c as net.Conn init net.connect(%q);
		net.setDeadline($c, 50);
		def x as bytes init net.readBytes($c, 1);
	`, addr))
	if runErr == nil {
		t.Fatal("expected a timeout error")
	}
	if !strings.Contains(runErr.Error(), "timed out") {
		t.Errorf("error should be a distinguishable timeout: %v", runErr)
	}
}

// TestSetDeadlineClearRestoresRead - arm a deadline, clear it with ms 0,
// then a read that arrives after the original deadline still succeeds.
func TestSetDeadlineClearRestoresRead(t *testing.T) {
	addr := pickListenerAddr(t)
	l, err := stdnet.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		c, aErr := l.Accept()
		if aErr != nil {
			return
		}
		defer c.Close()
		time.Sleep(120 * time.Millisecond) // send only after the original 50ms deadline would fire
		_, _ = c.Write([]byte("z"))
	}()

	out, runErr := runProg(t, fmt.Sprintf(`
		use io;
		use net;
		use convert;
		def c as net.Conn init net.connect(%q);
		net.setDeadline($c, 50);
		net.setDeadline($c, 0);
		def x as bytes init net.readBytes($c, 1);
		net.close($c);
		io.printf("%%s", convert.stringFromBytes($x, "utf-8"));
	`, addr))
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if out != "z" {
		t.Errorf("got %q, want %q", out, "z")
	}
}

// The eof probe arms its own short read deadline on the shared conn. It must
// restore the deadline set via net.setDeadline afterwards - if it cleared it,
// a subsequent read on a stalled peer would block forever, the exact hang
// setDeadline exists to prevent.
func TestEOFRestoresUserDeadline(t *testing.T) {
	addr := pickListenerAddr(t)
	l, err := stdnet.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	accepted := make(chan stdnet.Conn, 1)
	go func() {
		c, aErr := l.Accept()
		if aErr != nil {
			return
		}
		accepted <- c // keep open, never write
	}()

	done := make(chan struct{})
	var out string
	var runErr error
	go func() {
		out, runErr = runProg(t, fmt.Sprintf(`
			use io;
			use net;
			def c as net.Conn init net.connect(%q);
			net.setDeadline($c, 200);
			io.printf("eof=%%t\n", net.eof($c));
			try {
				def b as bytes init net.readBytes($c, 1);
				io.printf("read=%%d\n", len($b));
			} catch (e) {
				io.printf("err=%%s\n", $e.message);
			}
			net.close($c);
		`, addr))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("readBytes blocked: eof cleared the user-set deadline")
	}
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	want := "eof=false\nerr=net.readBytes: read timed out\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if c := <-accepted; c != nil {
		c.Close()
	}
}

// While another task is blocked in readBytes, an eof poll must not arm its
// probe deadline on the shared conn - that would spuriously time the blocked
// read out. It skips the probe (TryLock) and reports "not EOF"; the reader
// then receives the peer's data intact.
func TestEOFDoesNotDisturbConcurrentReader(t *testing.T) {
	addr := pickListenerAddr(t)
	l, err := stdnet.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		c, aErr := l.Accept()
		if aErr != nil {
			return
		}
		time.Sleep(300 * time.Millisecond) // long enough for many eof polls
		_, _ = c.Write([]byte("hello"))
		_ = c.Close()
	}()

	done := make(chan struct{})
	var out string
	var runErr error
	go func() {
		out, runErr = runProg(t, fmt.Sprintf(`
			use io;
			use net;
			use task;
			use convert;
			def c as net.Conn init net.connect(%q);
			def rd as task of string init spawn {
				def b as bytes init net.readBytes($c, 5);
				return convert.stringFromBytes($b, "utf-8");
			};
			def i as int init 0;
			while ($i < 20) {
				if (net.eof($c)) { break; }
				$i = $i + 1;
			}
			io.printf("got=%%s\n", task.wait($rd));
			net.close($c);
		`, addr))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent reader / eof poll deadlocked")
	}
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if out != "got=hello\n" {
		t.Errorf("got %q, want %q", out, "got=hello\n")
	}
}

// TestConnectWithTimeoutSucceeds - a timeout argument does not break a normal,
// reachable connect: the dial still completes and the round-trip works.
func TestConnectWithTimeoutSucceeds(t *testing.T) {
	addr := pickListenerAddr(t)
	l, err := stdnet.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("stdnet.Listen: %v", err)
	}
	defer l.Close()
	go func() {
		c, aErr := l.Accept()
		if aErr != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 2)
		if _, rErr := c.Read(buf); rErr != nil {
			return
		}
		_, _ = c.Write(buf)
	}()
	out, runErr := runProg(t, fmt.Sprintf(`
		use io;
		use net;
		use convert;
		def c as net.Conn init net.connect(%q, 5000);
		net.writeBytes($c, convert.bytesFromString("hi", "utf-8"));
		def reply as bytes init net.readBytes($c, 2);
		net.close($c);
		io.printf("%%s", convert.stringFromBytes($reply, "utf-8"));
	`, addr))
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if out != "hi" {
		t.Errorf("got %q, want %q", out, "hi")
	}
}

// TestConnectRejectsNegativeTimeout - a negative timeout is a catchable error,
// not a silent block.
func TestConnectRejectsNegativeTimeout(t *testing.T) {
	_, err := runProg(t, `use net; def c as net.Conn init net.connect("127.0.0.1:1", -1);`)
	if err == nil || !strings.Contains(err.Error(), "timeout must be >= 0") {
		t.Fatalf("expected negative-timeout error, got %v", err)
	}
}

// TestConnectTimeoutDoesNotBlockForever - dialing a non-routable address with a
// short timeout must return promptly with an error rather than hanging until the
// OS connect timeout (~minutes). 192.0.2.1 is TEST-NET-1 (RFC 5737, guaranteed
// non-routable); some environments return "unreachable"/"refused" instantly,
// which also satisfies the anti-hang property. Without the timeout a SYN-drop
// would block far past this bound.
func TestConnectTimeoutDoesNotBlockForever(t *testing.T) {
	start := time.Now()
	_, err := runProg(t, `use net; def c as net.Conn init net.connect("192.0.2.1:80", 400);`)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected a dial error to a blackhole address")
	}
	if elapsed > 8*time.Second {
		t.Fatalf("connect blocked %v despite a 400ms timeout", elapsed)
	}
}

// TestConnectTLSTreatsIntAsTimeout - net.connectTLS(addr, timeoutMs) must read a
// bare int second argument as the timeout, not misparse it as net.TLSOptions.
// The dial fails (no TLS server), but the error must be a dial/connect failure,
// proving the int reached the timeout slot rather than the options validator.
func TestConnectTLSTreatsIntAsTimeout(t *testing.T) {
	_, err := runProg(t, `use net; def c as net.Conn init net.connectTLS("127.0.0.1:1", 300);`)
	if err == nil {
		t.Fatal("expected a dial error (nothing is listening)")
	}
	if strings.Contains(err.Error(), "TLSOptions") {
		t.Fatalf("int timeout arg was misparsed as options: %v", err)
	}
	if !strings.Contains(err.Error(), "connectTLS") {
		t.Fatalf("expected a net.connectTLS error, got %v", err)
	}
}

// TestReadAllWholeBody proves net.readAll returns the entire stream to EOF in
// one call (the throughput path for a body / object download).
func TestReadAllWholeBody(t *testing.T) {
	addr := pickListenerAddr(t)
	l, err := stdnet.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("stdnet.Listen: %v", err)
	}
	defer l.Close()
	payload := strings.Repeat("abcdefghij", 5000) // 50 KB, several read chunks
	go func() {
		c, aErr := l.Accept()
		if aErr != nil {
			return
		}
		_, _ = c.Write([]byte(payload))
		c.Close() // EOF terminates readAll
	}()
	out, runErr := runProg(t, fmt.Sprintf(`
		use io; use net; use convert;
		def c as net.Conn init net.connect(%q);
		def body as bytes init net.readAll($c, 0, 5000);
		net.close($c);
		io.printf("%%d", len($body));
	`, addr))
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if out != fmt.Sprintf("%d", len(payload)) {
		t.Fatalf("readAll length = %s, want %d", out, len(payload))
	}
}

// TestReadAllCapRejects proves the maxBytes cap fails with a catchable error
// rather than an unbounded allocation.
func TestReadAllCapRejects(t *testing.T) {
	addr := pickListenerAddr(t)
	l, err := stdnet.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("stdnet.Listen: %v", err)
	}
	defer l.Close()
	go func() {
		c, aErr := l.Accept()
		if aErr != nil {
			return
		}
		_, _ = c.Write([]byte(strings.Repeat("x", 10000)))
		c.Close()
	}()
	_, runErr := runProg(t, fmt.Sprintf(`
		use net;
		def c as net.Conn init net.connect(%q);
		def body as bytes init net.readAll($c, 100, 5000);
	`, addr))
	if runErr == nil || !strings.Contains(runErr.Error(), "exceeds the 100-byte limit") {
		t.Fatalf("expected cap error, got %v", runErr)
	}
}

// TestReadNExact proves net.readN returns exactly n bytes and a short read
// (peer closed early) is a catchable error, not a truncated return.
func TestReadNExact(t *testing.T) {
	addr := pickListenerAddr(t)
	l, err := stdnet.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("stdnet.Listen: %v", err)
	}
	defer l.Close()
	go func() {
		c, aErr := l.Accept()
		if aErr != nil {
			return
		}
		_, _ = c.Write([]byte("ABCDEFGH"))
		c.Close()
	}()
	out, runErr := runProg(t, fmt.Sprintf(`
		use io; use net; use convert;
		def c as net.Conn init net.connect(%q);
		def five as bytes init net.readN($c, 5, 5000);
		def rejected as bool init false;
		try { net.readN($c, 100, 5000); } catch (e) { $rejected = true; }
		net.close($c);
		io.printf("%%s/%%t", convert.stringFromBytes($five, "utf-8"), $rejected);
	`, addr))
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if out != "ABCDE/true" {
		t.Fatalf("got %q, want ABCDE/true", out)
	}
}

// TestReadNCapRejectsHugeN proves net.readN refuses an oversized (wire-derived)
// length before allocating, matching readBytes' per-call cap - so an attacker
// controlling a length prefix cannot force an unbounded up-front allocation.
func TestReadNCapRejectsHugeN(t *testing.T) {
	addr := pickListenerAddr(t)
	l, err := stdnet.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("stdnet.Listen: %v", err)
	}
	defer l.Close()
	go func() {
		c, aErr := l.Accept()
		if aErr == nil {
			defer c.Close()
			var b [1]byte
			_, _ = c.Read(b[:]) // hold the connection open
		}
	}()
	_, runErr := runProg(t, fmt.Sprintf(`
		use net;
		def c as net.Conn init net.connect(%q);
		def b as bytes init net.readN($c, 999999999999);
	`, addr))
	if runErr == nil || !strings.Contains(runErr.Error(), "per-call limit") {
		t.Fatalf("expected per-call-limit error, got %v", runErr)
	}
}
