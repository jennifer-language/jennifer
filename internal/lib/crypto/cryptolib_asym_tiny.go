// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build tinygo

// jennifer-tiny stubs for the RSA and ECDSA algorithms ("rsa-sha256",
// "ecdsa-sha256", "rsa2048", "p256", "rsa-oaep-sha256", ...): they need
// crypto/x509, which is off the TinyGo build. Each names the algorithm asked
// for, so the error says what to change.
package cryptolib

import "fmt"

func asymUnavailable(fn, algo string) error {
	return fmt.Errorf("crypto.%s: %q is not available on jennifer-tiny (RSA and ECDSA need crypto/x509, which is off the TinyGo build); use the default jennifer binary, or ed25519", fn, algo)
}

func rsaSign(fn, algo, hashName string, msg, keyPem []byte) ([]byte, error) {
	return nil, asymUnavailable(fn, algo)
}

func rsaVerify(fn, algo, hashName string, msg, sig, keyPem []byte) (bool, error) {
	return false, asymUnavailable(fn, algo)
}

func ecdsaSign(fn, algo, hashName string, msg, keyPem []byte) ([]byte, error) {
	return nil, asymUnavailable(fn, algo)
}

func ecdsaVerify(fn, algo, hashName string, msg, sig, keyPem []byte) (bool, error) {
	return false, asymUnavailable(fn, algo)
}

func rsaOAEP(fn, algo string, wrap bool, data, keyPem []byte) ([]byte, error) {
	return nil, asymUnavailable(fn, algo)
}

func rsaKeypair(fn, algo string, bits int) (pub, priv []byte, err error) {
	return nil, nil, asymUnavailable(fn, algo)
}

func ecKeypair(fn, algo string) (pub, priv []byte, err error) {
	return nil, nil, asymUnavailable(fn, algo)
}

func rsaPublicPEM(fn, algo string, bits int, keyPem []byte) ([]byte, error) {
	return nil, asymUnavailable(fn, algo)
}

func ecPublicPEM(fn, algo string, keyPem []byte) ([]byte, error) {
	return nil, asymUnavailable(fn, algo)
}
