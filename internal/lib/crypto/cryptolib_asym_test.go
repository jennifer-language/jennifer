// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package cryptolib

import (
	"bytes"
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

func pemSign(algo string, msg, priv []byte) (interpreter.Value, error) {
	return signFn(noCtx, []interpreter.Value{strArg(algo), bytesArg(msg), bytesArg(priv)})
}

func pemVerify(algo string, msg, sig, pub []byte) (interpreter.Value, error) {
	return verifyFn(noCtx, []interpreter.Value{strArg(algo), bytesArg(msg), bytesArg(sig), bytesArg(pub)})
}

func TestRSASignVerifyRoundTrip(t *testing.T) {
	priv, pub := rsaKeyPEMs(t)
	msg := []byte("the quick brown fox")
	for _, algo := range []string{"rsa-sha256", "rsa-sha384", "rsa-sha512"} {
		sv, se := pemSign(algo, msg, priv)
		sig := mustBytes(t, sv, se)
		if len(sig) != 256 { // 2048-bit key -> 256-byte signature
			t.Errorf("%s: signature length %d, want 256", algo, len(sig))
		}
		v, err := pemVerify(algo, msg, sig, pub)
		if err != nil || !v.Bool {
			t.Errorf("%s: verify genuine = %v (err %v)", algo, v.Bool, err)
		}
		// A tampered message must not verify.
		bad, _ := pemVerify(algo, []byte("tampered"), sig, pub)
		if bad.Bool {
			t.Errorf("%s: tampered message verified", algo)
		}
		// Nor under another hash: the algorithm name binds the digest.
		other := "rsa-sha256"
		if algo == other {
			other = "rsa-sha512"
		}
		mism, err := pemVerify(other, msg, sig, pub)
		if err != nil || mism.Bool {
			t.Errorf("%s signature verified under %s = %v (err %v), want false", algo, other, mism.Bool, err)
		}
	}
}

func TestECDSASignVerifyRoundTrip(t *testing.T) {
	priv, pub := ecKeyPEMs(t)
	msg := []byte("the quick brown fox")
	sv, se := pemSign("ecdsa-sha256", msg, priv)
	sig := mustBytes(t, sv, se)
	if len(sig) != 64 { // P-256 JOSE signature is 32+32
		t.Fatalf("ES256 signature length %d, want 64", len(sig))
	}
	v, err := pemVerify("ecdsa-sha256", msg, sig, pub)
	if err != nil || !v.Bool {
		t.Errorf("ES256 verify genuine = %v (err %v)", v.Bool, err)
	}
	bad, _ := pemVerify("ecdsa-sha256", []byte("tampered"), sig, pub)
	if bad.Bool {
		t.Error("ES256 tampered message verified")
	}
	// A signature of the wrong length is not valid (must be false, not a panic).
	short, err := pemVerify("ecdsa-sha256", msg, []byte{1, 2, 3}, pub)
	if err != nil || short.Bool {
		t.Errorf("wrong-length signature: got %v (err %v), want false", short.Bool, err)
	}
}

// TestECDSAAllCurves pins keypair / publicKey / sign / verify across all three
// JOSE ES curves with their spec-paired hashes (ES256/P-256/SHA-256,
// ES384/P-384/SHA-384, ES512/P-521/SHA-512). The P-384 / P-521 paths are
// exactly where a JWS alg-binding bug hides: a coordinate-width or digest
// mismatch round-trips wrong. Signatures are the fixed-width JOSE R||S form
// (2*ceil(bits/8)), and a wrong-hash verify must be a plain false, never a
// panic.
func TestECDSAAllCurves(t *testing.T) {
	cases := []struct {
		curve, algo string
		sigLen      int
	}{
		{"p256", "ecdsa-sha256", 64},
		{"p384", "ecdsa-sha384", 96},
		{"p521", "ecdsa-sha512", 132},
	}
	msg := []byte("the quick brown fox")
	for _, c := range cases {
		pubPEM, privPEM := genKeypair(t, c.curve)
		// The private key is SEC1; the public half keypair returns is the PKIX
		// PEM of that same key, and publicKey derives it again.
		block, _ := pem.Decode(privPEM)
		if block == nil || block.Type != "EC PRIVATE KEY" {
			t.Fatalf("%s: private key is not an EC PRIVATE KEY PEM: %s", c.curve, privPEM)
		}
		ek, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			t.Fatalf("%s: parse: %v", c.curve, err)
		}
		pubDER, _ := x509.MarshalPKIXPublicKey(&ek.PublicKey)
		want := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
		if !bytes.Equal(pubPEM, want) {
			t.Errorf("%s: keypair public = %s, want the PKIX PEM of the private key %s", c.curve, pubPEM, want)
		}
		pv, err := publicKeyFn(noCtx, []interpreter.Value{strArg(c.curve), bytesArg(privPEM)})
		if err != nil || !bytes.Equal(pv.Bytes, want) {
			t.Errorf("%s: publicKey = %s (err %v), want %s", c.curve, pv.Bytes, err, want)
		}

		sv, se := pemSign(c.algo, msg, privPEM)
		sig := mustBytes(t, sv, se)
		if len(sig) != c.sigLen {
			t.Errorf("%s: signature length %d, want %d", c.curve, len(sig), c.sigLen)
		}
		v, err := pemVerify(c.algo, msg, sig, pubPEM)
		if err != nil || !v.Bool {
			t.Errorf("%s: verify genuine = %v (err %v)", c.curve, v.Bool, err)
		}
		// A tampered message must not verify.
		bad, _ := pemVerify(c.algo, []byte("tampered"), sig, pubPEM)
		if bad.Bool {
			t.Errorf("%s: tampered message verified", c.curve)
		}
		// A verify under a different (non-paired) hash must be a clean false.
		wrong := "ecdsa-sha256"
		if c.algo == wrong {
			wrong = "ecdsa-sha512"
		}
		mism, err := pemVerify(wrong, msg, sig, pubPEM)
		if err != nil || mism.Bool {
			t.Errorf("%s: verify under wrong hash %s = %v (err %v), want false", c.curve, wrong, mism.Bool, err)
		}
	}
}

// keypair("rsa2048") returns a PKCS#8 private PEM and the matching PKIX public
// PEM; publicKey derives the same public PEM and checks the modulus size the
// name promises.
func TestRSAKeypairAndPublicKey(t *testing.T) {
	pub, priv := genKeypair(t, "rsa2048")
	block, _ := pem.Decode(priv)
	if block == nil || block.Type != "PRIVATE KEY" {
		t.Fatalf("private key is not a PKCS#8 PRIVATE KEY PEM: %s", priv)
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	rk, ok := k.(*rsa.PrivateKey)
	if err != nil || !ok || rk.N.BitLen() != 2048 {
		t.Fatalf("private key: %T (err %v), want a 2048-bit RSA key", k, err)
	}
	if !strings.HasPrefix(string(pub), "-----BEGIN PUBLIC KEY-----") {
		t.Errorf("public key is not a PKIX PEM: %s", pub)
	}
	pv, err := publicKeyFn(noCtx, []interpreter.Value{strArg("rsa2048"), bytesArg(priv)})
	if err != nil || !bytes.Equal(pv.Bytes, pub) {
		t.Errorf("publicKey = %s (err %v), want the keypair's %s", pv.Bytes, err, pub)
	}
	// The pair works together.
	sv, se := pemSign("rsa-sha256", []byte("m"), priv)
	if v, err := pemVerify("rsa-sha256", []byte("m"), mustBytes(t, sv, se), pub); err != nil || !v.Bool {
		t.Errorf("keypair's halves do not sign / verify together (err %v)", err)
	}
	// A key of the wrong size or type for the name is an error.
	_, err = publicKeyFn(noCtx, []interpreter.Value{strArg("rsa3072"), bytesArg(priv)})
	wantErr(t, "publicKey rsa3072 of a 2048-bit key", err, "needs a 3072-bit RSA key")
	ecPriv, _ := ecKeyPEMs(t)
	_, err = publicKeyFn(noCtx, []interpreter.Value{strArg("rsa2048"), bytesArg(ecPriv)})
	wantErr(t, "publicKey rsa2048 of an EC key", err, "crypto.publicKey")
	_, err = publicKeyFn(noCtx, []interpreter.Value{strArg("p384"), bytesArg(ecPriv)})
	wantErr(t, "publicKey p384 of a P-256 key", err, `"p384" needs a key on that curve`)
	_, err = publicKeyFn(noCtx, []interpreter.Value{strArg("p256"), bytesArg(priv)})
	wantErr(t, "publicKey p256 of an RSA key", err, "non-ECDSA private key")
}

func TestAsymRejectsBadInput(t *testing.T) {
	priv, pub := rsaKeyPEMs(t)
	ecPriv, ecPub := ecKeyPEMs(t)
	msg := []byte("m")
	// Non-PEM key is an error.
	_, err := pemSign("rsa-sha256", msg, []byte("not pem"))
	wantErr(t, "rsa-sha256 non-PEM key", err, "not valid PEM")
	_, err = pemSign("ecdsa-sha256", msg, []byte("not pem"))
	wantErr(t, "ecdsa-sha256 non-PEM key", err, "not valid PEM")
	_, err = pemVerify("rsa-sha256", msg, make([]byte, 256), []byte("not pem"))
	wantErr(t, "rsa-sha256 verify non-PEM key", err, "not valid PEM")
	// Unknown algorithm is an error (the bare hash names of the old rsaSign
	// are not algorithms of sign).
	for _, algo := range []string{"rsa-md5", "sha256", "rs256"} {
		_, err = pemSign(algo, msg, priv)
		wantErr(t, "sign "+algo, err, "unknown algorithm")
	}
	// A key of the other family is rejected, not misused.
	_, err = pemSign("rsa-sha256", msg, ecPriv)
	wantErr(t, "rsa-sha256 with an EC private key", err, "crypto.sign")
	_, err = pemSign("ecdsa-sha256", msg, priv)
	wantErr(t, "ecdsa-sha256 with an RSA private key", err, "non-ECDSA private key")
	_, err = pemVerify("rsa-sha256", msg, make([]byte, 256), ecPub)
	wantErr(t, "rsa-sha256 verify with an EC public key", err, "non-RSA public key")
	_, err = pemVerify("ecdsa-sha256", msg, make([]byte, 64), pub)
	wantErr(t, "ecdsa-sha256 verify with an RSA public key", err, "non-ECDSA public key")
	// An Ed25519-style raw key is not a PEM key.
	_, err = pemSign("rsa-sha256", msg, make([]byte, 64))
	wantErr(t, "rsa-sha256 with a raw key", err, "not valid PEM")
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
		_, err := pemVerify("rsa-sha256", []byte("msg"), make([]byte, len(n.Bytes())), pem.EncodeToMemory(block))
		if err == nil || !strings.Contains(err.Error(), "RSA modulus too large") {
			t.Errorf("%s: err = %v, want modulus-too-large error", name, err)
		}
		// wrap parses public keys the same way.
		_, err = wrapFn(noCtx, []interpreter.Value{strArg("rsa-oaep-sha256"), bytesArg([]byte("k")), bytesArg(pem.EncodeToMemory(block))})
		if err == nil || !strings.Contains(err.Error(), "RSA modulus too large") {
			t.Errorf("wrap %s: err = %v, want modulus-too-large error", name, err)
		}
	}
}
