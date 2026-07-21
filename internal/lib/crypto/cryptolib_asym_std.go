// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

// Standard-Go implementation of the crypto library's asymmetric RSA / ECDSA
// signature surface (the JWT RS* / ES* algorithms). Keys are PEM-encoded. This
// pulls in crypto/rsa, crypto/ecdsa, and crypto/x509, which are heavy and not
// part of the TinyGo build; jennifer-tiny selects cryptolib_asym_tiny.go, whose
// stubs return a friendly "not available" error (as with `net`).
package cryptolib

import (
	"crypto"
	"crypto/ecdsa"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	gohash "hash"
	"math/big"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// digestOf hashes msg with the named algorithm and returns the digest plus the
// crypto.Hash identifier (which RSA PKCS#1 v1.5 signing needs). algo is one of
// "sha256" / "sha384" / "sha512" (the JOSE RS256 / ES256 family and their 384 /
// 512 variants).
func digestOf(algo string, msg []byte) ([]byte, crypto.Hash, error) {
	var h gohash.Hash
	var id crypto.Hash
	switch algo {
	case "sha256":
		h, id = sha256.New(), crypto.SHA256
	case "sha384":
		h, id = sha512.New384(), crypto.SHA384
	case "sha512":
		h, id = sha512.New(), crypto.SHA512
	default:
		return nil, 0, fmt.Errorf("unknown hash algorithm %q (want \"sha256\", \"sha384\", or \"sha512\")", algo)
	}
	h.Write(msg)
	return h.Sum(nil), id, nil
}

// asymArgs validates the (key, message, algo) argument shape shared by the sign
// functions: key and message are bytes, algo is a string.
func asymArgs(fn string, args []interpreter.Value, want int) error {
	if len(args) != want {
		return fmt.Errorf("crypto.%s expects %d arguments, got %d", fn, want, len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return fmt.Errorf("crypto.%s: key must be bytes (PEM)", fn)
	}
	if args[1].Kind != interpreter.KindBytes {
		return fmt.Errorf("crypto.%s: message must be bytes", fn)
	}
	if args[want-1].Kind != interpreter.KindString {
		return fmt.Errorf("crypto.%s: algorithm must be a string", fn)
	}
	return nil
}

func decodePEM(fn string, keyPem []byte) (*pem.Block, error) {
	block, _ := pem.Decode(keyPem)
	if block == nil {
		return nil, fmt.Errorf("crypto.%s: key is not valid PEM", fn)
	}
	return block, nil
}

// ----- RSA (RS256 / RS384 / RS512, PKCS#1 v1.5) -----------------------------

func parseRSAPrivate(fn string, keyPem []byte) (*rsa.PrivateKey, error) {
	block, err := decodePEM(fn, keyPem)
	if err != nil {
		return nil, err
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, nil
		}
		return nil, fmt.Errorf("crypto.%s: PEM holds a non-RSA private key", fn)
	}
	return nil, fmt.Errorf("crypto.%s: could not parse an RSA private key (want PKCS#1 or PKCS#8 PEM)", fn)
}

func parseRSAPublic(fn string, keyPem []byte) (*rsa.PublicKey, error) {
	block, err := decodePEM(fn, keyPem)
	if err != nil {
		return nil, err
	}
	if k, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rk, ok := k.(*rsa.PublicKey); ok {
			return rk, nil
		}
		return nil, fmt.Errorf("crypto.%s: PEM holds a non-RSA public key", fn)
	}
	if k, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return k, nil
	}
	return nil, fmt.Errorf("crypto.%s: could not parse an RSA public key (want PKIX or PKCS#1 PEM)", fn)
}

// rsaSignFn implements crypto.rsaSign(privatePem, message, algo) -> bytes: an
// RSASSA-PKCS1-v1.5 signature over message.
func rsaSignFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := asymArgs("rsaSign", args, 3); err != nil {
		return interpreter.Null(), err
	}
	priv, err := parseRSAPrivate("rsaSign", args[0].Bytes)
	if err != nil {
		return interpreter.Null(), err
	}
	digest, id, err := digestOf(args[2].Str, args[1].Bytes)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.rsaSign: %v", err)
	}
	sig, err := rsa.SignPKCS1v15(crand.Reader, priv, id, digest)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.rsaSign: %v", err)
	}
	return interpreter.BytesVal(sig), nil
}

// rsaVerifyFn implements crypto.rsaVerify(publicPem, message, signature, algo)
// -> bool. A genuine mismatch is false; a malformed key is a positioned error.
func rsaVerifyFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 4 {
		return interpreter.Null(), fmt.Errorf("crypto.rsaVerify expects 4 arguments (publicPem, message, signature, algo), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes || args[1].Kind != interpreter.KindBytes || args[2].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crypto.rsaVerify: key, message, and signature must be bytes")
	}
	if args[3].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("crypto.rsaVerify: algorithm must be a string")
	}
	pub, err := parseRSAPublic("rsaVerify", args[0].Bytes)
	if err != nil {
		return interpreter.Null(), err
	}
	digest, id, err := digestOf(args[3].Str, args[1].Bytes)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.rsaVerify: %v", err)
	}
	ok := rsa.VerifyPKCS1v15(pub, id, digest, args[2].Bytes) == nil
	return interpreter.BoolVal(ok), nil
}

// ----- ECDSA (ES256 / ES384 / ES512, JOSE R||S signatures) -----------------

func parseECPrivate(fn string, keyPem []byte) (*ecdsa.PrivateKey, error) {
	block, err := decodePEM(fn, keyPem)
	if err != nil {
		return nil, err
	}
	if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if ek, ok := k.(*ecdsa.PrivateKey); ok {
			return ek, nil
		}
		return nil, fmt.Errorf("crypto.%s: PEM holds a non-ECDSA private key", fn)
	}
	return nil, fmt.Errorf("crypto.%s: could not parse an EC private key (want SEC1 or PKCS#8 PEM)", fn)
}

func parseECPublic(fn string, keyPem []byte) (*ecdsa.PublicKey, error) {
	block, err := decodePEM(fn, keyPem)
	if err != nil {
		return nil, err
	}
	if k, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if ek, ok := k.(*ecdsa.PublicKey); ok {
			return ek, nil
		}
		return nil, fmt.Errorf("crypto.%s: PEM holds a non-ECDSA public key", fn)
	}
	return nil, fmt.Errorf("crypto.%s: could not parse an EC public key (want PKIX PEM)", fn)
}

// coordBytes is the fixed byte width of one ECDSA signature coordinate (r or s)
// for the key's curve: ceil(bitSize / 8). JOSE signatures are the fixed-width
// big-endian r concatenated with s, not the ASN.1 DER form crypto/ecdsa emits.
func coordBytes(bitSize int) int {
	return (bitSize + 7) / 8
}

// leftPad returns x as a big-endian byte slice of exactly size bytes.
func leftPad(x *big.Int, size int) []byte {
	b := x.Bytes()
	if len(b) >= size {
		return b[len(b)-size:]
	}
	out := make([]byte, size)
	copy(out[size-len(b):], b)
	return out
}

// ecdsaSignFn implements crypto.ecdsaSign(privatePem, message, algo) -> bytes:
// an ECDSA signature in the JOSE R||S fixed-width form.
func ecdsaSignFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if err := asymArgs("ecdsaSign", args, 3); err != nil {
		return interpreter.Null(), err
	}
	priv, err := parseECPrivate("ecdsaSign", args[0].Bytes)
	if err != nil {
		return interpreter.Null(), err
	}
	digest, _, err := digestOf(args[2].Str, args[1].Bytes)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.ecdsaSign: %v", err)
	}
	r, s, err := ecdsa.Sign(crand.Reader, priv, digest)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.ecdsaSign: %v", err)
	}
	size := coordBytes(priv.Curve.Params().BitSize)
	sig := append(leftPad(r, size), leftPad(s, size)...)
	return interpreter.BytesVal(sig), nil
}

// ecdsaVerifyFn implements crypto.ecdsaVerify(publicPem, message, signature,
// algo) -> bool. The signature is JOSE R||S; a wrong length is false (a
// malformed signature is not a valid one), a malformed key is a positioned error.
func ecdsaVerifyFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 4 {
		return interpreter.Null(), fmt.Errorf("crypto.ecdsaVerify expects 4 arguments (publicPem, message, signature, algo), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes || args[1].Kind != interpreter.KindBytes || args[2].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crypto.ecdsaVerify: key, message, and signature must be bytes")
	}
	if args[3].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("crypto.ecdsaVerify: algorithm must be a string")
	}
	pub, err := parseECPublic("ecdsaVerify", args[0].Bytes)
	if err != nil {
		return interpreter.Null(), err
	}
	digest, _, err := digestOf(args[3].Str, args[1].Bytes)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.ecdsaVerify: %v", err)
	}
	sig := args[2].Bytes
	size := coordBytes(pub.Curve.Params().BitSize)
	if len(sig) != 2*size {
		return interpreter.BoolVal(false), nil
	}
	r := new(big.Int).SetBytes(sig[:size])
	s := new(big.Int).SetBytes(sig[size:])
	ok := ecdsa.Verify(pub, digest, r, s)
	return interpreter.BoolVal(ok), nil
}
