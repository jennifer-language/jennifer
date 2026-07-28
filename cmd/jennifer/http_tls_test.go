// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	stdnet "net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// selfSignedPEM mints a self-signed cert (valid for localhost / 127.0.0.1) and
// returns its certificate and private key as PEM bytes.
func selfSignedPEM(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
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
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

// TestHttpRestTLSClientAgainstSelfSigned drives the http and rest client TLS
// surface against a self-signed loopback server: the default request refuses
// the untrusted cert (a throw), skipVerify accepts any cert, and withCA trusts
// the server's own cert while still authenticating it.
func TestHttpRestTLSClientAgainstSelfSigned(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})

	httpMod, _ := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))
	restMod, _ := filepath.Abs(filepath.Join("..", "..", "modules", "rest.j"))

	prog := fmt.Sprintf(`use testing;
use convert;
import %q as http;
import %q as rest;
def url as string init %q;
def base as string init %q;
def pem as bytes init convert.bytesFromString(%q, "utf-8");

# --- http surface ---
# default verification refuses the self-signed cert
def threw as bool init false;
try { def a as http.Response init http.request("GET", $url, {}, ""); } catch (e) { $threw = true; }
testing.assertEqual($threw, true);
# skipVerify reaches it
def sv as http.TlsOptions;
$sv.skipVerify = true;
def b as http.Response init http.requestTls("GET", $url, {}, "", $sv);
testing.assertEqual($b.status, 200);
testing.assertEqual($b.body, "ok");
# withCA trusts the self-signed cert (still authenticated against 127.0.0.1)
def ca as http.TlsOptions;
$ca.caCert = $pem;
def c as http.Response init http.requestTls("GET", $url, {}, "", $ca);
testing.assertEqual($c.status, 200);

# --- rest surface ---
def cli as rest.Client init rest.client($base);
def rthrew as bool init false;
try { def d as rest.Response init rest.get($cli, "/", {}); } catch (e) { $rthrew = true; }
testing.assertEqual($rthrew, true);
def open as rest.Response init rest.get(rest.insecure($cli), "/", {});
testing.assertEqual($open.status, 200);
def pinned as rest.Response init rest.get(rest.withCA($cli, $pem), "/", {});
testing.assertEqual($pinned.status, 200);
testing.assertEqual($pinned.body, "ok");`,
		httpMod, restMod, srv.URL+"/", srv.URL, string(certPEM))

	dir := t.TempDir()
	progPath := filepath.Join(dir, "tls_client.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("http/rest TLS client program failed with code %d", code)
	}
}

// TestHttpdListenTLSRoundTrip proves the full Jennifer TLS stack: a program
// spins a self-signed httpd.listenTLS loopback in a spawn and the http client
// (skipVerify) reaches it.
func TestHttpdListenTLSRoundTrip(t *testing.T) {
	certPEM, keyPEM := selfSignedPEM(t)
	httpMod, _ := filepath.Abs(filepath.Join("..", "..", "modules", "http.j"))

	prog := fmt.Sprintf(`use testing;
use convert;
use httpd;
use task;
import %q as http;
def cert as bytes init convert.bytesFromString(%q, "utf-8");
def key as bytes init convert.bytesFromString(%q, "utf-8");
def srv as httpd.Server init httpd.listenTLS("127.0.0.1:0", $cert, $key);
def addr as string init httpd.address($srv);
def t as task of null init spawn {
    def req as httpd.Request init httpd.accept($srv);
    httpd.respond($req, 200, "hi\n");
    return;
};
def o as http.TlsOptions;
$o.skipVerify = true;
def r as http.Response init http.requestTls("GET", "https://" + $addr + "/", {}, "", $o);
testing.assertEqual($r.status, 200);
testing.assertContains($r.body, "hi");
task.wait($t);`,
		httpMod, string(certPEM), string(keyPEM))

	dir := t.TempDir()
	progPath := filepath.Join(dir, "tls_server.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("httpd.listenTLS round-trip program failed with code %d", code)
	}
}
