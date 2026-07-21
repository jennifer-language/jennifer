// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package cryptolib

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
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
			bytesArg(priv), bytesArg(msg), interpreter.StringVal(algo)})
		sig := mustBytes(t, sv, se)
		if len(sig) != 256 { // 2048-bit key -> 256-byte signature
			t.Errorf("%s: signature length %d, want 256", algo, len(sig))
		}
		v, err := rsaVerifyFn(noCtx, []interpreter.Value{
			bytesArg(pub), bytesArg(msg), bytesArg(sig), interpreter.StringVal(algo)})
		if err != nil || !v.Bool {
			t.Errorf("%s: verify genuine = %v (err %v)", algo, v.Bool, err)
		}
		// A tampered message must not verify.
		bad, _ := rsaVerifyFn(noCtx, []interpreter.Value{
			bytesArg(pub), bytesArg([]byte("tampered")), bytesArg(sig), interpreter.StringVal(algo)})
		if bad.Bool {
			t.Errorf("%s: tampered message verified", algo)
		}
	}
}

func TestECDSASignVerifyRoundTrip(t *testing.T) {
	priv, pub := ecKeyPEMs(t)
	msg := []byte("the quick brown fox")
	sv, se := ecdsaSignFn(noCtx, []interpreter.Value{
		bytesArg(priv), bytesArg(msg), interpreter.StringVal("sha256")})
	sig := mustBytes(t, sv, se)
	if len(sig) != 64 { // P-256 JOSE signature is 32+32
		t.Fatalf("ES256 signature length %d, want 64", len(sig))
	}
	v, err := ecdsaVerifyFn(noCtx, []interpreter.Value{
		bytesArg(pub), bytesArg(msg), bytesArg(sig), interpreter.StringVal("sha256")})
	if err != nil || !v.Bool {
		t.Errorf("ES256 verify genuine = %v (err %v)", v.Bool, err)
	}
	bad, _ := ecdsaVerifyFn(noCtx, []interpreter.Value{
		bytesArg(pub), bytesArg([]byte("tampered")), bytesArg(sig), interpreter.StringVal("sha256")})
	if bad.Bool {
		t.Error("ES256 tampered message verified")
	}
	// A signature of the wrong length is not valid (must be false, not a panic).
	short, err := ecdsaVerifyFn(noCtx, []interpreter.Value{
		bytesArg(pub), bytesArg(msg), bytesArg([]byte{1, 2, 3}), interpreter.StringVal("sha256")})
	if err != nil || short.Bool {
		t.Errorf("wrong-length signature: got %v (err %v), want false", short.Bool, err)
	}
}

func TestAsymRejectsBadInput(t *testing.T) {
	priv, pub := rsaKeyPEMs(t)
	msg := []byte("m")
	// Non-PEM key is a positioned error.
	if _, err := rsaSignFn(noCtx, []interpreter.Value{
		bytesArg([]byte("not pem")), bytesArg(msg), interpreter.StringVal("sha256")}); err == nil {
		t.Error("rsaSign accepted a non-PEM key")
	}
	// Unknown algorithm is an error.
	if _, err := rsaSignFn(noCtx, []interpreter.Value{
		bytesArg(priv), bytesArg(msg), interpreter.StringVal("md5")}); err == nil {
		t.Error("rsaSign accepted an unknown algorithm")
	}
	// An EC public key handed to rsaVerify is rejected, not misused.
	ecPriv, _ := ecKeyPEMs(t)
	if _, err := rsaSignFn(noCtx, []interpreter.Value{
		bytesArg(ecPriv), bytesArg(msg), interpreter.StringVal("sha256")}); err == nil {
		t.Error("rsaSign accepted an EC private key")
	}
	_ = pub
}
