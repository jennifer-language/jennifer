// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package cryptolib

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// rsaKeyPEMs generates a fresh RSA key and returns its private (PKCS#8) and
// public (PKIX) PEM encodings.
func rsaKeyPEMs(t *testing.T) ([]byte, []byte) {
	t.Helper()
	k, err := rsa.GenerateKey(crand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: priv}),
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub})
}

// ecKeyPEMs generates a fresh P-256 key and returns its private (SEC1) and
// public (PKIX) PEM encodings.
func ecKeyPEMs(t *testing.T) ([]byte, []byte) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := x509.MarshalECPrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: priv}),
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub})
}

func mustBytes(t *testing.T, v interpreter.Value, err error) []byte {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Kind != interpreter.KindBytes {
		t.Fatalf("expected bytes, got kind %v", v.Kind)
	}
	return v.Bytes
}

func TestRSASignVerifyRoundTrip(t *testing.T) {
	priv, pub := rsaKeyPEMs(t)
	msg := []byte("the quick brown fox")
	for _, algo := range []string{"sha256", "sha384", "sha512"} {
		sv, se := rsaSignFn(noCtx, []interpreter.Value{
			interpreter.StringVal(algo), bytesArg(msg), bytesArg(priv)})
		sig := mustBytes(t, sv, se)
		if len(sig) != 256 { // 2048-bit key -> 256-byte signature
			t.Errorf("%s: signature length %d, want 256", algo, len(sig))
		}
		v, err := rsaVerifyFn(noCtx, []interpreter.Value{
			interpreter.StringVal(algo), bytesArg(msg), bytesArg(sig), bytesArg(pub)})
		if err != nil || !v.Bool {
			t.Errorf("%s: verify genuine = %v (err %v)", algo, v.Bool, err)
		}
		// A tampered message must not verify.
		bad, _ := rsaVerifyFn(noCtx, []interpreter.Value{
			interpreter.StringVal(algo), bytesArg([]byte("tampered")), bytesArg(sig), bytesArg(pub)})
		if bad.Bool {
			t.Errorf("%s: tampered message verified", algo)
		}
	}
}

func TestECDSASignVerifyRoundTrip(t *testing.T) {
	priv, pub := ecKeyPEMs(t)
	msg := []byte("the quick brown fox")
	sv, se := ecdsaSignFn(noCtx, []interpreter.Value{
		interpreter.StringVal("sha256"), bytesArg(msg), bytesArg(priv)})
	sig := mustBytes(t, sv, se)
	if len(sig) != 64 { // P-256 JOSE signature is 32+32
		t.Fatalf("ES256 signature length %d, want 64", len(sig))
	}
	v, err := ecdsaVerifyFn(noCtx, []interpreter.Value{
		interpreter.StringVal("sha256"), bytesArg(msg), bytesArg(sig), bytesArg(pub)})
	if err != nil || !v.Bool {
		t.Errorf("ES256 verify genuine = %v (err %v)", v.Bool, err)
	}
	bad, _ := ecdsaVerifyFn(noCtx, []interpreter.Value{
		interpreter.StringVal("sha256"), bytesArg([]byte("tampered")), bytesArg(sig), bytesArg(pub)})
	if bad.Bool {
		t.Error("ES256 tampered message verified")
	}
	// A signature of the wrong length is not valid (must be false, not a panic).
	short, err := ecdsaVerifyFn(noCtx, []interpreter.Value{
		interpreter.StringVal("sha256"), bytesArg(msg), bytesArg([]byte{1, 2, 3}), bytesArg(pub)})
	if err != nil || short.Bool {
		t.Errorf("wrong-length signature: got %v (err %v), want false", short.Bool, err)
	}
}

// TestECDSAAllCurves pins sign / verify across all three JOSE ES curves with
// their spec-paired hashes (ES256/P-256/SHA-256, ES384/P-384/SHA-384,
// ES512/P-521/SHA-512). The P-384 / P-521 paths carried zero coverage before,
// yet they are exactly where a JWS alg-binding bug hides: a coordinate-width or
// digest mismatch round-trips wrong. Signatures are the fixed-width JOSE R||S
// form (2*ceil(bits/8)), and a wrong-hash verify must be a plain false, never a
// panic.
func TestECDSAAllCurves(t *testing.T) {
	cases := []struct {
		curve, algo string
		sigLen      int
	}{
		{"p256", "sha256", 64},
		{"p384", "sha384", 96},
		{"p521", "sha512", 132},
	}
	msg := []byte("the quick brown fox")
	for _, c := range cases {
		kv, err := ecGenerateKeyFn(noCtx, []interpreter.Value{interpreter.StringVal(c.curve)})
		if err != nil {
			t.Fatalf("%s: generate: %v", c.curve, err)
		}
		// Derive a PKIX public PEM from the generated SEC1 private PEM.
		block, _ := pem.Decode(kv.Bytes)
		ek, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			t.Fatalf("%s: parse: %v", c.curve, err)
		}
		pubDER, _ := x509.MarshalPKIXPublicKey(&ek.PublicKey)
		pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

		sv, se := ecdsaSignFn(noCtx, []interpreter.Value{
			interpreter.StringVal(c.algo), bytesArg(msg), bytesArg(kv.Bytes)})
		sig := mustBytes(t, sv, se)
		if len(sig) != c.sigLen {
			t.Errorf("%s: signature length %d, want %d", c.curve, len(sig), c.sigLen)
		}
		v, err := ecdsaVerifyFn(noCtx, []interpreter.Value{
			interpreter.StringVal(c.algo), bytesArg(msg), bytesArg(sig), bytesArg(pubPEM)})
		if err != nil || !v.Bool {
			t.Errorf("%s: verify genuine = %v (err %v)", c.curve, v.Bool, err)
		}
		// A tampered message must not verify.
		bad, _ := ecdsaVerifyFn(noCtx, []interpreter.Value{
			interpreter.StringVal(c.algo), bytesArg([]byte("tampered")), bytesArg(sig), bytesArg(pubPEM)})
		if bad.Bool {
			t.Errorf("%s: tampered message verified", c.curve)
		}
		// A verify under a different (non-paired) hash must be a clean false.
		wrong := "sha256"
		if c.algo == "sha256" {
			wrong = "sha512"
		}
		mism, err := ecdsaVerifyFn(noCtx, []interpreter.Value{
			interpreter.StringVal(wrong), bytesArg(msg), bytesArg(sig), bytesArg(pubPEM)})
		if err != nil || mism.Bool {
			t.Errorf("%s: verify under wrong hash %s = %v (err %v), want false", c.curve, wrong, mism.Bool, err)
		}
	}
}

func TestAsymRejectsBadInput(t *testing.T) {
	priv, pub := rsaKeyPEMs(t)
	msg := []byte("m")
	// Non-PEM key is a positioned error.
	if _, err := rsaSignFn(noCtx, []interpreter.Value{
		interpreter.StringVal("sha256"), bytesArg(msg), bytesArg([]byte("not pem"))}); err == nil {
		t.Error("rsaSign accepted a non-PEM key")
	}
	// Unknown algorithm is an error.
	if _, err := rsaSignFn(noCtx, []interpreter.Value{
		interpreter.StringVal("md5"), bytesArg(msg), bytesArg(priv)}); err == nil {
		t.Error("rsaSign accepted an unknown algorithm")
	}
	// An EC public key handed to rsaVerify is rejected, not misused.
	ecPriv, _ := ecKeyPEMs(t)
	if _, err := rsaSignFn(noCtx, []interpreter.Value{
		interpreter.StringVal("sha256"), bytesArg(msg), bytesArg(ecPriv)}); err == nil {
		t.Error("rsaSign accepted an EC private key")
	}
	_ = pub
}

// TestRSAVerifyRejectsOversizedModulus pins the maxRSABits cap on PEM keys: a
// 16392-bit public key (PKIX or PKCS#1) is an error, not a slow verify.
func TestRSAVerifyRejectsOversizedModulus(t *testing.T) {
	n := new(big.Int).Lsh(big.NewInt(1), maxRSABits+7)
	n.Add(n, big.NewInt(1))
	pub := &rsa.PublicKey{N: n, E: 65537}
	pkix, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	for name, block := range map[string]*pem.Block{
		"pkix":  {Type: "PUBLIC KEY", Bytes: pkix},
		"pkcs1": {Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(pub)},
	} {
		_, err := rsaVerifyFn(noCtx, []interpreter.Value{
			interpreter.StringVal("sha256"),
			interpreter.BytesVal([]byte("msg")),
			interpreter.BytesVal(make([]byte, len(n.Bytes()))),
			interpreter.BytesVal(pem.EncodeToMemory(block)),
		})
		if err == nil || !strings.Contains(err.Error(), "RSA modulus too large") {
			t.Errorf("%s: err = %v, want modulus-too-large error", name, err)
		}
	}
}
