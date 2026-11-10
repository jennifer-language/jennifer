// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

// Standard-Go implementation of the crypto library's post-quantum KEM surface:
// ML-KEM (NIST FIPS 203, formerly Kyber), over crypto/mlkem. ML-KEM-768 is the
// recommended parameter set and the one exposed. crypto/mlkem pulls in the
// FIPS 140 module machinery (SHA-3 / Keccak), which is not part of the TinyGo
// build, so jennifer-tiny selects cryptolib_pqc_tiny.go (a friendly stub) - the
// same build-tag split as the RSA / ECDSA surface.
//
// It backs the "mlkem768" algorithm of crypto.keypair, crypto.publicKey,
// crypto.encapsulate and crypto.decapsulate: a textbook KEM. encapsulate wraps
// a fresh 32-byte shared secret under a public key (the ciphertext to send plus
// the secret to keep), and decapsulate recovers it with the private key. The
// shared secret is key-establishment material, NOT an encryption key: run it
// through crypto.hkdf and then crypto.encrypt("aes-gcm", ...). ML-KEM alone
// is also not a complete migration - real deployments combine it with a
// classical exchange (X25519) as a hybrid KEM.
package cryptolib

import (
	"crypto/mlkem"
	"fmt"
)

// mlkemKeypair generates an ML-KEM-768 key: the 1184-byte encapsulation key
// (public) and the 64-byte seed (private).
func mlkemKeypair(fn string) (pub, priv []byte, err error) {
	dk, err := mlkem.GenerateKey768()
	if err != nil {
		return nil, nil, fmt.Errorf("crypto.%s: %v", fn, err)
	}
	return dk.EncapsulationKey().Bytes(), dk.Bytes(), nil
}

func mlkemPrivate(fn string, priv []byte) (*mlkem.DecapsulationKey768, error) {
	dk, err := mlkem.NewDecapsulationKey768(priv)
	if err != nil {
		return nil, fmt.Errorf("crypto.%s: invalid mlkem768 private key: %v", fn, err)
	}
	return dk, nil
}

// mlkemPublic derives the encapsulation key from a stored private seed.
func mlkemPublic(fn string, priv []byte) ([]byte, error) {
	dk, err := mlkemPrivate(fn, priv)
	if err != nil {
		return nil, err
	}
	return dk.EncapsulationKey().Bytes(), nil
}

// mlkemEncapsulate wraps a fresh shared secret under a public key.
func mlkemEncapsulate(fn string, pub []byte) (ct, secret []byte, err error) {
	ek, err := mlkem.NewEncapsulationKey768(pub)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto.%s: invalid mlkem768 public key: %v", fn, err)
	}
	secret, ct = ek.Encapsulate()
	return ct, secret, nil
}

// mlkemDecapsulate recovers the shared secret from a ciphertext.
func mlkemDecapsulate(fn string, ct, priv []byte) ([]byte, error) {
	dk, err := mlkemPrivate(fn, priv)
	if err != nil {
		return nil, err
	}
	secret, err := dk.Decapsulate(ct)
	if err != nil {
		return nil, fmt.Errorf("crypto.%s: %v", fn, err)
	}
	return secret, nil
}
