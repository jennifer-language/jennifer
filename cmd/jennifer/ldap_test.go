// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLdapDirectory drives the Jennifer LDAP client against the Jennifer LDAP
// directory server in one program over loopback TCP: build a directory, spawn
// serveOn, then bind (ok + bad), search with filters, follow a group, mutate
// the live directory (add a user) and authenticate as the new user. A timeout
// guard fails fast if the exchange hangs.
func TestLdapDirectory(t *testing.T) {
	ldapMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "ldap.j"))
	if err != nil {
		t.Fatal(err)
	}
	transportMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "transport.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`import %q as ldap;
import %q as transport;
use net;
use task;
use testing;

func testDirectoryRoundTrip() {
    def d as ldap.Directory init ldap.directory([
        ldap.entry("uid=alice,ou=people,dc=example,dc=org", {
            "objectClass": ["inetOrgPerson", "person"], "uid": ["alice"],
            "cn": ["Alice"], "mail": ["alice@example.org"],
            "userPassword": [ldap.password("secret", "ssha")]}),
        ldap.group("cn=admins,ou=groups,dc=example,dc=org", ["uid=alice,ou=people,dc=example,dc=org"])
    ]);
    def listener as net.Listener init ldap.listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    def srv as task of null init spawn { ldap.serveOn($d, $listener); };

    def c as ldap.Conn init ldap.connect($addr, transport.Security.None);

    # simple bind: good and bad credentials
    testing.assertEqual(ldap.bind($c, "uid=alice,ou=people,dc=example,dc=org", "secret").code, 0);
    testing.assertEqual(ldap.bind($c, "uid=alice,ou=people,dc=example,dc=org", "wrong").code, 49);

    # search with a compound filter
    def hits as list of ldap.Entry init ldap.search($c, "ou=people,dc=example,dc=org",
        ldap.SCOPE_SUB, ldap.parseFilter("(&(objectClass=person)(uid=alice))"), ["cn", "mail"]);
    testing.assertEqual(len($hits), 1);
    testing.assertEqual(ldap.firstValue($hits[0], "mail"), "alice@example.org");
    # userPassword is withheld unless requested
    testing.assertEqual(len(ldap.values($hits[0], "userPassword")), 0);

    # group-membership search
    def groups as list of ldap.Entry init ldap.search($c, "ou=groups,dc=example,dc=org",
        ldap.SCOPE_SUB, ldap.parseFilter("(member=uid=alice,ou=people,dc=example,dc=org)"), ["cn"]);
    testing.assertEqual(len($groups), 1);

    # live mutation from the admin side, then authenticate as the new user
    ldap.addEntry($d, ldap.entry("uid=bob,ou=people,dc=example,dc=org", {
        "objectClass": ["inetOrgPerson"], "uid": ["bob"],
        "userPassword": [ldap.password("bobpw", "ssha256")]}));
    testing.assertEqual(ldap.bind($c, "uid=bob,ou=people,dc=example,dc=org", "bobpw").code, 0);
    testing.assertEqual(len(ldap.search($c, "ou=people,dc=example,dc=org",
        ldap.SCOPE_ONE, ldap.parseFilter("(objectClass=*)"), ["uid"])), 2);

    ldap.unbind($c);
    net.close($listener);
    task.wait($srv);
}
`, ldapMod, transportMod)
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
		if _, err := in.CallByName("testDirectoryRoundTrip"); err != nil {
			t.Errorf("testDirectoryRoundTrip failed: %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("client<->server round-trip hung")
	}
}

// TestLdapWriteOps drives the Jennifer LDAP client's write operations (add,
// modify, delete, modifyDN, and the RFC 3062 password-modify extended op)
// against a minimal Go fake server. The Jennifer directory server is read-only
// over LDAP, so writes are exercised here: the fake reads each request off the
// wire (verifying the client's BER framing) and answers a success LDAPResult.
func TestLdapWriteOps(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	addr := ln.Addr().String()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			msgID, opTag, rerr := ldapReadMessage(conn)
			if rerr != nil {
				return
			}
			if opTag == 0x42 { // UnbindRequest [APPLICATION 2] primitive
				return
			}
			respTag, ok := ldapRespTag(opTag)
			if !ok {
				return
			}
			if _, werr := conn.Write(ldapResult(msgID, respTag, 0)); werr != nil {
				return
			}
		}
	}()

	ldapMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "ldap.j"))
	if err != nil {
		t.Fatal(err)
	}
	transportMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "transport.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`import %q as ldap;
import %q as transport;
use net;
use testing;

func testWriteOps() {
    def c as ldap.Conn init ldap.connect(%q, transport.Security.None);
    testing.assertEqual(ldap.bind($c, "cn=admin,dc=example,dc=org", "pw").code, 0);
    testing.assertEqual(ldap.add($c, "uid=new,ou=people,dc=example,dc=org", {
        "objectClass": ["inetOrgPerson"], "uid": ["new"], "cn": ["New User"]}).code, 0);
    testing.assertEqual(ldap.modify($c, "uid=new,ou=people,dc=example,dc=org", [
        ldap.change(ldap.MOD_REPLACE, "mail", ["new@example.org"]),
        ldap.change(ldap.MOD_ADD, "description", ["created by test"])]).code, 0);
    testing.assertEqual(ldap.modifyDn($c, "uid=new,ou=people,dc=example,dc=org", "uid=renamed", true, "").code, 0);
    testing.assertEqual(ldap.passwordModify($c, "uid=renamed,ou=people,dc=example,dc=org", "old", "newpw").code, 0);
    testing.assertEqual(ldap.delete($c, "uid=renamed,ou=people,dc=example,dc=org").code, 0);
    ldap.unbind($c);
}
`, ldapMod, transportMod, addr)
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
		if _, err := in.CallByName("testWriteOps"); err != nil {
			t.Errorf("testWriteOps failed: %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("write-op round-trip hung")
	}
}

// TestLdapServerRobustness fires several malformed requests at the Jennifer
// directory server (each a well-framed BER LDAPMessage whose bind op is an empty
// sequence, so the handler throws reading the dn) and confirms the server keeps
// serving: a fault-cycling client must not drop the connection silently, leak
// the socket, or stop the accept loop (audit F1).
func TestLdapServerRobustness(t *testing.T) {
	ldapMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "ldap.j"))
	if err != nil {
		t.Fatal(err)
	}
	transportMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "transport.j"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := fmt.Sprintf(`import %q as ldap;
import %q as transport;
use net;
use asn1;
use task;
use testing;

func testServerSurvivesMalformed() {
    def d as ldap.Directory init ldap.directory([
        ldap.entry("uid=alice,ou=people,dc=example,dc=org", {
            "objectClass": ["inetOrgPerson"], "uid": ["alice"],
            "userPassword": [ldap.password("secret", "ssha")]})
    ]);
    def listener as net.Listener init ldap.listen("127.0.0.1:0");
    def addr as string init net.address($listener);
    def srv as task of null init spawn { ldap.serveOn($d, $listener); };

    # a well-framed LDAPMessage whose bind op is an empty sequence: the server
    # reads it fine, then throws reading the (missing) dn inside the handler.
    def bad as bytes init asn1.encode(asn1.sequence([
        asn1.integer(1),
        asn1.retag("application", 0, asn1.sequence([]))
    ]));
    def i as int init 0;
    while ($i < 5) {
        def raw as net.Conn init net.connect($addr);
        net.writeBytes($raw, $bad);
        net.close($raw);
        $i = $i + 1;
    }

    # after all those faults the server must still answer a valid client
    def c as ldap.Conn init ldap.connect($addr, transport.Security.None);
    testing.assertEqual(ldap.bind($c, "uid=alice,ou=people,dc=example,dc=org", "secret").code, 0);
    testing.assertEqual(ldap.bind($c, "uid=alice,ou=people,dc=example,dc=org", "wrong").code, 49);
    ldap.unbind($c);
    net.close($listener);
    task.wait($srv);
}
`, ldapMod, transportMod)
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
		if _, err := in.CallByName("testServerSurvivesMalformed"); err != nil {
			t.Errorf("testServerSurvivesMalformed failed: %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("server robustness round-trip hung")
	}
}

// ldapReadMessage reads one BER LDAPMessage off conn and returns its messageID
// and the protocolOp tag byte.
func ldapReadMessage(conn net.Conn) (msgID int, opTag byte, err error) {
	hdr := make([]byte, 2)
	if _, err = io.ReadFull(conn, hdr); err != nil {
		return
	}
	length := int(hdr[1])
	if length >= 0x80 {
		n := length & 0x7f
		lb := make([]byte, n)
		if _, err = io.ReadFull(conn, lb); err != nil {
			return
		}
		length = 0
		for _, b := range lb {
			length = length<<8 | int(b)
		}
	}
	content := make([]byte, length)
	if _, err = io.ReadFull(conn, content); err != nil {
		return
	}
	if len(content) < 2 || content[0] != 0x02 { // messageID INTEGER
		err = fmt.Errorf("ldap fake: malformed messageID")
		return
	}
	idLen := int(content[1])
	pos := 2
	for i := 0; i < idLen && pos+i < len(content); i++ {
		msgID = msgID<<8 | int(content[pos+i])
	}
	pos += idLen
	if pos >= len(content) {
		err = fmt.Errorf("ldap fake: missing protocolOp")
		return
	}
	opTag = content[pos]
	return
}

// ldapRespTag maps a request protocolOp tag to its response tag.
func ldapRespTag(reqTag byte) (byte, bool) {
	switch reqTag {
	case 0x60: // BindRequest [APPLICATION 0]
		return 0x61, true
	case 0x68: // AddRequest [APPLICATION 8]
		return 0x69, true
	case 0x66: // ModifyRequest [APPLICATION 6]
		return 0x67, true
	case 0x4a: // DelRequest [APPLICATION 10] primitive
		return 0x6b, true
	case 0x6c: // ModifyDNRequest [APPLICATION 12]
		return 0x6d, true
	case 0x77: // ExtendedRequest [APPLICATION 23]
		return 0x78, true
	default:
		return 0, false
	}
}

// ldapResult builds an LDAPMessage carrying an LDAPResult (resultCode, empty
// matchedDN, empty diagnosticMessage) under the given response tag.
func ldapResult(msgID int, respTag byte, code int) []byte {
	result := append(ldapBerTLV(0x0a, []byte{byte(code)}), ldapBerTLV(0x04, nil)...)
	result = append(result, ldapBerTLV(0x04, nil)...)
	op := ldapBerTLV(respTag, result)
	msg := append(ldapBerTLV(0x02, ldapMinInt(msgID)), op...)
	return ldapBerTLV(0x30, msg)
}

// ldapBerTLV wraps content in tag + short-form length (fake responses are tiny).
func ldapBerTLV(tag byte, content []byte) []byte {
	return append([]byte{tag, byte(len(content))}, content...)
}

// ldapMinInt encodes a non-negative int as minimal big-endian INTEGER content.
func ldapMinInt(n int) []byte {
	if n == 0 {
		return []byte{0}
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte(n & 0xff)}, b...)
		n >>= 8
	}
	if b[0]&0x80 != 0 {
		b = append([]byte{0}, b...)
	}
	return b
}
