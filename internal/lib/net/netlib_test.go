// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

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

	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/lib/convert"
	fslib "github.com/mplx/jennifer-lang/internal/lib/fs"
	iolib "github.com/mplx/jennifer-lang/internal/lib/io"
	netlib "github.com/mplx/jennifer-lang/internal/lib/net"
	tasklib "github.com/mplx/jennifer-lang/internal/lib/task"
	"github.com/mplx/jennifer-lang/internal/parser"
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
