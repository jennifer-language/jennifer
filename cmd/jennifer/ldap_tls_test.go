// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// privateCA mints a CA and a server certificate it signs for 127.0.0.1. It
// returns the CA certificate as PEM (what a client trusts) and the server's
// certificate chain with key (what the server presents). The server
// certificate is not self-signed, so trusting it needs the CA itself.
func privateCA(t *testing.T, name string) (caPEM []byte, server tls.Certificate) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "ldap.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	server = tls.Certificate{Certificate: [][]byte{leafDER}, PrivateKey: leafKey}
	return caPEM, server
}

// ldapTLSFake answers one LDAP connection: success to every bind, and to a
// StartTLS request when starttls is set, after which it continues over TLS.
// It stops at an unbind, a read error or a failed handshake.
func ldapTLSFake(conn net.Conn, cfg *tls.Config, starttls bool) {
	defer func() { conn.Close() }()
	for {
		msgID, opTag, err := ldapReadMessage(conn)
		if err != nil || opTag == 0x42 { // UnbindRequest
			return
		}
		switch {
		case opTag == 0x77 && starttls: // ExtendedRequest: StartTLS
			if _, err := conn.Write(ldapResult(msgID, 0x78, 0)); err != nil {
				return
			}
			conn = tls.Server(conn, cfg)
			starttls = false
		case opTag == 0x60: // BindRequest
			if _, err := conn.Write(ldapResult(msgID, 0x61, 0)); err != nil {
				return
			}
		default:
			return
		}
	}
}

// ldapServeFake accepts connections on ln until it is closed.
func ldapServeFake(ln net.Listener, cfg *tls.Config, starttls bool) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go ldapTLSFake(conn, cfg, starttls)
	}
}

// TestLdapPrivateCA proves connectWith and startTlsWith hand their TLS options
// to a real handshake: against a server whose certificate a private CA signed,
// LDAPS and StartTLS refuse by default (system roots) and with an unrelated CA,
// and bind over TLS once `caCert` names the server's CA.
func TestLdapPrivateCA(t *testing.T) {
	caPEM, serverCert := privateCA(t, "Test Private CA")
	otherPEM, _ := privateCA(t, "Unrelated CA")
	cfg := &tls.Config{Certificates: []tls.Certificate{serverCert}}

	ldaps, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ldaps.Close()
	go ldapServeFake(ldaps, nil, false)

	plain, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	go ldapServeFake(plain, cfg, true)

	ldapMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "ldap.j"))
	if err != nil {
		t.Fatal(err)
	}
	transportMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "transport.j"))
	if err != nil {
		t.Fatal(err)
	}
	prog := fmt.Sprintf(`import %q as ldap;
import %q as transport;
use convert;
use encoding;
use net;
use testing;

def ldaps as string init %q;
def plain as string init %q;
def ca as net.TLSOptions;
$ca.caCert = encoding.encode("utf-8", %q);
def other as net.TLSOptions;
$other.caCert = encoding.encode("utf-8", %q);
def none as net.TLSOptions;

# the kind a refused connection throws, "" if it connected. The failed
# handshake is net's error; the same address connects with the right CA, so
# "net" here is the certificate check.
func refused(address as string, security as transport.Security, tls as net.TLSOptions) {
    try {
        def c as ldap.Conn init ldap.connectWith($address, $security, $tls);
        ldap.close($c);
    } catch (e) {
        return $e.kind;
    }
    return "";
}

func binds(c as ldap.Conn) {
    # no allowInsecureAuth: a password is only sent over an encrypted Conn
    def code as int init ldap.bind($c, "cn=admin,dc=example,dc=org", "pw").code;
    ldap.unbind($c);
    return $code;
}

func testPrivateCA() {
    # LDAPS
    testing.assertEqual(refused($ldaps, transport.Security.Tls, $none), "net");
    testing.assertEqual(refused($ldaps, transport.Security.Tls, $other), "net");
    testing.assertEqual(binds(ldap.connectWith($ldaps, transport.Security.Tls, $ca)), 0);
    # StartTLS through connectWith
    testing.assertEqual(refused($plain, transport.Security.Starttls, $none), "net");
    testing.assertEqual(refused($plain, transport.Security.Starttls, $other), "net");
    def s as ldap.Conn init ldap.connectWith($plain, transport.Security.Starttls, $ca);
    testing.assertEqual($s.security, transport.Security.Starttls);
    testing.assertEqual(binds($s), 0);
    # StartTLS on an open plaintext Conn
    def p as ldap.Conn init ldap.connect($plain, transport.Security.None);
    testing.assertEqual(binds(ldap.startTlsWith($p, $ca)), 0);
}
`, ldapMod, transportMod, ldaps.Addr().String(), plain.Addr().String(), string(caPEM), string(otherPEM))
	progPath := filepath.Join(t.TempDir(), "prog.j")
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
		if _, err := in.CallByName("testPrivateCA"); err != nil {
			t.Errorf("testPrivateCA failed: %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("private-CA round-trip hung")
	}
}
