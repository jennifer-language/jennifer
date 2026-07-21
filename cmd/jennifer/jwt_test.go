// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestJwtAsymmetric drives the jwt module's RS256 / ES256 paths, which need PEM
// keys the pure-Jennifer overlay can't generate: the keys are minted here in Go,
// written as PEM, and the .j program reads them, signs, and verifies. HS* / EdDSA
// are covered by the overlay (modules/jwt_test.j).
func TestJwtAsymmetric(t *testing.T) {
	dir := t.TempDir()

	// RSA keypair -> PKCS#8 private + PKIX public PEM.
	rk, err := rsa.GenerateKey(crand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	writePEM(t, filepath.Join(dir, "rsa_priv.pem"), "PRIVATE KEY", mustMarshalPKCS8(t, rk))
	writePEM(t, filepath.Join(dir, "rsa_pub.pem"), "PUBLIC KEY", mustMarshalPKIX(t, &rk.PublicKey))

	// EC P-256 keypair -> SEC1 private + PKIX public PEM.
	ek, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecPriv, err := x509.MarshalECPrivateKey(ek)
	if err != nil {
		t.Fatal(err)
	}
	writePEM(t, filepath.Join(dir, "ec_priv.pem"), "EC PRIVATE KEY", ecPriv)
	writePEM(t, filepath.Join(dir, "ec_pub.pem"), "PUBLIC KEY", mustMarshalPKIX(t, &ek.PublicKey))

	jwtMod, err := filepath.Abs(filepath.Join("..", "..", "modules", "jwt.j"))
	if err != nil {
		t.Fatal(err)
	}
	prog := fmt.Sprintf(`use testing;
use json;
use fs;
import %q as jwt;

def claims as json.Value init json.decode("{\"sub\":\"ada\",\"scope\":\"read write\"}");

# RS256: sign with the RSA private key, verify with the public key.
def rpriv as bytes init fs.readBytes(%q);
def rpub as bytes init fs.readBytes(%q);
def rtok as string init jwt.sign($claims, $rpriv, "RS256");
testing.assertEqual(json.asString(jwt.verify($rtok, $rpub, "RS256"), "/sub"), "ada");
testing.assertEqual(json.asString(jwt.header($rtok), "/alg"), "RS256");

# A tampered RS256 token must not verify.
def rbad as bool init false;
try {
    def bad as string init strings.substring($rtok, 0, len($rtok) - 4) + "AAAA";
    jwt.verify($bad, $rpub, "RS256");
} catch (e) { $rbad = true; }
testing.assertTrue($rbad);

# ES256: sign with the EC private key, verify with the public key.
def epriv as bytes init fs.readBytes(%q);
def epub as bytes init fs.readBytes(%q);
def etok as string init jwt.sign($claims, $epriv, "ES256");
testing.assertEqual(json.asString(jwt.verify($etok, $epub, "ES256"), "/scope"), "read write");

# ES256 verified against the wrong (RSA) key family is rejected, not misused.
def emix as bool init false;
try { jwt.verify($etok, $rpub, "ES256"); } catch (e) { $emix = true; }
testing.assertTrue($emix);

use strings;`, jwtMod,
		filepath.Join(dir, "rsa_priv.pem"), filepath.Join(dir, "rsa_pub.pem"),
		filepath.Join(dir, "ec_priv.pem"), filepath.Join(dir, "ec_pub.pem"))
	progPath := filepath.Join(dir, "jwt.j")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := loadForTest(progPath); code != testExitPass {
		t.Fatalf("jwt program failed with code %d", code)
	}
}

func mustMarshalPKCS8(t *testing.T, k any) []byte {
	t.Helper()
	b, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustMarshalPKIX(t *testing.T, k any) []byte {
	t.Helper()
	b, err := x509.MarshalPKIXPublicKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func writePEM(t *testing.T, path, typ string, der []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}
