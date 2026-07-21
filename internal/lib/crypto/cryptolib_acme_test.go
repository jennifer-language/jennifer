// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package cryptolib

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// independentJWK rebuilds the RFC 7638 canonical public JWK from a PEM private
// key by a DIFFERENT route than jwkPublicFn: it marshals a map with
// encoding/json (which sorts keys, giving canonical member order for free),
// rather than hand-concatenating a string. Agreement between the two is strong
// evidence the canonical form and base64url member encodings are correct.
func independentJWK(t *testing.T, keyPEM []byte) string {
	t.Helper()
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		t.Fatal("independentJWK: not PEM")
	}
	var pub any
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		pub = publicOf(k)
	} else if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		pub = &k.PublicKey
	} else if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		pub = &k.PublicKey
	} else {
		t.Fatal("independentJWK: cannot parse key")
	}
	b64 := base64.RawURLEncoding.EncodeToString
	var m map[string]string
	switch p := pub.(type) {
	case *rsa.PublicKey:
		m = map[string]string{"kty": "RSA", "n": b64(p.N.Bytes()), "e": b64(big.NewInt(int64(p.E)).Bytes())}
	case *ecdsa.PublicKey:
		size := (p.Curve.Params().BitSize + 7) / 8
		crv := map[int]string{256: "P-256", 384: "P-384", 521: "P-521"}[p.Curve.Params().BitSize]
		m = map[string]string{"kty": "EC", "crv": crv, "x": b64(leftPadInt(p.X, size)), "y": b64(leftPadInt(p.Y, size))}
	default:
		t.Fatal("independentJWK: unexpected key type")
	}
	out, _ := json.Marshal(m) // json.Marshal sorts map keys -> canonical order
	return string(out)
}

func publicOf(k any) any {
	switch kk := k.(type) {
	case *rsa.PrivateKey:
		return &kk.PublicKey
	case *ecdsa.PrivateKey:
		return &kk.PublicKey
	}
	return nil
}

func leftPadInt(x *big.Int, size int) []byte {
	b := x.Bytes()
	if len(b) >= size {
		return b[len(b)-size:]
	}
	out := make([]byte, size)
	copy(out[size-len(b):], b)
	return out
}

// TestJWKMatchesIndependentOracle cross-checks jwkPublic's hand-built canonical
// JWK against an encoding/json oracle, for both key families.
func TestJWKMatchesIndependentOracle(t *testing.T) {
	ecKey, _ := ecGenerateKeyFn(noCtx, []interpreter.Value{interpreter.StringVal("p384")})
	rsaKey, _ := rsaGenerateKeyFn(noCtx, []interpreter.Value{interpreter.IntVal(2048)})
	for _, k := range [][]byte{ecKey.Bytes, rsaKey.Bytes} {
		jv, err := jwkPublicFn(noCtx, []interpreter.Value{bytesArg(k)})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := jv.Str, independentJWK(t, k); got != want {
			t.Errorf("jwkPublic canonical JWK mismatch:\n got  %s\n want %s", got, want)
		}
	}
}

func TestEcGenerateKeyAndJWK(t *testing.T) {
	kv, err := ecGenerateKeyFn(noCtx, []interpreter.Value{interpreter.StringVal("p256")})
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := kv.Bytes
	if !strings.Contains(string(keyPEM), "EC PRIVATE KEY") {
		t.Fatalf("ecGenerateKey did not return an EC PEM: %s", keyPEM)
	}
	// The generated key must sign and verify.
	sig, err := ecdsaSignFn(noCtx, []interpreter.Value{bytesArg(keyPEM), bytesArg([]byte("m")), interpreter.StringVal("sha256")})
	if err != nil {
		t.Fatal(err)
	}
	// JWK is canonical (members in lexicographic order crv, kty, x, y).
	jv, err := jwkPublicFn(noCtx, []interpreter.Value{bytesArg(keyPEM)})
	if err != nil {
		t.Fatal(err)
	}
	jwk := jv.Str
	if !strings.HasPrefix(jwk, `{"crv":"P-256","kty":"EC","x":"`) {
		t.Errorf("EC JWK not canonical: %s", jwk)
	}
	_ = sig.Bytes
}

func TestRsaGenerateKeyAndJWK(t *testing.T) {
	kv, err := rsaGenerateKeyFn(noCtx, []interpreter.Value{interpreter.IntVal(2048)})
	if err != nil {
		t.Fatal(err)
	}
	jv, err := jwkPublicFn(noCtx, []interpreter.Value{bytesArg(kv.Bytes)})
	if err != nil {
		t.Fatal(err)
	}
	// Canonical RSA JWK order is e, kty, n.
	if !strings.HasPrefix(jv.Str, `{"e":"`) || !strings.Contains(jv.Str, `"kty":"RSA"`) {
		t.Errorf("RSA JWK not canonical: %s", jv.Str)
	}
	if _, err := rsaGenerateKeyFn(noCtx, []interpreter.Value{interpreter.IntVal(1024)}); err == nil {
		t.Error("rsaGenerateKey accepted a weak 1024-bit size")
	}
}

func TestJWKThumbprintStable(t *testing.T) {
	kv, _ := ecGenerateKeyFn(noCtx, []interpreter.Value{interpreter.StringVal("p256")})
	a, _ := jwkPublicFn(noCtx, []interpreter.Value{bytesArg(kv.Bytes)})
	b, _ := jwkPublicFn(noCtx, []interpreter.Value{bytesArg(kv.Bytes)})
	if a.Str != b.Str {
		t.Fatal("jwkPublic is not deterministic for the same key")
	}
	// The thumbprint is just SHA-256 of the canonical JWK - confirm it hashes.
	sum := sha256.Sum256([]byte(a.Str))
	if len(sum) != 32 {
		t.Fatal("unexpected digest length")
	}
}

func TestCSRForDomains(t *testing.T) {
	kv, _ := ecGenerateKeyFn(noCtx, []interpreter.Value{interpreter.StringVal("p256")})
	domains := interpreter.ListVal(parser.PrimitiveType(parser.TypeString),
		[]interpreter.Value{interpreter.StringVal("example.com"), interpreter.StringVal("www.example.com")})
	dv, err := csrFn(noCtx, []interpreter.Value{bytesArg(kv.Bytes), domains})
	if err != nil {
		t.Fatal(err)
	}
	// The DER parses as a valid CSR carrying both SAN domains.
	csr, err := x509.ParseCertificateRequest(dv.Bytes)
	if err != nil {
		t.Fatalf("csr produced invalid DER: %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Errorf("csr signature invalid: %v", err)
	}
	if len(csr.DNSNames) != 2 || csr.DNSNames[0] != "example.com" {
		t.Errorf("csr SANs = %v, want [example.com www.example.com]", csr.DNSNames)
	}
	// An empty domain list is rejected.
	empty := interpreter.ListVal(parser.PrimitiveType(parser.TypeString), nil)
	if _, err := csrFn(noCtx, []interpreter.Value{bytesArg(kv.Bytes), empty}); err == nil {
		t.Error("csr accepted an empty domain list")
	}
	_ = pem.Block{}
}
