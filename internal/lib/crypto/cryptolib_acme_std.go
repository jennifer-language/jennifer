// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

// Standard-Go implementation of the crypto library's key-generation, CSR, and
// JWK surface - the asymmetric operations an ACME (RFC 8555) client needs on top
// of the sign / verify in cryptolib_asym_std.go. Same build-tag split: these pull
// in crypto/rsa, crypto/ecdsa, and crypto/x509, so jennifer-tiny gets the stubs
// in cryptolib_acme_tiny.go.
package cryptolib

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// b64uRaw is unpadded base64url, the encoding JOSE / JWK members use.
var b64uRaw = base64.RawURLEncoding

// ----- key generation -----

// rsaGenerateKeyFn implements crypto.rsaGenerateKey(bits) -> bytes: a fresh RSA
// private key as a PKCS#8 PEM. `bits` must be 2048, 3072, or 4096.
func rsaGenerateKeyFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 || args[0].Kind != interpreter.KindInt {
		return interpreter.Null(), fmt.Errorf("crypto.rsaGenerateKey expects 1 int argument (bits)")
	}
	bits := int(args[0].Int)
	if bits != 2048 && bits != 3072 && bits != 4096 {
		return interpreter.Null(), fmt.Errorf("crypto.rsaGenerateKey: bits must be 2048, 3072, or 4096; got %d", bits)
	}
	k, err := rsa.GenerateKey(crand.Reader, bits)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.rsaGenerateKey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.rsaGenerateKey: %v", err)
	}
	return interpreter.BytesVal(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
}

// ecGenerateKeyFn implements crypto.ecGenerateKey(curve) -> bytes: a fresh ECDSA
// private key as a SEC1 PEM. `curve` is "p256", "p384", or "p521".
func ecGenerateKeyFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 || args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("crypto.ecGenerateKey expects 1 string argument (curve)")
	}
	var curve elliptic.Curve
	switch args[0].Str {
	case "p256":
		curve = elliptic.P256()
	case "p384":
		curve = elliptic.P384()
	case "p521":
		curve = elliptic.P521()
	default:
		return interpreter.Null(), fmt.Errorf("crypto.ecGenerateKey: curve must be \"p256\", \"p384\", or \"p521\"; got %q", args[0].Str)
	}
	k, err := ecdsa.GenerateKey(curve, crand.Reader)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.ecGenerateKey: %v", err)
	}
	der, err := x509.MarshalECPrivateKey(k)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.ecGenerateKey: %v", err)
	}
	return interpreter.BytesVal(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})), nil
}

// ----- key parsing (any type) -----

// parseAnyPrivate parses a PEM private key of any supported type into a
// crypto.Signer (RSA or ECDSA), for CSR signing and public-key extraction.
func parseAnyPrivate(fn string, keyPem []byte) (crypto.Signer, error) {
	block, err := decodePEM(fn, keyPem)
	if err != nil {
		return nil, err
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if s, ok := k.(crypto.Signer); ok {
			return s, nil
		}
	}
	return nil, fmt.Errorf("crypto.%s: could not parse an RSA or EC private key", fn)
}

// ----- JWK (RFC 7517 / 7638) -----

// jwkPublicFn implements crypto.jwkPublic(privatePem) -> string: the RFC 7638
// canonical public JWK JSON (members in lexicographic order, no whitespace) of
// the key's public half. Hashing this with SHA-256 yields the JWK thumbprint
// (RFC 7638); embedding it verbatim gives a JWS `jwk` header.
func jwkPublicFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 || args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crypto.jwkPublic expects 1 bytes argument (private PEM)")
	}
	signer, err := parseAnyPrivate("jwkPublic", args[0].Bytes)
	if err != nil {
		return interpreter.Null(), err
	}
	switch pub := signer.Public().(type) {
	case *rsa.PublicKey:
		e := big.NewInt(int64(pub.E)).Bytes()
		jwk := `{"e":"` + b64uRaw.EncodeToString(e) + `","kty":"RSA","n":"` + b64uRaw.EncodeToString(pub.N.Bytes()) + `"}`
		return interpreter.StringVal(jwk), nil
	case *ecdsa.PublicKey:
		size := coordBytes(pub.Curve.Params().BitSize)
		crv, err := curveName(pub.Curve)
		if err != nil {
			return interpreter.Null(), fmt.Errorf("crypto.jwkPublic: %v", err)
		}
		jwk := `{"crv":"` + crv + `","kty":"EC","x":"` + b64uRaw.EncodeToString(leftPad(pub.X, size)) +
			`","y":"` + b64uRaw.EncodeToString(leftPad(pub.Y, size)) + `"}`
		return interpreter.StringVal(jwk), nil
	default:
		return interpreter.Null(), fmt.Errorf("crypto.jwkPublic: unsupported key type")
	}
}

func curveName(c elliptic.Curve) (string, error) {
	switch c {
	case elliptic.P256():
		return "P-256", nil
	case elliptic.P384():
		return "P-384", nil
	case elliptic.P521():
		return "P-521", nil
	default:
		return "", fmt.Errorf("unsupported curve")
	}
}

// ----- CSR (PKCS#10) -----

// csrFn implements crypto.csr(privatePem, domains) -> bytes: a DER-encoded
// PKCS#10 certificate-signing request for `domains` (as subject-alt DNS names,
// the first also the common name), signed with the private key. ACME's finalize
// step wants base64url of this DER.
func csrFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 || args[0].Kind != interpreter.KindBytes || args[1].Kind != interpreter.KindList {
		return interpreter.Null(), fmt.Errorf("crypto.csr expects (private PEM bytes, list of domain strings)")
	}
	domains := make([]string, 0, len(args[1].List))
	for _, d := range args[1].List {
		if d.Kind != interpreter.KindString {
			return interpreter.Null(), fmt.Errorf("crypto.csr: every domain must be a string")
		}
		domains = append(domains, d.Str)
	}
	if len(domains) == 0 {
		return interpreter.Null(), fmt.Errorf("crypto.csr: need at least one domain")
	}
	signer, err := parseAnyPrivate("csr", args[0].Bytes)
	if err != nil {
		return interpreter.Null(), err
	}
	tmpl := &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: domains[0]},
		DNSNames: domains,
	}
	der, err := x509.CreateCertificateRequest(crand.Reader, tmpl, signer)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.csr: %v", err)
	}
	return interpreter.BytesVal(der), nil
}
