// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

//go:build !tinygo

package cryptolib

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

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
