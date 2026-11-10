// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

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
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	gohash "hash"
	"math/big"
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

func decodePEM(fn string, keyPem []byte) (*pem.Block, error) {
	block, _ := pem.Decode(keyPem)
	if block == nil {
		return nil, fmt.Errorf("crypto.%s: key is not valid PEM", fn)
	}
	return block, nil
}

// ----- RSA (RS256 / RS384 / RS512, PKCS#1 v1.5) -----------------------------

// maxRSABits caps the modulus of any parsed RSA key. A modular exponentiation
// costs ~O(bits^2) and Go's crypto/rsa and x509 accept any size, so a key from
// a remote source (x5c chain, JWKS, fetched PEM) could pin a CPU for seconds
// per call. 16384 bits is well past any real RSA key.
const maxRSABits = 16384

// checkRSABits rejects a modulus over maxRSABits.
func checkRSABits(fn string, n *big.Int) error {
	if n.BitLen() > maxRSABits {
		return fmt.Errorf("crypto.%s: RSA modulus too large (%d bits; max %d)", fn, n.BitLen(), maxRSABits)
	}
	return nil
}

func parseRSAPrivate(fn string, keyPem []byte) (*rsa.PrivateKey, error) {
	block, err := decodePEM(fn, keyPem)
	if err != nil {
		return nil, err
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, checkRSABits(fn, k.N)
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, checkRSABits(fn, rk.N)
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
			return rk, checkRSABits(fn, rk.N)
		}
		return nil, fmt.Errorf("crypto.%s: PEM holds a non-RSA public key", fn)
	}
	if k, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return k, checkRSABits(fn, k.N)
	}
	return nil, fmt.Errorf("crypto.%s: could not parse an RSA public key (want PKIX or PKCS#1 PEM)", fn)
}

// rsaSign signs with RSASSA-PKCS1-v1.5 (sign "rsa-sha256" / 384 / 512).
func rsaSign(fn, algo, hashName string, msg, keyPem []byte) ([]byte, error) {
	priv, err := parseRSAPrivate(fn, keyPem)
	if err != nil {
		return nil, err
	}
	digest, id, err := digestOf(hashName, msg)
	if err != nil {
		return nil, fmt.Errorf("crypto.%s: %v", fn, err)
	}
	sig, err := rsa.SignPKCS1v15(crand.Reader, priv, id, digest)
	if err != nil {
		return nil, fmt.Errorf("crypto.%s: %v", fn, err)
	}
	return sig, nil
}

// rsaVerify checks an RSASSA-PKCS1-v1.5 signature. A genuine mismatch is false;
// a malformed key is an error.
func rsaVerify(fn, algo, hashName string, msg, sig, keyPem []byte) (bool, error) {
	pub, err := parseRSAPublic(fn, keyPem)
	if err != nil {
		return false, err
	}
	digest, id, err := digestOf(hashName, msg)
	if err != nil {
		return false, fmt.Errorf("crypto.%s: %v", fn, err)
	}
	return rsa.VerifyPKCS1v15(pub, id, digest, sig) == nil, nil
}

// rsaOAEP wraps (encrypts to a public key) or unwraps (decrypts with the private
// key) with RSAES-OAEP, MGF1 over the same hash, and an empty label. SHA-1 is
// accepted here only, for interop with formats that use it.
func rsaOAEP(fn, algo string, wrap bool, data, keyPem []byte) ([]byte, error) {
	h := sha256.New()
	if algo == "rsa-oaep-sha1" {
		h = sha1.New()
	}
	if wrap {
		pub, err := parseRSAPublic(fn, keyPem)
		if err != nil {
			return nil, err
		}
		out, err := rsa.EncryptOAEP(h, crand.Reader, pub, data, nil)
		if err != nil {
			return nil, fmt.Errorf("crypto.%s: %v", fn, err)
		}
		return out, nil
	}
	priv, err := parseRSAPrivate(fn, keyPem)
	if err != nil {
		return nil, err
	}
	out, err := rsa.DecryptOAEP(h, crand.Reader, priv, data, nil)
	if err != nil {
		// One message for every failure: telling a bad padding from a wrong key
		// apart is what an OAEP oracle needs.
		return nil, fmt.Errorf("crypto.%s: decryption failed (wrong key or corrupted ciphertext)", fn)
	}
	return out, nil
}

// rsaPublicPEM returns the PKIX public-key PEM of an RSA private key, which must
// have the modulus size the algorithm names.
func rsaPublicPEM(fn, algo string, bits int, keyPem []byte) ([]byte, error) {
	priv, err := parseRSAPrivate(fn, keyPem)
	if err != nil {
		return nil, err
	}
	if priv.N.BitLen() != bits {
		return nil, fmt.Errorf("crypto.%s: %q needs a %d-bit RSA key, got %d bits", fn, algo, bits, priv.N.BitLen())
	}
	return publicPEM(fn, &priv.PublicKey)
}

// publicPEM encodes a public key as a PKIX "PUBLIC KEY" PEM.
func publicPEM(fn string, pub any) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("crypto.%s: %v", fn, err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
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

// ecdsaSign signs with ECDSA over the key's curve and returns the JOSE R||S
// fixed-width form (sign "ecdsa-sha256" / 384 / 512).
func ecdsaSign(fn, algo, hashName string, msg, keyPem []byte) ([]byte, error) {
	priv, err := parseECPrivate(fn, keyPem)
	if err != nil {
		return nil, err
	}
	digest, _, err := digestOf(hashName, msg)
	if err != nil {
		return nil, fmt.Errorf("crypto.%s: %v", fn, err)
	}
	r, s, err := ecdsa.Sign(crand.Reader, priv, digest)
	if err != nil {
		return nil, fmt.Errorf("crypto.%s: %v", fn, err)
	}
	size := coordBytes(priv.Curve.Params().BitSize)
	return append(leftPad(r, size), leftPad(s, size)...), nil
}

// ecdsaVerify checks a JOSE R||S signature; a wrong length is false (a
// malformed signature is not a valid one), a malformed key is an error.
func ecdsaVerify(fn, algo, hashName string, msg, sig, keyPem []byte) (bool, error) {
	pub, err := parseECPublic(fn, keyPem)
	if err != nil {
		return false, err
	}
	digest, _, err := digestOf(hashName, msg)
	if err != nil {
		return false, fmt.Errorf("crypto.%s: %v", fn, err)
	}
	size := coordBytes(pub.Curve.Params().BitSize)
	if len(sig) != 2*size {
		return false, nil
	}
	r := new(big.Int).SetBytes(sig[:size])
	s := new(big.Int).SetBytes(sig[size:])
	return ecdsa.Verify(pub, digest, r, s), nil
}

// ecPublicPEM returns the PKIX public-key PEM of an EC private key, which must
// be on the curve the algorithm names.
func ecPublicPEM(fn, algo string, keyPem []byte) ([]byte, error) {
	priv, err := parseECPrivate(fn, keyPem)
	if err != nil {
		return nil, err
	}
	got, err := curveName(priv.Curve)
	if err != nil || ecAlgoCurve[algo] != got {
		return nil, fmt.Errorf("crypto.%s: %q needs a key on that curve, got %s", fn, algo, priv.Curve.Params().Name)
	}
	return publicPEM(fn, &priv.PublicKey)
}
