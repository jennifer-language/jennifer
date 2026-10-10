// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

// Standard-Go implementation of the crypto library's post-quantum KEM surface:
// ML-KEM (NIST FIPS 203, formerly Kyber), over crypto/mlkem. ML-KEM-768 is the
// recommended parameter set and the one exposed. crypto/mlkem pulls in the
// FIPS 140 module machinery (SHA-3 / Keccak), which is not part of the TinyGo
// build, so jennifer-tiny selects cryptolib_pqc_tiny.go (a friendly stub) - the
// same build-tag split as the RSA / ECDSA surface.
//
// The three verbs are a textbook KEM: mlkemKeypair draws a fresh keypair,
// mlkemEncapsulate wraps a fresh 32-byte shared secret under a public key
// (returning the ciphertext to send plus the secret to keep), and
// mlkemDecapsulate recovers that secret from the ciphertext with the private
// key. The shared secret is key-establishment material, NOT an encryption key:
// run it through crypto.hkdf and then crypto.encrypt (AES-256-GCM). ML-KEM alone
// is also not a complete migration - real deployments combine it with a
// classical exchange (X25519) as a hybrid KEM.
package cryptolib

import (
	"crypto/mlkem"
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// mlkemKeypairFn implements crypto.mlkemKeypair() -> crypto.Keypair. `public` is
// the 1184-byte ML-KEM-768 encapsulation key to publish; `private` is the 64-byte
// seed to keep (feed it back to mlkemDecapsulate). Reuses the crypto.Keypair
// struct (public / private bytes) like the Ed25519 keypair.
func mlkemKeypairFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 0 {
		return interpreter.Null(), fmt.Errorf("crypto.mlkemKeypair expects no arguments, got %d", len(args))
	}
	dk, err := mlkem.GenerateKey768()
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.mlkemKeypair: %v", err)
	}
	return interpreter.NamespacedStructVal(LibraryName, "Keypair", []interpreter.StructField{
		{Name: "public", Value: interpreter.BytesVal(dk.EncapsulationKey().Bytes())},
		{Name: "private", Value: interpreter.BytesVal(dk.Bytes())},
	}), nil
}

// mlkemEncapsulateFn implements crypto.mlkemEncapsulate(public) ->
// crypto.Encapsulation. It wraps a fresh 32-byte shared secret under the public
// key, returning the 1088-byte ciphertext to send to the key holder and the
// secret to keep. The two sides end up with the same secret once the holder
// decapsulates the ciphertext.
func mlkemEncapsulateFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("crypto.mlkemEncapsulate expects 1 argument (public), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crypto.mlkemEncapsulate: public key must be bytes")
	}
	ek, err := mlkem.NewEncapsulationKey768(args[0].Bytes)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.mlkemEncapsulate: invalid public key: %v", err)
	}
	secret, ciphertext := ek.Encapsulate()
	return interpreter.NamespacedStructVal(LibraryName, "Encapsulation", []interpreter.StructField{
		{Name: "ciphertext", Value: interpreter.BytesVal(ciphertext)},
		{Name: "sharedSecret", Value: interpreter.BytesVal(secret)},
	}), nil
}

// mlkemDecapsulateFn implements crypto.mlkemDecapsulate(private, ciphertext) ->
// bytes (the 32-byte shared secret). A malformed private key or a wrong-length
// ciphertext is a catchable error; a ciphertext that was not produced for this
// key yields a different (useless) secret by design - ML-KEM's implicit
// rejection, never an error - so the two sides simply fail to agree.
func mlkemDecapsulateFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("crypto.mlkemDecapsulate expects 2 arguments (private, ciphertext), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes || args[1].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crypto.mlkemDecapsulate: private key and ciphertext must be bytes")
	}
	dk, err := mlkem.NewDecapsulationKey768(args[0].Bytes)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("crypto.mlkemDecapsulate: invalid private key: %v", err)
	}
	secret, derr := dk.Decapsulate(args[1].Bytes)
	if derr != nil {
		return interpreter.Null(), fmt.Errorf("crypto.mlkemDecapsulate: %v", derr)
	}
	return interpreter.BytesVal(secret), nil
}
