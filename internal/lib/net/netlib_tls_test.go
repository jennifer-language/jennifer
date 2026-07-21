// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

// Internal tests for net TLS (connectTLS / startTLS). Internal (package
// netlib) so they can inject a trusted root for a self-signed local
// server through the unexported testTLSConfig seam and call the Fn
// entry points directly. Uses only loopback sockets - no external
// network.

package netlib

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	stdnet "net"
	"strconv"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func ctx() interpreter.BuiltinCtx { return interpreter.BuiltinCtx{} }

// genCert makes a self-signed cert valid for "localhost" / 127.0.0.1 and
// a CertPool trusting it.
func genCert(t *testing.T) (tls.Certificate, *x509.CertPool, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"localhost"},
		IPAddresses:           []stdnet.IP{stdnet.ParseIP("127.0.0.1")},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool, caPEM
}

func trust(t *testing.T, pool *x509.CertPool) {
	testRootCAs = pool
	t.Cleanup(func() { testRootCAs = nil })
}

// tlsOpts builds a net.TLSOptions value.
func tlsOpts(skipVerify bool, caCert []byte) Value {
	return interpreter.NamespacedStructVal(LibraryName, "TLSOptions", []interpreter.StructField{
		{Name: "skipVerify", Value: interpreter.BoolVal(skipVerify)},
		{Name: "caCert", Value: interpreter.BytesVal(caCert)},
	})
}

// tlsEchoServer starts a TLS echo server on loopback; returns its port.
func tlsEchoServer(t *testing.T, cert tls.Certificate) int {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go echoOnce(c)
		}
	}()
	return portOf(ln.Addr())
}

// startTLSEchoServer starts a PLAINTEXT server that answers "STARTTLS\n"
// with "GO\n", upgrades to TLS, then echoes one message.
func startTLSEchoServer(t *testing.T, cert tls.Certificate) int {
	t.Helper()
	ln, err := stdnet.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c stdnet.Conn) {
				buf := make([]byte, 16)
				if _, err := c.Read(buf); err != nil { // read "STARTTLS\n"
					c.Close()
					return
				}
				c.Write([]byte("GO\n"))
				tc := tls.Server(c, &tls.Config{Certificates: []tls.Certificate{cert}})
				if err := tc.Handshake(); err != nil {
					tc.Close()
					return
				}
				echoOnce(tc)
			}(c)
		}
	}()
	return portOf(ln.Addr())
}

func echoOnce(c stdnet.Conn) {
	defer c.Close()
	buf := make([]byte, 256)
	n, err := c.Read(buf)
	if err != nil {
		return
	}
	c.Write(buf[:n])
}

func portOf(a stdnet.Addr) int {
	_, p, _ := stdnet.SplitHostPort(a.String())
	port, _ := strconv.Atoi(p)
	return port
}

func TestConnectTLSRoundTrip(t *testing.T) {
	ResetForTest()
	cert, pool, _ := genCert(t)
	port := tlsEchoServer(t, cert)
	trust(t, pool)

	conn, err := connectTLSFn(ctx(), []Value{interpreter.StringVal("localhost:" + strconv.Itoa(port))})
	if err != nil {
		t.Fatalf("connectTLS: %v", err)
	}
	if _, err := writeBytesFn(ctx(), []Value{conn, interpreter.BytesVal([]byte("ping"))}); err != nil {
		t.Fatal(err)
	}
	got, err := readBytesFn(ctx(), []Value{conn, interpreter.IntVal(64)})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Bytes) != "ping" {
		t.Errorf("echo = %q, want %q", got.Bytes, "ping")
	}
	if _, err := closeFn(ctx(), []Value{conn}); err != nil {
		t.Error(err)
	}
}

func TestConnectTLSRejectsUntrustedCert(t *testing.T) {
	ResetForTest()
	cert, _, _ := genCert(t)
	port := tlsEchoServer(t, cert)
	testRootCAs = nil // verify against system roots -> self-signed cert is untrusted

	_, err := connectTLSFn(ctx(), []Value{interpreter.StringVal("localhost:" + strconv.Itoa(port))})
	if err == nil {
		t.Fatal("expected a certificate-verification error for a self-signed cert")
	}
}

// skipVerify accepts an otherwise-untrusted (self-signed) cert.
func TestConnectTLSSkipVerify(t *testing.T) {
	ResetForTest()
	cert, _, _ := genCert(t)
	port := tlsEchoServer(t, cert)
	testRootCAs = nil // untrusted...

	conn, err := connectTLSFn(ctx(), []Value{
		interpreter.StringVal("localhost:" + strconv.Itoa(port)), tlsOpts(true, nil)})
	if err != nil {
		t.Fatalf("connectTLS with skipVerify: %v", err)
	}
	if _, err := writeBytesFn(ctx(), []Value{conn, interpreter.BytesVal([]byte("x"))}); err != nil {
		t.Fatal(err)
	}
	got, err := readBytesFn(ctx(), []Value{conn, interpreter.IntVal(8)})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Bytes) != "x" {
		t.Errorf("echo = %q, want %q", got.Bytes, "x")
	}
	closeFn(ctx(), []Value{conn})
}

// caCert trusts a specific self-signed certificate (the secure path).
func TestConnectTLSWithCACert(t *testing.T) {
	ResetForTest()
	cert, _, caPEM := genCert(t)
	port := tlsEchoServer(t, cert)
	testRootCAs = nil // trust via caCert, not the test seam

	conn, err := connectTLSFn(ctx(), []Value{
		interpreter.StringVal("localhost:" + strconv.Itoa(port)), tlsOpts(false, caPEM)})
	if err != nil {
		t.Fatalf("connectTLS with caCert: %v", err)
	}
	if _, err := writeBytesFn(ctx(), []Value{conn, interpreter.BytesVal([]byte("ca"))}); err != nil {
		t.Fatal(err)
	}
	got, err := readBytesFn(ctx(), []Value{conn, interpreter.IntVal(8)})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Bytes) != "ca" {
		t.Errorf("echo = %q, want %q", got.Bytes, "ca")
	}
	closeFn(ctx(), []Value{conn})
}

// An invalid caCert PEM errors before dialling.
func TestConnectTLSBadCACert(t *testing.T) {
	ResetForTest()
	_, err := connectTLSFn(ctx(), []Value{
		interpreter.StringVal("localhost:443"), tlsOpts(false, []byte("-----not a pem-----"))})
	if err == nil {
		t.Fatal("expected an error for an invalid caCert PEM")
	}
}

func TestStartTLSUpgradesInPlace(t *testing.T) {
	ResetForTest()
	cert, pool, _ := genCert(t)
	port := startTLSEchoServer(t, cert)
	trust(t, pool)

	conn, err := connectFn(ctx(), []Value{interpreter.StringVal("localhost:" + strconv.Itoa(port))})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	// plaintext STARTTLS negotiation
	if _, err := writeBytesFn(ctx(), []Value{conn, interpreter.BytesVal([]byte("STARTTLS\n"))}); err != nil {
		t.Fatal(err)
	}
	ack, err := readBytesFn(ctx(), []Value{conn, interpreter.IntVal(8)})
	if err != nil {
		t.Fatal(err)
	}
	if string(ack.Bytes) != "GO\n" {
		t.Fatalf("ack = %q, want %q", ack.Bytes, "GO\n")
	}
	// upgrade in place
	up, err := startTLSFn(ctx(), []Value{conn})
	if err != nil {
		t.Fatalf("startTLS: %v", err)
	}
	// same handle id survives the upgrade
	if up.Fields[0].Value.Int != conn.Fields[0].Value.Int {
		t.Errorf("startTLS changed the handle id: %d -> %d", conn.Fields[0].Value.Int, up.Fields[0].Value.Int)
	}
	// now encrypted
	if _, err := writeBytesFn(ctx(), []Value{up, interpreter.BytesVal([]byte("secret"))}); err != nil {
		t.Fatal(err)
	}
	got, err := readBytesFn(ctx(), []Value{up, interpreter.IntVal(64)})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Bytes) != "secret" {
		t.Errorf("tls echo = %q, want %q", got.Bytes, "secret")
	}
	closeFn(ctx(), []Value{up})
}

func TestTLSArgErrors(t *testing.T) {
	ResetForTest()
	// arity / kind
	if _, err := connectTLSFn(ctx(), []Value{interpreter.StringVal("no-port")}); err == nil {
		t.Error("connectTLS: address without a port should error")
	}
	if _, err := connectTLSFn(ctx(), []Value{interpreter.IntVal(1)}); err == nil {
		t.Error("connectTLS: non-string address should error")
	}
	if _, err := startTLSFn(ctx(), []Value{interpreter.IntVal(1)}); err == nil {
		t.Error("startTLS: non-Conn argument should error")
	}
}
