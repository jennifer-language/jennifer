// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	stdasn1 "encoding/asn1"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// berTLV wraps content in a tag/length/value with a short-form length (all the
// fields this fake agent emits are well under 128 bytes).
func berTLV(tag byte, content []byte) []byte {
	return append([]byte{tag, byte(len(content))}, content...)
}

// berInt encodes a non-negative int as a minimal DER INTEGER.
func berInt(n int) []byte {
	if n == 0 {
		return berTLV(0x02, []byte{0})
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte(n & 0xff)}, b...)
		n >>= 8
	}
	if b[0]&0x80 != 0 { // keep the value positive
		b = append([]byte{0}, b...)
	}
	return berTLV(0x02, b)
}

// fakeSnmpResponse parses an SNMP request enough to echo its request-id, then
// builds a GetResponse binding sysDescr.0 to a canned octet string.
func fakeSnmpResponse(req []byte) ([]byte, error) {
	var msg struct {
		Version   int
		Community []byte
		PDU       stdasn1.RawValue
	}
	if _, err := stdasn1.Unmarshal(req, &msg); err != nil {
		return nil, err
	}
	// The PDU is [context] IMPLICIT SEQUENCE, so its content starts with the
	// request-id INTEGER.
	var reqID int
	if _, err := stdasn1.Unmarshal(msg.PDU.Bytes, &reqID); err != nil {
		return nil, err
	}

	oid := []byte{0x06, 0x08, 0x2b, 0x06, 0x01, 0x02, 0x01, 0x01, 0x01, 0x00} // 1.3.6.1.2.1.1.1.0
	val := berTLV(0x04, []byte("test agent"))
	vb := berTLV(0x30, append(oid, val...))
	vblist := berTLV(0x30, vb)

	pduContent := berInt(reqID)
	pduContent = append(pduContent, berInt(0)...) // error-status
	pduContent = append(pduContent, berInt(0)...) // error-index
	pduContent = append(pduContent, vblist...)
	pdu := berTLV(0xa2, pduContent) // [context 2] constructed = GetResponse

	body := berInt(msg.Version)
	body = append(body, berTLV(0x04, msg.Community)...)
	body = append(body, pdu...)
	return berTLV(0x30, body), nil
}

// TestSnmpGet drives snmp.get against a fake UDP agent and checks the decoded
// binding.
func TestSnmpGet(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	addr := pc.LocalAddr().String()

	go func() {
		buf := make([]byte, 65535)
		n, peer, rerr := pc.ReadFrom(buf)
		if rerr != nil {
			return
		}
		resp, berr := fakeSnmpResponse(buf[:n])
		if berr != nil {
			return
		}
		_, _ = pc.WriteTo(resp, peer)
	}()

	snmpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "snmp.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`import %q as snmp;
use testing;
def const ADDR as string init %q;

func testGet() {
    def c as snmp.Client init snmp.clientWith(ADDR, "public", snmp.VERSION2C, 2000, 0);
    def vbs as list of snmp.Varbind init snmp.get($c, ["1.3.6.1.2.1.1.1.0"]);
    testing.assertEqual(len($vbs), 1);
    testing.assertEqual($vbs[0].oid, "1.3.6.1.2.1.1.1.0");
    testing.assertEqual($vbs[0].type, "octetString");
    testing.assertEqual($vbs[0].value, "test agent");
}
`, snmpMod, addr)
	progPath := filepath.Join(dir, "prog.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	in, code := loadForTest(progPath)
	if in == nil || code != testExitPass {
		t.Fatalf("loadForTest failed: code %d", code)
	}
	if _, err := in.CallByName("testGet"); err != nil {
		t.Errorf("testGet failed: %v", err)
	}
}

// TestSnmpAgent drives the Jennifer SNMP client against the Jennifer SNMP agent
// (server) in one program over loopback UDP: bind, spawn serveOn, GET + walk,
// then discard. A timeout guard fails fast if the exchange hangs.
func TestSnmpAgent(t *testing.T) {
	snmpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "snmp.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`import %q as snmp;
use net;
use task;
use channel;
use testing;

func testAgentRoundTrip() {
    def a as snmp.Agent init snmp.agent("public", snmp.VERSION2C, [
        snmp.stringVar("1.3.6.1.2.1.1.1.0", "unit-test agent"),
        snmp.varbind("1.3.6.1.2.1.1.3.0", "timeTicks", "", 4200),
        snmp.intVar("1.3.6.1.2.1.1.7.0", 72)
    ]);
    def sock as net.UDPSocket init net.listenUDP("127.0.0.1:0");
    def addr as string init net.address($sock);
    def stop as channel of bool init channel.make(1);
    def server as task of null init spawn { snmp.serveOn($a, $sock, $stop); };

    def c as snmp.Client init snmp.clientWith($addr, "public", snmp.VERSION2C, 2000, 3);
    def vbs as list of snmp.Varbind init snmp.get($c, ["1.3.6.1.2.1.1.1.0"]);
    testing.assertEqual($vbs[0].value, "unit-test agent");

    def rows as list of snmp.Varbind init snmp.walk($c, "1.3.6.1.2.1.1");
    testing.assertEqual(len($rows), 3);
    testing.assertEqual($rows[2].number, 72);

    # Graceful shutdown: signal, then join.
    channel.send($stop, true);
    task.wait($server);
}
`, snmpMod)
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
		if _, err := in.CallByName("testAgentRoundTrip"); err != nil {
			t.Errorf("testAgentRoundTrip failed: %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("client<->agent round-trip hung")
	}
}

// TestSnmpWalkTerminates confirms a client walk stops when the agent does not
// advance the OID (a buggy or hostile agent), rather than looping to MAX_WALK.
// The fake agent always answers with the same OID, so walk must return exactly
// the one binding and then stop.
func TestSnmpWalkTerminates(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	addr := pc.LocalAddr().String()

	// A misbehaving agent: always replies with the same OID.
	go func() {
		buf := make([]byte, 65535)
		for {
			n, peer, rerr := pc.ReadFrom(buf)
			if rerr != nil {
				return
			}
			resp, berr := fakeSnmpResponse(buf[:n])
			if berr != nil {
				continue
			}
			_, _ = pc.WriteTo(resp, peer)
		}
	}()

	snmpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "snmp.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`import %q as snmp;
use testing;
def const ADDR as string init %q;

func testWalkTerminates() {
    def c as snmp.Client init snmp.clientWith(ADDR, "public", snmp.VERSION2C, 2000, 2);
    def rows as list of snmp.Varbind init snmp.walk($c, "1.3.6.1.2.1.1");
    testing.assertEqual(len($rows), 1);
}
`, snmpMod, addr)
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
		if _, err := in.CallByName("testWalkTerminates"); err != nil {
			t.Errorf("testWalkTerminates failed: %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("walk against a non-advancing agent did not terminate")
	}
}

// TestSnmpBadAddress confirms a client transport failure (an unresolvable
// address on sendTo) surfaces as an snmp-kind Error, honouring the documented
// contract that every client failure is kind "snmp".
func TestSnmpBadAddress(t *testing.T) {
	snmpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "snmp.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`import %q as snmp;
use testing;

func testBadAddress() {
    def c as snmp.Client init snmp.clientWith("nonexistent.invalid:161", "public", snmp.VERSION2C, 500, 0);
    def kind as string init "none";
    try {
        snmp.get($c, ["1.3.6.1.2.1.1.1.0"]);
    } catch (e) {
        $kind = $e.kind;
    }
    testing.assertEqual($kind, "snmp");
}
`, snmpMod)
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
		if _, err := in.CallByName("testBadAddress"); err != nil {
			t.Errorf("testBadAddress failed: %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("snmp.get on a bad address hung")
	}
}

// TestSnmpTimeout confirms a query against a silent agent throws (kind "snmp")
// rather than hanging, thanks to the UDP receive deadline.
func TestSnmpTimeout(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	addr := pc.LocalAddr().String()

	snmpMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "snmp.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`import %q as snmp;
use testing;
def const ADDR as string init %q;

func testTimeout() {
    def c as snmp.Client init snmp.clientWith(ADDR, "public", snmp.VERSION2C, 300, 1);
    def threw as bool init false;
    try {
        snmp.get($c, ["1.3.6.1.2.1.1.1.0"]);
    } catch (e) {
        $threw = true;
    }
    testing.assertTrue($threw);
}
`, snmpMod, addr)
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
		t.Fatal("snmp.get did not time out (UDP receive deadline not honoured)")
	}
}
